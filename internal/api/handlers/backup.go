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
	"github.com/swan-panel/swan-panel/internal/pki"
	"github.com/swan-panel/swan-panel/internal/policy"
)

const maxRestoreUploadBytes = 64 * 1024 * 1024

// BackupHandler manages encrypted backup creation and disaster recovery restore.
type BackupHandler struct {
	cfg *config.Config
}

// NewBackupHandler creates a new BackupHandler.
func NewBackupHandler(cfg *config.Config) *BackupHandler {
	return &BackupHandler{cfg: cfg}
}

// CreateBackup exports an AES-256-GCM encrypted .tar.enc archive of SQLite and PKI state.
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
		if errors.Is(err, backup.ErrInvalidPassphrase) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		slog.Error("Encrypted backup creation failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create encrypted backup"})
		return
	}

	audit.RecordFromContext(c, audit.EntryInput{
		Action:     "backup.created",
		Severity:   audit.SeverityWarning,
		TargetType: "backup",
		TargetID:   manifest.DBSHA256[:12],
		Summary:    fmt.Sprintf("Encrypted backup exported (%d PKI files)", len(manifest.PKIFiles)),
		Details:    map[string]any{"pki_files": len(manifest.PKIFiles), "size_bytes": len(archive)},
	})

	filename := fmt.Sprintf("swanpanel-backup-%s.tar.enc", time.Now().UTC().Format("2006-01-02"))
	c.Header("Cache-Control", "no-store, private")
	c.Header("Pragma", "no-cache")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "application/octet-stream", archive)
}

// RestoreBackup decrypts, validates, and atomically restores database and PKI state.
func (h *BackupHandler) RestoreBackup(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRestoreUploadBytes+1024*1024)
	if err := c.Request.ParseMultipartForm(maxRestoreUploadBytes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid backup upload payload"})
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

	encrypted, err := io.ReadAll(io.LimitReader(file, maxRestoreUploadBytes+1))
	if err != nil || len(encrypted) > maxRestoreUploadBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Backup file exceeds maximum allowed size"})
		return
	}

	manager, err := pki.GetPKIManager()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "CA unavailable"})
		return
	}

	manifest, err := backup.RestoreEncryptedBackup(
		c.Request.Context(),
		encrypted,
		passphrase,
		h.cfg.DBPath,
		manager.DataDir(),
		manager,
	)
	if err != nil {
		if errors.Is(err, backup.ErrInvalidPassphrase) || errors.Is(err, backup.ErrInvalidArchive) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		slog.Error("Backup restore failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Backup restore failed: " + err.Error()})
		return
	}

	audit.RecordFromContext(c, audit.EntryInput{
		Action:     "backup.restored",
		Severity:   audit.SeverityCritical,
		TargetType: "backup",
		TargetID:   manifest.DBSHA256[:12],
		Summary:    fmt.Sprintf("System restored from encrypted backup (%s)", manifest.CreatedAt),
		Details:    map[string]any{"backup_created_at": manifest.CreatedAt, "pki_files": len(manifest.PKIFiles)},
	})

	// Redeploy server certificates, signed CRL, and gateway policies after restore.
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
