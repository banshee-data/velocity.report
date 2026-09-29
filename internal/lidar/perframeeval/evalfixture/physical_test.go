package evalfixture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// The physical fixture writes a pack whose references pass the store's own
// validation against its sidecar, a split manifest for that pack, and an
// evidence database with every arm's rows, each stating its reference and
// support.
func TestWritePhysicalIsConsistent(t *testing.T) {
	f, err := WritePhysical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Manifest.PackDigest != f.PackDigest || len(pack.Samples) != PhysSamples {
		t.Fatalf("pack %s with %d samples", pack.Manifest.PackDigest, len(pack.Samples))
	}
	sidecar, err := annotation.LoadSidecar(pack)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := annotation.LoadPhysicalReferences(pack)
	if err != nil {
		t.Fatal(err)
	}
	if err := refs.ValidateLinks(pack, sidecar); err != nil {
		t.Fatalf("the fixture's references do not link to its sidecar: %v", err)
	}
	if len(refs.Objects) != 2 || len(refs.Following) != 2 || refs.Source.CalibrationID != PhysCalibration {
		t.Fatalf("references: %d objects, %d following, calibration %q", len(refs.Objects), len(refs.Following), refs.Source.CalibrationID)
	}
	manifest, err := annotation.LoadSplitManifest(f.SplitManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.ValidateAgainst(pack, sidecar); err != nil {
		t.Fatalf("the split manifest does not fit the pack: %v", err)
	}

	database, err := db.NewDB(f.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := sqlite.NewStateEstimateStore(database.DB)
	for params, want := range map[string]l5tracks.ReferencePoint{
		ParamsExact:            l5tracks.ReferenceBodyCentre,
		ParamsFaceBias:         l5tracks.ReferenceBodyCentre,
		ParamsOtherCalibration: l5tracks.ReferenceBodyCentre,
		ParamsMedoid:           l5tracks.ReferenceClusterMedoid,
	} {
		key := sqlite.EstimateVersionKey{SourceID: PhysSource, EstimatorID: EstimatorID,
			ObservationModelID: string(l5tracks.MeasurementNearEdgeCandidateV1), ParamHash: params, Stage: "online"}
		bodies, err := store.ListVersionSolidBodies(key)
		if err != nil {
			t.Fatal(err)
		}
		if len(bodies) == 0 {
			t.Fatalf("%s: no solid bodies", params)
		}
		for _, b := range bodies {
			if b.Reading.Estimate.Reference != want {
				t.Fatalf("%s: a body at %d names %s, want %s", params, b.FrameUnixNanos, b.Reading.Estimate.Reference, want)
			}
		}
	}
	points, err := store.ListVersionEstimates(sqlite.EstimateVersionKey{SourceID: PhysSource, EstimatorID: EstimatorID,
		ObservationModelID: string(l5tracks.MeasurementOBBCentreV1), ParamHash: ParamsExact, Stage: "final"})
	if err != nil {
		t.Fatal(err)
	}
	if len(points) == 0 {
		t.Fatal("no point estimates")
	}
	for _, e := range points {
		if e.Reference != l5tracks.ReferenceVisibleOBBCentre || e.Support != l5tracks.SupportObserved {
			t.Fatalf("point estimate %s states %s and %s", e.EstimateID, e.Reference, e.Support)
		}
	}
}

// A directory the fixture cannot write into is an error, not a panic.
func TestWritePhysicalRefusesAnUnwritableDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePhysical(file); err == nil {
		t.Fatal("the fixture wrote under a file")
	}
}
