package api

import (
	"crypto/subtle"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/handlers"
	"github.com/swan-panel/swan-panel/internal/api/middleware"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/backup"
	"github.com/swan-panel/swan-panel/internal/config"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"github.com/swan-panel/swan-panel/internal/mfa"
	"golang.org/x/time/rate"
)

// SetupRouter configures the HTTP router, enterprise security layers, and REST/SSE endpoints
func SetupRouter(cfg *config.Config, staticFS fs.FS, onRequestRestart ...func()) *gin.Engine {
	if db.DB != nil && cfg.DBPath != "" && cfg.Environment == "test" {
		if mfa.ActiveKeyPath() != mfa.ResolveKeyFilePath(cfg.DBPath) {
			_, _, _ = mfa.InitActiveKey(db.DB, cfg.DBPath, cfg.JWTSecret)
		}
	}
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
		slog.Info("Gin router operating in release mode")
	} else {
		gin.SetMode(gin.DebugMode)
		slog.Debug("Gin router operating in debug mode")
	}

	r := gin.New()
	r.Use(gin.Recovery())

	// Custom structured access logger using slog with accurate request duration
	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()
		path := c.Request.URL.Path
		method := c.Request.Method
		clientIP := c.ClientIP()

		if status >= 500 {
			slog.Error("HTTP request failed with server error", "method", method, "path", path, "status", status, "latency", latency.String(), "ip", clientIP)
		} else if status >= 400 {
			slog.Warn("HTTP client request rejected", "method", method, "path", path, "status", status, "latency", latency.String(), "ip", clientIP)
		} else {
			slog.Debug("HTTP request processed", "method", method, "path", path, "status", status, "latency", latency.String(), "ip", clientIP)
		}
	})

	// Enterprise OWASP Security Headers & CORS Whitelist Policy
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORSPolicy(cfg.AllowedOrigins, cfg.Environment != "production"))

	// Global DDoS Rate Limiter (50 req/sec, burst 100)
	r.Use(middleware.RateLimit(rate.Limit(50), 100))
	r.Use(middleware.MaintenanceGate(maintenance.DefaultCoordinator))

	authH := handlers.NewAuthHandler(cfg)
	settingsH := handlers.NewSettingsHandler()
	dashboardH := handlers.NewDashboardHandler()
	oidcH := handlers.NewOIDCHandler(cfg)
	usersH := handlers.NewUsersHandler()
	resH := handlers.NewResourcesHandler()
	accessH := handlers.NewAccessHandler()
	dnsH := handlers.NewDNSHandler()
	sessH := handlers.NewSessionsHandler()
	sysH := handlers.NewSystemHandler()
	auditH := handlers.NewAuditHandler()
	backupH := handlers.NewBackupHandler(cfg, onRequestRestart...)

	// 1. Public API
	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			slog.Debug("Health check endpoint invoked", "ip", c.ClientIP())
			attachCorrelatedRestoreStatus := func(payload gin.H) gin.H {
				reqID := strings.TrimSpace(c.Query("restore_id"))
				if reqID == "" || cfg.DBPath == "" {
					return payload
				}
				st, err := backup.ReadRestoreStatus(cfg.DBPath)
				if err != nil || st == nil || st.RestoreID == "" {
					return payload
				}
				if subtle.ConstantTimeCompare([]byte(reqID), []byte(st.RestoreID)) == 1 {
					payload["restore_status"] = st.Outcome
					if st.Message != "" {
						payload["restore_error"] = st.Message
					}
					if st.CompletedAt != "" {
						payload["restore_completed_at"] = st.CompletedAt
					}
				}
				return payload
			}
			st := maintenance.DefaultCoordinator.CurrentStatus()
			if st.Mode != maintenance.ModeNormal {
				if st.RetryAfterSeconds > 0 {
					c.Header("Retry-After", "5")
				}
				c.JSON(http.StatusServiceUnavailable, attachCorrelatedRestoreStatus(gin.H{
					"status":              "maintenance",
					"mode":                st.Mode,
					"maintenance":         true,
					"retry_after_seconds": st.RetryAfterSeconds,
					"service":             "SwanPanel",
				}))
				return
			}
			c.JSON(http.StatusOK, attachCorrelatedRestoreStatus(gin.H{"status": "healthy", "maintenance": false, "service": "SwanPanel"}))
		})

		// Login endpoint with strict brute-force rate limiter (5 req/sec, burst 10)
		authGroup := api.Group("/auth")
		authGroup.Use(middleware.RateLimit(rate.Limit(5), 10))
		{
			authGroup.POST("/login", authH.Login)
			authGroup.POST("/mfa/verify", authH.VerifyMFALogin)
			authGroup.GET("/setup/status", handlers.SetupStatus)
			authGroup.POST("/setup", handlers.SetupAdmin)
			authGroup.GET("/oidc/providers", oidcH.PublicProviders)
			authGroup.GET("/oidc/:id/start", oidcH.StartLogin)
			authGroup.POST("/oidc/exchange", oidcH.ExchangeTicket)
			authGroup.GET("/oidc/callback", oidcH.OIDCCallback)
		}

		// 2. Protected API (requires Bearer JWT; credentials in query strings are prohibited)
		protected := api.Group("")
		protected.Use(middleware.RequireAuth(cfg.JWTSecret))
		{
			// Real-time Event Stream (Server-Sent Events) - Authenticated
			protected.GET("/realtime/stream", realtime.DefaultBroker.SSEHandler())

			protected.GET("/auth/me", authH.Me)
			protected.PUT("/auth/password", authH.ChangePassword)
			protected.GET("/auth/mfa/status", authH.GetMFAStatus)
			protected.POST("/auth/mfa/setup", authH.SetupMFA)
			protected.POST("/auth/mfa/enable", authH.EnableMFA)
			protected.POST("/auth/mfa/disable", authH.DisableMFA)
			protected.POST("/auth/mfa/backup-codes", authH.RegenerateBackupCodes)
			protected.GET("/dashboard/stats", dashboardH.GetStats)
			protected.GET("/gateway/health", dashboardH.GetGatewayHealth)

			// Device management & configuration profiles
			protected.GET("/devices", resH.ListDevices)
			protected.POST("/devices", resH.CreateDevice)
			protected.GET("/devices/:id/config/:platform", resH.DownloadDeviceConfig)
			protected.POST("/devices/:id/revoke", resH.RevokeDevice)

			// Runtime metrics
			protected.GET("/sessions", sessH.ListSessions)
			protected.GET("/system/daemon/status", sysH.GetDaemonStatus)

			// 3. Admin-only API
			adminOnly := protected.Group("")
			adminOnly.Use(middleware.RequireAdmin())
			{
				// OIDC Identity Providers
				adminOnly.GET("/oidc/providers", oidcH.ListProviders)
				adminOnly.POST("/oidc/providers", oidcH.CreateProvider)
				adminOnly.PUT("/oidc/providers/:id", oidcH.UpdateProvider)
				adminOnly.DELETE("/oidc/providers/:id", oidcH.DeleteProvider)
				adminOnly.POST("/oidc/providers/:id/test", oidcH.TestConnection)

				// Users Directory & Approval Queue
				adminOnly.GET("/users", usersH.ListUsers)
				adminOnly.POST("/users", usersH.CreateLocalUser)
				adminOnly.PUT("/users/:id/role", usersH.ToggleAdminRole)
				adminOnly.POST("/users/:id/approve", usersH.ApproveUser)
				adminOnly.POST("/users/:id/reject", usersH.RejectUser)
				adminOnly.POST("/users/:id/mfa/reset", usersH.ResetUserMFA)
				adminOnly.DELETE("/users/:id", usersH.DeleteUser)
				adminOnly.GET("/user-groups", usersH.ListUserGroups)
				adminOnly.POST("/user-groups", usersH.CreateUserGroup)
				adminOnly.DELETE("/user-groups/:id", usersH.DeleteUserGroup)

				// Resources & Subnets
				adminOnly.DELETE("/devices/:id", resH.DeleteDevice)
				adminOnly.GET("/resource-groups", resH.ListResourceGroups)
				adminOnly.POST("/resource-groups", resH.CreateResourceGroup)
				adminOnly.DELETE("/resource-groups/:id", resH.DeleteResourceGroup)

				// Access Rules (Zero-Trust Policies)
				adminOnly.GET("/access-rules", accessH.ListAccessRules)
				adminOnly.GET("/access-rules/effective/:id", accessH.EffectiveDeviceAccess)
				adminOnly.POST("/access-rules", accessH.CreateAccessRule)
				adminOnly.DELETE("/access-rules/:id", accessH.DeleteAccessRule)

				// DNS Engine
				adminOnly.GET("/settings/dns", settingsH.GetDNSSettings)
				adminOnly.PUT("/settings/dns", dnsH.UpdateSettings)
				adminOnly.GET("/dns/overrides", dnsH.ListOverrides)
				adminOnly.POST("/dns/overrides", dnsH.CreateOverride)
				adminOnly.DELETE("/dns/overrides/:id", dnsH.DeleteOverride)

				// strongSwan Engine Settings
				adminOnly.GET("/settings/strongswan", settingsH.GetStrongswanSettings)
				adminOnly.PUT("/settings/strongswan", settingsH.UpdateStrongswanSettings)
				adminOnly.GET("/settings/ca", settingsH.GetCAIdentity)
				adminOnly.POST("/settings/ca/rotate", settingsH.RotateCAIdentity)

				// Daemon, Sessions & System Logs
				adminOnly.GET("/system/logs", sysH.GetLogs)
				adminOnly.POST("/sessions/terminate", sessH.TerminateSession)
				adminOnly.POST("/system/daemon", sysH.ControlDaemon)
				adminOnly.GET("/system/apply-status", sysH.GetApplyStatus)
				adminOnly.POST("/system/apply", sysH.RetryApply)
				adminOnly.POST("/system/backup", backupH.CreateBackup)
				adminOnly.POST("/system/restore", backupH.RestoreBackup)

				// Immutable Audit Trail & Alert Webhooks
				adminOnly.GET("/audit-logs", auditH.ListAuditLogs)
				adminOnly.GET("/audit-logs/verify", auditH.VerifyAuditChain)
				adminOnly.GET("/audit-logs/export", auditH.ExportAuditLogs)
				adminOnly.GET("/webhooks", auditH.ListWebhooks)
				adminOnly.GET("/webhooks/deliveries", auditH.ListWebhookDeliveries)
				adminOnly.POST("/webhooks", auditH.CreateWebhook)
				adminOnly.PUT("/webhooks/:id", auditH.UpdateWebhook)
				adminOnly.DELETE("/webhooks/:id", auditH.DeleteWebhook)
				adminOnly.POST("/webhooks/:id/test", auditH.TestWebhook)
			}
		}
	}

	// 4. Embedded Frontend SPA Static Serving (Single-Binary Delivery)
	if staticFS != nil {
		httpFS := http.FS(staticFS)
		fileServer := http.FileServer(httpFS)

		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			// Return JSON 404 for unmatched API requests
			if strings.HasPrefix(path, "/api") {
				slog.Warn("Unmatched API route requested", "path", path, "ip", c.ClientIP())
				c.JSON(http.StatusNotFound, gin.H{"error": "API route not found"})
				return
			}

			// Attempt serving exact static file
			f, err := staticFS.Open(strings.TrimPrefix(path, "/"))
			if err == nil {
				_ = f.Close()
				slog.Debug("Serving static asset from embedded filesystem", "path", path)
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}

			// SPA client-side routing fallback to index.html
			indexFile, err := staticFS.Open("index.html")
			if err == nil {
				_ = indexFile.Close()
				slog.Debug("Falling back to SPA index.html for client route", "requested_path", path)
				c.Request.URL.Path = "/"
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}

			slog.Error("SPA fallback index.html not found in embedded filesystem", "path", path)
			c.String(http.StatusNotFound, "Frontend application asset not found")
		})
	} else {
		slog.Warn("Static embedded filesystem not provided; SPA routing disabled")
	}

	return r
}
