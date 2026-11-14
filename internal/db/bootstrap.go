package db

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var bootstrapTokenFile string

// BootstrapTokenFile is the host-only path to the initial setup token.
func BootstrapTokenFile() string { return bootstrapTokenFile }

func bootstrapHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

// initializeBootstrap never creates a default account. The token only exists
// while the users table is empty; it is generated once, not every restart.
func initializeBootstrap(database *gorm.DB, dbPath string) error {
	bootstrapTokenFile = dbPath + ".setup-token"
	if configured := strings.TrimSpace(os.Getenv("BOOTSTRAP_TOKEN_FILE")); configured != "" {
		bootstrapTokenFile = configured
	}
	var count int64
	if err := database.Model(&User{}).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return nil
	}

	var state BootstrapState
	if err := database.First(&state, 1).Error; err == nil {
		stat, err := os.Stat(bootstrapTokenFile)
		if err != nil {
			return fmt.Errorf("one-time setup token file missing: %w", err)
		}
		if stat.Mode().Perm()&0077 != 0 {
			return errors.New("setup token file has unsafe permissions")
		}
		slog.Warn("Initial administrator setup required", "token_file", bootstrapTokenFile)
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return err
	}
	token := hex.EncodeToString(entropy)
	file, err := os.OpenFile(bootstrapTokenFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create setup token file: %w", err)
	}
	if _, err = file.WriteString(token + "\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(bootstrapTokenFile)
		return err
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(bootstrapTokenFile)
		return err
	}
	if err := database.Create(&BootstrapState{ID: 1, TokenHash: bootstrapHash(token)}).Error; err != nil {
		_ = os.Remove(bootstrapTokenFile)
		return err
	}
	slog.Warn("Initial administrator setup required; read the one-time token locally", "token_file", bootstrapTokenFile)
	return nil
}

// SetupRequired becomes false permanently once the initial account exists.
func SetupRequired() (bool, error) {
	var users int64
	if err := DB.Model(&User{}).Count(&users).Error; err != nil {
		return false, err
	}
	return users == 0, nil
}

// CreateInitialAdmin consumes the bootstrap token in the same transaction as
// creation of the initial root administrator. Only the token's hash is stored.
func CreateInitialAdmin(token, name, email, password string) (*User, error) {
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	if name == "" || email == "" {
		return nil, errors.New("administrator name and email required")
	}
	if len(password) < 12 || len(password) > 72 {
		return nil, errors.New("password must be 12–72 bytes")
	}
	var created User
	err := DB.Transaction(func(tx *gorm.DB) error {
		var users int64
		if err := tx.Model(&User{}).Count(&users).Error; err != nil {
			return err
		}
		if users != 0 {
			return errors.New("setup already completed")
		}
		var state BootstrapState
		if err := tx.First(&state, 1).Error; err != nil {
			return errors.New("setup token not available")
		}
		if subtle.ConstantTimeCompare([]byte(state.TokenHash), []byte(bootstrapHash(token))) != 1 {
			return errors.New("invalid setup token")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		created = User{
			ID: "usr_" + uuid.NewString(), Name: name, Email: email, PasswordHash: string(hash),
			AuthType: "local", IsAdmin: true, Status: "ACTIVE",
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		return tx.Delete(&BootstrapState{}, 1).Error
	})
	if err != nil {
		return nil, err
	}
	if err := os.Remove(bootstrapTokenFile); err != nil && !os.IsNotExist(err) {
		slog.Warn("Setup token hash consumed; remove stale token file manually", "path", bootstrapTokenFile, "error", err)
	}
	return &created, nil
}
