package audit

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
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
	"strings"
	"sync"
	"time"

	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

const webhookTimeout = 5 * time.Second

var (
	webhookSem = make(chan struct{}, 8)

	webhookClientMu       sync.RWMutex
	webhookClientOverride *http.Client
)

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

func currentWebhookClient(allowLocalhost bool) *http.Client {
	webhookClientMu.RLock()
	override := webhookClientOverride
	webhookClientMu.RUnlock()
	if override != nil {
		return override
	}

	dialer := &net.Dialer{Timeout: 3 * time.Second}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if !allowLocalhost {
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if isBlockedWebhookIP(ip) {
					return nil, fmt.Errorf("webhook destination resolves to restricted address")
				}
			}
			for _, ip := range ips {
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				err = dialErr
			}
			if err == nil {
				err = errors.New("no reachable webhook address")
			}
			return nil, err
		}
	}

	return &http.Client{
		Timeout:   webhookTimeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func isBlockedWebhookIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()
}

func allowLocalhostWebhooks() bool {
	return os.Getenv("APP_ENV") != "production" && os.Getenv("ENVIRONMENT") != "production"
}

// ValidateWebhookURL validates scheme, credentials, and SSRF restrictions on webhook URLs.
func ValidateWebhookURL(rawURL string, allowLocalhost bool) error {
	rawURL = strings.TrimSpace(rawURL)
	if len(rawURL) == 0 || len(rawURL) > 1024 {
		return errors.New("webhook URL must be between 1 and 1024 characters")
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.User != nil || u.Hostname() == "" || u.Fragment != "" {
		return errors.New("invalid webhook URL")
	}
	host := u.Hostname()
	if u.Scheme == "https" {
		if !allowLocalhost {
			if ip, err := netip.ParseAddr(host); err == nil && isBlockedWebhookIP(ip) {
				return errors.New("webhook URL cannot target private or loopback addresses in production")
			}
			if strings.EqualFold(host, "localhost") {
				return errors.New("webhook URL cannot target localhost in production")
			}
		}
		return nil
	}
	if allowLocalhost && u.Scheme == "http" && (strings.EqualFold(host, "localhost") || host == "127.0.0.1" || host == "::1") {
		return nil
	}
	return errors.New("webhook URL must use HTTPS")
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

// MaskWebhookURL redacts sensitive path tokens and query strings from stored webhook URLs.
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
	if len(parts) == 1 {
		return prefix + "/••••"
	}
	return prefix + "/" + parts[0] + "/••••"
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
func BuildWebhookPayload(webhook db.AlertWebhook, entry db.AuditLog) ([]byte, error) {
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

// DeliverWebhook sends one audit event to a webhook destination and records delivery telemetry.
func DeliverWebhook(ctx context.Context, database *gorm.DB, webhook db.AlertWebhook, entry db.AuditLog) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	allowLocal := allowLocalhostWebhooks()
	if err := ValidateWebhookURL(webhook.WebhookURL, allowLocal); err != nil {
		recordWebhookOutcome(ctx, database, webhook.ID, 0, err.Error())
		return 0, err
	}
	body, err := BuildWebhookPayload(webhook, entry)
	if err != nil {
		recordWebhookOutcome(ctx, database, webhook.ID, 0, err.Error())
		return 0, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, webhookTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, webhook.WebhookURL, bytes.NewReader(body))
	if err != nil {
		recordWebhookOutcome(ctx, database, webhook.ID, 0, err.Error())
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SwanPanel-Webhook/1.0")
	req.Header.Set("X-SwanPanel-Event", entry.Action)
	if webhook.SigningSecret != "" {
		req.Header.Set("X-SwanPanel-Signature", SignPayload(webhook.SigningSecret, body))
	}

	resp, err := currentWebhookClient(allowLocal).Do(req)
	if err != nil {
		slog.Warn("Webhook delivery failed", "webhook_id", webhook.ID, "provider", webhook.ProviderType, "error", err)
		recordWebhookOutcome(ctx, database, webhook.ID, 0, err.Error())
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8192))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		slog.Warn("Webhook endpoint returned non-2xx status", "webhook_id", webhook.ID, "status", resp.StatusCode)
		recordWebhookOutcome(ctx, database, webhook.ID, resp.StatusCode, errMsg)
		return resp.StatusCode, errors.New(errMsg)
	}

	recordWebhookOutcome(ctx, database, webhook.ID, resp.StatusCode, "")
	slog.Debug("Webhook alert delivered", "webhook_id", webhook.ID, "action", entry.Action, "status", resp.StatusCode)
	return resp.StatusCode, nil
}

func recordWebhookOutcome(ctx context.Context, database *gorm.DB, webhookID string, statusCode int, lastErr string) {
	if database == nil || webhookID == "" {
		return
	}
	now := time.Now().UTC()
	if len(lastErr) > 512 {
		lastErr = lastErr[:512]
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

// DispatchAsync sends matching active webhooks in bounded background workers.
func DispatchAsync(database *gorm.DB, entry db.AuditLog) {
	if database == nil {
		return
	}
	// Never fire outbound requests from unrelated unit tests unless a test client is installed.
	webhookClientMu.RLock()
	hasOverride := webhookClientOverride != nil
	webhookClientMu.RUnlock()
	if os.Getenv("APP_ENV") == "test" && !hasOverride {
		return
	}

	var hooks []db.AlertWebhook
	if err := database.Where("is_active = ?", true).Find(&hooks).Error; err != nil {
		slog.Warn("Failed to query active alert webhooks", "error", err)
		return
	}
	for _, hook := range hooks {
		if !MatchesEvent(hook.Events, entry.Action, entry.Severity) {
			continue
		}
		target := hook
		select {
		case webhookSem <- struct{}{}:
			go func() {
				defer func() { <-webhookSem }()
				_, _ = DeliverWebhook(context.Background(), database, target, entry)
			}()
		default:
			slog.Warn("Webhook dispatch queue saturated; skipping delivery", "webhook_id", target.ID, "action", entry.Action)
		}
	}
}
