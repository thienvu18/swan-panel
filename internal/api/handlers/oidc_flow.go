package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/api/middleware"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/db"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

const oidcTTL = 5 * time.Minute

func oidcHash(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }

func randomOIDCValue() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func validOIDCURL(v string) bool {
	u, err := url.Parse(v)
	if err != nil || u.User != nil || u.Hostname() == "" || u.Fragment != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1") && os.Getenv("APP_ENV") != "production" && os.Getenv("ENVIRONMENT") != "production"
}

func validateOIDCConfiguration(typ, issuer, auth, token, jwks, userInfo, mapping string) error {
	switch typ {
	case "google", "microsoft", "keycloak", "custom":
	default:
		return errors.New("unsupported identity provider type")
	}
	if !validOIDCURL(issuer) {
		return errors.New("valid HTTPS issuer URL required")
	}
	for _, endpoint := range []string{auth, token, jwks, userInfo} {
		if endpoint != "" && !validOIDCURL(endpoint) {
			return errors.New("OIDC endpoints must be HTTPS URLs")
		}
	}
	if typ != "custom" && (auth != "" || token != "" || jwks != "" || userInfo != "") {
		return errors.New("only custom identity providers may override protocol endpoints")
	}
	if typ == "custom" && (auth != "" || token != "" || jwks != "" || userInfo != "") {
		if auth == "" || token == "" || jwks == "" {
			return errors.New("custom OIDC endpoint overrides require authorization_url, token_url and jwks_url")
		}
	}
	if mapping != "" {
		var m map[string]string
		if err := json.Unmarshal([]byte(mapping), &m); err != nil {
			return errors.New("claim_mapping must be JSON object of claim names")
		}
		for k, v := range m {
			if k != "sub" && k != "email" && k != "name" && k != "email_verified" {
				return errors.New("unsupported claim mapping key: " + k)
			}
			if strings.TrimSpace(v) == "" {
				return errors.New("mapped claim names must not be empty")
			}
		}
	}
	return nil
}

// Replaceable in package tests with an in-memory TLS trust store.
var oidcHTTPClient = func() *http.Client { return &http.Client{Timeout: 8 * time.Second} }

// buildOIDCClient delegates discovery, signature/JWKS verification and token exchange to
// coreos/go-oidc and golang.org/x/oauth2. Application code only manages session state.
func buildOIDCClient(ctx context.Context, p db.OIDCProvider, redirect string) (*oidc.Provider, oauth2.Config, error) {
	var empty oauth2.Config
	if err := validateOIDCConfiguration(p.ProviderType, p.IssuerURL, p.AuthorizationURL, p.TokenURL, p.JWKSURL, p.UserInfoURL, p.ClaimMapping); err != nil {
		return nil, empty, err
	}
	clientCtx := oidc.ClientContext(ctx, oidcHTTPClient())
	var provider *oidc.Provider
	var err error
	if p.AuthorizationURL != "" {
		provider = (&oidc.ProviderConfig{
			IssuerURL:   strings.TrimRight(p.IssuerURL, "/"),
			AuthURL:     p.AuthorizationURL,
			TokenURL:    p.TokenURL,
			JWKSURL:     p.JWKSURL,
			UserInfoURL: p.UserInfoURL,
		}).NewProvider(clientCtx)
	} else {
		provider, err = oidc.NewProvider(clientCtx, strings.TrimRight(p.IssuerURL, "/"))
		if err != nil {
			return nil, empty, err
		}
	}
	endpoint := provider.Endpoint()
	if !validOIDCURL(endpoint.AuthURL) || !validOIDCURL(endpoint.TokenURL) {
		return nil, empty, errors.New("OIDC discovery returned insecure authorization/token endpoints")
	}
	if p.AuthorizationURL == "" {
		var metadata struct {
			JWKSURI     string `json:"jwks_uri"`
			UserInfoURL string `json:"userinfo_endpoint"`
		}
		if err := provider.Claims(&metadata); err != nil {
			return nil, empty, err
		}
		if !validOIDCURL(metadata.JWKSURI) {
			return nil, empty, errors.New("OIDC discovery returned insecure JWKS endpoint")
		}
		if metadata.UserInfoURL != "" && !validOIDCURL(metadata.UserInfoURL) {
			return nil, empty, errors.New("OIDC discovery returned insecure UserInfo endpoint")
		}
	}
	config := oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		RedirectURL:  redirect,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail},
	}
	return provider, config, nil
}

func (h *OIDCHandler) callbackURL(c *gin.Context) (string, error) {
	base := strings.TrimRight(os.Getenv("PUBLIC_URL"), "/")
	if base == "" {
		if h.cfg.Environment == "production" {
			return "", errors.New("PUBLIC_URL must be configured for OIDC in production")
		}
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		base = scheme + "://" + c.Request.Host
	}
	if !validOIDCURL(base) {
		return "", errors.New("PUBLIC_URL must be an HTTPS origin")
	}
	return base + "/api/auth/oidc/callback", nil
}

func (h *OIDCHandler) PublicProviders(c *gin.Context) {
	var providers []db.OIDCProvider
	if err := db.DB.Where("is_active = ?", true).Order("name").Find(&providers).Error; err != nil {
		c.JSON(500, gin.H{"error": "SSO unavailable"})
		return
	}
	out := make([]gin.H, 0, len(providers))
	for _, p := range providers {
		out = append(out, gin.H{"id": p.ID, "name": p.Name, "provider_type": p.ProviderType})
	}
	c.JSON(200, gin.H{"providers": out})
}

func (h *OIDCHandler) StartLogin(c *gin.Context) {
	required, err := db.SetupRequired()
	if err != nil || required {
		c.JSON(http.StatusForbidden, gin.H{"error": "Complete initial setup first"})
		return
	}
	var p db.OIDCProvider
	if err := db.DB.Where("id = ? AND is_active = ?", c.Param("id"), true).First(&p).Error; err != nil {
		c.JSON(404, gin.H{"error": "Provider unavailable"})
		return
	}
	callback, err := h.callbackURL(c)
	if err != nil {
		c.JSON(503, gin.H{"error": err.Error()})
		return
	}
	_, oauthConfig, err := buildOIDCClient(c.Request.Context(), p, callback)
	if err != nil {
		c.JSON(502, gin.H{"error": "Provider discovery failed"})
		return
	}
	state, err := randomOIDCValue()
	if err != nil {
		c.Status(500)
		return
	}
	nonce, err := randomOIDCValue()
	if err != nil {
		c.Status(500)
		return
	}
	verifier := oauth2.GenerateVerifier()
	attempt := db.OIDCLoginAttempt{StateHash: oidcHash(state), ProviderID: p.ID, Nonce: nonce, PKCEVerifier: verifier, ExpiresAt: time.Now().Add(oidcTTL)}
	if err := db.DB.Create(&attempt).Error; err != nil {
		c.Status(500)
		return
	}
	authURL := oauthConfig.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce))
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("swan_oidc_state", state, int(oidcTTL.Seconds()), "/api/auth/oidc/callback", "", strings.HasPrefix(callback, "https://"), true)
	c.Header("Cache-Control", "no-store")
	c.Redirect(http.StatusFound, authURL)
}

func oidcMappedClaim(data map[string]any, path string) any {
	value := any(data)
	for _, part := range strings.Split(path, ".") {
		m, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = m[part]
	}
	return value
}

func (h *OIDCHandler) OIDCCallback(c *gin.Context) {
	state := c.Query("state")
	cookie, err := c.Cookie("swan_oidc_state")
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie), []byte(state)) != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or missing OIDC browser state"})
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("swan_oidc_state", "", -1, "/api/auth/oidc/callback", "", c.Request.TLS != nil, true)
	var attempt db.OIDCLoginAttempt
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&attempt, "state_hash = ?", oidcHash(state)).Error; err != nil {
			return err
		}
		if err := tx.Delete(&attempt).Error; err != nil {
			return err
		}
		if time.Now().After(attempt.ExpiresAt) {
			return errors.New("OIDC attempt expired")
		}
		return nil
	})
	if err != nil {
		c.JSON(400, gin.H{"error": "OIDC state expired or already consumed"})
		return
	}
	if c.Query("error") != "" {
		c.JSON(400, gin.H{"error": "Provider authorization declined"})
		return
	}
	if c.Query("code") == "" {
		c.JSON(400, gin.H{"error": "Missing authorization code"})
		return
	}

	var p db.OIDCProvider
	if err := db.DB.Where("id = ? AND is_active = ?", attempt.ProviderID, true).First(&p).Error; err != nil {
		c.JSON(403, gin.H{"error": "Provider disabled"})
		return
	}
	callback, err := h.callbackURL(c)
	if err != nil {
		c.JSON(503, gin.H{"error": "Callback not configured"})
		return
	}
	provider, oauthConfig, err := buildOIDCClient(c.Request.Context(), p, callback)
	if err != nil {
		c.JSON(502, gin.H{"error": "Provider configuration invalid"})
		return
	}
	ctx := oidc.ClientContext(c.Request.Context(), oidcHTTPClient())
	exchange, err := oauthConfig.Exchange(ctx, c.Query("code"), oauth2.VerifierOption(attempt.PKCEVerifier))
	if err != nil {
		c.JSON(401, gin.H{"error": "Provider rejected authorization code"})
		return
	}
	rawIDToken, ok := exchange.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		c.JSON(502, gin.H{"error": "Provider did not return an ID token"})
		return
	}
	idToken, err := provider.VerifierContext(ctx, &oidc.Config{ClientID: p.ClientID}).Verify(ctx, rawIDToken)
	if err != nil {
		c.JSON(401, gin.H{"error": "ID token verification failed"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(attempt.Nonce)) != 1 {
		c.JSON(401, gin.H{"error": "ID token nonce mismatch"})
		return
	}
	if idToken.Subject == "" {
		c.JSON(401, gin.H{"error": "ID token has no stable subject"})
		return
	}
	data := map[string]any{}
	if err := idToken.Claims(&data); err != nil {
		c.JSON(401, gin.H{"error": "Invalid ID token claims"})
		return
	}
	if provider.UserInfoEndpoint() != "" && exchange.AccessToken != "" {
		info, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(exchange))
		if err == nil && info.Subject == idToken.Subject {
			var userInfoClaims map[string]any
			if info.Claims(&userInfoClaims) == nil {
				for k, v := range userInfoClaims {
					if _, ok := data[k]; !ok {
						data[k] = v
					}
				}
			}
		}
	}
	mapping := map[string]string{"sub": "sub", "email": "email", "name": "name", "email_verified": "email_verified"}
	if p.ProviderType == "microsoft" {
		mapping["email"] = "preferred_username"
	}
	if p.ClaimMapping != "" {
		var configured map[string]string
		if json.Unmarshal([]byte(p.ClaimMapping), &configured) == nil {
			for k, v := range configured {
				mapping[k] = v
			}
		}
	}
	// A provider that explicitly marks an email as unverified cannot enroll
	// it as a trustworthy directory address. Admin approval remains required.
	if verified, ok := oidcMappedClaim(data, mapping["email_verified"]).(bool); ok && !verified {
		c.JSON(403, gin.H{"error": "Identity provider reports an unverified email address"})
		return
	}
	subject, _ := oidcMappedClaim(data, mapping["sub"]).(string)
	if subject != idToken.Subject {
		c.JSON(401, gin.H{"error": "Mapped subject does not match verified OIDC sub claim"})
		return
	}
	var user db.User
	err = db.DB.Where("idp_provider_id = ? AND idp_subject = ? AND auth_type = ?", p.ID, subject, "oidc").First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		email, _ := oidcMappedClaim(data, mapping["email"]).(string)
		if email == "" {
			email, _ = data["email"].(string)
		}
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" || !strings.Contains(email, "@") {
			c.JSON(403, gin.H{"error": "Provider did not supply an email address"})
			return
		}
		// Never link accounts on email alone. Conflicts require manual administrator resolution.
		var conflict int64
		if err := db.DB.Model(&db.User{}).Where("email = ?", email).Count(&conflict).Error; err != nil || conflict > 0 {
			c.JSON(409, gin.H{"error": "Email already belongs to another identity; ask an administrator to link it"})
			return
		}
		name, _ := oidcMappedClaim(data, mapping["name"]).(string)
		if name == "" {
			name = email
		}
		providerID := p.ID
		user = db.User{ID: "usr_" + uuid.NewString(), Name: name, Email: email, AuthType: "oidc", IDPProviderID: &providerID, IDPSubject: subject, Status: "PENDING"}
		if err := db.DB.Create(&user).Error; err != nil {
			c.JSON(409, gin.H{"error": "Could not register pending identity"})
			return
		}
		realtime.DefaultBroker.Broadcast("approval_queue_update", map[string]any{"user_id": user.ID, "status": "PENDING"})
	} else if err != nil {
		c.JSON(500, gin.H{"error": "Identity lookup failed"})
		return
	}
	if user.Status != "ACTIVE" {
		c.Header("Cache-Control", "no-store")
		c.Redirect(http.StatusFound, "/#/oidc-pending")
		return
	}
	ticket, err := randomOIDCValue()
	if err != nil {
		c.Status(500)
		return
	}
	if err := db.DB.Create(&db.OIDCLoginTicket{TicketHash: oidcHash(ticket), UserID: user.ID, ExpiresAt: time.Now().Add(time.Minute)}).Error; err != nil {
		c.Status(500)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("swan_oidc_ticket", ticket, 60, "/api/auth/oidc/exchange", "", strings.HasPrefix(callback, "https://"), true)
	c.Header("Cache-Control", "no-store")
	c.Redirect(http.StatusFound, "/#/oidc-complete")
}

func (h *OIDCHandler) ExchangeTicket(c *gin.Context) {
	if origin := c.GetHeader("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(u.Host, c.Request.Host) {
			c.JSON(403, gin.H{"error": "Invalid request origin"})
			return
		}
	}
	ticket, err := c.Cookie("swan_oidc_ticket")
	if err != nil || ticket == "" {
		c.JSON(401, gin.H{"error": "No pending SSO session"})
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("swan_oidc_ticket", "", -1, "/api/auth/oidc/exchange", "", c.Request.TLS != nil, true)
	var user db.User
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		var t db.OIDCLoginTicket
		if err := tx.First(&t, "ticket_hash = ?", oidcHash(ticket)).Error; err != nil {
			return err
		}
		if err := tx.Delete(&t).Error; err != nil {
			return err
		}
		if time.Now().After(t.ExpiresAt) {
			return errors.New("SSO session expired")
		}
		if err := tx.First(&user, "id = ?", t.UserID).Error; err != nil {
			return err
		}
		if user.Status != "ACTIVE" || user.AuthType != "oidc" {
			return errors.New("user not active")
		}
		return nil
	})
	if err != nil {
		c.JSON(401, gin.H{"error": "SSO session expired or already used"})
		return
	}
	signed, err := middleware.GenerateToken(&user, h.cfg.JWTSecret, 24*time.Hour)
	if err != nil {
		c.Status(500)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"token": signed, "user": gin.H{"id": user.ID, "email": user.Email, "name": user.Name, "is_admin": user.IsAdmin, "status": user.Status}})
}
