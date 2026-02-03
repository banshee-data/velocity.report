// Package visualizer provides gRPC streaming of LiDAR perception data.
// This file provides feature flags and output mode configuration.
package visualizer

// ForwardMode specifies which output(s) to enable.
type ForwardMode int

const (
	// ForwardModeNone disables all forwarding.
	ForwardModeNone ForwardMode = 0

	// ForwardModeLidarView enables only the existing LidarView forwarding (port 2370).
	ForwardModeLidarView ForwardMode = 1

	// ForwardModeGRPC enables only gRPC streaming (port 50051).
	ForwardModeGRPC ForwardMode = 2

	// ForwardModeBoth enables both LidarView and gRPC simultaneously.
	ForwardModeBoth ForwardMode = 3
)

// String returns the string representation of a ForwardMode.
func (m ForwardMode) String() string {
	switch m {
	case ForwardModeNone:
		return "none"
	case ForwardModeLidarView:
		return "lidarview"
	case ForwardModeGRPC:
		return "grpc"
	case ForwardModeBoth:
		return "both"
	default:
		return "unknown"
	}
}

// ParseForwardMode parses a string into a ForwardMode.
func ParseForwardMode(s string) ForwardMode {
	switch s {
	case "none":
		return ForwardModeNone
	case "lidarview":
		return ForwardModeLidarView
	case "grpc":
		return ForwardModeGRPC
	case "both":
		return ForwardModeBoth
	default:
		return ForwardModeLidarView // default to existing behaviour
	}
}

// ForwardConfig holds output configuration flags.
type ForwardConfig struct {
	// Mode determines which outputs are enabled.
	Mode ForwardMode

	// LidarViewAddr is the address for LidarView forwarding.
	// Default: "127.0.0.1:2370"
	LidarViewAddr string

	// GRPCAddr is the address for gRPC streaming.
	// Default: "localhost:50051"
	GRPCAddr string

	// EnableDebugOverlays enables debug artifact emission.
	EnableDebugOverlays bool

	// RecordingEnabled enables frame recording to disk.
	RecordingEnabled bool

	// RecordingPath is the output path for recordings.
	// If empty, a timestamped path is generated.
	RecordingPath string
}

// DefaultForwardConfig returns the default forward configuration.
// Default: LidarView only (preserves existing behaviour).
func DefaultForwardConfig() ForwardConfig {
	return ForwardConfig{
		Mode:                ForwardModeLidarView,
		LidarViewAddr:       "127.0.0.1:2370",
		GRPCAddr:            "localhost:50051",
		EnableDebugOverlays: false,
		RecordingEnabled:    false,
	}
}

// LidarViewEnabled returns true if LidarView forwarding is enabled.
func (c ForwardConfig) LidarViewEnabled() bool {
	return c.Mode == ForwardModeLidarView || c.Mode == ForwardModeBoth
}

// GRPCEnabled returns true if gRPC streaming is enabled.
func (c ForwardConfig) GRPCEnabled() bool {
	return c.Mode == ForwardModeGRPC || c.Mode == ForwardModeBoth
}
