// VelocityVisualizerApp.swift
// Main entry point for the Velocity Visualizer macOS application.
//
// This app provides 3D visualisation of LiDAR point clouds, tracks, and
// debug overlays from the velocity.report tracking pipeline via gRPC.

import SwiftUI

@main
struct VelocityVisualizerApp: App {
    @StateObject private var appState = AppState()
    
    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(appState)
        }
        .commands {
            CommandGroup(replacing: .newItem) {
                Button("Connect") {
                    appState.toggleConnection()
                }
                .keyboardShortcut("c", modifiers: [.command, .shift])
                
                Divider()
                
                Button("Open Recording...") {
                    appState.openRecording()
                }
                .keyboardShortcut("o", modifiers: .command)
            }
            
            CommandMenu("Playback") {
                Button(appState.isPaused ? "Play" : "Pause") {
                    appState.togglePlayPause()
                }
                .keyboardShortcut(" ", modifiers: [])
                .disabled(!appState.isConnected)
                
                Button("Step Forward") {
                    appState.stepForward()
                }
                .keyboardShortcut(".", modifiers: [])
                .disabled(!appState.isConnected)
                
                Button("Step Backward") {
                    appState.stepBackward()
                }
                .keyboardShortcut(",", modifiers: [])
                .disabled(!appState.isConnected)
                
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
            
            CommandMenu("Overlays") {
                Toggle("Points", isOn: $appState.showPoints)
                    .keyboardShortcut("p", modifiers: [])
                
                Toggle("Boxes", isOn: $appState.showBoxes)
                    .keyboardShortcut("b", modifiers: [])
                
                Toggle("Trails", isOn: $appState.showTrails)
                    .keyboardShortcut("t", modifiers: [])
                
                Toggle("Velocity", isOn: $appState.showVelocity)
                    .keyboardShortcut("v", modifiers: [])
                
                Divider()
                
                Toggle("Debug Overlays", isOn: $appState.showDebug)
                    .keyboardShortcut("d", modifiers: [])
            }
            
            CommandMenu("Labels") {
                Button("Label Selected Track") {
                    appState.showLabelPanel = true
                }
                .keyboardShortcut("l", modifiers: [])
                .disabled(appState.selectedTrackID == nil)
                
                Divider()
                
                Button("Export Labels...") {
                    appState.exportLabels()
                }
                .keyboardShortcut("e", modifiers: .command)
            }
        }
    }
}
