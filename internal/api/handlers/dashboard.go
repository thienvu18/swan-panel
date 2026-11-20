package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/vici"
)

type DashboardHandler struct{ startTime time.Time }

func NewDashboardHandler() *DashboardHandler { return &DashboardHandler{startTime: time.Now()} }

func (h *DashboardHandler) GetStats(c *gin.Context) {
	ctx := c.Request.Context()
	isAdmin := c.GetBool("is_admin")
	query := db.DB.WithContext(ctx).Model(&db.Device{})
	if !isAdmin {
		query = query.Where("user_id = ?", c.GetString("user_id"))
	}
	var totalDevices, totalUsers, pendingUsers int64
	if err := query.Count(&totalDevices).Error; err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Unable to query device statistics"})
		return
	}
	if isAdmin {
		if err := db.DB.WithContext(ctx).Model(&db.User{}).Count(&totalUsers).Error; err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Unable to query user statistics"})
			return
		}
		if err := db.DB.WithContext(ctx).Model(&db.User{}).Where("status = ?", "PENDING").Count(&pendingUsers).Error; err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Unable to query user statistics"})
			return
		}
	}
	var devices []db.Device
	deviceQuery := db.DB.WithContext(ctx).Preload("ResourceGroup").Limit(10).Order("created_at desc")
	if !isAdmin {
		deviceQuery = deviceQuery.Where("user_id = ?", c.GetString("user_id"))
	}
	if err := deviceQuery.Find(&devices).Error; err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Unable to query recent devices"})
		return
	}
	// A failed VICI read is not evidence of zero live sessions.
	status := "offline"
	var activeSessionsCount *int
	liveSAs, err := vici.GetClient("").ListSAs(ctx)
	if err == nil {
		status = "online"
		liveSAs, err = sessionsForCurrentUser(c, liveSAs)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Unable to resolve session ownership"})
			return
		}
		count := len(liveSAs)
		activeSessionsCount = &count
	} else {
		slog.Warn("VPN runtime unavailable in dashboard", "error", err)
	}
	stats := gin.H{
		"active_sessions": activeSessionsCount, "total_devices": totalDevices,
		"gateway_status": status, "uptime_seconds": int64(time.Since(h.startTime).Seconds()),
	}
	if isAdmin {
		stats["total_users"] = totalUsers
		stats["pending_users"] = pendingUsers
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats, "devices": devices})
}
