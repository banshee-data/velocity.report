import SwiftUI
import simd

struct FacetPoseProposalOverlay: View {
    @ObservedObject var session: AnnotationSession
    var basis: OrthoViewBasis
    var viewport: OrthoViewport
    var top: Bool

    var body: some View {
        let result = session.currentFacetPoseProposal
        let points = session.currentPoints
        Canvas { context, _ in
            guard let result else { return }
            for index in result.matchedIndices {
                guard let point = points.point(at: Int(index)), basis.shows(point) else { continue }
                let at = viewport.screenPoint(from: basis.project(point))
                guard at.x.isFinite, at.y.isFinite else { continue }
                context.stroke(
                    Path(ellipseIn: CGRect(x: at.x - 5, y: at.y - 5, width: 10, height: 10)),
                    with: .color(.pink), lineWidth: 2)
            }
            guard top else { return }
            let centre = viewport.screenPoint(
                from: basis.project(SIMD3(Float(result.centre.xM), Float(result.centre.yM), 0)))
            guard centre.x.isFinite, centre.y.isFinite else { return }
            if let vertices = result.footprint {
                let screen = vertices.map {
                    viewport.screenPoint(from: basis.project(SIMD3(Float($0.x), Float($0.y), 0)))
                }
                guard screen.allSatisfy({ $0.x.isFinite && $0.y.isFinite }) else { return }
                var path = Path()
                path.move(to: screen[0])
                for point in screen.dropFirst() { path.addLine(to: point) }
                path.closeSubpath()
                context.stroke(
                    path, with: .color(.orange), style: StrokeStyle(lineWidth: 2, dash: [7, 4]))
            }
            context.stroke(
                Path(ellipseIn: CGRect(x: centre.x - 5, y: centre.y - 5, width: 10, height: 10)),
                with: .color(.orange), lineWidth: 2)
            context.draw(
                Text("body pose proposal · prior yaw").font(.system(size: 10)).foregroundColor(
                    .orange), at: CGPoint(x: centre.x, y: centre.y - 16))
        }.allowsHitTesting(false)
    }
}
