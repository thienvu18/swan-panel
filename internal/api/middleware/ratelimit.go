package middleware

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	limiters = make(map[string]*ipLimiter)
	mu       sync.Mutex
)

func init() {
	// Clean up stale IP limiters after 10 minutes to prevent unbounded memory growth
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			mu.Lock()
			purged := 0
			for ip, entry := range limiters {
				if time.Since(entry.lastSeen) > 10*time.Minute {
					delete(limiters, ip)
					purged++
				}
			}
			mu.Unlock()
			if purged > 0 {
				slog.Debug("Stale rate limiters cleaned up", "purged_count", purged)
			}
		}
	}()
}

// RateLimit creates a token-bucket rate limiter middleware per client IP
func RateLimit(r rate.Limit, b int) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		mu.Lock()
		entry, exists := limiters[ip]
		if !exists {
			entry = &ipLimiter{
				limiter: rate.NewLimiter(r, b),
			}
			limiters[ip] = entry
			slog.Debug("New rate limiter instance initialized for client IP", "ip", ip)
		}
		entry.lastSeen = time.Now()
		limiter := entry.limiter
		mu.Unlock()

		if !limiter.Allow() {
			slog.Warn("Rate limit threshold exceeded", "ip", ip, "path", c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "Too Many Requests",
				"message": "Too many requests sent. Please try again shortly.",
			})
			return
		}

		c.Next()
	}
}
