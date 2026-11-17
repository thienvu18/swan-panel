package policy

import (
	"context"
	"testing"
)

func TestParseLoopbackAddressesProtectsUnmanagedHostIPs(t *testing.T) {
	listing := "1: lo inet 127.0.0.1/8 scope host lo\\ valid_lft forever preferred_lft forever\n" +
		"1: lo inet 10.122.0.1/32 scope global lo:swanpanel\\ valid_lft forever preferred_lft forever\n" +
		"1: lo inet 172.28.0.1/32 scope global lo:swanpanel valid_lft forever preferred_lft forever\n" +
		"1: lo inet 10.250.0.1/32 scope global lo valid_lft forever preferred_lft forever\n" +
		"broken entry\n"
	all, managed := parseLoopbackAddresses(listing)
	for _, ip := range []string{"127.0.0.1", "10.122.0.1", "172.28.0.1", "10.250.0.1"} {
		if !all[ip] {
			t.Fatalf("missing existing loopback address: %s", ip)
		}
	}
	if !managed["10.122.0.1"] || !managed["172.28.0.1"] {
		t.Fatal("SwanPanel-owned aliases were not detected")
	}
	if managed["127.0.0.1"] || managed["10.250.0.1"] {
		t.Fatal("operator-managed loopback address must never be cleaned up")
	}
}

func TestGatewayLoopbackDoesNotMutateKernelInUnitTests(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	if err := syncGatewayLoopbackIPs(context.Background(), map[string]bool{
		"10.122.0.1": true,
	}, true); err != nil {
		t.Fatalf("unit tests must not mutate host networking: %v", err)
	}
}
