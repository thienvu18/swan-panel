package dns_test

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/swan-panel/swan-panel/internal/db"
	"github.com/swan-panel/swan-panel/internal/dns"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	if err := testDB.AutoMigrate(&db.DNSSettings{}, &db.DNSOverride{}, &db.Device{}, &db.ResourceGroup{}); err != nil {
		t.Fatalf("Auto-migrate failed: %v", err)
	}

	// Seed DNS Settings
	dnsSettings := db.DNSSettings{
		ID:                1,
		MagicDNSSuffix:    ".strongnet",
		UpstreamResolvers: "1.1.1.1, 8.8.8.8",
		IsMagicDNSEnabled: true,
	}
	testDB.Create(&dnsSettings)

	// Seed Resource Group & Device
	rg := db.ResourceGroup{
		ID:        "rg_eng",
		Name:      "Engineering Subnet",
		CIDRBlock: "10.0.1.0/24",
	}
	testDB.Create(&rg)

	dev := db.Device{
		ID:               "dev_macbook",
		Hostname:         "thienvu-macbook",
		DeviceType:       "user",
		ResourceGroupID:  "rg_eng",
		IPAllocationType: "dynamic",
		VirtualIP:        "10.0.1.15",
		Platform:         "macos",
		IsRevoked:        false,
	}
	testDB.Create(&dev)

	// Seed DNS Override
	override := db.DNSOverride{
		ID:            "dns_gitlab",
		Hostname:      "gitlab.internal",
		RecordType:    "A",
		TargetAddress: "192.168.1.100",
	}
	testDB.Create(&override)

	return testDB
}

func TestDNSEngine(t *testing.T) {
	testDB := setupTestDB(t)
	server := dns.NewServer(":5354", testDB)
	ctx := context.Background()

	// 1. Test MagicDNS resolution
	ip, err := server.ResolveLocalQuery(ctx, "thienvu-macbook.strongnet")
	if err != nil {
		t.Fatalf("Expected MagicDNS to resolve thienvu-macbook.strongnet, got error: %v", err)
	}
	if ip != "10.0.1.15" {
		t.Errorf("Expected 10.0.1.15, got %s", ip)
	}

	// 2. Test DNS Override resolution
	overrideIP, err := server.ResolveLocalQuery(ctx, "gitlab.internal")
	if err != nil {
		t.Fatalf("Expected DNS Override to resolve gitlab.internal, got error: %v", err)
	}
	if overrideIP != "192.168.1.100" {
		t.Errorf("Expected 192.168.1.100, got %s", overrideIP)
	}

	// 3. Test non-existent domain
	_, err = server.ResolveLocalQuery(ctx, "unknown-server.strongnet")
	if err == nil {
		t.Errorf("Expected error for non-existent domain, got nil")
	}
}
