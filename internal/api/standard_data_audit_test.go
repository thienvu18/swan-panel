package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
)

// TestStandardDataAudit verifies end-to-end functionality using realistic enterprise data
// covering all recently added and updated features.
func TestStandardDataAudit(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()

	// -------------------------------------------------------------
	// 1. Initial Setup Status & Administrator Provisioning
	// -------------------------------------------------------------
	req := httptest.NewRequest(http.MethodGet, "/api/auth/setup/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for setup status, got %d", w.Code)
	}

	// Read setup token from BootstrapTokenFile
	tokenFile := db.BootstrapTokenFile()
	tokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("failed to read setup token file: %v", err)
	}
	setupToken := strings.TrimSpace(string(tokenBytes))

	// Provision initial administrator
	setupPayload := map[string]string{
		"token":    setupToken,
		"name":     "Enterprise Admin",
		"email":    "admin@enterprise.corp",
		"password": "Corporate-Admin-Password-2026!",
	}
	setupBody, _ := json.Marshal(setupPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(string(setupBody)))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("expected 201/200 from admin setup, got %d: %s", w.Code, w.Body.String())
	}

	// Login with newly created admin credentials to get token
	loginPayload := map[string]string{
		"email":    "admin@enterprise.corp",
		"password": "Corporate-Admin-Password-2026!",
	}
	loginBody, _ := json.Marshal(loginPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(string(loginBody)))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from admin login, got %d: %s", w.Code, w.Body.String())
	}

	var setupRes struct {
		Token string `json:"token"`
		User  struct {
			ID      string `json:"id"`
			Email   string `json:"email"`
			IsAdmin bool   `json:"is_admin"`
		} `json:"user"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &setupRes); err != nil {
		t.Fatalf("failed to parse setup response: %v", err)
	}
	if !setupRes.User.IsAdmin || setupRes.Token == "" {
		t.Fatalf("invalid admin setup response: %+v", setupRes)
	}
	adminToken := setupRes.Token

	// Verify token is one-time only
	req = httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(string(setupBody)))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Fatalf("setup token must not be reusable")
	}

	// -------------------------------------------------------------
	// 2. Subnet / Resource Group Provisioning
	// -------------------------------------------------------------
	subnetPayload := map[string]string{
		"name":        "Engineering VPC",
		"cidr_block":  "10.150.0.0/24",
		"description": "Core Engineering & Microservices Enclave",
	}
	subnetBody, _ := json.Marshal(subnetPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/resource-groups", strings.NewReader(string(subnetBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create subnet: %d %s", w.Code, w.Body.String())
	}
	var subnetRes struct {
		ResourceGroup struct {
			ID        string `json:"id"`
			CIDRBlock string `json:"cidr_block"`
		} `json:"resource_group"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &subnetRes)
	engineeringVPCID := subnetRes.ResourceGroup.ID

	// -------------------------------------------------------------
	// 3. User & User Group Management
	// -------------------------------------------------------------
	// Create User Groups: Core-Engineering and External-Contractors
	groupEngPayload := map[string]any{
		"name":        "Core-Engineering",
		"description": "Internal Full-Time Engineering Team",
	}
	groupEngBody, _ := json.Marshal(groupEngPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/user-groups", strings.NewReader(string(groupEngBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create Core-Engineering group: %d %s", w.Code, w.Body.String())
	}
	var groupEngRes struct {
		UserGroup struct {
			ID string `json:"id"`
		} `json:"user_group"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &groupEngRes)
	engGroupID := groupEngRes.UserGroup.ID

	groupContractorPayload := map[string]any{
		"name":        "External-Contractors",
		"description": "Third-Party Contractors",
	}
	groupContractorBody, _ := json.Marshal(groupContractorPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/user-groups", strings.NewReader(string(groupContractorBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create External-Contractors group: %d %s", w.Code, w.Body.String())
	}
	var groupContractorRes struct {
		UserGroup struct {
			ID string `json:"id"`
		} `json:"user_group"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &groupContractorRes)
	contractorGroupID := groupContractorRes.UserGroup.ID

	// Create Alice (Engineer) in Core-Engineering
	alicePayload := map[string]any{
		"name":           "Alice Chen",
		"email":          "alice.chen@enterprise.corp",
		"password":       "Alice-Secure-Pass-2026!",
		"is_admin":       false,
		"user_group_ids": []string{engGroupID},
	}
	aliceBody, _ := json.Marshal(alicePayload)
	req = httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(string(aliceBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create Alice: %d %s", w.Code, w.Body.String())
	}
	var aliceRes struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &aliceRes)
	aliceID := aliceRes.User.ID

	// Create Bob (Contractor) in External-Contractors
	bobPayload := map[string]any{
		"name":           "Bob Smith",
		"email":          "bob.smith@enterprise.corp",
		"password":       "Bob-Secure-Pass-2026!",
		"is_admin":       false,
		"user_group_ids": []string{contractorGroupID},
	}
	bobBody, _ := json.Marshal(bobPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(string(bobBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create Bob: %d %s", w.Code, w.Body.String())
	}
	var bobRes struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &bobRes)
	bobID := bobRes.User.ID

	// -------------------------------------------------------------
	// 4. Device Provisioning Across All Platforms
	// -------------------------------------------------------------
	// 4a. Alice MacBook (macOS, Company-owned, Static VIP)
	macPayload := map[string]any{
		"hostname":           "alice-macbook-pro",
		"device_type":        "company",
		"resource_group_id":  engineeringVPCID,
		"ip_allocation_type": "static",
		"virtual_ip":         "10.150.0.10",
		"platform":           "macos",
	}
	macBody, _ := json.Marshal(macPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/devices", strings.NewReader(string(macBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create Alice Mac: %d %s", w.Code, w.Body.String())
	}
	var macRes struct {
		Device struct {
			ID string `json:"id"`
		} `json:"device"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &macRes)
	aliceMacID := macRes.Device.ID

	// 4b. Alice Android Device (Android native, User-owned)
	androidPayload := map[string]any{
		"hostname":           "alice-pixel-8",
		"device_type":        "user",
		"user_id":            aliceID,
		"resource_group_id":  engineeringVPCID,
		"ip_allocation_type": "dynamic",
		"platform":           "android",
	}
	androidBody, _ := json.Marshal(androidPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/devices", strings.NewReader(string(androidBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create Alice Android: %d %s", w.Code, w.Body.String())
	}
	var androidRes struct {
		Device struct {
			ID string `json:"id"`
		} `json:"device"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &androidRes)
	aliceAndroidID := androidRes.Device.ID

	// 4c. Bob Windows ThinkPad (Windows, User-owned)
	winPayload := map[string]any{
		"hostname":           "bob-thinkpad-x1",
		"device_type":        "user",
		"user_id":            bobID,
		"resource_group_id":  engineeringVPCID,
		"ip_allocation_type": "dynamic",
		"platform":           "windows",
	}
	winBody, _ := json.Marshal(winPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/devices", strings.NewReader(string(winBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create Bob Windows: %d %s", w.Code, w.Body.String())
	}
	var winRes struct {
		Device struct {
			ID string `json:"id"`
		} `json:"device"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &winRes)
	bobWinID := winRes.Device.ID

	// -------------------------------------------------------------
	// 5. Native Client Profiles Verification
	// -------------------------------------------------------------
	// macOS .mobileconfig download
	req = httptest.NewRequest(http.MethodGet, "/api/devices/"+aliceMacID+"/config/macos", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to download macOS config: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<!DOCTYPE plist") {
		t.Fatalf("macOS profile is not a valid plist XML")
	}

	// Windows PowerShell .ps1 download
	req = httptest.NewRequest(http.MethodGet, "/api/devices/"+bobWinID+"/config/windows", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to download Windows config: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Add-VpnConnection") {
		t.Fatalf("Windows script does not contain Add-VpnConnection")
	}

	// Android Native Setup Endpoint
	req = httptest.NewRequest(http.MethodGet, "/api/devices/"+aliceAndroidID+"/config/android-native", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to get Android native setup: %d %s", w.Code, w.Body.String())
	}
	var androidSetupRes struct {
		ClientP12      string `json:"client_p12"`
		ImportPassword string `json:"import_password"`
		ServerAddress  string `json:"server_address"`
		ServerIdentity string `json:"server_identity"`
		ClientIdentity string `json:"client_identity"`
		RootCA         string `json:"root_ca"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &androidSetupRes); err != nil {
		t.Fatalf("failed to parse Android setup: %v", err)
	}
	if androidSetupRes.ClientP12 == "" || androidSetupRes.ImportPassword == "" || androidSetupRes.RootCA == "" {
		t.Fatalf("incomplete Android native setup payload: %+v", androidSetupRes)
	}

	// -------------------------------------------------------------
	// 6. Zero-Trust Access Rules & Effective Access Evaluation
	// -------------------------------------------------------------
	// Rule 1: ALLOW Core-Engineering -> Engineering VPC (10.150.0.0/24)
	ruleAllowPayload := map[string]any{
		"subject_type": "user_group",
		"subject_id":   engGroupID,
		"action":       "ALLOW",
		"target_type":  "resource_group",
		"target_value": engineeringVPCID,
	}
	ruleAllowBody, _ := json.Marshal(ruleAllowPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/access-rules", strings.NewReader(string(ruleAllowBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create ALLOW rule: %d %s", w.Code, w.Body.String())
	}

	// Rule 2: DENY Core-Engineering -> 10.150.0.99/32 (Restricted Core Bastion)
	ruleDenyPayload := map[string]any{
		"subject_type": "user_group",
		"subject_id":   engGroupID,
		"action":       "DENY",
		"target_type":  "cidr_ip",
		"target_value": "10.150.0.99/32",
	}
	ruleDenyBody, _ := json.Marshal(ruleDenyPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/access-rules", strings.NewReader(string(ruleDenyBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create DENY rule: %d %s", w.Code, w.Body.String())
	}

	// 6a. Evaluate Effective Policy for Alice's Android Phone (owned by Alice in Core-Engineering):
	// Allowed destination: 10.150.0.15 -> should be ALLOW
	effURL := "/api/access-rules/effective/" + url.PathEscape(aliceAndroidID) + "?destination=10.150.0.15"
	req = httptest.NewRequest(http.MethodGet, effURL, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to get effective policy: %d %s", w.Code, w.Body.String())
	}
	var effAliceAllowed struct {
		Policy struct {
			AllowedSelectors []string `json:"allowed_selectors"`
			DeniedSelectors  []string `json:"denied_selectors"`
		} `json:"policy"`
		Decision struct {
			Action      string `json:"action"`
			Reason      string `json:"reason"`
			MatchedCIDR string `json:"matched_cidr"`
		} `json:"decision"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &effAliceAllowed); err != nil {
		t.Fatalf("failed to parse effective policy response: %v", err)
	}
	if effAliceAllowed.Decision.Action != "ALLOW" || effAliceAllowed.Decision.Reason != "explicit_allow" {
		t.Fatalf("expected explicit ALLOW for 10.150.0.15, got: %+v", effAliceAllowed.Decision)
	}

	// Denied destination: 10.150.0.99 -> should be DENY (explicit deny takes precedence over allow)
	effURLDeny := "/api/access-rules/effective/" + url.PathEscape(aliceAndroidID) + "?destination=10.150.0.99"
	req = httptest.NewRequest(http.MethodGet, effURLDeny, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to get effective policy: %d %s", w.Code, w.Body.String())
	}
	var effAliceDenied struct {
		Decision struct {
			Action      string `json:"action"`
			Reason      string `json:"reason"`
			MatchedCIDR string `json:"matched_cidr"`
		} `json:"decision"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &effAliceDenied)
	if effAliceDenied.Decision.Action != "DENY" || effAliceDenied.Decision.Reason != "explicit_deny" {
		t.Fatalf("expected explicit DENY for 10.150.0.99, got: %+v", effAliceDenied.Decision)
	}

	// Outside subnet destination: 192.168.1.100 -> should be default_deny
	effURLOutside := "/api/access-rules/effective/" + url.PathEscape(aliceAndroidID) + "?destination=192.168.1.100"
	req = httptest.NewRequest(http.MethodGet, effURLOutside, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to get effective policy: %d %s", w.Code, w.Body.String())
	}
	var effAliceDefaultDeny struct {
		Decision struct {
			Action string `json:"action"`
			Reason string `json:"reason"`
		} `json:"decision"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &effAliceDefaultDeny)
	if effAliceDefaultDeny.Decision.Action != "DENY" || effAliceDefaultDeny.Decision.Reason != "default_deny" {
		t.Fatalf("expected default_deny for 192.168.1.100, got: %+v", effAliceDefaultDeny.Decision)
	}

	// 6b. Evaluate Effective Policy for Bob's ThinkPad (Contractor):
	// Destination: 10.150.0.15 -> should be default_deny because no rule grants access to Contractors!
	effURLBob := "/api/access-rules/effective/" + url.PathEscape(bobWinID) + "?destination=10.150.0.15"
	req = httptest.NewRequest(http.MethodGet, effURLBob, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to get effective policy: %d %s", w.Code, w.Body.String())
	}
	var effBob struct {
		Decision struct {
			Action string `json:"action"`
			Reason string `json:"reason"`
		} `json:"decision"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &effBob)
	if effBob.Decision.Action != "DENY" || effBob.Decision.Reason != "default_deny" {
		t.Fatalf("expected default_deny for Bob contractor, got: %+v", effBob.Decision)
	}

	// -------------------------------------------------------------
	// 7. DNS Engine Split-DNS Overrides
	// -------------------------------------------------------------
	dnsPayload := map[string]string{
		"hostname":       "gitlab.internal.enterprise.corp",
		"record_type":    "A",
		"target_address": "10.150.0.50",
	}
	dnsBody, _ := json.Marshal(dnsPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/dns/overrides", strings.NewReader(string(dnsBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create DNS override: %d %s", w.Code, w.Body.String())
	}

	// -------------------------------------------------------------
	// 8. OIDC Provider Configuration & Secret Masking
	// -------------------------------------------------------------
	oidcPayload := map[string]string{
		"name":          "Enterprise SSO",
		"provider_type": "keycloak",
		"issuer_url":    "https://sso.enterprise.corp/realms/master",
		"client_id":     "enterprise-client-id",
		"client_secret": "super-confidential-secret-key-12345",
	}
	oidcBody, _ := json.Marshal(oidcPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/oidc/providers", strings.NewReader(string(oidcBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create OIDC provider: %d %s", w.Code, w.Body.String())
	}

	// List OIDC providers and ensure client secret is not leaked in plain text
	req = httptest.NewRequest(http.MethodGet, "/api/oidc/providers", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to list OIDC providers: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "super-confidential-secret-key-12345") {
		t.Fatalf("OIDC client secret leaked in list response!")
	}

	// -------------------------------------------------------------
	// 9. Server Settings & CA Inspection
	// -------------------------------------------------------------
	req = httptest.NewRequest(http.MethodGet, "/api/settings/strongswan", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to get strongswan settings: %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/settings/ca", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to get CA identity: %d", w.Code)
	}
	var caRes struct {
		CA struct {
			CommonName string `json:"common_name"`
			Serial     string `json:"serial"`
		} `json:"ca"`
		CanRotate bool `json:"can_rotate"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &caRes); err != nil {
		t.Fatalf("failed to parse CA response: %v", err)
	}
	if caRes.CA.CommonName == "" || caRes.CA.Serial == "" {
		t.Fatalf("invalid CA response: %+v", caRes)
	}

	// -------------------------------------------------------------
	// 10. Device Revocation Safeguard
	// -------------------------------------------------------------
	req = httptest.NewRequest(http.MethodPost, "/api/devices/"+aliceMacID+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to revoke device: %d %s", w.Code, w.Body.String())
	}

	// Policy for revoked device must fail
	req = httptest.NewRequest(http.MethodGet, "/api/access-rules/effective/"+aliceMacID, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 bad request for revoked device effective policy, got %d: %s", w.Code, w.Body.String())
	}
}
