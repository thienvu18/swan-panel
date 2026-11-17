package policy

import (
	"bytes"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/swan-panel/swan-panel/internal/vici"
)

// ManagedConnection is the single desired-state representation for VICI and disk.
type ManagedConnection struct {
	Connection vici.ConnectionConfig
	VirtualIP  string
	DNS        string
}

const defaultSwanctlFile = "/etc/swanctl/conf.d/swanpanel-gateway.conf"

func SwanctlFilePath() string {
	if path := os.Getenv("SWANPANEL_SWANCTL_FILE"); path != "" {
		return path
	}
	return defaultSwanctlFile
}

// Restrict untrusted settings to single-line strongSwan tokens. Never interpolate
// braces, comments or quotes into an administrator-managed configuration file.
var swanctlToken = regexp.MustCompile(`^[A-Za-z0-9_./:,+*%?@=-]+$`)
var swanctlKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func configToken(value string) (string, error) {
	if !swanctlToken.MatchString(value) {
		return "", fmt.Errorf("unsafe or empty strongSwan configuration value")
	}
	return value, nil
}

func writeConfigField(b *strings.Builder, indent, key, value string) error {
	if !swanctlKey.MatchString(key) {
		return fmt.Errorf("invalid strongSwan option name")
	}
	safe, err := configToken(value)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", key, err)
	}
	fmt.Fprintf(b, "%s%s = %s\n", indent, key, safe)
	return nil
}

func configAdvanced(b *strings.Builder, indent string, options map[string]any) error {
	keys := make([]string, 0, len(options))
	for key := range options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var value string
		switch v := options[key].(type) {
		case string:
			value = v
		case bool:
			if v {
				value = "yes"
			} else {
				value = "no"
			}
		case []any:
			items := make([]string, 0, len(v))
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return fmt.Errorf("invalid list item for %s", key)
				}
				items = append(items, s)
			}
			value = strings.Join(items, ",")
		default:
			return fmt.Errorf("unsupported advanced option %s", key)
		}
		if err := writeConfigField(b, indent, key, value); err != nil {
			return err
		}
	}
	return nil
}

// RenderSwanctlConfig mirrors the managed VICI connection/pool model, including
// its deny-by-default traffic selector when no access grants exist.
func RenderSwanctlConfig(plan []ManagedConnection) ([]byte, error) {
	var b strings.Builder
	b.WriteString("# SwanPanel-managed gateway connections. Do not edit manually.\nconnections {\n")
	for _, item := range plan {
		c := item.Connection
		ip, err := netip.ParseAddr(item.VirtualIP)
		if err != nil || !ip.Is4() {
			return nil, fmt.Errorf("invalid device virtual IP")
		}
		if _, err := configToken(c.Name); err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "  %s {\n", c.Name)
		mobike, encap := "no", "no"
		if c.MobikeEnabled {
			mobike = "yes"
		}
		if c.ForceEncapNATT {
			encap = "yes"
		}
		for _, f := range [][2]string{{"version", "2"}, {"send_cert", "always"}, {"mobike", mobike}, {"encap", encap}, {"reauth_time", c.ReauthTime}, {"rekey_time", c.RekeyTimeIKE}, {"dpd_delay", c.DPDDelay}, {"fragmentation", c.Fragmentation}, {"proposals", strings.ReplaceAll(c.IKEProposals, " ", "")}, {"pools", strings.Join(c.Pools, ",")}} {
			if f[1] != "" {
				if err := writeConfigField(&b, "    ", f[0], f[1]); err != nil {
					return nil, err
				}
			}
		}
		if _, overridden := c.AdvancedConnection["send_cert"]; overridden {
			return nil, fmt.Errorf("send_cert is reserved for native Apple client compatibility")
		}
		if err := configAdvanced(&b, "    ", c.AdvancedConnection); err != nil {
			return nil, err
		}
		b.WriteString("    local {\n      auth = pubkey\n      certs = server.crt\n")
		if err := writeConfigField(&b, "      ", "id", c.ServerIdentity); err != nil {
			return nil, err
		}
		b.WriteString("    }\n    remote {\n      auth = pubkey\n      revocation = strict\n")
		if err := writeConfigField(&b, "      ", "id", c.RemoteIdentity); err != nil {
			return nil, err
		}
		b.WriteString("    }\n    children {\n      net {\n        mode = tunnel\n")
		localTS := c.LocalTS
		if len(localTS) == 0 {
			localTS = []string{"0.0.0.0/32"}
		}
		for _, f := range [][2]string{{"remote_ts", strings.Join(c.RemoteTS, ",")}, {"local_ts", strings.Join(localTS, ",")}, {"esp_proposals", strings.ReplaceAll(c.ESPProposals, " ", "")}, {"inactivity", c.InactivityTimeout}, {"rekey_time", c.RekeyTimeChild}, {"dpd_action", c.DPDAction}} {
			if f[1] != "" {
				if err := writeConfigField(&b, "        ", f[0], f[1]); err != nil {
					return nil, err
				}
			}
		}
		if err := configAdvanced(&b, "        ", c.AdvancedChild); err != nil {
			return nil, err
		}
		b.WriteString("      }\n    }\n  }\n")
	}
	b.WriteString("}\npools {\n")
	for _, item := range plan {
		c := item.Connection
		if len(c.Pools) != 1 {
			return nil, fmt.Errorf("expected one pool per device")
		}
		if _, err := configToken(c.Pools[0]); err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "  %s {\n", c.Pools[0])
		if err := writeConfigField(&b, "    ", "addrs", item.VirtualIP+"/32"); err != nil {
			return nil, err
		}
		if item.DNS != "" {
			if err := writeConfigField(&b, "    ", "dns", item.DNS); err != nil {
				return nil, err
			}
		}
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

// PersistSwanctlConfig atomically installs only SwanPanel's dedicated fragment.
func PersistSwanctlConfig(path string, content []byte) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".swanpanel-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	if err := temp.Chmod(0640); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return err
	}
	folder, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = folder.Close() }()
	return folder.Sync()
}

func SwanctlConfigMatches(path string, content []byte) bool {
	existing, err := os.ReadFile(path)
	return err == nil && bytes.Equal(existing, content)
}
