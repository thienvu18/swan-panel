package handlers

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/pki"
	"github.com/swan-panel/swan-panel/internal/vici"
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

// CreateDevice provisions a new Company Node (US-04) or User Device (US-05)
func (h *ResourcesHandler) CreateDevice(c *gin.Context) {
	var req struct {
		Hostname         string  `json:"hostname" binding:"required"`
		DeviceType       string  `json:"device_type" binding:"required"` // 'company' or 'user'
		UserID           *string `json:"user_id"`
		ResourceGroupID  string  `json:"resource_group_id" binding:"required"`
		IPAllocationType string  `json:"ip_allocation_type"` // 'dynamic' or 'static'
		VirtualIP        string  `json:"virtual_ip"`
		Platform         string  `json:"platform"` // 'macos', 'windows', 'linux', 'ios', 'android'
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("Invalid create device payload", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	if req.DeviceType != "company" && req.DeviceType != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device type; must be 'company' or 'user'"})
		return
	}

	isAdmin := c.GetBool("is_admin")
	currentUserID := c.GetString("user_id")
	if !isAdmin {
		if req.DeviceType != "user" {
			slog.Warn("Non-admin user attempted to provision company node", "user_id", currentUserID)
			c.JSON(http.StatusForbidden, gin.H{"error": "Only administrators can provision company nodes"})
			return
		}
		// Enforce ownership to current user
		req.UserID = &currentUserID
	}

	if req.IPAllocationType == "" {
		req.IPAllocationType = "dynamic"
	}
	if req.Platform == "" {
		req.Platform = "macos"
	}

	// Verify Resource Group exists
	var rg db.ResourceGroup
	if err := db.DB.Where("id = ?", req.ResourceGroupID).First(&rg).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Resource Group not found"})
		return
	}

	// Calculate or assign Virtual IP
	virtualIP := req.VirtualIP
	if req.IPAllocationType == "static" {
		if virtualIP == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Static IP must be specified when using static allocation"})
			return
		}
		// Validate static IP falls within subnet
		_, ipNet, err := net.ParseCIDR(rg.CIDRBlock)
		if err != nil || !ipNet.Contains(net.ParseIP(virtualIP)) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Static IP %s does not belong to subnet %s", virtualIP, rg.CIDRBlock)})
			return
		}
	} else {
		// Dynamic IP allocation: pick next available IP in subnet
		allocated, err := allocateDynamicIP(rg.CIDRBlock)
		if err != nil {
			slog.Error("Failed to allocate dynamic IP from subnet", "cidr", rg.CIDRBlock, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to allocate IP from subnet pool"})
			return
		}
		virtualIP = allocated
	}

	// Issue X.509 client certificate
	pkiMgr, err := pki.GetPKIManager()
	if err != nil {
		slog.Error("Failed to get PKI manager", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "PKI engine unavailable"})
		return
	}

	identity := req.Hostname
	if req.DeviceType == "user" && req.UserID != nil {
		var u db.User
		if err := db.DB.Where("id = ?", *req.UserID).First(&u).Error; err == nil {
			identity = u.Email
		}
	}

	certBundle, err := pkiMgr.GenerateClientCertificate(req.Hostname, identity)
	if err != nil {
		slog.Error("Failed to generate client certificate", "hostname", req.Hostname, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate certificate"})
		return
	}

	// Check for Company Node vs User Device ownership
	var assignedUserID *string
	if req.DeviceType == "user" {
		assignedUserID = req.UserID
	}

	newDevice := db.Device{
		ID:               "dev_" + uuid.New().String()[:8],
		Hostname:         req.Hostname,
		DeviceType:       req.DeviceType,
		UserID:           assignedUserID,
		ResourceGroupID:  req.ResourceGroupID,
		IPAllocationType: req.IPAllocationType,
		VirtualIP:        virtualIP,
		Platform:         req.Platform,
		CertFingerprint:  certBundle.Fingerprint,
		IsRevoked:        false,
	}

	if err := db.DB.Create(&newDevice).Error; err != nil {
		slog.Error("Failed to save device record", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save device"})
		return
	}

	slog.Info("Device provisioned successfully", "id", newDevice.ID, "hostname", newDevice.Hostname, "ip", newDevice.VirtualIP, "type", newDevice.DeviceType)
	realtime.DefaultBroker.Broadcast("device_created", map[string]interface{}{
		"device_id": newDevice.ID,
		"hostname":  newDevice.Hostname,
		"ip":        newDevice.VirtualIP,
	})

	c.JSON(http.StatusCreated, gin.H{
		"device":     newDevice,
		"cert_pem":   certBundle.CertPEM,
		"ca_pem":     certBundle.CACertPEM,
		"message":    "Device provisioned and certificate issued",
	})
}

// DownloadDeviceConfig generates and serves signed configuration profiles (.mobileconfig, .ps1, swanctl.conf) (US-04, US-05)
func (h *ResourcesHandler) DownloadDeviceConfig(c *gin.Context) {
	id := c.Param("id")
	platform := c.Param("platform")
	if platform == "" {
		platform = c.Query("platform")
	}

	var dev db.Device
	if err := db.DB.Where("id = ?", id).First(&dev).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	// Enforce Object-Level Authorization (BOLA): Non-admins can only download configs for devices they own
	isAdmin := c.GetBool("is_admin")
	currentUserID := c.GetString("user_id")
	if !isAdmin && (dev.UserID == nil || *dev.UserID != currentUserID) {
		slog.Warn("Unauthorized access attempt to device configuration", "device_id", id, "user_id", currentUserID)
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: you do not own this device"})
		return
	}

	if dev.IsRevoked {
		c.JSON(http.StatusForbidden, gin.H{"error": "Device certificate has been revoked"})
		return
	}

	// Get strongSwan settings for server endpoint
	var sw db.StrongswanSettings
	if err := db.DB.First(&sw, 1).Error; err != nil {
		sw.ServerEndpoint = "vpn.company.com"
	}

	pkiMgr, _ := pki.GetPKIManager()
	certBundle, err := pkiMgr.GenerateClientCertificate(dev.Hostname, dev.Hostname)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate certificate bundle"})
		return
	}

	switch strings.ToLower(platform) {
	case "apple", "macos", "ios":
		cfg := pki.GenerateAppleMobileconfig(sw.ServerEndpoint, dev.Hostname, certBundle)
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.mobileconfig\"", dev.Hostname))
		c.Data(http.StatusOK, "application/x-apple-aspen-config", []byte(cfg))

	case "windows", "ps1":
		cfg := pki.GenerateWindowsPowerShellScript(sw.ServerEndpoint, dev.Hostname, certBundle)
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"setup-%s.ps1\"", dev.Hostname))
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(cfg))

	case "linux", "swanctl", "sh":
		if c.Query("format") == "conf" {
			cfg := pki.GenerateSwanctlConfig(sw.ServerEndpoint, dev.Hostname, certBundle)
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"swanctl-%s.conf\"", dev.Hostname))
			c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(cfg))
		} else {
			cfg := pki.GenerateLinuxInstallerScript(sw.ServerEndpoint, dev.Hostname, certBundle)
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"setup-%s.sh\"", dev.Hostname))
			c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", []byte(cfg))
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported platform; must be 'apple', 'windows', or 'linux'"})
	}
}

// RevokeDevice revokes device certificate and closes session (US-18)
func (h *ResourcesHandler) RevokeDevice(c *gin.Context) {
	id := c.Param("id")
	var dev db.Device
	if err := db.DB.Where("id = ?", id).First(&dev).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	// Enforce Object-Level Authorization (BOLA): Non-admins can only revoke devices they own
	isAdmin := c.GetBool("is_admin")
	currentUserID := c.GetString("user_id")
	if !isAdmin && (dev.UserID == nil || *dev.UserID != currentUserID) {
		slog.Warn("Unauthorized attempt to revoke device", "device_id", id, "user_id", currentUserID)
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: you do not own this device"})
		return
	}

	dev.IsRevoked = true
	if err := db.DB.Save(&dev).Error; err != nil {
		slog.Error("Failed to revoke device", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke device"})
		return
	}

	// Terminate active tunnel if currently connected
	viciClient := vici.GetClient("")
	_ = viciClient.TerminateSA(c.Request.Context(), dev.Hostname)

	slog.Info("Device revoked and certificate invalidated", "id", id, "hostname", dev.Hostname)
	realtime.DefaultBroker.Broadcast("device_revoked", map[string]interface{}{"device_id": id})
	c.JSON(http.StatusOK, gin.H{"message": "Device revoked successfully", "device": dev})
}

// DeleteDevice deletes a device
func (h *ResourcesHandler) DeleteDevice(c *gin.Context) {
	id := c.Param("id")
	if err := db.DB.Where("id = ?", id).Delete(&db.Device{}).Error; err != nil {
		slog.Error("Failed to delete device", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete device"})
		return
	}

	slog.Info("Device record deleted", "id", id)
	c.JSON(http.StatusOK, gin.H{"message": "Device deleted successfully"})
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
		ID               string `json:"id"`
		Name             string `json:"name"`
		CIDRBlock        string `json:"cidr_block"`
		Description      string `json:"description"`
		Capacity         int    `json:"capacity"`
		EnrolledNodeCount int   `json:"enrolled_node_count"`
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

	// Validate CIDR format
	if _, _, err := net.ParseCIDR(req.CIDRBlock); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid CIDR block format (e.g. 10.100.0.0/16)"})
		return
	}

	rg := db.ResourceGroup{
		ID:          "rg_" + uuid.New().String()[:8],
		Name:        req.Name,
		CIDRBlock:   req.CIDRBlock,
		Description: req.Description,
	}

	if err := db.DB.Create(&rg).Error; err != nil {
		slog.Error("Failed to create resource group", "name", req.Name, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create resource group"})
		return
	}

	slog.Info("Resource group created", "id", rg.ID, "name", rg.Name, "cidr", rg.CIDRBlock)
	c.JSON(http.StatusCreated, gin.H{"resource_group": rg})
}

// DeleteResourceGroup deletes a subnet if no devices are attached
func (h *ResourcesHandler) DeleteResourceGroup(c *gin.Context) {
	id := c.Param("id")

	var devCount int64
	db.DB.Model(&db.Device{}).Where("resource_group_id = ?", id).Count(&devCount)
	if devCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot delete resource group while devices are attached to it"})
		return
	}

	if err := db.DB.Where("id = ?", id).Delete(&db.ResourceGroup{}).Error; err != nil {
		slog.Error("Failed to delete resource group", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete resource group"})
		return
	}

	slog.Info("Resource group deleted", "id", id)
	c.JSON(http.StatusOK, gin.H{"message": "Resource group deleted successfully"})
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
	capacity := int(math.Pow(2, float64(hostBits))) - 2
	if capacity < 0 {
		return 0
	}
	return capacity
}

func allocateDynamicIP(cidr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}

	// Fetch existing IPs in this subnet
	var existingIPs []string
	db.DB.Model(&db.Device{}).Pluck("virtual_ip", &existingIPs)
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
		if !usedSet[candidate] {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no available IPs left in subnet pool")
}
