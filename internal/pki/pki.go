package pki

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
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

// initCA generates or loads the Root CA
func (m *PKIManager) initCA() error {
	slog.Info("Initializing internal SwanPanel X.509 Root CA")

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

	slog.Info("Root CA initialized successfully", "serial", serialNumber.Text(16), "cn", parsedCA.Subject.CommonName)
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
            id = %s
        }

        children {
            swanpanel-traffic {
                remote_ts = 0.0.0.0/0
                start_action = start
                dpd_action = restart
                close_action = restart
            }
        }

        version = 2
        mobike = yes
        encap = yes
        proposals = aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048
    }
}
`, serverFQDN, deviceHostname, serverFQDN)
}
