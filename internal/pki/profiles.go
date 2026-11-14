package pki

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/google/uuid"
)

func safeXML(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}

func randomProfilePassword() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func caDER(b *CertificateBundle) ([]byte, error) {
	if b == nil {
		return nil, errors.New("missing certificate bundle")
	}
	block, _ := pem.Decode([]byte(b.CACertPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("invalid CA PEM")
	}
	return block.Bytes, nil
}

// BuildAppleProfile generates a valid native IKEv2 profile using a PKCS#12
// identity, not a public PEM certificate lacking the private key.
// The .mobileconfig is sensitive even when PKCS#12 carries a password.
func BuildAppleProfile(endpoint, serverIdentity, hostname string, b *CertificateBundle) (string, error) {
	ca, err := caDER(b)
	if err != nil {
		return "", err
	}
	password, err := randomProfilePassword()
	if err != nil {
		return "", err
	}
	p12, err := GeneratePKCS12(b, password, false)
	if err != nil {
		return "", err
	}
	profileUUID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("swanpanel:profile:"+hostname)).String()
	caUUID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("swanpanel:ca:"+hostname)).String()
	keyUUID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("swanpanel:identity:"+hostname)).String()
	vpnUUID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("swanpanel:ikev2:"+hostname)).String()
	xmlProfile := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadDisplayName</key><string>SwanPanel VPN (%s)</string>
<key>PayloadIdentifier</key><string>com.swanpanel.profile.%s</string>
<key>PayloadUUID</key><string>%s</string>
<key>PayloadContent</key><array>
    <dict>
        <key>PayloadType</key><string>com.apple.security.root</string>
        <key>PayloadVersion</key><integer>1</integer>
        <key>PayloadDisplayName</key><string>SwanPanel Root CA</string>
        <key>PayloadIdentifier</key><string>com.swanpanel.ca.%s</string>
        <key>PayloadUUID</key><string>%s</string>
        <key>PayloadContent</key><data>%s</data>
    </dict>
    <dict>
        <key>PayloadType</key><string>com.apple.security.pkcs12</string>
        <key>PayloadVersion</key><integer>1</integer>
        <key>PayloadDisplayName</key><string>SwanPanel Client Identity</string>
        <key>PayloadIdentifier</key><string>com.swanpanel.identity.%s</string>
        <key>PayloadUUID</key><string>%s</string>
        <key>PayloadContent</key><data>%s</data>
        <key>Password</key><string>%s</string>
    </dict>
    <dict>
        <key>PayloadType</key><string>com.apple.vpn.managed</string>
        <key>PayloadVersion</key><integer>1</integer>
        <key>PayloadDisplayName</key><string>SwanPanel IKEv2</string>
        <key>PayloadIdentifier</key><string>com.swanpanel.vpn.%s</string>
        <key>PayloadUUID</key><string>%s</string>
        <key>UserDefinedName</key><string>SwanPanel VPN</string>
        <key>VPNType</key><string>IKEv2</string>
        <key>IKEv2</key><dict>
            <key>RemoteAddress</key><string>%s</string>
            <key>RemoteIdentifier</key><string>%s</string>
            <key>LocalIdentifier</key><string>%s</string>
            <key>AuthenticationMethod</key><string>Certificate</string>
            <key>PayloadCertificateUUID</key><string>%s</string>
            <key>ExtendedAuthEnabled</key><integer>0</integer>
            <key>DeadPeerDetectionRate</key><string>Medium</string>
            <key>DisableMOBIKE</key><integer>0</integer>
        </dict>
    </dict>
</array>
</dict></plist>`,
		safeXML(hostname), profileUUID, profileUUID, caUUID, caUUID, base64.StdEncoding.EncodeToString(ca),
		keyUUID, keyUUID, base64.StdEncoding.EncodeToString(p12), password,
		vpnUUID, vpnUUID, safeXML(endpoint), safeXML(serverIdentity), safeXML(hostname), keyUUID)
	return xmlProfile, nil
}

// BuildWindowsInstaller installs the client *machine* certificate with its
// private key, then creates an all-user machine-certificate IKEv2 connection.
func BuildWindowsInstaller(endpoint, hostname string, b *CertificateBundle) (string, error) {
	return BuildWindowsInstallerWithRoutes(endpoint, hostname, b, nil)
}
func BuildWindowsInstallerWithRoutes(endpoint, hostname string, b *CertificateBundle, routes []string) (string, error) {
	ca, err := caDER(b)
	if err != nil {
		return "", err
	}
	password, err := randomProfilePassword()
	if err != nil {
		return "", err
	}
	pfx, err := GeneratePKCS12(b, password, true)
	if err != nil {
		return "", err
	}
	endpoint = strings.ReplaceAll(endpoint, "'", "''")
	hostname = strings.ReplaceAll(hostname, "'", "''")
	return fmt.Sprintf(`# SwanPanel Windows native IKEv2 machine-certificate setup
# Run as Administrator. The script contains a sensitive device identity.
$ErrorActionPreference = 'Stop'
$vpnName = 'SwanPanel VPN (%s)'
$ca = [Convert]::FromBase64String('%s')
$caPath = Join-Path $env:TEMP ('swanpanel-ca-' + [guid]::NewGuid().ToString() + '.cer')
try {
    [System.IO.File]::WriteAllBytes($caPath, $ca)
    Import-Certificate -FilePath $caPath -CertStoreLocation Cert:\LocalMachine\Root | Out-Null
} finally {
    Remove-Item -Path $caPath -Force -ErrorAction SilentlyContinue
}
$pfx = [Convert]::FromBase64String('%s')
$pfxPassword = ConvertTo-SecureString '%s' -AsPlainText -Force
$pfxPath = Join-Path $env:TEMP ('swanpanel-' + [guid]::NewGuid().ToString() + '.pfx')
try {
    [System.IO.File]::WriteAllBytes($pfxPath, $pfx)
    Import-PfxCertificate -FilePath $pfxPath -CertStoreLocation Cert:\LocalMachine\My -Password $pfxPassword -Exportable:$false | Out-Null
} finally {
    Remove-Item -Path $pfxPath -Force -ErrorAction SilentlyContinue
}
$old = Get-VpnConnection -Name $vpnName -AllUserConnection -ErrorAction SilentlyContinue
if ($old) {
    Set-VpnConnection -Name $vpnName -AllUserConnection -ServerAddress '%s' -TunnelType Ikev2 -AuthenticationMethod MachineCertificate -SplitTunneling $true -Force
} else {
    Add-VpnConnection -Name $vpnName -AllUserConnection -ServerAddress '%s' -TunnelType Ikev2 -AuthenticationMethod MachineCertificate -SplitTunneling $true -Force
}
Set-VpnConnectionIPsecConfiguration -ConnectionName $vpnName -AllUserConnection -AuthenticationTransformConstants GCMAES256 -CipherTransformConstants GCMAES256 -EncryptionMethod GCMAES256 -IntegrityCheckMethod SHA384 -DHGroup ECP384 -PfsGroup ECP384 -Force
Write-Host 'SwanPanel IKEv2 VPN configured.'`, hostname, base64.StdEncoding.EncodeToString(ca), base64.StdEncoding.EncodeToString(pfx), password, endpoint, endpoint) + windowsRouteCommands(routes), nil
}

// BuildAndroidProfile targets the strongSwan VPN Client app. Android does not
// support silent installation of a native VPN profile from arbitrary .sswan JSON.

func windowsRouteCommands(routes []string) string {
	var lines strings.Builder
	for _, route := range routes {
		// Compiler canonicalizes and validates policy targets; reject any command injection.
		if _, err := netip.ParsePrefix(route); err != nil {
			continue
		}
		lines.WriteString("\nAdd-VpnConnectionRoute -ConnectionName $vpnName -AllUserConnection -DestinationPrefix '")
		lines.WriteString(route)
		lines.WriteString("' -ErrorAction SilentlyContinue")
	}
	return lines.String()
}

func BuildAndroidProfile(endpoint, serverIdentity, hostname string, b *CertificateBundle, routes []string) (string, error) {
	ca, err := caDER(b)
	if err != nil {
		return "", err
	}
	// strongSwan Android imports an unprotected P12. Treat .sswan as a secret.
	p12, err := GeneratePKCS12(b, "", false)
	if err != nil {
		return "", err
	}
	local := map[string]any{"id": hostname, "p12": base64.StdEncoding.EncodeToString(p12)}
	remote := map[string]any{"addr": endpoint, "id": serverIdentity, "cert": base64.StdEncoding.EncodeToString(ca)}
	profile := map[string]any{
		"uuid":   uuid.NewSHA1(uuid.NameSpaceURL, []byte("swanpanel:android:"+hostname)).String(),
		"name":   "SwanPanel (" + hostname + ")",
		"type":   "ikev2-cert",
		"remote": remote,
		"local":  local,
	}
	if len(routes) > 0 {
		profile["split-tunneling"] = map[string]any{"subnets": routes}
	}
	raw, err := json.MarshalIndent(profile, "", "  ")
	return string(raw), err
}

func BuildLinuxInstaller(endpoint, serverIdentity, hostname string, b *CertificateBundle, routes []string) (string, error) {
	if b == nil || b.PrivateKeyPEM == "" {
		return "", errors.New("missing client identity")
	}
	if !validDeviceID.MatchString(strings.ReplaceAll(hostname, ".", "-")) {
		return "", errors.New("unsafe device hostname")
	}
	for _, r := range routes {
		if _, err := netip.ParsePrefix(r); err != nil {
			return "", fmt.Errorf("invalid route: %w", err)
		}
	}
	remoteTS := "0.0.0.0/32" // Do not hijack the default route for a device with no grants.
	if len(routes) > 0 {
		remoteTS = strings.Join(routes, ",")
	}
	// Exact same identity and server ID used on the strongSwan side.
	return fmt.Sprintf(`#!/usr/bin/env bash
# SwanPanel Linux strongSwan native IKEv2 installer
set -euo pipefail
if [[ "$EUID" -ne 0 ]]; then echo "Run as root: sudo bash $0"; exit 1; fi
install -d -m 0700 /etc/swanctl/rsa
install -d -m 0755 /etc/swanctl/x509ca /etc/swanctl/x509 /etc/swanctl/conf.d
umask 077
cat > /etc/swanctl/x509ca/swanpanel-ca.crt <<'SWAN_CA'
%s
SWAN_CA
cat > /etc/swanctl/x509/swanpanel-client.crt <<'SWAN_CERT'
%s
SWAN_CERT
cat > /etc/swanctl/rsa/swanpanel-client.key <<'SWAN_KEY'
%s
SWAN_KEY
chmod 0600 /etc/swanctl/rsa/swanpanel-client.key
cat > /etc/swanctl/conf.d/swanpanel.conf <<'SWAN_CONFIG'
connections {
    swanpanel {
        version = 2
        remote_addrs = %s
        vips = 0.0.0.0
        mobike = yes
        local {
            auth = pubkey
            certs = swanpanel-client.crt
            id = %s
        }
        remote {
            auth = pubkey
            id = %s
            cacerts = swanpanel-ca.crt
        }
        children {
            swanpanel-traffic {
                remote_ts = %s
                start_action = none
                dpd_action = restart
                mode = tunnel
                esp_proposals = aes256gcm16-ecp384,aes256-sha256
            }
        }
        proposals = aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048
    }
}
SWAN_CONFIG
if ! command -v swanctl >/dev/null 2>&1; then
  echo "Install strongSwan with swanctl before connecting"; exit 1
fi
swanctl --load-all
swanctl --initiate --child swanpanel-traffic
echo "SwanPanel VPN connected; inspect with swanctl --list-sas"
`, strings.TrimSpace(b.CACertPEM), strings.TrimSpace(b.CertPEM), strings.TrimSpace(b.PrivateKeyPEM),
		endpoint, hostname, serverIdentity, remoteTS), nil
}

func BuildLinuxConfig(endpoint, serverIdentity, hostname string, routes []string) (string, error) {
	if endpoint == "" || serverIdentity == "" || hostname == "" {
		return "", errors.New("incomplete IKEv2 identity")
	}
	if !validDeviceID.MatchString(strings.ReplaceAll(hostname, ".", "-")) {
		return "", errors.New("invalid hostname")
	}
	remoteTS := "0.0.0.0/32"
	for _, r := range routes {
		if _, err := netip.ParsePrefix(r); err != nil {
			return "", err
		}
	}
	if len(routes) > 0 {
		remoteTS = strings.Join(routes, ",")
	}
	return fmt.Sprintf(`connections {
    swanpanel {
        version = 2
        remote_addrs = %s
        vips = 0.0.0.0
        mobike = yes
        local {
            auth = pubkey
            certs = swanpanel-client.crt
            id = %s
        }
        remote {
            auth = pubkey
            id = %s
            cacerts = swanpanel-ca.crt
        }
        children {
            swanpanel-traffic {
                remote_ts = %s
                start_action = none
                dpd_action = restart
                mode = tunnel
                esp_proposals = aes256gcm16-ecp384,aes256-sha256
            }
        }
        proposals = aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048
    }
}
`, endpoint, hostname, serverIdentity, remoteTS), nil
}
