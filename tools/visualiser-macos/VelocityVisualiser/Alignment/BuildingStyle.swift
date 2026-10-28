// BuildingStyle.swift
// How the operator wants the W4–W7 buildings drawn: the opacity of their
// edges, walls and roofs, and the thickness of three kinds of edge. All of
// it is applied when the frame is drawn, so a slider or a stepper answers at
// once and the overlay is never rebuilt for it.
//
//   Buildings   the wireframe: eaves, corners and the roof planes' edges
//   Walls       a face on each wall of a part, from its base to its top
//   Roofs       the roof planes' fills (their outlines belong to Buildings)
//
//   Eaves         the horizontal outlines at the base and the top of a part
//   Corners       the uprights at the part's vertices
//   Intermediate  the roof planes' outlines: ridges, hips and valleys
//
// Each surface has an opacity and an Opaque check; Opaque is solid whatever
// the opacity says, which the pane greys out while it is checked.

import Foundation
import simd

struct BuildingStyle: Equatable, Sendable, Codable {
    /// One surface's opacity.
    struct Surface: Equatable, Sendable, Codable {
        var opacity: Float
        /// Completely opaque: alpha is 1 whatever `opacity` is.
        var opaque: Bool

        init(opacity: Float, opaque: Bool = false) {
            self.opacity = opacity
            self.opaque = opaque
        }

        /// What the draw multiplies the surface's colour by, 0 to 1.
        var alpha: Float {
            guard !opaque else { return 1 }
            return opacity.isFinite ? min(max(opacity, 0), 1) : 1
        }

        var isVisible: Bool { alpha > 0 }
    }

    /// The wireframe: as it was drawn before there was a choice, solid.
    var buildings = Surface(opacity: 1, opaque: true)
    /// Wall faces: off until asked for, so the view is as it was.
    var walls = Surface(opacity: 0)
    /// Roof fills: as translucent as they were.
    var roofs = Surface(opacity: ReferenceStyle.roofFillAlpha)

    /// Line thicknesses in pixels.
    var eavesWidth = 1
    var cornersWidth = 1
    var intermediateWidth = 1

    static let widthRange = 1...8

    /// `width` held to `widthRange`.
    static func clampedWidth(_ width: Int) -> Int {
        min(max(width, widthRange.lowerBound), widthRange.upperBound)
    }

    /// The style with every width held to `widthRange`, as a saved value
    /// edited by hand might not be.
    var clamped: BuildingStyle {
        var out = self
        out.eavesWidth = Self.clampedWidth(eavesWidth)
        out.cornersWidth = Self.clampedWidth(cornersWidth)
        out.intermediateWidth = Self.clampedWidth(intermediateWidth)
        return out
    }

    static let defaultsKey = "alignment.buildingStyle"

    /// The style saved in `defaults`, else the default; a value that does not
    /// decode is ignored rather than trusted.
    static func saved(in defaults: UserDefaults?) -> BuildingStyle {
        guard let data = defaults?.data(forKey: defaultsKey),
            let style = try? JSONDecoder().decode(BuildingStyle.self, from: data)
        else { return BuildingStyle() }
        return style.clamped
    }

    func save(in defaults: UserDefaults?) {
        guard let data = try? JSONEncoder().encode(self) else { return }
        defaults?.set(data, forKey: Self.defaultsKey)
    }
}

/// Thick lines from one-pixel ones. Metal draws a line one pixel wide, so a
/// thicker one is the same line drawn several times, each copy moved a pixel
/// or so across the screen; the copies together fill a disc of the width.
enum LineThickness {
    /// Screen offsets in pixels, centred on zero, for a line `width` pixels
    /// thick: one copy for 1, then a disc of whole-pixel steps.
    static func offsets(width: Int) -> [SIMD2<Float>] {
        let n = BuildingStyle.clampedWidth(width)
        guard n > 1 else { return [SIMD2(0, 0)] }
        let reach = Float(n) / 2 + 0.01
        let first = -Float(n - 1) / 2
        var out = [SIMD2<Float>]()
        for i in 0..<n {
            for j in 0..<n {
                let p = SIMD2(first + Float(i), first + Float(j))
                if (p.x * p.x + p.y * p.y).squareRoot() <= reach { out.append(p) }
            }
        }
        return out
    }

    /// The matrix that moves clip space by `pixels` on a drawable of
    /// `drawable` pixels, applied after projection: identity when the
    /// drawable has no size yet.
    static func clipOffset(_ pixels: SIMD2<Float>, drawable: SIMD2<Float>) -> simd_float4x4 {
        var m = matrix_identity_float4x4
        guard drawable.x > 0, drawable.y > 0 else { return m }
        // clip.xy += ndc * clip.w, and a pixel is 2 / size in NDC.
        m.columns.3.x = 2 * pixels.x / drawable.x
        m.columns.3.y = 2 * pixels.y / drawable.y
        return m
    }
}
