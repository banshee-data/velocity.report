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

    /// An older server sends no source_mode. Fall back to the transport's own
    /// live flag rather than claiming replay.
    func testUnspecifiedSourceFallsBackToTheTransportFlag() {
        let state = AppState()
        state.sourceMode = .unspecified

        state.isLive = true
        XCTAssertTrue(state.isLiveSource)

        state.isLive = false
        XCTAssertFalse(state.isLiveSource)
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
