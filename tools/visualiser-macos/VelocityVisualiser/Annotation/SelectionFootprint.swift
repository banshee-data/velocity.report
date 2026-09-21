// SelectionFootprint.swift
// The shape of a selection, kept so that it can be laid over another sample.
//
// A membership is point indices, and an index means one return in one scan:
// it cannot be carried to the next sample. The space those returns occupied
// can. A footprint is that space, voxelised, and asking which of another
// sample's returns fall inside it gives a proposal for the same object there:
// exact for a building, and for a car one nudge along the road away.

import Foundation
import simd

struct SelectionFootprint: Equatable {
    /// Voxel edge, in metres. Half the column lattice's pitch: fine enough to
    /// keep a pedestrian apart from the wall behind them.
    static let pitch: Float = 0.25

    /// Occupied voxels, already dilated.
    private(set) var voxels: Set<SIMD3<Int32>> = []
    /// Bounds of the occupied space, in metres.
    private(set) var lower = simd_float3(repeating: .greatestFiniteMagnitude)
    private(set) var upper = simd_float3(repeating: -.greatestFiniteMagnitude)

    var isEmpty: Bool { voxels.isEmpty }

    /// The space around `points`, grown by `dilation` voxels each way. One
    /// voxel of growth is what lets the next scan's returns, which never land
    /// where this scan's did, still fall inside.
    init(points: [simd_float3], dilation: Int32 = 1) {
        for p in points where p.x.isFinite && p.y.isFinite && p.z.isFinite {
            let v = SelectionFootprint.voxel(of: p)
            for dx in -dilation...dilation {
                for dy in -dilation...dilation {
                    for dz in -dilation...dilation {
                        voxels.insert(SIMD3(v.x + dx, v.y + dy, v.z + dz))
                    }
                }
            }
            let reach = Float(dilation + 1) * SelectionFootprint.pitch
            lower = simd_min(lower, p - reach)
            upper = simd_max(upper, p + reach)
        }
    }

    static func voxel(of p: simd_float3) -> SIMD3<Int32> {
        SIMD3(
            Int32((p.x / pitch).rounded(.down)), Int32((p.y / pitch).rounded(.down)),
            Int32((p.z / pitch).rounded(.down)))
    }

    /// True when `p` is inside the footprint after the footprint has been
    /// moved by `offset`.
    func contains(_ p: simd_float3, offset: simd_float3) -> Bool {
        voxels.contains(SelectionFootprint.voxel(of: p - offset))
    }

    /// Canonical indices of the returns inside the moved footprint, ascending.
    func indices(in points: PackPoints, offset: simd_float3, where include: (Int) -> Bool) -> [Int]
    {
        guard !isEmpty else { return [] }
        let lo = lower + offset
        let hi = upper + offset
        var result: [Int] = []
        for index in 0..<points.count {
            let p = simd_float3(points.x[index], points.y[index], points.z[index])
            guard p.x >= lo.x, p.x <= hi.x, p.y >= lo.y, p.y <= hi.y, p.z >= lo.z, p.z <= hi.z,
                include(index), contains(p, offset: offset)
            else { continue }
            result.append(index)
        }
        return result
    }

    /// The horizontal offset near `prediction` that puts the most returns
    /// inside the footprint.
    func bestOffset(
        in points: PackPoints, near prediction: simd_float3, searchRadius: Float = 2,
        step: Float = SelectionFootprint.pitch, where include: (Int) -> Bool
    ) -> simd_float3 {
        fit(in: points, near: prediction, searchRadius: searchRadius, step: step, where: include)
            .offset
    }

    /// Where the footprint fits best near `prediction`, and how well.
    ///
    /// Horizontal only: road users move on the ground, and letting the search
    /// move a footprint vertically is how it would find the road beneath a car
    /// instead of the car. A tie goes to the offset nearest the prediction, so
    /// an empty scene leaves the footprint where it was predicted rather than
    /// at a corner of the search.
    func fit(
        in points: PackPoints, near prediction: simd_float3, searchRadius: Float = 2,
        step: Float = SelectionFootprint.pitch, where include: (Int) -> Bool
    ) -> FootprintFit {
        guard !isEmpty, step > 0 else { return FootprintFit(offset: prediction) }
        // Only returns the footprint could reach at some offset in the search.
        let lo = lower + prediction - searchRadius
        let hi = upper + prediction + searchRadius
        var nearby: [simd_float3] = []
        for index in 0..<points.count where include(index) {
            let p = simd_float3(points.x[index], points.y[index], points.z[index])
            if p.x >= lo.x, p.x <= hi.x, p.y >= lo.y, p.y <= hi.y, p.z >= lo.z, p.z <= hi.z {
                nearby.append(p)
            }
        }
        guard !nearby.isEmpty else { return FootprintFit(offset: prediction) }

        let steps = Int((searchRadius / step).rounded(.down))
        var scores: [(delta: simd_float3, count: Int)] = []
        scores.reserveCapacity((2 * steps + 1) * (2 * steps + 1))
        var best = FootprintFit(offset: prediction, count: -1)
        var bestDistance = Float.greatestFiniteMagnitude
        for ix in -steps...steps {
            for iy in -steps...steps {
                let delta = simd_float3(Float(ix) * step, Float(iy) * step, 0)
                var count = 0
                for p in nearby where contains(p, offset: prediction + delta) { count += 1 }
                scores.append((delta, count))
                let distance = simd_length(delta)
                if count > best.count || (count == best.count && distance < bestDistance) {
                    best = FootprintFit(offset: prediction + delta, count: count)
                    bestDistance = distance
                }
            }
        }
        best.runnerUp = SelectionFootprint.rival(
            to: best.offset - prediction, in: scores, side: 2 * steps + 1, step: step)
        return best
    }

    /// The best score at a different place from `bestDelta`.
    ///
    /// "A different place" is a separate peak, not a distance. A six-metre car
    /// moved a metre along itself still covers most of its own returns, and
    /// counting that as a rival refused to carry any large object at all. A
    /// rival is a local maximum with a valley between it and the best fit:
    /// somewhere on the straight line between them the score drops well below
    /// the rival's own, which is what two objects look like and one does not.
    static func rival(
        to bestDelta: simd_float3, in scores: [(delta: simd_float3, count: Int)], side: Int,
        step: Float
    ) -> Int {
        guard side > 0, scores.count == side * side else { return 0 }
        func score(_ ix: Int, _ iy: Int) -> Int {
            guard ix >= 0, iy >= 0, ix < side, iy < side else { return 0 }
            return scores[ix * side + iy].count
        }
        let half = (side - 1) / 2
        let bx = Int((bestDelta.x / step).rounded()) + half
        let by = Int((bestDelta.y / step).rounded()) + half

        var runnerUp = 0
        for ix in 0..<side {
            for iy in 0..<side {
                let here = score(ix, iy)
                guard here > runnerUp, ix != bx || iy != by else { continue }
                let distance =
                    Float((ix - bx) * (ix - bx) + (iy - by) * (iy - by)).squareRoot() * step
                guard distance >= FootprintFit.rivalDistance else { continue }
                // A peak of its own.
                var isPeak = true
                for dx in -1...1 {
                    for dy in -1...1 where score(ix + dx, iy + dy) > here { isPeak = false }
                }
                guard isPeak else { continue }
                // With a valley between it and the best fit.
                let samples = max(abs(ix - bx), abs(iy - by))
                var lowest = here
                for k in 1..<max(samples, 1) {
                    let t = Float(k) / Float(samples)
                    let lx = bx + Int((Float(ix - bx) * t).rounded())
                    let ly = by + Int((Float(iy - by) * t).rounded())
                    lowest = min(lowest, score(lx, ly))
                }
                if Float(lowest) < Float(here) * FootprintFit.valleyFraction { runnerUp = here }
            }
        }
        return runnerUp
    }
}

/// How well a footprint fitted a frame.
struct FootprintFit: Equatable {
    /// Nearer than this, two peaks are one fit measured a voxel to one side.
    static let rivalDistance: Float = 1.0
    /// How far the score must fall between two peaks, as a share of the lower
    /// one, for them to be two places.
    static let valleyFraction: Float = 0.7

    var offset: simd_float3
    /// Returns inside the footprint at `offset`.
    var count = 0
    /// Returns inside it at the best rival position: see `rival(to:in:side:step:)`.
    var runnerUp = 0
}

/// A footprint laid over the current sample, waiting to be accepted.
struct CarriedSelection: Equatable {
    var footprint: SelectionFootprint
    /// Where the footprint has been moved to, relative to where it was made.
    var offset: simd_float3
    /// The sample it was made from.
    var fromSampleID: Int
    /// +1 when carried forward in time, -1 when carried back.
    var direction: Int
}
