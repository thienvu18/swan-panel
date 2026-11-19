package policy

import (
	"fmt"
	"net/netip"
)

// DestinationDecision describes the effective configured policy for one IPv4
// destination. It is a prediction, not a packet-level connectivity test.
type DestinationDecision struct {
	Destination string       `json:"destination"`
	Action      string       `json:"action"`
	Reason      string       `json:"reason"`
	MatchedCIDR string       `json:"matched_cidr,omitempty"`
	RuleSources []RuleSource `json:"rule_sources,omitempty"`
}

// EvaluateDestination implements the same precedence as the firewall:
// explicit DENY before ALLOW, otherwise default DENY.
func EvaluateDestination(compiled *CompilationResult, destination string) (DestinationDecision, error) {
	if compiled == nil {
		return DestinationDecision{}, fmt.Errorf("missing device policy")
	}
	address, err := netip.ParseAddr(destination)
	if err != nil || !address.Is4() {
		return DestinationDecision{}, fmt.Errorf("destination must be an IPv4 address")
	}
	decision := DestinationDecision{Destination: address.String(), Action: "DENY", Reason: "default_deny"}
	for _, cidr := range compiled.DeniedSelectors {
		prefix, parseErr := netip.ParsePrefix(cidr)
		if parseErr != nil {
			return DestinationDecision{}, fmt.Errorf("invalid compiled DENY selector")
		}
		if prefix.Contains(address) {
			decision.Reason = "explicit_deny"
			decision.MatchedCIDR = cidr
			decision.RuleSources = compiled.RuleSources["DENY:"+cidr]
			return decision, nil
		}
	}
	for _, cidr := range compiled.AllowedSelectors {
		prefix, parseErr := netip.ParsePrefix(cidr)
		if parseErr != nil {
			return DestinationDecision{}, fmt.Errorf("invalid compiled ALLOW selector")
		}
		if prefix.Contains(address) {
			decision.Action = "ALLOW"
			decision.Reason = "explicit_allow"
			decision.MatchedCIDR = cidr
			decision.RuleSources = compiled.RuleSources["ALLOW:"+cidr]
			return decision, nil
		}
	}
	return decision, nil
}
