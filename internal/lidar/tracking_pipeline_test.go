package lidar

import (
	"testing"
)

// mockForegroundForwarder implements ForegroundForwarder for testing
type mockForegroundForwarder struct {
	forwardCalled bool
	lastPoints    []PointPolar
}

func (m *mockForegroundForwarder) ForwardForeground(points []PointPolar) {
	m.forwardCalled = true
	m.lastPoints = points
}

func TestIsNilInterface(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		{
			name:     "nil interface",
			value:    nil,
			expected: true,
		},
		{
			name:     "nil pointer",
			value:    (*mockForegroundForwarder)(nil),
			expected: true,
		},
		{
			name:     "nil slice",
			value:    ([]int)(nil),
			expected: true,
		},
		{
			name:     "nil map",
			value:    (map[string]int)(nil),
			expected: true,
		},
		{
			name:     "non-nil pointer",
			value:    &mockForegroundForwarder{},
			expected: false,
		},
		{
			name:     "non-nil value",
			value:    42,
			expected: false,
		},
		{
			name:     "non-nil string",
			value:    "hello",
			expected: false,
		},
		{
			name:     "empty slice (non-nil)",
			value:    []int{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNilInterface(tt.value)
			if result != tt.expected {
				t.Errorf("isNilInterface(%v) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestIsNilInterface_WithForegroundForwarder(t *testing.T) {
	// Test the specific case that caused the bug: nil pointer assigned to interface
	var fwd ForegroundForwarder

	// Case 1: uninitialized interface
	if !isNilInterface(fwd) {
		t.Error("expected nil interface to be detected as nil")
	}

	// Case 2: nil pointer assigned to interface (the bug case)
	var nilPtr *mockForegroundForwarder
	fwd = nilPtr
	if !isNilInterface(fwd) {
		t.Error("expected interface holding nil pointer to be detected as nil")
	}

	// Case 3: valid pointer assigned to interface
	validPtr := &mockForegroundForwarder{}
	fwd = validPtr
	if isNilInterface(fwd) {
		t.Error("expected interface holding valid pointer to be detected as non-nil")
	}
}

func TestTrackingPipelineConfig_NilForwarder(t *testing.T) {
	// Test that NewFrameCallback handles nil forwarder gracefully
	config := &TrackingPipelineConfig{
		BackgroundManager: nil, // Will cause early return, which is fine for this test
		FgForwarder:       nil,
		Tracker:           nil,
		Classifier:        nil,
		DB:                nil,
		SensorID:          "test-sensor",
		DebugMode:         false,
	}

	// This should not panic
	callback := config.NewFrameCallback()
	if callback == nil {
		t.Fatal("expected non-nil callback")
	}

	// Test with nil pointer assigned to interface (the bug scenario)
	var nilPtr *mockForegroundForwarder
	config.FgForwarder = nilPtr

	// This should also not panic
	callback = config.NewFrameCallback()
	if callback == nil {
		t.Fatal("expected non-nil callback with nil pointer forwarder")
	}
}

func TestTrackingPipelineConfig_WithValidForwarder(t *testing.T) {
	// Test that a valid forwarder is actually used
	mock := &mockForegroundForwarder{}

	config := &TrackingPipelineConfig{
		BackgroundManager: nil, // Will cause early return before forwarder is used
		FgForwarder:       mock,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()
	if callback == nil {
		t.Fatal("expected non-nil callback")
	}

	// Note: We can't easily test the full pipeline without mocking BackgroundManager,
	// but we've verified the callback is created without panicking
	if mock.forwardCalled {
		t.Error("forwarder should not be called during callback creation")
	}
}
