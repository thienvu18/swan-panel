package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/middleware"
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

	var user db.User
	// GORM Parameterized Query: strict immunity against SQL injection
	if err := db.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		slog.Warn("Login failed: email address not found in identity directory", "email", req.Email, "ip", c.ClientIP())
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	if user.Status != "ACTIVE" {
		slog.Warn("Login rejected: account is not active", "email", req.Email, "status", user.Status, "ip", c.ClientIP())
		c.JSON(http.StatusForbidden, gin.H{"error": "Account is pending approval or suspended"})
		return
	}

	// Verify Bcrypt password hash
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		slog.Warn("Login failed: password verification mismatch", "email", req.Email, "ip", c.ClientIP())
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
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

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":       user.ID,
			"email":    user.Email,
			"name":     user.Name,
			"is_admin": user.IsAdmin,
			"status":   user.Status,
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
	NewPassword     string `json:"new_password" binding:"required,min=8"`
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: new password must be at least 8 characters", "details": err.Error()})
		return
	}

	var user db.User
	if err := db.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
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

	if err := db.DB.Model(&user).Update("password_hash", string(newHash)).Error; err != nil {
		slog.Error("Failed to persist updated password", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to persist password change"})
		return
	}

	slog.Info("User password updated successfully", "user_id", user.ID, "email", user.Email)
	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
}
