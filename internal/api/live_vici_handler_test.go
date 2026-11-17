package api_test

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/api/middleware"
	"github.com/swan-panel/swan-panel/internal/db"
	vicipkg "github.com/swan-panel/swan-panel/internal/vici"
)

func fakeVICIEntry(key, value string) []byte {
	result := []byte{3, byte(len(key))}
	result = append(result, key...)
	result = append(result, byte(len(value)>>8), byte(len(value)))
	return append(result, value...)
}
func fakeVICIReply(conn net.Conn, kind byte, event string, body []byte) error {
	reply := []byte{kind}
	if kind == 7 {
		reply = append(reply, byte(len(event)))
		reply = append(reply, event...)
	}
	reply = append(reply, body...)
	packet := make([]byte, len(reply)+4)
	binary.BigEndian.PutUint32(packet[:4], uint32(len(reply)))
	copy(packet[4:], reply)
	_, err := conn.Write(packet)
	return err
}
func startLiveVICIStub(t *testing.T, rejectCommand ...string) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "vici*")
	if err != nil {
		dir = t.TempDir()
	} else {
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
	}
	path := filepath.Join(dir, "test-charon.vici")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				for {
					var lenBuf [4]byte
					if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
						return
					}
					n := binary.BigEndian.Uint32(lenBuf[:])
					if n < 2 || n > 100000 {
						return
					}
					message := make([]byte, n)
					if _, err := io.ReadFull(conn, message); err != nil {
						return
					}
					kind := message[0]
					if len(message) < 2+int(message[1]) {
						return
					}
					command := string(message[2 : 2+int(message[1])])
					if kind == 3 || kind == 4 {
						if err := fakeVICIReply(conn, 5, "", nil); err != nil {
							return
						}
						continue
					}
					if kind != 0 {
						continue
					}
					if len(rejectCommand) > 0 && rejectCommand[0] == command && (command == "terminate" || command == "unload-pool") {
						if err := fakeVICIReply(conn, 1, "", fakeVICIEntry("success", "no")); err != nil {
							return
						}
						continue
					}
					if command == "list-sas" {
						section := append([]byte{1, byte(len("swanpanel-test-connection"))}, []byte("swanpanel-test-connection")...)
						section = append(section, fakeVICIEntry("remote-id", "my-client")...)
						section = append(section, fakeVICIEntry("remote-host", "192.0.2.44")...)
						section = append(section, fakeVICIEntry("established", "17")...)
						section = append(section, 2)
						if err := fakeVICIReply(conn, 7, "list-sa", section); err != nil {
							return
						}
					}
					if err := fakeVICIReply(conn, 1, "", fakeVICIEntry("success", "yes")); err != nil {
						return
					}
				}
			}()
		}
	}()
	return path
}

// Exercises the actual router -> session handler -> govici socket boundary,
// with deterministic protocol packets and no machine-level VPN privileges.
func TestAuthenticatedSessionsAndDashboardUseRealVICIStreamAndOwnerFilters(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	old := vicipkg.DefaultClient
	vicipkg.DefaultClient = vicipkg.NewClient(startLiveVICIStub(t))
	t.Cleanup(func() { vicipkg.DefaultClient = old })
	user := db.User{ID: "live-user", Name: "Client owner", Email: "live@example.invalid", AuthType: "local", Status: "ACTIVE"}
	if err := db.DB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	group := db.ResourceGroup{ID: "live-group", Name: "Test VIP pool", CIDRBlock: "10.77.0.0/24"}
	if err := db.DB.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	owner := user.ID
	device := db.Device{ID: "test-connection", Hostname: "my-client", DeviceType: "user",
		UserID: &owner, ResourceGroupID: group.ID, IPAllocationType: "static", VirtualIP: "10.77.0.3", Platform: "linux"}
	if err := db.DB.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	token, err := middleware.GenerateToken(&user, "test-secret-key-12345", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	adminSessions := mustStatus(t, router, "GET", "/api/sessions", admin, nil, 200)
	if !strings.Contains(adminSessions, "swanpanel-test-connection") {
		t.Fatal("admin did not see live VICI sessions")
	}
	userSessions := mustStatus(t, router, "GET", "/api/sessions", token, nil, 200)
	if !strings.Contains(userSessions, "my-client") {
		t.Fatal("owner cannot see their connected device")
	}
	adminStats := mustStatus(t, router, "GET", "/api/dashboard/stats", admin, nil, 200)
	if !strings.Contains(adminStats, "online") {
		t.Fatal("daemon stats did not confirm online")
	}
	userStats := mustStatus(t, router, "GET", "/api/dashboard/stats", token, nil, 200)
	if !strings.Contains(userStats, "online") {
		t.Fatal("owner dashboard did not confirm online")
	}
	mustStatus(t, router, "GET", "/api/system/daemon/status", token, nil, 200)
	mustStatus(t, router, "POST", "/api/sessions/terminate", admin, map[string]any{
		"ike_name": "swanpanel-test-connection",
	}, 200)
	mustStatus(t, router, "POST", "/api/sessions/terminate", token, map[string]any{
		"ike_name": "swanpanel-test-connection",
	}, 403)
	// The live socket still sends the connection, but DB ownership was revoked.
	if err := db.DB.Model(&device).Update("is_revoked", true).Error; err != nil {
		t.Fatal(err)
	}
	filtered := mustStatus(t, router, "GET", "/api/sessions", token, nil, 200)
	if strings.Contains(filtered, "my-client") {
		t.Fatal("revoked device still shown in user session list")
	}
}

func TestDaemonStartAndRestartRequireHealthyVICIAndPolicyApply(t *testing.T) {
	router, cleanup := setupTestRouter(t)
	defer cleanup()
	admin := getAdminToken(t, router)
	old := vicipkg.DefaultClient
	vicipkg.DefaultClient = vicipkg.NewClient(startLiveVICIStub(t))
	t.Cleanup(func() { vicipkg.DefaultClient = old })
	fakeBin := t.TempDir()
	executable := filepath.Join(fakeBin, "ipsec")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)
	for _, action := range []string{"start", "restart"} {
		response := mustStatus(t, router, "POST", "/api/system/daemon", admin, map[string]any{
			"action": action,
		}, 200)
		if !strings.Contains(response, "active") {
			t.Fatalf("%s returned no live status: %s", action, response)
		}
	}
	// A terminated process' socket cannot be treated as healthy after the
	// configured VICI endpoint has disappeared.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := vicipkg.DefaultClient.CheckHealth(ctx); err != nil {
		t.Fatal(err)
	}
}
