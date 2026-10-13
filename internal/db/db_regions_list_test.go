package db

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// TestListRegionSnapshotsAndGetByID covers the evidence reader's two queries:
// the listing joins each region set to its grid snapshot's reason, and a
// region set is fetched whole by its id.
func TestListRegionSnapshotsAndGetByID(t *testing.T) {
	db, err := NewDB(t.TempDir() + "/regions_list.db")
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	if sets, err := db.ListRegionSnapshots(); err != nil || len(sets) != 0 {
		t.Fatalf("empty database lists %v, %v", sets, err)
	}
	for i, reason := range []string{"regions_identified", "replay_end"} {
		bgID, err := db.InsertBgSnapshot(&l3grid.BgSnapshot{
			SensorID: "s", TakenUnixNanos: int64(10 + i), Rings: 1, AzimuthBins: 1,
			ParamsJSON: "{}", GridBlob: []byte("blob"), SnapshotReason: reason,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.InsertRegionSnapshot(&l3grid.RegionSnapshot{
			SnapshotID: bgID, SensorID: "s", CreatedUnixNanos: int64(10 + i), RegionCount: 3 + i,
			RegionsJSON: `[]`, VarianceDataJSON: `{"variance_per_cell":[0]}`, SettlingFrames: 7,
			GridHash: "h", SourcePath: "a.pcap",
		}); err != nil {
			t.Fatal(err)
		}
	}

	sets, err := db.ListRegionSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 2 {
		t.Fatalf("got %d region sets, want 2", len(sets))
	}
	want := RegionSnapshotSummary{RegionSetID: 2, SnapshotID: 2, SensorID: "s", CreatedUnixNanos: 11,
		RegionCount: 4, SettlingFrames: 7, SourcePath: "a.pcap", SnapshotReason: "replay_end"}
	if sets[1] != want || sets[0].SnapshotReason != "regions_identified" {
		t.Fatalf("region sets: %+v", sets)
	}

	got, err := db.GetRegionSnapshotByID(2)
	if err != nil || got == nil {
		t.Fatalf("GetRegionSnapshotByID(2): %v, %v", got, err)
	}
	if got.SnapshotID != 2 || got.VarianceDataJSON != `{"variance_per_cell":[0]}` || got.SourcePath != "a.pcap" {
		t.Fatalf("region set 2: %+v", got)
	}
	if missing, err := db.GetRegionSnapshotByID(99); err != nil || missing != nil {
		t.Fatalf("GetRegionSnapshotByID(99): %v, %v", missing, err)
	}
}
