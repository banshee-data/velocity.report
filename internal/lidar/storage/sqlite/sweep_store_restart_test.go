package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
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
// closed: the tuner stops there as if the process had died.
type restartBackend struct {
	calls   atomic.Int32
	stallAt int32
	stalled chan struct{}
	release chan struct{}
	once    sync.Once
}

func newRestartBackend(stallAt int32) *restartBackend {
	return &restartBackend{stallAt: stallAt, stalled: make(chan struct{}), release: make(chan struct{})}
}

func (b *restartBackend) SensorID() string       { return "s1" }
func (b *restartBackend) FetchBuckets() []string { return []string{"1", "2", "4"} }
func (b *restartBackend) FetchAcceptanceMetrics() (map[string]interface{}, error) {
	return map[string]interface{}{
		"BucketsMeters":   []interface{}{1.0, 2.0, 4.0},
		"AcceptCounts":    []interface{}{10.0, 20.0, 30.0},
		"RejectCounts":    []interface{}{2.0, 3.0, 4.0},
		"Totals":          []interface{}{12.0, 23.0, 34.0},
		"AcceptanceRates": []interface{}{0.83, 0.87, 0.88},
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
func (b *restartBackend) StartPCAPReplayWithConfig(sweep.PCAPReplayConfig) error { return nil }
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

// The whole path a restart takes: an auto-tune checkpoints after round 1,
// the process stops during round 2, recovery suspends the sweep, and a fresh
// tuner on the same database resumes it from round 2 to completion with
// round 1's results carried over.
func TestSweepStore_RestartedAutoTuneResumes(t *testing.T) {
	db := setupTestSweepDB(t)
	addCheckpointColumns(t, db)
	db.SetMaxOpenConns(1) // one in-memory database for every goroutine
	defer db.Close()
	store := NewSweepStore(db)

	req := sweep.AutoTuneRequest{
		Params:         []sweep.SweepParam{{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05}},
		MaxRounds:      3,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		Interval:       "1ms",
		SettleTime:     "1ms",
		Objective:      "acceptance",
	}

	// First process: two combinations in round 1, then stall on round 2's
	// first. The stalled goroutine stands in for the dead process and is
	// never released.
	first := newRestartBackend(3)
	before := quietTuner(first, store)
	if err := before.Start(context.Background(), req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-first.stalled:
	case <-time.After(30 * time.Second):
		t.Fatal("the first tuner never reached round 2")
	}
	sweepID := before.GetSweepID()

	got, err := store.GetSweep(sweepID)
	if err != nil {
		t.Fatalf("GetSweep: %v", err)
	}
	if got.Status != "running" {
		t.Fatalf("status before restart = %q, want running", got.Status)
	}

	// Restart.
	if _, err := store.RecoverOrphanedSweeps(); err != nil {
		t.Fatalf("RecoverOrphanedSweeps: %v", err)
	}
	if got, _ := store.GetSweep(sweepID); got.Status != "suspended" {
		t.Fatalf("status after recovery = %q, want suspended", got.Status)
	}

	// Second process: a fresh tuner with no memory of the first.
	after := quietTuner(newRestartBackend(0), store)
	if err := after.Resume(context.Background(), sweepID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var state sweep.AutoTuneState
	for time.Now().Before(deadline) {
		state = after.GetAutoTuneState()
		if state.Status == sweep.SweepStatusComplete || state.Status == sweep.SweepStatusError {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if state.Status != sweep.SweepStatusComplete {
		t.Fatalf("resumed status = %q (%s), want complete", state.Status, state.Error)
	}
	if len(state.RoundResults) != 2 {
		t.Errorf("resumed rounds = %d, want 2 (rounds 2 and 3)", len(state.RoundResults))
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
