package api_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/pki"
	"github.com/swan-panel/swan-panel/internal/policy"
)

func TestUnavailableRootCAFailsClosedAcrossControlPlaneOperations(t *testing.T) {
	const child = "SWANPANEL_TEST_ROOT_CA_UNAVAILABLE"
	if os.Getenv(child) != "1" {
		// The production PKI manager uses sync.Once. An isolated test process
		// exercises its first-initialization failure without resetting globals
		// or making unrelated parallel tests unsafe.
		command := exec.Command(os.Args[0], "-test.run=^TestUnavailableRootCAFailsClosedAcrossControlPlaneOperations$")
		command.Env = append(os.Environ(), child+"=1")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated unavailable-CA test failed: %v\n%s", err, output)
		}
		return
	}
	path := filepath.Join(t.TempDir(), "not-a-pki-directory")
	if err := os.WriteFile(path, []byte("cannot create PKI here"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "production")
	t.Setenv("PKI_DATA_DIR", path)
	if manager, err := pki.GetPKIManager(); err == nil || manager != nil {
		t.Fatalf("unusable persistent PKI directory accepted: %+v %v", manager, err)
	}
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	mustStatus(t, router, "GET", "/api/settings/ca", admin, nil, 503)
	mustStatus(t, router, "POST", "/api/settings/ca/rotate", admin, caRotatePayload(), 503)
	payload := gatewaySettingsPayload(t, mustStatus(t, router, "GET", "/api/settings/strongswan", admin, nil, 200))
	result := mustStatus(t, router, "PUT", "/api/settings/strongswan", admin, payload, 202)
	if !strings.Contains(result, "CA unavailable") {
		t.Fatalf("gateway settings hid PKI failure: %s", result)
	}
	if err := policy.ReconcileGateway(context.Background(), db.DB); err == nil ||
		!strings.Contains(err.Error(), "initialize gateway CA") {
		t.Fatalf("reconciler accepted unavailable CA: %v", err)
	}
	group := db.ResourceGroup{ID: "unavailable-ca-net", Name: "Unavailable CA net", CIDRBlock: "10.242.0.0/24"}
	if err := db.DB.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	device := db.Device{ID: "unavailable-ca-device", Hostname: "unavailable-ca-client", DeviceType: "company",
		ResourceGroupID: group.ID, VirtualIP: "10.242.0.2", IPAllocationType: "static",
		Platform: "linux", CertSerial: "deadbeef"}
	if err := db.DB.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "POST", "/api/devices/"+device.ID+"/revoke", admin, nil, 503)
}
