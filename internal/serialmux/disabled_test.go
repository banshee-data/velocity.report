package serialmux

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDisabledSerialMux_UnsubscribeClosesChannel(t *testing.T) {
	d := NewDisabledSerialMux()
	id, ch := d.Subscribe()

	done := make(chan struct{})
	go func() {
		_, ok := <-ch
		if ok {
			t.Errorf("expected channel to be closed on unsubscribe")
		}
		close(done)
	}()

	// Give goroutine a moment to start and block on read
	time.Sleep(10 * time.Millisecond)

	d.Unsubscribe(id)

	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for subscriber to be unblocked after Unsubscribe")
	}
}

func TestDisabledSerialMux_CloseClosesAllChannels(t *testing.T) {
	d := NewDisabledSerialMux()
	id1, ch1 := d.Subscribe()
	_, ch2 := d.Subscribe()

	done1 := make(chan struct{})
	done2 := make(chan struct{})

	go func() {
		_, ok := <-ch1
		if ok {
			t.Errorf("expected ch1 to be closed on Close")
		}
		close(done1)
	}()

	go func() {
		_, ok := <-ch2
		if ok {
			t.Errorf("expected ch2 to be closed on Close")
		}
		close(done2)
	}()

	// Give goroutines a moment to start and block on read
	time.Sleep(10 * time.Millisecond)

	if err := d.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	select {
	case <-done1:
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for ch1 to be closed after Close")
	}

	select {
	case <-done2:
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for ch2 to be closed after Close")
	}

	// Ensure unsubscribing a non-existent id is a no-op (should not panic)
	d.Unsubscribe(id1)
}

func TestDisabledSerialMux_DoubleClose(t *testing.T) {
	d := NewDisabledSerialMux()
	_, _ = d.Subscribe()

	if err := d.Close(); err != nil {
		t.Fatalf("First Close returned error: %v", err)
	}

	// Second close should be a no-op and not panic
	if err := d.Close(); err != nil {
		t.Fatalf("Second Close returned error: %v", err)
	}
}

func TestDisabledSerialMux_SubscribeAfterClose(t *testing.T) {
	d := NewDisabledSerialMux()

	if err := d.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	// Subscribe after close should return a closed channel
	_, ch := d.Subscribe()

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Expected channel to be closed after subscribing to closed mux")
		}
	default:
		// Channel should be closed, so we should get the zero value immediately
	}
}

func TestDisabledSerialMux_SendCommand(t *testing.T) {
	d := NewDisabledSerialMux()

	// SendCommand should be a no-op that returns nil
	if err := d.SendCommand("test command"); err != nil {
		t.Errorf("SendCommand returned error: %v", err)
	}
}

func TestDisabledSerialMux_Monitor(t *testing.T) {
	d := NewDisabledSerialMux()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Monitor should block until context is cancelled
	err := d.Monitor(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Monitor returned %v, expected context.DeadlineExceeded", err)
	}
}

func TestDisabledSerialMux_Initialize(t *testing.T) {
	d := NewDisabledSerialMux()

	// Initialize should be a no-op that returns nil
	if err := d.Initialize(); err != nil {
		t.Errorf("Initialize returned error: %v", err)
	}
}

func TestDisabledSerialMux_AttachAdminRoutes(t *testing.T) {
	d := NewDisabledSerialMux()
	mux := http.NewServeMux()

	d.AttachAdminRoutes(mux)

	// Test that the route is registered and works
	req := httptest.NewRequest("GET", "/debug/serial-disabled", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", rec.Code)
	}

	if rec.Body.String() != "serial disabled" {
		t.Errorf("Expected 'serial disabled', got %q", rec.Body.String())
	}
}

func TestDisabledSerialMux_UnsubscribeNonExistent(t *testing.T) {
	d := NewDisabledSerialMux()

	// Unsubscribing a non-existent ID should not panic
	d.Unsubscribe("non-existent-id")
}
