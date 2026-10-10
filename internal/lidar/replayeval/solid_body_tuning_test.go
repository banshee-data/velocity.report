package replayeval

import (
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// candidateBlock is the geometry convergence candidate as a tuning block:
// A2 with the rectangle heading, fusion, the fit's sigma at 1.5,
// containment, growth admission, open-prior centring and the vehicle floor.
func candidateBlock() *config.L5SolidBody {
	return &config.L5SolidBody{
		Enabled: true, FullMembers: true, NearEdgeTracking: true,
		FaceHysteresis: true, CourseAlignedFaces: true,
		ExtentGrowthAdmission: true, VehicleExtentFloor: true,
		EndFaceCentring: true, EndFaceCentringOpenPrior: true, Containment: true,
		RectangleHeading: true, RectangleCourseFusion: true, RectangleSigmaScale: 1.5,
	}
}

// candidateExperiments is the same candidate as the replay names it.
var candidateExperiments = []string{
	ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack,
	ExperimentSolidBodyFaceHysteresis, ExperimentSolidBodyCourseFaces,
	ExperimentSolidBodyExtentGrowth, ExperimentSolidBodyVehicleExtentFloor,
	ExperimentSolidBodyEndFaceCentringOpenPrior, ExperimentSolidBodyContainment,
	ExperimentSolidBodyRectangleHeading, ExperimentSolidBodyRectangleCourseFusion,
	ExperimentSolidBodyRectangleSigmaMid,
}

func l5WithBlock(sb *config.L5SolidBody) *config.L5CvKfV1 {
	l5 := *config.MustLoadDefaultConfig().L5.CvKfV1
	l5.SolidBody = sb
	return &l5
}

// The experiment names are aliases for the block: the candidate written as
// a tuning block and the candidate named as experiments give one tracker
// configuration, origin included, and the block alone needs no experiment.
func TestSolidBodyBlockAndExperimentsAreOneConfiguration(t *testing.T) {
	fromBlock, err := trackerConfigFor(l5WithBlock(candidateBlock()), "", nil, nil)
	if err != nil {
		t.Fatalf("the block alone: %v", err)
	}
	fromNames, err := trackerConfigFor(config.MustLoadDefaultConfig().L5.CvKfV1, "", candidateExperiments, nil)
	if err != nil {
		t.Fatalf("the names alone: %v", err)
	}
	if fromBlock != fromNames {
		t.Fatalf("the block and the names differ:\n block %+v\n names %+v", fromBlock, fromNames)
	}
	if !fromBlock.SolidBody.Enabled || !fromBlock.NearEdgeTracking || fromBlock.SolidBody.OriginSource != OriginTrackingTransformIdentity ||
		fromBlock.SolidBody.RectangleSigmaScale != 1.5 || !fromBlock.SolidBody.EndFaceCentringOpenPrior {
		t.Fatalf("the candidate did not reach the tracker: %+v", fromBlock)
	}
	// Both names and block: the names add to the block, and a scale named
	// by experiment replaces the block's.
	both, err := trackerConfigFor(l5WithBlock(candidateBlock()), "", []string{ExperimentSolidBodyRectangleFit, ExperimentSolidBodyRectangleSigmaWide}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !both.SolidBody.RectangleFit || both.SolidBody.RectangleSigmaScale != 2 || !both.SolidBody.RectangleHeading || !both.NearEdgeTracking {
		t.Fatalf("names did not add to the block: %+v", both.SolidBody)
	}
	// A qualifying experiment over a block that is present but disabled
	// is refused as it is without any block.
	off := candidateBlock()
	*off = config.L5SolidBody{}
	if _, err := trackerConfigFor(l5WithBlock(off), "", []string{ExperimentSolidBodyContainment}, nil); err == nil || !strings.Contains(err.Error(), "without "+ExperimentSolidBody) {
		t.Fatalf("a qualifier over a disabled block was accepted: %v", err)
	}
}

// The block's near-edge tracking carries the same refusals as the experiment:
// the noise models A2 does not carry, and the OBB position model.
func TestSolidBodyBlockNearEdgeRefusals(t *testing.T) {
	l5 := l5WithBlock(candidateBlock())
	for _, refused := range []string{ExperimentAdaptiveUncertainty, ExperimentLikelihoodCost} {
		if _, err := trackerConfigFor(l5, "", []string{refused}, nil); err == nil || !strings.Contains(err.Error(), "does not combine") {
			t.Fatalf("%s over a near-edge block was accepted: %v", refused, err)
		}
	}
	if _, err := trackerConfigFor(l5, l5tracks.MeasurementOBBCentreV1, nil, nil); err == nil || !strings.Contains(err.Error(), "medoid position model") {
		t.Fatalf("the OBB model under a near-edge block was accepted: %v", err)
	}
	// A1 named over a block that runs A2 is accepted, as it is over the
	// experiment.
	got, err := trackerConfigFor(l5, "", []string{ExperimentNearEdgeTrackA1}, nil)
	if err != nil || !got.NearEdgeMedoidGate {
		t.Fatalf("A1 over a near-edge block: %v %+v", err, got.NearEdgeMedoidGate)
	}
	// The members and the observation model follow the block too.
	cfg := config.MustLoadDefaultConfig()
	cfg.L5.CvKfV1 = l5
	if !solidBodyFullMembers(cfg.L5.CvKfV1, nil) || !tuningNearEdge(cfg) {
		t.Fatal("the block's members or near-edge tracking were not read")
	}
	if got := stateObservationModelFor(nil, tuningNearEdge(cfg), ""); got != string(l5tracks.MeasurementNearEdgeCandidateV1) {
		t.Fatalf("observation model %q under a near-edge block", got)
	}
	plain := config.MustLoadDefaultConfig()
	if solidBodyFullMembers(plain.L5.CvKfV1, nil) || tuningNearEdge(plain) || tuningNearEdge(nil) {
		t.Fatal("the defaults read as carrying the body")
	}
}
