//
//  AnnotationViewStateTests.swift
//  VelocityVisualiserTests
//
//  Tests for what the annotation views show and from where: class visibility
//  and what it does to selection, framing that holds still between samples,
//  pan and zoom, frame sync with the main view, and the frame the 3D view
//  draws.
//

import CoreGraphics
import Foundation
import Testing
import simd

@testable import VelocityVisualiser

@MainActor
private func makeSession() throws -> (AnnotationSession, URL) {
    let dir = try PackFixture.write()
    let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
    session.operatorName = "dd"
    return (session, dir)
}

private let viewSize = CGSize(width: 400, height: 200)

/// Fixture sample 0: three foreground returns near (1.5, 1.25) and one ground
/// return far away at (40, -30).
private let farOutlierLasso = SelectionPolygon(
    rectFrom: simd_float2(39, -31), to: simd_float2(41, -29))
private let everythingLasso = SelectionPolygon(
    rectFrom: simd_float2(-100, -100), to: simd_float2(100, 100))

// MARK: - Visibility

struct PointVisibilityTests {
    @Test func eachToggleHidesItsOwnClass() {
        var visibility = PointVisibility()
        #expect(visibility.showsEverything)
        visibility.background = false
        #expect(!visibility.shows(PointClass.background))
        #expect(visibility.shows(PointClass.foreground))
        #expect(visibility.shows(PointClass.ground))
        #expect(!visibility.showsEverything)
    }

    @Test func aClassThisClientDoesNotKnowIsShown() {
        // A newer recorder adding a class must not make returns vanish.
        let nothing = PointVisibility(background: false, foreground: false, ground: false)
        #expect(nothing.shows(9))
    }

    @Test func noFilterShowsEveryPoint() {
        let classes: [UInt8] = [PointClass.background, PointClass.ground]
        #expect(PointVisibility.isVisible(0, classes: classes, under: nil))
        #expect(PointVisibility.isVisible(1, classes: classes, under: nil))
        let noGround = PointVisibility(background: true, foreground: true, ground: false)
        #expect(PointVisibility.isVisible(0, classes: classes, under: noGround))
        #expect(!PointVisibility.isVisible(1, classes: classes, under: noGround))
    }
}

@MainActor
struct VisibilitySelectionTests {
    @Test func aHiddenClassIsNotSelectableAndTheCountSaysWhy() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        session.beginStroke()
        #expect(session.previewSelection(polygon: farOutlierLasso).count == 1)

        session.visibility.ground = false
        session.beginStroke()
        let hidden = session.previewSelection(polygon: farOutlierLasso)
        #expect(hidden.count == 0)
        #expect(hidden.excludedByVisibility == 1)
    }

    @Test func aLassoOverEverythingTakesOnlyWhatIsShown() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        session.visibility.foreground = false
        #expect(session.select(polygon: everythingLasso, mode: .replace))
        #expect(session.canonicalSelection == [3])
    }

    @Test func aSphereCannotBeCentredOnAHiddenReturn() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        let overOutlier = simd_float2(40, -30)
        #expect(session.nearestPointIndex(toViewPoint: overOutlier, maxViewDistance: 1) == 3)
        session.visibility.ground = false
        #expect(session.nearestPointIndex(toViewPoint: overOutlier, maxViewDistance: 1) == nil)
    }

    @Test func changingTheFilterAbandonsAStrokePreviewedUnderTheOldOne() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        session.beginStroke()
        session.previewSelection(polygon: everythingLasso)
        session.visibility.ground = false

        #expect(session.pendingCandidates == nil)
        #expect(session.navigationGuard() == nil)
    }

    @Test func everyClassOnIsNoFilterAtAll() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(session.effectiveVisibility == nil)
        session.visibility.background = false
        #expect(session.effectiveVisibility == session.visibility)
    }
}

// MARK: - Framing

struct OrthoViewStateTests {
    @Test func panningMovesTheContentWithTheDrag() {
        var state = OrthoViewState(centre: .zero, halfHeight: 10)
        // Dragging right and down: the content follows, so the view's centre
        // moves left and, because screen y is flipped, up.
        state.pan(byPoints: CGSize(width: 20, height: 10), metresPerPoint: 0.1)
        #expect(abs(state.centre.x + 2) < 1e-5)
        #expect(abs(state.centre.y - 1) < 1e-5)
        #expect(state.halfHeight == 10)
    }

    @Test func zoomKeepsTheAnchorWhereItIsOnScreen() {
        var state = OrthoViewState(centre: simd_float2(3, -2), halfHeight: 10)
        let anchor = simd_float2(8, 1)
        let before = OrthoViewport(halfHeight: state.halfHeight, size: viewSize, centre: state.centre)
            .screenPoint(from: anchor)

        state.zoom(by: 0.5, about: anchor)

        let after = OrthoViewport(halfHeight: state.halfHeight, size: viewSize, centre: state.centre)
            .screenPoint(from: anchor)
        #expect(state.halfHeight == 5)
        #expect(abs(before.x - after.x) < 1e-3)
        #expect(abs(before.y - after.y) < 1e-3)
    }

    @Test func atTheZoomLimitTheViewStopsMovingToo() {
        var state = OrthoViewState(
            centre: simd_float2(3, -2), halfHeight: OrthoViewState.minimumHalfHeight)
        state.zoom(by: 0.5, about: simd_float2(50, 50))
        // The scale could not change, so neither may the centre: otherwise
        // scrolling at the limit would slide the view towards the cursor.
        #expect(state.halfHeight == OrthoViewState.minimumHalfHeight)
        #expect(state.centre == simd_float2(3, -2))

        state.halfHeight = OrthoViewState.maximumHalfHeight
        state.zoom(by: 2, about: simd_float2(50, 50))
        #expect(state.halfHeight == OrthoViewState.maximumHalfHeight)
    }

    @Test func aNonsenseFactorIsIgnored() {
        var state = OrthoViewState(centre: .zero, halfHeight: 10)
        state.zoom(by: 0, about: .zero)
        state.zoom(by: .nan, about: .zero)
        #expect(state == OrthoViewState(centre: .zero, halfHeight: 10))
    }

    @Test func scrollingUpZoomsInAndOneDeltaCannotRunAway() {
        #expect(ViewportInputView.zoomFactor(forScroll: 1) < 1)
        #expect(ViewportInputView.zoomFactor(forScroll: -1) > 1)
        #expect(ViewportInputView.zoomFactor(forScroll: 1000) == 0.5)
        #expect(ViewportInputView.zoomFactor(forScroll: -1000) == 2)
    }
}

struct TrimmedExtentTests {
    /// A hundred returns along a street and one at the edge of the sensor's range.
    private var street: PackPoints {
        var x = (0..<100).map { Float($0) * 0.1 }
        var y = [Float](repeating: 0, count: 100)
        x.append(180)
        y.append(180)
        return PackPoints(
            x: x, y: y, z: [Float](repeating: 0, count: 101),
            intensity: [UInt8](repeating: 0, count: 101),
            classification: [UInt8](repeating: 0, count: 101))
    }

    @Test func trimmingLeavesTheStragglerOutOfTheFrame() throws {
        let basis = OrthoViewBasis(.top)
        let whole = try #require(annotationExtent(of: street, basis: basis, trim: 0) { _ in true })
        let trimmed = try #require(
            annotationExtent(of: street, basis: basis, trim: 0.01) { _ in true })
        #expect(whole.halfWidth == 90)
        // The street is 9.9 m long; one return in a hundred and one is dropped
        // from each end.
        #expect(trimmed.halfWidth < 5)
        #expect(trimmed.halfHeight == 0)
    }

    @Test func onlyTheIncludedPointsAreMeasured() throws {
        let extent = try #require(
            annotationExtent(of: street, basis: OrthoViewBasis(.top), trim: 0) { $0 < 11 })
        #expect(abs(extent.halfWidth - 0.5) < 1e-5)
        #expect(abs(extent.centre.x - 0.5) < 1e-5)
    }

    @Test func nothingIncludedIsNoExtentRatherThanAZeroOne() {
        #expect(annotationExtent(of: street, basis: OrthoViewBasis(.top), trim: 0) { _ in false } == nil)
    }
}

@MainActor
struct SessionFramingTests {
    @Test func steppingDoesNotMoveOrRescaleTheViews() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        // The two fixture samples have different extents, so a view framed on
        // each sample in turn would jump here.
        let before = OrthoViewBasis.Standard.allCases.map {
            session.viewport(for: $0, size: viewSize)
        }
        #expect(session.stepForward() == nil)
        #expect(session.currentSample?.sampleID == 1)
        let after = OrthoViewBasis.Standard.allCases.map {
            session.viewport(for: $0, size: viewSize)
        }
        #expect(before == after)
    }

    @Test func whereTheOperatorLeftAViewIsWhereItStays() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        session.zoom(.top, size: viewSize, by: 0.5, aboutScreenPoint: CGPoint(x: 100, y: 50))
        session.pan(.top, size: viewSize, byPoints: CGSize(width: 30, height: -10))
        let placed = session.viewport(for: .top, size: viewSize)

        session.stepForward()
        #expect(session.viewport(for: .top, size: viewSize) == placed)
        // A window resize keeps the scale and the centre, and only the size
        // that was passed in changes.
        let resized = session.viewport(for: .top, size: CGSize(width: 800, height: 600))
        #expect(resized.halfHeight == placed.halfHeight)
        #expect(resized.centre == placed.centre)
    }

    @Test func panningOneViewLeavesTheOthersAlone() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        let front = session.viewport(for: .front, size: viewSize)
        session.pan(.top, size: viewSize, byPoints: CGSize(width: 50, height: 50))
        #expect(session.viewport(for: .front, size: viewSize) == front)
    }

    @Test func fittingTheSelectionFramesItAndForgetsThePanning() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        let cluster = SelectionPolygon(rectFrom: simd_float2(0, 0), to: simd_float2(3, 3))
        #expect(session.select(polygon: cluster, mode: .replace))
        session.pan(.top, size: viewSize, byPoints: CGSize(width: 500, height: 500))

        #expect(session.fitViews(to: .selection))

        let top = session.viewport(for: .top, size: viewSize)
        #expect(abs(top.centre.x - 1.5) < 1e-5)
        #expect(abs(top.centre.y - 1.25) < 1e-5)
        #expect(session.viewStates.isEmpty)
        // The 3D view is pointed at the same region: X and Y from the top
        // view, Z from the front.
        let focus = try #require(session.sceneFocus)
        #expect(abs(focus.centre.x - 1.5) < 1e-5)
        #expect(abs(focus.centre.y - 1.25) < 1e-5)
        #expect(abs(focus.centre.z - 0.75) < 1e-5)
    }

    @Test func fittingNothingLeavesTheViewsWhereTheyWere() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        session.pan(.top, size: viewSize, byPoints: CGSize(width: 40, height: 0))
        let placed = session.viewport(for: .top, size: viewSize)
        let focus = session.sceneFocus

        #expect(!session.fitViews(to: .selection))

        #expect(session.viewport(for: .top, size: viewSize) == placed)
        #expect(session.sceneFocus == focus)
    }

    @Test func fittingTheForegroundLeavesTheGroundOut() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(session.fitViews(to: .foreground))
        // The ground return at (40, -30) would put the centre near (20, -14).
        let top = session.viewport(for: .top, size: viewSize)
        #expect(abs(top.centre.x - 1.5) < 1e-5)
    }

    @Test func askingToFitTwiceMovesTheCameraTwice() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        let first = try #require(session.sceneFocus)
        session.fitViews(to: .sample)
        let second = try #require(session.sceneFocus)
        #expect(second.revision == first.revision + 1)
    }

    @Test func aSlabSetByHandIsKeptAcrossSamples() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        session.setSlab(DepthSlab(minDepth: -1.2, maxDepth: -0.6))
        session.stepForward()
        #expect(session.slab == DepthSlab(minDepth: -1.2, maxDepth: -0.6))

        // Reset, and it follows each sample's own depth again.
        session.unpinSlab()
        let ownDepth = session.slab
        session.stepBackward()
        #expect(session.slab != ownDepth)
        #expect(!session.slabIsPinned)
    }

    @Test func scaleBarIsARoundLengthNoMoreThanAQuarterOfTheView() {
        #expect(AnnotationScaleBar.length(forHalfHeight: 10) == 5)
        #expect(AnnotationScaleBar.length(forHalfHeight: 3) == 1)
        #expect(AnnotationScaleBar.length(forHalfHeight: 50) == 20)
        #expect(abs(AnnotationScaleBar.length(forHalfHeight: 0.5) - 0.2) < 1e-6)
        #expect(AnnotationScaleBar.label(5) == "5 m")
        #expect(AnnotationScaleBar.label(0.2) == "20 cm")
    }
}

// MARK: - Frame sync

struct FrameSyncTests {
    private func sample(_ id: Int, _ ts: Int64) -> AnnotationSample {
        AnnotationSample(
            sampleID: id, sourceOrdinal: id, sourceFrameID: UInt64(id), timestampNs: ts,
            sensorID: "s", pointCount: 0, byteOffset: 0)
    }

    private var samples: [AnnotationSample] {
        [sample(0, 1_000), sample(1, 2_000), sample(2, 3_000)]
    }

    @Test func nearestSampleOnEitherSideOfTheTimestamp() throws {
        #expect(try #require(AnnotationFrameSync.nearestSample(to: 2_000, in: samples)).index == 1)
        #expect(try #require(AnnotationFrameSync.nearestSample(to: 2_400, in: samples)).index == 1)
        #expect(try #require(AnnotationFrameSync.nearestSample(to: 2_600, in: samples)).index == 2)
        let between = try #require(AnnotationFrameSync.nearestSample(to: 2_400, in: samples))
        #expect(between.deltaNs == 400)
    }

    @Test func nearestSampleBeyondEitherEnd() throws {
        #expect(try #require(AnnotationFrameSync.nearestSample(to: 0, in: samples)).index == 0)
        #expect(try #require(AnnotationFrameSync.nearestSample(to: 9_000, in: samples)).index == 2)
        #expect(AnnotationFrameSync.nearestSample(to: 0, in: []) == nil)
    }

    @Test func aReplayOfAnotherRecordingDoesNotCoverThePack() {
        let hour: Int64 = 3_600_000_000_000
        #expect(AnnotationFrameSync.replayCovers(samples, logStartNs: 0, logEndNs: 5_000))
        #expect(!AnnotationFrameSync.replayCovers(samples, logStartNs: hour, logEndNs: 2 * hour))
        // No timeline at all: the main view has not loaded a replay.
        #expect(!AnnotationFrameSync.replayCovers(samples, logStartNs: 0, logEndNs: 0))
    }
}

@MainActor
struct SessionFrameSyncTests {
    // Fixture samples are at 1.0 s and 1.1 s.
    private func main(at timestampNs: Int64, seekable: Bool = true) -> MainViewPlayback {
        MainViewPlayback(
            timestampNs: timestampNs, logStartNs: 900_000_000, logEndNs: 5_000_000_000,
            seekable: seekable)
    }

    @Test func followsTheMainViewToItsFrame() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(session.follow(main(at: 1_100_000_000)))
        #expect(session.currentSample?.sampleID == 1)
        #expect(session.syncStatus == .inSync)
        // Already there: nothing to do, and still in sync.
        #expect(!session.follow(main(at: 1_100_000_000)))
        #expect(session.syncStatus == .inSync)
    }

    @Test func followingIsNotAnOperatorStep() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        // A followed step must not ask the main view to seek: during playback
        // that would drag it back to a frame it has already left.
        session.follow(main(at: 1_100_000_000))
        #expect(session.operatorNavigationRevision == 0)
        session.stepBackward()
        #expect(session.operatorNavigationRevision == 1)
    }

    @Test func aFrameThisPackDoesNotHoldLeavesTheWindowWhereItIs() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(!session.follow(main(at: 3_000_000_000)))
        #expect(session.currentSample?.sampleID == 0)
        #expect(session.syncStatus == .outsidePack)
    }

    @Test func saysWhyItCannotFollow() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        session.follow(main(at: 1_100_000_000, seekable: false))
        #expect(session.syncStatus == .mainNotSeekable)

        let elsewhere = MainViewPlayback(
            timestampNs: 9_000_000_000_000, logStartNs: 9_000_000_000_000,
            logEndNs: 9_100_000_000_000, seekable: true)
        session.follow(elsewhere)
        #expect(session.syncStatus == .differentRecording)
        #expect(session.currentSample?.sampleID == 0)
    }

    @Test func unsavedChangesHoldTheSampleAgainstTheMainView() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        _ = session.createObject(objectClass: "car")
        #expect(session.select(polygon: everythingLasso, mode: .replace))

        #expect(!session.follow(main(at: 1_100_000_000)))
        #expect(session.currentSample?.sampleID == 0)
        #expect(session.syncStatus == .heldByUnsavedChanges)
        #expect(session.selectionCount == 4)
    }

    @Test func leadsTheMainViewOnlyWhenItIsSomewhereElse() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }

        #expect(session.seekTarget(for: main(at: 2_000_000_000)) == 1_000_000_000)
        // Within a frame of the sample already: seeking would only stutter.
        #expect(session.seekTarget(for: main(at: 1_000_000_000)) == nil)
        #expect(session.seekTarget(for: main(at: 1_020_000_000)) == nil)
        #expect(session.seekTarget(for: main(at: 2_000_000_000, seekable: false)) == nil)
    }

    @Test func switchedOffItNeitherFollowsNorLeads() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        session.syncWithMainView = false

        #expect(!session.follow(main(at: 1_100_000_000)))
        #expect(session.seekTarget(for: main(at: 2_000_000_000)) == nil)
        #expect(session.currentSample?.sampleID == 0)
        #expect(session.syncStatus == .off)
    }

    @Test func aKeyPressThatArrivesByTwoRoutesIsAppliedOnce() throws {
        let (session, dir) = try makeSession()
        defer { try? FileManager.default.removeItem(at: dir) }
        session.tool = .sphere
        let start = session.sphereRadius

        session.adjustBrushSize(steps: 1, eventTimestamp: 12.5)
        let once = session.sphereRadius
        session.adjustBrushSize(steps: 1, eventTimestamp: 12.5)
        #expect(once > start)
        #expect(session.sphereRadius == once)

        session.adjustBrushSize(steps: 1, eventTimestamp: 12.75)
        #expect(session.sphereRadius > once)
    }
}

// MARK: - 3D scene

struct AnnotationSceneTests {
    private let points = PackPoints(
        x: [0, 1, 2, 3, .nan, 5], y: [0, 1, 2, 3, 0, 5], z: [0, 1, 2, 3, 0, 5],
        intensity: [10, 20, 30, 40, 50, 60], classification: [0, 1, 2, 1, 1, 1])
    private let classes: [UInt8] = [0, 1, 2, 1, 1, 1]

    private func shade(_ paletteIndex: Int) -> UInt8 { AnnotationPalette.shaderClass(paletteIndex) }

    @Test func eachEditStateIsDrawnInItsOwnColour() throws {
        var marks = AnnotationScene.Marks()
        marks.activeClass = "car"
        marks.saved = [1, 3]
        marks.selected = [1, 2]  // 1 saved and kept, 2 added, 3 saved and taken out
        marks.candidates = [2, 5]
        marks.others = [(objectClass: "pedestrian", indices: [0])]

        let cloud = try #require(
            AnnotationScene.frame(
                points: points, classes: classes, visibility: nil, marks: marks, sample: nil
            ).pointCloud)

        // The NaN is dropped. A return both selected and a candidate is drawn
        // as selected: it is already in the membership.
        #expect(cloud.x == [0, 1, 2, 3, 5])
        #expect(
            cloud.classification == [
                shade(AnnotationPalette.paletteIndex(forClass: "pedestrian")),
                shade(AnnotationPalette.paletteIndex(forClass: "car")),
                shade(AnnotationPalette.unsavedIndex), shade(AnnotationPalette.removedIndex),
                shade(AnnotationPalette.candidateIndex),
            ])
        #expect(cloud.intensity == [10, 20, 30, 40, 60])
    }

    @Test func unmarkedReturnsKeepTheirOwnClass() throws {
        let cloud = try #require(
            AnnotationScene.frame(
                points: points, classes: classes, visibility: nil, marks: AnnotationScene.Marks(),
                sample: nil
            ).pointCloud)
        #expect(cloud.classification == [0, 1, 2, 1, 1])
    }

    @Test func aHiddenClassIsNotDrawnUnlessItIsInAMask() throws {
        let noForeground = PointVisibility(background: true, foreground: false, ground: true)
        var marks = AnnotationScene.Marks()
        marks.selected = [3]
        let cloud = try #require(
            AnnotationScene.frame(
                points: points, classes: classes, visibility: noForeground, marks: marks,
                sample: nil
            ).pointCloud)

        #expect(cloud.x == [0, 2, 3])
        #expect(cloud.classification.last == shade(AnnotationPalette.unsavedIndex))
    }

    @Test func theFrameCarriesTheSampleItWasCutFrom() {
        let sample = AnnotationSample(
            sampleID: 4, sourceOrdinal: 9, sourceFrameID: 321, timestampNs: 77, sensorID: "s",
            pointCount: 6, byteOffset: 0)
        let bundle = AnnotationScene.frame(
            points: points, classes: classes, visibility: nil, marks: AnnotationScene.Marks(),
            sample: sample)
        #expect(bundle.frameID == 321)
        #expect(bundle.timestampNanos == 77)
    }

    @Test func lookingAtARegionTakesAllOfItIn() {
        var camera = Camera()
        camera.projection = .orthographic(halfHeight: 3)
        let focus = AnnotationSceneFocus(centre: simd_float3(5, 6, 1), radius: 10, revision: 1)
        camera.lookAt(focus)

        #expect(camera.target == simd_float3(5, 6, 1))
        #expect(camera.projection == .perspective)
        // Far enough back that a sphere of the radius fits the vertical field
        // of view, and above the ground looking down.
        let distance = simd_distance(camera.position, camera.target)
        #expect(distance * tan(camera.fov * .pi / 360) >= 10)
        #expect(camera.position.z > camera.target.z)
    }
}

// MARK: - Wiring

/// Source checks, as in AnnotationWiringTests: a view nobody mounts compiles
/// and passes every unit test.
struct AnnotationViewportWiringTests {
    private func source(_ relative: String, file: String = #filePath) throws -> String {
        var dir = URL(fileURLWithPath: file).deletingLastPathComponent()
        for _ in 0..<6 {
            let candidate = dir.appendingPathComponent("VelocityVisualiser/\(relative)")
            if FileManager.default.fileExists(atPath: candidate.path) {
                return try String(contentsOf: candidate, encoding: .utf8)
            }
            dir = dir.deletingLastPathComponent()
        }
        Issue.record("could not locate \(relative)")
        return ""
    }

    @Test func theWindowMountsTheThirdViewAndTheLinkToTheMainView() throws {
        let window = try source("UI/AnnotationWindow.swift")
        for symbol in [
            "AnnotationSceneView(", "MainViewLink(", "focusedSceneValue(\\.annotationSession",
            "session.viewport(for:",
        ] { #expect(window.contains(symbol), "AnnotationWindow does not use \(symbol)") }
    }

    @Test func theOverlayTakesItsInputFromTheLayerThatCanZoom() throws {
        let pane = try source("UI/AnnotationPane.swift")
        #expect(pane.contains("ViewportInputLayer("))
        // SwiftUI's drag gesture has no scroll wheel and no right button; if
        // it came back the views would stop zooming and panning.
        #expect(!pane.contains("DragGesture("))
    }

    @Test func theAppGivesTheWindowTheMainViewAndRoutesItsKeys() throws {
        let app = try source("App/VelocityVisualiserApp.swift")
        #expect(app.contains("AnnotationWindow().environmentObject(appState)"))
        #expect(app.contains("@FocusedValue(\\.annotationSession)"))
    }
}
