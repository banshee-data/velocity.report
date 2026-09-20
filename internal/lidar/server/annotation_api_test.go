package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// writeTestVRLOG records a small synthetic recording with point clouds, so
// annotation.Export has something real to cut a pack from. Mirrors the
// fixture in internal/lidar/annotation/export_test.go, which is unexported
// and so not importable here.
func writeTestVRLOG(t *testing.T, frameCount int) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vrlog")
	rec, err := recorder.NewRecorder(dir, "annotation-export-test")
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	for i := 0; i < frameCount; i++ {
		n := 3
		pc := &l9endpoints.PointCloudFrame{
			FrameID: uint64(i), TimestampNanos: int64(i+1) * 1_000_000_000, SensorID: "annotation-export-test",
			X: make([]float32, n), Y: make([]float32, n), Z: make([]float32, n),
			Intensity: make([]uint8, n), Classification: make([]uint8, n),
			PointCount: n,
		}
		bundle := &l9endpoints.FrameBundle{
			TimestampNanos: pc.TimestampNanos, PointCloud: pc,
		}
		if err := rec.Record(bundle); err != nil {
			t.Fatalf("record frame %d: %v", i, err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}
	return dir
}

// insertRunWithVRLog is setupTestRun plus a VRLogPath, since the export
// endpoint's whole reason to run is a run that has one.
func insertRunWithVRLog(t *testing.T, store *sqlite.AnalysisRunStore, runID, vrlogPath string) {
	t.Helper()
	run := &sqlite.AnalysisRun{
		RunID: runID, SensorID: "annotation-export-test", SourceType: "pcap",
		SourcePath: "/test/data.pcap", Status: "completed", VRLogPath: vrlogPath,
	}
	if err := store.InsertRun(run); err != nil {
		t.Fatalf("insert run: %v", err)
	}
}

func postAnnotationExport(ws *Server, runID string, body map[string]any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/annotation-export", reader)
	rec := httptest.NewRecorder()
	ws.handleAnnotationExport(rec, req, runID)
	return rec
}

func postAnnotationExportRaw(ws *Server, runID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/"+runID+"/annotation-export", strings.NewReader(body))
	rec := httptest.NewRecorder()
	ws.handleAnnotationExport(rec, req, runID)
	return rec
}

func TestHandleAnnotationExportRequiresConfiguredDirectory(t *testing.T) {
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	// annotationPacksDir deliberately left empty: an operator who never set
	// --lidar-annotation-dir gets a clear "not configured" rather than a
	// pack written somewhere unexpected.
	ws := &Server{db: testDB}
	rec := postAnnotationExport(ws, "any-run", map[string]any{"coverage": "full"})
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleAnnotationExportRequiresPOST(t *testing.T) {
	ws := &Server{annotationPacksDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/x/annotation-export", nil)
	rec := httptest.NewRecorder()
	ws.handleAnnotationExport(rec, req, "x")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAnnotationExportRequiresCoverage(t *testing.T) {
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	ws := &Server{db: testDB, annotationPacksDir: t.TempDir()}

	// Coverage must be checked before the run is even looked up: the request
	// is malformed regardless of which run it names.
	rec := postAnnotationExport(ws, "any-run", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("coverage")) {
		t.Errorf("error body %q does not mention coverage", rec.Body.String())
	}
}

func TestHandleAnnotationExportRejectsMalformedJSONAndMissingDatabase(t *testing.T) {
	t.Run("malformed JSON", func(t *testing.T) {
		testDB, cleanup := setupTestDBWrapped(t)
		defer cleanup()
		ws := &Server{db: testDB, annotationPacksDir: t.TempDir()}
		rec := postAnnotationExportRaw(ws, "any-run", "{")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("database not configured", func(t *testing.T) {
		ws := &Server{annotationPacksDir: t.TempDir()}
		rec := postAnnotationExport(ws, "any-run", map[string]any{"coverage": "full"})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandleAnnotationExportRunNotFound(t *testing.T) {
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	ws := &Server{db: testDB, annotationPacksDir: t.TempDir()}

	rec := postAnnotationExport(ws, "nonexistent-run", map[string]any{"coverage": "full"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleAnnotationExportReportsRunLookupFailure(t *testing.T) {
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	if err := testDB.DB.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	ws := &Server{db: testDB, annotationPacksDir: t.TempDir()}
	rec := postAnnotationExport(ws, "any-run", map[string]any{"coverage": "full"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleAnnotationExportRunWithoutVRLog(t *testing.T) {
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	store := sqlite.NewAnalysisRunStore(testDB)
	setupTestRun(t, store, "no-vrlog-run") // the shared fixture sets no VRLogPath

	ws := &Server{db: testDB, annotationPacksDir: t.TempDir()}
	rec := postAnnotationExport(ws, "no-vrlog-run", map[string]any{"coverage": "full"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAnnotationExportRefusesAVRLogOutsideTheSafeDirectory(t *testing.T) {
	// A run's stored VRLogPath is trusted less than the boundary that governs
	// replay: if it ever pointed outside vrlogSafeDir, export must refuse
	// rather than read whatever is there.
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	store := sqlite.NewAnalysisRunStore(testDB)

	elsewhere := writeTestVRLOG(t, 3)
	insertRunWithVRLog(t, store, "escaping-run", elsewhere)

	ws := &Server{
		db: testDB, annotationPacksDir: t.TempDir(),
		vrlogSafeDir: t.TempDir(), // a different directory than elsewhere's parent
	}
	rec := postAnnotationExport(ws, "escaping-run", map[string]any{"coverage": "full"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAnnotationExportWritesAnOpenablePack(t *testing.T) {
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	store := sqlite.NewAnalysisRunStore(testDB)

	vrlogDir := writeTestVRLOG(t, 3)
	insertRunWithVRLog(t, store, "good-run", vrlogDir)

	packsRoot := t.TempDir()
	ws := &Server{
		db: testDB, annotationPacksDir: packsRoot,
		vrlogSafeDir: filepath.Dir(vrlogDir),
	}

	rec := postAnnotationExport(ws, "good-run", map[string]any{
		"coverage": "full", "coverage_note": "handler test",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp annotationExportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SampleCount != 3 {
		t.Errorf("sample_count = %d, want 3", resp.SampleCount)
	}
	if resp.Coverage != "full" {
		t.Errorf("coverage = %q, want full", resp.Coverage)
	}

	// The pack directory reported back must be a real, reopenable pack, and
	// it must live under the configured packs directory rather than wherever
	// the VRLOG happened to be. Resolved through symlinks on both sides: the
	// handler canonicalises via security.ResolvePathWithinDirectory, and on
	// macOS t.TempDir() itself sits under a /var -> /private/var symlink, so
	// a literal-string comparison would fail for a reason that has nothing
	// to do with the boundary this asserts.
	canonicalRoot, err := filepath.EvalSymlinks(packsRoot)
	if err != nil {
		t.Fatalf("resolve packs root: %v", err)
	}
	rel, err := filepath.Rel(canonicalRoot, resp.PackDir)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		t.Errorf("pack dir %q is not under the configured packs directory %q", resp.PackDir, canonicalRoot)
	}
	if _, err := annotation.OpenPack(resp.PackDir); err != nil {
		t.Errorf("the exported pack does not reopen: %v", err)
	}
}

// TestHandleAnnotationExportCreatesAMissingPacksDirectory is the first export
// on any machine: the configured packs directory has never been written to,
// so it does not exist. Every other test here hands the handler a t.TempDir()
// that already does, which is how a handler that validated its output path
// inside that directory before creating it passed all of them and then
// answered the first real request with a 500.
func TestHandleAnnotationExportCreatesAMissingPacksDirectory(t *testing.T) {
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	store := sqlite.NewAnalysisRunStore(testDB)

	vrlogDir := writeTestVRLOG(t, 3)
	insertRunWithVRLog(t, store, "first-export", vrlogDir)

	packsRoot := filepath.Join(t.TempDir(), "lidar", "annotation-packs")
	ws := &Server{
		db: testDB, annotationPacksDir: packsRoot,
		vrlogSafeDir: filepath.Dir(vrlogDir),
	}

	rec := postAnnotationExport(ws, "first-export", map[string]any{"coverage": "full"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a packs directory that does not exist yet; body = %s", rec.Code, rec.Body.String())
	}
	if info, err := os.Stat(packsRoot); err != nil || !info.IsDir() {
		t.Errorf("packs directory %q was not created: %v", packsRoot, err)
	}
}

func TestHandleAnnotationExportRejectsUnusablePackPaths(t *testing.T) {
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	store := sqlite.NewAnalysisRunStore(testDB)
	vrlogDir := writeTestVRLOG(t, 3)

	t.Run("packs root is a file", func(t *testing.T) {
		packsRoot := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(packsRoot, nil, 0o644); err != nil {
			t.Fatalf("create packs-root file: %v", err)
		}
		insertRunWithVRLog(t, store, "packs-root-file", vrlogDir)
		ws := &Server{db: testDB, annotationPacksDir: packsRoot, vrlogSafeDir: filepath.Dir(vrlogDir)}
		rec := postAnnotationExport(ws, "packs-root-file", map[string]any{"coverage": "full"})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})

	t.Run("stored run ID escapes packs root", func(t *testing.T) {
		const runID = "../escape-packs-root"
		insertRunWithVRLog(t, store, runID, vrlogDir)
		ws := &Server{db: testDB, annotationPacksDir: t.TempDir(), vrlogSafeDir: filepath.Dir(vrlogDir)}
		rec := postAnnotationExport(ws, runID, map[string]any{"coverage": "full"})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandleAnnotationExportMaxSamplesCapsTheExport(t *testing.T) {
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	store := sqlite.NewAnalysisRunStore(testDB)

	vrlogDir := writeTestVRLOG(t, 5)
	insertRunWithVRLog(t, store, "capped-run", vrlogDir)

	ws := &Server{
		db: testDB, annotationPacksDir: t.TempDir(),
		vrlogSafeDir: filepath.Dir(vrlogDir),
	}
	rec := postAnnotationExport(ws, "capped-run", map[string]any{
		"coverage": "full", "max_samples": 2,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp annotationExportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SampleCount != 2 {
		t.Errorf("sample_count = %d, want 2 (max_samples should cap it)", resp.SampleCount)
	}
}

func TestHandleAnnotationExportRejectsNegativeMaxSamples(t *testing.T) {
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	ws := &Server{db: testDB, annotationPacksDir: t.TempDir()}

	rec := postAnnotationExport(ws, "any-run", map[string]any{"coverage": "full", "max_samples": -1})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAnnotationExportCleansUpAFailedExport(t *testing.T) {
	// A window that excludes every frame fails after Export has already
	// created the output directory. Leaving that directory behind would make
	// a retry with the same (run, timestamp) pair — unlikely, but possible
	// within the same second — fail on "already exists" instead of the real
	// cause.
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	store := sqlite.NewAnalysisRunStore(testDB)

	vrlogDir := writeTestVRLOG(t, 3) // timestamps 1s..3s
	insertRunWithVRLog(t, store, "empty-window-run", vrlogDir)

	packsRoot := t.TempDir()
	ws := &Server{
		db: testDB, annotationPacksDir: packsRoot,
		vrlogSafeDir: filepath.Dir(vrlogDir),
	}
	rec := postAnnotationExport(ws, "empty-window-run", map[string]any{
		"coverage": "full",
		"start_ns": int64(10_000_000_000), // after every recorded frame
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want %d", rec.Code, rec.Body.String(), http.StatusBadRequest)
	}

	entries, err := os.ReadDir(packsRoot)
	if err != nil {
		t.Fatalf("read packs root: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("packs root has %d entries after a failed export, want 0 (partial directory not cleaned up)", len(entries))
	}
}

func TestHandleAnnotationExportReportsCorruptVRLOG(t *testing.T) {
	// An existing but incomplete recording is a storage/read failure, unlike an
	// empty requested time window. It must remain a server error so the UI does
	// not suggest an operator can repair it by simply changing the window.
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	store := sqlite.NewAnalysisRunStore(testDB)
	vrlogDir := filepath.Join(t.TempDir(), "corrupt-vrlog")
	if err := os.Mkdir(vrlogDir, 0o755); err != nil {
		t.Fatalf("create corrupt VRLOG directory: %v", err)
	}
	insertRunWithVRLog(t, store, "corrupt-vrlog-run", vrlogDir)

	ws := &Server{db: testDB, annotationPacksDir: t.TempDir(), vrlogSafeDir: filepath.Dir(vrlogDir)}
	rec := postAnnotationExport(ws, "corrupt-vrlog-run", map[string]any{"coverage": "full"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s, want %d", rec.Code, rec.Body.String(), http.StatusInternalServerError)
	}
}

func TestHandleAnnotationExportRoutesThroughTheDispatcher(t *testing.T) {
	// The handler tests above call handleAnnotationExport directly; this
	// pins that a real request path actually reaches it, since a dispatcher
	// typo would leave every one of those tests passing while the route
	// itself 404s.
	testDB, cleanup := setupTestDBWrapped(t)
	defer cleanup()
	store := sqlite.NewAnalysisRunStore(testDB)
	vrlogDir := writeTestVRLOG(t, 2)
	insertRunWithVRLog(t, store, "dispatch-run", vrlogDir)

	ws := &Server{
		db: testDB, annotationPacksDir: t.TempDir(),
		vrlogSafeDir: filepath.Dir(vrlogDir),
	}
	b, _ := json.Marshal(map[string]any{"coverage": "full"})
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/dispatch-run/annotation-export", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	ws.handleRunTrackAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
