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
    ///
    /// Horizontal only: road users move on the ground, and letting the search
    /// move a footprint vertically is how it would find the road beneath a car
    /// instead of the car. A tie goes to the offset nearest the prediction, so
    /// an empty scene leaves the footprint where it was predicted rather than
    /// at a corner of the search.
    func bestOffset(
        in points: PackPoints, near prediction: simd_float3, searchRadius: Float = 2,
        step: Float = SelectionFootprint.pitch, where include: (Int) -> Bool
    ) -> simd_float3 {
        guard !isEmpty, step > 0 else { return prediction }
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
        guard !nearby.isEmpty else { return prediction }

        let steps = Int((searchRadius / step).rounded(.down))
        var best = prediction
        var bestCount = -1
        var bestDistance = Float.greatestFiniteMagnitude
        for ix in -steps...steps {
            for iy in -steps...steps {
                let delta = simd_float3(Float(ix) * step, Float(iy) * step, 0)
                let offset = prediction + delta
                var count = 0
                for p in nearby where contains(p, offset: offset) { count += 1 }
                let distance = simd_length(delta)
                if count > bestCount || (count == bestCount && distance < bestDistance) {
                    best = offset
                    bestCount = count
                    bestDistance = distance
                }
            }
        }
        return best
    }
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
