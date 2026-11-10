package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/handlers"
	"github.com/swan-panel/swan-panel/internal/api/middleware"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/config"
	"golang.org/x/time/rate"
)

// SetupRouter configures the HTTP router, enterprise security layers, and REST/SSE endpoints
func SetupRouter(cfg *config.Config, staticFS fs.FS) *gin.Engine {
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
		slog.Info("Gin router operating in release mode")
	} else {
		gin.SetMode(gin.DebugMode)
		slog.Debug("Gin router operating in debug mode")
	}

	r := gin.New()
	r.Use(gin.Recovery())

	// Custom structured access logger using slog
	r.Use(func(c *gin.Context) {
		start := c.Request.Context()
		c.Next()
		status := c.Writer.Status()
		path := c.Request.URL.Path
		method := c.Request.Method
		clientIP := c.ClientIP()

		if status >= 500 {
			slog.Error("HTTP request failed with server error", "method", method, "path", path, "status", status, "ip", clientIP)
		} else if status >= 400 {
			slog.Warn("HTTP client request rejected", "method", method, "path", path, "status", status, "ip", clientIP)
		} else {
			slog.Debug("HTTP request processed", "method", method, "path", path, "status", status, "ip", clientIP)
		}
		_ = start
	})

	// Enterprise OWASP Security Headers & CORS Policy
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORSPolicy())

	// Global DDoS Rate Limiter (50 req/sec, burst 100)
	r.Use(middleware.RateLimit(rate.Limit(50), 100))

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

	// 1. Public API
	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			slog.Debug("Health check endpoint invoked", "ip", c.ClientIP())
			c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "SwanPanel"})
		})

		// Login endpoint with strict brute-force rate limiter (5 req/sec, burst 10)
		authGroup := api.Group("/auth")
		authGroup.Use(middleware.RateLimit(rate.Limit(5), 10))
		{
			authGroup.POST("/login", authH.Login)
			authGroup.GET("/oidc/callback", oidcH.OIDCCallback)
		}

		// 2. Protected API (Requires valid Bearer JWT Token or ?token=)
		protected := api.Group("")
		protected.Use(middleware.RequireAuth(cfg.JWTSecret))
		{
			// Real-time Event Stream (Server-Sent Events) - Authenticated
			protected.GET("/realtime/stream", realtime.DefaultBroker.SSEHandler())

			protected.GET("/auth/me", authH.Me)
			protected.GET("/dashboard/stats", dashboardH.GetStats)

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

				// Daemon, Sessions & System Logs
				adminOnly.GET("/system/logs", sysH.GetLogs)
				adminOnly.POST("/sessions/terminate", sessH.TerminateSession)
				adminOnly.POST("/system/daemon", sysH.ControlDaemon)
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
