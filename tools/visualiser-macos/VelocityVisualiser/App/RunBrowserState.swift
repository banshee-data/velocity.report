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
        self.linkedTrackIDs = track.linkedTrackIDs ?? []
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
    let isLegacyTagLabel = userLabel == "split" || userLabel == "merge"
    let hasTagOnlyState =
        qualityLabel != nil || isLegacyTagLabel || snapshot.isSplitCandidate
        || snapshot.isMergeCandidate || !snapshot.linkedTrackIDs.isEmpty

    if hasManualLabel, let userLabel, !isLegacyTagLabel, !userLabel.isEmpty { return .classified }
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

    @Published var runs: [AnalysisRun] = []
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
        isLoading = true
        error = nil

        do {
            let fetchedRuns = try await apiClient.listRuns(limit: 50)
            runs = fetchedRuns
            logger.info("Fetched \(fetchedRuns.count) runs")
        } catch {
            self.error = "Failed to fetch runs: \(error.localizedDescription)"
            logger.error("Failed to fetch runs: \(error.localizedDescription)")
        }

        isLoading = false
    }

    /// Load a run for VRLOG replay.
    func loadRunForReplay(_ runID: String) async -> Bool {
        guard runs.first(where: { $0.runId == runID }) != nil else {
            error = "Run not found"
            return false
        }

        isLoadingReplay = true
        error = nil

        do {
            try await apiClient.loadVRLog(runID: runID)
            selectedRunID = runID
            logger.info("Loaded VRLOG for run \(runID)")
            isLoadingReplay = false
            return true
        } catch {
            self.error = "Failed to load VRLOG: \(error.localizedDescription)"
            logger.error("Failed to load VRLOG: \(error.localizedDescription)")
            isLoadingReplay = false
            return false
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
