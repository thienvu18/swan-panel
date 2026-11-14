package policy_test

import (
	"context"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
)

func TestPolicyAuthorizationCases(t *testing.T) {
	cases := []struct {
		label    string
		prepare  func(*testing.T)
		expected []string
		denied   []string
		errors   bool
	}{
		{"group and user allow", nil, []string{"10.100.0.0/16", "10.50.0.0/16"}, []string{"10.50.99.0/24"}, false},
		{"disabled user denies all", func(t *testing.T) {
			t.Helper()
			if err := db.DB.Model(&db.User{}).Where("id = ?", "usr_thienvu").Update("status", "SUSPENDED").Error; err != nil {
				t.Fatal(err)
			}
		}, nil, nil, false},
		{"deny exact /32", func(t *testing.T) {
			t.Helper()
			err := db.DB.Create(&db.AccessRule{ID: "deny-host", SubjectType: "device", SubjectID: "dev_thienvu", Action: "DENY", TargetType: "cidr_ip", TargetValue: "10.100.3.2", RuleNature: "user_exclusion"}).Error
			if err != nil {
				t.Fatal(err)
			}
		}, []string{"10.100.0.0/16", "10.50.0.0/16"}, []string{"10.100.3.2/32", "10.50.99.0/24"}, false},
		{"bad target blocks compilation", func(t *testing.T) {
			t.Helper()
			err := db.DB.Create(&db.AccessRule{ID: "invalid-host", SubjectType: "device", SubjectID: "dev_thienvu", Action: "ALLOW", TargetType: "cidr_ip", TargetValue: "192.168.1.0/33", RuleNature: "direct_grant"}).Error
			if err != nil {
				t.Fatal(err)
			}
		}, nil, nil, true},
		{"unknown resource group blocks compilation", func(t *testing.T) {
			t.Helper()
			err := db.DB.Create(&db.AccessRule{ID: "missing-rg", SubjectType: "device", SubjectID: "dev_thienvu", Action: "ALLOW", TargetType: "resource_group", TargetValue: "does-not-exist", RuleNature: "direct_grant"}).Error
			if err != nil {
				t.Fatal(err)
			}
		}, nil, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			database := setupPolicyTestDB(t)
			old := db.DB
			db.DB = database
			defer func() { db.DB = old }()
			if tc.prepare != nil {
				tc.prepare(t)
			}
			result, err := policy.NewCompiler(database).CompileForDevice(context.Background(), "dev_thienvu")
			if tc.errors {
				if err == nil {
					t.Fatal("invalid policy produced rules")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			contains := func(set []string, want string) bool {
				for _, item := range set {
					if item == want {
						return true
					}
				}
				return false
			}
			if len(tc.expected) == 0 && len(result.AllowedSelectors) != 0 {
				t.Fatalf("expected no access, got %v", result.AllowedSelectors)
			}
			for _, want := range tc.expected {
				if !contains(result.AllowedSelectors, want) {
					t.Fatalf("missing allow %s: %v", want, result.AllowedSelectors)
				}
			}
			for _, want := range tc.denied {
				if !contains(result.DeniedSelectors, want) {
					t.Fatalf("missing deny %s: %v", want, result.DeniedSelectors)
				}
			}
		})
	}
}

func TestFirewallOnlyProcessesIPsecAndDeniesByDefault(t *testing.T) {
	database := setupPolicyTestDB(t)
	script, err := policy.NewFirewall(database).Script(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "meta ipsec exists") {
		t.Fatal("policy does not restrict itself to decapsulated IPsec packets")
	}
	if !strings.Contains(script, "add rule inet swanpanel forward meta ipsec exists drop") {
		t.Fatal("VPN forwarding has no explicit final DROP")
	}
	if !strings.Contains(script, "add rule inet swanpanel input meta ipsec exists drop") {
		t.Fatal("VPN traffic can access management input ports")
	}
	if strings.Contains(script, "ip saddr 0.0.0.0/0") {
		t.Fatal("IPsec policy unexpectedly permits wildcard source address")
	}
	denyIndex := strings.Index(script, "ip daddr 10.50.99.0/24 drop")
	allowIndex := strings.Index(script, "ip daddr 10.50.0.0/16 accept")
	if denyIndex < 0 || allowIndex < 0 || denyIndex > allowIndex {
		t.Fatal("specific DENY is not applied before broader ALLOW")
	}
}

func TestPolicyForRevokedDeviceReturnsError(t *testing.T) {
	database := setupPolicyTestDB(t)
	if err := database.Model(&db.Device{}).Where("id = ?", "dev_thienvu").Update("is_revoked", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := policy.NewCompiler(database).CompileForDevice(context.Background(), "dev_thienvu"); err == nil {
		t.Fatal("revoked identity compiled into positive policy")
	}
}

func TestGatewayDNSNarrowInputExceptions(t *testing.T) {
	t.Setenv("ENABLE_DNS", "true")
	t.Setenv("DNS_PORT", "53")
	database := setupPolicyTestDB(t)
	script, err := policy.NewFirewall(database).Script(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "meta ipsec exists ip saddr 10.0.1.15 udp dport 53 accept") {
		t.Fatal("authorized device is unable to query internal DNS")
	}
	if strings.Contains(script, "tcp dport 8080 accept") {
		t.Fatal("VPN users can reach control-plane management port")
	}
	if err := database.Model(&db.User{}).Where("id = ?", "usr_thienvu").Update("status", "SUSPENDED").Error; err != nil {
		t.Fatal(err)
	}
	script, err = policy.NewFirewall(database).Script(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, "meta ipsec exists ip saddr 10.0.1.15 udp dport 53 accept") {
		t.Fatal("suspended user retained gateway DNS access")
	}
}

func TestDisabledDNSEmitsNoException(t *testing.T) {
	t.Setenv("ENABLE_DNS", "false")
	database := setupPolicyTestDB(t)
	script, err := policy.NewFirewall(database).Script(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, "udp dport 53 accept") || strings.Contains(script, "tcp dport 53 accept") {
		t.Fatal("disabled DNS service is accessible over VPN")
	}
}
