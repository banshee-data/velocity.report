//go:build pcap
// +build pcap

package pcapanalyse

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/pcapsplit"
)

func TestAnalysisStats_Methods(t *testing.T) {
	s := &analysisStats{}
	s.AddPacket(100)
	s.AddPacket(200)
	s.AddPoints(50)
	s.AddDropped()
	s.LogStats(true)
	pkts, pts, _ := s.getStats()
	if pkts != 2 || pts != 50 {
		t.Errorf("getStats = %d packets, %d points; want 2, 50", pkts, pts)
	}
}

func TestNewAnalysisFrameBuilder_WithAndWithoutDB(t *testing.T) {
	r := &AnalysisResult{TracksByClass: map[string]int{}}
	fb := newAnalysisFrameBuilder(Config{SensorID: "s"}, r)
	if fb.bgManager == nil || fb.tracker == nil || fb.classifier == nil {
		t.Fatal("builder components not initialised")
	}
	if fb.dbConn != nil {
		t.Error("expected nil dbConn without DBPath")
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	fbDB := newAnalysisFrameBuilder(Config{SensorID: "s", DBPath: dbPath, Benchmark: true}, r)
	if fbDB.dbConn == nil {
		t.Error("expected dbConn with DBPath")
	} else {
		_ = fbDB.dbConn.Close()
	}
	if got := createBackgroundManager("s", nil, nil); got == nil {
		t.Error("createBackgroundManager returned nil")
	}
}

func TestSetMotorSpeed_TracksChanges(t *testing.T) {
	fb := newAnalysisFrameBuilder(Config{SensorID: "s"}, &AnalysisResult{TracksByClass: map[string]int{}})
	fb.SetMotorSpeed(600)
	fb.SetMotorSpeed(600)
	fb.SetMotorSpeed(1200)
	if fb.rpmChanges != 1 {
		t.Errorf("rpmChanges = %d, want 1", fb.rpmChanges)
	}
	if len(fb.rpmValues) != 3 {
		t.Errorf("rpmValues = %d, want 3", len(fb.rpmValues))
	}
}

func TestGetCaptureStats_Synthetic(t *testing.T) {
	r := &AnalysisResult{
		PCAPFile: "x.pcap", TotalPoints: 1000, ForegroundPoints: 100,
		TotalFrames: 10, ConfirmedTracks: 2,
	}
	fb := newAnalysisFrameBuilder(Config{SensorID: "s", Motion: true}, r)
	base := time.Unix(1700000000, 0)
	for i := range 25 {
		ts := base.Add(time.Duration(i) * time.Second)
		fb.frameTimestamps = append(fb.frameTimestamps, ts)
		fb.rpmValues = append(fb.rpmValues, 600)
		fb.motionSamples = append(fb.motionSamples, pcapsplit.FrameSample{T: ts, Moving: i < 5})
	}
	fb.rpmChanges = 1
	stats := fb.getCaptureStats(r)
	if stats.TotalFrames != 10 {
		t.Errorf("TotalFrames = %d, want 10", stats.TotalFrames)
	}
	if stats.MinRPM != 600 || stats.MaxRPM != 600 {
		t.Errorf("RPM = %d-%d, want 600-600", stats.MinRPM, stats.MaxRPM)
	}
	if len(stats.FrameRate10s) == 0 {
		t.Error("expected 10s buckets")
	}
	if len(stats.MotionTimeline) == 0 {
		t.Error("expected motion timeline")
	}
}

func TestAttachMotionTimelineMatchesPCAPSplit(t *testing.T) {
	pcapFile := testCapture(t, 1500)
	config := baseConfig(t, pcapFile)
	result := &AnalysisResult{CaptureStats: &CaptureStats{File: pcapFile}}
	if err := attachMotionTimeline(config, result); err != nil {
		t.Fatal(err)
	}

	splitCfg := pcapsplit.DefaultSplitConfig()
	splitCfg.PCAPFile = pcapFile
	splitCfg.SensorID = config.SensorID
	splitCfg.UDPPort = config.UDPPort
	analysis, err := pcapsplit.Analyse(splitCfg)
	if err != nil {
		t.Fatal(err)
	}
	want := pcapsplit.BuildTimeline(analysis.Samples, splitCfg.TimelineConfig())
	if !reflect.DeepEqual(result.CaptureStats.MotionTimeline, want) {
		t.Fatalf("preview timeline differs from splitter\n got: %#v\nwant: %#v", result.CaptureStats.MotionTimeline, want)
	}
}

func TestAttachMotionTimelineRejectsMissingStats(t *testing.T) {
	if err := attachMotionTimeline(Config{}, nil); err == nil {
		t.Fatal("expected nil result to fail")
	}
	if err := attachMotionTimeline(Config{}, &AnalysisResult{}); err == nil {
		t.Fatal("expected missing capture stats to fail")
	}
	if err := attachMotionTimeline(
		Config{PCAPFile: filepath.Join(t.TempDir(), "missing.pcap"), SensorID: "hesai-pandar40p", UDPPort: testPCAPPort},
		&AnalysisResult{CaptureStats: &CaptureStats{}},
	); err == nil {
		t.Fatal("expected unreadable PCAP to fail")
	}
}

func sampleResult() *AnalysisResult {
	return &AnalysisResult{
		PCAPFile: "cap.pcap", DurationSecs: 60, TotalPackets: 100, TotalPoints: 1000,
		TotalFrames: 10, ForegroundPoints: 100, BackgroundPoints: 900, TotalClusters: 5,
		TotalTracks: 2, ConfirmedTracks: 1,
		TracksByClass: map[string]int{"vehicle": 1, "other": 1},
		Tracks: []*TrackExport{
			{TrackID: "t1", Class: "vehicle", Confidence: 0.9, StartTime: "a", EndTime: "b", DurationSecs: 5, Observations: 10, AvgSpeedMps: 12, MaxSpeedMps: 15},
		},
		SpeedStats:   SpeedStatistics{MinSpeed: 1, MaxSpeed: 15, AvgSpeed: 8, P85Speed: 13},
		CaptureStats: &CaptureStats{File: "cap.pcap", MotionTimeline: []pcapsplit.MotionPeriod{{Type: "static", StartSecs: 0, EndSecs: 60, DurationSecs: 60}}},
	}
}

func TestPrintFunctions(t *testing.T) {
	r := sampleResult()
	captureOutput(t, func() {
		printSummary(r, "seconds")
		printCaptureStats(*r.CaptureStats, "frames")
		printStats10s(CaptureStats{File: "x", FrameRate10s: []FrameRateBucket{{OffsetSecs: 0, Frames: 10, Hz: 10}}})
		printMotionTimeline(r.CaptureStats.MotionTimeline, "timestamp")
	})
	if formatMinSec(125) != "2m 05s" {
		t.Errorf("formatMinSec(125) = %q", formatMinSec(125))
	}
}

func TestExportResults_JSONAndCSV(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{PCAPFile: "cap.pcap", OutputDir: dir, ExportJSON: true, ExportCSV: true}
	captureOutput(t, func() {
		if err := exportResults(cfg, sampleResult()); err != nil {
			t.Errorf("exportResults: %v", err)
		}
	})
	for _, name := range []string{"cap_analysis.json", "cap_tracks.csv"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
}

func TestExportTracksCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.csv")
	tracks := []*TrackExport{{TrackID: "a", Class: "vehicle"}, {TrackID: "b", Class: "other"}}
	if err := exportTracksCSV(path, tracks); err != nil {
		t.Fatalf("exportTracksCSV: %v", err)
	}
	if data, _ := os.ReadFile(path); len(data) == 0 {
		t.Error("empty CSV")
	}
}

func TestExportTrainingData(t *testing.T) {
	dir := t.TempDir()
	frames := []*TrainingFrame{
		{FrameID: 0, Timestamp: time.Unix(1, 0), SensorID: "s", TotalPoints: 100, ForegroundPoints: 10, ForegroundBlob: []byte{1, 2, 3}},
	}
	captureOutput(t, func() {
		if err := exportTrainingData(dir, frames); err != nil {
			t.Errorf("exportTrainingData: %v", err)
		}
	})
	if _, err := os.Stat(filepath.Join(dir, "training_data", "frames_metadata.json")); err != nil {
		t.Errorf("missing metadata: %v", err)
	}
}

func TestPersistToDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "p.db")
	if err := persistToDatabase(dbPath, sampleResult(), nil); err != nil {
		t.Fatalf("persistToDatabase: %v", err)
	}
}

func TestComputeFrameTimeStats(t *testing.T) {
	if got := computeFrameTimeStats(nil); got.Samples != 0 {
		t.Errorf("empty = %+v", got)
	}
	s := computeFrameTimeStats([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	if s.MinMs != 1 || s.MaxMs != 10 || s.Samples != 10 {
		t.Errorf("stats = %+v", s)
	}
}

func TestBenchmarkHelpers(t *testing.T) {
	_ = getSystemInfo()
	if formatBytes(500) != "500 B" {
		t.Errorf("formatBytes(500) = %q", formatBytes(500))
	}
	if formatBytes(2048) != "2.0 KiB" {
		t.Errorf("formatBytes(2048) = %q", formatBytes(2048))
	}
	captureOutput(t, func() {
		printBenchmarkSummary(&PerformanceMetrics{WallClockMs: 100, FrameTimeStats: FrameTimeStats{AvgMs: 1}})
	})
}

func TestHandleBenchmarkOutput_NoBaseline(t *testing.T) {
	cfg := Config{PCAPFile: "cap.pcap", OutputDir: t.TempDir()}
	m := &PerformanceMetrics{WallClockMs: 100, FrameTimeStats: FrameTimeStats{AvgMs: 1, P95Ms: 2}}
	var code int
	captureOutput(t, func() { code = handleBenchmarkOutput(cfg, sampleResult(), m) })
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
}

func TestCompareWithBaseline(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "base.json")
	baseline := BenchmarkResult{Metrics: PerformanceMetrics{
		WallClockMs: 100, FrameTimeStats: FrameTimeStats{AvgMs: 10, P95Ms: 20},
		FramesPerSecond: 100, HeapAllocBytes: 1000, ClusterTimeMs: 10, TrackingTimeMs: 10,
	}}
	data, _ := json.Marshal(baseline)
	_ = os.WriteFile(baselinePath, data, 0o644)

	worse := &PerformanceMetrics{WallClockMs: 200, FrameTimeStats: FrameTimeStats{AvgMs: 20, P95Ms: 40}, FramesPerSecond: 50, HeapAllocBytes: 2000, ClusterTimeMs: 20, TrackingTimeMs: 20}
	cmp, hasReg := compareWithBaseline(baselinePath, worse, 0.10)
	if !hasReg || cmp == nil || len(cmp.Regressions) == 0 {
		t.Errorf("expected regressions, got %+v", cmp)
	}
	captureOutput(t, func() { printComparisonSummary(cmp, 0.10) })

	better := &PerformanceMetrics{WallClockMs: 50, FrameTimeStats: FrameTimeStats{AvgMs: 5, P95Ms: 10}, FramesPerSecond: 200, HeapAllocBytes: 500, ClusterTimeMs: 5, TrackingTimeMs: 5}
	cmp2, hasReg2 := compareWithBaseline(baselinePath, better, 0.10)
	if hasReg2 || cmp2 == nil || len(cmp2.Improvements) == 0 {
		t.Errorf("expected improvements, got %+v", cmp2)
	}
	captureOutput(t, func() { printComparisonSummary(cmp2, 0.10) })

	if cmp3, _ := compareWithBaseline(filepath.Join(dir, "nope.json"), worse, 0.10); cmp3 != nil {
		t.Error("expected nil for missing baseline")
	}
	badPath := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(badPath, []byte("{not json"), 0o644)
	if cmp4, _ := compareWithBaseline(badPath, worse, 0.10); cmp4 != nil {
		t.Error("expected nil for bad JSON")
	}
}
