// LassoOverlay.swift
// The drawing surface over an orthographic view: the stroke in progress, the
// points it would take, and what is already saved.
//
// It converts screen points to metres and hands those to the session; it never
// decides membership itself. That keeps the selection rule in one tested place
// instead of split between a gesture handler and an engine.

import AppKit
import SwiftUI
import simd

/// Captures a selection stroke and previews the candidates, and pans and
/// zooms the view it lies over.
///
/// A lasso drag is sampled into a polygon; holding shift adds, option
/// subtracts, as the session's mode resolution decides. The candidate count
/// appears while the stroke is being made, which is what the workflow
/// requires.
struct LassoOverlay: View {
    @ObservedObject var session: AnnotationSession
    /// Observed separately from the session, which does not republish when the
    /// brush moves: see BrushHover.swift. Taken from the session rather than
    /// passed in, so that call sites are unchanged.
    @ObservedObject private var hover: BrushHover
    /// Which view this overlay belongs to. The second view is read-only: it
    /// exists to check a selection, not to make one.
    let basisStandard: OrthoViewBasis.Standard
    let viewport: OrthoViewport
    var editable: Bool = true

    init(
        session: AnnotationSession, basisStandard: OrthoViewBasis.Standard, viewport: OrthoViewport,
        editable: Bool = true
    ) {
        self.session = session
        self._hover = ObservedObject(wrappedValue: session.hover)
        self.basisStandard = basisStandard
        self.viewport = viewport
        self.editable = editable
    }

    @State private var strokePoints: [CGPoint] = []
    @State private var rectangleMode = false
    /// Where a brush was at the last drag event, in world metres.
    @State private var lastBrushPosition: simd_float2?
    @State private var paintedCells: Set<ColumnCell> = []
    @State private var strokeNote: String?

    private var basis: OrthoViewBasis { session.basis(basisStandard) }

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
                if session.pendingSphere != nil || hover.sphere != nil { sphereOutline }
                if let candidates = session.pendingCandidates, editable {
                    candidateBadge(candidates)
                } else if session.carried != nil, editable {
                    carriedBadge
                } else if let sphere = hover.sphere, editable {
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
        // Saved masks are drawn over the cloud, so hiding a label state in the
        // point canvas alone left the mask on screen and the toggle looked
        // broken. These are the two sets the label states describe; the live
        // selection and the proposal overlays are not saved masks and are left
        // alone. Filtered here rather than in the closure, which is not on the
        // main actor.
        let hidesALabelState = !session.visibility.showsEveryLabelState
        let labelSets = session.effectiveLabelSets
        let visibility = session.effectiveVisibility
        func shown(_ index: Int) -> Bool {
            PointVisibility.showsLabelState(index, under: visibility, labels: labelSets)
        }
        let others =
            hidesALabelState
            ? session.otherMasks.map {
                (name: $0.name, objectClass: $0.objectClass, indices: $0.indices.filter(shown))
            } : session.otherMasks
        let saved = hidesALabelState ? session.savedSelection.filter(shown) : session.savedSelection
        let current = session.history.current
        let activeClass = session.activeObject?.objectClass
        let activeName = session.activeObjectName
        let carried = session.carriedIndices
        let hovered = hover.indices
        let proposed = session.proposedIndices
        let proposal = session.proposalIndices
        let viewport = viewport
        let basis = basis

        return Canvas { context, _ in
            func dots(_ indices: some Sequence<Int>, size: CGFloat) -> Path {
                var path = Path()
                for index in indices {
                    guard let p = points.point(at: index) else { continue }
                    guard basis.shows(p) else { continue }
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
                    guard basis.shows(p) else { continue }
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
            guard let sphere = session.pendingSphere ?? hover.sphere, metresPerPoint > 0 else {
                return
            }
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
            Text("\(hover.indices.count) under the brush").font(.caption.bold())
            Text(String(format: "centre z %.2f m · radius %.2f m", sphere.centre.z, sphere.radius))
                .font(.caption2).foregroundStyle(.secondary)
            if hover.depthOffset != 0 {
                Text(String(format: "moved %+.1f m in depth", hover.depthOffset)).font(.caption2)
                    .foregroundStyle(.secondary)
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

    // Keys the editing view takes while it has the focus. What each one does
    // is ViewportKey.meaning, so that the order — a carried proposal claims
    // all four arrows, and without one they step frames and move the ground —
    // is one readable function rather than a switch inside a view.
    private func handleKey(_ key: ViewportKey) -> Bool {
        guard editable else { return false }
        switch key.meaning(carrying: session.carried != nil) {
        case .nudgeCarried(let right, let up, let coarse):
            let step = coarse ? AnnotationSession.coarseNudgeStep : AnnotationSession.nudgeStep
            session.nudgeCarried(right: Float(right) * step, up: Float(up) * step)
        case .acceptCarried: session.acceptCarried()
        case .dismissCarried: session.dismissCarried()
        case .stepFrame(let forward):
            // Refused when the sample has unsaved changes; the status line
            // says so, and a beep is what says the key was heard at all.
            if (forward ? session.stepForward() : session.stepBackward()) != nil { NSSound.beep() }
        case .moveGround(let steps, let coarse): session.adjustGroundZ(steps: steps, coarse: coarse)
        case .toggleVoxel(let k): session.toggleVoxel(k)
        case .pass: return false
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
