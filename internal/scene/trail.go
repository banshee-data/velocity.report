package scene

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// The smoothed trail: the exported x, y, vx, vy as the fixed-assignment RTS
// smoother's whole-track estimate rather than the filter's online posterior.
//
// A published scene is an offline replay, so its trail can be drawn from each
// state as revised by everything the tracker saw after it, at no lag. The
// smoother (l5tracks/smoother.go) runs over the recording's filter record,
// the sidecar the replay wrote beside its chunks (recorder.FilterRecordFile),
// with the track-end horizon, and each released state replaces the four
// state components of its track in its frame. It is the whole-track arm of
// the fixed_lag_rts experiment, state for state: the same smoother, the same
// configuration, the same stage name.
//
// Nothing else moves. z is the cluster's lowest return and h its height
// (ground0005's), bx and by are the box model's centre (geom0012's), spd,
// mspd and hdg are the tracker's own derived kinematics, and every identity
// is the online tracker's, since fixed assignment never revises an
// association. The header names the estimator and stage the trail is, so a
// reader can tell a smoothed asset from a live one, and counts what the
// smoother did.

// TrailStageOnline is the stage of a trail drawn from the filter's posterior.
const TrailStageOnline = "online"

// TrailEstimatorOnline names the trail of an export made without
// Options.SmoothedTrail: the online estimator's own posterior, as every
// recording's tracks carry it. The estimator is named by its persisted
// estimator_id; cv_kf_v1 is the one L5 engine with code behind it.
var TrailEstimatorOnline = config.ImplementedL5Engine + "/" + TrailStageOnline

// TrailSmoothingSummary is header.json's account of a smoothed trail.
type TrailSmoothingSummary struct {
	// Final and FixedLag count the states the smoother released by stage:
	// final had every later step of its track; fixed_lag was released
	// early, at the window cap, a barrier or a reference change, with the
	// evidence it had by then (l5tracks.ReleaseReason).
	Final    int `json:"final"`
	FixedLag int `json:"fixed_lag"`
	// Substituted counts the track-frames read whose state the smoother
	// released, and OnlineFallback those it had no state for, which keep
	// the filter's posterior. Both count every rotation read, before the
	// stride.
	Substituted    int `json:"substituted"`
	OnlineFallback int `json:"online_fallback,omitempty"`
	// Smoother is the smoother's own accounting of the record.
	Smoother l5tracks.SmootherStats `json:"smoother"`
}

type trailKey struct {
	id    string
	nanos int64
}

// smoothedTrail is every released state of a recording's filter record,
// keyed by the track and the frame it belongs to.
type smoothedTrail struct {
	estimator string
	states    map[trailKey]l5tracks.FilterMoments
	summary   TrailSmoothingSummary
}

// loadSmoothedTrail runs the whole-track smoother over a recording's filter
// record. A recording without one cannot be smoothed: the posterior alone
// does not say what the filter predicted (filter_steps.go), and a trail
// re-derived from it would be another estimator under this one's name.
func loadSmoothedTrail(vrlogPath string) (*smoothedTrail, error) {
	if !recorder.HasFilterRecord(vrlogPath) {
		return nil, fmt.Errorf("%s has no filter record (%s): the smoothed trail needs a replay recorded with one",
			vrlogPath, recorder.FilterRecordFile)
	}
	smoother, err := l5tracks.NewFixedLagSmoother(l5tracks.SmootherConfig{Lag: l5tracks.LagTrackEnd()})
	if err != nil {
		return nil, err
	}
	trail := &smoothedTrail{states: map[trailKey]l5tracks.FilterMoments{}}
	take := func(states []l5tracks.SmoothedState) {
		for _, s := range states {
			trail.states[trailKey{s.TrackID, s.FrameUnixNanos}] = s.Smoothed
			if s.Stage == l5tracks.RefinementFinal {
				trail.summary.Final++
			} else {
				trail.summary.FixedLag++
			}
		}
	}
	header, err := recorder.ReadFilterRecord(vrlogPath, func(frame l5tracks.FilterFrame) error {
		take(smoother.Observe(frame))
		return nil
	})
	if err != nil {
		return nil, err
	}
	take(smoother.Flush())
	trail.summary.Smoother = smoother.Stats()
	trail.estimator = header.EstimatorID + "+" + l5tracks.SmootherID + "/" + string(l5tracks.RefinementFinal)
	if len(trail.states) == 0 {
		return nil, fmt.Errorf("%s: the filter record released no states; nothing to smooth", vrlogPath)
	}
	return trail, nil
}

// state returns the smoothed x, y, vx, vy for a track in a frame, or the
// online ones when the record has no state for it, and counts which.
func (tr *smoothedTrail) state(trackID string, frameNanos int64, x, y, vx, vy float32) (float32, float32, float32, float32) {
	if tr == nil {
		return x, y, vx, vy
	}
	if m, ok := tr.states[trailKey{trackID, frameNanos}]; ok {
		tr.summary.Substituted++
		return m.X, m.Y, m.VX, m.VY
	}
	tr.summary.OnlineFallback++
	return x, y, vx, vy
}
