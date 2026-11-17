package pki

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestGatewayCredentialDeploymentUsesConfiguredSwanctlDirectoryAndProtectsPrivateKeys(t *testing.T) {
	t.Setenv("APP_ENV", "ci-integration")
	data := t.TempDir()
	system := t.TempDir()
	t.Setenv("PKI_DATA_DIR", data)
	t.Setenv("SWANPANEL_SWANCTL_DIR", system)
	manager := &PKIManager{}
	if err := manager.initCA(); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnsureServerCertificates("vpn.example.test", []string{"192.0.2.9"}); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(system, "rsa", "server.key")
	caFile := filepath.Join(system, "x509ca", "ca.crt")
	certFile := filepath.Join(system, "x509", "server.crt")
	key, err := os.ReadFile(keyFile)
	if err != nil || len(key) == 0 {
		t.Fatalf("gateway key not installed: %v", err)
	}
	info, err := os.Stat(keyFile)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("gateway key not owner-only: %+v %v", info, err)
	}
	ca, err := os.ReadFile(caFile)
	if err != nil || !bytes.Equal(ca, []byte(manager.GetCACertPEM())) {
		t.Fatalf("installed root mismatch: %v", err)
	}
	certificate, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certificate)
	if block == nil {
		t.Fatal("invalid server certificate PEM")
	}
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil || parsed.VerifyHostname("vpn.example.test") != nil || parsed.VerifyHostname("192.0.2.9") != nil {
		t.Fatalf("gateway SANs invalid: %+v %v", parsed, err)
	}
	if err := manager.EnsureServerCertificates("vpn.example.test", []string{"192.0.2.9"}); err != nil {
		t.Fatal(err)
	}
	same, err := os.ReadFile(keyFile)
	if err != nil || !bytes.Equal(key, same) {
		t.Fatal("idempotent credential deployment rotated private key")
	}
	// Adding a new DNS gateway endpoint must rotate the certificate once.
	if err := manager.EnsureServerCertificates("vpn.example.test", []string{"192.0.2.9", "edge.example.test"}); err != nil {
		t.Fatal(err)
	}
	updatedPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	updatedBlock, _ := pem.Decode(updatedPEM)
	if updatedBlock == nil {
		t.Fatal("updated certificate is not PEM")
	}
	updatedCert, err := x509.ParseCertificate(updatedBlock.Bytes)
	if err != nil || updatedCert.VerifyHostname("edge.example.test") != nil ||
		updatedCert.VerifyHostname("192.0.2.9") != nil || updatedCert.VerifyHostname("vpn.example.test") != nil {
		t.Fatalf("gateway certificate SAN not updated: %+v %v", updatedCert, err)
	}
	if bytes.Equal(certificate, updatedPEM) {
		t.Fatal("new gateway endpoint reused a certificate without the new SAN")
	}
	if err := manager.EnsureServerCertificates("vpn.example.test", []string{"192.0.2.9", "edge.example.test"}); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(certFile)
	if err != nil || !bytes.Equal(updatedPEM, again) {
		t.Fatal("unchanged gateway endpoints rotated certificate unnecessarily")
	}
	// PublishCRL reloads credentials in production. In this test the
	// swanctl binary is deliberately replaced with a local command.
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "swanctl"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if err := manager.PublishCRL(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	crl, err := os.ReadFile(filepath.Join(system, "x509crl", "swanpanel.crl"))
	if err != nil || len(crl) == 0 {
		t.Fatalf("signed CRL not deployed: %v", err)
	}
}

func TestGatewayCredentialDeploymentRefusesBrokenSwanctlDirectory(t *testing.T) {
	t.Setenv("APP_ENV", "ci-integration")
	t.Setenv("PKI_DATA_DIR", t.TempDir())
	manager := &PKIManager{}
	if err := manager.initCA(); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "swanctl-file-not-dir")
	if err := os.WriteFile(root, []byte("inaccessible"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWANPANEL_SWANCTL_DIR", root)
	if err := manager.EnsureServerCertificates("vpn.example.test", nil); err == nil {
		t.Fatal("gateway ignored invalid destination for credentials")
	}
}

// A failed installation of any credential must not be reported as success.
// Root CA and gateway private-key material are intentionally left untouched.
func TestGatewayCredentialDeploymentRejectsBlockedCertificateFiles(t *testing.T) {
	t.Setenv("APP_ENV", "ci-integration")
	t.Setenv("PKI_DATA_DIR", t.TempDir())
	manager := &PKIManager{}
	if err := manager.initCA(); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"x509ca/ca.crt", "x509/server.crt", "rsa/server.key"} {
		t.Run(relative, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("SWANPANEL_SWANCTL_DIR", dir)
			if err := os.MkdirAll(filepath.Join(dir, relative), 0700); err != nil {
				t.Fatal(err)
			}
			if err := manager.EnsureServerCertificates("vpn.example.test", nil); err == nil {
				t.Fatal("writing certificate material over a directory succeeded")
			}
		})
	}
}
