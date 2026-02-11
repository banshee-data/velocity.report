// ContentView.swift
// Main content view for the Velocity Visualiser.
//
// This view composes the Metal render view with SwiftUI controls.

import MetalKit
import SwiftUI

struct ContentView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        HSplitView {
            // Main 3D view
            VStack(spacing: 0) {
                // Toolbar
                ToolbarView()

                // Metal view - frames are delivered directly to renderer via AppState
                ZStack {
                    MetalViewRepresentable(
                        showPoints: appState.showPoints, showBoxes: appState.showBoxes,
                        showClusters: appState.showClusters,  // M4
                        showTrails: appState.showTrails, showDebug: appState.showDebug,  // M6
                        pointSize: appState.pointSize,
                        onRendererCreated: { renderer in appState.registerRenderer(renderer) },
                        onTrackSelected: { trackID in appState.selectTrack(trackID) },
                        onCameraChanged: { appState.reprojectLabels() })

                    // Track label overlay (SwiftUI text positioned over 3D tracks)
                    if appState.showTrackLabels {
                        TrackLabelOverlay(labels: appState.trackLabels).allowsHitTesting(false)
                    }

                    // Capture Metal view size for label projection
                    GeometryReader { geometry in
                        Color.clear.onAppear { appState.metalViewSize = geometry.size }.onChange(
                            of: geometry.size
                        ) { _, newSize in
                            // Defer to next run loop to avoid AttributeGraph cycle
                            Task { @MainActor in appState.metalViewSize = newSize }
                        }
                    }.allowsHitTesting(false)
                }.frame(minWidth: 400, minHeight: 300)

                // Playback controls
                PlaybackControlsView()
            }.frame(minWidth: 600)

            // Side panel
            if appState.showSidePanel || appState.selectedTrackID != nil {
                SidePanelView().frame(width: 280)
            }
        }.frame(minWidth: 800, minHeight: 600)  // Keyboard shortcuts for playback
            .onKeyPress(.space) {
                appState.togglePlayPause()
                return .handled
            }.onKeyPress(",") {
                guard appState.isSeekable else { return .ignored }
                appState.stepBackward()
                return .handled
            }.onKeyPress(".") {
                guard appState.isSeekable else { return .ignored }
                appState.stepForward()
                return .handled
            }.onKeyPress("[") {
                appState.decreaseRate()
                return .handled
            }.onKeyPress("]") {
                appState.increaseRate()
                return .handled
            }  // Run browser sheet (Phase 4.1)
            .sheet(isPresented: $appState.showRunBrowser) {
                RunBrowserView().environmentObject(appState)
            }
    }
}

// MARK: - Toolbar

struct ToolbarView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        HStack {
            // Connection button
            ConnectionButtonView()

            Divider().frame(height: 20)

            // Connection status
            ConnectionStatusView()

            // Run browser button (Phase 4.1)
            if appState.isConnected {
                Divider().frame(height: 20)
                Button(action: { appState.showRunBrowser = true }) {
                    Label("Runs", systemImage: "doc.text.magnifyingglass")
                }.help("Browse analysis runs")

                Divider().frame(height: 20)
                Button(action: { appState.showSidePanel.toggle() }) {
                    Label("Inspector", systemImage: "sidebar.trailing")
                }.help("Toggle track inspector")
            }

            Spacer()

            // Stats (only render when connected)
            StatsDisplayView()

            Spacer()

            // Overlay toggles
            OverlayTogglesView()
        }.padding(.horizontal).padding(.vertical, 8).background(
            Color(nsColor: .controlBackgroundColor))
    }
}

// Extracted to break dependency cycle
struct ConnectionButtonView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        let isConnected = appState.isConnected
        let isConnecting = appState.isConnecting

        Button(action: { appState.toggleConnection() }) {
            if isConnecting {
                ProgressView().controlSize(.small).frame(width: 16, height: 16)
            } else {
                Image(systemName: isConnected ? "stop.circle.fill" : "play.circle.fill")
            }
            Text(isConnecting ? "Connecting..." : (isConnected ? "Disconnect" : "Connect"))
        }.tint(isConnected ? .red : .green).disabled(isConnecting)
    }
}

struct ConnectionStatusView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        let isConnected = appState.isConnected
        let errorMessage = appState.connectionError
        let hasError = errorMessage != nil

        HStack(spacing: 4) {
            Circle().fill(isConnected ? .green : (hasError ? .red : .gray)).frame(
                width: 8, height: 8)
            Text(errorMessage ?? (isConnected ? appState.serverAddress : "Disconnected")).font(
                .caption
            ).foregroundColor(hasError ? .red : .secondary)
        }
    }
}

struct StatsDisplayView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        if appState.isConnected {
            let fps = appState.fps
            let pointCount = appState.pointCount
            let trackCount = appState.trackCount
            let cacheStatus = appState.cacheStatus

            HStack(spacing: 16) {
                StatLabel(title: "FPS", value: String(format: "%.1f", fps))
                StatLabel(title: "Points", value: formatNumber(pointCount))
                StatLabel(title: "Tracks", value: "\(trackCount)")

                // M3.5: Cache status indicator
                if !cacheStatus.isEmpty && cacheStatus != "Not using split streaming" {
                    CacheStatusLabel(status: cacheStatus)
                }
            }.fixedSize()  // Prevent compression when viewport shrinks
        }
    }

    private func formatNumber(_ n: Int) -> String {
        if n >= 1000 { return String(format: "%.2fk", Double(n) / 1000) }
        return "\(n)"
    }
}

// M3.5: Cache status label with colour-coded indicator
struct CacheStatusLabel: View {
    let status: String

    private var statusColour: Color {
        if status.contains("Cached") {
            return .green
        } else if status.contains("Refreshing") {
            return .orange
        } else {
            return .gray
        }
    }

    var body: some View {
        HStack(spacing: 4) {
            Circle().fill(statusColour).frame(width: 6, height: 6)
            Text("BG").font(.caption2).foregroundColor(.secondary)
        }.help("Background cache: \(status)")
    }
}

struct StatLabel: View {
    let title: String
    let value: String

    var body: some View {
        VStack(alignment: .trailing, spacing: 0) {
            Text(title).font(.caption2).foregroundColor(.secondary)
            Text(value).font(.system(.caption, design: .monospaced)).fontWeight(.medium).frame(
                width: 50, alignment: .trailing)
        }
    }
}

// MARK: - Overlay Toggles

struct OverlayTogglesView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        HStack(spacing: 8) {
            ToggleButton(label: "P", isOn: $appState.showPoints, help: "Points")
            ToggleButton(label: "B", isOn: $appState.showBoxes, help: "Boxes")
            ToggleButton(label: "C", isOn: $appState.showClusters, help: "Clusters")  // M4
            ToggleButton(label: "T", isOn: $appState.showTrails, help: "Trails")
            ToggleButton(label: "V", isOn: $appState.showVelocity, help: "Velocity")
            ToggleButton(label: "L", isOn: $appState.showTrackLabels, help: "Track Labels")

            Divider().frame(height: 20)

            // Point size adjustment
            HStack(spacing: 4) {
                Text("Size").font(.caption2).foregroundColor(.secondary)
                Slider(value: $appState.pointSize, in: 1...20).frame(width: 60)
                Text("\(Int(appState.pointSize))").font(.caption).monospacedDigit().frame(width: 20)
            }.help("Point Size")

            Divider().frame(height: 20)

            ToggleButton(
                label: "D",
                isOn: Binding(get: { appState.showDebug }, set: { _ in appState.toggleDebug() }),
                help: "Debug Overlays")
        }.fixedSize()  // Prevent compression when viewport shrinks
    }
}

struct ToggleButton: View {
    let label: String
    @Binding var isOn: Bool
    let help: String

    var body: some View {
        Button(action: { isOn.toggle() }) {
            Text(label).font(.system(.caption, design: .monospaced)).fontWeight(
                isOn ? .bold : .regular
            ).foregroundColor(isOn ? .white : .secondary).frame(width: 24, height: 24).background(
                isOn ? Color.accentColor : Color(nsColor: .controlBackgroundColor).opacity(0.5)
            ).cornerRadius(4)
        }.buttonStyle(.plain).focusable(false).help(help)
    }
}

// MARK: - Playback Controls

/// Format playback rate for display: "0.5", "1", "2", "64" etc.
private func formatRate(_ rate: Float) -> String {
    if rate == Float(Int(rate)) {
        return String(Int(rate))
    } else {
        return String(format: "%.1f", rate)
    }
}

/// Format nanosecond duration as MM:SS or HH:MM:SS.
private func formatDuration(_ nanos: Int64) -> String {
    let totalSeconds = abs(nanos) / 1_000_000_000
    let hours = totalSeconds / 3600
    let minutes = (totalSeconds % 3600) / 60
    let seconds = totalSeconds % 60
    let prefix = nanos < 0 ? "-" : ""
    if hours > 0 { return String(format: "%@%d:%02d:%02d", prefix, hours, minutes, seconds) }
    return String(format: "%@%d:%02d", prefix, minutes, seconds)
}

struct PlaybackControlsView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        let isConnected = appState.isConnected
        let isLive = appState.isLive
        let isPaused = appState.isPaused
        let playbackRate = appState.playbackRate
        let isSeekable = appState.isSeekable

        HStack {
            // Play/Pause (disabled in live mode)
            Button(action: { appState.togglePlayPause() }) {
                Image(systemName: isPaused ? "play.fill" : "pause.fill")
            }.disabled(!isConnected || isLive)

            // Step buttons (only for seekable modes like .vrlog replay)
            if isSeekable {
                Button(action: { appState.stepBackward() }) {
                    Image(systemName: "backward.frame.fill")
                }.disabled(!isConnected || isLive)

                Button(action: { appState.stepForward() }) {
                    Image(systemName: "forward.frame.fill")
                }.disabled(!isConnected || isLive)
            }

            // Timeline (replay mode)
            if !isLive {
                if isSeekable {
                    // Interactive seek slider for .vrlog replay
                    Slider(value: $appState.replayProgress, in: 0...1) { editing in
                        appState.setSliderEditing(editing)
                        if !editing { appState.seek(to: appState.replayProgress) }
                    }.frame(minWidth: 200)
                } else {
                    // Read-only progress bar for PCAP replay
                    Slider(value: $appState.replayProgress, in: 0...1).frame(minWidth: 200)
                        .disabled(true)
                }

                // Time display
                TimeDisplayView()
            } else {
                Spacer()
            }

            // Rate control (disabled in live mode)
            HStack(spacing: 4) {
                Button(action: { appState.decreaseRate() }) { Image(systemName: "minus") }
                    .buttonStyle(.borderless).disabled(!isConnected || isLive)

                // Rate display: number + clickable "x" to reset to 1x
                HStack(spacing: 0) {
                    Text(formatRate(playbackRate)).font(.caption).monospacedDigit()
                    Button(action: { appState.resetRate() }) { Text("x").font(.caption) }
                        .buttonStyle(.borderless).disabled(!isConnected || isLive)
                }.frame(width: 45).foregroundColor(isLive ? .secondary : .primary)

                Button(action: { appState.increaseRate() }) { Image(systemName: "plus") }
                    .buttonStyle(.borderless).disabled(!isConnected || isLive)
            }.opacity(isLive ? 0.5 : 1.0)

            // Mode indicator (only show when connected)
            ModeIndicatorView(isLive: isLive, isConnected: isConnected)
        }.padding(.horizontal).padding(.vertical, 8).background(
            Color(nsColor: .controlBackgroundColor))
    }
}

/// Displays elapsed/total or remaining/total time. Click to toggle.
struct TimeDisplayView: View {
    @EnvironmentObject var appState: AppState
    @State private var showRemaining: Bool = false

    private var elapsed: Int64 { appState.currentTimestamp - appState.logStartTimestamp }

    private var total: Int64 { appState.logEndTimestamp - appState.logStartTimestamp }

    private var remaining: Int64 { appState.logEndTimestamp - appState.currentTimestamp }

    var body: some View {
        let currentText = showRemaining ? formatDuration(-remaining) : formatDuration(elapsed)
        let totalText = formatDuration(total)

        Button(action: { showRemaining.toggle() }) {
            Text("\(currentText) / \(totalText)").font(.system(.caption, design: .monospaced))
                .foregroundColor(.secondary)
        }.buttonStyle(.plain).help(
            showRemaining ? "Showing remaining time" : "Showing elapsed time")
    }
}

struct ModeIndicatorView: View {
    let isLive: Bool
    let isConnected: Bool

    var body: some View {
        if isConnected {
            Text(isLive ? "LIVE" : "REPLAY").font(.caption).fontWeight(.bold).foregroundColor(
                isLive ? .red : .orange
            ).padding(.horizontal, 8).padding(.vertical, 2).background(
                isLive ? Color.red.opacity(0.2) : Color.orange.opacity(0.2)
            ).cornerRadius(4)
        }
    }
}

// MARK: - Side Panel

struct SidePanelView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        ScrollView {
            HStack(alignment: .top, spacing: 16) {
                VStack(alignment: .leading, spacing: 16) {
                    // Track info
                    if let trackID = appState.selectedTrackID {
                        TrackInspectorView(trackID: trackID)
                    }

                    Divider()

                    // Label panel
                    LabelPanelView()

                    Divider()

                    // Debug overlay toggles
                    DebugOverlayTogglesView()

                    Divider()

                    // Export
                    Button(action: { appState.exportLabels() }) {
                        Label("Export Labels", systemImage: "square.and.arrow.up")
                    }.disabled(!appState.isConnected)

                    Spacer()
                }.frame(maxWidth: .infinity, alignment: .leading)

                Divider()

                VStack(alignment: .leading, spacing: 16) {
                    // Track list for selecting tracks
                    TrackListView()
                    Spacer()
                }.frame(width: 220, alignment: .leading)
            }.padding()
        }.background(Color(nsColor: .controlBackgroundColor))
    }
}

// MARK: - Track Inspector

struct TrackInspectorView: View {
    let trackID: String
    @EnvironmentObject var appState: AppState

    /// Find the current track data from the latest frame.
    private var track: Track? {
        appState.currentFrame?.tracks?.tracks.first(where: { $0.trackID == trackID })
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Track Inspector").font(.headline)
                Spacer()
                Button(action: { appState.selectTrack(nil) }) {
                    Image(systemName: "xmark.circle.fill").foregroundColor(.secondary)
                }.buttonStyle(.plain)
            }

            Text("ID: \(trackID)").font(.caption).foregroundColor(.secondary)

            if let t = track {
                Divider()

                // Position
                GroupBox(label: Text("Position").font(.caption2)) {
                    VStack(alignment: .leading, spacing: 2) {
                        DetailRow(label: "X", value: String(format: "%.2f m", t.x))
                        DetailRow(label: "Y", value: String(format: "%.2f m", t.y))
                        DetailRow(label: "Z", value: String(format: "%.2f m", t.z))
                    }
                }

                // Velocity
                GroupBox(label: Text("Velocity").font(.caption2)) {
                    VStack(alignment: .leading, spacing: 2) {
                        DetailRow(label: "Speed", value: String(format: "%.1f m/s", t.speedMps))
                        DetailRow(
                            label: "Heading",
                            value: String(format: "%.1f°", t.headingRad * 180 / .pi))
                        DetailRow(label: "Peak", value: String(format: "%.1f m/s", t.peakSpeedMps))
                        DetailRow(
                            label: "Average", value: String(format: "%.1f m/s", t.avgSpeedMps))
                    }
                }

                // Dimensions
                GroupBox(label: Text("Dimensions").font(.caption2)) {
                    VStack(alignment: .leading, spacing: 2) {
                        DetailRow(
                            label: "L×W×H",
                            value: String(
                                format: "%.1f × %.1f × %.1f m", t.bboxLengthAvg, t.bboxWidthAvg,
                                t.bboxHeightAvg))
                        DetailRow(
                            label: "OBB Heading",
                            value: String(format: "%.1f°", t.bboxHeadingRad * 180 / .pi))
                    }
                }

                // State
                GroupBox(label: Text("State").font(.caption2)) {
                    VStack(alignment: .leading, spacing: 2) {
                        HStack {
                            Text("State").font(.caption).foregroundColor(.secondary)
                            Spacer()
                            Text(stateLabel(t.state)).font(.caption).fontWeight(.medium)
                                .foregroundColor(stateColour(t.state))
                        }
                        DetailRow(label: "Hits", value: "\(t.hits)")
                        DetailRow(label: "Misses", value: "\(t.misses)")
                        DetailRow(
                            label: "Confidence", value: String(format: "%.0f%%", t.confidence * 100)
                        )
                        DetailRow(
                            label: "Duration", value: String(format: "%.1f s", t.trackDurationSecs))
                        DetailRow(
                            label: "Length", value: String(format: "%.1f m", t.trackLengthMetres))
                        if !t.classLabel.isEmpty { DetailRow(label: "Class", value: t.classLabel) }
                    }
                }
            } else {
                Text("Track data unavailable").font(.caption).foregroundColor(.secondary)
            }
        }
    }

    private func stateLabel(_ state: TrackState) -> String {
        switch state {
        case .unknown: return "Unknown"
        case .tentative: return "Tentative"
        case .confirmed: return "Confirmed"
        case .deleted: return "Deleted"
        }
    }

    private func stateColour(_ state: TrackState) -> Color {
        switch state {
        case .unknown: return .gray
        case .tentative: return .yellow
        case .confirmed: return .green
        case .deleted: return .red
        }
    }
}

/// Helper view for a label-value row in the inspector.
struct DetailRow: View {
    let label: String
    let value: String

    var body: some View {
        HStack {
            Text(label).font(.caption).foregroundColor(.secondary)
            Spacer()
            Text(value).font(.system(.caption, design: .monospaced))
        }
    }
}

// MARK: - Track List

/// Track list for selecting a track for labelling.
/// In run mode: fetches ALL tracks from the run via the API (so prior tracks are visible).
/// In live mode: shows tracks from the current frame.
struct TrackListView: View {
    @EnvironmentObject var appState: AppState
    @State private var isExpanded = false
    @State private var runTracks: [RunTrack] = []
    @State private var isFetchingRunTracks = false

    /// Tracks visible in the current frame (live mode or as supplementary info).
    private var frameTracks: [Track] {
        guard let trackSet = appState.currentFrame?.tracks else { return [] }
        return trackSet.tracks.sorted { $0.trackID < $1.trackID }
    }

    /// Track lookup for determining in-view state and colours.
    private var frameTrackByID: [String: Track] {
        Dictionary(uniqueKeysWithValues: frameTracks.map { ($0.trackID, $0) })
    }

    /// Whether we are in run replay mode.
    private var isRunMode: Bool { appState.currentRunID != nil }

    /// Display count for the header badge.
    private var displayCount: Int { isRunMode ? runTracks.count : frameTracks.count }

    /// Track IDs visible in the current frame (for run mode in-view indicator).
    private var inViewTrackIDs: Set<String> { Set(frameTracks.map { $0.trackID }) }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Button(action: {
                isExpanded.toggle()
                if isExpanded && isRunMode && runTracks.isEmpty { fetchRunTracks() }
            }) {
                HStack {
                    Label(
                        "Track List",
                        systemImage: isExpanded ? "list.bullet.circle.fill" : "list.bullet.circle")
                    Spacer()
                    if isFetchingRunTracks {
                        ProgressView().controlSize(.mini)
                    } else {
                        Text("\(displayCount)").font(.caption).foregroundColor(.secondary)
                    }
                    Image(systemName: isExpanded ? "chevron.up" : "chevron.down").font(.caption)
                        .foregroundColor(.secondary)
                }
            }.buttonStyle(.plain)

            if isExpanded {
                if isRunMode {
                    // Run mode: show all tracks from the analysis run
                    runTrackListContent
                } else {
                    // Live mode: show tracks from the current frame
                    frameTrackListContent
                }
            }
        }.onChange(of: appState.currentRunID) { _, newRunID in
            if newRunID != nil && isExpanded { fetchRunTracks() } else { runTracks = [] }
        }
    }

    // MARK: - Run Track List (API-fetched)

    @ViewBuilder private var runTrackListContent: some View {
        if runTracks.isEmpty && !isFetchingRunTracks {
            Text("No tracks in run").font(.caption).foregroundColor(.secondary)
        } else {
            ForEach(runTracks, id: \.trackId) { track in
                let frameTrack = frameTrackByID[track.trackId]
                let isInView = frameTrack != nil
                let statusColour =
                    frameTrack.map { trackStateColour($0.state) } ?? Color.gray.opacity(0.5)
                Button(action: { appState.selectTrack(track.trackId) }) {
                    HStack(spacing: 6) {
                        // Label status indicator
                        Circle().fill(isInView ? statusColour : Color.gray.opacity(0.3)).frame(
                            width: 8, height: 8)
                        VStack(alignment: .leading, spacing: 1) {
                            Text(track.trackId.truncated(12)).font(
                                .system(.caption, design: .monospaced)
                            ).lineLimit(1)
                            HStack(spacing: 4) {
                                if let speed = track.avgSpeedMps {
                                    Text(String(format: "%.1f m/s", speed)).font(.caption2)
                                }
                                if let label = track.userLabel, !label.isEmpty {
                                    Text(label).font(.caption2).foregroundColor(.orange)
                                }
                                if let quality = track.qualityLabel, !quality.isEmpty {
                                    Text(quality).font(.caption2).foregroundColor(.cyan)
                                }
                            }.foregroundColor(.secondary)
                        }
                        Spacer()
                        if track.trackId == appState.selectedTrackID {
                            Image(systemName: "checkmark.circle.fill").foregroundColor(.accentColor)
                                .font(.caption)
                        }
                    }.padding(.vertical, 2).padding(.horizontal, 4).background(
                        track.trackId == appState.selectedTrackID
                            ? Color.accentColor.opacity(0.15) : Color.clear
                    ).cornerRadius(4)
                }.buttonStyle(.plain)
            }
        }
    }

    // MARK: - Frame Track List (live mode)

    @ViewBuilder private var frameTrackListContent: some View {
        if frameTracks.isEmpty {
            Text("No active tracks").font(.caption).foregroundColor(.secondary)
        } else {
            ForEach(frameTracks, id: \.trackID) { track in
                Button(action: { appState.selectTrack(track.trackID) }) {
                    HStack(spacing: 6) {
                        Circle().fill(trackStateColour(track.state)).frame(width: 8, height: 8)
                        VStack(alignment: .leading, spacing: 1) {
                            Text(track.trackID.truncated(12)).font(
                                .system(.caption, design: .monospaced)
                            ).lineLimit(1)
                            HStack(spacing: 4) {
                                Text(String(format: "%.1f m/s", track.speedMps)).font(.caption2)
                                if !track.classLabel.isEmpty {
                                    Text(track.classLabel).font(.caption2).foregroundColor(.orange)
                                }
                            }.foregroundColor(.secondary)
                        }
                        Spacer()
                        if track.trackID == appState.selectedTrackID {
                            Image(systemName: "checkmark.circle.fill").foregroundColor(.accentColor)
                                .font(.caption)
                        }
                    }.padding(.vertical, 2).padding(.horizontal, 4).background(
                        track.trackID == appState.selectedTrackID
                            ? Color.accentColor.opacity(0.15) : Color.clear
                    ).cornerRadius(4)
                }.buttonStyle(.plain)
            }
        }
    }

    // MARK: - Helpers

    private func fetchRunTracks() {
        guard let runID = appState.currentRunID else { return }
        isFetchingRunTracks = true
        Task {
            do {
                let client = RunTrackLabelAPIClient()
                let tracks = try await client.listTracks(runID: runID, limit: 500)
                await MainActor.run {
                    self.runTracks = tracks
                    self.isFetchingRunTracks = false
                }
            } catch { await MainActor.run { self.isFetchingRunTracks = false } }
        }
    }

    private func trackStateColour(_ state: TrackState) -> Color {
        switch state {
        case .unknown: return .gray
        case .tentative: return .yellow
        case .confirmed: return .green
        case .deleted: return .red
        }
    }
}

// MARK: - Label Panel

struct LabelPanelView: View {
    @EnvironmentObject var appState: AppState

    // Canonical detection labels — must match Go validUserLabels and Svelte DetectionLabel
    let userLabels = [
        "good_vehicle", "good_pedestrian", "good_other", "noise", "noise_flora", "split", "merge",
        "missed",
    ]

    // Canonical quality labels — must match Go validQualityLabels and Svelte QualityLabel
    let qualityLabels = ["perfect", "good", "truncated", "noisy_velocity", "stopped_recovered"]

    @State private var lastAssignedLabel: String?
    @State private var lastAssignedQuality: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Label Track").font(.headline)

            if let trackID = appState.selectedTrackID {
                Text("Track: \(trackID.truncated(12))").font(.caption).foregroundColor(.secondary)

                // Run context indicator (Phase 4.3)
                if let runID = appState.currentRunID {
                    Text("Run: \(runID.truncated(12))").font(.caption2).foregroundColor(.orange)
                }

                // Detection labels (user_label)
                Text("Detection").font(.caption).foregroundColor(.secondary).padding(.top, 4)
                ForEach(Array(userLabels.enumerated()), id: \.offset) { index, label in
                    LabelButton(
                        label: label, shortcut: index < 9 ? "\(index + 1)" : nil,
                        isActive: lastAssignedLabel == label
                    ) {
                        appState.assignLabel(label)
                        withAnimation(.easeOut(duration: 0.3)) { lastAssignedLabel = label }
                    }
                }

                // Quality ratings (Phase 4.2 - only in run mode)
                if appState.currentRunID != nil {
                    Divider().padding(.vertical, 4)
                    Text("Quality").font(.caption).foregroundColor(.secondary)
                    ForEach(qualityLabels, id: \.self) { quality in
                        LabelButton(
                            label: quality, shortcut: nil, isActive: lastAssignedQuality == quality
                        ) {
                            appState.assignQuality(quality)
                            withAnimation(.easeOut(duration: 0.3)) { lastAssignedQuality = quality }
                        }
                    }
                }
            } else {
                Text("Select a track to label").font(.caption).foregroundColor(.secondary)
            }
        }.onChange(of: appState.selectedTrackID) { _, _ in
            // Reset feedback when track selection changes
            lastAssignedLabel = nil
            lastAssignedQuality = nil
        }
    }
}

/// A styled label button with hover and selection feedback.
struct LabelButton: View {
    let label: String
    let shortcut: String?
    let isActive: Bool
    let action: () -> Void

    @State private var isHovered = false

    var body: some View {
        Button(action: action) {
            HStack {
                if let shortcut {
                    Text(shortcut).font(.system(.caption, design: .monospaced)).foregroundColor(
                        .secondary
                    ).frame(width: 14, alignment: .trailing)
                }
                Text(displayName(label)).font(.callout)
                Spacer()
                if isActive {
                    Image(systemName: "checkmark.circle.fill").foregroundColor(.green).font(
                        .caption)
                }
            }.padding(.vertical, 3).padding(.horizontal, 6).background(
                isActive
                    ? Color.accentColor.opacity(0.2)
                    : (isHovered ? Color.primary.opacity(0.08) : Color.clear)
            ).cornerRadius(4)
        }.buttonStyle(.plain).onHover { hovering in isHovered = hovering }
    }

    /// Convert snake_case label to a readable display name.
    private func displayName(_ label: String) -> String {
        label.replacingOccurrences(of: "_", with: " ")
    }
}

// MARK: - Debug Overlay Toggles

struct DebugOverlayTogglesView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Debug Overlays").font(.headline)

            Toggle("Track Labels", isOn: $appState.showTrackLabels).font(.caption).toggleStyle(
                .checkbox)

            Toggle("Association Lines", isOn: $appState.showAssociation).font(.caption).toggleStyle(
                .checkbox
            ).disabled(!appState.showDebug)

            Toggle("Gating Ellipses", isOn: $appState.showGating).font(.caption).toggleStyle(
                .checkbox
            ).disabled(!appState.showDebug)

            Toggle("Residual Vectors", isOn: $appState.showResiduals).font(.caption).toggleStyle(
                .checkbox
            ).disabled(!appState.showDebug)

            if !appState.showDebug {
                Text("Enable Debug (D) to show overlays").font(.caption2).foregroundColor(
                    .secondary)
            }
        }
    }
}

// MARK: - Track Label Overlay

/// SwiftUI overlay that renders track ID and class labels above 3D bounding boxes.
/// Positions are projected from world space by MetalRenderer.projectTrackLabels().
struct TrackLabelOverlay: View {
    let labels: [MetalRenderer.TrackScreenLabel]

    var body: some View {
        ZStack {
            ForEach(labels) { label in
                TrackLabelPill(label: label).position(
                    x: CGFloat(label.screenX), y: CGFloat(label.screenY))
            }
        }
    }
}

/// A single track label pill: monospaced track ID prefix + class label.
struct TrackLabelPill: View {
    let label: MetalRenderer.TrackScreenLabel

    var body: some View {
        HStack(spacing: 3) {
            Text(String(label.id.prefix(8))).font(.system(size: 10, design: .monospaced))
                .foregroundColor(.white)

            if !label.classLabel.isEmpty {
                Text(label.classLabel).font(.system(size: 10)).foregroundColor(.yellow)
            }
        }.padding(.horizontal, 5).padding(.vertical, 2).background(
            label.isSelected ? Color.blue.opacity(0.8) : Color.black.opacity(0.6)
        ).cornerRadius(4).fontWeight(label.isSelected ? .bold : .regular)
    }
}

// MARK: - Metal View

struct MetalViewRepresentable: NSViewRepresentable {
    // Only pass stable properties - frame updates will come directly to the renderer
    var showPoints: Bool
    var showBoxes: Bool
    var showClusters: Bool  // M4
    var showTrails: Bool
    var showDebug: Bool  // M6
    var pointSize: Float

    // Closure to register the renderer with AppState
    var onRendererCreated: ((MetalRenderer) -> Void)?
    // M6: Track selection callback
    var onTrackSelected: ((String?) -> Void)?
    // Camera changed callback — reproject labels when the user orbits/pans/zooms
    var onCameraChanged: (() -> Void)?

    func makeNSView(context: Context) -> MTKView {
        let metalView = InteractiveMetalView()
        metalView.preferredFramesPerSecond = 60
        metalView.enableSetNeedsDisplay = false
        metalView.isPaused = false

        // Create renderer
        if let renderer = MetalRenderer(metalView: metalView) {
            context.coordinator.renderer = renderer
            metalView.renderer = renderer
            metalView.onTrackSelected = onTrackSelected
            metalView.onCameraChanged = onCameraChanged
            // Register the renderer so it can receive frame updates directly
            onRendererCreated?(renderer)
        }

        return metalView
    }

    func updateNSView(_ nsView: MTKView, context: Context) {
        guard let renderer = context.coordinator.renderer else { return }

        // Only update overlay settings - frames come directly to renderer
        renderer.showPoints = showPoints
        renderer.showBoxes = showBoxes
        renderer.showClusters = showClusters  // M4
        renderer.showTrails = showTrails
        renderer.showDebug = showDebug  // M6
        renderer.pointSize = pointSize

        // Update track selection callback
        if let metalView = nsView as? InteractiveMetalView {
            metalView.onTrackSelected = onTrackSelected
            metalView.onCameraChanged = onCameraChanged
        }
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    class Coordinator { var renderer: MetalRenderer? }
}

// MARK: - Interactive Metal View

/// Custom MTKView subclass that handles mouse and keyboard input for camera control.
class InteractiveMetalView: MTKView {
    weak var renderer: MetalRenderer?
    var onTrackSelected: ((String?) -> Void)?
    var onCameraChanged: (() -> Void)?
    private var lastMouseLocation = CGPoint.zero
    private var mouseDownLocation = CGPoint.zero  // M6: Track click detection

    override var acceptsFirstResponder: Bool { true }

    override func becomeFirstResponder() -> Bool {
        super.becomeFirstResponder()
        return true
    }

    // MARK: - Mouse Events

    override func mouseDown(with event: NSEvent) {
        lastMouseLocation = event.locationInWindow
        mouseDownLocation = event.locationInWindow
    }

    override func rightMouseDown(with event: NSEvent) { lastMouseLocation = event.locationInWindow }

    override func mouseDragged(with event: NSEvent) {
        let location = event.locationInWindow
        let deltaX = location.x - lastMouseLocation.x
        let deltaY = location.y - lastMouseLocation.y
        lastMouseLocation = location

        let shiftHeld = event.modifierFlags.contains(.shift)
        renderer?.handleMouseDrag(
            deltaX: deltaX, deltaY: deltaY, isRightButton: false, shiftHeld: shiftHeld)
        onCameraChanged?()
    }

    override func mouseUp(with event: NSEvent) {
        let location = event.locationInWindow
        let dx = location.x - mouseDownLocation.x
        let dy = location.y - mouseDownLocation.y
        let dragDistance = sqrt(dx * dx + dy * dy)

        // Only treat as a click if the mouse didn't move much (< 5 pixels)
        if dragDistance < 5.0 {
            // Convert to view coordinates
            let viewPoint = convert(location, from: nil)
            let trackID = renderer?.hitTestTrack(at: viewPoint, viewSize: bounds.size)
            onTrackSelected?(trackID)
        }
    }

    override func rightMouseDragged(with event: NSEvent) {
        let location = event.locationInWindow
        let deltaX = location.x - lastMouseLocation.x
        let deltaY = location.y - lastMouseLocation.y
        lastMouseLocation = location

        renderer?.handleMouseDrag(
            deltaX: deltaX, deltaY: deltaY, isRightButton: true, shiftHeld: false)
        onCameraChanged?()
    }

    override func scrollWheel(with event: NSEvent) {
        // Trackpad: use scrollingDeltaY. Mouse wheel: use deltaY
        let delta = event.hasPreciseScrollingDeltas ? event.scrollingDeltaY / 10 : event.deltaY
        renderer?.handleZoom(delta: delta)
        onCameraChanged?()
    }

    override func magnify(with event: NSEvent) {
        // Pinch gesture on trackpad
        renderer?.handleZoom(delta: event.magnification * 10)
        onCameraChanged?()
    }

    // MARK: - Keyboard Events

    override func keyDown(with event: NSEvent) {
        if let renderer = renderer,
            renderer.handleKeyDown(keyCode: event.keyCode, modifiers: event.modifierFlags)
        {
            // Key was handled (camera movement)
            onCameraChanged?()
            return
        }
        super.keyDown(with: event)
    }
}

// MARK: - Preview

#Preview { ContentView().environmentObject(AppState()) }
