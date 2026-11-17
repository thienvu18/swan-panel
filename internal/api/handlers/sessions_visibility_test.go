package handlers

import (
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
	"github.com/swan-panel/swan-panel/internal/vici"
)

func TestOrdinaryUserSessionListRequiresMatchingConnectionPeerAndVIP(t *testing.T) {
	devices := []db.Device{
		{ID: "device-owned", Hostname: "owned.example", VirtualIP: "10.1.0.2"},
		{ID: "device-revoked", Hostname: "revoked.example", VirtualIP: "10.1.0.3", IsRevoked: true},
	}
	sessions := []vici.SessionInfo{
		{IKEName: policy.ConnectionName("device-owned"), PeerIdentity: "owned.example", VirtualIP: "10.1.0.2"},
		{IKEName: policy.ConnectionName("other"), PeerIdentity: "owned.example", VirtualIP: "10.1.0.4"},
		{IKEName: policy.ConnectionName("device-owned"), PeerIdentity: "another.example", VirtualIP: "10.1.0.2"},
		{IKEName: policy.ConnectionName("device-owned"), PeerIdentity: "owned.example", VirtualIP: "10.1.0.9"},
		{IKEName: policy.ConnectionName("device-revoked"), PeerIdentity: "revoked.example", VirtualIP: "10.1.0.3"},
	}
	visible := visibleSessions(sessions, devices)
	if len(visible) != 1 || visible[0] != sessions[0] {
		t.Fatalf("unowned or misidentified session exposed: %+v", visible)
	}
}
