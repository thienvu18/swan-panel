package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	swanpanel "github.com/swan-panel/swan-panel"
	"github.com/swan-panel/swan-panel/internal/api"
	"github.com/swan-panel/swan-panel/internal/config"
	"github.com/swan-panel/swan-panel/internal/db"
)

func setupTestRouter(t *testing.T) (*gin.Engine, func()) {
	gin.SetMode(gin.TestMode)
	testDBPath := "test_swanpanel.db"

	cfg := &config.Config{
		Port:        8080,
		DBPath:      testDBPath,
		JWTSecret:   "test-secret-key-12345",
		Environment: "test",
	}

	_, err := db.InitDB(testDBPath)
	if err != nil {
		t.Fatalf("Failed to initialize test DB: %v", err)
	}

	router := api.SetupRouter(cfg, nil)

	cleanup := func() {
		sqlDB, _ := db.DB.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		_ = os.Remove(testDBPath)
		_ = os.Remove(testDBPath + "-wal")
		_ = os.Remove(testDBPath + "-shm")
	}

	return router, cleanup
}

func getAdminToken(t *testing.T, router *gin.Engine) string {
	loginPayload := map[string]string{
		"email":    "admin@swanpanel.internal",
		"password": "admin123456",
	}
	body, _ := json.Marshal(loginPayload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Admin login failed: %d, body: %s", w.Code, w.Body.String())
	}

	var loginRes map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &loginRes)
	return loginRes["token"].(string)
}

func TestHealthCheck(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got: %d", w.Code)
	}

	var res map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got: %v", res["status"])
	}
	if res["service"] != "SwanPanel" {
		t.Errorf("Expected service 'SwanPanel', got: %v", res["service"])
	}
}

func TestSecuritySQLInjectionPrevention(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()

	// Attempt classic SQL injection payload in email field
	attackPayload := map[string]string{
		"email":    "' OR '1'='1' --",
		"password": "random_password",
	}
	body, _ := json.Marshal(attackPayload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// Must be rejected
	if w.Code == http.StatusOK {
		t.Fatalf("SECURITY VULNERABILITY: SQL Injection payload bypassed authentication!")
	}
}

func TestLoginAndProtectedStatsFlow(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()

	// 1. Attempt access without token -> 401
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/api/dashboard/stats", nil)
	router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 without auth token, got: %d", w1.Code)
	}

	// 2. Login as admin
	token := getAdminToken(t, router)

	// 3. Access stats with valid Bearer Token -> 200 OK
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/dashboard/stats", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Errorf("Expected 200 with valid Bearer token, got: %d", w3.Code)
	}

	var statsRes map[string]interface{}
	_ = json.Unmarshal(w3.Body.Bytes(), &statsRes)
	stats, ok := statsRes["stats"].(map[string]interface{})
	if !ok || stats["gateway_status"] != "online" {
		t.Errorf("Stats payload incorrect: %v", statsRes)
	}
}

func TestOIDCProvidersCRUD(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)

	// 1. Create OIDC Provider
	createPayload := map[string]string{
		"name":          "Google Test SSO",
		"provider_type": "google",
		"issuer_url":    "https://accounts.google.com",
		"client_id":     "google-client-id-12345",
		"client_secret": "google-client-secret-abcde",
	}
	body, _ := json.Marshal(createPayload)
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("POST", "/api/oidc/providers", bytes.NewBuffer(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusCreated {
		t.Fatalf("Failed to create OIDC provider: %d, body: %s", w1.Code, w1.Body.String())
	}

	var createRes map[string]interface{}
	_ = json.Unmarshal(w1.Body.Bytes(), &createRes)
	p := createRes["provider"].(map[string]interface{})
	idpID := p["id"].(string)

	// 2. List Providers
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/oidc/providers", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("Failed to list OIDC providers: %d", w2.Code)
	}

	// 3. Delete Provider
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("DELETE", "/api/oidc/providers/"+idpID, nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("Failed to delete OIDC provider: %d", w3.Code)
	}
}

func TestUsersAndApprovalQueueFlow(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)

	// 1. Create User Group
	ugPayload := map[string]string{"name": "Security Engineers", "description": "SecOps"}
	bodyUG, _ := json.Marshal(ugPayload)
	wUG := httptest.NewRecorder()
	reqUG, _ := http.NewRequest("POST", "/api/user-groups", bytes.NewBuffer(bodyUG))
	reqUG.Header.Set("Content-Type", "application/json")
	reqUG.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wUG, reqUG)
	if wUG.Code != http.StatusCreated {
		t.Fatalf("Failed to create user group: %d", wUG.Code)
	}

	// 2. Create Local User (US-03B)
	userPayload := map[string]interface{}{
		"name":     "Alice Local",
		"email":    "alice@company.com",
		"password": "Password123!",
		"is_admin": false,
	}
	bodyUser, _ := json.Marshal(userPayload)
	wUser := httptest.NewRecorder()
	reqUser, _ := http.NewRequest("POST", "/api/users", bytes.NewBuffer(bodyUser))
	reqUser.Header.Set("Content-Type", "application/json")
	reqUser.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wUser, reqUser)
	if wUser.Code != http.StatusCreated {
		t.Fatalf("Failed to create local user: %d, body: %s", wUser.Code, wUser.Body.String())
	}

	var userRes map[string]interface{}
	_ = json.Unmarshal(wUser.Body.Bytes(), &userRes)
	u := userRes["user"].(map[string]interface{})
	aliceID := u["id"].(string)

	// 3. Promote Alice to Admin (US-03)
	wRole := httptest.NewRecorder()
	reqRole, _ := http.NewRequest("PUT", "/api/users/"+aliceID+"/role", nil)
	reqRole.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wRole, reqRole)
	if wRole.Code != http.StatusOK {
		t.Fatalf("Failed to toggle admin role: %d", wRole.Code)
	}

	var roleRes map[string]interface{}
	_ = json.Unmarshal(wRole.Body.Bytes(), &roleRes)
	if roleRes["is_admin"] != true {
		t.Errorf("Expected is_admin to be true after toggle")
	}

	// 4. Test OIDC Candidate Auto-Enrollment into Approval Queue (US-02)
	wEnroll := httptest.NewRecorder()
	reqEnroll, _ := http.NewRequest("GET", "/api/auth/oidc/callback?email=candidate@corp.com&name=Candidate+Bao", nil)
	router.ServeHTTP(wEnroll, reqEnroll)
	if wEnroll.Code != http.StatusCreated {
		t.Fatalf("Expected candidate enrollment 201, got: %d", wEnroll.Code)
	}

	var enrollRes map[string]interface{}
	_ = json.Unmarshal(wEnroll.Body.Bytes(), &enrollRes)
	candidate := enrollRes["user"].(map[string]interface{})
	candidateID := candidate["id"].(string)

	if candidate["status"] != "PENDING" {
		t.Errorf("Expected candidate status to be PENDING, got: %v", candidate["status"])
	}

	// 5. Approve Candidate (US-02)
	wApprove := httptest.NewRecorder()
	reqApprove, _ := http.NewRequest("POST", "/api/users/"+candidateID+"/approve", nil)
	reqApprove.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wApprove, reqApprove)
	if wApprove.Code != http.StatusOK {
		t.Fatalf("Failed to approve candidate: %d", wApprove.Code)
	}
}

func TestResourcesAndDeviceProvisioningFlow(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)

	// 1. Create Resource Group (Subnet)
	rgPayload := map[string]string{
		"name":        "Test Subnet",
		"cidr_block":  "10.200.0.0/24",
		"description": "Test environment subnet",
	}
	bodyRG, _ := json.Marshal(rgPayload)
	wRG := httptest.NewRecorder()
	reqRG, _ := http.NewRequest("POST", "/api/resource-groups", bytes.NewBuffer(bodyRG))
	reqRG.Header.Set("Content-Type", "application/json")
	reqRG.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wRG, reqRG)
	if wRG.Code != http.StatusCreated {
		t.Fatalf("Failed to create resource group: %d", wRG.Code)
	}

	var rgRes map[string]interface{}
	_ = json.Unmarshal(wRG.Body.Bytes(), &rgRes)
	rg := rgRes["resource_group"].(map[string]interface{})
	rgID := rg["id"].(string)

	// 2. Provision Company Node with Dynamic IP (US-04)
	devPayload := map[string]string{
		"hostname":           "srv-backend-01",
		"device_type":        "company",
		"resource_group_id":  rgID,
		"ip_allocation_type": "dynamic",
		"platform":           "linux",
	}
	bodyDev, _ := json.Marshal(devPayload)
	wDev := httptest.NewRecorder()
	reqDev, _ := http.NewRequest("POST", "/api/devices", bytes.NewBuffer(bodyDev))
	reqDev.Header.Set("Content-Type", "application/json")
	reqDev.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wDev, reqDev)
	if wDev.Code != http.StatusCreated {
		t.Fatalf("Failed to create device: %d, body: %s", wDev.Code, wDev.Body.String())
	}

	var devRes map[string]interface{}
	_ = json.Unmarshal(wDev.Body.Bytes(), &devRes)
	dev := devRes["device"].(map[string]interface{})
	devID := dev["id"].(string)

	if !strings.HasPrefix(dev["virtual_ip"].(string), "10.200.0.") {
		t.Errorf("Expected dynamic IP in 10.200.0.x, got: %v", dev["virtual_ip"])
	}

	// 3. Download Apple .mobileconfig Profile (US-05)
	wApple := httptest.NewRecorder()
	reqApple, _ := http.NewRequest("GET", "/api/devices/"+devID+"/config/apple", nil)
	reqApple.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wApple, reqApple)
	if wApple.Code != http.StatusOK {
		t.Fatalf("Failed to download Apple mobileconfig: %d", wApple.Code)
	}
	if !strings.Contains(wApple.Body.String(), "com.apple.vpn.managed") {
		t.Errorf("Apple profile missing com.apple.vpn.managed payload")
	}

	// 4. Download Windows PowerShell Script (US-05)
	wWin := httptest.NewRecorder()
	reqWin, _ := http.NewRequest("GET", "/api/devices/"+devID+"/config/windows", nil)
	reqWin.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wWin, reqWin)
	if wWin.Code != http.StatusOK {
		t.Fatalf("Failed to download Windows script: %d", wWin.Code)
	}
	if !strings.Contains(wWin.Body.String(), "Add-VpnConnection") {
		t.Errorf("PowerShell script missing Add-VpnConnection")
	}

	// 5. Revoke Device (US-18)
	wRevoke := httptest.NewRecorder()
	reqRevoke, _ := http.NewRequest("POST", "/api/devices/"+devID+"/revoke", nil)
	reqRevoke.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wRevoke, reqRevoke)
	if wRevoke.Code != http.StatusOK {
		t.Fatalf("Failed to revoke device: %d", wRevoke.Code)
	}
}

func TestAccessRulesPolicyFlow(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)

	// Create Exclusion DENY Rule (US-07)
	rulePayload := map[string]string{
		"subject_type": "user",
		"subject_id":   "usr_test",
		"action":       "DENY",
		"target_type":  "cidr_ip",
		"target_value": "10.99.0.0/24",
	}
	body, _ := json.Marshal(rulePayload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/access-rules", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create access rule: %d, body: %s", w.Code, w.Body.String())
	}

	// List rules
	wList := httptest.NewRecorder()
	reqList, _ := http.NewRequest("GET", "/api/access-rules", nil)
	reqList.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("Failed to list rules: %d", wList.Code)
	}
}

func TestDNSSettingsAndOverridesFlow(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)

	// 1. Update MagicDNS suffix & Upstreams (US-09)
	dnsPayload := map[string]string{
		"magic_dns_suffix":   ".internal.corp",
		"upstream_resolvers": "9.9.9.9, 1.1.1.1",
	}
	body, _ := json.Marshal(dnsPayload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/settings/dns", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to update DNS settings: %d", w.Code)
	}

	// 2. Create Custom DNS Override (US-09)
	overridePayload := map[string]string{
		"hostname":       "wiki.internal.corp",
		"record_type":    "A",
		"target_address": "10.0.1.50",
	}
	bodyO, _ := json.Marshal(overridePayload)
	wO := httptest.NewRecorder()
	reqO, _ := http.NewRequest("POST", "/api/dns/overrides", bytes.NewBuffer(bodyO))
	reqO.Header.Set("Content-Type", "application/json")
	reqO.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wO, reqO)
	if wO.Code != http.StatusCreated {
		t.Fatalf("Failed to create DNS override: %d", wO.Code)
	}
}

func TestSessionsAndDaemonControlFlow(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	token := getAdminToken(t, router)

	// 1. List active sessions (US-10)
	wSess := httptest.NewRecorder()
	reqSess, _ := http.NewRequest("GET", "/api/sessions", nil)
	reqSess.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wSess, reqSess)
	if wSess.Code != http.StatusOK {
		t.Fatalf("Failed to list active sessions: %d", wSess.Code)
	}

	// 2. Terminate session (US-10)
	termPayload := map[string]string{"ike_name": "ikev2-thienvu-macbook"}
	bodyTerm, _ := json.Marshal(termPayload)
	wTerm := httptest.NewRecorder()
	reqTerm, _ := http.NewRequest("POST", "/api/sessions/terminate", bytes.NewBuffer(bodyTerm))
	reqTerm.Header.Set("Content-Type", "application/json")
	reqTerm.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wTerm, reqTerm)
	if wTerm.Code != http.StatusOK {
		t.Fatalf("Failed to terminate session: %d", wTerm.Code)
	}

	// 3. Daemon Control: Restart (US-11)
	daemonPayload := map[string]string{"action": "restart"}
	bodyD, _ := json.Marshal(daemonPayload)
	wD := httptest.NewRecorder()
	reqD, _ := http.NewRequest("POST", "/api/system/daemon", bytes.NewBuffer(bodyD))
	reqD.Header.Set("Content-Type", "application/json")
	reqD.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wD, reqD)
	if wD.Code != http.StatusOK {
		t.Fatalf("Failed to control daemon: %d", wD.Code)
	}

	// 4. Query Contextual Logs with Filter (US-12)
	wLogs := httptest.NewRecorder()
	reqLogs, _ := http.NewRequest("GET", "/api/system/logs?scope=dev:thienvu-macbook&level=info", nil)
	reqLogs.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wLogs, reqLogs)
	if wLogs.Code != http.StatusOK {
		t.Fatalf("Failed to query contextual logs: %d", wLogs.Code)
	}
}

func TestEmbeddedStaticServing(t *testing.T) {
	staticFS, err := swanpanel.StaticFS()
	if err != nil {
		t.Fatalf("Failed to load staticFS: %v", err)
	}
	testDBPath := "test_static.db"
	cfg := &config.Config{
		Port:        8080,
		DBPath:      testDBPath,
		JWTSecret:   "test-secret-12345",
		Environment: "test",
	}
	_, _ = db.InitDB(testDBPath)
	defer func() {
		sqlDB, _ := db.DB.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		_ = os.Remove(testDBPath)
		_ = os.Remove(testDBPath + "-wal")
		_ = os.Remove(testDBPath + "-shm")
	}()

	router := api.SetupRouter(cfg, staticFS)

	// 1. Test GET / (Root SPA)
	wRoot := httptest.NewRecorder()
	reqRoot, _ := http.NewRequest("GET", "/", nil)
	router.ServeHTTP(wRoot, reqRoot)
	if wRoot.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for /, got %d: %s", wRoot.Code, wRoot.Body.String())
	}
	if !strings.Contains(wRoot.Body.String(), "<html") && !strings.Contains(wRoot.Body.String(), "<!doctype html") && !strings.Contains(wRoot.Body.String(), "<!DOCTYPE html") {
		t.Fatalf("Expected HTML response for /, got: %s", wRoot.Body.String())
	}

	// 2. Test GET /resources (Client-side route fallback to index.html)
	wSPA := httptest.NewRecorder()
	reqSPA, _ := http.NewRequest("GET", "/resources", nil)
	router.ServeHTTP(wSPA, reqSPA)
	if wSPA.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for /resources, got %d: %s", wSPA.Code, wSPA.Body.String())
	}
	if !strings.Contains(wSPA.Body.String(), "<html") && !strings.Contains(wSPA.Body.String(), "<!doctype html") && !strings.Contains(wSPA.Body.String(), "<!DOCTYPE html") {
		t.Fatalf("Expected HTML response for /resources, got: %s", wSPA.Body.String())
	}

	// 3. Test GET /api/nonexistent (should return 404 JSON, not HTML)
	wAPI := httptest.NewRecorder()
	reqAPI, _ := http.NewRequest("GET", "/api/nonexistent", nil)
	router.ServeHTTP(wAPI, reqAPI)
	if wAPI.Code != http.StatusNotFound {
		t.Fatalf("Expected 404 for nonexistent API route, got %d", wAPI.Code)
	}
	if !strings.Contains(wAPI.Body.String(), "API route not found") {
		t.Fatalf("Expected JSON error message, got: %s", wAPI.Body.String())
	}
}

func TestSecurityAuditRemediations(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()

	adminToken := getAdminToken(t, router)

	// 1. Create a standard (non-admin) user
	userPayload := map[string]interface{}{
		"email":    "alice@example.com",
		"name":     "Alice Employee",
		"password": "alicepassword123",
		"is_admin": false,
	}
	bodyUser, _ := json.Marshal(userPayload)
	wUser := httptest.NewRecorder()
	reqUser, _ := http.NewRequest("POST", "/api/users", bytes.NewBuffer(bodyUser))
	reqUser.Header.Set("Content-Type", "application/json")
	reqUser.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(wUser, reqUser)
	if wUser.Code != http.StatusCreated {
		t.Fatalf("Failed to create standard user: %d, body: %s", wUser.Code, wUser.Body.String())
	}

	var createdUserRes map[string]interface{}
	_ = json.Unmarshal(wUser.Body.Bytes(), &createdUserRes)
	aliceID := createdUserRes["user"].(map[string]interface{})["id"].(string)

	// Login as Alice to get standard user token
	loginPayload := map[string]string{
		"email":    "alice@example.com",
		"password": "alicepassword123",
	}
	bodyLogin, _ := json.Marshal(loginPayload)
	wLogin := httptest.NewRecorder()
	reqLogin, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(bodyLogin))
	reqLogin.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wLogin, reqLogin)
	if wLogin.Code != http.StatusOK {
		t.Fatalf("Alice login failed: %d", wLogin.Code)
	}
	var aliceLoginRes map[string]interface{}
	_ = json.Unmarshal(wLogin.Body.Bytes(), &aliceLoginRes)
	aliceToken := aliceLoginRes["token"].(string)

	// Create a resource group for device provisioning
	rgPayload := map[string]string{
		"name":       "Engineering VPC",
		"cidr_block": "10.60.0.0/24",
	}
	bodyRG, _ := json.Marshal(rgPayload)
	wRG := httptest.NewRecorder()
	reqRG, _ := http.NewRequest("POST", "/api/resource-groups", bytes.NewBuffer(bodyRG))
	reqRG.Header.Set("Content-Type", "application/json")
	reqRG.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(wRG, reqRG)
	var rgRes map[string]interface{}
	_ = json.Unmarshal(wRG.Body.Bytes(), &rgRes)
	rgID := rgRes["resource_group"].(map[string]interface{})["id"].(string)

	// Admin provisions a critical Company Node (database server)
	nodePayload := map[string]interface{}{
		"hostname":           "prod-postgres-db",
		"device_type":        "company",
		"resource_group_id":  rgID,
		"ip_allocation_type": "dynamic",
		"platform":           "linux",
	}
	bodyNode, _ := json.Marshal(nodePayload)
	wNode := httptest.NewRecorder()
	reqNode, _ := http.NewRequest("POST", "/api/devices", bytes.NewBuffer(bodyNode))
	reqNode.Header.Set("Content-Type", "application/json")
	reqNode.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(wNode, reqNode)
	var nodeRes map[string]interface{}
	_ = json.Unmarshal(wNode.Body.Bytes(), &nodeRes)
	companyNodeID := nodeRes["device"].(map[string]interface{})["id"].(string)

	// Alice provisions her own device
	aliceDevPayload := map[string]interface{}{
		"hostname":           "alice-macbook",
		"device_type":        "user",
		"resource_group_id":  rgID,
		"ip_allocation_type": "dynamic",
		"platform":           "macos",
	}
	bodyAliceDev, _ := json.Marshal(aliceDevPayload)
	wAliceDev := httptest.NewRecorder()
	reqAliceDev, _ := http.NewRequest("POST", "/api/devices", bytes.NewBuffer(bodyAliceDev))
	reqAliceDev.Header.Set("Content-Type", "application/json")
	reqAliceDev.Header.Set("Authorization", "Bearer "+aliceToken)
	router.ServeHTTP(wAliceDev, reqAliceDev)
	var aliceDevRes map[string]interface{}
	_ = json.Unmarshal(wAliceDev.Body.Bytes(), &aliceDevRes)
	aliceDevID := aliceDevRes["device"].(map[string]interface{})["id"].(string)

	// =========================================================================
	// TEST 1: FP-SWAN-AUTH-BOLA-01 (Broken Object-Level Authorization)
	// =========================================================================
	// Alice attempts to download configuration/private key for company node -> MUST BE FORBIDDEN (403)
	wBOLA1 := httptest.NewRecorder()
	reqBOLA1, _ := http.NewRequest("GET", "/api/devices/"+companyNodeID+"/config/apple", nil)
	reqBOLA1.Header.Set("Authorization", "Bearer "+aliceToken)
	router.ServeHTTP(wBOLA1, reqBOLA1)
	if wBOLA1.Code != http.StatusForbidden {
		t.Fatalf("FP-SWAN-AUTH-BOLA-01 regression! Expected 403 Forbidden when standard user downloads unowned device, got %d: %s", wBOLA1.Code, wBOLA1.Body.String())
	}

	// Alice attempts to revoke the company node -> MUST BE FORBIDDEN (403)
	wBOLA2 := httptest.NewRecorder()
	reqBOLA2, _ := http.NewRequest("POST", "/api/devices/"+companyNodeID+"/revoke", nil)
	reqBOLA2.Header.Set("Authorization", "Bearer "+aliceToken)
	router.ServeHTTP(wBOLA2, reqBOLA2)
	if wBOLA2.Code != http.StatusForbidden {
		t.Fatalf("FP-SWAN-AUTH-BOLA-01 regression! Expected 403 Forbidden when standard user revokes unowned device, got %d: %s", wBOLA2.Code, wBOLA2.Body.String())
	}

	// Alice downloads her own device configuration -> MUST SUCCEED (200)
	wOwn := httptest.NewRecorder()
	reqOwn, _ := http.NewRequest("GET", "/api/devices/"+aliceDevID+"/config/apple", nil)
	reqOwn.Header.Set("Authorization", "Bearer "+aliceToken)
	router.ServeHTTP(wOwn, reqOwn)
	if wOwn.Code != http.StatusOK {
		t.Fatalf("Alice should be able to download her own device configuration, got: %d", wOwn.Code)
	}

	// Alice queries /api/devices -> MUST ONLY return her own device, NOT company node
	wDevList := httptest.NewRecorder()
	reqDevList, _ := http.NewRequest("GET", "/api/devices", nil)
	reqDevList.Header.Set("Authorization", "Bearer "+aliceToken)
	router.ServeHTTP(wDevList, reqDevList)
	if wDevList.Code != http.StatusOK {
		t.Fatalf("Failed to list devices: %d", wDevList.Code)
	}
	var devListRes map[string]interface{}
	_ = json.Unmarshal(wDevList.Body.Bytes(), &devListRes)
	devs := devListRes["devices"].([]interface{})
	if len(devs) != 1 {
		t.Fatalf("Expected Alice to only see 1 device (her own), got %d devices", len(devs))
	}
	if devs[0].(map[string]interface{})["id"] != aliceDevID {
		t.Fatalf("Expected device to be Alice's own device, got: %v", devs[0].(map[string]interface{})["id"])
	}

	// =========================================================================
	// TEST 2: FP-SWAN-AUTH-SSE-01 (Unauthenticated SSE Stream)
	// =========================================================================
	// Unauthenticated connection to /api/realtime/stream -> MUST RETURN 401
	wSSEUnauth := httptest.NewRecorder()
	reqSSEUnauth, _ := http.NewRequest("GET", "/api/realtime/stream", nil)
	router.ServeHTTP(wSSEUnauth, reqSSEUnauth)
	if wSSEUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("FP-SWAN-AUTH-SSE-01 regression! Expected 401 Unauthorized for unauthenticated SSE, got %d", wSSEUnauth.Code)
	}

	// Authenticated connection with ?token= query parameter -> MUST SUCCEED
	wSSEAuth := httptest.NewRecorder()
	ctxSSE, cancelSSE := context.WithCancel(context.Background())
	reqSSEAuth, _ := http.NewRequestWithContext(ctxSSE, "GET", "/api/realtime/stream?token="+aliceToken, nil)
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancelSSE()
	}()
	router.ServeHTTP(wSSEAuth, reqSSEAuth)
	if wSSEAuth.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for SSE with query token, got %d", wSSEAuth.Code)
	}
	if !strings.Contains(wSSEAuth.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("Expected text/event-stream content type, got: %s", wSSEAuth.Header().Get("Content-Type"))
	}

	// =========================================================================
	// TEST 3: FP-SWAN-AUTH-BFLA-01 (Missing Function-Level Access Control on Logs)
	// =========================================================================
	// Alice attempts to access administrative system logs -> MUST BE FORBIDDEN (403)
	wLogNonAdmin := httptest.NewRecorder()
	reqLogNonAdmin, _ := http.NewRequest("GET", "/api/system/logs", nil)
	reqLogNonAdmin.Header.Set("Authorization", "Bearer "+aliceToken)
	router.ServeHTTP(wLogNonAdmin, reqLogNonAdmin)
	if wLogNonAdmin.Code != http.StatusForbidden {
		t.Fatalf("FP-SWAN-AUTH-BFLA-01 regression! Expected 403 Forbidden for non-admin on /api/system/logs, got %d: %s", wLogNonAdmin.Code, wLogNonAdmin.Body.String())
	}

	// Admin accesses system logs -> MUST SUCCEED (200)
	wLogAdmin := httptest.NewRecorder()
	reqLogAdmin, _ := http.NewRequest("GET", "/api/system/logs", nil)
	reqLogAdmin.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(wLogAdmin, reqLogAdmin)
	if wLogAdmin.Code != http.StatusOK {
		t.Fatalf("Admin should be able to access system logs, got: %d", wLogAdmin.Code)
	}
	_ = aliceID
}
