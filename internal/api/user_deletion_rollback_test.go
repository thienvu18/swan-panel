package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
)

func TestUserDeletionRejectsUnrevocableIdentityAndPreservesAllRows(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	uid := createAdminFixture(t, router, admin, "legacy-device")
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name": "Legacy-device test pool", "cidr_block": "10.213.0.0/24"})
	if group.Code != 201 {
		t.Fatal(group.Body.String())
	}
	device := db.Device{ID: "unrevocable-device", Hostname: "legacy-device-a", DeviceType: "user",
		UserID: &uid, ResourceGroupID: responseID(t, group, "resource_group"),
		IPAllocationType: "static", VirtualIP: "10.213.0.3", Platform: "linux"}
	if err := db.DB.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, router, "DELETE", "/api/users/"+uid, admin, nil)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("no certificate serial should block user deletion: %d %s", response.Code, response.Body.String())
	}
	var stored db.User
	if err := db.DB.First(&stored, "id = ?", uid).Error; err != nil {
		t.Fatal("partial deletion:", err)
	}
	if err := db.DB.First(&device, "id = ?", device.ID).Error; err != nil {
		t.Fatal(err)
	}
	if device.IsRevoked {
		t.Fatal("failed deletion modified device outside transaction")
	}
}

func TestUserDeletionTransactionRollsBackWhenDeviceDetachFails(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	uid := createAdminFixture(t, router, admin, "rollback-user")
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]any{
		"name": "Rollback test pool", "cidr_block": "10.212.0.0/24"})
	if group.Code != 201 {
		t.Fatal(group.Body.String())
	}
	device := requestJSON(t, router, "POST", "/api/devices", admin, map[string]any{
		"hostname": "rollback-device", "device_type": "user", "user_id": uid,
		"resource_group_id": responseID(t, group, "resource_group")})
	if device.Code != 201 {
		t.Fatal(device.Body.String())
	}
	id := responseID(t, device, "device")
	if err := db.DB.Exec("CREATE TRIGGER deny_detach BEFORE UPDATE OF user_id ON devices BEGIN SELECT RAISE(ABORT, 'cannot detach'); END;").Error; err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, router, "DELETE", "/api/users/"+uid, admin, nil)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("database failure ignored: %d %s", response.Code, response.Body.String())
	}
	var user db.User
	if err := db.DB.First(&user, "id = ?", uid).Error; err != nil {
		t.Fatal("rollback lost user", err)
	}
	var dev db.Device
	if err := db.DB.First(&dev, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if dev.IsRevoked {
		t.Fatal("rolled back revocation marked as complete")
	}
	var count int64
	if err := db.DB.Model(&db.CertificateRevocation{}).Where("device_id = ?", id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled-back deletion left revocation records: %d", count)
	}
}

func TestDeletedUserIsPersistedAndReportsPendingWhenIndependentGatewayPolicyIsBroken(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	uid := createAdminFixture(t, router, admin, "pending-user")
	group := db.ResourceGroup{ID: "unrelated-broken-group", Name: "Broken unrelated gateway", CIDRBlock: "10.211.0.0/24"}
	if err := db.DB.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	// Do not create a certificate for this deliberately malformed fixture;
	// it represents an unrelated operator-induced data-plane inconsistency.
	bad := db.Device{ID: "broken-unrelated-device", Hostname: "unrelated-device", DeviceType: "company",
		ResourceGroupID: group.ID, IPAllocationType: "static", VirtualIP: "bad", Platform: "linux"}
	if err := db.DB.Create(&bad).Error; err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, router, "DELETE", "/api/users/"+uid, admin, nil)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), "pending") {
		t.Fatalf("deletion must report pending state: %d %s", response.Code, response.Body.String())
	}
	var count int64
	if err := db.DB.Model(&db.User{}).Where("id = ?", uid).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("deleted user reappeared after gateway apply failed")
	}
}

func TestUserDeletionRejectsMissingDeviceTableWithoutPartialRemoval(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	uid := createAdminFixture(t, router, admin, "missing-device-table")
	if err := db.DB.Migrator().DropTable(&db.Device{}); err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, router, "DELETE", "/api/users/"+uid, admin, nil)
	if response.Code != 503 {
		t.Fatalf("missing inventory did not abort deletion: %d %s", response.Code, response.Body.String())
	}
}
