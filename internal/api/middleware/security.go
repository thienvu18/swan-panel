package middleware

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders applies enterprise-grade HTTP security headers according to OWASP standards
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME-sniffing XSS
		c.Header("X-Content-Type-Options", "nosniff")

		// Prevent Clickjacking
		c.Header("X-Frame-Options", "DENY")

		// Legacy browser XSS protection
		c.Header("X-XSS-Protection", "1; mode=block")

		// HSTS: Enforce HTTPS in production deployments
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")

		// Restrict referrer leakage
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Strict Content Security Policy (CSP)
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com data:; img-src 'self' data: https:; connect-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'self';")

		c.Next()
	}
}

// CORSPolicy configures explicit and secure CORS policies with strict origin whitelist validation
func CORSPolicy(allowedOrigins []string, isDev bool) gin.HandlerFunc {
	// Normalized map for O(1) whitelist lookup
	allowedMap := make(map[string]bool)
	for _, o := range allowedOrigins {
		allowedMap[strings.ToLower(strings.TrimRight(o, "/"))] = true
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			c.Next()
			return
		}

		normalizedOrigin := strings.ToLower(strings.TrimRight(origin, "/"))
		allowed := false

		if allowedMap[normalizedOrigin] {
			allowed = true
		} else if isDev {
			// In development mode, allow standard localhost and loopback ports
			if strings.HasPrefix(normalizedOrigin, "http://localhost:") ||
				strings.HasPrefix(normalizedOrigin, "http://127.0.0.1:") ||
				strings.HasPrefix(normalizedOrigin, "https://localhost:") ||
				normalizedOrigin == "http://localhost" ||
				normalizedOrigin == "http://127.0.0.1" {
				allowed = true
			}
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, X-Requested-With")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Max-Age", "86400")
			slog.Debug("CORS origin approved", "origin", origin)

			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(204)
				return
			}
		} else {
			slog.Warn("CORS request rejected: origin not in whitelist", "origin", origin, "path", c.Request.URL.Path, "ip", c.ClientIP())
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatusJSON(403, gin.H{"error": "Forbidden: CORS origin not allowed"})
				return
			}
		}

		c.Next()
	}
}
