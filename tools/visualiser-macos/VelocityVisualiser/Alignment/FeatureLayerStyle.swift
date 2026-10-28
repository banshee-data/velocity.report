// FeatureLayerStyle.swift
// How the visualiser draws Gerardus's W4–W7 reference layers, all in the
// site frame and the debug-line layout ([x, y, z, r, g, b, a] per vertex):
//
//   BUILDINGS         each building part's outline, extruded as a wireframe
//                     from min_height to height, and its roof planes, filled
//                     translucent (triangles) and outlined, coloured by slope
//                     or by their part's roof:shape
//   LANE_MARKINGS     bright lines on the road, white or yellow
//   STREET_FURNITURE  poles as a vertical line with a small ring for their
//                     diameter at each end; signs as short thick segments
//   TREES             a trunk line, and the crown as rings of crown_radius_m
//                     at crown_base_m and height_m
//
// The proto carries no colours and no meshes (Gerardus ADR-0007): every
// triangle here is the client's, from a planar polygon.

import Foundation
import simd

/// How roof planes are coloured.
enum RoofColouring: String, CaseIterable, Sendable {
    /// By slope_deg: flat blue through yellow to steep red.
    case slope
    /// By the roof:shape of the plane's building part; grey where the part
    /// has none.
    case shape
}

extension ReferenceStyle {
    // MARK: Tags

    /// A tag's leading number, so "12 m" reads as 12; nil when absent or not
    /// a number. Simple 3D Buildings' heights are metres unless they say
    /// otherwise, and Gerardus writes plain metres.
    static func number(_ tags: [String: String], _ key: String) -> Float? {
        guard let raw = tags[key]?.trimmingCharacters(in: .whitespaces), !raw.isEmpty else {
            return nil
        }
        let prefix = raw.prefix { "0123456789.-+".contains($0) }
        guard let value = Float(prefix), value.isFinite else { return nil }
        return value
    }

    // MARK: Building parts

    /// A building part's rise above its outline: height less min_height, or
    /// the feature's height_m when the tags carry no height. Zero when
    /// neither says.
    static func partRise(_ part: Gerardus_Align_V1alpha1_LineFeature) -> Float {
        if let height = number(part.tags, "height") {
            return max(0, height - (number(part.tags, "min_height") ?? 0))
        }
        return max(0, part.heightM)
    }

    /// Each part's outline at its base (the ground plus min_height, as the
    /// contract places it), the same ring `partRise` higher, and an upright
    /// at every vertex: a wireframe prism. A part with no height is its
    /// outline alone.
    static func buildingPartVertices(_ parts: [Gerardus_Align_V1alpha1_LineFeature]) -> [Float] {
        let edges = buildingPartEdges(parts)
        return edges.eaves + edges.corners
    }

    /// The prisms' edges by kind: the eaves are the horizontal outlines at
    /// the base and the top of a part, the corners the uprights between them.
    static func buildingPartEdges(
        _ parts: [Gerardus_Align_V1alpha1_LineFeature]
    ) -> (eaves: [Float], corners: [Float]) {
        var eaves = [Float]()
        var corners = [Float]()
        for part in parts {
            let ring = vertices3(part)
            guard ring.count >= 2 else { continue }
            let colour = lineColour(.buildingPart)
            let rise = partRise(part)
            let top = SIMD3<Float>(0, 0, rise)
            appendRing(&eaves, ring, closed: true, colour: colour)
            guard rise > 0 else { continue }
            appendRing(&eaves, ring.map { $0 + top }, closed: true, colour: colour)
            for p in ring { appendLine(&corners, p, p + top, colour) }
        }
        return (eaves, corners)
    }

    /// A face on each wall of each part, base to top, as two triangles an
    /// edge. Solid in the vertices: the draw applies the walls' opacity. A
    /// part with no height, or an edge with no length, has none.
    static func wallFillVertices(_ parts: [Gerardus_Align_V1alpha1_LineFeature]) -> [Float] {
        var out = [Float]()
        for part in parts {
            var ring = vertices3(part)
            if ring.count > 3, ring[0] == ring[ring.count - 1] { ring.removeLast() }
            let rise = partRise(part)
            guard ring.count >= 3, rise > 0 else { continue }
            let colour = lineColour(.buildingPart)
            let top = SIMD3<Float>(0, 0, rise)
            for i in 0..<ring.count {
                let a = ring[i]
                let b = ring[(i + 1) % ring.count]
                guard a != b else { continue }
                for p in [a, b, b + top, a, b + top, a + top] { append(&out, p, colour) }
            }
        }
        return out
    }

    // MARK: Roof planes

    /// How translucent a roof plane's fill is until the operator says
    /// otherwise (BuildingStyle.roofs); its outline is opaque.
    static let roofFillAlpha: Float = 0.35

    /// A roof plane's colour by slope: 0° blue, 22.5° yellow, 45° and over red.
    static func roofSlopeColour(_ slopeDeg: Float?) -> SIMD4<Float> {
        guard let slopeDeg else { return lineColour(.roofPlane) }
        let flat = SIMD3<Float>(0.35, 0.65, 1.00)
        let mid = SIMD3<Float>(1.00, 0.85, 0.25)
        let steep = SIMD3<Float>(1.00, 0.30, 0.25)
        let t = min(max(slopeDeg / 45, 0), 1)
        let rgb = t < 0.5 ? flat + (mid - flat) * (t * 2) : mid + (steep - mid) * ((t - 0.5) * 2)
        return SIMD4(rgb, 1)
    }

    /// Simple 3D Buildings' standard roof shapes, each with its own colour.
    static let roofShapeColours: [String: SIMD4<Float>] = [
        "flat": SIMD4(0.35, 0.65, 1.00, 1), "gabled": SIMD4(1.00, 0.45, 0.30, 1),
        "hipped": SIMD4(1.00, 0.75, 0.20, 1), "pyramidal": SIMD4(0.85, 0.35, 0.85, 1),
        "skillion": SIMD4(0.30, 0.90, 0.75, 1), "half-hipped": SIMD4(1.00, 0.60, 0.55, 1),
        "gambrel": SIMD4(0.65, 0.50, 0.30, 1), "mansard": SIMD4(0.60, 0.45, 1.00, 1),
        "saltbox": SIMD4(0.95, 0.95, 0.50, 1), "dome": SIMD4(0.50, 0.85, 1.00, 1),
        "round": SIMD4(0.40, 0.75, 0.55, 1), "onion": SIMD4(1.00, 0.40, 0.65, 1),
        "cone": SIMD4(0.75, 0.75, 0.35, 1),
    ]

    /// The colour for a part's roof:shape; grey for none, `unknown` or a
    /// value outside the table.
    static func roofShapeColour(_ shape: String?) -> SIMD4<Float> {
        roofShapeColours[shape ?? ""] ?? SIMD4(0.62, 0.62, 0.66, 1)
    }

    /// The plane's colour, opaque, by `colouring`. `parts` maps a building
    /// part's feature_id to its tags, for the plane's `part` tag.
    static func roofColour(
        _ plane: Gerardus_Align_V1alpha1_LineFeature, parts: [String: [String: String]],
        colouring: RoofColouring
    ) -> SIMD4<Float> {
        switch colouring {
        case .slope: return roofSlopeColour(number(plane.tags, "slope_deg"))
        case .shape: return roofShapeColour(parts[plane.tags["part"] ?? ""]?["roof:shape"])
        }
    }

    /// Roof planes as triangles and outlines, at their z. The fill is solid
    /// in the vertices: the draw applies the roofs' opacity.
    static func roofPlaneVertices(
        _ planes: [Gerardus_Align_V1alpha1_LineFeature],
        parts: [Gerardus_Align_V1alpha1_LineFeature], colouring: RoofColouring
    ) -> (fills: [Float], lines: [Float]) {
        var partTags = [String: [String: String]]()
        for part in parts where !part.featureID.isEmpty { partTags[part.featureID] = part.tags }
        var fills = [Float]()
        var lines = [Float]()
        for plane in planes {
            let ring = vertices3(plane)
            guard ring.count >= 3 else { continue }
            let colour = roofColour(plane, parts: partTags, colouring: colouring)
            appendRing(&lines, ring, closed: true, colour: colour)
            for i in triangulate(ring) { append(&fills, ring[i], colour) }
        }
        return (fills, lines)
    }

    /// Indices into `ring`, three per triangle, filling a simple planar
    /// polygon by ear clipping in the plane it lies in. A closing vertex that
    /// repeats the first is ignored; a degenerate ring gives no triangles.
    static func triangulate(_ ring: [SIMD3<Float>]) -> [Int] {
        var n = ring.count
        if n > 3, ring[0] == ring[n - 1] { n -= 1 }
        guard n >= 3 else { return [] }
        // Newell's normal; drop its largest axis to project into 2D.
        var normal = SIMD3<Float>.zero
        for i in 0..<n {
            let a = ring[i]
            let b = ring[(i + 1) % n]
            normal += SIMD3(
                (a.y - b.y) * (a.z + b.z), (a.z - b.z) * (a.x + b.x), (a.x - b.x) * (a.y + b.y))
        }
        let m = abs(normal)
        guard max(m.x, m.y, m.z) > 1e-9 else { return [] }
        func flat(_ p: SIMD3<Float>) -> SIMD2<Float> {
            if m.z >= m.x && m.z >= m.y {
                return SIMD2(p.x, p.y) * (normal.z < 0 ? SIMD2(-1, 1) : SIMD2(1, 1))
            }
            if m.x >= m.y { return SIMD2(p.y, p.z) * (normal.x < 0 ? SIMD2(-1, 1) : SIMD2(1, 1)) }
            return SIMD2(p.z, p.x) * (normal.y < 0 ? SIMD2(-1, 1) : SIMD2(1, 1))
        }
        let p = ring.prefix(n).map(flat)
        func cross(_ o: SIMD2<Float>, _ a: SIMD2<Float>, _ b: SIMD2<Float>) -> Float {
            (a.x - o.x) * (b.y - o.y) - (a.y - o.y) * (b.x - o.x)
        }
        // The projection keeps the normal's sense, so the ring runs
        // anticlockwise in 2D and an ear turns left.
        var left = Array(0..<n)
        var out = [Int]()
        out.reserveCapacity((n - 2) * 3)
        var guardCount = 0
        while left.count > 3, guardCount < n * n {
            guardCount += 1
            var clipped = false
            for k in 0..<left.count {
                let i0 = left[(k + left.count - 1) % left.count]
                let i1 = left[k]
                let i2 = left[(k + 1) % left.count]
                guard cross(p[i0], p[i1], p[i2]) > 0 else { continue }
                let inside = left.contains { j in
                    guard j != i0, j != i1, j != i2 else { return false }
                    return cross(p[i0], p[i1], p[j]) >= 0 && cross(p[i1], p[i2], p[j]) >= 0
                        && cross(p[i2], p[i0], p[j]) >= 0
                }
                guard !inside else { continue }
                out += [i0, i1, i2]
                left.remove(at: k)
                clipped = true
                break
            }
            // A ring that crosses itself has no ear left: fan what remains.
            if !clipped { break }
        }
        for k in 1..<(left.count - 1) { out += [left[0], left[k], left[k + 1]] }
        return out
    }

    // MARK: Lane markings

    /// A marking's colour from its `colour` tag.
    static func laneMarkingColour(_ marking: Gerardus_Align_V1alpha1_LineFeature) -> SIMD4<Float> {
        switch marking.tags["colour"] {
        case "white": return SIMD4(1.00, 1.00, 1.00, 1.00)
        case "yellow": return SIMD4(1.00, 0.82, 0.00, 1.00)
        default: return lineColour(.laneMarking)
        }
    }

    /// Markings as lines just above the road, closed area markings closed.
    static func laneMarkingVertices(_ markings: [Gerardus_Align_V1alpha1_LineFeature]) -> [Float] {
        var out = [Float]()
        for marking in markings {
            let lift = SIMD3<Float>(0, 0, lineLift)
            appendRing(
                &out, vertices3(marking).map { $0 + lift }, closed: marking.closed,
                colour: laneMarkingColour(marking))
        }
        return out
    }

    // MARK: Street furniture

    /// A pole's diameter when it carries none.
    static let defaultPoleDiameterM: Float = 0.2
    /// Segments in a drawn ring.
    static let ringSegments = 16
    /// How far each extra copy of a sign's face is set off, in metres, so
    /// a one-pixel line reads thick.
    static let signThicknessM: Float = 0.04

    /// Poles base to top with a ring of their diameter at each end; signs
    /// bottom to top of their face, drawn five times over to read thick.
    /// Anything else in the layer is drawn as a plain line.
    static func streetFurnitureVertices(
        _ features: [Gerardus_Align_V1alpha1_LineFeature]
    ) -> [Float] {
        var out = [Float]()
        for feature in features {
            let points = vertices3(feature)
            guard points.count >= 2 else { continue }
            let colour = lineColour(feature.kind)
            switch feature.kind {
            case .pole:
                let base = points[0]
                let top = points[points.count - 1]
                let radius = (number(feature.tags, "diameter_m") ?? defaultPoleDiameterM) / 2
                appendLine(&out, base, top, colour)
                appendCircle(&out, centre: base, radius: radius, colour: colour)
                appendCircle(&out, centre: top, radius: radius, colour: colour)
            case .sign:
                let d = signThicknessM
                for offset in [
                    SIMD3<Float>.zero, SIMD3(d, 0, 0), SIMD3(-d, 0, 0), SIMD3(0, d, 0),
                    SIMD3(0, -d, 0),
                ] { appendRing(&out, points.map { $0 + offset }, closed: false, colour: colour) }
            default: appendRing(&out, points, closed: feature.closed, colour: colour)
            }
        }
        return out
    }

    // MARK: Trees

    /// A crown's radius when the tree carries none.
    static let defaultCrownRadiusM: Float = 2
    static let treeTrunkColour = SIMD4<Float>(0.70, 0.48, 0.28, 0.95)

    /// Where a tree's crown is drawn: its centre over the trunk, its radius,
    /// and the heights of its bottom and top rings in the site frame.
    struct Crown: Equatable {
        var centre: SIMD2<Float>
        var radius: Float
        var bottom: Float
        var top: Float
    }

    /// The crown from the tags, measured from the trunk's base: bottom at
    /// crown_base_m (else the trunk segment's top), top at height_m (else a
    /// crown as tall as it is wide). Nil without a trunk segment.
    static func crown(_ tree: Gerardus_Align_V1alpha1_LineFeature) -> Crown? {
        let points = vertices3(tree)
        guard let base = points.first, let trunkTop = points.last, points.count >= 2 else {
            return nil
        }
        let radius = number(tree.tags, "crown_radius_m") ?? defaultCrownRadiusM
        let bottom = number(tree.tags, "crown_base_m").map { base.z + $0 } ?? trunkTop.z
        let top = number(tree.tags, "height_m").map { base.z + $0 } ?? bottom + 2 * radius
        return Crown(
            centre: SIMD2(trunkTop.x, trunkTop.y), radius: max(radius, 0), bottom: bottom,
            top: max(top, bottom))
    }

    /// Each tree's trunk, base to crown base, and its crown as two rings.
    static func treeVertices(_ trees: [Gerardus_Align_V1alpha1_LineFeature]) -> [Float] {
        var out = [Float]()
        for tree in trees {
            let points = vertices3(tree)
            guard points.count >= 2, let crown = crown(tree) else { continue }
            appendRing(&out, points, closed: false, colour: treeTrunkColour)
            let colour = lineColour(.tree)
            for z in [crown.bottom, crown.top] {
                appendCircle(
                    &out, centre: SIMD3(crown.centre, z), radius: crown.radius, colour: colour)
            }
        }
        return out
    }

    // MARK: Helpers

    /// A feature's vertices as given, without the kind's lift.
    static func vertices3(_ feature: Gerardus_Align_V1alpha1_LineFeature) -> [SIMD3<Float>] {
        let count = feature.xyz.count / 3
        return (0..<count).map {
            SIMD3(feature.xyz[$0 * 3], feature.xyz[$0 * 3 + 1], feature.xyz[$0 * 3 + 2])
        }
    }

    private static func append(_ out: inout [Float], _ p: SIMD3<Float>, _ colour: SIMD4<Float>) {
        out.append(contentsOf: [p.x, p.y, p.z, colour.x, colour.y, colour.z, colour.w])
    }

    private static func appendLine(
        _ out: inout [Float], _ a: SIMD3<Float>, _ b: SIMD3<Float>, _ colour: SIMD4<Float>
    ) {
        append(&out, a, colour)
        append(&out, b, colour)
    }

    private static func appendRing(
        _ out: inout [Float], _ points: [SIMD3<Float>], closed: Bool, colour: SIMD4<Float>
    ) {
        guard points.count >= 2 else { return }
        for i in 0..<(points.count - 1) { appendLine(&out, points[i], points[i + 1], colour) }
        if closed, points.count > 2 {
            appendLine(&out, points[points.count - 1], points[0], colour)
        }
    }

    /// A horizontal circle of `ringSegments` segments.
    private static func appendCircle(
        _ out: inout [Float], centre: SIMD3<Float>, radius: Float, colour: SIMD4<Float>
    ) {
        guard radius > 0 else { return }
        let ring = (0..<ringSegments).map { i -> SIMD3<Float> in
            let a = Float(i) / Float(ringSegments) * 2 * .pi
            return centre + SIMD3(cos(a), sin(a), 0) * radius
        }
        appendRing(&out, ring, closed: true, colour: colour)
    }
}
