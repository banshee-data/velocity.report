package lidar

import (
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/adapters"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Backward-compatible type aliases — canonical implementation is in adapters/.

// TrackPointCloudExporter handles exporting point clouds for individual tracks.
type TrackPointCloudExporter = adapters.TrackPointCloudExporter

// SensorConfig holds sensor-specific configuration for packet generation.
type SensorConfig = adapters.SensorConfig

// DefaultPandar40PConfig returns default configuration for Pandar40P sensor.
var DefaultPandar40PConfig = adapters.DefaultPandar40PConfig

// TrackPointCloudFrame represents a single frame of point cloud data for a track.
type TrackPointCloudFrame = adapters.TrackPointCloudFrame

// ExportTrackPointCloud extracts point clouds for a specific track.
func ExportTrackPointCloud(track *TrackedObject, observationHistory []*sqlite.TrackObservation) ([]*TrackPointCloudFrame, error) {
	return adapters.ExportTrackPointCloud(track, observationHistory)
}

// EncodePandar40PPacket encodes polar points into a Pandar40P-compatible UDP packet.
var EncodePandar40PPacket = adapters.EncodePandar40PPacket

// WritePCAPFile writes a sequence of packets to a PCAP file.
var WritePCAPFile = adapters.WritePCAPFile

// WriteNetworkStream sends packets to a UDP destination.
var WriteNetworkStream = adapters.WriteNetworkStream

// TrackPointCloudMetadata contains metadata for exported track point clouds.
type TrackPointCloudMetadata = adapters.TrackPointCloudMetadata

// ExtractMetadata generates metadata for an exported track point cloud.
func ExtractMetadata(track *TrackedObject, frames []*TrackPointCloudFrame) *TrackPointCloudMetadata {
	return adapters.ExtractMetadata(track, frames)
}

// Ensure the type alias for TrackObservation is compatible.
func init() {
	_ = func() *TrackObservation { return (*sqlite.TrackObservation)(nil) }
	_ = func() time.Time { return time.Time{} } // keep time import used
}
