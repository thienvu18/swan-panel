package vici_test

import (
	"context"
	"testing"

	"github.com/swan-panel/swan-panel/internal/vici"
)

func TestVICIClientFailClosedWithoutSocket(t *testing.T) {
	client := vici.NewClient("/tmp/swanpanel-test-charon-nonexistent.vici")
	ctx := context.Background()
	if _, err := client.ListSAs(ctx); err == nil {
		t.Fatal("Missing VICI socket must not return simulated sessions")
	}
	if err := client.TerminateSA(ctx, "unavailable"); err == nil {
		t.Fatal("Cannot claim a session was terminated without VICI")
	}
	if err := client.LoadConnection(ctx, "test-conn", map[string]any{"version": "2"}); err == nil {
		t.Fatal("Cannot claim a connection was applied without VICI")
	}
	if err := client.LoadPool(ctx, "test-pool", "10.10.10.0/24"); err == nil {
		t.Fatal("Cannot claim a pool was installed without VICI")
	}
	if client.IsConnected() {
		t.Fatal("Disconnected client reported connected")
	}
}
