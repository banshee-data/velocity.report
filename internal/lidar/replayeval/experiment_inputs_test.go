package replayeval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
)

// A tool without a coverage declaration or an evidence database refuses the
// experiments that need one, naming itself, and accepts the rest; a tool that
// has them accepts them.
func TestRefuseExperimentsWithout(t *testing.T) {
	none := ReplayInputs{}
	both := ReplayInputs{ContinuityCoverage: true, ObservationDB: true}
	cases := []struct {
		experiments []string
		have        ReplayInputs
		refused     string
	}{
		{nil, none, ""},
		{[]string{ExperimentDwellPolicy, ExperimentRegionOverridesCellTercile}, none, ""},
		{[]string{ExperimentCoastSupport}, none, "coverage"},
		{[]string{ExperimentClassCoastBounds}, none, "coverage"},
		{[]string{ExperimentOcclusionContinuity}, none, "coverage"},
		{[]string{ExperimentOcclusionContinuity}, ReplayInputs{ContinuityCoverage: true}, ""},
		// The reacquisition guard and capture-time inflation need no declaration.
		{[]string{ExperimentReacquisitionGuard, ExperimentCoastTimeInflation}, none, ""},
		{[]string{ExperimentSolidBody}, none, "evidence database"},
		{[]string{ExperimentSolidBody, ExperimentSolidBodyFaceHysteresis}, none, "evidence database"},
		{[]string{ExperimentSolidBody}, ReplayInputs{ObservationDB: true}, ""},
		// On the tracked filter the solid body changes the tracks, so the
		// recording itself is the arm.
		{[]string{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack}, none, ""},
		{[]string{ExperimentSolidBody, ExperimentCoastSupport}, both, ""},
	}
	for _, c := range cases {
		err := RefuseExperimentsWithout("some-tool", c.experiments, c.have)
		switch {
		case c.refused == "" && err != nil:
			t.Errorf("%v with %+v refused: %v", c.experiments, c.have, err)
		case c.refused != "" && err == nil:
			t.Errorf("%v with %+v accepted, want a refusal about the %s", c.experiments, c.have, c.refused)
		case c.refused != "" && (!strings.Contains(err.Error(), c.refused) || !strings.Contains(err.Error(), "some-tool")):
			t.Errorf("%v: refusal %q should name the %s and the tool", c.experiments, err, c.refused)
		}
	}
}

// ValidateExperiments refuses, without a capture, every combination Run
// refuses: two settings of one option, and a qualifier without what it
// qualifies.
func TestValidateExperimentsRefusesWhatRunRefuses(t *testing.T) {
	accepted := [][]string{
		nil,
		{ExperimentDwellPolicy},
		{ExperimentRegionOverridesCellTercile},
		{ExperimentRegionOverridesCellTercile, ExperimentRegionOverridesVolatileOnly},
		{ExperimentOverlapEnd1, ExperimentOverlapEndBody},
		{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack},
		{ExperimentSolidBody, ExperimentSolidBodyFaceState},
		{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack, ExperimentSolidBodySpanBoundsBelief},
		{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack, ExperimentSolidBodyExtentState},
	}
	for _, experiments := range accepted {
		if err := ValidateExperiments(experiments, "", "", nil); err != nil {
			t.Errorf("%v refused: %v", experiments, err)
		}
	}
	refused := [][]string{
		{ExperimentDwellPolicy, ExperimentDwellPolicyT40},
		{ExperimentNoRegionOverrides, ExperimentRegionOverridesCellTercile},
		{ExperimentNoRegionOverrides, ExperimentRegionOverridesQuietOnly},
		{ExperimentVolatileWindowCap1m, ExperimentVolatileWindowCap2m},
		{ExperimentOverlapEnd1, ExperimentOverlapEnd3},
		{ExperimentOverlapEndBody},
		{ExperimentSolidBodyFaceHysteresis},
		{ExperimentNearEdgeTrack},
		{ExperimentSolidBody, ExperimentSolidBodyRankOneMedoid, ExperimentSolidBodyRankOneMedoidTight},
		{ExperimentSolidBodyFaceState},
		{ExperimentSolidBodyHeadingFaceNoise},
		{ExperimentSolidBodyExtentState},
	}
	for _, experiments := range refused {
		if err := ValidateExperiments(experiments, "", "", nil); err == nil {
			t.Errorf("%v accepted, want the refusal Run gives", experiments)
		}
	}
	// A tuning file that cannot be read as one is refused, as Run refuses
	// it. (A missing one falls back to the embedded defaults, as in Run.)
	malformed := filepath.Join(t.TempDir(), "malformed.json")
	if err := os.WriteFile(malformed, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExperiments([]string{ExperimentDwellPolicy}, malformed, "", nil); err == nil {
		t.Error("a malformed tuning file was accepted")
	}
}

// fragment_join_track_membership widens the fragment join's footprints, so it
// is refused with a tuning file whose join is off, as Run refuses it, and
// accepted with the registered join set.
func TestFragmentJoinTrackMembershipNeedsTheJoin(t *testing.T) {
	membership := []string{ExperimentFragmentJoinTrackMembership}
	err := ValidateExperiments(membership, filepath.Join("..", "..", "..", "config", "tuning.defaults.json"), "", nil)
	if err == nil || !strings.Contains(err.Error(), "fragment_join is not enabled") {
		t.Fatalf("membership without the join: %v", err)
	}
	if err := ValidateExperiments(membership, filepath.Join("..", "..", "..", "config", "experiments", "motion-fragment-join.json"), "", nil); err != nil {
		t.Fatalf("membership with the registered join set refused: %v", err)
	}
	// Without the option the join's own set and the defaults both stand.
	if err := fragmentJoinTrackMembershipRefusal(nil, nil); err != nil {
		t.Fatalf("no option refused: %v", err)
	}
}

// Run refuses the option before it reads a capture.
func TestRunRefusesTrackMembershipWithoutTheJoin(t *testing.T) {
	_, err := Run(Config{
		PCAPFile:    filepath.Join(t.TempDir(), "absent.pcapng"),
		OutDir:      t.TempDir(),
		TuningFile:  filepath.Join("..", "..", "..", "config", "tuning.defaults.json"),
		Experiments: []string{ExperimentFragmentJoinTrackMembership},
	})
	if err == nil || !strings.Contains(err.Error(), "fragment_join is not enabled") {
		t.Fatalf("Run accepted the option without the join: %v", err)
	}
}

// Join records with the join off would be an empty file that reads as "no
// wrong joins"; Run refuses them before it reads a capture.
func TestRunRefusesJoinRecordsWithoutTheJoin(t *testing.T) {
	_, err := Run(Config{
		PCAPFile:            filepath.Join(t.TempDir(), "absent.pcapng"),
		OutDir:              t.TempDir(),
		TuningFile:          filepath.Join("..", "..", "..", "config", "tuning.defaults.json"),
		FragmentJoinRecords: func([]pipeline.FragmentJoinRecord) {},
	})
	if err == nil || !strings.Contains(err.Error(), "fragment join records need") {
		t.Fatalf("Run accepted join records without the join: %v", err)
	}
}
