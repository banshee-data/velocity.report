import SwiftUI
import simd

/// Top-only: body_xy does not provide a height or a surface in an elevation.
struct FacetBodyProjectionOverlay: View {
    @ObservedObject var session: AnnotationSession
    var basis: OrthoViewBasis
    var viewport: OrthoViewport

    var body: some View {
        let preview = session.currentFacetBodyPreview
        Canvas { context, _ in
            guard let projection = preview?.projection else { return }
            let origin = basis.project(
                SIMD3(Float(projection.origin.x), Float(projection.origin.y), 0))
            let style = StrokeStyle(lineWidth: 2, dash: [6, 4])
            if let normal = projection.normal {
                let vector = SIMD3(Float(-normal.y), Float(normal.x), 0)
                let tangent = SIMD2(simd_dot(vector, basis.right), simd_dot(vector, basis.up))
                // Choose a visible portion of an infinite line. Camera movement
                // changes this drawing only; it never supplies an along-edge anchor.
                let middle = origin + tangent * simd_dot(viewport.centre - origin, tangent)
                let span = 2 * (viewport.halfWidth + viewport.halfHeight)
                let a = viewport.screenPoint(from: middle - tangent * span)
                let b = viewport.screenPoint(from: middle + tangent * span)
                guard [a.x, a.y, b.x, b.y].allSatisfy(\.isFinite) else { return }
                var path = Path()
                path.move(to: a)
                path.addLine(to: b)
                context.stroke(path, with: .color(.purple), style: style)
            } else {
                let at = viewport.screenPoint(from: origin)
                guard at.x.isFinite, at.y.isFinite else { return }
                context.stroke(
                    Path(ellipseIn: CGRect(x: at.x - 7, y: at.y - 7, width: 14, height: 14)),
                    with: .color(.purple), style: style)
                context.draw(
                    Text("fixed body spot · proposal").font(.system(size: 10)).foregroundColor(
                        .purple), at: CGPoint(x: at.x, y: at.y - 18))
            }
        }.allowsHitTesting(false)
    }
}
