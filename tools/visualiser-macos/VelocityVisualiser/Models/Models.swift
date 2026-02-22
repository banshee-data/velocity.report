// Models.swift
// Swift data models that mirror the protobuf schema.
//
// These models are used before protobuf code generation is available.
// Once generated, these can be replaced with the generated types.

import Foundation

// MARK: - Frame Bundle

/// Frame type enumeration for split streaming (M3.5).
enum FrameType: Int {
    case full = 0  // Legacy: all points
    case foreground = 1  // Foreground + clusters + tracks only
    case background = 2  // Background snapshot
    case delta = 3  // Future: incremental update
}

/// Grid metadata for background snapshot.
struct GridMetadata {
    var rings: Int32 = 40
    var azimuthBins: Int32 = 1800
    var ringElevations: [Float] = []
    var settlingComplete: Bool = false
}

/// Background snapshot for cached rendering (M3.5).
/// This is streamed once and cached client-side to reduce bandwidth.
struct BackgroundSnapshot {
    var sequenceNumber: UInt64 = 0
    var timestampNanos: Int64 = 0
    var x: [Float] = []
    var y: [Float] = []
    var z: [Float] = []
    var confidence: [UInt32] = []  // TimesSeenCount per point
    var gridMetadata: GridMetadata?

    var pointCount: Int { x.count }
}

struct FrameBundle {
    var frameID: UInt64 = 0
    var timestampNanos: Int64 = 0
    var sensorID: String = ""
    var coordinateFrame: CoordinateFrameInfo?

    var pointCloud: PointCloudFrame?
    var clusters: ClusterSet?
    var tracks: TrackSet?
    var debug: DebugOverlaySet?
    var playbackInfo: PlaybackInfo?

    // M3.5 Split Streaming fields
    var frameType: FrameType = .full
    var background: BackgroundSnapshot?
    var backgroundSeq: UInt64 = 0
}

struct CoordinateFrameInfo {
    var frameID: String = ""
    var referenceFrame: String = ""
    var originLat: Double = 0
    var originLon: Double = 0
    var originAlt: Double = 0
    var rotationDeg: Float = 0
}

// MARK: - Point Cloud

enum DecimationMode: Int {
    case none = 0
    case uniform = 1
    case voxel = 2
    case foregroundOnly = 3
}

struct PointCloudFrame {
    var frameID: UInt64 = 0
    var timestampNanos: Int64 = 0
    var sensorID: String = ""

    var x: [Float] = []
    var y: [Float] = []
    var z: [Float] = []
    var intensity: [UInt8] = []
    var classification: [UInt8] = []

    var decimationMode: DecimationMode = .none
    var decimationRatio: Float = 1.0
    var pointCount: Int = 0
}

// MARK: - Clusters

/// OrientedBoundingBox represents a 7-DOF (7 Degrees of Freedom) 3D bounding box.
/// This format conforms to the AV industry standard specification.
/// See: docs/lidar/future/av-lidar-integration-plan.md for BoundingBox7DOF spec.
///
/// 7-DOF parameters:
/// - centerX/Y/Z: Centre position (metres, world frame)
/// - length: Box extent along heading direction (metres)
/// - width: Box extent perpendicular to heading (metres)
/// - height: Box extent along Z-axis (metres)
/// - headingRad: Yaw angle around Z-axis (radians, [-π, π])
struct OrientedBoundingBox {
    var centerX: Float = 0  // metres, world frame
    var centerY: Float = 0  // metres, world frame
    var centerZ: Float = 0  // metres, world frame
    var length: Float = 0  // metres, along heading direction
    var width: Float = 0  // metres, perpendicular to heading
    var height: Float = 0  // metres, Z extent
    var headingRad: Float = 0  // radians, rotation around Z-axis, [-π, π]
}

struct Cluster {
    var clusterID: Int64 = 0
    var sensorID: String = ""
    var timestampNanos: Int64 = 0

    var centroidX: Float = 0
    var centroidY: Float = 0
    var centroidZ: Float = 0

    var aabbLength: Float = 0
    var aabbWidth: Float = 0
    var aabbHeight: Float = 0

    var obb: OrientedBoundingBox?

    var pointsCount: Int = 0
    var heightP95: Float = 0
    var intensityMean: Float = 0

    var samplePoints: [Float] = []
}

enum ClusteringMethod: Int {
    case dbscan = 0
    case connectedComponents = 1
}

struct ClusterSet {
    var frameID: UInt64 = 0
    var timestampNanos: Int64 = 0
    var clusters: [Cluster] = []
    var method: ClusteringMethod = .dbscan
}

// MARK: - Tracks

enum TrackState: Int {
    case unknown = 0
    case tentative = 1
    case confirmed = 2
    case deleted = 3

    var colour: (r: Float, g: Float, b: Float) {
        switch self {
        case .unknown: return (0.5, 0.5, 0.5)
        case .tentative: return (1.0, 1.0, 0.0)  // yellow
        case .confirmed: return (0.0, 1.0, 0.0)  // green
        case .deleted: return (1.0, 0.0, 0.0)  // red
        }
    }
}

enum OcclusionState: Int {
    case none = 0
    case partial = 1
    case full = 2
}

enum MotionModel: Int {
    case cv = 0  // constant velocity
    case ca = 1  // constant acceleration
}

/// Source of the current heading estimate (for debug rendering).
/// When debug heading-source colouring is enabled, confirmed-track boxes
/// are tinted by heading origin instead of the default state colour.
enum HeadingSource: Int {
    case pca = 0           // raw PCA heading (no disambiguation)
    case velocity = 1      // disambiguated using Kalman velocity
    case displacement = 2  // disambiguated using position displacement
    case locked = 3        // heading locked (aspect ratio guard or jump rejection)

    /// Debug colour for heading-source overlay rendering.
    var colour: (r: Float, g: Float, b: Float) {
        switch self {
        case .pca: return (1.0, 1.0, 0.0)           // yellow — raw PCA
        case .velocity: return (0.3, 0.5, 1.0)      // blue — velocity-disambiguated
        case .displacement: return (1.0, 0.5, 0.0)  // orange — displacement-disambiguated
        case .locked: return (0.7, 0.7, 0.7)         // grey — heading locked
        }
    }
}

struct Track {
    var trackID: String = ""
    var sensorID: String = ""

    var state: TrackState = .unknown
    var hits: Int = 0
    var misses: Int = 0
    var observationCount: Int = 0

    var firstSeenNanos: Int64 = 0
    var lastSeenNanos: Int64 = 0

    var x: Float = 0
    var y: Float = 0
    var z: Float = 0

    var vx: Float = 0
    var vy: Float = 0
    var vz: Float = 0

    var speedMps: Float = 0
    var headingRad: Float = 0

    var covariance4x4: [Float] = []

    var bboxLengthAvg: Float = 0
    var bboxWidthAvg: Float = 0
    var bboxHeightAvg: Float = 0
    var bboxHeadingRad: Float = 0

    var heightP95Max: Float = 0
    var intensityMeanAvg: Float = 0
    var avgSpeedMps: Float = 0
    var peakSpeedMps: Float = 0

    var classLabel: String = ""
    var classConfidence: Float = 0

    var trackLengthMetres: Float = 0
    var trackDurationSecs: Float = 0
    var occlusionCount: Int = 0
    var confidence: Float = 0
    var occlusionState: OcclusionState = .none
    var motionModel: MotionModel = .cv

    // Rendering hints
    var alpha: Float = 1.0  // Opacity [0,1]; 1.0 = fully visible, used for fade-out
    var headingSource: HeadingSource = .pca  // Source of heading for debug rendering
}

struct TrackPoint {
    var x: Float = 0
    var y: Float = 0
    var timestampNanos: Int64 = 0
}

struct TrackTrail {
    var trackID: String = ""
    var points: [TrackPoint] = []
}

struct TrackSet {
    var frameID: UInt64 = 0
    var timestampNanos: Int64 = 0
    var tracks: [Track] = []
    var trails: [TrackTrail] = []
}

// MARK: - Debug Overlays

struct AssociationCandidate {
    var clusterID: Int64 = 0
    var trackID: String = ""
    var distance: Float = 0
    var accepted: Bool = false
}

struct GatingEllipse {
    var trackID: String = ""
    var centerX: Float = 0
    var centerY: Float = 0
    var semiMajor: Float = 0
    var semiMinor: Float = 0
    var rotationRad: Float = 0
}

struct InnovationResidual {
    var trackID: String = ""
    var predictedX: Float = 0
    var predictedY: Float = 0
    var measuredX: Float = 0
    var measuredY: Float = 0
    var residualMagnitude: Float = 0
}

struct StatePrediction {
    var trackID: String = ""
    var x: Float = 0
    var y: Float = 0
    var vx: Float = 0
    var vy: Float = 0
}

struct DebugOverlaySet {
    var frameID: UInt64 = 0
    var timestampNanos: Int64 = 0

    var associationCandidates: [AssociationCandidate] = []
    var gatingEllipses: [GatingEllipse] = []
    var residuals: [InnovationResidual] = []
    var predictions: [StatePrediction] = []
}

// MARK: - Playback

struct PlaybackInfo {
    var isLive: Bool = true
    var logStartNs: Int64 = 0
    var logEndNs: Int64 = 0
    var playbackRate: Float = 1.0
    var paused: Bool = false
    var currentFrameIndex: UInt64 = 0  // 0-based index in log
    var totalFrames: UInt64 = 0
    var seekable: Bool = false  // true when seek/step is supported (e.g. .vrlog replay)
}

// MARK: - Labels

struct LabelEvent: Identifiable, Codable {
    var id: String = UUID().uuidString
    var trackID: String = ""
    var classLabel: String = ""
    var startTimestampNs: Int64 = 0
    var endTimestampNs: Int64? = nil
    var confidence: Float? = nil
    var createdBy: String? = nil
    var createdAtNs: Int64 = 0
    var updatedAtNs: Int64? = nil
    var notes: String? = nil
    var sceneID: String? = nil
    var sourceFile: String? = nil

    enum CodingKeys: String, CodingKey {
        case id = "label_id"
        case trackID = "track_id"
        case classLabel = "class_label"
        case startTimestampNs = "start_timestamp_ns"
        case endTimestampNs = "end_timestamp_ns"
        case confidence
        case createdBy = "created_by"
        case createdAtNs = "created_at_ns"
        case updatedAtNs = "updated_at_ns"
        case notes
        case sceneID = "scene_id"
        case sourceFile = "source_file"
    }
}

struct LabelSet: Codable {
    var sessionID: String = ""
    var sourceFile: String = ""
    var labels: [LabelEvent] = []
}
