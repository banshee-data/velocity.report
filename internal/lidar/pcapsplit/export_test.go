package pcapsplit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleReport() Report {
	return Report{
		InputFile: "x.pcap", TotalPackets: 100, TotalFrames: 50, TotalDurationSec: 5,
		Config: DefaultSplitConfig(), Segments: BuildSegments(periodsFixture(), "out"),
	}
}

func TestExportFuncs_Success(t *testing.T) {
	dir := t.TempDir()
	r := sampleReport()
	if err := WriteSegmentsJSON(filepath.Join(dir, "s.json"), r); err != nil {
		t.Fatalf("json: %v", err)
	}
	frames := []FrameMetrics{{FrameID: 0, T: time.Now(), TotalPoints: 100, State: "static", Moving: false}}
	if err := WriteFrameMetricsCSV(filepath.Join(dir, "f.csv"), frames); err != nil {
		t.Fatalf("csv: %v", err)
	}
	if err := WriteSummary(filepath.Join(dir, "sum.txt"), r); err != nil {
		t.Fatalf("summary: %v", err)
	}
	for _, f := range []string{"s.json", "f.csv", "sum.txt"} {
		if _, e := os.Stat(filepath.Join(dir, f)); e != nil {
			t.Errorf("missing %s: %v", f, e)
		}
	}
}

func TestExportFuncs_Errors(t *testing.T) {
	f := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	bad := filepath.Join(f, "sub", "x") // parent is a file -> writes fail
	if err := WriteSegmentsJSON(bad, sampleReport()); err == nil {
		t.Error("WriteSegmentsJSON: expected error")
	}
	if err := WriteFrameMetricsCSV(bad, nil); err == nil {
		t.Error("WriteFrameMetricsCSV: expected error")
	}
	if err := WriteSummary(bad, sampleReport()); err == nil {
		t.Error("WriteSummary: expected error")
	}
}

func TestMetadataPathAndTimelineConfig(t *testing.T) {
	cfg := DefaultSplitConfig()
	cfg.OutputDir = "/tmp/out"
	if got := cfg.MetadataPath("x.json"); got != filepath.Join("/tmp/out", "x.json") {
		t.Errorf("MetadataPath = %q", got)
	}
	tc := cfg.TimelineConfig()
	if tc.SettlingSec != cfg.SettlingSec || tc.MotionTriggerSec != cfg.MotionTriggerSec ||
		tc.MaxMotionGapSec != cfg.MaxMotionGapSec || tc.MinSegmentSec != cfg.MinSegmentSec {
		t.Errorf("TimelineConfig mismatch: %+v vs %+v", tc, cfg)
	}
}

func TestRefine_LeadingStaticBridgedForward(t *testing.T) {
	// Leading short static followed by motion exercises motionAdjacent's i+1 arm.
	got := refineTimeline(buildPeriods(pspec{StaticLabel, 10}, pspec{MotionLabel, 50}), TimelineConfig{MaxMotionGapSec: 30})
	if len(got) != 1 || got[0].Type != MotionLabel {
		t.Fatalf("expected single motion period, got %v", types(got))
	}
}

func TestSecsToDurationZero(t *testing.T) {
	// SettlingSec=0 drives secsToDuration's non-positive branch via BuildTimeline.
	periods := BuildTimeline(
		[]FrameSample{{T: time.Unix(0, 0), Moving: false}, {T: time.Unix(1, 0), Moving: true}},
		TimelineConfig{SettlingSec: 0, MotionTriggerSec: 0},
	)
	if len(periods) == 0 {
		t.Error("expected periods")
	}
}
