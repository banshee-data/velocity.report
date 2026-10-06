package perframeeval

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// ScoreOptions are the scoring choices both arms must share.
type ScoreOptions struct {
	Gate l8analytics.MatchGate
	// FrameToleranceNanos is how far a hypothesis point may be moved in time
	// onto a reference frame.
	FrameToleranceNanos int64
	// MaxUnalignedFraction refuses an episode in which more than this share
	// of an arm's points inside the episode could not be put on a frame.
	MaxUnalignedFraction float64
	// Alphas are HOTA's thresholds; nil is the benchmark's sweep.
	Alphas []float64
}

// DefaultScoreOptions is the D2 A/B gate, a 10 ms frame tolerance (a tenth of
// a 10 Hz frame, far inside the half-frame at which alignment would become
// ambiguous), and at most 1% of points unscorable.
func DefaultScoreOptions() ScoreOptions {
	return ScoreOptions{
		Gate:                 l8analytics.FootprintGate(1.0),
		FrameToleranceNanos:  10_000_000,
		MaxUnalignedFraction: 0.01,
	}
}

// Validate refuses options that cannot produce a meaningful score.
func (o ScoreOptions) Validate() error {
	if err := o.Gate.Validate(); err != nil {
		return err
	}
	if o.FrameToleranceNanos < 0 {
		return fmt.Errorf("frame tolerance %d ns is negative", o.FrameToleranceNanos)
	}
	if o.MaxUnalignedFraction < 0 || o.MaxUnalignedFraction > 1 {
		return fmt.Errorf("max unaligned fraction %v is outside [0, 1]", o.MaxUnalignedFraction)
	}
	return nil
}

// EpisodeResult is one arm's score on one episode.
type EpisodeResult struct {
	EpisodeID string                     `json:"episode_id"`
	Reference EpisodeReferenceStats      `json:"reference"`
	Alignment AlignmentStats             `json:"alignment"`
	Metrics   l8analytics.PerFrameResult `json:"metrics"`
}

// ArmResult is one arm scored on every selected episode.
type ArmResult struct {
	Arm                 ArmIdentity           `json:"arm"`
	Reference           ReferenceIdentity     `json:"reference"`
	Gate                l8analytics.MatchGate `json:"gate"`
	FrameToleranceNanos int64                 `json:"frame_tolerance_ns"`
	Episodes            []EpisodeResult       `json:"episodes"`
	// Total pools the episodes: counts summed, ratios recomputed.
	Total l8analytics.PerFrameResult `json:"total"`
}

// ScoreArm scores one arm against every episode of the reference.
func ScoreArm(ref *Reference, hyp Hypothesis, opts ScoreOptions) (ArmResult, error) {
	if err := opts.Validate(); err != nil {
		return ArmResult{}, err
	}
	res := ArmResult{
		Arm: hyp.Identity, Reference: ref.Identity,
		Gate: opts.Gate, FrameToleranceNanos: opts.FrameToleranceNanos,
	}
	var clearParts []l8analytics.TrackMetrics
	var hota []l8analytics.HOTAResult
	var identity []l8analytics.IdentityMetrics
	for _, ep := range ref.Episodes {
		aligned, stats := alignToEpisode(hyp.Series, ep, opts.FrameToleranceNanos)
		if f := stats.UnalignedFraction(); f > opts.MaxUnalignedFraction {
			return ArmResult{}, fmt.Errorf("arm %s, episode %s: %d of %d points inside the episode (%.1f%%) are further than %d ns from every frame, above the %.1f%% limit; "+
				"frame boundaries differ between the runs, or the tolerance is too tight",
				hyp.Identity.Label, ep.EpisodeID, stats.UnalignedPoints,
				stats.AlignedPoints+stats.DuplicatePoints+stats.UnalignedPoints, 100*f,
				opts.FrameToleranceNanos, 100*opts.MaxUnalignedFraction)
		}
		m := l8analytics.EvaluatePerFrame(ep.Series, aligned, opts.Gate, opts.Alphas)
		res.Episodes = append(res.Episodes, EpisodeResult{
			EpisodeID: ep.EpisodeID, Reference: ep.Stats, Alignment: stats, Metrics: m,
		})
		clearParts = append(clearParts, m.CLEARMOT)
		hota = append(hota, m.HOTA)
		identity = append(identity, m.Identity)
	}
	pooled, err := l8analytics.CombineHOTA(hota)
	if err != nil {
		return ArmResult{}, err
	}
	res.Total = l8analytics.PerFrameResult{
		CLEARMOT: l8analytics.CombineTrackMetrics(clearParts),
		HOTA:     pooled,
		Identity: l8analytics.CombineIdentity(identity),
	}
	return res, nil
}
