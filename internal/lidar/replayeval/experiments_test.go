package replayeval

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
)

func TestNormaliseExperimentsSortsAndDeduplicates(t *testing.T) {
	got, err := NormaliseExperiments([]string{" likelihood_cost", "cascade", "", "likelihood_cost "})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{ExperimentCascade, ExperimentLikelihoodCost}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// range_adaptive_eps (experiment l4clus0001) is a name the harness accepts,
// spelled one way; a near miss is refused rather than run as the baseline.
func TestRangeAdaptiveEpsIsAKnownExperiment(t *testing.T) {
	got, err := ParseExperiments("range_adaptive_eps")
	if err != nil || !reflect.DeepEqual(got, []string{ExperimentRangeAdaptiveEps}) {
		t.Fatalf("ParseExperiments(range_adaptive_eps) = %v, %v", got, err)
	}
	if IsBackgroundExperiment(ExperimentRangeAdaptiveEps) {
		t.Error("range_adaptive_eps acts on L4, not the background model")
	}
	for _, bad := range []string{"range-adaptive-eps", "range_adaptive", "adaptive_eps", "Range_Adaptive_Eps"} {
		if _, err := ParseExperiments(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// A misspelt option must never run as the baseline and be reported as the
// option.
func TestNormaliseExperimentsRejectsUnknownNames(t *testing.T) {
	_, err := NormaliseExperiments([]string{"likelihood-cost"})
	if err == nil {
		t.Fatal("an unknown experiment name was accepted")
	}
	if !strings.Contains(err.Error(), ExperimentLikelihoodCost) {
		t.Errorf("error %q does not list the known names", err)
	}
}

// dwell_policy is accepted by its name alone, hashes apart from the default,
// and is not a background experiment (it needs a tracker); a near miss of the
// name is refused rather than run as the baseline.
func TestDwellPolicyIsAKnownExperiment(t *testing.T) {
	got, err := ParseExperiments(" dwell_policy ")
	if err != nil {
		t.Fatalf("dwell_policy refused: %v", err)
	}
	if want := []string{ExperimentDwellPolicy}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if experimentsHashSuffix(got) == nil {
		t.Error("dwell_policy left the parameter hash as the default's")
	}
	if IsBackgroundExperiment(ExperimentDwellPolicy) {
		t.Error("dwell_policy is not background-only: it needs L5")
	}
	for _, near := range []string{"dwell-policy", "dwell", "dwellpolicy", "Dwell_Policy"} {
		if _, err := ParseExperiments(near); err == nil {
			t.Errorf("%q was accepted", near)
		} else if !strings.Contains(err.Error(), ExperimentDwellPolicy) {
			t.Errorf("%q: error %q does not list dwell_policy among the known names", near, err)
		}
	}
}

// dwell_policy_t40 and dwell_policy_t60 (l3bg0015) are the dwell policy with
// a 40 s and a 60 s clock, and dwell_policy_t40_drift2m and
// dwell_policy_t40_median (l3bg0017) the 40 s clock with a 2 m drift
// tolerance and with drift measured from the run's running median. Each is
// accepted by its name alone, hashes apart from the default and from the
// other settings, needs a tracker, and builds the policy at its own T and
// drift rule, which the policy's report echoes; dwell_policy still builds the
// 20 s policy with the shipped rule it always did, and naming none builds
// nothing. Two settings together are refused, never resolved.
func TestDwellPolicySettings(t *testing.T) {
	median := l5tracks.StationaryRule{Anchor: l5tracks.StationaryAnchorMedian}
	cases := []struct {
		name       string
		after      time.Duration
		stationary l5tracks.StationaryRule
		drift      float64
		anchor     string
	}{
		{ExperimentDwellPolicy, 20 * time.Second, l5tracks.StationaryRule{}, 1, "first_estimate"},
		{ExperimentDwellPolicyT40, 40 * time.Second, l5tracks.StationaryRule{}, 1, "first_estimate"},
		{ExperimentDwellPolicyT60, 60 * time.Second, l5tracks.StationaryRule{}, 1, "first_estimate"},
		{ExperimentDwellPolicyT40Drift2m, 40 * time.Second, l5tracks.StationaryRule{DriftMetres: 2}, 2, "first_estimate"},
		{ExperimentDwellPolicyT40Median, 40 * time.Second, median, 1, "running_median"},
	}
	suffixes := map[string]string{}
	for _, c := range cases {
		got, err := ParseExperiments(" " + c.name + " ")
		if err != nil || !reflect.DeepEqual(got, []string{c.name}) {
			t.Fatalf("ParseExperiments(%s) = %v, %v", c.name, got, err)
		}
		suffix := experimentsHashSuffix(got)
		if suffix == nil {
			t.Errorf("%s left the parameter hash as the default's", c.name)
		}
		for other, s := range suffixes {
			if s == string(suffix) {
				t.Errorf("%s hashes as %s", c.name, other)
			}
		}
		suffixes[c.name] = string(suffix)
		if IsBackgroundExperiment(c.name) {
			t.Errorf("%s is not background-only: it needs L5", c.name)
		}
		policy, err := dwellPolicyFor(got)
		if err != nil || policy == nil {
			t.Fatalf("%s: policy %v, %v", c.name, policy, err)
		}
		if policy.AbsorbAfter != c.after || policy.Stationary != c.stationary {
			t.Errorf("%s: AbsorbAfter %v, rule %+v; want %v, %+v", c.name, policy.AbsorbAfter, policy.Stationary, c.after, c.stationary)
		}
		if r := policy.Report(); r.AbsorbAfterSeconds != c.after.Seconds() || r.StationarySpeedMps != l5tracks.StationarySpeedMps ||
			r.StationaryDriftMetres != c.drift || r.StationaryAnchor != c.anchor {
			t.Errorf("%s: report %+v, want T %v, %g m from %s and the shared speed", c.name, r, c.after, c.drift, c.anchor)
		}
	}
	plain, err := dwellPolicyFor([]string{ExperimentDwellPolicy})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plain.Report(), (&pipeline.DwellPolicy{}).Report()) || pipeline.DwellAbsorbAfter != 20*time.Second {
		t.Errorf("dwell_policy reports %+v, want the 20 s policy's %+v", plain.Report(), (&pipeline.DwellPolicy{}).Report())
	}
	for _, off := range [][]string{nil, {}, {ExperimentCascade, ExperimentSupportFilter}} {
		if policy, err := dwellPolicyFor(off); err != nil || policy != nil {
			t.Errorf("%v: policy %+v, %v; want none", off, policy, err)
		}
	}
	for _, refused := range [][]string{
		{ExperimentDwellPolicy, ExperimentDwellPolicyT40},
		{ExperimentDwellPolicy, ExperimentDwellPolicyT60},
		{ExperimentDwellPolicyT40, ExperimentDwellPolicyT60},
		{ExperimentDwellPolicy, ExperimentDwellPolicyT40, ExperimentDwellPolicyT60},
		{ExperimentDwellPolicyT40, ExperimentDwellPolicyT40Drift2m},
		{ExperimentDwellPolicyT40, ExperimentDwellPolicyT40Median},
		{ExperimentDwellPolicyT40Drift2m, ExperimentDwellPolicyT40Median},
		{ExperimentDwellPolicy, ExperimentDwellPolicyT40Median},
		{ExperimentDwellPolicyT60, ExperimentDwellPolicyT40Drift2m},
	} {
		if policy, err := dwellPolicyFor(refused); err == nil || !strings.Contains(err.Error(), "settings of one option") {
			t.Errorf("%v: policy %+v, %v; want the pair refused", refused, policy, err)
		}
	}
	for _, near := range []string{"dwell_policy_t20", "dwell_policy_t30", "dwell_policy_40", "dwell_policy_t40s", "dwell_policy_T60",
		"dwell_policy_drift2m", "dwell_policy_median", "dwell_policy_t60_median", "dwell_policy_t40_drift1m", "dwell_policy_t40_mean"} {
		if _, err := ParseExperiments(near); err == nil {
			t.Errorf("%q was accepted", near)
		}
	}
}

// Two dwell policy settings are refused before any capture is read.
func TestRunRefusesTwoDwellSettingsBeforeReadingACapture(t *testing.T) {
	for _, pair := range [][]string{
		{ExperimentDwellPolicyT60, ExperimentDwellPolicy},
		{ExperimentDwellPolicyT40Median, ExperimentDwellPolicyT40Drift2m},
	} {
		_, err := Run(Config{
			PCAPFile: "missing.pcap", OutDir: filepath.Join(t.TempDir(), "out"), UDPPort: 2369,
			Experiments: pair,
		})
		if err == nil || !strings.Contains(err.Error(), "settings of one option") {
			t.Fatalf("%v: got %v, want both dwell settings refused before the missing capture is read", pair, err)
		}
	}
}

func TestParseExperimentsEmptyIsShippedBehaviour(t *testing.T) {
	for _, in := range []string{"", "  ", ","} {
		got, err := ParseExperiments(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if len(got) != 0 {
			t.Errorf("%q: got %v, want none", in, got)
		}
		if got == nil {
			t.Errorf("%q: nil list would marshal as null, not []", in)
		}
	}
}

// The parameter hash of a replay without experiments must not move, or every
// earlier baseline stops being comparable. With experiments it must depend on
// the selection and not on the order it was written in.
func TestExperimentsHashSuffix(t *testing.T) {
	if s := experimentsHashSuffix(nil); s != nil {
		t.Errorf("no experiments produced suffix %q; existing parameter hashes would change", s)
	}
	if s := experimentsHashSuffix([]string{}); s != nil {
		t.Errorf("empty experiments produced suffix %q", s)
	}
	a, _ := ParseExperiments("cascade,likelihood_cost")
	b, _ := ParseExperiments("likelihood_cost,cascade")
	if string(experimentsHashSuffix(a)) != string(experimentsHashSuffix(b)) {
		t.Error("suffix depends on the order the experiments were written in")
	}
	c, _ := ParseExperiments("cascade")
	if string(experimentsHashSuffix(a)) == string(experimentsHashSuffix(c)) {
		t.Error("different selections share a suffix")
	}
}

func TestKnownExperimentsIsSortedAndComplete(t *testing.T) {
	got := KnownExperiments()
	want := []string{ExperimentAdaptiveUncertainty, ExperimentBirthGateBody, ExperimentBorderClasses, ExperimentBoxExtentSharedAxis, ExperimentBoxExtentSolidBody, ExperimentBoxGround, ExperimentCaptureGapPredict, ExperimentCascade, ExperimentClassCoastBounds,
		ExperimentClassIdentity, ExperimentCoastSupport, ExperimentCoastTimeInflation, ExperimentConsistentCovarianceCap, ExperimentConsistentSpeedClamp, ExperimentContainmentClaim, ExperimentContestedRejoin,
		ExperimentDensityCap, ExperimentDwellPolicy,
		ExperimentDwellPolicyT40, ExperimentDwellPolicyT40Drift2m, ExperimentDwellPolicyT40Median, ExperimentDwellPolicyT60,
		ExperimentFixedLagRTS, ExperimentFlipRule, ExperimentFragmentJoinTrackMembership,
		ExperimentHeldToL4, ExperimentLikelihoodCost, ExperimentMeasurementTime, ExperimentNearEdgeTrack, ExperimentNearEdgeTrackA1,
		ExperimentNoRegionOverrides,
		ExperimentOcclusionContinuity, ExperimentOverlapEnd1, ExperimentOverlapEnd3, ExperimentOverlapEndBody,
		ExperimentOverlapEndOlder, ExperimentRadiusGraphXY, ExperimentRadiusGraphXYOverlap, ExperimentRangeAdaptiveEps, ExperimentReacquisitionGuard, ExperimentRegionOverridesCellTercile, ExperimentRegionOverridesQuietOnly, ExperimentRegionOverridesVolatileOnly, ExperimentRobustBirthGate, ExperimentRobustLikelihoodCost, ExperimentSettlingHonest, ExperimentSettlingRelative0075, ExperimentSolidBody,
		ExperimentSolidBodyAnisotropicReferenceBias, ExperimentSolidBodyCensoredExtent, ExperimentSolidBodyContainment,
		ExperimentSolidBodyCourseFaces,
		ExperimentSolidBodyCourseHeading, ExperimentSolidBodyEndFaceCentring,
		ExperimentSolidBodyEndFaceCentringOpenPrior, ExperimentSolidBodyExtentAccrual, ExperimentSolidBodyExtentGrowth,
		ExperimentSolidBodyExtentPriorFloor, ExperimentSolidBodyExtentState,
		ExperimentSolidBodyFaceConsider, ExperimentSolidBodyFaceHysteresis, ExperimentSolidBodyFacePlaneSpans,
		ExperimentSolidBodyFaceState, ExperimentSolidBodyFullMembers, ExperimentSolidBodyHeadingFaceNoise,
		ExperimentSolidBodyRankOneMedoid, ExperimentSolidBodyRankOneMedoidTight, ExperimentSolidBodyRectangleCourseFusion,
		ExperimentSolidBodyRectangleFit, ExperimentSolidBodyRectangleHeading, ExperimentSolidBodyRectangleSigmaMid,
		ExperimentSolidBodyRectangleSigmaWide,
		ExperimentSolidBodyReferenceTranslation, ExperimentSolidBodySharedOrientation, ExperimentSolidBodySpanBoundsBelief,
		ExperimentSolidBodyVehicleExtentFloor, ExperimentSupportFilter, ExperimentSurfaceGround, ExperimentSurfaceGround5m, ExperimentSurfaceGroundGlobal, ExperimentTentativePriority,
		ExperimentTwoPointVelocityInit, ExperimentVolatileWindowCap1m, ExperimentVolatileWindowCap2m}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Each tracker experiment must reach exactly its own TrackerConfig field, and
// no experiment must leave the configuration the tuning file describes. The
// kirk0 A/B shows an option changes a replay; this shows it is the right one.
func TestTrackerExperimentsReachTheirOwnOption(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	shipped := l5tracks.TrackerConfigFromTuning(l5)
	got, err := trackerConfigFor(l5, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != shipped {
		t.Fatalf("no experiments changed the tracker configuration:\n got %+v\nwant %+v", got, shipped)
	}
	cases := map[string]func(*l5tracks.TrackerConfig){
		ExperimentAdaptiveUncertainty: func(c *l5tracks.TrackerConfig) {
			c.AdaptiveMeasurementNoise = true
		},
		ExperimentLikelihoodCost: func(c *l5tracks.TrackerConfig) { c.LikelihoodAssociationCost = true },
		ExperimentRobustLikelihoodCost: func(c *l5tracks.TrackerConfig) {
			c.RobustLikelihoodAssociationCost = true
		},
		ExperimentCascade:           func(c *l5tracks.TrackerConfig) { c.CascadedAssociation = true },
		ExperimentFlipRule:          func(c *l5tracks.TrackerConfig) { c.OBBHeadingFlipRule = true },
		ExperimentMeasurementTime:   func(c *l5tracks.TrackerConfig) { c.MeasurementTimePrediction = true },
		ExperimentCaptureGapPredict: func(c *l5tracks.TrackerConfig) { c.CaptureGapPrediction = true },
		ExperimentTwoPointVelocityInit: func(c *l5tracks.TrackerConfig) {
			c.TwoPointVelocityInit = true
		},
		ExperimentConsistentCovarianceCap: func(c *l5tracks.TrackerConfig) {
			c.ConsistentCovarianceCap = true
		},
		ExperimentConsistentSpeedClamp: func(c *l5tracks.TrackerConfig) {
			c.ConsistentSpeedClamp = true
		},
		ExperimentCoastTimeInflation: func(c *l5tracks.TrackerConfig) {
			c.OcclusionContinuity = continuityWith(func(o *l5tracks.OcclusionContinuityConfig) { o.CaptureTimeInflation = true })
		},
		ExperimentReacquisitionGuard: func(c *l5tracks.TrackerConfig) {
			c.OcclusionContinuity = continuityWith(func(o *l5tracks.OcclusionContinuityConfig) { o.ReacquisitionGuard = true })
		},
		// ContestedRejoin does nothing without the guard, so it brings it.
		ExperimentContestedRejoin: func(c *l5tracks.TrackerConfig) {
			c.OcclusionContinuity = continuityWith(func(o *l5tracks.OcclusionContinuityConfig) {
				o.ReacquisitionGuard, o.ContestedRejoin = true, true
			})
		},
		ExperimentTentativePriority: func(c *l5tracks.TrackerConfig) { c.TentativePriority = true },
		ExperimentClassIdentity:     func(c *l5tracks.TrackerConfig) { c.ClassIdentity = true },
		ExperimentOverlapEnd1:       func(c *l5tracks.TrackerConfig) { c.OverlapEndFrames = 1 },
		ExperimentOverlapEnd3:       func(c *l5tracks.TrackerConfig) { c.OverlapEndFrames = 3 },
		ExperimentBoxGround:         func(c *l5tracks.TrackerConfig) { c.BoxModel.GroundAnchor = true },
		// The replay tracks in the sensor frame, so the origin is the
		// sensor, derived from the identity tracking transform.
		ExperimentSolidBody: func(c *l5tracks.TrackerConfig) {
			c.SolidBody = l5tracks.SolidBodyOptions{Enabled: true, OriginSource: OriginTrackingTransformIdentity}
		},
	}
	for name, set := range cases {
		want := shipped
		set(&want)
		got, err := trackerConfigFor(l5, "", []string{name}, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != want {
			t.Errorf("%s:\n got %+v\nwant %+v", name, got, want)
		}
	}
	// The containment claim reaches its own option on top of the contained
	// box it reads, and is refused without that box.
	{
		want := shipped
		want.SolidBody = l5tracks.SolidBodyOptions{Enabled: true, OriginSource: OriginTrackingTransformIdentity, Containment: true}
		want.ContainmentClaim = true
		got, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodyContainment, ExperimentSolidBodyFullMembers, ExperimentContainmentClaim}, nil)
		if err != nil {
			t.Fatalf("%s: %v", ExperimentContainmentClaim, err)
		}
		if got != want {
			t.Errorf("%s:\n got %+v\nwant %+v", ExperimentContainmentClaim, got, want)
		}
		for _, without := range [][]string{
			{ExperimentContainmentClaim},
			{ExperimentSolidBody, ExperimentContainmentClaim},
			{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentContainmentClaim},
			{ExperimentSolidBody, ExperimentSolidBodyContainment, ExperimentContainmentClaim},
		} {
			if _, err := trackerConfigFor(l5, "", without, nil); err == nil || !strings.Contains(err.Error(), ExperimentContainmentClaim) {
				t.Errorf("%q: want a refusal naming %s, got %v", without, ExperimentContainmentClaim, err)
			}
		}
	}
	// The face remedies qualify the solid body, each reaching its own option,
	// and are refused on their own.
	for name, set := range map[string]func(*l5tracks.SolidBodyOptions){
		ExperimentSolidBodyFaceHysteresis: func(o *l5tracks.SolidBodyOptions) { o.FaceHysteresis = true },
		ExperimentSolidBodyFaceConsider:   func(o *l5tracks.SolidBodyOptions) { o.FaceEntryConsider = true },
		ExperimentSolidBodyCourseFaces:    func(o *l5tracks.SolidBodyOptions) { o.CourseAlignedFaces = true },
		ExperimentSolidBodyReferenceTranslation: func(o *l5tracks.SolidBodyOptions) {
			o.ReferenceTranslation = true
		},
		ExperimentSolidBodyRankOneMedoid:      func(o *l5tracks.SolidBodyOptions) { o.RankOneMedoidScale = 1 },
		ExperimentSolidBodyRankOneMedoidTight: func(o *l5tracks.SolidBodyOptions) { o.RankOneMedoidScale = 0.25 },
		ExperimentSolidBodyCourseHeading:      func(o *l5tracks.SolidBodyOptions) { o.CourseHeading = true },
		ExperimentSolidBodyExtentPriorFloor:   func(o *l5tracks.SolidBodyOptions) { o.ExtentPriorFloor = true },
		ExperimentSolidBodyFacePlaneSpans:     func(o *l5tracks.SolidBodyOptions) { o.FacePlaneSpans = true },
		ExperimentSolidBodyExtentGrowth:       func(o *l5tracks.SolidBodyOptions) { o.ExtentGrowthAdmission = true },
		ExperimentSolidBodyVehicleExtentFloor: func(o *l5tracks.SolidBodyOptions) { o.VehicleExtentFloor = true },
		ExperimentSolidBodyEndFaceCentring:    func(o *l5tracks.SolidBodyOptions) { o.EndFaceCentring = true },
		ExperimentSolidBodyContainment:        func(o *l5tracks.SolidBodyOptions) { o.Containment = true },
		ExperimentSolidBodyRectangleFit:       func(o *l5tracks.SolidBodyOptions) { o.RectangleFit = true },
		ExperimentSolidBodyRectangleHeading:   func(o *l5tracks.SolidBodyOptions) { o.RectangleHeading = true },
		ExperimentSolidBodyRectangleSigmaWide: func(o *l5tracks.SolidBodyOptions) { o.RectangleSigmaScale = 2 },
		ExperimentSolidBodyRectangleSigmaMid:  func(o *l5tracks.SolidBodyOptions) { o.RectangleSigmaScale = 1.5 },
		ExperimentSolidBodyRectangleCourseFusion: func(o *l5tracks.SolidBodyOptions) {
			o.RectangleCourseFusion = true
		},
		ExperimentSolidBodyEndFaceCentringOpenPrior: func(o *l5tracks.SolidBodyOptions) {
			o.EndFaceCentring, o.EndFaceCentringOpenPrior = true, true
		},
		ExperimentSolidBodyExtentState:      func(o *l5tracks.SolidBodyOptions) { o.ExtentState = true },
		ExperimentSolidBodyCensoredExtent:   func(o *l5tracks.SolidBodyOptions) { o.CensoredExtent = true },
		ExperimentSolidBodySpanBoundsBelief: func(o *l5tracks.SolidBodyOptions) { o.SpanBoundsBelief = true },
		ExperimentSolidBodyHeadingFaceNoise: func(o *l5tracks.SolidBodyOptions) { o.HeadingFaceNoise = true },
		ExperimentSolidBodyAnisotropicReferenceBias: func(o *l5tracks.SolidBodyOptions) {
			o.AnisotropicReferenceBias = true
		},
		// The combined arm is the five together, and nothing else.
		ExperimentSolidBodySharedOrientation: func(o *l5tracks.SolidBodyOptions) { o.SharedOrientation = true },
		ExperimentSolidBodyFaceState: func(o *l5tracks.SolidBodyOptions) {
			o.ExtentState, o.CensoredExtent, o.SpanBoundsBelief = true, true, true
			o.HeadingFaceNoise, o.AnisotropicReferenceBias = true, true
		},
	} {
		want := shipped
		want.SolidBody = l5tracks.SolidBodyOptions{Enabled: true, OriginSource: OriginTrackingTransformIdentity}
		set(&want.SolidBody)
		got, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, name}, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != want {
			t.Errorf("%s:\n got %+v\nwant %+v", name, got, want)
		}
		if _, err := trackerConfigFor(l5, "", []string{name}, nil); err == nil {
			t.Errorf("%s was accepted without %s", name, ExperimentSolidBody)
		}
	}
	// T5's two settings are one option, so naming both is refused rather
	// than resolved to either.
	if _, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodyRankOneMedoid,
		ExperimentSolidBodyRankOneMedoidTight}, nil); err == nil {
		t.Errorf("%s and %s were accepted together", ExperimentSolidBodyRankOneMedoid, ExperimentSolidBodyRankOneMedoidTight)
	}
	if _, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodyRectangleSigmaWide,
		ExperimentSolidBodyRectangleSigmaMid}, nil); err == nil {
		t.Errorf("%s and %s were accepted together", ExperimentSolidBodyRectangleSigmaWide, ExperimentSolidBodyRectangleSigmaMid)
	}
	if _, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodyEndFaceCentring,
		ExperimentSolidBodyEndFaceCentringOpenPrior}, nil); err == nil {
		t.Errorf("%s and %s were accepted together", ExperimentSolidBodyEndFaceCentring, ExperimentSolidBodyEndFaceCentringOpenPrior)
	}
	// The combined face-state arm named with one of its parts is the same
	// arm. On the tracked filter the extents in the state are carried by the
	// tracked filter itself, so both names combine with near_edge_track, as
	// the other four do.
	combined, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodyFaceState}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodyFaceState, ExperimentSolidBodyExtentState}, nil); err != nil || again != combined {
		t.Errorf("%s with %s is not the combined arm (%v)", ExperimentSolidBodyFaceState, ExperimentSolidBodyExtentState, err)
	}
	for _, name := range []string{ExperimentSolidBodyExtentState, ExperimentSolidBodyFaceState} {
		cfg, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack, name}, nil)
		if err != nil || !cfg.NearEdgeTracking || !cfg.SolidBody.ExtentState {
			t.Errorf("%s with %s: want the tracked extent state, got %v (%+v)", name, ExperimentNearEdgeTrack, err, cfg.SolidBody)
		}
	}
	tracked, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack,
		ExperimentSolidBodyCensoredExtent, ExperimentSolidBodySpanBoundsBelief, ExperimentSolidBodyHeadingFaceNoise,
		ExperimentSolidBodyAnisotropicReferenceBias}, nil)
	if err != nil || !tracked.NearEdgeTracking || tracked.SolidBody.ExtentState || !tracked.SolidBody.CensoredExtent ||
		!tracked.SolidBody.SpanBoundsBelief || !tracked.SolidBody.HeadingFaceNoise || !tracked.SolidBody.AnisotropicReferenceBias {
		t.Errorf("the four tracked face-state options: %v %+v", err, tracked.SolidBody)
	}
	// Full members change what L4 hands over, not the tracker's options, and
	// are refused without a solid body to read them.
	withBody, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers}, nil); err != nil || got != withBody {
		t.Errorf("%s changed the tracker configuration (%v)", ExperimentSolidBodyFullMembers, err)
	}
	if _, err := trackerConfigFor(l5, "", []string{ExperimentSolidBodyFullMembers}, nil); err == nil {
		t.Errorf("%s was accepted without %s", ExperimentSolidBodyFullMembers, ExperimentSolidBody)
	}
	// Pipeline, background and observer experiments must not touch the tracker.
	for _, name := range []string{ExperimentDensityCap, ExperimentDwellPolicy, ExperimentDwellPolicyT40, ExperimentDwellPolicyT60,
		ExperimentDwellPolicyT40Drift2m, ExperimentDwellPolicyT40Median,
		ExperimentRangeAdaptiveEps, ExperimentNoRegionOverrides, ExperimentRegionOverridesVolatileOnly, ExperimentRegionOverridesQuietOnly,
		ExperimentRegionOverridesCellTercile, ExperimentVolatileWindowCap1m, ExperimentVolatileWindowCap2m, ExperimentSupportFilter, ExperimentFixedLagRTS} {
		got, err := trackerConfigFor(l5, "", []string{name}, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != shipped {
			t.Errorf("%s changed the tracker configuration", name)
		}
	}
	got, err = trackerConfigFor(l5, l5tracks.MeasurementOBBCentreV1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.MeasurementSourceMode != l5tracks.MeasurementOBBCentreV1 {
		t.Errorf("measurement source mode not applied: %q", got.MeasurementSourceMode)
	}
	// Absence-explaining continuity experiments must not run until replayeval
	// can supply explicit sensor coverage.
	if _, err := trackerConfigFor(l5, "", []string{ExperimentCoastSupport}, nil); err == nil {
		t.Fatal("coast_support was accepted without explicit coverage")
	}
	if _, err := trackerConfigFor(l5, "", []string{ExperimentClassCoastBounds}, nil); err == nil {
		t.Fatal("class_coast_bounds was accepted without explicit coverage")
	}
	if _, err := trackerConfigFor(l5, "", []string{ExperimentOcclusionContinuity}, nil); err == nil {
		t.Fatal("occlusion_continuity was accepted without explicit coverage")
	}
	timeOnly, err := trackerConfigFor(l5, "", []string{ExperimentCoastTimeInflation, ExperimentReacquisitionGuard}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := shipped
	want.OcclusionContinuity = continuityWith(func(o *l5tracks.OcclusionContinuityConfig) {
		o.CaptureTimeInflation = true
		o.ReacquisitionGuard = true
	})
	if timeOnly != want {
		t.Errorf("time-only continuity options differ:\n got %+v\nwant %+v", timeOnly.OcclusionContinuity, want.OcclusionContinuity)
	}
}

// continuityWith is DefaultOcclusionContinuity's starting values with every
// switch off except those set.
func continuityWith(set func(*l5tracks.OcclusionContinuityConfig)) l5tracks.OcclusionContinuityConfig {
	oc := l5tracks.DefaultOcclusionContinuity()
	oc.ExplainAbsence, oc.CaptureTimeInflation, oc.ClassCoastBounds, oc.ReacquisitionGuard = false, false, false, false
	set(&oc)
	return oc
}

// near_edge_track runs the solid body it names from full members, reaches
// NearEdgeTracking and nothing else, and refuses what it would silently change
// the meaning of.
func TestNearEdgeTrackReachesItsOptionAndRefusesWhatItCannotCarry(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	arm := []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack}
	want, err := trackerConfigFor(l5, "", arm[:2], nil)
	if err != nil {
		t.Fatal(err)
	}
	want.NearEdgeTracking = true
	got, err := trackerConfigFor(l5, "", arm, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("near_edge_track:\n got %+v\nwant %+v", got, want)
	}
	for name, experiments := range map[string][]string{
		"without solid_body":   {ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack},
		"without full members": {ExperimentSolidBody, ExperimentNearEdgeTrack},
		"alone":                {ExperimentNearEdgeTrack},
		"with adaptive noise":  append([]string{ExperimentAdaptiveUncertainty}, arm...),
		"with likelihood cost": append([]string{ExperimentLikelihoodCost}, arm...),
	} {
		if _, err := trackerConfigFor(l5, "", experiments, nil); err == nil {
			t.Errorf("near_edge_track %s was accepted", name)
		}
	}
	if _, err := trackerConfigFor(l5, l5tracks.MeasurementOBBCentreV1, arm, nil); err == nil {
		t.Error("near_edge_track was accepted with the OBB-centre position model")
	}
	// The robust likelihood cost reads the gate's own S, A2's included, so
	// unlike likelihood_cost it combines, and reaches its own option only.
	{
		robust := want
		robust.RobustLikelihoodAssociationCost = true
		if got, err := trackerConfigFor(l5, "", append([]string{ExperimentRobustLikelihoodCost}, arm...), nil); err != nil || got != robust {
			t.Errorf("near_edge_track with the robust likelihood cost (%v):\n got %+v\nwant %+v", err, got, robust)
		}
	}
	if _, err := trackerConfigFor(l5, l5tracks.MeasurementMedoidV0, arm, nil); err != nil {
		t.Errorf("near_edge_track refused the medoid position model: %v", err)
	}
	// The smoother ends a chain at each reference change (S2.4), so it
	// combines, and observing the tracker changes none of its options.
	if got, err := trackerConfigFor(l5, "", append([]string{ExperimentFixedLagRTS}, arm...), nil); err != nil || got != want {
		t.Errorf("near_edge_track with the smoother (%v):\n got %+v\nwant %+v", err, got, want)
	}
	// A1 qualifies near_edge_track: it reaches its own option and nothing
	// else, and is refused alone.
	want.NearEdgeMedoidGate = true
	if got, err := trackerConfigFor(l5, "", append([]string{ExperimentNearEdgeTrackA1}, arm...), nil); err != nil || got != want {
		t.Errorf("near_edge_track_a1 (%v):\n got %+v\nwant %+v", err, got, want)
	}
	if _, err := trackerConfigFor(l5, "", append([]string{ExperimentNearEdgeTrackA1}, arm[:2]...), nil); err == nil {
		t.Error("near_edge_track_a1 was accepted without near_edge_track")
	}
}

func TestNearEdgeTrackRefusesTheUncertaintyReport(t *testing.T) {
	_, err := Run(Config{
		PCAPFile: "unused.pcap", OutDir: t.TempDir(), UncertaintyReport: true,
		Experiments: []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack},
	})
	if err == nil || !strings.Contains(err.Error(), "uncertainty report") {
		t.Fatalf("got %v, want the uncertainty-report refusal", err)
	}
}

func TestEstimateRowsNameTheModelThatUpdatedThem(t *testing.T) {
	for _, tc := range []struct {
		experiments []string
		mode        l5tracks.MeasurementSource
		want        l5tracks.MeasurementSource
	}{
		{nil, "", l5tracks.MeasurementMedoidV0},
		{nil, l5tracks.MeasurementOBBCentreV1, l5tracks.MeasurementOBBCentreV1},
		{[]string{ExperimentSolidBody}, "", l5tracks.MeasurementMedoidV0},
		{[]string{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack}, "", l5tracks.MeasurementNearEdgeCandidateV1},
	} {
		if got := stateObservationModelFor(tc.experiments, false, tc.mode); got != string(tc.want) {
			t.Errorf("%v with mode %q: rows name %s, want %s", tc.experiments, tc.mode, got, tc.want)
		}
	}
}

func TestApplyBackgroundExperimentsReachTheirOwnSwitch(t *testing.T) {
	newBM := func(t *testing.T) *l3grid.BackgroundManager {
		t.Helper()
		return l3grid.NewBackgroundManager(t.Name(), 4, 16, l3grid.BackgroundParams{}, nil)
	}
	cases := []struct {
		names         []string
		disable       bool
		scope         l3grid.RegionOverrideScope
		supportFilter bool
		cellTercile   bool
		windowCap     float64
	}{
		{nil, false, l3grid.RegionOverrideScopeAll, false, false, 0},
		{[]string{ExperimentNoRegionOverrides}, true, l3grid.RegionOverrideScopeAll, false, false, 0},
		{[]string{ExperimentRegionOverridesVolatileOnly}, false, l3grid.RegionOverrideScopeVolatile, false, false, 0},
		{[]string{ExperimentRegionOverridesQuietOnly}, false, l3grid.RegionOverrideScopeQuiet, false, false, 0},
		{[]string{ExperimentSupportFilter}, false, l3grid.RegionOverrideScopeAll, true, false, 0},
		{[]string{ExperimentRegionOverridesQuietOnly, ExperimentSupportFilter}, false, l3grid.RegionOverrideScopeQuiet, true, false, 0},
		{[]string{ExperimentRegionOverridesCellTercile}, false, l3grid.RegionOverrideScopeAll, false, true, 0},
		{[]string{ExperimentVolatileWindowCap1m}, false, l3grid.RegionOverrideScopeAll, false, false, 1},
		{[]string{ExperimentVolatileWindowCap2m}, false, l3grid.RegionOverrideScopeAll, false, false, 2},
		// The l3bg0010 levers combine with each other, the scopes they
		// still mean something under, and the support filter.
		{[]string{ExperimentRegionOverridesCellTercile, ExperimentVolatileWindowCap1m}, false, l3grid.RegionOverrideScopeAll, false, true, 1},
		{[]string{ExperimentRegionOverridesCellTercile, ExperimentRegionOverridesQuietOnly}, false, l3grid.RegionOverrideScopeQuiet, false, true, 0},
		{[]string{ExperimentRegionOverridesCellTercile, ExperimentRegionOverridesVolatileOnly, ExperimentVolatileWindowCap2m, ExperimentSupportFilter},
			false, l3grid.RegionOverrideScopeVolatile, true, true, 2},
		// Tracker experiments are not the background's to apply.
		{[]string{ExperimentCascade}, false, l3grid.RegionOverrideScopeAll, false, false, 0},
	}
	for _, c := range cases {
		bm := newBM(t)
		if err := ApplyBackgroundExperiments(bm, c.names); err != nil {
			t.Fatalf("%v: %v", c.names, err)
		}
		p := bm.Grid.Params
		if p.DisableRegionOverrides != c.disable || p.RegionOverrideScope != c.scope || (p.SupportFilter != nil) != c.supportFilter ||
			p.RegionOverridesByCellTercile != c.cellTercile || p.VolatileWindowCapMetres != c.windowCap {
			t.Errorf("%v: disable=%v scope=%v supportFilter=%v cellTercile=%v windowCap=%v, want %v %v %v %v %v", c.names,
				p.DisableRegionOverrides, p.RegionOverrideScope, p.SupportFilter != nil, p.RegionOverridesByCellTercile, p.VolatileWindowCapMetres,
				c.disable, c.scope, c.supportFilter, c.cellTercile, c.windowCap)
		}
	}
	for _, refused := range [][]string{
		{ExperimentNoRegionOverrides, ExperimentRegionOverridesVolatileOnly},
		{ExperimentNoRegionOverrides, ExperimentRegionOverridesQuietOnly},
		{ExperimentRegionOverridesVolatileOnly, ExperimentRegionOverridesQuietOnly},
		// Two settings of the cap.
		{ExperimentVolatileWindowCap1m, ExperimentVolatileWindowCap2m},
		// Options that would change nothing under the overrides they are
		// named with, and so run as another arm under their names.
		{ExperimentNoRegionOverrides, ExperimentRegionOverridesCellTercile},
		{ExperimentNoRegionOverrides, ExperimentVolatileWindowCap1m},
		{ExperimentRegionOverridesQuietOnly, ExperimentVolatileWindowCap2m},
		{ExperimentRegionOverridesCellTercile, ExperimentRegionOverridesQuietOnly, ExperimentVolatileWindowCap1m},
	} {
		if err := ApplyBackgroundExperiments(newBM(t), refused); err == nil {
			t.Errorf("%v was accepted", refused)
		}
	}
	background := map[string]bool{ExperimentNoRegionOverrides: true, ExperimentRegionOverridesVolatileOnly: true,
		ExperimentRegionOverridesQuietOnly: true, ExperimentSupportFilter: true, ExperimentRegionOverridesCellTercile: true,
		ExperimentVolatileWindowCap1m: true, ExperimentVolatileWindowCap2m: true, ExperimentSettlingHonest: true,
		ExperimentSettlingRelative0075: true}
	var wantBackground []string
	for _, name := range KnownExperiments() {
		if IsBackgroundExperiment(name) != background[name] {
			t.Errorf("IsBackgroundExperiment(%s) = %v, want %v", name, !background[name], background[name])
		}
		if background[name] {
			wantBackground = append(wantBackground, name)
		}
	}
	if got := BackgroundExperiments(); !reflect.DeepEqual(got, wantBackground) {
		t.Errorf("BackgroundExperiments() = %v, want %v", got, wantBackground)
	}
}

// A solid body switched on by the tuning file's block runs as surely as one
// named as an experiment, and a runner that summarises only the named one
// drops the summary from every arm run from a parameter set.
func TestSolidBodyEnabledReadsTheTuningBlockAndTheExperiment(t *testing.T) {
	shipped := config.MustLoadDefaultConfig()
	if solidBodyEnabled(shipped, nil) {
		t.Error("the shipped defaults run the solid body")
	}
	if !solidBodyEnabled(shipped, []string{ExperimentSolidBody}) {
		t.Error("the solid_body experiment was not read")
	}
	withBlock := config.MustLoadDefaultConfig()
	withBlock.L5.CvKfV1.SolidBody = &config.L5SolidBody{Enabled: true}
	if !solidBodyEnabled(withBlock, nil) {
		t.Error("a tuning block with enabled true was not read")
	}
	withBlock.L5.CvKfV1.SolidBody.Enabled = false
	if solidBodyEnabled(withBlock, nil) {
		t.Error("a tuning block with enabled false was read as on")
	}
	if solidBodyEnabled(nil, nil) {
		t.Error("a nil config was read as on")
	}
}

// Each background experiment reports a manager it cannot apply to rather than
// running as the baseline under its name.
func TestApplyBackgroundExperimentsReportsAManagerItCannotSet(t *testing.T) {
	for _, name := range []string{ExperimentNoRegionOverrides, ExperimentRegionOverridesVolatileOnly,
		ExperimentRegionOverridesQuietOnly, ExperimentSupportFilter, ExperimentRegionOverridesCellTercile,
		ExperimentVolatileWindowCap1m, ExperimentVolatileWindowCap2m} {
		err := ApplyBackgroundExperiments(nil, []string{name})
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s on a nil manager: %v, want an error naming it", name, err)
		}
	}
}

// Contradictory override settings are refused before any capture is read.
func TestRunRefusesContradictoryOverrideSettingsBeforeReadingACapture(t *testing.T) {
	_, err := Run(Config{
		PCAPFile: "missing.pcap", OutDir: filepath.Join(t.TempDir(), "out"), UDPPort: 2369,
		Experiments: []string{ExperimentRegionOverridesQuietOnly, ExperimentNoRegionOverrides},
	})
	if err == nil || !strings.Contains(err.Error(), "settings of one option") {
		t.Fatalf("got %v, want the contradiction refused before the missing capture is read", err)
	}
	if err := ValidateBackgroundExperiments([]string{ExperimentRegionOverridesVolatileOnly, ExperimentSupportFilter}); err != nil {
		t.Errorf("one scope with the support filter refused: %v", err)
	}
	_, err = Run(Config{
		PCAPFile: "missing.pcap", OutDir: filepath.Join(t.TempDir(), "out"), UDPPort: 2369,
		Experiments: []string{ExperimentVolatileWindowCap2m, ExperimentVolatileWindowCap1m},
	})
	if err == nil || !strings.Contains(err.Error(), "two settings of one option") {
		t.Fatalf("got %v, want both window caps refused before the missing capture is read", err)
	}
}

func TestOverlapEndSettingsAreExclusive(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	if _, err := trackerConfigFor(l5, "", []string{ExperimentOverlapEnd1, ExperimentOverlapEnd3}, nil); err == nil {
		t.Fatal("both overlap end settings were accepted")
	}
	// The qualifiers need the overlap end, and reach their own fields.
	for _, q := range []string{ExperimentOverlapEndBody, ExperimentOverlapEndOlder} {
		if _, err := trackerConfigFor(l5, "", []string{q}, nil); err == nil {
			t.Errorf("%s was accepted without the overlap end", q)
		}
	}
	got, err := trackerConfigFor(l5, "", []string{ExperimentOverlapEnd1, ExperimentOverlapEndBody, ExperimentOverlapEndOlder}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.OverlapEndFrames != 1 || !got.OverlapEndOnBody || !got.OverlapEndKeepOlder {
		t.Fatalf("qualified overlap end: frames %d body %v older %v", got.OverlapEndFrames, got.OverlapEndOnBody, got.OverlapEndKeepOlder)
	}
}

// birth_gate_body reads the overlap end's on-body test: refused without it,
// and with it reaches TrackerConfig.BirthGateOnBody.
func TestBirthGateNeedsTheOnBodyTest(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	if _, err := trackerConfigFor(l5, "", []string{ExperimentOverlapEnd1, ExperimentBirthGateBody}, nil); err == nil {
		t.Fatal("birth_gate_body was accepted without overlap_end_body")
	}
	got, err := trackerConfigFor(l5, "", []string{ExperimentOverlapEnd1, ExperimentOverlapEndBody, ExperimentBirthGateBody}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.BirthGateOnBody || !got.OverlapEndOnBody {
		t.Fatalf("birth gate: gate %v body %v", got.BirthGateOnBody, got.OverlapEndOnBody)
	}
	off, err := trackerConfigFor(l5, "", []string{ExperimentOverlapEnd1, ExperimentOverlapEndBody}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if off.BirthGateOnBody {
		t.Fatal("the birth gate is on without its name")
	}
}

// robust_birth_gate is the birth rule for the robust likelihood cost's
// population: refused without that cost, and with it reaches
// TrackerConfig.FragmentBirthGate alone.
func TestRobustBirthGateNeedsTheRobustCost(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	_, err := trackerConfigFor(l5, "", []string{ExperimentRobustBirthGate}, nil)
	if err == nil || !strings.Contains(err.Error(), ExperimentRobustLikelihoodCost) {
		t.Fatalf("robust_birth_gate without robust_likelihood_cost gave %v, want the refusal naming it", err)
	}
	if _, err := trackerConfigFor(l5, "", []string{ExperimentLikelihoodCost, ExperimentRobustBirthGate}, nil); err == nil {
		t.Fatal("robust_birth_gate was accepted with the Gaussian likelihood cost")
	}
	got, err := trackerConfigFor(l5, "", []string{ExperimentRobustLikelihoodCost, ExperimentRobustBirthGate}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.FragmentBirthGate || !got.RobustLikelihoodAssociationCost {
		t.Fatalf("robust birth gate: gate %v robust %v", got.FragmentBirthGate, got.RobustLikelihoodAssociationCost)
	}
	if got.BirthGateOnBody || got.OverlapEndFrames != 0 || got.TentativePriority || got.CascadedAssociation {
		t.Fatalf("robust_birth_gate switched on another identity option: %+v", got)
	}
	off, err := trackerConfigFor(l5, "", []string{ExperimentRobustLikelihoodCost}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if off.FragmentBirthGate {
		t.Fatal("the fragment birth gate is on without its name")
	}
	if names, err := NormaliseExperiments([]string{ExperimentRobustBirthGate, ExperimentRobustLikelihoodCost}); err != nil || len(names) != 2 {
		t.Fatalf("robust_birth_gate is not a known experiment: %v %v", names, err)
	}
}

// settling_honest is a background experiment, so lidar-bench accepts it, and
// it sets the first-check semantics on the manager it is applied to.
func TestSettlingHonestExperiment(t *testing.T) {
	names, err := NormaliseExperiments([]string{ExperimentSettlingHonest})
	if err != nil {
		t.Fatalf("NormaliseExperiments: %v", err)
	}
	if !IsBackgroundExperiment(ExperimentSettlingHonest) {
		t.Error("settling_honest should be a background experiment")
	}
	bm := l3grid.NewBackgroundManagerDI("settling-honest", 2, 4, l3grid.BackgroundParams{}, nil)
	if bm.GetParams().SettlingHonestFirstCheck {
		t.Fatal("the default is the passing first check")
	}
	if err := ApplyBackgroundExperiments(bm, names); err != nil {
		t.Fatalf("ApplyBackgroundExperiments: %v", err)
	}
	if !bm.GetParams().SettlingHonestFirstCheck {
		t.Error("settling_honest did not set SettlingHonestFirstCheck")
	}
}

// settling_relative_0075 is a background experiment that sets the ratio
// l3bg0030 fixed.
func TestSettlingRelativeExperiment(t *testing.T) {
	names, err := NormaliseExperiments([]string{ExperimentSettlingRelative0075})
	if err != nil {
		t.Fatalf("NormaliseExperiments: %v", err)
	}
	if !IsBackgroundExperiment(ExperimentSettlingRelative0075) {
		t.Error("settling_relative_0075 should be a background experiment")
	}
	bm := l3grid.NewBackgroundManagerDI("settling-relative", 2, 4, l3grid.BackgroundParams{}, nil)
	if bm.GetParams().SettlingRelativeSpreadRatio != 0 {
		t.Fatal("the default judges the absolute delta")
	}
	if err := ApplyBackgroundExperiments(bm, names); err != nil {
		t.Fatalf("ApplyBackgroundExperiments: %v", err)
	}
	if got := bm.GetParams().SettlingRelativeSpreadRatio; got != 0.075 {
		t.Errorf("settling_relative_0075 set the ratio to %v", got)
	}
}

// box_extent_solid_body reaches its own option, on top of the solid body
// with its extent state, by either name for that state; and it is refused
// without the box model, without the solid body, and without the extent
// state, each of which would leave it nothing to change.
func TestBoxExtentSolidBodyNeedsTheBoxAndTheExtentState(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	for _, state := range []string{ExperimentSolidBodyExtentState, ExperimentSolidBodyFaceState} {
		base, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, state}, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := base
		want.BoxModel.BoxExtentFromSolidBody = true
		got, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, state, ExperimentBoxExtentSolidBody}, nil)
		if err != nil {
			t.Fatalf("%s with %s: %v", ExperimentBoxExtentSolidBody, state, err)
		}
		if got != want {
			t.Errorf("%s with %s:\n got %+v\nwant %+v", ExperimentBoxExtentSolidBody, state, got, want)
		}
	}
	for _, without := range [][]string{
		{ExperimentBoxExtentSolidBody},
		{ExperimentSolidBody, ExperimentBoxExtentSolidBody},
		{ExperimentSolidBodyExtentState, ExperimentBoxExtentSolidBody},
		{ExperimentSolidBody, ExperimentSolidBodyCensoredExtent, ExperimentBoxExtentSolidBody},
	} {
		if _, err := trackerConfigFor(l5, "", without, nil); err == nil || !strings.Contains(err.Error(), ExperimentBoxExtentSolidBody) {
			t.Errorf("%q: want a refusal naming %s, got %v", without, ExperimentBoxExtentSolidBody, err)
		}
	}
	noBox := config.MustLoadDefaultConfig().L5.CvKfV1
	noBox.BoxModel = nil
	if _, err := trackerConfigFor(noBox, "", []string{ExperimentSolidBody, ExperimentSolidBodyExtentState, ExperimentBoxExtentSolidBody}, nil); err == nil ||
		!strings.Contains(err.Error(), "box model") {
		t.Errorf("with the box model off: want a refusal naming the box model, got %v", err)
	}
	// The box it draws is the recording, so a tool without an evidence
	// database may run it.
	if err := RefuseExperimentsWithout("some-tool", []string{ExperimentSolidBody, ExperimentSolidBodyExtentState, ExperimentBoxExtentSolidBody}, ReplayInputs{}); err != nil {
		t.Errorf("refused without an evidence database: %v", err)
	}
}

// solid_body_shared_orientation combines with the face state and with the
// box's extent from the solid body, and is refused where it would not be the
// arm it names: without the box model, on the tracked filter, and beside
// another source of the same orientation or face axis.
func TestSolidBodySharedOrientationRefusals(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	got, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodyFaceState,
		ExperimentSolidBodySharedOrientation, ExperimentBoxExtentSolidBody}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.SolidBody.SharedOrientation || !got.SolidBody.ExtentState || !got.BoxModel.BoxExtentFromSolidBody {
		t.Errorf("the shared orientation with the face state and the box extent: %+v %+v", got.SolidBody, got.BoxModel)
	}
	for _, other := range []string{ExperimentSolidBodyRectangleHeading, ExperimentSolidBodyCourseHeading, ExperimentSolidBodyCourseFaces} {
		if _, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodySharedOrientation, other}, nil); err == nil ||
			!strings.Contains(err.Error(), ExperimentSolidBodySharedOrientation) {
			t.Errorf("with %s: want a refusal naming %s, got %v", other, ExperimentSolidBodySharedOrientation, err)
		}
	}
	if _, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack,
		ExperimentSolidBodySharedOrientation}, nil); err == nil || !strings.Contains(err.Error(), ExperimentNearEdgeTrack) {
		t.Errorf("with %s: want a refusal naming it, got %v", ExperimentNearEdgeTrack, err)
	}
	noBox := config.MustLoadDefaultConfig().L5.CvKfV1
	noBox.BoxModel = nil
	if _, err := trackerConfigFor(noBox, "", []string{ExperimentSolidBody, ExperimentSolidBodySharedOrientation}, nil); err == nil ||
		!strings.Contains(err.Error(), "box model") {
		t.Errorf("with the box model off: want a refusal naming the box model, got %v", err)
	}
	if err := ValidateExperiments([]string{ExperimentSolidBodySharedOrientation}, "", "", nil); err == nil {
		t.Errorf("%s was accepted without %s", ExperimentSolidBodySharedOrientation, ExperimentSolidBody)
	}
}

// box_extent_shared_axis sets only its own option over the shared
// orientation and the box extent, and is refused without either, under
// which it would run as the arm the other name gives.
func TestBoxExtentSharedAxisNeedsTheBoxExtentAndTheSharedOrientation(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	stack := []string{ExperimentSolidBody, ExperimentSolidBodyFaceState, ExperimentSolidBodySharedOrientation, ExperimentBoxExtentSolidBody}
	base, err := trackerConfigFor(l5, "", stack, nil)
	if err != nil {
		t.Fatal(err)
	}
	if base.BoxModel.BodyExtentSharedAxis {
		t.Fatal("the stack without the name read the shared axis")
	}
	want := base
	want.BoxModel.BodyExtentSharedAxis = true
	got, err := trackerConfigFor(l5, "", append(append([]string{}, stack...), ExperimentBoxExtentSharedAxis), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("%s:\n got %+v\nwant %+v", ExperimentBoxExtentSharedAxis, got, want)
	}
	for _, without := range [][]string{
		{ExperimentSolidBody, ExperimentSolidBodyFaceState, ExperimentBoxExtentSolidBody, ExperimentBoxExtentSharedAxis},
		{ExperimentSolidBody, ExperimentSolidBodyFaceState, ExperimentSolidBodySharedOrientation, ExperimentBoxExtentSharedAxis},
		{ExperimentBoxExtentSharedAxis},
	} {
		if _, err := trackerConfigFor(l5, "", without, nil); err == nil || !strings.Contains(err.Error(), ExperimentBoxExtentSharedAxis) {
			t.Errorf("%q: want a refusal naming %s, got %v", without, ExperimentBoxExtentSharedAxis, err)
		}
	}
}

// solid_body_extent_accrual qualifies the extent state's span bounds: it
// combines with the face state, with the extent state and either source of
// bounds, and with near_edge_track, and is refused where there are no
// bounds to accrue, so it cannot run as the arm it qualifies under its name.
func TestSolidBodyExtentAccrualNeedsTheExtentStateBounds(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	for _, accepted := range [][]string{
		{ExperimentSolidBody, ExperimentSolidBodyFaceState, ExperimentSolidBodyExtentAccrual},
		{ExperimentSolidBody, ExperimentSolidBodyExtentState, ExperimentSolidBodyCensoredExtent, ExperimentSolidBodyExtentAccrual},
		{ExperimentSolidBody, ExperimentSolidBodyExtentState, ExperimentSolidBodySpanBoundsBelief, ExperimentSolidBodyExtentAccrual},
		{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack, ExperimentSolidBodyFaceState, ExperimentSolidBodyExtentAccrual},
	} {
		got, err := trackerConfigFor(l5, "", accepted, nil)
		if err != nil {
			t.Errorf("%q: %v", accepted, err)
			continue
		}
		if !got.SolidBody.ExtentBoundAccrual {
			t.Errorf("%q did not switch ExtentBoundAccrual on", accepted)
		}
	}
	got, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody, ExperimentSolidBodyFaceState}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.SolidBody.ExtentBoundAccrual {
		t.Error("solid_body_face_state alone switched ExtentBoundAccrual on")
	}
	for _, refused := range [][]string{
		{ExperimentSolidBody, ExperimentSolidBodyExtentAccrual},
		{ExperimentSolidBody, ExperimentSolidBodyExtentState, ExperimentSolidBodyExtentAccrual},
		{ExperimentSolidBody, ExperimentSolidBodyCensoredExtent, ExperimentSolidBodySpanBoundsBelief, ExperimentSolidBodyExtentAccrual},
	} {
		if _, err := trackerConfigFor(l5, "", refused, nil); err == nil || !strings.Contains(err.Error(), ExperimentSolidBodyExtentAccrual) {
			t.Errorf("%q: want a refusal naming %s, got %v", refused, ExperimentSolidBodyExtentAccrual, err)
		}
	}
	if _, err := trackerConfigFor(l5, "", []string{ExperimentSolidBodyFaceState, ExperimentSolidBodyExtentAccrual}, nil); err == nil ||
		!strings.Contains(err.Error(), ExperimentSolidBody) {
		t.Errorf("without %s: want a refusal naming it, got %v", ExperimentSolidBody, err)
	}
}

// box_ground stands the box model's box on the ground, so without the box
// model it would change nothing and run as the baseline under its name.
func TestBoxGroundNeedsTheBoxModel(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	l5.BoxModel = nil
	if _, err := trackerConfigFor(l5, "", []string{ExperimentBoxGround}, nil); err == nil {
		t.Fatal("box_ground was accepted with the box model off")
	}
}

// The ground specs' arms name surface_ground and box_ground as replay
// options, which the runner passes to -experiment; both must parse.
func TestGroundSpecsReplayOptionsParse(t *testing.T) {
	got, err := ParseExperiments("surface_ground,box_ground")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{ExperimentBoxGround, ExperimentSurfaceGround}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// robust_likelihood_cost and likelihood_cost are two assignment costs; the
// tracker would run the robust one alone, so naming both, or naming the
// robust one over a tuning file that sets the Gaussian, is refused with the
// reason rather than resolved.
func TestRobustLikelihoodCostRefusesTheGaussianLikelihoodCost(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	_, err := trackerConfigFor(l5, "", []string{ExperimentLikelihoodCost, ExperimentRobustLikelihoodCost}, nil)
	if err == nil {
		t.Fatal("likelihood_cost and robust_likelihood_cost were accepted together")
	}
	for _, want := range []string{ExperimentRobustLikelihoodCost, ExperimentLikelihoodCost, "name one"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not say %q", err, want)
		}
	}
	tuned := *l5
	tuned.Association = &config.L5Association{LikelihoodCost: true}
	if _, err := trackerConfigFor(&tuned, "", []string{ExperimentRobustLikelihoodCost}, nil); err == nil {
		t.Error("robust_likelihood_cost was accepted over a tuning file with association.likelihood_cost")
	}
	if got, err := trackerConfigFor(&tuned, "", nil, nil); err != nil || !got.LikelihoodAssociationCost || got.RobustLikelihoodAssociationCost {
		t.Errorf("the tuning file's likelihood_cost alone (%v): %+v", err, got)
	}
	// The composites the option is meant to be measured with are accepted.
	for _, with := range [][]string{
		{ExperimentTentativePriority},
		{ExperimentCascade},
		{ExperimentContestedRejoin},
		{ExperimentOverlapEnd1, ExperimentOverlapEndBody, ExperimentOverlapEndOlder, ExperimentBirthGateBody},
		{ExperimentAdaptiveUncertainty},
	} {
		got, err := trackerConfigFor(l5, "", append([]string{ExperimentRobustLikelihoodCost}, with...), nil)
		if err != nil || !got.RobustLikelihoodAssociationCost {
			t.Errorf("robust_likelihood_cost with %v (%v): %+v", with, err, got)
		}
	}
}
