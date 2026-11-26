package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/audit"
	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

// AuditHandler exposes immutable audit log queries, integrity verification, export, and alert webhooks.
type AuditHandler struct{}

// NewAuditHandler creates a new AuditHandler.
func NewAuditHandler() *AuditHandler {
	return &AuditHandler{}
}

// WebhookDTO is the safe external representation of an AlertWebhook without raw secret URLs or signing keys.
type WebhookDTO struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	ProviderType      string     `json:"provider_type"`
	WebhookURLPreview string     `json:"webhook_url_preview"`
	TelegramChatID    string     `json:"telegram_chat_id,omitempty"`
	HasSigningSecret  bool       `json:"has_signing_secret"`
	Events            []string   `json:"events"`
	IsActive          bool       `json:"is_active"`
	LastDeliveredAt   *time.Time `json:"last_delivered_at,omitempty"`
	LastStatusCode    int        `json:"last_status_code,omitempty"`
	LastError         string     `json:"last_error,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func toWebhookDTO(hook db.AlertWebhook) WebhookDTO {
	return WebhookDTO{
		ID:                hook.ID,
		Name:              hook.Name,
		ProviderType:      hook.ProviderType,
		WebhookURLPreview: audit.MaskWebhookURL(hook.WebhookURL),
		TelegramChatID:    hook.TelegramChatID,
		HasSigningSecret:  hook.SigningSecret != "",
		Events:            audit.DecodeEvents(hook.Events),
		IsActive:          hook.IsActive,
		LastDeliveredAt:   hook.LastDeliveredAt,
		LastStatusCode:    hook.LastStatusCode,
		LastError:         hook.LastError,
		CreatedAt:         hook.CreatedAt,
		UpdatedAt:         hook.UpdatedAt,
	}
}

func buildAuditLogQuery(c *gin.Context) *gorm.DB {
	query := db.DB.WithContext(c.Request.Context()).Model(&db.AuditLog{})

	if severity := strings.ToLower(strings.TrimSpace(c.Query("severity"))); severity != "" && severity != "all" {
		query = query.Where("severity = ?", severity)
	}
	if action := strings.TrimSpace(c.Query("action")); action != "" && action != "all" {
		if strings.HasSuffix(action, ".*") {
			query = query.Where("action LIKE ?", strings.TrimSuffix(action, "*")+"%")
		} else {
			query = query.Where("action = ?", action)
		}
	}
	if targetType := strings.TrimSpace(c.Query("target_type")); targetType != "" && targetType != "all" {
		query = query.Where("target_type = ?", targetType)
	}
	if actor := strings.TrimSpace(c.Query("actor")); actor != "" {
		like := "%" + actor + "%"
		query = query.Where("actor_email LIKE ? OR actor_id LIKE ?", like, like)
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		like := "%" + search + "%"
		query = query.Where("summary LIKE ? OR action LIKE ? OR actor_email LIKE ? OR target_id LIKE ? OR source_ip LIKE ?", like, like, like, like, like)
	}
	if fromRaw := strings.TrimSpace(c.Query("from")); fromRaw != "" {
		if t, err := time.Parse(time.RFC3339, fromRaw); err == nil {
			query = query.Where("created_at >= ?", t.UTC())
		}
	}
	if toRaw := strings.TrimSpace(c.Query("to")); toRaw != "" {
		if t, err := time.Parse(time.RFC3339, toRaw); err == nil {
			query = query.Where("created_at <= ?", t.UTC())
		}
	}
	return query
}

// ListAuditLogs returns paginated, filterable immutable audit entries (newest first).
func (h *AuditHandler) ListAuditLogs(c *gin.Context) {
	limit := 50
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			if parsed > 200 {
				parsed = 200
			}
			limit = parsed
		}
	}
	offset := 0
	if rawOffset := strings.TrimSpace(c.Query("offset")); rawOffset != "" {
		if parsed, err := strconv.Atoi(rawOffset); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	query := buildAuditLogQuery(c)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		slog.Error("Failed to count audit logs", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query audit logs"})
		return
	}

	var logs []db.AuditLog
	if err := query.Order("sequence desc").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		slog.Error("Failed to fetch audit logs", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch audit logs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"audit_logs": logs,
		"total":      total,
		"limit":      limit,
		"offset":     offset,
	})
}

// VerifyAuditChain verifies the SHA-256 hash chain across all stored audit entries.
func (h *AuditHandler) VerifyAuditChain(c *gin.Context) {
	result, err := audit.VerifyChain(c.Request.Context(), db.DB)
	if err != nil {
		slog.Error("Audit chain verification failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to verify audit log integrity"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"verification": result})
}

func safeCSVCell(value string) string {
	if value == "" {
		return ""
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	default:
		return value
	}
}

// ExportAuditLogs exports filtered audit logs as CSV or JSON Lines.
func (h *AuditHandler) ExportAuditLogs(c *gin.Context) {
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "csv")))
	if format != "csv" && format != "jsonl" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format must be csv or jsonl"})
		return
	}

	var logs []db.AuditLog
	if err := buildAuditLogQuery(c).Order("sequence asc").Limit(10000).Find(&logs).Error; err != nil {
		slog.Error("Failed to export audit logs", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to export audit logs"})
		return
	}

	c.Header("Cache-Control", "no-store, private")
	c.Header("Pragma", "no-cache")
	dateTag := time.Now().UTC().Format("2006-01-02")

	if format == "jsonl" {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		for _, entry := range logs {
			if err := enc.Encode(entry); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode export"})
				return
			}
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "swanpanel-audit-"+dateTag+".jsonl"))
		c.Data(http.StatusOK, "application/x-ndjson; charset=utf-8", buf.Bytes())
		return
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{
		"sequence", "id", "created_at", "severity", "action",
		"actor_id", "actor_email", "actor_role", "source_ip",
		"target_type", "target_id", "summary", "details", "prev_hash", "entry_hash",
	})
	for _, entry := range logs {
		_ = w.Write([]string{
			strconv.FormatUint(entry.Sequence, 10),
			safeCSVCell(entry.ID),
			entry.CreatedAt.UTC().Format(time.RFC3339),
			safeCSVCell(entry.Severity),
			safeCSVCell(entry.Action),
			safeCSVCell(entry.ActorID),
			safeCSVCell(entry.ActorEmail),
			safeCSVCell(entry.ActorRole),
			safeCSVCell(entry.SourceIP),
			safeCSVCell(entry.TargetType),
			safeCSVCell(entry.TargetID),
			safeCSVCell(entry.Summary),
			safeCSVCell(entry.Details),
			safeCSVCell(entry.PrevHash),
			safeCSVCell(entry.EntryHash),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to format CSV export"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "swanpanel-audit-"+dateTag+".csv"))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ListWebhooks returns all configured alert webhooks with redacted URLs and no secrets.
func (h *AuditHandler) ListWebhooks(c *gin.Context) {
	var hooks []db.AlertWebhook
	if err := db.DB.WithContext(c.Request.Context()).Order("created_at asc").Find(&hooks).Error; err != nil {
		slog.Error("Failed to query alert webhooks", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch webhooks"})
		return
	}
	out := make([]WebhookDTO, 0, len(hooks))
	for _, hook := range hooks {
		out = append(out, toWebhookDTO(hook))
	}
	c.JSON(http.StatusOK, gin.H{"webhooks": out})
}

func webhookAllowsLocalhost() bool {
	return os.Getenv("APP_ENV") != "production" && os.Getenv("ENVIRONMENT") != "production"
}

// CreateWebhook configures a new alert webhook endpoint.
func (h *AuditHandler) CreateWebhook(c *gin.Context) {
	var req struct {
		Name           string   `json:"name" binding:"required"`
		ProviderType   string   `json:"provider_type" binding:"required"`
		WebhookURL     string   `json:"webhook_url" binding:"required"`
		TelegramChatID string   `json:"telegram_chat_id"`
		SigningSecret  string   `json:"signing_secret"`
		Events         []string `json:"events" binding:"required"`
		IsActive       *bool    `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid webhook payload"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if len(name) == 0 || len(name) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Webhook name must be between 1 and 128 characters"})
		return
	}
	provider, err := audit.ValidateProviderType(req.ProviderType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := audit.ValidateWebhookURL(req.WebhookURL, webhookAllowsLocalhost()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	chatID := strings.TrimSpace(req.TelegramChatID)
	if provider == "telegram" && chatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Telegram chat ID is required for Telegram webhooks"})
		return
	}
	if len(req.SigningSecret) > 256 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Signing secret must not exceed 256 characters"})
		return
	}
	eventsJSON, _, err := audit.ParseAndNormalizeEvents(req.Events)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}

	hook := db.AlertWebhook{
		ID:             "wh_" + uuid.New().String()[:8],
		Name:           name,
		ProviderType:   provider,
		WebhookURL:     strings.TrimSpace(req.WebhookURL),
		TelegramChatID: chatID,
		SigningSecret:  req.SigningSecret,
		Events:         eventsJSON,
		IsActive:       active,
	}
	if err := db.DB.WithContext(c.Request.Context()).Create(&hook).Error; err != nil {
		slog.Error("Failed to persist alert webhook", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create webhook"})
		return
	}

	audit.RecordFromContext(c, audit.EntryInput{
		Action:     "webhook.created",
		Severity:   audit.SeverityInfo,
		TargetType: "webhook",
		TargetID:   hook.ID,
		Summary:    "Alert webhook created: " + hook.Name,
		Details:    map[string]any{"provider_type": hook.ProviderType, "events": audit.DecodeEvents(hook.Events)},
	})
	c.JSON(http.StatusCreated, gin.H{"webhook": toWebhookDTO(hook)})
}

// UpdateWebhook updates an existing alert webhook endpoint.
func (h *AuditHandler) UpdateWebhook(c *gin.Context) {
	id := c.Param("id")
	var hook db.AlertWebhook
	if err := db.DB.WithContext(c.Request.Context()).Where("id = ?", id).First(&hook).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Webhook not found"})
		return
	}

	var req struct {
		Name               *string  `json:"name"`
		ProviderType       *string  `json:"provider_type"`
		WebhookURL         *string  `json:"webhook_url"`
		TelegramChatID     *string  `json:"telegram_chat_id"`
		SigningSecret      *string  `json:"signing_secret"`
		ClearSigningSecret bool     `json:"clear_signing_secret"`
		Events             []string `json:"events"`
		IsActive           *bool    `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid webhook update payload"})
		return
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if len(name) == 0 || len(name) > 128 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Webhook name must be between 1 and 128 characters"})
			return
		}
		hook.Name = name
	}
	if req.ProviderType != nil {
		provider, err := audit.ValidateProviderType(*req.ProviderType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		hook.ProviderType = provider
	}
	if req.WebhookURL != nil && strings.TrimSpace(*req.WebhookURL) != "" {
		if err := audit.ValidateWebhookURL(*req.WebhookURL, webhookAllowsLocalhost()); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		hook.WebhookURL = strings.TrimSpace(*req.WebhookURL)
	}
	if req.TelegramChatID != nil {
		hook.TelegramChatID = strings.TrimSpace(*req.TelegramChatID)
	}
	if hook.ProviderType == "telegram" && hook.TelegramChatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Telegram chat ID is required for Telegram webhooks"})
		return
	}
	if req.ClearSigningSecret {
		hook.SigningSecret = ""
	} else if req.SigningSecret != nil && *req.SigningSecret != "" {
		if len(*req.SigningSecret) > 256 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Signing secret must not exceed 256 characters"})
			return
		}
		hook.SigningSecret = *req.SigningSecret
	}
	if req.Events != nil {
		eventsJSON, _, err := audit.ParseAndNormalizeEvents(req.Events)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		hook.Events = eventsJSON
	}
	if req.IsActive != nil {
		hook.IsActive = *req.IsActive
	}

	if err := db.DB.WithContext(c.Request.Context()).Save(&hook).Error; err != nil {
		slog.Error("Failed to update alert webhook", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update webhook"})
		return
	}

	audit.RecordFromContext(c, audit.EntryInput{
		Action:     "webhook.updated",
		Severity:   audit.SeverityInfo,
		TargetType: "webhook",
		TargetID:   hook.ID,
		Summary:    "Alert webhook updated: " + hook.Name,
		Details:    map[string]any{"provider_type": hook.ProviderType, "is_active": hook.IsActive},
	})
	c.JSON(http.StatusOK, gin.H{"webhook": toWebhookDTO(hook)})
}

// DeleteWebhook deletes an alert webhook.
func (h *AuditHandler) DeleteWebhook(c *gin.Context) {
	id := c.Param("id")
	var hook db.AlertWebhook
	if err := db.DB.WithContext(c.Request.Context()).Where("id = ?", id).First(&hook).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Webhook not found"})
		return
	}
	if err := db.DB.WithContext(c.Request.Context()).Where("id = ?", id).Delete(&db.AlertWebhook{}).Error; err != nil {
		slog.Error("Failed to delete alert webhook", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete webhook"})
		return
	}
	audit.RecordFromContext(c, audit.EntryInput{
		Action:     "webhook.deleted",
		Severity:   audit.SeverityWarning,
		TargetType: "webhook",
		TargetID:   id,
		Summary:    "Alert webhook deleted: " + hook.Name,
	})
	c.JSON(http.StatusOK, gin.H{"message": "Webhook deleted successfully"})
}

// TestWebhook sends a synthetic test notification to a configured webhook endpoint.
func (h *AuditHandler) TestWebhook(c *gin.Context) {
	id := c.Param("id")
	var hook db.AlertWebhook
	if err := db.DB.WithContext(c.Request.Context()).Where("id = ?", id).First(&hook).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Webhook not found"})
		return
	}

	testEntry := db.AuditLog{
		ID:         "aud_test_" + uuid.New().String()[:8],
		Sequence:   0,
		CreatedAt:  time.Now().UTC().Truncate(time.Millisecond),
		ActorID:    c.GetString("user_id"),
		ActorEmail: c.GetString("email"),
		ActorRole:  "admin",
		SourceIP:   c.ClientIP(),
		Action:     "webhook.test",
		Severity:   audit.SeverityInfo,
		TargetType: "webhook",
		TargetID:   hook.ID,
		Summary:    "Test alert notification from SwanPanel for webhook " + hook.Name,
	}
	testEntry.EntryHash = audit.ComputeEntryHash(testEntry)

	statusCode, err := audit.DeliverWebhook(c.Request.Context(), db.DB, hook, testEntry)
	audit.RecordFromContext(c, audit.EntryInput{
		Action:     "webhook.tested",
		Severity:   audit.SeverityInfo,
		TargetType: "webhook",
		TargetID:   hook.ID,
		Summary:    "Tested alert webhook: " + hook.Name,
		Details:    map[string]any{"status_code": statusCode, "success": err == nil},
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success":     false,
			"status_code": statusCode,
			"error":       "Webhook test delivery failed: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"status_code": statusCode,
		"message":     "Test alert delivered successfully",
	})
}
