package policy_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/policy"
	"github.com/swan-panel/swan-panel/internal/vici"
)

func managedExample() policy.ManagedConnection {
	return policy.ManagedConnection{
		VirtualIP: "10.17.0.8",
		Connection: vici.ConnectionConfig{
			Name: "swanpanel-dev123", ServerIdentity: "vpn.example.com",
			RemoteIdentity: "laptop.example.com", Pools: []string{"swanpanel-vip-dev123"},
			RemoteTS: []string{"dynamic"}, LocalTS: []string{"10.42.0.0/16", "10.43.1.0/24"},
			ReauthTime: "12h", RekeyTimeIKE: "4h", RekeyTimeChild: "1h",
			DPDDelay: "30s", DPDAction: "clear", DPDTimeout: "120s",
			MobikeEnabled: true, Fragmentation: "yes",
			IKEProposals:  "aes256gcm16-prfsha384-ecp384",
			ESPProposals:  "aes256gcm16",
			AdvancedChild: map[string]any{"start_action": "none"},
		},
	}
}

func TestManagedSwanctlConfigIncludesConnectionsAndPools(t *testing.T) {
	content, err := policy.RenderSwanctlConfig([]policy.ManagedConnection{managedExample()})
	if err != nil {
		t.Fatal(err)
	}
	config := string(content)
	for _, required := range []string{
		"connections {", "swanpanel-dev123 {", "pools = swanpanel-vip-dev123",
		"send_cert = always", "certs = server.crt",
		"id = vpn.example.com", "id = laptop.example.com", "revocation = strict",
		"remote_ts = dynamic", "local_ts = 10.42.0.0/16,10.43.1.0/24",
		"dpd_delay = 30s", "mobike = yes", "esp_proposals = aes256gcm16",
		"pools {", "addrs = 10.17.0.8/32", "start_action = none",
	} {
		if !strings.Contains(config, required) {
			t.Errorf("missing %q in config", required)
		}
	}
	if strings.Contains(config, "dpd_timeout") {
		t.Fatal("IKEv2 configuration must not emit unsupported dpd_timeout")
	}
}

func TestManagedSwanctlConfigEmptyAndInvalidOptions(t *testing.T) {
	empty, err := policy.RenderSwanctlConfig(nil)
	if err != nil || !strings.Contains(string(empty), "connections {") {
		t.Fatalf("empty config: %v", err)
	}
	malformed := managedExample()
	malformed.Connection.AdvancedConnection = map[string]any{"evil": "yes }\nsecrets { foo"}
	if _, err := policy.RenderSwanctlConfig([]policy.ManagedConnection{malformed}); err == nil {
		t.Fatal("configuration injection was accepted")
	}
	malformed = managedExample()
	malformed.Connection.AdvancedConnection = map[string]any{"send_cert": "ifasked"}
	if _, err := policy.RenderSwanctlConfig([]policy.ManagedConnection{malformed}); err == nil {
		t.Fatal("advanced options must not disable iOS server certificate delivery")
	}
	malformed = managedExample()
	malformed.Connection.Pools = nil
	if _, err := policy.RenderSwanctlConfig([]policy.ManagedConnection{malformed}); err == nil {
		t.Fatal("missing dedicated pool was accepted")
	}
}

func TestManagedSwanctlConfigPersistenceIsAtomicAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.d", "swanpanel-gateway.conf")
	first := []byte("connections {}\npools {}\n")
	if err := policy.PersistSwanctlConfig(path, first); err != nil {
		t.Fatal(err)
	}
	if !policy.SwanctlConfigMatches(path, first) {
		t.Fatal("content not persisted")
	}
	sentinel := time.Unix(12345, 0)
	if err := os.Chtimes(path, sentinel, sentinel); err != nil {
		t.Fatal(err)
	}
	if err := policy.PersistSwanctlConfig(path, first); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(sentinel) {
		t.Fatal("unchanged config was unnecessarily rewritten")
	}
	second := []byte("connections { example {} }\npools {}\n")
	if err := policy.PersistSwanctlConfig(path, second); err != nil {
		t.Fatal(err)
	}
	if !policy.SwanctlConfigMatches(path, second) || policy.SwanctlConfigMatches(path, first) {
		t.Fatal("atomic update failed to replace previous content")
	}
}

func TestManagedFileDriftAndMissingDaemonTriggersReconciliation(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	database := setupPolicyTestDB(t)
	if err := database.AutoMigrate(&db.RuntimeApplyState{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := policy.Reconcile(ctx, database); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "managed.conf")
	t.Setenv("SWANPANEL_SWANCTL_FILE", path)
	t.Setenv("APP_ENV", "ci-integration")
	needed, err := policy.NeedsReconciliation(ctx, database)
	if err != nil || !needed {
		t.Fatalf("missing persisted config must require repair: %v %v", needed, err)
	}
	if err := os.WriteFile(path, []byte("different config"), 0600); err != nil {
		t.Fatal(err)
	}
	needed, err = policy.NeedsReconciliation(ctx, database)
	if err != nil || !needed {
		t.Fatalf("managed file drift must require repair: %v %v", needed, err)
	}
}

func TestGatewayReconcileRestoresCredentialsAndRevocationList(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	dataDir := os.Getenv("PKI_DATA_DIR")
	database := setupPolicyTestDB(t)
	if err := database.AutoMigrate(&db.RuntimeApplyState{}, &db.CertificateRevocation{}); err != nil {
		t.Fatal(err)
	}
	if err := policy.ReconcileGateway(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"ca.crt", "server.crt", "server.key", "ca.crl"} {
		if _, err := os.Stat(filepath.Join(dataDir, file)); err != nil {
			t.Fatalf("missing gateway identity/CRL %q: %v", file, err)
		}
	}
}

func TestManagedConfigAdvancedOptionsAndInvalidInputs(t *testing.T) {
	managed := managedExample()
	managed.Connection.AdvancedConnection = map[string]any{
		"unique": true,
		"vips":   []any{"10.1.0.1", "10.1.0.2"},
	}
	managed.Connection.AdvancedChild = map[string]any{"policies": false}
	config, err := policy.RenderSwanctlConfig([]policy.ManagedConnection{managed})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{"unique = yes", "policies = no", "vips = 10.1.0.1,10.1.0.2", "send_cert = always"} {
		if !strings.Contains(string(config), entry) {
			t.Errorf("advanced option %q not persisted", entry)
		}
	}
	invalid := []struct {
		name   string
		change func(*policy.ManagedConnection)
	}{
		{"invalid VIP", func(p *policy.ManagedConnection) { p.VirtualIP = "::1" }},
		{"invalid connection name", func(p *policy.ManagedConnection) { p.Connection.Name = "evil}conn" }},
		{"invalid remote identity", func(p *policy.ManagedConnection) { p.Connection.RemoteIdentity = "unsafe }" }},
		{"invalid pool name", func(p *policy.ManagedConnection) { p.Connection.Pools = []string{"bad {pool"} }},
		{"unsupported advanced type", func(p *policy.ManagedConnection) { p.Connection.AdvancedChild = map[string]any{"broken": 42} }},
		{"unsupported advanced list value", func(p *policy.ManagedConnection) { p.Connection.AdvancedChild = map[string]any{"broken": []any{42}} }},
		{"unsupported advanced key", func(p *policy.ManagedConnection) { p.Connection.AdvancedChild = map[string]any{"bad name": "yes"} }},
		{"invalid proposal", func(p *policy.ManagedConnection) { p.Connection.ESPProposals = "valid,evil}" }},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			entry := managedExample()
			tc.change(&entry)
			if _, err := policy.RenderSwanctlConfig([]policy.ManagedConnection{entry}); err == nil {
				t.Fatal("unsafe managed configuration was rendered")
			}
		})
	}
}

func TestManagedConfigRejectsUnwritableDirectory(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "already-file")
	if err := os.WriteFile(parent, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := policy.PersistSwanctlConfig(filepath.Join(parent, "managed.conf"), []byte("safe")); err == nil {
		t.Fatal("persist incorrectly succeeded under non-directory")
	}
}
