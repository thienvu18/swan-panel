package api_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/db"
)

func postMultipartRestore(t *testing.T, router *gin.Engine, token, filename string, payload []byte, passphrase, confirm string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if payload != nil {
		part, err := writer.CreateFormFile("backup_file", filename)
		if err != nil {
			t.Fatalf("create multipart file: %v", err)
		}
		if _, err := part.Write(payload); err != nil {
			t.Fatalf("write multipart file: %v", err)
		}
	}
	_ = writer.WriteField("passphrase", passphrase)
	_ = writer.WriteField("confirm", confirm)
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, _ := http.NewRequest("POST", "/api/system/restore", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestBackupAndRestoreAPIRBACValidationAndRoundTrip(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)

	// Create a non-admin local operator to verify admin-only access enforcement.
	userPass := "OperatorBackupCheck!2026"
	mustStatus(t, router, "POST", "/api/users", admin, map[string]any{
		"name":     "Regular Operator",
		"email":    "backup-op@example.com",
		"password": userPass,
		"is_admin": false,
	}, http.StatusCreated)
	userLogin := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email":    "backup-op@example.com",
		"password": userPass,
	})
	if userLogin.Code != http.StatusOK {
		t.Fatalf("operator login failed: %s", userLogin.Body.String())
	}
	var opAuth struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(userLogin.Body.Bytes(), &opAuth)

	// Non-admin must be rejected on both backup and restore.
	mustStatus(t, router, "POST", "/api/system/backup", opAuth.Token, map[string]any{
		"passphrase": "valid-backup-passphrase",
	}, http.StatusForbidden)
	deniedRestore := postMultipartRestore(t, router, opAuth.Token, "backup.tar.enc", []byte("stub"), "valid-backup-passphrase", "RESTORE BACKUP")
	if deniedRestore.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin restore, got %d: %s", deniedRestore.Code, deniedRestore.Body.String())
	}

	// Short passphrase must be rejected.
	mustStatus(t, router, "POST", "/api/system/backup", admin, map[string]any{
		"passphrase": "too-short",
	}, http.StatusBadRequest)

	// Seed a record before taking the backup.
	if err := db.DB.Create(&db.DNSOverride{ID: "dns-pre", Hostname: "pre-backup.internal", RecordType: "A", TargetAddress: "10.88.0.10"}).Error; err != nil {
		t.Fatalf("seed pre-backup DNS record: %v", err)
	}

	passphrase := "disaster-recovery-secret-2026"
	exportResp := requestJSON(t, router, "POST", "/api/system/backup", admin, map[string]any{
		"passphrase": passphrase,
	})
	if exportResp.Code != http.StatusOK {
		t.Fatalf("expected 200 exporting backup, got %d: %s", exportResp.Code, exportResp.Body.String())
	}
	if !strings.Contains(exportResp.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("expected no-store Cache-Control header, got %q", exportResp.Header().Get("Cache-Control"))
	}
	if !strings.Contains(exportResp.Header().Get("Content-Disposition"), ".tar.enc") {
		t.Fatalf("expected .tar.enc Content-Disposition, got %q", exportResp.Header().Get("Content-Disposition"))
	}
	archiveBytes := exportResp.Body.Bytes()
	if len(archiveBytes) < 36 || string(archiveBytes[:8]) != "SWANBK01" {
		t.Fatalf("expected valid encrypted SWANBK01 archive, got len=%d", len(archiveBytes))
	}

	// Audit log must record backup creation immediately after export.
	createdLogs := mustStatus(t, router, "GET", "/api/audit-logs?action=backup.created", admin, nil, http.StatusOK)
	if !strings.Contains(createdLogs, `"backup.created"`) {
		t.Fatalf("expected backup.created audit log entry, got: %s", createdLogs)
	}

	// Mutate DB after export.
	if err := db.DB.Create(&db.DNSOverride{ID: "dns-post", Hostname: "post-backup-ephemeral.internal", RecordType: "A", TargetAddress: "10.88.0.99"}).Error; err != nil {
		t.Fatalf("seed post-backup record: %v", err)
	}

	// Reject restore missing typed confirmation or wrong passphrase.
	badConfirm := postMultipartRestore(t, router, admin, "backup.tar.enc", archiveBytes, passphrase, "yes")
	if badConfirm.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when confirmation string mismatches, got %d", badConfirm.Code)
	}
	badPass := postMultipartRestore(t, router, admin, "backup.tar.enc", archiveBytes, "wrong-passphrase-2026", "RESTORE BACKUP")
	if badPass.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when passphrase is wrong, got %d", badPass.Code)
	}

	// Perform verified restore.
	restoreResp := postMultipartRestore(t, router, admin, "swanpanel-backup.tar.enc", archiveBytes, passphrase, "RESTORE BACKUP")
	if restoreResp.Code != http.StatusOK && restoreResp.Code != http.StatusAccepted {
		t.Fatalf("expected 200 or 202 from restore, got %d: %s", restoreResp.Code, restoreResp.Body.String())
	}

	var preCount, postCount int64
	_ = db.DB.Model(&db.DNSOverride{}).Where("hostname = ?", "pre-backup.internal").Count(&preCount).Error
	_ = db.DB.Model(&db.DNSOverride{}).Where("hostname = ?", "post-backup-ephemeral.internal").Count(&postCount).Error
	if preCount != 1 || postCount != 0 {
		t.Fatalf("expected database state reverted to snapshot (pre=%d post=%d)", preCount, postCount)
	}

	// Pre-restore JWT must be invalidated across restore (B01 / B04 SessionEpoch rotation).
	mustStatus(t, router, "GET", "/api/audit-logs?action=backup.restored", admin, nil, http.StatusUnauthorized)

	// Sign in again with restored administrator credentials and verify audit trail.
	relogin := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email":    "admin@swanpanel.internal",
		"password": "admin123456789!",
	})
	if relogin.Code != http.StatusOK {
		t.Fatalf("post-restore login failed: %d %s", relogin.Code, relogin.Body.String())
	}
	var reloginBody struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(relogin.Body.Bytes(), &reloginBody); err != nil || reloginBody.Token == "" {
		t.Fatalf("missing post-restore token: %s", relogin.Body.String())
	}

	restoredLogs := mustStatus(t, router, "GET", "/api/audit-logs?action=backup.restored", reloginBody.Token, nil, http.StatusOK)
	if !strings.Contains(restoredLogs, `"backup.restored"`) {
		t.Fatalf("expected backup.restored audit log entry, got: %s", restoredLogs)
	}
}

func TestBackupAndRestoreRejectUnauthorizedUsersAndMalformedUploads(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)

	// Non-admin user cannot export or restore backups.
	memberResp := requestJSON(t, router, "POST", "/api/users", admin, map[string]any{
		"name":     "Member User",
		"email":    "member-backup@example.com",
		"password": "StrongMemberPassword!2026",
		"is_admin": false,
	})
	if memberResp.Code != http.StatusCreated {
		t.Fatalf("create user failed: %s", memberResp.Body.String())
	}
	loginResp := requestJSON(t, router, "POST", "/api/auth/login", "", map[string]any{
		"email":    "member-backup@example.com",
		"password": "StrongMemberPassword!2026",
	})
	var loginData struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(loginResp.Body.Bytes(), &loginData)
	mustStatus(t, router, "POST", "/api/system/backup", loginData.Token, map[string]any{"passphrase": "valid-passphrase-123"}, http.StatusForbidden)
	if resp := postMultipartRestore(t, router, loginData.Token, "b.tar.enc", []byte("x"), "valid-passphrase-123", "RESTORE BACKUP"); resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin restore, got %d", resp.Code)
	}

	// Missing/invalid JSON on backup and restore without file/invalid payload/passphrase reject cleanly with 400.
	mustStatus(t, router, "POST", "/api/system/backup", admin, map[string]any{}, http.StatusBadRequest)
	mustStatus(t, router, "POST", "/api/system/restore", admin, map[string]any{}, http.StatusBadRequest)
	shortPass := postMultipartRestore(t, router, admin, "b.tar.enc", []byte("x"), "short", "RESTORE BACKUP")
	if shortPass.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short passphrase on restore, got %d", shortPass.Code)
	}
	badMagic := postMultipartRestore(t, router, admin, "b.tar.enc", []byte("NOT-AN-ENCRYPTED-ARCHIVE"), "valid-passphrase-123", "RESTORE BACKUP")
	if badMagic.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for corrupt archive payload, got %d", badMagic.Code)
	}
}
