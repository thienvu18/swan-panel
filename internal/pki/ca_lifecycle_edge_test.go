package pki

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func freshTestCA(t *testing.T) *PKIManager {
	t.Helper()
	t.Setenv("APP_ENV", "test")
	t.Setenv("PKI_DATA_DIR", t.TempDir())
	manager := &PKIManager{}
	if err := manager.initCA(); err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestFreshRootCASubjectPersistsWithoutImplicitRotation(t *testing.T) {
	t.Setenv("CA_COMMON_NAME", "Engineering Root CA")
	t.Setenv("CA_ORGANIZATION", "Example Services")
	t.Setenv("CA_COUNTRY", "VN")
	t.Setenv("CA_PROVINCE", "Ho Chi Minh")
	t.Setenv("CA_LOCALITY", "District 1")
	manager := freshTestCA(t)
	original := manager.CAIdentity()
	if original.CommonName != "Engineering Root CA" || original.Organization != "Example Services" ||
		original.Country != "VN" || original.Province != "Ho Chi Minh" ||
		original.Locality != "District 1" || original.Serial == "" || original.NotAfter.IsZero() {
		t.Fatalf("custom root CA subject not persisted: %+v", original)
	}
	keyInfo, err := os.Stat(filepath.Join(manager.dataDir, "ca.key"))
	if err != nil {
		t.Fatal(err)
	}
	if keyInfo.Mode().Perm() != 0600 {
		t.Fatalf("root CA key insecure mode %v", keyInfo.Mode())
	}
	t.Setenv("CA_COMMON_NAME", "Some Other CA")
	reloaded := &PKIManager{}
	if err := reloaded.initCA(); err != nil {
		t.Fatal(err)
	}
	if reloaded.CAIdentity() != original {
		t.Fatal("existing CA silently changed when env subject changed")
	}
	if reloaded.GetCACertPEM() != manager.GetCACertPEM() {
		t.Fatal("reloading CA changed certificate")
	}
}

func TestRootCARejectsDamagedOrMissingMaterials(t *testing.T) {
	for _, tc := range []struct{ name, file, content string }{
		{"missing private key", "ca.key", ""},
		{"missing certificate", "ca.crt", ""},
		{"invalid private key", "ca.key", "not-pem"},
		{"invalid certificate", "ca.crt", "not-pem"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := freshTestCA(t)
			path := filepath.Join(manager.dataDir, tc.file)
			if tc.content == "" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			reloaded := &PKIManager{}
			if err := reloaded.initCA(); err == nil {
				t.Fatalf("%s was silently replaced with a new CA", tc.name)
			}
		})
	}
}

func TestRootCASubjectRotationRequiresExplicitValuesAndRetainsBackup(t *testing.T) {
	manager := freshTestCA(t)
	oldPEM := manager.GetCACertPEM()
	if err := manager.RotateRootCA(pkix.Name{}); err == nil {
		t.Fatal("empty CA subject accepted for rotation")
	}
	if err := manager.RotateRootCA(pkix.Name{CommonName: "Example"}); err == nil {
		t.Fatal("CA without Organization accepted")
	}
	err := manager.RotateRootCA(pkix.Name{CommonName: "Rotated QA Root", Organization: []string{"Integration QA"}, Country: []string{"VN"}})
	if err != nil {
		t.Fatal(err)
	}
	if manager.CAIdentity().CommonName != "Rotated QA Root" || manager.GetCACertPEM() == oldPEM {
		t.Fatal("confirmed CA rotation did not issue a new root")
	}
	entries, err := os.ReadDir(manager.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	foundCert, foundKey := false, false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "ca.crt.backup-") {
			foundCert = true
		}
		if strings.HasPrefix(entry.Name(), "ca.key.backup-") {
			foundKey = true
		}
	}
	if !foundCert || !foundKey {
		t.Fatal("rotation lost old root CA credentials")
	}
	newCert, _ := pem.Decode([]byte(manager.GetCACertPEM()))
	if newCert == nil {
		t.Fatal("rotated CA is not valid PEM")
	}
	cert, err := x509.ParseCertificate(newCert.Bytes)
	if err != nil || cert.CheckSignatureFrom(cert) != nil {
		t.Fatalf("rotated CA not self signed: %v", err)
	}
}

func TestGatewayCertificatesPreserveSANAndRefuseTamperedPrivateKeys(t *testing.T) {
	manager := freshTestCA(t)
	if err := manager.EnsureServerCertificates("", nil); err == nil {
		t.Fatal("empty server identity accepted")
	}
	if err := manager.EnsureServerCertificates("vpn.example.test", []string{"10.0.0.20", "not-an-ip"}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(manager.dataDir, "server.crt"))
	if err != nil {
		t.Fatal(err)
	}
	certPEM, _ := pem.Decode(first)
	if certPEM == nil {
		t.Fatal("missing server certificate PEM")
	}
	cert, err := x509.ParseCertificate(certPEM.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := cert.VerifyHostname("vpn.example.test"); err != nil {
		t.Fatal(err)
	}
	if err := cert.VerifyHostname("10.0.0.20"); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnsureServerCertificates("vpn.example.test", nil); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(manager.dataDir, "server.crt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("identical identity rotated a valid gateway certificate")
	}
	if err := manager.EnsureServerCertificates("new.example.test", nil); err != nil {
		t.Fatal(err)
	}
	changed, err := os.ReadFile(filepath.Join(manager.dataDir, "server.crt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(changed) == string(first) {
		t.Fatal("changed SAN reused old gateway certificate")
	}
	if err := os.WriteFile(filepath.Join(manager.dataDir, "server.key"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnsureServerCertificates("new.example.test", nil); err == nil {
		t.Fatal("corrupted server key silently replaced")
	}
}

func TestMissingGatewayServerKeyMustNotBeSilentlyReplaced(t *testing.T) {
	manager := freshTestCA(t)
	if err := manager.EnsureServerCertificates("vpn.example.test", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(manager.dataDir, "server.key")); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnsureServerCertificates("vpn.example.test", nil); err == nil {
		t.Fatal("partial gateway credential pair rotated without approval")
	}
}

// A Root CA change must also rotate its dedicated Apple profile signing leaf.
// Otherwise subsequent signed mobileconfig downloads fail signature verification.
func TestRootCARotationRenewsProfileSignerAndPreservesPreviousSignerBackup(t *testing.T) {
	manager := freshTestCA(t)
	beforeSignature, err := manager.SignAppleProfile([]byte("<plist><dict><key>Old</key></dict></plist>"))
	if err != nil || len(beforeSignature) == 0 {
		t.Fatalf("cannot issue initial signed profile: %v", err)
	}
	oldSigner, err := os.ReadFile(filepath.Join(manager.dataDir, "profile-signing.crt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RotateRootCA(pkix.Name{
		CommonName: "New Root For Apple Profiles", Organization: []string{"QA"}, Country: []string{"VN"},
	}); err != nil {
		t.Fatal(err)
	}
	newSignature, err := manager.SignAppleProfile([]byte("<plist><dict><key>New</key></dict></plist>"))
	if err != nil || len(newSignature) == 0 {
		t.Fatalf("rotated root cannot issue signed profile: %v", err)
	}
	newSigner, err := os.ReadFile(filepath.Join(manager.dataDir, "profile-signing.crt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(oldSigner) == string(newSigner) {
		t.Fatal("Apple profile signing leaf was not renewed after root rotation")
	}
	entries, err := os.ReadDir(manager.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	backupFound := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "profile-signing.crt.backup-") {
			backupFound = true
		}
	}
	if !backupFound {
		t.Fatal("old profile signer was not retained for disaster recovery")
	}
}
