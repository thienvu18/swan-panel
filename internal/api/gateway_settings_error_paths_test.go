package api_test

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
)

func gatewaySettingsPayload(t *testing.T, body string) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestGatewaySettingsPersistAndApplyValidChangesAndIgnoreUnsupportedDPDTimeout(t *testing.T) {
	router, closeDB := setupTestRouter(t)
	defer closeDB()
	token := getAdminToken(t, router)
	original := gatewaySettingsPayload(t, mustStatus(t, router, http.MethodGet, "/api/settings/strongswan", token, nil, 200))
	updated := gatewaySettingsPayload(t, mustStatus(t, router, http.MethodGet, "/api/settings/strongswan", token, nil, 200))
	updated["vpn_name"] = "Engineering VPN"
	updated["dpd_delay"] = "45s"
	updated["server_endpoint"] = "edge.gateway.example.test"
	updated["advanced_child"] = map[string]any{"start_action": "none"}
	response := mustStatus(t, router, http.MethodPut, "/api/settings/strongswan", token, updated, 200)
	if !strings.Contains(response, "applied") {
		t.Fatalf("valid config failed to apply: %s", response)
	}
	certificatePEM, err := os.ReadFile(filepath.Join(os.Getenv("PKI_DATA_DIR"), "server.crt"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certificatePEM)
	if block == nil {
		t.Fatal("gateway certificate is not PEM")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil || certificate.VerifyHostname("edge.gateway.example.test") != nil {
		t.Fatalf("saved gateway endpoint is not in certificate SAN: %v", err)
	}
	stored := gatewaySettingsPayload(t, mustStatus(t, router, http.MethodGet, "/api/settings/strongswan", token, nil, 200))
	if stored["vpn_name"] != "Engineering VPN" || stored["dpd_delay"] != "45s" || stored["dpd_timeout"] != original["dpd_timeout"] {
		t.Fatalf("persisted config does not match: %+v", stored)
	}
	updated["dpd_timeout"] = "125s"
	mustStatus(t, router, http.MethodPut, "/api/settings/strongswan", token, updated, 400)
	updated["dpd_timeout"] = ""
	mustStatus(t, router, http.MethodPut, "/api/settings/strongswan", token, updated, 200)
	after := gatewaySettingsPayload(t, mustStatus(t, router, http.MethodGet, "/api/settings/strongswan", token, nil, 200))
	if after["dpd_timeout"] != original["dpd_timeout"] {
		t.Fatal("empty unsupported DPD timeout overwrote stored value")
	}
}

func TestGatewaySettingsReportPendingWhenCertificateOrRevocationListCannotDeploy(t *testing.T) {
	router, closeDB := setupTestRouter(t)
	defer closeDB()
	token := getAdminToken(t, router)
	input := gatewaySettingsPayload(t, mustStatus(t, router, http.MethodGet, "/api/settings/strongswan", token, nil, 200))
	// Prepare a valid persisted server identity and then simulate corrupted key material.
	mustStatus(t, router, http.MethodPut, "/api/settings/strongswan", token, input, 200)
	keyPath := filepath.Join(os.Getenv("PKI_DATA_DIR"), "server.key")
	originalKey, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("corrupted private key"), 0600); err != nil {
		t.Fatal(err)
	}
	input["vpn_name"] = "Updated while signer unavailable"
	pending := mustStatus(t, router, http.MethodPut, "/api/settings/strongswan", token, input, 202)
	if !strings.Contains(pending, "pending") {
		t.Fatal("failed certificate silently marked applied")
	}
	if err := os.WriteFile(keyPath, originalKey, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.WriteFile(keyPath, originalKey, 0600) })

	// A broken durable revocation ledger prevents declaring the gateway ready.
	if err := db.DB.Create(&db.CertificateRevocation{Serial: "invalid-hex", DeviceID: "old-client"}).Error; err != nil {
		t.Fatal(err)
	}
	pending = mustStatus(t, router, http.MethodPut, "/api/settings/strongswan", token, input, 202)
	if !strings.Contains(pending, "CRL") {
		t.Fatalf("CRL publishing failure was hidden: %s", pending)
	}
	if err := db.DB.Where("serial = ?", "invalid-hex").Delete(&db.CertificateRevocation{}).Error; err != nil {
		t.Fatal(err)
	}
	// Invalid active device VIP makes policy reconciliation fail closed.
	group := db.ResourceGroup{ID: "bad-vip-group", Name: "Broken runtime subnet", CIDRBlock: "10.234.1.0/24"}
	if err := db.DB.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	device := db.Device{ID: "bad-vip-device", Hostname: "bad-vip-client", DeviceType: "company",
		ResourceGroupID: group.ID, IPAllocationType: "static", VirtualIP: "not-an-ip", Platform: "linux"}
	if err := db.DB.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	pending = mustStatus(t, router, http.MethodPut, "/api/settings/strongswan", token, input, 202)
	if !strings.Contains(pending, "gateway") {
		t.Fatalf("policy apply failure was hidden: %s", pending)
	}
}

func TestGatewaySettingsRejectMalformedPayloadsBeforeMutatingDatabase(t *testing.T) {
	router, closeDB := setupTestRouter(t)
	defer closeDB()
	token := getAdminToken(t, router)
	input := gatewaySettingsPayload(t, mustStatus(t, router, http.MethodGet, "/api/settings/strongswan", token, nil, 200))
	input["server_endpoint"] = "invalid domain/name"
	mustStatus(t, router, http.MethodPut, "/api/settings/strongswan", token, input, 400)
	input["server_endpoint"] = "gateway.example.test"
	input["server_identity"] = "bad;injected"
	mustStatus(t, router, http.MethodPut, "/api/settings/strongswan", token, input, 400)
	input["server_identity"] = "gateway.example.test"
	input["unknown_field"] = "malicious"
	mustStatus(t, router, http.MethodPut, "/api/settings/strongswan", token, input, 400)
	saved := gatewaySettingsPayload(t, mustStatus(t, router, http.MethodGet, "/api/settings/strongswan", token, nil, 200))
	if saved["server_endpoint"] == "gateway.example.test" {
		t.Fatal("rejected request modified active configuration")
	}
}

func TestGatewaySettingsRejectsConcatenatedJSONRequests(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	payload := mustStatus(t, router, "GET", "/api/settings/strongswan", token, nil, 200)
	req, err := http.NewRequest(http.MethodPut, "/api/settings/strongswan", strings.NewReader(payload+"\n{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != 400 {
		t.Fatalf("trailing JSON document accepted: %d %s", recorder.Code, recorder.Body.String())
	}
}
