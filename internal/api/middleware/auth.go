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
	jwt.RegisteredClaims
}

// GenerateToken signs a new JWT token for an authenticated user
func GenerateToken(user *db.User, secret string, duration time.Duration) (string, error) {
	claims := JWTClaims{
		UserID:  user.ID,
		Email:   user.Email,
		IsAdmin: user.IsAdmin,
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
		c.Set("user_id", user.ID)
		c.Set("email", user.Email)
		c.Set("is_admin", user.IsAdmin)
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
