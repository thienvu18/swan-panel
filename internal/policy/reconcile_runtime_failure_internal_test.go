package policy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func runtimeFailureFixture(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "runtime.db")),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&db.User{}, &db.UserGroup{}, &db.ResourceGroup{},
		&db.Device{}, &db.AccessRule{}, &db.StrongswanSettings{}); err != nil {
		t.Fatal(err)
	}
	settings := db.StrongswanSettings{ID: 1, ServerIdentity: "vpn.example.test",
		ServerEndpoint: "vpn.example.test", DPDDelay: "30s", DPDAction: "clear",
		Fragmentation: "yes", IKEProposals: "aes256gcm16", ESPProposals: "aes256gcm16"}
	if err := database.Create(&settings).Error; err != nil {
		t.Fatal(err)
	}
	group := db.ResourceGroup{ID: "runtime-failure-network", Name: "Runtime failure network", CIDRBlock: "10.201.0.0/24"}
	if err := database.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	device := db.Device{ID: "runtime-failure-device", Hostname: "device.example.test", DeviceType: "company",
		ResourceGroupID: group.ID, VirtualIP: "10.201.0.2", IPAllocationType: "static", Platform: "linux"}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	return database
}

func TestRuntimeReconciliationRejectsUnavailableModelsAndUnsafePersistedIdentities(t *testing.T) {
	t.Run("device table unavailable", func(t *testing.T) {
		t.Setenv("APP_ENV", "test")
		database := runtimeFailureFixture(t)
		if err := database.Migrator().DropTable(&db.Device{}); err != nil {
			t.Fatal(err)
		}
		if err := reconcileRuntime(context.Background(), database); err == nil {
			t.Fatal("missing device inventory accepted")
		}
	})
	t.Run("gateway settings unavailable", func(t *testing.T) {
		t.Setenv("APP_ENV", "test")
		database := runtimeFailureFixture(t)
		if err := database.Migrator().DropTable(&db.StrongswanSettings{}); err != nil {
			t.Fatal(err)
		}
		if err := reconcileRuntime(context.Background(), database); err == nil {
			t.Fatal("missing gateway settings accepted")
		}
	})
	t.Run("device policy cannot be compiled", func(t *testing.T) {
		t.Setenv("APP_ENV", "test")
		database := runtimeFailureFixture(t)
		if err := database.Migrator().DropTable(&db.AccessRule{}); err != nil {
			t.Fatal(err)
		}
		if err := reconcileRuntime(context.Background(), database); err == nil {
			t.Fatal("missing policy ledger accepted")
		}
	})
	t.Run("injected remote identity is rejected when rendering", func(t *testing.T) {
		t.Setenv("APP_ENV", "test")
		database := runtimeFailureFixture(t)
		if err := database.Model(&db.Device{}).Where("id = ?", "runtime-failure-device").
			Update("hostname", "device\\nmalicious").Error; err != nil {
			t.Fatal(err)
		}
		if err := reconcileRuntime(context.Background(), database); err == nil ||
			!errors.Is(err, ErrGatewayConfigurationRenderFailed) {
			t.Fatalf("unsafe config not rejected: %v", err)
		}
	})
	t.Run("managed configuration destination is a directory", func(t *testing.T) {
		t.Setenv("APP_ENV", "ci-integration")
		t.Setenv("SWANPANEL_SWANCTL_FILE", t.TempDir())
		database := runtimeFailureFixture(t)
		if err := reconcileRuntime(context.Background(), database); err == nil ||
			!errors.Is(err, ErrGatewayConfigurationPersistFailed) {
			t.Fatalf("invalid persistent configuration path accepted: %v", err)
		}
	})
	t.Run("nftables failure with fail-closed barrier failure", func(t *testing.T) {
		t.Setenv("APP_ENV", "ci-integration")
		t.Setenv("SWANPANEL_FIREWALL_DRY_RUN", "1")
		t.Setenv("SWANPANEL_SWANCTL_FILE", filepath.Join(t.TempDir(), "managed.conf"))
		database := runtimeFailureFixture(t)
		if err := reconcileRuntime(context.Background(), database); err == nil ||
			!errors.Is(err, ErrFirewallFailClosedFallbackFailed) {
			t.Fatalf("failed firewall barrier not reported: %v", err)
		}
	})
	t.Run("fail-closed barrier succeeds after primary nftables failure", func(t *testing.T) {
		t.Setenv("APP_ENV", "ci-integration")
		t.Setenv("SWANPANEL_FIREWALL_DRY_RUN", "")
		t.Setenv("SWANPANEL_SWANCTL_FILE", filepath.Join(t.TempDir(), "managed.conf"))
		bin := t.TempDir()
		counter := filepath.Join(t.TempDir(), "counter")
		t.Setenv("SIMULATED_NFT_COUNTER", counter)
		script := "#!/bin/sh\nif [ \"$1\" = \"list\" ]; then exit 1; fi\n" +
			"if [ ! -e \"$SIMULATED_NFT_COUNTER\" ]; then touch \"$SIMULATED_NFT_COUNTER\"; exit 4; fi\nexit 0\n"
		if err := os.WriteFile(filepath.Join(bin, "nft"), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
		database := runtimeFailureFixture(t)
		if err := reconcileRuntime(context.Background(), database); err == nil ||
			!errors.Is(err, ErrFirewallFailClosedBarrierActive) {
			t.Fatalf("firewall fallback result not preserved: %v", err)
		}
	})
}

func TestReconcileCannotClaimSuccessWithoutDurableStateAndDesiredDatabase(t *testing.T) {
	t.Run("runtime apply state table unavailable", func(t *testing.T) {
		t.Setenv("APP_ENV", "test")
		database := runtimeFailureFixture(t)
		if err := Reconcile(context.Background(), database); err == nil ||
			!errors.Is(err, ErrRuntimeApplyStateUnavailable) {
			t.Fatalf("missing state ledger allowed reconciliation: %v", err)
		}
	})
	t.Run("desired configuration cannot be read", func(t *testing.T) {
		t.Setenv("APP_ENV", "test")
		database := runtimeFailureFixture(t)
		if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
			t.Fatal(err)
		}
		if err := database.Migrator().DropTable(&db.AccessRule{}); err != nil {
			t.Fatal(err)
		}
		if err := Reconcile(context.Background(), database); err == nil ||
			!errors.Is(err, ErrDesiredConfigurationUnavailable) {
			t.Fatalf("unreadable desired state was accepted: %v", err)
		}
	})
	t.Run("transition to applying cannot be stored", func(t *testing.T) {
		t.Setenv("APP_ENV", "test")
		database := runtimeFailureFixture(t)
		if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
			t.Fatal(err)
		}
		if _, err := GetApplyState(context.Background(), database); err != nil {
			t.Fatal(err)
		}
		if err := database.Exec("CREATE TRIGGER deny_applying BEFORE UPDATE ON runtime_apply_states " +
			"WHEN NEW.status = 'applying' BEGIN SELECT RAISE(FAIL, 'no update'); END").Error; err != nil {
			t.Fatal(err)
		}
		if err := Reconcile(context.Background(), database); err == nil ||
			!errors.Is(err, ErrRuntimeApplyStatePersistenceFailed) {
			t.Fatalf("state transition error ignored: %v", err)
		}
	})
	t.Run("final applied state cannot be persisted", func(t *testing.T) {
		t.Setenv("APP_ENV", "test")
		database := runtimeFailureFixture(t)
		if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
			t.Fatal(err)
		}
		if _, err := GetApplyState(context.Background(), database); err != nil {
			t.Fatal(err)
		}
		if err := database.Exec("CREATE TRIGGER deny_final_apply BEFORE UPDATE ON runtime_apply_states " +
			"WHEN NEW.status = 'applied' BEGIN SELECT RAISE(FAIL, 'no durable commit'); END").Error; err != nil {
			t.Fatal(err)
		}
		if err := Reconcile(context.Background(), database); err == nil ||
			!errors.Is(err, ErrRuntimeApplyStatePersistenceFailed) {
			t.Fatalf("missing durable commit accepted: %v", err)
		}
	})
}
