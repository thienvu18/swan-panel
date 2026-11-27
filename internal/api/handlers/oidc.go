package handlers

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/config"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"gorm.io/gorm"
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
	Name             string `json:"name" binding:"required"`
	ProviderType     string `json:"provider_type" binding:"required"` // 'google', 'microsoft', 'keycloak', 'custom'
	IssuerURL        string `json:"issuer_url" binding:"required"`
	ClientID         string `json:"client_id" binding:"required"`
	ClientSecret     string `json:"client_secret" binding:"required"`
	AuthorizationURL string `json:"authorization_url"`
	TokenURL         string `json:"token_url"`
	JWKSURL          string `json:"jwks_url"`
	UserInfoURL      string `json:"userinfo_url"`
	ClaimMapping     string `json:"claim_mapping"`
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

	callback, callbackErr := h.callbackURL(c)
	configured := strings.TrimSpace(os.Getenv("PUBLIC_URL")) != ""
	c.JSON(http.StatusOK, gin.H{
		"providers":                  providers,
		"redirect_uri":               callback,
		"external_origin_configured": configured,
		"production":                 h.cfg.Environment == "production",
		"redirect_uri_valid":         callbackErr == nil,
	})
}

// CreateProvider adds a new identity provider
func (h *OIDCHandler) CreateProvider(c *gin.Context) {
	var req CreateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("Invalid OIDC provider payload", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	if err := validateOIDCConfiguration(req.ProviderType, req.IssuerURL, req.AuthorizationURL, req.TokenURL, req.JWKSURL, req.UserInfoURL, req.ClaimMapping); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	provider := db.OIDCProvider{
		ID:               "idp_" + uuid.New().String()[:8],
		Name:             req.Name,
		ProviderType:     req.ProviderType,
		IssuerURL:        strings.TrimSuffix(req.IssuerURL, "/"),
		ClientID:         req.ClientID,
		ClientSecret:     req.ClientSecret,
		AuthorizationURL: req.AuthorizationURL,
		TokenURL:         req.TokenURL,
		JWKSURL:          req.JWKSURL,
		UserInfoURL:      req.UserInfoURL,
		ClaimMapping:     req.ClaimMapping,
		IsActive:         true,
	}

	releaseMut := maintenance.DefaultCoordinator.BeginMutation()
	defer releaseMut()

	var createIDPAudit db.AuditLog
	if err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&provider).Error; err != nil {
			return err
		}
		var recErr error
		createIDPAudit, recErr = audit.RecordTx(tx, audit.EnrichFromContext(c, audit.EntryInput{
			Action:     "oidc.provider.created",
			Severity:   audit.SeverityInfo,
			TargetType: "oidc_provider",
			TargetID:   provider.ID,
			Summary:    "OIDC provider created: " + provider.Name,
			Details:    map[string]any{"provider_type": provider.ProviderType},
		}))
		return recErr
	}); err != nil {
		slog.Error("Failed to persist OIDC provider", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create provider"})
		return
	}

	slog.Info("New OIDC provider configured successfully", "id", provider.ID, "name", provider.Name)
	audit.NotifyCommitted(createIDPAudit)
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
		Name             string  `json:"name"`
		ProviderType     string  `json:"provider_type"`
		IssuerURL        string  `json:"issuer_url"`
		ClientID         string  `json:"client_id"`
		ClientSecret     string  `json:"client_secret"`
		IsActive         *bool   `json:"is_active"`
		AuthorizationURL *string `json:"authorization_url"`
		TokenURL         *string `json:"token_url"`
		JWKSURL          *string `json:"jwks_url"`
		UserInfoURL      *string `json:"userinfo_url"`
		ClaimMapping     *string `json:"claim_mapping"`
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
	if req.AuthorizationURL != nil {
		provider.AuthorizationURL = *req.AuthorizationURL
	}
	if req.TokenURL != nil {
		provider.TokenURL = *req.TokenURL
	}
	if req.JWKSURL != nil {
		provider.JWKSURL = *req.JWKSURL
	}
	if req.UserInfoURL != nil {
		provider.UserInfoURL = *req.UserInfoURL
	}
	if req.ClaimMapping != nil {
		provider.ClaimMapping = *req.ClaimMapping
	}
	if err := validateOIDCConfiguration(provider.ProviderType, provider.IssuerURL, provider.AuthorizationURL, provider.TokenURL, provider.JWKSURL, provider.UserInfoURL, provider.ClaimMapping); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.IsActive != nil {
		provider.IsActive = *req.IsActive
	}

	// Provider ID is part of the identity binding. Once users exist, changing
	// issuer/client ID/type could authenticate them against a different issuer.
	var linked int64
	if err := db.DB.Model(&db.User{}).Where("idp_provider_id = ?", id).Count(&linked).Error; err != nil {
		c.JSON(500, gin.H{"error": "Identity directory unavailable"})
		return
	}
	if linked > 0 {
		var previous db.OIDCProvider
		if err := db.DB.First(&previous, "id = ?", id).Error; err != nil {
			c.JSON(404, gin.H{"error": "Provider missing"})
			return
		}
		if previous.IssuerURL != provider.IssuerURL || previous.ClientID != provider.ClientID || previous.ProviderType != provider.ProviderType {
			c.JSON(http.StatusConflict, gin.H{"error": "Issuer, client ID and provider type cannot be changed after users are linked; create a new provider"})
			return
		}
	}
	releaseMut := maintenance.DefaultCoordinator.BeginMutation()
	defer releaseMut()

	var updIDPAudit db.AuditLog
	if err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&provider).Error; err != nil {
			return err
		}
		var recErr error
		updIDPAudit, recErr = audit.RecordTx(tx, audit.EnrichFromContext(c, audit.EntryInput{
			Action:     "oidc.provider.updated",
			Severity:   audit.SeverityInfo,
			TargetType: "oidc_provider",
			TargetID:   provider.ID,
			Summary:    "OIDC provider updated: " + provider.Name,
			Details:    map[string]any{"is_active": provider.IsActive},
		}))
		return recErr
	}); err != nil {
		slog.Error("Failed to update OIDC provider", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update provider"})
		return
	}

	slog.Info("OIDC provider updated", "id", id)
	audit.NotifyCommitted(updIDPAudit)
	c.JSON(http.StatusOK, gin.H{"provider": provider})
}

// DeleteProvider deletes an OIDC provider
func (h *OIDCHandler) DeleteProvider(c *gin.Context) {
	id := c.Param("id")
	var linked int64
	if err := db.DB.Model(&db.User{}).Where("idp_provider_id = ?", id).Count(&linked).Error; err != nil {
		c.JSON(500, gin.H{"error": "Identity directory unavailable"})
		return
	}
	if linked > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Provider has linked identities; disable it instead of deleting it"})
		return
	}
	releaseMut := maintenance.DefaultCoordinator.BeginMutation()
	defer releaseMut()

	var delIDPAudit db.AuditLog
	if err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Delete(&db.OIDCProvider{}).Error; err != nil {
			return err
		}
		var recErr error
		delIDPAudit, recErr = audit.RecordTx(tx, audit.EnrichFromContext(c, audit.EntryInput{
			Action:     "oidc.provider.deleted",
			Severity:   audit.SeverityWarning,
			TargetType: "oidc_provider",
			TargetID:   id,
			Summary:    "OIDC provider deleted: " + id,
		}))
		return recErr
	}); err != nil {
		slog.Error("Failed to delete OIDC provider", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete provider"})
		return
	}

	slog.Info("OIDC provider deleted", "id", id)
	audit.NotifyCommitted(delIDPAudit)
	c.JSON(http.StatusOK, gin.H{"message": "Provider deleted successfully"})
}

// TestConnection validates the provider metadata via the same library used for sign-in.
func (h *OIDCHandler) TestConnection(c *gin.Context) {
	var provider db.OIDCProvider
	if err := db.DB.First(&provider, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}
	upstream, _, err := buildOIDCClient(c.Request.Context(), provider, "")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "OIDC metadata verification failed"})
		return
	}
	endpoint := upstream.Endpoint()
	c.JSON(http.StatusOK, gin.H{
		"success":                true,
		"message":                "Provider metadata verified",
		"authorization_endpoint": endpoint.AuthURL,
		"token_endpoint":         endpoint.TokenURL,
	})
}
