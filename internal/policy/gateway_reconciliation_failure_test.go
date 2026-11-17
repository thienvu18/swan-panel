package policy_test

import (
	"context"
	"os"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
)

// The global production PKI manager is initialized only once. Keep its
// test directory for the entire package lifetime, including shuffled tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "swanpanel-policy-pki-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("PKI_DATA_DIR", dir)
	_ = os.Setenv("APP_ENV", "test")
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestGatewayReconciliationPersistsFailuresForUnusableGatewayState(t *testing.T) {
	t.Run("missing runtime apply state table", func(t *testing.T) {
		database := setupPolicyTestDB(t)
		if err := database.Migrator().DropTable(&db.RuntimeApplyState{}); err != nil {
			t.Fatal(err)
		}
		if err := policy.ReconcileGateway(context.Background(), database); err == nil {
			t.Fatal("reconciliation without an apply state ledger succeeded")
		}
	})
	t.Run("missing gateway settings", func(t *testing.T) {
		database := setupPolicyTestDB(t)
		if err := database.AutoMigrate(&db.RuntimeApplyState{}, &db.CertificateRevocation{}); err != nil {
			t.Fatal(err)
		}
		if err := database.Where("id = ?", 1).Delete(&db.StrongswanSettings{}).Error; err != nil {
			t.Fatal(err)
		}
		if err := policy.ReconcileGateway(context.Background(), database); err == nil {
			t.Fatal("missing settings accepted")
		}
		state, err := policy.GetApplyState(context.Background(), database)
		if err != nil || state.Status != "failed" || state.LastError == "" {
			t.Fatalf("failure not persisted: %+v %v", state, err)
		}
	})
	t.Run("missing revocation ledger", func(t *testing.T) {
		database := setupPolicyTestDB(t)
		if err := database.AutoMigrate(&db.RuntimeApplyState{}, &db.CertificateRevocation{}); err != nil {
			t.Fatal(err)
		}
		if err := database.Migrator().DropTable(&db.CertificateRevocation{}); err != nil {
			t.Fatal(err)
		}
		if err := policy.ReconcileGateway(context.Background(), database); err == nil {
			t.Fatal("missing revocation ledger ignored")
		}
	})
	t.Run("malformed certificate revocation entry", func(t *testing.T) {
		database := setupPolicyTestDB(t)
		if err := database.AutoMigrate(&db.RuntimeApplyState{}, &db.CertificateRevocation{}); err != nil {
			t.Fatal(err)
		}
		if err := database.Create(&db.CertificateRevocation{Serial: "not-hex", DeviceID: "obsolete"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := policy.ReconcileGateway(context.Background(), database); err == nil {
			t.Fatal("corrupted ledger marked applied")
		}
	})
	t.Run("server identity unavailable", func(t *testing.T) {
		database := setupPolicyTestDB(t)
		if err := database.AutoMigrate(&db.RuntimeApplyState{}, &db.CertificateRevocation{}); err != nil {
			t.Fatal(err)
		}
		if err := database.Model(&db.StrongswanSettings{}).Where("id = ?", 1).Update("server_identity", "").Error; err != nil {
			t.Fatal(err)
		}
		if err := policy.ReconcileGateway(context.Background(), database); err == nil {
			t.Fatal("missing server identity accepted")
		}
	})
	t.Run("invalid active VIP blocks reconciliation", func(t *testing.T) {
		database := setupPolicyTestDB(t)
		if err := database.AutoMigrate(&db.RuntimeApplyState{}, &db.CertificateRevocation{}); err != nil {
			t.Fatal(err)
		}
		if err := database.Model(&db.Device{}).Where("id = ?", "dev_thienvu").Update("virtual_ip", "invalid").Error; err != nil {
			t.Fatal(err)
		}
		if err := policy.ReconcileGateway(context.Background(), database); err == nil {
			t.Fatal("invalid VIP marked applied")
		}
	})
}
