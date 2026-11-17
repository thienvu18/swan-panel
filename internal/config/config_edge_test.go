package config

import (
	"reflect"
	"testing"
)

func TestConfigDefaultsAreSafeForDevelopment(t *testing.T) {
	for _, key := range []string{"PORT", "DNS_PORT", "ENABLE_DNS", "DB_PATH", "APP_ENV", "ENVIRONMENT", "JWT_SECRET", "VICI_SOCKET", "LOG_LEVEL", "ALLOWED_ORIGINS", "PKI_DATA_DIR"} {
		t.Setenv(key, "")
	}
	got := Load()
	if got.Port != 8080 || got.DNSPort != 5353 || !got.EnableDNS ||
		got.DBPath != "swanpanel.db" || got.Environment != "development" ||
		got.JWTSecret == "" || got.LogLevel != "info" ||
		got.VICISocket != "/var/run/charon.vici" || got.PKIDataDir != "/var/lib/swan-panel" ||
		len(got.AllowedOrigins) != 0 {
		t.Fatalf("unsafe or unexpected development defaults: %+v", got)
	}
}

func TestConfigParsesEnvironmentAndFallsBackOnMalformedInputs(t *testing.T) {
	t.Setenv("PORT", "43102")
	t.Setenv("DNS_PORT", "153")
	t.Setenv("ENABLE_DNS", "false")
	t.Setenv("DB_PATH", "/tmp/test.db")
	t.Setenv("APP_ENV", "")
	t.Setenv("ENVIRONMENT", "testing")
	t.Setenv("JWT_SECRET", "provided-secret")
	t.Setenv("VICI_SOCKET", "/tmp/custom.vici")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("ALLOWED_ORIGINS", " https://one.example, ,https://two.example ,, ")
	t.Setenv("PKI_DATA_DIR", "/tmp/pki")
	got := Load()
	if got.Port != 43102 || got.DNSPort != 153 || got.EnableDNS || got.DBPath != "/tmp/test.db" ||
		got.Environment != "testing" || got.JWTSecret != "provided-secret" ||
		got.VICISocket != "/tmp/custom.vici" || got.LogLevel != "debug" ||
		got.PKIDataDir != "/tmp/pki" || !reflect.DeepEqual(got.AllowedOrigins, []string{"https://one.example", "https://two.example"}) {
		t.Fatalf("environment values were not propagated: %+v", got)
	}
	t.Setenv("APP_ENV", "test")
	t.Setenv("PORT", "not-an-int")
	t.Setenv("DNS_PORT", "abc")
	t.Setenv("ENABLE_DNS", "not-a-bool")
	got = Load()
	if got.Environment != "test" || got.Port != 8080 || got.DNSPort != 5353 || !got.EnableDNS {
		t.Fatalf("invalid values were not defaulted: %+v", got)
	}
	t.Setenv("PORT", "0")
	t.Setenv("DNS_PORT", "-12")
	t.Setenv("ENABLE_DNS", "1")
	got = Load()
	if got.Port != 0 || got.DNSPort != -12 || !got.EnableDNS {
		t.Fatalf("integer/bool configuration does not match documented parsing: %+v", got)
	}
}

func TestConfigOriginParserDropsEmptyEntriesAndPreservesOrder(t *testing.T) {
	for _, tc := range []struct {
		in  string
		out []string
	}{
		{"", nil},
		{" , ,", nil},
		{"a,b, c", []string{"a", "b", "c"}},
		{"https://one,https://one", []string{"https://one", "https://one"}},
	} {
		if got := parseAllowedOrigins(tc.in); !reflect.DeepEqual(got, tc.out) {
			t.Errorf("parseAllowedOrigins(%q)=%v; want %v", tc.in, got, tc.out)
		}
	}
}
