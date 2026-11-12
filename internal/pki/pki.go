package pki

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CertificateBundle holds PEM-encoded certificate, private key, and CA certificate
type CertificateBundle struct {
	CertPEM        string
	PrivateKeyPEM  string
	CACertPEM      string
	Fingerprint    string
	CertDER        []byte
	PrivateKeyDER  []byte
	SerialNumber   string
}

// PKIManager handles internal X.509 Certificate Authority operations
type PKIManager struct {
	mu        sync.RWMutex
	caCert    *x509.Certificate
	caKey     *rsa.PrivateKey
	caCertPEM string
	caKeyPEM  string
}

// DefaultPKI is the global in-memory singleton PKI manager
var DefaultPKI *PKIManager
var once sync.Once

// GetPKIManager initializes and returns the default PKI manager
func GetPKIManager() (*PKIManager, error) {
	var initErr error
	once.Do(func() {
		pki := &PKIManager{}
		if err := pki.initCA(); err != nil {
			initErr = err
			return
		}
		DefaultPKI = pki
	})
	if initErr != nil {
		return nil, initErr
	}
	return DefaultPKI, nil
}

// resolvePKIDataDir discovers or creates a writable persistent directory for PKI state
func resolvePKIDataDir() string {
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
	caCertPath := filepath.Join(pkiDir, "ca.crt")
	caKeyPath := filepath.Join(pkiDir, "ca.key")

	// 1. Attempt loading existing persisted CA certificate and private key
	if certData, err1 := os.ReadFile(caCertPath); err1 == nil {
		if keyData, err2 := os.ReadFile(caKeyPath); err2 == nil {
			certBlock, _ := pem.Decode(certData)
			if certBlock != nil {
				parsedCA, err := x509.ParseCertificate(certBlock.Bytes)
				if err == nil && time.Now().Before(parsedCA.NotAfter) {
					parsedKey, errKey := parseRSAPrivateKeyFromPEM(keyData)
					if errKey == nil {
						m.caCert = parsedCA
						m.caKey = parsedKey
						m.caCertPEM = string(certData)
						m.caKeyPEM = string(keyData)
						slog.Info("Root CA loaded from persistent storage", "path", caCertPath, "serial", parsedCA.SerialNumber.Text(16), "cn", parsedCA.Subject.CommonName)
						return nil
					}
					slog.Warn("Failed to parse persisted CA private key; regenerating", "error", errKey)
				} else {
					slog.Warn("Persisted CA certificate expired or invalid; regenerating")
				}
			}
		}
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

	caTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:  []string{"SwanPanel Root Authority"},
			CommonName:    "SwanPanel Internal Root CA",
			Country:       []string{"US"},
			Province:      []string{"California"},
			Locality:      []string{"San Francisco"},
		},
		NotBefore:             time.Now().Add(-10 * time.Minute),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 years validity
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		MaxPathLen:            1,
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
		slog.Warn("Failed to persist CA certificate to disk", "path", caCertPath, "error", err)
	}
	if err := os.WriteFile(caKeyPath, keyBuf.Bytes(), 0600); err != nil {
		slog.Warn("Failed to persist CA private key to disk", "path", caKeyPath, "error", err)
	}

	// Synchronize to /etc/swanctl/x509ca/ if possible
	etcCADir := "/etc/swanctl/x509ca"
	if err := os.MkdirAll(etcCADir, 0755); err == nil {
		_ = os.WriteFile(filepath.Join(etcCADir, "ca.crt"), certBuf.Bytes(), 0644)
	}

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

	fingerprint := fmt.Sprintf("%x", serialNumber)

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
		DNSNames:              []string{serverFQDN},
		IPAddresses:           ipAddresses,
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
func (m *PKIManager) EnsureServerCertificates(serverFQDN string, serverIPs []string) error {
	caDir := "/etc/swanctl/x509ca"
	serverCertDir := "/etc/swanctl/x509"
	serverKeyDir := "/etc/swanctl/rsa"
	dataDir := "/var/lib/swan-panel"

	_ = os.MkdirAll(caDir, 0755)
	_ = os.MkdirAll(serverCertDir, 0755)
	_ = os.MkdirAll(serverKeyDir, 0700)
	_ = os.MkdirAll(dataDir, 0750)

	// Write CA cert
	if m.caCertPEM != "" {
		_ = os.WriteFile(filepath.Join(dataDir, "ca.crt"), []byte(m.caCertPEM), 0644)
		_ = os.WriteFile(filepath.Join(caDir, "ca.crt"), []byte(m.caCertPEM), 0644)
	}

	if serverFQDN == "" {
		serverFQDN = "swan-panel"
	}

	// Default IP SANs and DNS names
	allIPs := append([]string{"127.0.0.1", "172.18.0.2"}, serverIPs...)
	bundle, err := m.GenerateServerCertificate(serverFQDN, allIPs)
	if err != nil {
		return err
	}

	_ = os.WriteFile(filepath.Join(dataDir, "server.crt"), []byte(bundle.CertPEM), 0644)
	_ = os.WriteFile(filepath.Join(serverCertDir, "server.crt"), []byte(bundle.CertPEM), 0644)

	_ = os.WriteFile(filepath.Join(dataDir, "server.key"), []byte(bundle.PrivateKeyPEM), 0600)
	_ = os.WriteFile(filepath.Join(serverKeyDir, "server.key"), []byte(bundle.PrivateKeyPEM), 0600)

	slog.Info("Server certificates and CA synchronized to /etc/swanctl/ for strongSwan daemon", "fqdn", serverFQDN)

	if _, err := exec.LookPath("swanctl"); err == nil {
		_ = exec.Command("swanctl", "--load-creds").Run()
	}

	return nil
}

// GenerateAppleMobileconfig creates an Apple .mobileconfig XML profile for macOS and iOS
func GenerateAppleMobileconfig(serverFQDN, deviceHostname string, certBundle *CertificateBundle) string {
	caCertB64 := base64.StdEncoding.EncodeToString([]byte(certBundle.CACertPEM))
	clientCertB64 := base64.StdEncoding.EncodeToString([]byte(certBundle.CertPEM))
	clientKeyB64 := base64.StdEncoding.EncodeToString([]byte(certBundle.PrivateKeyPEM))

	profileUUID := fmt.Sprintf("swanpanel-vpn-%s", deviceHostname)
	vpnUUID := fmt.Sprintf("swanpanel-ikev2-%s", deviceHostname)
	caUUID := fmt.Sprintf("swanpanel-ca-%s", deviceHostname)
	clientCertUUID := fmt.Sprintf("swanpanel-client-%s", deviceHostname)

	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadDisplayName</key>
	<string>SwanPanel VPN (%s)</string>
	<key>PayloadIdentifier</key>
	<string>com.swanpanel.vpn.%s</string>
	<key>PayloadRemovalDisallowed</key>
	<false/>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>%s</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
	<key>PayloadContent</key>
	<array>
		<!-- Root CA Certificate Payload -->
		<dict>
			<key>PayloadType</key>
			<string>com.apple.security.root</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>PayloadIdentifier</key>
			<string>com.swanpanel.ca.%s</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadDisplayName</key>
			<string>SwanPanel Root CA</string>
			<key>PayloadContent</key>
			<data>%s</data>
		</dict>
		<!-- Client Certificate Payload -->
		<dict>
			<key>PayloadType</key>
			<string>com.apple.security.pem</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>PayloadIdentifier</key>
			<string>com.swanpanel.cert.%s</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadDisplayName</key>
			<string>SwanPanel Client Certificate (%s)</string>
			<key>PayloadContent</key>
			<data>%s</data>
		</dict>
		<!-- Native IKEv2 VPN Payload -->
		<dict>
			<key>PayloadType</key>
			<string>com.apple.vpn.managed</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>PayloadIdentifier</key>
			<string>com.swanpanel.vpn.config.%s</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadDisplayName</key>
			<string>SwanPanel IKEv2 (%s)</string>
			<key>UserDefinedName</key>
			<string>SwanPanel VPN</string>
			<key>VPNType</key>
			<string>IKEv2</string>
			<key>IKEv2</key>
			<dict>
				<key>RemoteAddress</key>
				<string>%s</string>
				<key>RemoteIdentifier</key>
				<string>%s</string>
				<key>LocalIdentifier</key>
				<string>%s</string>
				<key>AuthenticationMethod</key>
				<string>Certificate</string>
				<key>PayloadCertificateUUID</key>
				<string>%s</string>
				<key>ExtendedAuthEnabled</key>
				<integer>0</integer>
				<key>DeadPeerDetectionRate</key>
				<string>Medium</string>
				<key>DisableMOBIKE</key>
				<integer>0</integer>
				<key>DisableRedirect</key>
				<integer>0</integer>
				<key>EnablePFS</key>
				<integer>1</integer>
				<key>EnableCertificateRevocationCheck</key>
				<integer>0</integer>
			</dict>
		</dict>
	</array>
</dict>
</plist>`,
		deviceHostname, deviceHostname, profileUUID,
		deviceHostname, caUUID, caCertB64,
		deviceHostname, clientCertUUID, deviceHostname, clientCertB64,
		deviceHostname, vpnUUID, deviceHostname,
		serverFQDN, serverFQDN, deviceHostname, clientCertUUID,
	)
	_ = clientKeyB64
	return xml
}

// GenerateWindowsPowerShellScript creates a PowerShell script to install certs and add native VPN connection
func GenerateWindowsPowerShellScript(serverFQDN, deviceHostname string, certBundle *CertificateBundle) string {
	return fmt.Sprintf(`# SwanPanel Windows Native IKEv2 VPN Setup Script
# Run this script with Administrator privileges in PowerShell

$ErrorActionPreference = "Stop"
Write-Host "[SwanPanel] Installing native IKEv2 VPN configuration for %s..." -ForegroundColor Cyan

# 1. Save and import Root CA
$caCertPEM = @"
%s
"@
$caCertPath = "$env:TEMP\swanpanel-ca.crt"
Set-Content -Path $caCertPath -Value $caCertPEM
Import-Certificate -FilePath $caCertPath -CertStoreLocation Cert:\LocalMachine\Root | Out-Null
Remove-Item -Path $caCertPath -Force
Write-Host "[SwanPanel] Root CA installed into LocalMachine\Root store." -ForegroundColor Green

# 2. Add or update native IKEv2 VPN Connection
$vpnName = "SwanPanel VPN"
$existingVpn = Get-VpnConnection -Name $vpnName -ErrorAction SilentlyContinue

if ($existingVpn) {
    Write-Host "[SwanPanel] Updating existing VPN connection '$vpnName'..." -ForegroundColor Yellow
    Set-VpnConnection -Name $vpnName -ServerAddress "%s" -TunnelType Ikev2 -AuthenticationMethod MachineCertificate -EncryptionLevel Required -SplitTunneling $true
} else {
    Write-Host "[SwanPanel] Creating new native IKEv2 VPN connection '$vpnName'..." -ForegroundColor Green
    Add-VpnConnection -Name $vpnName -ServerAddress "%s" -TunnelType Ikev2 -AuthenticationMethod MachineCertificate -EncryptionLevel Required -SplitTunneling $true
}

# 3. Configure robust IPsec cryptography for Windows
Set-VpnConnectionIPsecConfiguration -ConnectionName $vpnName -AuthenticationTransformConstants GCMAES256 -CipherTransformConstants GCMAES256 -EncryptionMethod GCMAES256 -IntegrityCheckMethod SHA384 -DHGroup Group14 -PfsGroup PFS2048 -Force

Write-Host "[SwanPanel] Setup completed successfully! You can now connect from Windows Settings > Network > VPN." -ForegroundColor Cyan
`, deviceHostname, strings.TrimSpace(certBundle.CACertPEM), serverFQDN, serverFQDN)
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

// GenerateAndroidProfile creates a native strongSwan Android app configuration profile (.sswan JSON format)
func GenerateAndroidProfile(serverFQDN, deviceHostname string, certBundle *CertificateBundle) string {
	profileUUID := fmt.Sprintf("swanpanel-android-%s", deviceHostname)
	profileName := fmt.Sprintf("SwanPanel (%s)", deviceHostname)

	profile := map[string]interface{}{
		"uuid": profileUUID,
		"name": profileName,
		"type": "ikev2-cert",
		"remote": map[string]interface{}{
			"addr": serverFQDN,
			"id":   serverFQDN,
		},
		"local": map[string]interface{}{
			"id":   deviceHostname,
			"cert": strings.TrimSpace(certBundle.CertPEM),
			"key":  strings.TrimSpace(certBundle.PrivateKeyPEM),
		},
		"remote-ca": map[string]interface{}{
			"cert": strings.TrimSpace(certBundle.CACertPEM),
		},
		"split-tunneling": map[string]interface{}{
			"block-ipv4": false,
			"block-ipv6": false,
		},
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		slog.Error("Failed to marshal Android .sswan profile", "error", err)
		return "{}"
	}
	return string(data)
}
