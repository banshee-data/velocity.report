//
//  ModelExtendedTests.swift
//  VelocityVisualiserTests
//
//  Extended unit tests for Models with full property coverage.
//

import Foundation
import Testing

@testable import VelocityVisualiser

// MARK: - Track Extended Tests

struct TrackExtendedTests {
    @Test func trackDefaultValues() throws {
        let track = Track()
        #expect(track.sensorID == "")
        #expect(track.hits == 0)
        #expect(track.misses == 0)
        #expect(track.observationCount == 0)
        #expect(track.firstSeenNanos == 0)
        #expect(track.lastSeenNanos == 0)
        #expect(track.vx == 0)
        #expect(track.vy == 0)
        #expect(track.vz == 0)
        #expect(track.headingRad == 0)
        #expect(track.covariance4x4.isEmpty)
    }

    @Test func trackBoundingBoxProperties() throws {
        var track = Track()
        track.bboxLengthAvg = 4.5
        track.bboxWidthAvg = 1.8
        track.bboxHeightAvg = 1.6
        track.bboxHeadingRad = Float.pi / 2

        #expect(track.bboxLengthAvg == 4.5)
        #expect(track.bboxWidthAvg == 1.8)
        #expect(track.bboxHeightAvg == 1.6)
        #expect(abs(track.bboxHeadingRad - Float.pi / 2) < 0.001)
    }

    @Test func trackStatisticsProperties() throws {
        var track = Track()
        track.heightP95Max = 1.8
        track.intensityMeanAvg = 120.0
        track.avgSpeedMps = 8.5
        track.peakSpeedMps = 15.0
        track.trackLengthMetres = 125.0
        track.trackDurationSecs = 30.0
        track.occlusionCount = 5
        track.confidence = 0.92

        #expect(track.heightP95Max == 1.8)
        #expect(track.intensityMeanAvg == 120.0)
        #expect(track.avgSpeedMps == 8.5)
        #expect(track.peakSpeedMps == 15.0)
        #expect(track.trackLengthMetres == 125.0)
        #expect(track.trackDurationSecs == 30.0)
        #expect(track.occlusionCount == 5)
        #expect(track.confidence == 0.92)
    }

    @Test func trackClassification() throws {
        var track = Track()
        track.classLabel = "car"
        track.classConfidence = 0.95

        #expect(track.classLabel == "car")
        #expect(track.classConfidence == 0.95)
    }

    @Test func trackOcclusionState() throws {
        var track = Track()

        track.occlusionState = .none
        #expect(track.occlusionState == .none)

        track.occlusionState = .partial
        #expect(track.occlusionState == .partial)

        track.occlusionState = .full
        #expect(track.occlusionState == .full)
    }

    @Test func trackMotionModel() throws {
        var track = Track()

        track.motionModel = .cv
        #expect(track.motionModel == .cv)

        track.motionModel = .ca
        #expect(track.motionModel == .ca)
    }

    @Test func trackFirstLastSeen() throws {
        var track = Track()
        track.firstSeenNanos = 1_000_000_000
        track.lastSeenNanos = 5_000_000_000

        #expect(track.firstSeenNanos == 1_000_000_000)
        #expect(track.lastSeenNanos == 5_000_000_000)
        #expect(track.lastSeenNanos - track.firstSeenNanos == 4_000_000_000)
    }

    @Test func trackCovariance() throws {
        var track = Track()
        track.covariance4x4 = [
            1.0, 0.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 0.0, 1.0,
        ]

        #expect(track.covariance4x4.count == 16)
        #expect(track.covariance4x4[0] == 1.0)
        #expect(track.covariance4x4[5] == 1.0)
    }
}

// MARK: - Cluster Extended Tests

struct ClusterExtendedTests {
    @Test func clusterDefaultValues() throws {
        let cluster = Cluster()
        #expect(cluster.timestampNanos == 0)
        #expect(cluster.pointsCount == 0)
        #expect(cluster.heightP95 == 0)
        #expect(cluster.intensityMean == 0)
        #expect(cluster.samplePoints.isEmpty)
    }

    @Test func clusterPointProperties() throws {
        var cluster = Cluster()
        cluster.pointsCount = 150
        cluster.heightP95 = 1.6
        cluster.intensityMean = 128.5
        cluster.samplePoints = [1.0, 2.0, 3.0, 4.0, 5.0, 6.0]  // 2 points (x,y,z each)

        #expect(cluster.pointsCount == 150)
        #expect(cluster.heightP95 == 1.6)
        #expect(cluster.intensityMean == 128.5)
        #expect(cluster.samplePoints.count == 6)
    }

    @Test func clusterTimestamp() throws {
        var cluster = Cluster()
        cluster.timestampNanos = 1_234_567_890_000

        #expect(cluster.timestampNanos == 1_234_567_890_000)
    }

    @Test func clusterSensorID() throws {
        var cluster = Cluster()
        cluster.sensorID = "hesai-02"

        #expect(cluster.sensorID == "hesai-02")
    }
}

// MARK: - PointCloudFrame Extended Tests

struct PointCloudFrameExtendedTests {
    @Test func largePointCloud() throws {
        var pc = PointCloudFrame()
        pc.frameID = 100
        pc.pointCount = 65000

        // Simulate arrays of 65k points
        pc.x = Array(repeating: Float.random(in: -100...100), count: 65000)
        pc.y = Array(repeating: Float.random(in: -100...100), count: 65000)
        pc.z = Array(repeating: Float.random(in: 0...10), count: 65000)
        pc.intensity = Array(repeating: UInt8.random(in: 0...255), count: 65000)
        pc.classification = Array(repeating: UInt8(0), count: 65000)

        #expect(pc.x.count == 65000)
        #expect(pc.y.count == 65000)
        #expect(pc.z.count == 65000)
        #expect(pc.intensity.count == 65000)
        #expect(pc.classification.count == 65000)
    }

    @Test func decimationUniform() throws {
        var pc = PointCloudFrame()
        pc.decimationMode = .uniform
        pc.decimationRatio = 0.5

        #expect(pc.decimationMode == .uniform)
        #expect(pc.decimationRatio == 0.5)
    }

    @Test func decimationVoxel() throws {
        var pc = PointCloudFrame()
        pc.decimationMode = .voxel
        pc.decimationRatio = 0.25

        #expect(pc.decimationMode == .voxel)
        #expect(pc.decimationRatio == 0.25)
    }

    @Test func decimationForegroundOnly() throws {
        var pc = PointCloudFrame()
        pc.decimationMode = .foregroundOnly
        pc.decimationRatio = 1.0

        #expect(pc.decimationMode == .foregroundOnly)
        #expect(pc.decimationRatio == 1.0)
    }

    @Test func pointCloudTimestamp() throws {
        var pc = PointCloudFrame()
        pc.timestampNanos = 9_876_543_210

        #expect(pc.timestampNanos == 9_876_543_210)
    }
}

// MARK: - TrackPoint Extended Tests

struct TrackPointExtendedTests {
    @Test func trackPointInitialisation() throws {
        let point = TrackPoint(x: 15.5, y: 25.5, timestampNanos: 1_000_000_000)

        #expect(point.x == 15.5)
        #expect(point.y == 25.5)
        #expect(point.timestampNanos == 1_000_000_000)
    }

    @Test func trackPointDefault() throws {
        let point = TrackPoint()
        #expect(point.x == 0)
        #expect(point.y == 0)
        #expect(point.timestampNanos == 0)
    }
}

// MARK: - OrientedBoundingBox Extended Tests

struct OrientedBoundingBoxExtendedTests {
    @Test func obbAllProperties() throws {
        let obb = OrientedBoundingBox(
            centerX: 50.0, centerY: 75.0, centerZ: 1.2, length: 5.0, width: 2.0, height: 1.8,
            headingRad: Float.pi)

        #expect(obb.centerX == 50.0)
        #expect(obb.centerY == 75.0)
        #expect(obb.centerZ == 1.2)
        #expect(obb.length == 5.0)
        #expect(obb.width == 2.0)
        #expect(obb.height == 1.8)
        #expect(abs(obb.headingRad - Float.pi) < 0.001)
    }

    @Test func obbNegativeHeading() throws {
        var obb = OrientedBoundingBox()
        obb.headingRad = -Float.pi / 2  // -90 degrees

        #expect(obb.headingRad < 0)
        #expect(abs(obb.headingRad + Float.pi / 2) < 0.001)
    }
}

// MARK: - CoordinateFrameInfo Extended Tests

struct CoordinateFrameInfoExtendedTests {
    @Test func coordinateFrameAllProperties() throws {
        let coord = CoordinateFrameInfo(
            frameID: "world/lidar-01", referenceFrame: "WGS84", originLat: -33.8688,  // Sydney
            originLon: 151.2093, originAlt: 58.0, rotationDeg: 180.0)

        #expect(coord.frameID == "world/lidar-01")
        #expect(coord.referenceFrame == "WGS84")
        #expect(coord.originLat == -33.8688)
        #expect(coord.originLon == 151.2093)
        #expect(coord.originAlt == 58.0)
        #expect(coord.rotationDeg == 180.0)
    }

    @Test func coordinateFrameNegativeCoordinates() throws {
        var coord = CoordinateFrameInfo()
        coord.originLat = -45.0  // Southern hemisphere
        coord.originLon = -122.0  // Western hemisphere

        #expect(coord.originLat == -45.0)
        #expect(coord.originLon == -122.0)
    }
}

// MARK: - Debug Overlay Extended Tests

struct DebugOverlayExtendedTests {
    @Test func associationCandidateRejected() throws {
        let candidate = AssociationCandidate(
            clusterID: 5, trackID: "track-003", distance: 15.0,  // Too far
            accepted: false)

        #expect(candidate.clusterID == 5)
        #expect(candidate.trackID == "track-003")
        #expect(candidate.distance == 15.0)
        #expect(candidate.accepted == false)
    }

    @Test func gatingEllipseWithRotation() throws {
        let ellipse = GatingEllipse(
            trackID: "track-010", centerX: 100.0, centerY: 200.0, semiMajor: 10.0, semiMinor: 5.0,
            rotationRad: Float.pi / 3  // 60 degrees
        )

        #expect(ellipse.trackID == "track-010")
        #expect(ellipse.semiMajor == 10.0)
        #expect(ellipse.semiMinor == 5.0)
        #expect(abs(ellipse.rotationRad - Float.pi / 3) < 0.001)
    }

    @Test func innovationResidualLarge() throws {
        let residual = InnovationResidual(
            trackID: "track-outlier", predictedX: 50.0, predictedY: 50.0, measuredX: 55.0,
            measuredY: 55.0, residualMagnitude: 7.07  // sqrt(50)
        )

        #expect(residual.predictedX == 50.0)
        #expect(residual.measuredX == 55.0)
        #expect(residual.residualMagnitude > 7.0)
    }

    @Test func statePredictionWithVelocity() throws {
        let pred = StatePrediction(
            trackID: "track-fast", x: 100.0, y: 200.0, vx: 15.0,  // 15 m/s ~ 54 km/h
            vy: 0.0)

        #expect(pred.x == 100.0)
        #expect(pred.y == 200.0)
        #expect(pred.vx == 15.0)
        #expect(pred.vy == 0.0)
    }

    @Test func debugOverlayMultipleItems() throws {
        var overlay = DebugOverlaySet()
        overlay.frameID = 500
        overlay.timestampNanos = 5_000_000_000

        // Add multiple items of each type
        overlay.associationCandidates = [
            AssociationCandidate(clusterID: 1, trackID: "t1", distance: 1.0, accepted: true),
            AssociationCandidate(clusterID: 2, trackID: "t2", distance: 2.0, accepted: true),
            AssociationCandidate(clusterID: 3, trackID: "t1", distance: 5.0, accepted: false),
        ]
        overlay.gatingEllipses = [
            GatingEllipse(
                trackID: "t1", centerX: 10, centerY: 20, semiMajor: 3, semiMinor: 2, rotationRad: 0),
            GatingEllipse(
                trackID: "t2", centerX: 30, centerY: 40, semiMajor: 4, semiMinor: 3, rotationRad: 0),
        ]

        #expect(overlay.associationCandidates.count == 3)
        #expect(overlay.gatingEllipses.count == 2)
        #expect(overlay.associationCandidates.filter { $0.accepted }.count == 2)
    }
}

// MARK: - PlaybackInfo Extended Tests

struct PlaybackInfoExtendedTests {
    @Test func playbackInfoLongRecording() throws {
        var info = PlaybackInfo()
        info.isLive = false
        info.logStartNs = 0
        info.logEndNs = 3600_000_000_000  // 1 hour
        info.totalFrames = 360_000  // 10 Hz for 1 hour
        info.currentFrameIndex = 180_000  // Halfway

        #expect(info.totalFrames == 360_000)
        #expect(info.currentFrameIndex == 180_000)
        #expect(info.logEndNs == 3600_000_000_000)
    }

    @Test func playbackInfoHighRate() throws {
        var info = PlaybackInfo()
        info.playbackRate = 64.0  // Max rate

        #expect(info.playbackRate == 64.0)
    }

    @Test func playbackInfoSlowRate() throws {
        var info = PlaybackInfo()
        info.playbackRate = 0.5  // Half speed

        #expect(info.playbackRate == 0.5)
    }
}
