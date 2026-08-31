//
//  BranchBehaviourTests.swift
//  VelocityVisualiserTests
//
//  Behaviour this branch established, pinned so it is not quietly undone.
//  Each case below records something that was wrong once and is not now.
//

import XCTest

@testable import VelocityVisualiser

@available(macOS 15.0, *) @MainActor final class StatusVocabularyTests: XCTestCase {

    private func live(silent: Bool) -> AppState {
        let state = AppState()
        state.isConnected = true
        state.sourceMode = .live
        state.sensorSilent = silent
        return state
    }

    /// A sensor that has stopped still produces frames, empty ones, so the badge
    /// showed LIVE over a scene that had stopped changing.
    func testSilentLiveSourceReadsIdle() {
        XCTAssertEqual(live(silent: true).displayModeLabel, "IDLE")
    }

    func testStreamingLiveSourceReadsLive() {
        XCTAssertEqual(live(silent: false).displayModeLabel, "LIVE")
    }

    /// Idle is deliberately the same word for both waits: a live source with
    /// nothing arriving, and a connection with no source at all.
    func testBothWaitsUseTheSameWord() {
        let noSource = AppState()
        noSource.isConnected = true
        noSource.sourceMode = .unspecified

        XCTAssertEqual(noSource.displayModeLabel, live(silent: true).displayModeLabel)
    }

    /// Settling outranks silence: warm-up explains itself and resolves, where
    /// idle does not.
    func testSettlingOutranksSilence() {
        let state = live(silent: true)
        state.isSettling = true
        state.settlingElapsedSeconds = 4

        XCTAssertEqual(state.displayModeLabel, "SETTLING 04s")
    }

    /// The pill sits in a corner; a label that changes width every second draws
    /// the eye to the wrong thing.
    func testSettlingLabelKeepsItsWidth() {
        let state = AppState()
        state.isConnected = true
        state.isSettling = true

        var widths = Set<Int>()
        for seconds in [0, 5, 9, 10, 42, 99] as [Float] {
            state.settlingElapsedSeconds = seconds
            widths.insert(state.displayModeLabel.count)
        }

        XCTAssertEqual(widths.count, 1, "the settling label changes width: \(widths)")
    }

    /// Silence describes live input. A replay's packets came out of a file.
    func testReplayIsNeverIdle() {
        let state = AppState()
        state.isConnected = true
        state.sourceMode = .vrlog
        state.sensorSilent = true

        XCTAssertEqual(state.displayModeLabel, "REPLAY (VRLOG)")
    }
}

@available(macOS 15.0, *) @MainActor final class TransportControlVisibilityTests: XCTestCase {

    private func ui(mode: AppState.PlaybackMode) -> PlaybackControlsDerivedState {
        PlaybackControlsDerivedState(
            isConnected: true, mode: mode, isPaused: false, playbackRate: 1.0, busy: false,
            hasValidTimelineRange: true, hasFrameIndexProgress: true, currentFrameIndex: 0,
            totalFrames: 100)
    }

    /// Live input has one rate, so the controls were permanently inert
    /// furniture rather than something an operator could act on.
    func testRateControlsAreHiddenOnLive() {
        XCTAssertFalse(ui(mode: AppState.PlaybackMode.live).showRateControls)
        XCTAssertFalse(ui(mode: AppState.PlaybackMode.unknown).showRateControls)
    }

    func testRateControlsAreShownForReplays() {
        XCTAssertTrue(ui(mode: AppState.PlaybackMode.replaySeekable).showRateControls)
        XCTAssertTrue(ui(mode: AppState.PlaybackMode.replayNonSeekable).showRateControls)
    }

    /// The timeline and the rate controls belong to the same thing, so they
    /// appear and disappear together rather than leaving one stranded.
    func testTimelineAndRateAgree() {
        for mode in [AppState.PlaybackMode.live, .unknown, .replaySeekable, .replayNonSeekable] {
            let state = ui(mode: mode)
            XCTAssertEqual(
                state.showReplayTimeline, state.showRateControls,
                "timeline and rate disagree for \(mode)")
        }
    }
}

@available(macOS 15.0, *) @MainActor final class ViewpointPersistenceTests: XCTestCase {

    /// The camera is reset only by the R key. Changing source keeps the
    /// viewpoint, which is what makes a live scene and a replay of it
    /// comparable; a reset on every switch would throw that away.
    func testNothingResetsTheCameraButTheOperator() throws {
        let source = try String(contentsOfFile: Self.rendererPath(), encoding: .utf8)

        let calls =
            source
            .split(separator: "\n", omittingEmptySubsequences: false)
            .filter { $0.contains("resetCamera()") }
            .filter { !$0.trimmingCharacters(in: .whitespaces).hasPrefix("//") }
            .filter { !$0.contains("func resetCamera") }

        XCTAssertEqual(
            calls.count, 1,
            "resetCamera() is called \(calls.count) times; it should be the R key alone, "
                + "so a source change keeps the viewpoint: \(calls)")
        XCTAssertTrue(
            calls.first?.contains("case 15") == true
                || source.contains("case 15:  // R - Reset camera"),
            "the one reset should be the R key")
    }

    private static func rendererPath(file: String = #filePath) -> String {
        var dir = URL(fileURLWithPath: file).deletingLastPathComponent()
        for _ in 0..<6 {
            let candidate = dir.appendingPathComponent(
                "VelocityVisualiser/Rendering/MetalRenderer.swift")
            if FileManager.default.fileExists(atPath: candidate.path) { return candidate.path }
            dir = dir.deletingLastPathComponent()
        }
        return ""
    }
}
