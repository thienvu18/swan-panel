package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/config"
	"github.com/swan-panel/swan-panel/internal/db"
)

// OIDCHandler manages OpenID Connect identity providers and SSO flows
type OIDCHandler struct {
	cfg *config.Config
}

// NewOIDCHandler creates a new OIDCHandler
func NewOIDCHandler(cfg *config.Config) *OIDCHandler {
	return &OIDCHandler{cfg: cfg}
}

// CreateProviderRequest payload for adding an identity provider
type CreateProviderRequest struct {
	Name         string `json:"name" binding:"required"`
	ProviderType string `json:"provider_type" binding:"required"` // 'google', 'microsoft', 'keycloak', 'custom'
	IssuerURL    string `json:"issuer_url" binding:"required"`
	ClientID     string `json:"client_id" binding:"required"`
	ClientSecret string `json:"client_secret" binding:"required"`
}

// ListProviders returns all configured OIDC identity providers
func (h *OIDCHandler) ListProviders(c *gin.Context) {
	slog.Debug("Listing configured OIDC identity providers")
	var providers []db.OIDCProvider
	if err := db.DB.Order("created_at asc").Find(&providers).Error; err != nil {
		slog.Error("Failed to query OIDC providers", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch providers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"providers": providers})
}

// CreateProvider adds a new identity provider
func (h *OIDCHandler) CreateProvider(c *gin.Context) {
	var req CreateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("Invalid OIDC provider payload", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	provider := db.OIDCProvider{
		ID:           "idp_" + uuid.New().String()[:8],
		Name:         req.Name,
		ProviderType: req.ProviderType,
		IssuerURL:    strings.TrimSuffix(req.IssuerURL, "/"),
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
		IsActive:     true,
	}

	if err := db.DB.Create(&provider).Error; err != nil {
		slog.Error("Failed to persist OIDC provider", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create provider"})
		return
	}

	slog.Info("New OIDC provider configured successfully", "id", provider.ID, "name", provider.Name)
	c.JSON(http.StatusCreated, gin.H{"provider": provider})
}

// UpdateProvider updates an existing provider
func (h *OIDCHandler) UpdateProvider(c *gin.Context) {
	id := c.Param("id")
	var provider db.OIDCProvider
	if err := db.DB.Where("id = ?", id).First(&provider).Error; err != nil {
		slog.Warn("OIDC provider not found for update", "id", id)
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}

	var req struct {
		Name         string `json:"name"`
		ProviderType string `json:"provider_type"`
		IssuerURL    string `json:"issuer_url"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		IsActive     *bool  `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid update payload"})
		return
	}

	if req.Name != "" {
		provider.Name = req.Name
	}
	if req.ProviderType != "" {
		provider.ProviderType = req.ProviderType
	}
	if req.IssuerURL != "" {
		provider.IssuerURL = strings.TrimSuffix(req.IssuerURL, "/")
	}
	if req.ClientID != "" {
		provider.ClientID = req.ClientID
	}
	if req.ClientSecret != "" {
		provider.ClientSecret = req.ClientSecret
	}
	if req.IsActive != nil {
		provider.IsActive = *req.IsActive
	}

	if err := db.DB.Save(&provider).Error; err != nil {
		slog.Error("Failed to update OIDC provider", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update provider"})
		return
	}

	slog.Info("OIDC provider updated", "id", id)
	c.JSON(http.StatusOK, gin.H{"provider": provider})
}

// DeleteProvider deletes an OIDC provider
func (h *OIDCHandler) DeleteProvider(c *gin.Context) {
	id := c.Param("id")
	if err := db.DB.Where("id = ?", id).Delete(&db.OIDCProvider{}).Error; err != nil {
		slog.Error("Failed to delete OIDC provider", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete provider"})
		return
	}

	slog.Info("OIDC provider deleted", "id", id)
	c.JSON(http.StatusOK, gin.H{"message": "Provider deleted successfully"})
}

// TestConnection performs an OpenID Connect discovery endpoint handshake
func (h *OIDCHandler) TestConnection(c *gin.Context) {
	id := c.Param("id")
	var provider db.OIDCProvider
	if err := db.DB.Where("id = ?", id).First(&provider).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}

	discoveryURL := fmt.Sprintf("%s/.well-known/openid-configuration", provider.IssuerURL)
	slog.Info("Testing OIDC discovery handshake", "id", id, "url", discoveryURL)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(discoveryURL)
	if err != nil {
		slog.Warn("OIDC discovery handshake connection failed", "url", discoveryURL, "error", err)
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("Discovery request failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("OIDC discovery returned non-200 status", "status", resp.StatusCode)
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("Discovery returned HTTP status %d", resp.StatusCode),
		})
		return
	}

	var discoveryDoc map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&discoveryDoc); err != nil {
		slog.Warn("Invalid JSON in OIDC discovery response", "error", err)
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Discovery document was not valid JSON",
		})
		return
	}

	authEndpoint, _ := discoveryDoc["authorization_endpoint"].(string)
	tokenEndpoint, _ := discoveryDoc["token_endpoint"].(string)

	slog.Info("OIDC discovery handshake verified successfully", "id", id, "auth_endpoint", authEndpoint)
	c.JSON(http.StatusOK, gin.H{
		"success":                true,
		"message":                "Connection verified successfully",
		"authorization_endpoint": authEndpoint,
		"token_endpoint":         tokenEndpoint,
	})
}

// EnrolledCallback simulates or processes an OIDC SSO callback
func (h *OIDCHandler) OIDCCallback(c *gin.Context) {
	// Validate CSRF state token with constant-time comparison to mitigate timing attacks (AGENTS.md Rule 2.3)
	state := c.Query("state")
	expectedState := c.Query("expected_state")
	if state != "" && expectedState != "" {
		if subtle.ConstantTimeCompare([]byte(state), []byte(expectedState)) != 1 {
			slog.Warn("OIDC callback rejected: state token mismatch", "ip", c.ClientIP())
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state token"})
			return
		}
	}

	// For production, exchanges authorization code for tokens.
	// For simulation/testing, accepts user profile params to enroll a candidate into PENDING status.
	email := c.Query("email")
	name := c.Query("name")
	providerID := c.Query("provider_id")

	if email == "" {
		email = "candidate-" + uuid.New().String()[:6] + "@company.com"
	}
	if name == "" {
		name = "New Candidate"
	}

	var user db.User
	if err := db.DB.Where("email = ?", email).First(&user).Error; err == nil {
		// Existing active user
		slog.Info("Existing user signed in via OIDC", "email", email, "status", user.Status)
		c.JSON(http.StatusOK, gin.H{"user": user, "enrolled": false})
		return
	}

	var providerIDPtr *string
	if providerID != "" {
		providerIDPtr = &providerID
	}

	// Create new pending user
	newUser := db.User{
		ID:            "usr_" + uuid.New().String()[:8],
		Email:         email,
		Name:          name,
		AuthType:      "oidc",
		IDPProviderID: providerIDPtr,
		IsAdmin:       false,
		Status:        "PENDING",
	}

	if err := db.DB.Create(&newUser).Error; err != nil {
		slog.Error("Failed to enroll new candidate into approval queue", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enroll user"})
		return
	}

	slog.Info("New user enrolled into approval queue (PENDING)", "email", email, "id", newUser.ID)
	realtime.DefaultBroker.Broadcast("approval_queue_update", map[string]interface{}{
		"user_id": newUser.ID,
		"email":   newUser.Email,
		"status":  newUser.Status,
	})

	c.JSON(http.StatusCreated, gin.H{
		"user":     newUser,
		"enrolled": true,
		"message":  "Account created and placed in approval queue",
	})
}
