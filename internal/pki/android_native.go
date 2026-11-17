package pki

import "errors"

// AndroidNativeSetup is a one-response credential handoff for Android's
// built-in IKEv2/IPsec RSA client. The PKCS#12 password is generated for
// exactly these bytes; clients must not fetch the password and file separately.
// []byte is base64 encoded by Go's JSON encoder.
type AndroidNativeSetup struct {
	ClientP12      []byte `json:"client_p12"`
	RootCA         []byte `json:"root_ca"`
	ImportPassword string `json:"import_password"`
	ServerAddress  string `json:"server_address"`
	ServerIdentity string `json:"server_identity"`
	ClientIdentity string `json:"client_identity"`
}

// BuildAndroidNativeSetup never installs software or changes device settings.
// Android user consent is required to import keys into the OS keystore.
func BuildAndroidNativeSetup(endpoint, serverIdentity, hostname string, bundle *CertificateBundle) (*AndroidNativeSetup, error) {
	if bundle == nil || bundle.PrivateKeyPEM == "" {
		return nil, errors.New("device identity required")
	}
	if endpoint == "" || serverIdentity == "" || hostname == "" {
		return nil, errors.New("VPN endpoint and identities required")
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
	return &AndroidNativeSetup{
		ClientP12:      p12,
		RootCA:         ca,
		ImportPassword: password,
		ServerAddress:  endpoint,
		ServerIdentity: serverIdentity,
		ClientIdentity: hostname,
	}, nil
}
