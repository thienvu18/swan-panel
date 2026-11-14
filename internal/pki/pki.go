package pki

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"

	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"

	"path/filepath"
	"strings"
	"sync"

	"time"
)

// CertificateBundle holds PEM-encoded certificate, private key, and CA certificate
type CertificateBundle struct {
	CertPEM       string
	PrivateKeyPEM string
	CACertPEM     string
	Fingerprint   string
	CertDER       []byte
	PrivateKeyDER []byte
	SerialNumber  string
}

// PKIManager handles internal X.509 Certificate Authority operations
type PKIManager struct {
	mu        sync.RWMutex
	caCert    *x509.Certificate
	caKey     *rsa.PrivateKey
	caCertPEM string
	caKeyPEM  string
	dataDir   string
}

// DefaultPKI is the global in-memory singleton PKI manager
var DefaultPKI *PKIManager
var once sync.Once
var defaultInitErr error

// GetPKIManager initializes and returns the default PKI manager
func GetPKIManager() (*PKIManager, error) {
	once.Do(func() {
		manager := &PKIManager{}
		defaultInitErr = manager.initCA()
		if defaultInitErr == nil {
			DefaultPKI = manager
		}
	})
	return DefaultPKI, defaultInitErr
}

// resolvePKIDataDir discovers or creates a writable persistent directory for PKI state
func resolvePKIDataDir() string {
	if os.Getenv("APP_ENV") == "production" {
		if configured := strings.TrimSpace(os.Getenv("PKI_DATA_DIR")); configured != "" {
			return configured
		}
		return "/var/lib/swan-panel"
	}
	candidates := []string{
		os.Getenv("PKI_DATA_DIR"),
		"/var/lib/swan-panel",
		"data/pki",
		".swanpanel_pki",
	}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0700); err == nil {
			testFile := filepath.Join(dir, ".perm_test")
			if err := os.WriteFile(testFile, []byte("ok"), 0600); err == nil {
				_ = os.Remove(testFile)
				return dir
			}
		}
	}
	return "."
}

func parseRSAPrivateKeyFromPEM(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing private key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if keyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := keyInterface.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}
	return nil, fmt.Errorf("unsupported private key format")
}

// initCA generates or loads the Root CA from persistent storage
func (m *PKIManager) initCA() error {
	pkiDir := resolvePKIDataDir()
	m.dataDir = pkiDir
	caCertPath := filepath.Join(pkiDir, "ca.crt")
	caKeyPath := filepath.Join(pkiDir, "ca.key")

	if err := os.MkdirAll(pkiDir, 0700); err != nil {
		return err
	}
	// 1. Attempt loading existing persisted CA certificate and private key
	if certData, err1 := os.ReadFile(caCertPath); err1 == nil {
		if keyData, err2 := os.ReadFile(caKeyPath); err2 == nil {
			certBlock, _ := pem.Decode(certData)
			if certBlock != nil {
				parsedCA, err := x509.ParseCertificate(certBlock.Bytes)
				if err == nil && time.Now().Before(parsedCA.NotAfter) {
					parsedKey, errKey := parseRSAPrivateKeyFromPEM(keyData)
					if errKey == nil {
						if parsedCA.PublicKey.(*rsa.PublicKey).N.Cmp(parsedKey.N) != 0 {
							return fmt.Errorf("persisted CA certificate and private key do not match")
						}
						m.caCert = parsedCA
						m.caKey = parsedKey
						m.caCertPEM = string(certData)
						m.caKeyPEM = string(keyData)
						slog.Info("Root CA loaded from persistent storage", "path", caCertPath, "serial", parsedCA.SerialNumber.Text(16), "cn", parsedCA.Subject.CommonName)
						return nil
					}
					return fmt.Errorf("persisted CA private key is invalid; refusing to rotate CA implicitly: %w", errKey)
				} else {
					return fmt.Errorf("persisted CA certificate invalid or expired; CA rotation must be explicit")
				}
			}
		}
	}

	if _, err := os.Stat(caCertPath); err == nil {
		return fmt.Errorf("CA certificate exists but private key is missing; refusing implicit CA rotation")
	}
	if _, err := os.Stat(caKeyPath); err == nil {
		return fmt.Errorf("CA private key exists but certificate is missing; refusing implicit CA rotation")
	}
	slog.Info("Initializing new internal SwanPanel X.509 Root CA and persisting to disk")

	// Generate 4096-bit RSA key for Root CA
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		slog.Error("Failed to generate Root CA private key", "error", err)
		return fmt.Errorf("failed to generate CA key: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		slog.Error("Failed to generate CA serial number", "error", err)
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return err
	}
	ski := sha1.Sum(publicDER)
	caTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"SwanPanel Root Authority"},
			CommonName:   "SwanPanel Internal Root CA",
			Country:      []string{"US"},
			Province:     []string{"California"},
			Locality:     []string{"San Francisco"},
		},
		NotBefore:             time.Now().Add(-10 * time.Minute),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 years validity
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		MaxPathLen:            1,
		SubjectKeyId:          ski[:],
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &key.PublicKey, key)
	if err != nil {
		slog.Error("Failed to create self-signed CA certificate", "error", err)
		return fmt.Errorf("failed to create CA certificate: %w", err)
	}

	parsedCA, err := x509.ParseCertificate(caBytes)
	if err != nil {
		slog.Error("Failed to parse created CA certificate", "error", err)
		return fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	var certBuf bytes.Buffer
	if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: caBytes}); err != nil {
		return err
	}

	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	var keyBuf bytes.Buffer
	if err := pem.Encode(&keyBuf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return err
	}

	m.caCert = parsedCA
	m.caKey = key
	m.caCertPEM = certBuf.String()
	m.caKeyPEM = keyBuf.String()

	// Persist to disk with strict file permissions
	if err := os.WriteFile(caCertPath, certBuf.Bytes(), 0644); err != nil {
		return fmt.Errorf("persist CA certificate: %w", err)
	}
	if err := os.WriteFile(caKeyPath, keyBuf.Bytes(), 0600); err != nil {
		return fmt.Errorf("persist CA private key: %w", err)
	}

	// CA creation only persists application state. System credential installation
	// is an explicit startup/reconciliation action (never done by unit tests).

	slog.Info("Root CA initialized and persisted successfully", "serial", serialNumber.Text(16), "cn", parsedCA.Subject.CommonName, "path", caCertPath)
	return nil
}

// GetCACertPEM returns the Root CA certificate in PEM format
func (m *PKIManager) GetCACertPEM() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.caCertPEM
}

// GenerateClientCertificate generates a 2048-bit RSA client cert signed by Root CA
func (m *PKIManager) GenerateClientCertificate(hostname, identity string) (*CertificateBundle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	slog.Debug("Generating client certificate", "hostname", hostname, "identity", identity)

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		slog.Error("Failed to generate client RSA key", "error", err)
		return nil, fmt.Errorf("failed to generate client key: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		slog.Error("Failed to generate client serial number", "error", err)
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"SwanPanel Client"},
			CommonName:   hostname,
		},
		NotBefore:             time.Now().Add(-10 * time.Minute),
		NotAfter:              time.Now().AddDate(2, 0, 0), // 2 years validity
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{hostname},
	}

	if identity != "" && identity != hostname {
		template.DNSNames = append(template.DNSNames, identity)
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, m.caCert, &privKey.PublicKey, m.caKey)
	if err != nil {
		slog.Error("Failed to sign client certificate with CA", "error", err)
		return nil, fmt.Errorf("failed to sign client certificate: %w", err)
	}

	var certBuf bytes.Buffer
	if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		return nil, err
	}

	privKeyBytes := x509.MarshalPKCS1PrivateKey(privKey)
	var keyBuf bytes.Buffer
	if err := pem.Encode(&keyBuf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privKeyBytes}); err != nil {
		return nil, err
	}

	fingerprint := fmt.Sprintf("%x", sha256.Sum256(certBytes))

	slog.Info("Client certificate issued", "hostname", hostname, "serial", serialNumber.Text(16))

	return &CertificateBundle{
		CertPEM:       certBuf.String(),
		PrivateKeyPEM: keyBuf.String(),
		CACertPEM:     m.caCertPEM,
		Fingerprint:   fingerprint,
		CertDER:       certBytes,
		PrivateKeyDER: privKeyBytes,
		SerialNumber:  serialNumber.Text(16),
	}, nil
}

// GenerateServerCertificate generates a 2048-bit RSA server certificate for strongSwan daemon
func (m *PKIManager) GenerateServerCertificate(serverFQDN string, serverIPs []string) (*CertificateBundle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	slog.Debug("Generating gateway server certificate", "fqdn", serverFQDN)

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		slog.Error("Failed to generate server RSA key", "error", err)
		return nil, fmt.Errorf("failed to generate server key: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, err
	}

	var ipAddresses []net.IP
	if ip := net.ParseIP(serverFQDN); ip != nil {
		ipAddresses = append(ipAddresses, ip)
	}
	for _, ipStr := range serverIPs {
		if ip := net.ParseIP(ipStr); ip != nil {
			ipAddresses = append(ipAddresses, ip)
		}
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"SwanPanel Gateway"},
			CommonName:   serverFQDN,
		},
		NotBefore:             time.Now().Add(-10 * time.Minute),
		NotAfter:              time.Now().AddDate(3, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageIPSECEndSystem},
		BasicConstraintsValid: true,
		DNSNames:              nil,
		IPAddresses:           ipAddresses,
	}

	if net.ParseIP(serverFQDN) == nil {
		template.DNSNames = []string{serverFQDN}
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, template, m.caCert, &privKey.PublicKey, m.caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign server certificate: %w", err)
	}

	var certBuf bytes.Buffer
	if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		return nil, err
	}

	privKeyBytes := x509.MarshalPKCS1PrivateKey(privKey)
	var keyBuf bytes.Buffer
	if err := pem.Encode(&keyBuf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privKeyBytes}); err != nil {
		return nil, err
	}

	return &CertificateBundle{
		CertPEM:       certBuf.String(),
		PrivateKeyPEM: keyBuf.String(),
		CACertPEM:     m.caCertPEM,
		Fingerprint:   fmt.Sprintf("%x", serialNumber),
		CertDER:       certBytes,
		PrivateKeyDER: privKeyBytes,
		SerialNumber:  serialNumber.Text(16),
	}, nil
}

// EnsureServerCertificates writes CA and Server certificates into strongSwan directories (/etc/swanctl/)
func (m *PKIManager) EnsureServerCertificates(serverIdentity string, serverIPs []string) error {
	if serverIdentity == "" {
		return fmt.Errorf("strongSwan server identity is required")
	}
	caDir := "/etc/swanctl/x509ca"
	certDir := "/etc/swanctl/x509"
	keyDir := "/etc/swanctl/rsa"
	dataDir := m.dataDir

	// Persist and reuse the *same* server identity, unless its SAN/expiry requires rotation.
	certFile := filepath.Join(dataDir, "server.crt")
	keyFile := filepath.Join(dataDir, "server.key")
	certBytes, certErr := os.ReadFile(certFile)
	keyBytes, keyErr := os.ReadFile(keyFile)
	if certErr == nil && keyErr == nil {
		block, _ := pem.Decode(certBytes)
		key, err := parseRSAPrivateKeyFromPEM(keyBytes)
		if err != nil {
			return fmt.Errorf("existing gateway key invalid: %w", err)
		}
		if block == nil {
			return fmt.Errorf("existing gateway certificate invalid")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return err
		}
		if cert.PublicKey.(*rsa.PublicKey).N.Cmp(key.N) != 0 {
			return fmt.Errorf("gateway certificate/private key mismatch")
		}
		if err := cert.VerifyHostname(serverIdentity); err == nil && time.Until(cert.NotAfter) > 30*24*time.Hour {
			return m.syncServerMaterial(certBytes, keyBytes, caDir, certDir, keyDir)
		}
	} else if (certErr == nil) != (keyErr == nil) {
		return fmt.Errorf("gateway certificate/key pair incomplete, refusing silent rotation")
	}
	bundle, err := m.GenerateServerCertificate(serverIdentity, serverIPs)
	if err != nil {
		return err
	}
	if err := atomicPrivateFile(keyFile, []byte(bundle.PrivateKeyPEM)); err != nil {
		return err
	}
	if err := atomicPrivateFile(certFile, []byte(bundle.CertPEM)); err != nil {
		return err
	}
	return m.syncServerMaterial([]byte(bundle.CertPEM), []byte(bundle.PrivateKeyPEM), caDir, certDir, keyDir)
}

func (m *PKIManager) syncServerMaterial(cert, key []byte, caDir, certDir, keyDir string) error {
	// In unit tests the system directories are deliberately not accessed.
	if os.Getenv("APP_ENV") == "test" {
		return nil
	}
	for _, dir := range []string{caDir, certDir, keyDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create strongSwan credential directory %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(caDir, "ca.crt"), []byte(m.GetCACertPEM()), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(certDir, "server.crt"), cert, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(keyDir, "server.key"), key, 0600); err != nil {
		return err
	}
	return nil
}

// GenerateAppleMobileconfig always returns signed CMS/PKCS#7 DER, never unsigned XML.
func GenerateAppleMobileconfig(serverFQDN, deviceHostname string, certBundle *CertificateBundle) ([]byte, error) {
	manager, err := GetPKIManager()
	if err != nil {
		return nil, err
	}
	profile, err := BuildAppleProfile(serverFQDN, serverFQDN, deviceHostname, certBundle)
	if err != nil {
		return nil, err
	}
	return manager.SignAppleProfile([]byte(profile))
}

// GenerateWindowsPowerShellScript imports the private key and client certificate into LocalMachine.
func GenerateWindowsPowerShellScript(serverFQDN, deviceHostname string, certBundle *CertificateBundle) string {
	script, err := BuildWindowsInstaller(serverFQDN, deviceHostname, certBundle)
	if err != nil {
		slog.Error("Windows VPN profile generation failed", "error", err)
		return ""
	}
	return script
}

// GenerateSwanctlConfig creates a Linux strongSwan swanctl.conf configuration
func GenerateSwanctlConfig(serverFQDN, deviceHostname string, certBundle *CertificateBundle) string {
	return fmt.Sprintf(`# SwanPanel Linux Native swanctl.conf
# Place in /etc/swanctl/conf.d/swanpanel.conf

connections {
    swanpanel {
        remote_addrs = %s
        vips = 0.0.0.0

        local {
            auth = pubkey
            certs = client.crt
            id = %s
        }

        remote {
            auth = pubkey
            id = %%any
            cacerts = swanpanel-ca.crt
        }

        children {
            swanpanel-traffic {
                remote_ts = 0.0.0.0/0
                start_action = start
                dpd_action = restart
                close_action = none
                mode = tunnel
                esp_proposals = aes256gcm16-ecp384,aes256-sha256
            }
        }

        version = 2
        mobike = yes
        encap = yes
        proposals = aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048
    }
}
`, serverFQDN, deviceHostname)
}

// GenerateLinuxInstallerScript creates a complete self-installing bash script for Linux strongSwan
func GenerateLinuxInstallerScript(serverEndpoint, deviceHostname string, certBundle *CertificateBundle) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
# ==============================================================================
# SwanPanel Native Linux Client Installer
# Target Server: %s | Device: %s
# ==============================================================================
set -eo pipefail

if [ "$EUID" -ne 0 ]; then
  echo "[-] Please run as root: sudo bash $0"
  exit 1
fi

echo "[*] Installing SwanPanel native IKEv2 profile for '%s'..."

# Ensure strongSwan / swanctl directories exist
mkdir -p /etc/swanctl/x509ca /etc/swanctl/x509 /etc/swanctl/rsa /etc/swanctl/conf.d

# 1. Write Root CA certificate
cat << 'EOF_CA' > /etc/swanctl/x509ca/swanpanel-ca.crt
%s
EOF_CA

# 2. Write Client certificate
cat << 'EOF_CERT' > /etc/swanctl/x509/client.crt
%s
EOF_CERT

# 3. Write Client private key
cat << 'EOF_KEY' > /etc/swanctl/rsa/client.key
%s
EOF_KEY
chmod 600 /etc/swanctl/rsa/client.key

# 4. Write swanctl configuration
cat << 'EOF_CONF' > /etc/swanctl/conf.d/swanpanel.conf
connections {
    swanpanel {
        remote_addrs = %s
        vips = 0.0.0.0

        local {
            auth = pubkey
            certs = client.crt
            id = %s
        }

        remote {
            auth = pubkey
            id = %%any
            cacerts = swanpanel-ca.crt
        }

        children {
            swanpanel-traffic {
                remote_ts = 0.0.0.0/0
                start_action = start
                dpd_action = restart
                close_action = none
                mode = tunnel
                esp_proposals = aes256gcm16-ecp384,aes256-sha256
            }
        }

        version = 2
        mobike = yes
        encap = yes
        proposals = aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048
    }
}
EOF_CONF

echo "[+] Credentials and configuration installed to /etc/swanctl/."

# 5. Reload credentials and initiate connection if strongSwan is running
if command -v swanctl >/dev/null 2>&1; then
    swanctl --load-all || true
    echo "[*] Connecting to SwanPanel Gateway at %s..."
    swanctl --initiate --child swanpanel-traffic || true
    echo "[+] Done! Check status with: swanctl --list-sas"
else
    echo "[!] strongSwan swanctl not found. Please install strongswan and run: swanctl --load-all"
fi
`, serverEndpoint, deviceHostname, deviceHostname,
		strings.TrimSpace(certBundle.CACertPEM),
		strings.TrimSpace(certBundle.CertPEM),
		strings.TrimSpace(certBundle.PrivateKeyPEM),
		serverEndpoint,
		deviceHostname,
		serverEndpoint)
}

// GenerateAndroidProfile emits the official strongSwan Android .sswan schema.
func GenerateAndroidProfile(serverFQDN, deviceHostname string, certBundle *CertificateBundle) string {
	profile, err := BuildAndroidProfile(serverFQDN, serverFQDN, deviceHostname, certBundle, nil)
	if err != nil {
		slog.Error("Android VPN profile generation failed", "error", err)
		return ""
	}
	return profile
}
