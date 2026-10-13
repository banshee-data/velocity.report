package replayeval

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
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
	want := []string{ExperimentAdaptiveUncertainty, ExperimentCaptureGapPredict, ExperimentCascade, ExperimentClassCoastBounds,
		ExperimentCoastSupport, ExperimentCoastTimeInflation, ExperimentDensityCap, ExperimentDwellPolicy, ExperimentFixedLagRTS, ExperimentFlipRule,
		ExperimentLikelihoodCost, ExperimentMeasurementTime, ExperimentNearEdgeTrack, ExperimentNearEdgeTrackA1,
		ExperimentNoRegionOverrides,
		ExperimentOcclusionContinuity, ExperimentReacquisitionGuard, ExperimentRegionOverridesQuietOnly, ExperimentRegionOverridesVolatileOnly, ExperimentSolidBody, ExperimentSolidBodyContainment,
		ExperimentSolidBodyCourseFaces,
		ExperimentSolidBodyCourseHeading, ExperimentSolidBodyEndFaceCentring,
		ExperimentSolidBodyEndFaceCentringOpenPrior, ExperimentSolidBodyExtentGrowth,
		ExperimentSolidBodyExtentPriorFloor,
		ExperimentSolidBodyFaceConsider, ExperimentSolidBodyFaceHysteresis, ExperimentSolidBodyFacePlaneSpans,
		ExperimentSolidBodyFullMembers,
		ExperimentSolidBodyRankOneMedoid, ExperimentSolidBodyRankOneMedoidTight, ExperimentSolidBodyRectangleCourseFusion,
		ExperimentSolidBodyRectangleFit, ExperimentSolidBodyRectangleHeading, ExperimentSolidBodyRectangleSigmaMid,
		ExperimentSolidBodyRectangleSigmaWide,
		ExperimentSolidBodyReferenceTranslation,
		ExperimentSolidBodyVehicleExtentFloor, ExperimentSupportFilter}
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
		ExperimentLikelihoodCost:    func(c *l5tracks.TrackerConfig) { c.LikelihoodAssociationCost = true },
		ExperimentCascade:           func(c *l5tracks.TrackerConfig) { c.CascadedAssociation = true },
		ExperimentFlipRule:          func(c *l5tracks.TrackerConfig) { c.OBBHeadingFlipRule = true },
		ExperimentMeasurementTime:   func(c *l5tracks.TrackerConfig) { c.MeasurementTimePrediction = true },
		ExperimentCaptureGapPredict: func(c *l5tracks.TrackerConfig) { c.CaptureGapPrediction = true },
		ExperimentCoastTimeInflation: func(c *l5tracks.TrackerConfig) {
			c.OcclusionContinuity = continuityWith(func(o *l5tracks.OcclusionContinuityConfig) { o.CaptureTimeInflation = true })
		},
		ExperimentReacquisitionGuard: func(c *l5tracks.TrackerConfig) {
			c.OcclusionContinuity = continuityWith(func(o *l5tracks.OcclusionContinuityConfig) { o.ReacquisitionGuard = true })
		},
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
	for _, name := range []string{ExperimentDensityCap, ExperimentDwellPolicy, ExperimentNoRegionOverrides, ExperimentRegionOverridesVolatileOnly, ExperimentRegionOverridesQuietOnly, ExperimentSupportFilter, ExperimentFixedLagRTS} {
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
	}{
		{nil, false, l3grid.RegionOverrideScopeAll, false},
		{[]string{ExperimentNoRegionOverrides}, true, l3grid.RegionOverrideScopeAll, false},
		{[]string{ExperimentRegionOverridesVolatileOnly}, false, l3grid.RegionOverrideScopeVolatile, false},
		{[]string{ExperimentRegionOverridesQuietOnly}, false, l3grid.RegionOverrideScopeQuiet, false},
		{[]string{ExperimentSupportFilter}, false, l3grid.RegionOverrideScopeAll, true},
		{[]string{ExperimentRegionOverridesQuietOnly, ExperimentSupportFilter}, false, l3grid.RegionOverrideScopeQuiet, true},
		// Tracker experiments are not the background's to apply.
		{[]string{ExperimentCascade}, false, l3grid.RegionOverrideScopeAll, false},
	}
	for _, c := range cases {
		bm := newBM(t)
		if err := ApplyBackgroundExperiments(bm, c.names); err != nil {
			t.Fatalf("%v: %v", c.names, err)
		}
		p := bm.Grid.Params
		if p.DisableRegionOverrides != c.disable || p.RegionOverrideScope != c.scope || (p.SupportFilter != nil) != c.supportFilter {
			t.Errorf("%v: disable=%v scope=%v supportFilter=%v, want %v %v %v", c.names,
				p.DisableRegionOverrides, p.RegionOverrideScope, p.SupportFilter != nil,
				c.disable, c.scope, c.supportFilter)
		}
	}
	for _, pair := range [][]string{
		{ExperimentNoRegionOverrides, ExperimentRegionOverridesVolatileOnly},
		{ExperimentNoRegionOverrides, ExperimentRegionOverridesQuietOnly},
		{ExperimentRegionOverridesVolatileOnly, ExperimentRegionOverridesQuietOnly},
	} {
		if err := ApplyBackgroundExperiments(newBM(t), pair); err == nil {
			t.Errorf("%v was accepted", pair)
		}
	}
	background := map[string]bool{ExperimentNoRegionOverrides: true, ExperimentRegionOverridesVolatileOnly: true,
		ExperimentRegionOverridesQuietOnly: true, ExperimentSupportFilter: true}
	for _, name := range KnownExperiments() {
		if IsBackgroundExperiment(name) != background[name] {
			t.Errorf("IsBackgroundExperiment(%s) = %v, want %v", name, !background[name], background[name])
		}
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
		ExperimentRegionOverridesQuietOnly, ExperimentSupportFilter} {
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
}
