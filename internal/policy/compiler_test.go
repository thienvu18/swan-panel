package policy_test

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
	"gorm.io/gorm"
)

func setupPolicyTestDB(t *testing.T) *gorm.DB {
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	if err := testDB.AutoMigrate(
		&db.User{},
		&db.UserGroup{},
		&db.ResourceGroup{},
		&db.Device{},
		&db.AccessRule{},
	); err != nil {
		t.Fatalf("Auto-migration failed: %v", err)
	}

	// 1. Create Resource Groups
	rgProd := db.ResourceGroup{ID: "rg_prod", Name: "Production VPC", CIDRBlock: "10.100.0.0/16"}
	rgEng := db.ResourceGroup{ID: "rg_eng", Name: "Engineering Subnet", CIDRBlock: "10.0.1.0/24"}
	testDB.Create(&rgProd)
	testDB.Create(&rgEng)

	// 2. Create User Group
	ugDevOps := db.UserGroup{ID: "ug_devops", Name: "DevOps"}
	testDB.Create(&ugDevOps)

	// 3. Create User with Group
	usr := db.User{
		ID:         "usr_thienvu",
		Email:      "thienvu@company.com",
		Name:       "Thien Vu",
		Status:     "ACTIVE",
		UserGroups: []db.UserGroup{ugDevOps},
	}
	testDB.Create(&usr)

	// 4. Create User Device
	dev := db.Device{
		ID:               "dev_thienvu",
		Hostname:         "thienvu-macbook",
		DeviceType:       "user",
		UserID:           &usr.ID,
		ResourceGroupID:  "rg_eng",
		IPAllocationType: "dynamic",
		VirtualIP:        "10.0.1.15",
		Platform:         "macos",
	}
	testDB.Create(&dev)

	// 5. Create Access Rules:
	// - Group Policy: DevOps -> ALLOW Production VPC
	ruleGroup := db.AccessRule{
		ID:          "rule_grp_prod",
		SubjectType: "user_group",
		SubjectID:   "ug_devops",
		Action:      "ALLOW",
		TargetType:  "resource_group",
		TargetValue: "rg_prod",
		RuleNature:  "group_policy",
	}
	testDB.Create(&ruleGroup)

	// - Direct Grant: Thien Vu -> ALLOW 10.50.0.0/16
	ruleDirect := db.AccessRule{
		ID:          "rule_direct_staging",
		SubjectType: "user",
		SubjectID:   "usr_thienvu",
		Action:      "ALLOW",
		TargetType:  "cidr_ip",
		TargetValue: "10.50.0.0/16",
		RuleNature:  "direct_grant",
	}
	testDB.Create(&ruleDirect)

	// - Deny Exclusion: Thien Vu -> DENY 10.50.99.0/24
	ruleDeny := db.AccessRule{
		ID:          "rule_deny_payment",
		SubjectType: "user",
		SubjectID:   "usr_thienvu",
		Action:      "DENY",
		TargetType:  "cidr_ip",
		TargetValue: "10.50.99.0/24",
		RuleNature:  "user_exclusion",
	}
	testDB.Create(&ruleDeny)

	return testDB
}

func TestPolicyCompiler(t *testing.T) {
	testDB := setupPolicyTestDB(t)
	compiler := policy.NewCompiler(testDB)
	ctx := context.Background()

	result, err := compiler.CompileForDevice(ctx, "dev_thienvu")
	if err != nil {
		t.Fatalf("Compilation failed: %v", err)
	}

	if result.VirtualIP != "10.0.1.15" {
		t.Errorf("Expected VirtualIP 10.0.1.15, got %s", result.VirtualIP)
	}

	// Verify Allowed Selectors contains Production VPC (10.100.0.0/16) and Staging (10.50.0.0/16)
	hasProd := false
	hasStaging := false
	for _, s := range result.AllowedSelectors {
		if s == "10.100.0.0/16" {
			hasProd = true
		}
		if s == "10.50.0.0/16" {
			hasStaging = true
		}
	}

	if !hasProd {
		t.Errorf("Expected allowed selectors to include Production VPC (10.100.0.0/16), got %v", result.AllowedSelectors)
	}
	if !hasStaging {
		t.Errorf("Expected allowed selectors to include Direct Grant (10.50.0.0/16), got %v", result.AllowedSelectors)
	}

	// Verify Deny Exclusion generated nftables drop filter
	if len(result.DenyFilters) == 0 {
		t.Fatalf("Expected deny filters, got 0")
	}

	dropRule := result.DenyFilters[0]
	if !strings.Contains(dropRule, "nft add rule") || !strings.Contains(dropRule, "10.50.99.0/24 drop") {
		t.Errorf("Expected nftables drop rule for 10.50.99.0/24, got: %s", dropRule)
	}
}
