//
//  LiveToggleTests.swift
//  VelocityVisualiserTests
//
//  The Live toggle is the only thing that takes the pipeline off a recording.
//  Loading a PCAP or VRLOG turns it off, and it stays off for as long as that
//  recording is loaded — including after playback reaches the end. The server
//  no longer infers when to abandon a replay, because every rule for inferring
//  it surprised somebody: deciding from the analysis flag stranded
//  settle-before-recording runs, and deciding from live sensor presence reset
//  the grid under an operator who was still reading the replay.
//

import XCTest

@testable import VelocityVisualiser

@available(macOS 15.0, *) @MainActor final class LiveToggleTests: XCTestCase {

    func testLiveSourceIsOnForLiveInput() {
        let state = AppState()
        state.sourceMode = .live
        XCTAssertTrue(state.isLiveSource)
    }

    func testLiveSourceIsOffForEveryRecordingSource() {
        let state = AppState()

        for mode in [SourceMode.pcap, .pcapAnalysis, .vrlog] {
            state.sourceMode = mode
            XCTAssertFalse(
                state.isLiveSource,
                "source \(mode.rawValue) should read as replay, not live")
        }
    }

    /// A replay that has played to its end still holds the pipeline. The toggle
    /// must stay off so the operator sees that live input is not running.
    func testLiveSourceStaysOffWhenAReplayFinishes() {
        let state = AppState()
        state.sourceMode = .vrlog
        state.replayFinished = true

        XCTAssertFalse(
            state.isLiveSource,
            "a finished replay still owns the pipeline; the toggle must not flip itself back to live")
    }

    /// Before the first frame there is no source. It must not read as live:
    /// claiming live input the server has not reported is the confusion the
    /// source mode was introduced to end.
    func testUnspecifiedSourceIsNotLive() {
        let state = AppState()
        state.sourceMode = .unspecified

        XCTAssertFalse(state.isLiveSource)
    }

    /// The control offers exactly one transition: replay back to live. There is
    /// nowhere to go from live, because a recording has to be loaded from the
    /// run browser first — so the segments must not invite the move.
    func testSourceSegmentsCoverBothStates() {
        XCTAssertEqual(
            LiveToggleView.Source.allCases.map(\.rawValue), ["Live", "Replay"],
            "the segmented control must offer both states so the current one reads as selected")
    }

    /// Guard against a second request while one is in flight.
    func testReturnToLiveIgnoresASecondPressWhileInFlight() {
        let state = AppState()
        state.sourceMode = .vrlog
        state.isReturningToLive = true

        state.returnToLive()

        XCTAssertTrue(
            state.isReturningToLive,
            "a press during an in-flight request should be ignored, not queued")
        XCTAssertEqual(
            state.sourceMode, .vrlog,
            "the source must not change until the server confirms the switch")
    }
}

@available(macOS 15.0, *) @MainActor final class SettlingStatusTests: XCTestCase {

    /// Until the background grid settles, foreground extraction produces
    /// nothing and the scene renders empty — for about a minute after going
    /// live. Without saying so, a working sensor is indistinguishable from a
    /// dead one: the badge said LIVE over an empty view.
    func testSettlingOutranksTheSourceInTheStatusBadge() {
        let state = AppState()
        state.isConnected = true
        state.sourceMode = .live
        state.isSettling = true
        state.settlingElapsedSeconds = 4.2

        XCTAssertEqual(state.displayModeLabel, "SETTLING 4s")
    }

    func testBadgeReturnsToTheSourceOnceSettled() {
        let state = AppState()
        state.isConnected = true
        state.sourceMode = .live
        state.isSettling = true
        state.settlingElapsedSeconds = 5.9

        state.isSettling = false
        XCTAssertEqual(state.displayModeLabel, "LIVE")
    }

    /// A replay has its own settled grid recorded with it, so the badge should
    /// name the recording rather than a warm-up that is not happening.
    func testReplayBadgeIsUnaffectedWhenNotSettling() {
        let state = AppState()
        state.isConnected = true
        state.sourceMode = .vrlog
        state.isSettling = false

        XCTAssertEqual(state.displayModeLabel, "REPLAY (VRLOG)")
    }

    func testSettlingElapsedIsShownInWholeSeconds() {
        let state = AppState()
        state.isConnected = true
        state.sourceMode = .live
        state.isSettling = true

        state.settlingElapsedSeconds = 0
        XCTAssertEqual(state.displayModeLabel, "SETTLING 0s")

        state.settlingElapsedSeconds = 5.918
        XCTAssertEqual(
            state.displayModeLabel, "SETTLING 6s",
            "elapsed seconds must be whole: a badge ticking tenths is noise at this cadence")

        state.settlingElapsedSeconds = 5.4
        XCTAssertEqual(state.displayModeLabel, "SETTLING 5s")
    }
}
