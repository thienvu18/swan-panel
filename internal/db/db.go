package db

import (
	"fmt"
	"log/slog"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
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

	if err := db.Exec("PRAGMA journal_mode = WAL;").Error; err != nil {
		slog.Warn("Failed to configure PRAGMA journal_mode = WAL", "error", err)
	} else {
		slog.Debug("PRAGMA journal_mode = WAL configured successfully")
	}

	if err := db.Exec("PRAGMA foreign_keys = ON;").Error; err != nil {
		slog.Warn("Failed to configure PRAGMA foreign_keys = ON", "error", err)
	} else {
		slog.Debug("PRAGMA foreign_keys = ON configured successfully")
	}

	if err := db.Exec("PRAGMA busy_timeout = 5000;").Error; err != nil {
		slog.Warn("Failed to configure PRAGMA busy_timeout = 5000", "error", err)
	} else {
		slog.Debug("PRAGMA busy_timeout = 5000 configured successfully")
	}

	// Automatically migrate all relational tables
	slog.Debug("Starting database auto-migration for core models")
	err = db.AutoMigrate(
		&OIDCProvider{},
		&User{},
		&UserGroup{},
		&ResourceGroup{},
		&Device{},
		&AccessRule{},
		&DNSSettings{},
		&DNSOverride{},
		&StrongswanSettings{},
	)
	if err != nil {
		slog.Error("Auto-migration failed", "error", err)
		return nil, fmt.Errorf("failed to auto-migrate schema: %w", err)
	}
	slog.Info("Schema auto-migration completed successfully")

	DB = db

	// Seed initial baseline records if fresh database
	seedDefaults(db)

	return db, nil
}

func seedDefaults(db *gorm.DB) {
	// 1. Check & seed default Root Admin account
	var userCount int64
	if err := db.Model(&User{}).Count(&userCount).Error; err != nil {
		slog.Error("Failed to query user count during database initialization", "error", err)
	} else if userCount == 0 {
		slog.Info("Empty user table detected; bootstrapping default Root Admin credentials")
		hash, err := bcrypt.GenerateFromPassword([]byte("admin123456"), bcrypt.DefaultCost)
		if err != nil {
			slog.Error("Failed to generate bcrypt password hash for default admin", "error", err)
		} else {
			defaultAdmin := User{
				ID:           "usr_root_admin",
				Email:        "admin@swanpanel.internal",
				Name:         "System Administrator",
				AuthType:     "local",
				PasswordHash: string(hash),
				IsAdmin:      true,
				Status:       "ACTIVE",
			}
			if err := db.Create(&defaultAdmin).Error; err != nil {
				slog.Error("Failed to seed default Root Admin account", "error", err)
			} else {
				slog.Info("Default Root Admin initialized successfully", "email", defaultAdmin.Email)
			}
		}
	} else {
		slog.Debug("User table already populated, skipping root admin seeding", "user_count", userCount)
	}

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
