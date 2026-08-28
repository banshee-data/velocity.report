// VelocityVisualiserApp.swift
// Main entry point for the Velocity Visualiser macOS application.
//
// This app provides 3D visualisation of LiDAR point clouds, tracks, and
// debug overlays from the velocity.report tracking pipeline via gRPC.
//
// Note: "AttributeGraph: cycle detected" warnings on startup are a known
// SwiftUI bug in macOS 15.x affecting apps using @EnvironmentObject with
// Commands. These warnings are harmless and don't affect functionality.
// See: https://developer.apple.com/forums/thread/738596

import SwiftUI

private let appLogger = DevLogger(category: "App")

@main struct VelocityVisualiserApp: App {
    @StateObject private var appState = AppState()
    private static let watchdog = MainThreadWatchdog()

    var body: some Scene {
        WindowGroup {
            if AppState.isRunningUnderXCTest {
                EmptyView()
            } else {
                ContentView().environmentObject(appState)
            }
        }.commands {
            // Use simple commands without observing state changes
            // to minimize AttributeGraph cycles
            AppCommands(appState: appState)
        }

        Window("About VelocityVisualiser.app", id: "about") { AboutView() }.windowResizability(
            .contentSize
        ).defaultPosition(.center)
    }

    init() {
        // Before anything logs. Redirected stdout is block-buffered, so without
        // this the app's own output reaches a log file late, in 4 KB chunks,
        // and truncated mid-line when the process is killed.
        AppLog.configure()

        // Independent of the frame stream on purpose: when the stream stalls,
        // this is the only thing that can say whether the main thread stalled
        // with it.
        if !AppState.isRunningUnderXCTest { VelocityVisualiserApp.watchdog.start() }

        // Disable the macOS default tab bar (removes "Show Tab Bar" from View menu)
        NSWindow.allowsAutomaticWindowTabbing = false

        NotificationCenter.default.addObserver(
            forName: NSApplication.willTerminateNotification, object: nil, queue: .main
        ) { _ in appLogger.info("Application terminating, goodbye 👋") }
    }
}

// MARK: - Unified Commands (minimizes AttributeGraph cycles)

struct AppCommands: Commands {
    let appState: AppState
    @Environment(\.openWindow) private var openWindow

    var body: some Commands {
        // About panel
        CommandGroup(replacing: .appInfo) {
            Button("About VelocityVisualiser.app") { openWindow(id: "about") }
        }

        // View menu. Replacing .toolbar drops the macOS defaults (tab bar,
        // toolbar customisation) and puts our own items in their place.
        CommandGroup(replacing: .toolbar) {
            Button("Clear") { appState.clearAll() }.keyboardShortcut(
                "k", modifiers: [.command, .shift])
        }
        CommandGroup(replacing: .sidebar) {}

        // Connection commands
        CommandGroup(replacing: .newItem) {
            Button("Connect/Disconnect") { appState.toggleConnection() }.keyboardShortcut(
                "c", modifiers: [.command, .shift])
        }

        // Playback commands
        CommandMenu("Playback") {
            Button("Play/Pause") { appState.togglePlayPause() }.keyboardShortcut(" ", modifiers: [])

            Button("Step Forward") { appState.stepForward() }.keyboardShortcut(".", modifiers: [])

            Button("Step Backward") { appState.stepBackward() }.keyboardShortcut(",", modifiers: [])

            Divider()

            Button("Increase Rate") { appState.increaseRate() }.keyboardShortcut("]", modifiers: [])

            Button("Decrease Rate") { appState.decreaseRate() }.keyboardShortcut("[", modifiers: [])

            Divider()

            // Time display mode — explicit buttons because Picker in Commands
            // doesn't reliably call its setter (AppCommands is not @ObservedObject).
            Button("Elapsed Time") { appState.timeDisplayMode = .elapsed }
            Button("Remaining Time") { appState.timeDisplayMode = .remaining }
            Button("Frame Index") { appState.timeDisplayMode = .frames }
        }

        // Overlay commands - use direct bindings
        CommandMenu("Overlays") {
            Toggle(
                "Points",
                isOn: Binding(get: { appState.showPoints }, set: { appState.showPoints = $0 })
            ).keyboardShortcut("p", modifiers: [])

            Toggle(
                "Boxes",
                isOn: Binding(get: { appState.showBoxes }, set: { appState.showBoxes = $0 })
            ).keyboardShortcut("b", modifiers: [])

            Toggle(
                "Trails",
                isOn: Binding(get: { appState.showTrails }, set: { appState.showTrails = $0 })
            ).keyboardShortcut("t", modifiers: [])

            Toggle(
                "Velocity",
                isOn: Binding(get: { appState.showVelocity }, set: { appState.showVelocity = $0 })
            ).keyboardShortcut("v", modifiers: [])

            Toggle(
                "Grid", isOn: Binding(get: { appState.showGrid }, set: { appState.showGrid = $0 })
            ).keyboardShortcut("g", modifiers: [])

            Toggle(
                "Labels",
                isOn: Binding(
                    get: { appState.showTrackLabels }, set: { appState.showTrackLabels = $0 })
            ).keyboardShortcut("l", modifiers: [])
        }

        // Label commands — classification shortcuts at root level
        CommandMenu("Labels") {
            ForEach(Array(LabelPanelView.classificationLabels.enumerated()), id: \.offset) {
                index, entry in
                Button(entry.name) { appState.assignLabel(entry.name) }.keyboardShortcut(
                    KeyEquivalent(Character(String(index + 1))), modifiers: [])
            }
        }
    }
}
