package audit_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

func setupAuditDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit_test.db")
	database, err := db.InitDB(path)
	if err != nil {
		t.Fatalf("failed to initialize audit db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := database.DB(); err == nil {
			_ = sqlDB.Close()
		}
		db.DB = nil
	})
	return database
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestAuditLogChainIsAppendOnlyAndDetectsTampering(t *testing.T) {
	database := setupAuditDB(t)
	ctx := context.Background()

	first, err := audit.Record(ctx, database, audit.EntryInput{
		ActorID:    "usr_admin",
		ActorEmail: "admin@example.com",
		ActorRole:  "admin",
		SourceIP:   "192.0.2.10",
		Action:     "auth.login.success",
		Severity:   audit.SeverityInfo,
		TargetType: "user",
		TargetID:   "usr_admin",
		Summary:    "Admin signed in",
	})
	if err != nil {
		t.Fatalf("record first entry: %v", err)
	}
	if first.Sequence != 1 || first.PrevHash != audit.GenesisHash {
		t.Fatalf("unexpected genesis linkage: seq=%d prev=%s", first.Sequence, first.PrevHash)
	}

	second, err := audit.Record(ctx, database, audit.EntryInput{
		ActorID:    "usr_admin",
		ActorEmail: "admin@example.com",
		ActorRole:  "admin",
		SourceIP:   "192.0.2.10",
		Action:     "device.revoked",
		Severity:   audit.SeverityCritical,
		TargetType: "device",
		TargetID:   "dev_123",
		Summary:    "Revoked laptop-dev",
		Details:    map[string]any{"hostname": "laptop-dev"},
	})
	if err != nil {
		t.Fatalf("record second entry: %v", err)
	}
	if second.Sequence != 2 || second.PrevHash != first.EntryHash {
		t.Fatalf("unexpected chain link: seq=%d prev=%s want=%s", second.Sequence, second.PrevHash, first.EntryHash)
	}

	// SQLite triggers must refuse UPDATE and DELETE on audit_logs.
	if err := database.Model(&db.AuditLog{}).Where("id = ?", first.ID).Update("summary", "tampered").Error; err == nil {
		t.Fatal("expected SQLite trigger to block UPDATE on audit_logs")
	}
	if err := database.Where("id = ?", second.ID).Delete(&db.AuditLog{}).Error; err == nil {
		t.Fatal("expected SQLite trigger to block DELETE on audit_logs")
	}

	verification, err := audit.VerifyChain(ctx, database)
	if err != nil {
		t.Fatalf("verify chain: %v", err)
	}
	if !verification.Valid || verification.Total != 2 || verification.LatestEntryHash != second.EntryHash {
		t.Fatalf("expected valid chain of 2 entries, got %+v", verification)
	}

	// Simulate out-of-band SQLite tampering (dropping triggers first) and ensure VerifyChain catches it.
	if err := database.Exec("DROP TRIGGER IF EXISTS trg_audit_logs_no_update").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("UPDATE audit_logs SET summary = 'rewritten' WHERE sequence = 1").Error; err != nil {
		t.Fatal(err)
	}
	tampered, err := audit.VerifyChain(ctx, database)
	if err != nil {
		t.Fatalf("verify tampered chain: %v", err)
	}
	if tampered.Valid || tampered.BrokenSequence != 1 {
		t.Fatalf("expected tampered entry #1 detection, got %+v", tampered)
	}
}

func TestFailedLoginTrackingEscalatesToBruteforceAlert(t *testing.T) {
	database := setupAuditDB(t)

	for i := 0; i < 6; i++ {
		audit.TrackFailedLogin(nil, "Brute@Example.com", "invalid_password")
	}

	var failedCount int64
	if err := database.Model(&db.AuditLog{}).Where("action = ?", "auth.login.failed").Count(&failedCount).Error; err != nil {
		t.Fatal(err)
	}
	if failedCount != 6 {
		t.Fatalf("expected 6 auth.login.failed entries, got %d", failedCount)
	}

	var escalatedCount int64
	if err := database.Model(&db.AuditLog{}).Where("action = ? AND severity = ?", "auth.bruteforce.suspected", audit.SeverityCritical).Count(&escalatedCount).Error; err != nil {
		t.Fatal(err)
	}
	if escalatedCount != 1 {
		t.Fatalf("expected 1 debounced brute-force escalation, got %d", escalatedCount)
	}
}

func TestWebhookURLValidationAndSSRFProtection(t *testing.T) {
	for _, bad := range []string{
		"",
		"ftp://example.com/hook",
		"http://hooks.slack.com/services/a/b/c",
		"https://user:pass@hooks.slack.com/services/a/b/c",
		"https://hooks.slack.com/services/a/b/c#frag",
		"https://localhost/hook",
		"https://127.0.0.1/hook",
		"https://10.0.0.5/hook",
		"https://169.254.169.254/latest/meta-data",
		"https://192.168.1.10/hook",
	} {
		if err := audit.ValidateWebhookURL(bad, false); err == nil {
			t.Fatalf("expected production webhook validation to reject %q", bad)
		}
	}

	if err := audit.ValidateWebhookURL("https://hooks.slack.com/services/T00/B00/SECRET", false); err != nil {
		t.Fatalf("expected valid HTTPS webhook URL to pass: %v", err)
	}
	if err := audit.ValidateWebhookURL("http://127.0.0.1:9000/hook", true); err != nil {
		t.Fatalf("expected localhost webhook to be allowed in dev mode: %v", err)
	}

	masked := audit.MaskWebhookURL("https://hooks.slack.com/services/T00/B00/SECRET")
	if strings.Contains(masked, "SECRET") || !strings.HasPrefix(masked, "https://hooks.slack.com/services/") {
		t.Fatalf("unexpected masked URL: %s", masked)
	}
}

func TestWebhookPayloadFormattingSigningAndDelivery(t *testing.T) {
	database := setupAuditDB(t)
	ctx := context.Background()

	entry := db.AuditLog{
		ID:         "aud_sample",
		Sequence:   7,
		ActorEmail: "secops@example.com",
		ActorRole:  "admin",
		SourceIP:   "198.51.100.4",
		Action:     "device.revoked",
		Severity:   audit.SeverityCritical,
		TargetType: "device",
		TargetID:   "dev_laptop",
		Summary:    "Revoked <script>alert(1)</script>",
		EntryHash:  strings.Repeat("a", 64),
	}

	// Verify Telegram HTML escaping.
	tgPayload, err := audit.BuildWebhookPayload(db.AlertWebhook{
		ProviderType:   "telegram",
		TelegramChatID: "-100999",
	}, entry)
	if err != nil {
		t.Fatalf("build telegram payload: %v", err)
	}
	if strings.Contains(string(tgPayload), "<script>") || !strings.Contains(string(tgPayload), "-100999") {
		t.Fatalf("unexpected telegram payload: %s", string(tgPayload))
	}

	// Verify Slack and Teams formats.
	slackPayload, err := audit.BuildWebhookPayload(db.AlertWebhook{ProviderType: "slack"}, entry)
	if err != nil || !strings.Contains(string(slackPayload), "blocks") {
		t.Fatalf("unexpected slack payload: %v %s", err, string(slackPayload))
	}
	teamsPayload, err := audit.BuildWebhookPayload(db.AlertWebhook{ProviderType: "teams"}, entry)
	if err != nil || !strings.Contains(string(teamsPayload), "AdaptiveCard") {
		t.Fatalf("unexpected teams payload: %v %s", err, string(teamsPayload))
	}

	// Verify event matching.
	eventsJSON := `["device.*","severity:critical","auth.login.failed"]`
	if !audit.MatchesEvent(eventsJSON, "device.created", "info") {
		t.Fatal("expected wildcard prefix match")
	}
	if !audit.MatchesEvent(eventsJSON, "gateway.offline", "critical") {
		t.Fatal("expected severity match")
	}
	if audit.MatchesEvent(eventsJSON, "dns.override.created", "info") {
		t.Fatal("did not expect unrelated event to match")
	}

	// Verify HMAC signing and delivery outcome persistence via mocked HTTP RoundTripper.
	var capturedSig, capturedEvent string
	var capturedBody []byte
	nextStatus := http.StatusOK
	restore := audit.SetWebhookClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			capturedSig = req.Header.Get("X-SwanPanel-Signature")
			capturedEvent = req.Header.Get("X-SwanPanel-Event")
			capturedBody, _ = io.ReadAll(req.Body)
			return &http.Response{
				StatusCode: nextStatus,
				Body:       io.NopCloser(bytes.NewBufferString("ok")),
				Header:     make(http.Header),
			}, nil
		}),
	})
	defer restore()

	hook := db.AlertWebhook{
		ID:            "wh_test",
		Name:          "SIEM Receiver",
		ProviderType:  "generic",
		WebhookURL:    "https://siem.example.com/ingest",
		SigningSecret: "super-secret-hmac-key",
		Events:        `["*"]`,
		IsActive:      true,
	}
	if err := database.Create(&hook).Error; err != nil {
		t.Fatal(err)
	}

	code, err := audit.DeliverWebhook(ctx, database, hook, entry)
	if err != nil || code != http.StatusOK {
		t.Fatalf("expected 200 delivery, got code=%d err=%v", code, err)
	}
	if capturedEvent != "device.revoked" {
		t.Fatalf("unexpected event header: %s", capturedEvent)
	}
	mac := hmac.New(sha256.New, []byte(hook.SigningSecret))
	mac.Write(capturedBody)
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if capturedSig != expectedSig {
		t.Fatalf("signature mismatch: got=%s want=%s", capturedSig, expectedSig)
	}
	var bodyMap map[string]any
	if err := json.Unmarshal(capturedBody, &bodyMap); err != nil || bodyMap["action"] != "device.revoked" {
		t.Fatalf("unexpected generic JSON payload: %s", string(capturedBody))
	}

	// Verify non-2xx error telemetry is stored.
	nextStatus = http.StatusBadGateway
	code, err = audit.DeliverWebhook(ctx, database, hook, entry)
	if err == nil || code != http.StatusBadGateway {
		t.Fatalf("expected 502 error, got code=%d err=%v", code, err)
	}
	var updated db.AlertWebhook
	if err := database.First(&updated, "id = ?", hook.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.LastStatusCode != http.StatusBadGateway || updated.LastError != "HTTP 502" || updated.LastDeliveredAt == nil {
		t.Fatalf("unexpected persisted delivery failure status: %+v", updated)
	}
}
