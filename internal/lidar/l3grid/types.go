package l3grid

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Type aliases to avoid import cycles.

// PointPolar is re-exported from l4perception for use in L3 grid operations.
type PointPolar = l4perception.PointPolar

// PointASC is re-exported from l2frames for point cloud export operations.
type PointASC = l2frames.PointASC

// FrameID is a human-readable coordinate frame identifier.
type FrameID string

// BgSnapshot matches schema lidar_bg_snapshot table structure.
// It holds a compressed snapshot of the background grid state for persistence.
type BgSnapshot struct {
	SnapshotID         *int64 // will be set by database after insert
	SensorID           string // matches sensor_id TEXT NOT NULL
	TakenUnixNanos     int64  // matches taken_unix_nanos INTEGER NOT NULL
	Rings              int    // matches rings INTEGER NOT NULL
	AzimuthBins        int    // matches azimuth_bins INTEGER NOT NULL
	ParamsJSON         string // matches params_json TEXT NOT NULL
	RingElevationsJSON string // matches ring_elevations_json TEXT NULL - optional per-ring elevation JSON
	GridBlob           []byte // matches grid_blob BLOB NOT NULL (compressed BackgroundCell data)
	ChangedCellsCount  int    // matches changed_cells_count INTEGER
	SnapshotReason     string // matches snapshot_reason TEXT ('settling_complete', 'periodic_update', 'manual')
}

// RegionSnapshot matches schema lidar_bg_regions table structure for persisting
// region identification data. Used to skip settling time when scene hash matches.
type RegionSnapshot struct {
	RegionSetID      *int64 // will be set by database after insert
	SnapshotID       int64  // references lidar_bg_snapshot(snapshot_id)
	SensorID         string // matches sensor_id TEXT NOT NULL
	CreatedUnixNanos int64  // matches created_unix_nanos INTEGER NOT NULL
	RegionCount      int    // matches region_count INTEGER NOT NULL
	RegionsJSON      string // matches regions_json TEXT NOT NULL - serialised RegionData slice
	VarianceDataJSON string // matches variance_data_json TEXT - optional settling metrics
	SettlingFrames   int    // matches settling_frames INTEGER
	SceneHash        string // matches scene_hash TEXT - for scene similarity detection
	SourcePath       string // matches source_path TEXT - PCAP filename for exact match restoration
}

// RegionData is the serialisable form of a Region for JSON persistence.
// CellMask is omitted as it can be reconstructed from CellList.
type RegionData struct {
	ID           int          `json:"id"`
	Params       RegionParams `json:"params"`
	CellList     []int        `json:"cell_list"`
	MeanVariance float64      `json:"mean_variance"`
	CellCount    int          `json:"cell_count"`
}
