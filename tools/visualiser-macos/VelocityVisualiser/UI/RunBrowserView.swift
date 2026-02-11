// RunBrowserView.swift
// UI for browsing and loading analysis runs (Phase 4.1).
//
// This view displays a list of available runs with VRLOG files
// and allows the user to load a run for replay and labelling.

import SwiftUI

// MARK: - String Extension for Truncation

extension String {
    /// Truncate string with ellipsis. E.g. "abc123def456".truncated(8) -> "abc123de..."
    func truncated(_ maxLength: Int) -> String {
        if count <= maxLength { return self }
        return String(prefix(maxLength)) + "..."
    }
}

/// View for browsing and selecting analysis runs.
@available(macOS 15.0, *) struct RunBrowserView: View {
    @StateObject private var runBrowserState = RunBrowserState()
    @EnvironmentObject var appState: AppState
    @Environment(\.dismiss) private var dismiss

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
                // Run list
                List(runBrowserState.runs) { run in
                    RunRowView(run: run, isSelected: runBrowserState.selectedRunID == run.runId) {
                        Task { await loadRun(run) }
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
                            // Restore app state to live mode
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
        }.frame(width: 500, height: 400).onAppear { Task { await runBrowserState.fetchRuns() } }
    }

    private func loadRun(_ run: AnalysisRun) async {
        let success = await runBrowserState.loadRunForReplay(run.runId)
        if success {
            // Update app state to indicate we're in VRLOG replay mode
            appState.isLive = false
            // Set currentRunID so labels route to run-track API
            appState.currentRunID = run.runId
        }
    }
}

/// Row view for a single run in the list.
@available(macOS 15.0, *) struct RunRowView: View {
    let run: AnalysisRun
    let isSelected: Bool
    let onSelect: () -> Void

    var body: some View {
        HStack {
            // Status indicator
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 4) {
                    StatusDot(status: run.status)
                    Text(run.runId.truncated(12)).font(.system(.body, design: .monospaced))
                        .lineLimit(1)
                }

                Text(run.formattedDate).font(.caption).foregroundColor(.secondary)
            }

            Spacer()

            // Stats
            VStack(alignment: .trailing, spacing: 2) {
                Text("\(run.totalTracks) tracks").font(.caption)
                Text(formatDuration(run.durationSecs)).font(.caption).foregroundColor(.secondary)
            }

            // VRLOG indicator
            if run.hasVRLog {
                Image(systemName: "play.rectangle.fill").foregroundColor(.green).help(
                    "VRLOG available")
            } else {
                Image(systemName: "play.rectangle").foregroundColor(.gray).help("No VRLOG")
            }

            // Load button
            Button(action: onSelect) { Text(isSelected ? "Loaded" : "Load") }.buttonStyle(.bordered)
                .disabled(!run.hasVRLog || isSelected)
        }.padding(.vertical, 4).background(
            isSelected ? Color.accentColor.opacity(0.1) : Color.clear
        ).cornerRadius(4)
    }

    private func formatDuration(_ seconds: Double) -> String {
        let mins = Int(seconds) / 60
        let secs = Int(seconds) % 60
        return String(format: "%d:%02d", mins, secs)
    }
}

/// Status indicator dot for run status.
struct StatusDot: View {
    let status: String

    private var color: Color {
        switch status {
        case "completed": return .green
        case "running": return .orange
        case "failed": return .red
        default: return .gray
        }
    }

    var body: some View {
        Circle().fill(color).frame(width: 8, height: 8).help("Status: \(status)")
    }
}

// MARK: - Preview

@available(macOS 15.0, *) #Preview { RunBrowserView().environmentObject(AppState()) }
