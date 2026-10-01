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
            Text("Facets · same part, fresh returns").font(.headline)
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
                Button(features.viewingRevision == nil ? "Reload latest" : "Return to latest") {
                    Task { await features.load() }
                }.disabled(features.isDirty || features.busy)
                Button("New facet") { features.newFeature() }.disabled(
                    !features.canEdit || features.isDirty || session.activeObjectID == nil
                        || features.activeCount(objectID: session.activeObjectID ?? "") >= 4
                ).help(
                    "Keep at most four active facets per object. Retire one to free a slot; its evidence stays saved."
                )
            }
            if features.headRevision > 0 {
                Menu("Inspect retained revision") {
                    let first = features.headRevision > 19 ? features.headRevision - 19 : 1
                    ForEach(Array(first...features.headRevision).reversed(), id: \.self) {
                        revision in
                        Button("Revision \(revision)") {
                            Task { await features.load(revision: revision) }
                        }
                    }
                }.disabled(features.isDirty || features.busy)
                Text(
                    "Recent revisions preserve the original facet subsets and source pins. Inspection never changes today's object mask."
                ).font(.caption2).foregroundStyle(.secondary)
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
                            Text(feature.name.isEmpty ? "Unnamed facet" : feature.name)
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
                let supported = active.observations.filter { $0.decision == .acceptedProposal }
                    .count
                Text(
                    "\(supported) supported frames · \(active.observations.count - supported) absence/rejection decisions"
                ).font(.caption2).foregroundStyle(.secondary)
                Menu("Facet frames: \(active.observations.count) decisions") {
                    ForEach(
                        active.observations.sorted { $0.sampleID < $1.sampleID }, id: \.sampleID
                    ) { observation in
                        Button(
                            "Frame \(session.pack.samples.first(where: { $0.sampleID == Int(observation.sampleID) })?.sourceOrdinal ?? Int(observation.sampleID)) · \(decisionLabel(observation.decision)) · \(observation.pointIndices.count) returns"
                        ) { session.goToFacetObservation(sampleID: observation.sampleID) }
                    }
                }.disabled(features.isDirty || features.busy)
                HStack {
                    Button("Previous facet frame") {
                        if let observation = session.adjacentFacetObservation(forward: false) {
                            session.goToFacetObservation(sampleID: observation.sampleID)
                        }
                    }.disabled(
                        features.isDirty || features.busy
                            || session.adjacentFacetObservation(forward: false) == nil)
                    Button("Next facet frame") {
                        if let observation = session.adjacentFacetObservation(forward: true) {
                            session.goToFacetObservation(sampleID: observation.sampleID)
                        }
                    }.disabled(
                        features.isDirty || features.busy
                            || session.adjacentFacetObservation(forward: true) == nil)
                }
                Button(active.inactive ? "Activate facet" : "Retire facet, keep evidence") {
                    Task {
                        await features.setInactive(!active.inactive, author: session.operatorName)
                    }
                }.disabled(!features.canEdit || features.isDirty)
            }
            Button("Edit this frame") { session.editCurrentFeature() }.disabled(
                !features.canEdit || features.isDirty || session.featureOverlay?.hasSphere != true)
            TextField("Facet name, such as outer mirror", text: $features.name).disabled(
                !canEditMetadata)
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
            ).disabled(!features.canEdit || features.draft?.usesSubsetShape == true)
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
                let support = session.facetSupportSummary(observation: observation)
                if let fraction = support.fraction {
                    Text(
                        "Facet support: \(support.definiteCount) of \(support.objectCount) definite object returns (\(fraction * 100, specifier: "%.1f")%)"
                    ).font(.caption.monospacedDigit())
                }
                Text(support.explanation).font(.caption2).foregroundStyle(.secondary)
                let fit = session.facetGeometryFit(
                    indices: observation.pointIndices, geometry: features.geometry)
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
        let requirements = registrationRequirements
        return VStack(alignment: .leading, spacing: 5) {
            Divider()
            Text("Body registration · proposal").font(.caption.bold())
            if let feature = features.active, feature.hasAnchor {
                let a = feature.anchor
                if a.hasLine {
                    Text(
                        "Line normal (\(a.line.normalX, specifier: "%.2f"), \(a.line.normalY, specifier: "%.2f")) · offset \(a.line.offsetM, specifier: "%.2f") ± \(a.boundM, specifier: "%.2f") m"
                    ).font(.caption.monospacedDigit())
                    Text(
                        "Normal direction ± \(a.line.normalBoundRad * 180 / .pi, specifier: "%.1f")° · along-edge position unconstrained"
                    ).font(.caption2).foregroundStyle(.secondary)
                } else {
                    Text(
                        "Forward \(a.xM, specifier: "%.2f") m · left \(a.yM, specifier: "%.2f") m · ± \(a.boundM, specifier: "%.2f") m"
                    ).font(.caption.monospacedDigit())
                }
                Text(
                    "Height unresolved · body \(a.bodyID) · physical revision \(a.physicalRevision)"
                ).font(.caption2).textSelection(.enabled)
                Text(a.identityNote).font(.caption2)
                if a.origin == "tracker_seeded_proposal" {
                    Text(
                        "Tracker-assisted body relation · useful for assisted experiments, never independent reference truth."
                    ).font(.caption2).foregroundStyle(.orange)
                }
                Button("Project saved relation here · assisted") {
                    guard let sample = session.currentSample, !session.physical.isDirty,
                        !session.dirtySamples.contains(sample.sampleID)
                    else { return }
                    do {
                        let projection = try FacetBodyProjection.make(
                            feature: feature, sampleID: sample.sampleID,
                            timestampNs: sample.timestampNs,
                            packDigest: session.pack.manifest.packDigest,
                            physical: session.physical.state,
                            membershipDigest: session.membershipDigest)
                        if projection.assisted {
                            session.physical.expose(
                                objectIDs: [feature.objectID],
                                source: "Saved facet body relation " + feature.featureID)
                        }
                        features.showBodyPreview(
                            FacetBodyPreview(
                                featureID: feature.featureID, sampleID: sample.sampleID,
                                membershipDigest: session.membershipDigest, projection: projection),
                            objectID: feature.objectID)
                        features.message =
                            "Read-only Top preview. Subsequent facet support in this frame retains assistance."
                    } catch { features.message = error.localizedDescription }
                }.disabled(
                    features.busy || session.physical.isDirty
                        || session.currentSample.map { session.dirtySamples.contains($0.sampleID) }
                            != false
                )
                if let preview = session.currentFacetBodyPreview {
                    Button("Hide projected relation") { features.bodyPreview = nil }
                    Text(
                        "Purple dashed relation · fixed body mapping, no box/state update. Height unresolved."
                    ).font(.caption2).foregroundStyle(.secondary)
                    if let observation = session.featureOverlay,
                        observation.decision == .acceptedProposal
                    {
                        if let residuals = session.facetProjectionResiduals(
                            observation: observation), let mean = residuals.meanAbsoluteM,
                            let maximum = residuals.maximumAbsoluteM
                        {
                            Text(
                                "\(residuals.count) returns · mean \(mean, specifier: "%.3f") m · max \(maximum, specifier: "%.3f") m"
                            ).font(.caption.monospacedDigit())
                            Text(
                                preview.projection.isLine
                                    ? "Absolute normal distances; along-edge position is unconstrained. Alignment diagnostic, not accuracy."
                                    : "Horizontal distances to the fixed spot. Alignment diagnostic, not accuracy."
                            ).font(.caption2).foregroundStyle(.secondary)
                        }
                    }
                }
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
                "Save the same physical part's support in two frames, then review its body and resolved pose in Physical. A corner/protrusion uses a named spot. A straight edge uses two returns to define a line; its along-edge position stays unconstrained. Surface registration is not available."
            ).font(.caption2).foregroundStyle(.secondary)
            if let requirements {
                Label(
                    "\(requirements.supportedFrames) saved support frames · need at least 2",
                    systemImage: requirements.supportedFrames >= 2 ? "checkmark.circle" : "circle"
                ).font(.caption2)
                Label(
                    requirements.currentMembership && requirements.sourceSupported
                        ? "This frame's support uses the current saved object mask"
                        : "Save this frame's definite support against the current object mask",
                    systemImage: requirements.currentMembership && requirements.sourceSupported
                        ? "checkmark.circle" : "circle"
                ).font(.caption2)
                Label(
                    requirements.reviewedPose && requirements.namedAssistance
                        ? "Source body and resolved pose are reviewed with supported bounds"
                        : "In Physical, review the source body/pose, bounds and any named tracker assistance",
                    systemImage: requirements.reviewedPose && requirements.namedAssistance
                        ? "checkmark.circle" : "circle"
                ).font(.caption2)
                if !requirements.eligibleGeometry {
                    Text(
                        "Registration supports Corner, Protrusion or straight Edge. A Patch remains an unmapped proposal."
                    ).font(.caption2).foregroundStyle(.secondary)
                }
            }
            if session.physical.isDirty {
                Text("Save the Physical draft before registering.").font(.caption2).foregroundStyle(
                    .orange)
            }
            if let observation = session.featureOverlay, !features.isDirty,
                observation.decision == .acceptedProposal
            {
                let segment = features.geometry == .edge
                Picker(
                    segment ? "Segment start" : "Fixed return",
                    selection: $features.registrationPoint
                ) {
                    Text(segment ? "Choose direction start" : "Choose its physical spot").tag(
                        UInt32?.none)
                    ForEach(observation.pointIndices, id: \.self) { index in
                        Text("Return \(index)").tag(UInt32?.some(index))
                    }
                }
                Button("Use pinned return") { session.usePinnedFacetReturn() }.disabled(
                    inspection.pinned == nil)
                if segment {
                    Picker("Segment end", selection: $features.registrationEndPoint) {
                        Text("Choose direction end").tag(UInt32?.none)
                        ForEach(observation.pointIndices, id: \.self) { index in
                            Text("Return \(index)").tag(UInt32?.some(index))
                        }
                    }
                    Button("Use pinned return as end") {
                        session.usePinnedFacetReturn(segmentEnd: true)
                    }.disabled(inspection.pinned == nil)
                    Text(
                        "The two returns define direction only; they are not permanent endpoints. Choose widely separated returns on a real straight edge, not a scan boundary."
                    ).font(.caption2).foregroundStyle(.secondary)
                }
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
                        let anchor: FacetBodyAnchor
                        if segment {
                            guard let end = features.registrationEndPoint else { return }
                            anchor = try FacetBodyRegistration.makeSegment(
                                feature: feature, sampleID: sample.sampleID, startIndex: point,
                                endIndex: end, points: session.currentPoints, physical: physical,
                                returnBoundM: bound, identityNote: identityNote)
                        } else {
                            anchor = try FacetBodyRegistration.make(
                                feature: feature, sampleID: sample.sampleID, pointIndex: point,
                                points: session.currentPoints, physical: physical,
                                returnBoundM: bound, identityNote: identityNote)
                        }
                        Task { await features.registerAnchor(anchor, author: session.operatorName) }
                    } catch { features.message = error.localizedDescription }
                }.disabled(
                    !features.canEdit || session.physical.isDirty || requirements?.ready != true
                        || features.registrationPoint == nil || returnBoundM == nil
                        || session.physical.state == nil
                        || (segment
                            && (features.registrationEndPoint == nil
                                || features.registrationEndPoint == features.registrationPoint))
                )
            }
            Text(
                "The saved offset stays metric when dimensions change. This seed is a proposal; it does not move the online tracker."
            ).font(.caption2).foregroundStyle(.secondary)
        }.task { session.startPhysicalIfNeeded() }.onChange(of: features.activeID) { _, _ in
            features.registrationPoint = nil
            features.registrationEndPoint = nil
            identityNote = ""
            returnBoundM = nil
        }.onChange(of: session.sampleIndex) { _, _ in
            features.registrationPoint = nil
            features.registrationEndPoint = nil
        }
    }

    private var registrationRequirements: FacetRegistrationRequirements? {
        guard let feature = features.active, let sample = session.currentSample else { return nil }
        return .inspect(
            feature: feature, sampleID: sample.sampleID, physical: session.physical.state,
            membershipRevision: session.sidecar.revision, membershipDigest: session.membershipDigest
        )
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

    private var chosenReturns: [UInt32] {
        var chosen = [session.features.registrationPoint, session.features.registrationEndPoint]
            .compactMap { $0 }
        if chosen.isEmpty, let feature = session.features.active, feature.hasAnchor,
            feature.anchor.sourceSample == session.featureOverlay?.sampleID
        {
            let anchor = feature.anchor
            if anchor.hasLine {
                if anchor.line.hasSourceStartIndex { chosen.append(anchor.line.sourceStartIndex) }
                if anchor.line.hasSourceEndIndex { chosen.append(anchor.line.sourceEndIndex) }
            } else if anchor.hasSourcePointIndex {
                chosen.append(anchor.sourcePointIndex)
            }
        }
        return chosen
    }

    var body: some View {
        let observation = session.featureOverlay
        let points = session.currentPoints
        let draft = session.features.isDirty
        let feature = session.features.active
        let chosen = chosenReturns
        let segment = feature?.geometry == .edge
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
            for index in chosen where o.pointIndices.contains(index) {
                guard let p = points.point(at: Int(index)), basis.shows(p) else { continue }
                let at = viewport.screenPoint(from: basis.project(p))
                context.fill(
                    Path(CGRect(x: at.x - 4, y: at.y - 4, width: 8, height: 8)),
                    with: .color(.yellow))
                context.draw(
                    Text("\(segment ? "direction return" : "fixed return") \(index)").font(
                        .system(size: 10)
                    ).foregroundColor(.yellow), at: CGPoint(x: at.x, y: at.y - 13))
            }
            if segment, chosen.count == 2, let a = points.point(at: Int(chosen[0])),
                let b = points.point(at: Int(chosen[1])), basis.shows(a), basis.shows(b)
            {
                var line = Path()
                line.move(to: viewport.screenPoint(from: basis.project(a)))
                line.addLine(to: viewport.screenPoint(from: basis.project(b)))
                context.stroke(
                    line, with: .color(.yellow), style: StrokeStyle(lineWidth: 1.5, dash: [4, 3]))
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
