package pcapsplit

import (
	"strings"
	"testing"
	"time"
)

func TestRPMAccumulatorObserve(t *testing.T) {
	var r rpmAccumulator
	// Zero RPM is ignored entirely.
	r.observe(0)
	for _, rpm := range []uint16{600, 600, 602, 598, 600} {
		r.observe(rpm)
	}
	if r.min != 598 || r.max != 602 {
		t.Fatalf("min/max = %d/%d, want 598/602", r.min, r.max)
	}
	// Changes: 600->602, 602->598, 598->600 = 3 (the repeated 600 is not a change).
	if r.changes != 3 {
		t.Fatalf("changes = %d, want 3", r.changes)
	}
	if r.count != 5 {
		t.Fatalf("count = %d, want 5", r.count)
	}
}

func TestComputeCaptureStats(t *testing.T) {
	t0 := time.Unix(1000, 0)
	// 5 frames at 10 Hz (0.1s apart).
	frameTimes := make([]time.Time, 5)
	for i := range frameTimes {
		frameTimes[i] = t0.Add(time.Duration(i) * 100 * time.Millisecond)
	}
	var rpm rpmAccumulator
	rpm.observe(600)
	rpm.observe(600)

	s := computeCaptureStats("cap.pcapng", frameTimes, 1000, 50000, 1500, rpm)
	if s.TotalFrames != 5 || s.TotalPackets != 1000 || s.TotalPoints != 50000 {
		t.Fatalf("counts wrong: %+v", s)
	}
	if want := 0.4; s.DurationSecs < want-1e-9 || s.DurationSecs > want+1e-9 {
		t.Fatalf("duration = %f, want %f", s.DurationSecs, want)
	}
	if want := 10.0; s.AvgFrameRateHz != want || s.MinFrameRateHz != want || s.MaxFrameRateHz != want {
		t.Fatalf("frame rate from RPM wrong: %+v", s)
	}
	if s.AvgPointsPerFrame != 10000 {
		t.Fatalf("avg points/frame = %f, want 10000", s.AvgPointsPerFrame)
	}
	if want := 3.0; s.ForegroundPct != want {
		t.Fatalf("foreground pct = %f, want %f", s.ForegroundPct, want)
	}
}

func TestComputeFrameRateBuckets(t *testing.T) {
	t0 := time.Unix(0, 0)
	// 25 frames over 25 s at 1 Hz → three 10s buckets (10, 10, 5).
	var frameTimes []time.Time
	for i := range 25 {
		frameTimes = append(frameTimes, t0.Add(time.Duration(i)*time.Second))
	}
	buckets := computeFrameRateBuckets(frameTimes)
	if len(buckets) != 3 {
		t.Fatalf("buckets = %d, want 3", len(buckets))
	}
	if buckets[0].Frames != 10 || buckets[1].Frames != 10 || buckets[2].Frames != 5 {
		t.Fatalf("bucket frame counts = %d/%d/%d, want 10/10/5",
			buckets[0].Frames, buckets[1].Frames, buckets[2].Frames)
	}
	if buckets[1].OffsetSecs != 10 {
		t.Fatalf("bucket[1] offset = %f, want 10", buckets[1].OffsetSecs)
	}

	if computeFrameRateBuckets(nil) != nil {
		t.Fatal("expected nil buckets for empty input")
	}
}

func TestFormatStats10s(t *testing.T) {
	s := CaptureStats{
		File: "dir/cap.pcapng",
		FrameRate10s: []FrameRateBucket{
			{OffsetSecs: 0, Frames: 100, Hz: 10.0},
			{OffsetSecs: 70, Frames: 99, Hz: 9.9},
		},
	}
	out := FormatStats10s(s)
	if !strings.Contains(out, "[cap.pcapng] (000:00) frame_rate: 10.0 Hz") {
		t.Fatalf("missing first bucket line:\n%s", out)
	}
	if !strings.Contains(out, "[cap.pcapng] (001:10) frame_rate: 9.9 Hz") {
		t.Fatalf("missing 70s bucket line (mm:ss):\n%s", out)
	}
}

func TestFormatMotionTimelineUnits(t *testing.T) {
	periods := []MotionPeriod{
		{Type: StaticLabel, StartSecs: 0, EndSecs: 60, DurationSecs: 60, StartFrame: 0, EndFrame: 600,
			StartTime: time.Unix(0, 0).UTC(), EndTime: time.Unix(60, 0).UTC()},
		{Type: MotionLabel, StartSecs: 60, EndSecs: 90, DurationSecs: 30, StartFrame: 600, EndFrame: 900,
			StartTime: time.Unix(60, 0).UTC(), EndTime: time.Unix(90, 0).UTC()},
	}
	if got := FormatMotionTimeline(nil, "frames"); got != "" {
		t.Fatalf("empty periods should render nothing, got %q", got)
	}
	frames := FormatMotionTimeline(periods, "frames")
	if !strings.Contains(frames, "frame ") || !strings.Contains(frames, "600 frames") {
		t.Fatalf("frames units wrong:\n%s", frames)
	}
	secs := FormatMotionTimeline(periods, "seconds")
	if !strings.Contains(secs, "0.0s –") {
		t.Fatalf("seconds units wrong:\n%s", secs)
	}
	// 67% static / 33% motion over 90 s.
	if !strings.Contains(secs, "67% static, 33% motion") {
		t.Fatalf("summary line wrong:\n%s", secs)
	}
}

func TestSegmentBoundsUnits(t *testing.T) {
	s := Segment{
		StartSecs: 1.5, EndSecs: 9.5, StartFrame: 15, EndFrame: 95,
		StartTime: time.Unix(1, 0).UTC(), EndTime: time.Unix(9, 0).UTC(),
	}
	if got := segmentBounds(s, "frames"); !strings.Contains(got, "frame      15 →      95") {
		t.Fatalf("frames bounds = %q", got)
	}
	if got := segmentBounds(s, "timestamp"); !strings.Contains(got, "→") {
		t.Fatalf("timestamp bounds = %q", got)
	}
	if got := segmentBounds(s, ""); !strings.Contains(got, "1.5s →") {
		t.Fatalf("default (seconds) bounds = %q", got)
	}
}
