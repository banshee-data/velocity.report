package evalfixture

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func TestNearFacePackHoldsTheTwoVisibleFaces(t *testing.T) {
	f, err := WriteNearFacePack(t.TempDir(), NearFaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Samples) != NearFaceSamples {
		t.Fatalf("%d samples", len(pack.Samples))
	}
	s, err := annotation.LoadSidecar(pack)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Masks) != NearFaceSamples || len(s.Masks[0].PointIndices) != 20 || s.Objects[0].Status != annotation.StatusReviewed {
		t.Fatalf("sidecar %+v", s)
	}
	m, err := annotation.LoadSplitManifest(f.SplitManifestPath)
	if err != nil || m.PackDigest != pack.Manifest.PackDigest || len(m.Episodes) != 1 {
		t.Fatalf("manifest %+v, %v", m, err)
	}
	// The rear face sits half a length behind the centre.
	pts, err := pack.PointsAt(0)
	if err != nil {
		t.Fatal(err)
	}
	cx, _ := NearFaceCentre(0)
	if got := float64(pts.X[0]); math.Abs(got-(cx-NearFaceLength/2)) > 1e-5 { // float32 returns
		t.Fatalf("rear face x %v, want %v", got, cx-NearFaceLength/2)
	}

	proposed, err := WriteNearFacePack(t.TempDir(), NearFaceOptions{Proposed: true, PartialRear: true})
	if err != nil {
		t.Fatal(err)
	}
	pp, _ := annotation.OpenPack(proposed.PackDir)
	ps, _ := annotation.LoadSidecar(pp)
	if ps.Objects[0].Status != annotation.StatusProposed {
		t.Fatalf("object %+v", ps.Objects[0])
	}
	ppts, _ := pp.PointsAt(0)
	if ppts.Y[9] >= 5.0 { // a third of the width from the right edge: y below the centre
		t.Fatalf("partial rear reaches y=%v", ppts.Y[9])
	}
}

func TestNearFaceDBWritesOneVersionPerArm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e.db")
	err := WriteNearFaceDB(path, map[string]NearFaceBody{
		"exact":    {},
		"shifted":  {DX: -0.3, DY: 0.2, Length: 5, Width: 2},
		"medoid":   {Ref: l5tracks.ReferenceClusterMedoid},
		"prior":    {Prior: true},
		"bare":     {NoHeading: true, NoExtent: true},
		"far":      {Far: true},
		"gappy":    {Missing: func(i int) bool { return i != 0 }},
		"original": {},
	})
	if err != nil {
		t.Fatal(err)
	}
	database, err := sqlite.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	versions, err := sqlite.NewStateEstimateStore(database).ListSolidBodyVersions()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, v := range versions {
		counts[v.ParamHash] = v.Estimates
	}
	if len(counts) != 8 || counts["exact"] != NearFaceSamples || counts["gappy"] != 1 {
		t.Fatalf("versions %v", counts)
	}
}

func TestNearFaceFixturesReportAFailedWrite(t *testing.T) {
	// A pack cannot be written where a file stands in its place.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pack"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if f, err := WriteNearFacePack(dir, NearFaceOptions{}); err == nil || f != nil {
		t.Fatalf("pack over a file: %v, %v", f, err)
	}
	// Nor a split manifest where a directory stands.
	dir = t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "split.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if f, err := WriteNearFacePack(dir, NearFaceOptions{}); err == nil || f != nil {
		t.Fatalf("manifest over a directory: %v, %v", f, err)
	}
	// A database is not opened in a directory that is not there, and a second
	// write of the same observations is refused rather than doubled.
	if err := WriteNearFaceDB(filepath.Join(t.TempDir(), "absent", "e.db"), map[string]NearFaceBody{"a": {}}); err == nil {
		t.Fatal("database in a missing directory")
	}
	path := filepath.Join(t.TempDir(), "e.db")
	if err := WriteNearFaceDB(path, map[string]NearFaceBody{"a": {}}); err != nil {
		t.Fatal(err)
	}
	if err := WriteNearFaceDB(path, map[string]NearFaceBody{"a": {}}); err == nil {
		t.Fatal("the same observations written twice")
	}
}
