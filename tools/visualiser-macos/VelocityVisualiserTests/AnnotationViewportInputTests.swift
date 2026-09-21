//
//  AnnotationViewportInputTests.swift
//  VelocityVisualiserTests
//
//  Mounts a real annotation viewport in a window and sends it mouse and scroll
//  events through the window, the way AppKit delivers them.
//
//  Everything the viewport does with a gesture is tested elsewhere without a
//  view. What those tests cannot show is that a gesture arrives: that the
//  drawing layers stacked over the input layer let a click through to it, and
//  that a drag on screen becomes a selection of the returns under it. A
//  viewport that looks right and ignores the mouse passes every other test.
//

import AppKit
import SwiftUI
import Testing
import simd

@testable import VelocityVisualiser

@MainActor
struct AnnotationViewportInputTests {
    private struct Mounted {
        let session: AnnotationSession
        let window: NSWindow
        let input: ViewportInputView
        let packDirectory: URL

        func cleanUp() {
            window.close()
            try? FileManager.default.removeItem(at: packDirectory)
        }
    }

    private func mount(editable: Bool = true) throws -> Mounted {
        let dir = try PackFixture.write()
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 600, height: 400), styleMask: [.titled],
            backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        window.contentView = NSHostingView(
            rootView: AnnotationViewportView(session: session, standard: .top, editable: editable))
        window.orderFront(nil)
        window.layoutIfNeeded()
        // SwiftUI mounts a representable's NSView on a later turn of the loop.
        var input: ViewportInputView?
        for _ in 0..<20 where input == nil {
            RunLoop.current.run(until: Date().addingTimeInterval(0.05))
            input = window.contentView.flatMap(Self.inputView(in:))
        }
        return Mounted(
            session: session, window: window,
            input: try #require(input, "the viewport mounted no input layer"), packDirectory: dir)
    }

    private static func inputView(in view: NSView) -> ViewportInputView? {
        if let found = view as? ViewportInputView { return found }
        for child in view.subviews { if let found = inputView(in: child) { return found } }
        return nil
    }

    /// A world position in the top view, as a point in window coordinates.
    private func windowPoint(_ world: simd_float2, in mounted: Mounted) -> CGPoint {
        let viewport = mounted.session.viewport(for: .top, size: mounted.input.bounds.size)
        return mounted.input.convert(viewport.screenPoint(from: world), to: nil)
    }

    /// Delivers a mouse event to whichever view the window's hit test finds
    /// under it, which is how AppKit chooses.
    ///
    /// Not through `NSWindow.sendEvent`. The test host is not the active app,
    /// so its windows cannot become key, and AppKit spends a left click in a
    /// window that is not key on activating it and delivers it to nobody.
    private func send(_ type: NSEvent.EventType, at point: CGPoint, to mounted: Mounted) throws {
        let event = try #require(
            NSEvent.mouseEvent(
                with: type, location: point, modifierFlags: [],
                timestamp: ProcessInfo.processInfo.systemUptime,
                windowNumber: mounted.window.windowNumber, context: nil, eventNumber: 0,
                clickCount: 1, pressure: 1))
        let content = try #require(mounted.window.contentView)
        let target = try #require(
            content.hitTest(content.convert(point, from: nil)), "nothing under \(point)")
        switch type {
        case .leftMouseDown: target.mouseDown(with: event)
        case .leftMouseDragged: target.mouseDragged(with: event)
        case .leftMouseUp: target.mouseUp(with: event)
        case .rightMouseDown: target.rightMouseDown(with: event)
        case .rightMouseDragged: target.rightMouseDragged(with: event)
        case .rightMouseUp: target.rightMouseUp(with: event)
        default: Issue.record("unhandled event type \(type)")
        }
    }

    @Test func aClickInTheViewportReachesTheInputLayer() throws {
        let mounted = try mount()
        defer { mounted.cleanUp() }

        // The point canvas, the selection overlay, the label and the scale bar
        // are all drawn over the input layer. None of them may take the click.
        let centre = CGPoint(x: mounted.input.bounds.midX, y: mounted.input.bounds.midY)
        let inWindow = mounted.input.convert(centre, to: nil)
        let content = try #require(mounted.window.contentView)
        let hit = content.hitTest(content.convert(inWindow, from: nil))
        #expect(hit === mounted.input)
    }

    @Test func draggingALassoRoundTheClusterSelectsIt() throws {
        let mounted = try mount()
        defer { mounted.cleanUp() }

        // Round the three foreground returns near (1.5, 1.25), and nowhere
        // near the ground return at (40, -30).
        let corners = [
            simd_float2(0, 0), simd_float2(3, 0), simd_float2(3, 3), simd_float2(0, 3),
        ].map { windowPoint($0, in: mounted) }

        try send(.leftMouseDown, at: corners[0], to: mounted)
        for corner in corners.dropFirst() { try send(.leftMouseDragged, at: corner, to: mounted) }
        #expect(mounted.session.pendingCandidates?.count == 3)
        try send(.leftMouseUp, at: corners[3], to: mounted)

        #expect(mounted.session.canonicalSelection == [0, 1, 2])
        #expect(mounted.session.navigationGuard() == nil)
    }

    @Test func aClickWithTheLassoSelectsNothingAndLeavesNoStrokeOpen() throws {
        let mounted = try mount()
        defer { mounted.cleanUp() }

        let point = windowPoint(simd_float2(1.5, 1.25), in: mounted)
        try send(.leftMouseDown, at: point, to: mounted)
        try send(.leftMouseUp, at: point, to: mounted)

        #expect(mounted.session.selectionCount == 0)
        #expect(mounted.session.navigationGuard() == nil)
    }

    @Test func aSphereClickTakesTheReturnsRoundTheOneUnderTheCursor() throws {
        let mounted = try mount()
        defer { mounted.cleanUp() }
        mounted.session.tool = .sphere
        mounted.session.sphereRadius = 1

        let point = windowPoint(simd_float2(1.5, 1.25), in: mounted)
        try send(.leftMouseDown, at: point, to: mounted)
        try send(.leftMouseUp, at: point, to: mounted)

        // Brushes add by default, and all three returns are within a metre of
        // the middle one.
        #expect(mounted.session.canonicalSelection == [0, 1, 2])
    }

    @Test func draggingWithTheRightButtonPansAndSelectsNothing() throws {
        let mounted = try mount()
        defer { mounted.cleanUp() }
        let before = mounted.session.viewport(for: .top, size: mounted.input.bounds.size)

        let start = windowPoint(before.centre, in: mounted)
        try send(.rightMouseDown, at: start, to: mounted)
        try send(.rightMouseDragged, at: CGPoint(x: start.x + 60, y: start.y), to: mounted)
        try send(.rightMouseUp, at: CGPoint(x: start.x + 60, y: start.y), to: mounted)

        let after = mounted.session.viewport(for: .top, size: mounted.input.bounds.size)
        // The content followed the drag to the right, so the centre moved left.
        #expect(after.centre.x < before.centre.x)
        #expect(after.centre.y == before.centre.y)
        #expect(after.halfHeight == before.halfHeight)
        #expect(mounted.session.selectionCount == 0)
        #expect(mounted.session.pendingCandidates == nil)
    }

    @Test func theConfirmingViewPansWithTheLeftButtonAndNeverSelects() throws {
        let mounted = try mount(editable: false)
        defer { mounted.cleanUp() }
        let before = mounted.session.viewport(for: .top, size: mounted.input.bounds.size)

        let corners = [
            simd_float2(0, 0), simd_float2(3, 0), simd_float2(3, 3), simd_float2(0, 3),
        ].map { windowPoint($0, in: mounted) }
        try send(.leftMouseDown, at: corners[0], to: mounted)
        for corner in corners.dropFirst() { try send(.leftMouseDragged, at: corner, to: mounted) }
        try send(.leftMouseUp, at: corners[3], to: mounted)

        #expect(mounted.session.selectionCount == 0)
        #expect(mounted.session.viewport(for: .top, size: mounted.input.bounds.size) != before)
    }

    @Test func aClickInAnotherViewMakesItTheEditingView() throws {
        // The top view is the editing view; this mounts the front view.
        let dir = try PackFixture.write()
        defer { try? FileManager.default.removeItem(at: dir) }
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 400, height: 300), styleMask: [.titled],
            backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        defer { window.close() }
        window.contentView = NSHostingView(
            rootView: AnnotationViewportView(session: session, standard: .front, editable: false))
        window.orderFront(nil)
        var input: ViewportInputView?
        for _ in 0..<20 where input == nil {
            RunLoop.current.run(until: Date().addingTimeInterval(0.05))
            input = window.contentView.flatMap(Self.inputView(in:))
        }
        let view = try #require(input)
        let mounted = Mounted(session: session, window: window, input: view, packDirectory: dir)
        session.setSlab(DepthSlab(minDepth: -1, maxDepth: 0))

        let point = view.convert(CGPoint(x: 200, y: 150), to: nil)
        try send(.leftMouseDown, at: point, to: mounted)
        try send(.leftMouseUp, at: point, to: mounted)

        #expect(session.viewStandard == .front)
        #expect(session.secondViewStandard != .front)
        // The slab was along the top view's depth axis, which is not this one's.
        #expect(!session.slabIsPinned)
        #expect(session.selectionCount == 0)
    }

    @Test func scrollingZoomsTheView() throws {
        let mounted = try mount()
        defer { mounted.cleanUp() }
        let before = mounted.session.viewport(for: .top, size: mounted.input.bounds.size)

        let scroll = try #require(
            CGEvent(
                scrollWheelEvent2Source: nil, units: .line, wheelCount: 1, wheel1: 3, wheel2: 0,
                wheel3: 0))
        mounted.input.scrollWheel(with: try #require(NSEvent(cgEvent: scroll)))

        let after = mounted.session.viewport(for: .top, size: mounted.input.bounds.size)
        #expect(after.halfHeight < before.halfHeight)
    }
}

/// Mounts the whole workspace, 3D view included.
@MainActor
struct AnnotationWorkspaceMountTests {
    private static func find<T: NSView>(_ type: T.Type, in view: NSView) -> T? {
        if let found = view as? T { return found }
        for child in view.subviews { if let found = find(type, in: child) { return found } }
        return nil
    }

    @Test func the3DViewDrawsTheSampleAndFollowsTheSession() throws {
        let dir = try PackFixture.write()
        defer { try? FileManager.default.removeItem(at: dir) }
        let controller = AnnotationController()
        controller.openPack(at: dir)
        let session = try #require(controller.session)

        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 1200, height: 800), styleMask: [.titled],
            backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        defer { window.close() }
        window.contentView = NSHostingView(
            rootView: AnnotationWorkspace(
                session: session, controller: controller, showGenerateSheet: .constant(false)
            ).environmentObject(AppState()))
        window.orderFront(nil)

        func settle() { RunLoop.current.run(until: Date().addingTimeInterval(0.1)) }
        var metalView: InteractiveMetalView?
        for _ in 0..<20 where metalView?.renderer == nil {
            settle()
            metalView = window.contentView.flatMap { Self.find(InteractiveMetalView.self, in: $0) }
        }
        let renderer = try #require(metalView?.renderer, "the workspace mounted no 3D view")
        func drawn() -> Int { renderer.compositeRenderer?.getStats().foreground ?? -1 }

        // Sample 0 has four returns, and both editing views are mounted too.
        #expect(drawn() == 4)
        let content = try #require(window.contentView)
        #expect(Self.find(ViewportInputView.self, in: content) != nil)
        // The camera was pointed at the sample rather than left on the main
        // view's default, which looks at the sensor.
        let focus = try #require(session.sceneFocus)
        #expect(renderer.camera.target == focus.centre)

        // Hiding a class takes it out of the 3D view as well.
        session.visibility.ground = false
        settle()
        #expect(drawn() == 3)

        // And it steps with the session. Sample 1 has two returns, one of
        // them above the height band's ceiling, which is ground and hidden.
        session.stepForward()
        settle()
        #expect(drawn() == 1)
    }
}
