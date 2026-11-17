package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/handlers"
	"github.com/swan-panel/swan-panel/internal/db"
)

func TestPasswordChangeRejectsUnauthenticatedAndMissingIdentity(t *testing.T) {
	_, cleanup := setupTestRouter(t)
	defer cleanup()
	for _, tc := range []struct {
		name     string
		userID   string
		expected int
	}{
		{"unauthenticated context", "", http.StatusUnauthorized},
		{"deleted identity", "no-such-user", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			payload, _ := json.Marshal(map[string]string{
				"current_password": "original-secret", "new_password": "replacement-password",
			})
			c.Request = httptest.NewRequest("PUT", "/api/auth/password", bytes.NewReader(payload))
			c.Request.Header.Set("Content-Type", "application/json")
			if tc.userID != "" {
				c.Set("user_id", tc.userID)
			}
			handlers.NewAuthHandler(nil).ChangePassword(c)
			if recorder.Code != tc.expected {
				t.Fatalf("expected HTTP %d, got %d: %s", tc.expected, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestPasswordChangePreservesCredentialsIfDatabaseRejectsUpdate(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	if err := db.DB.Exec("CREATE TRIGGER deny_password_update BEFORE UPDATE OF password_hash ON users BEGIN SELECT RAISE(ABORT, 'password update denied'); END;").Error; err != nil {
		t.Fatal(err)
	}
	result := requestJSON(t, router, "PUT", "/api/auth/password", token, map[string]any{
		"current_password": "admin123456789!", "new_password": "replacement-password-2026",
	})
	if result.Code != http.StatusInternalServerError {
		t.Fatalf("database failure not reported: %d %s", result.Code, result.Body.String())
	}
	oldLogin := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]string{
		"email": "admin@swanpanel.internal", "password": "admin123456789!",
	})
	if oldLogin.Code != http.StatusOK {
		t.Fatalf("failed change corrupted existing password: %d", oldLogin.Code)
	}
}
