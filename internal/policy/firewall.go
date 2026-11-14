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
	sort.Slice(denies, func(i, j int) bool { return denies[i].dev+denies[i].cidr < denies[j].dev+denies[j].cidr })
	sort.Slice(allows, func(i, j int) bool { return allows[i].dev+allows[i].cidr < allows[j].dev+allows[j].cidr })
	for _, r := range denies {
		fmt.Fprintf(&b, "add rule inet swanpanel forward meta ipsec exists ip saddr %s ip daddr %s drop\n", r.dev, r.cidr)
	}
	for _, r := range allows {
		fmt.Fprintf(&b, "add rule inet swanpanel forward meta ipsec exists ip saddr %s ip daddr %s accept\n", r.dev, r.cidr)
	}
	b.WriteString("add rule inet swanpanel forward meta ipsec exists drop\n")
	// DNS is an explicit gateway service exception, scoped only to active device
	// virtual IPs. No device may reach the control-plane HTTP management ports.
	dnsEnabled := strings.ToLower(os.Getenv("ENABLE_DNS")) != "false"
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
	b.WriteString("add rule inet swanpanel input meta ipsec exists drop\n")
	return b.String(), nil
}

func (f *Firewall) Apply(ctx context.Context) error {
	applyMu.Lock()
	defer applyMu.Unlock()
	f.mu.Lock()
	defer f.mu.Unlock()
	script, err := f.Script(ctx)
	if err != nil {
		return err
	}
	if os.Getenv("APP_ENV") == "test" {
		return nil
	} // Explicit opt-in for isolated tests only.
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
	// The following delete and add are one nftables transaction, no intermediate policy gap.
	probe := exec.CommandContext(ctx, nft, "list", "table", "inet", "swanpanel")
	exists := probe.Run() == nil
	if exists {
		script = "delete table inet swanpanel\n" + script
	}
	command := exec.CommandContext(ctx, nft, "-f", "-")
	command.Stdin = strings.NewReader(script)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("nft ruleset rejected: %w: %s", err, stderr.String())
	}
	return nil
}
