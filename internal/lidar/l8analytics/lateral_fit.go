package l8analytics

import (
	"math"
	"sort"
)

// The label-free anchor-stability measure of the state-estimation plan's
// Section 1.5 and Gate G-GEO-1 criteria 1 and 2: the lateral residual of each
// estimate against a five-point time-domain straight-line fit, on moving
// tracks, and the share of those tracks with any excursion above half a metre.
//
// It is the method of scripts/lidar-jump-candidates.py, five_point_time_xy_fit_v1,
// in Go so an estimate series from any table can be scored the same way. The
// fit includes the centre point and uses future samples, so it is an
// attenuated, non-causal anomaly proxy rather than physical truth: a real lane
// change raises it too, which is why the gate pairs it with a labelled
// manoeuvre set.

// LateralFitMethod names the method, for any record that quotes these figures.
const LateralFitMethod = "five_point_time_xy_fit_v1"

// Thresholds of the plan's measurement.
const (
	// LateralFitMovingMinSpeedMps is the lifetime maximum speed a track needs
	// to count as moving.
	LateralFitMovingMinSpeedMps = 6.0
	// LateralFitLocalMinSpeedMps is the fitted speed a window needs: below it
	// the fitted direction is noise and "lateral" has no meaning.
	LateralFitLocalMinSpeedMps = 2.0
	// LateralFitMaxGapSecs splits a track's series where consecutive samples
	// are further apart than this.
	LateralFitMaxGapSecs = 0.3
	// LateralFitExcursionMetres is the excursion threshold of criterion 2.
	LateralFitExcursionMetres = 0.5
)

// LateralFitTrack is one track's time-ordered positions and the lifetime
// maximum speed that decides whether it is moving.
type LateralFitTrack struct {
	ID          string
	MaxSpeedMps float64
	Points      []SeriesPoint
}

// LateralFitSummary is the measure over a set of tracks.
type LateralFitSummary struct {
	Method string `json:"method"`
	// MovingTracks is how many tracks met the lifetime speed floor, and
	// ScoredTracks how many of those had at least one eligible window.
	MovingTracks int `json:"moving_tracks"`
	ScoredTracks int `json:"scored_tracks"`
	Windows      int `json:"windows"`
	// Residual percentiles, nearest rank, in metres.
	P50Metres float64 `json:"p50_m"`
	P95Metres float64 `json:"p95_m"`
	P99Metres float64 `json:"p99_m"`
	MaxMetres float64 `json:"max_m"`
	// TracksWithExcursion counts scored tracks with any residual above
	// LateralFitExcursionMetres; ExcursionShare is that over ScoredTracks.
	TracksWithExcursion int     `json:"tracks_with_excursion"`
	ExcursionShare      float64 `json:"excursion_share"`
}

// SummariseLateralFit scores tracks by the five-point method. Points must be
// in time order within each track; tracks are scored independently.
func SummariseLateralFit(tracks []LateralFitTrack) LateralFitSummary {
	s := LateralFitSummary{Method: LateralFitMethod}
	var residuals []float64
	for _, track := range tracks {
		if track.MaxSpeedMps < LateralFitMovingMinSpeedMps {
			continue
		}
		s.MovingTracks++
		scored, excursion := false, false
		var window []SeriesPoint
		for _, p := range track.Points {
			if !finite32(p.X) || !finite32(p.Y) {
				window = window[:0]
				continue
			}
			if n := len(window); n > 0 {
				gap := float64(p.TimestampNanos-window[n-1].TimestampNanos) / 1e9
				if !(gap > 0 && gap <= LateralFitMaxGapSecs) {
					window = window[:0]
				}
			}
			window = append(window, p)
			if len(window) > 5 {
				copy(window, window[1:])
				window = window[:5]
			}
			if len(window) != 5 {
				continue
			}
			r, ok := fivePointLateralResidual(window)
			if !ok {
				continue
			}
			scored = true
			residuals = append(residuals, r)
			if r > LateralFitExcursionMetres {
				excursion = true
			}
		}
		if scored {
			s.ScoredTracks++
		}
		if excursion {
			s.TracksWithExcursion++
		}
	}
	s.Windows = len(residuals)
	if len(residuals) > 0 {
		sort.Float64s(residuals)
		s.P50Metres = nearestRank(residuals, 50)
		s.P95Metres = nearestRank(residuals, 95)
		s.P99Metres = nearestRank(residuals, 99)
		s.MaxMetres = residuals[len(residuals)-1]
	}
	if s.ScoredTracks > 0 {
		s.ExcursionShare = float64(s.TracksWithExcursion) / float64(s.ScoredTracks)
	}
	return s
}

// fivePointLateralResidual is the centre point's perpendicular distance from
// the least-squares constant-velocity line through five samples, and false
// when the fitted speed is below LateralFitLocalMinSpeedMps.
func fivePointLateralResidual(w []SeriesPoint) (float64, bool) {
	centre := w[2].TimestampNanos
	var ts [5]float64
	var meanT, meanX, meanY float64
	for i, p := range w {
		ts[i] = float64(p.TimestampNanos-centre) / 1e9
		meanT += ts[i]
		meanX += float64(p.X)
		meanY += float64(p.Y)
	}
	meanT, meanX, meanY = meanT/5, meanX/5, meanY/5
	var denom, sx, sy float64
	for i, p := range w {
		dt := ts[i] - meanT
		denom += dt * dt
		sx += dt * (float64(p.X) - meanX)
		sy += dt * (float64(p.Y) - meanY)
	}
	if denom == 0 {
		return 0, false
	}
	vx, vy := sx/denom, sy/denom
	speed := math.Hypot(vx, vy)
	if speed < LateralFitLocalMinSpeedMps {
		return 0, false
	}
	// The fitted line at the centre's time (t = 0) passes through
	// (meanX - vx*meanT, meanY - vy*meanT).
	dx := float64(w[2].X) - (meanX - vx*meanT)
	dy := float64(w[2].Y) - (meanY - vy*meanT)
	return math.Abs(-vy*dx+vx*dy) / speed, true
}

// nearestRank is the p-th percentile of sorted values by nearest rank.
func nearestRank(sorted []float64, p float64) float64 {
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
