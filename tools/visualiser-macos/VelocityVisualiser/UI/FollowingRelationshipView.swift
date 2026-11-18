import SwiftUI

/// Relationships and projected geometry from an exact immutable sample. The
/// scene never invents vehicle corners, joins track IDs or scores alternatives.
struct FollowingRelationshipScene {
    struct OtherGap {
        let followerID: String
        let leaderID: String
        let points: [SIMD2<Double>]
        let gapM: Double
    }
    struct Actor: Identifiable {
        let id: String
        let position: SIMD2<Double>
        let leader: Bool
        let disposition: String
    }
    var path: [SIMD2<Double>] = []
    var gap: [SIMD2<Double>] = []
    var gapM: Double?
    var follower: SIMD2<Double>?
    var actors: [Actor] = []
    var otherGaps: [OtherGap] = []

    init(
        row: FollowingEvidence.Row?, path: FollowingReviewGeometry.Path?,
        contextRows: [FollowingEvidence.Row] = [], paths: [FollowingReviewGeometry.Result] = []
    ) {
        guard let row, let path else { return }
        self.path = path.vertices.map { SIMD2($0.x, $0.y) }
        if let body = row.point?.follower {
            follower = path.point(at: (body.leading.arcM + body.trailing.arcM) / 2)
        }
        actors = (row.decision.candidates ?? []).compactMap { candidate in
            guard let arc = candidate.centreArcM, let lateral = candidate.lateralM,
                let position = path.point(at: arc, lateral: lateral)
            else { return nil }
            return Actor(
                id: candidate.trackId, position: position,
                leader: candidate.trackId == row.decision.leaderTrackId,
                disposition: candidate.disposition)
        }
        if let from = row.point?.follower?.leading.arcM, let to = row.point?.leader?.trailing.arcM,
            to > from, let first = path.point(at: from), let last = path.point(at: to)
        {
            gap =
                [first]
                + path.vertices.filter { $0.arcM > from && $0.arcM < to }.map { SIMD2($0.x, $0.y) }
                + [last]
            gapM = row.point?.gap?.valueM
        }
        let visible = Set(actors.map(\.id)).union([row.followerTrackId])
        for other in contextRows
        where other.captureUnixNanos == row.captureUnixNanos
            && other.followerTrackId != row.followerTrackId
            && visible.contains(other.followerTrackId)
        {
            guard let leader = other.decision.leaderTrackId, visible.contains(leader),
                let otherPath = paths.first(where: { $0.followerTrackId == other.followerTrackId })?
                    .path, let from = other.point?.follower?.leading.arcM,
                let to = other.point?.leader?.trailing.arcM, to > from,
                let start = otherPath.point(at: from), let end = otherPath.point(at: to),
                let gapM = other.point?.gap?.valueM
            else { continue }
            let points =
                [start]
                + otherPath.vertices.filter { $0.arcM > from && $0.arcM < to }.map {
                    SIMD2($0.x, $0.y)
                } + [end]
            otherGaps.append(
                OtherGap(
                    followerID: other.followerTrackId, leaderID: leader, points: points, gapM: gapM)
            )
        }
    }
}

func followingDRACValue(_ row: FollowingEvidence.Row) -> Double? {
    guard row.experimentalDrac.status == "supported",
        row.experimentalDrac.calculation?.status == "supported"
    else { return nil }
    return row.experimentalDrac.calculation?.valueMps2
}

struct FollowingRelationshipView: View {
    @EnvironmentObject var appState: AppState
    @ObservedObject var review: FollowingReviewSession
    var period: FollowingAudit.Record? = nil
    var showsSavedRecords = true
    @State private var sliderSeconds: Double = 0
    @State private var recordLimit = 20

    private var bound: Bool {
        review.managedRunID == appState.currentRunID
            && review.managedTransport?.baseURL == VelocityReportServer.currentHTTPURL
    }
    private var row: FollowingEvidence.Row? { review.selectedRow }
    private var followers: [String] {
        if review.follower.isEmpty || review.followerChoices.contains(review.follower) {
            return review.followerChoices
        }
        return ([review.follower] + review.followerChoices)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Label("Following review", systemImage: "point.3.connected.trianglepath.dotted").font(
                .headline)
            if review.loading { ProgressView("Verifying recording and evidence…") }
            if let error = review.error {
                Text(error).foregroundStyle(.orange).font(.caption).lineLimit(3)
                Button("Reload review") {
                    if let id = review.managedRunID {
                        Task { await appState.openFollowingReview(id) }
                    }
                }.disabled(review.loading || review.savingRecord || review.pendingSave != nil)
            }
            if !bound {
                Text("Open this recording's review to restore its source and analysis context.")
                    .font(.caption)
            } else if let metadata = review.managedMetadata {
                if !followers.isEmpty {
                    SiteChoicePopover(
                        title: "Follower · \(short(review.follower))", selection: review.follower,
                        rows: followers.map { SiteChoiceRow(id: $0, title: short($0)) },
                        accessibilityTitle: "Choose follower", searchPrompt: "Find a follower"
                    ) { id in
                        review.follower = id
                        appState.selectTrack(id, showInspector: false)
                        appState.refreshFollowingOverlay()
                    }

                }
                if let row {
                    HStack {
                        Text(short(row.followerTrackId)).foregroundStyle(.cyan)
                        Image(systemName: "arrow.right")
                        Text(row.decision.leaderTrackId.map(short) ?? "Leader unresolved")
                            .foregroundStyle(.purple)
                    }.font(.headline)
                    FollowingRelationshipMap(
                        scene: FollowingRelationshipScene(
                            row: row, path: review.selectedPath,
                            contextRows: review.document?.rows.filter {
                                $0.captureUnixNanos == row.captureUnixNanos
                            } ?? [],
                            paths: review.managedGeometry?.paths.filter {
                                review.geometryIndex[$0.followerTrackId] != nil
                            } ?? []), followerID: row.followerTrackId
                    ) { id in appState.selectTrack(id, showInspector: false) }
                    HStack(alignment: .top) {
                        metric(
                            "Estimated path gap", row.point?.gap.map { "\(number($0.valueM)) m" })
                        metric("Net time gap", row.point?.timeGap?.valueS.map { "\(number($0)) s" })
                    }
                    HStack(alignment: .top) {
                        metric(
                            "Closing",
                            row.experimentalDrac.calculation?.closingSpeedMps.map {
                                "\(number($0)) m/s"
                            })
                        metric(
                            "Experimental DRAC",
                            followingDRACValue(row).map { "\(number($0)) m/s²" })
                    }
                    Text(
                        "Follower \(row.followerSpeedMps.map { number($0) } ?? "—") · leader \(row.leaderSpeedMps.map { number($0) } ?? "—") m/s"
                    ).font(.caption).monospacedDigit()
                    Text(status(row)).font(.caption).foregroundStyle(.secondary)
                    Text(appState.currentFollowingReviewOverlay.notice).font(.caption)
                        .foregroundStyle(.secondary)
                    if review.selectedPath == nil {
                        Text("Path geometry unavailable for this sample; no spatial gap is drawn.")
                            .font(.caption).foregroundStyle(.orange)
                    }
                    HStack { Button("Review this period…") { beginReview(row) } }.disabled(
                        review.savingRecord || review.loadingWindow || review.pendingSave != nil)
                    if review.savingRecord { ProgressView("Saving review…").controlSize(.small) }

                } else if !review.loading {
                    Text("No exact follower sample at this frame. No gap or pair is substituted.")
                        .font(.caption).foregroundStyle(.secondary)
                }
                if review.loadingWindow { ProgressView("Loading this period…").controlSize(.small) }
                FollowingRelationshipTimeline(review: review) { at in seek(at) }
                HStack {
                    Button {
                        step(-1)
                    } label: {
                        Image(systemName: "backward.end")
                    }.help("Previous exact follower sample")
                    Button {
                        step(1)
                    } label: {
                        Image(systemName: "forward.end")
                    }.help("Next exact follower sample")
                    Text(elapsed(metadata)).font(.caption).monospacedDigit()
                }
                Slider(
                    value: $sliderSeconds,
                    in:
                        0...max(
                            0.001,
                            Double(metadata.endExclusiveNanos - metadata.startNanos - 1) / 1e9),
                    onEditingChanged: { editing in
                        if !editing { seek(metadata.startNanos + Int64(sliderSeconds * 1e9)) }
                    }
                ).accessibilityLabel("Seek recording review time")
                if showsSavedRecords {
                    DisclosureGroup("Review records (\(review.audit?.records.count ?? 0))") {
                        ForEach((review.audit?.records ?? []).suffix(recordLimit)) { record in
                            savedRecord(record, metadata: metadata)
                        }
                        if (review.audit?.records.count ?? 0) > recordLimit {
                            Button("Show 20 earlier records") { recordLimit += 20 }
                        }
                    }
                }
                DisclosureGroup("Evidence details") {
                    Text(
                        "Source \(metadata.sourceId)\nCalibration \(metadata.calibrationId)\nAnalysis \(metadata.evidenceSha256)"
                    ).font(.caption).textSelection(.enabled)
                    if let row {
                        Text(
                            "Follower \(row.followerTrackId)\nLeader \(row.decision.leaderTrackId ?? "unresolved")\n\(row.captureUnixNanos) ns"
                        ).font(.caption).textSelection(.enabled)
                        ForEach(row.decision.candidates ?? [], id: \.trackId) { candidate in
                            Text("\(short(candidate.trackId)): \(candidate.disposition)").font(
                                .caption)
                        }
                    }
                }
            }
        }.onChange(of: appState.selectedTrackID) { _, id in
            guard bound else { return }
            review.followPlayback(at: appState.currentTimestamp, selectedTrackID: id)
            appState.refreshFollowingOverlay()
        }.onChange(of: appState.currentTimestamp) { _, at in
            guard bound else { return }
            review.followPlayback(at: at, selectedTrackID: appState.selectedTrackID)
            if let metadata = review.managedMetadata {
                sliderSeconds = Double(max(0, at - metadata.startNanos)) / 1e9
            }
        }.onChange(of: review.timestamp) { _, value in
            if let metadata = review.managedMetadata, let at = Int64(value) {
                sliderSeconds = Double(max(0, at - metadata.startNanos)) / 1e9
            }
        }.onChange(of: review.loadingWindow) { _, loading in
            if !loading, bound {
                review.followPlayback(
                    at: appState.currentTimestamp, selectedTrackID: appState.selectedTrackID)
            }
            appState.refreshFollowingOverlay()
        }
    }

    private func savedRecord(
        _ record: FollowingAudit.Record, metadata: FollowingReviewMetadata
    ) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Button("\(short(record.followerTrackID)) · \(record.conclusion.title)") {
                review.follower = record.followerTrackID
                appState.selectTrack(record.followerTrackID, showInspector: false)
                seek(record.startNanos)
            }.buttonStyle(.link).font(.caption)
            DisclosureGroup("Review observations") {
                Text("Reviewer: \(record.reviewer)")
                Text(
                    String(
                        format: "Period: %.3f–%.3f seconds",
                        Double(record.startNanos - metadata.startNanos) / 1e9,
                        Double(record.endExclusiveNanos - metadata.startNanos) / 1e9))
                Text("Identity: \(record.identityContinuity.title)")
                Text("Leader / path: \(record.leaderAndPath.title)")
                if let leader = record.proposedLeaderTrackID { Text("Observed leader: \(leader)") }
                Text("Endpoints: \(record.visibleEndpoints.title)")
                Text("Speeds: \(record.speedEvidence.title)")
                Text(record.notes).textSelection(.enabled)
            }.font(.caption)
        }
    }

    private func metric(_ label: String, _ value: String?) -> some View {
        VStack(alignment: .leading) {
            Text(label).font(.caption).foregroundStyle(.secondary)
            Text(value ?? "Unavailable").font(.callout).monospacedDigit()
        }.frame(maxWidth: .infinity, alignment: .leading)
    }
    private func number(_ x: Double) -> String { String(format: "%.3g", x) }
    private func short(_ id: String) -> String { String(id.suffix(8)) }
    private func status(_ row: FollowingEvidence.Row) -> String {
        ([
            row.bucket, row.experimentalDrac.status, row.decision.reason,
            row.experimentalDrac.evidenceReason,
        ].compactMap { $0 } + (row.point?.reasons ?? [])).joined(separator: " · ")
            .replacingOccurrences(of: "_", with: " ")
    }
    private func elapsed(_ metadata: FollowingReviewMetadata) -> String {
        let seconds =
            max(0, (Int64(review.timestamp) ?? metadata.startNanos) - metadata.startNanos)
            / 1_000_000_000
        return String(format: "%02lld:%02lld", seconds / 60, seconds % 60)
    }
    private func seek(_ at: Int64) {
        appState.seekToTimestamp(at)
        review.followPlayback(at: at, selectedTrackID: review.follower)
    }
    private func step(_ direction: Int) {
        guard let doc = review.document, let at = Int64(review.timestamp) else { return }
        let samples = doc.rows.filter { $0.followerTrackId == review.follower }.map(
            \.captureUnixNanos
        ).sorted()
        if let next = direction > 0
            ? samples.first(where: { $0 > at }) : samples.last(where: { $0 < at })
        {
            seek(next)
        } else if let slice = doc.reviewSlice, let metadata = review.managedMetadata {
            let target = direction > 0 ? slice.sampleEndExclusiveNanos : slice.sampleStartNanos - 1
            if target >= metadata.startNanos, target < metadata.endExclusiveNanos { seek(target) }
        }
    }
    private func beginReview(_ row: FollowingEvidence.Row) {
        review.startDraft(
            FollowingAudit.Record.reviewing(
                row: row, playback: appState.currentFollowingAuditPlayback, period: period))
    }
}

private struct FollowingRelationshipMap: View {
    let scene: FollowingRelationshipScene
    let followerID: String
    let choose: (String) -> Void
    @State private var showCandidateLabels = false

    var body: some View {
        Canvas { context, size in
            let project = projection(size)
            if scene.path.count > 1 {
                context.stroke(
                    line(scene.path.map(project)), with: .color(.secondary.opacity(0.45)),
                    lineWidth: 1)
            }
            for edge in scene.otherGaps {
                context.stroke(
                    line(edge.points.map(project)), with: .color(.secondary.opacity(0.45)),
                    lineWidth: 2)
                if showCandidateLabels, let mid = edge.points.dropFirst(edge.points.count / 2).first
                {
                    context.draw(
                        Text(
                            "\(String(edge.followerID.suffix(4))) → \(String(edge.leaderID.suffix(4))) · \(String(format: "%.2g", edge.gapM)) m"
                        ).font(.caption2), at: project(mid))
                }
            }
            for actor in scene.actors {
                let p = project(actor.position)
                if let follower = scene.follower {
                    context.stroke(
                        line([project(follower), p]),
                        with: .color(actor.leader ? .purple : .secondary.opacity(0.4)),
                        style: StrokeStyle(
                            lineWidth: actor.leader ? 2 : 1, dash: actor.leader ? [] : [4, 4]))
                }
                context.fill(
                    Path(ellipseIn: CGRect(x: p.x - 5, y: p.y - 5, width: 10, height: 10)),
                    with: .color(actor.leader ? .purple : .secondary))
                if showCandidateLabels && !actor.leader {
                    context.draw(
                        Text(String(actor.id.suffix(6))).font(.caption2).foregroundStyle(
                            .secondary), at: CGPoint(x: p.x, y: p.y - 14))
                }
            }
            if !scene.gap.isEmpty {
                context.stroke(line(scene.gap.map(project)), with: .color(.cyan), lineWidth: 4)
                if let gapM = scene.gapM, let first = scene.gap.first, let last = scene.gap.last {
                    let p = project((first + last) / 2)
                    context.draw(
                        Text(String(format: "%.3g m", gapM)).font(.caption.bold()).foregroundStyle(
                            .cyan), at: CGPoint(x: p.x - 12, y: p.y), anchor: .trailing)
                }
            }
            // Draw the reviewed pair last. Context labels must never obscure
            // which measurement belongs to this follower and chosen leader.
            for actor in scene.actors where actor.leader {
                let p = project(actor.position)
                context.fill(
                    Path(ellipseIn: CGRect(x: p.x - 6, y: p.y - 6, width: 12, height: 12)),
                    with: .color(.purple))
                context.draw(
                    Text("Leader \(String(actor.id.suffix(8)))").font(.caption.bold())
                        .foregroundStyle(.purple), at: CGPoint(x: p.x + 12, y: p.y),
                    anchor: .leading)
            }
            if let follower = scene.follower {
                let p = project(follower)
                context.fill(
                    Path(ellipseIn: CGRect(x: p.x - 6, y: p.y - 6, width: 12, height: 12)),
                    with: .color(.cyan))
                context.draw(
                    Text("Follower \(String(followerID.suffix(8)))").font(.caption.bold())
                        .foregroundStyle(.cyan), at: CGPoint(x: p.x + 12, y: p.y), anchor: .leading)
            }
        }.frame(height: 200).background(.quaternary.opacity(0.3)).clipShape(
            RoundedRectangle(cornerRadius: 6)
        ).accessibilityLabel(
            "Follower, selected leader, projected gap and competing track hypotheses"
        ).overlay {
            GeometryReader { geo in
                Color.clear.contentShape(Rectangle()).onTapGesture { location in
                    let project = projection(geo.size)
                    if let actor = scene.actors.min(by: {
                        distance(project($0.position), location)
                            < distance(project($1.position), location)
                    }), distance(project(actor.position), location) < 24 {
                        choose(actor.id)
                    }
                }
            }
        }.overlay(alignment: .bottomLeading) {
            HStack {
                Toggle("Candidate labels", isOn: $showCandidateLabels).toggleStyle(.checkbox)
                Spacer()
                Text("Dashed = alternatives").foregroundStyle(.secondary)
            }.font(.caption2).padding(5)
        }
    }
    private func line(_ points: [CGPoint]) -> Path {
        Path { p in
            if let first = points.first {
                p.move(to: first)
                points.dropFirst().forEach { p.addLine(to: $0) }
            }
        }
    }
    private func distance(_ a: CGPoint, _ b: CGPoint) -> Double { hypot(a.x - b.x, a.y - b.y) }
    private func projection(_ size: CGSize) -> (SIMD2<Double>) -> CGPoint {
        let points = scene.path + scene.actors.map(\.position) + (scene.follower.map { [$0] } ?? [])
        let xs = points.map(\.x)
        let ys = points.map(\.y)
        let xmin = xs.min() ?? 0
        let xmax = xs.max() ?? 1
        let ymin = ys.min() ?? 0
        let ymax = ys.max() ?? 1
        let scale = min(
            max(1, size.width - 44) / max(1, xmax - xmin),
            max(1, size.height - 54) / max(1, ymax - ymin))
        return { p in
            CGPoint(
                x: size.width / 2 + (p.x - (xmin + xmax) / 2) * scale,
                y: size.height / 2 - (p.y - (ymin + ymax) / 2) * scale)
        }
    }
}

struct FollowingTimelineSpan {
    let start: Int64
    var end: Int64
    let leaderID: String?
    let bucket: String
}

func followingTimelineSpans(
    _ rows: [FollowingEvidence.Row], follower: String
) -> [FollowingTimelineSpan] {
    var spans: [FollowingTimelineSpan] = []
    for row in rows.filter({ $0.followerTrackId == follower }).sorted(by: {
        $0.captureUnixNanos < $1.captureUnixNanos
    }) {
        let end = row.captureUnixNanos.addingReportingOverflow(row.intervalNanos)
        guard !end.overflow else { continue }
        if let previous = spans.last, previous.end == row.captureUnixNanos,
            previous.leaderID == row.decision.leaderTrackId, previous.bucket == row.bucket
        {
            spans[spans.count - 1].end = end.partialValue
        } else {
            spans.append(
                FollowingTimelineSpan(
                    start: row.captureUnixNanos, end: end.partialValue,
                    leaderID: row.decision.leaderTrackId, bucket: row.bucket))
        }
    }
    return spans
}

private struct FollowingRelationshipTimeline: View {
    @ObservedObject var review: FollowingReviewSession
    let seek: (Int64) -> Void
    var body: some View {
        if let doc = review.document, let slice = doc.reviewSlice {
            let rows = doc.rows.filter { $0.followerTrackId == review.follower }
            let spans = followingTimelineSpans(rows, follower: review.follower)
            Canvas { context, size in
                let duration = Double(slice.sampleEndExclusiveNanos - slice.sampleStartNanos)
                for span in spans {
                    let x = Double(span.start - slice.sampleStartNanos) / duration * size.width
                    let width = max(1, Double(span.end - span.start) / duration * size.width)
                    let colour: Color =
                        span.bucket == "following_eligible"
                        ? .cyan : span.bucket == "record_gap" ? .gray : .orange
                    let visibleWidth = min(width, size.width - x)
                    context.fill(
                        Path(CGRect(x: x, y: 0, width: visibleWidth, height: size.height)),
                        with: .color(colour.opacity(0.65)))
                    if visibleWidth > 48 {
                        context.draw(
                            Text(span.leaderID.map { String($0.suffix(4)) } ?? "—").font(.caption2),
                            at: CGPoint(x: x + visibleWidth / 2, y: size.height / 2))
                    }
                }

                if let at = Int64(review.timestamp) {
                    let x = Double(at - slice.sampleStartNanos) / duration * size.width
                    context.fill(
                        Path(CGRect(x: x, y: 0, width: 2, height: size.height)),
                        with: .color(.primary))
                }
            }.frame(height: 24).overlay {
                GeometryReader { geo in
                    Color.clear.contentShape(Rectangle()).onTapGesture { location in
                        let target =
                            slice.sampleStartNanos
                            + Int64(
                                Double(slice.sampleEndExclusiveNanos - slice.sampleStartNanos)
                                    * min(1, max(0, location.x / max(1, geo.size.width))))
                        if let sample = rows.min(by: {
                            abs($0.captureUnixNanos - target) < abs($1.captureUnixNanos - target)
                        }) {
                            seek(sample.captureUnixNanos)
                        }
                    }
                }
            }.accessibilityLabel(
                "Follower timeline: cyan following, orange excluded, grey record gaps; click to seek an exact sample"
            )
            Text("Following · excluded · missing — this minute").font(.caption2).foregroundStyle(
                .secondary)
        }
    }
}
