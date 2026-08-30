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

    private func frame(sourceMode: SourceMode, seekable: Bool, recording: Bool = false)
        -> FrameBundle
    {
        var f = FrameBundle(frameID: 1, timestampNanos: 0, sensorID: "test")
        f.playbackInfo = PlaybackInfo(
            logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0,
            paused: false, currentFrameIndex: 0, totalFrames: 100, seekable: seekable,
            replayEpoch: 1, sourceMode: sourceMode, recording: recording)
        return f
    }

    func testVRLogSourceModeIsReadFromTheFrame() throws {
        let state = AppState()
        state.isConnected = true
        state.onFrameReceived(frame(sourceMode: .vrlog, seekable: true))

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
            frame(sourceMode: .pcapAnalysis, seekable: false))

        let plain = AppState()
        plain.isConnected = true
        plain.onFrameReceived(frame(sourceMode: .pcap, seekable: false))

        XCTAssertEqual(analysis.displayModeLabel, "PCAP (ANALYSIS)")
        XCTAssertEqual(plain.displayModeLabel, "REPLAY (PCAP)")
        XCTAssertNotEqual(analysis.displayModeLabel, plain.displayModeLabel)
    }

    func testRecordingIsReadFromTheFrame() throws {
        let state = AppState()
        state.isConnected = true
        state.onFrameReceived(
            frame(sourceMode: .pcap, seekable: false, recording: true))

        XCTAssertTrue(state.isRecording)
    }

    /// Regression guard for the decoupling: seek controls must stay available
    /// on a seekable replay now that the mode no longer implies seekability.
    func testSeekSliderStaysEnabledOnVRLogReplay() throws {
        let state = AppState()
        state.isConnected = true
        state.onFrameReceived(frame(sourceMode: .vrlog, seekable: true))

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
        state.onFrameReceived(frame(sourceMode: .pcap, seekable: false))

        XCTAssertFalse(state.isSeekable)
        XCTAssertFalse(state.canInteractWithSeekSlider)
    }

    /// Seekability is an independent axis, so an unspecified source stays
    /// unspecified rather than being reconstructed from it. The inference this
    /// replaced read a seekable stream as VRLOG and a non-seekable one as PCAP,
    /// which is what could not tell a preserved analysis grid from a replay.
    func testUnspecifiedSourceModeIsNotInferredFromSeekability() throws {
        let seekable = AppState()
        seekable.isConnected = true
        seekable.onFrameReceived(frame(sourceMode: .unspecified, seekable: true))

        XCTAssertEqual(seekable.sourceMode, .unspecified)
        XCTAssertTrue(seekable.isSeekable, "seekability is still reported directly")
        XCTAssertFalse(seekable.isLiveSource)
    }

    func testLiveSourceModeReportsLive() throws {
        let state = AppState()
        state.isConnected = true
        state.onFrameReceived(frame(sourceMode: .live, seekable: false))

        XCTAssertEqual(state.sourceMode, .live)
        XCTAssertEqual(state.displayModeLabel, "LIVE")
        XCTAssertEqual(state.displayPlaybackMode, .live)
    }
}
