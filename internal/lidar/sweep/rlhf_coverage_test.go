package sweep

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// errForTest is a simple error type for tests.
type errForTest string

func (e errForTest) Error() string { return string(e) }

// initRunState sets up the tuner state as Start() would before calling run().
func initRunState(tuner *RLHFTuner, numRounds int) {
	tuner.mu.Lock()
	tuner.state = RLHFState{
		Status:      "running_reference",
		Mode:        "rlhf",
		TotalRounds: numRounds,
	}
	tuner.mu.Unlock()
}

// TestRun_SceneGetterNil tests run() when sceneGetter is nil.
func TestRun_SceneGetterNil(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetPersister(&mockRLHFPersister{})
	tuner.sweepID = "test-nil-getter"
	initRunState(tuner, 1)
	tuner.run(context.Background(), RLHFSweepRequest{SceneID: "s1"})
	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Fatalf("expected failed, got %s", state.Status)
	}
	if state.Error != "scene getter not configured" {
		t.Fatalf("unexpected error: %s", state.Error)
	}
}

// TestRun_SceneNotFound tests run() when scene lookup fails.
func TestRun_SceneNotFound(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetPersister(&mockRLHFPersister{})
	tuner.SetSceneGetter(&mockSceneGetter{err: errForTest("scene not found")})
	tuner.sweepID = "test-scene-notfound"
	initRunState(tuner, 1)
	tuner.run(context.Background(), RLHFSweepRequest{SceneID: "missing"})
	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Fatalf("expected failed, got %s", state.Status)
	}
}

// TestRun_RunCreatorNil tests runRound when run creator is nil.
func TestRun_RunCreatorNil(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetPersister(&mockRLHFPersister{})
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})
	tuner.sweepID = "test-no-creator"
	initRunState(tuner, 1)
	tuner.run(context.Background(), RLHFSweepRequest{
		SceneID: "s1", NumRounds: 1,
		Params: []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Fatalf("expected failed, got %s", state.Status)
	}
}

// TestRun_RunCreatorFails tests run when CreateSweepRun fails.
func TestRun_RunCreatorFails(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetPersister(&mockRLHFPersister{})
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})
	tuner.SetRunCreator(&mockRunCreator{err: errForTest("create failed")})
	tuner.sweepID = "test-create-fails"
	initRunState(tuner, 1)
	tuner.run(context.Background(), RLHFSweepRequest{
		SceneID: "s1", NumRounds: 1,
		Params: []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Fatalf("expected failed, got %s", state.Status)
	}
}

// TestRun_ContextCancelledDuringLabels tests cancellation during label wait.
func TestRun_ContextCancelledDuringLabels(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetPersister(&mockRLHFPersister{})
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})
	tuner.SetRunCreator(&mockRunCreator{runID: "run-1"})
	tuner.pollInterval = 10 * time.Millisecond
	tuner.sweepID = "test-ctx-cancel"
	initRunState(tuner, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately before run

	tuner.run(ctx, RLHFSweepRequest{
		SceneID: "s1", NumRounds: 1,
		Params:         []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
		RoundDurations: []int{1},
	})
	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Fatalf("expected failed, got %s", state.Status)
	}
}

// TestRun_AutoTunerNil tests run when autoTuner is nil after labels complete.
func TestRun_AutoTunerNil(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetPersister(&mockRLHFPersister{})
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})
	tuner.SetRunCreator(&mockRunCreator{runID: "run-1"})
	tuner.SetLabelQuerier(&mockLabelQuerier{total: 10, labelled: 10, byClass: map[string]int{"car": 10}})
	tuner.pollInterval = 10 * time.Millisecond
	tuner.sweepID = "test-no-autotuner"
	initRunState(tuner, 1)

	tuner.run(context.Background(), RLHFSweepRequest{
		SceneID: "s1", NumRounds: 1,
		Params:            []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
		RoundDurations:    []int{0},
		MinLabelThreshold: 0.5,
	})
	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Fatalf("expected failed, got %s", state.Status)
	}
}

// TestRun_WithExistingOptimalParams tests run with valid pre-existing optimal params.
func TestRun_WithExistingOptimalParams(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetPersister(&mockRLHFPersister{})
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{
			SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap",
			OptimalParamsJSON: json.RawMessage(`{"eps": 0.5}`),
		},
	})
	tuner.SetRunCreator(&mockRunCreator{err: errForTest("expected fail")})
	tuner.sweepID = "test-existing-params"
	initRunState(tuner, 1)

	tuner.run(context.Background(), RLHFSweepRequest{
		SceneID: "s1", NumRounds: 1,
		Params: []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Fatalf("expected failed, got %s", state.Status)
	}
}

// TestGetRLHFState_DeepCopy_MinClassCoverage tests deep copy.
func TestGetRLHFState_DeepCopy_MinClassCoverage(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.mu.Lock()
	tuner.state.MinClassCoverage = map[string]int{"car": 5, "pedestrian": 3}
	tuner.state.Recommendation = map[string]interface{}{"eps": 0.5}
	tuner.state.RoundHistory = []RLHFRound{
		{Round: 1, ReferenceRunID: "run-1", BestParams: map[string]float64{"eps": 0.5},
			LabelProgress: &LabelProgress{Total: 10, Labelled: 5, ByClass: map[string]int{"car": 5}}},
	}
	tuner.mu.Unlock()

	state := tuner.GetRLHFState()
	if state.MinClassCoverage["car"] != 5 {
		t.Fatalf("expected car=5, got %d", state.MinClassCoverage["car"])
	}
	if len(state.RoundHistory) != 1 || state.RoundHistory[0].BestParams["eps"] != 0.5 {
		t.Fatal("round history deep copy mismatch")
	}
}

// TestGetRLHFState_DeepCopy_LabelDeadline tests deep copy of LabelDeadline.
func TestGetRLHFState_DeepCopy_LabelDeadline(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	now := time.Now()
	tuner.mu.Lock()
	tuner.state.LabelDeadline = &now
	tuner.mu.Unlock()
	state := tuner.GetRLHFState()
	if state.LabelDeadline == nil || !state.LabelDeadline.Equal(now) {
		t.Fatal("label deadline mismatch")
	}
}

// TestStop_WithoutStart tests Stop() when nothing is running.
func TestStop_WithoutStart(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.Stop()
	if tuner.GetRLHFState().Status != "idle" {
		t.Fatal("expected idle after stop")
	}
}

// TestTemporalIoU_LargeValues tests temporalIoU with large timestamp values.
func TestTemporalIoU_LargeValues(t *testing.T) {
	a := int64(1700000000000000000)
	b := a + 1000000000
	c := a + 500000000
	d := a + 1500000000
	iou := temporalIoU(a, b, c, d)
	expected := 1.0 / 3.0
	if diff := iou - expected; diff > 0.001 || diff < -0.001 {
		t.Fatalf("expected ~%.4f, got %.4f", expected, iou)
	}
}

// TestContinueFromLabels_TemporalSpreadFail tests temporal spread gate failure.
func TestContinueFromLabels_TemporalSpreadFail(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.mu.Lock()
	tuner.state.Status = "awaiting_labels"
	tuner.state.ReferenceRunID = "run-1"
	tuner.state.MinLabelThreshold = 0.5
	tuner.state.MinTemporalSpreadSecs = 60.0 // require 60 seconds spread
	tuner.mu.Unlock()

	// Use a channel to signal that GetRunTracks was called, confirming
	// the temporal spread gate was reached.
	tracksCalled := make(chan struct{}, 1)
	tuner.SetLabelQuerier(&mockLabelQuerier{
		total:    10,
		labelled: 10,
		byClass:  map[string]int{"car": 10},
		// prevTracks has small temporal spread (only 1 second)
		prevTracks: []RLHFRunTrack{
			{TrackID: "t1", StartUnixNanos: 1000000000, EndUnixNanos: 2000000000, UserLabel: "car"},
		},
		onGetRunTracks: func() { tracksCalled <- struct{}{} },
	})

	err := tuner.ContinueFromLabels(60, false)

	// Wait for the tracks query signal to confirm the gate was reached.
	select {
	case <-tracksCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for GetRunTracks to be called")
	}

	if err == nil {
		t.Fatal("expected temporal spread error")
	}
	if !strings.Contains(err.Error(), "temporal spread") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestContinueFromLabels_ClassCoverageFail tests class coverage gate failure.
func TestContinueFromLabels_ClassCoverageFail(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.mu.Lock()
	tuner.state.Status = "awaiting_labels"
	tuner.state.ReferenceRunID = "run-1"
	tuner.state.MinLabelThreshold = 0.5
	tuner.state.MinClassCoverage = map[string]int{"car": 10, "pedestrian": 5}
	tuner.mu.Unlock()

	tuner.SetLabelQuerier(&mockLabelQuerier{
		total:    20,
		labelled: 15,
		byClass:  map[string]int{"car": 3}, // Not enough car labels
	})

	err := tuner.ContinueFromLabels(60, false)
	if err == nil {
		t.Fatal("expected class coverage error")
	}
	if !strings.Contains(err.Error(), "class coverage") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestContinueFromLabels_AddRoundAndDuration tests addRound and duration update.
func TestContinueFromLabels_AddRoundAndDuration(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.mu.Lock()
	tuner.state.Status = "awaiting_labels"
	tuner.state.ReferenceRunID = "run-1"
	tuner.state.TotalRounds = 2
	tuner.state.MinLabelThreshold = 0.5
	tuner.continueCh = make(chan continueSignal, 1) // buffered so it doesn't block
	tuner.mu.Unlock()

	tuner.SetLabelQuerier(&mockLabelQuerier{
		total: 10, labelled: 10, byClass: map[string]int{"car": 10},
	})

	err := tuner.ContinueFromLabels(120, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state := tuner.GetRLHFState()
	if state.TotalRounds != 3 {
		t.Fatalf("expected 3 total rounds (added 1), got %d", state.TotalRounds)
	}
	if state.NextSweepDuration != 120 {
		t.Fatalf("expected sweep duration 120, got %d", state.NextSweepDuration)
	}
}

// TestContinueFromLabels_LabelQuerierError tests error from label querier.
func TestContinueFromLabels_LabelQuerierError(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.mu.Lock()
	tuner.state.Status = "awaiting_labels"
	tuner.state.ReferenceRunID = "run-1"
	tuner.mu.Unlock()

	tuner.SetLabelQuerier(&mockLabelQuerier{err: errForTest("query failed")})

	err := tuner.ContinueFromLabels(60, false)
	if err == nil || !strings.Contains(err.Error(), "query failed") {
		t.Fatalf("expected query failed error, got: %v", err)
	}
}

// TestFailWithError_NoPersister tests failWithError without persister.
func TestFailWithError_NoPersister(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.failWithError("test error")
	state := tuner.GetRLHFState()
	if state.Status != "failed" || state.Error != "test error" {
		t.Fatal("expected failed state with error message")
	}
}

// TestRun_FullSuccessPath exercises the complete success path through run().
func TestRun_FullSuccessPath(t *testing.T) {
	// Use a real mock HTTP server so the auto-tuner's runner can complete.
	srv := sweepMockServer(t)
	client := sweepTestClient(t, srv)
	runner := NewRunner(client)
	at := NewAutoTuner(runner)
	at.SetGroundTruthScorer(func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		return 0.85, nil
	})

	persister := &mockRLHFPersister{}
	sceneSaver := &mockSceneSaver{}
	tuner := NewRLHFTuner(at)
	tuner.SetPersister(persister)
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{
			SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap",
			OptimalParamsJSON: json.RawMessage(`{"eps": 0.3}`),
		},
	})
	tuner.SetRunCreator(&mockRunCreator{runID: "run-1"})
	tuner.SetSceneStore(sceneSaver)
	tuner.SetLabelQuerier(&mockLabelQuerier{
		total: 10, labelled: 10, byClass: map[string]int{"car": 10},
	})
	tuner.pollInterval = 10 * time.Millisecond
	tuner.sweepID = "test-full-success"
	initRunState(tuner, 1)

	tuner.run(context.Background(), RLHFSweepRequest{
		SceneID:        "s1",
		NumRounds:      1,
		Params:         []SweepParam{{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05}},
		RoundDurations: []int{0},
		Iterations:     1,
		ValuesPerParam: 2,
		TopK:           2,
		Interval:       "1ms",
		SettleTime:     "1ms",
	})

	state := tuner.GetRLHFState()
	if state.Status != string(SweepStatusComplete) {
		t.Fatalf("expected completed, got %s (error: %s)", state.Status, state.Error)
	}
	if sceneSaver.calls == 0 {
		t.Fatal("expected SetOptimalParams to be called")
	}
	if persister.completeCalls == 0 {
		t.Fatal("expected SaveSweepComplete to be called")
	}
}

// TestRun_MultipleRounds exercises run() with 2 rounds using a real mock server.
func TestRun_MultipleRounds(t *testing.T) {
	srv := sweepMockServer(t)
	client := sweepTestClient(t, srv)
	runner := NewRunner(client)
	at := NewAutoTuner(runner)
	at.SetGroundTruthScorer(func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		return 0.85, nil
	})

	tuner := NewRLHFTuner(at)
	tuner.SetPersister(&mockRLHFPersister{})
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{
			SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap",
		},
	})
	tuner.SetRunCreator(&mockRunCreator{runID: "run-multi"})
	tuner.SetSceneStore(&mockSceneSaver{})
	tuner.SetLabelQuerier(&mockLabelQuerier{
		total: 10, labelled: 10, byClass: map[string]int{"car": 10},
		prevTracks: []RLHFRunTrack{
			{TrackID: "t1", StartUnixNanos: 1000, EndUnixNanos: 2000, UserLabel: "car", QualityLabel: "good"},
		},
		newTracks: []RLHFRunTrack{
			{TrackID: "nt1", StartUnixNanos: 1000, EndUnixNanos: 2000},
		},
	})
	tuner.pollInterval = 10 * time.Millisecond
	tuner.sweepID = "test-multi"
	initRunState(tuner, 2)

	tuner.run(context.Background(), RLHFSweepRequest{
		SceneID:         "s1",
		NumRounds:       2,
		Params:          []SweepParam{{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05}},
		RoundDurations:  []int{0, 0},
		CarryOverLabels: true,
		Iterations:      1,
		ValuesPerParam:  2,
		TopK:            2,
		Interval:        "1ms",
		SettleTime:      "1ms",
	})

	state := tuner.GetRLHFState()
	if state.Status != string(SweepStatusComplete) {
		t.Fatalf("expected completed, got %s (error: %s)", state.Status, state.Error)
	}
	if len(state.RoundHistory) != 2 {
		t.Fatalf("expected 2 rounds in history, got %d", len(state.RoundHistory))
	}
}

// TestCarryOverLabels_Success tests label carry-over between rounds.
func TestCarryOverLabels_Success(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	querier := &mockLabelQuerier{
		total:    10,
		labelled: 10,
		byClass:  map[string]int{"car": 5, "pedestrian": 5},
		prevTracks: []RLHFRunTrack{
			{TrackID: "t1", StartUnixNanos: 1000, EndUnixNanos: 2000, UserLabel: "car", QualityLabel: "good"},
			{TrackID: "t2", StartUnixNanos: 3000, EndUnixNanos: 4000, UserLabel: "pedestrian", QualityLabel: "good"},
		},
		newTracks: []RLHFRunTrack{
			{TrackID: "nt1", StartUnixNanos: 1000, EndUnixNanos: 2000}, // Overlaps t1
			{TrackID: "nt2", StartUnixNanos: 5000, EndUnixNanos: 6000}, // No overlap
		},
	}
	tuner.SetLabelQuerier(querier)

	carried, err := tuner.carryOverLabels("prev-run", "new-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if carried != 1 {
		t.Fatalf("expected 1 carried label, got %d", carried)
	}
	if querier.labelCalls != 1 {
		t.Fatalf("expected 1 label update call, got %d", querier.labelCalls)
	}
}

// TestCarryOverLabels_NilQuerier tests carry-over with no querier.
func TestCarryOverLabels_NilQuerier(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	carried, err := tuner.carryOverLabels("prev", "new")
	if err == nil {
		t.Fatal("expected error")
	}
	if carried != 0 {
		t.Fatalf("expected 0 carried, got %d", carried)
	}
}

// panicSceneGetter panics on GetScene to test panic recovery.
type panicSceneGetter struct{}

func (p *panicSceneGetter) GetScene(sceneID string) (*RLHFScene, error) {
	panic("intentional test panic")
}

func (p *panicSceneGetter) SetReferenceRun(sceneID, runID string) error {
	return nil
}

// TestRun_PanicRecovery tests that run() recovers from panics.
func TestRun_PanicRecovery(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetPersister(&mockRLHFPersister{})
	tuner.SetSceneGetter(&panicSceneGetter{})
	tuner.sweepID = "test-panic"
	initRunState(tuner, 1)

	// Should not crash - panic should be recovered
	tuner.run(context.Background(), RLHFSweepRequest{SceneID: "s1"})

	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Fatalf("expected failed, got %s", state.Status)
	}
	if !strings.Contains(state.Error, "panic") {
		t.Fatalf("expected panic error, got: %s", state.Error)
	}
}

// TestStart_MapRequest tests Start() with a map[string]interface{} request.
func TestStart_MapRequest(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetPersister(&mockRLHFPersister{})
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})

	req := map[string]interface{}{
		"scene_id":   "s1",
		"num_rounds": 1,
		"params": []interface{}{
			map[string]interface{}{"name": "eps", "type": "float64", "start": 0.1, "end": 1.0},
		},
	}

	err := tuner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tuner.Stop()
}

// TestStart_InvalidRequestType tests Start() with invalid type.
func TestStart_InvalidRequestType(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	err := tuner.Start(context.Background(), 42)
	if err == nil {
		t.Fatal("expected error for invalid request type")
	}
}

// TestStart_PersisterStartError tests Start with persister that fails SaveSweepStart.
func TestStart_PersisterStartError(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetPersister(&mockRLHFPersister{startErr: errForTest("persist fail")})
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})

	err := tuner.Start(context.Background(), RLHFSweepRequest{
		SceneID: "s1", NumRounds: 1,
		Params: []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	if err != nil {
		t.Fatalf("Start should not fail for persist error: %v", err)
	}

	// Poll until the background goroutine has progressed past "running_reference".
	deadline := time.After(2 * time.Second)
	for {
		state := tuner.GetRLHFState()
		if state.Status != "running_reference" {
			break
		}
		select {
		case <-deadline:
			t.Log("timed out waiting for state to change, proceeding with Stop")
			goto stop
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
stop:
	tuner.Stop()
}

// TestGetRLHFState_WithAutoTuneState tests deep copy with AutoTuneState.
func TestGetRLHFState_WithAutoTuneState(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.mu.Lock()
	ats := AutoTuneState{Status: SweepStatusRunning, Error: "test"}
	tuner.state.AutoTuneState = &ats
	tuner.state.RoundHistory = []RLHFRound{
		{Round: 1, ReferenceRunID: "r1", BestParams: map[string]float64{"eps": 0.5}},
	}
	now := time.Now()
	tuner.state.RoundHistory[0].LabelledAt = &now
	tuner.state.RoundHistory[0].BestScoreComponents = &ScoreComponents{CompositeScore: 0.8}
	tuner.mu.Unlock()

	state := tuner.GetRLHFState()
	if state.AutoTuneState == nil || state.AutoTuneState.Status != SweepStatusRunning {
		t.Fatal("auto tune state mismatch")
	}
	if state.RoundHistory[0].LabelledAt == nil {
		t.Fatal("expected non-nil LabelledAt in round history")
	}
	if state.RoundHistory[0].BestScoreComponents == nil || state.RoundHistory[0].BestScoreComponents.CompositeScore != 0.8 {
		t.Fatal("expected non-nil BestScoreComponents")
	}
}

// TestStart_BadMapRequest tests Start() with an invalid map request.
func TestStart_BadMapRequest(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	// Map with non-serializable value causes marshal error
	req := map[string]interface{}{
		"scene_id": make(chan int), // channels can't be marshaled
	}
	err := tuner.Start(context.Background(), req)
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

// TestStart_AlreadyRunning tests Start() when already running.
func TestStart_AlreadyRunning(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.mu.Lock()
	tuner.state.Status = "running_reference"
	tuner.mu.Unlock()

	err := tuner.Start(context.Background(), RLHFSweepRequest{
		SceneID: "s1", NumRounds: 1,
		Params: []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	if err != ErrSweepAlreadyRunning {
		t.Fatalf("expected ErrSweepAlreadyRunning, got: %v", err)
	}
}

// TestStart_ValidationErrors tests various validation failures in Start.
func TestStart_ValidationErrors(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tests := []struct {
		name string
		req  RLHFSweepRequest
	}{
		{"empty scene_id", RLHFSweepRequest{NumRounds: 1, Params: []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}}}},
		{"zero rounds", RLHFSweepRequest{SceneID: "s1", NumRounds: 0, Params: []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}}}},
		{"too many rounds", RLHFSweepRequest{SceneID: "s1", NumRounds: 11, Params: []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}}}},
		{"no params", RLHFSweepRequest{SceneID: "s1", NumRounds: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tuner.Start(context.Background(), tt.req)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// TestBuildAutoTuneRequest tests the auto-tune request builder.
func TestBuildAutoTuneRequest_Coverage(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	bounds := map[string][2]float64{"eps": {0.1, 1.0}, "minpts": {2, 10}}
	req := RLHFSweepRequest{
		SceneID:        "s1",
		Params:         []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}, {Name: "minpts", Type: "float64", Start: 2, End: 10}},
		ValuesPerParam: 5,
		TopK:           3,
		Iterations:     10,
		Interval:       "100ms",
		SettleTime:     "50ms",
		Seed:           "test-seed",
		SettleMode:     "first",
		GroundTruthWeights: &GroundTruthWeights{
			DetectionRate:  0.5,
			Fragmentation:  0.3,
			FalsePositives: 0.2,
		},
		AcceptanceCriteria: &AcceptanceCriteria{},
	}
	scene := &RLHFScene{
		SceneID:  "s1",
		SensorID: "sensor1",
		PCAPFile: "test.pcap",
	}
	result := tuner.buildAutoTuneRequest(bounds, req, scene, 1)
	if len(result.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(result.Params))
	}
}

// TestFailWithError_WithPersister tests failWithError when persister is configured.
func TestFailWithError_WithPersister(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	persister := &mockRLHFPersister{}
	tuner.SetPersister(persister)
	tuner.sweepID = "test-persist-fail"

	tuner.failWithError("some error")

	state := tuner.GetRLHFState()
	if state.Status != "failed" || state.Error != "some error" {
		t.Fatalf("expected failed state, got %s/%s", state.Status, state.Error)
	}
	if persister.completeCalls != 1 {
		t.Fatalf("expected 1 SaveSweepComplete call, got %d", persister.completeCalls)
	}
}

// TestFailWithError_PersisterError tests failWithError when persister returns error.
func TestFailWithError_PersisterError(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetPersister(&mockRLHFPersister{completeErr: errForTest("persist error")})
	tuner.sweepID = "test-persist-err"

	// Should not panic even if persister errors
	tuner.failWithError("test error")

	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Fatalf("expected failed, got %s", state.Status)
	}
}

// TestStop_WithAutoTuner tests Stop() when autoTuner is configured.
func TestStop_WithAutoTuner(t *testing.T) {
	at := NewAutoTuner(nil)
	tuner := NewRLHFTuner(at)
	ctx, cancel := context.WithCancel(context.Background())
	tuner.mu.Lock()
	tuner.cancel = cancel
	tuner.mu.Unlock()

	tuner.Stop()
	_ = ctx // suppress unused warning

	// Verify cancel was called
	if ctx.Err() == nil {
		t.Fatal("expected context to be cancelled")
	}
}

// TestWaitForLabelsOrDeadline_ThresholdMetAfterDeadline tests the polling path
// where threshold is met and deadline has passed.
func TestWaitForLabelsOrDeadline_ThresholdMetAfterDeadline(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetLabelQuerier(&mockLabelQuerier{
		total: 10, labelled: 10, byClass: map[string]int{"car": 10},
	})
	tuner.pollInterval = 1 * time.Millisecond

	// Use 0 duration so deadline is already in the past
	err := tuner.waitForLabelsOrDeadline(context.Background(), "run-1", 0, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestWaitForLabelsOrDeadline_DeadlineExpiredBelowThreshold tests deadline
// expiring without meeting threshold.
func TestWaitForLabelsOrDeadline_DeadlineExpiredBelowThreshold(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetLabelQuerier(&mockLabelQuerier{
		total: 10, labelled: 2, byClass: map[string]int{"car": 2},
	})
	tuner.pollInterval = 1 * time.Millisecond

	err := tuner.waitForLabelsOrDeadline(context.Background(), "run-1", 0, 0.5)
	if err == nil {
		t.Fatal("expected deadline expired error")
	}
	if !strings.Contains(err.Error(), "deadline expired") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestWaitForLabelsOrDeadline_NilQuerier tests polling with nil label querier.
// With nil querier, the ticker case hits 'continue' which skips the deadline check.
// The only way out is context cancellation.
func TestWaitForLabelsOrDeadline_NilQuerier(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	// No label querier set
	tuner.pollInterval = 1 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := tuner.waitForLabelsOrDeadline(ctx, "run-1", 60, 0.5)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestWaitForLabelsOrDeadline_ContinueSignal tests the continue channel path.
func TestWaitForLabelsOrDeadline_ContinueSignal(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.pollInterval = 100 * time.Second // very long to avoid ticker

	// Use a buffered channel so the signal is available immediately
	// without relying on timing between sender and receiver.
	tuner.continueCh = make(chan continueSignal, 1)
	tuner.continueCh <- continueSignal{NextSweepDurationMins: 30}

	err := tuner.waitForLabelsOrDeadline(context.Background(), "run-1", 60, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state := tuner.GetRLHFState()
	if state.NextSweepDuration != 30 {
		t.Fatalf("expected NextSweepDuration=30, got %d", state.NextSweepDuration)
	}
}

// TestWaitForLabelsOrDeadline_LabelQuerierError tests polling when querier returns error.
// When querier errors, the ticker case hits 'continue' which skips the deadline check.
// Use context timeout to break out.
func TestWaitForLabelsOrDeadline_LabelQuerierError(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetLabelQuerier(&mockLabelQuerier{err: errForTest("label error")})
	tuner.pollInterval = 1 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := tuner.waitForLabelsOrDeadline(ctx, "run-1", 60, 0.5)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestCarryOverLabels_GetRunTracksError tests carryOverLabels when GetRunTracks fails.
func TestCarryOverLabels_GetRunTracksError(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetLabelQuerier(&mockLabelQuerier{err: errForTest("tracks error")})

	carried, err := tuner.carryOverLabels("prev", "new")
	if err == nil {
		t.Fatal("expected error")
	}
	if carried != 0 {
		t.Fatalf("expected 0, got %d", carried)
	}
}

// TestCarryOverLabels_UpdateTrackLabelError tests label carry-over when update fails.
func TestCarryOverLabels_UpdateTrackLabelError(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetLabelQuerier(&mockLabelQuerier{
		prevTracks: []RLHFRunTrack{
			{TrackID: "t1", StartUnixNanos: 1000, EndUnixNanos: 2000, UserLabel: "car", QualityLabel: "good"},
		},
		newTracks: []RLHFRunTrack{
			{TrackID: "nt1", StartUnixNanos: 1000, EndUnixNanos: 2000},
		},
		updateErr: errForTest("update failed"),
	})

	carried, err := tuner.carryOverLabels("prev", "new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Update failed, so carried should be 0
	if carried != 0 {
		t.Fatalf("expected 0 carried (update error), got %d", carried)
	}
}

// TestGetDuration tests getDuration edge cases.
func TestGetDuration_Coverage(t *testing.T) {
	// Empty durations returns 60
	if d := getDuration(nil, 0); d != 60 {
		t.Fatalf("expected 60, got %d", d)
	}
	// Index beyond length returns last element
	if d := getDuration([]int{5, 10}, 5); d != 10 {
		t.Fatalf("expected 10, got %d", d)
	}
	// Valid index returns correct element
	if d := getDuration([]int{5, 10, 15}, 1); d != 10 {
		t.Fatalf("expected 10, got %d", d)
	}
}

// TestWaitForAutoTuneComplete_Deadline tests deadline path in waitForAutoTuneComplete.
func TestWaitForAutoTuneComplete_Deadline(t *testing.T) {
	at := NewAutoTuner(nil)
	at.mu.Lock()
	at.state.Status = SweepStatusRunning
	at.mu.Unlock()

	tuner := NewRLHFTuner(at)
	tuner.pollInterval = 1 * time.Millisecond

	// Deadline already passed
	_, err := tuner.waitForAutoTuneComplete(context.Background(), time.Now().Add(-1*time.Second))
	if err == nil {
		t.Fatal("expected deadline exceeded error")
	}
	if !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestWaitForAutoTuneComplete_Error tests error status in waitForAutoTuneComplete.
func TestWaitForAutoTuneComplete_Error(t *testing.T) {
	at := NewAutoTuner(nil)
	at.mu.Lock()
	at.state.Status = SweepStatusError
	at.state.Error = "something broke"
	at.mu.Unlock()

	tuner := NewRLHFTuner(at)
	tuner.pollInterval = 1 * time.Millisecond

	_, err := tuner.waitForAutoTuneComplete(context.Background(), time.Now().Add(5*time.Second))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "something broke") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestWaitForAutoTuneComplete_ContextCancelled tests context cancellation.
func TestWaitForAutoTuneComplete_ContextCancelled(t *testing.T) {
	at := NewAutoTuner(nil)
	at.mu.Lock()
	at.state.Status = SweepStatusRunning
	at.mu.Unlock()

	tuner := NewRLHFTuner(at)
	tuner.pollInterval = 1 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := tuner.waitForAutoTuneComplete(ctx, time.Now().Add(5*time.Second))
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestRun_FullSuccessWithPersisterErrors tests the run() success path when
// persister SaveSweepStart and marshal both have coverage.
func TestRun_FullSuccessWithPersisterErrors(t *testing.T) {
	srv := sweepMockServer(t)
	client := sweepTestClient(t, srv)
	runner := NewRunner(client)
	at := NewAutoTuner(runner)
	at.SetGroundTruthScorer(func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		return 0.85, nil
	})

	tuner := NewRLHFTuner(at)
	tuner.SetPersister(&mockRLHFPersister{startErr: errForTest("start persist fail")})
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{
			SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap",
		},
	})
	tuner.SetRunCreator(&mockRunCreator{runID: "run-p"})
	tuner.SetSceneStore(&mockSceneSaver{})
	tuner.SetLabelQuerier(&mockLabelQuerier{
		total: 10, labelled: 10, byClass: map[string]int{"car": 10},
	})
	tuner.pollInterval = 10 * time.Millisecond
	tuner.sweepID = "test-persist-err"
	initRunState(tuner, 1)

	tuner.run(context.Background(), RLHFSweepRequest{
		SceneID:        "s1",
		NumRounds:      1,
		Params:         []SweepParam{{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05}},
		RoundDurations: []int{0},
		Iterations:     1,
		ValuesPerParam: 2,
		TopK:           2,
		Interval:       "1ms",
		SettleTime:     "1ms",
	})

	state := tuner.GetRLHFState()
	if state.Status != string(SweepStatusComplete) {
		t.Fatalf("expected completed even with persister error, got %s (error: %s)", state.Status, state.Error)
	}
}

// TestRun_SetOptimalParamsError tests run completion when scene store fails.
func TestRun_SetOptimalParamsError(t *testing.T) {
	srv := sweepMockServer(t)
	client := sweepTestClient(t, srv)
	runner := NewRunner(client)
	at := NewAutoTuner(runner)
	at.SetGroundTruthScorer(func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		return 0.85, nil
	})

	tuner := NewRLHFTuner(at)
	tuner.SetPersister(&mockRLHFPersister{})
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{
			SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap",
		},
	})
	tuner.SetRunCreator(&mockRunCreator{runID: "run-opt"})
	tuner.SetSceneStore(&mockSceneSaver{err: errForTest("save error")})
	tuner.SetLabelQuerier(&mockLabelQuerier{
		total: 10, labelled: 10, byClass: map[string]int{"car": 10},
	})
	tuner.pollInterval = 10 * time.Millisecond
	tuner.sweepID = "test-opt-err"
	initRunState(tuner, 1)

	tuner.run(context.Background(), RLHFSweepRequest{
		SceneID:        "s1",
		NumRounds:      1,
		Params:         []SweepParam{{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.05}},
		RoundDurations: []int{0},
		Iterations:     1,
		ValuesPerParam: 2,
		TopK:           2,
		Interval:       "1ms",
		SettleTime:     "1ms",
	})

	state := tuner.GetRLHFState()
	// Should still complete even if save fails (logged, not fatal)
	if state.Status != string(SweepStatusComplete) {
		t.Fatalf("expected completed, got %s (error: %s)", state.Status, state.Error)
	}
}
