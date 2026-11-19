package handlers

import "testing"

func TestDNSOverrideTargetValidation(t *testing.T) {
	cases := []struct {
		name       string
		recordType string
		target     string
		valid      bool
	}{
		{"A with private IPv4", "A", "192.168.1.100", true},
		{"A with a hostname", "A", "gateway.enterprise.corp", false},
		{"A with an oversized octet", "A", "256.168.1.100", false},
		{"A with IPv6", "A", "2001:db8::1", false},
		{"CNAME with a hostname", "CNAME", "gateway.enterprise.corp", true},
		{"CNAME with a trailing dot", "CNAME", "gateway.enterprise.corp.", true},
		{"CNAME with an IPv4 literal", "CNAME", "192.168.1.100", false},
		{"CNAME with an empty label", "CNAME", "gateway..corp", false},
		{"CNAME with a single label", "CNAME", "gateway", false},
		{"CNAME with a leading hyphen", "CNAME", "-gateway.enterprise.corp", false},
		{"Existing AAAA API record", "AAAA", "2001:db8::1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := isValidDNSOverrideTarget(tc.recordType, tc.target); actual != tc.valid {
				t.Fatalf("isValidDNSOverrideTarget(%q, %q) = %t; expected %t", tc.recordType, tc.target, actual, tc.valid)
			}
		})
	}
}
