import Foundation
import Testing
import simd

@testable import VelocityVisualiser

@MainActor struct PhysicalAuthoringComprehensionTests {
    @Test func anUnboundedPlacementIsVisibleOnlyAsAnEditableSketch() {
        var k = PhysicalKeyframe(keyframeID: "k", sampleID: 0, timestampNs: 10)
        k.position = PhysicalPosition(status: .observed, xM: 3, yM: 4)
        let o = PhysicalObject(objectID: "o", keyframes: [k])
        let sketch = PhysicalGeometry.derive(object: o, keyframe: k, gate: .authoring)
        #expect(sketch.anchorPoint?.x == 3 && sketch.anchorPoint?.y == 4)
        #expect(sketch.unboundedDraft)
        #expect(PhysicalGeometry.derive(object: o, keyframe: k, gate: .preview).anchorPoint == nil)
        #expect(PhysicalGeometry.derive(object: o, keyframe: k).anchorPoint == nil)
        let vp = OrthoViewport(halfHeight: 10, size: CGSize(width: 400, height: 400), centre: .zero)
        let handles = PhysicalHandles.positions(sketch, basis: OrthoViewBasis(.top), viewport: vp)
        #expect(handles.move != nil && handles.turn != nil)
        #expect(sketch.yaw == nil, "the tool handle must not invent an orientation")
        #expect(k.position.boundM == nil && k.yaw.yawRad == nil)
    }

    @Test func aDraftYawIsDrawnWithoutInventingPrecision() {
        var k = PhysicalKeyframe(keyframeID: "k", sampleID: 0, timestampNs: 10)
        k.yaw = PhysicalYaw(status: .observed, axis: .frontRearAmbiguous, yawRad: 0.3)
        let o = PhysicalObject(objectID: "o", keyframes: [k])
        let sketch = PhysicalGeometry.derive(object: o, keyframe: k, gate: .authoring)
        #expect(sketch.yaw?.rad == 0.3 && sketch.unboundedDraft)
        #expect(PhysicalGeometry.derive(object: o, keyframe: k).yaw == nil)
        k.yaw.yawRad = .nan
        #expect(PhysicalGeometry.derive(object: o, keyframe: k, gate: .authoring).yaw == nil)
        k.position = PhysicalPosition(status: .observed, xM: .infinity, yM: 4)
        #expect(
            PhysicalGeometry.derive(object: o, keyframe: k, gate: .authoring).anchorPoint == nil)
    }

    @Test func ambiguousAxesCannotAcquireNamedFaces() {
        var k = PhysicalKeyframe(keyframeID: "k", sampleID: 0, timestampNs: 10)
        PhysicalDraft.setAxis(.frontRearAmbiguous, of: &k)
        PhysicalDraft.setAnchor(.leftFace, of: &k)
        #expect(k.anchor.kind == .bodyCentre)
        PhysicalDraft.setAxis(.resolved, of: &k)
        PhysicalDraft.setAnchor(.leftFace, of: &k)
        #expect(k.anchor.kind == .leftFace)
        PhysicalDraft.setAxis(.frontRearAmbiguous, of: &k)
        #expect(k.anchor.kind == .bodyCentre)
    }

    @Test func reloadReportsThatItReadAnEmptyDocumentAndDoesNotCreateABody() async throws {
        let dir = try SyntheticPack.write([SyntheticPack.car(at: simd_float2(5, 0))])
        let (transport, url, register) = AnnotationMockURLProtocol.makeSession()
        let fake = FakePhysicalService(packDir: dir.resolvingSymlinksInPath().path)
        register { try fake.handle($0) }
        let physical = PhysicalReferenceSession(
            packDirectory: dir, packDigest: "sha256:pack", sessionID: "s",
            client: PhysicalReferenceAPIClient(baseURL: url, session: transport),
            membershipDigest: { "" }, author: { "op" })
        await physical.load()
        #expect(physical.availability == .ready)
        #expect(physical.lastNote?.contains("No saved physical references") == true)
        #expect(physical.draft.isEmpty && !physical.isDirty)
    }

    @Test func aFirstTurnDragAuthorsAnAxisAndUndoKeepsThePlacement() async throws {
        let dir = try SyntheticPack.write([SyntheticPack.car(at: simd_float2(5, 0))])
        let (transport, url, register) = AnnotationMockURLProtocol.makeSession()
        let fake = FakePhysicalService(packDir: dir.resolvingSymlinksInPath().path)
        register { try fake.handle($0) }
        let session = try AnnotationSession(
            pack: try AnnotationPack.open(directory: dir),
            physicalClient: PhysicalReferenceAPIClient(baseURL: url, session: transport))
        await session.physical.load()
        session.workMode = .physical
        let o = session.createObject(objectClass: "car")
        session.placePhysicalAnchor(in: .top, at: simd_float2(3, 4))
        session.beginPhysicalDrag()
        session.updatePhysicalDrag(.turn, from: simd_float2(4, 4), to: simd_float2(3, 5))
        session.endPhysicalDrag()
        let k = try #require(session.physicalKeyframe)
        #expect(k.yaw.axis == .frontRearAmbiguous && k.yaw.status == .observed)
        #expect(abs((k.yaw.yawRad ?? 0) - .pi / 2) < 1e-6)
        #expect(k.yaw.boundRad == nil && k.position.boundM == nil)
        #expect(k.position.xM == 3 && k.position.yM == 4)
        #expect(session.physical.object(o.objectID)?.body == nil)
        session.physical.undo()
        #expect(session.physicalKeyframe?.yaw.axis == .unknown)
        #expect(session.physicalKeyframe?.position.xM == 3)
    }
}
