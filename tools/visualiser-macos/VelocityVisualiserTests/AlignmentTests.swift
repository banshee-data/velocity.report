//
//  AlignmentTests.swift
//  VelocityVisualiserTests
//
//  The Gerardus alignment client without a server: how a background is
//  packed for UploadScan, the pose maths behind the reference's model matrix
//  and the operator's nudges, the client-side colours, how the open capture's
//  site is found, and the session against a fake service — above all that a
//  nudge sends nothing.
//

import Combine
import Foundation
import SwiftProtobuf
import Testing
import simd

@testable import VelocityVisualiser

// MARK: - Helpers

private func snapshot(count: Int, sequence: UInt64 = 7, settled: Bool = true) -> BackgroundSnapshot
{
    var s = BackgroundSnapshot()
    s.sequenceNumber = sequence
    s.timestampNanos = 1_788_000_000_123_456_789
    s.x = (0..<count).map { Float($0) }
    s.y = (0..<count).map { Float($0) * 2 }
    s.z = (0..<count).map { Float($0) * -0.5 }
    s.confidence = (0..<count).map { UInt32($0 % 97) }
    s.gridMetadata = GridMetadata(
        rings: 40, azimuthBins: 1800, ringElevations: [], settlingComplete: settled)
    return s
}

/// Gerardus's own level pose (src/pose.rs from_heading_deg): +Y turned
/// clockwise from north by the heading.
private func levelPose(headingDeg: Double, at t: SIMD3<Double>) -> SitePose {
    let half = -headingDeg * .pi / 180 / 2
    return SitePose(translation: t, rotation: simd_quatd(ix: 0, iy: 0, iz: sin(half), r: cos(half)))
}

private func tiltedPose() -> SitePose {
    SitePose(
        translation: SIMD3(-9.4, -38.5, 23.0),
        rotation: simd_quatd(angle: 0.7, axis: simd_normalize(SIMD3(0.1, -0.2, 1.0))))
}

private func transform(_ m: simd_double4x4, _ p: SIMD3<Double>) -> SIMD3<Double> {
    let r = m * SIMD4(p, 1)
    return SIMD3(r.x, r.y, r.z)
}

private func close(_ a: SIMD3<Double>, _ b: SIMD3<Double>, _ tol: Double = 1e-9) -> Bool {
    simd_length(a - b) < tol
}

private func close(_ a: simd_double4x4, _ b: simd_double4x4, _ tol: Double = 1e-9) -> Bool {
    (0..<4).allSatisfy { c in simd_length(a[c] - b[c]) < tol }
}

// MARK: - PointBlock packing

struct ScanPackingTests {
    @Test func splitsAtTheContractLimitAndKeepsOrder() {
        let s = snapshot(count: 150_000)
        let blocks = ScanPacking.blocks(from: s)
        #expect(ScanPacking.maxPointsPerBlock == 65_536)
        #expect(blocks.map(\.x.count) == [65_536, 65_536, 18_928])
        #expect(blocks.flatMap(\.x) == s.x)
        #expect(blocks.flatMap(\.y) == s.y)
        #expect(blocks.flatMap(\.z) == s.z)
        #expect(blocks.flatMap(\.confidence) == s.confidence)
        for block in blocks {
            #expect(block.y.count == block.x.count && block.z.count == block.x.count)
            // Scans carry no classes and, from a background, no intensity.
            #expect(block.classification.isEmpty && block.intensity.isEmpty)
        }
    }

    @Test func exactlyOneFullBlock() {
        #expect(ScanPacking.blocks(from: snapshot(count: 65_536)).map(\.x.count) == [65_536])
    }

    @Test func emptySnapshotSendsNoBlocks() {
        #expect(ScanPacking.blocks(from: BackgroundSnapshot()).isEmpty)
    }

    @Test func mismatchedArraysAreCutToTheShortest() {
        var s = snapshot(count: 10)
        s.z.removeLast(3)
        s.confidence.removeLast(5)
        let blocks = ScanPacking.blocks(from: s, maxPoints: 4)
        #expect(blocks.map(\.x.count) == [4, 3])
        #expect(blocks.flatMap(\.z) == s.z)
        // Too short to cover every point, so left out rather than misaligned.
        #expect(blocks.allSatisfy { $0.confidence.isEmpty })
    }

    @Test func headerNamesTheSiteSensorCaseAndSnapshot() {
        let header = ScanPacking.header(
            l16Token: "808580f47", sensorID: "hesai-pandar40p",
            captureRef: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", snapshot: snapshot(count: 3))
        #expect(header.l16Token == "808580f47")
        #expect(header.sensorID == "hesai-pandar40p")
        #expect(header.captureRef == "6451411d-81c3-47f3-a5ff-5ce37b3ab768")
        #expect(header.backgroundSequence == 7)
        #expect(header.kind == .background)
        #expect(header.capturedAt.seconds == 1_788_000_000)
        #expect(header.capturedAt.nanos == 123_456_789)
    }
}

// MARK: - Pose and model matrix

struct SitePoseTests {
    @Test func rotationMatrixIsTheHamiltonOneSimdUses() {
        let pose = tiltedPose()
        let simdMatrix = simd_double3x3(pose.rotation)
        for c in 0..<3 { #expect(simd_length(pose.rotationMatrix[c] - simdMatrix[c]) < 1e-12) }
    }

    @Test func levelPoseHeadingFollowsGerardus() {
        let pose = levelPose(headingDeg: 35, at: .zero)
        #expect(abs(pose.headingDeg - 35) < 1e-9)
        // Sensor +Y points 35° east of north in the site frame.
        let ahead = pose.rotationMatrix * SIMD3(0, 1, 0)
        #expect(close(ahead, SIMD3(sin(35 * .pi / 180), cos(35 * .pi / 180), 0)))
    }

    @Test func inverseUndoesThePose() {
        let pose = tiltedPose()
        #expect(close(pose.inverseMatrix * pose.matrix, matrix_identity_double4x4))
        #expect(close(pose.matrix * pose.inverseMatrix, matrix_identity_double4x4))
    }

    @Test func modelMatrixDrawsSitePointsWhereTheSensorSawThem() {
        let pose = tiltedPose()
        let sensorPoint = SIMD3<Double>(4.0, 12.5, -1.6)
        // p_site = R·p_sensor + t, written out without the matrix helpers.
        let sitePoint = pose.rotationMatrix * sensorPoint + pose.translation
        #expect(close(transform(pose.inverseMatrix, sitePoint), sensorPoint))

        let model = pose.referenceModelMatrix
        let drawn =
            model * SIMD4<Float>(Float(sitePoint.x), Float(sitePoint.y), Float(sitePoint.z), 1)
        #expect(simd_length(SIMD3(drawn.x, drawn.y, drawn.z) - SIMD3<Float>(sensorPoint)) < 1e-4)
        #expect(abs(drawn.w - 1) < 1e-6)
    }

    @Test func protoRoundTrip() {
        let pose = tiltedPose()
        let wire = pose.proto(
            siteFrameID: "site/808580f47", sensorFrameID: "sensor/hesai-pandar40p", source: .manual)
        #expect(wire.siteFrameID == "site/808580f47")
        #expect(wire.sensorFrameID == "sensor/hesai-pandar40p")
        #expect(wire.source == .manual)
        #expect(wire.covariance.isEmpty)
        let back = SitePose(wire)
        #expect(close(back.matrix, pose.matrix, 1e-12))
    }

    @Test func unsetRotationIsTheIdentity() {
        var wire = Gerardus_Align_V1alpha1_SensorPose()
        wire.translationM.x = 1
        let pose = SitePose(wire)
        #expect(close(pose.matrix, simd_double4x4(translation: SIMD3(1, 0, 0))))
    }

    @Test func headingDeltaWrapsTheShortWay() {
        #expect(SitePose.headingDelta(2, 358) == 4)
        #expect(SitePose.headingDelta(358, 2) == -4)
        #expect(SitePose.wrap360(-10) == 350)
    }
}

struct PoseNudgeTests {
    @Test func nudgeMovesTheReferenceByExactlyTheStep() {
        let pose = tiltedPose()
        let step = PoseNudge(offset: SIMD3(0.5, -2, 0.1), yawDeg: 3)
        let moved = pose.nudged(by: step)
        // New model matrix = N · old one: the drawing undergoes the step.
        #expect(close(moved.inverseMatrix, step.matrix * pose.inverseMatrix))
    }

    @Test func turningTheReferenceAnticlockwiseTurnsTheSensorClockwise() {
        let pose = levelPose(headingDeg: 35, at: SIMD3(-9.4, -38.5, 23))
        let moved = pose.nudged(by: PoseNudge(yawDeg: 5))
        let delta = PoseDelta(from: pose, to: moved)
        #expect(abs(delta.headingDeg - 5) < 1e-9)
        // About the sensor origin, so the sensor stays put.
        #expect(abs(delta.east) < 1e-9 && abs(delta.north) < 1e-9 && abs(delta.up) < 1e-9)
    }

    @Test func shiftingTheReferenceMovesTheSensorTheOtherWay() {
        let pose = levelPose(headingDeg: 0, at: .zero)
        let moved = pose.nudged(by: PoseNudge(offset: SIMD3(1, 0, 0)))
        let delta = PoseDelta(from: pose, to: moved)
        #expect(abs(delta.east + 1) < 1e-12 && abs(delta.north) < 1e-12)
        #expect(abs(delta.headingDeg) < 1e-12)
    }

    @Test func manyNudgesStayRigid() {
        var pose = tiltedPose()
        for i in 0..<500 {
            let yaw = i.isMultiple(of: 2) ? 0.7 : -0.3
            pose = pose.nudged(by: PoseNudge(offset: SIMD3(0.1, -0.1, 0.01), yawDeg: yaw))
        }
        let r = pose.rotationMatrix
        let rtr = r.transpose * r
        for c in 0..<3 { #expect(simd_length(rtr[c] - matrix_identity_double3x3[c]) < 1e-9) }
        #expect(abs(simd_determinant(r) - 1) < 1e-9)
    }

    @Test func rendererFoldsTheModelIntoBothMatrices() {
        let base = MetalRenderer.Uniforms(
            modelViewProjection: simd_float4x4(diagonal: SIMD4(2, 3, 4, 1)),
            modelView: simd_float4x4(diagonal: SIMD4(5, 6, 7, 1)), pointSize: 5, time: 0,
            padding: .zero)
        let model = tiltedPose().referenceModelMatrix
        let out = MetalRenderer.uniforms(base, model: model)
        #expect(out.modelViewProjection == base.modelViewProjection * model)
        #expect(out.modelView == base.modelView * model)
        #expect(out.pointSize == base.pointSize)
    }
}

extension simd_double4x4 {
    fileprivate init(translation t: SIMD3<Double>) {
        self = matrix_identity_double4x4
        self[3] = SIMD4(t, 1)
    }
}

// MARK: - Reference geometry and colours

struct ReferenceStyleTests {
    @Test func paletteCodesCarryClassAndBrightness() {
        #expect(ReferenceStyle.palette.count == 256)
        #expect(ReferenceStyle.code(asprsClass: 2, brightness: 0) == 0)
        #expect(ReferenceStyle.code(asprsClass: 2, brightness: 1) == 31)
        #expect(ReferenceStyle.code(asprsClass: 1, brightness: 0) == 32)
        #expect(ReferenceStyle.code(asprsClass: 6, brightness: 2) == 95)
        #expect(ReferenceStyle.code(asprsClass: 250, brightness: 0) == 192)
        let dark = ReferenceStyle.palette[32]
        let light = ReferenceStyle.palette[63]
        #expect(dark.x < light.x && dark.w == 1)
    }

    @Test func aerialVerticesUseThePointLayoutAndSurviveTheShadersRounding() {
        var geometry = ReferenceGeometry()
        var message = Gerardus_Align_V1alpha1_StreamReferenceResponse()
        message.layer = .aerialPoints
        var block = Gerardus_Align_V1alpha1_PointBlock()
        block.x = [1, 2, 3]
        block.y = [4, 5, 6]
        block.z = [7, 8, 9]
        block.classification = [2, 1, 17]
        block.intensity = [0, 500, 1000]
        message.points = block
        message.layerComplete = true
        geometry.append(message)
        #expect(geometry.completeLayers == [.aerialPoints])

        let vertices = ReferenceStyle.aerialVertices(geometry)
        #expect(vertices.count == 15)
        #expect(Array(vertices[0..<3]) == [1, 4, 7])
        let codes = (0..<3).map { Int(vertices[$0 * 5 + 3] * 255 + 0.5) }
        #expect(codes[0] == ReferenceStyle.code(asprsClass: 2, brightness: 0))
        #expect(codes[2] == ReferenceStyle.code(asprsClass: 17, brightness: 1))
        // Classification 0 keeps the shader off its annotation palette.
        #expect((0..<3).allSatisfy { vertices[$0 * 5 + 4] == 0 })
    }

    @Test func missingClassesAndIntensityArePadded() {
        var geometry = ReferenceGeometry()
        var message = Gerardus_Align_V1alpha1_StreamReferenceResponse()
        var block = Gerardus_Align_V1alpha1_PointBlock()
        block.x = [0, 0]
        block.y = [0, 0]
        block.z = [0, 0]
        message.points = block
        geometry.append(message)
        geometry.append(message)
        #expect(geometry.aerialCount == 4)
        #expect(geometry.aerialClass == [1, 1, 1, 1])
        #expect(geometry.aerialIntensity == [0, 0, 0, 0])
        #expect(geometry.completeLayers.isEmpty)
    }

    @Test func linesBecomeSegmentsAndClosedOnesClose() {
        var open = Gerardus_Align_V1alpha1_LineFeature()
        open.kind = .road
        open.xyz = [0, 0, 10, 1, 0, 10, 1, 1, 10]
        var closed = open
        closed.kind = .building
        closed.closed = true
        let vertices = ReferenceStyle.lineVertices([open, closed])
        // 2 segments, then 3: two vertices each, seven floats a vertex.
        #expect(vertices.count == (2 + 3) * 2 * 7)
        #expect(vertices[2] == 10 + ReferenceStyle.lineLift)
        let road = ReferenceStyle.lineColour(.road)
        #expect(Array(vertices[3..<7]) == [road.x, road.y, road.z, road.w])
        // The closing segment of the building ends where it began.
        let last = vertices.count - 7
        #expect(Array(vertices[last..<(last + 2)]) == [0, 0])
    }

    @Test func approachTicksPointBackAlongTheRoadVehiclesArriveOn() {
        var node = Gerardus_Align_V1alpha1_Intersection()
        node.position.x = 10
        node.position.y = 20
        var northbound = Gerardus_Align_V1alpha1_Approach()
        northbound.bearingDeg = 0
        node.approaches = [northbound]
        let vertices = ReferenceStyle.intersectionVertices([node])
        // Two cross segments, then the tick: from the node, 8 m to the south.
        #expect(vertices.count == 3 * 2 * 7)
        let tickEnd = Array(vertices[(5 * 7)..<(5 * 7 + 2)])
        #expect(abs(tickEnd[0] - 10) < 1e-4)
        #expect(abs(tickEnd[1] - (20 - ReferenceStyle.approachLength)) < 1e-4)
    }

    @Test func intensityScaleIgnoresTheBrightestFew() {
        let intensity =
            Array(repeating: UInt32(100), count: 990) + Array(repeating: 60_000, count: 10)
        #expect(ReferenceStyle.intensityScale(intensity) == 100)
        #expect(ReferenceStyle.intensityScale([]) == 0)
    }
}

// MARK: - Site lookup

struct AlignmentSiteResolverTests {
    private let columbus = AlignmentSite(
        clipID: "6451411d", l16Token: "808580f47", l13Token: "808580f4",
        description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)
    private let neighbour = AlignmentSite(
        clipID: "aaaa", l16Token: "808580f43", l13Token: "808580f4",
        description: "S2 archive: Kearny at Pacific (20.0 min across 4 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.797, longitude: -122.405)
    private let elsewhere = AlignmentSite(
        clipID: "98fe9f80", l16Token: "808580f99", l13Token: "808580f9",
        description: "S2 archive: Embarcadero at Bay (15.9 min across 4 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.8067, longitude: -122.4061)

    @Test func clipWins() {
        let found = AlignmentSiteResolver.resolve(
            OpenCaptureEvidence(clipID: "98fe9f80", sourcePaths: []),
            sites: [columbus, elsewhere], filesByCase: [:])
        #expect(found?.site == elsewhere && found?.match == .clip)
    }

    @Test func captureFileMatchesByPath() {
        let found = AlignmentSiteResolver.resolve(
            OpenCaptureEvidence(sourcePaths: [
                "/Volumes/lidar/lidar/s2/s2_sf_3_20260902132037_00002.pcap"
            ]), sites: [columbus, elsewhere],
            filesByCase: ["6451411d": ["s2/s2_sf_3_20260902132037_00002.pcap"]])
        #expect(found?.site == columbus && found?.match == .captureFile)
    }

    @Test func captureNameGivesTheCell() {
        let path =
            "/Volumes/lidar/lidar/pcaps/sf-street-speeds/raw/lidar/20260902T1320_80858-0f4_columbus-broadway.pcapng"
        let alone = AlignmentSiteResolver.resolve(
            OpenCaptureEvidence(sourcePaths: [path]), sites: [columbus, elsewhere], filesByCase: [:]
        )
        #expect(alone?.site == columbus && alone?.match == .captureName)
        // A second site in the same L13 cell: the name's words choose.
        let shared = AlignmentSiteResolver.matchCaptureName(
            path, sites: [neighbour, columbus, elsewhere])
        #expect(shared == columbus)
    }

    @Test func nothingToGoOn() {
        #expect(
            AlignmentSiteResolver.resolve(
                OpenCaptureEvidence(sourcePaths: ["/tmp/kirk0.pcapng", ""]),
                sites: [columbus, elsewhere], filesByCase: [:]) == nil)
    }

    @Test func aVRLogDirectoryNamesItsRun() {
        #expect(
            AlignmentSiteResolver.runID(
                fromVRLogPath:
                    "/Volumes/lidar/offload/sensor_data/lidar/vrlog/e44ae459-ca38-43ee-99fd-badc3a3d7d8f"
            ) == "e44ae459-ca38-43ee-99fd-badc3a3d7d8f")
        #expect(AlignmentSiteResolver.runID(fromVRLogPath: "/tmp/kirk0.pcapng") == nil)
    }

    @Test func namesForThePane() {
        #expect(columbus.name == "Columbus at Broadway")
        #expect(columbus.l16Display == "80858-0f47")
    }
}

// MARK: - Session against a fake service

/// A stand-in for Gerardus: canned answers and a log of every call.
final class FakeAlignmentService: AlignmentServiceClient, @unchecked Sendable {
    private let lock = NSLock()
    private var _calls: [String] = []
    var calls: [String] {
        lock.lock()
        defer { lock.unlock() }
        return _calls
    }
    var reachable = true
    /// Probes that fail before the server first answers.
    var failuresBeforeAnswering = 0
    var lastHint: Gerardus_Align_V1alpha1_PoseFromHintRequest?
    var lastReference: Gerardus_Align_V1alpha1_StreamReferenceRequest?
    var lastRegister: Gerardus_Align_V1alpha1_RegisterRequest?
    var lastReview: Gerardus_Align_V1alpha1_ReviewRegistrationRequest?
    var uploadedBlocks: [Gerardus_Align_V1alpha1_PointBlock] = []
    var records: [Gerardus_Align_V1alpha1_RegistrationRecord] = []
    let hintPose = levelPose(headingDeg: 35, at: SIMD3(-9.4, -38.5, 23))

    private func log(_ name: String) {
        lock.lock()
        _calls.append(name)
        lock.unlock()
    }

    func connect() async throws -> Gerardus_Align_V1alpha1_GetServerInfoResponse {
        log("GetServerInfo")
        guard reachable else { throw GerardusClientError.notConnected }
        if failuresBeforeAnswering > 0 {
            failuresBeforeAnswering -= 1
            throw GerardusClientError.notConnected
        }
        var info = Gerardus_Align_V1alpha1_GetServerInfoResponse()
        info.serverVersion = "0.1.0"
        info.apiVersion = "gerardus.align.v1alpha1"
        info.availableLayers = [.aerialPoints, .osm, .intersections]
        return info
    }

    func disconnect() {}

    func prepareSite(
        l16Token: String, label: String,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_Site {
        log("PrepareSite")
        var progress = Gerardus_Align_V1alpha1_Progress()
        progress.stage = "aerial"
        progress.fraction = 1
        await onProgress(progress)
        var site = Gerardus_Align_V1alpha1_Site()
        site.l16Token = l16Token
        var osm = Gerardus_Align_V1alpha1_SourceInfo()
        osm.layer = .osm
        osm.attribution = "© OpenStreetMap contributors"
        site.sources = [osm]
        return site
    }

    func streamReference(
        _ request: Gerardus_Align_V1alpha1_StreamReferenceRequest
    ) async throws -> ReferenceGeometry {
        log("StreamReference")
        lastReference = request
        var geometry = ReferenceGeometry()
        geometry.aerialX = [0, 1]
        geometry.aerialY = [0, 1]
        geometry.aerialZ = [21, 21]
        geometry.aerialClass = [2, 2]
        geometry.aerialIntensity = [10, 20]
        var road = Gerardus_Align_V1alpha1_LineFeature()
        road.kind = .road
        road.xyz = [0, 0, 21, 5, 0, 21]
        geometry.lines = [road]
        return geometry
    }

    func poseFromHint(
        _ request: Gerardus_Align_V1alpha1_PoseFromHintRequest
    ) async throws -> Gerardus_Align_V1alpha1_PoseFromHintResponse {
        log("PoseFromHint")
        lastHint = request
        var response = Gerardus_Align_V1alpha1_PoseFromHintResponse()
        response.pose = hintPose.proto(
            siteFrameID: "site/\(request.l16Token)", sensorFrameID: "sensor/\(request.sensorID)",
            source: .hint)
        response.summary.headingDeg = 35
        return response
    }

    func uploadScan(
        header: Gerardus_Align_V1alpha1_ScanHeader, blocks: [Gerardus_Align_V1alpha1_PointBlock]
    ) async throws -> Gerardus_Align_V1alpha1_UploadScanResponse {
        log("UploadScan")
        uploadedBlocks = blocks
        var response = Gerardus_Align_V1alpha1_UploadScanResponse()
        response.scan.scanID = "sha256:scan"
        response.scan.header = header
        return response
    }

    /// What Register streams and returns. Empty: a FAILED run with no
    /// hypotheses, as for a site without aerial data.
    var streamed: [Gerardus_Align_V1alpha1_Hypothesis] = []
    var ranked: [Gerardus_Align_V1alpha1_Hypothesis]?
    var runState: Gerardus_Align_V1alpha1_RegistrationState = .failed
    var gates: [Gerardus_Align_V1alpha1_GateResult] = []
    /// Called after each streamed hypothesis, to look at the session then.
    var afterHypothesis: (@MainActor () -> Void)?
    /// Holds Register until the calling task is cancelled.
    var registerUntilCancelled = false

    func register(
        _ request: Gerardus_Align_V1alpha1_RegisterRequest,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void,
        onHypothesis: @escaping @Sendable (Gerardus_Align_V1alpha1_Hypothesis) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_Registration {
        log("Register")
        lastRegister = request
        if registerUntilCancelled {
            while !Task.isCancelled { await Task.yield() }
            throw CancellationError()
        }
        for hypothesis in streamed {
            await onHypothesis(hypothesis)
            await afterHypothesis?()
        }
        var registration = Gerardus_Align_V1alpha1_Registration()
        registration.registrationID = "gms:reg"
        registration.state = runState
        registration.hypotheses = ranked ?? streamed
        registration.gates = gates
        if streamed.isEmpty { registration.failureReason = "the site has no aerial points" }
        registration.initialPose = request.initialPose
        return registration
    }

    private(set) var evaluations: [Gerardus_Align_V1alpha1_EvaluatePoseRequest] = []
    /// Called inside EvaluatePose before it answers.
    var duringEvaluation: (@MainActor (Int) -> Void)?
    /// Where refine moves a pose: this far east, in metres.
    var refineShiftEastM = 1.0

    func evaluatePose(
        _ request: Gerardus_Align_V1alpha1_EvaluatePoseRequest
    ) async throws -> Gerardus_Align_V1alpha1_EvaluatePoseResponse {
        log("EvaluatePose")
        evaluations.append(request)
        await duringEvaluation?(evaluations.count)
        var response = Gerardus_Align_V1alpha1_EvaluatePoseResponse()
        response.pose = request.pose
        if request.refine {
            response.pose.translationM.x += refineShiftEastM
            response.pose.source = .icp
        }
        // Marks which call answered.
        response.summary.headingDeg = Double(evaluations.count)
        response.quality.groundP95M = 0.12
        response.quality.facadeP75M = 0.15
        response.quality.constraintRank = 6
        if request.includePointResiduals {
            response.pointResidualsM = (0..<Int(uploadedBlocks.reduce(0) { $0 + $1.x.count })).map {
                $0 % 3 == 0 ? .nan : Float($0 % 7) * 0.05
            }
        }
        return response
    }

    func reviewRegistration(
        _ request: Gerardus_Align_V1alpha1_ReviewRegistrationRequest
    ) async throws -> Gerardus_Align_V1alpha1_RegistrationRecord {
        log("ReviewRegistration")
        lastReview = request
        var record = Gerardus_Align_V1alpha1_RegistrationRecord()
        record.recordID = "gms:rec\(records.count)"
        switch request.decision {
        case .acceptPose: record.state = .manuallyPlaced
        case .rejectReason: record.state = .rejected
        default: record.state = .unspecified
        }
        record.reviewedAt.seconds = Int64(1_000 + records.count)
        records.append(record)
        return record
    }

    func listRegistrationRecords(
        l16Token: String
    ) async throws -> [Gerardus_Align_V1alpha1_RegistrationRecord] {
        log("ListRegistrationRecords")
        return records
    }
}

/// Records what the session hands the renderer.
final class FakeOverlaySink: ReferenceOverlaySink {
    var overlays: [ReferenceOverlayVertices?] = []
    var matrices: [simd_float4x4] = []
    var visibility: (Bool, Bool, Bool)?

    func setReferenceOverlay(_ vertices: ReferenceOverlayVertices?) { overlays.append(vertices) }
    func setReferenceModelMatrix(_ matrix: simd_float4x4) { matrices.append(matrix) }
    func setReferenceVisibility(aerial: Bool, osm: Bool, intersections: Bool) {
        visibility = (aerial, osm, intersections)
    }
    var residuals: [[Float]?] = []
    func setResidualOverlay(_ vertices: [Float]?, palette: [SIMD4<Float>]) {
        residuals.append(vertices)
    }
}

@MainActor struct AlignmentProbingTests {
    /// A session whose waits between probes take no time, so each test
    /// awaits the probing loop itself rather than a clock.
    private func session(_ service: FakeAlignmentService) -> AlignmentSession {
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        session.reprobeSleep = { _ in await Task.yield() }
        return session
    }

    @Test func probingRetriesUntilAServerAnswers() async {
        let service = FakeAlignmentService()
        service.failuresBeforeAnswering = 2
        let session = session(service)
        session.startProbing()
        // The loop ends by itself once a server has answered.
        await session.probing?.value
        #expect(session.isAvailable)
        #expect(service.calls == ["GetServerInfo", "GetServerInfo", "GetServerInfo"])
    }

    @Test func probingStopsWhenToldTo() async {
        let service = FakeAlignmentService()
        service.reachable = false
        let session = session(service)
        var waits = 0
        session.reprobeSleep = { _ in
            waits += 1
            if waits == 2 { session.stopProbing() }
        }
        session.startProbing()
        let loop = session.probing
        await loop?.value
        #expect(service.calls.count == 2)
        #expect(session.probing == nil)
        #expect(session.availability == .unavailable)
    }

    @Test func theAppLooksForGerardusOnlyWhileConnected() async {
        let service = FakeAlignmentService()
        service.failuresBeforeAnswering = 2
        let session = session(service)
        let app = AppState(autoConnectOnStartup: false, alignment: session)
        #expect(session.probing == nil)

        // Connecting starts the loop and disconnecting stops it, before it
        // has asked anything.
        app.isConnected = true
        #expect(session.probing != nil)
        app.isConnected = false
        #expect(session.probing == nil)
        #expect(service.calls.isEmpty)

        // No server has answered yet, so the pane cannot open.
        app.toggleAlignmentPane()
        #expect(!app.showAlignmentPane)

        // Connected again: it keeps asking until the server answers.
        app.isConnected = true
        await session.probing?.value
        #expect(service.calls.count == 3)
        #expect(session.isAvailable)
        app.toggleAlignmentPane()
        #expect(app.showAlignmentPane)
    }

    @Test func aMissDoesNotRepublishUnavailable() async {
        let service = FakeAlignmentService()
        service.reachable = false
        let session = session(service)
        var published = 0
        let watch = session.$availability.dropFirst().sink { _ in published += 1 }
        await session.probe()
        await session.probe()
        await session.probe()
        #expect(published == 1)
        watch.cancel()
    }

    @Test func theRealIntervalIsThirtySeconds() {
        let session = AlignmentSession(
            makeClient: { _ in FakeAlignmentService() }, lookup: nil, defaults: nil)
        #expect(session.reprobeInterval == .seconds(30))
    }
}

@MainActor struct AlignmentSessionTests {
    private let site = AlignmentSite(
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47",
        l13Token: "808580f4",
        description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

    private func placed() async -> (AlignmentSession, FakeAlignmentService, FakeOverlaySink) {
        let service = FakeAlignmentService()
        let sink = FakeOverlaySink()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        session.sink = sink
        await session.probe()
        session.choose(site)
        session.northAzimuthText = "35"
        await session.placeFromHint()
        return (session, service, sink)
    }

    @Test func noServerMeansUnavailable() async {
        let service = FakeAlignmentService()
        service.reachable = false
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        await session.probe()
        #expect(session.availability == .unavailable)
        #expect(!session.isAvailable)
        // Without a server nothing else is attempted.
        session.choose(site)
        await session.placeFromHint()
        #expect(service.calls == ["GetServerInfo"])
    }

    @Test func placingAsksForTheHintThenTheReferenceAroundIt() async {
        let (session, service, sink) = await placed()
        #expect(session.availability == .available(serverVersion: "0.1.0"))
        #expect(
            service.calls == [
                "GetServerInfo", "PrepareSite", "PoseFromHint", "StreamReference",
                "ListRegistrationRecords",
            ])
        let hint = service.lastHint
        #expect(hint?.l16Token == "808580f47")
        #expect(hint?.hint.position.latitudeDeg == 37.7978)
        #expect(hint?.hint.position.longitudeDeg == -122.4065)
        // north_azimuth_deg 35 at columbus-broadway: the hint heading is 180 − 35.
        #expect(hint?.hint.hasHeadingDeg == true && hint?.hint.headingDeg == 145)
        let reference = service.lastReference
        #expect(reference?.radiusM == 150)
        #expect(reference?.voxelSizeM == 0)
        #expect(reference?.centre.x == service.hintPose.translation.x)
        #expect(reference?.centre.y == service.hintPose.translation.y)
        #expect(reference?.layers == [.aerialPoints, .osm, .intersections])

        #expect(sink.overlays.last??.aerialCount == 2)
        #expect(sink.matrices.last == service.hintPose.referenceModelMatrix)
        #expect(session.osmAttributionVisible == "© OpenStreetMap contributors")
        session.showOSM = false
        #expect(session.osmAttributionVisible == nil)
    }

    @Test func emptyHeadingSendsNoHeading() async {
        let service = FakeAlignmentService()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        await session.probe()
        session.choose(site)
        await session.placeFromHint()
        #expect(service.lastHint?.hint.hasHeadingDeg == false)
    }

    @Test func nudgingChangesOnlyTheMatrix() async {
        let (session, service, sink) = await placed()
        let callsBefore = service.calls
        let matricesBefore = sink.matrices.count
        session.translationStepM = 0.5
        session.nudge(x: 1)
        session.nudge(y: -1)
        session.nudge(z: 1)
        session.nudge(yawSteps: 2)
        #expect(service.calls == callsBefore)
        #expect(sink.matrices.count == matricesBefore + 4)
        #expect(sink.matrices.last == session.pose?.referenceModelMatrix)
        #expect(session.delta?.isZero == false)
        #expect(abs((session.delta?.headingDeg ?? 0) - 2) < 1e-9)
        session.resetNudges()
        #expect(session.delta?.isZero == true)
        #expect(service.calls == callsBefore)
    }

    @Test func registerNeedsASettledBackground() async {
        let (session, service, _) = await placed()
        #expect(!session.canRegister)
        session.noteBackground(snapshot(count: 100, settled: false))
        #expect(!session.canRegister)
        await session.register()
        #expect(!service.calls.contains("UploadScan"))
    }

    @Test func acceptRecordsTheNudgedPose() async {
        let (session, service, _) = await placed()
        session.noteBackground(snapshot(count: 70_000))
        session.nudge(x: 1)
        await session.register()
        #expect(service.uploadedBlocks.map(\.x.count) == [65_536, 4_464])
        #expect(service.lastRegister?.scanID == "sha256:scan")
        #expect(service.lastRegister?.initialPose.source == .manual)
        #expect(service.lastRegister?.hint.headingDeg == 145)
        #expect(session.registration?.state == .failed)

        // The same background is not uploaded twice.
        await session.register()
        #expect(service.calls.filter { $0 == "UploadScan" }.count == 1)

        session.reviewer = "operator"
        await session.accept()
        let review = service.lastReview
        #expect(review?.registrationID == "gms:reg")
        #expect(review?.reviewer == "operator")
        guard case .acceptPose(let accepted) = review?.decision else {
            Issue.record("accept sent no pose")
            return
        }
        #expect(accepted.source == .manual)
        #expect(accepted.siteFrameID == "site/808580f47")
        #expect(accepted.sensorFrameID == "sensor/hesai-pandar40p")
        #expect(close(SitePose(accepted).matrix, session.pose!.matrix, 1e-12))
        #expect(session.lastRecord?.state == .manuallyPlaced)
        #expect(session.records.map(\.recordID) == ["gms:rec0"])
    }

    @Test func rejectWantsAReason() async {
        let (session, service, _) = await placed()
        session.noteBackground(snapshot(count: 10))
        await session.register()
        await session.reject()
        #expect(!service.calls.contains("ReviewRegistration"))
        #expect(session.errorMessage != nil)
        session.rejectReason = "reference is another corner"
        await session.reject()
        guard case .rejectReason(let reason) = service.lastReview?.decision else {
            Issue.record("reject sent no reason")
            return
        }
        #expect(reason == "reference is another corner")
    }

    @Test func recordsAreNewestFirst() async {
        let (session, service, _) = await placed()
        var older = Gerardus_Align_V1alpha1_RegistrationRecord()
        older.recordID = "old"
        older.reviewedAt.seconds = 10
        var newer = older
        newer.recordID = "new"
        newer.reviewedAt.seconds = 20
        service.records = [older, newer]
        await session.loadRecords()
        #expect(session.records.map(\.recordID) == ["new", "old"])
    }

    @Test func aNewCaptureDropsTheScanAndRegistration() async {
        let (session, _, _) = await placed()
        session.noteBackground(snapshot(count: 10))
        await session.register()
        session.captureChanged()
        #expect(session.background == nil)
        #expect(session.registration == nil)
        #expect(!session.canRegister)
        // The reference stays drawn until a site is placed again.
        #expect(session.reference != nil)
    }
}

// MARK: - Automatic registration

private func hypothesis(
    _ id: String, eastM: Double, headingDeg: Double
) -> Gerardus_Align_V1alpha1_Hypothesis {
    var h = Gerardus_Align_V1alpha1_Hypothesis()
    h.hypothesisID = id
    h.pose = levelPose(headingDeg: headingDeg, at: SIMD3(-9.4 + eastM, -38.5, 23)).proto(
        siteFrameID: "site/808580f47", sensorFrameID: "sensor/hesai-pandar40p", source: .icp)
    h.summary.headingDeg = headingDeg
    h.quality.groundP95M = 0.18
    h.quality.facadeP75M = 0.16
    h.quality.constraintRank = 6
    h.seedsConverged = 20
    h.seedsTotal = 20
    return h
}

private func gate(_ name: String, passed: Bool) -> Gerardus_Align_V1alpha1_GateResult {
    var g = Gerardus_Align_V1alpha1_GateResult()
    g.name = name
    g.value = 0.18
    g.threshold = 0.20
    g.passed = passed
    return g
}

@MainActor struct AlignmentRegistrationTests {
    private let site = AlignmentSite(
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47",
        l13Token: "808580f4",
        description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

    /// Placed from the hint, with a settled 300-point background, waits
    /// between nudge and evaluation that take no time, and a fake server
    /// that finds two hypotheses (ranked the other way round) and passes.
    private func placed() async -> (AlignmentSession, FakeAlignmentService, FakeOverlaySink) {
        let service = FakeAlignmentService()
        service.streamed = [
            hypothesis("h-west", eastM: -3, headingDeg: 140),
            hypothesis("h-east", eastM: 5, headingDeg: 143.5),
        ]
        service.ranked = [service.streamed[1], service.streamed[0]]
        service.runState = .candidate
        service.gates = [gate("ground_p95_m", passed: true), gate("seed_agreement", passed: true)]
        let sink = FakeOverlaySink()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        session.sink = sink
        session.evaluateSleep = { _ in await Task.yield() }
        await session.probe()
        session.choose(site)
        session.northAzimuthText = "35"
        await session.placeFromHint()
        session.noteBackground(snapshot(count: 300))
        return (session, service, sink)
    }

    @Test func registerShowsHypothesesAsTheyComeThenDrawsTheBest() async {
        let (session, service, sink) = await placed()
        var seen: [Int] = []
        service.afterHypothesis = { seen.append(session.hypotheses.count) }
        await session.register()
        // Each hypothesis showed as it converged, before the run ended.
        #expect(seen == [1, 2])
        #expect(session.registration?.state == .candidate)
        #expect(session.hypotheses.map(\.hypothesisID) == ["h-east", "h-west"])
        #expect(session.selectedHypothesisID == "h-east")
        let best = SitePose(service.ranked![0].pose)
        #expect(close(session.pose!.matrix, best.matrix, 1e-12))
        #expect(sink.matrices.last == best.referenceModelMatrix)
        #expect(session.acceptance == .hypothesis(index: 0, registered: true))
        #expect(!session.isRegistering)
        // The run starts from the pose drawn then, with the derived heading.
        #expect(service.lastRegister?.hint.headingDeg == 145)
    }

    @Test func acceptingTheDrawnHypothesisSendsItsID() async {
        let (session, service, _) = await placed()
        await session.register()
        session.showHypothesis(session.hypotheses[1])
        #expect(session.acceptance == .hypothesis(index: 1, registered: true))
        session.reviewer = "operator"
        await session.accept()
        guard case .acceptHypothesisID(let id) = service.lastReview?.decision else {
            Issue.record("accept sent no hypothesis")
            return
        }
        #expect(id == "h-west")
    }

    @Test func movingOffAHypothesisAcceptsThePose() async {
        let (session, service, _) = await placed()
        await session.register()
        session.nudge(x: 1)
        #expect(session.selectedHypothesisID == nil)
        #expect(session.acceptance == .pose)
        session.reviewer = "operator"
        await session.accept()
        guard case .acceptPose(let accepted) = service.lastReview?.decision else {
            Issue.record("accept sent no pose")
            return
        }
        #expect(accepted.source == .manual)
        #expect(close(SitePose(accepted).matrix, session.pose!.matrix, 1e-12))
    }

    @Test func aFailedRunsHypothesisIsManuallyPlaced() async {
        let (session, service, _) = await placed()
        service.runState = .failed
        service.gates = [gate("ground_p95_m", passed: true), gate("seed_agreement", passed: false)]
        await session.register()
        #expect(!session.registrationPassed)
        #expect(session.acceptance == .hypothesis(index: 0, registered: false))
    }

    @Test func aRunWithNoHypothesesLeavesTheOperatorsPose() async {
        let (session, service, _) = await placed()
        service.streamed = []
        service.ranked = nil
        service.runState = .failed
        let before = session.pose
        await session.register()
        #expect(session.hypotheses.isEmpty)
        #expect(session.pose == before)
        #expect(session.acceptance == .pose)
    }

    @Test func thePoseIsScoredOnceItRestsAndColoursTheScan() async {
        let (session, service, sink) = await placed()
        session.nudge(x: 1)
        session.nudge(x: 1)
        session.nudge(yawSteps: 1)
        // Moving sent nothing: only the last rest is scored.
        #expect(!service.calls.contains("EvaluatePose"))
        await session.pendingEvaluation?.value
        #expect(service.calls.filter { $0 == "EvaluatePose" }.count == 1)
        let request = service.evaluations.last
        #expect(request?.refine == false)
        #expect(request?.includePointResiduals == true)
        #expect(request?.scanID == "sha256:scan")
        #expect(close(SitePose(request!.pose).matrix, session.pose!.matrix, 1e-12))
        #expect(session.evaluation?.quality.constraintRank == 6)
        // The uploaded scan, coloured: five floats a point, in upload order.
        #expect(sink.residuals.last??.count == 300 * 5)
        session.showResiduals = false
        #expect(sink.residuals.last! == nil)
    }

    @Test func aLateScoreOfAnEarlierPoseIsDropped() async {
        let (session, service, _) = await placed()
        service.duringEvaluation = { call in if call == 1 { session.nudge(x: 1) } }
        session.nudge(x: 1)
        await session.pendingEvaluation?.value
        // The first answer came after the pose moved; the second is kept.
        await session.pendingEvaluation?.value
        #expect(service.evaluations.count == 2)
        #expect(session.evaluation?.summary.headingDeg == 2)
    }

    @Test func snapDrawsTheRefinedPose() async {
        let (session, service, sink) = await placed()
        let before = session.pose!
        await session.snap()
        #expect(service.evaluations.last?.refine == true)
        #expect(abs(session.pose!.translation.x - before.translation.x - 1) < 1e-9)
        #expect(sink.matrices.last == session.pose!.referenceModelMatrix)
        #expect(session.evaluation?.refined == true)
        // Snapping sends one request, and the snapped pose is not re-scored.
        await session.pendingEvaluation?.value
        #expect(service.evaluations.count == 1)
        await session.register()
        session.nudge(x: 0)
        await session.snap()
        session.reviewer = "operator"
        await session.accept()
        guard case .acceptPose(let accepted) = service.lastReview?.decision else {
            Issue.record("accept sent no pose")
            return
        }
        #expect(accepted.source == .icp)
    }

    @Test func aRunCanBeCancelled() async {
        let (session, service, _) = await placed()
        service.registerUntilCancelled = true
        session.startRegistration()
        for _ in 0..<1_000 where !session.isRegistering { await Task.yield() }
        #expect(session.isRegistering)
        session.cancelRegistration()
        await session.pendingRegistration?.value
        #expect(!session.isRegistering)
        #expect(session.errorMessage == "Registration cancelled.")
        #expect(session.registration == nil)
    }
}

struct ResidualStyleTests {
    @Test func codesRunFromNoReferenceThroughTheRamp() {
        #expect(ResidualStyle.palette.count == 256)
        #expect(ResidualStyle.code(residualM: .nan) == 0)
        #expect(ResidualStyle.code(residualM: 0) == 1)
        #expect(ResidualStyle.code(residualM: 0.25) == ResidualStyle.code(residualM: -0.25))
        #expect(ResidualStyle.code(residualM: 0.5) == 255)
        #expect(ResidualStyle.code(residualM: 3) == 255)
        // On the surface is green, far off is red.
        let near = ResidualStyle.palette[1]
        let far = ResidualStyle.palette[255]
        #expect(near.y > near.x && far.x > far.y)
    }

    @Test func verticesNeedOneResidualPerPoint() {
        let s = snapshot(count: 4)
        #expect(ResidualStyle.vertices(snapshot: s, residuals: [0, 0.1]) == nil)
        let vertices = ResidualStyle.vertices(snapshot: s, residuals: [0, .nan, 0.5, 0.1])
        #expect(vertices?.count == 20)
        #expect(Array(vertices![5..<8]) == [s.x[1], s.y[1], s.z[1]])
        #expect(Int(vertices![8] * 255 + 0.5) == 0)
        #expect(Int(vertices![13] * 255 + 0.5) == 255)
    }
}

// MARK: - Hint heading

struct HintHeadingTests {
    @Test func theThreeRegisteredSitesAreOneEightyMinusN() {
        // Gerardus registration found +Y at 143.5°, 49.4° and 129.9° where
        // site-index.json has 35, 131 and 51.
        for (north, found) in [(35.0, 143.5), (131.0, 49.4), (51.0, 129.9)] {
            let heading = AlignmentSession.hintHeading(northAzimuthDeg: north)
            #expect(abs(SitePose.headingDelta(heading, found)) <= 1.5)
        }
    }

    @Test func wrapsIntoACompassBearing() {
        #expect(AlignmentSession.hintHeading(northAzimuthDeg: 0) == 180)
        #expect(AlignmentSession.hintHeading(northAzimuthDeg: 180) == 0)
        #expect(AlignmentSession.hintHeading(northAzimuthDeg: 200) == 340)
        #expect(AlignmentSession.hintHeading(northAzimuthDeg: 350) == 190)
    }

    @MainActor @Test func theFieldIsNorthAzimuthAndTheHintIsDerived() {
        let session = AlignmentSession(
            makeClient: { _ in FakeAlignmentService() }, lookup: nil, defaults: nil)
        #expect(session.hintHeadingDeg == nil)
        session.northAzimuthText = " 35 "
        #expect(session.northAzimuthDeg == 35)
        #expect(session.hintHeadingDeg == 145)
        session.northAzimuthText = "north"
        #expect(session.hintHeadingDeg == nil)
    }

    @MainActor @Test func aValueTypedUnderTheOldMeaningIsNotRead() async throws {
        let suite = "alignment-tests-\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        // Typed when the field meant the heading itself.
        defaults.set("145.0", forKey: "alignment.heading.808580f47")
        let service = FakeAlignmentService()
        let session = AlignmentSession(
            makeClient: { _ in service }, lookup: nil, defaults: defaults)
        await session.probe()
        session.choose(
            AlignmentSite(
                clipID: "case", l16Token: "808580f47", l13Token: "808580f4",
                description: "S2 archive: Columbus at Broadway", sensorID: "hesai-pandar40p",
                latitude: 37.7978, longitude: -122.4065))
        #expect(session.northAzimuthText == "")
        session.northAzimuthText = "35"
        await session.placeFromHint()
        #expect(defaults.string(forKey: "alignment.northAzimuth.808580f47") == "35.0")
        #expect(service.lastHint?.hint.headingDeg == 145)
        defaults.removePersistentDomain(forName: suite)
    }
}

// MARK: - Address

struct GerardusAddressTests {
    @Test func readsHostsAddressesAndPorts() throws {
        let cases: [(String, String, Int, String)] = [
            ("127.0.0.1", "127.0.0.1", 50061, "127.0.0.1:50061"),
            ("  127.0.0.1:50061\n", "127.0.0.1", 50061, "127.0.0.1:50061"),
            (
                "machine.tailnet.ts.net", "machine.tailnet.ts.net", 50061,
                "machine.tailnet.ts.net:50061"
            ),
            (
                "Machine.Tailnet.TS.net.", "machine.tailnet.ts.net", 50061,
                "machine.tailnet.ts.net:50061"
            ), ("100.64.0.1:50062", "100.64.0.1", 50062, "100.64.0.1:50062"),
            ("[fd7a:115c:a1e0::1]:50061", "fd7a:115c:a1e0::1", 50061, "[fd7a:115c:a1e0::1]:50061"),
            ("fd7a:115c:a1e0::1", "fd7a:115c:a1e0::1", 50061, "[fd7a:115c:a1e0::1]:50061"),
            ("[::1]", "::1", 50061, "[::1]:50061"),
            ("FD7A::1", "fd7a::1", 50061, "[fd7a::1]:50061"),
            ("localhost:1", "localhost", 1, "localhost:1"),
            ("gerardus:65535", "gerardus", 65535, "gerardus:65535"),
        ]
        for (text, host, port, written) in cases {
            let address = try GerardusAddress.parse(text)
            #expect(address.host == host, "\(text)")
            #expect(address.port == port, "\(text)")
            #expect(address.description == written, "\(text)")
            // What is written reads back as the same address.
            #expect(try GerardusAddress.parse(address.description) == address)
        }
    }

    @Test func refusesWhatIsNotAHostAndPort() {
        let refused: [(String, GerardusAddress.ParseError)] = [
            ("", .empty), ("   ", .empty),
            (
                "http://machine.tailnet.ts.net:50061",
                .notAnAddress("http://machine.tailnet.ts.net:50061")
            ), ("machine.tailnet.ts.net/align", .notAnAddress("machine.tailnet.ts.net/align")),
            ("machine tailnet", .notAnAddress("machine tailnet")),
            ("user@machine", .notAnAddress("user@machine")), ("machine:0", .invalidPort("0")),
            ("machine:65536", .invalidPort("65536")), ("machine:http", .invalidPort("http")),
            ("machine:", .invalidPort("An empty port")), ("[fd7a::1", .notAnAddress("[fd7a::1")),
            ("[fd7a::1]50061", .notAnAddress("[fd7a::1]50061")),
            ("[machine]:50061", .invalidHost("machine")), ("fd7a::zz", .invalidHost("fd7a::zz")),
            ("300.1.1.1", .invalidHost("300.1.1.1")),
            ("-machine.example", .invalidHost("-machine.example")),
            ("ma_chine.example", .invalidHost("ma_chine.example")),
            ("machine..example", .invalidHost("machine..example")),
        ]
        for (text, expected) in refused {
            #expect(throws: expected, "\(text)") { try GerardusAddress.parse(text) }
        }
    }

    @Test func anIPv6LiteralIsDialledAndANameIsResolved() throws {
        let v6 = GerardusClient.target(try GerardusAddress.parse("[fd7a::1]:50061"))
        let name = GerardusClient.target(try GerardusAddress.parse("machine.tailnet.ts.net"))
        let v4 = GerardusClient.target(try GerardusAddress.parse("100.64.0.1"))
        #expect(String(describing: type(of: v6)).contains("IPv6"))
        #expect(String(describing: type(of: name)).contains("DNS"))
        #expect(String(describing: type(of: v4)).contains("DNS"))
    }

    @Test func theAddressIsKeptAsAPreference() throws {
        let suite = "alignment-address-tests-\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        #expect(GerardusAddress.preferenceKey == "alignment.serviceAddress")
        // Unset means the server on this Mac, exactly as before.
        #expect(GerardusAddress.saved(in: defaults) == .local)
        #expect(GerardusAddress.local.description == "127.0.0.1:50061")

        let remote = try GerardusAddress.parse("machine.tailnet.ts.net")
        remote.save(to: defaults)
        #expect(
            defaults.string(forKey: "alignment.serviceAddress") == "machine.tailnet.ts.net:50061")
        #expect(GerardusAddress.saved(in: defaults) == remote)

        // Going back to the default removes the preference.
        GerardusAddress.local.save(to: defaults)
        #expect(defaults.object(forKey: "alignment.serviceAddress") == nil)

        // A value broken outside the app falls back to this Mac.
        defaults.set("http://nowhere/", forKey: "alignment.serviceAddress")
        #expect(GerardusAddress.saved(in: defaults) == .local)
        #expect(GerardusAddress.saved(in: nil) == .local)
        defaults.removePersistentDomain(forName: suite)
    }
}

/// A server that answers only when the test lets it, to make a probe that
/// is still waiting when the address changes.
final class HeldAlignmentService: AlignmentServiceClient, @unchecked Sendable {
    private let inner = FakeAlignmentService()
    private var held: CheckedContinuation<Void, Never>?
    var isHeld: Bool { held != nil }
    private(set) var disconnected = false

    func release() {
        held?.resume()
        held = nil
    }

    func connect() async throws -> Gerardus_Align_V1alpha1_GetServerInfoResponse {
        await withCheckedContinuation { held = $0 }
        return try await inner.connect()
    }
    func disconnect() { disconnected = true }
    func prepareSite(
        l16Token: String, label: String,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_Site {
        try await inner.prepareSite(l16Token: l16Token, label: label, onProgress: onProgress)
    }
    func streamReference(
        _ request: Gerardus_Align_V1alpha1_StreamReferenceRequest
    ) async throws -> ReferenceGeometry { try await inner.streamReference(request) }
    func poseFromHint(
        _ request: Gerardus_Align_V1alpha1_PoseFromHintRequest
    ) async throws -> Gerardus_Align_V1alpha1_PoseFromHintResponse {
        try await inner.poseFromHint(request)
    }
    func uploadScan(
        header: Gerardus_Align_V1alpha1_ScanHeader, blocks: [Gerardus_Align_V1alpha1_PointBlock]
    ) async throws -> Gerardus_Align_V1alpha1_UploadScanResponse {
        try await inner.uploadScan(header: header, blocks: blocks)
    }
    func register(
        _ request: Gerardus_Align_V1alpha1_RegisterRequest,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void,
        onHypothesis: @escaping @Sendable (Gerardus_Align_V1alpha1_Hypothesis) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_Registration {
        try await inner.register(request, onProgress: onProgress, onHypothesis: onHypothesis)
    }
    func evaluatePose(
        _ request: Gerardus_Align_V1alpha1_EvaluatePoseRequest
    ) async throws -> Gerardus_Align_V1alpha1_EvaluatePoseResponse {
        try await inner.evaluatePose(request)
    }
    func reviewRegistration(
        _ request: Gerardus_Align_V1alpha1_ReviewRegistrationRequest
    ) async throws -> Gerardus_Align_V1alpha1_RegistrationRecord {
        try await inner.reviewRegistration(request)
    }
    func listRegistrationRecords(
        l16Token: String
    ) async throws -> [Gerardus_Align_V1alpha1_RegistrationRecord] {
        try await inner.listRegistrationRecords(l16Token: l16Token)
    }
}

@MainActor struct AlignmentAddressChangeTests {
    private let remote = try! GerardusAddress.parse("machine.tailnet.ts.net")

    /// Lets the session's own tasks run until `done` holds, with no clock.
    private func settle(_ done: () -> Bool) async {
        for _ in 0..<1_000 where !done() { await Task.yield() }
    }

    @Test func theSessionStartsFromTheSavedAddress() throws {
        let suite = "alignment-address-tests-\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        remote.save(to: defaults)
        var asked: [GerardusAddress] = []
        let session = AlignmentSession(
            makeClient: {
                asked.append($0)
                return FakeAlignmentService()
            }, lookup: nil, defaults: defaults)
        #expect(session.serviceAddress == remote)
        #expect(asked.isEmpty)
        defaults.removePersistentDomain(forName: suite)
    }

    @Test func changingTheAddressAsksTheNewServerAndKeepsIt() async throws {
        let suite = "alignment-address-tests-\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        let local = FakeAlignmentService()
        let unreachable = FakeAlignmentService()
        unreachable.reachable = false
        var asked: [GerardusAddress] = []
        let session = AlignmentSession(
            makeClient: { address in
                asked.append(address)
                return address == .local ? local : unreachable
            }, lookup: nil, defaults: defaults)
        await session.probe()
        #expect(session.isAvailable)

        // Not connected to velocity.report: one probe of the new address.
        session.useServiceAddress(remote)
        #expect(session.availability == .unknown)
        await settle { session.availability == .unavailable }
        #expect(session.availability == .unavailable)
        #expect(session.probeFailure != nil)
        #expect(!session.isChecking)
        #expect(asked == [.local, remote])
        #expect(defaults.string(forKey: "alignment.serviceAddress") == remote.description)

        // Back to this Mac: found again, and the preference is cleared.
        session.useServiceAddress(.local)
        await settle { session.isAvailable }
        #expect(session.isAvailable)
        #expect(session.probeFailure == nil)
        #expect(defaults.object(forKey: "alignment.serviceAddress") == nil)
        defaults.removePersistentDomain(forName: suite)
    }

    @Test func whileConnectedTheLoopFollowsTheNewAddress() async {
        let found = FakeAlignmentService()
        found.failuresBeforeAnswering = 1
        var asked: [GerardusAddress] = []
        let session = AlignmentSession(
            makeClient: { address in
                asked.append(address)
                return found
            }, lookup: nil, defaults: nil)
        session.reprobeSleep = { _ in await Task.yield() }
        session.startProbing()
        await session.probing?.value
        #expect(session.isAvailable)

        session.useServiceAddress(remote)
        #expect(session.probing != nil)
        await session.probing?.value
        #expect(session.isAvailable)
        #expect(session.serviceAddress == remote)
        // The first miss made a fresh client for the retry; the change made
        // one for the new address.
        #expect(asked == [.local, .local, remote])
    }

    @Test func aLateAnswerFromTheOldServerIsIgnored() async {
        let held = HeldAlignmentService()
        let unreachable = FakeAlignmentService()
        unreachable.reachable = false
        let session = AlignmentSession(
            makeClient: { $0 == .local ? held as AlignmentServiceClient : unreachable },
            lookup: nil, defaults: nil)
        let first = Task { await session.probe() }
        await settle { held.isHeld }
        #expect(session.isChecking)

        session.useServiceAddress(remote)
        await settle { session.availability == .unavailable }
        held.release()
        await first.value
        #expect(session.availability == .unavailable)
        #expect(session.serviceAddress == remote)
        #expect(held.disconnected)
    }

    @Test func changingTheAddressClosesThePane() async {
        let service = FakeAlignmentService()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        let app = AppState(autoConnectOnStartup: false, alignment: session)
        await session.probe()
        app.toggleAlignmentPane()
        #expect(app.showAlignmentPane)
        session.useServiceAddress(remote)
        #expect(!app.showAlignmentPane)
    }
}
