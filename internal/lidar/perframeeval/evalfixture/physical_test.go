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

// The physical fixture freezes: its one tuning partition passes the frozen
// split's review checks, and the written split binds to the pack at the
// revision it pins.
func TestPhysicalFixtureFreezes(t *testing.T) {
	dir := t.TempDir()
	f, err := WritePhysical(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "frozen.json")
	frozen, err := f.WriteFrozen(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := annotation.LoadFrozenSplit(path)
	if err != nil || loaded.SplitDigest != frozen.SplitDigest {
		t.Fatalf("frozen split reads back as %v, %v", loaded, err)
	}
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	view, sidecar, err := loaded.Bind(pack)
	if err != nil {
		t.Fatal(err)
	}
	if sidecar.Revision != 1 || len(view.Splits) != 1 || view.Splits[0].Name != PhysSplit || len(view.Episodes) != 1 {
		t.Fatalf("bound view %+v at revision %d", view, sidecar.Revision)
	}
	if _, err := f.WriteFrozen(filepath.Join(dir, "missing", "frozen.json")); err == nil {
		t.Fatal("a split written into a missing directory was not refused")
	}
	f.PackDir = filepath.Join(dir, "no-pack")
	if _, err := f.WriteFrozen(path); err == nil {
		t.Fatal("a draft of a missing pack froze")
	}
}

// The fixture's frozen split pins its physical references, and the legacy
// writer produces the version 2 layout: no pin, digest intact, binding to
// the same pack at the same annotation revision.
func TestPhysicalFixtureFreezesWithAndWithoutAPin(t *testing.T) {
	dir := t.TempDir()
	f, err := WritePhysical(dir)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := f.WriteFrozen(filepath.Join(dir, "v3.json"))
	if err != nil {
		t.Fatal(err)
	}
	pin := pinned.Packs[0].Physical
	if pinned.SchemaVersion != annotation.FrozenSplitSchemaVersion || pin == nil || pin.Revision != 1 || len(pin.Objects) != 2 ||
		pin.Coverage.Position.Scorable == 0 {
		t.Fatalf("pinned split %+v", pinned.Packs[0])
	}
	legacyPath := filepath.Join(dir, "v2.json")
	legacy, err := f.WriteFrozenMembershipOnly(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := annotation.LoadFrozenSplit(legacyPath)
	if err != nil {
		t.Fatalf("the legacy split does not read: %v", err)
	}
	if loaded.SchemaVersion != annotation.FrozenSplitSchemaVersionMembershipOnly || loaded.SplitDigest != legacy.SplitDigest ||
		loaded.SplitDigest == pinned.SplitDigest || loaded.Packs[0].Physical != nil {
		t.Fatalf("legacy split %+v", loaded)
	}
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	_, sidecar, err := loaded.Bind(pack)
	if err != nil {
		t.Fatal(err)
	}
	if doc, err := loaded.BindPhysical(pack, sidecar); doc != nil || err != nil {
		t.Fatalf("a legacy split bound a physical pin: %+v, %v", doc, err)
	}
	f.PackDir = filepath.Join(dir, "no-pack")
	if _, err := f.WriteFrozenMembershipOnly(filepath.Join(dir, "v2-b.json")); err == nil {
		t.Fatal("a legacy split of a missing pack froze")
	}
	f.PackDir = filepath.Join(dir, "pack")
	if _, err := f.WriteFrozenMembershipOnly(filepath.Join(dir, "missing", "v2.json")); err == nil {
		t.Fatal("a legacy split written into a missing directory was not refused")
	}
}
