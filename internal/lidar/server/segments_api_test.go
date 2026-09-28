package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
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
		{`INSERT INTO lidar_capture_files(capture_file_id,root_id,rel_path,size_bytes,modified_at_ns,first_packet_ns,last_packet_ns,probe_state,first_seen_at_ns,last_seen_at_ns) VALUES('file','root','capture.pcap',7,1,?,?, 'ok',1,1)`, []any{segmentTestTime - 1_000_000_000, segmentTestTime + 80_000_000_000}},
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

func TestHeldOutRequiresRandomCaptureAndWorksWithoutTrackerPoints(t *testing.T) {
	ws, _ := segmentServer(t)
	getWindows := func(finder string) []segments.Window {
		t.Helper()
		response := callSegment(t, ws, "GET", "/api/lidar/segments?run_id=run&finder="+finder+"&role=held_out", nil, ws.handleSegments)
		if response.Code != 200 {
			t.Fatalf("%s list: %d %s", finder, response.Code, response.Body.String())
		}
		var listing struct {
			Windows []segments.Window `json:"windows"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
			t.Fatal(err)
		}
		return listing.Windows
	}
	following := getWindows("following")
	if len(following) != 1 {
		t.Fatalf("following: %+v", following)
	}
	choose := func(window segments.Window, finder string) *httptest.ResponseRecorder {
		t.Helper()
		return callSegment(t, ws, "POST", "/api/lidar/segments/"+window.ID+"/case", map[string]any{"run_id": "run", "finder": finder, "role": "held_out"}, ws.handleSegmentByID)
	}
	if response := choose(following[0], "following"); response.Code != 400 {
		t.Fatalf("traffic was selected before random: %d %s", response.Code, response.Body.String())
	}
	if _, err := ws.db.Exec(`DELETE FROM lidar_track_observations`); err != nil {
		t.Fatal(err)
	}
	random := getWindows("random")
	if len(random) != 1 || random[0].Capture == "" || random[0].OffsetSeconds < 35 {
		t.Fatalf("tracker-free random: %+v", random)
	}
	if response := choose(random[0], "random"); response.Code != 201 {
		t.Fatalf("random case: %d %s", response.Code, response.Body.String())
	}
	// Restore one traffic candidate so the same capture can be selected after
	// its random control case exists.
	if _, err := ws.db.Exec(`INSERT INTO lidar_track_observations(track_id,ts_unix_nanos,frame_unix_nanos,frame_id,x,y,velocity_x,velocity_y) VALUES('f',?,?,'frame',0,0,10,0),('l',?,?,'frame',10,0,10,0)`, segmentTestTime, segmentTestTime, segmentTestTime, segmentTestTime); err != nil {
		t.Fatal(err)
	}
	following = getWindows("following")
	if len(following) != 1 {
		t.Fatalf("following after random: %+v", following)
	}
	if response := choose(following[0], "following"); response.Code != 201 {
		t.Fatalf("traffic after random: %d %s", response.Code, response.Body.String())
	}
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
	var queued struct {
		Job struct {
			ID string `json:"job_id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(clip.Body.Bytes(), &queued); err != nil || queued.Job.ID == "" {
		t.Fatalf("queued job: %s %v", clip.Body.String(), err)
	}
	if err := sqlite.NewSegmentStore(ws.db).LinkPack(queued.Job.ID, "clip-"+queued.Job.ID+"-1/pack", "sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	packed := callSegment(t, ws, "GET", "/api/lidar/segments?run_id=run&finder=following", nil, ws.handleSegments)
	var packedList struct {
		Windows []segments.Window `json:"windows"`
	}
	if err := json.Unmarshal(packed.Body.Bytes(), &packedList); err != nil || len(packedList.Windows) != 1 || packedList.Windows[0].Status != "packed" {
		t.Fatalf("packed status: %+v %v", packedList, err)
	}
	// The row holds a relative path; the operator is given one to open.
	if want := filepath.Join(ws.annotationPacksDir, "clip-"+queued.Job.ID+"-1", "pack"); packedList.Windows[0].PackDir != want {
		t.Fatalf("pack directory %q, want %q", packedList.Windows[0].PackDir, want)
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
	if err := os.WriteFile(filepath.Join(pack.Dir, "segment.json"), []byte(`{"schema":"velocity.report/annotation-segment","pack_digest":"`+pack.Manifest.PackDigest+`"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if item := readPackListing(pack.Dir); item.Error == "" || item.SegmentID != "" {
		t.Fatalf("accepted incomplete segment provenance: %+v", item)
	}
}

func TestSegmentCaseRequiresCompleteIndexedWindow(t *testing.T) {
	ws, _ := segmentServer(t)
	if _, err := ws.db.Exec(`UPDATE lidar_capture_files SET last_packet_ns=? WHERE capture_file_id='file'`, segmentTestTime+4_000_000_000); err != nil {
		t.Fatal(err)
	}
	list := callSegment(t, ws, "GET", "/api/lidar/segments?run_id=run&finder=following", nil, ws.handleSegments)
	if list.Code != 200 {
		t.Fatal(list.Body.String())
	}
	var listing struct {
		Windows []segments.Window `json:"windows"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Windows) != 1 {
		t.Fatalf("candidate count %d", len(listing.Windows))
	}
	response := callSegment(t, ws, "POST", "/api/lidar/segments/"+listing.Windows[0].ID+"/case", map[string]any{"run_id": "run", "finder": "following", "role": "tuning"}, ws.handleSegmentByID)
	if response.Code != 400 {
		t.Fatalf("partial capture accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestSegmentEndpointsRejectInvalidQueriesAndMethods(t *testing.T) {
	ws, _ := segmentServer(t)
	for _, tc := range []struct {
		name, method, path string
		handler            http.HandlerFunc
		want               int
	}{
		{"finders", "GET", "/api/lidar/segments/finders", ws.handleSegmentFinders, 200},
		{"finders method", "POST", "/api/lidar/segments/finders", ws.handleSegmentFinders, 405},
		{"list method", "POST", "/api/lidar/segments?run_id=run", ws.handleSegments, 405},
		{"missing run", "GET", "/api/lidar/segments", ws.handleSegments, 400},
		{"unknown run", "GET", "/api/lidar/segments?run_id=missing", ws.handleSegments, 400},
		{"bad width", "GET", "/api/lidar/segments?run_id=run&window_seconds=nan", ws.handleSegments, 400},
		{"bad role", "GET", "/api/lidar/segments?run_id=run&role=unknown", ws.handleSegments, 400},
		{"strip method", "POST", "/api/lidar/segments/strip?run_id=run", ws.handleSegmentStrip, 405},
		{"bad seed", "GET", "/api/lidar/segments/strip?run_id=run&seed=oops", ws.handleSegmentStrip, 400},
		{"strip missing run", "GET", "/api/lidar/segments/strip", ws.handleSegmentStrip, 400},
		{"bad case path", "POST", "/api/lidar/segments/none/wrong", ws.handleSegmentByID, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := callSegment(t, ws, tc.method, tc.path, nil, tc.handler)
			if response.Code != tc.want {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
		})
	}
	ws.annotationPacksDir = ""
	if response := callSegment(t, ws, "GET", "/api/annotations/packs", nil, ws.handleAnnotationPacks); response.Code != 501 {
		t.Fatalf("unconfigured packs: %d", response.Code)
	}
	if response := callSegment(t, ws, "POST", "/api/lidar/scenes/missing/clip", nil, func(w http.ResponseWriter, r *http.Request) { ws.handleSceneClip(w, r, "missing") }); response.Code != 501 {
		t.Fatalf("unconfigured clip: %d", response.Code)
	}
	ws.annotationPacksDir = t.TempDir()
	clipHandler := func(w http.ResponseWriter, r *http.Request) { ws.handleSceneClip(w, r, "missing") }
	if response := callSegment(t, ws, "POST", "/api/lidar/scenes/missing/clip", nil, clipHandler); response.Code != 404 {
		t.Fatalf("unknown case: %d", response.Code)
	}
	if err := ws.db.Close(); err != nil {
		t.Fatal(err)
	}
	if response := callSegment(t, ws, "POST", "/api/lidar/scenes/missing/clip", nil, clipHandler); response.Code != 500 {
		t.Fatalf("database failure was reported as missing case: %d", response.Code)
	}
}

func TestClipQueueReportsWriteFailureAndFailsUnlinkedJob(t *testing.T) {
	makeCase := func(t *testing.T, ws *Server) string {
		t.Helper()
		list := callSegment(t, ws, "GET", "/api/lidar/segments?run_id=run&finder=following", nil, ws.handleSegments)
		var listing struct {
			Windows []segments.Window `json:"windows"`
		}
		if err := json.Unmarshal(list.Body.Bytes(), &listing); err != nil || len(listing.Windows) != 1 {
			t.Fatalf("listing: %+v %v", listing, err)
		}
		created := callSegment(t, ws, "POST", "/api/lidar/segments/"+listing.Windows[0].ID+"/case", map[string]any{"run_id": "run", "finder": "following", "role": "tuning"}, ws.handleSegmentByID)
		if created.Code != 201 {
			t.Fatalf("case: %d %s", created.Code, created.Body.String())
		}
		var result struct {
			ReplayCaseID string `json:"replay_case_id"`
		}
		if err := json.Unmarshal(created.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.ReplayCaseID
	}
	queue := func(t *testing.T, ws *Server, id string) *httptest.ResponseRecorder {
		t.Helper()
		return callSegment(t, ws, "POST", "/api/lidar/scenes/"+id+"/clip", nil, func(w http.ResponseWriter, r *http.Request) { ws.handleSceneClip(w, r, id) })
	}
	t.Run("job insertion", func(t *testing.T) {
		ws, _ := segmentServer(t)
		id := makeCase(t, ws)
		if _, err := ws.db.Exec(`PRAGMA query_only=ON`); err != nil {
			t.Fatal(err)
		}
		if result := queue(t, ws, id); result.Code != 500 {
			t.Fatalf("read-only job queue: %d %s", result.Code, result.Body.String())
		}
	})
	t.Run("selection link", func(t *testing.T) {
		ws, _ := segmentServer(t)
		id := makeCase(t, ws)
		if _, err := ws.db.Exec(`CREATE TRIGGER reject_segment_job BEFORE INSERT ON lidar_segment_clip_jobs BEGIN SELECT RAISE(ABORT,'forced link failure'); END`); err != nil {
			t.Fatal(err)
		}
		if result := queue(t, ws, id); result.Code != 500 {
			t.Fatalf("unlinked job: %d %s", result.Code, result.Body.String())
		}
		// The job is written with its link or not at all, so there is no
		// job left for a worker to claim and fail.
		var jobs int
		if err := ws.db.QueryRow(`SELECT COUNT(*) FROM lidar_capture_jobs WHERE kind='vrlog_record'`).Scan(&jobs); err != nil || jobs != 0 {
			t.Fatalf("%d clip job(s) left without a segment: %v", jobs, err)
		}
	})
}

func TestSegmentRequestDefaultsAndEvidenceFailures(t *testing.T) {
	req, err := querySegmentRequest(httptest.NewRequest("GET", "/api/lidar/segments?run_id=run&seed=17&window_seconds=5", nil))
	if err != nil || req.Finder != "following" || req.Role != "tuning" || req.Parameters.RandomSeed != 17 || req.Parameters.WindowSeconds != 5 {
		t.Fatalf("defaults and parameters: %+v %v", req, err)
	}
	if _, err := querySegmentRequest(httptest.NewRequest("GET", "/api/lidar/segments?window_seconds=oops", nil)); err == nil {
		t.Fatal("invalid width accepted")
	}
	ws, _ := segmentServer(t)
	if windows, _, err := ws.findRunSegments(segmentRequest{RunID: "run"}); err != nil || len(windows) != 1 {
		t.Fatalf("default finder: %+v %v", windows, err)
	}
	bad := segments.DefaultParams()
	bad.WindowSeconds = -1
	if _, _, err := ws.findRunSegments(segmentRequest{RunID: "run", Finder: "following", Parameters: &bad}); err == nil {
		t.Fatal("invalid finder parameters accepted")
	}
	for _, finder := range []string{"following", "random"} {
		t.Run("missing capture index "+finder, func(t *testing.T) {
			ws, _ := segmentServer(t)
			if _, err := ws.db.Exec(`ALTER TABLE lidar_capture_files RENAME COLUMN probe_state TO broken_probe_state`); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ws.findRunSegments(segmentRequest{RunID: "run", Finder: finder}); err == nil {
				t.Fatal("missing capture index schema accepted")
			}
		})
	}
	t.Run("unreadable observations", func(t *testing.T) {
		ws, _ := segmentServer(t)
		if _, err := ws.db.Exec(`DROP TABLE lidar_track_observations`); err != nil {
			t.Fatal(err)
		}
		if _, _, err := ws.findRunSegments(segmentRequest{RunID: "run"}); err == nil {
			t.Fatal("missing run observations accepted")
		}
	})
	if response := callSegment(t, ws, "GET", "/api/lidar/segments?run_id=run&window_seconds=oops", nil, ws.handleSegments); response.Code != 400 {
		t.Fatalf("malformed list query: %d", response.Code)
	}
}

func TestSegmentCaptureScopeUsesReplayCaseFiles(t *testing.T) {
	ws, capture := segmentServer(t)
	scene := &sqlite.ReplayCase{SensorID: "sensor", PCAPFile: filepath.Base(capture)}
	store := sqlite.NewReplayCaseStore(ws.db)
	if err := store.InsertScene(scene); err != nil {
		t.Fatal(err)
	}
	run := &sqlite.AnalysisRun{ReplayCaseID: scene.ReplayCaseID}
	captures, err := ws.capturesForRun(run, segmentTestTime, segmentTestTime+10_000_000_000)
	if err != nil || len(captures) != 1 || captures[0].Path != capture {
		t.Fatalf("case capture scope: %+v %v", captures, err)
	}
	run.ReplayCaseID = "missing"
	if _, err := ws.capturesForRun(run, segmentTestTime, segmentTestTime+10_000_000_000); err == nil {
		t.Fatal("missing replay case accepted")
	}
	if resolved, err := ws.resolveSegmentCapture(filepath.Base(capture)); err != nil || resolved != capture {
		t.Fatalf("relative capture: %q %v", resolved, err)
	}
}

func TestSegmentStripPlotsTimeAndBoundsOutput(t *testing.T) {
	name := "/captures/" + strings.Repeat("long-name-", 5) + ".pcap"
	windows := []segments.Window{{Capture: name, StartNs: 20, Score: 4}, {Capture: name, StartNs: 10, Score: 1}, {StartNs: 30, Score: 0}}
	w := httptest.NewRecorder()
	if err := writeSegmentStrip(w, windows); err != nil {
		t.Fatal(err)
	}
	svg := w.Body.String()
	if !strings.Contains(svg, "unplaced") || !strings.Contains(svg, "...") || !strings.Contains(svg, `x="210"`) || !strings.Contains(svg, `x="790"`) {
		t.Fatalf("strip lost empty source, clipped label or time placement: %s", svg)
	}
	tooMany := make([]segments.Window, 101)
	for i := range tooMany {
		tooMany[i].Capture = fmt.Sprintf("capture-%d.pcap", i)
	}
	w = httptest.NewRecorder()
	if err := writeSegmentStrip(w, tooMany); err == nil || w.Body.Len() != 0 {
		t.Fatalf("oversized strip was rendered: %v", err)
	}
}

func firstFollowingSegment(t *testing.T, ws *Server, role string) segments.Window {
	t.Helper()
	list := callSegment(t, ws, "GET", "/api/lidar/segments?run_id=run&finder=following&role="+role, nil, ws.handleSegments)
	if list.Code != 200 {
		t.Fatalf("segment list: %d %s", list.Code, list.Body.String())
	}
	var body struct {
		Windows []segments.Window `json:"windows"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &body); err != nil || len(body.Windows) != 1 {
		t.Fatalf("segment list: %+v %v", body, err)
	}
	return body.Windows[0]
}

func TestSegmentCaseRejectsMalformedRequestsAndUnplacedWindows(t *testing.T) {
	ws, _ := segmentServer(t)
	chosen := firstFollowingSegment(t, ws, "tuning")
	path := "/api/lidar/segments/" + chosen.ID + "/case"
	if response := callSegment(t, ws, "POST", path, nil, ws.handleSegmentByID); response.Code != 400 {
		t.Fatalf("empty request: %d", response.Code)
	}
	if response := callSegment(t, ws, "POST", path, map[string]any{"run_id": "run", "finder": "following"}, ws.handleSegmentByID); response.Code != 201 {
		t.Fatalf("default tuning role: %d %s", response.Code, response.Body.String())
	}
	ws, _ = segmentServer(t)
	if _, err := ws.db.Exec(`DELETE FROM lidar_capture_files`); err != nil {
		t.Fatal(err)
	}
	chosen = firstFollowingSegment(t, ws, "tuning")
	if chosen.Capture != "" {
		t.Fatalf("unindexed window was placed: %+v", chosen)
	}
	path = "/api/lidar/segments/" + chosen.ID + "/case"
	if response := callSegment(t, ws, "POST", path, map[string]any{"run_id": "run", "finder": "following"}, ws.handleSegmentByID); response.Code != 400 {
		t.Fatalf("unplaced case: %d", response.Code)
	}
}

func TestSegmentCaseStoresTheFinderAndRoleThatRankedTheWindow(t *testing.T) {
	ws, _ := segmentServer(t)
	chosen := firstFollowingSegment(t, ws, "tuning")
	// Neither finder nor role is sent: both defaults must reach the stored row,
	// or the clip job later refuses the selection as drifted.
	response := callSegment(t, ws, "POST", "/api/lidar/segments/"+chosen.ID+"/case", map[string]any{"run_id": "run"}, ws.handleSegmentByID)
	if response.Code != 201 {
		t.Fatalf("default finder and role: %d %s", response.Code, response.Body.String())
	}
	var finder, role, windowJSON string
	if err := ws.db.QueryRow(`SELECT finder,role,window_json FROM lidar_segment_selections WHERE segment_id=?`, chosen.ID).Scan(&finder, &role, &windowJSON); err != nil {
		t.Fatal(err)
	}
	var stored segments.Window
	if err := json.Unmarshal([]byte(windowJSON), &stored); err != nil {
		t.Fatal(err)
	}
	if finder != "following" || role != "tuning" || stored.Finder != finder || stored.Role != role {
		t.Fatalf("stored finder %q role %q, window finder %q role %q", finder, role, stored.Finder, stored.Role)
	}
	if stored.ID != segments.Identity(finder, stored.Source, role, segments.DefaultParams(), stored.StartNs) {
		t.Fatalf("stored selection cannot reproduce its own identity: %+v", stored)
	}
	// The row names the selector that ranked the window, as it ran.
	selection, err := sqlite.NewSegmentStore(ws.db).Selection(chosen.ID)
	if err != nil {
		t.Fatal(err)
	}
	var chosenBy segments.SelectorProvenance
	if err := json.Unmarshal([]byte(selection.SelectorJSON), &chosenBy); err != nil {
		t.Fatal(err)
	}
	catalogue, err := segments.DefaultCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	following, _ := catalogue.Selector("following")
	if !reflect.DeepEqual(chosenBy, following.Provenance()) {
		t.Fatalf("stored selector %+v, want %+v", chosenBy, following.Provenance())
	}
}

// The server ranks with the catalogue it was given, and says so when it has
// none to rank with.
func TestSegmentsRankWithTheServersCatalogue(t *testing.T) {
	ws, _ := segmentServer(t)
	catalogue, err := segments.DefaultCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	// The catalogue a server was started with may lack what the default has.
	given := *catalogue
	given.Selectors = nil
	for _, s := range catalogue.Selectors {
		if s.ID != "split_flags" {
			given.Selectors = append(given.Selectors, s)
		}
	}
	ws.segmentSelectors = &given
	if _, _, err := ws.findRunSegments(segmentRequest{RunID: "run", Finder: "split_flags"}); err == nil || !strings.Contains(err.Error(), `unknown finder "split_flags"`) {
		t.Fatalf("a selector the server's catalogue does not have: %v", err)
	}
	if windows, sel, err := ws.findRunSegments(segmentRequest{RunID: "run"}); err != nil || len(windows) != 1 || sel.ID != "following" {
		t.Fatalf("the server's catalogue: %+v %+v %v", windows, sel, err)
	}
	// With no catalogue given, and none to be found or embedded, nothing is
	// ranked rather than something ranked some other way.
	ws.segmentSelectors = nil
	t.Chdir(t.TempDir())
	if _, _, err := ws.findRunSegments(segmentRequest{RunID: "run"}); err == nil || !strings.Contains(err.Error(), "segment selectors") {
		t.Fatalf("no catalogue: %v", err)
	}
}

func TestSegmentCaseReportsDatabaseWriteFailures(t *testing.T) {
	for _, tc := range []struct {
		name, sql, role string
	}{
		{"held-out guard query", `ALTER TABLE lidar_segment_selections RENAME COLUMN role TO broken_role`, "held_out"},
		{"insert replay case", `CREATE TRIGGER fail_segment_case BEFORE INSERT ON lidar_replay_cases BEGIN SELECT RAISE(ABORT,'forced case failure'); END`, "tuning"},
		{"link case files", `CREATE TRIGGER fail_segment_files BEFORE INSERT ON lidar_replay_case_files BEGIN SELECT RAISE(ABORT,'forced file failure'); END`, "tuning"},
		{"save selection", `CREATE TRIGGER fail_segment_selection BEFORE INSERT ON lidar_segment_selections BEGIN SELECT RAISE(ABORT,'forced selection failure'); END`, "tuning"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, _ := segmentServer(t)
			chosen := firstFollowingSegment(t, ws, tc.role)
			if _, err := ws.db.Exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			body := map[string]any{"run_id": "run", "finder": "following", "role": tc.role}
			response := callSegment(t, ws, "POST", "/api/lidar/segments/"+chosen.ID+"/case", body, ws.handleSegmentByID)
			if response.Code != 500 {
				t.Fatalf("write failure returned %d: %s", response.Code, response.Body.String())
			}
			if tc.name == "link case files" || tc.name == "save selection" {
				var count int
				if err := ws.db.QueryRow(`SELECT COUNT(*) FROM lidar_replay_cases`).Scan(&count); err != nil || count != 0 {
					t.Fatalf("orphaned replay case: %d %v", count, err)
				}
			}
		})
	}
}

func TestSegmentCaseValidatesMultiCaptureSeams(t *testing.T) {
	for _, tc := range []struct {
		name       string
		secondFrom int64
		want       int
	}{
		{"continuous", segmentTestTime + 5_000_000_000, 201},
		{"broken seam", segmentTestTime + 8_000_000_000, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, first := segmentServer(t)
			second := filepath.Join(filepath.Dir(first), "second.pcap")
			if err := os.WriteFile(second, []byte("fixture"), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := ws.db.Exec(`UPDATE lidar_capture_files SET last_packet_ns=? WHERE capture_file_id='file'`, segmentTestTime+5_000_000_000); err != nil {
				t.Fatal(err)
			}
			if _, err := ws.db.Exec(`INSERT INTO lidar_capture_files(capture_file_id,root_id,rel_path,size_bytes,modified_at_ns,first_packet_ns,last_packet_ns,probe_state,first_seen_at_ns,last_seen_at_ns) VALUES('second','root','second.pcap',7,1,?,?, 'ok',1,1)`, tc.secondFrom, segmentTestTime+80_000_000_000); err != nil {
				t.Fatal(err)
			}
			store := sqlite.NewReplayCaseStore(ws.db)
			scene := &sqlite.ReplayCase{SensorID: "sensor", PCAPFile: filepath.Base(first)}
			if err := store.InsertScene(scene); err != nil {
				t.Fatal(err)
			}
			if err := store.SetCaseFiles(scene.ReplayCaseID, []sqlite.ReplayCaseFile{{Ordinal: 0, PCAPFile: filepath.Base(first)}, {Ordinal: 1, PCAPFile: filepath.Base(second)}}); err != nil {
				t.Fatal(err)
			}
			if _, err := ws.db.Exec(`UPDATE lidar_run_records SET replay_case_id=? WHERE run_id='run'`, scene.ReplayCaseID); err != nil {
				t.Fatal(err)
			}
			chosen := firstFollowingSegment(t, ws, "tuning")
			body := map[string]any{"run_id": "run", "finder": "following", "role": "tuning"}
			response := callSegment(t, ws, "POST", "/api/lidar/segments/"+chosen.ID+"/case", body, ws.handleSegmentByID)
			if response.Code != tc.want {
				t.Fatalf("multi-capture case: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSegmentCaseRejectsCaptureChangesBetweenListAndSelection(t *testing.T) {
	ws, _ := segmentServer(t)
	chosen := firstFollowingSegment(t, ws, "tuning")
	request := func(ops segmentCaseOperations) *httptest.ResponseRecorder {
		t.Helper()
		path := "/api/lidar/segments/" + chosen.ID + "/case"
		return callSegment(t, ws, "POST", path, map[string]any{"run_id": "run", "finder": "following", "role": "tuning"}, func(w http.ResponseWriter, r *http.Request) {
			ws.handleSegmentByIDWith(w, r, ops)
		})
	}
	base := segmentCaseOperations{sqlite.NewAnalysisRunStore(ws.db).GetRun, ws.capturesForRun, ws.resolveSegmentCapture}
	for _, tc := range []struct {
		name string
		edit func(*segmentCaseOperations)
		want int
	}{
		{"run removed", func(ops *segmentCaseOperations) {
			ops.getRun = func(string) (*sqlite.AnalysisRun, error) { return nil, errors.New("run removed") }
		}, 404},
		{"capture index failed", func(ops *segmentCaseOperations) {
			ops.captures = func(*sqlite.AnalysisRun, int64, int64) ([]segments.Capture, error) {
				return nil, errors.New("index failed")
			}
		}, 400},
		{"captures removed", func(ops *segmentCaseOperations) {
			ops.captures = func(*sqlite.AnalysisRun, int64, int64) ([]segments.Capture, error) { return nil, nil }
		}, 400},
		{"capture changed", func(ops *segmentCaseOperations) {
			ops.resolve = func(string) (string, error) { return "", errors.New("capture changed") }
		}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ops := base
			tc.edit(&ops)
			if response := request(ops); response.Code != tc.want {
				t.Fatalf("case after source change: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSegmentStripEndpointRejectsExcessWindows(t *testing.T) {
	ws, _ := segmentServer(t)
	windows := make([]segments.Window, 101)
	for i := range windows {
		windows[i].Capture = fmt.Sprintf("capture-%d.pcap", i)
	}
	req := httptest.NewRequest("GET", "/api/lidar/segments/strip?run_id=run", nil)
	w := httptest.NewRecorder()
	ws.handleSegmentStripWith(w, req, func(segmentRequest) ([]segments.Window, segments.Selector, error) {
		return windows, segments.Selector{}, nil
	})
	if w.Code != 413 {
		t.Fatalf("oversized strip status: %d %s", w.Code, w.Body.String())
	}
}

func TestSceneClipRouteQueuesOnlyOnPost(t *testing.T) {
	ws, _ := segmentServer(t)
	chosen := firstFollowingSegment(t, ws, "tuning")
	created := callSegment(t, ws, "POST", "/api/lidar/segments/"+chosen.ID+"/case", map[string]any{"run_id": "run"}, ws.handleSegmentByID)
	var selected struct {
		ReplayCaseID string `json:"replay_case_id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &selected); err != nil || selected.ReplayCaseID == "" {
		t.Fatalf("case: %d %s", created.Code, created.Body.String())
	}
	path := "/api/lidar/scenes/" + selected.ReplayCaseID + "/clip"
	if response := callSegment(t, ws, "GET", path, nil, ws.handleSceneByID); response.Code != 405 {
		t.Fatalf("clip by GET: %d %s", response.Code, response.Body.String())
	}
	response := callSegment(t, ws, "POST", path, nil, ws.handleSceneByID)
	if response.Code != 202 {
		t.Fatalf("clip by POST: %d %s", response.Code, response.Body.String())
	}
	var queued struct {
		SegmentID string `json:"segment_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &queued); err != nil || queued.SegmentID != chosen.ID {
		t.Fatalf("queued clip names segment %q, want %q: %v", queued.SegmentID, chosen.ID, err)
	}
}

func TestSegmentCaptureOutsideTheSafeDirectoryIsRefused(t *testing.T) {
	ws, _ := segmentServer(t)
	chosen := firstFollowingSegment(t, ws, "tuning")
	// A capture path that cannot be expressed under the safe directory must
	// stop the case, even if the index and the resolver both accepted it.
	ops := segmentCaseOperations{
		getRun: sqlite.NewAnalysisRunStore(ws.db).GetRun,
		captures: func(*sqlite.AnalysisRun, int64, int64) ([]segments.Capture, error) {
			return []segments.Capture{{Path: "relative.pcap", FirstNs: chosen.StartNs, LastNs: chosen.EndNs}}, nil
		},
		resolve: func(path string) (string, error) { return path, nil },
	}
	response := callSegment(t, ws, "POST", "/api/lidar/segments/"+chosen.ID+"/case", map[string]any{"run_id": "run"}, func(w http.ResponseWriter, r *http.Request) {
		ws.handleSegmentByIDWith(w, r, ops)
	})
	if response.Code != 400 {
		t.Fatalf("capture outside the safe directory: %d %s", response.Code, response.Body.String())
	}
	var cases int
	if err := ws.db.QueryRow(`SELECT COUNT(*) FROM lidar_replay_cases`).Scan(&cases); err != nil || cases != 0 {
		t.Fatalf("refused capture still made %d replay cases: %v", cases, err)
	}
	ws.pcapSafeDir = "relative-safe-directory"
	if resolved, err := ws.resolveSegmentCapture("/absolute/capture.pcap"); err == nil {
		t.Fatalf("absolute capture resolved against a relative safe directory: %q", resolved)
	}
}

// replayedRun adds a run as an analysis replay leaves it: track summaries
// and a recording, and nothing in the observation table.
func replayedRun(t *testing.T, ws *Server, runID string) string {
	t.Helper()
	safe, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ws.vrlogSafeDir = safe
	recording := filepath.Join(safe, runID)
	rec, err := recorder.NewRecorder(recording, "sensor")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		ts := segmentTestTime + int64(i)*100_000_000
		tracks := []l9endpoints.Track{
			{TrackID: runID + "-follower", State: l9endpoints.TrackStateConfirmed, X: float32(i), VX: 10, MaxSpeedMps: 10},
			{TrackID: runID + "-leader", State: l9endpoints.TrackStateConfirmed, X: float32(i) + 10, VX: 10, MaxSpeedMps: 10},
		}
		if err := rec.Record(&l9endpoints.FrameBundle{FrameID: uint64(i), TimestampNanos: ts, SensorID: "sensor", Tracks: &l9endpoints.TrackSet{TimestampNanos: ts, Tracks: tracks}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(ws.pcapSafeDir, "capture.pcap")
	if _, err := ws.db.Exec(`INSERT INTO lidar_run_records(run_id,created_at,source_type,source_path,sensor_id,status,duration_secs,total_frames,total_clusters,total_tracks,confirmed_tracks,processing_time_ms,vrlog_path) VALUES(?,2,'pcap',?,'sensor','completed',0,0,0,0,0,0,?)`, runID, capture, recording); err != nil {
		t.Fatal(err)
	}
	for _, track := range []struct {
		id    string
		split int
	}{{runID + "-follower", 0}, {runID + "-leader", 1}} {
		if _, err := ws.db.Exec(`INSERT INTO lidar_run_tracks(run_id,track_id,sensor_id,track_state,start_unix_nanos,is_split_candidate) VALUES(?,?,'sensor','confirmed',?,?)`, runID, track.id, segmentTestTime, track.split); err != nil {
			t.Fatal(err)
		}
	}
	return recording
}

func TestSegmentsReadAReplayedRunFromItsRecording(t *testing.T) {
	ws, capture := segmentServer(t)
	replayedRun(t, ws, "replayed")
	var stored int
	if err := ws.db.QueryRow(`SELECT COUNT(*) FROM lidar_track_observations o JOIN lidar_run_tracks r ON r.track_id=o.track_id WHERE r.run_id='replayed'`).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("fixture must store no observations for the replayed run: %d %v", stored, err)
	}
	following, _, err := ws.findRunSegments(segmentRequest{RunID: "replayed", Finder: "following"})
	if err != nil {
		t.Fatal(err)
	}
	if len(following) != 1 || following[0].PairFrames != 5 || following[0].Capture != capture || following[0].Source != "replayed" {
		t.Fatalf("replayed run was not ranked from its recording: %+v", following)
	}
	// The marks are on the run's track summaries, which a replay does write.
	flagged, _, err := ws.findRunSegments(segmentRequest{RunID: "replayed", Finder: "split_flags"})
	if err != nil || len(flagged) != 1 || flagged[0].Events != 5 || len(flagged[0].TrackIDs) != 1 || flagged[0].TrackIDs[0] != "replayed-leader" {
		t.Fatalf("split marks of a replayed run: %+v %v", flagged, err)
	}
	// The whole flow works from the recording: the window can be chosen.
	response := callSegment(t, ws, "POST", "/api/lidar/segments/"+following[0].ID+"/case", map[string]any{"run_id": "replayed", "finder": "following"}, ws.handleSegmentByID)
	if response.Code != 201 {
		t.Fatalf("case from a replayed run: %d %s", response.Code, response.Body.String())
	}
	// A run that stored its observations is still read from them.
	live, _, err := ws.findRunSegments(segmentRequest{RunID: "run", Finder: "following"})
	if err != nil || len(live) != 1 || live[0].PairFrames != 5 {
		t.Fatalf("run with stored observations: %+v %v", live, err)
	}
}

func TestSegmentsReportAReplayedRunThatCannotBeRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*testing.T, *Server, string)
		want string
	}{
		{"recording removed", func(t *testing.T, _ *Server, recording string) {
			if err := os.RemoveAll(recording); err != nil {
				t.Fatal(err)
			}
		}, "recording could not be read"},
		{"recording outside the read boundary", func(t *testing.T, ws *Server, _ string) {
			elsewhere, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			ws.vrlogSafeDir = elsewhere
		}, "not within the allowed directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, _ := segmentServer(t)
			recording := replayedRun(t, ws, "replayed")
			tc.edit(t, ws, recording)
			response := callSegment(t, ws, "GET", "/api/lidar/segments?run_id=replayed&finder=following", nil, ws.handleSegments)
			if response.Code != 400 || !strings.Contains(response.Body.String(), tc.want) {
				t.Fatalf("unreadable replayed run: %d %s", response.Code, response.Body.String())
			}
		})
	}
	// A run with neither observations nor a recording has nothing to rank.
	ws, _ := segmentServer(t)
	if _, err := ws.db.Exec(`DELETE FROM lidar_track_observations`); err != nil {
		t.Fatal(err)
	}
	windows, _, err := ws.findRunSegments(segmentRequest{RunID: "run", Finder: "following"})
	if err != nil || len(windows) != 0 {
		t.Fatalf("run without a series: %+v %v", windows, err)
	}
}

// A window whose selection state cannot be read is not offered as a fresh
// candidate: an operator who chose it again would make a second replay case.
func TestSegmentsAreNotRankedWhenTheirStateCannotBeRead(t *testing.T) {
	ws, _ := segmentServer(t)
	if _, err := ws.db.Exec(`ALTER TABLE lidar_segment_clip_jobs RENAME COLUMN pack_dir TO broken_pack_dir`); err != nil {
		t.Fatal(err)
	}
	windows, _, err := ws.findRunSegments(segmentRequest{RunID: "run"})
	if err == nil || !strings.Contains(err.Error(), "read segment status") || windows != nil {
		t.Fatalf("unreadable selection state: %+v %v", windows, err)
	}
	response := callSegment(t, ws, "GET", "/api/lidar/segments?run_id=run", nil, ws.handleSegments)
	if response.Code != 400 || !strings.Contains(response.Body.String(), "read segment status") {
		t.Fatalf("unreadable selection state: %d %s", response.Code, response.Body.String())
	}
}
