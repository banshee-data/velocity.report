//
//  AnnotationFollowTests.swift
//  VelocityVisualiserTests
//
//  The annotation window's views and keys as an operator works through one
//  object: taking a stray return out, turning a fitted pose, stepping with the
//  arrows wherever the focus is, every view following the object chosen, and
//  its physical reference drawn in 3D.
//

import AppKit
import Foundation
import MetalKit
import SwiftUI
import Testing
import simd

@testable import VelocityVisualiser

@MainActor private func open(_ samples: [[SyntheticPack.Point]]) throws -> AnnotationSession {
    let dir = try SyntheticPack.write(samples)
    let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
    session.operatorName = "dd"
    return session
}

/// A session over `samples` whose physical references go to a fake service.
@MainActor private func openPhysical(
    _ samples: [[SyntheticPack.Point]]
) async throws -> AnnotationSession {
    let dir = try SyntheticPack.write(samples)
    let (urlSession, baseURL, register) = AnnotationMockURLProtocol.makeSession()
    let fake = FakePhysicalService(packDir: dir.resolvingSymlinksInPath().path)
    register { try fake.handle($0) }
    let session = try AnnotationSession(
        pack: try AnnotationPack.open(directory: dir),
        physicalClient: PhysicalReferenceAPIClient(baseURL: baseURL, session: urlSession))
    session.operatorName = "op"
    session.workMode = .physical
    for _ in 0..<200 where session.physical.availability != .ready {
        try? await Task.sleep(for: .milliseconds(10))
    }
    return session
}

/// A source file with its whitespace removed, so a check survives the
/// formatter wrapping a call.
private func source(_ relative: String) throws -> String {
    try String(
        contentsOf: URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().appendingPathComponent("VelocityVisualiser/\(relative)"),
        encoding: .utf8
    ).filter { !$0.isWhitespace }
}

private func keyEvent(_ code: UInt16, flags: NSEvent.ModifierFlags = []) -> NSEvent? {
    NSEvent.keyEvent(
        with: .keyDown, location: .zero, modifierFlags: flags, timestamp: 0, windowNumber: 0,
        context: nil, characters: "", charactersIgnoringModifiers: "", isARepeat: false,
        keyCode: code)
}

private func around(_ p: simd_float2, _ r: Float = 0.3) -> SelectionPolygon {
    SelectionPolygon(rectFrom: p - simd_float2(r, r), to: p + simd_float2(r, r))
}

// MARK: - Taking a return out

@MainActor struct SubtractingTests {
    /// A car return a metre and a bit above a road return, a tenth of a metre
    /// from it in plan: what the Top view shows as one dot over another.
    private let scene: [SyntheticPack.Point] = [(5, 5, -2.3, 2), (5.1, 5, -1.0, 1)]
    private let road = simd_float3(5, 5, -2.3)
    private let car = simd_float3(5.1, 5, -1.0)

    /// The car return alone in the membership, chosen in an elevation where
    /// the two are a metre apart, then back to the Top view.
    private func carSelected() throws -> AnnotationSession {
        let session = try open([scene])
        let elevation = try #require(
            OrthoViewBasis.Standard.elevations.first { session.basis($0).shows(car) })
        session.viewStandard = elevation
        #expect(session.select(polygon: around(session.basis(elevation).project(car))))
        #expect(session.history.current == [1])
        session.viewStandard = .top
        return session
    }

    @Test func aSubtractingBrushTakesItsDepthFromTheMembership() throws {
        let session = try carSelected()
        let click = session.basis(.top).project(road)
        let plain = session.brushSphere(atViewPoint: click, pickDistance: 0.5)
        #expect(
            simd_distance(plain.centre, car) > plain.radius,
            "the road's depth puts the brush a metre under the car return")
        let subtracting = session.brushSphere(
            atViewPoint: click, pickDistance: 0.5, subtracting: true)
        #expect(simd_distance(subtracting.centre, car) < subtracting.radius)
        #expect(simd_distance(subtracting.centre, road) > subtracting.radius)
        session.beginStroke()
        session.paint(sphere: subtracting)
        session.selectionMode = .subtract
        #expect(session.commitSelection())
        #expect(session.history.current.isEmpty)
    }

    @Test func aClickWithTheLassoTakesOrRemovesOneReturn() throws {
        let session = try carSelected()
        session.tool = .lasso
        let click = session.basis(.top).project(road)
        // A plain click still names nothing.
        #expect(session.clickSelect(atViewPoint: click, pickDistance: 0.5, mode: .replace) == nil)
        #expect(session.history.current == [1])
        // Option removes the nearest member, though the road is nearer.
        #expect(session.clickSelect(atViewPoint: click, pickDistance: 0.5, mode: .subtract) == nil)
        #expect(session.history.current.isEmpty)
        #expect(
            session.clickSelect(atViewPoint: click, pickDistance: 0.5, mode: .subtract)
                == session.nothingRemovedNote)
        // Shift adds the nearest return of any kind.
        #expect(session.clickSelect(atViewPoint: click, pickDistance: 0.5, mode: .add) == nil)
        #expect(session.history.current == [0])
        #expect(!session.strokeInProgress)
    }

    @Test func aStrokeInProgressIsNotUnsavedWork() throws {
        let session = try open([scene])
        _ = session.createObject(objectClass: "car")
        session.save()
        session.beginStroke()
        #expect(session.navigationGuard() == .strokeInProgress)
        #expect(!session.hasUnsavedWork, "every click flashed unsaved")
        session.cancelStroke()
        #expect(session.select(polygon: around(session.basis(.top).project(car))))
        #expect(session.hasUnsavedWork)
    }
}

// MARK: - Arrow keys

@MainActor struct ArrowKeyTests {
    @Test func theWindowTakesTheArrowsUnlessTextIsBeingEdited() throws {
        let right = try #require(keyEvent(124))
        #expect(
            AnnotationArrowKeys.takes(right, textHasFocus: false)
                == .nudge(right: 1, up: 0, coarse: false))
        #expect(AnnotationArrowKeys.takes(right, textHasFocus: true) == nil)
        let command = try #require(keyEvent(124, flags: .command))
        #expect(AnnotationArrowKeys.takes(command, textHasFocus: false) == nil)
        let s = try #require(keyEvent(1))
        #expect(AnnotationArrowKeys.takes(s, textHasFocus: false) == nil)
    }

    @Test func anArrowStepsTheFrameFromTheSession() throws {
        let car = SyntheticPack.car(at: simd_float2(5, 0))
        let session = try open([car, car, car])
        #expect(session.perform(.nudge(right: 1, up: 0, coarse: false)))
        #expect(session.sampleIndex == 1)
        #expect(session.perform(.nudge(right: -1, up: 0, coarse: false)))
        #expect(session.sampleIndex == 0)
        #expect(!session.perform(.inspectPin), "not an arrow's meaning")
    }

    @Test func theWindowInstallsTheMonitor() throws {
        let window = try source("UI/AnnotationWindow.swift")
        #expect(window.contains("AnnotationArrowKeys(session:session)"))
    }

    @Test func theModeMenuHeadsTheRightColumnOutsideItsScroll() throws {
        let window = try source("UI/AnnotationWindow.swift")
        let picker = try #require(window.range(of: "AnnotationModePicker(session:session)"))
        let editing = try #require(
            window.range(of: "AnnotationPane(session:session,column:.editing)"))
        let objects = try #require(
            window.range(of: "AnnotationPane(session:session,column:.objects)"))
        #expect(picker.lowerBound > objects.lowerBound && picker.lowerBound < editing.lowerBound)
        #expect(!(try source("UI/AnnotationPane.swift")).contains("modePicker"))
    }
}

// MARK: - The window's proportions

struct AnnotationLayoutTests {
    @Test func theViewsOpenInProportionNotByPoints() throws {
        #expect(AnnotationLayout.sceneFraction == 0.24 && AnnotationLayout.topFraction == 0.38)
        let window = try source("UI/AnnotationWindow.swift")
        #expect(!window.contains("VSplitView") && !window.contains("HSplitView"))
        #expect(window.contains("FractionSplit(axis:.vertical,fraction:$sceneFraction"))
        #expect(window.contains("FractionSplit(axis:.horizontal,fraction:$topFraction"))
    }

    @Test func aDividerDragMovesTheFractionAndStaysInRange() {
        typealias Split = FractionSplit<EmptyView, EmptyView>
        #expect(
            abs(Split.dragged(from: 0.24, by: 100, over: 1000, range: 0.1...0.8) - 0.34) < 1e-12)
        #expect(Split.dragged(from: 0.24, by: -500, over: 1000, range: 0.1...0.8) == 0.1)
        #expect(Split.dragged(from: 0.5, by: 900, over: 1000, range: 0.1...0.8) == 0.8)
        #expect(Split.dragged(from: 0.5, by: 10, over: 0, range: 0.1...0.8) == 0.5)
    }
}

// MARK: - Turning a pose

struct TurnAboutTheCentreTests {
    /// A pose anchored on its front face, two metres ahead of the centre.
    private func frontAnchored() -> PhysicalKeyframe {
        var k = PhysicalKeyframe(keyframeID: "kf", sampleID: 0, timestampNs: 1)
        PhysicalDraft.setAxis(.resolved, of: &k)
        k.yaw.yawRad = 0
        PhysicalDraft.setAnchor(.frontFace, of: &k)
        k.anchor.offsetM = 2
        k.anchor.offsetBoundM = 0.1
        k.position.xM = 12
        k.position.yM = 3
        return k
    }

    @Test func aFaceAnchorSwingsRoundTheCentre() throws {
        var k = frontAnchored()
        let pivot = try #require(PhysicalDraft.turnPivot(of: k))
        #expect(abs(pivot.x - 10) < 1e-9 && abs(pivot.y - 3) < 1e-9)
        PhysicalDraft.turn(&k, toRad: .pi / 2)
        #expect(abs((k.position.xM ?? 0) - 10) < 1e-9 && abs((k.position.yM ?? 0) - 5) < 1e-9)
        let after = try #require(PhysicalDraft.turnPivot(of: k))
        #expect(abs(after.x - 10) < 1e-9 && abs(after.y - 3) < 1e-9, "the centre moved")
    }

    @Test func aCentreAnchorStaysWhereItIs() {
        var k = PhysicalKeyframe(keyframeID: "kf", sampleID: 0, timestampNs: 1)
        PhysicalDraft.setAxis(.frontRearAmbiguous, of: &k)
        k.position.xM = 4
        k.position.yM = 1
        PhysicalDraft.turn(&k, toRad: 1)
        #expect(k.position.xM == 4 && k.position.yM == 1 && k.yaw.yawRad == 1)
    }

    @Test @MainActor func aTurnHandleDragIsTheSameWhicheverWayThePointerCame() async throws {
        let session = try await openPhysical([SyntheticPack.car(at: simd_float2(9, 2.7))])
        let object = session.createObject(objectClass: "car")
        session.placePhysicalAnchor(in: .top, at: simd_float2(12, 3))
        let sampleID = try #require(session.currentSample?.sampleID)
        session.physical.edit { objects in
            PhysicalDraft.updateKeyframe(
                objectID: object.objectID, sampleID: sampleID, in: &objects
            ) {
                // As a fit leaves a pose that saw only its front: anchored on
                // the front face, half a length ahead of the centre.
                PhysicalDraft.setAxis(.resolved, of: &$0)
                $0.yaw.yawRad = 0
                PhysicalDraft.setAnchor(.frontFace, of: &$0)
                $0.anchor.offsetM = 2
                $0.anchor.offsetBoundM = 0.1
            }
        }
        session.beginPhysicalDrag()
        session.updatePhysicalDrag(.turn, from: simd_float2(11, 3), to: simd_float2(10, 5))
        let once = try #require(session.physicalKeyframe)
        session.updatePhysicalDrag(.turn, from: simd_float2(11, 3), to: simd_float2(7, 3))
        session.updatePhysicalDrag(.turn, from: simd_float2(11, 3), to: simd_float2(10, 5))
        #expect(session.physicalKeyframe == once, "each update compounded the last")
        session.endPhysicalDrag()
        let k = try #require(session.physicalKeyframe)
        let centre = try #require(PhysicalDraft.turnPivot(of: k))
        #expect(abs(centre.x - 10) < 1e-9 && abs(centre.y - 3) < 1e-9)
        #expect(abs((k.yaw.yawRad ?? 0) - .pi / 2) < 1e-9)
    }
}

// MARK: - Following an object

struct ObjectFollowGeometryTests {
    @Test func aFrameBetweenTwoLabelledOnesIsPartWayBetweenThem() throws {
        #expect(ObjectLifecycle.bracket(1, labelled: [2, 5, 9]) == nil)
        #expect(ObjectLifecycle.bracket(10, labelled: [2, 5, 9]) == nil)
        let at = try #require(ObjectLifecycle.bracket(5, labelled: [2, 5, 9]))
        #expect(at.before == 5 && at.after == 5 && at.fraction == 0)
        let between = try #require(ObjectLifecycle.bracket(3, labelled: [2, 5, 9]))
        #expect(between.before == 2 && between.after == 5 && abs(between.fraction - 1 / 3) < 1e-6)
    }

    @Test func aFootprintInterpolatesEachViewItSees() throws {
        let bases = [(OrthoViewBasis.Standard.top, OrthoViewBasis(.top, azimuthDeg: 0))]
        let a = try #require(
            ObjectFootprint.of([simd_float3(0, 0, -1), simd_float3(2, 1, -0.5)], bases: bases))
        let b = try #require(
            ObjectFootprint.of([simd_float3(10, 0, -1), simd_float3(14, 1, -0.5)], bases: bases))
        let mid = a.interpolated(to: b, fraction: 0.5)
        #expect(simd_distance(mid.centre, simd_float3(6.5, 0.5, -0.75)) < 1e-5)
        let top = try #require(mid.extents[.top])
        #expect(abs(top.halfWidth - 1.5) < 1e-5)
    }

    @Test func aViewGrowsToKeepTheObjectWholeAndNeverShrinks() {
        var framing = ObjectFollowFraming()
        var extents: [OrthoViewBasis.Standard: AnnotationExtent] = [:]
        var states: [OrthoViewBasis.Standard: OrthoViewState] = [
            .top: .init(centre: .zero, halfHeight: 3)
        ]
        func footprint(_ x: Float, _ half: Float) -> ObjectFootprint {
            ObjectFootprint(
                extents: [
                    .top: AnnotationExtent(
                        centre: simd_float2(x, 0), halfHeight: half, halfWidth: half)
                ], lower: .zero, upper: .zero)
        }
        framing.fit(footprint(0, 2), extents: &extents, states: &states)
        #expect(states[.top] == nil, "clicking an object discards the operator's framing")
        framing.follow(footprint(5, 1), extents: &extents, states: &states)
        #expect(
            extents[.top]
                == AnnotationExtent(centre: simd_float2(5, 0), halfHeight: 2, halfWidth: 2))
        framing.follow(footprint(6, 3), extents: &extents, states: &states)
        #expect(extents[.top]?.halfHeight == 3)
        // A view the operator moves keeps their offset and moves with it.
        states[.top] = OrthoViewState(centre: simd_float2(7, 1), halfHeight: 1)
        framing.follow(footprint(8, 1), extents: &extents, states: &states)
        #expect(states[.top] == OrthoViewState(centre: simd_float2(9, 1), halfHeight: 1))
    }

    @Test func theCameraLooksAlongTheSensorsLineOfSight() {
        var camera = Camera()
        let focus = AnnotationSceneFocus(
            centre: simd_float3(20, 10, -1.5), radius: 3, revision: 1, fromSensor: true)
        camera.lookFromSensor(at: focus)
        #expect(camera.target == focus.centre)
        let back = simd_normalize(
            simd_float2(camera.position.x, camera.position.y) - simd_float2(20, 10))
        let outward = simd_normalize(simd_float2(20, 10))
        #expect(
            simd_dot(back, outward) < -0.999, "the camera is on the sensor's side of the object")
        let distance = simd_distance(camera.position, focus.centre)
        let halfFov = camera.fov * .pi / 360
        #expect(distance >= focus.radius / sin(halfFov), "the object's sphere is inside the view")
        var near = camera
        near.lookFromSensor(
            at: AnnotationSceneFocus(centre: focus.centre, radius: 0.5, revision: 2))
        #expect(simd_distance(near.position, focus.centre) >= 10 - 1e-4, "never nearer than 10 m")
        let rise = camera.position.z - focus.centre.z
        let run = simd_distance(
            simd_float2(camera.position.x, camera.position.y), simd_float2(20, 10))
        #expect(atan2(rise, run) >= 0.21 - 1e-4, "looks down on the object at least that steeply")
    }

    @Test func theCameraDistanceOnlyGrowsWhileFollowing() {
        var framing = ObjectFollowFraming()
        var extents: [OrthoViewBasis.Standard: AnnotationExtent] = [:]
        var states: [OrthoViewBasis.Standard: OrthoViewState] = [:]
        func sphere(_ r: Float) -> ObjectFootprint {
            ObjectFootprint(extents: [:], lower: simd_float3(-r, 0, 0), upper: simd_float3(r, 0, 0))
        }
        framing.fit(sphere(2), extents: &extents, states: &states)
        #expect(framing.radius == 2)
        framing.follow(sphere(1), extents: &extents, states: &states)
        #expect(framing.radius == 2, "a car showing only its front shrank the 3D view onto it")
        framing.follow(sphere(3), extents: &extents, states: &states)
        #expect(framing.radius == 3)
        framing.fit(sphere(1), extents: &extents, states: &states)
        #expect(framing.radius == 1, "clicking again starts over")
    }

    @Test func aGlideEasesFromOneCameraToTheNext() {
        var a = Camera()
        a.position = simd_float3(0, 0, 10)
        a.target = .zero
        var b = a
        b.position = simd_float3(10, 0, 10)
        b.target = simd_float3(10, 0, 0)
        #expect(Camera.glide(from: a, to: b, progress: 0).position == a.position)
        #expect(Camera.glide(from: a, to: b, progress: 1).target == b.target)
        #expect(Camera.glide(from: a, to: b, progress: 0.5).position == simd_float3(5, 0, 10))
        #expect(Camera.glide(from: a, to: b, progress: 0.1).position.x < 1, "eased, not linear")
    }
}

@MainActor struct ObjectFollowSessionTests {
    /// A car driving along +X: at 5, 7 and 9 m. Labelled in the first and
    /// last frames only.
    private func drivingCar() throws -> (AnnotationSession, String) {
        let frames = [5, 7, 9].map {
            SyntheticPack.car(at: simd_float2(Float($0), 0)) + SyntheticPack.wall
        }
        let session = try open(frames)
        let object = session.createObject(objectClass: "car")
        let box = { (x: Float) in
            SelectionPolygon(rectFrom: simd_float2(x - 1, -1), to: simd_float2(x + 1.6, 1.6))
        }
        #expect(session.select(polygon: box(5)))
        #expect(session.save())
        #expect(session.step(to: 2) == nil)
        #expect(session.select(polygon: box(9)))
        #expect(session.save())
        #expect(session.step(to: 1) == nil)
        session.fitViews(to: .sample)
        return (session, object.objectID)
    }

    private func topCentre(_ session: AnnotationSession) -> simd_float2? {
        session.referenceExtents[.top]?.centre
    }

    @Test func clickingAnObjectFramesItAndEveryStepFollowsIt() throws {
        let (session, objectID) = try drivingCar()
        let top = session.basis(.top)
        #expect(session.activate(objectID: objectID) == nil)
        #expect(session.sampleIndex == 0 && session.focusedObjectID == objectID)
        #expect(session.referenceFramesObject)
        #expect(session.sceneFocus?.fromSensor == true && session.sceneFocus?.animated == true)
        let first = try #require(topCentre(session))
        #expect(simd_distance(first, top.project(simd_float3(5.3, 0.3, 0))) < 1e-4)

        // Not labelled here: between the two frames either side.
        #expect(session.step(to: 1) == nil)
        let between = try #require(topCentre(session))
        #expect(simd_distance(between, top.project(simd_float3(7.3, 0.3, 0))) < 1e-4)
        #expect(session.step(to: 2) == nil)
        let last = try #require(topCentre(session))
        #expect(simd_distance(last, top.project(simd_float3(9.3, 0.3, 0))) < 1e-4)

        // A view the operator moved keeps where they put it, relative to the car.
        session.pan(
            .top, size: CGSize(width: 400, height: 400), byPoints: CGSize(width: 40, height: 0))
        let moved = try #require(session.viewStates[.top]?.centre)
        #expect(session.step(to: 1) == nil)
        let carried = try #require(session.viewStates[.top]?.centre)
        #expect(simd_distance(carried - moved, between - last) < 1e-4)
    }

    @Test func askingForAnotherFitStopsFollowing() throws {
        let (session, objectID) = try drivingCar()
        _ = session.activate(objectID: objectID)
        session.fitViews(to: .sample)
        #expect(session.focusedObjectID == nil && !session.referenceFramesObject)
        let fitted = topCentre(session)
        #expect(session.step(to: 2) == nil)
        #expect(topCentre(session) == fitted)
    }

    @Test func turningFollowingOffLeavesTheViewsAlone() throws {
        let (session, objectID) = try drivingCar()
        _ = session.activate(objectID: objectID)
        session.followsFocusedObject = false
        let framed = topCentre(session)
        #expect(session.step(to: 2) == nil)
        #expect(topCentre(session) == framed)
    }
}

// MARK: - The 3D view

@MainActor struct PhysicalSceneBoxTests {
    @Test func aWholeFootprintStandsOnTheObjectsLowestReturn() async throws {
        let session = try await openPhysical([SyntheticPack.car(at: simd_float2(9, 2.7))])
        let object = session.createObject(objectClass: "car")
        #expect(session.select(polygon: around(simd_float2(9.3, 3), 1)))
        #expect(session.physicalSceneBoxes.isEmpty, "no keyframe, no box")
        session.placePhysicalAnchor(in: .top, at: simd_float2(9.3, 3))
        let sampleID = try #require(session.currentSample?.sampleID)
        session.physical.edit { objects in
            PhysicalDraft.addBody(
                objectID: object.objectID, author: "op", session: "s", to: &objects)
            PhysicalDraft.updateBody(objectID: object.objectID, in: &objects) {
                $0.length = PhysicalDimension(
                    status: .observed, span: .full, lowerM: 4, upperM: 5, valueM: 4.5,
                    support: .init(frames: [sampleID]))
                $0.width = PhysicalDimension(
                    status: .observed, span: .full, lowerM: 1.7, upperM: 1.9, valueM: 1.8,
                    support: .init(frames: [sampleID]))
            }
            PhysicalDraft.updateKeyframe(
                objectID: object.objectID, sampleID: sampleID, in: &objects
            ) {
                PhysicalDraft.setAxis(.resolved, of: &$0)
                $0.yaw.yawRad = 0.3
                $0.yaw.boundRad = 0.05
                $0.position.boundM = 0.2
            }
        }
        let box = try #require(session.physicalSceneBoxes.first)
        #expect(session.physicalSceneBoxes.count == 1)
        #expect(abs(box.base.x - 9.3) < 1e-5 && abs(box.base.y - 3) < 1e-5)
        #expect(abs(box.base.z - -1) < 1e-5, "stands on the car's lowest return")
        #expect(abs(box.height - 0.3) < 1e-5, "as tall as its returns when no height is known")
        #expect(abs(box.length - 4.5) < 1e-5 && abs(box.width - 1.8) < 1e-5)
        #expect(abs(box.yawRad - 0.3) < 1e-5)
        #expect(box.colour.x == PhysicalReferenceOverlay.Style.unsaved.rgb.x)
        // The elevations stand the box on the same span.
        let span = session.physicalBoxSpan(
            objectID: object.objectID, body: session.physical.object(object.objectID)?.body)
        #expect(
            span.lowerBound == box.base.z
                && abs(span.upperBound - span.lowerBound - box.height) < 1e-5)
        session.physical.edit { objects in
            PhysicalDraft.updateBody(objectID: object.objectID, in: &objects) {
                $0.height = PhysicalDimension(
                    status: .observed, span: .full, lowerM: 1.4, upperM: 1.6, valueM: 1.5,
                    support: .init(frames: [sampleID]))
            }
        }
        let tall = try #require(session.physicalSceneBoxes.first)
        #expect(abs(tall.height - 1.5) < 1e-5, "a stated height is drawn")
        session.workMode = .points
        #expect(session.physicalSceneBoxes.isEmpty, "drawn in physical mode only")
    }

    @Test func theRendererKeepsOverlayBoxesApartFromTracks() throws {
        guard let device = MTLCreateSystemDefaultDevice() else { return }
        let view = MTKView()
        view.device = device
        guard let renderer = MetalRenderer(metalView: view) else { return }
        let box = MetalRenderer.OverlayBox(
            base: simd_float3(1, 2, -1), yawRad: 0.5, length: 4, width: 2, height: 1.5,
            colour: simd_float4(1, 0, 0, 1))
        renderer.setOverlayBoxes([box])
        #expect(renderer.overlayBoxes == [box])
        renderer.showBoxes = false
        #expect(renderer.overlayBoxes == [box], "showBoxes is about tracks")
        renderer.setOverlayBoxes([])
        #expect(renderer.overlayBoxes.isEmpty)
    }
}
