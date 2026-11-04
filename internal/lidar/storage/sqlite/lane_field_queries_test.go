package sqlite

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// seedLaneFieldRun stores a synthetic run: six cars moving along +Y at x 0.1
// with boxes, one car whose rows fall outside the run's window, and two
// pedestrians (under k). It returns the run id.
func seedLaneFieldRun(t *testing.T, db *sql.DB) string {
	t.Helper()
	const runID = "run-lf"
	const t0 = int64(1_788_000_000_000_000_000)
	store := NewRunStore(db)
	if err := store.InsertRun(&Run{RunID: runID, SensorID: "s1", SourceType: "pcap", SourcePath: "/captures/site-a.pcapng", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE lidar_runs SET frame_start_ns = ?, frame_end_ns = ? WHERE run_id = ?`, t0, t0+20_000_000_000, runID); err != nil {
		t.Fatal(err)
	}
	add := func(id, class string, x float64, start int64, n int, speed float64) {
		for _, stmt := range []struct {
			q    string
			args []any
		}{
			{`INSERT INTO lidar_tracks (track_id, sensor_id, frame_id, track_state, start_unix_nanos) VALUES (?, 's1', 'f', 'confirmed', ?)`, []any{id, start}},
			{`INSERT INTO lidar_run_tracks (run_id, track_id, sensor_id, track_state, start_unix_nanos, object_class) VALUES (?, ?, 's1', 'deleted', ?, ?)`, []any{runID, id, start, class}},
		} {
			if _, err := db.Exec(stmt.q, stmt.args...); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < n; i++ {
			ts := start + int64(i)*100_000_000
			y := speed * 0.1 * float64(i)
			if _, err := db.Exec(`INSERT INTO lidar_track_observations
				(track_id, ts_unix_nanos, frame_id, x, y, z, velocity_x, velocity_y, speed_mps, heading_rad,
				 bounding_box_length, bounding_box_width, bounding_box_height, frame_unix_nanos,
				 box_x, box_y, box_heading_rad, box_length, box_width)
				VALUES (?, ?, 'f', ?, ?, 0, 0, ?, ?, 1.5708, 4, 2, 1.5, ?, ?, ?, 1.5708, 4, 2)`,
				id, ts, x, y, speed, speed, ts, x, y); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i := 0; i < 6; i++ {
		add(fmt.Sprintf("car-%d", i), "car", 0.1, t0+int64(i)*2_000_000_000, 30, 8)
	}
	add("late", "car", 0.1, t0+40_000_000_000, 30, 8) // outside the window
	for i := 0; i < 2; i++ {
		add(fmt.Sprintf("ped-%d", i), "pedestrian", 6.1, t0, 30, 1.2)
	}
	return runID
}

func TestRunLaneFieldFromStoredObservations(t *testing.T) {
	d, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()
	runID := seedLaneFieldRun(t, d.DB)
	store := NewRunStore(d.DB)

	f, err := runLaneField(store, runID, l8analytics.LaneFieldPartAll)
	if err != nil {
		t.Fatal(err)
	}
	h := f.Header
	if h.SensorID != "s1" || h.WindowPart != "all" || h.K != 5 || h.CellM != 0.25 || h.L16Token != "" || h.RecordID != "" {
		t.Fatalf("header %+v", h)
	}
	if h.CaptureRef != "site-a.pcapng@2026-08-29T10:40:00Z" {
		t.Fatalf("capture_ref %q", h.CaptureRef)
	}
	if h.TrackCount != 8 { // six cars and two pedestrians; the late car is outside
		t.Fatalf("track_count %d, want 8", h.TrackCount)
	}
	if len(f.Groups) != 1 || f.Groups[0].ClassGroup != "vehicle" {
		t.Fatalf("groups %+v: pedestrians under k must not appear", f.Groups)
	}
	g := f.Groups[0]
	for c := range g.IX {
		if g.Tracks[c] > 6 || g.BoxTracks[c] > 6 {
			t.Fatalf("cell %d counts %d tracks; the late car's rows were read", c, g.Tracks[c])
		}
	}

	// The halves are built from the same rows and partition the window.
	first, err := runLaneField(store, runID, l8analytics.LaneFieldPartFirstHalf)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runLaneField(store, runID, l8analytics.LaneFieldPartSecondHalf)
	if err != nil {
		t.Fatal(err)
	}
	if first.Header.WindowEnd != second.Header.WindowStart || first.Header.WindowStart != h.WindowStart || second.Header.WindowEnd != h.WindowEnd {
		t.Fatalf("halves %s–%s and %s–%s do not partition %s–%s", first.Header.WindowStart, first.Header.WindowEnd,
			second.Header.WindowStart, second.Header.WindowEnd, h.WindowStart, h.WindowEnd)
	}

	// Two reads give the same bytes.
	a, _ := f.JSON()
	again, _ := runLaneField(store, runID, l8analytics.LaneFieldPartAll)
	b, _ := again.JSON()
	if !bytes.Equal(a, b) {
		t.Fatal("two reads of one run gave different bytes")
	}
}

func TestRunLaneFieldMissingAndEmptyRuns(t *testing.T) {
	d, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()
	store := NewRunStore(d.DB)
	if _, err := runLaneField(store, "nope", l8analytics.LaneFieldPartAll); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing run: %v, want ErrNotFound", err)
	}
	if err := store.InsertRun(&Run{RunID: "empty", SensorID: "s1", SourceType: "live", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	f, err := runLaneField(store, "empty", l8analytics.LaneFieldPartAll)
	if err != nil {
		t.Fatal(err)
	}
	if f.CellCount() != 0 || f.Header.SensorID != "s1" || f.Header.CaptureRef != "live:s1@run:empty" {
		t.Fatalf("empty run: %+v", f.Header)
	}
}

func TestLaneFieldMetaBuildAndFingerprint(t *testing.T) {
	start, end := int64(5_000_000_000), int64(9_000_000_000)
	m := LaneFieldMeta(&Run{RunID: "r", SensorID: "s", SourcePath: "/x/y.pcap", BuildVersion: "v0.5.1", BuildGitSHA: "abc", ParamsHash: "ph", FrameStartNs: &start, FrameEndNs: &end})
	if m.PipelineBuild != "v0.5.1+abc" || m.TuningFingerprint != "ph" || m.WindowStartNs != start || m.WindowEndNs != end || m.CaptureRef != "y.pcap@1970-01-01T00:00:05Z" {
		t.Fatalf("meta %+v", m)
	}
	if m := LaneFieldMeta(&Run{RunID: "r", SensorID: "s", BuildVersion: "v1"}); m.PipelineBuild != "v1" {
		t.Fatalf("meta %+v", m)
	}
}

// runLaneField builds a part from the store's observations, as
// lanefieldrun.Build does when they exist.
func runLaneField(store *RunStore, runID string, part l8analytics.LaneFieldPart) (l8analytics.LaneField, error) {
	run, err := store.GetRun(runID)
	if err != nil {
		return l8analytics.LaneField{}, err
	}
	tracks, err := store.LaneFieldTracks(run)
	if err != nil {
		return l8analytics.LaneField{}, err
	}
	return l8analytics.BuildLaneField(tracks, LaneFieldMeta(run), part, l8analytics.DefaultLaneFieldOptions())
}
