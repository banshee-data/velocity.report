// PhysicalCompareViews.swift
// Compare mode: a scored evaluation report at the current instant.
//
// Read-only. The overlay draws the reference as the report recorded it and
// the estimate beside it, each labelled; the pane shows the report's
// identity, whether it is bound to what is open, what was and was not
// scored, and this frame's component errors with their reasons. A medoid or
// a visible-return centre is labelled as what it is, and never drawn as a
// body centre.

import AppKit
import SwiftUI
import UniformTypeIdentifiers
import simd

struct PhysicalCompareOverlay: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject var inspector: PhysicalReportInspector
    let standard: OrthoViewBasis.Standard
    let viewport: OrthoViewport

    init(session: AnnotationSession, standard: OrthoViewBasis.Standard, viewport: OrthoViewport) {
        self.session = session
        self.inspector = session.reportInspector
        self.standard = standard
        self.viewport = viewport
    }

    static let referenceColour = Color(red: 0.3, green: 0.85, blue: 1.0)
    static let predictionColour = Color(red: 1.0, green: 0.35, blue: 0.85)

    var body: some View {
        let basis = session.basis(standard)
        let viewport = viewport
        let instants =
            session.currentSample.flatMap { sample in
                inspector.report?.instants(arm: inspector.arm, sampleID: sample.sampleID)
            } ?? []
        let armLabel = inspector.armIdentity?.label ?? "estimate"
        let revision = inspector.report?.reference.physicalRevision ?? 0
        let names = Dictionary(
            instants.map { ($0.objectID, session.displayName(objectID: $0.objectID)) },
            uniquingKeysWith: { a, _ in a })
        let metresPerPoint: Double =
            viewport.size.height > 0
            ? Double(viewport.halfHeight * 2) / Double(viewport.size.height) : 0
        return Canvas { context, size in
            guard metresPerPoint > 0, standard == .top else { return }
            func screen(_ x: Double, _ y: Double) -> CGPoint {
                viewport.screenPoint(from: basis.project(simd_float3(Float(x), Float(y), 0)))
            }
            func circle(_ x: Double, _ y: Double, _ r: Double) -> Path {
                let c = screen(x, y)
                let rp = max(CGFloat(r / metresPerPoint), 2)
                return Path(
                    ellipseIn: CGRect(x: c.x - rp, y: c.y - rp, width: rp * 2, height: rp * 2))
            }
            func outline(_ box: PhysicalBox) -> Path {
                var p = Path()
                p.addLines(box.corners.map { screen($0.x, $0.y) })
                p.closeSubpath()
                return p
            }
            for instant in instants {
                let name = names[instant.objectID] ?? instant.objectID
                if let g = instant.reference?.geometry {
                    let ref = Self.referenceColour
                    if let c = g.centre ?? g.anchorPoint {
                        context.stroke(circle(c.x, c.y, c.bound), with: .color(ref), lineWidth: 1.2)
                        context.draw(
                            Text("\(name) · reference r\(revision)").font(.system(size: 9))
                                .foregroundColor(ref),
                            at: CGPoint(x: screen(c.x, c.y).x, y: screen(c.x, c.y).y - 14))
                    }
                    if let box = g.box {
                        context.stroke(outline(box), with: .color(ref), lineWidth: 1.2)
                    }
                    for (p, mark) in [(g.front, "F"), (g.rear, "R")] {
                        if let p {
                            context.draw(
                                Text(mark).font(.system(size: 9, weight: .bold)).foregroundColor(
                                    ref), at: screen(p.x, p.y))
                        }
                    }
                }
                if let p = instant.prediction {
                    let pred = Self.predictionColour
                    let dash = StrokeStyle(lineWidth: 1.2, dash: [4, 3])
                    context.stroke(
                        circle(p.x, p.y, p.positionSigma), with: .color(pred), style: dash)
                    if let box = p.box {
                        context.stroke(outline(box), with: .color(pred), style: dash)
                    }
                    if let h = p.heading {
                        let o = screen(p.centreX ?? p.x, p.centreY ?? p.y)
                        let t = screen(
                            (p.centreX ?? p.x) + cos(h.rad) * 2, (p.centreY ?? p.y) + sin(h.rad) * 2
                        )
                        var line = Path()
                        line.move(to: o)
                        line.addLine(to: t)
                        context.stroke(line, with: .color(pred), style: dash)
                    }
                    // The point the estimate states is labelled as what it
                    // is: a body centre only when the estimate says so.
                    let s = screen(p.x, p.y)
                    context.draw(
                        Text("\(armLabel) · \(p.reference) · \(p.trackKey)").font(.system(size: 9))
                            .foregroundColor(pred), at: CGPoint(x: s.x, y: s.y + 14))
                }
            }
            _ = size
        }.allowsHitTesting(false)
    }
}

struct PhysicalComparePane: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject var inspector: PhysicalReportInspector
    @ObservedObject var physical: PhysicalReferenceSession

    init(session: AnnotationSession) {
        self.session = session
        self.inspector = session.reportInspector
        self.physical = session.physical
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Compare").font(.headline)
            Text(
                "Read-only. Once an estimate for an object has been on screen, any later edit of that "
                    + "object's reference is saved as tracker-assisted, naming the estimate."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            if !session.comparisonAllowed {
                Text("This pack is held out: estimates are not compared on it.").font(.caption)
                    .foregroundStyle(.orange)
            } else {
                HStack {
                    Button("Open Report…") { chooseReport() }
                    if inspector.report != nil { Button("Close") { inspector.close() } }
                }.controlSize(.small)
                if let error = inspector.error {
                    Text(error).font(.caption).foregroundStyle(.red).fixedSize(
                        horizontal: false, vertical: true)
                }
                if let report = inspector.report { reportSection(report) }
            }
            if !physical.exposure.isEmpty {
                Text(
                    "Estimates seen for: "
                        + physical.exposure.keys.sorted().map(session.displayName(objectID:))
                        .joined(separator: ", ")
                ).font(.caption2).foregroundStyle(.orange).fixedSize(
                    horizontal: false, vertical: true)
            }
        }
    }

    private func chooseReport() {
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.json]
        panel.canChooseDirectories = false
        panel.message = "Choose a per-frame evaluation report (JSON) run with -physical-reference."
        guard panel.runModal() == .OK, let url = panel.url else { return }
        Task {
            await inspector.open(
                url: url, packDigest: session.pack.manifest.packDigest, role: session.packRole,
                physical: session.physical)
            session.exposeComparedObjects()
        }
    }

    private func reportSection(_ report: PhysicalComparisonReport) -> some View {
        let ref = report.reference
        let arm = report.arms[inspector.arm]
        return VStack(alignment: .leading, spacing: 6) {
            Text(inspector.reportName ?? "report").font(.caption.bold()).lineLimit(1)
                .truncationMode(.middle)
            Picker(
                "Arm",
                selection: Binding(
                    get: { inspector.arm },
                    set: {
                        inspector.arm = $0
                        session.exposeComparedObjects()
                    })
            ) {
                ForEach(report.arms.indices, id: \.self) { i in
                    Text(
                        "\(i == 0 ? "A" : "B") · \(report.arms[i].arm.label) (\(report.arms[i].arm.stage))"
                    ).tag(i)
                }
            }.font(.caption)
            Text(
                "Split \(ref.split) (\(ref.splitRole)) · membership r\(ref.sidecarRevision) · "
                    + "reference r\(ref.physicalRevision)"
            ).font(.caption2).foregroundStyle(.secondary)
            revisionLine(ref)
            accounting(arm)
            instantsHere(report)
            if !arm.caveats.isEmpty {
                DisclosureGroup("Caveats (\(arm.caveats.count))") {
                    ForEach(arm.caveats, id: \.self) {
                        Text($0).font(.caption2).fixedSize(horizontal: false, vertical: true)
                    }
                }.font(.caption)
            }
        }
    }

    @ViewBuilder private func revisionLine(_ ref: ReportReferenceIdentity) -> some View {
        if let head = physical.state, head.revision != ref.physicalRevision {
            Text(
                "The references are now at revision \(head.revision). This report scored revision "
                    + "\(ref.physicalRevision) and is shown as scored; run a new evaluation to compare the current ones."
            ).font(.caption2).foregroundStyle(.orange).fixedSize(horizontal: false, vertical: true)
        }
        if inspector.retained != nil {
            Label("Scored revision retained, bytes match the report", systemImage: "checkmark.seal")
                .font(.caption2).foregroundStyle(.green)
        } else if let note = inspector.provenanceNote {
            Text(note).font(.caption2).foregroundStyle(.orange).fixedSize(
                horizontal: false, vertical: true)
        }
    }

    private func accounting(_ arm: ReportArm) -> some View {
        DisclosureGroup(
            "Accounting (\(arm.instants.count) instants, \(arm.reference.expectedInstants) expected)"
        ) {
            ForEach(arm.accounting.keys.sorted(), id: \.self) { component in
                let a = arm.accounting[component]!
                let rest = a.unscored.keys.sorted().map { category in
                    "\(category) \(a.unscored[category]!.values.reduce(0, +))"
                }.joined(separator: ", ")
                Text(
                    "\(component): \(a.scored)/\(a.expected) scored"
                        + (rest.isEmpty ? "" : " · \(rest)")
                ).font(.caption2.monospacedDigit()).fixedSize(horizontal: false, vertical: true)
            }
        }.font(.caption)
    }

    @ViewBuilder private func instantsHere(_ report: PhysicalComparisonReport) -> some View {
        let sample = session.currentSample
        let instants =
            sample.map { report.instants(arm: inspector.arm, sampleID: $0.sampleID) } ?? []
        Text("This frame").font(.caption.bold())
        if instants.isEmpty {
            Text("The report expected nothing at this frame.").font(.caption2).foregroundStyle(
                .secondary)
        }
        ForEach(Array(instants.enumerated()), id: \.offset) { _, instant in instantView(instant) }
    }

    private func instantView(_ i: ReportInstant) -> some View {
        let c = i.comparison
        func m(_ v: Double) -> String { String(format: "%.2f m", v) }
        return VStack(alignment: .leading, spacing: 1) {
            Text(session.displayName(objectID: i.objectID) + " · \(i.episodeID)").font(
                .caption.bold())
            if let match = i.match {
                Text(
                    "matched \(match.trackKey) at \(m(match.distance)) · \(match.candidates) in gate"
                ).font(.caption2)
            } else if i.prediction == nil {
                Text("no prediction matched").font(.caption2).foregroundStyle(.orange)
            }
            if let centre = c.centre {
                Text(
                    "centre \(m(centre.error)) (ref ±\(m(centre.referenceBound)), \(centre.predictionReference))"
                ).font(.caption2)
                if let step = centre.step {
                    Text(
                        "since sample \(step.fromSampleID): reference moved \(m(step.referenceMove)), "
                            + "estimate \(m(step.predictionMove))\(step.sameTrack ? "" : " (track changed)")"
                    ).font(.caption2).foregroundStyle(.secondary)
                }
            }
            if let yaw = c.yaw {
                Text(
                    String(
                        format: "yaw %.1f° (ref ±%.1f°)%@", PhysicalUnits.degrees(yaw.errorRad),
                        PhysicalUnits.degrees(yaw.referenceBoundRad),
                        yaw.axisOnly ? ", axis only" : "")
                ).font(.caption2)
            }
            ForEach([("length", c.length), ("width", c.width), ("height", c.height)], id: \.0) {
                name, d in
                if let d {
                    Text(
                        "\(name) \(m(d.predicted)) vs \(m(d.referenceValue)), outside bound \(m(d.outsideBound))"
                    ).font(.caption2)
                }
            }
            ForEach([("front", c.front), ("rear", c.rear)], id: \.0) { name, e in
                if let e {
                    Text("\(name) \(m(e.distance)) (ref ±\(m(e.referenceBound)))").font(.caption2)
                }
            }
            if let box = c.box { Text(String(format: "box IoU %.2f", box.iou)).font(.caption2) }
            let unscored = i.outcomes.filter { $0.value.category != "scored" }.sorted {
                $0.key < $1.key
            }
            ForEach(unscored, id: \.key) { component, outcome in
                Text("\(component): \(outcome.category)\(outcome.reason.map { " (\($0))" } ?? "")")
                    .font(.caption2).foregroundStyle(.secondary)
            }
        }.padding(6).background(
            Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 4))
    }
}
