package policy

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const swanPanelLoopbackLabel = "lo:swanpanel"

// parseLoopbackAddresses distinguishes addresses owned by SwanPanel from
// existing host-managed addresses, which must never be deleted.
func parseLoopbackAddresses(output string) (all, managed map[string]bool) {
	all = make(map[string]bool)
	managed = make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] != "inet" {
				continue
			}
			prefix, err := netip.ParsePrefix(fields[i+1])
			if err != nil || !prefix.Addr().Is4() {
				break
			}
			ip := prefix.Addr().String()
			all[ip] = true
			for _, field := range fields[i+2:] {
				cleanField := strings.TrimRight(field, "\\")
				if cleanField == swanPanelLoopbackLabel {
					managed[ip] = true
					break
				}
			}
			break
		}
	}
	return all, managed
}

// syncGatewayLoopbackIPs adds gateway DNS VIPs to the container's existing
// loopback. Only explicitly labeled addresses can be removed, and removal is
// deferred while any IKE SA remains active.
func syncGatewayLoopbackIPs(ctx context.Context, desired map[string]bool, removeStale bool) error {
	if env := os.Getenv("APP_ENV"); env == "test" || env == "ci-integration" {
		// Both modes use fake daemon/firewall models without NET_ADMIN.
		// Real Docker integration runs with its normal gateway environment.
		return nil
	}
	output, err := exec.CommandContext(ctx, "ip", "-o", "-4", "addr", "show", "dev", "lo").CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect loopback gateway VIPs: %w: %s", err, strings.TrimSpace(string(output)))
	}
	all, managed := parseLoopbackAddresses(string(output))
	wanted := make([]string, 0, len(desired))
	for ip := range desired {
		parsed, err := netip.ParseAddr(ip)
		if err != nil || !parsed.Is4() {
			return fmt.Errorf("invalid gateway DNS VIP: %s", ip)
		}
		wanted = append(wanted, ip)
	}
	sort.Strings(wanted)
	for _, ip := range wanted {
		if all[ip] {
			if hostStrongSwanMode() && !managed[ip] {
				return fmt.Errorf("gateway DNS VIP %s is already owned by another host service", ip)
			}
			continue // Preserve existing addresses; never claim another service's VIP.
		}
		output, err := exec.CommandContext(ctx, "ip", "addr", "add", ip+"/32", "dev", "lo",
			"label", swanPanelLoopbackLabel).CombinedOutput()
		if err != nil {
			return fmt.Errorf("assign gateway DNS VIP %s: %w: %s", ip, err, strings.TrimSpace(string(output)))
		}
		slog.Info("Assigned managed gateway DNS VIP to loopback", "ip", ip)
	}
	if !removeStale {
		return nil
	}
	stale := make([]string, 0)
	for ip := range managed {
		if !desired[ip] {
			stale = append(stale, ip)
		}
	}
	sort.Strings(stale)
	for _, ip := range stale {
		output, err := exec.CommandContext(ctx, "ip", "addr", "del", ip+"/32", "dev", "lo").CombinedOutput()
		if err != nil {
			return fmt.Errorf("remove stale managed gateway DNS VIP %s: %w: %s", ip, err, strings.TrimSpace(string(output)))
		}
		slog.Info("Removed stale managed gateway DNS VIP", "ip", ip)
	}
	return nil
}

// ManagedGatewayDNSVIPs returns only IPv4 loopback aliases carrying SwanPanel's
// explicit label. Stale aliases remain eligible while existing tunnels use them.
func ManagedGatewayDNSVIPs(ctx context.Context) ([]string, error) {
	output, err := exec.CommandContext(ctx, "ip", "-o", "-4", "addr", "show", "dev", "lo").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("inspect managed gateway DNS VIPs: %w: %s", err, strings.TrimSpace(string(output)))
	}
	_, managed := parseLoopbackAddresses(string(output))
	ips := make([]string, 0, len(managed))
	for ip := range managed {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips, nil
}
