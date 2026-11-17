package policy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/vici"
	"gorm.io/gorm"
)

var reconcileMu sync.Mutex

// DesiredDigest covers all inputs that determine the live VPN and firewall policy.
// DesiredDigest fingerprints only fields that affect the live firewall/VICI.
// Display names, timestamps and monitoring updates must not restart VPN policy.
func DesiredDigest(ctx context.Context, database *gorm.DB) (string, error) {
	var devices []db.Device
	var users []db.User
	var groups []db.ResourceGroup
	var rules []db.AccessRule
	var settings db.StrongswanSettings
	if err := database.WithContext(ctx).Order("id asc").Find(&devices).Error; err != nil {
		return "", err
	}
	if err := database.WithContext(ctx).Preload("UserGroups").Order("id asc").Find(&users).Error; err != nil {
		return "", err
	}
	if err := database.WithContext(ctx).Order("id asc").Find(&groups).Error; err != nil {
		return "", err
	}
	if err := database.WithContext(ctx).Order("id asc").Find(&rules).Error; err != nil {
		return "", err
	}
	if err := database.WithContext(ctx).First(&settings, 1).Error; err != nil {
		return "", err
	}
	type devicePolicy struct {
		ID, Hostname, DeviceType, ResourceGroupID, VirtualIP, Owner string
		IsRevoked                                                   bool
	}
	type userPolicy struct {
		ID, Status string
		GroupIDs   []string
	}
	type groupPolicy struct{ ID, CIDR string }
	type rulePolicy struct{ ID, SubjectType, SubjectID, Action, TargetType, TargetValue, RuleNature string }
	type connectionPolicy struct {
		ServerIdentity, ReauthTime, RekeyTimeIKE, RekeyTimeChild, InactivityTimeout string
		Fragmentation, DPDDelay, DPDAction, IKEProposals, ESPProposals              string
		MobikeEnabled, ForceEncapNATT                                               bool
		MSSClamping                                                                 int
		AdvancedConnection, AdvancedChild                                           map[string]any
	}
	snapshot := struct {
		Devices    []devicePolicy
		Users      []userPolicy
		Groups     []groupPolicy
		Rules      []rulePolicy
		Connection connectionPolicy
	}{
		Devices: make([]devicePolicy, 0, len(devices)),
		Users:   make([]userPolicy, 0, len(users)),
		Groups:  make([]groupPolicy, 0, len(groups)),
		Rules:   make([]rulePolicy, 0, len(rules)),
	}
	for _, device := range devices {
		owner := ""
		if device.UserID != nil {
			owner = *device.UserID
		}
		snapshot.Devices = append(snapshot.Devices, devicePolicy{
			device.ID, device.Hostname, device.DeviceType, device.ResourceGroupID,
			device.VirtualIP, owner, device.IsRevoked,
		})
	}
	for _, user := range users {
		ids := make([]string, 0, len(user.UserGroups))
		for _, group := range user.UserGroups {
			ids = append(ids, group.ID)
		}
		sort.Strings(ids)
		snapshot.Users = append(snapshot.Users, userPolicy{user.ID, user.Status, ids})
	}
	for _, group := range groups {
		snapshot.Groups = append(snapshot.Groups, groupPolicy{group.ID, group.CIDRBlock})
	}
	for _, rule := range rules {
		snapshot.Rules = append(snapshot.Rules, rulePolicy{
			rule.ID, rule.SubjectType, rule.SubjectID, rule.Action, rule.TargetType, rule.TargetValue, rule.RuleNature,
		})
	}
	snapshot.Connection = connectionPolicy{
		ServerIdentity: settings.ServerIdentity, ReauthTime: settings.ReauthTime,
		RekeyTimeIKE: settings.RekeyTimeIKE, RekeyTimeChild: settings.RekeyTimeChild,
		InactivityTimeout: settings.InactivityTimeout, Fragmentation: settings.Fragmentation,
		DPDDelay: settings.DPDDelay, DPDAction: settings.DPDAction,
		IKEProposals: settings.IKEProposals, ESPProposals: settings.ESPProposals,
		MobikeEnabled: settings.MobikeEnabled, ForceEncapNATT: settings.ForceEncapNATT,
		MSSClamping:        settings.MSSClamping,
		AdvancedConnection: settings.AdvancedConnection, AdvancedChild: settings.AdvancedChild,
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// NeedsReconciliation detects missed writes even when the last HTTP request succeeded.
func NeedsReconciliation(ctx context.Context, database *gorm.DB) (bool, error) {
	state, err := GetApplyState(ctx, database)
	if err != nil {
		return false, err
	}
	if state.Status != "applied" {
		return true, nil
	}
	digest, err := DesiredDigest(ctx, database)
	if err != nil {
		return false, err
	}
	if digest != state.AppliedDigest {
		return true, nil
	}
	if os.Getenv("APP_ENV") == "test" {
		return false, nil
	}
	content, err := os.ReadFile(SwanctlFilePath())
	if err != nil {
		return true, nil
	}
	hash := sha256.Sum256(content)
	if state.SwanctlDigest == "" || hex.EncodeToString(hash[:]) != state.SwanctlDigest {
		return true, nil
	}
	bootID, err := vici.GetClient("").DaemonBootID(ctx)
	if err != nil {
		return true, nil
	}
	if bootID != state.DaemonBootID {
		return true, nil
	}
	if hostStrongSwanMode() {
		vips, err := managedDockerForwardVIPs(ctx, database)
		if err != nil {
			return false, err
		}
		run, err := hostIptablesRunner(ctx)
		if err != nil {
			return true, nil
		}
		return dockerForwardNeedsSync(ctx, vips, run)
	}
	return false, nil
}

// GetApplyState reports the current persisted data-plane convergence status.
func GetApplyState(ctx context.Context, database *gorm.DB) (db.RuntimeApplyState, error) {
	var state db.RuntimeApplyState
	err := database.WithContext(ctx).Where("id = ?", 1).Attrs(db.RuntimeApplyState{Status: "pending"}).FirstOrCreate(&state).Error
	return state, err
}

func ConnectionName(deviceID string) string { return "swanpanel-" + deviceID }
func PoolName(deviceID string) string       { return "swanpanel-vip-" + deviceID }

// Reconcile converges persistent strongSwan config, runtime VICI and the nftables policy.
// Firewall changes are atomic: active tunnels are not globally blocked while VICI reloads.
func Reconcile(ctx context.Context, database *gorm.DB) (retErr error) {
	reconcileMu.Lock()
	defer reconcileMu.Unlock()
	state, err := GetApplyState(ctx, database)
	if err != nil {
		return fmt.Errorf("load runtime apply state: %w", err)
	}
	digest, err := DesiredDigest(ctx, database)
	if err != nil {
		return fmt.Errorf("read desired configuration: %w", err)
	}
	if digest != state.DesiredDigest || state.DesiredGeneration == 0 {
		state.DesiredGeneration++
		state.DesiredDigest = digest
	}
	state.Status = "applying"
	state.LastError = ""
	if err := database.WithContext(ctx).Save(&state).Error; err != nil {
		return err
	}
	defer func() {
		if retErr == nil {
			current, err := DesiredDigest(context.Background(), database)
			if err != nil {
				retErr = err
			} else if current != state.DesiredDigest {
				retErr = fmt.Errorf("desired state changed while applying; retry required")
			}
		}
		if retErr == nil && os.Getenv("APP_ENV") != "test" {
			bootCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			bootID, err := vici.GetClient("").DaemonBootID(bootCtx)
			cancel()
			if err != nil {
				retErr = fmt.Errorf("verify strongSwan daemon after apply: %w", err)
			} else {
				content, err := os.ReadFile(SwanctlFilePath())
				if err != nil {
					retErr = fmt.Errorf("verify persisted gateway config: %w", err)
				} else {
					sum := sha256.Sum256(content)
					state.SwanctlDigest = hex.EncodeToString(sum[:])
					state.DaemonBootID = bootID
				}
			}
		}
		if retErr != nil {
			state.Status = "failed"
			state.LastError = retErr.Error()
		} else {
			state.Status = "applied"
			state.AppliedGeneration = state.DesiredGeneration
			state.AppliedDigest = state.DesiredDigest
			now := time.Now().UTC()
			state.LastAppliedAt = &now
			state.LastError = ""
		}
		if err := database.WithContext(context.Background()).Save(&state).Error; err != nil {
			retErr = fmt.Errorf("persist apply state: %w (runtime error: %v)", err, retErr)
		}
	}()
	return reconcileRuntime(ctx, database)
}

// reconcileRuntime builds the same connections for VICI and the disk fragment.
// Swap firewall rules atomically without imposing a global VPN blackout.
func reconcileRuntime(ctx context.Context, database *gorm.DB) error {
	var devices []db.Device
	if err := database.WithContext(ctx).Find(&devices).Error; err != nil {
		return err
	}
	var settings db.StrongswanSettings
	if err := database.WithContext(ctx).First(&settings, 1).Error; err != nil {
		return err
	}
	var plan []ManagedConnection
	gatewayVIPs := make(map[string]bool)
	// Do not push an unreachable internal DNS VIP to clients when the embedded
	// DNS server is disabled (for example in the isolated Docker IKEv2 lab).
	// Match config.Load's default-true behavior for missing/invalid values.
	dnsEnabled := true
	if raw := os.Getenv("ENABLE_DNS"); raw != "" {
		if enabled, parseErr := strconv.ParseBool(raw); parseErr == nil {
			dnsEnabled = enabled
		}
	}
	compiler := NewCompiler(database)
	for _, d := range devices {
		if d.IsRevoked {
			continue
		}
		ip, err := netip.ParseAddr(d.VirtualIP)
		if err != nil || !ip.Is4() {
			return fmt.Errorf("device %s has invalid IPv4 VIP", d.ID)
		}
		result, err := compiler.CompileForDevice(ctx, d.ID)
		if err != nil {
			return fmt.Errorf("device %s policy: %w", d.ID, err)
		}
		var rg db.ResourceGroup
		gwIP := ""
		if dnsEnabled && d.ResourceGroupID != "" {
			if err := database.WithContext(ctx).First(&rg, "id = ?", d.ResourceGroupID).Error; err == nil {
				gwIP = GatewayIPForCIDR(rg.CIDRBlock)
			}
		}
		if gwIP != "" {
			gatewayVIPs[gwIP] = true
		}
		config := vici.ConnectionConfig{
			Name: ConnectionName(d.ID), ServerIdentity: settings.ServerIdentity, RemoteIdentity: d.Hostname,
			RemoteTS: []string{"dynamic"}, LocalTS: result.AllowedSelectors, Pools: []string{PoolName(d.ID)},
			ReauthTime: settings.ReauthTime, RekeyTimeIKE: settings.RekeyTimeIKE,
			RekeyTimeChild: settings.RekeyTimeChild, MobikeEnabled: settings.MobikeEnabled,
			ForceEncapNATT: settings.ForceEncapNATT, DPDDelay: settings.DPDDelay,
			DPDAction: settings.DPDAction, IKEProposals: settings.IKEProposals, ESPProposals: settings.ESPProposals,
			Fragmentation: settings.Fragmentation, InactivityTimeout: settings.InactivityTimeout,
			AdvancedConnection: settings.AdvancedConnection, AdvancedChild: settings.AdvancedChild,
		}
		plan = append(plan, ManagedConnection{Connection: config, VirtualIP: ip.String(), DNS: gwIP})
	}
	content, err := RenderSwanctlConfig(plan)
	if err != nil {
		return fmt.Errorf("render persistent strongSwan configuration: %w", err)
	}
	// Validate all connection options before touching the kernel. A rejected
	// advanced option must not leave a newly assigned gateway VIP behind.
	if err := syncGatewayLoopbackIPs(ctx, gatewayVIPs, false); err != nil {
		return err
	}
	firewall := NewFirewall(database)
	if os.Getenv("APP_ENV") == "test" {
		return firewall.Apply(ctx)
	}
	if err := PersistSwanctlConfig(SwanctlFilePath(), content); err != nil {
		return fmt.Errorf("persist strongSwan configuration: %w", err)
	}
	// The nftables table is replaced in one kernel transaction. Removing a
	// grant closes access immediately; other live clients stay connected.
	if err := firewall.Apply(ctx); err != nil {
		// Normal updates do not interrupt active tunnels. If the new ruleset
		// cannot be installed, attempt a fail-closed barrier as a last resort.
		fallbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		fallbackErr := firewall.ApplyDenyAll(fallbackCtx)
		cancel()
		if fallbackErr != nil {
			return fmt.Errorf("activate firewall policy: %w (fail-closed fallback: %v)", err, fallbackErr)
		}
		return fmt.Errorf("activate firewall policy: %w (fail-closed barrier active)", err)
	}
	client := vici.GetClient("")
	// Empty gateways must still verify the daemon is reachable. Capture the
	// daemon identity so a restart during apply cannot be marked successful.
	beforeBoot, err := client.DaemonBootID(ctx)
	if err != nil {
		return fmt.Errorf("VICI daemon unavailable: %w", err)
	}
	for _, item := range plan {
		var dnsList []string
		if item.DNS != "" {
			dnsList = []string{item.DNS}
		}
		if err := client.LoadPoolWithDNS(ctx, item.Connection.Pools[0], item.VirtualIP+"/32", dnsList); err != nil {
			return fmt.Errorf("device %s pool: %w", item.Connection.Name, err)
		}
		if err := client.LoadConnectionConfig(ctx, item.Connection); err != nil {
			return fmt.Errorf("device %s configuration: %w", item.Connection.Name, err)
		}
	}
	// Do not leave orphaned or legacy wildcard SwanPanel connections in charon.
	// Soft-deleted devices are absent from the normal GORM query above.
	loaded, err := client.ListManagedConnections(ctx)
	if err != nil {
		return fmt.Errorf("inspect loaded SwanPanel connections: %w", err)
	}
	desired := make(map[string]bool, len(plan))
	for _, item := range plan {
		desired[item.Connection.Name] = true
	}
	for _, name := range loaded {
		if !desired[name] {
			if err := client.UnloadConnection(ctx, name); err != nil {
				return fmt.Errorf("unload stale connection %s: %w", name, err)
			}
			slog.Info("Removed stale SwanPanel connection", "name", name)
		}
	}
	loadedPools, poolsErr := client.ListManagedPools(ctx)
	if poolsErr != nil {
		// Pool garbage collection is not allowed to fail a valid VPN policy.
		slog.Warn("Deferred stale pool cleanup: unable to list VICI pools", "error", poolsErr)
	} else {
		desiredPools := make(map[string]bool, len(plan))
		for _, item := range plan {
			desiredPools[item.Connection.Pools[0]] = true
		}
		for _, name := range loadedPools {
			if desiredPools[name] {
				continue
			}
			if err := client.UnloadPool(ctx, name); err != nil {
				// Charon retains pools with online leases.
				slog.Warn("Deferred stale SwanPanel pool cleanup until leases close", "pool", name, "error", err)
				continue
			}
			slog.Info("Removed stale SwanPanel pool", "name", name)
		}
	}
	// Remove obsolete loopback aliases only when no IKE SAs remain.
	// This avoids disrupting DNS for an established tunnel during a policy edit.
	if active, saErr := client.ListSAs(ctx); saErr != nil {
		slog.Warn("Deferred gateway loopback cleanup: cannot inspect active tunnels", "error", saErr)
	} else if len(active) == 0 {
		if err := syncGatewayLoopbackIPs(ctx, gatewayVIPs, true); err != nil {
			return err
		}
	}
	afterBoot, err := client.DaemonBootID(ctx)
	if err != nil || afterBoot != beforeBoot {
		return fmt.Errorf("strongSwan restarted while applying; reconcile again: %v", err)
	}
	return nil
}

// GatewayIPForCIDR returns the first host IPv4 address in a CIDR block, e.g. 10.122.0.1 for 10.122.0.0/24
func GatewayIPForCIDR(cidr string) string {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil || !prefix.Addr().Is4() {
		return ""
	}
	addr := prefix.Masked().Addr()
	b := addr.As4()
	b[3] |= 1
	return netip.AddrFrom4(b).String()
}
