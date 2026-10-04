import SwiftUI
import simd

/// The original estimate remains distinct from the editable reference. There
/// is no vertical position in a planar report, so it draws only in Top.
struct PhysicalSeedGhostOverlay: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject var physical: PhysicalReferenceSession
    let standard: OrthoViewBasis.Standard
    let viewport: OrthoViewport

    init(session: AnnotationSession, standard: OrthoViewBasis.Standard, viewport: OrthoViewport) {
        self.session = session
        self.physical = session.physical
        self.standard = standard
        self.viewport = viewport
    }

    var body: some View {
        let ghost = physical.seedGhost
        let basis = session.basis(standard)
        Canvas { context, _ in
            guard standard == .top, let ghost, ghost.objectID == session.activeObjectID,
                ghost.sampleID == session.currentSample?.sampleID, let box = ghost.prediction.box
            else { return }
            func screen(_ x: Double, _ y: Double) -> CGPoint {
                viewport.screenPoint(from: basis.project(SIMD3(Float(x), Float(y), 0)))
            }
            let colour = PhysicalCompareOverlay.predictionColour
            var path = Path()
            path.addLines(box.corners.map { screen($0.x, $0.y) })
            path.closeSubpath()
            context.stroke(
                path, with: .color(colour), style: StrokeStyle(lineWidth: 1, dash: [8, 4]))
            let centre = screen(box.centreX, box.centreY)
            context.draw(
                Text("original seed · \(ghost.prediction.trackKey)").font(.system(size: 9))
                    .foregroundColor(colour), at: CGPoint(x: centre.x, y: centre.y + 24))
        }.allowsHitTesting(false)
    }
}
