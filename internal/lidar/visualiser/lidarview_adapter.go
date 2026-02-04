// Package visualiser provides gRPC streaming of LiDAR perception data.
// This file implements the LidarView adapter that consumes FrameBundle
// and forwards foreground points via UDP (Pandar40P format).
package visualiser

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// ForegroundForwarder is the interface for forwarding foreground points.
// This matches the network.ForegroundForwarder interface but avoids import cycles.
type ForegroundForwarder interface {
	ForwardForeground(points []lidar.PointPolar)
}

// LidarViewAdapter consumes FrameBundle and forwards foreground points
// to the UDP forwarder (for LidarView compatibility).
type LidarViewAdapter struct {
	forwarder ForegroundForwarder
}

// NewLidarViewAdapter creates a new LidarView adapter.
func NewLidarViewAdapter(forwarder ForegroundForwarder) *LidarViewAdapter {
	return &LidarViewAdapter{
		forwarder: forwarder,
	}
}

// PublishFrameBundle extracts foreground points from a FrameBundle
// and forwards them via the UDP forwarder.
// This preserves the existing LidarView UDP forwarding behaviour.
func (a *LidarViewAdapter) PublishFrameBundle(bundleInterface interface{}, foregroundPoints []lidar.PointPolar) {
	if a.forwarder == nil || len(foregroundPoints) == 0 {
		return
	}

	// bundleInterface can be nil if called in LidarView-only mode
	// In that case, we just forward the points directly

	// Forward the polar points directly
	// The foreground extraction has already been done by the tracking pipeline
	a.forwarder.ForwardForeground(foregroundPoints)
}
