package sweep

import (
"context"
"encoding/json"
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
Params:         []SweepParam{{Name: "eps", Type: "float64", Start: 0.1, End: 1.0}},
RoundDurations: []int{0},
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
