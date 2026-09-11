package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/capindex"
)

// caseServer builds a server whose safe directory holds the named captures,
// with packet extents stubbed to a rolling series.
func caseServer(t *testing.T, gap time.Duration, names ...string) *Server {
	t.Helper()
	ws := sequenceServer(t, names...)
	base := time.Date(2026, 9, 2, 13, 45, 38, 0, time.UTC)
	index := map[string]int{}
	for i, n := range names {
		index[n] = i
	}
	original := probeExtent
	t.Cleanup(func() { probeExtent = original })
	probeExtent = func(path string, _ int) (capindex.Extent, error) {
		i := index[pathTail(path)]
		start := base.Add(time.Duration(i) * (5*time.Minute + gap))
		return capindex.Extent{
			PacketCount:   540000,
			FirstPacketNs: start.UnixNano(),
			LastPacketNs:  start.Add(5 * time.Minute).UnixNano(),
			UDPPort:       2369,
		}, nil
	}
	return ws
}

func TestValidateCaseFilesAcceptsAnAbuttingPair(t *testing.T) {
	// The shape the field produced: the last static stretch of
	// broadway_columbus spans files 7 and 8.
	ws := caseServer(t, 148*time.Millisecond, "file7.pcap", "file8.pcap")

	seq, err := ws.validateCaseFiles([]string{"file7.pcap", "file8.pcap"})
	if err != nil {
		t.Fatalf("validateCaseFiles: %v", err)
	}
	if len(seq.Segments) != 2 {
		t.Errorf("sequence holds %d captures, want 2", len(seq.Segments))
	}
	if !seq.Continuous() {
		t.Error("an abutting pair is not continuous")
	}
}

func TestValidateCaseFilesRejectsAGap(t *testing.T) {
	ws := caseServer(t, 90*time.Second, "file7.pcap", "file8.pcap")

	_, err := ws.validateCaseFiles([]string{"file7.pcap", "file8.pcap"})
	if err == nil {
		t.Fatal("a case with a ninety-second gap was accepted")
	}
	if !strings.Contains(err.Error(), "continuous") {
		t.Errorf("error %q does not say the captures are not continuous", err)
	}
}

func TestValidateCaseFilesRejectsAnUnknownCapture(t *testing.T) {
	ws := caseServer(t, 0, "file7.pcap")
	if _, err := ws.validateCaseFiles([]string{"file7.pcap", "absent.pcap"}); err == nil {
		t.Fatal("a case naming an absent capture was accepted")
	}
}

func TestValidateCaseFilesPrefersTheIndexOverProbing(t *testing.T) {
	// Reading a 700 MB capture to learn what the index already knows would make
	// authoring a case from the Captures page a minute-long wait per file.
	ws, _ := scannedSession(t, "a.pcap", "b.pcap")
	probed := 0
	original := probeExtent
	t.Cleanup(func() { probeExtent = original })
	probeExtent = func(string, int) (capindex.Extent, error) {
		probed++
		return capindex.Extent{}, nil
	}

	extent, ok := ws.indexedExtent("a.pcap")
	if !ok {
		t.Fatal("an indexed capture was not found in the index")
	}
	if extent.PacketCount == 0 {
		t.Error("the indexed extent carries no packet count")
	}
	if probed != 0 {
		t.Errorf("read %d captures to learn what the index already held", probed)
	}
}

func TestCreateCaseWithSeveralCaptures(t *testing.T) {
	ws := caseServer(t, 148*time.Millisecond, "file7.pcap", "file8.pcap")
	ws.db = mustCaseDB(t)

	body := `{"sensor_id":"hesai-pandar40p","pcap_files":["file7.pcap","file8.pcap"],
	          "pcap_start_secs":82,"pcap_duration_secs":449.4,
	          "description":"broadway_columbus last static stretch"}`
	rec := httptest.NewRecorder()
	ws.handleCreateScene(rec, httptest.NewRequest("POST", "/api/lidar/scenes", strings.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	payload := decodeCaseBody(t, rec)
	files, ok := payload["files"].([]any)
	if !ok || len(files) != 2 {
		t.Fatalf("response files = %v, want two captures", payload["files"])
	}
	// pcap_file stays the read-only projection of the first capture, so a
	// client that predates the list still replays the right thing.
	if payload["pcap_file"] != "file7.pcap" {
		t.Errorf("pcap_file = %v, want file7.pcap", payload["pcap_file"])
	}
}

func TestCreateCaseRejectsCapturesThatDoNotAbut(t *testing.T) {
	// Authoring time is the last point at which the operator can still fix it.
	ws := caseServer(t, 90*time.Second, "file7.pcap", "file8.pcap")
	ws.db = mustCaseDB(t)

	body := `{"sensor_id":"hesai-pandar40p","pcap_files":["file7.pcap","file8.pcap"]}`
	rec := httptest.NewRecorder()
	ws.handleCreateScene(rec, httptest.NewRequest("POST", "/api/lidar/scenes", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "continuous") {
		t.Errorf("body %q does not explain the captures are not continuous", rec.Body.String())
	}
}

func TestCreateCaseStillAcceptsASingleCapture(t *testing.T) {
	ws := caseServer(t, 0, "file7.pcap")
	ws.db = mustCaseDB(t)

	body := `{"sensor_id":"hesai-pandar40p","pcap_file":"file7.pcap"}`
	rec := httptest.NewRecorder()
	ws.handleCreateScene(rec, httptest.NewRequest("POST", "/api/lidar/scenes", strings.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	payload := decodeCaseBody(t, rec)
	files := payload["files"].([]any)
	if len(files) != 1 {
		t.Errorf("files = %v, want the single capture", files)
	}
}

func TestGetCaseReportsItsCaptures(t *testing.T) {
	// Reading a case without its file list would show only the first capture
	// and quietly misrepresent a multi-file case as single-file.
	ws := caseServer(t, 148*time.Millisecond, "file7.pcap", "file8.pcap")
	ws.db = mustCaseDB(t)

	body := `{"sensor_id":"hesai-pandar40p","pcap_files":["file7.pcap","file8.pcap"]}`
	rec := httptest.NewRecorder()
	ws.handleCreateScene(rec, httptest.NewRequest("POST", "/api/lidar/scenes", strings.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	caseID := decodeCaseBody(t, rec)["replay_case_id"].(string)

	getRec := httptest.NewRecorder()
	ws.handleGetScene(getRec, httptest.NewRequest("GET", "/api/lidar/scenes/"+caseID, nil), caseID)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get = %d: %s", getRec.Code, getRec.Body.String())
	}
	files := decodeCaseBody(t, getRec)["files"].([]any)
	if len(files) != 2 {
		t.Errorf("get returned %d captures, want 2", len(files))
	}
}

func TestCreateCaseWithNoCaptures(t *testing.T) {
	ws := caseServer(t, 0, "file7.pcap")
	ws.db = mustCaseDB(t)
	rec := httptest.NewRecorder()
	ws.handleCreateScene(rec,
		httptest.NewRequest("POST", "/api/lidar/scenes", strings.NewReader(`{"sensor_id":"s"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// mustCaseDB returns a migrated database for the replay-case handlers.
func mustCaseDB(t *testing.T) *db.DB {
	t.Helper()
	testDB, cleanup := db.NewTestDB(t)
	t.Cleanup(cleanup)
	return testDB
}

// decodeCaseBody reads a handler's JSON response.
func decodeCaseBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding response: %v (%s)", err, rec.Body.String())
	}
	return payload
}
