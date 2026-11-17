package dns

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	mdns "github.com/miekg/dns"
	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type captureDNSWriter struct{ response *mdns.Msg }

func (w *captureDNSWriter) LocalAddr() net.Addr          { return &net.UDPAddr{} }
func (w *captureDNSWriter) RemoteAddr() net.Addr         { return &net.UDPAddr{} }
func (w *captureDNSWriter) WriteMsg(msg *mdns.Msg) error { w.response = msg.Copy(); return nil }
func (w *captureDNSWriter) Write([]byte) (int, error)    { return 0, nil }
func (w *captureDNSWriter) Close() error                 { return nil }
func (w *captureDNSWriter) TsigStatus() error            { return nil }
func (w *captureDNSWriter) TsigTimersOnly(bool)          {}
func (w *captureDNSWriter) Hijack()                      {}

func setupDNSEdgeDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/dns.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&db.DNSSettings{}, &db.DNSOverride{}, &db.Device{}, &db.ResourceGroup{}); err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&db.DNSSettings{ID: 1, MagicDNSSuffix: ".corp.test",
		IsMagicDNSEnabled: true, UpstreamResolvers: "127.0.0.1:1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&db.ResourceGroup{ID: "edge-rg", Name: "Edge net", CIDRBlock: "10.88.0.0/24"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&db.Device{ID: "edge-device", Hostname: "edge-device", DeviceType: "company",
		ResourceGroupID: "edge-rg", IPAllocationType: "dynamic", VirtualIP: "10.88.0.2", Platform: "linux"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&db.DNSOverride{ID: "edge-override", Hostname: "service.corp.test", RecordType: "A", TargetAddress: "192.0.2.77"}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw, _ := database.DB(); _ = raw.Close() })
	return database
}
func dnsLookup(t *testing.T, s *Server, hostname string, qtype uint16) *mdns.Msg {
	t.Helper()
	req := new(mdns.Msg)
	req.SetQuestion(mdns.Fqdn(hostname), qtype)
	writer := &captureDNSWriter{}
	s.handleDNSQuery(writer, req)
	if writer.response == nil {
		t.Fatalf("no DNS response for %s", hostname)
	}
	return writer.response
}
func TestMagicDNSRespondsToExistingUnrevokedClientsAndOverrides(t *testing.T) {
	database := setupDNSEdgeDB(t)
	server := NewServer("", database)
	server.dnsClient.Timeout = 20 * time.Millisecond
	if server.listenAddr != ":53" {
		t.Fatalf("unexpected default DNS port %s", server.listenAddr)
	}
	for _, test := range []struct{ host, ip string }{
		{"edge-device.corp.test", "10.88.0.2"},
		{"service.corp.test", "192.0.2.77"},
	} {
		answer := dnsLookup(t, server, test.host, mdns.TypeA)
		if answer.Rcode != mdns.RcodeSuccess || len(answer.Answer) != 1 || !strings.Contains(answer.Answer[0].String(), test.ip) {
			t.Fatalf("%s returned unexpected DNS answer: %+v", test.host, answer)
		}
	}
	if err := database.Model(&db.Device{}).Where("id = ?", "edge-device").Update("is_revoked", true).Error; err != nil {
		t.Fatal(err)
	}
	revoked := dnsLookup(t, server, "edge-device.corp.test", mdns.TypeA)
	if revoked.Rcode != mdns.RcodeNameError {
		t.Fatalf("revoked device still resolved: %+v", revoked)
	}
	if result, err := server.ResolveLocalQuery(context.Background(), "edge-device.corp.test"); err == nil {
		t.Fatalf("revoked device still resolved locally: %s", result)
	}
	disabled := db.DNSSettings{}
	if err := database.First(&disabled, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&disabled).Update("is_magic_dns_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if result, err := server.ResolveLocalQuery(context.Background(), "edge-device.corp.test"); err == nil {
		t.Fatalf("disabled magic DNS still resolved: %s", result)
	}
}

func TestDNSUpstreamFailureReturnsSERVFAILAndNoQuestionDoesNotPanic(t *testing.T) {
	database := setupDNSEdgeDB(t)
	server := NewServer("127.0.0.1:0", database)
	server.dnsClient.Timeout = 10 * time.Millisecond
	response := dnsLookup(t, server, "not-found.example", mdns.TypeA)
	if response.Rcode != mdns.RcodeServerFailure || len(response.Answer) != 0 {
		t.Fatalf("unreachable upstream must return SERVFAIL: %+v", response)
	}
	response = dnsLookup(t, server, "edge-device.corp.test", mdns.TypeAAAA)
	if response.Rcode != mdns.RcodeSuccess || !response.Authoritative || len(response.Answer) != 0 {
		t.Fatalf("existing MagicDNS name without AAAA must return NODATA: %+v", response)
	}
	response = dnsLookup(t, server, "service.corp.test", mdns.TypeAAAA)
	if response.Rcode != mdns.RcodeSuccess || !response.Authoritative || len(response.Answer) != 0 {
		t.Fatalf("existing name without AAAA must return authoritative NODATA: %+v", response)
	}
	noQuestion := &captureDNSWriter{}
	server.handleDNSQuery(noQuestion, &mdns.Msg{MsgHdr: mdns.MsgHdr{Id: 35}})
	if noQuestion.response == nil {
		t.Fatal("empty question was not answered")
	}
	if got := parseResolvers("  ,  "); len(got) != 2 || got[0] != "1.1.1.1:53" {
		t.Fatalf("fallback upstreams incorrect: %+v", got)
	}
	if got := parseResolvers("127.0.0.1:1053,  8.8.8.8"); len(got) != 2 || got[1] != "8.8.8.8:53" {
		t.Fatalf("custom upstream parser incorrect: %+v", got)
	}
	server.Stop()
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	server.Stop()
	server.Stop()
}

func TestDNSForwardsToLocalUpstreamAndDoesNotLeakUnrelatedNames(t *testing.T) {
	database := setupDNSEdgeDB(t)
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	upstream := &mdns.Server{PacketConn: listener, Handler: mdns.HandlerFunc(func(w mdns.ResponseWriter, r *mdns.Msg) {
		result := new(mdns.Msg)
		result.SetReply(r)
		if len(r.Question) > 0 {
			rr, _ := mdns.NewRR(r.Question[0].Name + " 60 IN A 198.51.100.7")
			result.Answer = []mdns.RR{rr}
		}
		_ = w.WriteMsg(result)
	})}
	go func() { _ = upstream.ActivateAndServe() }()
	t.Cleanup(func() { _ = upstream.Shutdown() })
	if err := database.Model(&db.DNSSettings{}).Where("id = ?", 1).
		Update("upstream_resolvers", listener.LocalAddr().String()).Error; err != nil {
		t.Fatal(err)
	}
	server := NewServer("127.0.0.1:0", database)
	server.dnsClient.Timeout = time.Second
	for i := 0; i < 12; i++ {
		answer := dnsLookup(t, server, "external.example", mdns.TypeA)
		if answer.Rcode == mdns.RcodeSuccess && len(answer.Answer) == 1 {
			if !strings.Contains(answer.Answer[0].String(), "198.51.100.7") {
				t.Fatalf("wrong forwarded answer %+v", answer)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("local upstream forwarding failed")
}

func TestDNSWildcardExactPrecedenceAndMissingAAAA(t *testing.T) {
	database := setupDNSEdgeDB(t)
	records := []db.DNSOverride{
		{ID: "wildcard-a", Hostname: "*.example.test", RecordType: "A", TargetAddress: "192.0.2.10"},
		{ID: "exact-a", Hostname: "api.example.test", RecordType: "A", TargetAddress: "192.0.2.20"},
	}
	for _, record := range records {
		if err := database.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer("", database)
	for _, tc := range []struct{ host, address string }{
		{"api.example.test", "192.0.2.20"},
		{"nested.example.test", "192.0.2.10"},
		{"deep.nested.example.test", "192.0.2.10"},
	} {
		answer := dnsLookup(t, server, tc.host, mdns.TypeA)
		if answer.Rcode != mdns.RcodeSuccess || len(answer.Answer) != 1 ||
			!strings.Contains(answer.Answer[0].String(), tc.address) {
			t.Fatalf("override precedence for %s failed: %+v", tc.host, answer)
		}
	}
	for _, host := range []string{"api.example.test", "deep.nested.example.test"} {
		answer := dnsLookup(t, server, host, mdns.TypeAAAA)
		if answer.Rcode != mdns.RcodeSuccess || !answer.Authoritative || len(answer.Answer) != 0 {
			t.Fatalf("%s should return NODATA for missing AAAA: %+v", host, answer)
		}
	}
	unknownMagic := dnsLookup(t, server, "no-such-device.corp.test", mdns.TypeA)
	if unknownMagic.Rcode != mdns.RcodeNameError || !unknownMagic.Authoritative {
		t.Fatalf("unknown managed MagicDNS name must not reach upstream: %+v", unknownMagic)
	}
}
