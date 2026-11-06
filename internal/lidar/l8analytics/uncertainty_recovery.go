package l8analytics

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// FrozenRecoveryReport is only G-UNC-1 row 7. Synthetic recovery cannot
// replace the labelled manoeuvre row or the held-out identity guard.
type FrozenRecoveryReport struct {
	SchemaVersion   int    `json:"schema_version"`
	Scene           string `json:"scene"`
	CalibrationID   string `json:"calibration_id"`
	RecoveredFrames int    `json:"recovered_frames"` // -1 means never recovered
	Status          string `json:"row_7_status"`
	GateVerdict     string `json:"gate_verdict"`
}

// CheckFrozenRecovery repeats the exact seeded scene predeclared by
// TestRecoveryAfterFiveFrameOcclusionIsUnderThreeFrames, using the supplied
// frozen coefficients instead of its representative synthetic table.
func CheckFrozenRecovery(cal *l5tracks.NoiseCalibration) (*FrozenRecoveryReport, error) {
	if err := cal.Validate(); err != nil {
		return nil, err
	}
	if cal.ID == "" {
		return nil, fmt.Errorf("frozen recovery requires a calibration ID")
	}
	cfg := l5tracks.DefaultTrackerConfig()
	cfg.AdaptiveMeasurementNoise = true
	cfg.MeasurementNoiseCalibration = cal
	return frozenRecovery(cfg, cal.ID), nil
}

func frozenRecovery(cfg l5tracks.TrackerConfig, tableID string) *FrozenRecoveryReport {
	const occludedFrom, occludedTo = 25, 30
	tk := l5tracks.NewTracker(cfg)
	rng := rand.New(rand.NewSource(13))
	x, y := -30.0, 15.0
	var sequence int64
	recovered := -1
	for i := 0; i < 45; i++ {
		r := math.Hypot(x, y)
		ux, uy := x/r, y/r
		nr, nt := rng.NormFloat64()*0.03, rng.NormFloat64()*0.08
		mx, my := float32(x+nr*ux-nt*uy), float32(y+nr*uy+nt*ux)
		var clusters []l4perception.WorldCluster
		if i < occludedFrom || i >= occludedTo {
			clusters = []l4perception.WorldCluster{{CentroidX: mx, CentroidY: my,
				BoundingBoxLength: 4.5, BoundingBoxWidth: 1.8, BoundingBoxHeight: 1.5, PointsCount: 120,
				OBB: &l4perception.OrientedBoundingBox{CenterX: mx, CenterY: my, Length: 4.5, Width: 1.8, Height: 1.5}}}
		}
		tk.Update(clusters, time.Unix(1_700_000_000, 0).Add(time.Duration(i)*100*time.Millisecond))
		for _, tr := range tk.Tracks {
			if i == occludedFrom-1 && tr.TrackState == l5tracks.TrackConfirmed {
				sequence = tr.CreationSequence
			}
			if sequence != 0 && i >= occludedTo && recovered < 0 && tr.CreationSequence == sequence && tr.Misses == 0 &&
				math.Hypot(float64(tr.X)-x, float64(tr.Y)-y) <= 0.5 {
				recovered = i - occludedTo + 1
			}
		}
		x += 8 * 0.1
	}
	status := "outside"
	if recovered > 0 && recovered < 3 {
		status = "within"
	}
	return &FrozenRecoveryReport{SchemaVersion: 1, Scene: "g_unc_1_five_frame_occlusion_seed_13_v1",
		CalibrationID: tableID, RecoveredFrames: recovered, Status: status, GateVerdict: "individual_row_only_no_promotion"}
}
