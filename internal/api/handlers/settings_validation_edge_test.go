package handlers

import (
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
)

func validAdvancedFixture() db.StrongswanSettings {
	return db.StrongswanSettings{
		VPNName: "Employees VPN", CipherPreset: "modern", ReauthTime: "12h", RekeyTimeIKE: "4h",
		RekeyTimeChild: "1h", InactivityTimeout: "30m", DPDDelay: "30s", DPDTimeout: "120s",
		IKEProposals: "aes256gcm16-prfsha384-ecp384", ESPProposals: "aes256gcm16",
		Fragmentation: "yes",
	}
}

func TestAdvancedSettingsAllowOnlySafeFieldsAndWellTypedVICIValues(t *testing.T) {
	valid := validAdvancedFixture()
	valid.AdvancedConnection = map[string]any{"unique": "replace", "vips": []any{"10.0.0.5"}}
	valid.AdvancedChild = map[string]any{"copy_dscp": false, "start_action": []any{"none", "start"}}
	if err := validateAdvancedSettings(&valid); err != nil {
		t.Fatalf("valid advanced configuration rejected: %v", err)
	}
	for _, tc := range []struct {
		name       string
		value      any
		connection bool
	}{
		{"restricted connection auth", "illegal", true},
		{"mandatory Apple certificate delivery", "ifasked", true},
		{"restricted child local_ts", "illegal", false},
		{"invalid VICI type", map[string]any{"nested": "not supported"}, true},
		{"integer VICI value", 12, false},
		{"nil VICI value", nil, true},
		{"empty VICI list", []any{}, true},
		{"mixed type list", []any{"allowed", true}, false},
		{"newline in VICI value", "hello\nworld", true},
		{"oversized VICI value", strings.Repeat("a", 513), false},
		{"invalid VICI key name", "ok", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := validAdvancedFixture()
			key := "custom_setting"
			switch tc.name {
			case "restricted connection auth":
				key = "remote"
			case "mandatory Apple certificate delivery":
				key = "send_cert"
			case "restricted child local_ts":
				key = "local_ts"
			case "invalid VICI key name":
				key = "config.with.dots"
			}
			if tc.connection {
				value.AdvancedConnection = map[string]any{key: tc.value}
			} else {
				value.AdvancedChild = map[string]any{key: tc.value}
			}
			if err := validateAdvancedSettings(&value); err == nil {
				t.Fatalf("unsafe %s accepted", tc.name)
			}
		})
	}
}

func TestAdvancedSettingsRejectOversizedKeyCountsAndIncorrectDurations(t *testing.T) {
	invalid := validAdvancedFixture()
	invalid.AdvancedConnection = map[string]any{}
	for i := 0; i < 65; i++ {
		invalid.AdvancedConnection["option_"+strings.Repeat("a", i)+"b"] = "value"
	}
	if err := validateAdvancedSettings(&invalid); err == nil {
		t.Fatal("too many custom keys accepted")
	}
	for _, duration := range []string{"-1s", "99999999h", "30minutes", "30", "1.5h", "45ms", ""} {
		invalid = validAdvancedFixture()
		invalid.DPDDelay = duration
		if err := validateAdvancedSettings(&invalid); err == nil {
			t.Fatalf("invalid duration accepted: %q", duration)
		}
	}
	for _, value := range []int{-1, 499, 8961} {
		invalid = validAdvancedFixture()
		invalid.MSSClamping = value
		if err := validateAdvancedSettings(&invalid); err == nil {
			t.Fatalf("unsafe MSS %d accepted", value)
		}
	}
	for _, value := range []string{"script;start", "aes256\nunknown", ""} {
		invalid = validAdvancedFixture()
		invalid.IKEProposals = value
		if err := validateAdvancedSettings(&invalid); err == nil {
			t.Fatalf("unsafe proposal %q accepted", value)
		}
	}
}

func TestAdvancedSettingsPermitSafeDisplayNamesAndFormattingBoundaries(t *testing.T) {
	value := validAdvancedFixture()
	for _, name := range []string{"", "VPN", "Công ty VPN", "Corporate VPN with 128-character maximum"} {
		value.VPNName = name
		if err := validateAdvancedSettings(&value); err != nil {
			t.Fatalf("safe display name %q rejected: %v", name, err)
		}
	}
	value.VPNName = strings.Repeat("界", 129)
	if err := validateAdvancedSettings(&value); err == nil {
		t.Fatal("129-character Unicode name accepted")
	}
	value.VPNName = "VPN\rName"
	if err := validateAdvancedSettings(&value); err == nil {
		t.Fatal("multiline name accepted")
	}
	for _, fragmentation := range []string{"yes", "no", "force"} {
		value = validAdvancedFixture()
		value.Fragmentation = fragmentation
		if err := validateAdvancedSettings(&value); err != nil {
			t.Fatalf("valid fragmentation %q rejected: %v", fragmentation, err)
		}
	}
}
