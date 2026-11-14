package pki_test

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smallstep/pkcs7"
	"github.com/swan-panel/swan-panel/internal/pki"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "swanpanel-pki-tests-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("PKI_DATA_DIR", dir)
	_ = os.Setenv("APP_ENV", "test")
	status := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(status)
}

func signedProfileFixture(t *testing.T) (*pki.PKIManager, []byte, *x509.Certificate) {
	t.Helper()
	manager, err := pki.GetPKIManager()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := manager.GenerateClientCertificate("device-one", "device-one")
	if err != nil {
		t.Fatal(err)
	}
	xmlProfile, err := pki.BuildAppleProfile("vpn.example.com", "vpn.example.com", "device-one", bundle)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(xmlProfile, "com.apple.security.pkcs12") {
		t.Fatal("missing embedded Apple identity")
	}
	cmsDER, err := manager.SignAppleProfile([]byte(xmlProfile))
	if err != nil {
		t.Fatal(err)
	}
	caBlock, _ := pem.Decode([]byte(manager.GetCACertPEM()))
	if caBlock == nil {
		t.Fatal("missing CA cert")
	}
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return manager, cmsDER, ca
}

func TestAppleMobileconfigSMIMESignedData(t *testing.T) {
	_, cmsDER, ca := signedProfileFixture(t)
	if bytes.HasPrefix(cmsDER, []byte("<?xml")) {
		t.Fatal("Apple profile was returned unsigned")
	}
	signed, err := pkcs7.Parse(cmsDER)
	if err != nil {
		t.Fatalf("not valid CMS/PKCS#7: %v", err)
	}
	if !bytes.HasPrefix(signed.Content, []byte("<?xml")) {
		t.Fatal("CMS missing attached plist content")
	}
	if !bytes.Contains(signed.Content, []byte("com.apple.vpn.managed")) {
		t.Fatal("signed payload lacks VPN settings")
	}
	if signed.GetOnlySigner() == nil {
		t.Fatal("CMS signer missing")
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	if err := signed.VerifyWithChain(roots); err != nil {
		t.Fatalf("profile signed data does not verify with SwanPanel CA: %v", err)
	}
	if signer := signed.GetOnlySigner(); signer.IsCA {
		t.Fatal("CA private key was used directly as the signing key")
	}
}

func TestAppleSignedProfileDetectsModification(t *testing.T) {
	_, cmsDER, ca := signedProfileFixture(t)
	parsed, err := pkcs7.Parse(cmsDER)
	if err != nil {
		t.Fatal(err)
	}
	pos := bytes.Index(cmsDER, []byte("com.apple.vpn.managed"))
	if pos < 0 {
		t.Fatal("missing attached content in CMS")
	}
	changed := bytes.Clone(cmsDER)
	changed[pos] ^= 0x01
	tampered, err := pkcs7.Parse(changed)
	if err == nil {
		roots := x509.NewCertPool()
		roots.AddCert(ca)
		if err := tampered.VerifyWithChain(roots); err == nil {
			t.Fatal("mutated mobileconfig still verifies")
		}
	}
	if len(parsed.Content) == 0 {
		t.Fatal("original CMS content missing")
	}
}

func TestAppleProfileSignerPersistsAndIsPrivate(t *testing.T) {
	manager, first, ca := signedProfileFixture(t)
	second, err := manager.SignAppleProfile([]byte("<?xml version=\"1.0\"?><plist/>"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := pkcs7.Parse(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := pkcs7.Parse(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.GetOnlySigner().Raw, b.GetOnlySigner().Raw) {
		t.Fatal("signing certificate changed between profile downloads")
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	if err := b.VerifyWithChain(roots); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(os.Getenv("PKI_DATA_DIR"), "profile-signing.key")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("profile signing key permission is %o; expected 0600", info.Mode().Perm())
	}
}

func TestAppleProfileSigningRejectsInvalidContent(t *testing.T) {
	manager, err := pki.GetPKIManager()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SignAppleProfile(nil); err == nil {
		t.Fatal("empty profile was signed")
	}
}

func TestAppleProfileUntrustedSignerDoesNotVerify(t *testing.T) {
	_, cmsDER, _ := signedProfileFixture(t)
	parsed, err := pkcs7.Parse(cmsDER)
	if err != nil {
		t.Fatal(err)
	}
	if err := parsed.VerifyWithChain(x509.NewCertPool()); err == nil {
		t.Fatal("signed profile verified without a trusted CA")
	}
}
