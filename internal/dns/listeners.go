package dns

import (
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strings"

	"github.com/miekg/dns"
)

// socketPair owns both transports for one DNS address.
type socketPair struct {
	udp      *dns.Server
	tcp      *dns.Server
	packet   net.PacketConn
	listener net.Listener
}

func (p *socketPair) closeSockets() {
	_ = p.packet.Close()
	_ = p.listener.Close()
}

func (p *socketPair) stop() {
	_ = p.udp.Shutdown()
	_ = p.tcp.Shutdown()
}

// SyncListeners atomically adds DNS listeners before retiring obsolete ones.
// Host strongSwan mode uses only assigned gateway VIPs, leaving the host
// system resolver (for example 127.0.0.53:53) untouched.
func (s *Server) SyncListeners(addrs []string) error {
	wanted := make(map[string]bool, len(addrs))
	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if _, _, err := net.SplitHostPort(addr); err != nil {
			return fmt.Errorf("invalid DNS listen address %q: %w", addr, err)
		}
		wanted[addr] = true
	}
	ordered := make([]string, 0, len(wanted))
	for addr := range wanted {
		ordered = append(ordered, addr)
	}
	sort.Strings(ordered)

	s.mu.Lock()
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handleDNSQuery)
	staged := make(map[string]*socketPair)
	for _, addr := range ordered {
		if s.listeners[addr] != nil {
			continue
		}
		packet, err := net.ListenPacket("udp", addr)
		if err != nil {
			for _, pair := range staged {
				pair.closeSockets()
			}
			s.mu.Unlock()
			return fmt.Errorf("bind DNS UDP %s: %w", addr, err)
		}
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			_ = packet.Close()
			for _, pair := range staged {
				pair.closeSockets()
			}
			s.mu.Unlock()
			return fmt.Errorf("bind DNS TCP %s: %w", addr, err)
		}
		staged[addr] = &socketPair{
			udp:    &dns.Server{PacketConn: packet, Handler: mux},
			tcp:    &dns.Server{Listener: listener, Handler: mux},
			packet: packet, listener: listener,
		}
	}
	s.isRunning = true
	for addr, pair := range staged {
		s.listeners[addr] = pair
		go s.serveListener(addr, "udp", pair, pair.udp)
		go s.serveListener(addr, "tcp", pair, pair.tcp)
		slog.Info("Embedded DNS listener ready", "address", addr)
	}
	var retired []*socketPair
	for addr, pair := range s.listeners {
		if !wanted[addr] {
			delete(s.listeners, addr)
			retired = append(retired, pair)
			slog.Info("Retiring embedded DNS listener", "address", addr)
		}
	}
	s.mu.Unlock()
	for _, pair := range retired {
		pair.stop()
	}
	return nil
}

// A failed serving goroutine must not leave a dead listener marked healthy.
// The next SyncListeners call will rebind the missing address.
func (s *Server) serveListener(addr, protocol string, pair *socketPair, server *dns.Server) {
	if err := server.ActivateAndServe(); err != nil {
		s.mu.Lock()
		active := s.isRunning && s.listeners[addr] == pair
		if active {
			delete(s.listeners, addr)
		}
		s.mu.Unlock()
		if active {
			pair.stop()
			slog.Error("Embedded DNS listener stopped unexpectedly", "address", addr, "transport", protocol, "error", err)
		}
	}
}
