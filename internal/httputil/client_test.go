package httputil

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStandardClient_Wraps(t *testing.T) {
	customClient := &http.Client{}
	client := NewStandardClient(customClient)

	if client.Client != customClient {
		t.Error("expected custom client to be wrapped")
	}
}

func TestMockHTTPClient_AddResponse(t *testing.T) {
	mock := NewMockHTTPClient()
	mock.AddResponse(http.StatusOK, "hello")
	mock.AddResponse(http.StatusNotFound, "not found")

	if len(mock.Responses) != 2 {
		t.Fatalf("got %d responses, want 2", len(mock.Responses))
	}
}

func TestMockHTTPClient_Get(t *testing.T) {
	mock := NewMockHTTPClient()
	mock.AddResponse(http.StatusOK, `{"result": "success"}`)

	resp, err := mock.Get("http://example.com/api")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"result": "success"}` {
		t.Errorf("got body %q", string(body))
	}

	if mock.RequestCount() != 1 {
		t.Errorf("got %d requests, want 1", mock.RequestCount())
	}
}

func TestMockHTTPClient_Post(t *testing.T) {
	mock := NewMockHTTPClient()
	mock.AddResponse(http.StatusCreated, `{"id": 123}`)

	resp, err := mock.Post("http://example.com/api", "application/json", strings.NewReader(`{"name": "test"}`))
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	req := mock.GetRequest(0)
	if req == nil {
		t.Fatal("expected request to be recorded")
	}

	if req.Method != http.MethodPost {
		t.Errorf("got method %s, want POST", req.Method)
	}

	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("got Content-Type %q", req.Header.Get("Content-Type"))
	}
}

func TestMockHTTPClient_MultipleResponses(t *testing.T) {
	mock := NewMockHTTPClient()
	mock.AddResponse(http.StatusOK, "first")
	mock.AddResponse(http.StatusAccepted, "second")
	mock.AddResponse(http.StatusNoContent, "third")

	// First request
	resp1, _ := mock.Get("http://example.com/1")
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	if string(body1) != "first" {
		t.Errorf("first response: got %q, want 'first'", string(body1))
	}

	// Second request
	resp2, _ := mock.Get("http://example.com/2")
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if string(body2) != "second" {
		t.Errorf("second response: got %q, want 'second'", string(body2))
	}

	// Third request
	resp3, _ := mock.Get("http://example.com/3")
	if resp3.StatusCode != http.StatusNoContent {
		t.Errorf("third response: got status %d, want %d", resp3.StatusCode, http.StatusNoContent)
	}
	resp3.Body.Close()
}

func TestMockHTTPClient_AddErrorResponse(t *testing.T) {
	mock := NewMockHTTPClient()
	expectedErr := errors.New("connection refused")
	mock.AddErrorResponse(expectedErr)

	_, err := mock.Get("http://example.com/api")
	if err != expectedErr {
		t.Errorf("got error %v, want %v", err, expectedErr)
	}
}

func TestMockHTTPClient_DefaultError(t *testing.T) {
	mock := NewMockHTTPClient()
	expectedErr := errors.New("network error")
	mock.DefaultError = expectedErr

	_, err := mock.Get("http://example.com/api")
	if err != expectedErr {
		t.Errorf("got error %v, want %v", err, expectedErr)
	}
}

func TestMockHTTPClient_DoFunc(t *testing.T) {
	mock := NewMockHTTPClient()
	mock.DoFunc = func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTeapot,
			Body:       io.NopCloser(strings.NewReader("custom")),
			Request:    req,
		}, nil
	}

	resp, _ := mock.Get("http://example.com/api")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTeapot {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusTeapot)
	}
}

func TestMockHTTPClient_GetRequest(t *testing.T) {
	mock := NewMockHTTPClient()
	mock.AddResponse(http.StatusOK, "")
	mock.AddResponse(http.StatusOK, "")
	mock.Get("http://example.com/first")
	mock.Get("http://example.com/second")

	req0 := mock.GetRequest(0)
	if req0 == nil || !strings.Contains(req0.URL.String(), "first") {
		t.Error("GetRequest(0) should return first request")
	}

	req1 := mock.GetRequest(1)
	if req1 == nil || !strings.Contains(req1.URL.String(), "second") {
		t.Error("GetRequest(1) should return second request")
	}

	reqNil := mock.GetRequest(99)
	if reqNil != nil {
		t.Error("GetRequest with out of bounds index should return nil")
	}

	reqNeg := mock.GetRequest(-1)
	if reqNeg != nil {
		t.Error("GetRequest with negative index should return nil")
	}
}

func TestMockHTTPClient_Reset(t *testing.T) {
	mock := NewMockHTTPClient()
	mock.AddResponse(http.StatusOK, "test")
	mock.DefaultError = errors.New("error")
	mock.Get("http://example.com/api")
	mock.Reset()

	if len(mock.Requests) != 0 {
		t.Error("Reset should clear requests")
	}

	if len(mock.Responses) != 0 {
		t.Error("Reset should clear responses")
	}

	if mock.DefaultError != nil {
		t.Error("Reset should clear DefaultError")
	}
}

func TestMockHTTPClient_DefaultResponse(t *testing.T) {
	// When no responses are queued and no error is set, should return empty 200
	mock := NewMockHTTPClient()

	resp, err := mock.Get("http://example.com/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestStandardClient_Do(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("accepted"))
	}))
	defer server.Close()

	client := NewStandardClient(nil)
	req, err := http.NewRequest(http.MethodPut, server.URL+"/resource", strings.NewReader("data"))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "accepted" {
		t.Errorf("got body %q, want 'accepted'", string(body))
	}
}

func TestStandardClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/data" {
			t.Errorf("expected path /api/data, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	client := NewStandardClient(nil)
	resp, err := client.Get(server.URL + "/api/data")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"status": "ok"}` {
		t.Errorf("got body %q", string(body))
	}
}

func TestStandardClient_Post(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"name": "test"}` {
			t.Errorf("expected body '{\"name\": \"test\"}', got %s", string(body))
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 123}`))
	}))
	defer server.Close()

	client := NewStandardClient(nil)
	resp, err := client.Post(server.URL+"/api/create", "application/json", strings.NewReader(`{"name": "test"}`))
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	respBody, _ := io.ReadAll(resp.Body)
	if string(respBody) != `{"id": 123}` {
		t.Errorf("got body %q", string(respBody))
	}
}

func TestStandardClient_WithCustomClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Test with a custom http.Client
	customClient := &http.Client{}
	client := NewStandardClient(customClient)

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
