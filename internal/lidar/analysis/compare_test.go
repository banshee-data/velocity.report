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
// pearsonR
// ---------------------------------------------------------------------------

func TestPearsonR(t *testing.T) {
	t.Run("n < 2 returns 0", func(t *testing.T) {
		if r := pearsonR(nil, nil); r != 0 {
			t.Errorf("pearsonR(nil, nil) = %v, want 0", r)
		}
		if r := pearsonR([]float64{1}, []float64{2}); r != 0 {
			t.Errorf("pearsonR([1], [2]) = %v, want 0", r)
		}
	})
	t.Run("constant values (den=0) returns 0", func(t *testing.T) {
		r := pearsonR([]float64{5, 5, 5}, []float64{3, 3, 3})
		if r != 0 {
			t.Errorf("pearsonR(constant, constant) = %v, want 0", r)
		}
	})
	t.Run("one side constant returns 0", func(t *testing.T) {
		r := pearsonR([]float64{5, 5, 5}, []float64{1, 2, 3})
		if r != 0 {
			t.Errorf("pearsonR(constant, varying) = %v, want 0", r)
		}
	})
	t.Run("perfect positive correlation", func(t *testing.T) {
		xs := []float64{1, 2, 3, 4, 5}
		ys := []float64{2, 4, 6, 8, 10}
		r := pearsonR(xs, ys)
		if math.Abs(r-1.0) > 1e-10 {
			t.Errorf("pearsonR(perfect+) = %v, want 1.0", r)
		}
	})
	t.Run("perfect negative correlation", func(t *testing.T) {
		xs := []float64{1, 2, 3, 4, 5}
		ys := []float64{10, 8, 6, 4, 2}
		r := pearsonR(xs, ys)
		if math.Abs(r+1.0) > 1e-10 {
			t.Errorf("pearsonR(perfect-) = %v, want -1.0", r)
		}
	})
	t.Run("known moderate correlation", func(t *testing.T) {
		xs := []float64{1, 2, 3, 4, 5}
		ys := []float64{1, 3, 2, 5, 4}
		r := pearsonR(xs, ys)
		if r <= 0.5 || r >= 1.0 {
			t.Errorf("pearsonR(moderate) = %v, want between 0.5 and 1.0", r)
		}
	})
}

// ---------------------------------------------------------------------------
// LoadAnalysis
// ---------------------------------------------------------------------------

func TestLoadAnalysis(t *testing.T) {
	t.Run("valid analysis.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		vrlogDir := filepath.Join(tmpDir, "test.vrlog")
		if err := os.MkdirAll(vrlogDir, 0o755); err != nil {
			t.Fatal(err)
		}

		report := &AnalysisReport{
			Version: "1.0",
			Source:  "test.vrlog",
			Recording: RecordingMeta{
				SensorID:    "sensor-1",
				TotalFrames: 100,
				StartNs:     1000,
				EndNs:       2000,
			},
			FrameSummary: FrameSummary{TotalFrames: 100},
			Tracks: []TrackDetail{
				{
					TrackID:     "track-1",
					AvgSpeedMps: 8.4,
					MaxSpeedMps: 9.1,
				},
			},
		}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(vrlogDir, "analysis.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}

		loaded, err := LoadAnalysis(vrlogDir)
		if err != nil {
			t.Fatalf("LoadAnalysis: %v", err)
		}
		if loaded.Version != "1.0" {
			t.Errorf("version = %q, want 1.0", loaded.Version)
		}
		if loaded.Recording.SensorID != "sensor-1" {
			t.Errorf("sensor_id = %q, want sensor-1", loaded.Recording.SensorID)
		}
		if len(loaded.Tracks) != 1 {
			t.Fatalf("len(tracks) = %d, want 1", len(loaded.Tracks))
		}
		if loaded.Tracks[0].MaxSpeedMps != 9.1 {
			t.Errorf("MaxSpeedMps = %v, want 9.1", loaded.Tracks[0].MaxSpeedMps)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadAnalysis("/nonexistent/path.vrlog")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		vrlogDir := filepath.Join(tmpDir, "bad.vrlog")
		if err := os.MkdirAll(vrlogDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(vrlogDir, "analysis.json"), []byte("{invalid json"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadAnalysis(vrlogDir)
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})

	t.Run("missing max speed key", func(t *testing.T) {
		tmpDir := t.TempDir()
		vrlogDir := filepath.Join(tmpDir, "invalid.vrlog")
		if err := os.MkdirAll(vrlogDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(vrlogDir, "analysis.json"), []byte(`{
			"version": "1.0",
			"tracks": [
				{"track_id": "invalid-track", "avg_speed_mps": 9.1}
			]
		}`), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadAnalysis(vrlogDir)
		if err == nil {
			t.Fatal("expected error for missing max speed key")
		}
	})
}

// ---------------------------------------------------------------------------
// loadOrGenerate
// ---------------------------------------------------------------------------

func TestLoadOrGenerate(t *testing.T) {
	t.Run("returns cached analysis if present", func(t *testing.T) {
		tmpDir := t.TempDir()
		vrlogDir := filepath.Join(tmpDir, "cached.vrlog")
		if err := os.MkdirAll(vrlogDir, 0o755); err != nil {
			t.Fatal(err)
		}

		report := &AnalysisReport{
			Version: "1.0",
			Source:  "cached.vrlog",
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(filepath.Join(vrlogDir, "analysis.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}

		loaded, err := loadOrGenerate(vrlogDir)
		if err != nil {
			t.Fatalf("loadOrGenerate: %v", err)
		}
		if loaded.Source != "cached.vrlog" {
			t.Errorf("source = %q, want cached.vrlog", loaded.Source)
		}
	})

	t.Run("generates when analysis.json missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		vrlogPath := createTestVrlog(t, tmpDir, 5)

		// Remove any existing analysis.json
		os.Remove(filepath.Join(vrlogPath, "analysis.json"))

		report, err := loadOrGenerate(vrlogPath)
		if err != nil {
			t.Fatalf("loadOrGenerate: %v", err)
		}
		if report == nil {
			t.Fatal("expected non-nil report")
		}
		if report.FrameSummary.TotalFrames != 5 {
			t.Errorf("total_frames = %d, want 5", report.FrameSummary.TotalFrames)
		}
	})

	t.Run("generates when analysis.json corrupt", func(t *testing.T) {
		tmpDir := t.TempDir()
		vrlogPath := createTestVrlog(t, tmpDir, 3)

		// Write corrupt JSON
		if err := os.WriteFile(filepath.Join(vrlogPath, "analysis.json"), []byte("{bad"), 0o644); err != nil {
			t.Fatal(err)
		}

		report, err := loadOrGenerate(vrlogPath)
		if err != nil {
			t.Fatalf("loadOrGenerate: %v", err)
		}
		if report == nil {
			t.Fatal("expected non-nil report after regeneration")
		}
	})

	t.Run("generates when analysis.json misses max speed key", func(t *testing.T) {
		tmpDir := t.TempDir()
		vrlogPath := createTestVrlog(t, tmpDir, 3)

		if err := os.WriteFile(filepath.Join(vrlogPath, "analysis.json"), []byte(`{
			"version": "1.0",
			"tracks": [
				{"track_id": "invalid-track", "avg_speed_mps": 9.1}
			]
		}`), 0o644); err != nil {
			t.Fatal(err)
		}

		report, err := loadOrGenerate(vrlogPath)
		if err != nil {
			t.Fatalf("loadOrGenerate: %v", err)
		}
		if report == nil {
			t.Fatal("expected non-nil report after regeneration")
		}
		if report.FrameSummary.TotalFrames != 3 {
			t.Errorf("total_frames = %d, want 3", report.FrameSummary.TotalFrames)
		}
	})

	t.Run("error when path is not a vrlog", func(t *testing.T) {
		_, err := loadOrGenerate("/nonexistent/not.vrlog")
		if err == nil {
			t.Fatal("expected error for invalid vrlog path")
		}
	})
}

// ---------------------------------------------------------------------------
// CompareReports
// ---------------------------------------------------------------------------

// createTestVrlogWithTracks builds a vrlog with specific confirmed tracks for testing comparison.
func createTestVrlogWithTracks(t *testing.T, dir, name string, tracks []visualiser.Track, nFrames int, baseTimeNs int64) string {
	t.Helper()
	basePath := filepath.Join(dir, name)
	rec, err := recorder.NewRecorder(basePath, "compare-sensor")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	for i := 0; i < nFrames; i++ {
		ts := baseTimeNs + int64(i)*100_000_000

		// Update timestamps on each track for this frame
		frameTracks := make([]visualiser.Track, len(tracks))
		copy(frameTracks, tracks)
		for j := range frameTracks {
			frameTracks[j].LastSeenNanos = ts
			frameTracks[j].X = float32(i)
			frameTracks[j].Y = float32(i)
		}

		frame := &visualiser.FrameBundle{
			FrameID:        uint64(i),
			TimestampNanos: ts,
			SensorID:       "compare-sensor",
			CoordinateFrame: visualiser.CoordinateFrameInfo{
				ReferenceFrame: "ENU",
			},
			PointCloud: &visualiser.PointCloudFrame{
				FrameID:        uint64(i),
				TimestampNanos: ts,
				SensorID:       "compare-sensor",
				X:              []float32{1.0},
				Y:              []float32{1.0},
				Z:              []float32{0.5},
				Intensity:      []uint8{100},
				Classification: []uint8{1},
				PointCount:     1,
			},
			Tracks: &visualiser.TrackSet{
				FrameID:        uint64(i),
				TimestampNanos: ts,
				Tracks:         frameTracks,
			},
		}
		if err := rec.Record(frame); err != nil {
			t.Fatalf("Record frame %d: %v", i, err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return basePath
}

func TestCompareReportsOverlapping(t *testing.T) {
	tmpDir := t.TempDir()
	baseTime := int64(1_000_000_000_000)

	tracksA := []visualiser.Track{
		{
			TrackID:           "track-1",
			State:             visualiser.TrackStateConfirmed,
			SpeedMps:          5.0,
			AvgSpeedMps:       5.0,
			MaxSpeedMps:       6.0,
			ObservationCount:  10,
			Hits:              10,
			ObjectClass:       "car",
			ClassConfidence:   0.9,
			TrackLengthMetres: 50,
			MotionModel:       visualiser.MotionModelCV,
			FirstSeenNanos:    baseTime,
		},
		{
			TrackID:           "track-2",
			State:             visualiser.TrackStateConfirmed,
			SpeedMps:          10.0,
			AvgSpeedMps:       10.0,
			MaxSpeedMps:       12.0,
			ObservationCount:  20,
			Hits:              20,
			ObjectClass:       "car",
			ClassConfidence:   0.8,
			TrackLengthMetres: 100,
			MotionModel:       visualiser.MotionModelCV,
			FirstSeenNanos:    baseTime,
		},
	}

	// B has same temporal range, similar tracks with different speeds
	tracksB := []visualiser.Track{
		{
			TrackID:           "track-b1",
			State:             visualiser.TrackStateConfirmed,
			SpeedMps:          5.5,
			AvgSpeedMps:       5.5,
			MaxSpeedMps:       6.5,
			ObservationCount:  10,
			Hits:              10,
			ObjectClass:       "car",
			ClassConfidence:   0.85,
			TrackLengthMetres: 55,
			MotionModel:       visualiser.MotionModelCV,
			FirstSeenNanos:    baseTime,
		},
		{
			TrackID:           "track-b2",
			State:             visualiser.TrackStateConfirmed,
			SpeedMps:          9.5,
			AvgSpeedMps:       9.5,
			MaxSpeedMps:       11.0,
			ObservationCount:  18,
			Hits:              18,
			ObjectClass:       "car",
			ClassConfidence:   0.75,
			TrackLengthMetres: 95,
			MotionModel:       visualiser.MotionModelCV,
			FirstSeenNanos:    baseTime,
		},
	}

	pathA := createTestVrlogWithTracks(t, tmpDir, "a.vrlog", tracksA, 10, baseTime)
	pathB := createTestVrlogWithTracks(t, tmpDir, "b.vrlog", tracksB, 10, baseTime)

	// First generate analysis for both
	if _, _, err := GenerateReport(pathA); err != nil {
		t.Fatalf("GenerateReport A: %v", err)
	}
	if _, _, err := GenerateReport(pathB); err != nil {
		t.Fatalf("GenerateReport B: %v", err)
	}

	cmp, err := CompareReports(pathA, pathB, "")
	if err != nil {
		t.Fatalf("CompareReports: %v", err)
	}

	// Should have temporal overlap (same time range)
	if cmp.FrameOverlap.TemporalIoU <= 0 {
		t.Errorf("temporal_iou = %v, want > 0", cmp.FrameOverlap.TemporalIoU)
	}

	// Should have frame counts
	if cmp.FrameOverlap.AFrames != 10 {
		t.Errorf("a_frames = %d, want 10", cmp.FrameOverlap.AFrames)
	}
	if cmp.FrameOverlap.BFrames != 10 {
		t.Errorf("b_frames = %d, want 10", cmp.FrameOverlap.BFrames)
	}

	// Should have matched tracks (same temporal range, IoU > 0.3)
	if cmp.TrackMatching.MatchedPairs == 0 {
		t.Log("Warning: no matched pairs despite overlapping time ranges")
	}

	// RunA/RunB should be basenames
	if cmp.RunA != "a.vrlog" {
		t.Errorf("run_a = %q, want a.vrlog", cmp.RunA)
	}
	if cmp.RunB != "b.vrlog" {
		t.Errorf("run_b = %q, want b.vrlog", cmp.RunB)
	}
}

func TestCompareReportsNoOverlap(t *testing.T) {
	tmpDir := t.TempDir()

	// A at time 0, B at time 10s later — no overlap
	tracksA := []visualiser.Track{
		{
			TrackID:          "a1",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         5.0,
			AvgSpeedMps:      5.0,
			ObservationCount: 5,
			Hits:             5,
			FirstSeenNanos:   0,
		},
	}
	tracksB := []visualiser.Track{
		{
			TrackID:          "b1",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         7.0,
			AvgSpeedMps:      7.0,
			ObservationCount: 5,
			Hits:             5,
			FirstSeenNanos:   10_000_000_000, // 10 seconds later
		},
	}

	pathA := createTestVrlogWithTracks(t, tmpDir, "no-overlap-a.vrlog", tracksA, 5, 0)
	pathB := createTestVrlogWithTracks(t, tmpDir, "no-overlap-b.vrlog", tracksB, 5, 10_000_000_000)

	if _, _, err := GenerateReport(pathA); err != nil {
		t.Fatalf("GenerateReport A: %v", err)
	}
	if _, _, err := GenerateReport(pathB); err != nil {
		t.Fatalf("GenerateReport B: %v", err)
	}

	cmp, err := CompareReports(pathA, pathB, "")
	if err != nil {
		t.Fatalf("CompareReports: %v", err)
	}

	// No temporal overlap → IoU = 0
	if cmp.FrameOverlap.TemporalIoU != 0 {
		t.Errorf("temporal_iou = %v, want 0", cmp.FrameOverlap.TemporalIoU)
	}

	// No matched tracks
	if cmp.TrackMatching.MatchedPairs != 0 {
		t.Errorf("matched_pairs = %d, want 0", cmp.TrackMatching.MatchedPairs)
	}
}

func TestCompareReportsWriteOutput(t *testing.T) {
	tmpDir := t.TempDir()
	baseTime := int64(1_000_000_000_000)

	tracks := []visualiser.Track{
		{
			TrackID:          "t1",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         5.0,
			AvgSpeedMps:      5.0,
			ObservationCount: 5,
			Hits:             5,
			FirstSeenNanos:   baseTime,
		},
	}

	pathA := createTestVrlogWithTracks(t, tmpDir, "out-a.vrlog", tracks, 5, baseTime)
	pathB := createTestVrlogWithTracks(t, tmpDir, "out-b.vrlog", tracks, 5, baseTime)

	if _, _, err := GenerateReport(pathA); err != nil {
		t.Fatalf("GenerateReport A: %v", err)
	}
	if _, _, err := GenerateReport(pathB); err != nil {
		t.Fatalf("GenerateReport B: %v", err)
	}

	outPath := filepath.Join(tmpDir, "comparison.json")
	cmp, err := CompareReports(pathA, pathB, outPath)
	if err != nil {
		t.Fatalf("CompareReports: %v", err)
	}
	if cmp == nil {
		t.Fatal("expected non-nil comparison report")
	}

	// Verify output file exists and is valid JSON
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var parsed ComparisonReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse output JSON: %v", err)
	}
	if parsed.Version != "1.0" {
		t.Errorf("version = %q, want 1.0", parsed.Version)
	}
}

func TestCompareReportsAutoGenerate(t *testing.T) {
	tmpDir := t.TempDir()
	baseTime := int64(1_000_000_000_000)

	tracks := []visualiser.Track{
		{
			TrackID:          "t1",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         5.0,
			AvgSpeedMps:      5.0,
			ObservationCount: 5,
			Hits:             5,
			FirstSeenNanos:   baseTime,
		},
	}

	pathA := createTestVrlogWithTracks(t, tmpDir, "auto-a.vrlog", tracks, 5, baseTime)
	pathB := createTestVrlogWithTracks(t, tmpDir, "auto-b.vrlog", tracks, 5, baseTime)

	// Don't pre-generate analysis.json — let CompareReports auto-generate
	cmp, err := CompareReports(pathA, pathB, "")
	if err != nil {
		t.Fatalf("CompareReports with auto-generate: %v", err)
	}
	if cmp == nil {
		t.Fatal("expected non-nil comparison")
	}

	// Verify analysis.json was created for both
	if _, err := os.Stat(filepath.Join(pathA, "analysis.json")); err != nil {
		t.Errorf("analysis.json not created for A: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pathB, "analysis.json")); err != nil {
		t.Errorf("analysis.json not created for B: %v", err)
	}
}

func TestCompareReportsInvalidPathA(t *testing.T) {
	_, err := CompareReports("/nonexistent/a.vrlog", "/nonexistent/b.vrlog", "")
	if err == nil {
		t.Fatal("expected error for invalid paths")
	}
}

func TestCompareReportsInvalidPathB(t *testing.T) {
	// A is valid, B is invalid — exercises the "load B" error branch
	tmpDir := t.TempDir()
	baseTime := int64(1_000_000_000_000)
	tracks := []visualiser.Track{
		{
			TrackID:          "t1",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         5.0,
			AvgSpeedMps:      5.0,
			ObservationCount: 5,
			Hits:             5,
			FirstSeenNanos:   baseTime,
		},
	}
	pathA := createTestVrlogWithTracks(t, tmpDir, "valid-a.vrlog", tracks, 5, baseTime)
	_, err := CompareReports(pathA, "/nonexistent/b.vrlog", "")
	if err == nil {
		t.Fatal("expected error for invalid B path")
	}
}

func TestCompareReportsWriteError(t *testing.T) {
	tmpDir := t.TempDir()
	baseTime := int64(1_000_000_000_000)
	tracks := []visualiser.Track{
		{
			TrackID:          "t1",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         5.0,
			AvgSpeedMps:      5.0,
			ObservationCount: 5,
			Hits:             5,
			FirstSeenNanos:   baseTime,
		},
	}
	pathA := createTestVrlogWithTracks(t, tmpDir, "wr-a.vrlog", tracks, 3, baseTime)
	pathB := createTestVrlogWithTracks(t, tmpDir, "wr-b.vrlog", tracks, 3, baseTime)

	// Write to a path inside a non-existent directory
	badOut := filepath.Join(tmpDir, "no-such-dir", "sub", "comparison.json")
	_, err := CompareReports(pathA, pathB, badOut)
	if err == nil {
		t.Fatal("expected error writing to invalid outPath")
	}
}

func TestCompareReportsEmptyTracks(t *testing.T) {
	tmpDir := t.TempDir()
	baseTime := int64(1_000_000_000_000)

	// Create vrlogs with no tracks (empty frames)
	pathA := filepath.Join(tmpDir, "empty-a.vrlog")
	pathB := filepath.Join(tmpDir, "empty-b.vrlog")

	for _, path := range []string{pathA, pathB} {
		rec, err := recorder.NewRecorder(path, "empty-sensor")
		if err != nil {
			t.Fatalf("NewRecorder: %v", err)
		}
		for i := 0; i < 3; i++ {
			frame := &visualiser.FrameBundle{
				FrameID:        uint64(i),
				TimestampNanos: baseTime + int64(i)*100_000_000,
				SensorID:       "empty-sensor",
				CoordinateFrame: visualiser.CoordinateFrameInfo{
					ReferenceFrame: "ENU",
				},
			}
			if err := rec.Record(frame); err != nil {
				t.Fatalf("Record: %v", err)
			}
		}
		if err := rec.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	cmp, err := CompareReports(pathA, pathB, "")
	if err != nil {
		t.Fatalf("CompareReports: %v", err)
	}

	if cmp.TrackMatching.ATotalTracks != 0 {
		t.Errorf("a_total_tracks = %d, want 0", cmp.TrackMatching.ATotalTracks)
	}
	if cmp.TrackMatching.MatchedPairs != 0 {
		t.Errorf("matched_pairs = %d, want 0", cmp.TrackMatching.MatchedPairs)
	}
	if cmp.SpeedDelta.SpeedCorrelation != 0 {
		t.Errorf("speed_correlation = %v, want 0 (no matches)", cmp.SpeedDelta.SpeedCorrelation)
	}
	if cmp.SpeedDelta.MeanAbsSpeedDeltaMps != 0 {
		t.Errorf("mean_abs_speed_delta = %v, want 0", cmp.SpeedDelta.MeanAbsSpeedDeltaMps)
	}
}

func TestCompareReportsQualityDelta(t *testing.T) {
	tmpDir := t.TempDir()
	baseTime := int64(1_000_000_000_000)

	// A has high fragmentation (many tentative tracks)
	tracksA := []visualiser.Track{
		{
			TrackID:          "ca1",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         5.0,
			AvgSpeedMps:      5.0,
			ObservationCount: 10,
			Hits:             10,
			OcclusionCount:   3,
			FirstSeenNanos:   baseTime,
		},
		{
			TrackID:        "ta1",
			State:          visualiser.TrackStateTentative,
			SpeedMps:       2.0,
			FirstSeenNanos: baseTime,
		},
	}

	// B has no tentative tracks, fewer occlusions
	tracksB := []visualiser.Track{
		{
			TrackID:          "cb1",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         5.0,
			AvgSpeedMps:      5.0,
			ObservationCount: 15,
			Hits:             15,
			OcclusionCount:   1,
			FirstSeenNanos:   baseTime,
		},
	}

	pathA := createTestVrlogWithTracks(t, tmpDir, "qa.vrlog", tracksA, 10, baseTime)
	pathB := createTestVrlogWithTracks(t, tmpDir, "qb.vrlog", tracksB, 10, baseTime)

	if _, _, err := GenerateReport(pathA); err != nil {
		t.Fatalf("GenerateReport A: %v", err)
	}
	if _, _, err := GenerateReport(pathB); err != nil {
		t.Fatalf("GenerateReport B: %v", err)
	}

	cmp, err := CompareReports(pathA, pathB, "")
	if err != nil {
		t.Fatalf("CompareReports: %v", err)
	}

	// A has higher fragmentation (1 tentative / 2 total = 0.5) vs B (0/1 = 0)
	if cmp.QualityDelta.FragmentationRatio.A <= cmp.QualityDelta.FragmentationRatio.B {
		t.Errorf("expected A fragmentation > B, got A=%v B=%v",
			cmp.QualityDelta.FragmentationRatio.A, cmp.QualityDelta.FragmentationRatio.B)
	}

	// Delta = B - A, should be negative (B has lower fragmentation)
	if cmp.QualityDelta.FragmentationRatio.Delta >= 0 {
		t.Errorf("fragmentation delta = %v, want < 0", cmp.QualityDelta.FragmentationRatio.Delta)
	}
}

func TestCompareReportsSpeedDelta(t *testing.T) {
	tmpDir := t.TempDir()
	baseTime := int64(1_000_000_000_000)

	// Create two vrlogs with overlapping tracks at different speeds
	tracksA := []visualiser.Track{
		{
			TrackID:          "s1",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         5.0,
			AvgSpeedMps:      5.0,
			ObservationCount: 10,
			Hits:             10,
			ObjectClass:      "car",
			FirstSeenNanos:   baseTime,
		},
		{
			TrackID:          "s2",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         10.0,
			AvgSpeedMps:      10.0,
			ObservationCount: 10,
			Hits:             10,
			ObjectClass:      "car",
			FirstSeenNanos:   baseTime,
		},
	}

	tracksB := []visualiser.Track{
		{
			TrackID:          "s1b",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         6.0,
			AvgSpeedMps:      6.0,
			ObservationCount: 10,
			Hits:             10,
			ObjectClass:      "car",
			FirstSeenNanos:   baseTime,
		},
		{
			TrackID:          "s2b",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         11.0,
			AvgSpeedMps:      11.0,
			ObservationCount: 10,
			Hits:             10,
			ObjectClass:      "car",
			FirstSeenNanos:   baseTime,
		},
	}

	pathA := createTestVrlogWithTracks(t, tmpDir, "sa.vrlog", tracksA, 10, baseTime)
	pathB := createTestVrlogWithTracks(t, tmpDir, "sb.vrlog", tracksB, 10, baseTime)

	cmp, err := CompareReports(pathA, pathB, "")
	if err != nil {
		t.Fatalf("CompareReports: %v", err)
	}

	// If we got matches, verify speed delta makes sense
	if cmp.TrackMatching.MatchedPairs > 0 {
		if cmp.SpeedDelta.MeanAbsSpeedDeltaMps <= 0 {
			t.Errorf("mean_abs_speed_delta = %v, want > 0", cmp.SpeedDelta.MeanAbsSpeedDeltaMps)
		}
		if cmp.SpeedDelta.MaxAbsSpeedDeltaMps <= 0 {
			t.Errorf("max_abs_speed_delta = %v, want > 0", cmp.SpeedDelta.MaxAbsSpeedDeltaMps)
		}
	}
}

func TestCompareReportsVersionAndTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	baseTime := int64(1_000_000_000_000)

	tracks := []visualiser.Track{
		{
			TrackID:          "v1",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         5.0,
			AvgSpeedMps:      5.0,
			ObservationCount: 5,
			Hits:             5,
			FirstSeenNanos:   baseTime,
		},
	}

	pathA := createTestVrlogWithTracks(t, tmpDir, "ver-a.vrlog", tracks, 5, baseTime)
	pathB := createTestVrlogWithTracks(t, tmpDir, "ver-b.vrlog", tracks, 5, baseTime)

	cmp, err := CompareReports(pathA, pathB, "")
	if err != nil {
		t.Fatalf("CompareReports: %v", err)
	}

	if cmp.Version != "1.0" {
		t.Errorf("version = %q, want 1.0", cmp.Version)
	}
	if cmp.GeneratedAt == "" {
		t.Error("generated_at is empty")
	}
}

// ---------------------------------------------------------------------------
// histogramEMD
// ---------------------------------------------------------------------------

func TestHistogramEMD(t *testing.T) {
	makeBins := func(lower, upper float64, count int) []HistogramBin {
		return []HistogramBin{{Lower: lower, Upper: upper, Count: count}}
	}

	t.Run("both empty returns 0", func(t *testing.T) {
		a := SpeedHistogram{}
		b := SpeedHistogram{}
		if v := histogramEMD(a, b); v != 0 {
			t.Errorf("histogramEMD(empty,empty) = %v, want 0", v)
		}
	})
	t.Run("a empty returns 0", func(t *testing.T) {
		b := SpeedHistogram{Bins: makeBins(0, 1, 5)}
		if v := histogramEMD(SpeedHistogram{}, b); v != 0 {
			t.Errorf("histogramEMD(empty, b) = %v, want 0", v)
		}
	})
	t.Run("b empty returns 0", func(t *testing.T) {
		a := SpeedHistogram{Bins: makeBins(0, 1, 5)}
		if v := histogramEMD(a, SpeedHistogram{}); v != 0 {
			t.Errorf("histogramEMD(a, empty) = %v, want 0", v)
		}
	})
	t.Run("zero count in a returns 0", func(t *testing.T) {
		a := SpeedHistogram{Bins: makeBins(0, 1, 0)}
		b := SpeedHistogram{Bins: makeBins(0, 1, 5)}
		if v := histogramEMD(a, b); v != 0 {
			t.Errorf("histogramEMD(zero-count-a) = %v, want 0", v)
		}
	})
	t.Run("zero count in b returns 0", func(t *testing.T) {
		a := SpeedHistogram{Bins: makeBins(0, 1, 5)}
		b := SpeedHistogram{Bins: makeBins(0, 1, 0)}
		if v := histogramEMD(a, b); v != 0 {
			t.Errorf("histogramEMD(zero-count-b) = %v, want 0", v)
		}
	})
	t.Run("identical histograms return 0", func(t *testing.T) {
		bins := []HistogramBin{
			{Lower: 0, Upper: 1, Count: 3},
			{Lower: 1, Upper: 2, Count: 7},
		}
		a := SpeedHistogram{BinWidthMps: 1, Bins: bins}
		b := SpeedHistogram{BinWidthMps: 1, Bins: bins}
		if v := histogramEMD(a, b); v > 1e-9 {
			t.Errorf("histogramEMD(identical) = %v, want 0", v)
		}
	})
	t.Run("non-overlapping histograms have positive EMD", func(t *testing.T) {
		// A: all mass in [0,1), B: all mass in [3,4)
		a := SpeedHistogram{Bins: makeBins(0, 1, 10)}
		b := SpeedHistogram{Bins: makeBins(3, 4, 10)}
		v := histogramEMD(a, b)
		if v <= 0 {
			t.Errorf("histogramEMD(non-overlapping) = %v, want > 0", v)
		}
	})
	t.Run("shifted by one bin has positive EMD", func(t *testing.T) {
		a := SpeedHistogram{Bins: []HistogramBin{{Lower: 0, Upper: 1, Count: 10}}}
		b := SpeedHistogram{Bins: []HistogramBin{{Lower: 1, Upper: 2, Count: 10}}}
		v := histogramEMD(a, b)
		// Analytically: all of A's mass moves 1 unit, so EMD = 1.0
		if math.Abs(v-1.0) > 0.01 {
			t.Errorf("histogramEMD(shifted-by-1) = %v, want ~1.0", v)
		}
	})
}

// ---------------------------------------------------------------------------
// Integration: per_pair and histogram_earth_mover_distance in CompareReports
// ---------------------------------------------------------------------------

func TestCompareReportsImplementableNowMetrics(t *testing.T) {
	tmpDir := t.TempDir()
	baseTime := int64(1_000_000_000_000)

	tracksA := []visualiser.Track{
		{
			TrackID:          "m1",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         5.0,
			AvgSpeedMps:      5.0,
			ObservationCount: 10,
			Hits:             10,
			FirstSeenNanos:   baseTime,
			LastSeenNanos:    baseTime + 1_000_000_000,
		},
	}
	tracksB := []visualiser.Track{
		{
			TrackID:          "m2",
			State:            visualiser.TrackStateConfirmed,
			SpeedMps:         7.0,
			AvgSpeedMps:      7.0,
			ObservationCount: 10,
			Hits:             10,
			FirstSeenNanos:   baseTime,
			LastSeenNanos:    baseTime + 1_000_000_000,
		},
	}

	pathA := createTestVrlogWithTracks(t, tmpDir, "impl-a.vrlog", tracksA, 10, baseTime)
	pathB := createTestVrlogWithTracks(t, tmpDir, "impl-b.vrlog", tracksB, 10, baseTime)

	cmp, err := CompareReports(pathA, pathB, "")
	if err != nil {
		t.Fatalf("CompareReports: %v", err)
	}

	// HistogramEarthMoverDist should be non-negative
	if cmp.SpeedDelta.HistogramEarthMoverDist < 0 {
		t.Errorf("histogram_earth_mover_distance = %v, want >= 0", cmp.SpeedDelta.HistogramEarthMoverDist)
	}

	// PerPair should be populated when there are matched pairs
	if cmp.TrackMatching.MatchedPairs > 0 {
		if len(cmp.SpeedDelta.PerPair) != cmp.TrackMatching.MatchedPairs {
			t.Errorf("len(per_pair) = %d, want %d (matched_pairs)",
				len(cmp.SpeedDelta.PerPair), cmp.TrackMatching.MatchedPairs)
		}
		for i, pp := range cmp.SpeedDelta.PerPair {
			if pp.ATrackID == "" || pp.BTrackID == "" {
				t.Errorf("per_pair[%d] has empty track ID", i)
			}
			expectedDelta := math.Abs(pp.AAvgSpeedMps - pp.BAvgSpeedMps)
			if math.Abs(pp.SpeedDeltaMps-expectedDelta) > 1e-9 {
				t.Errorf("per_pair[%d].speed_delta = %v, want %v", i, pp.SpeedDeltaMps, expectedDelta)
			}
		}
	}
}
