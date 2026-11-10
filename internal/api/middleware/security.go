package middleware

import (
	"log/slog"

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

// CORSPolicy configures explicit and secure CORS policies without wildcards
func CORSPolicy() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, X-Requested-With")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Max-Age", "86400")
			slog.Debug("CORS headers attached to response", "origin", origin)
		}

		if c.Request.Method == "OPTIONS" {
			slog.Debug("CORS preflight request handled", "origin", origin)
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
