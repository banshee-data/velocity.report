package l8behaviour

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func goodSample() TrajectorySample {
	return fixtureCar(FixtureBaseUnixNanos, 10, 0, 8).followerBody().sample()
}

func TestTrajectorySampleValidate(t *testing.T) {
	if err := goodSample().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, fn := range map[string]func(*TrajectorySample){
		"no capture time":        func(s *TrajectorySample) { s.CaptureUnixNanos = 0 },
		"unknown state model":    func(s *TrajectorySample) { s.StateModel = "ca_cartesian_v2" },
		"no reference":           func(s *TrajectorySample) { s.Reference = ReferenceUnspecified },
		"no support":             func(s *TrajectorySample) { s.Support = SupportUnspecified },
		"no stage":               func(s *TrajectorySample) { s.Stage = StageUnspecified },
		"no estimation state":    func(s *TrajectorySample) { s.Estimation = EstimationUnspecified },
		"NaN position":           func(s *TrajectorySample) { s.X = math.NaN() },
		"infinite anchor offset": func(s *TrajectorySample) { s.AnchorToCentre.LateralM = math.Inf(1) },
		"NaN covariance":         func(s *TrajectorySample) { s.Covariance[7] = math.NaN() },
		"negative variance":      func(s *TrajectorySample) { s.Covariance[10] = -1 },
		"asymmetric covariance":  func(s *TrajectorySample) { s.Covariance[1] = 0.01 },
		"indefinite position block": func(s *TrajectorySample) {
			s.Covariance[1], s.Covariance[4] = 0.1, 0.1
		},
		"indefinite velocity block": func(s *TrajectorySample) {
			s.Covariance[11], s.Covariance[14] = 0.5, 0.5
		},
		"offset on a body-centre reference": func(s *TrajectorySample) { s.AnchorToCentre.LongitudinalM = 1 },
		"heading value without provenance": func(s *TrajectorySample) {
			s.Estimation, s.Faces, s.Heading = EstimationGeometryConverging, FaceVisibility{}, HeadingBelief{Rad: 1}
		},
		"unregistered heading provenance": func(s *TrajectorySample) { s.Heading.Provenance = BeliefProvenance(9) },
		"NaN heading":                     func(s *TrajectorySample) { s.Heading.Rad = math.NaN() },
		"heading weight above a half":     func(s *TrajectorySample) { s.Heading.AmbiguousModeWeight = 0.6 },
		"negative heading variance":       func(s *TrajectorySample) { s.Heading.VarianceRad2 = -0.1 },
		"zero length":                     func(s *TrajectorySample) { s.Length.Metres = 0 },
		"negative width sigma":            func(s *TrajectorySample) { s.Width.SigmaMetres = -0.1 },
		"unregistered length provenance":  func(s *TrajectorySample) { s.Length.Provenance = BeliefProvenance(9) },
		"converged class prior": func(s *TrajectorySample) {
			s.Length.Provenance = ProvenanceClassPrior
		},
		"extent value without provenance": func(s *TrajectorySample) {
			s.Estimation, s.Width = EstimationGeometryConverging, ExtentBelief{Metres: 1.8}
		},
		"face observed while coasting": func(s *TrajectorySample) {
			s.Support, s.LastObservedUnixNanos = SupportCoasted, s.CaptureUnixNanos-1
		},
		"face named with an unresolved heading": func(s *TrajectorySample) {
			s.Estimation, s.Heading.AmbiguousModeWeight = EstimationGeometryConverging, 0.5
		},
		"established with an unconverged extent": func(s *TrajectorySample) { s.Width.Converged = false },
		"established on a medoid": func(s *TrajectorySample) {
			s.Reference = ReferenceClusterMedoid
		},
		"observed after capture":          func(s *TrajectorySample) { s.LastObservedUnixNanos = s.CaptureUnixNanos + 1 },
		"never observed":                  func(s *TrajectorySample) { s.LastObservedUnixNanos = 0 },
		"observed instant seen earlier":   func(s *TrajectorySample) { s.LastObservedUnixNanos = s.CaptureUnixNanos - 1 },
		"medoid reference with an offset": func(s *TrajectorySample) { s.Reference, s.AnchorToCentre.LateralM = ReferenceClusterMedoid, 1 },
	} {
		s := goodSample()
		fn(&s)
		if err := s.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestProductionEmissionGuard(t *testing.T) {
	for _, c := range []struct {
		name string
		fn   func(*TrajectorySample)
		want SuppressionReason
	}{
		{"final, established, observed", func(s *TrajectorySample) {}, ReasonUnspecified},
		{"online stage is review-only", func(s *TrajectorySample) { s.Stage = StageOnline }, ReasonEstimateNotFinal},
		{"fixed-lag stage is review-only", func(s *TrajectorySample) { s.Stage = StageFixedLag }, ReasonEstimateNotFinal},
		{"provisional geometry", func(s *TrajectorySample) { s.Estimation = EstimationGeometryConverging }, ReasonExtentNotConverged},
		{"initialising", func(s *TrajectorySample) { s.Estimation = EstimationInitialising }, ReasonInsufficientObservation},
		{"model invalid", func(s *TrajectorySample) { s.Estimation = EstimationModelInvalid }, ReasonModelDegraded},
		{"coasted", func(s *TrajectorySample) {
			s.Support, s.Faces, s.LastObservedUnixNanos = SupportCoasted, FaceVisibility{}, s.CaptureUnixNanos-1
		}, ReasonNotObserved},
		{"coasted and degraded", func(s *TrajectorySample) {
			s.Support, s.Faces, s.LastObservedUnixNanos = SupportCoasted, FaceVisibility{}, s.CaptureUnixNanos-1
			s.Estimation = EstimationTemporarilyDegraded
		}, ReasonModelDegraded},
		{"medoid while converging", func(s *TrajectorySample) {
			s.Estimation, s.Reference = EstimationGeometryConverging, ReferenceClusterMedoid
		}, ReasonInsufficientObservation},
	} {
		s := goodSample()
		c.fn(&s)
		if err := s.Validate(); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := s.ProductionReason(); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
	// The pair of reasons behind "coasted and degraded" is retained in full.
	s := goodSample()
	s.Support, s.Faces, s.LastObservedUnixNanos = SupportCoasted, FaceVisibility{}, s.CaptureUnixNanos-1
	s.Estimation, s.Stage = EstimationTemporarilyDegraded, StageOnline
	want := []SuppressionReason{ReasonModelDegraded, ReasonNotObserved, ReasonEstimateNotFinal}
	if got := sampleReasons(s); !reflect.DeepEqual(got, want) {
		t.Fatalf("reasons %v, want %v", got, want)
	}
}

func TestTrajectoryValidate(t *testing.T) {
	good := FixtureOcclusion().Trajectories[1]
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, fn := range map[string]func(*Trajectory){
		"no samples":           func(tr *Trajectory) { tr.Samples = nil },
		"repeated capture":     func(tr *Trajectory) { tr.Samples[1] = tr.Samples[0] },
		"bad passage":          func(tr *Trajectory) { tr.Passage.TrackID = "" },
		"bad estimate":         func(tr *Trajectory) { tr.Estimate.ObsModelID = "" },
		"bad sample":           func(tr *Trajectory) { tr.Samples[3].Stage = StageUnspecified },
		"out of order samples": func(tr *Trajectory) { tr.Samples[2], tr.Samples[3] = tr.Samples[3], tr.Samples[2] },
	} {
		tr := FixtureOcclusion().Trajectories[1]
		fn(&tr)
		if err := tr.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestPassageValidate(t *testing.T) {
	good := Passage{TrackID: "trk_a", SensorID: "hesai-01", MotionClass: MotionPedestrian, ClassConfidence: ptr(0.7)}
	if err := good.Validate(); err != nil {
		t.Fatalf("a passage without a site is valid: %v", err)
	}
	for name, fn := range map[string]func(*Passage){
		"no track":          func(p *Passage) { p.TrackID = "" },
		"no sensor":         func(p *Passage) { p.SensorID = "" },
		"no class":          func(p *Passage) { p.MotionClass = MotionClassUnspecified },
		"confidence over 1": func(p *Passage) { p.ClassConfidence = ptr(1.01) },
		"NaN confidence":    func(p *Passage) { p.ClassConfidence = ptr(math.NaN()) },
	} {
		p := good
		fn(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestSupportSecondsAccounting(t *testing.T) {
	tr := FixtureOcclusion().Trajectories[1]
	// Drop frames 11 and 12 outright: the 300 ms hole is an unexplained miss,
	// attributed in full, never inherited by the observed frame before it.
	tr.Samples = append(tr.Samples[:11], tr.Samples[13:]...)
	got, err := tr.SupportSeconds(FixtureFramePeriodNanos * 3 / 2)
	if err != nil {
		t.Fatal(err)
	}
	// Observed: frames 0-4 (0.5 s) and 13 (0.1 s); frame 10's interval to
	// frame 13 is the hole.
	want := SupportSeconds{SupportObserved: 0.6, SupportCoasted: 0.5, SupportMissedUnknown: 0.3}
	for _, s := range SupportStates() {
		if math.Abs(got[s]-want[s]) > 1e-12 {
			t.Errorf("%s = %v, want %v", s, got[s], want[s])
		}
	}
	if math.Abs(got.ExposureSeconds()-0.6) > 1e-12 || math.Abs(got.TotalSeconds()-1.4) > 1e-12 {
		t.Fatalf("exposure %v total %v", got.ExposureSeconds(), got.TotalSeconds())
	}

	single := FixtureAlignedPair().Trajectories[0]
	if got, err := single.SupportSeconds(1); err != nil || len(got) != 0 || got.TotalSeconds() != 0 {
		t.Fatalf("a single sample closes the passage with no duration: %v %v", got, err)
	}
	if _, err := single.SupportSeconds(0); err == nil {
		t.Fatal("zero maximum interval accepted")
	}
	single.Samples = nil
	if _, err := single.SupportSeconds(1); err == nil || !strings.Contains(err.Error(), "no samples") {
		t.Fatalf("invalid trajectory accounted: %v", err)
	}
}
