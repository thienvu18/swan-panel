package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"github.com/swan-panel/swan-panel/internal/mfa"
	"github.com/swan-panel/swan-panel/internal/pki"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	restoreWorkDirName         = ".swanpanel-restore-work"
	journalFileName            = "journal.json"
	commitMarkerFileName       = "committed.done"
	restoreStatusFile          = ".swanpanel-restore-status.json"
	JournalStatePrepared       = "prepared"
	JournalStateApplying       = "applying"
	JournalStateCommitted      = "committed"
	JournalStateRolledBack     = "rolled_back"
	JournalStateRollbackFailed = "rollback_failed"

	OutcomeNone         = "none"
	OutcomeApplied      = "applied"
	OutcomeRolledBack   = "rolled_back"
	OutcomeFailedClosed = "failed_closed"
)

var (
	// ErrRollbackFailed is returned when both restore activation and rollback fail, locking fail-closed.
	ErrRollbackFailed = errors.New("backup restore and automatic rollback both failed; manual operator recovery required")
	// ErrRestoreJournalConflict is returned when an unresolved fail-closed journal already exists.
	ErrRestoreJournalConflict = errors.New("unresolved restore recovery state present; inspect restore journal before continuing")
	// ErrRestoredRolledBack is returned when restore activation fails and prior state is verified restored.
	ErrRestoredRolledBack = errors.New("restore failed and rolled back cleanly to prior state")
	// ErrRecoveryFailed is returned when startup restore recovery fails in an unknown or corrupt state (fail-closed).
	ErrRecoveryFailed = errors.New("startup restore recovery failed; failing closed")
)

// RestoreStatus stores a non-secret, correlated outcome record of the last restore attempt.
type RestoreStatus struct {
	RestoreID       string `json:"restore_id,omitempty"`
	Outcome         string `json:"outcome"`
	BackupCreatedAt string `json:"backup_created_at,omitempty"`
	Message         string `json:"message,omitempty"`
	CompletedAt     string `json:"completed_at"`
}

// RestoreJournal tracks crash-safe restore transactions on persistent storage across mount points.
type RestoreJournal struct {
	RestoreID         string   `json:"restore_id,omitempty"`
	State             string   `json:"state"`
	DBPath            string   `json:"db_path"`
	PKIDir            string   `json:"pki_dir"`
	MFAKeyPath        string   `json:"mfa_key_path"`
	DBWorkDir         string   `json:"db_work_dir"`
	PKIWorkDir        string   `json:"pki_work_dir"`
	StagedDBPath      string   `json:"staged_db_path"`
	StagedPKIDir      string   `json:"staged_pki_dir"`
	StagedMFAKeyPath  string   `json:"staged_mfa_key_path,omitempty"`
	RollbackCaptured  bool     `json:"rollback_captured"`
	RollbackDBPath    string   `json:"rollback_db_path"`
	RollbackPKIDir    string   `json:"rollback_pki_dir"`
	RollbackMFAKey    string   `json:"rollback_mfa_key_path,omitempty"`
	HasRollbackDB     bool     `json:"has_rollback_db"`
	HadExistingMFAKey bool     `json:"had_existing_mfa_key"`
	ActorID           string   `json:"actor_id,omitempty"`
	ActorEmail        string   `json:"actor_email,omitempty"`
	ActorRole         string   `json:"actor_role,omitempty"`
	SourceIP          string   `json:"source_ip,omitempty"`
	Manifest          Manifest `json:"manifest"`
	LastError         string   `json:"last_error,omitempty"`
	UpdatedAt         string   `json:"updated_at"`
}

// RestoreFaultHooks provides deterministic failure/crash injection points for automated tests.
type RestoreFaultHooks struct {
	OnBeforeSnapshotRollback func() error
	OnBeforeReplacePKI       func() error
	OnBeforeReplaceDB        func() error
	OnBeforeReopenDB         func() error
	OnBeforePKIReload        func() error
	OnBeforeCommitJournal    func() error
	OnBeforeWriteStatus      func(status RestoreStatus) error
	OnBeforeSyncDir          func(dir string) error
	OnRollbackPKI            func() error
	OnRollbackDB             func() error
}

// DBWorkDir returns the persistent restore staging/rollback directory on the database mount.
func DBWorkDir(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), restoreWorkDirName)
}

// PKIWorkDir returns the persistent restore staging/rollback directory on the PKI mount.
func PKIWorkDir(pkiDir string) string {
	return filepath.Join(pkiDir, restoreWorkDirName)
}

func journalPathForDB(dbPath string) string {
	return filepath.Join(DBWorkDir(dbPath), journalFileName)
}

func commitMarkerPathForDB(dbPath string) string {
	return filepath.Join(DBWorkDir(dbPath), commitMarkerFileName)
}

type commitMarker struct {
	RestoreID   string   `json:"restore_id,omitempty"`
	CommittedAt string   `json:"committed_at"`
	Manifest    Manifest `json:"manifest"`
}

func hasValidCommitMarker(dbPath string) (*commitMarker, bool) {
	raw, err := os.ReadFile(commitMarkerPathForDB(dbPath))
	if err != nil {
		return nil, false
	}
	var m commitMarker
	if err := json.Unmarshal(raw, &m); err != nil || m.CommittedAt == "" {
		return nil, false
	}
	return &m, true
}

func restoreStatusPathForDB(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), restoreStatusFile)
}

// ReadRestoreStatus loads the correlated outcome record of the most recent restore operation.
// Retained terminal recovery evidence (commit marker / restore journal) takes precedence over any
// preexisting status file so stale outcomes from an earlier restore can never mask a newer operation.
func ReadRestoreStatus(dbPath string) (*RestoreStatus, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, os.ErrNotExist
	}

	j, jErr := loadJournal(dbPath)
	if marker, hasMarker := hasValidCommitMarker(dbPath); hasMarker {
		restoreID := marker.RestoreID
		createdAt := marker.Manifest.CreatedAt
		if jErr == nil {
			if restoreID == "" {
				restoreID = j.RestoreID
			}
			if createdAt == "" {
				createdAt = j.Manifest.CreatedAt
			}
		}
		return &RestoreStatus{
			RestoreID:       restoreID,
			Outcome:         OutcomeApplied,
			BackupCreatedAt: createdAt,
			CompletedAt:     marker.CommittedAt,
		}, nil
	}
	if jErr == nil {
		switch j.State {
		case JournalStateCommitted:
			return &RestoreStatus{
				RestoreID:       j.RestoreID,
				Outcome:         OutcomeApplied,
				BackupCreatedAt: j.Manifest.CreatedAt,
				CompletedAt:     j.UpdatedAt,
			}, nil
		case JournalStateRolledBack:
			return &RestoreStatus{
				RestoreID:       j.RestoreID,
				Outcome:         OutcomeRolledBack,
				BackupCreatedAt: j.Manifest.CreatedAt,
				Message:         j.LastError,
				CompletedAt:     j.UpdatedAt,
			}, nil
		case JournalStateRollbackFailed:
			return &RestoreStatus{
				RestoreID:       j.RestoreID,
				Outcome:         OutcomeFailedClosed,
				BackupCreatedAt: j.Manifest.CreatedAt,
				Message:         j.LastError,
				CompletedAt:     j.UpdatedAt,
			}, nil
		}
	}

	raw, err := os.ReadFile(restoreStatusPathForDB(dbPath))
	if err == nil {
		var st RestoreStatus
		if unmarshalErr := json.Unmarshal(raw, &st); unmarshalErr == nil && st.Outcome != "" {
			if jErr == nil && j.RestoreID != "" && st.RestoreID != "" && st.RestoreID != j.RestoreID {
				return nil, os.ErrNotExist
			}
			return &st, nil
		}
		return nil, errors.New("corrupt restore status metadata")
	}
	return nil, err
}

func writeRestoreStatus(dbPath string, status RestoreStatus) error {
	return writeRestoreStatusWithHooks(dbPath, status, nil)
}

func writeRestoreStatusWithHooks(dbPath string, status RestoreStatus, hooks *RestoreFaultHooks) error {
	if strings.TrimSpace(dbPath) == "" {
		return nil
	}
	if status.CompletedAt == "" {
		status.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if hooks != nil && hooks.OnBeforeWriteStatus != nil {
		if err := hooks.OnBeforeWriteStatus(status); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return writeFileSynced(restoreStatusPathForDB(dbPath), data, 0600)
}

func syncDir(dir string) error {
	return syncDirWithHooks(dir, nil)
}

func syncDirWithHooks(dir string, hooks *RestoreFaultHooks) error {
	if hooks != nil && hooks.OnBeforeSyncDir != nil {
		if err := hooks.OnBeforeSyncDir(dir); err != nil {
			return err
		}
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	if syncErr != nil && !errors.Is(syncErr, syscall.EINVAL) && !errors.Is(syncErr, syscall.ENOTSUP) {
		return syncErr
	}
	return closeErr
}

func writeFileSynced(path string, data []byte, mode fs.FileMode) error {
	return writeFileSyncedWithHooks(path, data, mode, nil)
}

func writeFileSyncedWithHooks(path string, data []byte, mode fs.FileMode, hooks *RestoreFaultHooks) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-sync-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	if err := syncDirWithHooks(dir, hooks); err != nil {
		return err
	}
	return nil
}

func saveJournal(j *RestoreJournal) error {
	return saveJournalWithHooks(j, nil)
}

func saveJournalWithHooks(j *RestoreJournal, hooks *RestoreFaultHooks) error {
	j.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return writeFileSyncedWithHooks(journalPathForDB(j.DBPath), data, 0600, hooks)
}

func loadJournal(dbPath string) (*RestoreJournal, error) {
	path := journalPathForDB(dbPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var j RestoreJournal
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

var statfsFunc = syscall.Statfs

// SetStatfsForTest overrides filesystem free-space inspection in unit tests.
func SetStatfsForTest(fn func(string, *syscall.Statfs_t) error) func() {
	prev := statfsFunc
	if fn != nil {
		statfsFunc = fn
	}
	return func() { statfsFunc = prev }
}

func checkAvailableDiskSpace(dir string, requiredBytes uint64) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("prepare directory %s: %w", dir, err)
	}
	var st syscall.Statfs_t
	if err := statfsFunc(dir, &st); err != nil {
		return fmt.Errorf("inspect filesystem space on %s: %w", dir, err)
	}
	avail := uint64(st.Bavail) * uint64(st.Bsize)
	if avail < requiredBytes {
		return fmt.Errorf("insufficient disk space on %s: need at least %d bytes, available %d bytes", dir, requiredBytes, avail)
	}
	return nil
}

func cleanupRestoreWorkDirs(j *RestoreJournal) {
	if j == nil {
		return
	}
	if j.DBWorkDir != "" {
		_ = os.RemoveAll(j.DBWorkDir)
	}
	if j.PKIWorkDir != "" {
		_ = os.RemoveAll(j.PKIWorkDir)
	}
}

func captureRollbackSnapshots(j *RestoreJournal, hooks *RestoreFaultHooks) error {
	if hooks != nil && hooks.OnBeforeSnapshotRollback != nil {
		if err := hooks.OnBeforeSnapshotRollback(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(j.DBWorkDir, 0700); err != nil {
		return fmt.Errorf("prepare DB rollback work dir: %w", err)
	}
	if err := os.MkdirAll(j.PKIWorkDir, 0700); err != nil {
		return fmt.Errorf("prepare PKI rollback work dir: %w", err)
	}

	rollbackPKIDir := filepath.Join(j.PKIWorkDir, "rollback")
	_ = os.RemoveAll(rollbackPKIDir)
	if err := copyDirectoryPreservingWork(j.PKIDir, rollbackPKIDir); err != nil {
		return fmt.Errorf("snapshot live PKI for rollback: %w", err)
	}
	j.RollbackPKIDir = rollbackPKIDir

	rollbackDBPath := filepath.Join(j.DBWorkDir, "rollback.db")
	_ = os.Remove(rollbackDBPath)
	if _, statErr := os.Stat(j.DBPath); statErr == nil {
		snapConn, err := gorm.Open(sqlite.Open(j.DBPath), &gorm.Config{
			Logger: gormlogger.Default.LogMode(gormlogger.Silent),
		})
		if err != nil {
			return fmt.Errorf("open live database for post-drain rollback snapshot: %w", err)
		}
		vacErr := snapConn.Exec("VACUUM INTO ?", rollbackDBPath).Error
		if sqlDB, sErr := snapConn.DB(); sErr == nil {
			_ = sqlDB.Close()
		}
		if vacErr != nil {
			return fmt.Errorf("snapshot live database for rollback: %w", vacErr)
		}
		if err := syncDirWithHooks(filepath.Dir(rollbackDBPath), hooks); err != nil {
			return fmt.Errorf("sync rollback database snapshot directory: %w", err)
		}
		j.RollbackDBPath = rollbackDBPath
		j.HasRollbackDB = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat live database for rollback: %w", statErr)
	} else {
		j.HasRollbackDB = false
	}

	j.HadExistingMFAKey = false
	j.RollbackMFAKey = ""
	if strings.TrimSpace(j.MFAKeyPath) != "" {
		existingMFAKey, readErr := os.ReadFile(j.MFAKeyPath)
		if readErr == nil {
			rollbackMFAKey := filepath.Join(j.DBWorkDir, "rollback.mfa.key")
			if err := writeFileSyncedWithHooks(rollbackMFAKey, existingMFAKey, 0600, hooks); err != nil {
				return fmt.Errorf("snapshot live MFA key for rollback: %w", err)
			}
			j.RollbackMFAKey = rollbackMFAKey
			j.HadExistingMFAKey = true
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return fmt.Errorf("read live MFA key for rollback: %w", readErr)
		}
	}

	j.RollbackCaptured = true
	return nil
}

// CapturePendingRollbackSnapshots captures durable rollback copies for a staged/prepared journal
// once active mutations have been drained. Primarily used by lifecycle tests.
func CapturePendingRollbackSnapshots(dbPath string) error {
	j, err := loadJournal(dbPath)
	if err != nil {
		return err
	}
	if db.DB != nil {
		_ = db.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE);").Error
		if sqlDB, closeErr := db.DB.DB(); closeErr == nil {
			_ = sqlDB.Close()
		}
		db.DB = nil
	}
	if err := captureRollbackSnapshots(j, nil); err != nil {
		return err
	}
	return saveJournal(j)
}

func rollbackRestoreTransaction(j *RestoreJournal, hooks *RestoreFaultHooks, originalErr error) error {
	// Close any partially opened handle cleanly before reverting files.
	if db.DB != nil {
		if sqlDB, err := db.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		db.DB = nil
	}

	var rollbackErrs []error
	if !j.RollbackCaptured || strings.TrimSpace(j.RollbackPKIDir) == "" {
		rollbackErrs = append(rollbackErrs, errors.New("rollback snapshot material missing"))
	} else if _, statErr := os.Stat(j.RollbackPKIDir); statErr != nil {
		rollbackErrs = append(rollbackErrs, fmt.Errorf("rollback PKI snapshot unavailable: %w", statErr))
	}

	if len(rollbackErrs) == 0 && hooks != nil && hooks.OnRollbackPKI != nil {
		if err := hooks.OnRollbackPKI(); err != nil {
			rollbackErrs = append(rollbackErrs, err)
		}
	}
	if len(rollbackErrs) == 0 {
		if err := replaceDirectoryContentsPreservingWork(j.PKIDir, j.RollbackPKIDir); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restore rollback PKI: %w", err))
		}
	}

	if len(rollbackErrs) == 0 && hooks != nil && hooks.OnRollbackDB != nil {
		if err := hooks.OnRollbackDB(); err != nil {
			rollbackErrs = append(rollbackErrs, err)
		}
	}
	if len(rollbackErrs) == 0 && j.HasRollbackDB {
		if _, statErr := os.Stat(j.RollbackDBPath); statErr != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("rollback database snapshot unavailable: %w", statErr))
		} else {
			for _, suffix := range []string{"-wal", "-shm"} {
				if err := os.Remove(j.DBPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
					rollbackErrs = append(rollbackErrs, fmt.Errorf("remove %s sidecar: %w", suffix, err))
				}
			}
			if len(rollbackErrs) == 0 {
				if err := copyFileAtomicSynced(j.RollbackDBPath, j.DBPath, 0600); err != nil {
					rollbackErrs = append(rollbackErrs, fmt.Errorf("restore rollback database: %w", err))
				}
			}
		}
	}

	if len(rollbackErrs) == 0 && j.MFAKeyPath != "" {
		if j.HadExistingMFAKey && j.RollbackMFAKey != "" {
			if err := copyFileAtomicSynced(j.RollbackMFAKey, j.MFAKeyPath, 0600); err != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("restore rollback MFA key: %w", err))
			}
		} else {
			if err := os.Remove(j.MFAKeyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("clean temporary MFA key: %w", err))
			}
		}
	}

	if len(rollbackErrs) == 0 && j.HasRollbackDB {
		conn, err := db.InitDB(j.DBPath)
		if err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("reopen rollback database: %w", err))
		} else {
			var integrity string
			if err := conn.Raw("PRAGMA integrity_check;").Scan(&integrity).Error; err != nil || strings.ToLower(integrity) != "ok" {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("rollback database integrity check failed: %v", err))
			}
			_, _, mfaErr := mfa.InitActiveKey(conn, j.DBPath, "")
			if mfaErr != nil {
				slog.Warn("Rollback MFA key validation reported notice", "error", mfaErr)
			}
		}
	}
	if len(rollbackErrs) == 0 {
		if err := validateExtractedPKI(j.PKIDir); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("validate rollback PKI: %w", err))
		}
	}

	if len(rollbackErrs) != 0 {
		joined := errors.Join(rollbackErrs...)
		j.State = JournalStateRollbackFailed
		j.LastError = fmt.Sprintf("restore error: %v | rollback error: %v", originalErr, joined)
		if saveErr := saveJournal(j); saveErr != nil {
			slog.Error("CRITICAL: Failed to persist rollback_failed state in restore journal", "error", saveErr)
		}
		_ = writeRestoreStatus(j.DBPath, RestoreStatus{
			RestoreID:       j.RestoreID,
			Outcome:         OutcomeFailedClosed,
			BackupCreatedAt: j.Manifest.CreatedAt,
			Message:         "Backup restore and automatic rollback both failed",
		})
		maintenance.DefaultCoordinator.EnterFailedClosed("Backup restore rollback failed; rollback archive preserved on persistent storage")
		slog.Error("CRITICAL: Restore and rollback both failed; locking control plane in fail-closed mode", "error", joined)
		return fmt.Errorf("%w: %v", ErrRollbackFailed, joined)
	}

	// Do not clean up recovery evidence until the correlated rolled_back outcome is persisted.
	if stErr := writeRestoreStatusWithHooks(j.DBPath, RestoreStatus{
		RestoreID:       j.RestoreID,
		Outcome:         OutcomeRolledBack,
		BackupCreatedAt: j.Manifest.CreatedAt,
		Message:         originalErr.Error(),
	}, hooks); stErr != nil {
		j.State = JournalStateRolledBack
		j.LastError = originalErr.Error()
		if saveErr := saveJournal(j); saveErr != nil {
			slog.Error("Rollback succeeded, but failed to update retained recovery journal", "error", saveErr)
		}
		slog.Error("Rollback succeeded cleanly, but failed to persist restore status; retaining recovery evidence", "error", stErr)
	} else {
		cleanupRestoreWorkDirs(j)
	}
	return fmt.Errorf("%w: %v", ErrRestoredRolledBack, originalErr)
}

// RecoverOrApplyPendingRestore runs with exclusive control-plane ownership on startup (or
// after draining in tests) to either complete a prepared restore, roll back an interrupted
// restore, or lock fail-closed if prior rollback failed or journal recovery state is unknown/corrupt.
func RecoverOrApplyPendingRestore(dbPath, pkiDir string, manager *pki.PKIManager, hooks *RestoreFaultHooks) (*Manifest, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, nil
	}
	j, err := loadJournal(dbPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		_ = writeRestoreStatus(dbPath, RestoreStatus{
			Outcome: OutcomeFailedClosed,
			Message: "Restore transaction journal could not be read or parsed",
		})
		maintenance.DefaultCoordinator.EnterFailedClosed("Restore transaction journal corrupt or unreadable; manual operator inspection required")
		return nil, fmt.Errorf("%w: read restore transaction journal: %v", ErrRecoveryFailed, err)
	}

	switch j.State {
	case JournalStateRollbackFailed:
		_ = writeRestoreStatus(dbPath, RestoreStatus{
			RestoreID:       j.RestoreID,
			Outcome:         OutcomeFailedClosed,
			BackupCreatedAt: j.Manifest.CreatedAt,
			Message:         "Prior backup restore rollback failed",
		})
		maintenance.DefaultCoordinator.EnterFailedClosed("Prior backup restore rollback failed; manual operator intervention required")
		return nil, ErrRollbackFailed
	case JournalStateCommitted:
		if stErr := writeRestoreStatusWithHooks(j.DBPath, RestoreStatus{
			RestoreID:       j.RestoreID,
			Outcome:         OutcomeApplied,
			BackupCreatedAt: j.Manifest.CreatedAt,
		}, hooks); stErr != nil {
			slog.Error("Committed restore detected on startup, but persisting restore status failed; retaining recovery journal", "error", stErr)
		} else {
			cleanupRestoreWorkDirs(j)
		}
		return &j.Manifest, nil
	case JournalStateRolledBack:
		if stErr := writeRestoreStatusWithHooks(j.DBPath, RestoreStatus{
			RestoreID:       j.RestoreID,
			Outcome:         OutcomeRolledBack,
			BackupCreatedAt: j.Manifest.CreatedAt,
			Message:         j.LastError,
		}, hooks); stErr != nil {
			slog.Error("Previously rolled-back restore journal found, but persisting restore status failed; retaining recovery evidence", "error", stErr)
		} else {
			cleanupRestoreWorkDirs(j)
		}
		return nil, nil
	case JournalStateApplying:
		// If a durable commit marker succeeded before work-dir cleanup, commit completed cleanly.
		if marker, ok := hasValidCommitMarker(dbPath); ok {
			m := marker.Manifest
			if m.DBSHA256 == "" {
				m = j.Manifest
			}
			if stErr := writeRestoreStatusWithHooks(j.DBPath, RestoreStatus{
				RestoreID:       j.RestoreID,
				Outcome:         OutcomeApplied,
				BackupCreatedAt: m.CreatedAt,
			}, hooks); stErr != nil {
				slog.Error("Failed to persist restore outcome status after commit recovery; retaining recovery evidence", "error", stErr)
			} else {
				cleanupRestoreWorkDirs(j)
			}
			return &m, nil
		}
		// Previous process crashed mid-activation: roll back atomically before serving.
		slog.Warn("Detected interrupted restore transaction on startup; restoring pre-restore rollback state")
		return nil, rollbackRestoreTransaction(j, hooks, errors.New("process interrupted during prior restore activation"))
	case JournalStatePrepared:
		// Proceed with exclusive post-drain activation below.
	default:
		_ = writeRestoreStatus(dbPath, RestoreStatus{
			RestoreID:       j.RestoreID,
			Outcome:         OutcomeFailedClosed,
			BackupCreatedAt: j.Manifest.CreatedAt,
			Message:         fmt.Sprintf("Unrecognized restore journal state %q", j.State),
		})
		maintenance.DefaultCoordinator.EnterFailedClosed("Unrecognized restore transaction journal state; manual operator inspection required")
		return nil, fmt.Errorf("%w: unrecognized restore journal state %q", ErrRecoveryFailed, j.State)
	}

	// Acquire mutation barrier for synchronous/test callers and close any open DB handle
	// BEFORE capturing rollback snapshots, so every prior acknowledged write is included.
	releaseSnap := maintenance.DefaultCoordinator.AcquireSnapshotBarrier()
	defer releaseSnap()

	if db.DB != nil {
		_ = db.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE);").Error
		if sqlDB, closeErr := db.DB.DB(); closeErr == nil {
			_ = sqlDB.Close()
		}
		db.DB = nil
	}

	if !j.RollbackCaptured {
		if capErr := captureRollbackSnapshots(j, hooks); capErr != nil {
			// Live DB/PKI files were not touched yet; abort restore cleanly preserving prior live state.
			if _, statErr := os.Stat(j.DBPath); statErr == nil {
				_, _ = db.InitDB(j.DBPath)
			}
			if stErr := writeRestoreStatusWithHooks(j.DBPath, RestoreStatus{
				RestoreID:       j.RestoreID,
				Outcome:         OutcomeRolledBack,
				BackupCreatedAt: j.Manifest.CreatedAt,
				Message:         "Pre-activation rollback snapshot failed; prior live state preserved",
			}, hooks); stErr != nil {
				j.State = JournalStateRolledBack
				j.LastError = "Pre-activation rollback snapshot failed; prior live state preserved"
				_ = saveJournal(j)
				slog.Error("Pre-activation abort preserved prior state, but failed to persist restore status; retaining recovery journal", "error", stErr)
			} else {
				cleanupRestoreWorkDirs(j)
			}
			return nil, fmt.Errorf("%w: capture pre-activation rollback snapshot: %v", ErrRestoredRolledBack, capErr)
		}
	}

	j.State = JournalStateApplying
	if err := saveJournalWithHooks(j, hooks); err != nil {
		_ = writeRestoreStatus(j.DBPath, RestoreStatus{
			RestoreID:       j.RestoreID,
			Outcome:         OutcomeFailedClosed,
			BackupCreatedAt: j.Manifest.CreatedAt,
			Message:         "Failed to durably transition restore journal to applying",
		})
		maintenance.DefaultCoordinator.EnterFailedClosed("Failed to persist applying restore transaction journal; manual inspection required")
		return nil, fmt.Errorf("%w: transition restore journal to applying: %v", ErrRecoveryFailed, err)
	}

	failAndRollback := func(cause error) (*Manifest, error) {
		slog.Error("Restore activation step failed; rolling back", "error", cause)
		return nil, rollbackRestoreTransaction(j, hooks, cause)
	}

	if hooks != nil && hooks.OnBeforeReplacePKI != nil {
		if hookErr := hooks.OnBeforeReplacePKI(); hookErr != nil {
			return failAndRollback(hookErr)
		}
	}
	if err := replaceDirectoryContentsPreservingWork(j.PKIDir, j.StagedPKIDir); err != nil {
		return failAndRollback(fmt.Errorf("apply restored PKI state: %w", err))
	}

	if hooks != nil && hooks.OnBeforeReplaceDB != nil {
		if hookErr := hooks.OnBeforeReplaceDB(); hookErr != nil {
			return failAndRollback(hookErr)
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(j.DBPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return failAndRollback(fmt.Errorf("remove stale SQLite sidecar %s: %w", suffix, err))
		}
	}
	if err := copyFileAtomicSynced(j.StagedDBPath, j.DBPath, 0600); err != nil {
		return failAndRollback(fmt.Errorf("replace SQLite database: %w", err))
	}
	if j.MFAKeyPath != "" {
		if j.StagedMFAKeyPath != "" {
			if err := copyFileAtomicSynced(j.StagedMFAKeyPath, j.MFAKeyPath, 0600); err != nil {
				return failAndRollback(fmt.Errorf("activate restored MFA key: %w", err))
			}
		} else if j.HadExistingMFAKey {
			if err := os.Remove(j.MFAKeyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return failAndRollback(fmt.Errorf("clear stale host MFA key for legacy v1 restore: %w", err))
			}
		}
	}

	if hooks != nil && hooks.OnBeforeReopenDB != nil {
		if hookErr := hooks.OnBeforeReopenDB(); hookErr != nil {
			return failAndRollback(hookErr)
		}
	}
	reopenedDB, err := db.InitDB(j.DBPath)
	if err != nil {
		return failAndRollback(fmt.Errorf("reopen restored database: %w", err))
	}
	var integrity string
	if err := reopenedDB.Raw("PRAGMA integrity_check;").Scan(&integrity).Error; err != nil || strings.ToLower(integrity) != "ok" {
		return failAndRollback(fmt.Errorf("restored database integrity check failed: %q (%v)", integrity, err))
	}
	var fkCount int64
	if err := reopenedDB.Raw("SELECT COUNT(*) FROM pragma_foreign_key_check;").Scan(&fkCount).Error; err != nil || fkCount > 0 {
		return failAndRollback(fmt.Errorf("restored database failed foreign_key_check (violations=%d, err=%v)", fkCount, err))
	}

	// Activate MFA key cleanly and re-verify restored encrypted MFA state.
	if j.StagedMFAKeyPath != "" {
		keyStr, keyErr := mfa.ReadKeyFile(j.MFAKeyPath)
		if keyErr != nil {
			return failAndRollback(fmt.Errorf("verify activated MFA key: %w", keyErr))
		}
		if err := mfa.ValidateKeyDecryptsDB(reopenedDB, keyStr); err != nil {
			return failAndRollback(fmt.Errorf("restored MFA key failed verification: %w", err))
		}
		mfa.SetActiveKey(keyStr, j.MFAKeyPath)
	} else {
		// Legacy v1 restore without MFA secrets: ensure active key exists for future MFA enrollments.
		if _, _, err := mfa.InitActiveKey(reopenedDB, j.DBPath, ""); err != nil {
			return failAndRollback(fmt.Errorf("initialize active MFA key post-restore: %w", err))
		}
	}

	// Post-restore hygiene in one atomic DB transaction:
	// 1. Rotate SessionEpoch (invalidates all pre-restore JWTs).
	// 2. Purge stale MFA challenges and OIDC auth attempts/tickets.
	// 3. Cancel restored pending/retrying webhook deliveries so stale alerts never replay.
	// 4. Mark gateway RuntimeApplyState pending reconciliation.
	// 5. Append durable backup.restored audit record.
	var auditEntry db.AuditLog
	if err := reopenedDB.Transaction(func(tx *gorm.DB) error {
		if _, err := db.RotateSessionEpochTx(tx); err != nil {
			return fmt.Errorf("rotate post-restore session epoch: %w", err)
		}
		if err := tx.Where("1 = 1").Delete(&db.MFAChallenge{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&db.OIDCLoginAttempt{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&db.OIDCLoginTicket{}).Error; err != nil {
			return err
		}
		if _, err := audit.CancelPendingDeliveriesTx(tx); err != nil {
			return fmt.Errorf("cancel restored outbox queue: %w", err)
		}
		_ = tx.Model(&db.RuntimeApplyState{}).Where("id = ?", 1).Updates(map[string]any{
			"status":     "pending",
			"last_error": "System restored from backup; reconciliation pending",
		}).Error

		actorRole := j.ActorRole
		if actorRole == "" {
			actorRole = "admin"
		}
		var recErr error
		auditEntry, recErr = audit.RecordTx(tx, audit.EntryInput{
			ActorID:    j.ActorID,
			ActorEmail: j.ActorEmail,
			ActorRole:  actorRole,
			SourceIP:   j.SourceIP,
			Action:     "backup.restored",
			Severity:   audit.SeverityCritical,
			TargetType: "backup",
			TargetID:   j.Manifest.DBSHA256[:12],
			Summary:    fmt.Sprintf("System restored from encrypted backup (%s)", j.Manifest.CreatedAt),
			Details: map[string]any{
				"backup_created_at": j.Manifest.CreatedAt,
				"format_version":    j.Manifest.FormatVersion,
				"pki_files":         len(j.Manifest.PKIFiles),
			},
		})
		return recErr
	}); err != nil {
		return failAndRollback(fmt.Errorf("finalize restored database state: %w", err))
	}

	if hooks != nil && hooks.OnBeforePKIReload != nil {
		if hookErr := hooks.OnBeforePKIReload(); hookErr != nil {
			return failAndRollback(hookErr)
		}
	}
	if err := validateExtractedPKI(j.PKIDir); err != nil {
		return failAndRollback(fmt.Errorf("verify activated PKI state: %w", err))
	}
	if manager != nil {
		if err := manager.ReloadFromDisk(); err != nil {
			return failAndRollback(fmt.Errorf("reload PKI manager after restore: %w", err))
		}
	}

	// Commit via an independent durable commit marker BEFORE mutating journal.json away from
	// "applying" or deleting rollback snapshots. If commit or directory fsync fails, remove the
	// marker cleanly while journal.json stays "applying" so even a crash mid-rollback safely recovers.
	if hooks != nil && hooks.OnBeforeCommitJournal != nil {
		if hookErr := hooks.OnBeforeCommitJournal(); hookErr != nil {
			return failAndRollback(fmt.Errorf("persist committed restore journal: %w", hookErr))
		}
	}
	markerBytes, err := json.MarshalIndent(commitMarker{
		RestoreID:   j.RestoreID,
		CommittedAt: time.Now().UTC().Format(time.RFC3339),
		Manifest:    j.Manifest,
	}, "", "  ")
	if err != nil {
		return failAndRollback(fmt.Errorf("encode restore commit marker: %w", err))
	}
	markerPath := commitMarkerPathForDB(j.DBPath)
	if err := writeFileSyncedWithHooks(markerPath, markerBytes, 0600, hooks); err != nil {
		// Durably retract the commit marker (remove + syncDir) BEFORE any rollback begins;
		// if retraction or directory sync fails, fail-closed preserving all recovery material.
		remErr := os.Remove(markerPath)
		var retractSyncErr error
		if remErr == nil || errors.Is(remErr, os.ErrNotExist) {
			retractSyncErr = syncDirWithHooks(filepath.Dir(markerPath), hooks)
		}
		if (remErr != nil && !errors.Is(remErr, os.ErrNotExist)) || retractSyncErr != nil {
			j.State = JournalStateRollbackFailed
			j.LastError = fmt.Sprintf("commit marker sync error: %v | retract error: %v (sync=%v)", err, remErr, retractSyncErr)
			if saveErr := saveJournal(j); saveErr != nil {
				slog.Error("CRITICAL: Failed to persist rollback_failed journal after durable marker retraction error", "error", saveErr)
			}
			maintenance.DefaultCoordinator.EnterFailedClosed("Commit journal sync and durable marker cleanup both failed; manual inspection required")
			return nil, fmt.Errorf("%w: unable to durably retract unsynced commit marker (rem=%v, sync=%v)", ErrRollbackFailed, remErr, retractSyncErr)
		}
		return failAndRollback(fmt.Errorf("persist committed restore journal: %w", err))
	}
	j.State = JournalStateCommitted
	if err := saveJournal(j); err != nil {
		slog.Warn("Committed marker durable, but updating journal metadata reported warning", "error", err)
	}
	// Never delete recovery journal / commit marker until correlated status is durably persisted.
	// Do NOT roll back a durably committed restore merely because writing the status file failed.
	if stErr := writeRestoreStatusWithHooks(j.DBPath, RestoreStatus{
		RestoreID:       j.RestoreID,
		Outcome:         OutcomeApplied,
		BackupCreatedAt: j.Manifest.CreatedAt,
	}, hooks); stErr != nil {
		slog.Error("Restored state committed, but failed to persist applied restore status; retaining recovery journal evidence", "error", stErr)
	} else {
		cleanupRestoreWorkDirs(j)
	}
	audit.NotifyCommitted(auditEntry)

	slog.Info("Encrypted backup restored and committed", "created_at", j.Manifest.CreatedAt, "pki_files", len(j.Manifest.PKIFiles))
	return &j.Manifest, nil
}
