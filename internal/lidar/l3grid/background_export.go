package l3grid

import (
	"math"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// CoarseBucket represents aggregated metrics for a spatial bucket
type CoarseBucket struct {
	Ring            int     `json:"ring"`
	AzimuthDegStart float64 `json:"azimuth_deg_start"`
	AzimuthDegEnd   float64 `json:"azimuth_deg_end"`
	TotalCells      int     `json:"total_cells"`
	FilledCells     int     `json:"filled_cells"`
	SettledCells    int     `json:"settled_cells"`
	FrozenCells     int     `json:"frozen_cells"`
	MeanTimesSeen   float64 `json:"mean_times_seen"`
	MeanRangeMeters float64 `json:"mean_range_meters"`
	MinRangeMeters  float64 `json:"min_range_meters"`
	MaxRangeMeters  float64 `json:"max_range_meters"`
}

// GridHeatmap represents the full aggregated grid state
type GridHeatmap struct {
	SensorID      string                 `json:"sensor_id"`
	Timestamp     time.Time              `json:"timestamp"`
	GridParams    map[string]interface{} `json:"grid_params"`
	HeatmapParams map[string]interface{} `json:"heatmap_params"`
	Summary       map[string]interface{} `json:"summary"`
	Buckets       []CoarseBucket         `json:"buckets"`
}

// GetGridHeatmap aggregates the fine-grained grid into coarse spatial buckets
// for visualization and analysis. Returns nil if the manager or grid is nil.
//
// Parameters:
//   - azimuthBucketDeg: size of each azimuth bucket in degrees (e.g., 3.0 for 120 buckets)
//   - settledThreshold: minimum TimesSeenCount to consider a cell "settled"
func (bm *BackgroundManager) GetGridHeatmap(azimuthBucketDeg float64, settledThreshold uint32) *GridHeatmap {
	if bm == nil || bm.Grid == nil {
		return nil
	}

	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Calculate bucket dimensions
	azBinResDeg := 360.0 / float64(g.AzimuthBins)
	numAzBuckets := int(360.0 / azimuthBucketDeg)
	cellsPerAzBucket := int(azimuthBucketDeg / azBinResDeg)

	// Initialise buckets
	buckets := make([]CoarseBucket, 0, g.Rings*numAzBuckets)
	nowNanos := time.Now().UnixNano()

	totalFilled := 0
	totalSettled := 0
	totalFrozen := 0

	// Aggregate by ring and azimuth bucket
	for ring := 0; ring < g.Rings; ring++ {
		for azBucket := 0; azBucket < numAzBuckets; azBucket++ {
			bucket := CoarseBucket{
				Ring:            ring,
				AzimuthDegStart: float64(azBucket) * azimuthBucketDeg,
				AzimuthDegEnd:   float64(azBucket+1) * azimuthBucketDeg,
				TotalCells:      cellsPerAzBucket,
				MinRangeMeters:  math.MaxFloat64,
			}

			// Aggregate stats from fine cells in this bucket
			startAzBin := azBucket * cellsPerAzBucket
			endAzBin := startAzBin + cellsPerAzBucket

			var sumTimesSeen uint32
			var sumRange float64
			filledCount := 0

			for azBin := startAzBin; azBin < endAzBin && azBin < g.AzimuthBins; azBin++ {
				idx := g.Idx(ring, azBin)
				cell := g.Cells[idx]

				if cell.TimesSeenCount > 0 {
					bucket.FilledCells++
					filledCount++
					sumTimesSeen += cell.TimesSeenCount
					sumRange += float64(cell.AverageRangeMeters)

					if cell.TimesSeenCount >= settledThreshold {
						bucket.SettledCells++
					}

					if cell.AverageRangeMeters < float32(bucket.MinRangeMeters) {
						bucket.MinRangeMeters = float64(cell.AverageRangeMeters)
					}
					if cell.AverageRangeMeters > float32(bucket.MaxRangeMeters) {
						bucket.MaxRangeMeters = float64(cell.AverageRangeMeters)
					}
				}

				if cell.FrozenUntilUnixNanos > nowNanos {
					bucket.FrozenCells++
				}
			}

			// Calculate means
			if filledCount > 0 {
				bucket.MeanTimesSeen = float64(sumTimesSeen) / float64(filledCount)
				bucket.MeanRangeMeters = sumRange / float64(filledCount)
			}
			if bucket.FilledCells == 0 {
				bucket.MinRangeMeters = 0
				bucket.MaxRangeMeters = 0
			}

			totalFilled += bucket.FilledCells
			totalSettled += bucket.SettledCells
			totalFrozen += bucket.FrozenCells

			buckets = append(buckets, bucket)
		}
	}

	totalCells := g.Rings * g.AzimuthBins

	return &GridHeatmap{
		SensorID:  g.SensorID,
		Timestamp: time.Now(),
		GridParams: map[string]interface{}{
			"total_rings":                g.Rings,
			"total_azimuth_bins":         g.AzimuthBins,
			"azimuth_bin_resolution_deg": 360.0 / float64(g.AzimuthBins),
			"total_cells":                totalCells,
		},
		HeatmapParams: map[string]interface{}{
			"azimuth_bucket_deg": azimuthBucketDeg,
			"azimuth_buckets":    numAzBuckets,
			"ring_buckets":       g.Rings,
			"settled_threshold":  settledThreshold,
			"cells_per_bucket":   cellsPerAzBucket,
		},
		Summary: map[string]interface{}{
			"total_filled":  totalFilled,
			"total_settled": totalSettled,
			"total_frozen":  totalFrozen,
			"fill_rate":     float64(totalFilled) / float64(totalCells),
			"settle_rate":   float64(totalSettled) / float64(totalCells),
		},
		Buckets: buckets,
	}
}

// ToASCPoints converts the background grid to a slice of PointASC for export.
func (bm *BackgroundManager) ToASCPoints() []PointASC {
	if bm == nil || bm.Grid == nil {
		return nil
	}
	g := bm.Grid
	rings := g.Rings
	azBins := g.AzimuthBins
	// Read cells and ring elevations directly while holding RLock. This avoids
	// an extra copy but keeps readers mutually consistent; concurrent writers
	// (ProcessFramePolar) will block until the RLock is released.
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.Cells) != rings*azBins {
		return nil
	}

	var points []PointASC
	for ring := 0; ring < rings; ring++ {
		for azBin := 0; azBin < azBins; azBin++ {
			idx := g.Idx(ring, azBin)
			cell := g.Cells[idx]
			if cell.AverageRangeMeters == 0 {
				continue
			}
			az := float64(azBin) * 360.0 / float64(azBins)
			r := float64(cell.AverageRangeMeters)

			// If ring elevation angles are available, compute proper 3D coords
			var x, y, z float64
			if len(g.RingElevations) == rings {
				elev := g.RingElevations[ring]
				x, y, z = l4perception.SphericalToCartesian(r, az, elev)
			} else {
				// Fallback: project onto XY plane (z=0)
				x = r * math.Cos(az*math.Pi/180)
				y = r * math.Sin(az*math.Pi/180)
				z = 0.0
			}
			points = append(points, PointASC{
				X:         x,
				Y:         y,
				Z:         z,
				Intensity: 0,
				Extra:     []interface{}{r, cell.TimesSeenCount},
			})
		}
	}
	return points
}

// ExportBackgroundGridToASC exports the background grid using the shared ASC export utility.
// Returns the actual path where the file was written.
func (bm *BackgroundManager) ExportBackgroundGridToASC() (string, error) {
	points := bm.ToASCPoints()
	return l2frames.ExportPointsToASC(points, " AverageRangeMeters TimesSeenCount")
}

// ExportedCell represents a background cell for API consumption
type ExportedCell struct {
	Ring        int
	AzimuthDeg  float32
	Range       float32
	Spread      float32
	TimesSeen   uint32
	LastUpdate  int64
	FrozenUntil int64
}

// GetGridCells returns all non-empty cells from the grid.
func (bm *BackgroundManager) GetGridCells() []ExportedCell {
	if bm == nil || bm.Grid == nil {
		return nil
	}

	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()

	cells := make([]ExportedCell, 0, g.nonzeroCellCount)
	azBinResDeg := 360.0 / float32(g.AzimuthBins)

	for i, cell := range g.Cells {
		if cell.TimesSeenCount > 0 || cell.AverageRangeMeters > 0 {
			ring := i / g.AzimuthBins
			azBin := i % g.AzimuthBins
			azimuthDeg := float32(azBin) * azBinResDeg

			cells = append(cells, ExportedCell{
				Ring:        ring,
				AzimuthDeg:  azimuthDeg,
				Range:       cell.AverageRangeMeters,
				Spread:      cell.RangeSpreadMeters,
				TimesSeen:   cell.TimesSeenCount,
				LastUpdate:  cell.LastUpdateUnixNanos,
				FrozenUntil: cell.FrozenUntilUnixNanos,
			})
		}
	}
	return cells
}

// RegionInfo represents a region for API export
type RegionInfo struct {
	ID           int          `json:"id"`
	CellCount    int          `json:"cell_count"`
	MeanVariance float64      `json:"mean_variance"`
	Params       RegionParams `json:"params"`
	Cells        []struct {
		Ring       int     `json:"ring"`
		AzimuthDeg float32 `json:"azimuth_deg"`
	} `json:"cells,omitempty"` // Optional: can be large, include only on request
}

// RegionDebugInfo contains full region visualization data
type RegionDebugInfo struct {
	SensorID               string       `json:"sensor_id"`
	Timestamp              time.Time    `json:"timestamp"`
	IdentificationComplete bool         `json:"identification_complete"`
	IdentificationTime     time.Time    `json:"identification_time,omitempty"`
	FramesSampled          int          `json:"frames_sampled"`
	RegionCount            int          `json:"region_count"`
	Regions                []RegionInfo `json:"regions"`
	// Grid mapping: for each cell, which region it belongs to
	GridMapping []int `json:"grid_mapping"` // maps cell index to region ID
}

// GetRegionDebugInfo returns comprehensive region information for debugging
func (bm *BackgroundManager) GetRegionDebugInfo(includeCells bool) *RegionDebugInfo {
	if bm == nil || bm.Grid == nil || bm.Grid.RegionMgr == nil {
		return nil
	}

	g := bm.Grid
	rm := g.RegionMgr

	g.mu.RLock()
	defer g.mu.RUnlock()

	info := &RegionDebugInfo{
		SensorID:               g.SensorID,
		Timestamp:              time.Now(),
		IdentificationComplete: rm.IdentificationComplete,
		FramesSampled:          rm.SettlingMetrics.FramesSampled,
		RegionCount:            len(rm.Regions),
		Regions:                make([]RegionInfo, 0, len(rm.Regions)),
		GridMapping:            make([]int, len(rm.CellToRegionID)),
	}

	if rm.IdentificationComplete {
		info.IdentificationTime = rm.IdentificationTime
	}

	// Copy grid mapping
	copy(info.GridMapping, rm.CellToRegionID)

	// Export region information
	azBinResDeg := 360.0 / float32(g.AzimuthBins)
	for _, region := range rm.Regions {
		regionInfo := RegionInfo{
			ID:           region.ID,
			CellCount:    region.CellCount,
			MeanVariance: region.MeanVariance,
			Params:       region.Params,
		}

		// Optionally include cell list (can be large)
		if includeCells {
			regionInfo.Cells = make([]struct {
				Ring       int     `json:"ring"`
				AzimuthDeg float32 `json:"azimuth_deg"`
			}, 0, len(region.CellList))

			for _, cellIdx := range region.CellList {
				ring := cellIdx / g.AzimuthBins
				azBin := cellIdx % g.AzimuthBins
				azimuthDeg := float32(azBin) * azBinResDeg

				regionInfo.Cells = append(regionInfo.Cells, struct {
					Ring       int     `json:"ring"`
					AzimuthDeg float32 `json:"azimuth_deg"`
				}{
					Ring:       ring,
					AzimuthDeg: azimuthDeg,
				})
			}
		}

		info.Regions = append(info.Regions, regionInfo)
	}

	return info
}
