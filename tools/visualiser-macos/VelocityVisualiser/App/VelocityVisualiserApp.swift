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

@main
struct VelocityVisualiserApp: App {
    @StateObject private var appState = AppState()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(appState)
        }
        .commands {
            // Use simple commands without observing state changes
            // to minimize AttributeGraph cycles
            AppCommands(appState: appState)
        }
    }
}

// MARK: - Unified Commands (minimizes AttributeGraph cycles)

struct AppCommands: Commands {
    let appState: AppState

    var body: some Commands {
        // Connection commands
        CommandGroup(replacing: .newItem) {
            Button("Connect/Disconnect") {
                appState.toggleConnection()
            }
            .keyboardShortcut("c", modifiers: [.command, .shift])

            Divider()

            Button("Open Recording...") {
                appState.openRecording()
            }
            .keyboardShortcut("o", modifiers: .command)
        }

        // Playback commands
        CommandMenu("Playback") {
            Button("Play/Pause") {
                appState.togglePlayPause()
            }
            .keyboardShortcut(" ", modifiers: [])

            Button("Step Forward") {
                appState.stepForward()
            }
            .keyboardShortcut(".", modifiers: [])

            Button("Step Backward") {
                appState.stepBackward()
            }
            .keyboardShortcut(",", modifiers: [])

            Divider()

            Button("Increase Rate") {
                appState.increaseRate()
            }
            .keyboardShortcut("]", modifiers: [])

            Button("Decrease Rate") {
                appState.decreaseRate()
            }
            .keyboardShortcut("[", modifiers: [])
        }

        // Overlay commands - use direct bindings
        CommandMenu("Overlays") {
            Toggle(
                "Points",
                isOn: Binding(
                    get: { appState.showPoints },
                    set: { appState.showPoints = $0 }
                )
            )
            .keyboardShortcut("p", modifiers: [])

            Toggle(
                "Boxes",
                isOn: Binding(
                    get: { appState.showBoxes },
                    set: { appState.showBoxes = $0 }
                )
            )
            .keyboardShortcut("b", modifiers: [])

            Toggle(
                "Trails",
                isOn: Binding(
                    get: { appState.showTrails },
                    set: { appState.showTrails = $0 }
                )
            )
            .keyboardShortcut("t", modifiers: [])

            Toggle(
                "Velocity",
                isOn: Binding(
                    get: { appState.showVelocity },
                    set: { appState.showVelocity = $0 }
                )
            )
            .keyboardShortcut("v", modifiers: [])

            Divider()

            Toggle(
                "Debug Overlays",
                isOn: Binding(
                    get: { appState.showDebug },
                    set: { appState.showDebug = $0 }
                )
            )
            .keyboardShortcut("d", modifiers: [])
        }

        // Label commands
        CommandMenu("Labels") {
            Button("Label Selected Track") {
                appState.showLabelPanel = true
            }
            .keyboardShortcut("l", modifiers: [])

            Divider()

            Button("Export Labels...") {
                appState.exportLabels()
            }
            .keyboardShortcut("e", modifiers: .command)
        }
    }
}
