package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/api/middleware"
	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/db"
)

type stubTransport func(*http.Request) (*http.Response, error)

func (fn stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestAuditTrailEndpointsEnforceAdminRBACAndExportSafely(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)

	// Create a non-admin user and generate a standard user JWT.
	memberResp := requestJSON(t, router, "POST", "/api/users", admin, map[string]any{
		"name":     "Standard Operator",
		"email":    "operator@example.com",
		"password": "StrongOperatorPassword123!",
		"is_admin": false,
	})
	if memberResp.Code != http.StatusCreated {
		t.Fatalf("create user failed: %s", memberResp.Body.String())
	}
	memberID := responseID(t, memberResp, "user")
	var member db.User
	if err := db.DB.First(&member, "id = ?", memberID).Error; err != nil {
		t.Fatal(err)
	}
	memberToken, err := middleware.GenerateToken(&member, "test-secret-key-12345", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	// Non-admin must be rejected from all audit & webhook endpoints.
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/audit-logs"},
		{"GET", "/api/audit-logs/verify"},
		{"GET", "/api/audit-logs/export?format=csv"},
		{"GET", "/api/webhooks"},
		{"POST", "/api/webhooks"},
		{"DELETE", "/api/webhooks/wh_missing"},
	} {
		mustStatus(t, router, tc.method, tc.path, memberToken, nil, http.StatusForbidden)
	}

	// Trigger a formula-like summary via a resource group name starting with '=' to check CSV neutralization.
	rg := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name":       "=CMD('calc')",
		"cidr_block": "10.188.0.0/24",
	})
	if rg.Code != http.StatusCreated {
		t.Fatalf("create resource group: %s", rg.Body.String())
	}

	// Verify audit logs list & search filter.
	listed := mustStatus(t, router, "GET", "/api/audit-logs?action=resource_group.created", admin, nil, http.StatusOK)
	var listBody struct {
		AuditLogs []db.AuditLog `json:"audit_logs"`
		Total     int64         `json:"total"`
	}
	if err := json.Unmarshal([]byte(listed), &listBody); err != nil {
		t.Fatal(err)
	}
	if listBody.Total < 1 || len(listBody.AuditLogs) < 1 {
		t.Fatalf("expected resource_group.created audit log, got %s", listed)
	}

	// Verify cryptographic chain via API.
	verified := mustStatus(t, router, "GET", "/api/audit-logs/verify", admin, nil, http.StatusOK)
	if !strings.Contains(verified, `"valid":true`) {
		t.Fatalf("expected valid chain response, got %s", verified)
	}

	// Record an entry whose summary begins with '=' to verify CSV formula neutralization.
	if _, err := audit.Record(t.Context(), db.DB, audit.EntryInput{
		ActorEmail: "admin@swanpanel.internal",
		ActorRole:  "admin",
		Action:     "audit.formula_check",
		Severity:   audit.SeverityInfo,
		Summary:    "=HYPERLINK(\"https://evil.example\",\"click\")",
	}); err != nil {
		t.Fatal(err)
	}

	// Verify CSV export and JSONL export.
	csvBody := mustStatus(t, router, "GET", "/api/audit-logs/export?format=csv", admin, nil, http.StatusOK)
	if !strings.Contains(csvBody, "sequence,id,created_at,severity,action") {
		t.Fatalf("missing CSV header: %s", csvBody)
	}
	if !strings.Contains(csvBody, "'=HYPERLINK") {
		t.Fatalf("expected CSV formula prefix neutralization, got: %s", csvBody)
	}
	jsonlBody := mustStatus(t, router, "GET", "/api/audit-logs/export?format=jsonl", admin, nil, http.StatusOK)
	if !strings.Contains(jsonlBody, `"action":"resource_group.created"`) {
		t.Fatalf("missing JSONL audit entry: %s", jsonlBody)
	}
	mustStatus(t, router, "GET", "/api/audit-logs/export?format=xml", admin, nil, http.StatusBadRequest)
}

func TestAlertWebhookCRUDValidationAndTestDelivery(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)

	// Reject invalid provider or missing Telegram chat ID.
	mustStatus(t, router, "POST", "/api/webhooks", admin, map[string]any{
		"name":          "Bad",
		"provider_type": "pagerduty",
		"webhook_url":   "https://example.com/hook",
		"events":        []string{"*"},
	}, http.StatusBadRequest)
	mustStatus(t, router, "POST", "/api/webhooks", admin, map[string]any{
		"name":          "Missing Chat",
		"provider_type": "telegram",
		"webhook_url":   "https://api.telegram.org/bot123/sendMessage",
		"events":        []string{"*"},
	}, http.StatusBadRequest)

	// Create valid generic webhook with secret.
	created := requestJSON(t, router, "POST", "/api/webhooks", admin, map[string]any{
		"name":           "SOC Webhook",
		"provider_type":  "generic",
		"webhook_url":    "https://soc.example.com/hooks/secret-token-xyz",
		"signing_secret": "hmac-secret-value",
		"events":         []string{"device.*", "auth.bruteforce.suspected"},
		"is_active":      true,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create webhook failed: %s", created.Body.String())
	}
	if strings.Contains(created.Body.String(), "secret-token-xyz") || strings.Contains(created.Body.String(), "hmac-secret-value") {
		t.Fatalf("webhook creation response leaked raw secret/token: %s", created.Body.String())
	}
	hookID := responseID(t, created, "webhook")

	// Update webhook without re-entering URL.
	updated := mustStatus(t, router, "PUT", "/api/webhooks/"+hookID, admin, map[string]any{
		"name":      "SOC Primary Webhook",
		"is_active": true,
	}, http.StatusOK)
	if !strings.Contains(updated, "SOC Primary Webhook") || !strings.Contains(updated, `"has_signing_secret":true`) {
		t.Fatalf("unexpected webhook update response: %s", updated)
	}

	// Test webhook delivery using mocked transport (no real network required).
	restore := audit.SetWebhookClientForTest(&http.Client{
		Transport: stubTransport(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("X-SwanPanel-Signature") == "" {
				t.Error("missing X-SwanPanel-Signature header")
			}
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(bytes.NewBufferString(`{"ok":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	})
	defer restore()

	testRes := mustStatus(t, router, "POST", "/api/webhooks/"+hookID+"/test", admin, map[string]any{}, http.StatusOK)
	if !strings.Contains(testRes, `"success":true`) {
		t.Fatalf("expected successful test delivery, got %s", testRes)
	}

	// Delete webhook and confirm 404 on repeat.
	mustStatus(t, router, "DELETE", "/api/webhooks/"+hookID, admin, nil, http.StatusOK)
	mustStatus(t, router, "DELETE", "/api/webhooks/"+hookID, admin, nil, http.StatusNotFound)
}
