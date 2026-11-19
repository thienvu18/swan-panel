package handlers

import (
	"log/slog"
	"net/http"
	"net/netip"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/db"
)

// DNSHandler manages Split-DNS, MagicDNS settings, and Custom Overrides
type DNSHandler struct{}

// NewDNSHandler creates a new DNSHandler
func NewDNSHandler() *DNSHandler {
	return &DNSHandler{}
}

// GetSettings returns the current DNS configuration
func (h *DNSHandler) GetSettings(c *gin.Context) {
	var settings db.DNSSettings
	if err := db.DB.First(&settings, 1).Error; err != nil {
		slog.Error("Failed to fetch DNS settings", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"dns_settings": settings})
}

// UpdateSettings updates MagicDNS suffix and upstream resolvers
func (h *DNSHandler) UpdateSettings(c *gin.Context) {
	var req struct {
		MagicDNSSuffix    string `json:"magic_dns_suffix"`
		UpstreamResolvers string `json:"upstream_resolvers"`
		IsMagicDNSEnabled *bool  `json:"is_magic_dns_enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	var settings db.DNSSettings
	if err := db.DB.First(&settings, 1).Error; err != nil {
		settings.ID = 1
	}

	if req.MagicDNSSuffix != "" {
		settings.MagicDNSSuffix = req.MagicDNSSuffix
	}
	if req.UpstreamResolvers != "" {
		settings.UpstreamResolvers = req.UpstreamResolvers
	}
	if req.IsMagicDNSEnabled != nil {
		settings.IsMagicDNSEnabled = *req.IsMagicDNSEnabled
	}

	if err := db.DB.Save(&settings).Error; err != nil {
		slog.Error("Failed to update DNS settings", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update settings"})
		return
	}

	slog.Info("DNS engine settings updated", "magic_suffix", settings.MagicDNSSuffix, "upstreams", settings.UpstreamResolvers)
	c.JSON(http.StatusOK, gin.H{"dns_settings": settings, "message": "DNS settings updated successfully"})
}

// ListOverrides returns all Custom DNS Override records
func (h *DNSHandler) ListOverrides(c *gin.Context) {
	var overrides []db.DNSOverride
	if err := db.DB.Order("hostname asc").Find(&overrides).Error; err != nil {
		slog.Error("Failed to fetch DNS overrides", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch overrides"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"dns_overrides": overrides})
}

var dnsOverrideHostnameLabel = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func isValidDNSOverrideTarget(recordType, target string) bool {
	value := strings.TrimSpace(target)
	switch recordType {
	case "A":
		address, err := netip.ParseAddr(value)
		return err == nil && address.Is4()
	case "CNAME":
		hostname := strings.TrimSuffix(value, ".")
		if len(hostname) == 0 || len(hostname) > 253 || !strings.Contains(hostname, ".") {
			return false
		}
		if address, err := netip.ParseAddr(hostname); err == nil && address.Is4() {
			return false
		}
		for _, label := range strings.Split(hostname, ".") {
			if !dnsOverrideHostnameLabel.MatchString(label) {
				return false
			}
		}
		return true
	default:
		return true // Other record types may be created through existing API integrations.
	}
}

// CreateOverride creates a new custom DNS mapping (e.g. gitlab.internal -> 192.168.1.100)
func (h *DNSHandler) CreateOverride(c *gin.Context) {
	var req struct {
		Hostname      string `json:"hostname" binding:"required"`
		RecordType    string `json:"record_type"`
		TargetAddress string `json:"target_address" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	req.RecordType = strings.ToUpper(strings.TrimSpace(req.RecordType))
	if req.RecordType == "" {
		req.RecordType = "A"
	}
	req.TargetAddress = strings.TrimSpace(req.TargetAddress)
	if !isValidDNSOverrideTarget(req.RecordType, req.TargetAddress) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid DNS override target for record type " + req.RecordType})
		return
	}

	override := db.DNSOverride{
		ID:            "dns_" + uuid.New().String()[:8],
		Hostname:      req.Hostname,
		RecordType:    req.RecordType,
		TargetAddress: req.TargetAddress,
	}

	if err := db.DB.Create(&override).Error; err != nil {
		slog.Error("Failed to create DNS override", "hostname", req.Hostname, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create DNS override"})
		return
	}

	slog.Info("Custom DNS override created", "id", override.ID, "hostname", override.Hostname, "target", override.TargetAddress)
	c.JSON(http.StatusCreated, gin.H{"dns_override": override})
}

// DeleteOverride deletes a DNS override
func (h *DNSHandler) DeleteOverride(c *gin.Context) {
	id := c.Param("id")
	if err := db.DB.Where("id = ?", id).Delete(&db.DNSOverride{}).Error; err != nil {
		slog.Error("Failed to delete DNS override", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete override"})
		return
	}

	slog.Info("DNS override deleted", "id", id)
	c.JSON(http.StatusOK, gin.H{"message": "DNS override deleted successfully"})
}
