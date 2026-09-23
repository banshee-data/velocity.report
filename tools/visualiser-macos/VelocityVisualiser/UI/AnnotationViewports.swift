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

/// The keys an orthographic view acts on when it has the focus.
enum ViewportKey: Equatable {
    /// An arrow, as a direction in the view; `coarse` when shift is held.
    ///
    /// What an arrow means depends on what is on screen, and the view decides
    /// that rather than the menu, so the order is in one readable place: with
    /// a carried proposal it moves the proposal, and without one the
    /// left and right arrows step frames while up and down move the ground
    /// plane.
    case nudge(right: Int, up: Int, coarse: Bool)
    /// A digit, turning that voxel of the column stack on or off.
    case voxel(Int)
    case accept
    case cancel

    /// What this key does, which depends only on whether a proposal is being
    /// carried. Kept apart from the view so the order can be read and tested
    /// rather than inferred from a switch inside a body.
    enum Meaning: Equatable {
        case nudgeCarried(right: Int, up: Int, coarse: Bool)
        case acceptCarried
        case dismissCarried
        case stepFrame(forward: Bool)
        case moveGround(steps: Int, coarse: Bool)
        case toggleVoxel(Int)
        /// Not this view's key; let it go on up the chain.
        case pass
    }

    func meaning(carrying: Bool) -> Meaning {
        if carrying {
            switch self {
            case .nudge(let right, let up, let coarse):
                return .nudgeCarried(right: right, up: up, coarse: coarse)
            case .accept: return .acceptCarried
            case .cancel: return .dismissCarried
            case .voxel(let k): return .toggleVoxel(k)
            }
        }
        switch self {
        case .nudge(let right, 0, _): return .stepFrame(forward: right > 0)
        case .nudge(0, let up, let coarse): return .moveGround(steps: up, coarse: coarse)
        case .voxel(let k): return .toggleVoxel(k)
        case .nudge, .accept, .cancel: return .pass
        }
    }
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
    /// A click that was not a drag, in a view that takes no strokes.
    var onClick: (CGPoint) -> Void = { _ in }
    /// The cursor's position in the view, or nil once it has left.
    var onHover: (CGPoint?) -> Void = { _ in }
    /// Shift-scroll, in whole steps: moves the brush along the depth axis.
    var onDepthStep: (Int) -> Void = { _ in }
    /// A key the view may have a use for. Returns true when it did.
    var onKey: (ViewportKey) -> Bool = { _ in false }

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
    /// Where the left button went down, to tell a click from a pan.
    private var pressedAt: CGPoint?

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
        pressedAt = strokes ? nil : point
        if !strokes || event.modifierFlags.contains(.control) {
            drag = .pan(last: point)
            return
        }
        drag = .stroke(start: point)
        layer_?.onStrokeChanged(ViewportStroke(startLocation: point, location: point))
    }

    override func mouseDragged(with event: NSEvent) { dragged(to: location(of: event)) }

    override func mouseUp(with event: NSEvent) {
        let point = location(of: event)
        if case .stroke(let start) = drag {
            layer_?.onStrokeEnded(ViewportStroke(startLocation: start, location: point))
        } else if let pressedAt, hypot(point.x - pressedAt.x, point.y - pressedAt.y) < 4 {
            layer_?.onClick(point)
        }
        pressedAt = nil
        drag = nil
    }

    override func rightMouseDown(with event: NSEvent) { drag = .pan(last: location(of: event)) }
    override func rightMouseDragged(with event: NSEvent) { dragged(to: location(of: event)) }
    override func rightMouseUp(with event: NSEvent) { drag = nil }
    override func otherMouseDown(with event: NSEvent) { drag = .pan(last: location(of: event)) }
    override func otherMouseDragged(with event: NSEvent) { dragged(to: location(of: event)) }
    override func otherMouseUp(with event: NSEvent) { drag = nil }

    private func dragged(to point: CGPoint) {
        // The brush preview follows a pan or a stroke too, or it would sit
        // where the drag began until the button came up.
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
        if event.modifierFlags.contains(.shift) {
            // macOS turns a shifted wheel into a horizontal scroll, so the
            // movement may be on either axis.
            let raw =
                abs(event.scrollingDeltaY) >= abs(event.scrollingDeltaX)
                ? event.scrollingDeltaY : event.scrollingDeltaX
            depthScroll += event.hasPreciseScrollingDeltas ? raw / 10 : raw
            let steps = Int(depthScroll.rounded(.towardZero))
            if steps != 0 {
                depthScroll -= CGFloat(steps)
                // Scrolling up brings the brush towards the viewer.
                layer_?.onDepthStep(-steps)
            }
            return
        }
        // The main view's scaling: a trackpad reports many small deltas, a
        // wheel a few large ones.
        let delta = event.hasPreciseScrollingDeltas ? event.scrollingDeltaY / 10 : event.deltaY
        layer_?.onZoom(ViewportInputView.zoomFactor(forScroll: delta), location(of: event))
    }

    /// Scroll not yet amounting to a whole depth step.
    private var depthScroll: CGFloat = 0

    // MARK: Hover

    private var hoverArea: NSTrackingArea?

    override func updateTrackingAreas() {
        super.updateTrackingAreas()
        if let hoverArea { removeTrackingArea(hoverArea) }
        let area = NSTrackingArea(
            rect: .zero,
            options: [.mouseMoved, .mouseEnteredAndExited, .activeInKeyWindow, .inVisibleRect],
            owner: self, userInfo: nil)
        addTrackingArea(area)
        hoverArea = area
    }

    override func mouseMoved(with event: NSEvent) { layer_?.onHover(location(of: event)) }
    override func mouseExited(with event: NSEvent) { layer_?.onHover(nil) }

    // MARK: Keys

    override func keyDown(with event: NSEvent) {
        if let key = ViewportInputView.viewportKey(for: event), layer_?.onKey(key) == true {
            return
        }
        super.keyDown(with: event)
    }

    static func viewportKey(for event: NSEvent) -> ViewportKey? {
        let coarse = event.modifierFlags.contains(.shift)
        switch event.keyCode {
        case 123: return .nudge(right: -1, up: 0, coarse: coarse)
        case 124: return .nudge(right: 1, up: 0, coarse: coarse)
        case 125: return .nudge(right: 0, up: -1, coarse: coarse)
        case 126: return .nudge(right: 0, up: 1, coarse: coarse)
        case 36, 76: return .accept
        case 53: return .cancel
        default:
            // Digits 0 to 7, in the order they sit on the keyboard rather than
            // the order of their key codes, which is not monotonic.
            guard let k = [29, 18, 19, 20, 21, 23, 22, 26].firstIndex(of: Int(event.keyCode)) else {
                return nil
            }
            return .voxel(k)
        }
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
    /// The marks under the brush change on every mouse move, and the session
    /// no longer republishes for them: see BrushHover.swift.
    @ObservedObject private var hover: BrushHover
    let model: AnnotationSceneModel

    init(session: AnnotationSession, model: AnnotationSceneModel) {
        self.session = session
        self._hover = ObservedObject(wrappedValue: session.hover)
        self.model = model
    }

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
        // Every publish of the session arrives here and most change nothing
        // this view draws. It used to ask for a redraw regardless: seventy
        // thousand points drawn again, on the main thread, for a slider moving
        // in a sidebar, and in a release build the largest single cost of a
        // step.
        var changed = false
        if coordinator.sceneRevision != session.sceneRevision {
            changed = true
            coordinator.sceneRevision = session.sceneRevision
            var marks = AnnotationScene.Marks()
            marks.others = session.otherMasks.map { ($0.objectClass, $0.indices) }
            marks.activeClass = session.activeObject?.objectClass
            marks.saved = session.savedSelection
            marks.selected = session.history.current
            marks.candidates = Set(session.pendingCandidates?.indices ?? []).union(hover.indices)
                .union(session.carriedIndices).union(session.proposalIndices)
            // The renderer keeps a background until it is given another, so
            // it is sent once per snapshot and switched on and off after that.
            let backdrop = session.currentBackground.map { snapshot in
                AnnotationScene.Backdrop(
                    snapshot: snapshot, points: session.backgroundPoints,
                    changed: session.backgroundChanged,
                    upload: coordinator.backgroundID != snapshot.backgroundID)
            }
            coordinator.backgroundID = session.currentBackground?.backgroundID
            renderer.showBackground =
                session.currentBackground != nil && session.visibility.background
            renderer.updateFrame(
                AnnotationScene.frame(
                    points: session.currentPoints, classes: session.currentClasses,
                    visibility: session.effectiveVisibility, labels: session.effectiveLabelSets,
                    marks: marks, sample: session.currentSample, backdrop: backdrop))
        }
        if let focus = session.sceneFocus, focus.revision != coordinator.focusRevision {
            changed = true
            coordinator.focusRevision = focus.revision
            renderer.camera.lookAt(focus)
        }
        if changed { view.needsDisplay = true }
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    final class Coordinator {
        var renderer: MetalRenderer?
        var sceneRevision: Int?
        var focusRevision: Int?
        var backgroundID: Int?
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

    @State private var throttle = FollowThrottle()
    @State private var followScheduled = false
    @State private var lastLoopSeekAt: TimeInterval = -.infinity

    private var playback: MainViewPlayback {
        let seekable = appState.displayPlaybackMode == .replaySeekable
        return MainViewPlayback(
            timestampNs: appState.currentTimestamp, logStartNs: appState.logStartTimestamp,
            logEndNs: appState.logEndTimestamp, seekable: seekable,
            playing: seekable && !appState.isPaused && !appState.replayFinished,
            finished: seekable && appState.replayFinished)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Toggle("Follow the main view", isOn: $session.syncWithMainView).font(.caption).help(
                "Stepping here seeks the main view to the same frame, and moving the main view "
                    + "steps here. The main view has to be replaying the recording this pack "
                    + "was cut from.")
            if session.syncWithMainView {
                Toggle("Loop this pack's frames", isOn: $session.loopPackFrames).font(.caption)
                    .help(
                        "While the main view plays, send it back to this pack's first frame when "
                            + "it runs past the last. Space plays and pauses the main view.")
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
                of: appState.replayFinished
            ) { _, _ in followMainView() }.onChange(of: session.operatorNavigationRevision) {
                _, _ in leadMainView()
            }
    }

    /// Main view to this window.
    private func followMainView() {
        // A seek in flight still delivers frames from before it. Following
        // those would step this window back to where it has just left.
        guard !appState.isSeekingInProgress else { return }
        let main = playback
        let now = ProcessInfo.processInfo.systemUptime

        // Keep a playing main view inside the pack. Not more than once a
        // second: the seek takes a moment, and the frames that arrive before
        // it lands are still past the end.
        if let first = session.loopTarget(for: main) {
            if now - lastLoopSeekAt > 1 {
                lastLoopSeekAt = now
                appState.seekToTimestamp(first)
            }
            return
        }

        let wait = throttle.wait(now: now, playing: main.playing)
        guard wait <= 0 else {
            // Skipped, not forgotten: the frame the main view is on when the
            // wait is over is followed then, so a main view that pauses in
            // the meantime is still caught up with.
            guard !followScheduled else { return }
            followScheduled = true
            DispatchQueue.main.asyncAfter(deadline: .now() + wait) {
                followScheduled = false
                followMainView()
            }
            return
        }
        throttle.followed(at: now)
        guard session.follow(main) else { return }
        // Two turns of the main queue on: past the redraw this step caused,
        // which is most of what it costs.
        DispatchQueue.main.async {
            DispatchQueue.main.async {
                throttle.measured(cost: ProcessInfo.processInfo.systemUptime - now)
            }
        }
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

// MARK: - Sector ring

/// How much of the frame is labelled, in sixteen sectors round the sensor.
///
/// Drawn the way the top view is, +X to the right and sectors anticlockwise
/// from it, so that a sector with work left in it points at where that work is
/// on screen. Each wedge fills from the centre: agreed first, then in
/// question, and what is left of it is what nobody has labelled.
struct SectorRing: View {
    let completeness: FrameCompleteness
    let onSelect: (Int) -> Void

    var body: some View {
        GeometryReader { geometry in
            let size = geometry.size
            Canvas { context, size in
                let centre = CGPoint(x: size.width / 2, y: size.height / 2)
                let outer = min(size.width, size.height) / 2 - 1
                let inner = outer * 0.22
                for (index, tally) in completeness.sectors.enumerated() {
                    let bearings = FrameCompleteness.bearings(ofSector: index)
                    func wedge(from r0: CGFloat, to r1: CGFloat) -> Path {
                        var path = Path()
                        // Canvas angles run clockwise with y down; bearings
                        // run anticlockwise with y up. Negating turns one
                        // into the other.
                        let start = Angle(radians: -Double(bearings.lowerBound))
                        let end = Angle(radians: -Double(bearings.upperBound))
                        path.addArc(
                            center: centre, radius: r1, startAngle: start, endAngle: end,
                            clockwise: true)
                        path.addArc(
                            center: centre, radius: r0, startAngle: end, endAngle: start,
                            clockwise: false)
                        path.closeSubpath()
                        return path
                    }
                    let span = outer - inner
                    let agreedEdge = inner + span * CGFloat(tally.fractionAgreed)
                    let questionEdge = agreedEdge + span * CGFloat(tally.fractionInQuestion)
                    // Empty sectors are drawn faint rather than not at all, so
                    // that the ring stays a ring and the gaps mean "nothing
                    // here" and not "not drawn".
                    context.fill(
                        wedge(from: inner, to: outer),
                        with: .color(.white.opacity(tally.total > 0 ? 0.16 : 0.04)))
                    if tally.agreed > 0 {
                        context.fill(
                            wedge(from: inner, to: agreedEdge),
                            with: .color(AnnotationFrameStrip.agreedColour))
                    }
                    if tally.inQuestion > 0 {
                        context.fill(
                            wedge(from: agreedEdge, to: questionEdge),
                            with: .color(AnnotationFrameStrip.inQuestionColour))
                    }
                    context.stroke(
                        wedge(from: inner, to: outer), with: .color(.black.opacity(0.6)),
                        lineWidth: 0.5)
                }
                // The sensor, and which way is +X.
                context.fill(
                    Path(ellipseIn: CGRect(x: centre.x - 2, y: centre.y - 2, width: 4, height: 4)),
                    with: .color(.white))
                context.draw(
                    Text("+X").font(.system(size: 8)).foregroundColor(.secondary),
                    at: CGPoint(x: size.width - 8, y: centre.y - 7))
            }.contentShape(Rectangle()).gesture(
                DragGesture(minimumDistance: 0).onEnded { value in
                    if let sector = SectorRing.sector(at: value.location, in: size) {
                        onSelect(sector)
                    }
                })
        }
    }

    /// The sector under a point in the ring's own coordinates, or nil at the
    /// very centre, where every sector meets.
    static func sector(at location: CGPoint, in size: CGSize) -> Int? {
        let dx = Float(location.x - size.width / 2)
        // Screen y grows downward; the ring is drawn with +Y up.
        let dy = Float(size.height / 2 - location.y)
        guard dx * dx + dy * dy > 4 else { return nil }
        return FrameCompleteness.sector(x: dx, y: dy)
    }
}
