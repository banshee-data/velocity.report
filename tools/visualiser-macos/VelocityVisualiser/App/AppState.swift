// AppState.swift
// Global application state for the Velocity Visualiser.
//
// This class manages:
// - Connection state to the gRPC server
// - Playback state (live vs replay, pause/play, rate)
// - Overlay visibility toggles
// - Selected track for labelling

import AppKit
import Combine
import Foundation
import os

private let logger = Logger(subsystem: "report.velocity.visualiser", category: "AppState")

/// Global application state, observable by SwiftUI views.
@available(macOS 15.0, *) @MainActor class AppState: ObservableObject {

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
    @Published var isSeekingInProgress: Bool = false  // Prevents progress updates while seeking
    @Published var replayFinished: Bool = false  // True when replay stream reached EOF
    @Published var currentFrameIndex: UInt64 = 0  // 0-based index in log (for stepping)
    @Published var totalFrames: UInt64 = 0

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

    // Note: currentFrame is NOT @Published to avoid SwiftUI update cycles
    // Frames are delivered directly to the renderer via registerRenderer()
    var currentFrame: FrameBundle?
    @Published var frameCount: UInt64 = 0
    @Published var fps: Double = 0.0

    // MARK: - Stats

    @Published var pointCount: Int = 0
    @Published var clusterCount: Int = 0
    @Published var trackCount: Int = 0

    // MARK: - Renderer

    /// Weak reference to the Metal renderer for direct frame delivery
    private weak var renderer: MetalRenderer?

    /// Register the renderer to receive frame updates directly
    func registerRenderer(_ renderer: MetalRenderer) {
        self.renderer = renderer
        logger.info("Renderer registered")
    }

    // MARK: - Internal

    private var grpcClient: VisualiserClient?
    private var lastFrameTime: Date = Date()
    private var clientDelegate: ClientDelegateAdapter?

    // MARK: - Initialisation

    init() {
        // FPS is calculated per-frame using exponential moving average
    }

    deinit {
        // Cleanup if needed
    }

    // MARK: - Connection

    func toggleConnection() {
        print("[AppState] toggleConnection called, isConnected: \(isConnected)")
        logger.info("toggleConnection called, isConnected: \(self.isConnected)")
        if isConnected { disconnect() } else { connect() }
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
                await MainActor.run { self.isConnecting = false }
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
        guard !isLive else { return }

        let newPaused = !isPaused
        let wasFinished = replayFinished
        isPaused = newPaused  // Update immediately (optimistic)
        logger.info("Toggle play/pause: newPaused=\(newPaused), wasFinished=\(wasFinished)")

        Task {
            do {
                if newPaused {
                    try await grpcClient?.pause()
                } else {
                    try await grpcClient?.play()
                    // If replay had finished and we're resuming, restart the stream
                    if wasFinished {
                        logger.info("Restarting stream (replay was finished)")
                        await MainActor.run { self.replayFinished = false }
                        grpcClient?.restartStream()
                    }
                }
            } catch { logger.error("Failed to toggle playback: \(error.localizedDescription)") }
        }
    }

    func stepForward() {
        guard !isLive else { return }
        guard currentFrameIndex + 1 < totalFrames else { return }  // Don't step past end

        Task {
            do { try await grpcClient?.seek(toFrame: currentFrameIndex + 1) } catch {
                logger.error("Failed to step forward: \(error.localizedDescription)")
            }
        }
    }

    func stepBackward() {
        guard !isLive, currentFrameIndex > 0 else { return }

        Task {
            do { try await grpcClient?.seek(toFrame: currentFrameIndex - 1) } catch {
                logger.error("Failed to step backward: \(error.localizedDescription)")
            }
        }
    }

    // Available playback rates: 0.5x, 1x, 2x, 4x, 8x, 16x, 32x, 64x
    private static let availableRates: [Float] = [0.5, 1.0, 2.0, 4.0, 8.0, 16.0, 32.0, 64.0]

    func increaseRate() {
        guard !isLive else { return }

        // Find next higher rate
        let currentIndex = Self.availableRates.firstIndex { $0 >= playbackRate } ?? 0
        let newIndex = min(currentIndex + 1, Self.availableRates.count - 1)
        let newRate = Self.availableRates[newIndex]
        logger.info("Increasing rate from \(self.playbackRate) to \(newRate)")
        playbackRate = newRate  // Update immediately (optimistic)
        Task {
            do { try await grpcClient?.setRate(newRate) } catch {
                logger.error("Failed to increase rate: \(error.localizedDescription)")
            }
        }
    }

    func decreaseRate() {
        guard !isLive else { return }

        // Find next lower rate
        let currentIndex =
            Self.availableRates.lastIndex { $0 <= playbackRate } ?? (Self.availableRates.count - 1)
        let newIndex = max(currentIndex - 1, 0)
        let newRate = Self.availableRates[newIndex]
        logger.info("Decreasing rate from \(self.playbackRate) to \(newRate)")
        playbackRate = newRate  // Update immediately (optimistic)
        Task {
            do { try await grpcClient?.setRate(newRate) } catch {
                logger.error("Failed to decrease rate: \(error.localizedDescription)")
            }
        }
    }

    func resetRate() {
        guard !isLive else { return }

        let newRate: Float = 1.0
        logger.info("Resetting rate to \(newRate)")
        playbackRate = newRate
        Task {
            do { try await grpcClient?.setRate(newRate) } catch {
                logger.error("Failed to reset rate: \(error.localizedDescription)")
            }
        }
    }

    func seek(to progress: Double) {
        guard !isLive else { return }

        let targetTimestamp =
            logStartTimestamp + Int64(Double(logEndTimestamp - logStartTimestamp) * progress)

        replayProgress = progress  // Update immediately (optimistic)
        isSeekingInProgress = true
        let wasFinished = replayFinished
        logger.info(
            "Seeking to progress \(progress) (timestamp \(targetTimestamp)), wasFinished=\(wasFinished)"
        )

        Task {
            do {
                try await grpcClient?.seek(to: targetTimestamp)

                // If replay had finished, we need to restart the stream to resume playback
                if wasFinished {
                    logger.info("Replay was finished - sending play and restarting stream")
                    // Clear finished flag first to avoid race conditions
                    await MainActor.run {
                        self.replayFinished = false
                        self.isPaused = false
                    }
                    // Tell server to play, then restart stream
                    try await grpcClient?.play()
                    grpcClient?.restartStream()
                }
            } catch { logger.error("Failed to seek: \(error.localizedDescription)") }
            await MainActor.run { self.isSeekingInProgress = false }
        }
    }

    /// Called when slider editing state changes
    func setSliderEditing(_ editing: Bool) { isSeekingInProgress = editing }

    // MARK: - Recording

    func openRecording() {
        // Open file dialog
        let panel = NSOpenPanel()
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = false
        panel.message = "Select a .vrlog directory"

        panel.begin { [weak self] response in
            guard response == .OK, let url = panel.url else { return }
            self?.loadRecording(from: url)
        }
    }

    /// Load a recording from the given URL. Used by openRecording and for testing.
    func loadRecording(from url: URL) {
        Task { @MainActor [weak self] in
            self?.isLive = false
            // Note: Actual replay connection would need a reconnect to replay server
            print("Selected recording: \(url.path)")
        }
    }

    // MARK: - Labelling

    func selectTrack(_ trackID: String?) { selectedTrackID = trackID }

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

        // Clear finished state since we're receiving frames again
        if replayFinished { replayFinished = false }

        // Update playback info from frame
        if let playbackInfo = frame.playbackInfo {
            isLive = playbackInfo.isLive
            logStartTimestamp = playbackInfo.logStartNs
            logEndTimestamp = playbackInfo.logEndNs
            playbackRate = playbackInfo.playbackRate
            currentFrameIndex = playbackInfo.currentFrameIndex
            totalFrames = playbackInfo.totalFrames
            // Note: isPaused is NOT updated from frame to allow optimistic UI updates.
            // The server confirms pause state via the RPC response.

            // Log mode on first frame
            if frameCount == 1 {
                let mode = isLive ? "LIVE" : "REPLAY"
                logger.info(
                    "Mode: \(mode), rate: \(playbackInfo.playbackRate), totalFrames: \(playbackInfo.totalFrames)"
                )
            }
        }

        // Calculate FPS using exponential moving average
        let now = Date()
        let deltaTime = now.timeIntervalSince(lastFrameTime)
        if deltaTime > 0 {
            let instantFPS = 1.0 / deltaTime
            // Exponential moving average with alpha=0.2 for smoothing
            fps = fps == 0 ? instantFPS : (0.2 * instantFPS + 0.8 * fps)
        }
        lastFrameTime = now

        // Update stats
        pointCount = frame.pointCloud?.pointCount ?? 0
        clusterCount = frame.clusters?.clusters.count ?? 0
        trackCount = frame.tracks?.tracks.count ?? 0

        // Forward frame directly to renderer (bypasses SwiftUI)
        renderer?.updateFrame(frame)

        // Log every 100 frames to show activity
        if frameCount % 100 == 1 {
            logger.info(
                "Frame \(self.frameCount): \(self.pointCount) points, \(self.trackCount) tracks, FPS: \(String(format: "%.1f", self.fps))"
            )
        }

        // Update replay progress (skip if user is interacting with slider)
        if !isLive && !isSeekingInProgress && logEndTimestamp > logStartTimestamp {
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

    init(appState: AppState) { self.appState = appState }

    func clientDidConnect(_ client: VisualiserClient) {
        print("[ClientDelegate] ✅ CLIENT CONNECTED - Starting frame stream")
        delegateLogger.info("clientDidConnect called")
        Task { @MainActor [weak self] in
            self?.appState?.isConnected = true
            self?.appState?.connectionError = nil
            self?.appState?.replayFinished = false
            // Note: isLive is determined from first frame's PlaybackInfo
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
            // Only show simple error message, not verbose gRPC details
            if error != nil { self?.appState?.connectionError = "Connection lost" }
        }
    }

    func client(_ client: VisualiserClient, didReceiveFrame frame: FrameBundle) {
        Task { @MainActor [weak self] in self?.appState?.onFrameReceived(frame) }
    }

    func clientDidFinishStream(_ client: VisualiserClient) {
        print("[ClientDelegate] 🏁 REPLAY STREAM FINISHED")
        delegateLogger.info("clientDidFinishStream called - replay reached end")
        Task { @MainActor [weak self] in
            self?.appState?.replayFinished = true
            self?.appState?.isPaused = true  // Pause at end
            self?.appState?.replayProgress = 1.0
        }
    }
}
