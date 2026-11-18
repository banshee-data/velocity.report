import SwiftUI

/// A separate workspace keeps the recording viewport available while a period
/// is inspected. Evidence, drafts and saves still belong to the shared session.
struct FollowingReviewWindow: View {
    @EnvironmentObject var appState: AppState
    var body: some View { FollowingReviewWorkspace(review: appState.followingReview) }
}

private struct FollowingReviewWorkspace: View {
    @EnvironmentObject var appState: AppState
    @Environment(\.openWindow) private var openWindow
    @ObservedObject var review: FollowingReviewSession
    @State private var selectedID: UUID?
    @State private var pendingSelection: FollowingAudit.Record?
    @State private var includeOtherReviews = false
    @State private var navigationError: String?

    private var allRecords: [FollowingAudit.Record] {
        (review.audit?.records ?? []).sorted {
            if $0.startNanos != $1.startNanos { return $0.startNanos < $1.startNanos }
            if $0.savedAt != $1.savedAt { return $0.savedAt < $1.savedAt }
            return $0.id.uuidString < $1.id.uuidString
        }
    }
    private var nominations: [FollowingAudit.Record] {
        allRecords.filter { $0.notes.hasPrefix("NOMINATION ") }
    }
    private var records: [FollowingAudit.Record] {
        includeOtherReviews || nominations.isEmpty ? allRecords : nominations
    }
    private var period: FollowingAudit.Record? { allRecords.first { $0.id == selectedID } }
    private var bound: Bool {
        review.managedRunID != nil && review.managedRunID == appState.currentRunID
            && review.managedTransport?.baseURL == VelocityReportServer.currentHTTPURL
    }
    private var canChoose: Bool {
        bound && appState.isConnected && appState.displayPlaybackMode == .replaySeekable
            && appState.hasValidTimelineRange && !review.loading && !review.loadingWindow
            && review.managedDraft == nil && review.pendingSave == nil && !review.savingRecord
            && !appState.playbackControlsBusy && pendingSelection == nil
    }
    private var selectedIndex: Int? { records.firstIndex { $0.id == selectedID } }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label("Follower review", systemImage: "point.3.connected.trianglepath.dotted").font(
                    .title2)
                Spacer()
                Button("Show 3D recording") { openWindow(id: "main") }
                Button(appState.isPaused ? "Play" : "Pause") { appState.togglePlayPause() }
                    .disabled(!bound || appState.playbackControlsBusy)
            }
            if let id = review.managedRunID {
                Text("Recording · \(id)").font(.caption).foregroundStyle(.secondary).lineLimit(1)
                    .textSelection(.enabled)
            }
            if let navigationError { Text(navigationError).foregroundStyle(.orange) }
            if pendingSelection != nil { ProgressView("Pausing recording…").controlSize(.small) }
            if review.isManaged {
                HSplitView {
                    caseList.frame(minWidth: 260, idealWidth: 300, maxWidth: 380)
                    ScrollView {
                        VStack(alignment: .leading, spacing: 12) {
                            HStack {
                                Button("Previous case") { move(-1) }.disabled(
                                    !canChoose || (selectedIndex ?? 0) <= 0)
                                Button("Next case") { move(1) }.disabled(
                                    !canChoose || records.isEmpty
                                        || (selectedIndex ?? -1) >= records.count - 1)
                                if let period { Text(range(period)).monospacedDigit() }
                            }
                            checklist
                            if let period {
                                DisclosureGroup("Case notes and observations") {
                                    VStack(alignment: .leading, spacing: 5) {
                                        Text("Reviewer: \(period.reviewer)")
                                        Text("Identity: \(period.identityContinuity.title)")
                                        Text("Leader / path: \(period.leaderAndPath.title)")
                                        if let leader = period.proposedLeaderTrackID {
                                            Text("Observed leader: \(leader.suffix(8))")
                                        }
                                        Text("Endpoints: \(period.visibleEndpoints.title)")
                                        Text("Speeds: \(period.speedEvidence.title)")
                                        Text(period.notes).textSelection(.enabled)
                                    }.font(.caption)
                                }
                            }
                            if let pending = review.pendingSave {
                                Button("Retry review save · \(pending.runID.suffix(8))") {
                                    Task { await review.retryPendingSave() }
                                }.disabled(review.savingRecord)
                            }
                            FollowingRelationshipView(review: review, period: period)
                        }.padding(.horizontal, 14)
                    }.frame(minWidth: 500)
                }
            } else {
                ContentUnavailableView {
                    Label("Choose a recording to review", systemImage: "record.circle")
                } description: {
                    Text("Use Following in Runs to open its recording, evidence and saved periods.")
                } actions: {
                    Button("Choose recording") {
                        openWindow(id: "main")
                        appState.showRunBrowser = true
                    }
                }
            }
        }.padding(16).frame(minWidth: 860, minHeight: 680).modifier(
            FollowingManagedDraftSheet(review: review)
        ).onAppear {
            selectedID =
                records.last(where: {
                    $0.followerTrackID == review.follower
                        && appState.currentTimestamp >= $0.startNanos
                        && appState.currentTimestamp < $0.endExclusiveNanos
                })?.id
        }.onChange(of: review.managedRunID) { _, _ in
            selectedID = nil
            pendingSelection = nil
            navigationError = nil
        }
    }

    private var caseList: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Saved periods · \(records.count)").font(.headline)
            if !nominations.isEmpty {
                Toggle("Include other saved reviews", isOn: $includeOtherReviews).toggleStyle(
                    .checkbox
                ).font(.caption)
                Text("Nominations are starting points, not completed physical labels.").font(
                    .caption
                ).foregroundStyle(.secondary)
            }
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 6) {
                    ForEach(records) { record in
                        Button {
                            choose(record)
                        } label: {
                            VStack(alignment: .leading, spacing: 5) {
                                Text(record.caseTitle).font(.headline).lineLimit(2)
                                Text(range(record)).monospacedDigit()
                                Text("Follower \(record.followerTrackID.suffix(8))")
                                    .foregroundStyle(.cyan)
                                Text(record.conclusion.title).foregroundStyle(.secondary)
                            }.font(.caption).frame(maxWidth: .infinity, alignment: .leading)
                                .padding(10).background(
                                    selectedID == record.id
                                        ? Color.accentColor.opacity(0.2)
                                        : Color.secondary.opacity(0.08),
                                    in: RoundedRectangle(cornerRadius: 8))
                        }.buttonStyle(.plain).disabled(!canChoose).accessibilityLabel(
                            "\(record.caseTitle), \(range(record)), follower \(record.followerTrackID.suffix(8))"
                        )
                    }
                }
            }
            if records.isEmpty {
                Text("Choose a follower and review a period to start its saved list.").font(
                    .caption
                ).foregroundStyle(.secondary)
            }
        }.padding(.trailing, 10)
    }

    private var checklist: some View {
        VStack(alignment: .leading, spacing: 5) {
            Label("Leader · immediate predecessor on the same path?", systemImage: "car.2")
            Label("Gap · visible front/rear returns, or inferred extents?", systemImage: "ruler")
            Label(
                "Speeds · both actors moving consistently across frames?",
                systemImage: "speedometer")
        }.font(.callout).padding(10).frame(maxWidth: .infinity, alignment: .leading).background(
            .quaternary, in: RoundedRectangle(cornerRadius: 8))
    }

    private func range(_ record: FollowingAudit.Record) -> String {
        guard let origin = review.managedMetadata?.startNanos else { return "Period unavailable" }
        return String(
            format: "%.3f–%.3f s", Double(record.startNanos - origin) / 1e9,
            Double(record.endExclusiveNanos - origin) / 1e9)
    }
    private func move(_ direction: Int) {
        let index = (selectedIndex ?? -1) + direction
        guard records.indices.contains(index) else { return }
        choose(records[index])
    }
    private func choose(_ record: FollowingAudit.Record) {
        guard canChoose else { return }
        navigationError = nil
        if !appState.isPaused {
            pendingSelection = record
            Task {
                let paused = await appState.pauseForFollowingReview()
                guard pendingSelection?.id == record.id else { return }
                pendingSelection = nil
                if paused && bound {
                    show(record)
                } else {
                    navigationError = "Could not pause this recording. Reconnect and try again."
                }
            }
        } else {
            show(record)
        }
    }
    private func show(_ record: FollowingAudit.Record) {
        guard bound, review.managedDraft == nil, review.pendingSave == nil else { return }
        selectedID = record.id
        review.follower = record.followerTrackID
        appState.selectTrack(record.followerTrackID, showInspector: false)
        appState.seekToTimestamp(record.startNanos)
        review.followPlayback(at: record.startNanos, selectedTrackID: record.followerTrackID)
        appState.refreshFollowingOverlay()
    }
}
