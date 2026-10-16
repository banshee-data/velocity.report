package l3grid

import (
	"encoding/json"
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// Snapshot reasons an offline replay records with its region map. The grid
// snapshot taken when regions are identified holds the map the per-region
// overrides read for the rest of the replay; the one taken at the end holds
// the same regions over the grid the replay finished with.
const (
	SnapshotReasonRegionsIdentified = "regions_identified"
	SnapshotReasonReplayEnd         = "replay_end"
)

// RegionMapCell is one cell of a persisted region map, placed in the sensor
// frame. Positions follow l2frames.SphericalToCartesian (x right, y forward,
// z up) at the cell's settled range, its ring's elevation and its azimuth
// bin's centre, so a cell never seen while the grid settled sits at the
// origin; filter on TimesSeen before plotting.
type RegionMapCell struct {
	Cell       int `json:"cell"`
	Ring       int `json:"ring"`
	AzimuthBin int `json:"azimuth_bin"`
	// ElevationDeg is the ring's beam elevation, from the elevation table
	// the grid itself carried when the snapshot was taken.
	ElevationDeg float64 `json:"elevation_deg"`
	// AzimuthDeg is the centre of the azimuth bin. ProcessFramePolar puts
	// azimuth a in bin floor(a/360*bins), so a bin spans one resolution
	// step from bin*360/bins.
	AzimuthDeg float64 `json:"azimuth_deg"`
	// RegionID is -1 for a cell no region holds.
	RegionID int `json:"region_id"`
	// RegionTercile is the tercile the cell's region was identified from,
	// which decides the overrides the cell runs under; RegionVolatile is
	// RegionTercile == "volatile", the flag RegionOverrideScope reads.
	RegionTercile  string `json:"region_tercile"`
	RegionVolatile bool   `json:"region_volatile"`
	// RegionNoiseRelativeFraction and RegionNeighbourConfirmation are the
	// region's override parameters as identified.
	RegionNoiseRelativeFraction float32 `json:"region_noise_relative_fraction"`
	RegionNeighbourConfirmation int     `json:"region_neighbour_confirmation"`
	// CellTercile is the cell's own tercile before merging; "none" for a
	// cell unseen while settling, empty when the snapshot recorded none.
	CellTercile string `json:"cell_tercile"`
	// SettlingVariance is the cell's mean range spread over the settling
	// frames, the figure the terciles cut.
	SettlingVariance float64 `json:"settling_variance"`
	// SettledRangeM is the cell's background range (AverageRangeMeters)
	// when the grid snapshot was taken; LockedBaselineM is its locked
	// baseline, zero until locked.
	SettledRangeM   float32 `json:"settled_range_m"`
	LockedBaselineM float32 `json:"locked_baseline_m"`
	RangeSpreadM    float32 `json:"range_spread_m"`
	TimesSeen       uint32  `json:"times_seen"`
	XM              float64 `json:"x_m"`
	YM              float64 `json:"y_m"`
	ZM              float64 `json:"z_m"`
}

// RegionMap is a persisted region snapshot joined to the grid snapshot it
// links to: one RegionMapCell per grid cell, in cell-index order.
type RegionMap struct {
	Rings          int `json:"rings"`
	AzimuthBins    int `json:"azimuth_bins"`
	RegionCount    int `json:"region_count"`
	SettlingFrames int `json:"settling_frames"`
	// TercileThresholds are the two settling-variance cut points, when the
	// snapshot recorded them.
	TercileThresholds []float64       `json:"tercile_thresholds,omitempty"`
	Cells             []RegionMapCell `json:"cells"`
}

// BuildRegionMap decodes a region snapshot and the grid snapshot it links to
// into one row per cell. It refuses a pair that does not belong together or
// cannot be placed in space (no ring elevations), rather than returning a map
// whose geometry is guessed.
func BuildRegionMap(bg *BgSnapshot, regions *RegionSnapshot) (*RegionMap, error) {
	if bg == nil || regions == nil {
		return nil, fmt.Errorf("region map needs both a grid snapshot and a region snapshot")
	}
	if bg.SnapshotID != nil && regions.SnapshotID != *bg.SnapshotID {
		return nil, fmt.Errorf("region snapshot links grid snapshot %d, not %d", regions.SnapshotID, *bg.SnapshotID)
	}
	if bg.Rings <= 0 || bg.AzimuthBins <= 0 {
		return nil, fmt.Errorf("grid snapshot has %d rings and %d azimuth bins", bg.Rings, bg.AzimuthBins)
	}
	totalCells := bg.Rings * bg.AzimuthBins
	cells, err := deserializeGrid(bg.GridBlob)
	if err != nil {
		return nil, fmt.Errorf("decode grid snapshot: %w", err)
	}
	if len(cells) != totalCells {
		return nil, fmt.Errorf("grid snapshot holds %d cells, want %d rings x %d bins", len(cells), bg.Rings, bg.AzimuthBins)
	}
	var elevations []float64
	if bg.RingElevationsJSON == "" {
		return nil, fmt.Errorf("grid snapshot has no ring elevations, so its cells cannot be placed")
	}
	if err := json.Unmarshal([]byte(bg.RingElevationsJSON), &elevations); err != nil {
		return nil, fmt.Errorf("decode ring elevations: %w", err)
	}
	if len(elevations) != bg.Rings {
		return nil, fmt.Errorf("grid snapshot has %d ring elevations for %d rings", len(elevations), bg.Rings)
	}
	var regionData []RegionData
	if err := json.Unmarshal([]byte(regions.RegionsJSON), &regionData); err != nil {
		return nil, fmt.Errorf("decode regions: %w", err)
	}
	var variance VarianceData
	if regions.VarianceDataJSON == "" {
		return nil, fmt.Errorf("region snapshot has no settling variance")
	}
	if err := json.Unmarshal([]byte(regions.VarianceDataJSON), &variance); err != nil {
		return nil, fmt.Errorf("decode settling variance: %w", err)
	}
	if len(variance.VariancePerCell) != totalCells {
		return nil, fmt.Errorf("settling variance covers %d cells, want %d", len(variance.VariancePerCell), totalCells)
	}
	if variance.CellTercile != nil && len(variance.CellTercile) != totalCells {
		return nil, fmt.Errorf("cell terciles cover %d cells, want %d", len(variance.CellTercile), totalCells)
	}

	out := &RegionMap{
		Rings: bg.Rings, AzimuthBins: bg.AzimuthBins, RegionCount: len(regionData),
		SettlingFrames: regions.SettlingFrames, TercileThresholds: variance.TercileThresholds,
		Cells: make([]RegionMapCell, totalCells),
	}
	for i := range out.Cells {
		ring, bin := i/bg.AzimuthBins, i%bg.AzimuthBins
		c := cells[i]
		row := RegionMapCell{
			Cell: i, Ring: ring, AzimuthBin: bin,
			ElevationDeg: elevations[ring], AzimuthDeg: float64(2*bin+1) * 180 / float64(bg.AzimuthBins),
			RegionID: -1, RegionTercile: TercileNone.String(),
			SettlingVariance: variance.VariancePerCell[i],
			SettledRangeM:    c.AverageRangeMeters, LockedBaselineM: c.LockedBaseline,
			RangeSpreadM: c.RangeSpreadMeters, TimesSeen: c.TimesSeenCount,
		}
		if variance.CellTercile != nil {
			row.CellTercile = variance.CellTercile[i].String()
		}
		row.XM, row.YM, row.ZM = l2frames.SphericalToCartesian(float64(row.SettledRangeM), row.AzimuthDeg, row.ElevationDeg)
		out.Cells[i] = row
	}
	for _, r := range regionData {
		for _, idx := range r.CellList {
			if idx < 0 || idx >= totalCells {
				return nil, fmt.Errorf("region %d lists cell %d outside the %d-cell grid", r.ID, idx, totalCells)
			}
			row := &out.Cells[idx]
			if row.RegionID >= 0 {
				return nil, fmt.Errorf("cell %d is in both region %d and region %d", idx, row.RegionID, r.ID)
			}
			row.RegionID = r.ID
			row.RegionTercile = r.Tercile.String()
			row.RegionVolatile = r.Tercile == TercileVolatile
			row.RegionNoiseRelativeFraction = r.Params.NoiseRelativeFraction
			row.RegionNeighbourConfirmation = r.Params.NeighbourConfirmationCount
		}
	}
	return out, nil
}
