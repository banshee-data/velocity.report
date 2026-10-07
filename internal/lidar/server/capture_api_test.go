package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/capindex"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func TestNormaliseCaptureRoots(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	t.Run("safe dir comes first", func(t *testing.T) {
		// An unchanged deployment must behave exactly as it did before roots
		// existed, which means the safe directory leads the list.
		got := normaliseCaptureRoots("/volumes/safe", []string{"/volumes/extra"})
		if len(got) != 2 || got[0] != "/volumes/safe" {
			t.Fatalf("roots = %v, want the safe directory first", got)
		}
	})

	t.Run("duplicates collapse", func(t *testing.T) {
		got := normaliseCaptureRoots("/volumes/safe", []string{"/volumes/safe", "/volumes/extra", "/volumes/extra"})
		if len(got) != 2 {
			t.Errorf("roots = %v, want two distinct entries", got)
		}
	})

	t.Run("blanks are dropped", func(t *testing.T) {
		got := normaliseCaptureRoots("", []string{"", "   ", "/volumes/extra"})
		if len(got) != 1 || got[0] != "/volumes/extra" {
			t.Errorf("roots = %v, want just the real one", got)
		}
	})

	t.Run("relative paths resolve", func(t *testing.T) {
		got := normaliseCaptureRoots("./captures", nil)
		if len(got) != 1 {
			t.Fatalf("roots = %v, want one", got)
		}
		if want := filepath.Join(cwd, "captures"); got[0] != want {
			t.Errorf("root = %q, want the absolute %q", got[0], want)
		}
	})

	t.Run("no configuration yields no roots", func(t *testing.T) {
		if got := normaliseCaptureRoots("", nil); len(got) != 0 {
			t.Errorf("roots = %v, want none", got)
		}
	})
}

// captureAPIServer returns a server with a migrated database and the given
// roots, plus a prober that reports a fixed five-minute extent per file.
func captureAPIServer(t *testing.T, roots ...string) *Server {
	t.Helper()
	testDB, cleanup := db.NewTestDB(t)
	t.Cleanup(cleanup)
	return &Server{
		db:           testDB,
		udpPort:      2368,
		captureRoots: normaliseCaptureRoots("", roots),
	}
}

// stubCaptureProber makes every capture look like a five-minute file starting
// at a distinct offset, so a scanned directory derives one continuous session.
func stubCaptureProber(t *testing.T) {
	t.Helper()
	original := captureProber
	t.Cleanup(func() { captureProber = original })

	const fiveMinNs = int64(300) * 1_000_000_000
	base := int64(1_788_000_000) * 1_000_000_000
	var n int64
	captureProber = func(string, int) (capindex.Extent, error) {
		first := base + n*fiveMinNs
		n++
		return capindex.Extent{
			FirstPacketNs: first,
			LastPacketNs:  first + fiveMinNs,
			PacketCount:   540000,
			UDPPort:       2368,
		}, nil
	}
}

func writeCaptures(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("capture bytes"), 0o600); err != nil {
			t.Fatalf("writing %s: %v", n, err)
		}
	}
}

func doRequest(t *testing.T, ws *Server, method, target string, handler http.HandlerFunc) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(method, target, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s = %d: %s", method, target, rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding %s response: %v (%s)", target, err, rec.Body.String())
	}
	return payload
}

func TestCaptureRootsEndpointListsConfiguredVolumes(t *testing.T) {
	dir := t.TempDir()
	ws := captureAPIServer(t, dir)

	payload := doRequest(t, ws, "GET", "/api/lidar/capture/roots", ws.handleCaptureRoots)
	if got := payload["count"]; got != float64(1) {
		t.Fatalf("count = %v, want 1", got)
	}
	roots := payload["roots"].([]any)
	root := roots[0].(map[string]any)
	if root["last_scan_state"] != sqlite.ScanStateNever {
		t.Errorf("last_scan_state = %v, want %q", root["last_scan_state"], sqlite.ScanStateNever)
	}
}

func TestCaptureScanIndexesProbesAndDerives(t *testing.T) {
	dir := t.TempDir()
	writeCaptures(t, dir, "s2_sf_3_00001.pcap", "s2_sf_3_00002.pcap", "s2_sf_3_00003.pcap")
	ws := captureAPIServer(t, dir)
	stubCaptureProber(t)

	payload := doRequest(t, ws, "POST", "/api/lidar/capture/scan", ws.handleCaptureScan)
	roots := payload["roots"].([]any)
	if len(roots) != 1 {
		t.Fatalf("scanned %d roots, want 1", len(roots))
	}
	result := roots[0].(map[string]any)
	if result["state"] != capindex.StateOK {
		t.Fatalf("state = %v (%v), want %q", result["state"], result["error"], capindex.StateOK)
	}
	if result["added"] != float64(3) {
		t.Errorf("added = %v, want 3", result["added"])
	}
	if result["probed"] != float64(3) {
		t.Errorf("probed = %v, want 3", result["probed"])
	}
	sessions := result["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("derived %d sessions, want 1 for three abutting files", len(sessions))
	}
	if got := sessions[0].(map[string]any)["file_count"]; got != float64(3) {
		t.Errorf("session file_count = %v, want 3", got)
	}

	// A second scan of an unchanged volume must find nothing and re-probe
	// nothing: probing reads whole files and is the expensive half.
	again := doRequest(t, ws, "POST", "/api/lidar/capture/scan", ws.handleCaptureScan)
	res2 := again["roots"].([]any)[0].(map[string]any)
	if res2["added"] != float64(0) || res2["changed"] != float64(0) {
		t.Errorf("re-scan reported added=%v changed=%v, want 0 and 0", res2["added"], res2["changed"])
	}
	if res2["probed"] != float64(0) {
		t.Errorf("re-scan probed %v files, want 0", res2["probed"])
	}
	if drift, _ := res2["drift"].(string); !strings.Contains(drift, "no drift") {
		t.Errorf("re-scan drift = %q, want it to report no drift", drift)
	}
}

func TestCaptureScanSkipsProbingWhenAsked(t *testing.T) {
	dir := t.TempDir()
	writeCaptures(t, dir, "a.pcap")
	ws := captureAPIServer(t, dir)
	original := captureProber
	t.Cleanup(func() { captureProber = original })
	captureProber = func(string, int) (capindex.Extent, error) {
		t.Error("probe ran despite probe=false")
		return capindex.Extent{}, nil
	}

	payload := doRequest(t, ws, "POST", "/api/lidar/capture/scan?probe=false", ws.handleCaptureScan)
	result := payload["roots"].([]any)[0].(map[string]any)
	if result["added"] != float64(1) {
		t.Errorf("added = %v, want 1", result["added"])
	}
	if result["probed"] != float64(0) {
		t.Errorf("probed = %v, want 0", result["probed"])
	}
}

func TestScheduledCaptureScanMarksTheRootRunning(t *testing.T) {
	dir := t.TempDir()
	ws := captureAPIServer(t, dir)
	store, err := ws.captureStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.syncCaptureRoots(); err != nil {
		t.Fatal(err)
	}
	roots, err := store.ListRoots()
	if err != nil {
		t.Fatal(err)
	}
	results, claimed := scheduledCaptureScans(roots, roots[0].RootID)
	if len(claimed) != 1 {
		t.Fatal("scan was not scheduled")
	}
	t.Cleanup(func() { clearScheduledRoots(claimed) })
	if len(results) != 1 || results[0].State != captureScanStateRunning {
		t.Fatalf("scheduled result = %+v, want one running root", results)
	}
	views := captureRootViews(roots, time.Now())
	if !views[0].ScanInProgress {
		t.Error("root does not report its scheduled scan")
	}
	if p := views[0].ScanProgress; p == nil || p.Phase != captureScanQueued {
		t.Errorf("scan progress = %+v, want a queued scan", p)
	}

	// Asking again while it is scheduled claims nothing, so no second
	// goroutine probes the same volume, but still answers with the root.
	again, reclaimed := scheduledCaptureScans(roots, "")
	if len(reclaimed) != 0 {
		t.Errorf("a second request claimed %d roots already scheduled, want 0", len(reclaimed))
	}
	if len(again) != 1 || !strings.Contains(again[0].Drift, "already running") {
		t.Errorf("second request answered %+v, want the root reported as already running", again)
	}

	clearScheduledRoots(claimed)
	views = captureRootViews(roots, time.Now())
	if views[0].ScanInProgress || views[0].ScanProgress != nil {
		t.Errorf("cleared root still reports a scan: %+v", views[0])
	}
}

// gatedCaptureProber holds each probe until the test releases it, so a test
// can look at a background scan while it is part-way through.
func gatedCaptureProber(t *testing.T) (entered <-chan string, release chan<- struct{}) {
	t.Helper()
	original := captureProber
	t.Cleanup(func() { captureProber = original })

	in := make(chan string, 16)
	out := make(chan struct{})
	const fiveMinNs = int64(300) * 1_000_000_000
	base := int64(1_788_000_000) * 1_000_000_000
	var n int64
	captureProber = func(absPath string, _ int) (capindex.Extent, error) {
		in <- filepath.Base(absPath)
		<-out
		first := base + n*fiveMinNs
		n++
		return capindex.Extent{FirstPacketNs: first, LastPacketNs: first + fiveMinNs,
			PacketCount: 540000, UDPPort: 2368}, nil
	}
	return in, out
}

// rootView fetches the roots listing and returns the one root it holds.
func rootView(t *testing.T, ws *Server) map[string]any {
	t.Helper()
	payload := doRequest(t, ws, "GET", "/api/lidar/capture/roots", ws.handleCaptureRoots)
	roots := payload["roots"].([]any)
	if len(roots) != 1 {
		t.Fatalf("listed %d roots, want 1", len(roots))
	}
	return roots[0].(map[string]any)
}

func waitForProbe(t *testing.T, entered <-chan string) string {
	t.Helper()
	select {
	case name := <-entered:
		return name
	case <-time.After(5 * time.Second):
		t.Fatal("the background scan never reached a probe")
		return ""
	}
}

// The Captures page polls the roots listing while a background probe runs.
// It must be able to say how far the probe has got, which capture it is
// reading, and how the scan ended once it has.
func TestBackgroundCaptureScanReportsProgress(t *testing.T) {
	dir := t.TempDir()
	writeCaptures(t, dir, "a.pcap", "b.pcap", "c.pcap")
	ws := captureAPIServer(t, dir)
	entered, release := gatedCaptureProber(t)

	rec := httptest.NewRecorder()
	ws.handleCaptureScan(rec, httptest.NewRequest("POST", "/api/lidar/capture/scan?async=true", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("async scan = %d, want 202: %s", rec.Code, rec.Body.String())
	}

	if got := waitForProbe(t, entered); got != "a.pcap" {
		t.Fatalf("first probe read %q, want a.pcap", got)
	}
	root := rootView(t, ws)
	if root["scan_in_progress"] != true {
		t.Fatalf("scan_in_progress = %v mid-probe, want true", root["scan_in_progress"])
	}
	progress, ok := root["scan_progress"].(map[string]any)
	if !ok {
		t.Fatalf("no scan_progress mid-probe: %v", root)
	}
	if progress["phase"] != captureScanProbing || progress["done"] != float64(0) ||
		progress["total"] != float64(3) || progress["current"] != "a.pcap" {
		t.Errorf("progress before the first probe = %v, want probing 0 of 3 at a.pcap", progress)
	}
	if started, _ := progress["started_at_ns"].(float64); started <= 0 {
		t.Errorf("started_at_ns = %v, want the scan's start", progress["started_at_ns"])
	}

	release <- struct{}{}
	if got := waitForProbe(t, entered); got != "b.pcap" {
		t.Fatalf("second probe read %q, want b.pcap", got)
	}
	progress = rootView(t, ws)["scan_progress"].(map[string]any)
	if progress["done"] != float64(1) || progress["probed"] != float64(1) ||
		progress["current"] != "b.pcap" {
		t.Errorf("progress after one probe = %v, want 1 done, 1 probed, reading b.pcap", progress)
	}
	if elapsed, _ := progress["probe_elapsed_ns"].(float64); elapsed <= 0 {
		t.Errorf("probe_elapsed_ns = %v, want time since the first read began", progress["probe_elapsed_ns"])
	}

	release <- struct{}{}
	waitForProbe(t, entered)
	release <- struct{}{}

	deadline := time.Now().Add(5 * time.Second)
	for {
		root = rootView(t, ws)
		if root["scan_in_progress"] != true {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background scan never finished")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, still := root["scan_progress"]; still {
		t.Errorf("a finished scan still reports progress: %v", root["scan_progress"])
	}
	result, ok := root["last_scan_result"].(map[string]any)
	if !ok {
		t.Fatalf("no last_scan_result after the scan: %v", root)
	}
	if result["probed"] != float64(3) || result["added"] != float64(3) || result["probe"] != true {
		t.Errorf("last_scan_result = %v, want a probe that added and probed 3", result)
	}
	if finished, _ := result["finished_at_ns"].(float64); finished <= 0 {
		t.Errorf("finished_at_ns = %v, want when it ended", result["finished_at_ns"])
	}
	if _, has := result["sessions"]; has {
		t.Error("last_scan_result carries the derived sessions; the sessions route serves those")
	}

	sessions := doRequest(t, ws, "GET", "/api/lidar/capture/sessions", ws.handleCaptureSessions)
	if sessions["count"] != float64(1) {
		t.Errorf("sessions after the background probe = %v, want 1", sessions["count"])
	}
}

func TestCaptureRootsReportTheLastQuickScan(t *testing.T) {
	// A quick scan answers its own request, but a page reloaded since still
	// wants to say what the last look at the volume found.
	dir := t.TempDir()
	writeCaptures(t, dir, "a.pcap", "b.pcap")
	ws := captureAPIServer(t, dir)

	doRequest(t, ws, "POST", "/api/lidar/capture/scan?probe=false", ws.handleCaptureScan)
	root := rootView(t, ws)
	result, ok := root["last_scan_result"].(map[string]any)
	if !ok {
		t.Fatalf("no last_scan_result after a quick scan: %v", root)
	}
	if result["added"] != float64(2) || result["probe"] != false {
		t.Errorf("last_scan_result = %v, want a quick scan that added 2", result)
	}
	if root["scan_in_progress"] == true {
		t.Error("a finished quick scan reads as in progress")
	}
}

func TestCaptureScanReportsAnUnreachableVolume(t *testing.T) {
	// An unmounted external drive must read as unmounted, not as empty.
	missing := filepath.Join(t.TempDir(), "not-mounted")
	ws := captureAPIServer(t, missing)
	stubCaptureProber(t)

	payload := doRequest(t, ws, "POST", "/api/lidar/capture/scan", ws.handleCaptureScan)
	result := payload["roots"].([]any)[0].(map[string]any)
	if result["state"] != capindex.StateUnreachable {
		t.Errorf("state = %v, want %q", result["state"], capindex.StateUnreachable)
	}
	if result["error"] == nil || result["error"] == "" {
		t.Error("no reason reported for an unreachable volume")
	}
}

func TestCaptureFilesAndSessionsEndpoints(t *testing.T) {
	dir := t.TempDir()
	writeCaptures(t, dir, "a.pcap", "b.pcap")
	ws := captureAPIServer(t, dir)
	stubCaptureProber(t)

	scan := doRequest(t, ws, "POST", "/api/lidar/capture/scan", ws.handleCaptureScan)
	sessions := scan["roots"].([]any)[0].(map[string]any)["sessions"].([]any)
	sessionID := sessions[0].(map[string]any)["session_id"].(string)

	files := doRequest(t, ws, "GET", "/api/lidar/capture/files", ws.handleCaptureFiles)
	if files["count"] != float64(2) {
		t.Errorf("files count = %v, want 2", files["count"])
	}

	scoped := doRequest(t, ws, "GET",
		"/api/lidar/capture/files?session_id="+sessionID, ws.handleCaptureFiles)
	if scoped["count"] != float64(2) {
		t.Errorf("session files count = %v, want 2", scoped["count"])
	}

	listed := doRequest(t, ws, "GET", "/api/lidar/capture/sessions", ws.handleCaptureSessions)
	if listed["count"] != float64(1) {
		t.Errorf("sessions count = %v, want 1", listed["count"])
	}
}

func TestCaptureSessionLabelNamesASite(t *testing.T) {
	dir := t.TempDir()
	writeCaptures(t, dir, "a.pcap", "b.pcap")
	ws := captureAPIServer(t, dir)
	stubCaptureProber(t)

	scan := doRequest(t, ws, "POST", "/api/lidar/capture/scan", ws.handleCaptureScan)
	sessions := scan["roots"].([]any)[0].(map[string]any)["sessions"].([]any)
	sessionID := sessions[0].(map[string]any)["session_id"].(string)

	body := strings.NewReader(`{"session_id":"` + sessionID +
		`","label":"broadway_columbus","sensor_id":"hesai-pandar40p"}`)
	rec := httptest.NewRecorder()
	ws.handleCaptureSessionLabel(rec,
		httptest.NewRequest("POST", "/api/lidar/capture/session/label", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("label request = %d: %s", rec.Code, rec.Body.String())
	}

	listed := doRequest(t, ws, "GET", "/api/lidar/capture/sessions", ws.handleCaptureSessions)
	got := listed["sessions"].([]any)[0].(map[string]any)
	if got["label"] != "broadway_columbus" {
		t.Errorf("label = %v, want broadway_columbus", got["label"])
	}
}

func TestCaptureSessionLabelRejectsBadRequests(t *testing.T) {
	ws := captureAPIServer(t, t.TempDir())
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"not JSON", `nonsense`, http.StatusBadRequest},
		{"no session id", `{"label":"x"}`, http.StatusBadRequest},
		{"unknown session", `{"session_id":"ses-nope","label":"x"}`, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ws.handleCaptureSessionLabel(rec, httptest.NewRequest("POST",
				"/api/lidar/capture/session/label", strings.NewReader(tc.body)))
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestCaptureEndpointsWithoutADatabase(t *testing.T) {
	ws := &Server{captureRoots: []string{t.TempDir()}}
	for name, handler := range map[string]http.HandlerFunc{
		"roots":    ws.handleCaptureRoots,
		"scan":     ws.handleCaptureScan,
		"sessions": ws.handleCaptureSessions,
		"files":    ws.handleCaptureFiles,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler(rec, httptest.NewRequest("GET", "/api/lidar/capture/"+name, nil))
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
			}
		})
	}
}
