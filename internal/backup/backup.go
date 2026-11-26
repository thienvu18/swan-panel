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

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/pki"
	"golang.org/x/crypto/argon2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	backupMagic          = "SWANBK01"
	saltSize             = 16
	nonceSize            = 12
	headerSize           = len(backupMagic) + saltSize + nonceSize
	minPassphraseBytes   = 12
	maxPassphraseBytes   = 128
	maxBackupPlainBytes  = 256 * 1024 * 1024 // 256 MiB decompressed cap
	maxBackupCipherBytes = 64 * 1024 * 1024  // 64 MiB encrypted upload cap
)

var (
	backupOpMu sync.Mutex

	// ErrInvalidPassphrase is returned when a passphrase does not meet length requirements.
	ErrInvalidPassphrase = errors.New("passphrase must be between 12 and 128 bytes")
	// ErrInvalidArchive is returned when decryption, decompression, or archive validation fails.
	ErrInvalidArchive = errors.New("invalid backup archive or incorrect passphrase")
)

// Manifest describes the verified contents of a SwanPanel backup archive.
type Manifest struct {
	FormatVersion int      `json:"format_version"`
	CreatedAt     string   `json:"created_at"`
	DBSHA256      string   `json:"db_sha256"`
	PKIFiles      []string `json:"pki_files"`
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
// containing an online-consistent SQLite snapshot and all persisted PKI state.
func CreateEncryptedBackup(ctx context.Context, database *gorm.DB, pkiDir, passphrase string) ([]byte, Manifest, error) {
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

	backupOpMu.Lock()
	defer backupOpMu.Unlock()

	tmpDir, err := os.MkdirTemp("", "swanpanel-backup-*")
	if err != nil {
		return nil, manifest, fmt.Errorf("create staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Capture an online-consistent SQLite snapshot via VACUUM INTO.
	dbSnapshotPath := filepath.Join(tmpDir, "swanpanel.db")
	if err := database.WithContext(ctx).Exec("VACUUM INTO ?", dbSnapshotPath).Error; err != nil {
		return nil, manifest, fmt.Errorf("snapshot database: %w", err)
	}
	dbBytes, err := os.ReadFile(dbSnapshotPath)
	if err != nil {
		return nil, manifest, fmt.Errorf("read database snapshot: %w", err)
	}
	dbHash := sha256.Sum256(dbBytes)

	type stagedPKI struct {
		rel  string
		mode fs.FileMode
		data []byte
	}
	var pkiEntries []stagedPKI
	err = filepath.WalkDir(pkiDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == ".perm_test" || strings.HasPrefix(name, ".tmp-") {
			return nil
		}
		rel, err := filepath.Rel(pkiDir, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if strings.HasPrefix(relSlash, "..") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		pkiEntries = append(pkiEntries, stagedPKI{
			rel:  relSlash,
			mode: info.Mode().Perm(),
			data: content,
		})
		return nil
	})
	if err != nil {
		return nil, manifest, fmt.Errorf("collect PKI files: %w", err)
	}
	sort.Slice(pkiEntries, func(i, j int) bool { return pkiEntries[i].rel < pkiEntries[j].rel })

	pkiNames := make([]string, 0, len(pkiEntries))
	for _, entry := range pkiEntries {
		pkiNames = append(pkiNames, entry.rel)
	}
	manifest = Manifest{
		FormatVersion: 1,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		DBSHA256:      hex.EncodeToString(dbHash[:]),
		PKIFiles:      pkiNames,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, manifest, fmt.Errorf("encode manifest: %w", err)
	}

	var tarBuf bytes.Buffer
	gz := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gz)

	writeEntry := func(name string, mode int64, data []byte) error {
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
		return nil, manifest, fmt.Errorf("write manifest entry: %w", err)
	}
	if err := writeEntry("database/swanpanel.db", 0600, dbBytes); err != nil {
		return nil, manifest, fmt.Errorf("write database entry: %w", err)
	}
	for _, entry := range pkiEntries {
		mode := int64(entry.mode)
		if mode == 0 {
			mode = 0600
		}
		if err := writeEntry("pki/"+entry.rel, mode, entry.data); err != nil {
			return nil, manifest, fmt.Errorf("write PKI entry %s: %w", entry.rel, err)
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
	slog.Info("Encrypted backup archive created", "pki_files", len(pkiEntries), "size_bytes", len(encrypted))
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

func validateExtractedDatabase(dbPath string) error {
	testConn, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return fmt.Errorf("open restored database: %w", err)
	}
	sqlDB, err := testConn.DB()
	if err != nil {
		return fmt.Errorf("inspect restored database handle: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	var integrity string
	if err := testConn.Raw("PRAGMA integrity_check;").Scan(&integrity).Error; err != nil || strings.ToLower(integrity) != "ok" {
		return fmt.Errorf("database integrity check failed: %q (%v)", integrity, err)
	}
	var userCount int64
	if err := testConn.Table("users").Count(&userCount).Error; err != nil {
		return fmt.Errorf("restored database missing users table: %w", err)
	}
	return nil
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
	cert, err := x509.ParseCertificate(certBlock.Bytes)
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
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || pub.N.Cmp(rsaKey.N) != 0 {
		return errors.New("restored ca.crt and ca.key do not match")
	}
	return nil
}

// RestoreEncryptedBackup decrypts, validates, and atomically restores SQLite and PKI state.
func RestoreEncryptedBackup(ctx context.Context, encrypted []byte, passphrase, dbPath, pkiDir string, manager *pki.PKIManager) (Manifest, error) {
	var manifest Manifest
	if err := validatePassphrase(passphrase); err != nil {
		return manifest, err
	}
	if len(encrypted) < headerSize+16 || len(encrypted) > maxBackupCipherBytes {
		return manifest, ErrInvalidArchive
	}
	if string(encrypted[:len(backupMagic)]) != backupMagic {
		return manifest, ErrInvalidArchive
	}
	if strings.TrimSpace(dbPath) == "" || strings.TrimSpace(pkiDir) == "" {
		return manifest, errors.New("target database or PKI path is unavailable")
	}

	header := encrypted[:headerSize]
	salt := encrypted[len(backupMagic) : len(backupMagic)+saltSize]
	nonce := encrypted[len(backupMagic)+saltSize : headerSize]
	ciphertext := encrypted[headerSize:]

	key := deriveBackupKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return manifest, fmt.Errorf("initialize cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return manifest, fmt.Errorf("initialize GCM: %w", err)
	}
	compressed, err := gcm.Open(nil, nonce, ciphertext, header)
	if err != nil {
		return manifest, ErrInvalidArchive
	}

	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return manifest, ErrInvalidArchive
	}
	defer func() { _ = gz.Close() }()

	limited := io.LimitReader(gz, maxBackupPlainBytes+1)
	tr := tar.NewReader(limited)

	backupOpMu.Lock()
	defer backupOpMu.Unlock()

	stageDir, err := os.MkdirTemp("", "swanpanel-restore-*")
	if err != nil {
		return manifest, fmt.Errorf("create restore staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stageDir) }()

	stagePKIDir := filepath.Join(stageDir, "pki")
	if err := os.MkdirAll(stagePKIDir, 0700); err != nil {
		return manifest, fmt.Errorf("create staged PKI directory: %w", err)
	}
	var (
		hasManifest bool
		dbBytes     []byte
		totalBytes  int64
	)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return manifest, ErrInvalidArchive
		}
		if hdr.Typeflag != tar.TypeReg {
			return manifest, ErrInvalidArchive
		}
		if hdr.Size < 0 || totalBytes+hdr.Size > maxBackupPlainBytes {
			return manifest, ErrInvalidArchive
		}
		if !isSafeArchivePath(hdr.Name) {
			return manifest, ErrInvalidArchive
		}
		content, err := io.ReadAll(io.LimitReader(tr, hdr.Size+1))
		if err != nil || int64(len(content)) != hdr.Size {
			return manifest, ErrInvalidArchive
		}
		totalBytes += hdr.Size

		switch {
		case hdr.Name == "manifest.json":
			if err := json.Unmarshal(content, &manifest); err != nil || manifest.FormatVersion != 1 {
				return manifest, ErrInvalidArchive
			}
			hasManifest = true
		case hdr.Name == "database/swanpanel.db":
			dbBytes = content
		case strings.HasPrefix(hdr.Name, "pki/"):
			rel := strings.TrimPrefix(hdr.Name, "pki/")
			if rel == "" || !isSafeArchivePath(rel) {
				return manifest, ErrInvalidArchive
			}
			dest := filepath.Join(stagePKIDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
				return manifest, fmt.Errorf("prepare staged PKI subdir: %w", err)
			}
			mode := fs.FileMode(hdr.Mode).Perm()
			if mode == 0 || strings.HasSuffix(rel, ".key") {
				mode = 0600
			}
			if err := os.WriteFile(dest, content, mode); err != nil {
				return manifest, fmt.Errorf("write staged PKI file: %w", err)
			}
		default:
			return manifest, ErrInvalidArchive
		}
	}

	if !hasManifest || len(dbBytes) == 0 {
		return manifest, ErrInvalidArchive
	}
	actualDBHash := sha256.Sum256(dbBytes)
	actualDBHex := hex.EncodeToString(actualDBHash[:])
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(manifest.DBSHA256)), []byte(actualDBHex)) != 1 {
		return manifest, ErrInvalidArchive
	}

	stageDBPath := filepath.Join(stageDir, "swanpanel.db")
	if err := os.WriteFile(stageDBPath, dbBytes, 0600); err != nil {
		return manifest, fmt.Errorf("write staged database: %w", err)
	}
	if err := validateExtractedDatabase(stageDBPath); err != nil {
		return manifest, fmt.Errorf("validate restored database: %w", err)
	}
	if err := validateExtractedPKI(stagePKIDir); err != nil {
		return manifest, fmt.Errorf("validate restored PKI: %w", err)
	}

	// Snapshot existing DB (including WAL state) and PKI for rollback before replacing live state.
	rollbackPKIDir := filepath.Join(stageDir, "rollback_pki")
	if err := copyDirectory(pkiDir, rollbackPKIDir); err != nil {
		return manifest, fmt.Errorf("snapshot existing PKI for rollback: %w", err)
	}
	rollbackDBPath := filepath.Join(stageDir, "rollback.db")
	var hasRollbackDB bool
	if db.DB != nil {
		if err := db.DB.WithContext(ctx).Exec("VACUUM INTO ?", rollbackDBPath).Error; err != nil {
			return manifest, fmt.Errorf("snapshot existing database for rollback: %w", err)
		}
		hasRollbackDB = true
	} else if _, err := os.Stat(dbPath); err == nil {
		if err := copyFileAtomic(dbPath, rollbackDBPath, 0600); err != nil {
			return manifest, fmt.Errorf("snapshot existing database file for rollback: %w", err)
		}
		hasRollbackDB = true
	}

	rollbackAll := func() {
		_ = replaceDirectoryContents(pkiDir, rollbackPKIDir)
		if hasRollbackDB {
			for _, suffix := range []string{"-wal", "-shm"} {
				_ = os.Remove(dbPath + suffix)
			}
			if err := copyFileAtomic(rollbackDBPath, dbPath, 0600); err == nil {
				_, _ = db.InitDB(dbPath)
			}
		}
		if manager != nil {
			_ = manager.ReloadFromDisk()
		}
	}

	if err := replaceDirectoryContents(pkiDir, stagePKIDir); err != nil {
		_ = replaceDirectoryContents(pkiDir, rollbackPKIDir)
		return manifest, fmt.Errorf("apply restored PKI state: %w", err)
	}

	// Checkpoint and close current DB handle before swapping SQLite file and sidecars.
	if db.DB != nil {
		_ = db.DB.WithContext(ctx).Exec("PRAGMA wal_checkpoint(TRUNCATE);").Error
		if sqlDB, err := db.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(dbPath + suffix)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		rollbackAll()
		return manifest, fmt.Errorf("prepare database directory: %w", err)
	}
	if err := copyFileAtomic(stageDBPath, dbPath, 0600); err != nil {
		rollbackAll()
		return manifest, fmt.Errorf("replace SQLite database: %w", err)
	}

	if _, err := db.InitDB(dbPath); err != nil {
		rollbackAll()
		return manifest, fmt.Errorf("reopen restored database: %w", err)
	}

	if manager != nil {
		if err := manager.ReloadFromDisk(); err != nil {
			slog.Error("Failed to reload PKI manager from restored files", "error", err)
			rollbackAll()
			return manifest, fmt.Errorf("reload restored PKI: %w", err)
		}
	}

	slog.Info("Encrypted backup restored successfully", "created_at", manifest.CreatedAt, "pki_files", len(manifest.PKIFiles))
	return manifest, nil
}

func copyFileAtomic(src, dst string, mode fs.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-restore-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func copyDirectory(src, dst string) error {
	if err := os.MkdirAll(dst, 0700); err != nil {
		return err
	}
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
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
		return copyFileAtomic(path, target, mode)
	})
}

func replaceDirectoryContents(dst, src string) error {
	if err := os.MkdirAll(dst, 0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(dst)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return copyDirectory(src, dst)
}
