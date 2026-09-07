package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
)

// launchServer builds a server whose safe directory holds the named captures,
// with the packet counts stubbed to a clean five-minute roll-over series.
func launchServer(t *testing.T, sensorID string, names ...string) *Server {
	t.Helper()
	tmpDir := resolveSymlinks(t, t.TempDir())
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(tmpDir, name), testPCAPHeader, 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	index := make(map[string]int, len(names))
	for i, name := range names {
		index[name] = i
	}
	countPCAPPackets = func(path string, _ int) (network.PCAPCountResult, error) {
		i := index[filepath.Base(path)]
		// 4 ms of roll-over cost, comfortably inside the seamless bound, so no
		// revolution is dropped and the test is about sequencing alone.
		offset := time.Duration(i) * (testFiveMin + 4*time.Millisecond)
		return network.PCAPCountResult{
			Count:            1000,
			FirstTimestampNs: base.Add(offset).UnixNano(),
			LastTimestampNs:  base.Add(offset + testFiveMin).UnixNano(),
		}, nil
	}

	ws := NewServer(Config{
		Address:     ":0",
		Stats:       NewPacketStats(),
		SensorID:    sensorID,
		PCAPSafeDir: tmpDir,
		Parser:      &mockTimestampParser{},
	})
	ws.setBaseContext(context.Background())
	return ws
}

func TestStartPCAPLockedRefusesMultiFileOutsideAnalysisMode(t *testing.T) {
	// Realtime and scaled replay pace packets against one file's clock. Silently
	// replaying only the first file would look like success, so the request is
	// refused instead.
	t.Cleanup(restoreDatasourceHandlerSeams())
	ws := launchServer(t, "multi-realtime", "a.pcap", "b.pcap")

	err := ws.startPCAPLockedWithConfig("a.pcap", ReplayConfig{
		ReplayFiles:     []string{"a.pcap", "b.pcap"},
		SpeedMode:       "realtime",
		DurationSeconds: -1,
	})
	if err == nil {
		t.Fatal("multi-file realtime replay accepted, want an error")
	}
	for _, want := range []string{"analysis", "realtime"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	// The refusal must release the replay slot, or the next request sees a
	// spurious conflict.
	if state := ws.PipelineState(); state.Source == SourceModePCAP {
		t.Error("a refused multi-file start left the PCAP slot claimed")
	}
}

func TestStartPCAPLockedReplaysASequenceInAnalysisMode(t *testing.T) {
	t.Cleanup(restoreDatasourceHandlerSeams())
	sensorID := "multi-analysis"
	ws := launchServer(t, sensorID, "a.pcap", "b.pcap", "c.pcap")

	var mu sync.Mutex
	var gotSteps []capseq.ReadStep
	readPCAPSequence = func(_ context.Context, steps []capseq.ReadStep,
		cfg network.SequenceReplayConfig) (network.SequenceResult, error) {
		mu.Lock()
		gotSteps = append([]capseq.ReadStep(nil), steps...)
		mu.Unlock()
		if cfg.OnProgress != nil {
			cfg.OnProgress(3000, 3000)
		}
		return network.SequenceResult{StepsCompleted: len(steps)}, nil
	}
	// A sequence replay must not fall back to the single-file reader.
	readPCAPFile = func(_ context.Context, path string, _ int, _ network.Parser,
		_ network.FrameBuilder, _ network.PacketStatsInterface, _ *network.PacketForwarder,
		_, _ float64, _, _ uint64, _ func(current, total uint64)) error {
		t.Errorf("single-file reader called for %s during a sequence replay", path)
		return nil
	}
	getReplayFrameBuilder = func(string) replayFrameBuilder {
		return &stubReplayFrameBuilder{}
	}

	var progressCurrent, progressTotal uint64
	ws.onPCAPProgress = func(current, total uint64) {
		progressCurrent, progressTotal = current, total
	}
	var firstNs, lastNs int64
	ws.onPCAPTimestamps = func(first, last int64) { firstNs, lastNs = first, last }

	err := ws.startPCAPLockedWithConfig("a.pcap", ReplayConfig{
		ReplayFiles:      []string{"a.pcap", "b.pcap", "c.pcap"},
		AnalysisMode:     true,
		DisableRecording: true,
		SpeedMode:        "analysis",
		SensorID:         sensorID,
		DurationSeconds:  -1,
	})
	if err != nil {
		t.Fatalf("startPCAPLockedWithConfig(): %v", err)
	}
	waitForPCAPDone(t, ws)

	mu.Lock()
	steps := gotSteps
	mu.Unlock()
	if len(steps) != 3 {
		t.Fatalf("sequence reader got %d steps, want 3", len(steps))
	}
	for i, want := range []string{"a.pcap", "b.pcap", "c.pcap"} {
		if got := filepath.Base(steps[i].Path); got != want {
			t.Errorf("step %d = %q, want %q", i, got, want)
		}
	}

	// Progress must be scaled to the whole sequence, not to the first file: a
	// per-file total would show the bar completing three times over.
	if progressTotal != 3000 {
		t.Errorf("progress total = %d, want 3000 across all three files", progressTotal)
	}
	if progressCurrent != 3000 {
		t.Errorf("progress current = %d, want 3000", progressCurrent)
	}

	// The timeline extents must span the sequence, not the primary file.
	span := time.Duration(lastNs - firstNs)
	if want := 3*testFiveMin + 8*time.Millisecond; span != want {
		t.Errorf("reported span = %v, want %v", span, want)
	}
}

func TestStartPCAPLockedRejectsABrokenSequenceBeforeStarting(t *testing.T) {
	t.Cleanup(restoreDatasourceHandlerSeams())
	tmpDir := resolveSymlinks(t, t.TempDir())
	for _, name := range []string{"a.pcap", "b.pcap"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), testPCAPHeader, 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	countPCAPPackets = func(path string, _ int) (network.PCAPCountResult, error) {
		offset := time.Duration(0)
		if filepath.Base(path) == "b.pcap" {
			// A minute missing between the files.
			offset = testFiveMin + time.Minute
		}
		return network.PCAPCountResult{
			Count:            1000,
			FirstTimestampNs: base.Add(offset).UnixNano(),
			LastTimestampNs:  base.Add(offset + testFiveMin).UnixNano(),
		}, nil
	}
	readPCAPSequence = func(_ context.Context, _ []capseq.ReadStep,
		_ network.SequenceReplayConfig) (network.SequenceResult, error) {
		t.Error("replay started despite a broken join")
		return network.SequenceResult{}, nil
	}

	ws := NewServer(Config{
		Address: ":0", Stats: NewPacketStats(), SensorID: "broken-seq",
		PCAPSafeDir: tmpDir, Parser: &mockTimestampParser{},
	})
	ws.setBaseContext(context.Background())

	err := ws.startPCAPLockedWithConfig("a.pcap", ReplayConfig{
		ReplayFiles:     []string{"a.pcap", "b.pcap"},
		AnalysisMode:    true,
		SpeedMode:       "analysis",
		DurationSeconds: -1,
	})
	if err == nil {
		t.Fatal("broken sequence accepted, want an error before replay starts")
	}
	if !strings.Contains(err.Error(), "continuous") {
		t.Errorf("error %q does not explain the sequence is not continuous", err)
	}
	if state := ws.PipelineState(); state.Source == SourceModePCAP {
		t.Error("a refused sequence left the PCAP slot claimed")
	}
}

func TestStartPCAPLockedSingleFileStillUsesTheFileReader(t *testing.T) {
	// The single-file path must be untouched: one ReplayFiles entry, or none,
	// takes the original reader with the original per-file totals.
	t.Cleanup(restoreDatasourceHandlerSeams())
	sensorID := "single-file-unchanged"
	ws := launchServer(t, sensorID, "a.pcap")

	var mu sync.Mutex
	var fileReads int
	readPCAPSequence = func(_ context.Context, _ []capseq.ReadStep,
		_ network.SequenceReplayConfig) (network.SequenceResult, error) {
		t.Error("sequence reader called for a single-file replay")
		return network.SequenceResult{}, nil
	}
	readPCAPFile = func(_ context.Context, _ string, _ int, _ network.Parser,
		_ network.FrameBuilder, _ network.PacketStatsInterface, _ *network.PacketForwarder,
		_, _ float64, _, _ uint64, onProgress func(current, total uint64)) error {
		mu.Lock()
		fileReads++
		mu.Unlock()
		if onProgress != nil {
			onProgress(1000, 1000)
		}
		return nil
	}
	getReplayFrameBuilder = func(string) replayFrameBuilder {
		return &stubReplayFrameBuilder{}
	}

	err := ws.startPCAPLockedWithConfig("a.pcap", ReplayConfig{
		ReplayFiles:      []string{"a.pcap"},
		AnalysisMode:     true,
		DisableRecording: true,
		SpeedMode:        "analysis",
		SensorID:         sensorID,
		DurationSeconds:  -1,
	})
	if err != nil {
		t.Fatalf("startPCAPLockedWithConfig(): %v", err)
	}
	waitForPCAPDone(t, ws)

	mu.Lock()
	got := fileReads
	mu.Unlock()
	if got != 1 {
		t.Errorf("single-file reader called %d times, want 1", got)
	}
}
