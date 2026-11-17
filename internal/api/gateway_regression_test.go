package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/api/middleware"
	"github.com/swan-panel/swan-panel/internal/db"
)

func TestNonAdminDashboardCannotReadOtherUsersDevicesOrGlobalCounts(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	_ = getAdminToken(t, router)
	owner := db.User{ID: "scope-user-a", Name: "Owner A", Email: "scope-a@example.org", Status: "ACTIVE", AuthType: "local"}
	stranger := db.User{ID: "scope-user-b", Name: "Owner B", Email: "scope-b@example.org", Status: "ACTIVE", AuthType: "local"}
	if err := db.DB.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Create(&stranger).Error; err != nil {
		t.Fatal(err)
	}
	group := db.ResourceGroup{ID: "scope-group", Name: "Scope group", CIDRBlock: "10.91.0.0/24"}
	if err := db.DB.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	devices := []db.Device{
		{ID: "device-owned", Hostname: "owned", DeviceType: "user", UserID: &owner.ID,
			ResourceGroupID: group.ID, IPAllocationType: "static", VirtualIP: "10.91.0.2", Platform: "linux"},
		{ID: "device-private", Hostname: "private", DeviceType: "user", UserID: &stranger.ID,
			ResourceGroupID: group.ID, IPAllocationType: "static", VirtualIP: "10.91.0.3", Platform: "linux"},
	}
	for _, device := range devices {
		if err := db.DB.Create(&device).Error; err != nil {
			t.Fatal(err)
		}
	}
	token, err := middleware.GenerateToken(&owner, "test-secret-key-12345", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, router, http.MethodGet, "/api/dashboard/stats", token, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("stats status %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "device-private") || strings.Contains(response.Body.String(), "scope-b@example.org") {
		t.Fatalf("other user information exposed: %s", response.Body.String())
	}
	var payload struct {
		Stats   map[string]any
		Devices []db.Device
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Stats["total_devices"] != float64(1) || len(payload.Devices) != 1 || payload.Devices[0].ID != "device-owned" {
		t.Fatalf("dashboard not scoped: %+v", payload)
	}
	if _, leaked := payload.Stats["total_users"]; leaked {
		t.Fatal("global user count leaked")
	}
	if _, leaked := payload.Stats["pending_users"]; leaked {
		t.Fatal("pending user count leaked")
	}
	devicesResponse := requestJSON(t, router, http.MethodGet, "/api/devices", token, nil)
	if devicesResponse.Code != http.StatusOK || strings.Contains(devicesResponse.Body.String(), "device-private") {
		t.Fatalf("devices not scoped: %d %s", devicesResponse.Code, devicesResponse.Body.String())
	}
}

func TestAdvancedGatewayConfigAcceptsSafeExtensionsAndRejectsUnsafeKeys(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	settings := requestJSON(t, router, http.MethodGet, "/api/settings/strongswan", admin, nil)
	if settings.Code != http.StatusOK {
		t.Fatal(settings.Body.String())
	}
	var config map[string]any
	if err := json.Unmarshal(settings.Body.Bytes(), &config); err != nil {
		t.Fatal(err)
	}
	config["vpn_name"] = "Engineering VPN"
	config["advanced_connection"] = map[string]any{"unique": "replace"}
	config["advanced_child"] = map[string]any{"start_action": "none"}
	saved := requestJSON(t, router, http.MethodPut, "/api/settings/strongswan", admin, config)
	if saved.Code != http.StatusOK {
		t.Fatalf("safe advanced config rejected: %d %s", saved.Code, saved.Body.String())
	}
	loaded := requestJSON(t, router, http.MethodGet, "/api/settings/strongswan", admin, nil)
	if !strings.Contains(loaded.Body.String(), "Engineering VPN") ||
		!strings.Contains(loaded.Body.String(), "start_action") {
		t.Fatalf("advanced options were not persisted: %s", loaded.Body.String())
	}
	config["advanced_connection"] = map[string]any{"remote": map[string]any{"auth": "psk"}}
	denied := requestJSON(t, router, http.MethodPut, "/api/settings/strongswan", admin, config)
	if denied.Code != http.StatusBadRequest {
		t.Fatalf("auth override accepted: %d", denied.Code)
	}
	delete(config, "advanced_connection")
	config["unexpected_top_level"] = true
	denied = requestJSON(t, router, http.MethodPut, "/api/settings/strongswan", admin, config)
	if denied.Code != http.StatusBadRequest {
		t.Fatalf("unknown top-level key silently ignored: %d", denied.Code)
	}
}

func TestRootCACannotBeRotatedAfterDeviceIssuance(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, http.MethodPost, "/api/resource-groups", admin, map[string]any{
		"name": "CA lock network", "cidr_block": "10.73.0.0/24",
	})
	if group.Code != http.StatusCreated {
		t.Fatal(group.Body.String())
	}
	device := requestJSON(t, router, http.MethodPost, "/api/devices", admin, map[string]any{
		"hostname": "ca-lock-device", "device_type": "company",
		"resource_group_id":  responseID(t, group, "resource_group"),
		"ip_allocation_type": "dynamic", "platform": "macos",
	})
	if device.Code != http.StatusCreated {
		t.Fatal(device.Body.String())
	}
	status := requestJSON(t, router, http.MethodGet, "/api/settings/ca", admin, nil)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), "\"can_rotate\":false") {
		t.Fatalf("CA should be locked after first issue: %s", status.Body.String())
	}
	rotation := requestJSON(t, router, http.MethodPost, "/api/settings/ca/rotate", admin, map[string]string{
		"confirm": "ROTATE CA", "common_name": "Replaced CA",
		"organization": "Example", "country": "VN",
	})
	if rotation.Code != http.StatusConflict {
		t.Fatalf("unsafe CA rotation accepted: %d", rotation.Code)
	}
}

func TestDeletedDeviceReleasesVIPButNetworkGatewayAndBroadcastAreReserved(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, http.MethodPost, "/api/resource-groups", admin, map[string]any{
		"name": "Tiny VPN pool", "cidr_block": "10.72.0.0/30",
	})
	if group.Code != http.StatusCreated {
		t.Fatal(group.Body.String())
	}
	groupID := responseID(t, group, "resource_group")
	create := func(hostname string, mode string, ip string) *responseInfo {
		payload := map[string]any{"hostname": hostname, "device_type": "company",
			"resource_group_id": groupID, "ip_allocation_type": mode, "platform": "linux"}
		if ip != "" {
			payload["virtual_ip"] = ip
		}
		result := requestJSON(t, router, http.MethodPost, "/api/devices", admin, payload)
		return &responseInfo{status: result.Code, body: result.Body.String()}
	}
	for _, ip := range []string{"10.72.0.0", "10.72.0.1", "10.72.0.3"} {
		result := create("reserved-"+strings.ReplaceAll(ip, ".", "-"), "static", ip)
		if result.status != http.StatusConflict {
			t.Fatalf("reserved %s accepted: %+v", ip, result)
		}
	}
	first := create("first-device", "dynamic", "")
	if first.status != http.StatusCreated || !strings.Contains(first.body, "10.72.0.2") {
		t.Fatalf("unexpected lease: %+v", first)
	}
	exhausted := create("second-device", "dynamic", "")
	if exhausted.status != http.StatusConflict {
		t.Fatalf("exhausted /30 accepted: %+v", exhausted)
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(first.body), &parsed); err != nil {
		t.Fatal(err)
	}
	var d struct{ ID string }
	if err := json.Unmarshal(parsed["device"], &d); err != nil {
		t.Fatal(err)
	}
	deleted := requestJSON(t, router, http.MethodDelete, "/api/devices/"+d.ID, admin, nil)
	if deleted.Code != http.StatusOK {
		t.Fatalf("device deletion failed: %d %s", deleted.Code, deleted.Body.String())
	}
	reused := create("new-device", "dynamic", "")
	if reused.status != http.StatusCreated || !strings.Contains(reused.body, "10.72.0.2") {
		t.Fatalf("released address not reusable after enforced revocation: %+v", reused)
	}
}

type responseInfo struct {
	status int
	body   string
}
