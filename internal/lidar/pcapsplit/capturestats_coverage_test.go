package pcapsplit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCaptureBlockRendersSummaryLines(t *testing.T) {
	got := CaptureStats{
		AvgFrameRateHz:    19.8,
		MinFrameRateHz:    18.2,
		MaxFrameRateHz:    20.1,
		MinRPM:            1200,
		MaxRPM:            1200,
		TotalPoints:       123456,
		AvgPointsPerFrame: 28800,
		ForegroundPct:     4.25,
	}.captureBlock()

	for _, want := range []string{
		"Frame rate:  avg 19.8 Hz, min 18.2 Hz, max 20.1 Hz",
		"RPM:         1200–1200",
		"Points:      123456 (28800/frame, 4.2% foreground)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("captureBlock() missing %q\n---\n%s", want, got)
		}
	}
	// With a steady motor there is no change count to report.
	if strings.Contains(got, "changes") {
		t.Errorf("captureBlock() = %q, want no change count when RPMChanges is 0", got)
	}
}

func TestCaptureBlockReportsRPMChanges(t *testing.T) {
	got := CaptureStats{MinRPM: 600, MaxRPM: 1200, RPMChanges: 3}.captureBlock()

	if !strings.Contains(got, "RPM:         600–1200 (3 changes)") {
		t.Errorf("captureBlock() = %q, want the RPM change count", got)
	}
}

func TestComputeFrameRateBucketsSingleFrameTrailingBucket(t *testing.T) {
	// A trailing bucket holding one frame has ~zero elapsed time; dividing by
	// it would report an absurd rate, so the code substitutes the full bucket
	// width instead.
	t0 := time.Unix(0, 0)
	frameTimes := []time.Time{
		t0,
		t0.Add(1 * time.Second),
		// Lands exactly on the bucket boundary, opening a new bucket that
		// then holds only this frame.
		t0.Add(10 * time.Second),
	}

	buckets := computeFrameRateBuckets(frameTimes)

	if len(buckets) != 2 {
		t.Fatalf("got %d buckets, want 2", len(buckets))
	}
	trailing := buckets[1]
	if trailing.Frames != 1 {
		t.Errorf("trailing bucket frames = %d, want 1", trailing.Frames)
	}
	// 1 frame over the substituted 10s window.
	if trailing.Hz != 0.1 {
		t.Errorf("trailing bucket Hz = %v, want 0.1 (1 frame over the 10s bucket)", trailing.Hz)
	}
}

func TestComputeFrameRateBucketsNeedsMoreThanOneFrame(t *testing.T) {
	if got := computeFrameRateBuckets(nil); got != nil {
		t.Errorf("computeFrameRateBuckets(nil) = %v, want nil", got)
	}
	if got := computeFrameRateBuckets([]time.Time{time.Unix(0, 0)}); got != nil {
		t.Errorf("computeFrameRateBuckets(one frame) = %v, want nil", got)
	}
}

func TestFormatMotionTimelineTimestampUnits(t *testing.T) {
	start := time.Date(2026, 8, 14, 12, 30, 45, 500_000_000, time.UTC)
	periods := []MotionPeriod{
		{
			Type:         StaticLabel,
			StartFrame:   0,
			EndFrame:     100,
			StartSecs:    0,
			EndSecs:      5,
			DurationSecs: 5,
			StartTime:    start,
			EndTime:      start.Add(5 * time.Second),
		},
		{
			Type:         MotionLabel,
			StartFrame:   100,
			EndFrame:     300,
			StartSecs:    5,
			EndSecs:      15,
			DurationSecs: 10,
			StartTime:    start.Add(5 * time.Second),
			EndTime:      start.Add(15 * time.Second),
		},
	}

	got := FormatMotionTimeline(periods, "timestamp")

	// Absolute wall-clock boundaries, to millisecond precision.
	if !strings.Contains(got, "12:30:45.500") {
		t.Errorf("output missing the start timestamp\n---\n%s", got)
	}
	if !strings.Contains(got, "12:31:00.500") {
		t.Errorf("output missing the end timestamp\n---\n%s", got)
	}
	// 5s static of 15s total.
	if !strings.Contains(got, "33% static, 67% motion") {
		t.Errorf("output missing the static/motion split\n---\n%s", got)
	}
}

func TestFormatMotionTimelineEmpty(t *testing.T) {
	if got := FormatMotionTimeline(nil, "frames"); got != "" {
		t.Errorf("FormatMotionTimeline(nil) = %q, want empty", got)
	}
}

func TestWriteMotionTimelineJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "motion.json")
	periods := []MotionPeriod{
		{Type: StaticLabel, StartFrame: 0, EndFrame: 100, DurationSecs: 5},
		{Type: MotionLabel, StartFrame: 100, EndFrame: 300, DurationSecs: 10},
	}

	if err := WriteMotionTimelineJSON(path, "/captures/kirk0.pcapng", 15.5, periods); err != nil {
		t.Fatalf("WriteMotionTimelineJSON: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	var got motionTimelineReport
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decoding output: %v", err)
	}
	// Only the base name is recorded, not the operator's directory layout.
	if got.File != "kirk0.pcapng" {
		t.Errorf("File = %q, want %q", got.File, "kirk0.pcapng")
	}
	if got.DurationSecs != 15.5 {
		t.Errorf("DurationSecs = %v, want 15.5", got.DurationSecs)
	}
	if len(got.MotionTimeline) != 2 {
		t.Fatalf("got %d periods, want 2", len(got.MotionTimeline))
	}
	if got.MotionTimeline[0].Type != StaticLabel {
		t.Errorf("first period type = %q, want %q", got.MotionTimeline[0].Type, StaticLabel)
	}
}

func TestWriteMotionTimelineJSONReportsWriteFailure(t *testing.T) {
	// A path inside a nonexistent directory cannot be written.
	err := WriteMotionTimelineJSON(
		filepath.Join(t.TempDir(), "absent", "motion.json"), "f.pcapng", 1, nil)
	if err == nil {
		t.Fatal("WriteMotionTimelineJSON to an unwritable path succeeded, want an error")
	}
}
