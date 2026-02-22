package pipeline

import (
	"database/sql"
	"fmt"
	"reflect"
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
	ForwardForeground(points []l4perception.PointPolar)
}

// VisualiserPublisher interface allows publishing frames to the gRPC visualiser.
type VisualiserPublisher interface {
	Publish(frame interface{})
}

// VisualiserAdapter interface converts tracking outputs to FrameBundle.
type VisualiserAdapter interface {
	AdaptFrame(frame *l2frames.LiDARFrame, foregroundMask []bool, clusters []l4perception.WorldCluster, tracker l5tracks.TrackerInterface, debugFrame interface{}) interface{}
}

// LidarViewAdapter interface forwards FrameBundle to UDP (LidarView format).
type LidarViewAdapter interface {
	PublishFrameBundle(bundle interface{}, foregroundPoints []l4perception.PointPolar)
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

// ---------------------------------------------------------------------------
// Stage interfaces — layer-aligned contracts for the tracking pipeline.
//
// These interfaces define the boundaries between processing stages as
// described in docs/lidar/architecture/lidar-layer-alignment-refactor-review-20260217.md.
// Current code still uses the monolithic callback below; these contracts
// exist to guide incremental extraction of each stage into its own package.
// ---------------------------------------------------------------------------

// ForegroundStage extracts foreground (moving) points from a frame using
// the learned background model (L3 Grid).
type ForegroundStage interface {
	// ExtractForeground returns a boolean mask where true indicates a foreground point.
	ExtractForeground(polar []l4perception.PointPolar) (mask []bool, err error)
}

// PerceptionStage transforms foreground points into world coordinates,
// applies ground removal, and clusters them (L4 Perception).
type PerceptionStage interface {
	// Perceive takes foreground points and returns world-frame clusters.
	Perceive(foreground []l4perception.PointPolar, sensorID string) ([]l4perception.WorldCluster, error)
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
	PersistTrack(track *l5tracks.TrackedObject, worldFrame string) error
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
	// Zero means no limit (process every frame). Typical value: 12.
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
}

// NewFrameCallback creates a FrameBuilder callback that processes frames through
// the full tracking pipeline: foreground extraction, clustering, tracking, and persistence.
func (cfg *TrackingPipelineConfig) NewFrameCallback() func(*l2frames.LiDARFrame) {
	// Get AnalysisRunManager from registry if not explicitly set
	// This allows analysis runs to be started/stopped dynamically via webserver
	getRunManager := func() *sqlite.AnalysisRunManager {
		if cfg.AnalysisRunManager != nil {
			return cfg.AnalysisRunManager
		}
		return sqlite.GetAnalysisRunManager(cfg.SensorID)
	}

	// Pre-allocate a reusable polar slice to avoid 69k-element allocation
	// per frame (~5 MB). Safe because the callback runs synchronously.
	var polarBuf []l4perception.PointPolar

	// Frame-rate throttle state. We always run ProcessFramePolarWithMask to
	// keep the background model up to date, but skip the expensive
	// clustering→tracking→serialisation path when frames arrive faster than
	// MaxFrameRate. This prevents burst processing during PCAP catch-up
	// from consuming 250%+ CPU and flooding the gRPC client with drops.
	var lastProcessedTime time.Time
	var minFrameInterval time.Duration
	if cfg.MaxFrameRate > 0 {
		minFrameInterval = time.Duration(float64(time.Second) / cfg.MaxFrameRate)
	}
	var throttledFrames atomic.Uint64

	// Periodic DB pruning state.  Prune deleted tracks once per minute to
	// avoid unbounded storage growth from short-lived spurious tracks.
	const deletedTrackTTL = 5 * time.Minute
	const pruneInterval = 1 * time.Minute
	var lastPruneTime time.Time

	return func(frame *l2frames.LiDARFrame) {
		if frame == nil || len(frame.Points) == 0 {
			return
		}

		// Route frame completion to trace log to keep main log quiet during normal runs.
		tracef("[FrameBuilder] Completed frame: %s, Points: %d, Azimuth: %.1f°-%.1f°",
			frame.FrameID, len(frame.Points), frame.MinAzimuth, frame.MaxAzimuth)

		// Convert frame points to polar coordinates using reusable buffer
		n := len(frame.Points)
		if cap(polarBuf) < n {
			polarBuf = make([]l4perception.PointPolar, n)
		}
		polar := polarBuf[:n]
		for i, p := range frame.Points {
			polar[i] = l4perception.PointPolar{
				Channel:         p.Channel,
				Azimuth:         p.Azimuth,
				Elevation:       p.Elevation,
				Distance:        p.Distance,
				Intensity:       p.Intensity,
				Timestamp:       p.Timestamp.UnixNano(),
				BlockID:         p.BlockID,
				UDPSequence:     p.UDPSequence,
				RawBlockAzimuth: p.RawBlockAzimuth,
			}
		}

		if cfg.BackgroundManager == nil {
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
			opsf("[Tracking] Failed to get foreground mask: %v", err)
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
		backgroundPolar := make([]l4perception.PointPolar, 0, cap)
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
		l3grid.StoreForegroundSnapshot(cfg.SensorID, frame.StartTimestamp, foregroundPoints, backgroundPolar, totalPoints, len(foregroundPoints))

		if len(foregroundPoints) == 0 {
			// No foreground detected, skip tracking
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
					diagf("[Pipeline] Throttled %d frames (max %.0f fps)", count, cfg.MaxFrameRate)
				}
				// Only advance miss counters when there is a genuine
				// wall-clock gap (> 2× frame interval), not during short
				// throttle bursts from PCAP catch-up. This prevents
				// premature track deletion: tentative tracks have
				// max_misses=3, so a rapid burst of throttled frames
				// would kill them in ~300 ms otherwise.
				if cfg.Tracker != nil && now.Sub(lastProcessedTime) >= 2*minFrameInterval {
					cfg.Tracker.AdvanceMisses(frame.StartTimestamp)
					diagf("[Pipeline] AdvanceMisses: wall-clock gap %v during throttle, tracks penalised", now.Sub(lastProcessedTime))
				}
				return
			}
			lastProcessedTime = now
		}

		// Forward foreground points on 2370-style stream if configured
		// Use isNilInterface to handle Go interface nil pitfall
		if !isNilInterface(cfg.FgForwarder) {
			pointsToForward := foregroundPoints
			// If debug range is configured, only forward points within that range
			// This allows isolating specific regions for debugging without flooding the stream
			params := cfg.BackgroundManager.GetParams()
			if params.HasDebugRange() {
				filtered := make([]l4perception.PointPolar, 0, len(foregroundPoints))
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
			diagf("[Tracking] FgForwarder is nil, skipping foreground forwarding")
		}

		// Always log foreground extraction for tracking debugging
		tracef("[Tracking] Extracted %d foreground points from %d total", len(foregroundPoints), len(polar))

		// Stage 2: Transform to world coordinates
		worldPoints := l4perception.TransformToWorld(foregroundPoints, nil, cfg.SensorID)

		// Stage 2b: Ground removal (vertical filtering)
		// Remove ground plane and overhead structure returns to reduce false clusters.
		// Bounds are in sensor frame (identity pose): Z=0 is the sensor's horizontal
		// plane, ground is at approximately −3.0 m for a ~3 m mount height.
		filteredPoints := worldPoints
		if cfg.RemoveGround {
			var groundFilter *l4perception.HeightBandFilter
			if cfg.HeightBandFloor != 0 || cfg.HeightBandCeiling != 0 {
				groundFilter = l4perception.NewHeightBandFilter(cfg.HeightBandFloor, cfg.HeightBandCeiling)
			} else {
				groundFilter = l4perception.DefaultHeightBandFilter()
			}
			filteredPoints = groundFilter.FilterVertical(worldPoints)
			proc, kept, below, above := groundFilter.Stats()
			tracef("[Tracking] Ground filter: %d processed, %d kept, %d below floor, %d above ceiling",
				proc, kept, below, above)
		} else {
			diagf("[Tracking] Ground removal disabled, passing %d points through", len(worldPoints))
		}

		if len(filteredPoints) == 0 {
			return
		}

		// Stage 2c: Voxel grid downsampling (optional).
		// Reduces point density while preserving spatial structure, which
		// tightens cluster boundaries and speeds up DBSCAN.
		if cfg.VoxelLeafSize > 0 {
			before := len(filteredPoints)
			filteredPoints = l4perception.VoxelGrid(filteredPoints, cfg.VoxelLeafSize)
			tracef("[Tracking] Voxel downsample: %d → %d (leaf=%.3fm)",
				before, len(filteredPoints), cfg.VoxelLeafSize)
		}

		// Stage 3: Clustering (runtime-tunable via background params)
		dbscanParams := l4perception.DefaultDBSCANParams()
		params := cfg.BackgroundManager.GetParams()
		if params.ForegroundMinClusterPoints > 0 {
			dbscanParams.MinPts = params.ForegroundMinClusterPoints
		}
		if params.ForegroundDBSCANEps > 0 {
			dbscanParams.Eps = float64(params.ForegroundDBSCANEps)
		}
		// Cap input points to bound worst-case DBSCAN runtime on
		// unexpectedly dense frames (e.g. 12k+ foreground points).
		dbscanParams.MaxInputPoints = 8000

		clusters := l4perception.DBSCAN(filteredPoints, dbscanParams)
		if len(clusters) == 0 {
			// No clusters, but still record foreground stats (all points are noise)
			if cfg.Tracker != nil {
				cfg.Tracker.RecordFrameStats(len(filteredPoints), 0)
			}
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
			runManager.RecordFrame()
			runManager.RecordClusters(len(clusters))
		}

		// Always log clustering for tracking debugging
		tracef("[Tracking] Clustered into %d objects", len(clusters))

		// Stage 4: Track update
		if cfg.Tracker == nil {
			return
		}

		cfg.Tracker.Update(clusters, frame.StartTimestamp)

		// Stage 5: Classify and persist confirmed tracks
		confirmedTracks := cfg.Tracker.GetConfirmedTracks()
		tracef("[Tracking] %d confirmed tracks to persist", len(confirmedTracks))

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
			if cfg.DB != nil {
				worldFrame := fmt.Sprintf("site/%s", cfg.SensorID)
				if err := sqlite.InsertTrack(cfg.DB, track, worldFrame); err != nil {
					opsf("[Tracking] Failed to insert track %s: %v", track.TrackID, err)
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
						WorldFrame:        worldFrame,
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
					if err := sqlite.InsertTrackObservation(cfg.DB, obs); err != nil {
						opsf("[Tracking] Failed to insert observation for track %s: %v", track.TrackID, err)
					}
				}
			}
		}

		if len(confirmedTracks) > 0 {
			diagf("[Tracking] %d confirmed tracks active", len(confirmedTracks))
		}

		// Stage 6: Publish to visualiser (if enabled)
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

		// Stage 7: Periodic DB pruning of deleted tracks.
		// Runs at most once per pruneInterval to avoid contention.
		if cfg.DB != nil {
			now := time.Now()
			if lastPruneTime.IsZero() || now.Sub(lastPruneTime) >= pruneInterval {
				lastPruneTime = now
				if pruned, err := sqlite.PruneDeletedTracks(cfg.DB, cfg.SensorID, deletedTrackTTL); err != nil {
					opsf("[Tracking] Prune deleted tracks failed: %v", err)
				} else if pruned > 0 {
					diagf("[Tracking] Pruned %d deleted tracks older than %v", pruned, deletedTrackTTL)
				}
			}
		}
	}
}
