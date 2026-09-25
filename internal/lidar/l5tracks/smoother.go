package l5tracks

import (
	"fmt"
	"math"
)

// Retrospective refinement: a bounded fixed-lag Rauch–Tung–Striebel smoother
// for the four-state CV filter (state-estimation plan §10, Phase 5).
//
// The online filter answers "where is the object now, given what has been
// seen so far?". A report wants "where was it then, given everything seen
// since?". The RTS backward pass answers the second from the filter's own
// record, without re-running the filter:
//
//	C(k)   = P(k|k) F(k)ᵀ P(k+1|k)⁻¹
//	x(k|N) = x(k|k) + C(k) [x(k+1|N) - x(k+1|k)]
//	P(k|N) = P(k|k) + C(k) [P(k+1|N) - P(k+1|k)] C(k)ᵀ
//
// with F(k) the CV transition over the interval the filter predicted across
// (FilterStep.PredictedSecs) and every other term the filter's stored prior
// or posterior (see filter_steps.go). Starting the recursion at a later
// step's posterior x(j|j) gives the fixed-lag estimate x(k|j) exactly: the
// states before k are not needed. data/maths/tracking-maths.md §12 carries the
// derivation and the numerical rules below.
//
// Principle 0.2 is the design constraint, and it is testable. A state is
// revised only because an observation after it moved the most probable
// state: when every step after k is unobserved, the stored prior and
// posterior of those steps are the same numbers, the bracket is exactly zero,
// and x(k|N) is x(k|k) to the bit. Each released state therefore carries a
// Revision naming the observations that entered its window after the online
// estimate was published, and a non-zero revision with none is counted as a
// defect (SmootherStats.RevisionsWithoutEvidence), not smoothed over.
//
// What this smoother does not do. It never changes an association: a wrong
// assignment is smoothed into the trajectory, not repaired (plan §10.1, and
// the asynchronous tracking plan §2: fixed assignment is the control
// experiment). It does not model or remove the medoid's viewpoint bias
// (plan §3). It never marks a coasted state observed. And it does not repair
// the filter's time: a transition where MaxPredictDt clamped a gap is smoothed
// with the interval the filter used and flagged ClampedPrediction.
//
// The seam for revisable association is the input. A reassociating worker
// emits its own FilterFrames per hypothesis, re-filtered after changing point
// ownership; the backward pass here is unchanged, and the revision record's
// evidence list becomes the hypothesis's observations rather than the online
// tracker's. That worker belongs after the asynchronous capture boundary and
// is not implemented here.
//
// Numerics. Everything is computed in float64 from the float32 record.
// Covariances are symmetrised on entry and after each backward step. A prior
// P(k+1|k) that is not positive definite (the diagonal-only covariance cap can
// produce one) cannot be inverted: the chain is split there, a barrier, and
// the states before it are released with the evidence they had. A smoothed
// covariance that fails its positive-definiteness check is replaced by the
// filtered P(k|k), which is never smaller in exact arithmetic, and flagged:
// unsupported precision is suppressed rather than invented. A step with any
// non-finite value is refused and splits the chain. None of these paths is
// silent: each has a counter in SmootherStats and a flag on the state.

// RefinementStage is the persisted stage vocabulary of plan §10.1: which
// estimate of a state a record is. It is the stage column of
// lidar_track_estimates. The solid-body contract's two-valued EstimateStage
// (solid_body.go) is the same distinction seen from a live consumer: its
// StageLive is RefinementOnline, and its StageSmoothed is either of the other
// two.
type RefinementStage string

const (
	// RefinementOnline is the filter's own causal estimate.
	RefinementOnline RefinementStage = "online"
	// RefinementFixedLag is a state revised once, with a bounded look-ahead.
	RefinementFixedLag RefinementStage = "fixed_lag"
	// RefinementFinal is a state revised with all of its track's evidence, at
	// the track's close. Reports cite this stage.
	RefinementFinal RefinementStage = "final"
)

// SmootherID versions the smoothing algorithm. It changes when the maths or
// the release rules change, so persisted stages can say which produced them.
const SmootherID = "rts_fixed_assignment_v1"

// smootherMaxFrameRateHz is the fastest frame rate a capture-time lag's
// default window cap must hold without releasing early: the Pandar40P's
// 1,200 RPM mode.
const smootherMaxFrameRateHz = 20

// DefaultTrackEndWindowSteps bounds a full-track window: five minutes at
// 10 Hz. A longer chain releases its oldest states early, as fixed_lag with
// ReleaseWindowCap, rather than growing without bound.
const DefaultTrackEndWindowSteps = 3000

// SmootherLag is how much subsequent evidence a state waits for. Set Frames
// or Secs, not both; neither means the whole track (stage final).
type SmootherLag struct {
	// Frames counts the track's own subsequent filter steps.
	Frames int `json:"frames,omitempty"`
	// Secs is capture time after the state. A state's estimate uses every
	// step up to exactly Secs later, and is released when a step at or beyond
	// that time arrives.
	Secs float64 `json:"secs,omitempty"`
}

// LagFrames is a frame-count lag: the plan's three-frame comparator is
// LagFrames(3).
func LagFrames(n int) SmootherLag { return SmootherLag{Frames: n} }

// LagSeconds is a capture-time lag.
func LagSeconds(secs float64) SmootherLag { return SmootherLag{Secs: secs} }

// LagTrackEnd waits for the track to end: full-track RTS.
func LagTrackEnd() SmootherLag { return SmootherLag{} }

// IsTrackEnd reports whether the lag is the whole track.
func (l SmootherLag) IsTrackEnd() bool { return l.Frames == 0 && l.Secs == 0 }

// Mode names the lag's unit: frames, seconds or track_end.
func (l SmootherLag) Mode() string {
	switch {
	case l.Frames > 0:
		return "frames"
	case l.Secs > 0:
		return "seconds"
	default:
		return "track_end"
	}
}

// Value is the lag in its own unit, zero for track_end.
func (l SmootherLag) Value() float64 {
	if l.Frames > 0 {
		return float64(l.Frames)
	}
	return l.Secs
}

// String is a compact label: 3f, 0.5s or track.
func (l SmootherLag) String() string {
	switch {
	case l.Frames > 0:
		return fmt.Sprintf("%df", l.Frames)
	case l.Secs > 0:
		return fmt.Sprintf("%gs", l.Secs)
	default:
		return "track"
	}
}

// SmootherConfig configures one smoother.
type SmootherConfig struct {
	Lag SmootherLag `json:"lag"`
	// MaxWindowSteps bounds each track's window, which is the smoother's
	// memory bound: at no instant are more than this many steps per live
	// track held. Zero takes the lag's default (WindowCap). A full window
	// releases its oldest state, with the evidence it already holds, before
	// the next step joins; the state is flagged ReleaseWindowCap.
	MaxWindowSteps int `json:"max_window_steps"`
}

// Validate refuses a lag that names two units, a non-finite or negative lag,
// and a cap too small to hold a frame lag.
func (c SmootherConfig) Validate() error {
	lag := c.Lag
	if lag.Frames < 0 || lag.Secs < 0 || math.IsNaN(lag.Secs) || math.IsInf(lag.Secs, 0) {
		return fmt.Errorf("smoother lag must be finite and non-negative: %+v", lag)
	}
	if lag.Frames > 0 && lag.Secs > 0 {
		return fmt.Errorf("smoother lag names both frames and seconds: %+v", lag)
	}
	if c.MaxWindowSteps < 0 {
		return fmt.Errorf("smoother window cap must be non-negative, got %d", c.MaxWindowSteps)
	}
	if c.MaxWindowSteps > 0 && c.MaxWindowSteps < 2 {
		return fmt.Errorf("smoother window cap must hold at least two steps, got %d", c.MaxWindowSteps)
	}
	if lag.Frames > 0 && c.MaxWindowSteps > 0 && c.MaxWindowSteps < lag.Frames+1 {
		return fmt.Errorf("smoother window cap %d cannot hold a %d-frame lag", c.MaxWindowSteps, lag.Frames)
	}
	return nil
}

// WindowCap is the per-track step bound in force: MaxWindowSteps, or the
// lag's default. A frame lag needs the state plus its Frames successors. A
// capture-time lag gets room for every step of a 20 Hz capture across the
// lag, so at the Pandar40P's fastest rotation it never releases early.
func (c SmootherConfig) WindowCap() int {
	if c.MaxWindowSteps > 0 {
		return c.MaxWindowSteps
	}
	switch {
	case c.Lag.Frames > 0:
		return c.Lag.Frames + 1
	case c.Lag.Secs > 0:
		return 1 + int(math.Ceil(c.Lag.Secs*smootherMaxFrameRateHz-1e-9))
	default:
		return DefaultTrackEndWindowSteps
	}
}

// ReleaseReason says why a state left its window.
type ReleaseReason string

const (
	// ReleaseLag: the look-ahead reached the lag.
	ReleaseLag ReleaseReason = "lag"
	// ReleaseWindowCap: the memory bound was reached first.
	ReleaseWindowCap ReleaseReason = "window_cap"
	// ReleaseChainEnd: the track ended, or the input did (see ChainEnd).
	ReleaseChainEnd ReleaseReason = "chain_end"
	// ReleaseBarrier: the chain could not be smoothed across the next step
	// (a non-positive-definite prior, a refused step, capture time running
	// backwards), so the state kept only the evidence before the break.
	ReleaseBarrier ReleaseReason = "barrier"
)

// EvidenceRef identifies one observation that entered a state's window after
// its online estimate was published. Under fixed assignment it is the
// track's own observation at that step.
type EvidenceRef struct {
	FrameUnixNanos       int64   `json:"frame_unix_nanos"`
	StateUnixNanos       int64   `json:"state_unix_nanos"`
	ClusterID            int64   `json:"cluster_id"`
	MeasurementUnixNanos int64   `json:"measurement_unix_nanos"`
	InnovationX          float32 `json:"innovation_x"`
	InnovationY          float32 `json:"innovation_y"`
	NIS                  float32 `json:"nis"`
	HasInnovation        bool    `json:"has_innovation"`
}

// Revision is the audit record of one retrospective change: what the online
// estimate was (SmoothedState.Online), how far the refined one moved from it,
// and the observations that justified the move.
type Revision struct {
	DX, DY, DVX, DVY float64
	// PositionMetres and VelocityMps are the magnitudes of the change.
	PositionMetres float64
	VelocityMps    float64
	// Evidence lists, in capture order, the observed steps after the state
	// whose evidence entered its estimate.
	Evidence []EvidenceRef
}

// StrongestEvidence returns the evidence entry with the largest NIS: the
// observation that most surprised the filter within the state's look-ahead.
// For an impact it is the first frame after the event; see the Phase 8 test.
func (r Revision) StrongestEvidence() (EvidenceRef, bool) {
	best := -1
	for i, e := range r.Evidence {
		if e.HasInnovation && (best < 0 || e.NIS > r.Evidence[best].NIS) {
			best = i
		}
	}
	if best < 0 {
		return EvidenceRef{}, false
	}
	return r.Evidence[best], true
}

// SmoothedState is one retrospectively refined state.
type SmoothedState struct {
	TrackID          string
	CreationSequence int64
	FrameUnixNanos   int64
	StateUnixNanos   int64
	Stage            RefinementStage
	Lag              SmootherLag
	// Observed and Observation are the step's own, unchanged: a coasted
	// state is smoothed as a prediction and stays unobserved.
	Observed    bool
	Confirmed   bool
	Observation FilterObservation
	// Online is the filter's posterior; Smoothed is the refined estimate.
	Online   FilterMoments
	Smoothed FilterMoments
	Revision Revision
	// LookaheadSteps and LookaheadSecs are the subsequent steps, and the
	// capture time they span, whose evidence entered this estimate.
	LookaheadSteps int
	LookaheadSecs  float64
	// ReleasedAtUnixNanos is the capture time of the frame at which this
	// estimate became available: its latency to finality is this minus
	// StateUnixNanos.
	ReleasedAtUnixNanos int64
	Release             ReleaseReason
	// ChainEnd is set when Release is ReleaseChainEnd.
	ChainEnd ChainEndReason
	// LookaheadTruncated marks a state released with less look-ahead than
	// its configuration asks for: the track or the input ended first.
	LookaheadTruncated bool
	// ClampedPrediction marks a look-ahead that crossed a transition where
	// the filter predicted less capture time than elapsed.
	ClampedPrediction bool
	// CovarianceFallback marks a smoothed covariance that failed its
	// positive-definiteness check and was replaced by the filtered one.
	CovarianceFallback bool
	// RevisionWithoutEvidence is a defect marker: the state moved although no
	// observation entered its window. It should never be set.
	RevisionWithoutEvidence bool
}

// FinalityDelaySecs is the capture time from the state to its release.
func (s SmoothedState) FinalityDelaySecs() float64 {
	return float64(s.ReleasedAtUnixNanos-s.StateUnixNanos) / 1e9
}

// SmootherStats counts what the smoother held and every non-ordinary path it
// took. The counters are cumulative; OpenWindows and HeldSteps are current.
type SmootherStats struct {
	Steps                    int64   `json:"steps"`
	Released                 int64   `json:"released"`
	ReleasedByLag            int64   `json:"released_by_lag"`
	ReleasedByWindowCap      int64   `json:"released_by_window_cap"`
	ReleasedAtChainEnd       int64   `json:"released_at_chain_end"`
	ReleasedAtBarrier        int64   `json:"released_at_barrier"`
	LookaheadTruncated       int64   `json:"lookahead_truncated"`
	Barriers                 int64   `json:"barriers"`
	RefusedSteps             int64   `json:"refused_steps"`
	TimeRegressions          int64   `json:"time_regressions"`
	ClampedTransitions       int64   `json:"clamped_transitions"`
	CovarianceFallbacks      int64   `json:"covariance_fallbacks"`
	RevisionsWithoutEvidence int64   `json:"revisions_without_evidence"`
	MaxCovarianceAsymmetry   float64 `json:"max_covariance_asymmetry"`
	OpenWindows              int     `json:"open_windows"`
	HeldSteps                int     `json:"held_steps"`
	PeakHeldSteps            int     `json:"peak_held_steps"`
	MaxWindowSteps           int     `json:"max_window_steps"`
}

// FixedLagSmoother releases each state of each track once, when its lag is
// satisfied, its window is full, or its chain ends. It is not safe for
// concurrent use; the tracker calls its observer serially.
type FixedLagSmoother struct {
	cfg            SmootherConfig
	windowCap      int
	lagNanos       int64
	windows        map[string]*smootherWindow
	stats          SmootherStats
	lastFrameNanos int64
}

type smootherWindow struct {
	trackID  string
	sequence int64
	entries  []windowEntry
}

type windowEntry struct {
	step FilterStep
	// gain is C(k), the RTS gain to the next entry, set when it arrives.
	gain [16]float64
	// clampedNext: the filter predicted less time than elapsed to the next.
	clampedNext bool
}

// NewFixedLagSmoother validates the configuration and returns a smoother.
func NewFixedLagSmoother(cfg SmootherConfig) (*FixedLagSmoother, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &FixedLagSmoother{
		cfg: cfg, windowCap: cfg.WindowCap(),
		lagNanos: int64(math.Round(cfg.Lag.Secs * 1e9)),
		windows:  map[string]*smootherWindow{},
	}, nil
}

// Config returns the smoother's configuration.
func (s *FixedLagSmoother) Config() SmootherConfig { return s.cfg }

// Stats returns a snapshot of the counters.
func (s *FixedLagSmoother) Stats() SmootherStats {
	stats := s.stats
	stats.OpenWindows = len(s.windows)
	return stats
}

// ObserveFilterFrame lets a smoother be attached to a tracker directly when
// its output is not needed (a memory or cost measurement). Most callers use
// Observe, which returns what was released.
func (s *FixedLagSmoother) ObserveFilterFrame(frame FilterFrame) { s.Observe(frame) }

// Observe consumes one frame: chains that ended release every state they
// hold, then each step joins its track's window and releases whatever it
// completes. The result is ordered by creation sequence, then capture time.
func (s *FixedLagSmoother) Observe(frame FilterFrame) []SmoothedState {
	s.lastFrameNanos = frame.FrameUnixNanos
	var out []SmoothedState
	for _, end := range frame.Ended {
		if w := s.windows[end.TrackID]; w != nil {
			out = s.releaseAll(out, w, ReleaseChainEnd, end.Reason, end.FrameUnixNanos)
			s.drop(w)
		}
	}
	for i := range frame.Steps {
		out = s.append(out, frame.Steps[i])
	}
	return out
}

// Flush ends every open chain at the end of the input and releases what they
// hold. The states are final for this input, flagged LookaheadTruncated.
func (s *FixedLagSmoother) Flush() []SmoothedState {
	windows := make([]*smootherWindow, 0, len(s.windows))
	for _, w := range s.windows {
		windows = append(windows, w)
	}
	sortWindows(windows)
	var out []SmoothedState
	for _, w := range windows {
		out = s.releaseAll(out, w, ReleaseChainEnd, ChainEndCapture, s.lastFrameNanos)
		s.drop(w)
	}
	return out
}

func sortWindows(windows []*smootherWindow) {
	for i := 1; i < len(windows); i++ {
		for j := i; j > 0 && windowLess(windows[j], windows[j-1]); j-- {
			windows[j], windows[j-1] = windows[j-1], windows[j]
		}
	}
}

func windowLess(a, b *smootherWindow) bool {
	if a.sequence != b.sequence {
		return a.sequence < b.sequence
	}
	return a.trackID < b.trackID
}

// drop forgets a window whose states have all been released.
func (s *FixedLagSmoother) drop(w *smootherWindow) {
	delete(s.windows, w.trackID)
}

// append adds a step to its track's window, splitting the chain where the
// transition into it cannot be smoothed, then releases what it completes.
func (s *FixedLagSmoother) append(out []SmoothedState, step FilterStep) []SmoothedState {
	w := s.windows[step.TrackID]
	if !s.acceptable(step) {
		s.stats.RefusedSteps++
		if w != nil {
			s.stats.Barriers++
			out = s.releaseAll(out, w, ReleaseBarrier, "", step.FrameUnixNanos)
			s.drop(w)
		}
		return out
	}
	s.stats.Steps++
	if w != nil && len(w.entries) > 0 {
		prev := &w.entries[len(w.entries)-1]
		gain, ok := s.transition(prev, step)
		if !ok {
			s.stats.Barriers++
			out = s.releaseAll(out, w, ReleaseBarrier, "", step.FrameUnixNanos)
			s.drop(w)
			w = nil
		} else {
			prev.gain = gain
		}
	}
	if w == nil {
		w = &smootherWindow{trackID: step.TrackID, sequence: step.CreationSequence}
		s.windows[step.TrackID] = w
	}
	// The memory bound holds at every instant: make room before the step
	// joins, releasing the oldest state with the evidence already held.
	for len(w.entries) >= s.windowCap {
		out = s.releaseOldest(out, w, len(w.entries)-1, ReleaseWindowCap, step.FrameUnixNanos)
	}
	w.entries = append(w.entries, windowEntry{step: step})
	s.stats.HeldSteps++
	if s.stats.HeldSteps > s.stats.PeakHeldSteps {
		s.stats.PeakHeldSteps = s.stats.HeldSteps
	}
	if len(w.entries) > s.stats.MaxWindowSteps {
		s.stats.MaxWindowSteps = len(w.entries)
	}
	return s.releaseReady(out, w, step.FrameUnixNanos)
}

// acceptable refuses a step with any non-finite number, or a posterior
// covariance that is not positive definite once symmetrised.
func (s *FixedLagSmoother) acceptable(step FilterStep) bool {
	if !finiteMoments(step.Prior) || !finiteMoments(step.Posterior) {
		return false
	}
	for _, p := range [][16]float32{step.Prior.P, step.Posterior.P} {
		if a := asymmetry(p); a > s.stats.MaxCovarianceAsymmetry {
			s.stats.MaxCovarianceAsymmetry = a
		}
	}
	_, ok := cholesky4(symmetric64(step.Posterior.P))
	return ok
}

// transition computes C(k) = P(k|k) Fᵀ P(k+1|k)⁻¹ for the step after prev,
// and reports false where the chain cannot be smoothed across it.
func (s *FixedLagSmoother) transition(prev *windowEntry, next FilterStep) ([16]float64, bool) {
	if !next.Linked {
		// The recorder began a new chain for a track this window still holds:
		// an end was missed. Never smooth across it.
		return [16]float64{}, false
	}
	gapNanos := next.StateUnixNanos - prev.step.StateUnixNanos
	if gapNanos < 0 {
		s.stats.TimeRegressions++
		return [16]float64{}, false
	}
	priorChol, ok := cholesky4(symmetric64(next.Prior.P))
	if !ok {
		return [16]float64{}, false
	}
	tau := float64(next.PredictedSecs)
	if float64(gapNanos)/1e9-tau > 1e-3 {
		prev.clampedNext = true
		s.stats.ClampedTransitions++
	}
	// Solve P(k+1|k) X = F P(k|k); then C = Xᵀ, because both covariances are
	// symmetric: (P(k+1|k)⁻¹ F P(k|k))ᵀ = P(k|k) Fᵀ P(k+1|k)⁻¹.
	post := symmetric64(prev.step.Posterior.P)
	x := choleskySolve4(priorChol, cvTransitionTimes(tau, post))
	return transpose64(x), true
}

// releaseReady releases every state whose lag the newest step satisfies.
func (s *FixedLagSmoother) releaseReady(out []SmoothedState, w *smootherWindow, frameNanos int64) []SmoothedState {
	for len(w.entries) > 0 {
		last := len(w.entries) - 1
		oldest := w.entries[0].step.StateUnixNanos
		newest := w.entries[last].step.StateUnixNanos
		switch {
		case s.cfg.Lag.Frames > 0 && last >= s.cfg.Lag.Frames:
			out = s.releaseOldest(out, w, s.cfg.Lag.Frames, ReleaseLag, frameNanos)
		case s.cfg.Lag.Secs > 0 && newest-oldest >= s.lagNanos:
			j := 0
			for j < last && w.entries[j+1].step.StateUnixNanos-oldest <= s.lagNanos {
				j++
			}
			out = s.releaseOldest(out, w, j, ReleaseLag, frameNanos)
		default:
			return out
		}
	}
	return out
}

// releaseOldest releases the window's first state, smoothed with the
// evidence of entries 1..j, and removes it.
func (s *FixedLagSmoother) releaseOldest(out []SmoothedState, w *smootherWindow, j int, reason ReleaseReason, frameNanos int64) []SmoothedState {
	var state SmoothedState
	s.backward(w, 0, j, func(k int, mean [4]float64, cov [16]float64, fallback bool) {
		if k == 0 {
			state = s.stateFor(w, 0, j, mean, cov, fallback)
		}
	})
	state.Release = reason
	state.ReleasedAtUnixNanos = frameNanos
	state.Stage = RefinementFixedLag
	// A cap release comes before the lag, or before the track's end.
	state.LookaheadTruncated = reason == ReleaseWindowCap
	out = s.emit(out, state)
	copy(w.entries, w.entries[1:])
	w.entries = w.entries[:len(w.entries)-1]
	s.stats.HeldSteps--
	return out
}

// releaseAll releases every state in the window with all the evidence it
// holds: one backward pass from its last step.
func (s *FixedLagSmoother) releaseAll(out []SmoothedState, w *smootherWindow, reason ReleaseReason, end ChainEndReason, frameNanos int64) []SmoothedState {
	if len(w.entries) == 0 {
		return out
	}
	last := len(w.entries) - 1
	states := make([]SmoothedState, len(w.entries))
	s.backward(w, 0, last, func(k int, mean [4]float64, cov [16]float64, fallback bool) {
		states[k] = s.stateFor(w, k, last, mean, cov, fallback)
	})
	for k := range states {
		state := states[k]
		state.Release = reason
		state.ChainEnd = end
		state.ReleasedAtUnixNanos = frameNanos
		state.Stage = RefinementFixedLag
		if reason == ReleaseChainEnd {
			state.LookaheadTruncated = end == ChainEndCapture || s.lagUnmet(w, k, last)
			if s.cfg.Lag.IsTrackEnd() {
				state.Stage = RefinementFinal
			}
		} else if reason == ReleaseBarrier {
			state.LookaheadTruncated = true
		}
		out = s.emit(out, state)
	}
	s.stats.HeldSteps -= len(w.entries)
	w.entries = w.entries[:0]
	return out
}

// lagUnmet reports whether state k's look-ahead to j falls short of the lag.
func (s *FixedLagSmoother) lagUnmet(w *smootherWindow, k, j int) bool {
	switch {
	case s.cfg.Lag.Frames > 0:
		return j-k < s.cfg.Lag.Frames
	case s.cfg.Lag.Secs > 0:
		return w.entries[j].step.StateUnixNanos-w.entries[k].step.StateUnixNanos < s.lagNanos
	default:
		return false
	}
}

func (s *FixedLagSmoother) emit(out []SmoothedState, state SmoothedState) []SmoothedState {
	s.stats.Released++
	switch state.Release {
	case ReleaseLag:
		s.stats.ReleasedByLag++
	case ReleaseWindowCap:
		s.stats.ReleasedByWindowCap++
	case ReleaseChainEnd:
		s.stats.ReleasedAtChainEnd++
	case ReleaseBarrier:
		s.stats.ReleasedAtBarrier++
	}
	if state.LookaheadTruncated {
		s.stats.LookaheadTruncated++
	}
	if state.CovarianceFallback {
		s.stats.CovarianceFallbacks++
	}
	if state.RevisionWithoutEvidence {
		s.stats.RevisionsWithoutEvidence++
	}
	return append(out, state)
}

// backward runs the RTS recursion from entry j's posterior down to entry i,
// calling visit for every index from j to i with the smoothed mean and
// covariance. Entry j itself is its own posterior.
func (s *FixedLagSmoother) backward(w *smootherWindow, i, j int, visit func(k int, mean [4]float64, cov [16]float64, fallback bool)) {
	last := w.entries[j].step
	mean := vector64(last.Posterior)
	cov := symmetric64(last.Posterior.P)
	visit(j, mean, cov, false)
	for k := j - 1; k >= i; k-- {
		entry := &w.entries[k]
		next := w.entries[k+1].step
		priorMean := vector64(next.Prior)
		priorCov := symmetric64(next.Prior.P)
		postMean := vector64(entry.step.Posterior)
		postCov := symmetric64(entry.step.Posterior.P)

		var delta [4]float64
		for r := 0; r < 4; r++ {
			delta[r] = mean[r] - priorMean[r]
		}
		for r := 0; r < 4; r++ {
			sum := postMean[r]
			for c := 0; c < 4; c++ {
				sum += entry.gain[r*4+c] * delta[c]
			}
			mean[r] = sum
		}

		var dP [16]float64
		for n := range dP {
			dP[n] = cov[n] - priorCov[n]
		}
		smoothed := multiply64(multiply64(entry.gain, dP), transpose64(entry.gain))
		for n := range smoothed {
			smoothed[n] += postCov[n]
		}
		smoothed = symmetrise64(smoothed)
		fallback := false
		if _, ok := cholesky4(smoothed); !ok || !finite64(smoothed[:]) {
			smoothed = postCov
			fallback = true
		}
		cov = smoothed
		visit(k, mean, cov, fallback)
	}
}

// stateFor assembles the released record for entry k smoothed through j.
func (s *FixedLagSmoother) stateFor(w *smootherWindow, k, j int, mean [4]float64, cov [16]float64, fallback bool) SmoothedState {
	step := w.entries[k].step
	online := step.Posterior
	state := SmoothedState{
		TrackID: step.TrackID, CreationSequence: step.CreationSequence,
		FrameUnixNanos: step.FrameUnixNanos, StateUnixNanos: step.StateUnixNanos,
		Lag: s.cfg.Lag, Observed: step.Observed, Confirmed: step.Confirmed, Observation: step.Observation,
		Online:             online,
		Smoothed:           moments32(mean, cov),
		LookaheadSteps:     j - k,
		LookaheadSecs:      float64(w.entries[j].step.StateUnixNanos-step.StateUnixNanos) / 1e9,
		CovarianceFallback: fallback,
	}
	rev := Revision{
		DX: mean[0] - float64(online.X), DY: mean[1] - float64(online.Y),
		DVX: mean[2] - float64(online.VX), DVY: mean[3] - float64(online.VY),
	}
	rev.PositionMetres = math.Hypot(rev.DX, rev.DY)
	rev.VelocityMps = math.Hypot(rev.DVX, rev.DVY)
	for n := k + 1; n <= j; n++ {
		later := w.entries[n].step
		if !later.Observed {
			continue
		}
		rev.Evidence = append(rev.Evidence, EvidenceRef{
			FrameUnixNanos: later.FrameUnixNanos, StateUnixNanos: later.StateUnixNanos,
			ClusterID: later.Observation.ClusterID, MeasurementUnixNanos: later.Observation.MeasurementUnixNanos,
			InnovationX: later.Observation.InnovationX, InnovationY: later.Observation.InnovationY,
			NIS: later.Observation.NIS, HasInnovation: later.Observation.HasInnovation,
		})
	}
	for n := k; n < j; n++ {
		if w.entries[n].clampedNext {
			state.ClampedPrediction = true
			break
		}
	}
	state.Revision = rev
	state.RevisionWithoutEvidence = len(rev.Evidence) == 0 && (rev.PositionMetres != 0 || rev.VelocityMps != 0)
	return state
}

// --- float64 4×4 helpers ---------------------------------------------------

func finiteMoments(m FilterMoments) bool {
	for _, v := range [4]float32{m.X, m.Y, m.VX, m.VY} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	for _, v := range m.P {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return true
}

func finite64(values []float64) bool {
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func vector64(m FilterMoments) [4]float64 {
	return [4]float64{float64(m.X), float64(m.Y), float64(m.VX), float64(m.VY)}
}

func moments32(mean [4]float64, cov [16]float64) FilterMoments {
	m := FilterMoments{X: float32(mean[0]), Y: float32(mean[1]), VX: float32(mean[2]), VY: float32(mean[3])}
	for n := range cov {
		m.P[n] = float32(cov[n])
	}
	return m
}

func asymmetry(p [16]float32) float64 {
	worst := 0.0
	for r := 0; r < 4; r++ {
		for c := r + 1; c < 4; c++ {
			if d := math.Abs(float64(p[r*4+c]) - float64(p[c*4+r])); d > worst {
				worst = d
			}
		}
	}
	return worst
}

// symmetric64 widens a stored covariance and takes its symmetric part. The
// shipped (I-KH)P update is not a symmetric expression, so a long track's P
// carries float32 asymmetry; the smoother works on ½(P+Pᵀ).
func symmetric64(p [16]float32) [16]float64 {
	var out [16]float64
	for n := range p {
		out[n] = float64(p[n])
	}
	return symmetrise64(out)
}

func symmetrise64(a [16]float64) [16]float64 {
	for r := 0; r < 4; r++ {
		for c := r + 1; c < 4; c++ {
			v := 0.5 * (a[r*4+c] + a[c*4+r])
			a[r*4+c], a[c*4+r] = v, v
		}
	}
	return a
}

// cvTransitionTimes returns F(dt)·A for the CV transition, without forming F.
func cvTransitionTimes(dt float64, a [16]float64) [16]float64 {
	out := a
	for c := 0; c < 4; c++ {
		out[0*4+c] += dt * a[2*4+c]
		out[1*4+c] += dt * a[3*4+c]
	}
	return out
}

func multiply64(a, b [16]float64) [16]float64 {
	var out [16]float64
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			var sum float64
			for k := 0; k < 4; k++ {
				sum += a[r*4+k] * b[k*4+c]
			}
			out[r*4+c] = sum
		}
	}
	return out
}

func transpose64(a [16]float64) [16]float64 {
	var out [16]float64
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			out[r*4+c] = a[c*4+r]
		}
	}
	return out
}

// cholesky4 factors a symmetric matrix as L·Lᵀ and reports false unless it is
// positive definite with margin. A pivot must exceed a small multiple of the
// largest diagonal entry: a matrix that is positive definite only by rounding
// would make its inverse, and the gain built from it, meaningless.
func cholesky4(a [16]float64) ([16]float64, bool) {
	var l [16]float64
	scale := 0.0
	for d := 0; d < 4; d++ {
		scale = math.Max(scale, math.Abs(a[d*4+d]))
	}
	if !(scale > 0) || math.IsInf(scale, 0) {
		return l, false
	}
	floor := scale * 1e-12
	for r := 0; r < 4; r++ {
		for c := 0; c <= r; c++ {
			sum := a[r*4+c]
			for k := 0; k < c; k++ {
				sum -= l[r*4+k] * l[c*4+k]
			}
			if r == c {
				if !(sum > floor) {
					return l, false
				}
				l[r*4+r] = math.Sqrt(sum)
			} else {
				l[r*4+c] = sum / l[c*4+c]
			}
		}
	}
	return l, true
}

// choleskySolve4 solves (L·Lᵀ) X = B for X, column by column.
func choleskySolve4(l, b [16]float64) [16]float64 {
	var x [16]float64
	for col := 0; col < 4; col++ {
		var y [4]float64
		for r := 0; r < 4; r++ {
			sum := b[r*4+col]
			for k := 0; k < r; k++ {
				sum -= l[r*4+k] * y[k]
			}
			y[r] = sum / l[r*4+r]
		}
		for r := 3; r >= 0; r-- {
			sum := y[r]
			for k := r + 1; k < 4; k++ {
				sum -= l[k*4+r] * x[k*4+col]
			}
			x[r*4+col] = sum / l[r*4+r]
		}
	}
	return x
}
