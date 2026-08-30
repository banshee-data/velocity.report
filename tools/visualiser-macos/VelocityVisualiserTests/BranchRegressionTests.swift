//
//  BranchRegressionTests.swift
//  VelocityVisualiserTests
//
//  Regression cover for the client-side behaviour added while reworking the
//  pipeline's source model: settling arriving over the wire, the status badge
//  across every source, and the return-to-live request.
//

import XCTest

@testable import VelocityVisualiser

@available(macOS 15.0, *) @MainActor final class SettlingDeliveryTests: XCTestCase {

    private func frame(
        settling: Bool, elapsed: Float = 0,
        sourceMode: SourceMode = .live
    ) -> FrameBundle {
        var f = FrameBundle(frameID: 1, timestampNanos: 0, sensorID: "test")
        f.playbackInfo = PlaybackInfo(
            isLive: sourceMode == .live, settling: settling,
            settlingElapsedSeconds: elapsed,
            logStartNs: 0, logEndNs: 0, playbackRate: 1.0, paused: false,
            currentFrameIndex: 0, totalFrames: 0, seekable: false, replayEpoch: 0,
            sourceMode: sourceMode, recording: false)
        return f
    }

    /// Settling has to survive the frame decode, or the badge has nothing to
    /// show and an empty warm-up scene stays indistinguishable from a sensor
    /// that has stopped.
    func testSettlingIsReadFromTheFrame() {
        let state = AppState()
        state.isConnected = true

        state.onFrameReceived(frame(settling: true, elapsed: 2.5))

        XCTAssertTrue(state.isSettling)
        XCTAssertEqual(state.settlingElapsedSeconds, 2.5, accuracy: 0.0001)
        XCTAssertEqual(state.displayModeLabel, "SETTLING 2s")
    }

    /// The badge must follow settling down as well as up: once the grid is
    /// settled the source takes the badge back.
    func testSettlingClearsWhenTheServerStopsReportingIt() {
        let state = AppState()
        state.isConnected = true

        state.onFrameReceived(frame(settling: true, elapsed: 0.9))
        XCTAssertTrue(state.isSettling)

        state.onFrameReceived(frame(settling: false, elapsed: 1.0))

        XCTAssertFalse(state.isSettling)
        XCTAssertEqual(state.displayModeLabel, "LIVE")
    }

    /// A replay carries its own settled grid, so settling should not appear
    /// over a recording.
    func testReplayFramesDoNotReportSettling() {
        let state = AppState()
        state.isConnected = true

        state.onFrameReceived(frame(settling: false, elapsed: 0, sourceMode: .vrlog))

        XCTAssertFalse(state.isSettling)
        XCTAssertEqual(state.displayModeLabel, "REPLAY (VRLOG)")
    }
}

@available(macOS 15.0, *) @MainActor final class StatusBadgeTests: XCTestCase {

    /// Every source has to produce a distinct badge. The inference this
    /// replaced could not tell a preserved analysis grid from a plain PCAP
    /// replay, so a table over all of them is the guard against sliding back.
    func testEverySourceHasItsOwnBadge() {
        let cases: [(SourceMode, String)] = [
            (.live, "LIVE"),
            (.pcap, "REPLAY (PCAP)"),
            (.pcapAnalysis, "PCAP (ANALYSIS)"),
            (.vrlog, "REPLAY (VRLOG)"),
        ]

        var seen = Set<String>()
        for (mode, expected) in cases {
            let state = AppState()
            state.isConnected = true
            state.sourceMode = mode

            XCTAssertEqual(state.displayModeLabel, expected, "badge for \(mode.rawValue)")
            XCTAssertTrue(seen.insert(expected).inserted, "\(expected) is not unique to one source")
        }
    }

    /// An older server sends no source_mode. The badge falls back to the
    /// transport's own view rather than claiming a source it was not told.
    func testUnspecifiedSourceFallsBackToTheTransport() {
        let state = AppState()
        state.isConnected = true
        state.sourceMode = .unspecified
        state.setPlaybackMode(.replaySeekable)

        XCTAssertEqual(state.displayModeLabel, AppState.PlaybackMode.replaySeekable.modeLabel)
    }

    /// Before a connection there is no source to report.
    func testDisconnectedBadgeReportsConnecting() {
        let state = AppState()
        state.isConnected = false
        state.setPlaybackMode(.unknown)

        XCTAssertEqual(state.displayModeLabel, AppState.PlaybackMode.unknown.modeLabel)
    }
}

@available(macOS 15.0, *) @MainActor final class ReturnToLiveTests: XCTestCase {

    /// A client pointed at a port nothing listens on, so the request fails
    /// deterministically. Without it these tests post to whatever server is
    /// actually running and switch its source out from under the operator —
    /// which is exactly what happened the first time they were run.
    private func deadEndClient() -> RunTrackLabelAPIClient {
        RunTrackLabelAPIClient(baseURL: URL(string: "http://127.0.0.1:1")!)
    }

    /// The in-flight flag has to clear however the request ends, or a single
    /// failure leaves the segmented control disabled for the rest of the
    /// session with no way to retry.
    func testInFlightFlagClearsAfterAFailedRequest() async {
        let state = AppState()
        state.runTrackLabelClientOverride = deadEndClient()
        state.sourceMode = .vrlog

        state.returnToLive()

        let deadline = Date().addingTimeInterval(5)
        while state.isReturningToLive && Date() < deadline {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }

        XCTAssertFalse(
            state.isReturningToLive,
            "the in-flight flag survived a failed request; the control would stay disabled")
    }

    /// A failure must not pretend the switch happened. Reporting live while the
    /// server is still replaying is the class of lie this whole rework removed.
    func testAFailedRequestDoesNotClaimTheSourceChanged() async {
        let state = AppState()
        state.runTrackLabelClientOverride = deadEndClient()
        state.sourceMode = .vrlog

        state.returnToLive()

        let deadline = Date().addingTimeInterval(5)
        while state.isReturningToLive && Date() < deadline {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }

        XCTAssertEqual(
            state.sourceMode, .vrlog,
            "the source was reported live after the request failed")
        XCTAssertFalse(state.isLiveSource)
    }
}
