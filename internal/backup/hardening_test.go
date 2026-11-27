package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"github.com/swan-panel/swan-panel/internal/mfa"
)

func TestBackupAndRestoreHardeningFaultInjectionAndPortableMFA(t *testing.T) {
	maintenance.DefaultCoordinator.ResetForTest()
	defer maintenance.DefaultCoordinator.ResetForTest()
	defer mfa.ResetKeyStoreForTest()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "hostA", "swanpanel.db")
	pkiDir := filepath.Join(tmp, "hostA_pki")
	_ = os.MkdirAll(filepath.Dir(dbPath), 0700)
	generateTestCAFiles(t, pkiDir, "Host A CA")

	connA, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("init dbA: %v", err)
	}
	t.Cleanup(func() {
		if db.DB != nil {
			if s, err := db.DB.DB(); err == nil {
				_ = s.Close()
			}
		}
		db.DB = nil
	})

	keyA, keyPathA, err := mfa.InitActiveKey(connA, dbPath, "hostA-jwt-secret-1234567890")
	if err != nil {
		t.Fatalf("init mfa keyA: %v", err)
	}
	totpSeed, _ := mfa.GenerateSecret()
	encSeed, _ := mfa.EncryptSecret(totpSeed, keyA)
	_, backupDigests, _ := mfa.GenerateBackupCodes(keyA)
	user := db.User{
		ID:             "usr_mfa_1",
		Email:          "mfa-portable@example.com",
		Name:           "Portable MFA",
		AuthType:       "local",
		PasswordHash:   "$2a$10$dummyhashdummyhashdummyhashdummyhashdummyhashdummyha",
		Status:         "ACTIVE",
		MFAEnabled:     true,
		TOTPSecretEnc:  encSeed,
		MFABackupCodes: backupDigests,
	}
	if err := connA.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	epochBefore, _ := db.CurrentSessionEpoch(connA)
	// Seed a pending outbox row inside archive so restore cancellation can be verified.
	_ = connA.Create(&db.WebhookDelivery{
		ID:            "whd_stale_snapshot",
		EventID:       "aud_1",
		WebhookID:     "wh_1",
		Status:        audit.DeliveryPending,
		MaxAttempts:   5,
		NextAttemptAt: time.Now().UTC(),
		Action:        "device.created",
		Severity:      audit.SeverityInfo,
		EventJSON:     "{}",
	}).Error

	passphrase := "portable-mfa-passphrase-2026"
	archive, manifest, err := CreateEncryptedBackup(context.Background(), connA, pkiDir, passphrase)
	if err != nil {
		t.Fatalf("CreateEncryptedBackup failed: %v", err)
	}
	if !manifest.HasMFAKey || manifest.FormatVersion != CurrentBackupFormatVersion {
		t.Fatalf("expected v2 manifest with HasMFAKey=true, got %+v", manifest)
	}
	// Verify plaintext MFA key does not appear in manifest JSON.
	rawManifest, _ := json.Marshal(manifest)
	if bytes.Contains(rawManifest, []byte(keyA)) {
		t.Fatal("plaintext MFA key leaked in manifest")
	}

	// Simulate fresh host B with DIFFERENT DB path, DIFFERENT PKI path, DIFFERENT JWT secret & MFA key.
	if sqlDB, err := connA.DB(); err == nil {
		_ = sqlDB.Close()
	}
	db.DB = nil
	mfa.ResetKeyStoreForTest()

	dbPathB := filepath.Join(tmp, "hostB", "swanpanel.db")
	pkiDirB := filepath.Join(tmp, "hostB_pki")
	_ = os.MkdirAll(filepath.Dir(dbPathB), 0700)
	generateTestCAFiles(t, pkiDirB, "Host B Original CA")
	connB, err := db.InitDB(dbPathB)
	if err != nil {
		t.Fatalf("init dbB: %v", err)
	}
	keyB, keyPathB, err := mfa.InitActiveKey(connB, dbPathB, "hostB-different-jwt-secret!!")
	if err != nil || keyB == keyA {
		t.Fatalf("expected independent host B key: err=%v", err)
	}
	// Create a sentinel record on Host B to verify rollback preserves Host B on failure.
	_ = connB.Create(&db.DNSOverride{ID: "dns_hostB", Hostname: "hostb-sentinel.internal", RecordType: "A", TargetAddress: "10.55.0.1"}).Error

	// Fault injection 1: crash right after PrepareRestore leaves state="prepared", next startup applies cleanly!
	// First inject an error during OnBeforeReopenDB to prove crash-safe rollback back to Host B.
	_, err = PrepareRestore(context.Background(), archive, passphrase, dbPathB, pkiDirB, RestoreMetadata{
		ActorEmail: "admin@example.com",
		ActorRole:  "admin",
		MFAKeyPath: keyPathB,
	})
	if err != nil {
		t.Fatalf("PrepareRestore failed: %v", err)
	}
	_, err = RecoverOrApplyPendingRestore(dbPathB, pkiDirB, nil, &RestoreFaultHooks{
		OnBeforeReopenDB: func() error {
			return errors.New("simulated disk fault before reopen")
		},
	})
	if err == nil {
		t.Fatal("expected injected reopen failure to return error")
	}
	// Verify Host B sentinel DB & CA survived rollback untouched.
	var sentinel db.DNSOverride
	if err := db.DB.First(&sentinel, "hostname = ?", "hostb-sentinel.internal").Error; err != nil {
		t.Fatalf("Host B DB was not restored by rollback: %v", err)
	}
	caB, _ := os.ReadFile(filepath.Join(pkiDirB, "ca.crt"))
	if !bytes.Contains(caB, []byte("CERTIFICATE")) {
		t.Fatal("Host B CA missing after rollback")
	}
	keyBAfterRollback, _ := mfa.ReadKeyFile(keyPathB)
	if keyBAfterRollback != keyB {
		t.Fatalf("Host B MFA key altered after rollback: got=%q want=%q", keyBAfterRollback, keyB)
	}

	// Fault injection 2: rollback failure locks Fail-Closed state and retains work dirs!
	_, err = PrepareRestore(context.Background(), archive, passphrase, dbPathB, pkiDirB, RestoreMetadata{MFAKeyPath: keyPathB})
	if err != nil {
		t.Fatal(err)
	}
	_, err = RecoverOrApplyPendingRestore(dbPathB, pkiDirB, nil, &RestoreFaultHooks{
		OnBeforeReplaceDB: func() error { return errors.New("db write fail") },
		OnRollbackDB:      func() error { return errors.New("disk offline during rollback") },
	})
	if !errors.Is(err, ErrRollbackFailed) {
		t.Fatalf("expected ErrRollbackFailed, got %v", err)
	}
	if maintenance.DefaultCoordinator.CurrentStatus().Mode != maintenance.ModeFailedClosed {
		t.Fatalf("expected ModeFailedClosed, got %s", maintenance.DefaultCoordinator.CurrentStatus().Mode)
	}
	if _, statErr := os.Stat(journalPathForDB(dbPathB)); statErr != nil {
		t.Fatalf("expected rollback journal retained on rollback failure: %v", statErr)
	}
	// Preparing another restore must refuse while fail-closed journal exists.
	if _, err := PrepareRestore(context.Background(), archive, passphrase, dbPathB, pkiDirB, RestoreMetadata{}); !errors.Is(err, ErrRestoreJournalConflict) {
		t.Fatalf("expected ErrRestoreJournalConflict while fail-closed, got %v", err)
	}
	// Clear fail-closed state to test clean startup recovery completion.
	maintenance.DefaultCoordinator.ResetForTest()
	_ = os.RemoveAll(DBWorkDir(dbPathB))
	_ = os.RemoveAll(PKIWorkDir(pkiDirB))
	_, _ = db.InitDB(dbPathB)

	// Normal two-step D01 flow: PrepareRestore -> simulated controlled restart -> RecoverOrApplyPendingRestore!
	_, err = PrepareRestore(context.Background(), archive, passphrase, dbPathB, pkiDirB, RestoreMetadata{
		ActorEmail: "admin@example.com",
		ActorRole:  "admin",
		MFAKeyPath: keyPathB,
	})
	if err != nil {
		t.Fatalf("PrepareRestore for D01 completion failed: %v", err)
	}
	appliedManifest, err := RecoverOrApplyPendingRestore(dbPathB, pkiDirB, nil, nil)
	if err != nil || appliedManifest == nil {
		t.Fatalf("expected startup recovery apply to succeed: %v", err)
	}

	// Verify MFA user decrypts seamlessly with restored independent key on Host B!
	var restoredUser db.User
	if err := db.DB.First(&restoredUser, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("restored MFA user missing: %v", err)
	}
	decSeed, err := mfa.DecryptSecret(restoredUser.TOTPSecretEnc, mfa.ActiveKey(""))
	if err != nil || decSeed != totpSeed {
		t.Fatalf("restored MFA seed mismatch on Host B: err=%v dec=%q want=%q", err, decSeed, totpSeed)
	}

	// Verify session epoch rotated on restore (invalidates pre-restore tokens!).
	epochAfter, _ := db.CurrentSessionEpoch(db.DB)
	if epochAfter == "" || epochAfter == epochBefore {
		t.Fatalf("expected rotated session epoch after restore: before=%q after=%q", epochBefore, epochAfter)
	}

	// Verify restored pending outbox row was cancelled cleanly (not replayed!).
	var staleOutbox db.WebhookDelivery
	if err := db.DB.First(&staleOutbox, "id = ?", "whd_stale_snapshot").Error; err != nil {
		t.Fatalf("expected cancelled delivery row preserved for inspection: %v", err)
	}
	if staleOutbox.Status != audit.DeliveryCancelled {
		t.Fatalf("expected stale restored delivery marked cancelled, got %s", staleOutbox.Status)
	}

	// Verify backup.restored audit event was recorded cleanly and hash chain passes!
	chain, err := audit.VerifyChain(context.Background(), db.DB)
	if err != nil || !chain.Valid || chain.Total == 0 {
		t.Fatalf("expected valid hash chain post-restore, got %+v err=%v", chain, err)
	}
	_ = keyPathA
}

func TestBackupRejectsLegacyV1WithMFAAndEnforcesResourceLimits(t *testing.T) {
	maintenance.DefaultCoordinator.ResetForTest()
	defer maintenance.DefaultCoordinator.ResetForTest()
	defer mfa.ResetKeyStoreForTest()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "limits.db")
	pkiDir := filepath.Join(tmp, "pki")
	generateTestCAFiles(t, pkiDir, "Limits CA")
	conn, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if db.DB != nil {
			if s, err := db.DB.DB(); err == nil {
				_ = s.Close()
			}
		}
		db.DB = nil
	})

	// Seed encrypted MFA row in DB, then craft a legacy v1 archive without secrets/mfa.key.
	_ = conn.Create(&db.User{
		ID:            "usr_v1_mfa",
		Email:         "v1-mfa@example.com",
		Name:          "V1 MFA",
		AuthType:      "local",
		PasswordHash:  "hash",
		Status:        "ACTIVE",
		MFAEnabled:    true,
		TOTPSecretEnc: "encrypted-placeholder",
	}).Error
	if sqlDB, err := conn.DB(); err == nil {
		_ = sqlDB.Close()
	}
	rawDB, _ := os.ReadFile(dbPath)
	dbHash := sha256.Sum256(rawDB)
	caCert, _ := os.ReadFile(filepath.Join(pkiDir, "ca.crt"))
	caKey, _ := os.ReadFile(filepath.Join(pkiDir, "ca.key"))
	v1Manifest, _ := json.Marshal(Manifest{
		FormatVersion: 1,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		DBSHA256:      hex.EncodeToString(dbHash[:]),
		PKIFiles:      []string{"ca.crt", "ca.key"},
	})
	passphrase := "limits-and-legacy-passphrase"
	legacyV1Archive := encryptSyntheticArchive(t, passphrase, func(tw *tar.Writer) {
		for _, e := range []struct {
			n string
			d []byte
		}{
			{"manifest.json", v1Manifest},
			{"database/swanpanel.db", rawDB},
			{"pki/ca.crt", caCert},
			{"pki/ca.key", caKey},
		} {
			_ = tw.WriteHeader(&tar.Header{Name: e.n, Mode: 0600, Size: int64(len(e.d)), Typeflag: tar.TypeReg})
			_, _ = tw.Write(e.d)
		}
	})
	_, _ = db.InitDB(dbPath)

	if _, err := PrepareRestore(context.Background(), legacyV1Archive, passphrase, dbPath, pkiDir, RestoreMetadata{}); !errors.Is(err, ErrIncompatibleMFAArchive) {
		t.Fatalf("expected ErrIncompatibleMFAArchive for legacy v1 with MFA secrets, got %v", err)
	}

	// Duplicate tar entries must be rejected.
	dupArchive := encryptSyntheticArchive(t, passphrase, func(tw *tar.Writer) {
		for _, e := range []struct {
			n string
			d []byte
		}{
			{"manifest.json", v1Manifest},
			{"manifest.json", v1Manifest},
			{"database/swanpanel.db", rawDB},
		} {
			_ = tw.WriteHeader(&tar.Header{Name: e.n, Mode: 0600, Size: int64(len(e.d)), Typeflag: tar.TypeReg})
			_, _ = tw.Write(e.d)
		}
	})
	if _, err := PrepareRestore(context.Background(), dupArchive, passphrase, dbPath, pkiDir, RestoreMetadata{}); !errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("expected ErrInvalidArchive for duplicate entry, got %v", err)
	}

	// Exceeding MaxBackupEntries must be rejected.
	excessArchive := encryptSyntheticArchive(t, passphrase, func(tw *tar.Writer) {
		for i := 0; i <= MaxBackupEntries; i++ {
			_ = tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("pki/f_%04d", i), Mode: 0600, Size: 1, Typeflag: tar.TypeReg})
			_, _ = tw.Write([]byte("x"))
		}
	})
	if _, err := PrepareRestore(context.Background(), excessArchive, passphrase, dbPath, pkiDir, RestoreMetadata{}); !errors.Is(err, ErrBackupLimitExceeded) {
		t.Fatalf("expected ErrBackupLimitExceeded for excessive entry count, got %v", err)
	}

	// Exceeding MaxBackupCipherBytes must be rejected immediately.
	oversize := make([]byte, MaxBackupCipherBytes+1)
	copy(oversize, []byte(backupMagic))
	if _, err := PrepareRestore(context.Background(), oversize, passphrase, dbPath, pkiDir, RestoreMetadata{}); !errors.Is(err, ErrBackupLimitExceeded) {
		t.Fatalf("expected ErrBackupLimitExceeded for oversized cipher archive, got %v", err)
	}

	// Insufficient disk space check rejects restore without touching live state.
	// Remove MFA row so a clean v2 archive passes MFA validation and reaches disk check.
	_ = db.DB.Where("1 = 1").Delete(&db.User{}).Error
	cleanArchive, _, err := CreateEncryptedBackup(context.Background(), db.DB, pkiDir, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	restoreStatfs := SetStatfsForTest(func(s string, st *syscall.Statfs_t) error {
		st.Bavail = 1
		st.Bsize = 512
		return nil
	})
	defer restoreStatfs()
	if _, err := PrepareRestore(context.Background(), cleanArchive, passphrase, dbPath, pkiDir, RestoreMetadata{}); err == nil {
		t.Fatal("expected insufficient disk space error")
	}
}

func TestRestoreRollbackFaultFailClosedAndJournalRecovery(t *testing.T) {
	maintenance.DefaultCoordinator.ResetForTest()
	defer maintenance.DefaultCoordinator.ResetForTest()
	defer mfa.ResetKeyStoreForTest()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "rollback_test.db")
	pkiDir := filepath.Join(tmp, "pki")
	generateTestCAFiles(t, pkiDir, "Rollback CA")

	conn, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if db.DB != nil {
			if s, err := db.DB.DB(); err == nil {
				_ = s.Close()
			}
		}
		db.DB = nil
	})
	if _, _, err := mfa.InitActiveKey(conn, dbPath, "legacy-secret-12345"); err != nil {
		t.Fatal(err)
	}
	archive, _, err := CreateEncryptedBackup(context.Background(), conn, pkiDir, "rollback-pass-2026")
	if err != nil {
		t.Fatal(err)
	}

	// PrepareRestore succeeds, then startup swap fault triggers clean rollback.
	if _, err := PrepareRestore(context.Background(), archive, "rollback-pass-2026", dbPath, pkiDir, RestoreMetadata{
		ActorEmail: "admin@example.com",
		ActorRole:  "admin",
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	}); err != nil {
		t.Fatalf("PrepareRestore failed: %v", err)
	}
	if _, err := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, &RestoreFaultHooks{
		OnBeforeReplacePKI: func() error { return errors.New("simulated swap failure") },
	}); err == nil {
		t.Fatal("expected swap error to fail and roll back")
	}

	// Second case: when both swap and rollback fail, ErrRollbackFailed must engage fail-closed lock.
	if _, err := PrepareRestore(context.Background(), archive, "rollback-pass-2026", dbPath, pkiDir, RestoreMetadata{
		ActorEmail: "admin@example.com",
		ActorRole:  "admin",
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	}); err != nil {
		t.Fatalf("second PrepareRestore failed: %v", err)
	}
	_, err = RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, &RestoreFaultHooks{
		OnBeforeReplaceDB: func() error { return errors.New("swap db error") },
		OnRollbackDB:      func() error { return errors.New("rollback db error") },
	})
	if !errors.Is(err, ErrRollbackFailed) {
		t.Fatalf("expected ErrRollbackFailed on double fault, got %v", err)
	}
	if maintenance.DefaultCoordinator.CurrentStatus().Mode != maintenance.ModeFailedClosed {
		t.Fatalf("expected fail-closed maintenance mode, got %s", maintenance.DefaultCoordinator.CurrentStatus().Mode)
	}
}

func TestBackupValidationRejectsCorruptDBBadPKIAndUnsafePaths(t *testing.T) {
	maintenance.DefaultCoordinator.ResetForTest()
	defer maintenance.DefaultCoordinator.ResetForTest()
	defer mfa.ResetKeyStoreForTest()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "val_err.db")
	pkiDir := filepath.Join(tmp, "pki")
	generateTestCAFiles(t, pkiDir, "Validation CA")

	if _, _, err := CreateEncryptedBackup(context.Background(), nil, pkiDir, "valid-passphrase-2026"); err == nil {
		t.Fatal("expected nil database to fail backup creation")
	}

	conn, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if db.DB != nil {
			if s, err := db.DB.DB(); err == nil {
				_ = s.Close()
			}
		}
		db.DB = nil
	})
	if _, _, err := CreateEncryptedBackup(context.Background(), conn, pkiDir, "short"); err == nil {
		t.Fatal("expected short passphrase to fail backup creation")
	}

	// Safe archive path edge cases.
	for _, unsafe := range []string{"", "a\x00b", `a\b`, "/abs/path", ".", "..", "pki/../../secret"} {
		if isSafeArchivePath(unsafe) {
			t.Fatalf("expected unsafe path rejected: %q", unsafe)
		}
	}
	if !isSafeArchivePath("pki/ca.crt") {
		t.Fatal("expected pki/ca.crt safe")
	}

	// Direct database validation rejections (corrupt bytes & missing required table).
	badSQLite := filepath.Join(tmp, "corrupt.db")
	_ = os.WriteFile(badSQLite, []byte("not a sqlite database"), 0600)
	if _, err := validateExtractedDatabase(badSQLite); err == nil {
		t.Fatal("expected corrupt sqlite file rejected")
	}

	// PKI validation edge cases (missing ca.crt, missing ca.key, bad PEMs).
	emptyPKI := filepath.Join(tmp, "empty_pki")
	_ = os.MkdirAll(emptyPKI, 0700)
	if err := validateExtractedPKI(emptyPKI); err == nil {
		t.Fatal("expected missing ca.crt rejected")
	}
	_ = os.WriteFile(filepath.Join(emptyPKI, "ca.crt"), []byte("bad pem"), 0600)
	if err := validateExtractedPKI(emptyPKI); err == nil {
		t.Fatal("expected missing ca.key rejected")
	}
	_ = os.WriteFile(filepath.Join(emptyPKI, "ca.key"), []byte("bad key"), 0600)
	if err := validateExtractedPKI(emptyPKI); err == nil {
		t.Fatal("expected bad ca.crt PEM rejected")
	}
	caCert, _ := os.ReadFile(filepath.Join(pkiDir, "ca.crt"))
	_ = os.WriteFile(filepath.Join(emptyPKI, "ca.crt"), caCert, 0600)
	if err := validateExtractedPKI(emptyPKI); err == nil {
		t.Fatal("expected bad ca.key PEM rejected")
	}

	// Missing manifest.json rejected cleanly during PrepareRestore.
	noManifest := encryptSyntheticArchive(t, "no-manifest-pass-2026", func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "pki/ca.crt", Mode: 0600, Size: int64(len(caCert)), Typeflag: tar.TypeReg})
		_, _ = tw.Write(caCert)
	})
	if _, err := PrepareRestore(context.Background(), noManifest, "no-manifest-pass-2026", dbPath, pkiDir, RestoreMetadata{}); err == nil {
		t.Fatal("expected archive missing manifest.json rejected")
	}

	// Archive with symlink entry must be rejected cleanly.
	symlinkArchive := encryptSyntheticArchive(t, "symlink-pass-2026", func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "pki/evil-link", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink, Mode: 0777})
	})
	if _, err := PrepareRestore(context.Background(), symlinkArchive, "symlink-pass-2026", dbPath, pkiDir, RestoreMetadata{}); err == nil {
		t.Fatal("expected symlink entry rejected")
	}

	// Archive with path traversal tar entry must be rejected.
	traversalArchive := encryptSyntheticArchive(t, "trav-pass-2026", func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "../outside", Mode: 0600, Size: 1, Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte("x"))
	})
	if _, err := PrepareRestore(context.Background(), traversalArchive, "trav-pass-2026", dbPath, pkiDir, RestoreMetadata{}); err == nil {
		t.Fatal("expected traversal tar entry rejected")
	}

	// Archive missing database entry or with DB SHA-256 mismatch rejected.
	if sqlDB, err := conn.DB(); err == nil {
		_ = sqlDB.Close()
	}
	rawDB, _ := os.ReadFile(dbPath)
	caKey, _ := os.ReadFile(filepath.Join(pkiDir, "ca.key"))
	badManifest, _ := json.Marshal(Manifest{
		FormatVersion: CurrentBackupFormatVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		DBSHA256:      strings.Repeat("f", 64),
		PKIFiles:      []string{"ca.crt", "ca.key"},
	})
	hashMismatchArchive := encryptSyntheticArchive(t, "mismatch-pass-2026", func(tw *tar.Writer) {
		for _, e := range []struct {
			n string
			d []byte
		}{
			{"manifest.json", badManifest},
			{"database/swanpanel.db", rawDB},
			{"pki/ca.crt", caCert},
			{"pki/ca.key", caKey},
		} {
			_ = tw.WriteHeader(&tar.Header{Name: e.n, Mode: 0600, Size: int64(len(e.d)), Typeflag: tar.TypeReg})
			_, _ = tw.Write(e.d)
		}
	})
	if _, err := PrepareRestore(context.Background(), hashMismatchArchive, "mismatch-pass-2026", dbPath, pkiDir, RestoreMetadata{}); err == nil {
		t.Fatal("expected DB SHA256 mismatch rejected")
	}

	// Unexpected extra PKI entry absent from manifest rejected.
	sum := sha256.Sum256(rawDB)
	goodManifest, _ := json.Marshal(Manifest{
		FormatVersion: CurrentBackupFormatVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		DBSHA256:      hex.EncodeToString(sum[:]),
		PKIFiles:      []string{"ca.crt"},
	})
	extraFileArchive := encryptSyntheticArchive(t, "extra-pass-2026", func(tw *tar.Writer) {
		for _, e := range []struct {
			n string
			d []byte
		}{
			{"manifest.json", goodManifest},
			{"database/swanpanel.db", rawDB},
			{"pki/ca.crt", caCert},
			{"pki/ca.key", caKey},
		} {
			_ = tw.WriteHeader(&tar.Header{Name: e.n, Mode: 0600, Size: int64(len(e.d)), Typeflag: tar.TypeReg})
			_, _ = tw.Write(e.d)
		}
	})
	if _, err := PrepareRestore(context.Background(), extraFileArchive, "extra-pass-2026", dbPath, pkiDir, RestoreMetadata{}); err == nil {
		t.Fatal("expected manifest vs extracted PKI mismatch rejected")
	}
}

func TestRestoreValidationHandlesMissingFilesAndStaleJournals(t *testing.T) {
	maintenance.DefaultCoordinator.ResetForTest()
	defer maintenance.DefaultCoordinator.ResetForTest()
	defer mfa.ResetKeyStoreForTest()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "stale_test.db")
	pkiDir := filepath.Join(tmp, "pki")
	generateTestCAFiles(t, pkiDir, "Stale Journal CA")

	conn, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if db.DB != nil {
			if s, err := db.DB.DB(); err == nil {
				_ = s.Close()
			}
		}
		db.DB = nil
	})
	if _, _, err := mfa.InitActiveKey(conn, dbPath, "legacy-secret-12345"); err != nil {
		t.Fatal(err)
	}
	archive, _, err := CreateEncryptedBackup(context.Background(), conn, pkiDir, "stale-pass-2026")
	if err != nil {
		t.Fatal(err)
	}

	// Malformed manifest JSON and missing DB file rejected cleanly.
	badJSONManifest := encryptSyntheticArchive(t, "badjson-pass-2026", func(tw *tar.Writer) {
		d := []byte("{bad json")
		_ = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(d)), Typeflag: tar.TypeReg})
		_, _ = tw.Write(d)
	})
	if _, err := PrepareRestore(context.Background(), badJSONManifest, "badjson-pass-2026", dbPath, pkiDir, RestoreMetadata{}); err == nil {
		t.Fatal("expected invalid JSON manifest rejected")
	}
	validMan, _ := json.Marshal(Manifest{FormatVersion: CurrentBackupFormatVersion, DBSHA256: strings.Repeat("a", 64)})
	noDBArchive := encryptSyntheticArchive(t, "nodb-pass-2026", func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(validMan)), Typeflag: tar.TypeReg})
		_, _ = tw.Write(validMan)
	})
	if _, err := PrepareRestore(context.Background(), noDBArchive, "nodb-pass-2026", dbPath, pkiDir, RestoreMetadata{}); err == nil {
		t.Fatal("expected archive without database rejected")
	}

	// Inject reopen-DB fault during startup activation and verify rollback restores live DB cleanly.
	if _, err := PrepareRestore(context.Background(), archive, "stale-pass-2026", dbPath, pkiDir, RestoreMetadata{
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, &RestoreFaultHooks{
		OnBeforeReopenDB: func() error { return errors.New("reopen hook fail") },
	}); err == nil {
		t.Fatal("expected OnBeforeReopenDB fault to roll back")
	}

	// Unrecognized journal state returns an explicit fail-closed error, while Committed cleans up and returns manifest.
	_ = os.MkdirAll(DBWorkDir(dbPath), 0700)
	_ = os.WriteFile(filepath.Join(DBWorkDir(dbPath), "journal.json"), []byte(`{"state":"staged"}`), 0600)
	if _, err := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, nil); !errors.Is(err, ErrRecoveryFailed) {
		t.Fatalf("expected ErrRecoveryFailed for unrecognized journal state, got %v", err)
	}
	if maintenance.DefaultCoordinator.CurrentStatus().Mode != maintenance.ModeFailedClosed {
		t.Fatalf("expected fail-closed mode after unrecognized journal state, got %s", maintenance.DefaultCoordinator.CurrentStatus().Mode)
	}
	maintenance.DefaultCoordinator.ResetForTest()
	_ = os.WriteFile(filepath.Join(DBWorkDir(dbPath), "journal.json"), []byte(`{"state":"committed","manifest":{"format_version":2}}`), 0600)
	if m, err := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, nil); err != nil || m == nil {
		t.Fatalf("expected committed journal cleanup to return manifest, got m=%v err=%v", m, err)
	}

	// Interrupted mid-apply journal (JournalStateApplying) rolls back cleanly on startup recovery.
	if _, err := PrepareRestore(context.Background(), archive, "stale-pass-2026", dbPath, pkiDir, RestoreMetadata{
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	}); err != nil {
		t.Fatal(err)
	}
	if err := CapturePendingRollbackSnapshots(dbPath); err != nil {
		t.Fatal(err)
	}
	j, loadErr := loadJournal(dbPath)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	j.State = JournalStateApplying
	if err := saveJournal(j); err != nil {
		t.Fatal(err)
	}
	if m, err := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, nil); !errors.Is(err, ErrRestoredRolledBack) || m != nil {
		t.Fatalf("expected interrupted applying journal to roll back and return ErrRestoredRolledBack, got m=%v err=%v", m, err)
	}
}

func TestRollbackRetainsPostPrepareAcknowledgedWritesJournalFaultsAndMFAExportGuard(t *testing.T) {
	maintenance.DefaultCoordinator.ResetForTest()
	defer maintenance.DefaultCoordinator.ResetForTest()
	defer mfa.ResetKeyStoreForTest()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "h01", "swanpanel.db")
	pkiDir := filepath.Join(tmp, "h01_pki")
	_ = os.MkdirAll(filepath.Dir(dbPath), 0700)
	generateTestCAFiles(t, pkiDir, "H01 CA")

	conn, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if db.DB != nil {
			if s, err := db.DB.DB(); err == nil {
				_ = s.Close()
			}
			db.DB = nil
		}
	}()
	if _, _, err := mfa.InitActiveKey(conn, dbPath, "h01-jwt-secret-long-12345"); err != nil {
		t.Fatal(err)
	}
	archive, _, err := CreateEncryptedBackup(context.Background(), conn, pkiDir, "h01-passphrase-2026")
	if err != nil {
		t.Fatal(err)
	}

	// H01: Stage restore BEFORE maintenance drain, then commit a concurrent write BEFORE activation.
	prepared, err := PrepareRestore(context.Background(), archive, "h01-passphrase-2026", dbPath, pkiDir, RestoreMetadata{
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.RollbackCaptured || prepared.RestoreID == "" {
		t.Fatalf("expected PrepareRestore to defer rollback capture and issue RestoreID, got %+v", prepared)
	}
	if err := conn.Create(&db.DNSOverride{
		ID:            "dns_ack_post_prepare",
		Hostname:      "ack-post-prepare.strongnet",
		RecordType:    "A",
		TargetAddress: "10.99.0.50",
	}).Error; err != nil {
		t.Fatal(err)
	}

	// Simulate activation failure forcing rollback -> acknowledged write MUST survive!
	_, recErr := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, &RestoreFaultHooks{
		OnBeforeReopenDB: func() error { return errors.New("injected reopen failure") },
	})
	if !errors.Is(recErr, ErrRestoredRolledBack) {
		t.Fatalf("expected ErrRestoredRolledBack, got %v", recErr)
	}
	var retained int64
	if err := db.DB.Model(&db.DNSOverride{}).Where("id = ?", "dns_ack_post_prepare").Count(&retained).Error; err != nil || retained != 1 {
		t.Fatalf("H01 regression: acknowledged write after PrepareRestore lost on rollback; count=%d err=%v", retained, err)
	}
	// H07: Durable status file matches prepared.RestoreID with rolled_back outcome.
	st, stErr := ReadRestoreStatus(dbPath)
	if stErr != nil || st.RestoreID != prepared.RestoreID || st.Outcome != OutcomeRolledBack {
		t.Fatalf("expected correlated rolled_back status for %q, got st=%+v err=%v", prepared.RestoreID, st, stErr)
	}

	// H03: Failing journal commit OR directory sync preserves rollback recovery instead of deleting snapshots.
	if _, err := PrepareRestore(context.Background(), archive, "h01-passphrase-2026", dbPath, pkiDir, RestoreMetadata{
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	}); err != nil {
		t.Fatal(err)
	}
	_, commitFaultErr := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, &RestoreFaultHooks{
		OnBeforeCommitJournal: func() error { return errors.New("disk full writing committed journal") },
	})
	if !errors.Is(commitFaultErr, ErrRestoredRolledBack) {
		t.Fatalf("expected commit journal failure to trigger safe rollback, got %v", commitFaultErr)
	}
	retained = 0
	if err := db.DB.Model(&db.DNSOverride{}).Where("id = ?", "dns_ack_post_prepare").Count(&retained).Error; err != nil || retained != 1 {
		t.Fatalf("expected prior live record retained after commit-journal fault rollback, got count=%d err=%v", retained, err)
	}

	if _, err := PrepareRestore(context.Background(), archive, "h01-passphrase-2026", dbPath, pkiDir, RestoreMetadata{
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	}); err != nil {
		t.Fatal(err)
	}
	_, syncFaultErr := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, &RestoreFaultHooks{
		OnBeforeSyncDir: func(dir string) error {
			if strings.Contains(dir, restoreWorkDirName) {
				return errors.New("injected dir sync failure")
			}
			return nil
		},
	})
	if syncFaultErr == nil {
		t.Fatal("expected injected directory sync failure during journal commit to return error")
	}

	// H02: Corrupt JSON journal fails closed explicitly.
	maintenance.DefaultCoordinator.ResetForTest()
	_ = os.MkdirAll(DBWorkDir(dbPath), 0700)
	_ = os.WriteFile(filepath.Join(DBWorkDir(dbPath), "journal.json"), []byte(`{corrupt`), 0600)
	if _, err := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, nil); !errors.Is(err, ErrRecoveryFailed) {
		t.Fatalf("expected ErrRecoveryFailed on corrupt journal, got %v", err)
	}
	if maintenance.DefaultCoordinator.CurrentStatus().Mode != maintenance.ModeFailedClosed {
		t.Fatalf("expected ModeFailedClosed on corrupt journal")
	}
	maintenance.DefaultCoordinator.ResetForTest()
	_ = os.RemoveAll(DBWorkDir(dbPath))

	// H09: Exporting when encrypted MFA users exist without active key fails cleanly instead of producing broken archive.
	origKey, keyPath, initErr := mfa.InitActiveKey(db.DB, dbPath, "")
	if initErr != nil || origKey == "" {
		t.Fatal(initErr)
	}
	encSecret, encErr := mfa.EncryptSecret("JBSWY3DPEHPK3PXP", origKey)
	if encErr != nil {
		t.Fatal(encErr)
	}
	if err := db.DB.Create(&db.User{
		ID:             "usr_mfa_guard",
		Email:          "mfa-guard@example.com",
		Name:           "MFA Guard",
		AuthType:       "local",
		PasswordHash:   "hash",
		Status:         "ACTIVE",
		MFAEnabled:     true,
		TOTPSecretEnc:  encSecret,
		MFABackupCodes: `["kept-hash-1"]`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	mfa.ResetKeyStoreForTest()
	if _, _, expErr := CreateEncryptedBackup(context.Background(), db.DB, pkiDir, "h01-passphrase-2026"); expErr == nil {
		t.Fatal("expected backup export to fail when encrypted MFA state exists without active MFA key")
	}

	// H04 accepted owner policy: single-use backup codes remain intact in archive restore.
	mfa.SetActiveKey(origKey, keyPath)
	withCodesArchive, _, err := CreateEncryptedBackup(context.Background(), db.DB, pkiDir, "h01-passphrase-2026")
	if err != nil {
		t.Fatal(err)
	}
	restMan, err := RestoreEncryptedBackup(context.Background(), withCodesArchive, "h01-passphrase-2026", dbPath, pkiDir, nil)
	if err != nil || restMan.DBSHA256 == "" {
		t.Fatalf("restore with MFA backup codes failed: %v", err)
	}
	var restoredUser db.User
	if err := db.DB.First(&restoredUser, "id = ?", "usr_mfa_guard").Error; err != nil || restoredUser.MFABackupCodes != `["kept-hash-1"]` {
		t.Fatalf("expected MFA backup codes retained per D-H04 owner policy, got %q err=%v", restoredUser.MFABackupCodes, err)
	}
}

func TestCommitMarkerSyncFailureNeverLeavesCommittedJournalOnCrashMidRollback(t *testing.T) {
	maintenance.DefaultCoordinator.ResetForTest()
	defer maintenance.DefaultCoordinator.ResetForTest()
	defer mfa.ResetKeyStoreForTest()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "r01", "swanpanel.db")
	pkiDir := filepath.Join(tmp, "r01_pki")
	_ = os.MkdirAll(filepath.Dir(dbPath), 0700)
	generateTestCAFiles(t, pkiDir, "R01 CA")

	conn, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if db.DB != nil {
			if s, err := db.DB.DB(); err == nil {
				_ = s.Close()
			}
			db.DB = nil
		}
	}()
	if _, _, err := mfa.InitActiveKey(conn, dbPath, "r01-jwt-secret-long-12345"); err != nil {
		t.Fatal(err)
	}
	archive, _, err := CreateEncryptedBackup(context.Background(), conn, pkiDir, "r01-passphrase-2026")
	if err != nil {
		t.Fatal(err)
	}

	if err := conn.Create(&db.DNSOverride{
		ID:            "dns_r01_keep",
		Hostname:      "r01-keep.strongnet",
		RecordType:    "A",
		TargetAddress: "10.88.0.10",
	}).Error; err != nil {
		t.Fatal(err)
	}

	prepared, err := PrepareRestore(context.Background(), archive, "r01-passphrase-2026", dbPath, pkiDir, RestoreMetadata{
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	})
	if err != nil {
		t.Fatal(err)
	}

	// R01: Simulate commit dir-sync failing AFTER marker rename AND rollback itself crashing/failing.
	// journal.json on disk must stay "applying" (NOT "committed") with no leftover commit marker,
	// so the next startup finishes rollback cleanly rather than deleting rollback snapshots.
	syncCommitCalls := 0
	var journalStateDuringRollback string
	_, recErr := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, &RestoreFaultHooks{
		OnBeforeSyncDir: func(dir string) error {
			if strings.Contains(dir, restoreWorkDirName) {
				syncCommitCalls++
				// Fail after staging/applying work-dir sync during commit step.
				if _, stErr := os.Stat(commitMarkerPathForDB(dbPath)); stErr == nil {
					return errors.New("injected dir sync failure after commit marker rename")
				}
			}
			return nil
		},
		OnRollbackDB: func() error {
			jCheck, lErr := loadJournal(dbPath)
			if lErr == nil {
				journalStateDuringRollback = jCheck.State
			}
			if _, ok := hasValidCommitMarker(dbPath); ok {
				return errors.New("commit marker still present during rollback")
			}
			return errors.New("simulated crash mid-rollback")
		},
	})
	if !errors.Is(recErr, ErrRollbackFailed) || syncCommitCalls == 0 {
		t.Fatalf("expected ErrRollbackFailed on commit-sync + rollback crash, got recErr=%v calls=%d", recErr, syncCommitCalls)
	}
	if journalStateDuringRollback != JournalStateApplying {
		t.Fatalf("R01 regression: expected journal state %q during rollback after commit sync fault, got %q", JournalStateApplying, journalStateDuringRollback)
	}

	// Simulate operator clearing rollback_failed back to interrupted "applying" retry: recovery rolls back cleanly & retains pre-restore row!
	jRetry, lErr := loadJournal(dbPath)
	if lErr != nil {
		t.Fatal(lErr)
	}
	jRetry.State = JournalStateApplying
	if err := saveJournal(jRetry); err != nil {
		t.Fatal(err)
	}
	maintenance.DefaultCoordinator.ResetForTest()
	if _, err := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, nil); !errors.Is(err, ErrRestoredRolledBack) {
		t.Fatalf("expected clean rollback recovery from interrupted applying state, got %v", err)
	}
	var kept int64
	if err := db.DB.Model(&db.DNSOverride{}).Where("id = ?", "dns_r01_keep").Count(&kept).Error; err != nil || kept != 1 {
		t.Fatalf("expected pre-restore record retained after R01 recovery, got count=%d err=%v", kept, err)
	}
	if st, err := ReadRestoreStatus(dbPath); err != nil || st.RestoreID != prepared.RestoreID || st.Outcome != OutcomeRolledBack {
		t.Fatalf("expected rolled_back status for %q, got st=%+v err=%v", prepared.RestoreID, st, err)
	}

	// R01.1: If commit marker deletion fsync fails during retraction, fail-closed immediately
	// WITHOUT executing rollback or discarding recovery snapshots.
	_, prepRetractErr := PrepareRestore(context.Background(), archive, "r01-passphrase-2026", dbPath, pkiDir, RestoreMetadata{
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	})
	if prepRetractErr != nil {
		t.Fatal(prepRetractErr)
	}
	var rollbackInvokedAfterRetractSyncFail bool
	var commitMarkerSyncStarted bool
	_, retractSyncErr := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, &RestoreFaultHooks{
		OnBeforeSyncDir: func(dir string) error {
			if strings.Contains(dir, restoreWorkDirName) {
				if _, stErr := os.Stat(commitMarkerPathForDB(dbPath)); stErr == nil {
					commitMarkerSyncStarted = true
					return errors.New("dir sync fault during commit")
				}
				if commitMarkerSyncStarted {
					return errors.New("dir sync fault during marker retraction")
				}
			}
			return nil
		},
		OnRollbackDB: func() error {
			rollbackInvokedAfterRetractSyncFail = true
			return nil
		},
	})
	if !errors.Is(retractSyncErr, ErrRollbackFailed) || rollbackInvokedAfterRetractSyncFail {
		t.Fatalf("R01.1: expected fail-closed without running rollback when marker deletion sync fails, got err=%v rollbackInvoked=%v", retractSyncErr, rollbackInvokedAfterRetractSyncFail)
	}
	if _, statErr := os.Stat(filepath.Join(DBWorkDir(dbPath), "rollback.db")); statErr != nil {
		t.Fatalf("R01.1: expected rollback snapshot retained on fail-closed marker retraction failure: %v", statErr)
	}
	maintenance.DefaultCoordinator.ResetForTest()
	_ = os.RemoveAll(DBWorkDir(dbPath))
	_ = os.RemoveAll(PKIWorkDir(pkiDir))
	_, _ = db.InitDB(dbPath)
	mfa.ResetKeyStoreForTest()
	_, _, _ = mfa.InitActiveKey(db.DB, dbPath, "r01-jwt-secret-long-12345")

	// R03.1: Status-file persistence failure during commit OR rollback must retain recovery evidence
	// so API status queries still resolve the exact outcome (never roll back a durable commit).
	prepCommitted, err := PrepareRestore(context.Background(), archive, "r01-passphrase-2026", dbPath, pkiDir, RestoreMetadata{
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(restoreStatusPathForDB(dbPath))
	appliedMan, commitStatusErr := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, &RestoreFaultHooks{
		OnBeforeWriteStatus: func(status RestoreStatus) error {
			return errors.New("disk full writing status file")
		},
	})
	if commitStatusErr != nil || appliedMan == nil {
		t.Fatalf("R03.1: expected durable commit to succeed despite status write failure, got man=%v err=%v", appliedMan, commitStatusErr)
	}
	if _, jErr := os.Stat(journalPathForDB(dbPath)); jErr != nil {
		t.Fatalf("R03.1: expected journal retained when applied status write failed: %v", jErr)
	}
	if resolved, rErr := ReadRestoreStatus(dbPath); rErr != nil || resolved.RestoreID != prepCommitted.RestoreID || resolved.Outcome != OutcomeApplied {
		t.Fatalf("R03.1: expected fallback ReadRestoreStatus to recover applied outcome %q, got st=%+v err=%v", prepCommitted.RestoreID, resolved, rErr)
	}
	// Recovery cleanup succeeds once status file becomes writable on subsequent startup check.
	if _, retryErr := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, nil); retryErr != nil {
		t.Fatalf("R03.1: committed journal recovery failed: %v", retryErr)
	}
	if _, jErr := os.Stat(DBWorkDir(dbPath)); !errors.Is(jErr, os.ErrNotExist) {
		t.Fatalf("expected work dir cleaned after status write recovered, got %v", jErr)
	}

	// Same R03.1 retention on rolled-back branch.
	prepRolled, err := PrepareRestore(context.Background(), archive, "r01-passphrase-2026", dbPath, pkiDir, RestoreMetadata{
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(restoreStatusPathForDB(dbPath))
	_, rbStatusErr := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, &RestoreFaultHooks{
		OnBeforeReopenDB: func() error { return errors.New("injected activate fault") },
		OnBeforeWriteStatus: func(status RestoreStatus) error {
			return errors.New("disk full writing rolled_back status file")
		},
	})
	if !errors.Is(rbStatusErr, ErrRestoredRolledBack) {
		t.Fatalf("expected ErrRestoredRolledBack, got %v", rbStatusErr)
	}
	if _, jErr := os.Stat(journalPathForDB(dbPath)); jErr != nil {
		t.Fatalf("R03.1: expected journal retained when rolled_back status write failed: %v", jErr)
	}
	if resolved, rErr := ReadRestoreStatus(dbPath); rErr != nil || resolved.RestoreID != prepRolled.RestoreID || resolved.Outcome != OutcomeRolledBack {
		t.Fatalf("R03.1: expected fallback ReadRestoreStatus to recover rolled_back outcome %q, got st=%+v err=%v", prepRolled.RestoreID, resolved, rErr)
	}
	if _, retryErr := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, nil); retryErr != nil {
		t.Fatalf("R03.1: expected rolled_back journal cleanup on next startup to succeed, got %v", retryErr)
	}

	// R03.1 stale previous status file precedence: when a previous restore left a valid status file on disk
	// and a subsequent restore fails to overwrite it on BOTH applied and rolled_back paths, ReadRestoreStatus
	// MUST return the newer operation's outcome from the retained journal/marker, never the stale status.
	prevStatus := RestoreStatus{
		RestoreID:   "previous-operation",
		Outcome:     OutcomeApplied,
		CompletedAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
	}
	if err := writeRestoreStatus(dbPath, prevStatus); err != nil {
		t.Fatal(err)
	}
	prepNewApplied, err := PrepareRestore(context.Background(), archive, "r01-passphrase-2026", dbPath, pkiDir, RestoreMetadata{
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, &RestoreFaultHooks{
		OnBeforeWriteStatus: func(status RestoreStatus) error {
			return errors.New("cannot overwrite previous status file")
		},
	}); err != nil {
		t.Fatalf("expected second commit to succeed cleanly: %v", err)
	}
	if st, err := ReadRestoreStatus(dbPath); err != nil || st.RestoreID != prepNewApplied.RestoreID || st.Outcome != OutcomeApplied {
		t.Fatalf("R03.1 applied stale-status regression: expected RestoreID=%q outcome=%q, got st=%+v err=%v", prepNewApplied.RestoreID, OutcomeApplied, st, err)
	}
	// Clean up status on retry before testing rolled_back with preexisting status.
	if _, err := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, nil); err != nil {
		t.Fatal(err)
	}

	if err := writeRestoreStatus(dbPath, prevStatus); err != nil {
		t.Fatal(err)
	}
	prepNewRolled, err := PrepareRestore(context.Background(), archive, "r01-passphrase-2026", dbPath, pkiDir, RestoreMetadata{
		MFAKeyPath: mfa.ResolveKeyFilePath(dbPath),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, &RestoreFaultHooks{
		OnBeforeReopenDB: func() error { return errors.New("injected activate fault with previous status present") },
		OnBeforeWriteStatus: func(status RestoreStatus) error {
			return errors.New("cannot overwrite previous status file on rollback")
		},
	}); !errors.Is(err, ErrRestoredRolledBack) {
		t.Fatalf("expected ErrRestoredRolledBack, got %v", err)
	}
	if st, err := ReadRestoreStatus(dbPath); err != nil || st.RestoreID != prepNewRolled.RestoreID || st.Outcome != OutcomeRolledBack {
		t.Fatalf("R03.1 rolled_back stale-status regression: expected RestoreID=%q outcome=%q, got st=%+v err=%v", prepNewRolled.RestoreID, OutcomeRolledBack, st, err)
	}
	if _, err := RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, nil); err != nil {
		t.Fatal(err)
	}
}
