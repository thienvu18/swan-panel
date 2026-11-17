package api_test

import (
	"encoding/json"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/logger"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemStatusLogFilteringAndOfflineDaemonControls(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	status := mustStatus(t, router, "GET", "/api/system/daemon/status", token, nil, 503)
	if !strings.Contains(status, "unknown") {
		t.Fatalf("offline daemon reported healthy: %s", status)
	}
	mustStatus(t, router, "GET", "/api/sessions", token, nil, 503)
	mustStatus(t, router, "POST", "/api/sessions/terminate", token, nil, 400)
	mustStatus(t, router, "POST", "/api/sessions/terminate", token, map[string]any{"ike_name": "missing"}, 503)
	mustStatus(t, router, "POST", "/api/system/daemon", token, map[string]any{"action": "clear"}, 400)
	mustStatus(t, router, "POST", "/api/system/daemon", token, map[string]any{}, 400)
	mustStatus(t, router, "GET", "/api/system/apply-status", token, nil, 200)
	result := mustStatus(t, router, "POST", "/api/system/apply", token, map[string]any{}, 200)
	if !strings.Contains(result, "applied") {
		t.Fatalf("test-mode policy apply did not converge: %s", result)
	}
	mustStatus(t, router, "GET", "/api/system/apply-status", token, nil, 200)

	// The daemon control command is replaced only in this isolated test process;
	// no host strongSwan service is stopped or altered.
	fakeBin := t.TempDir()
	fakePath := filepath.Join(fakeBin, "ipsec")
	if err := os.WriteFile(fakePath, []byte("#!/bin/sh\nexit 13\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)
	mustStatus(t, router, "POST", "/api/system/daemon", token, map[string]any{"action": "stop"}, 503)
	if err := os.WriteFile(fakePath, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "POST", "/api/system/daemon", token, map[string]any{"action": "stop"}, 200)

	logger.AddLogEntry(logger.LogEntry{Level: "warn", Scope: "vpn-auth", Tag1: "192.0.2.3", Tag2: "ike", Message: "A client authentication failed"})
	logger.AddLogEntry(logger.LogEntry{Level: "info", Scope: "dns", Tag1: "127.0.0.1", Message: "Lookup complete"})
	filtered := mustStatus(t, router, "GET", "/api/system/logs?level=warn&search=authentication&scope=vpn", token, nil, 200)
	if !strings.Contains(filtered, "authentication failed") || strings.Contains(filtered, "Lookup complete") {
		t.Fatalf("log filters leaked unrelated event: %s", filtered)
	}
	mustStatus(t, router, "GET", "/api/system/logs?level=all&scope=all", token, nil, 200)
	mustStatus(t, router, "GET", "/api/system/logs?level=error&search=no-such-log", token, nil, 200)
}

func TestAdvancedGatewayRejectsInvalidFieldsAndPreservesStoredConfiguration(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	base := requestJSON(t, router, "GET", "/api/settings/strongswan", token, nil)
	if base.Code != 200 {
		t.Fatal(base.Body.String())
	}
	var original map[string]any
	if err := json.Unmarshal(base.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		changes map[string]any
	}{
		{"invalid duration", map[string]any{"reauth_time": "-1s"}},
		{"invalid fragmentation", map[string]any{"fragmentation": "unsafe"}},
		{"invalid cipher", map[string]any{"cipher_preset": "null"}},
		{"invalid IKE proposal", map[string]any{"ike_proposals": "aaa;INVALID"}},
		{"invalid MSS", map[string]any{"mss_clamping": 200}},
		{"forbidden auth override", map[string]any{"advanced_connection": map[string]any{"local": map[string]any{"auth": "psk"}}}},
		{"forbidden traffic selectors", map[string]any{"advanced_child": map[string]any{"local_ts": []string{"0.0.0.0/0"}}}},
		{"invalid object options", map[string]any{"advanced_child": map[string]any{"opaque": map[string]any{"nested": "bad"}}}},
		{"invalid list entries", map[string]any{"advanced_child": map[string]any{"start_action": []any{"none", 100}}}},
		{"invalid display name", map[string]any{"vpn_name": "Company\nInjected"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := map[string]any{}
			for key, item := range original {
				value[key] = item
			}
			for key, item := range tc.changes {
				value[key] = item
			}
			mustStatus(t, router, "PUT", "/api/settings/strongswan", token, value, http.StatusBadRequest)
		})
	}
	after := mustStatus(t, router, "GET", "/api/settings/strongswan", token, nil, 200)
	if !strings.Contains(after, "vpn.company.com") {
		t.Fatalf("rejected settings modified database: %s", after)
	}
}

func TestRootCAIdentityRejectsUnsafeRotationPayloads(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	current := mustStatus(t, router, "GET", "/api/settings/ca", token, nil, 200)
	if !strings.Contains(current, "can_rotate") {
		t.Fatalf("missing CA lifecycle metadata: %s", current)
	}
	for _, tc := range []struct {
		name    string
		payload map[string]any
		status  int
	}{
		{"no confirmation", map[string]any{"confirm": "no", "common_name": "My CA", "organization": "Example", "country": "VN"}, 400},
		{"invalid country", map[string]any{"confirm": "ROTATE CA", "common_name": "My CA", "organization": "Example", "country": "XYZ"}, 400},
		{"newline injection", map[string]any{"confirm": "ROTATE CA", "common_name": "Injected\nCA", "organization": "Example", "country": "VN"}, 400},
		{"missing subject", map[string]any{"confirm": "ROTATE CA", "country": "VN"}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mustStatus(t, router, "POST", "/api/settings/ca/rotate", token, tc.payload, tc.status)
		})
	}
	// A revoked historic identity must also lock the CA, even if device rows are gone.
	revocation := db.CertificateRevocation{Serial: "deadbeef", DeviceID: "deleted-device"}
	if err := db.DB.Create(&revocation).Error; err != nil {
		t.Fatal(err)
	}
	locked := mustStatus(t, router, "GET", "/api/settings/ca", token, nil, 200)
	if !strings.Contains(locked, "can_rotate\":false") {
		t.Fatal("revocation ledger did not lock CA lifecycle")
	}
	mustStatus(t, router, "POST", "/api/settings/ca/rotate", token, map[string]any{
		"confirm": "ROTATE CA", "common_name": "New CA", "organization": "Example", "country": "VN",
	}, 409)
}

// An empty installation can safely personalize the real CA subject before
// any device is issued. Unlike a VPN display label, this changes the signer.
func TestFreshInstallationRotatesRootCAOnlyWithExplicitConfirmation(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	before := mustStatus(t, router, "GET", "/api/settings/ca", token, nil, 200)
	if !strings.Contains(before, "can_rotate\":true") {
		t.Fatalf("empty installation must permit CA personalization: %s", before)
	}
	rotated := requestJSON(t, router, "POST", "/api/settings/ca/rotate", token, map[string]any{
		"confirm":      "ROTATE CA",
		"common_name":  "QA VPN Internal Root",
		"organization": "Engineering QA",
		"country":      "VN",
		"province":     "Ho Chi Minh",
		"locality":     "District 1",
	})
	if rotated.Code != 200 {
		t.Fatalf("explicit CA initialization failed: %d %s", rotated.Code, rotated.Body.String())
	}
	if !strings.Contains(rotated.Body.String(), "QA VPN Internal Root") {
		t.Fatal("updated CA identity not returned")
	}
	after := mustStatus(t, router, "GET", "/api/settings/ca", token, nil, 200)
	if !strings.Contains(after, "QA VPN Internal Root") {
		t.Fatal("CA subject not persisted in actual certificate")
	}
}

func TestExternalStrongSwanDaemonControlsFailClosed(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	t.Setenv("SWANPANEL_EXTERNAL_STRONGSWAN", "1")
	fakeBin := t.TempDir()
	marker := filepath.Join(fakeBin, "ipsec-invoked")
	ipsec := filepath.Join(fakeBin, "ipsec")
	if err := os.WriteFile(ipsec, []byte("#!/bin/sh\ntouch \""+marker+"\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)
	for _, action := range []string{"start", "stop", "restart"} {
		response := mustStatus(t, router, "POST", "/api/system/daemon", token, map[string]any{"action": action}, http.StatusConflict)
		if !strings.Contains(response, "host") {
			t.Fatalf("external daemon denial lacks actionable explanation: %s", response)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("attempted to control container daemon in external strongSwan mode")
	}
	status := mustStatus(t, router, "GET", "/api/system/daemon/status", token, nil, http.StatusServiceUnavailable)
	if !strings.Contains(status, "\"control_supported\":false") {
		t.Fatalf("external daemon state incorrectly advertises lifecycle control: %s", status)
	}
}
