package pki_test

import (
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/pki"
)

func TestCRLMultiDeviceAndRevocationReasons(t *testing.T) {
	manager, err := pki.GetPKIManager()
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.GenerateClientCertificate("rev-a", "rev-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.GenerateClientCertificate("rev-b", "rev-b")
	if err != nil {
		t.Fatal(err)
	}
	moment := time.Now().UTC().Add(-time.Minute)
	entries := []pki.RevokedCertificate{
		{Serial: first.SerialNumber, RevokedAt: moment},
		{Serial: second.SerialNumber, RevokedAt: moment},
	}
	der, err := manager.CRL(entries)
	if err != nil {
		t.Fatal(err)
	}
	list, err := x509.ParseRevocationList(der)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.RevokedCertificateEntries) != 2 {
		t.Fatalf("expected 2 revocations, got %d", len(list.RevokedCertificateEntries))
	}
	block, _ := pem.Decode([]byte(manager.GetCACertPEM()))
	if block == nil {
		t.Fatal("missing CA")
	}
	root, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := list.CheckSignatureFrom(root); err != nil {
		t.Fatalf("CRL signature invalid: %v", err)
	}
	found := map[string]bool{}
	for _, rev := range list.RevokedCertificateEntries {
		found[rev.SerialNumber.Text(16)] = true
		if rev.ReasonCode != 1 {
			t.Fatalf("revocation reason is not keyCompromise: %d", rev.ReasonCode)
		}
	}
	if !found[first.SerialNumber] || !found[second.SerialNumber] {
		t.Fatal("missing revoked serial in signed CRL")
	}
	if list.NextUpdate.Before(time.Now().Add(23 * time.Hour)) {
		t.Fatal("CRL expiry is too short for renewal schedule")
	}
}

func TestCRLRejectsMalformedSerial(t *testing.T) {
	manager, err := pki.GetPKIManager()
	if err != nil {
		t.Fatal(err)
	}
	for _, serial := range []string{"", "not-hex", "-f"} {
		if _, err := manager.CRL([]pki.RevokedCertificate{{Serial: serial, RevokedAt: time.Now()}}); err == nil {
			t.Errorf("invalid serial %q accepted", serial)
		}
	}
}

func TestRevokedSerialNotAnArbitraryNumber(t *testing.T) {
	manager, err := pki.GetPKIManager()
	if err != nil {
		t.Fatal(err)
	}
	device, err := manager.GenerateClientCertificate("revoke-test", "revoke-test")
	if err != nil {
		t.Fatal(err)
	}
	if serial, ok := new(big.Int).SetString(device.SerialNumber, 16); !ok || serial.Sign() <= 0 {
		t.Fatalf("issued certificate has invalid serial %q", device.SerialNumber)
	}
}
