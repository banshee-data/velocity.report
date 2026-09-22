// Lattice.swift
// One rule for which cell of a square lattice a position falls in.
//
// Four places quantised world positions with the same floor-divide and their
// own pitch constant: the selection footprint at 0.25 m, the proposer's voxels
// at 0.5 m and its plan bins at 2 m, and the column grid the brushes paint on.
// Four copies of the arithmetic is four chances for the rounding at a cell
// boundary to disagree, and they have to agree: a footprint fitted on one
// lattice is searched in steps of another, and the occupancy grid is meant to
// be the same lattice the brushes paint.

import Foundation
import simd

/// A square lattice of a fixed pitch, anchored on the origin.
struct Lattice: Equatable {
    /// Edge of one cell, in metres.
    let pitch: Float

    init(pitch: Float) { self.pitch = pitch }

    /// The cell containing a position, in three dimensions.
    func voxel(_ p: simd_float3) -> SIMD3<Int32> { SIMD3(index(p.x), index(p.y), index(p.z)) }

    /// The column containing a position, ignoring height.
    func cell(x: Float, y: Float) -> SIMD2<Int32> { SIMD2(index(x), index(y)) }

    /// World position of a column's centre.
    func centre(ofCell c: SIMD2<Int32>) -> simd_float2 {
        simd_float2((Float(c.x) + 0.5) * pitch, (Float(c.y) + 0.5) * pitch)
    }

    /// Floors, so that a position on a boundary belongs to the cell above it
    /// and the cell west of the origin is -1 rather than a second copy of 0.
    private func index(_ v: Float) -> Int32 { Int32((v / pitch).rounded(.down)) }
}
