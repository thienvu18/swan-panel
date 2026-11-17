package pki

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
)

func TestClientProfileBuildersRejectMissingOrMalformedCertificateMaterials(t *testing.T) {
	tests := []struct {
		name string
		run  func(*CertificateBundle) error
	}{
		{"Apple", func(b *CertificateBundle) error {
			_, err := BuildAppleProfileNamed("vpn.example", "vpn.example", "user", "VPN", b)
			return err
		}},
		{"Windows", func(b *CertificateBundle) error {
			_, err := BuildWindowsInstallerWithRoutesNamed("vpn.example", "user", "VPN", b, nil)
			return err
		}},
		{"Android", func(b *CertificateBundle) error {
			_, err := BuildAndroidProfileNamed("vpn.example", "vpn.example", "user", "VPN", b, nil)
			return err
		}},
		{"Native Android", func(b *CertificateBundle) error {
			_, err := BuildAndroidNativeSetup("vpn.example", "vpn.example", "user", b)
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(nil); err == nil {
				t.Fatal("nil identity accepted")
			}
			if err := tc.run(&CertificateBundle{CACertPEM: "not-pem", PrivateKeyPEM: "bad"}); err == nil {
				t.Fatal("malformed CA accepted")
			}
		})
	}
	manager := freshTestCA(t)
	bundle, err := manager.GenerateClientCertificate("client", "client")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range tests[:3] {
		t.Run(tc.name+"-bad-private-key", func(t *testing.T) {
			corrupted := *bundle
			corrupted.PrivateKeyPEM = "bad-key"
			if err := tc.run(&corrupted); err == nil {
				t.Fatal("corrupt private key accepted")
			}
		})
	}
}
func TestAppleProfileNamedEscapesUserFacingVPNNameAndDefaultsEmptyName(t *testing.T) {
	manager := freshTestCA(t)
	bundle, err := manager.GenerateClientCertificate("client", "client")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := BuildAppleProfileNamed("vpn.example", "vpn.example", "client", "R&D <VPN> \"Staff\" & Guests", bundle)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(profile, "R&amp;D &lt;VPN&gt;") {
		t.Fatal("VPN display name not XML-escaped")
	}
	if strings.Contains(profile, "<VPN>") {
		t.Fatal("unescaped VPN XML element injected")
	}
	decoder := xml.NewDecoder(strings.NewReader(profile))
	for {
		_, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("Apple profile is not valid XML: %v", err)
		}
	}
	fallback, err := BuildAppleProfileNamed("vpn.example", "vpn.example", "client", "", bundle)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fallback, "SwanPanel VPN") {
		t.Fatal("empty display name not defaulted")
	}
}

func TestWindowsProfileNamedEscapesPowerShellAndFiltersInvalidRoutingTargets(t *testing.T) {
	manager := freshTestCA(t)
	bundle, err := manager.GenerateClientCertificate("client", "client")
	if err != nil {
		t.Fatal(err)
	}
	body, err := BuildWindowsInstallerWithRoutesNamed("vpn'gateway", "user'host", "Team's VPN", bundle,
		[]string{"10.30.0.0/16", "not-a-prefix", "10.50.0.1/32"})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"Team''s VPN", "vpn''gateway", "user''host",
		"10.30.0.0/16", "10.50.0.1/32", "Add-VpnConnectionRoute",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("missing safe Windows profile value %q", fragment)
		}
	}
	if strings.Contains(body, "not-a-prefix") {
		t.Fatal("invalid route emitted as PowerShell")
	}
}

func TestAndroidNamedProfilesPreserveIdentityAndOptionalSplitRoutes(t *testing.T) {
	manager := freshTestCA(t)
	bundle, err := manager.GenerateClientCertificate("client", "client")
	if err != nil {
		t.Fatal(err)
	}
	for _, routes := range [][]string{nil, {"10.1.0.0/16"}} {
		body, err := BuildAndroidProfileNamed("vpn.example", "gateway.example", "client", "Engineering VPN", bundle, routes)
		if err != nil {
			t.Fatal(err)
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(body), &parsed); err != nil {
			t.Fatal(err)
		}
		if parsed["name"] != "Engineering VPN (client)" {
			t.Fatalf("VPN name missing: %+v", parsed)
		}
		remote, ok := parsed["remote"].(map[string]any)
		if !ok || remote["id"] != "gateway.example" {
			t.Fatalf("incorrect server certificate identity: %+v", parsed)
		}
		_, split := parsed["split-tunneling"]
		if split != (len(routes) > 0) {
			t.Fatalf("split routing incorrectly serialized: %+v", parsed)
		}
	}
}

func TestLinuxProfilesValidateHostnamesRoutesAndNeverHijackDefaultRouteWithoutGrants(t *testing.T) {
	manager := freshTestCA(t)
	bundle, err := manager.GenerateClientCertificate("client", "client")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildLinuxInstaller("vpn.example", "gateway.example", "client", nil, nil); err == nil {
		t.Fatal("Linux installer accepted missing private key")
	}
	if _, err := BuildLinuxInstaller("vpn.example", "gateway.example", "unsafe;rm", bundle, nil); err == nil {
		t.Fatal("unsafe shell hostname accepted")
	}
	if _, err := BuildLinuxInstaller("vpn.example", "gateway.example", "client", bundle, []string{"10.0.0.0/99"}); err == nil {
		t.Fatal("invalid CIDR emitted in Linux installer")
	}
	if _, err := BuildLinuxConfig("", "", "", nil); err == nil {
		t.Fatal("empty Linux identity accepted")
	}
	if _, err := BuildLinuxConfig("vpn.example", "gateway.example", "unsafe;rm", nil); err == nil {
		t.Fatal("unsafe Linux hostname accepted")
	}
	if _, err := BuildLinuxConfig("vpn.example", "gateway.example", "client", []string{"bad"}); err == nil {
		t.Fatal("invalid Linux traffic selector accepted")
	}
	body, err := BuildLinuxInstaller("vpn.example", "gateway.example", "client", bundle, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "remote_ts = 0.0.0.0/32") || strings.Contains(body, "remote_ts = 0.0.0.0/0") {
		t.Fatal("client installer grants unapproved default route")
	}
	config, err := BuildLinuxConfig("vpn.example", "gateway.example", "client", []string{"10.10.0.0/16"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(config, "remote_ts = 10.10.0.0/16") {
		t.Fatal("explicit access selector not exported")
	}
}

func TestPKCS12RejectsInvalidIdentityAndCAInputs(t *testing.T) {
	manager := freshTestCA(t)
	bundle, err := manager.GenerateClientCertificate("client", "client")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GeneratePKCS12(nil, "x", true); err == nil {
		t.Fatal("nil P12 input accepted")
	}
	cases := []struct {
		name   string
		change func(*CertificateBundle)
	}{
		{"bad certificate", func(b *CertificateBundle) { b.CertPEM = "invalid" }},
		{"bad private key", func(b *CertificateBundle) { b.PrivateKeyPEM = "invalid" }},
		{"bad CA", func(b *CertificateBundle) { b.CACertPEM = "invalid" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := *bundle
			tc.change(&data)
			if _, err := GeneratePKCS12(&data, "secret", true); err == nil {
				t.Fatal("invalid PKCS12 credentials accepted")
			}
		})
	}
}
