package sqlite

import (
	"errors"
	"testing"
	"time"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/lanes"
)

func laneRequest(id, row, limit string) lanes.Request {
	return lanes.Request{
		RequestID: id, Writer: lanes.WriterPainter, SensorID: "s1", RecordID: "gms:rec",
		Pose: lanes.Pose{Rotation: [4]float64{1, 0, 0, 0}}, Author: "operator",
		Features: []lanes.Feature{{
			FeatureID: "gms:e", RowID: row, Kind: lanes.KindTravelLane, XYZ: []float64{0, -2, 21, 50, -2, 21},
			Tags: map[string]string{"street": "KIRKHAM ST", "width_m": "3", "speed_limit": limit, "speed_limit_unit": "mph",
				"speed_limit_source": "sign", "speed_limit_effective_from": "2026-01-01"},
		}},
	}
}

func TestLaneSnapshotsAreAppendOnlyAndIdempotent(t *testing.T) {
	d, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()
	store := NewLaneSnapshotStore(d.DB)
	t0 := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)

	first, repeat, err := store.Put("808580f47", laneRequest("r1", "row1", "25"), t0)
	if err != nil || repeat {
		t.Fatalf("first: %v %v", err, repeat)
	}
	again, repeat, err := store.Put("808580f47", laneRequest("r1", "row1", "25"), t0.Add(time.Hour))
	if err != nil || !repeat || again.SnapshotID != first.SnapshotID {
		t.Fatalf("repeat: %v %v %s", err, repeat, again.SnapshotID)
	}
	if _, _, err := store.Put("808580f47", laneRequest("r1", "row2", "30"), t0); !errors.Is(err, ErrLaneRequestConflict) {
		t.Fatalf("conflict: %v", err)
	}
	if _, _, err := store.Put("808580f47", lanes.Request{RequestID: "bad"}, t0); !errors.Is(err, lanes.ErrInvalid) {
		t.Fatalf("invalid: %v", err)
	}
	// A second write supersedes the first, and is never earlier than it.
	second, _, err := store.Put("808580f47", laneRequest("r2", "row2", "30"), t0.Add(-time.Hour))
	if err != nil || second.Supersedes != first.SnapshotID || !second.FetchedAt.After(first.FetchedAt) {
		t.Fatalf("second: %v %+v", err, second)
	}
	latest, err := store.Latest("808580f47", "s1")
	if err != nil || latest.SnapshotID != second.SnapshotID {
		t.Fatalf("latest: %v", err)
	}
	h, err := store.History("808580f47", "s1")
	if err != nil || len(h.Versions) != 2 {
		t.Fatalf("history: %v %+v", err, h)
	}
	var lanesStored int
	if err := d.QueryRow(`SELECT COUNT(*) FROM lidar_lane_snapshot_lanes WHERE speed_limit IS NOT NULL AND effective_from = '2026-01-01'`).Scan(&lanesStored); err != nil || lanesStored != 2 {
		t.Fatalf("generated columns: %v %d", err, lanesStored)
	}
	// Append-only: rows cannot change or go.
	if _, err := d.Exec(`UPDATE lidar_lane_snapshots SET author = 'x'`); err == nil {
		t.Fatal("snapshot updated")
	}
	if _, err := d.Exec(`DELETE FROM lidar_lane_snapshot_lanes`); err == nil {
		t.Fatal("lanes deleted")
	}
	if all, _ := store.All("808580f47", ""); len(all) != 2 {
		t.Fatalf("all sensors: %d", len(all))
	}
	if none, _ := store.Latest("nowhere", "s1"); none != nil {
		t.Fatal("latest of nothing")
	}
}
