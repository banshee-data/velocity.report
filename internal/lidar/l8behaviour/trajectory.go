package l8behaviour

// The road-user trajectory contract behaviour metrics consume (Phase 6A
// subset), per Sections 2, 7.3 and 10.2 of
// docs/plans/lidar-behaviour-analytics-plan.md and Sections 5.4 to 5.6 of
// docs/plans/lidar-state-estimation-plan.md.
//
// A sample is one instant of one road user's physical estimate: pose with its
// 4x4 covariance, orientation and extent beliefs with provenance, and the
// three facts that decide what may be done with it: the observation support at
// that instant, the estimate stage it came from, and the estimation lifecycle.
// Behaviour code reads nothing else. In particular it never reads a raw
// bounding-box centre, whose viewpoint bias is the defect the estimation plan
// exists to fix.

import (
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// StateModelCVCartesianV1 is the only dynamic state layout this contract
// accepts: [x, y, vx, vy] with a 4x4 row-major covariance. A reader never
// infers dimensionality from the covariance length (state-estimation plan,
// Section 5.4), so the discriminator is checked rather than assumed.
const StateModelCVCartesianV1 = l5tracks.StateModelCVCartesianV1

// HeadingBelief is body orientation held outside the dynamic state, with its
// own variance and an explicit 180-degree ambiguity weight on the psi+pi mode.
// A zero Provenance means no orientation belief exists at all.
type HeadingBelief struct {
	Rad                 float64          `json:"rad"`
	VarianceRad2        float64          `json:"variance_rad2"`
	AmbiguousModeWeight float64          `json:"ambiguous_mode_weight"`
	Provenance          BeliefProvenance `json:"provenance,omitempty"`
}

// Resolved reports whether front and rear are distinguishable: a belief exists
// and its direction ambiguity has collapsed.
func (h HeadingBelief) Resolved() bool {
	return h.Provenance.Valid() && h.AmbiguousModeWeight == 0
}

func (h HeadingBelief) validate() error {
	if h.Provenance == ProvenanceUnspecified {
		if h.Rad != 0 || h.VarianceRad2 != 0 || h.AmbiguousModeWeight != 0 {
			return fmt.Errorf("heading without provenance must carry no value")
		}
		return nil
	}
	if !h.Provenance.Valid() {
		return fmt.Errorf("heading provenance is %s", h.Provenance)
	}
	if !finite(h.Rad) || !finiteNonNegative(h.VarianceRad2) {
		return fmt.Errorf("heading must be finite with a non-negative variance")
	}
	if !(h.AmbiguousModeWeight >= 0 && h.AmbiguousModeWeight <= 0.5) {
		return fmt.Errorf("heading ambiguous-mode weight must be in [0, 0.5]")
	}
	return nil
}

// ExtentBelief is one body dimension: value, one-sigma, the count of frames
// admitted as evidence for it, where it came from, and whether it met the
// convergence bound in force when the sample was built. The bound is part of
// the estimate's parameter hash, not this record.
type ExtentBelief struct {
	Metres           float64          `json:"metres"`
	SigmaMetres      float64          `json:"sigma_metres"`
	AdmissibleFrames int              `json:"admissible_frames"`
	Provenance       BeliefProvenance `json:"provenance,omitempty"`
	Converged        bool             `json:"converged"`
}

// Present reports whether any belief exists, even a class prior.
func (e ExtentBelief) Present() bool { return e.Provenance.Valid() }

func (e ExtentBelief) validate(name string) error {
	if e.Provenance == ProvenanceUnspecified {
		if e.Metres != 0 || e.SigmaMetres != 0 || e.AdmissibleFrames != 0 || e.Converged {
			return fmt.Errorf("%s without provenance must carry no value", name)
		}
		return nil
	}
	if !e.Provenance.Valid() {
		return fmt.Errorf("%s provenance is %s", name, e.Provenance)
	}
	if !(e.Metres > 0) || !finite(e.Metres) || !finiteNonNegative(e.SigmaMetres) || e.AdmissibleFrames < 0 {
		return fmt.Errorf("%s must be positive and finite with a non-negative sigma and frame count", name)
	}
	// A class prior never counts as converged however tight its sigma: the
	// bound is about evidence concerning this object.
	if e.Converged && !e.Provenance.IsEvidence() {
		return fmt.Errorf("%s cannot be converged on a %s", name, e.Provenance)
	}
	return nil
}

// BodyOffset is the body-frame displacement from a sample's reference point to
// the body centre: longitudinal toward the body's front, lateral toward its
// left. It is zero for a body-centre reference and carries the declared
// geometry of any other reference, which is how an offset anchor still yields
// the physical bumper rather than the anchor.
type BodyOffset struct {
	LongitudinalM float64 `json:"longitudinal_m"`
	LateralM      float64 `json:"lateral_m"`
}

// FaceVisibility records which of the body's own end faces had returns
// associated at this instant. It is what separates a directly observed
// endpoint from an inferred one; it is never set on an unobserved instant.
type FaceVisibility struct {
	FrontObserved bool `json:"front_observed"`
	RearObserved  bool `json:"rear_observed"`
}

// TrajectorySample is one instant of one road user's estimated state.
type TrajectorySample struct {
	CaptureUnixNanos int64          `json:"capture_unix_nanos"`
	StateModel       string         `json:"state_model"`
	Reference        ReferencePoint `json:"reference"`
	X                float64        `json:"x"`
	Y                float64        `json:"y"`
	VX               float64        `json:"vx"`
	VY               float64        `json:"vy"`
	// Covariance is the 4x4 dynamic covariance, row-major over [x, y, vx, vy].
	Covariance     [16]float64    `json:"covariance"`
	AnchorToCentre BodyOffset     `json:"anchor_to_centre"`
	Heading        HeadingBelief  `json:"heading"`
	Length         ExtentBelief   `json:"length"`
	Width          ExtentBelief   `json:"width"`
	Faces          FaceVisibility `json:"faces"`

	Support    SupportState    `json:"support"`
	Stage      EstimateStage   `json:"estimate_stage"`
	Estimation EstimationState `json:"estimation_state"`
	// LastObservedUnixNanos is the capture time of the latest associated
	// measurement; equal to CaptureUnixNanos on an observed instant, and the
	// origin of coast age otherwise.
	LastObservedUnixNanos int64 `json:"last_observed_unix_nanos"`
}

// covarianceTolerance absorbs rounding in the symmetry and
// positive-semidefiniteness checks, relative to the largest diagonal term.
const covarianceTolerance = 1e-9

// Validate rejects a sample that contradicts itself. These are caller errors,
// not suppressions: an estimator that claims "established" with an unconverged
// extent, or a face observed on a coasted instant, has a defect that must fail
// loudly rather than surface later as a confident gap.
func (s TrajectorySample) Validate() error {
	if s.CaptureUnixNanos <= 0 {
		return fmt.Errorf("sample capture time must be positive")
	}
	if s.StateModel != StateModelCVCartesianV1 {
		return fmt.Errorf("sample state model %q is not %q", s.StateModel, StateModelCVCartesianV1)
	}
	if !s.Reference.Valid() {
		return fmt.Errorf("sample reference point is %s", s.Reference)
	}
	if !s.Support.Valid() || !s.Stage.Valid() || !s.Estimation.Valid() {
		return fmt.Errorf("sample support %s, stage %s and estimation state %s must all be set",
			s.Support, s.Stage, s.Estimation)
	}
	for _, v := range []float64{s.X, s.Y, s.VX, s.VY, s.AnchorToCentre.LongitudinalM, s.AnchorToCentre.LateralM} {
		if !finite(v) {
			return fmt.Errorf("sample state and anchor offset must be finite")
		}
	}
	if err := validateCovariance(s.Covariance); err != nil {
		return err
	}
	if s.Reference != ReferenceNearFaceCentre && (s.AnchorToCentre != BodyOffset{}) {
		return fmt.Errorf("a %s reference must not carry an anchor offset; only %s does", s.Reference, ReferenceNearFaceCentre)
	}
	if err := s.Heading.validate(); err != nil {
		return err
	}
	if err := s.Length.validate("length"); err != nil {
		return err
	}
	if err := s.Width.validate("width"); err != nil {
		return err
	}
	if (s.Faces.FrontObserved || s.Faces.RearObserved) && s.Support != SupportObserved {
		return fmt.Errorf("a face cannot be observed on a %s instant", s.Support)
	}
	if (s.Faces.FrontObserved || s.Faces.RearObserved) && !s.Heading.Resolved() {
		return fmt.Errorf("a front or rear face cannot be named while the heading is unresolved")
	}
	if s.Estimation == EstimationEstablished &&
		!(s.Length.Converged && s.Width.Converged && s.Heading.Resolved() && s.Reference.IsPhysical()) {
		return fmt.Errorf("an established estimate requires converged extents, a resolved heading and a physical reference")
	}
	if s.LastObservedUnixNanos <= 0 || s.LastObservedUnixNanos > s.CaptureUnixNanos {
		return fmt.Errorf("sample last-observed time must be positive and not after capture")
	}
	if s.Support == SupportObserved && s.LastObservedUnixNanos != s.CaptureUnixNanos {
		return fmt.Errorf("an observed instant must have been last observed at its own capture time")
	}
	return nil
}

func validateCovariance(p [16]float64) error {
	scale := 1.0
	for i := 0; i < 4; i++ {
		scale = math.Max(scale, math.Abs(p[i*4+i]))
	}
	tol := covarianceTolerance * scale
	for i := 0; i < 16; i++ {
		if !finite(p[i]) {
			return fmt.Errorf("covariance must be finite")
		}
	}
	for i := 0; i < 4; i++ {
		if p[i*4+i] < 0 {
			return fmt.Errorf("covariance diagonal must not be negative")
		}
		for j := i + 1; j < 4; j++ {
			if math.Abs(p[i*4+j]-p[j*4+i]) > tol {
				return fmt.Errorf("covariance must be symmetric")
			}
		}
	}
	// The position and velocity blocks are what endpoints and speeds are
	// projected through; each must be positive semidefinite for u'Pu >= 0.
	for _, b := range [][3]int{{0, 1, 5}, {10, 11, 15}} {
		if p[b[0]]*p[b[2]]-p[b[1]]*p[b[1]] < -tol*scale {
			return fmt.Errorf("covariance position and velocity blocks must be positive semidefinite")
		}
	}
	return nil
}

// ProductionReason is the production-emission guard for one sample. It
// returns ReasonUnspecified only when a result derived from this sample may
// reach a production surface: final stage, established estimate, physical
// reference and an observed instant. Anything else is review-only, and the
// reason says why, in precedence order.
//
// geometry_converging maps to extent_not_converged because that is what the
// state means: the body geometry has not converged. The first increment
// publishes nothing provisional, so even a motion-only metric is review-only
// in that state (Section 12.2).
func (s TrajectorySample) ProductionReason() SuppressionReason {
	if reasons := sampleReasons(s); len(reasons) > 0 {
		return reasons[0]
	}
	return ReasonUnspecified
}

// sampleReasons lists every emission-guard reason that applies to one sample,
// in precedence order. A pairwise evaluation collects these for both parties
// so the reported reason is the most fundamental across the pair, not merely
// the first party's.
func sampleReasons(s TrajectorySample) []SuppressionReason {
	var set reasonSet
	switch s.Estimation {
	case EstimationModelInvalid, EstimationTemporarilyDegraded:
		set.add(ReasonModelDegraded)
	case EstimationInitialising:
		set.add(ReasonInsufficientObservation)
	case EstimationGeometryConverging:
		set.add(ReasonExtentNotConverged)
	}
	if !s.Reference.IsPhysical() {
		set.add(ReasonInsufficientObservation)
	}
	if s.Support != SupportObserved {
		set.add(ReasonNotObserved)
	}
	if s.Stage != StageFinal {
		set.add(ReasonEstimateNotFinal)
	}
	return set.ordered()
}

// reviewPoseAllowed reports whether this sample's pose may be shown at all,
// even on a review surface. Initialising and model_invalid forbid it, as does
// a reference that is not a place on the body.
func (s TrajectorySample) reviewPoseAllowed() bool {
	return s.Estimation.AllowsReportedPose() && s.Reference.IsPhysical()
}

// Passage is one road user traversing one site: never a driver, never a
// profile (Section 1.1). Class confidence gates applicability, and an absent
// confidence is unknown rather than zero.
type Passage struct {
	TrackID string `json:"track_id"`
	// SiteID is empty when the capture has no configured site.
	SiteID          string      `json:"site_id,omitempty"`
	SensorID        string      `json:"sensor_id"`
	MotionClass     MotionClass `json:"motion_class"`
	ClassLabel      string      `json:"class_label,omitempty"`
	ClassConfidence *float64    `json:"class_confidence,omitempty"`
}

// Validate requires identity and a set motion class.
func (p Passage) Validate() error {
	if p.TrackID == "" || p.SensorID == "" {
		return fmt.Errorf("passage requires a track id and a sensor id")
	}
	if !p.MotionClass.Valid() {
		return fmt.Errorf("passage %s motion class is %s", p.TrackID, p.MotionClass)
	}
	if p.ClassConfidence != nil && !(*p.ClassConfidence >= 0 && *p.ClassConfidence <= 1) {
		return fmt.Errorf("passage %s class confidence must be in [0, 1]", p.TrackID)
	}
	return nil
}

// EstimateIdentity names the estimator run a trajectory came from. Two
// trajectories paired into one result must share it: mixing estimator
// versions inside one number is the silent drift Section 10.3 forbids.
type EstimateIdentity struct {
	EstimatorID string `json:"estimator_id"`
	ObsModelID  string `json:"obs_model_id"`
	ParamHash   string `json:"param_hash"`
}

// Validate requires all three identities.
func (e EstimateIdentity) Validate() error {
	if e.EstimatorID == "" || e.ObsModelID == "" || e.ParamHash == "" {
		return fmt.Errorf("estimate identity requires estimator, observation model and parameter hash")
	}
	return nil
}

// Trajectory is one passage's samples in capture order.
type Trajectory struct {
	Passage  Passage            `json:"passage"`
	Estimate EstimateIdentity   `json:"estimate"`
	Samples  []TrajectorySample `json:"samples"`
}

// Validate checks identity, strict capture ordering and every sample.
func (t Trajectory) Validate() error {
	if err := t.Passage.Validate(); err != nil {
		return err
	}
	if err := t.Estimate.Validate(); err != nil {
		return fmt.Errorf("trajectory %s: %w", t.Passage.TrackID, err)
	}
	if len(t.Samples) == 0 {
		return fmt.Errorf("trajectory %s has no samples", t.Passage.TrackID)
	}
	for i, s := range t.Samples {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("trajectory %s sample %d: %w", t.Passage.TrackID, i, err)
		}
		if i > 0 && s.CaptureUnixNanos <= t.Samples[i-1].CaptureUnixNanos {
			return fmt.Errorf("trajectory %s samples must be strictly increasing in capture time", t.Passage.TrackID)
		}
	}
	return nil
}

// SampleAt returns the sample captured exactly at the given time. Pairwise
// metrics need synchronised states; interpolating to a common instant is the
// pairing step's job, not a lookup's.
func (t Trajectory) SampleAt(captureUnixNanos int64) (TrajectorySample, bool) {
	i := sort.Search(len(t.Samples), func(i int) bool {
		return t.Samples[i].CaptureUnixNanos >= captureUnixNanos
	})
	if i < len(t.Samples) && t.Samples[i].CaptureUnixNanos == captureUnixNanos {
		return t.Samples[i], true
	}
	return TrajectorySample{}, false
}

// SupportSeconds is a passage's duration by support state (Section 10.2,
// passage evidence). Only the observed share may enter an exposure
// denominator.
type SupportSeconds map[SupportState]float64

// ExposureSeconds is the time that may enter an exposure denominator.
func (s SupportSeconds) ExposureSeconds() float64 {
	var total float64
	for _, state := range SupportStates() {
		if state.CountsTowardExposure() {
			total += s[state]
		}
	}
	return total
}

// TotalSeconds is the accounted duration across every state. States are summed
// in vocabulary order rather than map order so the float result is identical
// on every run.
func (s SupportSeconds) TotalSeconds() float64 {
	var total float64
	for _, state := range SupportStates() {
		total += s[state]
	}
	return total
}

// SupportSeconds accounts the trajectory's duration by support state.
//
// Each interval between consecutive samples is attributed to the earlier
// sample's state (sample and hold), so the last sample closes the passage and
// contributes no duration of its own. An interval longer than maxIntervalNanos
// means estimate rows are missing outright, which is an unexplained miss: it
// is attributed to missed_unknown in full, never to the state either side of
// it, so a gap in the record cannot inflate observed time.
func (t Trajectory) SupportSeconds(maxIntervalNanos int64) (SupportSeconds, error) {
	if maxIntervalNanos <= 0 {
		return nil, fmt.Errorf("support accounting requires a positive maximum interval")
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	nanos := make(map[SupportState]int64)
	for i := 0; i+1 < len(t.Samples); i++ {
		dt := t.Samples[i+1].CaptureUnixNanos - t.Samples[i].CaptureUnixNanos
		state := t.Samples[i].Support
		if dt > maxIntervalNanos {
			state = SupportMissedUnknown
		}
		nanos[state] += dt
	}
	out := make(SupportSeconds, len(nanos))
	for state, n := range nanos {
		out[state] = float64(n) / 1e9
	}
	return out, nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
