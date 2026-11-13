// RoadSurfaceStyle.swift
// What the road surface preview draws, as vertex arrays the renderer takes:
// the filled surface (plain or shaded by support), its wireframe, the
// centreline from the profile, the boundary rings coloured by source, and the
// selected patch's outline.
//
// Layout is the debug-line one: [x, y, z, r, g, b, a] per vertex; triangles
// three vertices, lines two.
//
// Honesty rules kept here: a MISSING vertex has no triangle, so the fill has
// an open gap there; the gap's edge is drawn red in support mode. An inferred
// boundary is drawn dashed in orange, never solid like a surveyed one. Lines
// sit `lift` above the surface so they draw over it, and that lift is a
// drawing offset, not a height.

import Foundation
import simd

/// How the surface fill is coloured.
enum RoadShading: String, CaseIterable, Sendable {
    case plain
    case support

    var label: String { self == .plain ? "Plain" : "Support" }
}

/// What the road view shows. The class-2 points, City kerbs and City
/// centrelines are reference layers drawn by the existing reference overlay;
/// their switches live here so one control sets them.
struct RoadViewOptions: Equatable, Sendable {
    var surface = true
    var shading: RoadShading = .support
    var wireframe = false
    /// The centreline of each patch, from its height profile.
    var centreline = true
    var boundary = true
    var cityKerbs = true
    var class2Points = true
    /// The City's own centrelines, from the reference layer.
    var cityCentrelines = false
}

struct RoadOverlayVertices: Sendable, Equatable {
    var fill: [Float] = []
    var wire: [Float] = []
    var centre: [Float] = []
    var boundary: [Float] = []
    var gaps: [Float] = []
    var selection: [Float] = []

    var fillVertexCount: Int { fill.count / 7 }
    var wireVertexCount: Int { wire.count / 7 }
    var centreVertexCount: Int { centre.count / 7 }
    var boundaryVertexCount: Int { boundary.count / 7 }
    var gapVertexCount: Int { gaps.count / 7 }
    var selectionVertexCount: Int { selection.count / 7 }
    var isEmpty: Bool { fill.isEmpty && wire.isEmpty && centre.isEmpty && boundary.isEmpty }
}

/// What the renderer holds of an overlay: its vertex counts.
struct RoadOverlayCounts: Equatable, Sendable {
    var fillVertexCount = 0
    var wireVertexCount = 0
    var centreVertexCount = 0
    var boundaryVertexCount = 0
    var gapVertexCount = 0
    var selectionVertexCount = 0

    init() {}
    init(_ v: RoadOverlayVertices) {
        fillVertexCount = v.fillVertexCount
        wireVertexCount = v.wireVertexCount
        centreVertexCount = v.centreVertexCount
        boundaryVertexCount = v.boundaryVertexCount
        gapVertexCount = v.gapVertexCount
        selectionVertexCount = v.selectionVertexCount
    }
}

enum RoadSurfaceStyle {
    /// Drawing offset above the surface for lines, m.
    static let lift: Float = 0.04

    static func colour(_ support: RoadSupportClass) -> SIMD4<Float> {
        switch support {
        case .observed: return SIMD4(0.16, 0.62, 0.52, 0.80)
        case .interpolated: return SIMD4(0.96, 0.74, 0.18, 0.80)
        case .missing: return SIMD4(0.90, 0.15, 0.15, 1)
        case .unspecified: return SIMD4(0.5, 0.5, 0.5, 0.8)
        }
    }

    static func colour(_ source: RoadBoundarySourceClass) -> SIMD4<Float> {
        switch source {
        case .kerbPair: return SIMD4(0.20, 0.75, 1.00, 1)
        case .cityKerb: return SIMD4(0.45, 0.60, 1.00, 1)
        case .inferredWidth: return SIMD4(1.00, 0.55, 0.10, 1)
        case .authored: return SIMD4(0.85, 0.85, 0.85, 1)
        case .unspecified: return SIMD4(0.6, 0.6, 0.6, 1)
        }
    }

    static let plainColour = SIMD4<Float>(0.42, 0.43, 0.46, 0.85)
    static let selectionColour = SIMD4<Float>(1, 1, 1, 1)
    static let centreColour = SIMD4<Float>(0.95, 0.95, 0.35, 1)
    static let wireColour = SIMD4<Float>(0.9, 0.9, 0.9, 0.35)

    /// Everything to draw for `patches`. `selected` is outlined.
    static func vertices(
        patches: [RoadPatch], options: RoadViewOptions, selected: String? = nil
    ) -> RoadOverlayVertices {
        var out = RoadOverlayVertices()
        for patch in patches {
            if options.surface { appendFill(patch, shading: options.shading, to: &out.fill) }
            if options.wireframe { appendWire(patch, to: &out.wire) }
            if options.centreline { appendCentre(patch, to: &out.centre) }
            if options.boundary { appendBoundary(patch, to: &out.boundary) }
            if options.surface, options.shading == .support { appendGaps(patch, to: &out.gaps) }
            if patch.patchID == selected { appendSelection(patch, to: &out.selection) }
        }
        return out
    }

    @inline(__always) private static func push(
        _ out: inout [Float], _ p: SIMD3<Float>, _ c: SIMD4<Float>
    ) { out.append(contentsOf: [p.x, p.y, p.z, c.x, c.y, c.z, c.w]) }

    private static func appendFill(_ patch: RoadPatch, shading: RoadShading, to out: inout [Float])
    {
        out.reserveCapacity(out.count + patch.indices.count * 7)
        for t in 0..<patch.triangleCount {
            let ia = Int(patch.indices[3 * t])
            let ib = Int(patch.indices[3 * t + 1])
            let ic = Int(patch.indices[3 * t + 2])
            let colour: SIMD4<Float>
            switch shading {
            case .plain:
                // A little shade from the slope, so a bend and a crown read.
                let n = patch.normals[ia]
                let k = n.z.isFinite ? 0.75 + 0.25 * max(0, min(1, n.z)) : 1
                colour = SIMD4(
                    plainColour.x * k, plainColour.y * k, plainColour.z * k, plainColour.w)
            case .support:
                // The weakest of the three: one interpolated corner marks the triangle.
                let weakest = [ia, ib, ic].map { patch.support[$0] }.max {
                    $0.rawValue < $1.rawValue
                }
                colour = RoadSurfaceStyle.colour(weakest ?? .unspecified)
            }
            push(&out, patch.vertices[ia], colour)
            push(&out, patch.vertices[ib], colour)
            push(&out, patch.vertices[ic], colour)
        }
    }

    private static func appendWire(_ patch: RoadPatch, to out: inout [Float]) {
        let up = SIMD3<Float>(0, 0, lift)
        for t in 0..<patch.triangleCount {
            let v = (0..<3).map { patch.vertices[Int(patch.indices[3 * t + $0])] + up }
            for (a, b) in [(0, 1), (1, 2), (2, 0)] {
                push(&out, v[a], wireColour)
                push(&out, v[b], wireColour)
            }
        }
    }

    /// The centreline joins the profile's supported stations. At a missing
    /// station the line breaks: it is not drawn across the gap.
    private static func appendCentre(_ patch: RoadPatch, to out: inout [Float]) {
        let up = SIMD3<Float>(0, 0, lift)
        var previous: SIMD3<Float>?
        for station in patch.profile {
            if station.isMissing {
                previous = nil
                continue
            }
            let p = station.position + up
            if let q = previous {
                push(&out, q, centreColour)
                push(&out, p, centreColour)
            }
            previous = p
        }
    }

    /// Rings by edge source. Inferred edges are dashed. An edge with a NaN
    /// height is not drawn: the boundary has none to show there.
    private static func appendBoundary(_ patch: RoadPatch, to out: inout [Float]) {
        let up = SIMD3<Float>(0, 0, lift)
        for ring in patch.rings {
            let n = ring.points.count
            guard n >= 2 else { continue }
            for i in 0..<n {
                let a = ring.points[i]
                let b = ring.points[(i + 1) % n]
                guard a.z.isFinite, b.z.isFinite else { continue }
                let source = i < ring.edgeSource.count ? ring.edgeSource[i] : .unspecified
                let colour = RoadSurfaceStyle.colour(source)
                if source.isInferred {
                    appendDashed(a + up, b + up, colour, to: &out)
                } else {
                    push(&out, a + up, colour)
                    push(&out, b + up, colour)
                }
            }
        }
    }

    static func appendDashed(
        _ a: SIMD3<Float>, _ b: SIMD3<Float>, _ colour: SIMD4<Float>, dash: Float = 0.6,
        to out: inout [Float]
    ) {
        let length = simd_length(b - a)
        guard length > 1e-4 else { return }
        var s: Float = 0
        while s < length {
            let e = min(s + dash, length)
            push(&out, a + (b - a) * (s / length), colour)
            push(&out, a + (b - a) * (e / length), colour)
            s += 2 * dash
        }
    }

    private static func appendSelection(_ patch: RoadPatch, to out: inout [Float]) {
        let up = SIMD3<Float>(0, 0, lift * 3)
        for ring in patch.rings {
            let n = ring.points.count
            for i in 0..<n {
                let a = ring.points[i]
                let b = ring.points[(i + 1) % n]
                guard a.z.isFinite, b.z.isFinite else { continue }
                push(&out, a + up, selectionColour)
                push(&out, b + up, selectionColour)
            }
        }
    }

    /// Open edges inside the patch: mesh edges used by one triangle that do not
    /// lie near a ring. They outline missing ground, drawn red.
    static func openInteriorEdges(
        _ patch: RoadPatch, ringTolerance: Float = 1.0
    ) -> [(SIMD3<Float>, SIMD3<Float>)] {
        guard patch.count(of: .missing) > 0 else { return [] }
        var uses: [UInt64: Int] = [:]
        func key(_ a: UInt32, _ b: UInt32) -> UInt64 { UInt64(min(a, b)) << 32 | UInt64(max(a, b)) }
        for t in 0..<patch.triangleCount {
            let i = (0..<3).map { patch.indices[3 * t + $0] }
            for (a, b) in [(0, 1), (1, 2), (2, 0)] { uses[key(i[a], i[b]), default: 0] += 1 }
        }
        var out: [(SIMD3<Float>, SIMD3<Float>)] = []
        for (k, count) in uses where count == 1 {
            let a = patch.vertices[Int(k >> 32)]
            let b = patch.vertices[Int(k & 0xffff_ffff)]
            let mid = (SIMD2(a.x, a.y) + SIMD2(b.x, b.y)) / 2
            if patch.rings.contains(where: { distance(mid, to: $0) <= ringTolerance }) { continue }
            out.append((a, b))
        }
        return out
    }

    private static func appendGaps(_ patch: RoadPatch, to out: inout [Float]) {
        let up = SIMD3<Float>(0, 0, lift)
        for (a, b) in openInteriorEdges(patch) {
            push(&out, a + up, colour(.missing))
            push(&out, b + up, colour(.missing))
        }
    }

    static func distance(_ p: SIMD2<Float>, to ring: RoadRing) -> Float {
        var best = Float.infinity
        let n = ring.points.count
        for i in 0..<n {
            let a = ring.points[i]
            let b = ring.points[(i + 1) % n]
            guard a.x.isFinite, b.x.isFinite else { continue }
            let ab = SIMD2(b.x - a.x, b.y - a.y)
            let ap = p - SIMD2(a.x, a.y)
            let len2 = simd_dot(ab, ab)
            let t = len2 > 0 ? max(0, min(1, simd_dot(ap, ab) / len2)) : 0
            best = min(best, simd_length(ap - ab * t))
        }
        return best
    }

    // MARK: - Picking

    /// The patch a ray meets first, with the hit point. Triangles only: an open
    /// gap or a hole is not hit.
    static func pick(
        patches: [RoadPatch], origin: SIMD3<Float>, direction: SIMD3<Float>
    ) -> (patchID: String, point: SIMD3<Float>, distance: Float)? {
        var best: (String, SIMD3<Float>, Float)?
        for patch in patches {
            for t in 0..<patch.triangleCount {
                let a = patch.vertices[Int(patch.indices[3 * t])]
                let b = patch.vertices[Int(patch.indices[3 * t + 1])]
                let c = patch.vertices[Int(patch.indices[3 * t + 2])]
                guard let d = rayTriangle(origin, direction, a, b, c) else { continue }
                if best == nil || d < best!.2 { best = (patch.patchID, origin + direction * d, d) }
            }
        }
        return best.map { ($0.0, $0.1, $0.2) }
    }

    /// Möller–Trumbore, both faces.
    static func rayTriangle(
        _ o: SIMD3<Float>, _ d: SIMD3<Float>, _ a: SIMD3<Float>, _ b: SIMD3<Float>,
        _ c: SIMD3<Float>
    ) -> Float? {
        let e1 = b - a
        let e2 = c - a
        let p = simd_cross(d, e2)
        let det = simd_dot(e1, p)
        guard abs(det) > 1e-9 else { return nil }
        let inv = 1 / det
        let s = o - a
        let u = simd_dot(s, p) * inv
        guard u >= 0, u <= 1 else { return nil }
        let q = simd_cross(s, e1)
        let v = simd_dot(d, q) * inv
        guard v >= 0, u + v <= 1 else { return nil }
        let t = simd_dot(e2, q) * inv
        return t > 0 ? t : nil
    }
}
