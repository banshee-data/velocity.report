// AnnotationViewports.swift
// The parts of the annotation window that move: the input layer that pans and
// zooms an orthographic view, the 3D view, and the link to the main view.
//
// The orthographic views are where selection happens, and they are drawn and
// hit-tested through one projection (see AnnotationWindow.swift). The 3D view
// is for looking: it is the main view's renderer pointed at the pack sample,
// so orbiting it is the same as orbiting the main view, and it never decides
// what a gesture selects.

import AppKit
import Combine
import MetalKit
import SwiftUI
import simd

// MARK: - Input layer

/// One position of a selection stroke, in the view's points (origin top-left).
struct ViewportStroke: Equatable {
    var startLocation: CGPoint
    var location: CGPoint
}

/// Mouse and trackpad input for an orthographic view.
///
/// An AppKit view rather than SwiftUI gestures, because SwiftUI on macOS has
/// no scroll-wheel or secondary-button gesture, and a view an operator cannot
/// zoom is one they cannot select a car in. The left button draws with the
/// active tool; the right or middle button, or control with the left, pans;
/// scrolling and pinching zoom about the cursor, as they do in the main view.
struct ViewportInputLayer: NSViewRepresentable {
    /// False for the confirming view, where the left button pans instead.
    var strokesEnabled: Bool
    var onStrokeChanged: (ViewportStroke) -> Void
    var onStrokeEnded: (ViewportStroke) -> Void
    /// A drag of this many points, y growing downward.
    var onPan: (CGSize) -> Void
    /// A scale factor (below one zooms in) about a point in the view.
    var onZoom: (Float, CGPoint) -> Void

    func makeNSView(context: Context) -> ViewportInputView {
        let view = ViewportInputView()
        view.layer_ = self
        return view
    }

    func updateNSView(_ view: ViewportInputView, context: Context) { view.layer_ = self }
}

final class ViewportInputView: NSView {
    fileprivate var layer_: ViewportInputLayer?

    private enum Drag {
        case stroke(start: CGPoint)
        case pan(last: CGPoint)
    }
    private var drag: Drag?

    /// Top-left origin, to match the SwiftUI layers drawn over this view.
    override var isFlipped: Bool { true }
    override var acceptsFirstResponder: Bool { true }

    private func location(of event: NSEvent) -> CGPoint {
        convert(event.locationInWindow, from: nil)
    }

    override func mouseDown(with event: NSEvent) {
        // Take focus from any text field, so that the bracket keys size the
        // brush rather than typing into the operator's name.
        window?.makeFirstResponder(self)
        let point = location(of: event)
        let strokes = layer_?.strokesEnabled ?? false
        if !strokes || event.modifierFlags.contains(.control) {
            drag = .pan(last: point)
            return
        }
        drag = .stroke(start: point)
        layer_?.onStrokeChanged(ViewportStroke(startLocation: point, location: point))
    }

    override func mouseDragged(with event: NSEvent) { dragged(to: location(of: event)) }

    override func mouseUp(with event: NSEvent) {
        if case .stroke(let start) = drag {
            layer_?.onStrokeEnded(
                ViewportStroke(startLocation: start, location: location(of: event)))
        }
        drag = nil
    }

    override func rightMouseDown(with event: NSEvent) { drag = .pan(last: location(of: event)) }
    override func rightMouseDragged(with event: NSEvent) { dragged(to: location(of: event)) }
    override func rightMouseUp(with event: NSEvent) { drag = nil }
    override func otherMouseDown(with event: NSEvent) { drag = .pan(last: location(of: event)) }
    override func otherMouseDragged(with event: NSEvent) { dragged(to: location(of: event)) }
    override func otherMouseUp(with event: NSEvent) { drag = nil }

    private func dragged(to point: CGPoint) {
        switch drag {
        case .stroke(let start):
            layer_?.onStrokeChanged(ViewportStroke(startLocation: start, location: point))
        case .pan(let last):
            layer_?.onPan(CGSize(width: point.x - last.x, height: point.y - last.y))
            drag = .pan(last: point)
        case nil: break
        }
    }

    override func scrollWheel(with event: NSEvent) {
        // The main view's scaling: a trackpad reports many small deltas, a
        // wheel a few large ones.
        let delta = event.hasPreciseScrollingDeltas ? event.scrollingDeltaY / 10 : event.deltaY
        layer_?.onZoom(ViewportInputView.zoomFactor(forScroll: delta), location(of: event))
    }

    override func magnify(with event: NSEvent) {
        layer_?.onZoom(
            ViewportInputView.zoomFactor(forScroll: event.magnification * 10), location(of: event))
    }

    /// Scrolling up zooms in, as in the main view. Clamped so that one large
    /// wheel delta cannot take the view from a street to a wing mirror.
    static func zoomFactor(forScroll delta: CGFloat) -> Float {
        min(max(1 - Float(delta) * 0.1, 0.5), 2)
    }
}

// MARK: - 3D view

/// Holds the 3D view's renderer, so that controls outside the view can move
/// its camera.
@MainActor final class AnnotationSceneModel: ObservableObject {
    fileprivate(set) weak var renderer: MetalRenderer?

    /// Looks from where another view is looking: the main view's camera,
    /// brought across so the two show the same thing from the same place.
    func adopt(_ camera: Camera) {
        guard let renderer else { return }
        renderer.camera.position = camera.position
        renderer.camera.target = camera.target
        renderer.camera.up = camera.up
        renderer.view?.needsDisplay = true
    }
}

/// The 3D view: the main view's renderer, showing the current sample.
struct AnnotationSceneView: NSViewRepresentable {
    @ObservedObject var session: AnnotationSession
    let model: AnnotationSceneModel

    func makeNSView(context: Context) -> MTKView {
        let metalView = InteractiveMetalView()
        // Drawn on demand: nothing here changes unless the operator does
        // something.
        metalView.isPaused = true
        metalView.enableSetNeedsDisplay = true
        if let renderer = MetalRenderer(metalView: metalView) {
            // A pack carries points and nothing else, so there is nothing for
            // the other layers to draw.
            renderer.showBoxes = false
            renderer.showClusters = false
            renderer.showTrails = false
            renderer.showVelocity = false
            context.coordinator.renderer = renderer
            metalView.renderer = renderer
            model.renderer = renderer
        }
        return metalView
    }

    func updateNSView(_ view: MTKView, context: Context) {
        let coordinator = context.coordinator
        guard let renderer = coordinator.renderer else { return }

        // Every publish of the session arrives here, and most change nothing
        // this view draws. Rebuilding a frame of tens of thousands of points
        // for each would make a slider drag stutter.
        if coordinator.sceneRevision != session.sceneRevision {
            coordinator.sceneRevision = session.sceneRevision
            renderer.updateFrame(
                AnnotationScene.frame(
                    points: session.currentPoints, visibility: session.effectiveVisibility,
                    selected: session.history.current,
                    candidates: Set(session.pendingCandidates?.indices ?? []),
                    sample: session.currentSample))
        }
        if let focus = session.sceneFocus, focus.revision != coordinator.focusRevision {
            coordinator.focusRevision = focus.revision
            renderer.camera.lookAt(focus)
        }
        view.needsDisplay = true
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    final class Coordinator {
        var renderer: MetalRenderer?
        var sceneRevision: Int?
        var focusRevision: Int?
    }
}

// MARK: - Main view link

/// Keeps the annotation window and the main view on one frame.
///
/// Its own view, so that it alone depends on AppState. AppState publishes at
/// the stream's frame rate; a workspace that observed it would re-evaluate,
/// and redraw its point canvases, ten times a second for as long as the main
/// view was playing.
struct MainViewLink: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject var appState: AppState
    let scene: AnnotationSceneModel

    private var playback: MainViewPlayback {
        MainViewPlayback(
            timestampNs: appState.currentTimestamp, logStartNs: appState.logStartTimestamp,
            logEndNs: appState.logEndTimestamp,
            seekable: appState.displayPlaybackMode == .replaySeekable)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Toggle("Follow the main view", isOn: $session.syncWithMainView).font(.caption).help(
                "Stepping here seeks the main view to the same frame, and moving the main view "
                    + "steps here. The main view has to be replaying the recording this pack "
                    + "was cut from.")
            if session.syncWithMainView {
                Text(session.syncStatus.label).font(.caption2).foregroundStyle(
                    session.syncStatus == .inSync ? Color.green : Color.secondary
                ).fixedSize(horizontal: false, vertical: true)
            }
            Button("Match the main view's camera") {
                if let camera = appState.mainCamera { scene.adopt(camera) }
            }.controlSize(.small).help("Look at the 3D view from where the main view is looking")
        }.frame(maxWidth: .infinity, alignment: .leading).padding(.horizontal, 12).padding(
            .vertical, 8
        ).onAppear { align() }.onChange(of: session.syncWithMainView) { _, on in if on { align() } }
            .onChange(of: appState.currentTimestamp) { _, _ in followMainView() }.onChange(
                of: session.operatorNavigationRevision
            ) { _, _ in leadMainView() }
    }

    /// Main view to this window.
    private func followMainView() {
        // A seek in flight still delivers frames from before it. Following
        // those would step this window back to where it has just left.
        guard !appState.isSeekingInProgress else { return }
        session.follow(playback)
    }

    /// This window to the main view.
    private func leadMainView() {
        if let target = session.seekTarget(for: playback) { appState.seekToTimestamp(target) }
    }

    /// On opening a pack, or turning sync on: go to the main view's frame if
    /// this pack holds it, and otherwise bring the main view to this one.
    private func align() {
        guard session.syncWithMainView else { return }
        session.follow(playback)
        if session.syncStatus == .outsidePack { leadMainView() }
    }
}

// MARK: - Menu routing

/// The annotation session of the focused window, for the app's menu commands.
///
/// The Playback menu binds the bracket keys and the step keys with no
/// modifier, app-wide. With the annotation window in front they mean that
/// window's brush and that window's samples, and this is how the menu finds
/// out that it is in front.
struct AnnotationSessionFocusKey: FocusedValueKey { typealias Value = AnnotationSession }

extension FocusedValues {
    var annotationSession: AnnotationSession? {
        get { self[AnnotationSessionFocusKey.self] }
        set { self[AnnotationSessionFocusKey.self] = newValue }
    }
}
