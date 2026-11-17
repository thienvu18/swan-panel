package policy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

// Firewall uses a dedicated nftables table and atomically replaces its ruleset.
// It does not touch tables/chains owned by other services.
type Firewall struct {
	database *gorm.DB
	mu       sync.Mutex
}

func NewFirewall(database *gorm.DB) *Firewall { return &Firewall{database: database} }

var applyMu sync.Mutex

func (f *Firewall) Script(ctx context.Context) (string, error) {
	var devices []db.Device
	if err := f.database.WithContext(ctx).Where("is_revoked = ?", false).Order("id asc").Find(&devices).Error; err != nil {
		return "", err
	}
	var hostVIPs []string
	if hostStrongSwanMode() {
		var err error
		hostVIPs, err = f.managedIPv4VIPs(ctx)
		if err != nil {
			return "", err
		}
	}
	compiler := NewCompiler(f.database)
	var b strings.Builder
	// A separate chain has a high priority: no implicit permission for any tunneled traffic.
	b.WriteString("add table inet swanpanel\n")
	b.WriteString("add chain inet swanpanel forward { type filter hook forward priority -100; policy accept; }\n")
	b.WriteString("add chain inet swanpanel input { type filter hook input priority -100; policy accept; }\n")
	type pair struct {
		dev  string
		cidr string
	}
	var denies, allows []pair
	for _, d := range devices {
		result, err := compiler.CompileForDevice(ctx, d.ID)
		if err != nil {
			return "", err
		}
		if _, err := netip.ParseAddr(result.VirtualIP); err != nil {
			return "", err
		}
		for _, cidr := range result.DeniedSelectors {
			denies = append(denies, pair{result.VirtualIP, cidr})
		}
		for _, cidr := range result.AllowedSelectors {
			allows = append(allows, pair{result.VirtualIP, cidr})
		}
	}
	// Clamp tunneled TCP MSS before accepting any forwarding grant.
	var gateway db.StrongswanSettings
	if err := f.database.WithContext(ctx).First(&gateway, 1).Error; err != nil {
		return "", err
	}
	if gateway.MSSClamping > 0 {
		if hostStrongSwanMode() {
			for _, vip := range hostVIPs {
				fmt.Fprintf(&b, "add rule inet swanpanel forward meta ipsec exists ip saddr %s tcp flags syn tcp option maxseg size set %d\n", vip, gateway.MSSClamping)
			}
		} else {
			fmt.Fprintf(&b, "add rule inet swanpanel forward meta ipsec exists tcp flags syn tcp option maxseg size set %d\n", gateway.MSSClamping)
		}
	}
	dnsEnabled := strings.ToLower(os.Getenv("ENABLE_DNS")) != "false"
	// Do not force-reject DNS-over-TLS. Android's strict Private DNS mode
	// fails closed instead of falling back to the VPN-provided resolver.
	// Normal Zero-Trust allow/deny selectors continue to govern this traffic.
	sort.Slice(denies, func(i, j int) bool { return denies[i].dev+denies[i].cidr < denies[j].dev+denies[j].cidr })
	sort.Slice(allows, func(i, j int) bool { return allows[i].dev+allows[i].cidr < allows[j].dev+allows[j].cidr })
	for _, r := range denies {
		fmt.Fprintf(&b, "add rule inet swanpanel forward meta ipsec exists ip saddr %s ip daddr %s drop\n", r.dev, r.cidr)
	}
	for _, r := range allows {
		if hostStrongSwanMode() {
			// A later Docker FORWARD chain may still DROP an nftables ACCEPT.
			// Stamp only explicitly authorized IPsec flows for our DOCKER-USER gate.
			fmt.Fprintf(&b, "add rule inet swanpanel forward meta ipsec exists ip saddr %s ip daddr %s ct mark set ct mark | 0x40000000 accept\n", r.dev, r.cidr)
		} else {
			fmt.Fprintf(&b, "add rule inet swanpanel forward meta ipsec exists ip saddr %s ip daddr %s accept\n", r.dev, r.cidr)
		}
	}
	if hostStrongSwanMode() {
		for _, vip := range hostVIPs {
			fmt.Fprintf(&b, "add rule inet swanpanel forward meta ipsec exists ip saddr %s drop\n", vip)
		}
	} else {
		b.WriteString("add rule inet swanpanel forward meta ipsec exists drop\n")
	}
	// DNS is an explicit gateway service exception, scoped only to active device
	// virtual IPs. No device may reach the control-plane HTTP management ports.
	dnsPort := 53
	if configured := os.Getenv("DNS_PORT"); configured != "" {
		value, err := strconv.Atoi(configured)
		if err != nil || value < 1 || value > 65535 {
			return "", fmt.Errorf("invalid DNS_PORT")
		}
		dnsPort = value
	}
	if dnsEnabled {
		for _, d := range devices {
			if d.DeviceType == "user" {
				var owner db.User
				if d.UserID == nil || f.database.WithContext(ctx).First(&owner, "id = ?", *d.UserID).Error != nil || owner.Status != "ACTIVE" {
					continue
				}
			}
			fmt.Fprintf(&b, "add rule inet swanpanel input meta ipsec exists ip saddr %s udp dport %d accept\n", d.VirtualIP, dnsPort)
			fmt.Fprintf(&b, "add rule inet swanpanel input meta ipsec exists ip saddr %s tcp dport %d accept\n", d.VirtualIP, dnsPort)
		}
	}
	if hostStrongSwanMode() {
		for _, vip := range hostVIPs {
			fmt.Fprintf(&b, "add rule inet swanpanel input meta ipsec exists ip saddr %s drop\n", vip)
		}
	} else {
		b.WriteString("add rule inet swanpanel input meta ipsec exists drop\n")
	}
	// In external-charon mode, this policy executes in the host namespace.
	// Only the enrolled client VIPs are NATed; other host traffic is untouched.
	natScript, err := hostNATScript(ctx, f.database, devices)
	if err != nil {
		return "", err
	}
	b.WriteString(natScript)
	return b.String(), nil
}

// managedIPv4VIPs scopes host-mode enforcement to SwanPanel assignments.
// Include revoked devices to keep their established SAs denied during teardown;
// Resource Group CIDRs are not used because VICI allocates per-device /32 pools.
func (f *Firewall) managedIPv4VIPs(ctx context.Context) ([]string, error) {
	var devices []db.Device
	if err := f.database.WithContext(ctx).Select("virtual_ip").Order("virtual_ip asc").Find(&devices).Error; err != nil {
		return nil, fmt.Errorf("list SwanPanel-managed VIPs: %w", err)
	}
	vips := make([]string, 0, len(devices))
	seen := make(map[string]bool, len(devices))
	for _, device := range devices {
		ip, err := netip.ParseAddr(device.VirtualIP)
		if err != nil || !ip.Is4() {
			return nil, fmt.Errorf("invalid SwanPanel-managed IPv4 VIP %q", device.VirtualIP)
		}
		if !seen[ip.String()] {
			seen[ip.String()] = true
			vips = append(vips, ip.String())
		}
	}
	sort.Strings(vips)
	return vips, nil
}

// Apply publishes the full allow/deny set after VICI is prepared.
func (f *Firewall) Apply(ctx context.Context) error {
	script, err := f.Script(ctx)
	if err != nil {
		return err
	}
	return f.applyScript(ctx, script)
}

// ApplyDenyAll enforces a temporary fail-closed barrier during reconciliation.
// If VICI loading fails the barrier remains in place until the retry succeeds.
func (f *Firewall) ApplyDenyAll(ctx context.Context) error {
	script, err := f.denyAllScript(ctx)
	if err != nil {
		return err
	}
	return f.applyScript(ctx, script)
}

func (f *Firewall) denyAllScript(ctx context.Context) (string, error) {
	var b strings.Builder
	b.WriteString("add table inet swanpanel\n")
	b.WriteString("add chain inet swanpanel forward { type filter hook forward priority -100; policy accept; }\n")
	b.WriteString("add chain inet swanpanel input { type filter hook input priority -100; policy accept; }\n")
	if hostStrongSwanMode() {
		vips, err := f.managedIPv4VIPs(ctx)
		if err != nil {
			return "", err
		}
		for _, vip := range vips {
			fmt.Fprintf(&b, "add rule inet swanpanel forward meta ipsec exists ip saddr %s drop\n", vip)
			fmt.Fprintf(&b, "add rule inet swanpanel input meta ipsec exists ip saddr %s drop\n", vip)
		}
	} else {
		b.WriteString("add rule inet swanpanel forward meta ipsec exists drop\n")
		b.WriteString("add rule inet swanpanel input meta ipsec exists drop\n")
	}
	return b.String(), nil
}

func (f *Firewall) applyScript(ctx context.Context, script string) error {
	applyMu.Lock()
	defer applyMu.Unlock()
	f.mu.Lock()
	defer f.mu.Unlock()
	if os.Getenv("APP_ENV") == "test" {
		return nil
	}
	if os.Getenv("SWANPANEL_FIREWALL_DRY_RUN") == "1" {
		if os.Getenv("APP_ENV") == "production" {
			return errors.New("firewall dry-run prohibited in production")
		}
		return errors.New("firewall dry-run cannot enforce access rules")
	}
	nft, err := exec.LookPath("nft")
	if err != nil {
		return fmt.Errorf("nftables required for Zero-Trust enforcement: %w", err)
	}
	probe := exec.CommandContext(ctx, nft, "list", "table", "inet", "swanpanel")
	if probe.Run() == nil {
		script = "delete table inet swanpanel" + string(rune(10)) + script
	}
	if hostStrongSwanMode() {
		// Replace both managed tables in one nft transaction. The deny-all
		// fallback intentionally removes NAT without reintroducing permits.
		natProbe := exec.CommandContext(ctx, nft, "list", "table", "ip", "swanpanel_nat")
		if natProbe.Run() == nil {
			script = "delete table ip swanpanel_nat" + string(rune(10)) + script
		}
	}
	command := exec.CommandContext(ctx, nft, "-f", "-")
	command.Stdin = strings.NewReader(script)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("nft ruleset rejected: %w: %s", err, stderr.String())
	}
	if err := f.syncDockerForward(ctx); err != nil {
		return fmt.Errorf("synchronize Docker host forwarding: %w", err)
	}
	return nil
}
