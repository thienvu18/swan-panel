package pki_test

import (
	"strings"
	"testing"

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
	mobileconfig := pki.GenerateAppleMobileconfig("vpn.company.com", "test-laptop", clientBundle)
	if !strings.Contains(mobileconfig, "com.apple.vpn.managed") {
		t.Errorf("Mobileconfig missing com.apple.vpn.managed payload")
	}
	if !strings.Contains(mobileconfig, "vpn.company.com") {
		t.Errorf("Mobileconfig missing server FQDN")
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
}
