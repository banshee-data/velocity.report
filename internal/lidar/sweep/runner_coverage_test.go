package sweep

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/monitor"
)

// --- Runner accessor tests ---

func TestRunner_SetPersister(t *testing.T) {
	r := NewRunner(nil)
	mp := &mockPersister{}
	r.SetPersister(mp)
	if r.persister != mp {
		t.Error("SetPersister did not set persister")
	}
}

func TestRunner_GetSweepID_Empty(t *testing.T) {
	r := NewRunner(nil)
	if id := r.GetSweepID(); id != "" {
		t.Errorf("GetSweepID = %q, want empty", id)
	}
}

func TestRunner_AddWarning(t *testing.T) {
	r := NewRunner(nil)
	r.addWarning("test warning 1")
	r.addWarning("test warning 2")
	state := r.GetSweepState()
	if len(state.Warnings) != 2 {
		t.Errorf("warnings count = %d, want 2", len(state.Warnings))
	}
	if state.Warnings[0] != "test warning 1" {
		t.Errorf("warning[0] = %q, want 'test warning 1'", state.Warnings[0])
	}
}

func TestRunner_GetState_ReturnsInterface(t *testing.T) {
	r := NewRunner(nil)
	state := r.GetState()
	ss, ok := state.(SweepState)
	if !ok {
		t.Fatal("GetState did not return SweepState")
	}
	if ss.Status != SweepStatusIdle {
		t.Errorf("status = %q, want idle", ss.Status)
	}
}

func TestRunner_Stop_NilCancel(t *testing.T) {
	r := NewRunner(nil)
	r.Stop() // should not panic
}

func TestRunner_Stop_WithCancel(t *testing.T) {
	r := NewRunner(nil)
	_, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.Stop() // should not panic
}

// --- Start validation tests ---

func TestRunner_Start_InvalidRequestType(t *testing.T) {
	r := NewRunner(nil)
	err := r.Start(context.Background(), 42)
	if err == nil {
		t.Error("expected error for invalid request type")
	}
}

func TestRunner_Start_MapRequest(t *testing.T) {
	r := NewRunner(nil)
	m := map[string]interface{}{
		"mode": "multi",
	}
	err := r.Start(context.Background(), m)
	// Should fail on client nil check
	if err == nil {
		t.Error("expected error for nil client")
	}
}

func TestRunner_Start_TypedRequest(t *testing.T) {
	r := NewRunner(nil)
	err := r.Start(context.Background(), SweepRequest{Mode: "multi"})
	if err == nil {
		t.Error("expected error for nil client")
	}
}

func TestRunner_Start_NilClient(t *testing.T) {
	r := NewRunner(nil)
	err := r.start(context.Background(), SweepRequest{Mode: "multi"})
	if err == nil {
		t.Error("expected error for nil client")
	}
}

func TestRunner_Start_NilContext(t *testing.T) {
	r := NewRunner(testClient(t))
	//nolint:staticcheck
	err := r.start(nil, SweepRequest{
		Mode:        "multi",
		NoiseValues: []float64{0.01},
	})
	// Should start successfully with nil context (defaults to Background)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	r.Stop()
	time.Sleep(50 * time.Millisecond)
}

func TestRunner_Start_InvalidInterval(t *testing.T) {
	client := &monitor.Client{BaseURL: "http://localhost:8080", SensorID: "test"}
	r := NewRunner(client)
	err := r.start(context.Background(), SweepRequest{
		Mode:        "multi",
		NoiseValues: []float64{0.01},
		Interval:    "not-a-duration",
	})
	if err == nil {
		t.Error("expected error for invalid interval")
	}
}

func TestRunner_Start_InvalidSettleTime(t *testing.T) {
	client := &monitor.Client{BaseURL: "http://localhost:8080", SensorID: "test"}
	r := NewRunner(client)
	err := r.start(context.Background(), SweepRequest{
		Mode:        "multi",
		NoiseValues: []float64{0.01},
		SettleTime:  "not-a-duration",
	})
	if err == nil {
		t.Error("expected error for invalid settle time")
	}
}

func TestRunner_Start_ExcessiveIterations(t *testing.T) {
	client := &monitor.Client{BaseURL: "http://localhost:8080", SensorID: "test"}
	r := NewRunner(client)
	err := r.start(context.Background(), SweepRequest{
		Mode:        "multi",
		NoiseValues: []float64{0.01},
		Iterations:  501,
	})
	if err == nil {
		t.Error("expected error for iterations > 500")
	}
}

func TestRunner_Start_UnsupportedMode(t *testing.T) {
	client := &monitor.Client{BaseURL: "http://localhost:8080", SensorID: "test"}
	r := NewRunner(client)
	err := r.start(context.Background(), SweepRequest{
		Mode: "unknown",
	})
	if err == nil {
		t.Error("expected error for unsupported mode")
	}
}

func TestRunner_Start_DefaultCombinations(t *testing.T) {
	r := NewRunner(testClient(t))
	err := r.start(context.Background(), SweepRequest{
		Mode: "multi",
		// No values provided — should default to built-in ranges
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	r.Stop()
	time.Sleep(50 * time.Millisecond)
}

func TestRunner_Start_AlreadyRunning(t *testing.T) {
	client := &monitor.Client{BaseURL: "http://localhost:8080", SensorID: "test"}
	r := NewRunner(client)
	r.mu.Lock()
	r.state.Status = SweepStatusRunning
	r.mu.Unlock()

	err := r.start(context.Background(), SweepRequest{
		Mode:        "multi",
		NoiseValues: []float64{0.01},
	})
	if err != ErrSweepAlreadyRunning {
		t.Errorf("expected ErrSweepAlreadyRunning, got %v", err)
	}
}

func TestRunner_Start_WithPersister(t *testing.T) {
	r := NewRunner(testClient(t))
	mp := &mockPersister{}
	r.SetPersister(mp)

	err := r.start(context.Background(), SweepRequest{
		Mode:        "multi",
		NoiseValues: []float64{0.01},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	r.Stop()
	time.Sleep(50 * time.Millisecond)

	if !mp.startCalled {
		t.Error("expected persister.SaveSweepStart to be called")
	}
	if r.GetSweepID() == "" {
		t.Error("sweep ID should be set after start")
	}
}

func TestRunner_Start_DefaultIterations(t *testing.T) {
	r := NewRunner(testClient(t))

	err := r.start(context.Background(), SweepRequest{
		Mode:        "multi",
		NoiseValues: []float64{0.01},
		Iterations:  0, // should default to 30
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	r.Stop()
	time.Sleep(50 * time.Millisecond)
}

func TestRunner_Start_DefaultMode(t *testing.T) {
	r := NewRunner(testClient(t))

	err := r.start(context.Background(), SweepRequest{
		Mode:        "", // should default to "multi"
		NoiseValues: []float64{0.01},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	r.Stop()
	time.Sleep(50 * time.Millisecond)
}

// --- startGeneric tests ---

func TestRunner_StartGeneric_TooManyParams(t *testing.T) {
	client := &monitor.Client{BaseURL: "http://localhost:8080", SensorID: "test"}
	r := NewRunner(client)

	params := make([]SweepParam, 11)
	for i := range params {
		params[i] = SweepParam{Name: "p" + string(rune('a'+i)), Type: "float64", Start: 0, End: 1, Step: 0.5}
	}
	err := r.startGeneric(context.Background(), SweepRequest{
		Params: params,
	}, time.Second, time.Second)
	if err == nil {
		t.Error("expected error for too many params")
	}
}

func TestRunner_StartGeneric_InvalidParam(t *testing.T) {
	client := &monitor.Client{BaseURL: "http://localhost:8080", SensorID: "test"}
	r := NewRunner(client)

	err := r.startGeneric(context.Background(), SweepRequest{
		Params: []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1, Step: 0}},
	}, time.Second, time.Second)
	if err == nil {
		t.Error("expected error for step = 0")
	}
}

func TestRunner_StartGeneric_EmptyValues(t *testing.T) {
	client := &monitor.Client{BaseURL: "http://localhost:8080", SensorID: "test"}
	r := NewRunner(client)

	err := r.startGeneric(context.Background(), SweepRequest{
		Params: []SweepParam{{Name: "p", Type: "string"}}, // string with no values
	}, time.Second, time.Second)
	if err == nil {
		t.Error("expected error for no values")
	}
}

func TestRunner_StartGeneric_AlreadyRunning(t *testing.T) {
	client := &monitor.Client{BaseURL: "http://localhost:8080", SensorID: "test"}
	r := NewRunner(client)
	r.mu.Lock()
	r.state.Status = SweepStatusRunning
	r.mu.Unlock()

	err := r.startGeneric(context.Background(), SweepRequest{
		Params: []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1, Step: 0.5}},
	}, time.Second, time.Second)
	if err != ErrSweepAlreadyRunning {
		t.Errorf("expected ErrSweepAlreadyRunning, got %v", err)
	}
}

func TestRunner_StartGeneric_Success(t *testing.T) {
	r := NewRunner(testClient(t))
	mp := &mockPersister{}
	r.SetPersister(mp)

	err := r.startGeneric(context.Background(), SweepRequest{
		Params: []SweepParam{{Name: "p", Type: "float64", Start: 0, End: 1, Step: 0.5}},
	}, time.Second, time.Second)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	r.Stop()
	time.Sleep(50 * time.Millisecond)

	if !mp.startCalled {
		t.Error("expected persister to be called")
	}
}

func TestRunner_StartGeneric_TooManyCombos(t *testing.T) {
	client := &monitor.Client{BaseURL: "http://localhost:8080", SensorID: "test"}
	r := NewRunner(client)

	// Each param has many values -> product exceeds 1000
	err := r.startGeneric(context.Background(), SweepRequest{
		Params: []SweepParam{
			{Name: "a", Type: "float64", Start: 0, End: 100, Step: 0.1},
			{Name: "b", Type: "float64", Start: 0, End: 100, Step: 0.1},
		},
	}, time.Second, time.Second)
	if err == nil {
		t.Error("expected error for too many combinations")
	}
}

// --- persistComplete tests ---

func TestRunner_PersistComplete_NoPersister(t *testing.T) {
	r := NewRunner(nil)
	r.persistComplete("complete", "", nil) // should not panic
}

func TestRunner_PersistComplete_NoSweepID(t *testing.T) {
	r := NewRunner(nil)
	r.persister = &mockPersister{}
	r.persistComplete("complete", "", nil) // should not panic
}

func TestRunner_PersistComplete_WithData(t *testing.T) {
	r := NewRunner(nil)
	mp := &mockPersister{}
	r.SetPersister(mp)
	r.sweepID = "test-sweep"
	r.mu.Lock()
	r.state.Results = []ComboResult{{Noise: 0.01}}
	r.mu.Unlock()

	r.persistComplete("complete", "", nil)
	if !mp.completeCalled {
		t.Error("expected SaveSweepComplete to be called")
	}
}

// --- expandSweepParam tests ---

func TestExpandSweepParam_Float64Range(t *testing.T) {
	sp := SweepParam{Name: "p", Type: "float64", Start: 0, End: 1, Step: 0.5}
	if err := expandSweepParam(&sp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sp.Values) == 0 {
		t.Error("expected values to be generated")
	}
}

func TestExpandSweepParam_IntRange(t *testing.T) {
	sp := SweepParam{Name: "p", Type: "int", Start: 0, End: 5, Step: 1}
	if err := expandSweepParam(&sp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sp.Values) < 5 {
		t.Errorf("expected at least 5 values, got %d", len(sp.Values))
	}
}

func TestExpandSweepParam_Int64Range(t *testing.T) {
	sp := SweepParam{Name: "p", Type: "int64", Start: 0, End: 3, Step: 1}
	if err := expandSweepParam(&sp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sp.Values) != 4 { // 0,1,2,3
		t.Errorf("expected 4 values, got %d", len(sp.Values))
	}
}

func TestExpandSweepParam_BoolType(t *testing.T) {
	sp := SweepParam{Name: "p", Type: "bool"}
	if err := expandSweepParam(&sp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sp.Values) != 2 {
		t.Errorf("expected 2 bool values, got %d", len(sp.Values))
	}
}

func TestExpandSweepParam_StringNoValues(t *testing.T) {
	sp := SweepParam{Name: "p", Type: "string"}
	err := expandSweepParam(&sp)
	if err == nil {
		t.Error("expected error for string with no values")
	}
}

func TestExpandSweepParam_UnknownType(t *testing.T) {
	sp := SweepParam{Name: "p", Type: "complex128"}
	err := expandSweepParam(&sp)
	if err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestExpandSweepParam_Float64ZeroStep(t *testing.T) {
	sp := SweepParam{Name: "p", Type: "float64", Step: 0}
	err := expandSweepParam(&sp)
	if err == nil {
		t.Error("expected error for zero step")
	}
}

func TestExpandSweepParam_IntZeroStep(t *testing.T) {
	sp := SweepParam{Name: "p", Type: "int", Step: 0}
	err := expandSweepParam(&sp)
	if err == nil {
		t.Error("expected error for zero step")
	}
}

func TestExpandSweepParam_Int64ZeroStep(t *testing.T) {
	sp := SweepParam{Name: "p", Type: "int64", Step: 0}
	err := expandSweepParam(&sp)
	if err == nil {
		t.Error("expected error for zero step")
	}
}

func TestExpandSweepParam_WithExistingValues(t *testing.T) {
	sp := SweepParam{Name: "p", Type: "float64", Values: []interface{}{0.1, 0.5, 0.9}}
	if err := expandSweepParam(&sp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sp.Values) != 3 {
		t.Errorf("expected 3 values, got %d", len(sp.Values))
	}
}

func TestExpandSweepParam_WithExistingValues_CoercionError(t *testing.T) {
	sp := SweepParam{Name: "p", Type: "int", Values: []interface{}{"notanumber"}}
	err := expandSweepParam(&sp)
	if err == nil {
		t.Error("expected error for invalid int coercion")
	}
}

// --- coerceValue additional tests ---

func TestCoerceValue_Float64FromString(t *testing.T) {
	v, err := coerceValue("3.14", "float64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(float64) != 3.14 {
		t.Errorf("got %v, want 3.14", v)
	}
}

func TestCoerceValue_Float64FromBool(t *testing.T) {
	v, err := coerceValue(true, "float64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(float64) != 1.0 {
		t.Errorf("got %v, want 1.0", v)
	}
	v2, _ := coerceValue(false, "float64")
	if v2.(float64) != 0.0 {
		t.Errorf("got %v, want 0.0", v2)
	}
}

func TestCoerceValue_Float64InvalidString(t *testing.T) {
	_, err := coerceValue("abc", "float64")
	if err == nil {
		t.Error("expected error for invalid float64 string")
	}
}

func TestCoerceValue_IntFromFloat64(t *testing.T) {
	v, err := coerceValue(3.7, "int")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(int) != 3 {
		t.Errorf("got %v, want 3", v)
	}
}

func TestCoerceValue_IntFromString(t *testing.T) {
	v, err := coerceValue(" 42 ", "int")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(int) != 42 {
		t.Errorf("got %v, want 42", v)
	}
}

func TestCoerceValue_IntInvalidString(t *testing.T) {
	_, err := coerceValue("xyz", "int")
	if err == nil {
		t.Error("expected error for invalid int string")
	}
}

func TestCoerceValue_Int64FromFloat64(t *testing.T) {
	v, err := coerceValue(99.9, "int64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(int64) != 99 {
		t.Errorf("got %v, want 99", v)
	}
}

func TestCoerceValue_Int64FromString(t *testing.T) {
	v, err := coerceValue(" 123 ", "int64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(int64) != 123 {
		t.Errorf("got %v, want 123", v)
	}
}

func TestCoerceValue_Int64InvalidString(t *testing.T) {
	_, err := coerceValue("abc", "int64")
	if err == nil {
		t.Error("expected error for invalid int64 string")
	}
}

func TestCoerceValue_BoolFromString(t *testing.T) {
	v, err := coerceValue("true", "bool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(bool) != true {
		t.Errorf("got %v, want true", v)
	}
}

func TestCoerceValue_BoolFromFloat64(t *testing.T) {
	v, err := coerceValue(1.0, "bool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(bool) != true {
		t.Errorf("got %v, want true", v)
	}
	v2, _ := coerceValue(0.0, "bool")
	if v2.(bool) != false {
		t.Errorf("got %v, want false", v2)
	}
}

func TestCoerceValue_StringFromNonString(t *testing.T) {
	v, err := coerceValue(42, "string")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(string) != "42" {
		t.Errorf("got %q, want '42'", v)
	}
}

func TestCoerceValue_StringFromString(t *testing.T) {
	v, err := coerceValue(" hello ", "string")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(string) != "hello" {
		t.Errorf("got %q, want 'hello'", v)
	}
}

func TestCoerceValue_UnsupportedType(t *testing.T) {
	_, err := coerceValue(42, "complex128")
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestCoerceValue_UnsupportedCoercion(t *testing.T) {
	_, err := coerceValue(struct{}{}, "float64")
	if err == nil {
		t.Error("expected error for unsupported coercion")
	}
}

// --- GetSweepState deep copy ---

func TestRunner_GetSweepState_DeepCopy(t *testing.T) {
	r := NewRunner(nil)
	r.mu.Lock()
	r.state.Results = []ComboResult{{Noise: 0.01}, {Noise: 0.02}}
	r.mu.Unlock()

	state := r.GetSweepState()
	state.Results[0].Noise = 999
	// Original should be unchanged
	orig := r.GetSweepState()
	if orig.Results[0].Noise == 999 {
		t.Error("GetSweepState did not return deep copy")
	}
}

// --- Helper: mockClient implements the fields Runner expects ---
// We need a real monitor.Client since Runner uses client.SensorID and client.HTTPClient
// In these tests we null-check to avoid actual HTTP calls.

func init() {
	// Force Client to exist with zero-value structure for mocks
	_ = &monitor.Client{}
}

// --- TooManyCombinations test for legacy mode ---

func TestRunner_Start_TooManyCombinations(t *testing.T) {
	client := &monitor.Client{BaseURL: "http://localhost:8080", SensorID: "test"}
	r := NewRunner(client)

	vals := make([]float64, 20)
	for i := range vals {
		vals[i] = float64(i)
	}
	intVals := make([]int, 20)
	for i := range intVals {
		intVals[i] = i
	}
	err := r.start(context.Background(), SweepRequest{
		Mode:            "multi",
		NoiseValues:     vals,
		ClosenessValues: vals,
		NeighbourValues: intVals,
	})
	if err == nil {
		t.Error("expected error for too many combinations")
	}
}

// --- computeCombinations mode tests ---

func TestRunner_ComputeCombinations_NoiseMode_Range(t *testing.T) {
	client := &monitor.Client{BaseURL: "http://localhost:8080", SensorID: "test"}
	r := NewRunner(client)

	noise, closeness, neighbour := r.computeCombinations(SweepRequest{
		Mode:       "noise",
		NoiseStart: 0.01,
		NoiseEnd:   0.05,
		NoiseStep:  0.01,
	})
	if len(noise) == 0 {
		t.Error("expected noise values to be computed")
	}
	if len(closeness) == 0 {
		t.Error("expected default closeness")
	}
	if len(neighbour) == 0 {
		t.Error("expected default neighbour")
	}
}

func TestRunner_Start_ParamsMode(t *testing.T) {
	r := NewRunner(testClient(t))

	err := r.start(context.Background(), SweepRequest{
		Mode: "params",
		Params: []SweepParam{
			{Name: "noise", Type: "float64", Start: 0.01, End: 0.05, Step: 0.01},
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	r.Stop()
	time.Sleep(50 * time.Millisecond)
}

// --- persistComplete with results ---

func TestRunner_PersistComplete_WithResults(t *testing.T) {
	r := NewRunner(nil)
	mp := &mockPersister{}
	r.SetPersister(mp)
	r.sweepID = "test-sweep"
	r.mu.Lock()
	r.state.Results = []ComboResult{
		{Noise: 0.01, Closeness: 3.0, Neighbour: 3},
		{Noise: 0.02, Closeness: 4.0, Neighbour: 2},
	}
	r.mu.Unlock()

	rec, _ := json.Marshal(map[string]interface{}{"noise": 0.01})
	r.persistComplete("complete", "", rec)
	if !mp.completeCalled {
		t.Error("expected SaveSweepComplete to be called")
	}
	if mp.completeStatus != "complete" {
		t.Errorf("status = %q, want complete", mp.completeStatus)
	}
}

func TestRunner_PersistComplete_WithError(t *testing.T) {
	r := NewRunner(nil)
	mp := &mockPersister{}
	r.SetPersister(mp)
	r.sweepID = "test-sweep"

	r.persistComplete("error", "something failed", nil)
	if !mp.completeCalled {
		t.Error("expected SaveSweepComplete to be called")
	}
	if mp.completeStatus != "error" {
		t.Errorf("status = %q, want error", mp.completeStatus)
	}
}
