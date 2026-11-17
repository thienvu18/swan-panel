package policy

import (
	"context"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDenyAllHostModeIsRestrictedToEnrolledVIPs(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&db.Device{}); err != nil {
		t.Fatal(err)
	}
	for _, device := range []db.Device{
		{ID: "active", Hostname: "active", VirtualIP: "10.122.0.2", IsRevoked: false},
		{ID: "revoked", Hostname: "revoked", VirtualIP: "10.122.0.3", IsRevoked: true},
	} {
		if err := database.Create(&device).Error; err != nil {
			t.Fatal(err)
		}
	}
	firewall := NewFirewall(database)
	t.Setenv("SWANPANEL_EXTERNAL_STRONGSWAN", "1")
	hostRules, err := firewall.denyAllScript(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"10.122.0.2", "10.122.0.3"} {
		for _, chain := range []string{"forward", "input"} {
			if !strings.Contains(hostRules, chain+" meta ipsec exists ip saddr "+ip+" drop") {
				t.Fatalf("fail-closed barrier missing %s %s", chain, ip)
			}
		}
	}
	if strings.Contains(hostRules, "meta ipsec exists drop\n") {
		t.Fatal("host fail-closed barrier blocks unrelated host VPNs")
	}
	t.Setenv("SWANPANEL_EXTERNAL_STRONGSWAN", "")
	bridgeRules, err := firewall.denyAllScript(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bridgeRules, "forward meta ipsec exists drop") {
		t.Fatal("default Docker bridge lost its global fail-closed barrier")
	}
}

func TestHostDefaultRouteEgress(t *testing.T) {
	for _, tc := range []struct {
		name, routes, want string
		fail               bool
	}{
		{"wifi", "default via 192.0.2.1 dev wlan0 proto dhcp metric 600\n", "wlan0", false},
		{"duplicate", "default via 10.0.0.1 dev eth0 metric 100\ndefault via 10.0.0.2 dev eth0 metric 200\n", "eth0", false},
		{"two interfaces", "default via 10.0.0.1 dev eth0\ndefault via 10.0.0.2 dev wlan0\n", "", true},
		{"no route", "192.0.2.0/24 dev eth0 scope link\n", "", true},
		{"unresolved", "default nhid 1\n", "", true},
		{"injection", "default via 10.0.0.1 dev x\";flush\n", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDefaultRouteEgress(tc.routes)
			if (err != nil) != tc.fail || (!tc.fail && got != tc.want) {
				t.Fatalf("got %q err %v, want %q fail %t", got, err, tc.want, tc.fail)
			}
		})
	}
}

func TestHostEgressExplicitOverrideValidation(t *testing.T) {
	t.Setenv("SWANPANEL_HOST_EGRESS_INTERFACE", "enp2s0")
	if got, err := hostEgressInterface(context.Background()); err != nil || got != "enp2s0" {
		t.Fatalf("unexpected explicit interface %q: %v", got, err)
	}
	for _, invalid := range []string{"wlan0\";flush ruleset", "eth 0", "0123456789012345", "../wlan0"} {
		t.Setenv("SWANPANEL_HOST_EGRESS_INTERFACE", invalid)
		if _, err := hostEgressInterface(context.Background()); err == nil {
			t.Fatalf("accepted unsafe interface: %q", invalid)
		}
	}
}

func TestExternalModeExplicitlyOptIn(t *testing.T) {
	t.Setenv("SWANPANEL_EXTERNAL_STRONGSWAN", "")
	if hostStrongSwanMode() {
		t.Fatal("default Docker gateway must not use host NAT")
	}
	t.Setenv("SWANPANEL_EXTERNAL_STRONGSWAN", "1")
	if !hostStrongSwanMode() {
		t.Fatal("explicit host mode not activated")
	}
	if out, err := parseDefaultRouteEgress("default dev enp2s0 scope link\n"); err != nil || strings.TrimSpace(out) != "enp2s0" {
		t.Fatalf("route fallback failed: %q %v", out, err)
	}
}
