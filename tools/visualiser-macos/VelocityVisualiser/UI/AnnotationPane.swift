// AnnotationPane.swift
// The operator-facing point-cloud editing toolset.
//
// Three pieces: a lasso overlay that turns a drag into view-plane metres, the
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

/// Captures a lasso or rectangle drag and previews the candidates.
///
/// A drag is sampled into a polygon; holding shift adds, option subtracts, as
/// the session's mode resolution decides. The candidate count appears before
/// the gesture is committed, which is what the workflow requires.
struct LassoOverlay: View {
    @ObservedObject var session: AnnotationSession
    /// Which view this overlay belongs to. The second view is read-only: it
    /// exists to check a selection, not to make one.
    let basisStandard: OrthoViewBasis.Standard
    let viewport: OrthoViewport
    var editable: Bool = true

    @State private var strokePoints: [CGPoint] = []
    @State private var rectangleMode = false

    var body: some View {
        GeometryReader { _ in
            ZStack(alignment: .topLeading) {
                Color.clear.contentShape(Rectangle())

                selectedPointsLayer
                if strokePoints.count > 1 { strokeOutline }
                if let candidates = session.pendingCandidates, editable {
                    candidateBadge(candidates)
                }
            }.gesture(editable ? dragGesture : nil)
        }
    }

    // Selected points drawn over the render so the operator can see the mask
    // rather than inferring it from a count.
    private var selectedPointsLayer: some View {
        Canvas { context, _ in
            let basis = OrthoViewBasis(basisStandard)
            for index in session.history.current {
                guard let p = session.currentPoints.point(at: index) else { continue }
                let screen = viewport.screenPoint(from: basis.project(p))
                context.fill(
                    Path(ellipseIn: CGRect(x: screen.x - 2, y: screen.y - 2, width: 4, height: 4)),
                    with: .color(.orange))
            }
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
            Text("Return to accept, Esc to cancel").font(.caption2).foregroundStyle(.secondary)
        }.padding(6).background(.black.opacity(0.7), in: RoundedRectangle(cornerRadius: 4)).padding(
            8)
    }

    private var dragGesture: some Gesture {
        DragGesture(minimumDistance: 2).onChanged { value in
            if strokePoints.isEmpty {
                session.beginStroke()
                rectangleMode = NSEvent.modifierFlags.contains(.command)
            }
            if rectangleMode {
                // Command-drag is the quick axis-aligned rectangle.
                strokePoints = rectangleCorners(from: value.startLocation, to: value.location)
            } else {
                strokePoints.append(value.location)
            }
            previewCurrentStroke()
        }.onEnded { _ in
            guard !strokePoints.isEmpty else { return }
            session.selectionMode = SelectionMode.from(
                shiftHeld: NSEvent.modifierFlags.contains(.shift),
                optionHeld: NSEvent.modifierFlags.contains(.option))
            previewCurrentStroke()
            _ = session.commitSelection()
            strokePoints = []
        }
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
                session.resetSlabToSampleExtent()
            }

            Text("\(session.selectionCount) points selected").font(.caption)
            Text("Drag to lasso · ⌘ rectangle · ⇧ add · ⌥ subtract").font(.caption2)
                .foregroundStyle(.secondary)

            HStack {
                Button("Undo") { session.undo() }.disabled(!session.history.canUndo)
                Button("Redo") { session.redo() }.disabled(!session.history.canRedo)
                Button("Clear") { session.clearSelection() }.disabled(session.selectionCount == 0)
            }
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
                            session.slab = DepthSlab(minDepth: Float($0), maxDepth: slab.maxDepth)
                        }), in: Double(slab.minDepth - 20)...Double(slab.maxDepth))
                Slider(
                    value: Binding(
                        get: { Double(slab.maxDepth) },
                        set: {
                            session.slab = DepthSlab(minDepth: slab.minDepth, maxDepth: Float($0))
                        }), in: Double(slab.minDepth)...Double(slab.maxDepth + 20))
                Button("Reset to sample extent") { session.resetSlabToSampleExtent() }.font(
                    .caption)
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
