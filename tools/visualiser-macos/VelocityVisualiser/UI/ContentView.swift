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
                MetalViewRepresentable(
                    showPoints: appState.showPoints,
                    showBoxes: appState.showBoxes,
                    showTrails: appState.showTrails,
                    onRendererCreated: { renderer in
                        appState.registerRenderer(renderer)
                    }
                )
                .frame(minWidth: 400, minHeight: 300)

                // Playback controls
                PlaybackControlsView()
            }
            .frame(minWidth: 600)

            // Side panel
            if appState.showLabelPanel || appState.selectedTrackID != nil {
                SidePanelView()
                    .frame(width: 280)
            }
        }
        .frame(minWidth: 800, minHeight: 600)
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

            Spacer()

            // Stats (only render when connected)
            StatsDisplayView()

            Spacer()

            // Overlay toggles
            OverlayTogglesView()
        }
        .padding(.horizontal)
        .padding(.vertical, 8)
        .background(Color(nsColor: .controlBackgroundColor))
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
                ProgressView()
                    .controlSize(.small)
                    .frame(width: 16, height: 16)
            } else {
                Image(systemName: isConnected ? "stop.circle.fill" : "play.circle.fill")
            }
            Text(isConnecting ? "Connecting..." : (isConnected ? "Disconnect" : "Connect"))
        }
        .tint(isConnected ? .red : .green)
        .disabled(isConnecting)
    }
}

struct ConnectionStatusView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        let isConnected = appState.isConnected
        let errorMessage = appState.connectionError
        let hasError = errorMessage != nil

        HStack(spacing: 4) {
            Circle()
                .fill(isConnected ? .green : (hasError ? .red : .gray))
                .frame(width: 8, height: 8)
            Text(errorMessage ?? (isConnected ? appState.serverAddress : "Disconnected"))
                .font(.caption)
                .foregroundColor(hasError ? .red : .secondary)
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

            HStack(spacing: 16) {
                StatLabel(title: "FPS", value: String(format: "%.1f", fps))
                StatLabel(title: "Points", value: formatNumber(pointCount))
                StatLabel(title: "Tracks", value: "\(trackCount)")
            }
        }
    }

    private func formatNumber(_ n: Int) -> String {
        if n >= 1000 {
            return String(format: "%.2fk", Double(n) / 1000)
        }
        return "\(n)"
    }
}

struct StatLabel: View {
    let title: String
    let value: String

    var body: some View {
        VStack(alignment: .trailing, spacing: 0) {
            Text(title)
                .font(.caption2)
                .foregroundColor(.secondary)
            Text(value)
                .font(.system(.caption, design: .monospaced))
                .fontWeight(.medium)
                .frame(width: 50, alignment: .trailing)
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
            ToggleButton(label: "T", isOn: $appState.showTrails, help: "Trails")
            ToggleButton(label: "V", isOn: $appState.showVelocity, help: "Velocity")

            Divider().frame(height: 20)

            ToggleButton(label: "D", isOn: $appState.showDebug, help: "Debug")
        }
    }
}

struct ToggleButton: View {
    let label: String
    @Binding var isOn: Bool
    let help: String

    var body: some View {
        Button(action: { isOn.toggle() }) {
            Text(label)
                .font(.system(.caption, design: .monospaced))
                .fontWeight(isOn ? .bold : .regular)
                .foregroundColor(isOn ? .white : .secondary)
                .frame(width: 24, height: 24)
                .background(
                    isOn ? Color.accentColor : Color(nsColor: .controlBackgroundColor).opacity(0.5)
                )
                .cornerRadius(4)
        }
        .buttonStyle(.plain)
        .focusable(false)
        .help(help)
    }
}

// MARK: - Playback Controls

struct PlaybackControlsView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        let isConnected = appState.isConnected
        let isLive = appState.isLive
        let isPaused = appState.isPaused
        let playbackRate = appState.playbackRate

        HStack {
            // Play/Pause (disabled in live mode)
            Button(action: { appState.togglePlayPause() }) {
                Image(systemName: isPaused ? "play.fill" : "pause.fill")
            }
            .disabled(!isConnected || isLive)

            // Step buttons
            Button(action: { appState.stepBackward() }) {
                Image(systemName: "backward.frame.fill")
            }
            .disabled(!isConnected || isLive)

            Button(action: { appState.stepForward() }) {
                Image(systemName: "forward.frame.fill")
            }
            .disabled(!isConnected || isLive)

            // Timeline (replay mode)
            if !isLive {
                Slider(value: $appState.replayProgress, in: 0...1) { editing in
                    if !editing {
                        appState.seek(to: appState.replayProgress)
                    }
                }
                .frame(minWidth: 200)
            } else {
                Spacer()
            }

            // Rate control (disabled in live mode)
            HStack(spacing: 4) {
                Button(action: { appState.decreaseRate() }) {
                    Image(systemName: "minus")
                }
                .buttonStyle(.borderless)
                .disabled(!isConnected || isLive)

                Text(String(format: "%.2fx", playbackRate))
                    .font(.caption)
                    .frame(width: 40)
                    .foregroundColor(isLive ? .secondary : .primary)

                Button(action: { appState.increaseRate() }) {
                    Image(systemName: "plus")
                }
                .buttonStyle(.borderless)
                .disabled(!isConnected || isLive)
            }
            .opacity(isLive ? 0.5 : 1.0)

            // Mode indicator (only show when connected)
            ModeIndicatorView(isLive: isLive, isConnected: isConnected)
        }
        .padding(.horizontal)
        .padding(.vertical, 8)
        .background(Color(nsColor: .controlBackgroundColor))
    }
}

struct ModeIndicatorView: View {
    let isLive: Bool
    let isConnected: Bool

    var body: some View {
        if isConnected {
            Text(isLive ? "LIVE" : "REPLAY")
                .font(.caption)
                .fontWeight(.bold)
                .foregroundColor(isLive ? .red : .orange)
                .padding(.horizontal, 8)
                .padding(.vertical, 2)
                .background(isLive ? Color.red.opacity(0.2) : Color.orange.opacity(0.2))
                .cornerRadius(4)
        }
    }
}

// MARK: - Side Panel

struct SidePanelView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            // Track info
            if let trackID = appState.selectedTrackID {
                TrackInspectorView(trackID: trackID)
            }

            Divider()

            // Label panel
            LabelPanelView()

            Spacer()
        }
        .padding()
        .background(Color(nsColor: .controlBackgroundColor))
    }
}

// MARK: - Track Inspector

struct TrackInspectorView: View {
    let trackID: String
    @EnvironmentObject var appState: AppState

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Track Inspector")
                .font(.headline)

            Text("ID: \(trackID)")
                .font(.caption)
                .foregroundColor(.secondary)

            // TODO: Show track details from current frame

            Button("Deselect") {
                appState.selectTrack(nil)
            }
        }
    }
}

// MARK: - Label Panel

struct LabelPanelView: View {
    @EnvironmentObject var appState: AppState

    let labels = ["pedestrian", "car", "cyclist", "bird", "other"]

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Label Track")
                .font(.headline)

            if let trackID = appState.selectedTrackID {
                Text("Track: \(trackID)")
                    .font(.caption)
                    .foregroundColor(.secondary)

                ForEach(Array(labels.enumerated()), id: \.offset) { index, label in
                    Button(action: { appState.assignLabel(label) }) {
                        HStack {
                            Text("\(index + 1)")
                                .font(.system(.caption, design: .monospaced))
                                .foregroundColor(.secondary)
                            Text(label)
                            Spacer()
                        }
                    }
                    .buttonStyle(.plain)
                    .padding(.vertical, 4)
                }
            } else {
                Text("Select a track to label")
                    .font(.caption)
                    .foregroundColor(.secondary)
            }
        }
    }
}

// MARK: - Metal View

struct MetalViewRepresentable: NSViewRepresentable {
    // Only pass stable properties - frame updates will come directly to the renderer
    var showPoints: Bool
    var showBoxes: Bool
    var showTrails: Bool

    // Closure to register the renderer with AppState
    var onRendererCreated: ((MetalRenderer) -> Void)?

    func makeNSView(context: Context) -> MTKView {
        let metalView = MTKView()
        metalView.preferredFramesPerSecond = 60
        metalView.enableSetNeedsDisplay = false
        metalView.isPaused = false

        // Create renderer
        if let renderer = MetalRenderer(metalView: metalView) {
            context.coordinator.renderer = renderer
            // Register the renderer so it can receive frame updates directly
            onRendererCreated?(renderer)
        }

        return metalView
    }

    func updateNSView(_ nsView: MTKView, context: Context) {
        guard let renderer = context.coordinator.renderer else {
            return
        }

        // Only update overlay settings - frames come directly to renderer
        renderer.showPoints = showPoints
        renderer.showBoxes = showBoxes
        renderer.showTrails = showTrails
    }

    func makeCoordinator() -> Coordinator {
        Coordinator()
    }

    class Coordinator {
        var renderer: MetalRenderer?
    }
}

// MARK: - Preview

#Preview {
    ContentView()
        .environmentObject(AppState())
}
