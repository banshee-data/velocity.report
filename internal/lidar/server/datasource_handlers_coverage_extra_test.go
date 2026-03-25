package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

type stubReplayFrameBuilder struct {
	dropped uint64
	calls   []bool
}

func (s *stubReplayFrameBuilder) SetBlockOnFrameChannel(block bool) {
	s.calls = append(s.calls, block)
}

func (s *stubReplayFrameBuilder) DroppedFrames() uint64 {
	return s.dropped
}

func restoreDatasourceHandlerSeams() func() {
	origCount := countPCAPPackets
	origRead := readPCAPFile
	origReadRealtime := readPCAPFileRealtime
	origNewForegroundForwarder := newForegroundForwarder
	origAbsPath := absPath
	origStatPath := statPath
	origGetReplayFrameBuilder := getReplayFrameBuilder
	return func() {
		countPCAPPackets = origCount
		readPCAPFile = origRead
		readPCAPFileRealtime = origReadRealtime
		newForegroundForwarder = origNewForegroundForwarder
		absPath = origAbsPath
		statPath = origStatPath
		getReplayFrameBuilder = origGetReplayFrameBuilder
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
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()

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
	if len(frameBuilder.calls) != 2 || !frameBuilder.calls[0] || frameBuilder.calls[1] {
		t.Fatalf("unexpected SetBlockOnFrameChannel calls: %#v", frameBuilder.calls)
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
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()

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
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()

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
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()

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
		OnRecordingStart: func(string) {},
		OnRecordingStop:  func(string) string { return "/tmp/test.vrlog" },
	})
	ws.setBaseContext(context.Background())
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()

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
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()

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
	ws.dataSourceMu.Lock()
	ws.currentSource = DataSourcePCAP
	ws.dataSourceMu.Unlock()

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
