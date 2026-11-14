package handlers

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/swan-panel/swan-panel/internal/config"
	"github.com/swan-panel/swan-panel/internal/db"
)

func TestOIDCConfigurationValidation(t *testing.T) {
	cases := []struct {
		label, kind, issuer, auth, token, jwks, userInfo, mapping string
		valid                                                     bool
	}{
		{"Google Discovery", "google", "https://accounts.google.com", "", "", "", "", "", true},
		{"Entra tenant Discovery", "microsoft", "https://login.microsoftonline.com/tenant-123/v2.0", "", "", "", "", "", true},
		{"Keycloak Discovery", "keycloak", "https://sso.example.org/realms/acme", "", "", "", "", "", true},
		{"custom discovery", "custom", "https://id.example.org", "", "", "", "", "", true},
		{"custom explicit endpoints", "custom", "https://id.example.org", "https://id.example.org/authorize", "https://id.example.org/token", "https://id.example.org/jwks", "https://id.example.org/userinfo", `{"email":"profile.email","name":"profile.display"}`, true},
		{"http remote blocked", "custom", "http://internal.example.org", "", "", "", "", "", false},
		{"partial endpoints rejected", "custom", "https://id.example.org", "https://id.example.org/authorize", "", "", "", "", false},
		{"partial optional userInfo rejected", "custom", "https://id.example.org", "", "", "", "https://id.example.org/userinfo", "", false},
		{"override built-in provider rejected", "google", "https://accounts.google.com", "https://other.example.org/auth", "https://other.example.org/token", "https://other.example.org/jwks", "", "", false},
		{"invalid mapping field", "custom", "https://id.example.org", "", "", "", "", `{"is_admin":"roles.admin"}`, false},
		{"mapped sub empty", "custom", "https://id.example.org", "", "", "", "", `{"sub":""}`, false},
		{"malformed mapping", "custom", "https://id.example.org", "", "", "", "", `{"name":}`, false},
		{"invalid URL userInfo", "custom", "https://id.example.org", "https://id.example.org/auth", "https://id.example.org/token", "https://id.example.org/jwks", "http://169.254.169.254/", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			err := validateOIDCConfiguration(tc.kind, tc.issuer, tc.auth, tc.token, tc.jwks, tc.userInfo, tc.mapping)
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid && err == nil {
				t.Fatal("accepted insecure OIDC configuration")
			}
		})
	}
	data := map[string]any{"nested": map[string]any{"mail": "a@example.com"}}
	if mapped := oidcMappedClaim(data, "nested.mail"); mapped != "a@example.com" {
		t.Fatalf("nested mapping failed: %v", mapped)
	}
	if missing := oidcMappedClaim(data, "nested.missing"); missing != nil {
		t.Fatal("missing claim mapped to a value")
	}
}

type fakeOIDCProvider struct {
	server        *httptest.Server
	key           *rsa.PrivateKey
	nonce         string
	audience      string
	signatureMode string
	subject       string
}

func newFakeOIDCProvider(t *testing.T) *fakeOIDCProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeOIDCProvider{key: key, audience: "client-test", subject: "subject-123"}
	var srv *httptest.Server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                                srv.URL,
				"authorization_endpoint":                srv.URL + "/auth",
				"token_endpoint":                        srv.URL + "/token",
				"jwks_uri":                              srv.URL + "/jwks",
				"userinfo_endpoint":                     srv.URL + "/userinfo",
				"response_types_supported":              []string{"code"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
				"kid": "test-key", "kty": "RSA", "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
			}}})
		case "/token":
			if r.Method != "POST" {
				http.Error(w, "method", http.StatusMethodNotAllowed)
				return
			}
			if err := r.ParseForm(); err != nil {
				http.Error(w, "form", 400)
				return
			}
			if r.Form.Get("code") != "authorization-code" || len(r.Form.Get("code_verifier")) < 40 {
				http.Error(w, "missing authorization code or PKCE verifier", 400)
				return
			}
			now := time.Now().Unix()
			nonce := fake.nonce
			if fake.signatureMode == "bad-nonce" {
				nonce = "attacker-nonce"
			}
			claims := jwt.MapClaims{
				"iss": srv.URL, "aud": fake.audience, "exp": now + 600, "iat": now,
				"sub": fake.subject, "nonce": nonce,
				"email": "test@example.com", "name": "OIDC Test",
				"email_verified": true, "preferred_username": "test@example.com",
				"profile": map[string]any{"mail": "test@example.com", "display": "OIDC Custom"},
			}
			if fake.signatureMode == "bad-issuer" {
				claims["iss"] = "https://evil.example.com"
			}
			if fake.signatureMode == "expired" {
				claims["exp"] = now - 3600
			}
			if fake.signatureMode == "unverified" {
				claims["email_verified"] = false
			}
			idToken := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			idToken.Header["kid"] = "test-key"
			signingKey := key
			if fake.signatureMode == "bad-signature" {
				rogue, err := rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					http.Error(w, "keygen", 500)
					return
				}
				signingKey = rogue
			}
			signed, err := idToken.SignedString(signingKey)
			if err != nil {
				http.Error(w, "sign", 500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id_token": signed, "access_token": "access-token", "token_type": "Bearer", "expires_in": 3600})
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": fake.subject, "email": "test@example.com", "name": "OIDC Test"})
		default:
			http.NotFound(w, r)
		}
	})
	srv = httptest.NewTLSServer(handler)
	fake.server = srv
	t.Cleanup(srv.Close)
	return fake
}

func (f *fakeOIDCProvider) testRouter(t *testing.T, providerType, mapping string) *gin.Engine {
	t.Helper()
	t.Setenv("APP_ENV", "test")
	t.Setenv("PUBLIC_URL", f.server.URL)
	path := filepath.Join(t.TempDir(), "oidc.sqlite")
	database, err := db.InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sql, err := database.DB()
		if err != nil {
			t.Errorf("get OIDC test database connection: %v", err)
			return
		}
		if err := sql.Close(); err != nil {
			t.Errorf("close OIDC test database: %v", err)
		}
	})
	// An OIDC provider may only be configured after initial admin setup.
	admin := db.User{ID: "admin-test", Email: "admin@example.org", Name: "Admin", AuthType: "local", IsAdmin: true, Status: "ACTIVE"}
	if err := db.DB.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	provider := db.OIDCProvider{
		ID: "idp-test", Name: "Test IDP", ProviderType: providerType,
		IssuerURL: f.server.URL, ClientID: "client-test", ClientSecret: "provider-secret",
		ClaimMapping: mapping, IsActive: true,
	}
	if providerType == "custom" {
		provider.AuthorizationURL = f.server.URL + "/auth"
		provider.TokenURL = f.server.URL + "/token"
		provider.JWKSURL = f.server.URL + "/jwks"
		provider.UserInfoURL = f.server.URL + "/userinfo"
	}
	if err := db.DB.Create(&provider).Error; err != nil {
		t.Fatal(err)
	}
	oldFactory := oidcHTTPClient
	oidcHTTPClient = func() *http.Client { return f.server.Client() }
	t.Cleanup(func() { oidcHTTPClient = oldFactory })
	cfg := &config.Config{Environment: "test", JWTSecret: "oidc-test-jwt-secret-1234567890123456"}
	h := NewOIDCHandler(cfg)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/auth/oidc/:id/start", h.StartLogin)
	router.GET("/api/auth/oidc/callback", h.OIDCCallback)
	router.POST("/api/auth/oidc/exchange", h.ExchangeTicket)
	return router
}

func (f *fakeOIDCProvider) authorize(t *testing.T, router *gin.Engine) (*httptest.ResponseRecorder, *http.Cookie, string) {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/auth/oidc/idp-test/start", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != 302 {
		t.Fatalf("OIDC authorization did not begin: %d %s", res.Code, res.Body.String())
	}
	location, err := url.Parse(res.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	values := location.Query()
	if values.Get("response_type") != "code" || values.Get("code_challenge_method") != "S256" || values.Get("state") == "" || values.Get("nonce") == "" {
		t.Fatalf("required OIDC PKCE/state/nonce params absent: %v", values)
	}
	f.nonce = values.Get("nonce")
	var stateCookie *http.Cookie
	for _, cookie := range res.Result().Cookies() {
		if cookie.Name == "swan_oidc_state" {
			stateCookie = cookie
		}
	}
	if stateCookie == nil || !stateCookie.HttpOnly {
		t.Fatal("state cookie missing or not HttpOnly")
	}
	return res, stateCookie, values.Get("state")
}

func (f *fakeOIDCProvider) callback(router *gin.Engine, cookie *http.Cookie, state string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/api/auth/oidc/callback?state="+url.QueryEscape(state)+"&code=authorization-code", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	return res
}

func TestOIDCAuthorizationCodeLifecycle(t *testing.T) {
	for _, tc := range []struct{ kind, mapping string }{
		{"google", ""}, {"microsoft", ""}, {"keycloak", ""},
		{"custom", `{"email":"profile.mail","name":"profile.display"}`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			fake := newFakeOIDCProvider(t)
			router := fake.testRouter(t, tc.kind, tc.mapping)
			_, cookie, state := fake.authorize(t, router)
			missingCookie := fake.callback(router, nil, state)
			if missingCookie.Code != 400 {
				t.Fatalf("state without cookie accepted: %d", missingCookie.Code)
			}
			badState := fake.callback(router, cookie, "bad-state")
			if badState.Code != 400 {
				t.Fatalf("mismatched state accepted: %d", badState.Code)
			}
			pending := fake.callback(router, cookie, state)
			if pending.Code != 302 || !strings.Contains(pending.Header().Get("Location"), "oidc-pending") {
				t.Fatalf("unapproved identity admitted: %d %s", pending.Code, pending.Body.String())
			}
			var user db.User
			if err := db.DB.First(&user, "idp_provider_id = ? AND idp_subject = ?", "idp-test", fake.subject).Error; err != nil {
				t.Fatal(err)
			}
			if user.IsAdmin || user.Status != "PENDING" {
				t.Fatalf("new SSO user has unsafe role/status: %+v", user)
			}
			if tc.kind == "custom" && user.Name != "OIDC Custom" {
				t.Fatal("custom claim mapping was not applied")
			}
			replay := fake.callback(router, cookie, state)
			if replay.Code != 400 {
				t.Fatal("OIDC callback state replay accepted")
			}
			if err := db.DB.Model(&user).Update("status", "ACTIVE").Error; err != nil {
				t.Fatal(err)
			}
			_, cookie, state = fake.authorize(t, router)
			success := fake.callback(router, cookie, state)
			if success.Code != 302 || !strings.Contains(success.Header().Get("Location"), "oidc-complete") {
				t.Fatalf("approved identity failed SSO: %d %s", success.Code, success.Body.String())
			}
			var ticket *http.Cookie
			for _, c := range success.Result().Cookies() {
				if c.Name == "swan_oidc_ticket" {
					ticket = c
				}
			}
			if ticket == nil || !ticket.HttpOnly {
				t.Fatal("missing secure SSO exchange ticket")
			}
			exchange := func() *httptest.ResponseRecorder {
				req := httptest.NewRequest("POST", "/api/auth/oidc/exchange", nil)
				req.AddCookie(ticket)
				res := httptest.NewRecorder()
				router.ServeHTTP(res, req)
				return res
			}
			token := exchange()
			if token.Code != 200 || !strings.Contains(token.Body.String(), `"token"`) {
				t.Fatalf("active SSO session cannot exchange: %s", token.Body.String())
			}
			if again := exchange(); again.Code != 401 {
				t.Fatalf("SSO ticket replay accepted: %d", again.Code)
			}
		})
	}
}

func TestOIDCRejectsInvalidSignedTokens(t *testing.T) {
	for _, mode := range []string{"bad-nonce", "bad-issuer", "expired", "unverified", "bad-signature"} {
		t.Run(mode, func(t *testing.T) {
			fake := newFakeOIDCProvider(t)
			fake.signatureMode = mode
			router := fake.testRouter(t, "custom", "")
			_, cookie, state := fake.authorize(t, router)
			response := fake.callback(router, cookie, state)
			if response.Code == 200 || response.Code == 302 {
				t.Fatalf("%s token was accepted", mode)
			}
			var users int64
			if err := db.DB.Model(&db.User{}).Where("auth_type = ?", "oidc").Count(&users).Error; err != nil {
				t.Fatal(err)
			}
			if users != 0 {
				t.Fatal("invalid signed token enrolled a user")
			}
		})
	}
}

func TestOIDCRejectsWrongAudience(t *testing.T) {
	fake := newFakeOIDCProvider(t)
	fake.audience = "different-client"
	router := fake.testRouter(t, "google", "")
	_, cookie, state := fake.authorize(t, router)
	response := fake.callback(router, cookie, state)
	if response.Code != 401 {
		t.Fatalf("wrong audience accepted: %d", response.Code)
	}
}

func TestOIDCRejectsEmailCollision(t *testing.T) {
	fake := newFakeOIDCProvider(t)
	router := fake.testRouter(t, "google", "")
	existing := db.User{ID: "local-conflict", Email: "test@example.com", Name: "Existing", AuthType: "local", Status: "ACTIVE"}
	if err := db.DB.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	_, cookie, state := fake.authorize(t, router)
	response := fake.callback(router, cookie, state)
	if response.Code != 409 {
		t.Fatalf("SSO automatically linked by email: %d", response.Code)
	}
}

func TestOIDCStateReplayIsRejectedEvenIfUserExists(t *testing.T) {
	fake := newFakeOIDCProvider(t)
	router := fake.testRouter(t, "custom", "")
	_, cookie, state := fake.authorize(t, router)
	first := fake.callback(router, cookie, state)
	if first.Code != 302 {
		t.Fatalf("initial callback: %d", first.Code)
	}
	replay := fake.callback(router, cookie, state)
	if replay.Code != 400 {
		t.Fatal(fmt.Sprint("callback replay returned", replay.Code))
	}
}
