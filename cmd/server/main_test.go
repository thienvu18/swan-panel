package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/swan-panel/swan-panel/internal/backup"
	"github.com/swan-panel/swan-panel/internal/config"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"github.com/swan-panel/swan-panel/internal/mfa"
)

func TestControlPlaneStartupHelpersVersionFlagAndFailClosedListener(t *testing.T) {
	var out bytes.Buffer
	if handleVersionFlag([]string{"swan-panel"}, &out) {
		t.Fatal("expected false without --version flag")
	}
	if !handleVersionFlag([]string{"swan-panel", "--version"}, &out) {
		t.Fatal("expected true for --version flag")
	}
	if !strings.Contains(out.String(), "SwanPanel") {
		t.Fatalf("expected SwanPanel in version output, got %q", out.String())
	}

	maintenance.DefaultCoordinator.ResetForTest()
	defer maintenance.DefaultCoordinator.ResetForTest()
	defer mfa.ResetKeyStoreForTest()

	tmp := t.TempDir()
	t.Setenv("APP_ENV", "test")
	cfg := &config.Config{
		Port:        0,
		DBPath:      filepath.Join(tmp, "startup.db"),
		PKIDataDir:  filepath.Join(tmp, "pki"),
		JWTSecret:   "startup-test-jwt-secret-1234567890",
		Environment: "test",
		LogLevel:    "info",
	}

	manifest, failClosed := runStartupRecovery(cfg.DBPath, cfg.PKIDataDir)
	if manifest != nil || failClosed {
		t.Fatalf("expected clean startup recovery without pending restore, got manifest=%v failClosed=%v", manifest, failClosed)
	}

	conn, err := initializeDatabaseAndMFA(cfg)
	if err != nil || conn == nil {
		t.Fatalf("initializeDatabaseAndMFA failed: %v", err)
	}
	t.Cleanup(func() {
		if db.DB != nil {
			if s, err := db.DB.DB(); err == nil {
				_ = s.Close()
			}
		}
		db.DB = nil
	})
	if mfa.ActiveKey("") == "" {
		t.Fatal("expected active MFA key after initializeDatabaseAndMFA")
	}

	// Simulate failed rollback journal on disk so startup enters fail-closed maintenance HTTP mode.
	_ = os.MkdirAll(backup.DBWorkDir(cfg.DBPath), 0700)
	_ = os.WriteFile(filepath.Join(backup.DBWorkDir(cfg.DBPath), "journal.json"), []byte(`{"state":"rollback_failed","last_error":"simulated double fault"}`), 0600)
	_, failClosed = runStartupRecovery(cfg.DBPath, cfg.PKIDataDir)
	if !failClosed {
		t.Fatal("expected failClosed=true when rollback_failed journal exists")
	}

	// H02: Corrupt or unrecognized restore journals MUST also enter fail-closed mode.
	maintenance.DefaultCoordinator.ResetForTest()
	_ = os.WriteFile(filepath.Join(backup.DBWorkDir(cfg.DBPath), "journal.json"), []byte(`{"state":`), 0600)
	_, failClosed = runStartupRecovery(cfg.DBPath, cfg.PKIDataDir)
	if !failClosed || maintenance.DefaultCoordinator.CurrentStatus().Mode != maintenance.ModeFailedClosed {
		t.Fatalf("expected failClosed=true on corrupt JSON restore journal, got failClosed=%v mode=%s", failClosed, maintenance.DefaultCoordinator.CurrentStatus().Mode)
	}

	maintenance.DefaultCoordinator.ResetForTest()
	_ = os.WriteFile(filepath.Join(backup.DBWorkDir(cfg.DBPath), "journal.json"), []byte(`{"state":"corrupt_unknown"}`), 0600)
	_, failClosed = runStartupRecovery(cfg.DBPath, cfg.PKIDataDir)
	if !failClosed || maintenance.DefaultCoordinator.CurrentStatus().Mode != maintenance.ModeFailedClosed {
		t.Fatalf("expected failClosed=true on unrecognized journal state, got failClosed=%v mode=%s", failClosed, maintenance.DefaultCoordinator.CurrentStatus().Mode)
	}
	maintenance.DefaultCoordinator.ResetForTest()
	_ = os.RemoveAll(backup.DBWorkDir(cfg.DBPath))

	quit := make(chan os.Signal, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- serveFailClosedMode(cfg, nil, quit, "127.0.0.1:0")
	}()
	quit <- syscall.SIGTERM
	if serveErr := <-errCh; serveErr != nil {
		t.Fatalf("serveFailClosedMode shutdown returned error: %v", serveErr)
	}

	srv := newControlPlaneHTTPServer(":0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 from handler, got %d", rr.Code)
	}
}
