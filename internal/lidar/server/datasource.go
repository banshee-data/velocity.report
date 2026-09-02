package server

import (
	"context"
	"encoding/json"
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
	StartSeconds          float64         // Start offset in seconds
	DurationSeconds       float64         // Duration to replay (-1 for entire file)
	SpeedMode             string          // "realtime", "analysis", or "scaled"
	SpeedRatio            float64         // Speed multiplier (e.g., 2.0 = 2x speed)
	AnalysisMode          bool            // When true, preserve grid after completion
	DisableRecording      bool            // When true, skip VRLOG recording during PCAP replay
	SettleBeforeRecording bool            // When true, warm and reload the grid before recording a second pass
	SensorID              string          // Optional sensor override for analysis-run provenance
	PreferredRunID        string          // Optional caller-supplied run identifier
	ParentRunID           string          // Optional lineage for reprocess runs
	ReplayCaseID          string          // Optional source replay case for scene replays
	RequestedParamSetID   string          // Optional existing launch-intent lineage
	RequestedParamsJSON   json.RawMessage // Optional launch-intent parameters for provenance

	// Debug parameters
	DebugRingMin int
	DebugRingMax int
	DebugAzMin   float32
	DebugAzMax   float32
	EnableDebug  bool
	EnablePlots  bool
}

// DataSourceManager defines an interface for managing data sources.
// This abstraction enables unit testing of Server without real UDP/PCAP dependencies.
type DataSourceManager interface {
	// StartLiveListener starts the live UDP listener.
	StartLiveListener(ctx context.Context) error

	// StopLiveListener stops the live UDP listener.
	StopLiveListener() error

	// StartPCAPReplay starts replaying packets from a PCAP file.
	StartPCAPReplay(ctx context.Context, file string, config ReplayConfig) error

	// StopPCAPReplay stops the current PCAP replay.
	StopPCAPReplay() error
}

// MockDataSourceManager implements DataSourceManager for testing.
type MockDataSourceManager struct {
	mu sync.RWMutex

	// Observation state for assertions. There is deliberately no `source`
	// copy: what is driving the pipeline is answered by Server.PipelineState.
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
	return &MockDataSourceManager{}
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
	return nil
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

// Ensure MockDataSourceManager implements DataSourceManager.
var _ DataSourceManager = (*MockDataSourceManager)(nil)

// ErrSourceAlreadyActive is returned when trying to start a source that's already active.
var ErrSourceAlreadyActive = errors.New("data source already active")

// ErrNoSourceActive is returned when trying to stop a source that's not active.
var ErrNoSourceActive = errors.New("no data source active")

// ServerDataSourceOperations defines the interface for Server operations
// that the RealDataSourceManager needs to delegate to.
// This allows the DataSourceManager to call back into Server without circular dependencies.
type ServerDataSourceOperations interface {
	// StartLiveListenerInternal starts the UDP listener.
	StartLiveListenerInternal(ctx context.Context) error
	// StopLiveListenerInternal stops the UDP listener.
	StopLiveListenerInternal()
	// StartPCAPInternal starts PCAP replay with the given configuration.
	StartPCAPInternal(pcapFile string, config ReplayConfig) error
	// StopPCAPInternal stops the current PCAP replay.
	StopPCAPInternal()
	// BaseContext returns the base context for operations.
	BaseContext() context.Context
}

// RealDataSourceManager implements DataSourceManager using Server operations.
type RealDataSourceManager struct {
	mu sync.RWMutex

	// Server operations delegate
	ops ServerDataSourceOperations

	// Lifecycle guards. This manager tracks only whether it has already
	// started each source so its own calls stay idempotent; the authoritative
	// answer to "what is driving the pipeline" lives in Server.PipelineState.
	liveStarted bool
	pcapStarted bool
}

// NewRealDataSourceManager creates a new RealDataSourceManager.
func NewRealDataSourceManager(ops ServerDataSourceOperations) *RealDataSourceManager {
	return &RealDataSourceManager{ops: ops}
}

// StartLiveListener starts the live UDP listener.
func (r *RealDataSourceManager) StartLiveListener(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.liveStarted {
		return nil // Already started
	}

	if err := r.ops.StartLiveListenerInternal(ctx); err != nil {
		return err
	}

	r.liveStarted = true
	return nil
}

// StopLiveListener stops the live UDP listener.
func (r *RealDataSourceManager) StopLiveListener() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.liveStarted {
		return nil // Already stopped
	}

	r.ops.StopLiveListenerInternal()
	r.liveStarted = false
	return nil
}

// StartPCAPReplay starts replaying packets from a PCAP file.
func (r *RealDataSourceManager) StartPCAPReplay(ctx context.Context, file string, config ReplayConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.pcapStarted {
		return ErrSourceAlreadyActive
	}

	if err := r.ops.StartPCAPInternal(file, config); err != nil {
		return err
	}

	r.pcapStarted = true
	return nil
}

// StopPCAPReplay stops the current PCAP replay.
func (r *RealDataSourceManager) StopPCAPReplay() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.pcapStarted {
		return nil // Already stopped
	}

	r.ops.StopPCAPInternal()
	r.pcapStarted = false
	return nil
}

// Ensure RealDataSourceManager implements DataSourceManager.
var _ DataSourceManager = (*RealDataSourceManager)(nil)
