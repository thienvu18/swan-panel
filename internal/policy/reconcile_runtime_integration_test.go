package policy_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
	"github.com/swan-panel/swan-panel/internal/vici"
	"gorm.io/gorm"
)

type simulatedVICI struct {
	listener   net.Listener
	mu         sync.Mutex
	conns      []net.Conn
	received   []string
	reject     string
	statsCalls int
	onCommand  func(string)
}

func viciField(key, value string) []byte {
	b := bytes.NewBuffer([]byte{3, byte(len(key))})
	b.WriteString(key)
	_ = binary.Write(b, binary.BigEndian, uint16(len(value)))
	b.WriteString(value)
	return b.Bytes()
}

func viciSection(name string) []byte {
	b := bytes.NewBuffer([]byte{1, byte(len(name))})
	b.WriteString(name)
	b.Write(viciField("version", "2"))
	b.WriteByte(2)
	return b.Bytes()
}

func replyVICI(conn net.Conn, kind byte, name string, body []byte) error {
	msg := []byte{kind}
	if kind == 7 {
		msg = append(msg, byte(len(name)))
		msg = append(msg, name...)
	}
	msg = append(msg, body...)
	packet := make([]byte, len(msg)+4)
	binary.BigEndian.PutUint32(packet[:4], uint32(len(msg)))
	copy(packet[4:], msg)
	_, err := conn.Write(packet)
	return err
}

func startSimulatedVICI(t *testing.T, reject string) (*simulatedVICI, string) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "vici*")
	if err != nil {
		dir = t.TempDir()
	} else {
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
	}
	path := filepath.Join(dir, "charon.vici")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	stub := &simulatedVICI{listener: listener, reject: reject}
	t.Cleanup(func() {
		_ = listener.Close()
		stub.mu.Lock()
		defer stub.mu.Unlock()
		for _, c := range stub.conns {
			_ = c.Close()
		}
	})
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			stub.mu.Lock()
			stub.conns = append(stub.conns, c)
			stub.mu.Unlock()
			go stub.serve(c)
		}
	}()
	return stub, path
}

func (s *simulatedVICI) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(conn, hdr[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(hdr[:])
		if n < 2 || n > 1<<20 {
			return
		}
		msg := make([]byte, n)
		if _, err := io.ReadFull(conn, msg); err != nil {
			return
		}
		length := int(msg[1])
		if len(msg) < 2+length {
			return
		}
		cmd := string(msg[2 : 2+length])
		if msg[0] == 3 || msg[0] == 4 {
			if replyVICI(conn, 5, "", nil) != nil {
				return
			}
			continue
		}
		if msg[0] != 0 {
			continue
		}
		s.mu.Lock()
		s.received = append(s.received, cmd)
		reject := s.reject
		if cmd == "stats" {
			s.statsCalls++
			if s.statsCalls == 2 {
				switch reject {
				case "stats-restarts-during-apply":
					if err := os.Chtimes(s.listener.Addr().String(), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
						panic(err)
					}
				case "stats-removes-persisted-config":
					if err := os.Remove(os.Getenv("SWANPANEL_SWANCTL_FILE")); err != nil {
						panic(err)
					}
				}
			}
		}
		rejectCurrent := cmd == reject ||
			(cmd == "stats" && reject == "stats-rejects-after-apply" && s.statsCalls == 3)
		callback := s.onCommand
		s.mu.Unlock()
		if callback != nil {
			callback(cmd)
		}
		if rejectCurrent {
			if replyVICI(conn, 1, "", append(viciField("success", "no"), viciField("errmsg", "rejected by simulated daemon")...)) != nil {
				return
			}
			continue
		}
		if cmd == "list-conns" {
			if replyVICI(conn, 7, "list-conn", viciSection("swanpanel-stale")) != nil {
				return
			}
			if replyVICI(conn, 7, "list-conn", viciSection("external-connection")) != nil {
				return
			}
		}
		if cmd == "get-pools" {
			entries := append(viciSection("swanpanel-vip-dev_thienvu"), viciSection("swanpanel-vip-stale")...)
			entries = append(entries, viciSection("operator-pool")...)
			if replyVICI(conn, 1, "", entries) != nil {
				return
			}
			continue
		}
		if cmd == "stats" {
			if replyVICI(conn, 1, "", viciField("success", "yes")) != nil {
				return
			}
			continue
		}
		if replyVICI(conn, 1, "", viciField("success", "yes")) != nil {
			return
		}
	}
}

func (s *simulatedVICI) has(command string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range s.received {
		if name == command {
			return true
		}
	}
	return false
}

// The production (not APP_ENV=test) path is exercised with an actual govici
// UNIX socket and an isolated nft executable. No host firewall is touched.
func prepareSimulatedGateway(t *testing.T, reject string) (*simulatedVICI, string) {
	t.Helper()
	t.Setenv("APP_ENV", "ci-integration")
	t.Setenv("SWANPANEL_FIREWALL_DRY_RUN", "")
	t.Setenv("SWANPANEL_SWANCTL_FILE", filepath.Join(t.TempDir(), "conf.d", "managed.conf"))
	logPath := filepath.Join(t.TempDir(), "applied-nft.txt")
	t.Setenv("SIMULATED_NFT_LOG", logPath)
	bin := t.TempDir()
	cmd := "#!/bin/sh\nif [ \"$1\" = \"list\" ]; then exit 1; fi\nif [ \"$1\" = \"-f\" ]; then cat > \"$SIMULATED_NFT_LOG\"; exit 0; fi\nexit 5\n"
	if err := os.WriteFile(filepath.Join(bin, "nft"), []byte(cmd), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	server, socket := startSimulatedVICI(t, reject)
	t.Setenv("VICI_SOCKET", socket)
	original := vici.GetClient("")
	vici.DefaultClient = vici.NewClient(socket)
	t.Cleanup(func() { vici.DefaultClient = original })
	return server, logPath
}

func TestReconcileProductionPathConvergesWithoutTrafficBlackoutAndRepairsFileDrift(t *testing.T) {
	stub, logPath := prepareSimulatedGateway(t, "")
	database := setupPolicyTestDB(t)
	if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := policy.Reconcile(ctx, database); err != nil {
		t.Fatalf("real-path reconcile: %v", err)
	}
	state, err := policy.GetApplyState(ctx, database)
	if err != nil || state.Status != "applied" || state.SwanctlDigest == "" || state.DaemonBootID == "" {
		t.Fatalf("runtime state: %+v %v", state, err)
	}
	file, err := os.ReadFile(policy.SwanctlFilePath())
	if err != nil || !bytes.Contains(file, []byte("swanpanel-dev_thienvu")) {
		t.Fatalf("persistent connection absent: %s %v", file, err)
	}
	script, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(script), "meta ipsec exists drop") || !strings.Contains(string(script), "accept") {
		t.Fatalf("nft policy not applied: %v %s", err, script)
	}
	if strings.Count(string(script), "meta ipsec exists drop") > 2 {
		t.Fatal("unexpected blanket-deny firewall during successful policy apply")
	}
	for _, command := range []string{"stats", "load-pool", "load-conn", "list-conns", "unload-conn", "get-pools", "unload-pool", "list-sas"} {
		if !stub.has(command) {
			t.Errorf("missing VICI command %s", command)
		}
	}
	needed, err := policy.NeedsReconciliation(ctx, database)
	if err != nil || needed {
		t.Fatalf("stable gateway falsely drifted: %v %v", needed, err)
	}
	if err := os.WriteFile(policy.SwanctlFilePath(), []byte("# tampered\n"), 0600); err != nil {
		t.Fatal(err)
	}
	needed, err = policy.NeedsReconciliation(ctx, database)
	if err != nil || !needed {
		t.Fatalf("file drift not detected: %v %v", needed, err)
	}
	if err := policy.Reconcile(ctx, database); err != nil {
		t.Fatalf("repair drift: %v", err)
	}
	if needed, err = policy.NeedsReconciliation(ctx, database); err != nil || needed {
		t.Fatalf("file remained drifted: %v %v", needed, err)
	}
}

func TestReconcileOnlyPushesInternalDNSWhenServerIsEnabled(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		expectDNS   bool
	}{
		{"enabled", "true", true},
		{"disabled", "false", false},
		{"default on malformed config", "not-a-bool", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _ = prepareSimulatedGateway(t, "")
			t.Setenv("ENABLE_DNS", tc.value)
			database := setupPolicyTestDB(t)
			if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
				t.Fatal(err)
			}
			if err := policy.Reconcile(context.Background(), database); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(policy.SwanctlFilePath())
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(content), "dns = "); got != tc.expectDNS {
				t.Fatalf("unexpected DNS push with ENABLE_DNS=%q: %s", tc.value, content)
			}
		})
	}
}

func TestReconcileProductionPathDefersNonfatalPoolCleanupFailures(t *testing.T) {
	for _, reject := range []string{"get-pools", "unload-pool", "list-sas"} {
		t.Run(reject, func(t *testing.T) {
			stub, _ := prepareSimulatedGateway(t, reject)
			database := setupPolicyTestDB(t)
			if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := policy.Reconcile(ctx, database); err != nil {
				t.Fatalf("optional pool GC failure must not break VPN policy: %v", err)
			}
			if !stub.has(reject) {
				t.Fatalf("did not exercise %s", reject)
			}
			state, err := policy.GetApplyState(ctx, database)
			if err != nil || state.Status != "applied" {
				t.Fatalf("successful policy marked failed because of pool GC: %+v %v", state, err)
			}
		})
	}
}

func TestReconcileProductionPathHandlesLoopbackCommandErrorsWithoutHostChanges(t *testing.T) {
	for _, mode := range []string{"initial", "cleanup"} {
		t.Run(mode, func(t *testing.T) {
			stub, _ := prepareSimulatedGateway(t, "")
			t.Setenv("APP_ENV", "production")
			t.Setenv("TEST_IP_FAIL", mode)
			t.Setenv("TEST_IP_COUNTER", filepath.Join(t.TempDir(), "ip-show-seen"))
			bin := t.TempDir()
			script := "#!/bin/sh\n" +
				"if [ \"$1\" = \"-o\" ]; then\n" +
				"  if [ \"$TEST_IP_FAIL\" = \"initial\" ] || [ -f \"$TEST_IP_COUNTER\" ]; then exit 13; fi\n" +
				"  touch \"$TEST_IP_COUNTER\"\n" +
				"  printf '1: lo inet 127.0.0.1/8 scope host lo\\n'\n" +
				"fi\nexit 0\n"
			if err := os.WriteFile(filepath.Join(bin, "ip"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			database := setupPolicyTestDB(t)
			if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := policy.Reconcile(ctx, database); err == nil {
				t.Fatal("failed loopback command incorrectly reported applied")
			}
			state, err := policy.GetApplyState(ctx, database)
			if err != nil || state.Status != "failed" {
				t.Fatalf("loopback error must be retryable: %+v %v", state, err)
			}
			if mode == "cleanup" && !stub.has("get-pools") {
				t.Fatal("test never reached loopback cleanup")
			}
		})
	}
}

func TestReconcileRuntimeDetectsDaemonIdentityChangedDuringPoolCleanup(t *testing.T) {
	stub, _ := prepareSimulatedGateway(t, "")
	database := setupPolicyTestDB(t)
	if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
		t.Fatal(err)
	}
	changed := make(chan error, 1)
	stub.onCommand = func(command string) {
		if command == "list-sas" {
			changed <- os.Chtimes(stub.listener.Addr().String(), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	applyErr := policy.Reconcile(ctx, database)
	select {
	case err := <-changed:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("daemon identity mutation was not exercised")
	}
	if !errors.Is(applyErr, policy.ErrDaemonRestartedDuringApply) {
		t.Fatalf("daemon restart during apply incorrectly accepted: %v", applyErr)
	}
}

func TestReconcileProductionPathRejectsVICIAndKeepsRetryableState(t *testing.T) {
	stub, _ := prepareSimulatedGateway(t, "load-pool")
	database := setupPolicyTestDB(t)
	if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := policy.Reconcile(ctx, database); err == nil {
		t.Fatal("rejected VICI pool marked applied")
	}
	state, err := policy.GetApplyState(ctx, database)
	if err != nil || state.Status != "failed" || state.AppliedDigest != "" {
		t.Fatalf("failed state: %+v %v", state, err)
	}
	if !stub.has("load-pool") {
		t.Fatal("pool failure branch not exercised")
	}
	needed, err := policy.NeedsReconciliation(ctx, database)
	if err != nil || !needed {
		t.Fatalf("rejected daemon not scheduled for retry: %v %v", needed, err)
	}
}

func TestReconcileProductionPathFailsClosedOnDaemonReloadAndRestartFaults(t *testing.T) {
	scenarios := []struct {
		name   string
		reject string
	}{
		{"VICI connection load rejected", "load-conn"},
		{"VICI connection discovery rejected", "list-conns"},
		{"stale VICI connection cannot unload", "unload-conn"},
		{"daemon status unavailable", "stats"},
		{"daemon restarts while connections are applying", "stats-restarts-during-apply"},
		{"daemon stops responding during final verification", "stats-rejects-after-apply"},
		{"persisted configuration disappears before final verification", "stats-removes-persisted-config"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			_, _ = prepareSimulatedGateway(t, scenario.reject)
			database := setupPolicyTestDB(t)
			if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := policy.Reconcile(ctx, database); err == nil {
				t.Fatalf("simulated fault %s was reported as successful", scenario.reject)
			}
			state, err := policy.GetApplyState(ctx, database)
			if err != nil || state.Status != "failed" || state.AppliedDigest != "" || state.LastError == "" {
				t.Fatalf("unsafe applied state after %s: %+v %v", scenario.reject, state, err)
			}
		})
	}
}

func TestReconcileRechecksDesiredConfigurationAfterRuntimeApply(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*gorm.DB) error
	}{
		{"desired config changed", func(database *gorm.DB) error {
			return database.Model(&db.StrongswanSettings{}).Where("id = ?", 1).
				Update("dpd_action", "restart").Error
		}},
		{"desired config cannot be read", func(database *gorm.DB) error {
			return database.Migrator().DropTable(&db.AccessRule{})
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			stub, _ := prepareSimulatedGateway(t, "")
			database := setupPolicyTestDB(t)
			if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
				t.Fatal(err)
			}
			mutated := make(chan error, 1)
			stub.onCommand = func(name string) {
				if name == "unload-conn" {
					mutated <- scenario.mutate(database)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			applyErr := policy.Reconcile(ctx, database)
			select {
			case err := <-mutated:
				if err != nil {
					t.Fatal(err)
				}
			default:
				t.Fatal("concurrent desired-state mutation did not execute")
			}
			if applyErr == nil {
				t.Fatal("runtime was declared applied after desired state changed")
			}
			if scenario.name == "desired config changed" && !errors.Is(applyErr, policy.ErrDesiredStateChangedDuringApply) {
				t.Fatalf("unexpected stale state error: %v", applyErr)
			}
			if scenario.name == "desired config cannot be read" {
				// State must remain non-applied even if the durable ledger is
				// still writable after a read-side database failure.
				state, err := policy.GetApplyState(ctx, database)
				if err != nil || state.Status != "failed" {
					t.Fatalf("unreadable desired state incorrectly applied: %+v %v", state, err)
				}
			}
		})
	}
}
