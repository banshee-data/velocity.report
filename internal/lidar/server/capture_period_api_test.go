package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/pcapsplit"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// periodFixture is a two-capture session (five minutes each, from the stub
// prober's base) with a static period spanning the join and a motion period.
func periodFixture(t *testing.T) (*Server, string, string) {
	t.Helper()
	ws, sessionID := scannedSession(t, "a.pcap", "b.pcap")
	store, err := ws.captureStore()
	if err != nil {
		t.Fatal(err)
	}
	base := time.Unix(1_788_000_000, 0)
	if err := store.ReplaceSessionPeriods(sessionID, []sqlite.MotionPeriod{
		{Type: sqlite.PeriodStatic, Label: "static-0", StartNs: base.Add(time.Minute).UnixNano(),
			EndNs: base.Add(7 * time.Minute).UnixNano(), DurationNs: int64(6 * time.Minute)},
		{Type: sqlite.PeriodMotion, Label: "motion-0", StartNs: base.Add(7 * time.Minute).UnixNano(),
			EndNs: base.Add(10 * time.Minute).UnixNano(), DurationNs: int64(3 * time.Minute)},
	}); err != nil {
		t.Fatal(err)
	}
	periods, err := store.ListSessionPeriods(sessionID)
	if err != nil || len(periods) != 2 {
		t.Fatalf("periods %v %v", periods, err)
	}
	return ws, periods[0].PeriodID, periods[1].PeriodID
}

func periodRequest(ws *Server, method, target, body string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	handler(rec, req)
	return rec
}

func TestPeriodBackgroundIsBuiltOnceAndServedInTheSensorFrame(t *testing.T) {
	ws, staticID, motionID := periodFixture(t)

	var calls atomic.Int32
	var gotPaths []string
	var gotStart, gotWindow float64
	prev := periodBackgroundFunc
	t.Cleanup(func() { periodBackgroundFunc = prev })
	periodBackgroundFunc = func(_ context.Context, paths []string, _ []capseq.Segment, _ int,
		_ *cfgpkg.TuningConfig, start, window float64) (*pcapsplit.Background, error) {
		calls.Add(1)
		gotPaths, gotStart, gotWindow = paths, start, window
		return &pcapsplit.Background{
			Frames: 900, SettledThreshold: 50,
			FirstTime: time.Unix(1_788_000_060, 0), LastTime: time.Unix(1_788_000_150, 0),
			Points: []pcapsplit.BackgroundPoint{{X: 1.234, Y: -5.678, Z: 0.5}, {X: 10, Y: 0, Z: -2.31}},
		}, nil
	}

	for range 2 {
		payload := doRequest(t, ws, "GET", "/api/lidar/capture/periods/background?period_id="+staticID,
			ws.handleCapturePeriodBackground)
		if payload["count"] != float64(2) || payload["frame"] != "sensor" || payload["frames"] != float64(900) {
			t.Fatalf("payload %v", payload)
		}
		xyz := payload["xyz"].([]any)
		if len(xyz) != 6 || xyz[0] != 1.23 || xyz[1] != -5.68 || xyz[5] != -2.31 {
			t.Fatalf("xyz %v", xyz)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("built %d times, want once", calls.Load())
	}
	if len(gotPaths) != 2 || gotStart != 60 || gotWindow != 90 {
		t.Fatalf("built from %v at %v s for %v s", gotPaths, gotStart, gotWindow)
	}

	for target, want := range map[string]int{
		"/api/lidar/capture/periods/background":                       http.StatusBadRequest,
		"/api/lidar/capture/periods/background?period_id=" + motionID: http.StatusBadRequest,
		"/api/lidar/capture/periods/background?period_id=nope":        http.StatusNotFound,
	} {
		if rec := periodRequest(ws, "GET", target, "", ws.handleCapturePeriodBackground); rec.Code != want {
			t.Errorf("%s = %d, want %d", target, rec.Code, want)
		}
	}
}

func TestPeriodClipIsCreatedOnceAndShowsInThePeriods(t *testing.T) {
	ws, staticID, _ := periodFixture(t)

	rec := periodRequest(ws, "POST", "/api/lidar/capture/periods/clip",
		`{"period_id":"`+staticID+`"}`, ws.handleCapturePeriodClip)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var clip sqlite.Clip
	if err := json.Unmarshal(rec.Body.Bytes(), &clip); err != nil {
		t.Fatal(err)
	}
	if clip.FileCount != 2 || *clip.PCAPStartSecs != 60 || *clip.PCAPDurationSecs != 360 ||
		clip.SourcePeriodID != staticID || clip.Description != "static-0" {
		t.Fatalf("clip %+v", clip)
	}

	again := periodRequest(ws, "POST", "/api/lidar/capture/periods/clip",
		`{"period_id":"`+staticID+`"}`, ws.handleCapturePeriodClip)
	var same sqlite.Clip
	_ = json.Unmarshal(again.Body.Bytes(), &same)
	if again.Code != http.StatusOK || same.ClipID != clip.ClipID {
		t.Fatalf("second request: %d, clip %s want %s", again.Code, same.ClipID, clip.ClipID)
	}

	sessionID := clip.SessionID
	periods := doRequest(t, ws, "GET", "/api/lidar/capture/periods?session_id="+sessionID, ws.handleCapturePeriods)
	entry := periods["clips"].(map[string]any)[staticID].(map[string]any)
	if entry["clip_id"] != clip.ClipID || entry["located"] != false {
		t.Fatalf("before locating: %v", entry)
	}
	if _, err := sqlite.NewClipStore(ws.db).SetClipLocation(clip.ClipID, 37.7604, -122.4664, ""); err != nil {
		t.Fatal(err)
	}
	periods = doRequest(t, ws, "GET", "/api/lidar/capture/periods?session_id="+sessionID, ws.handleCapturePeriods)
	entry = periods["clips"].(map[string]any)[staticID].(map[string]any)
	if entry["located"] != true || entry["origin_lat"] != 37.7604 {
		t.Fatalf("after locating: %v", entry)
	}

	for body, want := range map[string]int{
		`{}`:                   http.StatusBadRequest,
		`not json`:             http.StatusBadRequest,
		`{"period_id":"nope"}`: http.StatusNotFound,
	} {
		if rec := periodRequest(ws, "POST", "/api/lidar/capture/periods/clip", body, ws.handleCapturePeriodClip); rec.Code != want {
			t.Errorf("%s = %d, want %d", body, rec.Code, want)
		}
	}
}

func TestBackgroundCacheSharesABuildAndForgetsFailures(t *testing.T) {
	var c backgroundCache
	var builds atomic.Int32
	release := make(chan struct{})
	build := func(context.Context) (*pcapsplit.Background, error) {
		builds.Add(1)
		<-release
		return &pcapsplit.Background{Frames: 1}, nil
	}
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if bg, err := c.get(context.Background(), "p", build); err != nil || bg.Frames != 1 {
				t.Errorf("get: %v %v", bg, err)
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if builds.Load() != 1 {
		t.Fatalf("%d builds for one key, want 1", builds.Load())
	}

	failing := func(context.Context) (*pcapsplit.Background, error) { return nil, errors.New("disk gone") }
	if _, err := c.get(context.Background(), "q", failing); err == nil {
		t.Fatal("a failed build succeeded")
	}
	if _, err := c.get(context.Background(), "q", func(context.Context) (*pcapsplit.Background, error) {
		return &pcapsplit.Background{Frames: 2}, nil
	}); err != nil {
		t.Fatal("a failure was cached")
	}

	for i := range backgroundCacheSize + 1 {
		key := string(rune('a' + i))
		_, _ = c.get(context.Background(), key, func(context.Context) (*pcapsplit.Background, error) {
			return &pcapsplit.Background{}, nil
		})
	}
	if len(c.done) > backgroundCacheSize {
		t.Fatalf("cache holds %d, cap %d", len(c.done), backgroundCacheSize)
	}

	ctx, cancel := context.WithCancel(context.Background())
	block := make(chan struct{})
	go func() {
		_, _ = c.get(context.Background(), "slow", func(context.Context) (*pcapsplit.Background, error) {
			<-block
			return &pcapsplit.Background{}, nil
		})
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	if _, err := c.get(ctx, "slow", build); !errors.Is(err, context.Canceled) {
		t.Fatalf("a waiter ignored its context: %v", err)
	}
	close(block)
}

func TestClipBackgroundUsesTheClipWindowAndLocatedClipsList(t *testing.T) {
	ws, staticID, _ := periodFixture(t)
	store, _ := ws.captureStore()
	roots, err := store.ListRoots()
	if err != nil || len(roots) == 0 {
		t.Fatalf("roots %v %v", roots, err)
	}
	// The replay check compares canonical paths; macOS's temp directory is a symlink.
	safe, err := filepath.EvalSymlinks(roots[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	ws.pcapSafeDir = safe

	created := periodRequest(ws, "POST", "/api/lidar/capture/periods/clip",
		`{"period_id":"`+staticID+`"}`, ws.handleCapturePeriodClip)
	var clip sqlite.Clip
	if err := json.Unmarshal(created.Body.Bytes(), &clip); err != nil {
		t.Fatal(err)
	}

	var gotStart, gotWindow float64
	var gotPaths int
	prev := periodBackgroundFunc
	t.Cleanup(func() { periodBackgroundFunc = prev })
	periodBackgroundFunc = func(_ context.Context, paths []string, _ []capseq.Segment, _ int,
		_ *cfgpkg.TuningConfig, start, window float64) (*pcapsplit.Background, error) {
		gotPaths, gotStart, gotWindow = len(paths), start, window
		return &pcapsplit.Background{Frames: 3, Points: []pcapsplit.BackgroundPoint{{X: 1, Y: 2, Z: 3}}}, nil
	}
	rec := periodRequest(ws, "GET", "/api/lidar/clips/"+clip.ClipID+"/background", "", ws.handleClipByID)
	if rec.Code != http.StatusOK {
		t.Fatalf("clip background: %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["clip_id"] != clip.ClipID || body["period_id"] != staticID || body["count"] != float64(1) {
		t.Fatalf("body %v", body)
	}
	if gotPaths != 2 || gotStart != 60 || gotWindow != 90 {
		t.Fatalf("built from %d paths at %v s for %v s", gotPaths, gotStart, gotWindow)
	}
	if rec := periodRequest(ws, "POST", "/api/lidar/clips/"+clip.ClipID+"/background", "", ws.handleClipByID); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST background: %d", rec.Code)
	}
	if rec := periodRequest(ws, "GET", "/api/lidar/clips/nope/background", "", ws.handleClipByID); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown clip: %d", rec.Code)
	}

	list := func() []sqlite.Clip {
		rec := periodRequest(ws, "GET", "/api/lidar/clips?located=1", "", ws.handleClips)
		var payload struct {
			Clips []sqlite.Clip `json:"clips"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &payload)
		return payload.Clips
	}
	if got := list(); len(got) != 0 {
		t.Fatalf("an unlocated clip was listed as located: %+v", got)
	}
	if _, err := sqlite.NewClipStore(ws.db).SetClipLocation(clip.ClipID, 37.7531, -122.4561, ""); err != nil {
		t.Fatal(err)
	}
	got := list()
	if len(got) != 1 || got[0].Location == nil || got[0].Location.Lat != 37.7531 {
		t.Fatalf("located clips %+v", got)
	}
}
