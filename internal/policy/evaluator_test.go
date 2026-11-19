package policy

import "testing"

func TestEvaluateDestinationMatchesDenyBeforeAllowAndDefaultDeny(t *testing.T) {
	compiled := &CompilationResult{
		AllowedSelectors: []string{"10.99.0.0/16"},
		DeniedSelectors:  []string{"10.99.1.0/24"},
		RuleSources: map[string][]RuleSource{
			"ALLOW:10.99.0.0/16": {{ID: "allow-device", SubjectType: "device", RuleNature: "direct_grant"}},
			"DENY:10.99.1.0/24":  {{ID: "deny-group", SubjectType: "user_group", RuleNature: "user_exclusion"}},
		},
	}
	cases := []struct {
		address, action, reason, cidr string
	}{
		{"10.99.2.10", "ALLOW", "explicit_allow", "10.99.0.0/16"},
		{"10.99.1.10", "DENY", "explicit_deny", "10.99.1.0/24"},
		{"172.16.0.1", "DENY", "default_deny", ""},
	}
	for _, test := range cases {
		t.Run(test.address, func(t *testing.T) {
			actual, err := EvaluateDestination(compiled, test.address)
			if err != nil {
				t.Fatal(err)
			}
			if actual.Action != test.action || actual.Reason != test.reason || actual.MatchedCIDR != test.cidr {
				t.Fatalf("unexpected decision: %+v", actual)
			}
			if test.cidr != "" && (len(actual.RuleSources) != 1 || actual.RuleSources[0].ID == "") {
				t.Fatalf("missing contributing rule source: %+v", actual.RuleSources)
			}
			if test.cidr == "" && len(actual.RuleSources) != 0 {
				t.Fatalf("default deny must not imply a matched rule: %+v", actual.RuleSources)
			}
		})
	}
}

func TestEvaluateDestinationRejectsUnsafeOrUnsupportedInput(t *testing.T) {
	for _, address := range []string{"", "127.0.0.1/32", "2001:db8::1", "10.1.1.1; DROP"} {
		if _, err := EvaluateDestination(&CompilationResult{}, address); err == nil {
			t.Errorf("expected validation error for %q", address)
		}
	}
	if _, err := EvaluateDestination(nil, "10.0.0.1"); err == nil {
		t.Fatal("missing policy was accepted")
	}
}
