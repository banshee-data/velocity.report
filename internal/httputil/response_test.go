package httputil

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSONError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	WriteJSONError(rec, http.StatusBadRequest, "test error")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %s, want application/json", ct)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["error"] != "test error" {
		t.Errorf("error = %s, want 'test error'", resp["error"])
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	data := map[string]string{"message": "hello"}
	WriteJSON(rec, http.StatusCreated, data)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["message"] != "hello" {
		t.Errorf("message = %s, want 'hello'", resp["message"])
	}
}

func TestWriteJSONOK(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	data := map[string]int{"count": 42}
	WriteJSONOK(rec, data)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["count"] != 42 {
		t.Errorf("count = %d, want 42", resp["count"])
	}
}

func TestMethodNotAllowed(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	MethodNotAllowed(rec)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestBadRequest(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	BadRequest(rec, "invalid input")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestInternalServerError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	InternalServerError(rec, "something went wrong")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestNotFound(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	NotFound(rec, "resource not found")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// failWriter wraps a ResponseRecorder but returns an error on Write after
// a configurable number of successful calls. This forces json.Encoder.Encode
// to fail so we can exercise the log.Printf error branches.
type failWriter struct {
	*httptest.ResponseRecorder
	writeCount int
	failAfter  int
}

func (fw *failWriter) Write(b []byte) (int, error) {
	fw.writeCount++
	if fw.writeCount > fw.failAfter {
		return 0, fmt.Errorf("simulated write failure")
	}
	return fw.ResponseRecorder.Write(b)
}

func TestWriteJSON_EncodeError(t *testing.T) {
	t.Parallel()

	// math.Inf(1) cannot be marshalled to JSON; Encode will return an error.
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, math.Inf(1))

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %s, want application/json", ct)
	}
}

func TestWriteJSONError_EncodeError(t *testing.T) {
	t.Parallel()

	// Use a writer whose Write method fails after the first call so that
	// json.Encoder.Encode returns an error for WriteJSONError.
	w := &failWriter{ResponseRecorder: httptest.NewRecorder(), failAfter: 0}
	WriteJSONError(w, http.StatusBadRequest, "test")
	// Ensure no panic — the error is logged internally.
}
