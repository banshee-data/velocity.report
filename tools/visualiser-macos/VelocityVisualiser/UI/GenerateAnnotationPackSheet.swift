// GenerateAnnotationPackSheet.swift
// Generates an annotation pack from a recorded run, without leaving the app.
//
// Before this, producing a pack meant finding the run's VRLOG directory on
// disk and running `velocity lidar annotation-export` from a terminal before
// the annotation window could open anything. The export itself was always
// a thin wrapper around one Go function; the missing piece was reaching it
// from here, which is what this sheet and its API client are for.

import Combine
import SwiftUI

private let generatePackLogger = DevLogger(category: "AnnotationExport")

/// State for the run picker + export request.
///
/// Separate from AnnotationController: this talks to the HTTP API and never
/// touches a pack on disk directly, so it has nothing in common with the
/// controller beyond handing it a directory to open when the export succeeds.
@MainActor final class GenerateAnnotationPackState: ObservableObject {
    @Published private(set) var runs: [AnalysisRun] = []
    @Published private(set) var isLoadingRuns = false
    @Published private(set) var isExporting = false
    @Published var lastError: String?

    @Published var selectedRunID: String?
    @Published var coverage: AnnotationCoverage = .full
    @Published var coverageNote: String = ""
    /// Empty means "use the server's own default (200)". A blank field reads
    /// more honestly than pre-filling 200, which would look like a value the
    /// operator chose rather than the cap that applies regardless.
    @Published var maxSamplesText: String = ""

    private let runsClient: RunTrackLabelAPIClient
    private let exportClient: AnnotationExportAPIClient

    init(
        runsClient: RunTrackLabelAPIClient = RunTrackLabelAPIClient(),
        exportClient: AnnotationExportAPIClient = AnnotationExportAPIClient()
    ) {
        self.runsClient = runsClient
        self.exportClient = exportClient
    }

    /// Runs with no VRLOG cannot be exported from, so they are filtered out
    /// here rather than merely disabled in the list: a picker full of mostly
    /// unusable rows is worse than a shorter, all-usable one.
    var exportableRuns: [AnalysisRun] { runs.filter { $0.hasVRLog } }

    func loadRuns() async {
        isLoadingRuns = true
        defer { isLoadingRuns = false }
        do {
            runs = try await runsClient.listRuns()
            if selectedRunID == nil { selectedRunID = exportableRuns.first?.id }
        } catch {
            lastError = "Could not load runs: \(error.localizedDescription)"
            generatePackLogger.error("Failed to load runs: \(error)")
        }
    }

    /// Parses maxSamplesText, returning nil for blank input (server default)
    /// and reporting a bad value rather than silently ignoring it — typing
    /// "2oo" and getting the server's 200 instead would read as success.
    private func parsedMaxSamples() -> Int? {
        let trimmed = maxSamplesText.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else { return nil }
        return Int(trimmed)
    }

    /// Runs the export, returning the pack directory on success. `nil`
    /// return with `lastError` set is a validation failure the caller need
    /// not log again.
    func generate() async -> URL? {
        guard let runID = selectedRunID else {
            lastError = "Choose a run first"
            return nil
        }
        let maxSamples: Int
        if let parsed = parsedMaxSamples() {
            guard parsed > 0 else {
                lastError = "Max samples must be a positive number, or blank for the default"
                return nil
            }
            maxSamples = parsed
        } else {
            maxSamples = 0
        }

        isExporting = true
        lastError = nil
        defer { isExporting = false }
        do {
            let result = try await exportClient.exportPack(
                runID: runID, coverage: coverage, coverageNote: coverageNote, maxSamples: maxSamples
            )
            generatePackLogger.info(
                "Exported pack \(result.datasetID) from run \(runID): \(result.sampleCount) samples"
            )
            return URL(fileURLWithPath: result.packDir)
        } catch {
            lastError = error.localizedDescription
            generatePackLogger.error("Export failed for run \(runID): \(error)")
            return nil
        }
    }
}

/// Sheet: pick a run, state its coverage, generate. On success it calls
/// `onGenerated` with the pack directory and dismisses itself; the caller
/// opens it, so this view never touches AnnotationController directly.
struct GenerateAnnotationPackSheet: View {
    @StateObject private var state = GenerateAnnotationPackState()
    @Environment(\.dismiss) private var dismiss
    let onGenerated: (URL) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                Text("Generate Annotation Pack").font(.headline)
                Spacer()
                Button(action: { Task { await state.loadRuns() } }) {
                    Image(systemName: "arrow.clockwise")
                }.buttonStyle(.borderless).disabled(state.isLoadingRuns)
            }

            runPicker

            Picker("Coverage", selection: $state.coverage) {
                ForEach(AnnotationCoverage.allCases) { Text($0.label).tag($0) }
            }
            // Coverage is stated, never guessed, on the export side too: see
            // internal/lidar/annotation.Export. The picker exists so that
            // choice is made here rather than defaulting silently to "full"
            // for a foreground-only recording.
            Text(
                "What the run's recording could see. A foreground-only run cannot support whole-scene segmentation."
            ).font(.caption).foregroundStyle(.secondary)

            TextField("Coverage note (optional)", text: $state.coverageNote).textFieldStyle(
                .roundedBorder)

            HStack {
                Text("Max samples")
                TextField("200", text: $state.maxSamplesText).textFieldStyle(.roundedBorder).frame(
                    width: 80)
                Text("blank = server default").font(.caption).foregroundStyle(.secondary)
            }

            if let error = state.lastError {
                Text(error).font(.caption).foregroundStyle(.red).fixedSize(
                    horizontal: false, vertical: true)
            }

            HStack {
                Spacer()
                Button("Cancel") { dismiss() }
                Button("Generate") {
                    Task {
                        if let packDir = await state.generate() {
                            onGenerated(packDir)
                            dismiss()
                        }
                    }
                }.keyboardShortcut(.defaultAction).disabled(
                    state.isExporting || state.selectedRunID == nil)
                if state.isExporting { ProgressView().controlSize(.small) }
            }
        }.padding(20).frame(width: 420).task { await state.loadRuns() }
    }

    @ViewBuilder private var runPicker: some View {
        if state.isLoadingRuns && state.runs.isEmpty {
            ProgressView("Loading runs…")
        } else if state.exportableRuns.isEmpty {
            Text("No runs with a VRLOG recording are available yet.").font(.caption)
                .foregroundStyle(.secondary)
        } else {
            Picker("Run", selection: $state.selectedRunID) {
                ForEach(state.exportableRuns) { run in
                    Text("\(run.shortIdPrefix) · \(run.formattedDate)").tag(Optional(run.id))
                }
            }
        }
    }
}
