package db

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newBootstrapDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "setup.sqlite")
	handle, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		raw, err := handle.DB()
		if err != nil {
			t.Errorf("get bootstrap test database: %v", err)
			return
		}
		if err := raw.Close(); err != nil {
			t.Errorf("close bootstrap test database: %v", err)
		}
	})
	return path
}

func readSetupToken(t *testing.T) string {
	t.Helper()
	info, err := os.Stat(BootstrapTokenFile())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("setup token must be 0600, got %o", info.Mode().Perm())
	}
	raw, err := os.ReadFile(BootstrapTokenFile())
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(string(raw))
	if len(token) != 64 {
		t.Fatalf("expected 256-bit random token, got %d hex characters", len(token))
	}
	return token
}

func TestOneTimeBootstrap(t *testing.T) {
	newBootstrapDB(t)
	var count int64
	if err := DB.Model(&User{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("new installation contains default users: count=%d err=%v", count, err)
	}
	token := readSetupToken(t)
	required, err := SetupRequired()
	if err != nil || !required {
		t.Fatal("initial setup must be required")
	}
	bad := []struct{ name, token, nameInput, email, password string }{
		{"wrong token", "bogus", "Root", "root@example.com", "a-very-long-password"},
		{"short password", token, "Root", "root@example.com", "short"},
		{"empty name", token, "", "root@example.com", "a-very-long-password"},
		{"empty email", token, "Root", "", "a-very-long-password"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := CreateInitialAdmin(tc.token, tc.nameInput, tc.email, tc.password); err == nil {
				t.Fatal("invalid setup input accepted")
			}
			pending, err := SetupRequired()
			if err != nil || !pending {
				t.Fatal("bad input consumed token")
			}
		})
	}
	admin, err := CreateInitialAdmin(token, "Root", "ROOT@EXAMPLE.COM", "a-very-long-password")
	if err != nil {
		t.Fatal(err)
	}
	if !admin.IsAdmin || admin.Status != "ACTIVE" || admin.AuthType != "local" || admin.Email != "root@example.com" {
		t.Fatalf("created admin invalid: %+v", admin)
	}
	if _, err := CreateInitialAdmin(token, "Second", "other@example.com", "another-strong-password"); err == nil {
		t.Fatal("token was accepted twice")
	}
	if _, err := os.Stat(BootstrapTokenFile()); !os.IsNotExist(err) {
		t.Fatal("token file not deleted")
	}
	pending, err := SetupRequired()
	if err != nil || pending {
		t.Fatal("setup remains open after admin creation")
	}
}

func TestBootstrapTokenStableAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.sqlite")
	first, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	token := readSetupToken(t)
	sqlFirst, err := first.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlFirst.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		raw, err := second.DB()
		if err != nil {
			t.Errorf("get reopened bootstrap database: %v", err)
			return
		}
		if err := raw.Close(); err != nil {
			t.Errorf("close reopened bootstrap database: %v", err)
		}
	})
	if again := readSetupToken(t); again != token {
		t.Fatal("restart regenerated one-time token")
	}
}

func TestConcurrentBootstrapSingleWinner(t *testing.T) {
	newBootstrapDB(t)
	token := readSetupToken(t)
	var wg sync.WaitGroup
	results := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := CreateInitialAdmin(token, "Root", "root@example.com", "a-very-long-password")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("expected exactly one successful setup; got %d", success)
	}
	var count int64
	if err := DB.Model(&User{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("expected exactly one account; count=%d error=%v", count, err)
	}
}

func TestBootstrapMissingTokenFileFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.sqlite")
	db1, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := db1.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(BootstrapTokenFile()); err != nil {
		t.Fatal(err)
	}
	if _, err := InitDB(path); err == nil {
		t.Fatal("missing bootstrap token silently reset")
	}
}
