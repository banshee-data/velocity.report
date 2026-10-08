package server

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// writeSceneTestVRLOG records a short recording of two tracks at 10 Hz under
// dir, so the exporter has rotations with tracks in them to project.
func writeSceneTestVRLOG(t *testing.T, dir string, frames int) string {
	t.Helper()
	vrlog := filepath.Join(dir, "run.vrlog")
	rec, err := recorder.NewRecorder(vrlog, "hesai-pandar40p")
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	for i := range frames {
		ts := int64(1_764_973_025_000_000_000) + int64(i)*100_000_000
		set := &l9endpoints.TrackSet{FrameID: uint64(i), TimestampNanos: ts}
		for _, id := range []string{"track_2", "track_9"} {
			set.Tracks = append(set.Tracks, l9endpoints.Track{
				TrackID: id, SensorID: "hesai-pandar40p", State: l9endpoints.TrackStateConfirmed,
				X: float32(i), Y: 2, Z: 0.8, SpeedMps: 10, BBoxLength: 4, BBoxWidth: 2, BBoxHeight: 1.5,
				ObjectClass: "car",
			})
		}
		if err := rec.Record(&l9endpoints.FrameBundle{
			FrameID: uint64(i), TimestampNanos: ts, SensorID: "hesai-pandar40p",
			FrameType: l9endpoints.FrameTypeForeground, Tracks: set,
		}); err != nil {
			t.Fatalf("record frame %d: %v", i, err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}
	return vrlog
}

// runSceneServer is a server with a database and a VRLOG directory, holding
// one run whose recording is vrlogPath ("" for a run without one).
func runSceneServer(t *testing.T, runID, vrlogPath, vrlogDir string) *Server {
	t.Helper()
	testDB, cleanup := setupTestDBWrapped(t)
	t.Cleanup(cleanup)
	store := sqlite.NewAnalysisRunStore(testDB)
	if err := store.InsertRun(&sqlite.AnalysisRun{
		RunID: runID, SensorID: "hesai-pandar40p", SourceType: "pcap",
		SourcePath: "/captures/kirk0.pcapng", Status: "completed", VRLogPath: vrlogPath,
	}); err != nil {
		t.Fatalf("insert run: %v", err)
	}
	return &Server{db: testDB, vrlogSafeDir: vrlogDir}
}

func getRunScene(ws *Server, runID, rel string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/"+runID+"/scene/"+rel, nil)
	rec := httptest.NewRecorder()
	ws.handleRunTrackAPI(rec, req)
	return rec
}

func TestRunSceneServesAnExportOfTheRunsOwnRecording(t *testing.T) {
	vrlogDir := t.TempDir()
	ws := runSceneServer(t, "run-1", writeSceneTestVRLOG(t, vrlogDir, 30), vrlogDir)

	manifest := getRunScene(ws, "run-1", "manifest.json")
	if manifest.Code != http.StatusOK {
		t.Fatalf("manifest = %d: %s", manifest.Code, manifest.Body.String())
	}
	var m runSceneManifest
	if err := json.Unmarshal(manifest.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(m.Parts) != 1 || m.Parts[0].URL != "./part-000/" || m.Site.Title != "kirk0.pcapng" {
		t.Fatalf("manifest = %+v, want one part titled by the capture", m)
	}

	index := getRunScene(ws, "run-1", "part-000/index.json")
	if index.Code != http.StatusOK {
		t.Fatalf("index = %d: %s", index.Code, index.Body.String())
	}
	var idx struct {
		Chunks []struct {
			ID int `json:"c"`
		} `json:"chunks"`
	}
	if err := json.Unmarshal(index.Body.Bytes(), &idx); err != nil || len(idx.Chunks) == 0 {
		t.Fatalf("index = %s (%v), want chunks", index.Body.String(), err)
	}

	// The chunk carries the tracker's own track IDs, so a box clicked in the
	// scene names a track the run's list has.
	chunk := getRunScene(ws, "run-1", fmt.Sprintf("part-000/frames/chunk_%04d.ndjson.gz", idx.Chunks[0].ID))
	if chunk.Code != http.StatusOK {
		t.Fatalf("chunk = %d: %s", chunk.Code, chunk.Body.String())
	}
	zr, err := gzip.NewReader(chunk.Body)
	if err != nil {
		t.Fatalf("chunk is not gzip: %v", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read chunk: %v", err)
	}
	if !strings.Contains(string(raw), `"track_2"`) {
		t.Errorf("chunk does not carry the run's track IDs: %.200s", raw)
	}
}

func TestRunSceneIsNotFoundWithoutARecording(t *testing.T) {
	vrlogDir := t.TempDir()
	cases := map[string]string{
		"no recording":                "",
		"missing recording":           filepath.Join(vrlogDir, "gone.vrlog"),
		"outside the VRLOG directory": writeSceneTestVRLOG(t, t.TempDir(), 5),
	}
	for name, vrlogPath := range cases {
		t.Run(name, func(t *testing.T) {
			ws := runSceneServer(t, "run-1", vrlogPath, vrlogDir)
			if rec := getRunScene(ws, "run-1", "manifest.json"); rec.Code != http.StatusNotFound {
				t.Fatalf("manifest = %d (%s), want 404", rec.Code, rec.Body.String())
			}
		})
	}
	ws := runSceneServer(t, "run-1", "", vrlogDir)
	if rec := getRunScene(ws, "no-such-run", "manifest.json"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown run = %d, want 404", rec.Code)
	}
}

func TestRunSceneServesOnlyTheExportsFiles(t *testing.T) {
	vrlogDir := t.TempDir()
	ws := runSceneServer(t, "run-1", writeSceneTestVRLOG(t, vrlogDir, 10), vrlogDir)
	for _, rel := range []string{"part-000", "part-000/", "stamp.json", "../../run.vrlog/header.json", "nope.json"} {
		if rec := getRunScene(ws, "run-1", rel); rec.Code != http.StatusNotFound {
			t.Errorf("%q = %d, want 404", rel, rec.Code)
		}
	}
}

func TestRunSceneExportsAgainWhenTheRecordingChanges(t *testing.T) {
	vrlogDir := t.TempDir()
	vrlog := writeSceneTestVRLOG(t, vrlogDir, 10)
	ws := runSceneServer(t, "run-1", vrlog, vrlogDir)

	if rec := getRunScene(ws, "run-1", "manifest.json"); rec.Code != http.StatusOK {
		t.Fatalf("first manifest = %d", rec.Code)
	}
	dir := filepath.Join(vrlogDir, runSceneCacheDir, "run-1")
	first, ok := readRunSceneStamp(dir)
	if !ok {
		t.Fatal("no stamp after the first export")
	}

	// Unchanged: the cached export is reused.
	if rec := getRunScene(ws, "run-1", "manifest.json"); rec.Code != http.StatusOK {
		t.Fatalf("second manifest = %d", rec.Code)
	}
	if again, _ := readRunSceneStamp(dir); again.ExportedAt != first.ExportedAt || again.Signature != first.Signature {
		t.Errorf("an unchanged recording was exported again: %+v then %+v", first, again)
	}

	// Changed: a new top-level file changes the signature, and it re-exports.
	if err := os.WriteFile(filepath.Join(vrlog, "notes.txt"), []byte("grew"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := getRunScene(ws, "run-1", "manifest.json"); rec.Code != http.StatusOK {
		t.Fatalf("manifest after change = %d", rec.Code)
	}
	if changed, _ := readRunSceneStamp(dir); changed.Signature == first.Signature {
		t.Error("a changed recording kept its old export")
	}
}
