package pipeline

import (
	"database/sql"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// ForegroundForwarder interface allows forwarding foreground points without importing network package.
type ForegroundForwarder interface {
	ForwardForeground(points []l2frames.PointPolar)
}

// VisualiserPublisher interface allows publishing frames to the gRPC visualiser.
type VisualiserPublisher interface {
	Publish(frame interface{})
}

// VisualiserAdapter interface converts tracking outputs to FrameBundle.
type VisualiserAdapter interface {
	AdaptFrame(frame *l2frames.LiDARFrame, foregroundMask []bool, clusters []l4perception.WorldCluster, tracker l5tracks.TrackerInterface, debugFrame interface{}) interface{}
	// AdaptEmptyFrame creates a minimal FrameBundle with no perception
	// data, preserving the frame's timestamp and ID for 1:1 PCAP-to-VRLOG
	// deterministic recording. The frame is marked as FrameTypeEmpty.
	AdaptEmptyFrame(frame *l2frames.LiDARFrame) interface{}
}

// LidarViewAdapter interface forwards FrameBundle to UDP (LidarView format).
type LidarViewAdapter interface {
	PublishFrameBundle(bundle interface{}, foregroundPoints []l2frames.PointPolar)
}

// isNilInterface checks if an interface value is nil or contains a nil pointer.
// This handles the Go interface nil pitfall where interface{} != nil but the underlying value is nil.
func isNilInterface(i interface{}) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface:
		return v.IsNil()
	}
	return false
}

// IsNilInterface is the exported version of isNilInterface for use by
// backward-compatible alias packages.
var IsNilInterface = isNilInterface

// heapBytes returns the current heap allocation in bytes.
func heapBytes() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// ---------------------------------------------------------------------------
// Stage interfaces — layer-aligned contracts for the tracking pipeline.
//
// These interfaces define the boundaries between processing stages as
// described in docs/lidar/architecture/lidar-layer-alignment-refactor-review.md.
// Current code still uses the monolithic callback below; these contracts
// exist to guide incremental extraction of each stage into its own package.
// ---------------------------------------------------------------------------

// ForegroundStage extracts foreground (moving) points from a frame using
// the learned background model (L3 Grid).
type ForegroundStage interface {
	// ExtractForeground returns a boolean mask where true indicates a foreground point.
	ExtractForeground(polar []l2frames.PointPolar) (mask []bool, err error)
}

// PerceptionStage transforms foreground points into world coordinates,
// applies ground removal, and clusters them (L4 Perception).
type PerceptionStage interface {
	// Perceive takes foreground points and returns world-frame clusters.
	Perceive(foreground []l2frames.PointPolar, sensorID string) ([]l4perception.WorldCluster, error)
}

// TrackingStage updates the tracker state with new cluster observations
// and returns confirmed tracks (L5 Tracks).
type TrackingStage interface {
	// UpdateTracks feeds clusters into the tracker and returns confirmed tracks.
	UpdateTracks(clusters []l4perception.WorldCluster, frameTime time.Time) ([]*l5tracks.TrackedObject, error)
}

// ObjectStage classifies confirmed tracks and attaches semantic labels (L6 Objects).
type ObjectStage interface {
	// Classify assigns or updates object class labels on each track.
	Classify(tracks []*l5tracks.TrackedObject)
}

// PersistenceSink writes pipeline outputs (tracks, observations) to storage.
// It is an adapter — not a domain layer — so implementations live outside
// L3-L6 packages (e.g. internal/lidar/storage/sqlite).
type PersistenceSink interface {
	// PersistTrack writes or updates a track record.
	PersistTrack(track *l5tracks.TrackedObject, frameID string) error
	// PersistObservation writes a single observation for a track.
	PersistObservation(obs *sqlite.TrackObservation) error
}

// PublishSink sends pipeline outputs to external consumers (visualiser, gRPC).
type PublishSink interface {
	// PublishFrame sends a processed frame to external subscribers.
	PublishFrame(frame *l2frames.LiDARFrame, mask []bool, clusters []l4perception.WorldCluster, tracker l5tracks.TrackerInterface)
}

// TrackingPipelineConfig holds dependencies for the tracking pipeline callback.
type TrackingPipelineConfig struct {
	BackgroundManager   *l3grid.BackgroundManager
	FgForwarder         ForegroundForwarder       // Use interface to avoid import cycle
	Tracker             l5tracks.TrackerInterface // Use interface for dependency injection and testing
	Classifier          *l6objects.TrackClassifier
	DB                  *sql.DB // Use standard sql.DB to avoid import cycle with db package
	SensorID            string
	AnalysisRunManager  *sqlite.AnalysisRunManager // Optional: for recording analysis runs
	VisualiserPublisher VisualiserPublisher        // Optional: gRPC publisher
	VisualiserAdapter   VisualiserAdapter          // Optional: adapter for gRPC
	LidarViewAdapter    LidarViewAdapter           // Optional: adapter for UDP forwarding

	// MaxFrameRate caps the rate at which frames are fully processed through
	// the tracking pipeline. When frames arrive faster than this rate (e.g.
	// during PCAP catch-up bursts), excess frames are dropped after background
	// update but before the expensive clustering/tracking/serialisation path.
	// Zero means no limit (process every frame).
	//
	// The value MUST exceed the sensor's maximum frame rate to avoid
	// dropping live data. Hesai Pandar40P runs at 10 or 20 Hz depending
	// on configuration; 25 fps provides 5 fps (25%) headroom above the 20 Hz mode.
	MaxFrameRate float64

	// VoxelLeafSize, when > 0, enables voxel grid downsampling before
	// DBSCAN clustering. Each cubic voxel of this side length (metres) is
	// reduced to a single representative point. Typical value: 0.08.
	// Zero disables voxel downsampling.
	VoxelLeafSize float64

	// FeatureExportFunc, when non-nil, is called for every confirmed track
	// after classification. This hook allows exporting feature vectors for
	// ML training data collection. The callback receives the track's
	// extracted features and the current classification result.
	FeatureExportFunc func(trackID string, features l6objects.TrackFeatures, class string, confidence float32)

	// HeightBandFloor is the lower bound (metres) for the vertical height
	// band filter. Values are in the same frame as the points passed to
	// FilterVertical — typically sensor frame where Z=0 is the sensor's
	// horizontal plane. Default: −2.8 (≈ 0.2 m above road for a ~3 m mount).
	HeightBandFloor float64

	// HeightBandCeiling is the upper bound (metres) for the vertical height
	// band filter. Default: +1.5 (allows tall trucks above sensor height).
	HeightBandCeiling float64

	// RemoveGround, when true (the default), enables the height band filter
	// that removes ground-plane and overhead-structure returns before
	// clustering. Set to false to disable ground removal entirely.
	RemoveGround bool

	// BenchmarkMode, when non-nil and true, enables per-frame performance
	// tracing: stage timing via FrameTimer, slow-frame alerts, periodic
	// health summaries (heap/goroutines), and pipeline lag detection.
	// When nil or false, all timing logic is skipped (zero overhead).
	// Toggle at runtime via atomic store; the pipeline checks each frame.
	BenchmarkMode *atomic.Bool

	// DisableTrackPersistence, when non-nil and true, skips all DB writes
	// (InsertTrack / InsertTrackObservation) for the frame. Use during
	// analysis replays and parameter sweeps to avoid polluting the
	// production track store. Toggle at runtime via atomic store.
	// When nil or false, normal persistence applies.
	DisableTrackPersistence *atomic.Bool
}

// NewFrameCallback creates a FrameBuilder callback that processes frames through
// the full tracking pipeline: foreground extraction, clustering, tracking, and persistence.
func (cfg *TrackingPipelineConfig) NewFrameCallback() func(*l2frames.LiDARFrame) {
	// Snapshot scalar config values so the closure is immune to the caller
	// mutating *cfg after NewFrameCallback returns. Interface/pointer fields
	// (BackgroundManager, Tracker, etc.) are intentionally shared — they
	// carry mutable state that the pipeline must see.
	maxFrameRate := cfg.MaxFrameRate
	voxelLeafSize := cfg.VoxelLeafSize
	heightBandFloor := cfg.HeightBandFloor
	heightBandCeiling := cfg.HeightBandCeiling
	removeGround := cfg.RemoveGround
	sensorID := cfg.SensorID

	// Get AnalysisRunManager from registry if not explicitly set
	// This allows analysis runs to be started/stopped dynamically via webserver
	getRunManager := func() *sqlite.AnalysisRunManager {
		if cfg.AnalysisRunManager != nil {
			return cfg.AnalysisRunManager
		}
		return sqlite.GetAnalysisRunManager(sensorID)
	}

	// Frame-rate throttle state. We always run ProcessFramePolarWithMask to
	// keep the background model up to date, but skip the expensive
	// clustering→tracking→serialisation path when frames arrive faster than
	// MaxFrameRate. This prevents burst processing during PCAP catch-up
	// from consuming 250%+ CPU and flooding the gRPC client with drops.
	var lastProcessedTime time.Time
	var minFrameInterval time.Duration
	if maxFrameRate > 0 {
		minFrameInterval = time.Duration(float64(time.Second) / maxFrameRate)
	}
	var throttledFrames atomic.Uint64

	// Periodic DB pruning state.  Prune deleted tracks once per minute to
	// avoid unbounded storage growth from short-lived spurious tracks.
	const deletedTrackTTL = 5 * time.Minute
	const pruneInterval = 1 * time.Minute
	var lastPruneTime time.Time

	// One-shot diag warnings for disabled features. Fires at most once per
	// callback instance (i.e. per PCAP/session) to avoid flooding the log.
	var logFgForwarderNilOnce sync.Once
	var logGroundDisabledOnce sync.Once

	// Cache the default DBSCAN params once at callback creation time rather
	// than loading from disk on every frame. The per-frame overrides
	// (Eps, MinPts, MaxInputPoints) from BackgroundParams still apply.
	defaultDBSCANParams := l4perception.DefaultDBSCANParams()

	// Pipeline performance tracing state.
	const slowFrameThresholdMs = 50.0 // emit diagf alert when frame exceeds this
	const healthSummaryInterval = 100 // emit health summary every N processed frames
	const timingWindowSize = 100      // rolling window for mean/p95 computation
	var processedFrameCount uint64    // frames that passed throttle and were fully processed
	var frameDurations []float64      // rolling window of frame durations (ms)
	var lastFrameEndTime time.Time    // for lag ratio computation
	var consecutiveBehind int         // consecutive frames where lag > 1.0

	// Deterministic recording: when a visualiser adapter and publisher are
	// configured, every sensor frame must produce a VRLOG entry — even
	// frames with no foreground objects. This helper publishes a minimal
	// empty FrameBundle (FrameTypeEmpty) at early-return points that would
	// otherwise skip Publish().
	hasVisualiser := !isNilInterface(cfg.VisualiserAdapter) && !isNilInterface(cfg.VisualiserPublisher)
	publishEmptyFrame := func(frame *l2frames.LiDARFrame) {
		if !hasVisualiser {
			return
		}
		emptyBundle := cfg.VisualiserAdapter.AdaptEmptyFrame(frame)
		cfg.VisualiserPublisher.Publish(emptyBundle)
	}

	return func(frame *l2frames.LiDARFrame) {
		if frame == nil || len(frame.Points) == 0 {
			return
		}

		// Route frame completion to trace log to keep main log quiet during normal runs.
		tracef("[FrameBuilder] Completed frame: %s, Points: %d, Azimuth: %.1f°-%.1f°",
			frame.FrameID, len(frame.Points), frame.MinAzimuth, frame.MaxAzimuth)

		// Use the frame-owned polar representation directly. The L2
		// frame builder populates PolarPoints alongside Points when
		// AddPointsPolar is used, eliminating the per-frame rebuild.
		polar := frame.PolarPoints

		if cfg.BackgroundManager == nil {
			publishEmptyFrame(frame)
			return
		}

		// Provide extra context at the handoff so we can trace delivery
		var firstAz, lastAz float64
		var firstTS, lastTS int64
		if len(polar) > 0 {
			firstAz = polar[0].Azimuth
			lastAz = polar[len(polar)-1].Azimuth
			firstTS = polar[0].Timestamp
			lastTS = polar[len(polar)-1].Timestamp
		}
		tracef("[FrameBuilder->Pipeline] Delivering frame %s -> %d points (azimuth: %.1f°->%.1f°, ts: %d->%d)",
			frame.FrameID, len(polar), firstAz, lastAz, firstTS, lastTS)

		// Stage 1: Foreground extraction
		mask, err := cfg.BackgroundManager.ProcessFramePolarWithMask(polar)
		if err != nil || mask == nil {
			opsf("Failed to get foreground mask: %v", err)
			publishEmptyFrame(frame)
			return
		}

		foregroundPoints := l3grid.ExtractForegroundPoints(polar, mask)
		totalPoints := len(polar)

		// Build downsampled background subset for debug overlay.
		// Instead of copying all ~67k background points into an intermediate
		// slice then downsampling, count background points first and stride
		// through the mask in a single pass. This avoids a ~5 MB allocation
		// per frame.
		const maxBackgroundChartPoints = 5000
		backgroundCount := totalPoints - len(foregroundPoints)
		stride := 1
		if backgroundCount > maxBackgroundChartPoints {
			stride = backgroundCount / maxBackgroundChartPoints
		}
		cap := backgroundCount
		if cap > maxBackgroundChartPoints {
			cap = maxBackgroundChartPoints
		}
		backgroundPolar := make([]l2frames.PointPolar, 0, cap)
		bgIdx := 0
		for i, isForeground := range mask {
			if isForeground {
				continue
			}
			if bgIdx%stride == 0 && len(backgroundPolar) < maxBackgroundChartPoints {
				backgroundPolar = append(backgroundPolar, polar[i])
			}
			bgIdx++
		}

		// Cache sensor-frame projections for debug visualization (aligns with polar background chart)
		l3grid.StoreForegroundSnapshot(sensorID, frame.StartTimestamp, foregroundPoints, backgroundPolar, totalPoints, len(foregroundPoints))

		if len(foregroundPoints) == 0 {
			// No foreground detected — still record an empty frame for
			// deterministic 1:1 PCAP-to-VRLOG mapping.
			publishEmptyFrame(frame)
			return
		}

		// Frame-rate throttle: skip the expensive downstream pipeline
		// (clustering, tracking, serialisation) when frames arrive faster
		// than MaxFrameRate. Background model update above still runs on
		// every frame so foreground extraction stays accurate.
		if minFrameInterval > 0 {
			now := time.Now()
			if !lastProcessedTime.IsZero() && now.Sub(lastProcessedTime) < minFrameInterval {
				count := throttledFrames.Add(1)
				if count%50 == 0 {
					diagf("[Pipeline] Throttled %d frames (max %.0f fps)", count, maxFrameRate)
				}
				// Do NOT advance miss counters during throttle.
				// PCAP catch-up floods frames faster than real-time;
				// advancing misses would kill tentative tracks
				// (max_misses=3) within ~300 ms. Live sensors never
				// reach this path (MaxFrameRate > sensor Hz), so
				// skipping AdvanceMisses has no effect on live tracking.
				//
				// Still record the frame for deterministic VRLOG mapping.
				publishEmptyFrame(frame)
				return
			}
			lastProcessedTime = now
		}

		// --- Performance Tracing ---
		// Timer starts after throttle check to capture only fully-processed frames.
		// Only active when BenchmarkMode is enabled (zero overhead otherwise).
		// All benchmark output uses opsf() (always visible) because the user
		// explicitly opted in via the dashboard checkbox. Using tracef/diagf
		// would hide output at the default --log-level=ops.
		var ft *frameTimer
		var emitTiming func(nPoints, nClusters, nTracks int)
		if cfg.BenchmarkMode != nil && cfg.BenchmarkMode.Load() {
			ft = newFrameTimer(frame.FrameID)
			emitTiming = func(nPoints, nClusters, nTracks int) {
				ft.End()
				totalMs := ft.TotalMs()
				processedFrameCount++

				opsf("[Benchmark] frame=%s total=%.1fms %s points=%d clusters=%d tracks=%d",
					frame.FrameID, totalMs, ft.Format(), nPoints, nClusters, nTracks)

				if totalMs > slowFrameThresholdMs {
					slowName, slowDur := ft.SlowestStage()
					opsf("[Benchmark] SLOW frame=%s total=%.1fms slowest=%s(%.1fms) %s points=%d clusters=%d tracks=%d",
						frame.FrameID, totalMs, slowName, float64(slowDur.Nanoseconds())/1e6,
						ft.Format(), nPoints, nClusters, nTracks)
				}

				// Rolling window of frame durations
				if len(frameDurations) >= timingWindowSize {
					frameDurations = frameDurations[1:]
				}
				frameDurations = append(frameDurations, totalMs)

				// Periodic health summary
				if processedFrameCount%uint64(healthSummaryInterval) == 0 && len(frameDurations) > 0 {
					window := make([]float64, len(frameDurations))
					copy(window, frameDurations)
					sort.Float64s(window)
					var sum float64
					for _, d := range window {
						sum += d
					}
					mean := sum / float64(len(window))
					p95Idx := int(float64(len(window)) * 0.95)
					if p95Idx >= len(window) {
						p95Idx = len(window) - 1
					}
					opsf("[Benchmark] health: processed=%d throttled=%d mean=%.1fms p95=%.1fms heap=%.1fMB goroutines=%d",
						processedFrameCount, throttledFrames.Load(), mean, window[p95Idx],
						float64(heapBytes())/1024/1024, runtime.NumGoroutine())
				}

				// Lag tracking: detect when processing falls behind frame arrival rate
				now := time.Now()
				if !lastFrameEndTime.IsZero() {
					interFrameGap := now.Sub(lastFrameEndTime)
					frameDur := ft.Total()
					if interFrameGap > 0 && frameDur > interFrameGap {
						consecutiveBehind++
						if consecutiveBehind >= 3 {
							lagRatio := float64(frameDur) / float64(interFrameGap)
							opsf("[Benchmark] BEHIND: lag=%.1fx (processing %.1fms, interval %.1fms) behind for %d frames",
								lagRatio, totalMs, float64(interFrameGap.Nanoseconds())/1e6, consecutiveBehind)
						}
					} else {
						consecutiveBehind = 0
					}
				}
				lastFrameEndTime = now
			}
			ft.Stage("forward")
		}

		// Forward foreground points on 2370-style stream if configured
		// Use isNilInterface to handle Go interface nil pitfall
		if !isNilInterface(cfg.FgForwarder) {
			pointsToForward := foregroundPoints
			// If debug range is configured, only forward points within that range
			// This allows isolating specific regions for debugging without flooding the stream
			params := cfg.BackgroundManager.GetParams()
			if params.HasDebugRange() {
				filtered := make([]l2frames.PointPolar, 0, len(foregroundPoints))
				for _, p := range foregroundPoints {
					// Channel is 1-based in PointPolar, but 0-based in params/grid
					if params.IsInDebugRange(p.Channel-1, p.Azimuth) {
						filtered = append(filtered, p)
					}
				}
				pointsToForward = filtered
			}
			if len(pointsToForward) > 0 {
				cfg.FgForwarder.ForwardForeground(pointsToForward)
			}
		} else {
			logFgForwarderNilOnce.Do(func() {
				diagf("FgForwarder is nil, skipping foreground forwarding")
			})
		}

		// Always log foreground extraction for tracking debugging
		tracef("Extracted %d foreground points from %d total", len(foregroundPoints), len(polar))

		// Stage 2: Transform to world coordinates
		if ft != nil {
			ft.Stage("transform")
		}
		worldPoints := l4perception.TransformToWorld(foregroundPoints, nil, sensorID)

		// Stage 2b: Ground removal (vertical filtering)
		// Remove ground plane and overhead structure returns to reduce false clusters.
		// Bounds are in sensor frame (identity pose): Z=0 is the sensor's horizontal
		// plane, ground is at approximately −3.0 m for a ~3 m mount height.
		filteredPoints := worldPoints
		if removeGround {
			var groundFilter *l4perception.HeightBandFilter
			if heightBandFloor != 0 || heightBandCeiling != 0 {
				groundFilter = l4perception.NewHeightBandFilter(heightBandFloor, heightBandCeiling)
			} else {
				groundFilter = l4perception.DefaultHeightBandFilter()
			}
			filteredPoints = groundFilter.FilterVertical(worldPoints)
			proc, kept, below, above := groundFilter.Stats()
			tracef("Ground filter: %d processed, %d kept, %d below floor, %d above ceiling",
				proc, kept, below, above)
		} else {
			logGroundDisabledOnce.Do(func() {
				diagf("Ground removal disabled, passing %d points through", len(worldPoints))
			})
		}

		if len(filteredPoints) == 0 {
			if emitTiming != nil {
				emitTiming(len(foregroundPoints), 0, 0)
			}
			publishEmptyFrame(frame)
			return
		}

		// Stage 2c: Voxel grid downsampling (optional).
		// Reduces point density while preserving spatial structure, which
		// tightens cluster boundaries and speeds up DBSCAN.
		if voxelLeafSize > 0 {
			before := len(filteredPoints)
			filteredPoints = l4perception.VoxelGrid(filteredPoints, voxelLeafSize)
			tracef("Voxel downsample: %d → %d (leaf=%.3fm)",
				before, len(filteredPoints), voxelLeafSize)
		}

		// Stage 3: Clustering (runtime-tunable via background params)
		if ft != nil {
			ft.Stage("cluster")
		}
		dbscanParams := defaultDBSCANParams
		params := cfg.BackgroundManager.GetParams()
		if params.ForegroundMinClusterPoints > 0 {
			dbscanParams.MinPts = params.ForegroundMinClusterPoints
		}
		if params.ForegroundDBSCANEps > 0 {
			dbscanParams.Eps = float64(params.ForegroundDBSCANEps)
		}
		// Cap input points to bound worst-case DBSCAN runtime.
		// Uses foreground_max_input_points from tuning (default 8000) so
		// operators can trade detection accuracy for performance at runtime.
		maxInputPoints := params.ForegroundMaxInputPoints
		if maxInputPoints <= 0 {
			maxInputPoints = 8000
		}
		dbscanParams.MaxInputPoints = maxInputPoints

		clusters := l4perception.DBSCAN(filteredPoints, dbscanParams)
		if len(clusters) == 0 {
			// No clusters, but still record foreground stats (all points are noise)
			if cfg.Tracker != nil {
				cfg.Tracker.RecordFrameStats(len(filteredPoints), 0)
			}
			if emitTiming != nil {
				emitTiming(len(foregroundPoints), 0, 0)
			}
			publishEmptyFrame(frame)
			return
		}

		// Record foreground capture stats: total foreground vs clustered points
		if cfg.Tracker != nil {
			clusteredPointCount := 0
			for _, c := range clusters {
				clusteredPointCount += c.PointsCount
			}
			cfg.Tracker.RecordFrameStats(len(filteredPoints), clusteredPointCount)
		}

		// Record clusters for analysis run if active
		if runManager := getRunManager(); runManager != nil && runManager.IsRunActive() {
			runManager.RecordFrame(frame.StartTimestamp.UnixNano())
			runManager.RecordClusters(len(clusters))
		}

		// Always log clustering for tracking debugging
		tracef("Clustered into %d objects", len(clusters))

		// Stage 4: Track update
		if ft != nil {
			ft.Stage("track")
		}
		if cfg.Tracker == nil {
			if emitTiming != nil {
				emitTiming(len(foregroundPoints), len(clusters), 0)
			}
			publishEmptyFrame(frame)
			return
		}

		cfg.Tracker.Update(clusters, frame.StartTimestamp)

		// Stage 5: Classify and persist confirmed tracks
		if ft != nil {
			ft.Stage("classify")
		}
		confirmedTracks := cfg.Tracker.GetConfirmedTracks()
		tracef("%d confirmed tracks to persist", len(confirmedTracks))

		// Open a per-frame transaction for batching all track/observation writes.
		// Skip entirely when DisableTrackPersistence is set (e.g. analysis replay)
		// or when there are no confirmed tracks to persist.
		var (
			dbTx       *sql.Tx
			frameID string
			txFailed   bool
		)
		if len(confirmedTracks) > 0 && cfg.DB != nil && (cfg.DisableTrackPersistence == nil || !cfg.DisableTrackPersistence.Load()) {
			frameID = fmt.Sprintf("site/%s", sensorID)
			if tx, txErr := cfg.DB.Begin(); txErr != nil {
				opsf("Failed to begin track persistence tx: %v", txErr)
			} else {
				dbTx = tx
			}
		}

		for _, track := range confirmedTracks {
			// Re-classify periodically as more observations accumulate.
			// Run every 5 observations after the initial classification
			// so the label improves as kinematic history grows.
			if cfg.Classifier != nil && track.ObservationCount >= cfg.Classifier.MinObservations {
				needsClassify := track.ObjectClass == "" ||
					(track.ObservationCount%5 == 0)
				if needsClassify {
					cfg.Classifier.ClassifyAndUpdate(track)
					// Write classification back to the live track under the
					// tracker lock so subsequent snapshots carry the label
					// and concurrent readers see consistent state (task 4.3).
					cfg.Tracker.UpdateClassification(
						track.TrackID,
						track.ObjectClass,
						track.ObjectConfidence,
						track.ClassificationModel,
					)
				}
			}

			// Export feature vector via hook (for ML training data)
			if cfg.FeatureExportFunc != nil && cfg.Classifier != nil && track.ObservationCount >= cfg.Classifier.MinObservations {
				features := l6objects.ExtractTrackFeatures(track)
				cfg.FeatureExportFunc(track.TrackID, features, track.ObjectClass, track.ObjectConfidence)
			}

			// Record track for analysis run if active
			if runManager := getRunManager(); runManager != nil && runManager.IsRunActive() {
				runManager.RecordTrack(track)
			}

			// Persist track to database
			if dbTx != nil && !txFailed {
				if err := sqlite.InsertTrack(dbTx, track, frameID); err != nil {
					opsf("Failed to insert track %s: %v", track.TrackID, err)
					txFailed = true
				}

				// Only persist observations for tracks that were matched to a
				// cluster this frame (Misses == 0).  Coasting tracks have
				// Misses > 0 and their position is a Kalman prediction, not a
				// real measurement — persisting those creates phantom straight
				// segments and contaminates quality metrics.
				if track.Misses == 0 {
					// Insert observation — use per-frame OBB dimensions (not running
					// averages) so each observation faithfully records the cluster
					// shape at this instant. The averaged values are stored on the
					// track record itself for classification/reporting.
					obs := &sqlite.TrackObservation{
						TrackID:           track.TrackID,
						TSUnixNanos:       frame.StartTimestamp.UnixNano(),
						FrameID:           frameID,
						X:                 track.X,
						Y:                 track.Y,
						Z:                 track.LatestZ,
						VelocityX:         track.VX,
						VelocityY:         track.VY,
						SpeedMps:          track.AvgSpeedMps,
						HeadingRad:        track.OBBHeadingRad,
						BoundingBoxLength: track.OBBLength,
						BoundingBoxWidth:  track.OBBWidth,
						BoundingBoxHeight: track.OBBHeight,
						HeightP95:         track.HeightP95Max,
						IntensityMean:     track.IntensityMeanAvg,
					}
					if err := sqlite.InsertTrackObservation(dbTx, obs); err != nil {
						opsf("Failed to insert observation for track %s: %v", track.TrackID, err)
						txFailed = true
					}
				}
			}
		}

		if dbTx != nil {
			if txFailed {
				if err := dbTx.Rollback(); err != nil {
					opsf("Failed to rollback track persistence tx: %v", err)
				}
			} else if err := dbTx.Commit(); err != nil {
				opsf("Failed to commit track persistence tx: %v", err)
			}
		}

		if len(confirmedTracks) > 0 {
			diagf("%d confirmed tracks active", len(confirmedTracks))
		}

		// Stage 6: Publish to visualiser (if enabled)
		if ft != nil {
			ft.Stage("publish")
		}
		if !isNilInterface(cfg.VisualiserAdapter) && !isNilInterface(cfg.VisualiserPublisher) {
			// Adapt frame to FrameBundle
			// Note: Debug collector is integrated in Tracker but requires explicit enablement
			// via Tracker.SetDebugCollector(). Pass nil here as debug collection is optional.
			frameBundle := cfg.VisualiserAdapter.AdaptFrame(frame, mask, clusters, cfg.Tracker, nil)

			// Publish to gRPC stream
			cfg.VisualiserPublisher.Publish(frameBundle)

			// Also forward to LidarView UDP if adapter is configured
			if !isNilInterface(cfg.LidarViewAdapter) {
				cfg.LidarViewAdapter.PublishFrameBundle(frameBundle, foregroundPoints)
			}

			tracef("[Visualiser] Published frame %s to gRPC", frame.FrameID)
		} else if !isNilInterface(cfg.LidarViewAdapter) {
			// LidarView-only mode (no gRPC)
			// Create a minimal bundle just for LidarView forwarding
			// This preserves the existing behavior when gRPC is disabled
			cfg.LidarViewAdapter.PublishFrameBundle(nil, foregroundPoints)
		}

		if emitTiming != nil {
			emitTiming(len(foregroundPoints), len(clusters), len(confirmedTracks))
		}

		// Stage 7: Periodic DB pruning of deleted tracks.
		// Runs at most once per pruneInterval to avoid contention.
		if cfg.DB != nil {
			now := time.Now()
			if lastPruneTime.IsZero() || now.Sub(lastPruneTime) >= pruneInterval {
				lastPruneTime = now
				if pruned, err := sqlite.PruneDeletedTracks(cfg.DB, sensorID, deletedTrackTTL); err != nil {
					opsf("Prune deleted tracks failed: %v", err)
				} else if pruned > 0 {
					diagf("Pruned %d deleted tracks older than %v", pruned, deletedTrackTTL)
				}
			}
		}
	}
}
