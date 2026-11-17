package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
)

func TestDeviceListingReportsUnavailableInventory(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	if err := db.DB.Migrator().DropTable(&db.Device{}); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, http.MethodGet, "/api/devices", admin, nil, 500)
}

func TestAccessRuleDeletionRetainsPendingStatusWhenGatewayPolicyIsInvalid(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	rule := db.AccessRule{ID: "pending-policy-delete", SubjectType: "user", SubjectID: "dummy",
		Action: "ALLOW", TargetType: "cidr_ip", TargetValue: "10.70.0.0/24"}
	if err := db.DB.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	group := db.ResourceGroup{ID: "group-invalid-vip", Name: "Invalid VIP group", CIDRBlock: "10.72.0.0/24"}
	if err := db.DB.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	device := db.Device{ID: "invalid-vip", Hostname: "invalid-vip.example.test", DeviceType: "company",
		ResourceGroupID: group.ID, VirtualIP: "not-a-vip", IPAllocationType: "static", Platform: "linux"}
	if err := db.DB.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	result := mustStatus(t, router, http.MethodDelete, "/api/access-rules/"+rule.ID, admin, nil, http.StatusAccepted)
	if !strings.Contains(result, "pending") {
		t.Fatalf("policy mutation not reported pending: %s", result)
	}
	var count int64
	if err := db.DB.Model(&db.AccessRule{}).Where("id = ?", rule.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("access rule was not deleted: %d %v", count, err)
	}
}

func TestGatewaySettingsRejectsUnavailablePersistenceAndUnsupportedAction(t *testing.T) {
	t.Run("missing current settings", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		admin := getAdminToken(t, router)
		payload := gatewaySettingsPayload(t, mustStatus(t, router, "GET", "/api/settings/strongswan", admin, nil, 200))
		if err := db.DB.Migrator().DropTable(&db.StrongswanSettings{}); err != nil {
			t.Fatal(err)
		}
		mustStatus(t, router, "PUT", "/api/settings/strongswan", admin, payload, 503)
	})
	t.Run("database refuses persisted update", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		admin := getAdminToken(t, router)
		payload := gatewaySettingsPayload(t, mustStatus(t, router, "GET", "/api/settings/strongswan", admin, nil, 200))
		if err := db.DB.Exec("CREATE TRIGGER block_gateway_write BEFORE UPDATE ON strongswan_settings BEGIN SELECT RAISE(FAIL, 'blocked settings'); END").Error; err != nil {
			t.Fatal(err)
		}
		mustStatus(t, router, "PUT", "/api/settings/strongswan", admin, payload, 500)
	})
	t.Run("invalid DPD action", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		admin := getAdminToken(t, router)
		payload := gatewaySettingsPayload(t, mustStatus(t, router, "GET", "/api/settings/strongswan", admin, nil, 200))
		payload["dpd_action"] = "unsafe"
		mustStatus(t, router, "PUT", "/api/settings/strongswan", admin, payload, 400)
		payload["dpd_action"] = "clear"
		payload["vpn_name"] = "   "
		mustStatus(t, router, "PUT", "/api/settings/strongswan", admin, payload, 200)
		stored := gatewaySettingsPayload(t, mustStatus(t, router, "GET", "/api/settings/strongswan", admin, nil, 200))
		if stored["vpn_name"] != "SwanPanel VPN" {
			t.Fatalf("blank VPN display name not normalized: %+v", stored)
		}
	})
}

func TestAdministratorRoleUpdateFailsWithoutPartialPrivilegeEscalation(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	userID := createAdminFixture(t, router, admin, "role-write-failure")
	if err := db.DB.Exec("CREATE TRIGGER block_role_change BEFORE UPDATE OF is_admin ON users BEGIN SELECT RAISE(ABORT, 'role locked'); END").Error; err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "PUT", "/api/users/"+userID+"/role", admin, nil, 503)
	var user db.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil || user.IsAdmin {
		t.Fatalf("failed privilege change was persisted: %+v %v", user, err)
	}
}

func TestUserGroupDeletionRejectsMissingPolicyLedgerAndReportsGatewayPending(t *testing.T) {
	t.Run("policy ledger missing", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		admin := getAdminToken(t, router)
		group := db.UserGroup{ID: "missing-rules-group", Name: "Missing rules"}
		if err := db.DB.Create(&group).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.DB.Migrator().DropTable(&db.AccessRule{}); err != nil {
			t.Fatal(err)
		}
		mustStatus(t, router, "DELETE", "/api/user-groups/"+group.ID, admin, nil, 409)
	})
	t.Run("association cannot be cleared", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		admin := getAdminToken(t, router)
		group := db.UserGroup{ID: "missing-join-group", Name: "Missing join"}
		if err := db.DB.Create(&group).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.DB.Migrator().DropTable("user_group_members"); err != nil {
			t.Fatal(err)
		}
		mustStatus(t, router, "DELETE", "/api/user-groups/"+group.ID, admin, nil, 409)
	})
	t.Run("gateway policy cannot apply", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		admin := getAdminToken(t, router)
		group := db.UserGroup{ID: "pending-delete-group", Name: "Pending delete"}
		if err := db.DB.Create(&group).Error; err != nil {
			t.Fatal(err)
		}
		network := db.ResourceGroup{ID: "pending-broken-network", Name: "Pending broken network", CIDRBlock: "10.70.0.0/24"}
		if err := db.DB.Create(&network).Error; err != nil {
			t.Fatal(err)
		}
		device := db.Device{ID: "pending-broken-device", Hostname: "pending-broken-device",
			DeviceType: "company", ResourceGroupID: network.ID, VirtualIP: "invalid",
			IPAllocationType: "static", Platform: "linux"}
		if err := db.DB.Create(&device).Error; err != nil {
			t.Fatal(err)
		}
		mustStatus(t, router, "DELETE", "/api/user-groups/"+group.ID, admin, nil, 202)
	})
}

func TestDeviceSoftDeletionFailurePreservesRecordedCertificateRevocation(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	network := db.ResourceGroup{ID: "blocked-deletion-net", Name: "Blocked deletion net", CIDRBlock: "10.71.0.0/24"}
	if err := db.DB.Create(&network).Error; err != nil {
		t.Fatal(err)
	}
	device := db.Device{ID: "blocked-delete-device", Hostname: "blocked-delete-device", DeviceType: "company",
		ResourceGroupID: network.ID, VirtualIP: "10.71.0.4", IPAllocationType: "static",
		Platform: "linux", CertSerial: "badc0ffee"}
	if err := db.DB.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Exec("CREATE TRIGGER block_device_soft_delete BEFORE UPDATE OF deleted_at ON devices BEGIN SELECT RAISE(ABORT, 'delete blocked'); END").Error; err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "DELETE", "/api/devices/"+device.ID, admin, nil, 500)
	var stored db.Device
	if err := db.DB.First(&stored, "id = ?", device.ID).Error; err != nil || !stored.IsRevoked {
		t.Fatalf("revocation was not retained before failed deletion: %+v %v", stored, err)
	}
	var count int64
	if err := db.DB.Model(&db.CertificateRevocation{}).Where("device_id = ?", device.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("revocation audit record missing: %d %v", count, err)
	}
}
