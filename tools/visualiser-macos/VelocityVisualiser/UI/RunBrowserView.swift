// RunBrowserView.swift
// UI for browsing and loading analysis runs.
//
// This view displays a list of available runs with VRLOG files
// and allows the user to load a run for replay and labelling.

import SwiftUI

private let runBrowserLogger = DevLogger(category: "RunBrowser")

@available(macOS 15.0, *) @MainActor func loadRunForReplayAndUpdateAppState(
    runID: String, appState: AppState, loadRunForReplay: @escaping @MainActor () async -> Bool
) async {
    runBrowserLogger.debug("loadRunForReplayAndUpdateAppState() — runID=\(runID)")
    // Reset stale playback state before loading the new VRLOG.
    // This clears isPaused, replayFinished, progress, timestamps.
    appState.prepareForNewReplay()

    let success = await loadRunForReplay()
    runBrowserLogger.debug("loadRunForReplay returned success=\(success)")
    if success {
        // Update app state to indicate we're in VRLOG replay mode
        appState.isLive = false
        // Set currentRunID so labels route to run-track API
        appState.currentRunID = runID
        await appState.runBrowserState.primeTrackCache(runID: runID)
        // Restart the gRPC stream AFTER the VRLOG has loaded on the
        // server.  Doing this before the HTTP POST would disconnect
        // the client while the server starts broadcasting, causing
        // frames_sent=0 (frames lost before the new stream connects).
        appState.restartGRPCStream()
        runBrowserLogger.debug("gRPC stream restarted for run \(runID)")
    }
}

/// View for browsing and selecting analysis runs.
@available(macOS 15.0, *) struct RunBrowserView: View {
    @StateObject private var runBrowserState: RunBrowserState
    @EnvironmentObject var appState: AppState
    @Environment(\.dismiss) private var dismiss

    init() { _runBrowserState = StateObject(wrappedValue: RunBrowserState()) }

    /// Test-only initialiser accepting a pre-configured state.
    init(state: RunBrowserState) { _runBrowserState = StateObject(wrappedValue: state) }

    /// Sheet height scales with item count: min 300, expands ~28pt per row, max 700.
    private var preferredHeight: CGFloat {
        if runBrowserState.runs.isEmpty { return 300 }
        let chrome: CGFloat = 120  // header + column header + footer + dividers
        let rows = CGFloat(runBrowserState.runs.count) * 28
        return min(max(chrome + rows, 300), 700)
    }

    var body: some View {
        VStack(spacing: 0) {
            // Header
            HStack {
                Text("Analysis Runs").font(.headline)
                Spacer()
                Button(action: { Task { await runBrowserState.refresh() } }) {
                    Image(systemName: "arrow.clockwise")
                }.buttonStyle(.borderless).disabled(runBrowserState.isLoading)
            }.padding()

            Divider()

            // Content
            if runBrowserState.isLoading && runBrowserState.runs.isEmpty {
                Spacer()
                ProgressView("Loading runs...").padding()
                Spacer()
            } else if let error = runBrowserState.error {
                Spacer()
                VStack(spacing: 8) {
                    Image(systemName: "exclamationmark.triangle").font(.largeTitle).foregroundColor(
                        .orange)
                    Text(error).foregroundColor(.secondary).multilineTextAlignment(.center)
                    Button("Retry") { Task { await runBrowserState.refresh() } }
                }.padding()
                Spacer()
            } else if runBrowserState.runs.isEmpty {
                Spacer()
                VStack(spacing: 8) {
                    Image(systemName: "doc.text.magnifyingglass").font(.largeTitle).foregroundColor(
                        .secondary)
                    Text("No runs available").foregroundColor(.secondary)
                    Text("Complete an analysis run to see it here").font(.caption).foregroundColor(
                        .secondary)
                }.padding()
                Spacer()
            } else {
                // Column headers
                HStack(spacing: 0) {
                    Text("Run").frame(width: 80, alignment: .leading)
                    Text("Date").frame(width: 130, alignment: .leading)
                    Text("Scene").frame(width: 80, alignment: .leading)
                    Text("Duration").frame(width: 60, alignment: .trailing)
                    Text("Tracks").frame(width: 50, alignment: .trailing)
                    Text("Labels").frame(width: 54, alignment: .center)
                    Spacer().frame(width: 70)  // Load button column
                }.font(.caption).foregroundColor(.secondary).padding(.horizontal, 20).padding(
                    .top, 6)

                // Run list
                List(runBrowserState.runs) { run in
                    RunRowView(run: run, isSelected: runBrowserState.selectedRunID == run.runId) {
                        Task {
                            await loadRunForReplayAndUpdateAppState(
                                runID: run.runId, appState: appState
                            ) { await runBrowserState.loadRunForReplay(run.runId) }
                        }
                    }
                }.listStyle(.inset)
            }

            Divider()

            // Footer
            HStack {
                if let selectedRunID = runBrowserState.selectedRunID {
                    Text("Loaded: \(selectedRunID)").font(.caption).foregroundColor(.secondary)
                        .lineLimit(1)
                    Spacer()
                    Button("Stop Replay") {
                        Task {
                            await runBrowserState.stopReplay()
                            await MainActor.run {
                                appState.isLive = true
                                appState.currentRunID = nil
                            }
                        }
                    }.buttonStyle(.bordered)
                } else {
                    Text("Select a run to load for labelling").font(.caption).foregroundColor(
                        .secondary)
                    Spacer()
                }
                Button("Close") { dismiss() }.buttonStyle(.bordered)
            }.padding()
        }.frame(width: 640, height: preferredHeight).onAppear {
            Task { await runBrowserState.fetchRuns() }
        }
    }

}

/// Row view for a single run — 6-column table layout.
@available(macOS 15.0, *) struct RunRowView: View {
    let run: AnalysisRun
    let isSelected: Bool
    let onSelect: () -> Void
    @State private var isHovered = false

    var body: some View {
        HStack(spacing: 0) {
            // Col 1: 0xfirst6uuid with status dot
            HStack(spacing: 4) {
                StatusDot(status: run.status)
                Text(run.shortIdPrefix).font(.system(.caption, design: .monospaced)).lineLimit(1)
            }.frame(width: 80, alignment: .leading)

            // Col 2: Date/time (space-padded for monospaced alignment)
            Text(run.formattedDate).font(.system(.caption, design: .monospaced)).frame(
                width: 130, alignment: .leading
            ).lineLimit(1)

            // Col 3: Scene name
            Text(run.sceneName ?? "-").font(.caption).frame(width: 80, alignment: .leading)
                .lineLimit(1)

            // Col 4: Duration mm:ss
            Text(runRowFormatDuration(run.durationSecs)).font(
                .system(.caption, design: .monospaced)
            ).frame(width: 60, alignment: .trailing)

            // Col 5: Tracks count
            Text("\(run.totalTracks)").font(.system(.caption, design: .monospaced)).frame(
                width: 50, alignment: .trailing)

            // Col 6: Label rollup
            RunLabelRollupIcon(rollup: run.labelRollup).frame(width: 54, alignment: .center)

            // Load button
            Button(action: onSelect) { Text(isSelected ? "Loaded" : "Load") }.buttonStyle(.bordered)
                .controlSize(.small).disabled(isSelected || !run.hasVRLog).frame(
                    width: 70, alignment: .trailing)
        }.frame(maxWidth: .infinity, alignment: .leading).padding(.vertical, 2).contentShape(
            Rectangle()
        ).background(
            isSelected
                ? Color.accentColor.opacity(0.1)
                : (isHovered ? Color.primary.opacity(0.08) : Color.clear)
        ).cornerRadius(4).onHover { hovering in isHovered = hovering }
    }
}

/// Map a run status string to a display colour.
/// Extracted from StatusDot for testability.
func statusDotColour(_ status: String) -> Color {
    switch status {
    case "completed": return .green
    case "running": return .orange
    case "failed": return .red
    default: return .gray
    }
}

/// Format a duration in seconds as M:SS.
/// Extracted from RunRowView for testability.
func runRowFormatDuration(_ seconds: Double) -> String {
    let mins = Int(seconds) / 60
    let secs = Int(seconds) % 60
    return String(format: "%d:%02d", mins, secs)
}

/// Status indicator dot for run status.
struct StatusDot: View {
    let status: String

    var body: some View {
        Circle().fill(statusDotColour(status)).frame(width: 8, height: 8).help("Status: \(status)")
    }
}

struct RunLabelRollupIcon: View {
    let rollup: RunLabelRollup?

    var body: some View {
        GeometryReader { geometry in
            let width = geometry.size.width
            let height = geometry.size.height
            let classifiedWidth = width * (rollup?.classifiedFraction ?? 0)
            let taggedWidth = width * (rollup?.taggedOnlyFraction ?? 0)
            let unlabelledWidth = max(0, width - classifiedWidth - taggedWidth)

            ZStack {
                Capsule().fill(Color.gray.opacity(0.18))
                HStack(spacing: 0) {
                    Color.green.frame(width: classifiedWidth)
                    Color.purple.frame(width: taggedWidth)
                    Color.gray.opacity(0.45).frame(width: unlabelledWidth)
                }.clipShape(Capsule())
                Capsule().stroke(Color.secondary.opacity(0.35), lineWidth: 1)
            }.frame(width: width, height: height)
        }.frame(width: 42, height: 10).help(rollup?.helpText ?? "No label rollup available")
            .accessibilityLabel(rollup?.helpText ?? "No label rollup available")
    }
}

// MARK: - Preview

@available(macOS 15.0, *) #Preview { RunBrowserView().environmentObject(AppState()) }
