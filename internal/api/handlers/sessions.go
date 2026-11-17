package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
	"github.com/swan-panel/swan-panel/internal/vici"
)

type SessionsHandler struct{}

func NewSessionsHandler() *SessionsHandler { return &SessionsHandler{} }

// Unmapped identities must never be disclosed to non-administrators.
func visibleSessions(sessions []vici.SessionInfo, devices []db.Device) []vici.SessionInfo {
	ownedConnections := make(map[string]db.Device, len(devices))
	for _, device := range devices {
		if !device.IsRevoked {
			ownedConnections[policy.ConnectionName(device.ID)] = device
		}
	}
	result := make([]vici.SessionInfo, 0, len(sessions))
	for _, session := range sessions {
		device, owned := ownedConnections[session.IKEName]
		if !owned || session.PeerIdentity != device.Hostname {
			continue
		}
		if session.VirtualIP != "" && session.VirtualIP != device.VirtualIP {
			continue
		}
		result = append(result, session)
	}
	return result
}

func sessionsForCurrentUser(c *gin.Context, sessions []vici.SessionInfo) ([]vici.SessionInfo, error) {
	if c.GetBool("is_admin") {
		return sessions, nil
	}
	var devices []db.Device
	if err := db.DB.WithContext(c.Request.Context()).Where("user_id = ? AND is_revoked = ?", c.GetString("user_id"), false).Find(&devices).Error; err != nil {
		return nil, err
	}
	return visibleSessions(sessions, devices), nil
}

func (h *SessionsHandler) ListSessions(c *gin.Context) {
	sessions, err := vici.GetClient("").ListSAs(c.Request.Context())
	if err != nil {
		slog.Error("Failed to list active sessions from VICI", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "strongSwan runtime unavailable"})
		return
	}
	sessions, err = sessionsForCurrentUser(c, sessions)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Unable to resolve session ownership"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions, "count": len(sessions)})
}

func (h *SessionsHandler) TerminateSession(c *gin.Context) {
	var req struct {
		IKEName string `json:"ike_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Field 'ike_name' is required"})
		return
	}
	if err := vici.GetClient("").TerminateSA(c.Request.Context(), req.IKEName); err != nil {
		slog.Error("Failed to terminate session via VICI", "ike_name", req.IKEName, "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Unable to terminate IKE session"})
		return
	}
	realtime.DefaultBroker.Broadcast("session_terminated", map[string]any{"ike_name": req.IKEName})
	c.JSON(http.StatusOK, gin.H{"message": "Session terminated successfully", "ike_name": req.IKEName})
}
