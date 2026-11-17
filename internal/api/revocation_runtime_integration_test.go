package api_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/handlers"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/pki"
	"github.com/swan-panel/swan-panel/internal/vici"
)

// Commands run against isolated stand-ins, not the host firewall/daemon.
// APP_ENV is non-test so certificate revocation exercises all runtime paths.
func simulatedRevocationEnvironment(t *testing.T, failNft bool, rejectCommand ...string) {
	t.Helper()
	bin := t.TempDir()
	status := "exit 0"
	if failNft {
		status = "exit 3"
	}
	script := "#!/bin/sh\nif [ \"$1\" = \"list\" ]; then exit 1; fi\n" + status + "\n"
	if err := os.WriteFile(filepath.Join(bin, "nft"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "swanctl"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("APP_ENV", "ci-integration")
	t.Setenv("SWANPANEL_FIREWALL_DRY_RUN", "")
	t.Setenv("SWANPANEL_SWANCTL_CRL_DIR", t.TempDir())
	t.Setenv("SWANPANEL_SWANCTL_FILE", filepath.Join(t.TempDir(), "managed.conf"))
	socket := startLiveVICIStub(t, rejectCommand...)
	t.Setenv("VICI_SOCKET", socket)
	previous := vici.GetClient("")
	vici.DefaultClient = vici.NewClient(socket)
	t.Cleanup(func() { vici.DefaultClient = previous })
}

func TestRevokingAnActiveCertificatePublishesCRLAndTerminatesLiveVICI(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name": "Client revocation test subnet", "cidr_block": "10.229.0.0/24"})
	if group.Code != 201 {
		t.Fatal(group.Body.String())
	}
	device := requestJSON(t, router, "POST", "/api/devices", admin, map[string]any{
		"hostname": "my-client", "device_type": "company", "resource_group_id": responseID(t, group, "resource_group")})
	if device.Code != 201 {
		t.Fatal(device.Body.String())
	}
	id := responseID(t, device, "device")
	simulatedRevocationEnvironment(t, false)
	revoked := requestJSON(t, router, "POST", "/api/devices/"+id+"/revoke", admin, nil)
	if revoked.Code != 200 {
		t.Fatalf("live revocation failed: %d %s", revoked.Code, revoked.Body.String())
	}
	var stored db.Device
	if err := db.DB.First(&stored, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.IsRevoked {
		t.Fatal("identity not disabled in durable database")
	}
	var ledger db.CertificateRevocation
	if err := db.DB.First(&ledger, "serial = ?", stored.CertSerial).Error; err != nil {
		t.Fatal(err)
	}
	crl := filepath.Join(os.Getenv("SWANPANEL_SWANCTL_CRL_DIR"), "swanpanel.crl")
	content, err := os.ReadFile(crl)
	if err != nil || len(content) < 50 {
		t.Fatalf("signed CRL not deployed: %v", err)
	}
	manager, err := pki.GetPKIManager()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.LoadDeviceBundle(id); err == nil {
		t.Fatal("revoked private key still exportable")
	}
	resp := requestJSON(t, router, "GET", "/api/devices/"+id+"/config/macos", admin, nil)
	if resp.Code != 403 {
		t.Fatalf("revoked identity downloaded: %d", resp.Code)
	}
}

func TestRevocationDefersBusyVICIPoolWithoutReenablingDevice(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name": "Busy pool revocation subnet", "cidr_block": "10.237.0.0/24"})
	if group.Code != http.StatusCreated {
		t.Fatal(group.Body.String())
	}
	device := requestJSON(t, router, "POST", "/api/devices", admin, map[string]any{
		"hostname": "my-client", "device_type": "company", "resource_group_id": responseID(t, group, "resource_group")})
	if device.Code != http.StatusCreated {
		t.Fatal(device.Body.String())
	}
	id := responseID(t, device, "device")
	simulatedRevocationEnvironment(t, false, "unload-pool")
	response := requestJSON(t, router, "POST", "/api/devices/"+id+"/revoke", admin, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("busy VICI lease delayed secure certificate revocation: %d %s", response.Code, response.Body.String())
	}
	var revoked db.Device
	if err := db.DB.First(&revoked, "id = ?", id).Error; err != nil || !revoked.IsRevoked {
		t.Fatalf("revocation not durable after pool unload rejection: %+v %v", revoked, err)
	}
	mustStatus(t, router, "GET", "/api/devices/"+id+"/config/android-native", admin, nil, http.StatusForbidden)
}

func TestRevocationFailureStillCommitsLedgerAndReportsPending(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name": "Revocation failure subnet", "cidr_block": "10.228.0.0/24"})
	if group.Code != 201 {
		t.Fatal(group.Body.String())
	}
	device := requestJSON(t, router, "POST", "/api/devices", admin, map[string]any{
		"hostname": "my-client", "device_type": "company", "resource_group_id": responseID(t, group, "resource_group")})
	if device.Code != 201 {
		t.Fatal(device.Body.String())
	}
	id := responseID(t, device, "device")
	simulatedRevocationEnvironment(t, true)
	response := requestJSON(t, router, "POST", "/api/devices/"+id+"/revoke", admin, nil)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "revocation_pending") {
		t.Fatalf("failed kernel enforcement must be reported pending: %d %s", response.Code, response.Body.String())
	}
	var stored db.Device
	if err := db.DB.First(&stored, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.IsRevoked {
		t.Fatal("failed firewall reverted durable revocation")
	}
	var ledger db.CertificateRevocation
	if err := db.DB.First(&ledger, "serial = ?", stored.CertSerial).Error; err != nil {
		t.Fatal(err)
	}
}

func TestLegacyCertificateWithoutSerialIsNeverMarkedRevoked(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name": "Legacy serial-less subnet", "cidr_block": "10.227.0.0/24"})
	if group.Code != 201 {
		t.Fatal(group.Body.String())
	}
	dev := db.Device{ID: "without-serial", Hostname: "no-serial", DeviceType: "company",
		ResourceGroupID: responseID(t, group, "resource_group"), IPAllocationType: "static", VirtualIP: "10.227.0.3", Platform: "linux"}
	if err := db.DB.Create(&dev).Error; err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, router, "POST", "/api/devices/"+dev.ID+"/revoke", admin, nil)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing serial accepted: %d %s", response.Code, response.Body.String())
	}
	if err := db.DB.First(&dev, "id = ?", dev.ID).Error; err != nil {
		t.Fatal(err)
	}
	if dev.IsRevoked {
		t.Fatal("serial-less device silently marked as revoked")
	}
}

func TestRevocationTransactionRollsBackWhenDeviceUpdateFails(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name": "Atomic revoke rollback subnet", "cidr_block": "10.226.0.0/24"})
	if group.Code != 201 {
		t.Fatal(group.Body.String())
	}
	device := requestJSON(t, router, "POST", "/api/devices", admin, map[string]any{
		"hostname": "revoke-rollback", "device_type": "company", "resource_group_id": responseID(t, group, "resource_group")})
	if device.Code != 201 {
		t.Fatal(device.Body.String())
	}
	id := responseID(t, device, "device")
	if err := db.DB.Exec("CREATE TRIGGER deny_revoke BEFORE UPDATE OF is_revoked ON devices BEGIN SELECT RAISE(ABORT, 'revoke denied'); END;").Error; err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, router, "POST", "/api/devices/"+id+"/revoke", admin, nil)
	if response.Code != 503 {
		t.Fatalf("rejected transaction returned %d: %s", response.Code, response.Body.String())
	}
	var dev db.Device
	if err := db.DB.First(&dev, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if dev.IsRevoked {
		t.Fatal("failed SQL transaction wrote revocation")
	}
	var count int64
	if err := db.DB.Model(&db.CertificateRevocation{}).Where("device_id = ?", id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed SQL transaction left a revocation ledger entry")
	}
}

func TestRevocationReportsPendingWhenDaemonCannotInspectLiveSessions(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name": "Offline revoke subnet", "cidr_block": "10.225.0.0/24"})
	if group.Code != 201 {
		t.Fatal(group.Body.String())
	}
	device := requestJSON(t, router, "POST", "/api/devices", admin, map[string]any{
		"hostname": "my-client", "device_type": "company", "resource_group_id": responseID(t, group, "resource_group")})
	if device.Code != 201 {
		t.Fatal(device.Body.String())
	}
	id := responseID(t, device, "device")
	simulatedRevocationEnvironment(t, false)
	vici.DefaultClient = vici.NewClient(filepath.Join(t.TempDir(), "unavailable.vici"))
	response := requestJSON(t, router, "POST", "/api/devices/"+id+"/revoke", admin, nil)
	if response.Code != 503 || !strings.Contains(response.Body.String(), "revocation_pending") {
		t.Fatalf("offline daemon silently accepted revoke: %d %s", response.Code, response.Body.String())
	}
	var dev db.Device
	if err := db.DB.First(&dev, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if !dev.IsRevoked {
		t.Fatal("daemon outage reverted durable certificate revocation")
	}
}

func TestRevocationRejectsUntrustedExistingCRLEntryWithoutDroppingRevocation(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name": "Broken CRL subnet", "cidr_block": "10.224.0.0/24"})
	if group.Code != 201 {
		t.Fatal(group.Body.String())
	}
	device := requestJSON(t, router, "POST", "/api/devices", admin, map[string]any{
		"hostname": "corrupt-crl-client", "device_type": "company", "resource_group_id": responseID(t, group, "resource_group")})
	if device.Code != 201 {
		t.Fatal(device.Body.String())
	}
	id := responseID(t, device, "device")
	if err := db.DB.Create(&db.CertificateRevocation{Serial: "not-hex", DeviceID: "legacy"}).Error; err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, router, "POST", "/api/devices/"+id+"/revoke", admin, nil)
	if response.Code != 503 {
		t.Fatalf("invalid CRL was silently accepted: %d %s", response.Code, response.Body.String())
	}
	var dev db.Device
	if err := db.DB.First(&dev, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if !dev.IsRevoked {
		t.Fatal("corrupted CRL reverted durable identity revocation")
	}
}

func TestRevocationReportsPendingWhenDaemonRefusesToTerminateAnExistingSession(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := db.ResourceGroup{ID: "terminate-reject-net", Name: "Terminate reject net", CIDRBlock: "10.239.0.0/24"}
	if err := db.DB.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	device := db.Device{ID: "terminate-reject-device", Hostname: "my-client", DeviceType: "company",
		ResourceGroupID: group.ID, VirtualIP: "10.239.0.2", Platform: "linux",
		IPAllocationType: "static", CertSerial: "aabbccdd"}
	if err := db.DB.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	simulatedRevocationEnvironment(t, false, "terminate")
	result := mustStatus(t, router, "POST", "/api/devices/"+device.ID+"/revoke", admin, nil, 503)
	if !strings.Contains(result, "revocation_pending") {
		t.Fatalf("rejected IKE termination was incorrectly reported successful: %s", result)
	}
	var stored db.Device
	if err := db.DB.First(&stored, "id = ?", device.ID).Error; err != nil || !stored.IsRevoked {
		t.Fatalf("rejected SA termination rolled back durable revocation: %+v %v", stored, err)
	}
}

func TestRevocationRejectsUnsafeDeviceBundleIdentifierAfterRecordingLedger(t *testing.T) {
	_, cleanup := setupTestRouter(t)
	defer cleanup()
	group := db.ResourceGroup{ID: "invalid-id-net", Name: "Invalid ID net", CIDRBlock: "10.240.0.0/24"}
	if err := db.DB.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	device := db.Device{ID: "../invalid-device-id", Hostname: "invalid-device-id", DeviceType: "company",
		ResourceGroupID: group.ID, VirtualIP: "10.240.0.2", Platform: "linux",
		IPAllocationType: "static", CertSerial: "aabbccddee"}
	if err := db.DB.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("POST", "/test/revoke", nil)
	context.Params = gin.Params{{Key: "id", Value: device.ID}}
	context.Set("is_admin", true)
	handlers.NewResourcesHandler().RevokeDevice(context)
	if recorder.Code != 503 {
		t.Fatalf("unsafe credential removal path was accepted: %d %s", recorder.Code, recorder.Body.String())
	}
	var stored db.Device
	if err := db.DB.First(&stored, "id = ?", device.ID).Error; err != nil || !stored.IsRevoked {
		t.Fatalf("unsafe removal corrupted durable revocation: %+v %v", stored, err)
	}
}
