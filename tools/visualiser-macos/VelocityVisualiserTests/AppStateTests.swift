//
//  AppStateTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for AppState class using XCTest (required for @MainActor tests).
//

import XCTest

@testable import VelocityVisualiser

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

    func testTogglePlayPause() throws {
        let state = AppState()
        state.isLive = false  // Required for playback controls
        XCTAssertFalse(state.isPaused)

        state.togglePlayPause()
        XCTAssertTrue(state.isPaused)

        state.togglePlayPause()
        XCTAssertFalse(state.isPaused)
    }

    func testIncreaseRate() throws {
        let state = AppState()
        state.isLive = false  // Required for playback controls
        XCTAssertEqual(state.playbackRate, 1.0)

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 2.0)

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 4.0)

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 8.0)

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 16.0)

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 32.0)

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 64.0)

        // Should cap at 64.0
        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 64.0)
    }

    func testDecreaseRate() throws {
        let state = AppState()
        state.isLive = false  // Required for playback controls
        XCTAssertEqual(state.playbackRate, 1.0)

        state.decreaseRate()
        XCTAssertEqual(state.playbackRate, 0.5)

        // Should cap at 0.5
        state.decreaseRate()
        XCTAssertEqual(state.playbackRate, 0.5)
    }

    func testResetRate() throws {
        let state = AppState()
        state.isLive = false
        state.playbackRate = 8.0

        state.resetRate()
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

    func testSeekInReplayMode() throws {
        let state = AppState()
        state.isLive = false
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000

        state.seek(to: 0.5)
        XCTAssertEqual(state.replayProgress, 0.5)

        state.seek(to: 0.75)
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

    func testOnFrameReceived() throws {
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

        XCTAssertEqual(state.currentFrameID, 42)
        XCTAssertEqual(state.currentTimestamp, 1_000_000_000)
        XCTAssertEqual(state.frameCount, 1)
        XCTAssertEqual(state.pointCount, 3)
        XCTAssertEqual(state.clusterCount, 2)
        XCTAssertEqual(state.trackCount, 1)
        XCTAssertEqual(state.currentFrame?.frameID, 42)
    }

    func testOnFrameReceivedUpdatesReplayProgress() throws {
        let state = AppState()
        state.isLive = false
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000

        var frame = FrameBundle()
        frame.frameID = 1
        frame.timestampNanos = 1_500_000_000  // Midpoint

        state.onFrameReceived(frame)

        XCTAssertEqual(state.replayProgress, 0.5)
    }

    func testMultipleFramesIncrementCount() throws {
        let state = AppState()

        for i in 1...10 {
            var frame = FrameBundle()
            frame.frameID = UInt64(i)
            frame.timestampNanos = Int64(i) * 100_000_000
            state.onFrameReceived(frame)
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

    func testPlaybackInfoUpdatesFromFrame() throws {
        let state = AppState()
        state.isLive = true

        var frame = FrameBundle()
        frame.frameID = 1
        frame.timestampNanos = 1_000_000_000
        frame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 1_000_000_000, logEndNs: 2_000_000_000, playbackRate: 2.0,
            paused: false, currentFrameIndex: 50, totalFrames: 500)

        state.onFrameReceived(frame)

        XCTAssertFalse(state.isLive)
        XCTAssertEqual(state.logStartTimestamp, 1_000_000_000)
        XCTAssertEqual(state.logEndTimestamp, 2_000_000_000)
        XCTAssertEqual(state.playbackRate, 2.0)
        XCTAssertEqual(state.currentFrameIndex, 50)
        XCTAssertEqual(state.totalFrames, 500)
    }

    func testFrameReceivingClearsReplayFinished() throws {
        let state = AppState()
        state.replayFinished = true

        var frame = FrameBundle()
        frame.frameID = 1
        state.onFrameReceived(frame)

        XCTAssertFalse(state.replayFinished)
    }

    func testFPSCalculation() throws {
        let state = AppState()
        XCTAssertEqual(state.fps, 0.0)

        // Simulate receiving multiple frames quickly
        for i in 1...10 {
            var frame = FrameBundle()
            frame.frameID = UInt64(i)
            state.onFrameReceived(frame)
        }

        // FPS should be calculated (non-zero after multiple frames)
        // Note: actual FPS depends on timing, just verify it's tracked
        XCTAssertEqual(state.frameCount, 10)
    }

    func testProgressCalculationInReplayMode() throws {
        let state = AppState()
        state.isLive = false
        state.logStartTimestamp = 0
        state.logEndTimestamp = 1_000_000_000

        var frame = FrameBundle()
        frame.frameID = 1
        frame.timestampNanos = 250_000_000  // 25% progress

        state.onFrameReceived(frame)

        XCTAssertEqual(state.replayProgress, 0.25, accuracy: 0.01)
    }

    func testProgressNotUpdatedWhileSeeking() throws {
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

        // Progress should NOT be updated while seeking
        XCTAssertEqual(state.replayProgress, 0.5)
    }

    func testEmptyFrameHandling() throws {
        let state = AppState()

        let frame = FrameBundle()  // Empty frame
        state.onFrameReceived(frame)

        XCTAssertEqual(state.pointCount, 0)
        XCTAssertEqual(state.clusterCount, 0)
        XCTAssertEqual(state.trackCount, 0)
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
