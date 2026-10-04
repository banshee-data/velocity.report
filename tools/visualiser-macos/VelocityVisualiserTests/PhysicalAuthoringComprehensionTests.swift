import Foundation
import Testing
import simd

@testable import VelocityVisualiser

@MainActor struct PhysicalAuthoringComprehensionTests {
    @Test func theProductionSessionKeepsEstimateExposureInItsPreferencesStore() throws {
        let suite = "annotation-exposure-wiring-\(UUID().uuidString)"
        let preferences = try #require(UserDefaults(suiteName: suite))
        defer { preferences.removePersistentDomain(forName: suite) }
        let dir = try SyntheticPack.write([SyntheticPack.car(at: SIMD2(5, 0))])
        defer { try? FileManager.default.removeItem(at: dir) }
        let pack = try AnnotationPack.open(directory: dir)
        let first = try AnnotationSession(pack: pack, defaults: preferences)
        first.physical.expose(objectIDs: ["car"], source: "run 7 · online")
        let reopened = try AnnotationSession(pack: pack, defaults: preferences)
        #expect(reopened.physical.exposure["car"] == "run 7 · online")
        let isolated = try AnnotationSession(pack: pack, defaults: nil)
        #expect(isolated.physical.exposure.isEmpty)
        #expect(PhysicalAnchorKind.leftFace.label == "Left face centre")
    }

    @Test func invalidDraftBoundsDoNotEnterTheOverlayOrSupportedGeometry() {
        var k = PhysicalKeyframe(keyframeID: "k", sampleID: 0, timestampNs: 10)
        k.position = PhysicalPosition(status: .observed, xM: 3, yM: 4, boundM: 0.2)
        k.yaw = PhysicalYaw(status: .observed, axis: .resolved, yawRad: 0, boundRad: 0.05)
        for value in [Double.nan, .infinity, -1] {
            var invalid = k
            invalid.position.boundM = value
            let o = PhysicalObject(objectID: "o", keyframes: [invalid])
            for gate in [PhysicalGeometryGate.authoring, .preview, .truth] {
                #expect(
                    PhysicalGeometry.derive(object: o, keyframe: invalid, gate: gate).anchorPoint
                        == nil)
            }
            invalid = k
            invalid.yaw.boundRad = value
            #expect(
                PhysicalGeometry.derive(
                    object: PhysicalObject(objectID: "o"), keyframe: invalid, gate: .authoring
                ).yaw == nil)
        }
        k.yaw.boundRad = 2 * .pi
        #expect(
            PhysicalGeometry.derive(
                object: PhysicalObject(objectID: "o"), keyframe: k, gate: .authoring
            ).yaw == nil)
    }

    @Test func partialOrInvalidDimensionIntervalsAreNotWholeObjectSizes() {
        for d in [
            PhysicalDimension(status: .observed, span: .full, lowerM: 4, upperM: 3),
            PhysicalDimension(status: .observed, span: .full, lowerM: -1, upperM: 3),
            PhysicalDimension(status: .observed, span: .full, lowerM: 2, upperM: .infinity),
            PhysicalDimension(status: .observed, span: .full, lowerM: 2, upperM: 3, valueM: 4),
            PhysicalDimension(status: .observed, span: .partial, lowerM: 2, upperM: 3),
        ] {
            let o = PhysicalObject(objectID: "o", body: PhysicalBody(bodyID: "b", length: d))
            let k = PhysicalKeyframe(keyframeID: "k", sampleID: 0, timestampNs: 10)
            #expect(PhysicalGeometry.derive(object: o, keyframe: k, gate: .authoring).length == nil)
        }
        let huge = PhysicalDimension(status: .observed, span: .full, lowerM: 1e308, upperM: 1.7e308)
        #expect(huge.best == nil, "An overflowing midpoint is not a size, and not an infinity")
    }

    @Test func theMidpointIsGosArithmeticToTheLastBit() {
        // Go's Best is (lo + hi) / 2. Halving each end first agrees with it
        // except at the two ends of the range: at the top, where the sum
        // overflows (above), and at the bottom, where halving rounds. There
        // it put the midpoint outside the interval.
        let tiny = Double.leastNonzeroMagnitude
        let d = PhysicalDimension(status: .observed, span: .full, lowerM: tiny, upperM: tiny)
        #expect(d.best?.value == (tiny + tiny) / 2)
        #expect(d.best?.halfWidth == 0)
        let car = PhysicalDimension(status: .observed, span: .full, lowerM: 4.1, upperM: 4.7)
        #expect(car.best?.value == (4.1 + 4.7) / 2)
        let stated = PhysicalDimension(
            status: .observed, span: .full, lowerM: 4.1, upperM: 4.7, valueM: 4.2)
        #expect(stated.best?.value == 4.2 && stated.best?.halfWidth == 4.7 - 4.2)
    }

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

@MainActor struct PhysicalWidthHandleTests {
    @Test func widthUsesTheTransverseAxisAndLeavesASideAnchorUncoupled() {
        let centre = PhysicalPlanar(x: 10, y: 5, bound: 0.2)
        #expect(PhysicalHandles.draggedWidth(centre: centre, yawRad: 0, pointer: SIMD2(15, 7)) == 4)
        #expect(
            abs(
                PhysicalHandles.draggedWidth(centre: centre, yawRad: .pi / 2, pointer: SIMD2(8, 12))
                    - 4) < 1e-9)
        #expect(
            PhysicalHandles.draggedWidth(centre: centre, yawRad: 0, pointer: SIMD2(10, 5)) == 0.1)
        var g = PhysicalGeometry(anchorKind: .bodyCentre)
        g.centre = centre
        g.yaw = PhysicalAngle(rad: 0, boundRad: 0.1, axis: .frontRearAmbiguous, status: .observed)
        g.width = PhysicalLinear(
            lower: 1.8, upper: 2.2, value: 2, halfWidth: 0.2, status: .observed)
        #expect(PhysicalHandles.widthHandlePoint(g)?.y == 6)
        g.anchorKind = .leftFace
        #expect(
            PhysicalHandles.widthHandlePoint(g) == nil,
            "a resize silently moved the fixed face's offset")
    }

    @Test func draggingWidthKeepsPoseAndLengthAndIsOneUndoStep() async throws {
        let dir = try SyntheticPack.write([SyntheticPack.car(at: SIMD2(5, 0))])
        let (transport, url, register) = AnnotationMockURLProtocol.makeSession()
        let fake = FakePhysicalService(packDir: dir.resolvingSymlinksInPath().path)
        register { try fake.handle($0) }
        let s = try AnnotationSession(
            pack: try AnnotationPack.open(directory: dir),
            physicalClient: PhysicalReferenceAPIClient(baseURL: url, session: transport))
        await s.physical.load()
        s.workMode = .physical
        let object = s.createObject(objectClass: "car")
        s.placePhysicalAnchor(in: .top, at: SIMD2(10, 5))
        s.physical.edit { objects in
            PhysicalDraft.addBody(
                objectID: object.objectID, author: "op", session: "s", to: &objects)
            PhysicalDraft.updateBody(objectID: object.objectID, in: &objects) {
                $0.width = PhysicalDimension(
                    status: .observed, span: .full, lowerM: 1.8, upperM: 2.2, valueM: 2)
                $0.length = PhysicalDimension(
                    status: .observed, span: .full, lowerM: 4, upperM: 5, valueM: 4.5)
            }
            PhysicalDraft.updateKeyframe(objectID: object.objectID, sampleID: 0, in: &objects) {
                PhysicalDraft.setAxis(.resolved, of: &$0)
                $0.yaw.yawRad = 0
                $0.yaw.boundRad = 0.05
            }
        }
        let pose = try #require(s.physicalKeyframe)
        let g = try #require(s.physicalPreview)
        let viewport = OrthoViewport(
            halfHeight: 10, size: CGSize(width: 400, height: 400), centre: SIMD2(10, 5))
        let at = try #require(
            PhysicalHandles.positions(g, basis: s.basis(.top), viewport: viewport).width)
        #expect(s.physicalHandle(at: at, viewport: viewport) == .width)
        s.beginPhysicalDrag()
        s.updatePhysicalDrag(.width, from: SIMD2(10, 6), to: SIMD2(10, 6.5))
        s.updatePhysicalDrag(.width, from: SIMD2(10, 6), to: SIMD2(10, 7))
        s.endPhysicalDrag()
        #expect(s.physicalKeyframe == pose)
        let body = try #require(s.physical.object(object.objectID)?.body)
        #expect(body.width.valueM == 4 && abs((body.width.lowerM ?? 0) - 3.8) < 1e-6)
        #expect(body.length.valueM == 4.5)
        s.physical.undo()
        #expect(s.physical.object(object.objectID)?.body?.width.valueM == 2)
    }
}

struct GuidedObjectSizeTests {
    @Test func valueThenExplicitToleranceDoesNotInventEvidenceOrPrecision() {
        var unknown = PhysicalDimension()
        PhysicalDraft.setDimensionValue(4.5, of: &unknown)
        #expect(unknown == PhysicalDimension())
        var d = PhysicalDimension(
            status: .priorOnly, span: .full,
            support: PhysicalSupport(external: "car size assumption"))
        PhysicalDraft.setDimensionValue(4.5, of: &d)
        #expect(d.valueM == 4.5 && d.lowerM == nil && d.upperM == nil && d.status == .priorOnly)
        #expect(PhysicalDraft.setDimensionTolerance(0.5, of: &d))
        #expect(d.lowerM == 4 && d.upperM == 5 && d.status == .priorOnly)
        PhysicalDraft.setDimensionValue(5, of: &d)
        #expect(d.lowerM == 4.5 && d.upperM == 5.5)
        #expect(PhysicalDraft.setDimensionTolerance(nil, of: &d))
        #expect(d.valueM == 5 && d.lowerM == nil && d.upperM == nil)
    }

    @Test func partialAndInvalidToleranceAreRefusedAndIntervalsCanBeAsymmetric() {
        var partial = PhysicalDimension(status: .observed, span: .partial, lowerM: 2)
        let original = partial
        PhysicalDraft.setDimensionValue(4, of: &partial)
        #expect(!PhysicalDraft.setDimensionTolerance(1, of: &partial) && partial == original)
        var d = PhysicalDimension(status: .inferred, span: .full, lowerM: 4, upperM: 6, valueM: 4.5)
        PhysicalDraft.setDimensionValue(5, of: &d)
        #expect(d.lowerM == 4.5 && d.upperM == 6.5)
        let was = d
        for tolerance in [-1, Double.nan, Double.infinity] {
            #expect(!PhysicalDraft.setDimensionTolerance(tolerance, of: &d) && d == was)
        }
        #expect(PhysicalDraft.setDimensionTolerance(10, of: &d))
        #expect(d.lowerM == 0 && d.upperM == 15)
    }

    @Test func aPriorSizeSketchIsVisibleButCannotBecomeScoredGeometry() {
        var body = PhysicalBody(bodyID: "b")
        body.length = PhysicalDimension(status: .priorOnly, span: .full, valueM: 4.5)
        body.width = PhysicalDimension(status: .priorOnly, span: .full, valueM: 2)
        var k = PhysicalKeyframe(keyframeID: "k", sampleID: 0, timestampNs: 1)
        k.position = PhysicalPosition(status: .observed, xM: 2, yM: 3)
        k.yaw = PhysicalYaw(status: .observed, axis: .frontRearAmbiguous, yawRad: 0.5)
        let object = PhysicalObject(objectID: "o", body: body, keyframes: [k])
        let sketch = PhysicalGeometry.derive(object: object, keyframe: k, gate: .authoring)
        #expect(sketch.box?.length == 4.5 && sketch.box?.width == 2)
        #expect(sketch.unboundedDraft && sketch.usesPriorSize)
        #expect(PhysicalGeometry.derive(object: object, keyframe: k, gate: .preview).box == nil)
        #expect(PhysicalGeometry.derive(object: object, keyframe: k).box == nil)
        #expect(body.length.lowerM == nil && k.position.boundM == nil && k.yaw.boundRad == nil)
    }
}
