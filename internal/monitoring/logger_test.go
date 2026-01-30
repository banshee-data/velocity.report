package monitoring

import (
	"testing"
)

func TestSetLogger(t *testing.T) {
	// Save original logger
	original := Logf
	defer func() { Logf = original }()

	// Test setting a custom logger
	called := false
	customLogger := func(format string, v ...interface{}) {
		called = true
	}

	SetLogger(customLogger)
	Logf("test message")

	if !called {
		t.Error("Custom logger was not called")
	}

	// Test setting nil logger (should create no-op)
	SetLogger(nil)
	// This should not panic
	Logf("test message")

	// Verify the logger is a no-op by checking it doesn't panic
	// and doesn't call anything
	noOpCalled := false
	testLogger := func(format string, v ...interface{}) {
		noOpCalled = true
	}
	SetLogger(testLogger)
	// First verify our test logger works
	Logf("test")
	if !noOpCalled {
		t.Error("Test logger should have been called")
	}

	// Now set to nil and verify it doesn't call our logger
	noOpCalled = false
	SetLogger(nil)
	Logf("test")
	if noOpCalled {
		t.Error("No-op logger should not have triggered callback")
	}
}

func TestLogf_Default(t *testing.T) {
	// Test that Logf is not nil by default
	if Logf == nil {
		t.Error("Logf should not be nil by default")
	}

	// Test that we can call it without panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Logf panicked: %v", r)
		}
	}()

	Logf("test message: %s", "value")
}
