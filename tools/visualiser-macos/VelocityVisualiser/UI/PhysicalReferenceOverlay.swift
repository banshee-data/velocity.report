// PhysicalReferenceOverlay.swift
// Draws the physical references at this sample over an orthographic view.
//
// Only what the keyframe establishes is drawn. A position with no length is a
// marker and its bound, not a plausible rectangle; a box appears only when
// centre, yaw, length and width are all known; an ambiguous axis shows an
// unsigned line and unsigned ends, never a front. In the elevations a
// position is a vertical line, because it has no height of its own, and a
// box stands where the 3D view stands it (`physicalBoxSpan`): the reference
// has a footprint and no floor.
//
// Proposed, reviewed and unsaved references are told apart by line style and
// a text label, not by colour alone.

import SwiftUI
import simd

struct PhysicalReferenceOverlay: View {
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

    enum Style: Equatable {
        case unsaved
        case proposed
        case reviewed

        var label: String {
            switch self {
            case .unsaved: return "unsaved"
            case .proposed: return "proposed ref"
            case .reviewed: return "reviewed ref"
            }
        }

        /// The same colour in the 3D view, which takes components.
        var rgb: SIMD3<Float> {
            switch self {
            case .unsaved: return SIMD3(1.0, 0.55, 0.1)
            case .proposed: return SIMD3(0.95, 0.85, 0.3)
            case .reviewed: return SIMD3(0.3, 0.95, 0.5)
            }
        }

        var colour: Color { Color(red: Double(rgb.x), green: Double(rgb.y), blue: Double(rgb.z)) }

        var dash: [CGFloat] {
            switch self {
            case .unsaved: return [2, 3]
            case .proposed: return [6, 4]
            case .reviewed: return []
            }
        }
    }

    /// How a draft keyframe is drawn: unsaved if it differs from the saved
    /// one, otherwise by its saved review.
    static func style(
        draft: PhysicalKeyframe, saved: PhysicalKeyframe?, body: PhysicalBody?,
        savedBody: PhysicalBody?
    ) -> Style {
        guard let saved, saved == draft, body == savedBody else { return .unsaved }
        return saved.review.status == .reviewed ? .reviewed : .proposed
    }

    var body: some View {
        let basis = session.basis(standard)
        let viewport = viewport
        let sampleID = session.currentSample?.sampleID
        let active = session.activeObjectID
        let items:
            [(
                name: String, geometry: PhysicalGeometry, keyframe: PhysicalKeyframe, style: Style,
                active: Bool, heights: ClosedRange<Float>
            )] = physical.draft.compactMap { object in
                guard let sampleID, let k = object.keyframe(sampleID: sampleID) else { return nil }
                let style = Self.style(
                    draft: k,
                    saved: physical.savedKeyframe(objectID: object.objectID, sampleID: sampleID),
                    body: object.body, savedBody: physical.savedBody(objectID: object.objectID))
                return (
                    session.displayName(objectID: object.objectID),
                    PhysicalGeometry.derive(object: object, keyframe: k, gate: .authoring), k,
                    style, object.objectID == active,
                    session.physicalBoxSpan(objectID: object.objectID, body: object.body)
                )
            }
        let metresPerPoint: Double =
            viewport.size.height > 0
            ? Double(viewport.halfHeight * 2) / Double(viewport.size.height) : 0

        return Canvas { context, size in
            guard metresPerPoint > 0 else { return }
            func screen(_ x: Double, _ y: Double, _ z: Double = 0) -> CGPoint {
                viewport.screenPoint(from: basis.project(simd_float3(Float(x), Float(y), Float(z))))
            }
            func radius(_ metres: Double) -> CGFloat { CGFloat(metres / metresPerPoint) }

            for item in items {
                let g = item.geometry
                let colour = item.style.colour.opacity(item.active ? 1 : 0.55)
                let stroke = StrokeStyle(lineWidth: item.active ? 1.6 : 1, dash: item.style.dash)
                let label =
                    "\(item.name) · \(item.style.label)"
                    + (g.unboundedDraft ? " · bounds not set" : "")
                    + (g.usesPriorSize ? " · size prior" : "")

                if standard == .top && item.active {
                    let at = PhysicalHandles.positions(
                        item.geometry, basis: basis, viewport: viewport)
                    if let m = at.move {
                        context.fill(
                            Path(CGRect(x: m.x - 4, y: m.y - 4, width: 8, height: 8)),
                            with: .color(colour.opacity(0.9)))
                    }
                    if let t = at.turn {
                        context.fill(
                            Path(
                                ellipseIn: CGRect(x: t.x - 4.5, y: t.y - 4.5, width: 9, height: 9)),
                            with: .color(colour))
                    }
                    if let l = at.length {
                        var diamond = Path()
                        diamond.move(to: CGPoint(x: l.x, y: l.y - 6))
                        diamond.addLine(to: CGPoint(x: l.x + 6, y: l.y))
                        diamond.addLine(to: CGPoint(x: l.x, y: l.y + 6))
                        diamond.addLine(to: CGPoint(x: l.x - 6, y: l.y))
                        diamond.closeSubpath()
                        context.fill(diamond, with: .color(colour))
                    }
                    if let w = at.width {
                        context.stroke(
                            Path(CGRect(x: w.x - 5, y: w.y - 5, width: 10, height: 10)),
                            with: .color(colour), lineWidth: 2)
                        context.draw(
                            Text("width").font(.system(size: 9)).foregroundColor(colour),
                            at: CGPoint(x: w.x, y: w.y - 12))
                    }
                    if at.move != nil {
                        context.draw(
                            Text(
                                "drag ■ to move, size held · drag ● to turn"
                                    + (at.length != nil ? " · drag ◆ for length" : "")
                                    + (at.width != nil ? " · drag □ for width" : "")
                            ).font(.system(size: 9)).foregroundColor(.secondary),
                            at: CGPoint(x: size.width / 2, y: size.height - 12))
                    }
                }
                if standard == .top {
                    drawTop(
                        g, item.keyframe, in: &context, screen: screen, radius: radius,
                        colour: colour, stroke: stroke)
                    if let p = g.centre ?? g.anchorPoint {
                        context.draw(
                            Text(label).font(
                                .system(size: 9, weight: item.active ? .bold : .regular)
                            ).foregroundColor(colour),
                            at: CGPoint(
                                x: screen(p.x, p.y).x, y: screen(p.x, p.y).y - radius(p.bound) - 10)
                        )
                    }
                } else {
                    drawElevation(
                        g, item.keyframe, heights: item.heights, basis: basis, in: &context,
                        size: size, screen: screen, colour: colour, stroke: stroke, label: label)
                }
            }
        }.allowsHitTesting(false)
    }

    private func drawTop(
        _ g: PhysicalGeometry, _ k: PhysicalKeyframe, in context: inout GraphicsContext,
        screen: (Double, Double, Double) -> CGPoint, radius: (Double) -> CGFloat, colour: Color,
        stroke: StrokeStyle
    ) {
        func circle(_ p: PhysicalPlanar) -> Path {
            let c = screen(p.x, p.y, 0)
            let r = max(radius(p.bound), 2)
            return Path(ellipseIn: CGRect(x: c.x - r, y: c.y - r, width: r * 2, height: r * 2))
        }
        func cross(_ p: PhysicalPlanar, size: CGFloat) -> Path {
            let c = screen(p.x, p.y, 0)
            var path = Path()
            path.move(to: CGPoint(x: c.x - size, y: c.y))
            path.addLine(to: CGPoint(x: c.x + size, y: c.y))
            path.move(to: CGPoint(x: c.x, y: c.y - size))
            path.addLine(to: CGPoint(x: c.x, y: c.y + size))
            return path
        }

        // The anchor as stated, with its horizontal bound.
        if let anchor = g.anchorPoint {
            context.stroke(cross(anchor, size: 5), with: .color(colour), lineWidth: 1.5)
            context.stroke(circle(anchor), with: .color(colour), style: stroke)
            if k.anchor.kind.isFace {
                let c = screen(anchor.x, anchor.y, 0)
                context.draw(
                    Text(k.anchor.kind.label.lowercased()).font(.system(size: 8)).foregroundColor(
                        colour), at: CGPoint(x: c.x + 14, y: c.y + 8))
            }
        }
        // The centre when it is derived from a face, with its wider bound.
        if let centre = g.centre, k.anchor.kind.isFace {
            context.stroke(circle(centre), with: .color(colour.opacity(0.7)), style: stroke)
            context.fill(
                Path(
                    ellipseIn: CGRect(origin: screen(centre.x, centre.y, 0), size: .zero).insetBy(
                        dx: -2, dy: -2)), with: .color(colour))
        }
        // The axis: an arrow when the front is known, an unsigned line when
        // only the axis is. Its length is fixed on screen: it says which way,
        // not how long.
        if let yaw = g.yaw, let origin = g.centre ?? g.anchorPoint {
            let o = screen(origin.x, origin.y, 0)
            let reach: CGFloat = 34
            func end(_ angle: Double, _ length: CGFloat) -> CGPoint {
                let tip = screen(origin.x + cos(angle), origin.y + sin(angle), 0)
                let dx = tip.x - o.x
                let dy = tip.y - o.y
                let n = max(hypot(dx, dy), 0.001)
                return CGPoint(x: o.x + dx / n * length, y: o.y + dy / n * length)
            }
            var axis = Path()
            axis.move(to: yaw.axis == .resolved ? o : end(yaw.rad + .pi, reach))
            let tip = end(yaw.rad, reach)
            axis.addLine(to: tip)
            context.stroke(axis, with: .color(colour), lineWidth: 1.5)
            if yaw.axis == .resolved {
                var head = Path()
                head.move(to: tip)
                head.addLine(to: end(yaw.rad, reach - 7).rotated(about: tip, by: 0.5))
                head.move(to: tip)
                head.addLine(to: end(yaw.rad, reach - 7).rotated(about: tip, by: -0.5))
                context.stroke(head, with: .color(colour), lineWidth: 1.5)
            }
            // The yaw bound as two faint rays.
            var wedge = Path()
            for side in [-1.0, 1.0] {
                wedge.move(to: o)
                wedge.addLine(to: end(yaw.rad + side * yaw.boundRad, reach * 0.8))
            }
            context.stroke(wedge, with: .color(colour.opacity(0.4)), lineWidth: 0.8)
        }
        // The box, only when every side of it is known.
        if let box = g.box {
            var outline = Path()
            let corners = box.corners.map { screen($0.x, $0.y, 0) }
            outline.addLines(corners)
            outline.closeSubpath()
            context.stroke(outline, with: .color(colour), style: stroke)
        }
        // Ends: signed where the front is known, unsigned where it is not.
        for (point, mark) in [(g.front, "F"), (g.rear, "R")] {
            guard let point else { continue }
            let c = screen(point.x, point.y, 0)
            context.stroke(circle(point), with: .color(colour.opacity(0.6)), lineWidth: 0.8)
            context.draw(
                Text(mark).font(.system(size: 9, weight: .bold)).foregroundColor(colour), at: c)
        }
        if g.front == nil && g.rear == nil, g.yaw?.axis == .frontRearAmbiguous {
            for point in g.ends {
                context.stroke(
                    circle(point), with: .color(colour.opacity(0.6)),
                    style: StrokeStyle(lineWidth: 0.8, dash: [2, 2]))
            }
        }
    }

    private func drawElevation(
        _ g: PhysicalGeometry, _ k: PhysicalKeyframe, heights: ClosedRange<Float>,
        basis: OrthoViewBasis, in context: inout GraphicsContext, size: CGSize,
        screen: (Double, Double, Double) -> CGPoint, colour: Color, stroke: StrokeStyle,
        label: String
    ) {
        guard let anchor = g.anchorPoint else { return }
        // Only the half-space this elevation looks into.
        guard basis.shows(simd_float3(Float(anchor.x), Float(anchor.y), 0)) else { return }
        func vertical(at x: CGFloat, style: StrokeStyle) {
            var line = Path()
            line.move(to: CGPoint(x: x, y: 0))
            line.addLine(to: CGPoint(x: x, y: size.height))
            context.stroke(line, with: .color(colour.opacity(0.7)), style: style)
        }
        let a = screen(anchor.x, anchor.y, 0)
        vertical(at: a.x, style: StrokeStyle(lineWidth: 1, dash: [3, 3]))
        if let z = k.position.zM {
            let p = screen(anchor.x, anchor.y, z)
            context.fill(
                Path(ellipseIn: CGRect(x: p.x - 3, y: p.y - 3, width: 6, height: 6)),
                with: .color(colour))
        }
        // The whole box, as the 3D view stands it: the footprint's four
        // corners at the bottom and the top, and the edges between them. A
        // turned box shows its near and far vertical edges inside the outline.
        if let box = g.box {
            let bottom = box.corners.map { screen($0.x, $0.y, Double(heights.lowerBound)) }
            let top = box.corners.map { screen($0.x, $0.y, Double(heights.upperBound)) }
            var edges = Path()
            for ring in [bottom, top] {
                edges.addLines(ring)
                edges.closeSubpath()
            }
            for (b, t) in zip(bottom, top) {
                edges.move(to: b)
                edges.addLine(to: t)
            }
            context.stroke(edges, with: .color(colour), style: stroke)
        }
        context.draw(
            Text(label).font(.system(size: 9)).foregroundColor(colour), at: CGPoint(x: a.x, y: 14))
    }
}

extension CGPoint {
    fileprivate func rotated(about centre: CGPoint, by angle: Double) -> CGPoint {
        let dx = x - centre.x
        let dy = y - centre.y
        let c = CGFloat(cos(angle))
        let s = CGFloat(sin(angle))
        return CGPoint(x: centre.x + dx * c - dy * s, y: centre.y + dx * s + dy * c)
    }
}
