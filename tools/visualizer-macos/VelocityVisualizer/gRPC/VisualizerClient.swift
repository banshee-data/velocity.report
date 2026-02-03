// VisualizerClient.swift
// gRPC client for streaming frame data from the Go pipeline.
//
// This is a placeholder implementation. The actual gRPC client will use
// grpc-swift and the generated protobuf stubs.

import Foundation
import Combine

/// Protocol for receiving frame data from the gRPC stream.
protocol VisualizerClientDelegate: AnyObject {
    func clientDidConnect(_ client: VisualizerClient)
    func clientDidDisconnect(_ client: VisualizerClient, error: Error?)
    func client(_ client: VisualizerClient, didReceiveFrame frame: FrameBundle)
}

/// gRPC client for the VisualizerService.
class VisualizerClient {
    
    // MARK: - Properties
    
    let address: String
    weak var delegate: VisualizerClientDelegate?
    
    private(set) var isConnected: Bool = false
    
    // Stream request configuration
    var includePoints: Bool = true
    var includeClusters: Bool = true
    var includeTracks: Bool = true
    var includeDebug: Bool = false
    var decimationMode: DecimationMode = .none
    var decimationRatio: Float = 1.0
    
    // TODO: Replace with actual gRPC channel and call objects
    // private var channel: GRPCChannel?
    // private var streamCall: ServerStreamingCall<...>?
    
    private var cancellables = Set<AnyCancellable>()
    
    // MARK: - Initialisation
    
    init(address: String) {
        self.address = address
    }
    
    deinit {
        disconnect()
    }
    
    // MARK: - Connection
    
    /// Connect to the gRPC server and start streaming.
    func connect() async throws {
        guard !isConnected else { return }
        
        // TODO: Implement actual gRPC connection
        // Example with grpc-swift:
        //
        // let group = PlatformSupport.makeEventLoopGroup(loopCount: 1)
        // let channel = try GRPCChannelPool.with(
        //     target: .host(host, port: port),
        //     transportSecurity: .plaintext,
        //     eventLoopGroup: group
        // )
        // let client = Velocity_Visualizer_V1_VisualizerServiceAsyncClient(channel: channel)
        //
        // let request = Velocity_Visualizer_V1_StreamRequest()
        // request.sensorID = "all"
        // request.includePoints = includePoints
        // ...
        //
        // for try await frame in client.streamFrames(request) {
        //     let bundle = decodeFrameBundle(frame)
        //     delegate?.client(self, didReceiveFrame: bundle)
        // }
        
        isConnected = true
        await MainActor.run {
            delegate?.clientDidConnect(self)
        }
    }
    
    /// Disconnect from the gRPC server.
    func disconnect() {
        guard isConnected else { return }
        
        // TODO: Close gRPC channel
        // channel?.close()
        
        isConnected = false
        delegate?.clientDidDisconnect(self, error: nil)
    }
    
    // MARK: - Playback Control
    
    /// Pause playback (replay mode only).
    func pause() async throws {
        // TODO: Send Pause RPC
        // let response = try await client.pause(.init())
    }
    
    /// Resume playback (replay mode only).
    func play() async throws {
        // TODO: Send Play RPC
    }
    
    /// Seek to a timestamp (replay mode only).
    func seek(to timestampNanos: Int64) async throws {
        // TODO: Send Seek RPC
    }
    
    /// Seek to a frame ID (replay mode only).
    func seek(toFrame frameID: UInt64) async throws {
        // TODO: Send Seek RPC
    }
    
    /// Set playback rate (replay mode only).
    func setRate(_ rate: Float) async throws {
        // TODO: Send SetRate RPC
    }
    
    // MARK: - Overlay Modes
    
    /// Update which overlays the server should emit.
    func setOverlayModes(
        showPoints: Bool,
        showClusters: Bool,
        showTracks: Bool,
        showTrails: Bool,
        showVelocity: Bool,
        showGating: Bool,
        showAssociation: Bool,
        showResiduals: Bool
    ) async throws {
        // TODO: Send SetOverlayModes RPC
    }
    
    // MARK: - Capabilities
    
    /// Query server capabilities.
    func getCapabilities() async throws -> ServerCapabilities {
        // TODO: Send GetCapabilities RPC
        return ServerCapabilities(
            supportsPoints: true,
            supportsClusters: true,
            supportsTracks: true,
            supportsDebug: true,
            supportsReplay: false,
            supportsRecording: false,
            availableSensors: ["hesai-01"]
        )
    }
    
    // MARK: - Recording
    
    /// Start recording frames on the server.
    func startRecording(outputPath: String? = nil) async throws -> RecordingStatus {
        // TODO: Send StartRecording RPC
        return RecordingStatus(recording: true, outputPath: "/tmp/recording.vrlog", framesRecorded: 0)
    }
    
    /// Stop recording frames on the server.
    func stopRecording() async throws -> RecordingStatus {
        // TODO: Send StopRecording RPC
        return RecordingStatus(recording: false, outputPath: "", framesRecorded: 0)
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

// MARK: - Frame Decoding

extension VisualizerClient {
    
    /// Decode a protobuf FrameBundle to the Swift model.
    /// TODO: Replace with actual protobuf decoding when generated.
    func decodeFrameBundle(_ protoFrame: Any) -> FrameBundle {
        // Placeholder implementation
        var frame = FrameBundle()
        // frame = ... decode from proto ...
        return frame
    }
}
