package network

import (
	"net"
	"time"
)

// UDPSocket defines an interface for UDP socket operations.
// This abstraction enables unit testing without real network connections.
type UDPSocket interface {
	// ReadFromUDP reads a UDP packet from the socket.
	ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error)

	// SetReadBuffer sets the size of the operating system's receive buffer.
	SetReadBuffer(bytes int) error

	// SetReadDeadline sets the deadline for future Read calls.
	SetReadDeadline(t time.Time) error

	// Close closes the socket.
	Close() error

	// LocalAddr returns the local network address.
	LocalAddr() net.Addr
}

// UDPSocketFactory defines an interface for creating UDP sockets.
// This abstraction enables dependency injection of socket creation.
type UDPSocketFactory interface {
	// ListenUDP creates and returns a new UDP socket.
	ListenUDP(network string, laddr *net.UDPAddr) (UDPSocket, error)
}

// RealUDPSocket wraps *net.UDPConn to implement UDPSocket.
type RealUDPSocket struct {
	conn *net.UDPConn
}

// NewRealUDPSocket wraps an existing *net.UDPConn.
func NewRealUDPSocket(conn *net.UDPConn) *RealUDPSocket {
	return &RealUDPSocket{conn: conn}
}

// ReadFromUDP reads from the UDP connection.
func (r *RealUDPSocket) ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error) {
	return r.conn.ReadFromUDP(b)
}

// SetReadBuffer sets the receive buffer size.
func (r *RealUDPSocket) SetReadBuffer(bytes int) error {
	return r.conn.SetReadBuffer(bytes)
}

// SetReadDeadline sets the read deadline.
func (r *RealUDPSocket) SetReadDeadline(t time.Time) error {
	return r.conn.SetReadDeadline(t)
}

// Close closes the UDP connection.
func (r *RealUDPSocket) Close() error {
	return r.conn.Close()
}

// LocalAddr returns the local network address.
func (r *RealUDPSocket) LocalAddr() net.Addr {
	return r.conn.LocalAddr()
}

// RealUDPSocketFactory implements UDPSocketFactory using net.ListenUDP.
type RealUDPSocketFactory struct{}

// NewRealUDPSocketFactory creates a new RealUDPSocketFactory.
func NewRealUDPSocketFactory() *RealUDPSocketFactory {
	return &RealUDPSocketFactory{}
}

// ListenUDP creates a new UDP socket.
func (f *RealUDPSocketFactory) ListenUDP(network string, laddr *net.UDPAddr) (UDPSocket, error) {
	conn, err := net.ListenUDP(network, laddr)
	if err != nil {
		return nil, err
	}
	return NewRealUDPSocket(conn), nil
}

// MockUDPSocket implements UDPSocket for testing.
type MockUDPSocket struct {
	// Packets holds the packets to return from ReadFromUDP.
	Packets []MockUDPPacket
	// ReadIndex tracks the current position in Packets.
	ReadIndex int
	// Closed indicates whether Close was called.
	Closed bool
	// ReadBufferSize holds the value set by SetReadBuffer.
	ReadBufferSize int
	// ReadDeadline holds the value set by SetReadDeadline.
	ReadDeadline time.Time
	// LocalAddress is returned by LocalAddr.
	LocalAddress *net.UDPAddr
	// ReadError is returned on the next ReadFromUDP call if set.
	ReadError error
	// SetReadBufferError is returned by SetReadBuffer if set.
	SetReadBufferError error
}

// MockUDPPacket represents a packet for mock testing.
type MockUDPPacket struct {
	Data []byte
	Addr *net.UDPAddr
}

// NewMockUDPSocket creates a new MockUDPSocket with the given packets.
func NewMockUDPSocket(packets []MockUDPPacket) *MockUDPSocket {
	return &MockUDPSocket{
		Packets: packets,
		LocalAddress: &net.UDPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 2368,
		},
	}
}

// ReadFromUDP returns the next packet from the mock buffer.
func (m *MockUDPSocket) ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error) {
	if m.Closed {
		return 0, nil, net.ErrClosed
	}
	if m.ReadError != nil {
		err := m.ReadError
		m.ReadError = nil
		return 0, nil, err
	}
	if m.ReadIndex >= len(m.Packets) {
		// Simulate timeout when no more packets
		return 0, nil, &net.OpError{
			Op:  "read",
			Net: "udp",
			Err: &timeoutError{},
		}
	}
	pkt := m.Packets[m.ReadIndex]
	m.ReadIndex++
	n = copy(b, pkt.Data)
	return n, pkt.Addr, nil
}

// SetReadBuffer records the buffer size.
func (m *MockUDPSocket) SetReadBuffer(bytes int) error {
	if m.SetReadBufferError != nil {
		return m.SetReadBufferError
	}
	m.ReadBufferSize = bytes
	return nil
}

// SetReadDeadline records the deadline.
func (m *MockUDPSocket) SetReadDeadline(t time.Time) error {
	m.ReadDeadline = t
	return nil
}

// Close marks the socket as closed.
func (m *MockUDPSocket) Close() error {
	m.Closed = true
	return nil
}

// LocalAddr returns the mock local address.
func (m *MockUDPSocket) LocalAddr() net.Addr {
	return m.LocalAddress
}

// Reset resets the mock socket state for reuse.
func (m *MockUDPSocket) Reset() {
	m.ReadIndex = 0
	m.Closed = false
	m.ReadBufferSize = 0
	m.ReadDeadline = time.Time{}
	m.ReadError = nil
}

// MockUDPSocketFactory implements UDPSocketFactory for testing.
type MockUDPSocketFactory struct {
	// Socket is the socket to return from ListenUDP.
	Socket *MockUDPSocket
	// Error is returned by ListenUDP if set.
	Error error
	// ListenCalls records all ListenUDP calls.
	ListenCalls []MockListenCall
}

// MockListenCall records a call to ListenUDP.
type MockListenCall struct {
	Network string
	Addr    *net.UDPAddr
}

// NewMockUDPSocketFactory creates a new MockUDPSocketFactory.
func NewMockUDPSocketFactory(socket *MockUDPSocket) *MockUDPSocketFactory {
	return &MockUDPSocketFactory{Socket: socket}
}

// ListenUDP returns the configured mock socket.
func (f *MockUDPSocketFactory) ListenUDP(network string, laddr *net.UDPAddr) (UDPSocket, error) {
	f.ListenCalls = append(f.ListenCalls, MockListenCall{
		Network: network,
		Addr:    laddr,
	})
	if f.Error != nil {
		return nil, f.Error
	}
	return f.Socket, nil
}

// timeoutError implements net.Error for timeout simulation.
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
