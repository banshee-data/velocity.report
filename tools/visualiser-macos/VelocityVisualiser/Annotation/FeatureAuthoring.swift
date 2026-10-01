// Feature proposals use the recording-domain protobuf and the local Go writer.
// They never edit whole-object masks or imply reviewed physical geometry.
import Combine
import Foundation
import SwiftProtobuf
import simd

typealias FeatureDocument = Velocity_Recording_V1_FeatureAnnotations
typealias FeatureCandidate = Velocity_Recording_V1_FeatureCandidate
typealias FeatureObservation = Velocity_Recording_V1_FeatureObservation
typealias FeatureSphere = Velocity_Recording_V1_FeatureSphere
typealias FeatureState = Velocity_Recording_V1_FeatureState
typealias FeatureDecision = Velocity_Recording_V1_FeatureDecision
typealias FeatureGeometry = Velocity_Recording_V1_FeatureGeometry

extension FeatureDocument {
    /// Read unknown fields for inspection, but never replace a newer contract.
    var knownEditingContract: Bool {
        guard unknownFields.data.isEmpty else { return false }
        return features.allSatisfy { f in
            f.unknownFields.data.isEmpty
                && [FeatureGeometry.unknown, .edge, .corner, .patch, .protrusion].contains(
                    f.geometry)
                && (f.partRelation == "unknown" || f.partRelation == "rigid_proposal")
                && (!f.hasAnchor
                    || (f.anchor.unknownFields.data.isEmpty
                        && f.anchor.coordinateDomain == "body_xy"
                        && f.anchor.method == "manual_named_return_v1"))
                && f.observations.allSatisfy { o in
                    o.unknownFields.data.isEmpty && o.sphere.unknownFields.data.isEmpty
                        && [FeatureDecision.acceptedProposal, .rejected, .missing, .occluded]
                            .contains(o.decision)
                }
        }
    }
}

extension FeatureSphere {
    var centre: SIMD3<Float> { SIMD3(Float(xM), Float(yM), Float(zM)) }
    init(centre: SIMD3<Float>, radius: Double) {
        self.init()
        xM = Double(centre.x)
        yM = Double(centre.y)
        zM = Double(centre.z)
        radiusM = radius
    }
}

/// Pure selection rules shared by clicking, resizing, and sequential previews.
enum FeatureSelection {
    static func indices(points: PackPoints, domain: [Int], sphere: FeatureSphere) -> [UInt32] {
        guard sphere.radiusM.isFinite, sphere.radiusM > 0, sphere.radiusM <= 5 else { return [] }
        return Set(domain).sorted().compactMap { i in
            guard i >= 0, let p = points.point(at: i), p.x.isFinite, p.y.isFinite, p.z.isFinite,
                simd_distance_squared(p, sphere.centre) <= Float(sphere.radiusM * sphere.radiusM)
            else { return nil }
            return UInt32(i)
        }
    }

    static func centre(points: PackPoints, domain: [Int]) -> SIMD3<Float>? {
        let values = domain.compactMap { points.point(at: $0) }.filter {
            $0.x.isFinite && $0.y.isFinite && $0.z.isFinite
        }
        guard !values.isEmpty else { return nil }
        return values.reduce(SIMD3<Float>.zero, +) / Float(values.count)
    }

    /// A small translation-only search within the SAME saved object's domain.
    /// It borrows the existing footprint matcher, never its automatic mask writes.
    static func propose(
        source: FeatureObservation, sourcePoints: PackPoints, sourceDomain: [Int],
        targetPoints: PackPoints, targetDomain: [Int], seconds: Double
    ) throws -> FeatureSphere {
        guard seconds > 0, seconds <= 0.5 else {
            throw FeatureError.message("Recording gap: seed this frame manually")
        }
        guard source.pointIndices.count >= 2,
            let a = centre(points: sourcePoints, domain: sourceDomain),
            let b = centre(points: targetPoints, domain: targetDomain)
        else { throw FeatureError.message("Sparse or missing object support: seed manually") }
        let shift = b - a
        guard simd_length(shift) <= Float(40 * seconds + 0.5) else {
            throw FeatureError.message("Object movement is implausible: propagation stopped")
        }
        let footprint = SelectionFootprint(
            points: source.pointIndices.compactMap { sourcePoints.point(at: Int($0)) }, dilation: 0)
        let fit = footprint.fit(
            in: targetPoints, near: shift, searchRadius: 0.25,
            rivalDistance: max(SelectionFootprint.pitch, Float(source.sphere.radiusM)),
            among: targetDomain, where: { _ in true })
        guard fit.count >= 2, Float(fit.runnerUp) < Float(fit.count) * 0.8 else {
            throw FeatureError.message("Local geometry is sparse or ambiguous: seed manually")
        }
        let sphere = FeatureSphere(
            centre: source.sphere.centre + fit.offset, radius: source.sphere.radiusM)
        let found = indices(points: targetPoints, domain: targetDomain, sphere: sphere).count
        guard found >= max(2, source.pointIndices.count / 2), found <= source.pointIndices.count * 2
        else { throw FeatureError.message("Feature support changed too much: seed manually") }
        return sphere
    }
}

enum FeatureSelectionTool: String, CaseIterable {
    case sphere
    case lasso
}

enum FeatureError: Error, LocalizedError {
    case message(String)
    var errorDescription: String? {
        if case .message(let text) = self { return text }
        return nil
    }
}

/// Transport stays local-first, with the same pack root as physical authoring.
struct FeatureAPIClient {
    var baseURL = PhysicalReferenceAPIClient.defaultBaseURL
    var session = APISession.shared

    func request(
        pack: AnnotationPack, edit: Velocity_Recording_V1_FeatureEdit? = nil
    ) async throws -> FeatureState {
        var url = URLComponents(
            url: baseURL.appendingPathComponent("api/annotations/features"),
            resolvingAgainstBaseURL: false)!
        url.queryItems = [
            URLQueryItem(
                name: "pack", value: PhysicalReferenceAPIClient.handle(for: pack.directory)),
            URLQueryItem(name: "pack_digest", value: pack.manifest.packDigest),
        ]
        var request = URLRequest(url: url.url!)
        if let edit {
            request.httpMethod = "POST"
            request.httpBody = try edit.serializedData()
            request.setValue("application/x-protobuf", forHTTPHeaderField: "Content-Type")
        }
        let (bytes, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            let error = (try? JSONSerialization.jsonObject(with: bytes)) as? [String: String]
            throw FeatureError.message(error?["error"] ?? "Feature service refused the request")
        }
        let state = try FeatureState(serializedBytes: bytes)
        guard state.document.schema == "velocity.report/feature-proposals",
            state.document.schemaVersion == 1,
            state.document.packDigest == pack.manifest.packDigest,
            state.document.datasetID == pack.manifest.datasetID,
            state.document.pointDomain == "legacy_pack"
        else { throw FeatureError.message("Unsupported feature document or wrong pack") }
        guard
            URL(fileURLWithPath: state.packDirectory).resolvingSymlinksInPath().standardizedFileURL
                == pack.directory.resolvingSymlinksInPath().standardizedFileURL
        else {
            throw FeatureError.message(
                "The service's copy is a different folder; open its pack before editing")
        }
        return state
    }
}

@MainActor final class FeatureAuthoring: ObservableObject {
    @Published private(set) var state: FeatureState?
    @Published var draft: FeatureObservation?
    @Published var activeID = ""
    @Published var name = "Feature"
    @Published var geometry: FeatureGeometry = .unknown
    @Published var semanticHint = ""
    @Published var radius = 0.2
    @Published var selectionTool: FeatureSelectionTool = .sphere
    @Published var registrationPoint: UInt32?
    @Published var message: String?
    @Published private(set) var busy = false
    @Published private(set) var readOnly = false
    private var draftObjectID: String?
    private let pack: AnnotationPack
    private let client: FeatureAPIClient

    init(pack: AnnotationPack, client: FeatureAPIClient = FeatureAPIClient()) {
        self.pack = pack
        self.client = client
    }

    var active: FeatureCandidate? { state?.document.features.first { $0.featureID == activeID } }
    var canEdit: Bool { state != nil && !busy && !readOnly }
    func activeCount(objectID: String) -> Int {
        state?.document.features.filter { $0.objectID == objectID && !$0.inactive }.count ?? 0
    }
    var isDirty: Bool { draft != nil }

    func load() async {
        guard !busy, draft == nil else { return }
        busy = true
        defer { busy = false }
        do {
            state = try await client.request(pack: pack)
            readOnly = state?.document.knownEditingContract != true
            message =
                readOnly
                ? "Read-only: this document contains feature fields or constraints this build cannot edit"
                : nil
        } catch {
            // Saved evidence can be inspected without a running service. Writes
            // still require Go's validator; a failed request never claims a save.
            if let bytes = try? Data(
                contentsOf: pack.directory.appendingPathComponent("feature-proposals.pb")),
                let document = try? FeatureDocument(serializedBytes: bytes),
                document.schema == "velocity.report/feature-proposals", document.schemaVersion == 1,
                document.packDigest == pack.manifest.packDigest,
                document.datasetID == pack.manifest.datasetID, document.pointDomain == "legacy_pack"
            {
                var cached = FeatureState()
                cached.document = document
                state = cached
            }
            readOnly = true
            message =
                "Read-only: start the local annotation service and reload. \(error.localizedDescription)"
        }
    }

    func choose(_ id: String) {
        guard draft == nil, !busy else { return }
        activeID = id
        registrationPoint = nil
        if let active {
            name = active.name
            geometry = active.geometry
            semanticHint = active.semanticHint
        }
    }

    func newFeature() {
        guard draft == nil, !busy else { return }
        activeID = ""
        registrationPoint = nil
        name = "Feature"
        geometry = .unknown
        semanticHint = ""
        message = "Click one of this object's saved returns to seed a sphere"
    }

    func seed(_ observation: FeatureObservation, objectID: String) {
        guard canEdit else { return }
        draft = observation
        draftObjectID = objectID
        if observation.hasSphere { radius = observation.sphere.radiusM }
        message =
            observation.hasProposedFromSample
            ? "Translation-only proposal: inspect both views, then accept or reject"
            : "Unsaved feature proposal: inspect both views before accepting"
    }

    func cancel() {
        guard !busy else { return }
        draft = nil
        draftObjectID = nil
        registrationPoint = nil
        if let active {
            name = active.name
            geometry = active.geometry
            semanticHint = active.semanticHint
        }
        message = "Propagation stopped; nothing saved"
    }

    func registerAnchor(_ anchor: FacetBodyAnchor?, author: String) async {
        guard canEdit, !isDirty, let active, let state else { return }
        guard !author.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            message = "Enter Labelled by before saving a body registration"
            return
        }
        var edit = Velocity_Recording_V1_FeatureEdit()
        edit.document = state.document
        guard let i = edit.document.features.firstIndex(where: { $0.featureID == active.featureID })
        else { return }
        if let anchor {
            edit.document.features[i].anchor = anchor
            edit.document.features[i].partRelation = "rigid_proposal"
        } else {
            edit.document.features[i].clearAnchor()
            edit.document.features[i].partRelation = "unknown"
        }
        edit.document.author = author
        edit.baseDigest = state.digest
        edit.membershipDigest = state.membershipDigest
        busy = true
        defer { busy = false }
        do {
            self.state = try await client.request(pack: pack, edit: edit)
            message =
                anchor == nil
                ? "Registration detached; facet observations and history are retained"
                : "Saved horizontal body registration proposal; it does not move a tracker or review a pose"
        } catch {
            readOnly = true
            message =
                "Registration not confirmed. Reload before retrying. \(error.localizedDescription)"
        }
    }

    func setInactive(_ inactive: Bool, author: String) async {
        guard canEdit, !isDirty, let active, let state else { return }
        guard !author.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            message = "Enter Labelled by before changing active facets"
            return
        }
        if !inactive && activeCount(objectID: active.objectID) >= 4 {
            message = "Four facets are active. Retire one before activating another."
            return
        }
        var edit = Velocity_Recording_V1_FeatureEdit()
        edit.document = state.document
        guard let i = edit.document.features.firstIndex(where: { $0.featureID == active.featureID })
        else { return }
        edit.document.features[i].inactive = inactive
        edit.document.author = author
        edit.baseDigest = state.digest
        edit.membershipDigest = state.membershipDigest
        busy = true
        defer { busy = false }
        do {
            self.state = try await client.request(pack: pack, edit: edit)
            message = inactive ? "Facet retired; its observations are retained" : "Facet activated"
        } catch {
            readOnly = true
            message = "Change not confirmed. Reload before retrying. \(error.localizedDescription)"
        }
    }

    func save(decision: FeatureDecision, author: String, membershipDigest: String) async -> Bool {
        guard canEdit, var observation = draft, let objectID = draftObjectID, let state else {
            return false
        }
        guard !author.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            message = "Enter Labelled by before saving a feature"
            return false
        }
        guard membershipDigest == state.membershipDigest else {
            message = "Membership changed: cancel the proposal, reload features, and reseed"
            return false
        }
        observation.decision = decision
        observation.author = author
        if decision != .acceptedProposal {
            observation.pointIndices = []
            observation.uncertainIndices = []
        }
        var doc = state.document
        var feature = active ?? FeatureCandidate()
        if feature.featureID.isEmpty {
            feature.featureID = UUID().uuidString.lowercased()
            feature.objectID = objectID
            feature.partID = "body"
            feature.partRelation = "unknown"
        }
        guard feature.objectID == objectID else {
            message = "Choose a feature of this object"
            return false
        }
        if !feature.inactive, active == nil, activeCount(objectID: objectID) >= 4 {
            message = "Four facets are active. Retire one before saving a new facet."
            return false
        }
        feature.name = name
        feature.geometry = geometry
        feature.semanticHint = semanticHint
        feature.observations.removeAll { $0.sampleID == observation.sampleID }
        feature.observations.append(observation)
        feature.observations.sort { $0.sampleID < $1.sampleID }
        if let index = doc.features.firstIndex(where: { $0.featureID == feature.featureID }) {
            doc.features[index] = feature
        } else {
            doc.features.append(feature)
        }
        doc.author = author
        var edit = Velocity_Recording_V1_FeatureEdit()
        edit.document = doc
        edit.baseDigest = state.digest
        edit.membershipDigest = membershipDigest
        busy = true
        defer { busy = false }
        do {
            self.state = try await client.request(pack: pack, edit: edit)
            activeID = feature.featureID
            draft = nil
            draftObjectID = nil
            message = "Saved feature proposal; membership and physical review unchanged"
            return true
        } catch {
            // A transport error may follow a successful commit. Require a
            // deliberate reload; never retry or rebase the retained draft.
            readOnly = true
            message =
                "Save not confirmed. Draft retained; cancel and reload before retrying. \(error.localizedDescription)"
            return false
        }
    }
}

extension AnnotationSession {
    func usePinnedFacetReturn() {
        guard workMode == .features, !features.isDirty, let sample = currentSample,
            let pinned = inspection.pinned, pinned.sampleID == sample.sampleID,
            pinned.pointIndex >= 0, pinned.pointIndex <= Int(UInt32.max),
            featureOverlay?.pointIndices.contains(UInt32(pinned.pointIndex)) == true
        else {
            features.message =
                "Inspect and pin a return belonging to this saved facet in this frame"
            return
        }
        features.registrationPoint = UInt32(pinned.pointIndex)
    }

    func featureDomain(sampleID: Int, objectID: String) -> [Int] {
        let masks = sidecar.masks.filter { $0.sampleID == sampleID && $0.status != .rejected }
        guard let mask = masks.first(where: { $0.objectID == objectID }) else { return [] }
        let excluded = Set(
            (mask.uncertainIndices ?? [])
                + masks.filter { $0.objectID != objectID }.flatMap {
                    $0.pointIndices + ($0.uncertainIndices ?? [])
                })
        return mask.pointIndices.filter { !excluded.contains($0) }
    }

    /// A lasso selects exact canonical indices inside the saved definite object
    /// mask and current depth slab. It never changes whole-object membership.
    func selectFacet(polygon: SelectionPolygon, mode: SelectionMode = .replace) {
        guard features.canEdit, let object = activeObjectID, let sample = currentSample,
            !dirtySamples.contains(sample.sampleID),
            features.active?.objectID == nil || features.active?.objectID == object
        else { return }
        let domain = Set(featureDomain(sampleID: sample.sampleID, objectID: object))
        let candidates = PointSelectionEngine.candidates(
            points: currentPoints, basis: editingBasis, polygon: polygon, slab: slab)
        let selected = candidates.indices.filter { domain.contains($0) }
        let previous =
            features.draft?.sampleID == UInt32(sample.sampleID)
            ? Set(features.draft?.pointIndices.map(Int.init) ?? []) : []
        let indices = PointSelectionEngine.apply(mode: mode, candidates: selected, to: previous)
            .sorted()
        guard !indices.isEmpty else {
            features.cancel()
            features.message = "No facet points selected; object membership is unchanged"
            return
        }
        guard let centre = FeatureSelection.centre(points: currentPoints, domain: indices) else {
            return
        }
        let radius =
            indices.compactMap { currentPoints.point(at: $0) }.map {
                Double(simd_distance($0, centre))
            }.max() ?? 0
        guard radius.isFinite, radius < 5 else {
            features.message = "Select a smaller facet: its support must fit within a 5 m radius"
            return
        }
        var observation = featureObservation(sample: sample)
        observation.pointIndices = indices.map(UInt32.init)
        observation.sphere = FeatureSphere(centre: centre, radius: max(0.02, radius + 0.00001))
        observation.method = "manual_lasso"
        if features.draft?.origin == "assisted_proposal"
            || features.active?.observations.contains(where: { $0.origin == "assisted_proposal" })
                == true
        {
            observation.origin = "assisted_proposal"
        }
        features.seed(observation, objectID: object)
        features.message =
            "\(indices.count) facet points · \(candidates.excludedBySlab) outside depth slab · inspect both views before saving"
    }

    func seedFeature(
        in standard: OrthoViewBasis.Standard, viewport: OrthoViewport, at location: CGPoint
    ) {
        guard features.selectionTool == .sphere, features.canEdit, let object = activeObjectID,
            let sample = currentSample, !dirtySamples.contains(sample.sampleID), !features.busy
        else { return }
        if let active = features.active, active.objectID != object {
            features.message = "Choose New feature for this object"
            return
        }
        let domain = featureDomain(sampleID: sample.sampleID, objectID: object)
        let allowed = Set(domain)
        let hits = IntensityPicker.candidates(
            points: currentPoints, classes: currentClasses, basis: basis(standard),
            viewport: viewport, at: location, radius: 12, isVisible: { allowed.contains($0) })
        guard let hit = hits.first, let point = currentPoints.point(at: hit.index) else {
            features.message = "Click a saved, definite return of the active object"
            return
        }
        var observation = featureObservation(sample: sample)
        observation.sphere = FeatureSphere(centre: point, radius: features.radius)
        observation.pointIndices = FeatureSelection.indices(
            points: currentPoints, domain: domain, sphere: observation.sphere)
        observation.method = "manual_sphere"
        if let previous = features.draft, previous.hasProposedFromSample {
            observation.proposedFromSample = previous.proposedFromSample
            observation.method = "object_translation_edited"
            observation.origin = "assisted_proposal"
        }
        if features.active?.observations.contains(where: { $0.origin == "assisted_proposal" })
            == true
        {
            observation.origin = "assisted_proposal"
        }
        features.seed(observation, objectID: object)
    }

    /// Explicitly recheck saved support against the current object's saved mask.
    func editCurrentFeature() {
        guard features.canEdit, !features.isDirty, let object = activeObjectID,
            features.active?.objectID == object, let sample = currentSample,
            !dirtySamples.contains(sample.sampleID), let saved = featureOverlay, saved.hasSphere
        else { return }
        var observation = featureObservation(sample: sample)
        observation.sphere = saved.sphere
        let domain = featureDomain(sampleID: sample.sampleID, objectID: object)
        if saved.method == "manual_lasso" {
            let allowed = Set(domain.map(UInt32.init))
            observation.pointIndices = saved.pointIndices.filter { allowed.contains($0) }
            observation.method = "manual_lasso"
        } else {
            observation.pointIndices = FeatureSelection.indices(
                points: currentPoints, domain: domain, sphere: saved.sphere)
            observation.method = "manual_sphere_edit"
        }
        if saved.origin == "assisted_proposal" { observation.origin = saved.origin }
        features.seed(observation, objectID: object)
    }

    /// Absence must be recordable without clicking an imaginary return.
    func stageFeatureAbsence() {
        guard features.canEdit, !features.isDirty, let object = activeObjectID,
            features.active?.objectID == object, let sample = currentSample,
            !dirtySamples.contains(sample.sampleID)
        else { return }
        var observation = featureObservation(sample: sample)
        observation.method = "manual_unobserved"
        features.seed(observation, objectID: object)
    }

    func resizeFeature(_ radius: Double) {
        guard features.canEdit, var draft = features.draft, draft.method != "manual_lasso",
            let object = activeObjectID,
            draft.sampleID == currentSample.map({ UInt32($0.sampleID) })
        else { return }
        features.radius = min(max(radius, 0.02), 5)
        draft.sphere.radiusM = features.radius
        draft.pointIndices = FeatureSelection.indices(
            points: currentPoints,
            domain: featureDomain(sampleID: Int(draft.sampleID), objectID: object),
            sphere: draft.sphere)
        if draft.hasProposedFromSample { draft.method = "object_translation_edited" }
        features.draft = draft
    }

    func featureObservation(sample: AnnotationSample) -> FeatureObservation {
        var o = FeatureObservation()
        o.sampleID = UInt32(sample.sampleID)
        o.sourceOrdinal = UInt32(sample.sourceOrdinal)
        o.timestampNs = sample.timestampNs
        o.membershipRevision = UInt64(sidecar.revision)
        o.membershipDigest = membershipDigest
        o.author = operatorName
        let assisted =
            activeObjectID.map { object in
                physical.exposure[object] != nil
                    || sidecar.masks.contains {
                        $0.objectID == object && $0.sampleID == sample.sampleID
                            && $0.provenance.algorithm != nil
                    }
            } ?? false
        o.origin = assisted ? "assisted_proposal" : "human_proposal"
        return o
    }

    func proposeNextFeature() {
        guard features.canEdit, !features.isDirty, let feature = features.active,
            feature.objectID == activeObjectID, let sample = currentSample,
            let source = feature.observations.first(where: {
                $0.sampleID == UInt32(sample.sampleID) && $0.decision == .acceptedProposal
            }), sampleIndex + 1 < samples.count
        else {
            features.message = "Save an accepted proposal in this frame first"
            return
        }
        let target = samples[sampleIndex + 1]
        guard target.sourceOrdinal == sample.sourceOrdinal + 1,
            !feature.observations.contains(where: { $0.sampleID == UInt32(target.sampleID) })
        else {
            features.message = "Next frame is skipped or already annotated: propagation stopped"
            return
        }
        guard source.membershipDigest == membershipDigest else {
            features.message =
                "Membership changed since this feature: inspect and reseed before propagation"
            return
        }
        do {
            let targetPoints = try pack.points(sampleID: target.sampleID)
            let targetDomain = featureDomain(sampleID: target.sampleID, objectID: feature.objectID)
            let sphere = try FeatureSelection.propose(
                source: source, sourcePoints: currentPoints,
                sourceDomain: featureDomain(sampleID: sample.sampleID, objectID: feature.objectID),
                targetPoints: targetPoints, targetDomain: targetDomain,
                seconds: Double(target.timestampNs - sample.timestampNs) / 1e9)
            guard stepForward() == nil else {
                features.message = "Save other edits before stepping"
                return
            }
            var observation = featureObservation(sample: target)
            observation.sphere = sphere
            observation.pointIndices = FeatureSelection.indices(
                points: targetPoints, domain: targetDomain, sphere: sphere)
            observation.proposedFromSample = source.sampleID
            observation.method = "object_mask_translation_v1"
            observation.origin = "assisted_proposal"
            features.seed(observation, objectID: feature.objectID)
        } catch { features.message = error.localizedDescription }
    }

    var featureOverlay: FeatureObservation? {
        guard workMode == .features, let sample = currentSample else { return nil }
        if let draft = features.draft, draft.sampleID == UInt32(sample.sampleID) { return draft }
        guard features.active?.objectID == activeObjectID else { return nil }
        return features.active?.observations.first { $0.sampleID == UInt32(sample.sampleID) }
    }
}
