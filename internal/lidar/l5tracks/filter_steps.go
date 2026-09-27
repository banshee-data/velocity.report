package l5tracks

import "sort"

// Filter steps: the record a retrospective smoother consumes.
//
// A Rauch–Tung–Striebel pass revises a state x(k|k) using the difference
// between the smoothed and the predicted state one step later. It therefore
// needs, for every step of a track, exactly what the online filter computed:
// the prior x(k|k-1), P(k|k-1) it associated and updated against, the
// posterior x(k|k), P(k|k) it published, and the interval it actually
// predicted across. Recomputing any of them outside the tracker would be a
// second implementation of the prediction step, and it would drift from the
// first the day either changes (the covariance cap, the occlusion inflation,
// the velocity clamp and the capture-gap sub-steps all shape the prior). So
// the tracker records its own numbers, and nothing else.
//
// The recorder is off unless an observer is attached with
// SetFilterStepObserver, and when off every hook is a nil-receiver return:
// the live tracker does no extra work and its behaviour cannot change.
// Attached, it only reads. TestFilterStepObserverDoesNotChangeTracking pins
// that on a multi-track scene, state for state.
//
// What a step says, and what it does not:
//
//   - Prior is the state entering the Kalman update for an associated track.
//     For a track that was not observed this frame, prior and posterior are
//     the same stored numbers: the prediction, after the miss handling's
//     covariance inflation. That makes the inflation part of this step's
//     transition, as the filter used it, and it makes a smoother's revision of
//     an unobserved step exactly zero unless later evidence moves it.
//   - PredictedSecs is the capture time the filter predicted across to reach
//     the prior, summed over sub-steps, after every clamp. It equals the state
//     time difference except where MaxPredictDt clamped a gap, and a smoother
//     must use it rather than the state times: the prior was built with it.
//   - Observed means the update was applied. An association whose update was
//     refused (singular innovation covariance) is UpdateRefused and
//     unobserved, because the filter did not assimilate it.
//   - A track's step is not recorded for the frame in which it is deleted,
//     whatever the reason. That frame carries no evidence (a miss-count or
//     coast-age expiry) or corrupt state (a non-finite reset), and the chain
//     ends at the previous step either way.
//
// The association is recorded as the online tracker made it. A smoother over
// these steps is a fixed-assignment smoother: it can move a state, never the
// observation that supported it. Revising association is a different
// estimator; see the smoother's documentation for the seam.

// FilterMoments is a four-state CV mean and its row-major covariance, exactly
// as the filter stored them.
type FilterMoments struct {
	X, Y, VX, VY float32
	P            [16]float32
}

func momentsOf(track *TrackedObject) FilterMoments {
	return FilterMoments{X: track.X, Y: track.Y, VX: track.VX, VY: track.VY, P: track.P}
}

// FilterObservation is the evidence behind an observed step: the cluster, its
// measured position and acquisition time, and the innovation the filter
// computed against its prior. HasInnovation is false for the observation that
// founded the track, which initialised the state rather than updating it.
type FilterObservation struct {
	ClusterID            int64
	MeasurementUnixNanos int64
	Source               MeasurementSource
	X, Y                 float32
	InnovationX          float32
	InnovationY          float32
	NIS                  float32
	HasInnovation        bool
	// GeometryCovariance is the cluster-shape covariance the online residual
	// retains beside the scalar R the gain used (see FilterResidual).
	GeometryCovariance MeasurementCovariance
}

// FilterStep is one track's filter record for one Update call.
type FilterStep struct {
	TrackID          string
	CreationSequence int64
	// FrameUnixNanos is the capture time passed to Update; StateUnixNanos is
	// the capture time the state refers to (see time_domain.go). They differ
	// only under MeasurementTimePrediction.
	FrameUnixNanos int64
	StateUnixNanos int64
	// PredictedSecs is the interval the filter predicted across from the
	// previous step's state to this step's prior. Zero when Linked is false.
	PredictedSecs float32
	// Linked is false for the first step of a chain: there is no recorded
	// predecessor whose posterior this step's prior was predicted from.
	Linked bool
	// Observed means an associated measurement updated the state this step.
	Observed bool
	// UpdateRefused marks an association the filter declined to assimilate.
	UpdateRefused bool
	// Confirmed is the lifecycle state at the end of the frame. The online
	// state-estimate sink persists exactly the observed, confirmed steps.
	Confirmed   bool
	Prior       FilterMoments
	Posterior   FilterMoments
	Observation FilterObservation
}

// ChainEndReason says why a track's chain of steps ended.
type ChainEndReason string

const (
	// ChainEndDeleted: the tracker deleted the track (misses, capture-time
	// coast age, or a non-finite reset).
	ChainEndDeleted ChainEndReason = "track_deleted"
	// ChainEndRemoved: the track left the tracker without being seen
	// deleted, for example when cleanup ran between two observed frames.
	ChainEndRemoved ChainEndReason = "track_removed"
	// ChainEndReset: Tracker.Reset cleared every track.
	ChainEndReset ChainEndReason = "tracker_reset"
	// ChainEndCapture: the caller ended the input (FixedLagSmoother.Flush).
	// The tracker never reports it; a capture ending is not a track ending.
	ChainEndCapture ChainEndReason = "capture_end"
)

// FilterChainEnd reports that a track will produce no further steps.
type FilterChainEnd struct {
	TrackID          string
	CreationSequence int64
	FrameUnixNanos   int64
	Reason           ChainEndReason
}

// FilterFrame is everything one Update call recorded: a step for every live
// track and an end for every chain that closed. Both are sorted by creation
// sequence, so the stream an observer sees does not depend on map order.
type FilterFrame struct {
	FrameUnixNanos int64
	Steps          []FilterStep
	Ended          []FilterChainEnd
}

// FilterStepObserver receives each frame's filter record. It is called
// synchronously from Update, under the tracker's lock: it must not call back
// into the tracker, and it owns the frame it is given.
type FilterStepObserver interface {
	ObserveFilterFrame(FilterFrame)
}

// SetFilterStepObserver attaches an observer to the tracker's filter record,
// or detaches it with nil. Attach it before the first Update whose steps it
// must see; tracks already alive are recorded from their next step, unlinked.
// This is an offline hook (replay evaluation, a smoother under test). The live
// pipeline does not attach one.
func (t *Tracker) SetFilterStepObserver(observer FilterStepObserver) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if observer == nil {
		t.filterSteps = nil
		return
	}
	t.filterSteps = &filterStepRecorder{
		observer: observer,
		chains:   map[string]int64{},
		pending:  map[string]*pendingFilterStep{},
	}
}

// filterStepRecorder carries one frame's partial records between the hooks.
// Every method is safe on a nil receiver, which is the recorder switched off.
type filterStepRecorder struct {
	observer FilterStepObserver
	// chains maps each open chain's track ID to its creation sequence.
	chains map[string]int64
	// pending holds this frame's prediction interval and prior per track.
	pending map[string]*pendingFilterStep
	// lastFrameNanos stamps chain ends that no Update call carries (Reset).
	lastFrameNanos int64
}

type pendingFilterStep struct {
	predictedSecs float32
	prior         FilterMoments
	hasPrior      bool
}

func (r *filterStepRecorder) pendingFor(trackID string) *pendingFilterStep {
	p := r.pending[trackID]
	if p == nil {
		p = &pendingFilterStep{}
		r.pending[trackID] = p
	}
	return p
}

// notePredict accumulates the interval predict() actually applied, after its
// own clamp. Called once per predict step, so capture-gap sub-steps and the
// measurement-time step add up to the whole interval.
func (r *filterStepRecorder) notePredict(track *TrackedObject, dt float32) {
	if r == nil {
		return
	}
	r.pendingFor(track.TrackID).predictedSecs += dt
}

// notePrior records the state entering the Kalman update.
func (r *filterStepRecorder) notePrior(track *TrackedObject) {
	if r == nil {
		return
	}
	p := r.pendingFor(track.TrackID)
	p.prior = momentsOf(track)
	p.hasPrior = true
}

// endFrame turns the frame's pending records into steps, closes the chains of
// tracks that were deleted or have gone, and hands the frame to the observer.
// Called once at the end of Update, before deleted tracks are cleaned up.
func (r *filterStepRecorder) endFrame(tracks map[string]*TrackedObject, nowNanos int64) {
	if r == nil {
		return
	}
	r.lastFrameNanos = nowNanos
	frame := FilterFrame{FrameUnixNanos: nowNanos}
	for id, track := range tracks {
		sequence, open := r.chains[id]
		if track.TrackState == TrackDeleted {
			if open {
				frame.Ended = append(frame.Ended, FilterChainEnd{TrackID: id, CreationSequence: sequence,
					FrameUnixNanos: nowNanos, Reason: ChainEndDeleted})
				delete(r.chains, id)
			}
			continue
		}
		step := FilterStep{
			TrackID: id, CreationSequence: track.CreationSequence,
			FrameUnixNanos: nowNanos, StateUnixNanos: track.StateUnixNanos,
			Linked: open, Confirmed: track.TrackState == TrackConfirmed,
			Posterior: momentsOf(track),
		}
		pending := r.pending[id]
		if open && pending != nil {
			step.PredictedSecs = pending.predictedSecs
		}
		switch {
		case pending != nil && pending.hasPrior && track.LastResidual.Valid:
			residual := track.LastResidual
			step.Observed = true
			step.Prior = pending.prior
			step.Observation = FilterObservation{
				ClusterID: track.LastClusterID, MeasurementUnixNanos: residual.Measurement.UnixNanos,
				Source: residual.Measurement.Source, X: residual.Measurement.X, Y: residual.Measurement.Y,
				InnovationX: residual.InnovationX, InnovationY: residual.InnovationY, NIS: residual.NIS,
				HasInnovation: true, GeometryCovariance: residual.GeometryCovariance,
			}
		case !open && track.StartUnixNanos == nowNanos:
			// Founded this frame: the state is the measurement, not an update.
			step.Observed = true
			step.Prior = step.Posterior
			step.Observation = FilterObservation{
				ClusterID: track.LastClusterID, MeasurementUnixNanos: track.LastMeasurementUnixNanos,
				Source: track.LastMeasurementSource, X: track.X, Y: track.Y,
			}
		default:
			step.Prior = step.Posterior
			step.UpdateRefused = pending != nil && pending.hasPrior
		}
		if !open {
			r.chains[id] = track.CreationSequence
		}
		frame.Steps = append(frame.Steps, step)
	}
	for id, sequence := range r.chains {
		if _, ok := tracks[id]; !ok {
			frame.Ended = append(frame.Ended, FilterChainEnd{TrackID: id, CreationSequence: sequence,
				FrameUnixNanos: nowNanos, Reason: ChainEndRemoved})
			delete(r.chains, id)
		}
	}
	clear(r.pending)
	sortFilterFrame(&frame)
	r.observer.ObserveFilterFrame(frame)
}

// endAll closes every open chain, for Reset.
func (r *filterStepRecorder) endAll(reason ChainEndReason) {
	if r == nil || len(r.chains) == 0 {
		return
	}
	frame := FilterFrame{FrameUnixNanos: r.lastFrameNanos}
	for id, sequence := range r.chains {
		frame.Ended = append(frame.Ended, FilterChainEnd{TrackID: id, CreationSequence: sequence,
			FrameUnixNanos: r.lastFrameNanos, Reason: reason})
	}
	clear(r.chains)
	clear(r.pending)
	sortFilterFrame(&frame)
	r.observer.ObserveFilterFrame(frame)
}

func sortFilterFrame(frame *FilterFrame) {
	sort.Slice(frame.Steps, func(i, j int) bool {
		a, b := frame.Steps[i], frame.Steps[j]
		if a.CreationSequence != b.CreationSequence {
			return a.CreationSequence < b.CreationSequence
		}
		return a.TrackID < b.TrackID
	})
	sort.Slice(frame.Ended, func(i, j int) bool {
		a, b := frame.Ended[i], frame.Ended[j]
		if a.CreationSequence != b.CreationSequence {
			return a.CreationSequence < b.CreationSequence
		}
		return a.TrackID < b.TrackID
	})
}
