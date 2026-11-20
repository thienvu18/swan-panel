package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
	"github.com/swan-panel/swan-panel/internal/vici"
)

const gatewayHealthInterval = 15 * time.Second
const gatewayHealthMaxAge = 45 * time.Second

type GatewayHealth struct {
	Status    string    `json:"status"`
	CheckedAt time.Time `json:"checked_at"`
}

// GatewayMonitor is the sole source of gateway health for the control plane.
// The backend probes on a fixed interval; browsers receive pushed snapshots.
type GatewayMonitor struct {
	mu       sync.RWMutex
	probeMu  sync.Mutex
	snapshot GatewayHealth
}

var DefaultGatewayMonitor = &GatewayMonitor{
	snapshot: GatewayHealth{Status: "unknown"},
}

func (m *GatewayMonitor) Snapshot() GatewayHealth {
	m.mu.RLock()
	current := m.snapshot
	m.mu.RUnlock()
	if current.CheckedAt.IsZero() || time.Since(current.CheckedAt) > gatewayHealthMaxAge {
		current.Status = "unknown"
	}
	return current
}

func gatewayStatus(runtimeErr error, bootID string, bootErr error, state db.RuntimeApplyState, applyErr error) string {
	if runtimeErr != nil {
		// A missing/refused local socket is evidence that the VPN service is down.
		// Timeouts and transport errors alone cannot establish that.
		if errors.Is(runtimeErr, os.ErrNotExist) || errors.Is(runtimeErr, syscall.ECONNREFUSED) {
			return "offline"
		}
		return "unknown"
	}
	if bootErr != nil {
		return "unknown"
	}
	if applyErr != nil || state.Status != "applied" ||
		state.DesiredGeneration != state.AppliedGeneration ||
		state.DaemonBootID == "" || state.DaemonBootID != bootID {
		// A restarted daemon can answer health probes before its saved VPN
		// configuration has been restored. Never report it as fully online.
		return "degraded"
	}
	return "online"
}

func (m *GatewayMonitor) refresh(ctx context.Context) GatewayHealth {
	m.probeMu.Lock()
	defer m.probeMu.Unlock()
	// Multiple tabs reconnecting together share a recent probe.
	m.mu.RLock()
	recent := m.snapshot
	m.mu.RUnlock()
	if !recent.CheckedAt.IsZero() && time.Since(recent.CheckedAt) < 2*time.Second {
		return recent
	}
	checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	client := vici.GetClient("")
	runtimeErr := client.CheckHealth(checkCtx)
	var state db.RuntimeApplyState
	var applyErr, bootErr error
	var bootID string
	if runtimeErr == nil {
		bootID, bootErr = client.DaemonBootID(checkCtx)
		state, applyErr = policy.GetApplyState(checkCtx, db.DB)
	}
	status := gatewayStatus(runtimeErr, bootID, bootErr, state, applyErr)
	snapshot := GatewayHealth{Status: status, CheckedAt: time.Now().UTC()}
	m.mu.Lock()
	previous := m.snapshot.Status
	m.snapshot = snapshot
	m.mu.Unlock()
	if status != previous {
		slog.Info("Gateway health changed", "from", previous, "to", status)
	} else {
		slog.Debug("Gateway health checked", "status", status)
	}
	if runtimeErr != nil {
		slog.Warn("Gateway runtime health probe unavailable", "status", status, "error", runtimeErr)
	} else if bootErr != nil {
		slog.Warn("Gateway daemon identity verification unavailable", "error", bootErr)
	} else if applyErr != nil {
		slog.Warn("Gateway apply state health probe unavailable", "error", applyErr)
	}
	return snapshot
}

// Run emits a small health snapshot every 15 seconds so SSE clients stay fresh.
// It is started once from main after gateway startup and stops with the server.
func (m *GatewayMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(gatewayHealthInterval)
	defer ticker.Stop()
	publish := func() {
		snapshot := m.refresh(ctx)
		realtime.DefaultBroker.Broadcast("gateway_health", snapshot)
	}
	publish()
	for {
		select {
		case <-ctx.Done():
			slog.Debug("Gateway health monitor stopped")
			return
		case <-ticker.C:
			publish()
		}
	}
}

func (h *DashboardHandler) GetGatewayHealth(c *gin.Context) {
	// Initial load and SSE reconnection explicitly ask for fresh evidence,
	// while normal operation remains a backend-driven 15-second push.
	snapshot := DefaultGatewayMonitor.refresh(c.Request.Context())
	realtime.DefaultBroker.Broadcast("gateway_health", snapshot)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"health": snapshot})
}
