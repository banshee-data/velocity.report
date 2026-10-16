package evalfixture

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// WriteRetentionDB writes the cases its comment lists: clusters at samples 0
// to 5, 7 and 8 and none at 6, the capped cluster at sample 3, and every
// retained point a point of its sample in the pack, bit for bit.
func TestWriteRetentionDBWritesTheCasesItDescribes(t *testing.T) {
	dir := t.TempDir()
	f, err := Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "observations.db")
	if err := WriteRetentionDB(path, f.PackDir); err != nil {
		t.Fatal(err)
	}
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	obs, err := sqlite.NewObservationStore(database.DB).ListBySource(RetentionSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 9 {
		t.Fatalf("%d observations, want 9", len(obs))
	}
	bySample := map[int]int{}
	for _, o := range obs {
		r := o.Snapshot()
		sample := -1
		for s := 0; s < 10; s++ {
			if SampleTime(s) == r.FrameUnixNanos {
				sample = s
			}
		}
		if sample < 0 {
			t.Fatalf("observation %s at %d is at no sample's time", r.ObservationID, r.FrameUnixNanos)
		}
		bySample[sample]++
		pts, err := pack.PointsAt(sample)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range r.Cluster.SamplePoints {
			found := false
			for i := range pts.X {
				if pts.X[i] == p[0] && pts.Y[i] == p[1] && pts.Z[i] == p[2] {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("sample %d: retained point %v is not a pack point", sample, p)
			}
		}
		if sample == 3 && (r.Cluster.PointsCount != 8 || len(r.Cluster.SamplePoints) != 2) {
			t.Fatalf("sample 3's cluster holds %d points with %d sampled, want 8 and 2",
				r.Cluster.PointsCount, len(r.Cluster.SamplePoints))
		}
	}
	var samples []int
	for s := range bySample {
		samples = append(samples, s)
	}
	sort.Ints(samples)
	want := []int{0, 1, 2, 3, 4, 5, 7, 8}
	if len(samples) != len(want) {
		t.Fatalf("clusters at samples %v, want %v", samples, want)
	}
	for i := range want {
		if samples[i] != want[i] {
			t.Fatalf("clusters at samples %v, want %v", samples, want)
		}
	}
	if bySample[0] != 2 {
		t.Fatalf("sample 0 has %d clusters, want car a's two fragments", bySample[0])
	}
}

func TestWriteRetentionDBRefusesAMissingPack(t *testing.T) {
	dir := t.TempDir()
	if err := WriteRetentionDB(filepath.Join(dir, "observations.db"), filepath.Join(dir, "no-pack")); err == nil {
		t.Fatal("wrote evidence for a pack that does not exist")
	}
}
