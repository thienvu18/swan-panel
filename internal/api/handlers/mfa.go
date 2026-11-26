package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/middleware"
	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/mfa"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	mfaChallengeTTL         = 5 * time.Minute
	mfaMaxChallengeAttempts = 5
)

func (h *AuthHandler) mfaRootSecret() string {
	if override := strings.TrimSpace(os.Getenv("MFA_ENCRYPTION_KEY")); override != "" {
		return override
	}
	return h.cfg.JWTSecret
}

func hashMFAToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (h *AuthHandler) issueMFAChallenge(c *gin.Context, user *db.User) {
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		slog.Error("Failed to generate MFA challenge token", "user_id", user.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize two-factor challenge"})
		return
	}
	token := base64.RawURLEncoding.EncodeToString(rawBytes)
	now := time.Now().UTC()
	challenge := db.MFAChallenge{
		ChallengeHash: hashMFAToken(token),
		UserID:        user.ID,
		Attempts:      0,
		ExpiresAt:     now.Add(mfaChallengeTTL),
	}

	err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("expires_at < ?", now).Delete(&db.MFAChallenge{}).Error; err != nil {
			return err
		}
		return tx.Create(&challenge).Error
	})
	if err != nil {
		slog.Error("Failed to persist MFA challenge", "user_id", user.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize two-factor challenge"})
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"mfa_required": true,
		"mfa_token":    token,
		"user": gin.H{
			"email": user.Email,
		},
	})
}

// VerifyMFALogin completes a two-step local login by verifying a TOTP or single-use recovery code.
func (h *AuthHandler) VerifyMFALogin(c *gin.Context) {
	var req struct {
		MFAToken string `json:"mfa_token" binding:"required"`
		Code     string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Two-factor token and verification code are required"})
		return
	}

	now := time.Now().UTC()
	rootSecret := h.mfaRootSecret()
	challengeHash := hashMFAToken(strings.TrimSpace(req.MFAToken))

	var (
		user        db.User
		verified    bool
		methodUsed  string
		failureCode = http.StatusUnauthorized
		failureMsg  = "Invalid verification code"
	)

	err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var challenge db.MFAChallenge
		if err := tx.First(&challenge, "challenge_hash = ?", challengeHash).Error; err != nil {
			failureMsg = "Two-factor session expired or invalid"
			return nil
		}
		if now.After(challenge.ExpiresAt) {
			_ = tx.Delete(&challenge).Error
			failureMsg = "Two-factor session expired; sign in again"
			return nil
		}
		if err := tx.First(&user, "id = ?", challenge.UserID).Error; err != nil {
			_ = tx.Delete(&challenge).Error
			failureMsg = "Account unavailable"
			return nil
		}
		if user.Status != "ACTIVE" || user.AuthType != "local" || !user.MFAEnabled || user.TOTPSecretEnc == "" {
			_ = tx.Delete(&challenge).Error
			failureCode = http.StatusForbidden
			failureMsg = "Two-factor authentication is unavailable for this account"
			return nil
		}

		secret, decErr := mfa.DecryptSecret(user.TOTPSecretEnc, rootSecret)
		if decErr != nil {
			slog.Error("Failed to decrypt TOTP secret", "user_id", user.ID, "error", decErr)
			failureCode = http.StatusInternalServerError
			failureMsg = "Unable to verify two-factor credentials"
			return nil
		}

		if step, ok := mfa.VerifyCode(secret, req.Code, now, user.MFALastUsedStep); ok {
			verified = true
			methodUsed = "totp"
			if err := tx.Model(&user).Update("mfa_last_used_step", step).Error; err != nil {
				return err
			}
			return tx.Delete(&challenge).Error
		}

		if updatedCodes, ok := mfa.ConsumeBackupCode(user.MFABackupCodes, req.Code, rootSecret); ok {
			verified = true
			methodUsed = "backup_code"
			if err := tx.Model(&user).Update("mfa_backup_codes", updatedCodes).Error; err != nil {
				return err
			}
			return tx.Delete(&challenge).Error
		}

		challenge.Attempts++
		if challenge.Attempts >= mfaMaxChallengeAttempts {
			failureMsg = "Too many invalid verification attempts; sign in again"
			return tx.Delete(&challenge).Error
		}
		return tx.Model(&challenge).Update("attempts", challenge.Attempts).Error
	})
	if err != nil {
		slog.Error("MFA verification transaction failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify two-factor code"})
		return
	}
	if !verified {
		if user.Email != "" {
			audit.TrackFailedLogin(c, user.Email, "invalid_mfa_code")
		}
		c.JSON(failureCode, gin.H{"error": failureMsg})
		return
	}

	token, err := middleware.GenerateToken(&user, h.cfg.JWTSecret, 24*time.Hour)
	if err != nil {
		slog.Error("MFA login failed to sign session JWT", "user_id", user.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session token"})
		return
	}

	role := "user"
	if user.IsAdmin {
		role = "admin"
	}
	audit.RecordFromContext(c, audit.EntryInput{
		ActorID:    user.ID,
		ActorEmail: user.Email,
		ActorRole:  role,
		Action:     "auth.login.success",
		Severity:   audit.SeverityInfo,
		TargetType: "user",
		TargetID:   user.ID,
		Summary:    "User signed in with two-factor authentication: " + user.Email,
		Details:    map[string]any{"mfa": true, "method": methodUsed},
	})

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":          user.ID,
			"email":       user.Email,
			"name":        user.Name,
			"is_admin":    user.IsAdmin,
			"status":      user.Status,
			"mfa_enabled": user.MFAEnabled,
		},
	})
}

func loadVerifiedLocalUser(c *gin.Context, password string) (*db.User, int, string) {
	userID := c.GetString("user_id")
	var user db.User
	if err := db.DB.WithContext(c.Request.Context()).Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, http.StatusNotFound, "User not found"
	}
	if user.AuthType != "local" || user.PasswordHash == "" {
		return nil, http.StatusBadRequest, "Two-factor authentication is only managed locally for local accounts"
	}
	if password != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
			return nil, http.StatusUnauthorized, "Current password is incorrect"
		}
	}
	return &user, 0, ""
}

// GetMFAStatus returns the authenticated user's TOTP enrollment state.
func (h *AuthHandler) GetMFAStatus(c *gin.Context) {
	var user db.User
	if err := db.DB.WithContext(c.Request.Context()).Where("id = ?", c.GetString("user_id")).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"supported":              user.AuthType == "local",
		"mfa_enabled":            user.MFAEnabled,
		"backup_codes_remaining": mfa.CountBackupCodes(user.MFABackupCodes),
	})
}

// SetupMFA generates a pending encrypted TOTP secret for the authenticated local account.
func (h *AuthHandler) SetupMFA(c *gin.Context) {
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Current password is required"})
		return
	}
	user, status, errMsg := loadVerifiedLocalUser(c, req.CurrentPassword)
	if user == nil {
		c.JSON(status, gin.H{"error": errMsg})
		return
	}
	if user.MFAEnabled {
		c.JSON(http.StatusConflict, gin.H{"error": "Two-factor authentication is already enabled"})
		return
	}

	secret, err := mfa.GenerateSecret()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate two-factor secret"})
		return
	}
	encrypted, err := mfa.EncryptSecret(secret, h.mfaRootSecret())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to protect two-factor secret"})
		return
	}
	if err := db.DB.WithContext(c.Request.Context()).Model(user).Update("pending_totp_enc", encrypted).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store pending two-factor setup"})
		return
	}

	issuer := "SwanPanel"
	var sw db.StrongswanSettings
	if err := db.DB.WithContext(c.Request.Context()).First(&sw, 1).Error; err == nil && strings.TrimSpace(sw.VPNName) != "" {
		issuer = strings.TrimSpace(sw.VPNName)
	}

	c.Header("Cache-Control", "no-store, private")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{
		"secret":      secret,
		"otpauth_uri": mfa.BuildOTPAuthURI(issuer, user.Email, secret),
		"issuer":      issuer,
		"account":     user.Email,
	})
}

// EnableMFA confirms a pending TOTP setup and returns single-use recovery codes once.
func (h *AuthHandler) EnableMFA(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Verification code is required"})
		return
	}
	user, status, errMsg := loadVerifiedLocalUser(c, "")
	if user == nil {
		c.JSON(status, gin.H{"error": errMsg})
		return
	}
	if user.MFAEnabled {
		c.JSON(http.StatusConflict, gin.H{"error": "Two-factor authentication is already enabled"})
		return
	}
	if user.PendingTOTPEnc == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Start two-factor setup first"})
		return
	}

	rootSecret := h.mfaRootSecret()
	var plaintextCodes []string
	var errCode int
	var errText string
	err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var current db.User
		if err := tx.Where("id = ?", user.ID).First(&current).Error; err != nil {
			errCode, errText = http.StatusNotFound, "User not found"
			return nil
		}
		if current.MFAEnabled {
			errCode, errText = http.StatusConflict, "Two-factor authentication is already enabled"
			return nil
		}
		if current.PendingTOTPEnc == "" {
			errCode, errText = http.StatusBadRequest, "Start two-factor setup first"
			return nil
		}
		secret, decErr := mfa.DecryptSecret(current.PendingTOTPEnc, rootSecret)
		if decErr != nil {
			errCode, errText = http.StatusInternalServerError, "Pending two-factor secret unavailable"
			return nil
		}
		step, ok := mfa.VerifyCode(secret, req.Code, time.Now().UTC(), 0)
		if !ok {
			errCode, errText = http.StatusUnauthorized, "Invalid verification code"
			return nil
		}
		codes, digestsJSON, genErr := mfa.GenerateBackupCodes(rootSecret)
		if genErr != nil {
			errCode, errText = http.StatusInternalServerError, "Failed to generate recovery codes"
			return nil
		}
		res := tx.Model(&db.User{}).
			Where("id = ? AND mfa_enabled = ?", current.ID, false).
			Updates(map[string]any{
				"mfa_enabled":        true,
				"totp_secret_enc":    current.PendingTOTPEnc,
				"pending_totp_enc":   "",
				"mfa_backup_codes":   digestsJSON,
				"mfa_last_used_step": step,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			errCode, errText = http.StatusConflict, "Two-factor authentication is already enabled"
			return nil
		}
		plaintextCodes = codes
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to activate two-factor authentication"})
		return
	}
	if errCode != 0 {
		c.JSON(errCode, gin.H{"error": errText})
		return
	}

	audit.RecordFromContext(c, audit.EntryInput{
		Action:     "auth.mfa.enabled",
		Severity:   audit.SeverityInfo,
		TargetType: "user",
		TargetID:   user.ID,
		Summary:    "Two-factor authentication enabled for " + user.Email,
	})

	c.Header("Cache-Control", "no-store, private")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{
		"mfa_enabled":  true,
		"backup_codes": plaintextCodes,
	})
}

// DisableMFA disables TOTP after verifying current password and an active TOTP or recovery code.
func (h *AuthHandler) DisableMFA(c *gin.Context) {
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		Code            string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Current password and verification code are required"})
		return
	}
	user, status, errMsg := loadVerifiedLocalUser(c, req.CurrentPassword)
	if user == nil {
		c.JSON(status, gin.H{"error": errMsg})
		return
	}
	if !user.MFAEnabled || user.TOTPSecretEnc == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Two-factor authentication is not enabled"})
		return
	}

	rootSecret := h.mfaRootSecret()
	secret, err := mfa.DecryptSecret(user.TOTPSecretEnc, rootSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to read two-factor configuration"})
		return
	}
	_, totpOK := mfa.VerifyCode(secret, req.Code, time.Now().UTC(), user.MFALastUsedStep)
	_, backupOK := mfa.ConsumeBackupCode(user.MFABackupCodes, req.Code, rootSecret)
	if !totpOK && !backupOK {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid verification code"})
		return
	}

	err = db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(user).Updates(map[string]any{
			"mfa_enabled":        false,
			"totp_secret_enc":    "",
			"pending_totp_enc":   "",
			"mfa_backup_codes":   "",
			"mfa_last_used_step": 0,
		}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", user.ID).Delete(&db.MFAChallenge{}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disable two-factor authentication"})
		return
	}

	audit.RecordFromContext(c, audit.EntryInput{
		Action:     "auth.mfa.disabled",
		Severity:   audit.SeverityWarning,
		TargetType: "user",
		TargetID:   user.ID,
		Summary:    "Two-factor authentication disabled for " + user.Email,
	})
	c.JSON(http.StatusOK, gin.H{"mfa_enabled": false})
}

// RegenerateBackupCodes replaces unused recovery codes after confirming password and live TOTP code.
func (h *AuthHandler) RegenerateBackupCodes(c *gin.Context) {
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		Code            string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Current password and TOTP code are required"})
		return
	}
	user, status, errMsg := loadVerifiedLocalUser(c, req.CurrentPassword)
	if user == nil {
		c.JSON(status, gin.H{"error": errMsg})
		return
	}
	if !user.MFAEnabled || user.TOTPSecretEnc == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Two-factor authentication is not enabled"})
		return
	}

	rootSecret := h.mfaRootSecret()
	var plaintextCodes []string
	var errCode int
	var errText string
	err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var current db.User
		if err := tx.Where("id = ?", user.ID).First(&current).Error; err != nil {
			errCode, errText = http.StatusNotFound, "User not found"
			return nil
		}
		if !current.MFAEnabled || current.TOTPSecretEnc == "" {
			errCode, errText = http.StatusBadRequest, "Two-factor authentication is not enabled"
			return nil
		}
		secret, decErr := mfa.DecryptSecret(current.TOTPSecretEnc, rootSecret)
		if decErr != nil {
			errCode, errText = http.StatusInternalServerError, "Unable to read two-factor configuration"
			return nil
		}
		step, ok := mfa.VerifyCode(secret, req.Code, time.Now().UTC(), current.MFALastUsedStep)
		if !ok {
			errCode, errText = http.StatusUnauthorized, "Invalid TOTP verification code"
			return nil
		}
		codes, digestsJSON, genErr := mfa.GenerateBackupCodes(rootSecret)
		if genErr != nil {
			errCode, errText = http.StatusInternalServerError, "Failed to generate recovery codes"
			return nil
		}
		res := tx.Model(&db.User{}).
			Where("id = ? AND mfa_last_used_step < ?", current.ID, step).
			Updates(map[string]any{
				"mfa_backup_codes":   digestsJSON,
				"mfa_last_used_step": step,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			errCode, errText = http.StatusUnauthorized, "Invalid TOTP verification code"
			return nil
		}
		plaintextCodes = codes
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save recovery codes"})
		return
	}
	if errCode != 0 {
		c.JSON(errCode, gin.H{"error": errText})
		return
	}

	audit.RecordFromContext(c, audit.EntryInput{
		Action:     "auth.mfa.backup_regenerated",
		Severity:   audit.SeverityInfo,
		TargetType: "user",
		TargetID:   user.ID,
		Summary:    "Two-factor recovery codes regenerated for " + user.Email,
	})

	c.Header("Cache-Control", "no-store, private")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{"backup_codes": plaintextCodes})
}

// ResetUserMFA allows an administrator to clear TOTP enrollment for a locked-out local user.
func (h *UsersHandler) ResetUserMFA(c *gin.Context) {
	id := c.Param("id")
	var user db.User
	err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}
		if user.AuthType != "local" {
			return errors.New("not a local account")
		}
		if err := tx.Model(&user).Updates(map[string]any{
			"mfa_enabled":        false,
			"totp_secret_enc":    "",
			"pending_totp_enc":   "",
			"mfa_backup_codes":   "",
			"mfa_last_used_step": 0,
		}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", user.ID).Delete(&db.MFAChallenge{}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unable to reset two-factor authentication: " + err.Error()})
		return
	}

	audit.RecordFromContext(c, audit.EntryInput{
		Action:     "user.mfa.reset",
		Severity:   audit.SeverityWarning,
		TargetType: "user",
		TargetID:   user.ID,
		Summary:    "Administrator reset two-factor authentication for " + user.Email,
	})
	c.JSON(http.StatusOK, gin.H{"message": "Two-factor authentication reset", "mfa_enabled": false})
}
