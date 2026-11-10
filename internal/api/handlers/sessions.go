package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/vici"
)

// SessionsHandler manages runtime IKE SA sessions and force disconnects
type SessionsHandler struct{}

// NewSessionsHandler creates a new SessionsHandler
func NewSessionsHandler() *SessionsHandler {
	return &SessionsHandler{}
}

// ListSessions returns all live active VPN sessions from VICI
func (h *SessionsHandler) ListSessions(c *gin.Context) {
	viciClient := vici.GetClient("")
	sessions, err := viciClient.ListSAs(c.Request.Context())
	if err != nil {
		slog.Error("Failed to list active sessions from VICI", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch active sessions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"sessions": sessions, "count": len(sessions)})
}

// TerminateSession immediately disconnects an active IKE SA session (US-10)
func (h *SessionsHandler) TerminateSession(c *gin.Context) {
	var req struct {
		IKEName string `json:"ike_name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Field 'ike_name' is required"})
		return
	}

	slog.Info("Admin requested immediate session termination", "ike_name", req.IKEName)
	viciClient := vici.GetClient("")
	if err := viciClient.TerminateSA(c.Request.Context(), req.IKEName); err != nil {
		slog.Error("Failed to terminate session via VICI", "ike_name", req.IKEName, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to terminate session"})
		return
	}

	// Broadcast session update over SSE so all open browser dashboards refresh immediately
	realtime.DefaultBroker.Broadcast("session_terminated", map[string]interface{}{
		"ike_name": req.IKEName,
	})

	c.JSON(http.StatusOK, gin.H{"message": "Session terminated successfully", "ike_name": req.IKEName})
}
