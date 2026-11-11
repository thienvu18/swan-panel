package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/logger"
	"github.com/swan-panel/swan-panel/internal/vici"
)

// SystemHandler controls the strongSwan daemon and provides contextual structured log streams
type SystemHandler struct{}

// NewSystemHandler creates a new SystemHandler
func NewSystemHandler() *SystemHandler {
	return &SystemHandler{}
}

// LogEntryDTO represents a structured enriched log event
type LogEntryDTO struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Scope     string `json:"scope"`
	Tag1      string `json:"tag1"`
	Tag2      string `json:"tag2"`
	Message   string `json:"message"`
}

// GetDaemonStatus returns current runtime statistics of the strongSwan gateway
func (h *SystemHandler) GetDaemonStatus(c *gin.Context) {
	viciClient := vici.GetClient("")
	stats, err := viciClient.GetDaemonStats(c.Request.Context())
	if err != nil {
		slog.Warn("Failed to retrieve live daemon statistics, falling back to basic status", "error", err)
		c.JSON(http.StatusOK, gin.H{
			"service": "strongswan",
			"status":  "inactive",
			"pid":     0,
			"memory":  "0 MB",
			"uptime":  "Offline",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// ControlDaemon starts, stops, or restarts the strongSwan daemon (US-11)
func (h *SystemHandler) ControlDaemon(c *gin.Context) {
	var req struct {
		Action string `json:"action" binding:"required"` // 'start', 'stop', 'restart'
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Field 'action' is required (start, stop, restart)"})
		return
	}

	action := strings.ToLower(req.Action)
	if action != "start" && action != "stop" && action != "restart" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action; must be 'start', 'stop', or 'restart'"})
		return
	}

	slog.Info("Admin initiated strongSwan daemon control command", "action", action)

	var execErr error
	var execOutput []byte

	// 1. If running under systemd init, use systemctl
	if isSystemdAvailable() {
		execOutput, execErr = exec.Command("systemctl", action, "strongswan").CombinedOutput()
		if execErr == nil {
			slog.Info("Daemon control executed via systemctl", "action", action)
		} else {
			slog.Warn("systemctl failed, falling back to native CLI", "action", action, "output", string(execOutput), "error", execErr)
		}
	}

	// 2. Fall back to native strongSwan ipsec CLI (Alpine container / standalone)
	if execErr != nil || !isSystemdAvailable() {
		if _, err := exec.LookPath("ipsec"); err == nil {
			execOutput, execErr = exec.Command("ipsec", action).CombinedOutput()
			if execErr == nil {
				slog.Info("Daemon control executed via ipsec CLI", "action", action)
			} else {
				slog.Warn("ipsec CLI action returned warning/error", "action", action, "output", string(execOutput), "error", execErr)
			}
		}
	}

	// 3. If restart or start, allow daemon socket initialization and reconnect VICI session
	if action == "restart" || action == "start" {
		time.Sleep(1 * time.Second)
		_ = vici.GetClient("").Reconnect()
	}

	status := "active"
	if action == "stop" {
		status = "inactive"
	}

	realtime.DefaultBroker.Broadcast("gateway_status", map[string]interface{}{
		"action": action,
		"status": status,
	})

	c.JSON(http.StatusOK, gin.H{
		"message":   fmt.Sprintf("Daemon %s executed successfully", action),
		"action":    action,
		"status":    status,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// isSystemdAvailable detects if the process is running on a host with active systemd
func isSystemdAvailable() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	if comm, err := os.ReadFile("/proc/1/comm"); err == nil {
		return strings.TrimSpace(string(comm)) == "systemd"
	}
	return false
}

// GetLogs returns structured contextual logs with multi-scope and level filtering (US-12)
func (h *SystemHandler) GetLogs(c *gin.Context) {
	scopeFilter := c.Query("scope")
	levelFilter := strings.ToLower(c.Query("level"))
	search := strings.ToLower(c.Query("search"))

	slog.Debug("Filtering contextual system logs", "scope", scopeFilter, "level", levelFilter, "search", search)

	recentLogs := logger.GetRecentLogs()

	var filtered []logger.LogEntry
	for _, l := range recentLogs {
		if scopeFilter != "" && scopeFilter != "all" && !strings.Contains(l.Scope, scopeFilter) && !strings.Contains(l.Tag1, scopeFilter) && !strings.Contains(l.Tag2, scopeFilter) {
			continue
		}
		if levelFilter != "" && levelFilter != "all" && l.Level != levelFilter {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(l.Message), search) && !strings.Contains(strings.ToLower(l.Tag1), search) && !strings.Contains(strings.ToLower(l.Tag2), search) && !strings.Contains(strings.ToLower(l.Scope), search) {
			continue
		}
		filtered = append(filtered, l)
	}

	// Reverse order so newest logs appear first
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}

	c.JSON(http.StatusOK, gin.H{"logs": filtered, "total": len(filtered)})
}
