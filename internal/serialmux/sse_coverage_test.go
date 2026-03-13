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

	// Wait for the SSE handler's subscription to become visible, then send
	// directly to the subscriber channel. This avoids racing the handler setup
	// and avoids silently dropping the test payload via a default branch.
	var subscribers []chan string
	deadline := time.Now().Add(500 * time.Millisecond)
	for len(subscribers) == 0 && time.Now().Before(deadline) {
		mux.subscriberMu.Lock()
		subscribers = make([]chan string, 0, len(mux.subscribers))
		for _, ch := range mux.subscribers {
			subscribers = append(subscribers, ch)
		}
		mux.subscriberMu.Unlock()
		if len(subscribers) == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if len(subscribers) == 0 {
		t.Fatal("expected an SSE subscriber to be registered")
	}

	for _, ch := range subscribers {
		select {
		case ch <- "hello-sse":
		case <-time.After(250 * time.Millisecond):
			t.Fatal("timed out delivering test SSE payload")
		}
	}

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
