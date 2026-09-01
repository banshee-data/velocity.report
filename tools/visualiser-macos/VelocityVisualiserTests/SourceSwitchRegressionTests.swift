//
//  SourceSwitchRegressionTests.swift
//  VelocityVisualiserTests
//
//  Regression cover for the source-switching faults found while reworking the
//  pipeline's state model. Each case below is something that shipped broken
//  once; the comment says what the operator saw, because that is what makes a
//  failure here legible to whoever hits it next.
//

import XCTest

@testable import VelocityVisualiser

/// Counts the calls that returning to live has to make, and holds the API
/// client away from any real server.
@available(macOS 15.0, *) @MainActor final class SpySourceSwitchState: AppState {
    var restartGRPCStreamCallCount = 0
    var clearAllCallCount = 0

    override func restartGRPCStream() { restartGRPCStreamCallCount += 1 }
    override func clearAll() {
        clearAllCallCount += 1
        super.clearAll()
    }

    /// Answers the return-to-live request from a stub rather than the network.
    /// Tests must never reach a running server: an earlier version of these
    /// posted to whatever was listening and switched its source out from under
    /// the operator.
    func useStubbedClient(succeeding: Bool) {
        StubURLProtocol.succeed = succeeding
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [StubURLProtocol.self]
        runTrackLabelClientOverride = RunTrackLabelAPIClient(
            baseURL: URL(string: "http://stub.invalid")!,
            session: URLSession(configuration: config))
    }
}

/// Answers every request without touching the network.
final class StubURLProtocol: URLProtocol {
    nonisolated(unsafe) static var succeed = true

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let code = Self.succeed ? 200 : 500
        let response = HTTPURLResponse(
            url: request.url!, statusCode: code, httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data("{}".utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}

@available(macOS 15.0, *) @MainActor final class ReturnToLiveStreamTests: XCTestCase {

    /// Loading a replay restarts the stream, because the source changes under
    /// one that is still carrying the old. Going the other way needs it just as
    /// much, and more so after a replay that wedged its client: that stream is
    /// blocked on a send it will never finish, so without a restart the view
    /// sat on the replay's last frame and no live data ever arrived.
    func testReturningToLiveRestartsTheStream() async {
        let state = SpySourceSwitchState()
        state.useStubbedClient(succeeding: true)
        state.sourceMode = .vrlog

        state.returnToLive()
        await waitUntil { state.restartGRPCStreamCallCount > 0 }

        XCTAssertEqual(
            state.restartGRPCStreamCallCount, 1,
            "returning to live did not restart the stream; a wedged replay stream stays wedged")
    }

    /// The run is no longer being replayed, so track labels must stop routing
    /// to its run-track API.
    func testReturningToLiveClearsTheRun() async {
        let state = SpySourceSwitchState()
        state.useStubbedClient(succeeding: true)
        state.sourceMode = .vrlog
        state.currentRunID = "run-under-replay"

        state.returnToLive()
        await waitUntil { state.currentRunID == nil }

        XCTAssertNil(state.currentRunID)
    }

    /// The grid is reset server-side, so the client must drop what it holds.
    func testReturningToLiveClearsTheScene() async {
        let state = SpySourceSwitchState()
        state.useStubbedClient(succeeding: true)
        state.sourceMode = .vrlog

        state.returnToLive()
        await waitUntil { state.clearAllCallCount > 0 }

        XCTAssertGreaterThan(state.clearAllCallCount, 0, "the replay's scene was left on screen")
    }

    /// A failed request must not report the source as live, and must not leave
    /// the control disabled for the rest of the session.
    func testAFailedReturnLeavesTheSourceAloneAndReleasesTheControl() async {
        let state = SpySourceSwitchState()
        state.useStubbedClient(succeeding: false)
        state.sourceMode = .vrlog

        state.returnToLive()
        await waitUntil { !state.isReturningToLive && state.restartGRPCStreamCallCount == 0 }

        XCTAssertEqual(state.sourceMode, .vrlog, "the source was reported live after a failure")
        XCTAssertFalse(state.isReturningToLive, "the in-flight flag survived a failure")
    }

    /// Two presses must not send two requests.
    func testASecondPressIsIgnoredWhileOneIsInFlight() async {
        let state = SpySourceSwitchState()
        state.useStubbedClient(succeeding: true)
        state.sourceMode = .vrlog

        state.returnToLive()
        state.returnToLive()
        await waitUntil { state.restartGRPCStreamCallCount > 0 }

        XCTAssertLessThanOrEqual(state.restartGRPCStreamCallCount, 1)
    }

    /// Waits for an outcome rather than the in-flight flag: the flag is set
    /// inside the request's own task, so it is still false when returnToLive
    /// returns and a wait on it would fall straight through.
    private func waitUntil(_ condition: () -> Bool) async {
        let deadline = Date().addingTimeInterval(5)
        while !condition() && Date() < deadline {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
    }
}

@available(macOS 15.0, *) @MainActor final class PlaybackModeMappingTests: XCTestCase {

    /// setPlaybackMode once wrote an `isLive` mirror alongside the mode, which
    /// made displayPlaybackMode guard against a disagreement that could not
    /// occur. Seekability is the one thing the mode actually decides.
    func testOnlySeekableReplayIsSeekable() {
        let cases: [(AppState.PlaybackMode, Bool)] = [
            (.unknown, false),
            (.live, false),
            (.replayNonSeekable, false),
            (.replaySeekable, true),
        ]

        for (mode, wantSeekable) in cases {
            let state = AppState()
            state.setPlaybackMode(mode)
            XCTAssertEqual(state.isSeekable, wantSeekable, "seekability for \(mode)")
            XCTAssertEqual(state.playbackMode, mode)
        }
    }

    /// Before a connection the mode is unknown; afterwards it is whatever the
    /// server reported, with nothing derived on the client's side.
    func testDisplayModeFollowsTheReportedMode() {
        let state = AppState()
        state.setPlaybackMode(.unknown)
        XCTAssertEqual(state.displayPlaybackMode, .unknown)

        state.isConnected = true
        state.setPlaybackMode(.replaySeekable)
        XCTAssertEqual(state.displayPlaybackMode, .replaySeekable)
    }
}
