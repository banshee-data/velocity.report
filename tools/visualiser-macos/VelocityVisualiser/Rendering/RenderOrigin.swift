// RenderOrigin.swift
// Camera-relative rendering (lidar-city-view-plan Item 6).
//
// Positions in the city run to 10 km and more, and Float32 has about 1 mm
// of resolution at 10 km: matrices built in Float32 at those distances
// jitter. So every placed drawable keeps its offset in Double, and the
// renderer draws in a local frame whose origin, `anchor`, is a Double
// point near the camera. Each draw's model matrix is computed in Double as
// (offset − anchor) and only then turned to Float32, so the values the GPU
// sees stay small near the eye.
//
// When the camera strays more than `recentreDistance` from the anchor, the
// anchor moves to the camera's target, and the camera, which is kept in the
// local frame, moves back by the same amount: nothing on screen moves.
// The sensor-frame view never recentres, so it draws exactly as it did.

import simd

struct RenderOrigin: Equatable, Sendable {
    /// The local frame's origin, in the world frame (the sensor's, or the
    /// city's in city mode), metres.
    var anchor: SIMD3<Double> = .zero
    /// How far the camera's target may go from the anchor before it moves.
    var recentreDistance: Float = 1_000

    /// A world point in the local frame.
    func local(_ world: SIMD3<Double>) -> SIMD3<Float> { SIMD3<Float>(world - anchor) }

    /// A local point in the world frame.
    func world(_ local: SIMD3<Float>) -> SIMD3<Double> { anchor + SIMD3<Double>(local) }

    /// The model matrix in the local frame for a drawable placed by
    /// `worldFromObject`, computed in Double.
    func model(_ worldFromObject: simd_double4x4) -> simd_float4x4 {
        var m = worldFromObject
        m.columns.3 -= SIMD4(anchor, 0)
        return simd_float4x4(m)
    }

    /// Moves the anchor to the target when it is too far; returns the shift,
    /// in the local frame, that the camera must move back by.
    mutating func recentre(target: SIMD3<Float>) -> SIMD3<Float>? {
        guard simd_length(target) > recentreDistance else { return nil }
        // Whole metres, so a recentred anchor is exactly representable.
        let shift = SIMD3<Double>(target).rounded(.toNearestOrEven)
        anchor += shift
        return SIMD3<Float>(shift)
    }
}

extension simd_float4x4 {
    /// A Double matrix as Float32, entry by entry.
    init(_ m: simd_double4x4) {
        self.init(
            columns: (
                SIMD4<Float>(m.columns.0), SIMD4<Float>(m.columns.1), SIMD4<Float>(m.columns.2),
                SIMD4<Float>(m.columns.3)
            ))
    }
}

extension simd_double4x4 {
    init(_ m: simd_float4x4) {
        self.init(
            columns: (
                SIMD4<Double>(m.columns.0), SIMD4<Double>(m.columns.1), SIMD4<Double>(m.columns.2),
                SIMD4<Double>(m.columns.3)
            ))
    }

    init(translation t: SIMD3<Double>) {
        self.init(
            columns: (
                SIMD4(1, 0, 0, 0), SIMD4(0, 1, 0, 0), SIMD4(0, 0, 1, 0), SIMD4(t.x, t.y, t.z, 1)
            ))
    }
}
