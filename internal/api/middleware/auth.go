package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/swan-panel/swan-panel/internal/db"
)

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

// RequireAuth enforces a valid Bearer JWT token in the Authorization header or query parameter
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
		} else {
			// Fallback to query parameter (required for browser native EventSource / SSE)
			tokenStr = c.Query("token")
		}

		if tokenStr == "" {
			slog.Warn("Authentication failed: Missing Authorization token", "ip", c.ClientIP(), "path", c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}

		claims := &JWTClaims{}

		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				slog.Warn("JWT validation rejected: unexpected signing method", "alg", token.Header["alg"])
				return nil, errors.New("unexpected signing algorithm")
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			slog.Warn("Authentication failed: Token is invalid or expired", "ip", c.ClientIP(), "error", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Session expired or invalid token"})
			return
		}

		slog.Debug("User authenticated via JWT", "user_id", claims.UserID, "email", claims.Email, "is_admin", claims.IsAdmin)
		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("is_admin", claims.IsAdmin)
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
