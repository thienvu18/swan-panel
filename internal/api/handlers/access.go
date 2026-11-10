package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/swan-panel/swan-panel/internal/db"
)

// AccessHandler manages Zero-Trust access control policies (ALLOW/DENY)
type AccessHandler struct{}

// NewAccessHandler creates a new AccessHandler
func NewAccessHandler() *AccessHandler {
	return &AccessHandler{}
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

// ListAccessRules returns all access policies
func (h *AccessHandler) ListAccessRules(c *gin.Context) {
	slog.Debug("Listing access rules")
	var rules []db.AccessRule
	if err := db.DB.Order("created_at asc").Find(&rules).Error; err != nil {
		slog.Error("Failed to query access rules", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch access rules"})
		return
	}

	var dtos []RuleDTO
	for _, r := range rules {
		subjName := r.SubjectID
		if r.SubjectType == "user_group" {
			var g db.UserGroup
			if err := db.DB.Where("id = ?", r.SubjectID).First(&g).Error; err == nil {
				subjName = g.Name
			}
		} else if r.SubjectType == "user" {
			var u db.User
			if err := db.DB.Where("id = ?", r.SubjectID).First(&u).Error; err == nil {
				subjName = u.Name
			}
		} else if r.SubjectType == "device" {
			var d db.Device
			if err := db.DB.Where("id = ?", r.SubjectID).First(&d).Error; err == nil {
				subjName = d.Hostname
			}
		}

		targetName := r.TargetValue
		if r.TargetType == "resource_group" {
			var rg db.ResourceGroup
			if err := db.DB.Where("id = ?", r.TargetValue).First(&rg).Error; err == nil {
				targetName = rg.Name + " (" + rg.CIDRBlock + ")"
			}
		}

		dtos = append(dtos, RuleDTO{
			ID:          r.ID,
			SubjectType: r.SubjectType,
			SubjectID:   r.SubjectID,
			SubjectName: subjName,
			Action:      r.Action,
			TargetType:  r.TargetType,
			TargetValue: r.TargetValue,
			TargetName:  targetName,
			RuleNature:  r.RuleNature,
		})
	}

	c.JSON(http.StatusOK, gin.H{"access_rules": dtos})
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

	if err := db.DB.Create(&rule).Error; err != nil {
		slog.Error("Failed to create access rule", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create access rule"})
		return
	}

	slog.Info("Access rule created", "id", rule.ID, "action", rule.Action, "nature", rule.RuleNature)
	c.JSON(http.StatusCreated, gin.H{"access_rule": rule})
}

// DeleteAccessRule deletes an access rule
func (h *AccessHandler) DeleteAccessRule(c *gin.Context) {
	id := c.Param("id")
	if err := db.DB.Where("id = ?", id).Delete(&db.AccessRule{}).Error; err != nil {
		slog.Error("Failed to delete access rule", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete access rule"})
		return
	}

	slog.Info("Access rule deleted", "id", id)
	c.JSON(http.StatusOK, gin.H{"message": "Access rule deleted successfully"})
}
