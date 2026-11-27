package handlers

import (
	"crypto/x509/pkix"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"github.com/swan-panel/swan-panel/internal/pki"
	"github.com/swan-panel/swan-panel/internal/policy"
	"gorm.io/gorm"
)

var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)

func (h *SettingsHandler) GetCAIdentity(c *gin.Context) {
	manager, err := pki.GetPKIManager()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "CA unavailable"})
		return
	}
	var devices, revocations int64
	if err := db.DB.Unscoped().Model(&db.Device{}).Count(&devices).Error; err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Unable to verify CA lifecycle"})
		return
	}
	if err := db.DB.Model(&db.CertificateRevocation{}).Count(&revocations).Error; err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Unable to verify CA lifecycle"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ca": manager.CAIdentity(), "can_rotate": devices == 0 && revocations == 0})
}

type rotateCARequest struct {
	Confirm      string `json:"confirm" binding:"required"`
	CommonName   string `json:"common_name" binding:"required"`
	Organization string `json:"organization" binding:"required"`
	Country      string `json:"country" binding:"required"`
	Province     string `json:"province"`
	Locality     string `json:"locality"`
}

func (h *SettingsHandler) RotateCAIdentity(c *gin.Context) {
	var req rotateCARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid CA subject"})
		return
	}
	if req.Confirm != "ROTATE CA" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Explicit ROTATE CA confirmation required"})
		return
	}
	for _, field := range []string{req.CommonName, req.Organization, req.Province, req.Locality} {
		if len(field) > 128 || strings.ContainsAny(field, "\x00\r\n") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "CA names must be single-line values under 129 characters"})
			return
		}
	}
	if !countryCode.MatchString(req.Country) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CA country must be an ISO 3166-1 alpha-2 code"})
		return
	}
	var devices, revocations int64
	if err := db.DB.Unscoped().Model(&db.Device{}).Count(&devices).Error; err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Cannot verify issued devices"})
		return
	}
	if err := db.DB.Model(&db.CertificateRevocation{}).Count(&revocations).Error; err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Cannot verify revocations"})
		return
	}
	if devices != 0 || revocations != 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "CA subject is immutable after issuing devices; changing it requires an explicit CA migration"})
		return
	}
	manager, err := pki.GetPKIManager()
	if err != nil {
		c.JSON(503, gin.H{"error": "CA unavailable"})
		return
	}
	name := pkix.Name{CommonName: strings.TrimSpace(req.CommonName),
		Organization: []string{strings.TrimSpace(req.Organization)},
		Country:      []string{req.Country}, Province: []string{strings.TrimSpace(req.Province)},
		Locality: []string{strings.TrimSpace(req.Locality)}}
	releaseMut := maintenance.DefaultCoordinator.BeginMutation()
	defer releaseMut()

	if err := manager.RotateRootCA(name); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("CA rotation failed: %v", err)})
		return
	}
	// The CA is already rotated; a later apply failure cannot roll it back
	// safely. Persist both runtime retry state and audit evidence in one transaction.
	var auditEntry db.AuditLog
	if err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&db.RuntimeApplyState{}).Where("id = ?", 1).Update("status", "pending").Error; err != nil {
			return err
		}
		var recErr error
		auditEntry, recErr = audit.RecordTx(tx, audit.EnrichFromContext(c, audit.EntryInput{
			Action:     "gateway.ca.rotated",
			Severity:   audit.SeverityCritical,
			TargetType: "ca",
			TargetID:   manager.CAIdentity().Serial,
			Summary:    "Root CA rotated: " + name.CommonName,
			Details:    map[string]any{"common_name": name.CommonName, "organization": req.Organization, "country": req.Country},
		}))
		return recErr
	}); err != nil {
		c.JSON(http.StatusAccepted, gin.H{"ca": manager.CAIdentity(), "runtime_status": "pending", "message": "CA rotated; gateway state recording requires attention"})
		return
	}
	audit.NotifyCommitted(auditEntry)
	if err := policy.ReconcileGatewayWithinMutation(c.Request.Context(), db.DB); err != nil {
		c.JSON(http.StatusAccepted, gin.H{"ca": manager.CAIdentity(), "runtime_status": "pending", "message": "CA rotated; gateway reconciliation pending"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ca": manager.CAIdentity(), "runtime_status": "applied"})
}
