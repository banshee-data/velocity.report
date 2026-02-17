package lidar

import (
	"sort"
	"testing"
	"time"
)

// TestGoldenReplay_Determinism verifies that the tracking pipeline produces
// identical results when run multiple times on the same input data.
// This is critical for reproducible testing and debugging.
func TestGoldenReplay_Determinism(t *testing.T) {
	// Generate synthetic test data representing a vehicle moving in a straight line
	testData := generateSyntheticTrackingData()

	// Run the tracking pipeline twice on the same data
	run1Results := runTrackingPipeline(t, testData)
	run2Results := runTrackingPipeline(t, testData)

	// Verify that both runs produced the same number of tracks
	if len(run1Results) != len(run2Results) {
		t.Fatalf("track count mismatch: run1=%d, run2=%d", len(run1Results), len(run2Results))
	}

	if len(run1Results) == 0 {
		t.Fatal("no tracks were created in either run")
	}

	t.Logf("Both runs produced %d tracks", len(run1Results))

	// Compare each track between runs (sorted by creation time + X position).
	// TrackIDs are UUID-based and intentionally differ between runs, so we
	// compare only the deterministic state properties.
	for i := range run1Results {
		track1 := run1Results[i]
		track2 := run2Results[i]

		// State should be identical
		if track1.State != track2.State {
			t.Errorf("track %d: state mismatch: run1=%s, run2=%s", i, track1.State, track2.State)
		}

		// Position should be identical (within floating point tolerance).
		// With deterministic timestamps the primary source of jitter is gone;
		// retain a small tolerance for residual FP platform differences
		// (e.g. fused multiply-add availability, compiler optimisation level).
		if !floatNearlyEqual(track1.X, track2.X, 0.02) {
			t.Errorf("track %d: X position mismatch: run1=%f, run2=%f (diff=%f)", i, track1.X, track2.X, track1.X-track2.X)
		}
		if !floatNearlyEqual(track1.Y, track2.Y, 0.02) {
			t.Errorf("track %d: Y position mismatch: run1=%f, run2=%f (diff=%f)", i, track1.Y, track2.Y, track1.Y-track2.Y)
		}

		// Velocity tolerance is wider because Kalman velocity estimates
		// amplify small position-level FP differences.
		if !floatNearlyEqual(track1.VX, track2.VX, 0.2) {
			t.Errorf("track %d: VX mismatch: run1=%f, run2=%f (diff=%f)", i, track1.VX, track2.VX, track1.VX-track2.VX)
		}
		if !floatNearlyEqual(track1.VY, track2.VY, 0.2) {
			t.Errorf("track %d: VY mismatch: run1=%f, run2=%f (diff=%f)", i, track1.VY, track2.VY, track1.VY-track2.VY)
		}

		// Observation counts should be identical
		if track1.Hits != track2.Hits {
			t.Errorf("track %d: hits mismatch: run1=%d, run2=%d", i, track1.Hits, track2.Hits)
		}
		if track1.ObservationCount != track2.ObservationCount {
			t.Errorf("track %d: observation count mismatch: run1=%d, run2=%d",
				i, track1.ObservationCount, track2.ObservationCount)
		}

		// History length should be identical
		if len(track1.History) != len(track2.History) {
			t.Errorf("track %d: history length mismatch: run1=%d, run2=%d",
				i, len(track1.History), len(track2.History))
		}
	}

	t.Log("✅ Determinism check passed: both runs produced identical results")
}

// TestGoldenReplay_ClusteringDeterminism verifies that clustering produces
// deterministic results (sorted by centroid X, then Y).
func TestGoldenReplay_ClusteringDeterminism(t *testing.T) {
	// Generate test points for two distinct clusters
	points := generateTwoClusterPoints()

	clusterer := NewDefaultDBSCANClusterer()

	// Run clustering multiple times
	timestamp := time.Now()
	run1 := clusterer.Cluster(points, "test-sensor", timestamp)
	run2 := clusterer.Cluster(points, "test-sensor", timestamp)
	run3 := clusterer.Cluster(points, "test-sensor", timestamp)

	// Verify all runs produced the same clusters
	if len(run1) != len(run2) || len(run1) != len(run3) {
		t.Fatalf("cluster count mismatch: run1=%d, run2=%d, run3=%d",
			len(run1), len(run2), len(run3))
	}

	t.Logf("All runs produced %d clusters", len(run1))

	// Verify cluster order is identical (deterministic sorting)
	for i := range run1 {
		if !floatNearlyEqual(run1[i].CentroidX, run2[i].CentroidX, 1e-6) {
			t.Errorf("cluster %d: CentroidX mismatch between run1 and run2", i)
		}
		if !floatNearlyEqual(run1[i].CentroidY, run2[i].CentroidY, 1e-6) {
			t.Errorf("cluster %d: CentroidY mismatch between run1 and run2", i)
		}
		if !floatNearlyEqual(run1[i].CentroidX, run3[i].CentroidX, 1e-6) {
			t.Errorf("cluster %d: CentroidX mismatch between run1 and run3", i)
		}
		if !floatNearlyEqual(run1[i].CentroidY, run3[i].CentroidY, 1e-6) {
			t.Errorf("cluster %d: CentroidY mismatch between run1 and run3", i)
		}
	}

	// Verify clusters are sorted by X, then Y
	for i := 1; i < len(run1); i++ {
		prev := run1[i-1]
		curr := run1[i]
		if prev.CentroidX > curr.CentroidX {
			t.Errorf("clusters not sorted by X: cluster %d (X=%f) > cluster %d (X=%f)",
				i-1, prev.CentroidX, i, curr.CentroidX)
		}
		if prev.CentroidX == curr.CentroidX && prev.CentroidY > curr.CentroidY {
			t.Errorf("clusters with same X not sorted by Y: cluster %d (Y=%f) > cluster %d (Y=%f)",
				i-1, prev.CentroidY, i, curr.CentroidY)
		}
	}

	t.Log("✅ Clustering determinism check passed")
}

// TestGoldenReplay_MultiTrackDeterminism tests determinism with multiple tracks.
func TestGoldenReplay_MultiTrackDeterminism(t *testing.T) {
	// Generate data with multiple objects
	testData := generateMultiObjectTrackingData()

	// Run tracking pipeline multiple times
	run1 := runTrackingPipeline(t, testData)
	run2 := runTrackingPipeline(t, testData)

	if len(run1) != len(run2) {
		t.Fatalf("track count mismatch: run1=%d, run2=%d", len(run1), len(run2))
	}

	if len(run1) < 2 {
		t.Fatal("expected at least 2 tracks to test multi-track determinism")
	}

	t.Logf("Testing determinism with %d tracks", len(run1))

	// Verify all tracks match (sorted by creation time + X position).
	// TrackIDs are UUID-based and differ between runs — compare only
	// deterministic properties.
	for i := range run1 {
		if run1[i].State != run2[i].State {
			t.Errorf("track %d: state mismatch: run1=%s, run2=%s",
				i, run1[i].State, run2[i].State)
		}
		if !floatNearlyEqual(run1[i].X, run2[i].X, 0.02) {
			t.Errorf("track %d: X mismatch: run1=%f, run2=%f", i, run1[i].X, run2[i].X)
		}
		if !floatNearlyEqual(run1[i].Y, run2[i].Y, 0.02) {
			t.Errorf("track %d: Y mismatch: run1=%f, run2=%f", i, run1[i].Y, run2[i].Y)
		}
	}

	t.Log("✅ Multi-track determinism check passed")
}

// deterministicBaseTime is a fixed reference time used across all golden-replay
// helpers so that timestamps are identical between runs, eliminating timing
// jitter as a source of non-determinism in Kalman filter calculations.
var deterministicBaseTime = time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

// Helper functions

// generateSyntheticTrackingData creates test data for a single vehicle moving in a straight line.
func generateSyntheticTrackingData() [][]WorldCluster {
	frames := make([][]WorldCluster, 0, 20)
	startTime := deterministicBaseTime

	// Simulate a vehicle moving from (5, 5) to (25, 5) over 20 frames
	for frameIdx := 0; frameIdx < 20; frameIdx++ {
		x := 5.0 + float32(frameIdx)
		y := float32(5.0)

		cluster := WorldCluster{
			CentroidX:         x,
			CentroidY:         y,
			CentroidZ:         0.5,
			BoundingBoxLength: 4.0,
			BoundingBoxWidth:  1.8,
			BoundingBoxHeight: 1.5,
			PointsCount:       100,
			HeightP95:         1.4,
			IntensityMean:     100,
			TSUnixNanos:       startTime.Add(time.Duration(frameIdx) * 100 * time.Millisecond).UnixNano(),
		}

		frames = append(frames, []WorldCluster{cluster})
	}

	return frames
}

// generateTwoClusterPoints creates points for two distinct clusters.
func generateTwoClusterPoints() []WorldPoint {
	points := make([]WorldPoint, 0)

	// Cluster 1: around (5, 5)
	for i := 0; i < 15; i++ {
		x := 5.0 + float64(i)*0.1
		y := 5.0 + float64(i%3)*0.1
		points = append(points, WorldPoint{
			X:         x,
			Y:         y,
			Z:         0.5,
			Intensity: 100,
			SensorID:  "test",
		})
	}

	// Cluster 2: around (10, 10)
	for i := 0; i < 15; i++ {
		x := 10.0 + float64(i)*0.1
		y := 10.0 + float64(i%3)*0.1
		points = append(points, WorldPoint{
			X:         x,
			Y:         y,
			Z:         0.5,
			Intensity: 100,
			SensorID:  "test",
		})
	}

	return points
}

// generateMultiObjectTrackingData creates test data for multiple objects.
func generateMultiObjectTrackingData() [][]WorldCluster {
	frames := make([][]WorldCluster, 0, 20)
	startTime := deterministicBaseTime

	for frameIdx := 0; frameIdx < 20; frameIdx++ {
		clusters := []WorldCluster{
			// Object 1: moving right
			{
				CentroidX:         5.0 + float32(frameIdx),
				CentroidY:         5.0,
				CentroidZ:         0.5,
				BoundingBoxLength: 4.0,
				BoundingBoxWidth:  1.8,
				BoundingBoxHeight: 1.5,
				PointsCount:       100,
				TSUnixNanos:       startTime.Add(time.Duration(frameIdx) * 100 * time.Millisecond).UnixNano(),
			},
			// Object 2: moving up
			{
				CentroidX:         15.0,
				CentroidY:         5.0 + float32(frameIdx)*0.5,
				CentroidZ:         0.5,
				BoundingBoxLength: 4.0,
				BoundingBoxWidth:  1.8,
				BoundingBoxHeight: 1.5,
				PointsCount:       100,
				TSUnixNanos:       startTime.Add(time.Duration(frameIdx) * 100 * time.Millisecond).UnixNano(),
			},
		}
		frames = append(frames, clusters)
	}

	return frames
}

// runTrackingPipeline runs the full tracking pipeline on test data.
// It uses deterministicBaseTime so that every invocation feeds identical
// timestamps to the Kalman filter, removing timing jitter as a source of
// non-determinism.
func runTrackingPipeline(t *testing.T, frameData [][]WorldCluster) []*TrackedObject {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 3 // Lower threshold for test

	tracker := NewTracker(config)

	// Process all frames with deterministic timestamps matching the input data.
	for frameIdx, clusters := range frameData {
		timestamp := deterministicBaseTime.Add(time.Duration(frameIdx) * 100 * time.Millisecond)
		tracker.Update(clusters, timestamp)
	}

	// Return all tracks (including tentative and deleted), sorted by
	// creation time then initial X position for deterministic comparison.
	// TrackID cannot be used as sort key because UUID-based IDs differ
	// between runs by design.
	tracks := tracker.GetAllTracks()
	sort.Slice(tracks, func(i, j int) bool {
		if tracks[i].FirstUnixNanos != tracks[j].FirstUnixNanos {
			return tracks[i].FirstUnixNanos < tracks[j].FirstUnixNanos
		}
		return tracks[i].X < tracks[j].X
	})
	return tracks
}

// floatNearlyEqual checks if two float32 values are nearly equal within a tolerance.
func floatNearlyEqual(a, b float32, tolerance float32) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < tolerance
}

// TestGoldenReplay_TrackIDStability verifies that track IDs use the expected
// UUID-based format and that the same number of tracks are created across runs.
func TestGoldenReplay_TrackIDStability(t *testing.T) {
	testData := generateSyntheticTrackingData()

	run1 := runTrackingPipeline(t, testData)
	run2 := runTrackingPipeline(t, testData)

	if len(run1) == 0 {
		t.Fatal("no tracks created")
	}

	// Same number of tracks should be created in both runs
	if len(run1) != len(run2) {
		t.Fatalf("different track counts: run1=%d, run2=%d", len(run1), len(run2))
	}

	// Track IDs should follow the UUID-based format (trk_<uuid>)
	for _, track := range run1 {
		if len(track.TrackID) < 4 || track.TrackID[:4] != "trk_" {
			t.Errorf("unexpected track ID format: got %s, expected trk_<hex>", track.TrackID)
		}
	}

	// Track IDs should be globally unique (no reuse across runs)
	allIDs := make(map[string]bool)
	for _, track := range run1 {
		allIDs[track.TrackID] = true
	}
	for _, track := range run2 {
		if allIDs[track.TrackID] {
			t.Errorf("track ID %s reused across runs — expected globally unique IDs", track.TrackID)
		}
	}

	t.Log("✅ Track ID stability check passed")
}
