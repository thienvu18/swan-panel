package policy

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"strings"

	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/gorm"
)

// hostStrongSwanMode is opt-in. The standard Docker bridge gateway is unchanged.
func hostStrongSwanMode() bool {
	return os.Getenv("SWANPANEL_EXTERNAL_STRONGSWAN") == "1"
}

// hostNATScript installs masquerade rules only for enrolled IPv4 VPN clients.
// Unlike a generic POSTROUTING masquerade, unrelated host traffic is untouched.
func hostNATScript(ctx context.Context, database *gorm.DB, devices []db.Device) (string, error) {
	if !hostStrongSwanMode() {
		return "", nil
	}
	iface, err := hostEgressInterface(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("add table ip swanpanel_nat\n")
	b.WriteString("add chain ip swanpanel_nat postrouting { type nat hook postrouting priority srcnat; policy accept; }\n")
	for _, device := range devices {
		addr, err := netip.ParseAddr(device.VirtualIP)
		if err != nil || !addr.Is4() {
			return "", fmt.Errorf("invalid VPN address for NAT device %s", device.ID)
		}
		if device.DeviceType == "user" {
			var owner db.User
			if device.UserID == nil || database.WithContext(ctx).First(&owner, "id = ?", *device.UserID).Error != nil || owner.Status != "ACTIVE" {
				continue
			}
		}
		fmt.Fprintf(&b, "add rule ip swanpanel_nat postrouting ip saddr %s oifname %q counter masquerade\n", addr, iface)
	}
	return b.String(), nil
}

// hostEgressInterface requires an unambiguous host default route unless the
// administrator explicitly pins the outgoing interface.
func hostEgressInterface(ctx context.Context) (string, error) {
	if iface := os.Getenv("SWANPANEL_HOST_EGRESS_INTERFACE"); iface != "" {
		return validateHostInterface(iface)
	}
	ip, err := exec.LookPath("ip")
	if err != nil {
		return "", fmt.Errorf("host NAT needs iproute2 or SWANPANEL_HOST_EGRESS_INTERFACE: %w", err)
	}
	routes, err := exec.CommandContext(ctx, ip, "-4", "route", "show", "default").Output()
	if err != nil {
		return "", fmt.Errorf("read host default route for VPN NAT: %w", err)
	}
	return parseDefaultRouteEgress(string(routes))
}

func validateHostInterface(iface string) (string, error) {
	if len(iface) == 0 || len(iface) > 15 {
		return "", fmt.Errorf("invalid NAT egress interface %q", iface)
	}
	for _, ch := range iface {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '-' || ch == '.' || ch == ':') {
			return "", fmt.Errorf("invalid NAT egress interface %q", iface)
		}
	}
	return iface, nil
}

func parseDefaultRouteEgress(routes string) (string, error) {
	var selected string
	for _, line := range strings.Split(routes, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "default" {
			continue
		}
		var iface string
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "dev" {
				iface = fields[i+1]
				break
			}
		}
		var err error
		iface, err = validateHostInterface(iface)
		if err != nil {
			return "", fmt.Errorf("host NAT cannot resolve default route %q: %w", line, err)
		}
		if selected != "" && selected != iface {
			return "", fmt.Errorf("multiple host default-route interfaces; set SWANPANEL_HOST_EGRESS_INTERFACE explicitly")
		}
		selected = iface
	}
	if selected == "" {
		return "", fmt.Errorf("no host IPv4 default route; set SWANPANEL_HOST_EGRESS_INTERFACE")
	}
	return selected, nil
}
