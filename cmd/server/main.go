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
	"github.com/swan-panel/swan-panel/internal/vici"
)

func main() {
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
		slog.Warn("Failed to load embedded static filesystem; running in API-only mode", "error", err)
	} else {
		slog.Info("Embedded React SPA filesystem loaded", "single_binary", true)
	}

	router := api.SetupRouter(cfg, staticFS)

	// 4. Initialize PKI & synchronize strongSwan Server and CA certificates
	pkiMgr, err := pki.GetPKIManager()
	if err != nil {
		slog.Error("Failed to initialize PKI manager", "error", err)
	} else {
		_ = pkiMgr.EnsureServerCertificates("swan-panel", []string{"172.18.0.2", "127.0.0.1"})
	}

	// 5. Initialize strongSwan VICI client and sync stored configuration into RAM
	viciClient := vici.GetClient(cfg.VICISocket)
	if viciClient.IsConnected() {
		slog.Info("strongSwan daemon connected via VICI on startup", "socket", cfg.VICISocket)

		// 5.1 Pre-load default IP address pool into strongSwan RAM
		_ = viciClient.LoadPool(context.Background(), "swanpanel-pool", "10.0.1.0/24")

		// 5.2 Load stored connection settings
		var swanSettings db.StrongswanSettings
		if err := db.DB.First(&swanSettings, 1).Error; err == nil {
			connCfg := vici.ConnectionConfig{
				Name:           "swanpanel-ikev2",
				ServerIdentity: swanSettings.ServerIdentity,
				ReauthTime:     swanSettings.ReauthTime,
				RekeyTimeIKE:   swanSettings.RekeyTimeIKE,
				RekeyTimeChild: swanSettings.RekeyTimeChild,
				MobikeEnabled:  swanSettings.MobikeEnabled,
				ForceEncapNATT: swanSettings.ForceEncapNATT,
				DPDDelay:       swanSettings.DPDDelay,
				DPDTimeout:     swanSettings.DPDTimeout,
				IKEProposals:   swanSettings.IKEProposals,
				ESPProposals:   swanSettings.ESPProposals,
				Pools:          []string{"swanpanel-pool"},
			}
			if err := viciClient.LoadConnectionConfig(context.Background(), connCfg); err != nil {
				slog.Warn("Failed to pre-load stored strongSwan settings on startup", "error", err)
			} else {
				slog.Info("Stored strongSwan configuration applied dynamically to daemon in RAM on startup")
			}
		}
	} else {
		slog.Info("strongSwan daemon socket not active on startup; VICI client will auto-connect on demand", "socket", cfg.VICISocket)
	}

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
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
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
