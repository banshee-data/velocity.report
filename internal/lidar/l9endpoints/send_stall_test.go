package l9endpoints

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/pb"
)

// blockingStreamServer is a client that has stopped reading: Send never
// returns until the test releases it, which is what gRPC flow control does to
// a real stream whose peer is not draining.
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

func (b *blockingStreamServer) sendCalls() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

func (b *blockingStreamServer) Context() context.Context        { return b.ctx }
func (b *blockingStreamServer) SetHeader(md metadata.MD) error  { return nil }
func (b *blockingStreamServer) SendHeader(md metadata.MD) error { return nil }
func (b *blockingStreamServer) SetTrailer(md metadata.MD)       {}
func (b *blockingStreamServer) SendMsg(msg interface{}) error   { return nil }
func (b *blockingStreamServer) RecvMsg(msg interface{}) error   { return nil }

// TestStreamEndsWhenTheClientStopsReading is the regression guard for the
// four-minute stall of 2026-08-26. stream.Send carries no deadline of its own,
// so a client that stops reading blocks the stream loop indefinitely — and
// because every piece of send instrumentation runs after Send returns, the
// stall produces no diagnostic at all. The loop must give up instead.
func TestStreamEndsWhenTheClientStopsReading(t *testing.T) {
	original := sendStallTimeout
	sendStallTimeout = 100 * time.Millisecond
	t.Cleanup(func() { sendStallTimeout = original })

	pub := NewPublisher(Config{SensorID: "test-sensor"})
	if err := pub.Start(); err != nil {
		t.Fatalf("start publisher: %v", err)
	}
	t.Cleanup(pub.Stop)
	srv := NewServer(pub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newBlockingStreamServer(ctx)
	defer close(stream.release)

	// Feed one frame so the loop reaches a send.
	go func() {
		time.Sleep(10 * time.Millisecond)
		pub.Publish(&FrameBundle{FrameID: 1, SensorID: "test-sensor"})
	}()

	done := make(chan error, 1)
	go func() {
		done <- srv.streamFromPublisher(ctx, &pb.StreamRequest{SensorId: "all"}, stream)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stream returned nil; a client that stopped reading must end the stream")
		}
		if got := status.Code(err); got != codes.DeadlineExceeded {
			t.Errorf("status code = %v, want %v (err=%v)", got, codes.DeadlineExceeded, err)
		}
		if stream.sendCalls() != 1 {
			t.Errorf("Send called %d times, want 1: gRPC forbids a concurrent Send, so the loop must not retry", stream.sendCalls())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not return: a blocked Send still stalls the loop indefinitely")
	}
}

// TestStreamReturnsWhenContextIsCancelledDuringASend covers shutdown while a
// send is outstanding: the loop must not wait out the stall timeout first.
func TestStreamReturnsWhenContextIsCancelledDuringASend(t *testing.T) {
	original := sendStallTimeout
	sendStallTimeout = 10 * time.Second
	t.Cleanup(func() { sendStallTimeout = original })

	pub := NewPublisher(Config{SensorID: "test-sensor"})
	if err := pub.Start(); err != nil {
		t.Fatalf("start publisher: %v", err)
	}
	t.Cleanup(pub.Stop)
	srv := NewServer(pub)

	ctx, cancel := context.WithCancel(context.Background())
	stream := newBlockingStreamServer(ctx)
	defer close(stream.release)

	go func() {
		time.Sleep(10 * time.Millisecond)
		pub.Publish(&FrameBundle{FrameID: 1, SensorID: "test-sensor"})
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		done <- srv.streamFromPublisher(ctx, &pb.StreamRequest{SensorId: "all"}, stream)
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not interrupt an outstanding send")
	}
}
