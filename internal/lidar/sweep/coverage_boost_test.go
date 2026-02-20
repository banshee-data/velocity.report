package sweep

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"testing"
	"time"
)

// --- scenePCAPStart / scenePCAPDuration (hint_notifications.go) ---

func TestScenePCAPStart_NilScene(t *testing.T) {
	if got := scenePCAPStart(nil); got != 0 {
		t.Errorf("expected 0, got %f", got)
	}
}

func TestScenePCAPStart_NilPtr(t *testing.T) {
	scene := &HINTScene{}
	if got := scenePCAPStart(scene); got != 0 {
		t.Errorf("expected 0, got %f", got)
	}
}

func TestScenePCAPStart_WithValue(t *testing.T) {
	v := 42.5
	scene := &HINTScene{PCAPStartSecs: &v}
	if got := scenePCAPStart(scene); got != 42.5 {
		t.Errorf("expected 42.5, got %f", got)
	}
}

func TestScenePCAPDuration_NilScene(t *testing.T) {
	if got := scenePCAPDuration(nil); got != -1 {
		t.Errorf("expected -1, got %f", got)
	}
}

func TestScenePCAPDuration_NilPtr(t *testing.T) {
	scene := &HINTScene{}
	if got := scenePCAPDuration(scene); got != -1 {
		t.Errorf("expected -1, got %f", got)
	}
}

func TestScenePCAPDuration_WithValue(t *testing.T) {
	v := 120.0
	scene := &HINTScene{PCAPDurationSecs: &v}
	if got := scenePCAPDuration(scene); got != 120.0 {
		t.Errorf("expected 120.0, got %f", got)
	}
}

// --- defaultHINTParams (hint.go) ---

func TestDefaultHINTParams_ForegroundOnly(t *testing.T) {
	params := defaultHINTParams(false)
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(params))
	}
	if params[0].Name != "foreground_min_cluster_points" {
		t.Errorf("expected foreground_min_cluster_points, got %s", params[0].Name)
	}
	if params[1].Name != "foreground_dbscan_eps" {
		t.Errorf("expected foreground_dbscan_eps, got %s", params[1].Name)
	}
}

func TestDefaultHINTParams_WithBackground(t *testing.T) {
	params := defaultHINTParams(true)
	if len(params) != 6 {
		t.Fatalf("expected 6 params, got %d", len(params))
	}
	// Check background params are included
	names := map[string]bool{}
	for _, p := range params {
		names[p.Name] = true
	}
	for _, expected := range []string{"noise_relative", "closeness_multiplier", "background_update_fraction", "safety_margin_meters"} {
		if !names[expected] {
			t.Errorf("missing expected param %s", expected)
		}
	}
}

// --- SetGroundTruthScorerDetailed (auto.go) ---

func TestSetGroundTruthScorerDetailed(t *testing.T) {
	at := NewAutoTuner(nil)
	called := false
	scorer := func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, *ScoreComponents, error) {
		called = true
		return 0.95, nil, nil
	}
	at.SetGroundTruthScorerDetailed(scorer)

	if at.groundTruthScorerDetailed == nil {
		t.Fatal("scorer not set")
	}
	score, _, err := at.groundTruthScorerDetailed("s", "r", DefaultGroundTruthWeights())
	if err != nil || score != 0.95 || !called {
		t.Errorf("unexpected result: score=%f, called=%v, err=%v", score, called, err)
	}
}

// --- GetSuspendedSweepID (auto.go) ---

func TestGetSuspendedSweepID_NotSuspended(t *testing.T) {
	at := NewAutoTuner(nil)
	if id := at.GetSuspendedSweepID(); id != "" {
		t.Errorf("expected empty, got %q", id)
	}
}

func TestGetSuspendedSweepID_WhenSuspended(t *testing.T) {
	at := NewAutoTuner(nil)
	at.mu.Lock()
	at.state.Status = SweepStatusSuspended
	at.sweepID = "test-sweep-123"
	at.mu.Unlock()

	if id := at.GetSuspendedSweepID(); id != "test-sweep-123" {
		t.Errorf("expected test-sweep-123, got %q", id)
	}
}

// --- Resume error paths (auto.go) ---

func TestResume_AlreadyRunning(t *testing.T) {
	at := NewAutoTuner(nil)
	at.SetLogger(log.New(io.Discard, "", 0))
	at.mu.Lock()
	at.state.Status = SweepStatusRunning
	at.mu.Unlock()

	err := at.Resume(context.Background(), "some-id")
	if err != ErrSweepAlreadyRunning {
		t.Errorf("expected ErrSweepAlreadyRunning, got %v", err)
	}
}

func TestResume_EmptySweepID(t *testing.T) {
	at := NewAutoTuner(nil)
	at.SetLogger(log.New(io.Discard, "", 0))

	err := at.Resume(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty sweep ID")
	}
}

func TestResume_NoPersister(t *testing.T) {
	at := NewAutoTuner(nil)
	at.SetLogger(log.New(io.Discard, "", 0))

	err := at.Resume(context.Background(), "sweep-123")
	if err == nil || err.Error() != "cannot resume: no persister configured" {
		t.Errorf("expected no persister error, got %v", err)
	}
}

// resumeTestPersister implements SweepPersister for testing Resume error paths.
type resumeTestPersister struct {
	loadErr     error
	checkpoint  int
	boundsData  json.RawMessage
	resultsData json.RawMessage
	requestData json.RawMessage
}

func (m *resumeTestPersister) SaveSweepStart(sweepID, sensorID, mode string, request json.RawMessage, startedAt time.Time, objectiveName, objectiveVersion string) error {
	return nil
}
func (m *resumeTestPersister) SaveSweepComplete(sweepID, status string, results, recommendation, roundResults json.RawMessage, completedAt time.Time, errMsg string, scoreComponents, recommendationExplanation, labelProvenanceSummary json.RawMessage, transformPipelineName, transformPipelineVersion string) error {
	return nil
}
func (m *resumeTestPersister) SaveSweepCheckpoint(sweepID string, round int, bounds, results, request json.RawMessage) error {
	return nil
}
func (m *resumeTestPersister) LoadSweepCheckpoint(sweepID string) (int, json.RawMessage, json.RawMessage, json.RawMessage, error) {
	return m.checkpoint, m.boundsData, m.resultsData, m.requestData, m.loadErr
}
func (m *resumeTestPersister) GetSuspendedSweep() (string, int, error) {
	return "", 0, nil
}

func TestResume_CheckpointLoadError(t *testing.T) {
	at := NewAutoTuner(nil)
	at.SetLogger(log.New(io.Discard, "", 0))
	at.SetPersister(&resumeTestPersister{loadErr: fmt.Errorf("db broken")})

	err := at.Resume(context.Background(), "sweep-123")
	if err == nil || err.Error() != "failed to load checkpoint: db broken" {
		t.Errorf("expected load checkpoint error, got %v", err)
	}
}

func TestResume_InvalidRequestJSON(t *testing.T) {
	at := NewAutoTuner(nil)
	at.SetLogger(log.New(io.Discard, "", 0))
	at.SetPersister(&resumeTestPersister{
		checkpoint:  1,
		requestData: json.RawMessage(`{invalid json`),
		boundsData:  json.RawMessage(`[]`),
		resultsData: json.RawMessage(`[]`),
	})

	err := at.Resume(context.Background(), "sweep-123")
	if err == nil {
		t.Fatal("expected error for invalid request JSON")
	}
}

func TestResume_NoRequestFound(t *testing.T) {
	at := NewAutoTuner(nil)
	at.SetLogger(log.New(io.Discard, "", 0))
	at.SetPersister(&resumeTestPersister{
		checkpoint:  1,
		requestData: nil,
		boundsData:  json.RawMessage(`[]`),
		resultsData: json.RawMessage(`[]`),
	})
	// No lastRequest set either
	err := at.Resume(context.Background(), "sweep-123")
	if err == nil {
		t.Fatal("expected error for no request")
	}
}

// --- WaitForChange (hint_progress.go) ---

func TestWaitForChange_ImmediateDifference(t *testing.T) {
	at := NewAutoTuner(nil)
	rt := NewHINTTuner(at)

	// Status is "idle", passing a different lastStatus should return immediately
	result := rt.WaitForChange(context.Background(), "running")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestWaitForChange_ContextCancelled(t *testing.T) {
	at := NewAutoTuner(nil)
	rt := NewHINTTuner(at)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Status is "idle", passing "idle" should block until context cancelled
	done := make(chan interface{})
	go func() {
		done <- rt.WaitForChange(ctx, "idle")
	}()

	select {
	case result := <-done:
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForChange did not return after context cancellation")
	}
}
