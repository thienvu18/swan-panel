package policy_test

import (
	"context"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
)

func TestDefaultDenyWithoutExplicitGrant(t *testing.T) {
	database := setupPolicyTestDB(t)
	orphan := db.Device{
		ID: "dev_no_grants", Hostname: "no-grants", DeviceType: "company",
		ResourceGroupID: "rg_eng", IPAllocationType: "static", VirtualIP: "10.0.1.25",
		Platform: "linux",
	}
	if err := database.Create(&orphan).Error; err != nil {
		t.Fatal(err)
	}
	result, err := policy.NewCompiler(database).CompileForDevice(context.Background(), orphan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AllowedSelectors) != 0 {
		t.Fatalf("Unassigned node inherited access from its IP pool: %v", result.AllowedSelectors)
	}
	rules, err := policy.NewFirewall(database).Script(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rules, "add rule inet swanpanel forward meta ipsec exists drop") {
		t.Fatal("No kernel default-deny rule for IPsec forwarding")
	}
	denyAt := strings.Index(rules, "ip daddr 10.50.99.0/24 drop")
	allowAt := strings.Index(rules, "ip daddr 10.50.0.0/16 accept")
	if denyAt < 0 || allowAt < 0 || denyAt > allowAt {
		t.Fatal("DENY rules must take precedence over broader ALLOW rules")
	}
}

func TestRevokedDeviceCannotCompileGrants(t *testing.T) {
	database := setupPolicyTestDB(t)
	if err := database.Model(&db.Device{}).Where("id = ?", "dev_thienvu").Update("is_revoked", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := policy.NewCompiler(database).CompileForDevice(context.Background(), "dev_thienvu"); err == nil {
		t.Fatal("Revoked device compiled into an active policy")
	}
}
