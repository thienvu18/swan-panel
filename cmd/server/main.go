package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	swanpanel "github.com/swan-panel/swan-panel"
	"github.com/swan-panel/swan-panel/internal/api"
	"github.com/swan-panel/swan-panel/internal/config"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/dns"
	"github.com/swan-panel/swan-panel/internal/logger"
	"github.com/swan-panel/swan-panel/internal/pki"
	"github.com/swan-panel/swan-panel/internal/policy"
	"github.com/swan-panel/swan-panel/internal/vici"
)

// version is set using -ldflags "-X main.version=vX.Y.Z" in release builds.
var version = "dev"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println("SwanPanel", version)
		return
	}
	// 1. Load configuration and initialize structured logger
	cfg := config.Load()
	logger.InitLogger(cfg.LogLevel, cfg.Environment)

	slog.Info("Starting SwanPanel gateway",
		"environment", cfg.Environment,
		"log_level", cfg.LogLevel,
		"port", cfg.Port,
		"db_path", cfg.DBPath,
	)

	// 2. Initialize SQLite Database (WAL Mode)
	_, err := db.InitDB(cfg.DBPath)
	if err != nil {
		slog.Error("Fatal: Failed to initialize SQLite database", "path", cfg.DBPath, "error", err)
		os.Exit(1)
	}
	slog.Info("SQLite database connected successfully", "wal_mode", true, "tables", 8)

	// 3. Initialize Router and embedded SPA filesystem
	staticFS, err := swanpanel.StaticFS()
	if err != nil {
		if cfg.Environment == "production" {
			slog.Error("Production frontend bundle unavailable; refusing to start", "error", err)
			os.Exit(1)
		}
		// In local development Vite serves the UI independently from port 3000.
		slog.Info("Embedded frontend not available; use the Vite development UI", "error", err)
	} else {
		slog.Info("Embedded React SPA filesystem loaded", "single_binary", true)
	}

	router := api.SetupRouter(cfg, staticFS)

	// 4. Initialize the persistent CA, gateway identity, CRL and data plane.
	// A missing data plane is reported as degraded. It must never be simulated.
	manager, err := pki.GetPKIManager()
	if err != nil {
		slog.Error("PKI initialization failed", "error", err)
		os.Exit(1)
	}
	var sw db.StrongswanSettings
	if err := db.DB.First(&sw, 1).Error; err != nil {
		slog.Error("Missing gateway settings", "error", err)
		os.Exit(1)
	}
	if err := manager.EnsureServerCertificates(sw.ServerIdentity, []string{sw.ServerEndpoint}); err != nil {
		slog.Error("Gateway certificate deployment failed", "error", err)
		if cfg.Environment == "production" {
			os.Exit(1)
		}
	}
	syncRevocations := func(ctx context.Context) error {
		var rows []db.CertificateRevocation
		if err := db.DB.WithContext(ctx).Find(&rows).Error; err != nil {
			return err
		}
		certs := make([]pki.RevokedCertificate, 0, len(rows))
		for _, r := range rows {
			certs = append(certs, pki.RevokedCertificate{Serial: r.Serial, RevokedAt: r.RevokedAt})
		}
		return manager.PublishCRL(ctx, certs)
	}
	if err := syncRevocations(context.Background()); err != nil {
		slog.Error("Cannot synchronize signed CRL", "error", err)
		if cfg.Environment == "production" {
			os.Exit(1)
		}
	}
	client := vici.GetClient(cfg.VICISocket)
	if !client.IsConnected() {
		slog.Warn("strongSwan VICI unavailable; VPN functions remain unavailable")
	}
	if err := policy.Reconcile(context.Background(), db.DB); err != nil {
		slog.Warn("Data plane initial reconciliation failed; no applied state is claimed", "error", err)
		if cfg.Environment == "production" {
			os.Exit(1)
		}
	}
	// Refresh the short-lived CRL and reapply the desired runtime policy after daemon restarts.
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
				if err := syncRevocations(ctx); err != nil {
					slog.Error("CRL refresh failed", "error", err)
				}
				if err := policy.Reconcile(ctx, db.DB); err != nil {
					slog.Error("Periodic policy reconciliation failed", "error", err)
				}
				cancel()
			}
		}
	}()

	// 6. Start Embedded DNS Server if enabled
	var dnsServer *dns.Server
	if cfg.EnableDNS {
		dnsAddr := fmt.Sprintf(":%d", cfg.DNSPort)
		dnsServer = dns.NewServer(dnsAddr, db.DB)
		if err := dnsServer.Start(); err != nil {
			slog.Warn("Failed to initialize embedded DNS server", "addr", dnsAddr, "error", err)
		} else {
			slog.Info("Embedded DNS server started", "port", cfg.DNSPort)
		}
	}

	srv := &http.Server{
		Addr:        fmt.Sprintf(":%d", cfg.Port),
		Handler:     router,
		ReadTimeout: 15 * time.Second,
		// SSE responses are intentionally long-lived. A global WriteTimeout would
		// terminate every authenticated event stream after 15 seconds.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	// 4. Start HTTP Server in background goroutine
	go func() {
		slog.Info("HTTP server listener active", "address", fmt.Sprintf("http://0.0.0.0:%d", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Fatal: HTTP listener encountered unrecoverable error", "error", err)
			os.Exit(1)
		}
	}()

	// 5. Listen for termination signals (Graceful Shutdown)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	slog.Info("Termination signal received; commencing graceful shutdown", "signal", sig.String())

	close(stopReconciliation)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Forced shutdown due to timeout", "error", err)
		os.Exit(1)
	}

	if dnsServer != nil {
		dnsServer.Stop()
	}

	slog.Info("SwanPanel shutdown completed cleanly")
}
