package sqlite

import (
	"testing"
)

func TestCompareParams_Identical(t *testing.T) {
	p := &RunParams{
		Background: BackgroundParamsExport{
			BackgroundUpdateFraction:       0.05,
			ClosenessSensitivityMultiplier: 1.5,
			SafetyMarginMeters:             0.3,
			NeighborConfirmationCount:      2,
			NoiseRelativeFraction:          0.1,
			SeedFromFirstObservation:       true,
		},
		Clustering: ClusteringParamsExport{
			Eps:    0.5,
			MinPts: 4,
		},
		Tracking: TrackingParamsExport{
			MaxTracks:             50,
			MaxMisses:             5,
			HitsToConfirm:         3,
			GatingDistanceSquared: 4.0,
		},
	}

	diff := compareParams(p, p)
	if len(diff) != 0 {
		t.Errorf("expected empty diff for identical params, got %v", diff)
	}
}

func TestCompareParams_BackgroundDifferences(t *testing.T) {
	p1 := &RunParams{
		Background: BackgroundParamsExport{
			BackgroundUpdateFraction:       0.05,
			ClosenessSensitivityMultiplier: 1.5,
			SafetyMarginMeters:             0.3,
			NeighborConfirmationCount:      2,
			NoiseRelativeFraction:          0.1,
			SeedFromFirstObservation:       true,
		},
	}
	p2 := &RunParams{
		Background: BackgroundParamsExport{
			BackgroundUpdateFraction:       0.10,
			ClosenessSensitivityMultiplier: 2.0,
			SafetyMarginMeters:             0.3,
			NeighborConfirmationCount:      2,
			NoiseRelativeFraction:          0.1,
			SeedFromFirstObservation:       true,
		},
	}

	diff := compareParams(p1, p2)
	bgDiff, ok := diff["background"].(map[string]any)
	if !ok {
		t.Fatal("expected 'background' key in diff")
	}
	if _, ok := bgDiff["background_update_fraction"]; !ok {
		t.Error("expected background_update_fraction in diff")
	}
	if _, ok := bgDiff["closeness_sensitivity_multiplier"]; !ok {
		t.Error("expected closeness_sensitivity_multiplier in diff")
	}
	// Fields that didn't change should not appear
	if _, ok := bgDiff["safety_margin_meters"]; ok {
		t.Error("safety_margin_meters should not appear when unchanged")
	}
}

func TestCompareParams_ClusteringDifferences(t *testing.T) {
	p1 := &RunParams{
		Clustering: ClusteringParamsExport{Eps: 0.5, MinPts: 4},
	}
	p2 := &RunParams{
		Clustering: ClusteringParamsExport{Eps: 0.8, MinPts: 6},
	}

	diff := compareParams(p1, p2)
	clDiff, ok := diff["clustering"].(map[string]any)
	if !ok {
		t.Fatal("expected 'clustering' key in diff")
	}
	if _, ok := clDiff["eps"]; !ok {
		t.Error("expected eps in clustering diff")
	}
	if _, ok := clDiff["min_pts"]; !ok {
		t.Error("expected min_pts in clustering diff")
	}
}

func TestCompareParams_TrackingDifferences(t *testing.T) {
	p1 := &RunParams{
		Tracking: TrackingParamsExport{MaxTracks: 50, MaxMisses: 5, HitsToConfirm: 3, GatingDistanceSquared: 4.0},
	}
	p2 := &RunParams{
		Tracking: TrackingParamsExport{MaxTracks: 100, MaxMisses: 10, HitsToConfirm: 3, GatingDistanceSquared: 4.0},
	}

	diff := compareParams(p1, p2)
	trDiff, ok := diff["tracking"].(map[string]any)
	if !ok {
		t.Fatal("expected 'tracking' key in diff")
	}
	if _, ok := trDiff["max_tracks"]; !ok {
		t.Error("expected max_tracks in tracking diff")
	}
	if _, ok := trDiff["max_misses"]; !ok {
		t.Error("expected max_misses in tracking diff")
	}
	// hits_to_confirm and gating_distance_squared should not appear
	if _, ok := trDiff["hits_to_confirm"]; ok {
		t.Error("hits_to_confirm should not appear when unchanged")
	}
	if _, ok := trDiff["gating_distance_squared"]; ok {
		t.Error("gating_distance_squared should not appear when unchanged")
	}
}

func TestCompareParams_AllDifferent(t *testing.T) {
	p1 := &RunParams{
		Background: BackgroundParamsExport{BackgroundUpdateFraction: 0.05},
		Clustering: ClusteringParamsExport{Eps: 0.5},
		Tracking:   TrackingParamsExport{MaxTracks: 50},
	}
	p2 := &RunParams{
		Background: BackgroundParamsExport{BackgroundUpdateFraction: 0.10},
		Clustering: ClusteringParamsExport{Eps: 0.8},
		Tracking:   TrackingParamsExport{MaxTracks: 100},
	}

	diff := compareParams(p1, p2)
	if _, ok := diff["background"]; !ok {
		t.Error("expected background diff")
	}
	if _, ok := diff["clustering"]; !ok {
		t.Error("expected clustering diff")
	}
	if _, ok := diff["tracking"]; !ok {
		t.Error("expected tracking diff")
	}
}

func TestCompareParams_NoDifferences(t *testing.T) {
	p := &RunParams{
		Background: BackgroundParamsExport{
			BackgroundUpdateFraction: 0.05,
			NoiseRelativeFraction:    0.1,
		},
		Clustering: ClusteringParamsExport{Eps: 0.5, MinPts: 4},
		Tracking:   TrackingParamsExport{MaxTracks: 50, MaxMisses: 5},
	}

	// Copy the same values
	p2 := &RunParams{
		Background: BackgroundParamsExport{
			BackgroundUpdateFraction: 0.05,
			NoiseRelativeFraction:    0.1,
		},
		Clustering: ClusteringParamsExport{Eps: 0.5, MinPts: 4},
		Tracking:   TrackingParamsExport{MaxTracks: 50, MaxMisses: 5},
	}

	diff := compareParams(p, p2)
	if len(diff) != 0 {
		t.Errorf("expected empty diff for identical params, got %v", diff)
	}
}

func TestCompareParams_SeedFromFirstDifference(t *testing.T) {
	p1 := &RunParams{
		Background: BackgroundParamsExport{SeedFromFirstObservation: true},
	}
	p2 := &RunParams{
		Background: BackgroundParamsExport{SeedFromFirstObservation: false},
	}

	diff := compareParams(p1, p2)
	bgDiff, ok := diff["background"].(map[string]any)
	if !ok {
		t.Fatal("expected 'background' key in diff")
	}
	if _, ok := bgDiff["seed_from_first_observation"]; !ok {
		t.Error("expected seed_from_first_observation in diff")
	}
}

func TestComputeTemporalIoU_FullOverlap(t *testing.T) {
	ref := &RunTrack{StartUnixNanos: 100, EndUnixNanos: 200}
	cand := &RunTrack{StartUnixNanos: 100, EndUnixNanos: 200}

	iou := computeTemporalIoU(ref, cand)
	if iou != 1.0 {
		t.Errorf("expected IoU 1.0 for full overlap, got %f", iou)
	}
}

func TestComputeTemporalIoU_NoOverlap(t *testing.T) {
	ref := &RunTrack{StartUnixNanos: 100, EndUnixNanos: 200}
	cand := &RunTrack{StartUnixNanos: 300, EndUnixNanos: 400}

	iou := computeTemporalIoU(ref, cand)
	if iou != 0.0 {
		t.Errorf("expected IoU 0.0 for no overlap, got %f", iou)
	}
}

func TestComputeTemporalIoU_PartialOverlap(t *testing.T) {
	ref := &RunTrack{StartUnixNanos: 100, EndUnixNanos: 300}
	cand := &RunTrack{StartUnixNanos: 200, EndUnixNanos: 400}

	iou := computeTemporalIoU(ref, cand)
	// Intersection: 200-300 = 100, Union: 100-400 = 300, IoU = 100/300 ≈ 0.333
	if iou < 0.33 || iou > 0.34 {
		t.Errorf("expected IoU ~0.333 for partial overlap, got %f", iou)
	}
}

func TestComputeTemporalIoU_ContainedTrack(t *testing.T) {
	ref := &RunTrack{StartUnixNanos: 100, EndUnixNanos: 400}
	cand := &RunTrack{StartUnixNanos: 200, EndUnixNanos: 300}

	iou := computeTemporalIoU(ref, cand)
	// Intersection: 200-300 = 100, Union: 100-400 = 300, IoU = 100/300 ≈ 0.333
	if iou < 0.33 || iou > 0.34 {
		t.Errorf("expected IoU ~0.333, got %f", iou)
	}
}

func TestComputeTemporalIoU_AdjacentTracks(t *testing.T) {
	ref := &RunTrack{StartUnixNanos: 100, EndUnixNanos: 200}
	cand := &RunTrack{StartUnixNanos: 200, EndUnixNanos: 300}

	iou := computeTemporalIoU(ref, cand)
	// Edge-touching: intersection is 0 duration, IoU should be 0
	if iou != 0.0 {
		t.Errorf("expected IoU 0.0 for adjacent tracks, got %f", iou)
	}
}
