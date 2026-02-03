// Models.swift
// Swift data models that mirror the protobuf schema.
//
// These models are used before protobuf code generation is available.
// Once generated, these can be replaced with the generated types.

import Foundation

// MARK: - Frame Bundle

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
    var centerX: Float = 0      // metres, world frame
    var centerY: Float = 0      // metres, world frame
    var centerZ: Float = 0      // metres, world frame
    var length: Float = 0       // metres, along heading direction
    var width: Float = 0        // metres, perpendicular to heading
    var height: Float = 0       // metres, Z extent
    var headingRad: Float = 0   // radians, rotation around Z-axis, [-π, π]
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
        case .tentative: return (1.0, 1.0, 0.0) // yellow
        case .confirmed: return (0.0, 1.0, 0.0) // green
        case .deleted: return (1.0, 0.0, 0.0) // red
        }
    }
}

enum OcclusionState: Int {
    case none = 0
    case partial = 1
    case full = 2
}

enum MotionModel: Int {
    case cv = 0 // constant velocity
    case ca = 1 // constant acceleration
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
}

// MARK: - Labels

struct LabelEvent: Identifiable, Codable {
    var id: String = UUID().uuidString
    var trackID: String = ""
    var classLabel: String = ""
    var startFrameID: UInt64 = 0
    var endFrameID: UInt64 = 0
    var createdNanos: Int64 = 0
    var annotator: String = ""
    var notes: String = ""
}

struct LabelSet: Codable {
    var sessionID: String = ""
    var sourceFile: String = ""
    var labels: [LabelEvent] = []
}
