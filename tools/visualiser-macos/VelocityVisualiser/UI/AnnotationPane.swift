// AnnotationPane.swift
// The operator-facing point-cloud editing toolset.
//
// Three pieces: an overlay that turns a drag into view-plane metres, the
// controls that decide what the gesture means, and a second view that has to
// agree before a mask can be marked reviewed.
//
// The overlay converts screen points to metres and hands those to the session;
// it never decides membership itself. That keeps the selection rule in one
// tested place instead of split between a gesture handler and an engine.

import SwiftUI
import simd

// MARK: - Screen/world mapping

/// Maps between a view's points and an orthographic view plane's metres.
///
/// Kept as a value type with no SwiftUI dependency so the conversion is
/// testable without mounting a view: an off-by-one in the vertical flip is
/// exactly the kind of bug that silently selects the wrong points.
struct OrthoViewport: Equatable {
    /// Half-height of the visible world volume, in metres.
    var halfHeight: Float
    /// The view's size in points.
    var size: CGSize
    /// View-plane coordinates at the centre of the view, in metres.
    var centre: simd_float2

    var halfWidth: Float {
        guard size.height > 0 else { return halfHeight }
        return halfHeight * Float(size.width / size.height)
    }

    /// Screen point (origin top-left, y down) to view-plane metres (y up).
    func worldPoint(from screen: CGPoint) -> simd_float2 {
        guard size.width > 0, size.height > 0 else { return centre }
        let nx = Float(screen.x / size.width) * 2 - 1
        // Screen y grows downward; the view plane's up axis grows upward.
        let ny = 1 - Float(screen.y / size.height) * 2
        return simd_float2(centre.x + nx * halfWidth, centre.y + ny * halfHeight)
    }

    /// View-plane metres back to a screen point, for drawing the outline and
    /// the selected points over the render.
    func screenPoint(from world: simd_float2) -> CGPoint {
        guard halfWidth > 0, halfHeight > 0 else { return .zero }
        let nx = (world.x - centre.x) / halfWidth
        let ny = (world.y - centre.y) / halfHeight
        return CGPoint(
            x: CGFloat((nx + 1) * 0.5) * size.width, y: CGFloat((1 - ny) * 0.5) * size.height)
    }
}

// MARK: - Lasso overlay

/// Captures a selection stroke and previews the candidates, and pans and
/// zooms the view it lies over.
///
/// A lasso drag is sampled into a polygon; holding shift adds, option
/// subtracts, as the session's mode resolution decides. The candidate count
/// appears while the stroke is being made, which is what the workflow
/// requires.
struct LassoOverlay: View {
    @ObservedObject var session: AnnotationSession
    /// Which view this overlay belongs to. The second view is read-only: it
    /// exists to check a selection, not to make one.
    let basisStandard: OrthoViewBasis.Standard
    let viewport: OrthoViewport
    var editable: Bool = true

    @State private var strokePoints: [CGPoint] = []
    @State private var rectangleMode = false
    /// Where a brush was at the last drag event, in world metres.
    @State private var lastBrushPosition: simd_float2?
    @State private var paintedCells: Set<ColumnCell> = []
    @State private var strokeNote: String?

    private var basis: OrthoViewBasis { OrthoViewBasis(basisStandard) }

    /// Metres on the view plane per point on screen.
    private var metresPerPoint: Float {
        guard viewport.size.height > 0 else { return 0 }
        return viewport.halfHeight * 2 / Float(viewport.size.height)
    }

    private var showsGrid: Bool { session.tool == .column || session.showColumnGrid }

    var body: some View {
        GeometryReader { _ in
            ZStack(alignment: .topLeading) {
                // Underneath everything and the only layer that takes input:
                // the layers above are drawings of the session's state.
                ViewportInputLayer(
                    strokesEnabled: editable, onStrokeChanged: strokeChanged,
                    onStrokeEnded: strokeEnded,
                    onPan: { session.pan(basisStandard, size: viewport.size, byPoints: $0) },
                    onZoom: { factor, anchor in
                        session.zoom(
                            basisStandard, size: viewport.size, by: factor, aboutScreenPoint: anchor
                        )
                    }, onClick: { _ in if !editable { session.makeEditingView(basisStandard) } },
                    onHover: { location in
                        guard editable else { return }
                        session.hover(
                            atViewPoint: location.map { viewport.worldPoint(from: $0) },
                            pickDistance: metresPerPoint * 12)
                    }, onDepthStep: { if editable { session.adjustBrushDepth(steps: $0) } },
                    onKey: handleKey)

                if showsGrid { gridLayer }
                maskLayer
                if strokePoints.count > 1 { strokeOutline }
                if session.pendingSphere != nil || session.hoverSphere != nil { sphereOutline }
                if let candidates = session.pendingCandidates, editable {
                    candidateBadge(candidates)
                } else if session.carried != nil, editable {
                    carriedBadge
                } else if let sphere = session.hoverSphere, editable {
                    hoverBadge(sphere)
                } else if let strokeNote, editable {
                    noteBadge(strokeNote)
                }
            }
        }
    }

    // The masks, drawn over the points so the operator sees them rather than
    // inferring them from a count.
    //
    // A handful of states, each one Path filled once: a membership can run to
    // thousands of points, and a fill per point makes the preview stutter
    // during the very drag it is meant to give feedback on.
    private var maskLayer: some View {
        // Read here, on the main actor, and not inside the renderer closure.
        let points = session.currentPoints
        let others = session.otherMasks
        let saved = session.savedSelection
        let current = session.history.current
        let activeClass = session.activeObject?.objectClass
        let activeName = session.activeObjectName
        let carried = session.carriedIndices
        let hovered = session.hoverIndices
        let proposed = session.proposedIndices
        let proposal = session.proposalIndices
        let viewport = viewport
        let basis = basis

        return Canvas { context, _ in
            func dots(_ indices: some Sequence<Int>, size: CGFloat) -> Path {
                var path = Path()
                for index in indices {
                    guard let p = points.point(at: index) else { continue }
                    let screen = viewport.screenPoint(from: basis.project(p))
                    path.addEllipse(
                        in: CGRect(
                            x: screen.x - size / 2, y: screen.y - size / 2, width: size,
                            height: size))
                }
                return path
            }

            // An object's name, above its points, as the main view names a
            // track above its box. Without it two cars are two blue patches.
            func name(_ text: String, over indices: some Sequence<Int>, colour: Color, bold: Bool) {
                var top = CGFloat.greatestFiniteMagnitude
                var sumX: CGFloat = 0
                var count: CGFloat = 0
                for index in indices {
                    guard let p = points.point(at: index) else { continue }
                    let screen = viewport.screenPoint(from: basis.project(p))
                    top = min(top, screen.y)
                    sumX += screen.x
                    count += 1
                }
                guard count > 0 else { return }
                let label = Text(text).font(.system(size: 10, weight: bold ? .bold : .regular))
                    .foregroundColor(colour)
                context.draw(label, at: CGPoint(x: sumX / count, y: top - 8))
            }

            // What is already labelled, in each object's own colour.
            for other in others {
                let colour = AnnotationPalette.colour(forClass: other.objectClass)
                context.fill(dots(other.indices, size: 3), with: .color(colour.opacity(0.8)))
                name(other.name, over: other.indices, colour: colour, bold: false)
            }

            // Saved and still in: the object's colour. In but not saved:
            // orange. Saved but taken out since: a red ring round the return.
            if let activeClass {
                context.fill(
                    dots(current.intersection(saved), size: 4),
                    with: .color(AnnotationPalette.colour(forClass: activeClass)))
            }
            context.fill(
                dots(current.subtracting(saved), size: 4),
                with: .color(AnnotationPalette.colour(AnnotationPalette.unsavedIndex)))
            context.stroke(
                dots(saved.subtracting(current), size: 6),
                with: .color(AnnotationPalette.colour(AnnotationPalette.removedIndex)), lineWidth: 1
            )
            if let activeName, let activeClass {
                name(
                    activeName, over: current.union(saved),
                    colour: AnnotationPalette.colour(forClass: activeClass), bold: true)
            }

            // Proposals: what the carried footprint covers, and what the brush
            // under the cursor would take.
            // Everything waiting to be graded in this frame, faintly, and the
            // proposal being looked at, boldly.
            context.fill(dots(proposed, size: 2.5), with: .color(.cyan.opacity(0.45)))
            context.stroke(dots(proposal, size: 5), with: .color(.cyan), lineWidth: 1.2)
            context.stroke(dots(carried, size: 5), with: .color(.cyan), lineWidth: 1)
            context.fill(
                dots(hovered, size: 3),
                with: .color(AnnotationPalette.colour(AnnotationPalette.candidateIndex)))
        }.allowsHitTesting(false)
    }

    private var strokeOutline: some View {
        Path { path in
            path.move(to: strokePoints[0])
            for p in strokePoints.dropFirst() { path.addLine(to: p) }
            path.closeSubpath()
        }.stroke(Color.yellow, style: StrokeStyle(lineWidth: 1.5, dash: [4, 3])).allowsHitTesting(
            false)
    }

    // The sphere of the stroke in progress, in this view. An orthographic
    // projection of a sphere is a circle of the same radius, so both views
    // draw it true to scale; the second view shows its reach along the axis
    // the operator dragging in the first cannot see.
    private var sphereOutline: some View {
        Canvas { context, _ in
            guard let sphere = session.pendingSphere ?? session.hoverSphere, metresPerPoint > 0
            else { return }
            let centre = viewport.screenPoint(from: basis.project(sphere.centre))
            let r = CGFloat(sphere.radius / metresPerPoint)
            let circle = Path(
                ellipseIn: CGRect(x: centre.x - r, y: centre.y - r, width: r * 2, height: r * 2))
            context.stroke(
                circle, with: .color(.yellow), style: StrokeStyle(lineWidth: 1.5, dash: [4, 3]))
            let mark = Path(
                ellipseIn: CGRect(x: centre.x - 2.5, y: centre.y - 2.5, width: 5, height: 5))
            context.fill(mark, with: .color(.yellow))
        }.allowsHitTesting(false)
    }

    // The local column lattice.
    //
    // In the top view it is the lattice itself, with the painted columns
    // filled. In the front and side views a column is a vertical strip, so
    // what is drawn instead is the ground and the voxel boundaries above it,
    // the enabled voxels shaded: the operator can see that "not the ground"
    // means what they intend before they paint.
    private var gridLayer: some View {
        Canvas { context, size in
            guard metresPerPoint > 0 else { return }
            let grid = session.columnGrid
            if basisStandard == .top {
                drawLattice(grid, in: &context, size: size)
            } else {
                drawVoxelBands(grid, in: &context, size: size)
            }
        }.allowsHitTesting(false)
    }

    private func drawLattice(_ grid: ColumnGrid, in context: inout GraphicsContext, size: CGSize) {
        var fill = Path()
        let cells = session.pendingCells.union(paintedCells)
        for cell in cells {
            let lo = viewport.screenPoint(
                from: simd_float2(Float(cell.i) * grid.pitch, Float(cell.j) * grid.pitch))
            let hi = viewport.screenPoint(
                from: simd_float2(Float(cell.i + 1) * grid.pitch, Float(cell.j + 1) * grid.pitch))
            fill.addRect(
                CGRect(
                    x: min(lo.x, hi.x), y: min(lo.y, hi.y), width: abs(hi.x - lo.x),
                    height: abs(hi.y - lo.y)))
        }
        context.fill(fill, with: .color(.yellow.opacity(0.22)))

        // Below a few points per column the lines would be a grey wash that
        // hides the returns. The painted columns above are still drawn.
        guard CGFloat(grid.pitch / metresPerPoint) >= 5 else { return }
        let topLeft = viewport.worldPoint(from: .zero)
        let bottomRight = viewport.worldPoint(from: CGPoint(x: size.width, y: size.height))
        var lines = Path()
        let firstI = Int((min(topLeft.x, bottomRight.x) / grid.pitch).rounded(.down))
        let lastI = Int((max(topLeft.x, bottomRight.x) / grid.pitch).rounded(.up))
        for i in firstI...lastI {
            let x = viewport.screenPoint(from: simd_float2(Float(i) * grid.pitch, 0)).x
            lines.move(to: CGPoint(x: x, y: 0))
            lines.addLine(to: CGPoint(x: x, y: size.height))
        }
        let firstJ = Int((min(topLeft.y, bottomRight.y) / grid.pitch).rounded(.down))
        let lastJ = Int((max(topLeft.y, bottomRight.y) / grid.pitch).rounded(.up))
        for j in firstJ...lastJ {
            let y = viewport.screenPoint(from: simd_float2(0, Float(j) * grid.pitch)).y
            lines.move(to: CGPoint(x: 0, y: y))
            lines.addLine(to: CGPoint(x: size.width, y: y))
        }
        context.stroke(lines, with: .color(.white.opacity(0.13)), lineWidth: 0.5)
    }

    private func drawVoxelBands(_ grid: ColumnGrid, in context: inout GraphicsContext, size: CGSize)
    {
        // Front and side views both have world Z as their vertical axis.
        func screenY(height: Float) -> CGFloat {
            viewport.screenPoint(from: simd_float2(viewport.centre.x, grid.groundZ + height)).y
        }
        for k in 0..<ColumnGrid.voxelCount where session.enabledVoxels & (1 << UInt8(k)) != 0 {
            let range = grid.heightRange(ofVoxel: k)
            let top = screenY(height: range.upperBound)
            let bottom = screenY(height: range.lowerBound)
            context.fill(
                Path(CGRect(x: 0, y: top, width: size.width, height: bottom - top)),
                with: .color(.yellow.opacity(0.07)))
        }
        var boundaries = Path()
        for k in 1...ColumnGrid.voxelCount {
            let y = screenY(height: Float(k) * grid.pitch)
            boundaries.move(to: CGPoint(x: 0, y: y))
            boundaries.addLine(to: CGPoint(x: size.width, y: y))
        }
        context.stroke(boundaries, with: .color(.white.opacity(0.13)), lineWidth: 0.5)
        var ground = Path()
        ground.move(to: CGPoint(x: 0, y: screenY(height: 0)))
        ground.addLine(to: CGPoint(x: size.width, y: screenY(height: 0)))
        context.stroke(ground, with: .color(.green.opacity(0.7)), lineWidth: 1)
    }

    private func candidateBadge(_ candidates: SelectionCandidates) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text("\(candidates.count) point\(candidates.count == 1 ? "" : "s")").font(
                .caption.bold())
            if candidates.excludedBySlab > 0 {
                // Say why the count is lower than expected rather than
                // leaving the operator to assume the lasso missed.
                Text("\(candidates.excludedBySlab) outside the slab").font(.caption2)
                    .foregroundStyle(.secondary)
            }
            if candidates.excludedByVoxels > 0 {
                Text("\(candidates.excludedByVoxels) in voxels that are off").font(.caption2)
                    .foregroundStyle(.secondary)
            }
            if candidates.excludedByVisibility > 0 {
                Text("\(candidates.excludedByVisibility) in classes that are hidden").font(
                    .caption2
                ).foregroundStyle(.secondary)
            }
            if let sphere = session.pendingSphere {
                Text(String(format: "radius %.2f m", sphere.radius)).font(.caption2)
                    .foregroundStyle(.secondary)
            }
            // The stroke is applied when the button comes up. Undo takes it
            // back; there is no separate accept step to describe.
            Text("Release to apply").font(.caption2).foregroundStyle(.secondary)
        }.padding(6).background(.black.opacity(0.7), in: RoundedRectangle(cornerRadius: 4)).padding(
            8
        ).allowsHitTesting(false)
    }

    // Where the brush is, in the terms the view it is in cannot show.
    private func hoverBadge(_ sphere: SelectionSphere) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text("\(session.hoverIndices.count) under the brush").font(.caption.bold())
            Text(String(format: "centre z %.2f m · radius %.2f m", sphere.centre.z, sphere.radius))
                .font(.caption2).foregroundStyle(.secondary)
            if session.brushDepthOffset != 0 {
                Text(String(format: "moved %+.1f m in depth", session.brushDepthOffset)).font(
                    .caption2
                ).foregroundStyle(.secondary)
            }
        }.padding(6).background(.black.opacity(0.7), in: RoundedRectangle(cornerRadius: 4)).padding(
            8
        ).allowsHitTesting(false)
    }

    private var carriedBadge: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text("\(session.carriedIndices.count) points carried over").font(.caption.bold())
            Text("Arrows nudge (⇧ for 0.5 m) · Return accepts · Esc dismisses").font(.caption2)
                .foregroundStyle(.secondary)
        }.padding(6).background(.black.opacity(0.7), in: RoundedRectangle(cornerRadius: 4)).padding(
            8
        ).allowsHitTesting(false)
    }

    // Keys the editing view takes while it has the focus. Only the carried
    // proposal uses them, and with none on screen they go on up the chain.
    private func handleKey(_ key: ViewportKey) -> Bool {
        guard editable, session.carried != nil else { return false }
        switch key {
        case .nudge(let right, let up, let coarse):
            let step = coarse ? AnnotationSession.coarseNudgeStep : AnnotationSession.nudgeStep
            session.nudgeCarried(right: Float(right) * step, up: Float(up) * step)
        case .accept: session.acceptCarried()
        case .cancel: session.dismissCarried()
        }
        return true
    }

    private func noteBadge(_ note: String) -> some View {
        Text(note).font(.caption2).foregroundStyle(.secondary).padding(6).background(
            .black.opacity(0.7), in: RoundedRectangle(cornerRadius: 4)
        ).padding(8).allowsHitTesting(false)
    }

    // Called from the button going down, so a brush responds to a click. A
    // click with the lasso samples one vertex, which encloses nothing, and
    // ends as a stroke that named nothing.
    private func strokeChanged(_ value: ViewportStroke) {
        switch session.tool {
        case .lasso: lassoChanged(value)
        case .sphere: sphereChanged(value)
        case .column: columnChanged(value)
        }
    }

    private func strokeEnded(_ value: ViewportStroke) {
        defer { resetStroke() }
        session.selectionMode = SelectionMode.from(
            tool: session.tool, shiftHeld: NSEvent.modifierFlags.contains(.shift),
            optionHeld: NSEvent.modifierFlags.contains(.option))
        switch session.tool {
        case .lasso: previewCurrentStroke()
        case .sphere, .column: break
        }
        // A gesture that named nothing (a click with the lasso, a sphere
        // with no return under it, a column brush outside the top view)
        // must not leave a stroke open: navigation is guarded on it.
        guard session.pendingCandidates != nil else {
            session.cancelStroke()
            return
        }
        _ = session.commitSelection()
    }

    private func resetStroke() {
        strokePoints = []
        lastBrushPosition = nil
        paintedCells = []
    }

    private func lassoChanged(_ value: ViewportStroke) {
        if strokePoints.isEmpty {
            session.beginStroke()
            rectangleMode = NSEvent.modifierFlags.contains(.command)
            strokeNote = nil
        }
        if rectangleMode {
            // Command-drag is the quick axis-aligned rectangle.
            strokePoints = rectangleCorners(from: value.startLocation, to: value.location)
        } else {
            strokePoints.append(value.location)
        }
        previewCurrentStroke()
    }

    // The sphere is a brush: it marks wherever it is dragged, at the radius
    // the slider and the bracket keys set.
    private func sphereChanged(_ value: ViewportStroke) {
        let position = viewport.worldPoint(from: value.location)
        if lastBrushPosition == nil {
            session.beginStroke()
            strokeNote = nil
        }
        for step in BrushStroke.path(
            from: lastBrushPosition ?? position, to: position, radius: session.sphereRadius)
        {
            session.paint(
                sphere: session.brushSphere(atViewPoint: step, pickDistance: metresPerPoint * 12))
        }
        lastBrushPosition = position
    }

    private func columnChanged(_ value: ViewportStroke) {
        // A column is vertical, so it is chosen from above. In the other views
        // a click names a strip of columns one behind another, which is a
        // lasso's job.
        guard basisStandard == .top else {
            strokeNote = "The column brush paints in the Top view"
            return
        }
        if lastBrushPosition == nil {
            session.beginStroke()
            strokeNote = nil
        }
        let position = viewport.worldPoint(from: value.location)
        paintedCells.formUnion(
            BrushStroke.cells(
                from: lastBrushPosition ?? position, to: position, grid: session.columnGrid,
                radius: session.columnBrushRadius))
        lastBrushPosition = position
        session.previewSelection(cells: paintedCells)
    }

    private func rectangleCorners(from a: CGPoint, to b: CGPoint) -> [CGPoint] {
        [
            CGPoint(x: a.x, y: a.y), CGPoint(x: b.x, y: a.y), CGPoint(x: b.x, y: b.y),
            CGPoint(x: a.x, y: b.y),
        ]
    }

    private func previewCurrentStroke() {
        guard strokePoints.count >= 3 else { return }
        let polygon = SelectionPolygon(vertices: strokePoints.map { viewport.worldPoint(from: $0) })
        session.previewSelection(polygon: polygon)
    }
}

// MARK: - Controls

/// The annotation controls: object identity, view, slab, review and save.
struct AnnotationPane: View {
    /// The controls are two columns either side of the views, not one.
    enum Column {
        /// What is being labelled: the run and frame, progress, proposals and
        /// objects.
        case objects
        /// How: display, tools, slab, carrying, saving and review.
        case editing
    }

    static let columnWidth: CGFloat = 280

    @ObservedObject var session: AnnotationSession
    var column: Column = .objects

    @State private var newObjectClass = "car"
    @State private var newObjectSubtype = ""
    @State private var showDiscardPrompt = false

    @State private var applyResult: String?
    @State private var showReviewAllPrompt = false
    @State private var proposalClass = "car"
    @State private var proposalSort = ProposalSort.mostFrames
    @State private var proposalFilter = ProposalFilter()
    @State private var splitClass = "pedestrian"

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                switch column {
                case .objects:
                    sourceSection
                    Divider()
                    progressSection
                    Divider()
                    proposalSection
                    Divider()
                    objectSection
                case .editing:
                    displaySection
                    Divider()
                    selectionSection
                    Divider()
                    slabSection
                    if session.activeObjectID != nil {
                        Divider()
                        propagationSection
                    }
                    if session.carried != nil {
                        Divider()
                        carriedSection
                    }
                    Divider()
                    reviewSection
                }
            }.padding(12)
        }.frame(width: AnnotationPane.columnWidth).alert(
            "Unsaved membership", isPresented: $showDiscardPrompt
        ) {
            Button("Keep editing", role: .cancel) {}
            Button("Discard and reload", role: .destructive) { session.reload() }
        } message: {
            Text("This frame has unsaved changes. Save it, or discard them, before moving on.")
        }
    }

    // MARK: Source

    // The main view's words, for the things that are the same thing: a run,
    // a frame, a label and who made it. "Source", "sample" and "operator" were
    // this window's own, and an operator who knew the main view had to work
    // out that a sample is a frame and that the operator field wanted their
    // name and not the name of a track.
    private var sourceSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Run").font(.headline)
            Text(session.pack.manifest.source.vrlogPath).font(.caption.monospaced()).textSelection(
                .enabled
            ).lineLimit(1).truncationMode(.middle).help(
                "The run this pack was cut from. Pack \(session.pack.manifest.datasetID).")
            if let pcap = session.pack.manifest.source.pcapBasename {
                Text(pcap).font(.caption2).foregroundStyle(.secondary).lineLimit(1).truncationMode(
                    .middle)
            }
            // Coverage is shown before any labelling: a foreground-only pack
            // cannot support whole-scene segmentation, and a mask saved in
            // ignorance of that reads as a stronger claim than it is.
            Text(session.pack.coverageCaveat).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)

            if let sample = session.currentSample {
                // The run's own frame number first: it is the one the main
                // view's timeline shows.
                Text(
                    "Frame \(sample.sourceOrdinal) · \(session.sampleIndex + 1) of "
                        + "\(session.samples.count) in this pack · \(session.currentPoints.count) points"
                ).font(.caption)
            }
            backgroundLine
            HStack {
                Button("Previous frame") { handleStep { session.stepBackward() } }.disabled(
                    session.sampleIndex == 0)
                Button("Next frame") { handleStep { session.stepForward() } }.disabled(
                    session.sampleIndex >= session.samples.count - 1)
            }.controlSize(.small)

            Text("Labelled by").font(.caption).padding(.top, 4)
            TextField("Your name", text: $session.operatorName).textFieldStyle(.roundedBorder).font(
                .caption
            ).help(
                "Who is labelling. Saved with every label you make, as the main view's track "
                    + "labels are. Not the name of an object or a track.")
        }
    }

    // Which settled background is behind this frame, and how long it has been.
    private var backgroundLine: some View {
        Group {
            if let background = session.currentBackground, let sample = session.currentSample {
                let age = sample.sourceOrdinal - background.sourceOrdinal
                HStack(spacing: 4) {
                    Image(systemName: "square.stack.3d.down.forward")
                    Text(
                        "Settled background \(background.backgroundID + 1) of "
                            + "\(session.pack.backgrounds.count) · \(background.pointCount) points · "
                            + (age == 0 ? "this frame" : "\(age) frames old"))
                }.font(.caption2).foregroundStyle(
                    session.backgroundUpdatedHere ? Color.yellow : Color.secondary)
            } else {
                Text("No settled background in this pack. Generate it again to carry one.").font(
                    .caption2
                ).foregroundStyle(.secondary)
            }
        }
    }

    // MARK: Progress

    private var progressSection: some View {
        let completeness = session.completeness
        return VStack(alignment: .leading, spacing: 6) {
            Text("This frame").font(.headline)
            HStack(alignment: .center, spacing: 10) {
                SectorRing(completeness: completeness) { session.fitViews(toSector: $0) }.frame(
                    width: 118, height: 118)
                VStack(alignment: .leading, spacing: 3) {
                    Text(String(format: "%.0f%% agreed", completeness.whole.fractionAgreed * 100))
                        .font(.caption.bold())
                    tallyLine(
                        "agreed", completeness.whole.agreed, AnnotationFrameStrip.agreedColour)
                    tallyLine(
                        "in question", completeness.whole.inQuestion,
                        AnnotationFrameStrip.inQuestionColour)
                    tallyLine("not labelled", completeness.whole.unlabelled, .gray)
                }
            }
            Text(
                "Foreground the tracker could use, in 22.5° sectors round the sensor, laid out as "
                    + "the top view is. Agreed is labelled by a person; in question is proposed or "
                    + "unsaved. Click a sector to go to what is left in it."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
        }
    }

    private func tallyLine(_ label: String, _ count: Int, _ colour: Color) -> some View {
        HStack(spacing: 4) {
            Circle().fill(colour).frame(width: 7, height: 7)
            Text("\(count) \(label)").font(.caption2.monospacedDigit()).foregroundStyle(.secondary)
        }
    }

    // MARK: Proposals

    /// The proposals worth an operator's attention first: fixed clutter, and
    /// what moved some distance or lasted a couple of seconds. The rest is
    /// mostly the speckle of every frame, and is there when asked for.
    private var listedProposals: [ObjectProposal] {
        proposalSort.sorted(session.proposals.filter(proposalFilter.admits))
    }

    private var proposalSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Proposed objects").font(.headline)
            if let frame = session.proposalProgress {
                HStack(spacing: 6) {
                    ProgressView().controlSize(.small)
                    Text("Reading the pack · \(frame + 1) of \(session.samples.count)").font(
                        .caption.monospacedDigit())
                }
            } else {
                Button(session.proposals.isEmpty ? "Propose objects" : "Propose again") {
                    Task { await session.proposeObjects() }
                }.controlSize(.small).help(
                    "Finds what nobody has labelled yet: clutter that stays put, as one proposal "
                        + "a patch, and everything else as one proposal an object, followed "
                        + "through the frames. You grade each once.")
            }

            if !session.proposals.isEmpty {
                let listed = listedProposals
                Text(
                    "\(listed.count) listed of \(session.proposals.count). Click one, step through "
                        + "its frames, then accept, split, merge or reject it."
                ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                    horizontal: false, vertical: true)
                HStack(spacing: 4) {
                    Picker("Sort", selection: $proposalSort) {
                        ForEach(ProposalSort.allCases) { Text($0.label).tag($0) }
                    }.labelsHidden()
                    Picker("Type", selection: $proposalFilter.type) {
                        Text("All types").tag(String?.none)
                        ForEach(ProposalFilter.types(in: session.proposals), id: \.name) { type in
                            Text("\(type.name) (\(type.count))").tag(String?.some(type.name))
                        }
                    }.labelsHidden()
                }.controlSize(.small).font(.caption2)
                Toggle("List the small and short ones too", isOn: $proposalFilter.includeSmall)
                    .font(.caption2)
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 2) {
                        ForEach(listed) { proposal in proposalRow(proposal) }
                    }
                }.frame(maxHeight: 170)
                if let selected = session.selectedProposal { proposalActions(selected) }
            }
        }
    }

    private func proposalRow(_ proposal: ObjectProposal) -> some View {
        let isSelected = session.selectedProposalID == proposal.id
        return HStack(spacing: 5) {
            RoundedRectangle(cornerRadius: 2).fill(
                AnnotationPalette.colour(forClass: proposal.classGuess)
            ).frame(width: 8, height: 8)
            Text(
                "\(proposal.kind == .fixed ? "fixed" : "≈ " + proposal.classGuess) · frames "
                    + "\(proposal.firstFrame + 1)–\(proposal.lastFrame + 1) · ~\(proposal.meanPoints) pts"
                    + (proposal.kind == .moving
                        ? String(format: " · %.0f m", proposal.travelled) : "")
                    + (proposalSort == .steadiest
                        ? String(format: " · ±%.0f%%", proposal.unsteadiness * 100) : "")
            ).font(.caption2.monospacedDigit()).lineLimit(1)
            Spacer(minLength: 0)
        }.padding(.vertical, 2).padding(.horizontal, 4).background(
            isSelected ? Color.accentColor.opacity(0.25) : Color.clear,
            in: RoundedRectangle(cornerRadius: 3)
        ).contentShape(Rectangle()).onTapGesture {
            proposalClass = proposal.classGuess
            session.selectProposal(isSelected ? nil : proposal.id)
        }
    }

    // Grading one proposal. Split is by the frame on screen, because where a
    // chain ran from one car onto the next is something the operator sees
    // while stepping through it, not a number they know.
    private func proposalActions(_ proposal: ObjectProposal) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 4) {
                Picker("Class", selection: $proposalClass) {
                    ForEach(AnnotationPalette.classes) { Text($0.name).tag($0.name) }
                }.labelsHidden().font(.caption).frame(maxWidth: 110)
                Button("Accept") {
                    _ = session.acceptProposal(proposal.id, objectClass: proposalClass)
                }
                Button("Noise") { _ = session.acceptProposal(proposal.id, objectClass: "noise") }
                    .help("It is not an object: label its points as noise")
            }.controlSize(.small)
            HStack(spacing: 4) {
                Button("Accept up to here") {
                    _ = session.acceptProposal(
                        proposal.id, objectClass: proposalClass,
                        frames: proposal.firstFrame...session.sampleIndex)
                }.disabled(session.sampleIndex < proposal.firstFrame)
                Button("From here") {
                    _ = session.acceptProposal(
                        proposal.id, objectClass: proposalClass,
                        frames: session.sampleIndex...max(session.sampleIndex, proposal.lastFrame))
                }.disabled(session.sampleIndex > proposal.lastFrame)
            }.controlSize(.small).help(
                "Split: accept part of it, by the frame on screen. The rest stays proposed.")
            HStack(spacing: 4) {
                if let active = session.activeObjectID {
                    Button("Add to \(session.displayName(objectID: active))") {
                        _ = session.acceptProposal(
                            proposal.id, objectClass: proposalClass, into: active)
                    }.help(
                        "Merge: these are more frames of the object being edited. Frames it "
                            + "already has are left alone.")
                }
                Button("Dismiss") { session.dismissProposal(proposal.id) }
            }.controlSize(.small)
        }
    }

    // MARK: Display

    private var displaySection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Display").font(.headline)
            // The recorder's own classes, in the main view's colours. What is
            // hidden cannot be selected, so switching the background off is
            // also how a lasso is kept from taking the wall behind a van.
            let counts = session.classCounts
            HStack(spacing: 4) {
                Toggle(
                    "Background \(counts[PointClass.background, default: 0])",
                    isOn: $session.visibility.background)
                Toggle(
                    "Foreground \(counts[PointClass.foreground, default: 0])",
                    isOn: $session.visibility.foreground)
                Toggle(
                    "Ground \(counts[PointClass.ground, default: 0])",
                    isOn: $session.visibility.ground)
            }.toggleStyle(.button).controlSize(.small).font(.caption2)

            // Ground is what the run's height band took out before clustering,
            // so that what is left on screen is what the tracker had to work
            // with.
            Text(groundCaption).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            if counts[PointClass.background, default: 0] == 0,
                session.pack.manifest.hasClassification
            {
                Text("No background in this sample: the recording kept foreground returns only.")
                    .font(.caption2).foregroundStyle(.secondary).fixedSize(
                        horizontal: false, vertical: true)
            }

            // The views hold their framing from one sample to the next. These
            // are the only things that move them, other than the operator.
            HStack(spacing: 4) {
                Text("Fit").font(.caption)
                Button("Sample") { session.fitViews(to: .sample) }
                Button("Foreground") { session.fitViews(to: .foreground) }.disabled(
                    !session.pack.manifest.hasClassification)
                Button("Selection") { session.fitViews(to: .selection) }.disabled(
                    session.selectionCount == 0)
            }.controlSize(.small)
            Text(
                "Scroll or pinch to zoom. Drag with the right button, or with control held, to pan."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
        }
    }

    private var groundCaption: String {
        let band = session.heightBand
        guard band.removeGround else {
            return "Ground: this run removed nothing by height, so only recorded ground counts."
        }
        let rule = String(
            format: "Ground: below %.2f m or above %.2f m, what the run's height band removed",
            band.floorM, band.ceilingM)
        return session.heightBandIsAssumed
            ? rule + ". Assumed: this pack does not record its run's band. Generate it again to get"
                + " the run's own." : rule + "."
    }

    // MARK: Object

    private var objectSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Objects").font(.headline)
            // Said once, here, because nothing else on screen says it.
            Text(
                "An object is one real thing followed through the frames: this car, that wall. You "
                    + "label its points in each frame. It is your answer, where a track in the "
                    + "main view is the tracker's, and the two are kept apart so that a split "
                    + "track cannot split one. Click an object to edit it."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)

            HStack(spacing: 6) {
                RoundedRectangle(cornerRadius: 2).fill(
                    AnnotationPalette.colour(forClass: newObjectClass)
                ).frame(width: 10, height: 10)
                Picker("Class", selection: $newObjectClass) {
                    // Research subtypes stay in their own field: a dataset
                    // export must not enable a reserved production enum value
                    // through the class picker.
                    Section("Moves") {
                        ForEach(AnnotationPalette.classes.filter { $0.kind == .moving }) {
                            Text($0.name).tag($0.name)
                        }
                    }
                    Section("Does not move: misread as foreground") {
                        ForEach(AnnotationPalette.classes.filter { $0.kind == .fixed }) {
                            Text($0.name).tag($0.name)
                        }
                    }
                }.labelsHidden().font(.caption)
            }
            if let label = AnnotationPalette.annotationClass(named: newObjectClass)?.mainViewLabel {
                Text("The main view labels a track of this \"\(label)\".").font(.caption2)
                    .foregroundStyle(.secondary)
            }
            TextField("Subtype (optional)", text: $newObjectSubtype).textFieldStyle(.roundedBorder)
                .font(.caption)
            Button(session.selectionCount > 0 ? "New object from selection" : "New object") {
                // Refused with unsaved changes on another object: creating
                // one makes it the object under edit.
                guard session.activeObjectID == nil || session.navigationGuard() == nil else {
                    showDiscardPrompt = true
                    return
                }
                _ = session.createObject(
                    objectClass: newObjectClass,
                    subtype: newObjectSubtype.isEmpty ? nil : newObjectSubtype)
                session.secondViewChecked = false
            }

            if session.sidecar.objects.isEmpty {
                Text("No objects yet").font(.caption).foregroundStyle(.secondary)
            } else {
                ForEach(session.sidecar.objects) { object in objectRow(object) }
            }

            if let active = session.activeObject, !session.removedFromSaved.isEmpty,
                session.selectionCount > 0
            {
                splitControls(active)
            }

            if let active = session.activeObject, AnnotationPalette.isFixed(active.objectClass) {
                Button("Apply to every frame") {
                    applyResult = session.applySelectionToAllSamples().map {
                        "Saved \(session.displayName(objectID: active.objectID)) into \($0) frames."
                    }
                }.disabled(session.selectionCount == 0).help(
                    "A \(active.objectClass) does not move. Saves its points into every frame from "
                        + "the space this selection occupies, leaving frames already labelled alone."
                )
                if let applyResult { Text(applyResult).font(.caption2).foregroundStyle(.secondary) }
            }
        }
    }

    private func objectRow(_ object: AnnotationObject) -> some View {
        HStack(spacing: 6) {
            Image(
                systemName: session.activeObjectID == object.objectID
                    ? "largecircle.fill.circle" : "circle")
            RoundedRectangle(cornerRadius: 2).fill(
                AnnotationPalette.colour(forClass: object.objectClass)
            ).frame(width: 10, height: 10)
            VStack(alignment: .leading, spacing: 0) {
                Text(
                    session.displayName(objectID: object.objectID)
                        + (object.subtype.map { " · \($0)" } ?? "")
                ).font(.caption).bold(session.activeObjectID == object.objectID)
                // How far through the pack the object has been followed.
                Text(
                    "labelled in \(session.savedSampleCount(objectID: object.objectID)) of "
                        + "\(session.samples.count) frames · "
                        + "\(session.reviewedSampleCount(objectID: object.objectID)) reviewed"
                ).font(.caption2).foregroundStyle(.secondary).help(object.objectID)
            }
            Spacer()
            // Of its frames, not of the object record: an object marked
            // reviewed whose frames are not has nothing that counts.
            let saved = session.savedSampleCount(objectID: object.objectID)
            let reviewed = session.reviewedSampleCount(objectID: object.objectID)
            Text(
                saved > 0 && reviewed == saved
                    ? "reviewed" : reviewed > 0 ? "part reviewed" : "proposed"
            ).font(.caption2).foregroundStyle(
                saved > 0 && reviewed == saved ? Color.green : Color.secondary)
        }.contentShape(Rectangle()).onTapGesture {
            if session.activate(objectID: object.objectID) != nil {
                showDiscardPrompt = true
                return
            }
            session.secondViewChecked = false
            applyResult = nil
        }
    }

    // MARK: Propagation

    // One press for the frames an operator would only have agreed with, one
    // at a time, at six seconds each.
    private var propagationSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Carry through the frames").font(.headline)
            if let frame = session.propagationProgress {
                HStack(spacing: 6) {
                    ProgressView().controlSize(.small)
                    Text("Frame \(frame + 1) of \(session.samples.count)").font(
                        .caption.monospacedDigit())
                    Spacer()
                    Button("Stop") { session.cancelPropagation() }.controlSize(.small)
                }
            } else {
                HStack(spacing: 4) {
                    Button("◀◀ Back") { Task { await session.propagate(direction: -1) } }
                        .keyboardShortcut("[", modifiers: .command)
                    Button("Forward ▶▶") { Task { await session.propagate(direction: 1) } }
                        .keyboardShortcut("]", modifiers: .command)
                }.controlSize(.small).disabled(session.selectionCount == 0)
            }
            if let outcome = session.lastPropagation {
                Text(outcome.summary).font(.caption2).foregroundStyle(
                    outcome.stop.needsOperator ? Color.orange : Color.secondary
                ).fixedSize(horizontal: false, vertical: true)
            }
            Text(
                "From this frame's saved points, on through every frame where the fit is one you "
                    + "would have accepted. It stops and hands over where the object is lost, "
                    + "doubles, could be in two places, or is further than it could have moved. "
                    + "What it writes is in question until you review it."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
        }
    }

    // What was labelled as one object is two. The operator takes the second
    // one's points out of the first, which shows them ringed in red, and this
    // makes those a new object and divides the other frames the same way.
    private func splitControls(_ active: AnnotationObject) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(
                "\(session.removedFromSaved.count) points taken out of "
                    + "\(session.displayName(objectID: active.objectID)). If they are a second "
                    + "object, split them off."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            HStack(spacing: 4) {
                Picker("Class", selection: $splitClass) {
                    ForEach(AnnotationPalette.classes) { Text($0.name).tag($0.name) }
                }.labelsHidden().frame(maxWidth: 110)
                Button("Split off as new object") {
                    if let result = session.splitRemovedIntoNewObject(objectClass: splitClass) {
                        applyResult =
                            "Split \(session.displayName(objectID: result.objectID)) off through "
                            + "\(result.frames) frames."
                    }
                }
            }.controlSize(.small).help(
                "Divides this frame as you have, then carries the division through the other "
                    + "frames this object is labelled in, as far as the second object can be "
                    + "found. Every frame it changes goes back to proposed.")
            if let applyResult { Text(applyResult).font(.caption2).foregroundStyle(.secondary) }
        }.onAppear { splitClass = active.objectClass }
    }

    // MARK: Carried selection

    // The keys do the same from the editing view. These are here so that the
    // proposal can be dealt with without knowing the keys.
    private var carriedSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Carried from the last frame").font(.headline)
            Text(
                "\(session.carriedIndices.count) points, outlined in cyan. Move the outline over "
                    + "the object, then accept."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            HStack(spacing: 4) {
                let step = AnnotationSession.nudgeStep
                Button("◀") { session.nudgeCarried(right: -step, up: 0) }
                Button("▲") { session.nudgeCarried(right: 0, up: step) }
                Button("▼") { session.nudgeCarried(right: 0, up: -step) }
                Button("▶") { session.nudgeCarried(right: step, up: 0) }
                Button("Refit") { session.refitCarried() }.help(
                    "Search again for where the outline covers the most points, from here")
            }.controlSize(.small)
            HStack(spacing: 4) {
                Button("Accept and save") { if session.acceptCarried() { _ = session.save() } }
                    .keyboardShortcut(.return, modifiers: .command)
                Button("Accept") { session.acceptCarried() }
                Button("Dismiss") { session.dismissCarried() }
            }.controlSize(.small)
        }
    }

    // MARK: Selection

    private var selectionSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Selection").font(.headline)
            Picker("View", selection: $session.viewStandard) {
                ForEach(OrthoViewBasis.Standard.allCases, id: \.self) { Text($0.label).tag($0) }
            }.font(.caption)

            Picker("Tool", selection: $session.tool) {
                ForEach(SelectionTool.allCases) { Text($0.label).tag($0) }
            }.pickerStyle(.segmented).labelsHidden().onChange(of: session.tool) { _, _ in
                // Changing tool mid-stroke would apply one tool's gesture
                // under another's rule.
                session.cancelStroke()
            }

            Text("\(session.selectionCount) points selected").font(.caption)
            Text(session.tool.hint).font(.caption2).foregroundStyle(.secondary)
            legend

            switch session.tool {
            case .lasso: EmptyView()
            case .sphere: sphereControls
            case .column: columnControls
            }

            HStack {
                Button("Undo") { session.undo() }.disabled(!session.history.canUndo)
                Button("Redo") { session.redo() }.disabled(!session.history.canRedo)
                Button("Clear") { session.clearSelection() }.disabled(session.selectionCount == 0)
            }
        }
    }

    private var legend: some View {
        HStack(spacing: 8) {
            legendEntry(
                "saved",
                session.activeObject.map { AnnotationPalette.colour(forClass: $0.objectClass) }
                    ?? .gray)
            legendEntry("unsaved", AnnotationPalette.colour(AnnotationPalette.unsavedIndex))
            legendEntry("removed", AnnotationPalette.colour(AnnotationPalette.removedIndex))
            legendEntry("proposed", .cyan)
        }
    }

    private func legendEntry(_ label: String, _ colour: Color) -> some View {
        HStack(spacing: 3) {
            Circle().fill(colour).frame(width: 7, height: 7)
            Text(label).font(.caption2).foregroundStyle(.secondary)
        }
    }

    private var sphereControls: some View {
        HStack {
            Text("Radius").font(.caption)
            Slider(
                value: $session.sphereRadius,
                in: SelectionSphere.minimumRadius...SelectionSphere.maximumRadius)
            Text(String(format: "%.2f m", session.sphereRadius)).font(.caption.monospacedDigit())
                .frame(width: 52, alignment: .trailing)
        }.help("The brush's radius. [ and ] step it. Shift-scroll moves the brush in depth.")
    }

    private var columnControls: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("Brush").font(.caption)
                Slider(
                    value: $session.columnBrushRadius, in: 0...BrushStroke.maximumColumnRadius,
                    step: BrushStroke.columnRadiusStep)
                Text(
                    session.columnBrushRadius == 0
                        ? "1 column" : String(format: "%.2f m", session.columnBrushRadius)
                ).font(.caption.monospacedDigit()).frame(width: 60, alignment: .trailing)
            }

            // One toggle per voxel, ground at the left. The heights are the
            // point: "not the ground" is one click on the first of them.
            Text("Voxels, 0.5 m each from the ground").font(.caption2).foregroundStyle(.secondary)
            HStack(spacing: 3) {
                ForEach(0..<ColumnGrid.voxelCount, id: \.self) { k in
                    let bit = UInt8(1) << UInt8(k)
                    Toggle(
                        isOn: Binding(
                            get: { session.enabledVoxels & bit != 0 },
                            set: { on in
                                session.enabledVoxels =
                                    on ? session.enabledVoxels | bit : session.enabledVoxels & ~bit
                            })
                    ) { Text("\(k)").font(.caption2.monospacedDigit()).frame(width: 14) }
                    .toggleStyle(.button).controlSize(.small).help(
                        String(
                            format: "%.1f to %.1f m above the ground",
                            session.columnGrid.heightRange(ofVoxel: k).lowerBound,
                            session.columnGrid.heightRange(ofVoxel: k).upperBound))
                }
            }
            HStack {
                Button("Stack") { session.enabledVoxels = ColumnGrid.stackMask }.help(
                    "Voxels 1 to 4, 0.5 to 2.5 m: where road users are")
                Button("All") { session.enabledVoxels = ColumnGrid.allVoxelsMask }
            }.controlSize(.small)

            HStack {
                Text("Ground Z").font(.caption)
                Stepper(value: $session.columnGrid.groundZ, in: -20...20, step: 0.05) {
                    Text(String(format: "%.2f m", session.columnGrid.groundZ)).font(
                        .caption.monospacedDigit())
                }
                Button("Estimate") { session.estimateGround() }.controlSize(.small).help(
                    "The height below which a twentieth of this sample's returns lie")
            }
            Toggle("Show grid with other tools", isOn: $session.showColumnGrid).font(.caption)
        }
    }

    // MARK: Slab

    private var slabSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Depth slab").font(.headline)
            Text(session.viewStandard.depthAxisLabel).font(.caption).foregroundStyle(.secondary)
            if let slab = session.slab {
                // The slab is part of the selection rule, not a display
                // filter: it is what keeps a top-down lasso from taking a
                // lamp post along with the van beneath it.
                Text(String(format: "%.2f m to %.2f m", slab.minDepth, slab.maxDepth)).font(
                    .caption.monospaced())
                Slider(
                    value: Binding(
                        get: { Double(slab.minDepth) },
                        set: {
                            session.setSlab(DepthSlab(minDepth: Float($0), maxDepth: slab.maxDepth))
                        }), in: Double(slab.minDepth - 20)...Double(slab.maxDepth))
                Slider(
                    value: Binding(
                        get: { Double(slab.maxDepth) },
                        set: {
                            session.setSlab(DepthSlab(minDepth: slab.minDepth, maxDepth: Float($0)))
                        }), in: Double(slab.minDepth)...Double(slab.maxDepth + 20))
                Button("Reset to sample extent") { session.unpinSlab() }.font(.caption)
                if session.slabIsPinned {
                    Text("Kept from sample to sample until reset").font(.caption2).foregroundStyle(
                        .secondary)
                }
            } else {
                Text("No points in this sample").font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    // MARK: Review and save

    private var reviewSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Save").font(.headline)
            Picker("Visibility", selection: $session.maskVisibility) {
                ForEach(Visibility.allCases, id: \.self) { Text($0.label).tag($0) }
            }.font(.caption)
            Picker("Completeness", selection: $session.maskCompleteness) {
                ForEach(MaskCompleteness.allCases, id: \.self) { Text($0.label).tag($0) }
            }.font(.caption)

            // A membership seen from one angle has not been inspected for
            // contamination, so review is gated on the second view.
            Toggle("Checked from another view", isOn: $session.secondViewChecked).font(.caption)

            HStack {
                Button("Save points") { _ = session.save() }.keyboardShortcut(
                    "s", modifiers: .command)
                Button("Save and next") {
                    if session.save() { handleStep { session.stepForward() } }
                }.disabled(session.sampleIndex >= session.samples.count - 1)
            }
            HStack {
                Button("Review this frame") {
                    _ = session.markFrameReviewed(secondViewConfirmed: session.secondViewChecked)
                }.disabled(!session.secondViewChecked)
                Button("Review all frames…") { showReviewAllPrompt = true }.disabled(
                    !session.secondViewChecked || session.activeObjectID == nil)
            }
            if let id = session.activeObjectID {
                // Only a reviewed mask of a reviewed object is reference truth,
                // so this is the count that says how much of it there is.
                Text(
                    "\(session.reviewedSampleCount(objectID: id)) of "
                        + "\(session.savedSampleCount(objectID: id)) saved frames reviewed"
                ).font(.caption2).foregroundStyle(.secondary)
            }
            if session.navigationGuard() != nil {
                Text("Unsaved changes").font(.caption2).foregroundStyle(.orange)
            }
            Text("Revision \(session.sidecar.revision)").font(.caption2).foregroundStyle(.secondary)
        }.alert(
            "Review every frame of \(session.activeObjectName ?? "this object")?",
            isPresented: $showReviewAllPrompt
        ) {
            Button("Cancel", role: .cancel) {}
            Button("Mark all reviewed") {
                _ = session.markAllFramesReviewed(secondViewConfirmed: session.secondViewChecked)
            }
        } message: {
            Text(
                "This says you have been through every saved frame of this object and its points "
                    + "are right, including frames that were filled in for you. They count as "
                    + "reference truth from then on. Changing a frame's points afterwards puts "
                    + "that frame back to unreviewed.")
        }
    }

    private func handleStep(_ step: () -> AnnotationGuard?) {
        if step() != nil { showDiscardPrompt = true }
    }
}

// MARK: - Status

/// What went wrong, or failing that what to do next. One line, always on
/// screen, across the top of the views: a refused save used to report itself
/// at the foot of a scrolling column, where a save that failed looked like one
/// that had worked.
struct AnnotationStatusStrip: View {
    @ObservedObject var session: AnnotationSession

    var body: some View {
        HStack(alignment: .top, spacing: 6) {
            if let error = session.lastError {
                Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.red)
                Text(error).foregroundStyle(.red)
            } else if let next = session.nextStep {
                Image(systemName: "arrow.right.circle").foregroundStyle(.secondary)
                Text(next).foregroundStyle(.secondary)
            } else {
                Image(systemName: "checkmark.circle").foregroundStyle(.green)
                Text("Saved.").foregroundStyle(.secondary)
            }
        }.font(.caption).fixedSize(horizontal: false, vertical: true).frame(
            maxWidth: .infinity, alignment: .leading
        ).padding(.horizontal, 12).padding(.vertical, 6)
    }
}
