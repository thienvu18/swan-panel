package handlers

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"github.com/swan-panel/swan-panel/internal/pki"
	"github.com/swan-panel/swan-panel/internal/policy"
	"github.com/swan-panel/swan-panel/internal/vici"
)

var (
	errSubnetOverlap           = errors.New("subnet overlap")
	errResourceGroupHasDevices = errors.New("resource group has attached devices")
	errResourceGroupHasRules   = errors.New("resource group is referenced by access rules")
)

// ResourcesHandler manages Devices, Provisioning, Native Profiles, and Resource Groups
type ResourcesHandler struct{}

// NewResourcesHandler creates a new ResourcesHandler
func NewResourcesHandler() *ResourcesHandler {
	return &ResourcesHandler{}
}

// ListDevices returns registered devices (scoped by ownership for standard users)
func (h *ResourcesHandler) ListDevices(c *gin.Context) {
	isAdmin := c.GetBool("is_admin")
	currentUserID := c.GetString("user_id")
	userID := c.Query("user_id")
	devType := c.Query("device_type")
	slog.Debug("Listing devices", "user_id", userID, "device_type", devType, "is_admin", isAdmin)

	query := db.DB.Preload("User").Preload("ResourceGroup")
	// Enforce tenant boundary: Non-admin users can only view their own registered devices
	if !isAdmin {
		query = query.Where("user_id = ?", currentUserID)
	} else {
		if userID != "" {
			query = query.Where("user_id = ?", userID)
		}
		if devType != "" {
			query = query.Where("device_type = ?", devType)
		}
	}

	var devices []db.Device
	if err := query.Order("created_at desc").Find(&devices).Error; err != nil {
		slog.Error("Failed to query devices", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch devices"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"devices": devices})
}

// CreateDevice enrolls a distinct VPN identity. Its certificate never changes on download.
var safeDeviceHostname = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,126}[A-Za-z0-9]$`)

func (h *ResourcesHandler) CreateDevice(c *gin.Context) {
	var req struct {
		Hostname         string  `json:"hostname" binding:"required"`
		DeviceType       string  `json:"device_type" binding:"required"`
		UserID           *string `json:"user_id"`
		ResourceGroupID  string  `json:"resource_group_id" binding:"required"`
		IPAllocationType string  `json:"ip_allocation_type"`
		VirtualIP        string  `json:"virtual_ip"`
		Platform         string  `json:"platform"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid device payload"})
		return
	}
	if !safeDeviceHostname.MatchString(req.Hostname) || strings.Contains(req.Hostname, "..") {
		c.JSON(400, gin.H{"error": "Hostname must be a DNS-compatible identity"})
		return
	}
	if req.DeviceType != "company" && req.DeviceType != "user" {
		c.JSON(400, gin.H{"error": "Invalid device type"})
		return
	}
	if req.IPAllocationType == "" {
		req.IPAllocationType = "dynamic"
	}
	if req.IPAllocationType != "dynamic" && req.IPAllocationType != "static" {
		c.JSON(400, gin.H{"error": "Invalid IP allocation type"})
		return
	}
	if req.Platform == "" {
		req.Platform = "macos"
	}
	switch req.Platform {
	case "macos", "ios", "windows", "linux", "android":
	default:
		c.JSON(400, gin.H{"error": "Unsupported platform"})
		return
	}
	if !c.GetBool("is_admin") {
		if req.DeviceType != "user" {
			c.JSON(403, gin.H{"error": "Cannot provision company node"})
			return
		}
		current := c.GetString("user_id")
		req.UserID = &current
	}
	if req.DeviceType == "user" && (req.UserID == nil || *req.UserID == "") {
		c.JSON(400, gin.H{"error": "User devices require an owner"})
		return
	}
	newDevice := db.Device{
		ID: "dev_" + uuid.NewString(), Hostname: req.Hostname,
		DeviceType: req.DeviceType, ResourceGroupID: req.ResourceGroupID,
		IPAllocationType: req.IPAllocationType, Platform: req.Platform,
		UserID: req.UserID,
	}
	// Certificates can be generated before the DB transaction, then persisted
	// exactly once. Never derive any client IKE ID from an unverified email.
	releaseMut := maintenance.DefaultCoordinator.BeginMutation()
	defer releaseMut()

	manager, err := pki.GetPKIManager()
	if err != nil {
		c.JSON(503, gin.H{"error": "CA unavailable"})
		return
	}
	bundle, err := manager.GenerateClientCertificate(req.Hostname, req.Hostname)
	if err != nil {
		c.JSON(503, gin.H{"error": "Certificate issuance failed"})
		return
	}
	newDevice.CertSerial = bundle.SerialNumber
	newDevice.CertFingerprint = bundle.Fingerprint
	if err := manager.StoreDeviceBundle(newDevice.ID, bundle); err != nil {
		c.JSON(503, gin.H{"error": "Failed to persist client identity"})
		return
	}
	var createDevAudit db.AuditLog
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		var group db.ResourceGroup
		if err := tx.Where("id = ?", req.ResourceGroupID).First(&group).Error; err != nil {
			return fmt.Errorf("resource group not found")
		}
		if req.UserID != nil {
			var owner db.User
			if err := tx.Where("id = ? AND status = ?", *req.UserID, "ACTIVE").First(&owner).Error; err != nil {
				return fmt.Errorf("device owner is not active")
			}
		}
		if req.IPAllocationType == "static" {
			ip, err := netip.ParseAddr(req.VirtualIP)
			cidr, errCIDR := netip.ParsePrefix(group.CIDRBlock)
			if err != nil || errCIDR != nil || !ip.Is4() || !cidr.Contains(ip) || isReservedVirtualIP(cidr, ip) {
				return fmt.Errorf("static IPv4 address is not a usable address within resource group")
			}
			newDevice.VirtualIP = ip.String()
		} else {
			ip, err := allocateDynamicIP(tx, group.CIDRBlock)
			if err != nil {
				return err
			}
			newDevice.VirtualIP = ip
		}
		if err := tx.Create(&newDevice).Error; err != nil { // Unique virtual_ip prevents parallel duplicate assignment.
			return err
		}
		var recErr error
		createDevAudit, recErr = audit.RecordTx(tx, audit.EnrichFromContext(c, audit.EntryInput{
			Action:     "device.created",
			Severity:   audit.SeverityInfo,
			TargetType: "device",
			TargetID:   newDevice.ID,
			Summary:    fmt.Sprintf("Device enrolled: %s (%s)", newDevice.Hostname, newDevice.VirtualIP),
			Details:    map[string]any{"hostname": newDevice.Hostname, "virtual_ip": newDevice.VirtualIP, "platform": newDevice.Platform},
		}))
		return recErr
	})
	if err != nil {
		_ = manager.DeleteDeviceBundle(newDevice.ID)
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	realtime.DefaultBroker.Broadcast("device_created", map[string]any{"device_id": newDevice.ID})
	audit.NotifyCommitted(createDevAudit)
	if err := policy.Reconcile(c.Request.Context(), db.DB); err != nil {
		slog.Error("Device enrolled but runtime cannot be enforced", "id", newDevice.ID, "error", err)
		c.JSON(http.StatusAccepted, gin.H{"device": newDevice, "runtime_status": "pending", "message": "Identity persisted but gateway policy application failed"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"device": newDevice, "runtime_status": "applied", "message": "Device enrolled; configuration ready for download"})
}

// DownloadDeviceConfig delivers the previously issued identity. Legacy devices
// whose private keys were not persisted must be re-enrolled, not silently reissued.
func (h *ResourcesHandler) DownloadDeviceConfig(c *gin.Context) {
	var dev db.Device
	if err := db.DB.First(&dev, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(404, gin.H{"error": "Device not found"})
		return
	}
	if !c.GetBool("is_admin") && (dev.UserID == nil || *dev.UserID != c.GetString("user_id")) {
		c.JSON(403, gin.H{"error": "Device does not belong to you"})
		return
	}
	if dev.IsRevoked {
		c.JSON(403, gin.H{"error": "Device identity revoked"})
		return
	}
	manager, err := pki.GetPKIManager()
	if err != nil {
		c.JSON(503, gin.H{"error": "CA unavailable"})
		return
	}
	bundle, err := manager.LoadDeviceBundle(dev.ID)
	if err != nil || dev.CertSerial == "" || bundle.SerialNumber != dev.CertSerial || bundle.Fingerprint != dev.CertFingerprint {
		c.JSON(409, gin.H{"error": "Existing identity cannot be recovered safely; revoke and re-enroll the device"})
		return
	}
	var sw db.StrongswanSettings
	if err := db.DB.First(&sw, 1).Error; err != nil {
		c.JSON(503, gin.H{"error": "Gateway settings unavailable"})
		return
	}
	result, err := policy.NewCompiler(db.DB).CompileForDevice(c.Request.Context(), dev.ID)
	if err != nil {
		c.JSON(503, gin.H{"error": "Policy compilation failed"})
		return
	}
	c.Header("Cache-Control", "no-store, private")
	c.Header("Pragma", "no-cache")
	platform := strings.ToLower(c.Param("platform"))
	if platform == "android-native" || platform == "android-p12" {
		setup, err := pki.BuildAndroidNativeSetup(sw.ServerEndpoint, sw.ServerIdentity, dev.Hostname, bundle)
		if err != nil {
			slog.Error("Native Android credential export failed", "device_id", dev.ID, "error", err)
			c.JSON(503, gin.H{"error": "Unable to export native Android credentials"})
			return
		}
		// Keep the randomly encrypted PKCS#12 bytes and its password together.
		// Neither the private key nor import password is persisted in API state.
		c.JSON(http.StatusOK, setup)
		return
	}
	if platform == "apple" || platform == "macos" || platform == "ios" {
		profile, profileErr := pki.BuildAppleProfileNamed(sw.ServerEndpoint, sw.ServerIdentity, dev.Hostname, sw.VPNName, bundle)
		if profileErr != nil {
			c.JSON(503, gin.H{"error": "Cannot generate Apple IKEv2 profile"})
			return
		}
		signed, signErr := manager.SignAppleProfile([]byte(profile))
		if signErr != nil {
			slog.Error("Apple profile S/MIME signing failed", "device_id", dev.ID, "error", signErr)
			c.JSON(503, gin.H{"error": "Cannot sign Apple configuration profile"})
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", dev.Hostname+".mobileconfig"))
		c.Data(http.StatusOK, "application/x-apple-aspen-config", signed)
		return
	}
	var filename, contentType, body string
	switch platform {
	case "windows", "ps1":
		filename = "setup-" + dev.Hostname + ".ps1"
		contentType = "text/plain; charset=utf-8"
		body, err = pki.BuildWindowsInstallerWithRoutesNamed(sw.ServerEndpoint, dev.Hostname, sw.VPNName, bundle, result.AllowedSelectors)
	case "linux", "swanctl", "sh":
		if c.Query("format") == "conf" {
			filename = "swanctl-" + dev.Hostname + ".conf"
			contentType = "text/plain; charset=utf-8"
			body, err = pki.BuildLinuxConfig(sw.ServerEndpoint, sw.ServerIdentity, dev.Hostname, result.AllowedSelectors)
		} else {
			filename = "setup-" + dev.Hostname + ".sh"
			contentType = "text/x-shellscript; charset=utf-8"
			body, err = pki.BuildLinuxInstaller(sw.ServerEndpoint, sw.ServerIdentity, dev.Hostname, bundle, result.AllowedSelectors)
		}
	case "android", "sswan":
		filename = dev.Hostname + ".sswan"
		contentType = "application/vnd.strongswan.profile"
		body, err = pki.BuildAndroidProfileNamed(sw.ServerEndpoint, sw.ServerIdentity, dev.Hostname, sw.VPNName, bundle, result.AllowedSelectors)
	default:
		c.JSON(400, gin.H{"error": "Unsupported platform"})
		return
	}
	if err != nil || body == "" {
		slog.Error("Client profile generation failed", "device_id", dev.ID, "platform", platform, "error", err)
		c.JSON(503, gin.H{"error": "Unable to generate a valid VPN profile"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(200, contentType, []byte(body))
}

func applyRevocation(ctx context.Context, dev *db.Device) error {
	releaseMut := maintenance.DefaultCoordinator.BeginMutation()
	defer releaseMut()

	if dev.CertSerial == "" {
		return errors.New("issued certificate serial is missing")
	}
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(dev).Update("is_revoked", true).Error; err != nil {
			return err
		}
		rev := db.CertificateRevocation{Serial: dev.CertSerial, DeviceID: dev.ID, RevokedAt: time.Now().UTC()}
		return tx.Where("serial = ?", rev.Serial).FirstOrCreate(&rev).Error
	})
	if err != nil {
		return err
	}
	// Continue every revocation action even if one enforcement component fails.
	// A failed nftables update must not prevent terminating live IKE sessions.
	var failures []error
	if err := policy.NewFirewall(db.DB).Apply(ctx); err != nil {
		failures = append(failures, fmt.Errorf("firewall revocation: %w", err))
	}
	if err := publishRevocationLedger(ctx); err != nil {
		failures = append(failures, fmt.Errorf("signed CRL publication: %w", err))
	}
	if err := policy.Reconcile(ctx, db.DB); err != nil {
		failures = append(failures, fmt.Errorf("device policy reconciliation: %w", err))
	}
	if os.Getenv("APP_ENV") != "test" {
		client := vici.GetClient("")
		// Best effort: even if the central reconciler stopped at nftables,
		// disallow new handshakes using the old per-device VICI connection.
		if err := client.UnloadConnection(ctx, policy.ConnectionName(dev.ID)); err != nil {
			slog.Warn("Device connection may already have been unloaded", "id", dev.ID, "error", err)
		}
		active, err := client.ListSAs(ctx)
		if err != nil {
			failures = append(failures, fmt.Errorf("inspect active IKE SAs: %w", err))
		} else {
			for _, sa := range active {
				if sa.PeerIdentity == dev.Hostname || sa.IKEName == policy.ConnectionName(dev.ID) {
					if err := client.TerminateSA(ctx, sa.IKEName); err != nil {
						failures = append(failures, fmt.Errorf("terminate IKE SA %s: %w", sa.IKEName, err))
					}
				}
			}
			// Best-effort immediate cleanup after terminating a revoked device.
			// Charon refuses pool removal while a lease is still held; the next
			// reconciliation retries it without disconnecting unrelated sessions.
			if err := client.UnloadPool(ctx, policy.PoolName(dev.ID)); err != nil {
				slog.Warn("Deferred revoked device pool cleanup", "device_id", dev.ID, "error", err)
			}
		}
	}
	manager, err := pki.GetPKIManager()
	if err != nil {
		failures = append(failures, err)
	} else if err := manager.DeleteDeviceBundle(dev.ID); err != nil {
		failures = append(failures, fmt.Errorf("remove exported device identity: %w", err))
	}
	if len(failures) > 0 {
		joined := errors.Join(failures...)
		// Ensure RuntimeApplyState is never left as "applied" when CRL/SA/firewall revocation failed.
		_ = db.DB.Model(&db.RuntimeApplyState{}).Where("id = ?", 1).Updates(map[string]any{
			"status":     "failed",
			"last_error": joined.Error(),
		}).Error
		return joined
	}
	return nil
}

func publishRevocationLedger(ctx context.Context) error {
	var records []db.CertificateRevocation
	if err := db.DB.WithContext(ctx).Find(&records).Error; err != nil {
		return err
	}
	entries := make([]pki.RevokedCertificate, 0, len(records))
	for _, record := range records {
		entries = append(entries, pki.RevokedCertificate{Serial: record.Serial, RevokedAt: record.RevokedAt})
	}
	manager, err := pki.GetPKIManager()
	if err != nil {
		return err
	}
	return manager.PublishCRL(ctx, entries)
}

func (h *ResourcesHandler) RevokeDevice(c *gin.Context) {
	var dev db.Device
	if err := db.DB.First(&dev, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(404, gin.H{"error": "Device not found"})
		return
	}
	if !c.GetBool("is_admin") && (dev.UserID == nil || *dev.UserID != c.GetString("user_id")) {
		c.JSON(403, gin.H{"error": "Device does not belong to you"})
		return
	}
	if err := applyRevocation(c.Request.Context(), &dev); err != nil {
		slog.Error("Device revoked in database; enforcement reconciliation pending", "id", dev.ID, "error", err)
		_ = audit.RecordFromContext(c, audit.EntryInput{
			Action:     "device.revoked",
			Severity:   audit.SeverityCritical,
			TargetType: "device",
			TargetID:   dev.ID,
			Summary:    fmt.Sprintf("Device certificate revoked (gateway enforcement pending): %s", dev.Hostname),
			Details:    map[string]any{"hostname": dev.Hostname, "cert_serial": dev.CertSerial, "runtime_status": "pending"},
		})
		c.JSON(503, gin.H{"error": "Device revocation recorded, but gateway enforcement is pending", "revocation_pending": true})
		return
	}
	realtime.DefaultBroker.Broadcast("device_revoked", map[string]any{"device_id": dev.ID})
	if err := audit.RecordFromContext(c, audit.EntryInput{
		Action:     "device.revoked",
		Severity:   audit.SeverityCritical,
		TargetType: "device",
		TargetID:   dev.ID,
		Summary:    "Device certificate revoked: " + dev.Hostname,
		Details:    map[string]any{"hostname": dev.Hostname, "cert_serial": dev.CertSerial},
	}); err != nil {
		c.JSON(503, gin.H{"error": "Device revoked, but audit persistence failed"})
		return
	}
	c.JSON(200, gin.H{"message": "Device certificate revoked and active sessions terminated"})
}

func (h *ResourcesHandler) DeleteDevice(c *gin.Context) {
	var dev db.Device
	if err := db.DB.First(&dev, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(404, gin.H{"error": "Device not found"})
		return
	}
	if err := applyRevocation(c.Request.Context(), &dev); err != nil {
		c.JSON(503, gin.H{"error": "Cannot delete a device until certificate revocation is enforced"})
		return
	}
	var delDevAudit db.AuditLog
	if err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&dev).Error; err != nil {
			return err
		}
		var recErr error
		delDevAudit, recErr = audit.RecordTx(tx, audit.EnrichFromContext(c, audit.EntryInput{
			Action:     "device.deleted",
			Severity:   audit.SeverityWarning,
			TargetType: "device",
			TargetID:   dev.ID,
			Summary:    "Device revoked and deleted: " + dev.Hostname,
			Details:    map[string]any{"hostname": dev.Hostname, "cert_serial": dev.CertSerial},
		}))
		return recErr
	}); err != nil {
		c.JSON(500, gin.H{"error": "Failed to delete device"})
		return
	}
	audit.NotifyCommitted(delDevAudit)
	c.JSON(200, gin.H{"message": "Device revoked and deleted"})
}

// ListResourceGroups returns subnets with node capacity
func (h *ResourcesHandler) ListResourceGroups(c *gin.Context) {
	slog.Debug("Listing resource groups")
	var groups []db.ResourceGroup
	if err := db.DB.Preload("Devices").Order("name asc").Find(&groups).Error; err != nil {
		slog.Error("Failed to query resource groups", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch resource groups"})
		return
	}

	type GroupDTO struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		CIDRBlock         string `json:"cidr_block"`
		Description       string `json:"description"`
		Capacity          int    `json:"capacity"`
		EnrolledNodeCount int    `json:"enrolled_node_count"`
	}

	var dtoList []GroupDTO
	for _, g := range groups {
		capacity := calculateCIDRCapacity(g.CIDRBlock)
		activeNodes := 0
		for _, d := range g.Devices {
			if !d.IsRevoked {
				activeNodes++
			}
		}
		dtoList = append(dtoList, GroupDTO{
			ID:                g.ID,
			Name:              g.Name,
			CIDRBlock:         g.CIDRBlock,
			Description:       g.Description,
			Capacity:          capacity,
			EnrolledNodeCount: activeNodes,
		})
	}

	c.JSON(http.StatusOK, gin.H{"resource_groups": dtoList})
}

// CreateResourceGroup creates a new subnet
func (h *ResourcesHandler) CreateResourceGroup(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		CIDRBlock   string `json:"cidr_block" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	// Normalize IPv4 subnet and disallow overlapping VPN virtual-address pools.
	newPrefix, err := netip.ParsePrefix(req.CIDRBlock)
	if err != nil || !newPrefix.Addr().Is4() || newPrefix.Bits() < 8 || newPrefix.Bits() > 30 {
		c.JSON(400, gin.H{"error": "Resource group must be a valid IPv4 CIDR (/8 to /30)"})
		return
	}
	newPrefix = newPrefix.Masked()
	req.CIDRBlock = newPrefix.String()
	rg := db.ResourceGroup{
		ID:          "rg_" + uuid.New().String()[:8],
		Name:        req.Name,
		CIDRBlock:   req.CIDRBlock,
		Description: req.Description,
	}
	// Serialize the overlap check with INSERT on the same SQLite writer.
	// A competing request must not pass a stale check and create a collision.
	releaseMut := maintenance.DefaultCoordinator.BeginMutation()
	defer releaseMut()

	var createRGAudit db.AuditLog
	err = db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var existing []db.ResourceGroup
		if err := tx.Find(&existing).Error; err != nil {
			return err
		}
		for _, group := range existing {
			old, err := netip.ParsePrefix(group.CIDRBlock)
			if err == nil && (newPrefix.Contains(old.Addr()) || old.Contains(newPrefix.Addr())) {
				return errSubnetOverlap
			}
		}
		if err := tx.Create(&rg).Error; err != nil {
			return err
		}
		var recErr error
		createRGAudit, recErr = audit.RecordTx(tx, audit.EnrichFromContext(c, audit.EntryInput{
			Action:     "resource_group.created",
			Severity:   audit.SeverityInfo,
			TargetType: "resource_group",
			TargetID:   rg.ID,
			Summary:    fmt.Sprintf("Subnet created: %s (%s)", rg.Name, rg.CIDRBlock),
			Details:    map[string]any{"cidr_block": rg.CIDRBlock},
		}))
		return recErr
	})
	if errors.Is(err, errSubnetOverlap) {
		c.JSON(http.StatusConflict, gin.H{"error": "This VPN subnet overlaps an existing resource group"})
		return
	}
	if err != nil {
		slog.Error("Failed to create resource group", "name", req.Name, "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Failed to create resource group"})
		return
	}

	slog.Info("Resource group created", "id", rg.ID, "name", rg.Name, "cidr", rg.CIDRBlock)
	audit.NotifyCommitted(createRGAudit)
	c.JSON(http.StatusCreated, gin.H{"resource_group": rg})
}

// DeleteResourceGroup checks references and removes the group within one
// SQLite write transaction. Device enrollment and access-rule creation also
// use transactions; the shared SQLite writer serializes conflicting writes.
func (h *ResourcesHandler) DeleteResourceGroup(c *gin.Context) {
	id := c.Param("id")
	releaseMut := maintenance.DefaultCoordinator.BeginMutation()
	defer releaseMut()

	var delRGAudit db.AuditLog
	err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var group db.ResourceGroup
		if err := tx.Where("id = ?", id).First(&group).Error; err != nil {
			return err
		}
		var deviceCount int64
		if err := tx.Model(&db.Device{}).Where("resource_group_id = ?", id).Count(&deviceCount).Error; err != nil {
			return fmt.Errorf("count attached devices: %w", err)
		}
		if deviceCount > 0 {
			return errResourceGroupHasDevices
		}
		var ruleCount int64
		if err := tx.Model(&db.AccessRule{}).
			Where("target_type = ? AND target_value = ?", "resource_group", id).
			Count(&ruleCount).Error; err != nil {
			return fmt.Errorf("count referencing access rules: %w", err)
		}
		if ruleCount > 0 {
			return errResourceGroupHasRules
		}
		if err := tx.Delete(&group).Error; err != nil {
			return err
		}
		var recErr error
		delRGAudit, recErr = audit.RecordTx(tx, audit.EnrichFromContext(c, audit.EntryInput{
			Action:     "resource_group.deleted",
			Severity:   audit.SeverityInfo,
			TargetType: "resource_group",
			TargetID:   id,
			Summary:    "Subnet deleted: " + id,
		}))
		return recErr
	})
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Resource group not found"})
	case errors.Is(err, errResourceGroupHasDevices):
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot delete resource group while devices are attached to it"})
	case errors.Is(err, errResourceGroupHasRules):
		c.JSON(http.StatusConflict, gin.H{"error": "Remove referencing access rules before deleting this subnet"})
	case err != nil:
		slog.Error("Resource group deletion failed; transaction rolled back", "id", id, "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Resource group deletion failed; no changes committed"})
	default:
		slog.Info("Resource group deleted", "id", id)
		audit.NotifyCommitted(delRGAudit)
		c.JSON(http.StatusOK, gin.H{"message": "Resource group deleted successfully"})
	}
}

func calculateCIDRCapacity(cidr string) int {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0
	}
	ones, bits := ipNet.Mask.Size()
	hostBits := bits - ones
	if hostBits <= 1 {
		return 1
	}
	// Total usable hosts: 2^hostBits - 2
	capacity := int(math.Pow(2, float64(hostBits))) - 3
	if capacity < 0 {
		return 0
	}
	return capacity
}

// Never issue the subnet network, gateway, or broadcast address.
func isReservedVirtualIP(prefix netip.Prefix, addr netip.Addr) bool {
	if !addr.Is4() || !prefix.Contains(addr) {
		return true
	}
	start := prefix.Masked().Addr().As4()
	host := addr.As4()
	first := binary.BigEndian.Uint32(start[:])
	value := binary.BigEndian.Uint32(host[:])
	end := first | (^uint32(0) >> uint(prefix.Bits()))
	return value == first || value == first+1 || value == end
}

func allocateDynamicIP(tx *gorm.DB, cidr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}

	// Fetch existing IPs in this subnet
	var existingIPs []string
	if err := tx.Model(&db.Device{}).Pluck("virtual_ip", &existingIPs).Error; err != nil {
		return "", fmt.Errorf("load allocated IPs: %w", err)
	}
	usedSet := make(map[string]bool)
	for _, ip := range existingIPs {
		usedSet[ip] = true
	}

	ip := ipNet.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("IPv6 not currently supported for dynamic pool")
	}

	netIP := binary.BigEndian.Uint32(ip)
	mask := binary.BigEndian.Uint32(ipNet.Mask)
	broadcast := netIP | ^mask

	// Usable host IPs: from netIP + 1 up to broadcast - 1
	for cur := netIP + 1; cur < broadcast; cur++ {
		curBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(curBytes, cur)
		candidate := net.IP(curBytes).String()
		if !usedSet[candidate] && !isReservedVirtualIP(netip.MustParsePrefix(cidr), netip.MustParseAddr(candidate)) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no available IPs left in subnet pool")
}
