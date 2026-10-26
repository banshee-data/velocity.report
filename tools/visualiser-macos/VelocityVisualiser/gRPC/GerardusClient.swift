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
    /// A streaming call ended without the message that closes it.
    case incompleteStream(String)
    /// The server refused the request (PERMISSION_DENIED). Gerardus answers so
    /// when REGISTERED_SCANS is asked for by a peer outside loopback and the
    /// tailnet (site model spike §8).
    case permissionDenied(String)

    var errorDescription: String? {
        switch self {
        case .notConnected: return "Not connected to the alignment service"
        case .incompleteStream(let rpc): return "\(rpc) ended without its result"
        case .permissionDenied(let message):
            return message.isEmpty ? "Permission denied" : "Permission denied: \(message)"
        }
    }

    /// One line for the pane: a gRPC status reads as its code and message.
    static func describe(_ error: Error) -> String {
        if let rpc = error as? RPCError { return "\(rpc.code): \(rpc.message)" }
        return error.localizedDescription
    }

    /// These statuses refuse the mutation. Transport/internal failures have
    /// an unknown outcome and must retain the original request identity.
    static func isDefiniteRejection(_ error: Error) -> Bool {
        guard let rpc = error as? RPCError else { return false }
        switch rpc.code {
        case .invalidArgument, .notFound, .alreadyExists, .permissionDenied, .unauthenticated,
            .failedPrecondition, .unimplemented, .outOfRange:
            return true
        default: return false
        }
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
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void,
        onHypothesis: @escaping @Sendable (Gerardus_Align_V1alpha1_Hypothesis) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_Registration
    func evaluatePose(
        _ request: Gerardus_Align_V1alpha1_EvaluatePoseRequest
    ) async throws -> Gerardus_Align_V1alpha1_EvaluatePoseResponse
    func reviewRegistration(
        _ request: Gerardus_Align_V1alpha1_ReviewRegistrationRequest
    ) async throws -> Gerardus_Align_V1alpha1_RegistrationRecord
    func reviewRegistrations(
        _ request: Gerardus_Align_V1alpha1_ReviewRegistrationsRequest
    ) async throws -> [Gerardus_Align_V1alpha1_RegistrationRecord]
    func listRegistrationRecords(
        l16Token: String
    ) async throws -> [Gerardus_Align_V1alpha1_RegistrationRecord]
    /// The site model: the site's accepted scans and the model_id the
    /// registered layers are built from. Nil while the server answers
    /// UNIMPLEMENTED, as Gerardus does until it builds site models.
    func getSiteModel(
        l16Token: String
    ) async throws -> Gerardus_Align_V1alpha1_GetSiteModelResponse?
    /// A logged run: for a refinement, the gates its scan's j1 was judged by.
    func getRegistration(id: String) async throws -> Gerardus_Align_V1alpha1_Registration
    /// Re-solves a site's accepted poses together. Writes one run per scan;
    /// nothing is recorded until an operator accepts each run's j1.
    func refineSite(
        _ request: Gerardus_Align_V1alpha1_RefineSiteRequest,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_RefineSiteResult
    /// The site's marked street use (STREET_USE): each feature's current
    /// version, with its id, author and row.
    func listStreetUse(l16Token: String) async throws -> [Gerardus_Align_V1alpha1_StreetUseFeature]
    /// Adds, changes or removes marked features in one transaction, all or
    /// nothing. Gerardus snaps and drapes every vertex, and answers with every
    /// current feature and its problems ("<feature id>: message").
    func editStreetUse(
        _ request: Gerardus_Align_V1alpha1_EditStreetUseRequest
    ) async throws -> Gerardus_Align_V1alpha1_EditStreetUseResponse
}

/// A client that has no street use answers as a server from before it would.
@available(macOS 15.0, *) extension AlignmentServiceClient {
    func reviewRegistrations(
        _ request: Gerardus_Align_V1alpha1_ReviewRegistrationsRequest
    ) async throws -> [Gerardus_Align_V1alpha1_RegistrationRecord] {
        throw RPCError(code: .unimplemented, message: "Atomic ReviewRegistrations is required")
    }
    func listStreetUse(l16Token: String) async throws -> [Gerardus_Align_V1alpha1_StreetUseFeature]
    { throw RPCError(code: .unimplemented, message: "ListStreetUse") }

    func editStreetUse(
        _ request: Gerardus_Align_V1alpha1_EditStreetUseRequest
    ) async throws -> Gerardus_Align_V1alpha1_EditStreetUseResponse {
        throw RPCError(code: .unimplemented, message: "EditStreetUse")
    }
}

/// gRPC client for Gerardus's SiteService and RegistrationService.
///
/// SolveTiePoints is not called: the server answers it with UNIMPLEMENTED
/// until tie-point placement is built. GetSiteModel is, and its UNIMPLEMENTED
/// reads as no site model (a server older than site model spike V2).
@available(macOS 15.0, *) final class GerardusClient: AlignmentServiceClient, @unchecked Sendable {

    let address: GerardusAddress
    private let _clientTask = LockedState<Task<Void, Error>?>(nil)
    private let _grpcClient = LockedState<GRPCClient<HTTP2ClientTransport.Posix>?>(nil)

    var isConnected: Bool { _grpcClient.value != nil }

    /// Defaults to the address saved in the app's preferences (Settings).
    init(address: GerardusAddress = .saved(in: .standard)) { self.address = address }

    deinit { disconnect() }

    // MARK: - Connection

    /// Opens the channel and asks the server what it is. Throws when nothing
    /// answers within a few seconds, and leaves no channel open behind it.
    func connect() async throws -> Gerardus_Align_V1alpha1_GetServerInfoResponse {
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
                target: Self.target(address), transportSecurity: .plaintext, config: config,
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

    /// An IPv6 literal is dialled as it stands; anything else goes through
    /// the resolver, which also takes IPv4 literals and MagicDNS names.
    static func target(_ address: GerardusAddress) -> any ResolvableTarget {
        if address.host.contains(":") {
            return ResolvableTargets.IPv6(addresses: [
                SocketAddress.IPv6(host: address.host, port: address.port)
            ])
        }
        return ResolvableTargets.DNS(host: address.host, port: address.port)
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
    /// layer, into one value. A refusal (PERMISSION_DENIED, which Gerardus
    /// gives REGISTERED_SCANS outside loopback and the tailnet) is thrown as
    /// `GerardusClientError.permissionDenied`.
    func streamReference(
        _ request: Gerardus_Align_V1alpha1_StreamReferenceRequest
    ) async throws -> ReferenceGeometry {
        do {
            return try await siteService().streamReference(request: ClientRequest(message: request))
            { response in
                var geometry = ReferenceGeometry()
                for try await message in response.messages { geometry.append(message) }
                return geometry
            }
        } catch let error as RPCError where error.code == .permissionDenied {
            throw GerardusClientError.permissionDenied(error.message)
        }
    }

    func getSiteModel(
        l16Token: String
    ) async throws -> Gerardus_Align_V1alpha1_GetSiteModelResponse? {
        var request = Gerardus_Align_V1alpha1_GetSiteModelRequest()
        request.l16Token = l16Token
        do {
            return try await siteService().getSiteModel(request: ClientRequest(message: request))
        } catch let error as RPCError where error.code == .unimplemented { return nil }
    }

    func listStreetUse(l16Token: String) async throws -> [Gerardus_Align_V1alpha1_StreetUseFeature]
    {
        var request = Gerardus_Align_V1alpha1_ListStreetUseRequest()
        request.l16Token = l16Token
        return try await siteService().listStreetUse(request: ClientRequest(message: request))
            .features
    }

    func editStreetUse(
        _ request: Gerardus_Align_V1alpha1_EditStreetUseRequest
    ) async throws -> Gerardus_Align_V1alpha1_EditStreetUseResponse {
        try await siteService().editStreetUse(request: ClientRequest(message: request))
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

    /// Registers an uploaded scan: a coarse search 30 m around the start at
    /// every heading, ICP on the best distinct poses, then the gates. Takes
    /// 10 to 25 s on a real scan. Each hypothesis is reported as it
    /// converges; the registration that ends the stream ranks them, best
    /// first. Cancelling the calling task stops the run.
    func register(
        _ request: Gerardus_Align_V1alpha1_RegisterRequest,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void,
        onHypothesis: @escaping @Sendable (Gerardus_Align_V1alpha1_Hypothesis) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_Registration {
        try await registrationService().register(request: ClientRequest(message: request)) {
            response in
            var registration: Gerardus_Align_V1alpha1_Registration?
            for try await message in response.messages {
                switch message.event {
                case .progress(let progress): await onProgress(progress)
                case .hypothesis(let hypothesis): await onHypothesis(hypothesis)
                case .registration(let result): registration = result
                case nil: break
                }
            }
            guard let registration else { throw GerardusClientError.incompleteStream("Register") }
            return registration
        }
    }

    /// Scores a pose against the reference, optionally refining it with
    /// local ICP first. About 35 ms (150 ms with refine) once the server has
    /// the area cached; the first call near a sensor takes a second or two.
    func evaluatePose(
        _ request: Gerardus_Align_V1alpha1_EvaluatePoseRequest
    ) async throws -> Gerardus_Align_V1alpha1_EvaluatePoseResponse {
        try await registrationService().evaluatePose(request: ClientRequest(message: request))
    }

    func reviewRegistration(
        _ request: Gerardus_Align_V1alpha1_ReviewRegistrationRequest
    ) async throws -> Gerardus_Align_V1alpha1_RegistrationRecord {
        try await registrationService().reviewRegistration(request: ClientRequest(message: request))
            .record
    }

    func reviewRegistrations(
        _ request: Gerardus_Align_V1alpha1_ReviewRegistrationsRequest
    ) async throws -> [Gerardus_Align_V1alpha1_RegistrationRecord] {
        try await registrationService().reviewRegistrations(
            request: ClientRequest(message: request)
        ).records
    }

    func getRegistration(id: String) async throws -> Gerardus_Align_V1alpha1_Registration {
        var request = Gerardus_Align_V1alpha1_GetRegistrationRequest()
        request.registrationID = id
        return try await registrationService().getRegistration(
            request: ClientRequest(message: request)
        ).registration
    }

    /// Re-solves the site's accepted poses as a small pose graph of 3DEP
    /// priors and scan pairs (site model spike §6.4): seconds per pair of
    /// scans. Needs two accepted scans and 3DEP, or the server answers
    /// FAILED_PRECONDITION. Each scan's run holds one hypothesis, j1, the
    /// refined pose; nothing is recorded until an operator accepts it.
    func refineSite(
        _ request: Gerardus_Align_V1alpha1_RefineSiteRequest,
        onProgress: @escaping @Sendable (Gerardus_Align_V1alpha1_Progress) async -> Void
    ) async throws -> Gerardus_Align_V1alpha1_RefineSiteResult {
        try await registrationService().refineSite(request: ClientRequest(message: request)) {
            response in
            var result: Gerardus_Align_V1alpha1_RefineSiteResult?
            for try await message in response.messages {
                switch message.event {
                case .progress(let progress): await onProgress(progress)
                case .result(let finished): result = finished
                case nil: break
                }
            }
            guard let result else { throw GerardusClientError.incompleteStream("RefineSite") }
            return result
        }
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
