//
//  ServerSettingsTests.swift
//  VelocityVisualiserTests
//
//  Settings ▸ Servers and the status indicator: velocity.report's URL
//  validated, saved and migrated from `annotation.serviceURL`; both Test
//  connection buttons' result states with their latency; the indicator's
//  states; connecting at launch and retrying with backoff on a fake clock;
//  Reconnect; and a changed URL applied, and reconnected, once.
//

import Foundation
import GRPCCore
import SwiftUI
import Testing

@testable import VelocityVisualiser

// MARK: - Fakes

/// A clock the tests step: each reading is the last plus `step`.
private final class SteppingClock: @unchecked Sendable {
    private let lock = NSLock()
    private var instant = ContinuousClock.now
    let step: Duration
    init(step: Duration) { self.step = step }
    func now() -> ContinuousClock.Instant {
        lock.lock()
        defer { lock.unlock() }
        let current = instant
        instant += step
        return current
    }
}

/// Stands in for the retry's sleep: records each wait and holds it until
/// the test advances it.
@MainActor private final class FakeSleeper {
    private(set) var requested: [Duration] = []
    private var waiting: [CheckedContinuation<Void, Never>] = []
    var pending: Int { waiting.count }

    func sleep(_ duration: Duration) async throws {
        requested.append(duration)
        await withCheckedContinuation { waiting.append($0) }
    }

    func advance() {
        guard !waiting.isEmpty else { return }
        waiting.removeFirst().resume()
    }

    /// Lets every held wait finish, so none is left suspended.
    func drain() { while !waiting.isEmpty { advance() } }
}

@MainActor private func until(_ condition: () -> Bool) async {
    for _ in 0..<1000 where !condition() { try? await Task.sleep(for: .milliseconds(2)) }
}

private func freshDefaults() -> UserDefaults {
    let name = "ServerSettingsTests.\(UUID().uuidString)"
    let defaults = UserDefaults(suiteName: name)!
    defaults.removePersistentDomain(forName: name)
    return defaults
}

private func response(
    _ status: Int, url: URL = URL(string: "http://h:8080/api/version")!
) -> URLResponse {
    HTTPURLResponse(url: url, statusCode: status, httpVersion: "HTTP/1.1", headerFields: nil)!
}

/// An AppState whose stream client fails at once (its address does not
/// parse), so nothing dials a real server, and whose retries wait on `sleeper`.
@MainActor private func offlineState(
    defaults: UserDefaults? = nil, sleeper: FakeSleeper, autoConnect: Bool = false
) -> AppState {
    let state = AppState(autoConnectOnStartup: autoConnect, serverDefaults: defaults)
    state.makeVisualiserClient = { _ in VisualiserClient(address: "unparseable") }
    state.reconnectSleep = { try await sleeper.sleep($0) }
    return state
}

// MARK: - Address validation

struct VelocityReportServerParseTests {
    @Test func aBareHostIsHTTPOn8080WithTheStreamOn50051() throws {
        let server = try VelocityReportServer.parse(url: "localhost")
        #expect(server == .local)
        #expect(server.urlDescription == "http://localhost:8080")
        #expect(server.streamAddress == "localhost:50051")
    }

    @Test func schemesHostsPortsAndPathsAreNormalised() throws {
        let named = try VelocityReportServer.parse(url: " HTTP://Machine.Tailnet.TS.net:9090/ ")
        #expect(named.urlDescription == "http://machine.tailnet.ts.net:9090")
        #expect(named.streamAddress == "machine.tailnet.ts.net:50051")

        let secure = try VelocityReportServer.parse(url: "https://lidar.example.org")
        #expect(secure.httpPort == 443 && secure.scheme == "https")

        let v6 = try VelocityReportServer.parse(
            url: "http://[FD7A:115C:A1E0::1]:8080", streamPort: "50052")
        #expect(v6.host == "fd7a:115c:a1e0::1")
        #expect(v6.urlDescription == "http://[fd7a:115c:a1e0::1]:8080")
        #expect(v6.streamAddress == "[fd7a:115c:a1e0::1]:50052")

        let proxied = try VelocityReportServer.parse(url: "http://10.0.0.5/velocity/")
        #expect(proxied.path == "/velocity")
        #expect(
            proxied.httpURL.appendingPathComponent("api/version").absoluteString
                == "http://10.0.0.5:8080/velocity/api/version")
    }

    @Test func badURLsAreRefusedWithAReason() {
        func refusal(_ url: String, _ port: String = "") -> VelocityReportServer.ParseError? {
            do {
                _ = try VelocityReportServer.parse(url: url, streamPort: port)
                return nil
            } catch { return error as? VelocityReportServer.ParseError }
        }
        #expect(refusal("  ") == .empty)
        #expect(refusal("ftp://host") == .unsupportedScheme("ftp"))
        #expect(refusal("http://ada@host") == .notAURL("http://ada@host"))
        #expect(refusal("http://host/?x=1") == .notAURL("http://host/?x=1"))
        #expect(refusal("http://host#top") == .notAURL("http://host#top"))
        #expect(refusal("http://host:99999") == .invalidPort("99999"))
        #expect(refusal("http://host:abc") == .invalidPort("abc"))
        #expect(refusal("http://bad_host") == .invalidHost("bad_host"))
        #expect(refusal("http://1.2.3") == .invalidHost("1.2.3"))
        #expect(refusal("two words") == .notAURL("two words"))
        #expect(refusal("localhost", "0") == .invalidStreamPort("0"))
        #expect(refusal("localhost", "70000") == .invalidStreamPort("70000"))
        #expect(refusal("localhost", "fifty") == .invalidStreamPort("fifty"))
        for error: VelocityReportServer.ParseError in [
            .empty, .unsupportedScheme("ftp"), .notAURL("x"), .invalidHost("x"), .invalidPort("x"),
            .invalidStreamPort("x"),
        ] { #expect(!(error.errorDescription ?? "").isEmpty) }
    }

    @Test func gerardusAddressesAreValidatedAsBefore() throws {
        #expect(try GerardusAddress.parse("[::1]").description == "[::1]:50061")
        #expect(throws: GerardusAddress.ParseError.self) {
            try GerardusAddress.parse("http://machine:50061")
        }
        #expect(throws: GerardusAddress.ParseError.self) { try GerardusAddress.parse("host:0") }
    }

    @Test func theStreamDialsIPv6LiteralsAndRefusesBadAddresses() throws {
        _ = try VisualiserClient.target(for: "[fd7a::1]:50051")
        _ = try VisualiserClient.target(for: "machine.tailnet.ts.net:50051")
        #expect(throws: VisualiserClientError.self) { try VisualiserClient.target(for: "host") }
        #expect(throws: VisualiserClientError.self) { try VisualiserClient.target(for: "[]:1") }
        #expect(throws: VisualiserClientError.self) {
            try VisualiserClient.target(for: "host:70000")
        }
    }
}

// MARK: - Persistence and migration

struct VelocityReportServerPreferenceTests {
    @Test func aServerIsSavedAndFoundAgain() throws {
        let store = freshDefaults()
        #expect(VelocityReportServer.saved(in: store) == .local)
        #expect(VelocityReportServer.saved(in: nil) == .local)

        let remote = try VelocityReportServer.parse(
            url: "http://machine.tailnet.ts.net:8080", streamPort: "50052")
        remote.save(to: store)
        #expect(store.string(forKey: VelocityReportServer.urlKey) == remote.urlDescription)
        #expect(store.integer(forKey: VelocityReportServer.streamPortKey) == 50052)
        #expect(VelocityReportServer.saved(in: store) == remote)

        // This Mac's is the default, kept by removing the keys.
        VelocityReportServer.local.save(to: store)
        #expect(store.object(forKey: VelocityReportServer.urlKey) == nil)
        #expect(store.object(forKey: VelocityReportServer.streamPortKey) == nil)
    }

    @Test func aSavedURLThatNoLongerParsesMeansThisMac() {
        let store = freshDefaults()
        store.set("gopher://elsewhere", forKey: VelocityReportServer.urlKey)
        #expect(VelocityReportServer.saved(in: store) == .local)
    }

    @Test func theOldKeyIsReadThenMovedOnce() throws {
        let store = freshDefaults()
        store.set("http://10.0.0.5:8080", forKey: VelocityReportServer.legacyURLKey)
        let expected = try VelocityReportServer.parse(url: "http://10.0.0.5:8080")
        // Read before it is moved, so nothing changes for a user who has it.
        #expect(VelocityReportServer.saved(in: store) == expected)

        #expect(VelocityReportServer.migrate(in: store) == .migrated(expected))
        #expect(store.string(forKey: VelocityReportServer.urlKey) == "http://10.0.0.5:8080")
        #expect(store.object(forKey: VelocityReportServer.legacyURLKey) == nil)
        #expect(VelocityReportServer.saved(in: store) == expected)
        #expect(VelocityReportServer.migrate(in: store) == .nothingToMigrate)
    }

    @Test func anOldKeyNamingThisMacIsSimplyRemoved() {
        let store = freshDefaults()
        store.set("http://localhost:8080/", forKey: VelocityReportServer.legacyURLKey)
        #expect(VelocityReportServer.migrate(in: store) == .migrated(.local))
        #expect(store.object(forKey: VelocityReportServer.legacyURLKey) == nil)
        #expect(store.object(forKey: VelocityReportServer.urlKey) == nil)
    }

    @Test func anUnreadableOldKeyIsLeftAlone() {
        let store = freshDefaults()
        store.set("not a url", forKey: VelocityReportServer.legacyURLKey)
        #expect(VelocityReportServer.migrate(in: store) == .unreadable("not a url"))
        #expect(store.string(forKey: VelocityReportServer.legacyURLKey) == "not a url")
        #expect(VelocityReportServer.saved(in: store) == .local)
    }

    @Test func theNewKeyWinsOverTheOld() throws {
        let store = freshDefaults()
        store.set("http://old:8080", forKey: VelocityReportServer.legacyURLKey)
        store.set("http://new:8080", forKey: VelocityReportServer.urlKey)
        #expect(VelocityReportServer.migrate(in: store) == .nothingToMigrate)
        #expect(VelocityReportServer.saved(in: store).host == "new")
    }

    @MainActor @Test func launchMigratesAndUsesTheSavedServer() {
        let store = freshDefaults()
        store.set("http://10.0.0.5:8080", forKey: VelocityReportServer.legacyURLKey)
        let state = AppState(autoConnectOnStartup: false, serverDefaults: store)
        #expect(state.connection.server.host == "10.0.0.5")
        #expect(state.serverAddress == "10.0.0.5:50051")
        #expect(store.object(forKey: VelocityReportServer.legacyURLKey) == nil)
        #expect(store.string(forKey: VelocityReportServer.urlKey) == "http://10.0.0.5:8080")
    }
}

// MARK: - Test connection

struct ConnectionTestResultTests {
    private let server = VelocityReportServer.local

    @Test func versionAnsweredIsOKWithItsLatency() async {
        let clock = SteppingClock(step: .milliseconds(12))
        let body = #"{"version":"0.6.1","git_sha":"abc1234","build_time":"2026-10-25T10:00:00Z"}"#
        let result = await ServerConnectionTest.velocityReportAPI(
            server,
            fetch: { request in
                #expect(request.url?.absoluteString == "http://localhost:8080/api/version")
                #expect(request.timeoutInterval == 3)
                return (Data(body.utf8), response(200))
            }, now: { clock.now() })
        #expect(
            result
                == .ok(
                    ServerIdentity(
                        version: "0.6.1", gitSHA: "abc1234", buildTime: "2026-10-25T10:00:00Z"),
                    latency: .milliseconds(12)))
        #expect(result.summary == "Reachable, 12 ms.")
        #expect(result.latency == .milliseconds(12))
    }

    @Test func anotherHTTPServerIsTheWrongService() async {
        let clock = SteppingClock(step: .milliseconds(3))
        let notFound = await ServerConnectionTest.velocityReportAPI(
            server, fetch: { _ in (Data("404".utf8), response(404)) }, now: { clock.now() })
        guard case .wrongService(let detail, let latency) = notFound else {
            Issue.record("\(notFound)")
            return
        }
        #expect(detail.contains("404") && latency == .milliseconds(3))

        let notJSON = await ServerConnectionTest.velocityReportAPI(
            server, fetch: { _ in (Data("<html>".utf8), response(200)) }, now: { clock.now() })
        guard case .wrongService = notJSON else {
            Issue.record("\(notJSON)")
            return
        }
    }

    @Test func refusedIsUnreachableAndSlowIsATimeout() async {
        let refused = await ServerConnectionTest.velocityReportAPI(
            server, fetch: { _ in throw URLError(.cannotConnectToHost) }, now: { .now })
        guard case .unreachable = refused else {
            Issue.record("\(refused)")
            return
        }
        #expect(refused.latency == nil)
        let slow = await ServerConnectionTest.velocityReportAPI(
            server, fetch: { _ in throw URLError(.timedOut) }, now: { .now })
        #expect(slow == .timedOut(after: .seconds(3)))
        #expect(slow.summary == "No answer within 3 s.")
    }

    @MainActor @Test func theStreamIsTestedByGetCapabilities() async {
        let clock = SteppingClock(step: .milliseconds(5))
        let capabilities: Velocity_Visualiser_V1_CapabilitiesResponse = {
            var capabilities = Velocity_Visualiser_V1_CapabilitiesResponse()
            capabilities.availableSensors = ["hesai-pandar40p"]
            return capabilities
        }()
        let report = await ServerConnectionTest.velocityReport(
            server, fetch: { _ in throw URLError(.cannotConnectToHost) },
            streamProbe: { address, timeout in
                #expect(address == "localhost:50051" && timeout == .seconds(3))
                return capabilities
            }, now: { clock.now() })
        // The two calls run together and share the clock, so only the
        // direct call below pins the latency.
        guard case .ok(let identity, _) = report.stream, case .unreachable = report.api else {
            Issue.record("\(report)")
            return
        }
        #expect(identity == ServerIdentity(sensors: ["hesai-pandar40p"]))
        let alone = await ServerConnectionTest.velocityReportStream(
            server, probe: { _, _ in capabilities }, now: { clock.now() })
        #expect(alone == .ok(identity, latency: .milliseconds(5)))

        func stream(_ code: RPCError.Code) async -> ConnectionTestResult {
            await ServerConnectionTest.velocityReportStream(
                server, probe: { _, _ in throw RPCError(code: code, message: "m") },
                now: { clock.now() })
        }
        guard case .wrongService(_, let latency) = await stream(.unimplemented) else {
            Issue.record("UNIMPLEMENTED is not the wrong service")
            return
        }
        #expect(latency == .milliseconds(5))
        #expect(await stream(.deadlineExceeded) == .timedOut(after: .seconds(3)))
        #expect(await stream(.unavailable) == .unreachable("unavailable: m"))
    }

    @Test func gerardusReportsItsVersionShaLatencyAndScans() async {
        let clock = SteppingClock(step: .milliseconds(7))
        let info: Gerardus_Align_V1alpha1_GetServerInfoResponse = {
            var info = Gerardus_Align_V1alpha1_GetServerInfoResponse()
            info.serverVersion = "0.1.0+3f2a9c1e0b7d"
            info.apiVersion = "gerardus.align.v1alpha1"
            info.availableLayers = [.aerialPoints, .registeredScans]
            return info
        }()
        let ok = await ServerConnectionTest.gerardus(connect: { info }, now: { clock.now() })
        #expect(
            ok
                == .ok(
                    ServerIdentity(
                        version: "0.1.0", gitSHA: "3f2a9c1e0b7d",
                        apiVersion: "gerardus.align.v1alpha1", servesScans: true),
                    latency: .milliseconds(7)))

        func failing(_ code: RPCError.Code) async -> ConnectionTestResult {
            await ServerConnectionTest.gerardus(
                connect: { throw RPCError(code: code, message: "") }, now: { clock.now() })
        }
        guard case .wrongService(let detail, _) = await failing(.unimplemented) else {
            Issue.record("a server without SiteService is not the wrong service")
            return
        }
        #expect(detail.contains("Gerardus"))
        #expect(await failing(.deadlineExceeded) == .timedOut(after: .seconds(3)))
        #expect(await failing(.unavailable) == .unreachable("unavailable"))
        let other = await ServerConnectionTest.gerardus(
            connect: { throw GerardusClientError.notConnected }, now: { clock.now() })
        #expect(other == .unreachable("Not connected to the alignment service"))
    }

    @MainActor @Test func theSessionsTestTakesUpAServerItHadNotFound() async {
        let fake = FakeAlignmentService()
        let session = AlignmentSession(makeClient: { _ in fake }, lookup: nil, defaults: nil)
        #expect(!session.isAvailable)
        let result = await session.testConnection()
        #expect(result.isOK)
        #expect(session.isAvailable)

        // Another address is only tried: the session stays where it is.
        let other = AlignmentSession(makeClient: { _ in fake }, lookup: nil, defaults: nil)
        let elsewhere = await other.testConnection(GerardusAddress(host: "elsewhere", port: 1))
        #expect(elsewhere.isOK)
        #expect(!other.isAvailable)
    }

    @Test func latencyReadsInMilliseconds() {
        #expect(ConnectionTestResult.milliseconds(.microseconds(400)) == "0.4 ms")
        #expect(ConnectionTestResult.milliseconds(.milliseconds(1500)) == "1500 ms")
        #expect(ConnectionTestResult.seconds(.milliseconds(2500)) == "2.5 s")
    }
}

// MARK: - The status indicator

struct ServerStatusMappingTests {
    @Test func velocityReportsStates() {
        func status(
            _ connected: Bool, _ connecting: Bool, _ open: Bool, _ error: String?
        ) -> ServerStatus {
            ServerStatus.velocityReport(
                isConnected: connected, isConnecting: connecting, isStreamOpen: open, error: error)
        }
        #expect(status(true, false, true, nil) == .connected)
        // A transport with no stream accepted yet is not connected.
        #expect(status(true, false, false, nil) == .connecting)
        #expect(status(false, true, false, "old") == .connecting)
        #expect(status(false, false, false, nil) == .disconnected)
        #expect(status(false, false, false, "") == .disconnected)
        #expect(status(false, false, false, "Connection lost") == .error("Connection lost"))
        let long = String(repeating: "x", count: 80)
        #expect(status(false, false, false, long).label.count <= "Error: ".count + 33)
    }

    @MainActor @Test func gerardusStates() {
        #expect(
            ServerStatus.gerardus(availability: .unknown, isChecking: true, failure: nil)
                == .connecting)
        #expect(
            ServerStatus.gerardus(
                availability: .available(serverVersion: "0.1.0"), isChecking: false, failure: nil)
                == .connected)
        #expect(
            ServerStatus.gerardus(availability: .unknown, isChecking: false, failure: nil)
                == .disconnected)
        #expect(
            ServerStatus.gerardus(
                availability: .unavailable, isChecking: false,
                failure: "unavailable: connection refused") == .error("unavailable"))
        #expect(
            ServerStatus.gerardus(availability: .unavailable, isChecking: false, failure: nil)
                == .error("no answer"))
    }

    @Test func salientReasonsFitTheFrame() {
        #expect(
            ServerStatus.salientReason("unavailable: Connection refused") == "Connection refused")
        #expect(
            ServerStatus.salientReason("Failed: cannot connect to 127.0.0.1:50051")
                == "cannot connect to 127.0.0.1:50051")
        #expect(
            ServerStatus.salientReason("The operation couldn’t be completed. (Connection refused)")
                == "Connection refused")
        #expect(
            ServerStatus.salientReason("Error: wrapper (inner (deepest cause).)") == "deepest cause"
        )
        #expect(ServerStatus.salientReason("no answer.") == "no answer")
        #expect(ServerStatus.salientReason("  ") == "  ")
        let long = String(repeating: "x", count: 60)
        #expect(ServerStatus.salientReason(long) == String(repeating: "x", count: 48) + "…")
        // Only an error says anything after the name; the full error wins
        // over the status's truncated reason.
        #expect(ServerStatusIndicator.frameReason(.connected, lastError: "stale") == nil)
        #expect(ServerStatusIndicator.frameReason(.connecting, lastError: "stale") == nil)
        #expect(ServerStatusIndicator.frameReason(.disconnected, lastError: nil) == nil)
        #expect(
            ServerStatusIndicator.frameReason(
                .error("Failed: cannot connect to 127.0.0.1"),
                lastError: "unavailable: Connection refused") == "Connection refused")
        #expect(
            ServerStatusIndicator.frameReason(.error("unavailable"), lastError: nil)
                == "unavailable")
    }

    @Test func labelsAndColours() {
        #expect(ServerStatus.connected.label == "Connected")
        #expect(ServerStatus.connecting.label == "Connecting…")
        #expect(ServerStatus.disconnected.label == "Disconnected")
        #expect(ServerStatus.error("refused").label == "Error: refused")
        #expect(ServerStatusDot.colour(.connected) == .green)
        #expect(ServerStatusDot.colour(.error("x")) == .red)
        #expect(ServerStatus.shortReason("Connection lost") == "Connection lost")
    }

    @MainActor @Test func appStateKeepsTheIndicatorsStatusInStep() async {
        let state = AppState(autoConnectOnStartup: false, serverDefaults: nil)
        #expect(state.connection.status == .disconnected)
        state.isConnecting = true
        #expect(state.connection.status == .connecting)
        state.isConnecting = false
        state.connectionError = "Connection lost"
        #expect(state.connection.status == .error("Connection lost"))
        state.isConnected = true
        state.streamDidOpen(client: nil)
        #expect(state.connection.status == .connected)
        let tooltip = ServerStatusIndicator.tooltip(
            connection: state.connection, session: state.alignment, gerardus: .disconnected)
        #expect(tooltip.contains("http://localhost:8080"))
        #expect(tooltip.contains("127.0.0.1:50061"))
    }
}

// MARK: - Lifecycle

@MainActor struct ConnectionLifecycleTests {
    @Test func backoffDoublesToItsCeiling() {
        let backoff = ReconnectBackoff()
        let delays = (1...7).map { backoff.delay(afterFailures: $0) }
        #expect(
            delays == [
                .seconds(1), .seconds(2), .seconds(4), .seconds(8), .seconds(16), .seconds(30),
                .seconds(30),
            ])
        #expect(backoff.delay(afterFailures: 0) == .seconds(1))
    }

    @Test func launchConnectsAndRetriesWithBackoff() async {
        let sleeper = FakeSleeper()
        let state = offlineState(sleeper: sleeper, autoConnect: true)
        // Launch waits half a second, then connects on its own.
        await until { sleeper.pending == 1 }
        #expect(state.wantsConnection)
        #expect(state.connectionAttempts == 1)
        #expect(sleeper.requested == [.seconds(1)])
        #expect(state.connection.retryIn == .seconds(1))
        #expect(state.isRetryScheduled)
        guard case .error = state.connection.status else {
            Issue.record("\(state.connection.status) is not an error")
            return
        }
        #expect(state.connection.lastError != nil)

        sleeper.advance()
        await until { sleeper.pending == 1 && state.connectionAttempts == 2 }
        #expect(sleeper.requested == [.seconds(1), .seconds(2)])
        sleeper.advance()
        await until { sleeper.requested.count == 3 }
        #expect(state.connectionAttempts == 3)
        #expect(sleeper.requested.last == .seconds(4))
        state.disconnect()
        sleeper.drain()
    }

    @Test func anOpenStreamStartsTheBackoffAgain() async {
        let sleeper = FakeSleeper()
        let state = offlineState(sleeper: sleeper)
        state.startConnection()
        await until { sleeper.pending == 1 }
        sleeper.advance()
        await until { sleeper.requested.count == 2 }
        #expect(state.failuresSinceConnected == 2)

        state.streamDidOpen(client: nil)
        #expect(state.failuresSinceConnected == 0)
        #expect(!state.isRetryScheduled)
        #expect(state.connection.retryIn == nil)

        // The next drop waits the first step again.
        state.connectionDropped(client: nil, reason: "unavailable")
        await until { sleeper.requested.count == 3 }
        #expect(sleeper.requested.last == .seconds(1))
        #expect(state.connection.lastError == "unavailable")
        state.disconnect()
        sleeper.drain()
    }

    @Test func oneConnectAloneIsNotRetried() async {
        let sleeper = FakeSleeper()
        let state = offlineState(sleeper: sleeper)
        state.connect()
        await until { !state.isConnecting }
        #expect(state.connectionError != nil)
        #expect(sleeper.requested.isEmpty)
        #expect(!state.isRetryScheduled)
    }

    @Test func disconnectStopsRetrying() async {
        let sleeper = FakeSleeper()
        let state = offlineState(sleeper: sleeper)
        state.startConnection()
        await until { sleeper.pending == 1 }
        state.disconnect()
        sleeper.drain()
        #expect(!state.wantsConnection && !state.isRetryScheduled)
        sleeper.advance()
        try? await Task.sleep(for: .milliseconds(20))
        #expect(state.connectionAttempts == 1)
    }

    @Test func reconnectRetriesAtOnceAndStartsTheBackoffAgain() async {
        let sleeper = FakeSleeper()
        let state = offlineState(sleeper: sleeper)
        state.startConnection()
        await until { sleeper.pending == 1 }
        sleeper.advance()
        await until { sleeper.requested.count == 2 }
        #expect(state.connectionAttempts == 2)

        state.reconnect()
        #expect(state.connectionAttempts == 3)
        #expect(state.isConnecting)
        await until { sleeper.requested.count == 3 }
        // Started afresh: the first step, not the third.
        #expect(sleeper.requested.last == .seconds(1))
        state.disconnect()
        sleeper.drain()
    }

    @Test func aDropFromAReplacedClientIsIgnored() async {
        let sleeper = FakeSleeper()
        let state = offlineState(sleeper: sleeper)
        state.startConnection()
        let stale = VisualiserClient(address: "stale:1")
        state.connectionDropped(client: stale, reason: "late")
        state.streamDidOpen(client: stale)
        #expect(!state.isStreamOpen)
        #expect(state.connection.lastError != "late")
        state.disconnect()
        sleeper.drain()
    }
}

// MARK: - Changing the URL

@MainActor struct ServerChangeTests {
    @Test func aChangedURLIsSavedAndReconnectsOnce() async throws {
        let store = freshDefaults()
        let sleeper = FakeSleeper()
        let state = offlineState(defaults: store, sleeper: sleeper)
        let remote = try VelocityReportServer.parse(
            url: "http://machine.tailnet.ts.net", streamPort: "50052")

        #expect(state.useVelocityReportServer(remote))
        #expect(state.connectionAttempts == 1)
        #expect(state.serverAddress == "machine.tailnet.ts.net:50052")
        #expect(state.connection.server == remote)
        #expect(VelocityReportServer.saved(in: store) == remote)

        // The same server again is not a change, and not a reconnect.
        #expect(!state.useVelocityReportServer(remote))
        #expect(state.connectionAttempts == 1)
        state.disconnect()
        sleeper.drain()
    }

    @Test func settingsApplyParsesFirstAndAppliesOnce() async throws {
        let store = freshDefaults()
        let sleeper = FakeSleeper()
        let state = offlineState(defaults: store, sleeper: sleeper)

        let bad = VelocityReportSettings.apply(url: "ftp://x", streamPort: "", to: state)
        #expect(bad == .failure(.unsupportedScheme("ftp")))
        let badPort = VelocityReportSettings.apply(url: "x", streamPort: "0", to: state)
        #expect(badPort == .failure(.invalidStreamPort("0")))
        #expect(state.connectionAttempts == 0)
        #expect(store.object(forKey: VelocityReportServer.urlKey) == nil)

        let good = try VelocityReportSettings.apply(url: "10.0.0.5:9000", streamPort: "", to: state)
            .get()
        #expect(good.urlDescription == "http://10.0.0.5:9000")
        #expect(state.connectionAttempts == 1)
        _ = VelocityReportSettings.apply(url: "http://10.0.0.5:9000/", streamPort: "", to: state)
        #expect(state.connectionAttempts == 1)
        #expect(VelocityReportSettings.streamPortText(good) == "")
        state.disconnect()
        sleeper.drain()
    }

    @Test func settingsOpensOnServers() {
        let store = freshDefaults()
        SettingsTab.select(.servers, in: store)
        #expect(store.string(forKey: SettingsTab.key) == "servers")
    }
}
