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
    /// The return a sphere stroke is centred on, fixed when the stroke begins.
    @State private var sphereCentre: simd_float3?
    /// Where the column brush was at the last drag event, in world metres.
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
                    })

                if showsGrid { gridLayer }
                selectedPointsLayer
                if strokePoints.count > 1 { strokeOutline }
                if session.pendingSphere != nil { sphereOutline }
                if let candidates = session.pendingCandidates, editable {
                    candidateBadge(candidates)
                } else if let strokeNote, editable {
                    noteBadge(strokeNote)
                }
            }
        }
    }

    // Selected points drawn over the render so the operator can see the mask
    // rather than inferring it from a count.
    //
    // One Path, filled once. A membership can run to thousands of points and a
    // fill per point makes the preview stutter during the very drag it is
    // meant to give feedback on.
    private var selectedPointsLayer: some View {
        Canvas { context, _ in
            var path = Path()
            for index in session.history.current {
                guard let p = session.currentPoints.point(at: index) else { continue }
                let screen = viewport.screenPoint(from: basis.project(p))
                path.addEllipse(in: CGRect(x: screen.x - 2, y: screen.y - 2, width: 4, height: 4))
            }
            context.fill(path, with: .color(.orange))
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
            guard let sphere = session.pendingSphere, metresPerPoint > 0 else { return }
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
        case .sphere:
            sphereChanged(value)
            if let sphere = session.pendingSphere { session.sphereRadius = sphere.radius }
        case .column: break
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
        sphereCentre = nil
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

    private func sphereChanged(_ value: ViewportStroke) {
        if sphereCentre == nil {
            // Twelve points of slop: near enough to mean "that return", far
            // enough to hit one without zooming in.
            guard
                let index = session.nearestPointIndex(
                    toViewPoint: viewport.worldPoint(from: value.startLocation),
                    maxViewDistance: metresPerPoint * 12),
                let centre = session.currentPoints.point(at: index)
            else {
                strokeNote = "No return under the cursor to centre a sphere on"
                return
            }
            session.beginStroke()
            sphereCentre = centre
            strokeNote = nil
        }
        guard let centre = sphereCentre else { return }
        let dragged = simd_distance(
            viewport.worldPoint(from: value.startLocation),
            viewport.worldPoint(from: value.location))
        let radius = BrushStroke.sphereRadius(dragMetres: dragged, remembered: session.sphereRadius)
        session.previewSelection(sphere: SelectionSphere(centre: centre, radius: radius))
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
    @ObservedObject var session: AnnotationSession

    @State private var newObjectClass = "car"
    @State private var newObjectSubtype = ""
    @State private var secondViewConfirmed = false
    @State private var showDiscardPrompt = false

    /// The seven selectable production labels. Research subtypes stay in
    /// their own field: a dataset export must not enable a reserved
    /// production enum value through the class picker.
    private let classes = ["car", "van", "truck", "bus", "motorcycle", "pedestrian", "cyclist"]

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                sourceSection
                Divider()
                displaySection
                Divider()
                objectSection
                Divider()
                selectionSection
                Divider()
                slabSection
                Divider()
                reviewSection
                if let error = session.lastError { errorBanner(error) }
            }.padding(12)
        }.frame(width: 300)
    }

    // MARK: Source

    private var sourceSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Source").font(.headline)
            Text(session.pack.manifest.datasetID).font(.caption.monospaced()).textSelection(
                .enabled)
            // Coverage is shown before any labelling: a foreground-only pack
            // cannot support whole-scene segmentation, and a mask saved in
            // ignorance of that reads as a stronger claim than it is.
            Text(session.pack.coverageCaveat).font(.caption).foregroundStyle(.secondary)
            if let sample = session.currentSample {
                Text(
                    "Sample \(sample.sampleID) of \(session.samples.count) · \(session.currentPoints.count) points"
                ).font(.caption)
            }
            HStack {
                Button("Previous") { handleStep { session.stepBackward() } }.disabled(
                    session.sampleIndex == 0)
                Button("Next") { handleStep { session.stepForward() } }.disabled(
                    session.sampleIndex >= session.samples.count - 1)
            }
            TextField("Operator name", text: $session.operatorName).textFieldStyle(.roundedBorder)
                .font(.caption)
        }
    }

    // MARK: Display

    private var displaySection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Display").font(.headline)
            // The recorder's own classes, in the main view's colours. What is
            // hidden cannot be selected, so switching the background off is
            // also how a lasso is kept from taking the wall behind a van.
            HStack(spacing: 4) {
                Toggle("Background", isOn: $session.visibility.background)
                Toggle("Foreground", isOn: $session.visibility.foreground)
                Toggle("Ground", isOn: $session.visibility.ground)
            }.toggleStyle(.button).controlSize(.small).disabled(
                !session.pack.manifest.hasClassification)
            if !session.pack.manifest.hasClassification {
                Text("This pack was recorded without point classes, so there is nothing to filter.")
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

    // MARK: Object

    private var objectSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Reference object").font(.headline)
            // Created independently of predicted tracks: a split track must
            // not split the reference vehicle.
            Picker("Class", selection: $newObjectClass) {
                ForEach(classes, id: \.self) { Text($0).tag($0) }
            }.font(.caption)
            TextField("Subtype (optional)", text: $newObjectSubtype).textFieldStyle(.roundedBorder)
                .font(.caption)
            Button("New object") {
                _ = session.createObject(
                    objectClass: newObjectClass,
                    subtype: newObjectSubtype.isEmpty ? nil : newObjectSubtype)
                secondViewConfirmed = false
            }

            if session.sidecar.objects.isEmpty {
                Text("No objects yet").font(.caption).foregroundStyle(.secondary)
            } else {
                ForEach(session.sidecar.objects) { object in objectRow(object) }
            }
        }
    }

    private func objectRow(_ object: AnnotationObject) -> some View {
        HStack(spacing: 6) {
            Image(
                systemName: session.activeObjectID == object.objectID
                    ? "largecircle.fill.circle" : "circle")
            VStack(alignment: .leading, spacing: 0) {
                Text(object.objectClass + (object.subtype.map { " · \($0)" } ?? "")).font(.caption)
                Text(object.objectID).font(.caption2).foregroundStyle(.secondary)
            }
            Spacer()
            Text(object.status.rawValue).font(.caption2).foregroundStyle(
                object.status == .reviewed ? .green : .secondary)
        }.contentShape(Rectangle()).onTapGesture {
            guard session.navigationGuard() == nil else {
                showDiscardPrompt = true
                return
            }
            session.activeObjectID = object.objectID
            secondViewConfirmed = false
        }
    }

    // MARK: Selection

    private var selectionSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Selection").font(.headline)
            Picker("View", selection: $session.viewStandard) {
                ForEach(OrthoViewBasis.Standard.allCases, id: \.self) { Text($0.label).tag($0) }
            }.font(.caption).onChange(of: session.viewStandard) { _, newValue in
                // Keep the confirming view on a genuinely different axis.
                session.secondViewStandard = OrthoViewBasis.secondView(for: newValue)
                // The depth axis has changed, so a slab set along the old one
                // means nothing along the new.
                session.unpinSlab()
            }

            Picker("Tool", selection: $session.tool) {
                ForEach(SelectionTool.allCases) { Text($0.label).tag($0) }
            }.pickerStyle(.segmented).labelsHidden().onChange(of: session.tool) { _, _ in
                // Changing tool mid-stroke would apply one tool's gesture
                // under another's rule.
                session.cancelStroke()
            }

            Text("\(session.selectionCount) points selected").font(.caption)
            Text(session.tool.hint).font(.caption2).foregroundStyle(.secondary)

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

    private var sphereControls: some View {
        HStack {
            Text("Radius").font(.caption)
            Slider(
                value: $session.sphereRadius,
                in: SelectionSphere.minimumRadius...SelectionSphere.maximumRadius)
            Text(String(format: "%.2f m", session.sphereRadius)).font(.caption.monospacedDigit())
                .frame(width: 52, alignment: .trailing)
        }.help("A click uses this radius. A drag sets it. [ and ] step it.")
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
            Text("Review").font(.headline)
            Picker("Visibility", selection: $session.maskVisibility) {
                ForEach(Visibility.allCases, id: \.self) { Text($0.label).tag($0) }
            }.font(.caption)
            Picker("Completeness", selection: $session.maskCompleteness) {
                ForEach(MaskCompleteness.allCases, id: \.self) { Text($0.label).tag($0) }
            }.font(.caption)

            // A membership seen from one angle has not been inspected for
            // contamination, so review is gated on the second view.
            Toggle("Checked in \(session.secondViewStandard.label)", isOn: $secondViewConfirmed)
                .font(.caption)

            HStack {
                Button("Save mask") { _ = session.save() }
                Button("Mark reviewed") {
                    _ = session.markObjectReviewed(secondViewConfirmed: secondViewConfirmed)
                }.disabled(!secondViewConfirmed)
            }
            if session.navigationGuard() != nil {
                Text("Unsaved changes").font(.caption2).foregroundStyle(.orange)
            }
            Text("Revision \(session.sidecar.revision)").font(.caption2).foregroundStyle(.secondary)
        }.alert("Unsaved membership", isPresented: $showDiscardPrompt) {
            Button("Keep editing", role: .cancel) {}
            Button("Discard and reload", role: .destructive) { session.reload() }
        } message: {
            Text("This sample has unsaved changes. Save it, or discard them, before moving on.")
        }
    }

    private func errorBanner(_ message: String) -> some View {
        Text(message).font(.caption).foregroundStyle(.red).fixedSize(
            horizontal: false, vertical: true)
    }

    private func handleStep(_ step: () -> AnnotationGuard?) {
        if step() != nil { showDiscardPrompt = true }
    }
}
