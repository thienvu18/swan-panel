package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

// Server encapsulates the embedded DNS engine
type Server struct {
	mu           sync.RWMutex
	udpServer    *dns.Server
	tcpServer    *dns.Server
	listenAddr   string
	database     *gorm.DB
	dnsClient    *dns.Client
	isRunning    bool
	cache        map[string]*dnsCacheEntry
}

type dnsCacheEntry struct {
	msg       *dns.Msg
	expiresAt time.Time
}

// NewServer creates a new instance of the embedded DNS server
func NewServer(listenAddr string, database *gorm.DB) *Server {
	if listenAddr == "" {
		listenAddr = ":53"
	}
	return &Server{
		listenAddr: listenAddr,
		database:   database,
		dnsClient: &dns.Client{
			Net:     "udp",
			Timeout: 2 * time.Second,
		},
		cache: make(map[string]*dnsCacheEntry),
	}
}

// Start begins listening on UDP and TCP
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		slog.Warn("DNS Server is already running")
		return nil
	}

	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handleDNSQuery)

	s.udpServer = &dns.Server{
		Addr:    s.listenAddr,
		Net:     "udp",
		Handler: mux,
	}

	s.tcpServer = &dns.Server{
		Addr:    s.listenAddr,
		Net:     "tcp",
		Handler: mux,
	}

	s.isRunning = true
	slog.Info("Starting embedded SwanPanel Split-DNS server", "addr", s.listenAddr)

	go func() {
		if err := s.udpServer.ListenAndServe(); err != nil && s.isRunning {
			slog.Warn("DNS UDP server stopped or port unavailable (continuing)", "error", err)
		}
	}()

	go func() {
		if err := s.tcpServer.ListenAndServe(); err != nil && s.isRunning {
			slog.Warn("DNS TCP server stopped or port unavailable (continuing)", "error", err)
		}
	}()

	return nil
}

// Stop terminates the DNS server
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return
	}
	s.isRunning = false
	if s.udpServer != nil {
		_ = s.udpServer.Shutdown()
	}
	if s.tcpServer != nil {
		_ = s.tcpServer.Shutdown()
	}
	slog.Info("Embedded DNS server shutdown completed")
}

// handleDNSQuery resolves queries using local MagicDNS, DNS Overrides, or upstream forwarders
func (s *Server) handleDNSQuery(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true
	msg.RecursionAvailable = true

	if len(r.Question) == 0 {
		_ = w.WriteMsg(msg)
		return
	}

	q := r.Question[0]
	qName := strings.ToLower(strings.TrimSuffix(q.Name, "."))
	qType := q.Qtype

	slog.Debug("DNS query received", "query", qName, "type", dns.TypeToString[qType])

	// 1. Fetch current DNS settings from DB
	var settings db.DNSSettings
	if err := s.database.First(&settings, 1).Error; err != nil {
		settings = db.DNSSettings{
			MagicDNSSuffix:    ".strongnet",
			UpstreamResolvers: "1.1.1.1, 8.8.8.8",
			IsMagicDNSEnabled: true,
		}
	}

	magicSuffix := strings.ToLower(strings.TrimPrefix(settings.MagicDNSSuffix, "."))

	// 2. Check MagicDNS (*.strongnet or configured suffix)
	if settings.IsMagicDNSEnabled && qType == dns.TypeA && strings.HasSuffix(qName, "."+magicSuffix) {
		hostname := strings.TrimSuffix(qName, "."+magicSuffix)

		var dev db.Device
		if err := s.database.Where("hostname = ? AND is_revoked = ?", hostname, false).First(&dev).Error; err == nil {
			ip := net.ParseIP(dev.VirtualIP)
			if ip != nil && ip.To4() != nil {
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    5,
					},
					A: ip.To4(),
				}
				msg.Answer = append(msg.Answer, rr)
				slog.Debug("MagicDNS resolved hostname directly to virtual IP", "hostname", hostname, "ip", dev.VirtualIP)
				_ = w.WriteMsg(msg)
				return
			}
		}
	}

	// 3. Check Custom DNS Overrides
	if qType == dns.TypeA {
		var override db.DNSOverride
		if err := s.database.Where("hostname = ?", qName).First(&override).Error; err == nil {
			ip := net.ParseIP(override.TargetAddress)
			if ip != nil && ip.To4() != nil {
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					A: ip.To4(),
				}
				msg.Answer = append(msg.Answer, rr)
				slog.Debug("DNS Override matched", "hostname", qName, "target", override.TargetAddress)
				_ = w.WriteMsg(msg)
				return
			}
		}
	}

	// 4. Forward to Upstream Resolvers
	upstreams := parseResolvers(settings.UpstreamResolvers)
	upstreamResp := s.forwardUpstream(r, upstreams)
	if upstreamResp != nil {
		_ = w.WriteMsg(upstreamResp)
		return
	}

	// Fallback to NXDOMAIN if cannot be resolved
	msg.Rcode = dns.RcodeNameError
	_ = w.WriteMsg(msg)
}

func parseResolvers(raw string) []string {
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			if !strings.Contains(trimmed, ":") {
				trimmed += ":53"
			}
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		result = []string{"1.1.1.1:53", "8.8.8.8:53"}
	}
	return result
}

func (s *Server) forwardUpstream(r *dns.Msg, upstreams []string) *dns.Msg {
	for _, upstream := range upstreams {
		resp, _, err := s.dnsClient.Exchange(r, upstream)
		if err == nil && resp != nil {
			return resp
		}
		slog.Debug("Upstream resolver failed, trying next", "resolver", upstream, "error", err)
	}
	return nil
}

// ResolveLocalQuery is a direct in-memory resolver function used for testing and local lookups
func (s *Server) ResolveLocalQuery(ctx context.Context, hostname string) (string, error) {
	var settings db.DNSSettings
	if err := s.database.First(&settings, 1).Error; err != nil {
		settings = db.DNSSettings{
			MagicDNSSuffix:    ".strongnet",
			UpstreamResolvers: "1.1.1.1, 8.8.8.8",
			IsMagicDNSEnabled: true,
		}
	}

	cleanHost := strings.ToLower(strings.TrimSpace(hostname))
	magicSuffix := strings.ToLower(strings.TrimPrefix(settings.MagicDNSSuffix, "."))

	if strings.HasSuffix(cleanHost, "."+magicSuffix) {
		devName := strings.TrimSuffix(cleanHost, "."+magicSuffix)
		var dev db.Device
		if err := s.database.WithContext(ctx).Where("hostname = ? AND is_revoked = ?", devName, false).First(&dev).Error; err == nil {
			return dev.VirtualIP, nil
		}
	}

	var override db.DNSOverride
	if err := s.database.WithContext(ctx).Where("hostname = ?", cleanHost).First(&override).Error; err == nil {
		return override.TargetAddress, nil
	}

	return "", fmt.Errorf("hostname %s not found in local DNS records", hostname)
}
