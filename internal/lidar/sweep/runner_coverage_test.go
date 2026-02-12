package sweep

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// runnerMockServer is like sweepMockServer but is for the Runner's run()/runGeneric() path.
func runnerMockServer(t *testing.T) *httptest.Server {
	t.Helper()

	acceptanceJSON := `{
		"BucketsMeters": [1,2,4],
		"AcceptCounts": [10,20,30],
		"RejectCounts": [2,3,4],
		"Totals": [12,23,34],
		"AcceptanceRates": [0.83,0.87,0.88]
	}`
	gridStatusJSON := `{"background_count": 42, "settled": true}`
	trackMetricsJSON := `{
		"active_tracks": 3,
		"mean_alignment_deg": 2.5,
		"misalignment_ratio": 0.1,
		"heading_jitter_deg": 1.0,
		"speed_jitter_mps": 0.5,
		"fragmentation_ratio": 0.05,
		"foreground_capture_ratio": 0.85,
		"unbounded_point_ratio": 0.02,
		"empty_box_ratio": 0.01
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/api/lidar/acceptance/reset"):
			fmt.Fprint(w, `{}`)
		case strings.Contains(path, "/api/lidar/acceptance"):
			fmt.Fprint(w, acceptanceJSON)
		case strings.Contains(path, "/api/lidar/grid/reset"):
			fmt.Fprint(w, `{}`)
		case strings.Contains(path, "/api/lidar/grid_status"):
			fmt.Fprint(w, gridStatusJSON)
		case strings.Contains(path, "/api/lidar/tuning-params"):
			fmt.Fprint(w, `{}`)
		case strings.Contains(path, "/api/lidar/params"):
			fmt.Fprint(w, `{}`)
		case strings.Contains(path, "/api/lidar/tracks/metrics"):
			fmt.Fprint(w, trackMetricsJSON)
		case strings.Contains(path, "/api/lidar/pcap/start"):
			fmt.Fprint(w, `{"status":"started"}`)
		case strings.Contains(path, "/api/lidar/pcap/stop"):
			fmt.Fprint(w, `{}`)
		case strings.Contains(path, "/api/lidar/data_source"):
			fmt.Fprint(w, `{"pcap_in_progress":false,"source":"live"}`)
		case strings.Contains(path, "/api/lidar/data-source"):
			fmt.Fprint(w, `{"source":"live"}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runnerTestClient(t *testing.T, srv *httptest.Server) *monitor.Client {
	t.Helper()
	return monitor.NewClient(srv.Client(), srv.URL, "test-sensor")
}

// waitForRunnerStatus waits for the runner's sweep to reach a status.
func waitForRunnerStatus(t *testing.T, r *Runner, timeout time.Duration, targets ...SweepStatus) SweepState {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state := r.GetSweepState()
		for _, target := range targets {
			if state.Status == target {
				return state
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	state := r.GetSweepState()
	t.Fatalf("timed out waiting for %v, current=%q error=%q", targets, state.Status, state.Error)
	return state
}

// --- Legacy run() tests ---

func TestRunnerCov2_RunLegacyComplete(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode:            "multi",
		NoiseValues:     []float64{0.01, 0.02},
		ClosenessValues: []float64{1.5},
		NeighbourValues: []int{1},
		Iterations:      1,
		Interval:        "10ms",
		SettleTime:      "10ms",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
	if state.Status != SweepStatusComplete {
		t.Errorf("status = %q, want complete", state.Status)
	}
	if len(state.Results) != 2 {
		t.Errorf("results = %d, want 2", len(state.Results))
	}
}

func TestRunnerCov2_RunLegacyCancelled(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	ctx, cancel := context.WithCancel(context.Background())

	// Use many combos so we can cancel mid-way
	req := SweepRequest{
		Mode:            "multi",
		NoiseValues:     []float64{0.01, 0.02, 0.03, 0.04, 0.05},
		ClosenessValues: []float64{1.5, 2.0, 2.5},
		NeighbourValues: []int{1, 2},
		Iterations:      1,
		Interval:        "10ms",
		SettleTime:      "10ms",
	}
	err := r.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Cancel quickly
	time.Sleep(100 * time.Millisecond)
	cancel()

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusError, SweepStatusComplete)
	// Either it was cancelled (error) or it completed very fast
	if state.Status != SweepStatusError && state.Status != SweepStatusComplete {
		t.Errorf("expected error or complete, got %q", state.Status)
	}
}

func TestRunnerCov2_RunLegacyToggleSeed(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode:            "multi",
		NoiseValues:     []float64{0.01, 0.02},
		ClosenessValues: []float64{1.5},
		NeighbourValues: []int{1},
		Iterations:      1,
		Interval:        "10ms",
		SettleTime:      "10ms",
		Seed:            "toggle",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
	if state.Status != SweepStatusComplete {
		t.Errorf("status = %q, want complete", state.Status)
	}
}

func TestRunnerCov2_RunLegacySeedFalse(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode:            "multi",
		NoiseValues:     []float64{0.01},
		ClosenessValues: []float64{1.5},
		NeighbourValues: []int{1},
		Iterations:      1,
		Interval:        "10ms",
		SettleTime:      "10ms",
		Seed:            "false",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
	if state.Status != SweepStatusComplete {
		t.Errorf("status = %q, want complete", state.Status)
	}
}

func TestRunnerCov2_RunLegacySettleOnce(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode:            "multi",
		NoiseValues:     []float64{0.01, 0.02},
		ClosenessValues: []float64{1.5},
		NeighbourValues: []int{1},
		Iterations:      1,
		Interval:        "10ms",
		SettleTime:      "10ms",
		SettleMode:      "once",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
	if state.Status != SweepStatusComplete {
		t.Errorf("status = %q, want complete", state.Status)
	}
}

func TestRunnerCov2_RunLegacyWithPersister(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)
	r.SetPersister(&runnerMockPersister{})

	req := SweepRequest{
		Mode:            "multi",
		NoiseValues:     []float64{0.01},
		ClosenessValues: []float64{1.5},
		NeighbourValues: []int{1},
		Iterations:      1,
		Interval:        "10ms",
		SettleTime:      "10ms",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
	if state.Status != SweepStatusComplete {
		t.Errorf("status = %q, want complete", state.Status)
	}
	if r.GetSweepID() == "" {
		t.Error("expected non-empty sweep ID")
	}
}

// --- Legacy run() PCAP mode ---

func TestRunnerCov2_RunLegacyPCAPMode(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode:            "multi",
		DataSource:      "pcap",
		PCAPFile:        "test.pcap",
		NoiseValues:     []float64{0.01},
		ClosenessValues: []float64{1.5},
		NeighbourValues: []int{1},
		Iterations:      1,
		Interval:        "10ms",
		SettleTime:      "10ms",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete, SweepStatusError)
	_ = state // Either status is acceptable
}

func TestRunnerCov2_RunLegacyPCAPSettleOnce(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode:            "multi",
		DataSource:      "pcap",
		PCAPFile:        "test.pcap",
		NoiseValues:     []float64{0.01, 0.02},
		ClosenessValues: []float64{1.5},
		NeighbourValues: []int{1},
		Iterations:      1,
		Interval:        "10ms",
		SettleTime:      "10ms",
		SettleMode:      "once",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete, SweepStatusError)
	_ = state
}

// --- runGeneric() tests ---

func TestRunnerCov2_RunGenericComplete(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode: "params",
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Values: []interface{}{0.01, 0.02}},
		},
		Iterations: 1,
		Interval:   "10ms",
		SettleTime: "10ms",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
	if len(state.Results) != 2 {
		t.Errorf("results = %d, want 2", len(state.Results))
	}
}

func TestRunnerCov2_RunGenericCancelled(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	ctx, cancel := context.WithCancel(context.Background())

	req := SweepRequest{
		Mode: "params",
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Values: []interface{}{0.01, 0.02, 0.03, 0.04, 0.05}},
			{Name: "closeness_multiplier", Type: "float64", Values: []interface{}{1.0, 1.5, 2.0}},
		},
		Iterations: 1,
		Interval:   "10ms",
		SettleTime: "10ms",
	}
	err := r.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	cancel()

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusError, SweepStatusComplete)
	_ = state
}

func TestRunnerCov2_RunGenericToggleSeed(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode: "params",
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Values: []interface{}{0.01, 0.02}},
		},
		Iterations: 1,
		Interval:   "10ms",
		SettleTime: "10ms",
		Seed:       "toggle",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
	if state.Status != SweepStatusComplete {
		t.Errorf("status = %q, want complete", state.Status)
	}
}

func TestRunnerCov2_RunGenericSeedFalse(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode: "params",
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Values: []interface{}{0.01}},
		},
		Iterations: 1,
		Interval:   "10ms",
		SettleTime: "10ms",
		Seed:       "false",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
	if state.Status != SweepStatusComplete {
		t.Errorf("status = %q, want complete", state.Status)
	}
}

func TestRunnerCov2_RunGenericSettleOnce(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode: "params",
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Values: []interface{}{0.01, 0.02}},
		},
		Iterations: 1,
		Interval:   "10ms",
		SettleTime: "10ms",
		SettleMode: "once",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
	if state.Status != SweepStatusComplete {
		t.Errorf("status = %q, want complete", state.Status)
	}
}

func TestRunnerCov2_RunGenericPCAPMode(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode:       "params",
		DataSource: "pcap",
		PCAPFile:   "test.pcap",
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Values: []interface{}{0.01}},
		},
		Iterations: 1,
		Interval:   "10ms",
		SettleTime: "10ms",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete, SweepStatusError)
	_ = state
}

func TestRunnerCov2_RunGenericPCAPSettleOnce(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode:       "params",
		DataSource: "pcap",
		PCAPFile:   "test.pcap",
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Values: []interface{}{0.01, 0.02}},
		},
		Iterations: 1,
		Interval:   "10ms",
		SettleTime: "10ms",
		SettleMode: "once",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete, SweepStatusError)
	_ = state
}

func TestRunnerCov2_RunGenericWithPersister(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)
	r.SetPersister(&runnerMockPersister{})

	req := SweepRequest{
		Mode: "params",
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Values: []interface{}{0.01}},
		},
		Iterations: 1,
		Interval:   "10ms",
		SettleTime: "10ms",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
	if state.Status != SweepStatusComplete {
		t.Errorf("status = %q, want complete", state.Status)
	}
}

// --- SetParams failure in run() ---

func TestRunnerCov2_RunLegacySetParamsFail(t *testing.T) {
	// Return 500 for params endpoint to test error path
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/api/lidar/params") || strings.Contains(r.URL.Path, "/api/lidar/tuning-params") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"test failure"}`)
			return
		}
		fmt.Fprint(w, `{}`)
	}))
	t.Cleanup(srv.Close)

	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode:            "multi",
		NoiseValues:     []float64{0.01},
		ClosenessValues: []float64{1.5},
		NeighbourValues: []int{1},
		Iterations:      1,
		Interval:        "10ms",
		SettleTime:      "10ms",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusError, SweepStatusComplete)
	_ = state
}

func TestRunnerCov2_RunGenericSetParamsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/api/lidar/params") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"test failure"}`)
			return
		}
		if strings.Contains(r.URL.Path, "/api/lidar/acceptance") {
			fmt.Fprint(w, `{"BucketsMeters":[1],"AcceptCounts":[10],"RejectCounts":[2],"Totals":[12],"AcceptanceRates":[0.83]}`)
			return
		}
		fmt.Fprint(w, `{}`)
	}))
	t.Cleanup(srv.Close)

	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode: "params",
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Values: []interface{}{0.01}},
		},
		Iterations: 1,
		Interval:   "10ms",
		SettleTime: "10ms",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state := waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
	// Should complete with warnings since generic continues on error
	if len(state.Warnings) == 0 {
		t.Error("expected warnings about failed params")
	}
}

// --- computeComboResult ---

func TestRunnerCov2_ComputeComboResult_EmptyResults(t *testing.T) {
	r := &Runner{}
	combo := r.computeComboResult(0.01, 1.5, 1, nil, []string{"1", "2"})
	if combo.Noise != 0.01 {
		t.Errorf("noise = %f, want 0.01", combo.Noise)
	}
	if len(combo.BucketMeans) != 0 {
		t.Errorf("expected empty bucket means, got %d", len(combo.BucketMeans))
	}
}

func TestRunnerCov2_ComputeComboResult_WithResults(t *testing.T) {
	r := &Runner{}
	results := []SampleResult{
		{
			OverallAcceptPct:       85.0,
			NonzeroCells:           100,
			AcceptanceRates:        []float64{0.8, 0.9},
			ActiveTracks:           3,
			MeanAlignmentDeg:       2.0,
			MisalignmentRatio:      0.1,
			HeadingJitterDeg:       1.0,
			SpeedJitterMps:         0.5,
			FragmentationRatio:     0.05,
			ForegroundCaptureRatio: 0.85,
			UnboundedPointRatio:    0.02,
			EmptyBoxRatio:          0.01,
		},
		{
			OverallAcceptPct: 90.0,
			NonzeroCells:     120,
			AcceptanceRates:  []float64{0.85, 0.92},
			ActiveTracks:     4,
		},
	}
	combo := r.computeComboResult(0.01, 1.5, 1, results, []string{"bucket1", "bucket2"})
	if combo.OverallAcceptMean == 0 {
		t.Error("expected non-zero OverallAcceptMean")
	}
	if len(combo.BucketMeans) != 2 {
		t.Errorf("bucket means = %d, want 2", len(combo.BucketMeans))
	}
}

// --- computeCombinations (legacy) ---

func TestRunnerCov2_ComputeCombinations_NoiseMode(t *testing.T) {
	r := &Runner{}
	req := SweepRequest{
		Mode:       "noise",
		NoiseStart: 0.01, NoiseEnd: 0.03, NoiseStep: 0.01,
		FixedCloseness: 1.5,
		FixedNeighbour: 1,
	}
	n, c, nb := r.computeCombinations(req)
	if len(n) != 3 {
		t.Errorf("noise combos = %d, want 3", len(n))
	}
	if len(c) != 1 || c[0] != 1.5 {
		t.Errorf("closeness = %v, want [1.5]", c)
	}
	if len(nb) != 1 || nb[0] != 1 {
		t.Errorf("neighbour = %v, want [1]", nb)
	}
}

func TestRunnerCov2_ComputeCombinations_ClosenessMode(t *testing.T) {
	r := &Runner{}
	req := SweepRequest{
		Mode:           "closeness",
		ClosenessStart: 1.0, ClosenessEnd: 2.0, ClosenessStep: 0.5,
		FixedNoise:     0.01,
		FixedNeighbour: 1,
	}
	n, c, nb := r.computeCombinations(req)
	if len(c) != 3 {
		t.Errorf("closeness combos = %d, want 3", len(c))
	}
	if len(n) != 1 || n[0] != 0.01 {
		t.Errorf("noise = %v, want [0.01]", n)
	}
	_ = nb
}

func TestRunnerCov2_ComputeCombinations_NeighbourMode(t *testing.T) {
	r := &Runner{}
	req := SweepRequest{
		Mode:           "neighbour",
		NeighbourStart: 0, NeighbourEnd: 2, NeighbourStep: 1,
		FixedNoise:     0.01,
		FixedCloseness: 1.5,
	}
	n, c, nb := r.computeCombinations(req)
	if len(nb) != 3 {
		t.Errorf("neighbour combos = %d, want 3", len(nb))
	}
	_ = n
	_ = c
}

func TestRunnerCov2_ComputeCombinations_MultiWithRanges(t *testing.T) {
	r := &Runner{}
	req := SweepRequest{
		Mode:       "multi",
		NoiseStart: 0.01, NoiseEnd: 0.02, NoiseStep: 0.01,
		ClosenessStart: 1.0, ClosenessEnd: 1.5, ClosenessStep: 0.5,
		NeighbourStart: 0, NeighbourEnd: 1, NeighbourStep: 1,
	}
	n, c, nb := r.computeCombinations(req)
	if len(n) == 0 || len(c) == 0 || len(nb) == 0 {
		t.Errorf("expected non-empty combos, got n=%d c=%d nb=%d", len(n), len(c), len(nb))
	}
}

// --- cartesianProduct ---

func TestRunnerCov2_CartesianProduct_Empty(t *testing.T) {
	result, err := cartesianProduct(nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestRunnerCov2_CartesianProduct_TooLarge(t *testing.T) {
	params := []SweepParam{
		{Name: "a", Values: make([]interface{}, 200)},
		{Name: "b", Values: make([]interface{}, 200)},
	}
	_, err := cartesianProduct(params)
	if err == nil {
		t.Error("expected error for too-large product")
	}
}

// --- toFloat64 / toInt ---

func TestRunnerCov2_ToFloat64(t *testing.T) {
	tests := []struct {
		in interface{}
		ok bool
	}{
		{1.5, true},
		{int(3), true},
		{int64(4), true},
		{"not a number", false},
	}
	for _, tc := range tests {
		_, ok := toFloat64(tc.in)
		if ok != tc.ok {
			t.Errorf("toFloat64(%v) ok=%v, want %v", tc.in, ok, tc.ok)
		}
	}
}

func TestRunnerCov2_ToInt(t *testing.T) {
	tests := []struct {
		in interface{}
		ok bool
	}{
		{int(3), true},
		{float64(2.5), true},
		{int64(4), true},
		{"not a number", false},
	}
	for _, tc := range tests {
		_, ok := toInt(tc.in)
		if ok != tc.ok {
			t.Errorf("toInt(%v) ok=%v, want %v", tc.in, ok, tc.ok)
		}
	}
}

// --- start() validation ---

func TestRunnerCov2_Start_NilContext(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode:            "multi",
		NoiseValues:     []float64{0.01},
		ClosenessValues: []float64{1.5},
		NeighbourValues: []int{1},
		Iterations:      1,
		Interval:        "10ms",
		SettleTime:      "10ms",
	}
	// Should not panic with nil context
	err := r.start(nil, req)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
}

func TestRunnerCov2_Start_TooManyIterations(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode:            "multi",
		NoiseValues:     []float64{0.01},
		ClosenessValues: []float64{1.5},
		NeighbourValues: []int{1},
		Iterations:      501,
	}
	err := r.Start(context.Background(), req)
	if err == nil {
		t.Error("expected error for >500 iterations")
	}
}

func TestRunnerCov2_Start_UnsupportedMode(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{Mode: "invalid"}
	err := r.Start(context.Background(), req)
	if err == nil {
		t.Error("expected error for unsupported mode")
	}
}

func TestRunnerCov2_Start_BadInterval(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{Interval: "bad_dur"}
	err := r.Start(context.Background(), req)
	if err == nil {
		t.Error("expected error for bad interval")
	}
}

func TestRunnerCov2_Start_BadSettleTime(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{SettleTime: "bad_dur"}
	err := r.Start(context.Background(), req)
	if err == nil {
		t.Error("expected error for bad settle_time")
	}
}

// --- startGeneric() validation ---

func TestRunnerCov2_StartGeneric_TooManyParams(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	params := make([]SweepParam, 11)
	for i := range params {
		params[i] = SweepParam{Name: fmt.Sprintf("p%d", i), Type: "float64", Values: []interface{}{0.1}}
	}
	req := SweepRequest{
		Mode:       "params",
		Params:     params,
		Iterations: 1,
		Interval:   "10ms",
		SettleTime: "10ms",
	}
	err := r.Start(context.Background(), req)
	if err == nil {
		t.Error("expected error for too many params")
	}
}

func TestRunnerCov2_StartGeneric_TooManyCombos(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	// 5 params × 10 values each = 100,000 combos > 1000 limit
	params := make([]SweepParam, 5)
	for i := range params {
		vals := make([]interface{}, 10)
		for j := range vals {
			vals[j] = float64(j)
		}
		params[i] = SweepParam{Name: fmt.Sprintf("p%d", i), Type: "float64", Values: vals}
	}
	req := SweepRequest{
		Mode:       "params",
		Params:     params,
		Iterations: 1,
		Interval:   "10ms",
		SettleTime: "10ms",
	}
	err := r.Start(context.Background(), req)
	if err == nil {
		t.Error("expected error for too many combos")
	}
}

func TestRunnerCov2_StartGeneric_AlreadyRunning(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	req := SweepRequest{
		Mode: "params",
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Values: []interface{}{0.01, 0.02, 0.03, 0.04, 0.05, 0.06, 0.07, 0.08}},
		},
		Iterations: 3,
		Interval:   "100ms",
		SettleTime: "100ms",
	}
	err := r.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Try to start again while still running
	err = r.Start(context.Background(), req)
	if err != ErrSweepAlreadyRunning {
		t.Errorf("expected ErrSweepAlreadyRunning, got %v", err)
	}

	r.Stop()
	waitForRunnerStatus(t, r, 30*time.Second, SweepStatusError, SweepStatusComplete)
}

// --- Start via map[string]interface{} ---

func TestRunnerCov2_Start_ViaMap(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	reqMap := map[string]interface{}{
		"mode":             "multi",
		"noise_values":     []interface{}{0.01},
		"closeness_values": []interface{}{1.5},
		"neighbour_values": []interface{}{1.0},
		"iterations":       1,
		"interval":         "10ms",
		"settle_time":      "10ms",
	}
	err := r.Start(context.Background(), reqMap)
	if err != nil {
		t.Fatalf("Start via map: %v", err)
	}
	waitForRunnerStatus(t, r, 30*time.Second, SweepStatusComplete)
}

func TestRunnerCov2_Start_ViaInvalidType(t *testing.T) {
	srv := runnerMockServer(t)
	client := runnerTestClient(t, srv)
	r := NewRunner(client)

	err := r.Start(context.Background(), 42)
	if err == nil {
		t.Error("expected error for invalid request type")
	}
}

// --- GetState / GetSweepState ---

func TestRunnerCov2_GetState(t *testing.T) {
	r := &Runner{state: SweepState{Status: SweepStatusIdle}}
	state := r.GetState()
	if ss, ok := state.(SweepState); !ok || ss.Status != SweepStatusIdle {
		t.Errorf("unexpected state: %v", state)
	}
}

// --- addWarning ---

func TestRunnerCov2_AddWarning(t *testing.T) {
	r := &Runner{state: SweepState{}}
	r.addWarning("test warning")
	state := r.GetSweepState()
	if len(state.Warnings) != 1 || state.Warnings[0] != "test warning" {
		t.Errorf("warnings = %v, want [\"test warning\"]", state.Warnings)
	}
}

// --- persistComplete ---

func TestRunnerCov2_PersistComplete_NoPersister(t *testing.T) {
	r := &Runner{}
	// Should not panic
	r.persistComplete("complete", "", nil)
}

func TestRunnerCov2_PersistComplete_NoSweepID(t *testing.T) {
	r := &Runner{persister: &runnerMockPersister{}}
	// Should return early - sweepID is empty
	r.persistComplete("complete", "", nil)
}

func TestRunnerCov2_PersistComplete_WithPersister(t *testing.T) {
	mp := &runnerMockPersister{}
	r := &Runner{
		persister: mp,
		sweepID:   "test-sweep",
		state: SweepState{
			Results: []ComboResult{{Noise: 0.01}},
		},
	}
	r.persistComplete("complete", "", nil)
}

// --- runnerMockPersister ---

type runnerMockPersister struct {
	startErr    error
	completeErr error
}

func (m *runnerMockPersister) SaveSweepStart(sweepID, sensorID, mode string, request json.RawMessage, startedAt time.Time) error {
	return m.startErr
}

func (m *runnerMockPersister) SaveSweepComplete(sweepID, status string, results, recommendation, roundResults json.RawMessage, completedAt time.Time, errMsg string) error {
	return m.completeErr
}
