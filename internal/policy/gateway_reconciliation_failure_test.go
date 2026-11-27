package policy_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"github.com/swan-panel/swan-panel/internal/pki"
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
	t.Run("R04 CRL sync failure leaves gateway state failed, never applied", func(t *testing.T) {
		database := setupPolicyTestDB(t)
		if err := database.AutoMigrate(&db.RuntimeApplyState{}, &db.CertificateRevocation{}); err != nil {
			t.Fatal(err)
		}
		mgr, err := pki.GetPKIManager()
		if err != nil {
			t.Fatal(err)
		}
		crlPath := filepath.Join(mgr.DataDir(), "ca.crl")
		_ = os.RemoveAll(crlPath)
		if err := os.MkdirAll(crlPath, 0700); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.RemoveAll(crlPath) }()

		if err := policy.ReconcileGateway(context.Background(), database); err == nil {
			t.Fatal("expected ReconcileGateway to fail when CRL cannot be written")
		}
		st, err := policy.GetApplyState(context.Background(), database)
		if err != nil || st.Status != "failed" {
			t.Fatalf("R04 regression: expected RuntimeApplyState status=failed after CRL failure, got status=%q err=%v", st.Status, err)
		}
	})
	t.Run("R02 ReconcileGatewayWithinMutation does not deadlock with waiting snapshot barrier", func(t *testing.T) {
		database := setupPolicyTestDB(t)
		if err := database.AutoMigrate(&db.RuntimeApplyState{}, &db.CertificateRevocation{}); err != nil {
			t.Fatal(err)
		}
		coord := maintenance.DefaultCoordinator
		coord.ResetForTest()
		releaseMut := coord.BeginMutation()
		barrierAcquired := make(chan struct{})
		go func() {
			releaseSnap := coord.AcquireSnapshotBarrier()
			close(barrierAcquired)
			releaseSnap()
		}()
		// Give writer goroutine a brief moment to queue on snapshotMu.Lock().
		time.Sleep(20 * time.Millisecond)

		done := make(chan error, 1)
		go func() {
			done <- policy.ReconcileGatewayWithinMutation(context.Background(), database)
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("ReconcileGatewayWithinMutation failed: %v", err)
			}
		case <-time.After(2 * time.Second):
			releaseMut()
			t.Fatal("R02 deadlock: ReconcileGatewayWithinMutation stalled behind waiting AcquireSnapshotBarrier")
		}
		releaseMut()
		select {
		case <-barrierAcquired:
		case <-time.After(2 * time.Second):
			t.Fatal("AcquireSnapshotBarrier did not finish after BeginMutation released")
		}
	})
}
