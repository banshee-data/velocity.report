package server

import (
	"context"
	"net"
	"net/http"
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

func TestServeForwardGuardsApplyWhenEnforcementIsOn(t *testing.T) {
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	next := http.NotFoundHandler()
	for _, mode := range []api.CapEnforcement{api.EnforcementOn, api.EnforcementHardened} {
		g := serveForwardGuards{mode: mode, forwards: noForwards{}}
		if wrap := g.listener("test"); wrap == nil || wrap(raw) == raw {
			t.Fatalf("mode %v: the listener was not guarded", mode)
		}
		if wrap := g.handler("test"); wrap == nil || wrap(next) == nil {
			t.Fatalf("mode %v: the handler was not guarded", mode)
		}
	}
	for name, g := range map[string]serveForwardGuards{
		"off trusts every caller":       {mode: api.EnforcementOff, forwards: noForwards{}},
		"no Serve reader to guard with": {mode: api.EnforcementHardened},
	} {
		if g.listener("test") != nil || g.handler("test") != nil {
			t.Fatalf("%s: guarded anyway", name)
		}
	}
}

// The alternate LiDAR listener has no gate: with grants enforced it must be
// local, so tailnet peers cannot reach its routes ungated.
func TestEnforcedListenersKeepTheLiDARListenerLocal(t *testing.T) {
	const serve, grpc = "127.0.0.1:8082", "localhost:50051"
	for _, tc := range []struct {
		mode  api.CapEnforcement
		lidar string
		ok    bool
	}{
		{api.EnforcementOff, "0.0.0.0:8081", true},
		{api.EnforcementOn, "127.0.0.1:8081", true},
		{api.EnforcementOn, "0.0.0.0:8081", false},
		{api.EnforcementOn, "[::]:8081", false},
		{api.EnforcementHardened, "127.0.0.1:8081", true},
		{api.EnforcementHardened, "0.0.0.0:8081", false},
	} {
		if err := validateEnforcedListeners(tc.mode, serve, tc.lidar, grpc); (err == nil) != tc.ok {
			t.Errorf("mode %v, LiDAR %s: %v", tc.mode, tc.lidar, err)
		}
	}
	if err := validateEnforcedListeners(api.EnforcementHardened, "0.0.0.0:8082", "127.0.0.1:8081", grpc); err == nil {
		t.Error("hardened accepted a remote Serve backend")
	}
}
