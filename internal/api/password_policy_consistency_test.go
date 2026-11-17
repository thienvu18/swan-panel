package api_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestPasswordChangeUsesSameLengthPolicyAsLocalUserCreation(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	for _, value := range []string{
		"12345678", strings.Repeat("x", 11),
		strings.Repeat("x", 73), strings.Repeat("界", 25),
	} {
		result := requestJSON(t, router, http.MethodPut, "/api/auth/password", token, map[string]any{
			"current_password": "admin123456789!", "new_password": value,
		})
		if result.Code != http.StatusBadRequest {
			t.Errorf("invalid password length %d bytes accepted: %d", len(value), result.Code)
		}
	}
	oldLogin := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]string{
		"email": "admin@swanpanel.internal", "password": "admin123456789!",
	})
	if oldLogin.Code != http.StatusOK {
		t.Fatalf("rejected changes unexpectedly updated credentials: %d", oldLogin.Code)
	}
	updated := strings.Repeat("A", 72)
	changed := requestJSON(t, router, "PUT", "/api/auth/password", token, map[string]any{
		"current_password": "admin123456789!", "new_password": updated,
	})
	if changed.Code != http.StatusOK {
		t.Fatalf("valid 72-byte password rejected: %d %s", changed.Code, changed.Body.String())
	}
	login := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]string{
		"email": "admin@swanpanel.internal", "password": updated,
	})
	if login.Code != http.StatusOK {
		t.Fatalf("updated credentials cannot sign in: %d", login.Code)
	}
}
