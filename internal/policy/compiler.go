package policy

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

// CompilationResult represents compiled network traffic selectors and firewall filters for a device
type CompilationResult struct {
	DeviceID         string   `json:"device_id"`
	VirtualIP        string   `json:"virtual_ip"`
	AllowedSelectors []string `json:"allowed_selectors"` // Traffic Selectors (IKEv2 remote_ts)
	DenyFilters      []string `json:"deny_filters"`      // Gateway Kernel nftables drop rules
}

// Compiler compiles Zero-Trust access rules for enrolled devices
type Compiler struct {
	database *gorm.DB
}

// NewCompiler creates a new Policy Compiler
func NewCompiler(database *gorm.DB) *Compiler {
	return &Compiler{database: database}
}

// CompileForDevice evaluates all access rules and generates runtime Traffic Selectors and nftables DROP rules
func (c *Compiler) CompileForDevice(ctx context.Context, deviceID string) (*CompilationResult, error) {
	slog.Debug("Compiling Zero-Trust access policies for device", "device_id", deviceID)

	var dev db.Device
	if err := c.database.WithContext(ctx).Preload("User.UserGroups").Preload("ResourceGroup").Where("id = ?", deviceID).First(&dev).Error; err != nil {
		slog.Error("Device not found for policy compilation", "device_id", deviceID, "error", err)
		return nil, fmt.Errorf("device not found: %w", err)
	}

	result := &CompilationResult{
		DeviceID:  dev.ID,
		VirtualIP: dev.VirtualIP,
	}

	allowedSet := make(map[string]struct{})

	// Base rule: Enrolled Resource Group subnet
	if dev.ResourceGroup.CIDRBlock != "" {
		allowedSet[dev.ResourceGroup.CIDRBlock] = struct{}{}
	}

	// 1. If device belongs to a User, evaluate User Groups and Direct Grants
	if dev.UserID != nil && dev.User != nil {
		userID := *dev.UserID

		// Group Policies: find all groups user belongs to
		for _, grp := range dev.User.UserGroups {
			var rules []db.AccessRule
			if err := c.database.WithContext(ctx).Where("subject_type = ? AND subject_id = ? AND action = ?", "user_group", grp.ID, "ALLOW").Find(&rules).Error; err == nil {
				for _, r := range rules {
					targetCIDR := c.resolveTargetCIDR(ctx, r.TargetType, r.TargetValue)
					if targetCIDR != "" {
						allowedSet[targetCIDR] = struct{}{}
					}
				}
			}
		}

		// Direct Grants: ALLOW rules for this specific User
		var directRules []db.AccessRule
		if err := c.database.WithContext(ctx).Where("subject_type = ? AND subject_id = ? AND action = ?", "user", userID, "ALLOW").Find(&directRules).Error; err == nil {
			for _, r := range directRules {
				targetCIDR := c.resolveTargetCIDR(ctx, r.TargetType, r.TargetValue)
				if targetCIDR != "" {
					allowedSet[targetCIDR] = struct{}{}
				}
			}
		}

		// Deny Exclusions: DENY rules take ultimate precedence via gateway firewall
		var denyRules []db.AccessRule
		if err := c.database.WithContext(ctx).Where("subject_type = ? AND subject_id = ? AND action = ?", "user", userID, "DENY").Find(&denyRules).Error; err == nil {
			for _, r := range denyRules {
				targetCIDR := c.resolveTargetCIDR(ctx, r.TargetType, r.TargetValue)
				if targetCIDR != "" {
					// Synthesize nftables DROP rule for linux kernel gateway
					ruleCmd := fmt.Sprintf("nft add rule inet filter forward ip saddr %s ip daddr %s drop", dev.VirtualIP, targetCIDR)
					result.DenyFilters = append(result.DenyFilters, ruleCmd)
					slog.Info("Generated kernel exclusion DROP filter", "device", dev.Hostname, "user_id", userID, "target", targetCIDR)
				}
			}
		}
	}

	for cidr := range allowedSet {
		result.AllowedSelectors = append(result.AllowedSelectors, cidr)
	}

	slog.Info("Policy compilation complete", "device", dev.Hostname, "selectors_count", len(result.AllowedSelectors), "deny_filters_count", len(result.DenyFilters))
	return result, nil
}

func (c *Compiler) resolveTargetCIDR(ctx context.Context, targetType, targetValue string) string {
	if targetType == "cidr_ip" {
		val := strings.TrimSpace(targetValue)
		if !strings.Contains(val, "/") {
			val += "/32"
		}
		return val
	}

	if targetType == "resource_group" {
		var rg db.ResourceGroup
		if err := c.database.WithContext(ctx).Where("id = ? OR name = ?", targetValue, targetValue).First(&rg).Error; err == nil {
			return rg.CIDRBlock
		}
	}

	return ""
}
