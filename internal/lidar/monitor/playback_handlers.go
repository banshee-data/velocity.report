package monitor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// handlePCAPStart switches the data source to PCAP replay and starts ingestion.
// Method: POST. Query param: sensor_id (required to match configured sensor).
func (ws *WebServer) handlePCAPStart(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	if sensorID != ws.sensorID {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("unexpected sensor_id '%s'", sensorID))
		return
	}

	var pcapFile string
	analysisMode := true // Default: always create analysis run + VRLOG
	var speedMode string
	var speedRatio float64 = 1.0
	var startSeconds float64 = 0
	var durationSeconds float64 = -1
	var debugRingMin, debugRingMax int
	var debugAzMin, debugAzMax float32
	var enableDebug bool
	var enablePlots bool

	// Accept both JSON and form data
	contentType := r.Header.Get("Content-Type")
	if contentType == "application/json" || contentType == "application/json; charset=utf-8" {
		// Parse JSON body
		var req struct {
			PCAPFile        string  `json:"pcap_file"`
			AnalysisMode    bool    `json:"analysis_mode"`
			SpeedMode       string  `json:"speed_mode"`
			SpeedRatio      float64 `json:"speed_ratio"`
			StartSeconds    float64 `json:"start_seconds"`
			DurationSeconds float64 `json:"duration_seconds"`
			DebugRingMin    int     `json:"debug_ring_min"`
			DebugRingMax    int     `json:"debug_ring_max"`
			DebugAzMin      float32 `json:"debug_az_min"`
			DebugAzMax      float32 `json:"debug_az_max"`
			EnableDebug     bool    `json:"enable_debug"`
			EnablePlots     bool    `json:"enable_plots"`
		}
		// Set defaults
		req.DurationSeconds = -1
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				ws.writeJSONError(w, http.StatusBadRequest, "missing JSON body for PCAP request")
				return
			}
			ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
			return
		}
		pcapFile = req.PCAPFile
		analysisMode = req.AnalysisMode
		speedMode = req.SpeedMode
		if req.SpeedRatio > 0 {
			speedRatio = req.SpeedRatio
		}
		startSeconds = req.StartSeconds
		durationSeconds = req.DurationSeconds
		debugRingMin = req.DebugRingMin
		debugRingMax = req.DebugRingMax
		debugAzMin = req.DebugAzMin
		debugAzMax = req.DebugAzMax
		enableDebug = req.EnableDebug
		enablePlots = req.EnablePlots
	} else {
		// Parse form data (default for HTML forms)
		if err := r.ParseForm(); err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid form data: %v", err))
			return
		}
		pcapFile = r.FormValue("pcap_file")
		analysisMode = r.FormValue("analysis_mode") == "true" || r.FormValue("analysis_mode") == "1"
		speedMode = r.FormValue("speed_mode")
		if v := r.FormValue("speed_ratio"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
				speedRatio = f
			}
		}
		if v := r.FormValue("start_seconds"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
				startSeconds = f
			}
		}
		if v := r.FormValue("duration_seconds"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				durationSeconds = f
			}
		} else {
			durationSeconds = -1
		}
		if v := r.FormValue("debug_ring_min"); v != "" {
			if i, err := strconv.Atoi(v); err == nil && i >= 0 {
				debugRingMin = i
			}
		}
		if v := r.FormValue("debug_ring_max"); v != "" {
			if i, err := strconv.Atoi(v); err == nil && i >= 0 {
				debugRingMax = i
			}
		}
		if v := r.FormValue("debug_az_min"); v != "" {
			if f, err := strconv.ParseFloat(v, 32); err == nil && f >= 0 {
				debugAzMin = float32(f)
			}
		}
		if v := r.FormValue("debug_az_max"); v != "" {
			if f, err := strconv.ParseFloat(v, 32); err == nil && f >= 0 {
				debugAzMax = float32(f)
			}
		}
		enableDebug = r.FormValue("enable_debug") == "true" || r.FormValue("enable_debug") == "1"
		enablePlots = r.FormValue("enable_plots") == "true" || r.FormValue("enable_plots") == "1"
	}

	if speedMode == "" {
		speedMode = "fastest"
	}

	if pcapFile == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'pcap_file' in request body")
		return
	}

	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()

	if ws.currentSource == DataSourcePCAP {
		ws.writeJSONError(w, http.StatusConflict, "PCAP replay already active")
		return
	}

	ws.stopLiveListenerLocked()

	if err := ws.resetAllState(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to reset background grid: %v", err))
		if restartErr := ws.startLiveListenerLocked(); restartErr != nil {
			log.Printf("Failed to restart live listener after reset error: %v", restartErr)
			return
		}
		return
	}

	// Set source path on BackgroundManager for region restoration
	// This allows skipping settling when replaying the same PCAP file
	if mgr := l3grid.GetBackgroundManager(ws.sensorID); mgr != nil {
		mgr.SetSourcePath(pcapFile)
	}

	// Set analysis mode BEFORE startPCAPLocked so the goroutine inside can
	// read it immediately without a race.
	ws.pcapMu.Lock()
	ws.pcapAnalysisMode = analysisMode
	ws.pcapMu.Unlock()

	if err := ws.startPCAPLocked(pcapFile, speedMode, speedRatio, startSeconds, durationSeconds, debugRingMin, debugRingMax, debugAzMin, debugAzMax, enableDebug, enablePlots); err != nil {
		var sErr *switchError
		if errors.As(err, &sErr) {
			ws.writeJSONError(w, sErr.status, sErr.Error())
		} else {
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		if restartErr := ws.startLiveListenerLocked(); restartErr != nil {
			log.Printf("Failed to restart live listener after PCAP error: %v", restartErr)
		}
		return
	}

	ws.currentSource = DataSourcePCAP
	currentFile := ws.currentPCAPFile

	mode := "replay"
	if analysisMode {
		mode = "analysis"
	}
	log.Printf("[DataSource] switched to PCAP %s mode for sensor=%s file=%s", mode, sensorID, currentFile)

	// Notify visualiser gRPC server that we are now replaying.
	// NOTE: This must happen before the goroutine produces any frames so that
	// the gRPC server injects PlaybackInfo into streamed frames. The goroutine
	// won't emit frames until after pre-counting completes (which takes measurable
	// time), so this call races are unlikely in practice. However, the goroutine
	// also calls onPCAPStarted internally as a belt-and-braces guard.
	if ws.onPCAPStarted != nil {
		ws.onPCAPStarted()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "started",
		"sensor_id":      sensorID,
		"current_source": string(ws.currentSource),
		"pcap_file":      currentFile,
		"analysis_mode":  analysisMode,
	})
}

// handlePCAPStop cancels any active PCAP replay and returns to live UDP.
// Method: POST. Query param: sensor_id (required to match configured sensor).
func (ws *WebServer) handlePCAPStop(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = r.FormValue("sensor_id")
	}
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	if sensorID != ws.sensorID {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("unexpected sensor_id '%s'", sensorID))
		return
	}

	// Acquire dataSourceMu first to maintain consistent lock ordering with handlePCAPStart
	// (always dataSourceMu → pcapMu) to prevent deadlock
	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()

	if ws.currentSource != DataSourcePCAP && ws.currentSource != DataSourcePCAPAnalysis {
		ws.writeJSONError(w, http.StatusConflict, "system is not in PCAP mode")
		return
	}

	// Now acquire pcapMu while holding dataSourceMu (consistent ordering)
	ws.pcapMu.Lock()
	if !ws.pcapInProgress {
		ws.pcapMu.Unlock()
		ws.writeJSONError(w, http.StatusConflict, "no PCAP replay in progress")
		return
	}
	cancel := ws.pcapCancel
	done := ws.pcapDone
	ws.pcapCancel = nil
	ws.pcapDone = nil
	ws.pcapMu.Unlock()

	// Release dataSourceMu before waiting for goroutine completion to avoid deadlock
	// (the PCAP goroutine needs dataSourceMu to finish)
	// NOTE: We must unlock manually here because we need to wait for done.
	// Since handlePCAPStop defers the release of dataSourceMu, we must re-lock before returning.
	ws.dataSourceMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	// Reacquire dataSourceMu for subsequent operations
	// This lock will be released by the deferred Unlock when function returns
	ws.dataSourceMu.Lock()

	// If in analysis mode, only reset grid if explicitly requested
	ws.pcapMu.Lock()
	analysisMode := ws.pcapAnalysisMode
	ws.pcapAnalysisMode = false // Clear flag when stopping
	ws.pcapMu.Unlock()

	if !analysisMode {
		// Normal mode: always reset all state when stopping
		if err := ws.resetAllState(); err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to reset state: %v", err))
			return
		}
	} else {
		// Analysis mode: still reset frame builder to clear stale frames
		ws.resetFrameBuilder()
		log.Printf("[DataSource] preserving grid from PCAP analysis for sensor=%s", sensorID)
	}

	// Clear source path since we're returning to live mode
	if mgr := l3grid.GetBackgroundManager(ws.sensorID); mgr != nil {
		mgr.SetSourcePath("")
	}

	if err := ws.startLiveListenerLocked(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start live listener: %v", err))
		return
	}

	ws.currentSource = DataSourceLive
	ws.currentPCAPFile = ""

	log.Printf("[DataSource] switched to Live after PCAP stop for sensor=%s", sensorID)

	// Notify visualiser gRPC server that replay has ended
	if ws.onPCAPStopped != nil {
		ws.onPCAPStopped()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "stopped",
		"sensor_id":      sensorID,
		"current_source": string(ws.currentSource),
	})
}

// handlePCAPResumeLive switches from PCAP analysis mode back to Live while preserving the background grid.
// This allows overlaying live data on top of PCAP-analyzed background.
// Method: POST. Query param: sensor_id (required to match configured sensor).
func (ws *WebServer) handlePCAPResumeLive(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = r.FormValue("sensor_id")
	}
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	if sensorID != ws.sensorID {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("unexpected sensor_id '%s'", sensorID))
		return
	}

	ws.dataSourceMu.Lock()
	defer ws.dataSourceMu.Unlock()

	if ws.currentSource != DataSourcePCAPAnalysis {
		ws.writeJSONError(w, http.StatusConflict, "system is not in PCAP analysis mode")
		return
	}

	// Start live listener without resetting the grid
	if err := ws.startLiveListenerLocked(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start live listener: %v", err))
		return
	}

	ws.currentSource = DataSourceLive
	ws.currentPCAPFile = ""

	ws.pcapMu.Lock()
	ws.pcapAnalysisMode = false
	ws.pcapMu.Unlock()

	log.Printf("[DataSource] resumed Live from PCAP analysis for sensor=%s (grid preserved)", sensorID)

	// Notify visualiser gRPC server that replay has ended
	if ws.onPCAPStopped != nil {
		ws.onPCAPStopped()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "resumed_live",
		"sensor_id":      sensorID,
		"current_source": string(ws.currentSource),
		"grid_preserved": true,
	})
}

// handlePlaybackStatus returns the current playback state.
// GET /api/lidar/playback/status
func (ws *WebServer) handlePlaybackStatus(w http.ResponseWriter, r *http.Request) {
	if ws.getPlaybackStatus == nil {
		// Return default live status when no playback callback is configured
		status := &PlaybackStatusInfo{
			Mode:     "live",
			Paused:   false,
			Rate:     1.0,
			Seekable: false,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
		return
	}

	status := ws.getPlaybackStatus()
	if status == nil {
		status = &PlaybackStatusInfo{
			Mode:     "live",
			Paused:   false,
			Rate:     1.0,
			Seekable: false,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handlePlaybackPause pauses playback.
// POST /api/lidar/playback/pause
func (ws *WebServer) handlePlaybackPause(w http.ResponseWriter, r *http.Request) {
	if ws.onPlaybackPause == nil {
		ws.writeJSONError(w, http.StatusNotImplemented, "playback pause not configured")
		return
	}

	ws.onPlaybackPause()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"paused":  true,
	})
}

// handlePlaybackPlay resumes playback.
// POST /api/lidar/playback/play
func (ws *WebServer) handlePlaybackPlay(w http.ResponseWriter, r *http.Request) {
	if ws.onPlaybackPlay == nil {
		ws.writeJSONError(w, http.StatusNotImplemented, "playback play not configured")
		return
	}

	ws.onPlaybackPlay()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"paused":  false,
	})
}

// handlePlaybackSeek seeks to a specific timestamp.
// POST /api/lidar/playback/seek
// Body: {"timestamp_ns": 1234567890}
func (ws *WebServer) handlePlaybackSeek(w http.ResponseWriter, r *http.Request) {
	if ws.onPlaybackSeek == nil {
		ws.writeJSONError(w, http.StatusNotImplemented, "playback seek not configured")
		return
	}

	var body struct {
		TimestampNs int64 `json:"timestamp_ns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := ws.onPlaybackSeek(body.TimestampNs); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("seek failed: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"timestamp_ns": body.TimestampNs,
	})
}

// handlePlaybackRate sets the playback rate.
// POST /api/lidar/playback/rate
// Body: {"rate": 1.5}
func (ws *WebServer) handlePlaybackRate(w http.ResponseWriter, r *http.Request) {
	if ws.onPlaybackRate == nil {
		ws.writeJSONError(w, http.StatusNotImplemented, "playback rate not configured")
		return
	}

	var body struct {
		Rate float32 `json:"rate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Rate <= 0 || body.Rate > 100 {
		ws.writeJSONError(w, http.StatusBadRequest, "rate must be greater than 0 and at most 100")
		return
	}

	ws.onPlaybackRate(body.Rate)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"rate":    body.Rate,
	})
}

// handleVRLogLoad loads a VRLOG for replay by run ID.
// POST /api/lidar/vrlog/load
// Body: {"run_id": "abc123"} or {"vrlog_path": "/path/to/vrlog"}
func (ws *WebServer) handleVRLogLoad(w http.ResponseWriter, r *http.Request) {

	if ws.onVRLogLoad == nil {
		ws.writeJSONError(w, http.StatusNotImplemented, "vrlog load not configured")
		return
	}

	var body struct {
		RunID     string `json:"run_id"`
		VRLogPath string `json:"vrlog_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var vrlogPath string

	// If run_id is provided, look up the vrlog_path from the database
	if body.RunID != "" {
		if ws.db == nil {
			ws.writeJSONError(w, http.StatusInternalServerError, "database not configured")
			return
		}
		store := sqlite.NewAnalysisRunStore(ws.db.DB)
		run, err := store.GetRun(body.RunID)
		if err != nil {
			ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("run not found: %v", err))
			return
		}
		if run.VRLogPath == "" {
			ws.writeJSONError(w, http.StatusBadRequest, "run has no vrlog_path")
			return
		}
		vrlogPath = run.VRLogPath
	} else if body.VRLogPath != "" {
		vrlogPath = body.VRLogPath
	} else {
		ws.writeJSONError(w, http.StatusBadRequest, "run_id or vrlog_path required")
		return
	}

	// Path validation to prevent directory traversal and restrict to data directory
	baseVRLogDir := ws.vrlogSafeDir
	if baseVRLogDir == "" {
		// Default to /var/lib/velocity-report if not configured
		baseVRLogDir = "/var/lib/velocity-report"
	}
	cleanedPath := filepath.Clean(vrlogPath)

	if !filepath.IsAbs(cleanedPath) {
		ws.writeJSONError(w, http.StatusBadRequest, "vrlog_path must be absolute")
		return
	}

	baseWithSep := baseVRLogDir + string(os.PathSeparator)
	if cleanedPath != baseVRLogDir && !strings.HasPrefix(cleanedPath, baseWithSep) {
		ws.writeJSONError(w, http.StatusBadRequest, "vrlog_path must be within allowed directory")
		return
	}

	vrlogPath = cleanedPath

	if err := ws.onVRLogLoad(vrlogPath); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to load vrlog: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"vrlog_path": vrlogPath,
	})
}

// handleVRLogStop stops VRLOG replay and returns to live mode.
// POST /api/lidar/vrlog/stop
func (ws *WebServer) handleVRLogStop(w http.ResponseWriter, r *http.Request) {
	if ws.onVRLogStop == nil {
		ws.writeJSONError(w, http.StatusNotImplemented, "vrlog stop not configured")
		return
	}

	ws.onVRLogStop()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}
