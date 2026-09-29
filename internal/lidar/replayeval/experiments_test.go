package replayeval

import (
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
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
		ExperimentCoastSupport, ExperimentCoastTimeInflation, ExperimentDensityCap, ExperimentFixedLagRTS, ExperimentFlipRule,
		ExperimentLikelihoodCost, ExperimentMeasurementTime, ExperimentNearEdgeTrack, ExperimentNoRegionOverrides,
		ExperimentOcclusionContinuity, ExperimentReacquisitionGuard, ExperimentSolidBody, ExperimentSolidBodyCourseFaces,
		ExperimentSolidBodyFaceConsider, ExperimentSolidBodyFaceHysteresis, ExperimentSolidBodyFullMembers,
		ExperimentSolidBodyHalfExtents, ExperimentSolidBodyReferenceTranslation}
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
		ExperimentSolidBodyHalfExtents: func(o *l5tracks.SolidBodyOptions) { o.HalfExtentState = true },
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
	for _, name := range []string{ExperimentDensityCap, ExperimentNoRegionOverrides, ExperimentFixedLagRTS} {
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
		"with the smoother":    append([]string{ExperimentFixedLagRTS}, arm...),
		"with half-extents":    append([]string{ExperimentSolidBodyHalfExtents}, arm...),
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
		if got := stateObservationModelFor(tc.experiments, tc.mode); got != string(tc.want) {
			t.Errorf("%v with mode %q: rows name %s, want %s", tc.experiments, tc.mode, got, tc.want)
		}
	}
}
