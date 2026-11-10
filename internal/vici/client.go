package vici

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/strongswan/govici/vici"
)

// SessionInfo represents an active runtime IKE SA session
type SessionInfo struct {
	IKEName      string `json:"ike_name"`
	PeerIdentity string `json:"peer_identity"`
	VirtualIP    string `json:"virtual_ip"`
	RemoteWAN    string `json:"remote_wan"`
	Uptime       string `json:"uptime"`
	CipherSuite  string `json:"cipher_suite"`
	Established  int64  `json:"established_seconds"`
}

// Client wraps govici.Session with auto-reconnect and graceful simulation fallback
type Client struct {
	mu           sync.RWMutex
	socketPath   string
	session      *vici.Session
	isSimulated  bool
	mockSessions []SessionInfo
}

// DefaultClient is the package singleton client
var DefaultClient *Client
var once sync.Once

// GetClient initializes or returns the global VICI client
func GetClient(socketPath string) *Client {
	once.Do(func() {
		if socketPath == "" {
			socketPath = "/var/run/charon.vici"
		}
		DefaultClient = NewClient(socketPath)
	})
	return DefaultClient
}

// NewClient creates a new VICI client instance
func NewClient(socketPath string) *Client {
	c := &Client{
		socketPath: socketPath,
		mockSessions: []SessionInfo{
			{
				IKEName:      "ikev2-thienvu-macbook",
				PeerIdentity: "thienvu-macbook",
				VirtualIP:    "10.0.1.15",
				RemoteWAN:    "113.161.42.18:500",
				Uptime:       "3h 45m",
				CipherSuite:  "AES_GCM_256",
				Established:  13500,
			},
			{
				IKEName:      "ikev2-db-primary-prod",
				PeerIdentity: "db-primary-prod",
				VirtualIP:    "10.100.1.5",
				RemoteWAN:    "171.244.18.9:4500",
				Uptime:       "14d 6h",
				CipherSuite:  "AES_GCM_256",
				Established:  1231200,
			},
		},
	}
	c.connect()
	return c
}

func (c *Client) connect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if UNIX socket exists on filesystem
	if _, err := os.Stat(c.socketPath); err != nil {
		slog.Info("strongSwan VICI socket not detected, operating in development simulation mode", "socket", c.socketPath)
		c.isSimulated = true
		c.session = nil
		return
	}

	session, err := vici.NewSession(vici.WithSocketPath(c.socketPath))
	if err != nil {
		slog.Warn("Failed to connect to strongSwan charon.vici socket, falling back to simulated mode", "socket", c.socketPath, "error", err)
		c.isSimulated = true
		c.session = nil
		return
	}

	c.session = session
	c.isSimulated = false
	slog.Info("Successfully connected to strongSwan charon daemon via VICI socket", "socket", c.socketPath)
}

// IsConnected returns whether the client is connected to a live charon daemon
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.isSimulated && c.session != nil
}

// ListSAs queries active IKE and CHILD SAs
func (c *Client) ListSAs(ctx context.Context) ([]SessionInfo, error) {
	c.mu.RLock()
	isSim := c.isSimulated
	session := c.session
	c.mu.RUnlock()

	if isSim || session == nil {
		slog.Debug("Returning simulated active VICI sessions", "count", len(c.mockSessions))
		c.mu.RLock()
		defer c.mu.RUnlock()
		res := make([]SessionInfo, len(c.mockSessions))
		copy(res, c.mockSessions)
		return res, nil
	}

	slog.Debug("Querying live strongSwan SAs via VICI list-sas")
	req := vici.NewMessage()
	msgs, err := session.StreamedCommandRequest("list-sas", "list-sa", req)
	if err != nil {
		slog.Error("VICI list-sas command failed", "error", err)
		return nil, fmt.Errorf("failed to stream list-sas: %w", err)
	}

	var results []SessionInfo
	for _, m := range msgs {
		for _, ikeName := range m.Keys() {
			ikeMap := m.Get(ikeName)
			if ikeDict, ok := ikeMap.(*vici.Message); ok {
				peerID := ikeDict.Get("remote-id")
				remoteHost := ikeDict.Get("remote-host")
				establishedStr := ikeDict.Get("established")
				encrAlg := ikeDict.Get("encr-alg")

				vip := ""
				if vipsMsg, ok := ikeDict.Get("remote-vips").(*vici.Message); ok {
					for _, k := range vipsMsg.Keys() {
						vip = k
						break
					}
				}

				peerIDStr := ""
				if peerID != nil {
					peerIDStr = fmt.Sprintf("%v", peerID)
				}
				remoteHostStr := ""
				if remoteHost != nil {
					remoteHostStr = fmt.Sprintf("%v", remoteHost)
				}
				encrStr := "AES_GCM_256"
				if encrAlg != nil {
					encrStr = fmt.Sprintf("%v", encrAlg)
				}

				uptime := "Active"
				var establishedSec int64
				if establishedStr != nil {
					uptime = fmt.Sprintf("%vs", establishedStr)
				}

				results = append(results, SessionInfo{
					IKEName:      ikeName,
					PeerIdentity: peerIDStr,
					VirtualIP:    vip,
					RemoteWAN:    remoteHostStr,
					Uptime:       uptime,
					CipherSuite:  encrStr,
					Established:  establishedSec,
				})
			}
		}
	}

	slog.Info("Retrieved live sessions from strongSwan daemon", "count", len(results))
	return results, nil
}

// TerminateSA immediately closes an active IKE SA session
func (c *Client) TerminateSA(ctx context.Context, ikeName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	slog.Info("Terminating strongSwan IKE SA", "ike_name", ikeName)

	if c.isSimulated || c.session == nil {
		// Remove from mock list
		var updated []SessionInfo
		for _, s := range c.mockSessions {
			if s.IKEName != ikeName && s.PeerIdentity != ikeName {
				updated = append(updated, s)
			}
		}
		c.mockSessions = updated
		slog.Info("Simulated SA session terminated successfully", "ike_name", ikeName)
		return nil
	}

	msg := vici.NewMessage()
	if err := msg.Set("ike", ikeName); err != nil {
		return err
	}

	resp, err := c.session.CommandRequest("terminate", msg)
	if err != nil {
		slog.Error("VICI terminate command failed", "ike", ikeName, "error", err)
		return fmt.Errorf("failed to execute terminate: %w", err)
	}

	if success := resp.Get("success"); success != nil && success.(string) == "yes" {
		slog.Info("strongSwan SA terminated successfully", "ike", ikeName)
		return nil
	}

	slog.Warn("VICI terminate returned non-success response", "ike", ikeName, "errmsg", resp.Get("errmsg"))
	return nil
}

// LoadConnection dynamically loads connection parameters into strongSwan RAM without service restart
func (c *Client) LoadConnection(ctx context.Context, connName string, params map[string]interface{}) error {
	c.mu.RLock()
	isSim := c.isSimulated
	session := c.session
	c.mu.RUnlock()

	slog.Info("Loading dynamic connection into strongSwan RAM via VICI load-conn", "conn", connName)

	if isSim || session == nil {
		slog.Info("Simulated VICI load-conn executed successfully in RAM (zero downtime)", "conn", connName)
		return nil
	}

	msg := vici.NewMessage()
	subMsg := vici.NewMessage()
	for k, v := range params {
		if err := subMsg.Set(k, v); err != nil {
			slog.Warn("Failed to set param in VICI message", "key", k, "error", err)
		}
	}

	if err := msg.Set(connName, subMsg); err != nil {
		return err
	}

	resp, err := session.CommandRequest("load-conn", msg)
	if err != nil {
		slog.Error("VICI load-conn command failed", "conn", connName, "error", err)
		return fmt.Errorf("failed to load connection: %w", err)
	}

	if success := resp.Get("success"); success != nil && success.(string) == "yes" {
		slog.Info("Connection dynamically applied into strongSwan RAM with zero downtime", "conn", connName)
		return nil
	}

	return fmt.Errorf("VICI load-conn rejected: %v", resp.Get("errmsg"))
}
