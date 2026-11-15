import AppKit
import CryptoKit
import SwiftUI
import UniformTypeIdentifiers

/// File review only. The exported baseline's decisions and suppressions are
/// displayed verbatim; it never paints a pairing onto an unverified recording.
struct FollowingEvidenceView: View {
    @EnvironmentObject var appState: AppState
    @State private var document: FollowingEvidence?
    @State private var fileIdentity = ""
    @State private var error: String?
    @State private var loading = false
    @State private var timestamp = ""
    @State private var follower = ""
    @State private var times: [Int64] = []

    var body: some View {
        DisclosureGroup("Following evidence (offline baseline)") {
            VStack(alignment: .leading, spacing: 8) {
                Button(loading ? "Reading evidence…" : "Open following evidence…", action: open)
                    .disabled(loading)
                if let error { Text(error).foregroundStyle(.orange) }
                if let doc = document {
                    provenance(doc)
                    TextField("Capture Unix nanoseconds", text: $timestamp)
                    TextField("Follower track ID (empty shows all)", text: $follower)
                    HStack {
                        Button("Previous sample") { move(-1) }
                        Button("Next sample") { move(1) }
                        Button("Use frame time") {
                            timestamp = String(appState.currentTimestamp)
                            follower = appState.selectedTrackID ?? ""
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
                    let rows = Int64(timestamp).map { doc.rows(at: $0, follower: follower) } ?? []
                    if rows.isEmpty {
                        Text("No exact sample. No interpolation or substituted pair.")
                            .foregroundStyle(.secondary)
                    }
                    ForEach(rows, id: \.followerTrackId) { row in rowView(row, doc: doc) }
                }
            }.font(.caption).textSelection(.enabled).padding(.top, 6)
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
            Text("Analyser \(doc.context.analysisBuildGitSha) · file \(fileIdentity)")
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
            Text("Follower \(row.followerTrackId)").bold()
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
                    "Reasons: \((p.reasons ?? []).joined(separator: ", ")) · coast \(p.coastAgeNanos ?? 0) ns"
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

    private func endpoint(_ e: FollowingEvidence.Endpoint, name: String) -> some View {
        Text(
            "\(name): arc \(number(e.arcM)) m · σ \(number(e.sigmaM)) m · \(e.source) · \(e.support) · extent \(e.extentConverged ? "converged" : "unconverged") · length/width \(e.lengthProvenance ?? "unknown")/\(e.widthProvenance ?? "unknown")"
        )
    }
    private func number(_ value: Double) -> String { String(format: "%.5g", value) }
    private func move(_ direction: Int) {
        guard let t = Int64(timestamp) else { return }
        let candidates = direction > 0 ? times.filter { $0 > t } : times.filter { $0 < t }
        if let next = direction > 0 ? candidates.first : candidates.last {
            timestamp = String(next)
        }
    }
    private func open() {
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.json]
        panel.message = "Choose a following_evidence_v1 export from velocity report headway."
        guard panel.runModal() == .OK, let url = panel.url else { return }
        loading = true
        error = nil
        document = nil
        Task {
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
                document = result.0
                fileIdentity = "sha256:" + result.1
                times = result.2
                timestamp = String(times[0])
            } catch { self.error = error.localizedDescription }
            loading = false
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
