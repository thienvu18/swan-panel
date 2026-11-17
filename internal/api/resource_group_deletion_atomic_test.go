package api_test

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/swan-panel/swan-panel/internal/db"
)

func TestResourceGroupDeleteIsAtomicAndMissingGroupsReturnNotFound(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	group := requestJSON(t, router, "POST", "/api/resource-groups", admin,
		map[string]any{"name": "Atomic deletion empty subnet", "cidr_block": "10.217.0.0/24"})
	if group.Code != http.StatusCreated {
		t.Fatal(group.Body.String())
	}
	id := responseID(t, group, "resource_group")
	mustStatus(t, router, "DELETE", "/api/resource-groups/"+id, admin, nil, http.StatusOK)
	mustStatus(t, router, "DELETE", "/api/resource-groups/"+id, admin, nil, http.StatusNotFound)
	mustStatus(t, router, "DELETE", "/api/resource-groups/never-created", admin, nil, http.StatusNotFound)
}

func TestResourceGroupDeleteRollsBackOnStorageFailure(t *testing.T) {
	cases := []struct {
		name   string
		inject func(t *testing.T)
	}{
		{
			name: "device inventory count fails",
			inject: func(t *testing.T) {
				if err := db.DB.Migrator().DropTable(&db.Device{}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "rule reference count fails",
			inject: func(t *testing.T) {
				if err := db.DB.Migrator().DropTable(&db.AccessRule{}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "DELETE statement aborts",
			inject: func(t *testing.T) {
				if err := db.DB.Exec("CREATE TRIGGER deny_subnet_delete BEFORE DELETE ON resource_groups BEGIN SELECT RAISE(ABORT, 'deletion denied'); END;").Error; err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, cleanup := setupTestRouter(t)
			defer cleanup()
			admin := getAdminToken(t, router)
			group := requestJSON(t, router, "POST", "/api/resource-groups", admin,
				map[string]any{"name": "Storage fault subnet", "cidr_block": "10.218.0.0/24"})
			if group.Code != http.StatusCreated {
				t.Fatal(group.Body.String())
			}
			id := responseID(t, group, "resource_group")
			tc.inject(t)
			mustStatus(t, router, "DELETE", "/api/resource-groups/"+id, admin, nil, http.StatusServiceUnavailable)
			var count int64
			if err := db.DB.Model(&db.ResourceGroup{}).Where("id = ?", id).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("storage error must not remove group: count=%d error=%v", count, err)
			}
		})
	}
}

func TestConcurrentRuleCreationAndSubnetDeletionNeverLeavesOrphanedTarget(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	userID := createAdminFixture(t, router, admin, "subnet-race-subject")
	for i := 0; i < 12; i++ {
		group := requestJSON(t, router, "POST", "/api/resource-groups", admin,
			map[string]any{"name": fmt.Sprintf("Access race %d", i),
				"cidr_block": fmt.Sprintf("10.219.%d.0/24", i)})
		if group.Code != http.StatusCreated {
			t.Fatal(group.Body.String())
		}
		id := responseID(t, group, "resource_group")

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var ruleCode, deleteCode int
		go func() {
			defer wg.Done()
			<-start
			ruleCode = requestJSON(t, router, "POST", "/api/access-rules", admin,
				map[string]any{"subject_type": "user", "subject_id": userID,
					"action": "ALLOW", "target_type": "resource_group", "target_value": id}).Code
		}()
		go func() {
			defer wg.Done()
			<-start
			deleteCode = requestJSON(t, router, "DELETE", "/api/resource-groups/"+id, admin, nil).Code
		}()
		close(start)
		wg.Wait()

		var groupCount, ruleCount int64
		if err := db.DB.Model(&db.ResourceGroup{}).Where("id = ?", id).Count(&groupCount).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.DB.Model(&db.AccessRule{}).Where("target_type = ? AND target_value = ?", "resource_group", id).Count(&ruleCount).Error; err != nil {
			t.Fatal(err)
		}
		if groupCount == 0 && ruleCount != 0 {
			t.Fatalf("orphaned rule created after subnet removal (iteration %d, create=%d, delete=%d)", i, ruleCode, deleteCode)
		}
		if groupCount > 0 && ruleCount == 0 {
			t.Fatalf("deletion did not complete on an unreferenced subnet (iteration %d, create=%d, delete=%d)", i, ruleCode, deleteCode)
		}
		if groupCount == 0 && deleteCode != http.StatusOK {
			t.Fatalf("deleted subnet returned HTTP %d (iteration %d)", deleteCode, i)
		}
		if ruleCount != 0 && deleteCode != http.StatusConflict {
			t.Fatalf("referenced subnet was deleted (iteration %d, create=%d, delete=%d)", i, ruleCode, deleteCode)
		}
	}
}

func TestConcurrentDeviceEnrollmentAndSubnetDeletionNeverLeavesOrphanedDevice(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	for i := 0; i < 3; i++ {
		group := requestJSON(t, router, "POST", "/api/resource-groups", admin,
			map[string]any{"name": fmt.Sprintf("Enrollment race %d", i),
				"cidr_block": fmt.Sprintf("10.220.%d.0/24", i)})
		if group.Code != http.StatusCreated {
			t.Fatal(group.Body.String())
		}
		id := responseID(t, group, "resource_group")
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var enrollCode, deleteCode int
		go func() {
			defer wg.Done()
			<-start
			enrollCode = requestJSON(t, router, "POST", "/api/devices", admin,
				map[string]any{"hostname": fmt.Sprintf("enrollment-race-%d", i), "device_type": "company",
					"resource_group_id": id, "platform": "linux"}).Code
		}()
		go func() {
			defer wg.Done()
			<-start
			deleteCode = requestJSON(t, router, "DELETE", "/api/resource-groups/"+id, admin, nil).Code
		}()
		close(start)
		wg.Wait()
		var groupCount, deviceCount int64
		if err := db.DB.Model(&db.ResourceGroup{}).Where("id = ?", id).Count(&groupCount).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.DB.Model(&db.Device{}).Where("resource_group_id = ?", id).Count(&deviceCount).Error; err != nil {
			t.Fatal(err)
		}
		if groupCount == 0 && deviceCount != 0 {
			t.Fatalf("orphaned device created after subnet removal (iteration %d, enrollment=%d, delete=%d)", i, enrollCode, deleteCode)
		}
		if groupCount == 0 && deleteCode != http.StatusOK {
			t.Fatalf("subnet deletion returned HTTP %d (iteration %d)", deleteCode, i)
		}
		if deviceCount > 0 && deleteCode != http.StatusConflict {
			t.Fatalf("subnet with enrolled device deleted (iteration %d, enroll=%d delete=%d)", i, enrollCode, deleteCode)
		}
	}
}
