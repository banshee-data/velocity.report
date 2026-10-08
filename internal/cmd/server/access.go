package server

import (
	"fmt"
	"net"
	"net/http"

	"github.com/banshee-data/velocity.report/internal/access"
	"github.com/banshee-data/velocity.report/internal/api"
)

// validateEnforcedListeners runs before sensors, workers or network servers
// start.  The alternate LiDAR HTTP listener has no gate of its own, so with
// grants enforced on the main listener it must not offer the same routes to
// tailnet peers ungated: on requires it local, and hardened requires every
// listener but the main one local.  off trusts every caller.
func validateEnforcedListeners(mode api.CapEnforcement, serve, lidarHTTP, grpc string) error {
	switch mode {
	case api.EnforcementHardened:
		return validateHardenedListeners(serve, lidarHTTP, grpc)
	case api.EnforcementOn:
		if err := access.ValidateLoopbackListen(lidarHTTP); err != nil {
			return fmt.Errorf("LiDAR HTTP: %w", err)
		}
	}
	return nil
}

// validateHardenedListeners runs before sensors, workers or network servers start.
// The standalone LiDAR HTTP/gRPC services currently trust local processes; they
// must not become remotely accessible while bypassing the main operation policy.
func validateHardenedListeners(serve, lidarHTTP, grpc string) error {
	for _, listener := range []struct{ name, address string }{
		{"Serve backend", serve}, {"LiDAR HTTP", lidarHTTP}, {"gRPC", grpc},
	} {
		if err := access.ValidateLoopbackListen(listener.address); err != nil {
			return fmt.Errorf("%s: %w", listener.name, err)
		}
	}
	return nil
}

// serveForwardGuards wraps each named listener and HTTP handler so that it
// refuses local connections and requests while tailscaled forwards to it
// through a Serve handler the manager did not install (access.GuardListener,
// access.GuardHandler).  With enforcement off both are nil and leave the
// server as it is: that profile trusts every caller.
type serveForwardGuards struct {
	mode     api.CapEnforcement
	forwards access.ServeForwards
}

func (g serveForwardGuards) enabled() bool {
	return g.mode != api.EnforcementOff && g.forwards != nil
}

func (g serveForwardGuards) listener(name string) func(net.Listener) net.Listener {
	if !g.enabled() {
		return nil
	}
	return func(l net.Listener) net.Listener { return access.GuardListener(l, g.forwards, name) }
}

func (g serveForwardGuards) handler(name string) func(http.Handler) http.Handler {
	if !g.enabled() {
		return nil
	}
	return func(h http.Handler) http.Handler { return access.GuardHandler(h, g.forwards, name) }
}
