package maintenance

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCoordinatorMaintenanceAndSnapshotBarrier(t *testing.T) {
	coord := NewCoordinator()

	var sseClosed, workersStopped atomic.Bool
	coord.SetLifecycleHooks(
		func() { sseClosed.Store(true) },
		func() { workersStopped.Store(true) },
	)

	releaseReq, err := coord.BeginRequest()
	if err != nil {
		t.Fatalf("expected normal mode to accept request: %v", err)
	}

	releaseMut := coord.BeginMutation()
	barrierAcquired := make(chan struct{})
	go func() {
		releaseSnap := coord.AcquireSnapshotBarrier()
		close(barrierAcquired)
		releaseSnap()
	}()

	select {
	case <-barrierAcquired:
		t.Fatal("snapshot barrier should wait while mutation lock is held")
	case <-time.After(30 * time.Millisecond):
	}
	releaseMut()
	select {
	case <-barrierAcquired:
	case <-time.After(time.Second):
		t.Fatal("snapshot barrier did not acquire after mutation released")
	}

	drainDone := make(chan error, 1)
	go func() {
		drainDone <- coord.EnterMaintenance("restore in progress", time.Second)
	}()

	time.Sleep(20 * time.Millisecond)
	if _, err := coord.BeginRequest(); !errors.Is(err, ErrMaintenance) {
		t.Fatalf("expected ErrMaintenance during drain, got %v", err)
	}
	releaseReq()

	if err := <-drainDone; err != nil {
		t.Fatalf("expected clean drain, got %v", err)
	}
	if !sseClosed.Load() || !workersStopped.Load() {
		t.Fatal("expected SSE and worker stop hooks to fire when entering maintenance")
	}

	coord.ExitMaintenance()
	releaseAfter, err := coord.BeginRequest()
	if err != nil {
		t.Fatalf("expected ExitMaintenance to restore normal mode: %v", err)
	}
	releaseAfter()

	coord.EnterFailedClosed("rollback failed")
	if _, err := coord.BeginRequest(); !errors.Is(err, ErrFailedClosed) {
		t.Fatalf("expected ErrFailedClosed, got %v", err)
	}
}
