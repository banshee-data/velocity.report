package sweep

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/monitor"
)

// testClient returns a *monitor.Client backed by a dummy HTTP server.
// The server returns empty JSON for any request, preventing nil pointer panics
// when Runner goroutines try to call monitor.Client methods.
func testClient(t *testing.T) *monitor.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return monitor.NewClient(srv.Client(), srv.URL, "test-sensor")
}

// --- AutoTuner accessor tests ---

func TestAutoTuner_SetPersister(t *testing.T) {
	at := NewAutoTuner(nil)
	mock := &mockPersister{}
	at.SetPersister(mock)
	if at.persister != mock {
		t.Error("SetPersister did not set persister")
	}
}

func TestAutoTuner_GetSweepID_Empty(t *testing.T) {
	at := NewAutoTuner(nil)
	if id := at.GetSweepID(); id != "" {
		t.Errorf("GetSweepID = %q, want empty", id)
	}
}

func TestAutoTuner_SetGroundTruthScorer(t *testing.T) {
	at := NewAutoTuner(nil)
	scorer := func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		return 1.0, nil
	}
	at.SetGroundTruthScorer(scorer)
	if at.groundTruthScorer == nil {
		t.Error("SetGroundTruthScorer did not set scorer")
	}
}

func TestAutoTuner_SetSceneStore(t *testing.T) {
	at := NewAutoTuner(nil)
	mock := &mockSceneStore{}
	at.SetSceneStore(mock)
	if at.sceneStore != mock {
		t.Error("SetSceneStore did not set store")
	}
}

func TestAutoTuner_GetAutoTuneState_Initial(t *testing.T) {
	at := NewAutoTuner(nil)
	state := at.GetAutoTuneState()
	if state.Status != SweepStatusIdle {
		t.Errorf("initial status = %q, want idle", state.Status)
	}
	if state.Mode != "auto" {
		t.Errorf("mode = %q, want auto", state.Mode)
	}
}

func TestAutoTuner_GetState_ReturnsAutoTuneState(t *testing.T) {
	at := NewAutoTuner(nil)
	state := at.GetState()
	ats, ok := state.(AutoTuneState)
	if !ok {
		t.Fatal("GetState did not return AutoTuneState")
	}
	if ats.Status != SweepStatusIdle {
		t.Errorf("status = %q, want idle", ats.Status)
	}
}

func TestAutoTuner_Stop_NilCancel(t *testing.T) {
	at := NewAutoTuner(nil)
	at.Stop() // should not panic
}

func TestAutoTuner_Stop_WithCancel(t *testing.T) {
	at := NewAutoTuner(nil)
	_, cancel := context.WithCancel(context.Background())
	at.cancel = cancel
	at.Stop() // should not panic
}

// --- Start validation tests ---

func TestAutoTuner_Start_NilRunner(t *testing.T) {
	at := NewAutoTuner(nil)
	err := at.Start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 3,
	})
	if err == nil {
		t.Error("expected error for nil runner")
	}
}

func TestAutoTuner_Start_InvalidRequestType(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	err := at.Start(context.Background(), 42) // invalid type
	if err == nil {
		t.Error("expected error for invalid request type")
	}
}

func TestAutoTuner_Start_MapRequest(t *testing.T) {
	runner := NewRunner(testClient(t))
	at := NewAutoTuner(runner)
	m := map[string]interface{}{
		"params": []interface{}{
			map[string]interface{}{"name": "p", "type": "float64", "start": 0.0, "end": 1.0},
		},
		"values_per_param": 3,
	}
	err := at.Start(context.Background(), m)
	// Map request is marshalled to AutoTuneRequest and auto-tune starts
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	at.Stop()
	time.Sleep(50 * time.Millisecond)
}

func TestAutoTuner_Start_MaxRoundsExceedsLimit(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		MaxRounds:      11,
		ValuesPerParam: 5,
	})
	if err == nil || err.Error() != "max_rounds must not exceed 10, got 11" {
		t.Errorf("expected max_rounds error, got %v", err)
	}
}

func TestAutoTuner_Start_ValuesPerParamTooLow(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 1,
	})
	if err == nil {
		t.Error("expected error for values_per_param < 2")
	}
}

func TestAutoTuner_Start_ValuesPerParamTooHigh(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 21,
	})
	if err == nil {
		t.Error("expected error for values_per_param > 20")
	}
}

func TestAutoTuner_Start_TopKTooHigh(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 3,
		TopK:           51,
	})
	if err == nil {
		t.Error("expected error for top_k > 50")
	}
}

func TestAutoTuner_Start_NoParams(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         nil,
		ValuesPerParam: 3,
	})
	if err == nil {
		t.Error("expected error for no params")
	}
}

func TestAutoTuner_Start_TooManyParams(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	params := make([]SweepParam, 11)
	for i := range params {
		params[i] = SweepParam{Name: "p" + string(rune('a'+i)), Type: "float64", Start: 0, End: 1}
	}
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         params,
		ValuesPerParam: 3,
	})
	if err == nil {
		t.Error("expected error for too many params")
	}
}

func TestAutoTuner_Start_InvalidParamBounds(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 5, End: 1}},
		ValuesPerParam: 3,
	})
	if err == nil {
		t.Error("expected error for start >= end")
	}
}

func TestAutoTuner_Start_UnsupportedParamType(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "string", Start: 0, End: 1}},
		ValuesPerParam: 3,
	})
	if err == nil {
		t.Error("expected error for unsupported param type")
	}
}

func TestAutoTuner_Start_GroundTruth_NoSceneID(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 3,
		Objective:      "ground_truth",
	})
	if err == nil {
		t.Error("expected error for ground_truth without scene_id")
	}
}

func TestAutoTuner_Start_GroundTruth_NoScorer(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 3,
		Objective:      "ground_truth",
		SceneID:        "scene-1",
	})
	if err == nil {
		t.Error("expected error for ground_truth without scorer")
	}
}

func TestAutoTuner_Start_AlreadyRunning(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	at.mu.Lock()
	at.state.Status = SweepStatusRunning
	at.mu.Unlock()

	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 3,
	})
	if err != ErrSweepAlreadyRunning {
		t.Errorf("expected ErrSweepAlreadyRunning, got %v", err)
	}
}

func TestAutoTuner_Start_IntParams(t *testing.T) {
	runner := NewRunner(testClient(t))
	at := NewAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "int", Start: 0, End: 10}},
		ValuesPerParam: 3,
	})
	// Should pass validation and start (runner may fail later in background)
	if err != nil {
		t.Errorf("unexpected error for int params: %v", err)
	}
	// Clean up
	at.Stop()
	time.Sleep(50 * time.Millisecond)
}

func TestAutoTuner_Start_Int64Params(t *testing.T) {
	runner := NewRunner(testClient(t))
	at := NewAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "int64", Start: 0, End: 100}},
		ValuesPerParam: 3,
	})
	if err != nil {
		t.Errorf("unexpected error for int64 params: %v", err)
	}
	at.Stop()
	time.Sleep(50 * time.Millisecond)
}

// --- waitForSweepComplete tests ---

func TestAutoTuner_WaitForSweepComplete_Cancelled(t *testing.T) {
	runner := NewRunner(nil)
	at := NewAutoTuner(runner)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := at.waitForSweepComplete(ctx)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestAutoTuner_WaitForSweepComplete_Complete(t *testing.T) {
	runner := NewRunner(nil)
	runner.mu.Lock()
	runner.state.Status = SweepStatusComplete
	runner.mu.Unlock()

	at := NewAutoTuner(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := at.waitForSweepComplete(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAutoTuner_WaitForSweepComplete_Error(t *testing.T) {
	runner := NewRunner(nil)
	runner.mu.Lock()
	runner.state.Status = SweepStatusError
	runner.state.Error = "test error"
	runner.mu.Unlock()

	at := NewAutoTuner(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := at.waitForSweepComplete(ctx)
	if err == nil {
		t.Error("expected error for sweep error state")
	}
}

func TestAutoTuner_WaitForSweepComplete_UnexpectedStatus(t *testing.T) {
	runner := NewRunner(nil)
	runner.mu.Lock()
	runner.state.Status = SweepStatus("unknown")
	runner.mu.Unlock()

	at := NewAutoTuner(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := at.waitForSweepComplete(ctx)
	if err == nil {
		t.Error("expected error for unexpected status")
	}
}

// --- setError tests ---

func TestAutoTuner_SetError(t *testing.T) {
	at := NewAutoTuner(nil)
	at.setError("test failure")
	state := at.GetAutoTuneState()
	if state.Status != SweepStatusError {
		t.Errorf("status = %q, want error", state.Status)
	}
	if state.Error != "test failure" {
		t.Errorf("error = %q, want 'test failure'", state.Error)
	}
	if state.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestAutoTuner_SetError_WithPersister(t *testing.T) {
	at := NewAutoTuner(nil)
	mp := &mockPersister{}
	at.SetPersister(mp)
	at.sweepID = "test-sweep-123"
	at.setError("db failure")
	if !mp.completeCalled {
		t.Error("expected persister.SaveSweepComplete to be called")
	}
	if mp.completeStatus != "error" {
		t.Errorf("status = %q, want error", mp.completeStatus)
	}
}

// --- persistComplete tests ---

func TestAutoTuner_PersistComplete_NoPersister(t *testing.T) {
	at := NewAutoTuner(nil)
	at.persistComplete("complete", nil, nil, nil) // should not panic
}

func TestAutoTuner_PersistComplete_NoSweepID(t *testing.T) {
	at := NewAutoTuner(nil)
	at.persister = &mockPersister{}
	at.persistComplete("complete", nil, nil, nil) // should not panic
}

func TestAutoTuner_PersistComplete_WithData(t *testing.T) {
	at := NewAutoTuner(nil)
	mp := &mockPersister{}
	at.SetPersister(mp)
	at.sweepID = "test-123"

	results := []ComboResult{{Noise: 0.01, Closeness: 3.0}}
	rec := map[string]interface{}{"noise": 0.01}
	errMsg := "something went wrong"
	at.persistComplete("error", results, rec, &errMsg)

	if !mp.completeCalled {
		t.Error("expected SaveSweepComplete called")
	}
}

func TestAutoTuner_PersistComplete_WithRoundResults(t *testing.T) {
	at := NewAutoTuner(nil)
	mp := &mockPersister{}
	at.SetPersister(mp)
	at.sweepID = "test-456"

	at.mu.Lock()
	at.state.RoundResults = []RoundSummary{{
		Round: 1, BestScore: 0.9,
		Bounds:     map[string][2]float64{"p": {0, 1}},
		BestParams: map[string]interface{}{"p": 0.5},
	}}
	at.mu.Unlock()

	at.persistComplete("complete", nil, nil, nil)
	if !mp.completeCalled {
		t.Error("expected SaveSweepComplete called")
	}
}

// --- sortScoredResults tests ---

func TestSortScoredResults(t *testing.T) {
	scored := []ScoredResult{
		{Score: 0.5},
		{Score: 0.9},
		{Score: 0.1},
		{Score: 0.7},
	}
	sorted := sortScoredResults(scored)
	if sorted[0].Score != 0.9 {
		t.Errorf("first score = %f, want 0.9", sorted[0].Score)
	}
	if sorted[3].Score != 0.1 {
		t.Errorf("last score = %f, want 0.1", sorted[3].Score)
	}
	// Original should be unchanged
	if scored[0].Score != 0.5 {
		t.Error("original was mutated")
	}
}

func TestSortScoredResults_Empty(t *testing.T) {
	sorted := sortScoredResults(nil)
	if len(sorted) != 0 {
		t.Errorf("expected empty, got %d", len(sorted))
	}
}

// --- copyBounds tests ---

func TestCopyBounds(t *testing.T) {
	original := map[string][2]float64{
		"a": {1.0, 2.0},
		"b": {3.0, 4.0},
	}
	copied := copyBounds(original)
	if len(copied) != 2 {
		t.Errorf("len = %d, want 2", len(copied))
	}
	// Mutate copy shouldn't affect original
	copied["a"] = [2]float64{99, 99}
	if original["a"][0] != 1.0 {
		t.Error("original was mutated")
	}
}

func TestCopyBounds_Empty(t *testing.T) {
	copied := copyBounds(map[string][2]float64{})
	if len(copied) != 0 {
		t.Errorf("expected empty map, got %d", len(copied))
	}
}

// --- copyParamValues tests (already tested but adding more) ---

func TestCopyParamValues_NilInput(t *testing.T) {
	result := copyParamValues(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

// --- GetAutoTuneState deep copy test ---

func TestAutoTuner_GetAutoTuneState_DeepCopy(t *testing.T) {
	at := NewAutoTuner(nil)
	at.mu.Lock()
	at.state.RoundResults = []RoundSummary{{
		Round:      1,
		BestScore:  0.8,
		Bounds:     map[string][2]float64{"p": {0, 1}},
		BestParams: map[string]interface{}{"p": 0.5},
		TopK: []ScoredResult{
			{Score: 0.8, ComboResult: ComboResult{ParamValues: map[string]interface{}{"p": 0.5}}},
		},
	}}
	at.state.Results = []ComboResult{{ParamValues: map[string]interface{}{"p": 0.3}}}
	at.state.Recommendation = map[string]interface{}{"p": 0.5}
	at.mu.Unlock()

	state := at.GetAutoTuneState()
	if len(state.RoundResults) != 1 {
		t.Errorf("expected 1 round result, got %d", len(state.RoundResults))
	}
	if len(state.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(state.Results))
	}
	if state.Recommendation["p"] != 0.5 {
		t.Errorf("recommendation p = %v, want 0.5", state.Recommendation["p"])
	}

	// Mutate copy
	state.RoundResults[0].BestScore = 999
	state.Results[0].Noise = 999
	state.Recommendation["p"] = 999

	// Original should be unchanged
	originalState := at.GetAutoTuneState()
	if originalState.RoundResults[0].BestScore == 999 {
		t.Error("deep copy failed - round results mutated")
	}
	if originalState.Results[0].Noise == 999 {
		t.Error("deep copy failed - results mutated")
	}
}

// --- Start with persister ---

func TestAutoTuner_Start_WithPersister(t *testing.T) {
	runner := NewRunner(testClient(t))
	at := NewAutoTuner(runner)
	mp := &mockPersister{}
	at.SetPersister(mp)

	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 3,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	at.Stop()
	time.Sleep(100 * time.Millisecond)

	if !mp.startCalled {
		t.Error("expected persister.SaveSweepStart to be called")
	}
	if at.GetSweepID() == "" {
		t.Error("sweep ID should be set after start")
	}
}

func TestAutoTuner_Start_NilContext(t *testing.T) {
	runner := NewRunner(testClient(t))
	at := NewAutoTuner(runner)
	//nolint:staticcheck
	err := at.start(nil, AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 3,
	})
	if err != nil {
		t.Errorf("unexpected error with nil context: %v", err)
	}
	at.Stop()
	time.Sleep(50 * time.Millisecond)
}

func TestAutoTuner_Start_GroundTruth_DefaultWeights(t *testing.T) {
	runner := NewRunner(testClient(t))
	at := NewAutoTuner(runner)
	at.SetGroundTruthScorer(func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		return 0.9, nil
	})
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 3,
		Objective:      "ground_truth",
		SceneID:        "scene-1",
		// GroundTruthWeights not set -> should get defaults
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	at.Stop()
	time.Sleep(50 * time.Millisecond)
}

// --- Helper mocks ---

type mockPersister struct {
	startCalled    bool
	completeCalled bool
	completeStatus string
}

func (m *mockPersister) SaveSweepStart(sweepID, sensorID, mode string, request json.RawMessage, startedAt time.Time) error {
	m.startCalled = true
	return nil
}

func (m *mockPersister) SaveSweepComplete(sweepID, status string, results, recommendation, roundResults json.RawMessage, completedAt time.Time, errMsg string) error {
	m.completeCalled = true
	m.completeStatus = status
	return nil
}

type mockSceneStore struct{}

func (m *mockSceneStore) SetOptimalParams(sceneID string, paramsJSON json.RawMessage) error {
	return nil
}
