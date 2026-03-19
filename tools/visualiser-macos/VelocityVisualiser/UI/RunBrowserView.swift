// RunBrowserView.swift
// UI for browsing and loading analysis runs.
//
// This view displays a list of available runs with VRLOG files
// and allows the user to load a run for replay and labelling.

import SwiftUI

private let runBrowserLogger = DevLogger(category: "RunBrowser")

private enum RunBrowserLayout {
    static let statusDotSize: CGFloat = 8
    static let runStatusSpacing: CGFloat = 4
    static let runWidth: CGFloat = 80
    static let dateWidth: CGFloat = 120
    static let replayCaseWidth: CGFloat = 40
    static let durationWidth: CGFloat = 60
    static let tracksWidth: CGFloat = 50
    static let labelsWidth: CGFloat = 54
    static let rowInset = EdgeInsets(top: 0, leading: 24, bottom: 0, trailing: 24)
}

@available(macOS 15.0, *) @MainActor func loadRunForReplayAndUpdateAppState(
    runID: String, appState: AppState, runBrowserState: RunBrowserState,
    loadRunForReplay: @escaping @MainActor () async -> VRLogLoadResponse?
) async {
    runBrowserLogger.debug("loadRunForReplayAndUpdateAppState() — runID=\(runID)")
    // Reset stale playback state before loading the new VRLOG.
    // This clears isPaused, replayFinished, progress, timestamps.
    appState.prepareForNewReplay()

    let loadResponse = await loadRunForReplay()
    runBrowserLogger.debug("loadRunForReplay returned success=\(loadResponse != nil)")
    if let loadResponse {
        appState.setReplayFrameEncoding(loadResponse.frameEncoding)
        // Set currentRunID so labels route to run-track API
        appState.currentRunID = runID
        await runBrowserState.primeTrackCache(runID: runID)
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

    /// Inject a shared run-browser state instance for the production sheet or tests.
    init(state: RunBrowserState) { _runBrowserState = StateObject(wrappedValue: state) }

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
                // Sticky header outside ScrollView. Both header and rows
                // use the same horizontal padding constant for alignment.
                RunBrowserHeaderRow().padding(.top, 6).padding(.bottom, 4).padding(
                    .horizontal, RunBrowserLayout.rowInset.leading)

                Divider()

                ScrollView {
                    LazyVStack(spacing: 0) {
                        ForEach(runBrowserState.runs) { run in
                            RunRowView(
                                run: run, isSelected: runBrowserState.selectedRunID == run.runId
                            ) {
                                Task {
                                    if runBrowserState.selectedRunID == run.runId {
                                        dismiss()
                                        return
                                    }
                                    guard run.hasVRLog else { return }
                                    await loadRunForReplayAndUpdateAppState(
                                        runID: run.runId, appState: appState,
                                        runBrowserState: runBrowserState
                                    ) { await runBrowserState.loadRunForReplay(run.runId) }
                                    if runBrowserState.selectedRunID == run.runId { dismiss() }
                                }
                            }
                        }
                    }
                }
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
                                appState.setReplayFrameEncoding(nil)
                                appState.setPlaybackMode(.live)
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
        }.frame(width: 450, height: 700).onAppear { Task { await runBrowserState.fetchRuns() } }
    }

}

private struct RunBrowserHeaderRow: View {
    var body: some View {
        HStack(spacing: 0) {
            HStack(spacing: RunBrowserLayout.runStatusSpacing) {
                Color.clear.frame(
                    width: RunBrowserLayout.statusDotSize, height: RunBrowserLayout.statusDotSize)
                Text("Run")
            }.frame(width: RunBrowserLayout.runWidth, alignment: .leading)
            Text("Date").frame(width: RunBrowserLayout.dateWidth, alignment: .leading)
            Text("Case").frame(width: RunBrowserLayout.replayCaseWidth, alignment: .leading)
            Text("Duration").frame(width: RunBrowserLayout.durationWidth, alignment: .trailing)
            Text("Tracks").frame(width: RunBrowserLayout.tracksWidth, alignment: .trailing)
            Text("Labels").frame(width: RunBrowserLayout.labelsWidth, alignment: .center)
        }.frame(maxWidth: .infinity, alignment: .leading).font(.caption).foregroundColor(.secondary)
    }
}

/// Row view for a single run — 6-column table layout.
@available(macOS 15.0, *) struct RunRowView: View {
    let run: AnalysisRun
    let isSelected: Bool
    let onSelect: () -> Void
    @State private var isHovered = false

    private var rowBackground: Color {
        if isSelected { return Color.accentColor.opacity(0.1) }
        if isHovered, run.hasVRLog { return Color.primary.opacity(0.08) }
        return Color.clear
    }

    var body: some View {
        Button(action: onSelect) {
            HStack(spacing: 0) {
                // Col 1: 0xfirst6uuid with status dot
                HStack(spacing: RunBrowserLayout.runStatusSpacing) {
                    StatusDot(status: run.status)
                    Text(run.shortIdPrefix).font(.system(.caption, design: .monospaced)).lineLimit(
                        1)
                }.frame(width: RunBrowserLayout.runWidth, alignment: .leading)

                // Col 2: Date/time (space-padded for monospaced alignment)
                Text(run.formattedDate).font(.system(.caption, design: .monospaced)).frame(
                    width: RunBrowserLayout.dateWidth, alignment: .leading
                ).lineLimit(1)

                // Col 3: Replay case name
                Text(run.replayCaseName ?? "-").font(.caption).frame(
                    width: RunBrowserLayout.replayCaseWidth, alignment: .leading
                ).lineLimit(1)

                // Col 4: Duration mm:ss
                Text(runRowFormatDuration(run.durationSecs)).font(
                    .system(.caption, design: .monospaced)
                ).frame(width: RunBrowserLayout.durationWidth, alignment: .trailing)

                // Col 5: Tracks count
                Text("\(run.totalTracks)").font(.system(.caption, design: .monospaced)).frame(
                    width: RunBrowserLayout.tracksWidth, alignment: .trailing)

                // Col 6: Label rollup
                RunLabelRollupIcon(rollup: run.labelRollup).frame(
                    width: RunBrowserLayout.labelsWidth, alignment: .center)
            }.frame(maxWidth: .infinity, alignment: .leading).padding(.vertical, 2).padding(
                .horizontal, RunBrowserLayout.rowInset.leading
            ).contentShape(Rectangle())
        }.buttonStyle(.plain).disabled(!run.hasVRLog).background(rowBackground).onHover {
            hovering in isHovered = hovering
        }
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
        Circle().fill(statusDotColour(status)).frame(
            width: RunBrowserLayout.statusDotSize, height: RunBrowserLayout.statusDotSize
        ).help("Status: \(status)")
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
                    Color.confirmedGreen.frame(width: classifiedWidth)
                    Color.accentColor.frame(width: taggedWidth)
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
