package handlers

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/backup"
	"github.com/swan-panel/swan-panel/internal/config"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"github.com/swan-panel/swan-panel/internal/mfa"
	"github.com/swan-panel/swan-panel/internal/pki"
	"github.com/swan-panel/swan-panel/internal/policy"
)

// BackupHandler manages encrypted backup creation and crash-safe disaster recovery restore.
type BackupHandler struct {
	cfg              *config.Config
	onRequestRestart func()
}

// NewBackupHandler creates a new BackupHandler.
func NewBackupHandler(cfg *config.Config, onRequestRestart ...func()) *BackupHandler {
	var hook func()
	if len(onRequestRestart) > 0 {
		hook = onRequestRestart[0]
	}
	return &BackupHandler{cfg: cfg, onRequestRestart: hook}
}

// CreateBackup exports an AES-256-GCM encrypted .tar.enc archive of SQLite, PKI, and MFA key state.
func (h *BackupHandler) CreateBackup(c *gin.Context) {
	var req struct {
		Passphrase string `json:"passphrase" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Backup passphrase is required"})
		return
	}

	manager, err := pki.GetPKIManager()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "CA unavailable"})
		return
	}

	archive, manifest, err := backup.CreateEncryptedBackup(c.Request.Context(), db.DB, manager.DataDir(), req.Passphrase)
	if err != nil {
		if errors.Is(err, backup.ErrInvalidPassphrase) || errors.Is(err, backup.ErrBackupLimitExceeded) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, backup.ErrBackupBusy) {
			c.Header("Retry-After", "5")
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "retry_after_seconds": 5})
			return
		}
		slog.Error("Encrypted backup creation failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create encrypted backup"})
		return
	}

	if err := audit.RecordFromContext(c, audit.EntryInput{
		Action:     "backup.created",
		Severity:   audit.SeverityWarning,
		TargetType: "backup",
		TargetID:   manifest.DBSHA256[:12],
		Summary:    fmt.Sprintf("Encrypted backup exported (%d PKI files)", len(manifest.PKIFiles)),
		Details:    map[string]any{"pki_files": len(manifest.PKIFiles), "size_bytes": len(archive), "format_version": manifest.FormatVersion},
	}); err != nil {
		slog.Error("Backup audit persistence failed; failing closed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Unable to record mandatory backup audit trail"})
		return
	}

	filename := fmt.Sprintf("swanpanel-backup-%s.tar.enc", time.Now().UTC().Format("2006-01-02"))
	c.Header("Cache-Control", "no-store, private")
	c.Header("Pragma", "no-cache")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "application/octet-stream", archive)
}

// RestoreBackup decrypts, pre-validates, persists a crash-safe restore transaction,
// enters maintenance mode, and triggers controlled restart (or synchronous activation in tests).
func (h *BackupHandler) RestoreBackup(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, backup.MaxBackupCipherBytes+1024*1024)
	if err := c.Request.ParseMultipartForm(backup.MaxBackupCipherBytes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid backup upload payload or size exceeds 64 MiB limit"})
		return
	}

	passphrase := c.PostForm("passphrase")
	confirm := strings.TrimSpace(c.PostForm("confirm"))
	if confirm != "RESTORE BACKUP" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Explicit RESTORE BACKUP confirmation required"})
		return
	}

	fileHeader, err := c.FormFile("backup_file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Encrypted backup file is required"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unable to read uploaded backup file"})
		return
	}
	defer func() { _ = file.Close() }()

	encrypted, err := io.ReadAll(io.LimitReader(file, backup.MaxBackupCipherBytes+1))
	if err != nil || len(encrypted) > backup.MaxBackupCipherBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Backup file exceeds maximum allowed 64 MiB limit"})
		return
	}

	manager, err := pki.GetPKIManager()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "CA unavailable"})
		return
	}

	releaseRestore, ok := maintenance.DefaultCoordinator.TryLockRestore()
	if !ok {
		c.Header("Retry-After", "5")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Another restore operation is already in progress", "maintenance": true, "retry_after_seconds": 5})
		return
	}
	defer releaseRestore()

	mfaPath := h.cfg.MFAKeyFile
	if mfaPath == "" {
		mfaPath = mfa.ResolveKeyFilePath(h.cfg.DBPath)
	}
	enriched := audit.EnrichFromContext(c, audit.EntryInput{})
	journal, err := backup.PrepareRestore(c.Request.Context(), encrypted, passphrase, h.cfg.DBPath, manager.DataDir(), backup.RestoreMetadata{
		ActorID:    enriched.ActorID,
		ActorEmail: enriched.ActorEmail,
		ActorRole:  enriched.ActorRole,
		SourceIP:   enriched.SourceIP,
		MFAKeyPath: mfaPath,
	})
	if err != nil {
		switch {
		case errors.Is(err, backup.ErrInvalidPassphrase),
			errors.Is(err, backup.ErrInvalidArchive),
			errors.Is(err, backup.ErrBackupLimitExceeded),
			errors.Is(err, backup.ErrIncompatibleMFAArchive):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, backup.ErrBackupBusy):
			c.Header("Retry-After", "5")
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "retry_after_seconds": 5})
		case errors.Is(err, backup.ErrRestoreJournalConflict):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "code": "RECOVERY_FAILED_CLOSED"})
		default:
			slog.Error("Backup preflight staging failed", "error", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Backup restore staging failed: " + err.Error()})
		}
		return
	}

	// Release this request's own in-flight counter BEFORE draining all other requests.
	if rel, exists := c.Get("maintenance_release"); exists {
		if fn, isFn := rel.(func()); isFn {
			fn()
		}
	}
	if drainErr := maintenance.DefaultCoordinator.EnterMaintenance("Restoring encrypted backup", 5*time.Second); drainErr != nil {
		slog.Warn("Entering maintenance mode after drain deadline", "error", drainErr)
	}

	// Production controlled-restart path: respond 202 restarting, flush HTTP, then
	// signal graceful shutdown so service/container supervision restarts cleanly into startup recovery.
	if h.onRequestRestart != nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusAccepted, gin.H{
			"restore_id":          journal.RestoreID,
			"runtime_status":      "restarting",
			"maintenance":         true,
			"retry_after_seconds": 5,
			"manifest":            journal.Manifest,
			"message":             "Backup validated and staged; control plane is restarting to apply restored state",
		})
		c.Writer.Flush()
		go h.onRequestRestart()
		return
	}

	// Test/in-process synchronous path when no process supervisor restart hook is attached.
	defer maintenance.DefaultCoordinator.ExitMaintenance()
	manifest, err := backup.RecoverOrApplyPendingRestore(h.cfg.DBPath, manager.DataDir(), manager, nil)
	if err != nil {
		slog.Error("In-process backup restore activation failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"restore_id": journal.RestoreID,
			"error":      "Backup restore failed: " + err.Error(),
		})
		return
	}

	var sw db.StrongswanSettings
	if err := db.DB.WithContext(c.Request.Context()).First(&sw, 1).Error; err != nil {
		c.JSON(http.StatusAccepted, gin.H{"runtime_status": "pending", "manifest": manifest, "message": "Backup restored; gateway settings unavailable"})
		return
	}
	if err := manager.EnsureServerCertificates(sw.ServerIdentity, []string{sw.ServerEndpoint}); err != nil {
		c.JSON(http.StatusAccepted, gin.H{"runtime_status": "pending", "manifest": manifest, "message": "Backup restored; server certificate sync pending"})
		return
	}
	if err := publishRevocationLedger(c.Request.Context()); err != nil {
		c.JSON(http.StatusAccepted, gin.H{"runtime_status": "pending", "manifest": manifest, "message": "Backup restored; CRL sync pending"})
		return
	}
	if err := policy.Reconcile(c.Request.Context(), db.DB); err != nil {
		c.JSON(http.StatusAccepted, gin.H{"runtime_status": "pending", "manifest": manifest, "message": "Backup restored; gateway reconciliation pending"})
		return
	}

	if realtime.DefaultBroker != nil {
		realtime.DefaultBroker.Broadcast("strongswan_settings_updated", sw)
	}
	c.JSON(http.StatusOK, gin.H{
		"runtime_status": "applied",
		"manifest":       manifest,
		"message":        "Backup restored and gateway configuration applied",
	})
}
