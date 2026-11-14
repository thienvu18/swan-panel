package pki

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

var validDeviceID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,100}$`)

// RevokedCertificate is a permanent entry in the CA revocation ledger.
type RevokedCertificate struct {
	Serial    string
	RevokedAt time.Time
}

func atomicPrivateFile(path string, data []byte) (resultErr error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".swanpanel-")
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			if closeErr := file.Close(); closeErr != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("close temporary private file: %w", closeErr))
			}
		}
		if removeErr := os.Remove(file.Name()); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove temporary private file: %w", removeErr))
		}
	}()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	closeErr := file.Close()
	closed = true
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}

func (m *PKIManager) deviceBundlePath(deviceID string) (string, error) {
	if !validDeviceID.MatchString(deviceID) {
		return "", errors.New("invalid device identifier")
	}
	return filepath.Join(m.dataDir, "devices", deviceID+".json"), nil
}

// StoreDeviceBundle persists the *same* certificate and private key for every subsequent download.
func (m *PKIManager) StoreDeviceBundle(deviceID string, bundle *CertificateBundle) error {
	if bundle == nil || bundle.PrivateKeyPEM == "" {
		return errors.New("invalid identity bundle")
	}
	path, err := m.deviceBundlePath(deviceID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("identity already exists for %s; explicit rotation required", deviceID)
	} else if !os.IsNotExist(err) {
		return err
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		return err
	}
	return atomicPrivateFile(path, raw)
}

func (m *PKIManager) LoadDeviceBundle(deviceID string) (*CertificateBundle, error) {
	path, err := m.deviceBundlePath(deviceID)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bundle CertificateBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(bundle.CertPEM))
	if block == nil {
		return nil, errors.New("stored client certificate is invalid")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	if err := cert.CheckSignatureFrom(m.caCert); err != nil {
		return nil, err
	}
	if cert.SerialNumber.Text(16) != bundle.SerialNumber {
		return nil, errors.New("stored serial number does not match certificate")
	}
	key, err := parseRSAPrivateKeyFromPEM([]byte(bundle.PrivateKeyPEM))
	if err != nil {
		return nil, err
	}
	publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("stored device certificate has unsupported public-key type")
	}
	if key.N.Cmp(publicKey.N) != 0 || key.E != publicKey.E {
		return nil, errors.New("private key does not match the stored certificate")
	}
	if time.Now().After(cert.NotAfter) {
		return nil, errors.New("device certificate expired; rotate device identity")
	}
	bundle.CertDER = cert.Raw
	bundle.PrivateKeyDER = x509.MarshalPKCS1PrivateKey(key)
	return &bundle, nil
}

// DeleteDeviceBundle erases the exportable private key after revocation has been recorded.
func (m *PKIManager) DeleteDeviceBundle(deviceID string) error {
	path, err := m.deviceBundlePath(deviceID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// GeneratePKCS12 is used by native Apple/Windows clients and the Android strongSwan app.
// LegacyDES is required for Apple/Android compatibility; the exported profile must only
// be delivered over authenticated HTTPS and is sensitive even when password-protected.
func GeneratePKCS12(bundle *CertificateBundle, password string, includeCA bool) ([]byte, error) {
	if bundle == nil {
		return nil, errors.New("certificate bundle required")
	}
	certBlock, _ := pem.Decode([]byte(bundle.CertPEM))
	if certBlock == nil {
		return nil, errors.New("invalid client certificate PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}
	key, err := parseRSAPrivateKeyFromPEM([]byte(bundle.PrivateKeyPEM))
	if err != nil {
		return nil, err
	}
	var chain []*x509.Certificate
	if includeCA {
		caBlock, _ := pem.Decode([]byte(bundle.CACertPEM))
		if caBlock == nil {
			return nil, errors.New("invalid CA certificate PEM")
		}
		ca, err := x509.ParseCertificate(caBlock.Bytes)
		if err != nil {
			return nil, err
		}
		chain = append(chain, ca)
	}
	return pkcs12.LegacyDES.Encode(key, cert, chain, password)
}

// CRL uses a complete, durable revocation list, not only in-memory flags.
// Revocation entries must remain after devices are deleted.
func (m *PKIManager) CRL(revoked []RevokedCertificate) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.caCert == nil || m.caKey == nil {
		return nil, errors.New("CA unavailable")
	}
	// Every newly initialized SwanPanel Root CA includes a SubjectKeyId.
	if len(m.caCert.SubjectKeyId) == 0 {
		return nil, errors.New("root CA has no SubjectKeyId; an explicit CA migration is required for signed CRL")
	}
	issuer := *m.caCert
	now := time.Now().UTC()
	entries := make([]x509.RevocationListEntry, 0, len(revoked))
	for _, r := range revoked {
		serial, valid := new(big.Int).SetString(strings.TrimSpace(r.Serial), 16)
		if !valid || serial.Sign() <= 0 {
			return nil, fmt.Errorf("invalid revoked serial %q", r.Serial)
		}
		entries = append(entries, x509.RevocationListEntry{
			SerialNumber: serial, RevocationTime: r.RevokedAt.UTC(),
			ReasonCode: 1,
		})
	}
	return x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(now.UnixNano()), ThisUpdate: now.Add(-time.Minute),
		NextUpdate: now.Add(24 * time.Hour), RevokedCertificateEntries: entries,
	}, &issuer, crypto.Signer(m.caKey))
}

func (m *PKIManager) PublishCRL(ctx context.Context, revoked []RevokedCertificate) error {
	data, err := m.CRL(revoked)
	if err != nil {
		return err
	}
	// Store in application data first (for restarts/backups).
	if err := atomicPrivateFile(filepath.Join(m.dataDir, "ca.crl"), data); err != nil {
		return err
	}
	// Device-side CRL must be loaded by charon for revocation to be effective.
	if os.Getenv("APP_ENV") == "test" {
		return nil
	}
	path := filepath.Join("/etc/swanctl/x509crl", "swanpanel.crl")
	if err := atomicPrivateFile(path, data); err != nil {
		return fmt.Errorf("cannot install revocation list: %w", err)
	}
	command := exec.CommandContext(ctx, "swanctl", "--load-creds", "--noprompt")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("strongSwan CRL reload failed: %w: %s", err, string(output))
	}
	return nil
}
