package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Regression tests for a scene batch run against a second server on a host
// that already had one. The first server held the visualiser's gRPC port, the
// second was started without a publisher to get past it, and every replay then
// ran to the end and reported 'completed' with no VRLOG — forty minutes a site
// before the exporter found nothing to export.

// replayOpts is what varies between the replays below: what the recorder
// returns, whether the availability probe is wired and what it says, and
// whether the replay settles first, as publish-scenes.py does by default.
type replayOpts struct {
	recordingPath string
	available     func() bool
	settle        bool
}

// replayWithRecorder replays one stub capture in analysis mode with recording
// requested, the way publish-scenes.py starts a scene, and reports how many
// times the capture was read and the run record the replay left behind.
func replayWithRecorder(t *testing.T, recordingPath string) (reads int, run *sqlite.Run, ws *Server) {
	t.Helper()
	return replayWith(t, replayOpts{recordingPath: recordingPath})
}

func replayWith(t *testing.T, opts replayOpts) (reads int, run *sqlite.Run, ws *Server) {
	t.Helper()
	recordingPath := opts.recordingPath
	restore := restoreDatasourceHandlerSeams()
	t.Cleanup(restore)

	sensorID := "second-server-" + strings.ReplaceAll(t.Name(), "/", "-")
	tmpDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(tmpDir, "scene.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatal(err)
	}
	dbWrapped, cleanupDB := setupTestDBWrapped(t)
	t.Cleanup(cleanupDB)

	countPCAPPackets = func(string, int) (network.PCAPCountResult, error) {
		return network.PCAPCountResult{Count: 10}, nil
	}
	readPCAPFile = func(context.Context, string, int, network.Parser, network.FrameBuilder,
		network.PacketStatsInterface, *network.PacketForwarder, float64, float64, uint64, uint64,
		func(current, total uint64)) error {
		reads++
		return nil
	}
	prepareSettledPCAPReplay = func(*Server, string, string) error { return nil }

	ws = NewServer(Config{
		Address:            ":0",
		Stats:              NewPacketStats(),
		SensorID:           sensorID,
		PCAPSafeDir:        tmpDir,
		DB:                 dbWrapped,
		Parser:             &mockTimestampParser{},
		RecordingAvailable: opts.available,
		OnRecordingStart:   func(string) string { return recordingPath },
		OnRecordingStop:    func(string) string { return recordingPath },
	})
	ws.setBaseContext(context.Background())

	if err := ws.startPCAPLockedWithConfig("scene.pcap", ReplayConfig{
		AnalysisMode:          true,
		SettleBeforeRecording: opts.settle,
		SpeedMode:             "analysis",
		SensorID:              sensorID,
	}); err != nil {
		t.Fatalf("startPCAPLockedWithConfig: %v", err)
	}
	waitForPCAPDone(t, ws)

	runID := ws.LastRunID()
	if runID == "" {
		t.Fatal("no analysis run was started")
	}
	run, err := sqlite.NewRunStore(dbWrapped).GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun(%s): %v", runID, err)
	}
	return reads, run, ws
}

// A replay that asked for a recording the server cannot make fails at once,
// before any packets are replayed, and the run says why.
func TestReplay_RecordingUnavailableFailsTheRunBeforeReplaying(t *testing.T) {
	reads, run, ws := replayWithRecorder(t, "")

	if reads != 0 {
		t.Errorf("capture read %d time(s) although the recording could not start; want 0", reads)
	}
	if run.Status != "failed" {
		t.Errorf("run status = %q, want failed: a run with no VRLOG must not read as completed", run.Status)
	}
	if !strings.Contains(run.ErrorMessage, "VRLOG recording could not start") {
		t.Errorf("run error = %q, want it to name the recording", run.ErrorMessage)
	}
	if ws.PipelineState().Recording {
		t.Error("recording reported active although it never started")
	}
}

// Settling is publish-scenes.py's default, and the recorder starts only after
// the settling pass. The availability probe fails the run before that pass, so
// a run that cannot record does not read the whole capture once to settle.
func TestReplay_RecordingUnavailableFailsBeforeTheSettlingPass(t *testing.T) {
	reads, run, _ := replayWith(t, replayOpts{
		available: func() bool { return false },
		settle:    true,
	})

	if reads != 0 {
		t.Errorf("capture read %d time(s) although the probe said recording is unavailable; want 0", reads)
	}
	if run.Status != "failed" || !strings.Contains(run.ErrorMessage, "VRLOG recording could not start") {
		t.Errorf("run = %q (%q), want failed naming the recording", run.Status, run.ErrorMessage)
	}
}

// A recorder that fails although the probe said yes is still caught when it
// starts: the settling pass has run, the recorded pass does not, and the run
// fails rather than completing with no VRLOG.
func TestReplay_RecorderFailingAfterAPassingProbeStillFailsTheRun(t *testing.T) {
	reads, run, _ := replayWith(t, replayOpts{
		available: func() bool { return true },
		settle:    true,
	})

	if reads != 1 {
		t.Errorf("capture read %d time(s), want 1 (the settling pass only)", reads)
	}
	if run.Status != "failed" {
		t.Errorf("run status = %q, want failed", run.Status)
	}
}

// With a recorder that works, settling and recording both run and complete.
func TestReplay_SettledRecordingReplaysTwiceAndCompletes(t *testing.T) {
	reads, run, _ := replayWith(t, replayOpts{
		recordingPath: "/tmp/scene.vrlog",
		available:     func() bool { return true },
		settle:        true,
	})

	if reads != 2 {
		t.Errorf("capture read %d time(s), want 2 (settle, then record)", reads)
	}
	if run.Status != "completed" {
		t.Errorf("run status = %q, want completed (error %q)", run.Status, run.ErrorMessage)
	}
}

// The control: with a recorder the same replay reads the capture and completes.
func TestReplay_RecordingAvailableReplaysAndCompletes(t *testing.T) {
	reads, run, _ := replayWithRecorder(t, "/tmp/scene.vrlog")

	if reads != 1 {
		t.Errorf("capture read %d time(s), want 1", reads)
	}
	if run.Status != "completed" {
		t.Errorf("run status = %q, want completed (error %q)", run.Status, run.ErrorMessage)
	}
}

// startMonitor runs Start in the background and waits for it to serve.
func startMonitor(t *testing.T, srv *Server) {
	t.Helper()
	ready := make(chan struct{})
	srv.SetOnReady(func() { close(ready) })
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- srv.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-errc
	})
	select {
	case <-ready:
	case err := <-errc:
		t.Fatalf("Start returned before serving: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not become ready")
	}
}

// The monitor port held by another server moves rather than taking the
// monitor down, and Addr names the port that was bound.
func TestStart_HeldMonitorPortFallsBack(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	srv := NewServer(Config{
		Address:        held.Addr().String(),
		ListenFallback: true,
		Stats:          NewPacketStats(),
		UDPListenerConfig: network.UDPListenerConfig{
			Address: "127.0.0.1:0",
		},
	})
	startMonitor(t, srv)

	got := srv.Addr()
	if got == "" || got == held.Addr().String() {
		t.Fatalf("Addr() = %q, want a port other than the held %s", got, held.Addr())
	}
	resp, err := http.Get("http://" + got + "/")
	if err != nil {
		t.Fatalf("the fallback address does not serve: %v", err)
	}
	resp.Body.Close()
}

// Without fallback a held port is still a startup error, as it was.
func TestStart_HeldMonitorPortFailsWithoutFallback(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	srv := NewServer(Config{Address: held.Addr().String(), Stats: NewPacketStats()})
	err = srv.Start(context.Background())
	if err == nil {
		t.Fatal("Start returned nil on a held port without ListenFallback")
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		t.Errorf("start error = %v, want address in use", err)
	}
}

// The live sensor port cannot move, so a server tolerating another on the host
// serves replays without it rather than not serving at all.
func TestStart_HeldLiveUDPPortServesReplaysWithFallback(t *testing.T) {
	srv := NewServer(Config{
		Address:        "127.0.0.1:0",
		ListenFallback: true,
		Stats:          NewPacketStats(),
		UDPListenerConfig: network.UDPListenerConfig{
			Address: "127.0.0.1:0", SocketFactory: failingUDPSockets{},
		},
	})
	startMonitor(t, srv)
	if srv.Addr() == "" {
		t.Fatal("monitor did not bind although only the UDP listener failed")
	}
}
