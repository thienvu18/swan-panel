package vici

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

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

// ConnectionConfig defines parameters for dynamic strongSwan IKEv2 connection loading
type ConnectionConfig struct {
	Name               string   `json:"name"`
	ServerIdentity     string   `json:"server_identity"`
	ReauthTime         string   `json:"reauth_time"`
	RekeyTimeIKE       string   `json:"rekey_time_ike"`
	RekeyTimeChild     string   `json:"rekey_time_child"`
	MobikeEnabled      bool     `json:"mobike_enabled"`
	ForceEncapNATT     bool     `json:"force_encap_natt"`
	DPDDelay           string   `json:"dpd_delay"`
	DPDTimeout         string   `json:"dpd_timeout"`
	IKEProposals       string   `json:"ike_proposals"`
	ESPProposals       string   `json:"esp_proposals"`
	Pools              []string `json:"pools"`
	RemoteIdentity     string   `json:"remote_identity"`
	RemoteTS           []string `json:"remote_ts"`
	LocalTS            []string `json:"local_ts"`
	DPDAction          string   `json:"dpd_action"`
	Fragmentation      string
	InactivityTimeout  string
	AdvancedConnection map[string]any
	AdvancedChild      map[string]any
}

// DaemonStats represents live charon daemon runtime statistics
type DaemonStats struct {
	Service   string `json:"service"`
	Status    string `json:"status"`
	PID       int    `json:"pid"`
	Memory    string `json:"memory"`
	Uptime    string `json:"uptime"`
	Workers   int    `json:"workers"`
	ActiveSAs int    `json:"active_sas"`
	// Populated by the API: an external host daemon cannot be controlled
	// safely through the container's local ipsec/systemctl binaries.
	ControlSupported bool `json:"control_supported"`
}

// Client wraps govici.Session with thread-safe auto-reconnect and robust VICI command handling
type Client struct {
	mu         sync.RWMutex
	socketPath string
	session    *vici.Session
}

// DefaultClient is the package singleton client
var DefaultClient *Client
var once sync.Once
var readProcStatus = os.ReadFile

// GetClient initializes or returns the global VICI client
func GetClient(socketPath string) *Client {
	once.Do(func() {
		if socketPath == "" {
			socketPath = resolveSocketPath("")
		}
		DefaultClient = NewClient(socketPath)
	})
	return DefaultClient
}

// resolveSocketPath discovers the active VICI socket on the filesystem
func resolveSocketPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	candidates := []string{
		os.Getenv("VICI_SOCKET"),
		"/var/run/charon.vici",
		"/run/charon.vici",
	}
	for _, p := range candidates {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv("VICI_SOCKET"); env != "" {
		return env
	}
	return "/var/run/charon.vici"
}

// NewClient creates a new VICI client instance
func NewClient(socketPath string) *Client {
	c := &Client{socketPath: socketPath}
	_, _ = c.getSession()
	return c
}

// getSession retrieves or establishes an active VICI session
func (c *Client) getSession() (*vici.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.session != nil {
		return c.session, nil
	}

	targetSocket := resolveSocketPath(c.socketPath)
	c.socketPath = targetSocket

	if _, err := os.Stat(targetSocket); err != nil {
		c.session = nil
		slog.Debug("strongSwan VICI socket not found on filesystem", "socket", targetSocket)
		return nil, fmt.Errorf("VICI socket not found at %s: %w", targetSocket, err)
	}

	session, err := vici.NewSession(vici.WithSocketPath(targetSocket))
	if err != nil {
		c.session = nil
		slog.Warn("Failed to establish VICI connection to strongSwan daemon", "socket", targetSocket, "error", err)
		return nil, fmt.Errorf("failed to connect to %s: %w", targetSocket, err)
	}

	c.session = session
	slog.Info("Successfully established VICI session with strongSwan charon daemon", "socket", targetSocket)
	return c.session, nil
}

// invalidateSession closes and clears the cached session after IPC failure
func (c *Client) invalidateSession() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		_ = c.session.Close()
		c.session = nil
	}
}

// Reconnect closes existing session and tries to establish a new connection
func (c *Client) Reconnect() error {
	c.invalidateSession()
	_, err := c.getSession()
	return err
}

// IsConnected returns whether the client is currently connected to a live charon daemon
func (c *Client) IsConnected() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return c.CheckHealth(ctx) == nil
}

// CheckHealth uses a live VICI command; a cached socket is not proof of health.
func (c *Client) CheckHealth(ctx context.Context) error {
	session, err := c.getSession()
	if err != nil {
		return err
	}
	resp, err := session.Call(ctx, "stats", vici.NewMessage())
	if err != nil {
		c.invalidateSession()
		return fmt.Errorf("VICI health check failed: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("empty VICI health response")
	}
	return nil
}

// DaemonBootID tracks the UNIX socket inode that changes when charon restarts,
// even if the desired database configuration has not changed.
func (c *Client) DaemonBootID(ctx context.Context) (string, error) {
	if err := c.CheckHealth(ctx); err != nil {
		return "", err
	}
	c.mu.RLock()
	path := c.socketPath
	c.mu.RUnlock()
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%d-%d", stat.Ino, info.ModTime().UnixNano()), nil
	}
	return fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano()), nil
}

// ListSAs queries active IKE and CHILD SAs
func (c *Client) ListSAs(ctx context.Context) ([]SessionInfo, error) {
	session, err := c.getSession()
	if err != nil {
		return nil, err
	}

	slog.Debug("Querying live strongSwan SAs via VICI list-sas")
	req := vici.NewMessage()
	var results []SessionInfo
	for m, streamErr := range session.CallStreaming(ctx, "list-sas", "list-sa", req) {
		if streamErr != nil {
			c.invalidateSession()
			slog.Error("VICI list-sas command failed, invalidated session", "error", streamErr)
			return nil, fmt.Errorf("failed to stream list-sas: %w", streamErr)
		}

		for _, ikeName := range m.Keys() {
			ikeMap := m.Get(ikeName)
			if ikeDict, ok := ikeMap.(*vici.Message); ok {
				peerID := ikeDict.Get("remote-id")
				remoteHost := ikeDict.Get("remote-host")
				establishedStr := ikeDict.Get("established")
				encrAlg := ikeDict.Get("encr-alg")

				vip := ""
				if vips, ok := ikeDict.Get("remote-vips").([]string); ok && len(vips) > 0 {
					vip = vips[0]
				} else if vipsIfs, ok := ikeDict.Get("remote-vips").([]interface{}); ok && len(vipsIfs) > 0 {
					vip = fmt.Sprintf("%v", vipsIfs[0])
				} else if vipsMsg, ok := ikeDict.Get("remote-vips").(*vici.Message); ok {
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
					if s, err := strconv.ParseInt(fmt.Sprintf("%v", establishedStr), 10, 64); err == nil {
						establishedSec = s
					}
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
	session, err := c.getSession()
	if err != nil {
		return err
	}

	slog.Info("Terminating strongSwan IKE SA via VICI", "ike_name", ikeName)
	msg := vici.NewMessage()
	if err := msg.Set("ike", ikeName); err != nil {
		return err
	}

	resp, err := session.Call(ctx, "terminate", msg)
	if err != nil {
		c.invalidateSession()
		slog.Error("VICI terminate command failed", "ike", ikeName, "error", err)
		return fmt.Errorf("failed to execute terminate: %w", err)
	}

	if success := resp.Get("success"); success != nil && success.(string) == "yes" {
		return nil
	}
	return fmt.Errorf("VICI termination rejected for %s: %v", ikeName, resp.Get("errmsg"))
}

// LoadConnectionConfig dynamically loads strongSwan connection parameters into RAM via VICI load-conn
func (c *Client) LoadConnectionConfig(ctx context.Context, cfg ConnectionConfig) error {
	session, err := c.getSession()
	if err != nil {
		return fmt.Errorf("failed to get VICI session: %w", err)
	}

	connName := cfg.Name
	if connName == "" {
		connName = "swanpanel-ikev2"
	}

	slog.Info("Loading dynamic connection into strongSwan RAM via VICI load-conn", "conn", connName)

	connMsg := vici.NewMessage()
	_ = connMsg.Set("version", "2")

	if cfg.ReauthTime != "" {
		_ = connMsg.Set("reauth_time", cfg.ReauthTime)
	}
	if cfg.RekeyTimeIKE != "" {
		_ = connMsg.Set("rekey_time", cfg.RekeyTimeIKE)
	}
	if cfg.MobikeEnabled {
		_ = connMsg.Set("mobike", "yes")
	} else {
		_ = connMsg.Set("mobike", "no")
	}
	if cfg.ForceEncapNATT {
		_ = connMsg.Set("encap", "yes")
	} else {
		_ = connMsg.Set("encap", "no")
	}
	if cfg.DPDDelay != "" {
		_ = connMsg.Set("dpd_delay", cfg.DPDDelay)
	}

	if cfg.Fragmentation != "" {
		if err := connMsg.Set("fragmentation", cfg.Fragmentation); err != nil {
			return err
		}
	}
	for key, value := range cfg.AdvancedConnection {
		switch v := value.(type) {
		case bool:
			if v {
				value = "yes"
			} else {
				value = "no"
			}
		case []any:
			items := make([]string, 0, len(v))
			for _, s := range v {
				items = append(items, s.(string))
			}
			value = items
		}
		if err := connMsg.Set(key, value); err != nil {
			return fmt.Errorf("advanced connection option %s: %w", key, err)
		}
	}
	// Proposals: govici encodes []string as a VICI LIST
	if cfg.IKEProposals != "" {
		var ikeProps []string
		for _, p := range strings.Split(cfg.IKEProposals, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				ikeProps = append(ikeProps, p)
			}
		}
		if len(ikeProps) > 0 {
			_ = connMsg.Set("proposals", ikeProps)
		}
	}

	// Pools: link IP address pool for client virtual IP assignment
	pools := cfg.Pools
	if len(pools) == 0 {
		pools = []string{"swanpanel-pool"}
	}
	_ = connMsg.Set("pools", pools)

	// Ensure gateway unconditionally sends certificate payloads so iOS/macOS native clients authenticate
	_ = connMsg.Set("send_cert", "always")

	// Local authentication round
	localMsg := vici.NewMessage()
	_ = localMsg.Set("auth", "pubkey")
	if cfg.ServerIdentity != "" {
		_ = localMsg.Set("id", cfg.ServerIdentity)
	} else {
		_ = localMsg.Set("id", "swan-panel")
	}
	_ = connMsg.Set("local", localMsg)

	// Remote authentication round
	remoteMsg := vici.NewMessage()
	_ = remoteMsg.Set("auth", "pubkey")
	if cfg.RemoteIdentity == "" {
		return fmt.Errorf("remote identity must be explicit for each device")
	}
	_ = remoteMsg.Set("id", cfg.RemoteIdentity)
	_ = remoteMsg.Set("revocation", "strict")
	_ = connMsg.Set("remote", remoteMsg)

	// Child SA configuration
	childMsg := vici.NewMessage()
	_ = childMsg.Set("mode", "tunnel")
	if len(cfg.RemoteTS) == 0 {
		return fmt.Errorf("remote traffic selector required")
	}
	_ = childMsg.Set("remote_ts", cfg.RemoteTS)
	if len(cfg.LocalTS) > 0 {
		_ = childMsg.Set("local_ts", cfg.LocalTS)
	} else {
		// Tunnel may come up without grants, but nftables drops all forwarded traffic.
		_ = childMsg.Set("local_ts", []string{"0.0.0.0/32"})
	}
	if cfg.ESPProposals != "" {
		var espProps []string
		for _, p := range strings.Split(cfg.ESPProposals, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				espProps = append(espProps, p)
			}
		}
		if len(espProps) > 0 {
			_ = childMsg.Set("esp_proposals", espProps)
		}
	}
	if cfg.InactivityTimeout != "" {
		if err := childMsg.Set("inactivity", cfg.InactivityTimeout); err != nil {
			return err
		}
	}
	for key, value := range cfg.AdvancedChild {
		switch v := value.(type) {
		case bool:
			if v {
				value = "yes"
			} else {
				value = "no"
			}
		case []any:
			items := make([]string, 0, len(v))
			for _, s := range v {
				items = append(items, s.(string))
			}
			value = items
		}
		if err := childMsg.Set(key, value); err != nil {
			return fmt.Errorf("advanced child option %s: %w", key, err)
		}
	}
	if cfg.RekeyTimeChild != "" {
		_ = childMsg.Set("rekey_time", cfg.RekeyTimeChild)
	}
	if cfg.DPDAction != "" {
		_ = childMsg.Set("dpd_action", cfg.DPDAction)
	} else {
		_ = childMsg.Set("dpd_action", "clear")
	}

	childrenMsg := vici.NewMessage()
	_ = childrenMsg.Set("net", childMsg)
	_ = connMsg.Set("children", childrenMsg)

	rootMsg := vici.NewMessage()
	_ = rootMsg.Set(connName, connMsg)

	resp, err := session.Call(ctx, "load-conn", rootMsg)
	if err != nil {
		c.invalidateSession()
		slog.Error("VICI load-conn command failed", "conn", connName, "error", err)
		return fmt.Errorf("failed to load connection: %w", err)
	}

	if success := resp.Get("success"); success != nil && success.(string) == "yes" {
		slog.Info("Connection dynamically applied into strongSwan RAM with zero downtime", "conn", connName)
		return nil
	}

	errMsg := resp.Get("errmsg")
	slog.Error("VICI load-conn rejected by strongSwan daemon", "conn", connName, "errmsg", errMsg)
	return fmt.Errorf("VICI load-conn rejected: %v", errMsg)
}

// LoadConnection provides backwards compatibility for map-based parameters
func (c *Client) LoadConnection(ctx context.Context, connName string, params map[string]interface{}) error {
	cfg := ConnectionConfig{Name: connName}
	if v, ok := params["reauth_time"].(string); ok {
		cfg.ReauthTime = v
	}
	if v, ok := params["rekey_time"].(string); ok {
		cfg.RekeyTimeIKE = v
	}
	if v, ok := params["dpd_delay"].(string); ok {
		cfg.DPDDelay = v
	}
	if v, ok := params["dpd_timeout"].(string); ok {
		cfg.DPDTimeout = v
	}
	if v, ok := params["proposals"].(string); ok {
		cfg.IKEProposals = v
	}
	if v, ok := params["esp_proposals"].(string); ok {
		cfg.ESPProposals = v
	}
	if v, ok := params["mobike"].(string); ok {
		cfg.MobikeEnabled = v == "yes" || v == "true"
	}
	if v, ok := params["encap"].(string); ok {
		cfg.ForceEncapNATT = v == "yes" || v == "true"
	}
	return c.LoadConnectionConfig(ctx, cfg)
}

// LoadPool dynamically configures an IP address pool in strongSwan RAM via VICI load-pool
func (c *Client) LoadPool(ctx context.Context, poolName, cidr string) error {
	return c.LoadPoolWithDNS(ctx, poolName, cidr, nil)
}

// LoadPoolWithDNS dynamically configures an IP address pool and DNS server attributes in strongSwan RAM via VICI load-pool
func (c *Client) LoadPoolWithDNS(ctx context.Context, poolName, cidr string, dnsServers []string) error {
	session, err := c.getSession()
	if err != nil {
		return err
	}

	slog.Info("Loading IP pool into strongSwan RAM via VICI", "pool", poolName, "cidr", cidr, "dns", dnsServers)
	subMsg := vici.NewMessage()
	_ = subMsg.Set("addrs", cidr)
	if len(dnsServers) > 0 {
		_ = subMsg.Set("dns", dnsServers)
	}
	rootMsg := vici.NewMessage()
	_ = rootMsg.Set(poolName, subMsg)

	resp, err := session.Call(ctx, "load-pool", rootMsg)
	if err != nil {
		c.invalidateSession()
		slog.Error("VICI load-pool command failed", "pool", poolName, "error", err)
		return fmt.Errorf("failed to load pool: %w", err)
	}

	if success := resp.Get("success"); success != nil && success.(string) == "yes" {
		slog.Info("IP Pool loaded into strongSwan RAM successfully", "pool", poolName, "cidr", cidr, "dns", dnsServers)
		return nil
	}

	return fmt.Errorf("VICI load-pool rejected: %v", resp.Get("errmsg"))
}

// ListManagedPools only reports pools using SwanPanel's reserved device prefix.
// It never considers pools created by another strongSwan operator.
func (c *Client) ListManagedPools(ctx context.Context) ([]string, error) {
	session, err := c.getSession()
	if err != nil {
		return nil, err
	}
	response, err := session.Call(ctx, "get-pools", vici.NewMessage())
	if err != nil {
		c.invalidateSession()
		return nil, fmt.Errorf("list strongSwan pools: %w", err)
	}
	if response == nil {
		return nil, fmt.Errorf("empty VICI pool listing")
	}
	if response.Get("success") == "no" {
		return nil, fmt.Errorf("VICI get-pools rejected: %v", response.Get("errmsg"))
	}
	var names []string
	for _, name := range response.Keys() {
		if strings.HasPrefix(name, "swanpanel-vip-") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// UnloadPool is safe for live clients: charon rejects removal of a pool with
// an online lease. Callers should defer cleanup until clients disconnect.
func (c *Client) UnloadPool(ctx context.Context, name string) error {
	if !strings.HasPrefix(name, "swanpanel-vip-") {
		return fmt.Errorf("refusing to unload unmanaged pool")
	}
	session, err := c.getSession()
	if err != nil {
		return err
	}
	request := vici.NewMessage()
	if err := request.Set("name", name); err != nil {
		return err
	}
	response, err := session.Call(ctx, "unload-pool", request)
	if err != nil {
		c.invalidateSession()
		return fmt.Errorf("VICI unload-pool %s: %w", name, err)
	}
	if response == nil || response.Get("success") != "yes" {
		return fmt.Errorf("VICI unload-pool %s: %v", name, response)
	}
	return nil
}

// GetDaemonStats returns live charon runtime metrics via VICI stats and Linux /proc
func (c *Client) GetDaemonStats(ctx context.Context) (*DaemonStats, error) {
	stats := &DaemonStats{
		Service: "strongswan",
		Status:  "inactive",
		PID:     0,
		Memory:  "0 MB",
		Uptime:  "Offline",
	}

	// 1. Read PID from strongSwan pidfiles
	pidPaths := []string{
		os.Getenv("SWANPANEL_CHARON_PID_FILE"),
		"/run/charon.pid",
		"/var/run/charon.pid",
		"/var/run/starter.charon.pid",
		"/run/starter.charon.pid",
	}
	for _, p := range pidPaths {
		if p == "" {
			continue
		}
		if data, err := os.ReadFile(p); err == nil {
			trimmed := strings.TrimSpace(string(data))
			if pid, err := strconv.Atoi(trimmed); err == nil && pid > 0 {
				stats.PID = pid
				break
			}
		}
	}

	// 2. Query VICI stats command
	session, err := c.getSession()
	if err != nil {
		return stats, err
	}
	if session != nil {
		req := vici.NewMessage()
		resp, err := session.Call(ctx, "stats", req)
		if err != nil {
			c.invalidateSession()
			return stats, fmt.Errorf("VICI stats: %w", err)
		}
		if err == nil && resp != nil {
			stats.Status = "active"

			type rawStats struct {
				Uptime struct {
					Running string `vici:"running"`
					Since   string `vici:"since"`
				} `vici:"uptime"`
				Workers struct {
					Total string `vici:"total"`
				} `vici:"workers"`
				Ikesas struct {
					Total string `vici:"total"`
				} `vici:"ikesas"`
			}

			var parsed rawStats
			if err := vici.UnmarshalMessage(resp, &parsed); err == nil {
				if secs, err := strconv.ParseInt(parsed.Uptime.Running, 10, 64); err == nil && secs >= 0 {
					stats.Uptime = formatDuration(time.Duration(secs) * time.Second)
				} else if parsed.Uptime.Since != "" {
					stats.Uptime = parsed.Uptime.Since
				}
				if tot, err := strconv.Atoi(parsed.Workers.Total); err == nil {
					stats.Workers = tot
				}
				if tot, err := strconv.Atoi(parsed.Ikesas.Total); err == nil {
					stats.ActiveSAs = tot
				}
			}

			if stats.Uptime == "Offline" {
				stats.Uptime = "Active"
			}
		}
	}

	// 3. Read memory usage from Linux /proc/<pid>/status
	if stats.PID > 0 {
		statusFile := fmt.Sprintf("/proc/%d/status", stats.PID)
		if data, err := readProcStatus(statusFile); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "VmRSS:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						if kb, err := strconv.ParseFloat(fields[1], 64); err == nil {
							stats.Memory = fmt.Sprintf("%.1f MB", kb/1024.0)
						}
					}
					break
				}
			}
		}
	}

	return stats, nil
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// ListManagedConnections discovers every previously loaded SwanPanel peer,
// including stale connections for devices that have since been soft-deleted.
func (c *Client) ListManagedConnections(ctx context.Context) ([]string, error) {
	session, err := c.getSession()
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool)
	for msg, streamErr := range session.CallStreaming(ctx, "list-conns", "list-conn", vici.NewMessage()) {
		if streamErr != nil {
			c.invalidateSession()
			return nil, fmt.Errorf("list strongSwan connections: %w", streamErr)
		}
		for _, name := range msg.Keys() {
			if strings.HasPrefix(name, "swanpanel-") {
				known[name] = true
			}
		}
	}
	names := make([]string, 0, len(known))
	for name := range known {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// UnloadConnection removes an issued device identity from the daemon.
func (c *Client) UnloadConnection(ctx context.Context, name string) error {
	session, err := c.getSession()
	if err != nil {
		return err
	}
	request := vici.NewMessage()
	if err := request.Set("name", name); err != nil {
		return err
	}
	resp, err := session.Call(ctx, "unload-conn", request)
	if err != nil {
		c.invalidateSession()
		return err
	}
	if resp.Get("success") == "yes" {
		return nil
	}
	return fmt.Errorf("unload-conn failed: %v", resp.Get("errmsg"))
}
