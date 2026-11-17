package pki

import (
	"crypto/x509/pkix"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CAIdentity is public certificate metadata only. Private keys are never returned.
type CAIdentity struct {
	CommonName   string    `json:"common_name"`
	Organization string    `json:"organization"`
	Country      string    `json:"country"`
	Province     string    `json:"province"`
	Locality     string    `json:"locality"`
	Serial       string    `json:"serial"`
	NotAfter     time.Time `json:"not_after"`
}

func identityPart(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func (m *PKIManager) CAIdentity() CAIdentity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.caCert == nil {
		return CAIdentity{}
	}
	cert := m.caCert
	return CAIdentity{
		CommonName:   cert.Subject.CommonName,
		Organization: identityPart(cert.Subject.Organization),
		Country:      identityPart(cert.Subject.Country),
		Province:     identityPart(cert.Subject.Province),
		Locality:     identityPart(cert.Subject.Locality),
		Serial:       cert.SerialNumber.Text(16), NotAfter: cert.NotAfter,
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// Names are applied to the actual X.509 Root CA only when it is first issued.
// Existing roots are never replaced automatically by environment changes.
func defaultCASubject() pkix.Name {
	return pkix.Name{
		CommonName:   envOr("CA_COMMON_NAME", "SwanPanel Internal Root CA"),
		Organization: []string{envOr("CA_ORGANIZATION", "SwanPanel Root Authority")},
		Country:      []string{envOr("CA_COUNTRY", "US")},
		Province:     []string{envOr("CA_PROVINCE", "California")},
		Locality:     []string{envOr("CA_LOCALITY", "San Francisco")},
	}
}

// RotateRootCA creates a new root only on an explicit, administrator-approved
// operation. The API must reject rotation if any device/certificate was issued.
// Old credentials remain in timestamped backups for manual disaster recovery.
func (m *PKIManager) RotateRootCA(subject pkix.Name) error {
	if subject.CommonName == "" || len(subject.Organization) == 0 {
		return fmt.Errorf("CA common name and organization required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	oldCert, oldKey, oldCertPEM, oldKeyPEM := m.caCert, m.caKey, m.caCertPEM, m.caKeyPEM
	files := []string{"ca.crt", "ca.key", "server.crt", "server.key", "ca.crl", "profile-signing.crt", "profile-signing.key"}
	suffix := ".backup-" + time.Now().UTC().Format("20060102T150405.000000000")
	moved := []string{}
	restore := func() {
		for _, name := range moved {
			// Only remove files for which a backup actually exists.
			// A failed rename must never erase unrelated existing credentials.
			_ = os.Remove(filepath.Join(m.dataDir, name))
			_ = os.Rename(filepath.Join(m.dataDir, name)+suffix, filepath.Join(m.dataDir, name))
		}
		m.caCert, m.caKey, m.caCertPEM, m.caKeyPEM = oldCert, oldKey, oldCertPEM, oldKeyPEM
	}
	for _, name := range files {
		path := filepath.Join(m.dataDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			restore()
			return err
		}
		if err := os.Rename(path, path+suffix); err != nil {
			restore()
			return fmt.Errorf("backup CA material: %w", err)
		}
		moved = append(moved, name)
	}
	if err := m.initCAWithSubject(subject); err != nil {
		restore()
		return fmt.Errorf("root CA rotation failed; previous root restored: %w", err)
	}
	return nil
}
