package audit_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

func TestWebhookSSRFPolicyAllowlistAndDialValidation(t *testing.T) {
	// Misspelled/unset environment variables must NEVER enable permissive webhook mode.
	t.Setenv("APP_ENV", "staging")
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:8888")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:8888")

	// Reject loopback, private, CGNAT, link-local metadata, IPv4-mapped IPv6, multicast without allowlist.
	for _, blocked := range []string{
		"https://127.0.0.1/hook",
		"https://[::1]/hook",
		"https://[::ffff:127.0.0.1]/hook",
		"https://[::ffff:169.254.169.254]/latest/meta-data",
		"https://169.254.169.254/latest/meta-data",
		"https://10.20.0.5/hook",
		"https://192.168.1.50:8443/hook",
		"https://172.16.0.10/hook",
		"https://100.64.0.5/hook",
		"https://0.0.0.0/hook",
		"https://224.0.0.1/hook",
		"https://localhost.localdomain",
	} {
		if blocked == "https://localhost.localdomain" {
			// Skip hostname-only check above; test dial-time resolver below.
			continue
		}
		if err := audit.ValidateWebhookURL(blocked, false); err == nil {
			t.Fatalf("expected SSRF policy to block %q by default", blocked)
		}
	}

	// Parse explicit restricted private allowlist; verify wildcards and http are rejected.
	if _, err := audit.ParseAllowlist("https://*.internal.corp"); err == nil {
		t.Fatal("expected wildcard host allowlist to be rejected")
	}
	if _, err := audit.ParseAllowlist("http://10.20.0.5:8080"); err == nil {
		t.Fatal("expected http scheme in allowlist to be rejected")
	}
	rules, err := audit.ParseAllowlist("https://hooks.internal.corp:8443,10.20.0.0/24:443")
	if err != nil {
		t.Fatalf("parse valid allowlist: %v", err)
	}
	restoreAllowlist := audit.SetAllowlistForTest(rules)
	defer restoreAllowlist()

	// Private destination allowed when host/IP and port strictly match allowlist.
	if err := audit.ValidateWebhookURL("https://10.20.0.15/hooks", false); err != nil {
		t.Fatalf("expected allowlisted private CIDR HTTPS port 443 to pass: %v", err)
	}
	// Wrong port must be rejected.
	if err := audit.ValidateWebhookURL("https://10.20.0.15:9000/hooks", false); err == nil {
		t.Fatal("expected non-allowlisted port on private IP to be rejected")
	}
	// Even WITH an allowlist matching, loopback and link-local metadata must ALWAYS be blocked.
	evenWithBroadAllowlist, _ := audit.ParseAllowlist("127.0.0.0/8:443,169.254.0.0/16:443")
	restoreBroad := audit.SetAllowlistForTest(evenWithBroadAllowlist)
	if err := audit.ValidateWebhookURL("https://127.0.0.1/hook", false); err == nil {
		t.Fatal("expected loopback to remain blocked even when allowlisted")
	}
	if err := audit.ValidateWebhookURL("https://169.254.169.254/latest", false); err == nil {
		t.Fatal("expected link-local metadata IP to remain blocked even when allowlisted")
	}
	restoreBroad()

	// Mixed DNS or DNS rebinding to private/loopback IP must fail at dial time.
	database := setupAuditDB(t)
	restoreDNS := audit.SetResolverForTest(func(ctx context.Context, network, host string) ([]netip.Addr, error) {
		if host == "rebind.example.com" {
			return []netip.Addr{
				netip.MustParseAddr("93.184.216.34"),
				netip.MustParseAddr("192.168.10.20"), // mixed private IP not allowlisted
			}, nil
		}
		if host == "meta.example.com" {
			return []netip.Addr{netip.MustParseAddr("::ffff:169.254.169.254")}, nil
		}
		return nil, errors.New("dns failure")
	})
	defer restoreDNS()

	entry := db.AuditLog{ID: "aud_ssrf", Sequence: 1, Action: "device.created", Severity: audit.SeverityInfo, Summary: "test"}
	for _, host := range []string{"rebind.example.com", "meta.example.com"} {
		hook := db.AlertWebhook{
			ID:           "wh_" + host,
			Name:         host,
			ProviderType: "generic",
			WebhookURL:   "https://" + host + "/secret-path-token?auth=topsecret",
			Events:       `["*"]`,
			IsActive:     true,
		}
		if err := database.Create(&hook).Error; err != nil {
			t.Fatal(err)
		}
		_, delErr := audit.DeliverWebhook(context.Background(), database, hook, entry)
		if delErr == nil {
			t.Fatalf("expected dial-time SSRF rejection for %s", host)
		}
		if strings.Contains(delErr.Error(), "secret-path-token") || strings.Contains(delErr.Error(), "topsecret") {
			t.Fatalf("error leaked sensitive URL components: %v", delErr)
		}
		var saved db.AlertWebhook
		_ = database.First(&saved, "id = ?", hook.ID).Error
		if strings.Contains(saved.LastError, "secret-path-token") || strings.Contains(saved.LastError, "topsecret") {
			t.Fatalf("persisted last_error leaked sensitive URL components: %s", saved.LastError)
		}
	}
}

func TestWebhookCredentialSanitizationAcrossFailureModes(t *testing.T) {
	database := setupAuditDB(t)
	sentinelSecret := "SUPER_SECRET_BOT_TOKEN_XYZ_98765"
	sentinelQuery := "apiKey=QUERY_SECRET_12345"
	webhookURL := "https://hooks.example.com/services/" + sentinelSecret + "?" + sentinelQuery

	hook := db.AlertWebhook{
		ID:            "wh_redact",
		Name:          "Redaction Check",
		ProviderType:  "generic",
		WebhookURL:    webhookURL,
		SigningSecret: "SIGNING_SECRET_ABC",
		Events:        `["*"]`,
		IsActive:      true,
	}
	if err := database.Create(&hook).Error; err != nil {
		t.Fatal(err)
	}
	entry := db.AuditLog{ID: "aud_redact", Sequence: 1, Action: "device.revoked", Severity: audit.SeverityCritical, Summary: "revoked"}

	// Simulate raw *url.Error embedding full URL and timeout/DNS/redirect failures.
	modes := []func(*http.Request) (*http.Response, error){
		func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("Post %q: context deadline exceeded", req.URL.String())
		},
		func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("Post %q: x509: certificate signed by unknown authority", req.URL.String())
		},
		func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("Get %q: %w", req.URL.String(), audit.ErrWebhookRedirectRejected)
		},
		func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(`{"detail":"bad token ` + sentinelSecret + `"}`)),
				Header:     make(http.Header),
			}, nil
		},
	}

	for i, fn := range modes {
		restore := audit.SetWebhookClientForTest(&http.Client{Transport: roundTripFunc(fn)})
		_, err := audit.DeliverWebhook(context.Background(), database, hook, entry)
		restore()
		if err == nil {
			t.Fatalf("mode %d expected error", i)
		}
		if strings.Contains(err.Error(), sentinelSecret) || strings.Contains(err.Error(), sentinelQuery) || strings.Contains(err.Error(), "SIGNING_SECRET_ABC") {
			t.Fatalf("mode %d error leaked credentials: %v", i, err)
		}
		var saved db.AlertWebhook
		_ = database.First(&saved, "id = ?", hook.ID).Error
		if strings.Contains(saved.LastError, sentinelSecret) || strings.Contains(saved.LastError, sentinelQuery) {
			t.Fatalf("mode %d persisted last_error leaked credentials: %q", i, saved.LastError)
		}
	}
}

func TestWebhookOutboxReliabilityRetryLeaseAndRestoreCancellation(t *testing.T) {
	database := setupAuditDB(t)
	ctx := context.Background()

	// Real local test HTTP receiver with controllable response codes and dedupe tracking.
	var mu sync.Mutex
	var attempts atomic.Int32
	var receivedDeliveries []string
	var mode atomic.Int32 // 0 -> 503, 1 -> 200 OK, 2 -> 400 permanent error

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedDeliveries = append(receivedDeliveries, r.Header.Get("X-SwanPanel-Delivery"))
		mu.Unlock()
		attempts.Add(1)

		// Verify HMAC header.
		mac := hmac.New(sha256.New, []byte("outbox-signing-secret"))
		mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if r.Header.Get("X-SwanPanel-Signature") != expected {
			http.Error(w, "bad sig", http.StatusBadRequest)
			return
		}
		switch mode.Load() {
		case 0:
			http.Error(w, "busy", http.StatusServiceUnavailable)
		case 1:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case 2:
			http.Error(w, "permanent bad request", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	// Point test client at TLS test server; URL itself is public-looking HTTPS for validation.
	restoreClient := audit.SetWebhookClientForTest(srv.Client())
	defer restoreClient()

	hook := db.AlertWebhook{
		ID:            "wh_outbox",
		Name:          "Outbox Receiver",
		ProviderType:  "generic",
		WebhookURL:    srv.URL + "/ingest",
		SigningSecret: "outbox-signing-secret",
		Events:        `["device.*", "user.*"]`,
		IsActive:      true,
	}
	// Route requests to srv.URL via custom transport overriding host rewrite while keeping Valid HTTPS host check.
	hook.WebhookURL = "https://siem.example.com/ingest"
	clientWithRewrite := srv.Client()
	origTransport := clientWithRewrite.Transport
	clientWithRewrite.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		cloned := req.Clone(req.Context())
		cloned.URL.Scheme = "https"
		cloned.URL.Host = strings.TrimPrefix(srv.URL, "https://")
		return origTransport.RoundTrip(cloned)
	})
	audit.SetWebhookClientForTest(clientWithRewrite)

	if err := database.Create(&hook).Error; err != nil {
		t.Fatal(err)
	}

	// Burst exceeding worker count (e.g. 10 concurrent events) enqueues all durably without drops.
	for i := 0; i < 10; i++ {
		if _, err := audit.Record(ctx, database, audit.EntryInput{
			ActorEmail: "admin@example.com",
			ActorRole:  "admin",
			Action:     "device.created",
			Severity:   audit.SeverityInfo,
			TargetType: "device",
			TargetID:   fmt.Sprintf("dev_%d", i),
			Summary:    fmt.Sprintf("Created device %d", i),
		}); err != nil {
			t.Fatalf("record audit #%d: %v", i, err)
		}
	}

	var queuedCount int64
	if err := database.Model(&db.WebhookDelivery{}).Where("status = ?", audit.DeliveryPending).Count(&queuedCount).Error; err != nil {
		t.Fatal(err)
	}
	if queuedCount != 10 {
		t.Fatalf("expected all 10 burst deliveries durably enqueued, got %d", queuedCount)
	}

	// Bounded worker batching drains all 10 pending deliveries safely across passes without starvation.
	mode.Store(0)
	totalFirstPass := 0
	for i := 0; i < 4; i++ {
		processed, err := audit.ProcessOutboxBatch(ctx, database, fmt.Sprintf("worker-pass1-%d", i))
		if err != nil {
			t.Fatalf("outbox batch %d failed: %v", i, err)
		}
		if processed == 0 {
			break
		}
		totalFirstPass += processed
	}
	if totalFirstPass != 10 {
		t.Fatalf("expected 10 total items processed across bounded worker batches, got %d", totalFirstPass)
	}
	var retryingCount int64
	_ = database.Model(&db.WebhookDelivery{}).Where("status = ?", audit.DeliveryRetrying).Count(&retryingCount).Error
	if retryingCount != 10 {
		t.Fatalf("expected 10 items in retrying state after 503, got %d", retryingCount)
	}

	// Simulate time advancing past retry delay (and normal restart recovery).
	_ = database.Model(&db.WebhookDelivery{}).Where("status = ?", audit.DeliveryRetrying).Update("next_attempt_at", time.Now().UTC().Add(-time.Second)).Error
	mode.Store(1)
	for i := 0; i < 4; i++ {
		n, err := audit.ProcessOutboxBatch(ctx, database, fmt.Sprintf("worker-pass2-%d", i))
		if err != nil {
			t.Fatalf("retry pass failed: %v", err)
		}
		if n == 0 {
			break
		}
	}
	var deliveredCount int64
	_ = database.Model(&db.WebhookDelivery{}).Where("status = ?", audit.DeliveryDelivered).Count(&deliveredCount).Error
	if deliveredCount != 10 {
		t.Fatalf("expected all 10 items delivered on retry, got %d", deliveredCount)
	}

	// Permanent 4xx should transition directly to terminal failed without retrying.
	mode.Store(2)
	_, err := audit.Record(ctx, database, audit.EntryInput{
		Action:     "user.deleted",
		Severity:   audit.SeverityWarning,
		TargetType: "user",
		TargetID:   "usr_99",
		Summary:    "Deleted user",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = audit.ProcessOutboxBatch(ctx, database, "worker-3")
	var permFailed db.WebhookDelivery
	if err := database.Where("action = ?", "user.deleted").First(&permFailed).Error; err != nil {
		t.Fatal(err)
	}
	if permFailed.Status != audit.DeliveryFailed || permFailed.AttemptCount != 1 {
		t.Fatalf("expected immediate terminal failure on 400, got status=%s attempts=%d", permFailed.Status, permFailed.AttemptCount)
	}

	// Pending deliveries cancelled explicitly on restore.
	mode.Store(1)
	_, err = audit.Record(ctx, database, audit.EntryInput{
		Action:   "user.created",
		Severity: audit.SeverityInfo,
		Summary:  "Should be cancelled on restore",
	})
	if err != nil {
		t.Fatal(err)
	}
	var cancelledCount int64
	err = database.Transaction(func(tx *gorm.DB) error {
		var cErr error
		cancelledCount, cErr = audit.CancelPendingDeliveriesTx(tx)
		return cErr
	})
	if err != nil || cancelledCount != 1 {
		t.Fatalf("expected 1 cancelled delivery on restore, got %d err=%v", cancelledCount, err)
	}
	var cancelledRow db.WebhookDelivery
	_ = database.Where("action = ?", "user.created").First(&cancelledRow).Error
	if cancelledRow.Status != audit.DeliveryCancelled {
		t.Fatalf("expected cancelled status, got %s", cancelledRow.Status)
	}

	// Verify stable delivery IDs and JSON serialization on receiver.
	var payloadMap map[string]any
	samplePayload, _ := audit.BuildWebhookPayload(hook, db.AuditLog{ID: "aud_x", Action: "device.created"}, "whd_unique_123")
	_ = json.Unmarshal(samplePayload, &payloadMap)
	if payloadMap["delivery_id"] != "whd_unique_123" || payloadMap["event_id"] != "aud_x" {
		t.Fatalf("unexpected generic outbox payload: %s", string(samplePayload))
	}
}

func TestOutboxWorkerLifecycleAllowlistAndCancelledDeliveries(t *testing.T) {
	// Exercise ParseAllowlist valid & invalid forms and MaskWebhookURL.
	rules, err := audit.ParseAllowlist("10.20.0.0/16, siem.internal:8443, https://vault.corp.local:9443")
	if err != nil || len(rules) != 3 {
		t.Fatalf("expected 3 parsed allowlist rules, got %v err=%v", rules, err)
	}
	for _, bad := range []string{"*", "https://*", "http://siem.local:8443", "://bad-url"} {
		if _, err := audit.ParseAllowlist(bad); err == nil {
			t.Fatalf("expected allowlist entry %q rejected", bad)
		}
	}
	for _, raw := range []string{"://bad", "https://example.com", "https://example.com?q=secret", "https://example.com/single"} {
		_ = audit.MaskWebhookURL(raw)
	}
	masked := audit.MaskWebhookURL("https://hooks.slack.com/services/T000/B000/SECRET")
	if strings.Contains(masked, "SECRET") {
		t.Fatalf("MaskWebhookURL leaked secret segment: %s", masked)
	}
	if audit.MaskWebhookURL("://invalid") != "••••" ||
		audit.MaskWebhookURL("https://example.com") != "https://example.com" ||
		audit.MaskWebhookURL("https://example.com?token=secret") != "https://example.com/••••" ||
		audit.MaskWebhookURL("https://example.com/single") != "https://example.com/••••" {
		t.Fatalf("unexpected MaskWebhookURL edge results")
	}

	// Start and cleanly stop outbox worker while handling disabled hooks and bad snapshots.
	restoreClient := audit.SetWebhookClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
		}),
	})
	defer restoreClient()

	database := setupAuditDB(t)
	disabledHook := db.AlertWebhook{
		ID:           "wh_disabled_outbox",
		Name:         "Disabled Hook",
		ProviderType: "generic",
		WebhookURL:   "https://disabled.example.com/hook",
		Events:       `["*"]`,
		IsActive:     false,
	}
	if err := database.Create(&disabledHook).Error; err != nil {
		t.Fatal(err)
	}
	corruptItem := db.WebhookDelivery{
		ID:            "whd_bad_snapshot",
		WebhookID:     disabledHook.ID,
		EventID:       "aud_corrupt",
		Action:        "device.created",
		Severity:      audit.SeverityInfo,
		Status:        audit.DeliveryPending,
		EventJSON:     "{not-json",
		MaxAttempts:   3,
		NextAttemptAt: time.Now().UTC().Add(-time.Second),
	}
	if err := database.Create(&corruptItem).Error; err != nil {
		t.Fatal(err)
	}
	// Enable hook briefly so worker reaches snapshot unmarshal error -> DeliveryFailed.
	_ = database.Model(&db.AlertWebhook{}).Where("id = ?", disabledHook.ID).Update("is_active", true).Error

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	done := audit.StartOutboxWorker(workerCtx, func() *gorm.DB { return database })
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var row db.WebhookDelivery
		if err := database.First(&row, "id = ?", corruptItem.ID).Error; err == nil && row.Status == audit.DeliveryFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancelWorker()
	<-done

	var checked db.WebhookDelivery
	if err := database.First(&checked, "id = ?", corruptItem.ID).Error; err != nil || checked.Status != audit.DeliveryFailed {
		t.Fatalf("expected worker to mark corrupt snapshot DeliveryFailed, got %+v err=%v", checked, err)
	}

	// Verify disabled webhook cancels pending deliveries cleanly in ProcessOutboxBatch.
	_ = database.Model(&db.AlertWebhook{}).Where("id = ?", disabledHook.ID).Update("is_active", false).Error
	_ = database.Create(&db.WebhookDelivery{
		ID:            "whd_disabled_cancel",
		WebhookID:     disabledHook.ID,
		EventID:       "aud_disabled",
		Action:        "device.created",
		Severity:      audit.SeverityInfo,
		Status:        audit.DeliveryPending,
		EventJSON:     `{"id":"aud_disabled","action":"device.created"}`,
		MaxAttempts:   3,
		NextAttemptAt: time.Now().UTC().Add(-time.Second),
	}).Error
	if _, err := audit.ProcessOutboxBatch(context.Background(), database, "worker-cancel"); err != nil {
		t.Fatal(err)
	}
	var cancelled db.WebhookDelivery
	_ = database.First(&cancelled, "id = ?", "whd_disabled_cancel").Error
	if cancelled.Status != audit.DeliveryCancelled {
		t.Fatalf("expected disabled hook delivery cancelled, got %s", cancelled.Status)
	}

	// Deleted webhook removes target before outbox delivery -> DeliveryCancelled, and pruning trims >200 rows.
	_ = database.Create(&db.WebhookDelivery{
		ID:            "whd_deleted_hook",
		WebhookID:     "wh_never_existed",
		EventID:       "aud_missing_hook",
		Action:        "device.created",
		Severity:      audit.SeverityInfo,
		Status:        audit.DeliveryPending,
		EventJSON:     `{"id":"aud_missing_hook","action":"device.created"}`,
		MaxAttempts:   3,
		NextAttemptAt: time.Now().UTC().Add(-time.Second),
	}).Error
	for i := 0; i < 205; i++ {
		_ = database.Create(&db.WebhookDelivery{
			ID:            fmt.Sprintf("whd_prune_%03d", i),
			WebhookID:     disabledHook.ID,
			EventID:       fmt.Sprintf("aud_p_%03d", i),
			Action:        "device.created",
			Severity:      audit.SeverityInfo,
			Status:        audit.DeliveryDelivered,
			EventJSON:     `{}`,
			MaxAttempts:   3,
			NextAttemptAt: time.Now().UTC(),
		}).Error
	}
	if _, err := audit.ProcessOutboxBatch(context.Background(), database, "worker-prune"); err != nil {
		t.Fatal(err)
	}
	var delHookRow db.WebhookDelivery
	_ = database.First(&delHookRow, "id = ?", "whd_deleted_hook").Error
	if delHookRow.Status != audit.DeliveryCancelled {
		t.Fatalf("expected missing hook delivery cancelled, got %s", delHookRow.Status)
	}
	var terminalCount int64
	_ = database.Model(&db.WebhookDelivery{}).Count(&terminalCount).Error
	if terminalCount > 210 {
		t.Fatalf("expected terminal outbox rows pruned near retention cap, got %d", terminalCount)
	}
}

func TestWebhookHardeningMaskingReservedIPBlockingAndClaimOwnership(t *testing.T) {
	t.Setenv("APP_ENV", "production")

	// H05: Never reveal first URL path segment (e.g. Telegram bot token or secret slug).
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{"https://example.com/SECRET/path", "https://example.com/••••"},
		{"https://api.telegram.org/bot123456:TOPSECRET/sendMessage", "https://api.telegram.org/••••"},
		{"https://hooks.slack.com/services/T00/B00/XXX", "https://hooks.slack.com/••••"},
	} {
		if got := audit.MaskWebhookURL(tc.raw); got != tc.want {
			t.Fatalf("MaskWebhookURL(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}

	legacyScrubbed := audit.SanitizeStoredError("Post \"https://api.telegram.org/bot999:SECRET/sendMessage?token=1\": dial failed", "https://api.telegram.org/bot999:SECRET/sendMessage?token=1", "signing-sec")
	if strings.Contains(legacyScrubbed, "SECRET") || strings.Contains(legacyScrubbed, "bot999") {
		t.Fatalf("SanitizeStoredError leaked sensitive URL or token: %q", legacyScrubbed)
	}

	// H10: Special/reserved/non-routable/metadata targets remain blocked even if allowlisted.
	broadAllow, _ := audit.ParseAllowlist("0.0.0.0/0:443,100.64.0.0/10:443,192.0.2.0/24:443,2001:db8::/32:443,fd00:ec2::254/128:443")
	restoreAllow := audit.SetAllowlistForTest(broadAllow)
	defer restoreAllow()
	for _, blocked := range []string{
		"https://0.0.0.1/hook",
		"https://100.100.100.200/latest/meta-data",
		"https://192.0.2.10/hook",
		"https://198.51.100.4/hook",
		"https://203.0.113.9/hook",
		"https://198.18.0.1/hook",
		"https://240.0.0.1/hook",
		"https://[2001:db8::1]/hook",
		"https://[fec0::1]/hook",
		"https://[fd00:ec2::254]/latest/meta-data",
		"https://[100::1]/hook",
		"https://metadata.google.internal/computeMetadata/v1",
	} {
		if err := audit.ValidateWebhookURL(blocked, false); err == nil {
			t.Fatalf("expected always-blocked reserved/metadata target %q to be rejected even when allowlisted", blocked)
		}
	}
	// RFC 1918 and non-metadata CGNAT remain allowlistable per D03.
	corpRules, _ := audit.ParseAllowlist("100.64.0.0/16:443,10.10.0.0/16:443")
	restoreCorp := audit.SetAllowlistForTest(corpRules)
	defer restoreCorp()
	if err := audit.ValidateWebhookURL("https://100.64.0.5/hook", false); err != nil {
		t.Fatalf("expected non-metadata allowlisted CGNAT address to pass: %v", err)
	}

	// H11: Stale worker cannot overwrite an outbox delivery after claim ownership changes.
	database := setupAuditDB(t)
	restoreClient := audit.SetWebhookClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
		}),
	})
	defer restoreClient()
	hook := db.AlertWebhook{
		ID:           "wh_claim_owner",
		Name:         "Claim Test",
		ProviderType: "generic",
		WebhookURL:   "https://example.com/hook",
		Events:       `["*"]`,
		IsActive:     true,
	}
	if err := database.Create(&hook).Error; err != nil {
		t.Fatal(err)
	}
	stolen := db.WebhookDelivery{
		ID:            "whd_stolen_claim",
		WebhookID:     hook.ID,
		EventID:       "aud_claim",
		Action:        "device.created",
		Severity:      audit.SeverityInfo,
		Status:        audit.DeliveryPending,
		ClaimedBy:     "newer-worker",
		EventJSON:     `{"id":"aud_claim","action":"device.created"}`,
		MaxAttempts:   3,
		NextAttemptAt: time.Now().UTC().Add(-time.Minute),
	}
	if err := database.Create(&stolen).Error; err != nil {
		t.Fatal(err)
	}
	leaseFuture := time.Now().UTC().Add(time.Minute)
	_ = database.Model(&db.WebhookDelivery{}).Where("id = ?", stolen.ID).Update("lease_until", leaseFuture).Error
	processed, err := audit.ProcessOutboxBatch(context.Background(), database, "stale-worker")
	if err != nil || processed != 0 {
		t.Fatalf("expected stale-worker to claim 0 leased items, got %d err=%v", processed, err)
	}
}
