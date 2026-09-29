package evalfixture

import (
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Every estimate the fixture writes states a reference point and a support
// token, so its evidence database is one the stores write and read as a
// replay's would: medoid positions, each observed.
func TestWriteDBStatesReferenceAndSupport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.db")
	if err := WriteDB(path, []string{"online", "final"}); err != nil {
		t.Fatal(err)
	}
	database, err := db.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := sqlite.NewStateEstimateStore(database.DB)
	online, err := store.ListBySource(SourceID)
	if err != nil {
		t.Fatal(err)
	}
	var want int
	for _, tr := range ArmA {
		want += tr.to - tr.from + 1
	}
	if len(online) != want {
		t.Fatalf("%d online estimates, want arm A's %d", len(online), want)
	}
	for _, e := range online {
		if e.Reference != l5tracks.ReferenceClusterMedoid || e.Support != l5tracks.SupportObserved {
			t.Fatalf("estimate %s states %s and %q", e.EstimateID, e.Reference, e.Support)
		}
	}
}
