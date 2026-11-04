package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	coredb "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// laneFieldServer stores a synthetic run of five cars along +Y and serves it.
func laneFieldServer(t *testing.T) *Server {
	t.Helper()
	testDB, cleanup := setupTestDBWrapped(t)
	t.Cleanup(cleanup)
	store := sqlite.NewRunStore(testDB)
	if err := store.InsertRun(&sqlite.Run{RunID: "run-1", SensorID: "s1", SourceType: "pcap", SourcePath: "/captures/a.pcapng", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertRun(&sqlite.Run{RunID: "run-empty", SensorID: "s1", SourceType: "pcap", SourcePath: "/captures/b.pcapng", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	const t0 = int64(1_788_000_000_000_000_000)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("car-%d", i)
		laneFieldExec(t, testDB, `INSERT INTO lidar_tracks (track_id, sensor_id, frame_id, track_state, start_unix_nanos) VALUES (?, 's1', 'f', 'confirmed', ?)`, id, t0)
		laneFieldExec(t, testDB, `INSERT INTO lidar_run_tracks (run_id, track_id, sensor_id, track_state, start_unix_nanos, object_class) VALUES ('run-1', ?, 's1', 'confirmed', ?, 'car')`, id, t0)
		for j := 0; j < 20; j++ {
			ts := t0 + int64(i)*1_000_000_000 + int64(j)*100_000_000
			laneFieldExec(t, testDB, `INSERT INTO lidar_track_observations (track_id, ts_unix_nanos, frame_id, x, y, velocity_x, velocity_y, speed_mps, frame_unix_nanos) VALUES (?, ?, 'f', 0.1, ?, 0, 6, 6, ?)`,
				id, ts, 0.6*float64(j), ts)
		}
	}
	return &Server{db: testDB}
}

func laneFieldExec(t *testing.T, db *coredb.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func getLaneField(ws *Server, method, runID, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/lidar/runs/"+runID+"/lane-field"+query, nil)
	rec := httptest.NewRecorder()
	ws.handleRunTrackAPI(rec, req)
	return rec
}

func TestRunLaneFieldRoute(t *testing.T) {
	ws := laneFieldServer(t)

	rec := getLaneField(ws, http.MethodGet, "run-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content type %q", ct)
	}
	var f l8analytics.LaneField
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatal(err)
	}
	if f.Header.WindowPart != "all" || f.Header.K != 5 || f.Header.TrackCount != 5 || len(f.Groups) != 1 {
		t.Fatalf("field %+v", f.Header)
	}

	// The route serves exactly the store's bytes, which the CLI writes too.
	want, err := sqlite.NewRunStore(ws.db).RunLaneField("run-1", l8analytics.LaneFieldPartAll)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, _ := want.JSON()
	if !bytes.Equal(rec.Body.Bytes(), wantBytes) {
		t.Fatal("the route's bytes differ from RunLaneField's JSON")
	}

	for _, part := range []string{"first_half", "second_half", "all"} {
		rec := getLaneField(ws, http.MethodGet, "run-1", "?part="+part)
		if rec.Code != http.StatusOK {
			t.Fatalf("part %s: status %d", part, rec.Code)
		}
		var f l8analytics.LaneField
		_ = json.Unmarshal(rec.Body.Bytes(), &f)
		if f.Header.WindowPart != part {
			t.Fatalf("part %s served %s", part, f.Header.WindowPart)
		}
	}
}

func TestRunLaneFieldRouteErrors(t *testing.T) {
	ws := laneFieldServer(t)
	cases := []struct {
		name, method, run, query string
		want                     int
	}{
		{"no such run", http.MethodGet, "nope", "", http.StatusNotFound},
		{"bad part", http.MethodGet, "run-1", "?part=middle", http.StatusBadRequest},
		{"k is not a parameter", http.MethodGet, "run-1", "?k=1", http.StatusBadRequest},
		{"post", http.MethodPost, "run-1", "", http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		if rec := getLaneField(ws, c.method, c.run, c.query); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, rec.Code, c.want)
		}
	}
	// A run with no observations gives an empty field with a header.
	rec := getLaneField(ws, http.MethodGet, "run-empty", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("empty run: status %d", rec.Code)
	}
	var f l8analytics.LaneField
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Groups) != 0 || f.Header.SensorID != "s1" || f.Header.CaptureRef == "" {
		t.Fatalf("empty run field %+v", f)
	}
}
