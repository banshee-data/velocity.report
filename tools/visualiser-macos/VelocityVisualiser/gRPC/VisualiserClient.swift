// VisualiserClient.swift
// gRPC client for streaming frame data from the Go pipeline.
//
// This implementation uses grpc-swift 2.x (GRPCCore) for async streaming.

import Combine
import Foundation
import GRPCCore
import GRPCNIOTransportHTTP2
import GRPCProtobuf
import Network
import os

private let logger = Logger(subsystem: "report.velocity.visualiser", category: "VisualiserClient")

/// Protocol for receiving frame data from the gRPC stream.
protocol VisualiserClientDelegate: AnyObject {
    func clientDidConnect(_ client: VisualiserClient)
    func clientDidDisconnect(_ client: VisualiserClient, error: Error?)
    func client(_ client: VisualiserClient, didReceiveFrame frame: FrameBundle)
    func clientDidFinishStream(_ client: VisualiserClient)  // Called when replay stream ends (EOF)
}

/// Error types for the visualiser client.
enum VisualiserClientError: Error, LocalizedError {
    case notConnected
    case connectionFailed(String)
    case streamError(String)
    case invalidAddress(String)

    var errorDescription: String? {
        switch self {
        case .notConnected: return "Not connected to server"
        case .connectionFailed(let message): return "Connection failed: \(message)"
        case .streamError(let message): return "Stream error: \(message)"
        case .invalidAddress(let message): return "Invalid address: \(message)"
        }
    }
}

/// gRPC client for the VisualiserService.
///
/// Usage:
/// ```swift
/// let client = VisualiserClient(address: "localhost:50051")
/// client.delegate = self
/// try await client.connect()
/// // Frames will be delivered via delegate
/// client.disconnect()
/// ```
@available(macOS 15.0, *) final class VisualiserClient {

    // MARK: - Properties

    let address: String
    private let _isConnected = LockedState(false)
    private let _streamTask = LockedState<Task<Void, Never>?>(nil)
    private let _clientTask = LockedState<Task<Void, Error>?>(nil)
    private let _grpcClient = LockedState<GRPCClient<HTTP2ClientTransport.Posix>?>(nil)

    weak var delegate: VisualiserClientDelegate?

    var isConnected: Bool { _isConnected.value }

    // Stream request configuration
    var includePoints: Bool = true
    var includeClusters: Bool = true
    var includeTracks: Bool = true
    var includeDebug: Bool = false
    var decimationMode: Velocity_Visualiser_V1_DecimationMode = .decimationNone
    var decimationRatio: Float = 1.0

    // MARK: - Initialisation

    init(address: String) { self.address = address }

    deinit { disconnect() }

    // MARK: - Connection

    /// Connect to the gRPC server and start streaming frames.
    func connect() async throws {
        print("[VisualiserClient] 🔌 connect() called, address: \(address)")
        logger.info("connect() called, address: \(self.address)")

        guard !isConnected else {
            logger.debug("Already connected, skipping")
            return
        }

        // Parse address
        let components = address.split(separator: ":")
        guard components.count == 2, let port = Int(components[1]) else {
            logger.error("Invalid address format: \(self.address)")
            throw VisualiserClientError.invalidAddress(
                "Invalid format: \(address), expected host:port")
        }
        let host = String(components[0])
        logger.debug("Parsed host: \(host), port: \(port)")

        // Create gRPC transport and client
        do {
            print("[VisualiserClient] Creating gRPC transport to \(host):\(port)...")

            // Configure max message size for large point clouds (64k+ points).
            // Default 4MB is insufficient; use 16MB to handle full-resolution frames.
            let methodConfig = MethodConfig(
                names: [.init(service: "", method: "")],  // Empty = default for all methods
                maxRequestMessageBytes: 16 * 1024 * 1024,  // 16 MB
                maxResponseMessageBytes: 16 * 1024 * 1024  // 16 MB
            )
            let serviceConfig = ServiceConfig(methodConfig: [methodConfig])

            let transport = try HTTP2ClientTransport.Posix(
                target: .dns(host: host, port: port), transportSecurity: .plaintext,
                serviceConfig: serviceConfig)

            let grpcClient = GRPCClient(transport: transport)
            _grpcClient.value = grpcClient

            // Start the client in a background task
            let clientTask = Task { try await grpcClient.runConnections() }
            _clientTask.value = clientTask

            // Give the client a moment to establish connection
            try await Task.sleep(for: .milliseconds(100))

            _isConnected.value = true
            print("[VisualiserClient] ✅ gRPC client created and running")
            logger.info("gRPC client created and running")

            await MainActor.run { self.delegate?.clientDidConnect(self) }
            print("[VisualiserClient] 🚀 Starting gRPC stream...")
            logger.debug("Delegate notified, starting streaming task...")

            // Start streaming frames from the server
            startStreamingTask()

        } catch {
            print("[VisualiserClient] ❌ Failed to create gRPC transport: \(error)")
            logger.error("Failed to create gRPC transport: \(error.localizedDescription)")
            throw VisualiserClientError.connectionFailed(error.localizedDescription)
        }
    }

    /// Disconnect from the gRPC server.
    func disconnect() {
        guard isConnected else { return }
        print("[VisualiserClient] 🔌 Disconnecting...")
        logger.info("disconnect() called")

        // Cancel streaming task
        _streamTask.value?.cancel()
        _streamTask.value = nil

        // Close gRPC client
        _grpcClient.value?.beginGracefulShutdown()
        _clientTask.value?.cancel()
        _clientTask.value = nil
        _grpcClient.value = nil

        _isConnected.value = false
        delegate?.clientDidDisconnect(self, error: nil)
        print("[VisualiserClient] Disconnected")
    }

    /// Restart the frame stream (used after seek when replay has finished).
    func restartStream() {
        guard isConnected else { return }
        print("[VisualiserClient] Restarting stream...")
        _streamTask.value?.cancel()
        startStreamingTask()
    }

    // MARK: - Streaming

    private func startStreamingTask() {
        let task = Task { [weak self] in
            guard let self = self else { return }

            do { try await self.streamFrames() } catch {
                if !Task.isCancelled {
                    print("[VisualiserClient] ❌ Stream error: \(error)")
                    logger.error("Stream error: \(error.localizedDescription)")
                    await MainActor.run { self.delegate?.clientDidDisconnect(self, error: error) }
                }
            }
        }
        _streamTask.value = task
    }

    private func streamFrames() async throws {
        guard let grpcClient = _grpcClient.value else { throw VisualiserClientError.notConnected }

        // Create service client
        let serviceClient = Velocity_Visualiser_V1_VisualiserService.Client(wrapping: grpcClient)

        // Build stream request
        var request = Velocity_Visualiser_V1_StreamRequest()
        request.sensorID = "all"
        request.includePoints = includePoints
        request.includeClusters = includeClusters
        request.includeTracks = includeTracks
        request.includeDebug = includeDebug
        request.pointDecimation = decimationMode
        request.decimationRatio = decimationRatio

        print("[VisualiserClient] 📡 Starting StreamFrames RPC...")
        logger.info("Starting StreamFrames RPC with request: sensor=\(request.sensorID)")

        // Call the streaming RPC
        // Capture self weakly for use in the sendable closure
        try await serviceClient.streamFrames(
            request: ClientRequest(message: request),
            onResponse: {
                [weak self] (response: StreamingClientResponse<Velocity_Visualiser_V1_FrameBundle>)
                in
                print("[VisualiserClient] 📥 Received stream response")

                switch response.accepted {
                case .success(let contents):
                    print("[VisualiserClient] ✅ Stream accepted, metadata: \(contents.metadata)")

                    var frameCount: UInt64 = 0
                    for try await protoFrame in response.messages {
                        frameCount += 1

                        // Log every 100 frames
                        if frameCount % 100 == 1 {
                            print(
                                "[VisualiserClient] 📊 Received frame \(frameCount): id=\(protoFrame.frameID), points=\(protoFrame.pointCloud.pointCount)"
                            )
                        }

                        // Decode proto to internal model off the main actor,
                        // then hop to MainActor only to notify the delegate.
                        guard let strongSelf = self else { continue }
                        let frame = strongSelf.decodeFrameBundle(protoFrame)

                        await MainActor.run { [weak strongSelf] in
                            guard let self = strongSelf else { return }
                            self.delegate?.client(self, didReceiveFrame: frame)
                        }
                    }
                    print("[VisualiserClient] Stream ended after \(frameCount) frames")

                    // Notify delegate that stream finished (replay complete)
                    await MainActor.run { [weak self] in
                        guard let self = self else { return }
                        self.delegate?.clientDidFinishStream(self)
                    }

                case .failure(let error):
                    print("[VisualiserClient] ❌ Stream rejected: \(error)")
                    throw VisualiserClientError.streamError(String(describing: error))
                }
            })
    }

    // MARK: - Playback Control

    /// Pause playback (replay mode only).
    func pause() async throws {
        guard isConnected, let grpcClient = _grpcClient.value else {
            throw VisualiserClientError.notConnected
        }
        let serviceClient = Velocity_Visualiser_V1_VisualiserService.Client(wrapping: grpcClient)
        let request = Velocity_Visualiser_V1_PauseRequest()
        _ = try await serviceClient.pause(request: ClientRequest(message: request))
    }

    /// Resume playback (replay mode only).
    func play() async throws {
        guard isConnected, let grpcClient = _grpcClient.value else {
            throw VisualiserClientError.notConnected
        }
        let serviceClient = Velocity_Visualiser_V1_VisualiserService.Client(wrapping: grpcClient)
        let request = Velocity_Visualiser_V1_PlayRequest()
        _ = try await serviceClient.play(request: ClientRequest(message: request))
    }

    /// Seek to a timestamp (replay mode only).
    func seek(to timestampNanos: Int64) async throws {
        guard isConnected, let grpcClient = _grpcClient.value else {
            throw VisualiserClientError.notConnected
        }
        let serviceClient = Velocity_Visualiser_V1_VisualiserService.Client(wrapping: grpcClient)
        var request = Velocity_Visualiser_V1_SeekRequest()
        request.timestampNs = timestampNanos
        _ = try await serviceClient.seek(request: ClientRequest(message: request))
    }

    /// Seek to a frame ID (replay mode only).
    func seek(toFrame frameID: UInt64) async throws {
        guard isConnected, let grpcClient = _grpcClient.value else {
            throw VisualiserClientError.notConnected
        }
        let serviceClient = Velocity_Visualiser_V1_VisualiserService.Client(wrapping: grpcClient)
        var request = Velocity_Visualiser_V1_SeekRequest()
        request.frameID = frameID
        _ = try await serviceClient.seek(request: ClientRequest(message: request))
    }

    /// Set playback rate (replay mode only).
    func setRate(_ rate: Float) async throws {
        guard isConnected, let grpcClient = _grpcClient.value else {
            throw VisualiserClientError.notConnected
        }
        let serviceClient = Velocity_Visualiser_V1_VisualiserService.Client(wrapping: grpcClient)
        var request = Velocity_Visualiser_V1_SetRateRequest()
        request.rate = rate
        _ = try await serviceClient.setRate(request: ClientRequest(message: request))
    }

    /// Send overlay mode preferences to the server.
    func setOverlayModes(
        showPoints: Bool, showClusters: Bool, showTracks: Bool, showTrails: Bool,
        showVelocity: Bool, showGating: Bool, showAssociation: Bool, showResiduals: Bool
    ) async throws {
        guard isConnected, let grpcClient = _grpcClient.value else {
            throw VisualiserClientError.notConnected
        }
        let serviceClient = Velocity_Visualiser_V1_VisualiserService.Client(wrapping: grpcClient)
        var request = Velocity_Visualiser_V1_OverlayModeRequest()
        request.showPoints = showPoints
        request.showClusters = showClusters
        request.showTracks = showTracks
        request.showTrails = showTrails
        request.showVelocity = showVelocity
        request.showGating = showGating
        request.showAssociation = showAssociation
        request.showResiduals = showResiduals
        _ = try await serviceClient.setOverlayModes(request: ClientRequest(message: request))
    }

    // MARK: - Capabilities

    /// Query server capabilities.
    func getCapabilities() async throws -> ServerCapabilities {
        guard isConnected, let grpcClient = _grpcClient.value else {
            throw VisualiserClientError.notConnected
        }
        let serviceClient = Velocity_Visualiser_V1_VisualiserService.Client(wrapping: grpcClient)
        let request = Velocity_Visualiser_V1_CapabilitiesRequest()
        let response = try await serviceClient.getCapabilities(
            request: ClientRequest(message: request))

        return ServerCapabilities(
            supportsPoints: response.supportsPoints, supportsClusters: response.supportsClusters,
            supportsTracks: response.supportsTracks, supportsDebug: response.supportsDebug,
            supportsReplay: response.supportsReplay, supportsRecording: response.supportsRecording,
            availableSensors: response.availableSensors)
    }

    // MARK: - Recording

    /// Start recording frames on the server.
    func startRecording(outputPath: String? = nil) async throws -> RecordingStatus {
        guard isConnected, let grpcClient = _grpcClient.value else {
            throw VisualiserClientError.notConnected
        }
        let serviceClient = Velocity_Visualiser_V1_VisualiserService.Client(wrapping: grpcClient)
        var request = Velocity_Visualiser_V1_RecordingRequest()
        if let path = outputPath { request.outputPath = path }
        let response = try await serviceClient.startRecording(
            request: ClientRequest(message: request))

        return RecordingStatus(
            recording: response.recording, outputPath: response.outputPath,
            framesRecorded: response.framesRecorded)
    }

    /// Stop recording frames on the server.
    func stopRecording() async throws -> RecordingStatus {
        guard isConnected, let grpcClient = _grpcClient.value else {
            throw VisualiserClientError.notConnected
        }
        let serviceClient = Velocity_Visualiser_V1_VisualiserService.Client(wrapping: grpcClient)
        let request = Velocity_Visualiser_V1_RecordingRequest()
        let response = try await serviceClient.stopRecording(
            request: ClientRequest(message: request))

        return RecordingStatus(
            recording: response.recording, outputPath: response.outputPath,
            framesRecorded: response.framesRecorded)
    }
}

// MARK: - Supporting Types

struct ServerCapabilities {
    var supportsPoints: Bool = false
    var supportsClusters: Bool = false
    var supportsTracks: Bool = false
    var supportsDebug: Bool = false
    var supportsReplay: Bool = false
    var supportsRecording: Bool = false
    var availableSensors: [String] = []
}

struct RecordingStatus {
    var recording: Bool = false
    var outputPath: String = ""
    var framesRecorded: UInt64 = 0
}

// MARK: - Locked State Helper

/// Thread-safe wrapper for mutable state.
final class LockedState<Value>: @unchecked Sendable {
    private var _value: Value
    private let lock = NSLock()

    init(_ value: Value) { _value = value }

    var value: Value {
        get {
            lock.lock()
            defer { lock.unlock() }
            return _value
        }
        set {
            lock.lock()
            defer { lock.unlock() }
            _value = newValue
        }
    }
}

// MARK: - Frame Decoding

@available(macOS 15.0, *) extension VisualiserClient {

    /// Decode a protobuf FrameBundle to the Swift model.
    func decodeFrameBundle(_ proto: Velocity_Visualiser_V1_FrameBundle) -> FrameBundle {
        var frame = FrameBundle()
        frame.frameID = proto.frameID
        frame.timestampNanos = proto.timestampNs
        frame.sensorID = proto.sensorID

        // Coordinate frame
        if proto.hasCoordinateFrame {
            frame.coordinateFrame = CoordinateFrameInfo(
                frameID: proto.coordinateFrame.frameID,
                referenceFrame: proto.coordinateFrame.referenceFrame,
                originLat: proto.coordinateFrame.originLat,
                originLon: proto.coordinateFrame.originLon,
                originAlt: proto.coordinateFrame.originAlt,
                rotationDeg: proto.coordinateFrame.rotationDeg)
        }

        // Point cloud
        if proto.hasPointCloud {
            let pc = proto.pointCloud
            frame.pointCloud = PointCloudFrame(
                frameID: pc.frameID, timestampNanos: pc.timestampNs, sensorID: pc.sensorID, x: pc.x,
                y: pc.y, z: pc.z, intensity: pc.intensity.map { UInt8($0) },
                classification: pc.classification.map { UInt8($0) },
                decimationMode: DecimationMode(rawValue: Int(pc.decimationMode.rawValue)) ?? .none,
                decimationRatio: pc.decimationRatio, pointCount: Int(pc.pointCount))
        }

        // Clusters
        if proto.hasClusters {
            frame.clusters = ClusterSet(
                frameID: proto.clusters.frameID, timestampNanos: proto.clusters.timestampNs,
                clusters: proto.clusters.clusters.map { c in
                    Cluster(
                        clusterID: c.clusterID, sensorID: c.sensorID, timestampNanos: c.timestampNs,
                        centroidX: c.centroidX, centroidY: c.centroidY, centroidZ: c.centroidZ,
                        aabbLength: c.aabbLength, aabbWidth: c.aabbWidth, aabbHeight: c.aabbHeight,
                        obb: c.hasObb
                            ? OrientedBoundingBox(
                                centerX: c.obb.centerX, centerY: c.obb.centerY,
                                centerZ: c.obb.centerZ, length: c.obb.length, width: c.obb.width,
                                height: c.obb.height, headingRad: c.obb.headingRad) : nil,
                        pointsCount: Int(c.pointsCount), heightP95: c.heightP95,
                        intensityMean: c.intensityMean, samplePoints: c.samplePoints)
                },
                method: ClusteringMethod(rawValue: Int(proto.clusters.method.rawValue)) ?? .dbscan)
        }

        // Tracks
        if proto.hasTracks {
            frame.tracks = TrackSet(
                frameID: proto.tracks.frameID, timestampNanos: proto.tracks.timestampNs,
                tracks: proto.tracks.tracks.map { t in
                    Track(
                        trackID: t.trackID, sensorID: t.sensorID,
                        state: TrackState(rawValue: Int(t.state.rawValue)) ?? .unknown,
                        hits: Int(t.hits), misses: Int(t.misses),
                        observationCount: Int(t.observationCount), firstSeenNanos: t.firstSeenNs,
                        lastSeenNanos: t.lastSeenNs, x: t.x, y: t.y, z: t.z, vx: t.vx, vy: t.vy,
                        vz: t.vz, speedMps: t.speedMps, headingRad: t.headingRad,
                        covariance4x4: t.covariance4X4, bboxLengthAvg: t.bboxLengthAvg,
                        bboxWidthAvg: t.bboxWidthAvg, bboxHeightAvg: t.bboxHeightAvg,
                        bboxHeadingRad: t.bboxHeadingRad, heightP95Max: t.heightP95Max,
                        intensityMeanAvg: t.intensityMeanAvg, avgSpeedMps: t.avgSpeedMps,
                        peakSpeedMps: t.peakSpeedMps, classLabel: t.classLabel,
                        classConfidence: t.classConfidence, trackLengthMetres: t.trackLengthMetres,
                        trackDurationSecs: t.trackDurationSecs,
                        occlusionCount: Int(t.occlusionCount), confidence: t.confidence,
                        occlusionState: OcclusionState(rawValue: Int(t.occlusionState.rawValue))
                            ?? .none,
                        motionModel: MotionModel(rawValue: Int(t.motionModel.rawValue)) ?? .cv,
                        alpha: t.alpha > 0 ? t.alpha : 1.0)
                },
                trails: proto.tracks.trails.map { trail in
                    TrackTrail(
                        trackID: trail.trackID,
                        points: trail.points.map { p in
                            TrackPoint(x: p.x, y: p.y, timestampNanos: p.timestampNs)
                        })
                })
        }

        // Playback info
        if proto.hasPlaybackInfo {
            frame.playbackInfo = PlaybackInfo(
                isLive: proto.playbackInfo.isLive, logStartNs: proto.playbackInfo.logStartNs,
                logEndNs: proto.playbackInfo.logEndNs,
                playbackRate: proto.playbackInfo.playbackRate, paused: proto.playbackInfo.paused,
                currentFrameIndex: proto.playbackInfo.currentFrameIndex,
                totalFrames: proto.playbackInfo.totalFrames)
        }

        // Debug overlays
        if proto.hasDebug {
            let d = proto.debug
            frame.debug = DebugOverlaySet(
                frameID: d.frameID, timestampNanos: d.timestampNs,
                associationCandidates: d.associationCandidates.map { a in
                    AssociationCandidate(
                        clusterID: a.clusterID, trackID: a.trackID, distance: a.distance,
                        accepted: a.accepted)
                },
                gatingEllipses: d.gatingEllipses.map { g in
                    GatingEllipse(
                        trackID: g.trackID, centerX: g.centerX, centerY: g.centerY,
                        semiMajor: g.semiMajor, semiMinor: g.semiMinor, rotationRad: g.rotationRad)
                },
                residuals: d.residuals.map { r in
                    InnovationResidual(
                        trackID: r.trackID, predictedX: r.predictedX, predictedY: r.predictedY,
                        measuredX: r.measuredX, measuredY: r.measuredY,
                        residualMagnitude: r.residualMagnitude)
                },
                predictions: d.predictions.map { p in
                    StatePrediction(trackID: p.trackID, x: p.x, y: p.y, vx: p.vx, vy: p.vy)
                })
        }

        // M3.5 Split Streaming fields
        frame.frameType = FrameType(rawValue: Int(proto.frameType.rawValue)) ?? .full
        frame.backgroundSeq = proto.backgroundSeq

        if proto.hasBackground {
            let bg = proto.background
            var snapshot = BackgroundSnapshot()
            snapshot.sequenceNumber = bg.sequenceNumber
            snapshot.timestampNanos = bg.timestampNanos
            snapshot.x = bg.x
            snapshot.y = bg.y
            snapshot.z = bg.z
            snapshot.confidence = bg.confidence
            if bg.hasGridMetadata {
                snapshot.gridMetadata = GridMetadata(
                    rings: bg.gridMetadata.rings, azimuthBins: bg.gridMetadata.azimuthBins,
                    ringElevations: bg.gridMetadata.ringElevations,
                    settlingComplete: bg.gridMetadata.settlingComplete)
            }
            frame.background = snapshot
        }

        return frame
    }
}
