// PointSelection.swift
// Canonical-index point selection for reference annotation.
//
// Every operation here works on a pack's canonical point arrays, never on GPU
// buffer positions after a rendering filter. That distinction is the whole
// point: a display may decimate, cull or reorder for drawing, and a membership
// index recorded against the drawn order would be meaningless the next time
// the view changed.
//
// Selection is deliberately through-slab, not hidden-surface picking. An
// orthographic lasso plus a depth range selects every canonical point inside
// both, including points behind the ones the operator can see. That is an
// honest, explainable rule; perspective front-surface painting needs a
// depth/ID buffer and is a separate increment.

import Foundation
import simd

// MARK: - Orthographic view basis

/// A fixed orthographic viewing basis for selection.
///
/// Selection maths never uses the render camera's perspective matrix. A
/// perspective projection makes the same screen polygon mean a different world
/// volume at different depths, so a lasso would silently select more of the
/// scene far away than near. An orthographic basis keeps the rule the operator
/// sees on screen — "inside this outline, within this depth range" — exactly
/// the rule applied to the arrays.
struct OrthoViewBasis: Equatable {
    /// Standard viewing directions. `top` looks down the world -Z axis, so
    /// the view plane is the ground plane and depth is height; `front` and
    /// `side` look along the horizontal axes, so depth is a horizontal range.
    enum Standard: String, CaseIterable, Equatable {
        case top
        case front
        case back
        case side
        case farSide

        /// The four horizontal looks, in the order they are stacked: the two
        /// along Y, then the two along X. Each is the sensor looking outward,
        /// so between them they show every side of an object without the
        /// operator orbiting anything.
        static let elevations: [Standard] = [.front, .back, .side, .farSide]

        var label: String {
            switch self {
            case .top: return "Top (X-Y)"
            case .front: return "Front (X-Z)"
            case .back: return "Back (X-Z)"
            case .side: return "Side (Y-Z)"
            case .farSide: return "Far side (Y-Z)"
            }
        }

        /// What the depth axis means in this view, for the slab control's label.
        var depthAxisLabel: String {
            switch self {
            case .top: return "Height (Z)"
            case .front, .back: return "Depth (Y)"
            case .side, .farSide: return "Depth (X)"
            }
        }
    }

    /// View-plane horizontal axis, unit length.
    var right: simd_float3
    /// View-plane vertical axis, unit length.
    var up: simd_float3
    /// Depth axis, unit length; depth increases away from the viewer.
    var forward: simd_float3
    /// World point mapping to view-plane origin.
    var origin: simd_float3
    /// True when the view shows only what is in front of it.
    ///
    /// Without this an elevation draws the whole scene, so the half behind the
    /// sensor lands on top of the half in front and the two views along one
    /// axis are the same picture mirrored. With it the four elevations are
    /// four half-spaces, each a 180 degree sector of azimuth, and between them
    /// they partition the scene instead of drawing it twice.
    var facingOnly = false

    init(right: simd_float3, up: simd_float3, forward: simd_float3, origin: simd_float3 = .zero) {
        self.right = simd_normalize(right)
        self.up = simd_normalize(up)
        self.forward = simd_normalize(forward)
        self.origin = origin
    }

    init(_ standard: Standard, origin: simd_float3 = .zero) {
        self.init(standard: standard, origin: origin)
    }

    private init(standard: Standard, origin: simd_float3) {
        switch standard {
        case .top:
            // Looking down: screen right is world +X, screen up is world +Y,
            // depth runs downward so a lower point is "further away".
            self.init(
                right: simd_float3(1, 0, 0), up: simd_float3(0, 1, 0),
                forward: simd_float3(0, 0, -1), origin: origin)
        case .front:
            // Looking along +Y: screen right is world +X, screen up is world +Z.
            self.init(
                right: simd_float3(1, 0, 0), up: simd_float3(0, 0, 1),
                forward: simd_float3(0, 1, 0), origin: origin)
        case .back:
            // Looking along -Y, from the other side of the scene. Screen right
            // is world -X, so the view is the front one turned around rather
            // than mirrored: a car driving right in Front drives left here.
            self.init(
                right: simd_float3(-1, 0, 0), up: simd_float3(0, 0, 1),
                forward: simd_float3(0, -1, 0), origin: origin)
        case .side:
            // Looking along -X: screen right is world +Y, screen up is world +Z.
            self.init(
                right: simd_float3(0, 1, 0), up: simd_float3(0, 0, 1),
                forward: simd_float3(-1, 0, 0), origin: origin)
        case .farSide:
            // Looking along +X: screen right is world -Y.
            self.init(
                right: simd_float3(0, -1, 0), up: simd_float3(0, 0, 1),
                forward: simd_float3(1, 0, 0), origin: origin)
        }
        // The top view looks down on everything; the elevations each take
        // their own half.
        facingOnly = standard != .top
    }

    /// Whether this view shows a point at all, before any class filter or
    /// depth slab. A point exactly on the plane belongs to the view looking at
    /// it, so that the boundary is claimed once rather than by neither.
    func shows(_ p: simd_float3) -> Bool { !facingOnly || depth(p) >= 0 }

    /// Projects a world point onto the view plane, in metres.
    func project(_ p: simd_float3) -> simd_float2 {
        let d = p - origin
        return simd_float2(simd_dot(d, right), simd_dot(d, up))
    }

    /// Signed distance along the depth axis, in metres.
    func depth(_ p: simd_float3) -> Float { simd_dot(p - origin, forward) }

    /// A basis looking at the same origin from a different standard direction,
    /// for the second-view check the workflow requires. Returning a different
    /// standard rather than an arbitrary rotation keeps the confirming view
    /// one an operator can reason about.
    static func secondView(for standard: Standard) -> Standard {
        switch standard {
        case .top: return .front
        case .front: return .side
        case .side: return .top
        // The elevations added later confirm against the top view, which is
        // the one an operator reads a position in.
        case .back, .farSide: return .top
        }
    }
}

// MARK: - Selection shapes

/// A closed polygon in view-plane metres.
struct SelectionPolygon: Equatable {
    var vertices: [simd_float2]

    init(vertices: [simd_float2]) { self.vertices = vertices }

    /// An axis-aligned rectangle, the quick case the operator reaches for most.
    init(rectFrom a: simd_float2, to b: simd_float2) {
        let minX = min(a.x, b.x)
        let maxX = max(a.x, b.x)
        let minY = min(a.y, b.y)
        let maxY = max(a.y, b.y)
        self.vertices = [
            simd_float2(minX, minY), simd_float2(maxX, minY), simd_float2(maxX, maxY),
            simd_float2(minX, maxY),
        ]
    }

    /// True when fewer than three vertices make no enclosed area.
    var isDegenerate: Bool { vertices.count < 3 }

    /// Even-odd containment. Handles concave lassos, which a convex-only test
    /// would silently get wrong on exactly the hand-drawn outlines this is for.
    func contains(_ p: simd_float2) -> Bool {
        guard !isDegenerate else { return false }
        var inside = false
        var j = vertices.count - 1
        for i in 0..<vertices.count {
            let a = vertices[i]
            let b = vertices[j]
            // Half-open vertical span test: a vertex exactly on the ray is
            // counted once, not twice, so a point level with a vertex does
            // not flip parity twice and read as outside.
            if (a.y > p.y) != (b.y > p.y) {
                let t = (p.y - a.y) / (b.y - a.y)
                let crossX = a.x + t * (b.x - a.x)
                if p.x < crossX { inside.toggle() }
            }
            j = i
        }
        return inside
    }

    /// View-plane bounding box, used to reject most points before the
    /// per-edge test on large frames.
    var bounds: (min: simd_float2, max: simd_float2)? {
        guard let first = vertices.first else { return nil }
        var lo = first
        var hi = first
        for v in vertices.dropFirst() {
            lo = simd_min(lo, v)
            hi = simd_max(hi, v)
        }
        return (lo, hi)
    }
}

/// A depth range along the view's depth axis, in metres.
///
/// The slab is what keeps a top-down lasso from taking a lamp post along with
/// the van beneath it. It is part of the selection rule, not a display filter.
struct DepthSlab: Equatable {
    var minDepth: Float
    var maxDepth: Float

    init(minDepth: Float, maxDepth: Float) {
        self.minDepth = min(minDepth, maxDepth)
        self.maxDepth = max(minDepth, maxDepth)
    }

    func contains(_ depth: Float) -> Bool { depth >= minDepth && depth <= maxDepth }
}

/// How a new candidate set combines with the existing membership.
enum SelectionMode: String, Equatable {
    /// Replaces membership. The default when no modifier is held.
    case replace
    /// Adds to membership; the shift-drag case.
    case add
    /// Removes from membership; the option-drag case.
    case subtract

    /// Resolves the mode from the modifier keys held during a drag, so the
    /// mapping lives next to the semantics rather than in the view layer.
    static func from(shiftHeld: Bool, optionHeld: Bool) -> SelectionMode {
        if optionHeld { return .subtract }
        if shiftHeld { return .add }
        return .replace
    }
}

// MARK: - Selection engine

/// The result of evaluating a selection gesture, before it is applied.
///
/// The workflow requires the candidate count to be shown before acceptance,
/// so evaluation and application are deliberately separate steps.
struct SelectionCandidates: Equatable {
    /// Canonical indices inside both polygon and slab, ascending.
    var indices: [Int]
    /// Indices inside the polygon but rejected by the slab. Surfaced so an
    /// operator who expected more points can see the slab is what excluded
    /// them, rather than assuming the lasso missed.
    var excludedBySlab: Int
    /// Indices inside a painted column but in a voxel that is switched off, or
    /// outside the column's height. Zero for every tool but the column brush.
    var excludedByVoxels: Int = 0
    /// Indices the gesture covered whose class is hidden. A hidden return is
    /// not selectable, and the count says so rather than leaving a short
    /// selection unexplained.
    var excludedByVisibility: Int = 0

    var count: Int { indices.count }
}

/// Evaluates selection gestures against canonical point arrays.
struct PointSelectionEngine {
    /// Points inside both the polygon and the slab, as ascending canonical
    /// indices. Non-finite coordinates are skipped: a NaN cannot be inside
    /// anything, and letting one through would poison a saved mask.
    static func candidates(
        points: PackPoints, basis: OrthoViewBasis, polygon: SelectionPolygon, slab: DepthSlab?
    ) -> SelectionCandidates {
        guard !polygon.isDegenerate, let bounds = polygon.bounds else {
            return SelectionCandidates(indices: [], excludedBySlab: 0)
        }

        var indices: [Int] = []
        var excludedBySlab = 0
        for i in 0..<points.count {
            let p = simd_float3(points.x[i], points.y[i], points.z[i])
            guard p.x.isFinite, p.y.isFinite, p.z.isFinite else { continue }

            let v = basis.project(p)
            // Cheap rejection first; the per-edge crossing test is the
            // expensive part on a 100k-point frame.
            if v.x < bounds.min.x || v.x > bounds.max.x || v.y < bounds.min.y || v.y > bounds.max.y
            {
                continue
            }
            guard polygon.contains(v) else { continue }

            guard basis.shows(p) else { continue }
            if let slab, !slab.contains(basis.depth(p)) {
                excludedBySlab += 1
                continue
            }
            indices.append(i)
        }
        return SelectionCandidates(indices: indices, excludedBySlab: excludedBySlab)
    }

    /// Combines candidates with the existing membership.
    static func apply(mode: SelectionMode, candidates: [Int], to current: Set<Int>) -> Set<Int> {
        let incoming = Set(candidates)
        switch mode {
        case .replace: return incoming
        case .add: return current.union(incoming)
        case .subtract: return current.subtracting(incoming)
        }
    }

    /// The canonical stored form of a membership set: ascending, de-duplicated.
    /// An identical membership always encodes identically, so two saves of the
    /// same selection produce the same bytes.
    static func canonicalIndices(_ membership: Set<Int>) -> [Int] { membership.sorted() }

    /// Validates operator-supplied indices against a sample's point domain.
    /// Rejects negative, duplicate and out-of-range references rather than
    /// clamping them, because a silently clamped index is a wrong label.
    static func validate(indices: [Int], pointCount: Int) -> Result<[Int], SelectionValidationError>
    {
        var seen = Set<Int>()
        for i in indices {
            if i < 0 || i >= pointCount {
                return .failure(.outOfRange(index: i, pointCount: pointCount))
            }
            if !seen.insert(i).inserted { return .failure(.duplicate(index: i)) }
        }
        return .success(indices.sorted())
    }
}

enum SelectionValidationError: Error, Equatable {
    case outOfRange(index: Int, pointCount: Int)
    case duplicate(index: Int)

    var message: String {
        switch self {
        case .outOfRange(let index, let pointCount):
            return "point index \(index) is outside this sample's 0..<\(pointCount) domain"
        case .duplicate(let index): return "point index \(index) appears more than once"
        }
    }
}

// MARK: - Undo/redo

/// An undo stack over membership edits for one sample.
///
/// This is the in-session stroke history the operator expects from a normal
/// editor. It is separate from the sidecar's revision log, which recovers
/// saved snapshots: undoing a stroke that was never saved must not need a
/// file write, and restoring a saved revision must not silently discard
/// unsaved strokes.
struct MembershipHistory: Equatable {
    private(set) var current: Set<Int>
    private var past: [Set<Int>] = []
    private var future: [Set<Int>] = []

    /// Bounded so a long labelling session cannot grow without limit on a
    /// frame with many thousands of points.
    let limit: Int

    init(initial: Set<Int> = [], limit: Int = 64) {
        self.current = initial
        self.limit = max(1, limit)
    }

    var canUndo: Bool { !past.isEmpty }
    var canRedo: Bool { !future.isEmpty }

    /// Records an edit. A no-op edit is not recorded, so undo never appears
    /// to do nothing.
    @discardableResult mutating func commit(_ next: Set<Int>) -> Bool {
        guard next != current else { return false }
        past.append(current)
        if past.count > limit { past.removeFirst(past.count - limit) }
        future.removeAll()
        current = next
        return true
    }

    @discardableResult mutating func undo() -> Bool {
        guard let previous = past.popLast() else { return false }
        future.append(current)
        current = previous
        return true
    }

    @discardableResult mutating func redo() -> Bool {
        guard let next = future.popLast() else { return false }
        past.append(current)
        current = next
        return true
    }

    /// Replaces the membership and clears the history, for loading a saved
    /// mask. Keeping strokes from a different sample's history would let undo
    /// walk membership backwards into another frame's selection.
    mutating func reset(to membership: Set<Int>) {
        current = membership
        past.removeAll()
        future.removeAll()
    }
}
