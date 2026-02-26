// ContentView.swift
// Main content view for the Velocity Visualiser.
//
// This view composes the Metal render view with SwiftUI controls.

import MetalKit
import SwiftUI
import os

private let uiLogger = DevLogger(category: "UI")

struct ContentView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        VStack(spacing: 0) {
            // Toolbar always at the top, spanning full width
            ToolbarView()

            // Main content below toolbar
            HSplitView {
                // Main 3D view
                VStack(spacing: 0) {
                    // Filter bar above the Metal view, not above inspector
                    if appState.showFilterPane { FilterBarView() }
                    // Metal view - frames are delivered directly to renderer via AppState
                    ZStack {
                        MetalViewRepresentable(
                            showPoints: appState.showPoints,
                            showBackground: appState.showBackground, showBoxes: appState.showBoxes,
                            showClusters: appState.showClusters,
                            showVelocity: appState.showVelocity, showTrails: appState.showTrails,
                            showDebug: appState.showDebug, showGrid: appState.showGrid,
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
                            Color.clear.onAppear {
                                let size = geometry.size
                                Task { @MainActor in
                                    appState.updateMetalViewSize(
                                        size, source: "ContentView.onAppear")
                                }
                            }.onChange(of: geometry.size) { _, newSize in
                                // Defer to next run loop to avoid AttributeGraph cycle
                                Task { @MainActor in
                                    appState.updateMetalViewSize(
                                        newSize, source: "ContentView.GeometryReader.onChange")
                                }
                            }
                        }.allowsHitTesting(false)
                    }.frame(minWidth: 400, minHeight: 300)
                }.frame(minWidth: 600)

                // Side panel
                if appState.showSidePanel || appState.selectedTrackID != nil {
                    SidePanelView().frame(width: 520)
                }

            }

            // Playback controls span full width below the split view
            PlaybackControlsView()
        }.frame(minWidth: 800, minHeight: 600)  // Keyboard shortcuts for playback
            .onKeyPress(.space) { handleKeyPress(.space, appState: appState) }.onKeyPress(",") {
                handleKeyPress(.comma, appState: appState)
            }.onKeyPress(".") { handleKeyPress(.period, appState: appState) }.onKeyPress("[") {
                handleKeyPress(.decreaseRate, appState: appState)
            }.onKeyPress("]") { handleKeyPress(.increaseRate, appState: appState) }
            // Label shortcuts: keys 1-9 assign detection labels
            .onKeyPress("1") { handleKeyPress(.label1, appState: appState) }.onKeyPress("2") {
                handleKeyPress(.label2, appState: appState)
            }.onKeyPress("3") { handleKeyPress(.label3, appState: appState) }.onKeyPress("4") {
                handleKeyPress(.label4, appState: appState)
            }.onKeyPress("5") { handleKeyPress(.label5, appState: appState) }.onKeyPress("6") {
                handleKeyPress(.label6, appState: appState)
            }.onKeyPress("7") { handleKeyPress(.label7, appState: appState) }.onKeyPress("8") {
                handleKeyPress(.label8, appState: appState)
            }.onKeyPress("9") { handleKeyPress(.label9, appState: appState) }
            // Track navigation: up/down arrows traverse the track list
            .onKeyPress(.upArrow) { handleKeyPress(.selectPrevTrack, appState: appState) }
            .onKeyPress(.downArrow) { handleKeyPress(.selectNextTrack, appState: appState) }
            // Overlay toggle hotkeys
            .onKeyPress("f") { handleKeyPress(.togglePoints, appState: appState) }.onKeyPress("k") {
                handleKeyPress(.toggleBackground, appState: appState)
            }.onKeyPress("b") { handleKeyPress(.toggleBoxes, appState: appState) }.onKeyPress("c") {
                handleKeyPress(.toggleClusters, appState: appState)
            }.onKeyPress("t") { handleKeyPress(.toggleTrails, appState: appState) }.onKeyPress("v")
        { handleKeyPress(.toggleVelocity, appState: appState) }.onKeyPress("l") {
            handleKeyPress(.toggleLabels, appState: appState)
        }.onKeyPress("g") { handleKeyPress(.toggleGrid, appState: appState) }  // Run browser sheet
            .sheet(isPresented: $appState.showRunBrowser) {
                RunBrowserView().environmentObject(appState)
            }
    }
}

// MARK: - Keyboard Shortcut Handling

/// All keyboard actions that can be triggered by hotkeys.
enum KeyAction {
    case space, comma, period, decreaseRate, increaseRate
    case label1, label2, label3, label4, label5, label6, label7, label8, label9
    case selectPrevTrack, selectNextTrack
    case togglePoints, toggleBackground, toggleBoxes, toggleClusters
    case toggleTrails, toggleVelocity, toggleLabels, toggleGrid
}

/// Handle a keyboard action, returning the SwiftUI key-press result.
/// Extracted from ContentView closures for testability.
@MainActor func handleKeyPress(_ action: KeyAction, appState: AppState) -> KeyPress.Result {
    uiLogger.debug("handleKeyPress(\(String(describing: action)))")
    switch action {
    case .space:
        uiLogger.debug("Key: SPACE → togglePlayPause()")
        appState.togglePlayPause()
        return .handled
    case .comma:
        guard appState.isSeekable else {
            uiLogger.debug("Key: COMMA → ignored (not seekable)")
            return .ignored
        }
        uiLogger.debug("Key: COMMA → stepBackward()")
        appState.stepBackward()
        return .handled
    case .period:
        guard appState.isSeekable else {
            uiLogger.debug("Key: PERIOD → ignored (not seekable)")
            return .ignored
        }
        uiLogger.debug("Key: PERIOD → stepForward()")
        appState.stepForward()
        return .handled
    case .decreaseRate:
        uiLogger.debug("Key: [ → decreaseRate()")
        appState.decreaseRate()
        return .handled
    case .increaseRate:
        uiLogger.debug("Key: ] → increaseRate()")
        appState.increaseRate()
        return .handled
    case .label1: return assignLabelByIndex(0, appState: appState)
    case .label2: return assignLabelByIndex(1, appState: appState)
    case .label3: return assignLabelByIndex(2, appState: appState)
    case .label4: return assignLabelByIndex(3, appState: appState)
    case .label5: return assignLabelByIndex(4, appState: appState)
    case .label6: return assignLabelByIndex(5, appState: appState)
    case .label7: return assignLabelByIndex(6, appState: appState)
    case .label8: return assignLabelByIndex(7, appState: appState)
    case .label9: return assignLabelByIndex(8, appState: appState)
    case .selectPrevTrack:
        appState.selectPreviousTrack()
        return .handled
    case .selectNextTrack:
        appState.selectNextTrack()
        return .handled
    case .togglePoints:
        appState.showPoints.toggle()
        return .handled
    case .toggleBackground:
        appState.showBackground.toggle()
        return .handled
    case .toggleBoxes:
        appState.showBoxes.toggle()
        return .handled
    case .toggleClusters:
        appState.showClusters.toggle()
        return .handled
    case .toggleTrails:
        appState.showTrails.toggle()
        return .handled
    case .toggleVelocity:
        appState.showVelocity.toggle()
        return .handled
    case .toggleLabels:
        appState.showTrackLabels.toggle()
        return .handled
    case .toggleGrid:
        appState.showGrid.toggle()
        return .handled
    }
}

/// Assign a classification label by index. Returns .ignored if no track selected
/// or the label index is out of range.
@MainActor private func assignLabelByIndex(_ index: Int, appState: AppState) -> KeyPress.Result {
    guard appState.selectedTrackID != nil else { return .ignored }
    let labels = LabelPanelView.classificationLabels
    guard labels.count > index else { return .ignored }
    appState.assignLabel(labels[index].name)
    return .handled
}

// MARK: - Track List Helpers (extracted for testability)

/// Typealias for rank-tracking entries used by climb detection.
typealias RankEntry = (rank: Int, climbedAt: Date?)

/// Check whether a track has climbed at least 1 rank within the past `window` seconds.
/// Extracted from TrackListView for testability.
func isTrackClimbing(
    _ trackID: String, ranks: [String: RankEntry], now: Date = Date(), window: TimeInterval = 2.0
) -> Bool {
    guard let entry = ranks[trackID], let climbedAt = entry.climbedAt else { return false }
    return now.timeIntervalSince(climbedAt) < window
}

/// Compute updated ranks from a list of (id, peakSpeed) pairs sorted descending.
/// Compares against `previousRanks` to detect climbs.
/// Extracted from TrackListView.updateRanks() for testability.
func computeRanks(
    speedSorted: [(id: String, peak: Float)], previousRanks: [String: RankEntry], now: Date = Date()
) -> [String: RankEntry] {
    var newRanks: [String: RankEntry] = [:]
    for (index, entry) in speedSorted.enumerated() {
        if let old = previousRanks[entry.id] {
            if index < old.rank {
                newRanks[entry.id] = (rank: index, climbedAt: now)
            } else {
                newRanks[entry.id] = (rank: index, climbedAt: old.climbedAt)
            }
        } else {
            newRanks[entry.id] = (rank: index, climbedAt: nil)
        }
    }
    return newRanks
}

/// Build speed-sorted entries for run-mode tracks.
/// Extracted from TrackListView.updateRanks() for testability.
func buildRunModeSpeedEntries(
    runTracks: [RunTrack], frameTrackByID: [String: Track], trackPeakSpeed: [String: Float]
) -> [(id: String, peak: Float)] {
    runTracks.map {
        let live = frameTrackByID[$0.trackId].map { Float($0.peakSpeedMps) }
        let persistent = trackPeakSpeed[$0.trackId]
        let api = Float($0.peakSpeedMps ?? 0)
        return (id: $0.trackId, peak: [live, persistent, api].compactMap { $0 }.max() ?? 0)
    }.sorted { $0.peak > $1.peak }
}

/// Build speed-sorted entries for frame-mode tracks.
/// Extracted from TrackListView.updateRanks() for testability.
func buildFrameModeSpeedEntries(
    tracks: [Track], trackPeakSpeed: [String: Float]
) -> [(id: String, peak: Float)] {
    tracks.map {
        let persistent = trackPeakSpeed[$0.trackID] ?? 0
        return (id: $0.trackID, peak: max($0.peakSpeedMps, persistent))
    }.sorted { $0.peak > $1.peak }
}

/// Compute updated ranks for the track list.
/// Combines run-mode and frame-mode logic into a single call.
/// Extracted from TrackListView.updateRanks() for testability.
@MainActor func computeUpdatedRanks(
    isRunMode: Bool, runTracks: [RunTrack], frameTrackByID: [String: Track], appState: AppState,
    previousRanks: [String: RankEntry], now: Date = Date()
) -> [String: RankEntry] {
    let speedSorted: [(id: String, peak: Float)]
    if isRunMode {
        speedSorted = buildRunModeSpeedEntries(
            runTracks: runTracks, frameTrackByID: frameTrackByID,
            trackPeakSpeed: appState.trackPeakSpeed)
    } else {
        let tracks =
            appState.hasActiveFilters
            ? appState.filteredTracks : (appState.currentFrame?.tracks?.tracks ?? [])
        speedSorted = buildFrameModeSpeedEntries(
            tracks: tracks, trackPeakSpeed: appState.trackPeakSpeed)
    }
    return computeRanks(speedSorted: speedSorted, previousRanks: previousRanks, now: now)
}

/// Collect tags for a run-mode track (classification + quality flags).
/// Extracted from TrackListView for testability.
func runTrackTags(
    _ track: RunTrack, userLabels: [String: String], userQualityFlags: [String: String]
) -> [(String, Color)] {
    var tags: [(String, Color)] = []
    let displayLabel = userLabels[track.trackId] ?? track.userLabel
    if let label = displayLabel, !label.isEmpty { tags.append((label, .confirmedGreen)) }
    let quality = userQualityFlags[track.trackId] ?? track.qualityLabel
    if let quality = quality, !quality.isEmpty {
        for flag in quality.split(separator: ",") {
            let trimmedFlag = flag.trimmingCharacters(in: .whitespacesAndNewlines)
            if !trimmedFlag.isEmpty { tags.append((trimmedFlag, .accentColor)) }
        }
    }
    return tags
}

/// Collect tags for a live-mode track (classification label).
/// Extracted from TrackListView for testability.
func frameTrackTags(_ track: Track, userLabels: [String: String]) -> [(String, Color)] {
    var tags: [(String, Color)] = []
    let displayLabel = userLabels[track.trackID] ?? track.classLabel
    if !displayLabel.isEmpty { tags.append((displayLabel, .confirmedGreen)) }
    return tags
}

/// Compute the best (maximum) speed for a run-mode track from live, persistent, and API sources.
/// Extracted from TrackListView row rendering for testability.
func bestRunTrackSpeed(
    trackId: String, apiPeakSpeed: Double?, frameTrack: Track?, trackPeakSpeed: [String: Float]
) -> Double? {
    let liveSpeed = frameTrack.map { Double($0.peakSpeedMps) }
    let persistentPeak = trackPeakSpeed[trackId].map { Double($0) }
    return [liveSpeed, persistentPeak, apiPeakSpeed].compactMap { $0 }.max()
}

/// Sort tracks by peak speed descending, using both live and persistent peak data.
/// Extracted from TrackListView.frameTracks for testability.
func sortTracksByPeakSpeed(_ tracks: [Track], trackPeakSpeed: [String: Float]) -> [Track] {
    tracks.sorted {
        max($0.peakSpeedMps, trackPeakSpeed[$0.trackID] ?? 0)
            > max($1.peakSpeedMps, trackPeakSpeed[$1.trackID] ?? 0)
    }
}

/// Sort run tracks by peak speed descending, using live, persistent, and API peak data.
/// Extracted from TrackListView.sortedRunTracks for testability.
func sortRunTracksByPeakSpeed(
    _ runTracks: [RunTrack], frameTrackByID: [String: Track], trackPeakSpeed: [String: Float]
) -> [RunTrack] {
    runTracks.sorted { a, b in
        let peakA =
            [
                frameTrackByID[a.trackId].map { Double($0.peakSpeedMps) },
                trackPeakSpeed[a.trackId].map { Double($0) }, a.peakSpeedMps,
            ].compactMap { $0 }.max() ?? 0
        let peakB =
            [
                frameTrackByID[b.trackId].map { Double($0.peakSpeedMps) },
                trackPeakSpeed[b.trackId].map { Double($0) }, b.peakSpeedMps,
            ].compactMap { $0 }.max() ?? 0
        return peakA > peakB
    }
}

/// Parse a comma-separated quality label string into a set of flag names.
/// Extracted from LabelPanelView for testability.
func parseQualityFlags(_ quality: String) -> Set<String> {
    let trimmedFlags = quality.split(separator: ",").map {
        String($0).trimmingCharacters(in: .whitespaces)
    }.filter { !$0.isEmpty }
    return Set(trimmedFlags)
}

/// Toggle a flag in a set: remove if present, insert if absent.
/// Returns the updated set. Extracted from LabelPanelView for testability.
func toggleFlag(_ flag: String, in flags: Set<String>) -> Set<String> {
    var updated = flags
    if updated.contains(flag) { updated.remove(flag) } else { updated.insert(flag) }
    return updated
}

/// Serialise a set of quality flags to a sorted comma-separated string.
/// Extracted from LabelPanelView for testability.
func serialiseFlags(_ flags: Set<String>) -> String { flags.sorted().joined(separator: ",") }

/// Sync labels fetched from the run-track API into the AppState and local view state.
/// Extracted from LabelPanelView .onChange closure for testability.
/// Returns (updatedFlags, isCarriedOver).
@MainActor func applyFetchedTrackLabels(
    track: RunTrack, trackID: String, appState: AppState
) -> (flags: Set<String>, isCarriedOver: Bool) {
    var flags: Set<String> = []
    var carried = false
    if let label = track.userLabel, !label.isEmpty { appState.userLabels[trackID] = label }
    if let quality = track.qualityLabel, !quality.isEmpty {
        flags = parseQualityFlags(quality)
        appState.userQualityFlags[trackID] = serialiseFlags(flags)
    }
    if track.labelerId == "hint-carryover" { carried = true }
    return (flags, carried)
}

/// Sync a batch of run tracks into AppState labels/flags — used by TrackListView.fetchRunTracks().
/// Extracted for testability. Returns the number of synced labels.
@MainActor @discardableResult func syncRunTracksToAppState(
    _ tracks: [RunTrack], appState: AppState
) -> Int {
    var count = 0
    for track in tracks {
        if let label = track.userLabel, !label.isEmpty {
            if appState.userLabels[track.trackId] == nil {
                appState.userLabels[track.trackId] = label
                count += 1
            }
        }
        if let quality = track.qualityLabel, !quality.isEmpty {
            if appState.userQualityFlags[track.trackId] == nil {
                appState.userQualityFlags[track.trackId] = quality
            }
        }
    }
    return count
}

// MARK: - Standalone Track Row Views (extracted for testability)

/// A single row in the run-mode track list. Extracted from TrackListView for
/// standalone testability — all parameters are explicit, no @State dependency.
struct RunTrackRowView: View {
    let trackId: String
    let shortID: String
    let statusColour: Color
    let isInView: Bool
    let bestSpeed: Double?
    let showClimbArrow: Bool
    let tags: [(String, Color)]
    let isSelected: Bool
    let onSelect: () -> Void

    var body: some View {
        Button(action: onSelect) {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 4) {
                    Circle().fill(isInView ? statusColour : Color.gray.opacity(0.3)).frame(
                        width: 6, height: 6)
                    Text(shortID).font(.system(.caption, design: .monospaced)).foregroundColor(
                        .white
                    ).fixedSize()
                    if let speed = bestSpeed {
                        Text(String(format: "%.1f m/s", speed)).font(.caption2).fixedSize()
                    }
                    if showClimbArrow {
                        Text("▲").font(.system(size: 8, weight: .bold)).foregroundColor(.green)
                            .fixedSize()
                    }
                    Spacer()
                }.foregroundColor(.secondary).lineLimit(1)
                if !tags.isEmpty { TagRow(tags: tags) }
            }.padding(.vertical, 4).padding(.horizontal, 4).background(
                isSelected ? Color.accentColor.opacity(0.15) : Color.clear
            ).cornerRadius(4).contentShape(Rectangle())
        }.buttonStyle(.plain)
    }
}

/// A single row in the frame-mode (live) track list. Extracted from TrackListView for
/// standalone testability — all parameters are explicit, no @State dependency.
struct FrameTrackRowView: View {
    let trackId: String
    let shortID: String
    let statusColour: Color
    let speedDisplay: String
    let showClimbArrow: Bool
    let tags: [(String, Color)]
    let isSelected: Bool
    let onSelect: () -> Void

    var body: some View {
        Button(action: onSelect) {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 4) {
                    Circle().fill(statusColour).frame(width: 6, height: 6)
                    Text(shortID).font(.system(.caption, design: .monospaced)).foregroundColor(
                        .white
                    ).fixedSize()
                    Text(speedDisplay).font(.caption2).fixedSize()
                    if showClimbArrow {
                        Text("▲").font(.system(size: 8, weight: .bold)).foregroundColor(.green)
                            .fixedSize()
                    }
                    Spacer()
                }.foregroundColor(.secondary).lineLimit(1)
                if !tags.isEmpty { TagRow(tags: tags) }
            }.padding(.vertical, 4).padding(.horizontal, 4).background(
                isSelected ? Color.accentColor.opacity(0.15) : Color.clear
            ).cornerRadius(4).contentShape(Rectangle())
        }.buttonStyle(.plain)
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

            // Run browser button
            if appState.isConnected {
                Divider().frame(height: 20)
                Button(action: { appState.showRunBrowser = true }) {
                    Label("Runs", systemImage: "doc.text.magnifyingglass")
                }.help("Browse analysis runs")

                Divider().frame(height: 20)
                Button(action: { appState.showSidePanel.toggle() }) {
                    Label("Inspector", systemImage: "sidebar.trailing")
                }.help("Toggle track inspector")

                Divider().frame(height: 20)
                Button(action: { appState.showFilterPane.toggle() }) {
                    Label("Filters", systemImage: "line.3.horizontal.decrease.circle")
                }.help("Toggle track filter pane").foregroundColor(
                    appState.hasActiveFilters ? .orange : nil)

                Divider().frame(height: 20)
                Button(action: { appState.clearAll() }) {
                    Label("Clear", systemImage: "xmark.circle")
                }.help("Clear all except background grid")
            }

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

        Circle().fill(isConnected ? .green : (hasError ? .red : .gray)).frame(width: 8, height: 8)
            .help(errorMessage ?? (isConnected ? appState.serverAddress : "Disconnected"))
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
            ToggleButton(label: "F", isOn: $appState.showPoints, help: "Foreground Points")
            ToggleButton(label: "K", isOn: $appState.showBackground, help: "Background Points")
            ToggleButton(label: "B", isOn: $appState.showBoxes, help: "Boxes")
            ToggleButton(label: "C", isOn: $appState.showClusters, help: "Clusters")
            ToggleButton(label: "T", isOn: $appState.showTrails, help: "Trails")
            ToggleButton(label: "V", isOn: $appState.showVelocity, help: "Velocity")
            ToggleButton(label: "L", isOn: $appState.showTrackLabels, help: "Track Labels")
            ToggleButton(label: "G", isOn: $appState.showGrid, help: "Ground Grid")

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
func formatRate(_ rate: Float) -> String {
    if rate == Float(Int(rate)) {
        return String(Int(rate))
    } else {
        return String(format: "%.1f", rate)
    }
}

/// Format nanosecond duration as MM:SS or HH:MM:SS.
func formatDuration(_ nanos: Int64) -> String {
    let totalSeconds = abs(nanos) / 1_000_000_000
    let hours = totalSeconds / 3600
    let minutes = (totalSeconds % 3600) / 60
    let seconds = totalSeconds % 60
    let prefix = nanos < 0 ? "-" : ""
    if hours > 0 { return String(format: "%@%d:%02d:%02d", prefix, hours, minutes, seconds) }
    return String(format: "%@%d:%02d", prefix, minutes, seconds)
}

@available(macOS 15.0, *) struct PlaybackControlsDerivedState: Equatable {
    let mode: AppState.PlaybackMode
    let isConnected: Bool
    let isPaused: Bool
    let replayFinished: Bool
    let playbackRate: Float
    let showStepButtons: Bool
    let stepBackwardDisabled: Bool
    let stepForwardDisabled: Bool
    let showReplayTimeline: Bool
    let showSeekableSlider: Bool
    let showReadOnlyProgress: Bool
    let showReplayMetadataUnavailable: Bool
    let seekSliderDisabled: Bool
    let playPauseDisabled: Bool
    let rateControlsDisabled: Bool
    let modeLabel: String

    init(
        isConnected: Bool, mode: AppState.PlaybackMode, isPaused: Bool,
        replayFinished: Bool = false, playbackRate: Float, busy: Bool, hasValidTimelineRange: Bool,
        hasFrameIndexProgress: Bool, currentFrameIndex: UInt64, totalFrames: UInt64
    ) {
        self.isConnected = isConnected
        self.mode = mode
        // When replay has finished, always treat as paused (show play icon)
        self.isPaused = isPaused || replayFinished
        self.replayFinished = replayFinished
        self.playbackRate = playbackRate
        self.modeLabel = mode.modeLabel
        let isReplay = mode == .replayNonSeekable || mode == .replaySeekable
        let isSeekableReplay = mode == .replaySeekable
        let isLiveOrUnknown = mode == .live || mode == .unknown

        showStepButtons = isSeekableReplay
        stepBackwardDisabled = !isConnected || busy || currentFrameIndex == 0
        stepForwardDisabled =
            !isConnected || busy || totalFrames == 0 || currentFrameIndex + 1 >= totalFrames
        showReplayTimeline = isReplay || replayFinished
        showSeekableSlider = isSeekableReplay || replayFinished
        showReadOnlyProgress =
            mode == .replayNonSeekable && (hasValidTimelineRange || hasFrameIndexProgress)
        showReplayMetadataUnavailable =
            mode == .replayNonSeekable && !hasValidTimelineRange && !hasFrameIndexProgress
        seekSliderDisabled = !isConnected || busy || !hasValidTimelineRange
        // When replay finished, always allow the play button (to restart)
        playPauseDisabled = replayFinished ? false : (!isConnected || busy || isLiveOrUnknown)
        rateControlsDisabled = !isConnected || busy || isLiveOrUnknown
    }
}

struct PlaybackControlsView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        let ui = PlaybackControlsDerivedState(
            isConnected: appState.isConnected, mode: appState.displayPlaybackMode,
            isPaused: appState.isPaused, replayFinished: appState.replayFinished,
            playbackRate: appState.playbackRate, busy: appState.playbackControlsBusy,
            hasValidTimelineRange: appState.hasValidTimelineRange,
            hasFrameIndexProgress: appState.hasFrameIndexProgress,
            currentFrameIndex: appState.currentFrameIndex, totalFrames: appState.totalFrames)

        HStack {
            // Play/Pause (disabled in live mode)
            Button(action: {
                uiLogger.debug("UI: Play/Pause button clicked")
                appState.togglePlayPause()
            }) { Image(systemName: ui.isPaused ? "play.fill" : "pause.fill") }.disabled(
                ui.playPauseDisabled)

            // Step buttons (only for seekable modes like .vrlog replay)
            if ui.showStepButtons {
                Button(action: {
                    uiLogger.debug("UI: Step backward button clicked")
                    appState.stepBackward()
                }) { Image(systemName: "backward.frame.fill") }.disabled(ui.stepBackwardDisabled)

                Button(action: {
                    uiLogger.debug("UI: Step forward button clicked")
                    appState.stepForward()
                }) { Image(systemName: "forward.frame.fill") }.disabled(ui.stepForwardDisabled)
            }

            // Timeline (replay mode)
            if ui.showReplayTimeline {
                if ui.showSeekableSlider {
                    // Interactive seek slider for .vrlog replay
                    Slider(value: $appState.replayProgress, in: 0...1) { editing in
                        if editing {
                            uiLogger.debug("UI: Slider drag started")
                            appState.setSliderEditing(true)
                        } else {
                            uiLogger.debug(
                                "UI: Slider drag ended — seeking to \(appState.replayProgress)")
                            // Capture target before allowing frame updates to overwrite progress.
                            // seek() sets isSeekingInProgress = true and clears it on completion,
                            // so we don't call setSliderEditing(false) here to avoid a race.
                            appState.seek(to: appState.replayProgress)
                        }
                    }.frame(minWidth: 200).disabled(ui.seekSliderDisabled)
                } else if ui.showReadOnlyProgress {
                    // Read-only progress bar for PCAP replay
                    Slider(value: .constant(appState.displayReplayProgress), in: 0...1).frame(
                        minWidth: 200
                    ).disabled(true)
                } else {
                    Text("Replay metadata unavailable").font(.caption).foregroundColor(.secondary)
                        .frame(minWidth: 200, alignment: .leading)
                }

                // Time display
                TimeDisplayView()
            } else {
                Spacer()
            }

            // Rate control (disabled in live mode)
            HStack(spacing: 4) {
                Button(action: { appState.decreaseRate() }) {
                    Image(systemName: "minus").frame(width: 22, height: 22).contentShape(
                        Rectangle())
                }.buttonStyle(.borderless).disabled(ui.rateControlsDisabled)

                // Rate display: number + clickable "x" to reset to 1x
                HStack(spacing: 0) {
                    Text(formatRate(ui.playbackRate)).font(.caption).monospacedDigit()
                    Button(action: { appState.resetRate() }) {
                        Text("x").font(.caption).frame(width: 16, height: 22).contentShape(
                            Rectangle())
                    }.buttonStyle(.borderless).disabled(ui.rateControlsDisabled)
                }.frame(width: 45).foregroundColor(ui.rateControlsDisabled ? .secondary : .primary)

                Button(action: { appState.increaseRate() }) {
                    Image(systemName: "plus").frame(width: 22, height: 22).contentShape(Rectangle())
                }.buttonStyle(.borderless).disabled(ui.rateControlsDisabled)
            }.opacity(ui.rateControlsDisabled ? 0.5 : 1.0)

            // Mode indicator (only show when connected)
            PlaybackModeBadgeView(
                modeLabel: ui.modeLabel, mode: ui.mode, isConnected: ui.isConnected)
        }.padding(.horizontal).padding(.vertical, 8).background(
            Color(nsColor: .controlBackgroundColor))
    }
}

/// Displays elapsed, remaining, or frame-index time. Click to cycle mode.
/// The display mode is stored on AppState and can also be changed from the Playback menu.
struct TimeDisplayView: View {
    @EnvironmentObject var appState: AppState

    /// Whether we have valid log boundaries to compute durations.
    private var hasValidRange: Bool { appState.logEndTimestamp > appState.logStartTimestamp }

    private var elapsed: Int64 { max(0, appState.currentTimestamp - appState.logStartTimestamp) }

    private var total: Int64 { appState.logEndTimestamp - appState.logStartTimestamp }

    private var remaining: Int64 { max(0, appState.logEndTimestamp - appState.currentTimestamp) }

    /// Text for the current display mode, with automatic fallback.
    private var displayText: String {
        let mode = appState.timeDisplayMode
        if mode == .frames || !hasValidRange {
            // Frames mode, or forced fallback when timestamps unavailable
            if appState.totalFrames > 0 {
                return "F\(appState.currentFrameIndex + 1)/\(appState.totalFrames)"
            }
            return "--:-- / --:--"
        }
        let totalText = formatDuration(total)
        switch mode {
        case .elapsed: return "\(formatDuration(elapsed)) / \(totalText)"
        case .remaining: return "\(formatDuration(-remaining)) / \(totalText)"
        case .frames: return ""  // Handled above
        }
    }

    /// Tooltip describing the current display mode.
    private var tooltip: String {
        switch appState.timeDisplayMode {
        case .elapsed: return "Showing elapsed time (click to cycle)"
        case .remaining: return "Showing remaining time (click to cycle)"
        case .frames: return "Showing frame index (click to cycle)"
        }
    }

    var body: some View {
        Button(action: { appState.cycleTimeDisplayMode() }) {
            Text(displayText).font(.system(.caption, design: .monospaced)).foregroundColor(
                .secondary)
        }.buttonStyle(.plain).fixedSize().help(tooltip)
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

@available(macOS 15.0, *) struct PlaybackModeBadgeView: View {
    let modeLabel: String
    let mode: AppState.PlaybackMode
    let isConnected: Bool

    private var foreground: Color {
        switch mode {
        case .live: return .red
        case .replayNonSeekable: return .orange
        case .replaySeekable: return .blue
        case .unknown: return .secondary
        }
    }

    var body: some View {
        if isConnected {
            Text(modeLabel).font(.caption).fontWeight(.bold).foregroundColor(foreground).padding(
                .horizontal, 8
            ).padding(.vertical, 2).background(foreground.opacity(0.16)).cornerRadius(4)
        }
    }
}

// MARK: - Side Panel

struct SidePanelView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        HStack(alignment: .top, spacing: 0) {
            // Left column: inspector + labels (scrolls independently)
            ScrollView(.vertical, showsIndicators: false) {
                VStack(alignment: .leading, spacing: 16) {
                    // Track header + labels first
                    if let trackID = appState.selectedTrackID {
                        TrackInspectorHeaderView(trackID: trackID)
                    }

                    // Label panel
                    LabelPanelView()

                    // History chart + detail cards (only when a track is selected)
                    if let trackID = appState.selectedTrackID {
                        Divider()
                        TrackHistoryGraphView(trackID: trackID)
                    }

                    // Detail cards (position, velocity, dimensions, state)
                    if let trackID = appState.selectedTrackID {
                        TrackInspectorDetailCards(trackID: trackID)
                    }

                    Spacer()
                }.padding()
            }.scrollIndicators(.never).frame(maxWidth: .infinity, alignment: .leading)

            Divider()

            // Right column: track list (scrolls independently)
            ScrollView(.vertical, showsIndicators: false) {
                VStack(alignment: .leading, spacing: 16) {
                    TrackListView()
                    Spacer()
                }.padding()
            }.scrollIndicators(.never).frame(width: 136, alignment: .leading)
        }.background(Color(nsColor: .controlBackgroundColor))
    }
}

// MARK: - Track Inspector

/// Header for the track inspector: title, close button, ID, run ID, and "not in frame" message.
struct TrackInspectorHeaderView: View {
    let trackID: String
    @EnvironmentObject var appState: AppState

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

            if let runID = appState.currentRunID {
                Text("Run: \(runID)").font(.caption).foregroundColor(.secondary)
            }

        }
    }
}

/// Detail cards for the track inspector: position, velocity, dimensions, state.
struct TrackInspectorDetailCards: View {
    let trackID: String
    @EnvironmentObject var appState: AppState

    private var track: Track? {
        appState.currentFrame?.tracks?.tracks.first(where: { $0.trackID == trackID })
    }

    var body: some View {
        if let t = track {
            VStack(alignment: .leading, spacing: 8) {
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
                    }
                }

                // Dimensions
                GroupBox(label: Text("Dimensions").font(.caption2)) {
                    VStack(alignment: .leading, spacing: 2) {
                        DetailRow(
                            label: "L×W×H",
                            value: String(
                                format: "%.1f × %.1f × %.1f m", t.bboxLength, t.bboxWidth,
                                t.bboxHeight))
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
                            Text(trackStateLabel(t.state)).font(.caption).fontWeight(.medium)
                                .foregroundColor(trackStateColour(t.state))
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
                        DetailRow(
                            label: "Class",
                            value: t.classLabel.isEmpty ? "Not classified" : t.classLabel)
                    }
                }
            }
        } else {
            Text("Track not in current frame").font(.caption).foregroundColor(.secondary)
        }
    }

}

/// Human-readable label for a track state. Extracted for testability.
func trackStateLabel(_ state: TrackState) -> String {
    switch state {
    case .unknown: return "Unknown"
    case .tentative: return "Tentative"
    case .confirmed: return "Confirmed"
    case .deleted: return "Deleted"
    }
}

/// Colour for a track state dot/badge. Extracted for testability.
func trackStateColour(_ state: TrackState) -> Color {
    switch state {
    case .unknown: return .gray
    case .tentative: return .yellow
    case .confirmed: return .green
    case .deleted: return .red
    }
}

/// Composite view for backward compatibility — shows header, detail cards, and history.
struct TrackInspectorView: View {
    let trackID: String
    @EnvironmentObject var appState: AppState

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            TrackInspectorHeaderView(trackID: trackID)
            TrackInspectorDetailCards(trackID: trackID)
            TrackHistoryGraphView(trackID: trackID)
        }
    }
}

// MARK: - Track History Graph

/// Inline sparkline graph showing peak speed, velocity, and heading over recent frames.
struct TrackHistoryGraphView: View {
    let trackID: String
    @EnvironmentObject var appState: AppState

    private var samples: [AppState.TrackSample] { appState.trackHistory[trackID] ?? [] }

    /// Trim trailing near-zero-speed samples (lead-out) from the chart.
    /// Keeps the full history for tracks that have always been slow/static
    /// (e.g. objects waiting at an intersection).
    private var trimmedSamples: [AppState.TrackSample] {
        let raw = samples
        guard raw.count >= 2 else { return raw }
        let threshold: Float = 0.3  // m/s — below this is considered stationary
        let peakSpeed = raw.map(\.speedMps).max() ?? 0
        // If the track never exceeded threshold, keep everything (static object).
        guard peakSpeed > threshold else { return raw }
        // Find last index where speed exceeds the threshold, keep up to 5 frames of
        // lead-out so the chart doesn't end abruptly.
        if let lastMoving = raw.lastIndex(where: { $0.speedMps > threshold }) {
            let keepTo = min(lastMoving + 5, raw.count - 1)
            return Array(raw[...keepTo])
        }
        return raw
    }

    var body: some View {
        let display = trimmedSamples
        if display.count >= 2 {
            // Use persistent peak from AppState (survives ring-buffer eviction)
            let globalPeak = CGFloat(appState.trackPeakSpeed[trackID] ?? 0)

            GroupBox(label: Text("History").font(.caption2)) {
                VStack(alignment: .leading, spacing: 8) {
                    // Heading sparkline (orange) — on top
                    SparklineView(
                        values: display.map { CGFloat($0.headingDeg) }, colour: .orange,
                        label: "Heading (°)")

                    // Velocity sparkline (cyan) with dashed red line at global max
                    SparklineView(
                        values: display.map { CGFloat($0.speedMps) }, colour: .cyan,
                        label: "Velocity (m/s)", peakValue: globalPeak)
                }
            }
        }
    }
}

/// A minimal sparkline chart drawn with a SwiftUI Path.
struct SparklineView: View {
    let values: [CGFloat]
    let colour: Color
    let label: String
    /// Optional peak value shown as a dashed red horizontal line with right-axis label.
    var peakValue: CGFloat? = nil

    /// Font size used for the inline sparkline labels.
    private static let labelFontSize: CGFloat = 8
    /// Vertical padding so labels don't clip the chart edge.
    private static let labelPad: CGFloat = 1
    /// Right margin for metric labels — keeps header, peak and current value aligned.
    private static let metricTrailing: CGFloat = 4
    /// Lighter red for sparkline peak line and labels — perceptually matched to orange/cyan.
    private static let peakRed = Color(red: 1.0, green: 0.45, blue: 0.4)

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            // Header row: label + peak metric (always above the sparkline)
            HStack {
                Text(label).font(.system(size: 9)).foregroundColor(.secondary)
                Spacer()
                if let peak = peakValue {
                    Text(String(format: "%.1f", peak)).font(
                        .system(size: Self.labelFontSize, design: .monospaced)
                    ).foregroundColor(Self.peakRed)
                }
            }.padding(.trailing, Self.metricTrailing)
            GeometryReader { geo in
                let size = geo.size
                // Main sparkline
                sparklinePath(in: size).stroke(colour, lineWidth: 1.5)
                // Peak speed dashed line
                if let peak = peakValue {
                    peakLinePath(peak: peak, in: size).stroke(
                        Self.peakRed, style: StrokeStyle(lineWidth: 1, dash: [4, 3]))
                }
                // In-chart current value label
                currentValueLabel(in: size)
            }.frame(height: 40)
        }
    }

    /// Current value label right-aligned at the trailing edge of the sparkline.
    @ViewBuilder private func currentValueLabel(in size: CGSize) -> some View {
        let minVal = values.min() ?? 0
        let maxVal = max(values.max() ?? 1, peakValue ?? 0)
        let range = maxVal - minVal
        let effectiveRange = range < 0.001 ? 1.0 : range

        if let last = values.last {
            let lastY = size.height - (size.height * (last - minVal) / effectiveRange)
            let nearBottom = lastY > size.height - 12
            let labelY = nearBottom ? lastY - 8 - Self.labelPad : lastY + 8 + Self.labelPad
            let clampedY = min(max(labelY, 4), size.height - 4)
            VStack {
                Spacer().frame(height: max(clampedY - 4, 0))
                HStack {
                    Spacer()
                    Text(String(format: "%.1f", last)).font(
                        .system(size: Self.labelFontSize, design: .monospaced)
                    ).foregroundColor(colour)
                }.padding(.trailing, Self.metricTrailing)
                Spacer()
            }.frame(width: size.width, height: size.height)
        }
    }

    /// Compute the sparkline path for a given canvas size.
    /// Internal for testability via @testable import.
    func sparklinePath(in size: CGSize) -> Path {
        guard values.count >= 2 else { return Path() }

        let minVal = values.min() ?? 0
        let maxVal = max(values.max() ?? 1, peakValue ?? 0)
        let range = maxVal - minVal
        let effectiveRange = range < 0.001 ? 1.0 : range

        return Path { path in
            for (index, value) in values.enumerated() {
                let x = size.width * CGFloat(index) / CGFloat(values.count - 1)
                let y = size.height - (size.height * (value - minVal) / effectiveRange)
                if index == 0 {
                    path.move(to: CGPoint(x: x, y: y))
                } else {
                    path.addLine(to: CGPoint(x: x, y: y))
                }
            }
        }
    }

    /// Compute a horizontal line at the given peak value.
    func peakLinePath(peak: CGFloat, in size: CGSize) -> Path {
        guard values.count >= 2 else { return Path() }

        let minVal = values.min() ?? 0
        let maxVal = max(values.max() ?? 1, peak)
        let range = maxVal - minVal
        let effectiveRange = range < 0.001 ? 1.0 : range
        let y = size.height - (size.height * (peak - minVal) / effectiveRange)

        return Path { path in
            path.move(to: CGPoint(x: 0, y: y))
            path.addLine(to: CGPoint(x: size.width, y: y))
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
/// Sort order for the track list.
enum TrackSortOrder: String, CaseIterable {
    case firstSeen = "First seen"
    case peakSpeed = "Max velocity"
}

/// In run mode: fetches ALL tracks from the run via the API (so prior tracks are visible).
/// In live mode: shows tracks from the current frame.
struct TrackListView: View {
    @EnvironmentObject var appState: AppState
    @State private var runTracks: [RunTrack] = []
    @State private var isFetchingRunTracks = false
    /// Guard flag to prevent fetchRunTracks re-entry when syncing API labels to appState.
    @State private var isSyncingAPILabels = false
    @State private var sortOrder: TrackSortOrder = .firstSeen
    /// Tracks the rank (index) of each track by peak speed.
    /// `climbedAt` is non-nil when the track recently climbed the leaderboard.
    @State private var previousRanks: [String: RankEntry] = [:]

    /// Tracks visible in the current frame (live mode or as supplementary info).
    /// Uses filtered tracks when filters are active.
    private var frameTracks: [Track] {
        let tracks =
            appState.hasActiveFilters
            ? appState.filteredTracks : (appState.currentFrame?.tracks?.tracks ?? [])
        switch sortOrder {
        case .firstSeen: return tracks.sorted { $0.firstSeenNanos < $1.firstSeenNanos }
        case .peakSpeed:
            return sortTracksByPeakSpeed(tracks, trackPeakSpeed: appState.trackPeakSpeed)
        }
    }

    /// Run tracks sorted according to the active sort order.
    /// In peak-speed mode, uses live frame speed when available so the list re-sorts in real time.
    private var sortedRunTracks: [RunTrack] {
        switch sortOrder {
        case .firstSeen:
            return runTracks.sorted { ($0.startUnixNanos ?? 0) < ($1.startUnixNanos ?? 0) }
        case .peakSpeed:
            return sortRunTracksByPeakSpeed(
                runTracks, frameTrackByID: frameTrackByID, trackPeakSpeed: appState.trackPeakSpeed)
        }
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

    /// Snapshot current speed-sorted ranks into previousRanks for climb detection.
    private func updateRanks() {
        previousRanks = computeUpdatedRanks(
            isRunMode: isRunMode, runTracks: runTracks, frameTrackByID: frameTrackByID,
            appState: appState, previousRanks: previousRanks)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Track count badge + sort picker
            HStack(spacing: 4) {
                if isFetchingRunTracks {
                    ProgressView().controlSize(.mini)
                } else {
                    Text("\(displayCount)").font(.caption).foregroundColor(.secondary)
                }
                Spacer()
                Picker("", selection: $sortOrder) {
                    ForEach(TrackSortOrder.allCases, id: \.self) { order in
                        Text(order.rawValue).tag(order)
                    }
                }.pickerStyle(.menu).labelsHidden().controlSize(.mini).frame(maxWidth: 90)
            }

            if isRunMode { runTrackListContent } else { frameTrackListContent }
            Spacer().frame(height: 8)
        }.onChange(of: appState.currentRunID) { _, newRunID in
            if newRunID != nil { fetchRunTracks() } else { runTracks = [] }
        }.onChange(of: appState.userLabels) { _, _ in
            if isRunMode && !isSyncingAPILabels { fetchRunTracks() }
        }.onChange(of: appState.currentFrameIndex) { _, _ in
            if sortOrder == .peakSpeed { updateRanks() }
            syncTrackListOrder()
        }.onChange(of: sortOrder) { _, _ in syncTrackListOrder() }.onAppear {
            if isRunMode { fetchRunTracks() }
            syncTrackListOrder()
        }
    }

    // MARK: - Run Track List (API-fetched)

    @ViewBuilder private var runTrackListContent: some View {
        if runTracks.isEmpty && !isFetchingRunTracks {
            Text("No tracks in run").font(.caption).foregroundColor(.secondary)
        } else {
            ForEach(sortedRunTracks, id: \.trackId) { track in
                let frameTrack = frameTrackByID[track.trackId]
                let isInView = frameTrack != nil
                let statusColour =
                    frameTrack.map { trackStateColour($0.state) } ?? Color.gray.opacity(0.5)
                RunTrackRowView(
                    trackId: track.trackId, shortID: track.trackId.shortTrackID,
                    statusColour: statusColour, isInView: isInView,
                    bestSpeed: bestRunTrackSpeed(
                        trackId: track.trackId, apiPeakSpeed: track.peakSpeedMps,
                        frameTrack: frameTrack, trackPeakSpeed: appState.trackPeakSpeed),
                    showClimbArrow: sortOrder == .peakSpeed
                        && isTrackClimbing(track.trackId, ranks: previousRanks),
                    tags: runTrackTags(
                        track, userLabels: appState.userLabels,
                        userQualityFlags: appState.userQualityFlags),
                    isSelected: track.trackId == appState.selectedTrackID,
                    onSelect: { appState.selectTrack(track.trackId) })
            }
        }
    }

    // MARK: - Frame Track List (live mode)

    @ViewBuilder private var frameTrackListContent: some View {
        if frameTracks.isEmpty {
            Text("No active tracks").font(.caption).foregroundColor(.secondary)
        } else {
            ForEach(frameTracks, id: \.trackID) { track in
                FrameTrackRowView(
                    trackId: track.trackID, shortID: track.trackID.shortTrackID,
                    statusColour: trackStateColour(track.state),
                    speedDisplay: String(
                        format: "%.1f m/s",
                        max(track.peakSpeedMps, appState.trackPeakSpeed[track.trackID] ?? 0)),
                    showClimbArrow: sortOrder == .peakSpeed
                        && isTrackClimbing(track.trackID, ranks: previousRanks),
                    tags: frameTrackTags(track, userLabels: appState.userLabels),
                    isSelected: track.trackID == appState.selectedTrackID,
                    onSelect: { appState.selectTrack(track.trackID) })
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
                    self.isSyncingAPILabels = true
                    syncRunTracksToAppState(tracks, appState: appState)
                    self.isSyncingAPILabels = false
                    syncTrackListOrder()
                }
            } catch { await MainActor.run { self.isFetchingRunTracks = false } }
        }
    }

    /// Push the current visible track ordering into AppState so that
    /// up/down keyboard navigation matches what the user sees in the list.
    private func syncTrackListOrder() {
        if isRunMode {
            appState.trackListOrder = sortedRunTracks.map { $0.trackId }
        } else {
            appState.trackListOrder = frameTracks.map { $0.trackID }
        }
    }
}

// MARK: - Label Panel

struct LabelPanelView: View {
    @EnvironmentObject var appState: AppState

    // Canonical classification labels — order must match proto ObjectClass enum (1=noise .. 9=motorcyclist)
    static let classificationLabels: [(name: String, help: String)] = [
        ("noise", "Spurious track caused by sensor noise, rain, dust, or vegetation"),
        ("dynamic", "Ambiguous detection unsure between moving objects"),
        ("pedestrian", "Person walking, running, or using a mobility aid"),
        ("cyclist", "Person on a bicycle or e-scooter"), ("bird", "Bird or other airborne fauna"),
        ("bus", "Bus, coach, or large passenger vehicle (length > 7 m)"),
        ("car", "Passenger car, SUV, or van"),
        ("truck", "Pickup truck, box truck, or freight vehicle"),
        ("motorcyclist", "Person riding a motorcycle"),
    ]

    // Canonical quality flags — multi-select, must match Go validQualityLabels and Svelte QualityLabel
    static let qualityFlags: [(name: String, help: String)] = [
        ("good", "Clean, accurate track with correct speed and trajectory"),
        ("noisy", "Track has noisy position or speed estimates"),
        ("jitter_velocity", "Speed estimates jitter significantly"),
        ("jitter_heading", "Heading estimates jitter significantly"),
        ("merge", "Two or more distinct objects incorrectly merged into one track"),
        ("split", "Single object incorrectly split into multiple tracks"),
        ("truncated", "Track starts late or ends early compared to the real object"),
        ("disconnected", "Track was lost and recovered — identity may have changed"),
    ]

    @State private var activeFlags: Set<String> = []
    @State private var isCarriedOver: Bool = false

    /// The current label for the selected track, derived from appState for
    /// consistency when labels are assigned via keyboard shortcuts.
    private var currentLabel: String? {
        guard let trackID = appState.selectedTrackID else { return nil }
        return appState.userLabels[trackID]
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let _ = appState.selectedTrackID {
                // Carried-over label badge
                if isCarriedOver {
                    Text("↻ carried").font(.caption2).foregroundColor(.white).padding(
                        .horizontal, 6
                    ).padding(.vertical, 2).background(Color.orange.opacity(0.8)).cornerRadius(4)
                }

                Text("Labels").font(.subheadline).foregroundColor(.secondary)

                // Classification labels + quality flags in two columns
                HStack(alignment: .top, spacing: 8) {
                    // Left column: classification labels (single-select)
                    VStack(alignment: .leading, spacing: 0) {
                        Text("Classification").font(.caption).foregroundColor(.secondary).padding(
                            .bottom, 4)
                        ForEach(Array(Self.classificationLabels.enumerated()), id: \.offset) {
                            index, entry in
                            LabelButton(
                                label: entry.name, shortcut: "\(index + 1)",
                                isActive: currentLabel == entry.name, helpText: entry.help
                            ) { appState.assignLabel(entry.name) }
                        }
                    }.frame(maxWidth: .infinity, alignment: .leading)

                    // Right column: quality flags (multi-select)
                    if appState.currentRunID != nil {
                        VStack(alignment: .leading, spacing: 0) {
                            Text("Flags").font(.caption).foregroundColor(.secondary).padding(
                                .bottom, 4)
                            ForEach(Array(Self.qualityFlags.enumerated()), id: \.offset) {
                                _, entry in
                                FlagToggleButton(
                                    label: entry.name, isActive: activeFlags.contains(entry.name),
                                    helpText: entry.help
                                ) {
                                    withAnimation(.easeOut(duration: 0.3)) {
                                        activeFlags = toggleFlag(entry.name, in: activeFlags)
                                    }
                                    appState.assignQuality(serialiseFlags(activeFlags))
                                }
                            }
                        }.frame(maxWidth: .infinity, alignment: .leading)
                    }
                }.padding(.top, 4)

                // Bulk label: apply to all visible (filtered) tracks
                // TODO: Re-enable when backend bulk-label API is ready
                // Divider().padding(.vertical, 4)
                // BulkLabelView()
            } else {
                Text("Select a track to label").font(.caption).foregroundColor(.secondary)

                // Bulk label available even without selection
                // TODO: Re-enable when backend bulk-label API is ready
                // if appState.filteredTracks.count > 0 {
                //     Divider().padding(.vertical, 4)
                //     BulkLabelView()
                // }
            }
        }.onChange(of: appState.selectedTrackID) { _, newTrackID in
            // Reset feedback when track selection changes
            activeFlags = []
            isCarriedOver = false
            // Fetch existing labels for the newly selected track
            if let trackID = newTrackID, let runID = appState.currentRunID {
                Task {
                    do {
                        let client = RunTrackLabelAPIClient()
                        let track = try await client.getTrack(runID: runID, trackID: trackID)
                        await MainActor.run {
                            guard appState.selectedTrackID == trackID else { return }
                            let result = applyFetchedTrackLabels(
                                track: track, trackID: trackID, appState: appState)
                            activeFlags = result.flags
                            isCarriedOver = result.isCarriedOver
                        }
                    } catch {
                        // Silently ignore — track may not exist in API yet
                    }
                }
            }
        }
    }
}

// MARK: - Bulk Label

/// Apply a classification label to all visible (filtered) tracks at once.
struct BulkLabelView: View {
    @EnvironmentObject var appState: AppState
    @State private var bulkLabelApplied: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            let count = appState.filteredTracks.count
            Text("Label All Visible (\(count))").font(.caption).foregroundColor(.secondary)

            ForEach(Array(LabelPanelView.classificationLabels.enumerated()), id: \.offset) {
                _, entry in
                Button(action: {
                    appState.assignLabelToAllVisible(entry.name)
                    withAnimation(.easeOut(duration: 0.3)) { bulkLabelApplied = entry.name }
                    // Clear feedback after 2 seconds
                    DispatchQueue.main.asyncAfter(deadline: .now() + 2) {
                        if bulkLabelApplied == entry.name { bulkLabelApplied = nil }
                    }
                }) {
                    HStack {
                        Text(entry.name).font(.callout)
                        Spacer()
                        if bulkLabelApplied == entry.name {
                            Image(systemName: "checkmark.circle.fill").foregroundColor(.green).font(
                                .caption)
                        }
                    }.padding(.vertical, 2).padding(.horizontal, 6).background(
                        bulkLabelApplied == entry.name ? Color.green.opacity(0.15) : Color.clear
                    ).cornerRadius(4)
                }.buttonStyle(.plain).help("Apply '\(entry.name)' to all \(count) visible tracks")
                    .disabled(count == 0)
            }
        }
    }
}

/// A styled label button with hover and selection feedback.
struct LabelButton: View {
    let label: String
    let shortcut: String?
    let isActive: Bool
    var helpText: String = ""
    let action: () -> Void

    @State private var isHovered = false

    var body: some View {
        Button(action: action) {
            HStack {
                if let shortcut {
                    ZStack {
                        Circle().fill(
                            isActive
                                ? Color.confirmedGreen.opacity(0.8) : Color.secondary.opacity(0.3)
                        ).frame(width: 16, height: 16)
                        Text(shortcut).font(
                            .system(size: 9, weight: .semibold, design: .monospaced)
                        ).foregroundColor(isActive ? .white : .secondary)
                    }
                }
                Text(displayName(label)).font(.callout)
                Spacer()
            }.padding(.vertical, 3).padding(.horizontal, 6).background(
                isActive
                    ? Color.confirmedGreen.opacity(0.2)
                    : (isHovered ? Color.primary.opacity(0.08) : Color.clear)
            ).cornerRadius(4)
        }.buttonStyle(.plain).onHover { hovering in isHovered = hovering }.help(
            helpText.isEmpty ? displayName(label) : helpText)
    }

    /// Convert snake_case label to a readable display name.
    private func displayName(_ label: String) -> String {
        label.replacingOccurrences(of: "_", with: " ")
    }
}

/// A styled toggle button for multi-select quality flags.
struct FlagToggleButton: View {
    let label: String
    let isActive: Bool
    var helpText: String = ""
    let action: () -> Void

    @State private var isHovered = false

    var body: some View {
        Button(action: action) {
            HStack {
                Image(systemName: isActive ? "checkmark.square.fill" : "square").foregroundColor(
                    isActive ? .accentColor : .secondary
                ).font(.caption)
                Text(displayName(label)).font(.callout)
                Spacer()
            }.padding(.vertical, 3).padding(.horizontal, 6).background(
                isActive
                    ? Color.accentColor.opacity(0.15)
                    : (isHovered ? Color.primary.opacity(0.08) : Color.clear)
            ).cornerRadius(4)
        }.buttonStyle(.plain).onHover { hovering in isHovered = hovering }.help(
            helpText.isEmpty ? displayName(label) : helpText)
    }

    /// Convert snake_case label to a readable display name.
    private func displayName(_ label: String) -> String {
        label.replacingOccurrences(of: "_", with: " ")
    }
}

// MARK: - Filter Pane

/// Standalone filter pane shown as a separate right-side panel.
/// Controls which tracks are visible in the 3D view and track list.
struct FilterBarView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        HStack(spacing: 16) {
            // Only points in boxes toggle
            Toggle("In boxes", isOn: $appState.filterOnlyInBox).font(.caption).toggleStyle(
                .checkbox
            ).fixedSize().help("Hide foreground points that are not inside any bounding box")

            Divider().frame(height: 20)

            // Hits range slider
            HStack(spacing: 4) {
                Text("Hits").font(.caption).foregroundColor(.secondary).frame(
                    width: 50, alignment: .trailing
                ).fixedSize()
                RangeSliderView(
                    low: Binding(
                        get: { Double(appState.filterMinHits) },
                        set: { appState.filterMinHits = Int($0) }),
                    high: Binding(
                        get: { Double(appState.filterMaxHits) },
                        set: { appState.filterMaxHits = Int($0) }), range: 0...50, step: 1
                ).frame(width: 140).help("Hits range (min–max)")
                Text(
                    "\(appState.filterMinHits)–\(appState.filterMaxHits == 0 ? "∞" : "\(appState.filterMaxHits)")"
                ).font(.system(.caption, design: .monospaced)).frame(
                    width: 44, alignment: .trailing
                ).fixedSize()
            }.fixedSize()

            Divider().frame(height: 20)

            // Points/frame range slider
            HStack(spacing: 4) {
                Text("Pts/frm").font(.caption).foregroundColor(.secondary).frame(
                    width: 50, alignment: .trailing
                ).fixedSize()
                RangeSliderView(
                    low: Binding(
                        get: { Double(appState.filterMinPointsPerFrame) },
                        set: { appState.filterMinPointsPerFrame = Int($0) }),
                    high: Binding(
                        get: { Double(appState.filterMaxPointsPerFrame) },
                        set: { appState.filterMaxPointsPerFrame = Int($0) }), range: 0...100,
                    step: 1
                ).frame(width: 140).help("Points per frame range (min–max)")
                Text(
                    "\(appState.filterMinPointsPerFrame)–\(appState.filterMaxPointsPerFrame == 0 ? "∞" : "\(appState.filterMaxPointsPerFrame)")"
                ).font(.system(.caption, design: .monospaced)).frame(
                    width: 44, alignment: .trailing
                ).fixedSize()
            }.fixedSize()

            Divider().frame(height: 20)

            // Active filter summary
            if appState.hasActiveFilters {
                let total = appState.currentFrame?.tracks?.tracks.count ?? 0
                let filtered = appState.filteredTracks.count
                Text("\(filtered)/\(total)").font(.caption).foregroundColor(.orange)
            }

            // Reset button
            if appState.hasActiveFilters {
                Button(action: {
                    appState.filterOnlyInBox = false
                    appState.filterMinHits = 0
                    appState.filterMaxHits = 0
                    appState.filterMinPointsPerFrame = 0
                    appState.filterMaxPointsPerFrame = 0
                    appState.resetAdmittedTracks(reason: "filterResetButton")
                }) { Image(systemName: "arrow.counterclockwise") }.buttonStyle(.plain)
                    .foregroundColor(.accentColor).help("Reset all filters")
            }

            Spacer()

            // Close button
            Button(action: { appState.showFilterPane = false }) {
                Image(systemName: "xmark.circle.fill").foregroundColor(.secondary)
            }.buttonStyle(.plain)
        }.padding(.horizontal, 12).padding(.vertical, 6).background(
            Color(nsColor: .controlBackgroundColor)
        ).onChange(of: appState.filterMinHits) { oldValue, newValue in
            uiLogger.debug("filterMinHits \(oldValue) -> \(newValue)")
            appState.resetAdmittedTracks(reason: "filterMinHits")
        }.onChange(of: appState.filterMaxHits) { oldValue, newValue in
            uiLogger.debug("filterMaxHits \(oldValue) -> \(newValue)")
            appState.resetAdmittedTracks(reason: "filterMaxHits")
        }.onChange(of: appState.filterMinPointsPerFrame) { oldValue, newValue in
            uiLogger.debug("filterMinPointsPerFrame \(oldValue) -> \(newValue)")
            appState.resetAdmittedTracks(reason: "filterMinPointsPerFrame")
        }.onChange(of: appState.filterMaxPointsPerFrame) { oldValue, newValue in
            uiLogger.debug("filterMaxPointsPerFrame \(oldValue) -> \(newValue)")
            appState.resetAdmittedTracks(reason: "filterMaxPointsPerFrame")
        }.onChange(of: appState.filterOnlyInBox) { oldValue, newValue in
            uiLogger.debug("filterOnlyInBox \(oldValue) -> \(newValue)")
            appState.resetAdmittedTracks(reason: "filterOnlyInBox")
        }
    }
}

/// A dual-handle range slider. The low handle cannot exceed the high handle
/// and vice versa. When high is at the maximum value it represents "no limit" (0 in AppState).
/// Compute the x-position for a value within a range slider.
/// Extracted from RangeSliderView for testability.
func rangeSliderXPosition(value: Double, range: ClosedRange<Double>, trackWidth: CGFloat) -> CGFloat
{
    guard trackWidth > 0, range.upperBound > range.lowerBound else { return 0 }
    let fraction = (value - range.lowerBound) / (range.upperBound - range.lowerBound)
    return CGFloat(fraction) * trackWidth
}

/// Compute the snapped value for an x-position within a range slider.
/// Extracted from RangeSliderView for testability.
func rangeSliderValueForX(
    x: CGFloat, range: ClosedRange<Double>, trackWidth: CGFloat, step: Double
) -> Double {
    guard trackWidth > 0, range.upperBound > range.lowerBound, step > 0 else {
        return range.lowerBound
    }
    let fraction = Double(max(0, min(x, trackWidth)) / trackWidth)
    let raw = range.lowerBound + fraction * (range.upperBound - range.lowerBound)
    return (raw / step).rounded() * step
}

struct RangeSliderView: View {
    @Binding var low: Double
    @Binding var high: Double
    let range: ClosedRange<Double>
    var step: Double = 1

    private let thumbRadius: CGFloat = 6
    private let trackHeight: CGFloat = 4

    /// The effective high value: 0 means "no limit" → use range max for display.
    private var effectiveHigh: Double { high <= 0 ? range.upperBound : high }

    var body: some View {
        GeometryReader { geo in
            let trackWidth = max(1, geo.size.width - thumbRadius * 2)
            let xPosition: (Double) -> CGFloat = { value in
                rangeSliderXPosition(value: value, range: range, trackWidth: trackWidth)
            }
            let valueForX: (CGFloat) -> Double = { x in
                rangeSliderValueForX(x: x, range: range, trackWidth: trackWidth, step: step)
            }

            ZStack(alignment: .leading) {
                // Track background
                RoundedRectangle(cornerRadius: trackHeight / 2).fill(Color.gray.opacity(0.3)).frame(
                    height: trackHeight
                ).padding(.horizontal, thumbRadius)

                // Active range highlight
                let lowX = xPosition(low) + thumbRadius
                let highX = xPosition(effectiveHigh) + thumbRadius
                RoundedRectangle(cornerRadius: trackHeight / 2).fill(Color.accentColor.opacity(0.5))
                    .frame(width: max(0, highX - lowX), height: trackHeight).offset(x: lowX)

                // Low thumb
                Circle().fill(Color.accentColor).frame(
                    width: thumbRadius * 2, height: thumbRadius * 2
                ).offset(x: xPosition(low)).gesture(
                    DragGesture(minimumDistance: 0).onChanged { drag in
                        let newVal = min(valueForX(drag.location.x - thumbRadius), effectiveHigh)
                        low = max(range.lowerBound, newVal)
                    })

                // High thumb
                Circle().fill(Color.accentColor).frame(
                    width: thumbRadius * 2, height: thumbRadius * 2
                ).offset(x: xPosition(effectiveHigh)).gesture(
                    DragGesture(minimumDistance: 0).onChanged { drag in
                        let newVal = max(valueForX(drag.location.x - thumbRadius), low)
                        let clamped = min(range.upperBound, newVal)
                        // If dragged to the max, set to 0 (no limit)
                        high = clamped >= range.upperBound ? 0 : clamped
                    })
            }
        }.frame(height: thumbRadius * 2 + 4)
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

            // TODO: Re-enable when backend debug overlay data is available
            // Toggle("Association Lines", isOn: $appState.showAssociation).font(.caption).toggleStyle(
            //     .checkbox
            // ).disabled(!appState.showDebug)

            // Toggle("Gating Ellipses", isOn: $appState.showGating).font(.caption).toggleStyle(
            //     .checkbox
            // ).disabled(!appState.showDebug)

            // Toggle("Residual Vectors", isOn: $appState.showResiduals).font(.caption).toggleStyle(
            //     .checkbox
            // ).disabled(!appState.showDebug)

            // if !appState.showDebug {
            //     Text("Enable Debug (D) to show overlays").font(.caption2).foregroundColor(
            //         .secondary)
            // }
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

/// A single track label pill: short 4-char hex suffix + class label.
/// Mirrors the tentative/confirmed design language:
/// - Yellow text when showing the auto-classifier label (tentative)
/// - Green text when showing a user-assigned label (confirmed)
struct TrackLabelPill: View {
    let label: MetalRenderer.TrackScreenLabel

    /// The display text and colour for the classification label.
    private var classDisplay: (text: String, colour: Color)? {
        if !label.userLabel.isEmpty {
            return (label.userLabel, .confirmedGreen)
        } else if !label.classLabel.isEmpty {
            return (label.classLabel, .yellow)
        }
        return nil
    }

    var body: some View {
        HStack(spacing: 3) {
            Text(label.id.shortTrackID).font(.system(size: 10, design: .monospaced))
                .foregroundColor(.white)

            if let display = classDisplay {
                Text(display.text).font(.system(size: 10)).foregroundColor(display.colour)
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
    var showBackground: Bool
    var showBoxes: Bool
    var showClusters: Bool
    var showVelocity: Bool
    var showTrails: Bool
    var showDebug: Bool
    var showGrid: Bool
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
        renderer.showBackground = showBackground
        renderer.showBoxes = showBoxes
        renderer.showClusters = showClusters
        renderer.showVelocity = showVelocity
        renderer.showTrails = showTrails
        renderer.showDebug = showDebug
        renderer.showGrid = showGrid
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

// MARK: - Flow Layout

/// Horizontal row of tag pills showing at most 2 values.
/// When there are 3+ tags the third slot shows an accented "…" overflow indicator.
struct TagRow: View {
    let tags: [(String, Color)]

    var body: some View {
        HStack(spacing: 3) {
            ForEach(Array(tags.prefix(2).enumerated()), id: \.offset) { _, tag in
                TagPill(text: tag.0, colour: tag.1)
            }
            if tags.count > 2 { TagPill(text: "\u{2026}", colour: .accentColor) }
        }.lineLimit(1).fixedSize(horizontal: false, vertical: true)
    }
}

/// Compact tag pill: 6-character-truncated label with coloured background.
struct TagPill: View {
    let text: String
    let colour: Color

    var body: some View {
        Text(String(text.prefix(6))).font(.system(size: 9, design: .monospaced)).lineLimit(1)
            .fixedSize().foregroundColor(.white).padding(.horizontal, 3).padding(.vertical, 1)
            .background(colour.opacity(0.6)).cornerRadius(3)
    }
}

// MARK: - Confirmed Green Colour

extension Color {
    /// Green used for confirmed/user-assigned labels, matching the confirmed track state.
    /// Slightly desaturated for legibility against dark backgrounds.
    static let confirmedGreen = Color(red: 0.25, green: 0.82, blue: 0.38)
}

// MARK: - Track ID Helpers

extension String {
    /// Returns a short 4-character hex suffix from a track ID (e.g. "trk_a1b2c3d4" → "c3d4").
    var shortTrackID: String {
        // Track IDs use the format "trk_XXXXXXXX". Extract the last 4 hex characters.
        if let underscoreIndex = lastIndex(of: "_") {
            let hex = self[index(after: underscoreIndex)...]
            return String(hex.suffix(4))
        }
        return String(suffix(4))
    }
}
