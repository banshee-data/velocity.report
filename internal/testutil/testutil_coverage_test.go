package testutil

import (
	"errors"
	"net/http"
	"testing"
)

// TestAssertStatusCode_Matching tests matching status codes (no failure).
func TestAssertStatusCode_Matching(t *testing.T) {
	fakeT := &testing.T{}
	AssertStatusCode(fakeT, http.StatusOK, http.StatusOK)
	if fakeT.Failed() {
		t.Error("expected no failure for matching status codes")
	}
}

// TestAssertNoError_NilErr tests nil error path.
func TestAssertNoError_NilErr(t *testing.T) {
	fakeT := &testing.T{}
	AssertNoError(fakeT, nil)
	if fakeT.Failed() {
		t.Error("expected no failure for nil error")
	}
}

// TestAssertError_WithErr tests non-nil error path.
func TestAssertError_WithErr(t *testing.T) {
	fakeT := &testing.T{}
	AssertError(fakeT, errors.New("something wrong"))
	if fakeT.Failed() {
		t.Error("expected no failure when error is present")
	}
}

// TestNewTestRequest_MethodAndPath verifies method and path are set.
func TestNewTestRequest_MethodAndPath(t *testing.T) {
	req := NewTestRequest(http.MethodPost, "/api/test")
	if req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.Method)
	}
	if req.URL.Path != "/api/test" {
		t.Errorf("path = %s, want /api/test", req.URL.Path)
	}
}

// TestNewTestRequest_GET tests GET request creation.
func TestNewTestRequest_GET(t *testing.T) {
	req := NewTestRequest(http.MethodGet, "/")
	if req.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", req.Method)
	}
}

// TestNewTestRecorder_InitialState verifies the recorder starts clean.
func TestNewTestRecorder_InitialState(t *testing.T) {
	w := NewTestRecorder()
	if w.Code != http.StatusOK {
		t.Errorf("initial Code = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.Len() != 0 {
		t.Errorf("initial body length = %d, want 0", w.Body.Len())
	}
}

// TestNewTestRecorder_WriteHeader verifies header writing.
func TestNewTestRecorder_WriteHeader(t *testing.T) {
	w := NewTestRecorder()
	w.WriteHeader(http.StatusNotFound)
	if w.Code != http.StatusNotFound {
		t.Errorf("Code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestNewTestRequest_DELETE tests DELETE request creation.
func TestNewTestRequest_DELETE(t *testing.T) {
	req := NewTestRequest(http.MethodDelete, "/api/resource/123")
	if req.Method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", req.Method)
	}
	if req.URL.Path != "/api/resource/123" {
		t.Errorf("path = %s, want /api/resource/123", req.URL.Path)
	}
}

// TestNewTestRequest_PUT tests PUT request creation.
func TestNewTestRequest_PUT(t *testing.T) {
	req := NewTestRequest(http.MethodPut, "/api/update")
	if req.Method != http.MethodPut {
		t.Errorf("method = %s, want PUT", req.Method)
	}
}
