// PhysicalReferenceSession.swift
// The physical-reference draft for one open pack: what the service last
// returned, what the operator has changed since, undo, and the service's
// verdict on the draft.
//
// It is separate from AnnotationSession's membership state on purpose. A
// mask's review and a body's review are different judgements, saved to
// different files through different writers; one dirty flag for both would
// let saving membership look like saving a pose, or the reverse.
//
// Rules the operator cannot get round from here, because the service applies
// them whatever this sends: a save never reviews anything; review is a
// separate action on a saved record; a revised body is a new body and resets
// its keyframes' reviews; a stale base or changed membership is refused. This
// class makes those visible before the round trip, not instead of it.

import Combine
import Foundation

@MainActor final class PhysicalReferenceSession: ObservableObject {
    enum Availability: Equatable {
        case notLoaded
        case loading
        case ready
        /// The document opened, but this client may not edit it.
        case readOnly(String)
        /// Nothing can be shown or saved, and why.
        case unavailable(String)
    }

    @Published private(set) var availability: Availability = .notLoaded
    /// The service's last word on the pack: the saved document and its token.
    @Published private(set) var state: PhysicalPackState?
    /// The operator's working copy of the objects.
    @Published private(set) var draft: [PhysicalObject] = []
    /// The service's diagnostics for the current draft, when they are current.
    @Published private(set) var validation: PhysicalEditResult?
    @Published private(set) var busy = false
    @Published private(set) var lastError: String?
    @Published private(set) var lastErrorCode: String?
    @Published private(set) var lastNote: String?
    /// Set when a write may have committed without an answer, or the stored
    /// document moved underneath: another write waits for a reload.
    @Published private(set) var needsReload = false

    /// Retained revisions, newest first, once asked for.
    @Published private(set) var history: [PhysicalRevisionSummary] = []

    /// Objects whose tracker estimate the operator has been shown, and the
    /// estimate's identity. An edit to such an object's record afterwards is
    /// tracker-assisted, whatever mode it is made in; see applyExposure.
    @Published private(set) var exposure: [String: String] = [:]
    private let exposureDefaults: UserDefaults?
    private var exposureKey: String { "annotation.physicalExposure." + packDigest }

    /// The draft as it was when a drag began: the whole drag is one undo step.
    private var gestureStart: [PhysicalObject]?

    private var undoStack: [[PhysicalObject]] = []
    private var redoStack: [[PhysicalObject]] = []
    static let undoLimit = 64

    let packHandle: String
    let packDigest: String
    let localDirectory: URL
    let sessionID: String
    private let client: PhysicalReferenceAPIClient
    /// The membership sidecar digest the operator is looking at.
    private let membershipDigest: () -> String
    /// Who is editing: the operator name the membership pane holds.
    private let author: () -> String

    /// Bumped by every request and every edit, so an answer to an earlier
    /// question cannot land on a later draft.
    private var generation = 0
    private var validationTask: Task<Void, Never>?
    /// How long an edit waits for the next before asking for validation.
    var validationDelay: Duration = .milliseconds(250)

    init(
        packDirectory: URL, packDigest: String, sessionID: String,
        client: PhysicalReferenceAPIClient = PhysicalReferenceAPIClient(),
        membershipDigest: @escaping () -> String, author: @escaping () -> String,
        exposureDefaults: UserDefaults? = nil
    ) {
        self.exposureDefaults = exposureDefaults
        self.exposure =
            exposureDefaults?.dictionary(forKey: "annotation.physicalExposure." + packDigest)
            as? [String: String] ?? [:]
        self.localDirectory = packDirectory
        self.packHandle = PhysicalReferenceAPIClient.handle(for: packDirectory)
        self.packDigest = packDigest
        self.sessionID = sessionID
        self.client = client
        self.membershipDigest = membershipDigest
        self.author = author
    }

    // MARK: Derived

    var isDirty: Bool {
        guard let state else { return false }
        return draft != state.document.objects
    }

    var canEdit: Bool { availability == .ready && !busy }
    var canUndo: Bool { !undoStack.isEmpty }
    var canRedo: Bool { !redoStack.isEmpty }

    func object(_ id: String) -> PhysicalObject? { draft.first { $0.objectID == id } }
    func savedObject(_ id: String) -> PhysicalObject? { state?.document.object(id) }

    /// A record as saved, for deciding whether it can be reviewed.
    func savedBody(objectID: String) -> PhysicalBody? { savedObject(objectID)?.body }
    func savedKeyframe(objectID: String, sampleID: Int) -> PhysicalKeyframe? {
        savedObject(objectID)?.keyframe(sampleID: sampleID)
    }

    /// Records the stored document says no longer hold against membership.
    var staleProblems: [PhysicalLinkProblem] { state?.stale ?? [] }

    /// Reviewed records whose membership changed after their review.
    var driftProblems: [PhysicalLinkProblem] { state?.reviewDrift ?? [] }

    /// Whether a reviewed record is listed as drifted, so it may be reviewed
    /// again against the current membership.
    func hasDrift(record id: String?) -> Bool {
        guard let id else { return false }
        return driftProblems.contains { $0.record.contains("\"\(id)\"") }
    }

    // MARK: Loading

    /// Reads the current document. With `keepDraft`, the operator's working
    /// copy survives and is re-based on what was read: the deliberate way to
    /// reconcile after a conflict, with the draft shown against the new head.
    func load(keepDraft: Bool = false) async {
        generation &+= 1
        let asked = generation
        let keep = keepDraft && isDirty ? draft : nil
        busy = true
        if state == nil { availability = .loading }
        defer { if asked == generation { busy = false } }
        do {
            let loaded = try await client.load(handle: packHandle, packDigest: packDigest)
            guard asked == generation else { return }
            accept(loaded, draft: keep)
            needsReload = false
            clearError()
        } catch let error as PhysicalReferenceAPIError {
            guard asked == generation else { return }
            if state == nil { availability = .unavailable(Self.unavailableReason(error)) }
            report(error)
        } catch {
            guard asked == generation else { return }
            report(.transport(error.localizedDescription))
        }
    }

    private func accept(_ loaded: PhysicalPackState, draft kept: [PhysicalObject]?) {
        state = loaded
        draft = kept ?? loaded.document.objects
        if kept == nil {
            undoStack = []
            redoStack = []
        }
        validation = nil
        availability = Self.availability(of: loaded, localDirectory: localDirectory)
        if kept != nil { scheduleValidation() }
    }

    static func availability(of state: PhysicalPackState, localDirectory: URL) -> Availability {
        // The service writes its own copy of the pack. If that is not the
        // folder this window has open, saving would put references beside
        // different membership than the operator is looking at.
        let served = URL(fileURLWithPath: state.packDir).resolvingSymlinksInPath()
            .standardizedFileURL
        let local = localDirectory.resolvingSymlinksInPath().standardizedFileURL
        guard served.path == local.path else {
            return .unavailable(
                "The annotation service's copy of this pack is \(served.path), but this window "
                    + "opened \(local.path). Physical references are saved through the service, "
                    + "so open the pack from the service's annotation folder.")
        }
        guard state.document.editable else {
            return .readOnly(
                "This document is \(state.document.schema) version \(state.document.schemaVersion); "
                    + "this build edits version \(PhysicalReferenceDocument.supportedVersion) only."
            )
        }
        return .ready
    }

    static func unavailableReason(_ error: PhysicalReferenceAPIError) -> String {
        switch error.code {
        case "pack_not_found":
            return "The annotation service has no pack \"\(error.errorDescription ?? "")\". "
                + "Open a pack from the folder the service was started with (--lidar-annotation-dir)."
        case "not_configured":
            return "The annotation service was started without --lidar-annotation-dir."
        default: return error.errorDescription ?? "The annotation service is unavailable."
        }
    }

    // MARK: Editing

    /// Applies one change to the draft, as one undo step.
    func edit(_ change: (inout [PhysicalObject]) -> Void) {
        guard canEdit, gestureStart == nil else { return }
        var next = draft
        change(&next)
        PhysicalDraft.applyExposure(before: draft, after: &next, exposure: exposure)
        PhysicalDraft.canonicalise(&next)
        guard next != draft else { return }
        undoStack.append(draft)
        if undoStack.count > Self.undoLimit { undoStack.removeFirst() }
        redoStack = []
        draft = next
        generation &+= 1
        validation = nil
        lastNote = nil
        scheduleValidation()
    }

    // MARK: Gestures

    /// Starts a drag: the draft follows the pointer, and the whole drag is
    /// one undo step when it ends.
    func beginGesture() {
        guard canEdit, gestureStart == nil else { return }
        gestureStart = draft
    }

    /// Moves the draft with the pointer, from the state the drag began in.
    func updateGesture(_ change: (inout [PhysicalObject]) -> Void) {
        guard let start = gestureStart else { return }
        var next = start
        change(&next)
        PhysicalDraft.applyExposure(before: start, after: &next, exposure: exposure)
        PhysicalDraft.canonicalise(&next)
        draft = next
    }

    func endGesture() {
        guard let start = gestureStart else { return }
        gestureStart = nil
        guard draft != start else { return }
        undoStack.append(start)
        if undoStack.count > Self.undoLimit { undoStack.removeFirst() }
        redoStack = []
        generation &+= 1
        validation = nil
        lastNote = nil
        scheduleValidation()
    }

    var gestureInProgress: Bool { gestureStart != nil }

    /// A keyframe as it was when the current drag began.
    func gestureStartKeyframe(objectID: String, sampleID: Int) -> PhysicalKeyframe? {
        gestureStart?.first { $0.objectID == objectID }?.keyframe(sampleID: sampleID)
    }

    /// An object's body as it was when the current drag began.
    func gestureStartBody(objectID: String) -> PhysicalBody? {
        gestureStart?.first { $0.objectID == objectID }?.body
    }

    // MARK: Exposure

    /// Records that the operator has seen an estimate for these objects. It
    /// is kept for the pack across launches, and nothing here clears it.
    func expose(objectIDs: some Sequence<String>, source: String) {
        var changed = false
        for id in objectIDs where exposure[id] == nil {
            exposure[id] = source
            changed = true
        }
        if changed { exposureDefaults?.set(exposure, forKey: exposureKey) }
    }

    func undo() {
        guard canEdit, let previous = undoStack.popLast() else { return }
        redoStack.append(draft)
        draft = previous
        generation &+= 1
        validation = nil
        scheduleValidation()
    }

    func redo() {
        guard canEdit, let next = redoStack.popLast() else { return }
        undoStack.append(draft)
        draft = next
        generation &+= 1
        validation = nil
        scheduleValidation()
    }

    /// Drops the working copy for the saved document. Saved history is not
    /// touched: nothing was written.
    func discard() {
        guard let state else { return }
        generation &+= 1
        validationTask?.cancel()
        draft = state.document.objects
        undoStack = []
        redoStack = []
        validation = nil
        lastNote = nil
    }

    // MARK: Validation

    private func scheduleValidation() {
        validationTask?.cancel()
        guard isDirty else { return }
        let asked = generation
        let delay = validationDelay
        validationTask = Task { [weak self] in
            try? await Task.sleep(for: delay)
            guard !Task.isCancelled else { return }
            await self?.validate(generation: asked)
        }
    }

    /// Asks the service what it would make of the draft, without writing.
    func validateNow() async { await validate(generation: generation) }

    private func validate(generation asked: Int) async {
        guard asked == generation, let request = editRequest() else { return }
        do {
            let result = try await client.validate(request)
            guard asked == generation else { return }
            validation = result
        } catch let error as PhysicalReferenceAPIError {
            guard asked == generation else { return }
            report(error)
        } catch {}
    }

    private func editRequest() -> PhysicalReferenceAPIClient.EditRequest? {
        guard let state else { return nil }
        return PhysicalReferenceAPIClient.EditRequest(
            pack: packHandle, packDigest: packDigest, baseRevision: state.revision,
            baseDigest: state.digest, membershipDigest: membershipDigest(), author: author(),
            session: sessionID, objects: draft)
    }

    // MARK: Writing

    /// Saves the draft as a proposal. Returns true once the service has
    /// answered that it committed.
    @discardableResult func save() async -> Bool {
        guard canEdit, !needsReload else {
            if needsReload {
                lastError = "Reload before saving: the last write's outcome is not known."
            }
            return false
        }
        guard !author().trimmingCharacters(in: .whitespaces).isEmpty else {
            lastError =
                "Enter your name under Labelled by before saving: a reference has an author."
            return false
        }
        guard isDirty, let request = editRequest() else { return false }
        generation &+= 1
        validationTask?.cancel()
        busy = true
        defer { busy = false }
        do {
            let result = try await client.save(request)
            guard let saved = result.state else {
                needsReload = true
                lastError =
                    "The service saved but did not return the document. Reload before editing."
                return false
            }
            // A save answers for the draft that was sent. Anything typed while
            // it was in flight was refused by canEdit, so nothing is lost here.
            accept(saved, draft: nil)
            clearError()
            lastNote = Self.describeSave(result, revision: saved.revision)
            return true
        } catch let error as PhysicalReferenceAPIError {
            if case .invalid(let result) = error { validation = result }
            report(error)
            return false
        } catch {
            report(.transport(error.localizedDescription))
            return false
        }
    }

    /// Reviews one saved record. Refused while the draft differs from what is
    /// saved: review confirms a saved record, and an unsaved edit is not one.
    @discardableResult func review(
        kind: PhysicalRecordKind, objectID: String, recordID: String
    ) async -> Bool {
        guard canEdit, !needsReload, let state else { return false }
        guard !isDirty else {
            lastError = "Save or discard the draft first: a review confirms the saved record."
            return false
        }
        let reviewer = author()
        guard !reviewer.trimmingCharacters(in: .whitespaces).isEmpty else {
            lastError = "Enter your name under Labelled by before reviewing."
            return false
        }
        generation &+= 1
        busy = true
        defer { busy = false }
        do {
            let reviewed = try await client.review(
                PhysicalReferenceAPIClient.ReviewRequest(
                    pack: packHandle, packDigest: packDigest, baseRevision: state.revision,
                    baseDigest: state.digest, membershipDigest: membershipDigest(), kind: kind,
                    objectID: objectID, recordID: recordID, reviewer: reviewer, session: sessionID))
            accept(reviewed, draft: nil)
            clearError()
            lastNote = "Reviewed \(kind.rawValue) as revision \(reviewed.revision)."
            return true
        } catch let error as PhysicalReferenceAPIError {
            report(error)
            return false
        } catch {
            report(.transport(error.localizedDescription))
            return false
        }
    }

    /// A retained revision, for showing what a report was scored against.
    /// Not adopted as the draft: a report's revision is read, never edited.
    func loadRetained(revision: Int) async throws -> PhysicalPackState {
        try await client.load(handle: packHandle, packDigest: packDigest, revision: revision)
    }

    // MARK: History

    func loadHistory() async {
        do {
            history = try await client.history(handle: packHandle, packDigest: packDigest)
        } catch let error as PhysicalReferenceAPIError { report(error) } catch {}
    }

    /// Makes a retained revision current, as a new revision. Refused while
    /// the draft has unsaved changes: they would be lost under it.
    @discardableResult func restore(revision: Int) async -> Bool {
        guard canEdit, !needsReload, let state else { return false }
        guard !isDirty else {
            lastError = "Save or discard the draft before restoring a revision."
            return false
        }
        guard !author().trimmingCharacters(in: .whitespaces).isEmpty else {
            lastError = "Enter your name under Labelled by before restoring."
            return false
        }
        generation &+= 1
        busy = true
        defer { busy = false }
        do {
            let restored = try await client.restore(
                PhysicalReferenceAPIClient.RestoreRequest(
                    pack: packHandle, packDigest: packDigest, baseRevision: state.revision,
                    baseDigest: state.digest, membershipDigest: membershipDigest(),
                    revision: revision, author: author(), session: sessionID))
            accept(restored, draft: nil)
            clearError()
            lastNote = "Restored revision \(revision) as revision \(restored.revision)."
            await loadHistory()
            return true
        } catch let error as PhysicalReferenceAPIError {
            report(error)
            return false
        } catch {
            report(.transport(error.localizedDescription))
            return false
        }
    }

    /// Membership was saved, merged, split or reloaded: the links the service
    /// checks may have changed. A clean session re-reads; a dirty one keeps
    /// its draft and asks again what the service makes of it.
    func membershipDidChange() async {
        guard state != nil else { return }
        if isDirty {
            lastNote = "Membership changed; the draft is rechecked against it."
            await validateNow()
        } else {
            await load()
        }
    }

    // MARK: Errors

    /// Explains why an action was not taken, without asking the service.
    func refuse(_ message: String) {
        lastError = message
        lastErrorCode = nil
    }

    private func report(_ error: PhysicalReferenceAPIError) {
        lastError = error.errorDescription
        lastErrorCode = error.code
        switch error {
        case .transport:
            // The request may have landed. Writing again before reading could
            // overwrite a commit this client never heard about.
            needsReload = true
        case .refused(_, let code, _) where code == "conflict": needsReload = true
        default: break
        }
    }

    private func clearError() {
        lastError = nil
        lastErrorCode = nil
    }

    static func describeSave(_ result: PhysicalEditResult, revision: Int) -> String {
        var parts = ["Saved revision \(revision) as a proposal."]
        if !result.resetReviews.isEmpty {
            parts.append("Review reset on \(result.resetReviews.joined(separator: ", ")).")
        }
        if let renamed = result.renamedBodies, !renamed.isEmpty {
            parts.append(
                "Revised body saved as "
                    + renamed.sorted { $0.key < $1.key }.map { "\($0.value) (was \($0.key))" }
                    .joined(separator: ", ") + ".")
        }
        return parts.joined(separator: " ")
    }
}

// MARK: - Draft edits

/// The edits the pane makes, as pure functions of the draft, so each can be
/// tested without a session or a service.
enum PhysicalDraft {
    /// The order the service stores: objects by ID, keyframes by sample,
    /// support frames sorted. Keeping the draft in it means an unchanged draft
    /// compares equal to what was saved.
    static func canonicalise(_ objects: inout [PhysicalObject]) {
        objects.sort { $0.objectID < $1.objectID }
        for i in objects.indices { objects[i].keyframes.sort { $0.sampleID < $1.sampleID } }
    }

    static func newID(_ prefix: String) -> String {
        prefix + "_"
            + UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(12).lowercased()
    }

    /// A fresh independent record's review: proposed, manual, authored here.
    static func review(author: String, session: String) -> PhysicalReview {
        PhysicalReview(
            status: .proposed, origin: .independent, method: "manual_box", trackerSource: nil,
            uncertaintyAssumptions: nil,
            provenance: Provenance(
                author: author, session: session, createdUTC: SidecarStore.utcTimestamp(),
                operation: "author"))
    }

    static func index(of objectID: String, in objects: inout [PhysicalObject]) -> Int {
        if let i = objects.firstIndex(where: { $0.objectID == objectID }) { return i }
        objects.append(PhysicalObject(objectID: objectID))
        return objects.count - 1
    }

    /// A body with every dimension unknown. Nothing is prefilled: a typical
    /// car's size is not evidence about this one.
    static func addBody(
        objectID: String, author: String, session: String, to objects: inout [PhysicalObject]
    ) {
        let i = index(of: objectID, in: &objects)
        guard objects[i].body == nil else { return }
        objects[i].body = PhysicalBody(
            bodyID: newID("body"), review: review(author: author, session: session))
    }

    static func removeBody(objectID: String, from objects: inout [PhysicalObject]) {
        guard let i = objects.firstIndex(where: { $0.objectID == objectID }) else { return }
        objects[i].body = nil
        dropIfEmpty(i, &objects)
    }

    /// A keyframe at this sample with nothing known yet. It references its
    /// own sample and time, exactly, and no other.
    static func addKeyframe(
        objectID: String, sample: AnnotationSample, author: String, session: String,
        to objects: inout [PhysicalObject]
    ) {
        let i = index(of: objectID, in: &objects)
        guard objects[i].keyframe(sampleID: sample.sampleID) == nil else { return }
        objects[i].keyframes.append(
            PhysicalKeyframe(
                keyframeID: newID("kf"), sampleID: sample.sampleID, timestampNs: sample.timestampNs,
                review: review(author: author, session: session)))
    }

    static func removeKeyframe(
        objectID: String, sampleID: Int, from objects: inout [PhysicalObject]
    ) {
        guard let i = objects.firstIndex(where: { $0.objectID == objectID }) else { return }
        objects[i].keyframes.removeAll { $0.sampleID == sampleID }
        dropIfEmpty(i, &objects)
    }

    private static func dropIfEmpty(_ i: Int, _ objects: inout [PhysicalObject]) {
        if objects[i].body == nil && objects[i].keyframes.isEmpty { objects.remove(at: i) }
    }

    /// Mutates one keyframe in place.
    static func updateKeyframe(
        objectID: String, sampleID: Int, in objects: inout [PhysicalObject],
        _ change: (inout PhysicalKeyframe) -> Void
    ) {
        guard let i = objects.firstIndex(where: { $0.objectID == objectID }),
            let k = objects[i].keyframes.firstIndex(where: { $0.sampleID == sampleID })
        else { return }
        change(&objects[i].keyframes[k])
    }

    static func updateBody(
        objectID: String, in objects: inout [PhysicalObject], _ change: (inout PhysicalBody) -> Void
    ) {
        guard let i = objects.firstIndex(where: { $0.objectID == objectID }), objects[i].body != nil
        else { return }
        change(&objects[i].body!)
    }

    /// Places the keyframe's anchor where the operator clicked in the top
    /// view. Placing it on the returns is an observation at this sample; the
    /// bound is left for the operator to state, because the evidence slack is
    /// a validation rule and not a precision.
    static func place(
        objectID: String, sampleID: Int, x: Double, y: Double, in objects: inout [PhysicalObject]
    ) {
        updateKeyframe(objectID: objectID, sampleID: sampleID, in: &objects) { k in
            k.position.xM = x
            k.position.yM = y
            if k.position.status == .unknown {
                k.position.status = .observed
                k.position.support = PhysicalSupport(frames: [sampleID])
            }
        }
    }

    /// Sets an evidence status and makes the value fields agree with it:
    /// unknown carries nothing; anything else keeps what is there.
    static func setStatus(_ status: PhysicalEvidence, of dimension: inout PhysicalDimension) {
        dimension.status = status
        switch status {
        case .unknown: dimension = PhysicalDimension()
        case .observed: if dimension.span == nil { dimension.span = .full }
        case .inferred, .priorOnly:
            // Only an observation can be partial.
            dimension.span = .full
        }
    }

    /// Moves a full-span dimension to a new value, carrying its interval
    /// with it: the bounds keep their distances from the value they had (or
    /// from the interval's midpoint), so a drag revises the belief and not
    /// the stated uncertainty.
    static func setLength(
        _ value: Double, of dimension: inout PhysicalDimension, from start: PhysicalDimension
    ) {
        guard let lo = start.lowerM, let hi = start.upperM else { return }
        let was = start.valueM ?? (lo + hi) / 2
        dimension.valueM = value
        dimension.lowerM = max(value - (was - lo), 0)
        dimension.upperM = value + (hi - was)
    }

    static func setSpan(_ span: PhysicalSpan, of dimension: inout PhysicalDimension) {
        dimension.span = span
        if span == .partial {
            // A partial span is a lower bound: no upper bound, no value.
            dimension.upperM = nil
            dimension.valueM = nil
        }
    }

    static func setStatus(_ status: PhysicalEvidence, of position: inout PhysicalPosition) {
        position.status = status
        if status == .unknown { position = PhysicalPosition() }
    }

    /// Sets the axis state and keeps the keyframe consistent with it: an
    /// unknown axis carries no yaw, and an unresolved one names no bumper and
    /// no face.
    static func setAxis(_ axis: PhysicalAxisState, of k: inout PhysicalKeyframe) {
        k.yaw.axis = axis
        if axis == .unknown {
            k.yaw = PhysicalYaw(status: .unknown, axis: .unknown)
        } else if k.yaw.status == .unknown {
            k.yaw.status = .observed
            if k.yaw.support.frameList.isEmpty {
                k.yaw.support = PhysicalSupport(frames: [k.sampleID])
            }
        }
        if axis != .resolved {
            k.front = PhysicalEndpoint()
            k.rear = PhysicalEndpoint()
            if k.anchor.kind.isFace { k.anchor = PhysicalAnchor() }
        }
    }

    static func setStatus(_ status: PhysicalEvidence, of endpoint: inout PhysicalEndpoint) {
        endpoint.status = status
        if status == .unknown { endpoint = PhysicalEndpoint() }
    }

    static func setAnchor(_ kind: PhysicalAnchorKind, of k: inout PhysicalKeyframe) {
        k.anchor.kind = kind
        if kind == .bodyCentre {
            k.anchor.offsetM = nil
            k.anchor.offsetBoundM = nil
        }
    }
}

// MARK: - Copies, repair and exposure

extension PhysicalDraft {
    /// The object's keyframe nearest this sample in time, other than one at
    /// the sample itself.
    static func nearestKeyframe(
        of object: PhysicalObject, to sample: AnnotationSample
    ) -> PhysicalKeyframe? {
        object.keyframes.filter { $0.sampleID != sample.sampleID }.min {
            abs($0.timestampNs - sample.timestampNs) < abs($1.timestampNs - sample.timestampNs)
        }
    }

    /// A proposal at `sample` copied from another keyframe.
    ///
    /// A copy is not an observation at its new instant: every component the
    /// source observed becomes inferred from the frames it was observed in,
    /// and nothing is interpolated. It keeps the source's actual origin, so a
    /// copy of a tracker-assisted keyframe stays tracker-assisted. The
    /// operator then confirms or corrects it against this frame's returns,
    /// which is what may make any part of it observed again.
    static func copy(
        _ source: PhysicalKeyframe, to sample: AnnotationSample, author: String, session: String
    ) -> PhysicalKeyframe {
        func carried(
            _ status: PhysicalEvidence, _ support: PhysicalSupport
        ) -> (PhysicalEvidence, PhysicalSupport) {
            guard status == .observed else { return (status, support) }
            var frames = Set(support.frameList)
            frames.insert(source.sampleID)
            return (.inferred, PhysicalSupport(frames: frames.sorted(), external: support.external))
        }
        var k = source
        k.keyframeID = newID("kf")
        k.sampleID = sample.sampleID
        k.timestampNs = sample.timestampNs
        (k.position.status, k.position.support) = carried(
            source.position.status, source.position.support)
        (k.yaw.status, k.yaw.support) = carried(source.yaw.status, source.yaw.support)
        (k.front.status, k.front.support) = carried(source.front.status, source.front.support)
        (k.rear.status, k.rear.support) = carried(source.rear.status, source.rear.support)
        // A shared error names one observation at the source's instant.
        k.sharedErrors = nil
        var review = PhysicalDraft.review(author: author, session: session)
        review.origin = source.review.origin
        review.trackerSource = source.review.trackerSource
        review.method = "copy_of:" + source.keyframeID
        review.uncertaintyAssumptions =
            "Copied from sample \(source.sampleID), not observed at this instant. "
            + (source.review.uncertaintyAssumptions ?? "")
        k.review = review
        return k
    }

    /// Moves an object's references to another object, as a deliberate repair
    /// after a merge or a rejection. Refused, with the reason, where the
    /// target already has a body or a keyframe at the same sample: two answers
    /// to one question are for a person to choose between.
    static func reassign(
        from sourceID: String, to targetID: String, in objects: inout [PhysicalObject]
    ) -> String? {
        guard sourceID != targetID, let s = objects.firstIndex(where: { $0.objectID == sourceID })
        else { return "Nothing to move." }
        let source = objects[s]
        let t = index(of: targetID, in: &objects)
        if source.body != nil && objects[t].body != nil {
            return "Both objects have a body. Remove one before moving the references."
        }
        let taken = Set(objects[t].keyframes.map(\.sampleID))
        if let clash = source.keyframes.first(where: { taken.contains($0.sampleID) }) {
            return "Both objects have a keyframe at sample \(clash.sampleID). Remove one first."
        }
        if let body = source.body { objects[t].body = body }
        objects[t].keyframes += source.keyframes
        objects.removeAll { $0.objectID == sourceID }
        return nil
    }

    /// Removes the record a link problem names: an object's whole reference,
    /// its body, or one keyframe. The service names records as
    /// `object "<id>"`, optionally followed by `body "<id>"` or
    /// `keyframe "<id>"`.
    @discardableResult static func remove(
        problem: PhysicalLinkProblem, from objects: inout [PhysicalObject]
    ) -> Bool {
        let quoted = problem.record.split(separator: "\"", omittingEmptySubsequences: false).map(
            String.init)
        // object "<a>" [body|keyframe "<b>"] splits into ["object ", a, " body ", b, ""].
        guard quoted.count >= 3, quoted[0].trimmingCharacters(in: .whitespaces) == "object" else {
            return false
        }
        let objectID = quoted[1]
        guard let i = objects.firstIndex(where: { $0.objectID == objectID }) else { return false }
        if quoted.count >= 5 {
            let kind = quoted[2].trimmingCharacters(in: .whitespaces)
            let recordID = quoted[3]
            switch kind {
            case "body" where objects[i].body?.bodyID == recordID: objects[i].body = nil
            case "keyframe": objects[i].keyframes.removeAll { $0.keyframeID == recordID }
            default: return false
            }
            if objects[i].body == nil && objects[i].keyframes.isEmpty { objects.remove(at: i) }
            return true
        }
        objects.remove(at: i)
        return true
    }

    /// Holds an edit to what the operator has seen. For an object whose
    /// estimate was shown, any body or keyframe the edit changes or creates
    /// becomes tracker-assisted, naming the estimate. A record saved as
    /// independent is never relabelled: its revision stays in history, and
    /// the edit continues as a new tracker-assisted record under a new ID.
    static func applyExposure(
        before: [PhysicalObject], after: inout [PhysicalObject], exposure: [String: String]
    ) {
        guard !exposure.isEmpty else { return }
        let old = Dictionary(before.map { ($0.objectID, $0) }, uniquingKeysWith: { a, _ in a })
        func assist(_ review: inout PhysicalReview, source: String) {
            review.origin = .trackerAssisted
            review.trackerSource = source
        }
        for i in after.indices {
            guard let source = exposure[after[i].objectID] else { continue }
            let previous = old[after[i].objectID]
            if var body = after[i].body, body != previous?.body, body.review.origin == .independent
            {
                if previous?.body?.bodyID == body.bodyID { body.bodyID = newID("body") }
                assist(&body.review, source: source)
                after[i].body = body
            }
            for k in after[i].keyframes.indices {
                var kf = after[i].keyframes[k]
                let was = previous?.keyframes.first { $0.keyframeID == kf.keyframeID }
                guard kf != was, kf.review.origin == .independent else { continue }
                if was != nil { kf.keyframeID = newID("kf") }
                assist(&kf.review, source: source)
                after[i].keyframes[k] = kf
            }
        }
    }
}
