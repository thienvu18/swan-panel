package pki

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
)

// BuildAndroidNativeBundle exports credentials and instructions for Android's
// built-in IKEv2/IPsec RSA client (where available; OEM support varies).
// Android does not support installing a native VPN profile automatically via JSON.
func BuildAndroidNativeBundle(endpoint, serverIdentity, hostname string, bundle *CertificateBundle) ([]byte, error) {
	if bundle == nil || bundle.PrivateKeyPEM == "" {
		return nil, errors.New("device identity required")
	}
	ca, err := caDER(bundle)
	if err != nil {
		return nil, err
	}
	password, err := randomProfilePassword()
	if err != nil {
		return nil, err
	}
	p12, err := GeneratePKCS12(bundle, password, true)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	add := func(name string, data []byte) error {
		entry, err := writer.Create(name)
		if err != nil {
			return err
		}
		_, err = entry.Write(data)
		return err
	}
	if err := add("swanpanel-client.p12", p12); err != nil {
		return nil, err
	}
	if err := add("swanpanel-ca.cer", ca); err != nil {
		return nil, err
	}
	instructions := fmt.Sprintf(`SwanPanel — Android native IKEv2/IPsec RSA
Device identity: %s
VPN server address: %s
Server certificate identity: %s
P12 import password: %s

1. Unpack locally. Treat the entire archive as a secret: it contains the private key.
2. Android Settings > Security > Install certificate > VPN & app user certificate.
3. Select swanpanel-client.p12; enter the password above.
4. Install swanpanel-ca.cer as a CA certificate where supported.
5. Settings > Network & Internet > VPN > Add VPN.
6. Choose IKEv2/IPsec RSA if present; server address: %s.
7. Select the imported client certificate; use the identity %s.
8. Verify certificate/server identity and connect.
9. Delete extracted credentials and the archive after installation.
If the OEM does not offer IKEv2/IPsec RSA, use the strongSwan app and the .sswan export instead.
`, hostname, endpoint, serverIdentity, password, endpoint, hostname)
	if err := add("README-ANDROID-NATIVE.txt", []byte(instructions)); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
