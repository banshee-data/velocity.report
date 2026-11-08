// RunBrowserState.swift
// State management for the run browser.
//
// This manages the list of available runs and loading runs for replay.

import Combine
import Foundation
import os

private let logger = DevLogger(category: "RunBrowserState")

private enum RunLabelBucket {
    case classified
    case taggedOnly
    case unlabelled
}

private struct RunTrackLabelSnapshot {
    var userLabel: String?
    var qualityLabel: String?
    var labelSource: String?
    var isSplitCandidate: Bool
    var isMergeCandidate: Bool
    var linkedTrackIDs: [String]

    init(track: RunTrack) {
        self.userLabel = track.userLabel
        self.qualityLabel = track.qualityLabel
        self.labelSource = track.labelSource
        self.isSplitCandidate = track.isSplitCandidate ?? false
        self.isMergeCandidate = track.isMergeCandidate ?? false
        self.linkedTrackIDs = track.linkedTrackIds ?? []
    }
}

private func normaliseRunLabelValue(_ value: String?) -> String? {
    guard let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines), !trimmed.isEmpty
    else { return nil }
    return trimmed
}

private func isHumanManualLabelSource(_ labelSource: String?) -> Bool {
    guard let labelSource = normaliseRunLabelValue(labelSource) else { return true }
    return labelSource == "human_manual"
}

private func runLabelBucket(for snapshot: RunTrackLabelSnapshot) -> RunLabelBucket {
    let userLabel = normaliseRunLabelValue(snapshot.userLabel)
    let qualityLabel = normaliseRunLabelValue(snapshot.qualityLabel)
    let hasManualLabel = isHumanManualLabelSource(snapshot.labelSource)
    let hasTagOnlyState =
        qualityLabel != nil || snapshot.isSplitCandidate || snapshot.isMergeCandidate
        || !snapshot.linkedTrackIDs.isEmpty

    if hasManualLabel, let userLabel, !userLabel.isEmpty { return .classified }
    if hasManualLabel, hasTagOnlyState { return .taggedOnly }
    return .unlabelled
}

private func computeRunLabelRollup(from snapshots: [RunTrackLabelSnapshot]) -> RunLabelRollup {
    var classified = 0
    var taggedOnly = 0
    var unlabelled = 0

    for snapshot in snapshots {
        switch runLabelBucket(for: snapshot) {
        case .classified: classified += 1
        case .taggedOnly: taggedOnly += 1
        case .unlabelled: unlabelled += 1
        }
    }

    return RunLabelRollup(
        total: snapshots.count, classified: classified, taggedOnly: taggedOnly,
        unlabelled: unlabelled)
}

/// State for browsing and selecting analysis runs.
@available(macOS 15.0, *) @MainActor class RunBrowserState: ObservableObject {

    // MARK: - Published State

    @Published var runs: [Run] = []
    /// The clips the runs name, by clip ID, for the groups' headers.
    @Published var clips: [String: ClipSummary] = [:]
    @Published var isLoading: Bool = false
    @Published var error: String?
    @Published var selectedRunID: String?
    @Published var isLoadingReplay: Bool = false

    // MARK: - Dependencies

    private let apiClient: RunTrackLabelAPIClient
    private var runTrackSnapshots: [String: [String: RunTrackLabelSnapshot]] = [:]

    // MARK: - Initialisation

    init(apiClient: RunTrackLabelAPIClient = RunTrackLabelAPIClient()) {
        self.apiClient = apiClient
    }

    // MARK: - Actions

    /// Fetch the list of runs from the backend.
    func fetchRuns() async {
        guard !isLoading else { return }
        isLoading = true
        error = nil

        do {
            let fetchedRuns = try await apiClient.listRuns(limit: 50)
            runs = fetchedRuns
            logger.info("Fetched \(fetchedRuns.count) runs")
            await fetchClips()
        } catch  where Self.isCancellation(error) {
            // A task that was cancelled, such as a window's lookup for a pack
            // closed meanwhile, failed nothing the operator asked for.
            logger.info("Fetching runs cancelled")
        } catch {
            self.error = "Failed to fetch runs: \(error.localizedDescription)"
            logger.error("Failed to fetch runs: \(error.localizedDescription)")
        }

        isLoading = false
    }

    /// Whether an error only reports that its task was cancelled.
    static func isCancellation(_ error: Error) -> Bool {
        if error is CancellationError { return true }
        if let error = error as? URLError, error.code == .cancelled { return true }
        return false
    }

    /// Fetch the clips, for naming the groups. A server that cannot list them
    /// leaves the groups named by clip ID, so a failure here is logged only.
    func fetchClips() async {
        do {
            let list = try await apiClient.listClips()
            var byID = Dictionary(
                list.map { ($0.clipId, $0) }, uniquingKeysWith: { first, _ in first })
            // The plain list names every clip; only the located list carries
            // locations, so the sites come from it.
            if let located = try? await apiClient.listClips(located: true) {
                for clip in located where byID[clip.clipId] != nil {
                    byID[clip.clipId]?.location = clip.location
                }
            }
            clips = byID
        } catch { logger.error("Failed to fetch clips: \(error.localizedDescription)") }
    }

    /// The name of the clip a run was made from, for the Annotation window's
    /// title, or nil when the run has none or cannot be found.
    ///
    /// It reads the run on its own rather than through `fetchRuns`, so a
    /// lookup that is cancelled or fails writes nothing the sheet shows, and
    /// two lookups cannot add the same run to the list twice.
    func clipName(forRun runID: String) async -> String? {
        var run = runs.first { $0.runId == runID }
        if run == nil { run = try? await apiClient.getRun(runID) }
        guard !Task.isCancelled, let run, let clip = RunGrouping.clipID(of: run) else { return nil }
        if clips[clip] == nil { await fetchClips() }
        return clips[clip]?.name ?? "Clip \(clip.prefix(8))"
    }

    /// Find a run a link names: in the list, else asked of the server, so a
    /// run older than the latest 50 still opens. A run found on the server
    /// joins the list, where `loadRunForReplay` looks for it. Returns the
    /// run, or why it could not be found.
    func findRun(_ runID: String) async -> Result<Run, RunLookupError> {
        if runs.isEmpty { await fetchRuns() }
        if let run = runs.first(where: { $0.runId == runID }) { return .success(run) }
        do {
            let run = try await apiClient.getRun(runID)
            runs.append(run)
            return .success(run)
        } catch let RunTrackLabelAPIClient.APIError.requestFailed(response)
            where (response as? HTTPURLResponse)?.statusCode == 404
        { return .failure(.notFound) } catch let RunTrackLabelAPIClient.APIError.server(status, _)
            where status == 404
        { return .failure(.notFound) } catch {
            return .failure(.unreachable(error.localizedDescription))
        }
    }

    /// Why `findRun` found no run.
    enum RunLookupError: Error, Equatable {
        case notFound
        case unreachable(String)
    }

    /// Load a run for VRLOG replay.
    func loadRunForReplay(_ runID: String) async -> VRLogLoadResponse? {
        guard !isLoadingReplay else { return nil }
        guard runs.first(where: { $0.runId == runID }) != nil else {
            error = "Run not found"
            return nil
        }

        isLoadingReplay = true
        error = nil

        do {
            let loadResponse = try await apiClient.loadVRLog(runID: runID)
            selectedRunID = runID
            let frameEncoding = (loadResponse.frameEncoding ?? "unknown").lowercased()
            logger.info("Loaded VRLOG for run \(runID) using \(frameEncoding) frames")
            isLoadingReplay = false
            return loadResponse
        } catch {
            self.error = "Failed to load VRLOG: \(error.localizedDescription)"
            logger.error("Failed to load VRLOG: \(error.localizedDescription)")
            isLoadingReplay = false
            return nil
        }
    }

    /// Stop the current VRLOG replay.
    func stopReplay() async {
        do {
            try await apiClient.stopVRLog()
            selectedRunID = nil
            logger.info("Stopped VRLOG replay")
        } catch { logger.error("Failed to stop VRLOG: \(error.localizedDescription)") }
    }

    /// Refresh the run list.
    func refresh() async { await fetchRuns() }

    /// Prime local track-level label state for a run so menu rollups can update without refetching.
    func primeTrackCache(runID: String) async {
        if runTrackSnapshots[runID] != nil { return }

        do {
            let tracks = try await apiClient.listTracks(runID: runID)
            runTrackSnapshots[runID] = Dictionary(
                uniqueKeysWithValues: tracks.map { ($0.trackId, RunTrackLabelSnapshot(track: $0)) })
            updateRunLabelRollup(runID)
            logger.info("Primed run label cache for \(runID) with \(tracks.count) tracks")
        } catch {
            logger.error(
                "Failed to prime run label cache for \(runID): \(error.localizedDescription)")
        }
    }

    /// Update local rollup state after a successful label write.
    func applySuccessfulLabelUpdate(
        runID: String, trackID: String, userLabel: String? = nil, qualityLabel: String? = nil
    ) {
        guard var trackStates = runTrackSnapshots[runID], var snapshot = trackStates[trackID] else {
            return
        }

        if let userLabel {
            snapshot.userLabel = normaliseRunLabelValue(userLabel)
            snapshot.labelSource = "human_manual"
        }
        if let qualityLabel {
            snapshot.qualityLabel = normaliseRunLabelValue(qualityLabel)
            snapshot.labelSource = "human_manual"
        }

        trackStates[trackID] = snapshot
        runTrackSnapshots[runID] = trackStates
        updateRunLabelRollup(runID)
    }

    private func updateRunLabelRollup(_ runID: String) {
        guard let trackStates = runTrackSnapshots[runID],
            let index = runs.firstIndex(where: { $0.runId == runID })
        else { return }

        runs[index].labelRollup = computeRunLabelRollup(from: Array(trackStates.values))
    }
}
