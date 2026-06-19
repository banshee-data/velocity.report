package pcapsplit

import (
	"testing"
	"time"
)

// pspec is a (type, duration) period specification for buildPeriods.
type pspec struct {
	typ string
	dur float64
}

// buildPeriods constructs contiguous periods from (type, durationSecs) pairs.
func buildPeriods(specs ...pspec) []MotionPeriod {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t := base
	out := make([]MotionPeriod, 0, len(specs))
	for _, s := range specs {
		start := t
		t = t.Add(time.Duration(s.dur * float64(time.Second)))
		out = append(out, finalisePeriod(MotionPeriod{
			Type:      s.typ,
			StartTime: start,
			EndTime:   t,
		}, base))
	}
	return out
}

func types(periods []MotionPeriod) []string {
	out := make([]string, len(periods))
	for i, p := range periods {
		out[i] = p.Type
	}
	return out
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRefine_BridgeShortStaticGap(t *testing.T) {
	// A 20 s static gap (< 30 s) flanked by motion is folded into motion.
	periods := buildPeriods(
		pspec{MotionLabel, 50},
		pspec{StaticLabel, 20},
		pspec{MotionLabel, 50},
	)
	got := refineTimeline(periods, TimelineConfig{MaxMotionGapSec: 30})
	if len(got) != 1 || got[0].Type != MotionLabel {
		t.Fatalf("expected single bridged motion period, got %v", types(got))
	}
	if got[0].DurationSecs < 119 {
		t.Errorf("bridged duration=%.1f, want ~120", got[0].DurationSecs)
	}
}

func TestRefine_LongStaticGapNotBridged(t *testing.T) {
	periods := buildPeriods(
		pspec{MotionLabel, 50},
		pspec{StaticLabel, 90}, // > 30 s -> kept
		pspec{MotionLabel, 50},
	)
	got := refineTimeline(periods, TimelineConfig{MaxMotionGapSec: 30})
	if !equalStrs(types(got), []string{MotionLabel, StaticLabel, MotionLabel}) {
		t.Fatalf("long gap should be preserved, got %v", types(got))
	}
}

func TestRefine_MergeShortSegment(t *testing.T) {
	// A 2 s motion blip (< 5 s) between two static runs is merged away.
	periods := buildPeriods(
		pspec{StaticLabel, 100},
		pspec{MotionLabel, 2},
		pspec{StaticLabel, 100},
	)
	got := refineTimeline(periods, TimelineConfig{MinSegmentSec: 5})
	if len(got) != 1 || got[0].Type != StaticLabel {
		t.Fatalf("short motion blip should merge away, got %v", types(got))
	}
}

func TestRefine_NoOpWhenDisabled(t *testing.T) {
	periods := buildPeriods(
		pspec{MotionLabel, 3},
		pspec{StaticLabel, 4},
	)
	got := refineTimeline(periods, TimelineConfig{}) // both knobs 0 -> untouched
	if !equalStrs(types(got), []string{MotionLabel, StaticLabel}) {
		t.Fatalf("disabled refinement changed the timeline: %v", types(got))
	}
}

func TestRefine_LeadingShortSegmentMergesForward(t *testing.T) {
	periods := buildPeriods(
		pspec{MotionLabel, 2}, // leading, short -> merges into following static
		pspec{StaticLabel, 100},
	)
	got := refineTimeline(periods, TimelineConfig{MinSegmentSec: 5})
	if len(got) != 1 || got[0].Type != StaticLabel {
		t.Fatalf("leading short segment should merge forward, got %v", types(got))
	}
	if got[0].StartSecs > 0.001 {
		t.Errorf("merged period should start at 0, got %.3f", got[0].StartSecs)
	}
}
