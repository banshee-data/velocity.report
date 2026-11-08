package sweep

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestAutoCheckpoint_RoundTripsResultsAndRounds(t *testing.T) {
	results := []ComboResult{{RunID: "r1"}, {RunID: "r2"}}
	rounds := []RoundSummary{{Round: 1, BestScore: 0.82, TopK: []ScoredResult{{Score: 0.82}}}}
	data, err := encodeAutoCheckpoint(results, rounds)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := decodeAutoCheckpoint(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(cp.Results) != 2 || cp.Results[1].RunID != "r2" || len(cp.RoundResults) != 1 || cp.RoundResults[0].BestScore != 0.82 {
		t.Fatalf("decoded %+v", cp)
	}
}

// A checkpoint saved before round summaries were kept is a bare results
// array; it still resumes, with no rounds.
func TestAutoCheckpoint_DecodesLegacyResultsArray(t *testing.T) {
	cp, err := decodeAutoCheckpoint(json.RawMessage(` [{"run_id":"r1"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cp.Results) != 1 || cp.Results[0].RunID != "r1" || cp.RoundResults != nil {
		t.Fatalf("decoded %+v", cp)
	}
	if cp, err := decodeAutoCheckpoint(nil); err != nil || cp.Results != nil {
		t.Fatalf("empty checkpoint: %+v, %v", cp, err)
	}
	if _, err := decodeAutoCheckpoint(json.RawMessage(`{"results":`)); err == nil {
		t.Fatal("truncated checkpoint decoded")
	}
}

func TestBestOfRounds(t *testing.T) {
	if bestOfRounds(nil) != nil {
		t.Fatal("best of no rounds")
	}
	best := bestOfRounds([]RoundSummary{
		{Round: 1, TopK: []ScoredResult{{Score: 0.82, ComboResult: ComboResult{RunID: "early"}}}},
		{Round: 2},
		{Round: 3, TopK: []ScoredResult{{Score: 0.80}}},
		{Round: 4, TopK: []ScoredResult{{Score: 0.82, ComboResult: ComboResult{RunID: "tie"}}}},
	})
	if best == nil || best.RunID != "early" {
		t.Fatalf("best = %+v, want round 1's (an earlier round wins a tie)", best)
	}
}

// Resume refuses while the runner is busy with another sweep and leaves the
// checkpoint untouched, rather than failing the resumed tune's first round.
func TestAutoTuner_ResumeRefusedWhileRunnerBusy(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	backend := runnerMockBackend()
	backend.SetTuningParamsFn = func(map[string]interface{}) error {
		<-release
		return nil
	}
	runner := newQuietRunner(backend)
	if err := runner.Start(context.Background(), replaySlotSweepRequest()); err != nil {
		t.Fatalf("runner Start: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for runner.GetSweepState().Status != SweepStatusRunning && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	at := newQuietAutoTuner(runner)
	persister := &mockPersister{}
	at.SetPersister(persister)
	err := at.Resume(context.Background(), "sweep-1")
	if !errors.Is(err, ErrSweepAlreadyRunning) {
		t.Fatalf("Resume = %v, want ErrSweepAlreadyRunning", err)
	}
	persister.mu.Lock()
	defer persister.mu.Unlock()
	if persister.completeCalled {
		t.Fatal("a refused resume wrote a final status")
	}
}
