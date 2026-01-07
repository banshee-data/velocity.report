package lidar

import (
	"database/sql"
	"encoding/json"
	"log"
	"sync"
	"time"

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
}

// analysisRunManagers stores per-sensor analysis run managers.
var (
	armMu       sync.RWMutex
	armRegistry = make(map[string]*AnalysisRunManager)
)

// NewAnalysisRunManager creates a new manager for tracking analysis runs.
func NewAnalysisRunManager(db *sql.DB, sensorID string) *AnalysisRunManager {
	return &AnalysisRunManager{
		store:      NewAnalysisRunStore(db),
		sensorID:   sensorID,
		tracksSeen: make(map[string]bool),
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
func (m *AnalysisRunManager) StartRun(sourcePath string, params RunParams) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate unique run ID
	runID := uuid.New().String()

	// Serialize params
	paramsJSON, err := params.ToJSON()
	if err != nil {
		return "", err
	}

	m.currentRun = &AnalysisRun{
		RunID:      runID,
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SourcePath: sourcePath,
		SensorID:   m.sensorID,
		ParamsJSON: paramsJSON,
		Status:     "running",
	}

	if err := m.store.InsertRun(m.currentRun); err != nil {
		m.currentRun = nil
		return "", err
	}

	m.startTime = time.Now()
	m.totalFrames = 0
	m.totalClusters = 0
	m.tracksSeen = make(map[string]bool)

	log.Printf("[AnalysisRunManager] Started run %s for %s", runID, sourcePath)
	return runID, nil
}

// RecordFrame increments the frame count for the current run.
func (m *AnalysisRunManager) RecordFrame() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalFrames++
}

// RecordClusters increments the cluster count for the current run.
func (m *AnalysisRunManager) RecordClusters(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalClusters += count
}

// RecordTrack records a track for the current analysis run.
// This inserts a RunTrack record and returns true if this is a new track.
func (m *AnalysisRunManager) RecordTrack(track *TrackedObject) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentRun == nil {
		return false
	}

	// Check if we've already recorded this track
	if m.tracksSeen[track.TrackID] {
		return false
	}
	m.tracksSeen[track.TrackID] = true

	// Compute quality metrics before export
	track.ComputeQualityMetrics()

	// Create RunTrack from TrackedObject
	runTrack := RunTrackFromTrackedObject(m.currentRun.RunID, track)

	// Insert into database
	if err := m.store.InsertRunTrack(runTrack); err != nil {
		log.Printf("[AnalysisRunManager] Failed to insert run track %s: %v", track.TrackID, err)
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
	durationSecs := processingTime.Seconds()

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
	}

	if err := m.store.CompleteRun(m.currentRun.RunID, stats); err != nil {
		return err
	}

	log.Printf("[AnalysisRunManager] Completed run %s: %d frames, %d clusters, %d tracks in %.2fs",
		m.currentRun.RunID, stats.TotalFrames, stats.TotalClusters, stats.TotalTracks, durationSecs)

	m.currentRun = nil
	return nil
}

// FailRun marks the current run as failed with an error message.
func (m *AnalysisRunManager) FailRun(errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentRun == nil {
		return nil
	}

	if err := m.store.UpdateRunStatus(m.currentRun.RunID, "failed", errMsg); err != nil {
		return err
	}

	log.Printf("[AnalysisRunManager] Failed run %s: %s", m.currentRun.RunID, errMsg)
	m.currentRun = nil
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

// GetCurrentRunParams retrieves the current run's parameters for display.
func (m *AnalysisRunManager) GetCurrentRunParams() (RunParams, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.currentRun == nil {
		return RunParams{}, false
	}

	var params RunParams
	if err := json.Unmarshal(m.currentRun.ParamsJSON, &params); err != nil {
		return RunParams{}, false
	}
	return params, true
}
