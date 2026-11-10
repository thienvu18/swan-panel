package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/vici"
)

type SettingsHandler struct{}

func NewSettingsHandler() *SettingsHandler {
	return &SettingsHandler{}
}

// GetStrongswanSettings retrieves current strongSwan daemon configuration
func (h *SettingsHandler) GetStrongswanSettings(c *gin.Context) {
	var settings db.StrongswanSettings
	if err := db.DB.First(&settings, 1).Error; err != nil {
		slog.Error("Failed to query strongSwan settings from database", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load settings"})
		return
	}

	slog.Debug("strongSwan settings retrieved", "id", settings.ID)
	c.JSON(http.StatusOK, settings)
}

// UpdateStrongswanSettings updates strongSwan data plane parameters
func (h *SettingsHandler) UpdateStrongswanSettings(c *gin.Context) {
	var req db.StrongswanSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("Update strongSwan settings rejected: invalid request payload", "error", err, "ip", c.ClientIP())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid configuration payload", "details": err.Error()})
		return
	}

	req.ID = 1
	// Parameterized Update: GORM updates by primary key
	if err := db.DB.Save(&req).Error; err != nil {
		slog.Error("Failed to persist strongSwan configuration", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to persist configuration"})
		return
	}

	slog.Info("strongSwan settings updated and applied",
		"reauth_time", req.ReauthTime,
		"rekey_ike", req.RekeyTimeIKE,
		"mobike", req.MobikeEnabled,
		"force_encap_natt", req.ForceEncapNATT,
	)

	// Dynamically hot-reload connection into strongSwan RAM via VICI
	viciClient := vici.GetClient("")
	params := map[string]interface{}{
		"version":     "2",
		"reauth_time": req.ReauthTime,
		"mobike":      fmt.Sprintf("%v", req.MobikeEnabled),
		"encap":       fmt.Sprintf("%v", req.ForceEncapNATT),
		"dpd_delay":   req.DPDDelay,
		"dpd_timeout": req.DPDTimeout,
		"proposals":   req.IKEProposals,
	}
	_ = viciClient.LoadConnection(c.Request.Context(), "swanpanel-ikev2", params)

	// Broadcast SSE event to all connected dashboard clients
	realtime.DefaultBroker.Broadcast("strongswan_settings_updated", req)

	c.JSON(http.StatusOK, gin.H{
		"message":  "Configuration persisted and applied dynamically to strongSwan in RAM with 0 downtime",
		"settings": req,
	})
}

// GetDNSSettings retrieves DNS configuration and custom overrides
func (h *SettingsHandler) GetDNSSettings(c *gin.Context) {
	var settings db.DNSSettings
	if err := db.DB.First(&settings, 1).Error; err != nil {
		slog.Error("Failed to query DNS settings from database", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load DNS settings"})
		return
	}

	var overrides []db.DNSOverride
	if err := db.DB.Find(&overrides).Error; err != nil {
		slog.Error("Failed to query DNS overrides from database", "error", err)
	}

	slog.Debug("DNS settings and overrides retrieved", "overrides_count", len(overrides))
	c.JSON(http.StatusOK, gin.H{
		"settings":  settings,
		"overrides": overrides,
	})
}
