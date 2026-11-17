import Foundation

struct FollowingPendingSave {
    let runID: String
    let binding: FollowingAudit.Binding
    let record: FollowingAudit.Record
    let transport: FollowingReviewTransport
}

struct FollowingReviewDraft: Identifiable {
    var record: FollowingAudit.Record
    var endText: String?
    let runID: String
    let binding: FollowingAudit.Binding
    let evidence: FollowingEvidence
    let transport: FollowingReviewTransport
    let timeOrigin: Int64
    var id: UUID { record.id }
}

extension FollowingReviewSession {
    var isManaged: Bool { managedRunID != nil }

    func recordingChanged(to runID: String?) {
        guard let current = managedRunID, current != runID else { return }
        managedGeneration += 1
        failedManagedWindow = nil
        windowTask?.cancel()
        managedRunID = nil
        managedMetadata = nil
        managedGeometry = nil
        managedTransport = nil
        document = nil
        audit = nil
        times = []
        sampleIndices = [:]
        geometryIndex = [:]
        followerChoices = []
        follower = ""
        timestamp = ""
        loadingWindow = false
        savingRecord = false
        finishImport()
        error = nil
    }

    func openRecording(
        runID: String, at: Int64, transport: FollowingReviewTransport = FollowingReviewTransport()
    ) async {
        guard !loading || isManaged else {
            error = "Wait for the evidence import to finish before opening a recording review."
            return
        }
        guard !auditUnsaved, pendingSave == nil, managedDraft == nil else {
            error = "Save the outstanding review before opening another review."
            return
        }
        managedGeneration += 1
        failedManagedWindow = nil
        let generation = managedGeneration
        windowTask?.cancel()
        managedRunID = runID
        managedTransport = transport
        managedMetadata = nil
        managedGeometry = nil
        document = nil
        audit = nil
        evidenceURL = nil
        follower = ""
        times = []
        sampleIndices = [:]
        geometryIndex = [:]
        followerChoices = []
        timestamp = ""
        error = nil
        finishImport()
        _ = beginImport()
        defer { if managedGeneration == generation { finishImport() } }
        do {
            let metadata = try await transport.metadata(runID: runID)
            guard managedGeneration == generation else { return }
            managedMetadata = metadata
            fileIdentity = metadata.evidenceSha256
            let initial = max(metadata.startNanos, min(at, metadata.endExclusiveNanos - 1))
            timestamp = String(initial)
            try await loadManagedWindow(at: initial, generation: generation)
        } catch { if managedGeneration == generation { self.error = error.localizedDescription } }
    }

    func loadManagedWindow(at nanos: Int64, generation: Int) async throws {
        guard let transport = managedTransport, let metadata = managedMetadata,
            let runID = managedRunID
        else { return }
        loadingWindow = true
        var loaded = false
        defer {
            if managedGeneration == generation {
                if loaded {
                    failedManagedWindow = nil
                } else if !Task.isCancelled {
                    // Publish the refusal before loading ends: the view follows
                    // the latest playback time when this flag becomes false.
                    failedManagedWindow = managedWindowRange(at: nanos, metadata: metadata)
                }
                loadingWindow = false
            }
        }
        async let geometry = transport.geometry(runID: runID, at: nanos, metadata: metadata)
        let doc = try await transport.evidence(runID: runID, at: nanos, metadata: metadata)
        async let savedAudit = transport.audit(
            runID: runID, evidence: doc, sha: metadata.evidenceSha256)
        let (paths, saved) = try await (geometry, savedAudit)
        guard managedGeneration == generation, !Task.isCancelled else { return }
        guard
            saved.records.allSatisfy({
                $0.startNanos >= metadata.startNanos && $0.endExclusiveNanos >= $0.startNanos
                    && $0.endExclusiveNanos <= metadata.endExclusiveNanos
            })
        else {
            throw FollowingEvidence.ReviewError.invalid(
                "Saved review periods exceed this recording.")
        }
        document = doc
        sampleIndices = [:]
        for (index, row) in doc.rows.enumerated() {
            sampleIndices[row.followerTrackId, default: [:]][row.captureUnixNanos] = index
        }
        followerChoices = sampleIndices.keys.sorted()
        managedGeometry = paths
        let ids = Dictionary(
            doc.analysis.timelines.compactMap { timeline in
                timeline.geometryId.map { (timeline.followerTrackId, $0) }
            }, uniquingKeysWith: { first, _ in first })
        geometryIndex = Dictionary(
            paths.paths.compactMap { result in
                guard let path = result.path, ids[result.followerTrackId] == path.id else {
                    return nil
                }
                return (result.followerTrackId, path)
            }, uniquingKeysWith: { first, _ in first })
        audit = saved
        times = Array(Set(doc.rows.map(\.captureUnixNanos))).sorted()
        if follower.isEmpty { follower = doc.rows.first?.followerTrackId ?? "" }
        loaded = true
        error = nil
    }

    /// Called by the shared playback clock. No sample is interpolated to make
    /// an absent pairing look present, and slices never become user choices.
    func followPlayback(at nanos: Int64, selectedTrackID: String?) {
        guard let metadata = managedMetadata, nanos >= metadata.startNanos,
            nanos < metadata.endExclusiveNanos
        else { return }
        timestamp = String(nanos)
        if let selectedTrackID, !selectedTrackID.isEmpty { follower = selectedTrackID }

        if let slice = document?.reviewSlice, nanos >= slice.sampleStartNanos,
            nanos < slice.sampleEndExclusiveNanos
        {
            return
        }
        guard !loadingWindow, failedManagedWindow?.contains(nanos) != true else { return }
        let generation = managedGeneration
        windowTask?.cancel()
        windowTask = Task {
            do { try await loadManagedWindow(at: nanos, generation: generation) } catch {
                if managedGeneration == generation, !Task.isCancelled {
                    self.error = error.localizedDescription
                }
            }
        }
    }

    /// Empty minutes have no slice descriptor. Remember the complete gap as
    /// well, so advancing frames cannot turn one refusal into a request loop.
    private func managedWindowRange(
        at nanos: Int64, metadata: FollowingReviewMetadata
    ) -> Range<Int64> {
        if let slice = metadata.slices.first(where: {
            nanos >= $0.startNanos && nanos < $0.endExclusiveNanos
        }) {
            return slice.startNanos..<slice.endExclusiveNanos
        }
        let start =
            metadata.slices.filter { $0.endExclusiveNanos <= nanos }.map(\.endExclusiveNanos).max()
            ?? metadata.startNanos
        let end =
            metadata.slices.filter { $0.startNanos > nanos }.map(\.startNanos).min()
            ?? metadata.endExclusiveNanos
        return start..<end
    }

    func saveManagedRecord(_ record: FollowingAudit.Record) async {
        guard let transport = managedTransport, let runID = managedRunID, let doc = document,
            let binding = audit?.binding, !savingRecord, pendingSave == nil
        else { return }
        let generation = managedGeneration
        do {
            try FollowingAudit.validate(record, evidence: doc)
            pendingSave = FollowingPendingSave(
                runID: runID, binding: binding, record: record, transport: transport)
            await retryPendingSave()
        } catch { if managedGeneration == generation { self.error = error.localizedDescription } }
    }

    func startDraft(_ record: FollowingAudit.Record) {
        guard managedDraft == nil, pendingSave == nil, let runID = managedRunID,
            let binding = audit?.binding, let doc = document, let transport = managedTransport,
            let origin = managedMetadata?.startNanos
        else { return }
        managedDraft = FollowingReviewDraft(
            record: record, runID: runID, binding: binding, evidence: doc, transport: transport,
            timeOrigin: origin)
    }

    /// Capture the original recording and evidence synchronously, before the
    /// form disappears or a task can run after a source transition.
    func submitDraft(_ record: FollowingAudit.Record) throws {
        guard let draft = managedDraft, draft.record.id == record.id, pendingSave == nil,
            !savingRecord
        else {
            throw FollowingEvidence.ReviewError.invalid(
                "Resolve the outstanding review save first.")
        }
        try FollowingAudit.validate(record, evidence: draft.evidence)
        pendingSave = FollowingPendingSave(
            runID: draft.runID, binding: draft.binding, record: record, transport: draft.transport)
        managedDraft = nil
    }

    func retryPendingSave() async {
        guard let pending = pendingSave, !savingRecord else { return }
        savingRecord = true
        error = nil
        defer { savingRecord = false }
        do {
            try await pending.transport.append(
                runID: pending.runID, binding: pending.binding, record: pending.record)
            guard pendingSave?.record.id == pending.record.id else { return }
            pendingSave = nil
            if managedRunID == pending.runID, fileIdentity == pending.binding.evidenceSHA256,
                let doc = document
            {
                if audit?.records.contains(where: { $0.id == pending.record.id }) == false {
                    audit?.records.append(pending.record)
                }
                do {
                    let saved = try await pending.transport.audit(
                        runID: pending.runID, evidence: doc, sha: fileIdentity)
                    guard saved.records.contains(where: { $0.id == pending.record.id }) else {
                        throw FollowingEvidence.ReviewError.invalid(
                            "The confirmed review was not returned by the audit refresh.")
                    }
                    if managedRunID == pending.runID, fileIdentity == pending.binding.evidenceSHA256
                    {
                        audit = saved
                    }
                } catch {
                    self.error =
                        "Review saved; could not refresh other review records: \(error.localizedDescription)"
                }
            }
        } catch {
            self.error =
                "Review not confirmed saved. Retry the same record: \(error.localizedDescription)"
        }
    }

    var selectedRow: FollowingEvidence.Row? {
        guard let at = Int64(timestamp) else { return nil }
        if isManaged {
            guard let index = sampleIndices[follower]?[at] else { return nil }
            return document?.rows[index]
        }
        return document?.rows.first { $0.captureUnixNanos == at && $0.followerTrackId == follower }
    }

    var selectedPath: FollowingReviewGeometry.Path? { geometryIndex[follower] }
}
