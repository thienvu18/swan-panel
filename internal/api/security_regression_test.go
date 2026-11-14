package api_test

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/smallstep/pkcs7"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/pki"
)

func requestJSON(t *testing.T, router *gin.Engine, method, url, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func responseID(t *testing.T, r *httptest.ResponseRecorder, top string) string {
	t.Helper()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(r.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var val struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload[top], &val); err != nil {
		t.Fatal(err)
	}
	if val.ID == "" {
		t.Fatalf("missing id from %s: %s", top, r.Body.String())
	}
	return val.ID
}

func TestBootstrapAPIBlocksUntilAdminCreated(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	status := requestJSON(t, router, "GET", "/api/auth/setup/status", "", nil)
	if status.Code != 200 || !strings.Contains(status.Body.String(), `"setup_required":true`) {
		t.Fatalf("new install should require setup: %d %s", status.Code, status.Body.String())
	}
	login := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]string{
		"email": "root@example.com", "password": "not-initialized",
	})
	if login.Code != http.StatusForbidden {
		t.Fatalf("login before setup must be forbidden: %d", login.Code)
	}
	token := getAdminToken(t, router)
	replay := requestJSON(t, router, "POST", "/api/auth/setup", "", map[string]string{
		"name": "Replay", "email": "replay@example.com", "password": "strong-password-123", "token": "bogus",
	})
	if replay.Code == http.StatusCreated {
		t.Fatal("second admin bootstrap accepted")
	}
	status = requestJSON(t, router, "GET", "/api/auth/setup/status", "", nil)
	if status.Code != 200 || !strings.Contains(status.Body.String(), `"setup_required":false`) {
		t.Fatalf("setup remains open: %s", status.Body.String())
	}
	auth := requestJSON(t, router, "GET", "/api/auth/me", token, nil)
	if auth.Code != 200 {
		t.Fatalf("new administrator JWT rejected: %s", auth.Body.String())
	}
}

func TestSignedAppleProfileStableIdentityAndRevocation(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]string{
		"name": "Signed profile network", "cidr_block": "10.78.0.0/24",
	})
	if group.Code != 201 {
		t.Fatalf("group: %s", group.Body.String())
	}
	groupID := responseID(t, group, "resource_group")
	device := requestJSON(t, router, "POST", "/api/devices", admin, map[string]string{
		"hostname": "profile-device", "device_type": "company",
		"resource_group_id": groupID, "ip_allocation_type": "dynamic", "platform": "macos",
	})
	if device.Code != 201 {
		t.Fatalf("device: %d %s", device.Code, device.Body.String())
	}
	deviceID := responseID(t, device, "device")
	manager, err := pki.GetPKIManager()
	if err != nil {
		t.Fatal(err)
	}
	original, err := manager.LoadDeviceBundle(deviceID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		resp := requestJSON(t, router, "GET", "/api/devices/"+deviceID+"/config/apple", admin, nil)
		if resp.Code != 200 {
			t.Fatalf("download %d: %d %s", i, resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Header().Get("Content-Type"), "application/x-apple-aspen-config") {
			t.Fatal("incorrect Apple configuration profile MIME type")
		}
		signed, err := pkcs7.Parse(resp.Body.Bytes())
		if err != nil {
			t.Fatal("profile not valid signed CMS:", err)
		}
		rootPEM, _ := pem.Decode([]byte(original.CACertPEM))
		if rootPEM == nil {
			t.Fatal("invalid trusted CA")
		}
		ca, err := x509.ParseCertificate(rootPEM.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		roots := x509.NewCertPool()
		roots.AddCert(ca)
		if err := signed.VerifyWithChain(roots); err != nil {
			t.Fatalf("downloaded Apple profile signature invalid: %v", err)
		}
		if !bytes.Contains(signed.Content, []byte("profile-device")) {
			t.Fatal("signed profile has wrong device identity")
		}
		again, err := manager.LoadDeviceBundle(deviceID)
		if err != nil {
			t.Fatal(err)
		}
		if again.PrivateKeyPEM != original.PrivateKeyPEM || again.CertPEM != original.CertPEM {
			t.Fatal("download rotated client identity")
		}
	}
	revoked := requestJSON(t, router, "POST", "/api/devices/"+deviceID+"/revoke", admin, nil)
	if revoked.Code != 200 {
		t.Fatalf("revoke failed: %d %s", revoked.Code, revoked.Body.String())
	}
	var ledger db.CertificateRevocation
	if err := db.DB.First(&ledger, "serial = ?", original.SerialNumber).Error; err != nil {
		t.Fatal("revocation not recorded:", err)
	}
	denied := requestJSON(t, router, "GET", "/api/devices/"+deviceID+"/config/apple", admin, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("revoked identity still downloadable: %d", denied.Code)
	}
	if _, err := manager.LoadDeviceBundle(deviceID); err == nil {
		t.Fatal("revoked private key remains exportable")
	}
}

func TestNeverAcceptQueryBearerToken(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	for _, endpoint := range []string{"/api/auth/me", "/api/dashboard/stats", "/api/devices"} {
		resp := requestJSON(t, router, "GET", endpoint+"?token="+token, "", nil)
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("%s accepted token in query: %d", endpoint, resp.Code)
		}
	}
}

func TestJWTPrivilegeAndStatusReadFromDB(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	var admin db.User
	if err := db.DB.Where("email = ?", "admin@swanpanel.internal").First(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Model(&admin).Update("is_admin", false).Error; err != nil {
		t.Fatal(err)
	}
	denied := requestJSON(t, router, "GET", "/api/users", token, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("old admin JWT retained privileges: %d", denied.Code)
	}
	if err := db.DB.Model(&admin).Update("status", "SUSPENDED").Error; err != nil {
		t.Fatal(err)
	}
	denied = requestJSON(t, router, "GET", "/api/auth/me", token, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("suspended user retained access: %d", denied.Code)
	}
}

func TestCustomOIDCProviderCRUDAndNoSecretLeak(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	create := requestJSON(t, router, "POST", "/api/oidc/providers", token, map[string]any{
		"name": "Custom Identity", "provider_type": "custom", "issuer_url": "https://id.example.org",
		"client_id": "client-a", "client_secret": "top-secret-value",
		"authorization_url": "https://id.example.org/authorize",
		"token_url":         "https://id.example.org/token",
		"jwks_url":          "https://id.example.org/jwks",
		"userinfo_url":      "https://id.example.org/userinfo",
		"claim_mapping":     `{"email":"profile.email","name":"profile.display"}`,
	})
	if create.Code != 201 {
		t.Fatalf("custom provider create failed: %d %s", create.Code, create.Body.String())
	}
	if strings.Contains(create.Body.String(), "top-secret-value") {
		t.Fatal("OIDC client secret leaked in API response")
	}
	id := responseID(t, create, "provider")
	providers := requestJSON(t, router, "GET", "/api/auth/oidc/providers", "", nil)
	if providers.Code != 200 || !strings.Contains(providers.Body.String(), id) {
		t.Fatal("active provider omitted from public discovery")
	}
	if strings.Contains(providers.Body.String(), "client-a") {
		t.Fatal("public provider list leaked client configuration")
	}

	update := requestJSON(t, router, "PUT", "/api/oidc/providers/"+id, token, map[string]any{
		"is_active": false, "claim_mapping": `{"email":"profile.mail"}`,
	})
	if update.Code != 200 {
		t.Fatalf("cannot edit custom claim mapping: %s", update.Body.String())
	}
	if strings.Contains(update.Body.String(), "top-secret-value") {
		t.Fatal("OIDC secret leaked after editing")
	}
	providers = requestJSON(t, router, "GET", "/api/auth/oidc/providers", "", nil)
	if strings.Contains(providers.Body.String(), id) {
		t.Fatal("disabled provider was publicly offered for login")
	}

	invalid := requestJSON(t, router, "PUT", "/api/oidc/providers/"+id, token, map[string]any{
		"claim_mapping": `{"is_admin":"roles"}`,
	})
	if invalid.Code != 400 {
		t.Fatalf("unsafe claim mapping accepted: %d", invalid.Code)
	}
	providerID := id
	pending := db.User{ID: "linked-idp-user", Email: "linked@example.org", Name: "Linked",
		AuthType: "oidc", IDPProviderID: &providerID, IDPSubject: "subject-a", Status: "PENDING"}
	if err := db.DB.Create(&pending).Error; err != nil {
		t.Fatal(err)
	}
	changedIssuer := requestJSON(t, router, "PUT", "/api/oidc/providers/"+id, token, map[string]string{
		"issuer_url": "https://attacker.example.org",
	})
	if changedIssuer.Code != 409 {
		t.Fatalf("provider identity issuer was changed with linked users: %d", changedIssuer.Code)
	}
	deleted := requestJSON(t, router, "DELETE", "/api/oidc/providers/"+id, token, nil)
	if deleted.Code != 409 {
		t.Fatalf("provider deleted with linked users: %d", deleted.Code)
	}
}
