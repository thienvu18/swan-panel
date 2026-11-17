package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/db"
)

func mustStatus(t *testing.T, router *gin.Engine, method, path, token string, payload any, status int) string {
	t.Helper()
	result := requestJSON(t, router, method, path, token, payload)
	if result.Code != status {
		t.Fatalf("%s %s expected %d got %d: %s", method, path, status, result.Code, result.Body.String())
	}
	return result.Body.String()
}
func createAdminFixture(t *testing.T, router *gin.Engine, token, name string) string {
	t.Helper()
	result := requestJSON(t, router, "POST", "/api/users", token, map[string]any{
		"name": name, "email": name + "@example.invalid", "password": "Test-Secure-Pass-123!", "is_admin": false,
	})
	if result.Code != http.StatusCreated {
		t.Fatalf("create fixture user: %s", result.Body.String())
	}
	return responseID(t, result, "user")
}

func TestAdministratorCannotDemoteOrDeleteLastActiveAdministrator(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	var admin db.User
	if err := db.DB.First(&admin, "email = ?", "admin@swanpanel.internal").Error; err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "PUT", "/api/users/"+admin.ID+"/role", token, nil, http.StatusConflict)
	mustStatus(t, router, "DELETE", "/api/users/"+admin.ID, token, nil, http.StatusConflict)
	mustStatus(t, router, "PUT", "/api/users/missing/role", token, nil, http.StatusNotFound)
	mustStatus(t, router, "DELETE", "/api/users/missing", token, nil, http.StatusNotFound)
	second := createAdminFixture(t, router, token, "backup-admin")
	mustStatus(t, router, "PUT", "/api/users/"+second+"/role", token, nil, http.StatusOK)
	mustStatus(t, router, "PUT", "/api/users/"+admin.ID+"/role", token, nil, http.StatusOK)
	mustStatus(t, router, "GET", "/api/users", token, nil, http.StatusForbidden)
}

func TestConcurrentAdminDemotionsPreserveOneActiveAdministrator(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	var primary db.User
	if err := db.DB.First(&primary, "email = ?", "admin@swanpanel.internal").Error; err != nil {
		t.Fatal(err)
	}
	secondary := createAdminFixture(t, router, token, "concurrent-backup")
	mustStatus(t, router, "PUT", "/api/users/"+secondary+"/role", token, nil, http.StatusOK)
	start := make(chan struct{})
	responses := make(chan int, 2)
	for _, id := range []string{primary.ID, secondary} {
		go func(userID string) {
			<-start
			responses <- requestJSON(t, router, "PUT", "/api/users/"+userID+"/role", token, nil).Code
		}(id)
	}
	close(start)
	first, second := <-responses, <-responses
	if first == http.StatusOK && second == http.StatusOK {
		t.Fatal("concurrent requests demoted both administrators")
	}
	var active int64
	if err := db.DB.Model(&db.User{}).Where("is_admin = ? AND status = ?", true, "ACTIVE").Count(&active).Error; err != nil || active != 1 {
		t.Fatalf("last administrator invariant violated: %d %v", active, err)
	}
}

func TestUserGroupApprovalRejectsInvalidReferencesWithoutPartialMembership(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	mustStatus(t, router, "POST", "/api/users", token, map[string]any{"name": "missing"}, http.StatusBadRequest)
	uid := createAdminFixture(t, router, token, "groupmember")
	mustStatus(t, router, "POST", "/api/users", token, map[string]any{
		"name": "Duplicate", "email": "groupmember@example.invalid", "password": "Strong-Pass-123!",
	}, http.StatusConflict)
	mustStatus(t, router, "POST", "/api/users", token, map[string]any{
		"name": "Unknown", "email": "unknown@example.invalid", "password": "Strong-Pass-123!",
		"user_group_ids": []string{"missing-group"},
	}, http.StatusBadRequest)
	mustStatus(t, router, "POST", "/api/users/"+uid+"/approve", token, map[string]any{
		"user_group_ids": []string{"missing-group"},
	}, http.StatusBadRequest)
	var user db.User
	if err := db.DB.Preload("UserGroups").First(&user, "id = ?", uid).Error; err != nil {
		t.Fatal(err)
	}
	if len(user.UserGroups) != 0 {
		t.Fatal("failed group assignment partially persisted")
	}
	created := requestJSON(t, router, "POST", "/api/user-groups", token, map[string]string{
		"name": "Test Group", "description": "Access review group",
	})
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	gid := responseID(t, created, "user_group")
	mustStatus(t, router, "POST", "/api/users/"+uid+"/approve", token, map[string]any{
		"user_group_ids": []string{gid, gid},
	}, http.StatusOK)
	if err := db.DB.Preload("UserGroups").First(&user, "id = ?", uid).Error; err != nil {
		t.Fatal(err)
	}
	if len(user.UserGroups) != 1 || user.UserGroups[0].ID != gid {
		t.Fatalf("membership not deduplicated: %+v", user.UserGroups)
	}
	if groups := mustStatus(t, router, "GET", "/api/user-groups", token, nil, http.StatusOK); !strings.Contains(groups, gid) {
		t.Fatal("group missing from listing")
	}
	mustStatus(t, router, "GET", "/api/users?status=ACTIVE", token, nil, http.StatusOK)
	mustStatus(t, router, "POST", "/api/users/missing/approve", token, nil, http.StatusBadRequest)
	mustStatus(t, router, "POST", "/api/users/"+uid+"/approve", token, map[string]any{
		"user_group_ids": "invalid",
	}, http.StatusBadRequest)
	mustStatus(t, router, "DELETE", "/api/user-groups/missing", token, nil, http.StatusConflict)
	rule := requestJSON(t, router, "POST", "/api/access-rules", token, map[string]any{
		"subject_type": "user_group", "subject_id": gid, "action": "ALLOW",
		"target_type": "cidr_ip", "target_value": "10.50.0.0/24",
	})
	if rule.Code != 201 {
		t.Fatalf("policy: %s", rule.Body.String())
	}
	rid := responseID(t, rule, "access_rule")
	mustStatus(t, router, "DELETE", "/api/user-groups/"+gid, token, nil, http.StatusConflict)
	mustStatus(t, router, "DELETE", "/api/access-rules/"+rid, token, nil, http.StatusOK)
	mustStatus(t, router, "DELETE", "/api/user-groups/"+gid, token, nil, http.StatusOK)
	mustStatus(t, router, "DELETE", "/api/users/"+uid, token, nil, http.StatusOK)
}

func TestUserApprovalRejectionAndDeletionPreserveAccountState(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	candidate := db.User{ID: "pending-candidate", Name: "Pending", Email: "pending@example.invalid",
		AuthType: "oidc", Status: "PENDING"}
	if err := db.DB.Create(&candidate).Error; err != nil {
		t.Fatal(err)
	}
	mustStatus(t, router, "POST", "/api/users/"+candidate.ID+"/reject", token, nil, http.StatusOK)
	var count int64
	if err := db.DB.Model(&db.User{}).Where("id = ?", candidate.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("rejected candidate still active")
	}
	active := createAdminFixture(t, router, token, "active-reject")
	mustStatus(t, router, "POST", "/api/users/"+active+"/reject", token, nil, http.StatusOK)
	if err := db.DB.Model(&db.User{}).Where("id = ?", active).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("reject endpoint deleted active user")
	}
	mustStatus(t, router, "DELETE", "/api/users/"+active, token, nil, http.StatusOK)
}

func TestAccessPoliciesRejectInvalidSubjectsActionsAndTargets(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	uid := createAdminFixture(t, router, token, "policy-user")
	base := map[string]any{"subject_type": "user", "subject_id": uid, "action": "ALLOW",
		"target_type": "cidr_ip", "target_value": "192.0.2.1"}
	for _, tc := range []struct {
		name  string
		patch map[string]any
	}{
		{"unknown action", map[string]any{"action": "GRANT"}},
		{"unknown subject type", map[string]any{"subject_type": "external"}},
		{"unknown subject id", map[string]any{"subject_id": "missing"}},
		{"IPv6 target", map[string]any{"target_value": "2001:db8::1"}},
		{"invalid prefix", map[string]any{"target_value": "1.2.3.4/55"}},
		{"unknown target type", map[string]any{"target_type": "device"}},
		{"unknown target group", map[string]any{"target_type": "resource_group", "target_value": "missing"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{}
			for k, v := range base {
				payload[k] = v
			}
			for k, v := range tc.patch {
				payload[k] = v
			}
			mustStatus(t, router, "POST", "/api/access-rules", token, payload, http.StatusBadRequest)
		})
	}
	created := requestJSON(t, router, "POST", "/api/access-rules", token, base)
	if created.Code != http.StatusCreated {
		t.Fatal(created.Body.String())
	}
	rid := responseID(t, created, "access_rule")
	list := mustStatus(t, router, "GET", "/api/access-rules", token, nil, http.StatusOK)
	if !strings.Contains(list, rid) || !strings.Contains(list, "192.0.2.1/32") {
		t.Fatalf("IPv4 /32 policy normalization failed: %s", list)
	}
	mustStatus(t, router, "DELETE", "/api/access-rules/"+rid, token, nil, http.StatusOK)
	mustStatus(t, router, "DELETE", "/api/access-rules/"+rid, token, nil, http.StatusNotFound)
}

func TestDNSOverridesAndPartialSettingsUpdatesPreserveUnchangedFields(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	mustStatus(t, router, "POST", "/api/dns/overrides", token, map[string]any{"hostname": "a.corp"}, http.StatusBadRequest)
	mustStatus(t, router, "PUT", "/api/settings/dns", token, map[string]any{
		"magic_dns_suffix": ".example.internal", "upstream_resolvers": "127.0.0.1:153",
		"is_magic_dns_enabled": false,
	}, http.StatusOK)
	updated := mustStatus(t, router, "PUT", "/api/settings/dns", token, map[string]any{
		"upstream_resolvers": "127.0.0.1:154",
	}, http.StatusOK)
	if !strings.Contains(updated, "example.internal") || !strings.Contains(updated, "127.0.0.1:154") ||
		!strings.Contains(updated, "false") {
		t.Fatalf("partial update lost existing DNS fields: %s", updated)
	}
	mustStatus(t, router, "GET", "/api/settings/dns", token, nil, http.StatusOK)
	created := requestJSON(t, router, "POST", "/api/dns/overrides", token, map[string]any{
		"hostname": "a.corp", "target_address": "10.1.1.1",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create override: %s", created.Body.String())
	}
	id := responseID(t, created, "dns_override")
	if listing := mustStatus(t, router, "GET", "/api/dns/overrides", token, nil, http.StatusOK); !strings.Contains(listing, id) {
		t.Fatal("created DNS override missing")
	}
	mustStatus(t, router, "DELETE", "/api/dns/overrides/"+id, token, nil, http.StatusOK)
}

func TestAdminFixtureReturnsExpectedUserFields(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	result := requestJSON(t, router, "POST", "/api/users", token, map[string]any{
		"name": "Serialized", "email": "serialized@example.invalid", "password": "Test-Secure-Pass-123!",
	})
	if result.Code != 201 {
		t.Fatal(result.Body.String())
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(response["user"]), "serialized@example.invalid") {
		t.Fatal("user object missing normalized identity")
	}
}

func TestLocalAdminCannotBeCreatedWithWeakPassword(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)
	for _, password := range []string{"x", "short123", strings.Repeat("x", 73)} {
		res := requestJSON(t, router, "POST", "/api/users", token, map[string]any{
			"name": "Weak Administrator", "email": "weak-admin@example.invalid",
			"password": password, "is_admin": true,
		})
		if res.Code != http.StatusBadRequest {
			t.Fatalf("weak admin password length %d accepted: %d %s", len(password), res.Code, res.Body.String())
		}
	}
	var count int64
	if err := db.DB.Model(&db.User{}).Where("email = ?", "weak-admin@example.invalid").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("weak admin was persisted: count=%d, error=%v", count, err)
	}
}
