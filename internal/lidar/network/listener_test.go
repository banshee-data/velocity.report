package network

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// MockFullPacketStats implements PacketStatsInterface for testing
type MockFullPacketStats struct {
	packetCount int
	droppedCnt  int
	pointCount  int
	logCalls    int
}

func (m *MockFullPacketStats) AddPacket(bytes int) {
	m.packetCount++
}

func (m *MockFullPacketStats) AddDropped() {
	m.droppedCnt++
}

func (m *MockFullPacketStats) AddPoints(count int) {
	m.pointCount += count
}

func (m *MockFullPacketStats) LogStats(parsePackets bool) {
	m.logCalls++
}

// MockParser implements Parser interface for testing
type MockParser struct {
	points      []lidar.PointPolar
	motorSpeed  uint16
	parseErr    error
	parseCalled int
}

func (m *MockParser) ParsePacket(packet []byte) ([]lidar.PointPolar, error) {
	m.parseCalled++
	if m.parseErr != nil {
		return nil, m.parseErr
	}
	return m.points, nil
}

func (m *MockParser) GetLastMotorSpeed() uint16 {
	return m.motorSpeed
}

// MockFrameBuilder implements FrameBuilder interface for testing
type MockFrameBuilder struct {
	points      []lidar.PointPolar
	motorSpeed  uint16
	addCalled   int
	speedCalled int
}

func (m *MockFrameBuilder) AddPointsPolar(points []lidar.PointPolar) {
	m.addCalled++
	m.points = append(m.points, points...)
}

func (m *MockFrameBuilder) SetMotorSpeed(rpm uint16) {
	m.speedCalled++
	m.motorSpeed = rpm
}

func TestNewUDPListener_Defaults(t *testing.T) {
	config := UDPListenerConfig{
		Address: ":2368",
		RcvBuf:  1024 * 1024,
	}

	listener := NewUDPListener(config)

	if listener == nil {
		t.Fatal("NewUDPListener returned nil")
	}
	if listener.address != ":2368" {
		t.Errorf("Expected address ':2368', got '%s'", listener.address)
	}
	if listener.rcvBuf != 1024*1024 {
		t.Errorf("Expected rcvBuf %d, got %d", 1024*1024, listener.rcvBuf)
	}
	// Check default log interval is set
	if listener.logInterval != time.Minute {
		t.Errorf("Expected default log interval 1 minute, got %v", listener.logInterval)
	}
	// stats should be noopStats by default
	if listener.stats == nil {
		t.Error("Expected default noop stats, got nil")
	}
}

func TestNewUDPListener_WithStats(t *testing.T) {
	stats := &MockFullPacketStats{}
	config := UDPListenerConfig{
		Address:     ":2368",
		RcvBuf:      1024 * 1024,
		Stats:       stats,
		LogInterval: 30 * time.Second,
	}

	listener := NewUDPListener(config)

	if listener.stats != stats {
		t.Error("Expected custom stats to be used")
	}
	if listener.logInterval != 30*time.Second {
		t.Errorf("Expected log interval 30s, got %v", listener.logInterval)
	}
}

func TestNewUDPListener_WithParser(t *testing.T) {
	parser := &MockParser{
		points:     []lidar.PointPolar{{Distance: 10.0}},
		motorSpeed: 600,
	}

	config := UDPListenerConfig{
		Address: ":2368",
		RcvBuf:  1024 * 1024,
		Parser:  parser,
	}

	listener := NewUDPListener(config)

	if listener.parser != parser {
		t.Error("Expected custom parser to be set")
	}
}

func TestNewUDPListener_WithFrameBuilder(t *testing.T) {
	fb := &MockFrameBuilder{}

	config := UDPListenerConfig{
		Address:      ":2368",
		RcvBuf:       1024 * 1024,
		FrameBuilder: fb,
	}

	listener := NewUDPListener(config)

	if listener.frameBuilder != fb {
		t.Error("Expected custom frame builder to be set")
	}
}

func TestNewUDPListener_WithForwarder(t *testing.T) {
	// Create a forwarder that will work
	stats := &MockPacketStats{}
	forwarder, err := NewPacketForwarder("localhost", 12345, stats, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}
	defer forwarder.conn.Close()

	config := UDPListenerConfig{
		Address:   ":2368",
		RcvBuf:    1024 * 1024,
		Forwarder: forwarder,
	}

	listener := NewUDPListener(config)

	if listener.forwarder != forwarder {
		t.Error("Expected forwarder to be set")
	}
}

func TestNewUDPListener_DisableParsing(t *testing.T) {
	config := UDPListenerConfig{
		Address:        ":2368",
		RcvBuf:         1024 * 1024,
		DisableParsing: true,
	}

	listener := NewUDPListener(config)

	if !listener.disableParsing {
		t.Error("Expected disableParsing to be true")
	}
}

func TestUDPListener_Close_Nil(t *testing.T) {
	listener := &UDPListener{}

	// Close with nil connection should not error
	err := listener.Close()
	if err != nil {
		t.Errorf("Close with nil conn returned error: %v", err)
	}
}

func TestNoopStats(t *testing.T) {
	stats := &noopStats{}

	// These should all be no-ops and not panic
	stats.AddPacket(100)
	stats.AddDropped()
	stats.AddPoints(50)
	stats.LogStats(true)
	stats.LogStats(false)
}

// TestDefaultSensorConfig tests the default sensor configuration
func TestDefaultSensorConfig(t *testing.T) {
	config := DefaultSensorConfig()

	if config == nil {
		t.Fatal("DefaultSensorConfig returned nil")
	}
	if config.MotorSpeedRPM != 600.0 {
		t.Errorf("Expected MotorSpeedRPM 600, got %f", config.MotorSpeedRPM)
	}
	if config.Channels != 40 {
		t.Errorf("Expected Channels 40, got %d", config.Channels)
	}
}

// TestForegroundForwarder_InvalidAddress tests error handling for invalid addresses
func TestForegroundForwarder_InvalidAddress(t *testing.T) {
	_, err := NewForegroundForwarder("invalid-host-12345", 2370, nil)
	if err == nil {
		t.Error("Expected error for invalid address, got nil")
	}
}

// TestForegroundForwarder_ForwardForeground_Empty tests forwarding empty points
func TestForegroundForwarder_ForwardForeground_Empty(t *testing.T) {
	// Create a valid forwarder
	ff, err := NewForegroundForwarder("127.0.0.1", 12346, nil)
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}
	defer ff.Close()

	// Should not panic or block
	ff.ForwardForeground(nil)
	ff.ForwardForeground([]lidar.PointPolar{})
}

// TestForegroundForwarder_NewWithNilConfig tests creation with nil config
func TestForegroundForwarder_NewWithNilConfig(t *testing.T) {
	ff, err := NewForegroundForwarder("127.0.0.1", 12347, nil)
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}
	defer ff.Close()

	// Should use default config
	if ff.sensorConfig == nil {
		t.Error("Expected default sensor config to be set")
	}
	if ff.sensorConfig.MotorSpeedRPM != 600.0 {
		t.Errorf("Expected default motor speed 600, got %f", ff.sensorConfig.MotorSpeedRPM)
	}
}

// TestForegroundForwarder_NewWithCustomConfig tests creation with custom config
func TestForegroundForwarder_NewWithCustomConfig(t *testing.T) {
	config := &SensorConfig{
		MotorSpeedRPM: 1200.0,
		Channels:      64,
	}

	ff, err := NewForegroundForwarder("127.0.0.1", 12348, config)
	if err != nil {
		t.Fatalf("Failed to create forwarder: %v", err)
	}
	defer ff.Close()

	if ff.sensorConfig.MotorSpeedRPM != 1200.0 {
		t.Errorf("Expected motor speed 1200, got %f", ff.sensorConfig.MotorSpeedRPM)
	}
	if ff.sensorConfig.Channels != 64 {
		t.Errorf("Expected channels 64, got %d", ff.sensorConfig.Channels)
	}
}
