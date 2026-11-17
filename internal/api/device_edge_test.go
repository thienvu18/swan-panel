package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/api/middleware"
	"github.com/swan-panel/swan-panel/internal/db"
	"time"
)

func provisionDevice(t *testing.T, router *gin.Engine, token, group, hostname, mode, vip string, extras map[string]any) int {
	t.Helper()
	body := map[string]any{"hostname": hostname, "device_type": "company", "resource_group_id": group,
		"ip_allocation_type": mode, "platform": "linux"}
	if vip != "" {
		body["virtual_ip"] = vip
	}
	for k, v := range extras {
		body[k] = v
	}
	return requestJSON(t, router, "POST", "/api/devices", token, body).Code
}

func TestResourceGroupsRejectOverlappingCIDRAndProtectDeviceReferences(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	invalid := []string{"not-a-cidr", "10.0.0.0/31", "10.0.0.0/7", "2001:db8::/64"}
	for _, prefix := range invalid {
		code := requestJSON(t, router, "POST", "/api/resource-groups", token, map[string]any{
			"name": "invalid", "cidr_block": prefix,
		}).Code
		if code != http.StatusBadRequest {
			t.Fatalf("invalid prefix %q accepted: %d", prefix, code)
		}
	}
	first := requestJSON(t, router, "POST", "/api/resource-groups", token, map[string]string{
		"name": "Subnet", "cidr_block": "10.42.0.5/24",
	})
	if first.Code != 201 {
		t.Fatal(first.Body.String())
	}
	id := responseID(t, first, "resource_group")
	mustStatus(t, router, "POST", "/api/resource-groups", token, map[string]string{
		"name": "Nested", "cidr_block": "10.42.0.0/25",
	}, http.StatusConflict)
	mustStatus(t, router, "POST", "/api/resource-groups", token, map[string]string{
		"name": "Supernet", "cidr_block": "10.42.0.0/16",
	}, http.StatusConflict)
	listing := mustStatus(t, router, "GET", "/api/resource-groups", token, nil, http.StatusOK)
	if !strings.Contains(listing, "10.42.0.0/24") {
		t.Fatal("subnet not normalized")
	}
	if got := provisionDevice(t, router, token, id, "referenced-device", "dynamic", "", nil); got != http.StatusCreated {
		t.Fatalf("device: %d", got)
	}
	mustStatus(t, router, "DELETE", "/api/resource-groups/"+id, token, nil, http.StatusConflict)
	mustStatus(t, router, "DELETE", "/api/resource-groups/missing", token, nil, http.StatusNotFound)
}

func TestDeviceProvisioningRejectsHostileIdentityAndDisallowedIP(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	rg := requestJSON(t, router, "POST", "/api/resource-groups", token, map[string]string{
		"name": "Enrollment subnet", "cidr_block": "10.44.0.0/29",
	})
	if rg.Code != 201 {
		t.Fatal(rg.Body.String())
	}
	gid := responseID(t, rg, "resource_group")
	cases := []struct {
		name, mode, vip string
		extra           map[string]any
		expected        int
	}{
		{"bad..hostname", "dynamic", "", nil, 400},
		{"ok-unknown-platform", "dynamic", "", map[string]any{"platform": "freebsd"}, 400},
		{"ok-unknown-type", "dynamic", "", map[string]any{"device_type": "outside"}, 400},
		{"ok-unknown-allocation", "manual", "", nil, 400},
		{"ok-missing-owner", "dynamic", "", map[string]any{"device_type": "user"}, 400},
		{"ok-invalid-ip", "static", "not-ip", nil, 409},
		{"ok-other-network", "static", "192.0.2.3", nil, 409},
		{"ok-network", "static", "10.44.0.0", nil, 409},
		{"ok-gateway", "static", "10.44.0.1", nil, 409},
		{"ok-broadcast", "static", "10.44.0.7", nil, 409},
		{"ok-unknown-resource", "dynamic", "", map[string]any{"resource_group_id": "missing"}, 409},
		{"ok-non-active-owner", "dynamic", "", map[string]any{"device_type": "user", "user_id": "missing"}, 409},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := provisionDevice(t, router, token, gid, tc.name, tc.mode, tc.vip, tc.extra)
			if got != tc.expected {
				t.Fatalf("invalid enrollment accepted or returned wrong status: %d expected %d", got, tc.expected)
			}
		})
	}
	good := requestJSON(t, router, "POST", "/api/devices", token, map[string]any{
		"hostname": "test-device", "device_type": "company", "resource_group_id": gid,
		"ip_allocation_type": "static", "virtual_ip": "10.44.0.2", "platform": "ios",
	})
	if good.Code != http.StatusCreated {
		t.Fatal(good.Body.String())
	}
	id := responseID(t, good, "device")
	if got := provisionDevice(t, router, token, gid, "duplicate-vip", "static", "10.44.0.2", nil); got != 409 {
		t.Fatalf("duplicate active VIP accepted: %d", got)
	}
	denied := requestJSON(t, router, "GET", "/api/devices/"+id+"/config/unsupported", token, nil)
	if denied.Code != 400 {
		t.Fatalf("invalid export accepted: %d", denied.Code)
	}
	missing := requestJSON(t, router, "GET", "/api/devices/missing/config/linux", token, nil)
	if missing.Code != 404 {
		t.Fatalf("missing device export: %d", missing.Code)
	}
	mustStatus(t, router, "POST", "/api/devices/missing/revoke", token, nil, 404)
	mustStatus(t, router, "DELETE", "/api/devices/missing", token, nil, 404)
}

func TestOwnerCannotProvisionCompanyNodeOrDownloadOtherUsersCertificate(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	userID := createAdminFixture(t, router, admin, "untrusted-owner")
	strangerID := createAdminFixture(t, router, admin, "another-owner")
	var user db.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		t.Fatal(err)
	}
	token, err := middleware.GenerateToken(&user, "test-secret-key-12345", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	rg := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]string{
		"name": "Users subnet", "cidr_block": "10.45.0.0/24",
	})
	if rg.Code != 201 {
		t.Fatal(rg.Body.String())
	}
	gid := responseID(t, rg, "resource_group")
	if got := provisionDevice(t, router, token, gid, "untrusted-company", "dynamic", "", nil); got != 403 {
		t.Fatalf("ordinary user enrolled company device: %d", got)
	}
	owned := requestJSON(t, router, "POST", "/api/devices", token, map[string]any{
		"hostname": "owner-device", "device_type": "user", "resource_group_id": gid,
		"platform": "linux", "user_id": strangerID,
	})
	if owned.Code != 201 {
		t.Fatal(owned.Body.String())
	}
	did := responseID(t, owned, "device")
	var actual db.Device
	if err := db.DB.First(&actual, "id = ?", did).Error; err != nil {
		t.Fatal(err)
	}
	if actual.UserID == nil || *actual.UserID != userID {
		t.Fatalf("user spoofed device owner: %+v", actual.UserID)
	}
	var stranger db.User
	if err := db.DB.First(&stranger, "id = ?", strangerID).Error; err != nil {
		t.Fatal(err)
	}
	strangerToken, err := middleware.GenerateToken(&stranger, "test-secret-key-12345", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "GET", "/api/devices/"+did+"/config/linux", strangerToken, nil, 403)
	mustStatus(t, router, "POST", "/api/devices/"+did+"/revoke", strangerToken, nil, 403)
	listing := mustStatus(t, router, "GET", "/api/devices", strangerToken, nil, 200)
	if strings.Contains(listing, did) {
		t.Fatal("device private key owner leak")
	}
}

func TestResourceGroupDeletionBlockedByAccessRuleEvenWithoutDevices(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	uid := createAdminFixture(t, router, token, "resource-policy-user")
	rg := requestJSON(t, router, "POST", "/api/resource-groups", token, map[string]string{
		"name": "Policy subnet", "cidr_block": "10.46.0.0/24",
	})
	if rg.Code != 201 {
		t.Fatal(rg.Body.String())
	}
	gid := responseID(t, rg, "resource_group")
	rule := requestJSON(t, router, "POST", "/api/access-rules", token, map[string]any{
		"subject_type": "user", "subject_id": uid, "action": "ALLOW",
		"target_type": "resource_group", "target_value": gid,
	})
	if rule.Code != 201 {
		t.Fatal(rule.Body.String())
	}
	rid := responseID(t, rule, "access_rule")
	mustStatus(t, router, "DELETE", "/api/resource-groups/"+gid, token, nil, 409)
	mustStatus(t, router, "DELETE", "/api/access-rules/"+rid, token, nil, 200)
	mustStatus(t, router, "DELETE", "/api/resource-groups/"+gid, token, nil, 200)
}

// Every platform must receive a stable, previously issued certificate rather
// than reissuing it on each download. A revoked identity must never be exported.
func TestDeviceExportsUseStableCertificatesAcrossAllSupportedClientPlatforms(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	rg := requestJSON(t, router, "POST", "/api/resource-groups", token, map[string]string{
		"name": "Profile export subnet", "cidr_block": "10.47.0.0/24",
	})
	if rg.Code != http.StatusCreated {
		t.Fatal(rg.Body.String())
	}
	gid := responseID(t, rg, "resource_group")
	device := requestJSON(t, router, "POST", "/api/devices", token, map[string]any{
		"hostname": "profile-export", "device_type": "company", "resource_group_id": gid,
		"platform": "linux", "ip_allocation_type": "dynamic",
	})
	if device.Code != http.StatusCreated {
		t.Fatalf("enroll profile export identity: %s", device.Body.String())
	}
	id := responseID(t, device, "device")
	var row db.Device
	if err := db.DB.First(&row, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	firstSerial := row.CertSerial
	if firstSerial == "" {
		t.Fatal("device was enrolled without a certificate serial")
	}
	endpoints := []struct{ path, required string }{
		{"config/apple", ""}, // Signed PKCS7 binary validated by status/content type.
		{"config/windows", "Add-VpnConnection"},
		{"config/linux", "swanctl"},
		{"config/linux?format=conf", "connections {"},
		{"config/android", "ikev2-cert"},
		{"config/android-native", ""},
	}
	for _, tc := range endpoints {
		t.Run(tc.path, func(t *testing.T) {
			response := requestJSON(t, router, "GET", "/api/devices/"+id+"/"+tc.path, token, nil)
			if response.Code != http.StatusOK {
				t.Fatalf("profile %s returned %d: %s", tc.path, response.Code, response.Body.String())
			}
			if tc.required != "" && !strings.Contains(response.Body.String(), tc.required) {
				t.Fatalf("profile %s missing required content %q", tc.path, tc.required)
			}
			if len(response.Body.Bytes()) < 200 {
				t.Fatalf("profile %s unexpectedly empty", tc.path)
			}
		})
	}
	if err := db.DB.First(&row, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if row.CertSerial != firstSerial {
		t.Fatal("profile download rotated issued client certificate")
	}
	mustStatus(t, router, "POST", "/api/devices/"+id+"/revoke", token, nil, http.StatusOK)
	mustStatus(t, router, "GET", "/api/devices/"+id+"/config/linux", token, nil, http.StatusForbidden)
	mustStatus(t, router, "GET", "/api/devices/"+id+"/config/windows", token, nil, http.StatusForbidden)
}
