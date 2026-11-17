package vici_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/vici"
)

// A protocol-level UNIX socket responder. Unlike a mocked Client interface,
// this exercises the real govici codec, connection/cache and stream parser.
type testVICIServer struct {
	listener    net.Listener
	received    chan []byte
	mu          sync.Mutex
	conns       []net.Conn
	failCommand string
}

func startVICIServer(t *testing.T, failCommand string) (*testVICIServer, string) {
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
	server := &testVICIServer{listener: listener, received: make(chan []byte, 128), failCommand: failCommand}
	t.Cleanup(func() {
		_ = listener.Close()
		server.mu.Lock()
		for _, conn := range server.conns {
			_ = conn.Close()
		}
		server.mu.Unlock()
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			server.mu.Lock()
			server.conns = append(server.conns, conn)
			server.mu.Unlock()
			go server.serve(conn)
		}
	}()
	return server, path
}

func wireKey(key, val string) []byte {
	buf := bytes.NewBuffer([]byte{3, byte(len(key))})
	buf.WriteString(key)
	_ = binary.Write(buf, binary.BigEndian, uint16(len(val)))
	buf.WriteString(val)
	return buf.Bytes()
}
func wireSection(name string, values ...[]byte) []byte {
	buf := bytes.NewBuffer([]byte{1, byte(len(name))})
	buf.WriteString(name)
	for _, val := range values {
		buf.Write(val)
	}
	buf.WriteByte(2)
	return buf.Bytes()
}
func wireList(name string, values ...string) []byte {
	buf := bytes.NewBuffer([]byte{4, byte(len(name))})
	buf.WriteString(name)
	for _, value := range values {
		buf.WriteByte(5)
		_ = binary.Write(buf, binary.BigEndian, uint16(len(value)))
		buf.WriteString(value)
	}
	buf.WriteByte(6)
	return buf.Bytes()
}
func writeVICIPacket(conn net.Conn, kind byte, name string, entries ...[]byte) error {
	buf := bytes.NewBuffer([]byte{kind})
	if kind == 7 {
		buf.WriteByte(byte(len(name)))
		buf.WriteString(name)
	}
	for _, entry := range entries {
		buf.Write(entry)
	}
	payload := buf.Bytes()
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	_, err := conn.Write(append(length[:], payload...))
	return err
}
func (s *testVICIServer) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	for {
		var length [4]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			return
		}
		size := binary.BigEndian.Uint32(length[:])
		if size == 0 || size > 1<<20 {
			return
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(conn, data); err != nil {
			return
		}
		copyData := append([]byte(nil), data...)
		s.received <- copyData
		if len(data) < 2 {
			return
		}
		kind, nameLength := data[0], int(data[1])
		if len(data) < 2+nameLength {
			return
		}
		name := string(data[2 : 2+nameLength])
		if kind == 3 || kind == 4 {
			if err := writeVICIPacket(conn, 5, ""); err != nil {
				return
			}
			continue
		}
		if kind != 0 {
			continue
		}
		if s.failCommand == name {
			if err := writeVICIPacket(conn, 1, "", wireKey("success", "no"), wireKey("errmsg", "rejected by test daemon")); err != nil {
				return
			}
			continue
		}
		if name == "list-conns" {
			managed := wireSection("swanpanel-dev123", wireKey("version", "2"))
			external := wireSection("external-ikev2", wireKey("version", "2"))
			stale := wireSection("swanpanel-stale", wireKey("version", "2"))
			if err := writeVICIPacket(conn, 7, "list-conn", managed, external, stale); err != nil {
				return
			}
		}
		if name == "list-sas" {
			event := wireSection("swanpanel-abc",
				wireKey("remote-id", "client.test"),
				wireKey("remote-host", "192.0.2.2"),
				wireKey("established", "65"),
				wireKey("encr-alg", "AES_GCM_16"),
				wireList("remote-vips", "10.20.0.2"))
			if err := writeVICIPacket(conn, 7, "list-sa", event); err != nil {
				return
			}
		}
		if name == "get-pools" {
			if err := writeVICIPacket(conn, 1, "",
				wireSection("swanpanel-vip-dev123", wireKey("size", "1")),
				wireSection("external-operator", wireKey("size", "4")),
				wireSection("swanpanel-vip-stale", wireKey("size", "1"))); err != nil {
				return
			}
			continue
		}
		if name == "stats" {
			if s.failCommand == "stats-uses-since" {
				if err := writeVICIPacket(conn, 1, "",
					wireSection("uptime", wireKey("running", "unknown"), wireKey("since", "2026-11-15T00:00:00Z"))); err != nil {
					return
				}
				continue
			}
			if err := writeVICIPacket(conn, 1, "",
				wireSection("uptime", wireKey("running", "3661"), wireKey("since", "2026-01-01")),
				wireSection("workers", wireKey("total", "8")),
				wireSection("ikesas", wireKey("total", "2"))); err != nil {
				return
			}
			continue
		}
		if err := writeVICIPacket(conn, 1, "", wireKey("success", "yes")); err != nil {
			return
		}
	}
}

func receiveCommand(t *testing.T, s *testVICIServer, expected string) []byte {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-s.received:
			if len(msg) < 2 {
				continue
			}
			size := int(msg[1])
			if len(msg) >= 2+size && string(msg[2:2+size]) == expected {
				return msg
			}
		case <-deadline:
			t.Fatalf("command %q not observed", expected)
		}
	}
}

func TestVICIProtocolConnectionPoolAndSessionLifecycle(t *testing.T) {
	server, path := startVICIServer(t, "")
	client := vici.NewClient(path)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if !client.IsConnected() {
		t.Fatal("VICI stats returned successfully but health check failed")
	}
	if err := client.CheckHealth(ctx); err != nil {
		t.Fatal(err)
	}
	stats, err := client.GetDaemonStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Status != "active" || stats.Workers != 8 || stats.ActiveSAs != 2 ||
		!strings.Contains(stats.Uptime, "1h 1m") {
		t.Fatalf("incorrect runtime stats: %+v", stats)
	}
	cfg := vici.ConnectionConfig{
		Name: "swanpanel-abc", ServerIdentity: "gateway.test", RemoteIdentity: "client.test",
		RemoteTS: []string{"dynamic"}, LocalTS: []string{"10.99.0.0/24"},
		Pools: []string{"vip-abc"}, ReauthTime: "10h", RekeyTimeIKE: "4h",
		RekeyTimeChild: "1h", MobikeEnabled: true, ForceEncapNATT: true,
		DPDDelay: "30s", DPDAction: "restart", Fragmentation: "yes",
		InactivityTimeout: "15m", IKEProposals: "aes256gcm16-prfsha384-ecp384",
		ESPProposals:       "aes256gcm16",
		AdvancedConnection: map[string]any{"unique": "replace", "send_cert": true, "vips": []any{"10.0.0.9"}},
		AdvancedChild:      map[string]any{"start_action": "none", "copy_dscp": true, "mark_in": []any{"20/20"}},
	}
	if err := client.LoadPool(ctx, "vip-abc", "10.20.0.2/32"); err != nil {
		t.Fatal(err)
	}
	pool := receiveCommand(t, server, "load-pool")
	if !bytes.Contains(pool, []byte("10.20.0.2/32")) {
		t.Fatalf("VIP pool not sent in VICI request: %q", pool)
	}
	if err := client.LoadConnectionConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	conn := receiveCommand(t, server, "load-conn")
	for _, key := range []string{"fragmentation", "reauth_time", "rekey_time", "inactivity", "start_action", "copy_dscp", "10.99.0.0/24", "client.test", "gateway.test", "vips", "send_cert", "always"} {
		if !bytes.Contains(conn, []byte(key)) {
			t.Errorf("VICI request missing %q", key)
		}
	}
	sessions, err := client.ListSAs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].IKEName != "swanpanel-abc" ||
		sessions[0].PeerIdentity != "client.test" || sessions[0].VirtualIP != "10.20.0.2" ||
		sessions[0].Established != 65 {
		t.Fatalf("malformed streamed session: %+v", sessions)
	}
	if err := client.TerminateSA(ctx, "swanpanel-abc"); err != nil {
		t.Fatal(err)
	}
	if err := client.UnloadConnection(ctx, "swanpanel-abc"); err != nil {
		t.Fatal(err)
	}
}

func TestVICIProtocolPoolGarbageCollectionPreservesExternalPools(t *testing.T) {
	server, path := startVICIServer(t, "")
	client := vici.NewClient(path)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	names, err := client.ListManagedPools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "swanpanel-vip-dev123" || names[1] != "swanpanel-vip-stale" {
		t.Fatalf("SwanPanel must only enumerate its own pools: %v", names)
	}
	if err := client.UnloadPool(ctx, "external-operator"); err == nil {
		t.Fatal("unmanaged pool unload unexpectedly accepted")
	}
	if err := client.UnloadPool(ctx, "swanpanel-vip-stale"); err != nil {
		t.Fatal(err)
	}
	request := receiveCommand(t, server, "unload-pool")
	if !bytes.Contains(request, []byte("swanpanel-vip-stale")) {
		t.Fatalf("wrong pool sent to VICI: %q", request)
	}

	_, listFailurePath := startVICIServer(t, "get-pools")
	listFailure := vici.NewClient(listFailurePath)
	if _, err := listFailure.ListManagedPools(ctx); err == nil {
		t.Fatal("VICI get-pools rejection returned a misleading empty pool list")
	}

	_, failurePath := startVICIServer(t, "unload-pool")
	failed := vici.NewClient(failurePath)
	if err := failed.UnloadPool(ctx, "swanpanel-vip-online"); err == nil {
		t.Fatal("charon rejection (for example a live lease) was ignored")
	}
}

func TestVICIProtocolRejectsRejectedCommandsAndInvalidConfig(t *testing.T) {
	for _, command := range []string{"load-pool", "load-conn", "terminate", "unload-conn"} {
		t.Run(command, func(t *testing.T) {
			_, path := startVICIServer(t, command)
			client := vici.NewClient(path)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var err error
			switch command {
			case "load-pool":
				err = client.LoadPool(ctx, "test", "10.0.0.2/32")
			case "load-conn":
				err = client.LoadConnectionConfig(ctx, vici.ConnectionConfig{
					RemoteIdentity: "client", RemoteTS: []string{"dynamic"}, Name: "test",
				})
			case "terminate":
				err = client.TerminateSA(ctx, "test")
			case "unload-conn":
				err = client.UnloadConnection(ctx, "test")
			}
			if err == nil {
				t.Fatalf("%s was rejected by VICI but returned success", command)
			}
		})
	}
	_, path := startVICIServer(t, "")
	client := vici.NewClient(path)
	for _, cfg := range []vici.ConnectionConfig{
		{Name: "no-remote", RemoteTS: []string{"dynamic"}},
		{Name: "no-ts", RemoteIdentity: "test"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := client.LoadConnectionConfig(ctx, cfg)
		cancel()
		if err == nil {
			t.Fatalf("invalid config %+v was accepted", cfg)
		}
	}
}

func TestVICIProtocolRestartsAndInvalidatesSocketSessions(t *testing.T) {
	server, path := startVICIServer(t, "")
	client := vici.NewClient(path)
	if err := client.CheckHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	server.mu.Lock()
	for _, conn := range server.conns {
		_ = conn.Close()
	}
	server.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.CheckHealth(ctx); err == nil {
		t.Fatal("closed socket incorrectly reported healthy")
	}
	if err := client.Reconnect(); err != nil {
		t.Fatal(fmt.Errorf("reconnecting VICI unix socket: %w", err))
	}
	if !client.IsConnected() {
		t.Fatal("reconnected daemon remained unhealthy")
	}
}

// The legacy map-based API cannot invent a remote identity and must remain
// fail-closed even when all other connection options are supplied.
func TestLegacyVICIConnectionParametersCannotBypassPerDeviceIdentity(t *testing.T) {
	_, path := startVICIServer(t, "")
	client := vici.NewClient(path)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, params := range []map[string]interface{}{
		{},
		{
			"reauth_time": "12h", "rekey_time": "4h", "dpd_delay": "30s", "dpd_timeout": "60s",
			"proposals": "aes256gcm16-prfsha384-ecp384", "esp_proposals": "aes256gcm16",
			"mobike": "yes", "encap": "true",
		},
		{"reauth_time": 10, "mobike": false, "encap": false},
	} {
		if err := client.LoadConnection(ctx, "legacy", params); err == nil {
			t.Fatalf("legacy configuration without explicit device identity accepted: %+v", params)
		}
	}
}

func TestManagedVICIConnectionsAndDaemonBootIdentity(t *testing.T) {
	_, path := startVICIServer(t, "")
	client := vici.NewClient(path)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	boot, err := client.DaemonBootID(ctx)
	if err != nil || boot == "" {
		t.Fatalf("healthy socket identity unavailable: %q %v", boot, err)
	}
	again, err := client.DaemonBootID(ctx)
	if err != nil || boot != again {
		t.Fatalf("healthy daemon identity changed without restart: %q / %q (%v)", boot, again, err)
	}
	names, err := client.ListManagedConnections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "swanpanel-dev123" || names[1] != "swanpanel-stale" {
		t.Fatalf("SwanPanel-owned connections should be isolated: %v", names)
	}
}

// The pidfile is intentionally injected via the same environment variable as
// nonstandard strongSwan host deployments; tests never edit /run or /var/run.
func TestDaemonStatsReadsConfiguredPIDAndProcessMemory(t *testing.T) {
	_, socket := startVICIServer(t, "")
	cleanup := vici.SetReadProcStatusForTest(func(name string) ([]byte, error) {
		return []byte("Name:\tcharon\nVmRSS:\t   20480 kB\n"), nil
	})
	defer cleanup()
	pidFile := filepath.Join(t.TempDir(), "charon.pid")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprint(os.Getpid())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWANPANEL_CHARON_PID_FILE", pidFile)
	client := vici.NewClient(socket)
	stats, err := client.GetDaemonStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.PID != os.Getpid() || stats.Status != "active" || stats.Workers != 8 || stats.ActiveSAs != 2 {
		t.Fatalf("daemon stats did not parse configured pid and VICI: %+v", stats)
	}
	if stats.Memory != "20.0 MB" {
		t.Fatalf("process memory was not read: %+v", stats)
	}
	if err := os.WriteFile(pidFile, []byte("not-a-pid"), 0600); err != nil {
		t.Fatal(err)
	}
	stats, err = client.GetDaemonStats(context.Background())
	if err != nil || stats.Status != "active" || stats.PID == os.Getpid() {
		t.Fatalf("invalid pid not ignored: %+v %v", stats, err)
	}
}

func TestDaemonStatsReportsFailedVICIRequestAndKeepsOfflineStatus(t *testing.T) {
	_, socket := startVICIServer(t, "stats")
	client := vici.NewClient(socket)
	stats, err := client.GetDaemonStats(context.Background())
	if err == nil || stats == nil || stats.Status == "active" {
		t.Fatalf("rejected daemon reported active: %+v %v", stats, err)
	}
	other := vici.NewClient(filepath.Join(t.TempDir(), "missing.vici"))
	stats, err = other.GetDaemonStats(context.Background())
	if err == nil || stats == nil || stats.Status != "inactive" {
		t.Fatalf("missing daemon reported active: %+v %v", stats, err)
	}
}

func TestDaemonStatsFallsBackToSinceWhenRunningSecondsAreNotNumeric(t *testing.T) {
	_, socket := startVICIServer(t, "stats-uses-since")
	client := vici.NewClient(socket)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stats, err := client.GetDaemonStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Status != "active" || stats.Uptime != "2026-11-15T00:00:00Z" {
		t.Fatalf("VICI since fallback was not reported: %+v", stats)
	}
}
