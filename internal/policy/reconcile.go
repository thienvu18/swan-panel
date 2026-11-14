package policy

import (
	"context"
	"fmt"
	"net/netip"
	"os"

	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/vici"
	"gorm.io/gorm"
)

func ConnectionName(deviceID string) string { return "swanpanel-" + deviceID }
func PoolName(deviceID string) string       { return "swanpanel-vip-" + deviceID }

// Reconcile fails closed at the firewall before installing device-specific IKEv2 connections.
// No wildcard certificate-authenticated connection is created.
func Reconcile(ctx context.Context, database *gorm.DB) error {
	if err := NewFirewall(database).Apply(ctx); err != nil {
		return fmt.Errorf("firewall enforcement is unavailable: %w", err)
	}
	var devices []db.Device
	if err := database.WithContext(ctx).Find(&devices).Error; err != nil {
		return err
	}
	var settings db.StrongswanSettings
	if err := database.WithContext(ctx).First(&settings, 1).Error; err != nil {
		return err
	}
	if os.Getenv("APP_ENV") == "test" {
		compiler := NewCompiler(database)
		for _, d := range devices {
			if !d.IsRevoked {
				if _, err := compiler.CompileForDevice(ctx, d.ID); err != nil {
					return err
				}
			}
		}
		return nil // No data-plane actions in isolated tests; not a production simulation.
	}
	client := vici.GetClient("")
	compiler := NewCompiler(database)
	for _, d := range devices {
		name := ConnectionName(d.ID)
		if d.IsRevoked {
			_ = client.UnloadConnection(ctx, name)
			continue
		}
		ip, err := netip.ParseAddr(d.VirtualIP)
		if err != nil || !ip.Is4() {
			return fmt.Errorf("device %s has invalid IPv4 VIP", d.ID)
		}
		result, err := compiler.CompileForDevice(ctx, d.ID)
		if err != nil {
			return err
		}
		if err := client.LoadPool(ctx, PoolName(d.ID), d.VirtualIP); err != nil {
			return fmt.Errorf("device %s pool: %w", d.ID, err)
		}
		cfg := vici.ConnectionConfig{
			Name: name, ServerIdentity: settings.ServerIdentity, RemoteIdentity: d.Hostname,
			// strongSwan substitutes the assigned VIP into the dynamic remote TS.
			RemoteTS: []string{"dynamic"},
			LocalTS:  result.AllowedSelectors, Pools: []string{PoolName(d.ID)},
			ReauthTime: settings.ReauthTime, RekeyTimeIKE: settings.RekeyTimeIKE,
			RekeyTimeChild: settings.RekeyTimeChild, MobikeEnabled: settings.MobikeEnabled,
			ForceEncapNATT: settings.ForceEncapNATT, DPDDelay: settings.DPDDelay,
			DPDTimeout: settings.DPDTimeout, DPDAction: settings.DPDAction,
			IKEProposals: settings.IKEProposals, ESPProposals: settings.ESPProposals,
		}
		if err := client.LoadConnectionConfig(ctx, cfg); err != nil {
			return fmt.Errorf("device %s configuration: %w", d.ID, err)
		}
	}
	return nil
}
