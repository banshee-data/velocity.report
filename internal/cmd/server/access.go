package server

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/access"
)

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
