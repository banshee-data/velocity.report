package network

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// --- custom mock types for coverage-specific scenarios ---

// deadlineErrorSocket is a mock UDPSocket that returns an error from
// SetReadDeadline so we can exercise the warning log path in Start().
type deadlineErrorSocket struct {
	closed bool
}

func (s *deadlineErrorSocket) ReadFromUDP(b []byte) (int, *net.UDPAddr, error) {
	if s.closed {
		return 0, nil, net.ErrClosed
	}
	// Small delay prevents tight busy-looping when SetReadDeadline fails
	// (the mock returns instantly, unlike a real socket that would block).
	time.Sleep(10 * time.Millisecond)
	return 0, nil, &net.OpError{Op: "read", Net: "udp", Err: &timeoutError{}}
}

func (s *deadlineErrorSocket) SetReadBuffer(int) error { return nil }

func (s *deadlineErrorSocket) SetReadDeadline(time.Time) error {
	return errors.New("mock SetReadDeadline error")
}

func (s *deadlineErrorSocket) Close() error {
	s.closed = true
	return nil
}

func (s *deadlineErrorSocket) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 19001}
}

// deadlineErrorSocketFactory returns a deadlineErrorSocket from ListenUDP.
type deadlineErrorSocketFactory struct {
	socket *deadlineErrorSocket
}

func (f *deadlineErrorSocketFactory) ListenUDP(string, *net.UDPAddr) (UDPSocket, error) {
	return f.socket, nil
}

// ctxCancelSocket cancels a context and then returns a non-timeout read error
// so the Start loop hits the `ctx.Err() != nil` branch after a read failure.
type ctxCancelSocket struct {
	cancel context.CancelFunc
	called int
}

func (s *ctxCancelSocket) ReadFromUDP(b []byte) (int, *net.UDPAddr, error) {
	s.called++
	if s.called == 1 {
		// Cancel the context, then return a non-timeout error.
		s.cancel()
		return 0, nil, errors.New("connection reset by peer")
	}
	// Subsequent calls: timeout (shouldn't normally be reached).
	return 0, nil, &net.OpError{Op: "read", Net: "udp", Err: &timeoutError{}}
}

func (s *ctxCancelSocket) SetReadBuffer(int) error         { return nil }
func (s *ctxCancelSocket) SetReadDeadline(time.Time) error { return nil }
func (s *ctxCancelSocket) Close() error                    { return nil }
func (s *ctxCancelSocket) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 19002}
}

// ctxCancelSocketFactory returns a ctxCancelSocket from ListenUDP.
type ctxCancelSocketFactory struct {
	socket *ctxCancelSocket
}

func (f *ctxCancelSocketFactory) ListenUDP(string, *net.UDPAddr) (UDPSocket, error) {
	return f.socket, nil
}

// --- coverage tests ---

// TestListenerCov_Start_WithForwarder exercises the l.forwarder.Start(ctx)
// branch inside Start() which is otherwise uncovered.
func TestListenerCov_Start_WithForwarder(t *testing.T) {
	// Create a real PacketForwarder (writes to a local UDP endpoint).
	fwdStats := &MockPacketStats{}
	forwarder, err := NewPacketForwarder("127.0.0.1", 19010, fwdStats, time.Hour)
	if err != nil {
		t.Fatalf("NewPacketForwarder: %v", err)
	}
	defer forwarder.Close()

	// Use a mock socket that only returns timeouts (no real packets).
	mockSocket := NewMockUDPSocket(nil)
	mockFactory := NewMockUDPSocketFactory(mockSocket)

	stats := &MockFullPacketStats{}
	listener := NewUDPListener(UDPListenerConfig{
		Address:       "127.0.0.1:19011",
		RcvBuf:        65536,
		SocketFactory: mockFactory,
		Stats:         stats,
		Forwarder:     forwarder,
		LogInterval:   time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- listener.Start(ctx) }()

	// Let the listener run long enough to enter the loop (and call forwarder.Start).
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not exit after cancellation")
	}
}

// TestListenerCov_Start_SetReadDeadlineError exercises the log path when
// conn.SetReadDeadline returns an error inside Start()'s read loop.
func TestListenerCov_Start_SetReadDeadlineError(t *testing.T) {
	sock := &deadlineErrorSocket{}
	factory := &deadlineErrorSocketFactory{socket: sock}

	listener := NewUDPListener(UDPListenerConfig{
		Address:       "127.0.0.1:19012",
		RcvBuf:        65536,
		SocketFactory: factory,
		LogInterval:   time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- listener.Start(ctx) }()

	// Give time for at least one iteration that hits the SetReadDeadline error.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not exit after cancellation")
	}
}

// TestListenerCov_Start_ReadErrorCtxCancelled exercises the branch where a
// non-timeout read error is returned after the context has been cancelled,
// causing Start() to return ctx.Err() from the inner check.
func TestListenerCov_Start_ReadErrorCtxCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sock := &ctxCancelSocket{cancel: cancel}
	factory := &ctxCancelSocketFactory{socket: sock}

	listener := NewUDPListener(UDPListenerConfig{
		Address:       "127.0.0.1:19013",
		RcvBuf:        65536,
		SocketFactory: factory,
		LogInterval:   time.Hour,
	})

	done := make(chan error, 1)
	go func() { done <- listener.Start(ctx) }()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not exit")
	}
}

// TestListenerCov_HandlePacket_ParserNoFrameBuilder exercises handlePacket
// with a parser that succeeds but no FrameBuilder configured, covering the
// AddPoints call without entering the FrameBuilder blocks.
func TestListenerCov_HandlePacket_ParserNoFrameBuilder(t *testing.T) {
	stats := &MockFullPacketStats{}
	parser := &MockParser{
		points:     []lidar.PointPolar{{Distance: 5.0, Azimuth: 90.0}},
		motorSpeed: 600,
	}

	listener := NewUDPListener(UDPListenerConfig{
		Address: ":19014",
		RcvBuf:  65536,
		Stats:   stats,
		Parser:  parser,
		// FrameBuilder deliberately nil
	})

	if err := listener.handlePacket(make([]byte, 100)); err != nil {
		t.Fatalf("handlePacket error: %v", err)
	}
	if stats.GetPointCount() != 1 {
		t.Errorf("expected 1 point, got %d", stats.GetPointCount())
	}
}
