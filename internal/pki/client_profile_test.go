package pki_test

import (
	"archive/zip"
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/pki"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

func TestPersistedIdentityAndNativeProfiles(t *testing.T) {
	manager, err := pki.GetPKIManager()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := manager.GenerateClientCertificate("test-vpn-device", "test-vpn-device")
	if err != nil {
		t.Fatal(err)
	}
	id := "dev_" + uuid.NewString()
	if err := manager.StoreDeviceBundle(id, bundle); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DeleteDeviceBundle(id); err != nil {
			t.Errorf("remove temporary device identity: %v", err)
		}
	})
	if err := manager.StoreDeviceBundle(id, bundle); err == nil {
		t.Fatal("overwrote issued certificate unexpectedly")
	}
	stored, err := manager.LoadDeviceBundle(id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SerialNumber != bundle.SerialNumber || stored.PrivateKeyPEM != bundle.PrivateKeyPEM {
		t.Fatal("device identity was not stable across downloads")
	}

	apple, err := pki.BuildAppleProfile("vpn.example.net", "vpn.example.net", "test-vpn-device", stored)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(apple, "com.apple.security.pkcs12") || !strings.Contains(apple, "com.apple.vpn.managed") {
		t.Fatal("Apple profile missing PKCS#12 identity or native IKEv2 payload")
	}
	if strings.Contains(apple, stored.PrivateKeyPEM) {
		t.Fatal("Apple configuration exposes a raw PEM private key")
	}

	windows, err := pki.BuildWindowsInstaller("vpn.example.net", "test-vpn-device", stored)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(windows, "Import-PfxCertificate") || !strings.Contains(windows, "Import-Certificate") {
		t.Fatal("Windows installer does not install the private key and CA")
	}

	android, err := pki.BuildAndroidProfile("vpn.example.net", "vpn.example.net", "test-vpn-device", stored, []string{"10.20.0.0/16"})
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Local struct {
			P12 string `json:"p12"`
		} `json:"local"`
		Split struct {
			Subnets []string `json:"subnets"`
		} `json:"split-tunneling"`
	}
	if err := json.Unmarshal([]byte(android), &payload); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.Local.P12)
	if err != nil {
		t.Fatal(err)
	}
	_, cert, err := pkcs12.Decode(decoded, "")
	if err != nil {
		t.Fatal(err)
	}
	if cert.SerialNumber.Text(16) != bundle.SerialNumber {
		t.Fatal("Android P12 has the wrong device certificate")
	}
	if len(payload.Split.Subnets) != 1 || payload.Split.Subnets[0] != "10.20.0.0/16" {
		t.Fatal("Android split-tunnel routes missing")
	}

	archive, err := pki.BuildAndroidNativeBundle("vpn.example.net", "vpn.example.net", "test-vpn-device", stored)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, entry := range reader.File {
		names[entry.Name] = true
	}
	for _, want := range []string{"swanpanel-client.p12", "swanpanel-ca.cer", "README-ANDROID-NATIVE.txt"} {
		if !names[want] {
			t.Fatalf("Android native bundle missing %s", want)
		}
	}
}

func TestSignedRevocationList(t *testing.T) {
	manager, err := pki.GetPKIManager()
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(manager.GetCACertPEM()))
	if block == nil {
		t.Fatal("missing Root CA")
	}
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(ca.SubjectKeyId) == 0 {
		t.Fatal("new Root CA lacks SubjectKeyId")
	}
	bundle, err := manager.GenerateClientCertificate("revoked-test-device", "revoked-test-device")
	if err != nil {
		t.Fatal(err)
	}
	der, err := manager.CRL([]pki.RevokedCertificate{{Serial: bundle.SerialNumber, RevokedAt: time.Now().Add(-time.Minute)}})
	if err != nil {
		t.Fatal(err)
	}
	list, err := x509.ParseRevocationList(der)
	if err != nil {
		t.Fatal(err)
	}
	if err := list.CheckSignatureFrom(ca); err != nil {
		t.Fatal(err)
	}
	if len(list.RevokedCertificateEntries) != 1 || list.RevokedCertificateEntries[0].SerialNumber.Text(16) != bundle.SerialNumber {
		t.Fatal("CRL does not contain revoked client certificate")
	}
}

func TestTamperedStoredIdentityFailsClosed(t *testing.T) {
	manager, err := pki.GetPKIManager()
	if err != nil {
		t.Fatal(err)
	}
	original, err := manager.GenerateClientCertificate("tamper-a", "tamper-a")
	if err != nil {
		t.Fatal(err)
	}
	other, err := manager.GenerateClientCertificate("tamper-b", "tamper-b")
	if err != nil {
		t.Fatal(err)
	}
	id := "dev_" + uuid.NewString()
	if err := manager.StoreDeviceBundle(id, original); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DeleteDeviceBundle(id); err != nil {
			t.Errorf("remove tampered test identity: %v", err)
		}
	})
	path := filepath.Join(os.Getenv("PKI_DATA_DIR"), "devices", id+".json")
	corrupted := *original
	corrupted.PrivateKeyPEM = other.PrivateKeyPEM
	encoded, err := json.Marshal(corrupted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.LoadDeviceBundle(id); err == nil {
		t.Fatal("tampered private key was accepted")
	}
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.LoadDeviceBundle(id); err == nil {
		t.Fatal("malformed persisted identity JSON was accepted")
	}
}

func TestPKCS12WrongPasswordRejected(t *testing.T) {
	manager, err := pki.GetPKIManager()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := manager.GenerateClientCertificate("p12-device", "p12-device")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := pki.GeneratePKCS12(bundle, "correct-password", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := pkcs12.DecodeChain(raw, "incorrect-password"); err == nil {
		t.Fatal("PKCS#12 accepted an invalid password")
	}
	privateKey, cert, chain, err := pkcs12.DecodeChain(raw, "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if privateKey == nil || cert == nil {
		t.Fatal("P12 is missing a private key or client certificate")
	}
	if cert.SerialNumber.Text(16) != bundle.SerialNumber {
		t.Fatal("P12 contains wrong device identity")
	}
	if len(chain) != 1 {
		t.Fatalf("P12 must include the expected root CA certificate, got %d CA certificates", len(chain))
	}
}
