package analysis

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/recorder"
)

// ---------------------------------------------------------------------------
// trackStateName
// ---------------------------------------------------------------------------

func TestTrackStateName(t *testing.T) {
	tests := []struct {
		state visualiser.TrackState
		want  string
	}{
		{visualiser.TrackStateTentative, "tentative"},
		{visualiser.TrackStateConfirmed, "confirmed"},
		{visualiser.TrackStateDeleted, "deleted"},
		{visualiser.TrackState(99), "unknown"},
	}
	for _, tc := range tests {
		got := trackStateName(tc.state)
		if got != tc.want {
			t.Errorf("trackStateName(%d) = %q, want %q", tc.state, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// motionModelName
// ---------------------------------------------------------------------------

func TestMotionModelName(t *testing.T) {
	tests := []struct {
		model visualiser.MotionModel
		want  string
	}{
		{visualiser.MotionModelCV, "CV"},
		{visualiser.MotionModelCA, "CA"},
		{visualiser.MotionModel(99), "unknown"},
	}
	for _, tc := range tests {
		got := motionModelName(tc.model)
		if got != tc.want {
			t.Errorf("motionModelName(%d) = %q, want %q", tc.model, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// meanFloat32
// ---------------------------------------------------------------------------

func TestMeanFloat32(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := meanFloat32(nil); got != 0 {
			t.Errorf("meanFloat32(nil) = %v, want 0", got)
		}
	})
	t.Run("single", func(t *testing.T) {
		if got := meanFloat32([]float32{7.0}); got != 7.0 {
			t.Errorf("meanFloat32([7]) = %v, want 7", got)
		}
	})
	t.Run("multiple", func(t *testing.T) {
		got := meanFloat32([]float32{2, 4, 6})
		if got != 4.0 {
			t.Errorf("meanFloat32([2,4,6]) = %v, want 4", got)
		}
	})
}

// ---------------------------------------------------------------------------
// computeDistStats
// ---------------------------------------------------------------------------

func TestComputeDistStats(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		if got := computeDistStats(nil); got != nil {
			t.Errorf("computeDistStats(nil) = %v, want nil", got)
		}
	})
	t.Run("single value", func(t *testing.T) {
		ds := computeDistStats([]float64{5.0})
		if ds == nil {
			t.Fatal("expected non-nil DistStats")
		}
		if ds.Min != 5.0 || ds.Max != 5.0 || ds.Avg != 5.0 {
			t.Errorf("min/max/avg = %v/%v/%v, want 5/5/5", ds.Min, ds.Max, ds.Avg)
		}
		if ds.Samples != 1 {
			t.Errorf("samples = %d, want 1", ds.Samples)
		}
	})
	t.Run("known distribution", func(t *testing.T) {
		// 100 values: 1..100
		vals := make([]float64, 100)
		for i := range vals {
			vals[i] = float64(i + 1)
		}
		ds := computeDistStats(vals)
		if ds == nil {
			t.Fatal("expected non-nil DistStats")
		}
		if ds.Min != 1.0 {
			t.Errorf("min = %v, want 1", ds.Min)
		}
		if ds.Max != 100.0 {
			t.Errorf("max = %v, want 100", ds.Max)
		}
		if math.Abs(ds.Avg-50.5) > 0.01 {
			t.Errorf("avg = %v, want 50.5", ds.Avg)
		}
		if ds.Samples != 100 {
			t.Errorf("samples = %d, want 100", ds.Samples)
		}
		// P50 = floor(100*0.5) = index 50 → value 51
		if ds.P50 != 51 {
			t.Errorf("P50 = %v, want 51", ds.P50)
		}
		// P85 = floor(100*0.85) = index 85 → value 86
		if ds.P85 != 86 {
			t.Errorf("P85 = %v, want 86", ds.P85)
		}
		// P98 = floor(100*0.98) = index 98 → value 99
		if ds.P98 != 99 {
			t.Errorf("P98 = %v, want 99", ds.P98)
		}
	})
	t.Run("does not mutate input", func(t *testing.T) {
		vals := []float64{5, 3, 1, 4, 2}
		orig := make([]float64, len(vals))
		copy(orig, vals)
		computeDistStats(vals)
		for i, v := range vals {
			if v != orig[i] {
				t.Errorf("input mutated at index %d: got %v, want %v", i, v, orig[i])
			}
		}
	})
}

// ---------------------------------------------------------------------------
// buildSpeedHistogram
// ---------------------------------------------------------------------------

func TestBuildSpeedHistogram(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		if got := buildSpeedHistogram(nil, 1.0); got != nil {
			t.Errorf("buildSpeedHistogram(nil) = %v, want nil", got)
		}
	})
	t.Run("single value", func(t *testing.T) {
		bins := buildSpeedHistogram([]float32{2.5}, 1.0)
		if bins == nil {
			t.Fatal("expected non-nil bins")
		}
		// 2.5 m/s → bin index 2 (lower=2, upper=3)
		totalCount := 0
		for _, b := range bins {
			totalCount += b.Count
		}
		if totalCount != 1 {
			t.Errorf("total count = %d, want 1", totalCount)
		}
		if len(bins) < 3 {
			t.Fatalf("expected at least 3 bins, got %d", len(bins))
		}
		if bins[2].Count != 1 {
			t.Errorf("bin[2].count = %d, want 1", bins[2].Count)
		}
	})
	t.Run("negative speed clamped to bin 0", func(t *testing.T) {
		bins := buildSpeedHistogram([]float32{-1.0}, 1.0)
		if bins == nil {
			t.Fatal("expected non-nil bins")
		}
		// maxSpeed is 0 (since -1.0 < 0), nBins = ceil(0/1)+1 = 1
		// idx = int(-1.0/1.0) = -1 → clamped to 0
		if bins[0].Count != 1 {
			t.Errorf("bin[0].count = %d, want 1 (negative speed clamped)", bins[0].Count)
		}
	})
	t.Run("boundary speed clamped to last bin", func(t *testing.T) {
		// Speed exactly at maxSpeed may produce idx == nBins; clamped to nBins-1
		bins := buildSpeedHistogram([]float32{3.0}, 1.0)
		if bins == nil {
			t.Fatal("expected non-nil bins")
		}
		totalCount := 0
		for _, b := range bins {
			totalCount += b.Count
		}
		if totalCount != 1 {
			t.Errorf("total count = %d, want 1", totalCount)
		}
	})
	t.Run("bin boundaries are correct", func(t *testing.T) {
		bins := buildSpeedHistogram([]float32{0.5, 1.5}, 1.0)
		if bins == nil {
			t.Fatal("expected non-nil bins")
		}
		if bins[0].Lower != 0 || bins[0].Upper != 1.0 {
			t.Errorf("bin[0] = [%v, %v), want [0, 1)", bins[0].Lower, bins[0].Upper)
		}
		if bins[0].Count != 1 {
			t.Errorf("bin[0].count = %d, want 1", bins[0].Count)
		}
		if bins[1].Count != 1 {
			t.Errorf("bin[1].count = %d, want 1", bins[1].Count)
		}
	})
}

// ---------------------------------------------------------------------------
// GenerateReport — integration test using recorder + synthetic generator
// ---------------------------------------------------------------------------

// createTestVrlog writes a minimal .vrlog with the synthetic generator.
func createTestVrlog(t *testing.T, dir string, nFrames int) string {
	t.Helper()
	basePath := filepath.Join(dir, "test.vrlog")
	rec, err := recorder.NewRecorder(basePath, "test-sensor")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	gen := visualiser.NewSyntheticGenerator("test-sensor")
	for i := 0; i < nFrames; i++ {
		if err := rec.Record(gen.NextFrame()); err != nil {
			t.Fatalf("Record frame %d: %v", i, err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return basePath
}

func TestGenerateReport(t *testing.T) {
	tmpDir := t.TempDir()
	vrlogPath := createTestVrlog(t, tmpDir, 20)

	report, outPath, err := GenerateReport(vrlogPath)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	// Verify output file exists
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("output file does not exist: %v", err)
	}
	expectedPath := filepath.Join(vrlogPath, "analysis.json")
	if outPath != expectedPath {
		t.Errorf("outPath = %q, want %q", outPath, expectedPath)
	}

	// Verify JSON is valid by re-reading it
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var parsed AnalysisReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse output JSON: %v", err)
	}

	// Basic structural assertions
	if report.Version != "1.0" {
		t.Errorf("version = %q, want %q", report.Version, "1.0")
	}
	if report.GeneratedAt == "" {
		t.Error("generated_at is empty")
	}
	if report.Source == "" {
		t.Error("source is empty")
	}

	// Frame summary
	if report.FrameSummary.TotalFrames != 20 {
		t.Errorf("total_frames = %d, want 20", report.FrameSummary.TotalFrames)
	}

	// Frame interval should exist for >1 frame
	if report.FrameSummary.FrameIntervalMs == nil {
		t.Error("frame_interval_ms is nil for multi-frame recording")
	}

	// Synthetic generator creates tracks
	if report.TrackSummary.TotalTracks == 0 {
		t.Error("expected at least one track from synthetic generator")
	}
	if len(report.Tracks) == 0 {
		t.Error("expected non-empty track details")
	}

	// Recording metadata from header
	if report.Recording.SensorID == "" {
		t.Error("sensor_id is empty")
	}
}

func TestGenerateReportInvalidPath(t *testing.T) {
	_, _, err := GenerateReport("/nonexistent/path.vrlog")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestGenerateReportSingleFrame(t *testing.T) {
	// Single frame — no frame intervals
	tmpDir := t.TempDir()
	vrlogPath := createTestVrlog(t, tmpDir, 1)

	report, _, err := GenerateReport(vrlogPath)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	if report.FrameSummary.TotalFrames != 1 {
		t.Errorf("total_frames = %d, want 1", report.FrameSummary.TotalFrames)
	}
	// No interval stats for single frame
	if report.FrameSummary.FrameIntervalMs != nil {
		t.Error("expected nil frame_interval_ms for single frame")
	}
}

func TestGenerateReportTrackSorting(t *testing.T) {
	tmpDir := t.TempDir()
	vrlogPath := createTestVrlog(t, tmpDir, 30)

	report, _, err := GenerateReport(vrlogPath)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	// Verify tracks are sorted by first_seen_ns
	for i := 1; i < len(report.Tracks); i++ {
		if report.Tracks[i].FirstSeenNs < report.Tracks[i-1].FirstSeenNs {
			t.Errorf("tracks not sorted by first_seen_ns: track[%d]=%d < track[%d]=%d",
				i, report.Tracks[i].FirstSeenNs, i-1, report.Tracks[i-1].FirstSeenNs)
		}
	}
}

func TestGenerateReportClassificationDistribution(t *testing.T) {
	tmpDir := t.TempDir()
	vrlogPath := createTestVrlog(t, tmpDir, 30)

	report, _, err := GenerateReport(vrlogPath)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	// Should have classification distribution with at least one class
	if report.TrackSummary.ConfirmedTracks > 0 && len(report.ClassificationDistribution) == 0 {
		t.Error("expected non-empty classification distribution for confirmed tracks")
	}

	// Verify ClassStats fields are sensible
	for cls, stats := range report.ClassificationDistribution {
		if stats.Count <= 0 {
			t.Errorf("class %q has count %d", cls, stats.Count)
		}
		if stats.AvgObservations < 0 {
			t.Errorf("class %q has negative avg_observations", cls)
		}
	}
}

func TestGenerateReportSpeedHistogram(t *testing.T) {
	tmpDir := t.TempDir()
	vrlogPath := createTestVrlog(t, tmpDir, 30)

	report, _, err := GenerateReport(vrlogPath)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	if report.SpeedHistogram.BinWidthMps != 1.0 {
		t.Errorf("bin_width = %v, want 1.0", report.SpeedHistogram.BinWidthMps)
	}
	if report.TrackSummary.ConfirmedTracks > 0 {
		if report.SpeedHistogram.Bins == nil {
			t.Error("expected non-nil histogram bins for confirmed tracks")
		}
		// Verify total count across bins matches confirmed track count
		totalCount := 0
		for _, b := range report.SpeedHistogram.Bins {
			totalCount += b.Count
		}
		if totalCount != report.SpeedHistogram.TotalTracks {
			t.Errorf("histogram total count %d != total_tracks %d", totalCount, report.SpeedHistogram.TotalTracks)
		}
	}
}

func TestGenerateReportOcclusion(t *testing.T) {
	tmpDir := t.TempDir()
	vrlogPath := createTestVrlog(t, tmpDir, 20)

	report, _, err := GenerateReport(vrlogPath)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	if report.TrackSummary.Occlusion == nil {
		t.Fatal("expected non-nil occlusion summary")
	}
	// Occlusion values should be non-negative
	if report.TrackSummary.Occlusion.TotalOcclusions < 0 {
		t.Error("total occlusions < 0")
	}
}

func TestGenerateReportFragmentation(t *testing.T) {
	tmpDir := t.TempDir()
	vrlogPath := createTestVrlog(t, tmpDir, 20)

	report, _, err := GenerateReport(vrlogPath)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	// Fragmentation ratio in [0, 1]
	if report.TrackSummary.FragmentationRatio < 0 || report.TrackSummary.FragmentationRatio > 1 {
		t.Errorf("fragmentation_ratio = %v, want [0,1]", report.TrackSummary.FragmentationRatio)
	}
}

// Test with a vrlog that has frames with no tracks/clusters/points
func TestGenerateReportEmptyFrames(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "empty.vrlog")

	// Create a vrlog with frames that have no point cloud, no clusters, no tracks
	rec, err := recorder.NewRecorder(basePath, "empty-sensor")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	for i := 0; i < 5; i++ {
		frame := &visualiser.FrameBundle{
			FrameID:        uint64(i),
			TimestampNanos: int64(i) * 100_000_000, // 100ms apart
			SensorID:       "empty-sensor",
			CoordinateFrame: visualiser.CoordinateFrameInfo{
				ReferenceFrame: "ENU",
			},
			// No PointCloud, Clusters, or Tracks
		}
		if err := rec.Record(frame); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	report, _, err := GenerateReport(basePath)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	if report.FrameSummary.TotalFrames != 5 {
		t.Errorf("total_frames = %d, want 5", report.FrameSummary.TotalFrames)
	}
	if report.TrackSummary.TotalTracks != 0 {
		t.Errorf("total_tracks = %d, want 0", report.TrackSummary.TotalTracks)
	}
	if report.FrameSummary.AvgPointsPerFrame != 0 {
		t.Errorf("avg_points_per_frame = %v, want 0", report.FrameSummary.AvgPointsPerFrame)
	}
	if report.SpeedHistogram.Bins != nil {
		t.Error("expected nil histogram bins for zero tracks")
	}
}

// Test with frames containing tracks to exercise all track accumulation branches
func TestGenerateReportWithMixedTracks(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "mixed.vrlog")

	rec, err := recorder.NewRecorder(basePath, "mixed-sensor")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	baseTime := int64(1_000_000_000_000) // 1000 seconds in nanos

	for i := 0; i < 10; i++ {
		ts := baseTime + int64(i)*100_000_000
		frame := &visualiser.FrameBundle{
			FrameID:        uint64(i),
			TimestampNanos: ts,
			SensorID:       "mixed-sensor",
			CoordinateFrame: visualiser.CoordinateFrameInfo{
				ReferenceFrame: "ENU",
			},
			PointCloud: &visualiser.PointCloudFrame{
				FrameID:        uint64(i),
				TimestampNanos: ts,
				SensorID:       "mixed-sensor",
				X:              []float32{1.0, 2.0},
				Y:              []float32{1.0, 2.0},
				Z:              []float32{0.5, 0.5},
				Intensity:      []uint8{100, 200},
				Classification: []uint8{1, 0}, // one foreground, one background
				PointCount:     2,
			},
			Clusters: &visualiser.ClusterSet{
				FrameID:        uint64(i),
				TimestampNanos: ts,
				Clusters:       []visualiser.Cluster{{ClusterID: 1}},
			},
			Tracks: &visualiser.TrackSet{
				FrameID:        uint64(i),
				TimestampNanos: ts,
				Tracks: []visualiser.Track{
					{
						TrackID:           "confirmed-1",
						State:             visualiser.TrackStateConfirmed,
						SpeedMps:          5.0,
						AvgSpeedMps:       5.0,
						PeakSpeedMps:      6.0,
						X:                 float32(i),
						Y:                 float32(i),
						BBoxLength:        2.0,
						BBoxWidth:         1.5,
						BBoxHeight:        1.0,
						HeightP95Max:      1.2,
						Hits:              i + 1,
						Misses:            1,
						ObservationCount:  i + 1,
						OcclusionCount:    2,
						Confidence:        0.9,
						MotionModel:       visualiser.MotionModelCV,
						ObjectClass:       "car",
						ClassConfidence:   0.85,
						TrackLengthMetres: float32(i) * 0.5,
						FirstSeenNanos:    baseTime,
						LastSeenNanos:     ts,
					},
					{
						TrackID:          "tentative-1",
						State:            visualiser.TrackStateTentative,
						SpeedMps:         3.0,
						ObservationCount: 2,
						MotionModel:      visualiser.MotionModelCA,
						FirstSeenNanos:   ts,
						LastSeenNanos:    ts,
					},
				},
			},
		}
		if i == 5 {
			// Add a deleted track mid-way
			frame.Tracks.Tracks = append(frame.Tracks.Tracks, visualiser.Track{
				TrackID:        "deleted-1",
				State:          visualiser.TrackStateDeleted,
				SpeedMps:       1.0,
				FirstSeenNanos: baseTime,
				LastSeenNanos:  ts,
			})
		}
		if err := rec.Record(frame); err != nil {
			t.Fatalf("Record frame %d: %v", i, err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	report, _, err := GenerateReport(basePath)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	// Verify confirmed/tentative/deleted counts
	if report.TrackSummary.ConfirmedTracks < 1 {
		t.Error("expected at least 1 confirmed track")
	}

	// Should have foreground percentage > 0
	if report.FrameSummary.ForegroundPct <= 0 {
		t.Error("expected positive foreground_pct")
	}

	// Verify track details have expected fields
	for _, td := range report.Tracks {
		if td.TrackID == "confirmed-1" {
			if td.State != "confirmed" {
				t.Errorf("track state = %q, want confirmed", td.State)
			}
			if td.MotionModel != "CV" {
				t.Errorf("motion model = %q, want CV", td.MotionModel)
			}
			if td.ObjectClass != "car" {
				t.Errorf("object_class = %q, want car", td.ObjectClass)
			}
			if td.PeakSpeedMps != 6.0 {
				t.Errorf("peak_speed = %v, want 6.0", td.PeakSpeedMps)
			}
		}
	}

	// Classification distribution should include "car"
	if _, ok := report.ClassificationDistribution["car"]; !ok {
		t.Error("expected 'car' in classification distribution")
	}
}
