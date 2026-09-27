package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

const segmentTestTime int64 = 1788466680 * 1_000_000_000

func segmentServer(t *testing.T) (*Server, string) {
	t.Helper()
	database, cleanup := setupTestDBWrapped(t)
	t.Cleanup(cleanup)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(dir, "capture.pcap")
	if err := os.WriteFile(capture, []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO lidar_capture_roots(root_id,path,created_at_ns,updated_at_ns) VALUES('root',?,1,1)`, []any{dir}},
		{`INSERT INTO lidar_capture_files(capture_file_id,root_id,rel_path,size_bytes,modified_at_ns,first_packet_ns,last_packet_ns,probe_state,first_seen_at_ns,last_seen_at_ns) VALUES('file','root','capture.pcap',7,1,?,?, 'ok',1,1)`, []any{segmentTestTime - 1_000_000_000, segmentTestTime + 20_000_000_000}},
		{`INSERT INTO lidar_run_records(run_id,created_at,source_type,source_path,sensor_id,status,duration_secs,total_frames,total_clusters,total_tracks,confirmed_tracks,processing_time_ms) VALUES('run',1,'pcap',?,'sensor','completed',0,0,0,0,0,0)`, []any{capture}},
	}
	for _, q := range statements {
		if _, err := database.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"f", "l"} {
		if _, err := database.Exec(`INSERT INTO lidar_tracks(track_id,sensor_id,frame_id,track_state,start_unix_nanos,max_speed_mps) VALUES(?,'sensor','frame','confirmed',?,10)`, id, segmentTestTime); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`INSERT INTO lidar_run_tracks(run_id,track_id,sensor_id,track_state,start_unix_nanos) VALUES('run',?,'sensor','confirmed',?)`, id, segmentTestTime); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		ts := segmentTestTime + int64(i)*100_000_000
		for _, id := range []string{"f", "l"} {
			x := float64(i)
			if id == "l" {
				x += 10
			}
			if _, err := database.Exec(`INSERT INTO lidar_track_observations(track_id,ts_unix_nanos,frame_unix_nanos,frame_id,x,y,velocity_x,velocity_y) VALUES(?,?,?,'frame',?,0,10,0)`, id, ts, ts, x); err != nil {
				t.Fatal(err)
			}
		}
	}
	return &Server{db: database, pcapSafeDir: dir, annotationPacksDir: t.TempDir()}, capture
}

func callSegment(t *testing.T, ws *Server, method, path string, body any, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestSegmentAPISelectionAndClipQueue(t *testing.T) {
	ws, _ := segmentServer(t)
	list := callSegment(t, ws, "GET", "/api/lidar/segments?run_id=run&finder=following", nil, ws.handleSegments)
	if list.Code != 200 {
		t.Fatalf("list %d: %s", list.Code, list.Body.String())
	}
	var listing struct {
		Windows []segments.Window `json:"windows"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Windows) != 1 || listing.Windows[0].PairFrames != 5 || listing.Windows[0].Capture == "" {
		t.Fatalf("listing: %+v", listing.Windows)
	}
	chosen := listing.Windows[0]
	for _, tc := range []struct {
		finder, role string
		want         int
	}{{"leader_changes", "held_out", 400}, {"following", "held_out", 404}} {
		rec := callSegment(t, ws, "POST", "/api/lidar/segments/"+chosen.ID+"/case", map[string]any{"run_id": "run", "finder": tc.finder, "role": tc.role}, ws.handleSegmentByID)
		if rec.Code != tc.want {
			t.Fatalf("%s/%s: %d %s", tc.finder, tc.role, rec.Code, rec.Body.String())
		}
	}
	created := callSegment(t, ws, "POST", "/api/lidar/segments/"+chosen.ID+"/case", map[string]any{"run_id": "run", "finder": "following", "role": "tuning"}, ws.handleSegmentByID)
	if created.Code != 201 {
		t.Fatalf("case %d: %s", created.Code, created.Body.String())
	}
	var caseBody struct {
		ReplayCaseID string `json:"replay_case_id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &caseBody); err != nil {
		t.Fatal(err)
	}
	if caseBody.ReplayCaseID == "" {
		t.Fatal("missing case ID")
	}
	again := callSegment(t, ws, "POST", "/api/lidar/segments/"+chosen.ID+"/case", map[string]any{"run_id": "run", "finder": "following", "role": "tuning"}, ws.handleSegmentByID)
	if again.Code != 200 {
		t.Fatalf("duplicate case: %d", again.Code)
	}
	clip := callSegment(t, ws, "POST", "/api/lidar/scenes/"+caseBody.ReplayCaseID+"/clip", nil, func(w http.ResponseWriter, r *http.Request) { ws.handleSceneClip(w, r, caseBody.ReplayCaseID) })
	if clip.Code != 202 {
		t.Fatalf("clip %d: %s", clip.Code, clip.Body.String())
	}
	clipAgain := callSegment(t, ws, "POST", "/api/lidar/scenes/"+caseBody.ReplayCaseID+"/clip", nil, func(w http.ResponseWriter, r *http.Request) { ws.handleSceneClip(w, r, caseBody.ReplayCaseID) })
	if clipAgain.Code != 202 || clipAgain.Body.String() != clip.Body.String() {
		t.Fatalf("clip dedup: %s vs %s", clip.Body.String(), clipAgain.Body.String())
	}
	strip := callSegment(t, ws, "GET", "/api/lidar/segments/strip?run_id=run&finder=following", nil, ws.handleSegmentStrip)
	if strip.Code != 200 || strip.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("strip %d %s", strip.Code, strip.Body.String())
	}
}

func TestPackInventoryReadsSidecarReview(t *testing.T) {
	ws, _ := segmentServer(t)
	vrlog := writeTestVRLOG(t, 2)
	pack, err := annotation.Export(annotation.ExportConfig{VRLOGPath: vrlog, OutDir: filepath.Join(ws.annotationPacksDir, "one"), Coverage: annotation.CoverageForegroundOnly})
	if err != nil {
		t.Fatal(err)
	}
	side := annotation.NewSidecar(pack)
	side.Objects = []annotation.Object{{ObjectID: "a", Class: "car", Status: annotation.StatusReviewed}, {ObjectID: "b", Class: "car", Status: annotation.StatusProposed}}
	side.Masks = []annotation.FrameMask{{ObjectID: "a", SampleID: 0, Status: annotation.StatusReviewed}, {ObjectID: "b", SampleID: 1, Status: annotation.StatusReviewed}}
	b, _ := json.Marshal(side)
	if err := os.WriteFile(filepath.Join(pack.Dir, "annotations.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
	item := readPackListing(pack.Dir)
	if item.ReviewedObjects != 1 || item.ReviewedMasks != 1 || item.Status != "reviewed" {
		t.Fatalf("inventory %+v", item)
	}
	response := callSegment(t, ws, "GET", "/api/annotations/packs", nil, ws.handleAnnotationPacks)
	if response.Code != 200 {
		t.Fatalf("inventory status %d: %s", response.Code, response.Body.String())
	}
}
