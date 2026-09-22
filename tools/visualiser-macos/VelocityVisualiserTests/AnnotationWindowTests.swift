//
//  AnnotationWindowTests.swift
//  VelocityVisualiserTests
//
//  The annotation toolset was fully built and fully tested and still could
//  not be reached: nothing in the app presented it. Every unit test passed
//  throughout. So alongside the framing maths, these tests check the wiring
//  itself — a menu item, a window scene, and a pack picker — because that is
//  the part whose absence the other 600 tests could not detect.
//

import CoreGraphics
import Foundation
import Testing
import simd

@testable import VelocityVisualiser

struct AnnotationFramingTests {
    /// A 4 m by 2 m box of points centred on (10, 5) in the top view.
    private func boxPoints() -> PackPoints {
        PackPoints(
            x: [8, 12, 8, 12], y: [4, 4, 6, 6], z: [0, 0, 1, 1],
            intensity: [0, 0, 0, 0], classification: [0, 0, 0, 0])
    }

    @Test func extentIsTheCentreAndHalfSpanInTheViewPlane() throws {
        let extent = try #require(annotationExtent(of: boxPoints(), basis: OrthoViewBasis(.top)))
        #expect(abs(extent.centre.x - 10) < 1e-5)
        #expect(abs(extent.centre.y - 5) < 1e-5)
        #expect(abs(extent.halfWidth - 2) < 1e-5)
        #expect(abs(extent.halfHeight - 1) < 1e-5)
    }

    @Test func extentFollowsTheBasisRatherThanTheWorldAxes() throws {
        // In the front view the vertical axis is world Z, so the same points
        // have a different height. A framing that ignored the basis would
        // scale the confirming view wrongly and hide contamination.
        let extent = try #require(annotationExtent(of: boxPoints(), basis: OrthoViewBasis(.front)))
        #expect(abs(extent.halfHeight - 0.5) < 1e-5, "front view height is world Z")
        #expect(abs(extent.centre.y - 0.5) < 1e-5)
    }

    @Test func anEmptySampleHasNoExtent() {
        // nil rather than a zero extent: "nothing to show" and "everything at
        // the origin" need different handling, and a zero extent would frame
        // an empty view at an arbitrary scale.
        #expect(annotationExtent(of: PackPoints(), basis: OrthoViewBasis(.top)) == nil)
    }

    @Test func framingFitsTheWiderAxisNotJustTheTaller() {
        // A street seen from above is wide and flat. Framing on height alone
        // would run it off both sides of the view.
        let wide = AnnotationExtent(centre: .zero, halfHeight: 1, halfWidth: 40)
        let half = annotationFramingHalfHeight(
            extent: wide, size: CGSize(width: 400, height: 200), margin: 1, floor: 0.001)
        // Aspect is 2, so 40 m of half-width needs 20 m of half-height.
        #expect(abs(half - 20) < 1e-4)
    }

    @Test func framingUsesHeightWhenHeightIsTheBindingAxis() {
        let tall = AnnotationExtent(centre: .zero, halfHeight: 30, halfWidth: 1)
        let half = annotationFramingHalfHeight(
            extent: tall, size: CGSize(width: 400, height: 200), margin: 1, floor: 0.001)
        #expect(abs(half - 30) < 1e-4)
    }

    @Test func framingLeavesAMargin() {
        let extent = AnnotationExtent(centre: .zero, halfHeight: 10, halfWidth: 1)
        let half = annotationFramingHalfHeight(
            extent: extent, size: CGSize(width: 200, height: 200), margin: 1.15, floor: 0.001)
        #expect(half > 10, "points on the exact edge of the view cannot be lassoed")
        #expect(abs(half - 11.5) < 1e-4)
    }

    @Test func framingNeverCollapsesToZero() {
        // A single point, or a perfectly flat one-plane sample, has zero
        // extent on an axis. Zero half-height divides by zero in the screen
        // mapping, so the floor is load-bearing rather than cosmetic.
        let degenerate = AnnotationExtent(centre: .zero, halfHeight: 0, halfWidth: 0)
        let half = annotationFramingHalfHeight(
            extent: degenerate, size: CGSize(width: 400, height: 200))
        #expect(half > 0)

        // And the resulting viewport must round-trip a point rather than
        // returning .zero from its guard.
        let viewport = OrthoViewport(
            halfHeight: half, size: CGSize(width: 400, height: 200), centre: .zero)
        let screen = viewport.screenPoint(from: .zero)
        #expect(abs(screen.x - 200) < 1e-6)
        #expect(abs(screen.y - 100) < 1e-6)
    }

    @Test func framingSurvivesAZeroSizedView() {
        // SwiftUI lays a GeometryReader out at zero before its first pass.
        let extent = AnnotationExtent(centre: .zero, halfHeight: 5, halfWidth: 5)
        let half = annotationFramingHalfHeight(extent: extent, size: .zero)
        #expect(half.isFinite && half > 0)
    }
}

@MainActor struct AnnotationControllerTests {
    @Test func startsWithNoSession() {
        let controller = AnnotationController()
        #expect(controller.session == nil)
        #expect(controller.packName == nil)
        #expect(controller.lastError == nil)
    }

    /// The happy path, against bytes Go's writer actually produced.
    ///
    /// PackFixture is pinned on both sides — TestSwiftFixturePackBytesAreStable
    /// fails on the Go side if the writer changes — so this exercises the
    /// route an operator takes: choose a directory, get a working session.
    @Test func openingARealPackProducesAUsableSession() throws {
        let dir = try PackFixture.write()
        defer { try? FileManager.default.removeItem(at: dir) }

        let controller = AnnotationController()
        controller.openPack(at: dir)

        // The error, when there is one, is the useful part of a failure here.
        #expect(controller.lastError == nil, "open failed: \(controller.lastError ?? "")")
        let session = try #require(controller.session)
        #expect(controller.packName == dir.lastPathComponent)
        #expect(session.samples.count == 2)
        // The session must land on a sample with its points decoded, or the
        // window opens on an empty view and the operator has nothing to lasso.
        #expect(session.currentSample != nil)
        #expect(session.currentPoints.count > 0)

        // And that sample must be framable: the window's viewport comes from
        // this, so a nil extent here is a blank editing surface.
        let extent = try #require(
            annotationExtent(of: session.currentPoints, basis: OrthoViewBasis(.top)))
        #expect(annotationFramingHalfHeight(extent: extent, size: CGSize(width: 800, height: 600)) > 0)
    }

    @Test func openingASecondPackReplacesTheFirst() throws {
        // "Open Another Pack…" must not leave the previous pack's indices in
        // play against the new pack's points.
        let first = try PackFixture.write()
        let second = try PackFixture.write()
        defer {
            try? FileManager.default.removeItem(at: first)
            try? FileManager.default.removeItem(at: second)
        }

        let controller = AnnotationController()
        controller.openPack(at: first)
        let firstSession = try #require(controller.session)
        controller.openPack(at: second)
        let secondSession = try #require(controller.session)

        #expect(firstSession !== secondSession)
        #expect(controller.packName == second.lastPathComponent)
    }

    @Test func aFailedOpenDiscardsTheSessionItHad() throws {
        // Otherwise the window keeps showing the old pack's points while the
        // title and the error describe a different one.
        let dir = try PackFixture.write()
        defer { try? FileManager.default.removeItem(at: dir) }

        let controller = AnnotationController()
        controller.openPack(at: dir)
        #expect(controller.session != nil)

        controller.openPack(at: URL(fileURLWithPath: "/nonexistent/pack-dir"))
        #expect(controller.session == nil)
        #expect(controller.lastError != nil)
    }

    @Test func openingAMissingPackReportsRatherThanCrashes() {
        let controller = AnnotationController()
        controller.openPack(at: URL(fileURLWithPath: "/nonexistent/pack-dir"))
        #expect(controller.session == nil)
        let error = controller.lastError ?? ""
        #expect(!error.isEmpty, "a failed open must say so")
        #expect(error.contains("pack-dir"), "the message must name the directory tried")
    }

    @Test func openingACorruptPackLeavesNoSession() throws {
        // Fails closed. Indices recorded against different bytes are not
        // approximately valid, so a half-open session is worse than none.
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(
            "annotation-corrupt-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: dir) }
        try Data("{ not json".utf8).write(to: dir.appendingPathComponent("manifest.json"))
        try Data().write(to: dir.appendingPathComponent("samples.json"))
        try Data().write(to: dir.appendingPathComponent("points.bin"))

        let controller = AnnotationController()
        controller.openPack(at: dir)
        #expect(controller.session == nil)
        #expect(controller.lastError != nil)
    }

    @Test func closeClearsTheErrorAsWellAsTheSession() {
        // A stale error left behind the empty state reads as if the next open
        // had failed too.
        let controller = AnnotationController()
        controller.openPack(at: URL(fileURLWithPath: "/nonexistent/pack-dir"))
        #expect(controller.lastError != nil)
        controller.close()
        #expect(controller.lastError == nil)
        #expect(controller.session == nil)
    }
}

/// The wiring. Source checks, in the manner of `InertModifierTests`, because
/// an unreferenced view compiles, passes every unit test, and is invisible.
struct AnnotationWiringTests {
    private static func sourcePath(_ relative: String, file: String = #filePath) -> String {
        var dir = URL(fileURLWithPath: file).deletingLastPathComponent()
        for _ in 0..<6 {
            let candidate = dir.appendingPathComponent("VelocityVisualiser/\(relative)")
            if FileManager.default.fileExists(atPath: candidate.path) { return candidate.path }
            dir = dir.deletingLastPathComponent()
        }
        return ""
    }

    private func source(_ relative: String) throws -> String {
        let path = Self.sourcePath(relative)
        #expect(!path.isEmpty, "could not locate \(relative)")
        return try String(contentsOfFile: path, encoding: .utf8)
    }

    @Test func theAppDeclaresAnAnnotationWindowAndMenuItem() throws {
        let app = try source("App/VelocityVisualiserApp.swift")
        #expect(app.contains("id: \"annotation\""), "no annotation window scene")
        #expect(app.contains("AnnotationWindow()"), "the scene does not host AnnotationWindow")
        #expect(app.contains("CommandMenu(\"Annotation\")"), "no way to open the window")
        #expect(
            app.contains("openWindow(id: \"annotation\")"), "the menu item does not open the window"
        )
    }

    @Test func theWindowReachesThePaneTheOverlayAndThePicker() throws {
        let window = try source("UI/AnnotationWindow.swift")
        // Each of these was unreferenced before the wiring landed.
        for symbol in ["AnnotationPane(session:", "LassoOverlay(", "NSOpenPanel(", "AnnotationPack.open("]
        {
            #expect(window.contains(symbol), "AnnotationWindow does not use \(symbol)")
        }
    }

    @Test func thePickerChoosesADirectoryNotAFile() throws {
        // A pack is three files bound by digests; picking one of them could
        // only ever be the start of an error message.
        let window = try source("UI/AnnotationWindow.swift")
        #expect(window.contains("canChooseDirectories = true"))
        #expect(window.contains("canChooseFiles = false"))
    }

    @Test func onlyTheEditingViewTakesStrokes() throws {
        // Review is gated on a check from another view. The top view and all
        // four elevations are on screen, and if each took strokes there would
        // be five editing surfaces and no view left to check in.
        let window = try source("UI/AnnotationWindow.swift")
        #expect(
            window.contains("ForEach(OrthoViewBasis.Standard.elevations"),
            "the four elevations are not all mounted")
        #expect(
            window.contains("standard: .top"), "the top view is not mounted on its own")
        #expect(
            window.contains("editable: standard == session.viewStandard"),
            "an elevation other than the editing view accepts strokes")
        #expect(
            window.contains("editable: session.viewStandard == .top"),
            "the top view accepts strokes when it is not the editing view")
    }

    /// Every standard has to be reachable, or a view exists that nothing can
    /// mount and no selection can ever be checked in.
    @Test func theTopViewAndTheElevationsCoverEveryStandard() {
        let mounted = Set([OrthoViewBasis.Standard.top] + OrthoViewBasis.Standard.elevations)
        #expect(mounted == Set(OrthoViewBasis.Standard.allCases))
        #expect(OrthoViewBasis.Standard.elevations.count == 4)
    }

    // "Generate from Run…", "Open Pack…" and "Close" in AnnotationWorkspace
    // each replace or discard the open session (a new one, or none at all)
    // with no check of their own — AnnotationController.openPack() and
    // close() both do this unconditionally. Before this guard, any of the
    // three lost an unsaved lasso selection silently the moment it was
    // clicked, the same way stepping or switching objects used to before
    // AnnotationPane's handleStep existed.
    @Test func openAnotherAndCloseAreAllGuarded() throws {
        let window = try source("UI/AnnotationWindow.swift")
        for guarded in [
            "guardedNavigate { showGenerateSheet = true }",
            "guardedNavigate { controller.choosePack() }",
            "guardedNavigate { controller.close() }",
        ] {
            #expect(window.contains(guarded), "not routed through guardedNavigate: \(guarded)")
        }
    }

    @Test func theGuardChecksNavigationGuardBeforeActing() throws {
        let window = try source("UI/AnnotationWindow.swift")
        #expect(window.contains("session.navigationGuard() != nil"))
    }

    @Test func discardingCallsReloadBeforeThePendingAction() throws {
        // Order matters: the action (opening a different pack, or closing)
        // must run against a session that has already discarded its unsaved
        // membership, not before — reload() is what makes navigationGuard()
        // return nil again afterward.
        let window = try source("UI/AnnotationWindow.swift")
        let reloadIndex = window.range(of: "session.reload()")
        let actionIndex = window.range(of: "action?()")
        let reload = try #require(reloadIndex, "Discard and Continue does not call session.reload()")
        let action = try #require(actionIndex, "the pending action is never invoked")
        #expect(reload.lowerBound < action.lowerBound, "reload() must run before the pending action")
    }

    @Test func keepEditingClearsThePendingActionRatherThanRunningIt() throws {
        // A cancel that still fires the deferred action would make "Keep
        // Editing" indistinguishable from "Discard and Continue".
        let window = try source("UI/AnnotationWindow.swift")
        #expect(window.contains("Button(\"Keep Editing\", role: .cancel) { pendingAction = nil }"))
    }
}
