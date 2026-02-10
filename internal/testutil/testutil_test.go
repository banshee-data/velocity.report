package testutil

import (
	"errors"
	"net/http"
	"testing"
)

// TestAssertStatusCode verifies that AssertStatusCode executes without panicking.
// Note: Testing t.Errorf/t.Fatalf calls requires a mock testing.T implementation
// which adds complexity. These helpers are best validated through integration
// tests where they're actually used.
func TestAssertStatusCode(t *testing.T) {
	t.Parallel()

	// Verify the function executes without panicking for matching codes
	// We can't easily verify failure behavior without a mock T
	AssertStatusCode(t, http.StatusOK, http.StatusOK)
	AssertStatusCode(t, http.StatusNotFound, http.StatusNotFound)
}

func TestAssertStatusCode_FailurePath(t *testing.T) {
	t.Parallel()

	ok := t.Run("status mismatch", func(t *testing.T) {
		AssertStatusCode(t, http.StatusOK, http.StatusBadRequest)
	})
	if ok {
		t.Fatal("expected subtest to fail on mismatched status code")
	}
}

func TestAssertNoError(t *testing.T) {
	t.Parallel()

	// Verify nil error doesn't cause issues
	AssertNoError(t, nil)
}

func TestAssertNoError_FailurePath(t *testing.T) {
	t.Parallel()

	ok := t.Run("unexpected error", func(t *testing.T) {
		AssertNoError(t, errors.New("boom"))
	})
	if ok {
		t.Fatal("expected subtest to fail when error is non-nil")
	}
}

func TestAssertError(t *testing.T) {
	t.Parallel()

	// Verify non-nil error is handled correctly
	AssertError(t, errors.New("test error"))
}

func TestAssertError_FailurePath(t *testing.T) {
	t.Parallel()

	ok := t.Run("missing expected error", func(t *testing.T) {
		AssertError(t, nil)
	})
	if ok {
		t.Fatal("expected subtest to fail when error is nil")
	}
}

func TestNewTestRequest(t *testing.T) {
	t.Parallel()

	req := NewTestRequest("GET", "/test")
	if req.Method != "GET" {
		t.Errorf("method = %s, want GET", req.Method)
	}
	if req.URL.Path != "/test" {
		t.Errorf("path = %s, want /test", req.URL.Path)
	}
}

func TestNewTestRecorder(t *testing.T) {
	t.Parallel()

	rec := NewTestRecorder()
	if rec == nil {
		t.Fatal("recorder is nil")
	}
}
