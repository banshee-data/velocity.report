package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func getRunFrame(ws *Server, runID, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/runs/"+runID+"/vrlog-frame"+query, nil)
	rec := httptest.NewRecorder()
	ws.handleRunTrackAPI(rec, req)
	return rec
}

// An instant maps to the index of the first recorded frame at or after it,
// which is the frame a velocityreport:// link opens the visualiser on.
func TestRunFrameMapsAnInstantToTheRecordingsFrameIndex(t *testing.T) {
	dir := t.TempDir()
	vrlog := writeSceneTestVRLOG(t, dir, 10)
	ws := runSceneServer(t, "run-1", vrlog, dir)

	const start = int64(1_764_973_025_000_000_000)
	cases := []struct {
		atNs int64
		want uint64
	}{
		{start, 0},
		{start + 300_000_000, 3},
		{start + 250_000_000, 3}, // between frames: the next one
		{start - 1_000_000_000, 0},
		{start + 60_000_000_000, 9}, // past the end: the last
	}
	for _, c := range cases {
		rec := getRunFrame(ws, "run-1", fmt.Sprintf("?at_ns=%d", c.atNs))
		if rec.Code != http.StatusOK {
			t.Fatalf("at %d: status %d: %s", c.atNs, rec.Code, rec.Body.String())
		}
		var body struct {
			Frame       uint64 `json:"frame"`
			TotalFrames uint64 `json:"total_frames"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Frame != c.want || body.TotalFrames != 10 {
			t.Errorf("at %d: frame %d of %d, want %d of 10", c.atNs, body.Frame, body.TotalFrames, c.want)
		}
	}
}

func TestRunFrameRefusals(t *testing.T) {
	dir := t.TempDir()
	vrlog := writeSceneTestVRLOG(t, dir, 3)
	ws := runSceneServer(t, "run-1", vrlog, dir)
	if rec := getRunFrame(ws, "run-1", "?at_ns=soon"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad at_ns: status %d, want 400", rec.Code)
	}
	if rec := getRunFrame(ws, "run-1", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("missing at_ns: status %d, want 400", rec.Code)
	}
	if rec := getRunFrame(ws, "nope", "?at_ns=1"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown run: status %d, want 404", rec.Code)
	}
	bare := runSceneServer(t, "run-2", "", dir)
	if rec := getRunFrame(bare, "run-2", "?at_ns=1"); rec.Code != http.StatusNotFound {
		t.Errorf("run without a recording: status %d, want 404", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/runs/run-1/vrlog-frame?at_ns=1", nil)
	rec := httptest.NewRecorder()
	ws.handleRunTrackAPI(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d, want 405", rec.Code)
	}
}
