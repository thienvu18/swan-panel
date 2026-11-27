package maintenance

import (
	"errors"
	"sync"
	"time"
)

// Mode describes the operational lifecycle mode of the SwanPanel control plane.
type Mode string

const (
	ModeNormal       Mode = "normal"
	ModeMaintenance  Mode = "maintenance"
	ModeFailedClosed Mode = "failed_closed"
)

var (
	// ErrMaintenance is returned when control-plane operations are paused for restore/restart.
	ErrMaintenance = errors.New("system is in maintenance mode for backup restoration")
	// ErrFailedClosed is returned when a prior restore rollback failed and operator recovery is required.
	ErrFailedClosed = errors.New("system is locked in fail-closed recovery mode; inspect restore journal")
	// ErrSnapshotTimeout is returned if in-flight mutations do not yield before deadline.
	ErrSnapshotTimeout = errors.New("timed out waiting for active mutations to finish")
)

// Status describes the current coordinator state for health and HTTP 503 responses.
type Status struct {
	Mode              Mode   `json:"mode"`
	Reason            string `json:"reason,omitempty"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
}

// Coordinator synchronizes normal API traffic, PKI/DB mutations, backup snapshots,
// and exclusive maintenance/restore transitions without holding locks across SSE streams.
type Coordinator struct {
	mu           sync.RWMutex
	mode         Mode
	reason       string
	snapshotMu   sync.RWMutex
	restoreMu    sync.Mutex
	inFlightWG   sync.WaitGroup
	onCloseSSE   func()
	onStopWorker func()
}

// DefaultCoordinator is the process-wide maintenance and snapshot coordinator.
var DefaultCoordinator = NewCoordinator()

// NewCoordinator constructs a fresh Coordinator in ModeNormal.
func NewCoordinator() *Coordinator {
	return &Coordinator{mode: ModeNormal}
}

// ResetForTest returns the coordinator to ModeNormal. Intended solely for unit tests.
func (c *Coordinator) ResetForTest() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mode = ModeNormal
	c.reason = ""
}

// SetLifecycleHooks registers callbacks invoked when entering maintenance mode
// to close active SSE streams and stop background workers.
func (c *Coordinator) SetLifecycleHooks(closeSSE, stopWorkers func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onCloseSSE = closeSSE
	c.onStopWorker = stopWorkers
}

// CurrentStatus returns the active coordinator mode and sanitized reason.
func (c *Coordinator) CurrentStatus() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	retry := 0
	if c.mode == ModeMaintenance {
		retry = 5
	}
	return Status{
		Mode:              c.mode,
		Reason:            c.reason,
		RetryAfterSeconds: retry,
	}
}

// BeginRequest registers an active HTTP request if the control plane is serving normally.
func (c *Coordinator) BeginRequest() (func(), error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	switch c.mode {
	case ModeMaintenance:
		return nil, ErrMaintenance
	case ModeFailedClosed:
		return nil, ErrFailedClosed
	}
	c.inFlightWG.Add(1)
	var once sync.Once
	return func() {
		once.Do(c.inFlightWG.Done)
	}, nil
}

// BeginMutation acquires a shared snapshot read lock so domain writes and PKI mutations
// cannot interleave inconsistently with a backup point-in-time snapshot.
func (c *Coordinator) BeginMutation() func() {
	c.snapshotMu.RLock()
	var once sync.Once
	return func() {
		once.Do(c.snapshotMu.RUnlock)
	}
}

// AcquireSnapshotBarrier exclusively pauses domain mutations while SQLite and PKI state
// are captured together, releasing immediately afterward before compression or encryption.
func (c *Coordinator) AcquireSnapshotBarrier() func() {
	c.snapshotMu.Lock()
	var once sync.Once
	return func() {
		once.Do(c.snapshotMu.Unlock)
	}
}

// TryLockRestore serializes backup restore requests so duplicate concurrent restores fail immediately.
func (c *Coordinator) TryLockRestore() (func(), bool) {
	if !c.restoreMu.TryLock() {
		return nil, false
	}
	var once sync.Once
	return func() {
		once.Do(c.restoreMu.Unlock)
	}, true
}

// EnterMaintenance switches the coordinator into maintenance mode, closes SSE streams,
// stops background workers, and waits up to drainTimeout for in-flight requests to finish.
func (c *Coordinator) EnterMaintenance(reason string, drainTimeout time.Duration) error {
	c.mu.Lock()
	if c.mode == ModeFailedClosed {
		c.mu.Unlock()
		return ErrFailedClosed
	}
	c.mode = ModeMaintenance
	c.reason = reason
	closeSSE := c.onCloseSSE
	stopWorkers := c.onStopWorker
	c.mu.Unlock()

	if closeSSE != nil {
		closeSSE()
	}
	if stopWorkers != nil {
		stopWorkers()
	}

	if drainTimeout <= 0 {
		drainTimeout = 5 * time.Second
	}
	done := make(chan struct{})
	go func() {
		c.inFlightWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(drainTimeout):
		return ErrSnapshotTimeout
	}
}

// ExitMaintenance returns the coordinator to ModeNormal (e.g., after an in-process
// rollback or test-mode restore activation).
func (c *Coordinator) ExitMaintenance() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode == ModeMaintenance {
		c.mode = ModeNormal
		c.reason = ""
	}
}

// EnterFailedClosed locks the control plane in fail-closed recovery mode.
func (c *Coordinator) EnterFailedClosed(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mode = ModeFailedClosed
	c.reason = reason
}
