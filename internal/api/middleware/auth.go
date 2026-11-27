package middleware

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/maintenance"
	"gorm.io/gorm"
)

// SecureTokenCompare performs a constant-time comparison of two cryptographic strings
// to mitigate timing attack vectors as strictly mandated by AGENTS.md Rule 2.3
func SecureTokenCompare(tokenA, tokenB string) bool {
	return subtle.ConstantTimeCompare([]byte(tokenA), []byte(tokenB)) == 1
}

type JWTClaims struct {
	UserID  string `json:"user_id"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
	Epoch   string `json:"epoch,omitempty"`
	jwt.RegisteredClaims
}

// GenerateToken signs a new JWT token for an authenticated user bound to the active session epoch.
func GenerateToken(user *db.User, secret string, duration time.Duration) (string, error) {
	epoch, _ := db.CurrentSessionEpoch(db.DB)
	claims := JWTClaims{
		UserID:  user.ID,
		Email:   user.Email,
		IsAdmin: user.IsAdmin,
		Epoch:   epoch,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "SwanPanel",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(secret))
	if err != nil {
		slog.Error("Failed to sign JWT token", "user_id", user.ID, "error", err)
		return "", err
	}

	slog.Debug("JWT token generated successfully", "user_id", user.ID, "expires_in", duration.String())
	return signedToken, nil
}

// RequireAuth enforces Bearer JWTs; query-string tokens are intentionally prohibited.
func RequireAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenStr string
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				tokenStr = parts[1]
			} else {
				slog.Warn("Authentication failed: Malformed Authorization header", "ip", c.ClientIP(), "header", parts[0])
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization header format"})
				return
			}
		}

		if tokenStr == "" {
			slog.Warn("Authentication failed: Missing Authorization token", "ip", c.ClientIP(), "path", c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}

		claims := &JWTClaims{}

		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				slog.Warn("JWT validation rejected: unexpected signing method", "alg", token.Header["alg"])
				return nil, errors.New("unexpected signing algorithm")
			}
			return []byte(secret), nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer("SwanPanel"), jwt.WithExpirationRequired())

		if err != nil || !token.Valid {
			slog.Warn("Authentication failed: Token is invalid or expired", "ip", c.ClientIP(), "error", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Session expired or invalid token"})
			return
		}

		if required, err := db.SetupRequired(); err != nil || required {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Complete one-time administrator setup before accessing the API"})
			return
		}
		// Re-evaluate account status and privilege on EVERY request; never authorize by stale JWT claims.
		var user db.User
		if err := db.DB.WithContext(c.Request.Context()).Where("id = ?", claims.UserID).First(&user).Error; err != nil {
			status := http.StatusUnauthorized
			if err != gorm.ErrRecordNotFound {
				status = http.StatusServiceUnavailable
			}
			c.AbortWithStatusJSON(status, gin.H{"error": "Account unavailable"})
			return
		}
		if user.Status != "ACTIVE" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Account suspended or pending approval"})
			return
		}
		if currentEpoch, err := db.CurrentSessionEpoch(db.DB); err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Session verification unavailable"})
			return
		} else if currentEpoch != "" && !SecureTokenCompare(claims.Epoch, currentEpoch) {
			slog.Warn("Session token invalidated by session epoch rotation", "user_id", user.ID)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Session invalidated; sign in again"})
			return
		}
		c.Set("user_id", user.ID)
		c.Set("email", user.Email)
		c.Set("is_admin", user.IsAdmin)
		c.Next()
	}
}

// MaintenanceGate rejects API traffic with HTTP 503 during backup restore maintenance
// or fail-closed recovery states, while allowing /api/health to report runtime status.
func MaintenanceGate(coord *maintenance.Coordinator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if coord == nil || !strings.HasPrefix(c.Request.URL.Path, "/api") || c.Request.URL.Path == "/api/health" {
			c.Next()
			return
		}
		release, err := coord.BeginRequest()
		if err != nil {
			if errors.Is(err, maintenance.ErrFailedClosed) {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
					"error":       "Control plane is locked in fail-closed recovery mode; manual recovery required",
					"code":        "RECOVERY_FAILED_CLOSED",
					"maintenance": true,
				})
				return
			}
			c.Header("Retry-After", "5")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error":               "System restore is in progress; control plane restarting",
				"code":                "MAINTENANCE_MODE",
				"maintenance":         true,
				"retry_after_seconds": 5,
			})
			return
		}
		c.Set("maintenance_release", release)
		defer release()
		c.Next()
	}
}

// RequireAdmin protects endpoints restricted strictly to Administrators
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin, exists := c.Get("is_admin")
		if !exists || !isAdmin.(bool) {
			slog.Warn("Access denied: Non-admin attempted to access admin endpoint",
				"user_id", c.GetString("user_id"),
				"ip", c.ClientIP(),
				"path", c.Request.URL.Path,
			)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Access denied: Administrator privileges required"})
			return
		}

		slog.Debug("Admin authorization verified", "user_id", c.GetString("user_id"), "path", c.Request.URL.Path)
		c.Next()
	}
}
