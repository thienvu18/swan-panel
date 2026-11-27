package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/api/middleware"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/mfa"
)

func TestLocalAccountMFALifecycleAndChallengeEnforcement(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)

	// 1. Create a local operator account with a known password.
	password := "OperatorMFA-StrongPass!2026"
	created := requestJSON(t, router, "POST", "/api/users", admin, map[string]any{
		"name":     "MFA Operator",
		"email":    "mfa-op@example.com",
		"password": password,
		"is_admin": false,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create user failed: %s", created.Body.String())
	}

	// Initial login succeeds directly before MFA is enabled.
	firstLogin := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email":    "mfa-op@example.com",
		"password": password,
	})
	if firstLogin.Code != http.StatusOK {
		t.Fatalf("initial login failed: %s", firstLogin.Body.String())
	}
	var loginPayload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(firstLogin.Body.Bytes(), &loginPayload); err != nil || loginPayload.Token == "" {
		t.Fatalf("missing initial JWT: %s", firstLogin.Body.String())
	}
	userToken := loginPayload.Token

	// 2. Check initial status.
	statusBody := mustStatus(t, router, "GET", "/api/auth/mfa/status", userToken, nil, http.StatusOK)
	if !strings.Contains(statusBody, `"mfa_enabled":false`) || !strings.Contains(statusBody, `"supported":true`) {
		t.Fatalf("unexpected initial MFA status: %s", statusBody)
	}

	// 3. Setup requires current password.
	mustStatus(t, router, "POST", "/api/auth/mfa/setup", userToken, map[string]any{
		"current_password": "wrong-password",
	}, http.StatusUnauthorized)

	setupResp := requestJSON(t, router, "POST", "/api/auth/mfa/setup", userToken, map[string]any{
		"current_password": password,
	})
	if setupResp.Code != http.StatusOK {
		t.Fatalf("setup MFA failed: %s", setupResp.Body.String())
	}
	var setupData struct {
		Secret     string `json:"secret"`
		OTPAuthURI string `json:"otpauth_uri"`
	}
	if err := json.Unmarshal(setupResp.Body.Bytes(), &setupData); err != nil || setupData.Secret == "" {
		t.Fatalf("invalid setup payload: %s", setupResp.Body.String())
	}

	// 4. Enable MFA with valid TOTP code (use -30s step so a fresh current-step code can be tested right after!).
	mustStatus(t, router, "POST", "/api/auth/mfa/enable", userToken, map[string]any{
		"code": "000000",
	}, http.StatusUnauthorized)

	enrollCode, err := mfa.GenerateCode(setupData.Secret, time.Now().UTC().Add(-30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	enableResp := requestJSON(t, router, "POST", "/api/auth/mfa/enable", userToken, map[string]any{
		"code": enrollCode,
	})
	if enableResp.Code != http.StatusOK {
		t.Fatalf("enable MFA failed: %s", enableResp.Body.String())
	}
	var enableData struct {
		MFAEnabled  bool     `json:"mfa_enabled"`
		BackupCodes []string `json:"backup_codes"`
	}
	if err := json.Unmarshal(enableResp.Body.Bytes(), &enableData); err != nil || !enableData.MFAEnabled || len(enableData.BackupCodes) != 8 {
		t.Fatalf("unexpected enable MFA response: %s", enableResp.Body.String())
	}

	// 5. Next login must return mfa_required=true and NO JWT until verified.
	step1 := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email":    "mfa-op@example.com",
		"password": password,
	})
	if step1.Code != http.StatusOK {
		t.Fatalf("MFA login step 1 failed: %s", step1.Body.String())
	}
	if strings.Contains(step1.Body.String(), `"token"`) {
		t.Fatalf("MFA login step 1 leaked JWT before TOTP verification: %s", step1.Body.String())
	}
	var challengeData struct {
		MFARequired bool   `json:"mfa_required"`
		MFAToken    string `json:"mfa_token"`
	}
	if err := json.Unmarshal(step1.Body.Bytes(), &challengeData); err != nil || !challengeData.MFARequired || challengeData.MFAToken == "" {
		t.Fatalf("expected mfa_required challenge, got %s", step1.Body.String())
	}

	// Wrong code rejected.
	mustStatus(t, router, "POST", "/api/auth/mfa/verify", "", map[string]any{
		"mfa_token": challengeData.MFAToken,
		"code":      "999999",
	}, http.StatusUnauthorized)

	// Valid current-step TOTP code issues JWT.
	loginCode, err := mfa.GenerateCode(setupData.Secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	step2 := requestJSON(t, router, "POST", "/api/auth/mfa/verify", "", map[string]any{
		"mfa_token": challengeData.MFAToken,
		"code":      loginCode,
	})
	if step2.Code != http.StatusOK || !strings.Contains(step2.Body.String(), `"token"`) {
		t.Fatalf("expected MFA verify to issue JWT, got %d %s", step2.Code, step2.Body.String())
	}
	// Challenge token is single-use.
	mustStatus(t, router, "POST", "/api/auth/mfa/verify", "", map[string]any{
		"mfa_token": challengeData.MFAToken,
		"code":      loginCode,
	}, http.StatusUnauthorized)

	// 6. Verify backup code login and single-use consumption.
	step1Backup := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email":    "mfa-op@example.com",
		"password": password,
	})
	var challenge2 struct {
		MFAToken string `json:"mfa_token"`
	}
	_ = json.Unmarshal(step1Backup.Body.Bytes(), &challenge2)
	backupVerify := requestJSON(t, router, "POST", "/api/auth/mfa/verify", "", map[string]any{
		"mfa_token": challenge2.MFAToken,
		"code":      enableData.BackupCodes[0],
	})
	if backupVerify.Code != http.StatusOK {
		t.Fatalf("backup code verification failed: %s", backupVerify.Body.String())
	}

	// Remaining backup codes should now be 7.
	statusAfterBackup := mustStatus(t, router, "GET", "/api/auth/mfa/status", userToken, nil, http.StatusOK)
	if !strings.Contains(statusAfterBackup, `"backup_codes_remaining":7`) {
		t.Fatalf("expected 7 remaining backup codes, got %s", statusAfterBackup)
	}

	// Reusing consumed backup code must fail.
	step1Replay := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email":    "mfa-op@example.com",
		"password": password,
	})
	var challenge3 struct {
		MFAToken string `json:"mfa_token"`
	}
	_ = json.Unmarshal(step1Replay.Body.Bytes(), &challenge3)
	mustStatus(t, router, "POST", "/api/auth/mfa/verify", "", map[string]any{
		"mfa_token": challenge3.MFAToken,
		"code":      enableData.BackupCodes[0],
	}, http.StatusUnauthorized)

	// 7. Regenerate backup codes (using +30s step since current step was consumed) and disable MFA.
	regenCode, err := mfa.GenerateCode(setupData.Secret, time.Now().UTC().Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	regenResp := requestJSON(t, router, "POST", "/api/auth/mfa/backup-codes", userToken, map[string]any{
		"current_password": password,
		"code":             regenCode,
	})
	if regenResp.Code != http.StatusOK {
		t.Fatalf("regenerate backup codes failed: %s", regenResp.Body.String())
	}
	var regenData struct {
		BackupCodes []string `json:"backup_codes"`
	}
	_ = json.Unmarshal(regenResp.Body.Bytes(), &regenData)
	if len(regenData.BackupCodes) != 8 {
		t.Fatalf("expected 8 regenerated backup codes, got %d", len(regenData.BackupCodes))
	}

	mustStatus(t, router, "POST", "/api/auth/mfa/disable", userToken, map[string]any{
		"current_password": password,
		"code":             regenData.BackupCodes[0],
	}, http.StatusOK)
}

func TestMFAChallengeLocksOutAfterMaxAttemptsAndAdminReset(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)

	password := "LockedOutMFA-Pass!2026"
	created := requestJSON(t, router, "POST", "/api/users", admin, map[string]any{
		"name":     "Locked User",
		"email":    "locked-mfa@example.com",
		"password": password,
		"is_admin": false,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create user failed: %s", created.Body.String())
	}
	userID := responseID(t, created, "user")

	var user db.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		t.Fatal(err)
	}
	userToken, err := middleware.GenerateToken(&user, "test-secret-key-12345", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	setupResp := requestJSON(t, router, "POST", "/api/auth/mfa/setup", userToken, map[string]any{
		"current_password": password,
	})
	var setupData struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(setupResp.Body.Bytes(), &setupData)
	enrollCode, _ := mfa.GenerateCode(setupData.Secret, time.Now().UTC().Add(-30*time.Second))
	mustStatus(t, router, "POST", "/api/auth/mfa/enable", userToken, map[string]any{"code": enrollCode}, http.StatusOK)

	step1 := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email":    "locked-mfa@example.com",
		"password": password,
	})
	var challenge struct {
		MFAToken string `json:"mfa_token"`
	}
	_ = json.Unmarshal(step1.Body.Bytes(), &challenge)

	// 5 bad attempts destroy the challenge token, so even a valid code on attempt 6 fails.
	for i := 0; i < 5; i++ {
		mustStatus(t, router, "POST", "/api/auth/mfa/verify", "", map[string]any{
			"mfa_token": challenge.MFAToken,
			"code":      "000000",
		}, http.StatusUnauthorized)
	}
	validCode, _ := mfa.GenerateCode(setupData.Secret, time.Now().UTC())
	mustStatus(t, router, "POST", "/api/auth/mfa/verify", "", map[string]any{
		"mfa_token": challenge.MFAToken,
		"code":      validCode,
	}, http.StatusUnauthorized)

	// Non-admin cannot reset MFA; admin can reset MFA.
	mustStatus(t, router, "POST", "/api/users/"+userID+"/mfa/reset", userToken, nil, http.StatusForbidden)
	mustStatus(t, router, "POST", "/api/users/"+userID+"/mfa/reset", admin, nil, http.StatusOK)

	// After admin reset, password login works directly again.
	afterReset := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email":    "locked-mfa@example.com",
		"password": password,
	})
	if afterReset.Code != http.StatusOK || !strings.Contains(afterReset.Body.String(), `"token"`) {
		t.Fatalf("expected direct login after admin MFA reset, got %s", afterReset.Body.String())
	}
}

func TestMFAValidationBranchesAndDisabledFlows(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)

	// Unauthenticated / bad payload / invalid MFA challenge token cases.
	mustStatus(t, router, "GET", "/api/auth/mfa/status", "", nil, http.StatusUnauthorized)
	statusBody := mustStatus(t, router, "GET", "/api/auth/mfa/status", admin, nil, http.StatusOK)
	if !strings.Contains(statusBody, `"mfa_enabled":false`) {
		t.Fatalf("unexpected mfa status body: %s", statusBody)
	}

	// Missing password or wrong password on setup, enable without setup, wrong password on disable/regen, missing user reset.
	mustStatus(t, router, "POST", "/api/auth/mfa/setup", admin, map[string]any{"current_password": "wrong"}, http.StatusUnauthorized)
	mustStatus(t, router, "POST", "/api/auth/mfa/enable", admin, map[string]any{"code": "123456"}, http.StatusBadRequest)
	mustStatus(t, router, "POST", "/api/auth/mfa/disable", admin, map[string]any{"current_password": "wrong", "code": "123456"}, http.StatusUnauthorized)
	mustStatus(t, router, "POST", "/api/auth/mfa/backup-codes", admin, map[string]any{"current_password": "wrong", "code": "123456"}, http.StatusUnauthorized)
	mustStatus(t, router, "POST", "/api/users/usr_missing/mfa/reset", admin, nil, http.StatusNotFound)
	mustStatus(t, router, "POST", "/api/auth/mfa/verify", "", map[string]any{"mfa_token": "bogus", "code": "123456"}, http.StatusUnauthorized)

	// Create local user with known password and exercise wrong-code branches.
	password := "MFAErrorPathPass!2026"
	created := requestJSON(t, router, "POST", "/api/users", admin, map[string]any{
		"name":     "MFA Edge User",
		"email":    "mfa-edge@example.com",
		"password": password,
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create user failed: %s", created.Body.String())
	}
	userID := responseID(t, created, "user")
	loginResp := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email":    "mfa-edge@example.com",
		"password": password,
	})
	var loginData struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(loginResp.Body.Bytes(), &loginData)

	setupResp := requestJSON(t, router, "POST", "/api/auth/mfa/setup", loginData.Token, map[string]any{"current_password": password})
	var setupData struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(setupResp.Body.Bytes(), &setupData)
	// Wrong TOTP on enable rejected (401); valid code succeeds.
	mustStatus(t, router, "POST", "/api/auth/mfa/enable", loginData.Token, map[string]any{"code": "000000"}, http.StatusUnauthorized)
	code, _ := mfa.GenerateCode(setupData.Secret, time.Now().UTC())
	mustStatus(t, router, "POST", "/api/auth/mfa/enable", loginData.Token, map[string]any{"code": code}, http.StatusOK)

	// Wrong password / wrong TOTP on regen and disable fail cleanly.
	mustStatus(t, router, "POST", "/api/auth/mfa/backup-codes", loginData.Token, map[string]any{"current_password": "wrong", "code": "123456"}, http.StatusUnauthorized)
	mustStatus(t, router, "POST", "/api/auth/mfa/backup-codes", loginData.Token, map[string]any{"current_password": password, "code": "000000"}, http.StatusUnauthorized)
	mustStatus(t, router, "POST", "/api/auth/mfa/disable", loginData.Token, map[string]any{"current_password": "wrong", "code": "123456"}, http.StatusUnauthorized)
	mustStatus(t, router, "POST", "/api/auth/mfa/disable", loginData.Token, map[string]any{"current_password": password, "code": "000000"}, http.StatusUnauthorized)

	// Reset for clean admin audit logging check on userID, then verify disable/backup-codes return 400 when MFA is disabled.
	mustStatus(t, router, "POST", "/api/users/"+userID+"/mfa/reset", admin, nil, http.StatusOK)
	mustStatus(t, router, "POST", "/api/auth/mfa/disable", loginData.Token, map[string]any{"current_password": password, "code": "123456"}, http.StatusBadRequest)
	mustStatus(t, router, "POST", "/api/auth/mfa/backup-codes", loginData.Token, map[string]any{"current_password": password, "code": "123456"}, http.StatusBadRequest)
}
