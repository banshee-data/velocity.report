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

/// The renderer keeps a buffer and a toggle for each new layer.
@MainActor struct ReferenceOverlayRendererTests {
    @Test func eachNewLayerHasABufferAndAToggle() {
        guard let device = MTLCreateSystemDefaultDevice() else { return }
        let view = MTKView()
        view.device = device
        guard let renderer = MetalRenderer(metalView: view) else { return }
        var g = ReferenceGeometry()
        for layer in AlignmentSession.bundleLayers + AlignmentSession.registeredLayers {
            for message in referenceMessages(layer, modelID: "sha256:m") { g.append(message) }
        }
        renderer.setReferenceOverlay(
            ReferenceStyle.vertices(for: g, centre: SIMD3(-9.4, -38.5, 23)))
        #expect(renderer.referenceScanCount == 5)
        #expect(renderer.referenceFootprintVertexCount == 6)
        #expect(renderer.referenceObservedVertexCount == 16)
        #expect(renderer.referenceGroundMeshVertexCount == 8)
        var shown = ReferenceVisibility()
        shown.registeredScans = false
        shown.groundMesh = false
        renderer.setReferenceVisibility(shown)
        #expect(renderer.referenceVisibility == shown)
        renderer.setReferenceOverlay(nil)
        #expect(renderer.referenceScanCount == 0)
        #expect(renderer.referenceObservedVertexCount == 0)
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
    default: return []
    }
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
    /// Serve the registered layers empty and complete, with no model_id, as
    /// Gerardus does before it builds them.
    var emptyRegisteredLayers = false
    var lastRegister: Gerardus_Align_V1alpha1_RegisterRequest?
    var lastReview: Gerardus_Align_V1alpha1_ReviewRegistrationRequest?
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
            if emptyRegisteredLayers, ReferenceGeometry.registeredLayers.contains(layer) {
                var empty = Gerardus_Align_V1alpha1_StreamReferenceResponse()
                empty.layer = layer
                empty.layerComplete = true
                geometry.append(empty)
                continue
            }
            for message in referenceMessages(layer, modelID: servedModelID) {
                geometry.append(message)
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
    var visibility: ReferenceVisibility?

    func setReferenceOverlay(_ vertices: ReferenceOverlayVertices?) { overlays.append(vertices) }
    func setReferenceModelMatrix(_ matrix: simd_float4x4) { matrices.append(matrix) }
    func setReferenceVisibility(_ visibility: ReferenceVisibility) { self.visibility = visibility }
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
    func getSiteModel(
        l16Token: String
    ) async throws -> Gerardus_Align_V1alpha1_GetSiteModelResponse? {
        try await inner.getSiteModel(l16Token: l16Token)
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
            snapshot: s)
        #expect(header.kind == .accumulated)
        #expect(header.captureRef == "run-7#f10-59")
        #expect(header.backgroundSequence == 0)
        #expect(header.capturedAt.seconds == 1_788_000_000)
        #expect(header.capturedAt.nanos == 500_000_000)
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
        #expect(session.streamRefusal?.contains("replay the capture's pcap") == true)
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
