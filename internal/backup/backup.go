package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"github.com/swan-panel/swan-panel/internal/mfa"
	"github.com/swan-panel/swan-panel/internal/pki"
	"golang.org/x/crypto/argon2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	backupMagic        = "SWANBK01"
	saltSize           = 16
	nonceSize          = 12
	headerSize         = len(backupMagic) + saltSize + nonceSize
	minPassphraseBytes = 12
	maxPassphraseBytes = 128

	// Export and restore share identical aligned resource caps.
	MaxBackupPlainBytes  = 256 * 1024 * 1024 // 256 MiB decompressed tar stream cap
	MaxBackupCipherBytes = 64 * 1024 * 1024  // 64 MiB encrypted ciphertext cap
	MaxBackupEntries     = 4096              // Maximum tar archive entries
	MaxBackupEntryBytes  = 200 * 1024 * 1024 // Single file cap

	CurrentBackupFormatVersion = 2
	archiveMFAKeyEntry         = "secrets/mfa.key"
)

var (
	backupOpMu sync.Mutex
	cryptoSem  = make(chan struct{}, 2)

	// ErrInvalidPassphrase is returned when a passphrase does not meet length requirements.
	ErrInvalidPassphrase = errors.New("passphrase must be between 12 and 128 bytes")
	// ErrInvalidArchive is returned when decryption, decompression, or archive validation fails.
	ErrInvalidArchive = errors.New("invalid backup archive or incorrect passphrase")
	// ErrBackupLimitExceeded is returned when an exported or uploaded backup exceeds resource bounds.
	ErrBackupLimitExceeded = errors.New("backup archive exceeds documented size or entry limits")
	// ErrBackupBusy is returned when concurrent Argon2id crypto capacity is saturated.
	ErrBackupBusy = errors.New("another backup or restore cryptographic operation is in progress")
	// ErrIncompatibleMFAArchive is returned when a legacy v1 backup has encrypted MFA secrets without a portable MFA key.
	ErrIncompatibleMFAArchive = errors.New("legacy format_version=1 archive contains encrypted MFA state without a portable MFA encryption key")
)

// RestoreMetadata records non-secret operator context in the restore journal.
type RestoreMetadata struct {
	ActorID    string
	ActorEmail string
	ActorRole  string
	SourceIP   string
	MFAKeyPath string
}

// Manifest describes the verified contents of a SwanPanel backup archive.
// Never store encryption keys or passphrases in Manifest.
type Manifest struct {
	FormatVersion int      `json:"format_version"`
	CreatedAt     string   `json:"created_at"`
	DBSHA256      string   `json:"db_sha256"`
	PKIFiles      []string `json:"pki_files"`
	HasMFAKey     bool     `json:"has_mfa_key,omitempty"`
}

func acquireCryptoSlot(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case cryptoSem <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-cryptoSem }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, ErrBackupBusy
	}
}

func deriveBackupKey(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, 3, 64*1024, 2, 32)
}

func validatePassphrase(passphrase string) error {
	if len(passphrase) < minPassphraseBytes || len(passphrase) > maxPassphraseBytes {
		return ErrInvalidPassphrase
	}
	return nil
}

// CreateEncryptedBackup creates an authenticated AES-256-GCM encrypted .tar.enc archive
// containing a consistent SQLite + PKI snapshot and the active independent MFA key.
func CreateEncryptedBackup(ctx context.Context, database *gorm.DB, pkiDir, passphrase string) ([]byte, Manifest, error) {
	return CreateEncryptedBackupWithOptions(ctx, database, pkiDir, passphrase, mfa.ActiveKey(""))
}

// CreateEncryptedBackupWithOptions allows explicit specification of the portable MFA key to bundle.
func CreateEncryptedBackupWithOptions(ctx context.Context, database *gorm.DB, pkiDir, passphrase, mfaKey string) ([]byte, Manifest, error) {
	var manifest Manifest
	if database == nil {
		return nil, manifest, errors.New("database is unavailable")
	}
	if err := validatePassphrase(passphrase); err != nil {
		return nil, manifest, err
	}
	if strings.TrimSpace(pkiDir) == "" {
		return nil, manifest, errors.New("PKI directory is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	releaseCrypto, err := acquireCryptoSlot(ctx)
	if err != nil {
		return nil, manifest, err
	}
	defer releaseCrypto()

	backupOpMu.Lock()
	defer backupOpMu.Unlock()

	tmpDir, err := os.MkdirTemp("", "swanpanel-backup-*")
	if err != nil {
		return nil, manifest, fmt.Errorf("create staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	type stagedPKI struct {
		rel  string
		mode fs.FileMode
		data []byte
	}
	var dbBytes []byte
	var pkiEntries []stagedPKI
	var mfaKeyMaterial string

	// Freeze domain/PKI mutations ONLY for consistent snapshot capture; perform
	// compression and Argon2id/AES-GCM encryption outside the mutation barrier.
	func() {
		releaseSnap := maintenance.DefaultCoordinator.AcquireSnapshotBarrier()
		defer releaseSnap()

		dbSnapshotPath := filepath.Join(tmpDir, "swanpanel.db")
		if err = database.WithContext(ctx).Exec("VACUUM INTO ?", dbSnapshotPath).Error; err != nil {
			err = fmt.Errorf("snapshot database: %w", err)
			return
		}
		dbInfo, statErr := os.Stat(dbSnapshotPath)
		if statErr != nil {
			err = fmt.Errorf("stat database snapshot: %w", statErr)
			return
		}
		if dbInfo.Size() < 0 || dbInfo.Size() > MaxBackupEntryBytes || dbInfo.Size() > MaxBackupPlainBytes {
			err = fmt.Errorf("%w: database snapshot (%d bytes) exceeds limits", ErrBackupLimitExceeded, dbInfo.Size())
			return
		}
		plainRunning := dbInfo.Size()
		var entryRunning = 2 // manifest + db (manifest size checked afterward)

		if strings.TrimSpace(mfaKey) != "" {
			mfaKeyMaterial = strings.TrimSpace(mfaKey)
		} else if kp := mfa.ActiveKeyPath(); kp != "" {
			k, readErr := mfa.ReadKeyFile(kp)
			if readErr != nil {
				if !errors.Is(readErr, os.ErrNotExist) {
					err = fmt.Errorf("read active MFA key for backup: %w", readErr)
					return
				}
			} else {
				mfaKeyMaterial = k
			}
		}
		if mfaKeyMaterial != "" {
			entryRunning++
			plainRunning += int64(len(mfaKeyMaterial) + 1)
			if plainRunning > MaxBackupPlainBytes {
				err = fmt.Errorf("%w: archive payload exceeds uncompressed limit (%d bytes)", ErrBackupLimitExceeded, MaxBackupPlainBytes)
				return
			}
		} else {
			var mfaUsersCount int64
			if cntErr := database.WithContext(ctx).Model(&db.User{}).
				Where("totp_secret_enc != '' OR pending_totp_enc != ''").
				Count(&mfaUsersCount).Error; cntErr != nil {
				err = fmt.Errorf("verify MFA account state before backup: %w", cntErr)
				return
			}
			if mfaUsersCount > 0 {
				err = errors.New("backup export failed: encrypted MFA accounts exist but active MFA key is unavailable")
				return
			}
		}

		dbBytes, err = readFileBounded(dbSnapshotPath, MaxBackupEntryBytes)
		if err != nil {
			err = fmt.Errorf("read database snapshot: %w", err)
			return
		}

		err = filepath.WalkDir(pkiDir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				if d.Name() == restoreWorkDirName {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			name := d.Name()
			if name == ".perm_test" || strings.HasPrefix(name, ".tmp-") || strings.HasPrefix(name, ".swanpanel") {
				return nil
			}
			rel, relErr := filepath.Rel(pkiDir, path)
			if relErr != nil {
				return relErr
			}
			relSlash := filepath.ToSlash(rel)
			if strings.HasPrefix(relSlash, "..") {
				return nil
			}
			info, infoErr := d.Info()
			if infoErr != nil {
				return infoErr
			}
			entryRunning++
			if entryRunning > MaxBackupEntries {
				return fmt.Errorf("%w: entry count exceeds maximum %d", ErrBackupLimitExceeded, MaxBackupEntries)
			}
			if info.Size() < 0 || info.Size() > MaxBackupEntryBytes || plainRunning+info.Size() > MaxBackupPlainBytes {
				return fmt.Errorf("%w: PKI archive payload exceeds size limits", ErrBackupLimitExceeded)
			}
			content, readErr := readFileBounded(path, MaxBackupEntryBytes)
			if readErr != nil {
				return readErr
			}
			plainRunning += int64(len(content))
			if plainRunning > MaxBackupPlainBytes {
				return fmt.Errorf("%w: archive payload exceeds uncompressed limit (%d bytes)", ErrBackupLimitExceeded, MaxBackupPlainBytes)
			}
			pkiEntries = append(pkiEntries, stagedPKI{
				rel:  relSlash,
				mode: info.Mode().Perm(),
				data: content,
			})
			return nil
		})
	}()
	if err != nil {
		return nil, manifest, err
	}

	sort.Slice(pkiEntries, func(i, j int) bool { return pkiEntries[i].rel < pkiEntries[j].rel })
	pkiNames := make([]string, 0, len(pkiEntries))
	for _, entry := range pkiEntries {
		pkiNames = append(pkiNames, entry.rel)
	}

	dbHash := sha256.Sum256(dbBytes)
	manifest = Manifest{
		FormatVersion: CurrentBackupFormatVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		DBSHA256:      hex.EncodeToString(dbHash[:]),
		PKIFiles:      pkiNames,
		HasMFAKey:     mfaKeyMaterial != "",
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, manifest, fmt.Errorf("encode manifest: %w", err)
	}

	// Enforce aligned entry count & uncompressed limits before finalizing.
	totalEntries := 2 + len(pkiEntries) // manifest + db
	if mfaKeyMaterial != "" {
		totalEntries++
	}
	if totalEntries > MaxBackupEntries {
		return nil, manifest, fmt.Errorf("%w: entry count %d exceeds maximum %d", ErrBackupLimitExceeded, totalEntries, MaxBackupEntries)
	}

	var tarBuf bytes.Buffer
	gz := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gz)
	var plainTotal int64

	writeEntry := func(name string, mode int64, data []byte) error {
		if int64(len(data)) > MaxBackupEntryBytes || plainTotal+int64(len(data)) > MaxBackupPlainBytes {
			return fmt.Errorf("%w: archive payload exceeds uncompressed limit (%d bytes)", ErrBackupLimitExceeded, MaxBackupPlainBytes)
		}
		plainTotal += int64(len(data))
		hdr := &tar.Header{
			Name:     name,
			Mode:     mode,
			Size:     int64(len(data)),
			ModTime:  time.Now().UTC(),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}

	if err := writeEntry("manifest.json", 0600, manifestBytes); err != nil {
		return nil, manifest, err
	}
	if err := writeEntry("database/swanpanel.db", 0600, dbBytes); err != nil {
		return nil, manifest, err
	}
	if mfaKeyMaterial != "" {
		if err := writeEntry(archiveMFAKeyEntry, 0600, []byte(mfaKeyMaterial+"\n")); err != nil {
			return nil, manifest, err
		}
	}
	for _, entry := range pkiEntries {
		mode := int64(entry.mode)
		if mode == 0 {
			mode = 0600
		}
		if err := writeEntry("pki/"+entry.rel, mode, entry.data); err != nil {
			return nil, manifest, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, manifest, fmt.Errorf("finalize tar archive: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, manifest, fmt.Errorf("finalize gzip stream: %w", err)
	}

	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, manifest, fmt.Errorf("generate salt: %w", err)
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, manifest, fmt.Errorf("generate nonce: %w", err)
	}

	header := make([]byte, 0, headerSize)
	header = append(header, []byte(backupMagic)...)
	header = append(header, salt...)
	header = append(header, nonce...)

	key := deriveBackupKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, manifest, fmt.Errorf("initialize cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, manifest, fmt.Errorf("initialize GCM: %w", err)
	}
	encrypted := gcm.Seal(header, nonce, tarBuf.Bytes(), header)
	if len(encrypted) > MaxBackupCipherBytes {
		return nil, manifest, fmt.Errorf("%w: encrypted archive %d bytes exceeds maximum %d bytes", ErrBackupLimitExceeded, len(encrypted), MaxBackupCipherBytes)
	}

	slog.Info("Encrypted backup archive created", "format_version", manifest.FormatVersion, "pki_files", len(pkiEntries), "size_bytes", len(encrypted))
	return encrypted, manifest, nil
}

func isSafeArchivePath(name string) bool {
	if name == "" || strings.ContainsRune(name, '\x00') || strings.ContainsRune(name, '\\') {
		return false
	}
	if strings.HasPrefix(name, "/") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return false
	}
	return clean == name
}

func validateExtractedDatabase(dbPath string) (*gorm.DB, error) {
	testConn, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open restored database: %w", err)
	}
	var integrity string
	if err := testConn.Raw("PRAGMA integrity_check;").Scan(&integrity).Error; err != nil || strings.ToLower(integrity) != "ok" {
		if sqlDB, sErr := testConn.DB(); sErr == nil {
			_ = sqlDB.Close()
		}
		return nil, fmt.Errorf("database integrity check failed: %q (%v)", integrity, err)
	}
	if err := testConn.Exec("PRAGMA foreign_keys = ON;").Error; err == nil {
		var fkViolations int64
		if fkErr := testConn.Raw("SELECT COUNT(*) FROM pragma_foreign_key_check;").Scan(&fkViolations).Error; fkErr != nil || fkViolations > 0 {
			if sqlDB, sErr := testConn.DB(); sErr == nil {
				_ = sqlDB.Close()
			}
			return nil, fmt.Errorf("database foreign_key_check failed (violations=%d, err=%v)", fkViolations, fkErr)
		}
	}
	for _, table := range []string{"users", "audit_logs", "strongswan_settings"} {
		var cnt int64
		if err := testConn.Table(table).Count(&cnt).Error; err != nil {
			if sqlDB, sErr := testConn.DB(); sErr == nil {
				_ = sqlDB.Close()
			}
			return nil, fmt.Errorf("restored database missing required table %s: %w", table, err)
		}
	}
	return testConn, nil
}

func validateExtractedPKI(pkiDir string) error {
	caCertBytes, err := os.ReadFile(filepath.Join(pkiDir, "ca.crt"))
	if err != nil {
		return fmt.Errorf("restored backup missing ca.crt: %w", err)
	}
	caKeyBytes, err := os.ReadFile(filepath.Join(pkiDir, "ca.key"))
	if err != nil {
		return fmt.Errorf("restored backup missing ca.key: %w", err)
	}
	certBlock, _ := pem.Decode(caCertBytes)
	if certBlock == nil {
		return errors.New("restored ca.crt is not valid PEM")
	}
	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse restored ca.crt: %w", err)
	}
	keyBlock, _ := pem.Decode(caKeyBytes)
	if keyBlock == nil {
		return errors.New("restored ca.key is not valid PEM")
	}
	var rsaKey *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes); err == nil {
		rsaKey = k
	} else if iface, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err == nil {
		rsaKey, _ = iface.(*rsa.PrivateKey)
	}
	if rsaKey == nil {
		return errors.New("restored ca.key is not a valid RSA private key")
	}
	pub, ok := caCert.PublicKey.(*rsa.PublicKey)
	if !ok || pub.N.Cmp(rsaKey.N) != 0 {
		return errors.New("restored ca.crt and ca.key do not match")
	}

	// Verify optional server cert/key consistency if both or either is present.
	srvCertPath := filepath.Join(pkiDir, "server.crt")
	srvKeyPath := filepath.Join(pkiDir, "server.key")
	srvCertBytes, errCert := os.ReadFile(srvCertPath)
	srvKeyBytes, errKey := os.ReadFile(srvKeyPath)
	if (errCert == nil) != (errKey == nil) {
		return errors.New("restored PKI contains partial gateway server.crt/server.key pair")
	}
	if errCert == nil {
		sb, _ := pem.Decode(srvCertBytes)
		kb, _ := pem.Decode(srvKeyBytes)
		if sb == nil || kb == nil {
			return errors.New("restored server certificate or key is invalid PEM")
		}
		sc, err := x509.ParseCertificate(sb.Bytes)
		if err != nil {
			return fmt.Errorf("parse restored server.crt: %w", err)
		}
		var sk *rsa.PrivateKey
		if k, err := x509.ParsePKCS1PrivateKey(kb.Bytes); err == nil {
			sk = k
		} else if iface, err := x509.ParsePKCS8PrivateKey(kb.Bytes); err == nil {
			sk, _ = iface.(*rsa.PrivateKey)
		}
		sPub, ok := sc.PublicKey.(*rsa.PublicKey)
		if !ok || sk == nil || sPub.N.Cmp(sk.N) != 0 {
			return errors.New("restored server.crt and server.key do not match")
		}
		if err := sc.CheckSignatureFrom(caCert); err != nil {
			return fmt.Errorf("restored server.crt is not signed by restored CA: %w", err)
		}
	}

	return nil
}

// PrepareRestore decrypts, authenticates, verifies schema/PKI/MFA key compatibility,
// creates persistent rollback copies across mount points, and writes a durable 'prepared'
// transaction journal WITHOUT mutating live state yet.
func PrepareRestore(ctx context.Context, encrypted []byte, passphrase, dbPath, pkiDir string, meta RestoreMetadata) (*RestoreJournal, error) {
	if err := validatePassphrase(passphrase); err != nil {
		return nil, err
	}
	if len(encrypted) < headerSize+16 || len(encrypted) > MaxBackupCipherBytes {
		if len(encrypted) > MaxBackupCipherBytes {
			return nil, fmt.Errorf("%w: encrypted archive exceeds %d bytes", ErrBackupLimitExceeded, MaxBackupCipherBytes)
		}
		return nil, ErrInvalidArchive
	}
	if string(encrypted[:len(backupMagic)]) != backupMagic {
		return nil, ErrInvalidArchive
	}
	if strings.TrimSpace(dbPath) == "" || strings.TrimSpace(pkiDir) == "" {
		return nil, errors.New("target database or PKI path is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Never overwrite a failed-closed unresolved restore journal.
	if existing, err := loadJournal(dbPath); err == nil {
		if existing.State == JournalStateRollbackFailed || existing.State == JournalStateApplying {
			return nil, ErrRestoreJournalConflict
		}
	}

	releaseCrypto, err := acquireCryptoSlot(ctx)
	if err != nil {
		return nil, err
	}

	header := encrypted[:headerSize]
	salt := encrypted[len(backupMagic) : len(backupMagic)+saltSize]
	nonce := encrypted[len(backupMagic)+saltSize : headerSize]
	ciphertext := encrypted[headerSize:]

	key := deriveBackupKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		releaseCrypto()
		return nil, fmt.Errorf("initialize cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		releaseCrypto()
		return nil, fmt.Errorf("initialize GCM: %w", err)
	}
	compressed, err := gcm.Open(nil, nonce, ciphertext, header)
	if err != nil {
		releaseCrypto()
		return nil, ErrInvalidArchive
	}

	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		releaseCrypto()
		return nil, ErrInvalidArchive
	}
	defer func() { _ = gz.Close() }()

	limited := io.LimitReader(gz, MaxBackupPlainBytes+1)
	tr := tar.NewReader(limited)

	// Extract first into an isolated temporary staging dir, verify everything BEFORE touching
	// persistent restore-work dirs or entering maintenance mode.
	tmpStage, err := os.MkdirTemp("", "swanpanel-preflight-*")
	if err != nil {
		releaseCrypto()
		return nil, fmt.Errorf("create preflight staging directory: %w", err)
	}
	cleanupTmpStage := true
	defer func() {
		if cleanupTmpStage {
			_ = os.RemoveAll(tmpStage)
		}
	}()

	tmpPKIDir := filepath.Join(tmpStage, "pki")
	if err := os.MkdirAll(tmpPKIDir, 0700); err != nil {
		releaseCrypto()
		return nil, fmt.Errorf("create staged PKI directory: %w", err)
	}

	var (
		manifest     Manifest
		hasManifest  bool
		dbBytes      []byte
		mfaKeyBytes  []byte
		extractedPKI []string
		seenEntries  = make(map[string]bool)
		entryCount   int
		totalBytes   int64
	)

	for {
		hdr, tarErr := tr.Next()
		if errors.Is(tarErr, io.EOF) {
			break
		}
		if tarErr != nil {
			releaseCrypto()
			return nil, ErrInvalidArchive
		}
		entryCount++
		if entryCount > MaxBackupEntries {
			releaseCrypto()
			return nil, ErrBackupLimitExceeded
		}
		if hdr.Typeflag != tar.TypeReg {
			releaseCrypto()
			return nil, ErrInvalidArchive
		}
		if hdr.Size < 0 || hdr.Size > MaxBackupEntryBytes || totalBytes+hdr.Size > MaxBackupPlainBytes {
			releaseCrypto()
			return nil, ErrBackupLimitExceeded
		}
		if !isSafeArchivePath(hdr.Name) || seenEntries[hdr.Name] {
			releaseCrypto()
			return nil, ErrInvalidArchive
		}
		seenEntries[hdr.Name] = true

		content, readErr := io.ReadAll(io.LimitReader(tr, hdr.Size+1))
		if readErr != nil || int64(len(content)) != hdr.Size {
			releaseCrypto()
			return nil, ErrInvalidArchive
		}
		totalBytes += hdr.Size

		switch {
		case hdr.Name == "manifest.json":
			if err := json.Unmarshal(content, &manifest); err != nil || (manifest.FormatVersion != 1 && manifest.FormatVersion != CurrentBackupFormatVersion) {
				releaseCrypto()
				return nil, ErrInvalidArchive
			}
			hasManifest = true
		case hdr.Name == "database/swanpanel.db":
			dbBytes = content
		case hdr.Name == archiveMFAKeyEntry:
			mfaKeyBytes = content
		case strings.HasPrefix(hdr.Name, "pki/"):
			rel := strings.TrimPrefix(hdr.Name, "pki/")
			if rel == "" || !isSafeArchivePath(rel) || strings.HasPrefix(rel, restoreWorkDirName) {
				releaseCrypto()
				return nil, ErrInvalidArchive
			}
			dest := filepath.Join(tmpPKIDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
				releaseCrypto()
				return nil, fmt.Errorf("prepare staged PKI subdir: %w", err)
			}
			mode := fs.FileMode(hdr.Mode).Perm()
			if mode == 0 || strings.HasSuffix(rel, ".key") {
				mode = 0600
			}
			if err := os.WriteFile(dest, content, mode); err != nil {
				releaseCrypto()
				return nil, fmt.Errorf("write staged PKI file: %w", err)
			}
			extractedPKI = append(extractedPKI, rel)
		default:
			releaseCrypto()
			return nil, ErrInvalidArchive
		}
	}
	releaseCrypto()

	if !hasManifest || len(dbBytes) == 0 {
		return nil, ErrInvalidArchive
	}
	actualDBHash := sha256.Sum256(dbBytes)
	actualDBHex := hex.EncodeToString(actualDBHash[:])
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(manifest.DBSHA256)), []byte(actualDBHex)) != 1 {
		return nil, ErrInvalidArchive
	}

	// Verify manifest PKIFiles set equals extracted PKI files 1-to-1.
	sort.Strings(extractedPKI)
	expectedPKI := append([]string(nil), manifest.PKIFiles...)
	sort.Strings(expectedPKI)
	if len(extractedPKI) != len(expectedPKI) {
		return nil, ErrInvalidArchive
	}
	for i := range expectedPKI {
		if extractedPKI[i] != expectedPKI[i] {
			return nil, ErrInvalidArchive
		}
	}

	tmpDBPath := filepath.Join(tmpStage, "swanpanel.db")
	if err := os.WriteFile(tmpDBPath, dbBytes, 0600); err != nil {
		return nil, fmt.Errorf("write staged database: %w", err)
	}
	stagedConn, err := validateExtractedDatabase(tmpDBPath)
	if err != nil {
		return nil, fmt.Errorf("validate restored database: %w", err)
	}
	defer func() {
		if sqlDB, sErr := stagedConn.DB(); sErr == nil {
			_ = sqlDB.Close()
		}
	}()

	if err := validateExtractedPKI(tmpPKIDir); err != nil {
		return nil, fmt.Errorf("validate restored PKI: %w", err)
	}

	// Verify MFA key presence & decryptability BEFORE touching live storage.
	var stagedMFAKey string
	var mfaUsersCount int64
	if err := stagedConn.Model(&db.User{}).
		Where("totp_secret_enc != '' OR pending_totp_enc != ''").
		Count(&mfaUsersCount).Error; err != nil {
		return nil, fmt.Errorf("inspect restored MFA users: %w", err)
	}
	if len(mfaKeyBytes) > 0 {
		stagedMFAKey = strings.TrimSpace(string(mfaKeyBytes))
		if err := mfa.ValidateKeyDecryptsDB(stagedConn, stagedMFAKey); err != nil {
			return nil, fmt.Errorf("validate restored MFA key compatibility: %w", err)
		}
	} else if manifest.HasMFAKey || (manifest.FormatVersion >= 2 && mfaUsersCount > 0) {
		return nil, errors.New("backup archive missing portable MFA encryption key")
	} else if mfaUsersCount > 0 {
		return nil, ErrIncompatibleMFAArchive
	}

	// Verify sufficient disk space on both DB mount and PKI mount for staged + rollback copies.
	var liveDBSize int64
	if dbStat, stErr := os.Stat(dbPath); stErr == nil {
		liveDBSize = dbStat.Size()
	}
	neededDBBytes := uint64(len(dbBytes)*2+int(liveDBSize)) + 1024*1024
	if err := checkAvailableDiskSpace(filepath.Dir(dbPath), neededDBBytes); err != nil {
		return nil, err
	}
	neededPKIBytes := uint64(totalBytes*2) + 1024*1024
	if err := checkAvailableDiskSpace(pkiDir, neededPKIBytes); err != nil {
		return nil, err
	}

	mfaKeyPath := meta.MFAKeyPath
	if strings.TrimSpace(mfaKeyPath) == "" {
		mfaKeyPath = mfa.ResolveKeyFilePath(dbPath)
	}

	dbWork := DBWorkDir(dbPath)
	pkiWork := PKIWorkDir(pkiDir)
	_ = os.RemoveAll(dbWork)
	_ = os.RemoveAll(pkiWork)

	if err := os.MkdirAll(dbWork, 0700); err != nil {
		return nil, fmt.Errorf("create DB restore work dir: %w", err)
	}
	if err := os.MkdirAll(pkiWork, 0700); err != nil {
		_ = os.RemoveAll(dbWork)
		return nil, fmt.Errorf("create PKI restore work dir: %w", err)
	}

	stagedDBPersisted := filepath.Join(dbWork, "staged.db")
	if err := writeFileSynced(stagedDBPersisted, dbBytes, 0600); err != nil {
		_ = os.RemoveAll(dbWork)
		_ = os.RemoveAll(pkiWork)
		return nil, fmt.Errorf("persist staged DB on DB mount: %w", err)
	}

	var stagedMFAPersisted string
	if stagedMFAKey != "" {
		stagedMFAPersisted = filepath.Join(dbWork, "staged.mfa.key")
		if err := writeFileSynced(stagedMFAPersisted, []byte(stagedMFAKey+"\n"), 0600); err != nil {
			_ = os.RemoveAll(dbWork)
			_ = os.RemoveAll(pkiWork)
			return nil, fmt.Errorf("persist staged MFA key: %w", err)
		}
	}

	stagedPKIPersisted := filepath.Join(pkiWork, "staged")
	if err := copyDirectoryPreservingWork(tmpPKIDir, stagedPKIPersisted); err != nil {
		_ = os.RemoveAll(dbWork)
		_ = os.RemoveAll(pkiWork)
		return nil, fmt.Errorf("persist staged PKI on PKI mount: %w", err)
	}

	// Defer rollback snapshots until maintenance mode drains in-flight writers,
	// capturing rollback copies exclusively post-drain right before activation begins.
	journal := &RestoreJournal{
		RestoreID:        uuid.New().String(),
		State:            JournalStatePrepared,
		DBPath:           dbPath,
		PKIDir:           pkiDir,
		MFAKeyPath:       mfaKeyPath,
		DBWorkDir:        dbWork,
		PKIWorkDir:       pkiWork,
		StagedDBPath:     stagedDBPersisted,
		StagedPKIDir:     stagedPKIPersisted,
		StagedMFAKeyPath: stagedMFAPersisted,
		RollbackCaptured: false,
		RollbackDBPath:   filepath.Join(dbWork, "rollback.db"),
		RollbackPKIDir:   filepath.Join(pkiWork, "rollback"),
		ActorID:          meta.ActorID,
		ActorEmail:       meta.ActorEmail,
		ActorRole:        meta.ActorRole,
		SourceIP:         meta.SourceIP,
		Manifest:         manifest,
	}
	if err := saveJournal(journal); err != nil {
		_ = os.RemoveAll(dbWork)
		_ = os.RemoveAll(pkiWork)
		return nil, fmt.Errorf("write durable restore transaction journal: %w", err)
	}
	return journal, nil
}

func readFileBounded(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrBackupLimitExceeded
	}
	return data, nil
}

// RestoreEncryptedBackup decrypts, stages, validates, and atomically applies SQLite, PKI,
// and portable MFA key state with crash-safe rollback protection.
func RestoreEncryptedBackup(ctx context.Context, encrypted []byte, passphrase, dbPath, pkiDir string, manager *pki.PKIManager) (Manifest, error) {
	return RestoreEncryptedBackupWithMeta(ctx, encrypted, passphrase, dbPath, pkiDir, manager, RestoreMetadata{})
}

// RestoreEncryptedBackupWithMeta executes a complete synchronous restore with actor audit metadata.
func RestoreEncryptedBackupWithMeta(ctx context.Context, encrypted []byte, passphrase, dbPath, pkiDir string, manager *pki.PKIManager, meta RestoreMetadata) (Manifest, error) {
	var empty Manifest
	releaseRestore, ok := maintenance.DefaultCoordinator.TryLockRestore()
	if !ok {
		return empty, errors.New("another backup restore operation is already in progress")
	}
	defer releaseRestore()

	backupOpMu.Lock()
	defer backupOpMu.Unlock()

	_, err := PrepareRestore(ctx, encrypted, passphrase, dbPath, pkiDir, meta)
	if err != nil {
		return empty, err
	}

	if err := maintenance.DefaultCoordinator.EnterMaintenance("Restoring encrypted backup", 5*time.Second); err != nil {
		slog.Warn("Entering restore maintenance mode with active request timeout", "error", err)
	}
	defer maintenance.DefaultCoordinator.ExitMaintenance()

	manifest, err := RecoverOrApplyPendingRestore(dbPath, pkiDir, manager, nil)
	if err != nil {
		return empty, err
	}
	if manifest != nil {
		return *manifest, nil
	}
	return empty, nil
}

func copyFileAtomicSynced(src, dst string, mode fs.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return writeFileSynced(dst, data, mode)
}

func copyDirectoryPreservingWork(src, dst string) error {
	if err := os.MkdirAll(dst, 0700); err != nil {
		return err
	}
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() && d.Name() == restoreWorkDirName {
			return filepath.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0600
		}
		return copyFileAtomicSynced(path, target, mode)
	})
	if err == nil {
		err = syncDir(dst)
	}
	return err
}

func replaceDirectoryContentsPreservingWork(dst, src string) error {
	srcInfo, statErr := os.Stat(src)
	if statErr != nil {
		return fmt.Errorf("staged or rollback PKI directory %s unavailable: %w", src, statErr)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("staged or rollback PKI path %s is not a directory", src)
	}
	if err := os.MkdirAll(dst, 0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(dst)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == restoreWorkDirName {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return copyDirectoryPreservingWork(src, dst)
}
