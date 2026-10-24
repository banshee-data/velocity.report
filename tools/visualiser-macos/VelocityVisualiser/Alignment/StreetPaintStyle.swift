// StreetPaintStyle.swift
// What the scene draws for the street painter, already coloured, in the site
// frame: the marks styled by kind, the draft being drawn, the snap preview
// and the selection's handles. The renderer draws it through the
// reference's model matrix, like everything else from Gerardus.
//
// Lines in Metal are a pixel wide, so anything that must read as wide (a
// driveway's span, a lane's width, a selection, a problem's halo) is drawn as
// triangles. Sizes meant to stay the same on screen, such as handles, are
// given in points and scaled by the view's metres per point.

import Foundation
import simd

/// Vertex data for the street painter's overlay, in the debug-line layout:
/// [x, y, z, r, g, b, a] per vertex.
struct StreetOverlayVertices: Equatable, Sendable {
    /// Triangles, three vertices each: fills, bands and handles.
    var fills: [Float] = []
    /// Line segments, two vertices each.
    var lines: [Float] = []

    var fillVertexCount: Int { fills.count / 7 }
    var lineVertexCount: Int { lines.count / 7 }
    var isEmpty: Bool { fills.isEmpty && lines.isEmpty }
}

/// What the overlay shows besides the marks.
struct StreetPaintScene: Sendable {
    var marks: [StreetMark] = []
    var selectedKey: String?
    /// Feature ids with a problem from the last save.
    var flagged: Set<String> = []
    /// The vertices placed so far for the tool, and where the cursor would
    /// place the next.
    var tool: StreetPaintTool = .select
    var drawing: [SIMD3<Double>] = []
    var cursor: SIMD3<Double>?
    var cursorSnapped = false
    /// A row of spaces previewed between the two vertices of the row tool.
    var rowPreview: StreetGeometry.SpaceRow?
    var rowPreviewZ: Double = 0
    /// While a vertex is dragged: which, and where it is now.
    var dragKey: String?
    var dragIndex: Int?
    /// Handles and drafts are shown only while painting.
    var painting = false
    var metresPerPoint: Double = 0.05
}

/// The painter's tools.
enum StreetPaintTool: String, CaseIterable, Identifiable, Sendable {
    case select
    case surface
    case lane
    case space
    case row
    case driveway

    var id: String { rawValue }

    var label: String {
        switch self {
        case .select: return "Select"
        case .surface: return "Surface"
        case .lane: return "Lane"
        case .space: return "Space"
        case .row: return "Row"
        case .driveway: return "Driveway"
        }
    }

    var symbol: String {
        switch self {
        case .select: return "cursorarrow"
        case .surface: return "square.dashed.inset.filled"
        case .lane: return "arrow.up.forward"
        case .space: return "parkingsign"
        case .row: return "rectangle.split.3x1"
        case .driveway: return "road.lanes"
        }
    }

    var hint: String {
        switch self {
        case .select:
            return
                "Click a mark to select it. Drag a vertex to move it; drag an edge to add one; right-click a vertex to remove it. Drag empty ground to pan."
        case .surface: return "Click the corners of the surface; Return or a double-click finishes."
        case .lane:
            return
                "Click along the lane in its direction of travel; Return or a double-click finishes."
        case .space: return "Click the corners of the space; Return or a double-click finishes."
        case .row:
            return "Click two points along the kerb: the row runs between them, on the street side."
        case .driveway: return "Click the two ends of the kerb cut."
        }
    }

    /// The kind a drawing tool makes; nil for Select.
    var kind: StreetUseKind? {
        switch self {
        case .select: return nil
        case .surface: return .surface
        case .lane: return .travelLane
        case .space, .row: return .parkingSpace
        case .driveway: return .driveway
        }
    }

    /// Whether the tool finishes by itself after two clicks.
    var finishesAtTwo: Bool { self == .row || self == .driveway }
}

enum StreetPaintStyle {
    /// Drawn this far above the vertices, over the reference's draped lines
    /// (lifted 0.10 m). Display only.
    static let lift: Double = 0.15
    static let fillAlpha: Float = 0.30
    static let driveway = SIMD4<Float>(0.96, 0.45, 0.71, 0.90)
    static let problem = SIMD4<Float>(0.94, 0.27, 0.27, 0.50)
    static let selection = SIMD4<Float>(1, 1, 1, 0.55)
    static let handle = SIMD4<Float>(1, 1, 1, 1)
    static let handleDragged = SIMD4<Float>(0.13, 0.77, 0.37, 1)
    static let draft = SIMD4<Float>(1, 1, 1, 0.95)
    static let snapped = SIMD4<Float>(0.13, 0.77, 0.37, 1)
    /// A driveway's span, metres across.
    static let drivewayWidthM: Double = 0.8
    /// Point sizes, scaled by the view's metres per point.
    static let handleHalfPoints: Double = 4
    static let selectionPoints: Double = 3
    static let problemPoints: Double = 8
    static let cursorRadiusPoints: Double = 7
    static let arrowSpacingM: Double = 8
    static let arrowSizeM: Double = 1.2

    static func surfaceColour(_ surface: String?) -> SIMD4<Float> {
        switch surface {
        case "carriageway": return SIMD4(0.55, 0.57, 0.62, 1)
        case "sidewalk": return SIMD4(0.90, 0.84, 0.70, 1)
        case "verge": return SIMD4(0.35, 0.78, 0.38, 1)
        case "apron": return SIMD4(0.90, 0.60, 0.32, 1)
        case "median": return SIMD4(0.72, 0.52, 0.90, 1)
        default: return SIMD4(0.75, 0.75, 0.75, 1)
        }
    }

    /// The web street editor's colours, so a mark reads the same in both.
    static func spaceColour(_ type: String?) -> SIMD4<Float> {
        switch type {
        case "metered": return SIMD4(0.23, 0.51, 0.96, 1)
        case "permit": return SIMD4(0.66, 0.33, 0.97, 1)
        case "loading": return SIMD4(0.92, 0.70, 0.03, 1)
        case "disabled": return SIMD4(0.02, 0.71, 0.83, 1)
        case "free": return SIMD4(0.06, 0.73, 0.51, 1)
        default: return SIMD4(0.58, 0.64, 0.72, 1)
        }
    }

    static func laneColour(_ type: String?) -> SIMD4<Float> {
        switch type {
        case "bus": return SIMD4(0.94, 0.27, 0.27, 1)
        case "bike": return SIMD4(0.13, 0.77, 0.37, 1)
        case "turn": return SIMD4(0.98, 0.45, 0.09, 1)
        default: return SIMD4(0.90, 0.91, 0.92, 1)
        }
    }

    static func colour(_ mark: StreetMark) -> SIMD4<Float> {
        switch mark.kind {
        case .surface: return surfaceColour(mark.tags[StreetUseTags.surface])
        case .travelLane: return laneColour(mark.tags[StreetUseTags.laneType])
        case .parkingSpace: return spaceColour(mark.tags[StreetUseTags.spaceType])
        case .driveway: return driveway
        }
    }

    // MARK: Building

    static func vertices(_ scene: StreetPaintScene) -> StreetOverlayVertices {
        var out = Builder()
        let mpp = max(scene.metresPerPoint, 1e-4)
        // Surfaces first, so lanes, spaces and driveways draw over them.
        let ordered =
            scene.marks.filter { $0.kind == .surface } + scene.marks.filter { $0.kind != .surface }
        for mark in ordered {
            let flagged = !mark.featureID.isEmpty && scene.flagged.contains(mark.featureID)
            if flagged { out.outlineBand(mark, width: problemPoints * mpp, colour: problem) }
            appendMark(&out, mark)
            if scene.painting, mark.key == scene.selectedKey {
                out.outlineBand(mark, width: selectionPoints * mpp, colour: selection)
                for (i, p) in mark.points.enumerated() {
                    let dragged = mark.key == scene.dragKey && i == scene.dragIndex
                    out.square(
                        p, half: handleHalfPoints * mpp, colour: dragged ? handleDragged : handle)
                }
            }
        }
        if scene.painting { appendDraft(&out, scene, mpp: mpp) }
        return out.vertices
    }

    private static func appendMark(_ out: inout Builder, _ mark: StreetMark) {
        let colour = colour(mark)
        let points = mark.points
        guard points.count >= 2 else { return }
        switch mark.kind {
        case .surface:
            out.fill(points, colour: SIMD4(colour.x, colour.y, colour.z, fillAlpha))
            out.polyline(points, closed: true, colour: SIMD4(colour.x, colour.y, colour.z, 0.9))
        case .parkingSpace:
            out.fill(points, colour: SIMD4(colour.x, colour.y, colour.z, 0.12))
            out.polyline(points, closed: true, colour: colour)
            let count = Int(mark.tags[StreetUseTags.count] ?? "") ?? 1
            let z = points.map(\.z).reduce(0, +) / Double(points.count)
            for divider in StreetGeometry.rowDividers(mark.plan, count: count) {
                out.segment(SIMD3(divider.a, z), SIMD3(divider.b, z), colour: colour)
            }
        case .travelLane:
            if let width = Double(mark.tags[StreetUseTags.widthM] ?? ""), width > 0 {
                out.band(points, width: width, colour: SIMD4(colour.x, colour.y, colour.z, 0.16))
            }
            out.polyline(points, closed: false, colour: colour)
            let z = points.map(\.z).reduce(0, +) / Double(points.count)
            for arrow in StreetGeometry.laneArrows(mark.plan, spacing: arrowSpacingM) {
                out.chevron(at: SIMD3(arrow.at, z), bearing: arrow.bearing, colour: colour)
            }
        case .driveway:
            out.band(points, width: drivewayWidthM, colour: colour)
            out.polyline(points, closed: false, colour: colour)
        }
    }

    private static func appendDraft(_ out: inout Builder, _ scene: StreetPaintScene, mpp: Double) {
        guard scene.tool != .select else { return }
        if let row = scene.rowPreview {
            let z = scene.rowPreviewZ
            out.fill(row.ring.map { SIMD3($0, z) }, colour: SIMD4(1, 1, 1, 0.10))
            out.polyline(row.ring.map { SIMD3($0, z) }, closed: true, colour: draft)
            for divider in row.dividers {
                out.segment(SIMD3(divider.a, z), SIMD3(divider.b, z), colour: draft)
            }
        } else {
            let points = scene.drawing + (scene.cursor.map { [$0] } ?? [])
            let closing = scene.tool.kind?.closed == true && points.count >= 3
            out.polyline(points, closed: closing, colour: draft)
        }
        for p in scene.drawing { out.square(p, half: handleHalfPoints * mpp * 0.75, colour: draft) }
        if let cursor = scene.cursor {
            if scene.cursorSnapped {
                out.ring(cursor, radius: cursorRadiusPoints * mpp, colour: snapped)
                out.square(cursor, half: 2 * mpp, colour: snapped)
            } else {
                let h = 4 * mpp
                out.segment(cursor - SIMD3(h, 0, 0), cursor + SIMD3(h, 0, 0), colour: draft)
                out.segment(cursor - SIMD3(0, h, 0), cursor + SIMD3(0, h, 0), colour: draft)
            }
        }
    }

    /// Accumulates triangles and segments, lifted above the ground.
    struct Builder {
        var vertices = StreetOverlayVertices()

        private func lifted(_ p: SIMD3<Double>) -> SIMD3<Float> {
            SIMD3(Float(p.x), Float(p.y), Float(p.z + StreetPaintStyle.lift))
        }

        private mutating func add(_ p: SIMD3<Double>, _ c: SIMD4<Float>, to lines: Bool) {
            let q = lifted(p)
            let v = [q.x, q.y, q.z, c.x, c.y, c.z, c.w]
            if lines {
                vertices.lines.append(contentsOf: v)
            } else {
                vertices.fills.append(contentsOf: v)
            }
        }

        mutating func segment(_ a: SIMD3<Double>, _ b: SIMD3<Double>, colour: SIMD4<Float>) {
            add(a, colour, to: true)
            add(b, colour, to: true)
        }

        mutating func triangle(
            _ a: SIMD3<Double>, _ b: SIMD3<Double>, _ c: SIMD3<Double>, colour: SIMD4<Float>
        ) {
            add(a, colour, to: false)
            add(b, colour, to: false)
            add(c, colour, to: false)
        }

        mutating func polyline(_ points: [SIMD3<Double>], closed: Bool, colour: SIMD4<Float>) {
            guard points.count >= 2 else { return }
            for i in 0..<(points.count - 1) { segment(points[i], points[i + 1], colour: colour) }
            if closed, points.count > 2 {
                segment(points[points.count - 1], points[0], colour: colour)
            }
        }

        /// A polygon filled by ear clipping.
        mutating func fill(_ points: [SIMD3<Double>], colour: SIMD4<Float>) {
            let indices = StreetGeometry.triangulate(points.map { PlanPoint($0.x, $0.y) })
            for t in stride(from: 0, to: indices.count - 2, by: 3) {
                triangle(
                    points[indices[t]], points[indices[t + 1]], points[indices[t + 2]],
                    colour: colour)
            }
        }

        /// Each segment of a polyline as a quad `width` metres across.
        mutating func band(
            _ points: [SIMD3<Double>], width: Double, colour: SIMD4<Float>, closed: Bool = false
        ) {
            guard points.count >= 2 else { return }
            var pairs = (0..<(points.count - 1)).map { (points[$0], points[$0 + 1]) }
            if closed, points.count > 2 { pairs.append((points[points.count - 1], points[0])) }
            for (a, b) in pairs {
                let d = PlanPoint(b.x - a.x, b.y - a.y)
                let length = simd_length(d)
                guard length > 1e-9 else { continue }
                let n = PlanPoint(-d.y, d.x) / length * (width / 2)
                let o = SIMD3(n, 0)
                triangle(a - o, a + o, b + o, colour: colour)
                triangle(a - o, b + o, b - o, colour: colour)
            }
        }

        mutating func outlineBand(_ mark: StreetMark, width: Double, colour: SIMD4<Float>) {
            band(mark.points, width: width, colour: colour, closed: mark.kind.closed)
        }

        mutating func square(_ p: SIMD3<Double>, half: Double, colour: SIMD4<Float>) {
            let a = p + SIMD3(-half, -half, 0)
            let b = p + SIMD3(half, -half, 0)
            let c = p + SIMD3(half, half, 0)
            let d = p + SIMD3(-half, half, 0)
            triangle(a, b, c, colour: colour)
            triangle(a, c, d, colour: colour)
        }

        mutating func ring(_ p: SIMD3<Double>, radius: Double, colour: SIMD4<Float>) {
            let n = 20
            for i in 0..<n {
                let a0 = Double(i) / Double(n) * 2 * .pi
                let a1 = Double(i + 1) / Double(n) * 2 * .pi
                segment(
                    p + SIMD3(cos(a0), sin(a0), 0) * radius,
                    p + SIMD3(cos(a1), sin(a1), 0) * radius, colour: colour)
            }
        }

        /// An arrowhead pointing along `bearing` (degrees clockwise from north).
        mutating func chevron(at p: SIMD3<Double>, bearing: Double, colour: SIMD4<Float>) {
            let t = bearing * .pi / 180
            let size = StreetPaintStyle.arrowSizeM
            func back(_ angle: Double) -> SIMD3<Double> {
                p - SIMD3(sin(t + angle), cos(t + angle), 0) * size
            }
            segment(back(0.5), p, colour: colour)
            segment(p, back(-0.5), colour: colour)
        }
    }
}
