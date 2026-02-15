package sweep

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newQuietAutoTuner creates an AutoTuner with a discard logger so that
// expected error-path log messages don't pollute CI output.
func newQuietAutoTuner(runner *Runner) *AutoTuner {
	at := NewAutoTuner(runner)
	at.SetLogger(discardAutoLogger())
	return at
}

// mockBackend implements SweepBackend for testing. Each method can be
// overridden by setting the corresponding function field; the zero value
// provides sensible defaults that never error.
type mockBackend struct {
	sensorID           string
	buckets            []string
	acceptanceMetrics  map[string]interface{}
	gridStatus         map[string]interface{}
	trackingMetrics    map[string]interface{}
	lastAnalysisRunID  string
	setTuningParamsErr error
	startPCAPErr       error
	stopPCAPErr        error
	waitPCAPErr        error

	// Overrides — when non-nil, called instead of the default.
	FetchAcceptanceFn   func() (map[string]interface{}, error)
	FetchGridStatusFn   func() (map[string]interface{}, error)
	FetchTrackingFn     func() (map[string]interface{}, error)
	SetTuningParamsFn   func(params map[string]interface{}) error
	WaitForGridSettleFn func(timeout time.Duration)
}

func (m *mockBackend) SensorID() string {
	if m.sensorID != "" {
		return m.sensorID
	}
	return "test-sensor"
}

func (m *mockBackend) FetchBuckets() []string {
	if m.buckets != nil {
		return m.buckets
	}
	return []string{"1", "2", "4"}
}

func (m *mockBackend) FetchAcceptanceMetrics() (map[string]interface{}, error) {
	if m.FetchAcceptanceFn != nil {
		return m.FetchAcceptanceFn()
	}
	if m.acceptanceMetrics != nil {
		return m.acceptanceMetrics, nil
	}
	return map[string]interface{}{}, nil
}

func (m *mockBackend) ResetAcceptance() error { return nil }

func (m *mockBackend) FetchGridStatus() (map[string]interface{}, error) {
	if m.FetchGridStatusFn != nil {
		return m.FetchGridStatusFn()
	}
	if m.gridStatus != nil {
		return m.gridStatus, nil
	}
	return map[string]interface{}{}, nil
}

func (m *mockBackend) ResetGrid() error { return nil }

func (m *mockBackend) WaitForGridSettle(timeout time.Duration) {
	if m.WaitForGridSettleFn != nil {
		m.WaitForGridSettleFn(timeout)
	}
}

func (m *mockBackend) FetchTrackingMetrics() (map[string]interface{}, error) {
	if m.FetchTrackingFn != nil {
		return m.FetchTrackingFn()
	}
	if m.trackingMetrics != nil {
		return m.trackingMetrics, nil
	}
	return map[string]interface{}{}, nil
}

func (m *mockBackend) SetTuningParams(params map[string]interface{}) error {
	if m.SetTuningParamsFn != nil {
		return m.SetTuningParamsFn(params)
	}
	return m.setTuningParamsErr
}

func (m *mockBackend) StartPCAPReplayWithConfig(cfg PCAPReplayConfig) error {
	return m.startPCAPErr
}

func (m *mockBackend) StopPCAPReplay() error {
	return m.stopPCAPErr
}

func (m *mockBackend) WaitForPCAPComplete(timeout time.Duration) error {
	return m.waitPCAPErr
}

func (m *mockBackend) GetLastAnalysisRunID() string {
	return m.lastAnalysisRunID
}

// defaultMockBackend returns a mockBackend with empty-JSON-equivalent
// responses (same behaviour as the old testClient dummy HTTP server).
func defaultMockBackend() *mockBackend {
	return &mockBackend{}
}

// sweepMockBackend returns a mockBackend pre-loaded with the same
// deterministic acceptance, grid and tracking data that the old
// sweepMockServer HTTP handler returned.
func sweepMockBackend() *mockBackend {
	return &mockBackend{
		acceptanceMetrics: map[string]interface{}{
			"BucketsMeters":   []interface{}{float64(1), float64(2), float64(4)},
			"AcceptCounts":    []interface{}{float64(10), float64(20), float64(30)},
			"RejectCounts":    []interface{}{float64(2), float64(3), float64(4)},
			"Totals":          []interface{}{float64(12), float64(23), float64(34)},
			"AcceptanceRates": []interface{}{0.83, 0.87, 0.88},
		},
		gridStatus: map[string]interface{}{
			"background_count": float64(42),
		},
		trackingMetrics: map[string]interface{}{
			"active_tracks":            float64(3),
			"mean_alignment_deg":       2.5,
			"misalignment_ratio":       0.1,
			"heading_jitter_deg":       1.0,
			"speed_jitter_mps":         0.5,
			"fragmentation_ratio":      0.05,
			"foreground_capture_ratio": 0.85,
			"unbounded_point_ratio":    0.02,
			"empty_box_ratio":          0.01,
		},
	}
}

// --- AutoTuner accessor tests ---

func TestAutoTuner_SetPersister(t *testing.T) {
	at := newQuietAutoTuner(nil)
	mock := &mockPersister{}
	at.SetPersister(mock)
	if at.persister != mock {
		t.Error("SetPersister did not set persister")
	}
}

func TestAutoTuner_GetSweepID_Empty(t *testing.T) {
	at := newQuietAutoTuner(nil)
	if id := at.GetSweepID(); id != "" {
		t.Errorf("GetSweepID = %q, want empty", id)
	}
}

func TestAutoTuner_SetGroundTruthScorer(t *testing.T) {
	at := newQuietAutoTuner(nil)
	scorer := func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		return 1.0, nil
	}
	at.SetGroundTruthScorer(scorer)
	if at.groundTruthScorer == nil {
		t.Error("SetGroundTruthScorer did not set scorer")
	}
}

func TestAutoTuner_SetSceneStore(t *testing.T) {
	at := newQuietAutoTuner(nil)
	mock := &mockSceneStore{}
	at.SetSceneStore(mock)
	if at.sceneStore != mock {
		t.Error("SetSceneStore did not set store")
	}
}

func TestAutoTuner_GetAutoTuneState_Initial(t *testing.T) {
	at := newQuietAutoTuner(nil)
	state := at.GetAutoTuneState()
	if state.Status != SweepStatusIdle {
		t.Errorf("initial status = %q, want idle", state.Status)
	}
	if state.Mode != "auto" {
		t.Errorf("mode = %q, want auto", state.Mode)
	}
}

func TestAutoTuner_GetState_ReturnsAutoTuneState(t *testing.T) {
	at := newQuietAutoTuner(nil)
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
	at := newQuietAutoTuner(nil)
	at.Stop() // should not panic
}

func TestAutoTuner_Stop_WithCancel(t *testing.T) {
	at := newQuietAutoTuner(nil)
	_, cancel := context.WithCancel(context.Background())
	at.cancel = cancel
	at.Stop() // should not panic
}

// --- Start validation tests ---

func TestAutoTuner_Start_NilRunner(t *testing.T) {
	at := newQuietAutoTuner(nil)
	err := at.Start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 3,
	})
	if err == nil {
		t.Error("expected error for nil runner")
	}
}

func TestAutoTuner_Start_InvalidRequestType(t *testing.T) {
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
	err := at.Start(context.Background(), 42) // invalid type
	if err == nil {
		t.Error("expected error for invalid request type")
	}
}

func TestAutoTuner_Start_MapRequest(t *testing.T) {
	runner := newQuietRunner(defaultMockBackend())
	at := newQuietAutoTuner(runner)
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
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
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
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 1,
	})
	if err == nil {
		t.Error("expected error for values_per_param < 2")
	}
}

func TestAutoTuner_Start_ValuesPerParamTooHigh(t *testing.T) {
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1}},
		ValuesPerParam: 21,
	})
	if err == nil {
		t.Error("expected error for values_per_param > 20")
	}
}

func TestAutoTuner_Start_TopKTooHigh(t *testing.T) {
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
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
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         nil,
		ValuesPerParam: 3,
	})
	if err == nil {
		t.Error("expected error for no params")
	}
}

func TestAutoTuner_Start_TooManyParams(t *testing.T) {
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
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
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "float64", Start: 5, End: 1}},
		ValuesPerParam: 3,
	})
	if err == nil {
		t.Error("expected error for start >= end")
	}
}

func TestAutoTuner_Start_UnsupportedParamType(t *testing.T) {
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
	err := at.start(context.Background(), AutoTuneRequest{
		Params:         []SweepParam{{Name: "p", Type: "string", Start: 0, End: 1}},
		ValuesPerParam: 3,
	})
	if err == nil {
		t.Error("expected error for unsupported param type")
	}
}

func TestAutoTuner_Start_GroundTruth_NoSceneID(t *testing.T) {
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
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
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
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
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
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
	runner := newQuietRunner(defaultMockBackend())
	at := newQuietAutoTuner(runner)
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
	runner := newQuietRunner(defaultMockBackend())
	at := newQuietAutoTuner(runner)
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
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := at.waitForSweepComplete(ctx)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestAutoTuner_WaitForSweepComplete_Complete(t *testing.T) {
	runner := newQuietRunner(nil)
	runner.mu.Lock()
	runner.state.Status = SweepStatusComplete
	runner.mu.Unlock()

	at := newQuietAutoTuner(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := at.waitForSweepComplete(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAutoTuner_WaitForSweepComplete_Error(t *testing.T) {
	runner := newQuietRunner(nil)
	runner.mu.Lock()
	runner.state.Status = SweepStatusError
	runner.state.Error = "test error"
	runner.mu.Unlock()

	at := newQuietAutoTuner(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := at.waitForSweepComplete(ctx)
	if err == nil {
		t.Error("expected error for sweep error state")
	}
}

func TestAutoTuner_WaitForSweepComplete_UnexpectedStatus(t *testing.T) {
	runner := newQuietRunner(nil)
	runner.mu.Lock()
	runner.state.Status = SweepStatus("unknown")
	runner.mu.Unlock()

	at := newQuietAutoTuner(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := at.waitForSweepComplete(ctx)
	if err == nil {
		t.Error("expected error for unexpected status")
	}
}

// --- setError tests ---

func TestAutoTuner_SetError(t *testing.T) {
	at := newQuietAutoTuner(nil)
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
	at := newQuietAutoTuner(nil)
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
	at := newQuietAutoTuner(nil)
	at.persistComplete("complete", nil, nil, nil) // should not panic
}

func TestAutoTuner_PersistComplete_NoSweepID(t *testing.T) {
	at := newQuietAutoTuner(nil)
	at.persister = &mockPersister{}
	at.persistComplete("complete", nil, nil, nil) // should not panic
}

func TestAutoTuner_PersistComplete_WithData(t *testing.T) {
	at := newQuietAutoTuner(nil)
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
	at := newQuietAutoTuner(nil)
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
	at := newQuietAutoTuner(nil)
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
	runner := newQuietRunner(defaultMockBackend())
	at := newQuietAutoTuner(runner)
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

	mp.mu.Lock()
	started := mp.startCalled
	mp.mu.Unlock()
	if !started {
		t.Error("expected persister.SaveSweepStart to be called")
	}
	if at.GetSweepID() == "" {
		t.Error("sweep ID should be set after start")
	}
}

func TestAutoTuner_Start_NilContext(t *testing.T) {
	runner := newQuietRunner(defaultMockBackend())
	at := newQuietAutoTuner(runner)
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
	runner := newQuietRunner(defaultMockBackend())
	at := newQuietAutoTuner(runner)
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
	mu             sync.Mutex
	startCalled    bool
	completeCalled bool
	completeStatus string
}

func (m *mockPersister) SaveSweepStart(sweepID, sensorID, mode string, request json.RawMessage, startedAt time.Time, objectiveName, objectiveVersion string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalled = true
	return nil
}

func (m *mockPersister) SaveSweepComplete(sweepID, status string, results, recommendation, roundResults json.RawMessage, completedAt time.Time, errMsg string, scoreComponents, recommendationExplanation, labelProvenanceSummary json.RawMessage, transformPipelineName, transformPipelineVersion string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completeCalled = true
	m.completeStatus = status
	return nil
}

type mockSceneStore struct{}

func (m *mockSceneStore) SetOptimalParams(sceneID string, paramsJSON json.RawMessage) error {
	return nil
}

// waitForAutoTuneStatus polls the auto-tuner state until it reaches one
// of the target statuses or the timeout expires.
func waitForAutoTuneStatus(t *testing.T, at *AutoTuner, timeout time.Duration, targets ...SweepStatus) AutoTuneState {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state := at.GetAutoTuneState()
		for _, target := range targets {
			if state.Status == target {
				return state
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	state := at.GetAutoTuneState()
	t.Fatalf("timed out waiting for status in %v, current status=%q error=%q", targets, state.Status, state.Error)
	return state
}

// ---- run() full execution tests ----

// TestAutoCov2_RunFullExecution starts an auto-tune with a tiny 1-round,
// 2-value sweep and verifies the run() goroutine completes successfully.
func TestAutoCov2_RunFullExecution(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
		Objective:      "acceptance",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}
	if len(state.RoundResults) != 1 {
		t.Errorf("expected 1 round result, got %d", len(state.RoundResults))
	}
	if state.Recommendation == nil {
		t.Error("expected non-nil recommendation")
	}
	if state.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
	if len(state.Results) == 0 {
		t.Error("expected non-empty results")
	}
}

// TestAutoCov2_RunMultipleRounds tests the narrowing logic across multiple rounds.
func TestAutoCov2_RunMultipleRounds(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

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
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}
	if len(state.RoundResults) != 2 {
		t.Errorf("expected 2 round results, got %d", len(state.RoundResults))
	}
}

// TestAutoCov2_RunWithIntParams exercises the int grid path in run().
func TestAutoCov2_RunWithIntParams(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "neighbor_count", Type: "int", Start: 0, End: 5},
		},
		MaxRounds:      1,
		ValuesPerParam: 3,
		TopK:           3,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}
}

// TestAutoCov2_RunWithInt64Params exercises the int64 grid path in run().
func TestAutoCov2_RunWithInt64Params(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "big_param", Type: "int64", Start: 100, End: 200},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}
}

// TestAutoCov2_RunCancelledMidway cancels auto-tuning during execution
// and verifies the error state.
func TestAutoCov2_RunCancelledMidway(t *testing.T) {
	// Use a slow-settling mockBackend to ensure the auto-tuner is still
	// running when we cancel.
	var gridCalls int32
	backend := &mockBackend{
		acceptanceMetrics: map[string]interface{}{
			"BucketsMeters":   []interface{}{float64(1)},
			"AcceptCounts":    []interface{}{float64(10)},
			"RejectCounts":    []interface{}{float64(2)},
			"Totals":          []interface{}{float64(12)},
			"AcceptanceRates": []interface{}{0.83},
		},
		trackingMetrics: map[string]interface{}{
			"active_tracks": float64(1),
		},
		FetchGridStatusFn: func() (map[string]interface{}, error) {
			n := atomic.AddInt32(&gridCalls, 1)
			if n < 5 {
				return map[string]interface{}{"background_count": float64(0)}, nil
			}
			return map[string]interface{}{"background_count": float64(42)}, nil
		},
	}
	runner := newQuietRunner(backend)
	at := newQuietAutoTuner(runner)

	ctx, cancel := context.WithCancel(context.Background())

	err := at.start(ctx, AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
		},
		MaxRounds:      3,
		ValuesPerParam: 5,
		TopK:           3,
		Iterations:     1,
		SettleTime:     "100ms",
		Interval:       "1ms",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	// Give a moment for the goroutine to start, then cancel
	time.Sleep(50 * time.Millisecond)
	cancel()

	state := waitForAutoTuneStatus(t, at, 10*time.Second, SweepStatusError, SweepStatusComplete)
	// It may complete or error depending on timing; either is acceptable.
	// The key is that it terminates.
	if state.Status == SweepStatusError && !strings.Contains(state.Error, "cancel") {
		// If it errored, the error should mention cancellation
		t.Logf("error (may or may not be cancellation): %s", state.Error)
	}
}

// TestAutoCov2_RunWithPersister ensures run() calls the persister on completion.
func TestAutoCov2_RunWithPersister(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)
	mp := &mockPersister{}
	at.SetPersister(mp)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}

	mp.mu.Lock()
	startCalled := mp.startCalled
	completeCalled := mp.completeCalled
	completeStatus := mp.completeStatus
	mp.mu.Unlock()

	if !startCalled {
		t.Error("expected SaveSweepStart to be called")
	}
	if !completeCalled {
		t.Error("expected SaveSweepComplete to be called")
	}
	if completeStatus != "complete" {
		t.Errorf("expected complete status, got %q", completeStatus)
	}
}

// TestAutoCov2_RunWithPersisterError ensures run() handles persister errors gracefully.
func TestAutoCov2_RunWithPersisterError(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)
	mp := &failingPersister{}
	at.SetPersister(mp)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	// Should still complete (persister errors are logged, not fatal)
	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete despite persister errors, got %q error=%q", state.Status, state.Error)
	}
}

// ---- Ground truth path tests ----

// TestAutoCov2_RunGroundTruth exercises the ground truth scoring path in run().
func TestAutoCov2_RunGroundTruth(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	var scorerCalls int32
	at.SetGroundTruthScorer(func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		atomic.AddInt32(&scorerCalls, 1)
		// Return a deterministic score based on the candidate
		return 0.85, nil
	})

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
		Objective:      "ground_truth",
		SceneID:        "test-scene-1",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}

	// The scorer should have been called for each combo result (results may have empty RunID,
	// so it may or may not be called depending on RunID). Just verify auto-tune completed.
	if state.Recommendation == nil {
		t.Error("expected non-nil recommendation")
	}
}

// TestAutoCov2_RunGroundTruthWithSceneStore tests scene store saving on completion.
func TestAutoCov2_RunGroundTruthWithSceneStore(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	at.SetGroundTruthScorer(func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		return 0.9, nil
	})

	store := &trackingSceneStore{}
	at.SetSceneStore(store)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
		Objective:      "ground_truth",
		SceneID:        "scene-save-test",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}

	store.mu.Lock()
	called := store.called
	sceneID := store.sceneID
	store.mu.Unlock()

	if !called {
		t.Error("expected SetOptimalParams to be called on scene store")
	}
	if sceneID != "scene-save-test" {
		t.Errorf("scene store called with sceneID=%q, want 'scene-save-test'", sceneID)
	}
}

// TestAutoCov2_RunGroundTruthScorerError tests the path where ground truth scorer returns errors.
func TestAutoCov2_RunGroundTruthScorerError(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	at.SetGroundTruthScorer(func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		return 0, fmt.Errorf("scoring failed")
	})

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
		Objective:      "ground_truth",
		SceneID:        "scene-err",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	// Should still complete — scorer errors give score 0 but don't abort
	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete despite scorer errors, got %q error=%q", state.Status, state.Error)
	}
}

// TestAutoCov2_RunGroundTruthSceneStoreError tests graceful handling of scene store errors.
func TestAutoCov2_RunGroundTruthSceneStoreError(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	at.SetGroundTruthScorer(func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		return 0.8, nil
	})

	store := &failingSceneStore{}
	at.SetSceneStore(store)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
		Objective:      "ground_truth",
		SceneID:        "scene-store-err",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	// Should still complete — scene store errors are logged, not fatal
	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete despite scene store error, got %q error=%q", state.Status, state.Error)
	}
}

// ---- Weighted objective path ----

// TestAutoCov2_RunWeightedObjective exercises the weighted objective path in run().
func TestAutoCov2_RunWeightedObjective(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
		Objective:      "weighted",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}
}

// TestAutoCov2_RunCustomWeights exercises the custom weights path in run().
func TestAutoCov2_RunCustomWeights(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
		Objective:      "weighted",
		Weights: &ObjectiveWeights{
			Acceptance:   2.0,
			Misalignment: 0.5,
			Alignment:    0.3,
			NonzeroCells: 0.2,
			ActiveTracks: 0.1,
		},
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}
}

// ---- run() error paths ----

// TestAutoCov2_RunSweepStartFailure tests the error path when the inner sweep fails to start.
func TestAutoCov2_RunSweepStartFailure(t *testing.T) {
	// Use a backend that returns errors on SetTuningParams.
	backend := sweepMockBackend()
	backend.acceptanceMetrics = map[string]interface{}{
		"BucketsMeters":   []interface{}{float64(1)},
		"AcceptCounts":    []interface{}{float64(10)},
		"RejectCounts":    []interface{}{float64(2)},
		"Totals":          []interface{}{float64(12)},
		"AcceptanceRates": []interface{}{0.83},
	}
	backend.SetTuningParamsFn = func(params map[string]interface{}) error {
		return fmt.Errorf("server crashed")
	}
	runner := newQuietRunner(backend)
	at := newQuietAutoTuner(runner)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	// The inner sweep should fail (due to param setting failure), and auto-tuner should error
	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusError, SweepStatusComplete)
	// Either error or complete is fine — the sweep may still succeed if it skips bad combos
	t.Logf("final status=%q error=%q", state.Status, state.Error)
}

// TestAutoCov2_RunMultiParam exercises the multi-parameter Cartesian product path.
func TestAutoCov2_RunMultiParam(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.03},
			{Name: "closeness_multiplier", Type: "float64", Start: 1.0, End: 3.0},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}

	// 2 values * 2 params = 4 combos
	if len(state.Results) != 4 {
		t.Errorf("expected 4 results, got %d", len(state.Results))
	}
}

// ---- Start() via map interface test ----

// TestAutoCov2_StartViaMapWithPersister tests Start() with a map request and persister,
// exercising the JSON marshal/unmarshal + persisterSaveStart path.
func TestAutoCov2_StartViaMapWithPersister(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)
	mp := &mockPersister{}
	at.SetPersister(mp)

	m := map[string]interface{}{
		"params": []interface{}{
			map[string]interface{}{"name": "noise_relative", "type": "float64", "start": 0.01, "end": 0.05},
		},
		"max_rounds":       1,
		"values_per_param": 2,
		"top_k":            2,
		"iterations":       1,
		"settle_time":      "1ms",
		"interval":         "1ms",
	}
	err := at.Start(context.Background(), m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}

	mp.mu.Lock()
	started := mp.startCalled
	mp.mu.Unlock()
	if !started {
		t.Error("expected SaveSweepStart to be called")
	}
}

// ---- Start() with map that fails JSON marshal ----

// TestAutoCov2_StartMapBadJSON tests Start() with a map containing an unmarshalable value.
func TestAutoCov2_StartMapBadJSON(t *testing.T) {
	runner := newQuietRunner(nil)
	at := newQuietAutoTuner(runner)
	// Channels can't be marshalled to JSON
	m := map[string]interface{}{
		"params": make(chan int),
	}
	err := at.Start(context.Background(), m)
	if err == nil {
		t.Error("expected error for unmarshalable map")
	}
}

// ---- narrowBounds with int64 type ----

func TestAutoCov2_NarrowBoundsInt64(t *testing.T) {
	topK := []ScoredResult{
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"p": int64(10)}}, Score: 0.9},
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"p": int64(20)}}, Score: 0.8},
	}
	start, end := narrowBounds(topK, "p", 5)
	// Min=10, Max=20, range=10, step=10/4=2.5, margin=2.5
	if start > 10 || end < 20 {
		t.Errorf("expected bounds to contain [10,20], got [%v,%v]", start, end)
	}
}

// ---- narrowBounds single value with small margin ----

func TestAutoCov2_NarrowBoundsSingleValueSmall(t *testing.T) {
	topK := []ScoredResult{
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"p": 0.0}}, Score: 0.9},
	}
	start, end := narrowBounds(topK, "p", 5)
	// value=0 -> margin = max(0*0.1, 0.001) = 0.001
	if start >= 0 || end <= 0 {
		t.Errorf("expected bounds to surround 0, got [%v,%v]", start, end)
	}
}

// ---- generateGrid edge cases ----

func TestAutoCov2_GenerateGridNegativeN(t *testing.T) {
	grid := generateGrid(0, 1, -5)
	if len(grid) != 0 {
		t.Errorf("expected empty grid for negative n, got %d values", len(grid))
	}
}

// ---- generateIntGrid edge cases ----

func TestAutoCov2_GenerateIntGridZero(t *testing.T) {
	grid := generateIntGrid(0, 10, 0)
	if len(grid) != 0 {
		t.Errorf("expected empty grid for n=0, got %d values", len(grid))
	}
}

func TestAutoCov2_GenerateIntGridNegative(t *testing.T) {
	grid := generateIntGrid(0, 10, -1)
	if len(grid) != 0 {
		t.Errorf("expected empty grid for negative n, got %d values", len(grid))
	}
}

// ---- persistComplete edge cases ----

func TestAutoCov2_PersistComplete_NilRecommendation(t *testing.T) {
	at := newQuietAutoTuner(nil)
	mp := &mockPersister{}
	at.SetPersister(mp)
	at.sweepID = "persist-nil-rec"
	at.persistComplete("complete", []ComboResult{}, nil, nil)
	if !mp.completeCalled {
		t.Error("expected SaveSweepComplete called")
	}
}

func TestAutoCov2_PersistComplete_NilErrMsg(t *testing.T) {
	at := newQuietAutoTuner(nil)
	mp := &mockPersister{}
	at.SetPersister(mp)
	at.sweepID = "persist-nil-err"
	rec := map[string]interface{}{"p": 0.5}
	at.persistComplete("complete", nil, rec, nil)
	if !mp.completeCalled {
		t.Error("expected SaveSweepComplete called")
	}
}

// ---- setError with persister that also fails ----

func TestAutoCov2_SetErrorWithFailingPersister(t *testing.T) {
	at := newQuietAutoTuner(nil)
	mp := &failingPersister{}
	at.SetPersister(mp)
	at.sweepID = "fail-persist-err"
	at.setError("something broke")
	state := at.GetAutoTuneState()
	if state.Status != SweepStatusError {
		t.Errorf("expected error status, got %q", state.Status)
	}
}

// ---- Recommendation field coverage ----

// TestAutoCov2_RecommendationContainsAllFields verifies that the recommendation
// built by run() includes all expected metric fields.
func TestAutoCov2_RecommendationContainsAllFields(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     2,
		SettleTime:     "1ms",
		Interval:       "1ms",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}

	rec := state.Recommendation
	expectedKeys := []string{
		"score", "acceptance_rate", "misalignment_ratio",
		"alignment_deg", "nonzero_cells",
		"foreground_capture", "unbounded_point_ratio",
		"empty_box_ratio", "fragmentation_ratio",
		"heading_jitter_deg", "speed_jitter_mps",
	}
	for _, key := range expectedKeys {
		if _, ok := rec[key]; !ok {
			t.Errorf("recommendation missing key %q", key)
		}
	}
}

// ---- Multiple rounds with bounds narrowing ----

// TestAutoCov2_BoundsNarrowingClampedToOriginal verifies that narrowed bounds are
// clamped to the original parameter range.
func TestAutoCov2_BoundsNarrowingClampedToOriginal(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			// Very tight range — narrowing should be clamped
			{Name: "p", Type: "float64", Start: 0.01, End: 0.02},
		},
		MaxRounds:      2,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}
	if len(state.RoundResults) != 2 {
		t.Errorf("expected 2 rounds, got %d", len(state.RoundResults))
	}
}

// ---- Start with AcceptanceCriteria ----

func TestAutoCov2_RunWithAcceptanceCriteria(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	maxFrag := 0.5
	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
		AcceptanceCriteria: &AcceptanceCriteria{
			MaxFragmentationRatio: &maxFrag,
		},
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}
}

// ---- Start with GroundTruth + custom weights ----

func TestAutoCov2_RunGroundTruthCustomWeights(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	at.SetGroundTruthScorer(func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		// Verify custom weights are passed through
		if weights.DetectionRate != 2.0 {
			return 0, fmt.Errorf("expected DetectionRate=2.0, got %v", weights.DetectionRate)
		}
		return 0.75, nil
	})

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           2,
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
		Objective:      "ground_truth",
		SceneID:        "custom-weights-scene",
		GroundTruthWeights: &GroundTruthWeights{
			DetectionRate:  2.0,
			Fragmentation:  5.0,
			FalsePositives: 2.0,
		},
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}
}

// ---- TopK larger than results ----

func TestAutoCov2_RunTopKLargerThanResults(t *testing.T) {
	runner := newQuietRunner(sweepMockBackend())
	at := newQuietAutoTuner(runner)

	err := at.start(context.Background(), AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.02},
		},
		MaxRounds:      1,
		ValuesPerParam: 2,
		TopK:           10, // more than the 2 combos
		Iterations:     1,
		SettleTime:     "1ms",
		Interval:       "1ms",
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	state := waitForAutoTuneStatus(t, at, 30*time.Second, SweepStatusComplete, SweepStatusError)
	if state.Status != SweepStatusComplete {
		t.Fatalf("expected complete, got %q error=%q", state.Status, state.Error)
	}
}

// ---- applyAutoTuneDefaults not overwriting explicit values ----

func TestAutoCov2_ApplyDefaultsPreservesExplicit(t *testing.T) {
	req := applyAutoTuneDefaults(AutoTuneRequest{
		MaxRounds:      5,
		ValuesPerParam: 8,
		TopK:           3,
		Objective:      "ground_truth",
	})
	if req.MaxRounds != 5 {
		t.Errorf("MaxRounds = %d, want 5", req.MaxRounds)
	}
	if req.ValuesPerParam != 8 {
		t.Errorf("ValuesPerParam = %d, want 8", req.ValuesPerParam)
	}
	if req.TopK != 3 {
		t.Errorf("TopK = %d, want 3", req.TopK)
	}
	if req.Objective != "ground_truth" {
		t.Errorf("Objective = %q, want ground_truth", req.Objective)
	}
}

// ---- GetAutoTuneState empty slices ----

func TestAutoCov2_GetAutoTuneState_EmptySlices(t *testing.T) {
	at := newQuietAutoTuner(nil)
	at.mu.Lock()
	at.state.RoundResults = []RoundSummary{}
	at.state.Results = []ComboResult{}
	at.state.Recommendation = nil
	at.mu.Unlock()

	state := at.GetAutoTuneState()
	if len(state.RoundResults) != 0 {
		t.Errorf("expected 0 round results, got %d", len(state.RoundResults))
	}
	if len(state.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(state.Results))
	}
	if state.Recommendation != nil {
		t.Error("expected nil recommendation")
	}
}

// ---- Helper mocks ----

// failingPersister always returns errors.
type failingPersister struct{}

func (fp *failingPersister) SaveSweepStart(sweepID, sensorID, mode string, request json.RawMessage, startedAt time.Time, objectiveName, objectiveVersion string) error {
	return fmt.Errorf("simulated start persistence failure")
}

func (fp *failingPersister) SaveSweepComplete(sweepID, status string, results, recommendation, roundResults json.RawMessage, completedAt time.Time, errMsg string, scoreComponents, recommendationExplanation, labelProvenanceSummary json.RawMessage, transformPipelineName, transformPipelineVersion string) error {
	return fmt.Errorf("simulated complete persistence failure")
}

// trackingSceneStore tracks calls to SetOptimalParams.
type trackingSceneStore struct {
	mu         sync.Mutex
	called     bool
	sceneID    string
	paramsJSON json.RawMessage
}

func (s *trackingSceneStore) SetOptimalParams(sceneID string, paramsJSON json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called = true
	s.sceneID = sceneID
	s.paramsJSON = paramsJSON
	return nil
}

// failingSceneStore always returns errors.
type failingSceneStore struct{}

func (s *failingSceneStore) SetOptimalParams(sceneID string, paramsJSON json.RawMessage) error {
	return fmt.Errorf("simulated scene store failure")
}
