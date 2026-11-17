package handlers

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/swan-panel/swan-panel/internal/db"
)

var advancedVICIKey = regexp.MustCompile("^[a-z][a-z0-9_]{0,63}$")
var durationPattern = regexp.MustCompile("^(?:0|[1-9][0-9]{0,6})(?:s|m|h|d|w)$")
var proposalPattern = regexp.MustCompile("^[a-zA-Z0-9_,+.-]+$")
var swanctlAdvancedValue = regexp.MustCompile("^[A-Za-z0-9_./:,+*%?@=-]+$")

// These keys are controlled by the zero-trust reconciler. Advanced VICI
// settings must never override peer authentication, identity, pools or selectors.
var managedConnectionKeys = map[string]bool{
	"version": true, "local": true, "remote": true, "children": true,
	"pools": true, "local_addrs": true, "remote_addrs": true,
	"reauth_time": true, "rekey_time": true, "proposals": true,
	"mobike": true, "encap": true, "fragmentation": true, "dpd_delay": true,
	"send_cert": true,
}
var managedChildKeys = map[string]bool{
	"remote_ts": true, "local_ts": true, "mode": true,
	"esp_proposals": true, "dpd_action": true, "rekey_time": true,
	"inactivity": true,
}

func validateAdvancedSection(scope string, options map[string]any, blocked map[string]bool) error {
	if len(options) > 64 {
		return fmt.Errorf("advanced_%s has too many keys (maximum 64)", scope)
	}
	for key, value := range options {
		if !advancedVICIKey.MatchString(key) || blocked[key] {
			return fmt.Errorf("advanced_%s key %q is invalid or reserved", scope, key)
		}
		checkString := func(s string) error {
			if len(s) > 512 || !swanctlAdvancedValue.MatchString(s) {
				return fmt.Errorf("advanced_%s.%s has an invalid value", scope, key)
			}
			return nil
		}
		switch v := value.(type) {
		case string:
			if err := checkString(v); err != nil {
				return err
			}
		case bool:
			// Booleans become yes/no strings when sent over VICI.
		case []any:
			if len(v) == 0 || len(v) > 32 {
				return fmt.Errorf("advanced_%s.%s has invalid list size", scope, key)
			}
			for _, entry := range v {
				str, ok := entry.(string)
				if !ok {
					return fmt.Errorf("advanced_%s.%s list values must be strings", scope, key)
				}
				if err := checkString(str); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("advanced_%s.%s must be a string, boolean or string array", scope, key)
		}
	}
	return nil
}

func validateAdvancedSettings(settings *db.StrongswanSettings) error {
	if utf8.RuneCountInString(strings.TrimSpace(settings.VPNName)) > 128 ||
		strings.ContainsAny(settings.VPNName, "\x00\r\n") {
		return fmt.Errorf("vpn_name must be a single-line display name (maximum 128 characters)")
	}
	if settings.CipherPreset != "modern" && settings.CipherPreset != "balanced" && settings.CipherPreset != "custom" {
		return fmt.Errorf("cipher_preset must be modern, balanced or custom")
	}
	for _, duration := range []string{
		settings.ReauthTime, settings.RekeyTimeIKE, settings.RekeyTimeChild,
		settings.InactivityTimeout, settings.DPDDelay, settings.DPDTimeout,
	} {
		if !durationPattern.MatchString(duration) {
			return fmt.Errorf("invalid duration %q", duration)
		}
	}
	if !proposalPattern.MatchString(settings.IKEProposals) || !proposalPattern.MatchString(settings.ESPProposals) {
		return fmt.Errorf("invalid IKE or ESP proposals")
	}
	if settings.Fragmentation != "yes" && settings.Fragmentation != "no" && settings.Fragmentation != "force" {
		return fmt.Errorf("fragmentation must be yes, no or force")
	}
	if settings.MSSClamping < 0 || settings.MSSClamping > 8960 ||
		(settings.MSSClamping != 0 && settings.MSSClamping < 500) {
		return fmt.Errorf("mss_clamping must be disabled (0) or between 500 and 8960")
	}
	if err := validateAdvancedSection("connection", settings.AdvancedConnection, managedConnectionKeys); err != nil {
		return err
	}
	if err := validateAdvancedSection("child", settings.AdvancedChild, managedChildKeys); err != nil {
		return err
	}
	return nil
}
