package handlers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/db"
)

func TestGatewayStatusUsesVerifiedEvidence(t *testing.T) {
	applied := db.RuntimeApplyState{
		Status: "applied", DesiredGeneration: 4,
		AppliedGeneration: 4, DaemonBootID: "boot-a",
	}
	tests := []struct {
		name       string
		runtimeErr error
		bootID     string
		bootErr    error
		state      db.RuntimeApplyState
		applyErr   error
		want       string
	}{
		{"verified online", nil, "boot-a", nil, applied, nil, "online"},
		{"missing socket", fmt.Errorf("dial: %w", os.ErrNotExist), "", nil, applied, nil, "offline"},
		{"refused socket", syscall.ECONNREFUSED, "", nil, applied, nil, "offline"},
		{"timeout is not offline", context.DeadlineExceeded, "", nil, applied, nil, "unknown"},
		{"ambiguous failure", errors.New("vici response error"), "", nil, applied, nil, "unknown"},
		{"boot identity cannot be read", nil, "", errors.New("stat failed"), applied, nil, "unknown"},
		{"apply state cannot be read", nil, "boot-a", nil, applied, errors.New("database unavailable"), "degraded"},
		{"daemon restarted", nil, "boot-b", nil, applied, nil, "degraded"},
		{"boot identity absent", nil, "", nil, applied, nil, "degraded"},
		{"different revision", nil, "boot-a", nil, db.RuntimeApplyState{
			Status: "applied", DesiredGeneration: 5, AppliedGeneration: 4, DaemonBootID: "boot-a",
		}, nil, "degraded"},
		{"pending apply", nil, "boot-a", nil, db.RuntimeApplyState{
			Status: "pending", DesiredGeneration: 4, AppliedGeneration: 4, DaemonBootID: "boot-a",
		}, nil, "degraded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gatewayStatus(tt.runtimeErr, tt.bootID, tt.bootErr, tt.state, tt.applyErr)
			if got != tt.want {
				t.Fatalf("gatewayStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
func TestGatewayMonitorSnapshotExpires(t *testing.T) {
	now := time.Now().UTC()
	m := &GatewayMonitor{snapshot: GatewayHealth{Status: "online", CheckedAt: now.Add(-5 * time.Second)}}
	if got := m.Snapshot().Status; got != "online" {
		t.Fatalf("fresh snapshot = %q, want online", got)
	}
	m.snapshot.CheckedAt = now.Add(-gatewayHealthMaxAge - time.Second)
	if got := m.Snapshot().Status; got != "unknown" {
		t.Fatalf("expired snapshot = %q, want unknown", got)
	}
	m.snapshot.CheckedAt = time.Time{}
	if got := m.Snapshot().Status; got != "unknown" {
		t.Fatalf("empty snapshot = %q, want unknown", got)
	}
}
