package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
	"gorm.io/gorm"
)

var (
	errMissingRuleSubject = errors.New("access rule subject does not exist")
	errMissingRuleTarget  = errors.New("target resource group does not exist")
)

// AccessHandler manages Zero-Trust access control policies (ALLOW/DENY)
type AccessHandler struct{}

// NewAccessHandler creates a new AccessHandler
func NewAccessHandler() *AccessHandler {
	return &AccessHandler{}
}

// EffectiveDeviceAccess returns an explanatory view of compiled policy.
// It does not claim to probe the live network or override the runtime apply state.
func (h *AccessHandler) EffectiveDeviceAccess(c *gin.Context) {
	deviceID := c.Param("id")
	rawID := strings.TrimPrefix(deviceID, "dev_")
	if _, err := uuid.Parse(rawID); err != nil {
		slog.Warn("Rejected effective policy lookup: malformed device ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	compiled, err := policy.NewCompiler(db.DB).CompileForDevice(c.Request.Context(), deviceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
			return
		}
		if errors.Is(err, policy.ErrDeviceRevoked) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Device is revoked"})
			return
		}
		slog.Warn("Unable to compile effective device policy", "device_id", deviceID, "error", err)
		c.JSON(http.StatusConflict, gin.H{"error": "Device policy is unavailable"})
		return
	}

	runtimeState, err := policy.GetApplyState(c.Request.Context(), db.DB)
	if err != nil {
		slog.Warn("Effective policy runtime state unavailable", "device_id", deviceID, "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Gateway apply state is unavailable"})
		return
	}

	response := gin.H{
		"policy": compiled,
		"runtime": gin.H{
			"status":             runtimeState.Status,
			"desired_generation": runtimeState.DesiredGeneration,
			"applied_generation": runtimeState.AppliedGeneration,
		},
	}
	if destination := strings.TrimSpace(c.Query("destination")); destination != "" {
		decision, evalErr := policy.EvaluateDestination(compiled, destination)
		if evalErr != nil {
			slog.Warn("Rejected effective policy destination", "device_id", deviceID, "error", evalErr)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Destination must be an IPv4 address"})
			return
		}
		response["decision"] = decision
	}
	c.JSON(http.StatusOK, response)
}

// RuleDTO represents an enriched access rule for client consumption
type RuleDTO struct {
	ID          string `json:"id"`
	SubjectType string `json:"subject_type"` // 'user_group', 'user', 'device'
	SubjectID   string `json:"subject_id"`
	SubjectName string `json:"subject_name"`
	Action      string `json:"action"`      // 'ALLOW' or 'DENY'
	TargetType  string `json:"target_type"` // 'resource_group', 'cidr_ip'
	TargetValue string `json:"target_value"`
	TargetName  string `json:"target_name"`
	RuleNature  string `json:"rule_nature"` // 'group_policy', 'direct_grant', 'user_exclusion'
}

// ListAccessRules returns enriched access rules using batched reference lookups.
func (h *AccessHandler) ListAccessRules(c *gin.Context) {
	slog.Debug("Listing access rules")
	rules, err := listAccessRuleDTOs(c.Request.Context(), db.DB)
	if err != nil {
		slog.Error("Failed to query access rules", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch access rules"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"access_rules": rules})
}

// CreateAccessRule creates a new rule
func (h *AccessHandler) CreateAccessRule(c *gin.Context) {
	var req struct {
		SubjectType string `json:"subject_type" binding:"required"`
		SubjectID   string `json:"subject_id" binding:"required"`
		Action      string `json:"action" binding:"required"`
		TargetType  string `json:"target_type" binding:"required"`
		TargetValue string `json:"target_value" binding:"required"`
		RuleNature  string `json:"rule_nature"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	if req.Action != "ALLOW" && req.Action != "DENY" {
		c.JSON(400, gin.H{"error": "Invalid policy action"})
		return
	}
	if req.SubjectType != "user" && req.SubjectType != "user_group" && req.SubjectType != "device" {
		c.JSON(400, gin.H{"error": "Invalid subject type"})
		return
	}
	switch req.TargetType {
	case "resource_group":
		// Existence is checked inside the write transaction below so subnet
		// deletion cannot race between the target check and INSERT.
	case "cidr_ip":
		target := strings.TrimSpace(req.TargetValue)
		if addr, err := netip.ParseAddr(target); err == nil {
			if !addr.Is4() {
				c.JSON(400, gin.H{"error": "IPv6 policies not supported"})
				return
			}
			req.TargetValue = netip.PrefixFrom(addr, 32).String()
		} else {
			prefix, err := netip.ParsePrefix(target)
			if err != nil || !prefix.Addr().Is4() {
				c.JSON(400, gin.H{"error": "Invalid IPv4 policy target"})
				return
			}
			req.TargetValue = prefix.Masked().String()
		}
	default:
		c.JSON(400, gin.H{"error": "Invalid target type"})
		return
	}
	ruleNature := req.RuleNature
	if ruleNature == "" {
		if req.Action == "DENY" {
			ruleNature = "user_exclusion"
		} else if req.SubjectType == "user_group" {
			ruleNature = "group_policy"
		} else {
			ruleNature = "direct_grant"
		}
	}

	rule := db.AccessRule{
		ID:          "rule_" + uuid.New().String()[:8],
		SubjectType: req.SubjectType,
		SubjectID:   req.SubjectID,
		Action:      req.Action,
		TargetType:  req.TargetType,
		TargetValue: req.TargetValue,
		RuleNature:  ruleNature,
	}

	err := db.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		subject := any(&db.User{})
		switch req.SubjectType {
		case "user_group":
			subject = &db.UserGroup{}
		case "device":
			subject = &db.Device{}
		}
		var subjectCount int64
		if err := tx.Model(subject).Where("id = ?", req.SubjectID).Count(&subjectCount).Error; err != nil {
			return fmt.Errorf("validate rule subject: %w", err)
		}
		if subjectCount == 0 {
			return errMissingRuleSubject
		}
		if req.TargetType == "resource_group" {
			var targetCount int64
			if err := tx.Model(&db.ResourceGroup{}).Where("id = ?", req.TargetValue).Count(&targetCount).Error; err != nil {
				return fmt.Errorf("validate rule target: %w", err)
			}
			if targetCount == 0 {
				return errMissingRuleTarget
			}
		}
		return tx.Create(&rule).Error
	})
	switch {
	case errors.Is(err, errMissingRuleSubject):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subject does not exist"})
		return
	case errors.Is(err, errMissingRuleTarget):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Target resource group does not exist"})
		return
	case err != nil:
		slog.Error("Failed to create access rule", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Failed to create access rule"})
		return
	}

	slog.Info("Access rule created", "id", rule.ID, "action", rule.Action, "nature", rule.RuleNature)
	if err := policy.Reconcile(c.Request.Context(), db.DB); err != nil {
		slog.Error("Policy saved but could not be enforced", "rule", rule.ID, "error", err)
		c.JSON(http.StatusAccepted, gin.H{"access_rule": rule, "runtime_status": "pending", "error": "Gateway policy reconciliation failed"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"access_rule": rule, "runtime_status": "applied"})
}

// DeleteAccessRule deletes an access rule
func (h *AccessHandler) DeleteAccessRule(c *gin.Context) {
	id := c.Param("id")
	result := db.DB.Where("id = ?", id).Delete(&db.AccessRule{})
	if err := result.Error; err != nil {
		slog.Error("Failed to delete access rule", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete access rule"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Access rule not found"})
		return
	}
	slog.Info("Access rule deleted", "id", id)
	if err := policy.Reconcile(c.Request.Context(), db.DB); err != nil {
		slog.Error("Policy deletion pending reconciliation", "rule", id, "error", err)
		c.JSON(http.StatusAccepted, gin.H{"runtime_status": "pending", "error": "Rule deleted but gateway policy not synchronized"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Access rule deleted and enforced"})
}
