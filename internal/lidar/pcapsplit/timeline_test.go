package pcapsplit

import (
	"testing"
	"time"
)

// seg describes one stretch of a synthetic motion stream.
type seg struct {
	durSecs float64
	moving  bool
}

// makeSamples builds a 10 Hz sample stream from the given segments, starting at
// a fixed epoch so timestamps are deterministic.
func makeSamples(segs ...seg) []FrameSample {
	const hz = 10.0
	step := time.Duration(float64(time.Second) / hz)
	t := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var out []FrameSample
	for _, s := range segs {
		n := int(s.durSecs * hz)
		for range n {
			out = append(out, FrameSample{T: t, Moving: s.moving})
			t = t.Add(step)
		}
	}
	return out
}

// approxEq reports whether got is within tol of want.
func approxEq(got, want, tol float64) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d <= tol
}

const tol = 0.2 // 2 frames at 10 Hz

func TestBuildTimeline_Empty(t *testing.T) {
	if got := BuildTimeline(nil, DefaultTimelineConfig()); got != nil {
		t.Fatalf("expected nil for empty input, got %v", got)
	}
}

func TestBuildTimeline_AllStatic(t *testing.T) {
	periods := BuildTimeline(makeSamples(seg{120, false}), DefaultTimelineConfig())
	if len(periods) != 1 {
		t.Fatalf("expected 1 period, got %d: %+v", len(periods), periods)
	}
	if periods[0].Type != StaticLabel {
		t.Errorf("expected static, got %s", periods[0].Type)
	}
	if !approxEq(periods[0].StartSecs, 0, tol) {
		t.Errorf("start=%.2f, want ~0", periods[0].StartSecs)
	}
}

func TestBuildTimeline_AllMotion(t *testing.T) {
	periods := BuildTimeline(makeSamples(seg{120, true}), DefaultTimelineConfig())
	if len(periods) != 1 || periods[0].Type != MotionLabel {
		t.Fatalf("expected 1 motion period, got %+v", periods)
	}
}

func TestBuildTimeline_TransientMotionSpikeIgnored(t *testing.T) {
	// A 2 s foreground spike (< MotionTriggerSec=5) must not split the static run.
	periods := BuildTimeline(
		makeSamples(seg{50, false}, seg{2, true}, seg{50, false}),
		DefaultTimelineConfig(),
	)
	if len(periods) != 1 || periods[0].Type != StaticLabel {
		t.Fatalf("expected single static period, got %+v", periods)
	}
}

func TestBuildTimeline_SustainedMotionFlips(t *testing.T) {
	// Static then sustained motion: flip dated to the moment motion began (50 s).
	periods := BuildTimeline(
		makeSamples(seg{50, false}, seg{100, true}),
		DefaultTimelineConfig(),
	)
	if len(periods) != 2 {
		t.Fatalf("expected 2 periods, got %+v", periods)
	}
	if periods[0].Type != StaticLabel || periods[1].Type != MotionLabel {
		t.Fatalf("expected static->motion, got %s->%s", periods[0].Type, periods[1].Type)
	}
	if !approxEq(periods[1].StartSecs, 50, tol) {
		t.Errorf("motion start=%.2f, want ~50 (cut at onset, not confirmation)", periods[1].StartSecs)
	}
}

func TestBuildTimeline_ShortStopBridged(t *testing.T) {
	// A 20 s stop (< SettlingSec=60) inside motion stays within the motion period.
	periods := BuildTimeline(
		makeSamples(seg{50, true}, seg{20, false}, seg{50, true}),
		DefaultTimelineConfig(),
	)
	if len(periods) != 1 || periods[0].Type != MotionLabel {
		t.Fatalf("expected single bridged motion period, got %+v", periods)
	}
}

func TestBuildTimeline_SustainedStopFlips(t *testing.T) {
	// An 80 s stop (>= SettlingSec=60) ends the motion period at its onset (50 s).
	periods := BuildTimeline(
		makeSamples(seg{50, true}, seg{80, false}),
		DefaultTimelineConfig(),
	)
	if len(periods) != 2 {
		t.Fatalf("expected 2 periods, got %+v", periods)
	}
	if periods[0].Type != MotionLabel || periods[1].Type != StaticLabel {
		t.Fatalf("expected motion->static, got %s->%s", periods[0].Type, periods[1].Type)
	}
	if !approxEq(periods[1].StartSecs, 50, tol) {
		t.Errorf("static start=%.2f, want ~50", periods[1].StartSecs)
	}
}

func TestBuildTimeline_MotionStaticMotion(t *testing.T) {
	periods := BuildTimeline(
		makeSamples(seg{40, true}, seg{90, false}, seg{40, true}),
		DefaultTimelineConfig(),
	)
	if len(periods) != 3 {
		t.Fatalf("expected 3 periods, got %+v", periods)
	}
	want := []string{MotionLabel, StaticLabel, MotionLabel}
	for i, w := range want {
		if periods[i].Type != w {
			t.Errorf("period %d type=%s, want %s", i, periods[i].Type, w)
		}
	}
	// Second motion flip: static ran 90 s, motion resumes at 130 s.
	if !approxEq(periods[2].StartSecs, 130, tol) {
		t.Errorf("final motion start=%.2f, want ~130", periods[2].StartSecs)
	}
}

func TestBuildTimeline_PeriodsContiguousAndOrdered(t *testing.T) {
	periods := BuildTimeline(
		makeSamples(seg{40, true}, seg{90, false}, seg{40, true}),
		DefaultTimelineConfig(),
	)
	for i := range periods {
		if periods[i].EndSecs < periods[i].StartSecs {
			t.Errorf("period %d: end %.2f before start %.2f", i, periods[i].EndSecs, periods[i].StartSecs)
		}
		if !approxEq(periods[i].DurationSecs, periods[i].EndSecs-periods[i].StartSecs, 1e-6) {
			t.Errorf("period %d: duration %.2f != end-start", i, periods[i].DurationSecs)
		}
		if i > 0 && !approxEq(periods[i].StartSecs, periods[i-1].EndSecs, tol) {
			t.Errorf("gap/overlap between period %d end %.2f and period %d start %.2f",
				i-1, periods[i-1].EndSecs, i, periods[i].StartSecs)
		}
	}
}
