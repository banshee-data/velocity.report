package network

import (
	"errors"
	"sync"
	"time"
)

// PCAPPacket represents a single packet read from a PCAP file.
type PCAPPacket struct {
	Data      []byte
	Timestamp time.Time
}

// PCAPReader defines an interface for reading packets from PCAP files.
// This abstraction enables unit testing without real PCAP files.
type PCAPReader interface {
	// Open opens a PCAP file for reading.
	Open(filename string) error

	// SetBPFFilter sets a BPF filter on the PCAP reader.
	SetBPFFilter(filter string) error

	// NextPacket returns the next packet from the PCAP file.
	// Returns nil, time.Time{}, io.EOF when no more packets are available.
	NextPacket() (*PCAPPacket, error)

	// Close closes the PCAP reader and releases resources.
	Close()

	// LinkType returns the link type of the PCAP file (for gopacket compatibility).
	// Uses int to accommodate link types > 255 (e.g., Linux Cooked Capture v2 is 276).
	LinkType() int
}

// PCAPReaderFactory defines an interface for creating PCAP readers.
// This abstraction enables dependency injection of PCAP reader creation.
type PCAPReaderFactory interface {
	// NewReader creates a new PCAPReader instance.
	NewReader() PCAPReader
}

// MockPCAPReader implements PCAPReader for testing.
type MockPCAPReader struct {
	mu sync.Mutex

	// Packets holds the packets to return from NextPacket.
	Packets []PCAPPacket

	// ReadIndex tracks the current position in Packets.
	ReadIndex int

	// OpenError is returned by Open if set.
	OpenError error

	// FilterError is returned by SetBPFFilter if set.
	FilterError error

	// OpenedFile records the filename passed to Open.
	OpenedFile string

	// AppliedFilter records the filter passed to SetBPFFilter.
	AppliedFilter string

	// Closed indicates whether Close was called.
	Closed bool

	// MockLinkType is the link type to return.
	MockLinkType int
}

// NewMockPCAPReader creates a new MockPCAPReader with the given packets.
func NewMockPCAPReader(packets []PCAPPacket) *MockPCAPReader {
	return &MockPCAPReader{
		Packets:      packets,
		MockLinkType: 1, // Ethernet
	}
}

// Open records the filename and returns any configured error.
func (m *MockPCAPReader) Open(filename string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.OpenedFile = filename
	if m.OpenError != nil {
		return m.OpenError
	}
	return nil
}

// SetBPFFilter records the filter and returns any configured error.
func (m *MockPCAPReader) SetBPFFilter(filter string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.AppliedFilter = filter
	if m.FilterError != nil {
		return m.FilterError
	}
	return nil
}

// NextPacket returns the next packet from the mock buffer.
func (m *MockPCAPReader) NextPacket() (*PCAPPacket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Closed {
		return nil, errors.New("reader closed")
	}
	if m.ReadIndex >= len(m.Packets) {
		return nil, nil // EOF - no more packets
	}
	pkt := m.Packets[m.ReadIndex]
	m.ReadIndex++
	return &pkt, nil
}

// Close marks the reader as closed.
func (m *MockPCAPReader) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Closed = true
}

// LinkType returns the mock link type.
func (m *MockPCAPReader) LinkType() int {
	return m.MockLinkType
}

// Reset resets the mock reader state for reuse.
func (m *MockPCAPReader) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ReadIndex = 0
	m.Closed = false
	m.OpenedFile = ""
	m.AppliedFilter = ""
	m.OpenError = nil
	m.FilterError = nil
}

// AddPacket adds a packet to the mock reader.
func (m *MockPCAPReader) AddPacket(data []byte, timestamp time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Packets = append(m.Packets, PCAPPacket{
		Data:      data,
		Timestamp: timestamp,
	})
}

// MockPCAPReaderFactory implements PCAPReaderFactory for testing.
type MockPCAPReaderFactory struct {
	mu sync.Mutex

	// Reader is the reader to return from NewReader.
	Reader *MockPCAPReader

	// CreateCalls records the number of NewReader calls.
	CreateCalls int
}

// NewMockPCAPReaderFactory creates a new MockPCAPReaderFactory.
func NewMockPCAPReaderFactory(reader *MockPCAPReader) *MockPCAPReaderFactory {
	return &MockPCAPReaderFactory{Reader: reader}
}

// NewReader returns the configured mock reader.
func (f *MockPCAPReaderFactory) NewReader() PCAPReader {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.CreateCalls++
	return f.Reader
}
