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

    enum PlaybackMode: String, Equatable {
        case unknown
        case live
        case replayNonSeekable
        case replaySeekable

        var modeLabel: String {
            switch self {
            case .unknown: return "CONNECTING"
            case .live: return "LIVE"
            case .replayNonSeekable: return "REPLAY (PCAP)"
            case .replaySeekable: return "REPLAY (VRLOG)"
            }
        }
    }

    enum PlaybackCommandKind: Equatable {
        case togglePlayPause
        case seek
        case stepForward
        case stepBackward
        case setRate
    }

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
    @Published var playbackMode: PlaybackMode = .live
    @Published fileprivate(set) var hasPlaybackMetadata: Bool = false

    // For replay mode
    @Published var logStartTimestamp: Int64 = 0
    @Published var logEndTimestamp: Int64 = 0
    @Published var replayProgress: Double = 0.0
    @Published var isSeekingInProgress: Bool = false  // Prevents progress updates while seeking
    @Published var replayFinished: Bool = false  // True when replay stream reached EOF
    @Published var currentFrameIndex: UInt64 = 0  // 0-based index in log (for stepping)
    @Published var totalFrames: UInt64 = 0
    @Published var isSeekable: Bool = false  // True when seek/step is supported (e.g. .vrlog replay)
    @Published private(set) var inFlightPlaybackCommand: PlaybackCommandKind?
    @Published private(set) var commandStartedAt: Date?
    @Published private(set) var pendingSeekTargetTimestamp: Int64?
    @Published private(set) var pendingSeekTargetFrameIndex: UInt64?

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

    /// Local cache of user-assigned labels, keyed by track ID.
    /// Provides immediate feedback in the track list before the server round-trips.
    @Published var userLabels: [String: String] = [:]

    /// Local cache of user-assigned quality flags, keyed by track ID.
    /// Provides immediate feedback in the track list before the server round-trips.
    @Published var userQualityFlags: [String: String] = [:]

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
    @Published var filterMaxHits: Int = 0  // Maximum hits (0 = no limit)
    @Published var filterMinPointsPerFrame: Int = 0  // Minimum observation count per frame
    @Published var filterMaxPointsPerFrame: Int = 0  // Maximum observation count (0 = no limit)

    /// Track IDs that have been admitted by the filter at least once.
    /// Once a track passes the filter criteria, it stays visible for its entire lifetime.
    /// This set is cleared when filter parameters change, forcing re-evaluation.
    private(set) var admittedTrackIDs: Set<String> = []

    /// Evaluate whether a track passes the current filter criteria.
    private func trackPassesFilter(_ track: Track) -> Bool {
        if filterMinHits > 0 && track.hits < filterMinHits { return false }
        if filterMaxHits > 0 && track.hits > filterMaxHits { return false }
        if filterMinPointsPerFrame > 0 && track.observationCount < filterMinPointsPerFrame {
            return false
        }
        if filterMaxPointsPerFrame > 0 && track.observationCount > filterMaxPointsPerFrame {
            return false
        }
        return true
    }

    /// Update admitted tracks based on current frame.
    /// Tracks that pass filters are permanently admitted until filters change.
    func updateAdmittedTracks() {
        guard let trackSet = currentFrame?.tracks else { return }
        for track in trackSet.tracks {
            if trackPassesFilter(track) { admittedTrackIDs.insert(track.trackID) }
        }
    }

    /// Reset admitted tracks — called when filter parameters change.
    func resetAdmittedTracks() {
        admittedTrackIDs.removeAll()
        // Re-evaluate current frame immediately
        updateAdmittedTracks()
    }

    /// Tracks from the current frame that are admitted (passed filters at some point).
    var filteredTracks: [Track] {
        guard let trackSet = currentFrame?.tracks else { return [] }
        guard hasActiveFilters else { return trackSet.tracks }
        return trackSet.tracks.filter { admittedTrackIDs.contains($0.trackID) }
    }

    /// Set of track IDs that pass the current filters.
    var filteredTrackIDs: Set<String> { Set(filteredTracks.map { $0.trackID }) }

    /// Whether any filter is actively narrowing the track set.
    var hasActiveFilters: Bool {
        filterOnlyInBox || filterMinHits > 0 || filterMaxHits > 0 || filterMinPointsPerFrame > 0
            || filterMaxPointsPerFrame > 0
    }

    // MARK: - Track History (for velocity/heading graphs)

    /// Per-track history of velocity and heading samples.
    struct TrackSample {
        let frameIndex: UInt64
        let speedMps: Float
        let peakSpeedMps: Float
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
    var playbackCommandClientOverride: PlaybackRPCClient?
    private var lastFrameTime: Date = Date()
    private var clientDelegate: ClientDelegateAdapter?
    private let labelClient = LabelAPIClient()  // M6: REST API client for labels
    private let runTrackLabelClient = RunTrackLabelAPIClient()  // Run-track labels
    private var queuedSeekProgress: Double?
    private var playbackStateGeneration: UInt64 = 0

    // MARK: - Run State

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

    // MARK: - Playback Derived State

    var hasValidTimelineRange: Bool { logEndTimestamp > logStartTimestamp }

    var hasFrameIndexProgress: Bool { totalFrames > 1 }

    var playbackControlsBusy: Bool { inFlightPlaybackCommand != nil }

    var displayPlaybackMode: PlaybackMode {
        if !isConnected && playbackMode == .unknown { return .unknown }
        if hasPlaybackMetadata { return playbackMode }
        if playbackMode == .unknown { return .unknown }
        if isLive { return .live }
        return isSeekable ? .replaySeekable : .replayNonSeekable
    }

    var displayReplayProgress: Double {
        if hasValidTimelineRange { return replayProgress }
        guard totalFrames > 1 else { return replayProgress }
        let denom = Double(totalFrames - 1)
        guard denom > 0 else { return replayProgress }
        return max(0, min(1, Double(currentFrameIndex) / denom))
    }

    var canInteractWithSeekSlider: Bool {
        displayPlaybackMode == .replaySeekable && hasValidTimelineRange && !playbackControlsBusy
    }

    var shouldShowReplayMetadataUnavailable: Bool {
        displayPlaybackMode == .replayNonSeekable && !hasValidTimelineRange
            && !hasFrameIndexProgress
    }

    // MARK: - Playback Helpers

    private var playbackRPCClient: PlaybackRPCClient? {
        playbackCommandClientOverride ?? grpcClient
    }

    fileprivate func setPlaybackMode(_ mode: PlaybackMode) {
        playbackMode = mode
        switch mode {
        case .unknown:
            // Preserve legacy defaults for callers/tests that still inspect these fields directly.
            isLive = isConnected ? isLive : true
            isSeekable = false
        case .live:
            isLive = true
            isSeekable = false
        case .replayNonSeekable:
            isLive = false
            isSeekable = false
        case .replaySeekable:
            isLive = false
            isSeekable = true
        }
    }

    private func inferPlaybackMode(isLive: Bool, seekable: Bool) -> PlaybackMode {
        if isLive { return .live }
        return seekable ? .replaySeekable : .replayNonSeekable
    }

    private func bumpPlaybackGeneration() { playbackStateGeneration &+= 1 }

    fileprivate func resetPlaybackState(mode: PlaybackMode) {
        bumpPlaybackGeneration()
        setPlaybackMode(mode)
        hasPlaybackMetadata = false
        isPaused = false
        replayFinished = false
        replayProgress = 0
        isSeekingInProgress = false
        currentTimestamp = 0
        currentFrameID = 0
        logStartTimestamp = 0
        logEndTimestamp = 0
        currentFrameIndex = 0
        totalFrames = 0
        inFlightPlaybackCommand = nil
        commandStartedAt = nil
        pendingSeekTargetTimestamp = nil
        pendingSeekTargetFrameIndex = nil
        queuedSeekProgress = nil
    }

    private func beginPlaybackCommand(_ kind: PlaybackCommandKind) -> Bool {
        guard inFlightPlaybackCommand == nil else { return false }
        inFlightPlaybackCommand = kind
        commandStartedAt = Date()
        return true
    }

    private func finishPlaybackCommand(_ kind: PlaybackCommandKind) {
        if inFlightPlaybackCommand == kind {
            inFlightPlaybackCommand = nil
            commandStartedAt = nil
            if kind != .seek {
                pendingSeekTargetTimestamp = nil
                pendingSeekTargetFrameIndex = nil
            }
        }
        if kind != .seek { isSeekingInProgress = false }
        if let nextSeek = queuedSeekProgress, inFlightPlaybackCommand == nil {
            queuedSeekProgress = nil
            seek(to: nextSeek)
        }
    }

    private func applyPlaybackAck(_ ack: VisualiserPlaybackStatus) {
        isPaused = ack.paused
        playbackRate = ack.rate
        if ack.currentTimestampNs > 0 { currentTimestamp = ack.currentTimestampNs }
        if ack.currentFrameID > 0 { currentFrameID = ack.currentFrameID }
    }

    private func guardPlaybackCommand(
        kind: PlaybackCommandKind, requiresSeekable: Bool = false
    ) -> PlaybackRPCClient? {
        guard isConnected else {
            logger.warning("Ignoring playback command \(String(describing: kind)) — not connected")
            return nil
        }
        guard displayPlaybackMode != .unknown else {
            logger.warning("Ignoring playback command \(String(describing: kind)) — mode unknown")
            return nil
        }
        if requiresSeekable && displayPlaybackMode != .replaySeekable {
            logger.warning(
                "Ignoring playback command \(String(describing: kind)) — replay is not seekable")
            return nil
        }
        guard let client = playbackRPCClient else {
            logger.error(
                "Ignoring playback command \(String(describing: kind)) — gRPC client missing")
            return nil
        }
        return client
    }

    func setPlaybackModeForTesting(_ mode: PlaybackMode) { setPlaybackMode(mode) }

    func canRunPlaybackCommandForTesting(
        _ kind: PlaybackCommandKind, requiresSeekable: Bool = false
    ) -> Bool { guardPlaybackCommand(kind: kind, requiresSeekable: requiresSeekable) != nil }

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
        bumpPlaybackGeneration()
        setPlaybackMode(.unknown)

        grpcClient = VisualiserClient(address: serverAddress)
        grpcClient?.includeDebug = showDebug  // M6: Request debug data when enabled
        clientDelegate = ClientDelegateAdapter(appState: self, generation: playbackStateGeneration)
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
        resetPlaybackState(mode: .unknown)
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

    /// Reset playback state for a new VRLOG replay.
    /// Call before loading a new VRLOG to clear stale state from a previous
    /// replay (finished flag, pause state, progress, timestamps, history).
    /// Note: the gRPC stream restart is handled by the caller AFTER the HTTP
    /// POST so the server already has the VRLOG loaded when frames start.
    func prepareForNewReplay() {
        logger.info("prepareForNewReplay() — resetting playback state")
        resetPlaybackState(mode: .unknown)
        frameCount = 0
        trackHistory = [:]
        userLabels = [:]
        userQualityFlags = [:]
    }

    /// Restart the gRPC frame stream.  Call after loading a new VRLOG on
    /// the server so the client reconnects and picks up the new replay.
    func restartGRPCStream() {
        bumpPlaybackGeneration()
        if grpcClient != nil {
            clientDelegate = ClientDelegateAdapter(
                appState: self, generation: playbackStateGeneration)
            grpcClient?.delegate = clientDelegate
        }
        grpcClient?.restartStream()
    }

    func togglePlayPause() {
        guard displayPlaybackMode != .live else { return }
        guard !playbackControlsBusy else { return }
        guard let client = guardPlaybackCommand(kind: .togglePlayPause) else { return }
        guard beginPlaybackCommand(.togglePlayPause) else { return }

        let previousPaused = isPaused
        let newPaused = !isPaused
        let wasFinished = replayFinished
        isPaused = newPaused  // Optimistic
        logger.info("Toggle play/pause: newPaused=\(newPaused), wasFinished=\(wasFinished)")

        Task { @MainActor [weak self] in
            guard let self else { return }
            defer { self.finishPlaybackCommand(.togglePlayPause) }
            do {
                let ack: VisualiserPlaybackStatus
                if newPaused {
                    ack = try await client.pause()
                } else {
                    ack = try await client.play()
                }
                self.applyPlaybackAck(ack)
                if !newPaused && wasFinished {
                    logger.info("Restarting stream (replay was finished)")
                    self.replayFinished = false
                    self.restartGRPCStream()
                }
            } catch {
                self.isPaused = previousPaused
                logger.error("Failed to toggle playback: \(error.localizedDescription)")
            }
        }
    }

    func stepForward() {
        guard displayPlaybackMode == .replaySeekable else { return }
        guard currentFrameIndex + 1 < totalFrames else { return }  // Don't step past end
        guard !playbackControlsBusy else { return }
        let targetFrame = currentFrameIndex + 1
        runStepCommand(kind: .stepForward, targetFrame: targetFrame)
    }

    func stepBackward() {
        guard displayPlaybackMode == .replaySeekable, currentFrameIndex > 0 else { return }
        guard !playbackControlsBusy else { return }
        let targetFrame = currentFrameIndex - 1
        runStepCommand(kind: .stepBackward, targetFrame: targetFrame)
    }

    private func runStepCommand(kind: PlaybackCommandKind, targetFrame: UInt64) {
        guard let client = guardPlaybackCommand(kind: kind, requiresSeekable: true) else { return }
        guard beginPlaybackCommand(kind) else { return }

        let wasFinished = replayFinished
        let previousPaused = isPaused
        isSeekingInProgress = true
        pendingSeekTargetFrameIndex = targetFrame

        Task { @MainActor [weak self] in
            guard let self else { return }
            defer { self.finishPlaybackCommand(kind) }
            do {
                // Auto-pause so the next frame doesn't immediately overwrite the seek.
                if !self.isPaused {
                    self.isPaused = true
                    let pauseAck = try await client.pause()
                    self.applyPlaybackAck(pauseAck)
                }
                let seekAck = try await client.seek(toFrame: targetFrame)
                self.applyPlaybackAck(seekAck)
                if wasFinished {
                    logger.info("Restarting stream after step (replay was finished)")
                    self.replayFinished = false
                    self.restartGRPCStream()
                }
            } catch {
                self.isPaused = previousPaused
                logger.error("Failed step command: \(error.localizedDescription)")
            }
        }
    }

    // Available playback rates: 0.5x, 1x, 2x, 4x, 8x, 16x, 32x, 64x
    private static let availableRates: [Float] = [0.5, 1.0, 2.0, 4.0, 8.0, 16.0, 32.0, 64.0]

    func increaseRate() {
        guard displayPlaybackMode != .live else { return }
        let currentIndex = Self.availableRates.firstIndex { $0 >= playbackRate } ?? 0
        let newIndex = min(currentIndex + 1, Self.availableRates.count - 1)
        runRateChange(to: Self.availableRates[newIndex], logPrefix: "Increasing")
    }

    func decreaseRate() {
        guard displayPlaybackMode != .live else { return }
        let currentIndex =
            Self.availableRates.lastIndex { $0 <= playbackRate } ?? (Self.availableRates.count - 1)
        let newIndex = max(currentIndex - 1, 0)
        runRateChange(to: Self.availableRates[newIndex], logPrefix: "Decreasing")
    }

    func resetRate() {
        guard displayPlaybackMode != .live else { return }
        runRateChange(to: 1.0, logPrefix: "Resetting")
    }

    private func runRateChange(to newRate: Float, logPrefix: String) {
        guard !playbackControlsBusy else { return }
        guard let client = guardPlaybackCommand(kind: .setRate) else { return }
        guard beginPlaybackCommand(.setRate) else { return }

        let previousRate = playbackRate
        logger.info("\(logPrefix) rate from \(self.playbackRate) to \(newRate)")
        playbackRate = newRate  // Optimistic

        Task { @MainActor [weak self] in
            guard let self else { return }
            defer { self.finishPlaybackCommand(.setRate) }
            do {
                let ack = try await client.setRate(newRate)
                self.applyPlaybackAck(ack)
            } catch {
                self.playbackRate = previousRate
                logger.error("Failed to change rate: \(error.localizedDescription)")
            }
        }
    }

    func seek(to progress: Double) {
        guard displayPlaybackMode == .replaySeekable else { return }
        guard hasValidTimelineRange else { return }

        let clampedProgress = max(0, min(1, progress))
        let targetTimestamp =
            logStartTimestamp + Int64(Double(logEndTimestamp - logStartTimestamp) * clampedProgress)
        if inFlightPlaybackCommand == .seek {
            queuedSeekProgress = clampedProgress
            replayProgress = clampedProgress
            currentTimestamp = targetTimestamp
            pendingSeekTargetTimestamp = targetTimestamp
            return
        }
        guard !playbackControlsBusy else { return }
        guard let client = guardPlaybackCommand(kind: .seek, requiresSeekable: true) else { return }
        guard beginPlaybackCommand(.seek) else { return }
        let previousProgress = replayProgress
        let previousTimestamp = currentTimestamp
        let previousPaused = isPaused
        let wasFinished = replayFinished

        replayProgress = clampedProgress  // Optimistic
        currentTimestamp = targetTimestamp  // Sync timer display with slider position
        isSeekingInProgress = true
        pendingSeekTargetTimestamp = targetTimestamp
        logger.info(
            "Seeking to progress \(clampedProgress) (timestamp \(targetTimestamp)), wasFinished=\(wasFinished)"
        )

        Task { @MainActor [weak self] in
            guard let self else { return }
            defer { self.finishPlaybackCommand(.seek) }
            do {
                let seekAck = try await client.seek(to: targetTimestamp)
                self.applyPlaybackAck(seekAck)

                if wasFinished {
                    logger.info("Replay was finished - sending play and restarting stream")
                    self.replayFinished = false
                    self.isPaused = false
                    let playAck = try await client.play()
                    self.applyPlaybackAck(playAck)
                    self.restartGRPCStream()
                }
            } catch {
                self.replayProgress = previousProgress
                self.currentTimestamp = previousTimestamp
                self.isPaused = previousPaused
                logger.error("Failed to seek: \(error.localizedDescription)")
            }
            self.isSeekingInProgress = false
            self.pendingSeekTargetTimestamp = nil
        }
    }

    /// Called when slider editing state changes
    func setSliderEditing(_ editing: Bool) {
        if playbackControlsBusy && inFlightPlaybackCommand == .seek { return }
        isSeekingInProgress = editing
    }

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
        userLabels[trackID] = label  // Immediate local feedback

        Task {
            do {
                // Use run-track label API when in run replay mode
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
        for track in tracks { userLabels[track.trackID] = label }  // Immediate local feedback

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

    /// Assign quality rating to the selected track.
    func assignQuality(_ quality: String) {
        guard let trackID = selectedTrackID, let runID = currentRunID else { return }
        logger.info("Assigning quality '\(quality)' to track \(trackID)")
        userQualityFlags[trackID] = quality  // Immediate local feedback

        Task {
            do {
                _ = try await runTrackLabelClient.updateLabel(
                    runID: runID, trackID: trackID, qualityLabel: quality)
                logger.info("Quality '\(quality)' saved for track \(trackID)")
            } catch { logger.error("Failed to save quality: \(error.localizedDescription)") }
        }
    }

    /// Mark track as split.
    /// Note: split/merge flags require additional API support in the backend.
    func markAsSplit(_ isSplit: Bool) {
        guard let trackID = selectedTrackID, let _ = currentRunID else { return }
        logger.info("Marking track \(trackID) as split=\(isSplit) (requires backend support)")
        // TODO: Add split flag support to backend API when needed
    }

    /// Mark track as merge.
    /// Note: split/merge flags require additional API support in the backend.
    func markAsMerge(_ isMerge: Bool) {
        guard let trackID = selectedTrackID, let _ = currentRunID else { return }
        logger.info("Marking track \(trackID) as merge=\(isMerge) (requires backend support)")
        // TODO: Add merge flag support to backend API when needed
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

    func onFrameReceived(_ frame: FrameBundle, generation: UInt64? = nil) {
        let eventGeneration = generation ?? playbackStateGeneration
        guard eventGeneration == playbackStateGeneration else {
            logger.debug("Ignoring stale frame for generation \(eventGeneration)")
            return
        }
        // Update non-published frame data immediately (bypasses SwiftUI)
        currentFrame = frame

        // Apply persistent track filters BEFORE rendering so filtered tracks
        // never appear even briefly.
        updateAdmittedTracks()

        // Compute hidden track IDs before forwarding to renderer
        if hasActiveFilters {
            let visibleIDs = filteredTrackIDs
            let allIDs = Set(frame.tracks?.tracks.map { $0.trackID } ?? [])
            renderer?.hiddenTrackIDs = allIDs.subtracting(visibleIDs)
        } else {
            renderer?.hiddenTrackIDs = []
        }

        // Set renderer flags before updateFrame so filtering is applied during rendering
        renderer?.showClusters = showClusters  // M4: Update cluster toggle
        renderer?.showDebug = showDebug  // M6: Debug overlay master toggle
        renderer?.showGating = showGating  // M6: Gating ellipses
        renderer?.showAssociation = showAssociation  // M6: Association lines
        renderer?.showResiduals = showResiduals  // M6: Residual vectors
        renderer?.selectedTrackID = selectedTrackID  // M6: Track selection highlight
        renderer?.filterOnlyInBox = filterOnlyInBox  // Filter foreground points outside boxes

        // Forward frame to renderer (uses hiddenTrackIDs already set above)
        renderer?.updateFrame(frame)

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
                newLabels: newLabels, generation: eventGeneration)
        }
    }

    /// Applies @Published state mutations from a received frame.
    /// Called from a deferred Task to avoid AttributeGraph cycles.
    private func applyFrameStateUpdate(
        frame: FrameBundle, instantFPS: Double, newCacheStatus: String,
        newLabels: [MetalRenderer.TrackScreenLabel], generation: UInt64
    ) {
        guard generation == playbackStateGeneration else {
            logger.debug("Skipping stale deferred frame update for generation \(generation)")
            return
        }
        currentFrameID = frame.frameID
        currentTimestamp = frame.timestampNanos
        frameCount += 1

        // Clear finished state since we're receiving frames again
        if replayFinished { replayFinished = false }

        // Update playback info from frame
        if let playbackInfo = frame.playbackInfo {
            hasPlaybackMetadata = true
            setPlaybackMode(
                inferPlaybackMode(isLive: playbackInfo.isLive, seekable: playbackInfo.seekable))
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

        // Apply pre-computed FPS
        fps = fps == 0 ? instantFPS : (0.2 * instantFPS + 0.8 * fps)

        // Update stats
        pointCount = frame.pointCloud?.pointCount ?? 0
        clusterCount = frame.clusters?.clusters.count ?? 0
        trackCount = frame.tracks?.tracks.count ?? 0

        // Accumulate velocity/heading history for each track.
        // When seeking or stepping backwards, trim samples that are ahead of
        // the current frame to avoid invalid (non-monotonic) graphs.
        if let tracks = frame.tracks?.tracks {
            for track in tracks {
                let sample = TrackSample(
                    frameIndex: currentFrameIndex, speedMps: track.speedMps,
                    peakSpeedMps: track.peakSpeedMps, headingDeg: track.headingRad * 180 / .pi)
                var samples = trackHistory[track.trackID] ?? []
                // Remove any samples at or beyond the current frame index
                // (handles backward seeks and steps)
                samples.removeAll { $0.frameIndex >= currentFrameIndex }
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
        } else if !isLive && !isSeekingInProgress && !hasValidTimelineRange && totalFrames > 1 {
            replayProgress = displayReplayProgress
        }
    }

    func handleStreamFinished(expectedGeneration: UInt64?) {
        if let expectedGeneration, expectedGeneration != playbackStateGeneration {
            logger.debug("Ignoring stale finish callback for generation \(expectedGeneration)")
            return
        }
        replayFinished = true
        isPaused = true  // Pause at end
        replayProgress = 1.0
        // Sync currentTimestamp to logEndTimestamp so the time display matches the seek bar.
        if logEndTimestamp > 0 { currentTimestamp = logEndTimestamp }
    }
}

// MARK: - Client Delegate Adapter

private let delegateLogger = Logger(
    subsystem: "report.velocity.visualiser", category: "ClientDelegate")

/// Adapter to bridge VisualiserClientDelegate to AppState.
@available(macOS 15.0, *)
final class ClientDelegateAdapter: VisualiserClientDelegate, @unchecked Sendable {
    private weak var appState: AppState?
    private let generation: UInt64?

    init(appState: AppState, generation: UInt64? = nil) {
        self.appState = appState
        self.generation = generation
    }

    func clientDidConnect(_ client: VisualiserClient) {
        print("[ClientDelegate] ✅ CLIENT CONNECTED - Starting frame stream")
        delegateLogger.info("clientDidConnect called")
        Task { @MainActor [weak self] in
            self?.appState?.isConnected = true
            self?.appState?.connectionError = nil
            self?.appState?.replayFinished = false
            self?.appState?.hasPlaybackMetadata = false
            self?.appState?.setPlaybackMode(.unknown)
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
            self?.appState?.currentFrame = nil
            self?.appState?.resetPlaybackState(mode: .unknown)
            // Only show simple error message, not verbose gRPC details
            if error != nil { self?.appState?.connectionError = "Connection lost" }
        }
    }

    func client(_ client: VisualiserClient, didReceiveFrame frame: FrameBundle) {
        let generation = self.generation
        Task { @MainActor [weak self] in
            self?.appState?.onFrameReceived(frame, generation: generation)
        }
    }

    func clientDidFinishStream(_ client: VisualiserClient) {
        print("[ClientDelegate] 🏁 REPLAY STREAM FINISHED")
        delegateLogger.info("clientDidFinishStream called - replay reached end")
        let generation = self.generation
        Task { @MainActor [weak self] in
            self?.appState?.handleStreamFinished(expectedGeneration: generation)
        }
    }
}
