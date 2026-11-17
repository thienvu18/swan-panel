package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
)

func caRotatePayload() map[string]any {
	return map[string]any{
		"confirm": "ROTATE CA", "common_name": "P1 Candidate Root",
		"organization": "SwanPanel Test", "country": "VN",
		"province": "Ho Chi Minh", "locality": "District 1",
	}
}

func TestRootCARotationRejectsUnavailableDatabaseAndBlankIdentity(t *testing.T) {
	t.Run("missing device inventory", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		admin := getAdminToken(t, router)
		if err := db.DB.Migrator().DropTable(&db.Device{}); err != nil {
			t.Fatal(err)
		}
		mustStatus(t, router, "POST", "/api/settings/ca/rotate", admin, caRotatePayload(), http.StatusServiceUnavailable)
	})
	t.Run("missing revocation ledger", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		admin := getAdminToken(t, router)
		if err := db.DB.Migrator().DropTable(&db.CertificateRevocation{}); err != nil {
			t.Fatal(err)
		}
		mustStatus(t, router, "POST", "/api/settings/ca/rotate", admin, caRotatePayload(), http.StatusServiceUnavailable)
	})
	t.Run("malformed CA identity", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		admin := getAdminToken(t, router)
		payload := caRotatePayload()
		payload["common_name"] = "  "
		mustStatus(t, router, "POST", "/api/settings/ca/rotate", admin, payload, http.StatusServiceUnavailable)
	})
	t.Run("oversized CA locality", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		admin := getAdminToken(t, router)
		payload := caRotatePayload()
		payload["locality"] = strings.Repeat("z", 129)
		mustStatus(t, router, "POST", "/api/settings/ca/rotate", admin, payload, http.StatusBadRequest)
	})
}

func TestRootCARotationPreservesCommittedIdentityWhenGatewayCannotApply(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	// The root itself can rotate, but the subsequent gateway identity will
	// fail. The HTTP response must report pending, not claim rollback.
	if err := db.DB.Model(&db.StrongswanSettings{}).Where("id = ?", 1).Update("server_identity", "").Error; err != nil {
		t.Fatal(err)
	}
	response := mustStatus(t, router, "POST", "/api/settings/ca/rotate", admin, caRotatePayload(), http.StatusAccepted)
	if !strings.Contains(response, "pending") || !strings.Contains(response, "P1 Candidate Root") {
		t.Fatalf("rotated signer not reported as pending: %s", response)
	}
	stored := mustStatus(t, router, "GET", "/api/settings/ca", admin, nil, 200)
	if !strings.Contains(stored, "P1 Candidate Root") {
		t.Fatal("committed CA identity vanished after gateway error")
	}
}

func TestRootCARotationRecordsPendingWhenRuntimeStateTableIsMissing(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	if err := db.DB.Migrator().DropTable(&db.RuntimeApplyState{}); err != nil {
		t.Fatal(err)
	}
	response := mustStatus(t, router, "POST", "/api/settings/ca/rotate", admin, caRotatePayload(), http.StatusAccepted)
	if !strings.Contains(response, "pending") {
		t.Fatalf("failed status persistence hidden: %s", response)
	}
}
