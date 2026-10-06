package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// scannedSession sets a server up with an indexed, sessioned volume and returns
// the session id.
func scannedSession(t *testing.T, names ...string) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	writeCaptures(t, dir, names...)
	ws := captureAPIServer(t, dir)
	stubCaptureProber(t)

	payload := doRequest(t, ws, "POST", "/api/lidar/capture/scan", ws.handleCaptureScan)
	root := payload["roots"].([]any)[0].(map[string]any)
	if root["state"] != "ok" {
		t.Fatalf("scan state = %v (%v)", root["state"], root["error"])
	}
	sessions := root["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("derived %d sessions, want 1", len(sessions))
	}
	return ws, sessions[0].(map[string]any)["session_id"].(string)
}

func TestMotionPassEndpointQueuesWork(t *testing.T) {
	ws, sessionID := scannedSession(t, "a.pcap", "b.pcap", "c.pcap")

	payload := doRequest(t, ws, "POST",
		"/api/lidar/capture/motion-pass?session_id="+sessionID, ws.handleCaptureMotionPass)
	job := payload["job"].(map[string]any)
	if job["state"] != sqlite.JobQueued {
		t.Errorf("state = %v, want %q", job["state"], sqlite.JobQueued)
	}
	if job["kind"] != sqlite.JobKindMotionPass {
		t.Errorf("kind = %v, want %q", job["kind"], sqlite.JobKindMotionPass)
	}

	// Asking twice must not queue a second pass over the same session: it
	// reads every byte of every file and would halve its own throughput.
	again := doRequest(t, ws, "POST",
		"/api/lidar/capture/motion-pass?session_id="+sessionID, ws.handleCaptureMotionPass)
	if again["job"].(map[string]any)["job_id"] != job["job_id"] {
		t.Error("a second motion pass was queued for the same session")
	}

	listed := doRequest(t, ws, "GET", "/api/lidar/capture/jobs", ws.handleCaptureJobs)
	if listed["count"] != float64(1) {
		t.Errorf("jobs count = %v, want 1", listed["count"])
	}
}

func TestMotionPassEndpointRejectsBadRequests(t *testing.T) {
	ws, _ := scannedSession(t, "a.pcap")
	for _, tc := range []struct {
		name   string
		target string
		want   int
	}{
		{"no session", "/api/lidar/capture/motion-pass", http.StatusBadRequest},
		{"unknown session", "/api/lidar/capture/motion-pass?session_id=ses-nope", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ws.handleCaptureMotionPass(rec, httptest.NewRequest("POST", tc.target, nil))
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestCaptureJobCancelEndpoint(t *testing.T) {
	ws, sessionID := scannedSession(t, "a.pcap")
	queued := doRequest(t, ws, "POST",
		"/api/lidar/capture/motion-pass?session_id="+sessionID, ws.handleCaptureMotionPass)
	jobID := queued["job"].(map[string]any)["job_id"].(string)

	payload := doRequest(t, ws, "POST",
		"/api/lidar/capture/jobs/cancel?job_id="+jobID, ws.handleCaptureJobCancel)
	if payload["state"] != sqlite.JobCancelled {
		t.Errorf("state = %v, want %q", payload["state"], sqlite.JobCancelled)
	}

	// Cancelling again, or cancelling something absent, is a 404 rather than a
	// silent success.
	for _, target := range []string{
		"/api/lidar/capture/jobs/cancel?job_id=" + jobID,
		"/api/lidar/capture/jobs/cancel?job_id=job-nope",
	} {
		rec := httptest.NewRecorder()
		ws.handleCaptureJobCancel(rec, httptest.NewRequest("POST", target, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", target, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	ws.handleCaptureJobCancel(rec, httptest.NewRequest("POST", "/api/lidar/capture/jobs/cancel", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("cancel with no job_id = %d, want 400", rec.Code)
	}
}

func TestCapturePeriodsEndpointSummarisesTheTimeline(t *testing.T) {
	ws, sessionID := scannedSession(t, "a.pcap", "b.pcap")
	store, err := ws.captureStore()
	if err != nil {
		t.Fatalf("captureStore: %v", err)
	}

	base := time.Date(2026, 9, 2, 13, 20, 0, 0, time.UTC)
	if err := store.ReplaceSessionPeriods(sessionID, []sqlite.MotionPeriod{
		{Type: sqlite.PeriodStatic, Label: "static-0", StartNs: base.UnixNano(),
			EndNs: base.Add(2 * time.Minute).UnixNano(), DurationNs: int64(2 * time.Minute)},
		{Type: sqlite.PeriodMotion, Label: "motion-0", StartNs: base.Add(2 * time.Minute).UnixNano(),
			EndNs: base.Add(3 * time.Minute).UnixNano(), DurationNs: int64(time.Minute)},
	}); err != nil {
		t.Fatalf("ReplaceSessionPeriods: %v", err)
	}

	payload := doRequest(t, ws, "GET",
		"/api/lidar/capture/periods?session_id="+sessionID, ws.handleCapturePeriods)
	if payload["count"] != float64(2) {
		t.Errorf("count = %v, want 2", payload["count"])
	}
	if payload["static_seconds"] != float64(120) {
		t.Errorf("static_seconds = %v, want 120", payload["static_seconds"])
	}
	if payload["motion_seconds"] != float64(60) {
		t.Errorf("motion_seconds = %v, want 60", payload["motion_seconds"])
	}

	rec := httptest.NewRecorder()
	ws.handleCapturePeriods(rec, httptest.NewRequest("GET", "/api/lidar/capture/periods", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("periods with no session_id = %d, want 400", rec.Code)
	}
}

func TestRunMotionPassStoresTheTimeline(t *testing.T) {
	ws, sessionID := scannedSession(t, "a.pcap", "b.pcap")
	store, _ := ws.captureStore()

	original := sessionMotionPassFunc
	t.Cleanup(func() { sessionMotionPassFunc = original })
	var sawPaths []string
	sessionMotionPassFunc = func(_ context.Context, paths []string, _ int,
		_ *cfgpkg.TuningConfig, report func(int64, int64, string)) ([]sqlite.MotionPeriod, error) {
		sawPaths = paths
		report(1, 2, "halfway")
		base := time.Date(2026, 9, 2, 13, 20, 0, 0, time.UTC)
		return []sqlite.MotionPeriod{{
			Type: sqlite.PeriodStatic, Label: "static-0",
			StartNs: base.UnixNano(), EndNs: base.Add(time.Minute).UnixNano(),
			DurationNs: int64(time.Minute),
		}}, nil
	}

	job := capjobs.Job{JobID: "job-1", Kind: capjobs.KindMotionPass, SessionID: sessionID}
	var progress []capjobs.Progress
	if err := ws.runMotionPass(context.Background(), job, func(p capjobs.Progress) {
		progress = append(progress, p)
	}); err != nil {
		t.Fatalf("runMotionPass: %v", err)
	}

	// The pass must receive the session's captures joined, not one at a time.
	if len(sawPaths) != 2 {
		t.Errorf("the pass saw %d captures, want the session's 2", len(sawPaths))
	}
	periods, err := store.ListSessionPeriods(sessionID)
	if err != nil {
		t.Fatalf("ListSessionPeriods: %v", err)
	}
	if len(periods) != 1 || periods[0].Label != "static-0" {
		t.Errorf("stored periods = %+v, want the pass's one", periods)
	}
	if len(progress) < 2 {
		t.Errorf("reported %d progress updates, want the start and the pass's own", len(progress))
	}
}

func TestRunMotionPassRequiresASession(t *testing.T) {
	ws, _ := scannedSession(t, "a.pcap")
	err := ws.runMotionPass(context.Background(),
		capjobs.Job{JobID: "job-1", Kind: capjobs.KindMotionPass}, func(capjobs.Progress) {})
	if err == nil {
		t.Fatal("a motion pass with no session succeeded")
	}
}

func TestRunCaptureJobRejectsAnUnknownKind(t *testing.T) {
	ws, _ := scannedSession(t, "a.pcap")
	err := ws.runCaptureJob(context.Background(),
		capjobs.Job{JobID: "job-1", Kind: "nonsense"}, func(capjobs.Progress) {})
	if err == nil || !strings.Contains(err.Error(), "nonsense") {
		t.Fatalf("error = %v, want it to name the unknown kind", err)
	}
}

func TestSafeCaptureJoinRefusesEscapes(t *testing.T) {
	// The capture index stores paths relative to a root precisely so this
	// check has something to check against.
	root := t.TempDir()
	if _, err := safeCaptureJoin(root, "sub/a.pcap"); err != nil {
		t.Errorf("a path inside the root was refused: %v", err)
	}
	for _, escape := range []string{"../outside.pcap", "sub/../../outside.pcap"} {
		if _, err := safeCaptureJoin(root, escape); err == nil {
			t.Errorf("%q escaped its root", escape)
		}
	}
}

func TestCaptureJobStoreMapsAnEmptyQueue(t *testing.T) {
	// The runner has its own name for "nothing to do" so it does not have to
	// know about SQL.
	ws, _ := scannedSession(t, "a.pcap")
	store, _ := ws.captureStore()
	adapter := captureJobStore{store: store}

	if _, err := adapter.ClaimNextJob(); !errors.Is(err, capjobs.ErrNoWork) {
		t.Fatalf("claiming an empty queue = %v, want capjobs.ErrNoWork", err)
	}
}

func TestStartCaptureJobsWithoutADatabaseIsHarmless(t *testing.T) {
	ws := &Server{captureRoots: []string{t.TempDir()}}
	if err := ws.StartCaptureJobs(context.Background()); err != nil {
		t.Fatalf("StartCaptureJobs with no database: %v", err)
	}
	if ws.captureRunner != nil {
		t.Error("a runner was started with nothing to queue work in")
	}
}

func TestSessionUDPPortPrefersWhatTheIndexObserved(t *testing.T) {
	// The port this process listens on and the port a capture was recorded on
	// differ whenever the captures came from elsewhere. The probe already
	// established which is right, so the pass must not second-guess it.
	port := 2369
	files := []sqlite.CaptureFile{
		{RelPath: "a.pcap"},
		{RelPath: "b.pcap", UDPPort: &port},
	}
	if got := sessionUDPPort(files, 12369); got != 2369 {
		t.Errorf("sessionUDPPort = %d, want the observed 2369", got)
	}
}

func TestSessionUDPPortFallsBackToTheConfiguredPort(t *testing.T) {
	zero := 0
	for _, files := range [][]sqlite.CaptureFile{
		nil,
		{{RelPath: "a.pcap"}},
		{{RelPath: "a.pcap", UDPPort: &zero}},
	} {
		if got := sessionUDPPort(files, 2369); got != 2369 {
			t.Errorf("sessionUDPPort(%v) = %d, want the configured 2369", files, got)
		}
	}
}
