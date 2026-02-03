// ContentView.swift
// Main content view for the Velocity Visualizer.
//
// This view composes the Metal render view with SwiftUI controls.

import SwiftUI
import MetalKit

struct ContentView: View {
    @EnvironmentObject var appState: AppState
    
    var body: some View {
        HSplitView {
            // Main 3D view
            VStack(spacing: 0) {
                // Toolbar
                ToolbarView()
                
                // Metal view
                MetalViewRepresentable()
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
            // Connection
            Button(action: { appState.toggleConnection() }) {
                Image(systemName: appState.isConnected ? "stop.circle.fill" : "play.circle.fill")
                Text(appState.isConnected ? "Disconnect" : "Connect")
            }
            .tint(appState.isConnected ? .red : .green)
            
            Divider().frame(height: 20)
            
            // Connection status
            HStack(spacing: 4) {
                Circle()
                    .fill(appState.isConnected ? Color.green : Color.gray)
                    .frame(width: 8, height: 8)
                Text(appState.isConnected ? appState.serverAddress : "Disconnected")
                    .font(.caption)
                    .foregroundColor(.secondary)
            }
            
            Spacer()
            
            // Stats
            if appState.isConnected {
                HStack(spacing: 16) {
                    StatLabel(title: "FPS", value: String(format: "%.1f", appState.fps))
                    StatLabel(title: "Points", value: formatNumber(appState.pointCount))
                    StatLabel(title: "Tracks", value: "\(appState.trackCount)")
                }
            }
            
            Spacer()
            
            // Overlay toggles
            OverlayTogglesView()
        }
        .padding(.horizontal)
        .padding(.vertical, 8)
        .background(Color(nsColor: .controlBackgroundColor))
    }
    
    private func formatNumber(_ n: Int) -> String {
        if n >= 1000 {
            return String(format: "%.1fk", Double(n) / 1000)
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
                .font(.caption)
                .fontWeight(.medium)
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
                .background(isOn ? Color.accentColor : Color.clear)
                .cornerRadius(4)
        }
        .buttonStyle(.plain)
        .help(help)
    }
}

// MARK: - Playback Controls

struct PlaybackControlsView: View {
    @EnvironmentObject var appState: AppState
    
    var body: some View {
        HStack {
            // Play/Pause
            Button(action: { appState.togglePlayPause() }) {
                Image(systemName: appState.isPaused ? "play.fill" : "pause.fill")
            }
            .disabled(!appState.isConnected)
            
            // Step buttons
            Button(action: { appState.stepBackward() }) {
                Image(systemName: "backward.frame.fill")
            }
            .disabled(!appState.isConnected || appState.isLive)
            
            Button(action: { appState.stepForward() }) {
                Image(systemName: "forward.frame.fill")
            }
            .disabled(!appState.isConnected || appState.isLive)
            
            // Timeline (replay mode)
            if !appState.isLive {
                Slider(value: $appState.replayProgress, in: 0...1) { editing in
                    if !editing {
                        appState.seek(to: appState.replayProgress)
                    }
                }
                .frame(minWidth: 200)
            } else {
                Spacer()
            }
            
            // Rate control
            HStack(spacing: 4) {
                Button(action: { appState.decreaseRate() }) {
                    Image(systemName: "minus")
                }
                .buttonStyle(.borderless)
                
                Text(String(format: "%.2fx", appState.playbackRate))
                    .font(.caption)
                    .frame(width: 40)
                
                Button(action: { appState.increaseRate() }) {
                    Image(systemName: "plus")
                }
                .buttonStyle(.borderless)
            }
            
            // Mode indicator
            Text(appState.isLive ? "LIVE" : "REPLAY")
                .font(.caption)
                .fontWeight(.bold)
                .foregroundColor(appState.isLive ? .red : .orange)
                .padding(.horizontal, 8)
                .padding(.vertical, 2)
                .background(appState.isLive ? Color.red.opacity(0.2) : Color.orange.opacity(0.2))
                .cornerRadius(4)
        }
        .padding(.horizontal)
        .padding(.vertical, 8)
        .background(Color(nsColor: .controlBackgroundColor))
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
    
    func makeNSView(context: Context) -> MTKView {
        let metalView = MTKView()
        metalView.preferredFramesPerSecond = 60
        metalView.enableSetNeedsDisplay = false
        metalView.isPaused = false
        
        // Create renderer
        if let renderer = MetalRenderer(metalView: metalView) {
            context.coordinator.renderer = renderer
        }
        
        return metalView
    }
    
    func updateNSView(_ nsView: MTKView, context: Context) {
        // Update renderer settings from app state
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
