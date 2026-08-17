package network

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// opsCapture is a concurrency-safe io.Writer for capturing the ops log.
type opsCapture struct {
	mu sync.Mutex
	b  strings.Builder
}

func (o *opsCapture) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.Write(p)
}

func (o *opsCapture) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.String()
}

// TestPacketForwarder_LogsDroppedPacketsOnWriteError covers the write-failure
// and periodic-reporting branches of the forwarding worker.
//
// Pointing at a closed port is not enough: UDP is connectionless, so a send to
// a dead port normally succeeds at the syscall level. Closing the connection
// itself makes every Write fail deterministically.
func TestPacketForwarder_LogsDroppedPacketsOnWriteError(t *testing.T) {
	var ops opsCapture
	lidar.SetLogWriters(lidar.LogWriters{Ops: &ops})
	t.Cleanup(func() { lidar.SetLogWriters(lidar.LogWriters{}) })

	stats := &MockPacketStats{}
	forwarder, err := NewPacketForwarder("127.0.0.1", 12399, stats, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("NewPacketForwarder: %v", err)
	}
	// Force every subsequent Write to fail.
	if err := forwarder.conn.Close(); err != nil {
		t.Fatalf("closing conn: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	forwarder.Start(ctx)

	for i := 0; i < 5; i++ {
		forwarder.ForwardAsync([]byte("packet"))
	}

	// Wait for at least one ticker window so the drop report is emitted.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(ops.String(), "Dropped") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	got := ops.String()
	if !strings.Contains(got, "Dropped") {
		t.Fatalf("ops log missing the dropped-packet report\n---\n%s", got)
	}
	if !strings.Contains(got, "PacketForwarder") {
		t.Errorf("ops log missing the PacketForwarder tag\n---\n%s", got)
	}
}

// TestPacketForwarder_DropReportResetsBetweenIntervals checks that the counter
// is cleared after reporting, so a single burst is not re-reported forever.
func TestPacketForwarder_DropReportResetsBetweenIntervals(t *testing.T) {
	var ops opsCapture
	lidar.SetLogWriters(lidar.LogWriters{Ops: &ops})
	t.Cleanup(func() { lidar.SetLogWriters(lidar.LogWriters{}) })

	stats := &MockPacketStats{}
	forwarder, err := NewPacketForwarder("127.0.0.1", 12398, stats, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("NewPacketForwarder: %v", err)
	}
	if err := forwarder.conn.Close(); err != nil {
		t.Fatalf("closing conn: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	forwarder.Start(ctx)

	forwarder.ForwardAsync([]byte("packet"))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(ops.String(), "Dropped") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	first := strings.Count(ops.String(), "Dropped")
	if first == 0 {
		t.Fatalf("ops log missing the dropped-packet report\n---\n%s", ops.String())
	}

	// Let several further ticker windows elapse with no new traffic.
	time.Sleep(100 * time.Millisecond)

	if got := strings.Count(ops.String(), "Dropped"); got != first {
		t.Errorf("drop reports = %d, want it to stay at %d once the counter reset", got, first)
	}
}
