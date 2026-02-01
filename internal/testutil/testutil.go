// Package testutil provides shared test utilities and fixtures.
//
// This package centralises common test helpers to reduce code duplication
// across test files and improve test maintainability.
package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// AssertStatusCode checks that the response status code matches expected.
func AssertStatusCode(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("status code = %d, want %d", got, want)
	}
}

// AssertNoError fails the test if err is not nil.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// AssertError fails the test if err is nil.
func AssertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// NewTestRequest creates a test HTTP request.
func NewTestRequest(method, path string) *http.Request {
	return httptest.NewRequest(method, path, nil)
}

// NewTestRecorder creates a test response recorder.
func NewTestRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}
