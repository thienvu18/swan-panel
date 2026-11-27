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
	"github.com/swan-panel/swan-panel/internal/audit"
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
		slog.Warn("Failed to retrieve live daemon statistics", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"service":           "strongswan",
			"status":            "unknown",
			"error":             "VICI unavailable",
			"pid":               0,
			"memory":            "0 MB",
			"uptime":            "Offline",
			"control_supported": os.Getenv("SWANPANEL_EXTERNAL_STRONGSWAN") != "1",
		})
		return
	}

	stats.ControlSupported = os.Getenv("SWANPANEL_EXTERNAL_STRONGSWAN") != "1"
	c.JSON(http.StatusOK, stats)
}

// ControlDaemon starts, stops, or restarts the strongSwan daemon (US-11)
func (h *SystemHandler) ControlDaemon(c *gin.Context) {
	var req struct {
		Action string `json:"action" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Action must be start, stop or restart"})
		return
	}
	action := strings.ToLower(req.Action)
	if action != "start" && action != "stop" && action != "restart" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid daemon action"})
		return
	}
	if os.Getenv("SWANPANEL_EXTERNAL_STRONGSWAN") == "1" {
		// The container has no safe authority to manage the host's systemd unit.
		// Never start a second charon or execute ipsec against the wrong daemon.
		c.JSON(http.StatusConflict, gin.H{"error": "External host strongSwan lifecycle control is unavailable; manage its systemd unit on the host"})
		return
	}
	ctx := c.Request.Context()
	systemd := isSystemdAvailable()
	unit := os.Getenv("STRONGSWAN_SYSTEMD_UNIT")
	if unit == "" {
		unit = "strongswan"
	}
	var cmd *exec.Cmd
	if systemd {
		cmd = exec.CommandContext(ctx, "systemctl", action, unit)
	} else if _, err := exec.LookPath("ipsec"); err == nil {
		cmd = exec.CommandContext(ctx, "ipsec", action)
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No supported daemon control mechanism"})
		return
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("strongSwan control failed", "action", action, "error", err, "output", string(output))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Daemon control command failed"})
		return
	}
	client := vici.GetClient("")
	if action == "stop" {
		// A stopped daemon is expected to make VICI unavailable.
		if err := client.Reconnect(); err == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Daemon still responds after stop"})
			return
		}
		if systemd {
			active := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", unit)
			if active.Run() == nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Systemd still reports daemon active"})
				return
			}
		} else {
			stats, _ := client.GetDaemonStats(ctx)
			if stats != nil && stats.PID > 0 {
				if _, err := os.Stat(fmt.Sprintf("/proc/%d", stats.PID)); err == nil {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Daemon process is still running"})
					return
				}
			}
		}
		realtime.DefaultBroker.Broadcast("gateway_status", gin.H{"status": "inactive", "action": action})
		if auditErr := audit.RecordFromContext(c, audit.EntryInput{
			Action:     "gateway.daemon.controlled",
			Severity:   audit.SeverityWarning,
			TargetType: "gateway",
			TargetID:   "charon",
			Summary:    "strongSwan daemon stopped by administrator",
			Details:    map[string]any{"action": action},
		}); auditErr != nil {
			slog.Warn("Failed to record daemon stop audit event", "action", action, "error", auditErr)
		}
		c.JSON(http.StatusOK, gin.H{"message": "Daemon stopped and verified", "status": "inactive", "action": action})
		return
	}
	// Wait for VICI readiness rather than assuming startup completes in one second.
	deadline := time.Now().Add(15 * time.Second)
	for {
		if err := client.Reconnect(); err == nil && client.CheckHealth(ctx) == nil {
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Daemon command returned but VICI did not become healthy"})
			return
		}
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
	if err := policy.Reconcile(ctx, db.DB); err != nil {
		slog.Error("Daemon running without fully applied policy", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Daemon started, but firewall/policy synchronization failed"})
		return
	}
	runtime, err := client.GetDaemonStats(ctx)
	if err != nil || runtime.Status != "active" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Daemon state verification failed"})
		return
	}
	realtime.DefaultBroker.Broadcast("gateway_status", gin.H{"status": runtime.Status, "action": action})
	if auditErr := audit.RecordFromContext(c, audit.EntryInput{
		Action:     "gateway.daemon.controlled",
		Severity:   audit.SeverityWarning,
		TargetType: "gateway",
		TargetID:   "charon",
		Summary:    fmt.Sprintf("strongSwan daemon %s completed", action),
		Details:    map[string]any{"action": action, "status": runtime.Status},
	}); auditErr != nil {
		slog.Warn("Failed to record daemon control audit event", "action", action, "error", auditErr)
	}
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Daemon %s completed and verified", action),
		"action":  action, "status": runtime.Status, "timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// GetApplyStatus exposes durable data-plane synchronization state.
func (h *SystemHandler) GetApplyStatus(c *gin.Context) {
	state, err := policy.GetApplyState(c.Request.Context(), db.DB)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Runtime apply state unavailable"})
		return
	}
	needed, err := policy.NeedsReconciliation(c.Request.Context(), db.DB)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Cannot inspect pending gateway configuration"})
		return
	}
	status := state.Status
	if needed && status == "applied" {
		status = "pending"
	}
	c.JSON(http.StatusOK, gin.H{
		"status": status, "desired_generation": state.DesiredGeneration,
		"applied_generation": state.AppliedGeneration, "last_error": state.LastError,
		"last_applied_at": state.LastAppliedAt,
	})
}

// RetryApply lets administrators explicitly retry a failed data-plane update.
func (h *SystemHandler) RetryApply(c *gin.Context) {
	if err := policy.ReconcileGateway(c.Request.Context(), db.DB); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Data-plane reconciliation failed"})
		return
	}
	_ = audit.RecordFromContext(c, audit.EntryInput{
		Action:     "gateway.apply.retried",
		Severity:   audit.SeverityInfo,
		TargetType: "gateway",
		TargetID:   "1",
		Summary:    "Gateway configuration reconciliation retried and applied",
	})
	c.JSON(http.StatusOK, gin.H{"runtime_status": "applied"})
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
