package handlers

import (
	"context"

	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

// listAccessRuleDTOs loads names in batches rather than running one query per
// rule. It preserves historical fallback to stored IDs for missing references.
func listAccessRuleDTOs(ctx context.Context, database *gorm.DB) ([]RuleDTO, error) {
	var rules []db.AccessRule
	if err := database.WithContext(ctx).Order("created_at asc").Find(&rules).Error; err != nil {
		return nil, err
	}

	groups, users, devices, resources := map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}
	groupIDs, userIDs, deviceIDs, resourceIDs := []string{}, []string{}, []string{}, []string{}
	for _, rule := range rules {
		switch rule.SubjectType {
		case "user_group":
			groupIDs = append(groupIDs, rule.SubjectID)
		case "user":
			userIDs = append(userIDs, rule.SubjectID)
		case "device":
			deviceIDs = append(deviceIDs, rule.SubjectID)
		}
		if rule.TargetType == "resource_group" {
			resourceIDs = append(resourceIDs, rule.TargetValue)
		}
	}

	if len(groupIDs) > 0 {
		var rows []db.UserGroup
		if err := database.WithContext(ctx).Select("id", "name").Where("id IN ?", groupIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			groups[row.ID] = row.Name
		}
	}
	if len(userIDs) > 0 {
		var rows []db.User
		if err := database.WithContext(ctx).Select("id", "name").Where("id IN ?", userIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			users[row.ID] = row.Name
		}
	}
	if len(deviceIDs) > 0 {
		var rows []db.Device
		if err := database.WithContext(ctx).Select("id", "hostname").Where("id IN ?", deviceIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			devices[row.ID] = row.Hostname
		}
	}
	if len(resourceIDs) > 0 {
		var rows []db.ResourceGroup
		if err := database.WithContext(ctx).Where("id IN ?", resourceIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			resources[row.ID] = row.Name + " (" + row.CIDRBlock + ")"
		}
	}

	result := make([]RuleDTO, 0, len(rules))
	for _, rule := range rules {
		subject := rule.SubjectID
		switch rule.SubjectType {
		case "user_group":
			if label := groups[rule.SubjectID]; label != "" {
				subject = label
			}
		case "user":
			if label := users[rule.SubjectID]; label != "" {
				subject = label
			}
		case "device":
			if label := devices[rule.SubjectID]; label != "" {
				subject = label
			}
		}
		target := rule.TargetValue
		if rule.TargetType == "resource_group" && resources[rule.TargetValue] != "" {
			target = resources[rule.TargetValue]
		}
		result = append(result, RuleDTO{
			ID: rule.ID, SubjectType: rule.SubjectType, SubjectID: rule.SubjectID,
			SubjectName: subject, Action: rule.Action, TargetType: rule.TargetType,
			TargetValue: rule.TargetValue, TargetName: target, RuleNature: rule.RuleNature,
		})
	}
	return result, nil
}
