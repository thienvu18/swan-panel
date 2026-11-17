package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/api/middleware"
	"github.com/swan-panel/swan-panel/internal/db"
)

func TestDeviceDirectoryRespectsOwnershipEvenWithCrossOwnerQueryParameters(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	aliceID := createAdminFixture(t, router, admin, "device-alice")
	bobID := createAdminFixture(t, router, admin, "device-bob")
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name": "Device isolation subnet", "cidr_block": "10.152.1.0/24",
	})
	if group.Code != http.StatusCreated {
		t.Fatal(group.Body.String())
	}
	groupID := responseID(t, group, "resource_group")
	for _, tc := range []struct{ host, id string }{
		{"alice-laptop", aliceID}, {"bob-laptop", bobID},
	} {
		response := requestJSON(t, router, "POST", "/api/devices", admin, map[string]any{
			"hostname": tc.host, "device_type": "user", "user_id": tc.id,
			"resource_group_id": groupID,
		})
		if response.Code != http.StatusCreated {
			t.Fatal(response.Body.String())
		}
	}
	var owner db.User
	if err := db.DB.First(&owner, "id = ?", aliceID).Error; err != nil {
		t.Fatal(err)
	}
	token, err := middleware.GenerateToken(&owner, "test-secret-key-12345", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	listed := mustStatus(t, router, "GET", "/api/devices?user_id="+bobID+"&device_type=company", token, nil, 200)
	if !strings.Contains(listed, "alice-laptop") || strings.Contains(listed, "bob-laptop") {
		t.Fatalf("non-admin obtained another owner's devices: %s", listed)
	}
	adminFilter := mustStatus(t, router, "GET", "/api/devices?user_id="+bobID+"&device_type=user", admin, nil, 200)
	if !strings.Contains(adminFilter, "bob-laptop") || strings.Contains(adminFilter, "alice-laptop") {
		t.Fatalf("administrator filter escaped its query: %s", adminFilter)
	}
	mustStatus(t, router, "DELETE", "/api/devices/missing", token, nil, 403)
	mustStatus(t, router, "GET", "/api/devices?device_type=company", admin, nil, 200)
}
func TestDeviceDeletionRevokesIdentityBeforeSoftDelete(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name": "Device removal subnet", "cidr_block": "10.153.0.0/24",
	})
	if group.Code != 201 {
		t.Fatal(group.Body.String())
	}
	provisioned := requestJSON(t, router, "POST", "/api/devices", admin, map[string]any{
		"hostname": "delete-secure-client", "device_type": "company",
		"resource_group_id": responseID(t, group, "resource_group"),
	})
	if provisioned.Code != 201 {
		t.Fatal(provisioned.Body.String())
	}
	deviceID := responseID(t, provisioned, "device")
	mustStatus(t, router, "DELETE", "/api/devices/missing", admin, nil, 404)
	mustStatus(t, router, "DELETE", "/api/devices/"+deviceID, admin, nil, 200)
	var count int64
	if err := db.DB.Model(&db.Device{}).Where("id = ?", deviceID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("deleted device remained active: %d %v", count, err)
	}
	var deleted db.Device
	if err := db.DB.Unscoped().First(&deleted, "id = ?", deviceID).Error; err != nil {
		t.Fatal(err)
	}
	if !deleted.IsRevoked || !deleted.DeletedAt.Valid {
		t.Fatalf("identity not revoked before deletion: %+v", deleted)
	}
	var ledger db.CertificateRevocation
	if err := db.DB.First(&ledger, "serial = ?", deleted.CertSerial).Error; err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "GET", "/api/devices/"+deviceID+"/config/macos", admin, nil, 404)
}

func TestDeviceDeletionCannotRemoveIdentityWithMissingRevocationSerial(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name": "Legacy deletion subnet", "cidr_block": "10.154.0.0/24",
	})
	if group.Code != 201 {
		t.Fatal(group.Body.String())
	}
	dev := db.Device{ID: "legacy-serialless-device", Hostname: "serialless-client", DeviceType: "company",
		ResourceGroupID:  responseID(t, group, "resource_group"),
		IPAllocationType: "static", VirtualIP: "10.154.0.4", Platform: "linux",
	}
	if err := db.DB.Create(&dev).Error; err != nil {
		t.Fatal(err)
	}
	result := requestJSON(t, router, "DELETE", "/api/devices/"+dev.ID, admin, nil)
	if result.Code != 503 {
		t.Fatalf("unsafe deletion not blocked: %d %s", result.Code, result.Body.String())
	}
	var persisted db.Device
	if err := db.DB.First(&persisted, "id = ?", dev.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.IsRevoked {
		t.Fatal("serial-less device incorrectly marked revoked")
	}
}
