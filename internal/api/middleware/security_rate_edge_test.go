package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

func makeLimiterRouter(global, authentication gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(global)
	r.GET("/public", func(c *gin.Context) { c.Status(http.StatusOK) })
	auth := r.Group("/auth")
	auth.Use(authentication)
	auth.GET("/login", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}
func checkRequestStatus(t *testing.T, router *gin.Engine, path, ip string, expected int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = ip + ":1024"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != expected {
		t.Fatalf("%s from %s expected %d, got %d %s", path, ip, expected, w.Code, w.Body.String())
	}
}
func TestRateLimitSeparatesGlobalAndAuthenticationPoliciesPerMiddlewareAndClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	global := RateLimit(rate.Every(24*time.Hour), 10)
	auth := RateLimit(rate.Every(24*time.Hour), 2)
	router := makeLimiterRouter(global, auth)
	ip := "198.51.100.4"
	checkRequestStatus(t, router, "/auth/login", ip, http.StatusOK)
	checkRequestStatus(t, router, "/auth/login", ip, http.StatusOK)
	checkRequestStatus(t, router, "/auth/login", ip, http.StatusTooManyRequests)
	checkRequestStatus(t, router, "/public", ip, http.StatusOK)
	checkRequestStatus(t, router, "/public", ip, http.StatusOK)
	for i := 0; i < 2; i++ {
		checkRequestStatus(t, router, "/auth/login", "198.51.100.5", http.StatusOK)
	}
	checkRequestStatus(t, router, "/auth/login", "198.51.100.5", http.StatusTooManyRequests)

	// New router/middleware instances must not reuse tokens spent in old routers.
	next := makeLimiterRouter(RateLimit(rate.Every(24*time.Hour), 3), RateLimit(rate.Every(24*time.Hour), 1))
	checkRequestStatus(t, next, "/auth/login", ip, http.StatusOK)
	checkRequestStatus(t, next, "/auth/login", ip, http.StatusTooManyRequests)
	checkRequestStatus(t, next, "/public", ip, http.StatusOK)
}

func TestSecurityHeadersRejectFrameEmbeddingAndControlCaching(t *testing.T) {
	router := gin.New()
	router.Use(SecurityHeaders())
	router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "OK") })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	for header, expected := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	} {
		if w.Header().Get(header) != expected {
			t.Fatalf("security response header %s not enforced: %q", header, w.Header().Get(header))
		}
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("CSP missing")
	}
}

func TestCORSPolicyAllowsOnlyConfiguredOriginsAndDevLoopback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		origin   string
		dev      bool
		expected int
		allow    bool
	}{
		{"explicit origin", "https://example.org", false, 204, true},
		{"normalized origin", "HTTPS://EXAMPLE.ORG/", false, 204, true},
		{"untrusted origin", "https://attacker.org", false, 403, false},
		{"production localhost", "http://localhost:3000", false, 403, false},
		{"development localhost", "http://localhost:3000", true, 204, true},
		{"development loopback", "http://127.0.0.1:9012", true, 204, true},
		{"no origin", "", false, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(CORSPolicy([]string{"https://example.org"}, tc.dev))
			router.OPTIONS("/test", func(c *gin.Context) { c.Status(200) })
			req := httptest.NewRequest(http.MethodOptions, "/test", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.expected {
				t.Fatalf("%q: got %d want %d", tc.origin, w.Code, tc.expected)
			}
			allowed := w.Header().Get("Access-Control-Allow-Origin") != ""
			if allowed != tc.allow {
				t.Fatalf("CORS origin %q allowed=%v want %v", tc.origin, allowed, tc.allow)
			}
		})
	}
}
