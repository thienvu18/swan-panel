package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api"
	"github.com/swan-panel/swan-panel/internal/api/handlers"
	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/backup"
	"github.com/swan-panel/swan-panel/internal/config"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"github.com/swan-panel/swan-panel/internal/mfa"
	"github.com/swan-panel/swan-panel/internal/pki"
	"github.com/swan-panel/swan-panel/internal/policy"
)

func TestAuditInsertFailureRollsBackSecurityMutationsFailClosed(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)

	// Force SQLite insert failure on audit_logs via a test trigger to verify
	// security-critical domain mutations fail closed without orphaned partial writes.
	if err := db.DB.Exec(`CREATE TRIGGER trg_test_fail_audit_insert BEFORE INSERT ON audit_logs BEGIN SELECT RAISE(ABORT, 'simulated audit disk full'); END;`).Error; err != nil {
		t.Fatalf("install failing audit trigger: %v", err)
	}
	defer func() {
		_ = db.DB.Exec(`DROP TRIGGER IF EXISTS trg_test_fail_audit_insert`).Error
	}()

	rgResp := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name":       "Blocked Subnet",
		"cidr_block": "10.222.0.0/24",
	})
	if rgResp.Code < 400 {
		t.Fatalf("expected subnet creation to fail when audit persistence fails, got %d", rgResp.Code)
	}
	var rgCount int64
	_ = db.DB.Model(&db.ResourceGroup{}).Where("name = ?", "Blocked Subnet").Count(&rgCount).Error
	if rgCount != 0 {
		t.Fatalf("expected resource group write rolled back on audit failure, found %d", rgCount)
	}

	userResp := requestJSON(t, router, "POST", "/api/users", admin, map[string]any{
		"name":     "Blocked User",
		"email":    "blocked-audit@example.com",
		"password": "StrongBlockedUser!2026",
	})
	if userResp.Code < 400 {
		t.Fatalf("expected user creation to fail when audit persistence fails, got %d", userResp.Code)
	}
	var userCount int64
	_ = db.DB.Model(&db.User{}).Where("email = ?", "blocked-audit@example.com").Count(&userCount).Error
	if userCount != 0 {
		t.Fatalf("expected user write rolled back on audit failure, found %d", userCount)
	}

	backupResp := requestJSON(t, router, "POST", "/api/system/backup", admin, map[string]any{
		"passphrase": "valid-passphrase-2026",
	})
	if backupResp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected backup export to fail closed (503) when audit log cannot persist, got %d", backupResp.Code)
	}
}

func TestControlledRestartMaintenanceModeAndConcurrentBackupSafety(t *testing.T) {
	maintenance.DefaultCoordinator.ResetForTest()
	defer maintenance.DefaultCoordinator.ResetForTest()
	defer mfa.ResetKeyStoreForTest()

	gin.SetMode(gin.TestMode)
	t.Setenv("APP_ENV", "test")
	dbPath := filepath.Join(t.TempDir(), "restart_test.db")
	pkiDir := os.Getenv("PKI_DATA_DIR")

	if _, err := db.InitDB(dbPath); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if db.DB != nil {
			if s, err := db.DB.DB(); err == nil {
				_ = s.Close()
			}
		}
		db.DB = nil
	}()

	cfg := &config.Config{
		Port:        8080,
		DBPath:      dbPath,
		PKIDataDir:  pkiDir,
		JWTSecret:   "test-secret-key-12345",
		Environment: "test",
	}
	var restartCalled atomic.Bool
	router := api.SetupRouter(cfg, nil, func() {
		restartCalled.Store(true)
	})
	admin := getAdminToken(t, router)

	// Export valid backup while concurrent mutations run under snapshot coordination.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
				"name":       fmt.Sprintf("Concurrent Net %d", idx),
				"cidr_block": fmt.Sprintf("10.1%d.0.0/24", 40+idx),
			})
		}(i)
	}
	exportResp := requestJSON(t, router, "POST", "/api/system/backup", admin, map[string]any{
		"passphrase": "concurrent-safe-pass-2026",
	})
	wg.Wait()
	if exportResp.Code != http.StatusOK {
		t.Fatalf("backup export failed under concurrent load: %d %s", exportResp.Code, exportResp.Body.String())
	}

	// Trigger D01 controlled restart restore path.
	restoreResp := postMultipartRestore(t, router, admin, "backup.tar.enc", exportResp.Body.Bytes(), "concurrent-safe-pass-2026", "RESTORE BACKUP")
	var restoreMeta struct {
		RestoreID     string `json:"restore_id"`
		RuntimeStatus string `json:"runtime_status"`
	}
	if err := json.Unmarshal(restoreResp.Body.Bytes(), &restoreMeta); err != nil || restoreResp.Code != http.StatusAccepted || restoreMeta.RuntimeStatus != "restarting" || restoreMeta.RestoreID == "" {
		t.Fatalf("expected 202 restarting response with restore_id, got %d: %s (err=%v)", restoreResp.Code, restoreResp.Body.String(), err)
	}
	// Give async restart goroutine a brief moment to execute.
	for i := 0; i < 20 && !restartCalled.Load(); i++ {
		// yield via small syscall
		_, _ = http.Get("http://127.0.0.1:0/")
	}
	if !restartCalled.Load() {
		t.Fatal("expected controlled restart callback to be triggered")
	}

	// While in maintenance mode, /api/health and API endpoints return 503 Maintenance with Retry-After.
	healthReq, _ := http.NewRequest("GET", "/api/health", nil)
	healthW := httptest.NewRecorder()
	router.ServeHTTP(healthW, healthReq)
	if healthW.Code != http.StatusServiceUnavailable || !strings.Contains(healthW.Body.String(), `"maintenance":true`) {
		t.Fatalf("expected /api/health 503 maintenance during restart, got %d: %s", healthW.Code, healthW.Body.String())
	}
	apiReq, _ := http.NewRequest("GET", "/api/devices", nil)
	apiReq.Header.Set("Authorization", "Bearer "+admin)
	apiW := httptest.NewRecorder()
	router.ServeHTTP(apiW, apiReq)
	if apiW.Code != http.StatusServiceUnavailable || apiW.Header().Get("Retry-After") != "5" {
		t.Fatalf("expected /api/devices 503 + Retry-After: 5 during maintenance, got %d (%q)", apiW.Code, apiW.Header().Get("Retry-After"))
	}

	// Simulate fresh process startup running RecoverOrApplyPendingRestore with exclusive ownership.
	maintenance.DefaultCoordinator.ResetForTest()
	mgr, _ := pki.GetPKIManager()
	applied, err := backup.RecoverOrApplyPendingRestore(dbPath, mgr.DataDir(), mgr, nil)
	if err != nil || applied == nil {
		t.Fatalf("expected startup recovery apply to succeed: %v", err)
	}
	// Pre-restore JWT must now be invalid against restarted DB, while correlated health outcome reports applied.
	healthDoneReq, _ := http.NewRequest("GET", "/api/health?restore_id="+restoreMeta.RestoreID, nil)
	healthDoneW := httptest.NewRecorder()
	router.ServeHTTP(healthDoneW, healthDoneReq)
	if healthDoneW.Code != http.StatusOK || !strings.Contains(healthDoneW.Body.String(), `"restore_status":"applied"`) {
		t.Fatalf("expected correlated /api/health to report restore_status=applied, got %d: %s", healthDoneW.Code, healthDoneW.Body.String())
	}
	postReq, _ := http.NewRequest("GET", "/api/devices", nil)
	postReq.Header.Set("Authorization", "Bearer "+admin)
	postW := httptest.NewRecorder()
	router.ServeHTTP(postW, postReq)
	if postW.Code != http.StatusUnauthorized {
		t.Fatalf("expected pre-restore JWT rejected after startup restore, got %d: %s", postW.Code, postW.Body.String())
	}
}

func TestWebhookAPISanitizedErrorsAndDeliveriesEndpoint(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)

	// Private URL rejected even when APP_ENV is non-production (fail-safe D03).
	mustStatus(t, router, "POST", "/api/webhooks", admin, map[string]any{
		"name":          "Private SSRF Blocked",
		"provider_type": "generic",
		"webhook_url":   "https://10.0.0.50/secret",
		"events":        []string{"*"},
	}, http.StatusBadRequest)

	sentinel := "SECRET_WEBHOOK_PATH_99999"
	created := requestJSON(t, router, "POST", "/api/webhooks", admin, map[string]any{
		"name":           "Failure Sanitizer Hook",
		"provider_type":  "generic",
		"webhook_url":    "https://hooks.example.com/services/" + sentinel + "?token=QUERYSECRET",
		"signing_secret": "SIGNING_SECRET_XYZ",
		"events":         []string{"*"},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create hook failed: %s", created.Body.String())
	}
	hookID := responseID(t, created, "webhook")

	restore := audit.SetWebhookClientForTest(&http.Client{
		Transport: stubTransport(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("Post %q: context deadline exceeded", req.URL.String())
		}),
	})
	defer restore()

	testResp := requestJSON(t, router, "POST", "/api/webhooks/"+hookID+"/test", admin, map[string]any{})
	if testResp.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 test delivery failure, got %d", testResp.Code)
	}
	if strings.Contains(testResp.Body.String(), sentinel) || strings.Contains(testResp.Body.String(), "QUERYSECRET") || strings.Contains(testResp.Body.String(), "SIGNING_SECRET_XYZ") {
		t.Fatalf("test webhook error response leaked credentials: %s", testResp.Body.String())
	}

	// Verify /api/webhooks/deliveries endpoint requires admin & returns sanitized JSON array.
	deliveriesBody := mustStatus(t, router, "GET", "/api/webhooks/deliveries", admin, nil, http.StatusOK)
	var delivResp struct {
		Deliveries []json.RawMessage `json:"deliveries"`
	}
	if err := json.Unmarshal([]byte(deliveriesBody), &delivResp); err != nil {
		t.Fatalf("invalid deliveries payload: %v", err)
	}
	if bytes.Contains([]byte(deliveriesBody), []byte(sentinel)) {
		t.Fatalf("deliveries endpoint leaked sentinel URL secret")
	}
}

func TestOperationalSettingsDNSSessionsAndSetupStatusEndpoints(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)

	// Setup status reports initialized=true after bootstrap.
	mustStatus(t, router, "GET", "/api/auth/setup/status", "", nil, http.StatusOK)

	// Read-only settings, DNS, overrides, logs, stats, me, and offline VICI session/daemon endpoints.
	mustStatus(t, router, "GET", "/api/auth/me", admin, nil, http.StatusOK)
	mustStatus(t, router, "GET", "/api/resource-groups", admin, nil, http.StatusOK)
	mustStatus(t, router, "GET", "/api/dashboard/stats", admin, nil, http.StatusOK)
	mustStatus(t, router, "GET", "/api/system/logs?level=info&search=setup", admin, nil, http.StatusOK)
	mustStatus(t, router, "GET", "/api/settings/dns", admin, nil, http.StatusOK)
	mustStatus(t, router, "GET", "/api/dns/overrides", admin, nil, http.StatusOK)
	mustStatus(t, router, "GET", "/api/sessions", admin, nil, http.StatusServiceUnavailable)
	mustStatus(t, router, "GET", "/api/system/daemon/status", admin, nil, http.StatusServiceUnavailable)
	mustStatus(t, router, "GET", "/api/settings/strongswan", admin, nil, http.StatusOK)

	// Start and immediately cancel an isolated GatewayMonitor.Run loop cleanly (never mutate global DefaultGatewayMonitor).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	isolatedMonitor := &handlers.GatewayMonitor{}
	isolatedMonitor.Run(ctx)
	_ = isolatedMonitor.Snapshot()

	// Valid DNS override creation + delete roundtrip, plus validation negative cases.
	createdOvr := requestJSON(t, router, "POST", "/api/dns/overrides", admin, map[string]any{
		"hostname":       "intranet.internal",
		"record_type":    "A",
		"target_address": "10.20.30.40",
	})
	if createdOvr.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating DNS override, got %d: %s", createdOvr.Code, createdOvr.Body.String())
	}
	listedOvr := mustStatus(t, router, "GET", "/api/dns/overrides", admin, nil, http.StatusOK)
	var ovrList struct {
		Overrides []struct {
			ID string `json:"id"`
		} `json:"dns_overrides"`
	}
	if err := json.Unmarshal([]byte(listedOvr), &ovrList); err != nil || len(ovrList.Overrides) == 0 {
		t.Fatalf("expected created override in list, got %s err=%v", listedOvr, err)
	}
	mustStatus(t, router, "DELETE", "/api/dns/overrides/"+ovrList.Overrides[0].ID, admin, nil, http.StatusOK)

	// DNS override validation negative cases and missing delete target.
	mustStatus(t, router, "POST", "/api/dns/overrides", admin, map[string]any{
		"hostname":       "bad.internal",
		"record_type":    "A",
		"target_address": "not-an-ip",
	}, http.StatusBadRequest)
	mustStatus(t, router, "POST", "/api/dns/overrides", admin, map[string]any{
		"hostname":       "alias.internal",
		"record_type":    "CNAME",
		"target_address": "10.0.0.5",
	}, http.StatusBadRequest)
	mustStatus(t, router, "DELETE", "/api/dns/overrides/ovr_missing", admin, nil, http.StatusOK)

	// Missing settings/override/log tables fail safely with 500 without panicking.
	_ = db.DB.Migrator().DropTable(&db.DNSOverride{}, &db.DNSSettings{}, &db.StrongswanSettings{}, &db.AlertWebhook{})
	mustStatus(t, router, "GET", "/api/dns/overrides", admin, nil, http.StatusInternalServerError)
	mustStatus(t, router, "GET", "/api/settings/dns", admin, nil, http.StatusInternalServerError)
	mustStatus(t, router, "GET", "/api/settings/strongswan", admin, nil, http.StatusInternalServerError)
	mustStatus(t, router, "GET", "/api/webhooks", admin, nil, http.StatusInternalServerError)
}

func TestRevocationCRLAndConcurrentCARotationWithBackupHardening(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)

	// R02: Concurrent CA rotation and encrypted backup export must complete without RWMutex deadlock.
	var wg sync.WaitGroup
	wg.Add(2)
	rotCh := make(chan int, 1)
	bakCh := make(chan int, 1)
	go func() {
		defer wg.Done()
		resp := requestJSON(t, router, "POST", "/api/settings/ca/rotate", admin, map[string]any{
			"confirm":      "ROTATE CA",
			"common_name":  "Concurrent Safe Root CA",
			"organization": "SwanPanel QA",
			"country":      "US",
		})
		rotCh <- resp.Code
	}()
	go func() {
		defer wg.Done()
		resp := requestJSON(t, router, "POST", "/api/system/backup", admin, map[string]any{
			"passphrase": "concurrent-ca-backup-2026",
		})
		bakCh <- resp.Code
	}()
	wg.Wait()
	if code := <-rotCh; code != http.StatusOK {
		t.Fatalf("expected concurrent CA rotation to succeed cleanly, got %d", code)
	}
	if code := <-bakCh; code != http.StatusOK {
		t.Fatalf("expected concurrent backup export to succeed cleanly, got %d", code)
	}

	// R04: If CRL sync fails during device revocation, RuntimeApplyState must NOT claim "applied".
	rgBody := mustStatus(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name":       "R04 Test Net",
		"cidr_block": "10.210.0.0/24",
	}, http.StatusCreated)
	var rgResp struct {
		Group struct {
			ID string `json:"id"`
		} `json:"resource_group"`
	}
	_ = json.Unmarshal([]byte(rgBody), &rgResp)

	devBody := mustStatus(t, router, "POST", "/api/devices", admin, map[string]any{
		"hostname":           "r04-dev.internal",
		"device_type":        "company",
		"resource_group_id":  rgResp.Group.ID,
		"ip_allocation_type": "static",
		"virtual_ip":         "10.210.0.10",
		"platform":           "linux",
	}, http.StatusCreated)
	var devResp struct {
		Device struct {
			ID string `json:"id"`
		} `json:"device"`
	}
	_ = json.Unmarshal([]byte(devBody), &devResp)

	mgr, err := pki.GetPKIManager()
	if err != nil {
		t.Fatal(err)
	}
	crlPath := filepath.Join(mgr.DataDir(), "ca.crl")
	_ = os.RemoveAll(crlPath)
	if err := os.MkdirAll(crlPath, 0700); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(crlPath) }()

	revResp := requestJSON(t, router, "POST", "/api/devices/"+devResp.Device.ID+"/revoke", admin, nil)
	if revResp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 pending revocation when CRL sync fails, got %d: %s", revResp.Code, revResp.Body.String())
	}
	state, stErr := policy.GetApplyState(context.Background(), db.DB)
	if stErr != nil || state.Status != "failed" {
		t.Fatalf("R04 regression: expected RuntimeApplyState status=failed after CRL revocation error, got %q err=%v", state.Status, stErr)
	}
}
