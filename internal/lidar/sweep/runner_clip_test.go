package sweep

import (
	"context"
	"sync"
	"testing"
	"time"
)

// clipRecordingBackend records every replay the runner asks for, so a test
// can read which clip each one named.
type clipRecordingBackend struct {
	*mockBackend
	mu      sync.Mutex
	replays []PCAPReplayConfig
}

func (b *clipRecordingBackend) StartPCAPReplayWithConfig(cfg PCAPReplayConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.replays = append(b.replays, cfg)
	return nil
}

func (b *clipRecordingBackend) clipIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := make([]string, len(b.replays))
	for i, cfg := range b.replays {
		ids[i] = cfg.ClipID
	}
	return ids
}

func newClipRecordingBackend() *clipRecordingBackend {
	return &clipRecordingBackend{mockBackend: runnerMockBackend()}
}

// Every combination of a sweep started for a clip replays under that clip,
// so each run record the replays start names it. A sweep with no clip names
// none.
func TestRunner_ReplaysCarryTheSweepClip(t *testing.T) {
	for _, tc := range []struct {
		name   string
		clipID string
	}{
		{"for a clip", "clip-kirk0"},
		{"without a clip", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := newClipRecordingBackend()
			r := newQuietRunner(backend)

			req := replaySlotSweepRequest()
			req.ClipID = tc.clipID
			if err := r.Start(context.Background(), req); err != nil {
				t.Fatalf("Start: %v", err)
			}
			state := waitForRunnerStatus(t, r, 10*time.Second, SweepStatusComplete, SweepStatusError)
			if state.Status != SweepStatusComplete {
				t.Fatalf("status = %q (%s), want complete", state.Status, state.Error)
			}

			ids := backend.clipIDs()
			if len(ids) != 3 {
				t.Fatalf("replays = %d, want one per combination (3)", len(ids))
			}
			for i, id := range ids {
				if id != tc.clipID {
					t.Errorf("replay %d clip = %q, want %q", i, id, tc.clipID)
				}
			}
		})
	}
}

// The clip reaches the runner from a map request as well, which is how the
// HTTP sweep start hands the request over.
func TestRunner_StartFromMapReadsClipID(t *testing.T) {
	backend := newClipRecordingBackend()
	r := newQuietRunner(backend)

	err := r.Start(context.Background(), map[string]interface{}{
		"data_source": "capture",
		"pcap_file":   "test.pcap",
		"clip_id":     "clip-from-map",
		"params": []interface{}{map[string]interface{}{
			"name": "noise_relative", "type": "float64", "values": []interface{}{0.01},
		}},
		"iterations":  1,
		"interval":    "10ms",
		"settle_time": "10ms",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForRunnerStatus(t, r, 10*time.Second, SweepStatusComplete, SweepStatusError)

	ids := backend.clipIDs()
	if len(ids) != 1 || ids[0] != "clip-from-map" {
		t.Fatalf("replay clips = %v, want [clip-from-map]", ids)
	}
}

// An auto-tune for a clip passes the clip to every round's sweep, whatever
// its objective.
func TestAutoTuner_RoundSweepsCarryTheClip(t *testing.T) {
	backend := newClipRecordingBackend()
	at := newQuietAutoTuner(newQuietRunner(backend))

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      2,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
		Objective:      "acceptance",
		DataSource:     "capture",
		PCAPFile:       "test.pcap",
		ClipID:         "clip-auto",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("status = %q (%s), want complete", state.Status, state.Error)
	}

	ids := backend.clipIDs()
	if len(ids) < 4 {
		t.Fatalf("replays = %d, want two rounds of two", len(ids))
	}
	for i, id := range ids {
		if id != "clip-auto" {
			t.Errorf("replay %d clip = %q, want clip-auto", i, id)
		}
	}
}
