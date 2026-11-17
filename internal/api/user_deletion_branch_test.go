package api_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

func rejectCountQueries(t *testing.T) {
	t.Helper()
	const name = "test:reject-admin-count"
	if err := db.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*int64); ok &&
			strings.Contains(fmt.Sprint(tx.Statement.Clauses["WHERE"]), "is_admin") {
			_ = tx.AddError(errors.New("count query unavailable"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.DB.Callback().Query().Remove(name)
	})
}

func createDeviceForUserDeletion(t *testing.T, userID, suffix, serial string, revoked bool) {
	t.Helper()
	group := db.ResourceGroup{ID: "deletion-net-" + suffix, Name: "Deletion net " + suffix,
		CIDRBlock: "10.203.0.0/24"}
	if err := db.DB.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	device := db.Device{ID: "deletion-device-" + suffix, Hostname: "deletion-host-" + suffix,
		DeviceType: "user", UserID: &userID, ResourceGroupID: group.ID,
		IPAllocationType: "static", VirtualIP: "10.203.0.3", Platform: "linux",
		CertSerial: serial, IsRevoked: revoked}
	if err := db.DB.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
}

func TestAdministratorMutationsFailClosedWhenLastAdminCountCannotBeRead(t *testing.T) {
	t.Run("role mutation", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		token := getAdminToken(t, router)
		user := db.User{}
		if err := db.DB.First(&user, "email = ?", "admin@swanpanel.internal").Error; err != nil {
			t.Fatal(err)
		}
		rejectCountQueries(t)
		mustStatus(t, router, "PUT", "/api/users/"+user.ID+"/role", token, nil, 503)
	})
	t.Run("account deletion", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		token := getAdminToken(t, router)
		user := db.User{}
		if err := db.DB.First(&user, "email = ?", "admin@swanpanel.internal").Error; err != nil {
			t.Fatal(err)
		}
		rejectCountQueries(t)
		mustStatus(t, router, "DELETE", "/api/users/"+user.ID, token, nil, 503)
	})
}

func TestUserDeletionRollsBackWhenRevocationOrMembershipPersistenceFails(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		suffix  string
		prepare func(*testing.T)
	}{
		{"mark revoked fails", "revoked-write", func(t *testing.T) {
			if err := db.DB.Exec("CREATE TRIGGER block_revoked_update BEFORE UPDATE OF is_revoked ON devices " +
				"BEGIN SELECT RAISE(ABORT, 'cannot revoke'); END").Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"revocation ledger unavailable", "rev-ledger", func(t *testing.T) {
			if err := db.DB.Migrator().DropTable(&db.CertificateRevocation{}); err != nil {
				t.Fatal(err)
			}
		}},
		{"user membership ledger unavailable", "membership", func(t *testing.T) {
			if err := db.DB.Migrator().DropTable("user_group_members"); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			router, cleanup := setupTestRouter(t)
			defer cleanup()
			admin := getAdminToken(t, router)
			uid := createAdminFixture(t, router, admin, "deletion-"+scenario.suffix)
			createDeviceForUserDeletion(t, uid, scenario.suffix, "abc123", false)
			scenario.prepare(t)
			mustStatus(t, router, "DELETE", "/api/users/"+uid, admin, nil, 503)
			var user db.User
			if err := db.DB.First(&user, "id = ?", uid).Error; err != nil {
				t.Fatal("failed deletion removed user:", err)
			}
		})
	}
}

func TestUserDeletionPreservesAlreadyRevokedLegacyDeviceAndReportsFailedEnforcement(t *testing.T) {
	t.Run("already revoked legacy identity without serial", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		admin := getAdminToken(t, router)
		uid := createAdminFixture(t, router, admin, "legacy-revoked")
		createDeviceForUserDeletion(t, uid, "legacy-revoked", "", true)
		mustStatus(t, router, "DELETE", "/api/users/"+uid, admin, nil, 200)
	})
	t.Run("revocation ledger contains corrupt historical serial", func(t *testing.T) {
		router, cleanup := setupTestRouter(t)
		defer cleanup()
		admin := getAdminToken(t, router)
		uid := createAdminFixture(t, router, admin, "bad-history")
		createDeviceForUserDeletion(t, uid, "bad-history", "abc123", false)
		if err := db.DB.Create(&db.CertificateRevocation{Serial: "invalid-serial", DeviceID: "old-device"}).Error; err != nil {
			t.Fatal(err)
		}
		result := mustStatus(t, router, "DELETE", "/api/users/"+uid, admin, nil, http.StatusAccepted)
		if !strings.Contains(result, "pending") {
			t.Fatalf("failed post-commit revocation was hidden: %s", result)
		}
	})
}
