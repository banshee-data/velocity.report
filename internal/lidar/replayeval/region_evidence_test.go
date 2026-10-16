package replayeval

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// regionEvidenceGrid is a two-ring, four-bin grid with every cell seen and a
// settling spread that rises from cell to cell, ready for IdentifyRegions.
func regionEvidenceGrid(t *testing.T) *l3grid.BackgroundManager {
	t.Helper()
	bg := l3grid.NewBackgroundManagerDI("region-evidence", 2, 4, l3grid.BackgroundParams{NoiseRelativeFraction: 0.01}, nil)
	if bg == nil {
		t.Fatal("no background manager")
	}
	if err := bg.SetRingElevations([]float64{-1, 1}); err != nil {
		t.Fatal(err)
	}
	bg.SetSourcePath("captures/site.pcapng")
	for i := range bg.Grid.Cells {
		bg.Grid.Cells[i] = l3grid.BackgroundCell{AverageRangeMeters: 10, TimesSeenCount: 40}
		bg.Grid.RegionMgr.SettlingMetrics.VariancePerCell[i] = float64(i)
	}
	bg.Grid.RegionMgr.SettlingMetrics.FramesSampled = 30
	return bg
}

func openEvidence(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.NewDB(filepath.Join(t.TempDir(), "observations.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestRegionEvidenceWritesIdentifiedAndFinalMaps(t *testing.T) {
	bg := regionEvidenceGrid(t)
	database := openEvidence(t)
	evidence := newRegionEvidence(bg, database)
	t0, t1, t2 := time.Unix(100, 0), time.Unix(101, 500), time.Unix(160, 0)

	evidence.observe(t0)
	if evidence.identified != nil {
		t.Fatal("staged a region map before regions were identified")
	}
	if err := bg.Grid.RegionMgr.IdentifyRegions(bg.Grid, 50); err != nil {
		t.Fatal(err)
	}
	evidence.observe(t1)
	evidence.observe(t1.Add(time.Second)) // the identified map is staged once
	if sets, err := database.ListRegionSnapshots(); err != nil || len(sets) != 0 {
		t.Fatalf("region rows written before the replay finished: %v %v", sets, err)
	}
	// Later frames move the grid; the end map must show it, the identified
	// map must not.
	bg.Grid.Cells[5].AverageRangeMeters = 25
	if err := evidence.finish(t2); err != nil {
		t.Fatal(err)
	}

	sets, err := database.ListRegionSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 2 {
		t.Fatalf("got %d region sets, want the identified map and the final one: %+v", len(sets), sets)
	}
	wantReasons := []string{l3grid.SnapshotReasonRegionsIdentified, l3grid.SnapshotReasonReplayEnd}
	wantTimes := []int64{t1.UnixNano(), t2.UnixNano()}
	wantRange := []float32{10, 25}
	for i, set := range sets {
		if set.SnapshotReason != wantReasons[i] || set.CreatedUnixNanos != wantTimes[i] {
			t.Fatalf("region set %d: reason %q at %d, want %q at %d", i, set.SnapshotReason, set.CreatedUnixNanos, wantReasons[i], wantTimes[i])
		}
		if set.SourcePath != "captures/site.pcapng" || set.RegionCount != len(bg.Grid.RegionMgr.Regions) {
			t.Fatalf("region set %d: source %q, %d regions", i, set.SourcePath, set.RegionCount)
		}
		regions, err := database.GetRegionSnapshotByID(set.RegionSetID)
		if err != nil {
			t.Fatal(err)
		}
		grid, err := database.GetBgSnapshotByID(regions.SnapshotID)
		if err != nil || grid == nil {
			t.Fatalf("region set %d links grid snapshot %d: %v", i, regions.SnapshotID, err)
		}
		if grid.TakenUnixNanos != wantTimes[i] {
			t.Fatalf("grid snapshot %d taken at %d, want the frame time %d", i, grid.TakenUnixNanos, wantTimes[i])
		}
		m, err := l3grid.BuildRegionMap(grid, regions)
		if err != nil {
			t.Fatal(err)
		}
		if got := m.Cells[5].SettledRangeM; got != wantRange[i] {
			t.Fatalf("%s map: cell 5 range %v, want %v", wantReasons[i], got, wantRange[i])
		}
		if m.Cells[7].CellTercile != "volatile" || m.Cells[0].CellTercile != "stable" {
			t.Fatalf("%s map: cell terciles %q and %q", wantReasons[i], m.Cells[0].CellTercile, m.Cells[7].CellTercile)
		}
	}
}

func TestRegionEvidenceWithoutRegionsWritesTheGridOnly(t *testing.T) {
	bg := regionEvidenceGrid(t)
	database := openEvidence(t)
	evidence := newRegionEvidence(bg, database)
	evidence.observe(time.Unix(1, 0))
	if err := evidence.finish(time.Unix(2, 0)); err != nil {
		t.Fatal(err)
	}
	if sets, err := database.ListRegionSnapshots(); err != nil || len(sets) != 0 {
		t.Fatalf("an unsettled replay wrote region sets: %v %v", sets, err)
	}
	grid, err := database.GetLatestBgSnapshot("region-evidence")
	if err != nil || grid == nil || grid.SnapshotReason != l3grid.SnapshotReasonReplayEnd {
		t.Fatalf("final grid snapshot: %+v %v", grid, err)
	}
}

func TestRegionEvidenceReportsAFailedWrite(t *testing.T) {
	bg := regionEvidenceGrid(t)
	database := openEvidence(t)
	evidence := newRegionEvidence(bg, database)
	if err := bg.Grid.RegionMgr.IdentifyRegions(bg.Grid, 50); err != nil {
		t.Fatal(err)
	}
	evidence.observe(time.Unix(1, 0))
	database.Close()
	if err := evidence.finish(time.Unix(2, 0)); err == nil {
		t.Fatal("finish reported success writing to a closed database")
	}
	var none *regionEvidence
	none.observe(time.Unix(1, 0))
	if err := none.finish(time.Unix(2, 0)); err != nil {
		t.Fatalf("a replay without evidence failed its region map: %v", err)
	}
}
