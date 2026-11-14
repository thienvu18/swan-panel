package pki_test

import (
	"strings"
	"testing"

	"github.com/smallstep/pkcs7"
	"github.com/swan-panel/swan-panel/internal/pki"
)

func TestPKIManager(t *testing.T) {
	mgr, err := pki.GetPKIManager()
	if err != nil {
		t.Fatalf("Failed to initialize PKI manager: %v", err)
	}

	caPEM := mgr.GetCACertPEM()
	if !strings.Contains(caPEM, "BEGIN CERTIFICATE") {
		t.Errorf("Expected CA PEM to contain 'BEGIN CERTIFICATE', got: %s", caPEM)
	}

	// 1. Test Client Certificate Generation
	clientBundle, err := mgr.GenerateClientCertificate("test-laptop", "user@company.com")
	if err != nil {
		t.Fatalf("Failed to generate client certificate: %v", err)
	}

	if !strings.Contains(clientBundle.CertPEM, "BEGIN CERTIFICATE") {
		t.Errorf("Invalid client certificate PEM")
	}
	if !strings.Contains(clientBundle.PrivateKeyPEM, "BEGIN RSA PRIVATE KEY") {
		t.Errorf("Invalid client private key PEM")
	}
	if clientBundle.Fingerprint == "" {
		t.Errorf("Expected client certificate fingerprint to be non-empty")
	}

	// 2. Test Apple Mobileconfig Generation
	mobileconfig, err := pki.GenerateAppleMobileconfig("vpn.company.com", "test-laptop", clientBundle)
	if err != nil {
		t.Fatalf("sign Apple profile: %v", err)
	}
	parsed, err := pkcs7.Parse(mobileconfig)
	if err != nil {
		t.Fatalf("parse signed Apple mobileconfig: %v", err)
	}
	if err := parsed.Verify(); err != nil {
		t.Fatalf("verify Apple mobileconfig signature: %v", err)
	}
	if !strings.Contains(string(parsed.Content), "com.apple.vpn.managed") {
		t.Error("signed mobileconfig missing IKEv2 VPN payload")
	}
	if !strings.Contains(string(parsed.Content), "vpn.company.com") {
		t.Error("signed mobileconfig missing gateway name")
	}

	// 3. Test Windows PowerShell Script Generation
	psScript := pki.GenerateWindowsPowerShellScript("vpn.company.com", "test-laptop", clientBundle)
	if !strings.Contains(psScript, "Add-VpnConnection") {
		t.Errorf("PowerShell script missing Add-VpnConnection")
	}

	// 4. Test Linux swanctl configuration
	swanctlConf := pki.GenerateSwanctlConfig("vpn.company.com", "test-laptop", clientBundle)
	if !strings.Contains(swanctlConf, "connections {") {
		t.Errorf("swanctl.conf missing connections block")
	}

	// 5. Test Android strongSwan profile generation (.sswan JSON)
	androidProfile := pki.GenerateAndroidProfile("vpn.company.com", "test-laptop", clientBundle)
	if !strings.Contains(androidProfile, "ikev2-cert") {
		t.Errorf("Android profile missing ikev2-cert type")
	}
	if !strings.Contains(androidProfile, "vpn.company.com") {
		t.Errorf("Android profile missing server FQDN")
	}
	if !strings.Contains(androidProfile, "test-laptop") {
		t.Errorf("Android profile missing device hostname")
	}
}
