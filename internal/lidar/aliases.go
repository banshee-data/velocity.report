// Package lidar provides backward-compatible type aliases for the layer-aligned
// sub-packages. These aliases allow the integration and cross-layer tests in
// this package to reference types without fully-qualified imports.
//
// New code should import from the canonical layer packages directly:
//
//	l2frames, l3grid, l4perception, l5tracks, l6objects, pipeline, storage/sqlite, adapters
package lidar

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// ── L2 Frames ────────────────────────────────────────────────────────

type FrameBuilder = l2frames.FrameBuilder
type LiDARFrame = l2frames.LiDARFrame

// ── L3 Grid ──────────────────────────────────────────────────────────

type BackgroundGrid = l3grid.BackgroundGrid
type BackgroundManager = l3grid.BackgroundManager
type BackgroundParams = l3grid.BackgroundParams
type BgSnapshot = l3grid.BgSnapshot
type BgStore = l3grid.BgStore
type FrameID = l3grid.FrameID
type RegionData = l3grid.RegionData
type RegionSnapshot = l3grid.RegionSnapshot

var NewBackgroundManager = l3grid.NewBackgroundManager
var StoreForegroundSnapshot = l3grid.StoreForegroundSnapshot

// ── L4 Perception ────────────────────────────────────────────────────

type Point = l4perception.Point
type PointPolar = l4perception.PointPolar
type WorldCluster = l4perception.WorldCluster
type WorldPoint = l4perception.WorldPoint

var DBSCAN = l4perception.DBSCAN
var NewDefaultDBSCANClusterer = l4perception.NewDefaultDBSCANClusterer

// ── L5 Tracks ────────────────────────────────────────────────────────

type TrackedObject = l5tracks.TrackedObject
type Tracker = l5tracks.Tracker
type TrackerConfig = l5tracks.TrackerConfig
type TrackerInterface = l5tracks.TrackerInterface
type TrackingMetrics = l5tracks.TrackingMetrics

var DefaultTrackerConfig = l5tracks.DefaultTrackerConfig
var NewTracker = l5tracks.NewTracker

// ── L6 Objects ───────────────────────────────────────────────────────

type ObjectClass = l6objects.ObjectClass
type TrackFeatures = l6objects.TrackFeatures

var NewTrackClassifier = l6objects.NewTrackClassifier

// ── Pipeline ─────────────────────────────────────────────────────────

type ForegroundForwarder = pipeline.ForegroundForwarder
type LidarViewAdapter = pipeline.LidarViewAdapter
type TrackingPipelineConfig = pipeline.TrackingPipelineConfig
type VisualiserAdapter = pipeline.VisualiserAdapter
type VisualiserPublisher = pipeline.VisualiserPublisher

// ── Storage ──────────────────────────────────────────────────────────

type AnalysisRunManager = sqlite.AnalysisRunManager

var DefaultRunParams = sqlite.DefaultRunParams
var GetActiveTracks = sqlite.GetActiveTracks
var InsertTrack = sqlite.InsertTrack
var InsertTrackObservation = sqlite.InsertTrackObservation
var NewAnalysisRunManager = sqlite.NewAnalysisRunManager
var RegisterAnalysisRunManager = sqlite.RegisterAnalysisRunManager
