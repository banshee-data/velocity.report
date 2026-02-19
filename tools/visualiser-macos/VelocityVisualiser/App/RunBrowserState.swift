// RunBrowserState.swift
// State management for the run browser.
//
// This manages the list of available runs and loading runs for replay.

import Combine
import Foundation
import os

private let logger = Logger(subsystem: "report.velocity.visualiser", category: "RunBrowserState")

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
}
