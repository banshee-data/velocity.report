package lidar

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"reflect"
)

// ForegroundForwarder interface allows forwarding foreground points without importing network package.
type ForegroundForwarder interface {
	ForwardForeground(points []PointPolar)
}

// VisualiserPublisher interface allows publishing frames to the gRPC visualiser.
type VisualiserPublisher interface {
	Publish(frame interface{})
}

// VisualiserAdapter interface converts tracking outputs to FrameBundle.
type VisualiserAdapter interface {
	AdaptFrame(frame *LiDARFrame, foregroundMask []bool, clusters []WorldCluster, tracker TrackerInterface) interface{}
}

// LidarViewAdapter interface forwards FrameBundle to UDP (LidarView format).
type LidarViewAdapter interface {
	PublishFrameBundle(bundle interface{}, foregroundPoints []PointPolar)
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

// TrackingPipelineConfig holds dependencies for the tracking pipeline callback.
type TrackingPipelineConfig struct {
	BackgroundManager   *BackgroundManager
	FgForwarder         ForegroundForwarder // Use interface to avoid import cycle
	Tracker             TrackerInterface    // Use interface for dependency injection and testing
	Classifier          *TrackClassifier
	DB                  *sql.DB // Use standard sql.DB to avoid import cycle with db package
	SensorID            string
	DebugMode           bool
	AnalysisRunManager  *AnalysisRunManager // Optional: for recording analysis runs
	VisualiserPublisher VisualiserPublisher // Optional: gRPC publisher
	VisualiserAdapter   VisualiserAdapter   // Optional: adapter for gRPC
	LidarViewAdapter    LidarViewAdapter    // Optional: adapter for UDP forwarding
}

// NewFrameCallback creates a FrameBuilder callback that processes frames through
// the full tracking pipeline: foreground extraction, clustering, tracking, and persistence.
func (cfg *TrackingPipelineConfig) NewFrameCallback() func(*LiDARFrame) {
	// Get AnalysisRunManager from registry if not explicitly set
	// This allows analysis runs to be started/stopped dynamically via webserver
	getRunManager := func() *AnalysisRunManager {
		if cfg.AnalysisRunManager != nil {
			return cfg.AnalysisRunManager
		}
		return GetAnalysisRunManager(cfg.SensorID)
	}

	return func(frame *LiDARFrame) {
		if frame == nil || len(frame.Points) == 0 {
			return
		}

		// Route frame completion to debug log to keep main log quiet during normal runs.
		Debugf("[FrameBuilder] Completed frame: %s, Points: %d, Azimuth: %.1f°-%.1f°",
			frame.FrameID, len(frame.Points), frame.MinAzimuth, frame.MaxAzimuth)

		// Convert frame points to polar coordinates
		polar := make([]PointPolar, 0, len(frame.Points))
		for _, p := range frame.Points {
			polar = append(polar, PointPolar{
				Channel:         p.Channel,
				Azimuth:         p.Azimuth,
				Elevation:       p.Elevation,
				Distance:        p.Distance,
				Intensity:       p.Intensity,
				Timestamp:       p.Timestamp.UnixNano(),
				BlockID:         p.BlockID,
				UDPSequence:     p.UDPSequence,
				RawBlockAzimuth: p.RawBlockAzimuth,
			})
		}

		if cfg.BackgroundManager == nil {
			return
		}

		if cfg.DebugMode {
			// Provide extra context at the exact handoff so we can trace delivery
			var firstAz, lastAz float64
			var firstTS, lastTS int64
			if len(polar) > 0 {
				firstAz = polar[0].Azimuth
				lastAz = polar[len(polar)-1].Azimuth
				firstTS = polar[0].Timestamp
				lastTS = polar[len(polar)-1].Timestamp
			}
			log.Printf("[FrameBuilder->Pipeline] Delivering frame %s -> %d points (azimuth: %.1f°->%.1f°, ts: %d->%d)",
				frame.FrameID, len(polar), firstAz, lastAz, firstTS, lastTS)
		}

		// Phase 1: Foreground extraction
		mask, err := cfg.BackgroundManager.ProcessFramePolarWithMask(polar)
		if err != nil || mask == nil {
			if cfg.DebugMode {
				log.Printf("[Tracking] Failed to get foreground mask: %v", err)
			}
			return
		}

		foregroundPoints := ExtractForegroundPoints(polar, mask)
		totalPoints := len(polar)

		// Build background subset for debug overlay (downsample to keep chart light)
		backgroundPolar := make([]PointPolar, 0, totalPoints-len(foregroundPoints))
		for i, isForeground := range mask {
			if !isForeground {
				backgroundPolar = append(backgroundPolar, polar[i])
			}
		}

		const maxBackgroundChartPoints = 5000
		if len(backgroundPolar) > maxBackgroundChartPoints {
			stride := len(backgroundPolar) / maxBackgroundChartPoints
			if stride < 1 {
				stride = 1
			}
			downsampled := make([]PointPolar, 0, maxBackgroundChartPoints)
			for i := 0; i < len(backgroundPolar); i += stride {
				downsampled = append(downsampled, backgroundPolar[i])
				if len(downsampled) >= maxBackgroundChartPoints {
					break
				}
			}
			backgroundPolar = downsampled
		}

		// Cache sensor-frame projections for debug visualization (aligns with polar background chart)
		StoreForegroundSnapshot(cfg.SensorID, frame.StartTimestamp, foregroundPoints, backgroundPolar, totalPoints, len(foregroundPoints))

		if len(foregroundPoints) == 0 {
			// No foreground detected, skip tracking
			return
		}

		// Forward foreground points on 2370-style stream if configured
		// Use isNilInterface to handle Go interface nil pitfall
		if !isNilInterface(cfg.FgForwarder) {
			pointsToForward := foregroundPoints
			// If debug range is configured, only forward points within that range
			// This allows isolating specific regions for debugging without flooding the stream
			params := cfg.BackgroundManager.GetParams()
			if params.HasDebugRange() {
				filtered := make([]PointPolar, 0, len(foregroundPoints))
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
		} else if cfg.DebugMode {
			Debugf("[Tracking] FgForwarder is nil, skipping foreground forwarding")
		}

		// Always log foreground extraction for tracking debugging
		Debugf("[Tracking] Extracted %d foreground points from %d total", len(foregroundPoints), len(polar))

		// Phase 2: Transform to world coordinates
		worldPoints := TransformToWorld(foregroundPoints, nil, cfg.SensorID)

		// Phase 3: Clustering (runtime-tunable via background params)
		dbscanParams := DefaultDBSCANParams()
		params := cfg.BackgroundManager.GetParams()
		if params.ForegroundMinClusterPoints > 0 {
			dbscanParams.MinPts = params.ForegroundMinClusterPoints
		}
		if params.ForegroundDBSCANEps > 0 {
			dbscanParams.Eps = float64(params.ForegroundDBSCANEps)
		}

		clusters := DBSCAN(worldPoints, dbscanParams)
		if len(clusters) == 0 {
			return
		}

		// Record clusters for analysis run if active
		if runManager := getRunManager(); runManager != nil && runManager.IsRunActive() {
			runManager.RecordFrame()
			runManager.RecordClusters(len(clusters))
		}

		// Always log clustering for tracking debugging
		Debugf("[Tracking] Clustered into %d objects", len(clusters))

		// Phase 4: Track update
		if cfg.Tracker == nil {
			return
		}

		cfg.Tracker.Update(clusters, frame.StartTimestamp)

		// Phase 5: Classify and persist confirmed tracks
		confirmedTracks := cfg.Tracker.GetConfirmedTracks()
		Debugf("[Tracking] %d confirmed tracks to persist", len(confirmedTracks))

		for _, track := range confirmedTracks {
			// Classify if not already classified and has enough observations
			if track.ObjectClass == "" && track.ObservationCount >= 5 && cfg.Classifier != nil {
				cfg.Classifier.ClassifyAndUpdate(track)
			}

			// Record track for analysis run if active
			if runManager := getRunManager(); runManager != nil && runManager.IsRunActive() {
				runManager.RecordTrack(track)
			}

			// Persist track to database
			if cfg.DB != nil {
				worldFrame := fmt.Sprintf("site/%s", cfg.SensorID)
				if err := InsertTrack(cfg.DB, track, worldFrame); err != nil {
					if cfg.DebugMode {
						log.Printf("[Tracking] Failed to insert track %s: %v", track.TrackID, err)
					}
				}

				// Insert observation
				obs := &TrackObservation{
					TrackID:           track.TrackID,
					TSUnixNanos:       frame.StartTimestamp.UnixNano(),
					WorldFrame:        worldFrame,
					X:                 track.X,
					Y:                 track.Y,
					Z:                 0, // TrackedObject doesn't have Z
					VelocityX:         track.VX,
					VelocityY:         track.VY,
					SpeedMps:          track.AvgSpeedMps,
					HeadingRad:        float32(math.Atan2(float64(track.VY), float64(track.VX))),
					BoundingBoxLength: track.BoundingBoxLengthAvg,
					BoundingBoxWidth:  track.BoundingBoxWidthAvg,
					BoundingBoxHeight: track.BoundingBoxHeightAvg,
					HeightP95:         track.HeightP95Max,
					IntensityMean:     track.IntensityMeanAvg,
				}
				if err := InsertTrackObservation(cfg.DB, obs); err != nil {
					if cfg.DebugMode {
						log.Printf("[Tracking] Failed to insert observation for track %s: %v", track.TrackID, err)
					}
				}
			}
		}

		if cfg.DebugMode && len(confirmedTracks) > 0 {
			Debugf("[Tracking] %d confirmed tracks active", len(confirmedTracks))
		}

		// Phase 6: Publish to visualiser (if enabled)
		if !isNilInterface(cfg.VisualiserAdapter) && !isNilInterface(cfg.VisualiserPublisher) {
			// Adapt frame to FrameBundle
			frameBundle := cfg.VisualiserAdapter.AdaptFrame(frame, mask, clusters, cfg.Tracker)

			// Publish to gRPC stream
			cfg.VisualiserPublisher.Publish(frameBundle)

			// Also forward to LidarView UDP if adapter is configured
			if !isNilInterface(cfg.LidarViewAdapter) {
				cfg.LidarViewAdapter.PublishFrameBundle(frameBundle, foregroundPoints)
			}

			Debugf("[Visualiser] Published frame %s to gRPC", frame.FrameID)
		} else if !isNilInterface(cfg.LidarViewAdapter) {
			// LidarView-only mode (no gRPC)
			// Create a minimal bundle just for LidarView forwarding
			// This preserves the existing behavior when gRPC is disabled
			cfg.LidarViewAdapter.PublishFrameBundle(nil, foregroundPoints)
		}
	}
}
