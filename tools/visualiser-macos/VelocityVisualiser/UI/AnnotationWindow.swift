// AnnotationWindow.swift
// The entry point to the point-cloud annotation toolset.
//
// The pieces this composes — pack reading, selection, the sidecar protocol,
// the controls — all landed together and were tested, but nothing presented
// them, so the toolset could not be reached from the running app. This is the
// wiring: a menu item, a pack picker, and a window that puts the lasso overlay
// over a drawn point cloud.
//
// The points are drawn with SwiftUI Canvas rather than through the Metal
// renderer, and that is a correctness decision rather than an expedient one.
// Selection maps screen points to metres through OrthoViewport; if the drawing
// went through a second, independent projection in the renderer, any
// disagreement between them would show as points being selected that the
// operator did not draw around — silently, and only for some camera states.
// One projection, used for both, cannot drift from itself.
//
// The 3D view beside them does go through the Metal renderer, and that is
// consistent with the above: it takes no selection gesture. It is there to be
// moved around in, and nothing reads a point index back out of it.

import AppKit
import Combine
import SwiftUI
import simd

private let annotationLogger = DevLogger(category: "Annotation")

// MARK: - Framing

/// The view-plane bounds of a set of points under one basis.
struct AnnotationExtent: Equatable {
    var centre: simd_float2
    /// Half-height of the content, in metres, before any margin.
    var halfHeight: Float
    var halfWidth: Float
}

/// Measures a sample's extent in the view plane, for framing the camera.
///
/// Returns nil for an empty sample rather than a zero extent, so the caller
/// distinguishes "nothing to show" from "everything at the origin" — the
/// second would otherwise frame an empty view at an arbitrary scale.
func annotationExtent(of points: PackPoints, basis: OrthoViewBasis) -> AnnotationExtent? {
    guard points.count > 0 else { return nil }
    var minX = Float.greatestFiniteMagnitude
    var minY = Float.greatestFiniteMagnitude
    var maxX = -Float.greatestFiniteMagnitude
    var maxY = -Float.greatestFiniteMagnitude
    var seen = false
    for index in 0..<points.count {
        guard let p = points.point(at: index) else { continue }
        let v = basis.project(p)
        minX = min(minX, v.x)
        maxX = max(maxX, v.x)
        minY = min(minY, v.y)
        maxY = max(maxY, v.y)
        seen = true
    }
    guard seen else { return nil }
    return AnnotationExtent(
        centre: simd_float2((minX + maxX) / 2, (minY + maxY) / 2),
        halfHeight: max((maxY - minY) / 2, 0), halfWidth: max((maxX - minX) / 2, 0))
}

/// Half-height that frames `extent` in a view of `size`, with a margin.
///
/// Both axes are considered: framing on height alone would cut off a wide,
/// flat sample — which is what a top-down view of a street is — at the sides.
/// The floor stops a single-point or perfectly flat sample from collapsing the
/// view to zero scale, which would divide by zero in the screen mapping.
func annotationFramingHalfHeight(
    extent: AnnotationExtent, size: CGSize, margin: Float = 1.15, floor: Float = 1.0
) -> Float {
    let aspect = size.height > 0 ? Float(size.width / size.height) : 1
    let neededForWidth = aspect > 0 ? extent.halfWidth / aspect : extent.halfWidth
    return max(max(extent.halfHeight, neededForWidth) * margin, floor)
}

// MARK: - Controller

/// Owns the annotation session and the act of opening a pack.
///
/// Separate from AppState: annotation reads packs off disk and never touches
/// the frame stream, so coupling it to the live view's state would only give
/// each a way to break the other. The one link between the two is
/// `MainViewLink`, which keeps them on the same frame and knows nothing of
/// either's points.
@MainActor final class AnnotationController: ObservableObject {
    @Published private(set) var session: AnnotationSession?
    @Published var lastError: String?

    private let folderAccess: PackFolderAccess
    /// Asks the operator for the folder to grant, given where the pack is.
    /// Injected so the grant flow can be tested without an open panel.
    private let askForFolder: (URL) -> URL?
    /// Held for as long as the session it serves: SidecarStore writes into the
    /// pack on every save, so access has to outlive the act of opening.
    private var grant: PackFolderGrant?

    init(folderAccess: PackFolderAccess = PackFolderAccess(), askForFolder: ((URL) -> URL?)? = nil)
    {
        self.folderAccess = folderAccess
        self.askForFolder = askForFolder ?? AnnotationController.runFolderPanel
    }

    /// Name of the open pack's directory, for the window's title bar.
    var packName: String? { session?.pack.directory.lastPathComponent }

    /// Opens a pack directory the operator chose, replacing any open session.
    ///
    /// No folder grant is involved: choosing the directory in an open panel is
    /// itself the grant, for as long as the app runs.
    ///
    /// Errors are surfaced rather than logged and swallowed: a digest
    /// mismatch means the indices an operator is about to record would be
    /// recorded against different bytes, and that has to be visible.
    func openPack(at directory: URL) { open(directory, holding: nil) }

    /// Opens a pack the server has just written.
    ///
    /// The operator never chose this path — it arrived in an HTTP response —
    /// so the sandbox refuses it until a folder containing it has been
    /// granted. A remembered folder is used silently; otherwise the operator
    /// is asked once, and the answer is kept for later launches.
    func openGeneratedPack(at directory: URL) {
        if let remembered = folderAccess.beginAccess(for: directory) {
            open(directory, holding: remembered)
            return
        }

        guard let folder = askForFolder(directory.deletingLastPathComponent()) else {
            lastError =
                "The pack was written to \(directory.path), but the app has not been given "
                + "access to that folder. Generate again to grant it, or open the pack with "
                + "Open Annotation Pack."
            return
        }
        guard PackFolderAccess.folder(folder, contains: directory) else {
            lastError =
                "\(folder.lastPathComponent) does not contain the new pack. It was written to "
                + "\(directory.path); grant that folder or one above it."
            return
        }

        // Failing to remember is not failing to open: the panel has already
        // granted the folder for this launch. The operator is simply asked
        // again next time.
        do { try folderAccess.remember(folder) } catch {
            annotationLogger.error("Could not remember pack folder \(folder.path): \(error)")
        }
        open(directory, holding: folderAccess.beginAccess(for: directory))
    }

    private func open(_ directory: URL, holding newGrant: PackFolderGrant?) {
        grant?.end()
        grant = newGrant
        do {
            let pack = try AnnotationPack.open(directory: directory)
            session = try AnnotationSession(pack: pack)
            lastError = nil
            annotationLogger.info("Opened annotation pack \(directory.lastPathComponent)")
        } catch {
            session = nil
            grant?.end()
            grant = nil
            lastError = "Could not open \(directory.lastPathComponent): \(error)"
            annotationLogger.error("Failed to open annotation pack: \(error)")
        }
    }

    /// The production `askForFolder`: a directory panel opened on the folder
    /// the pack was written to, so granting it is a single click.
    private static func runFolderPanel(suggested: URL) -> URL? {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.canCreateDirectories = false
        panel.directoryURL = suggested
        panel.prompt = "Grant Access"
        panel.message =
            "The new pack is in this folder. Grant access so it can be opened and its "
            + "annotations saved. You will not be asked again for packs written here."
        guard panel.runModal() == .OK else { return nil }
        return panel.url
    }

    /// Prompts for a pack directory.
    ///
    /// A directory rather than a file: a pack is manifest.json, samples.json
    /// and points.bin together, and the digests bind them, so letting an
    /// operator pick one file of the three could only ever be the start of an
    /// error message.
    func choosePack() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.prompt = "Open Pack"
        panel.message = "Choose an annotation pack directory (containing manifest.json)."
        guard panel.runModal() == .OK, let url = panel.url else { return }
        openPack(at: url)
    }

    func close() {
        session = nil
        lastError = nil
        grant?.end()
        grant = nil
    }
}

// MARK: - Window

/// The annotation window: empty until a pack is opened, then the workspace.
struct AnnotationWindow: View {
    @StateObject private var controller = AnnotationController()
    @State private var showGenerateSheet = false

    var body: some View {
        Group {
            if let session = controller.session {
                AnnotationWorkspace(
                    session: session, controller: controller, showGenerateSheet: $showGenerateSheet)
            } else {
                emptyState
            }
        }.frame(minWidth: 1040, minHeight: 640).navigationTitle(
            controller.packName.map { "Annotation — \($0)" } ?? "Annotation"
        ).sheet(isPresented: $showGenerateSheet) {
            GenerateAnnotationPackSheet { packDir in controller.openGeneratedPack(at: packDir) }
        }
    }

    private var emptyState: some View {
        VStack(spacing: 14) {
            Image(systemName: "lasso.and.sparkles").font(.system(size: 44)).foregroundStyle(
                .secondary)
            Text("No annotation pack open").font(.headline)
            Text("Generate one from a recorded run, or open a pack already on disk.").font(.caption)
                .foregroundStyle(.secondary)
            HStack {
                // Generate is the primary path: it needs nothing an operator
                // does not already have from having recorded a run. Opening
                // by directory stays available for a pack made elsewhere, or
                // by the CLI export this wraps.
                Button("Generate from Run…") { showGenerateSheet = true }.keyboardShortcut(
                    "g", modifiers: [.command, .shift])
                Button("Open Annotation Pack…") { controller.choosePack() }.keyboardShortcut(
                    "o", modifiers: .command)
            }
            if let error = controller.lastError {
                Text(error).font(.caption).foregroundStyle(.red).multilineTextAlignment(.center)
                    .frame(maxWidth: 520).fixedSize(horizontal: false, vertical: true)
            }
        }.padding(40).frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

/// The editing surface: the working view, the confirming view, and the pane.
struct AnnotationWorkspace: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject var controller: AnnotationController
    @Binding var showGenerateSheet: Bool

    /// Set when a guarded action is deferred behind the discard-confirmation
    /// alert, so "Discard and Continue" knows what to do once the operator
    /// has actually chosen to lose the unsaved membership.
    @State private var pendingAction: (() -> Void)?

    @EnvironmentObject private var appState: AppState
    @StateObject private var scene = AnnotationSceneModel()

    private var showDiscardPrompt: Binding<Bool> {
        Binding(get: { pendingAction != nil }, set: { if !$0 { pendingAction = nil } })
    }

    // The bracket keys size the active brush, as they do in every paint
    // program an operator has used.
    //
    // Hidden buttons rather than a key handler on the viewport: a shortcut
    // with no modifier is delivered to the window wherever the focus is, so
    // it works without first clicking into the point view, and it gives way
    // to a text field, so typing a bracket into the operator's name types a
    // bracket.
    private var brushSizeKeys: some View {
        Group {
            Button("Smaller brush") {
                session.adjustBrushSize(steps: -1, eventTimestamp: NSApp.currentEvent?.timestamp)
            }.keyboardShortcut("[", modifiers: [])
            Button("Larger brush") {
                session.adjustBrushSize(steps: 1, eventTimestamp: NSApp.currentEvent?.timestamp)
            }.keyboardShortcut("]", modifiers: [])
        }.opacity(0).frame(width: 0, height: 0).accessibilityHidden(true)
    }

    var body: some View {
        HStack(spacing: 0) {
            HSplitView {
                // The editing view, and the only one that takes a stroke.
                AnnotationViewportView(
                    session: session, standard: session.viewStandard, editable: true
                ).frame(minWidth: 360, minHeight: 300)

                VSplitView {
                    // The 3D view: the main view's renderer on this sample,
                    // moved the way the main view is moved. It is for looking,
                    // not for selecting.
                    AnnotationSceneView(session: session, model: scene).overlay(
                        alignment: .topLeading
                    ) {
                        Text("3D · drag to orbit, shift-drag to pan, scroll to zoom").font(
                            .caption2
                        ).padding(4).foregroundStyle(.secondary).allowsHitTesting(false)
                    }.frame(minHeight: 200)

                    // The confirming view. Review is gated on it, so it is
                    // always on screen rather than behind a toggle: a check an
                    // operator has to go and find is a check that gets skipped.
                    AnnotationViewportView(
                        session: session, standard: session.secondViewStandard, editable: false
                    ).frame(minHeight: 160)
                }.frame(minWidth: 280)
            }.frame(maxWidth: .infinity)

            Divider()

            VStack(spacing: 0) {
                MainViewLink(session: session, appState: appState, scene: scene).frame(width: 300)
                Divider()
                AnnotationPane(session: session)
                Divider()
                HStack {
                    Menu("Open Another…") {
                        // Both leave the current pack behind — one for a
                        // different run's pack, one for a directory already
                        // on disk — and both must go through the same
                        // unsaved-membership check AnnotationPane already
                        // applies to stepping and switching objects.
                        // Bypassing it here would lose a lasso selection
                        // silently the moment either is clicked.
                        Button("Generate from Run…") {
                            guardedNavigate { showGenerateSheet = true }
                        }
                        Button("Open Pack…") { guardedNavigate { controller.choosePack() } }
                    }.menuStyle(.borderlessButton).fixedSize()
                    Spacer()
                    Button("Close") { guardedNavigate { controller.close() } }
                }.padding(8)
            }
        }.background { brushSizeKeys }.focusedSceneValue(\.annotationSession, session).alert(
            "Unsaved membership", isPresented: showDiscardPrompt
        ) {
            Button("Keep Editing", role: .cancel) { pendingAction = nil }
            Button("Discard and Continue", role: .destructive) {
                let action = pendingAction
                pendingAction = nil
                session.reload()
                action?()
            }
        } message: {
            Text(
                "This sample has unsaved changes. Save it, or discard them, before opening another pack."
            )
        }
    }

    /// Runs `action` immediately if nothing would be lost, or defers it
    /// behind the discard-confirmation alert if the session has an unsaved
    /// stroke or membership. Mirrors AnnotationPane's `handleStep`, which
    /// guards Previous/Next and object-row switching the same way — this is
    /// the same check applied to the window's other three ways to leave the
    /// current pack.
    private func guardedNavigate(_ action: @escaping () -> Void) {
        guard session.navigationGuard() != nil else {
            action()
            return
        }
        pendingAction = action
    }
}

/// One orthographic view of the current sample, with the selection overlay.
struct AnnotationViewportView: View {
    @ObservedObject var session: AnnotationSession
    let standard: OrthoViewBasis.Standard
    let editable: Bool

    var body: some View {
        GeometryReader { geometry in
            let basis = OrthoViewBasis(standard)
            // The session's framing, not one measured from this sample: see
            // AnnotationViewState.swift.
            let viewport = session.viewport(for: standard, size: geometry.size)
            ZStack(alignment: .topLeading) {
                Color.black
                AnnotationPointCanvas(
                    points: session.currentPoints, classes: session.currentClasses, basis: basis,
                    viewport: viewport, visibility: session.effectiveVisibility,
                    identity: AnnotationPointCanvas.Identity(
                        packDigest: session.pack.manifest.packDigest,
                        sampleID: session.currentSample?.sampleID ?? -1)
                ).equatable()
                LassoOverlay(
                    session: session, basisStandard: standard, viewport: viewport,
                    editable: editable)
                Text(standard.label + (editable ? "" : " · check")).font(.caption2).padding(4)
                    .foregroundStyle(.secondary).allowsHitTesting(false)
                scaleBar(viewport).frame(
                    maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomLeading
                ).allowsHitTesting(false)
            }.clipped()
        }
    }

    // A length the operator can hold a car against. With the framing held
    // still it stays the same from sample to sample, and says what the zoom is.
    private func scaleBar(_ viewport: OrthoViewport) -> some View {
        let metres = AnnotationScaleBar.length(forHalfHeight: viewport.halfHeight)
        let points =
            viewport.size.height > 0
            ? CGFloat(metres / (viewport.halfHeight * 2)) * viewport.size.height : 0
        return VStack(alignment: .leading, spacing: 2) {
            Text(AnnotationScaleBar.label(metres)).font(.caption2.monospacedDigit())
            Rectangle().frame(width: max(points, 1), height: 2)
        }.foregroundStyle(.secondary).padding(6)
    }
}

/// Picks the scale bar's length: the largest of 1, 2 or 5 times a power of ten
/// that is no more than a quarter of the view's height.
enum AnnotationScaleBar {
    static func length(forHalfHeight halfHeight: Float) -> Float {
        let limit = max(halfHeight, 0.01) / 2
        let decade = pow(10, floor(log10(limit)))
        for multiple in [Float(5), 2, 1] where multiple * decade <= limit {
            return multiple * decade
        }
        return decade
    }

    static func label(_ metres: Float) -> String {
        metres >= 1 ? String(format: "%.0f m", metres) : String(format: "%.0f cm", metres * 100)
    }
}

/// Draws the sample's points, coloured by class as the main view colours them.
///
/// Each class goes into one Path and is filled once. A full-coverage sample
/// is a whole revolution — tens of thousands of points — and a fill per point
/// makes stepping through samples visibly slow.
struct AnnotationPointCanvas: View, Equatable {
    /// Which points these are, without comparing them.
    struct Identity: Equatable {
        var packDigest: String
        var sampleID: Int
    }

    let points: PackPoints
    /// Display classes: see `PointClass.displayClasses`.
    let classes: [UInt8]
    let basis: OrthoViewBasis
    let viewport: OrthoViewport
    let visibility: PointVisibility?
    let identity: Identity

    // Points are immutable under a pack digest and sample, so two canvases
    // with the same identity draw the same points. Comparing that instead of
    // the arrays is what lets the session publish during a drag, for the
    // candidate count, without every publish redrawing the whole cloud.
    static func == (lhs: AnnotationPointCanvas, rhs: AnnotationPointCanvas) -> Bool {
        lhs.identity == rhs.identity && lhs.basis == rhs.basis && lhs.viewport == rhs.viewport
            && lhs.visibility == rhs.visibility
    }

    static let backgroundColour = Color(red: 0.55, green: 0.55, blue: 0.62)
    static let foregroundColour = Color(red: 0.35, green: 0.95, blue: 0.4)
    static let groundColour = Color(red: 0.68, green: 0.57, blue: 0.38)

    var body: some View {
        Canvas { context, size in
            var background = Path()
            var foreground = Path()
            var ground = Path()
            var unclassified = Path()
            // Generous, so a return on the edge is drawn rather than popping
            // in a frame late while panning.
            let visible = CGRect(origin: .zero, size: size).insetBy(dx: -2, dy: -2)
            for index in 0..<points.count {
                guard PointVisibility.isVisible(index, classes: classes, under: visibility) else {
                    continue
                }
                let p = simd_float3(points.x[index], points.y[index], points.z[index])
                let screen = viewport.screenPoint(from: basis.project(p))
                // Zoomed in, most of the sample is off screen, and a path of
                // tens of thousands of rectangles nobody can see is most of
                // the cost of a redraw.
                guard visible.contains(screen) else { continue }
                let rect = CGRect(x: screen.x - 0.75, y: screen.y - 0.75, width: 1.5, height: 1.5)
                switch index < classes.count ? classes[index] : PointClass.unclassified {
                case PointClass.foreground: foreground.addRect(rect)
                case PointClass.ground: ground.addRect(rect)
                case PointClass.background: background.addRect(rect)
                default: unclassified.addRect(rect)
                }
            }
            context.fill(background, with: .color(Self.backgroundColour.opacity(0.6)))
            context.fill(ground, with: .color(Self.groundColour.opacity(0.7)))
            context.fill(unclassified, with: .color(.white.opacity(0.65)))
            context.fill(foreground, with: .color(Self.foregroundColour.opacity(0.9)))
        }.allowsHitTesting(false)
    }
}
