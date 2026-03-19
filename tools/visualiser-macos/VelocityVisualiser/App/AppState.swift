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

private let logger = DevLogger(category: "AppState")

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

    /// Display mode for the timeline clock in playback controls.
    enum TimeDisplayMode: String, CaseIterable, Equatable {
        case elapsed  // "0:12 / 1:30"
        case remaining  // "-1:18 / 1:30"
        case frames  // "F120/900"

        /// Returns the next mode in the cycle.
        var next: TimeDisplayMode {
            let all = Self.allCases
            let idx = all.firstIndex(of: self)!
            return all[(idx + 1) % all.count]
        }

        var menuLabel: String {
            switch self {
            case .elapsed: return "Elapsed Time"
            case .remaining: return "Remaining Time"
            case .frames: return "Frame Index"
            }
        }
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
    @Published private(set) var replayFrameEncoding: String?

    // For replay mode
    @Published var logStartTimestamp: Int64 = 0
    @Published var logEndTimestamp: Int64 = 0
    @Published var replayProgress: Double = 0.0
    @Published var isSeekingInProgress: Bool = false  // Prevents progress updates while seeking
    @Published var replayFinished: Bool = false  // True when replay stream reached EOF
    @Published var currentFrameIndex: UInt64 = 0  // 0-based index in log (for stepping)
    @Published var totalFrames: UInt64 = 0
    @Published var isSeekable: Bool = false  // True when seek/step is supported (e.g. .vrlog replay)
    @Published var timeDisplayMode: TimeDisplayMode = .elapsed  // Clock display mode
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
    @Published var showHeadingSource: Bool = false  // Debug: colour boxes by heading source
    @Published var includeAssociatedClusters: Bool = false  // Debug: stream track-associated DBSCAN clusters
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

    /// Ordered track IDs as displayed in the track list.
    /// Updated by TrackListView whenever the list re-sorts so that
    /// up/down keyboard navigation follows the visible ordering.
    var trackListOrder: [String] = []
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
    /// Metal view drawable size — intentionally NOT @Published to avoid
    /// GeometryReader → metalViewSize → objectWillChange → layout → cycle.
    var metalViewSize: CGSize = .zero
    /// Cached data for the currently selected track, updated each frame.
    /// Used by inspector views to avoid reading non-Published currentFrame.
    @Published var selectedTrackData: Track?

    func currentSelectedTrackData() -> Track? { selectedTrackData }

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
    func resetAdmittedTracks(reason: String = "unspecified") {
        let previousCount = admittedTrackIDs.count
        admittedTrackIDs.removeAll()
        // Re-evaluate current frame immediately
        updateAdmittedTracks()
        logger.debug(
            "resetAdmittedTracks(reason: \(reason)) previous=\(previousCount) current=\(self.admittedTrackIDs.count) filtersActive=\(self.hasActiveFilters)"
        )
    }

    /// Tracks that have been admitted by filters, drawn from allSeenTracks
    /// so they persist even after leaving the current frame.
    var filteredTracks: [Track] {
        guard hasActiveFilters else { return Array(allSeenTracks.values) }
        return allSeenTracks.values.filter { admittedTrackIDs.contains($0.trackID) }
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
        let maxSpeedMps: Float
        let headingDeg: Float
    }

    /// Ring buffer of recent samples per track (keyed by trackID).
    private(set) var trackHistory: [String: [TrackSample]] = [:]
    /// Persistent all-time max speed per track (survives ring-buffer eviction).
    private(set) var trackMaxSpeed: [String: Float] = [:]
    /// Persistent all-time max hits per track across the current session/replay.
    private(set) var trackMaxHits: [String: Int] = [:]
    private static let maxHistorySamples = 120  // ~12 s at 10 Hz

    /// Accumulated tracks across all frames in the current session/replay.
    /// Tracks persist in this dictionary even after they leave the sensor's field of view,
    /// ensuring the track list does not lose entries when a track goes out of frame.
    /// Cleared on disconnect, new replay, or clearAll.
    private(set) var allSeenTracks: [String: Track] = [:]
    /// Track IDs present in the most recent frame (for in-view indicators).
    private(set) var inViewTrackIDs: Set<String> = []

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
        trackLabels = r.projectTrackLabels(viewSize: metalViewSize).map { label in
            var patched = label
            patched.userLabel = userLabels[label.id] ?? ""
            return patched
        }
    }

    /// Update the cached Metal view size and skip redundant writes to reduce SwiftUI churn.
    func updateMetalViewSize(_ newSize: CGSize, source: String) {
        guard metalViewSize != newSize else { return }
        let formattedSize = String(format: "%.1fx%.1f", newSize.width, newSize.height)
        logger.debug("metalViewSize <- \(formattedSize) source=\(source)")
        metalViewSize = newSize
    }

    // MARK: - Internal

    private var grpcClient: VisualiserClient?
    var playbackCommandClientOverride: PlaybackRPCClient?
    private var lastFrameTime: Date = Date()
    /// Clock for throttling @Published UI updates when the side panel is
    /// open.  Metal rendering runs at full speed regardless.
    private var lastUIUpdateClock = ContinuousClock.now
    private var clientDelegate: ClientDelegateAdapter?
    private let labelClient = LabelAPIClient()  // M6: REST API client for labels
    private let runTrackLabelClient = RunTrackLabelAPIClient()  // Run-track labels
    let runBrowserState = RunBrowserState()
    private var queuedSeekProgress: Double?
    private var seekWaitFrameCount: Int = 0  // Frames since seek RPC completed; safety valve
    private var playbackStateGeneration: UInt64 = 0
    private var lastSeenReplayEpoch: UInt64 = 0

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
        return playbackMode
    }

    var displayReplayProgress: Double {
        if hasValidTimelineRange { return replayProgress }
        guard totalFrames > 1 else { return replayProgress }
        let denom = Double(totalFrames - 1)
        guard denom > 0 else { return replayProgress }
        return max(0, min(1, Double(currentFrameIndex) / denom))
    }

    var shouldShowLegacyJSONReplayBadge: Bool {
        displayPlaybackMode == .replaySeekable && replayFrameEncoding == "json"
    }

    var canInteractWithSeekSlider: Bool {
        displayPlaybackMode == .replaySeekable && (hasValidTimelineRange || hasFrameIndexProgress)
            && !playbackControlsBusy
    }

    var shouldShowReplayMetadataUnavailable: Bool {
        displayPlaybackMode == .replayNonSeekable && !hasValidTimelineRange
            && !hasFrameIndexProgress
    }

    // MARK: - Playback Helpers

    private var playbackRPCClient: PlaybackRPCClient? {
        playbackCommandClientOverride ?? grpcClient
    }

    func setPlaybackMode(_ mode: PlaybackMode) {
        playbackMode = mode
        if mode != .replaySeekable { replayFrameEncoding = nil }
        switch mode {
        case .unknown:
            isLive = false
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

    func setReplayFrameEncoding(_ frameEncoding: String?) {
        replayFrameEncoding = frameEncoding?.lowercased()
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
        replayFrameEncoding = nil
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
        grpcClient?.includeAssociatedClusters = includeAssociatedClusters
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
        trackHistory = [:]
        trackMaxSpeed = [:]
        trackMaxHits = [:]
        allSeenTracks = [:]
        inViewTrackIDs = []
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
        trackMaxSpeed = [:]
        trackMaxHits = [:]
        allSeenTracks = [:]
        inViewTrackIDs = []
        renderer?.clearTransientData()
    }

    // MARK: - Playback Control

    /// Reset playback state for a new VRLOG replay.
    /// Call before loading a new VRLOG to clear stale state from a previous
    /// replay (finished flag, pause state, progress, timestamps, history).
    /// Note: the gRPC stream restart is handled by the caller AFTER the HTTP
    /// POST so the server already has the VRLOG loaded when frames start.
    func prepareForNewReplay() {
        logger.info(
            "prepareForNewReplay() — resetting playback state (was: mode=\(self.displayPlaybackMode.rawValue) paused=\(self.isPaused) finished=\(self.replayFinished) frame=\(self.currentFrameIndex)/\(self.totalFrames))"
        )
        resetPlaybackState(mode: .unknown)
        frameCount = 0
        trackHistory = [:]
        trackMaxSpeed = [:]
        trackMaxHits = [:]
        allSeenTracks = [:]
        inViewTrackIDs = []
        userLabels = [:]
        userQualityFlags = [:]
    }

    /// Restart the gRPC frame stream.  Call after loading a new VRLOG on
    /// the server so the client reconnects and picks up the new replay.
    func restartGRPCStream() {
        logger.debug(
            "restartGRPCStream() called — generation will bump from \(self.playbackStateGeneration)"
        )
        bumpPlaybackGeneration()
        if grpcClient != nil {
            clientDelegate = ClientDelegateAdapter(
                appState: self, generation: playbackStateGeneration)
            grpcClient?.delegate = clientDelegate
        }
        grpcClient?.restartStream()
    }

    func togglePlayPause() {
        logger.debug(
            "togglePlayPause() called — mode=\(self.displayPlaybackMode.rawValue) paused=\(self.isPaused) finished=\(self.replayFinished) busy=\(self.playbackControlsBusy) frame=\(self.currentFrameIndex)/\(self.totalFrames)"
        )
        guard displayPlaybackMode != .live else {
            logger.debug("togglePlayPause() ignored — live mode")
            return
        }
        guard !playbackControlsBusy else {
            logger.debug(
                "togglePlayPause() ignored — playback controls busy (inFlight=\(String(describing: self.inFlightPlaybackCommand)))"
            )
            return
        }

        // When replay has finished, restart from the beginning.
        // The server pauses at EOF (keeps the stream alive), so Seek+Play
        // RPCs work on the existing connection.  Fall back to stream
        // restart if the RPCs fail for any reason.
        if replayFinished {
            logger.info("Replay finished — restarting from beginning")
            isPaused = false
            replayFinished = false
            replayProgress = 0
            currentTimestamp = logStartTimestamp
            currentFrameIndex = 0

            Task { @MainActor [weak self] in
                guard let self else { return }
                if let client = self.playbackRPCClient {
                    do {
                        logger.debug("Restart: trying seek(to: \(self.logStartTimestamp)) + play()")
                        let seekAck = try await client.seek(to: self.logStartTimestamp)
                        self.applyPlaybackAck(seekAck)
                        let playAck = try await client.play()
                        self.applyPlaybackAck(playAck)
                        logger.debug("Restart: seek+play succeeded")
                    } catch {
                        logger.warning(
                            "Restart RPCs failed: \(error.localizedDescription) — falling back to stream restart"
                        )
                        self.restartGRPCStream()
                    }
                } else {
                    logger.debug(
                        "togglePlayPause() restart path — no playbackRPCClient, restarting stream")
                    self.restartGRPCStream()
                }
            }
            return
        }

        guard let client = guardPlaybackCommand(kind: .togglePlayPause) else {
            logger.debug("togglePlayPause() — guardPlaybackCommand returned nil")
            return
        }
        guard beginPlaybackCommand(.togglePlayPause) else {
            logger.debug(
                "togglePlayPause() — beginPlaybackCommand returned false (command already in flight)"
            )
            return
        }

        let previousPaused = isPaused
        let newPaused = !isPaused
        isPaused = newPaused  // Optimistic
        logger.info("Toggle play/pause: newPaused=\(newPaused)")

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
            } catch {
                self.isPaused = previousPaused
                logger.error("Failed to toggle playback: \(error.localizedDescription)")
            }
        }
    }

    func stepForward() { stepForward(by: 1) }

    func stepForward(by frameCount: UInt64) {
        logger.debug(
            "stepForward(by: \(frameCount)) called — mode=\(self.displayPlaybackMode.rawValue) frame=\(self.currentFrameIndex)/\(self.totalFrames) busy=\(self.playbackControlsBusy)"
        )
        guard frameCount > 0 else {
            logger.debug("stepForward() ignored — requested frameCount was 0")
            return
        }
        guard displayPlaybackMode == .replaySeekable else {
            logger.debug(
                "stepForward() ignored — not seekable (mode=\(self.displayPlaybackMode.rawValue))")
            return
        }
        guard totalFrames > 0 else {
            logger.debug("stepForward() ignored — totalFrames is 0")
            return
        }
        let lastFrameIndex = totalFrames - 1
        guard currentFrameIndex < lastFrameIndex else {
            logger.debug(
                "stepForward() ignored — at end (frame \(self.currentFrameIndex + 1) >= total \(self.totalFrames))"
            )
            return
        }
        guard !playbackControlsBusy else {
            logger.debug("stepForward() ignored — busy")
            return
        }
        let actualFrameCount = min(frameCount, lastFrameIndex - currentFrameIndex)
        let targetFrame = currentFrameIndex + actualFrameCount
        runStepCommand(kind: .stepForward, targetFrame: targetFrame)
    }

    func stepBackward() { stepBackward(by: 1) }

    func stepBackward(by frameCount: UInt64) {
        logger.debug(
            "stepBackward(by: \(frameCount)) called — mode=\(self.displayPlaybackMode.rawValue) frame=\(self.currentFrameIndex) busy=\(self.playbackControlsBusy)"
        )
        guard frameCount > 0 else {
            logger.debug("stepBackward() ignored — requested frameCount was 0")
            return
        }
        guard displayPlaybackMode == .replaySeekable, currentFrameIndex > 0 else {
            logger.debug("stepBackward() ignored — not seekable or already at frame 0")
            return
        }
        guard !playbackControlsBusy else {
            logger.debug("stepBackward() ignored — busy")
            return
        }
        let actualFrameCount = min(frameCount, currentFrameIndex)
        let targetFrame = currentFrameIndex - actualFrameCount
        runStepCommand(kind: .stepBackward, targetFrame: targetFrame)
    }

    private func runStepCommand(kind: PlaybackCommandKind, targetFrame: UInt64) {
        logger.debug("runStepCommand(\(String(describing: kind)), targetFrame=\(targetFrame))")
        guard let client = guardPlaybackCommand(kind: kind, requiresSeekable: true) else {
            logger.debug("runStepCommand() — guardPlaybackCommand returned nil")
            return
        }
        guard beginPlaybackCommand(kind) else {
            logger.debug("runStepCommand() — beginPlaybackCommand returned false")
            return
        }

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
                    // The server pauses at EOF and stays alive, so a seek is
                    // enough — the streaming loop already picked up the seek
                    // and delivered the stepped frame through the existing
                    // stream.  No stream restart needed.
                    self.replayFinished = false
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

    /// Cycles the clock display mode: elapsed → remaining → frames → elapsed.
    func cycleTimeDisplayMode() {
        timeDisplayMode = timeDisplayMode.next
        logger.debug("Time display mode: \(self.timeDisplayMode.rawValue)")
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

    /// Seek to an exact nanosecond timestamp.
    ///
    /// Computes progress for the UI and passes the raw timestamp through
    /// to the gRPC seek RPC — avoiding the lossy nanos→progress→nanos
    /// round-trip that `seek(to: progress)` would perform.
    func seekToTimestamp(_ timestampNanos: Int64) {
        guard hasValidTimelineRange, logEndTimestamp > logStartTimestamp else { return }
        let clamped = max(logStartTimestamp, min(logEndTimestamp, timestampNanos))
        let progress =
            Double(clamped - logStartTimestamp) / Double(logEndTimestamp - logStartTimestamp)
        logger.info(
            "seekToTimestamp(\(timestampNanos)) — clamped=\(clamped) progress=\(String(format: "%.4f", progress)) logRange=[\(self.logStartTimestamp)…\(self.logEndTimestamp)]"
        )
        seek(to: progress, rawTimestamp: clamped)
    }

    func seek(to progress: Double, rawTimestamp: Int64? = nil) {
        logger.debug(
            "seek(to: \(progress)) called — mode=\(self.displayPlaybackMode.rawValue) seekable=\(self.isSeekable) hasValidRange=\(self.hasValidTimelineRange) hasFrameProgress=\(self.hasFrameIndexProgress) busy=\(self.playbackControlsBusy) inFlight=\(String(describing: self.inFlightPlaybackCommand))"
        )
        guard displayPlaybackMode == .replaySeekable else {
            logger.debug("seek() ignored — not seekable")
            return
        }
        guard hasValidTimelineRange || hasFrameIndexProgress else {
            logger.debug(
                "seek() ignored — no valid timeline range or frame progress (start=\(self.logStartTimestamp) end=\(self.logEndTimestamp) totalFrames=\(self.totalFrames))"
            )
            return
        }

        let clampedProgress = max(0, min(1, progress))
        // Prefer frame-index seek when available — matches the frame-index
        // progress computation and avoids lossy timestamp interpolation.
        // Explicit rawTimestamp (e.g. seekToTimestamp) always uses timestamp.
        let useTimestampSeek: Bool
        if rawTimestamp != nil {
            useTimestampSeek = true
        } else if hasFrameIndexProgress {
            useTimestampSeek = false
        } else {
            useTimestampSeek = hasValidTimelineRange
        }
        let targetTimestamp: Int64 =
            rawTimestamp
            ?? (useTimestampSeek
                ? logStartTimestamp
                    + Int64(Double(logEndTimestamp - logStartTimestamp) * clampedProgress)
                : currentTimestamp)
        let targetFrame: UInt64 =
            !useTimestampSeek && totalFrames > 1
            ? UInt64(Double(totalFrames - 1) * clampedProgress) : 0

        if inFlightPlaybackCommand == .seek {
            queuedSeekProgress = clampedProgress
            replayProgress = clampedProgress
            if useTimestampSeek { currentTimestamp = targetTimestamp }
            pendingSeekTargetTimestamp = useTimestampSeek ? targetTimestamp : nil
            pendingSeekTargetFrameIndex = !useTimestampSeek ? targetFrame : nil
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
        if useTimestampSeek { currentTimestamp = targetTimestamp }
        isSeekingInProgress = true
        pendingSeekTargetTimestamp = useTimestampSeek ? targetTimestamp : nil
        pendingSeekTargetFrameIndex = !useTimestampSeek ? targetFrame : nil
        logger.info(
            "Seeking to progress \(clampedProgress) (timestamp \(targetTimestamp), frame \(targetFrame), useTimestamp=\(useTimestampSeek)), wasFinished=\(wasFinished)"
        )

        Task { @MainActor [weak self] in
            guard let self else { return }
            defer { self.finishPlaybackCommand(.seek) }
            do {
                let seekAck: VisualiserPlaybackStatus
                if useTimestampSeek {
                    seekAck = try await client.seek(to: targetTimestamp)
                } else {
                    seekAck = try await client.seek(toFrame: targetFrame)
                }
                self.applyPlaybackAck(seekAck)

                if wasFinished {
                    // The server pauses at EOF and stays alive, so a seek +
                    // play is enough — the existing stream resumes from the
                    // new position.  No stream restart needed.
                    logger.info("Replay was finished — sending play after seek")
                    self.replayFinished = false
                    self.isPaused = false
                    let playAck = try await client.play()
                    self.applyPlaybackAck(playAck)
                }
            } catch {
                self.replayProgress = previousProgress
                self.currentTimestamp = previousTimestamp
                self.isPaused = previousPaused
                self.isSeekingInProgress = false
                self.pendingSeekTargetTimestamp = nil
                self.pendingSeekTargetFrameIndex = nil
                logger.error("Failed to seek: \(error.localizedDescription)")
            }
            // On success, isSeekingInProgress stays true until the first
            // post-seek frame arrives in applyFrameStateUpdate.  This prevents
            // stale buffered frames from flickering the seek bar back to the
            // pre-seek position.
            self.seekWaitFrameCount = 0
        }
    }

    /// Called when slider editing state changes
    func setSliderEditing(_ editing: Bool) {
        logger.debug(
            "setSliderEditing(\(editing)) — busy=\(self.playbackControlsBusy) inFlight=\(String(describing: self.inFlightPlaybackCommand))"
        )
        if playbackControlsBusy && inFlightPlaybackCommand == .seek { return }
        isSeekingInProgress = editing
    }

    // MARK: - Track Navigation

    /// Select the next track in the track list order. Wraps to the first track at the end.
    func selectNextTrack() {
        let ids = trackListOrder
        guard !ids.isEmpty else { return }
        if let current = selectedTrackID, let idx = ids.firstIndex(of: current) {
            let next = ids[(idx + 1) % ids.count]
            selectTrackQuietly(next)
        } else {
            selectTrackQuietly(ids[0])
        }
    }

    /// Select the previous track in the track list order. Wraps to the last track at the start.
    func selectPreviousTrack() {
        let ids = trackListOrder
        guard !ids.isEmpty else { return }
        if let current = selectedTrackID, let idx = ids.firstIndex(of: current) {
            let prev = ids[(idx - 1 + ids.count) % ids.count]
            selectTrackQuietly(prev)
        } else {
            selectTrackQuietly(ids.last!)
        }
    }

    /// Select a track without popping open the side panel if it isn't already visible.
    private func selectTrackQuietly(_ trackID: String) {
        selectedTrackID = trackID
        selectedTrackData = allSeenTracks[trackID]
        renderer?.selectedTrackID = trackID
        reprojectLabels()
    }

    // MARK: - Labelling

    func selectTrack(_ trackID: String?) {
        selectedTrackID = trackID
        selectedTrackData = trackID.flatMap { allSeenTracks[$0] }
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
                    runBrowserState.applySuccessfulLabelUpdate(
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
                        runBrowserState.applySuccessfulLabelUpdate(
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
                runBrowserState.applySuccessfulLabelUpdate(
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

    private func restartStreamForRequestOptionChange() {
        guard !isConnecting else { return }
        guard isConnected else { return }
        logger.info("Restarting stream to apply request-scoped debug options")
        disconnect()
        connect()
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
        restartStreamForRequestOptionChange()
        sendOverlayPreferences()
    }

    /// Toggle heading-source colouring in the renderer.
    func toggleHeadingSource() {
        showHeadingSource.toggle()
        renderer?.showHeadingSource = showHeadingSource
    }

    /// Toggle whether debug cluster rendering includes track-associated clusters.
    func toggleIncludeAssociatedClusters() {
        includeAssociatedClusters.toggle()
        grpcClient?.includeAssociatedClusters = includeAssociatedClusters
        restartStreamForRequestOptionChange()
    }

    // MARK: - Frame Handling

    func onFrameReceived(_ frame: FrameBundle, generation: UInt64? = nil) {
        let eventGeneration = generation ?? playbackStateGeneration
        guard eventGeneration == playbackStateGeneration else {
            let epoch = frame.playbackInfo?.replayEpoch ?? 0
            logger.debug("Ignoring stale frame for generation \(eventGeneration) (epoch=\(epoch))")
            return
        }
        let perfStart = ContinuousClock.now
        let trace = PerformanceTrace.begin(
            "OnFrameReceived",
            "frame=\(frame.frameID) type=\(frame.frameType.rawValue) gen=\(eventGeneration)")

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
        renderer?.showVelocity = showVelocity  // Velocity/heading arrows on tracks
        renderer?.showDebug = showDebug  // M6: Debug overlay master toggle
        renderer?.showGating = showGating  // M6: Gating ellipses
        renderer?.showAssociation = showAssociation  // M6: Association lines
        renderer?.showResiduals = showResiduals  // M6: Residual vectors
        renderer?.showHeadingSource = showHeadingSource  // Debug: colour boxes by heading source
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
            newLabels = r.projectTrackLabels(viewSize: metalViewSize).map { label in
                var patched = label
                patched.userLabel = self.userLabels[label.id] ?? ""
                return patched
            }
        } else {
            newLabels = []
        }

        // Performance diagnostic: log per-frame processing cost periodically
        let perfElapsed = ContinuousClock.now - perfStart
        let perfMs =
            Double(perfElapsed.components.seconds) * 1000 + Double(
                perfElapsed.components.attoseconds) / 1e15
        if frameCount % 60 == 0 {
            logger.info(
                "[Perf] frame \(self.frameCount) processed in \(String(format: "%.1f", perfMs))ms (fps=\(String(format: "%.1f", self.fps)) type=\(frame.frameType.rawValue) points=\(frame.pointCloud?.pointCount ?? 0) tracks=\(frame.tracks?.tracks.count ?? 0))"
            )
        }

        // Defer @Published state mutations to the next run loop iteration
        // to avoid SwiftUI AttributeGraph cycles during view updates.
        //
        // When the side panel is visible, each @Published mutation triggers
        // a full SwiftUI body re-evaluation of TrackListView (sort),
        // TrackHistoryGraphView (sparkline), etc.  Throttle the deferred
        // update to ~10 fps so the main thread stays available for Metal
        // rendering and gRPC frame delivery.  The renderer.updateFrame()
        // call above is unaffected — 3D visuals stay at full speed.
        let uiNow = ContinuousClock.now
        let panelOpen = showSidePanel || selectedTrackID != nil
        let minUIInterval: ContinuousClock.Duration =
            panelOpen
            ? .milliseconds(100)  // ~10 fps UI when panel visible
            : .milliseconds(16)  // ~60 fps cap to avoid landing inside layout passes
        if uiNow - lastUIUpdateClock >= minUIInterval {
            lastUIUpdateClock = uiNow
            Task { [weak self] in
                guard let self else { return }
                self.applyFrameStateUpdate(
                    frame: frame, instantFPS: instantFPS, newCacheStatus: newCacheStatus,
                    newLabels: newLabels, generation: eventGeneration)
            }
        }

        trace.end(
            "tracks=\(frame.tracks?.tracks.count ?? 0) labels=\(newLabels.count) cache=\(newCacheStatus)"
        )
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
        let trace = PerformanceTrace.begin(
            "ApplyFrameStateUpdate",
            "frame=\(frame.frameID) gen=\(generation) labels=\(newLabels.count)")
        defer {
            trace.end(
                "fps=\(String(format: "%.1f", fps)) tracks=\(trackCount) progress=\(String(format: "%.3f", replayProgress))"
            )
        }

        currentFrameID = frame.frameID
        currentTimestamp = frame.timestampNanos
        frameCount += 1

        // Update playback info from frame
        if let playbackInfo = frame.playbackInfo {
            if !hasPlaybackMetadata { hasPlaybackMetadata = true }
            setPlaybackMode(
                inferPlaybackMode(isLive: playbackInfo.isLive, seekable: playbackInfo.seekable))
            if logStartTimestamp != playbackInfo.logStartNs {
                logStartTimestamp = playbackInfo.logStartNs
            }
            if logEndTimestamp != playbackInfo.logEndNs { logEndTimestamp = playbackInfo.logEndNs }
            if playbackRate != playbackInfo.playbackRate {
                playbackRate = playbackInfo.playbackRate
            }
            currentFrameIndex = playbackInfo.currentFrameIndex
            if totalFrames != playbackInfo.totalFrames { totalFrames = playbackInfo.totalFrames }
            // Note: isPaused is NOT updated from frame to allow optimistic UI updates.
            // The server confirms pause state via the RPC response.

            // Detect replay source change (e.g. VRLOG→PCAP transition).
            // When the epoch advances, clear accumulated track state so stale
            // history from the previous replay doesn't contaminate the new one.
            let epoch = playbackInfo.replayEpoch
            if epoch > 0 && epoch != lastSeenReplayEpoch {
                logger.info(
                    "Replay epoch changed \(lastSeenReplayEpoch) → \(epoch), clearing track state")
                lastSeenReplayEpoch = epoch
                trackHistory.removeAll()
                trackMaxSpeed.removeAll()
                trackMaxHits.removeAll()
                allSeenTracks.removeAll()
                inViewTrackIDs = []
                selectedTrackData = nil
            }

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
            // Update in-view set and accumulate all seen tracks
            inViewTrackIDs = Set(tracks.map { $0.trackID })
            for track in tracks {
                allSeenTracks[track.trackID] = track

                let sample = TrackSample(
                    frameIndex: currentFrameIndex, speedMps: track.speedMps,
                    maxSpeedMps: track.maxSpeedMps, headingDeg: track.headingRad * 180 / .pi)
                var samples = trackHistory[track.trackID] ?? []
                // Remove any samples at or beyond the current frame index
                // (handles backward seeks and steps)
                samples.removeAll { $0.frameIndex >= currentFrameIndex }
                samples.append(sample)
                if samples.count > Self.maxHistorySamples {
                    samples.removeFirst(samples.count - Self.maxHistorySamples)
                }
                trackHistory[track.trackID] = samples

                // Update persistent max speed (survives ring-buffer eviction)
                let prevMax = trackMaxSpeed[track.trackID] ?? 0
                if track.maxSpeedMps > prevMax { trackMaxSpeed[track.trackID] = track.maxSpeedMps }

                // Update persistent max hits so sort/display remains stable after a track leaves frame.
                let prevHitMax = trackMaxHits[track.trackID] ?? 0
                if track.hits > prevHitMax { trackMaxHits[track.trackID] = track.hits }
            }
        } else {
            inViewTrackIDs = []
        }

        // Update cached selected track data for inspector stability
        if let selectedTrackID { selectedTrackData = allSeenTracks[selectedTrackID] }

        // M3.5: Update cache status
        if cacheStatus != newCacheStatus { cacheStatus = newCacheStatus }

        // Update track label overlay positions
        trackLabels = newLabels

        // Log every 100 frames to show activity
        if frameCount % 100 == 1 {
            logger.info(
                "Frame \(self.frameCount): \(self.pointCount) points, \(self.trackCount) tracks, FPS: \(String(format: "%.1f", self.fps))"
            )
        }

        // Clear seeking guard when the first post-seek frame arrives.
        // The seek RPC keeps isSeekingInProgress=true so stale buffered
        // frames can't flicker the seek bar.  Once the command finishes
        // (inFlightPlaybackCommand == nil), the next matching frame clears it.
        if isSeekingInProgress && inFlightPlaybackCommand == nil {
            seekWaitFrameCount += 1
            var seekLanded = false
            if let targetFrame = pendingSeekTargetFrameIndex, totalFrames > 1 {
                let diff =
                    currentFrameIndex >= targetFrame
                    ? currentFrameIndex - targetFrame : targetFrame - currentFrameIndex
                seekLanded = diff <= 1
            } else if let targetTs = pendingSeekTargetTimestamp {
                seekLanded = abs(currentTimestamp - targetTs) < 100_000_000  // 100 ms
            } else {
                seekLanded = true  // No target known — clear immediately
            }
            if seekLanded || seekWaitFrameCount > 10 {
                isSeekingInProgress = false
                pendingSeekTargetTimestamp = nil
                pendingSeekTargetFrameIndex = nil
                seekWaitFrameCount = 0
            }
        }

        // Update replay progress (skip if user is interacting with slider).
        // Frame-index progress is preferred — it is robust against non-linear
        // timestamp distribution and background-frame timestamp contamination.
        if !isLive && !isSeekingInProgress {
            if totalFrames > 1 {
                replayProgress = max(0, min(1, Double(currentFrameIndex) / Double(totalFrames - 1)))
            } else if logEndTimestamp > logStartTimestamp {
                let progress =
                    Double(currentTimestamp - logStartTimestamp)
                    / Double(logEndTimestamp - logStartTimestamp)
                replayProgress = max(0, min(1, progress))
            }
        }

        // Detect replay completion from frame metadata.  This is more reliable
        // than waiting for gRPC stream termination, which may never propagate
        // in grpc-swift-v2's NIO transport (the `for try await` iterator can
        // hang indefinitely after the server closes the stream with OK status).
        if let playbackInfo = frame.playbackInfo, !playbackInfo.isLive,
            playbackInfo.totalFrames > 0,
            playbackInfo.currentFrameIndex + 1 >= playbackInfo.totalFrames
        {
            if !replayFinished {
                logger.info(
                    "Replay complete: frame \(playbackInfo.currentFrameIndex + 1)/\(playbackInfo.totalFrames)"
                )
                replayFinished = true
                isPaused = true
                replayProgress = 1.0
                if logEndTimestamp > 0 { currentTimestamp = logEndTimestamp }
            }
        } else if replayFinished, let playbackInfo = frame.playbackInfo, !playbackInfo.isLive,
            playbackInfo.totalFrames > 0,
            playbackInfo.currentFrameIndex + 1 < playbackInfo.totalFrames
        {
            // User seeked/stepped away from the last frame — clear finished
            // state so that pressing play resumes from the new position
            // instead of restarting from the beginning.
            logger.info(
                "Clearing replay-finished: now at frame \(playbackInfo.currentFrameIndex + 1)/\(playbackInfo.totalFrames)"
            )
            replayFinished = false
        }
    }

    func handleStreamFinished(expectedGeneration: UInt64?) {
        logger.debug(
            "handleStreamFinished(gen=\(String(describing: expectedGeneration))) — currentGen=\(self.playbackStateGeneration) frame=\(self.currentFrameIndex)/\(self.totalFrames)"
        )
        if let expectedGeneration, expectedGeneration != playbackStateGeneration {
            logger.debug("Ignoring stale finish callback for generation \(expectedGeneration)")
            return
        }
        replayFinished = true
        isPaused = true  // Pause at end
        replayProgress = 1.0
        logger.debug("Stream finished — set replayFinished=true, isPaused=true, progress=1.0")
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
            guard let appState = self?.appState else { return }
            appState.isConnected = true
            appState.connectionError = nil
            appState.replayFinished = false
            appState.hasPlaybackMetadata = false
            appState.setPlaybackMode(.unknown)
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
            guard let appState = self?.appState else { return }
            appState.isConnected = false
            appState.currentFrame = nil
            // If the replay had already finished (handleStreamFinished ran
            // synchronously before this Task), preserve the finished state
            // instead of fully resetting — the user should see "play" to
            // restart, not a broken disabled state.
            if appState.replayFinished {
                delegateLogger.info("Preserving replay-finished state after disconnect")
            } else {
                appState.resetPlaybackState(mode: .unknown)
                if error != nil { appState.connectionError = "Connection lost" }
            }
        }
    }

    func client(_ client: VisualiserClient, didReceiveFrame frame: FrameBundle) {
        let generation = self.generation
        // Called from MainActor.run in streamFrames() — call directly
        // to ensure backpressure (gRPC loop waits for processing to complete
        // before reading the next frame, preventing unbounded task queueing).
        MainActor.assumeIsolated { [weak self] in
            self?.appState?.onFrameReceived(frame, generation: generation)
        }
    }

    func clientDidFinishStream(_ client: VisualiserClient) {
        print("[ClientDelegate] 🏁 REPLAY STREAM FINISHED")
        delegateLogger.info("clientDidFinishStream called - replay reached end")
        let generation = self.generation
        // Call synchronously — we are already on MainActor (called from
        // @MainActor handleStreamTermination).  Using MainActor.assumeIsolated
        // avoids queueing a Task that could be delayed behind hundreds of
        // pending frame-delivery Tasks, or overridden by clientDidDisconnect.
        MainActor.assumeIsolated { [weak self] in
            self?.appState?.handleStreamFinished(expectedGeneration: generation)
        }
    }
}
