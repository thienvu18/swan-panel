package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/db"
)

func TestAdminEffectivePolicyExplainsAllowDenyAndRuntimeState(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)

	groupID, deviceID := uuid.NewString(), uuid.NewString()
	if err := db.DB.Create(&db.ResourceGroup{ID: groupID, Name: "Effective Test", CIDRBlock: "10.122.0.0/24"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Create(&db.Device{ID: deviceID, Hostname: "effective-test", DeviceType: "company",
		ResourceGroupID: groupID, VirtualIP: "10.122.0.2", IPAllocationType: "static", Platform: "ios"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Save(&db.RuntimeApplyState{ID: 1, Status: "applied", DesiredGeneration: 7, AppliedGeneration: 7}).Error; err != nil {
		t.Fatal(err)
	}

	request := func(target, auth string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		router.ServeHTTP(recorder, req)
		return recorder
	}
	endpoint := "/api/access-rules/effective/" + url.PathEscape(deviceID)

	if actual := request(endpoint, ""); actual.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated policy access: %d", actual.Code)
	}
	createAdminFixture(t, router, token, "effective-policy-viewer")
	viewerLogin := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]string{
		"email": "effective-policy-viewer@example.invalid", "password": "Test-Secure-Pass-123!",
	})
	if viewerLogin.Code != http.StatusOK {
		t.Fatalf("viewer login failed: %s", viewerLogin.Body.String())
	}
	var viewer struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(viewerLogin.Body.Bytes(), &viewer); err != nil {
		t.Fatal(err)
	}
	if actual := request(endpoint, viewer.Token); actual.Code != http.StatusForbidden {
		t.Fatalf("non-admin could inspect policy: %d", actual.Code)
	}
	if actual := request("/api/access-rules/effective/not-a-uuid", token); actual.Code != http.StatusBadRequest {
		t.Fatalf("invalid ID: %d", actual.Code)
	}

	var response struct {
		Policy struct {
			Allowed []string `json:"allowed_selectors"`
			Denied  []string `json:"denied_selectors"`
		} `json:"policy"`
		Runtime struct {
			Status  string `json:"status"`
			Desired uint64 `json:"desired_generation"`
			Applied uint64 `json:"applied_generation"`
		} `json:"runtime"`
		Decision struct {
			Action      string `json:"action"`
			Reason      string `json:"reason"`
			RuleSources []struct {
				ID          string `json:"id"`
				SubjectType string `json:"subject_type"`
			} `json:"rule_sources"`
		} `json:"decision"`
	}

	getDecision := func(address string) string {
		t.Helper()
		w := request(endpoint+"?destination="+url.QueryEscape(address), token)
		if w.Code != http.StatusOK {
			t.Fatalf("effective policy endpoint returned %d: %s", w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Runtime.Status != "applied" || response.Runtime.Desired != 7 || response.Runtime.Applied != 7 {
			t.Fatalf("incorrect gateway apply state: %+v", response.Runtime)
		}
		return response.Decision.Action + "/" + response.Decision.Reason
	}

	if got := getDecision("10.99.2.10"); got != "DENY/default_deny" {
		t.Fatalf("unexpected initial decision: %s", got)
	}

	allowRuleID, denyRuleID := uuid.NewString(), uuid.NewString()
	for _, rule := range []db.AccessRule{
		{ID: allowRuleID, SubjectType: "device", SubjectID: deviceID, Action: "ALLOW", TargetType: "cidr_ip", TargetValue: "10.99.0.0/16"},
		{ID: denyRuleID, SubjectType: "device", SubjectID: deviceID, Action: "DENY", TargetType: "cidr_ip", TargetValue: "10.99.1.0/24"},
	} {
		if err := db.DB.Create(&rule).Error; err != nil {
			t.Fatal(err)
		}
	}

	if got := getDecision("10.99.2.10"); got != "ALLOW/explicit_allow" {
		t.Fatalf("ALLOW mismatch: %s", got)
	}
	if len(response.Decision.RuleSources) != 1 || response.Decision.RuleSources[0].ID != allowRuleID ||
		response.Decision.RuleSources[0].SubjectType != "device" {
		t.Fatalf("ALLOW source mismatch: %+v", response.Decision.RuleSources)
	}
	if got := getDecision("10.99.1.10"); got != "DENY/explicit_deny" {
		t.Fatalf("DENY precedence mismatch: %s", got)
	}
	if len(response.Decision.RuleSources) != 1 || response.Decision.RuleSources[0].ID != denyRuleID {
		t.Fatalf("DENY source mismatch: %+v", response.Decision.RuleSources)
	}
	if got := request(endpoint+"?destination=not-an-address", token); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid destination code: %d", got.Code)
	}
}
