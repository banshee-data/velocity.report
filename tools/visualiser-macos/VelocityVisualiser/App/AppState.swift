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
import UniformTypeIdentifiers
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
    @Published var isSeekable: Bool = false  // True when seek/step is supported (e.g. .vrlog replay)

    // MARK: - Overlay Toggles

    @Published var showPoints: Bool = true
    @Published var showBackground: Bool = true  // Background grid points (K toggle)
    @Published var showBoxes: Bool = true
    @Published var showClusters: Bool = true  // M4: Cluster rendering toggle
    @Published var showTrails: Bool = true
    @Published var showVelocity: Bool = true
    @Published var showDebug: Bool = false
    @Published var showGating: Bool = false  // M6: Gating ellipses
    @Published var showAssociation: Bool = false  // M6: Association lines
    @Published var showResiduals: Bool = false  // M6: Residual vectors
    @Published var showGrid: Bool = true  // Ground reference grid
    @Published var showTrackLabels: Bool = true  // Track ID/class labels above 3D boxes
    @Published var pointSize: Float = 5.0  // Point size for rendering (1-20)

    // MARK: - Labelling State

    @Published var selectedTrackID: String?
    @Published var showLabelPanel: Bool = false
    @Published var showSidePanel: Bool = false
    @Published var showFilterPane: Bool = false  // Separate filter pane on the right

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
    @Published var cacheStatus: String = ""  // M3.5: Background cache status
    @Published var trackLabels: [MetalRenderer.TrackScreenLabel] = []  // Projected track labels for overlay
    @Published var metalViewSize: CGSize = .zero  // Metal view drawable size

    // MARK: - Track Filters

    @Published var filterOnlyInBox: Bool = false  // Only show foreground points inside bounding boxes
    @Published var filterMinHits: Int = 0  // Minimum number of frames (hits) for a track
    @Published var filterMinPointsPerFrame: Int = 0  // Minimum observation count per frame
    @Published var filterMinConfidence: Float = 0.0  // Minimum confidence threshold [0,1]

    /// Tracks from the current frame that pass all active filters.
    var filteredTracks: [Track] {
        guard let trackSet = currentFrame?.tracks else { return [] }
        return trackSet.tracks.filter { track in
            if filterMinHits > 0 && track.hits < filterMinHits { return false }
            if filterMinPointsPerFrame > 0 && track.observationCount < filterMinPointsPerFrame {
                return false
            }
            if filterMinConfidence > 0 && track.confidence < filterMinConfidence { return false }
            return true
        }
    }

    /// Set of track IDs that pass the current filters.
    var filteredTrackIDs: Set<String> { Set(filteredTracks.map { $0.trackID }) }

    /// Whether any filter is actively narrowing the track set.
    var hasActiveFilters: Bool {
        filterOnlyInBox || filterMinHits > 0 || filterMinPointsPerFrame > 0
            || filterMinConfidence > 0
    }

    // MARK: - Track History (for velocity/heading graphs)

    /// Per-track history of velocity and heading samples.
    struct TrackSample {
        let frameIndex: UInt64
        let speedMps: Float
        let headingDeg: Float
    }

    /// Ring buffer of recent samples per track (keyed by trackID).
    private(set) var trackHistory: [String: [TrackSample]] = [:]
    private static let maxHistorySamples = 120  // ~12 s at 10 Hz

    // MARK: - Renderer

    /// Weak reference to the Metal renderer for direct frame delivery
    private weak var renderer: MetalRenderer?

    /// Register the renderer to receive frame updates directly
    func registerRenderer(_ renderer: MetalRenderer) {
        self.renderer = renderer
        logger.info("Renderer registered")
    }

    /// Reproject track labels after camera movement (orbit/pan/zoom).
    func reprojectLabels() {
        guard showTrackLabels, let r = renderer, metalViewSize.width > 0 else { return }
        trackLabels = r.projectTrackLabels(viewSize: metalViewSize)
    }

    // MARK: - Internal

    private var grpcClient: VisualiserClient?
    private var lastFrameTime: Date = Date()
    private var clientDelegate: ClientDelegateAdapter?
    private let labelClient = LabelAPIClient()  // M6: REST API client for labels
    private let runTrackLabelClient = RunTrackLabelAPIClient()  // Phase 4.2: Run-track labels

    // MARK: - Run State (Phase 4.1)

    @Published var currentRunID: String?  // Current run being replayed
    @Published var showRunBrowser: Bool = false  // Show run browser sheet

    // MARK: - Initialisation

    init() {
        // FPS is calculated per-frame using exponential moving average

        // Auto-connect on startup
        Task {
            try? await Task.sleep(nanoseconds: 500_000_000)  // 500ms delay
            await MainActor.run {
                logger.info("Auto-connecting to \(self.serverAddress) on startup")
                self.connect()
            }
        }
    }

    deinit {
        // Cleanup if needed
    }

    // MARK: - Connection

    func toggleConnection() {
        print(
            "[AppState] toggleConnection called, isConnected: \(isConnected), isConnecting: \(isConnecting)"
        )
        logger.info(
            "toggleConnection called, isConnected: \(self.isConnected), isConnecting: \(self.isConnecting)"
        )
        // Guard: ignore toggle while a connection attempt is in flight to
        // prevent the auto-connect / user-click race that immediately
        // disconnects a freshly-established connection.
        if isConnecting {
            logger.info("toggleConnection ignored — connection attempt in progress")
            return
        }
        if isConnected { disconnect() } else { connect() }
    }

    func connect() {
        // Guard: do not start a second connection if one is already
        // in progress or established (prevents auto-connect racing
        // with a user-initiated connect).
        guard !isConnecting && !isConnected else {
            logger.info("connect() skipped — already connecting or connected")
            return
        }
        print("[AppState] 🔌 CONNECTING to \(serverAddress)...")
        logger.info("connect() starting, serverAddress: \(self.serverAddress)")
        isConnecting = true
        connectionError = nil

        grpcClient = VisualiserClient(address: serverAddress)
        grpcClient?.includeDebug = showDebug  // M6: Request debug data when enabled
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
        // Reset playback timestamps to prevent stale values on reconnect
        logStartTimestamp = 0
        logEndTimestamp = 0
        currentTimestamp = 0
        replayProgress = 0
        frameCount = 0
        logger.debug("Disconnected")
    }

    /// Clear all transient visualisation data while preserving the background grid and connection.
    func clearAll() {
        logger.info("clearAll() called — clearing transient data")
        currentFrame = nil
        selectedTrackID = nil
        trackLabels = []
        pointCount = 0
        clusterCount = 0
        trackCount = 0
        cacheStatus = ""
        trackHistory = [:]
        renderer?.clearTransientData()
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
        guard !isLive, isSeekable else { return }
        guard currentFrameIndex + 1 < totalFrames else { return }  // Don't step past end

        Task {
            do {
                // Auto-pause so the next frame doesn't immediately overwrite the seek.
                if !isPaused {
                    isPaused = true
                    try await grpcClient?.pause()
                }
                try await grpcClient?.seek(toFrame: currentFrameIndex + 1)
            } catch { logger.error("Failed to step forward: \(error.localizedDescription)") }
        }
    }

    func stepBackward() {
        guard !isLive, isSeekable, currentFrameIndex > 0 else { return }

        Task {
            do {
                // Auto-pause so the next frame doesn't immediately overwrite the seek.
                if !isPaused {
                    isPaused = true
                    try await grpcClient?.pause()
                }
                try await grpcClient?.seek(toFrame: currentFrameIndex - 1)
            } catch { logger.error("Failed to step backward: \(error.localizedDescription)") }
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
        guard !isLive, isSeekable else { return }

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

    func selectTrack(_ trackID: String?) {
        selectedTrackID = trackID
        renderer?.selectedTrackID = trackID
        if trackID != nil {
            showLabelPanel = true
            showSidePanel = true
        }
        // Reproject labels so the overlay highlights the newly selected track
        reprojectLabels()
    }

    func assignLabel(_ label: String) {
        guard let trackID = selectedTrackID else { return }
        logger.info("Assigning label '\(label)' to track \(trackID)")

        Task {
            do {
                // Phase 4.3: Use run-track label API when in run replay mode
                if let runID = currentRunID {
                    _ = try await runTrackLabelClient.updateLabel(
                        runID: runID, trackID: trackID, userLabel: label)
                    logger.info(
                        "Run-track label '\(label)' saved for track \(trackID) in run \(runID)")
                } else {
                    // Fallback to free-form label API for live mode
                    _ = try await labelClient.createLabel(
                        trackID: trackID, classLabel: label, startTimestampNs: currentTimestamp)
                    logger.info("Label '\(label)' saved for track \(trackID)")
                }
            } catch { logger.error("Failed to save label: \(error.localizedDescription)") }
        }
    }

    /// Assign a label to all tracks that pass the current filters.
    func assignLabelToAllVisible(_ label: String) {
        let tracks = filteredTracks
        guard !tracks.isEmpty else { return }
        logger.info("Assigning label '\(label)' to \(tracks.count) visible tracks")

        Task {
            var succeeded = 0
            var failed = 0
            for track in tracks {
                do {
                    if let runID = currentRunID {
                        _ = try await runTrackLabelClient.updateLabel(
                            runID: runID, trackID: track.trackID, userLabel: label)
                    } else {
                        _ = try await labelClient.createLabel(
                            trackID: track.trackID, classLabel: label,
                            startTimestampNs: currentTimestamp)
                    }
                    succeeded += 1
                } catch {
                    failed += 1
                    logger.error(
                        "Failed to label track \(track.trackID): \(error.localizedDescription)")
                }
            }
            logger.info(
                "Bulk label complete: \(succeeded) succeeded, \(failed) failed out of \(tracks.count)"
            )
        }
    }

    /// Assign quality rating to the selected track (Phase 4.2).
    func assignQuality(_ quality: String) {
        guard let trackID = selectedTrackID, let runID = currentRunID else { return }
        logger.info("Assigning quality '\(quality)' to track \(trackID)")

        Task {
            do {
                _ = try await runTrackLabelClient.updateLabel(
                    runID: runID, trackID: trackID, qualityLabel: quality)
                logger.info("Quality '\(quality)' saved for track \(trackID)")
            } catch { logger.error("Failed to save quality: \(error.localizedDescription)") }
        }
    }

    /// Mark track as split (Phase 4.2).
    /// Note: split/merge flags require additional API support in the backend.
    func markAsSplit(_ isSplit: Bool) {
        guard let trackID = selectedTrackID, let _ = currentRunID else { return }
        logger.info("Marking track \(trackID) as split=\(isSplit) (requires backend support)")
        // TODO: Add split flag support to backend API when needed
    }

    /// Mark track as merge (Phase 4.2).
    /// Note: split/merge flags require additional API support in the backend.
    func markAsMerge(_ isMerge: Bool) {
        guard let trackID = selectedTrackID, let _ = currentRunID else { return }
        logger.info("Marking track \(trackID) as merge=\(isMerge) (requires backend support)")
        // TODO: Add merge flag support to backend API when needed
    }

    func exportLabels() {
        let panel = NSSavePanel()
        panel.allowedContentTypes = [.json]
        panel.nameFieldStringValue = "labels-\(labelClient.sessionID).json"

        panel.begin { [weak self] response in
            guard response == .OK, let url = panel.url else { return }
            guard let self = self else { return }

            Task {
                do {
                    try await self.labelClient.exportToFile(url)
                    logger.info("Labels exported to \(url.path)")
                } catch { logger.error("Failed to export labels: \(error.localizedDescription)") }
            }
        }
    }

    /// Send overlay mode preferences to the server via gRPC.
    func sendOverlayPreferences() {
        Task {
            do {
                try await grpcClient?.setOverlayModes(
                    showPoints: showPoints, showClusters: showClusters, showTracks: showBoxes,
                    showTrails: showTrails, showVelocity: showVelocity, showGating: showGating,
                    showAssociation: showAssociation, showResiduals: showResiduals)
            } catch {
                logger.error("Failed to send overlay preferences: \(error.localizedDescription)")
            }
        }
    }

    /// Toggle debug mode — also toggles includeDebug on the stream.
    func toggleDebug() {
        showDebug.toggle()

        // When debug is enabled, also enable sub-toggles as defaults
        if showDebug {
            showGating = true
            showAssociation = true
            showResiduals = true
        }

        // Update client stream to include/exclude debug data
        grpcClient?.includeDebug = showDebug
        sendOverlayPreferences()
    }

    // MARK: - Frame Handling

    func onFrameReceived(_ frame: FrameBundle) {
        // Update non-published frame data immediately (bypasses SwiftUI)
        currentFrame = frame

        // Forward frame directly to renderer (bypasses SwiftUI)
        renderer?.updateFrame(frame)
        renderer?.showClusters = showClusters  // M4: Update cluster toggle
        renderer?.showDebug = showDebug  // M6: Debug overlay master toggle
        renderer?.showGating = showGating  // M6: Gating ellipses
        renderer?.showAssociation = showAssociation  // M6: Association lines
        renderer?.showResiduals = showResiduals  // M6: Residual vectors
        renderer?.selectedTrackID = selectedTrackID  // M6: Track selection highlight

        // Apply track filters to renderer — compute hidden track IDs
        if hasActiveFilters {
            let visibleIDs = filteredTrackIDs
            let allIDs = Set(frame.tracks?.tracks.map { $0.trackID } ?? [])
            renderer?.hiddenTrackIDs = allIDs.subtracting(visibleIDs)
        } else {
            renderer?.hiddenTrackIDs = []
        }

        // Pre-compute values for deferred UI update
        let now = Date()
        let deltaTime = now.timeIntervalSince(lastFrameTime)
        lastFrameTime = now
        let instantFPS = deltaTime > 0 ? 1.0 / deltaTime : 0
        let newCacheStatus = renderer?.getCacheStatus() ?? ""

        let newLabels: [MetalRenderer.TrackScreenLabel]
        if showTrackLabels, let r = renderer, metalViewSize.width > 0 {
            newLabels = r.projectTrackLabels(viewSize: metalViewSize)
        } else {
            newLabels = []
        }

        // Defer @Published state mutations to the next run loop iteration
        // to avoid SwiftUI AttributeGraph cycles during view updates.
        Task { [weak self] in
            guard let self else { return }
            self.applyFrameStateUpdate(
                frame: frame, instantFPS: instantFPS, newCacheStatus: newCacheStatus,
                newLabels: newLabels)
        }
    }

    /// Applies @Published state mutations from a received frame.
    /// Called from a deferred Task to avoid AttributeGraph cycles.
    private func applyFrameStateUpdate(
        frame: FrameBundle, instantFPS: Double, newCacheStatus: String,
        newLabels: [MetalRenderer.TrackScreenLabel]
    ) {
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
            isSeekable = playbackInfo.seekable
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

        // Apply pre-computed FPS
        fps = fps == 0 ? instantFPS : (0.2 * instantFPS + 0.8 * fps)

        // Update stats
        pointCount = frame.pointCloud?.pointCount ?? 0
        clusterCount = frame.clusters?.clusters.count ?? 0
        trackCount = frame.tracks?.tracks.count ?? 0

        // Accumulate velocity/heading history for each track
        if let tracks = frame.tracks?.tracks {
            for track in tracks {
                let sample = TrackSample(
                    frameIndex: currentFrameIndex, speedMps: track.speedMps,
                    headingDeg: track.headingRad * 180 / .pi)
                var samples = trackHistory[track.trackID] ?? []
                samples.append(sample)
                if samples.count > Self.maxHistorySamples {
                    samples.removeFirst(samples.count - Self.maxHistorySamples)
                }
                trackHistory[track.trackID] = samples
            }
        }

        // M3.5: Update cache status
        cacheStatus = newCacheStatus

        // Update track label overlay positions
        trackLabels = newLabels

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
