package mfa

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

const minMFAKeyBytes = 16

var (
	keyStoreMu    sync.RWMutex
	activeMFAKey  string
	activeKeyPath string
)

// ResolveKeyFilePath determines the persistent file path storing the independent MFA encryption key.
func ResolveKeyFilePath(dbPath string) string {
	if configured := strings.TrimSpace(os.Getenv("MFA_KEY_FILE")); configured != "" {
		return configured
	}
	if cleanDB := strings.TrimSpace(dbPath); cleanDB != "" {
		return cleanDB + ".mfa-key"
	}
	return ".swanpanel.mfa-key"
}

// ActiveKey returns the active MFA encryption key initialized for the current process,
// falling back to environment override or fallback secret only when keystore is uninitialized.
func ActiveKey(fallbackSecret string) string {
	keyStoreMu.RLock()
	cur := activeMFAKey
	keyStoreMu.RUnlock()
	if cur != "" {
		return cur
	}
	if env := strings.TrimSpace(os.Getenv("MFA_ENCRYPTION_KEY")); env != "" {
		return env
	}
	return fallbackSecret
}

// ActiveKeyPath returns the active key file path registered by InitActiveKey.
func ActiveKeyPath() string {
	keyStoreMu.RLock()
	defer keyStoreMu.RUnlock()
	return activeKeyPath
}

// SetActiveKey stores the active MFA key and key file path in process memory.
func SetActiveKey(key, keyPath string) {
	keyStoreMu.Lock()
	activeMFAKey = key
	if keyPath != "" {
		activeKeyPath = keyPath
	}
	keyStoreMu.Unlock()
}

// ResetKeyStoreForTest clears in-memory MFA key state between isolated unit tests.
func ResetKeyStoreForTest() {
	keyStoreMu.Lock()
	activeMFAKey = ""
	activeKeyPath = ""
	keyStoreMu.Unlock()
}

// GenerateRandomMFAKey creates a cryptographically random 256-bit hex-encoded MFA encryption key.
func GenerateRandomMFAKey() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate MFA encryption key entropy: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// WriteKeyFileAtomic writes the independent MFA key with mode 0600 and fsyncs file and parent directory.
func WriteKeyFileAtomic(keyPath, key string) error {
	clean := strings.TrimSpace(key)
	if len(clean) < minMFAKeyBytes {
		return errors.New("MFA encryption key must be at least 16 bytes")
	}
	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("prepare MFA key directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-mfakey-*")
	if err != nil {
		return fmt.Errorf("stage MFA key file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set MFA key permissions: %w", err)
	}
	if _, err := tmp.WriteString(clean + "\n"); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write MFA key: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync MFA key: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close MFA key: %w", err)
	}
	if err := os.Rename(tmpName, keyPath); err != nil {
		return fmt.Errorf("install MFA key file: %w", err)
	}
	cleanup = false
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// ReadKeyFile validates permissions (strict 0600 in production) and reads a persisted MFA key file.
func ReadKeyFile(keyPath string) (string, error) {
	info, err := os.Stat(keyPath)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("MFA key path is not a regular file")
	}
	if (os.Getenv("APP_ENV") == "production" || os.Getenv("ENVIRONMENT") == "production") && info.Mode().Perm()&0077 != 0 {
		return "", errors.New("MFA key file has unsafe permissions; expected 0600")
	}
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("read MFA key file: %w", err)
	}
	key := strings.TrimSpace(string(raw))
	if len(key) < minMFAKeyBytes {
		return "", errors.New("persisted MFA key is empty or shorter than minimum length")
	}
	return key, nil
}

// ValidateKeyDecryptsDB checks that every persisted encrypted TOTP secret in database
// decrypts cleanly with candidateKey before activating a restored or migrated configuration.
func ValidateKeyDecryptsDB(database *gorm.DB, candidateKey string) error {
	if database == nil {
		return errors.New("database is nil")
	}
	if len(strings.TrimSpace(candidateKey)) < minMFAKeyBytes {
		return errors.New("MFA encryption key missing or invalid")
	}
	var users []db.User
	if err := database.Select("id", "totp_secret_enc", "pending_totp_enc", "mfa_backup_codes").
		Where("totp_secret_enc != '' OR pending_totp_enc != '' OR mfa_backup_codes != ''").
		Find(&users).Error; err != nil {
		return fmt.Errorf("inspect encrypted MFA users: %w", err)
	}
	for _, u := range users {
		if u.TOTPSecretEnc != "" {
			secret, err := DecryptSecret(u.TOTPSecretEnc, candidateKey)
			if err != nil {
				return fmt.Errorf("encrypted TOTP secret for user %s cannot be decrypted with supplied MFA key", u.ID)
			}
			if _, err := decodeBase32Secret(secret); err != nil {
				return fmt.Errorf("decrypted TOTP secret for user %s is invalid", u.ID)
			}
		}
		if u.PendingTOTPEnc != "" {
			if _, err := DecryptSecret(u.PendingTOTPEnc, candidateKey); err != nil {
				return fmt.Errorf("pending TOTP secret for user %s cannot be decrypted with supplied MFA key", u.ID)
			}
		}
	}
	return nil
}

// InitActiveKey resolves, initializes, or safely migrates the persistent independent MFA
// encryption key so MFA secrets are decoupled from JWT_SECRET rotations.
func InitActiveKey(database *gorm.DB, dbPath, legacyJWTSecret string) (string, string, error) {
	keyPath := ResolveKeyFilePath(dbPath)
	migratingPath := keyPath + ".migrating"

	// Recover incomplete crash during legacy migration if migratingPath exists.
	if _, err := os.Stat(keyPath); errors.Is(err, os.ErrNotExist) {
		if migKey, migErr := ReadKeyFile(migratingPath); migErr == nil {
			if database == nil || ValidateKeyDecryptsDB(database, migKey) == nil {
				if renErr := os.Rename(migratingPath, keyPath); renErr == nil {
					slog.Info("Recovered pending MFA encryption key migration")
				}
			}
		}
	}

	// 1. Persisted key file already exists: enforce validity and decryptability.
	if existingKey, err := ReadKeyFile(keyPath); err == nil {
		if database != nil {
			if valErr := ValidateKeyDecryptsDB(database, existingKey); valErr != nil {
				return "", keyPath, fmt.Errorf("persisted MFA key mismatch: %w", valErr)
			}
		}
		SetActiveKey(existingKey, keyPath)
		return existingKey, keyPath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", keyPath, err
	}

	// 2. Key file does not exist yet: inspect DB for existing encrypted MFA rows.
	var encryptedUsers []db.User
	if database != nil {
		if err := database.Where("totp_secret_enc != '' OR pending_totp_enc != ''").Find(&encryptedUsers).Error; err != nil {
			return "", keyPath, fmt.Errorf("query existing MFA state: %w", err)
		}
	}

	envKey := strings.TrimSpace(os.Getenv("MFA_ENCRYPTION_KEY"))

	// 2a. Fresh DB with zero encrypted MFA state: persist envKey (if valid) or generate random key.
	if len(encryptedUsers) == 0 {
		initialKey := envKey
		if initialKey == "" {
			var genErr error
			initialKey, genErr = GenerateRandomMFAKey()
			if genErr != nil {
				return "", keyPath, genErr
			}
		}
		if len(initialKey) < minMFAKeyBytes {
			return "", keyPath, errors.New("MFA_ENCRYPTION_KEY must be at least 16 bytes")
		}
		if err := WriteKeyFileAtomic(keyPath, initialKey); err != nil {
			return "", keyPath, err
		}
		SetActiveKey(initialKey, keyPath)
		slog.Info("Initialized persistent MFA encryption key", "path", keyPath)
		return initialKey, keyPath, nil
	}

	// 2b. Existing MFA users present without key file: safely migrate from envKey or legacy JWT_SECRET.
	var candidates []string
	if envKey != "" {
		candidates = append(candidates, envKey)
	}
	if strings.TrimSpace(legacyJWTSecret) != "" && legacyJWTSecret != envKey {
		candidates = append(candidates, legacyJWTSecret)
	}

	var matchedSource string
	for _, cand := range candidates {
		if ValidateKeyDecryptsDB(database, cand) == nil {
			matchedSource = cand
			break
		}
	}
	if matchedSource == "" {
		return "", keyPath, errors.New("MFA key file missing and existing encrypted MFA secrets could not be decrypted; refusing to generate a replacement key")
	}

	newKey, err := GenerateRandomMFAKey()
	if err != nil {
		return "", keyPath, err
	}
	// Write crash-recoverable migrating file before rewriting DB rows.
	if err := WriteKeyFileAtomic(migratingPath, newKey); err != nil {
		return "", keyPath, err
	}

	if err := database.Transaction(func(tx *gorm.DB) error {
		var rows []db.User
		if err := tx.Where("totp_secret_enc != '' OR pending_totp_enc != ''").Find(&rows).Error; err != nil {
			return err
		}
		for _, u := range rows {
			updates := map[string]any{}
			if u.TOTPSecretEnc != "" {
				plain, decErr := DecryptSecret(u.TOTPSecretEnc, matchedSource)
				if decErr != nil {
					return decErr
				}
				reEnc, encErr := EncryptSecret(plain, newKey)
				if encErr != nil {
					return encErr
				}
				updates["totp_secret_enc"] = reEnc
			}
			if u.PendingTOTPEnc != "" {
				plain, decErr := DecryptSecret(u.PendingTOTPEnc, matchedSource)
				if decErr != nil {
					return decErr
				}
				reEnc, encErr := EncryptSecret(plain, newKey)
				if encErr != nil {
					return encErr
				}
				updates["pending_totp_enc"] = reEnc
			}
			// Legacy recovery code HMACs are keyed on the old secret; clear them cleanly
			// while preserving live TOTP secrets across the migration.
			if u.MFABackupCodes != "" {
				updates["mfa_backup_codes"] = ""
			}
			if len(updates) > 0 {
				if err := tx.Model(&db.User{}).Where("id = ?", u.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		_ = os.Remove(migratingPath)
		return "", keyPath, fmt.Errorf("migrate encrypted MFA secrets: %w", err)
	}

	if err := os.Rename(migratingPath, keyPath); err != nil {
		return "", keyPath, fmt.Errorf("finalize migrated MFA key file: %w", err)
	}
	SetActiveKey(newKey, keyPath)
	slog.Warn("Migrated existing MFA secrets to dedicated persistent key; legacy recovery codes were invalidated and should be regenerated")
	return newKey, keyPath, nil
}
