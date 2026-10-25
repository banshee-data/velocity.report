// ServerConnection.swift
// The two servers the app talks to, as Settings ▸ Servers and the main
// window's status indicator show them: what state each connection is in,
// how a retry is paced, and what Test connection found.
//
// velocity.report is the server the app cannot work without: it connects at
// launch and retries with backoff while nothing answers. Gerardus is
// optional, asked by AlignmentSession while velocity.report is connected.

import Combine
import Foundation
import GRPCCore

// MARK: - Retry pacing

/// How long to wait before the next connection attempt: one second after
/// the first failure, doubling each time, never more than thirty. A
/// connection that opens its stream starts the count again.
struct ReconnectBackoff: Equatable, Sendable {
    var initial: Duration = .seconds(1)
    var maximum: Duration = .seconds(30)

    /// The wait after `failures` failures in a row (1 for the first).
    func delay(afterFailures failures: Int) -> Duration {
        guard failures > 1 else { return min(initial, maximum) }
        var delay = initial
        for _ in 1..<failures {
            delay *= 2
            if delay >= maximum { return maximum }
        }
        return delay
    }
}

// MARK: - Status

/// What the indicator says about one server.
enum ServerStatus: Equatable, Sendable {
    case connected
    case connecting
    case disconnected
    case error(String)

    /// The text beside the dot.
    var label: String {
        switch self {
        case .connected: return "Connected"
        case .connecting: return "Connecting…"
        case .disconnected: return "Disconnected"
        case .error(let reason): return "Error: \(reason)"
        }
    }

    /// The longest reason shown beside the dot; the tooltip has the rest.
    static let maxReasonLength = 32

    /// velocity.report's connection. A client whose service has not yet
    /// answered (GetCapabilities, or the stream accepted) is still
    /// connecting: the transport is made at once, whether or not anything
    /// listens.
    static func velocityReport(
        isConnected: Bool, isConnecting: Bool, isStreamOpen: Bool, error: String?
    ) -> ServerStatus {
        if isConnected && isStreamOpen { return .connected }
        if isConnecting || isConnected { return .connecting }
        if let error, !error.isEmpty { return .error(error.truncated(maxReasonLength)) }
        return .disconnected
    }

    /// Gerardus, as AlignmentSession last found it. Not yet asked is
    /// disconnected; asked with no answer is an error with its reason.
    @available(macOS 15.0, *) static func gerardus(
        availability: AlignmentSession.Availability, isChecking: Bool, failure: String?
    ) -> ServerStatus {
        if isChecking { return .connecting }
        switch availability {
        case .available: return .connected
        case .unknown: return .disconnected
        case .unavailable:
            let reason = failure.map(shortReason) ?? "no answer"
            return .error(reason.truncated(maxReasonLength))
        }
    }

    /// A gRPC failure as its code ("unavailable"), which is what fits beside
    /// a dot; anything else as its description.
    static func shortReason(_ description: String) -> String {
        // GerardusClientError.describe gives "code: message".
        if let colon = description.firstIndex(of: ":") {
            let code = description[..<colon]
            if !code.contains(" ") { return String(code) }
        }
        return description
    }
}

// MARK: - Test connection

/// What a server said about itself when tested.
struct ServerIdentity: Equatable, Sendable {
    var version: String?
    var gitSHA: String?
    var buildTime: String?
    /// Gerardus's API version (gerardus.align.v1alpha1).
    var apiVersion: String?
    /// Gerardus only: whether it serves registered scans; nil when it does
    /// not say.
    var servesScans: Bool?
    /// velocity.report only: the sensors its stream offers.
    var sensors: [String]?
}

/// One Test connection: what came back, and how long it took.
enum ConnectionTestResult: Equatable, Sendable {
    /// It answered as the server it should be.
    case ok(ServerIdentity, latency: Duration)
    /// Something answered, but not this service: another server on the
    /// port, or a server too old to answer the call.
    case wrongService(String, latency: Duration)
    /// Nothing could be reached: refused, no route, no such host.
    case unreachable(String)
    /// Nothing answered within the test's timeout.
    case timedOut(after: Duration)

    var isOK: Bool {
        if case .ok = self { return true }
        return false
    }

    var latency: Duration? {
        switch self {
        case .ok(_, let latency), .wrongService(_, let latency): return latency
        case .unreachable, .timedOut: return nil
        }
    }

    /// One line for Settings.
    var summary: String {
        switch self {
        case .ok(_, let latency): return "Reachable, \(Self.milliseconds(latency))."
        case .wrongService(let detail, let latency):
            return "Wrong service (\(Self.milliseconds(latency))): \(detail)"
        case .unreachable(let reason): return "Unreachable: \(reason)"
        case .timedOut(let after): return "No answer within \(Self.seconds(after))."
        }
    }

    /// "12 ms", or "0.4 ms" below one.
    static func milliseconds(_ duration: Duration) -> String {
        let ms =
            Double(duration.components.seconds) * 1000 + Double(duration.components.attoseconds)
            / 1e15
        if ms < 1 { return String(format: "%.1f ms", ms) }
        return "\(Int(ms.rounded())) ms"
    }

    static func seconds(_ duration: Duration) -> String {
        let s = Double(duration.components.seconds) + Double(duration.components.attoseconds) / 1e18
        return s == s.rounded() ? "\(Int(s)) s" : String(format: "%.1f s", s)
    }
}

/// velocity.report's two endpoints, tested together.
struct VelocityReportTestReport: Equatable, Sendable {
    /// `GET /api/version` on the HTTP API.
    var api: ConnectionTestResult
    /// GetCapabilities on the frame stream's port.
    var stream: ConnectionTestResult
}

/// The Test connection buttons. The calls are passed in so that tests can
/// answer them; the defaults reach the real servers.
enum ServerConnectionTest {
    static let timeout: Duration = .seconds(3)

    typealias HTTPFetch = @Sendable (URLRequest) async throws -> (Data, URLResponse)
    typealias StreamProbe =
        @Sendable (_ address: String, _ timeout: Duration) async throws ->
        Velocity_Visualiser_V1_CapabilitiesResponse
    typealias Clock = @Sendable () -> ContinuousClock.Instant

    static let liveFetch: HTTPFetch = { try await APISession.shared.data(for: $0) }
    @available(macOS 15.0, *) static let liveStreamProbe: StreamProbe = {
        try await VisualiserClient.probeCapabilities(address: $0, timeout: $1)
    }

    /// The body of `/api/version`.
    private struct VersionBody: Decodable {
        var version: String?
        var git_sha: String?
        var build_time: String?
    }

    /// Asks velocity.report's HTTP API for its version and its stream for its
    /// capabilities, each with a short timeout.
    @available(macOS 15.0, *) static func velocityReport(
        _ server: VelocityReportServer, fetch: HTTPFetch = liveFetch,
        streamProbe: StreamProbe = liveStreamProbe, now: Clock = { .now }
    ) async -> VelocityReportTestReport {
        async let api = velocityReportAPI(server, fetch: fetch, now: now)
        async let stream = velocityReportStream(server, probe: streamProbe, now: now)
        return await VelocityReportTestReport(api: api, stream: stream)
    }

    static func velocityReportAPI(
        _ server: VelocityReportServer, fetch: HTTPFetch, now: Clock
    ) async -> ConnectionTestResult {
        var request = URLRequest(url: server.httpURL.appendingPathComponent("api/version"))
        request.timeoutInterval = seconds(timeout)
        request.cachePolicy = .reloadIgnoringLocalCacheData
        let start = now()
        do {
            let (data, response) = try await fetch(request)
            return classifyVersion(data: data, response: response, latency: now() - start)
        } catch { return classifyHTTPError(error) }
    }

    @available(macOS 15.0, *) static func velocityReportStream(
        _ server: VelocityReportServer, probe: StreamProbe, now: Clock
    ) async -> ConnectionTestResult {
        let start = now()
        do {
            let capabilities = try await probe(server.streamAddress, timeout)
            return .ok(
                ServerIdentity(sensors: capabilities.availableSensors), latency: now() - start)
        } catch {
            return classifyRPCError(
                error, latency: now() - start,
                wrongService: "something answers gRPC there, but not the visualiser stream")
        }
    }

    /// What `/api/version` answered. velocity.report answers 200 with its
    /// version; anything else that answers is some other server.
    static func classifyVersion(
        data: Data, response: URLResponse, latency: Duration
    ) -> ConnectionTestResult {
        guard let http = response as? HTTPURLResponse else {
            return .wrongService("the answer is not HTTP", latency: latency)
        }
        guard http.statusCode == 200 else {
            return .wrongService(
                "/api/version answered HTTP \(http.statusCode); is this velocity.report?",
                latency: latency)
        }
        guard let body = try? JSONDecoder().decode(VersionBody.self, from: data),
            let version = body.version, !version.isEmpty
        else {
            return .wrongService(
                "/api/version did not answer with a version; is this velocity.report?",
                latency: latency)
        }
        return .ok(
            ServerIdentity(
                version: version, gitSHA: body.git_sha.flatMap { $0.isEmpty ? nil : $0 },
                buildTime: body.build_time.flatMap { $0.isEmpty ? nil : $0 }), latency: latency)
    }

    static func classifyHTTPError(_ error: Error) -> ConnectionTestResult {
        if let url = error as? URLError {
            if url.code == .timedOut { return .timedOut(after: timeout) }
            return .unreachable(url.localizedDescription)
        }
        return .unreachable(error.localizedDescription)
    }

    /// A failed gRPC call: past its deadline is a timeout, UNIMPLEMENTED is
    /// a server that does not offer the service, and the rest is unreachable.
    static func classifyRPCError(
        _ error: Error, latency: Duration, wrongService: String
    ) -> ConnectionTestResult {
        guard let rpc = error as? RPCError else { return .unreachable(error.localizedDescription) }
        switch rpc.code {
        case .deadlineExceeded: return .timedOut(after: timeout)
        case .unimplemented: return .wrongService(wrongService, latency: latency)
        default:
            return .unreachable(rpc.message.isEmpty ? "\(rpc.code)" : "\(rpc.code): \(rpc.message)")
        }
    }

    /// Times one GetServerInfo (`connect`, on a channel of the test's own)
    /// and reads what it says.
    static func gerardus(
        connect: () async throws -> Gerardus_Align_V1alpha1_GetServerInfoResponse,
        now: Clock = { .now }
    ) async -> ConnectionTestResult {
        let start = now()
        do {
            let info = try await connect()
            let version = GerardusSettings.version(info.serverVersion)
            return .ok(
                ServerIdentity(
                    version: version.version.isEmpty ? nil : version.version,
                    gitSHA: version.gitSHA,
                    apiVersion: info.apiVersion.isEmpty ? nil : info.apiVersion,
                    servesScans: GerardusSettings.servesScans(info)), latency: now() - start)
        } catch {
            return classifyRPCError(
                error, latency: now() - start,
                wrongService: "something answers gRPC there, but not Gerardus's SiteService")
        }
    }

    static func seconds(_ duration: Duration) -> TimeInterval {
        Double(duration.components.seconds) + Double(duration.components.attoseconds) / 1e18
    }
}

// MARK: - Shared state

/// velocity.report's connection as Settings and the indicator show it.
/// Kept apart from AppState, which publishes on every frame, so the Settings
/// window redraws only when the connection changes.
@MainActor final class VelocityReportConnection: ObservableObject {
    /// The server in use: the saved one, until Settings applies another.
    @Published fileprivate(set) var server: VelocityReportServer
    @Published fileprivate(set) var status: ServerStatus = .disconnected
    /// The last thing that went wrong, in full, for the tooltip.
    @Published fileprivate(set) var lastError: String?
    /// The wait before the next attempt, while one is scheduled.
    @Published fileprivate(set) var retryIn: Duration?

    init(server: VelocityReportServer) { self.server = server }

    /// The indicator's tooltip: where, and what last went wrong.
    var tooltip: String {
        var lines = ["velocity.report \(server.urlDescription), stream \(server.streamAddress)"]
        lines.append(status.label)
        if let lastError { lines.append("Last error: \(lastError)") }
        if let retryIn { lines.append("Retrying in \(ConnectionTestResult.seconds(retryIn)).") }
        return lines.joined(separator: "\n")
    }
}

/// AppState's handle on the connection model; the setters stay in this file.
extension VelocityReportConnection {
    func update(
        server: VelocityReportServer? = nil, status: ServerStatus? = nil, lastError: String?? = nil,
        retryIn: Duration?? = nil
    ) {
        if let server, server != self.server { self.server = server }
        if let status, status != self.status { self.status = status }
        if let lastError, lastError != self.lastError { self.lastError = lastError }
        if let retryIn, retryIn != self.retryIn { self.retryIn = retryIn }
    }
}
