package audit

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

const webhookTimeout = 5 * time.Second

var (
	// ErrWebhookRedirectRejected is returned when an outbound webhook attempts to redirect.
	ErrWebhookRedirectRejected = errors.New("webhook HTTP redirects are not permitted")
	// ErrWebhookSSRFBlocked is returned when a webhook target or resolved IP violates SSRF policy.
	ErrWebhookSSRFBlocked = errors.New("webhook destination blocked by outbound security policy")
	// ErrWebhookInvalidURL is returned when a webhook URL fails syntactic or scheme validation.
	ErrWebhookInvalidURL = errors.New("invalid webhook URL")

	webhookClientMu       sync.RWMutex
	webhookClientOverride *http.Client

	lookupNetIPMu sync.RWMutex
	lookupNetIP   = net.DefaultResolver.LookupNetIP

	allowlistMu       sync.RWMutex
	allowlistOverride *[]AllowlistRule

	telegramBotPattern = regexp.MustCompile(`bot[0-9]+:[A-Za-z0-9_-]+`)
	rawURLPattern      = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

// AllowlistRule represents an explicit administrator-configured private HTTPS webhook destination.
type AllowlistRule struct {
	Host   string
	Prefix netip.Prefix
	HasIP  bool
	Port   uint16
}

// SetWebhookClientForTest overrides the HTTP client used for webhook delivery in tests.
func SetWebhookClientForTest(client *http.Client) func() {
	webhookClientMu.Lock()
	previous := webhookClientOverride
	webhookClientOverride = client
	webhookClientMu.Unlock()
	return func() {
		webhookClientMu.Lock()
		webhookClientOverride = previous
		webhookClientMu.Unlock()
	}
}

// SetResolverForTest overrides DNS lookup during webhook transport dialing in tests.
func SetResolverForTest(fn func(ctx context.Context, network, host string) ([]netip.Addr, error)) func() {
	lookupNetIPMu.Lock()
	prev := lookupNetIP
	if fn != nil {
		lookupNetIP = fn
	}
	lookupNetIPMu.Unlock()
	return func() {
		lookupNetIPMu.Lock()
		lookupNetIP = prev
		lookupNetIPMu.Unlock()
	}
}

// SetAllowlistForTest sets deterministic allowlist rules for unit tests.
func SetAllowlistForTest(rules []AllowlistRule) func() {
	allowlistMu.Lock()
	prev := allowlistOverride
	copied := append([]AllowlistRule(nil), rules...)
	allowlistOverride = &copied
	allowlistMu.Unlock()
	return func() {
		allowlistMu.Lock()
		allowlistOverride = prev
		allowlistMu.Unlock()
	}
}

// ParseAllowlist parses WEBHOOK_ALLOWED_DESTINATIONS syntax:
// comma-separated entries of `<hostname|ip|cidr>[:port]` or `https://<hostname|ip|cidr>[:port]`.
// Wildcards and non-HTTPS schemes are rejected fail-closed.
func ParseAllowlist(raw string) ([]AllowlistRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var rules []AllowlistRule
	for _, token := range strings.Split(raw, ",") {
		entry := strings.TrimSpace(token)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "*") {
			return nil, fmt.Errorf("wildcard hosts are not permitted in webhook allowlist: %q", entry)
		}
		if strings.HasPrefix(strings.ToLower(entry), "http://") {
			return nil, fmt.Errorf("plaintext http is not permitted in webhook allowlist: %q", entry)
		}
		if strings.HasPrefix(strings.ToLower(entry), "https://") {
			entry = entry[len("https://"):]
			if slash := strings.IndexByte(entry, '/'); slash >= 0 {
				entry = entry[:slash]
			}
		}
		hostPart := entry
		port := uint16(443)
		// Check host:port carefully (handling IPv6 CIDR/bracket notation).
		if strings.HasPrefix(entry, "[") {
			h, p, err := net.SplitHostPort(entry)
			if err != nil {
				hostPart = strings.Trim(entry, "[]")
			} else {
				hostPart = h
				parsedPort, pErr := parseValidPort(p)
				if pErr != nil {
					return nil, pErr
				}
				port = parsedPort
			}
		} else if colonCount := strings.Count(entry, ":"); colonCount == 1 {
			h, p, err := net.SplitHostPort(entry)
			if err != nil {
				return nil, fmt.Errorf("invalid allowlist destination %q: %w", entry, err)
			}
			hostPart = h
			parsedPort, pErr := parseValidPort(p)
			if pErr != nil {
				return nil, pErr
			}
			port = parsedPort
		} else if strings.Contains(entry, "/") && strings.LastIndexByte(entry, ':') > strings.LastIndexByte(entry, '/') {
			// Handle IPv6 CIDR + :port if split is unambiguous; e.g., [fc00::/7]:8443 is handled above.
			return nil, fmt.Errorf("bracket IPv6 CIDR allowlist entries with ports: %q", entry)
		}

		hostPart = strings.TrimSpace(strings.ToLower(strings.TrimSuffix(hostPart, ".")))
		if hostPart == "" {
			return nil, fmt.Errorf("empty host in webhook allowlist entry %q", entry)
		}
		if prefix, err := netip.ParsePrefix(hostPart); err == nil {
			unmapped := netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits())
			if prefix.Addr().Is4In6() {
				unmapped = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
			}
			rules = append(rules, AllowlistRule{Prefix: unmapped.Masked(), HasIP: true, Port: port})
			continue
		}
		if addr, err := netip.ParseAddr(hostPart); err == nil {
			u := addr.Unmap()
			bits := 32
			if u.Is6() {
				bits = 128
			}
			rules = append(rules, AllowlistRule{Prefix: netip.PrefixFrom(u, bits), HasIP: true, Port: port})
			continue
		}
		rules = append(rules, AllowlistRule{Host: hostPart, Port: port})
	}
	return rules, nil
}

func parseValidPort(raw string) (uint16, error) {
	val, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || val < 1 || val > 65535 {
		return 0, fmt.Errorf("invalid webhook allowlist port %q", raw)
	}
	return uint16(val), nil
}

// ActiveAllowlist reads configured private webhook destinations from environment or test override.
func ActiveAllowlist() []AllowlistRule {
	allowlistMu.RLock()
	override := allowlistOverride
	allowlistMu.RUnlock()
	if override != nil {
		return *override
	}
	rules, err := ParseAllowlist(os.Getenv("WEBHOOK_ALLOWED_DESTINATIONS"))
	if err != nil {
		slog.Warn("Ignoring invalid WEBHOOK_ALLOWED_DESTINATIONS configuration", "error", err)
		return nil
	}
	return rules
}

func resolveURLPort(u *url.URL) (uint16, error) {
	if raw := u.Port(); raw != "" {
		return parseValidPort(raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return 443, nil
	case "http":
		return 80, nil
	default:
		return 0, ErrWebhookInvalidURL
	}
}

var (
	alwaysBlockedPrefixes = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),          // RFC 1122 "this host on this network"
		netip.MustParsePrefix("100.100.100.200/32"), // Cloud provider metadata endpoint inside CGNAT
		netip.MustParsePrefix("192.0.0.0/24"),       // IETF Protocol Assignments
		netip.MustParsePrefix("192.0.2.0/24"),       // TEST-NET-1
		netip.MustParsePrefix("198.18.0.0/15"),      // Benchmarking
		netip.MustParsePrefix("198.51.100.0/24"),    // TEST-NET-2
		netip.MustParsePrefix("203.0.113.0/24"),     // TEST-NET-3
		netip.MustParsePrefix("240.0.0.0/4"),        // Reserved / Limited broadcast
		netip.MustParsePrefix("100::/64"),           // IPv6 Discard-Only
		netip.MustParsePrefix("2001::/32"),          // Teredo tunneling
		netip.MustParsePrefix("2001:db8::/32"),      // Documentation
		netip.MustParsePrefix("2002::/16"),          // 6to4 tunneling
		netip.MustParsePrefix("fec0::/10"),          // Deprecated IPv6 site-local
		netip.MustParsePrefix("fd00:ec2::254/128"),  // AWS EC2 IPv6 metadata
	}
	cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")
)

func isAlwaysBlockedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, pfx := range alwaysBlockedPrefixes {
		if pfx.Contains(ip) {
			return true
		}
	}
	return false
}

func isPrivateDestinationIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsPrivate() || cgnatPrefix.Contains(ip)
}

func hostMatchesAllowlist(host string, port uint16, allowlist []AllowlistRule) bool {
	cleanHost := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	ip, err := netip.ParseAddr(cleanHost)
	isIP := err == nil
	if isIP {
		ip = ip.Unmap()
	}
	for _, rule := range allowlist {
		if rule.Port != port {
			continue
		}
		if rule.HasIP && isIP && rule.Prefix.Contains(ip) {
			return true
		}
		if !rule.HasIP && !isIP && strings.EqualFold(rule.Host, cleanHost) {
			return true
		}
	}
	return false
}

func ipMatchesAllowlist(host string, port uint16, ip netip.Addr, allowlist []AllowlistRule) bool {
	ip = ip.Unmap()
	cleanHost := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, rule := range allowlist {
		if rule.Port != port {
			continue
		}
		if rule.HasIP {
			if rule.Prefix.Contains(ip) {
				return true
			}
			continue
		}
		if strings.EqualFold(rule.Host, cleanHost) {
			return true
		}
	}
	return false
}

func validateDialTarget(host string, port uint16, resolvedIPs []netip.Addr, allowlist []AllowlistRule) error {
	if len(resolvedIPs) == 0 {
		return ErrWebhookSSRFBlocked
	}
	for _, rawIP := range resolvedIPs {
		ip := rawIP.Unmap()
		if isAlwaysBlockedIP(ip) {
			return ErrWebhookSSRFBlocked
		}
		if isPrivateDestinationIP(ip) {
			if !ipMatchesAllowlist(host, port, ip, allowlist) {
				return ErrWebhookSSRFBlocked
			}
		}
	}
	return nil
}

func newWebhookTransport(allowlist []AllowlistRule, allowLocalhostInTest bool) *http.Transport {
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	transport := &http.Transport{
		// Explicitly disable environment proxies so HTTP_PROXY/HTTPS_PROXY cannot bypass SSRF checks.
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if !allowLocalhostInTest {
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, portStr, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			portNum, pErr := parseValidPort(portStr)
			if pErr != nil {
				return nil, ErrWebhookInvalidURL
			}

			lookupNetIPMu.RLock()
			resolver := lookupNetIP
			lookupNetIPMu.RUnlock()

			ips, err := resolver(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			if valErr := validateDialTarget(host, portNum, ips, allowlist); valErr != nil {
				return nil, valErr
			}
			var lastErr error
			for _, rawIP := range ips {
				ip := rawIP.Unmap()
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), portStr))
				if dialErr == nil {
					return conn, nil
				}
				lastErr = dialErr
			}
			if lastErr == nil {
				lastErr = errors.New("no reachable webhook address")
			}
			return nil, lastErr
		}
	}
	return transport
}

func currentWebhookClient(allowlist []AllowlistRule) *http.Client {
	webhookClientMu.RLock()
	override := webhookClientOverride
	webhookClientMu.RUnlock()
	if override != nil {
		return override
	}
	return &http.Client{
		Timeout:   webhookTimeout,
		Transport: newWebhookTransport(allowlist, false),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return ErrWebhookRedirectRejected
		},
	}
}

// ValidateWebhookURL validates scheme, credentials, and SSRF restrictions on webhook URLs.
// Fail-safe by default: strict public HTTPS is enforced unless destination explicitly matches
// WEBHOOK_ALLOWED_DESTINATIONS (private RFC1918/ULA HTTPS only) or allowLocalhostInTest is passed directly.
func ValidateWebhookURL(rawURL string, allowLocalhostInTest bool) error {
	return ValidateWebhookURLWithAllowlist(rawURL, ActiveAllowlist(), allowLocalhostInTest)
}

// ValidateWebhookURLWithAllowlist validates a webhook destination against explicit allowlist rules.
func ValidateWebhookURLWithAllowlist(rawURL string, allowlist []AllowlistRule, allowLocalhostInTest bool) error {
	rawURL = strings.TrimSpace(rawURL)
	if len(rawURL) == 0 || len(rawURL) > 1024 {
		return fmt.Errorf("%w: URL must be between 1 and 1024 characters", ErrWebhookInvalidURL)
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.User != nil || u.Hostname() == "" || u.Fragment != "" {
		return ErrWebhookInvalidURL
	}
	port, portErr := resolveURLPort(u)
	if portErr != nil {
		return ErrWebhookInvalidURL
	}
	host := u.Hostname()
	if strings.EqualFold(u.Scheme, "https") {
		if !allowLocalhostInTest {
			cleanHost := strings.ToLower(strings.TrimSuffix(host, "."))
			if cleanHost == "localhost" || strings.HasSuffix(cleanHost, ".localhost") ||
				cleanHost == "metadata.google.internal" || cleanHost == "metadata" {
				return fmt.Errorf("%w: reserved local or metadata hostnames are prohibited", ErrWebhookSSRFBlocked)
			}
			if ip, err := netip.ParseAddr(host); err == nil {
				unmapped := ip.Unmap()
				if isAlwaysBlockedIP(unmapped) {
					return fmt.Errorf("%w: restricted IP address", ErrWebhookSSRFBlocked)
				}
				if isPrivateDestinationIP(unmapped) && !hostMatchesAllowlist(host, port, allowlist) {
					return fmt.Errorf("%w: private IP address requires explicit allowlist entry", ErrWebhookSSRFBlocked)
				}
			}
		}
		return nil
	}
	if allowLocalhostInTest && strings.EqualFold(u.Scheme, "http") && (strings.EqualFold(host, "localhost") || host == "127.0.0.1" || host == "::1") {
		return nil
	}
	return fmt.Errorf("%w: webhook URL must use HTTPS", ErrWebhookInvalidURL)
}

// SanitizeWebhookError maps raw transport, URL, DNS, TLS, and HTTP errors into bounded,
// credential-safe diagnostic messages without leaking webhook URLs, tokens, queries, or secrets.
func SanitizeWebhookError(err error, sensitiveValues ...string) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrWebhookRedirectRejected) {
		return "HTTP redirects are not permitted for webhooks"
	}
	if errors.Is(err, ErrWebhookSSRFBlocked) {
		return "Webhook destination blocked by outbound security policy"
	}
	if errors.Is(err, ErrWebhookInvalidURL) {
		return "Invalid webhook destination URL"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Webhook delivery timed out"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		if errors.Is(urlErr.Err, ErrWebhookRedirectRejected) {
			return "HTTP redirects are not permitted for webhooks"
		}
		if errors.Is(urlErr.Err, ErrWebhookSSRFBlocked) {
			return "Webhook destination blocked by outbound security policy"
		}
		if urlErr.Timeout() {
			return "Webhook delivery timed out"
		}
		err = urlErr.Err
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "Webhook DNS resolution failed"
	}
	var certInvalid x509.CertificateInvalidError
	var unknownAuth x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	lowerMsg := strings.ToLower(err.Error())
	if errors.As(err, &certInvalid) || errors.As(err, &unknownAuth) || errors.As(err, &hostnameErr) || strings.Contains(lowerMsg, "tls:") || strings.Contains(lowerMsg, "x509:") {
		return "Webhook TLS certificate verification failed"
	}
	if strings.Contains(lowerMsg, "context deadline exceeded") {
		return "Webhook delivery timed out"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Timeout() {
			return "Webhook connection timed out"
		}
		return "Webhook network connection failed"
	}

	msg := err.Error()
	if strings.HasPrefix(msg, "HTTP ") && len(msg) <= 16 {
		return msg
	}
	if rawURLPattern.MatchString(msg) {
		return "Webhook delivery request failed"
	}
	for _, s := range sensitiveValues {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "[redacted]")
		}
	}
	msg = telegramBotPattern.ReplaceAllString(msg, "bot[redacted]")
	if len(msg) > 160 {
		msg = msg[:160]
	}
	return msg
}

// ValidateProviderType verifies supported webhook formats.
func ValidateProviderType(provider string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch p {
	case "generic", "slack", "telegram", "teams":
		return p, nil
	default:
		return "", errors.New("provider_type must be generic, slack, telegram, or teams")
	}
}

// ParseAndNormalizeEvents validates a list of event subscription patterns.
func ParseAndNormalizeEvents(events []string) (string, []string, error) {
	if len(events) == 0 {
		return "", nil, errors.New("at least one event pattern is required")
	}
	seen := make(map[string]bool, len(events))
	out := make([]string, 0, len(events))
	for _, raw := range events {
		ev := strings.ToLower(strings.TrimSpace(raw))
		if ev == "" || len(ev) > 64 || strings.ContainsAny(ev, "\r\n\t ") {
			return "", nil, errors.New("invalid event pattern")
		}
		if !seen[ev] {
			seen[ev] = true
			out = append(out, ev)
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", nil, err
	}
	return string(encoded), out, nil
}

// DecodeEvents parses stored event subscription JSON safely.
func DecodeEvents(eventsJSON string) []string {
	var list []string
	if err := json.Unmarshal([]byte(eventsJSON), &list); err != nil {
		return nil
	}
	return list
}

// SanitizeStoredError scrubs legacy persisted webhook diagnostic messages before API exposure.
func SanitizeStoredError(raw string, sensitiveValues ...string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	return SanitizeWebhookError(errors.New(trimmed), sensitiveValues...)
}

// MaskWebhookURL redacts all path segments, tokens, and query strings from stored webhook URLs.
func MaskWebhookURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "••••"
	}
	prefix := u.Scheme + "://" + u.Host
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		if u.RawQuery != "" {
			return prefix + "/••••"
		}
		return prefix
	}
	return prefix + "/••••"
}

// MatchesEvent reports whether an audit event matches the webhook's subscription patterns.
func MatchesEvent(eventsJSON, action, severity string) bool {
	patterns := DecodeEvents(eventsJSON)
	action = strings.ToLower(strings.TrimSpace(action))
	severity = strings.ToLower(strings.TrimSpace(severity))
	for _, p := range patterns {
		if p == "*" || p == action {
			return true
		}
		if strings.HasSuffix(p, ".*") && strings.HasPrefix(action, strings.TrimSuffix(p, "*")) {
			return true
		}
		if strings.HasPrefix(p, "severity:") && strings.TrimPrefix(p, "severity:") == severity {
			return true
		}
	}
	return false
}

// SignPayload returns the hex-encoded HMAC-SHA256 signature header value.
func SignPayload(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// BuildWebhookPayload formats the audit event for the configured webhook provider.
func BuildWebhookPayload(webhook db.AlertWebhook, entry db.AuditLog, deliveryID string) ([]byte, error) {
	actor := entry.ActorEmail
	if actor == "" {
		actor = entry.ActorRole
	}
	ts := entry.CreatedAt.UTC().Format(time.RFC3339)

	switch webhook.ProviderType {
	case "slack":
		text := fmt.Sprintf("[%s] SwanPanel %s: %s", strings.ToUpper(entry.Severity), entry.Action, entry.Summary)
		payload := map[string]any{
			"text": text,
			"blocks": []map[string]any{
				{
					"type": "section",
					"text": map[string]any{
						"type": "mrkdwn",
						"text": fmt.Sprintf("*SwanPanel Alert* (`%s` · `%s`)\n%s", entry.Severity, entry.Action, entry.Summary),
					},
				},
				{
					"type": "context",
					"elements": []map[string]any{
						{"type": "mrkdwn", "text": fmt.Sprintf("Actor: `%s` | Target: `%s:%s` | Seq: `#%d` | %s", actor, entry.TargetType, entry.TargetID, entry.Sequence, ts)},
					},
				},
			},
		}
		return json.Marshal(payload)

	case "telegram":
		msg := fmt.Sprintf(
			"<b>SwanPanel Alert [%s]</b>\n<b>Action:</b> <code>%s</code>\n<b>Summary:</b> %s\n<b>Actor:</b> <code>%s</code>\n<b>Target:</b> <code>%s:%s</code>\n<b>Time:</b> <code>%s</code> (#%d)",
			html.EscapeString(strings.ToUpper(entry.Severity)),
			html.EscapeString(entry.Action),
			html.EscapeString(entry.Summary),
			html.EscapeString(actor),
			html.EscapeString(entry.TargetType),
			html.EscapeString(entry.TargetID),
			html.EscapeString(ts),
			entry.Sequence,
		)
		payload := map[string]any{
			"text":                     msg,
			"parse_mode":               "HTML",
			"disable_web_page_preview": true,
		}
		if strings.TrimSpace(webhook.TelegramChatID) != "" {
			payload["chat_id"] = strings.TrimSpace(webhook.TelegramChatID)
		}
		return json.Marshal(payload)

	case "teams":
		payload := map[string]any{
			"type": "message",
			"attachments": []map[string]any{
				{
					"contentType": "application/vnd.microsoft.card.adaptive",
					"content": map[string]any{
						"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
						"type":    "AdaptiveCard",
						"version": "1.4",
						"body": []map[string]any{
							{
								"type":   "TextBlock",
								"size":   "Medium",
								"weight": "Bolder",
								"text":   fmt.Sprintf("SwanPanel Alert [%s]: %s", strings.ToUpper(entry.Severity), entry.Action),
							},
							{
								"type": "TextBlock",
								"wrap": true,
								"text": entry.Summary,
							},
							{
								"type": "FactSet",
								"facts": []map[string]string{
									{"title": "Actor", "value": actor},
									{"title": "Source IP", "value": entry.SourceIP},
									{"title": "Target", "value": entry.TargetType + ":" + entry.TargetID},
									{"title": "Sequence", "value": fmt.Sprintf("#%d", entry.Sequence)},
									{"title": "Time", "value": ts},
								},
							},
						},
					},
				},
			},
		}
		return json.Marshal(payload)

	default:
		var parsedDetails any
		if entry.Details != "" {
			_ = json.Unmarshal([]byte(entry.Details), &parsedDetails)
		}
		payload := map[string]any{
			"delivery_id": deliveryID,
			"event_id":    entry.ID,
			"sequence":    entry.Sequence,
			"timestamp":   ts,
			"action":      entry.Action,
			"severity":    entry.Severity,
			"actor_id":    entry.ActorID,
			"actor_email": entry.ActorEmail,
			"actor_role":  entry.ActorRole,
			"source_ip":   entry.SourceIP,
			"target_type": entry.TargetType,
			"target_id":   entry.TargetID,
			"summary":     entry.Summary,
			"details":     parsedDetails,
			"entry_hash":  entry.EntryHash,
		}
		return json.Marshal(payload)
	}
}

type webhookDeliveryError struct {
	sanitized string
	retryable bool
	status    int
}

func (e *webhookDeliveryError) Error() string { return e.sanitized }

func deliverWithDeliveryID(ctx context.Context, database *gorm.DB, webhook db.AlertWebhook, entry db.AuditLog, deliveryID string) (int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	allowlist := ActiveAllowlist()
	if err := ValidateWebhookURLWithAllowlist(webhook.WebhookURL, allowlist, false); err != nil {
		safeErr := SanitizeWebhookError(err, webhook.WebhookURL, webhook.SigningSecret)
		recordWebhookOutcome(ctx, database, webhook.ID, 0, safeErr)
		return 0, false, &webhookDeliveryError{sanitized: safeErr, retryable: false}
	}
	body, err := BuildWebhookPayload(webhook, entry, deliveryID)
	if err != nil {
		safeErr := "Failed to serialize webhook payload"
		recordWebhookOutcome(ctx, database, webhook.ID, 0, safeErr)
		return 0, false, &webhookDeliveryError{sanitized: safeErr, retryable: false}
	}

	reqCtx, cancel := context.WithTimeout(ctx, webhookTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, webhook.WebhookURL, bytes.NewReader(body))
	if err != nil {
		safeErr := SanitizeWebhookError(ErrWebhookInvalidURL, webhook.WebhookURL, webhook.SigningSecret)
		recordWebhookOutcome(ctx, database, webhook.ID, 0, safeErr)
		return 0, false, &webhookDeliveryError{sanitized: safeErr, retryable: false}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SwanPanel-Webhook/1.0")
	req.Header.Set("X-SwanPanel-Event", entry.Action)
	req.Header.Set("X-SwanPanel-Event-ID", entry.ID)
	if deliveryID != "" {
		req.Header.Set("X-SwanPanel-Delivery", deliveryID)
	}
	req.Header.Set("X-SwanPanel-Timestamp", entry.CreatedAt.UTC().Format(time.RFC3339))
	if webhook.SigningSecret != "" {
		req.Header.Set("X-SwanPanel-Signature", SignPayload(webhook.SigningSecret, body))
	}

	resp, err := currentWebhookClient(allowlist).Do(req)
	if err != nil {
		safeErr := SanitizeWebhookError(err, webhook.WebhookURL, webhook.SigningSecret)
		retryable := !errors.Is(err, ErrWebhookRedirectRejected) && !errors.Is(err, ErrWebhookSSRFBlocked)
		if urlErr, ok := err.(*url.Error); ok {
			if errors.Is(urlErr.Err, ErrWebhookRedirectRejected) || errors.Is(urlErr.Err, ErrWebhookSSRFBlocked) {
				retryable = false
			}
		}
		slog.Warn("Webhook delivery failed", "webhook_id", webhook.ID, "provider", webhook.ProviderType, "reason", safeErr)
		recordWebhookOutcome(ctx, database, webhook.ID, 0, safeErr)
		return 0, retryable, &webhookDeliveryError{sanitized: safeErr, retryable: retryable}
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8192))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		slog.Warn("Webhook endpoint returned non-2xx status", "webhook_id", webhook.ID, "status", resp.StatusCode, "retryable", retryable)
		recordWebhookOutcome(ctx, database, webhook.ID, resp.StatusCode, errMsg)
		return resp.StatusCode, retryable, &webhookDeliveryError{sanitized: errMsg, retryable: retryable, status: resp.StatusCode}
	}

	recordWebhookOutcome(ctx, database, webhook.ID, resp.StatusCode, "")
	slog.Debug("Webhook alert delivered", "webhook_id", webhook.ID, "action", entry.Action, "status", resp.StatusCode)
	return resp.StatusCode, false, nil
}

// DeliverWebhook sends one audit event to a webhook destination and records sanitized delivery telemetry.
func DeliverWebhook(ctx context.Context, database *gorm.DB, webhook db.AlertWebhook, entry db.AuditLog) (int, error) {
	status, _, err := deliverWithDeliveryID(ctx, database, webhook, entry, entry.ID)
	return status, err
}

func recordWebhookOutcome(ctx context.Context, database *gorm.DB, webhookID string, statusCode int, lastErr string) {
	if database == nil || webhookID == "" {
		return
	}
	now := time.Now().UTC()
	if len(lastErr) > 256 {
		lastErr = lastErr[:256]
	}
	if err := database.WithContext(ctx).Model(&db.AlertWebhook{}).
		Where("id = ?", webhookID).
		Updates(map[string]any{
			"last_delivered_at": now,
			"last_status_code":  statusCode,
			"last_error":        lastErr,
		}).Error; err != nil {
		slog.Warn("Failed to persist webhook delivery status", "webhook_id", webhookID, "error", err)
	}
}
