package audit

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/api/realtime"
	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

const (
	// GenesisHash is the previous hash referenced by the first audit log entry.
	GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"

	bruteforceWindow        = 5 * time.Minute
	bruteforceThreshold     = 5
	bruteforceAlertCooldown = 5 * time.Minute
	bruteforceMaxTracked    = 2048
)

// EntryInput describes an auditable control-plane or security event.
type EntryInput struct {
	ActorID    string
	ActorEmail string
	ActorRole  string
	SourceIP   string
	Action     string
	Severity   string
	TargetType string
	TargetID   string
	Summary    string
	Details    map[string]any
}

// ChainVerification summarizes integrity verification across stored audit logs.
type ChainVerification struct {
	Valid           bool      `json:"valid"`
	Total           int64     `json:"total"`
	CheckedAt       time.Time `json:"checked_at"`
	BrokenSequence  uint64    `json:"broken_sequence,omitempty"`
	FailureReason   string    `json:"failure_reason,omitempty"`
	LatestSequence  uint64    `json:"latest_sequence,omitempty"`
	LatestEntryHash string    `json:"latest_entry_hash,omitempty"`
}

type canonicalEntry struct {
	ID         string `json:"id"`
	Sequence   uint64 `json:"sequence"`
	CreatedAt  string `json:"created_at"`
	ActorID    string `json:"actor_id"`
	ActorEmail string `json:"actor_email"`
	ActorRole  string `json:"actor_role"`
	SourceIP   string `json:"source_ip"`
	Action     string `json:"action"`
	Severity   string `json:"severity"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Summary    string `json:"summary"`
	Details    string `json:"details"`
	PrevHash   string `json:"prev_hash"`
}

// ComputeEntryHash computes the deterministic SHA-256 digest for an audit entry.
func ComputeEntryHash(entry db.AuditLog) string {
	payload := canonicalEntry{
		ID:         entry.ID,
		Sequence:   entry.Sequence,
		CreatedAt:  entry.CreatedAt.UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z"),
		ActorID:    entry.ActorID,
		ActorEmail: entry.ActorEmail,
		ActorRole:  entry.ActorRole,
		SourceIP:   entry.SourceIP,
		Action:     entry.Action,
		Severity:   entry.Severity,
		TargetType: entry.TargetType,
		TargetID:   entry.TargetID,
		Summary:    entry.Summary,
		Details:    entry.Details,
		PrevHash:   entry.PrevHash,
	}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func normalizeSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case SeverityCritical:
		return SeverityCritical
	case SeverityWarning, "warn":
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

func truncateField(v string, max int) string {
	v = strings.TrimSpace(v)
	if len(v) <= max {
		return v
	}
	return v[:max]
}

// EnrichFromContext populates actor identity and client IP fields from a Gin request context.
func EnrichFromContext(c *gin.Context, input EntryInput) EntryInput {
	if c == nil {
		return input
	}
	if input.ActorID == "" {
		input.ActorID = c.GetString("user_id")
	}
	if input.ActorEmail == "" {
		input.ActorEmail = c.GetString("email")
	}
	if input.ActorRole == "" {
		if input.ActorID == "" {
			input.ActorRole = "anonymous"
		} else if c.GetBool("is_admin") {
			input.ActorRole = "admin"
		} else {
			input.ActorRole = "user"
		}
	}
	if input.SourceIP == "" {
		input.SourceIP = c.ClientIP()
	}
	return input
}

// RecordTx appends an immutable hash-chained audit record and enqueues matching
// persistent webhook outbox deliveries inside the caller's existing database transaction.
// Call NotifyCommitted(entry) after the transaction commits.
func RecordTx(tx *gorm.DB, input EntryInput) (db.AuditLog, error) {
	if tx == nil {
		return db.AuditLog{}, errors.New("audit transaction is nil")
	}
	detailsJSON := ""
	if len(input.Details) > 0 {
		encoded, err := json.Marshal(input.Details)
		if err != nil {
			return db.AuditLog{}, fmt.Errorf("marshal audit details: %w", err)
		}
		detailsJSON = string(encoded)
	}

	var latest db.AuditLog
	var seq uint64 = 1
	prevHash := GenesisHash

	err := tx.Order("sequence desc").Limit(1).First(&latest).Error
	if err == nil {
		seq = latest.Sequence + 1
		prevHash = latest.EntryHash
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return db.AuditLog{}, fmt.Errorf("query latest audit sequence: %w", err)
	}

	created := db.AuditLog{
		ID:         "aud_" + uuid.New().String()[:12],
		Sequence:   seq,
		CreatedAt:  time.Now().UTC().Truncate(time.Millisecond),
		ActorID:    truncateField(input.ActorID, 64),
		ActorEmail: truncateField(input.ActorEmail, 256),
		ActorRole:  truncateField(input.ActorRole, 32),
		SourceIP:   truncateField(input.SourceIP, 64),
		Action:     truncateField(input.Action, 64),
		Severity:   normalizeSeverity(input.Severity),
		TargetType: truncateField(input.TargetType, 64),
		TargetID:   truncateField(input.TargetID, 128),
		Summary:    truncateField(input.Summary, 512),
		Details:    detailsJSON,
		PrevHash:   prevHash,
	}
	if created.ActorRole == "" {
		created.ActorRole = "system"
	}
	if created.Summary == "" {
		created.Summary = created.Action
	}
	created.EntryHash = ComputeEntryHash(created)
	if err := tx.Create(&created).Error; err != nil {
		return db.AuditLog{}, err
	}
	if err := enqueueWebhookDeliveriesTx(tx, created); err != nil {
		return db.AuditLog{}, fmt.Errorf("enqueue alert webhook outbox: %w", err)
	}
	return created, nil
}

// NotifyCommitted publishes SSE invalidations and wakes outbox workers after commit.
func NotifyCommitted(entry db.AuditLog) {
	slog.Info("Audit event recorded",
		"sequence", entry.Sequence,
		"action", entry.Action,
		"severity", entry.Severity,
		"actor", entry.ActorEmail,
		"target_type", entry.TargetType,
		"target_id", entry.TargetID,
	)

	if realtime.DefaultBroker != nil {
		realtime.DefaultBroker.Broadcast("audit_event", map[string]any{
			"id":       entry.ID,
			"sequence": entry.Sequence,
			"action":   entry.Action,
			"severity": entry.Severity,
		})
	}
	NotifyOutbox()
}

// Record appends an immutable, hash-chained audit entry in its own transaction and notifies outbox workers.
func Record(ctx context.Context, database *gorm.DB, input EntryInput) (db.AuditLog, error) {
	if database == nil {
		return db.AuditLog{}, errors.New("audit database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var created db.AuditLog
	err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var recErr error
		created, recErr = RecordTx(tx, input)
		return recErr
	})
	if err != nil {
		slog.Error("Failed to persist immutable audit entry", "action", input.Action, "error", err)
		return db.AuditLog{}, err
	}

	NotifyCommitted(created)
	return created, nil
}

// RecordFromContext records an audit event using actor metadata from the Gin HTTP context.
func RecordFromContext(c *gin.Context, input EntryInput) error {
	if db.DB == nil {
		return errors.New("audit database unavailable")
	}
	enriched := EnrichFromContext(c, input)
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	_, err := Record(ctx, db.DB, enriched)
	return err
}

// VerifyChain validates sequential ordering, previous-hash linkage, and entry hashes.
func VerifyChain(ctx context.Context, database *gorm.DB) (ChainVerification, error) {
	result := ChainVerification{
		Valid:     true,
		CheckedAt: time.Now().UTC(),
	}
	if database == nil {
		return result, errors.New("audit database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	rows, err := database.WithContext(ctx).Model(&db.AuditLog{}).Order("sequence asc").Rows()
	if err != nil {
		return result, fmt.Errorf("stream audit logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var expectedSeq uint64 = 1
	expectedPrev := GenesisHash
	for rows.Next() {
		var entry db.AuditLog
		if err := database.ScanRows(rows, &entry); err != nil {
			return result, fmt.Errorf("scan audit log row: %w", err)
		}
		result.Total++
		result.LatestSequence = entry.Sequence
		result.LatestEntryHash = entry.EntryHash

		if entry.Sequence != expectedSeq {
			result.Valid = false
			result.BrokenSequence = entry.Sequence
			result.FailureReason = fmt.Sprintf("sequence gap or mismatch: expected %d, got %d", expectedSeq, entry.Sequence)
			return result, nil
		}
		if subtle.ConstantTimeCompare([]byte(entry.PrevHash), []byte(expectedPrev)) != 1 {
			result.Valid = false
			result.BrokenSequence = entry.Sequence
			result.FailureReason = "previous hash linkage mismatch"
			return result, nil
		}
		recomputed := ComputeEntryHash(entry)
		if subtle.ConstantTimeCompare([]byte(entry.EntryHash), []byte(recomputed)) != 1 {
			result.Valid = false
			result.BrokenSequence = entry.Sequence
			result.FailureReason = "entry hash mismatch"
			return result, nil
		}
		expectedSeq++
		expectedPrev = entry.EntryHash
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate audit logs: %w", err)
	}
	return result, nil
}

type failureTracker struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	lastEmit map[string]time.Time
}

var loginFailureTracker = &failureTracker{
	attempts: make(map[string][]time.Time),
	lastEmit: make(map[string]time.Time),
}

// TrackFailedLogin records a failed authentication attempt and emits a critical
// anomaly event when repeated failures cross the threshold within the sliding window.
func TrackFailedLogin(c *gin.Context, email, reason string) {
	ip := ""
	if c != nil {
		ip = c.ClientIP()
	}
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if err := RecordFromContext(c, EntryInput{
		ActorEmail: normalizedEmail,
		ActorRole:  "anonymous",
		SourceIP:   ip,
		Action:     "auth.login.failed",
		Severity:   SeverityWarning,
		TargetType: "user",
		TargetID:   normalizedEmail,
		Summary:    "Failed sign-in attempt for " + normalizedEmail,
		Details:    map[string]any{"reason": reason},
	}); err != nil {
		slog.Warn("Failed to record failed login audit entry", "email", normalizedEmail, "error", err)
	}

	key := ip + "|" + normalizedEmail
	now := time.Now().UTC()
	cutoff := now.Add(-bruteforceWindow)

	loginFailureTracker.mu.Lock()
	if len(loginFailureTracker.attempts) >= bruteforceMaxTracked {
		loginFailureTracker.attempts = make(map[string][]time.Time)
		loginFailureTracker.lastEmit = make(map[string]time.Time)
	}
	list := loginFailureTracker.attempts[key]
	pruned := list[:0]
	for _, ts := range list {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}
	pruned = append(pruned, now)
	loginFailureTracker.attempts[key] = pruned

	shouldEscalate := len(pruned) >= bruteforceThreshold &&
		(loginFailureTracker.lastEmit[key].IsZero() || now.Sub(loginFailureTracker.lastEmit[key]) >= bruteforceAlertCooldown)
	if shouldEscalate {
		loginFailureTracker.lastEmit[key] = now
	}
	count := len(pruned)
	loginFailureTracker.mu.Unlock()

	if shouldEscalate {
		if err := RecordFromContext(c, EntryInput{
			ActorEmail: normalizedEmail,
			ActorRole:  "anonymous",
			SourceIP:   ip,
			Action:     "auth.bruteforce.suspected",
			Severity:   SeverityCritical,
			TargetType: "user",
			TargetID:   normalizedEmail,
			Summary:    fmt.Sprintf("Repeated failed sign-in attempts (%d in 5m) for %s from %s", count, normalizedEmail, ip),
			Details:    map[string]any{"attempts": count, "window_seconds": int(bruteforceWindow.Seconds())},
		}); err != nil {
			slog.Warn("Failed to record brute-force escalation audit entry", "email", normalizedEmail, "error", err)
		}
	}
}
