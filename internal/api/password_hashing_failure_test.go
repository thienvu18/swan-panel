package api_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/handlers"
	"github.com/swan-panel/swan-panel/internal/db"
)

type unavailablePasswordEntropy struct{}

func (unavailablePasswordEntropy) Read([]byte) (int, error) {
	return 0, errors.New("simulated entropy unavailable")
}

func TestPasswordChangeRejectsMalformedPayloadAndCryptographicEntropyFailure(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	_ = getAdminToken(t, router)
	var user db.User
	if err := db.DB.First(&user, "email = ?", "admin@swanpanel.internal").Error; err != nil {
		t.Fatal(err)
	}
	invoke := func(payload any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest("PUT", "/api/auth/password", bytes.NewReader(body))
		context.Request.Header.Set("Content-Type", "application/json")
		context.Set("user_id", user.ID)
		handlers.NewAuthHandler(nil).ChangePassword(context)
		return recorder
	}
	if result := invoke(map[string]any{"new_password": "too-few-arguments"}); result.Code != http.StatusBadRequest {
		t.Fatalf("incomplete password request accepted: %d %s", result.Code, result.Body.String())
	}
	original := rand.Reader
	rand.Reader = unavailablePasswordEntropy{}
	defer func() { rand.Reader = original }()
	result := invoke(map[string]any{
		"current_password": "admin123456789!",
		"new_password":     "new-secure-password-2026",
	})
	if result.Code != http.StatusInternalServerError {
		t.Fatalf("failed CSPRNG ignored: %d %s", result.Code, result.Body.String())
	}
}
