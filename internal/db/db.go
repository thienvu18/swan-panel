package db

import (
	"fmt"
	"log/slog"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB initializes SQLite connection with WAL mode, foreign keys, and runs Auto-Migrate
func InitDB(dbPath string) (*gorm.DB, error) {
	slog.Debug("Opening SQLite database connection", "path", dbPath)

	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
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
	)
	if err != nil {
		slog.Error("Auto-migration failed", "error", err)
		return nil, fmt.Errorf("failed to auto-migrate schema: %w", err)
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

	return db, nil
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
