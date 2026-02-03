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

/// Global application state, observable by SwiftUI views.
@MainActor
class AppState: ObservableObject {

    // MARK: - Connection State

    @Published var isConnected: Bool = false
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
        if isConnected {
            disconnect()
        } else {
            connect()
        }
    }

    func connect() {
        // TODO: Implement gRPC connection
        // grpcClient = VisualiserClient(address: serverAddress)
        // grpcClient?.startStreaming(...)

        isConnected = true
        connectionError = nil
        isLive = true

        // Placeholder: simulate frame data
        startSimulation()
    }

    func disconnect() {
        grpcClient = nil
        isConnected = false
        currentFrame = nil
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

        // Update replay progress
        if !isLive && logEndTimestamp > logStartTimestamp {
            let progress =
                Double(currentTimestamp - logStartTimestamp)
                / Double(logEndTimestamp - logStartTimestamp)
            replayProgress = max(0, min(1, progress))
        }
    }

    // MARK: - Simulation (for development)

    private var simulationTimer: Timer?

    private func startSimulation() {
        simulationTimer = Timer.scheduledTimer(withTimeInterval: 0.1, repeats: true) {
            [weak self] _ in
            Task { @MainActor in
                self?.generateSimulatedFrame()
            }
        }
    }

    private func stopSimulation() {
        simulationTimer?.invalidate()
        simulationTimer = nil
    }

    private func generateSimulatedFrame() {
        // Generate synthetic frame for testing
        var frame = FrameBundle()
        frame.frameID = currentFrameID + 1
        frame.timestampNanos = Int64(Date().timeIntervalSince1970 * 1_000_000_000)
        frame.sensorID = "synthetic"

        // Generate synthetic point cloud
        var pointCloud = PointCloudFrame()
        let pointCount = 10000
        pointCloud.x = (0..<pointCount).map { _ in Float.random(in: -20...20) }
        pointCloud.y = (0..<pointCount).map { _ in Float.random(in: 0...50) }
        pointCloud.z = (0..<pointCount).map { _ in Float.random(in: 0...3) }
        pointCloud.intensity = (0..<pointCount).map { _ in UInt8.random(in: 0...255) }
        pointCloud.pointCount = pointCount
        frame.pointCloud = pointCloud

        // Generate synthetic tracks
        var trackSet = TrackSet()
        let trackCount = 5
        let time = Date().timeIntervalSince1970
        for i in 0..<trackCount {
            var track = Track()
            track.trackID = "track_\(i)"
            track.state = .confirmed
            track.x = Float(sin(time + Double(i) * 0.5) * 10)
            track.y = Float(20 + Double(i) * 5)
            track.vx = Float(cos(time + Double(i) * 0.5) * 5)
            track.vy = 0
            track.speedMps = abs(track.vx)
            track.headingRad = atan2(track.vy, track.vx)
            trackSet.tracks.append(track)
        }
        frame.tracks = trackSet

        onFrameReceived(frame)
    }
}
