// PhysicalFitSection.swift
// The first thing in physical-reference mode: fit the object's size and poses
// to its reviewed returns, and see what the fit found at this frame.
//
// The fit fills the draft; it does not save or review. What it shows here is
// what each bound adds up and why each end counts as seen or not, so the one
// judgement left to the operator, whether an end is real, is made with the
// evidence in view.

import SwiftUI

struct PhysicalFitSection: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject var physical: PhysicalReferenceSession
    let objectID: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Fit to reviewed points").font(.subheadline.bold())
            Text(
                "Measures this object's size and pose from its reviewed returns. The result is a proposal: check it in the views, save, then review."
            ).font(.caption2).foregroundStyle(.secondary).fixedSize(
                horizontal: false, vertical: true)
            if let blocker {
                Text(blocker).font(.caption2).foregroundStyle(.orange).fixedSize(
                    horizontal: false, vertical: true)
            }
            HStack {
                Button("Fit object") {
                    Task { await physical.fit(objectID: objectID, scope: .object) }
                }.help(
                    "Fits the size from the frames that show it end to end, and places a pose at each of them."
                )
                Button("Fit pose here") {
                    guard let sample = session.currentSample else { return }
                    Task {
                        await physical.fit(
                            objectID: objectID, scope: .pose(sampleID: sample.sampleID))
                    }
                }.disabled(session.currentSample == nil).help(
                    "Fits the pose at this frame. The size is kept if the draft has one.")
            }.controlSize(.small).disabled(
                blocker != nil || !physical.canEdit || physical.needsReload)
            if let fit = physical.lastFit, fit.objectID == objectID { fitted(fit) }
        }
    }

    /// Why the fit cannot run yet, in the operator's terms.
    private var blocker: String? {
        guard let object = session.sidecar.objects.first(where: { $0.objectID == objectID }) else {
            return "This object is not in the saved annotation yet."
        }
        if object.status != .reviewed {
            return "Review this object before fitting: the fit reads only reviewed points."
        }
        let proposed = session.sidecar.masks.filter {
            $0.objectID == objectID && $0.status == .proposed
        }.count
        if proposed > 0 {
            return
                "\(proposed) frame\(proposed == 1 ? " is" : "s are") still proposed for this object: review every frame before fitting."
        }
        if !session.dirtySamples.isEmpty {
            return "Save the point changes first: the fit reads the saved masks."
        }
        return nil
    }

    private func fitted(_ fit: PhysicalFitResult) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            if let body = fit.object?.body { Text(sizeLine(body)).font(.caption.monospacedDigit()) }
            ForEach(fit.notes ?? [], id: \.self) { note in
                Text(note).font(.caption2).foregroundStyle(.orange).fixedSize(
                    horizontal: false, vertical: true)
            }
            if let sample = session.currentSample {
                if let frame = fit.frame(sampleID: sample.sampleID) {
                    frameLines(frame)
                } else {
                    Text("This frame has no reviewed returns of the object.").font(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
        }.padding(6).background(
            Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 6))
    }

    private func sizeLine(_ body: PhysicalBody) -> String {
        func one(_ name: String, _ d: PhysicalDimension) -> String {
            switch d.choice {
            case .unknown: return "\(name) unknown"
            case .atLeast: return String(format: "%@ ≥ %.2f m", name, d.lowerM ?? 0)
            case .full:
                guard let best = d.best else { return "\(name) unbounded" }
                return String(format: "%@ %.2f ± %.2f m", name, best.value, best.halfWidth)
            }
        }
        return [one("length", body.length), one("width", body.width), one("height", body.height)]
            .joined(separator: " · ")
    }

    private func frameLines(_ f: PhysicalFitFrame) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            if let skipped = f.skipped {
                Text("This frame: \(f.returns) returns, \(skipped).").font(.caption2)
                    .foregroundStyle(.secondary)
            } else {
                Text(
                    String(
                        format:
                            "This frame: %d returns at %.1f m, aspect %.0f°; heading from %@ ± %.1f°",
                        f.returns, f.rangeM, f.aspectDeg, f.yawSource, f.yawBoundRad * 180 / .pi)
                ).font(.caption2.monospacedDigit()).foregroundStyle(.secondary)
                face("front", f.front)
                face("rear", f.rear)
                face("left", f.left)
                face("right", f.right)
                ForEach(f.detached ?? [], id: \.self) { d in
                    Text(d).font(.caption2).foregroundStyle(.orange).fixedSize(
                        horizontal: false, vertical: true)
                }
            }
        }
    }

    private func face(_ name: String, _ face: PhysicalFitFace) -> some View {
        let terms = (face.terms ?? []).map { String(format: "%@ %.3f", $0.name, $0.m) }.joined(
            separator: ", ")
        return Text(
            String(
                format: "%@ · %@ · %@ · ± %.3f m (%@)", name, face.seen ? "seen" : "not seen",
                face.reason, face.boundM, terms)
        ).font(.caption2.monospacedDigit()).foregroundStyle(face.seen ? .primary : .secondary)
            .fixedSize(horizontal: false, vertical: true).textSelection(.enabled)
    }
}
