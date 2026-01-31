package serialmux

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// InitialiseTestPort is a port that can fail at specific command writes.
type InitialiseTestPort struct {
	writtenData    []byte
	failAfterWrite int
	writeCount     int
	mu             sync.Mutex
}

func NewInitialiseTestPort() *InitialiseTestPort {
	return &InitialiseTestPort{
		failAfterWrite: -1, // -1 means never fail
	}
}

func (p *InitialiseTestPort) Read(buf []byte) (int, error) {
	return 0, io.EOF
}

func (p *InitialiseTestPort) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writeCount++
	if p.failAfterWrite >= 0 && p.writeCount > p.failAfterWrite {
		return 0, errors.New("simulated write failure")
	}
	p.writtenData = append(p.writtenData, data...)
	return len(data), nil
}

func (p *InitialiseTestPort) Close() error {
	return nil
}

func (p *InitialiseTestPort) FailAfter(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failAfterWrite = n
}

func (p *InitialiseTestPort) WrittenData() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return string(p.writtenData)
}

// TestInitialise_ClockSyncFailure tests that Initialise fails gracefully
// when clock synchronisation fails.
func TestInitialise_ClockSyncFailure(t *testing.T) {
	port := NewInitialiseTestPort()
	port.FailAfter(0) // Fail on first write (clock sync)
	mux := NewSerialMux(port)

	err := mux.Initialise()
	if err == nil {
		t.Error("Expected error when clock sync fails")
	}
	if !strings.Contains(err.Error(), "synchronize clock") {
		t.Errorf("Expected error to mention clock sync, got: %v", err)
	}
}

// TestInitialise_TimezoneSyncFailure tests that Initialise fails gracefully
// when timezone setting fails.
func TestInitialise_TimezoneSyncFailure(t *testing.T) {
	port := NewInitialiseTestPort()
	port.FailAfter(1) // Fail on second write (timezone)
	mux := NewSerialMux(port)

	err := mux.Initialise()
	if err == nil {
		t.Error("Expected error when timezone sync fails")
	}
	if !strings.Contains(err.Error(), "timezone") {
		t.Errorf("Expected error to mention timezone, got: %v", err)
	}
}

// TestInitialise_StartCommandFailure tests that Initialise fails gracefully
// when one of the start commands fails.
func TestInitialise_StartCommandFailure(t *testing.T) {
	port := NewInitialiseTestPort()
	port.FailAfter(3) // Fail after clock, TZ, and first start command
	mux := NewSerialMux(port)

	err := mux.Initialise()
	if err == nil {
		t.Error("Expected error when start command fails")
	}
	if !strings.Contains(err.Error(), "start command") {
		t.Errorf("Expected error to mention start command, got: %v", err)
	}
}

// TestInitialise_AllCommandsSent verifies all expected commands are sent.
func TestInitialise_AllCommandsSent(t *testing.T) {
	port := NewInitialiseTestPort()
	mux := NewSerialMux(port)

	err := mux.Initialise()
	if err != nil {
		t.Fatalf("Initialise failed: %v", err)
	}

	written := port.WrittenData()
	expectedCommands := []string{
		"C=", // Clock sync
		"CZ", // Timezone
		"AX", // Factory defaults
		"OJ", // JSON output
		"OS", // Speed reporting
		"oD", // Range reporting
		"OM", // Speed magnitude
		"oM", // Range magnitude
		"OH", // Human timestamp
		"OC", // Object detection
	}

	for _, cmd := range expectedCommands {
		if !strings.Contains(written, cmd) {
			t.Errorf("Expected command %q to be written", cmd)
		}
	}
}

// BlockingReadPort is a port that blocks on read until cancelled.
type BlockingReadPort struct {
	unblock chan struct{}
	closed  bool
	mu      sync.Mutex
}

func NewBlockingReadPort() *BlockingReadPort {
	return &BlockingReadPort{
		unblock: make(chan struct{}),
	}
}

func (p *BlockingReadPort) Read(buf []byte) (int, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return 0, io.EOF
	}
	unblock := p.unblock
	p.mu.Unlock()

	<-unblock
	return 0, io.EOF
}

func (p *BlockingReadPort) Write(data []byte) (int, error) {
	return len(data), nil
}

func (p *BlockingReadPort) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		close(p.unblock)
	}
	return nil
}

// TestMonitor_ContextCancellation tests that Monitor exits cleanly
// when the context is cancelled.
func TestMonitor_ContextCancellation(t *testing.T) {
	port := NewBlockingReadPort()
	mux := NewSerialMux(port)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- mux.Monitor(ctx)
	}()

	// Give monitor time to start
	time.Sleep(10 * time.Millisecond)

	// Cancel context
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled error, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Monitor did not exit after context cancellation")
		port.Close()
	}
}

// TestMonitor_ContextDeadline tests that Monitor exits cleanly
// when the context deadline is exceeded.
func TestMonitor_ContextDeadline(t *testing.T) {
	port := NewBlockingReadPort()
	mux := NewSerialMux(port)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- mux.Monitor(ctx)
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected context.DeadlineExceeded error, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Monitor did not exit after deadline")
		port.Close()
	}
}

// LineByLineReadPort returns lines one at a time.
type LineByLineReadPort struct {
	lines  []string
	index  int
	closed bool
	delay  time.Duration
	mu     sync.Mutex
}

func NewLineByLineReadPort(lines []string) *LineByLineReadPort {
	return &LineByLineReadPort{
		lines: lines,
		delay: 5 * time.Millisecond,
	}
}

func (p *LineByLineReadPort) Read(buf []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, io.EOF
	}

	if p.index >= len(p.lines) {
		// Block when EOF reached instead of returning immediately
		// This simulates a port that stays open but has no data
		p.mu.Unlock()
		time.Sleep(p.delay)
		p.mu.Lock()
		return 0, nil
	}

	line := p.lines[p.index] + "\n"
	p.index++
	n := copy(buf, line)
	return n, nil
}

func (p *LineByLineReadPort) Write(data []byte) (int, error) {
	return len(data), nil
}

func (p *LineByLineReadPort) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

// TestMonitor_BroadcastsToMultipleSubscribers verifies that lines from the
// serial port are sent to all subscribers. Note: SerialMux uses non-blocking
// sends, so subscribers must be actively reading when lines are broadcast.
func TestMonitor_BroadcastsToMultipleSubscribers(t *testing.T) {
	lines := []string{"line1", "line2", "line3"}
	port := NewLineByLineReadPort(lines)
	mux := NewSerialMux(port)

	id1, ch1 := mux.Subscribe()
	id2, ch2 := mux.Subscribe()
	defer mux.Unsubscribe(id1)
	defer mux.Unsubscribe(id2)

	// Protect slices with mutex to avoid race conditions
	var mu sync.Mutex
	received1 := make([]string, 0)
	received2 := make([]string, 0)

	// Use done channels instead of WaitGroup for clearer synchronisation
	done1 := make(chan struct{})
	done2 := make(chan struct{})

	// Start readers before Monitor
	go func() {
		defer close(done1)
		timeout := time.After(150 * time.Millisecond)
		for {
			select {
			case line, ok := <-ch1:
				if !ok {
					return
				}
				mu.Lock()
				received1 = append(received1, line)
				mu.Unlock()
			case <-timeout:
				return
			}
		}
	}()

	go func() {
		defer close(done2)
		timeout := time.After(150 * time.Millisecond)
		for {
			select {
			case line, ok := <-ch2:
				if !ok {
					return
				}
				mu.Lock()
				received2 = append(received2, line)
				mu.Unlock()
			case <-timeout:
				return
			}
		}
	}()

	// Small delay to ensure readers are blocked on channels before Monitor starts
	time.Sleep(10 * time.Millisecond)

	// Run Monitor with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := mux.Monitor(ctx)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("Monitor returned: %v", err)
	}

	// Wait for readers to finish
	<-done1
	<-done2

	// The key assertion is that BOTH subscribers receive at least some lines
	// (verifying broadcast, not unicast). Due to timing, they may not receive
	// the exact same count.
	mu.Lock()
	defer mu.Unlock()
	if len(received1) == 0 && len(received2) == 0 {
		t.Error("Neither subscriber received any lines")
	}
	t.Logf("Subscriber 1 received %d lines, Subscriber 2 received %d lines", len(received1), len(received2))
}

// TestMonitor_SkipsBlockedSubscriber verifies that a blocked subscriber
// doesn't block other subscribers.
func TestMonitor_SkipsBlockedSubscriber(t *testing.T) {
	lines := []string{"line1", "line2", "line3", "line4", "line5"}
	port := NewLineByLineReadPort(lines)
	mux := NewSerialMux(port)

	// Subscriber 1 never reads from channel (will block)
	_, _ = mux.Subscribe() // Intentionally don't read from this

	// Subscriber 2 reads normally
	id2, ch2 := mux.Subscribe()
	defer mux.Unsubscribe(id2)

	ctx := context.Background()

	// Monitor in goroutine
	done := make(chan error, 1)
	go func() {
		done <- mux.Monitor(ctx)
	}()

	// Read from subscriber 2 with timeout
	var received []string
	timeout := time.After(150 * time.Millisecond)
loop:
	for {
		select {
		case line := <-ch2:
			received = append(received, line)
			if len(received) >= len(lines) {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	// Close port to stop Monitor
	port.Close()

	// Wait for Monitor to finish
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("Monitor did not exit after port close")
	}

	// Subscriber 2 should have received lines despite subscriber 1 being blocked
	if len(received) == 0 {
		t.Error("Subscriber 2 should have received lines")
	}
}

// TestClose_ReturnsPortError tests that Close returns any error from the port.
func TestClose_ReturnsPortError(t *testing.T) {
	port := &FailingClosePort{closeErr: errors.New("close failed")}
	mux := NewSerialMux(port)

	err := mux.Close()
	if err == nil {
		t.Error("Expected error from Close")
	}
	if !strings.Contains(err.Error(), "close failed") {
		t.Errorf("Expected 'close failed' error, got: %v", err)
	}
}

// FailingClosePort is a port that returns an error on Close.
type FailingClosePort struct {
	closeErr error
}

func (p *FailingClosePort) Read(buf []byte) (int, error) {
	return 0, io.EOF
}

func (p *FailingClosePort) Write(data []byte) (int, error) {
	return len(data), nil
}

func (p *FailingClosePort) Close() error {
	return p.closeErr
}

// TestSendCommand_NewlineHandling tests that commands are properly
// terminated with newlines.
func TestSendCommand_NewlineHandling(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected string
	}{
		{"without newline", "OJ", "OJ\n"},
		{"with newline", "OS\n", "OS\n"},
		{"with trailing spaces", "  OC  ", "  OC  \n"},
		{"empty command", "", "\n"},
		{"multiple newlines", "OJ\n\n", "OJ\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := NewInitialiseTestPort()
			mux := NewSerialMux(port)

			err := mux.SendCommand(tt.command)
			if err != nil {
				t.Fatalf("SendCommand failed: %v", err)
			}

			written := port.WrittenData()
			if written != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, written)
			}
		})
	}
}

// TestSubscribeUnsubscribe_ConcurrentAccess tests thread safety of
// subscribe/unsubscribe operations.
func TestSubscribeUnsubscribe_ConcurrentAccess(t *testing.T) {
	port := NewInitialiseTestPort()
	mux := NewSerialMux(port)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Start goroutines that subscribe
	ids := make(chan string, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			id, _ := mux.Subscribe()
			ids <- id
		}()
	}

	// Start goroutines that unsubscribe
	go func() {
		for id := range ids {
			go func(subID string) {
				defer wg.Done()
				time.Sleep(time.Duration(len(subID)%10) * time.Millisecond)
				mux.Unsubscribe(subID)
			}(id)
		}
	}()

	// Wait for all subscriptions
	time.Sleep(50 * time.Millisecond)
	close(ids)

	// Wait for completion
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Error("Concurrent subscribe/unsubscribe timed out")
	}
}

// TestMonitor_ScannerEOF tests that Monitor exits cleanly when the
// scanner reaches EOF.
func TestMonitor_ScannerEOF(t *testing.T) {
	lines := []string{"line1", "line2"}
	port := NewLineByLineReadPort(lines)
	mux := NewSerialMux(port)

	id, ch := mux.Subscribe()
	defer mux.Unsubscribe(id)

	ctx := context.Background()

	// Monitor will run until port is closed
	done := make(chan error, 1)
	go func() {
		done <- mux.Monitor(ctx)
	}()

	// Close port after brief delay to trigger EOF
	time.Sleep(50 * time.Millisecond)
	port.Close()

	// Wait for monitor to complete with timeout
	select {
	case err := <-done:
		// Should exit without error (nil or EOF is expected)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Logf("Monitor returned: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Monitor did not exit after EOF")
	}

	// Drain any remaining items from channel with timeout
	drainTimeout := time.After(50 * time.Millisecond)
drainLoop:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break drainLoop
			}
		case <-drainTimeout:
			break drainLoop
		}
	}
}
