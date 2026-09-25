package l5tracks

// Pre-gate innovation statistics and gate-rejection evidence, for calibrating
// the adaptive measurement noise and testing it against G-UNC-1.
//
// Section 8.3 of the state-estimation plan asks for NIS over eligible
// observations recorded before the innovation gate, with eligibility and
// association provenance defined before calibration. Accepted-only NIS cannot
// serve: the gate has removed the tail the chi-squared test is about. But NIS
// over every candidate pairing is no better, because most candidates in a
// busy frame belong to some other object and test association, not noise.
//
// Eligibility is therefore declared here, independent of the covariance being
// judged: a confirmed track's pairing with a cluster is eligible when that
// cluster is the only one physically plausible for the track (inside
// MaxPositionJumpMetres, below MaxReasonableSpeedMps, not a fragment the guard
// forbids), and the track is the only active track for which the cluster is
// plausible. Membership turns on Euclidean distance and the tracker's own
// plausibility rules, never on S, so a noise model cannot select the
// observations that make it pass. The price is stated: ambiguous scenes are
// excluded, and the Euclidean bound (5 m by default, many sigma) truncates the
// far tail lightly rather than not at all.
//
// Everything here runs only inside a calibration window opened by
// BeginUncertaintyCalibration. It reads tracker state and never writes it, so
// the estimate is identical with the window open or closed.
//
// Gate-rejection evidence. When the gate forbids an eligible pairing, the
// cluster was either a spurious measurement or a genuine manoeuvre the
// constant-velocity prediction could not follow. Later frames tell them apart
// by which hypothesis they continue. Each rejection opens an event holding two
// predictions for the frames that follow: the track's own (it coasts on, or is
// updated along, the path it was on) and the rejected measurement carried
// forward at the track's velocity (the object really was where that cluster
// said). On the first later frame whose best-explained cluster is at most half
// as far from one prediction as from the other, the event closes: persistent
// when the rejected measurement's continuation wins, which is what a real turn
// or brake does, transient when the track's own prediction wins, which is what
// an isolated spurious return does. Neither within
// GateRejectionLookaheadFrames association frames, or the track ending first,
// closes it as unresolved. NIS is deliberately not the test: after a miss the
// track's covariance is inflated, so a manoeuvre's next cluster can sit well
// inside the 99% bound while still departing from the old path.
//
// This is label-free evidence, not a verdict; G-UNC-1's manoeuvre criterion is
// scored against a labelled manoeuvre set.

import (
	"math"
	"sort"
)

const (
	// ChiSquared1DOF95 is the 95th percentile of chi-squared with one degree
	// of freedom, the bound for a one-dimensional normalised innovation.
	ChiSquared1DOF95 = 3.841459
	// ChiSquared2DOF99 is the 99th percentile with two degrees of freedom. An
	// innovation inside it is an ordinary observation of the prediction.
	ChiSquared2DOF99 = 9.210340
	// MaxUncertaintySamples bounds the calibration window's sample list. A
	// sample is under a hundred bytes, so this is about 100 MB at most; a
	// 34-minute corpus case produces of the order of 1e5. Samples past the cap
	// are counted, never silently lost.
	MaxUncertaintySamples = 1 << 20
	// GateRejectionLookaheadFrames is how many association frames a gate
	// rejection event stays open before it closes as unresolved.
	GateRejectionLookaheadFrames = 3
	// gateRejectionDecisiveRatio is how much better one hypothesis must
	// explain a later cluster than the other for the frame to decide: the
	// winner's distance at most this fraction of the loser's.
	gateRejectionDecisiveRatio = 0.5
)

// GateRejectionOutcome names how a gate-rejection event closed.
type GateRejectionOutcome string

const (
	// GateRejectionPersistent: a later cluster continued the rejected
	// measurement's path rather than the track's prediction. The departure
	// recurred, which is what a real turn or brake does.
	GateRejectionPersistent GateRejectionOutcome = "persistent"
	// GateRejectionTransient: a later cluster continued the track's own
	// prediction. The rejected measurement did not recur, which is what a
	// spurious one does.
	GateRejectionTransient GateRejectionOutcome = "transient"
	// GateRejectionUnresolved: no later frame decided within the look-ahead,
	// or the track ended first.
	GateRejectionUnresolved GateRejectionOutcome = "unresolved"
)

// UncertaintySample is one eligible pre-gate pairing, decomposed along and
// across the sensor's line of sight. It carries everything the calibration fit
// and the G-UNC-1 checks need, and no track identity: the frame time and the
// measured position are the join keys a labelled scorer matches a reference
// object on, as the per-frame harness does.
type UncertaintySample struct {
	FrameUnixNanos int64             `json:"frame_unix_nanos"`
	X              float32           `json:"x"`
	Y              float32           `json:"y"`
	Source         MeasurementSource `json:"source"`
	// Rank is the measurement dimension m. Every online position measurement
	// is two-dimensional; a rank-one face measurement would carry 1.
	Rank        int     `json:"rank"`
	RangeMetres float32 `json:"range_m"`
	Support     int     `json:"support"`
	// AspectRad is the sensor's bearing in the body frame; see
	// MeasurementGeometry.
	AspectRad float32 `json:"aspect_rad"`
	SpeedMps  float32 `json:"speed_mps"`
	// Misses is the track's consecutive missed frames before this one. A
	// coasting track's predicted covariance carries OcclusionCovInflation, an
	// allowance R must not be fitted to absorb.
	Misses int `json:"misses"`
	// Innovation, measurement minus prediction, projected on the axes.
	InnovRadial     float32 `json:"innov_radial"`
	InnovTangential float32 `json:"innov_tangential"`
	// PredRadial and PredTangential are the predicted position covariance
	// HPHᵀ projected on each axis.
	PredRadial     float32 `json:"pred_radial"`
	PredTangential float32 `json:"pred_tangential"`
	// NoiseRadial and NoiseTangential are the R the filter used, projected.
	NoiseRadial     float32 `json:"noise_radial"`
	NoiseTangential float32 `json:"noise_tangential"`
	// PhysRadial and PhysTangential are the Section 8.1 physics terms for this
	// geometry, recorded whichever model ran, so a calibration can be fitted
	// from a replay of the shipped filter.
	PhysRadial     float32 `json:"phys_radial"`
	PhysTangential float32 `json:"phys_tangential"`
	// NIS is the joint squared Mahalanobis distance under the S the gate used.
	NIS float32 `json:"nis"`
	// Gated is whether the gate forbade the pairing; Assigned whether the
	// assignment accepted it.
	Gated    bool `json:"gated"`
	Assigned bool `json:"assigned"`
}

// UncertaintyWindow is what a calibration window recorded.
type UncertaintyWindow struct {
	Samples []UncertaintySample
	// SamplesDropped counts samples past MaxUncertaintySamples.
	SamplesDropped int
	PreGate        []PreGateBandSummary
}

// pendingGateRejection is an open event: the rejected measurement, the
// track's velocity when it was rejected, and the frame time, from which the
// rejected hypothesis is carried forward.
type pendingGateRejection struct {
	zX, zY    float32
	vX, vY    float32
	atNanos   int64
	band      int
	remaining int
}

type preGateCandidate struct {
	cluster     int
	dx, dy      float32
	d2          float32
	measurement PositionMeasurement
	noise       NoiseEvaluation
}

type preGateTrack struct {
	id         string
	track      *TrackedObject
	speed      float32
	band       int
	confirmed  bool
	candidates []preGateCandidate
}

// preGateFrame is one frame's pre-gate evaluation, taken after prediction and
// before association.
type preGateFrame struct {
	tracks []preGateTrack
	// claims[ci] is how many active tracks cluster ci is plausible for.
	claims []int
}

// BeginUncertaintyCalibration opens a calibration window: from the next Update
// the tracker records pre-gate samples, the pre-gate band set and
// gate-rejection events. It clears anything an earlier window recorded. It
// changes no estimate.
func (t *Tracker) BeginUncertaintyCalibration() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calibrationEnabled = true
	t.preGate = PreGateBands{}
	t.uncertaintySamples = nil
	t.uncertaintySamplesDropped = 0
	t.uncertaintySampleCap = MaxUncertaintySamples
	t.pendingRejections = map[string]*pendingGateRejection{}
}

// UncertaintyWindow returns a copy of what the open window has recorded. It is
// empty when no window was opened.
func (t *Tracker) UncertaintyWindow() UncertaintyWindow {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if !t.calibrationEnabled {
		return UncertaintyWindow{}
	}
	return UncertaintyWindow{
		Samples:        append([]UncertaintySample(nil), t.uncertaintySamples...),
		SamplesDropped: t.uncertaintySamplesDropped,
		PreGate:        t.preGate.Summarise(t.openRejections()),
	}
}

// openRejections counts still-open events per band. Caller holds t.mu.
func (t *Tracker) openRejections() [ResidualBandCount]int {
	var open [ResidualBandCount]int
	for _, p := range t.pendingRejections {
		open[p.band]++
	}
	return open
}

// sortedActiveTrackIDs is associate()'s ordering: creation sequence, then ID.
func (t *Tracker) sortedActiveTrackIDs() []string {
	ids := make([]string, 0, len(t.Tracks))
	for id, track := range t.Tracks {
		if track.TrackState != TrackDeleted {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := t.Tracks[ids[i]], t.Tracks[ids[j]]
		if a.CreationSequence != b.CreationSequence {
			return a.CreationSequence < b.CreationSequence
		}
		return ids[i] < ids[j]
	})
	return ids
}

// observePreGate evaluates every physically plausible pairing of an active
// track with a cluster, under the same measurement, plausibility rules and S
// the gate uses. Caller holds t.mu.
func (t *Tracker) observePreGate(clusters []WorldCluster, dt float32) *preGateFrame {
	frame := &preGateFrame{claims: make([]int, len(clusters))}
	for _, id := range t.sortedActiveTrackIDs() {
		track := t.Tracks[id]
		speed := float32(math.Hypot(float64(track.VX), float64(track.VY)))
		pt := preGateTrack{id: id, track: track, speed: speed, band: residualBand(speed),
			confirmed: track.TrackState == TrackConfirmed}
		for ci := range clusters {
			if c, ok := t.preGateCandidate(track, clusters[ci], ci, dt); ok {
				pt.candidates = append(pt.candidates, c)
				frame.claims[ci]++
			}
		}
		frame.tracks = append(frame.tracks, pt)
	}
	return frame
}

// preGateCandidate repeats the gate's arithmetic without its debug side
// effects. It reports false for a pairing the tracker would never consider:
// implausible, a guarded fragment, or a singular S.
func (t *Tracker) preGateCandidate(track *TrackedObject, cluster WorldCluster, ci int, dt float32) (preGateCandidate, bool) {
	measurement := t.measurementForCluster(cluster, t.LastUpdateNanos)
	dx := measurement.X - track.X
	dy := measurement.Y - track.Y
	euclidean := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if euclidean > t.Config.MaxPositionJumpMetres {
		return preGateCandidate{}, false
	}
	if dt > 0 && euclidean/reacquisitionPlausibilityDt(t.Config.OcclusionContinuity.ReacquisitionGuard, track, dt) > t.Config.MaxReasonableSpeedMps {
		return preGateCandidate{}, false
	}
	if t.Config.AssociationExtentCostWeight <= 0 && t.isFragmentFor(track, cluster) {
		return preGateCandidate{}, false
	}
	noise := t.evaluateNoise(track, cluster, measurement)
	S00 := track.P[0*4+0] + t.Config.MeasurementNoise
	S01 := track.P[0*4+1]
	S10 := track.P[1*4+0]
	S11 := track.P[1*4+1] + t.Config.MeasurementNoise
	if t.Config.AdaptiveMeasurementNoise {
		S00, S01, S10, S11 = adaptiveInnovationCovariance(&track.P, noise.SiteCovariance)
	}
	det := S00*S11 - S01*S10
	if det < MinDeterminantThreshold {
		return preGateCandidate{}, false
	}
	invS00 := S11 / det
	invS01 := -S01 / det
	invS10 := -S10 / det
	invS11 := S00 / det
	d2 := dx*dx*invS00 + dx*dy*(invS01+invS10) + dy*dy*invS11
	return preGateCandidate{cluster: ci, dx: dx, dy: dy, d2: d2, measurement: measurement, noise: noise}, true
}

// resolvePreGate records the frame's eligible samples, closes or advances open
// rejection events, and opens new ones. associations is associate()'s result.
// Caller holds t.mu.
func (t *Tracker) resolvePreGate(frame *preGateFrame, associations []string) {
	byID := make(map[string]int, len(frame.tracks))
	for i, pt := range frame.tracks {
		byID[pt.id] = i
	}

	// Close or advance events opened on earlier frames. Only counts are
	// accumulated, so map order cannot change a result.
	hadEvent := make(map[string]bool, len(t.pendingRejections))
	for id, pending := range t.pendingRejections {
		hadEvent[id] = true
		i, active := byID[id]
		outcome := GateRejectionOutcome("")
		if !active {
			outcome = GateRejectionUnresolved
		} else {
			outcome = classifyGateRejection(frame.tracks[i], pending, t.LastUpdateNanos)
		}
		if outcome == "" {
			pending.remaining--
			if pending.remaining > 0 {
				continue
			}
			outcome = GateRejectionUnresolved
		}
		acc := &t.preGate[pending.band]
		switch outcome {
		case GateRejectionPersistent:
			acc.RejectionPersistent++
		case GateRejectionTransient:
			acc.RejectionTransient++
		default:
			acc.RejectionUnresolved++
		}
		delete(t.pendingRejections, id)
	}

	for _, pt := range frame.tracks {
		if !pt.confirmed {
			continue
		}
		acc := &t.preGate[pt.band]
		acc.Candidates += len(pt.candidates)
		if len(pt.candidates) != 1 {
			continue
		}
		c := pt.candidates[0]
		if frame.claims[c.cluster] != 1 || !c.noise.Geometry.Valid {
			continue
		}
		assigned := c.cluster < len(associations) && associations[c.cluster] == pt.id
		gated := c.d2 > t.Config.GatingDistanceSquared
		acc.Eligible++
		acc.NISSum += float64(c.d2)
		if float64(c.d2) > ChiSquared2DOF95 {
			acc.NISOverThreshold++
		}
		if gated {
			acc.Gated++
		}
		if assigned {
			acc.Assigned++
		}
		t.recordUncertaintySample(t.uncertaintySample(pt, c, gated, assigned))

		if gated && !hadEvent[pt.id] {
			t.pendingRejections[pt.id] = &pendingGateRejection{
				zX: c.measurement.X, zY: c.measurement.Y, vX: pt.track.VX, vY: pt.track.VY,
				atNanos: t.LastUpdateNanos, band: pt.band, remaining: GateRejectionLookaheadFrames,
			}
			acc.Rejections++
			if c.cluster < len(associations) && associations[c.cluster] == "" {
				acc.RejectedUnassigned++
			}
		}
	}
}

// classifyGateRejection reads one later frame for an open event. Of the
// track's plausible clusters it takes the one best explained by either
// hypothesis, and decides only if one hypothesis explains it decisively
// better. It returns "" when the frame decides nothing.
func classifyGateRejection(pt preGateTrack, pending *pendingGateRejection, nowNanos int64) GateRejectionOutcome {
	elapsed := float32(float64(nowNanos-pending.atNanos) / 1e9)
	hx := pending.zX + pending.vX*elapsed
	hy := pending.zY + pending.vY*elapsed
	best := -1
	var bestPred, bestRej float32
	for i, c := range pt.candidates {
		pred := float32(math.Hypot(float64(c.dx), float64(c.dy)))
		rej := float32(math.Hypot(float64(c.measurement.X-hx), float64(c.measurement.Y-hy)))
		if best < 0 || min(pred, rej) < min(bestPred, bestRej) {
			best, bestPred, bestRej = i, pred, rej
		}
	}
	switch {
	case best < 0:
		return ""
	case bestRej <= gateRejectionDecisiveRatio*bestPred:
		return GateRejectionPersistent
	case bestPred <= gateRejectionDecisiveRatio*bestRej:
		return GateRejectionTransient
	default:
		return ""
	}
}

// uncertaintySample projects one eligible pairing on the line of sight.
func (t *Tracker) uncertaintySample(pt preGateTrack, c preGateCandidate, gated, assigned bool) UncertaintySample {
	g := c.noise.Geometry
	ux, uy := g.RadialX, g.RadialY
	vx, vy := -uy, ux
	p := &pt.track.P
	pxy := (p[0*4+1] + p[1*4+0]) / 2
	project := func(ax, ay, xx, xy, yy float32) float32 { return ax*ax*xx + 2*ax*ay*xy + ay*ay*yy }
	noiseRadial, noiseTangential := t.Config.MeasurementNoise, t.Config.MeasurementNoise
	if t.Config.AdaptiveMeasurementNoise {
		noiseRadial, noiseTangential = c.noise.Radial, c.noise.Tangential
	}
	return UncertaintySample{
		FrameUnixNanos: t.LastUpdateNanos, X: c.measurement.X, Y: c.measurement.Y,
		Source: c.measurement.Source, Rank: 2,
		RangeMetres: g.RangeMetres, Support: g.Support, AspectRad: g.AspectRad, SpeedMps: pt.speed, Misses: pt.track.Misses,
		InnovRadial:     ux*c.dx + uy*c.dy,
		InnovTangential: vx*c.dx + vy*c.dy,
		PredRadial:      project(ux, uy, p[0*4+0], pxy, p[1*4+1]),
		PredTangential:  project(vx, vy, p[0*4+0], pxy, p[1*4+1]),
		NoiseRadial:     noiseRadial, NoiseTangential: noiseTangential,
		PhysRadial: c.noise.PhysRadial, PhysTangential: c.noise.PhysTangential,
		NIS: c.d2, Gated: gated, Assigned: assigned,
	}
}

func (t *Tracker) recordUncertaintySample(s UncertaintySample) {
	if len(t.uncertaintySamples) >= t.uncertaintySampleCap {
		t.uncertaintySamplesDropped++
		return
	}
	t.uncertaintySamples = append(t.uncertaintySamples, s)
}
