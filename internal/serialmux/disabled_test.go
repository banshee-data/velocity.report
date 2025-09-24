package serialmux

import (
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
