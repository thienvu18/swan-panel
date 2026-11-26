package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/middleware"
	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/config"
	"github.com/swan-panel/swan-panel/internal/db"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type AuthHandler struct {
	cfg *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{cfg: cfg}
}

// Login authenticates a user account (Local or Admin)
// Uses parameterized GORM queries with 100% SQL injection immunity
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("Login rejected: malformed request payload", "ip", c.ClientIP(), "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email or password format", "details": err.Error()})
		return
	}

	if setup, err := db.SetupRequired(); err != nil || setup {
		c.JSON(http.StatusForbidden, gin.H{"error": "Complete one-time administrator setup first"})
		return
	}
	var user db.User
	// GORM Parameterized Query: strict immunity against SQL injection
	if err := db.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		slog.Warn("Login failed: email address not found in identity directory", "email", req.Email, "ip", c.ClientIP())
		audit.TrackFailedLogin(c, req.Email, "unknown_email")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	if user.Status != "ACTIVE" {
		slog.Warn("Login rejected: account is not active", "email", req.Email, "status", user.Status, "ip", c.ClientIP())
		audit.TrackFailedLogin(c, req.Email, "account_inactive")
		c.JSON(http.StatusForbidden, gin.H{"error": "Account is pending approval or suspended"})
		return
	}

	if user.AuthType != "local" || user.PasswordHash == "" {
		audit.TrackFailedLogin(c, req.Email, "oidc_required")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Use your identity provider to sign in"})
		return
	}

	// Verify Bcrypt password hash
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		slog.Warn("Login failed: password verification mismatch", "email", req.Email, "ip", c.ClientIP())
		audit.TrackFailedLogin(c, req.Email, "invalid_password")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	if user.MFAEnabled && user.TOTPSecretEnc != "" {
		h.issueMFAChallenge(c, &user)
		return
	}

	// Issue 24-hour signed JWT session token
	token, err := middleware.GenerateToken(&user, h.cfg.JWTSecret, 24*time.Hour)
	if err != nil {
		slog.Error("Login failure: error creating signed session token", "email", req.Email, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session token"})
		return
	}

	slog.Info("User authenticated successfully", "user_id", user.ID, "email", user.Email, "is_admin", user.IsAdmin, "ip", c.ClientIP())
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
		Summary:    "User signed in via local credentials: " + user.Email,
	})

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

// Me returns the profile of the currently authenticated user
func (h *AuthHandler) Me(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var user db.User
	if err := db.DB.Preload("UserGroups").Where("id = ?", userID).First(&user).Error; err != nil {
		slog.Warn("User profile lookup failed: record not found", "user_id", userID)
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	slog.Debug("User profile retrieved successfully", "user_id", userID, "email", user.Email)
	c.JSON(http.StatusOK, gin.H{"user": user})
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// ChangePassword allows the authenticated user to safely update their account password
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("Password change rejected: invalid input", "error", err, "ip", c.ClientIP())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid password change request", "details": err.Error()})
		return
	}

	if len(req.NewPassword) < 12 || len(req.NewPassword) > 72 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "New password must be 12 to 72 bytes"})
		return
	}

	var user db.User
	if err := db.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if user.AuthType != "local" || user.PasswordHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SSO accounts cannot change local passwords"})
		return
	}
	// Verify current password with constant-time bcrypt verification
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		slog.Warn("Password change rejected: current password mismatch", "user_id", userID, "ip", c.ClientIP())
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("Failed to hash new password", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to securely hash password"})
		return
	}

	if err := db.DB.Model(&user).Updates(map[string]any{
		"password_hash":    string(newHash),
		"pending_totp_enc": "",
	}).Error; err != nil {
		slog.Error("Failed to persist updated password", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to persist password change"})
		return
	}

	slog.Info("User password updated successfully", "user_id", user.ID, "email", user.Email)
	audit.RecordFromContext(c, audit.EntryInput{
		Action:     "auth.password.changed",
		Severity:   audit.SeverityInfo,
		TargetType: "user",
		TargetID:   user.ID,
		Summary:    "Password changed for account " + user.Email,
	})
	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
}
