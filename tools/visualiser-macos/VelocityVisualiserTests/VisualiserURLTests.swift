//
//  VisualiserURLTests.swift
//  VelocityVisualiserTests
//
//  The velocityreport:// links: parsing, refusals, the inbox that holds
//  links until the app state is ready, and finding a linked run.
//

import Foundation
import XCTest

@testable import VelocityVisualiser

final class VisualiserURLParseTests: XCTestCase {
    private func parse(_ string: String) -> Result<VisualiserURL, VisualiserURL.ParseError> {
        VisualiserURL.parse(URL(string: string)!)
    }

    func testRunWithFrame() {
        XCTAssertEqual(
            parse("velocityreport://run/3f2a9c1e-7b4d-4e0a-9c55-0d1e2f3a4b5c?frame=42"),
            .success(.run(runID: "3f2a9c1e-7b4d-4e0a-9c55-0d1e2f3a4b5c", frame: 42)))
    }

    func testClipReplayRunIDAndFrameZero() {
        XCTAssertEqual(
            parse("velocityreport://run/replay-kirk0.a_1-1a2b3c4d?frame=0"),
            .success(.run(runID: "replay-kirk0.a_1-1a2b3c4d", frame: 0)))
    }

    func testMissingFrameOpensAtTheStart() {
        XCTAssertEqual(
            parse("velocityreport://run/run-001"), .success(.run(runID: "run-001", frame: nil)))
        XCTAssertEqual(
            parse("velocityreport://run/run-001/"), .success(.run(runID: "run-001", frame: nil)))
    }

    func testSchemeAndTargetAreCaseInsensitive() {
        XCTAssertEqual(
            parse("VelocityReport://RUN/run-001?frame=7"),
            .success(.run(runID: "run-001", frame: 7)))
    }

    func testOtherQueryItemsAreIgnored() {
        XCTAssertEqual(
            parse("velocityreport://run/run-001?track=t1&frame=5"),
            .success(.run(runID: "run-001", frame: 5)))
    }

    func testBadFrames() {
        for frame in ["-1", "abc", "", "1.5", "+3", "1e3", "99999999999999999999999"] {
            guard case .failure(.badFrame) = parse("velocityreport://run/run-001?frame=\(frame)")
            else { return XCTFail("frame=\(frame) was accepted") }
        }
        guard case .failure(.badFrame) = parse("velocityreport://run/run-001?frame=1&frame=2")
        else { return XCTFail("two frames were accepted") }
    }

    func testBadRunIDs() {
        for path in ["run/a/b", "run/..", "run/a%2Fb", "run/a%20b", "run/caf%C3%A9"] {
            guard case .failure(.badRunID) = parse("velocityreport://\(path)") else {
                return XCTFail("\(path) was accepted")
            }
        }
        let long = String(repeating: "a", count: VisualiserURL.maxRunIDLength + 1)
        guard case .failure(.badRunID) = parse("velocityreport://run/\(long)") else {
            return XCTFail("an over-long run ID was accepted")
        }
    }

    func testMissingRunID() {
        XCTAssertEqual(parse("velocityreport://run"), .failure(.missingRunID))
        XCTAssertEqual(parse("velocityreport://run/"), .failure(.missingRunID))
    }

    func testPackIsNotSupportedYet() {
        let result = parse("velocityreport://pack/kirk0-w3")
        XCTAssertEqual(result, .failure(.notSupportedYet("pack")))
        if case .failure(let error) = result {
            XCTAssertTrue(error.message.contains("not supported yet"), error.message)
        }
    }

    func testUnknownTargetAndScheme() {
        XCTAssertEqual(parse("velocityreport://view/abc"), .failure(.unknownTarget("view")))
        guard case .failure(.unknownTarget) = parse("velocityreport:run/run-001") else {
            return XCTFail("a link with no host was accepted")
        }
        XCTAssertEqual(parse("https://run/run-001"), .failure(.wrongScheme("https")))
    }

    func testEveryRefusalSaysSomething() {
        let errors: [VisualiserURL.ParseError] = [
            .wrongScheme("https"), .notSupportedYet("pack"), .unknownTarget("view"), .missingRunID,
            .badRunID("a/b"), .badFrame("x"),
        ]
        for error in errors { XCTAssertFalse(error.message.isEmpty) }
    }

    func testLinkRoundTrips() {
        let url = VisualiserURL.link(runID: "run-001", frame: 12)
        XCTAssertEqual(url?.absoluteString, "velocityreport://run/run-001?frame=12")
        XCTAssertEqual(url.map(VisualiserURL.parse), .success(.run(runID: "run-001", frame: 12)))
        XCTAssertEqual(
            VisualiserURL.link(runID: "run-001", frame: nil)?.absoluteString,
            "velocityreport://run/run-001")
        XCTAssertNil(VisualiserURL.link(runID: "a/b", frame: 1))
    }
}

@MainActor final class VisualiserURLInboxTests: XCTestCase {
    func testLinksWaitForTheHandler() {
        let inbox = VisualiserURLInbox()
        let first = URL(string: "velocityreport://run/a")!
        let second = URL(string: "velocityreport://run/b")!
        inbox.receive([first])
        var seen: [URL] = []
        inbox.setHandler { seen.append($0) }
        XCTAssertEqual(seen, [first])
        inbox.receive([second])
        XCTAssertEqual(seen, [first, second])
    }
}

@available(macOS 15.0, *) @MainActor final class VisualiserLinkOpeningTests: XCTestCase {
    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        super.tearDown()
    }

    func testRefusedLinkSaysWhyAndOpensNothing() async {
        let appState = AppState()
        await appState.openLink(URL(string: "velocityreport://pack/kirk0-w3")!)
        XCTAssertEqual(appState.linkAlert?.kind, .refused)
        XCTAssertEqual(appState.linkAlert?.title, "Cannot open link")
        XCTAssertTrue(appState.linkAlert?.message.contains("not supported yet") == true)
        XCTAssertNil(appState.currentRunID)
        XCTAssertNil(appState.pendingLinkFrame)
    }

    private static let runJSON = """
        {"run_id": "old-run", "created_at": "2026-02-10T20:05:09-08:00", "source_type": "vrlog",
         "source_path": null, "sensor_id": "hesai-01", "duration_secs": 1.0, "total_frames": 10,
         "total_clusters": 0, "total_tracks": 0, "confirmed_tracks": 0, "status": "completed",
         "error_message": null, "vrlog_path": "/v/old-run", "notes": null}
        """

    private static func respond(
        _ request: URLRequest, _ status: Int, _ body: String
    ) -> (HTTPURLResponse, Data) {
        (
            HTTPURLResponse(
                url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!,
            Data(body.utf8)
        )
    }

    func testFindRunAsksTheServerForARunOutsideTheList() async {
        let state = RunBrowserState(apiClient: makeMockRunTrackClient())
        MockURLProtocol.requestHandler = { request in
            if request.url!.path == "/api/lidar/runs" {
                return Self.respond(request, 200, #"{"runs": [], "count": 0}"#)
            }
            XCTAssertEqual(request.url!.path, "/api/lidar/runs/old-run")
            return Self.respond(request, 200, Self.runJSON)
        }
        let result = await state.findRun("old-run")
        guard case .success(let run) = result else { return XCTFail("\(result)") }
        XCTAssertEqual(run.runId, "old-run")
        XCTAssertEqual(state.runs.map(\.runId), ["old-run"])
    }

    func testFindRunReportsAnUnknownRun() async {
        let state = RunBrowserState(apiClient: makeMockRunTrackClient())
        MockURLProtocol.requestHandler = { request in
            if request.url!.path == "/api/lidar/runs" {
                return Self.respond(request, 200, #"{"runs": [], "count": 0}"#)
            }
            return Self.respond(request, 404, "not found")
        }
        let result = await state.findRun("missing")
        XCTAssertEqual(result.failureValue, .notFound)
        XCTAssertTrue(state.runs.isEmpty)
    }
}

/// Holds the VRLOG load request until released, so a test can act while a
/// link's load is in flight. The handler runs on URL loading's own thread.
private final class LoadGate: @unchecked Sendable {
    private let lock = NSLock()
    private let semaphore = DispatchSemaphore(value: 0)
    private var held = false
    private var _loads = 0

    init(hold: Bool) { held = hold }

    var loads: Int { lock.withLock { _loads } }

    func arrive() {
        let wait = lock.withLock { () -> Bool in
            _loads += 1
            return held
        }
        if wait { semaphore.wait() }
    }

    func release() {
        lock.withLock { held = false }
        semaphore.signal()
    }
}

/// A link opening a run against a mocked server, then the frames of the
/// replay it started arriving.
@available(macOS 15.0, *) @MainActor final class VisualiserLinkFrameTests: XCTestCase {
    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        super.tearDown()
    }

    private static let runsJSON = """
        {"runs": [{"run_id": "r1", "created_at": "2026-02-10T20:05:09-08:00",
          "source_type": "pcap", "source_path": "/p", "sensor_id": "s", "duration_secs": 1,
          "total_frames": 100, "total_clusters": 0, "total_tracks": 0, "confirmed_tracks": 0,
          "status": "completed", "error_message": null, "vrlog_path": "/v/r1", "notes": null}],
         "count": 1}
        """

    /// An app state whose run browser talks to the mock, with the playback
    /// client faked and connected.
    private func makeState(gate: LoadGate) -> (AppState, FakePlaybackRPCClient) {
        MockURLProtocol.requestHandler = { request in
            let path = request.url!.path
            let body: String
            if path == "/api/lidar/runs" {
                body = Self.runsJSON
            } else if path == "/api/lidar/vrlog/load" {
                gate.arrive()
                body = #"{"success": true, "vrlog_path": "/v/r1", "frame_encoding": "foreground"}"#
            } else {
                body = #"{"run_id": "r1", "tracks": [], "count": 0}"#
            }
            return (
                HTTPURLResponse(
                    url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!,
                Data(body.utf8)
            )
        }
        let state = AppState(runBrowserState: RunBrowserState(apiClient: makeMockRunTrackClient()))
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        return (state, fake)
    }

    /// A seekable VRLOG frame of a replay `totalFrames` long.
    private func frame(totalFrames: UInt64, index: UInt64 = 0) -> FrameBundle {
        var frame = FrameBundle()
        frame.frameID = index + 1
        frame.timestampNanos = 1_000_000_000 + Int64(index)
        frame.playbackInfo = PlaybackInfo(
            logStartNs: 0, logEndNs: 10, playbackRate: 1.0, paused: false, currentFrameIndex: index,
            totalFrames: totalFrames, seekable: true, sourceMode: .vrlog)
        return frame
    }

    private func waitFor(
        timeout: TimeInterval = 3.0, _ condition: @escaping @MainActor () -> Bool,
        file: StaticString = #filePath, line: UInt = #line
    ) async throws {
        let deadline = ContinuousClock.now + .seconds(timeout)
        while !condition() {
            guard ContinuousClock.now < deadline else {
                XCTFail("Timed out waiting for condition", file: file, line: line)
                return
            }
            try await Task.sleep(for: .milliseconds(10))
        }
    }

    func testLinkPausesOnItsFrameOnceTheNewReplayIsSeekable() async throws {
        let (state, fake) = makeState(gate: LoadGate(hold: false))
        await state.openLink(URL(string: "velocityreport://run/r1?frame=42")!)
        XCTAssertNil(state.linkAlert)
        XCTAssertEqual(state.currentRunID, "r1")
        XCTAssertEqual(
            state.pendingLinkFrame,
            AppState.PendingLinkFrame(
                runID: "r1", frame: 42, generation: state.playbackGenerationForTesting))

        state.onFrameReceived(frame(totalFrames: 100))
        try await waitFor { fake.seekFrameCalls == [42] && state.inFlightPlaybackCommand == nil }
        XCTAssertNil(state.pendingLinkFrame)
        XCTAssertEqual(fake.pauseCallCount, 1)
        XCTAssertNil(state.linkAlert)
    }

    func testFramesOfTheReplayPlayingDuringTheLoadAreNotTheLinks() async throws {
        // A replay of the same run is already playing, seekable, when the
        // link arrives: its frames keep coming until the load returns.
        let gate = LoadGate(hold: true)
        let (state, fake) = makeState(gate: gate)
        state.currentRunID = "r1"
        let opening = Task {
            await state.openLink(URL(string: "velocityreport://run/r1?frame=42")!)
        }
        try await waitFor { gate.loads == 1 }

        state.onFrameReceived(frame(totalFrames: 100, index: 7))
        try await Task.sleep(for: .milliseconds(50))
        XCTAssertTrue(fake.seekFrameCalls.isEmpty, "the old replay was sought")
        XCTAssertNil(state.pendingLinkFrame)

        gate.release()
        await opening.value
        state.onFrameReceived(frame(totalFrames: 100))
        try await waitFor { fake.seekFrameCalls == [42] }
    }

    func testAnotherReplayStartedFirstDropsTheFrame() async throws {
        let (state, fake) = makeState(gate: LoadGate(hold: false))
        await state.openLink(URL(string: "velocityreport://run/r1?frame=42")!)
        XCTAssertNotNil(state.pendingLinkFrame)
        state.restartGRPCStream()  // as opening another run does
        state.onFrameReceived(frame(totalFrames: 100))
        try await waitFor { state.pendingLinkFrame == nil }
        try await Task.sleep(for: .milliseconds(50))
        XCTAssertTrue(fake.seekFrameCalls.isEmpty)
    }

    func testASecondLinkToTheLoadingRunReplacesTheFrame() async throws {
        let gate = LoadGate(hold: true)
        let (state, fake) = makeState(gate: gate)
        state.runBrowserState.error = "Failed to fetch runs: an earlier failure"
        let first = Task { await state.openLink(URL(string: "velocityreport://run/r1?frame=10")!) }
        try await waitFor { gate.loads == 1 }

        await state.openLink(URL(string: "velocityreport://run/r1?frame=42")!)
        XCTAssertNil(state.linkAlert, "the second link failed")

        gate.release()
        await first.value
        XCTAssertNil(state.linkAlert)
        XCTAssertNil(state.runBrowserState.error)
        XCTAssertEqual(state.pendingLinkFrame?.frame, 42)
        XCTAssertEqual(gate.loads, 1)

        state.onFrameReceived(frame(totalFrames: 100))
        try await waitFor { fake.seekFrameCalls == [42] }
    }

    func testALinkToAnotherRunWhileOneLoadsIsRefused() async throws {
        let gate = LoadGate(hold: true)
        let (state, _) = makeState(gate: gate)
        let first = Task { await state.openLink(URL(string: "velocityreport://run/r1?frame=10")!) }
        try await waitFor { gate.loads == 1 }
        await state.openLink(URL(string: "velocityreport://run/r2?frame=3")!)
        XCTAssertEqual(state.linkAlert?.kind, .refused)
        XCTAssertTrue(state.linkAlert?.message.contains("still loading") == true)
        gate.release()
        await first.value
        XCTAssertEqual(state.currentRunID, "r1")
    }

    func testAFramePastTheEndShowsTheLastWithANotice() async throws {
        let (state, fake) = makeState(gate: LoadGate(hold: false))
        await state.openLink(URL(string: "velocityreport://run/r1?frame=500")!)
        state.onFrameReceived(frame(totalFrames: 100))
        try await waitFor { fake.seekFrameCalls == [99] }
        XCTAssertEqual(state.linkAlert?.kind, .notice)
        XCTAssertEqual(state.linkAlert?.title, "Link opened")
        XCTAssertTrue(state.linkAlert?.message.contains("100 frames") == true)
    }

    func testNoFrameOpensAtTheStartWithoutSeeking() async throws {
        let (state, fake) = makeState(gate: LoadGate(hold: false))
        await state.openLink(URL(string: "velocityreport://run/r1")!)
        XCTAssertEqual(state.currentRunID, "r1")
        XCTAssertNil(state.pendingLinkFrame)
        state.onFrameReceived(frame(totalFrames: 100))
        try await Task.sleep(for: .milliseconds(50))
        XCTAssertTrue(fake.seekFrameCalls.isEmpty)
    }

    func testShowFrameClampsAndNeedsASeekableReplay() async throws {
        let (state, fake) = makeState(gate: LoadGate(hold: false))
        state.showFrame(3)
        XCTAssertTrue(fake.seekFrameCalls.isEmpty, "sought with no replay")
        state.setPlaybackModeForTesting(.replaySeekable)
        state.totalFrames = 10
        state.showFrame(20)
        try await waitFor { fake.seekFrameCalls == [9] && state.inFlightPlaybackCommand == nil }
        XCTAssertTrue(state.isPaused)
    }
}

extension Result {
    fileprivate var failureValue: Failure? {
        if case .failure(let error) = self { return error }
        return nil
    }
}
