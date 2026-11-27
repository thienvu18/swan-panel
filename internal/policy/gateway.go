package policy

import (
	"context"
	"errors"
	"fmt"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"github.com/swan-panel/swan-panel/internal/pki"
	"gorm.io/gorm"
)

// ErrGatewayCAUnavailable indicates that the gateway identity root cannot be initialized.
var ErrGatewayCAUnavailable = errors.New("initialize gateway CA")

// ReconcileGateway also restores credentials and the signed CRL, not only
// connections and firewall. This supports failed CA rotations and daemon
// restarts via both the manual retry endpoint and the background worker.
func ReconcileGateway(ctx context.Context, database *gorm.DB) error {
	releaseMut := maintenance.DefaultCoordinator.BeginMutation()
	defer releaseMut()
	return ReconcileGatewayWithinMutation(ctx, database)
}

// ReconcileGatewayWithinMutation performs full gateway reconciliation when the caller
// already holds maintenance.DefaultCoordinator.BeginMutation(), avoiding recursive RWMutex RLock deadlocks.
func ReconcileGatewayWithinMutation(ctx context.Context, database *gorm.DB) (retErr error) {
	defer func() {
		if retErr == nil {
			return
		}
		state, err := GetApplyState(context.Background(), database)
		if err != nil {
			return
		}
		state.Status = "failed"
		state.LastError = retErr.Error()
		_ = database.WithContext(context.Background()).Save(&state).Error
	}()
	manager, err := pki.GetPKIManager()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrGatewayCAUnavailable, err)
	}
	var settings db.StrongswanSettings
	if err := database.WithContext(ctx).First(&settings, 1).Error; err != nil {
		return fmt.Errorf("read gateway settings: %w", err)
	}
	if err := manager.EnsureServerCertificates(settings.ServerIdentity, []string{settings.ServerEndpoint}); err != nil {
		return fmt.Errorf("deploy gateway identity: %w", err)
	}
	var revocations []db.CertificateRevocation
	if err := database.WithContext(ctx).Find(&revocations).Error; err != nil {
		return err
	}
	entries := make([]pki.RevokedCertificate, 0, len(revocations))
	for _, rev := range revocations {
		entries = append(entries, pki.RevokedCertificate{Serial: rev.Serial, RevokedAt: rev.RevokedAt})
	}
	if err := manager.PublishCRL(ctx, entries); err != nil {
		return fmt.Errorf("publish signed revocation list: %w", err)
	}
	return Reconcile(ctx, database)
}
