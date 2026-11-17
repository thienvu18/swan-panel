package policy

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"strings"

	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

const (
	dockerForwardChain = "SWANPANEL-FWD"
	// Reserved connection-tracking mark: only nftables Zero-Trust ALLOW sets it.
	dockerForwardMark = "0x40000000/0x40000000"
)

type iptablesCommand func(context.Context, ...string) (string, error)

// The Alpine container and Linux host may select different iptables backends.
// Pick the one which actually owns Docker's chain, not merely the default binary.
func hostIptablesRunner(ctx context.Context) (iptablesCommand, error) {
	var fallback, dropPolicy iptablesCommand
	for _, name := range []string{"iptables", "iptables-nft", "iptables-legacy"} {
		binary, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		run := func(ctx context.Context, args ...string) (string, error) {
			output, err := exec.CommandContext(ctx, binary, append([]string{"-w"}, args...)...).CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
			}
			return string(output), nil
		}
		if _, err := run(ctx, "-S", "DOCKER-USER"); err == nil {
			return run, nil
		}
		forward, err := run(ctx, "-S", "FORWARD")
		if err != nil {
			continue
		}
		if fallback == nil {
			fallback = run
		}
		if strings.Contains(forward, "-P FORWARD DROP") {
			dropPolicy = run
		}
	}
	if dropPolicy != nil {
		return dropPolicy, nil
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("host forwarding requires a usable iptables backend")
}

func dockerNftBackendDetected(ctx context.Context) bool {
	nft, err := exec.LookPath("nft")
	return err == nil && exec.CommandContext(ctx, nft, "list", "table", "ip", "docker-bridges").Run() == nil
}

// The host-mode Docker exception is narrower than a VPN subnet: one /32 per
// currently enrolled, active owner, and only flows stamped by nftables ALLOW.
func managedDockerForwardVIPs(ctx context.Context, database *gorm.DB) ([]string, error) {
	var devices []db.Device
	if err := database.WithContext(ctx).Where("is_revoked = ?", false).Order("virtual_ip asc").Find(&devices).Error; err != nil {
		return nil, err
	}
	var vips []string
	for _, device := range devices {
		ip, err := netip.ParseAddr(device.VirtualIP)
		if err != nil || !ip.Is4() {
			return nil, fmt.Errorf("invalid Docker forwarding VIP for device %s", device.ID)
		}
		if device.DeviceType == "user" {
			var owner db.User
			if device.UserID == nil || database.WithContext(ctx).First(&owner, "id = ?", *device.UserID).Error != nil || owner.Status != "ACTIVE" {
				continue
			}
		}
		vips = append(vips, ip.String()+"/32")
	}
	return vips, nil
}

// Docker's iptables FORWARD DROP runs after nftables forward priority -100.
// Never install a broad source/destination subnet ACCEPT; the connmark is set
// exclusively on IPsec traffic that passed SwanPanel's per-device nft policy.
func syncHostDockerForward(ctx context.Context, vips []string, run iptablesCommand) error {
	if dockerNftBackendDetected(ctx) {
		return fmt.Errorf("Docker nftables backend has no supported DOCKER-USER integration; host VPN forwarding needs a dedicated firewall adapter")
	}
	if _, err := run(ctx, "-S", "DOCKER-USER"); err != nil {
		forward, forwardErr := run(ctx, "-S", "FORWARD")
		if forwardErr != nil {
			return fmt.Errorf("inspect Docker forwarding backend: %w", forwardErr)
		}
		if strings.Contains(forward, "-P FORWARD DROP") {
			return fmt.Errorf("FORWARD policy is DROP but DOCKER-USER is absent; host VPN forwarding cannot be safely installed")
		}
		slog.Debug("No Docker iptables forwarding chain; host forwarding integration not required")
		return nil
	}
	if _, err := run(ctx, "-S", dockerForwardChain); err != nil {
		if _, err := run(ctx, "-N", dockerForwardChain); err != nil {
			return fmt.Errorf("create owned Docker forwarding chain: %w", err)
		}
	}
	// The existing jump, if any, remains in place. Refreshing this owned chain
	// temporarily denies SwanPanel forwarding; it never widens permission.
	if _, err := run(ctx, "-F", dockerForwardChain); err != nil {
		return err
	}
	for _, vip := range vips {
		for _, args := range [][]string{
			{"-A", dockerForwardChain, "-s", vip, "-m", "policy", "--dir", "in", "--pol", "ipsec", "-m", "connmark", "--mark", dockerForwardMark, "-j", "ACCEPT"},
			{"-A", dockerForwardChain, "-d", vip, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-m", "connmark", "--mark", dockerForwardMark, "-j", "ACCEPT"},
		} {
			if _, err := run(ctx, args...); err != nil {
				return fmt.Errorf("install VIP-scoped Docker forwarding rule for %s: %w", vip, err)
			}
		}
	}
	if _, err := run(ctx, "-A", dockerForwardChain, "-j", "RETURN"); err != nil {
		return err
	}
	dockerRules, err := run(ctx, "-S", "DOCKER-USER")
	if err != nil {
		return err
	}
	if !strings.Contains(dockerRules+"\n", "-j "+dockerForwardChain+"\n") {
		if _, err := run(ctx, "-I", "DOCKER-USER", "1", "-j", dockerForwardChain); err != nil {
			return fmt.Errorf("attach owned Docker forwarding chain: %w", err)
		}
	}
	return nil
}

func dockerForwardNeedsSync(ctx context.Context, vips []string, run iptablesCommand) (bool, error) {
	if dockerNftBackendDetected(ctx) {
		return true, nil
	}
	dockerRules, err := run(ctx, "-S", "DOCKER-USER")
	if err != nil {
		forward, forwardErr := run(ctx, "-S", "FORWARD")
		if forwardErr != nil {
			return true, forwardErr
		}
		return strings.Contains(forward, "-P FORWARD DROP"), nil
	}
	if !strings.Contains(dockerRules+"\n", "-j "+dockerForwardChain+"\n") {
		return true, nil
	}
	rules, err := run(ctx, "-S", dockerForwardChain)
	if err != nil {
		return true, nil
	}
	lines := strings.Split(strings.TrimSpace(rules), "\n")
	if len(lines) != 2*len(vips)+2 || lines[0] != "-N "+dockerForwardChain ||
		lines[len(lines)-1] != "-A "+dockerForwardChain+" -j RETURN" {
		return true, nil
	}
	seen := make(map[string]int, len(vips)*2)
	for _, line := range lines[1 : len(lines)-1] {
		matched := false
		for _, vip := range vips {
			source := "-A " + dockerForwardChain + " -s " + vip + " "
			reverse := "-A " + dockerForwardChain + " -d " + vip + " "
			if strings.HasPrefix(line, source) &&
				strings.Contains(line, "-m policy --dir in --pol ipsec") &&
				strings.Contains(line, "-m connmark --mark "+dockerForwardMark) &&
				strings.HasSuffix(line, " -j ACCEPT") {
				seen["src:"+vip]++
				matched = true
				break
			}
			if strings.HasPrefix(line, reverse) &&
				strings.Contains(line, "-m conntrack --ctstate ") &&
				strings.Contains(line, "ESTABLISHED") && strings.Contains(line, "RELATED") &&
				strings.Contains(line, "-m connmark --mark "+dockerForwardMark) &&
				strings.HasSuffix(line, " -j ACCEPT") {
				seen["dst:"+vip]++
				matched = true
				break
			}
		}
		if !matched {
			return true, nil
		}
	}
	for _, vip := range vips {
		if seen["src:"+vip] != 1 || seen["dst:"+vip] != 1 {
			return true, nil
		}
	}
	return false, nil
}

func (f *Firewall) syncDockerForward(ctx context.Context) error {
	if !hostStrongSwanMode() || os.Getenv("APP_ENV") == "test" {
		return nil
	}
	vips, err := managedDockerForwardVIPs(ctx, f.database)
	if err != nil {
		return err
	}
	run, err := hostIptablesRunner(ctx)
	if err != nil {
		return err
	}
	return syncHostDockerForward(ctx, vips, run)
}
