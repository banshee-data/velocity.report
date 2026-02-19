package network

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestRealUDPSocketFactory_ListenUDP(t *testing.T) {
	factory := NewRealUDPSocketFactory()

	// Listen on a random port
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	socket, err := factory.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}
	defer socket.Close()

	// Verify we got a valid socket
	localAddr := socket.LocalAddr()
	if localAddr == nil {
		t.Error("Expected non-nil local address")
	}
}

func TestRealUDPSocket_SetReadBuffer(t *testing.T) {
	factory := NewRealUDPSocketFactory()
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	socket, err := factory.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}
	defer socket.Close()

	// Set read buffer size
	err = socket.SetReadBuffer(65536)
	if err != nil {
		// Some systems may not allow setting buffer size
		t.Logf("SetReadBuffer returned: %v (may be expected on some systems)", err)
	}
}

func TestRealUDPSocket_SetReadDeadline(t *testing.T) {
	factory := NewRealUDPSocketFactory()
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	socket, err := factory.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}
	defer socket.Close()

	// Set read deadline
	deadline := time.Now().Add(100 * time.Millisecond)
	err = socket.SetReadDeadline(deadline)
	if err != nil {
		t.Errorf("SetReadDeadline failed: %v", err)
	}
}

func TestMockUDPSocket_ReadFromUDP(t *testing.T) {
	packets := []MockUDPPacket{
		{Data: []byte("packet1"), Addr: &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 1234}},
		{Data: []byte("packet2"), Addr: &net.UDPAddr{IP: net.ParseIP("192.168.1.2"), Port: 5678}},
	}
	socket := NewMockUDPSocket(packets)

	buf := make([]byte, 1024)

	// Read first packet
	n, addr, err := socket.ReadFromUDP(buf)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if string(buf[:n]) != "packet1" {
		t.Errorf("Expected 'packet1', got: %s", string(buf[:n]))
	}
	if addr.Port != 1234 {
		t.Errorf("Expected port 1234, got: %d", addr.Port)
	}

	// Read second packet
	n, addr, err = socket.ReadFromUDP(buf)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if string(buf[:n]) != "packet2" {
		t.Errorf("Expected 'packet2', got: %s", string(buf[:n]))
	}
	if addr.Port != 5678 {
		t.Errorf("Expected port 5678, got: %d", addr.Port)
	}

	// Third read should timeout
	_, _, err = socket.ReadFromUDP(buf)
	if err == nil {
		t.Error("Expected timeout error")
	}
	netErr, ok := err.(net.Error)
	if !ok {
		t.Errorf("Expected net.Error, got: %T", err)
	} else if !netErr.Timeout() {
		t.Error("Expected timeout error")
	}
}

func TestMockUDPSocket_ReadError(t *testing.T) {
	socket := NewMockUDPSocket(nil)
	socket.ReadError = errors.New("mock read error")

	buf := make([]byte, 1024)
	_, _, err := socket.ReadFromUDP(buf)
	if err == nil {
		t.Error("Expected error")
	}
	if err.Error() != "mock read error" {
		t.Errorf("Expected 'mock read error', got: %v", err)
	}

	// Error should be cleared after first read
	_, _, err = socket.ReadFromUDP(buf)
	if err == nil || !err.(net.Error).Timeout() {
		t.Error("Expected timeout error after read error is consumed")
	}
}

func TestMockUDPSocket_Closed(t *testing.T) {
	socket := NewMockUDPSocket([]MockUDPPacket{{Data: []byte("test")}})

	// Close the socket
	err := socket.Close()
	if err != nil {
		t.Errorf("Unexpected close error: %v", err)
	}
	if !socket.Closed {
		t.Error("Expected socket to be marked as closed")
	}

	// Read should fail on closed socket
	buf := make([]byte, 1024)
	_, _, err = socket.ReadFromUDP(buf)
	if err != net.ErrClosed {
		t.Errorf("Expected net.ErrClosed, got: %v", err)
	}
}

func TestMockUDPSocket_SetReadBuffer(t *testing.T) {
	socket := NewMockUDPSocket(nil)

	err := socket.SetReadBuffer(65536)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if socket.ReadBufferSize != 65536 {
		t.Errorf("Expected buffer size 65536, got: %d", socket.ReadBufferSize)
	}
}

func TestMockUDPSocket_SetReadBufferError(t *testing.T) {
	socket := NewMockUDPSocket(nil)
	socket.SetReadBufferError = errors.New("buffer error")

	err := socket.SetReadBuffer(65536)
	if err == nil {
		t.Error("Expected error")
	}
	if err.Error() != "buffer error" {
		t.Errorf("Expected 'buffer error', got: %v", err)
	}
}

func TestMockUDPSocket_SetReadDeadline(t *testing.T) {
	socket := NewMockUDPSocket(nil)
	deadline := time.Now().Add(1 * time.Second)

	err := socket.SetReadDeadline(deadline)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !socket.ReadDeadline.Equal(deadline) {
		t.Errorf("Expected deadline %v, got: %v", deadline, socket.ReadDeadline)
	}
}

func TestMockUDPSocket_LocalAddr(t *testing.T) {
	socket := NewMockUDPSocket(nil)
	socket.LocalAddress = &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 9999}

	addr := socket.LocalAddr()
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		t.Fatalf("Expected *net.UDPAddr, got: %T", addr)
	}
	if udpAddr.Port != 9999 {
		t.Errorf("Expected port 9999, got: %d", udpAddr.Port)
	}
}

func TestMockUDPSocket_Reset(t *testing.T) {
	socket := NewMockUDPSocket([]MockUDPPacket{{Data: []byte("test")}})

	// Consume the packet and modify state
	buf := make([]byte, 1024)
	socket.ReadFromUDP(buf)
	socket.SetReadBuffer(1024)
	socket.SetReadDeadline(time.Now())
	socket.ReadError = errors.New("error")

	// Reset
	socket.Reset()

	// Verify reset state
	if socket.ReadIndex != 0 {
		t.Errorf("Expected ReadIndex 0, got: %d", socket.ReadIndex)
	}
	if socket.Closed {
		t.Error("Expected Closed to be false")
	}
	if socket.ReadBufferSize != 0 {
		t.Errorf("Expected ReadBufferSize 0, got: %d", socket.ReadBufferSize)
	}
	if !socket.ReadDeadline.IsZero() {
		t.Errorf("Expected zero ReadDeadline, got: %v", socket.ReadDeadline)
	}
	if socket.ReadError != nil {
		t.Errorf("Expected nil ReadError, got: %v", socket.ReadError)
	}

	// Should be able to read the packet again
	n, _, err := socket.ReadFromUDP(buf)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if string(buf[:n]) != "test" {
		t.Errorf("Expected 'test', got: %s", string(buf[:n]))
	}
}

func TestMockUDPSocketFactory_ListenUDP(t *testing.T) {
	mockSocket := NewMockUDPSocket(nil)
	factory := NewMockUDPSocketFactory(mockSocket)

	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2368}
	socket, err := factory.ListenUDP("udp", addr)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if socket != mockSocket {
		t.Error("Expected mock socket to be returned")
	}

	// Verify call was recorded
	if len(factory.ListenCalls) != 1 {
		t.Fatalf("Expected 1 call, got: %d", len(factory.ListenCalls))
	}
	if factory.ListenCalls[0].Network != "udp" {
		t.Errorf("Expected network 'udp', got: %s", factory.ListenCalls[0].Network)
	}
	if factory.ListenCalls[0].Addr.Port != 2368 {
		t.Errorf("Expected port 2368, got: %d", factory.ListenCalls[0].Addr.Port)
	}
}

func TestMockUDPSocketFactory_ListenUDP_Error(t *testing.T) {
	factory := NewMockUDPSocketFactory(nil)
	factory.Error = errors.New("mock listen error")

	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2368}
	socket, err := factory.ListenUDP("udp", addr)
	if err == nil {
		t.Error("Expected error")
	}
	if socket != nil {
		t.Error("Expected nil socket on error")
	}
	if err.Error() != "mock listen error" {
		t.Errorf("Expected 'mock listen error', got: %v", err)
	}
}

func TestTimeoutError(t *testing.T) {
	err := &timeoutError{}

	if err.Error() != "i/o timeout" {
		t.Errorf("Expected 'i/o timeout', got: %s", err.Error())
	}
	if !err.Timeout() {
		t.Error("Expected Timeout() to return true")
	}
	if !err.Temporary() {
		t.Error("Expected Temporary() to return true")
	}
}
