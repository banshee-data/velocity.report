import SwiftUI
import simd

struct FeatureAuthoringPane: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject var features: FeatureAuthoring

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Feature Candidates · object masks unchanged").font(.headline)
            Text(
                "Save this object's points first. Click a return to seed a sphere; drag pans the view."
            ).font(.caption)
            Text("Part: body · relation and metric anchor unresolved").font(.caption2)
                .foregroundStyle(.secondary)
            HStack {
                Button("Reload") { Task { await features.load() } }.disabled(
                    features.isDirty || features.busy)
                Button("New feature") { features.newFeature() }.disabled(
                    !features.canEdit || features.isDirty)
            }
            if let state = features.state {
                Text("Revision \(state.document.revision)").font(.caption.monospacedDigit())
                ForEach(
                    state.document.features.filter { $0.objectID == session.activeObjectID },
                    id: \.featureID
                ) { feature in
                    Button {
                        features.choose(feature.featureID)
                        if let current = session.featureOverlay, current.hasSphere {
                            features.radius = current.sphere.radiusM
                        }
                    } label: {
                        HStack {
                            Text(feature.name.isEmpty ? "Feature" : feature.name)
                            Spacer()
                            Text("\(feature.observations.count) frames")
                            if feature.featureID == features.activeID {
                                Image(systemName: "checkmark")
                            }
                        }
                    }.disabled(features.isDirty || features.busy).font(.caption)
                }
            }
            if let active = features.active {
                Text(active.featureID).font(.caption2.monospaced()).textSelection(.enabled)
            }
            Button("Edit this frame") { session.editCurrentFeature() }.disabled(
                !features.canEdit || features.isDirty || session.featureOverlay?.hasSphere != true)
            TextField("Feature name", text: $features.name).disabled(!features.canEdit)
            Picker("Geometry", selection: $features.geometry) {
                Text("Unknown").tag(FeatureGeometry.unknown)
                Text("Edge").tag(FeatureGeometry.edge)
                Text("Corner").tag(FeatureGeometry.corner)
                Text("Patch").tag(FeatureGeometry.patch)
                Text("Protrusion").tag(FeatureGeometry.protrusion)
            }.disabled(!features.canEdit)
            TextField("Tentative meaning (optional)", text: $features.semanticHint).disabled(
                !features.canEdit)
            Text(
                "Geometry describes shape; meaning is a tentative label, such as mirror or wheel well. These are labels, not recognition presets."
            ).font(.caption2).foregroundStyle(.secondary)
            Text(
                "Radius \(features.radius, specifier: "%.2f") m · diameter \(features.radius * 2, specifier: "%.2f") m"
            ).font(.caption.monospacedDigit())
            Slider(
                value: Binding(
                    get: { features.radius },
                    set: {
                        features.radius = $0
                        session.resizeFeature($0)
                    }), in: 0.02...2, step: 0.01
            ).disabled(!features.canEdit)
            if let observation = session.featureOverlay {
                Text(features.isDirty ? "Unsaved preview" : decisionLabel(observation.decision))
                    .font(.caption).foregroundStyle(features.isDirty ? .orange : .cyan)
                Text(
                    "\(observation.pointIndices.count) returns · \(observation.method) · \(observation.origin)"
                ).font(.caption).textSelection(.enabled)
            }
            Text(
                "Accepted proposals are not reviewed physical references. Propagation is translation-only and stops at gaps or weak support."
            ).font(.caption2).foregroundStyle(.secondary)
            HStack {
                Button("Accept and save") { save(.acceptedProposal) }.disabled(
                    !features.canEdit || (features.draft?.pointIndices.isEmpty ?? true))
                Button("Reject") { save(.rejected) }.disabled(
                    !features.canEdit || features.draft == nil)
            }
            HStack {
                Button("Missing") { save(.missing) }
                Button("Occluded") { save(.occluded) }
            }.disabled(
                !features.canEdit
                    || (features.draft == nil
                        && features.active?.objectID != session.activeObjectID)
            )
            Button("Preview next frame") { session.proposeNextFeature() }.disabled(
                !features.canEdit || features.isDirty || features.active == nil)
            Button("Cancel / stop") { features.cancel() }.disabled(features.busy)
            if features.busy { ProgressView().controlSize(.small) }
            if let message = features.message {
                Text(message).font(.caption).foregroundStyle(.orange).textSelection(.enabled)
            }
        }.textFieldStyle(.roundedBorder).controlSize(.small)
    }

    private func decisionLabel(_ decision: FeatureDecision) -> String {
        switch decision {
        case .acceptedProposal: return "Saved proposal"
        case .rejected: return "Rejected"
        case .missing: return "Missing"
        case .occluded: return "Occluded"
        default: return "Unresolved"
        }
    }

    private func save(_ decision: FeatureDecision) {
        if features.draft == nil, decision == .missing || decision == .occluded {
            session.stageFeatureAbsence()
        }
        Task {
            _ = await features.save(
                decision: decision, author: session.operatorName,
                membershipDigest: session.membershipDigest)
        }
    }
}

struct FeatureSelectionOverlay: View {
    @ObservedObject var session: AnnotationSession
    var basis: OrthoViewBasis
    var viewport: OrthoViewport

    var body: some View {
        let observation = session.featureOverlay
        let points = session.currentPoints
        let draft = session.features.isDirty
        Canvas { context, _ in
            guard let o = observation, o.hasSphere else { return }
            let colour: Color = draft ? .orange : .cyan
            let centre = viewport.screenPoint(from: basis.project(o.sphere.centre))
            let radius =
                CGFloat(o.sphere.radiusM) * viewport.size.height / CGFloat(2 * viewport.halfHeight)
            guard radius.isFinite, radius > 0 else { return }
            let circle = Path(
                ellipseIn: CGRect(
                    x: centre.x - radius, y: centre.y - radius, width: 2 * radius,
                    height: 2 * radius))
            context.stroke(
                circle, with: .color(colour),
                style: StrokeStyle(lineWidth: 1.5, dash: draft ? [4, 3] : []))
            for index in o.pointIndices {
                guard let p = points.point(at: Int(index)), basis.shows(p) else { continue }
                let screen = viewport.screenPoint(from: basis.project(p))
                context.stroke(
                    Path(ellipseIn: CGRect(x: screen.x - 3, y: screen.y - 3, width: 6, height: 6)),
                    with: .color(colour), lineWidth: 1.5)
            }
        }.allowsHitTesting(false)
    }
}
