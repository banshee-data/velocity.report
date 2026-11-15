import AppKit
import CryptoKit
import SwiftUI
import UniformTypeIdentifiers

/// File review only. The exported baseline's decisions and suppressions are
/// displayed verbatim; it never paints a pairing onto an unverified recording.
struct FollowingEvidenceView: View {
    @EnvironmentObject var appState: AppState
    var body: some View { FollowingEvidenceContent(review: appState.followingReview) }
}

private struct FollowingEvidenceContent: View {
    @EnvironmentObject var appState: AppState
    @ObservedObject var review: FollowingReviewSession
    @State private var auditLimit = 20
    @State private var draft: FollowingAudit.Record?

    var body: some View {
        DisclosureGroup("Following evidence (offline baseline)") {
            VStack(alignment: .leading, spacing: 8) {
                Button(
                    review.loading ? "Reading evidence…" : "Open following evidence…", action: open
                ).disabled(review.loading)
                if let error = review.error { Text(error).foregroundStyle(.orange) }
                if let doc = review.document {
                    provenance(doc)
                    HStack {
                        Button("Open audit…") { openAudit(doc) }
                        Button(review.auditUnsaved ? "Save audit… (unsaved)" : "Save audit…") {
                            saveAudit()
                        }.disabled(review.audit?.records.isEmpty != false)
                    }
                    Text(
                        "\(review.audit?.records.count ?? 0) operator review records. Audits retain uncertainty; they do not qualify physical measurements."
                    )
                    if let audit = review.audit, !audit.records.isEmpty {
                        DisclosureGroup("Review records (\(audit.records.count))") {
                            ForEach(audit.records.suffix(auditLimit)) { item in
                                VStack(alignment: .leading, spacing: 4) {
                                    Text("\(item.reviewer) · \(item.conclusion.title)").bold()
                                    Text(
                                        "\(item.followerTrackID) · \(item.startNanos)–\(item.endExclusiveNanos) ns"
                                    )
                                    Text(
                                        "Identity: \(item.identityContinuity.title); path/leader: \(item.leaderAndPath.title); endpoints: \(item.visibleEndpoints.title); speeds: \(item.speedEvidence.title)"
                                    )
                                    Text(item.notes)
                                    Button("Show evidence sample") {
                                        review.timestamp = String(item.startNanos)
                                        review.follower = item.followerTrackID
                                    }
                                    Divider()
                                }
                            }
                            if audit.records.count > auditLimit {
                                Button("Show 20 more records") { auditLimit += 20 }
                            }
                        }
                    }
                    TextField("Capture Unix nanoseconds", text: $review.timestamp)
                    TextField("Follower track ID (empty shows all)", text: $review.follower)
                    HStack {
                        Button("Previous sample") { move(-1) }
                        Button("Next sample") { move(1) }
                        Button("Use frame time") {
                            review.timestamp = String(appState.currentTimestamp)
                            review.follower = appState.selectedTrackID ?? ""
                        }
                    }.controlSize(.small)
                    Text(
                        doc.frameSourceNote(
                            Set(
                                appState.currentFrame?.tracks?.trails.flatMap {
                                    $0.points.filter {
                                        $0.captureTimestampNanos == appState.currentTimestamp
                                    }.map(\.sourceID).filter { !$0.isEmpty }
                                } ?? []))
                    ).foregroundStyle(.orange)
                    let rows =
                        Int64(review.timestamp).map { doc.rows(at: $0, follower: review.follower) }
                        ?? []
                    if rows.isEmpty {
                        Text("No exact sample. No interpolation or substituted pair.")
                            .foregroundStyle(.secondary)
                    }
                    ForEach(rows, id: \.followerTrackId) { row in rowView(row, doc: doc) }
                }
            }.font(.caption).textSelection(.enabled).padding(.top, 6).disabled(review.loading)
        }.sheet(item: $draft) { record in
            if let doc = review.document {
                FollowingAuditEditor(record: record) { edited in
                    guard var updated = review.audit else { return }
                    try updated.append(edited, evidence: doc)
                    review.audit = updated
                    review.auditUnsaved = true
                    draft = nil
                }
            }
        }
    }

    private func provenance(_ doc: FollowingEvidence) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text("\(doc.context.siteNamespace)/\(doc.context.siteId)").bold()
            Text("Source \(doc.context.sourceId)")
            Text("Binding \(doc.context.bindingId) · policy \(doc.context.policyId)")
            Text(
                "\(doc.analysisVersion.estimateStage) · \(doc.analysisVersion.estimatorId) · \(doc.analysisVersion.obsModelId)"
            )
            Text(
                "Method \(doc.analysisVersion.methodId) · parameters \(doc.analysisVersion.paramHash)"
            )
            Text("Analyser \(doc.context.analysisBuildGitSha) · file \(review.fileIdentity)")
            if let source = doc.context.sourceManifest {
                Text("Extractor \(source.extractorId)")
                ForEach(source.capturePaths.indices, id: \.self) { i in
                    Text(
                        "Capture \(source.capturePaths[i]) · SHA-256 \(i < source.captureSha256s.count ? source.captureSha256s[i] : "missing")"
                    )
                }
                if let w = source.window {
                    Text(
                        "Capture-relative window: offset \(w.startOffsetNanos) ns · warm-up \(w.warmupNanos) ns · duration \(w.durationNanos) ns (zero means EOF)"
                    )
                } else {
                    Text("Historical source: replay-window guarantee unavailable.")
                }
            } else {
                Text(
                    "Source manifest unavailable; capture bytes and replay window cannot be verified here."
                )
            }
            Text(
                "Recording producer build is not supplied by this evidence contract. File review does not repeat the exporter's arithmetic or capture-hash validation."
            )
            Text(
                "Track-derived local path; no lane snapshot. Coordinate frame ID is absent from this export. File evidence is not overlaid on playback."
            )
            Text(
                "Experimental longitudinal evidence, not physically qualified. DRAC assumes a constant leader speed and does not measure lateral collision risk. Missing bounds mean unknown uncertainty."
            )
        }.foregroundStyle(.secondary)
    }

    private func rowView(_ row: FollowingEvidence.Row, doc: FollowingEvidence) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text("Follower \(row.followerTrackId)").bold()
                Button("Audit this period…") { startAudit(row) }
            }
            Text("\(row.captureUnixNanos) ns · interval \(row.intervalNanos) ns · \(row.bucket)")
            Text("Selected leader: \(row.decision.leaderTrackId ?? "none")")
            Text("\(row.decision.reason ?? "") \(row.decision.condition ?? "")")
            Text(
                "Path \(doc.analysis.timelines.first { $0.followerTrackId == row.followerTrackId }?.geometryId ?? "unavailable")"
            )
            if let p = row.point {
                if let g = p.gap {
                    Text(
                        "Baseline front-to-rear path gap \(number(g.valueM)) m · marginal σ \(number(g.sigmaM)) m · \(g.reason ?? "arithmetic retained")"
                    )
                }
                if let t = p.timeGap {
                    Text(
                        "Net time gap \(t.valueS.map(number) ?? "undefined") s · marginal σ \(t.sigmaS.map(number) ?? "unavailable") s · \(t.reason ?? "arithmetic retained")"
                    )
                }
                Text(
                    "Projected follower / leader \(row.followerSpeedMps.map(number) ?? "unavailable") / \(row.leaderSpeedMps.map(number) ?? "unavailable") m/s"
                )
                if let e = p.follower?.leading { endpoint(e, name: "Follower front") }
                if let e = p.leader?.trailing { endpoint(e, name: "Leader rear") }
                Text(
                    "Reasons: \((p.reasons ?? []).joined(separator: ", ")) · coast \(p.coastAgeNanos.map { String($0) } ?? "unavailable") ns"
                )
            }
            Text(
                "Experimental DRAC: \(row.experimentalDrac.status) · \(row.experimentalDrac.evidenceReason ?? "")"
            )
            if let c = row.experimentalDrac.calculation {
                Text(
                    "Demand \(c.valueMps2.map(number) ?? "suppressed") m/s² · closing \(c.closingSpeedMps.map(number) ?? "unavailable") m/s"
                )
                if let b = c.boundsMps2 {
                    Text("Declared bounds \(number(b.lower))–\(number(b.upper)) m/s²")
                }
            }
            Text(
                "Each ID names a distinct track, not a verified physical actor. Candidate status describes a relationship hypothesis. Track breaks are not joined."
            ).foregroundStyle(.secondary)
            ForEach(row.decision.candidates ?? [], id: \.trackId) { candidate in
                Text(
                    "\(candidate.trackId): \(candidate.disposition) · path member \(candidate.established ? "yes" : "no") · synchronised \(candidate.synchronised ? "yes" : "no") · lateral \(candidate.lateralM.map(number) ?? "unavailable") m"
                )
            }
            Text(
                "Lateral offset is not footprint overlap. Non-selected candidates have no exported endpoint, speed or DRAC evaluation; lane transitions and encroachment remain unsupported."
            ).foregroundStyle(.orange)
            Divider()
        }
    }

    private func startAudit(_ row: FollowingEvidence.Row) {
        let run = appState.runBrowserState.runs.first { $0.runId == appState.currentRunID }
        let sources = Set(
            appState.currentFrame?.tracks?.trails.flatMap {
                $0.points.filter { $0.captureTimestampNanos == appState.currentTimestamp }.map(
                    \.sourceID
                ).filter { !$0.isEmpty }
            } ?? [])
        draft = FollowingAudit.Record(
            row: row,
            playback: .init(
                runID: appState.currentRunID, sourcePath: run?.sourcePath,
                recordingPath: run?.vrlogPath, captureNanos: appState.currentTimestamp,
                startNanos: appState.logStartTimestamp, endNanos: appState.logEndTimestamp,
                coordinateFrameID: appState.currentFrame?.coordinateFrame?.frameID,
                frameSourceIDs: sources.sorted()))
    }

    private func confirmReplacingUnsavedAudit() -> Bool {
        guard review.auditUnsaved else { return true }
        let alert = NSAlert()
        alert.messageText = "Replace unsaved audit records?"
        alert.informativeText = "Save the audit first to keep these observations."
        alert.addButton(withTitle: "Cancel")
        alert.addButton(withTitle: "Discard unsaved audit")
        return alert.runModal() == .alertSecondButtonReturn
    }

    private func openAudit(_ doc: FollowingEvidence) {
        guard confirmReplacingUnsavedAudit() else { return }
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.json]
        panel.message = "Choose an audit saved against these exact evidence bytes."
        guard panel.runModal() == .OK, let url = panel.url else { return }
        do {
            let size = try url.resourceValues(forKeys: [.fileSizeKey]).fileSize ?? 0
            guard size <= 16 * 1024 * 1024 else {
                throw FollowingEvidence.ReviewError.invalid("Audit exceeds the 16 MiB limit.")
            }
            review.audit = try FollowingAudit.decode(
                Data(contentsOf: url), evidence: doc, sha256: review.fileIdentity)
            review.auditUnsaved = false
            review.error = nil
        } catch { review.error = error.localizedDescription }
    }

    private func saveAudit() {
        guard let audit = review.audit else { return }
        let panel = NSSavePanel()
        panel.allowedContentTypes = [.json]
        panel.nameFieldStringValue = "following-audit.json"
        panel.message =
            "Save local review evidence on the attached LiDAR volume. Keep raw audit files outside Git."
        guard panel.runModal() == .OK, let url = panel.url else { return }
        do {
            guard
                url.resolvingSymlinksInPath().standardizedFileURL
                    != review.evidenceURL?.resolvingSymlinksInPath().standardizedFileURL
            else {
                throw FollowingEvidence.ReviewError.invalid(
                    "Choose a separate audit file; the evidence export must remain unchanged.")
            }
            try audit.encoded().write(to: url, options: .atomic)
            review.auditUnsaved = false
            review.error = nil
        } catch { review.error = error.localizedDescription }
    }

    private func endpoint(_ e: FollowingEvidence.Endpoint, name: String) -> some View {
        Text(
            "\(name): arc \(number(e.arcM)) m · σ \(number(e.sigmaM)) m · \(e.source) · \(e.support) · extent \(e.extentConverged ? "converged" : "unconverged") · length/width \(e.lengthProvenance ?? "unknown")/\(e.widthProvenance ?? "unknown")"
        )
    }
    private func number(_ value: Double) -> String { String(format: "%.5g", value) }
    private func move(_ direction: Int) {
        guard let t = Int64(review.timestamp) else { return }
        let candidates =
            direction > 0 ? review.times.filter { $0 > t } : review.times.filter { $0 < t }
        if let next = direction > 0 ? candidates.first : candidates.last {
            review.timestamp = String(next)
        }
    }
    private func open() {
        guard confirmReplacingUnsavedAudit() else { return }
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.json]
        panel.message = "Choose a following_evidence_v1 export from velocity report headway."
        guard panel.runModal() == .OK, let url = panel.url else { return }
        guard review.beginImport() else { return }
        Task {
            defer { review.finishImport() }
            do {
                let result = try await Task.detached {
                    let size = try url.resourceValues(forKeys: [.fileSizeKey]).fileSize ?? 0
                    guard size <= 1024 * 1024 * 1024 else {
                        throw FollowingEvidence.ReviewError.invalid(
                            "Evidence exceeds the 1 GiB viewer limit; use a bounded export.")
                    }
                    let data = try Data(contentsOf: url)
                    let doc = try FollowingEvidence.decode(data)
                    let digest = SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
                    return (doc, digest, Array(Set(doc.rows.map(\.captureUnixNanos))).sorted())
                }.value
                review.document = result.0
                review.evidenceURL = url
                review.fileIdentity = "sha256:" + result.1
                review.audit = FollowingAudit(
                    binding: .init(evidence: result.0, sha256: review.fileIdentity))
                review.auditUnsaved = false
                review.times = result.2
                review.timestamp = String(review.times[0])
            } catch { review.error = error.localizedDescription }
        }
    }
}

/// Always-visible review context inside the inspector, including the absence
/// of a located clip. Choosing a site must never silently bind another run.
struct RecordingReviewContext: View {
    @EnvironmentObject var appState: AppState
    var body: some View {
        let run = appState.runBrowserState.runs.first { $0.runId == appState.currentRunID }
        GroupBox("Recording review context") {
            VStack(alignment: .leading, spacing: 3) {
                Text("Chosen site: \(appState.siteContext.site?.title ?? "none")")
                Text("Run: \(appState.currentRunID ?? "unavailable")")
                Text("Source: \(run?.sourcePath ?? "unavailable")")
                Text("Recording: \(run?.vrlogPath ?? "unavailable")")
                Text("Capture time: \(appState.currentTimestamp) ns")
                Text(
                    "Recording window: \(appState.logStartTimestamp)–\(appState.logEndTimestamp) ns"
                )
                Text(
                    "Frame: \(appState.currentFrame?.coordinateFrame?.frameID ?? "unavailable") · \(appState.currentFrame?.coordinateFrame?.referenceFrame ?? "unavailable")"
                )
                Text("Configuration: \(run?.configHash ?? "unrecorded")")
                Text("Parameters: \(run?.paramsHash ?? "unrecorded")")
                if let clip = run?.clipId, !clip.isEmpty {
                    Text(
                        "Recorded clip: \(clip). Check its site and capture/window binding before accepting geometry."
                    )
                } else {
                    Text(
                        "No recorded clip binding. The chosen site is viewing context; source-to-site agreement is unverified."
                    ).foregroundStyle(.orange)
                }
            }.font(.caption2).textSelection(.enabled).frame(
                maxWidth: .infinity, alignment: .leading)
        }
    }
}
