import XCTest

@testable import VelocityVisualiser

/// Covers reading the playback source mode from the server instead of inferring
/// it from `isLive` and `seekable`.
///
/// The inference could not distinguish a preserved analysis grid from an
/// ordinary PCAP replay, and treated "seekable" as a synonym for "VRLOG". Mode
/// and seekability are independent axes: the source says what is playing, and
/// `seekable` says whether it can be scrubbed.
@available(macOS 15.0, *) @MainActor final class PlaybackSourceModeTests: XCTestCase {

    private func frame(sourceMode: SourceMode, isLive: Bool, seekable: Bool, recording: Bool = false)
        -> FrameBundle
    {
        var f = FrameBundle(frameID: 1, timestampNanos: 0, sensorID: "test")
        f.playbackInfo = PlaybackInfo(
            isLive: isLive, logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0,
            paused: false, currentFrameIndex: 0, totalFrames: 100, seekable: seekable,
            replayEpoch: 1, sourceMode: sourceMode, recording: recording)
        return f
    }

    func testVRLogSourceModeIsReadFromTheFrame() throws {
        let state = AppState()
        state.isConnected = true
        state.onFrameReceived(frame(sourceMode: .vrlog, isLive: false, seekable: true))

        XCTAssertEqual(state.sourceMode, .vrlog)
        XCTAssertEqual(state.displayModeLabel, "REPLAY (VRLOG)")
        XCTAssertTrue(state.isSeekable)
    }

    /// The case the inference could not express: a finished analysis replay
    /// with the grid retained looked identical to an ordinary PCAP replay.
    func testAnalysisModeIsDistinguishableFromPlainPCAP() throws {
        let analysis = AppState()
        analysis.isConnected = true
        analysis.onFrameReceived(
            frame(sourceMode: .pcapAnalysis, isLive: false, seekable: false))

        let plain = AppState()
        plain.isConnected = true
        plain.onFrameReceived(frame(sourceMode: .pcap, isLive: false, seekable: false))

        XCTAssertEqual(analysis.displayModeLabel, "PCAP (ANALYSIS)")
        XCTAssertEqual(plain.displayModeLabel, "REPLAY (PCAP)")
        XCTAssertNotEqual(analysis.displayModeLabel, plain.displayModeLabel)
    }

    func testRecordingIsReadFromTheFrame() throws {
        let state = AppState()
        state.isConnected = true
        state.onFrameReceived(
            frame(sourceMode: .pcap, isLive: false, seekable: false, recording: true))

        XCTAssertTrue(state.isRecording)
    }

    /// Regression guard for the decoupling: seek controls must stay available
    /// on a seekable replay now that the mode no longer implies seekability.
    func testSeekSliderStaysEnabledOnVRLogReplay() throws {
        let state = AppState()
        state.isConnected = true
        state.onFrameReceived(frame(sourceMode: .vrlog, isLive: false, seekable: true))

        XCTAssertTrue(state.isSeekable)
        XCTAssertTrue(state.hasFrameIndexProgress)
        XCTAssertTrue(
            state.canInteractWithSeekSlider,
            "Seek controls must remain available on a seekable VRLOG replay")
    }

    /// A non-seekable source must not gain seek controls just because the
    /// server named a replay mode.
    func testSeekSliderDisabledOnNonSeekablePCAPReplay() throws {
        let state = AppState()
        state.isConnected = true
        state.onFrameReceived(frame(sourceMode: .pcap, isLive: false, seekable: false))

        XCTAssertFalse(state.isSeekable)
        XCTAssertFalse(state.canInteractWithSeekSlider)
    }

    /// Servers predating the field leave the client on its previous behaviour.
    func testUnspecifiedSourceModeFallsBackToInference() throws {
        let live = AppState()
        live.isConnected = true
        live.onFrameReceived(frame(sourceMode: .unspecified, isLive: true, seekable: false))
        XCTAssertEqual(live.displayModeLabel, "LIVE")

        let seekableReplay = AppState()
        seekableReplay.isConnected = true
        seekableReplay.onFrameReceived(
            frame(sourceMode: .unspecified, isLive: false, seekable: true))
        XCTAssertEqual(seekableReplay.displayModeLabel, "REPLAY (VRLOG)")
        XCTAssertTrue(seekableReplay.isSeekable)

        let nonSeekableReplay = AppState()
        nonSeekableReplay.isConnected = true
        nonSeekableReplay.onFrameReceived(
            frame(sourceMode: .unspecified, isLive: false, seekable: false))
        XCTAssertEqual(nonSeekableReplay.displayModeLabel, "REPLAY (PCAP)")
    }

    func testLiveSourceModeReportsLive() throws {
        let state = AppState()
        state.isConnected = true
        state.onFrameReceived(frame(sourceMode: .live, isLive: true, seekable: false))

        XCTAssertEqual(state.sourceMode, .live)
        XCTAssertEqual(state.displayModeLabel, "LIVE")
        XCTAssertEqual(state.displayPlaybackMode, .live)
    }
}
