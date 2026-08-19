package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// stubReplayFrameBuilder records SetBlockOnFrameChannel calls made by the
// replay goroutine.
//
// The mutex is load-bearing: the final SetBlockOnFrameChannel(false) runs from
// a deferred call inside that goroutine, and waitForPCAPDone does not wait when
// pcapDone has already been cleared — so the test can read these fields while
// the goroutine's defers are still unwinding. Without synchronisation the race
// detector flags it under cross-package load (-race -p 2).
type stubReplayFrameBuilder struct {
	mu      sync.Mutex
	dropped uint64
	calls   []bool
	drains  int
	flushes int
}

func (s *stubReplayFrameBuilder) SetBlockOnFrameChannel(block bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, block)
}

func (s *stubReplayFrameBuilder) DroppedFrames() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dropped
}

func (s *stubReplayFrameBuilder) WaitForCallbacks() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drains++
}

func (s *stubReplayFrameBuilder) FlushPendingFrames() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushes++
}

// blockCalls returns a copy of the recorded calls for assertions.
func (s *stubReplayFrameBuilder) blockCalls() []bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]bool(nil), s.calls...)
}

func restoreDatasourceHandlerSeams() func() {
	origCount := countPCAPPackets
	origRead := readPCAPFile
	origReadRealtime := readPCAPFileRealtime
	origNewForegroundForwarder := newForegroundForwarder
	origAbsPath := absPath
	origStatPath := statPath
	origGetReplayFrameBuilder := getReplayFrameBuilder
	origPrepareSettledPCAPReplay := prepareSettledPCAPReplay
	return func() {
		countPCAPPackets = origCount
		readPCAPFile = origRead
		readPCAPFileRealtime = origReadRealtime
		newForegroundForwarder = origNewForegroundForwarder
		absPath = origAbsPath
		statPath = origStatPath
		getReplayFrameBuilder = origGetReplayFrameBuilder
		prepareSettledPCAPReplay = origPrepareSettledPCAPReplay
	}
}

func stopLiveListenerIfRunning(ws *Server) {
	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()
	ws.stopLiveListenerLocked()
}

func TestFailReplayAnalysisRun_FailRunError(t *testing.T) {
	dbWrapped, cleanupDB := setupTestDBWrapped(t)
	defer cleanupDB()

	ws := NewServer(Config{
		Address:  ":0",
		Stats:    NewPacketStats(),
		SensorID: "fail-run-error",
		DB:       dbWrapped,
	})

	runID, err := ws.startReplayAnalysisRun("/tmp/fail-run-error.pcap", ReplayConfig{
		AnalysisMode:   true,
		SensorID:       "fail-run-error",
		PreferredRunID: "fail-run-error-id",
	})
	if err != nil {
		t.Fatalf("startReplayAnalysisRun() error: %v", err)
	}
	if runID == "" {
		t.Fatal("expected run ID")
	}
	if err := dbWrapped.DB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	ws.failReplayAnalysisRun(runID, "boom")
}

func TestGetReplayFrameBuilder_DefaultRegistered(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)

	sensorID := "replay-frame-builder-default"
	fb := l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{SensorID: sensorID})
	defer fb.Close()

	if got := getReplayFrameBuilder(sensorID); got == nil {
		t.Fatal("expected registered replay frame builder")
	}
}

func TestPrepareSettledPCAPReplay(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)
	frameBuilder := &stubReplayFrameBuilder{}
	getReplayFrameBuilder = func(string) replayFrameBuilder { return frameBuilder }

	t.Run("requires a registered manager", func(t *testing.T) {
		ws := NewServer(Config{SensorID: "settle-prepare-missing-manager"})
		err := prepareSettledPCAPReplay(ws, ws.sensorID, "/captures/missing.pcap")
		if err == nil || !strings.Contains(err.Error(), "no background manager") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("requires settling to complete", func(t *testing.T) {
		sensorID := "settle-prepare-unsettled"
		mgr := l3grid.NewBackgroundManagerDI(sensorID, 2, 4, l3grid.BackgroundParams{}, nil)
		l3grid.RegisterBackgroundManager(sensorID, mgr)
		ws := NewServer(Config{SensorID: sensorID})
		err := prepareSettledPCAPReplay(ws, sensorID, "/captures/short.pcap")
		if err == nil || !strings.Contains(err.Error(), "before the background grid settled") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reports missing persisted snapshot", func(t *testing.T) {
		sensorID := "settle-prepare-missing-snapshot"
		dbWrapped, cleanupDB := setupTestDBWrapped(t)
		defer cleanupDB()
		mgr := l3grid.NewBackgroundManagerDI(sensorID, 2, 4, l3grid.BackgroundParams{}, dbWrapped)
		mgr.Grid.SettlingComplete = true
		l3grid.RegisterBackgroundManager(sensorID, mgr)
		ws := NewServer(Config{SensorID: sensorID})
		err := prepareSettledPCAPReplay(ws, sensorID, "/captures/not-persisted.pcap")
		if err == nil || !strings.Contains(err.Error(), "no settled background snapshot") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reports lookup failure", func(t *testing.T) {
		sensorID := "settle-prepare-lookup-error"
		dbWrapped, cleanupDB := setupTestDBWrapped(t)
		mgr := l3grid.NewBackgroundManagerDI(sensorID, 2, 4, l3grid.BackgroundParams{}, dbWrapped)
		mgr.Grid.SettlingComplete = true
		l3grid.RegisterBackgroundManager(sensorID, mgr)
		cleanupDB()
		ws := NewServer(Config{SensorID: sensorID})
		if err := prepareSettledPCAPReplay(ws, sensorID, "/captures/db-error.pcap"); err == nil {
			t.Fatal("expected lookup error")
		}
	})

	t.Run("reports corrupt persisted snapshot", func(t *testing.T) {
		sensorID := "settle-prepare-corrupt-snapshot"
		path := "/captures/corrupt.pcap"
		dbWrapped, cleanupDB := setupTestDBWrapped(t)
		defer cleanupDB()
		mgr := l3grid.NewBackgroundManagerDI(sensorID, 2, 4, l3grid.BackgroundParams{}, dbWrapped)
		requirePersistedGrid(t, mgr, dbWrapped)
		_, err := dbWrapped.InsertRegionSnapshot(&l3grid.RegionSnapshot{
			SnapshotID:  *mgr.Grid.SnapshotID,
			SensorID:    sensorID,
			SourcePath:  path,
			RegionCount: 1,
			RegionsJSON: "not-json",
		})
		if err != nil {
			t.Fatalf("InsertRegionSnapshot() error: %v", err)
		}
		mgr.Grid.SettlingComplete = true
		l3grid.RegisterBackgroundManager(sensorID, mgr)
		ws := NewServer(Config{SensorID: sensorID})
		if err := prepareSettledPCAPReplay(ws, sensorID, path); err == nil {
			t.Fatal("expected corrupt snapshot error")
		}
	})

	t.Run("restores the exact source snapshot", func(t *testing.T) {
		sensorID := "settle-prepare-success"
		path := "/captures/complete.pcap"
		dbWrapped, cleanupDB := setupTestDBWrapped(t)
		defer cleanupDB()
		mgr := l3grid.NewBackgroundManagerDI(sensorID, 2, 4, l3grid.BackgroundParams{}, dbWrapped)
		requirePersistedGrid(t, mgr, dbWrapped)
		_, err := dbWrapped.InsertRegionSnapshot(&l3grid.RegionSnapshot{
			SnapshotID:  *mgr.Grid.SnapshotID,
			SensorID:    sensorID,
			SourcePath:  path,
			RegionCount: 1,
			RegionsJSON: `[{"id":0,"cell_list":[0],"cell_count":1}]`,
		})
		if err != nil {
			t.Fatalf("InsertRegionSnapshot() error: %v", err)
		}
		mgr.Grid.SettlingComplete = true
		l3grid.RegisterBackgroundManager(sensorID, mgr)
		ws := NewServer(Config{SensorID: sensorID})
		if err := prepareSettledPCAPReplay(ws, sensorID, path); err != nil {
			t.Fatalf("prepareSettledPCAPReplay() error: %v", err)
		}
		if !mgr.IsSettlingComplete() {
			t.Fatal("restored manager should remain settled")
		}
	})

	frameBuilder.mu.Lock()
	defer frameBuilder.mu.Unlock()
	if frameBuilder.flushes == 0 || frameBuilder.drains == 0 {
		t.Fatalf("prepare did not flush and drain callbacks: flushes=%d drains=%d", frameBuilder.flushes, frameBuilder.drains)
	}
}

func requirePersistedGrid(t *testing.T, mgr *l3grid.BackgroundManager, store l3grid.BgStore) {
	t.Helper()
	if err := mgr.Persist(store, "settle-prepare-test"); err != nil {
		t.Fatalf("Persist() error: %v", err)
	}
	if mgr.Grid.SnapshotID == nil {
		t.Fatal("Persist() did not assign a snapshot ID")
	}
}

func TestResolvePCAPPath_StatReportsFileDisappeared(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)
	tmpDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(tmpDir, "vanished.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	statPath = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	ws := NewServer(Config{SensorID: "vanished-pcap", PCAPSafeDir: tmpDir})
	_, err := ws.resolvePCAPPath("vanished.pcap")
	var switchErr *switchError
	if !errors.As(err, &switchErr) || switchErr.status != 404 {
		t.Fatalf("resolvePCAPPath() error = %v, want 404 switchError", err)
	}
}

func TestStartPCAPLocked_AnalysisMode_CountErrorProgressAndDroppedFrames(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)

	sensorID := "pcap-analysis-seams"
	tmpDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(tmpDir, "analysis-seams.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	var progressCurrent uint64
	var progressTotal uint64
	frameBuilder := &stubReplayFrameBuilder{dropped: 2}

	countPCAPPackets = func(string, int) (network.PCAPCountResult, error) {
		return network.PCAPCountResult{}, errors.New("count failed")
	}
	readPCAPFile = func(
		_ context.Context,
		_ string,
		_ int,
		_ network.Parser,
		_ network.FrameBuilder,
		_ network.PacketStatsInterface,
		_ *network.PacketForwarder,
		_ float64,
		_ float64,
		_ uint64,
		_ uint64,
		onProgress func(current, total uint64),
	) error {
		if onProgress != nil {
			onProgress(3, 7)
		}
		return nil
	}
	getReplayFrameBuilder = func(string) replayFrameBuilder {
		return frameBuilder
	}

	ws := NewServer(Config{
		Address:     ":0",
		Stats:       NewPacketStats(),
		SensorID:    sensorID,
		PCAPSafeDir: tmpDir,
		Parser:      &mockTimestampParser{},
	})
	ws.setBaseContext(context.Background())
	ws.onPCAPProgress = func(current, total uint64) {
		progressCurrent = current
		progressTotal = total
	}

	err := ws.startPCAPLockedWithConfig("analysis-seams.pcap", ReplayConfig{
		AnalysisMode:     true,
		DisableRecording: true,
		SpeedMode:        "analysis",
		SensorID:         sensorID,
	})
	if err != nil {
		t.Fatalf("startPCAPLockedWithConfig() error: %v", err)
	}

	waitForPCAPDone(t, ws)

	if progressCurrent != 3 || progressTotal != 7 {
		t.Fatalf("unexpected progress callback values: got (%d, %d)", progressCurrent, progressTotal)
	}
	if calls := frameBuilder.blockCalls(); len(calls) != 2 || !calls[0] || calls[1] {
		t.Fatalf("unexpected SetBlockOnFrameChannel calls: %#v", calls)
	}
}

func TestStartPCAPLocked_SettleBeforeRecordingRunsTwoPasses(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)

	sensorID := "pcap-settle-before-recording"
	tmpDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(tmpDir, "two-pass.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	dbWrapped, cleanupDB := setupTestDBWrapped(t)
	defer cleanupDB()

	var events []string
	countPCAPPackets = func(string, int) (network.PCAPCountResult, error) {
		return network.PCAPCountResult{Count: 10}, nil
	}
	var passDuringSettle, passDuringRecorded ReplayPass
	var recordingDuringSettle bool
	var wsRef *Server
	readPCAPFile = func(
		_ context.Context,
		_ string,
		_ int,
		_ network.Parser,
		_ network.FrameBuilder,
		_ network.PacketStatsInterface,
		_ *network.PacketForwarder,
		_ float64,
		_ float64,
		_ uint64,
		_ uint64,
		_ func(current, total uint64),
	) error {
		state := wsRef.PipelineState()
		if len(events) == 0 {
			events = append(events, "settle-pass")
			passDuringSettle = state.Pass
			recordingDuringSettle = state.Recording
		} else {
			events = append(events, "recorded-pass")
			passDuringRecorded = state.Pass
		}
		return nil
	}
	prepareSettledPCAPReplay = func(_ *Server, gotSensorID, sourcePath string) error {
		if gotSensorID != sensorID {
			t.Errorf("prepare sensor = %q, want %q", gotSensorID, sensorID)
		}
		if sourcePath != filepath.Join(tmpDir, "two-pass.pcap") {
			t.Errorf("prepare path = %q", sourcePath)
		}
		events = append(events, "reload-grid")
		return nil
	}
	frameBuilder := &stubReplayFrameBuilder{}
	getReplayFrameBuilder = func(string) replayFrameBuilder { return frameBuilder }

	ws := NewServer(Config{
		Address:     ":0",
		Stats:       NewPacketStats(),
		SensorID:    sensorID,
		PCAPSafeDir: tmpDir,
		DB:          dbWrapped,
		Parser:      &mockTimestampParser{},
		OnRecordingStart: func(string) string {
			events = append(events, "recording-start")
			return "/tmp/events.vrlog"
		},
		OnRecordingStop: func(string) string {
			events = append(events, "recording-stop")
			return ""
		},
	})
	ws.setBaseContext(context.Background())
	wsRef = ws

	if got := ws.PipelineState().TotalPasses; got != 1 {
		t.Fatalf("TotalPasses before start = %d, want 1", got)
	}

	err := ws.startPCAPLockedWithConfig("two-pass.pcap", ReplayConfig{
		AnalysisMode:          true,
		SettleBeforeRecording: true,
		SpeedMode:             "analysis",
		SensorID:              sensorID,
	})
	if err != nil {
		t.Fatalf("startPCAPLockedWithConfig() error: %v", err)
	}
	waitForPCAPDone(t, ws)

	want := []string{"settle-pass", "reload-grid", "recording-start", "recorded-pass", "recording-stop"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %#v, want %#v", events, want)
	}

	// The two passes must be distinguishable from outside: both replay the
	// same window and report the same source, and packet progress restarts
	// between them.
	if passDuringSettle != ReplayPassSettling {
		t.Errorf("pass during settling = %q, want %q", passDuringSettle, ReplayPassSettling)
	}
	if recordingDuringSettle {
		t.Error("recording reported active during the settling pass")
	}
	if passDuringRecorded != ReplayPassRecording {
		t.Errorf("pass during recorded pass = %q, want %q", passDuringRecorded, ReplayPassRecording)
	}
	frameBuilder.mu.Lock()
	drains := frameBuilder.drains
	flushes := frameBuilder.flushes
	frameBuilder.mu.Unlock()
	if drains != 1 {
		t.Fatalf("final callback drains = %d, want 1", drains)
	}
	if flushes != 1 {
		t.Fatalf("final pending-frame flushes = %d, want 1", flushes)
	}
}

func TestStartPCAPLocked_RealtimePlotsSuccessAndStopped(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)

	sensorID := "pcap-realtime-plot-success"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	tmpDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(tmpDir, "realtime-success.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	var timestamps [2]int64
	var stopped bool
	countPCAPPackets = func(string, int) (network.PCAPCountResult, error) {
		return network.PCAPCountResult{
			Count:            11,
			FirstTimestampNs: 10,
			LastTimestampNs:  90,
		}, nil
	}
	newForegroundForwarder = func(string, int, *network.SensorConfig) (*network.ForegroundForwarder, error) {
		return nil, errors.New("ff failed")
	}
	readPCAPFileRealtime = func(
		_ context.Context,
		_ string,
		_ int,
		_ network.Parser,
		_ network.FrameBuilder,
		_ network.PacketStatsInterface,
		cfg network.RealtimeReplayConfig,
	) error {
		if cfg.OnFrameCallback == nil {
			t.Fatal("expected OnFrameCallback")
		}
		cfg.OnFrameCallback(l3grid.GetBackgroundManager(sensorID), []l2frames.PointPolar{
			{Channel: 2, Azimuth: 10, Distance: 5},
		})
		return nil
	}

	ws := NewServer(Config{
		Address:           ":0",
		Stats:             NewPacketStats(),
		SensorID:          sensorID,
		PCAPSafeDir:       tmpDir,
		PlotsBaseDir:      filepath.Join(tmpDir, "plots"),
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
		OnPCAPStopped:     func() { stopped = true },
	})
	defer stopLiveListenerIfRunning(ws)

	ws.setBaseContext(context.Background())
	ws.onPCAPTimestamps = func(startNs, endNs int64) {
		timestamps[0] = startNs
		timestamps[1] = endNs
	}

	err := ws.startPCAPLockedWithConfig("realtime-success.pcap", ReplayConfig{
		SpeedMode:    "scaled",
		SpeedRatio:   1.25,
		SensorID:     sensorID,
		EnablePlots:  true,
		DebugRingMin: 1,
		DebugRingMax: 1,
		DebugAzMax:   359,
	})
	if err != nil {
		t.Fatalf("startPCAPLockedWithConfig() error: %v", err)
	}

	waitForPCAPDone(t, ws)

	if timestamps != [2]int64{10, 90} {
		t.Fatalf("unexpected timestamp callback values: %#v", timestamps)
	}
	if !stopped {
		t.Fatal("expected onPCAPStopped callback")
	}
}

func TestStartPCAPLocked_RealtimePlotGenerateError(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)

	sensorID := "pcap-realtime-plot-error"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	tmpDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(tmpDir, "realtime-plot-error.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	countPCAPPackets = func(string, int) (network.PCAPCountResult, error) {
		return network.PCAPCountResult{Count: 9}, nil
	}

	ws := NewServer(Config{
		Address:           ":0",
		Stats:             NewPacketStats(),
		SensorID:          sensorID,
		PCAPSafeDir:       tmpDir,
		PlotsBaseDir:      filepath.Join(tmpDir, "plots-error"),
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	})
	defer stopLiveListenerIfRunning(ws)

	ws.setBaseContext(context.Background())

	readPCAPFileRealtime = func(
		_ context.Context,
		_ string,
		_ int,
		_ network.Parser,
		_ network.FrameBuilder,
		_ network.PacketStatsInterface,
		cfg network.RealtimeReplayConfig,
	) error {
		cfg.OnFrameCallback(l3grid.GetBackgroundManager(sensorID), []l2frames.PointPolar{
			{Channel: 2, Azimuth: 10, Distance: 5},
		})
		if ws.gridPlotter == nil {
			t.Fatal("expected grid plotter")
		}
		if err := os.RemoveAll(ws.gridPlotter.GetOutputDir()); err != nil {
			t.Fatalf("RemoveAll(plot dir): %v", err)
		}
		return nil
	}

	err := ws.startPCAPLockedWithConfig("realtime-plot-error.pcap", ReplayConfig{
		SpeedMode:    "scaled",
		SpeedRatio:   1.25,
		SensorID:     sensorID,
		EnablePlots:  true,
		DebugRingMin: 1,
		DebugRingMax: 1,
		DebugAzMax:   359,
	})
	if err != nil {
		t.Fatalf("startPCAPLockedWithConfig() error: %v", err)
	}

	waitForPCAPDone(t, ws)
}

func TestStartPCAPLocked_AnalysisMode_FailRunError(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)

	sensorID := "pcap-analysis-failrun"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	tmpDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(tmpDir, "analysis-failrun.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	dbWrapped, cleanupDB := setupTestDBWrapped(t)
	defer cleanupDB()

	readPCAPFile = func(
		_ context.Context,
		_ string,
		_ int,
		_ network.Parser,
		_ network.FrameBuilder,
		_ network.PacketStatsInterface,
		_ *network.PacketForwarder,
		_ float64,
		_ float64,
		_ uint64,
		_ uint64,
		_ func(current, total uint64),
	) error {
		if err := dbWrapped.DB.Close(); err != nil {
			t.Fatalf("close db: %v", err)
		}
		return errors.New("replay failed")
	}

	ws := NewServer(Config{
		Address:     ":0",
		Stats:       NewPacketStats(),
		SensorID:    sensorID,
		PCAPSafeDir: tmpDir,
		DB:          dbWrapped,
	})
	ws.setBaseContext(context.Background())

	err := ws.startPCAPLockedWithConfig("analysis-failrun.pcap", ReplayConfig{
		AnalysisMode:     true,
		DisableRecording: true,
		SpeedMode:        "analysis",
		SensorID:         sensorID,
		PreferredRunID:   "analysis-failrun",
	})
	if err != nil {
		t.Fatalf("startPCAPLockedWithConfig() error: %v", err)
	}

	waitForPCAPDone(t, ws)
}

func TestStartPCAPLocked_AnalysisMode_CompleteAndVRLogUpdateErrors(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)

	sensorID := "pcap-analysis-complete"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	tmpDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(tmpDir, "analysis-complete.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	dbWrapped, cleanupDB := setupTestDBWrapped(t)
	defer cleanupDB()

	readPCAPFile = func(
		_ context.Context,
		_ string,
		_ int,
		_ network.Parser,
		_ network.FrameBuilder,
		_ network.PacketStatsInterface,
		_ *network.PacketForwarder,
		_ float64,
		_ float64,
		_ uint64,
		_ uint64,
		_ func(current, total uint64),
	) error {
		if err := dbWrapped.DB.Close(); err != nil {
			t.Fatalf("close db: %v", err)
		}
		return nil
	}

	ws := NewServer(Config{
		Address:          ":0",
		Stats:            NewPacketStats(),
		SensorID:         sensorID,
		PCAPSafeDir:      tmpDir,
		DB:               dbWrapped,
		OnRecordingStart: func(string) string { return "/tmp/test.vrlog" },
		OnRecordingStop:  func(string) string { return "/tmp/test.vrlog" },
	})
	ws.setBaseContext(context.Background())

	err := ws.startPCAPLockedWithConfig("analysis-complete.pcap", ReplayConfig{
		AnalysisMode:   true,
		SpeedMode:      "analysis",
		SensorID:       sensorID,
		PreferredRunID: "analysis-complete",
	})
	if err != nil {
		t.Fatalf("startPCAPLockedWithConfig() error: %v", err)
	}

	waitForPCAPDone(t, ws)
}

func TestStartPCAPLocked_NonAnalysis_ResetStateErrorAndStopped(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)

	sensorID := "pcap-reset-state-error"
	l3grid.RegisterBackgroundManager(sensorID, &l3grid.BackgroundManager{})
	defer func() { _ = l3grid.NewBackgroundManager(sensorID, 2, 2, l3grid.BackgroundParams{}, nil) }()

	tmpDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(tmpDir, "reset-state-error.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	readPCAPFile = func(
		_ context.Context,
		_ string,
		_ int,
		_ network.Parser,
		_ network.FrameBuilder,
		_ network.PacketStatsInterface,
		_ *network.PacketForwarder,
		_ float64,
		_ float64,
		_ uint64,
		_ uint64,
		_ func(current, total uint64),
	) error {
		return nil
	}

	var stopped bool
	ws := NewServer(Config{
		Address:           ":0",
		Stats:             NewPacketStats(),
		SensorID:          sensorID,
		PCAPSafeDir:       tmpDir,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
		OnPCAPStopped:     func() { stopped = true },
	})
	defer stopLiveListenerIfRunning(ws)

	ws.setBaseContext(context.Background())

	err := ws.startPCAPLocked("reset-state-error.pcap", "analysis", 1.0, 0, 0, 0, 0, 0, 0, false, false)
	if err != nil {
		t.Fatalf("startPCAPLocked() error: %v", err)
	}

	waitForPCAPDone(t, ws)

	if !stopped {
		t.Fatal("expected onPCAPStopped callback")
	}
}

func TestStartPCAPLocked_NonAnalysis_StartLiveListenerError(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)

	sensorID := "pcap-start-live-error"
	cleanupBg := setupTestBackgroundManager(t, sensorID)
	defer cleanupBg()

	tmpDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(tmpDir, "start-live-error.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	ws := NewServer(Config{
		Address:           ":0",
		Stats:             NewPacketStats(),
		SensorID:          sensorID,
		PCAPSafeDir:       tmpDir,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	})
	readPCAPFile = func(
		_ context.Context,
		_ string,
		_ int,
		_ network.Parser,
		_ network.FrameBuilder,
		_ network.PacketStatsInterface,
		_ *network.PacketForwarder,
		_ float64,
		_ float64,
		_ uint64,
		_ uint64,
		_ func(current, total uint64),
	) error {
		ws.setBaseContext(nil)
		return nil
	}

	ws.setBaseContext(context.Background())

	err := ws.startPCAPLocked("start-live-error.pcap", "analysis", 1.0, 0, 0, 0, 0, 0, 0, false, false)
	if err != nil {
		t.Fatalf("startPCAPLocked() error: %v", err)
	}

	waitForPCAPDone(t, ws)
}

func TestResolvePCAPPath_AbsAndStatErrors(t *testing.T) {
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)

	tmpDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(tmpDir, "stat-error.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	ws := &Server{pcapSafeDir: tmpDir}

	absPath = func(string) (string, error) {
		return "", errors.New("abs failed")
	}
	if _, err := ws.resolvePCAPPath("stat-error.pcap"); err == nil || !strings.Contains(err.Error(), "invalid PCAP safe directory configuration") {
		t.Fatalf("expected abs-path error, got %v", err)
	}

	absPath = filepath.Abs
	statPath = func(string) (os.FileInfo, error) {
		return nil, errors.New("stat failed")
	}
	if _, err := ws.resolvePCAPPath("stat-error.pcap"); err == nil || !strings.Contains(err.Error(), "cannot access PCAP file") {
		t.Fatalf("expected stat-path error, got %v", err)
	}
}
