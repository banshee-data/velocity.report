package perframeeval

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// solidBodyOffsetY is how far the fixture's solid bodies sit from arm A's
// point estimates, so the two arms' MOTP can be told apart by hand.
const solidBodyOffsetY = 0.25

// writeSolidBodies files a solid body beside each of arm A's online point
// estimates, the way the solid_body replay experiment does.
func writeSolidBodies(t *testing.T, path string) {
	t.Helper()
	database, err := db.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := sqlite.NewStateEstimateStore(database.DB)
	points, err := store.ListEstimatePositions(sqlite.EstimateVersion{
		SourceID: evalfixture.SourceID, EstimatorID: evalfixture.EstimatorID,
		ObservationModelID: evalfixture.ObservationModelID, ParamHash: evalfixture.ParamsA, Stage: StageOnline,
	})
	if err != nil || len(points) == 0 {
		t.Fatalf("fixture point estimates: %d, %v", len(points), err)
	}
	for _, p := range points {
		id := fmt.Sprintf("solid_body/%d/%d", p.CreationSequence, p.FrameUnixNanos)
		if err := store.InsertSolidBody(sqlite.TrackSolidBody{
			EstimateID: id, TrackID: p.TrackID, ObservationID: "observation/" + id,
			SourceID: evalfixture.SourceID, CalibrationID: "calibration/v1/fixture",
			FrameUnixNanos: p.FrameUnixNanos, MeasurementUnixNanos: p.FrameUnixNanos,
			EstimatorID: evalfixture.EstimatorID, ObservationModelID: string(l5tracks.MeasurementNearEdgeCandidateV1),
			ParamHash: evalfixture.ParamsA, Stage: StageOnline, CreationSequence: p.CreationSequence,
			Reading: l5tracks.SolidBodyReading{Estimate: l5tracks.SolidBodyEstimate{
				StateModel: l5tracks.StateModelCVCartesianV1, Reference: l5tracks.ReferenceBodyCentre,
				X: p.X, Y: p.Y + solidBodyOffsetY,
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSolidBodyArmScoresAgainstItsPointEstimates(t *testing.T) {
	f := fixture(t)
	writeSolidBodies(t, f.DBPath)
	cfg := baseConfig(f)
	cfg.A = ArmSpec{Label: "point", DBPath: f.DBPath, ParamHash: evalfixture.ParamsA, Stage: StageOnline, DeclaredBaseline: true}
	cfg.B = cfg.A
	cfg.B.Label, cfg.B.SolidBodies = "solid", true

	c, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.A.Arm.Kind != ArmEstimates || c.B.Arm.Kind != ArmEstimates {
		t.Fatalf("kinds %s and %s: a solid-body arm is an estimate arm", c.A.Arm.Kind, c.B.Arm.Kind)
	}
	if c.A.Arm.Table != "" || c.B.Arm.Table != "lidar_track_solid_bodies" {
		t.Fatalf("tables %q and %q", c.A.Arm.Table, c.B.Arm.Table)
	}
	if c.B.Arm.ObservationModelID != string(l5tracks.MeasurementNearEdgeCandidateV1) || c.B.Arm.Tracks != c.A.Arm.Tracks {
		t.Fatalf("solid arm identity %+v", c.B.Arm)
	}
	// Same identities, so only MOTP moves, by exactly the offset.
	a, b := c.Total.A, c.Total.B
	if a.Matches != b.Matches || a.IDSwitches != b.IDSwitches || a.FP != b.FP || a.FN != b.FN {
		t.Fatalf("counts differ: point %+v, solid %+v", a, b)
	}
	if a.MOTP > 1e-6 || math.Abs(b.MOTP-solidBodyOffsetY) > 1e-6 {
		t.Fatalf("MOTP point %v, solid %v; want 0 and %v", a.MOTP, b.MOTP, solidBodyOffsetY)
	}
	var caveat bool
	for _, cv := range c.Caveats {
		caveat = caveat || strings.Contains(cv, "Arm solid scores solid-body positions")
	}
	if !caveat {
		t.Fatalf("no solid-body caveat in %q", c.Caveats)
	}
	if md := RenderMarkdown(*c); !strings.Contains(md, "| solid | estimates (lidar_track_solid_bodies) |") ||
		!strings.Contains(md, "| point | estimates | ") {
		t.Fatalf("markdown arm table does not say where each arm was read from:\n%s", md)
	}
}

func TestSolidBodyArmRefusesAnAnalysisRun(t *testing.T) {
	f := fixture(t)
	_, err := LoadArm(ArmSpec{Label: "x", DBPath: f.DBPath, RunID: evalfixture.RunA, SolidBodies: true, DeclaredBaseline: true})
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("an analysis run was read as solid bodies: %v", err)
	}
}

func TestSolidBodyArmWithNoSolidBodiesSaysSo(t *testing.T) {
	f := fixture(t)
	_, err := LoadArm(ArmSpec{Label: "x", DBPath: f.DBPath, SolidBodies: true, Stage: StageOnline, DeclaredBaseline: true})
	if err == nil || !strings.Contains(err.Error(), "no estimates at all") {
		t.Fatalf("an empty solid-body table was not reported: %v", err)
	}
}
