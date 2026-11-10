package vici_test

import (
	"context"
	"testing"

	"github.com/swan-panel/swan-panel/internal/vici"
)

func TestVICIClient(t *testing.T) {
	client := vici.NewClient("/tmp/test-charon-not-exist.vici")
	ctx := context.Background()

	// 1. Test listing sessions
	sessions, err := client.ListSAs(ctx)
	if err != nil {
		t.Fatalf("Failed to list SAs: %v", err)
	}

	if len(sessions) == 0 {
		t.Fatalf("Expected baseline sessions, got 0")
	}

	initialCount := len(sessions)
	targetName := sessions[0].IKEName

	// 2. Test terminating session
	if err := client.TerminateSA(ctx, targetName); err != nil {
		t.Fatalf("Failed to terminate SA: %v", err)
	}

	updatedSessions, err := client.ListSAs(ctx)
	if err != nil {
		t.Fatalf("Failed to list updated SAs: %v", err)
	}

	if len(updatedSessions) != initialCount-1 {
		t.Errorf("Expected %d sessions after termination, got %d", initialCount-1, len(updatedSessions))
	}

	// 3. Test dynamic load-conn
	params := map[string]interface{}{
		"version": "2",
		"mobike":  "yes",
	}
	if err := client.LoadConnection(ctx, "test-conn", params); err != nil {
		t.Fatalf("Failed to load connection: %v", err)
	}
}
