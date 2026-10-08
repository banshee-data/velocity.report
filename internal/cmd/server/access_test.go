package server

import (
	"context"
	"net"
	"testing"

	"github.com/banshee-data/velocity.report/internal/api"
)

func TestHardenedListenerContainment(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8081", "[::1]:50051", "localhost:8082"} {
		if err := validateHardenedListeners(address, address, address); err != nil {
			t.Fatal(err)
		}
	}
	for _, address := range []string{":50051", "0.0.0.0:8081", "[::]:8082", "192.168.1.2:50051", "example.com:8081", "127.0.0.1:invalid", "127.0.0.1:65536", ""} {
		for position := 0; position < 3; position++ {
			listeners := []string{"127.0.0.1:8082", "127.0.0.1:8081", "localhost:50051"}
			listeners[position] = address
			if err := validateHardenedListeners(listeners[0], listeners[1], listeners[2]); err == nil {
				t.Fatalf("accepted %q in position %d", address, position)
			}
		}
	}
}

type noForwards struct{}

func (noForwards) UnmanagedServePorts(context.Context) (map[uint16]bool, error) {
	return map[uint16]bool{}, nil
}

func TestListenerGuardAppliesWhenEnforcementIsOn(t *testing.T) {
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, mode := range []api.CapEnforcement{api.EnforcementOn, api.EnforcementHardened} {
		wrap := listenerGuard(mode, noForwards{})("test")
		if wrap == nil || wrap(raw) == raw {
			t.Fatalf("mode %v: the listener was not guarded", mode)
		}
	}
	if wrap := listenerGuard(api.EnforcementOff, noForwards{})("test"); wrap != nil {
		t.Fatal("off mode trusts every caller and must not read tailscaled")
	}
	if wrap := listenerGuard(api.EnforcementHardened, nil)("test"); wrap != nil {
		t.Fatal("without a Serve reader there is nothing to guard with")
	}
}
