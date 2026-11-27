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
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/db"
)

func generateTestCAFiles(t *testing.T, pkiDir, commonName string) {
	t.Helper()
	if err := os.MkdirAll(pkiDir, 0700); err != nil {
		t.Fatalf("mkdir pkiDir: %v", err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(filepath.Join(pkiDir, "ca.crt"), certPEM, 0644); err != nil {
		t.Fatalf("write ca.crt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkiDir, "ca.key"), keyPEM, 0600); err != nil {
		t.Fatalf("write ca.key: %v", err)
	}
}

func encryptSyntheticArchive(t *testing.T, passphrase string, writeEntries func(tw *tar.Writer)) []byte {
	t.Helper()
	var tarBuf bytes.Buffer
	gz := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gz)
	writeEntries(tw)
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	salt := make([]byte, saltSize)
	nonce := make([]byte, nonceSize)
	_, _ = rand.Read(salt)
	_, _ = rand.Read(nonce)

	header := make([]byte, 0, headerSize)
	header = append(header, []byte(backupMagic)...)
	header = append(header, salt...)
	header = append(header, nonce...)

	key := deriveBackupKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new GCM: %v", err)
	}
	return gcm.Seal(header, nonce, tarBuf.Bytes(), header)
}

func TestPassphraseValidation(t *testing.T) {
	if err := validatePassphrase("short-pass"); !errors.Is(err, ErrInvalidPassphrase) {
		t.Fatalf("expected ErrInvalidPassphrase for short input, got %v", err)
	}
	tooLong := string(bytes.Repeat([]byte("a"), 129))
	if err := validatePassphrase(tooLong); !errors.Is(err, ErrInvalidPassphrase) {
		t.Fatalf("expected ErrInvalidPassphrase for 129-byte input, got %v", err)
	}
	if err := validatePassphrase("valid-passphrase-12"); err != nil {
		t.Fatalf("unexpected error for valid passphrase: %v", err)
	}
}

func TestEncryptedBackupAndRestoreRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "swanpanel.db")
	pkiDir := filepath.Join(tmp, "pki")
	generateTestCAFiles(t, pkiDir, "Original Backup CA")
	markerPath := filepath.Join(pkiDir, "devices", "laptop.crt")
	if err := os.MkdirAll(filepath.Dir(markerPath), 0700); err != nil {
		t.Fatalf("mkdir devices: %v", err)
	}
	if err := os.WriteFile(markerPath, []byte("original-device-cert"), 0600); err != nil {
		t.Fatalf("write marker device cert: %v", err)
	}

	conn, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		if db.DB != nil {
			if sqlDB, sqlErr := db.DB.DB(); sqlErr == nil {
				_ = sqlDB.Close()
			}
		}
	})

	record := db.DNSOverride{ID: "dns-orig", Hostname: "internal.corp.example", RecordType: "A", TargetAddress: "10.99.0.10"}
	if err := conn.Create(&record).Error; err != nil {
		t.Fatalf("seed dns record: %v", err)
	}

	passphrase := "disaster-recovery-pass-2026"
	archive, manifest, err := CreateEncryptedBackup(context.Background(), conn, pkiDir, passphrase)
	if err != nil {
		t.Fatalf("CreateEncryptedBackup failed: %v", err)
	}
	if len(archive) <= headerSize || manifest.FormatVersion != CurrentBackupFormatVersion || len(manifest.PKIFiles) < 3 {
		t.Fatalf("unexpected backup manifest/size: len=%d manifest=%+v", len(archive), manifest)
	}

	// Mutate both DB and PKI state after backup was taken.
	if err := db.DB.Where("hostname = ?", "internal.corp.example").Delete(&db.DNSOverride{}).Error; err != nil {
		t.Fatalf("delete dns record: %v", err)
	}
	if err := db.DB.Create(&db.DNSOverride{ID: "dns-post", Hostname: "post-backup-only.example", RecordType: "A", TargetAddress: "10.99.0.99"}).Error; err != nil {
		t.Fatalf("create post-backup record: %v", err)
	}
	if err := os.WriteFile(markerPath, []byte("overwritten-device-cert"), 0600); err != nil {
		t.Fatalf("overwrite marker: %v", err)
	}

	restoredManifest, err := RestoreEncryptedBackup(context.Background(), archive, passphrase, dbPath, pkiDir, nil)
	if err != nil {
		t.Fatalf("RestoreEncryptedBackup failed: %v", err)
	}
	if restoredManifest.DBSHA256 != manifest.DBSHA256 {
		t.Fatalf("restored manifest hash mismatch: got %q want %q", restoredManifest.DBSHA256, manifest.DBSHA256)
	}

	var restoredRecord db.DNSOverride
	if err := db.DB.Where("hostname = ?", "internal.corp.example").First(&restoredRecord).Error; err != nil {
		t.Fatalf("expected original DNS record after restore: %v", err)
	}
	var count int64
	if err := db.DB.Model(&db.DNSOverride{}).Where("hostname = ?", "post-backup-only.example").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("post-backup DNS record should not exist after restore: count=%d err=%v", count, err)
	}
	gotMarker, err := os.ReadFile(markerPath)
	if err != nil || string(gotMarker) != "original-device-cert" {
		t.Fatalf("expected restored PKI device file, got %q (%v)", string(gotMarker), err)
	}
}

func TestRestoreRejectsWrongPassphraseAndTamperedCiphertext(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "swanpanel.db")
	pkiDir := filepath.Join(tmp, "pki")
	generateTestCAFiles(t, pkiDir, "Tamper Test CA")

	conn, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		if db.DB != nil {
			if sqlDB, sqlErr := db.DB.DB(); sqlErr == nil {
				_ = sqlDB.Close()
			}
		}
	})

	archive, _, err := CreateEncryptedBackup(context.Background(), conn, pkiDir, "correct-passphrase-123")
	if err != nil {
		t.Fatalf("CreateEncryptedBackup failed: %v", err)
	}

	if _, err := RestoreEncryptedBackup(context.Background(), archive, "wrong-passphrase-123", dbPath, pkiDir, nil); !errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("expected ErrInvalidArchive for wrong passphrase, got %v", err)
	}

	tampered := append([]byte(nil), archive...)
	tampered[len(tampered)-1] ^= 0xff
	if _, err := RestoreEncryptedBackup(context.Background(), tampered, "correct-passphrase-123", dbPath, pkiDir, nil); !errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("expected ErrInvalidArchive for tampered ciphertext, got %v", err)
	}
}

func TestRestoreRejectsUnsafeTarEntries(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "swanpanel.db")
	pkiDir := filepath.Join(tmp, "pki")
	generateTestCAFiles(t, pkiDir, "Safe Path CA")

	passphrase := "safe-archive-check-123"
	for _, tc := range []struct {
		name string
		hdr  *tar.Header
		body []byte
	}{
		{
			name: "zip slip traversal",
			hdr:  &tar.Header{Name: "pki/../escape.txt", Mode: 0600, Size: 4, Typeflag: tar.TypeReg},
			body: []byte("evil"),
		},
		{
			name: "symlink entry",
			hdr:  &tar.Header{Name: "pki/link", Mode: 0777, Size: 0, Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
			body: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encrypted := encryptSyntheticArchive(t, passphrase, func(tw *tar.Writer) {
				_ = tw.WriteHeader(tc.hdr)
				if len(tc.body) > 0 {
					_, _ = tw.Write(tc.body)
				}
			})
			if _, err := RestoreEncryptedBackup(context.Background(), encrypted, passphrase, dbPath, pkiDir, nil); !errors.Is(err, ErrInvalidArchive) {
				t.Fatalf("expected ErrInvalidArchive for %s, got %v", tc.name, err)
			}
		})
	}
}

func TestRestoreRejectsMismatchedCAWithoutClobberingLiveState(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "swanpanel.db")
	pkiDir := filepath.Join(tmp, "pki")
	generateTestCAFiles(t, pkiDir, "Live Unchanged CA")

	if _, err := db.InitDB(dbPath); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		if db.DB != nil {
			if sqlDB, sqlErr := db.DB.DB(); sqlErr == nil {
				_ = sqlDB.Close()
			}
		}
	})
	liveBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read live db: %v", err)
	}
	liveCA, err := os.ReadFile(filepath.Join(pkiDir, "ca.crt"))
	if err != nil {
		t.Fatalf("read live ca.crt: %v", err)
	}

	// Build second distinct keypair to forge mismatched ca.crt / ca.key inside archive.
	otherDir := filepath.Join(tmp, "other-pki")
	generateTestCAFiles(t, otherDir, "Mismatch Key CA")
	wrongKey, err := os.ReadFile(filepath.Join(otherDir, "ca.key"))
	if err != nil {
		t.Fatalf("read wrong key: %v", err)
	}

	hash := sha256.Sum256(liveBytes)
	manifestBytes, _ := json.Marshal(Manifest{
		FormatVersion: 1,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		DBSHA256:      hex.EncodeToString(hash[:]),
		PKIFiles:      []string{"ca.crt", "ca.key"},
	})
	passphrase := "mismatched-ca-passphrase"
	forged := encryptSyntheticArchive(t, passphrase, func(tw *tar.Writer) {
		entries := []struct {
			name string
			data []byte
		}{
			{"manifest.json", manifestBytes},
			{"database/swanpanel.db", liveBytes},
			{"pki/ca.crt", liveCA},
			{"pki/ca.key", wrongKey},
		}
		for _, e := range entries {
			_ = tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0600, Size: int64(len(e.data)), Typeflag: tar.TypeReg})
			_, _ = tw.Write(e.data)
		}
	})

	if _, err := RestoreEncryptedBackup(context.Background(), forged, passphrase, dbPath, pkiDir, nil); err == nil {
		t.Fatal("expected error for mismatched CA keypair, got nil")
	}
	afterCA, err := os.ReadFile(filepath.Join(pkiDir, "ca.crt"))
	if err != nil || !bytes.Equal(afterCA, liveCA) {
		t.Fatalf("expected live CA certificate to remain untouched after failed restore")
	}
}
