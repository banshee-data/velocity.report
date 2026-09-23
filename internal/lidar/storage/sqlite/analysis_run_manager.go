package sqlite

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/configasset"
	"github.com/google/uuid"
)

// AnalysisRunManager coordinates analysis run lifecycle and track collection.
// It is safe for concurrent use and provides hooks for the tracking pipeline.
type AnalysisRunManager struct {
	mu         sync.RWMutex
	store      *AnalysisRunStore
	currentRun *AnalysisRun
	sensorID   string
	startTime  time.Time

	// Stats collected during the run
	totalFrames   int
	totalClusters int
	tracksSeen    map[string]bool // Track IDs seen during this run

	// pendingTracks holds, for each track offered again since its row was
	// last written, the newest measurements offered. The pipeline offers every
	// confirmed track on every frame; the row is inserted from the first
	// offer, when the track is a few observations old, so without this it
	// would describe that moment for ever. Values, not pointers: a
	// TrackMeasurement holds no references, so a copy cannot be changed
	// underneath by the tracker. Emptied by each flush, which bounds it by the
	// tracks alive in one flush interval rather than by the length of the run.
	pendingTracks map[string]TrackMeasurement
	lastFlush     time.Time
	// now is the clock the flush interval is measured on. A field so tests
	// can move time rather than wait for it.
	now func() time.Time

	// Frame timestamps for data duration (vs wall-clock processing time)
	firstFrameNs int64 // timestamp of first frame (nanoseconds)
	lastFrameNs  int64 // timestamp of last frame (nanoseconds)
}

// analysisRunManagers stores per-sensor analysis run managers.
var (
	armMu       sync.RWMutex
	armRegistry = make(map[string]*AnalysisRunManager)
)

// runTrackFlushInterval is how often pending track measurements are written
// during a run, on the wall clock.
//
// Writing at the end of the run would be enough for a replay that finishes.
// It is not enough for a live run, which can last days and end with the power:
// every row would be left describing a first sighting, which is the defect
// this exists to remove. Ten seconds keeps the write to one small transaction
// every hundred or so frames, and means a reader mid-run (the track labelling
// views) sees measurements at most that stale. Wall clock rather than data
// time because the cost being bounded is database writes, and a replay runs
// through data time many times faster than real time.
const runTrackFlushInterval = 10 * time.Second

// AnalysisRunStartOptions captures immutable run-config provenance for an
// analysis replay.
type AnalysisRunStartOptions struct {
	PreferredRunID      string
	SourceType          string
	SourcePath          string
	SensorID            string
	ParentRunID         string
	ReplayCaseID        string
	RequestedParamSetID string
	RequestedParamsJSON json.RawMessage
	EffectiveConfig     *cfgpkg.TuningConfig
}

// NewAnalysisRunManager creates a new manager for tracking analysis runs.
func NewAnalysisRunManager(db DBClient, sensorID string) *AnalysisRunManager {
	return &AnalysisRunManager{
		store:         NewAnalysisRunStore(db),
		sensorID:      sensorID,
		tracksSeen:    make(map[string]bool),
		pendingTracks: make(map[string]TrackMeasurement),
		now:           time.Now,
	}
}

// NewAnalysisRunManagerDI creates a new manager without registering it in the
// global registry. Prefer this constructor when wiring dependencies
// explicitly via pipeline.SensorRuntime.
func NewAnalysisRunManagerDI(db DBClient, sensorID string) *AnalysisRunManager {
	return &AnalysisRunManager{
		store:         NewAnalysisRunStore(db),
		sensorID:      sensorID,
		tracksSeen:    make(map[string]bool),
		pendingTracks: make(map[string]TrackMeasurement),
		now:           time.Now,
	}
}

// RegisterAnalysisRunManager registers a manager for a sensor ID.
func RegisterAnalysisRunManager(sensorID string, manager *AnalysisRunManager) {
	armMu.Lock()
	defer armMu.Unlock()
	armRegistry[sensorID] = manager
}

// GetAnalysisRunManager retrieves the manager for a sensor ID.
func GetAnalysisRunManager(sensorID string) *AnalysisRunManager {
	armMu.RLock()
	defer armMu.RUnlock()
	return armRegistry[sensorID]
}

// StartRun begins a new analysis run for PCAP processing.
// It returns the run ID that can be used for track association.
func (m *AnalysisRunManager) StartRun(sourcePath string, _ RunParams) (string, error) {
	run := &AnalysisRun{
		SourceType: "pcap",
		SourcePath: sourcePath,
		SensorID:   m.sensorID,
		Status:     "running",
	}

	return m.startPreparedRun(run)
}

// StartRunWithConfig begins a new analysis run backed by immutable run-config
// provenance. It records the exact effective config and optional launch intent
// before execution starts.
func (m *AnalysisRunManager) StartRunWithConfig(opts AnalysisRunStartOptions) (string, error) {
	if opts.EffectiveConfig == nil {
		return "", fmt.Errorf("effective config is required")
	}

	sensorID := strings.TrimSpace(opts.SensorID)
	if sensorID == "" {
		sensorID = m.sensorID
	}
	sourceType := strings.TrimSpace(opts.SourceType)
	if sourceType == "" {
		sourceType = "pcap"
	}

	run := &AnalysisRun{
		RunID:               strings.TrimSpace(opts.PreferredRunID),
		SourceType:          sourceType,
		SourcePath:          opts.SourcePath,
		SensorID:            sensorID,
		ParentRunID:         strings.TrimSpace(opts.ParentRunID),
		ReplayCaseID:        strings.TrimSpace(opts.ReplayCaseID),
		RequestedParamSetID: strings.TrimSpace(opts.RequestedParamSetID),
		Status:              "running",
	}

	configStore := configasset.NewStore(m.store.db)
	buildIdentity := configasset.ReadBuildIdentity()
	effectiveParamSet, err := configasset.MakeEffectiveParamSet(opts.EffectiveConfig)
	if err != nil {
		return "", err
	}

	runConfig, err := configStore.EnsureRunConfig(effectiveParamSet, buildIdentity)
	if err != nil {
		return "", err
	}
	run.RunConfigID = runConfig.RunConfigID

	if len(opts.RequestedParamsJSON) > 0 {
		requestedParamSet, err := configasset.MakeRequestedParamSet(opts.RequestedParamsJSON)
		if err != nil {
			return "", err
		}
		storedRequestedParamSet, err := configStore.EnsureParamSet(requestedParamSet)
		if err != nil {
			return "", err
		}
		run.RequestedParamSetID = storedRequestedParamSet.ParamSetID
	}

	return m.startPreparedRun(run)
}

func (m *AnalysisRunManager) startPreparedRun(run *AnalysisRun) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if strings.TrimSpace(run.RunID) == "" {
		run.RunID = uuid.NewString()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now()
	}
	if strings.TrimSpace(run.SourceType) == "" {
		run.SourceType = "pcap"
	}
	if strings.TrimSpace(run.SensorID) == "" {
		run.SensorID = m.sensorID
	}
	if strings.TrimSpace(run.Status) == "" {
		run.Status = "running"
	}

	m.currentRun = run
	if err := m.store.InsertRun(m.currentRun); err != nil {
		m.currentRun = nil
		return "", err
	}

	m.startTime = time.Now()
	m.totalFrames = 0
	m.totalClusters = 0
	m.tracksSeen = make(map[string]bool)
	m.pendingTracks = make(map[string]TrackMeasurement)
	m.lastFlush = m.now()
	m.firstFrameNs = 0
	m.lastFrameNs = 0

	diagf("[AnalysisRunManager] Started run %s for %s", run.RunID, run.SourcePath)
	return run.RunID, nil
}

// RecordFrame increments the frame count and tracks the frame timestamp.
// The timestampNs is the data timestamp (e.g. PCAP packet time), not wall-clock.
func (m *AnalysisRunManager) RecordFrame(timestampNs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalFrames++
	if timestampNs > 0 {
		if m.firstFrameNs == 0 {
			m.firstFrameNs = timestampNs
		}
		m.lastFrameNs = timestampNs
	}
	// Checked here as well as in RecordTrack: when the last tracks in view
	// die, nothing offers a track again, and their final measurements would
	// otherwise wait for the end of the run.
	m.flushPendingTracksIfDueLocked()
}

// RecordClusters increments the cluster count for the current run.
func (m *AnalysisRunManager) RecordClusters(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalClusters += count
}

// RecordTrack records a track for the current analysis run. It is called for
// every confirmed track on every frame. The first call for a track inserts its
// RunTrack row and returns true; later calls return false and keep the track's
// newest measurements for the next flush, so the row ends up describing the
// finished track rather than its first sighting.
func (m *AnalysisRunManager) RecordTrack(track *TrackedObject) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentRun == nil {
		return false
	}

	// Already recorded: remember how it looks now. Its row was written from
	// the first sighting and is brought up to date by the next flush.
	if m.tracksSeen[track.TrackID] {
		m.pendingTracks[track.TrackID] = track.TrackMeasurement
		m.flushPendingTracksIfDueLocked()
		return false
	}
	m.tracksSeen[track.TrackID] = true

	// Compute quality metrics before export
	track.ComputeQualityMetrics()

	// Create RunTrack from TrackedObject
	runTrack := RunTrackFromTrackedObject(m.currentRun.RunID, track)

	// Insert into database
	if err := m.store.InsertRunTrack(runTrack); err != nil {
		opsf("[AnalysisRunManager] Failed to insert run track %s: %v", track.TrackID, err)
		return false
	}

	return true
}

// CompleteRun finalizes the current analysis run with statistics.
func (m *AnalysisRunManager) CompleteRun() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentRun == nil {
		return nil
	}

	processingTime := time.Since(m.startTime)

	// Use data timestamps for duration (PCAP time, not wall-clock).
	// Falls back to wall-clock only if no frame timestamps were recorded.
	var durationSecs float64
	if m.firstFrameNs > 0 && m.lastFrameNs > m.firstFrameNs {
		durationSecs = float64(m.lastFrameNs-m.firstFrameNs) / 1e9
	} else {
		durationSecs = processingTime.Seconds()
	}

	confirmedCount := 0
	for trackID := range m.tracksSeen {
		// We could query track states here, but for now just count all
		_ = trackID
		confirmedCount++
	}

	stats := &AnalysisStats{
		DurationSecs:     durationSecs,
		TotalFrames:      m.totalFrames,
		TotalClusters:    m.totalClusters,
		TotalTracks:      len(m.tracksSeen),
		ConfirmedTracks:  confirmedCount,
		ProcessingTimeMs: processingTime.Milliseconds(),
		CompletedAt:      time.Now(),
		FrameStartNs:     m.firstFrameNs,
		FrameEndNs:       m.lastFrameNs,
	}

	// Before the run is marked complete, so that a completed run never has
	// rows still describing first sightings. A failure here does not stop the
	// run completing, which would strand it as "running"; it is reported once
	// the run's own state is settled.
	flushErr := m.flushPendingTracksLocked()

	if err := m.store.CompleteRun(m.currentRun.RunID, stats); err != nil {
		return err
	}

	diagf("[AnalysisRunManager] Completed run %s: %d frames, %d clusters, %d tracks in %.2fs",
		m.currentRun.RunID, stats.TotalFrames, stats.TotalClusters, stats.TotalTracks, durationSecs)

	runID := m.currentRun.RunID
	m.currentRun = nil
	if flushErr != nil {
		return fmt.Errorf("run %s completed, but its tracks' final measurements were not written: %w", runID, flushErr)
	}
	return nil
}

// FailRun marks the current run as failed with an error message.
func (m *AnalysisRunManager) FailRun(errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentRun == nil {
		return nil
	}

	// A run that fails late has still measured its tracks. Best effort: the
	// failure being reported is the one that matters.
	if err := m.flushPendingTracksLocked(); err != nil {
		opsf("[AnalysisRunManager] Failed run %s: final track measurements not written: %v", m.currentRun.RunID, err)
	}

	runID := m.currentRun.RunID
	m.currentRun = nil

	if err := m.store.UpdateRunStatus(runID, "failed", errMsg); err != nil {
		return err
	}

	opsf("[AnalysisRunManager] Failed run %s: %s", runID, errMsg)
	return nil
}

// flushPendingTracksIfDueLocked writes pending measurements once the flush
// interval has passed. The caller holds m.mu.
func (m *AnalysisRunManager) flushPendingTracksIfDueLocked() {
	if m.currentRun == nil || m.now().Sub(m.lastFlush) < runTrackFlushInterval {
		return
	}
	if err := m.flushPendingTracksLocked(); err != nil {
		opsf("[AnalysisRunManager] Run %s: track measurements not written, will retry: %v", m.currentRun.RunID, err)
	}
}

// flushPendingTracksLocked writes every pending measurement to its row. The
// caller holds m.mu. On failure the measurements stay pending, so the next
// flush retries them with whatever has been offered since; lastFlush advances
// either way, so a database that keeps failing is retried once per interval
// rather than on every frame.
func (m *AnalysisRunManager) flushPendingTracksLocked() error {
	m.lastFlush = m.now()
	if m.currentRun == nil || len(m.pendingTracks) == 0 {
		return nil
	}
	if err := m.store.UpdateRunTrackMeasurements(m.currentRun.RunID, m.pendingTracks); err != nil {
		return err
	}
	m.pendingTracks = make(map[string]TrackMeasurement)
	return nil
}

// IsRunActive returns true if there's an active analysis run.
func (m *AnalysisRunManager) IsRunActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentRun != nil
}

// CurrentRunID returns the current run ID, or empty string if no run is active.
func (m *AnalysisRunManager) CurrentRunID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.currentRun == nil {
		return ""
	}
	return m.currentRun.RunID
}
