package handlers

import (
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/pki"
	"github.com/swan-panel/swan-panel/internal/policy"
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

var gatewayDNSLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

func validGatewayIdentifier(value string) bool {
	if value == "" || strings.ContainsAny(value, "\r\n\t ';\x60$\\") {
		return false
	}
	if ip := net.ParseIP(value); ip != nil {
		return true
	}
	if len(value) > 253 || strings.Contains(value, "..") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if !gatewayDNSLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func validateGatewaySettings(req *db.StrongswanSettings) bool {
	if !validGatewayIdentifier(req.ServerEndpoint) || !validGatewayIdentifier(req.ServerIdentity) {
		return false
	}
	if req.DPDAction != "" && req.DPDAction != "clear" && req.DPDAction != "restart" && req.DPDAction != "trap" {
		return false
	}
	for _, value := range []string{req.IKEProposals, req.ESPProposals, req.ReauthTime, req.RekeyTimeIKE, req.RekeyTimeChild, req.DPDDelay, req.DPDTimeout} {
		if len(value) > 256 || strings.ContainsAny(value, "\r\n;`$\\") {
			return false
		}
	}
	return true
}

// UpdateStrongswanSettings updates strongSwan data plane parameters
func (h *SettingsHandler) UpdateStrongswanSettings(c *gin.Context) {
	var req db.StrongswanSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("Update strongSwan settings rejected: invalid request payload", "error", err, "ip", c.ClientIP())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid configuration payload", "details": err.Error()})
		return
	}

	if !validGatewayIdentifier(req.ServerEndpoint) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server_endpoint: provide an IP address or DNS hostname"})
		return
	}
	if !validGatewayIdentifier(req.ServerIdentity) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server_identity: provide an IP address or DNS hostname"})
		return
	}
	if !validateGatewaySettings(&req) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid strongSwan settings"})
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

	// Reissue a gateway certificate only when its identity changes/approaches expiry,
	// and synchronize device-specific connections, never the old wildcard tunnel.
	manager, err := pki.GetPKIManager()
	if err != nil {
		c.JSON(http.StatusAccepted, gin.H{"runtime_status": "pending", "error": "CA unavailable; configuration stored"})
		return
	}
	if err := manager.EnsureServerCertificates(req.ServerIdentity, nil); err != nil {
		slog.Error("Gateway certificate setup failed", "error", err)
		c.JSON(http.StatusAccepted, gin.H{"runtime_status": "pending", "error": "Gateway certificate configuration failed"})
		return
	}
	if err := publishRevocationLedger(c.Request.Context()); err != nil {
		slog.Error("CRL sync failed", "error", err)
		c.JSON(http.StatusAccepted, gin.H{"runtime_status": "pending", "error": "CRL synchronization failed"})
		return
	}
	if err := policy.Reconcile(c.Request.Context(), db.DB); err != nil {
		slog.Error("Gateway policy reconciliation failed", "error", err)
		c.JSON(http.StatusAccepted, gin.H{"runtime_status": "pending", "error": "Configuration stored but not applied to gateway"})
		return
	}
	realtime.DefaultBroker.Broadcast("strongswan_settings_updated", req)
	c.JSON(http.StatusOK, gin.H{"settings": req, "runtime_status": "applied", "message": "Configuration applied to gateway"})
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
