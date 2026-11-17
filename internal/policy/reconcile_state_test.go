package policy_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
)

func TestReconcilePersistsGenerationAndDetectsChangedDesiredState(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	database := setupPolicyTestDB(t)
	if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	wanted, err := policy.NeedsReconciliation(ctx, database)
	if err != nil || !wanted {
		t.Fatalf("new gateway must need reconciliation: %v %v", wanted, err)
	}
	if err := policy.Reconcile(ctx, database); err != nil {
		t.Fatal(err)
	}
	wanted, err = policy.NeedsReconciliation(ctx, database)
	if err != nil || wanted {
		t.Fatalf("reconciled desired state still pending: %v %v", wanted, err)
	}
	initial, err := policy.GetApplyState(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	if initial.DesiredGeneration == 0 || initial.AppliedDigest == "" {
		t.Fatal("applied generation lacks a durable digest")
	}
	if err := database.Model(&db.User{}).Where("id = ?", "usr_thienvu").Update("status", "SUSPENDED").Error; err != nil {
		t.Fatal(err)
	}
	wanted, err = policy.NeedsReconciliation(ctx, database)
	if err != nil || !wanted {
		t.Fatalf("revoked access was not detected: %v %v", wanted, err)
	}
	if err := policy.Reconcile(ctx, database); err != nil {
		t.Fatal(err)
	}
	updated, err := policy.GetApplyState(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AppliedGeneration <= initial.AppliedGeneration || updated.DesiredDigest == initial.DesiredDigest ||
		updated.Status != "applied" {
		t.Fatalf("apply state did not advance: %+v %+v", initial, updated)
	}
}

func TestFirewallMSSClampingOnlyAffectsIPsecForwarding(t *testing.T) {
	database := setupPolicyTestDB(t)
	script, err := policy.NewFirewall(database).Script(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "meta ipsec exists tcp flags syn tcp option maxseg size set 1360") {
		t.Fatal("MSS rule missing or not scoped to VPN forwarding")
	}
	if strings.Contains(script, "tcp dport 853 reject") {
		t.Fatal("strict Android Private DNS must not be forcibly rejected without an explicit access policy")
	}
}

// Updating display metadata and device health must not rebuild the live
// firewall; that would interrupt established tunnels for no policy change.
func TestRuntimeDigestIgnoresDeviceMonitoringAndDisplayOnlyChanges(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	database := setupPolicyTestDB(t)
	if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := policy.Reconcile(ctx, database); err != nil {
		t.Fatal(err)
	}

	seen := time.Now().UTC()
	if err := database.Model(&db.Device{}).Where("id = ?", "dev_thienvu").
		Update("last_seen_at", seen).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&db.ResourceGroup{}).Where("id = ?", "rg_eng").
		Update("name", "New user-facing subnet name").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&db.StrongswanSettings{}).Where("id = ?", 1).
		Update("vpn_name", "Company VPN").Error; err != nil {
		t.Fatal(err)
	}

	needed, err := policy.NeedsReconciliation(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	if needed {
		t.Fatal("cosmetic and telemetry updates caused an unnecessary firewall apply")
	}

	if err := database.Model(&db.StrongswanSettings{}).Where("id = ?", 1).
		Update("dpd_action", "restart").Error; err != nil {
		t.Fatal(err)
	}
	needed, err = policy.NeedsReconciliation(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	if !needed {
		t.Fatal("changing a live CHILD_SA option did not trigger reconcile")
	}
}

// A failed kernel/firewall apply must stay durable and cannot be reported
// as applied even when the write to the database already succeeded.
func TestRuntimeApplyPersistsFailuresAndRecoversOnlyAfterSuccessfulRetry(t *testing.T) {
	database := setupPolicyTestDB(t)
	if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	t.Setenv("APP_ENV", "ci-integration")
	t.Setenv("SWANPANEL_FIREWALL_DRY_RUN", "1")
	failure := policy.Reconcile(ctx, database)
	if failure == nil {
		t.Fatal("firewall dry-run incorrectly marked policy applied")
	}
	state, err := policy.GetApplyState(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "failed" || state.AppliedGeneration != 0 || state.LastError == "" {
		t.Fatalf("failed apply not persisted correctly: %+v", state)
	}
	needed, err := policy.NeedsReconciliation(ctx, database)
	if err != nil || !needed {
		t.Fatalf("failed apply was not marked for retry: needed=%v err=%v", needed, err)
	}
	firstGeneration := state.DesiredGeneration
	// Retry of identical desired state must reuse the same generation.
	if err := policy.Reconcile(ctx, database); err == nil {
		t.Fatal("dry-run failure was swallowed on retry")
	}
	state, err = policy.GetApplyState(ctx, database)
	if err != nil || state.DesiredGeneration != firstGeneration {
		t.Fatalf("retry changed generation: %+v %v", state, err)
	}
	t.Setenv("APP_ENV", "test")
	t.Setenv("SWANPANEL_FIREWALL_DRY_RUN", "")
	if err := policy.Reconcile(ctx, database); err != nil {
		t.Fatal(err)
	}
	state, err = policy.GetApplyState(ctx, database)
	if err != nil || state.Status != "applied" || state.AppliedGeneration != firstGeneration ||
		state.AppliedDigest != state.DesiredDigest || state.LastAppliedAt == nil {
		t.Fatalf("successful recovery not recorded: %+v %v", state, err)
	}
}

func TestRuntimePolicyRejectsInvalidDeviceVIPWithoutUpdatingAppliedDigest(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	database := setupPolicyTestDB(t)
	if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := policy.Reconcile(ctx, database); err != nil {
		t.Fatal(err)
	}
	initial, err := policy.GetApplyState(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&db.Device{}).Where("id = ?", "dev_thienvu").Update("virtual_ip", "invalid-vip").Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "ci-integration")
	t.Setenv("SWANPANEL_FIREWALL_DRY_RUN", "1")
	if err := policy.Reconcile(ctx, database); err == nil {
		t.Fatal("invalid VPN address accepted for data plane")
	}
	state, err := policy.GetApplyState(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "failed" || state.AppliedDigest != initial.AppliedDigest {
		t.Fatalf("invalid VIP marked applied: %+v", state)
	}
}
