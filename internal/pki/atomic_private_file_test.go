package pki

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicPrivateFilePreservesPrivatePermissions(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "identities")
	target := filepath.Join(directory, "client-key.pem")
	secret := []byte("sensitive private-key material")
	if err := atomicPrivateFile(target, secret); err != nil {
		t.Fatalf("write private-key file: %v", err)
	}
	actual, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, secret) {
		t.Fatal("private-key file contents changed")
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("private-key permissions %o, expected 0600", got)
	}
	dirInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("private-key directory permissions %o, expected 0700", got)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "client-key.pem" {
		t.Fatalf("temporary private-key files were left behind: %v", entries)
	}
}

func TestAtomicPrivateFileCleansUpFailedRename(t *testing.T) {
	directory := t.TempDir()
	// Replacing an existing directory with a regular file must fail.
	if err := atomicPrivateFile(directory, []byte("should-not-persist")); err == nil {
		t.Fatal("unexpectedly replaced a directory with private-key contents")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary private-key file leaked after failed rename: %v", entries)
	}
}
