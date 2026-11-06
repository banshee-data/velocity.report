package l5tracks

import "math"

// SteadinessSummary is a scoring-window diagnostic, not position accuracy.
// Each sample was observed while its track was already confirmed at update
// entry. Warm-up and tentative updates are excluded; ended tracks are retained.
// Null measures mean no eligible samples, never perfect steadiness.
type SteadinessSummary struct {
	Population             string   `json:"population"`
	HeadingJitterDeg       *float64 `json:"heading_jitter_deg"`
	HeadingJitterSamples   int      `json:"heading_jitter_samples"`
	SpeedJitterMps         *float64 `json:"speed_jitter_mps"`
	SpeedJitterSamples     int      `json:"speed_jitter_samples"`
	CourseAlignmentP50Deg  *float64 `json:"course_alignment_p50_deg"`
	CourseAlignmentP90Deg  *float64 `json:"course_alignment_p90_deg"`
	CourseAlignmentSamples int      `json:"course_alignment_samples"`
}

type steadinessWindow struct {
	headingSumSq, speedSumSq float64
	headingCount, speedCount int
	courseHist               [CourseAlignmentBins]uint64
	courseCount              int
}

func (t *Tracker) observeBaselineSpeedJitter(track *TrackedObject, delta float64) {
	if t.baselineEnabled && track.TrackState == TrackConfirmed {
		t.baselineSteadiness.speedSumSq += delta * delta
		t.baselineSteadiness.speedCount++
	}
}

func (t *Tracker) observeBaselineHeadingJitter(track *TrackedObject, delta float64) {
	if t.baselineEnabled && track.TrackState == TrackConfirmed {
		t.baselineSteadiness.headingSumSq += delta * delta
		t.baselineSteadiness.headingCount++
	}
}

func (t *Tracker) observeBaselineCourse(track *TrackedObject, bin int) {
	if t.baselineEnabled && track.TrackState == TrackConfirmed {
		t.baselineSteadiness.courseHist[bin]++
		t.baselineSteadiness.courseCount++
	}
}

func (s steadinessWindow) summarise() *SteadinessSummary {
	out := &SteadinessSummary{Population: "confirmed_at_update_entry_scoring_window_including_terminated_tracks",
		HeadingJitterSamples: s.headingCount, SpeedJitterSamples: s.speedCount, CourseAlignmentSamples: s.courseCount}
	if s.headingCount > 0 {
		v := math.Sqrt(s.headingSumSq/float64(s.headingCount)) * 180 / math.Pi
		out.HeadingJitterDeg = &v
	}
	if s.speedCount > 0 {
		v := math.Sqrt(s.speedSumSq / float64(s.speedCount))
		out.SpeedJitterMps = &v
	}
	if s.courseCount > 0 {
		p50 := float64(courseHistPercentile(s.courseHist, s.courseCount, 50))
		p90 := float64(courseHistPercentile(s.courseHist, s.courseCount, 90))
		out.CourseAlignmentP50Deg, out.CourseAlignmentP90Deg = &p50, &p90
	}
	return out
}
