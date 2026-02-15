package sweep

import (
	"math"
	"testing"
)

func TestNormaliseTransform(t *testing.T) {
	transform := &NormaliseTransform{
		Metric: "test_metric",
		Min:    0.0,
		Max:    100.0,
	}

	tests := []struct {
		name     string
		metrics  map[string]float64
		expected map[string]float64
	}{
		{
			name:     "normalise value in range",
			metrics:  map[string]float64{"test_metric": 50.0},
			expected: map[string]float64{"test_metric": 0.5},
		},
		{
			name:     "normalise min value",
			metrics:  map[string]float64{"test_metric": 0.0},
			expected: map[string]float64{"test_metric": 0.0},
		},
		{
			name:     "normalise max value",
			metrics:  map[string]float64{"test_metric": 100.0},
			expected: map[string]float64{"test_metric": 1.0},
		},
		{
			name:     "metric not present",
			metrics:  map[string]float64{"other_metric": 50.0},
			expected: map[string]float64{"other_metric": 50.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transform.Apply(tt.metrics)
			for k, expectedVal := range tt.expected {
				if result[k] != expectedVal {
					t.Errorf("expected %s = %f, got %f", k, expectedVal, result[k])
				}
			}
		})
	}
}

func TestNormaliseTransform_DivisionByZero(t *testing.T) {
	transform := &NormaliseTransform{
		Metric: "test_metric",
		Min:    50.0,
		Max:    50.0, // Min == Max
	}

	metrics := map[string]float64{"test_metric": 50.0}
	result := transform.Apply(metrics)

	// Should remain unchanged when Max == Min
	if result["test_metric"] != 50.0 {
		t.Errorf("expected value to remain 50.0, got %f", result["test_metric"])
	}
}

func TestClipTransform(t *testing.T) {
	transform := &ClipTransform{
		Metric: "test_metric",
		Min:    0.0,
		Max:    1.0,
	}

	tests := []struct {
		name     string
		metrics  map[string]float64
		expected map[string]float64
	}{
		{
			name:     "value within range",
			metrics:  map[string]float64{"test_metric": 0.5},
			expected: map[string]float64{"test_metric": 0.5},
		},
		{
			name:     "value below min",
			metrics:  map[string]float64{"test_metric": -0.5},
			expected: map[string]float64{"test_metric": 0.0},
		},
		{
			name:     "value above max",
			metrics:  map[string]float64{"test_metric": 1.5},
			expected: map[string]float64{"test_metric": 1.0},
		},
		{
			name:     "metric not present",
			metrics:  map[string]float64{"other_metric": 2.0},
			expected: map[string]float64{"other_metric": 2.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transform.Apply(tt.metrics)
			for k, expectedVal := range tt.expected {
				if result[k] != expectedVal {
					t.Errorf("expected %s = %f, got %f", k, expectedVal, result[k])
				}
			}
		})
	}
}

func TestLogScaleTransform(t *testing.T) {
	transform := &LogScaleTransform{
		Metric: "test_metric",
	}

	tests := []struct {
		name     string
		metrics  map[string]float64
		expected map[string]float64
	}{
		{
			name:     "log of positive value",
			metrics:  map[string]float64{"test_metric": 10.0},
			expected: map[string]float64{"test_metric": math.Log(11.0)},
		},
		{
			name:     "log of zero",
			metrics:  map[string]float64{"test_metric": 0.0},
			expected: map[string]float64{"test_metric": math.Log(1.0)},
		},
		{
			name:     "metric not present",
			metrics:  map[string]float64{"other_metric": 10.0},
			expected: map[string]float64{"other_metric": 10.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transform.Apply(tt.metrics)
			for k, expectedVal := range tt.expected {
				if math.Abs(result[k]-expectedVal) > 1e-9 {
					t.Errorf("expected %s = %f, got %f", k, expectedVal, result[k])
				}
			}
		})
	}
}

func TestClassWeightTransform(t *testing.T) {
	transform := &ClassWeightTransform{
		Metric: "test_metric",
		Weight: 2.5,
	}

	tests := []struct {
		name     string
		metrics  map[string]float64
		expected map[string]float64
	}{
		{
			name:     "multiply by weight",
			metrics:  map[string]float64{"test_metric": 4.0},
			expected: map[string]float64{"test_metric": 10.0},
		},
		{
			name:     "multiply zero",
			metrics:  map[string]float64{"test_metric": 0.0},
			expected: map[string]float64{"test_metric": 0.0},
		},
		{
			name:     "metric not present",
			metrics:  map[string]float64{"other_metric": 4.0},
			expected: map[string]float64{"other_metric": 4.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transform.Apply(tt.metrics)
			for k, expectedVal := range tt.expected {
				if result[k] != expectedVal {
					t.Errorf("expected %s = %f, got %f", k, expectedVal, result[k])
				}
			}
		})
	}
}

func TestRoundModifierTransform(t *testing.T) {
	currentRound := 2

	transform := &RoundModifierTransform{
		Metric:       "test_metric",
		Multiplier:   3.0,
		Round:        2,
		CurrentRound: &currentRound,
	}

	tests := []struct {
		name         string
		metrics      map[string]float64
		currentRound int
		expected     map[string]float64
	}{
		{
			name:         "apply when round matches",
			metrics:      map[string]float64{"test_metric": 5.0},
			currentRound: 2,
			expected:     map[string]float64{"test_metric": 15.0},
		},
		{
			name:         "skip when round doesn't match",
			metrics:      map[string]float64{"test_metric": 5.0},
			currentRound: 3,
			expected:     map[string]float64{"test_metric": 5.0},
		},
		{
			name:         "metric not present",
			metrics:      map[string]float64{"other_metric": 5.0},
			currentRound: 2,
			expected:     map[string]float64{"other_metric": 5.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*transform.CurrentRound = tt.currentRound
			result := transform.Apply(tt.metrics)
			for k, expectedVal := range tt.expected {
				if result[k] != expectedVal {
					t.Errorf("expected %s = %f, got %f", k, expectedVal, result[k])
				}
			}
		})
	}
}

func TestRoundModifierTransform_NilCurrentRound(t *testing.T) {
	transform := &RoundModifierTransform{
		Metric:       "test_metric",
		Multiplier:   3.0,
		Round:        2,
		CurrentRound: nil,
	}

	metrics := map[string]float64{"test_metric": 5.0}
	result := transform.Apply(metrics)

	// Should remain unchanged when CurrentRound is nil
	if result["test_metric"] != 5.0 {
		t.Errorf("expected value to remain 5.0, got %f", result["test_metric"])
	}
}

func TestTransformPipeline_Apply(t *testing.T) {
	pipeline := NewTransformPipeline("test", "1.0",
		&ClipTransform{Metric: "value", Min: 0.0, Max: 100.0},
		&NormaliseTransform{Metric: "value", Min: 0.0, Max: 100.0},
		&ClassWeightTransform{Metric: "value", Weight: 2.0},
	)

	metrics := map[string]float64{"value": 150.0}
	result := pipeline.Apply(metrics)

	// 150.0 -> clip to 100.0 -> normalise to 1.0 -> weight by 2.0 = 2.0
	expected := 2.0
	if math.Abs(result["value"]-expected) > 1e-9 {
		t.Errorf("expected value = %f, got %f", expected, result["value"])
	}
}

func TestTransformPipeline_TransformNames(t *testing.T) {
	pipeline := NewTransformPipeline("test", "1.0",
		&ClipTransform{Metric: "value", Min: 0.0, Max: 100.0},
		&LogScaleTransform{Metric: "count"},
		&ClassWeightTransform{Metric: "weight", Weight: 2.0},
	)

	names := pipeline.TransformNames()
	expected := []string{"clip:value", "log:count", "weight:weight"}

	if len(names) != len(expected) {
		t.Fatalf("expected %d transform names, got %d", len(expected), len(names))
	}

	for i, name := range names {
		if name != expected[i] {
			t.Errorf("expected transform name %s, got %s", expected[i], name)
		}
	}
}

func TestDefaultTransformPipeline(t *testing.T) {
	pipeline := DefaultTransformPipeline()

	if pipeline.Name != "default" {
		t.Errorf("expected name 'default', got '%s'", pipeline.Name)
	}

	if pipeline.Version != "1.0" {
		t.Errorf("expected version '1.0', got '%s'", pipeline.Version)
	}

	if len(pipeline.transforms) != 0 {
		t.Errorf("expected 0 transforms, got %d", len(pipeline.transforms))
	}

	// Should pass through metrics unchanged
	metrics := map[string]float64{"test": 42.0}
	result := pipeline.Apply(metrics)

	if result["test"] != 42.0 {
		t.Errorf("expected value to remain 42.0, got %f", result["test"])
	}
}

func TestGroundTruthTransformPipeline(t *testing.T) {
	pipeline := GroundTruthTransformPipeline()

	if pipeline.Name != "ground_truth" {
		t.Errorf("expected name 'ground_truth', got '%s'", pipeline.Name)
	}

	if pipeline.Version != "1.0" {
		t.Errorf("expected version '1.0', got '%s'", pipeline.Version)
	}

	// Test that it clips ratio metrics to [0,1]
	metrics := map[string]float64{
		"acceptance_rate":     1.5,
		"misalignment_ratio":  -0.1,
		"foreground_capture":  0.8,
		"empty_box_ratio":     1.2,
		"fragmentation_ratio": 0.5,
		"nonzero_cells":       100.0,
		"active_tracks":       10.0,
	}

	result := pipeline.Apply(metrics)

	// Check clipping
	if result["acceptance_rate"] != 1.0 {
		t.Errorf("expected acceptance_rate clipped to 1.0, got %f", result["acceptance_rate"])
	}
	if result["misalignment_ratio"] != 0.0 {
		t.Errorf("expected misalignment_ratio clipped to 0.0, got %f", result["misalignment_ratio"])
	}
	if result["empty_box_ratio"] != 1.0 {
		t.Errorf("expected empty_box_ratio clipped to 1.0, got %f", result["empty_box_ratio"])
	}

	// Check log scaling
	expectedNonzeroCells := math.Log(101.0)
	if math.Abs(result["nonzero_cells"]-expectedNonzeroCells) > 1e-9 {
		t.Errorf("expected nonzero_cells = %f, got %f", expectedNonzeroCells, result["nonzero_cells"])
	}

	expectedActiveTracks := math.Log(11.0)
	if math.Abs(result["active_tracks"]-expectedActiveTracks) > 1e-9 {
		t.Errorf("expected active_tracks = %f, got %f", expectedActiveTracks, result["active_tracks"])
	}
}

func TestTransformRegistry_Register(t *testing.T) {
	registry := NewTransformRegistry()
	pipeline := NewTransformPipeline("test", "1.0")

	registry.Register(pipeline)

	retrieved, ok := registry.Get("test")
	if !ok {
		t.Fatal("expected to find registered pipeline")
	}

	if retrieved.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", retrieved.Name)
	}
}

func TestTransformRegistry_Get_NotFound(t *testing.T) {
	registry := NewTransformRegistry()

	_, ok := registry.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent pipeline")
	}
}

func TestTransformRegistry_List(t *testing.T) {
	registry := NewTransformRegistry()

	pipeline1 := NewTransformPipeline("test1", "1.0",
		&ClipTransform{Metric: "value", Min: 0.0, Max: 1.0},
	)
	pipeline2 := NewTransformPipeline("test2", "2.0",
		&LogScaleTransform{Metric: "count"},
		&ClassWeightTransform{Metric: "weight", Weight: 2.0},
	)

	registry.Register(pipeline1)
	registry.Register(pipeline2)

	infos := registry.List()

	if len(infos) != 2 {
		t.Fatalf("expected 2 pipelines, got %d", len(infos))
	}

	// Find test1
	var test1Info *TransformPipelineInfo
	for i := range infos {
		if infos[i].Name == "test1" {
			test1Info = &infos[i]
			break
		}
	}

	if test1Info == nil {
		t.Fatal("expected to find test1 pipeline in list")
	}

	if test1Info.Version != "1.0" {
		t.Errorf("expected test1 version '1.0', got '%s'", test1Info.Version)
	}

	if len(test1Info.Transforms) != 1 || test1Info.Transforms[0] != "clip:value" {
		t.Errorf("expected test1 to have [clip:value], got %v", test1Info.Transforms)
	}

	// Find test2
	var test2Info *TransformPipelineInfo
	for i := range infos {
		if infos[i].Name == "test2" {
			test2Info = &infos[i]
			break
		}
	}

	if test2Info == nil {
		t.Fatal("expected to find test2 pipeline in list")
	}

	if len(test2Info.Transforms) != 2 {
		t.Errorf("expected test2 to have 2 transforms, got %d", len(test2Info.Transforms))
	}
}

func TestDefaultTransformRegistry(t *testing.T) {
	registry := DefaultTransformRegistry()

	infos := registry.List()
	if len(infos) != 2 {
		t.Fatalf("expected 2 default pipelines, got %d", len(infos))
	}

	// Check for default pipeline
	_, ok := registry.Get("default")
	if !ok {
		t.Error("expected to find 'default' pipeline")
	}

	// Check for ground_truth pipeline
	_, ok = registry.Get("ground_truth")
	if !ok {
		t.Error("expected to find 'ground_truth' pipeline")
	}
}

func TestTransformRegistry_ReplaceExisting(t *testing.T) {
	registry := NewTransformRegistry()

	pipeline1 := NewTransformPipeline("test", "1.0")
	pipeline2 := NewTransformPipeline("test", "2.0")

	registry.Register(pipeline1)
	registry.Register(pipeline2)

	retrieved, ok := registry.Get("test")
	if !ok {
		t.Fatal("expected to find registered pipeline")
	}

	// Should have the second version
	if retrieved.Version != "2.0" {
		t.Errorf("expected version '2.0', got '%s'", retrieved.Version)
	}

	// Should only have one pipeline named "test"
	infos := registry.List()
	count := 0
	for _, info := range infos {
		if info.Name == "test" {
			count++
		}
	}

	if count != 1 {
		t.Errorf("expected 1 pipeline named 'test', got %d", count)
	}
}

func TestTransform_Names(t *testing.T) {
	tests := []struct {
		name      string
		transform Transform
		expected  string
	}{
		{
			name:      "normalise transform",
			transform: &NormaliseTransform{Metric: "test"},
			expected:  "normalise:test",
		},
		{
			name:      "clip transform",
			transform: &ClipTransform{Metric: "test"},
			expected:  "clip:test",
		},
		{
			name:      "log transform",
			transform: &LogScaleTransform{Metric: "test"},
			expected:  "log:test",
		},
		{
			name:      "weight transform",
			transform: &ClassWeightTransform{Metric: "test"},
			expected:  "weight:test",
		},
		{
			name:      "round modifier transform",
			transform: &RoundModifierTransform{Metric: "test"},
			expected:  "round_modifier:test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := tt.transform.Name()
			if name != tt.expected {
				t.Errorf("expected name '%s', got '%s'", tt.expected, name)
			}
		})
	}
}

func TestTransformPipeline_PreservesOtherMetrics(t *testing.T) {
	pipeline := NewTransformPipeline("test", "1.0",
		&ClipTransform{Metric: "value1", Min: 0.0, Max: 1.0},
	)

	metrics := map[string]float64{
		"value1": 2.0,
		"value2": 3.0,
		"value3": 4.0,
	}

	result := pipeline.Apply(metrics)

	// value1 should be clipped
	if result["value1"] != 1.0 {
		t.Errorf("expected value1 = 1.0, got %f", result["value1"])
	}

	// Other values should be preserved
	if result["value2"] != 3.0 {
		t.Errorf("expected value2 = 3.0, got %f", result["value2"])
	}
	if result["value3"] != 4.0 {
		t.Errorf("expected value3 = 4.0, got %f", result["value3"])
	}
}

func TestTransform_Immutability(t *testing.T) {
	tests := []struct {
		name      string
		transform Transform
		initial   map[string]float64
		expected  map[string]float64
	}{
		{
			name:      "normalise transform doesn't modify original",
			transform: &NormaliseTransform{Metric: "value", Min: 0.0, Max: 100.0},
			initial:   map[string]float64{"value": 50.0},
			expected:  map[string]float64{"value": 50.0}, // Original unchanged
		},
		{
			name:      "clip transform doesn't modify original",
			transform: &ClipTransform{Metric: "value", Min: 0.0, Max: 1.0},
			initial:   map[string]float64{"value": 2.0},
			expected:  map[string]float64{"value": 2.0}, // Original unchanged
		},
		{
			name:      "log transform doesn't modify original",
			transform: &LogScaleTransform{Metric: "value"},
			initial:   map[string]float64{"value": 10.0},
			expected:  map[string]float64{"value": 10.0}, // Original unchanged
		},
		{
			name:      "weight transform doesn't modify original",
			transform: &ClassWeightTransform{Metric: "value", Weight: 2.0},
			initial:   map[string]float64{"value": 5.0},
			expected:  map[string]float64{"value": 5.0}, // Original unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to verify against expected
			originalCopy := make(map[string]float64, len(tt.initial))
			for k, v := range tt.initial {
				originalCopy[k] = v
			}

			// Apply transform
			_ = tt.transform.Apply(tt.initial)

			// Verify original map is unchanged
			for k, expectedVal := range tt.expected {
				if tt.initial[k] != expectedVal {
					t.Errorf("original map was modified: expected %s = %f, got %f", k, expectedVal, tt.initial[k])
				}
			}
		})
	}
}

func TestRoundModifierTransform_Immutability(t *testing.T) {
	currentRound := 2
	transform := &RoundModifierTransform{
		Metric:       "value",
		Multiplier:   3.0,
		Round:        2,
		CurrentRound: &currentRound,
	}

	initial := map[string]float64{"value": 5.0}
	_ = transform.Apply(initial)

	// Verify original map is unchanged
	if initial["value"] != 5.0 {
		t.Errorf("original map was modified: expected value = 5.0, got %f", initial["value"])
	}
}
