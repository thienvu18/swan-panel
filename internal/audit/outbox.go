package audit

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

const (
	DeliveryPending   = "pending"
	DeliveryRetrying  = "retrying"
	DeliveryDelivered = "delivered"
	DeliveryFailed    = "failed"
	DeliveryCancelled = "cancelled"

	outboxMaxAttempts    = 5
	outboxWorkers        = 4
	outboxLeaseDuration  = 45 * time.Second
	outboxBatchSize      = outboxWorkers
	outboxRetentionAge   = 7 * 24 * time.Hour
	outboxMaxTerminalRow = 2000
)

var (
	outboxWakeCh = make(chan struct{}, 1)
)

// NotifyOutbox wakes the background outbox dispatcher immediately after an audit commit.
func NotifyOutbox() {
	select {
	case outboxWakeCh <- struct{}{}:
	default:
	}
}

func shouldSkipOutboxForAction(action string) bool {
	return strings.HasPrefix(action, "webhook.delivery.") || action == "webhook.tested"
}

func enqueueWebhookDeliveriesTx(tx *gorm.DB, entry db.AuditLog) error {
	if tx == nil || shouldSkipOutboxForAction(entry.Action) {
		return nil
	}
	var hooks []db.AlertWebhook
	if err := tx.Where("is_active = ?", true).Find(&hooks).Error; err != nil {
		return err
	}
	if len(hooks) == 0 {
		return nil
	}
	encodedEvent, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, hook := range hooks {
		if !MatchesEvent(hook.Events, entry.Action, entry.Severity) {
			continue
		}
		item := db.WebhookDelivery{
			ID:            "whd_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16],
			EventID:       entry.ID,
			WebhookID:     hook.ID,
			Status:        DeliveryPending,
			AttemptCount:  0,
			MaxAttempts:   outboxMaxAttempts,
			NextAttemptAt: now,
			Action:        entry.Action,
			Severity:      entry.Severity,
			EventJSON:     string(encodedEvent),
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
	}
	return nil
}

// CancelPendingDeliveriesTx marks pending/retrying outbox deliveries as cancelled
// so restoring an older backup never automatically replays stale queued alerts.
func CancelPendingDeliveriesTx(tx *gorm.DB) (int64, error) {
	if tx == nil {
		return 0, nil
	}
	now := time.Now().UTC()
	res := tx.Model(&db.WebhookDelivery{}).
		Where("status IN ?", []string{DeliveryPending, DeliveryRetrying}).
		Updates(map[string]any{
			"status":       DeliveryCancelled,
			"claimed_by":   "",
			"lease_until":  nil,
			"last_error":   "Cancelled across system restore",
			"completed_at": now,
		})
	return res.RowsAffected, res.Error
}

func computeRetryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	base := 5 * time.Second
	for i := 1; i < attempt; i++ {
		base *= 3
		if base > 5*time.Minute {
			base = 5 * time.Minute
			break
		}
	}
	// Deterministic-safe bounded jitter [0, 1000ms) using crypto/rand.
	jitterMs := int64(0)
	if n, err := rand.Int(rand.Reader, big.NewInt(1000)); err == nil {
		jitterMs = n.Int64()
	}
	return base + time.Duration(jitterMs)*time.Millisecond
}

func shouldSkipAutomaticOutbound() bool {
	webhookClientMu.RLock()
	hasOverride := webhookClientOverride != nil
	webhookClientMu.RUnlock()
	return os.Getenv("APP_ENV") == "test" && !hasOverride
}

// ProcessOutboxBatch claims due pending/retrying deliveries using a bounded lease and
// processes them across bounded workers without dropping work on saturation.
func ProcessOutboxBatch(ctx context.Context, database *gorm.DB, workerInstanceID string) (int, error) {
	if database == nil {
		return 0, nil
	}
	if shouldSkipAutomaticOutbound() {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if workerInstanceID == "" {
		workerInstanceID = "outbox-" + uuid.New().String()[:8]
	}

	now := time.Now().UTC()
	leaseExpiry := now.Add(outboxLeaseDuration)
	var claimed []db.WebhookDelivery

	err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where(
			"status IN ? AND next_attempt_at <= ? AND (lease_until IS NULL OR lease_until < ?)",
			[]string{DeliveryPending, DeliveryRetrying},
			now,
			now,
		).Order("next_attempt_at asc, created_at asc").Limit(outboxBatchSize).Find(&claimed).Error; err != nil {
			return err
		}
		if len(claimed) == 0 {
			return nil
		}
		ids := make([]string, len(claimed))
		for i, item := range claimed {
			ids[i] = item.ID
		}
		return tx.Model(&db.WebhookDelivery{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"claimed_by":  workerInstanceID,
				"lease_until": leaseExpiry,
			}).Error
	})
	if err != nil {
		return 0, err
	}
	if len(claimed) == 0 {
		pruneTerminalDeliveries(ctx, database)
		return 0, nil
	}

	jobs := make(chan db.WebhookDelivery, len(claimed))
	for _, item := range claimed {
		jobs <- item
	}
	close(jobs)

	workerCount := outboxWorkers
	if len(claimed) < workerCount {
		workerCount = len(claimed)
	}
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				processSingleDelivery(ctx, database, workerInstanceID, item)
			}
		}()
	}
	wg.Wait()
	pruneTerminalDeliveries(ctx, database)
	return len(claimed), nil
}

func updateClaimedDelivery(ctx context.Context, database *gorm.DB, deliveryID, workerInstanceID string, updates map[string]any) {
	if database == nil {
		return
	}
	res := database.WithContext(ctx).Model(&db.WebhookDelivery{}).
		Where("id = ? AND claimed_by = ?", deliveryID, workerInstanceID).
		Updates(updates)
	if res.Error != nil {
		slog.Error("Failed to persist webhook outbox state transition", "delivery_id", deliveryID, "worker", workerInstanceID, "error", res.Error)
		return
	}
	if res.RowsAffected == 0 {
		slog.Warn("Skipped outbox delivery state update because worker claim ownership was lost", "delivery_id", deliveryID, "worker", workerInstanceID)
	}
}

func processSingleDelivery(ctx context.Context, database *gorm.DB, workerInstanceID string, item db.WebhookDelivery) {
	now := time.Now().UTC()
	var hook db.AlertWebhook
	if err := database.WithContext(ctx).First(&hook, "id = ?", item.WebhookID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			updateClaimedDelivery(ctx, database, item.ID, workerInstanceID, map[string]any{
				"status":       DeliveryCancelled,
				"claimed_by":   "",
				"lease_until":  nil,
				"last_error":   "Webhook deleted before delivery",
				"completed_at": now,
			})
			return
		}
		slog.Error("Transient database read failure loading webhook for outbox delivery", "delivery_id", item.ID, "webhook_id", item.WebhookID, "error", err)
		updateClaimedDelivery(ctx, database, item.ID, workerInstanceID, map[string]any{
			"claimed_by":      "",
			"lease_until":     nil,
			"next_attempt_at": time.Now().UTC().Add(5 * time.Second),
			"last_error":      "Temporary database read failure before webhook delivery",
		})
		return
	}
	if !hook.IsActive {
		updateClaimedDelivery(ctx, database, item.ID, workerInstanceID, map[string]any{
			"status":       DeliveryCancelled,
			"claimed_by":   "",
			"lease_until":  nil,
			"last_error":   "Webhook disabled",
			"completed_at": now,
		})
		return
	}

	var entry db.AuditLog
	if err := json.Unmarshal([]byte(item.EventJSON), &entry); err != nil {
		updateClaimedDelivery(ctx, database, item.ID, workerInstanceID, map[string]any{
			"status":       DeliveryFailed,
			"claimed_by":   "",
			"lease_until":  nil,
			"last_error":   "Invalid queued audit snapshot",
			"completed_at": now,
		})
		return
	}

	statusCode, retryable, err := deliverWithDeliveryID(ctx, database, hook, entry, item.ID)
	attempts := item.AttemptCount + 1
	if err == nil {
		updateClaimedDelivery(ctx, database, item.ID, workerInstanceID, map[string]any{
			"status":           DeliveryDelivered,
			"attempt_count":    attempts,
			"claimed_by":       "",
			"lease_until":      nil,
			"last_status_code": statusCode,
			"last_error":       "",
			"completed_at":     time.Now().UTC(),
		})
		return
	}

	safeErr := SanitizeWebhookError(err, hook.WebhookURL, hook.SigningSecret)
	if !retryable || attempts >= item.MaxAttempts {
		updateClaimedDelivery(ctx, database, item.ID, workerInstanceID, map[string]any{
			"status":           DeliveryFailed,
			"attempt_count":    attempts,
			"claimed_by":       "",
			"lease_until":      nil,
			"last_status_code": statusCode,
			"last_error":       safeErr,
			"completed_at":     time.Now().UTC(),
		})
		return
	}

	nextAttempt := time.Now().UTC().Add(computeRetryBackoff(attempts))
	updateClaimedDelivery(ctx, database, item.ID, workerInstanceID, map[string]any{
		"status":           DeliveryRetrying,
		"attempt_count":    attempts,
		"next_attempt_at":  nextAttempt,
		"claimed_by":       "",
		"lease_until":      nil,
		"last_status_code": statusCode,
		"last_error":       safeErr,
	})
}

func pruneTerminalDeliveries(ctx context.Context, database *gorm.DB) {
	if database == nil {
		return
	}
	cutoff := time.Now().UTC().Add(-outboxRetentionAge)
	if err := database.WithContext(ctx).
		Where("status IN ? AND updated_at < ?", []string{DeliveryDelivered, DeliveryFailed, DeliveryCancelled}, cutoff).
		Delete(&db.WebhookDelivery{}).Error; err != nil {
		slog.Warn("Failed to prune expired terminal webhook deliveries", "error", err)
	}

	var totalTerminal int64
	if err := database.WithContext(ctx).Model(&db.WebhookDelivery{}).
		Where("status IN ?", []string{DeliveryDelivered, DeliveryFailed, DeliveryCancelled}).
		Count(&totalTerminal).Error; err == nil && totalTerminal > outboxMaxTerminalRow {
		excess := int(totalTerminal - outboxMaxTerminalRow)
		var oldIDs []string
		if err := database.WithContext(ctx).Model(&db.WebhookDelivery{}).
			Where("status IN ?", []string{DeliveryDelivered, DeliveryFailed, DeliveryCancelled}).
			Order("updated_at asc").
			Limit(excess).
			Pluck("id", &oldIDs).Error; err == nil && len(oldIDs) > 0 {
			if delErr := database.WithContext(ctx).Where("id IN ?", oldIDs).Delete(&db.WebhookDelivery{}).Error; delErr != nil {
				slog.Warn("Failed to trim excess terminal webhook deliveries", "error", delErr)
			}
		}
	}
}

// StartOutboxWorker runs the persistent webhook delivery loop until ctx is cancelled.
func StartOutboxWorker(ctx context.Context, dbProvider func() *gorm.DB) <-chan struct{} {
	done := make(chan struct{})
	instanceID := "worker-" + uuid.New().String()[:8]
	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			processed := 0
			if database := dbProvider(); database != nil {
				var err error
				processed, err = ProcessOutboxBatch(ctx, database, instanceID)
				if err != nil && ctx.Err() == nil {
					slog.Warn("Webhook outbox processing pass failed", "error", err)
				}
			}
			if processed >= outboxBatchSize {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-outboxWakeCh:
			case <-ticker.C:
			}
		}
	}()
	return done
}
