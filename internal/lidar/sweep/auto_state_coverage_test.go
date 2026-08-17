package sweep

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// checkpointPersister returns scripted checkpoint data so Resume's
// deserialisation branches can each be driven independently.
type checkpointPersister struct {
	round    int
	bounds   json.RawMessage
	results  json.RawMessage
	request  json.RawMessage
	loadErr  error
	saveErr  error
	saveSeen bool
}

func (p *checkpointPersister) SaveSweepStart(string, string, string, json.RawMessage, time.Time, string, string) error {
	return nil
}

func (p *checkpointPersister) SaveSweepComplete(string, string, json.RawMessage, json.RawMessage, json.RawMessage, time.Time, string, json.RawMessage, json.RawMessage, json.RawMessage, string, string) error {
	return nil
}

func (p *checkpointPersister) SaveSweepCheckpoint(string, int, json.RawMessage, json.RawMessage, json.RawMessage) error {
	p.saveSeen = true
	return p.saveErr
}

func (p *checkpointPersister) LoadSweepCheckpoint(string) (int, json.RawMessage, json.RawMessage, json.RawMessage, error) {
	return p.round, p.bounds, p.results, p.request, p.loadErr
}

func (p *checkpointPersister) GetSuspendedSweep() (string, int, error) { return "", 0, nil }

// suspendedTuner returns an auto-tuner parked in the suspended state with the
// given persister and sweep ID.
func suspendedTuner(t *testing.T, p SweepPersister, sweepID string) *AutoTuner {
	t.Helper()
	at := NewAutoTuner(NewRunner(&mockBackend{}))
	if p != nil {
		at.SetPersister(p)
	}
	at.mu.Lock()
	at.state.Status = SweepStatusSuspended
	at.sweepID = sweepID
	at.mu.Unlock()
	return at
}

func TestSuspendRejectsSweepThatIsNotRunning(t *testing.T) {
	at := NewAutoTuner(NewRunner(&mockBackend{}))

	// Nothing is running, so there is no checkpoint to take.
	err := at.Suspend()
	if err == nil {
		t.Fatal("Suspend on an idle tuner succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "cannot suspend") {
		t.Errorf("error = %v, want it to explain the sweep is not running", err)
	}
}

func TestResumeRejectsRunningSweep(t *testing.T) {
	at := NewAutoTuner(NewRunner(&mockBackend{}))
	at.mu.Lock()
	at.state.Status = SweepStatusRunning
	at.mu.Unlock()

	if err := at.Resume(context.Background(), ""); !errors.Is(err, ErrSweepAlreadyRunning) {
		t.Fatalf("Resume while running = %v, want ErrSweepAlreadyRunning", err)
	}
}

func TestResumeWithoutSweepID(t *testing.T) {
	// No explicit ID and nothing suspended in memory: there is nothing to
	// resume, and that must be said plainly rather than loading a random
	// checkpoint.
	at := suspendedTuner(t, &checkpointPersister{}, "")

	err := at.Resume(context.Background(), "")
	if err == nil {
		t.Fatal("Resume without a sweep ID succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "no suspended sweep to resume") {
		t.Errorf("error = %v, want it to report nothing to resume", err)
	}
}

func TestResumeWithoutPersister(t *testing.T) {
	// Checkpoints live in the database; with no persister there is no way to
	// recover the state a resume needs.
	at := suspendedTuner(t, nil, "sweep-1")

	err := at.Resume(context.Background(), "")
	if err == nil {
		t.Fatal("Resume without a persister succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "no persister configured") {
		t.Errorf("error = %v, want it to name the missing persister", err)
	}
}

func TestResumeReportsCheckpointLoadFailure(t *testing.T) {
	at := suspendedTuner(t, &checkpointPersister{loadErr: errors.New("database gone")}, "sweep-1")

	err := at.Resume(context.Background(), "sweep-1")
	if err == nil {
		t.Fatal("Resume with a failing checkpoint load succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "failed to load checkpoint") {
		t.Errorf("error = %v, want it to name the checkpoint load", err)
	}
}

func TestResumeRejectsMalformedCheckpointRequest(t *testing.T) {
	at := suspendedTuner(t, &checkpointPersister{request: json.RawMessage("{not json")}, "sweep-1")

	err := at.Resume(context.Background(), "sweep-1")
	if err == nil {
		t.Fatal("Resume with a malformed checkpoint request succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "unmarshal checkpoint request") {
		t.Errorf("error = %v, want it to name the request", err)
	}
}

func TestResumeRequiresARequestSomewhere(t *testing.T) {
	// An empty checkpoint request and no remembered lastRequest means the
	// sweep parameters are simply unknown.
	at := suspendedTuner(t, &checkpointPersister{}, "sweep-1")

	err := at.Resume(context.Background(), "sweep-1")
	if err == nil {
		t.Fatal("Resume without any request succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "no request found in checkpoint") {
		t.Errorf("error = %v, want it to report the missing request", err)
	}
}

func TestResumeRejectsMalformedCheckpointResults(t *testing.T) {
	at := suspendedTuner(t, &checkpointPersister{
		request: json.RawMessage(`{"objective":"acceptance","max_rounds":1}`),
		results: json.RawMessage("[not json"),
	}, "sweep-1")

	err := at.Resume(context.Background(), "sweep-1")
	if err == nil {
		t.Fatal("Resume with malformed checkpoint results succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "unmarshal checkpoint results") {
		t.Errorf("error = %v, want it to name the results", err)
	}
}

func TestResumeRejectsMalformedCheckpointBounds(t *testing.T) {
	at := suspendedTuner(t, &checkpointPersister{
		request: json.RawMessage(`{"objective":"acceptance","max_rounds":1}`),
		results: json.RawMessage("[]"),
		bounds:  json.RawMessage("{not json"),
	}, "sweep-1")

	err := at.Resume(context.Background(), "sweep-1")
	if err == nil {
		t.Fatal("Resume with malformed checkpoint bounds succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "unmarshal checkpoint bounds") {
		t.Errorf("error = %v, want it to name the bounds", err)
	}
}

func TestResumeFallsBackToLastRequest(t *testing.T) {
	// A checkpoint written before the request was persisted still resumes if
	// the tuner remembers the original request in memory.
	at := suspendedTuner(t, &checkpointPersister{results: json.RawMessage("[]")}, "sweep-1")
	at.mu.Lock()
	at.lastRequest = &AutoTuneRequest{Objective: "acceptance", MaxRounds: 1}
	at.mu.Unlock()

	err := at.Resume(context.Background(), "sweep-1")
	// The backend is a stub, so the resumed run fails — but it must get past
	// the "no request found" guard, which is what this covers.
	if err != nil && strings.Contains(err.Error(), "no request found in checkpoint") {
		t.Errorf("error = %v, want the in-memory request to be used", err)
	}
}

func TestGetSuspendedSweepID(t *testing.T) {
	at := suspendedTuner(t, &checkpointPersister{}, "sweep-42")
	if got := at.GetSuspendedSweepID(); got != "sweep-42" {
		t.Errorf("GetSuspendedSweepID() = %q, want %q", got, "sweep-42")
	}

	// Only a suspended sweep has an ID worth resuming.
	at.mu.Lock()
	at.state.Status = SweepStatusIdle
	at.mu.Unlock()
	if got := at.GetSuspendedSweepID(); got != "" {
		t.Errorf("GetSuspendedSweepID() on an idle tuner = %q, want empty", got)
	}
}
