// AppState.swift
// Global application state for the Velocity Visualiser.
//
// This class manages:
// - Connection state to the gRPC server
// - Playback state (live vs replay, pause/play, rate)
// - Overlay visibility toggles
// - Selected track for labelling

import Combine
import Foundation
import os

private let logger = Logger(subsystem: "report.velocity.visualiser", category: "AppState")

/// Global application state, observable by SwiftUI views.
@available(macOS 15.0, *)
@MainActor
class AppState: ObservableObject {

    // MARK: - Connection State

    @Published var isConnected: Bool = false
    @Published var isConnecting: Bool = false
    @Published var serverAddress: String = "localhost:50051"
    @Published var connectionError: String?

    // MARK: - Playback State

    @Published var isPaused: Bool = false
    @Published var playbackRate: Float = 1.0
    @Published var isLive: Bool = true
    @Published var currentTimestamp: Int64 = 0
    @Published var currentFrameID: UInt64 = 0

    // For replay mode
    @Published var logStartTimestamp: Int64 = 0
    @Published var logEndTimestamp: Int64 = 0
    @Published var replayProgress: Double = 0.0

    // MARK: - Overlay Toggles

    @Published var showPoints: Bool = true
    @Published var showBoxes: Bool = true
    @Published var showTrails: Bool = true
    @Published var showVelocity: Bool = true
    @Published var showDebug: Bool = false

    // MARK: - Labelling State

    @Published var selectedTrackID: String?
    @Published var showLabelPanel: Bool = false

    // MARK: - Frame Data

    @Published var currentFrame: FrameBundle?
    @Published var frameCount: UInt64 = 0
    @Published var fps: Double = 0.0

    // MARK: - Stats

    @Published var pointCount: Int = 0
    @Published var clusterCount: Int = 0
    @Published var trackCount: Int = 0

    // MARK: - Internal

    private var grpcClient: VisualiserClient?
    private var frameReceiveTime: Date = Date()
    private var fpsCounter: Int = 0
    private var fpsTimer: Timer?
    private var clientDelegate: ClientDelegateAdapter?

    // MARK: - Initialisation

    init() {
        // Start FPS timer
        fpsTimer = Timer.scheduledTimer(withTimeInterval: 1.0, repeats: true) { [weak self] _ in
            Task { @MainActor in
                self?.fps = Double(self?.fpsCounter ?? 0)
                self?.fpsCounter = 0
            }
        }
    }

    deinit {
        fpsTimer?.invalidate()
    }

    // MARK: - Connection

    func toggleConnection() {
        print("[AppState] toggleConnection called, isConnected: \(isConnected)")
        logger.info("toggleConnection called, isConnected: \(self.isConnected)")
        if isConnected {
            disconnect()
        } else {
            connect()
        }
    }

    func connect() {
        print("[AppState] 🔌 CONNECTING to \(serverAddress)...")
        logger.info("connect() starting, serverAddress: \(self.serverAddress)")
        isConnecting = true
        connectionError = nil

        grpcClient = VisualiserClient(address: serverAddress)
        clientDelegate = ClientDelegateAdapter(appState: self)
        grpcClient?.delegate = clientDelegate
        logger.debug("Created VisualiserClient and delegate")

        Task {
            do {
                logger.debug("Calling grpcClient.connect()...")
                try await grpcClient?.connect()
                print("[AppState] ✅ CONNECTION SUCCEEDED to \(serverAddress)")
                logger.info("grpcClient.connect() succeeded!")
                await MainActor.run {
                    self.isConnecting = false
                }
            } catch {
                print("[AppState] ❌ CONNECTION FAILED: \(error.localizedDescription)")
                logger.error("Connection error: \(error.localizedDescription)")
                await MainActor.run {
                    self.connectionError = "Failed: cannot connect to \(self.serverAddress)"
                    self.isConnected = false
                    self.isConnecting = false
                    self.grpcClient = nil
                    self.clientDelegate = nil
                }
            }
        }
    }

    func disconnect() {
        print("[AppState] 🔌 DISCONNECTING...")
        logger.info("disconnect() called")
        grpcClient?.disconnect()
        grpcClient = nil
        clientDelegate = nil
        isConnected = false
        currentFrame = nil
        logger.debug("Disconnected")
    }

    // MARK: - Playback Control

    func togglePlayPause() {
        isPaused.toggle()
        // TODO: Send Pause/Play RPC
    }

    func stepForward() {
        // TODO: Request next frame in replay mode
    }

    func stepBackward() {
        // TODO: Request previous frame in replay mode
    }

    func increaseRate() {
        playbackRate = min(playbackRate * 2.0, 4.0)
        // TODO: Send SetRate RPC
    }

    func decreaseRate() {
        playbackRate = max(playbackRate / 2.0, 0.25)
        // TODO: Send SetRate RPC
    }

    func seek(to progress: Double) {
        guard !isLive else { return }

        let targetTimestamp =
            logStartTimestamp + Int64(Double(logEndTimestamp - logStartTimestamp) * progress)
        // TODO: Send Seek RPC
        replayProgress = progress
    }

    // MARK: - Recording

    func openRecording() {
        // TODO: Open file dialog and load recording
        isLive = false
    }

    // MARK: - Labelling

    func selectTrack(_ trackID: String?) {
        selectedTrackID = trackID
    }

    func assignLabel(_ label: String) {
        guard let trackID = selectedTrackID else { return }
        // TODO: Store label in LabelStore
        print("Assigned label '\(label)' to track \(trackID)")
    }

    func exportLabels() {
        // TODO: Export labels to JSON
    }

    // MARK: - Frame Handling

    func onFrameReceived(_ frame: FrameBundle) {
        currentFrame = frame
        currentFrameID = frame.frameID
        currentTimestamp = frame.timestampNanos
        frameCount += 1
        fpsCounter += 1

        // Update stats
        pointCount = frame.pointCloud?.pointCount ?? 0
        clusterCount = frame.clusters?.clusters.count ?? 0
        trackCount = frame.tracks?.tracks.count ?? 0

        // Log every 100 frames to show activity
        if frameCount % 100 == 1 {
            print(
                "[AppState] 📊 Frame \(frameCount): \(pointCount) points, \(trackCount) tracks, FPS: \(String(format: "%.1f", fps))"
            )
            logger.info(
                "Frame \(self.frameCount): \(self.pointCount) points, \(self.trackCount) tracks, FPS: \(String(format: "%.1f", self.fps))"
            )
        }

        // Update replay progress
        if !isLive && logEndTimestamp > logStartTimestamp {
            let progress =
                Double(currentTimestamp - logStartTimestamp)
                / Double(logEndTimestamp - logStartTimestamp)
            replayProgress = max(0, min(1, progress))
        }
    }
}

// MARK: - Client Delegate Adapter

private let delegateLogger = Logger(
    subsystem: "report.velocity.visualiser", category: "ClientDelegate")

/// Adapter to bridge VisualiserClientDelegate to AppState.
@available(macOS 15.0, *)
final class ClientDelegateAdapter: VisualiserClientDelegate, @unchecked Sendable {
    private weak var appState: AppState?

    init(appState: AppState) {
        self.appState = appState
    }

    func clientDidConnect(_ client: VisualiserClient) {
        print("[ClientDelegate] ✅ CLIENT CONNECTED - Starting frame stream")
        delegateLogger.info("clientDidConnect called")
        Task { @MainActor [weak self] in
            self?.appState?.isConnected = true
            self?.appState?.connectionError = nil
            self?.appState?.isLive = true
            delegateLogger.debug("AppState updated: isConnected=true")
        }
    }

    func clientDidDisconnect(_ client: VisualiserClient, error: Error?) {
        print(
            "[ClientDelegate] ❌ CLIENT DISCONNECTED, error: \(error?.localizedDescription ?? "none")"
        )
        delegateLogger.warning(
            "clientDidDisconnect called, error: \(error?.localizedDescription ?? "none")")
        Task { @MainActor [weak self] in
            self?.appState?.isConnected = false
            if let error = error {
                self?.appState?.connectionError = error.localizedDescription
            }
        }
    }

    func client(_ client: VisualiserClient, didReceiveFrame frame: FrameBundle) {
        Task { @MainActor [weak self] in
            self?.appState?.onFrameReceived(frame)
        }
    }
}
