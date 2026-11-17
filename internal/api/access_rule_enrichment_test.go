package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
)

func TestAccessRuleDirectoryResolvesSubjectsAndResourceTargets(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	ownerID := createAdminFixture(t, router, admin, "access-subject-user")
	team := db.UserGroup{ID: "access-team", Name: "Network operators"}
	subnet := db.ResourceGroup{ID: "access-target", Name: "Internal applications", CIDRBlock: "10.172.0.0/24"}
	if err := db.DB.Create(&team).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Create(&subnet).Error; err != nil {
		t.Fatal(err)
	}
	device := db.Device{
		ID: "access-device", Hostname: "access-laptop", DeviceType: "user",
		UserID: &ownerID, ResourceGroupID: subnet.ID, IPAllocationType: "static",
		VirtualIP: "10.172.0.6", Platform: "linux",
	}
	if err := db.DB.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range []db.AccessRule{
		{ID: "user-allow", SubjectType: "user", SubjectID: ownerID, Action: "ALLOW",
			TargetType: "resource_group", TargetValue: subnet.ID, RuleNature: "direct_grant"},
		{ID: "team-allow", SubjectType: "user_group", SubjectID: team.ID, Action: "ALLOW",
			TargetType: "cidr_ip", TargetValue: "10.180.0.0/24", RuleNature: "group_policy"},
		{ID: "laptop-deny", SubjectType: "device", SubjectID: device.ID, Action: "DENY",
			TargetType: "cidr_ip", TargetValue: "10.190.0.0/24", RuleNature: "user_exclusion"},
		{ID: "orphaned-rule", SubjectType: "device", SubjectID: "missing-device", Action: "DENY",
			TargetType: "resource_group", TargetValue: "missing-group", RuleNature: "user_exclusion"},
	} {
		if err := db.DB.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	data := mustStatus(t, router, "GET", "/api/access-rules", admin, nil, http.StatusOK)
	for _, expected := range []string{
		"access-subject-user", "Network operators", "access-laptop",
		"Internal applications (10.172.0.0/24)", "missing-device", "missing-group",
	} {
		if !strings.Contains(data, expected) {
			t.Errorf("access rule directory did not render %q", expected)
		}
	}
}
func TestAccessRuleDirectoryAndDeletionReturnErrorsOnBrokenStorage(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	mustStatus(t, router, "DELETE", "/api/access-rules/missing", admin, nil, 404)
	if err := db.DB.Migrator().DropTable(&db.AccessRule{}); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "GET", "/api/access-rules", admin, nil, http.StatusInternalServerError)
	mustStatus(t, router, "DELETE", "/api/access-rules/missing", admin, nil, http.StatusInternalServerError)
}
