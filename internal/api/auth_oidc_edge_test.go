package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/api/middleware"
	"github.com/swan-panel/swan-panel/internal/db"
	"time"
)

func TestPasswordChangesRejectWrongCurrentPasswordAndInvalidateOldSecret(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	endpoint := "/api/auth/password"
	mustStatus(t, router, "PUT", endpoint, token, map[string]any{
		"current_password": "wrong", "new_password": "new-password-123",
	}, http.StatusUnauthorized)
	mustStatus(t, router, "PUT", endpoint, token, map[string]any{
		"current_password": "admin123456789!", "new_password": "short",
	}, http.StatusBadRequest)
	mustStatus(t, router, "PUT", endpoint, token, map[string]any{
		"current_password": "admin123456789!", "new_password": "really-long-new-password-2026",
	}, http.StatusOK)
	mustStatus(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email": "admin@swanpanel.internal", "password": "admin123456789!",
	}, http.StatusUnauthorized)
	login := mustStatus(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email": "admin@swanpanel.internal", "password": "really-long-new-password-2026",
	}, http.StatusOK)
	if !strings.Contains(login, "token") {
		t.Fatal("updated password did not produce a session token")
	}
	// A new SSO account has no local password and must use its identity provider.
	oidc := db.User{ID: "oidc-locked", Name: "Federated", Email: "federated@example.invalid", AuthType: "oidc", Status: "ACTIVE"}
	if err := db.DB.Create(&oidc).Error; err != nil {
		t.Fatal(err)
	}
	jwt, err := middleware.GenerateToken(&oidc, "test-secret-key-12345", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "PUT", endpoint, jwt, map[string]any{
		"current_password": "fake", "new_password": "new-password-2026",
	}, http.StatusBadRequest)
	mustStatus(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email": "federated@example.invalid", "password": "password-2026",
	}, http.StatusUnauthorized)
}

func TestOIDCProviderRejectsUnsafeMetadataAndInvalidSSOExchange(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	mustStatus(t, router, "GET", "/api/oidc/providers", admin, nil, 200)
	mustStatus(t, router, "POST", "/api/oidc/providers", admin, map[string]any{"name": "missing"}, 400)
	for _, tc := range []struct {
		name                 string
		providerType, issuer string
	}{
		{"unknown provider type", "made-up", "https://login.example.org"},
		{"plaintext issuer", "custom", "http://login.example.org"},
		{"loopback SSRF", "custom", "https://127.0.0.1:443"},
		{"no URL", "custom", "not-a-url"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "loopback SSRF" {
				t.Setenv("APP_ENV", "production")
			}
			response := requestJSON(t, router, "POST", "/api/oidc/providers", admin, map[string]any{
				"name": "Bad Provider", "provider_type": tc.providerType,
				"issuer_url": tc.issuer, "client_id": "client", "client_secret": "secret",
				"authorization_url": "https://id.example.org/auth",
				"token_url":         "https://id.example.org/token", "jwks_url": "https://id.example.org/jwks",
			})
			if response.Code != 400 {
				t.Fatalf("unsafe OIDC provider accepted: %d %s", response.Code, response.Body.String())
			}
		})
	}
	mustStatus(t, router, "PUT", "/api/oidc/providers/missing", admin, map[string]any{"name": "ignored"}, 404)
	mustStatus(t, router, "POST", "/api/oidc/providers/missing/test", admin, map[string]any{}, 404)
	mustStatus(t, router, "GET", "/api/auth/oidc/missing/start", "", nil, 404)
	mustStatus(t, router, "GET", "/api/auth/oidc/callback?error=access_denied", "", nil, 400)
	mustStatus(t, router, "POST", "/api/auth/oidc/exchange", "", nil, 401)
	invalidOrigin := requestJSON(t, router, "POST", "/api/auth/oidc/exchange", "", nil)
	if invalidOrigin.Code != 401 {
		t.Fatalf("exchange without pending session accepted: %d", invalidOrigin.Code)
	}
}

func TestOIDCProviderMetadataCanBeUpdatedButLinkedIssuerCannotBeReplaced(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	created := requestJSON(t, router, "POST", "/api/oidc/providers", admin, map[string]any{
		"name": "External ID", "provider_type": "custom", "issuer_url": "https://id.example.org",
		"client_id": "client-id", "client_secret": "secret",
		"authorization_url": "https://id.example.org/auth",
		"token_url":         "https://id.example.org/token",
		"jwks_url":          "https://id.example.org/jwks",
	})
	if created.Code != 201 {
		t.Fatalf("provider creation failed: %s", created.Body.String())
	}
	id := responseID(t, created, "provider")
	mustStatus(t, router, "POST", "/api/oidc/providers/"+id+"/test", admin, map[string]any{}, 200)
	mustStatus(t, router, "PUT", "/api/oidc/providers/"+id, admin, map[string]any{
		"name": "New Provider Name", "client_secret": "new-secret",
	}, 200)
	idp := id
	linked := db.User{ID: "idp-linked", Name: "Linked Identity", Email: "idp-linked@example.invalid",
		AuthType: "oidc", Status: "ACTIVE", IDPProviderID: &idp, IDPSubject: "sub-1"}
	if err := db.DB.Create(&linked).Error; err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "PUT", "/api/oidc/providers/"+id, admin, map[string]any{
		"client_id": "new-client-id",
	}, 409)
	mustStatus(t, router, "DELETE", "/api/oidc/providers/"+id, admin, nil, 409)
	mustStatus(t, router, "PUT", "/api/oidc/providers/"+id, admin, map[string]any{
		"is_active": false,
	}, 200)
	public := mustStatus(t, router, "GET", "/api/auth/oidc/providers", "", nil, 200)
	if strings.Contains(public, id) {
		t.Fatal("inactive provider publicly advertised")
	}
}
