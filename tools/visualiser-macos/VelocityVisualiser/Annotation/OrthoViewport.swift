// OrthoViewport.swift
// Between a view's points and an orthographic view plane's metres.
//
// It lives beside the session rather than with the views because the session
// uses it too, and because it is the one piece of the editing surface that can
// be tested without mounting anything.

import CoreGraphics
import simd

/// Maps between a view's points and an orthographic view plane's metres.
///
/// Kept as a value type with no SwiftUI dependency so the conversion is
/// testable without mounting a view: an off-by-one in the vertical flip is
/// exactly the kind of bug that silently selects the wrong points.
struct OrthoViewport: Equatable {
    /// Half-height of the visible world volume, in metres.
    var halfHeight: Float
    /// The view's size in points.
    var size: CGSize
    /// View-plane coordinates at the centre of the view, in metres.
    var centre: simd_float2

    var halfWidth: Float {
        guard size.height > 0 else { return halfHeight }
        return halfHeight * Float(size.width / size.height)
    }

    /// Screen point (origin top-left, y down) to view-plane metres (y up).
    func worldPoint(from screen: CGPoint) -> simd_float2 {
        guard size.width > 0, size.height > 0 else { return centre }
        let nx = Float(screen.x / size.width) * 2 - 1
        // Screen y grows downward; the view plane's up axis grows upward.
        let ny = 1 - Float(screen.y / size.height) * 2
        return simd_float2(centre.x + nx * halfWidth, centre.y + ny * halfHeight)
    }

    /// View-plane metres back to a screen point, for drawing the outline and
    /// the selected points over the render.
    func screenPoint(from world: simd_float2) -> CGPoint {
        guard halfWidth > 0, halfHeight > 0 else { return .zero }
        let nx = (world.x - centre.x) / halfWidth
        let ny = (world.y - centre.y) / halfHeight
        return CGPoint(
            x: CGFloat((nx + 1) * 0.5) * size.width, y: CGFloat((1 - ny) * 0.5) * size.height)
    }
}
