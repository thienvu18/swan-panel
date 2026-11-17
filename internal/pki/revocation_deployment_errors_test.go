package pki

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignedRevocationListPublicationFailsClosedWhenStorageOrReloadIsUnavailable(t *testing.T) {
	t.Setenv("APP_ENV", "ci-integration")
	t.Setenv("PKI_DATA_DIR", t.TempDir())
	manager := &PKIManager{}
	if err := manager.initCA(); err != nil {
		t.Fatal(err)
	}

	t.Run("application revocation list destination is a directory", func(t *testing.T) {
		invalid := filepath.Join(manager.dataDir, "ca.crl")
		if err := os.Mkdir(invalid, 0700); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(invalid) })
		if err := manager.PublishCRL(context.Background(), nil); err == nil {
			t.Fatal("publishing over a directory must fail")
		}
		if err := os.Remove(invalid); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("daemon revocation list directory cannot be created", func(t *testing.T) {
		invalid := filepath.Join(t.TempDir(), "not-directory")
		if err := os.WriteFile(invalid, []byte("blocked"), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SWANPANEL_SWANCTL_CRL_DIR", invalid)
		if err := manager.PublishCRL(context.Background(), nil); err == nil ||
			!strings.Contains(err.Error(), "cannot install revocation list") {
			t.Fatalf("deployment error not reported: %v", err)
		}
	})

	t.Run("daemon rejects credential reload", func(t *testing.T) {
		t.Setenv("SWANPANEL_SWANCTL_CRL_DIR", t.TempDir())
		binDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(binDir, "swanctl"), []byte("#!/bin/sh\nexit 7\n"), 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
		t.Setenv("VICI_SOCKET", filepath.Join(t.TempDir(), "charon.vici"))
		if err := manager.PublishCRL(context.Background(), nil); err == nil ||
			!strings.Contains(err.Error(), "CRL reload failed") {
			t.Fatalf("reload failure not surfaced: %v", err)
		}
	})
}

func TestSwanctlRevocationDirectoryResolvesNativeDefaultAndConfiguredOverrides(t *testing.T) {
	t.Setenv("SWANPANEL_SWANCTL_CRL_DIR", "")
	t.Setenv("SWANPANEL_SWANCTL_DIR", "")
	if got := swanctlCRLDirectory(); got != "/etc/swanctl/x509crl" {
		t.Fatalf("native strongSwan CRL directory changed: %s", got)
	}
	root := t.TempDir()
	t.Setenv("SWANPANEL_SWANCTL_DIR", root)
	if got := swanctlCRLDirectory(); got != filepath.Join(root, "x509crl") {
		t.Fatalf("configured strongSwan root ignored: %s", got)
	}
	override := t.TempDir()
	t.Setenv("SWANPANEL_SWANCTL_CRL_DIR", override)
	if got := swanctlCRLDirectory(); got != override {
		t.Fatalf("dedicated CRL directory ignored: %s", got)
	}
}
