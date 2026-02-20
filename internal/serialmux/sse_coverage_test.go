package serialmux

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestAttachAdminRoutes_TailSSE_DataStreaming exercises the SSE handler happy
// path: subscribe, receive data, then client disconnects.
func TestAttachAdminRoutes_TailSSE_DataStreaming(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	httpMux := http.NewServeMux()
	mux.AttachAdminRoutes(httpMux)

	// Use httptest.Server so we get real HTTP with client-side context control.
	ts := httptest.NewServer(httpMux)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/debug/tail", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected text/event-stream, got %s", ct)
	}

	// Read the initial ping comment
	scanner := bufio.NewScanner(resp.Body)
	if scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, ": ping") {
			t.Errorf("expected initial ping, got %q", line)
		}
	}

	// Push data through subscriber system
	mux.subscriberMu.Lock()
	for _, ch := range mux.subscribers {
		select {
		case ch <- "hello-sse":
		default:
		}
	}
	mux.subscriberMu.Unlock()

	// Read the SSE data line (skip blank lines between events)
	gotData := false
	for i := 0; i < 5 && scanner.Scan(); i++ {
		line := scanner.Text()
		if strings.Contains(line, "hello-sse") {
			gotData = true
			break
		}
	}
	if !gotData {
		t.Error("did not receive SSE data event")
	}

	// Cancel context to trigger client disconnect path
	cancel()
}

// TestAttachAdminRoutes_TailSSE_ContextCancelled exercises the context
// cancellation path in the SSE handler.
func TestAttachAdminRoutes_TailSSE_ContextCancelled(t *testing.T) {
	port := NewTestSerialPort("")
	mux := NewSerialMux(port)

	httpMux := http.NewServeMux()
	mux.AttachAdminRoutes(httpMux)

	ts := httptest.NewServer(httpMux)
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/debug/tail", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Cancel quickly to exercise context cancellation path
	cancel()
	resp.Body.Close()
}
