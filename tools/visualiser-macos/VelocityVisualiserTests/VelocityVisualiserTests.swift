//
//  VelocityVisualiserTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for the Velocity Visualiser macOS application.
//

import Foundation
import Testing

@testable import VelocityVisualiser

// MARK: - Model Tests

struct FrameBundleTests {
    @Test func defaultInitialisation() throws {
        let bundle = FrameBundle()
        #expect(bundle.frameID == 0)
        #expect(bundle.timestampNanos == 0)
        #expect(bundle.sensorID == "")
        #expect(bundle.pointCloud == nil)
        #expect(bundle.clusters == nil)
        #expect(bundle.tracks == nil)
        #expect(bundle.debug == nil)
        #expect(bundle.playbackInfo == nil)
    }

    @Test func customInitialisation() throws {
        var bundle = FrameBundle()
        bundle.frameID = 42
        bundle.timestampNanos = 1_000_000_000
        bundle.sensorID = "hesai-01"

        #expect(bundle.frameID == 42)
        #expect(bundle.timestampNanos == 1_000_000_000)
        #expect(bundle.sensorID == "hesai-01")
    }

    @Test func coordinateFrameInfo() throws {
        var coord = CoordinateFrameInfo()
        coord.frameID = "site/hesai-01"
        coord.referenceFrame = "ENU"
        coord.originLat = 51.5074
        coord.originLon = -0.1278
        coord.originAlt = 10.0
        coord.rotationDeg = 45.0

        #expect(coord.frameID == "site/hesai-01")
        #expect(coord.referenceFrame == "ENU")
        #expect(coord.originLat == 51.5074)
        #expect(coord.originLon == -0.1278)
        #expect(coord.originAlt == 10.0)
        #expect(coord.rotationDeg == 45.0)
    }
}

struct PointCloudFrameTests {
    @Test func defaultInitialisation() throws {
        let pc = PointCloudFrame()
        #expect(pc.frameID == 0)
        #expect(pc.pointCount == 0)
        #expect(pc.x.isEmpty)
        #expect(pc.y.isEmpty)
        #expect(pc.z.isEmpty)
        #expect(pc.intensity.isEmpty)
        #expect(pc.classification.isEmpty)
        #expect(pc.decimationMode == .none)
        #expect(pc.decimationRatio == 1.0)
    }

    @Test func withPoints() throws {
        var pc = PointCloudFrame()
        pc.frameID = 1
        pc.x = [1.0, 2.0, 3.0]
        pc.y = [4.0, 5.0, 6.0]
        pc.z = [0.1, 0.2, 0.3]
        pc.intensity = [100, 150, 200]
        pc.classification = [0, 1, 0]
        pc.pointCount = 3

        #expect(pc.x.count == 3)
        #expect(pc.y.count == 3)
        #expect(pc.z.count == 3)
        #expect(pc.pointCount == 3)
        #expect(pc.intensity[1] == 150)
        #expect(pc.classification[1] == 1)
    }

    @Test func decimationModes() throws {
        #expect(DecimationMode.none.rawValue == 0)
        #expect(DecimationMode.uniform.rawValue == 1)
        #expect(DecimationMode.voxel.rawValue == 2)
        #expect(DecimationMode.foregroundOnly.rawValue == 3)
    }
}

struct ClusterTests {
    @Test func defaultInitialisation() throws {
        let cluster = Cluster()
        #expect(cluster.clusterID == 0)
        #expect(cluster.sensorID == "")
        #expect(cluster.centroidX == 0)
        #expect(cluster.centroidY == 0)
        #expect(cluster.centroidZ == 0)
        #expect(cluster.aabbLength == 0)
        #expect(cluster.aabbWidth == 0)
        #expect(cluster.aabbHeight == 0)
        #expect(cluster.obb == nil)
    }

    @Test func orientedBoundingBox() throws {
        var obb = OrientedBoundingBox()
        obb.centerX = 10.0
        obb.centerY = 20.0
        obb.centerZ = 0.8
        obb.length = 4.5
        obb.width = 1.8
        obb.height = 1.6
        obb.headingRad = Float.pi / 4

        #expect(obb.centerX == 10.0)
        #expect(obb.centerY == 20.0)
        #expect(obb.centerZ == 0.8)
        #expect(obb.length == 4.5)
        #expect(obb.width == 1.8)
        #expect(obb.height == 1.6)
        #expect(abs(obb.headingRad - Float.pi / 4) < 0.001)
    }

    @Test func clusterWithOBB() throws {
        var cluster = Cluster()
        cluster.clusterID = 1
        cluster.obb = OrientedBoundingBox(
            centerX: 10.0, centerY: 20.0, centerZ: 0.8, length: 4.5, width: 1.8, height: 1.6,
            headingRad: 0.0)

        #expect(cluster.obb != nil)
        #expect(cluster.obb?.length == 4.5)
    }

    @Test func clusteringMethods() throws {
        #expect(ClusteringMethod.dbscan.rawValue == 0)
        #expect(ClusteringMethod.connectedComponents.rawValue == 1)
    }
}

struct TrackTests {
    @Test func defaultInitialisation() throws {
        let track = Track()
        #expect(track.trackID == "")
        #expect(track.state == .unknown)
        #expect(track.x == 0)
        #expect(track.y == 0)
        #expect(track.z == 0)
        #expect(track.speedMps == 0)
        #expect(track.confidence == 0)
        #expect(track.motionModel == .cv)
    }

    @Test func trackStates() throws {
        #expect(TrackState.unknown.rawValue == 0)
        #expect(TrackState.tentative.rawValue == 1)
        #expect(TrackState.confirmed.rawValue == 2)
        #expect(TrackState.deleted.rawValue == 3)
    }

    @Test func trackStateColours() throws {
        let unknown = TrackState.unknown.colour
        #expect(unknown.r == 0.5)
        #expect(unknown.g == 0.5)
        #expect(unknown.b == 0.5)

        let tentative = TrackState.tentative.colour
        #expect(tentative.r == 1.0)
        #expect(tentative.g == 1.0)
        #expect(tentative.b == 0.0)  // yellow

        let confirmed = TrackState.confirmed.colour
        #expect(confirmed.r == 0.0)
        #expect(confirmed.g == 1.0)
        #expect(confirmed.b == 0.0)  // green

        let deleted = TrackState.deleted.colour
        #expect(deleted.r == 1.0)
        #expect(deleted.g == 0.0)
        #expect(deleted.b == 0.0)  // red
    }

    @Test func motionModels() throws {
        #expect(MotionModel.cv.rawValue == 0)
        #expect(MotionModel.ca.rawValue == 1)
    }

    @Test func occlusionStates() throws {
        #expect(OcclusionState.none.rawValue == 0)
        #expect(OcclusionState.partial.rawValue == 1)
        #expect(OcclusionState.full.rawValue == 2)
    }

    @Test func trackWithVelocity() throws {
        var track = Track()
        track.trackID = "track-001"
        track.state = .confirmed
        track.x = 15.0
        track.y = 25.0
        track.z = 0.8
        track.vx = 5.0
        track.vy = 0.0
        track.vz = 0.0
        track.speedMps = 5.0
        track.headingRad = 0.0
        track.confidence = 0.95

        #expect(track.trackID == "track-001")
        #expect(track.state == .confirmed)
        #expect(track.speedMps == 5.0)
        #expect(track.confidence == 0.95)
    }
}

struct TrackTrailTests {
    @Test func defaultInitialisation() throws {
        let trail = TrackTrail()
        #expect(trail.trackID == "")
        #expect(trail.points.isEmpty)
    }

    @Test func withPoints() throws {
        var trail = TrackTrail()
        trail.trackID = "track-001"
        trail.points = [
            TrackPoint(x: 10.0, y: 20.0, timestampNanos: 1_000_000_000),
            TrackPoint(x: 11.0, y: 21.0, timestampNanos: 1_100_000_000),
            TrackPoint(x: 12.0, y: 22.0, timestampNanos: 1_200_000_000),
        ]

        #expect(trail.points.count == 3)
        #expect(trail.points[0].x == 10.0)
        #expect(trail.points[2].timestampNanos == 1_200_000_000)
    }
}

struct DebugOverlayTests {
    @Test func associationCandidate() throws {
        var candidate = AssociationCandidate()
        candidate.clusterID = 1
        candidate.trackID = "track-001"
        candidate.distance = 2.5
        candidate.accepted = true

        #expect(candidate.clusterID == 1)
        #expect(candidate.trackID == "track-001")
        #expect(candidate.distance == 2.5)
        #expect(candidate.accepted == true)
    }

    @Test func gatingEllipse() throws {
        var ellipse = GatingEllipse()
        ellipse.trackID = "track-001"
        ellipse.centerX = 10.0
        ellipse.centerY = 20.0
        ellipse.semiMajor = 5.0
        ellipse.semiMinor = 3.0
        ellipse.rotationRad = Float.pi / 6

        #expect(ellipse.trackID == "track-001")
        #expect(ellipse.semiMajor == 5.0)
        #expect(ellipse.semiMinor == 3.0)
    }

    @Test func innovationResidual() throws {
        var residual = InnovationResidual()
        residual.trackID = "track-001"
        residual.predictedX = 10.0
        residual.predictedY = 20.0
        residual.measuredX = 10.5
        residual.measuredY = 20.2
        residual.residualMagnitude = 0.54

        #expect(residual.predictedX == 10.0)
        #expect(residual.measuredX == 10.5)
        #expect(residual.residualMagnitude == 0.54)
    }

    @Test func statePrediction() throws {
        var pred = StatePrediction()
        pred.trackID = "track-001"
        pred.x = 15.0
        pred.y = 25.0
        pred.vx = 5.0
        pred.vy = 0.0

        #expect(pred.trackID == "track-001")
        #expect(pred.x == 15.0)
        #expect(pred.vx == 5.0)
    }
}

struct PlaybackInfoTests {
    @Test func defaultInitialisation() throws {
        let info = PlaybackInfo()
        #expect(info.isLive == true)
        #expect(info.logStartNs == 0)
        #expect(info.logEndNs == 0)
        #expect(info.playbackRate == 1.0)
        #expect(info.paused == false)
        #expect(info.currentFrameIndex == 0)
        #expect(info.totalFrames == 0)
    }

    @Test func replayMode() throws {
        var info = PlaybackInfo()
        info.isLive = false
        info.logStartNs = 1_000_000_000
        info.logEndNs = 2_000_000_000
        info.playbackRate = 0.5
        info.paused = true
        info.currentFrameIndex = 50
        info.totalFrames = 500

        #expect(info.isLive == false)
        #expect(info.logEndNs - info.logStartNs == 1_000_000_000)
        #expect(info.playbackRate == 0.5)
        #expect(info.paused == true)
        #expect(info.currentFrameIndex == 50)
        #expect(info.totalFrames == 500)
    }
}

struct LabelTests {
    @Test func labelEventDefaultInitialisation() throws {
        let label = LabelEvent()
        #expect(label.trackID == "")
        #expect(label.classLabel == "")
        #expect(label.createdBy == nil)
        #expect(label.notes == nil)
        #expect(!label.id.isEmpty)  // UUID should be generated
    }

    @Test func labelEventWithData() throws {
        var label = LabelEvent()
        label.trackID = "track-001"
        label.classLabel = "car"
        label.startTimestampNs = 100_000_000
        label.endTimestampNs = 200_000_000
        label.createdBy = "david"
        label.notes = "White sedan"

        #expect(label.trackID == "track-001")
        #expect(label.classLabel == "car")
        #expect(label.startTimestampNs == 100_000_000)
        #expect(label.endTimestampNs == 200_000_000)
        #expect(label.createdBy == "david")
        #expect(label.notes == "White sedan")
    }

    @Test func labelSetDefaultInitialisation() throws {
        let labelSet = LabelSet()
        #expect(labelSet.sessionID == "")
        #expect(labelSet.sourceFile == "")
        #expect(labelSet.labels.isEmpty)
    }

    @Test func labelSetWithLabels() throws {
        var labelSet = LabelSet()
        labelSet.sessionID = "session-001"
        labelSet.sourceFile = "recording.vrlog"

        var l1 = LabelEvent()
        l1.trackID = "track-001"
        l1.classLabel = "car"
        l1.startTimestampNs = 100_000_000
        l1.endTimestampNs = 200_000_000
        l1.createdBy = "david"

        var l2 = LabelEvent()
        l2.trackID = "track-002"
        l2.classLabel = "pedestrian"
        l2.startTimestampNs = 150_000_000
        l2.endTimestampNs = 250_000_000
        l2.createdBy = "david"

        labelSet.labels = [l1, l2]

        #expect(labelSet.labels.count == 2)
        #expect(labelSet.labels[0].classLabel == "car")
        #expect(labelSet.labels[1].classLabel == "pedestrian")
    }

    @Test func labelEventCodable() throws {
        var label = LabelEvent()
        label.trackID = "track-001"
        label.classLabel = "car"

        let encoder = JSONEncoder()
        let data = try encoder.encode(label)
        #expect(data.count > 0)

        let decoder = JSONDecoder()
        let decoded = try decoder.decode(LabelEvent.self, from: data)
        #expect(decoded.trackID == "track-001")
        #expect(decoded.classLabel == "car")
    }

    @Test func labelSetCodable() throws {
        var labelSet = LabelSet()
        labelSet.sessionID = "session-001"

        var l = LabelEvent()
        l.trackID = "track-001"
        l.classLabel = "car"
        l.startTimestampNs = 100_000_000
        l.endTimestampNs = 200_000_000

        labelSet.labels = [l]

        let encoder = JSONEncoder()
        let data = try encoder.encode(labelSet)
        #expect(data.count > 0)

        let decoder = JSONDecoder()
        let decoded = try decoder.decode(LabelSet.self, from: data)
        #expect(decoded.sessionID == "session-001")
        #expect(decoded.labels.count == 1)
    }
}

// MARK: - TrackSet and ClusterSet Tests

struct TrackSetTests {
    @Test func defaultInitialisation() throws {
        let trackSet = TrackSet()
        #expect(trackSet.frameID == 0)
        #expect(trackSet.timestampNanos == 0)
        #expect(trackSet.tracks.isEmpty)
        #expect(trackSet.trails.isEmpty)
    }

    @Test func withTracksAndTrails() throws {
        var trackSet = TrackSet()
        trackSet.frameID = 1
        trackSet.timestampNanos = 1_000_000_000
        trackSet.tracks = [
            Track(trackID: "track-001", state: .confirmed),
            Track(trackID: "track-002", state: .tentative),
        ]
        trackSet.trails = [
            TrackTrail(
                trackID: "track-001", points: [TrackPoint(x: 10.0, y: 20.0, timestampNanos: 0)])
        ]

        #expect(trackSet.tracks.count == 2)
        #expect(trackSet.trails.count == 1)
        #expect(trackSet.tracks[0].trackID == "track-001")
    }
}

struct ClusterSetTests {
    @Test func defaultInitialisation() throws {
        let clusterSet = ClusterSet()
        #expect(clusterSet.frameID == 0)
        #expect(clusterSet.timestampNanos == 0)
        #expect(clusterSet.clusters.isEmpty)
        #expect(clusterSet.method == .dbscan)
    }

    @Test func withClusters() throws {
        var clusterSet = ClusterSet()
        clusterSet.frameID = 1
        clusterSet.clusters = [
            Cluster(clusterID: 1, centroidX: 10.0, centroidY: 20.0),
            Cluster(clusterID: 2, centroidX: 30.0, centroidY: 40.0),
        ]
        clusterSet.method = .connectedComponents

        #expect(clusterSet.clusters.count == 2)
        #expect(clusterSet.method == .connectedComponents)
    }
}

// MARK: - DebugOverlaySet Tests

struct DebugOverlaySetTests {
    @Test func defaultInitialisation() throws {
        let overlay = DebugOverlaySet()
        #expect(overlay.frameID == 0)
        #expect(overlay.timestampNanos == 0)
        #expect(overlay.associationCandidates.isEmpty)
        #expect(overlay.gatingEllipses.isEmpty)
        #expect(overlay.residuals.isEmpty)
        #expect(overlay.predictions.isEmpty)
    }

    @Test func withDebugData() throws {
        var overlay = DebugOverlaySet()
        overlay.frameID = 1
        overlay.associationCandidates = [
            AssociationCandidate(clusterID: 1, trackID: "track-001", distance: 2.0, accepted: true)
        ]
        overlay.gatingEllipses = [
            GatingEllipse(
                trackID: "track-001", centerX: 10.0, centerY: 20.0, semiMajor: 5.0, semiMinor: 3.0,
                rotationRad: 0.0)
        ]
        overlay.residuals = [
            InnovationResidual(
                trackID: "track-001", predictedX: 10.0, predictedY: 20.0, measuredX: 10.5,
                measuredY: 20.2, residualMagnitude: 0.54)
        ]
        overlay.predictions = [
            StatePrediction(trackID: "track-001", x: 15.0, y: 25.0, vx: 5.0, vy: 0.0)
        ]

        #expect(overlay.associationCandidates.count == 1)
        #expect(overlay.gatingEllipses.count == 1)
        #expect(overlay.residuals.count == 1)
        #expect(overlay.predictions.count == 1)
    }
}

// MARK: - Complete FrameBundle Tests

struct FrameBundleIntegrationTests {
    @Test func completeFrameBundle() throws {
        var bundle = FrameBundle()
        bundle.frameID = 42
        bundle.timestampNanos = 1_000_000_000
        bundle.sensorID = "hesai-01"

        bundle.coordinateFrame = CoordinateFrameInfo(
            frameID: "site/hesai-01", referenceFrame: "ENU", originLat: 51.5074, originLon: -0.1278,
            originAlt: 10.0, rotationDeg: 0.0)

        bundle.pointCloud = PointCloudFrame(
            frameID: 42, timestampNanos: 1_000_000_000, sensorID: "hesai-01", x: [1.0, 2.0, 3.0],
            y: [4.0, 5.0, 6.0], z: [0.1, 0.2, 0.3], intensity: [100, 150, 200],
            classification: [0, 1, 0], decimationMode: .none, decimationRatio: 1.0, pointCount: 3)

        bundle.clusters = ClusterSet(
            frameID: 42, timestampNanos: 1_000_000_000,
            clusters: [Cluster(clusterID: 1, centroidX: 10.0, centroidY: 20.0, centroidZ: 0.8)],
            method: .dbscan)

        bundle.tracks = TrackSet(
            frameID: 42, timestampNanos: 1_000_000_000,
            tracks: [Track(trackID: "track-001", state: .confirmed, x: 10.0, y: 20.0)],
            trails: [
                TrackTrail(
                    trackID: "track-001",
                    points: [TrackPoint(x: 10.0, y: 20.0, timestampNanos: 1_000_000_000)])
            ])

        bundle.playbackInfo = PlaybackInfo(
            isLive: true, logStartNs: 0, logEndNs: 0, playbackRate: 1.0, paused: false,
            currentFrameIndex: 0, totalFrames: 0)

        #expect(bundle.frameID == 42)
        #expect(bundle.pointCloud?.pointCount == 3)
        #expect(bundle.clusters?.clusters.count == 1)
        #expect(bundle.tracks?.tracks.count == 1)
        #expect(bundle.tracks?.trails.count == 1)
        #expect(bundle.playbackInfo?.isLive == true)
    }
}
