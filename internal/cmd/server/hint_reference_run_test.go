package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/server"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
)

// fastHintRunCreator is a hintRunCreator whose reference run samples once
// and polls often, so a test spends its time on the replay alone.
func fastHintRunCreator(runner *sweep.Runner) *hintRunCreator {
	return &hintRunCreator{
		runner:       runner,
		pollInterval: 10 * time.Millisecond,
		timeout:      30 * time.Second,
		settleTime:   "0s",
		interval:     "1ms",
		iterations:   1,
	}
}

// A HINT reference run over a clip replays the clip's capture through the
// in-process server, returns the analysis run's ID and leaves a run record
// that names the clip.
func TestHintRunCreator_ReferenceRunReplaysTheClip(t *testing.T) {
	sensorID := "test-hint-reference-run"
	_ = l3grid.NewBackgroundManager(sensorID, 128, 360, l3grid.BackgroundParams{}, nil)
	t.Cleanup(func() { l3grid.RegisterBackgroundManager(sensorID, nil) })

	captureDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// A capture with a global header and no packets: the replay starts an
	// analysis run and finishes at once.
	pcapHeader := []byte{
		0xd4, 0xc3, 0xb2, 0xa1, 0x02, 0x00, 0x04, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xff, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
	}
	if err := os.WriteFile(filepath.Join(captureDir, "reference.pcap"), pcapHeader, 0o644); err != nil {
		t.Fatal(err)
	}

	database, cleanupDB := dbpkg.NewTestDB(t)
	defer cleanupDB()
	clipStore := sqlite.NewClipStore(database)
	stored := &sqlite.Clip{SensorID: sensorID, Files: []sqlite.ClipCapture{{PCAPFile: "reference.pcap"}}}
	if err := clipStore.InsertClip(stored); err != nil {
		t.Fatalf("insert clip: %v", err)
	}

	ws := server.NewServer(server.Config{
		Stats:             server.NewPacketStats(),
		SensorID:          sensorID,
		PCAPSafeDir:       captureDir,
		DB:                database,
		UDPListenerConfig: network.UDPListenerConfig{Address: "127.0.0.1:0"},
		ListenFallback:    true,
	})
	ready := make(chan struct{})
	ws.SetOnReady(func() { close(ready) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = ws.Start(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("the LiDAR server did not start")
	}

	// The clip as HINT reads it, and the reference run HINT asks for.
	clip, err := (&hintClipAdapter{store: clipStore}).GetClip(stored.ClipID)
	if err != nil {
		t.Fatalf("GetClip: %v", err)
	}
	creator := fastHintRunCreator(sweep.NewRunner(server.NewDirectBackend(sensorID, ws)))
	runID, err := creator.CreateSweepRun(sweep.ReferenceRun{
		ClipID:           clip.ClipID,
		SensorID:         clip.SensorID,
		PCAPFile:         clip.PCAPFile,
		PCAPStartSecs:    0,
		PCAPDurationSecs: -1,
		ParamsJSON:       json.RawMessage(`{"l3.ema_baseline_v1.noise_relative":0.02}`),
	})
	if err != nil {
		t.Fatalf("CreateSweepRun: %v", err)
	}
	if runID == "" {
		t.Fatal("CreateSweepRun returned an empty run ID")
	}
	if last := ws.LastRunID(); last != runID {
		t.Errorf("run ID = %q, want the replay's analysis run %q", runID, last)
	}

	run, err := sqlite.NewRunStore(database).GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun(%s): %v", runID, err)
	}
	if run.ClipID != stored.ClipID {
		t.Errorf("run %s clip = %q, want %q", runID, run.ClipID, stored.ClipID)
	}
	if run.SourceType != "pcap" {
		t.Errorf("run %s source type = %q, want a capture replay", runID, run.SourceType)
	}
	var column *string
	if err := database.QueryRow(`SELECT clip_id FROM lidar_runs WHERE run_id = ?`, runID).Scan(&column); err != nil {
		t.Fatalf("read clip_id: %v", err)
	}
	if column == nil || *column != stored.ClipID {
		t.Errorf("lidar_runs.clip_id = %v, want %q", column, stored.ClipID)
	}
}

// replayingSweepBackend replays nothing but records the replay it was asked
// for and answers with a run ID, as a server does once a replay has started
// its analysis run. A live sweep resets the grid instead, which it counts.
type replayingSweepBackend struct {
	erroringSweepBackend

	mu         sync.Mutex
	replays    []sweep.PCAPReplayConfig
	gridResets int
}

func (b *replayingSweepBackend) SetTuningParams(map[string]interface{}) error { return nil }

func (b *replayingSweepBackend) StartPCAPReplayWithConfig(cfg sweep.PCAPReplayConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.replays = append(b.replays, cfg)
	return nil
}

func (b *replayingSweepBackend) ResetGrid() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gridResets++
	return errors.New("live sweep: no grid to reset")
}

func (b *replayingSweepBackend) GetLastRunID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.replays) == 0 {
		return ""
	}
	return "run-from-replay"
}

// The reference run used to be sent as data source "pcap", which the runner
// does not replay: it swept the live sensor instead and HINT failed with
// "reference run completed without run ID". It now replays the clip's
// capture window and names the clip on the replay.
func TestHintRunCreator_ReferenceRunIsACaptureReplay(t *testing.T) {
	backend := &replayingSweepBackend{}
	creator := fastHintRunCreator(sweep.NewRunner(backend))

	runID, err := creator.CreateSweepRun(sweep.ReferenceRun{
		ClipID:           "clip-42",
		SensorID:         "test-sensor",
		PCAPFile:         "site/kirk0.pcapng",
		PCAPStartSecs:    12.5,
		PCAPDurationSecs: 40,
		ParamsJSON:       json.RawMessage(`{"l3.ema_baseline_v1.noise_relative":0.02}`),
	})
	if err != nil {
		if strings.Contains(err.Error(), "completed without run ID") {
			t.Fatalf("the reference run did not replay its capture: %v", err)
		}
		t.Fatalf("CreateSweepRun: %v", err)
	}
	if runID != "run-from-replay" {
		t.Errorf("run ID = %q, want %q", runID, "run-from-replay")
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.gridResets != 0 {
		t.Errorf("the reference run reset the live grid %d times, want a replay only", backend.gridResets)
	}
	if len(backend.replays) != 1 {
		t.Fatalf("replays = %d, want 1", len(backend.replays))
	}
	got := backend.replays[0]
	if got.PCAPFile != "site/kirk0.pcapng" || got.StartSeconds != 12.5 || got.DurationSeconds != 40 {
		t.Errorf("replay = %s from %gs for %gs, want site/kirk0.pcapng from 12.5s for 40s",
			got.PCAPFile, got.StartSeconds, got.DurationSeconds)
	}
	if got.ClipID != "clip-42" {
		t.Errorf("replay clip = %q, want %q", got.ClipID, "clip-42")
	}
	if !got.AnalysisMode {
		t.Error("replay is not in analysis mode, so it starts no analysis run")
	}
	if got.DisableRecording {
		t.Error("replay disables recording; a reference run records a VRLOG for labelling")
	}
}
