package handlers

import (
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
)

func TestGatewayIdentifierValidation(t *testing.T) {
	cases := []struct {
		name  string
		valid bool
	}{
		{"vpn.example.com", true},
		{"vpn-company-01", true},
		{"203.0.113.12", true},
		{"2001:db8::1", true},
		{"vpn.example.com; rm -rf /", false},
		{"example\nINJECTED", false},
		{"bad..example", false},
		{"https://vpn.example.com", false},
		{"$ENV_EXPANSION", false},
		{"@vpn.example.com", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validGatewayIdentifier(tc.name); got != tc.valid {
				t.Fatalf("validGatewayIdentifier(%q)=%v want %v", tc.name, got, tc.valid)
			}
		})
	}
}

func TestGatewaySettingsRejectInjection(t *testing.T) {
	valid := db.StrongswanSettings{ServerEndpoint: "vpn.example.com", ServerIdentity: "vpn.example.com", DPDAction: "clear",
		IKEProposals: "aes256gcm16-prfsha384-ecp384", ESPProposals: "aes256gcm16"}
	if !validateGatewaySettings(&valid) {
		t.Fatal("valid gateway config was rejected")
	}
	invalidIdentity := valid
	invalidIdentity.ServerIdentity = "host\nSWAN_CONFIG\nrm -rf /"
	if validateGatewaySettings(&invalidIdentity) {
		t.Fatal("script injection in identity was accepted")
	}
	invalidProposal := valid
	invalidProposal.IKEProposals = "aes256gcm16\nremote { auth = psk }"
	if validateGatewaySettings(&invalidProposal) {
		t.Fatal("VICI proposal injection accepted")
	}
	invalidAction := valid
	invalidAction.DPDAction = "unsafe"
	if validateGatewaySettings(&invalidAction) {
		t.Fatal("unrecognized DPD action accepted")
	}
}

func TestGatewaySettingsAcceptLANEndpointAndSeparateCertificateIdentity(t *testing.T) {
	// The VPN client reaches the Mac on LAN using the endpoint IP, while
	// authenticating the strongSwan gateway against its certificate identity.
	settings := db.StrongswanSettings{
		ServerEndpoint:    "192.168.101.159",
		ServerIdentity:    "vpn.company.com",
		ReauthTime:        "12h",
		RekeyTimeIKE:      "4h",
		RekeyTimeChild:    "1h",
		InactivityTimeout: "30m",
		DPDDelay:          "30s",
		DPDTimeout:        "120s",
		DPDAction:         "clear",
		IKEProposals:      "aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048",
		ESPProposals:      "aes256gcm16,aes128gcm16,chacha20poly1305",
	}
	if !validateGatewaySettings(&settings) {
		t.Fatal("LAN IP endpoint with separate valid certificate identity must be accepted")
	}

	settings.ServerIdentity = "" // Mirrors the payload omitted by the old UI.
	if validateGatewaySettings(&settings) {
		t.Fatal("gateway identity must be provided explicitly")
	}
}
