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
import GRPCCore
import Metal
import MetalKit
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
            captureRef: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", snapshot: snapshot(count: 3),
            capturedAtNanos: 1_788_000_000_123_456_789)
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

// MARK: - Site model layers

/// The registered layers' payloads: decoding, colours and the ground mesh.
struct SiteModelStyleTests {
    private func geometry(
        _ layers: [Gerardus_Align_V1alpha1_ReferenceLayer], modelID: String = "sha256:m"
    ) -> ReferenceGeometry {
        var geometry = ReferenceGeometry()
        for layer in layers {
            for message in referenceMessages(layer, modelID: modelID) { geometry.append(message) }
        }
        return geometry
    }

    private func flatField(n: UInt32, cell: Float = 1) -> Gerardus_Align_V1alpha1_Heightfield {
        var field = Gerardus_Align_V1alpha1_Heightfield()
        field.cellM = cell
        field.nx = n
        field.ny = n
        field.z = Array(repeating: 0, count: Int(n * n))
        return field
    }

    @Test func scanPointsDecodeWithTheirScanRecordAndModel() {
        let g = geometry([.registeredScans])
        #expect(g.scans.map(\.scanID) == [ownScanID, "sha256:other"])
        #expect(g.scans.map(\.recordID) == ["gms:rec-own", "gms:rec-other"])
        #expect(g.scans.allSatisfy { $0.kind == .background })
        #expect(g.scans.first?.captureRef == "capture/\(ownScanID)")
        #expect(g.scanPointCount == 5)
        #expect(g.scanOrder == [ownScanID, "sha256:other"])
        #expect(g.modelID == "sha256:m")
        #expect(g.completeLayers == [.registeredScans])
        // Scan points never land among the aerial ones.
        #expect(g.aerialCount == 0)
    }

    @Test func aBundleLayerLeavesTheModelIDAlone() {
        var g = geometry([.registeredScans], modelID: "sha256:m")
        var aerial = Gerardus_Align_V1alpha1_StreamReferenceResponse()
        aerial.layer = .aerialPoints
        aerial.points.x = [1]
        aerial.points.y = [1]
        aerial.points.z = [1]
        g.append(aerial)
        #expect(g.modelID == "sha256:m")
    }

    @Test func scansTakeOneCodeEachAboveTheASPRSGroups() {
        #expect(ReferenceStyle.scanCodeBase == 7 * 32)
        #expect(ReferenceStyle.scanCodeCount == 32)
        #expect(ReferenceStyle.scanCode(index: 0) == 224)
        #expect(ReferenceStyle.scanCode(index: 31) == 255)
        #expect(ReferenceStyle.scanCode(index: 32) == 224)
        #expect(ReferenceStyle.code(asprsClass: 250, brightness: 1) < 224)
        let colours = (224..<256).map { ReferenceStyle.palette[$0] }
        #expect(Set(colours.map { [$0.x, $0.y, $0.z] }).count == 32)
        #expect(colours.allSatisfy { $0.w == 1 && $0 != SIMD4(1, 1, 1, 1) })
        // None is the live foreground's green.
        #expect(colours.allSatisfy { !($0.y > $0.x + 0.2 && $0.y > $0.z + 0.2) })
    }

    @Test func scanVerticesAreColouredByScanAndHiddenByID() {
        let g = geometry([.registeredScans])
        let all = ReferenceStyle.scanVertices(g, hidden: [])
        #expect(all.count == 5 * 5)
        #expect(Array(all[0..<3]) == [-9, -38, 21])
        let codes = (0..<5).map { Int(all[$0 * 5 + 3] * 255 + 0.5) }
        #expect(codes == [224, 224, 224, 225, 225])
        // Classification 0 keeps the shader on its table path.
        #expect((0..<5).allSatisfy { all[$0 * 5 + 4] == 0 })

        let others = ReferenceStyle.scanVertices(g, hidden: [ownScanID])
        #expect(others.count == 2 * 5)
        // Hiding a scan does not recolour the rest.
        #expect(Int(others[3] * 255 + 0.5) == 225)
        let vertices = ReferenceStyle.vertices(for: g, hiddenScanIDs: [ownScanID])
        #expect(vertices.scanCount == 2)
        #expect(vertices.aerialCount == 0)
        #expect(vertices.palette == ReferenceStyle.palette)
    }

    @Test func eachObservedKindHasItsOwnColour() {
        #expect(ReferenceStyle.lineColour(.sensor) == SIMD4(1, 1, 1, 1))
        let amber = ReferenceStyle.lineColour(.correction)
        #expect(amber.x == 1 && amber.y > 0.5 && amber.y < 0.85 && amber.z == 0)
        let cyan = ReferenceStyle.lineColour(.facade)
        #expect(cyan.x < 0.1 && cyan.y > 0.8 && cyan.z > 0.9)
        let kinds: [Gerardus_Align_V1alpha1_FeatureKind] = [
            .road, .path, .kerb, .rail, .building, .sensor, .correction, .facade, .pole, .other,
        ]
        let colours = kinds.map { ReferenceStyle.lineColour($0) }
        #expect(Set(colours.map { [$0.x, $0.y, $0.z] }).count == kinds.count)
    }

    @Test func eachLineLayerKeepsItsOwnFeatures() {
        let g = geometry([.osm, .footprints, .observed])
        #expect(g.lines.map(\.kind) == [.road])
        #expect(g.footprints.map(\.kind) == [.building])
        #expect(g.observed.map(\.kind) == [.sensor, .correction, .facade, .pole])
        #expect(g.modelID == "sha256:m")
        let v = ReferenceStyle.vertices(for: g)
        #expect(v.osmVertexCount == 2)
        // The closed outline: three segments.
        #expect(v.footprintVertexCount == 6)
        // Sensor 2 segments, correction 1, facade 4 (foot, top, two
        // uprights), pole 1.
        #expect(v.observedVertexCount == 16)
    }

    @Test func sensorsAndPolesAreDrawnWhereTheyAre() {
        var sensor = Gerardus_Align_V1alpha1_LineFeature()
        sensor.kind = .sensor
        sensor.xyz = [0, 0, 21, 0, 0, 23]
        let v = ReferenceStyle.lineVertices([sensor])
        // The stalk ends at the sensor's origin: no display lift.
        #expect(v[2] == 21 && v[9] == 23)
        let white = ReferenceStyle.lineColour(.sensor)
        #expect(Array(v[3..<7]) == [white.x, white.y, white.z, white.w])
        sensor.kind = .pole
        #expect(ReferenceStyle.lineVertices([sensor])[2] == 21)
        #expect(ReferenceStyle.lift(.correction) == 0)
        #expect(ReferenceStyle.lift(.facade) == ReferenceStyle.lineLift)
    }

    @Test func aFacadeTraceStandsAsHighAsTheScansSawIt() {
        var facade = Gerardus_Align_V1alpha1_LineFeature()
        facade.kind = .facade
        facade.xyz = [0, 5, 21, 10, 5, 21]
        facade.heightM = 6
        let v = ReferenceStyle.lineVertices([facade])
        #expect(v.count == 4 * 2 * 7)
        let lift = ReferenceStyle.lineLift
        #expect(v[2] == 21 + lift)
        // The second segment is the top edge.
        #expect(v[14 + 2] == 21 + lift + 6)
        #expect(v[21 + 2] == 21 + lift + 6)
        facade.heightM = 0
        #expect(ReferenceStyle.lineVertices([facade]).count == 2 * 7)
    }

    @Test func aHeightfieldDecodesAndDrawsAsAWireframeAroundItsHoles() {
        let g = geometry([.groundMesh])
        #expect(g.groundMesh.count == 1)
        let field = g.groundMesh[0]
        #expect(field.nx == 3 && field.ny == 2 && field.cellM == 1)
        #expect(field.origin.x == -10 && field.origin.y == -40)
        #expect(g.groundMeshCells == 5)
        #expect(g.modelID == "sha256:m")
        #expect(g.completeLayers == [.groundMesh])

        let v = ReferenceStyle.groundMeshVertices(g.groundMesh, centre: nil)
        // Row 0's two edges and the columns at i = 0 and 2; (1, 1) is NaN.
        #expect(v.count == 4 * 2 * 7)
        for k in 0..<(v.count / 7) {
            // z[j·nx + i] lies at origin + (i, j)·cell_m.
            let i = v[k * 7] + 10
            let j = v[k * 7 + 1] + 40
            #expect(i == i.rounded() && j == j.rounded())
            #expect(!(i == 1 && j == 1))
            #expect(v[k * 7 + 2] == 21 + ReferenceStyle.lineLift)
        }
        let colour = ReferenceStyle.groundMeshColour
        #expect(Array(v[3..<7]) == [colour.x, colour.y, colour.z, colour.w])
    }

    @Test func theGroundMeshIsClippedToTheStreamingRadius() {
        let field = flatField(n: 21)
        let v = ReferenceStyle.groundMeshVertices(
            [field], centre: SIMD2(10, 10), radiusM: 5, denseRadiusM: 100)
        #expect(!v.isEmpty)
        for k in 0..<(v.count / 7) { #expect(hypot(v[k * 7] - 10, v[k * 7 + 1] - 10) <= 5 + 1e-4) }
        #expect(ReferenceStyle.groundMeshRadiusM == 150)
    }

    @Test func beyondFortyMetresTheMeshIsDrawnEveryTwo() {
        #expect(ReferenceStyle.groundMeshDenseRadiusM == 40)
        #expect(ReferenceStyle.groundMeshCoarseSpacingM == 2)
        let field = flatField(n: 9)
        // All of it far from the sensor: a 2 m lattice, 5 lines each way of
        // 4 edges.
        let coarse = ReferenceStyle.groundMeshVertices(
            [field], centre: SIMD2(-1000, -1000), radiusM: 10_000, denseRadiusM: 40)
        #expect(coarse.count == 40 * 2 * 7)
        for k in 0..<(coarse.count / 7) {
            #expect(Int(coarse[k * 7]) % 2 == 0 && Int(coarse[k * 7 + 1]) % 2 == 0)
        }
        // All of it near: every 1 m line, 9 each way of 8 edges.
        let dense = ReferenceStyle.groundMeshVertices([field], centre: SIMD2(4, 4))
        #expect(dense.count == 144 * 2 * 7)
        // A coarse edge is not drawn across a hole.
        var holed = field
        holed.z[1] = .nan
        let around = ReferenceStyle.groundMeshVertices(
            [holed], centre: SIMD2(-1000, -1000), radiusM: 10_000, denseRadiusM: 40)
        #expect(around.count == 39 * 2 * 7)
    }

    @Test func aMalformedHeightfieldDrawsNothing() {
        var short = flatField(n: 3)
        short.z.removeLast()
        #expect(ReferenceStyle.groundMeshVertices([short], centre: nil).isEmpty)
        var flat = flatField(n: 3)
        flat.cellM = 0
        #expect(ReferenceStyle.groundMeshVertices([flat], centre: nil).isEmpty)
        #expect(ReferenceStyle.groundMeshVertices([], centre: nil).isEmpty)
    }

    @Test func replacingTheRegisteredLayersKeepsTheBundlesOwn() {
        var g = geometry(
            [.aerialPoints, .osm, .footprints, .registeredScans, .observed, .groundMesh],
            modelID: "sha256:a")
        g.replaceRegistered(with: geometry([.observed], modelID: "sha256:b"))
        #expect(g.aerialCount == 2 && g.lines.count == 1 && g.footprints.count == 1)
        #expect(g.scans.isEmpty && g.groundMesh.isEmpty)
        #expect(g.observed.count == 4)
        #expect(g.modelID == "sha256:b")
        #expect(g.completeLayers == [.aerialPoints, .osm, .footprints, .observed])
    }
}

// MARK: - Boxes and wall planes

/// DataSF boxes (REFERENCE_LAYER_BOXES) and the observed layer's wall planes
/// (site model spike §6.6): decoding, colours, and closed rectangles drawn as
/// given.
struct BoxesAndWallPlanesStyleTests {
    private func geometry(_ layers: [Gerardus_Align_V1alpha1_ReferenceLayer]) -> ReferenceGeometry {
        var geometry = ReferenceGeometry()
        for layer in layers {
            for message in referenceMessages(layer, modelID: "sha256:m") {
                geometry.append(message)
            }
        }
        return geometry
    }

    private func observed(
        _ features: [Gerardus_Align_V1alpha1_LineFeature]
    ) -> Gerardus_Align_V1alpha1_StreamReferenceResponse {
        var message = Gerardus_Align_V1alpha1_StreamReferenceResponse()
        message.layer = .observed
        message.lines.features = features
        message.modelID = "sha256:m"
        message.layerComplete = true
        return message
    }

    /// The vertex at `index` of a line layout, as (x, y, z).
    private func point(_ v: [Float], _ index: Int) -> [Float] {
        Array(v[(index * 7)..<(index * 7 + 3)])
    }

    private func colour(_ v: [Float], _ index: Int) -> SIMD4<Float> {
        SIMD4(v[index * 7 + 3], v[index * 7 + 4], v[index * 7 + 5], v[index * 7 + 6])
    }

    @Test func boxWallsDecodeIntoTheirOwnLayerWithTheirTags() {
        let g = geometry([.footprints, .boxes])
        #expect(g.footprints.count == 1)
        #expect(g.boxes.count == 2)
        #expect(g.lines.isEmpty && g.observed.isEmpty)
        #expect(g.boxes.allSatisfy { $0.kind == .building && $0.closed && $0.xyz.count == 12 })
        #expect(g.boxes.map { $0.tags["wall"] } == ["0", "1"])
        #expect(g.boxes.first?.tags["roof_p50_m"] == "12.0")
        #expect(g.boxes.first?.tags["shift_east_m"] == "0.3")
        #expect(g.partyWallCount == 1)
        #expect(g.boxes.map(ReferenceStyle.isPartyWall) == [false, true])
        #expect(g.completeLayers == [.footprints, .boxes])
        // Boxes are bundle data: they carry no model id.
        #expect(g.modelID == "")
    }

    @Test func boxWallsAreDrawnClosedAndPartyWallsFainter() {
        let g = geometry([.boxes])
        let v = ReferenceStyle.boxVertices(g.boxes)
        // Two rectangles of four segments each: nothing added for height_m.
        #expect(v.count == 2 * 4 * 2 * 7)
        let lift = ReferenceStyle.lineLift
        #expect(point(v, 0) == [10, 10, 21 + lift])
        // The fourth segment of the first wall closes it on its first vertex.
        #expect(point(v, 7) == [10, 10, 21 + lift])
        #expect(point(v, 15) == [20, 10, 21 + lift])
        // The roof edge stays at the roof, not at the roof plus height_m.
        #expect((0..<16).allSatisfy { point(v, $0)[2] <= 33 + lift + 1e-4 })

        let street = colour(v, 0)
        let party = colour(v, 8)
        #expect(street == ReferenceStyle.boxWallColour)
        #expect(party == ReferenceStyle.partyWallColour)
        #expect(party.w < street.w / 2)
        #expect(SIMD3(party.x, party.y, party.z) == SIMD3(street.x, street.y, street.z))
        // A building red, distinct from the flat footprints' outline.
        let flat = ReferenceStyle.lineColour(.building)
        #expect(street != flat)
        #expect(street.x > street.y + 0.2 && street.x > street.z + 0.2)
        #expect(street.x < flat.x)

        let all = ReferenceStyle.vertices(for: g)
        #expect(all.boxVertexCount == 16)
        #expect(all.footprintVertexCount == 0)
    }

    @Test func wallPlanesDecodeApartFromTheObservedFeatures() {
        var g = ReferenceGeometry()
        var facade = Gerardus_Align_V1alpha1_LineFeature()
        facade.kind = .facade
        facade.xyz = [0, 5, 21, 10, 5, 21]
        g.append(
            observed([facade, wallPlane(role: "wall"), wallPlane(role: "recess", offset: -0.4)]))
        #expect(g.observed.map(\.kind) == [.facade])
        #expect(g.wallPlanes.count == 2)
        #expect(g.wallPlaneRoles == [.wall: 1, .recess: 1])
        #expect(g.wallPlanes.last?.tags["offset_m"] == "-0.4")
        #expect(g.completeLayers == [.observed])

        // The wall planes are registered data: replaced with the observed
        // layer, while the boxes stay with the bundle.
        var drawn = geometry([.boxes, .observed])
        #expect(drawn.wallPlanes.isEmpty)
        drawn.replaceRegistered(with: g)
        #expect(drawn.wallPlanes.count == 2)
        #expect(drawn.observed.count == 1)
        #expect(drawn.boxes.count == 2)
        #expect(drawn.completeLayers == [.boxes, .observed])
        var merged = ReferenceGeometry()
        merged.merge(drawn)
        #expect(merged.wallPlanes.count == 2 && merged.boxes.count == 2)
    }

    @Test func eachWallPlaneRoleHasItsOwnColour() {
        let wall = ReferenceStyle.wallPlaneColour(.wall)
        let popout = ReferenceStyle.wallPlaneColour(.popout)
        let recess = ReferenceStyle.wallPlaneColour(.recess)
        // Cyan-teal, orange, violet.
        #expect(wall.x < 0.2 && wall.y > 0.7 && wall.z > 0.6 && wall.z < wall.y)
        #expect(popout.x == 1 && popout.y > 0.4 && popout.y < 0.75 && popout.z < 0.2)
        #expect(recess.z == 1 && recess.x > 0.5 && recess.y < 0.5)
        // None is a facade trace's cyan, so planes and traces read apart.
        let facade = ReferenceStyle.lineColour(.facade)
        let colours = [wall, popout, recess, facade].map { [$0.x, $0.y, $0.z] }
        #expect(Set(colours).count == 4)
        #expect(ReferenceStyle.lineColour(.wallPlane) == wall)

        #expect(ReferenceStyle.wallPlaneRole(wallPlane(role: "popout")) == .popout)
        #expect(ReferenceStyle.wallPlaneRole(wallPlane(role: "recess")) == .recess)
        #expect(ReferenceStyle.wallPlaneRole(wallPlane(role: "bay")) == .other)
        var untagged = wallPlane(role: "wall")
        untagged.tags = [:]
        #expect(ReferenceStyle.wallPlaneRole(untagged) == .other)
        #expect(ReferenceStyle.wallPlaneColour(.other) == ReferenceStyle.otherColour)
    }

    @Test func wallPlanesAreDrawnClosedWithoutAFacadeTop() {
        // A height_m on a plane adds nothing: the rectangle is the whole of it.
        let planes = [
            wallPlane(role: "wall", height: 6), wallPlane(role: "popout", offset: 0.6),
            wallPlane(role: "recess", offset: -0.3),
        ]
        let v = ReferenceStyle.wallPlaneVertices(planes)
        #expect(v.count == 3 * 4 * 2 * 7)
        let lift = ReferenceStyle.lineLift
        #expect(point(v, 0) == [11, 10, 21 + lift])
        // The closing segment runs from the last vertex back to the first.
        #expect(point(v, 6) == [11, 10, 30 + lift])
        #expect(point(v, 7) == [11, 10, 21 + lift])
        #expect((0..<(v.count / 7)).allSatisfy { point(v, $0)[2] <= 30 + lift + 1e-4 })
        #expect(colour(v, 0) == ReferenceStyle.wallPlaneColour(.wall))
        #expect(colour(v, 8) == ReferenceStyle.wallPlaneColour(.popout))
        #expect(colour(v, 16) == ReferenceStyle.wallPlaneColour(.recess))
        // A popout stands in front of the box wall: towards −y here.
        #expect(abs(point(v, 8)[1] - 9.4) < 1e-4)

        // In the overlay, planes have their own buffer, apart from the
        // observed features (which a facade trace still gets its top in).
        var g = geometry([.observed])
        g.append(observed(planes))
        let overlay = ReferenceStyle.vertices(for: g)
        #expect(overlay.wallPlaneVertexCount == 24)
        #expect(overlay.observedVertexCount == 16)

        // An open plane is drawn open, as given.
        var open = wallPlane(role: "wall")
        open.closed = false
        #expect(ReferenceStyle.wallPlaneVertices([open]).count == 3 * 2 * 7)
    }

    @Test func roleCountsReadForThePane() {
        var summary = ReferenceSummary(
            aerialPoints: 0, osmFeatures: 0, intersections: 0, osmAttribution: "",
            aerialAttribution: "")
        #expect(summary.wallPlaneRoleText == "")
        summary.wallPlaneRoles = [.wall: 3, .popout: 1, .recess: 2]
        #expect(summary.wallPlaneRoleText == "3 walls · 1 popout · 2 recesses")
        summary.wallPlaneRoles = [.wall: 1, .recess: 1, .other: 2, .popout: 0]
        #expect(summary.wallPlaneRoleText == "1 wall · 1 recess · 2 other")
    }
}

/// The renderer keeps a buffer and a toggle for each new layer.
@MainActor struct ReferenceOverlayRendererTests {
    @Test func eachNewLayerHasABufferAndAToggle() {
        guard let device = MTLCreateSystemDefaultDevice() else { return }
        let view = MTKView()
        view.device = device
        guard let renderer = MetalRenderer(metalView: view) else { return }
        var g = ReferenceGeometry()
        for layer in AlignmentSession.bundleLayers + AlignmentSession.listedOnlyLayers
            + AlignmentSession.registeredLayers
        { for message in referenceMessages(layer, modelID: "sha256:m") { g.append(message) } }
        var planes = Gerardus_Align_V1alpha1_StreamReferenceResponse()
        planes.layer = .observed
        planes.lines.features = [wallPlane(role: "wall"), wallPlane(role: "popout", offset: 0.5)]
        g.append(planes)
        renderer.setReferenceOverlay(
            ReferenceStyle.vertices(for: g, centre: SIMD3(-9.4, -38.5, 23)))
        #expect(renderer.referenceScanCount == 5)
        #expect(renderer.referenceFootprintVertexCount == 6)
        #expect(renderer.referenceBoxVertexCount == 16)
        #expect(renderer.referenceObservedVertexCount == 16)
        #expect(renderer.referenceWallPlaneVertexCount == 16)
        #expect(renderer.referenceGroundMeshVertexCount == 8)
        var shown = ReferenceVisibility()
        #expect(shown.boxes && shown.wallPlanes)
        shown.registeredScans = false
        shown.groundMesh = false
        shown.boxes = false
        shown.wallPlanes = false
        renderer.setReferenceVisibility(shown)
        #expect(renderer.referenceVisibility == shown)
        renderer.setReferenceOverlay(nil)
        #expect(renderer.referenceScanCount == 0)
        #expect(renderer.referenceBoxVertexCount == 0)
        #expect(renderer.referenceObservedVertexCount == 0)
        #expect(renderer.referenceWallPlaneVertexCount == 0)
        #expect(renderer.referenceGroundMeshVertexCount == 0)
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
            OpenCaptureEvidence(clipID: "98fe9f80", sourcePaths: []), sites: [columbus, elsewhere],
            filesByCase: [:])
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

/// The scan the fake's UploadScan names, and so the open capture's own.
private let ownScanID = "sha256:scan"

/// What the fake Gerardus streams for one layer. The registered layers are
/// shaped as SITE-MODEL-SPIKE.md §6 describes them: two accepted scans (the
/// open capture's own and another), a sensor, a correction, a facade trace
/// and a pole, and a small ground heightfield near the hint pose.
private func referenceMessages(
    _ layer: Gerardus_Align_V1alpha1_ReferenceLayer, modelID: String
) -> [Gerardus_Align_V1alpha1_StreamReferenceResponse] {
    func message(
        _ fill: (inout Gerardus_Align_V1alpha1_StreamReferenceResponse) -> Void
    ) -> Gerardus_Align_V1alpha1_StreamReferenceResponse {
        var m = Gerardus_Align_V1alpha1_StreamReferenceResponse()
        m.layer = layer
        fill(&m)
        return m
    }
    func line(
        _ kind: Gerardus_Align_V1alpha1_FeatureKind, _ xyz: [Float], height: Float = 0
    ) -> Gerardus_Align_V1alpha1_LineFeature {
        var feature = Gerardus_Align_V1alpha1_LineFeature()
        feature.kind = kind
        feature.xyz = xyz
        feature.heightM = height
        return feature
    }
    func scan(_ id: String, record: String, points: Int) -> Gerardus_Align_V1alpha1_ScanPoints {
        var block = Gerardus_Align_V1alpha1_ScanPoints()
        block.scanID = id
        block.recordID = record
        block.kind = .background
        block.captureRef = "capture/\(id)"
        block.points.x = (0..<points).map { Float($0) - 9 }
        block.points.y = (0..<points).map { _ in -38 }
        block.points.z = (0..<points).map { _ in 21 }
        return block
    }
    switch layer {
    case .aerialPoints:
        return [
            message {
                $0.points.x = [0, 1]
                $0.points.y = [0, 1]
                $0.points.z = [21, 21]
                $0.points.classification = [2, 2]
                $0.points.intensity = [10, 20]
                $0.layerComplete = true
            }
        ]
    case .osm:
        return [
            message {
                $0.lines.features = [line(.road, [0, 0, 21, 5, 0, 21])]
                $0.layerComplete = true
            }
        ]
    case .footprints:
        var outline = line(.building, [10, 10, 21, 20, 10, 21, 20, 20, 21])
        outline.closed = true
        return [
            message {
                $0.lines.features = [outline]
                $0.layerComplete = true
            }
        ]
    case .intersections: return [message { $0.layerComplete = true }]
    case .boxes:
        // The footprint's box: one street wall and one party wall, each a
        // closed vertical rectangle from the ground (21 m) to the roof.
        return [
            message {
                $0.lines.features = [
                    boxWall(
                        [10, 10, 21, 20, 10, 21, 20, 10, 33, 10, 10, 33], wall: 0, party: false),
                    boxWall(
                        [20, 10, 21, 20, 20, 21, 20, 20, 33, 20, 10, 33], wall: 1, party: true),
                ]
                $0.layerComplete = true
            }
        ]
    case .registeredScans:
        return [
            message {
                $0.scanPoints = scan(ownScanID, record: "gms:rec-own", points: 3)
                $0.modelID = modelID
            },
            message {
                $0.scanPoints = scan("sha256:other", record: "gms:rec-other", points: 2)
                $0.modelID = modelID
                $0.layerComplete = true
            },
        ]
    case .observed:
        return [
            message {
                $0.lines.features = [
                    line(.sensor, [-9.4, -38.5, 21, -9.4, -38.5, 23, -7.7, -36.0, 23]),
                    line(.correction, [-12, -40, 21.2, -9.4, -38.5, 21.2]),
                    line(.facade, [0, 5, 21, 10, 5, 21], height: 6),
                    line(.pole, [3, 3, 21, 3, 3, 27]),
                ]
                $0.modelID = modelID
                $0.layerComplete = true
            }
        ]
    case .groundMesh:
        return [
            message {
                $0.groundMesh.origin.x = -10
                $0.groundMesh.origin.y = -40
                $0.groundMesh.cellM = 1
                $0.groundMesh.nx = 3
                $0.groundMesh.ny = 2
                $0.groundMesh.z = [21, 21, 21, 21, .nan, 21]
                $0.modelID = modelID
                $0.layerComplete = true
            }
        ]
    case .cityStreets:
        // A centreline down the middle of a street running east, 10 m wide.
        return [
            message {
                var street = line(.road, [-40, 0, 21, 40, 0, 21])
                street.featureID = "datasf:streets/1"
                $0.lines.features = [street]
                $0.layerComplete = true
            }
        ]
    case .cityIntersections:
        return [
            message {
                var node = Gerardus_Align_V1alpha1_Intersection()
                node.intersectionID = "datasf:nodes/1"
                node.position.z = 21
                $0.intersections.intersections = [node]
                $0.layerComplete = true
            }
        ]
    case .kerbs:
        // The street's two kerbs, 5 m either side of the centreline.
        var north = line(.kerb, [-40, 5, 21, 40, 5, 21])
        north.featureID = "datasf:curbs/north"
        var south = line(.kerb, [-40, -5, 21, 40, -5, 21])
        south.featureID = "datasf:curbs/south"
        return [
            message {
                $0.lines.features = [north, south]
                $0.layerComplete = true
            }
        ]
    default: return featureLayerMessages(layer)
    }
}

/// One wall of a DataSF box, tagged as Gerardus tags it.
private func boxWall(_ xyz: [Float], wall: Int, party: Bool) -> Gerardus_Align_V1alpha1_LineFeature
{
    var feature = Gerardus_Align_V1alpha1_LineFeature()
    feature.featureID = "datasf:ynuv-fyni/1#\(wall)"
    feature.kind = .building
    feature.xyz = xyz
    feature.closed = true
    feature.heightM = 12
    feature.tags = [
        "building_id": "1", "wall": "\(wall)", "party": party ? "true" : "false",
        "roof_p50_m": "12.0", "roof_p95_m": "12.6", "shift_east_m": "0.3", "shift_north_m": "-0.2",
    ]
    return feature
}

/// A wall plane in front of the box's street wall: a closed vertical
/// rectangle with a role.
private func wallPlane(
    role: String, offset: Float = 0, height: Float = 0
) -> Gerardus_Align_V1alpha1_LineFeature {
    var feature = Gerardus_Align_V1alpha1_LineFeature()
    feature.kind = .wallPlane
    let y = 10 - offset
    feature.xyz = [11, y, 21, 15, y, 21, 15, y, 30, 11, y, 30]
    feature.closed = true
    feature.heightM = height
    feature.tags = [
        "role": role, "offset_m": "\(offset)", "points": "420", "building_id": "1", "wall": "0",
    ]
    return feature
}

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
    /// Every StreamReference request, in order.
    var references: [Gerardus_Align_V1alpha1_StreamReferenceRequest] = []
    /// What GetServerInfo lists: every layer, as a server run with
    /// `serve --serve-scans` would.
    var availableLayers: [Gerardus_Align_V1alpha1_ReferenceLayer] = [
        .aerialPoints, .osm, .footprints, .intersections, .registeredScans, .observed, .groundMesh,
    ]
    /// Layers StreamReference refuses with PERMISSION_DENIED.
    var refusedLayers: Set<Gerardus_Align_V1alpha1_ReferenceLayer> = []
    /// The model_id the registered layers carry.
    var servedModelID = "sha256:model-a"
    /// What GetSiteModel answers; nil is UNIMPLEMENTED, as Gerardus answers
    /// until V2 builds the site model.
    var siteModelID: String?
    /// The scans GetSiteModel lists, when it answers.
    var siteModelScans: [String] = [ownScanID, "sha256:other"]
    /// The capture_ref and sensor position GetSiteModel gives a scan, where
    /// a test sets them.
    var siteModelCaptureRefs: [String: String] = [:]
    var siteModelPositions: [String: SIMD3<Double>] = [:]
    /// Serve the registered layers empty and complete, with no model_id, as
    /// Gerardus does before it builds them.
    var emptyRegisteredLayers = false
    /// Serve the boxes layer empty and complete, as Gerardus does before it
    /// builds them.
    var emptyBoxes = false
    /// Wall planes the observed layer carries; none until Gerardus builds them.
    var wallPlanes: [Gerardus_Align_V1alpha1_LineFeature] = []
    var lastRegister: Gerardus_Align_V1alpha1_RegisterRequest?
    /// Refuse Register with against_boxes, FAILED_PRECONDITION with this
    /// message, as Gerardus refuses a site with no box walls.
    var refuseAgainstBoxes: String?
    var lastReview: Gerardus_Align_V1alpha1_ReviewRegistrationRequest?
    /// Every ReviewRegistration request, in order.
    var reviews: [Gerardus_Align_V1alpha1_ReviewRegistrationRequest] = []
    var reviewBatches: [Gerardus_Align_V1alpha1_ReviewRegistrationsRequest] = []
    var reviewFailure: RPCError?
    var streetUseFailure: RPCError?
    /// Runs whose review is refused with FAILED_PRECONDITION, as Gerardus
    /// refuses a j1 whose scan's accepted pose changed since RefineSite ran.
    var refusedReviews: Set<String> = []
    /// What RefineSite answers, the last request it got, and the runs
    /// GetRegistration answers, by id.
    var refineResult = Gerardus_Align_V1alpha1_RefineSiteResult()
    var lastRefine: Gerardus_Align_V1alpha1_RefineSiteRequest?
    var runs: [String: Gerardus_Align_V1alpha1_Registration] = [:]
    /// Called inside RefineSite before it answers, to look at the session then.
    var duringRefine: (@MainActor () -> Void)?
    /// The site's marked street use, as ListStreetUse answers it, and what
    /// EditStreetUse answers with; nil `streetUse` is UNIMPLEMENTED, as a
    /// server from before street use answers.
    var streetUse: [Gerardus_Align_V1alpha1_StreetUseFeature]? = []
    var streetUseEdits: [Gerardus_Align_V1alpha1_EditStreetUseRequest] = []
    /// The problems EditStreetUse reports, given the features after the edit.
    var streetUseProblems: ([Gerardus_Align_V1alpha1_StreetUseFeature]) -> [String] = { _ in [] }
    /// Refuse EditStreetUse with this message, as Gerardus refuses a batch
    /// it cannot apply whole.
    var refuseStreetUse: String?
    private var streetUseCounter = 0
    var uploadedBlocks: [Gerardus_Align_V1alpha1_PointBlock] = []
    /// Every UploadScan header, in order.
    var uploadedHeaders: [Gerardus_Align_V1alpha1_ScanHeader] = []
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
        info.availableLayers = availableLayers
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

    /// Answers each requested layer with the messages a server would send,
    /// gathered the way GerardusClient gathers them.
    func streamReference(
        _ request: Gerardus_Align_V1alpha1_StreamReferenceRequest
    ) async throws -> ReferenceGeometry {
        log("StreamReference")
        references.append(request)
        if request.layers.contains(where: refusedLayers.contains) {
            throw GerardusClientError.permissionDenied(
                "REGISTERED_SCANS is served only to loopback and tailnet peers")
        }
        var geometry = ReferenceGeometry()
        for layer in request.layers {
            if (emptyRegisteredLayers && ReferenceGeometry.registeredLayers.contains(layer))
                || (emptyBoxes && layer == .boxes)
            {
                var empty = Gerardus_Align_V1alpha1_StreamReferenceResponse()
                empty.layer = layer
                empty.layerComplete = true
                geometry.append(empty)
                continue
            }
            for message in referenceMessages(layer, modelID: servedModelID) {
                geometry.append(message)
            }
            if layer == .observed, !wallPlanes.isEmpty {
                var planes = Gerardus_Align_V1alpha1_StreamReferenceResponse()
                planes.layer = .observed
                planes.lines.features = wallPlanes
                planes.modelID = servedModelID
                geometry.append(planes)
            }
        }
        return geometry
    }

    func getSiteModel(
        l16Token: String
    ) async throws -> Gerardus_Align_V1alpha1_GetSiteModelResponse? {
        log("GetSiteModel")
        guard let siteModelID else { return nil }
        var response = Gerardus_Align_V1alpha1_GetSiteModelResponse()
        response.modelID = siteModelID
        response.scans = siteModelScans.map { id in
            var scan = Gerardus_Align_V1alpha1_ModelScan()
            scan.scanID = id
            scan.state = .registered
            if let ref = siteModelCaptureRefs[id] { scan.captureRef = ref }
            if let at = siteModelPositions[id] {
                scan.pose = levelPose(headingDeg: 0, at: at).proto(
                    siteFrameID: "site", sensorFrameID: "sensor", source: .manual)
            }
            return scan
        }
        return response
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
        uploadedHeaders.append(header)
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
        if request.againstBoxes, let refuseAgainstBoxes {
            throw RPCError(code: .failedPrecondition, message: refuseAgainstBoxes)
        }
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
        registration.scanID = request.scanID
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
        reviews.append(request)
        if let reviewFailure { throw reviewFailure }
        if refusedReviews.contains(request.registrationID) {
            throw RPCError(
                code: .failedPrecondition,
                message: "the scan's accepted pose has changed since this refinement ran")
        }
        var record = Gerardus_Align_V1alpha1_RegistrationRecord()
        record.recordID = "gms:rec\(records.count)"
        // The scan the run registered, as Gerardus records it.
        record.scanID =
            runs[request.registrationID]?.scanID
            ?? (request.registrationID == "gms:reg" ? lastRegister?.scanID ?? "" : "")
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
        return records.reversed()
    }

    func reviewRegistrations(
        _ request: Gerardus_Align_V1alpha1_ReviewRegistrationsRequest
    ) async throws -> [Gerardus_Align_V1alpha1_RegistrationRecord] {
        log("ReviewRegistrations")
        reviewBatches.append(request)
        if let reviewFailure { throw reviewFailure }
        guard !request.reviews.contains(where: { refusedReviews.contains($0.registrationID) })
        else {
            throw RPCError(code: .aborted, message: "a refinement dependency changed; refine again")
        }
        var accepted: [Gerardus_Align_V1alpha1_RegistrationRecord] = []
        for review in request.reviews { accepted.append(try await reviewRegistration(review)) }
        return accepted
    }

    func getRegistration(id: String) async throws -> Gerardus_Align_V1alpha1_Registration {
        log("GetRegistration")
        guard let run = runs[id] else { throw GerardusClientError.incompleteStream("no run \(id)") }
        return run
    }

    func listStreetUse(l16Token: String) async throws -> [Gerardus_Align_V1alpha1_StreetUseFeature]
    {
        log("ListStreetUse")
        guard let streetUse else { throw RPCError(code: .unimplemented, message: "ListStreetUse") }
        return streetUse
    }

    /// Applies the edits as Gerardus would, whole or not at all: removes,
    /// then upserts, a new feature given an id. Each vertex is snapped to a
    /// 0.1 m grid and draped at 21 m, so what comes back is not quite what
    /// was sent.
    func editStreetUse(
        _ request: Gerardus_Align_V1alpha1_EditStreetUseRequest
    ) async throws -> Gerardus_Align_V1alpha1_EditStreetUseResponse {
        log("EditStreetUse")
        streetUseEdits.append(request)
        if let streetUseFailure { throw streetUseFailure }
        guard !request.author.isEmpty else {
            throw RPCError(code: .invalidArgument, message: "author is required")
        }
        if let refuseStreetUse {
            throw RPCError(code: .failedPrecondition, message: refuseStreetUse)
        }
        var features = streetUse ?? []
        for edit in request.edits {
            switch edit.change {
            case .revertRowID:
                throw RPCError(
                    code: .unimplemented, message: "server-history undo is outside this fake")
            case .removeFeatureID(let id): features.removeAll { $0.featureID == id }
            case .upsert(var feature):
                if feature.featureID.isEmpty {
                    streetUseCounter += 1
                    feature.featureID = "gms:01NEW\(streetUseCounter)"
                }
                var snapped = [Float]()
                for i in stride(from: 0, to: feature.xyz.count - 2, by: 3) {
                    let x: Float = (feature.xyz[i] * 10).rounded() / 10
                    let y: Float = (feature.xyz[i + 1] * 10).rounded() / 10
                    snapped.append(contentsOf: [x, y, 21])
                }
                feature.xyz = snapped
                feature.author = request.author
                feature.rowID = "row-\(feature.featureID)-\(streetUseEdits.count)"
                if let i = features.firstIndex(where: { $0.featureID == feature.featureID }) {
                    features[i] = feature
                } else {
                    features.append(feature)
                }
            case nil: break
            }
        }
        streetUse = features
        var response = Gerardus_Align_V1alpha1_EditStreetUseResponse()
        response.features = features
        response.problems = streetUseProblems(features)
        return response
    }

    func refineSite(
        _ request: Gerardus_Align_V1alpha1_RefineSiteRequest,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_RefineSiteResult {
        log("RefineSite")
        lastRefine = request
        var progress = Gerardus_Align_V1alpha1_Progress()
        progress.stage = "solve"
        progress.fraction = -1
        await onProgress(progress)
        await duringRefine?()
        return refineResult
    }
}

/// Records what the session hands the renderer.
final class FakeOverlaySink: ReferenceOverlaySink {
    var overlays: [ReferenceOverlayVertices?] = []
    var matrices: [simd_float4x4] = []
    var visibility: ReferenceVisibility?

    func setReferenceOverlay(_ vertices: ReferenceOverlayVertices?) { overlays.append(vertices) }
    func setReferenceModelMatrix(_ matrix: simd_float4x4) { matrices.append(matrix) }
    func setReferenceVisibility(_ visibility: ReferenceVisibility) { self.visibility = visibility }
    var residuals: [[Float]?] = []
    func setResidualOverlay(_ vertices: [Float]?, palette: [SIMD4<Float>]) {
        residuals.append(vertices)
    }
    /// Every point the camera was asked to frame, in the site frame.
    var focuses: [SIMD3<Double>] = []
    func focusCamera(onSitePoint point: SIMD3<Double>) { focuses.append(point) }
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

        // A transport alone is not connected: every retry makes one.
        app.isConnected = true
        #expect(session.probing == nil)
        // The service answering starts the loop and disconnecting stops it,
        // before it has asked anything.
        app.streamDidOpen(client: nil)
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
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
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
        // The bundle's layers on one call, then each registered layer on its own.
        #expect(
            service.calls == [
                "GetServerInfo", "PrepareSite", "PoseFromHint", "StreamReference",
                "StreamReference", "StreamReference", "StreamReference", "GetSiteModel",
                "ListRegistrationRecords",
            ])
        let hint = service.lastHint
        #expect(hint?.l16Token == "808580f47")
        #expect(hint?.hint.position.latitudeDeg == 37.7978)
        #expect(hint?.hint.position.longitudeDeg == -122.4065)
        // north_azimuth_deg 35 at columbus-broadway: the hint heading is 180 − 35.
        #expect(hint?.hint.hasHeadingDeg == true && hint?.hint.headingDeg == 145)
        let reference = service.references.first
        #expect(reference?.radiusM == 150)
        #expect(reference?.voxelSizeM == 0)
        #expect(reference?.centre.x == service.hintPose.translation.x)
        #expect(reference?.centre.y == service.hintPose.translation.y)
        #expect(reference?.layers == [.aerialPoints, .osm, .footprints, .intersections])
        #expect(
            service.references.dropFirst().map(\.layers) == [
                [.registeredScans], [.observed], [.groundMesh],
            ])
        #expect(service.references.allSatisfy { $0.radiusM == 150 })

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
        #expect(review?.hasExpectedRecordID == true && review?.expectedRecordID == "")
        #expect(review?.requestID.isEmpty == false)
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

    @Test func aLostReviewResponseRetriesItsIDAndPrecondition() async {
        let (session, service, _) = await placed()
        session.noteBackground(snapshot(count: 10))
        await session.register()
        session.reviewer = "operator"
        service.reviewFailure = RPCError(code: .unavailable, message: "lost response")
        await session.accept()
        #expect(session.lastRecord == nil)
        service.reviewFailure = nil
        await session.accept()
        #expect(service.reviews.count == 2)
        #expect(service.reviews[0] == service.reviews[1])
        #expect(!service.reviews[0].requestID.isEmpty)
    }

    @Test func expectedHeadsFollowAppendOrderEvenIfTheClockMovesBackwards() async {
        let (session, service, _) = await placed()
        session.noteBackground(snapshot(count: 10))
        await session.register()
        var old = Gerardus_Align_V1alpha1_RegistrationRecord()
        old.scanID = session.registration!.scanID
        old.recordID = "old"
        old.reviewedAt.seconds = 100
        var head = old
        head.recordID = "head"
        head.reviewedAt.seconds = 50
        service.records = [old, head]
        await session.loadRecords()
        session.reviewer = "operator"
        await session.accept()
        #expect(service.lastReview?.expectedRecordID == "head")
    }

    @Test func aStaleReviewKeepsItsPoseAndNoteUntilAnExplicitRead() async {
        let (session, service, _) = await placed()
        session.noteBackground(snapshot(count: 10))
        await session.register()
        session.reviewer = "operator"
        session.note = "keep this draft"
        let pose = session.pose
        service.reviewFailure = RPCError(code: .aborted, message: "newer head")
        await session.accept()
        #expect(session.reviewNeedsReload)
        #expect(session.note == "keep this draft")
        #expect(session.pose?.matrix == pose?.matrix)
        await session.accept()
        #expect(service.reviews.count == 1)
        service.reviewFailure = nil
        await session.loadRecords()
        #expect(!session.reviewNeedsReload)
        await session.accept()
        #expect(service.reviews.count == 2)
        #expect(service.reviews[0].requestID != service.reviews[1].requestID)
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

// MARK: - Site model in the session

@MainActor struct AlignmentSiteModelTests {
    private let site = AlignmentSite(
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
        description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

    private func placed(
        _ configure: (FakeAlignmentService) -> Void = { _ in }
    ) async -> (AlignmentSession, FakeAlignmentService, FakeOverlaySink) {
        let service = FakeAlignmentService()
        configure(service)
        let sink = FakeOverlaySink()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        session.sink = sink
        await session.probe()
        session.choose(site)
        session.northAzimuthText = "35"
        await session.placeFromHint()
        return (session, service, sink)
    }

    /// Placed, with the background uploaded: the open capture's scan is
    /// then known by its id.
    private func registered(
        _ configure: (FakeAlignmentService) -> Void = { _ in }
    ) async -> (AlignmentSession, FakeAlignmentService, FakeOverlaySink) {
        let (session, service, sink) = await placed(configure)
        session.noteBackground(snapshot(count: 10))
        await session.register()
        return (session, service, sink)
    }

    @Test func theRegisteredLayersAreDrawnAndCounted() async throws {
        let (session, _, sink) = await placed()
        let summary = try #require(session.reference)
        #expect(summary.footprints == 1)
        #expect(summary.registeredScans == 2)
        // The open capture's scan is not known before its upload.
        #expect(summary.registeredPoints == 5)
        #expect(summary.ownScanPoints == 0)
        #expect(summary.observedFeatures == 4)
        #expect(summary.groundMeshCells == 5)
        #expect(summary.modelID == "sha256:model-a")
        #expect(session.registeredLayersNote == nil)
        #expect(session.errorMessage == nil)

        let overlay = try #require(sink.overlays.last ?? nil)
        #expect(overlay.scanCount == 5)
        #expect(overlay.footprintVertexCount == 6)
        #expect(overlay.observedVertexCount == 16)
        // The ground mesh lies within 40 m of the sensor: drawn cell by cell.
        #expect(overlay.groundMeshVertexCount == 8)

        #expect(sink.visibility == ReferenceVisibility())
        session.showGroundMesh = false
        session.showFootprints = false
        #expect(sink.visibility?.groundMesh == false)
        #expect(sink.visibility?.footprints == false)
        #expect(sink.visibility?.registeredScans == true)
        session.showRegisteredScans = false
        session.showObserved = false
        #expect(sink.visibility?.registeredScans == false)
        #expect(sink.visibility?.observed == false)
    }

    @Test func theOpenCapturesOwnScanIsHiddenUntilAskedFor() async {
        let (session, _, sink) = await registered()
        #expect(session.scan?.scanID == ownScanID)
        #expect(session.reference?.ownScanPoints == 3)
        #expect(session.reference?.registeredPoints == 2)
        #expect(sink.overlays.last??.scanCount == 2)

        session.showOwnScan = true
        await session.pendingStyling?.value
        #expect(sink.overlays.last??.scanCount == 5)
        #expect(session.reference?.registeredPoints == 5)

        session.showOwnScan = false
        await session.pendingStyling?.value
        #expect(sink.overlays.last??.scanCount == 2)

        // Another capture opens: the scan is no longer this capture's own.
        session.captureChanged()
        await session.pendingStyling?.value
        #expect(sink.overlays.last??.scanCount == 5)
        #expect(session.reference?.ownScanPoints == 0)
    }

    @Test func refusedScansLeaveEveryOtherLayerDrawn() async {
        let (session, service, sink) = await placed { $0.refusedLayers = [.registeredScans] }
        #expect(session.errorMessage == nil)
        #expect(session.reference?.aerialPoints == 2)
        #expect(session.reference?.registeredScans == 0)
        #expect(session.reference?.observedFeatures == 4)
        #expect(session.reference?.groundMeshCells == 5)
        #expect(session.reference?.modelID == "sha256:model-a")
        #expect(session.registeredLayersNote?.contains("--serve-scans") == true)
        #expect(sink.overlays.last??.scanCount == 0)
        #expect(sink.overlays.last??.aerialCount == 2)
        // Placement carried on to the records.
        #expect(service.calls.last == "ListRegistrationRecords")
    }

    @Test func anotherFailureIsNamedWithItsLayer() {
        let note = AlignmentSession.note(
            layer: .groundMesh, error: GerardusClientError.incompleteStream("StreamReference"))
        #expect(note == "Ground mesh not drawn: StreamReference ended without its result")
    }

    @Test func emptyRegisteredLayersAreNotAnError() async {
        let (session, _, sink) = await placed { $0.emptyRegisteredLayers = true }
        #expect(session.errorMessage == nil)
        #expect(session.registeredLayersNote == nil)
        #expect(session.reference?.registeredScans == 0)
        #expect(session.reference?.observedFeatures == 0)
        #expect(session.reference?.groundMeshCells == 0)
        #expect(session.reference?.modelID == "")
        #expect(session.reference?.aerialPoints == 2)
        #expect(sink.overlays.last??.scanCount == 0)
    }

    @Test func aServerWithoutTheSiteModelIsAskedForTheBundleOnly() async {
        let (session, service, _) = await registered {
            $0.availableLayers = [.aerialPoints, .osm, .intersections]
        }
        #expect(service.references.map(\.layers) == [[.aerialPoints, .osm, .intersections]])
        #expect(session.reference?.registeredScans == 0)
        #expect(session.registeredLayersNote == nil)
        // Nor is the site model asked about after an accept.
        await session.accept()
        #expect(session.lastRecord != nil)
        #expect(!service.calls.contains("GetSiteModel"))
        #expect(service.references.count == 1)
    }

    @Test func anAcceptReStreamsTheRegisteredLayersWhenTheModelMoves() async {
        let (session, service, sink) = await registered { $0.siteModelID = "sha256:model-a" }
        let before = service.references.count
        service.servedModelID = "sha256:model-b"
        service.siteModelID = "sha256:model-b"
        await session.accept()
        #expect(
            Array(service.calls.suffix(6)) == [
                "ReviewRegistration", "ListRegistrationRecords", "GetSiteModel", "StreamReference",
                "StreamReference", "StreamReference",
            ])
        #expect(
            service.references.dropFirst(before).map(\.layers) == [
                [.registeredScans], [.observed], [.groundMesh],
            ])
        #expect(session.reference?.modelID == "sha256:model-b")
        // The bundle's layers were not asked for again, and are still drawn,
        // and the open capture's copy is still hidden.
        #expect(session.reference?.aerialPoints == 2)
        #expect(sink.overlays.last??.aerialCount == 2)
        #expect(sink.overlays.last??.scanCount == 2)
        #expect(session.busy == nil)

        // A second decision on a model that has not moved streams nothing.
        let streamed = service.references.count
        await session.accept()
        #expect(service.calls.last == "GetSiteModel")
        #expect(service.references.count == streamed)
    }

    @Test func anUnchangedModelIsNotReStreamed() async {
        let (session, service, _) = await registered { $0.siteModelID = "sha256:model-a" }
        let before = service.references.count
        await session.accept()
        #expect(service.calls.last == "GetSiteModel")
        #expect(service.references.count == before)
        #expect(session.reference?.modelID == "sha256:model-a")
    }

    @Test func anUnimplementedSiteModelStillReStreamsAfterAnAccept() async {
        // Today's Gerardus: GetSiteModel answers UNIMPLEMENTED.
        let (session, service, _) = await registered()
        let before = service.references.count
        service.servedModelID = "sha256:model-b"
        await session.accept()
        #expect(service.calls.contains("GetSiteModel"))
        #expect(service.references.count == before + 3)
        #expect(session.reference?.modelID == "sha256:model-b")
        #expect(session.errorMessage == nil)
    }

    @Test func aRejectionAlsoAsksWhetherTheModelMoved() async {
        let (session, service, _) = await registered { $0.siteModelID = "sha256:model-a" }
        session.rejectReason = "wrong corner"
        await session.reject()
        #expect(service.calls.last == "GetSiteModel")
    }

    @Test func theModelIDShortensForThePane() {
        #expect(AlignmentFormat.shortID("sha256:0123456789abcdef") == "sha256:0123456789ab")
        #expect(AlignmentFormat.shortID("plain-id-that-is-long") == "plain-id-tha")
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
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
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
    func getSiteModel(
        l16Token: String
    ) async throws -> Gerardus_Align_V1alpha1_GetSiteModelResponse? {
        try await inner.getSiteModel(l16Token: l16Token)
    }
    func getRegistration(id: String) async throws -> Gerardus_Align_V1alpha1_Registration {
        try await inner.getRegistration(id: id)
    }
    func refineSite(
        _ request: Gerardus_Align_V1alpha1_RefineSiteRequest,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_RefineSiteResult {
        try await inner.refineSite(request, onProgress: onProgress)
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

// MARK: - Registering against the site model (spike V5)

private let columbusSite = AlignmentSite(
    clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
    description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
    sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

/// A session placed at columbus-broadway against a fake configured first.
@MainActor private func placedSession(
    fullFrames: ReplayFullFramesSwitch? = FakeFullFrames(),
    _ configure: (FakeAlignmentService) -> Void = { _ in }
) async -> (AlignmentSession, FakeAlignmentService) {
    let service = FakeAlignmentService()
    configure(service)
    let session = AlignmentSession(
        makeClient: { _ in service }, lookup: nil, defaults: nil, fullFrames: fullFrames)
    session.sink = FakeOverlaySink()
    // Nothing is scored in these tests unless they ask.
    session.evaluateSleep = { _ in try await Task.sleep(for: .seconds(3600)) }
    await session.probe()
    session.choose(columbusSite)
    session.northAzimuthText = "35"
    await session.placeFromHint()
    return (session, service)
}

@MainActor struct AgainstSiteModelTests {
    @Test func theSwitchIsOffByDefaultAndSetsTheRequestField() async {
        let (session, service) = await placedSession()
        session.noteBackground(snapshot(count: 10))
        #expect(!session.againstSiteModel)
        // The fake lists every layer and its model holds another scan.
        #expect(session.againstSiteModelUnavailable == nil)
        await session.register()
        #expect(service.lastRegister?.againstSiteModel == false)

        session.againstSiteModel = true
        await session.register()
        #expect(service.lastRegister?.againstSiteModel == true)
        #expect(session.registersAgainstSiteModel)
    }

    @Test func aServerWithoutTheScansLayerDisablesIt() async {
        let (session, service) = await placedSession {
            $0.availableLayers = [.aerialPoints, .osm, .footprints, .intersections, .observed]
        }
        session.noteBackground(snapshot(count: 10))
        session.againstSiteModel = true
        #expect(session.againstSiteModelUnavailable?.contains("registered-scans") == true)
        #expect(!session.registersAgainstSiteModel)
        await session.register()
        #expect(service.lastRegister?.againstSiteModel == false)
    }

    @Test func aModelHoldingOnlyThisScanDisablesItOnceTheScanIsKnown() async {
        let (session, service) = await placedSession {
            $0.siteModelID = "sha256:model-a"
            $0.siteModelScans = [ownScanID]
        }
        #expect(session.siteModelScanIDs == [ownScanID])
        session.noteBackground(snapshot(count: 10))
        session.againstSiteModel = true
        // Before the upload the scan's id is not known.
        #expect(session.againstSiteModelUnavailable == nil)
        await session.register()
        // Decided after the upload: the model holds nothing but this scan.
        #expect(service.lastRegister?.againstSiteModel == false)
        #expect(session.againstSiteModelUnavailable?.contains("besides this one") == true)
    }

    @Test func anEmptyModelDisablesIt() async {
        let (session, _) = await placedSession {
            $0.siteModelID = "sha256:model-a"
            $0.siteModelScans = []
        }
        #expect(session.againstSiteModelUnavailable?.contains("no accepted scan yet") == true)
    }

    @Test func getSiteModelsListWinsOverTheStreamedScans() async {
        let (session, _) = await placedSession {
            $0.siteModelID = "sha256:model-a"
            $0.siteModelScans = ["sha256:k1", "sha256:k2"]
        }
        #expect(session.siteModelScanIDs == ["sha256:k1", "sha256:k2"])
        #expect(session.againstSiteModelUnavailable == nil)
    }

    @Test func todaysServerFallsBackToTheStreamedScans() async {
        // GetSiteModel UNIMPLEMENTED: the registered layer names the scans.
        let (session, service) = await placedSession()
        #expect(service.calls.contains("GetSiteModel"))
        #expect(session.siteModelScanIDs == [ownScanID, "sha256:other"])
    }

    @Test func unknownScansDisableIt() async {
        // The scans layer refused, and GetSiteModel unanswered.
        let (session, _) = await placedSession { $0.refusedLayers = [.registeredScans] }
        #expect(session.siteModelScanIDs == nil)
        #expect(session.againstSiteModelUnavailable?.contains("has not said") == true)
    }

    @Test func anAcceptUpdatesTheModelsScans() async {
        let (session, service) = await placedSession {
            $0.siteModelID = "sha256:model-a"
            $0.siteModelScans = [ownScanID]
        }
        session.noteBackground(snapshot(count: 10))
        await session.register()
        #expect(session.againstSiteModelUnavailable != nil)
        service.siteModelID = "sha256:model-b"
        service.siteModelScans = [ownScanID, "sha256:k2"]
        session.reviewer = "operator"
        await session.accept()
        #expect(session.siteModelScanIDs == [ownScanID, "sha256:k2"])
        #expect(session.againstSiteModelUnavailable == nil)
    }
}

// MARK: - Against the DataSF boxes (Gerardus 4e90c8c, q7)

@MainActor struct AgainstBoxesTests {
    /// Every layer the default fake lists, and BOXES.
    private static let withBoxes: [Gerardus_Align_V1alpha1_ReferenceLayer] = [
        .aerialPoints, .osm, .footprints, .intersections, .registeredScans, .observed, .groundMesh,
        .boxes,
    ]

    @Test func theSwitchIsOffByDefaultAndSetsTheRequestField() async {
        let (session, service) = await placedSession { $0.availableLayers = Self.withBoxes }
        session.noteBackground(snapshot(count: 10))
        #expect(!session.againstBoxes)
        // The fake serves two box walls.
        #expect(session.reference?.boxWalls == 2)
        #expect(session.againstBoxesUnavailable == nil)
        await session.register()
        #expect(service.lastRegister?.againstBoxes == false)
        #expect(session.registeredAgainst == AlignmentModelReference())

        session.againstBoxes = true
        await session.register()
        #expect(service.lastRegister?.againstBoxes == true)
        #expect(service.lastRegister?.againstSiteModel == false)
        #expect(session.registersAgainstBoxes)
        #expect(session.registeredAgainst == AlignmentModelReference(siteModel: false, boxes: true))
    }

    @Test func aServerThatDoesNotListBoxesDisablesIt() async {
        // The default fake lists every layer but BOXES.
        let (session, service) = await placedSession()
        session.noteBackground(snapshot(count: 10))
        session.againstBoxes = true
        #expect(session.againstBoxesUnavailable?.contains("no boxes layer") == true)
        #expect(!session.registersAgainstBoxes)
        await session.register()
        #expect(service.lastRegister?.againstBoxes == false)

        // Nor a server that lists no layers at all: BOXES must be listed.
        let (quiet, _) = await placedSession { $0.availableLayers = [] }
        #expect(quiet.againstBoxesUnavailable?.contains("no boxes layer") == true)
    }

    @Test func aBoxesLayerWithNoWallsDisablesIt() async {
        let (session, service) = await placedSession {
            $0.availableLayers = Self.withBoxes
            $0.emptyBoxes = true
        }
        session.noteBackground(snapshot(count: 10))
        session.againstBoxes = true
        #expect(session.reference?.boxesServed == true)
        #expect(session.againstBoxesUnavailable?.contains("holds no walls") == true)
        await session.register()
        #expect(service.lastRegister?.againstBoxes == false)
    }

    @Test func itIsDisabledUntilTheReferenceIsStreamed() async {
        let service = FakeAlignmentService()
        service.availableLayers = Self.withBoxes
        let session = AlignmentSession(
            makeClient: { _ in service }, lookup: nil, defaults: nil, fullFrames: FakeFullFrames())
        session.sink = FakeOverlaySink()
        await session.probe()
        #expect(session.againstBoxesUnavailable?.contains("not been streamed") == true)
    }

    @Test func bothSwitchesCanBeOnTogether() async {
        let (session, service) = await placedSession { $0.availableLayers = Self.withBoxes }
        session.noteBackground(snapshot(count: 10))
        session.againstSiteModel = true
        session.againstBoxes = true
        #expect(session.againstSiteModelUnavailable == nil)
        #expect(session.againstBoxesUnavailable == nil)
        await session.register()
        #expect(service.lastRegister?.againstSiteModel == true)
        #expect(service.lastRegister?.againstBoxes == true)
        #expect(session.registeredAgainst == AlignmentModelReference(siteModel: true, boxes: true))

        // Turning one off leaves the other sent.
        session.againstSiteModel = false
        await session.register()
        #expect(service.lastRegister?.againstSiteModel == false)
        #expect(service.lastRegister?.againstBoxes == true)
    }

    @Test func theCaptionNamesWhatTheRunFittedAgainst() {
        let siteModel = AlignmentModelReference(siteModel: true, boxes: false)
        let boxes = AlignmentModelReference(siteModel: false, boxes: true)
        let both = AlignmentModelReference(siteModel: true, boxes: true)
        #expect(
            AlignmentFormat.modelFitCaption(siteModel)
                == "Against the site model, reported, not gated: ")
        #expect(
            AlignmentFormat.modelFitCaption(boxes)
                == "Against the DataSF boxes, reported, not gated: ")
        #expect(
            AlignmentFormat.modelFitCaption(both)
                == "Against the site model and boxes, reported, not gated: ")

        var q = Gerardus_Align_V1alpha1_FitQuality()
        q.modelP50M = 0.04
        q.modelP75M = 0.07
        q.modelPoints = 800
        #expect(
            AlignmentFormat.modelFit(q, against: siteModel)
                == "site model p50 0.04 m · p75 0.07 m · 800 points")
        #expect(
            AlignmentFormat.modelFit(q, against: boxes)
                == "boxes p50 0.04 m · p75 0.07 m · 800 points")
        #expect(
            AlignmentFormat.modelFit(q, against: both)
                == "site model and boxes p50 0.04 m · p75 0.07 m · 800 points")
        q.modelPoints = 0
        #expect(AlignmentFormat.modelFit(q, against: boxes) == nil)
    }

    @Test func aFailedPreconditionShowsTheServersMessage() async {
        let (session, service) = await placedSession {
            $0.availableLayers = Self.withBoxes
            $0.refuseAgainstBoxes = "site 808580f47 has no box walls"
        }
        session.noteBackground(snapshot(count: 10))
        session.againstBoxes = true
        await session.register()
        #expect(service.lastRegister?.againstBoxes == true)
        let message = session.errorMessage ?? ""
        #expect(message.contains("site 808580f47 has no box walls"), "\(message)")
        #expect(session.registration == nil)
        #expect(!session.isRegistering)
        #expect(session.busy == nil)
    }
}

@MainActor struct ModelResidualTests {
    private func quality(points: UInt32) -> Gerardus_Align_V1alpha1_FitQuality {
        var q = Gerardus_Align_V1alpha1_FitQuality()
        q.groundP95M = 0.12
        q.facadeP75M = 0.15
        q.modelP50M = 0.04
        q.modelP75M = 0.07
        q.modelPoints = points
        return q
    }

    @Test func theyAreFormattedOnlyWhenTheFitUsedTheModel() {
        #expect(AlignmentFormat.modelFit(quality(points: 0)) == nil)
        #expect(
            AlignmentFormat.modelFit(quality(points: 12_345))
                == "site model p50 0.04 m · p75 0.07 m · 12345 points")
        // The 3DEP line is unchanged by them.
        #expect(!AlignmentFormat.quality(quality(points: 12_345)).contains("model"))
    }

    @Test func theSessionReportsThemForTheBestHypothesisOnly() async {
        let (session, service) = await placedSession()
        var best = Gerardus_Align_V1alpha1_Hypothesis()
        best.hypothesisID = "h1"
        best.pose = service.hintPose.proto(
            siteFrameID: "site/808580f47", sensorFrameID: "sensor/hesai-pandar40p", source: .icp)
        best.quality = quality(points: 0)
        var second = best
        second.hypothesisID = "h2"
        second.quality = quality(points: 900)
        service.streamed = [best, second]
        service.runState = .candidate
        service.gates = [gate("ground_p95_m", passed: true)]
        session.noteBackground(snapshot(count: 10))
        await session.register()
        // model_points 0 on the best: nothing to report.
        #expect(session.modelFit == nil)

        best.quality = quality(points: 1_200)
        service.streamed = [best, second]
        await session.register()
        #expect(session.modelFit?.modelPoints == 1_200)
        #expect(session.modelFit?.modelP75M == 0.07)
        // Reported, not gated: the verdict is the gates' alone.
        #expect(session.registrationPassed)
        service.gates = [gate("ground_p95_m", passed: false)]
        await session.register()
        #expect(session.modelFit != nil)
        #expect(!session.registrationPassed)
    }
}

// MARK: - Accumulated frames (spike V5)

/// One replayed frame: `classes` per point, at x = `x0` + 0.1 m per point.
private func cloud(
    classes: [UInt8], x0: Float = 0, mode: DecimationMode = .none, ratio: Float = 0
) -> PointCloudFrame {
    var pc = PointCloudFrame()
    pc.timestampNanos = 1_788_000_000_000_000_000
    pc.x = classes.indices.map { x0 + Float($0) * 0.1 }
    pc.y = classes.map { _ in 2 }
    pc.z = classes.map { _ in -1.5 }
    pc.intensity = classes.map { _ in 40 }
    pc.classification = classes
    pc.decimationMode = mode
    pc.decimationRatio = ratio
    pc.pointCount = classes.count
    return pc
}

private func frame(
    _ cloud: PointCloudFrame, index: UInt64, source: SourceMode = .pcap
) -> FrameBundle {
    var bundle = FrameBundle()
    bundle.frameID = 9_000 + index
    bundle.frameType = .full
    bundle.pointCloud = cloud
    var playback = PlaybackInfo()
    playback.sourceMode = source
    playback.currentFrameIndex = index
    playback.totalFrames = 5_000
    playback.seekable = source == .vrlog
    bundle.playbackInfo = playback
    return bundle
}

struct FrameAccumulatorTests {
    @Test func onlyBackgroundAndGroundAreKept() {
        var acc = FrameAccumulator(frameCount: 2)
        #expect(acc.add(cloud(classes: [0, 1, 2, 1, 0, 3]), frameNumber: 10) == .taken)
        // Points 0, 2 and 4: background, ground, background. Foreground (1)
        // and anything else never are.
        #expect(acc.x == [0, 0.2, 0.4].map { Float($0) })
        #expect(acc.pointsKept == 3)
        #expect(!acc.isComplete)
        #expect(acc.add(cloud(classes: [1, 1, 1]), frameNumber: 11) == .taken)
        #expect(acc.pointsKept == 3)
        #expect(acc.isComplete)
        #expect(acc.captureRef(run: "run-7") == "run-7#f10-11")
    }

    @Test func decimatedFramesAreRefused() {
        let full = [
            cloud(classes: [0], mode: .none, ratio: 0), cloud(classes: [0], mode: .none, ratio: 1),
            cloud(classes: [0], mode: .uniform, ratio: 1),
        ]
        for pc in full { #expect(FrameAccumulation.refusal(pc) == nil) }
        let decimated = [
            cloud(classes: [1], mode: .foregroundOnly, ratio: 0),
            cloud(classes: [0], mode: .uniform, ratio: 0.5),
            cloud(classes: [0], mode: .voxel, ratio: 1),
            cloud(classes: [0], mode: .none, ratio: 0.25),
        ]
        for pc in decimated {
            #expect(FrameAccumulation.refusal(pc) != nil)
            var acc = FrameAccumulator(frameCount: 1)
            guard case .refused = acc.add(pc, frameNumber: 1) else {
                Issue.record("a decimated frame was taken")
                continue
            }
            #expect(acc.pointsKept == 0)
        }
        #expect(
            FrameAccumulation.refusal(cloud(classes: [1], mode: .foregroundOnly))?.contains(
                "foreground points only") == true)
    }

    @Test func pointsWithoutClassesAreRefused() {
        var pc = cloud(classes: [0, 0])
        pc.classification = []
        #expect(FrameAccumulation.refusal(pc)?.contains("no classes") == true)
    }

    @Test func aRepeatedFrameIsSkippedAndAJumpBackRefused() {
        var acc = FrameAccumulator(frameCount: 5)
        #expect(acc.add(cloud(classes: [0]), frameNumber: 40) == .taken)
        #expect(acc.add(cloud(classes: [0]), frameNumber: 40) == .skipped)
        #expect(acc.framesTaken == 1)
        guard case .refused = acc.add(cloud(classes: [0]), frameNumber: 12) else {
            Issue.record("a jump back was taken")
            return
        }
    }

    @Test func thinningKeepsOneCentroidPerFiveCentimetreVoxel() {
        var pc = cloud(classes: [0, 0, 2, 0, 0])
        pc.x = [0.01, 0.03, 0.06, -0.01, 0.01]
        pc.y = [0.01, 0.03, 0.01, 0.01, 0.01]
        pc.z = [0.01, 0.03, 0.01, 0.01, 0.06]
        var acc = FrameAccumulator(frameCount: 1)
        #expect(acc.add(pc, frameNumber: 1) == .taken)
        let thinned = FrameAccumulation.thinned(acc)
        #expect(FrameAccumulation.voxelM == 0.05)
        // (0.01, 0.01, 0.01) and (0.03, 0.03, 0.03) share a voxel; the rest
        // each have their own, across the zero boundary too.
        #expect(thinned.pointCount == 4)
        let points = (0..<thinned.pointCount).map {
            SIMD3(thinned.x[$0], thinned.y[$0], thinned.z[$0])
        }
        #expect(points.contains { simd_length($0 - SIMD3(0.02, 0.02, 0.02)) < 1e-6 })
        #expect(points.contains { simd_length($0 - SIMD3(-0.01, 0.01, 0.01)) < 1e-6 })
        #expect(points.contains { simd_length($0 - SIMD3(0.06, 0.01, 0.01)) < 1e-6 })
        #expect(points.contains { simd_length($0 - SIMD3(0.01, 0.01, 0.06)) < 1e-6 })
        // Weighted alike: no confidence. And repeatable.
        #expect(thinned.confidence.isEmpty)
        #expect(FrameAccumulation.thinned(acc).x == thinned.x)
        #expect(thinned.timestampNanos == pc.timestampNanos)
    }

    @Test func manyFramesOfAStillSceneThinToOneFrame() {
        var acc = FrameAccumulator(frameCount: 50)
        for n in 0..<50 {
            // Each point 0.02 m into its voxel, with range noise that keeps
            // it there.
            var pc = cloud(classes: Array(repeating: 0, count: 100), x0: 0.02)
            pc.x = pc.x.map { $0 + 0.003 * Float(n % 5) }
            #expect(acc.add(pc, frameNumber: UInt64(n)) == .taken)
        }
        #expect(acc.pointsKept == 5_000)
        #expect(FrameAccumulation.thinned(acc).pointCount == 100)
    }

    @Test func theAccumulatedHeaderNamesItsKindAndFrames() {
        var s = BackgroundSnapshot()
        s.timestampNanos = 1_788_000_000_500_000_000
        let header = ScanPacking.accumulatedHeader(
            l16Token: "808580f47", sensorID: "hesai-pandar40p", captureRef: "run-7#f10-59",
            snapshot: s, capturedAtNanos: s.timestampNanos)
        #expect(header.kind == .accumulated)
        #expect(header.captureRef == "run-7#f10-59")
        #expect(header.backgroundSequence == 0)
        #expect(header.capturedAt.seconds == 1_788_000_000)
        #expect(header.capturedAt.nanos == 500_000_000)
    }
}

private func movingTrack(
    _ id: String, state: TrackState = .confirmed, speed: Float = 8, maxSpeed: Float = 8,
    length: Float = 4.5
) -> Track {
    var t = Track()
    t.trackID = id
    t.state = state
    t.x = 10
    t.y = 2
    t.z = -1.8
    t.speedMps = speed
    t.maxSpeedMps = maxSpeed
    t.bboxLength = length
    t.bboxWidth = 1.9
    t.bboxHeight = 1.5
    t.bboxHeadingRad = 0.25
    t.classLabel = "car"
    return t
}

struct TransientBoxTests {
    @Test func onlyConfirmedTracksThatHaveMovedAreTransient() {
        var drawn = movingTrack("drawn")
        drawn.bboxCenterX = 10.4
        drawn.bboxCenterY = 2.1
        drawn.bboxCenterValid = true
        var nan = movingTrack("nan")
        nan.bboxHeadingRad = .nan
        nan.headingRad = .nan
        let tracks = [
            movingTrack("moving"), drawn, movingTrack("tentative", state: .tentative),
            movingTrack("hedge", speed: 0.1, maxSpeed: 0.6),
            movingTrack("parked since", speed: 0, maxSpeed: 6), movingTrack("no box", length: 0),
            nan,
        ]
        let boxes = FrameAccumulation.transientBoxes(tracks)
        #expect(boxes.count == 3)
        // Centred halfway up from the base the box is drawn on, at the
        // tracked position or the box model's own centre.
        #expect(boxes[0].centre == SIMD3(10, 2, -1.05))
        #expect(boxes[0].size == SIMD3(4.5, 1.9, 1.5))
        #expect(boxes[0].headingRad == 0.25)
        #expect(boxes[0].label == "car")
        #expect(boxes[1].centre == SIMD3(10.4, 2.1, -1.05))
        #expect(FrameAccumulation.transientMinSpeedMps == 1)
    }

    @Test func theAccumulatorKeepsTheBoxesOfFramesItTakes() {
        var acc = FrameAccumulator(frameCount: 3)
        #expect(acc.add(cloud(classes: [0]), tracks: [movingTrack("a")], frameNumber: 1) == .taken)
        #expect(
            acc.add(cloud(classes: [0]), tracks: [movingTrack("a")], frameNumber: 1) == .skipped)
        #expect(
            acc.add(
                cloud(classes: [0]), tracks: [movingTrack("a"), movingTrack("b")], frameNumber: 2)
                == .taken)
        guard
            case .refused = acc.add(
                cloud(classes: [0], mode: .voxel), tracks: [movingTrack("c")], frameNumber: 3)
        else {
            Issue.record("a voxel frame was taken")
            return
        }
        #expect(acc.transient.count == 3)
        #expect(acc.add(cloud(classes: [0]), frameNumber: 4) == .taken)
        #expect(acc.transient.count == 3)
    }

    @Test func theAccumulatedHeaderCarriesTheBoxesAsBox3() throws {
        let box = try #require(FrameAccumulation.transientBoxes([movingTrack("a")]).first)
        let header = ScanPacking.accumulatedHeader(
            l16Token: "808580f47", sensorID: "hesai-pandar40p", captureRef: "run-7#f1-2",
            snapshot: BackgroundSnapshot(), capturedAtNanos: nil, transient: [box])
        let b3 = try #require(header.transient.first)
        #expect(header.transient.count == 1)
        #expect(abs(b3.centre.x - 10) < 1e-6 && abs(b3.centre.y - 2) < 1e-6)
        #expect(abs(b3.centre.z + 1.05) < 1e-6)
        #expect(abs(b3.sizeM.x - 4.5) < 1e-6 && abs(b3.sizeM.y - 1.9) < 1e-6)
        #expect(abs(b3.sizeM.z - 1.5) < 1e-6)
        #expect(abs(b3.headingRad - 0.25) < 1e-6)
        #expect(b3.label == "car")
        // A background scan's header carries none.
        #expect(
            ScanPacking.header(
                l16Token: "808580f47", sensorID: "hesai-pandar40p", captureRef: "c",
                snapshot: BackgroundSnapshot(), capturedAtNanos: nil
            ).transient.isEmpty)
    }
}

struct CapturedAtTests {
    // kirk0's capture, 2025-12-05, replayed on 2026-10-23.
    static let logStart: Int64 = 1_764_973_024_741_089_000
    static let logEnd: Int64 = 1_764_973_107_924_472_000
    static let replayedAt: Int64 = 1_792_800_000_000_000_000

    @Test func aReplayStampsTheCaptureNotTheReplay() {
        for source in [SourceMode.pcap, .pcapAnalysis, .vrlog] {
            // A background snapshot, stamped when the server took it today.
            #expect(
                ScanPacking.capturedAtNanos(
                    source: source, logStartNs: Self.logStart, logEndNs: Self.logEnd,
                    observedNanos: Self.replayedAt) == Self.logStart)
            // A frame stamped with its own capture time keeps it.
            let frameTime = Self.logStart + 30_000_000_000
            #expect(
                ScanPacking.capturedAtNanos(
                    source: source, logStartNs: Self.logStart, logEndNs: Self.logEnd,
                    observedNanos: frameTime) == frameTime)
            // No log span: unset, never now.
            #expect(
                ScanPacking.capturedAtNanos(
                    source: source, logStartNs: 0, logEndNs: 0, observedNanos: Self.replayedAt)
                    == nil)
        }
        #expect(
            ScanPacking.capturedAtNanos(
                source: .live, logStartNs: 0, logEndNs: 0, observedNanos: Self.replayedAt)
                == Self.replayedAt)
        #expect(
            ScanPacking.capturedAtNanos(
                source: .unspecified, logStartNs: Self.logStart, logEndNs: Self.logEnd,
                observedNanos: Self.replayedAt) == nil)
        let unset = ScanPacking.header(
            l16Token: "808580f47", sensorID: "hesai-pandar40p", captureRef: "c",
            snapshot: BackgroundSnapshot(), capturedAtNanos: nil)
        #expect(!unset.hasCapturedAt)
    }
}

@MainActor struct CapturedAtUploadTests {
    private func replayFrame(_ n: UInt64) -> FrameBundle {
        var pc = cloud(classes: [0, 2], x0: Float(n))
        pc.timestampNanos = CapturedAtTests.replayedAt + Int64(n) * 100_000_000
        var bundle = frame(pc, index: n)
        bundle.playbackInfo?.logStartNs = CapturedAtTests.logStart
        bundle.playbackInfo?.logEndNs = CapturedAtTests.logEnd
        return bundle
    }

    @Test func aReplayTodayStampsTheOriginalCaptureTime() async throws {
        let (session, service) = await placedSession()
        session.accumulateFrameCount = 1
        session.startAccumulating(run: "run-k0")
        session.noteFrame(replayFrame(1))
        await session.pendingAccumulation?.value
        let accumulated = try #require(service.uploadedHeaders.last)
        #expect(accumulated.kind == .accumulated)
        let start = CapturedAtTests.logStart
        #expect(accumulated.capturedAt.seconds == start / 1_000_000_000)
        #expect(accumulated.capturedAt.nanos == Int32(start % 1_000_000_000))

        // The background, stamped by the server at replay time, too.
        session.useBackground()
        var background = snapshot(count: 10)
        background.timestampNanos = CapturedAtTests.replayedAt
        session.noteBackground(background)
        await session.register()
        let header = try #require(service.uploadedHeaders.last)
        #expect(header.kind == .background)
        #expect(header.capturedAt.seconds == start / 1_000_000_000)
    }

    @Test func aReplayWithoutALogSpanLeavesItUnset() async throws {
        let (session, service) = await placedSession()
        session.accumulateFrameCount = 1
        session.startAccumulating(run: "run-k0")
        session.noteFrame(frame(cloud(classes: [0, 2]), index: 1))
        await session.pendingAccumulation?.value
        #expect(try #require(service.uploadedHeaders.last).hasCapturedAt == false)
    }
}

@MainActor struct AccumulatedTransientUploadTests {
    @Test func theFramesMovingTracksGoUpWithTheScan() async throws {
        let (session, service) = await placedSession()
        session.accumulateFrameCount = 2
        session.startAccumulating(run: "run-42")
        for n in UInt64(100)...101 {
            var bundle = frame(cloud(classes: [0, 2], x0: Float(n)), index: n)
            var tracks = TrackSet()
            tracks.tracks = [movingTrack("car-\(n)"), movingTrack("hedge", speed: 0, maxSpeed: 0)]
            bundle.tracks = tracks
            session.noteFrame(bundle)
        }
        await session.pendingAccumulation?.value
        let header = try #require(service.uploadedHeaders.last)
        #expect(header.kind == .accumulated)
        #expect(header.transient.count == 2)
        #expect(header.transient.allSatisfy { $0.label == "car" })
    }
}

@MainActor struct AccumulateFramesTests {
    @Test func accumulatedFramesAreUploadedAndRegistered() async throws {
        let (session, service) = await placedSession()
        session.accumulateFrameCount = 3
        #expect(session.canAccumulate)
        session.startAccumulating(run: "run-42")
        for n in UInt64(100)...102 {
            session.noteFrame(frame(cloud(classes: [0, 1, 2, 1], x0: Float(n)), index: n))
        }
        #expect(session.accumulating == nil)
        await session.pendingAccumulation?.value

        let header = try #require(service.uploadedHeaders.last)
        #expect(header.kind == .accumulated)
        #expect(header.captureRef == "run-42#f100-102")
        #expect(header.l16Token == "808580f47")
        #expect(header.sensorID == "hesai-pandar40p")
        // Two points a frame (background and ground), none of them foreground.
        let uploadedX = service.uploadedBlocks.flatMap(\.x)
        #expect(uploadedX.count == 6)
        let foregroundX = (100...102).flatMap { n in [Float(n) + 0.1, Float(n) + 0.3] }
        #expect(uploadedX.allSatisfy { x in !foregroundX.contains { abs($0 - x) < 1e-4 } })
        #expect(service.uploadedBlocks.allSatisfy { $0.confidence.isEmpty })
        let expected = AlignmentAccumulatedScan(
            scanID: ownScanID, captureRef: "run-42#f100-102", frames: 3, pointsCollected: 6,
            points: 6)
        #expect(session.accumulated == expected)
        #expect(session.scan?.scanID == ownScanID)

        // No background has arrived: Register uses the accumulated scan, and
        // uploads nothing more.
        #expect(session.background == nil)
        #expect(session.canRegister)
        session.againstSiteModel = true
        await session.register()
        #expect(service.calls.filter { $0 == "UploadScan" }.count == 1)
        #expect(service.lastRegister?.scanID == ownScanID)
        // The model's only other scan is "sha256:other", so the switch holds.
        #expect(service.lastRegister?.againstSiteModel == true)
    }

    @Test func aReplayWithoutARunIsNamedByItsClip() async {
        let (session, service) = await placedSession()
        session.accumulateFrameCount = 1
        session.startAccumulating(run: nil)
        session.noteFrame(frame(cloud(classes: [0]), index: 7))
        await session.pendingAccumulation?.value
        #expect(service.uploadedHeaders.last?.captureRef == "\(columbusSite.clipID)#f7-7")
    }

    @Test func aLargeAccumulationIsUploadedInContractBlocks() async {
        let (session, service) = await placedSession()
        session.accumulateFrameCount = 1
        session.startAccumulating(run: "run-42")
        // 70,000 background points 0.1 m apart: thinning keeps every one.
        session.noteFrame(frame(cloud(classes: Array(repeating: 0, count: 70_000)), index: 1))
        await session.pendingAccumulation?.value
        #expect(service.uploadedBlocks.map(\.x.count) == [65_536, 4_464])
        #expect(session.accumulated?.points == 70_000)
    }

    @Test func aSplitStreamOfUnknownSourceRefusesBeforeAnythingIsCollected() async {
        let (session, service) = await placedSession()
        // velocity.report's split streaming, from a server that names no
        // source: foreground points only, and nothing to switch.
        session.noteFrame(
            frame(cloud(classes: [1, 1], mode: .foregroundOnly), index: 1, source: .unspecified))
        #expect(session.streamRefusal?.contains("foreground points only") == true)
        #expect(!session.canAccumulate)
        session.startAccumulating(run: "run-42")
        #expect(session.accumulating == nil)
        // Full frames again: the button comes back.
        session.noteFrame(frame(cloud(classes: [0]), index: 2, source: .unspecified))
        #expect(session.streamRefusal == nil)
        #expect(session.canAccumulate)
        #expect(!service.calls.contains("UploadScan"))
    }

    @Test func aDecimatedFrameMidwayStopsTheAccumulation() async {
        let (session, service) = await placedSession()
        session.accumulateFrameCount = 3
        session.startAccumulating(run: "run-42")
        session.noteFrame(frame(cloud(classes: [0, 2]), index: 1))
        #expect(session.accumulating?.frames == 1)
        session.noteFrame(frame(cloud(classes: [0, 2], mode: .uniform, ratio: 0.5), index: 2))
        #expect(session.accumulating == nil)
        #expect(session.accumulationRefusal?.contains("decimated") == true)
        session.noteFrame(frame(cloud(classes: [0, 2]), index: 3))
        #expect(session.pendingAccumulation == nil)
        #expect(!service.calls.contains("UploadScan"))
        #expect(session.accumulated == nil)
    }

    @Test func aLiveSensorRefuses() async {
        let (session, _) = await placedSession()
        session.noteFrame(frame(cloud(classes: [0]), index: 1, source: .live))
        #expect(session.streamRefusal == AlignmentSession.liveRefusal)
        #expect(!session.canAccumulate)
    }

    @Test func framesWithNothingToKeepUploadNothing() async {
        let (session, service) = await placedSession()
        session.accumulateFrameCount = 2
        session.startAccumulating(run: "run-42")
        session.noteFrame(frame(cloud(classes: [1, 1]), index: 1))
        session.noteFrame(frame(cloud(classes: [1]), index: 2))
        #expect(session.accumulationRefusal?.contains("no background or ground") == true)
        #expect(!service.calls.contains("UploadScan"))
    }

    @Test func aReplayedBackgroundTakesTheRunsNameNotThePickersClip() async {
        // Replaying one position's run while the picker shows another clip
        // at the same site once filed K1's background under K2's name.
        let (session, service) = await placedSession()
        session.noteLoadedRun(fullFrames: false, captureRef: "run-k1")
        session.noteBackground(snapshot(count: 10))
        await session.register()
        #expect(service.uploadedHeaders.last?.captureRef == "run-k1")

        // Without a loaded run (a live sensor) the picker's clip names it.
        session.captureChanged()
        session.noteBackground(snapshot(count: 12))
        await session.register()
        #expect(service.uploadedHeaders.last?.captureRef == columbusSite.clipID)
    }

    @Test func theBackgroundCanBeRegisteredAgain() async {
        let (session, service) = await placedSession()
        session.accumulateFrameCount = 1
        session.startAccumulating(run: "run-42")
        session.noteFrame(frame(cloud(classes: [0, 2]), index: 1))
        await session.pendingAccumulation?.value
        #expect(session.accumulated != nil)

        session.useBackground()
        #expect(session.accumulated == nil)
        #expect(!session.canRegister)
        session.noteBackground(snapshot(count: 10))
        await session.register()
        #expect(service.uploadedHeaders.map(\.kind) == [.accumulated, .background])
        #expect(service.uploadedBlocks.flatMap(\.x).count == 10)
    }

    @Test func aNewCaptureDropsTheAccumulation() async {
        let (session, _) = await placedSession()
        session.accumulateFrameCount = 1
        session.startAccumulating(run: "run-42")
        session.noteFrame(frame(cloud(classes: [0]), index: 1))
        await session.pendingAccumulation?.value
        #expect(session.accumulated != nil)
        session.captureChanged()
        #expect(session.accumulated == nil)
        #expect(session.scan == nil)
        #expect(!session.canRegister)
    }

    @Test func cancellingStopsTheCollection() async {
        let (session, service) = await placedSession()
        session.accumulateFrameCount = 5
        session.startAccumulating(run: "run-42")
        session.noteFrame(frame(cloud(classes: [0]), index: 1))
        session.cancelAccumulating()
        #expect(session.accumulating == nil)
        session.noteFrame(frame(cloud(classes: [0]), index: 2))
        #expect(session.accumulating == nil)
        #expect(!service.calls.contains("UploadScan"))
        #expect(session.accumulationRefusal == nil)
    }

    @Test func theAppTapsEveryFrame() async {
        let service = FakeAlignmentService()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        let app = AppState(autoConnectOnStartup: false, alignment: session)
        await session.probe()
        app.onFrameReceived(
            frame(cloud(classes: [1], mode: .foregroundOnly), index: 1, source: .vrlog))
        #expect(session.streamRefusal == AlignmentSession.vrlogRefusal)
    }
}

// MARK: - Full frames for a pcap replay's accumulation

/// velocity.report's full-frames switch, answering as told.
actor FakeFullFrames: ReplayFullFramesSwitch {
    private(set) var calls: [Bool] = []
    private var failure: ReplayFullFramesError?

    init(failing failure: ReplayFullFramesError? = nil) { self.failure = failure }

    func setFullFrames(_ enabled: Bool) async throws {
        calls.append(enabled)
        if let failure { throw failure }
    }
}

@MainActor struct AccumulateFullFramesTests {
    @Test func aPCAPReplayIsSwitchedToFullFramesForTheCollectionOnly() async throws {
        let fullFrames = FakeFullFrames()
        let (session, service) = await placedSession(fullFrames: fullFrames)
        // Split streaming is what a pcap replay sends until asked: no refusal.
        session.noteFrame(frame(cloud(classes: [1, 1], mode: .foregroundOnly), index: 1))
        #expect(session.streamRefusal == nil)
        #expect(session.canAccumulate)
        #expect(await fullFrames.calls.isEmpty)

        session.accumulateFrameCount = 2
        session.startAccumulating(run: "run-42")
        await session.pendingFullFrames?.value
        #expect(await fullFrames.calls == [true])

        // A frame split before the switch took effect is waited out.
        session.noteFrame(frame(cloud(classes: [1], mode: .foregroundOnly), index: 2))
        #expect(session.accumulating?.frames == 0)
        #expect(session.accumulationRefusal == nil)

        // Full frames: background kept, foreground (1) never.
        session.noteFrame(frame(cloud(classes: [0, 1, 0]), index: 3))
        session.noteFrame(frame(cloud(classes: [0, 1, 0], x0: 10), index: 4))
        #expect(session.accumulating == nil)
        await session.pendingFullFrames?.value
        #expect(await fullFrames.calls == [true, false])

        await session.pendingAccumulation?.value
        let header = try #require(service.uploadedHeaders.last)
        #expect(header.kind == .accumulated)
        #expect(header.captureRef == "run-42#f3-4")
        let uploadedX = service.uploadedBlocks.flatMap(\.x)
        #expect(uploadedX.count == 4)
        #expect(uploadedX.allSatisfy { abs($0 - 0.1) > 1e-4 && abs($0 - 10.1) > 1e-4 })
        #expect(session.accumulated?.frames == 2)
    }

    @Test func fullFramesGoOffWhenTheCollectionStopsShort() async {
        let fullFrames = FakeFullFrames()
        let (session, _) = await placedSession(fullFrames: fullFrames)
        session.noteFrame(frame(cloud(classes: [0]), index: 1))
        session.accumulateFrameCount = 5

        // Refused midway: a decimated frame.
        session.startAccumulating(run: "run-42")
        session.noteFrame(frame(cloud(classes: [0, 2], mode: .uniform, ratio: 0.5), index: 2))
        #expect(session.accumulationRefusal?.contains("decimated") == true)
        await session.pendingFullFrames?.value
        #expect(await fullFrames.calls == [true, false])

        // Cancelled.
        session.noteFrame(frame(cloud(classes: [0]), index: 3))
        session.startAccumulating(run: "run-42")
        session.cancelAccumulating()
        await session.pendingFullFrames?.value
        #expect(await fullFrames.calls == [true, false, true, false])

        // The source turned into a live sensor.
        session.startAccumulating(run: "run-42")
        session.noteFrame(frame(cloud(classes: [0]), index: 4, source: .live))
        #expect(session.accumulationRefusal == AlignmentSession.liveRefusal)
        await session.pendingFullFrames?.value
        #expect(await fullFrames.calls == [true, false, true, false, true, false])

        // Another capture opened.
        session.noteFrame(frame(cloud(classes: [0]), index: 5))
        session.startAccumulating(run: "run-42")
        session.captureChanged()
        #expect(session.accumulating == nil)
        await session.pendingFullFrames?.value
        #expect(await fullFrames.calls.suffix(2) == [true, false])
        #expect(await fullFrames.calls.count == 8)
    }

    @Test func closingThePaneSwitchesFullFramesOff() async {
        let fullFrames = FakeFullFrames()
        let (session, _) = await placedSession(fullFrames: fullFrames)
        let app = AppState(autoConnectOnStartup: false, alignment: session)
        app.showAlignmentPane = true
        session.noteFrame(frame(cloud(classes: [0]), index: 1))
        session.startAccumulating(run: "run-42")
        app.showAlignmentPane = false
        #expect(session.accumulating == nil)
        await session.pendingFullFrames?.value
        #expect(await fullFrames.calls == [true, false])
    }

    @Test func aVRLOGReplayIsRefusedUpFront() async {
        let fullFrames = FakeFullFrames()
        let (session, service) = await placedSession(fullFrames: fullFrames)
        // Even a frame that looks full: a VRLOG holds foreground points only.
        session.noteFrame(frame(cloud(classes: [0, 1]), index: 1, source: .vrlog))
        #expect(session.streamRefusal == AlignmentSession.vrlogRefusal)
        #expect(
            session.streamRefusal
                == "This run's VRLOG holds foreground points only; replay its pcap with Full frames, or load a full-frames run."
        )
        #expect(!session.canAccumulate)
        session.startAccumulating(run: "run-42")
        #expect(session.accumulating == nil)
        // An empty frame says the same.
        var empty = frame(cloud(classes: []), index: 2, source: .vrlog)
        empty.pointCloud = nil
        session.noteFrame(empty)
        #expect(session.streamRefusal == AlignmentSession.vrlogRefusal)
        #expect(await fullFrames.calls.isEmpty)
        #expect(!service.calls.contains("UploadScan"))
    }

    @Test func aFullFramesRunsVRLOGAccumulatesWithNothingToSwitch() async throws {
        let fullFrames = FakeFullFrames()
        let (session, service) = await placedSession(fullFrames: fullFrames)
        // A foreground run's VRLOG is refused; loading a full-frames run lifts it.
        session.noteFrame(frame(cloud(classes: [0, 1]), index: 1, source: .vrlog))
        #expect(session.streamRefusal == AlignmentSession.vrlogRefusal)
        session.noteLoadedRun(fullFrames: true)
        #expect(session.streamRefusal == nil)
        #expect(session.canAccumulate)

        session.accumulateFrameCount = 2
        session.startAccumulating(run: "run-full")
        session.noteFrame(frame(cloud(classes: [0, 1, 0]), index: 10, source: .vrlog))
        session.noteFrame(frame(cloud(classes: [0, 1, 0], x0: 10), index: 11, source: .vrlog))
        await session.pendingAccumulation?.value
        #expect(service.uploadedHeaders.last?.captureRef == "run-full#f10-11")
        #expect(service.uploadedBlocks.flatMap(\.x).count == 4)
        #expect(session.pendingFullFrames == nil)
        #expect(await fullFrames.calls.isEmpty)

        // Another capture forgets it.
        session.captureChanged()
        session.noteFrame(frame(cloud(classes: [0, 1]), index: 1, source: .vrlog))
        #expect(session.streamRefusal == AlignmentSession.vrlogRefusal)
    }

    @Test func aServerThatRefusesFullFramesStopsTheAccumulation() async {
        let reason = "full frames are only for a running PCAP replay"
        let fullFrames = FakeFullFrames(failing: .refused(reason))
        let (session, _) = await placedSession(fullFrames: fullFrames)
        session.noteFrame(frame(cloud(classes: [1], mode: .foregroundOnly), index: 1))
        session.startAccumulating(run: "run-42")
        await session.pendingFullFrames?.value
        #expect(session.accumulating == nil)
        #expect(session.accumulationRefusal?.contains("would not stream full frames") == true)
        #expect(session.accumulationRefusal?.contains(reason) == true)
        // Nothing was switched on, so nothing is switched off.
        #expect(await fullFrames.calls == [true])
        // Split frames after the refusal are no reason to refuse the button.
        session.noteFrame(frame(cloud(classes: [1], mode: .foregroundOnly), index: 2))
        #expect(session.canAccumulate)
    }

    @Test func anOlderServerWithoutTheSwitchIsSaidSo() async {
        let fullFrames = FakeFullFrames(failing: .unsupported)
        let (session, _) = await placedSession(fullFrames: fullFrames)
        session.noteFrame(frame(cloud(classes: [1], mode: .foregroundOnly), index: 1))
        session.startAccumulating(run: "run-42")
        await session.pendingFullFrames?.value
        #expect(session.accumulating == nil)
        #expect(session.accumulationRefusal?.contains("cannot stream full frames") == true)
        #expect(await fullFrames.calls == [true])
    }

    @Test func aFailedRequestIsFollowedByASwitchOff() async {
        let fullFrames = FakeFullFrames(failing: .failed("timed out"))
        let (session, _) = await placedSession(fullFrames: fullFrames)
        session.noteFrame(frame(cloud(classes: [1], mode: .foregroundOnly), index: 1))
        session.startAccumulating(run: "run-42")
        // The answer is lost, not a refusal: the server may have switched.
        await session.pendingFullFrames?.value
        await session.pendingFullFrames?.value
        #expect(session.accumulationRefusal?.contains("timed out") == true)
        #expect(await fullFrames.calls == [true, false])
    }

    @Test func withoutVelocityReportAPCAPReplayIsRefused() async {
        let (session, _) = await placedSession(fullFrames: nil)
        session.noteFrame(frame(cloud(classes: [1], mode: .foregroundOnly), index: 1))
        session.startAccumulating(run: "run-42")
        #expect(session.accumulating == nil)
        #expect(session.accumulationRefusal == AlignmentSession.noFullFramesRefusal)
    }
}

/// The HTTP half of the switch, against a stub server.
struct ReplayFullFramesClientTests {
    private func client(
        answer: @escaping (URLRequest) throws -> (Int, String)
    ) -> ReplayFullFramesClient {
        let (session, baseURL, register) = AnnotationMockURLProtocol.makeSession()
        register { request in
            let (status, body) = try answer(request)
            let response = HTTPURLResponse(
                url: request.url!, statusCode: status, httpVersion: nil,
                headerFields: ["Content-Type": "application/json"])!
            return (response, Data(body.utf8))
        }
        return ReplayFullFramesClient(baseURL: baseURL, session: session)
    }

    @Test func itPostsTheSwitchAndChecksTheAnswer() async throws {
        let seen = LockedBox<(String?, String, Bool?)>()
        let c = client { request in
            let body = try requestJSONBody(request)
            seen.set((request.httpMethod, request.url!.path, body["enabled"] as? Bool))
            return (200, #"{"success":true,"full_frames":true,"current_source":"pcap"}"#)
        }
        try await c.setFullFrames(true)
        let (method, path, enabled) = try #require(seen.get())
        #expect(method == "POST")
        #expect(path == "/api/lidar/replay/full_frames")
        #expect(enabled == true)
    }

    @Test func refusalsCarryTheServersReason() async {
        for status in [409, 501] {
            let c = client { _ in
                (status, #"{"error":"full frames are only for a running PCAP replay"}"#)
            }
            await #expect(
                throws: ReplayFullFramesError.refused(
                    "full frames are only for a running PCAP replay")
            ) { try await c.setFullFrames(true) }
        }
    }

    @Test func aServerWithoutTheEndpointIsUnsupported() async {
        for (status, body) in [
            (404, "404 page not found"), (405, ""), (200, "<!doctype html><html></html>"),
        ] {
            let c = client { _ in (status, body) }
            await #expect(throws: ReplayFullFramesError.unsupported) {
                try await c.setFullFrames(true)
            }
        }
    }

    @Test func anAnswerThatDisagreesFails() async {
        let c = client { _ in (200, #"{"success":true,"full_frames":false}"#) }
        await #expect(throws: ReplayFullFramesError.self) { try await c.setFullFrames(true) }
        let d = client { _ in (500, #"{"error":"boom"}"#) }
        await #expect(throws: ReplayFullFramesError.failed("HTTP 500: boom")) {
            try await d.setFullFrames(false)
        }
    }
}

/// A value a stub server's handler can hand back to the test.
final class LockedBox<T>: @unchecked Sendable {
    private let lock = NSLock()
    private var value: T?
    func set(_ v: T) { lock.withLock { value = v } }
    func get() -> T? { lock.withLock { value } }
}

// MARK: - Refinement (site model spike V6)

@MainActor struct AlignmentRefinementTests {
    private let site = AlignmentSite(
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
        description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

    /// Placed at a site whose model holds `scans`, with a refinement of the
    /// first two of them ready: scan a passed every gate, scan b failed one.
    private func placed(
        scans: [String] = ["sha256:a", "sha256:b"]
    ) async -> (AlignmentSession, FakeAlignmentService) {
        let service = FakeAlignmentService()
        service.siteModelID = "sha256:model-a"
        service.siteModelScans = scans
        var results: [Gerardus_Align_V1alpha1_RefinedScan] = []
        for (id, passed) in [("a", true), ("b", false)] {
            var refined = Gerardus_Align_V1alpha1_RefinedScan()
            refined.scanID = "sha256:\(id)"
            refined.registrationID = "gms:refine-\(id)"
            refined.deltaM = 0.03
            refined.jointConstraintRank = 6
            results.append(refined)
            var run = Gerardus_Align_V1alpha1_Registration()
            run.registrationID = refined.registrationID
            run.state = passed ? .candidate : .failed
            run.gates = [
                gate("ground_p95_m", passed: true), gate("refine_shift_m", passed: passed),
            ]
            service.runs[run.registrationID] = run
        }
        service.refineResult.scans = results
        service.refineResult.modelID = "sha256:model-a"
        service.refineResult.nEff = 500
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        session.sink = FakeOverlaySink()
        await session.probe()
        session.choose(site)
        session.northAzimuthText = "35"
        await session.placeFromHint()
        return (session, service)
    }

    @Test func refineListsEachScanWithItsGatesAndWeighting() async {
        let (session, service) = await placed()
        #expect(session.canRefine)
        session.refineNEff = 250
        await session.refine()
        #expect(service.lastRefine?.l16Token == "808580f47")
        #expect(service.lastRefine?.nEff == 250)
        #expect(service.calls.filter { $0 == "GetRegistration" }.count == 2)
        let scans = session.refinement?.scans ?? []
        #expect(scans.map(\.scanID) == ["sha256:a", "sha256:b"])
        #expect(session.refinedPassed(scans[0]))
        #expect(!session.refinedPassed(scans[1]), "a failed gate records MANUALLY_PLACED")
        #expect(!session.isRefining && session.busy == nil && session.progress == nil)
    }

    @Test func refineNeedsTwoAcceptedScans() async {
        let (session, service) = await placed(scans: ["sha256:a"])
        #expect(!session.canRefine)
        await session.refine()
        #expect(!service.calls.contains("RefineSite"))
    }

    @Test func acceptRefinementSendsEveryJointHypothesisThenRestreams() async {
        let (session, service) = await placed()
        service.runs["gms:refine-a"]?.refinementHeads = [
            "sha256:a": "head-a", "sha256:b": "head-b",
        ]
        await session.refine()
        session.reviewer = ""
        #expect(!session.canAcceptRefinement, "a reviewer is required")
        session.reviewer = "operator"
        #expect(session.canAcceptRefinement)
        // Accepting moves the site model.
        service.siteModelID = "sha256:model-b"
        service.servedModelID = "sha256:model-b"
        let streamsBefore = service.calls.filter { $0 == "StreamReference" }.count
        await session.acceptRefinement()

        #expect(service.reviews.map(\.registrationID) == ["gms:refine-a", "gms:refine-b"])
        #expect(service.reviewBatches.count == 1)
        #expect(!service.reviewBatches[0].requestID.isEmpty)
        #expect(service.reviews.map(\.expectedRecordID) == ["head-a", "head-b"])
        for review in service.reviews {
            guard case .acceptHypothesisID(let id) = review.decision else {
                Issue.record("accept sent no hypothesis")
                return
            }
            #expect(id == "j1" && review.reviewer == "operator")
        }
        #expect(session.refinedAccepted == ["sha256:a", "sha256:b"])
        #expect(session.errorMessage == nil)
        #expect(service.calls.contains("ListRegistrationRecords"))
        #expect(service.calls.filter { $0 == "StreamReference" }.count > streamsBefore)

        // Accepted once: a second press records nothing.
        #expect(!session.canAcceptRefinement)
        await session.acceptRefinement()
        #expect(service.reviews.count == 2)
    }

    @Test func aStaleJointHypothesisStopsTheAcceptanceAndSaysSo() async {
        let (session, service) = await placed()
        await session.refine()
        session.reviewer = "operator"
        service.refusedReviews = ["gms:refine-b"]
        await session.acceptRefinement()
        #expect(session.refinedAccepted.isEmpty)
        #expect(service.records.isEmpty)
        #expect(service.reviewBatches.count == 1)
        let message = session.errorMessage ?? ""
        #expect(message.contains("no partial acceptance"), "\(message)")
        #expect(message.contains("refine again"), "\(message)")
        #expect(session.busy == nil)
    }

    @Test func aLostBatchResponseRetriesTheIdenticalAtomicRequest() async {
        let (session, service) = await placed()
        await session.refine()
        session.reviewer = "operator"
        service.reviewFailure = RPCError(code: .unavailable, message: "lost response")
        await session.acceptRefinement()
        #expect(session.refinedAccepted.isEmpty)
        service.reviewFailure = nil
        await session.acceptRefinement()
        #expect(service.reviewBatches.count == 2)
        #expect(service.reviewBatches[0] == service.reviewBatches[1])
        #expect(session.refinedAccepted == ["sha256:a", "sha256:b"])
    }
}

// MARK: - What blocks Accept

/// A registration open at a site whose model can be refined: the operator's
/// path in the report of a blocked Accept (register, refine, accept).
@MainActor struct AcceptBlockingTests {
    private let site = AlignmentSite(
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
        description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

    /// Placed at a site whose model holds two accepted scans, a refinement
    /// of them ready (both pass), and a candidate run that passed its gates
    /// registered from a settled background.
    private func registered(
        defaults: UserDefaults? = nil
    ) async -> (AlignmentSession, FakeAlignmentService) {
        let service = FakeAlignmentService()
        service.siteModelID = "sha256:model-a"
        service.siteModelScans = ["sha256:a", "sha256:b"]
        for id in ["a", "b"] {
            var refined = Gerardus_Align_V1alpha1_RefinedScan()
            refined.scanID = "sha256:\(id)"
            refined.registrationID = "gms:refine-\(id)"
            service.refineResult.scans.append(refined)
            var run = Gerardus_Align_V1alpha1_Registration()
            run.registrationID = refined.registrationID
            run.scanID = refined.scanID
            run.state = .candidate
            run.gates = [gate("ground_p95_m", passed: true)]
            service.runs[run.registrationID] = run
        }
        service.refineResult.modelID = "sha256:model-a"
        service.streamed = [hypothesis("h-east", eastM: 5, headingDeg: 143.5)]
        service.runState = .candidate
        service.gates = [gate("ground_p95_m", passed: true)]
        let session = AlignmentSession(
            makeClient: { _ in service }, lookup: nil, defaults: defaults)
        session.sink = FakeOverlaySink()
        session.evaluateSleep = { _ in try await Task.sleep(for: .seconds(3600)) }
        await session.probe()
        session.choose(site)
        session.northAzimuthText = "35"
        await session.placeFromHint()
        session.noteBackground(snapshot(count: 300))
        await session.register()
        session.reviewer = "operator"
        return (session, service)
    }

    /// The model moves when anything is accepted, as Gerardus's does.
    private func modelMovesOnAccept(_ service: FakeAlignmentService) {
        service.siteModelID = "sha256:model-b"
        service.servedModelID = "sha256:model-b"
    }

    @Test func registerRefineThenAcceptTheHypothesis() async {
        let (session, service) = await registered()
        await session.refine()
        #expect(session.busy == nil)
        #expect(session.canReview)
        await session.accept()
        guard case .acceptHypothesisID(let id) = service.lastReview?.decision else {
            Issue.record("accept sent no hypothesis")
            return
        }
        #expect(id == "h-east")
        #expect(session.busy == nil)
    }

    @Test func acceptingTheRefinementLeavesNothingBusy() async {
        let (session, service) = await registered()
        await session.refine()
        modelMovesOnAccept(service)
        await session.acceptRefinement()
        #expect(service.reviews.map(\.registrationID) == ["gms:refine-a", "gms:refine-b"])
        // The registered layers were re-streamed for the moved model, and the
        // busy line that re-stream set is gone with it.
        #expect(session.busy == nil, "busy: \(session.busy ?? "")")
        #expect(session.canReview)
        #expect(session.canRefine)
        await session.accept()
        #expect(service.reviews.last?.registrationID == "gms:reg")
    }

    @Test func aBlankReviewerBlocksBothAcceptsAndSaysSo() async {
        let (session, service) = await registered()
        await session.refine()
        session.reviewer = "   "
        #expect(session.showsReviewerField)
        #expect(!session.canReview)
        #expect(session.reviewBlockers == [AlignmentSession.noReviewerReason])
        #expect(!session.canAcceptRefinement)
        #expect(session.acceptRefinementBlockers == [AlignmentSession.noReviewerReason])
        // Gerardus refuses a decision without a reviewer: nothing is sent.
        await session.accept()
        await session.acceptRefinement()
        #expect(service.reviews.isEmpty)
        session.reviewer = "operator"
        #expect(session.reviewBlockers.isEmpty && session.acceptRefinementBlockers.isEmpty)
    }

    @Test func whileRefiningBothSayWhatTheyWaitFor() async {
        let (session, service) = await registered()
        var during: ([String], [String]) = ([], [])
        service.duringRefine = {
            during = (session.reviewBlockers, session.acceptRefinementBlockers)
        }
        await session.refine()
        let busy = "Busy refining the site: wait for it to finish."
        #expect(during.0 == [busy])
        // No "no refinement yet" while one is running.
        #expect(during.1 == [busy])
        #expect(session.reviewBlockers.isEmpty)
    }

    @Test func beforeAnythingIsPlacedTheReasonsSaySo() async {
        let service = FakeAlignmentService()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        session.reviewer = "operator"
        await session.probe()
        #expect(
            session.reviewBlockers == [
                AlignmentSession.noRegistrationReason, AlignmentSession.noPoseReason,
            ])
        #expect(session.acceptRefinementBlockers == ["No refinement yet: press Refine site."])
        #expect(!session.showsReviewerField)
        #expect(session.refineBlocker == "Place the sensor from its hint first.")
    }

    @Test func aRecordedRefinementSaysToRefineAgain() async {
        let (session, service) = await registered()
        await session.refine()
        modelMovesOnAccept(service)
        await session.acceptRefinement()
        #expect(
            session.acceptRefinementBlockers == [
                "Recorded 2 of 2 refined poses: refine again to record another refinement."
            ])
        await session.refine()
        #expect(session.canAcceptRefinement)
    }

    @Test func acceptingOneOfTheRefinedScansOnItsOwnSaysToRefineAgain() async {
        let (session, service) = await registered()
        // The open capture's scan is one the refinement moved.
        var own = Gerardus_Align_V1alpha1_RefinedScan()
        own.scanID = "sha256:scan"
        own.registrationID = "gms:refine-own"
        service.refineResult.scans.append(own)
        await session.refine()
        #expect(session.canAcceptRefinement)
        await session.accept()
        #expect(!session.canAcceptRefinement)
        let reasons = session.acceptRefinementBlockers
        #expect(reasons.count == 1 && reasons[0].contains("accepted again"), "\(reasons)")
        // Accept refinement would have recorded a and b, then been refused.
        await session.acceptRefinement()
        #expect(service.reviews.map(\.registrationID) == ["gms:reg"])
        // A rejection moves no pose.
        await session.refine()
        session.rejectReason = "wrong corner"
        await session.reject()
        #expect(session.canAcceptRefinement)
    }

    @Test func anEmptySavedReviewerFallsBackToAName() throws {
        let suite = "AcceptBlockingTests.\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        defaults.set("", forKey: AlignmentSession.reviewerKey)
        defaults.set("Annotator", forKey: "annotation.operatorName")
        let session = AlignmentSession(
            makeClient: { _ in FakeAlignmentService() }, lookup: nil, defaults: defaults)
        #expect(session.reviewer == "Annotator")
    }
}

// MARK: - The refinement column

@MainActor struct RefinementColumnTests {
    private let site = AlignmentSite(
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
        description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

    /// Placed at a site whose model holds two accepted scans, with a
    /// refinement of them ready and no registration open.
    private func placed() async -> (AlignmentSession, FakeAlignmentService) {
        let service = FakeAlignmentService()
        service.siteModelID = "sha256:model-a"
        service.siteModelScans = ["sha256:a", "sha256:b"]
        for id in ["a", "b"] {
            var refined = Gerardus_Align_V1alpha1_RefinedScan()
            refined.scanID = "sha256:\(id)"
            refined.registrationID = "gms:refine-\(id)"
            service.refineResult.scans.append(refined)
            var run = Gerardus_Align_V1alpha1_Registration()
            run.registrationID = refined.registrationID
            run.state = .candidate
            run.gates = [gate("ground_p95_m", passed: true), gate("refine_shift_m", passed: true)]
            service.runs[run.registrationID] = run
        }
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        session.sink = FakeOverlaySink()
        session.reviewer = "operator"
        await session.probe()
        session.choose(site)
        session.northAzimuthText = "35"
        await session.placeFromHint()
        return (session, service)
    }

    @Test func itIsClosedUntilARefinementRunsAndOpensWhenOneDoes() async {
        let (session, service) = await placed()
        #expect(!session.refinementColumnOpen)
        var openWhileRunning = false
        service.duringRefine = { openWhileRunning = session.refinementColumnOpen }
        await session.refine()
        #expect(openWhileRunning, "the column is open while the refinement runs")
        #expect(session.refinementColumnOpen)
        #expect(session.showsReviewerField, "Accept refinement is shown, so its reviewer is")
    }

    @Test func closingItKeepsTheRefinementForWhenItReopens() async {
        let (session, _) = await placed()
        await session.refine()
        session.closeRefinementColumn()
        #expect(!session.refinementColumnOpen)
        #expect(session.refinement?.scans.map(\.scanID) == ["sha256:a", "sha256:b"])
        #expect(session.refinedRuns.count == 2)
        // Neither Accept is shown now, so neither is the reviewer.
        #expect(!session.showsReviewerField)
        session.openRefinementColumn()
        #expect(session.refinementColumnOpen)
        #expect(session.refinement?.scans.count == 2)
        #expect(
            session.refinedRuns["sha256:a"]?.gates.map(\.name) == [
                "ground_p95_m", "refine_shift_m",
            ])
        #expect(session.canAcceptRefinement)
    }

    @Test func theOperatorCanOpenItBeforeRefining() async {
        let (session, service) = await placed()
        session.openRefinementColumn()
        #expect(session.refinementColumnOpen)
        #expect(session.refinement == nil)
        #expect(!service.calls.contains("RefineSite"), "opening the column refines nothing")
        #expect(session.acceptRefinementBlockers == ["No refinement yet: press Refine site."])
        #expect(session.canRefine && session.refineBlocker == nil)
    }

    @Test func aRefinementAtAnotherSiteIsNotCarriedOver() async {
        let (session, _) = await placed()
        await session.refine()
        session.choose(
            AlignmentSite(
                clipID: "other", l16Token: "808580f49", l13Token: "808580f4", description: "",
                sensorID: "hesai-pandar40p", latitude: 37.79, longitude: -122.40))
        #expect(!session.refinementColumnOpen)
        #expect(session.refinement == nil)
    }
}

// MARK: - Boxes and wall planes in the session

@MainActor struct BoxesAndWallPlanesSessionTests {
    private let site = AlignmentSite(
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
        description: "S2 archive: Columbus at Broadway (33.9 min across 7 captures)",
        sensorID: "hesai-pandar40p", latitude: 37.7978, longitude: -122.4065)

    /// Every layer the default fake lists, and BOXES.
    private static let withBoxes: [Gerardus_Align_V1alpha1_ReferenceLayer] = [
        .aerialPoints, .osm, .footprints, .intersections, .registeredScans, .observed, .groundMesh,
        .boxes,
    ]

    private func placed(
        _ configure: (FakeAlignmentService) -> Void = { _ in }
    ) async -> (AlignmentSession, FakeAlignmentService, FakeOverlaySink) {
        let service = FakeAlignmentService()
        configure(service)
        let sink = FakeOverlaySink()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        session.sink = sink
        await session.probe()
        session.choose(site)
        session.northAzimuthText = "35"
        await session.placeFromHint()
        return (session, service, sink)
    }

    @Test func listedBoxesAreAskedForWithTheBundleAndDrawn() async throws {
        let (session, service, sink) = await placed {
            $0.availableLayers = Self.withBoxes
            $0.wallPlanes = [
                wallPlane(role: "wall"), wallPlane(role: "wall"),
                wallPlane(role: "popout", offset: 0.6), wallPlane(role: "recess", offset: -0.3),
            ]
        }
        #expect(session.errorMessage == nil)
        // On the bundle's call, not a call of its own.
        #expect(
            service.references.map(\.layers) == [
                [.aerialPoints, .osm, .footprints, .intersections, .boxes], [.registeredScans],
                [.observed], [.groundMesh],
            ])
        let summary = try #require(session.reference)
        #expect(summary.boxesServed)
        #expect(summary.boxWalls == 2 && summary.partyWalls == 1)
        #expect(summary.footprints == 1)
        // Wall planes are counted apart from the observed features.
        #expect(summary.observedFeatures == 4)
        #expect(summary.wallPlanes == 4)
        #expect(summary.wallPlaneRoles == [.wall: 2, .popout: 1, .recess: 1])
        #expect(summary.wallPlaneRoleText == "2 walls · 1 popout · 1 recess")

        let overlay = try #require(sink.overlays.last ?? nil)
        #expect(overlay.boxVertexCount == 16)
        #expect(overlay.footprintVertexCount == 6)
        #expect(overlay.wallPlaneVertexCount == 32)
        #expect(overlay.observedVertexCount == 16)

        // Both on by default, each with its own toggle.
        #expect(sink.visibility?.boxes == true && sink.visibility?.wallPlanes == true)
        session.showBoxes = false
        #expect(sink.visibility?.boxes == false)
        #expect(sink.visibility?.footprints == true && sink.visibility?.wallPlanes == true)
        session.showWallPlanes = false
        #expect(sink.visibility?.wallPlanes == false)
        #expect(sink.visibility?.observed == true)
        session.showObserved = false
        session.showBoxes = true
        #expect(sink.visibility?.observed == false && sink.visibility?.boxes == true)
        #expect(sink.visibility?.wallPlanes == false)
    }

    @Test func anEmptyBoxesLayerAndNoWallPlanesAreNotAnError() async throws {
        // Gerardus today: BOXES listed, served empty and complete; OBSERVED
        // without a wall plane.
        let (session, service, sink) = await placed {
            $0.availableLayers = Self.withBoxes
            $0.emptyBoxes = true
        }
        #expect(session.errorMessage == nil)
        #expect(session.registeredLayersNote == nil)
        #expect(service.references.first?.layers.last == .boxes)
        let summary = try #require(session.reference)
        // Served, so the toggle is shown, with nothing in it.
        #expect(summary.boxesServed)
        #expect(summary.boxWalls == 0 && summary.partyWalls == 0)
        #expect(summary.wallPlanes == 0 && summary.wallPlaneRoles.isEmpty)
        #expect(summary.wallPlaneRoleText == "")
        #expect(summary.observedFeatures == 4)
        #expect(summary.footprints == 1 && summary.aerialPoints == 2)
        let overlay = try #require(sink.overlays.last ?? nil)
        #expect(overlay.boxVertexCount == 0 && overlay.wallPlaneVertexCount == 0)
        #expect(overlay.footprintVertexCount == 6)
    }

    @Test func aServerThatDoesNotListBoxesIsNotAskedForThem() async throws {
        let (session, service, _) = await placed()
        #expect(!service.availableLayers.contains(.boxes))
        #expect(service.references.first?.layers == AlignmentSession.bundleLayers)
        #expect(!service.references.contains { $0.layers.contains(.boxes) })
        let summary = try #require(session.reference)
        #expect(!summary.boxesServed)
        #expect(summary.boxWalls == 0)

        // Nor one that lists no layers at all: BOXES is asked for only when
        // listed, since an older server would refuse the bundle with it.
        let (_, quiet, _) = await placed { $0.availableLayers = [] }
        #expect(quiet.references.first?.layers == AlignmentSession.bundleLayers)
    }

    @Test func anAcceptKeepsTheBoxesAndReplacesTheWallPlanes() async throws {
        let (session, service, sink) = await placed {
            $0.availableLayers = Self.withBoxes
            $0.siteModelID = "sha256:model-a"
            $0.wallPlanes = [wallPlane(role: "wall")]
        }
        session.noteBackground(snapshot(count: 10))
        await session.register()
        let before = service.references.count
        service.servedModelID = "sha256:model-b"
        service.siteModelID = "sha256:model-b"
        service.wallPlanes = [wallPlane(role: "wall"), wallPlane(role: "popout", offset: 0.5)]
        await session.accept()
        // Only the registered layers are re-streamed.
        #expect(
            service.references.dropFirst(before).map(\.layers) == [
                [.registeredScans], [.observed], [.groundMesh],
            ])
        #expect(session.reference?.boxWalls == 2)
        #expect(session.reference?.wallPlanes == 2)
        #expect(sink.overlays.last??.boxVertexCount == 16)
        #expect(sink.overlays.last??.wallPlaneVertexCount == 16)
    }
}
