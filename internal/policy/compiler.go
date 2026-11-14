package policy

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

type CompilationResult struct {
	DeviceID         string   `json:"device_id"`
	VirtualIP        string   `json:"virtual_ip"`
	AllowedSelectors []string `json:"allowed_selectors"`
	DeniedSelectors  []string `json:"denied_selectors"`
	DenyFilters      []string `json:"deny_filters"` // Legacy diagnostics; never execute these strings.
}

type Compiler struct{ database *gorm.DB }

func NewCompiler(database *gorm.DB) *Compiler { return &Compiler{database: database} }

// Default deny: enrollment into a subnet never grants access to that subnet.
func (c *Compiler) CompileForDevice(ctx context.Context, deviceID string) (*CompilationResult, error) {
	var dev db.Device
	if err := c.database.WithContext(ctx).Preload("User.UserGroups").First(&dev, "id = ?", deviceID).Error; err != nil {
		return nil, err
	}
	if dev.IsRevoked {
		return nil, fmt.Errorf("device %s revoked", deviceID)
	}
	addr, err := netip.ParseAddr(dev.VirtualIP)
	if err != nil || !addr.Is4() {
		return nil, fmt.Errorf("invalid IPv4 device address: %s", dev.VirtualIP)
	}
	result := &CompilationResult{DeviceID: dev.ID, VirtualIP: addr.String()}
	allow := map[string]bool{}
	deny := map[string]bool{}
	subjectIDs := map[string]map[string]bool{"device": {dev.ID: true}}
	if dev.UserID != nil && dev.User != nil && dev.User.Status == "ACTIVE" {
		subjectIDs["user"] = map[string]bool{*dev.UserID: true}
		subjectIDs["user_group"] = map[string]bool{}
		for _, group := range dev.User.UserGroups {
			subjectIDs["user_group"][group.ID] = true
		}
	} else if dev.DeviceType == "user" {
		// Suspended or missing owner -> no grants, regardless of device-specific rules.
		return result, nil
	}
	var rules []db.AccessRule
	if err := c.database.WithContext(ctx).Find(&rules).Error; err != nil {
		return nil, err
	}
	for _, rule := range rules {
		if !subjectIDs[rule.SubjectType][rule.SubjectID] {
			continue
		}
		cidr, err := c.resolveTargetCIDR(ctx, rule.TargetType, rule.TargetValue)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", rule.ID, err)
		}
		switch rule.Action {
		case "ALLOW":
			allow[cidr] = true
		case "DENY":
			deny[cidr] = true
		default:
			return nil, fmt.Errorf("invalid policy action for %s", rule.ID)
		}
	}
	for cidr := range allow {
		result.AllowedSelectors = append(result.AllowedSelectors, cidr)
	}
	for cidr := range deny {
		result.DeniedSelectors = append(result.DeniedSelectors, cidr)
		result.DenyFilters = append(result.DenyFilters, fmt.Sprintf("nft add rule inet swanpanel forward ip saddr %s ip daddr %s drop", dev.VirtualIP, cidr))
	}
	sort.Strings(result.AllowedSelectors)
	sort.Strings(result.DeniedSelectors)
	sort.Strings(result.DenyFilters)
	return result, nil
}

func (c *Compiler) resolveTargetCIDR(ctx context.Context, kind, value string) (string, error) {
	if kind == "resource_group" {
		var group db.ResourceGroup
		if err := c.database.WithContext(ctx).Where("id = ?", value).First(&group).Error; err != nil {
			return "", err
		}
		value = group.CIDRBlock
	} else if kind != "cidr_ip" {
		return "", fmt.Errorf("invalid target type")
	}
	value = strings.TrimSpace(value)
	if addr, err := netip.ParseAddr(value); err == nil {
		if !addr.Is4() {
			return "", fmt.Errorf("IPv6 policy is not supported")
		}
		return netip.PrefixFrom(addr, 32).String(), nil
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil || !prefix.Addr().Is4() {
		return "", fmt.Errorf("invalid IPv4 CIDR %q", value)
	}
	return prefix.Masked().String(), nil
}
