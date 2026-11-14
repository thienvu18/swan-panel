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
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/logger"
	"github.com/swan-panel/swan-panel/internal/policy"
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

	var command *exec.Cmd
	if isSystemdAvailable() {
		command = exec.CommandContext(c.Request.Context(), "systemctl", action, "strongswan")
	} else if _, err := exec.LookPath("ipsec"); err == nil {
		command = exec.CommandContext(c.Request.Context(), "ipsec", action)
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Neither systemd strongswan unit nor ipsec CLI is available"})
		return
	}
	output, err := command.CombinedOutput()
	if err != nil {
		slog.Error("Daemon command failed", "action", action, "error", err, "output", string(output))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "strongSwan control command failed"})
		return
	}
	if action == "restart" || action == "start" {
		time.Sleep(1 * time.Second)
		if err := vici.GetClient("").Reconnect(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Daemon command completed but VICI is unavailable"})
			return
		}
		if err := policy.Reconcile(c.Request.Context(), db.DB); err != nil {
			slog.Error("Daemon resumed without enforced policy", "error", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Daemon started but security policy was not applied"})
			return
		}
	}
	runtime, err := vici.GetClient("").GetDaemonStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Cannot verify daemon state"})
		return
	}
	expected := "active"
	if action == "stop" {
		expected = "inactive"
	}
	if runtime.Status != expected {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Daemon command returned, but runtime state does not match request", "status": runtime.Status})
		return
	}
	realtime.DefaultBroker.Broadcast("gateway_status", map[string]any{"action": action, "status": runtime.Status})
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Daemon %s completed and verified", action),
		"action":  action, "status": runtime.Status, "timestamp": time.Now().Format(time.RFC3339),
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
