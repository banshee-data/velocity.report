package server

import (
	"context"
	"sync/atomic"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func (ws *Server) setBaseContext(ctx context.Context) {
	ws.baseCtxMu.Lock()
	ws.baseCtx = ctx
	ws.baseCtxMu.Unlock()
}

func (ws *Server) baseContext() context.Context {
	ws.baseCtxMu.RLock()
	defer ws.baseCtxMu.RUnlock()
	return ws.baseCtx
}

// CurrentSource returns the active data source type.
func (ws *Server) CurrentSource() DataSource {
	ws.dataSourceMu.RLock()
	defer ws.dataSourceMu.RUnlock()
	return ws.currentSource
}

// CurrentPCAPFile returns the path of the active PCAP file (empty if none).
func (ws *Server) CurrentPCAPFile() string {
	ws.dataSourceMu.RLock()
	defer ws.dataSourceMu.RUnlock()
	return ws.currentPCAPFile
}

// PCAPSpeedRatio returns the configured PCAP playback speed ratio.
func (ws *Server) PCAPSpeedRatio() float64 {
	ws.pcapMu.Lock()
	defer ws.pcapMu.Unlock()
	return ws.pcapSpeedRatio
}

// SetTracker sets the tracker reference for direct config access via /api/lidar/params.
// Also propagates to trackAPI if available.
func (ws *Server) SetTracker(tracker *l5tracks.Tracker) {
	ws.tracker = tracker
	if ws.trackAPI != nil {
		ws.trackAPI.SetTracker(tracker)
	}
}

// SetClassifier sets the classifier reference used by the tracking pipeline.
// This allows live updates of classification thresholds through /api/lidar/params.
func (ws *Server) SetClassifier(classifier *l6objects.TrackClassifier) {
	ws.classifier = classifier
}

// SetSweepRunner sets the sweep runner for web-triggered parameter sweeps.
func (ws *Server) SetSweepRunner(runner SweepRunner) {
	ws.sweepRunner = runner
}

// SetAutoTuneRunner sets the auto-tune runner for web-triggered auto-tuning.
func (ws *Server) SetAutoTuneRunner(runner AutoTuneRunner) {
	ws.autoTuneRunner = runner
}

// SetHINTRunner sets the HINT runner for human-in-the-loop parameter tuning.
func (ws *Server) SetHINTRunner(runner HINTRunner) {
	ws.hintRunner = runner
}

// SetSweepStore sets the sweep store for persisting sweep results.
func (ws *Server) SetSweepStore(store *sqlite.SweepStore) {
	ws.sweepStore = store
}

// BenchmarkMode returns a pointer to the atomic.Bool controlling pipeline
// performance tracing. The caller can pass this to TrackingPipelineConfig
// so benchmark logging is toggled at runtime via the dashboard checkbox.
func (ws *Server) BenchmarkMode() *atomic.Bool {
	return &ws.pcapBenchmarkMode
}

// DisableTrackPersistenceFlag returns a pointer to the atomic.Bool that
// suppresses DB track/observation writes. Wire this into
// TrackingPipelineConfig.DisableTrackPersistence so analysis replays
// and parameter sweeps do not pollute the production track store.
func (ws *Server) DisableTrackPersistenceFlag() *atomic.Bool {
	return &ws.pcapDisableTrackPersistence
}

// updateLatestFgCounts refreshes cached foreground counts for the status UI.
func (ws *Server) updateLatestFgCounts(sensorID string) {
	ws.fgCountsMu.Lock()
	defer ws.fgCountsMu.Unlock()

	for k := range ws.latestFgCounts {
		delete(ws.latestFgCounts, k)
	}

	if sensorID == "" {
		return
	}

	snap := l3grid.GetForegroundSnapshot(sensorID)
	if snap == nil {
		return
	}

	ws.latestFgCounts["total"] = snap.TotalPoints
	ws.latestFgCounts["foreground"] = snap.ForegroundCount
	ws.latestFgCounts["background"] = snap.BackgroundCount
}

// getLatestFgCounts returns a copy to avoid races in templates.
func (ws *Server) getLatestFgCounts() map[string]int {
	ws.fgCountsMu.RLock()
	defer ws.fgCountsMu.RUnlock()

	if len(ws.latestFgCounts) == 0 {
		return nil
	}

	copyMap := make(map[string]int, len(ws.latestFgCounts))
	for k, v := range ws.latestFgCounts {
		copyMap[k] = v
	}
	return copyMap
}

func (ws *Server) resetBackgroundGrid() error {
	mgr := l3grid.GetBackgroundManager(ws.sensorID)
	if mgr == nil {
		return nil
	}
	if err := mgr.ResetGrid(); err != nil {
		return err
	}
	return nil
}

// resetFrameBuilder clears all buffered frame state to prevent stale data
// from contaminating a new data source.
func (ws *Server) resetFrameBuilder() {
	fb := l2frames.GetFrameBuilder(ws.sensorID)
	if fb != nil {
		fb.Reset()
	}
}

// resetAllState performs a comprehensive reset of all processing state
// when switching data sources. This includes the background grid, frame
// builder, tracker, and any other stateful components.
func (ws *Server) resetAllState() error {
	// Reset frame builder first to discard any in-flight frames
	ws.resetFrameBuilder()

	// Reset background grid
	if err := ws.resetBackgroundGrid(); err != nil {
		return err
	}

	// Reset tracker to clear Kalman filter state and restart track IDs from 1
	if ws.tracker != nil {
		ws.tracker.Reset()
	}

	return nil
}
