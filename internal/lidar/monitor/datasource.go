package monitor

import (
	"context"
	"errors"
	"sync"
)

// DataSource represents the type of data source currently active.
type DataSource string

// Defined data sources for the monitor.
const (
	DataSourceLive         DataSource = "live"
	DataSourcePCAP         DataSource = "pcap"
	DataSourcePCAPAnalysis DataSource = "pcap_analysis"
)

// ReplayConfig contains configuration for PCAP replay.
type ReplayConfig struct {
	StartSeconds    float64 // Start offset in seconds
	DurationSeconds float64 // Duration to replay (-1 for entire file)
	SpeedMode       string  // "realtime", "fast", or custom speed
	SpeedRatio      float64 // Speed multiplier (e.g., 2.0 = 2x speed)
	AnalysisMode    bool    // When true, preserve grid after completion
}

// DataSourceManager defines an interface for managing data sources.
// This abstraction enables unit testing of WebServer without real UDP/PCAP dependencies.
type DataSourceManager interface {
	// StartLiveListener starts the live UDP listener.
	StartLiveListener(ctx context.Context) error

	// StopLiveListener stops the live UDP listener.
	StopLiveListener() error

	// StartPCAPReplay starts replaying packets from a PCAP file.
	StartPCAPReplay(ctx context.Context, file string, config ReplayConfig) error

	// StopPCAPReplay stops the current PCAP replay.
	StopPCAPReplay() error

	// CurrentSource returns the currently active data source.
	CurrentSource() DataSource

	// CurrentPCAPFile returns the currently replaying PCAP file, if any.
	CurrentPCAPFile() string

	// IsPCAPInProgress returns true if a PCAP replay is currently active.
	IsPCAPInProgress() bool
}

// MockDataSourceManager implements DataSourceManager for testing.
type MockDataSourceManager struct {
	mu sync.RWMutex

	// Current state
	source      DataSource
	pcapFile    string
	pcapConfig  ReplayConfig
	liveStarted bool
	pcapStarted bool

	// Error injection
	StartLiveError error
	StopLiveError  error
	StartPCAPError error
	StopPCAPError  error

	// Call tracking
	StartLiveCalls int
	StopLiveCalls  int
	StartPCAPCalls int
	StopPCAPCalls  int

	// Context tracking
	LastLiveCtx context.Context
	LastPCAPCtx context.Context
}

// NewMockDataSourceManager creates a new MockDataSourceManager.
func NewMockDataSourceManager() *MockDataSourceManager {
	return &MockDataSourceManager{
		source: DataSourceLive,
	}
}

// StartLiveListener starts the mock live listener.
func (m *MockDataSourceManager) StartLiveListener(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StartLiveCalls++
	m.LastLiveCtx = ctx

	if m.StartLiveError != nil {
		return m.StartLiveError
	}

	m.liveStarted = true
	m.source = DataSourceLive
	return nil
}

// StopLiveListener stops the mock live listener.
func (m *MockDataSourceManager) StopLiveListener() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StopLiveCalls++

	if m.StopLiveError != nil {
		return m.StopLiveError
	}

	m.liveStarted = false
	return nil
}

// StartPCAPReplay starts mock PCAP replay.
func (m *MockDataSourceManager) StartPCAPReplay(ctx context.Context, file string, config ReplayConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StartPCAPCalls++
	m.LastPCAPCtx = ctx

	if m.StartPCAPError != nil {
		return m.StartPCAPError
	}

	m.pcapFile = file
	m.pcapConfig = config
	m.pcapStarted = true
	if config.AnalysisMode {
		m.source = DataSourcePCAPAnalysis
	} else {
		m.source = DataSourcePCAP
	}
	return nil
}

// StopPCAPReplay stops mock PCAP replay.
func (m *MockDataSourceManager) StopPCAPReplay() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StopPCAPCalls++

	if m.StopPCAPError != nil {
		return m.StopPCAPError
	}

	m.pcapStarted = false
	m.pcapFile = ""
	m.source = DataSourceLive
	return nil
}

// CurrentSource returns the current data source.
func (m *MockDataSourceManager) CurrentSource() DataSource {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.source
}

// CurrentPCAPFile returns the current PCAP file.
func (m *MockDataSourceManager) CurrentPCAPFile() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.pcapFile
}

// IsPCAPInProgress returns true if PCAP replay is active.
func (m *MockDataSourceManager) IsPCAPInProgress() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.pcapStarted
}

// IsLiveStarted returns true if live listener is running.
func (m *MockDataSourceManager) IsLiveStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.liveStarted
}

// GetLastPCAPConfig returns the last PCAP config used.
func (m *MockDataSourceManager) GetLastPCAPConfig() ReplayConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.pcapConfig
}

// Reset resets the mock state.
func (m *MockDataSourceManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.source = DataSourceLive
	m.pcapFile = ""
	m.pcapConfig = ReplayConfig{}
	m.liveStarted = false
	m.pcapStarted = false
	m.StartLiveError = nil
	m.StopLiveError = nil
	m.StartPCAPError = nil
	m.StopPCAPError = nil
	m.StartLiveCalls = 0
	m.StopLiveCalls = 0
	m.StartPCAPCalls = 0
	m.StopPCAPCalls = 0
	m.LastLiveCtx = nil
	m.LastPCAPCtx = nil
}

// SetSource sets the current source (for testing state setup).
func (m *MockDataSourceManager) SetSource(source DataSource) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.source = source
}

// Ensure MockDataSourceManager implements DataSourceManager.
var _ DataSourceManager = (*MockDataSourceManager)(nil)

// ErrSourceAlreadyActive is returned when trying to start a source that's already active.
var ErrSourceAlreadyActive = errors.New("data source already active")

// ErrNoSourceActive is returned when trying to stop a source that's not active.
var ErrNoSourceActive = errors.New("no data source active")
