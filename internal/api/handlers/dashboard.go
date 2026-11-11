package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/vici"
)

type DashboardHandler struct {
	startTime time.Time
}

func NewDashboardHandler() *DashboardHandler {
	return &DashboardHandler{startTime: time.Now()}
}

// GetStats returns summary statistics for the control plane dashboard
func (h *DashboardHandler) GetStats(c *gin.Context) {
	var totalDevices, totalUsers, pendingUsers int64

	if err := db.DB.Model(&db.Device{}).Count(&totalDevices).Error; err != nil {
		slog.Error("Failed to query total device count for dashboard", "error", err)
	}
	if err := db.DB.Model(&db.User{}).Count(&totalUsers).Error; err != nil {
		slog.Error("Failed to query total user count for dashboard", "error", err)
	}
	if err := db.DB.Model(&db.User{}).Where("status = ?", "PENDING").Count(&pendingUsers).Error; err != nil {
		slog.Error("Failed to query pending user count for dashboard", "error", err)
	}

	// Fetch recently registered devices
	var devices []db.Device
	if err := db.DB.Preload("ResourceGroup").Preload("User").Limit(10).Order("created_at desc").Find(&devices).Error; err != nil {
		slog.Error("Failed to query recent devices for dashboard", "error", err)
	}

	// Fetch live active VPN tunnels from strongSwan VICI socket
	viciClient := vici.GetClient("")
	activeSessionsCount := 0
	if liveSAs, err := viciClient.ListSAs(c.Request.Context()); err == nil {
		activeSessionsCount = len(liveSAs)
	}

	uptimeSeconds := int64(time.Since(h.startTime).Seconds())
	slog.Debug("Dashboard stats retrieved", "devices", totalDevices, "users", totalUsers, "active_sessions", activeSessionsCount, "uptime", uptimeSeconds)

	c.JSON(http.StatusOK, gin.H{
		"stats": gin.H{
			"active_sessions": activeSessionsCount,
			"total_devices":   totalDevices,
			"total_users":     totalUsers,
			"pending_users":   pendingUsers,
			"gateway_status":  "online",
			"uptime_seconds":  uptimeSeconds,
		},
		"devices": devices,
	})
}
