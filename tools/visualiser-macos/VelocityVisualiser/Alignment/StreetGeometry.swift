// StreetGeometry.swift
// Geometry for the street painter, in the site frame's east/north plane, in
// metres. Nothing here talks to a server or draws.
//
// Ported from the web street editor (web/src/lib/streets/geometry.ts): the
// snap that previews what Gerardus does on save (vertices within 0.5 m of a
// kerb, a centreline, a box wall or another mark land on it), a row of
// parked-car spaces along a kerb, and a lane's direction of travel. The web
// works in a local equirectangular frame about the site; the visualiser has
// Gerardus's site frame itself, east-north-up at the site's L16 cell, so no
// projection is needed.
//
// Also here: the ground under the cursor (GroundSurface, from the ground mesh
// heightfield) and the top-down plan view the painter draws in (PlanView),
// which maps the Metal view's pixels to the site frame and back.

import CoreGraphics
import Foundation
import simd

/// A point in the site frame's horizontal plane: east, north, metres.
typealias PlanPoint = SIMD2<Double>

/// A segment between two plan points.
struct PlanSegment: Equatable, Sendable {
    var a: PlanPoint
    var b: PlanPoint
}

enum StreetGeometry {
    /// How close a vertex must be to a target to snap to it: Gerardus's own
    /// snapping distance on EditStreetUse.
    static let snapMetres = 0.5

    /// A line or ring the painter snaps to.
    struct SnapTarget: Equatable, Sendable {
        enum Layer: String, Sendable {
            case kerb
            case centreline
            case wall
            case mark
        }

        /// The feature it came from, so a mark being edited can leave itself out.
        var id: String
        var layer: Layer
        var points: [PlanPoint]
        var closed: Bool
    }

    struct Snap: Equatable, Sendable {
        /// What it snapped to: a vertex, a point along an edge, or nothing.
        enum On: Equatable, Sendable {
            case vertex
            case edge
            case none
        }

        var point: PlanPoint
        var on: On
        var distance: Double
        var target: SnapTarget?

        var snapped: Bool { on != .none }
    }

    // MARK: Distances and snapping

    static func distance(_ a: PlanPoint, _ b: PlanPoint) -> Double { simd_length(a - b) }

    /// The nearest point to p on the segment a–b, its fraction t along it, and
    /// the distance.
    static func closestOnSegment(
        _ p: PlanPoint, _ a: PlanPoint, _ b: PlanPoint
    ) -> (point: PlanPoint, t: Double, distance: Double) {
        let d = b - a
        let length2 = simd_length_squared(d)
        let t = length2 == 0 ? 0 : min(1, max(0, simd_dot(p - a, d) / length2))
        let point = a + d * t
        return (point, t, distance(p, point))
    }

    /// The edges of a polyline, with the closing edge of a closed ring.
    static func edges(_ points: [PlanPoint], closed: Bool) -> [PlanSegment] {
        var out = [PlanSegment]()
        guard points.count >= 2 else { return out }
        for i in 0..<(points.count - 1) { out.append(PlanSegment(a: points[i], b: points[i + 1])) }
        if closed, points.count > 2 {
            out.append(PlanSegment(a: points[points.count - 1], b: points[0]))
        }
        return out
    }

    /// Snaps p to the nearest target within tolerance. A vertex within reach
    /// wins over a nearer edge, so marks meet end to end rather than nearly;
    /// otherwise the nearest point along any edge. Targets whose id is
    /// `excluding` are left out: a mark being moved does not snap to itself.
    static func snap(
        _ p: PlanPoint, to targets: [SnapTarget], tolerance: Double = snapMetres,
        excluding excluded: String? = nil
    ) -> Snap {
        var best = Snap(point: p, on: .none, distance: .infinity)
        for target in targets where target.id != excluded {
            for v in target.points {
                let d = distance(p, v)
                if d <= tolerance, d < best.distance {
                    best = Snap(point: v, on: .vertex, distance: d, target: target)
                }
            }
        }
        if best.on == .vertex { return best }
        for target in targets where target.id != excluded {
            for edge in edges(target.points, closed: target.closed) {
                let c = closestOnSegment(p, edge.a, edge.b)
                if c.distance <= tolerance, c.distance < best.distance {
                    best = Snap(point: c.point, on: .edge, distance: c.distance, target: target)
                }
            }
        }
        return best.on == .none ? Snap(point: p, on: .none, distance: 0) : best
    }

    /// The nearest point on any of the targets, however far; nil when there
    /// are none.
    static func nearestOn(
        _ p: PlanPoint, _ targets: [SnapTarget]
    ) -> (point: PlanPoint, distance: Double)? {
        var best: (point: PlanPoint, distance: Double)?
        for target in targets {
            if target.points.count == 1 {
                let d = distance(p, target.points[0])
                if best == nil || d < best!.distance { best = (target.points[0], d) }
                continue
            }
            for edge in edges(target.points, closed: target.closed) {
                let c = closestOnSegment(p, edge.a, edge.b)
                if best == nil || c.distance < best!.distance { best = (c.point, c.distance) }
            }
        }
        return best
    }

    /// The distance from p to a polyline or ring.
    static func distance(from p: PlanPoint, toPolyline points: [PlanPoint], closed: Bool) -> Double
    {
        if points.count == 1 { return distance(p, points[0]) }
        return edges(points, closed: closed).map { closestOnSegment(p, $0.a, $0.b).distance }.min()
            ?? .infinity
    }

    /// Whether p lies inside a simple polygon (even-odd rule).
    static func contains(_ polygon: [PlanPoint], _ p: PlanPoint) -> Bool {
        guard polygon.count >= 3 else { return false }
        var inside = false
        var j = polygon.count - 1
        for i in 0..<polygon.count {
            let a = polygon[i]
            let b = polygon[j]
            if (a.y > p.y) != (b.y > p.y), p.x < (b.x - a.x) * (p.y - a.y) / (b.y - a.y) + a.x {
                inside.toggle()
            }
            j = i
        }
        return inside
    }

    /// The signed area of a ring, positive when anticlockwise seen from above.
    static func signedArea(_ ring: [PlanPoint]) -> Double {
        guard ring.count >= 3 else { return 0 }
        var sum = 0.0
        for i in 0..<ring.count {
            let a = ring[i]
            let b = ring[(i + 1) % ring.count]
            sum += a.x * b.y - b.x * a.y
        }
        return sum / 2
    }

    // MARK: Inserting a vertex

    /// Where a vertex goes when the operator clicks near an edge of a mark:
    /// the nearest point on its nearest edge, and the index the new vertex
    /// takes (the edge's end). Nil when no edge lies within `tolerance`.
    static func insertion(
        at p: PlanPoint, into points: [PlanPoint], closed: Bool, tolerance: Double
    ) -> (index: Int, point: PlanPoint)? {
        var best: (index: Int, point: PlanPoint, distance: Double)?
        for (i, edge) in edges(points, closed: closed).enumerated() {
            let c = closestOnSegment(p, edge.a, edge.b)
            guard c.distance <= tolerance else { continue }
            if best == nil || c.distance < best!.distance { best = (i + 1, c.point, c.distance) }
        }
        return best.map { ($0.index, $0.point) }
    }

    // MARK: A row of spaces along the kerb

    /// +1 when p is left of the line a→b (seen from above), −1 when right.
    /// On the line counts as left.
    static func sideOf(_ a: PlanPoint, _ b: PlanPoint, _ p: PlanPoint) -> Double {
        let d = b - a
        let cross = d.x * (p.y - a.y) - d.y * (p.x - a.x)
        return cross < 0 ? -1 : 1
    }

    /// The side of the kerb span a→b the street is on: towards the nearest
    /// centreline from the span's midpoint. Left when there is no centreline.
    static func streetSide(_ a: PlanPoint, _ b: PlanPoint, centrelines: [SnapTarget]) -> Double {
        let mid = (a + b) / 2
        guard let near = nearestOn(mid, centrelines) else { return 1 }
        return sideOf(a, b, near.point)
    }

    struct SpaceRow: Equatable, Sendable {
        /// The row's outline: a, b, then b and a moved `depth` to the side.
        var ring: [PlanPoint]
        /// The lines between neighbouring spaces, from the kerb outwards.
        var dividers: [PlanSegment]
        /// Each space's length along the kerb, metres.
        var spaceLength: Double
    }

    /// A run of `count` equal spaces along the kerb from a to b, `depth`
    /// metres deep on `side` (+1 left of a→b, −1 right). Nil when the span
    /// has no length, the count is below one or the depth is not positive.
    static func rowOfSpaces(
        _ a: PlanPoint, _ b: PlanPoint, count: Int, depth: Double, side: Double
    ) -> SpaceRow? {
        let d = b - a
        let length = simd_length(d)
        guard length >= 1e-6, count >= 1, depth > 0 else { return nil }
        // The left normal of (e, n) is (−n, e).
        let offset = PlanPoint(-d.y, d.x) / length * depth * (side < 0 ? -1 : 1)
        var dividers = [PlanSegment]()
        for i in 1..<max(count, 1) {
            let p = a + d * (Double(i) / Double(count))
            dividers.append(PlanSegment(a: p, b: p + offset))
        }
        return SpaceRow(
            ring: [a, b, b + offset, a + offset], dividers: dividers,
            spaceLength: length / Double(count))
    }

    /// The dividers of a row drawn as one four-cornered outline: the first
    /// edge is the kerb side and the third the far side, as `rowOfSpaces`
    /// makes it. Any other outline, or a count below two, has none.
    static func rowDividers(_ ring: [PlanPoint], count: Int) -> [PlanSegment] {
        guard ring.count == 4, count >= 2 else { return [] }
        return (1..<count).map { i in
            let f = Double(i) / Double(count)
            return PlanSegment(
                a: ring[0] + (ring[1] - ring[0]) * f, b: ring[3] + (ring[2] - ring[3]) * f)
        }
    }

    // MARK: A lane's direction

    /// [0, 360).
    static func wrap360(_ degrees: Double) -> Double {
        let wrapped = degrees.truncatingRemainder(dividingBy: 360)
        return wrapped < 0 ? wrapped + 360 : wrapped
    }

    /// The bearing from a to b, degrees clockwise from north (the site's +Y).
    static func bearing(_ a: PlanPoint, _ b: PlanPoint) -> Double {
        let d = b - a
        return wrap360(atan2(d.x, d.y) * 180 / .pi)
    }

    /// The direction of travel of a lane drawn first vertex to last: the
    /// bearing of the whole run. Nil for a lane with fewer than two distinct
    /// ends.
    static func laneDirection(_ points: [PlanPoint]) -> Double? {
        guard points.count >= 2, let a = points.first, let b = points.last, distance(a, b) >= 1e-6
        else { return nil }
        return bearing(a, b)
    }

    /// Arrows along a lane every `spacing` metres, starting half a spacing
    /// in, each with the bearing of the segment it sits on. A lane shorter
    /// than one spacing gets one arrow at its middle.
    static func laneArrows(
        _ points: [PlanPoint], spacing: Double = 8
    ) -> [(at: PlanPoint, bearing: Double)] {
        var segments = [(a: PlanPoint, b: PlanPoint, length: Double, start: Double)]()
        var total = 0.0
        if points.count >= 2 {
            for i in 0..<(points.count - 1) {
                let length = distance(points[i], points[i + 1])
                if length < 1e-9 { continue }
                segments.append((points[i], points[i + 1], length, total))
                total += length
            }
        }
        guard let last = segments.last, spacing > 0 else { return [] }
        var marks = [Double]()
        if total < spacing {
            marks.append(total / 2)
        } else {
            var s = spacing / 2
            while s < total {
                marks.append(s)
                s += spacing
            }
        }
        return marks.map { s in
            let seg = segments.first { s <= $0.start + $0.length } ?? last
            let f = (s - seg.start) / seg.length
            return (seg.a + (seg.b - seg.a) * f, bearing(seg.a, seg.b))
        }
    }

    // MARK: Filling a polygon

    /// Triangles filling a simple polygon, as indices into it, three per
    /// triangle, by ear clipping. A ring of either winding is filled; a
    /// self-intersecting one is filled as far as ears can be found.
    static func triangulate(_ polygon: [PlanPoint]) -> [Int] {
        var ring = Array(polygon.indices)
        guard ring.count >= 3 else { return [] }
        if signedArea(polygon) < 0 { ring.reverse() }
        var out = [Int]()
        func isConvex(_ a: PlanPoint, _ b: PlanPoint, _ c: PlanPoint) -> Bool {
            let ab = b - a
            let bc = c - b
            return ab.x * bc.y - ab.y * bc.x > 1e-12
        }
        func inTriangle(_ p: PlanPoint, _ a: PlanPoint, _ b: PlanPoint, _ c: PlanPoint) -> Bool {
            func side(_ u: PlanPoint, _ v: PlanPoint) -> Double {
                (v.x - u.x) * (p.y - u.y) - (v.y - u.y) * (p.x - u.x)
            }
            return side(a, b) >= 0 && side(b, c) >= 0 && side(c, a) >= 0
        }
        var guardCount = 0
        while ring.count > 3, guardCount < polygon.count * polygon.count {
            guardCount += 1
            var clipped = false
            for i in 0..<ring.count {
                let ia = ring[(i + ring.count - 1) % ring.count]
                let ib = ring[i]
                let ic = ring[(i + 1) % ring.count]
                let (a, b, c) = (polygon[ia], polygon[ib], polygon[ic])
                guard isConvex(a, b, c) else { continue }
                let blocked = ring.contains { j in
                    j != ia && j != ib && j != ic && inTriangle(polygon[j], a, b, c)
                }
                if blocked { continue }
                out.append(contentsOf: [ia, ib, ic])
                ring.remove(at: i)
                clipped = true
                break
            }
            if !clipped { break }
        }
        if ring.count == 3 { out.append(contentsOf: ring) }
        return out
    }
}

// MARK: - The ground under the cursor

/// The ground surface, from the GROUND_MESH heightfields: what a click in the
/// plan view lands on. Cell centres are sampled bilinearly; a cell with some
/// corners missing takes the mean of those it has.
struct GroundSurface: Sendable {
    struct Field: Sendable {
        /// The south-west cell centre, site frame.
        var origin: PlanPoint
        var cellM: Double
        var nx: Int
        var ny: Int
        /// Row-major from the south-west: z[j * nx + i]; NaN where no ground.
        var z: [Float]
    }

    var fields: [Field] = []

    init(fields: [Field] = []) { self.fields = fields }

    init(heightfields: [Gerardus_Align_V1alpha1_Heightfield]) {
        fields = heightfields.compactMap { h in
            let nx = Int(h.nx)
            let ny = Int(h.ny)
            guard nx > 0, ny > 0, h.cellM > 0, h.z.count >= nx * ny else { return nil }
            return Field(
                origin: PlanPoint(h.origin.x, h.origin.y), cellM: Double(h.cellM), nx: nx, ny: ny,
                z: h.z)
        }
    }

    var isEmpty: Bool { fields.isEmpty }

    /// The ground height at p, or nil where no field holds ground there.
    func height(at p: PlanPoint) -> Double? {
        for field in fields {
            let u = (p.x - field.origin.x) / field.cellM
            let v = (p.y - field.origin.y) / field.cellM
            // Half a cell beyond the outer centres still reads the edge cells.
            guard u >= -0.5, v >= -0.5, u <= Double(field.nx) - 0.5, v <= Double(field.ny) - 0.5
            else { continue }
            let i0 = min(max(Int(floor(u)), 0), max(field.nx - 2, 0))
            let j0 = min(max(Int(floor(v)), 0), max(field.ny - 2, 0))
            let i1 = min(i0 + 1, field.nx - 1)
            let j1 = min(j0 + 1, field.ny - 1)
            let fu = min(max(u - Double(i0), 0), 1)
            let fv = min(max(v - Double(j0), 0), 1)
            let corners: [(z: Float, w: Double)] = [
                (field.z[j0 * field.nx + i0], (1 - fu) * (1 - fv)),
                (field.z[j0 * field.nx + i1], fu * (1 - fv)),
                (field.z[j1 * field.nx + i0], (1 - fu) * fv),
                (field.z[j1 * field.nx + i1], fu * fv),
            ]
            let held = corners.filter { $0.z.isFinite }
            guard !held.isEmpty else { continue }
            if held.count == 4 { return held.reduce(0) { $0 + Double($1.z) * $1.w } }
            return held.reduce(0) { $0 + Double($1.z) } / Double(held.count)
        }
        return nil
    }

    /// Where a ray from `origin` along `direction` (site frame) first meets
    /// the ground, or nil if it meets none within `maxDistance`. Marched in
    /// steps of half the finest cell, then bisected to a millimetre.
    func intersect(
        origin: SIMD3<Double>, direction: SIMD3<Double>, maxDistance: Double = 1000
    ) -> SIMD3<Double>? {
        let length = simd_length(direction)
        guard length > 0, !fields.isEmpty else { return nil }
        let dir = direction / length
        // Straight down: the ground under the point, wherever it is.
        if abs(dir.x) < 1e-9, abs(dir.y) < 1e-9 {
            guard dir.z < 0, let z = height(at: PlanPoint(origin.x, origin.y)), z <= origin.z else {
                return nil
            }
            return SIMD3(origin.x, origin.y, z)
        }
        let step = max(0.05, (fields.map(\.cellM).min() ?? 1) / 2)
        func above(_ t: Double) -> Double? {
            let p = origin + dir * t
            return height(at: PlanPoint(p.x, p.y)).map { p.z - $0 }
        }
        var t0 = 0.0
        var h0 = above(t0)
        var t = step
        while t <= maxDistance {
            let h = above(t)
            if let h, let previous = h0, previous >= 0, h <= 0 {
                var lo = t0
                var hi = t
                while hi - lo > 1e-3 {
                    let mid = (lo + hi) / 2
                    if let m = above(mid), m > 0 { lo = mid } else { hi = mid }
                }
                return origin + dir * hi
            }
            t0 = t
            h0 = h
            t += step
        }
        return nil
    }
}

// MARK: - The plan view

/// The painter's top-down orthographic view over the site: north up, east to
/// the right, looking straight down the site frame's −Z. Rotation is locked;
/// the operator pans and zooms.
///
/// The visualiser draws in the sensor frame, with the reference placed by the
/// inverse of the pose, so the camera is built in the site frame and carried
/// into the sensor frame by the same inverse. Because the view looks straight
/// down, a pixel maps to a site-frame (east, north) without any depth: a
/// click lands on the ground under it, whatever the ground's height.
struct PlanView: Equatable, Sendable {
    /// The site point at the view's centre.
    var centre: PlanPoint
    /// The height the camera looks at: the ground near the centre.
    var groundZ: Double
    /// Half the view's height in metres; its width follows the aspect.
    var halfHeight: Double

    static let eyeHeight: Float = 300
    static let minHalfHeight = 2.0
    static let maxHalfHeight = 400.0
    static let defaultHalfHeight = 30.0

    init(centre: PlanPoint, groundZ: Double, halfHeight: Double = defaultHalfHeight) {
        self.centre = centre
        self.groundZ = groundZ
        self.halfHeight = min(max(halfHeight, Self.minHalfHeight), Self.maxHalfHeight)
    }

    /// The camera, in the sensor frame the visualiser draws in, for a sensor
    /// at `pose`.
    func camera(pose: SitePose, aspectRatio: Float) -> Camera {
        let toSensor = pose.inverseMatrix
        func sensor(_ p: SIMD3<Double>) -> simd_float3 {
            let q = toSensor * SIMD4(p, 1)
            return simd_float3(Float(q.x), Float(q.y), Float(q.z))
        }
        let ground = SIMD3(centre.x, centre.y, groundZ)
        let north = pose.rotationMatrix.transpose * SIMD3(0, 1, 0)
        var camera = Camera()
        camera.target = sensor(ground)
        camera.position = sensor(ground + SIMD3(0, 0, Double(Self.eyeHeight)))
        camera.up = simd_float3(Float(north.x), Float(north.y), Float(north.z))
        camera.projection = .orthographic(halfHeight: Float(halfHeight))
        camera.aspectRatio = aspectRatio
        camera.nearPlane = 0.01
        camera.farPlane = Self.eyeHeight * 2
        return camera
    }

    /// Metres per point of the view, vertically and horizontally alike.
    func metresPerPoint(viewHeight: CGFloat) -> Double {
        viewHeight > 0 ? 2 * halfHeight / Double(viewHeight) : 0
    }

    /// The site (east, north) under a point of the view. View points have
    /// their origin at the bottom left, as an unflipped NSView's do.
    func sitePoint(atView point: CGPoint, size: CGSize) -> PlanPoint {
        let mpp = metresPerPoint(viewHeight: size.height)
        return centre
            + PlanPoint(
                Double(point.x - size.width / 2) * mpp, Double(point.y - size.height / 2) * mpp)
    }

    /// The point of the view a site (east, north) lies under.
    func viewPoint(ofSite p: PlanPoint, size: CGSize) -> CGPoint {
        let mpp = metresPerPoint(viewHeight: size.height)
        guard mpp > 0 else { return .zero }
        let d = (p - centre) / mpp
        return CGPoint(x: size.width / 2 + CGFloat(d.x), y: size.height / 2 + CGFloat(d.y))
    }

    /// The ray through a point of the view, in the site frame: straight down
    /// from the camera's height.
    func ray(
        atView point: CGPoint, size: CGSize
    ) -> (origin: SIMD3<Double>, direction: SIMD3<Double>) {
        let p = sitePoint(atView: point, size: size)
        return (SIMD3(p.x, p.y, groundZ + Double(Self.eyeHeight)), SIMD3(0, 0, -1))
    }

    /// Drags the ground with the pointer: what was under it stays under it.
    mutating func pan(byViewDelta dx: CGFloat, _ dy: CGFloat, size: CGSize) {
        let mpp = metresPerPoint(viewHeight: size.height)
        centre -= PlanPoint(Double(dx), Double(dy)) * mpp
    }

    /// Zooms by `factor` (above 1 zooms out) about a point of the view, which
    /// stays over the same ground.
    mutating func zoom(by factor: Double, about point: CGPoint, size: CGSize) {
        guard factor > 0, factor.isFinite else { return }
        let anchor = sitePoint(atView: point, size: size)
        let next = min(max(halfHeight * factor, Self.minHalfHeight), Self.maxHalfHeight)
        let scale = next / halfHeight
        centre = anchor + (centre - anchor) * scale
        halfHeight = next
    }
}
