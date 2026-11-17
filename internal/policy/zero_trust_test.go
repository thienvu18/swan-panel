package policy_test

import (
	"context"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
)

func TestHostFirewallScopesDropsAndMSSByManagedVIP(t *testing.T) {
	t.Setenv("SWANPANEL_EXTERNAL_STRONGSWAN", "1")
	t.Setenv("SWANPANEL_HOST_EGRESS_INTERFACE", "eth0")
	database := setupPolicyTestDB(t)
	if err := database.Create(&db.Device{
		ID: "revoked-vip", Hostname: "revoked-vip", DeviceType: "company",
		ResourceGroupID: "rg_eng", IPAllocationType: "static", VirtualIP: "10.0.1.99",
		Platform: "linux", IsRevoked: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	rules, err := policy.NewFirewall(database).Script(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{
		"forward meta ipsec exists ip saddr 10.0.1.15 drop",
		"input meta ipsec exists ip saddr 10.0.1.15 drop",
		"forward meta ipsec exists ip saddr 10.0.1.99 drop",
		"input meta ipsec exists ip saddr 10.0.1.99 drop",
		"forward meta ipsec exists ip saddr 10.0.1.15 tcp flags syn",
	} {
		if !strings.Contains(rules, rule) {
			t.Errorf("host firewall missing VIP-scoped rule %q", rule)
		}
	}
	if !strings.Contains(rules, "ip saddr 10.0.1.15 ip daddr 10.50.0.0/16 ct mark set ct mark | 0x40000000 accept") {
		t.Fatal("host-mode ALLOW did not stamp the authorized connection for Docker forwarding")
	}
	for _, unsafe := range []string{
		"forward meta ipsec exists drop\n",
		"input meta ipsec exists drop\n",
		"forward meta ipsec exists tcp flags syn",
		"ip saddr 10.0.1.0/24 drop",
	} {
		if strings.Contains(rules, unsafe) {
			t.Errorf("host policy can affect unrelated VPNs: %q", unsafe)
		}
	}
}

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

func TestHostNATOnlyCoversActiveVPNDevices(t *testing.T) {
	database := setupPolicyTestDB(t)
	ctx := context.Background()
	t.Setenv("SWANPANEL_HOST_EGRESS_INTERFACE", "wlan0")
	t.Setenv("SWANPANEL_EXTERNAL_STRONGSWAN", "")
	bridgeRules, err := policy.NewFirewall(database).Script(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(bridgeRules, "swanpanel_nat") {
		t.Fatal("default one-container bridge mode unexpectedly generated host NAT")
	}
	t.Setenv("SWANPANEL_EXTERNAL_STRONGSWAN", "1")
	rules, err := policy.NewFirewall(database).Script(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expected := "ip saddr 10.0.1.15 oifname \"wlan0\" counter masquerade"
	if !strings.Contains(rules, expected) {
		t.Fatalf("active VPN VIP missing restricted NAT: %s", rules)
	}
	if strings.Contains(rules, "oifname \"eth0\" masquerade") {
		t.Fatal("host NAT incorrectly inherited the default container egress")
	}
	if err := database.Model(&db.User{}).Where("id = ?", "usr_thienvu").Update("status", "SUSPENDED").Error; err != nil {
		t.Fatal(err)
	}
	rules, err = policy.NewFirewall(database).Script(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rules, "masquerade") {
		t.Fatal("suspended user retained an active VPN NAT grant")
	}
}
