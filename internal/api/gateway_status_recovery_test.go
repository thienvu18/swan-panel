package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
)

func TestApplyStatusReportsUncommittedDriftAndAppliedGeneration(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	mustStatus(t, router, "POST", "/api/system/apply", token, nil, http.StatusOK)
	response := mustStatus(t, router, "GET", "/api/system/apply-status", token, nil, http.StatusOK)
	if !strings.Contains(response, "\"status\":\"applied\"") {
		t.Fatalf("successful gateway apply not reported: %s", response)
	}
	if err := db.DB.Model(&db.StrongswanSettings{}).Where("id = ?", 1).Update("dpd_delay", "15s").Error; err != nil {
		t.Fatal(err)
	}
	drifted := mustStatus(t, router, "GET", "/api/system/apply-status", token, nil, http.StatusOK)
	if !strings.Contains(drifted, "\"status\":\"pending\"") {
		t.Fatalf("desired state drift not reported: %s", drifted)
	}
	mustStatus(t, router, "POST", "/api/system/apply", token, nil, http.StatusOK)
	response = mustStatus(t, router, "GET", "/api/system/apply-status", token, nil, http.StatusOK)
	if !strings.Contains(response, "\"status\":\"applied\"") {
		t.Fatalf("retry did not converge desired state: %s", response)
	}
}

func TestApplyStatusRejectsMissingPersistentApplyState(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	if err := db.DB.Migrator().DropTable(&db.RuntimeApplyState{}); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "GET", "/api/system/apply-status", token, nil, http.StatusServiceUnavailable)
}

func TestRetryApplyFailsWhenGatewayConfigurationCannotBeLoaded(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	if err := db.DB.Migrator().DropTable(&db.StrongswanSettings{}); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "POST", "/api/system/apply", token, nil, http.StatusServiceUnavailable)
}
func TestCAIdentityStatusRefusesIncompleteCertificateInventories(t *testing.T) {
	for _, model := range []string{"devices", "certificate_revocations"} {
		t.Run(model, func(t *testing.T) {
			router, cleanup := setupTestRouter(t)
			defer cleanup()
			token := getAdminToken(t, router)
			if err := db.DB.Exec("DROP TABLE " + model).Error; err != nil {
				t.Fatal(err)
			}
			mustStatus(t, router, "GET", "/api/settings/ca", token, nil, http.StatusServiceUnavailable)
		})
	}
}

func TestApplyStatusRejectsUnavailableDeviceInventoryAfterConvergence(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	mustStatus(t, router, "POST", "/api/system/apply", token, nil, http.StatusOK)
	if err := db.DB.Migrator().DropTable(&db.Device{}); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "GET", "/api/system/apply-status", token, nil, http.StatusServiceUnavailable)
}
