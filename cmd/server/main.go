package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	swanpanel "github.com/swan-panel/swan-panel"
	"github.com/swan-panel/swan-panel/internal/api"
	"github.com/swan-panel/swan-panel/internal/api/handlers"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/backup"
	"github.com/swan-panel/swan-panel/internal/config"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/dns"
	"github.com/swan-panel/swan-panel/internal/logger"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"github.com/swan-panel/swan-panel/internal/mfa"
	"github.com/swan-panel/swan-panel/internal/pki"
	"github.com/swan-panel/swan-panel/internal/policy"
	"github.com/swan-panel/swan-panel/internal/vici"
	"gorm.io/gorm"
)

// version is set using -ldflags "-X main.version=vX.Y.Z" in release builds.
var version = "dev"

func handleVersionFlag(args []string, out io.Writer) bool {
	if len(args) == 2 && (args[1] == "--version" || args[1] == "-version") {
		_, _ = fmt.Fprintln(out, "SwanPanel", version)
		return true
	}
	return false
}

func runStartupRecovery(dbPath, pkiDir string) (*backup.Manifest, bool) {
	recoveredManifest, restoreErr := backup.RecoverOrApplyPendingRestore(dbPath, pkiDir, nil, nil)
	switch {
	case restoreErr == nil:
		if recoveredManifest != nil {
			slog.Info("Startup backup restore applied cleanly before serving", "backup_created_at", recoveredManifest.CreatedAt)
		}
		return recoveredManifest, false
	case errors.Is(restoreErr, backup.ErrRestoredRolledBack):
		slog.Error("Startup restore activation failed and safely rolled back to prior state", "error", restoreErr)
		return nil, false
	default:
		slog.Error("CRITICAL: Startup restore recovery failed in an unsafe or unknown state; locking fail-closed", "error", restoreErr)
		maintenance.DefaultCoordinator.EnterFailedClosed("Startup restore recovery failed; inspect restore journal before serving")
		return nil, true
	}
}

func newControlPlaneHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}
}

func serveFailClosedMode(cfg *config.Config, staticFS fs.FS, quit <-chan os.Signal, listenAddr string) error {
	router := api.SetupRouter(cfg, staticFS)
	if listenAddr == "" {
		listenAddr = fmt.Sprintf(":%d", cfg.Port)
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	srv := newControlPlaneHTTPServer(listenAddr, router)
	errCh := make(chan error, 1)
	go func() {
		slog.Error("Control plane running in fail-closed recovery mode", "address", "http://"+ln.Addr().String())
		if listenErr := srv.Serve(ln); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			errCh <- listenErr
		}
	}()
	select {
	case listenErr := <-errCh:
		return listenErr
	case <-quit:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func initializeDatabaseAndMFA(cfg *config.Config) (*gorm.DB, error) {
	dbConn := db.DB
	if dbConn == nil {
		var err error
		dbConn, err = db.InitDB(cfg.DBPath)
		if err != nil {
			return nil, fmt.Errorf("initialize SQLite database: %w", err)
		}
	}
	tables, _ := dbConn.Migrator().GetTables()
	slog.Info("SQLite database connected successfully", "wal_mode", true, "tables", len(tables))

	_, keyPath, mfaErr := mfa.InitActiveKey(dbConn, cfg.DBPath, cfg.JWTSecret)
	if mfaErr != nil {
		return nil, fmt.Errorf("initialize MFA key (%s): %w", keyPath, mfaErr)
	}
	slog.Info("Persistent MFA encryption keystore ready", "path", keyPath)
	return dbConn, nil
}

func main() {
	if handleVersionFlag(os.Args, os.Stdout) {
		return
	}
	// 1. Load configuration and initialize structured logger
	cfg := config.Load()
	logger.InitLogger(cfg.LogLevel, cfg.Environment)
	if cfg.MFAKeyFile != "" {
		_ = os.Setenv("MFA_KEY_FILE", cfg.MFAKeyFile)
	}
	// Log notifications invalidate the admin logs view; payloads never leave the server.
	var lastLogSignal atomic.Int64
	logger.SetLogEventPublisher(func() {
		now := time.Now().UnixMilli()
		previous := lastLogSignal.Load()
		if now-previous < 750 || !lastLogSignal.CompareAndSwap(previous, now) {
			return
		}
		realtime.DefaultBroker.Broadcast("log_event", nil)
	})

	slog.Info("Starting SwanPanel gateway",
		"environment", cfg.Environment,
		"log_level", cfg.LogLevel,
		"port", cfg.Port,
		"db_path", cfg.DBPath,
	)

	// Exclusive startup restore recovery / journal activation BEFORE opening listeners or workers.
	_, failedClosedMode := runStartupRecovery(cfg.DBPath, cfg.PKIDataDir)

	staticFS, err := swanpanel.StaticFS()
	if err != nil {
		if cfg.Environment == "production" {
			slog.Error("Production frontend bundle unavailable; refusing to start", "error", err)
			os.Exit(1)
		}
		slog.Info("Embedded frontend not available; use the Vite development UI", "error", err)
	} else {
		slog.Info("Embedded React SPA filesystem loaded", "single_binary", true)
	}

	quit := make(chan os.Signal, 2)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	var triggerOnce sync.Once
	triggerRestart := func() {
		triggerOnce.Do(func() {
			time.Sleep(150 * time.Millisecond)
			quit <- syscall.SIGTERM
		})
	}

	// If a prior rollback failed, lock fail-closed: expose maintenance HTTP 503 health/status without
	// starting database migrations, DNS listeners, or VPN reconciliation.
	if failedClosedMode {
		if err := serveFailClosedMode(cfg, staticFS, quit, ""); err != nil {
			slog.Error("HTTP listener error in fail-closed mode", "error", err)
			os.Exit(1)
		}
		return
	}

	// Initialize SQLite Database (WAL Mode) & persistent independent MFA encryption key
	if _, err := initializeDatabaseAndMFA(cfg); err != nil {
		slog.Error("Fatal: Control-plane startup failed", "error", err)
		os.Exit(1)
	}

	router := api.SetupRouter(cfg, staticFS, triggerRestart)

	// 4. Initialize the persistent CA, gateway identity, CRL and data plane.
	if _, err := pki.GetPKIManager(); err != nil {
		slog.Error("PKI initialization failed", "error", err)
		os.Exit(1)
	}
	var sw db.StrongswanSettings
	if err := db.DB.First(&sw, 1).Error; err != nil {
		slog.Error("Missing gateway settings", "error", err)
		os.Exit(1)
	}
	client := vici.GetClient(cfg.VICISocket)
	if !client.IsConnected() {
		slog.Warn("strongSwan VICI unavailable; VPN functions remain unavailable")
	}
	// Run unified gateway reconciliation (server cert + CRL + VICI/nftables) so a CRL or
	// server-cert error can never be overwritten with "applied" by policy.Reconcile alone, while
	// keeping the control plane online in pending/failed state so the retry worker recovers when VICI returns.
	if err := policy.ReconcileGateway(context.Background(), db.DB); err != nil {
		slog.Warn("Data plane initial reconciliation failed; gateway remains pending automatic recovery", "error", err)
	}

	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	go handlers.DefaultGatewayMonitor.Run(workerCtx)
	outboxDone := audit.StartOutboxWorker(workerCtx, func() *gorm.DB { return db.DB })

	// Failed or interrupted applies are retried promptly and survive restarts.
	stopRetry := make(chan struct{})
	var stopOnce sync.Once
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopRetry:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
				needed, err := policy.NeedsReconciliation(ctx, db.DB)
				if err == nil && needed {
					if err := policy.ReconcileGateway(ctx, db.DB); err != nil {
						slog.Error("Retry of pending gateway state failed", "error", err)
					}
				}
				cancel()
			}
		}
	}()

	stopReconciliation := make(chan struct{})
	go func() {
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-stopReconciliation:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := policy.ReconcileGateway(ctx, db.DB); err != nil {
					slog.Error("Periodic policy reconciliation failed", "error", err)
				}
				cancel()
			}
		}
	}()

	// 6. Start Embedded DNS Server if enabled
	var dnsServer *dns.Server
	stopDNSRefresh := make(chan struct{})
	var dnsRefreshDone chan struct{}
	if cfg.EnableDNS {
		dnsAddr := fmt.Sprintf(":%d", cfg.DNSPort)
		dnsServer = dns.NewServer(dnsAddr, db.DB)
		if os.Getenv("SWANPANEL_EXTERNAL_STRONGSWAN") == "1" {
			syncHostDNS := func(ctx context.Context) error {
				ips, err := policy.ManagedGatewayDNSVIPs(ctx)
				if err != nil {
					return err
				}
				addrs := make([]string, 0, len(ips))
				for _, ip := range ips {
					addrs = append(addrs, fmt.Sprintf("%s:%d", ip, cfg.DNSPort))
				}
				return dnsServer.SyncListeners(addrs)
			}
			if err := syncHostDNS(context.Background()); err != nil {
				slog.Error("Host gateway DNS bind failed", "error", err)
				if cfg.Environment == "production" {
					os.Exit(1)
				}
			}
			dnsRefreshDone = make(chan struct{})
			go func() {
				defer close(dnsRefreshDone)
				ticker := time.NewTicker(3 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-stopDNSRefresh:
						return
					case <-ticker.C:
						ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
						if err := syncHostDNS(ctx); err != nil {
							slog.Error("Host gateway DNS listener reconciliation failed", "error", err)
						}
						cancel()
					}
				}
			}()
			slog.Info("Host gateway DNS follows managed loopback VIPs", "port", cfg.DNSPort)
		} else if err := dnsServer.Start(); err != nil {
			slog.Error("Failed to bind embedded DNS server", "addr", dnsAddr, "error", err)
			if cfg.Environment == "production" {
				os.Exit(1)
			}
		} else {
			slog.Info("Embedded DNS server started", "port", cfg.DNSPort)
		}
	}

	stopAllWorkers := func() {
		stopOnce.Do(func() {
			cancelWorkers()
			close(stopRetry)
			close(stopReconciliation)
			close(stopDNSRefresh)
		})
	}
	maintenance.DefaultCoordinator.SetLifecycleHooks(
		func() { realtime.DefaultBroker.CloseAll() },
		stopAllWorkers,
	)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("HTTP server listener active", "address", fmt.Sprintf("http://0.0.0.0:%d", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Fatal: HTTP listener encountered unrecoverable error", "error", err)
			os.Exit(1)
		}
	}()

	sig := <-quit
	slog.Info("Termination signal received; commencing graceful shutdown", "signal", sig.String())

	stopAllWorkers()
	realtime.DefaultBroker.CloseAll()
	if dnsRefreshDone != nil {
		<-dnsRefreshDone
	}
	<-outboxDone

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Forced shutdown due to timeout", "error", err)
		os.Exit(1)
	}

	if dnsServer != nil {
		dnsServer.Stop()
	}
	if db.DB != nil {
		if sqlDB, err := db.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	slog.Info("SwanPanel shutdown completed cleanly")
}
