package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
)

var _ sweep.SweepRoundCheckpointer = (*SweepStore)(nil)

// addCheckpointColumns adds the checkpoint columns the production schema has
// and setupTestSweepDB omits. Recovery reads them.
func addCheckpointColumns(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, col := range []string{
		"checkpoint_round INTEGER",
		"checkpoint_bounds TEXT",
		"checkpoint_results TEXT",
		"checkpoint_request TEXT",
	} {
		if _, err := db.Exec("ALTER TABLE lidar_tuning_sweeps ADD COLUMN " + col); err != nil {
			t.Fatalf("alter table: %v", err)
		}
	}
}

// A running sweep with a checkpoint is suspended by recovery, keeping the
// checkpoint for Resume; one without is failed with the restart message.
func TestSweepStore_RecoverOrphanedSweeps_SuspendsCheckpointed(t *testing.T) {
	db := setupTestSweepDB(t)
	addCheckpointColumns(t, db)
	defer db.Close()
	store := NewSweepStore(db)

	for _, rec := range []SweepRecord{
		{SweepID: "cp", SensorID: "s1", Mode: "auto", Status: "running", Request: json.RawMessage(`{}`), StartedAt: time.Now().UTC()},
		{SweepID: "plain", SensorID: "s1", Mode: "manual", Status: "running", Request: json.RawMessage(`{}`), StartedAt: time.Now().UTC()},
	} {
		if err := store.InsertSweep(rec); err != nil {
			t.Fatalf("InsertSweep %s: %v", rec.SweepID, err)
		}
	}
	if err := store.SaveSweepRoundCheckpoint("cp", 2,
		json.RawMessage(`{"p":[0.1,0.2]}`), json.RawMessage(`[]`), json.RawMessage(`{"max_rounds":3}`)); err != nil {
		t.Fatalf("SaveSweepRoundCheckpoint: %v", err)
	}
	// The round checkpoint leaves the sweep running.
	if got, _ := store.GetSweep("cp"); got.Status != "running" {
		t.Fatalf("status after round checkpoint = %q, want running", got.Status)
	}

	affected, err := store.RecoverOrphanedSweeps()
	if err != nil {
		t.Fatalf("RecoverOrphanedSweeps: %v", err)
	}
	if affected != 2 {
		t.Fatalf("affected = %d, want 2", affected)
	}

	cp, _ := store.GetSweep("cp")
	if cp.Status != "suspended" || cp.Error != "" || cp.CompletedAt != nil {
		t.Errorf("checkpointed sweep = status %q, error %q, completed %v; want suspended, no error, incomplete",
			cp.Status, cp.Error, cp.CompletedAt)
	}
	round, bounds, _, request, err := store.LoadSweepCheckpoint("cp")
	if err != nil {
		t.Fatalf("LoadSweepCheckpoint after recovery: %v", err)
	}
	if round != 2 || string(bounds) != `{"p":[0.1,0.2]}` || string(request) != `{"max_rounds":3}` {
		t.Errorf("checkpoint = round %d bounds %s request %s", round, bounds, request)
	}
	if id, r, err := store.GetSuspendedSweep(); err != nil || id != "cp" || r != 2 {
		t.Errorf("GetSuspendedSweep = %q, %d, %v; want cp, 2", id, r, err)
	}

	plain, _ := store.GetSweep("plain")
	if plain.Status != "failed" || plain.Error != "incomplete: server restarted" || plain.CompletedAt == nil {
		t.Errorf("sweep without checkpoint = status %q, error %q, completed %v; want failed with restart message",
			plain.Status, plain.Error, plain.CompletedAt)
	}
}

// A round checkpoint never revives a sweep that has already finished.
func TestSweepStore_SaveSweepRoundCheckpoint_IgnoresFinished(t *testing.T) {
	db := setupTestSweepDB(t)
	addCheckpointColumns(t, db)
	defer db.Close()
	store := NewSweepStore(db)

	if err := store.InsertSweep(SweepRecord{SweepID: "done", SensorID: "s1", Mode: "auto", Status: "completed",
		Request: json.RawMessage(`{}`), StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSweepRoundCheckpoint("done", 2, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := store.LoadSweepCheckpoint("done"); err == nil {
		t.Fatal("a completed sweep gained a loadable checkpoint")
	}
	got, _ := store.GetSweep("done")
	if got.Status != "completed" {
		t.Fatalf("status = %q, want completed", got.Status)
	}
}

// restartBackend is a minimal in-process sweep backend. When stallAt is
// positive, the stallAt-th SetTuningParams call blocks until release is
// closed: the tuner stops there as if the process had died. accept is the
// acceptance fraction every sample reports, which is the whole score under
// the acceptance objective; startErr fails every replay start.
type restartBackend struct {
	calls    atomic.Int32
	stallAt  int32
	stalled  chan struct{}
	release  chan struct{}
	once     sync.Once
	accept   float64
	startErr error
}

func newRestartBackend(stallAt int32) *restartBackend {
	return &restartBackend{stallAt: stallAt, stalled: make(chan struct{}), release: make(chan struct{}), accept: 0.8}
}

func (b *restartBackend) SensorID() string       { return "s1" }
func (b *restartBackend) FetchBuckets() []string { return []string{"1", "2", "4"} }
func (b *restartBackend) FetchAcceptanceMetrics() (map[string]interface{}, error) {
	a := b.accept * 100
	return map[string]interface{}{
		"BucketsMeters":   []interface{}{1.0, 2.0, 4.0},
		"AcceptCounts":    []interface{}{a, a, a},
		"RejectCounts":    []interface{}{100 - a, 100 - a, 100 - a},
		"Totals":          []interface{}{100.0, 100.0, 100.0},
		"AcceptanceRates": []interface{}{b.accept, b.accept, b.accept},
	}, nil
}
func (b *restartBackend) ResetAcceptance() error { return nil }
func (b *restartBackend) FetchGridStatus() (map[string]interface{}, error) {
	return map[string]interface{}{"background_count": 42.0}, nil
}
func (b *restartBackend) ResetGrid() error                  { return nil }
func (b *restartBackend) WaitForGridSettle(_ time.Duration) {}
func (b *restartBackend) FetchTrackingMetrics() (map[string]interface{}, error) {
	return map[string]interface{}{"active_tracks": 3.0}, nil
}
func (b *restartBackend) SetTuningParams(map[string]interface{}) error {
	if n := b.calls.Add(1); b.stallAt > 0 && n == b.stallAt {
		b.once.Do(func() { close(b.stalled) })
		<-b.release
	}
	return nil
}
func (b *restartBackend) StartPCAPReplayWithConfig(sweep.PCAPReplayConfig) error { return b.startErr }
func (b *restartBackend) StopPCAPReplay() error                                  { return nil }
func (b *restartBackend) WaitForPCAPComplete(time.Duration) error                { return nil }
func (b *restartBackend) GetLastRunID() string                                   { return "" }

func quietTuner(backend sweep.SweepBackend, store *SweepStore) *sweep.AutoTuner {
	quiet := log.New(io.Discard, "", 0)
	runner := sweep.NewRunner(backend)
	runner.SetLogger(quiet)
	at := sweep.NewAutoTuner(runner)
	at.SetLogger(quiet)
	at.SetPersister(store)
	return at
}

func restartRequest() sweep.AutoTuneRequest {
	return sweep.AutoTuneRequest{
		Params:         []sweep.SweepParam{{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05}},
		MaxRounds:      3,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		Interval:       "1ms",
		SettleTime:     "1ms",
		Objective:      "acceptance",
		DataSource:     "capture",
		PCAPFile:       "site.pcap",
	}
}

// startAndKill runs an auto-tune on first until it stalls on its third
// combination (round 2's first), then recovers the store as a restart would.
// The stalled goroutine stands in for the dead process and is never released.
func startAndKill(t *testing.T, store *SweepStore, first *restartBackend) string {
	t.Helper()
	before := quietTuner(first, store)
	if err := before.Start(context.Background(), restartRequest()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-first.stalled:
	case <-time.After(30 * time.Second):
		t.Fatal("the first tuner never reached round 2")
	}
	sweepID := before.GetSweepID()
	if got, err := store.GetSweep(sweepID); err != nil || got.Status != "running" {
		t.Fatalf("before restart: %+v, %v; want running", got, err)
	}
	if _, err := store.RecoverOrphanedSweeps(); err != nil {
		t.Fatalf("RecoverOrphanedSweeps: %v", err)
	}
	if got, _ := store.GetSweep(sweepID); got.Status != "suspended" {
		t.Fatalf("status after recovery = %q, want suspended", got.Status)
	}
	return sweepID
}

func waitForTune(t *testing.T, at *sweep.AutoTuner, statuses ...sweep.SweepStatus) sweep.AutoTuneState {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		state := at.GetAutoTuneState()
		for _, s := range statuses {
			if state.Status == s {
				return state
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	state := at.GetAutoTuneState()
	t.Fatalf("status = %q (%s), want one of %v", state.Status, state.Error, statuses)
	return state
}

func restartDB(t *testing.T) *SweepStore {
	t.Helper()
	db := setupTestSweepDB(t)
	addCheckpointColumns(t, db)
	db.SetMaxOpenConns(1) // one in-memory database for every goroutine
	t.Cleanup(func() { db.Close() })
	return NewSweepStore(db)
}

// The whole path a restart takes: an auto-tune checkpoints after round 1,
// the process stops during round 2, recovery suspends the sweep, and a fresh
// tuner on the same database resumes it from round 2 to completion with
// round 1's results and summary carried over.
func TestSweepStore_RestartedAutoTuneResumes(t *testing.T) {
	store := restartDB(t)
	sweepID := startAndKill(t, store, newRestartBackend(3))

	after := quietTuner(newRestartBackend(0), store)
	if err := after.Resume(context.Background(), sweepID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	state := waitForTune(t, after, sweep.SweepStatusComplete, sweep.SweepStatusError)
	if state.Status != sweep.SweepStatusComplete {
		t.Fatalf("resumed status = %q (%s), want complete", state.Status, state.Error)
	}
	if len(state.RoundResults) != 3 {
		t.Errorf("round summaries = %d, want 3 (round 1 carried over, rounds 2 and 3 run)", len(state.RoundResults))
	}
	for i, rs := range state.RoundResults {
		if rs.Round != i+1 {
			t.Errorf("round summary %d is round %d, want %d", i, rs.Round, i+1)
		}
	}
	if len(state.Results) != 6 {
		t.Errorf("results = %d, want 6 (round 1 carried over, rounds 2 and 3 run)", len(state.Results))
	}

	final, err := store.GetSweep(sweepID)
	if err != nil {
		t.Fatalf("GetSweep: %v", err)
	}
	if final.Status != "completed" {
		t.Errorf("stored status = %q, want completed", final.Status)
	}
}

// When the round before the restart holds the best result, the resumed tune
// recommends it rather than the best of the rounds it ran itself.
func TestSweepStore_RestartedAutoTuneKeepsEarlierBest(t *testing.T) {
	store := restartDB(t)
	first := newRestartBackend(3)
	first.accept = 0.82
	sweepID := startAndKill(t, store, first)

	later := newRestartBackend(0)
	later.accept = 0.80
	after := quietTuner(later, store)
	if err := after.Resume(context.Background(), sweepID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	state := waitForTune(t, after, sweep.SweepStatusComplete, sweep.SweepStatusError)
	if state.Status != sweep.SweepStatusComplete {
		t.Fatalf("resumed status = %q (%s), want complete", state.Status, state.Error)
	}
	score, ok := state.Recommendation["score"].(float64)
	if !ok {
		t.Fatalf("recommendation has no score: %v", state.Recommendation)
	}
	if score < state.RoundResults[0].BestScore-1e-9 || state.RoundResults[0].BestScore <= state.RoundResults[1].BestScore {
		t.Fatalf("recommended score %.4f; round best scores %.4f, %.4f, %.4f: want round 1's",
			score, state.RoundResults[0].BestScore, state.RoundResults[1].BestScore, state.RoundResults[2].BestScore)
	}
}

// A resume whose first round cannot run leaves the sweep suspended on its
// checkpoint, so a later resume still works.
func TestSweepStore_FailedResumeStaysResumable(t *testing.T) {
	store := restartDB(t)
	sweepID := startAndKill(t, store, newRestartBackend(3))

	busy := newRestartBackend(0)
	busy.startErr = fmt.Errorf("%w: a replay was still running", sweep.ErrReplaySlotBusy)
	failed := quietTuner(busy, store)
	if err := failed.Resume(context.Background(), sweepID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	state := waitForTune(t, failed, sweep.SweepStatusSuspended, sweep.SweepStatusError, sweep.SweepStatusComplete)
	if state.Status != sweep.SweepStatusSuspended || state.Error == "" {
		t.Fatalf("after a failed resume: status %q error %q; want suspended with the error", state.Status, state.Error)
	}
	if got, _ := store.GetSweep(sweepID); got.Status != "suspended" {
		t.Fatalf("stored status after a failed resume = %q, want suspended", got.Status)
	}
	if round, _, _, _, err := store.LoadSweepCheckpoint(sweepID); err != nil || round != 2 {
		t.Fatalf("checkpoint after a failed resume: round %d, %v; want round 2", round, err)
	}

	again := quietTuner(newRestartBackend(0), store)
	if err := again.Resume(context.Background(), sweepID); err != nil {
		t.Fatalf("second Resume: %v", err)
	}
	if state := waitForTune(t, again, sweep.SweepStatusComplete, sweep.SweepStatusError); state.Status != sweep.SweepStatusComplete {
		t.Fatalf("second resume: %q (%s), want complete", state.Status, state.Error)
	}
}
