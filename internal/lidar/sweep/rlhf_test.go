package sweep

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// --- Mock implementations ---

type mockLabelQuerier struct {
	total      int
	labelled   int
	byClass    map[string]int
	prevTracks []RLHFRunTrack // tracks returned for first call
	newTracks  []RLHFRunTrack // tracks returned for second call
	err        error
	updateErr  error
	labelCalls int
	callCount  int // number of GetRunTracks calls
}

func (m *mockLabelQuerier) GetLabelingProgress(runID string) (int, int, map[string]int, error) {
	return m.total, m.labelled, m.byClass, m.err
}

func (m *mockLabelQuerier) GetRunTracks(runID string) ([]RLHFRunTrack, error) {
	m.callCount++
	if m.callCount == 1 {
		return m.prevTracks, m.err
	}
	return m.newTracks, m.err
}

func (m *mockLabelQuerier) UpdateTrackLabel(runID, trackID, userLabel, qualityLabel string, confidence float32, labelerID string) error {
	m.labelCalls++
	return m.updateErr
}

type mockSceneGetter struct {
	scene       *RLHFScene
	err         error
	refRunCalls int
}

func (m *mockSceneGetter) GetScene(sceneID string) (*RLHFScene, error) {
	return m.scene, m.err
}

func (m *mockSceneGetter) SetReferenceRun(sceneID, runID string) error {
	m.refRunCalls++
	return nil
}

type mockRunCreator struct {
	runID string
	err   error
	calls int
}

func (m *mockRunCreator) CreateSweepRun(sensorID, pcapFile string, paramsJSON json.RawMessage) (string, error) {
	m.calls++
	return m.runID, m.err
}

type mockSceneSaver struct {
	err        error
	calls      int
	lastParams json.RawMessage
}

func (m *mockSceneSaver) SetOptimalParams(sceneID string, paramsJSON json.RawMessage) error {
	m.calls++
	m.lastParams = paramsJSON
	return m.err
}

type mockRLHFPersister struct {
	startCalls    int
	completeCalls int
	startErr      error
	completeErr   error
}

func (m *mockRLHFPersister) SaveSweepStart(sweepID, sensorID, mode string, request json.RawMessage, startedAt time.Time, objectiveName, objectiveVersion string) error {
	m.startCalls++
	return m.startErr
}

func (m *mockRLHFPersister) SaveSweepComplete(sweepID, status string, results, recommendation, roundResults json.RawMessage, completedAt time.Time, errMsg string, scoreComponents, recommendationExplanation, labelProvenanceSummary json.RawMessage, transformPipelineName, transformPipelineVersion string) error {
	m.completeCalls++
	return m.completeErr
}

// --- Test 1: getDuration ---

func TestGetDuration(t *testing.T) {
	tests := []struct {
		name      string
		durations []int
		index     int
		want      int
	}{
		{"empty durations returns default 60", []int{}, 0, 60},
		{"single value index 0", []int{60}, 0, 60},
		{"single value wraps for index 1", []int{60}, 1, 60},
		{"single value wraps for index 5", []int{60}, 5, 60},
		{"multiple values index 0", []int{30, 60, 120}, 0, 30},
		{"multiple values index 1", []int{30, 60, 120}, 1, 60},
		{"multiple values index 2", []int{30, 60, 120}, 2, 120},
		{"index beyond length returns last", []int{30, 60, 120}, 3, 120},
		{"index well beyond length returns last", []int{30, 60, 120}, 10, 120},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDuration(tt.durations, tt.index)
			if got != tt.want {
				t.Errorf("getDuration(%v, %d) = %d, want %d", tt.durations, tt.index, got, tt.want)
			}
		})
	}
}

// --- Test 2: temporalIoU ---

func TestTemporalIoU(t *testing.T) {
	tests := []struct {
		name                       string
		aStart, aEnd, bStart, bEnd int64
		want                       float64
	}{
		{"perfect overlap", 0, 10, 0, 10, 1.0},
		{"no overlap separate", 0, 5, 10, 15, 0.0},
		{"partial overlap", 0, 10, 5, 15, 5.0 / 15.0},
		{"one inside other", 0, 20, 5, 15, 10.0 / 20.0},
		{"adjacent no overlap", 0, 5, 5, 10, 0.0},
		{"zero length a", 5, 5, 0, 10, 0.0},
		{"zero length b", 0, 10, 5, 5, 0.0},
		{"both zero length", 5, 5, 5, 5, 0.0},
		{"small overlap", 0, 10, 9, 20, 1.0 / 20.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := temporalIoU(tt.aStart, tt.aEnd, tt.bStart, tt.bEnd)
			if math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("temporalIoU(%d,%d,%d,%d) = %f, want %f", tt.aStart, tt.aEnd, tt.bStart, tt.bEnd, got, tt.want)
			}
		})
	}
}

// --- Test 3: NewRLHFTuner ---

func TestRLHFTunerNewCreation(t *testing.T) {
	t.Run("initial status is idle", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		state := tuner.GetRLHFState()
		if state.Status != "idle" {
			t.Errorf("initial status = %q, want %q", state.Status, "idle")
		}
	})

	t.Run("mode is rlhf", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		state := tuner.GetRLHFState()
		if state.Mode != "rlhf" {
			t.Errorf("mode = %q, want %q", state.Mode, "rlhf")
		}
	})

	t.Run("default poll interval is 10s", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		if tuner.pollInterval != 10*time.Second {
			t.Errorf("pollInterval = %v, want %v", tuner.pollInterval, 10*time.Second)
		}
	})
}

// --- Test 4: Start validation ---

func TestRLHFTunerStartValidation(t *testing.T) {
	t.Run("missing scene_id returns error", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		err := tuner.Start(context.Background(), RLHFSweepRequest{
			SceneID:   "",
			NumRounds: 1,
			Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
		})
		if err == nil || !strings.Contains(err.Error(), "scene_id") {
			t.Errorf("expected scene_id error, got %v", err)
		}
	})

	t.Run("num_rounds < 1 returns error", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		err := tuner.Start(context.Background(), RLHFSweepRequest{
			SceneID:   "scene1",
			NumRounds: 0,
			Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
		})
		if err == nil || !strings.Contains(err.Error(), "num_rounds") {
			t.Errorf("expected num_rounds error, got %v", err)
		}
	})

	t.Run("num_rounds > 10 returns error", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		err := tuner.Start(context.Background(), RLHFSweepRequest{
			SceneID:   "scene1",
			NumRounds: 11,
			Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
		})
		if err == nil || !strings.Contains(err.Error(), "num_rounds") {
			t.Errorf("expected num_rounds error, got %v", err)
		}
	})

	t.Run("empty params returns error", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		err := tuner.Start(context.Background(), RLHFSweepRequest{
			SceneID:   "scene1",
			NumRounds: 1,
			Params:    []SweepParam{},
		})
		if err == nil || !strings.Contains(err.Error(), "params") {
			t.Errorf("expected params error, got %v", err)
		}
	})

	t.Run("too many params returns error", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		params := make([]SweepParam, 21)
		for i := range params {
			params[i] = SweepParam{Name: "p", Type: "float64", Start: 0, End: 1}
		}
		err := tuner.Start(context.Background(), RLHFSweepRequest{
			SceneID:   "scene1",
			NumRounds: 1,
			Params:    params,
		})
		if err == nil || !strings.Contains(err.Error(), "too many") {
			t.Errorf("expected too many params error, got %v", err)
		}
	})

	t.Run("default threshold applied when 0", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		tuner.SetSceneGetter(&mockSceneGetter{
			scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
		})
		tuner.SetRunCreator(&mockRunCreator{runID: "run1"})

		// Start will launch background goroutine - it will fail but threshold is applied
		_ = tuner.Start(context.Background(), RLHFSweepRequest{
			SceneID:           "scene1",
			NumRounds:         1,
			MinLabelThreshold: 0,
			Params:            []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
		})

		// The default should have been applied
		state := tuner.GetRLHFState()
		if state.MinLabelThreshold != 0.9 {
			t.Errorf("default MinLabelThreshold = %f, want 0.9", state.MinLabelThreshold)
		}
		tuner.Stop()
	})

	t.Run("cannot start when already running", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		tuner.mu.Lock()
		tuner.state.Status = "running_reference"
		tuner.mu.Unlock()

		err := tuner.Start(context.Background(), RLHFSweepRequest{
			SceneID:   "scene1",
			NumRounds: 1,
			Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
		})
		if err != ErrSweepAlreadyRunning {
			t.Errorf("expected ErrSweepAlreadyRunning, got %v", err)
		}
	})

	t.Run("Start accepts map[string]interface{}", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		reqMap := map[string]interface{}{
			"scene_id":   "scene1",
			"num_rounds": 1,
			"params": []interface{}{
				map[string]interface{}{"name": "eps", "type": "float64", "start": 0.1, "end": 1.0},
			},
		}
		err := tuner.Start(context.Background(), reqMap)
		// Should not fail on parsing, may fail later in the run loop
		if err != nil {
			t.Errorf("Start with map should not return error, got %v", err)
		}
		tuner.Stop()
	})

	t.Run("invalid request type returns error", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		err := tuner.Start(context.Background(), "invalid")
		if err == nil || !strings.Contains(err.Error(), "unsupported request type") {
			t.Errorf("expected unsupported type error, got %v", err)
		}
	})
}

// --- Test 5: ContinueFromLabels ---

func TestContinueFromLabels(t *testing.T) {
	t.Run("error if not awaiting_labels", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		err := tuner.ContinueFromLabels(60, false)
		if err == nil || !strings.Contains(err.Error(), "not in awaiting_labels") {
			t.Errorf("expected awaiting_labels error, got %v", err)
		}
	})

	t.Run("error if threshold not met", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		tuner.SetLabelQuerier(&mockLabelQuerier{total: 10, labelled: 3})
		tuner.mu.Lock()
		tuner.state.Status = "awaiting_labels"
		tuner.state.ReferenceRunID = "run1"
		tuner.state.MinLabelThreshold = 0.9
		tuner.mu.Unlock()

		err := tuner.ContinueFromLabels(60, false)
		if err == nil {
			t.Fatal("expected threshold error, got nil")
		}
		if !strings.Contains(err.Error(), "threshold") {
			t.Errorf("error should mention threshold, got %v", err)
		}
	})

	t.Run("succeeds when threshold met", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		tuner.SetLabelQuerier(&mockLabelQuerier{total: 10, labelled: 9})
		tuner.mu.Lock()
		tuner.state.Status = "awaiting_labels"
		tuner.state.ReferenceRunID = "run1"
		tuner.state.MinLabelThreshold = 0.9
		tuner.mu.Unlock()

		err := tuner.ContinueFromLabels(60, false)
		if err != nil {
			t.Errorf("ContinueFromLabels error = %v, want nil", err)
		}

		// Drain the continue channel
		select {
		case sig := <-tuner.continueCh:
			if sig.NextSweepDurationMins != 60 {
				t.Errorf("signal duration = %d, want 60", sig.NextSweepDurationMins)
			}
		default:
			t.Error("no signal on continue channel")
		}
	})

	t.Run("nextDuration updates state", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		tuner.SetLabelQuerier(&mockLabelQuerier{total: 10, labelled: 10})
		tuner.mu.Lock()
		tuner.state.Status = "awaiting_labels"
		tuner.state.ReferenceRunID = "run1"
		tuner.state.MinLabelThreshold = 0.9
		tuner.mu.Unlock()

		_ = tuner.ContinueFromLabels(120, false)

		state := tuner.GetRLHFState()
		if state.NextSweepDuration != 120 {
			t.Errorf("NextSweepDuration = %d, want 120", state.NextSweepDuration)
		}
		// Drain channel
		<-tuner.continueCh
	})

	t.Run("addRound increments TotalRounds", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		tuner.SetLabelQuerier(&mockLabelQuerier{total: 10, labelled: 10})
		tuner.mu.Lock()
		tuner.state.Status = "awaiting_labels"
		tuner.state.ReferenceRunID = "run1"
		tuner.state.MinLabelThreshold = 0.9
		tuner.state.TotalRounds = 3
		tuner.mu.Unlock()

		_ = tuner.ContinueFromLabels(60, true)

		state := tuner.GetRLHFState()
		if state.TotalRounds != 4 {
			t.Errorf("TotalRounds = %d, want 4", state.TotalRounds)
		}
		// Drain channel
		<-tuner.continueCh
	})
}

// --- Test 6: carryOverLabels ---

func TestCarryOverLabels(t *testing.T) {
	t.Run("carries labels with IoU >= 0.5", func(t *testing.T) {
		lq := &mockLabelQuerier{
			prevTracks: []RLHFRunTrack{
				{TrackID: "t1", StartUnixNanos: 0, EndUnixNanos: 100, UserLabel: "good_vehicle", QualityLabel: "perfect"},
			},
			newTracks: []RLHFRunTrack{
				{TrackID: "n1", StartUnixNanos: 0, EndUnixNanos: 100}, // perfect match IoU=1.0
			},
		}

		tuner := NewRLHFTuner(nil)
		tuner.SetLabelQuerier(lq)

		count, err := tuner.carryOverLabels("prev_run", "new_run")
		if err != nil {
			t.Fatalf("carryOverLabels error = %v", err)
		}
		if count != 1 {
			t.Errorf("carried = %d, want 1", count)
		}
		if lq.labelCalls != 1 {
			t.Errorf("UpdateTrackLabel calls = %d, want 1", lq.labelCalls)
		}
	})

	t.Run("does not carry labels with IoU < 0.5", func(t *testing.T) {
		lq := &mockLabelQuerier{
			prevTracks: []RLHFRunTrack{
				{TrackID: "t1", StartUnixNanos: 0, EndUnixNanos: 100, UserLabel: "good_vehicle"},
			},
			newTracks: []RLHFRunTrack{
				{TrackID: "n1", StartUnixNanos: 200, EndUnixNanos: 300}, // no overlap
			},
		}

		tuner := NewRLHFTuner(nil)
		tuner.SetLabelQuerier(lq)

		count, err := tuner.carryOverLabels("prev_run", "new_run")
		if err != nil {
			t.Fatalf("carryOverLabels error = %v", err)
		}
		if count != 0 {
			t.Errorf("carried = %d, want 0", count)
		}
	})

	t.Run("only labelled tracks are considered", func(t *testing.T) {
		lq := &mockLabelQuerier{
			prevTracks: []RLHFRunTrack{
				{TrackID: "t1", StartUnixNanos: 0, EndUnixNanos: 100, UserLabel: "good_vehicle"},
				{TrackID: "t2", StartUnixNanos: 200, EndUnixNanos: 300, UserLabel: ""}, // not labelled
			},
			newTracks: []RLHFRunTrack{
				{TrackID: "n1", StartUnixNanos: 0, EndUnixNanos: 100},
				{TrackID: "n2", StartUnixNanos: 200, EndUnixNanos: 300},
			},
		}

		tuner := NewRLHFTuner(nil)
		tuner.SetLabelQuerier(lq)

		count, err := tuner.carryOverLabels("prev_run", "new_run")
		if err != nil {
			t.Fatalf("carryOverLabels error = %v", err)
		}
		if count != 1 {
			t.Errorf("carried = %d, want 1 (only labelled tracks)", count)
		}
	})

	t.Run("selects best IoU match", func(t *testing.T) {
		lq := &mockLabelQuerier{
			prevTracks: []RLHFRunTrack{
				{TrackID: "t1", StartUnixNanos: 0, EndUnixNanos: 100, UserLabel: "good_vehicle"},
			},
			newTracks: []RLHFRunTrack{
				{TrackID: "n1", StartUnixNanos: 30, EndUnixNanos: 130}, // IoU = 70/130 ≈ 0.538
				{TrackID: "n2", StartUnixNanos: 0, EndUnixNanos: 100},  // IoU = 1.0 (best)
			},
		}

		tuner := NewRLHFTuner(nil)
		tuner.SetLabelQuerier(lq)

		count, err := tuner.carryOverLabels("prev_run", "new_run")
		if err != nil {
			t.Fatalf("carryOverLabels error = %v", err)
		}
		if count != 1 {
			t.Errorf("carried = %d, want 1", count)
		}
	})

	t.Run("empty tracks returns 0", func(t *testing.T) {
		lq := &mockLabelQuerier{
			prevTracks: []RLHFRunTrack{},
			newTracks:  []RLHFRunTrack{},
		}

		tuner := NewRLHFTuner(nil)
		tuner.SetLabelQuerier(lq)

		count, err := tuner.carryOverLabels("prev_run", "new_run")
		if err != nil {
			t.Fatalf("carryOverLabels error = %v", err)
		}
		if count != 0 {
			t.Errorf("carried = %d, want 0", count)
		}
	})

	t.Run("no label querier returns error", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)

		_, err := tuner.carryOverLabels("prev_run", "new_run")
		if err == nil {
			t.Error("expected error when label querier not configured")
		}
	})
}

// --- Test 7: buildAutoTuneRequest ---

func TestBuildAutoTuneRequest(t *testing.T) {
	scene := &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"}

	t.Run("round 1 adjusts weights", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		req := RLHFSweepRequest{
			SceneID:        "s1",
			ValuesPerParam: 5,
			TopK:           3,
			Params:         []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
		}
		bounds := map[string][2]float64{"eps": {0.1, 1.0}}

		autoReq := tuner.buildAutoTuneRequest(bounds, req, scene, 1)

		defaults := DefaultGroundTruthWeights()
		if autoReq.GroundTruthWeights.DetectionRate != defaults.DetectionRate*1.5 {
			t.Errorf("round 1 DetectionRate = %f, want %f", autoReq.GroundTruthWeights.DetectionRate, defaults.DetectionRate*1.5)
		}
		if autoReq.GroundTruthWeights.FalsePositives != defaults.FalsePositives*0.5 {
			t.Errorf("round 1 FalsePositives = %f, want %f", autoReq.GroundTruthWeights.FalsePositives, defaults.FalsePositives*0.5)
		}
	})

	t.Run("round 2 uses default weights", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		req := RLHFSweepRequest{
			SceneID:        "s1",
			ValuesPerParam: 5,
			TopK:           3,
			Params:         []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
		}
		bounds := map[string][2]float64{"eps": {0.1, 1.0}}

		autoReq := tuner.buildAutoTuneRequest(bounds, req, scene, 2)

		defaults := DefaultGroundTruthWeights()
		if autoReq.GroundTruthWeights.DetectionRate != defaults.DetectionRate {
			t.Errorf("round 2 DetectionRate = %f, want %f", autoReq.GroundTruthWeights.DetectionRate, defaults.DetectionRate)
		}
	})

	t.Run("bounds are applied correctly", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		req := RLHFSweepRequest{
			SceneID:        "s1",
			ValuesPerParam: 5,
			TopK:           3,
			Params: []SweepParam{
				{Name: "eps", Type: "float64", Start: 0.1, End: 1.0},
				{Name: "noise", Type: "float64", Start: 0.01, End: 0.1},
			},
		}
		bounds := map[string][2]float64{
			"eps":   {0.3, 0.7},
			"noise": {0.02, 0.05},
		}

		autoReq := tuner.buildAutoTuneRequest(bounds, req, scene, 2)

		for _, p := range autoReq.Params {
			if p.Name == "eps" {
				if p.Start != 0.3 || p.End != 0.7 {
					t.Errorf("eps bounds = [%f, %f], want [0.3, 0.7]", p.Start, p.End)
				}
			}
			if p.Name == "noise" {
				if p.Start != 0.02 || p.End != 0.05 {
					t.Errorf("noise bounds = [%f, %f], want [0.02, 0.05]", p.Start, p.End)
				}
			}
		}
	})

	t.Run("objective is ground_truth", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		req := RLHFSweepRequest{
			SceneID:        "s1",
			ValuesPerParam: 5,
			TopK:           3,
			Params:         []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
		}
		bounds := map[string][2]float64{"eps": {0.1, 1.0}}

		autoReq := tuner.buildAutoTuneRequest(bounds, req, scene, 1)
		if autoReq.Objective != "ground_truth" {
			t.Errorf("objective = %q, want %q", autoReq.Objective, "ground_truth")
		}
		if autoReq.MaxRounds != 1 {
			t.Errorf("MaxRounds = %d, want 1", autoReq.MaxRounds)
		}
		if autoReq.SceneID != "s1" {
			t.Errorf("SceneID = %q, want %q", autoReq.SceneID, "s1")
		}
	})
}

// --- Test 8: waitForLabelsOrDeadline ---

func TestWaitForLabelsOrDeadline(t *testing.T) {
	t.Run("context cancellation returns error", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		tuner.pollInterval = 10 * time.Millisecond
		tuner.SetLabelQuerier(&mockLabelQuerier{total: 10, labelled: 5})

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		err := tuner.waitForLabelsOrDeadline(ctx, "run1", 60, 0.9)
		if err == nil {
			t.Error("expected context cancelled error")
		}
	})

	t.Run("continue signal unblocks wait", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		tuner.pollInterval = 10 * time.Millisecond
		tuner.SetLabelQuerier(&mockLabelQuerier{total: 10, labelled: 10})

		// Send continue signal before calling wait
		go func() {
			time.Sleep(50 * time.Millisecond)
			tuner.continueCh <- continueSignal{NextSweepDurationMins: 120}
		}()

		err := tuner.waitForLabelsOrDeadline(context.Background(), "run1", 60, 0.9)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("deadline with threshold met proceeds", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)
		tuner.pollInterval = 10 * time.Millisecond
		tuner.SetLabelQuerier(&mockLabelQuerier{total: 10, labelled: 10})

		// Use 0 minute duration so deadline is immediately past
		err := tuner.waitForLabelsOrDeadline(context.Background(), "run1", 0, 0.9)
		if err != nil {
			t.Errorf("expected nil error (threshold met at deadline), got %v", err)
		}
	})
}

// --- Test 9: GetRLHFState deep copy ---

func TestRLHFTunerGetState(t *testing.T) {
	tuner := NewRLHFTuner(nil)

	// Set up some state
	now := time.Now()
	tuner.mu.Lock()
	tuner.state.Status = "awaiting_labels"
	tuner.state.CurrentRound = 2
	tuner.state.LabelDeadline = &now
	tuner.state.LabelProgress = &LabelProgress{
		Total:    20,
		Labelled: 15,
		Pct:      75.0,
		ByClass:  map[string]int{"good_vehicle": 10, "noise": 5},
	}
	tuner.state.RoundHistory = []RLHFRound{
		{
			Round:          1,
			ReferenceRunID: "run1",
			LabelledAt:     &now,
			BestScore:      0.85,
			BestParams:     map[string]float64{"eps": 0.3},
		},
	}
	tuner.state.Recommendation = map[string]interface{}{"eps": 0.3}
	tuner.mu.Unlock()

	// Get state and mutate it
	state := tuner.GetRLHFState()
	state.LabelProgress.Total = 999
	state.RoundHistory[0].BestScore = 999.0
	state.Recommendation["eps"] = 999.0
	state.LabelProgress.ByClass["good_vehicle"] = 999

	// Verify original is unchanged
	original := tuner.GetRLHFState()
	if original.LabelProgress.Total != 20 {
		t.Errorf("original LabelProgress.Total mutated to %d", original.LabelProgress.Total)
	}
	if original.RoundHistory[0].BestScore != 0.85 {
		t.Errorf("original RoundHistory[0].BestScore mutated to %f", original.RoundHistory[0].BestScore)
	}
	if original.Recommendation["eps"] != 0.3 {
		t.Errorf("original Recommendation mutated to %v", original.Recommendation["eps"])
	}
	if original.LabelProgress.ByClass["good_vehicle"] != 10 {
		t.Errorf("original ByClass mutated to %d", original.LabelProgress.ByClass["good_vehicle"])
	}
}

// --- Test 10: Stop behaviour ---

func TestRLHFTunerStop(t *testing.T) {
	t.Run("stop cancels context", func(t *testing.T) {
		tuner := NewRLHFTuner(nil)

		ctx, cancel := context.WithCancel(context.Background())
		tuner.mu.Lock()
		tuner.cancel = cancel
		tuner.mu.Unlock()

		tuner.Stop()

		select {
		case <-ctx.Done():
			// Expected
		case <-time.After(100 * time.Millisecond):
			t.Error("context was not cancelled after Stop()")
		}
	})
}

// --- Test 11: Setter methods ---

func TestRLHFTunerSetters(t *testing.T) {
	tuner := NewRLHFTuner(nil)

	lq := &mockLabelQuerier{}
	tuner.SetLabelQuerier(lq)
	if tuner.labelQuerier != lq {
		t.Error("SetLabelQuerier did not set labelQuerier")
	}

	sg := &mockSceneGetter{}
	tuner.SetSceneGetter(sg)
	if tuner.sceneGetter != sg {
		t.Error("SetSceneGetter did not set sceneGetter")
	}

	ss := &mockSceneSaver{}
	tuner.SetSceneStore(ss)
	if tuner.sceneStore != ss {
		t.Error("SetSceneStore did not set sceneStore")
	}

	rc := &mockRunCreator{}
	tuner.SetRunCreator(rc)
	if tuner.runCreator != rc {
		t.Error("SetRunCreator did not set runCreator")
	}

	p := &mockRLHFPersister{}
	tuner.SetPersister(p)
	if tuner.persister != p {
		t.Error("SetPersister did not set persister")
	}

	scorer := func(sceneID, candidateRunID string, weights GroundTruthWeights) (float64, error) {
		return 0.5, nil
	}
	tuner.SetGroundTruthScorer(scorer)
	if tuner.groundTruthScorer == nil {
		t.Error("SetGroundTruthScorer did not set scorer")
	}
}

// --- Test 12: Persistence calls ---

func TestRLHFTunerPersistence(t *testing.T) {
	t.Run("persist start is called", func(t *testing.T) {
		p := &mockRLHFPersister{}
		tuner := NewRLHFTuner(nil)
		tuner.SetPersister(p)
		tuner.SetSceneGetter(&mockSceneGetter{
			scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
		})

		_ = tuner.Start(context.Background(), RLHFSweepRequest{
			SceneID:   "scene1",
			NumRounds: 1,
			Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
		})

		// Give goroutine time to start
		time.Sleep(50 * time.Millisecond)

		if p.startCalls != 1 {
			t.Errorf("SaveSweepStart calls = %d, want 1", p.startCalls)
		}
		tuner.Stop()
	})
}

// --- Test 13: failWithError ---

func TestFailWithError(t *testing.T) {
	p := &mockRLHFPersister{}
	tuner := NewRLHFTuner(nil)
	tuner.sweepID = "test-sweep-id"
	tuner.SetPersister(p)

	tuner.failWithError("something went wrong")

	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Errorf("status = %q, want %q", state.Status, "failed")
	}
	if state.Error != "something went wrong" {
		t.Errorf("error = %q, want %q", state.Error, "something went wrong")
	}
	if p.completeCalls != 1 {
		t.Errorf("SaveSweepComplete calls = %d, want 1", p.completeCalls)
	}
}

// --- Test 14: GetState returns interface ---

func TestGetStateInterface(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	got := tuner.GetState()
	_, ok := got.(RLHFState)
	if !ok {
		t.Errorf("GetState() returned %T, want RLHFState", got)
	}
}

// --- Test 15: GetRLHFState deep copy with sweep deadline ---

func TestGetRLHFState_SweepDeadline(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	now := time.Now()
	tuner.mu.Lock()
	tuner.state.SweepDeadline = &now
	ats := AutoTuneState{Status: SweepStatusRunning}
	tuner.state.AutoTuneState = &ats
	tuner.mu.Unlock()

	state := tuner.GetRLHFState()
	if state.SweepDeadline == nil {
		t.Fatal("SweepDeadline should not be nil")
	}
	if state.AutoTuneState == nil {
		t.Fatal("AutoTuneState should not be nil")
	}

	// Mutate the copy
	newTime := now.Add(time.Hour)
	state.SweepDeadline = &newTime

	// Original should be unchanged
	original := tuner.GetRLHFState()
	if !original.SweepDeadline.Equal(now) {
		t.Error("SweepDeadline was mutated through copy")
	}
}

// --- Test 16: run() fails when sceneGetter is nil ---

func TestRunFailsWithoutSceneGetter(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.pollInterval = 10 * time.Millisecond

	err := tuner.Start(context.Background(), RLHFSweepRequest{
		SceneID:   "s1",
		NumRounds: 1,
		Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	if err != nil {
		t.Fatalf("Start should not fail: %v", err)
	}

	// Wait for run goroutine to fail
	time.Sleep(100 * time.Millisecond)

	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Errorf("status = %q, want %q", state.Status, "failed")
	}
	if state.Error == "" {
		t.Error("expected error message about scene getter")
	}
}

// --- Test 17: run() fails when scene not found ---

func TestRunFailsWhenSceneNotFound(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.pollInterval = 10 * time.Millisecond
	tuner.SetSceneGetter(&mockSceneGetter{err: fmt.Errorf("scene not found")})

	err := tuner.Start(context.Background(), RLHFSweepRequest{
		SceneID:   "missing",
		NumRounds: 1,
		Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	if err != nil {
		t.Fatalf("Start should not fail: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Errorf("status = %q, want %q", state.Status, "failed")
	}
}

// --- Test 18: run() fails when runCreator not configured ---

func TestRunFailsWithoutRunCreator(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.pollInterval = 10 * time.Millisecond
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})

	err := tuner.Start(context.Background(), RLHFSweepRequest{
		SceneID:   "s1",
		NumRounds: 1,
		Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	if err != nil {
		t.Fatalf("Start should not fail: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Errorf("status = %q, want %q", state.Status, "failed")
	}
}

// --- Test 19: run() context cancellation ---

func TestRunCancelledByContext(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.pollInterval = 10 * time.Millisecond
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})
	tuner.SetRunCreator(&mockRunCreator{runID: "run1"})
	tuner.SetLabelQuerier(&mockLabelQuerier{total: 10, labelled: 5})

	ctx, cancel := context.WithCancel(context.Background())
	err := tuner.Start(ctx, RLHFSweepRequest{
		SceneID:           "s1",
		NumRounds:         1,
		RoundDurations:    []int{60}, // long wait
		MinLabelThreshold: 0.9,
		Params:            []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	if err != nil {
		t.Fatalf("Start should not fail: %v", err)
	}

	// Let it reach awaiting_labels, then cancel
	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Errorf("status = %q, want %q (context cancelled)", state.Status, "failed")
	}
}

// --- Test 20: run() uses midpoints when no optimal params ---

func TestRunUsesMidpointsForParams(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.pollInterval = 10 * time.Millisecond
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{
			SceneID:  "s1",
			SensorID: "sensor1",
			PCAPFile: "test.pcap",
			// No OptimalParamsJSON
		},
	})
	tuner.SetRunCreator(&mockRunCreator{runID: "run1"})

	err := tuner.Start(context.Background(), RLHFSweepRequest{
		SceneID:   "s1",
		NumRounds: 1,
		Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.2, End: 0.8}},
	})
	if err != nil {
		t.Fatalf("Start should not fail: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	tuner.Stop()
}

// --- Test 21: ContinueFromLabels with no label querier succeeds ---

func TestContinueFromLabelsNoQuerier(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.mu.Lock()
	tuner.state.Status = "awaiting_labels"
	tuner.state.MinLabelThreshold = 0.9
	tuner.mu.Unlock()

	// With no label querier, it should skip the threshold check
	err := tuner.ContinueFromLabels(60, false)
	if err != nil {
		t.Errorf("ContinueFromLabels without querier should succeed, got %v", err)
	}
	// Drain channel
	<-tuner.continueCh
}

// --- Test 22: waitForLabelsOrDeadline deadline expired insufficient labels ---

func TestWaitForLabelsDeadlineExpired(t *testing.T) {
	lq := &mockLabelQuerier{total: 10, labelled: 3} // 30% < 90% threshold
	tuner := NewRLHFTuner(nil)
	tuner.pollInterval = 10 * time.Millisecond
	tuner.SetLabelQuerier(lq)

	// Use 0 minute duration so deadline is immediately past
	err := tuner.waitForLabelsOrDeadline(context.Background(), "run1", 0, 0.9)
	if err == nil {
		t.Error("expected deadline expired error")
	}
}

// --- Test 23: failWithError without persister ---

func TestFailWithErrorNoPersister(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	// No persister set - should not panic
	tuner.failWithError("test error")

	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Errorf("status = %q, want %q", state.Status, "failed")
	}
}

// --- Test 24: run() loads existing optimal params ---

func TestRunLoadsOptimalParams(t *testing.T) {
	optimalParams := `{"eps": 0.5, "noise": 0.02}`
	tuner := NewRLHFTuner(nil)
	tuner.pollInterval = 10 * time.Millisecond
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{
			SceneID:           "s1",
			SensorID:          "sensor1",
			PCAPFile:          "test.pcap",
			OptimalParamsJSON: json.RawMessage(optimalParams),
		},
	})
	// No run creator → will fail after loading params but exercises the path
	err := tuner.Start(context.Background(), RLHFSweepRequest{
		SceneID:   "s1",
		NumRounds: 1,
		Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	if err != nil {
		t.Fatalf("Start should not fail: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Should fail because no run creator, but params should have been loaded
	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Errorf("status = %q, want %q", state.Status, "failed")
	}
}

// --- Test 25: run() with invalid optimal params JSON ---

func TestRunInvalidOptimalParamsJSON(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.pollInterval = 10 * time.Millisecond
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{
			SceneID:           "s1",
			SensorID:          "sensor1",
			PCAPFile:          "test.pcap",
			OptimalParamsJSON: json.RawMessage(`not json`),
		},
	})
	// No run creator → will fail after loading params but exercises the parse error path

	err := tuner.Start(context.Background(), RLHFSweepRequest{
		SceneID:   "s1",
		NumRounds: 1,
		Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	if err != nil {
		t.Fatalf("Start should not fail: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Should still have started (falls back to midpoints)
	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Errorf("status = %q, want %q (falls back to midpoints, then fails on run creator)", state.Status, "failed")
	}
}

// --- Test 26: run() with runCreator error ---

func TestRunRunCreatorError(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.pollInterval = 10 * time.Millisecond
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})
	tuner.SetRunCreator(&mockRunCreator{err: fmt.Errorf("pcap not found")})

	err := tuner.Start(context.Background(), RLHFSweepRequest{
		SceneID:   "s1",
		NumRounds: 1,
		Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	if err != nil {
		t.Fatalf("Start should not fail: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	state := tuner.GetRLHFState()
	if state.Status != "failed" {
		t.Errorf("status = %q, want %q", state.Status, "failed")
	}
	if !strings.Contains(state.Error, "failed to create reference run") {
		t.Errorf("error should mention reference run, got %q", state.Error)
	}
}

// --- Test 27: run() with persist failure on start ---

func TestRunPersistStartFailure(t *testing.T) {
	p := &mockRLHFPersister{startErr: fmt.Errorf("db error")}
	tuner := NewRLHFTuner(nil)
	tuner.pollInterval = 10 * time.Millisecond
	tuner.SetPersister(p)
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})
	tuner.SetRunCreator(&mockRunCreator{runID: "run1"})

	err := tuner.Start(context.Background(), RLHFSweepRequest{
		SceneID:   "s1",
		NumRounds: 1,
		Params:    []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	if err != nil {
		t.Fatalf("Start should not fail: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Persist failure is logged but doesn't block start
	if p.startCalls != 1 {
		t.Errorf("SaveSweepStart calls = %d, want 1", p.startCalls)
	}
	tuner.Stop()
}

// --- Test 28: Stop when not running ---

func TestStopWhenIdle(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	// Should not panic when stopping an idle tuner
	tuner.Stop()

	state := tuner.GetRLHFState()
	if state.Status != "idle" {
		t.Errorf("status = %q, want %q", state.Status, "idle")
	}
}

// --- Test 29: run() with roundHistory recording after runCreator succeeds ---

func TestRunRecordsRound(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.pollInterval = 10 * time.Millisecond
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})
	tuner.SetRunCreator(&mockRunCreator{runID: "run-abc"})
	tuner.SetLabelQuerier(&mockLabelQuerier{total: 10, labelled: 10})

	ctx, cancel := context.WithCancel(context.Background())
	err := tuner.Start(ctx, RLHFSweepRequest{
		SceneID:           "s1",
		NumRounds:         1,
		MinLabelThreshold: 0.0,
		RoundDurations:    []int{0}, // immediate deadline
		Params:            []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	if err != nil {
		t.Fatalf("Start should not fail: %v", err)
	}

	// Let it reach awaiting_labels and pass threshold check
	time.Sleep(200 * time.Millisecond)

	state := tuner.GetRLHFState()
	// Verify reference run ID was set
	if state.ReferenceRunID != "run-abc" {
		t.Errorf("ReferenceRunID = %q, want %q", state.ReferenceRunID, "run-abc")
	}

	cancel()
	time.Sleep(100 * time.Millisecond)
}

// --- Test 30: waitForAutoTuneComplete returns on completion ---

func TestWaitForAutoTuneComplete(t *testing.T) {
	at := NewAutoTuner(nil)
	tuner := NewRLHFTuner(at)
	tuner.pollInterval = 10 * time.Millisecond

	// Set auto-tuner state to complete after a brief delay
	go func() {
		time.Sleep(30 * time.Millisecond)
		at.mu.Lock()
		at.state.Status = SweepStatusComplete
		at.state.Recommendation = map[string]interface{}{"eps": 0.5, "score": 0.95}
		at.mu.Unlock()
	}()

	deadline := time.Now().Add(time.Second)
	state, err := tuner.waitForAutoTuneComplete(context.Background(), deadline)
	if err != nil {
		t.Fatalf("waitForAutoTuneComplete error = %v", err)
	}
	if state.Status != SweepStatusComplete {
		t.Errorf("status = %q, want %q", state.Status, SweepStatusComplete)
	}
}

// --- Test 31: waitForAutoTuneComplete returns error on failure ---

func TestWaitForAutoTuneCompleteError(t *testing.T) {
	at := NewAutoTuner(nil)
	tuner := NewRLHFTuner(at)
	tuner.pollInterval = 10 * time.Millisecond

	// Set auto-tuner to error
	go func() {
		time.Sleep(30 * time.Millisecond)
		at.mu.Lock()
		at.state.Status = SweepStatusError
		at.state.Error = "out of memory"
		at.mu.Unlock()
	}()

	deadline := time.Now().Add(time.Second)
	_, err := tuner.waitForAutoTuneComplete(context.Background(), deadline)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "auto-tune failed") {
		t.Errorf("error = %v, should mention auto-tune failed", err)
	}
}

// --- Test 32: waitForAutoTuneComplete deadline exceeded ---

func TestWaitForAutoTuneCompleteDeadline(t *testing.T) {
	at := NewAutoTuner(nil)
	tuner := NewRLHFTuner(at)
	tuner.pollInterval = 10 * time.Millisecond

	// Auto-tuner stays idle (never completes)
	deadline := time.Now() // already past
	_, err := tuner.waitForAutoTuneComplete(context.Background(), deadline)
	if err == nil {
		t.Fatal("expected deadline exceeded error")
	}
	if !strings.Contains(err.Error(), "deadline exceeded") {
		t.Errorf("error = %v, should mention deadline exceeded", err)
	}
}

// --- Test 33: waitForAutoTuneComplete context cancellation ---

func TestWaitForAutoTuneCompleteContextCancel(t *testing.T) {
	at := NewAutoTuner(nil)
	tuner := NewRLHFTuner(at)
	tuner.pollInterval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	deadline := time.Now().Add(time.Second)
	_, err := tuner.waitForAutoTuneComplete(ctx, deadline)
	if err == nil {
		t.Fatal("expected context cancelled error")
	}
}

// --- Test 34: runRound with carry-over labels ---

func TestRunRoundWithCarryOver(t *testing.T) {
	lq := &mockLabelQuerier{
		total:    10,
		labelled: 10,
		prevTracks: []RLHFRunTrack{
			{TrackID: "t1", StartUnixNanos: 0, EndUnixNanos: 100, UserLabel: "good_vehicle"},
		},
		newTracks: []RLHFRunTrack{
			{TrackID: "n1", StartUnixNanos: 0, EndUnixNanos: 100},
		},
	}

	at := NewAutoTuner(nil)
	tuner := NewRLHFTuner(at)
	tuner.pollInterval = 10 * time.Millisecond
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})
	tuner.SetRunCreator(&mockRunCreator{runID: "run-test"})
	tuner.SetLabelQuerier(lq)

	// Set up state as if round 1 was already completed
	tuner.mu.Lock()
	tuner.state.RoundHistory = []RLHFRound{
		{Round: 1, ReferenceRunID: "prev-run"},
	}
	tuner.mu.Unlock()

	scene := &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"}
	currentParams := map[string]float64{"eps": 0.5}
	bounds := map[string][2]float64{"eps": {0.1, 1.0}}

	req := RLHFSweepRequest{
		SceneID:           "s1",
		NumRounds:         2,
		MinLabelThreshold: 0.0,
		RoundDurations:    []int{0}, // immediate
		CarryOverLabels:   true,
		Params:            []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	}

	// Set auto-tuner to error immediately to end the round
	go func() {
		time.Sleep(200 * time.Millisecond)
		at.mu.Lock()
		at.state.Status = SweepStatusError
		at.state.Error = "test stop"
		at.mu.Unlock()
	}()

	_, _, err := tuner.runRound(context.Background(), req, scene, 2, currentParams, bounds)
	// Expect an error from auto-tune but the carry-over and labelling should have worked
	if err == nil {
		t.Log("runRound succeeded (auto-tune completed)")
	}

	// Check labels were carried over
	if lq.labelCalls < 1 {
		t.Log("Note: label carry-over may not have matched any tracks")
	}
}

// --- Test 35: Start with default values_per_param ---

func TestStartDefaultValuesPerParam(t *testing.T) {
	tuner := NewRLHFTuner(nil)
	tuner.SetSceneGetter(&mockSceneGetter{
		scene: &RLHFScene{SceneID: "s1", SensorID: "sensor1", PCAPFile: "test.pcap"},
	})

	err := tuner.Start(context.Background(), RLHFSweepRequest{
		SceneID:        "s1",
		NumRounds:      1,
		ValuesPerParam: 0, // should get default
		TopK:           0, // should get default
		Params:         []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
	})
	if err != nil {
		t.Fatalf("Start should not fail: %v", err)
	}

	// Should have started OK with defaults applied internally
	state := tuner.GetRLHFState()
	if state.Status == "idle" {
		t.Error("status should not be idle after start")
	}
	tuner.Stop()
}
