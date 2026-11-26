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
	mu         sync.RWMutex
	listeners  map[string]*socketPair
	listenAddr string
	database   *gorm.DB
	dnsClient  *dns.Client
	isRunning  bool
}

// NewServer creates a new instance of the embedded DNS server
func NewServer(listenAddr string, database *gorm.DB) *Server {
	if listenAddr == "" {
		listenAddr = ":53"
	}
	return &Server{
		listenAddr: listenAddr,
		listeners:  make(map[string]*socketPair),
		database:   database,
		dnsClient: &dns.Client{
			Net:     "udp",
			Timeout: 2 * time.Second,
		},
	}
}

// Start begins listening on UDP and TCP. Binding errors are returned synchronously.
func (s *Server) Start() error {
	return s.SyncListeners([]string{s.listenAddr})
}

// Stop terminates all embedded DNS listeners.
func (s *Server) Stop() {
	s.mu.Lock()
	if !s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = false
	retired := s.listeners
	s.listeners = make(map[string]*socketPair)
	s.mu.Unlock()

	for _, pair := range retired {
		pair.stop()
	}
	slog.Info("Embedded DNS server shutdown completed")
}

// db returns the active database connection. If s.database's connection has been
// closed or replaced (for example, during atomic backup restoration), db transparently
// updates to the live global db.DB handle.
func (s *Server) db() *gorm.DB {
	s.mu.RLock()
	cur := s.database
	s.mu.RUnlock()

	if cur != nil {
		if sqlDB, err := cur.DB(); err == nil && sqlDB.Ping() == nil {
			return cur
		}
	}
	if db.DB != nil {
		s.mu.Lock()
		s.database = db.DB
		s.mu.Unlock()
		return db.DB
	}
	return cur
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
	if err := s.db().First(&settings, 1).Error; err != nil {
		settings = db.DNSSettings{
			MagicDNSSuffix:    ".strongnet",
			UpstreamResolvers: "1.1.1.1, 8.8.8.8",
			IsMagicDNSEnabled: true,
		}
	}

	magicSuffix := strings.ToLower(strings.TrimPrefix(settings.MagicDNSSuffix, "."))

	// 2. Check MagicDNS (*.strongnet or configured suffix)
	if settings.IsMagicDNSEnabled && strings.HasSuffix(qName, "."+magicSuffix) {
		hostname := strings.TrimSuffix(qName, "."+magicSuffix)

		var dev db.Device
		if err := s.db().Where("hostname = ? AND is_revoked = ?", hostname, false).First(&dev).Error; err == nil {
			if qType == dns.TypeA {
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
			// The name exists, but it has no record of the requested type.
			// Return authoritative NOERROR/NODATA without leaking to upstream.
			_ = w.WriteMsg(msg)
			return
		}
	}

	// 3. Check Custom DNS Overrides (exact FQDN match or standard wildcard *.domain)
	var override db.DNSOverride
	found := false
	if err := s.db().Where("hostname = ?", qName).First(&override).Error; err == nil {
		found = true
	} else {
		// Standard wildcard matching: e.g. for "a.b.google.com", check "*.b.google.com", then "*.google.com"
		curr := qName
		for {
			dotIdx := strings.Index(curr, ".")
			if dotIdx <= 0 || dotIdx >= len(curr)-1 {
				break
			}
			wildcardHost := "*" + curr[dotIdx:]
			if err := s.db().Where("hostname = ?", wildcardHost).First(&override).Error; err == nil {
				found = true
				break
			}
			curr = curr[dotIdx+1:]
		}
	}
	if found {
		targetIP := net.ParseIP(override.TargetAddress)
		if qType == dns.TypeA && targetIP != nil && targetIP.To4() != nil {
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				A: targetIP.To4(),
			}
			msg.Answer = append(msg.Answer, rr)
			slog.Debug("DNS Override matched A", "hostname", qName, "target", override.TargetAddress)
			_ = w.WriteMsg(msg)
			return
		}
		if qType == dns.TypeAAAA && targetIP != nil && targetIP.To16() != nil && targetIP.To4() == nil {
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				AAAA: targetIP.To16(),
			}
			msg.Answer = append(msg.Answer, rr)
			slog.Debug("DNS Override matched AAAA", "hostname", qName, "target", override.TargetAddress)
			_ = w.WriteMsg(msg)
			return
		}
		// If an override is configured for this hostname but for a different record type (e.g. A exists, AAAA queried),
		// do NOT forward to upstream to prevent leaking real upstream addresses.
		// The name exists, so the correct response is NOERROR with no answers (NODATA).
		slog.Debug("DNS Override exists for hostname; returning NODATA for unconfigured type", "hostname", qName, "type", dns.TypeToString[qType])
		_ = w.WriteMsg(msg)
		return
	}

	// Managed MagicDNS names never escape to public resolvers, even when
	// the device is unknown or revoked. Other local overrides took priority.
	if settings.IsMagicDNSEnabled && strings.HasSuffix(qName, "."+magicSuffix) {
		msg.Rcode = dns.RcodeNameError
		_ = w.WriteMsg(msg)
		return
	}

	// 4. Forward to Upstream Resolvers
	upstreams := parseResolvers(settings.UpstreamResolvers)
	upstreamResp := s.forwardUpstream(r, upstreams)
	if upstreamResp != nil {
		_ = w.WriteMsg(upstreamResp)
		return
	}

	// Resolver outage is not proof a public name does not exist.
	msg.Authoritative = false
	msg.Rcode = dns.RcodeServerFailure
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
	if err := s.db().First(&settings, 1).Error; err != nil {
		settings = db.DNSSettings{
			MagicDNSSuffix:    ".strongnet",
			UpstreamResolvers: "1.1.1.1, 8.8.8.8",
			IsMagicDNSEnabled: true,
		}
	}

	cleanHost := strings.ToLower(strings.TrimSpace(hostname))
	magicSuffix := strings.ToLower(strings.TrimPrefix(settings.MagicDNSSuffix, "."))

	if settings.IsMagicDNSEnabled && strings.HasSuffix(cleanHost, "."+magicSuffix) {
		devName := strings.TrimSuffix(cleanHost, "."+magicSuffix)
		var dev db.Device
		if err := s.db().WithContext(ctx).Where("hostname = ? AND is_revoked = ?", devName, false).First(&dev).Error; err == nil {
			return dev.VirtualIP, nil
		}
	}

	var override db.DNSOverride
	if err := s.db().WithContext(ctx).Where("hostname = ?", cleanHost).First(&override).Error; err == nil {
		return override.TargetAddress, nil
	}
	// Check wildcard match
	curr := cleanHost
	for {
		dotIdx := strings.Index(curr, ".")
		if dotIdx <= 0 || dotIdx >= len(curr)-1 {
			break
		}
		wildcardHost := "*" + curr[dotIdx:]
		if err := s.db().WithContext(ctx).Where("hostname = ?", wildcardHost).First(&override).Error; err == nil {
			return override.TargetAddress, nil
		}
		curr = curr[dotIdx+1:]
	}

	return "", fmt.Errorf("hostname %s not found in local DNS records", hostname)
}
