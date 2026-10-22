package sweep

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// slotCountingBackend records how often the runner asks to start and stop a
// replay, and fails every start with startErr.
type slotCountingBackend struct {
	*mockBackend
	startErr error
	starts   atomic.Int32
	stops    atomic.Int32
}

func (b *slotCountingBackend) StartPCAPReplayWithConfig(PCAPReplayConfig) error {
	b.starts.Add(1)
	return b.startErr
}

func (b *slotCountingBackend) StopPCAPReplay() error {
	b.stops.Add(1)
	return nil
}

func replaySlotSweepRequest() SweepRequest {
	return SweepRequest{
		DataSource: "capture",
		PCAPFile:   "test.pcap",
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Values: []interface{}{0.01, 0.02, 0.03}},
		},
		Iterations: 1,
		Interval:   "10ms",
		SettleTime: "10ms",
	}
}

// A replay slot that stays busy is the pipeline's condition, not one
// combination's, so the sweep stops with an error naming it rather than
// skipping every combination after its own wait and completing empty.
func TestRunner_ReplaySlotBusyStopsSweep(t *testing.T) {
	backend := &slotCountingBackend{
		mockBackend: runnerMockBackend(),
		startErr:    fmt.Errorf("%w: a replay was still running", ErrReplaySlotBusy),
	}
	r := newQuietRunner(backend)

	if err := r.Start(context.Background(), replaySlotSweepRequest()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	state := waitForRunnerStatus(t, r, 10*time.Second, SweepStatusComplete, SweepStatusError)

	if state.Status != SweepStatusError {
		t.Fatalf("status = %q, want error", state.Status)
	}
	if !strings.Contains(state.Error, "combination 1/3") || !strings.Contains(state.Error, ErrReplaySlotBusy.Error()) {
		t.Errorf("error = %q, want it to name combination 1/3 and the busy slot", state.Error)
	}
	if got := backend.starts.Load(); got != 1 {
		t.Errorf("start attempts = %d, want 1: later combinations must not wait again", got)
	}
	// The replay holding the slot belongs to someone else.
	if got := backend.stops.Load(); got != 0 {
		t.Errorf("StopPCAPReplay calls = %d, want 0: the sweep must not stop a replay it did not start", got)
	}
	if len(state.Results) != 0 {
		t.Errorf("results = %d, want 0", len(state.Results))
	}
}

// Any other start failure is specific enough to the combination that the
// sweep still skips it and carries on.
func TestRunner_OtherStartErrorSkipsCombination(t *testing.T) {
	backend := &slotCountingBackend{
		mockBackend: runnerMockBackend(),
		startErr:    errors.New("pcap file not found"),
	}
	r := newQuietRunner(backend)

	if err := r.Start(context.Background(), replaySlotSweepRequest()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	state := waitForRunnerStatus(t, r, 10*time.Second, SweepStatusComplete, SweepStatusError)

	if state.Status != SweepStatusComplete {
		t.Fatalf("status = %q, want complete", state.Status)
	}
	if got := backend.starts.Load(); got != 3 {
		t.Errorf("start attempts = %d, want 3", got)
	}
	if len(state.Warnings) != 3 {
		t.Errorf("warnings = %v, want one per skipped combination", state.Warnings)
	}
}
