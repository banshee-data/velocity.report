import SwiftUI
import simd

struct FeatureAuthoringPane: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject var features: FeatureAuthoring

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Facets · points fixed to one part").font(.headline)
            Text(
                "Save the object's points first. Select a small edge, surface patch, or local feature such as a mirror tip. Each facet keeps its identity across frames."
            ).font(.caption)
            if let object = session.activeObjectID {
                Text("\(features.activeCount(objectID: object)) of 4 facets active").font(
                    .caption.bold())
            }
            Picker("Select points", selection: $features.selectionTool) {
                Text("Sphere").tag(FeatureSelectionTool.sphere)
                Text("Lasso subset").tag(FeatureSelectionTool.lasso)
            }.pickerStyle(.segmented).disabled(!features.canEdit)
            Text(
                features.selectionTool == .lasso
                    ? "Drag an outline in the editing view. Shift adds; Option removes. The depth slab limits selection."
                    : "Click a saved return to seed a sphere. Drag pans the view."
            ).font(.caption2).foregroundStyle(.secondary)
            Text("Part: body · relation and metric anchor unresolved").font(.caption2)
                .foregroundStyle(.secondary)
            HStack {
                Button("Reload") { Task { await features.load() } }.disabled(
                    features.isDirty || features.busy)
                Button("New facet") { features.newFeature() }.disabled(
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
                            Text(
                                feature.inactive
                                    ? "retired" : "\(feature.observations.count) frames")
                            if feature.featureID == features.activeID {
                                Image(systemName: "checkmark")
                            }
                        }
                    }.disabled(features.isDirty || features.busy).font(.caption)
                }
            }
            if let active = features.active {
                Text(active.featureID).font(.caption2.monospaced()).textSelection(.enabled)
                Button(active.inactive ? "Activate facet" : "Retire facet, keep evidence") {
                    Task {
                        await features.setInactive(!active.inactive, author: session.operatorName)
                    }
                }.disabled(!features.canEdit || features.isDirty)
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
            ).disabled(!features.canEdit || features.draft?.method == "manual_lasso")
            if let observation = session.featureOverlay {
                Text(features.isDirty ? "Unsaved preview" : decisionLabel(observation.decision))
                    .font(.caption).foregroundStyle(features.isDirty ? .orange : .cyan)
                Text(
                    "\(observation.pointIndices.count) returns · \(observation.method) · \(observation.origin)"
                ).font(.caption).textSelection(.enabled)
            }
            if let observation = session.featureOverlay {
                let fit = FacetGeometryFit.analyse(
                    points: session.currentPoints, indices: observation.pointIndices,
                    geometry: features.geometry)
                VStack(alignment: .leading, spacing: 3) {
                    Text("Geometry check · proposal only").font(.caption.bold())
                    Text(fit.explanation).font(.caption2)
                    if fit.direction != nil {
                        Text(
                            "Span \(fit.spanM, specifier: "%.2f") m · spread \(fit.transverseSpreadM, specifier: "%.2f") m · RMS \(fit.residualM, specifier: "%.3f") m"
                        ).font(.caption2.monospacedDigit())
                        Text("Weak: \(fit.weakDirections)").font(.caption2).foregroundStyle(
                            .secondary)
                    }
                }
            }
            Text(
                "Accepted proposals are not reviewed physical references. Propagation is translation-only and stops at gaps or weak support."
            ).font(.caption2).foregroundStyle(.secondary)
            HStack {
                Button("Save facet proposal") { save(.acceptedProposal) }.disabled(
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
