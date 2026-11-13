// RoadWorkingSet.swift
// What the driving view keeps near the eye: drawn within 75 m, fetched out to
// 150 m, dropped beyond 200 m. The radii are the plan's proposed defaults
// (GGP-ROAD-PROTOTYPE) and are adjusted from the first measured run.
//
// Eviction drops a patch's mesh and keeps its skeleton (rings, profile, stats,
// hashes), so a route can still pass it and an inspector can still name it.

import Foundation
import simd

struct RoadResidencyPolicy: Equatable, Sendable {
    var visibleM: Float = 75
    var prefetchM: Float = 150
    var evictM: Float = 200

    /// Radii must nest: drawn inside fetched inside kept.
    var isValid: Bool { 0 < visibleM && visibleM <= prefetchM && prefetchM <= evictM }
}

enum RoadWorkingSet {
    /// Resident patches within the visible radius of `eye`.
    static func visible(
        _ patches: [RoadPatch], eye: SIMD2<Float>, policy: RoadResidencyPolicy
    ) -> [RoadPatch] {
        patches.filter { $0.isResident && $0.distance(from: eye) <= policy.visibleM }
    }

    /// Ids of patches in view, for noticing a change that needs a rebuild.
    static func visibleIDs(
        _ patches: [RoadPatch], eye: SIMD2<Float>, policy: RoadResidencyPolicy
    ) -> Set<String> { Set(visible(patches, eye: eye, policy: policy).map(\.patchID)) }

    /// Whether the eye has moved far enough that the fetch window should move:
    /// a quarter of the prefetch radius from where it was asked last.
    static func needsWindow(
        eye: SIMD2<Float>, lastRequestedAt: SIMD2<Float>?, policy: RoadResidencyPolicy
    ) -> Bool {
        guard let last = lastRequestedAt else { return true }
        return simd_length(eye - last) >= policy.prefetchM / 4
    }
}
