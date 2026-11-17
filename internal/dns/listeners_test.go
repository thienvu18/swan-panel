package dns_test

import (
	"fmt"
	"net"
	"testing"

	"github.com/miekg/dns"
	swanDNS "github.com/swan-panel/swan-panel/internal/dns"
)

func freeDNSAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return addr
}

func assertDNSResponse(t *testing.T, addr string) {
	t.Helper()
	for _, protocol := range []string{"udp", "tcp"} {
		client := &dns.Client{Net: protocol}
		query := new(dns.Msg)
		query.SetQuestion("thienvu-macbook.strongnet.", dns.TypeA)
		response, _, err := client.Exchange(query, addr)

		if err != nil {
			t.Fatalf("%s DNS query to %s: %v", protocol, addr, err)
		}
		if len(response.Answer) != 1 {
			t.Fatalf("%s DNS response answers = %d, want 1", protocol, len(response.Answer))
		}
	}
}

func TestDNSStartFailsWhenTCPPortOccupiedAndCanRetry(t *testing.T) {
	db := setupTestDB(t)
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := occupied.Addr().String()
	server := swanDNS.NewServer(addr, db)
	defer server.Stop()
	if err := server.Start(); err == nil {
		t.Fatal("DNS Start reported success with TCP port already occupied")
	}
	_ = occupied.Close()
	if err := server.Start(); err != nil {
		t.Fatalf("DNS retry after releasing port failed: %v", err)
	}
	assertDNSResponse(t, addr)
}

func TestDNSSyncPreservesExistingListenerOnBindFailure(t *testing.T) {
	db := setupTestDB(t)
	addr := freeDNSAddress(t)
	server := swanDNS.NewServer(addr, db)
	defer server.Stop()
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	if err := server.SyncListeners([]string{addr, occupied.Addr().String()}); err == nil {
		t.Fatal("expected a bind failure for new occupied listener")
	}
	assertDNSResponse(t, addr)

	if err := server.SyncListeners([]string{addr}); err != nil {
		t.Fatalf("sync existing address failed: %v", err)
	}
	assertDNSResponse(t, addr)

	if err := server.SyncListeners(nil); err != nil {
		t.Fatal(err)
	}
	probe, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(fmt.Errorf("retired DNS TCP port remained occupied: %w", err))
	}
	_ = probe.Close()
}

func TestDNSHostVIPCoexistsWithOtherLoopbackResolver(t *testing.T) {
	// Model systemd-resolved on 127.0.0.53 and SwanPanel on a separate VIP.
	portAddr := freeDNSAddress(t)
	_, port, _ := net.SplitHostPort(portAddr)
	otherAddr := net.JoinHostPort("127.0.0.53", port)
	otherTCP, err := net.Listen("tcp", otherAddr)
	if err != nil {
		t.Skipf("host cannot assign second loopback address: %v", err)
	}
	defer otherTCP.Close()
	otherUDP, err := net.ListenPacket("udp", otherAddr)
	if err != nil {
		t.Skipf("host cannot bind UDP on second loopback address: %v", err)
	}
	defer otherUDP.Close()

	server := swanDNS.NewServer(portAddr, setupTestDB(t))
	defer server.Stop()
	if err := server.Start(); err != nil {
		t.Fatalf("binding managed VIP alongside resolver on %s failed: %v", otherAddr, err)
	}
	assertDNSResponse(t, portAddr)
}
