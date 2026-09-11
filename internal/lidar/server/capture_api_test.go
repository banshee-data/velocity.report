package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
