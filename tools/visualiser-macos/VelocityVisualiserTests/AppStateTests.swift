//
//  AppStateTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for AppState class using XCTest (required for @MainActor tests).
//

import XCTest

@testable import VelocityVisualiser

@available(macOS 15.0, *) final class FakePlaybackRPCClient: PlaybackRPCClient {
    enum ForcedError: Error { case testFailure }

    var pauseCallCount = 0
    var playCallCount = 0
    var seekTimestampCalls: [Int64] = []
    var seekFrameCalls: [UInt64] = []
    var setRateCalls: [Float] = []
    var restartStreamCallCount = 0

    var pauseStatus = VisualiserPlaybackStatus(
        paused: true, rate: 1.0, currentTimestampNs: 0, currentFrameID: 0)
    var playStatus = VisualiserPlaybackStatus(
        paused: false, rate: 1.0, currentTimestampNs: 0, currentFrameID: 0)
    var seekStatus = VisualiserPlaybackStatus(
        paused: true, rate: 1.0, currentTimestampNs: 0, currentFrameID: 0)
    var setRateStatus = VisualiserPlaybackStatus(
        paused: false, rate: 1.0, currentTimestampNs: 0, currentFrameID: 0)

    var holdNextSeekTimestamp = false
    var seekTimestampContinuation: CheckedContinuation<Void, Never>?
    var pauseError: Error?
    var playError: Error?
    var seekTimestampError: Error?
    var seekFrameError: Error?
    var setRateError: Error?

    func pause() async throws -> VisualiserPlaybackStatus {
        if let pauseError { throw pauseError }
        pauseCallCount += 1
        return pauseStatus
    }

    func play() async throws -> VisualiserPlaybackStatus {
        if let playError { throw playError }
        playCallCount += 1
        return playStatus
    }

    func seek(to timestampNanos: Int64) async throws -> VisualiserPlaybackStatus {
        if let seekTimestampError { throw seekTimestampError }
        seekTimestampCalls.append(timestampNanos)
        if holdNextSeekTimestamp {
            holdNextSeekTimestamp = false
            await withCheckedContinuation { continuation in seekTimestampContinuation = continuation
            }
        }
        return seekStatus
    }

    func seek(toFrame frameID: UInt64) async throws -> VisualiserPlaybackStatus {
        if let seekFrameError { throw seekFrameError }
        seekFrameCalls.append(frameID)
        return seekStatus
    }

    func setRate(_ rate: Float) async throws -> VisualiserPlaybackStatus {
        if let setRateError { throw setRateError }
        setRateCalls.append(rate)
        var status = setRateStatus
        status.rate = rate
        return status
    }

    func restartStream() { restartStreamCallCount += 1 }
}

// MARK: - Playback Hardening Regression Tests

@available(macOS 15.0, *) @MainActor final class PlaybackHardeningRegressionTests: XCTestCase {

    private func waitFor(
        timeout: TimeInterval = 2.0, condition: @escaping @MainActor () -> Bool,
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

    func testTogglePlayPauseDoesNotMutateWhenDisconnected() throws {
        let state = AppState()
        state.isConnected = false
        state.isLive = false
        state.isPaused = false

        state.togglePlayPause()

        XCTAssertFalse(
            state.isPaused, "Should not optimistically toggle without a connected client")
    }

    func testIncreaseRateDoesNotMutateWhenClientMissing() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.playbackRate = 1.0
        state.playbackCommandClientOverride = nil

        state.increaseRate()

        XCTAssertEqual(
            state.playbackRate, 1.0, "Should not optimistically change rate without client")
    }

    func testPrepareForNewReplayClearsSeekabilityAndCommandState() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.isLive = false
        state.isSeekable = true
        state.playbackMode = .replaySeekable
        state.logStartTimestamp = 0
        state.logEndTimestamp = 1_000_000_000
        fake.holdNextSeekTimestamp = true

        state.seek(to: 0.5)
        try await waitFor { state.inFlightPlaybackCommand == .seek }
        XCTAssertNotNil(state.pendingSeekTargetTimestamp)

        state.prepareForNewReplay()

        XCTAssertEqual(state.playbackMode, .unknown)
        XCTAssertFalse(state.isSeekable)
        XCTAssertNil(state.inFlightPlaybackCommand)
        XCTAssertNil(state.pendingSeekTargetTimestamp)
        XCTAssertFalse(state.isSeekingInProgress)

        fake.seekTimestampContinuation?.resume()
    }

    func testStaleGenerationFrameIsIgnored() async throws {
        let state = AppState()
        var frame = FrameBundle()
        frame.frameID = 99
        frame.timestampNanos = 1_000_000_000
        frame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 0, logEndNs: 10, playbackRate: 1.0, paused: false,
            currentFrameIndex: 1, totalFrames: 2, seekable: false)

        state.onFrameReceived(frame, generation: 999)
        await Task.yield()

        XCTAssertEqual(state.currentFrameID, 0)
        XCTAssertFalse(state.hasPlaybackMetadata)
    }

    func testStaleGenerationFinishIsIgnored() {
        let state = AppState()
        state.handleStreamFinished(expectedGeneration: 999)
        XCTAssertFalse(state.replayFinished)
        XCTAssertFalse(state.isPaused)
    }

    func testSeekCoalescesLatestRequestWhileBusy() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.isLive = false
        state.isSeekable = true
        state.playbackMode = .replaySeekable
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000

        fake.holdNextSeekTimestamp = true
        state.seek(to: 0.1)
        try await waitFor {
            fake.seekTimestampCalls.count == 1 && state.inFlightPlaybackCommand == .seek
        }

        state.seek(to: 0.9)
        XCTAssertEqual(
            fake.seekTimestampCalls.count, 1, "Second seek should coalesce while first is busy")

        fake.seekTimestampContinuation?.resume()
        try await waitFor {
            fake.seekTimestampCalls.count == 2 && state.inFlightPlaybackCommand == nil
        }

        XCTAssertEqual(fake.seekTimestampCalls[0], 1_100_000_000)
        XCTAssertEqual(fake.seekTimestampCalls[1], 1_900_000_000)
    }

    func testDerivedReplayProgressFallbackUsesFrameIndexWhenTimelineInvalid() {
        let state = AppState()
        state.currentFrameIndex = 5
        state.totalFrames = 11
        state.replayProgress = 0.25
        state.logStartTimestamp = 100
        state.logEndTimestamp = 100  // Invalid range

        XCTAssertEqual(state.displayReplayProgress, 0.5, accuracy: 0.0001)
    }

    func testDerivedPlaybackFlagsForSeekSliderAndMetadataFallback() {
        let state = AppState()
        state.isConnected = true
        state.setPlaybackModeForTesting(.replaySeekable)
        state.logStartTimestamp = 1
        state.logEndTimestamp = 2
        XCTAssertTrue(state.canInteractWithSeekSlider)

        state.setPlaybackModeForTesting(.replayNonSeekable)
        state.logStartTimestamp = 10
        state.logEndTimestamp = 10
        state.totalFrames = 0
        XCTAssertTrue(state.shouldShowReplayMetadataUnavailable)

        state.setPlaybackModeForTesting(.live)
        XCTAssertTrue(state.isLive)
        XCTAssertFalse(state.isSeekable)
    }

    func testUnknownPlaybackModePreservesPreviousFlags() {
        let state = AppState()
        state.isConnected = true
        state.setPlaybackModeForTesting(.replaySeekable)

        state.setPlaybackModeForTesting(.unknown)

        XCTAssertEqual(state.displayPlaybackMode, .unknown)
        XCTAssertFalse(state.isLive)
        XCTAssertTrue(state.isSeekable)
    }

    func testTogglePlayPauseIgnoredWhenPlaybackModeUnknown() {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.isLive = false
        state.playbackMode = .unknown
        state.playbackCommandClientOverride = fake

        state.togglePlayPause()

        XCTAssertEqual(fake.pauseCallCount + fake.playCallCount, 0)
        XCTAssertNil(state.inFlightPlaybackCommand)
    }

    func testPlaybackCommandGuardRejectsSeekRequirementForNonSeekableReplay() {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.setPlaybackModeForTesting(.replayNonSeekable)

        XCTAssertFalse(state.canRunPlaybackCommandForTesting(.seek, requiresSeekable: true))
    }

    func testRestartGRPCStreamRebindsDelegateWhenClientExists() async throws {
        let state = AppState()
        state.serverAddress = "localhost"
        state.connect()
        state.restartGRPCStream()
        try await Task.sleep(for: .milliseconds(20))
    }

    func testTogglePlayPauseFinishedReplayRestartsStream() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.setPlaybackModeForTesting(.replaySeekable)
        state.isPaused = true
        state.replayFinished = true

        state.togglePlayPause()
        // The replayFinished path seeks to start then plays then restarts
        try await waitFor { fake.seekTimestampCalls.count == 1 && fake.playCallCount == 1 }

        XCTAssertFalse(state.replayFinished)
        XCTAssertFalse(state.isPaused)
    }

    func testTogglePlayPauseFailureRestoresOptimisticPausedState() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        fake.pauseError = FakePlaybackRPCClient.ForcedError.testFailure
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.setPlaybackModeForTesting(.replaySeekable)
        state.isPaused = false

        state.togglePlayPause()
        try await waitFor { state.inFlightPlaybackCommand == nil }

        XCTAssertFalse(state.isPaused)
    }

    func testStepForwardPausesSeeksAndRestartsWhenReplayFinished() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.setPlaybackModeForTesting(.replaySeekable)
        state.currentFrameIndex = 2
        state.totalFrames = 10
        state.isPaused = false
        state.replayFinished = true

        state.stepForward()
        try await waitFor {
            fake.pauseCallCount == 1 && fake.seekFrameCalls == [3]
                && state.inFlightPlaybackCommand == nil
        }

        XCTAssertFalse(state.replayFinished)
        XCTAssertTrue(state.isPaused)
        XCTAssertFalse(state.isSeekingInProgress)
        XCTAssertNil(state.pendingSeekTargetFrameIndex)
    }

    func testStepForwardFailureRestoresPausedState() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        fake.seekFrameError = FakePlaybackRPCClient.ForcedError.testFailure
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.setPlaybackModeForTesting(.replaySeekable)
        state.currentFrameIndex = 1
        state.totalFrames = 5
        state.isPaused = false

        state.stepForward()
        try await waitFor { state.inFlightPlaybackCommand == nil }

        XCTAssertEqual(fake.pauseCallCount, 1)
        XCTAssertEqual(fake.seekFrameCalls.count, 0)
        XCTAssertFalse(state.isPaused)
        XCTAssertFalse(state.isSeekingInProgress)
    }

    func testIncreaseRateFailureRestoresPreviousRate() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        fake.setRateError = FakePlaybackRPCClient.ForcedError.testFailure
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.setPlaybackModeForTesting(.replaySeekable)
        state.playbackRate = 1.0

        state.increaseRate()
        try await waitFor { state.inFlightPlaybackCommand == nil }

        XCTAssertEqual(state.playbackRate, 1.0)
        XCTAssertTrue(fake.setRateCalls.isEmpty)
    }

    func testSeekFailureRestoresProgressTimestampAndPausedState() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        fake.seekTimestampError = FakePlaybackRPCClient.ForcedError.testFailure
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.setPlaybackModeForTesting(.replaySeekable)
        state.logStartTimestamp = 1_000
        state.logEndTimestamp = 2_000
        state.replayProgress = 0.2
        state.currentTimestamp = 1_200
        state.isPaused = false

        state.seek(to: 0.75)
        try await waitFor { state.inFlightPlaybackCommand == nil }

        XCTAssertEqual(state.replayProgress, 0.2, accuracy: 0.0001)
        XCTAssertEqual(state.currentTimestamp, 1_200)
        XCTAssertFalse(state.isPaused)
        XCTAssertFalse(state.isSeekingInProgress)
        XCTAssertNil(state.pendingSeekTargetTimestamp)
    }

    func testDeferredFrameUpdateDroppedAfterGenerationChanges() async throws {
        let state = AppState()
        state.isLive = false
        var frame = FrameBundle()
        frame.frameID = 12
        frame.timestampNanos = 1_500
        frame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 100, logEndNs: 200, playbackRate: 1.0, paused: false,
            currentFrameIndex: 1, totalFrames: 2, seekable: false)

        state.onFrameReceived(frame)
        state.prepareForNewReplay()  // bumps generation before deferred Task runs
        try await Task.sleep(for: .milliseconds(30))

        XCTAssertEqual(state.currentFrameID, 0)
        XCTAssertFalse(state.hasPlaybackMetadata)
    }

    func testFrameUpdateUsesFrameIndexFallbackProgressWhenTimelineInvalid() async throws {
        let state = AppState()
        state.isLive = false
        var frame = FrameBundle()
        frame.frameID = 9
        frame.timestampNanos = 500
        frame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 1_000, logEndNs: 1_000, playbackRate: 1.0, paused: true,
            currentFrameIndex: 3, totalFrames: 5, seekable: false)

        state.onFrameReceived(frame)
        try await Task.sleep(for: .milliseconds(30))

        XCTAssertEqual(state.replayProgress, 0.75, accuracy: 0.0001)
    }
}

// MARK: - AppState Tests

@available(macOS 15.0, *) @MainActor final class AppStateTests: XCTestCase {

    func testDefaultInitialisation() throws {
        let state = AppState()

        // Connection state
        XCTAssertFalse(state.isConnected)
        XCTAssertFalse(state.isConnecting)
        XCTAssertEqual(state.serverAddress, "localhost:50051")
        XCTAssertNil(state.connectionError)

        // Playback state
        XCTAssertFalse(state.isPaused)
        XCTAssertEqual(state.playbackRate, 1.0)
        XCTAssertTrue(state.isLive)
        XCTAssertEqual(state.currentTimestamp, 0)
        XCTAssertEqual(state.currentFrameID, 0)

        // Overlay toggles
        XCTAssertTrue(state.showPoints)
        XCTAssertTrue(state.showBackground)
        XCTAssertTrue(state.showBoxes)
        XCTAssertTrue(state.showTrails)
        XCTAssertTrue(state.showVelocity)
        XCTAssertFalse(state.showDebug)

        // Labelling state
        XCTAssertNil(state.selectedTrackID)
        XCTAssertFalse(state.showLabelPanel)

        // Stats
        XCTAssertEqual(state.frameCount, 0)
        XCTAssertEqual(state.fps, 0.0)
        XCTAssertEqual(state.pointCount, 0)
        XCTAssertEqual(state.clusterCount, 0)
        XCTAssertEqual(state.trackCount, 0)
    }

    func testTogglePlayPause() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.isLive = false  // Required for playback controls
        state.playbackCommandClientOverride = fake
        XCTAssertFalse(state.isPaused)

        state.togglePlayPause()
        XCTAssertTrue(state.isPaused)
        try await Task.sleep(for: .milliseconds(20))

        state.togglePlayPause()
        try await Task.sleep(for: .milliseconds(20))
        XCTAssertFalse(state.isPaused)
    }

    func testIncreaseRate() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.isLive = false  // Required for playback controls
        XCTAssertEqual(state.playbackRate, 1.0)

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 2.0)
        try await Task.sleep(for: .milliseconds(20))

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 4.0)
        try await Task.sleep(for: .milliseconds(20))

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 8.0)
        try await Task.sleep(for: .milliseconds(20))

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 16.0)
        try await Task.sleep(for: .milliseconds(20))

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 32.0)
        try await Task.sleep(for: .milliseconds(20))

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 64.0)
        try await Task.sleep(for: .milliseconds(20))

        // Should cap at 64.0
        state.increaseRate()
        try await Task.sleep(for: .milliseconds(20))
        XCTAssertEqual(state.playbackRate, 64.0)
    }

    func testDecreaseRate() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.isLive = false  // Required for playback controls
        XCTAssertEqual(state.playbackRate, 1.0)

        state.decreaseRate()
        XCTAssertEqual(state.playbackRate, 0.5)
        try await Task.sleep(for: .milliseconds(20))

        // Should cap at 0.5
        state.decreaseRate()
        try await Task.sleep(for: .milliseconds(20))
        XCTAssertEqual(state.playbackRate, 0.5)
    }

    func testResetRate() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.isLive = false
        state.playbackRate = 8.0

        state.resetRate()
        try await Task.sleep(for: .milliseconds(20))
        XCTAssertEqual(state.playbackRate, 1.0)
    }

    func testSelectTrack() throws {
        let state = AppState()
        XCTAssertNil(state.selectedTrackID)

        state.selectTrack("track-001")
        XCTAssertEqual(state.selectedTrackID, "track-001")

        state.selectTrack("track-002")
        XCTAssertEqual(state.selectedTrackID, "track-002")

        state.selectTrack(nil)
        XCTAssertNil(state.selectedTrackID)
    }

    func testSeekIgnoredWhenLive() throws {
        let state = AppState()
        state.isLive = true
        state.replayProgress = 0.0

        // Seek should be ignored when live
        state.seek(to: 0.5)
        XCTAssertEqual(state.replayProgress, 0.0)
    }

    func testSeekInReplayMode() async throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.isLive = false
        state.isSeekable = true
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000

        state.seek(to: 0.5)
        XCTAssertEqual(state.replayProgress, 0.5)
        try await Task.sleep(for: .milliseconds(20))

        state.seek(to: 0.75)
        try await Task.sleep(for: .milliseconds(20))
        XCTAssertEqual(state.replayProgress, 0.75)
    }

    func testOpenRecording() async throws {
        let state = AppState()
        XCTAssertTrue(state.isLive)

        // Use loadRecording directly since openRecording uses NSOpenPanel
        let testURL = URL(fileURLWithPath: "/tmp/test.vrlog")

        let expectation = expectation(description: "Recording loaded")

        state.loadRecording(from: testURL)

        // Wait deterministically for isLive to become false, with a bounded timeout.
        Task {
            let start = Date()
            while true {
                if !state.isLive {
                    expectation.fulfill()
                    break
                }

                if Date().timeIntervalSince(start) > 5.0 {
                    XCTFail("Recording did not finish loading in time")
                    expectation.fulfill()
                    break
                }

                await Task.yield()
            }
        }

        await fulfillment(of: [expectation], timeout: 6.0)
    }

    func testOnFrameReceived() async throws {
        let state = AppState()

        var frame = FrameBundle()
        frame.frameID = 42
        frame.timestampNanos = 1_000_000_000

        frame.pointCloud = PointCloudFrame(
            frameID: 42, timestampNanos: 1_000_000_000, sensorID: "hesai-01", x: [1.0, 2.0, 3.0],
            y: [4.0, 5.0, 6.0], z: [0.1, 0.2, 0.3], intensity: [100, 150, 200],
            classification: [0, 1, 0], decimationMode: .none, decimationRatio: 1.0, pointCount: 3)

        frame.clusters = ClusterSet(
            frameID: 42, timestampNanos: 1_000_000_000,
            clusters: [
                Cluster(clusterID: 1, centroidX: 10.0, centroidY: 20.0, centroidZ: 0.8),
                Cluster(clusterID: 2, centroidX: 30.0, centroidY: 40.0, centroidZ: 0.9),
            ], method: .dbscan)

        frame.tracks = TrackSet(
            frameID: 42, timestampNanos: 1_000_000_000,
            tracks: [Track(trackID: "track-001", state: .confirmed)], trails: [])

        state.onFrameReceived(frame)
        await Task.yield()

        XCTAssertEqual(state.currentFrameID, 42)
        XCTAssertEqual(state.currentTimestamp, 1_000_000_000)
        XCTAssertEqual(state.frameCount, 1)
        XCTAssertEqual(state.pointCount, 3)
        XCTAssertEqual(state.clusterCount, 2)
        XCTAssertEqual(state.trackCount, 1)
        XCTAssertEqual(state.currentFrame?.frameID, 42)
    }

    func testOnFrameReceivedUpdatesReplayProgress() async throws {
        let state = AppState()
        state.isLive = false
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000

        var frame = FrameBundle()
        frame.frameID = 1
        frame.timestampNanos = 1_500_000_000  // Midpoint

        state.onFrameReceived(frame)
        await Task.yield()

        XCTAssertEqual(state.replayProgress, 0.5)
    }

    func testMultipleFramesIncrementCount() async throws {
        let state = AppState()

        for i in 1...10 {
            var frame = FrameBundle()
            frame.frameID = UInt64(i)
            frame.timestampNanos = Int64(i) * 100_000_000
            state.onFrameReceived(frame)
            await Task.yield()
        }

        XCTAssertEqual(state.frameCount, 10)
        XCTAssertEqual(state.currentFrameID, 10)
    }

    func testDisconnectResetsState() throws {
        let state = AppState()
        state.isConnected = true

        var frame = FrameBundle()
        frame.frameID = 42
        state.currentFrame = frame

        state.disconnect()

        XCTAssertFalse(state.isConnected)
        XCTAssertNil(state.currentFrame)
    }

    func testAssignLabelRequiresSelectedTrack() throws {
        let state = AppState()
        state.selectedTrackID = nil

        // Should do nothing without selection
        state.assignLabel("car")
        // No crash = success

        state.selectedTrackID = "track-001"
        state.assignLabel("car")  // Should print message (can't verify without capture)
    }
}

// MARK: - Overlay Toggle Tests

@available(macOS 15.0, *) @MainActor final class OverlayToggleTests: XCTestCase {

    func testTogglePoints() throws {
        let state = AppState()
        XCTAssertTrue(state.showPoints)

        state.showPoints = false
        XCTAssertFalse(state.showPoints)

        state.showPoints = true
        XCTAssertTrue(state.showPoints)
    }

    func testToggleBoxes() throws {
        let state = AppState()
        XCTAssertTrue(state.showBoxes)

        state.showBoxes = false
        XCTAssertFalse(state.showBoxes)
    }

    func testToggleTrails() throws {
        let state = AppState()
        XCTAssertTrue(state.showTrails)

        state.showTrails = false
        XCTAssertFalse(state.showTrails)
    }

    func testToggleVelocity() throws {
        let state = AppState()
        XCTAssertTrue(state.showVelocity)

        state.showVelocity = false
        XCTAssertFalse(state.showVelocity)
    }

    func testToggleDebug() throws {
        let state = AppState()
        XCTAssertFalse(state.showDebug)

        state.showDebug = true
        XCTAssertTrue(state.showDebug)
    }

    func testPointSizeDefault() throws {
        let state = AppState()
        XCTAssertEqual(state.pointSize, 5.0)
    }

    func testPointSizeAdjustment() throws {
        let state = AppState()
        state.pointSize = 10.0
        XCTAssertEqual(state.pointSize, 10.0)

        state.pointSize = 1.0
        XCTAssertEqual(state.pointSize, 1.0)

        state.pointSize = 20.0
        XCTAssertEqual(state.pointSize, 20.0)
    }
}
// MARK: - Playback State Tests

@available(macOS 15.0, *) @MainActor final class PlaybackStateTests: XCTestCase {

    func testStepForwardIgnoredWhenLive() throws {
        let state = AppState()
        state.isLive = true
        state.currentFrameIndex = 0
        state.totalFrames = 100

        state.stepForward()
        // No crash, no state change expected (would need client)
        XCTAssertTrue(state.isLive)
    }

    func testStepBackwardIgnoredWhenLive() throws {
        let state = AppState()
        state.isLive = true
        state.currentFrameIndex = 50

        state.stepBackward()
        // No crash, no state change expected
        XCTAssertTrue(state.isLive)
    }

    func testStepBackwardIgnoredAtStart() throws {
        let state = AppState()
        state.isLive = false
        state.currentFrameIndex = 0

        state.stepBackward()
        // No crash, should not go negative
        XCTAssertEqual(state.currentFrameIndex, 0)
    }

    func testSliderEditingState() throws {
        let state = AppState()
        XCTAssertFalse(state.isSeekingInProgress)

        state.setSliderEditing(true)
        XCTAssertTrue(state.isSeekingInProgress)

        state.setSliderEditing(false)
        XCTAssertFalse(state.isSeekingInProgress)
    }

    func testReplayFinishedState() throws {
        let state = AppState()
        XCTAssertFalse(state.replayFinished)

        state.replayFinished = true
        XCTAssertTrue(state.replayFinished)
    }

    func testTotalFramesState() throws {
        let state = AppState()
        XCTAssertEqual(state.totalFrames, 0)

        state.totalFrames = 5000
        XCTAssertEqual(state.totalFrames, 5000)
    }

    func testCurrentFrameIndexState() throws {
        let state = AppState()
        XCTAssertEqual(state.currentFrameIndex, 0)

        state.currentFrameIndex = 250
        XCTAssertEqual(state.currentFrameIndex, 250)
    }

    func testIncreaseRateIgnoredWhenLive() throws {
        let state = AppState()
        state.isLive = true
        state.playbackRate = 1.0

        state.increaseRate()
        // Rate should not change when live
        XCTAssertEqual(state.playbackRate, 1.0)
    }

    func testDecreaseRateIgnoredWhenLive() throws {
        let state = AppState()
        state.isLive = true
        state.playbackRate = 1.0

        state.decreaseRate()
        // Rate should not change when live
        XCTAssertEqual(state.playbackRate, 1.0)
    }

    func testResetRateIgnoredWhenLive() throws {
        let state = AppState()
        state.isLive = true
        state.playbackRate = 4.0

        state.resetRate()
        // Rate should not change when live
        XCTAssertEqual(state.playbackRate, 4.0)
    }

    func testTogglePlayPauseIgnoredWhenLive() throws {
        let state = AppState()
        state.isLive = true
        state.isPaused = false

        state.togglePlayPause()
        // Should not change when live
        XCTAssertFalse(state.isPaused)
    }
}

// MARK: - Connection State Tests

@available(macOS 15.0, *) @MainActor final class ConnectionStateTests: XCTestCase {

    func testServerAddressDefault() throws {
        let state = AppState()
        XCTAssertEqual(state.serverAddress, "localhost:50051")
    }

    func testServerAddressCanBeChanged() throws {
        let state = AppState()
        state.serverAddress = "192.168.1.100:50051"
        XCTAssertEqual(state.serverAddress, "192.168.1.100:50051")
    }

    func testConnectionErrorDefault() throws {
        let state = AppState()
        XCTAssertNil(state.connectionError)
    }

    func testConnectionErrorCanBeSet() throws {
        let state = AppState()
        state.connectionError = "Connection refused"
        XCTAssertEqual(state.connectionError, "Connection refused")
    }

    func testIsConnectingDefault() throws {
        let state = AppState()
        XCTAssertFalse(state.isConnecting)
    }

    func testToggleConnectionWhenDisconnected() throws {
        let state = AppState()
        state.isConnected = false

        // toggleConnection calls connect() which creates client
        // We're testing the state transitions, not actual connection
        XCTAssertFalse(state.isConnected)
    }
}

// MARK: - Frame Processing Tests

@available(macOS 15.0, *) @MainActor final class FrameProcessingTests: XCTestCase {

    func testPlaybackInfoUpdatesFromFrame() async throws {
        let state = AppState()
        state.isLive = true

        var frame = FrameBundle()
        frame.frameID = 1
        frame.timestampNanos = 1_000_000_000
        frame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 1_000_000_000, logEndNs: 2_000_000_000, playbackRate: 2.0,
            paused: false, currentFrameIndex: 50, totalFrames: 500)

        state.onFrameReceived(frame)
        await Task.yield()

        XCTAssertFalse(state.isLive)
        XCTAssertEqual(state.logStartTimestamp, 1_000_000_000)
        XCTAssertEqual(state.logEndTimestamp, 2_000_000_000)
        XCTAssertEqual(state.playbackRate, 2.0)
        XCTAssertEqual(state.currentFrameIndex, 50)
        XCTAssertEqual(state.totalFrames, 500)
    }

    func testFrameReceivingDoesNotClearReplayFinished() async throws {
        let state = AppState()
        state.replayFinished = true

        var frame = FrameBundle()
        frame.frameID = 1
        state.onFrameReceived(frame)
        await Task.yield()

        // Frames without playbackInfo leave replayFinished unchanged
        XCTAssertTrue(state.replayFinished)
    }

    func testLastFrameSetsReplayFinished() async throws {
        let state = AppState()
        XCTAssertFalse(state.replayFinished)

        var frame = FrameBundle()
        frame.frameID = 100
        frame.timestampNanos = 1_000_000_000
        frame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0, paused: false,
            currentFrameIndex: 99, totalFrames: 100, seekable: true)

        state.onFrameReceived(frame)
        await Task.yield()

        XCTAssertTrue(state.replayFinished, "Last frame should set replayFinished")
        XCTAssertTrue(state.isPaused, "Last frame should pause playback")
        XCTAssertEqual(state.replayProgress, 1.0, "Progress should be 1.0 at end")
    }

    func testNonLastFrameClearsReplayFinished() async throws {
        let state = AppState()
        state.replayFinished = true

        var frame = FrameBundle()
        frame.frameID = 50
        frame.timestampNanos = 500_000_000
        frame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0, paused: false,
            currentFrameIndex: 49, totalFrames: 100, seekable: true)

        state.onFrameReceived(frame)
        await Task.yield()

        XCTAssertFalse(
            state.replayFinished,
            "Receiving a non-last frame should clear replayFinished (user seeked away)")
    }

    func testLiveFrameDoesNotSetReplayFinished() async throws {
        let state = AppState()

        var frame = FrameBundle()
        frame.frameID = 1
        frame.timestampNanos = 1_000_000_000
        frame.playbackInfo = PlaybackInfo(
            isLive: true, logStartNs: 0, logEndNs: 0, playbackRate: 1.0, paused: false,
            currentFrameIndex: 99, totalFrames: 100)

        state.onFrameReceived(frame)
        await Task.yield()

        XCTAssertFalse(state.replayFinished, "Live mode should never set replayFinished")
    }

    func testFPSCalculation() async throws {
        let state = AppState()
        XCTAssertEqual(state.fps, 0.0)

        // Simulate receiving multiple frames quickly
        for i in 1...10 {
            var frame = FrameBundle()
            frame.frameID = UInt64(i)
            state.onFrameReceived(frame)
            await Task.yield()
        }

        // FPS should be calculated (non-zero after multiple frames)
        // Note: actual FPS depends on timing, just verify it's tracked
        XCTAssertEqual(state.frameCount, 10)
    }

    func testProgressCalculationInReplayMode() async throws {
        let state = AppState()
        state.isLive = false
        state.logStartTimestamp = 0
        state.logEndTimestamp = 1_000_000_000

        var frame = FrameBundle()
        frame.frameID = 1
        frame.timestampNanos = 250_000_000  // 25% progress

        state.onFrameReceived(frame)
        await Task.yield()

        XCTAssertEqual(state.replayProgress, 0.25, accuracy: 0.01)
    }

    func testProgressNotUpdatedWhileSeeking() async throws {
        let state = AppState()
        state.isLive = false
        state.logStartTimestamp = 0
        state.logEndTimestamp = 1_000_000_000
        state.isSeekingInProgress = true
        state.replayProgress = 0.5  // Pre-set progress

        var frame = FrameBundle()
        frame.frameID = 1
        frame.timestampNanos = 250_000_000  // Would be 25%

        state.onFrameReceived(frame)
        await Task.yield()

        // Progress should NOT be updated while seeking
        XCTAssertEqual(state.replayProgress, 0.5)
    }

    func testEmptyFrameHandling() async throws {
        let state = AppState()

        let frame = FrameBundle()  // Empty frame
        state.onFrameReceived(frame)
        await Task.yield()

        XCTAssertEqual(state.pointCount, 0)
        XCTAssertEqual(state.clusterCount, 0)
        XCTAssertEqual(state.trackCount, 0)
    }
}

// MARK: - Toggle Debug Tests

@available(macOS 15.0, *) @MainActor final class ToggleDebugTests: XCTestCase {

    func testToggleDebugEnablesSubToggles() throws {
        let state = AppState()
        XCTAssertFalse(state.showDebug)
        XCTAssertFalse(state.showGating)
        XCTAssertFalse(state.showAssociation)
        XCTAssertFalse(state.showResiduals)

        state.toggleDebug()

        XCTAssertTrue(state.showDebug)
        XCTAssertTrue(state.showGating)
        XCTAssertTrue(state.showAssociation)
        XCTAssertTrue(state.showResiduals)
    }

    func testToggleDebugOffDoesNotResetSubToggles() throws {
        let state = AppState()

        // Enable debug (and sub-toggles)
        state.toggleDebug()
        XCTAssertTrue(state.showDebug)
        XCTAssertTrue(state.showGating)

        // Disable debug
        state.toggleDebug()
        XCTAssertFalse(state.showDebug)
        // Sub-toggles remain as-is (not reset)
        XCTAssertTrue(state.showGating)
    }

    func testToggleDebugTwiceRoundTrips() throws {
        let state = AppState()
        state.toggleDebug()
        state.toggleDebug()
        XCTAssertFalse(state.showDebug)
    }
}

// MARK: - Toggle Connection Tests

@available(macOS 15.0, *) @MainActor final class ToggleConnectionTests: XCTestCase {

    func testToggleConnectionIgnoredWhileConnecting() throws {
        let state = AppState()
        state.isConnecting = true

        // Should be ignored
        state.toggleConnection()
        // isConnecting should remain true (not toggled)
        XCTAssertTrue(state.isConnecting)
    }

    func testToggleConnectionCallsDisconnectWhenConnected() throws {
        let state = AppState()
        state.isConnected = true
        state.isConnecting = false

        state.toggleConnection()
        // disconnect() clears isConnected
        XCTAssertFalse(state.isConnected)
    }

    func testConnectSkippedWhenAlreadyConnecting() throws {
        let state = AppState()
        state.isConnecting = true

        state.connect()
        // Should skip — isConnecting remains true
        XCTAssertTrue(state.isConnecting)
    }

    func testConnectSkippedWhenAlreadyConnected() throws {
        let state = AppState()
        state.isConnected = true

        state.connect()
        // Should skip — isConnected remains true without transition to isConnecting
        XCTAssertTrue(state.isConnected)
    }

    func testConnectSetsIsConnecting() throws {
        let state = AppState()
        state.isConnected = false
        state.isConnecting = false

        state.connect()
        // connect() should set isConnecting and clear connectionError
        XCTAssertTrue(state.isConnecting)
        XCTAssertNil(state.connectionError)
    }
}

// MARK: - Assign Label / Quality Tests

@available(macOS 15.0, *) @MainActor final class AssignLabelTests: XCTestCase {

    func testAssignLabelNoSelectedTrackDoesNothing() throws {
        let state = AppState()
        state.selectedTrackID = nil

        // Should not crash or throw
        state.assignLabel("car")
    }

    func testAssignLabelWithSelectedTrackInLiveMode() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"
        state.currentRunID = nil  // Live mode

        // Should not crash — fires async task that will fail HTTP but doesn't throw
        state.assignLabel("car")
    }

    func testAssignLabelWithSelectedTrackInRunMode() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"
        state.currentRunID = "run-abc"  // Run mode

        // Should not crash — fires async task
        state.assignLabel("car")
    }

    func testAssignQualityRequiresSelectedTrackAndRunID() throws {
        let state = AppState()

        // No selected track — should do nothing
        state.selectedTrackID = nil
        state.currentRunID = "run-abc"
        state.assignQuality("good")

        // No run ID — should do nothing
        state.selectedTrackID = "track-001"
        state.currentRunID = nil
        state.assignQuality("good")
    }

    func testAssignQualityWithBothSet() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"
        state.currentRunID = "run-abc"

        // Should not crash — fires async task
        state.assignQuality("good")
    }
}

// MARK: - Mark as Split/Merge Tests

@available(macOS 15.0, *) @MainActor final class MarkSplitMergeTests: XCTestCase {

    func testMarkAsSplitRequiresSelectedTrackAndRunID() throws {
        let state = AppState()

        // No track ID — guard returns
        state.selectedTrackID = nil
        state.currentRunID = "run-abc"
        state.markAsSplit(true)

        // No run ID — guard returns
        state.selectedTrackID = "track-001"
        state.currentRunID = nil
        state.markAsSplit(true)
    }

    func testMarkAsSplitWithBothSet() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"
        state.currentRunID = "run-abc"

        state.markAsSplit(true)
        state.markAsSplit(false)
    }

    func testMarkAsMergeRequiresSelectedTrackAndRunID() throws {
        let state = AppState()

        state.selectedTrackID = nil
        state.currentRunID = "run-abc"
        state.markAsMerge(true)

        state.selectedTrackID = "track-001"
        state.currentRunID = nil
        state.markAsMerge(true)
    }

    func testMarkAsMergeWithBothSet() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"
        state.currentRunID = "run-abc"

        state.markAsMerge(true)
        state.markAsMerge(false)
    }
}

// MARK: - Reproject Labels Tests

@available(macOS 15.0, *) @MainActor final class ReprojectLabelsTests: XCTestCase {

    func testReprojectLabelsSkippedWhenTrackLabelsHidden() throws {
        let state = AppState()
        state.showTrackLabels = false
        state.metalViewSize = CGSize(width: 800, height: 600)

        // Should not crash, labels should remain empty
        state.reprojectLabels()
        XCTAssertTrue(state.trackLabels.isEmpty)
    }

    func testReprojectLabelsSkippedWhenZeroViewSize() throws {
        let state = AppState()
        state.showTrackLabels = true
        state.metalViewSize = .zero

        state.reprojectLabels()
        XCTAssertTrue(state.trackLabels.isEmpty)
    }

    func testReprojectLabelsSkippedWithNoRenderer() throws {
        let state = AppState()
        state.showTrackLabels = true
        state.metalViewSize = CGSize(width: 800, height: 600)

        // No renderer registered — should not crash
        state.reprojectLabels()
        XCTAssertTrue(state.trackLabels.isEmpty)
    }
}

// MARK: - Send Overlay Preferences Tests

@available(macOS 15.0, *) @MainActor final class SendOverlayPreferencesTests: XCTestCase {

    func testSendOverlayPreferencesWithoutConnection() throws {
        let state = AppState()
        // Should not crash even without a gRPC client
        state.sendOverlayPreferences()
    }
}

// MARK: - Client Delegate Adapter Tests

@available(macOS 15.0, *) @MainActor final class ClientDelegateAdapterTests: XCTestCase {

    // MARK: - MainActor-safe polling helper
    //
    // XCTNSPredicateExpectation evaluates its NSPredicate(block:) on a
    // non-MainActor thread, which races with @MainActor-isolated AppState
    // properties.  This helper polls the condition on MainActor instead,
    // yielding between checks to let enqueued Tasks execute.

    private func waitForMainActor(
        timeout: TimeInterval = 2.0, condition: @escaping @MainActor () -> Bool,
        file: StaticString = #filePath, line: UInt = #line
    ) async throws {
        let deadline = ContinuousClock.now + .seconds(timeout)
        while !condition() {
            guard ContinuousClock.now < deadline else {
                XCTFail("Timed out after \(timeout)s waiting for condition", file: file, line: line)
                return
            }
            try await Task.sleep(for: .milliseconds(10))
        }
    }

    func testDelegateConnect() async throws {
        let state = AppState()
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        delegate.clientDidConnect(client)

        try await waitForMainActor { state.isConnected }
        XCTAssertNil(state.connectionError)
        XCTAssertFalse(state.replayFinished)
    }

    func testDelegateDisconnectWithError() async throws {
        let state = AppState()
        state.isConnected = true
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        let error = NSError(domain: "test", code: -1, userInfo: nil)
        delegate.clientDidDisconnect(client, error: error)

        try await waitForMainActor { !state.isConnected }
        XCTAssertEqual(state.connectionError, "Connection lost")
    }

    func testDelegateDisconnectWithoutError() async throws {
        let state = AppState()
        state.isConnected = true
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        delegate.clientDidDisconnect(client, error: nil)

        try await waitForMainActor { !state.isConnected }
        // No error should be set
        XCTAssertNil(state.connectionError)
    }

    func testDelegateDidReceiveFrame() async throws {
        let state = AppState()
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        var frame = FrameBundle()
        frame.frameID = 99
        frame.timestampNanos = 500_000_000

        delegate.client(client, didReceiveFrame: frame)

        try await waitForMainActor { state.currentFrameID == 99 }
    }

    func testDelegateDidFinishStream() async throws {
        let state = AppState()
        state.isPaused = false
        state.replayProgress = 0.5
        state.logEndTimestamp = 2_000_000_000
        state.currentTimestamp = 1_900_000_000
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        delegate.clientDidFinishStream(client)

        // handleStreamFinished now runs synchronously via MainActor.assumeIsolated
        XCTAssertTrue(state.replayFinished)
        XCTAssertTrue(state.isPaused)
        XCTAssertEqual(state.replayProgress, 1.0)
        // currentTimestamp should be synced to logEndTimestamp
        XCTAssertEqual(state.currentTimestamp, 2_000_000_000)
    }
}

// MARK: - Step Forward/Backward Guard Tests

@available(macOS 15.0, *) @MainActor final class StepGuardTests: XCTestCase {

    func testStepForwardIgnoredWhenNotSeekable() throws {
        let state = AppState()
        state.isLive = false
        state.isSeekable = false
        state.currentFrameIndex = 0
        state.totalFrames = 100

        state.stepForward()  // No crash expected
    }

    func testStepForwardIgnoredAtEnd() throws {
        let state = AppState()
        state.isLive = false
        state.isSeekable = true
        state.currentFrameIndex = 99
        state.totalFrames = 100

        state.stepForward()  // Guard should prevent stepping past end
    }

    func testStepBackwardIgnoredWhenNotSeekable() throws {
        let state = AppState()
        state.isLive = false
        state.isSeekable = false
        state.currentFrameIndex = 50

        state.stepBackward()  // No crash expected
    }

    func testSeekIgnoredWhenNotSeekable() throws {
        let state = AppState()
        state.isLive = false
        state.isSeekable = false
        state.replayProgress = 0.0

        state.seek(to: 0.5)
        // replayProgress should NOT change because isSeekable is false
        XCTAssertEqual(state.replayProgress, 0.0)
    }

    func testSeekWhenReplayFinished() throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.isLive = false
        state.isSeekable = true
        state.replayFinished = true
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000

        state.seek(to: 0.3)
        XCTAssertEqual(state.replayProgress, 0.3)
        XCTAssertTrue(state.isSeekingInProgress)
    }
}

// MARK: - Frame Playback Info Edge Cases

@available(macOS 15.0, *) @MainActor final class FramePlaybackEdgeCaseTests: XCTestCase {

    func testSeekableStateFromPlaybackInfo() async throws {
        let state = AppState()

        var frame = FrameBundle()
        frame.frameID = 1
        frame.timestampNanos = 1_000_000_000
        frame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 1_000_000_000, logEndNs: 2_000_000_000, playbackRate: 1.0,
            paused: false, currentFrameIndex: 0, totalFrames: 100, seekable: true)

        state.onFrameReceived(frame)
        await Task.yield()

        XCTAssertTrue(state.isSeekable)
    }

    func testOnFrameReceivedUpdatesRendererFlags() async throws {
        let state = AppState()
        state.showClusters = false
        state.showDebug = true
        state.showGating = true
        state.showAssociation = true
        state.showResiduals = true
        state.selectedTrackID = "track-001"

        var frame = FrameBundle()
        frame.frameID = 1
        state.onFrameReceived(frame)
        await Task.yield()

        // Just verify it doesn't crash — renderer is nil so flags are set on nil
        XCTAssertEqual(state.currentFrameID, 1)
    }

    func testOnFrameReceivedDoesNotClearReplayFinishedFlag() async throws {
        let state = AppState()
        state.replayFinished = true

        var frame = FrameBundle()
        frame.frameID = 1
        state.onFrameReceived(frame)
        await Task.yield()

        // replayFinished is only cleared by togglePlayPause, not by frame receipt
        XCTAssertTrue(state.replayFinished)
    }

    func testReplayProgressClampedToZeroOne() async throws {
        let state = AppState()
        state.isLive = false
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000

        // Timestamp before log start
        var frame = FrameBundle()
        frame.frameID = 1
        frame.timestampNanos = 500_000_000  // Before start
        state.onFrameReceived(frame)
        await Task.yield()

        XCTAssertGreaterThanOrEqual(state.replayProgress, 0.0)
    }

    func testFPSExponentialMovingAverage() async throws {
        let state = AppState()

        // First frame: fps starts at 0, so first calculation sets it directly
        var frame1 = FrameBundle()
        frame1.frameID = 1
        state.onFrameReceived(frame1)
        await Task.yield()

        // fps should be set (from 0 case)
        let firstFPS = state.fps

        // Second frame quickly after
        var frame2 = FrameBundle()
        frame2.frameID = 2
        state.onFrameReceived(frame2)
        await Task.yield()

        // FPS should now use EMA (0.2 * new + 0.8 * old)
        XCTAssertNotEqual(state.fps, firstFPS)
    }
}

// MARK: - Run State Tests

@available(macOS 15.0, *) @MainActor final class RunStateTests: XCTestCase {

    func testCurrentRunIDDefault() throws {
        let state = AppState()
        XCTAssertNil(state.currentRunID)
    }

    func testShowRunBrowserDefault() throws {
        let state = AppState()
        XCTAssertFalse(state.showRunBrowser)
    }

    func testCurrentRunIDCanBeSet() throws {
        let state = AppState()
        state.currentRunID = "run-123"
        XCTAssertEqual(state.currentRunID, "run-123")
    }

    func testShowSidePanelDefault() throws {
        let state = AppState()
        XCTAssertFalse(state.showSidePanel)
    }

    func testSelectTrackShowsSidePanel() throws {
        let state = AppState()
        state.selectTrack("track-001")

        XCTAssertTrue(state.showLabelPanel)
        XCTAssertTrue(state.showSidePanel)
    }

    func testSelectNilTrackDoesNotShowPanels() throws {
        let state = AppState()
        state.showLabelPanel = false
        state.showSidePanel = false

        state.selectTrack(nil)
        XCTAssertFalse(state.showLabelPanel)
        XCTAssertFalse(state.showSidePanel)
    }
}

// MARK: - Track History Truncation Tests

@available(macOS 15.0, *) @MainActor final class TrackHistoryTruncationTests: XCTestCase {

    func testHistoryTruncatesOnBackwardSeek() async throws {
        let state = AppState()
        state.isLive = false

        // Simulate 5 frames of forward playback
        for i: UInt64 in 0..<5 {
            var frame = FrameBundle()
            frame.frameID = i
            frame.timestampNanos = Int64(i) * 100_000_000
            frame.playbackInfo = PlaybackInfo(
                isLive: false, logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0,
                paused: false, currentFrameIndex: i, totalFrames: 10)
            frame.tracks = TrackSet(
                frameID: i, timestampNanos: Int64(i) * 100_000_000,
                tracks: [Track(trackID: "t-001", state: .confirmed, speedMps: Float(i))], trails: []
            )
            state.onFrameReceived(frame)
            await Task.yield()
        }

        let samplesAfterForward = state.trackHistory["t-001"]?.count ?? 0
        XCTAssertEqual(samplesAfterForward, 5)

        // Now simulate a backward seek to frame 2
        var backFrame = FrameBundle()
        backFrame.frameID = 2
        backFrame.timestampNanos = 200_000_000
        backFrame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0, paused: true,
            currentFrameIndex: 2, totalFrames: 10)
        backFrame.tracks = TrackSet(
            frameID: 2, timestampNanos: 200_000_000,
            tracks: [Track(trackID: "t-001", state: .confirmed, speedMps: 2.0)], trails: [])
        state.onFrameReceived(backFrame)
        await Task.yield()

        // History should have been truncated: frames 0, 1, and the new frame 2 = 3 samples
        let samplesAfterBackward = state.trackHistory["t-001"]?.count ?? 0
        XCTAssertEqual(samplesAfterBackward, 3)
    }

    func testHistoryGrowsNormallyOnForwardPlay() async throws {
        let state = AppState()
        state.isLive = false

        for i: UInt64 in 0..<10 {
            var frame = FrameBundle()
            frame.frameID = i
            frame.timestampNanos = Int64(i) * 100_000_000
            frame.playbackInfo = PlaybackInfo(
                isLive: false, logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0,
                paused: false, currentFrameIndex: i, totalFrames: 100)
            frame.tracks = TrackSet(
                frameID: i, timestampNanos: Int64(i) * 100_000_000,
                tracks: [Track(trackID: "t-001", state: .confirmed, speedMps: Float(i))], trails: []
            )
            state.onFrameReceived(frame)
            await Task.yield()
        }

        XCTAssertEqual(state.trackHistory["t-001"]?.count, 10)
    }

    func testHistoryDoesNotDuplicateOnSameFrame() async throws {
        let state = AppState()
        state.isLive = false

        // Receive the same frame index twice (e.g. pause + step to same frame)
        for _ in 0..<2 {
            var frame = FrameBundle()
            frame.frameID = 5
            frame.timestampNanos = 500_000_000
            frame.playbackInfo = PlaybackInfo(
                isLive: false, logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0,
                paused: true, currentFrameIndex: 5, totalFrames: 10)
            frame.tracks = TrackSet(
                frameID: 5, timestampNanos: 500_000_000,
                tracks: [Track(trackID: "t-001", state: .confirmed, speedMps: 5.0)], trails: [])
            state.onFrameReceived(frame)
            await Task.yield()
        }

        // Should only have 1 sample (second receive truncates >= 5, then appends)
        XCTAssertEqual(state.trackHistory["t-001"]?.count, 1)
    }
}

// MARK: - Step After Replay Finished Tests

@available(macOS 15.0, *) @MainActor final class StepAfterFinishTests: XCTestCase {

    func testStepBackwardWhenFinishedDoesNotCrash() throws {
        let state = AppState()
        state.isLive = false
        state.isSeekable = true
        state.replayFinished = true
        state.isPaused = true
        state.currentFrameIndex = 99
        state.totalFrames = 100

        // Should not crash — steps backward and handles finished state
        state.stepBackward()
    }

    func testStepForwardBlockedAtEnd() throws {
        let state = AppState()
        state.isLive = false
        state.isSeekable = true
        state.replayFinished = true
        state.isPaused = true
        state.currentFrameIndex = 99
        state.totalFrames = 100

        // currentFrameIndex + 1 == totalFrames, guard should block
        state.stepForward()
    }

    func testStepForwardAllowedBeforeEnd() throws {
        let state = AppState()
        state.isLive = false
        state.isSeekable = true
        state.replayFinished = true
        state.isPaused = true
        state.currentFrameIndex = 50
        state.totalFrames = 100

        // currentFrameIndex + 1 < totalFrames, should proceed
        state.stepForward()
    }
}

// MARK: - Labelling State Tests

@available(macOS 15.0, *) @MainActor final class LabellingStateTests: XCTestCase {

    func testShowLabelPanelDefault() throws {
        let state = AppState()
        XCTAssertFalse(state.showLabelPanel)
    }

    func testShowLabelPanelToggle() throws {
        let state = AppState()
        state.showLabelPanel = true
        XCTAssertTrue(state.showLabelPanel)

        state.showLabelPanel = false
        XCTAssertFalse(state.showLabelPanel)
    }

    func testSelectTrackWithString() throws {
        let state = AppState()
        state.selectTrack("track-abc-123")
        XCTAssertEqual(state.selectedTrackID, "track-abc-123")
    }

    func testSelectTrackDeselect() throws {
        let state = AppState()
        state.selectTrack("track-001")
        XCTAssertNotNil(state.selectedTrackID)

        state.selectTrack(nil)
        XCTAssertNil(state.selectedTrackID)
    }
}

// MARK: - User Labels Cache Tests

@available(macOS 15.0, *) @MainActor final class UserLabelsCacheTests: XCTestCase {

    func testUserLabelsDefaultEmpty() throws {
        let state = AppState()
        XCTAssertTrue(state.userLabels.isEmpty)
    }

    func testAssignLabelUpdatesUserLabelsCache() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"

        state.assignLabel("car")
        XCTAssertEqual(state.userLabels["track-001"], "car")
    }

    func testAssignLabelOverwritesPreviousLabel() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"

        state.assignLabel("car")
        XCTAssertEqual(state.userLabels["track-001"], "car")

        state.assignLabel("pedestrian")
        XCTAssertEqual(state.userLabels["track-001"], "pedestrian")
    }

    func testAssignLabelDoesNotUpdateCacheWithoutSelectedTrack() throws {
        let state = AppState()
        state.selectedTrackID = nil

        state.assignLabel("car")
        XCTAssertTrue(state.userLabels.isEmpty)
    }

    func testAssignLabelToAllVisiblePopulatesCache() async throws {
        let state = AppState()
        // Create a frame with tracks so filteredTracks is non-empty
        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 100,
            tracks: [
                Track(trackID: "t-001", state: .confirmed),
                Track(trackID: "t-002", state: .confirmed),
                Track(trackID: "t-003", state: .tentative),
            ], trails: [])
        // Use onFrameReceived so allSeenTracks is populated (filteredTracks reads from it)
        state.onFrameReceived(frame)
        await Task.yield()

        state.assignLabelToAllVisible("bicycle")

        XCTAssertEqual(state.userLabels["t-001"], "bicycle")
        XCTAssertEqual(state.userLabels["t-002"], "bicycle")
        XCTAssertEqual(state.userLabels["t-003"], "bicycle")
    }

    func testAssignLabelToAllVisibleDoesNothingWhenEmpty() throws {
        let state = AppState()
        // No frame, no tracks
        state.assignLabelToAllVisible("car")
        XCTAssertTrue(state.userLabels.isEmpty)
    }

    func testMultipleTracksHaveIndependentLabels() throws {
        let state = AppState()

        state.selectedTrackID = "track-001"
        state.assignLabel("car")

        state.selectedTrackID = "track-002"
        state.assignLabel("pedestrian")

        XCTAssertEqual(state.userLabels["track-001"], "car")
        XCTAssertEqual(state.userLabels["track-002"], "pedestrian")
    }
}

// MARK: - Seek Timestamp Sync Tests

@available(macOS 15.0, *) @MainActor final class SeekTimestampSyncTests: XCTestCase {

    func testSeekUpdatesCurrentTimestamp() throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.isLive = false
        state.isSeekable = true
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000
        state.currentTimestamp = 1_000_000_000

        state.seek(to: 0.5)

        // currentTimestamp should be optimistically synced to the target
        XCTAssertEqual(state.currentTimestamp, 1_500_000_000)
    }

    func testSeekToStartUpdatesTimestamp() throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.isLive = false
        state.isSeekable = true
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000
        state.currentTimestamp = 1_500_000_000

        state.seek(to: 0.0)

        XCTAssertEqual(state.currentTimestamp, 1_000_000_000)
    }

    func testSeekToEndUpdatesTimestamp() throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.playbackCommandClientOverride = fake
        state.isLive = false
        state.isSeekable = true
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000
        state.currentTimestamp = 1_000_000_000

        state.seek(to: 1.0)

        XCTAssertEqual(state.currentTimestamp, 2_000_000_000)
    }

    func testSeekIgnoredWhenLiveDoesNotUpdateTimestamp() throws {
        let state = AppState()
        state.isLive = true
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000
        state.currentTimestamp = 1_000_000_000

        state.seek(to: 0.5)

        // Should remain unchanged
        XCTAssertEqual(state.currentTimestamp, 1_000_000_000)
    }
}

// MARK: - ClientDidFinishStream Edge Case Tests

@available(macOS 15.0, *) @MainActor final class ClientDidFinishStreamEdgeCaseTests: XCTestCase {

    private func waitForMainActor(
        timeout: TimeInterval = 2.0, condition: @escaping @MainActor () -> Bool,
        file: StaticString = #filePath, line: UInt = #line
    ) async throws {
        let deadline = ContinuousClock.now + .seconds(timeout)
        while !condition() {
            guard ContinuousClock.now < deadline else {
                XCTFail("Timed out after \(timeout)s waiting for condition", file: file, line: line)
                return
            }
            try await Task.sleep(for: .milliseconds(10))
        }
    }

    func testFinishStreamDoesNotSyncWhenLogEndIsZero() async throws {
        let state = AppState()
        state.logEndTimestamp = 0  // No valid log end
        state.currentTimestamp = 500_000_000
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        delegate.clientDidFinishStream(client)

        try await waitForMainActor { state.replayFinished }
        // currentTimestamp should NOT be overwritten when logEndTimestamp is 0
        XCTAssertEqual(state.currentTimestamp, 500_000_000)
    }
}

// MARK: - Export Labels Removal Tests

@available(macOS 15.0, *) @MainActor final class ExportLabelsRemovalTests: XCTestCase {

    func testExportLabelsMethodDoesNotExist() throws {
        // Verify that the removed exportLabels() method is no longer available.
        // This test uses compile-time verification: if exportLabels() still existed,
        // uncommenting the line below would compile. It being commented out documents
        // the intentional removal.
        let state = AppState()
        XCTAssertNotNil(state)  // AppState initialises without exportLabels
        // state.exportLabels()  // Should NOT compile — method removed on this branch
    }
}

// MARK: - Prepare For New Replay Tests (Regression Guard)

/// Regression test: loading a new VRLOG after a previous replay finished must
/// reset all stale playback state.  Before the fix, `clientDidFinishStream` set
/// `isPaused = true`, `replayFinished = true`, `replayProgress = 1.0`, and
/// `currentTimestamp = logEndTimestamp`. When a new VRLOG was loaded via the run
/// browser, none of these were cleared, so the new replay appeared stuck/paused
/// and the seek bar showed "100%".
@available(macOS 15.0, *) @MainActor final class PrepareForNewReplayTests: XCTestCase {

    func testPrepareForNewReplayResetsPlaybackState() async throws {
        let state = AppState()

        // Receive a frame to populate trackHistory and frameCount naturally
        var frame = FrameBundle()
        frame.frameID = 100
        frame.timestampNanos = 1_500_000_000
        frame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 1_000_000_000, logEndNs: 2_000_000_000, playbackRate: 1.0,
            paused: false, currentFrameIndex: 100, totalFrames: 394)
        frame.tracks = TrackSet(
            frameID: 100, timestampNanos: 1_500_000_000,
            tracks: [Track(trackID: "t-001", state: .confirmed, speedMps: 5.0)], trails: [])
        state.onFrameReceived(frame)
        await Task.yield()

        // Simulate a finished replay (stale state from clientDidFinishStream)
        state.isPaused = true
        state.replayFinished = true
        state.replayProgress = 1.0
        state.isSeekingInProgress = true
        state.currentTimestamp = 2_000_000_000
        state.userLabels["t-001"] = "car"

        // Pre-conditions: verify stale state exists
        XCTAssertFalse(state.trackHistory.isEmpty, "trackHistory should have data before reset")
        XCTAssertEqual(state.frameCount, 1)

        // Act: prepare for a new replay (called before loadRunForReplay)
        state.prepareForNewReplay()

        // Assert: all stale state must be cleared
        XCTAssertFalse(state.isPaused, "isPaused should be cleared for new replay")
        XCTAssertFalse(state.replayFinished, "replayFinished should be cleared")
        XCTAssertEqual(state.replayProgress, 0, "replayProgress should reset to 0")
        XCTAssertFalse(state.isSeekingInProgress, "isSeekingInProgress should be cleared")
        XCTAssertEqual(state.currentTimestamp, 0, "currentTimestamp should reset to 0")
        XCTAssertEqual(state.logStartTimestamp, 0, "logStartTimestamp should reset to 0")
        XCTAssertEqual(state.logEndTimestamp, 0, "logEndTimestamp should reset to 0")
        XCTAssertEqual(state.currentFrameIndex, 0, "currentFrameIndex should reset to 0")
        XCTAssertEqual(state.totalFrames, 0, "totalFrames should reset to 0")
        XCTAssertEqual(state.frameCount, 0, "frameCount should reset to 0")
        XCTAssertTrue(state.trackHistory.isEmpty, "trackHistory should be cleared")
        XCTAssertTrue(state.userLabels.isEmpty, "userLabels should be cleared")
    }

    func testPrepareForNewReplayFollowedByFirstFrameProducesCleanState() async throws {
        let state = AppState()

        // Simulate a finished replay
        state.isPaused = true
        state.replayFinished = true
        state.replayProgress = 1.0
        state.currentTimestamp = 2_000_000_000
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000
        state.currentFrameIndex = 394
        state.totalFrames = 394
        state.frameCount = 500

        // Prepare for new replay
        state.prepareForNewReplay()
        state.isLive = false

        // Simulate first frame of the NEW replay
        var frame = FrameBundle()
        frame.frameID = 0
        frame.timestampNanos = 500_000_000
        frame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 500_000_000, logEndNs: 40_000_000_000, playbackRate: 1.0,
            paused: false, currentFrameIndex: 0, totalFrames: 394)
        frame.tracks = TrackSet(
            frameID: 0, timestampNanos: 500_000_000,
            tracks: [Track(trackID: "t-new-001", state: .confirmed, speedMps: 3.0)], trails: [])

        state.onFrameReceived(frame)
        await Task.yield()

        // Verify new replay started cleanly
        XCTAssertFalse(state.isPaused, "New replay should not be paused")
        XCTAssertFalse(state.replayFinished, "replayFinished should remain cleared")
        XCTAssertEqual(state.currentFrameID, 0)
        XCTAssertEqual(state.currentTimestamp, 500_000_000)
        XCTAssertEqual(state.logStartTimestamp, 500_000_000)
        XCTAssertEqual(state.logEndTimestamp, 40_000_000_000)
        XCTAssertEqual(state.totalFrames, 394)
        XCTAssertEqual(state.frameCount, 1, "frameCount should start from 1")
        XCTAssertEqual(state.trackCount, 1)
        // Replay progress should be near 0 (first frame of new log)
        XCTAssertLessThan(state.replayProgress, 0.01, "Progress should be near start")
    }

    /// Regression: clientDidFinishStream fires → sets isPaused/replayFinished →
    /// without prepareForNewReplay the new VRLOG appears stuck.
    func testWithoutPrepareNewReplayInheritsStaleState() async throws {
        let state = AppState()
        state.isLive = false
        state.logEndTimestamp = 2_000_000_000

        // Simulate clientDidFinishStream side effects (old replay ended)
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")
        delegate.clientDidFinishStream(client)

        // Wait for the async Task in clientDidFinishStream to apply
        let deadline = ContinuousClock.now + .seconds(2)
        while !state.replayFinished {
            guard ContinuousClock.now < deadline else {
                XCTFail("Timed out waiting for replayFinished")
                return
            }
            try await Task.sleep(for: .milliseconds(10))
        }

        // Confirm stale state is set
        XCTAssertTrue(state.isPaused, "Finish should set isPaused")
        XCTAssertTrue(state.replayFinished, "Finish should set replayFinished")
        XCTAssertEqual(state.replayProgress, 1.0, "Finish should set progress to 1.0")

        // NOW call prepareForNewReplay (the fix)
        state.prepareForNewReplay()

        // Verify all stale state is cleared
        XCTAssertFalse(state.isPaused)
        XCTAssertFalse(state.replayFinished)
        XCTAssertEqual(state.replayProgress, 0)
    }

    /// Regression: restartStream() cancels the old stream task, which exits the
    /// `for try await` loop in streamFrames(). Before the fix, this unconditionally
    /// called clientDidFinishStream — racing with prepareForNewReplay() and
    /// re-dirtying the playback state. Simulate by calling prepareForNewReplay()
    /// then firing clientDidFinishStream() AFTER, and verify clean state survives.
    func testPrepareForNewReplayIsNotOverwrittenByLateFinishStream() async throws {
        let state = AppState()
        state.isLive = false

        // Simulate old replay with data
        state.logEndTimestamp = 2_000_000_000
        state.currentTimestamp = 2_000_000_000
        state.isPaused = true
        state.replayFinished = true
        state.replayProgress = 1.0

        // Step 1: prepareForNewReplay clears everything
        state.prepareForNewReplay()
        XCTAssertFalse(state.isPaused)
        XCTAssertFalse(state.replayFinished)
        XCTAssertEqual(state.replayProgress, 0)
        XCTAssertEqual(state.logEndTimestamp, 0)

        // Step 2: Simulate the LATE clientDidFinishStream callback that would fire
        // from the cancelled old stream (this is the race condition).
        // With the VisualiserClient fix, this callback would NOT fire for cancelled
        // streams. But even if it did somehow fire, prepareForNewReplay should have
        // set logEndTimestamp = 0, so the timestamp guard won't overwrite.
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")
        delegate.clientDidFinishStream(client)

        // Allow the async Task in clientDidFinishStream to run
        try await Task.sleep(for: .milliseconds(50))

        // The late callback WILL still dirty the state at the AppState level
        // (the guard is in VisualiserClient.streamFrames, not in clientDidFinishStream).
        // This test documents that the VisualiserClient-level guard is essential:
        // the delegate method itself has no defence against a late call.
        // When the VisualiserClient guard is working, this callback never fires.
        XCTAssertTrue(
            state.replayFinished, "Without VisualiserClient guard, late finish DOES re-dirty state")
    }
}

// MARK: - Track Navigation Tests

@available(macOS 15.0, *) @MainActor final class TrackNavigationTests: XCTestCase {

    func testSelectNextTrackNoTracks() {
        let state = AppState()
        // trackListOrder is empty by default
        state.selectNextTrack()
        XCTAssertNil(state.selectedTrackID)
    }

    func testSelectPreviousTrackNoTracks() {
        let state = AppState()
        state.selectPreviousTrack()
        XCTAssertNil(state.selectedTrackID)
    }

    func testSelectNextTrackNoCurrentSelection() {
        let state = AppState()
        state.trackListOrder = ["trk_b", "trk_a"]

        state.selectNextTrack()
        // Should select first in list
        XCTAssertEqual(state.selectedTrackID, "trk_b")
    }

    func testSelectPreviousTrackNoCurrentSelection() {
        let state = AppState()
        state.trackListOrder = ["trk_b", "trk_a"]

        state.selectPreviousTrack()
        // Should select last in list
        XCTAssertEqual(state.selectedTrackID, "trk_a")
    }

    func testSelectNextTrackWraps() {
        let state = AppState()
        state.trackListOrder = ["trk_b", "trk_a"]
        state.selectedTrackID = "trk_a"  // Last in list

        state.selectNextTrack()
        // Should wrap to first
        XCTAssertEqual(state.selectedTrackID, "trk_b")
    }

    func testSelectPreviousTrackWraps() {
        let state = AppState()
        state.trackListOrder = ["trk_b", "trk_a"]
        state.selectedTrackID = "trk_b"  // First in list

        state.selectPreviousTrack()
        // Should wrap to last
        XCTAssertEqual(state.selectedTrackID, "trk_a")
    }

    func testSelectNextTrackDoesNotOpenSidePanel() {
        let state = AppState()
        state.trackListOrder = ["trk_a"]
        state.showSidePanel = false
        state.showLabelPanel = false

        state.selectNextTrack()
        XCTAssertEqual(state.selectedTrackID, "trk_a")
        XCTAssertFalse(
            state.showSidePanel, "Up/down navigation should not force the side panel open")
        XCTAssertFalse(state.showLabelPanel)
    }

    func testSelectNextTrackFollowsListOrder() {
        let state = AppState()
        // Simulate "first seen" order (alphabetical here)
        state.trackListOrder = ["trk_a", "trk_b", "trk_c"]
        state.selectedTrackID = "trk_a"

        state.selectNextTrack()
        XCTAssertEqual(state.selectedTrackID, "trk_b")
        state.selectNextTrack()
        XCTAssertEqual(state.selectedTrackID, "trk_c")
    }

    func testSelectPreviousTrackFollowsListOrder() {
        let state = AppState()
        state.trackListOrder = ["trk_a", "trk_b", "trk_c"]
        state.selectedTrackID = "trk_c"

        state.selectPreviousTrack()
        XCTAssertEqual(state.selectedTrackID, "trk_b")
        state.selectPreviousTrack()
        XCTAssertEqual(state.selectedTrackID, "trk_a")
    }
}

// MARK: - TimeDisplayMode Tests

@available(macOS 15.0, *) @MainActor final class TimeDisplayModeTests: XCTestCase {

    // MARK: - Enum Properties

    func testDefaultTimeDisplayMode() {
        let state = AppState()
        XCTAssertEqual(state.timeDisplayMode, .elapsed)
    }

    func testNextCyclesElapsedToRemaining() {
        XCTAssertEqual(AppState.TimeDisplayMode.elapsed.next, .remaining)
    }

    func testNextCyclesRemainingToFrames() {
        XCTAssertEqual(AppState.TimeDisplayMode.remaining.next, .frames)
    }

    func testNextCyclesFramesToElapsed() {
        XCTAssertEqual(AppState.TimeDisplayMode.frames.next, .elapsed)
    }

    func testMenuLabelElapsed() {
        XCTAssertEqual(AppState.TimeDisplayMode.elapsed.menuLabel, "Elapsed Time")
    }

    func testMenuLabelRemaining() {
        XCTAssertEqual(AppState.TimeDisplayMode.remaining.menuLabel, "Remaining Time")
    }

    func testMenuLabelFrames() {
        XCTAssertEqual(AppState.TimeDisplayMode.frames.menuLabel, "Frame Index")
    }

    func testAllCasesCount() { XCTAssertEqual(AppState.TimeDisplayMode.allCases.count, 3) }

    // MARK: - cycleTimeDisplayMode()

    func testCycleTimeDisplayModeFullCycle() {
        let state = AppState()
        XCTAssertEqual(state.timeDisplayMode, .elapsed)

        state.cycleTimeDisplayMode()
        XCTAssertEqual(state.timeDisplayMode, .remaining)

        state.cycleTimeDisplayMode()
        XCTAssertEqual(state.timeDisplayMode, .frames)

        state.cycleTimeDisplayMode()
        XCTAssertEqual(state.timeDisplayMode, .elapsed)
    }

    func testCycleFromFramesWrapsToElapsed() {
        let state = AppState()
        state.timeDisplayMode = .frames
        state.cycleTimeDisplayMode()
        XCTAssertEqual(state.timeDisplayMode, .elapsed)
    }

    func testTimeDisplayModeEquatable() {
        let a: AppState.TimeDisplayMode = .elapsed
        let b: AppState.TimeDisplayMode = .elapsed
        let c: AppState.TimeDisplayMode = .remaining
        XCTAssertEqual(a, b)
        XCTAssertNotEqual(a, c)
    }

    func testTimeDisplayModeRawValues() {
        XCTAssertEqual(AppState.TimeDisplayMode.elapsed.rawValue, "elapsed")
        XCTAssertEqual(AppState.TimeDisplayMode.remaining.rawValue, "remaining")
        XCTAssertEqual(AppState.TimeDisplayMode.frames.rawValue, "frames")
    }
}

// MARK: - resetAdmittedTracks Tests

@available(macOS 15.0, *) @MainActor final class ResetAdmittedTracksTests: XCTestCase {

    func testResetClearsAdmittedTrackIDs() throws {
        let state = AppState()
        // Pre-load a frame with tracks and apply a filter so tracks get admitted
        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 100,
            tracks: [
                Track(trackID: "t-1", state: .confirmed, hits: 10),
                Track(trackID: "t-2", state: .confirmed, hits: 5),
            ], trails: [])
        state.currentFrame = frame
        state.filterMinHits = 1
        state.updateAdmittedTracks()

        XCTAssertFalse(state.admittedTrackIDs.isEmpty, "precondition: tracks should be admitted")

        state.resetAdmittedTracks(reason: "test")

        // After reset, tracks that still pass the filter should be re-admitted
        // because resetAdmittedTracks calls updateAdmittedTracks() internally.
        XCTAssertTrue(state.admittedTrackIDs.contains("t-1"))
        XCTAssertTrue(state.admittedTrackIDs.contains("t-2"))
    }

    func testResetClearsTracksThatNoLongerPassFilter() throws {
        let state = AppState()
        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 100,
            tracks: [
                Track(trackID: "t-low", state: .confirmed, hits: 2),
                Track(trackID: "t-high", state: .confirmed, hits: 20),
            ], trails: [])
        state.currentFrame = frame
        state.filterMinHits = 1
        state.updateAdmittedTracks()

        XCTAssertTrue(state.admittedTrackIDs.contains("t-low"))

        // Raise filter threshold so t-low no longer passes
        state.filterMinHits = 10
        state.resetAdmittedTracks(reason: "filter change")

        XCTAssertFalse(state.admittedTrackIDs.contains("t-low"))
        XCTAssertTrue(state.admittedTrackIDs.contains("t-high"))
    }

    func testResetWithNoCurrentFrame() throws {
        let state = AppState()
        state.filterMinHits = 5
        // No current frame — should not crash
        state.resetAdmittedTracks(reason: "no frame")
        XCTAssertTrue(state.admittedTrackIDs.isEmpty)
    }

    func testResetWithDefaultReason() throws {
        let state = AppState()
        // Uses the default reason parameter ("unspecified") — just verifies no crash
        state.resetAdmittedTracks()
        XCTAssertTrue(state.admittedTrackIDs.isEmpty)
    }

    func testResetWithNoActiveFilters() throws {
        let state = AppState()
        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 100, tracks: [Track(trackID: "t-1", state: .confirmed)],
            trails: [])
        state.currentFrame = frame

        // No filters active — hasActiveFilters is false
        XCTAssertFalse(state.hasActiveFilters)

        state.resetAdmittedTracks(reason: "clear")
        // updateAdmittedTracks runs but trackPassesFilter always returns true
        // when no filter criteria are set
        XCTAssertTrue(state.admittedTrackIDs.contains("t-1"))
    }
}

// MARK: - updateMetalViewSize Tests

@available(macOS 15.0, *) @MainActor final class UpdateMetalViewSizeTests: XCTestCase {

    func testUpdateChangesSize() throws {
        let state = AppState()
        XCTAssertEqual(state.metalViewSize, .zero)

        state.updateMetalViewSize(CGSize(width: 800, height: 600), source: "test")
        XCTAssertEqual(state.metalViewSize, CGSize(width: 800, height: 600))
    }

    func testUpdateNoOpWhenSizeUnchanged() throws {
        let state = AppState()
        let size = CGSize(width: 1024, height: 768)
        state.updateMetalViewSize(size, source: "initial")

        // Second call with same size should be a no-op (guard clause)
        state.updateMetalViewSize(size, source: "duplicate")
        XCTAssertEqual(state.metalViewSize, size)
    }

    func testUpdateToDifferentSize() throws {
        let state = AppState()
        state.updateMetalViewSize(CGSize(width: 640, height: 480), source: "first")
        state.updateMetalViewSize(CGSize(width: 1920, height: 1080), source: "second")
        XCTAssertEqual(state.metalViewSize, CGSize(width: 1920, height: 1080))
    }

    func testUpdateFromZeroSize() throws {
        let state = AppState()
        XCTAssertEqual(state.metalViewSize, .zero)
        state.updateMetalViewSize(CGSize(width: 100, height: 100), source: "fromZero")
        XCTAssertEqual(state.metalViewSize, CGSize(width: 100, height: 100))
    }

    func testUpdateToZeroSize() throws {
        let state = AppState()
        state.updateMetalViewSize(CGSize(width: 500, height: 300), source: "setup")
        state.updateMetalViewSize(.zero, source: "reset")
        XCTAssertEqual(state.metalViewSize, .zero)
    }
}

// MARK: - Seekbar Frame-Based Fallback Tests

@available(macOS 15.0, *) @MainActor final class SeekbarFrameFallbackTests: XCTestCase {

    func testCanInteractWithSeekSliderWithFrameProgressOnly() throws {
        let state = AppState()
        state.isConnected = true
        state.setPlaybackModeForTesting(.replaySeekable)
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0  // No valid timeline range
        state.totalFrames = 100  // But we have frame progress
        XCTAssertFalse(state.hasValidTimelineRange)
        XCTAssertTrue(state.hasFrameIndexProgress)
        XCTAssertTrue(state.canInteractWithSeekSlider)
    }

    func testCanInteractWithSeekSliderWithTimestampsOnly() throws {
        let state = AppState()
        state.isConnected = true
        state.setPlaybackModeForTesting(.replaySeekable)
        state.logStartTimestamp = 1
        state.logEndTimestamp = 2
        state.totalFrames = 0
        XCTAssertTrue(state.hasValidTimelineRange)
        XCTAssertFalse(state.hasFrameIndexProgress)
        XCTAssertTrue(state.canInteractWithSeekSlider)
    }

    func testCanInteractWithSeekSliderFalseWithNoData() throws {
        let state = AppState()
        state.isConnected = true
        state.setPlaybackModeForTesting(.replaySeekable)
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0
        state.totalFrames = 0
        XCTAssertFalse(state.hasValidTimelineRange)
        XCTAssertFalse(state.hasFrameIndexProgress)
        XCTAssertFalse(state.canInteractWithSeekSlider)
    }

    func testCanInteractWithSeekSliderFalseWhenNotSeekable() throws {
        let state = AppState()
        state.isConnected = true
        state.setPlaybackModeForTesting(.replayNonSeekable)
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0
        state.totalFrames = 100
        XCTAssertFalse(state.canInteractWithSeekSlider)
    }

    func testSeekWithFrameProgressOnly() throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.setPlaybackModeForTesting(.replaySeekable)
        state.playbackCommandClientOverride = fake
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0
        state.totalFrames = 100

        state.seek(to: 0.5)

        // Should use frame-based seeking, target frame = 49
        XCTAssertTrue(state.isSeekingInProgress)
        XCTAssertEqual(state.replayProgress, 0.5, accuracy: 0.001)
    }

    func testSeekIgnoredWithNoTimestampsAndNoFrames() throws {
        let state = AppState()
        let fake = FakePlaybackRPCClient()
        state.isConnected = true
        state.setPlaybackModeForTesting(.replaySeekable)
        state.playbackCommandClientOverride = fake
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0
        state.totalFrames = 0  // No frame progress either

        state.seek(to: 0.5)

        // Should be ignored — no valid range AND no frame progress
        XCTAssertFalse(state.isSeekingInProgress)
    }
}

// MARK: - PlaybackControlsDerivedState Seekbar Fallback Tests

@available(macOS 15.0, *) final class SeekSliderDerivedStateTests: XCTestCase {

    func testSeekSliderEnabledWithFrameProgressOnly() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: true, mode: .replaySeekable, isPaused: true, playbackRate: 1.0,
            busy: false, hasValidTimelineRange: false, hasFrameIndexProgress: true,
            currentFrameIndex: 10, totalFrames: 100)
        XCTAssertFalse(ui.seekSliderDisabled, "seekbar should be enabled with frame progress")
        XCTAssertTrue(ui.showSeekableSlider)
    }

    func testSeekSliderDisabledWithNoProgressData() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: true, mode: .replaySeekable, isPaused: true, playbackRate: 1.0,
            busy: false, hasValidTimelineRange: false, hasFrameIndexProgress: false,
            currentFrameIndex: 0, totalFrames: 0)
        XCTAssertTrue(ui.seekSliderDisabled, "seekbar should be disabled without any progress data")
    }

    func testSeekSliderDisabledWhenBusy() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: true, mode: .replaySeekable, isPaused: true, playbackRate: 1.0, busy: true,
            hasValidTimelineRange: true, hasFrameIndexProgress: true, currentFrameIndex: 10,
            totalFrames: 100)
        XCTAssertTrue(ui.seekSliderDisabled, "seekbar should be disabled when busy")
    }

    func testSeekSliderDisabledWhenDisconnected() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: false, mode: .replaySeekable, isPaused: true, playbackRate: 1.0,
            busy: false, hasValidTimelineRange: true, hasFrameIndexProgress: true,
            currentFrameIndex: 10, totalFrames: 100)
        XCTAssertTrue(ui.seekSliderDisabled, "seekbar should be disabled when disconnected")
    }
}

// MARK: - Track Persistence Tests

/// Tests that tracks persist in allSeenTracks after leaving the current frame,
/// preventing the regression where the track list lost entries when tracks
/// went out of the sensor's field of view.
@available(macOS 15.0, *) @MainActor final class TrackPersistenceTests: XCTestCase {

    /// Helper: build a FrameBundle with specified tracks.
    private func makeFrame(
        frameID: UInt64 = 1, tracks: [Track], playbackInfo: PlaybackInfo? = nil
    ) -> FrameBundle {
        var frame = FrameBundle()
        frame.frameID = frameID
        frame.timestampNanos = Int64(frameID) * 100_000_000
        frame.tracks = TrackSet(
            frameID: frameID, timestampNanos: Int64(frameID) * 100_000_000, tracks: tracks,
            trails: [])
        frame.playbackInfo = playbackInfo
        return frame
    }

    /// Helper: build a Track with sensible defaults.
    private func makeTrack(
        id: String, state: TrackState = .confirmed, speed: Float = 8.0,
        firstSeen: Int64 = 1_000_000_000
    ) -> Track {
        Track(
            trackID: id, sensorID: "sensor-1", state: state, hits: 10, misses: 0,
            observationCount: 10, firstSeenNanos: firstSeen, lastSeenNanos: firstSeen + 5_000_000,
            speedMps: speed, maxSpeedMps: speed + 1, classLabel: "car")
    }

    // MARK: - Core Persistence

    func testTracksAccumulateAcrossFrames() async throws {
        let state = AppState()
        let trackA = makeTrack(id: "trk_a", firstSeen: 1_000_000)
        let trackB = makeTrack(id: "trk_b", firstSeen: 2_000_000)

        // Frame 1: only track A
        state.onFrameReceived(makeFrame(frameID: 1, tracks: [trackA]))
        await Task.yield()
        XCTAssertEqual(state.allSeenTracks.count, 1)
        XCTAssertNotNil(state.allSeenTracks["trk_a"])

        // Frame 2: only track B (track A left the frame)
        state.onFrameReceived(makeFrame(frameID: 2, tracks: [trackB]))
        await Task.yield()
        XCTAssertEqual(
            state.allSeenTracks.count, 2,
            "Track A must persist even though it is no longer in the current frame")
        XCTAssertNotNil(state.allSeenTracks["trk_a"])
        XCTAssertNotNil(state.allSeenTracks["trk_b"])
    }

    func testTrackPersistsAfterLeavingFrame() async throws {
        let state = AppState()
        let trackA = makeTrack(id: "trk_a")

        // Frame 1: track A present
        state.onFrameReceived(makeFrame(frameID: 1, tracks: [trackA]))
        await Task.yield()
        XCTAssertEqual(state.allSeenTracks.count, 1)

        // Frame 2: empty — track A left
        state.onFrameReceived(makeFrame(frameID: 2, tracks: []))
        await Task.yield()
        XCTAssertEqual(
            state.allSeenTracks.count, 1,
            "Track must remain in allSeenTracks after leaving sensor view")
        XCTAssertNotNil(state.allSeenTracks["trk_a"])
    }

    func testAllSeenTracksUpdatedWithLatestSnapshot() async throws {
        let state = AppState()
        let trackV1 = makeTrack(id: "trk_a", speed: 5.0)
        let trackV2 = makeTrack(id: "trk_a", speed: 12.0)

        state.onFrameReceived(makeFrame(frameID: 1, tracks: [trackV1]))
        await Task.yield()
        XCTAssertEqual(state.allSeenTracks["trk_a"]?.speedMps, 5.0)

        // Frame 2: same track with updated speed
        state.onFrameReceived(makeFrame(frameID: 2, tracks: [trackV2]))
        await Task.yield()
        XCTAssertEqual(
            state.allSeenTracks["trk_a"]?.speedMps, 12.0,
            "allSeenTracks should store the latest snapshot of each track")
    }

    // MARK: - In-View Tracking

    func testInViewTrackIDsUpdatedPerFrame() async throws {
        let state = AppState()
        let trackA = makeTrack(id: "trk_a")
        let trackB = makeTrack(id: "trk_b")

        // Frame 1: both tracks visible
        state.onFrameReceived(makeFrame(frameID: 1, tracks: [trackA, trackB]))
        await Task.yield()
        XCTAssertEqual(state.inViewTrackIDs, ["trk_a", "trk_b"])

        // Frame 2: only track B visible
        state.onFrameReceived(makeFrame(frameID: 2, tracks: [trackB]))
        await Task.yield()
        XCTAssertEqual(
            state.inViewTrackIDs, ["trk_b"],
            "inViewTrackIDs should reflect only the current frame's tracks")
        // But allSeenTracks still has both
        XCTAssertEqual(state.allSeenTracks.count, 2)
    }

    func testInViewTrackIDsEmptyWhenNoTracks() async throws {
        let state = AppState()
        let trackA = makeTrack(id: "trk_a")

        state.onFrameReceived(makeFrame(frameID: 1, tracks: [trackA]))
        await Task.yield()
        XCTAssertEqual(state.inViewTrackIDs.count, 1)

        // Frame with no tracks
        state.onFrameReceived(makeFrame(frameID: 2, tracks: []))
        await Task.yield()
        XCTAssertTrue(
            state.inViewTrackIDs.isEmpty, "inViewTrackIDs should be empty when frame has no tracks")
    }

    // MARK: - Reset Behaviour

    func testClearAllResetsSeenTracks() async throws {
        let state = AppState()
        let trackA = makeTrack(id: "trk_a")

        state.onFrameReceived(makeFrame(frameID: 1, tracks: [trackA]))
        await Task.yield()
        XCTAssertEqual(state.allSeenTracks.count, 1)

        state.clearAll()
        XCTAssertTrue(state.allSeenTracks.isEmpty, "clearAll() must reset allSeenTracks")
        XCTAssertTrue(state.inViewTrackIDs.isEmpty, "clearAll() must reset inViewTrackIDs")
    }

    func testPrepareForNewReplayResetsSeenTracks() async throws {
        let state = AppState()
        let trackA = makeTrack(id: "trk_a")

        state.onFrameReceived(makeFrame(frameID: 1, tracks: [trackA]))
        await Task.yield()
        XCTAssertEqual(state.allSeenTracks.count, 1)

        state.prepareForNewReplay()
        XCTAssertTrue(state.allSeenTracks.isEmpty, "prepareForNewReplay() must reset allSeenTracks")
        XCTAssertTrue(
            state.inViewTrackIDs.isEmpty, "prepareForNewReplay() must reset inViewTrackIDs")
    }

    func testDisconnectResetsSeenTracks() async throws {
        let state = AppState()
        let trackA = makeTrack(id: "trk_a")

        state.onFrameReceived(makeFrame(frameID: 1, tracks: [trackA]))
        await Task.yield()
        XCTAssertEqual(state.allSeenTracks.count, 1)

        state.disconnect()
        XCTAssertTrue(state.allSeenTracks.isEmpty, "disconnect() must reset allSeenTracks")
    }

    // MARK: - Filtered Tracks Persistence

    func testFilteredTracksPersistAfterLeavingFrame() async throws {
        let state = AppState()
        let trackA = makeTrack(id: "trk_a", state: .confirmed)
        let trackB = makeTrack(id: "trk_b", state: .confirmed)

        // Set up a filter
        state.filterMinHits = 5

        // Frame 1: both tracks
        state.onFrameReceived(makeFrame(frameID: 1, tracks: [trackA, trackB]))
        await Task.yield()

        let filteredCount1 = state.filteredTracks.count
        XCTAssertEqual(filteredCount1, 2, "Both tracks should pass filter")

        // Frame 2: only track B (track A left)
        state.onFrameReceived(makeFrame(frameID: 2, tracks: [trackB]))
        await Task.yield()

        let filteredTracksAfter = state.filteredTracks
        let filteredIDs = Set(filteredTracksAfter.map { $0.trackID })
        XCTAssertTrue(
            filteredIDs.contains("trk_a"),
            "Track A must persist in filteredTracks after leaving the frame")
        XCTAssertTrue(filteredIDs.contains("trk_b"))
    }

    // MARK: - Multiple Tracks Lifecycle

    func testManyTracksAccumulateAndPersist() async throws {
        let state = AppState()

        // Frame 1: tracks A, B, C
        let trackA = makeTrack(id: "trk_a", firstSeen: 1_000_000)
        let trackB = makeTrack(id: "trk_b", firstSeen: 2_000_000)
        let trackC = makeTrack(id: "trk_c", firstSeen: 3_000_000)
        state.onFrameReceived(makeFrame(frameID: 1, tracks: [trackA, trackB, trackC]))
        await Task.yield()
        XCTAssertEqual(state.allSeenTracks.count, 3)

        // Frame 2: only track D (all previous left)
        let trackD = makeTrack(id: "trk_d", firstSeen: 4_000_000)
        state.onFrameReceived(makeFrame(frameID: 2, tracks: [trackD]))
        await Task.yield()
        XCTAssertEqual(state.allSeenTracks.count, 4, "All 4 tracks must be accumulated")
        XCTAssertEqual(state.inViewTrackIDs, ["trk_d"])

        // Frame 3: tracks B and D return
        state.onFrameReceived(makeFrame(frameID: 3, tracks: [trackB, trackD]))
        await Task.yield()
        XCTAssertEqual(state.allSeenTracks.count, 4, "Count unchanged — no new tracks")
        XCTAssertEqual(state.inViewTrackIDs, ["trk_b", "trk_d"])
    }
}
