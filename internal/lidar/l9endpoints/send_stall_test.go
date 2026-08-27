package l9endpoints

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	pb "github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/pb"
)

// blockingStreamServer is a client that has stopped reading: Send never returns
// until the test releases it, which is what gRPC flow control does to a stream
// whose peer is not draining.
type blockingStreamServer struct {
	ctx     context.Context
	release chan struct{}
	mu      sync.Mutex
	calls   int
}

func newBlockingStreamServer(ctx context.Context) *blockingStreamServer {
	return &blockingStreamServer{ctx: ctx, release: make(chan struct{})}
}

func (b *blockingStreamServer) Send(frame *pb.FrameBundle) error {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	<-b.release
	return nil
}

func (b *blockingStreamServer) Context() context.Context        { return b.ctx }
func (b *blockingStreamServer) SetHeader(md metadata.MD) error  { return nil }
func (b *blockingStreamServer) SendHeader(md metadata.MD) error { return nil }
func (b *blockingStreamServer) SetTrailer(md metadata.MD)       {}
func (b *blockingStreamServer) SendMsg(msg interface{}) error   { return nil }
func (b *blockingStreamServer) RecvMsg(msg interface{}) error   { return nil }

// lockedBuffer is a log sink that is safe to poll while the logger writes to it
// from another goroutine.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitFor polls cond until it holds or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}

func startStalledStream(t *testing.T) (*blockingStreamServer, *lockedBuffer, chan error, context.CancelFunc) {
	t.Helper()

	opsBuf := &lockedBuffer{}
	SetLogWriters(opsBuf, nil, nil)
	t.Cleanup(func() { SetLogWriters(nil, nil, nil) })

	pub := NewPublisher(Config{SensorID: "test-sensor"})
	if err := pub.Start(); err != nil {
		t.Fatalf("start publisher: %v", err)
	}
	t.Cleanup(pub.Stop)
	srv := NewServer(pub)

	ctx, cancel := context.WithCancel(context.Background())
	stream := newBlockingStreamServer(ctx)

	go func() {
		time.Sleep(10 * time.Millisecond)
		pub.Publish(&FrameBundle{FrameID: 1, SensorID: "test-sensor"})
	}()

	done := make(chan error, 1)
	go func() {
		done <- srv.streamFromPublisher(ctx, &pb.StreamRequest{SensorId: "all"}, stream)
	}()

	return stream, opsBuf, done, cancel
}

// TestStreamReportsAStallWithoutSeveringIt is the regression guard for both
// halves of the 2026-08-26 investigation.
//
// stream.Send carries no deadline, so a client that stops reading blocks the
// stream loop — and because every piece of send instrumentation runs after Send
// returns, the stall produced no diagnostic at all. It has to be reported.
//
// It must not be acted on by closing the stream. That was tried and made things
// worse: a client that has stopped reading is usually one whose UI thread is
// blocked, and such a client cannot run its reconnect logic either. Severing
// left a replay streaming to nobody for two and a half minutes, where the same
// client had previously recovered on its own after four.
func TestStreamReportsAStallWithoutSeveringIt(t *testing.T) {
	original := sendStallTimeout
	sendStallTimeout = 50 * time.Millisecond
	t.Cleanup(func() { sendStallTimeout = original })

	stream, opsBuf, done, cancel := startStalledStream(t)
	defer cancel()

	waitFor(t, 2*time.Second, func() bool {
		return strings.Contains(opsBuf.String(), "has not read for")
	}, "the stall was never reported; a blocked send is invisible again")

	// The stream must still be alive.
	select {
	case err := <-done:
		t.Fatalf("stream returned %v while the client was only stalled; severing removes its chance to recover", err)
	case <-time.After(200 * time.Millisecond):
	}

	// Only one Send may be outstanding: gRPC forbids a concurrent SendMsg.
	stream.mu.Lock()
	calls := stream.calls
	stream.mu.Unlock()
	if calls != 1 {
		t.Errorf("Send called %d times, want 1 while one is still outstanding", calls)
	}

	// Once the client reads again the send completes and recovery is reported.
	close(stream.release)
	waitFor(t, 2*time.Second, func() bool {
		return strings.Contains(opsBuf.String(), "resumed reading after")
	}, "recovery was never reported")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not return after cancellation")
	}
}

// TestStreamReturnsWhenContextIsCancelledDuringASend covers a client that has
// genuinely gone away, which is the case that does end the stream — and the
// reason the stall path does not need to.
func TestStreamReturnsWhenContextIsCancelledDuringASend(t *testing.T) {
	original := sendStallTimeout
	sendStallTimeout = 10 * time.Second
	t.Cleanup(func() { sendStallTimeout = original })

	stream, _, done, cancel := startStalledStream(t)
	defer close(stream.release)

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not interrupt an outstanding send")
	}
}
