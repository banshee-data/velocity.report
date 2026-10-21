// GerardusClient.swift
// gRPC client for the Gerardus alignment service (gerardus.align.v1alpha1).
//
// Gerardus (banshee-data/gerardus) prepares reference data for a survey site
// (USGS 3DEP aerial points, OpenStreetMap ways and intersections) and keeps
// an append-only log of every alignment decision. The visualiser reaches it on
// a second channel beside its channel to velocity.report; velocity.report's
// server is not in the path. The contract is vendored at
// proto/gerardus/align/v1alpha1/align.proto.
//
// The dependency is optional. `connect()` asks for the server's info first,
// and a caller that gets an error from it shows none of the alignment UI.
//
// This implementation uses grpc-swift 2.x (GRPCCore), like VisualiserClient.

import Foundation
import GRPCCore
import GRPCNIOTransportHTTP2
import GRPCProtobuf
import os

private let logger = Logger(subsystem: "report.velocity.visualiser", category: "GerardusClient")

/// Error types for the Gerardus client.
enum GerardusClientError: Error, LocalizedError, Equatable {
    case notConnected
    case invalidAddress(String)
    /// A streaming call ended without the message that closes it.
    case incompleteStream(String)

    var errorDescription: String? {
        switch self {
        case .notConnected: return "Not connected to the alignment service"
        case .invalidAddress(let message): return "Invalid address: \(message)"
        case .incompleteStream(let rpc): return "\(rpc) ended without its result"
        }
    }

    /// One line for the pane: a gRPC status reads as its code and message.
    static func describe(_ error: Error) -> String {
        if let rpc = error as? RPCError { return "\(rpc.code): \(rpc.message)" }
        return error.localizedDescription
    }
}

/// The calls the alignment session makes. Abstracted so tests can count them:
/// nudging a pose must make none.
@available(macOS 15.0, *) protocol AlignmentServiceClient: AnyObject, Sendable {
    func connect() async throws -> Gerardus_Align_V1alpha1_GetServerInfoResponse
    func disconnect()
    func prepareSite(
        l16Token: String, label: String,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_Site
    func streamReference(
        _ request: Gerardus_Align_V1alpha1_StreamReferenceRequest
    ) async throws -> ReferenceGeometry
    func poseFromHint(
        _ request: Gerardus_Align_V1alpha1_PoseFromHintRequest
    ) async throws -> Gerardus_Align_V1alpha1_PoseFromHintResponse
    func uploadScan(
        header: Gerardus_Align_V1alpha1_ScanHeader, blocks: [Gerardus_Align_V1alpha1_PointBlock]
    ) async throws -> Gerardus_Align_V1alpha1_UploadScanResponse
    func register(
        _ request: Gerardus_Align_V1alpha1_RegisterRequest,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_Registration
    func reviewRegistration(
        _ request: Gerardus_Align_V1alpha1_ReviewRegistrationRequest
    ) async throws -> Gerardus_Align_V1alpha1_RegistrationRecord
    func listRegistrationRecords(
        l16Token: String
    ) async throws -> [Gerardus_Align_V1alpha1_RegistrationRecord]
}

/// gRPC client for Gerardus's SiteService and RegistrationService.
///
/// EvaluatePose and SolveTiePoints are not called: the server answers both
/// with UNIMPLEMENTED until registration is built.
@available(macOS 15.0, *) final class GerardusClient: AlignmentServiceClient, @unchecked Sendable {

    /// Where `gerardus serve` listens unless told otherwise.
    static let defaultAddress = "127.0.0.1:50061"
    /// UserDefaults key that overrides the address, next to `annotation.serviceURL`.
    static let addressKey = "alignment.serviceAddress"

    static var configuredAddress: String {
        let configured = UserDefaults.standard.string(forKey: addressKey)?.trimmingCharacters(
            in: .whitespaces)
        if let configured, !configured.isEmpty { return configured }
        return defaultAddress
    }

    let address: String
    private let _clientTask = LockedState<Task<Void, Error>?>(nil)
    private let _grpcClient = LockedState<GRPCClient<HTTP2ClientTransport.Posix>?>(nil)

    var isConnected: Bool { _grpcClient.value != nil }

    init(address: String = GerardusClient.configuredAddress) { self.address = address }

    deinit { disconnect() }

    // MARK: - Connection

    /// Opens the channel and asks the server what it is. Throws when nothing
    /// answers within a few seconds, and leaves no channel open behind it.
    func connect() async throws -> Gerardus_Align_V1alpha1_GetServerInfoResponse {
        let (host, port) = try Self.parse(address)
        if _grpcClient.value == nil {
            // Reference chunks are about 1 MB (65,536 points); the visualiser
            // channel's limits leave room for a server that sends larger ones.
            let methodConfig = MethodConfig(
                names: [.init(service: "", method: "")], maxRequestMessageBytes: 16 * 1024 * 1024,
                maxResponseMessageBytes: 16 * 1024 * 1024)
            var config = HTTP2ClientTransport.Posix.Config.defaults
            config.http2.targetWindowSize = 16 * 1024 * 1024
            config.http2.maxFrameSize = 1 << 20
            let transport = try HTTP2ClientTransport.Posix(
                target: .dns(host: host, port: port), transportSecurity: .plaintext, config: config,
                serviceConfig: ServiceConfig(methodConfig: [methodConfig]))
            let grpcClient = GRPCClient(transport: transport)
            _grpcClient.value = grpcClient
            _clientTask.value = Task { try await grpcClient.runConnections() }
        }

        do {
            var options = CallOptions.defaults
            options.timeout = .seconds(3)
            options.waitForReady = false
            let info = try await siteService().getServerInfo(
                request: ClientRequest(message: Gerardus_Align_V1alpha1_GetServerInfoRequest()),
                options: options)
            logger.info(
                "Gerardus \(info.serverVersion) at \(self.address) speaks \(info.apiVersion)")
            return info
        } catch {
            logger.info(
                "No alignment service at \(self.address): \(GerardusClientError.describe(error))")
            disconnect()
            throw error
        }
    }

    func disconnect() {
        _grpcClient.value?.beginGracefulShutdown()
        _clientTask.value?.cancel()
        _clientTask.value = nil
        _grpcClient.value = nil
    }

    static func parse(_ address: String) throws -> (host: String, port: Int) {
        let components = address.split(separator: ":")
        guard components.count == 2, let port = Int(components[1]), !components[0].isEmpty else {
            throw GerardusClientError.invalidAddress(
                "Invalid format: \(address), expected host:port")
        }
        return (String(components[0]), port)
    }

    private func siteService() throws
        -> Gerardus_Align_V1alpha1_SiteService.Client<HTTP2ClientTransport.Posix>
    {
        guard let grpcClient = _grpcClient.value else { throw GerardusClientError.notConnected }
        return Gerardus_Align_V1alpha1_SiteService.Client(wrapping: grpcClient)
    }

    private func registrationService() throws
        -> Gerardus_Align_V1alpha1_RegistrationService.Client<HTTP2ClientTransport.Posix>
    {
        guard let grpcClient = _grpcClient.value else { throw GerardusClientError.notConnected }
        return Gerardus_Align_V1alpha1_RegistrationService.Client(wrapping: grpcClient)
    }

    // MARK: - SiteService

    /// Builds the site's reference bundle, or finds it cached, reporting
    /// progress as it goes. Returns the site from the stream's last message.
    func prepareSite(
        l16Token: String, label: String,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_Site {
        var request = Gerardus_Align_V1alpha1_PrepareSiteRequest()
        request.l16Token = l16Token
        request.label = label
        return try await siteService().prepareSite(request: ClientRequest(message: request)) {
            response in
            var site: Gerardus_Align_V1alpha1_Site?
            for try await message in response.messages {
                switch message.event {
                case .progress(let progress): await onProgress(progress)
                case .site(let prepared): site = prepared
                case nil: break
                }
            }
            guard let site else { throw GerardusClientError.incompleteStream("PrepareSite") }
            return site
        }
    }

    /// Streams reference geometry in the site frame and gathers it, layer by
    /// layer, into one value.
    func streamReference(
        _ request: Gerardus_Align_V1alpha1_StreamReferenceRequest
    ) async throws -> ReferenceGeometry {
        try await siteService().streamReference(request: ClientRequest(message: request)) {
            response in
            var geometry = ReferenceGeometry()
            for try await message in response.messages { geometry.append(message) }
            return geometry
        }
    }

    // MARK: - RegistrationService

    func poseFromHint(
        _ request: Gerardus_Align_V1alpha1_PoseFromHintRequest
    ) async throws -> Gerardus_Align_V1alpha1_PoseFromHintResponse {
        try await registrationService().poseFromHint(request: ClientRequest(message: request))
    }

    /// Uploads a scan: the header first, then each block, on one client stream.
    func uploadScan(
        header: Gerardus_Align_V1alpha1_ScanHeader, blocks: [Gerardus_Align_V1alpha1_PointBlock]
    ) async throws -> Gerardus_Align_V1alpha1_UploadScanResponse {
        try await registrationService().uploadScan(requestProducer: { writer in
            var first = Gerardus_Align_V1alpha1_UploadScanRequest()
            first.header = header
            try await writer.write(first)
            for block in blocks {
                var part = Gerardus_Align_V1alpha1_UploadScanRequest()
                part.points = block
                try await writer.write(part)
            }
        })
    }

    /// Runs registration for an uploaded scan. Until the automatic stages are
    /// built the server records every run as FAILED, carrying its initial pose.
    func register(
        _ request: Gerardus_Align_V1alpha1_RegisterRequest,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_Registration {
        try await registrationService().register(request: ClientRequest(message: request)) {
            response in
            var registration: Gerardus_Align_V1alpha1_Registration?
            for try await message in response.messages {
                switch message.event {
                case .progress(let progress): await onProgress(progress)
                case .registration(let result): registration = result
                // Hypotheses arrive again, ranked, on the registration.
                case .hypothesis, nil: break
                }
            }
            guard let registration else { throw GerardusClientError.incompleteStream("Register") }
            return registration
        }
    }

    func reviewRegistration(
        _ request: Gerardus_Align_V1alpha1_ReviewRegistrationRequest
    ) async throws -> Gerardus_Align_V1alpha1_RegistrationRecord {
        try await registrationService().reviewRegistration(request: ClientRequest(message: request))
            .record
    }

    /// A site's records, newest first.
    func listRegistrationRecords(
        l16Token: String
    ) async throws -> [Gerardus_Align_V1alpha1_RegistrationRecord] {
        var request = Gerardus_Align_V1alpha1_ListRegistrationRecordsRequest()
        request.l16Token = l16Token
        return try await registrationService().listRegistrationRecords(
            request: ClientRequest(message: request)
        ).records
    }
}
