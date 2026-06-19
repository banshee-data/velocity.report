package pcapsplit

import "time"

// refineTimeline applies the optional post-processing knobs to the raw
// hysteresis periods: bridging short static gaps into motion, then merging
// sub-minimum segments into a neighbour. Each step is skipped when its
// threshold is <= 0, so the default (raw hysteresis) view is unchanged.
func refineTimeline(periods []MotionPeriod, cfg TimelineConfig) []MotionPeriod {
	if len(periods) == 0 {
		return periods
	}
	t0 := periods[0].StartTime

	if cfg.MaxMotionGapSec > 0 {
		for i := range periods {
			if periods[i].Type == StaticLabel &&
				periods[i].DurationSecs < cfg.MaxMotionGapSec &&
				motionAdjacent(periods, i) {
				periods[i].Type = MotionLabel
			}
		}
		periods = coalesce(periods, t0)
	}

	if cfg.MinSegmentSec > 0 {
		periods = mergeShort(periods, cfg.MinSegmentSec, t0)
		periods = coalesce(periods, t0)
	}
	return periods
}

// motionAdjacent reports whether period i has a motion period on either side.
func motionAdjacent(periods []MotionPeriod, i int) bool {
	if i > 0 && periods[i-1].Type == MotionLabel {
		return true
	}
	if i+1 < len(periods) && periods[i+1].Type == MotionLabel {
		return true
	}
	return false
}

// coalesce merges adjacent periods that share a type and recomputes their
// relative-second fields against t0.
func coalesce(periods []MotionPeriod, t0 time.Time) []MotionPeriod {
	if len(periods) == 0 {
		return periods
	}
	out := make([]MotionPeriod, 0, len(periods))
	cur := periods[0]
	for _, p := range periods[1:] {
		if p.Type == cur.Type {
			cur.EndTime = p.EndTime
			continue
		}
		out = append(out, finalisePeriod(cur, t0))
		cur = p
	}
	return append(out, finalisePeriod(cur, t0))
}

// mergeShort folds periods shorter than minSec into the previous kept period;
// a leading short period is folded into the one that follows it.
func mergeShort(periods []MotionPeriod, minSec float64, t0 time.Time) []MotionPeriod {
	if len(periods) <= 1 {
		return periods
	}
	out := make([]MotionPeriod, 0, len(periods))
	for _, p := range periods {
		if p.DurationSecs < minSec && len(out) > 0 {
			last := &out[len(out)-1]
			last.EndTime = p.EndTime
			*last = finalisePeriod(*last, t0)
			continue
		}
		out = append(out, p)
	}
	if len(out) > 1 && out[0].DurationSecs < minSec {
		out[1].StartTime = out[0].StartTime
		out[1] = finalisePeriod(out[1], t0)
		out = out[1:]
	}
	return out
}

// finalisePeriod recomputes StartSecs/EndSecs/DurationSecs from the absolute
// times relative to t0.
func finalisePeriod(p MotionPeriod, t0 time.Time) MotionPeriod {
	p.StartSecs = p.StartTime.Sub(t0).Seconds()
	p.EndSecs = p.EndTime.Sub(t0).Seconds()
	p.DurationSecs = p.EndSecs - p.StartSecs
	return p
}
