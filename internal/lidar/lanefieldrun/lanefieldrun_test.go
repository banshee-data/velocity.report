package lanefieldrun

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

const t0 = int64(1_788_000_000_000_000_000)

// recordRun writes a synthetic recording: five cars along +Y at x 0.1, each
// for 20 frames, every fourth frame a miss placed far away; and a sixth
// track the run never recorded. It stores the run with that recording.
func recordRun(t *testing.T) (*sqlite.RunStore, string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "run.vrlog")
	rec, err := recorder.NewRecorder(path, "s1")
	if err != nil {
		t.Fatal(err)
	}
	for f := 0; f < 120; f++ {
		ts := t0 + int64(f)*100_000_000
		var tracks []l9endpoints.Track
		for i := 0; i < 6; i++ {
			j := f - 20*i
			if j < 0 || j >= 20 {
				continue
			}
			tr := l9endpoints.Track{
				TrackID: fmt.Sprintf("car-%d", i), X: 0.1, Y: float32(0.6 * float64(j)), VY: 6, SpeedMps: 6,
				BBoxLength: 4, BBoxWidth: 2, BBoxHeadingRad: 1.5708,
			}
			if j%4 == 3 {
				tr.Misses = 1
				tr.X, tr.Y = 40, 40 // a coasted prediction: must not count
			}
			if i == 5 {
				tr.TrackID = "never-confirmed"
			}
			tracks = append(tracks, tr)
		}
		if err := rec.Record(&l9endpoints.FrameBundle{FrameID: uint64(f), TimestampNanos: ts, SensorID: "s1", Tracks: &l9endpoints.TrackSet{TimestampNanos: ts, Tracks: tracks}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	d, cleanup := dbpkg.NewTestDB(t)
	t.Cleanup(cleanup)
	store := sqlite.NewRunStore(d.DB)
	if err := store.InsertRun(&sqlite.Run{RunID: "run-v", SensorID: "s1", SourceType: "pcap", SourcePath: "/c/a.pcapng", Status: "completed", VRLogPath: path}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := d.Exec(`INSERT INTO lidar_run_tracks (run_id, track_id, sensor_id, track_state, start_unix_nanos, object_class) VALUES ('run-v', ?, 's1', 'confirmed', ?, 'car')`, fmt.Sprintf("car-%d", i), t0); err != nil {
			t.Fatal(err)
		}
	}
	return store, "run-v", path
}

func TestBuildReadsTheRecordingWhenNoObservationsAreStored(t *testing.T) {
	store, runID, path := recordRun(t)
	resolve := func(s string) (string, error) { return s, nil }
	f, source, err := Build(store, runID, l8analytics.LaneFieldPartAll, resolve)
	if err != nil {
		t.Fatal(err)
	}
	if source != SourceVRLOG {
		t.Fatalf("source %q", source)
	}
	if f.Header.TrackCount != 5 || len(f.Groups) != 1 || f.Groups[0].ClassGroup != "vehicle" {
		t.Fatalf("field %+v groups %d", f.Header, len(f.Groups))
	}
	// No miss reached the field: nothing near (40, 40).
	g := f.Groups[0]
	for c := range g.IX {
		x := f.Header.Origin[0] + float64(g.IX[c])*f.Header.CellM
		y := f.Header.Origin[1] + float64(g.IY[c])*f.Header.CellM
		if x > 30 || y > 30 {
			t.Fatalf("a missed instant at (%v, %v) was counted", x, y)
		}
	}

	// The same instants given directly build the same bytes.
	classes, _ := store.RunTrackClasses(runID)
	tracks, err := FromVRLOG(path, classes)
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range tracks {
		if len(tr.Instants) != 15 { // 20 frames less 5 misses
			t.Fatalf("%s has %d instants, want 15", tr.TrackID, len(tr.Instants))
		}
		if tr.Instants[0].BoxLength != 4 || tr.Instants[0].BoxX != tr.Instants[0].X {
			t.Fatalf("box %+v: the cluster's box at the estimate when no published box", tr.Instants[0])
		}
	}
	run, _ := store.GetRun(runID)
	direct, _ := l8analytics.BuildLaneField(tracks, sqlite.LaneFieldMeta(run), l8analytics.LaneFieldPartAll, l8analytics.DefaultLaneFieldOptions())
	a, _ := f.JSON()
	b, _ := direct.JSON()
	if !bytes.Equal(a, b) {
		t.Fatal("Build and the direct build differ")
	}
}

func TestBuildWithoutAReadableRecording(t *testing.T) {
	store, runID, _ := recordRun(t)
	if _, _, err := Build(store, runID, l8analytics.LaneFieldPartAll, nil); !errors.Is(err, ErrNoRecording) {
		t.Fatalf("no resolver: %v", err)
	}
	refuse := func(string) (string, error) { return "", errors.New("outside") }
	if _, _, err := Build(store, runID, l8analytics.LaneFieldPartAll, refuse); !errors.Is(err, ErrNoRecording) {
		t.Fatalf("refused path: %v", err)
	}
	if _, _, err := Build(store, "nope", l8analytics.LaneFieldPartAll, nil); !errors.Is(err, sqlite.ErrNotFound) {
		t.Fatalf("missing run: %v", err)
	}
}

func TestFromHeadlessVRLOGKeepsConfirmedTracksWithTheirLastClass(t *testing.T) {
	path := filepath.Join(t.TempDir(), "headless.vrlog")
	rec, err := recorder.NewRecorder(path, "s1")
	if err != nil {
		t.Fatal(err)
	}
	for f := 0; f < 30; f++ {
		ts := t0 + int64(f)*100_000_000
		tracks := []l9endpoints.Track{
			// Tentative for its first five frames, then confirmed; classed late.
			{TrackID: "car-a", X: 0.1, Y: float32(0.5 * float64(f)), VY: 5, SpeedMps: 5, BBoxLength: 4, BBoxWidth: 2},
			// Never confirmed: must not count.
			{TrackID: "ghost", X: 3, Y: 3, State: l9endpoints.TrackStateTentative, ObjectClass: "car"},
		}
		if f >= 5 {
			tracks[0].State = l9endpoints.TrackStateConfirmed
		}
		if f >= 20 {
			tracks[0].ObjectClass = "car"
		}
		if err := rec.Record(&l9endpoints.FrameBundle{FrameID: uint64(f), TimestampNanos: ts, SensorID: "s1", Tracks: &l9endpoints.TrackSet{TimestampNanos: ts, Tracks: tracks}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := FromHeadlessVRLOG(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Frames != 30 || got.WindowStartNs != t0 || got.WindowEndNs != t0+29*100_000_000 {
		t.Fatalf("window = %+v", got)
	}
	if len(got.Tracks) != 1 || got.Tracks[0].TrackID != "car-a" || got.Tracks[0].Class != "car" || len(got.Tracks[0].Instants) != 30 {
		t.Fatalf("tracks = %+v", got.Tracks)
	}
}
