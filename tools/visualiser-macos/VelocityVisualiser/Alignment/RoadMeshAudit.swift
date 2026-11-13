// RoadMeshAudit.swift
// The first-pass mesh checks of the review, run on the loaded patches rather
// than by eye: inverted or degenerate triangles, triangles that overlap one
// another in plan (within a patch or across patches), and height steps at the
// seams where patches share vertices (the contract allows 5 mm).
//
// It reads geometry only. A clean audit says the mesh is well formed, not that
// it matches the road.

import Foundation
import simd

struct RoadPatchAudit: Equatable, Sendable {
    var patchID: String
    var triangles: Int
    /// Counter-clockwise seen from +z is the contract; a clockwise triangle faces down.
    var inverted: Int
    var degenerate: Int
    /// Triangles whose centroid lies inside another triangle of the same patch.
    var overlapsWithin: Int
    /// ... or inside a triangle of another patch.
    var overlapsAcross: Int
    var isClean: Bool {
        inverted == 0 && degenerate == 0 && overlapsWithin == 0 && overlapsAcross == 0
    }
}

struct RoadSeamStep: Equatable, Sendable {
    var patchA: String
    var patchB: String
    var at: SIMD2<Float>
    var stepM: Float
}

struct RoadMeshAudit: Equatable, Sendable {
    var patches: [RoadPatchAudit]
    /// Vertex pairs of different patches within `seamToleranceXY` of one another,
    /// counted and the worst step in height.
    var seamPairs: Int
    var seamStepMaxM: Float
    var seamViolations: [RoadSeamStep]

    static let seamToleranceXY: Float = 0.005
    static let seamToleranceZ: Float = 0.005

    var isClean: Bool { patches.allSatisfy(\.isClean) && seamViolations.isEmpty }
    var invertedTotal: Int { patches.reduce(0) { $0 + $1.inverted } }
    var overlapTotal: Int { patches.reduce(0) { $0 + $1.overlapsWithin + $1.overlapsAcross } }
    var degenerateTotal: Int { patches.reduce(0) { $0 + $1.degenerate } }

    static func audit(_ patches: [RoadPatch]) -> RoadMeshAudit {
        let resident = patches.filter(\.isResident)
        // Cell index of triangles for the overlap test.
        struct Tri {
            var a: SIMD2<Float>
            var b: SIMD2<Float>
            var c: SIMD2<Float>
            var patch: Int
            var id: Int
        }
        let cell: Float = 4
        func key(_ x: Float, _ y: Float) -> SIMD2<Int32> {
            SIMD2(Int32((x / cell).rounded(.down)), Int32((y / cell).rounded(.down)))
        }
        var tris: [Tri] = []
        var grid: [SIMD2<Int32>: [Int]] = [:]
        var inverted = [Int](repeating: 0, count: resident.count)
        var degenerate = [Int](repeating: 0, count: resident.count)
        for (pi, p) in resident.enumerated() {
            for t in 0..<p.triangleCount {
                let a = p.vertices[Int(p.indices[3 * t])]
                let b = p.vertices[Int(p.indices[3 * t + 1])]
                let c = p.vertices[Int(p.indices[3 * t + 2])]
                let area2 = (b.x - a.x) * (c.y - a.y) - (b.y - a.y) * (c.x - a.x)
                if abs(area2) < 1e-6 {
                    degenerate[pi] += 1
                    continue
                }
                if area2 < 0 { inverted[pi] += 1 }
                let i = tris.count
                tris.append(
                    Tri(
                        a: SIMD2(a.x, a.y), b: SIMD2(b.x, b.y), c: SIMD2(c.x, c.y), patch: pi, id: i
                    ))
                let lo = simd_min(simd_min(SIMD2(a.x, a.y), SIMD2(b.x, b.y)), SIMD2(c.x, c.y))
                let hi = simd_max(simd_max(SIMD2(a.x, a.y), SIMD2(b.x, b.y)), SIMD2(c.x, c.y))
                let k0 = key(lo.x, lo.y)
                let k1 = key(hi.x, hi.y)
                for x in k0.x...k1.x {
                    for y in k0.y...k1.y { grid[SIMD2(x, y), default: []].append(i) }
                }
            }
        }
        var within = [Int](repeating: 0, count: resident.count)
        var across = [Int](repeating: 0, count: resident.count)
        for t in tris {
            let m = (t.a + t.b + t.c) / 3
            var hitWithin = false
            var hitAcross = false
            for j in grid[key(m.x, m.y)] ?? [] where j != t.id {
                let o = tris[j]
                if strictlyInside(m, o.a, o.b, o.c) {
                    if o.patch == t.patch { hitWithin = true } else { hitAcross = true }
                }
            }
            if hitWithin { within[t.patch] += 1 }
            if hitAcross { across[t.patch] += 1 }
        }
        // Seams: vertices of different patches within a few millimetres in plan.
        var vgrid: [SIMD2<Int32>: [(Int, SIMD3<Float>)]] = [:]
        let vcell: Float = 0.05
        for (pi, p) in resident.enumerated() {
            for v in p.vertices where v.z.isFinite {
                vgrid[
                    SIMD2(Int32((v.x / vcell).rounded(.down)), Int32((v.y / vcell).rounded(.down))),
                    default: []
                ].append((pi, v))
            }
        }
        var pairs = 0
        var worst: Float = 0
        var violations: [RoadSeamStep] = []
        for (k, items) in vgrid {
            var candidates = items
            for dx in -1...1 {
                for dy in -1...1 where dx != 0 || dy != 0 {
                    candidates += vgrid[SIMD2(k.x + Int32(dx), k.y + Int32(dy))] ?? []
                }
            }
            for (pa, va) in items {
                for (pb, vb) in candidates where pb > pa {
                    guard simd_length(SIMD2(va.x - vb.x, va.y - vb.y)) <= seamToleranceXY else {
                        continue
                    }
                    pairs += 1
                    let step = abs(va.z - vb.z)
                    worst = max(worst, step)
                    if step > seamToleranceZ, violations.count < 50 {
                        violations.append(
                            RoadSeamStep(
                                patchA: resident[pa].patchID, patchB: resident[pb].patchID,
                                at: SIMD2(va.x, va.y), stepM: step))
                    }
                }
            }
        }
        let audits = resident.enumerated().map { i, p in
            RoadPatchAudit(
                patchID: p.patchID, triangles: p.triangleCount, inverted: inverted[i],
                degenerate: degenerate[i], overlapsWithin: within[i], overlapsAcross: across[i])
        }
        return RoadMeshAudit(
            patches: audits, seamPairs: pairs, seamStepMaxM: worst, seamViolations: violations)
    }

    /// Strictly inside, by a margin that leaves shared edges and corners out.
    static func strictlyInside(
        _ p: SIMD2<Float>, _ a: SIMD2<Float>, _ b: SIMD2<Float>, _ c: SIMD2<Float>
    ) -> Bool {
        let d = (b.y - c.y) * (a.x - c.x) + (c.x - b.x) * (a.y - c.y)
        guard abs(d) > 1e-12 else { return false }
        let l1 = ((b.y - c.y) * (p.x - c.x) + (c.x - b.x) * (p.y - c.y)) / d
        let l2 = ((c.y - a.y) * (p.x - c.x) + (a.x - c.x) * (p.y - c.y)) / d
        let l3 = 1 - l1 - l2
        let m: Float = 0.02
        return l1 > m && l2 > m && l3 > m
    }
}
