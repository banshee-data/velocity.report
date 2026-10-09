// ObjectFocus.swift
// Keeping one object in view as the frames change: where it is in a frame,
// where it is between two labelled frames, and how the views follow it.
//
// Clicking an object frames it in every view. From then on each step re-centres
// the views on it, at the scale the operator has, so a car driving through the
// scene stays under the cursor rather than leaving the view after a dozen
// frames. Nothing here selects anything: it moves the views.

import Foundation
import simd

/// Where an object's returns are in one frame: their extent in each
/// orthographic view, and their bounds in the scene.
struct ObjectFootprint: Equatable {
    /// Per view. An elevation shows half the scene, so a view the object is
    /// behind has no entry, and is left alone.
    var extents: [OrthoViewBasis.Standard: AnnotationExtent]
    var lower: simd_float3
    var upper: simd_float3

    var centre: simd_float3 { (lower + upper) / 2 }
    /// Half the bounds' diagonal: what a camera has to take in.
    var radius: Float { simd_length(upper - lower) / 2 }

    /// The footprint of these returns, or nil when there are none.
    static func of(
        _ points: [simd_float3], bases: [(OrthoViewBasis.Standard, OrthoViewBasis)]
    ) -> ObjectFootprint? {
        let finite = points.filter { $0.x.isFinite && $0.y.isFinite && $0.z.isFinite }
        guard let first = finite.first else { return nil }
        var lower = first
        var upper = first
        for p in finite {
            lower = simd_min(lower, p)
            upper = simd_max(upper, p)
        }
        var extents: [OrthoViewBasis.Standard: AnnotationExtent] = [:]
        for (standard, basis) in bases {
            var lo = simd_float2(repeating: .greatestFiniteMagnitude)
            var hi = simd_float2(repeating: -.greatestFiniteMagnitude)
            var seen = false
            for p in finite where basis.shows(p) {
                let v = basis.project(p)
                lo = simd_min(lo, v)
                hi = simd_max(hi, v)
                seen = true
            }
            guard seen else { continue }
            extents[standard] = AnnotationExtent(
                centre: (lo + hi) / 2, halfHeight: (hi.y - lo.y) / 2, halfWidth: (hi.x - lo.x) / 2)
        }
        return ObjectFootprint(extents: extents, lower: lower, upper: upper)
    }

    /// Part of the way from this footprint to another: where the object is
    /// in a frame between two it is labelled in. A view that sees it in only
    /// one of the two takes that one.
    func interpolated(to other: ObjectFootprint, fraction t: Float) -> ObjectFootprint {
        let t = min(max(t, 0), 1)
        var extents: [OrthoViewBasis.Standard: AnnotationExtent] = [:]
        for standard in Set(self.extents.keys).union(other.extents.keys) {
            switch (self.extents[standard], other.extents[standard]) {
            case (let a?, let b?):
                extents[standard] = AnnotationExtent(
                    centre: simd_mix(a.centre, b.centre, simd_float2(repeating: t)),
                    halfHeight: a.halfHeight + (b.halfHeight - a.halfHeight) * t,
                    halfWidth: a.halfWidth + (b.halfWidth - a.halfWidth) * t)
            case (let a?, nil): extents[standard] = a
            case (nil, let b?): extents[standard] = b
            case (nil, nil): break
            }
        }
        let mix = simd_float3(repeating: t)
        return ObjectFootprint(
            extents: extents, lower: simd_mix(lower, other.lower, mix),
            upper: simd_mix(upper, other.upper, mix))
    }
}

/// How the views follow an object from frame to frame.
///
/// A view the operator has not moved frames the object, and grows, never
/// shrinks, to keep it whole: a scale that changed every frame with the
/// returns the car happened to show would make it pulse. A view the operator
/// has panned or zoomed keeps their offset and scale and moves with the
/// object, so following never undoes what they did by hand.
struct ObjectFollowFraming: Equatable {
    /// Half width and half height each view has framed the object at.
    var halfSizes: [OrthoViewBasis.Standard: simd_float2] = [:]
    /// Where the object was in each view at the last step.
    var centres: [OrthoViewBasis.Standard: simd_float2] = [:]

    /// Frames the object tightly in every view that sees it, discarding what
    /// the operator did by hand: what clicking an object asks for.
    mutating func fit(
        _ footprint: ObjectFootprint, extents: inout [OrthoViewBasis.Standard: AnnotationExtent],
        states: inout [OrthoViewBasis.Standard: OrthoViewState]
    ) {
        halfSizes = [:]
        centres = [:]
        for (standard, e) in footprint.extents {
            halfSizes[standard] = simd_float2(e.halfWidth, e.halfHeight)
            centres[standard] = e.centre
            extents[standard] = e
            states[standard] = nil
        }
    }

    /// Moves every view that sees the object to where it is now.
    mutating func follow(
        _ footprint: ObjectFootprint, extents: inout [OrthoViewBasis.Standard: AnnotationExtent],
        states: inout [OrthoViewBasis.Standard: OrthoViewState]
    ) {
        for (standard, e) in footprint.extents {
            if var state = states[standard] {
                // Where the operator put it, carried along with the object.
                if let last = centres[standard] {
                    state.centre += e.centre - last
                } else {
                    state.centre = e.centre
                }
                states[standard] = state
            } else {
                let size = simd_max(
                    halfSizes[standard] ?? .zero, simd_float2(e.halfWidth, e.halfHeight))
                halfSizes[standard] = size
                extents[standard] = AnnotationExtent(
                    centre: e.centre, halfHeight: size.y, halfWidth: size.x)
            }
            centres[standard] = e.centre
        }
    }
}

/// Where the object is at a frame it has no mask in, from the frames around
/// it that do. Before its first labelled frame and after its last there is
/// nothing to say, and the views are left where they are.
enum ObjectLifecycle {
    /// The two labelled frames either side of `index`, and how far between
    /// them it is; the frame itself when it is labelled. Nil outside the
    /// object's lifecycle.
    static func bracket(
        _ index: Int, labelled: [Int]
    ) -> (before: Int, after: Int, fraction: Float)? {
        guard let first = labelled.first, let last = labelled.last, index >= first, index <= last
        else { return nil }
        if labelled.contains(index) { return (index, index, 0) }
        guard let after = labelled.first(where: { $0 > index }),
            let before = labelled.last(where: { $0 < index })
        else { return nil }
        return (before, after, Float(index - before) / Float(after - before))
    }
}

extension Camera {
    /// Looks at a region along the sensor's own line of sight to it, from far
    /// enough back to take it in: the object as the LiDAR saw it. The sensor
    /// is the pack's origin.
    ///
    /// The line is steepened to at least `minimumPitch` below the horizontal.
    /// From the sensor's 2.3 m a car fifty metres off is seen almost edge on,
    /// and from there a roof and the road around it are both a line.
    mutating func lookFromSensor(at focus: AnnotationSceneFocus, minimumPitch: Float = 0.21) {
        let radius = max(focus.radius, 1)
        let halfFov = fov * .pi / 360
        let distance = min(max(radius / tan(halfFov) * 1.25, 2), 500)
        let ray = focus.centre
        let horizontal = simd_length(simd_float2(ray.x, ray.y))
        guard horizontal > 0.5 else {
            lookAt(focus)
            return
        }
        let pitch = max(atan2(-ray.z, horizontal), minimumPitch)
        let heading = simd_float2(ray.x, ray.y) / horizontal
        let direction = simd_float3(heading.x * cos(pitch), heading.y * cos(pitch), -sin(pitch))
        target = focus.centre
        position = focus.centre - direction * distance
        up = simd_float3(0, 0, 1)
        projection = .perspective
    }

    /// Part of the way from one camera to another, eased at both ends, so a
    /// view that follows a moving object glides rather than jumps.
    static func glide(from a: Camera, to b: Camera, progress: Float) -> Camera {
        let t = min(max(progress, 0), 1)
        let eased = t * t * (3 - 2 * t)
        var c = b
        let mix = simd_float3(repeating: eased)
        c.position = simd_mix(a.position, b.position, mix)
        c.target = simd_mix(a.target, b.target, mix)
        let up = simd_mix(a.up, b.up, mix)
        c.up = simd_length(up) > 1e-4 ? simd_normalize(up) : b.up
        return c
    }
}
