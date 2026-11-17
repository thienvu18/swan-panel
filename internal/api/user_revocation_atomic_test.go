package api_test

import (
	"net/http"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
)

func TestUserDeletionAtomicallyRecordsAllIssuedCertificateRevocations(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	uid := createAdminFixture(t, router, token, "revoke-all-user")
	rg := requestJSON(t, router, "POST", "/api/resource-groups", token, map[string]any{
		"name": "Revocation subnet", "cidr_block": "10.76.0.0/24",
	})
	if rg.Code != http.StatusCreated {
		t.Fatal(rg.Body.String())
	}
	gid := responseID(t, rg, "resource_group")
	ids := []string{}
	for _, hostname := range []string{"revoke-laptop", "revoke-phone"} {
		response := requestJSON(t, router, "POST", "/api/devices", token, map[string]any{
			"hostname": hostname, "device_type": "user", "user_id": uid,
			"resource_group_id": gid, "platform": "linux",
		})
		if response.Code != http.StatusCreated {
			t.Fatalf("device %s: %s", hostname, response.Body.String())
		}
		ids = append(ids, responseID(t, response, "device"))
	}
	if response := requestJSON(t, router, "DELETE", "/api/users/"+uid, token, nil); response.Code != http.StatusOK {
		t.Fatalf("delete user: %d %s", response.Code, response.Body.String())
	}
	var users int64
	if err := db.DB.Model(&db.User{}).Where("id = ?", uid).Count(&users).Error; err != nil || users != 0 {
		t.Fatalf("deleted user still exists: %d %v", users, err)
	}
	for _, id := range ids {
		var dev db.Device
		if err := db.DB.First(&dev, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if !dev.IsRevoked {
			t.Fatalf("device %s still active", id)
		}
		var rev db.CertificateRevocation
		if err := db.DB.First(&rev, "serial = ?", dev.CertSerial).Error; err != nil {
			t.Fatalf("device %s revocation absent: %v", id, err)
		}
	}
}
