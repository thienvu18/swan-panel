package db

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB initializes SQLite connection with WAL mode, foreign keys, and runs Auto-Migrate
func InitDB(dbPath string) (*gorm.DB, error) {
	slog.Debug("Opening SQLite database connection", "path", dbPath)

	gormLog := logger.New(log.New(os.Stderr, "\r\n", log.LstdFlags), logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
		Colorful:                  false,
	})
	config := &gorm.Config{
		Logger: gormLog,
	}

	db, err := gorm.Open(sqlite.Open(dbPath), config)
	if err != nil {
		slog.Error("Database connection failed", "path", dbPath, "error", err)
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Enable WAL Mode, Foreign Keys, and Busy Timeout for concurrency stability
	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("Failed to obtain underlying SQL DB instance", "error", err)
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1) // SQLite single writer pool recommendation

	// SQLite PRAGMAs are connection-local (except WAL). With one pooled
	// connection they apply to every GORM query. Never silently continue when
	// foreign-key checks, locking or WAL mode cannot be enabled.
	for _, statement := range []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
	} {
		if err := db.Exec(statement).Error; err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("configure SQLite (%s): %w", statement, err)
		}
	}
	var journalMode string
	if err := db.Raw("PRAGMA journal_mode;").Scan(&journalMode).Error; err != nil || journalMode != "wal" {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("SQLite WAL mode not enabled: mode=%q, error=%v", journalMode, err)
	}
	var foreignKeys, busyTimeout int
	if err := db.Raw("PRAGMA foreign_keys;").Scan(&foreignKeys).Error; err != nil || foreignKeys != 1 {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("SQLite foreign_keys not enabled: value=%d, error=%v", foreignKeys, err)
	}
	if err := db.Raw("PRAGMA busy_timeout;").Scan(&busyTimeout).Error; err != nil || busyTimeout != 5000 {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("SQLite busy_timeout not enabled: value=%d, error=%v", busyTimeout, err)
	}

	// Initialize the current relational schema for fresh installations.
	slog.Debug("Starting database auto-migration for core models")
	err = db.AutoMigrate(
		&OIDCProvider{},
		&BootstrapState{},
		&OIDCLoginAttempt{},
		&OIDCLoginTicket{},
		&MFAChallenge{},
		&CertificateRevocation{},
		&User{},
		&UserGroup{},
		&ResourceGroup{},
		&Device{},
		&AccessRule{},
		&DNSSettings{},
		&DNSOverride{},
		&StrongswanSettings{},
		&RuntimeApplyState{},
		&AuditLog{},
		&AlertWebhook{},
		&SystemState{},
		&WebhookDelivery{},
	)
	if err != nil {
		slog.Error("Auto-migration failed", "error", err)
		return nil, fmt.Errorf("failed to auto-migrate schema: %w", err)
	}
	// Enforce append-only immutability on audit_logs at the SQLite engine level.
	for _, triggerSQL := range []string{
		`CREATE TRIGGER IF NOT EXISTS trg_audit_logs_no_update BEFORE UPDATE ON audit_logs BEGIN SELECT RAISE(ABORT, 'audit_logs is append-only and immutable'); END;`,
		`CREATE TRIGGER IF NOT EXISTS trg_audit_logs_no_delete BEFORE DELETE ON audit_logs BEGIN SELECT RAISE(ABORT, 'audit_logs is append-only and immutable'); END;`,
	} {
		if err := db.Exec(triggerSQL).Error; err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("create audit immutability trigger: %w", err)
		}
	}
	// Remove the legacy global unique index only after the active-device partial
	// unique index has been created. Retired identities keep their CRL ledger.
	if db.Migrator().HasIndex(&Device{}, "idx_devices_virtual_ip") {
		if err := db.Migrator().DropIndex(&Device{}, "idx_devices_virtual_ip"); err != nil {
			return nil, fmt.Errorf("migrate active-only VIP uniqueness: %w", err)
		}
	}
	slog.Info("Schema auto-migration completed successfully")

	DB = db

	// Installer token is generated before any server listener is started.
	if err := initializeBootstrap(db, dbPath); err != nil {
		return nil, fmt.Errorf("initial bootstrap: %w", err)
	}
	// Seed non-credential defaults only.
	seedDefaults(db)
	if err := EnsureSystemState(db); err != nil {
		return nil, fmt.Errorf("initialize system security state: %w", err)
	}

	return db, nil
}

func newSessionEpoch() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// EnsureSystemState creates the singleton SystemState record with an active session epoch if absent.
func EnsureSystemState(database *gorm.DB) error {
	if database == nil {
		return errors.New("database is nil")
	}
	var st SystemState
	err := database.First(&st, 1).Error
	if err == nil && st.SessionEpoch != "" {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	epoch, genErr := newSessionEpoch()
	if genErr != nil {
		return genErr
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return database.Create(&SystemState{ID: 1, SessionEpoch: epoch, RestoreGeneration: 0}).Error
	}
	return database.Model(&SystemState{}).Where("id = ?", 1).Update("session_epoch", epoch).Error
}

// CurrentSessionEpoch reads the active JWT revocation epoch from SQLite.
func CurrentSessionEpoch(database *gorm.DB) (string, error) {
	if database == nil {
		return "", nil
	}
	var st SystemState
	if err := database.Select("id", "session_epoch").First(&st, 1).Error; err != nil {
		return "", err
	}
	return st.SessionEpoch, nil
}

// RotateSessionEpochTx generates a fresh random session epoch and bumps restore generation
// inside tx, immediately invalidating all previously minted JWT session tokens.
func RotateSessionEpochTx(tx *gorm.DB) (string, error) {
	if tx == nil {
		return "", errors.New("transaction is nil")
	}
	epoch, err := newSessionEpoch()
	if err != nil {
		return "", err
	}
	var st SystemState
	err = tx.First(&st, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		st = SystemState{ID: 1, SessionEpoch: epoch, RestoreGeneration: 1}
		if err := tx.Create(&st).Error; err != nil {
			return "", err
		}
		return epoch, nil
	}
	if err != nil {
		return "", err
	}
	if err := tx.Model(&SystemState{}).Where("id = ?", 1).Updates(map[string]any{
		"session_epoch":      epoch,
		"restore_generation": st.RestoreGeneration + 1,
	}).Error; err != nil {
		return "", err
	}
	return epoch, nil
}

func seedDefaults(db *gorm.DB) {
	// 1. No users or passwords are seeded automatically.
	// 2. Seed default DNS settings
	var dnsCount int64
	if err := db.Model(&DNSSettings{}).Count(&dnsCount).Error; err != nil {
		slog.Error("Failed to query DNS settings count", "error", err)
	} else if dnsCount == 0 {
		slog.Info("Seeding initial DNS default settings", "magic_dns_suffix", ".strongnet")
		defaultDNS := DNSSettings{
			ID:                1,
			MagicDNSSuffix:    ".strongnet",
			UpstreamResolvers: "1.1.1.1, 8.8.8.8",
			IsMagicDNSEnabled: true,
		}
		if err := db.Create(&defaultDNS).Error; err != nil {
			slog.Error("Failed to seed default DNS settings", "error", err)
		} else {
			slog.Debug("Default DNS settings persisted")
		}
	} else {
		slog.Debug("DNS settings already exist, skipping default seeding")
	}

	// 3. Seed default strongSwan settings
	var swCount int64
	if err := db.Model(&StrongswanSettings{}).Count(&swCount).Error; err != nil {
		slog.Error("Failed to query strongSwan settings count", "error", err)
	} else if swCount == 0 {
		slog.Info("Seeding initial strongSwan daemon settings")
		defaultSW := StrongswanSettings{
			ID:                1,
			VPNName:           "SwanPanel VPN",
			ServerEndpoint:    "vpn.company.com",
			ServerIdentity:    "vpn.company.com",
			ReauthTime:        "12h",
			RekeyTimeIKE:      "4h",
			RekeyTimeChild:    "1h",
			InactivityTimeout: "30m",
			MobikeEnabled:     true,
			ForceEncapNATT:    false,
			Fragmentation:     "yes",
			DPDDelay:          "30s",
			DPDTimeout:        "120s",
			DPDAction:         "clear",
			CipherPreset:      "balanced",
			IKEProposals:      "aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048",
			ESPProposals:      "aes256gcm16,aes128gcm16,chacha20poly1305",
			MSSClamping:       1360,
		}
		if err := db.Create(&defaultSW).Error; err != nil {
			slog.Error("Failed to seed default strongSwan settings", "error", err)
		} else {
			slog.Debug("Default strongSwan settings persisted")
		}
	} else {
		slog.Debug("strongSwan settings already exist, skipping default seeding")
	}
}
