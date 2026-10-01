import SwiftUI
import simd

struct FeatureAuthoringPane: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject var features: FeatureAuthoring
    @ObservedObject private var inspection: IntensityInspectionState
    @State private var returnBoundM: Double?
    @State private var identityNote = ""

    init(session: AnnotationSession, features: FeatureAuthoring) {
        self.session = session
        self.features = features
        self.inspection = session.inspection
    }

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
            if features.active?.hasAnchor != true {
                Text("Part: body · metric anchor unresolved").font(.caption2).foregroundStyle(
                    .secondary)
            }
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
            TextField("Feature name", text: $features.name).disabled(!canEditMetadata)
            Picker("Geometry", selection: $features.geometry) {
                Text("Unknown").tag(FeatureGeometry.unknown)
                Text("Edge").tag(FeatureGeometry.edge)
                Text("Corner").tag(FeatureGeometry.corner)
                Text("Patch").tag(FeatureGeometry.patch)
                Text("Protrusion").tag(FeatureGeometry.protrusion)
            }.disabled(!canEditMetadata)
            TextField("Tentative meaning (optional)", text: $features.semanticHint).disabled(
                !canEditMetadata)
            if features.active != nil && !features.isDirty {
                Text(
                    "Use Edit this frame before changing its name, type or meaning. Those edits save with its support proposal."
                ).font(.caption2).foregroundStyle(.secondary)
            }
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
                if !features.isDirty && observation.membershipDigest != session.membershipDigest {
                    Text(
                        "Support belongs to an older object mask. Recheck and save this frame before propagating or registering it."
                    ).font(.caption2).foregroundStyle(.orange)
                }
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
            registrationSection
            Button("Cancel / stop") { features.cancel() }.disabled(features.busy)
            if features.busy { ProgressView().controlSize(.small) }
            if let message = features.message {
                Text(message).font(.caption).foregroundStyle(.orange).textSelection(.enabled)
            }
        }.textFieldStyle(.roundedBorder).controlSize(.small).onChange(of: session.activeObjectID) {
            _, _ in features.newFeature()
        }
    }

    private var canEditMetadata: Bool {
        features.canEdit && (features.active == nil || features.isDirty)
    }

    private var registrationSection: some View {
        VStack(alignment: .leading, spacing: 5) {
            Divider()
            Text("Body registration · proposal").font(.caption.bold())
            if let feature = features.active, feature.hasAnchor {
                let a = feature.anchor
                Text(
                    "Forward \(a.xM, specifier: "%.2f") m · left \(a.yM, specifier: "%.2f") m · ± \(a.boundM, specifier: "%.2f") m"
                ).font(.caption.monospacedDigit())
                Text(
                    "Height unresolved · body \(a.bodyID) · physical revision \(a.physicalRevision)"
                ).font(.caption2).textSelection(.enabled)
                Text(a.identityNote).font(.caption2)
                Button("Detach registration, keep facet") {
                    Task { await features.registerAnchor(nil, author: session.operatorName) }
                }.disabled(!features.canEdit || features.isDirty)
                Text(
                    "Detach before revising this registration's source support. Its previous mapping remains in revision history."
                ).font(.caption2).foregroundStyle(.secondary)
                if let state = session.physical.state, state.digest != a.physicalDigest {
                    Text(
                        "The physical reference has changed. This mapping keeps its original body frame; recheck before using it."
                    ).font(.caption2).foregroundStyle(.orange)
                }
            }
            Text(
                "For a compact, repeatable corner or protrusion only. Save its support in two frames, and review the object's body and resolved pose in Physical. A side patch's changing centre is not a point anchor."
            ).font(.caption2).foregroundStyle(.secondary)
            if let observation = session.featureOverlay, !features.isDirty,
                observation.decision == .acceptedProposal
            {
                Picker("Fixed return", selection: $features.registrationPoint) {
                    Text("Choose its physical spot").tag(UInt32?.none)
                    ForEach(observation.pointIndices, id: \.self) { index in
                        Text("Return \(index)").tag(UInt32?.some(index))
                    }
                }
                Button("Use pinned return") { session.usePinnedFacetReturn() }.disabled(
                    inspection.pinned == nil)
                Text(
                    "Turn on Inspect returns, hover the physical spot, then press M to pin it. The chosen point is yellow in both views."
                ).font(.caption2).foregroundStyle(.secondary)
                TextField("Repeatable spot, such as outer mirror tip", text: $identityNote)
                TextField("Return uncertainty ± m", value: $returnBoundM, format: .number)
                Button("Register to saved body") {
                    guard let feature = features.active, let sample = session.currentSample,
                        let point = features.registrationPoint,
                        let physical = session.physical.state, let bound = returnBoundM
                    else { return }
                    do {
                        let anchor = try FacetBodyRegistration.make(
                            feature: feature, sampleID: sample.sampleID, pointIndex: point,
                            points: session.currentPoints, physical: physical, returnBoundM: bound,
                            identityNote: identityNote)
                        Task { await features.registerAnchor(anchor, author: session.operatorName) }
                    } catch { features.message = error.localizedDescription }
                }.disabled(
                    !features.canEdit || session.physical.isDirty
                        || features.registrationPoint == nil || returnBoundM == nil
                        || session.physical.state == nil)
            }
            Text(
                "The saved offset stays metric when dimensions change. This seed is a proposal; it does not move the online tracker."
            ).font(.caption2).foregroundStyle(.secondary)
        }.task { session.startPhysicalIfNeeded() }.onChange(of: features.activeID) { _, _ in
            features.registrationPoint = nil
            identityNote = ""
            returnBoundM = nil
        }.onChange(of: session.sampleIndex) { _, _ in features.registrationPoint = nil }
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
        let feature = session.features.active
        let chosen =
            session.features.registrationPoint
            ?? (feature?.hasAnchor == true && feature?.anchor.sourceSample == observation?.sampleID
                ? feature?.anchor.sourcePointIndex : nil)
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
            if let chosen, o.pointIndices.contains(chosen), let p = points.point(at: Int(chosen)),
                basis.shows(p)
            {
                let at = viewport.screenPoint(from: basis.project(p))
                context.fill(
                    Path(CGRect(x: at.x - 4, y: at.y - 4, width: 8, height: 8)),
                    with: .color(.yellow))
                context.draw(
                    Text("fixed return \(chosen)").font(.system(size: 10)).foregroundColor(.yellow),
                    at: CGPoint(x: at.x, y: at.y - 13))
            }
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
