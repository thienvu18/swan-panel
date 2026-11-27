package mfa

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
)

func TestInitActiveKeyMigratesLegacyJWTSecretAndFailsClosedWithoutKey(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "mfa_mig.db")
	conn, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		ResetKeyStoreForTest()
		if sqlDB, err := conn.DB(); err == nil {
			_ = sqlDB.Close()
		}
		db.DB = nil
	})

	// Seed a user whose TOTP secret was encrypted with legacy JWT_SECRET.
	legacySecret := "legacy-jwt-signing-secret-123456"
	rawSeed, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := EncryptSecret(rawSeed, legacySecret)
	if err != nil {
		t.Fatal(err)
	}
	user := db.User{
		ID:             "usr_legacy_mfa",
		Email:          "legacy-mfa@example.com",
		Name:           "Legacy User",
		AuthType:       "local",
		PasswordHash:   "$2a$10$dummyhashdummyhashdummyhashdummyhashdummyhashdummyha",
		Status:         "ACTIVE",
		MFAEnabled:     true,
		TOTPSecretEnc:  enc,
		MFABackupCodes: `["old-digest"]`,
	}
	if err := conn.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Wrong fallback secret must fail closed rather than generating a replacement key.
	if _, _, err := InitActiveKey(conn, dbPath, "wrong-jwt-secret-000000"); err == nil {
		t.Fatal("expected fail-closed error when key file missing and legacy secret cannot decrypt MFA row")
	}

	// Matching legacy JWT_SECRET migrates cleanly to independent key file and preserves TOTP seed.
	activeKey, keyPath, err := InitActiveKey(conn, dbPath, legacySecret)
	if err != nil {
		t.Fatalf("expected legacy migration to succeed: %v", err)
	}
	if activeKey == legacySecret || len(activeKey) < 32 {
		t.Fatalf("expected independent random MFA key, got len=%d", len(activeKey))
	}

	var migrated db.User
	if err := conn.First(&migrated, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	decrypted, err := DecryptSecret(migrated.TOTPSecretEnc, activeKey)
	if err != nil || decrypted != rawSeed {
		t.Fatalf("migrated TOTP secret mismatch: err=%v decrypted=%q want=%q", err, decrypted, rawSeed)
	}
	if migrated.MFABackupCodes != "" {
		t.Fatalf("expected legacy backup code HMACs cleared on migration, got %q", migrated.MFABackupCodes)
	}

	// Subsequent restart still works even if JWT_SECRET rotates completely.
	ResetKeyStoreForTest()
	reloadedKey, _, err := InitActiveKey(conn, dbPath, "completely-rotated-new-jwt-secret!")
	if err != nil || reloadedKey != activeKey {
		t.Fatalf("expected persisted MFA key across JWT rotation: err=%v", err)
	}

	// Verify 0600 file permissions enforced in production.
	if err := os.Chmod(keyPath, 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "production")
	if _, _, err := InitActiveKey(conn, dbPath, "any"); err == nil {
		t.Fatal("expected overly permissive MFA key file to be rejected in production")
	}
}

func TestResolveAndValidateKeyFileEdgeCases(t *testing.T) {
	ResetKeyStoreForTest()
	defer ResetKeyStoreForTest()

	tmp := t.TempDir()
	customPath := filepath.Join(tmp, "custom.mfa-key")
	t.Setenv("MFA_KEY_FILE", customPath)
	if got := ResolveKeyFilePath(filepath.Join(tmp, "db.sqlite")); got != customPath {
		t.Fatalf("expected MFA_KEY_FILE override %q, got %q", customPath, got)
	}

	if err := WriteKeyFileAtomic(customPath, "too-short"); err == nil {
		t.Fatal("expected short MFA key write rejected")
	}
	if err := os.WriteFile(customPath, []byte("short\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadKeyFile(customPath); err == nil {
		t.Fatal("expected short persisted MFA key rejected on read")
	}
	if _, err := ReadKeyFile(tmp); err == nil {
		t.Fatal("expected directory path rejected by ReadKeyFile")
	}

	dbPath := filepath.Join(tmp, "val.db")
	conn, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := conn.DB(); err == nil {
			_ = sqlDB.Close()
		}
		db.DB = nil
	})
	seed, _ := GenerateSecret()
	enc, _ := EncryptSecret(seed, "correct-key-material-12345")
	_ = conn.Create(&db.User{
		ID:            "usr_val",
		Email:         "val@example.com",
		Name:          "Val",
		AuthType:      "local",
		PasswordHash:  "hash",
		Status:        "ACTIVE",
		MFAEnabled:    true,
		TOTPSecretEnc: enc,
	}).Error
	if err := ValidateKeyDecryptsDB(conn, "wrong-key-material-987654"); err == nil {
		t.Fatal("expected ValidateKeyDecryptsDB to reject mismatched key")
	}
	if err := ValidateKeyDecryptsDB(conn, "correct-key-material-12345"); err != nil {
		t.Fatalf("expected matching key to validate: %v", err)
	}
	SetActiveKey("correct-key-material-12345", customPath)
	if ActiveKeyPath() != customPath || ActiveKey("fallback") != "correct-key-material-12345" {
		t.Fatalf("unexpected active key state")
	}

	// Calling InitActiveKey with an existing valid key file matching encrypted DB succeeds directly.
	if err := WriteKeyFileAtomic(customPath, "correct-key-material-12345"); err != nil {
		t.Fatal(err)
	}
	gotKey, gotPath, err := InitActiveKey(conn, dbPath, "unused-legacy-jwt")
	if err != nil || gotKey != "correct-key-material-12345" || gotPath != customPath {
		t.Fatalf("expected InitActiveKey to accept valid persisted key, got err=%v path=%q", err, gotPath)
	}

	// Mismatched existing key file against encrypted TOTP row fails closed.
	if err := WriteKeyFileAtomic(customPath, "mismatched-key-material-999999"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InitActiveKey(conn, dbPath, "also-wrong-legacy"); err == nil {
		t.Fatal("expected InitActiveKey to fail closed when existing key cannot decrypt DB secrets")
	}
}
