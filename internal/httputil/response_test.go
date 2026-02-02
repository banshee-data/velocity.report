package httputil

import (
	"encoding/json"
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
