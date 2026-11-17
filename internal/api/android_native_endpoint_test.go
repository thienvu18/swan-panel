package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

func TestAndroidNativeExportOneResponseAndRevocation(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin, map[string]string{
		"name": "Android native credential pool", "cidr_block": "10.176.0.0/24",
	})
	if group.Code != http.StatusCreated {
		t.Fatal(group.Body.String())
	}
	device := requestJSON(t, router, "POST", "/api/devices", admin, map[string]any{
		"hostname": "android-native-test", "device_type": "company",
		"resource_group_id": responseID(t, group, "resource_group"), "platform": "android",
	})
	if device.Code != http.StatusCreated {
		t.Fatal(device.Body.String())
	}
	id := responseID(t, device, "device")
	response := requestJSON(t, router, "GET", "/api/devices/"+id+"/config/android-native", admin, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("native credential export: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("private key response could be cached")
	}
	var setup struct {
		ClientP12      []byte `json:"client_p12"`
		RootCA         []byte `json:"root_ca"`
		ImportPassword string `json:"import_password"`
		ServerAddress  string `json:"server_address"`
		ServerIdentity string `json:"server_identity"`
		ClientIdentity string `json:"client_identity"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &setup); err != nil {
		t.Fatal(err)
	}
	if setup.ClientIdentity != "android-native-test" || setup.ServerAddress == "" ||
		setup.ServerIdentity == "" || len(setup.RootCA) == 0 || len(setup.ImportPassword) < 16 {
		t.Fatalf("incomplete native Android setup: %+v", setup)
	}
	key, cert, ca, err := pkcs12.DecodeChain(setup.ClientP12, setup.ImportPassword)
	if err != nil || key == nil || cert == nil || len(ca) == 0 || cert.Subject.CommonName != setup.ClientIdentity {
		t.Fatalf("Android PKCS#12 cannot be installed with paired password: cert=%v ca=%d err=%v", cert, len(ca), err)
	}
	beforeRevocation := requestJSON(t, router, "GET", "/api/devices/"+id+"/config/android-p12", admin, nil)
	if beforeRevocation.Code != http.StatusOK {
		t.Fatal("android-p12 compatibility alias broken")
	}
	revoked := requestJSON(t, router, "POST", "/api/devices/"+id+"/revoke", admin, nil)
	if revoked.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", revoked.Code, revoked.Body.String())
	}
	forbidden := requestJSON(t, router, "GET", "/api/devices/"+id+"/config/android-native", admin, nil)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("revoked private key exported: %d %s", forbidden.Code, forbidden.Body.String())
	}
}
