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

    /// How far the lattice is turned from the sensor's own axes, in degrees
    /// clockwise, so that its squares can line up with the kerbs instead of
    /// with however the tripod was set down.
    ///
    /// This is the scene's `grid_azimuth_deg`. The value belongs to the street
    /// and lives in map-marks.json; a lattice is only ever given a copy to
    /// quantise with. See AnnotationSession's grid azimuth.
    let azimuthDeg: Float

    private let cosA: Float
    private let sinA: Float

    init(pitch: Float, azimuthDeg: Float = 0) {
        self.pitch = pitch
        self.azimuthDeg = azimuthDeg
        let radians = azimuthDeg * .pi / 180
        cosA = cos(radians)
        sinA = sin(radians)
    }

    /// The cell containing a position, in three dimensions. Height is not
    /// turned: the lattice spins about the vertical.
    func voxel(_ p: simd_float3) -> SIMD3<Int32> {
        let c = cell(x: p.x, y: p.y)
        return SIMD3(c.x, c.y, index(p.z))
    }

    /// The column containing a position, ignoring height.
    func cell(x: Float, y: Float) -> SIMD2<Int32> {
        let g = intoLattice(x: x, y: y)
        return SIMD2(index(g.x), index(g.y))
    }

    /// World position of a column's centre.
    func centre(ofCell c: SIMD2<Int32>) -> simd_float2 {
        let g = simd_float2((Float(c.x) + 0.5) * pitch, (Float(c.y) + 0.5) * pitch)
        // Back out of the lattice's frame: the inverse turn.
        return simd_float2(g.x * cosA - g.y * sinA, g.x * sinA + g.y * cosA)
    }

    /// A position in the lattice's own frame.
    private func intoLattice(x: Float, y: Float) -> simd_float2 {
        guard azimuthDeg != 0 else { return simd_float2(x, y) }
        return simd_float2(x * cosA + y * sinA, -x * sinA + y * cosA)
    }

    /// Floors, so that a position on a boundary belongs to the cell above it
    /// and the cell west of the origin is -1 rather than a second copy of 0.
    private func index(_ v: Float) -> Int32 { Int32((v / pitch).rounded(.down)) }
}
