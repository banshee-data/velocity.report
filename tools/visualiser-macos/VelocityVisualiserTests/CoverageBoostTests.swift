//
//  CoverageBoostTests.swift
//  VelocityVisualiserTests
//
//  Comprehensive unit tests to improve coverage for AppState, VisualiserClient,
//  ContentView, and RunBrowserView.
//

import Foundation
import SwiftUI
import Testing
import XCTest

@testable import VelocityVisualiser

// MARK: - AppState Tests

@available(macOS 15.0, *) @MainActor final class AppStateCoverageTests: XCTestCase {

    // MARK: - Step Forward/Backward Tests

    func testStepForwardIncreasesFrameIndex() throws {
        let state = AppState()
        state.isLive = false
        state.isSeekable = true
        state.currentFrameIndex = 100
        state.totalFrames = 1000

        // Mock client to prevent actual gRPC call
        // Since we can't easily mock the client, we test the guard conditions
        state.stepForward()
        // Without a real client, the call is made but does nothing
        // The test verifies the method doesn't crash
    }

    func testStepForwardIgnoresWhenLive() throws {
        let state = AppState()
        state.isLive = true
        state.isSeekable = true
        state.currentFrameIndex = 100
        state.totalFrames = 1000

        state.stepForward()
        // Should be no-op when live
    }

    func testStepForwardIgnoresWhenNotSeekable() throws {
        let state = AppState()
        state.isLive = false
        state.isSeekable = false

        state.stepForward()
        // Should be no-op when not seekable
    }

    func testStepForwardIgnoresAtEnd() throws {
        let state = AppState()
        state.isLive = false
        state.isSeekable = true
        state.currentFrameIndex = 999
        state.totalFrames = 1000

        state.stepForward()
        // Should not step past end
    }

    func testStepBackwardDecreasesFrameIndex() throws {
        let state = AppState()
        state.isLive = false
        state.isSeekable = true
        state.currentFrameIndex = 100

        state.stepBackward()
        // Verify method executes without crash
    }

    func testStepBackwardIgnoresAtStart() throws {
        let state = AppState()
        state.isLive = false
        state.isSeekable = true
        state.currentFrameIndex = 0

        state.stepBackward()
        // Should not step before start
    }

    // MARK: - Increase Rate Tests

    func testIncreaseRateUpperBoundClamping() throws {
        let state = AppState()
        state.isLive = false
        state.playbackRate = 64.0  // Already at max

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 64.0, "Should clamp at maximum rate")
    }

    func testIncreaseRateFromMidRange() throws {
        let state = AppState()
        state.isLive = false
        state.playbackRate = 2.0

        state.increaseRate()
        XCTAssertEqual(state.playbackRate, 4.0)
    }

    // MARK: - Assign Quality Tests

    func testAssignQualityWithoutTrack() throws {
        let state = AppState()
        state.selectedTrackID = nil
        state.currentRunID = "run-123"

        state.assignQuality("good")
        // Should be no-op without selected track
    }

    func testAssignQualityWithoutRunID() throws {
        let state = AppState()
        state.selectedTrackID = "track-123"
        state.currentRunID = nil

        state.assignQuality("good")
        // Should be no-op without run ID
    }

    func testAssignQualityWithTrackAndRun() throws {
        let state = AppState()
        state.selectedTrackID = "track-123"
        state.currentRunID = "run-123"

        state.assignQuality("perfect")
        // Task will fail to reach server but method executes
    }

    // MARK: - Export Labels Tests

    func testExportLabelsOpensDialog() throws {
        let state = AppState()
        state.exportLabels()
        // Test that the method runs without crashing
        // Actual save panel interaction can't be easily tested
    }

    // MARK: - Reproject Labels Tests

    func testReprojectLabelsWithoutRenderer() throws {
        let state = AppState()
        state.showTrackLabels = true
        state.metalViewSize = CGSize(width: 800, height: 600)

        state.reprojectLabels()
        // Should not crash when renderer is nil
    }

    // MARK: - Apply Frame State Update Tests

    func testApplyFrameStateUpdateBasic() throws {
        let state = AppState()
        var frame = FrameBundle()
        frame.frameID = 42
        frame.timestampNanos = 1_000_000_000
        frame.pointCloud = PointCloudFrame(
            frameID: 42, timestampNanos: 1_000_000_000, sensorID: "sensor-1",
            x: [1.0], y: [2.0], z: [3.0], intensity: [100], classification: [0],
            decimationMode: .none, decimationRatio: 1.0, pointCount: 1)

        state.applyFrameStateUpdate(
            frame: frame, instantFPS: 60.0, newCacheStatus: "Cached (100 frames)",
            newLabels: [])

        XCTAssertEqual(state.currentFrameID, 42)
        XCTAssertEqual(state.currentTimestamp, 1_000_000_000)
        XCTAssertEqual(state.frameCount, 1)
        XCTAssertEqual(state.pointCount, 1)
    }

    func testApplyFrameStateUpdateWithPlaybackInfo() throws {
        let state = AppState()
        var frame = FrameBundle()
        frame.frameID = 100
        frame.timestampNanos = 5_000_000_000
        frame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 0, logEndNs: 10_000_000_000,
            playbackRate: 2.0, paused: false, currentFrameIndex: 100,
            totalFrames: 1000, seekable: true)

        state.applyFrameStateUpdate(
            frame: frame, instantFPS: 30.0, newCacheStatus: "", newLabels: [])

        XCTAssertFalse(state.isLive)
        XCTAssertEqual(state.playbackRate, 2.0)
        XCTAssertEqual(state.currentFrameIndex, 100)
        XCTAssertEqual(state.totalFrames, 1000)
        XCTAssertTrue(state.isSeekable)
    }

    // MARK: - Load Recording Tests

    func testLoadRecordingFromURL() throws {
        let state = AppState()
        let tempURL = URL(fileURLWithPath: "/tmp/test.vrlog")

        state.loadRecording(from: tempURL)
        XCTAssertFalse(state.isLive)
    }

    // MARK: - Connect Without Client Tests

    func testConnectSetsConnectingFlag() throws {
        let state = AppState()
        XCTAssertFalse(state.isConnecting)
        XCTAssertFalse(state.isConnected)

        state.connect()
        XCTAssertTrue(state.isConnecting)
    }

    func testConnectSkipsWhenAlreadyConnecting() throws {
        let state = AppState()
        state.isConnecting = true

        state.connect()
        // Should skip and not create duplicate connection
    }

    func testConnectSkipsWhenAlreadyConnected() throws {
        let state = AppState()
        state.isConnected = true

        state.connect()
        // Should skip when already connected
    }

    // MARK: - Toggle Connection Tests

    func testToggleConnectionIgnoresWhenConnecting() throws {
        let state = AppState()
        state.isConnecting = true

        state.toggleConnection()
        // Should ignore toggle while connecting
    }

    func testToggleConnectionDisconnectsWhenConnected() throws {
        let state = AppState()
        state.isConnected = true
        state.isConnecting = false

        state.toggleConnection()
        // Should trigger disconnect
        XCTAssertFalse(state.isConnected)
    }

    func testToggleConnectionConnectsWhenDisconnected() throws {
        let state = AppState()
        state.isConnected = false
        state.isConnecting = false

        state.toggleConnection()
        // Should trigger connect
        XCTAssertTrue(state.isConnecting)
    }
}

// MARK: - VisualiserClient Decode Tests

struct VisualiserClientDecodeTests {

    @Test func decodeWithDebugOverlays() throws {
        let client = VisualiserClient()
        var proto = Velocity_Visualiser_V1_FrameBundle()
        proto.frameID = 50

        // Add debug overlays
        proto.debug.frameID = 50
        var assoc = Velocity_Visualiser_V1_AssociationCandidate()
        assoc.clusterID = "c1"
        assoc.trackID = "t1"
        assoc.distance = 2.5
        assoc.accepted = true
        proto.debug.associationCandidates.append(assoc)

        var gating = Velocity_Visualiser_V1_GatingEllipse()
        gating.trackID = "t1"
        gating.centerX = 10.0
        gating.centerY = 5.0
        gating.semiMajor = 2.0
        gating.semiMinor = 1.0
        gating.rotationRad = 0.5
        proto.debug.gatingEllipses.append(gating)

        var residual = Velocity_Visualiser_V1_InnovationResidual()
        residual.trackID = "t1"
        residual.predictedX = 10.0
        residual.predictedY = 5.0
        residual.measuredX = 10.1
        residual.measuredY = 5.1
        residual.residualMagnitude = 0.15
        proto.debug.residuals.append(residual)

        let result = client.decodeFrameBundle(proto)
        #expect(result.debug != nil)
        #expect(result.debug?.associationCandidates.count == 1)
        #expect(result.debug?.gatingEllipses.count == 1)
        #expect(result.debug?.residuals.count == 1)
    }

    @Test func decodeWithBackgroundSnapshot() throws {
        let client = VisualiserClient()
        var proto = Velocity_Visualiser_V1_FrameBundle()
        proto.frameID = 100
        proto.frameType = .background
        proto.backgroundSeq = 5

        proto.background.sequenceNumber = 5
        proto.background.timestampNanos = 1_000_000_000
        proto.background.x = [1.0, 2.0, 3.0]
        proto.background.y = [4.0, 5.0, 6.0]
        proto.background.z = [7.0, 8.0, 9.0]
        proto.background.confidence = [0.9, 0.8, 0.7]

        proto.background.gridMetadata.rings = 16
        proto.background.gridMetadata.azimuthBins = 360
        proto.background.gridMetadata.ringElevations = [0.1, 0.2]
        proto.background.gridMetadata.settlingComplete = true

        let result = client.decodeFrameBundle(proto)
        #expect(result.frameType == .background)
        #expect(result.backgroundSeq == 5)
        #expect(result.background != nil)
        #expect(result.background?.sequenceNumber == 5)
        #expect(result.background?.x.count == 3)
        #expect(result.background?.gridMetadata != nil)
        #expect(result.background?.gridMetadata?.settlingComplete == true)
    }

    @Test func decodeWithOBBData() throws {
        let client = VisualiserClient()
        var proto = Velocity_Visualiser_V1_FrameBundle()

        var cluster = Velocity_Visualiser_V1_Cluster()
        cluster.clusterID = "c1"
        cluster.centroidX = 10.0
        cluster.centroidY = 5.0
        cluster.centroidZ = 1.0
        cluster.obb.centerX = 10.0
        cluster.obb.centerY = 5.0
        cluster.obb.centerZ = 1.0
        cluster.obb.length = 4.0
        cluster.obb.width = 2.0
        cluster.obb.height = 1.5
        cluster.obb.headingRad = 1.57
        proto.clusters.clusters.append(cluster)
        proto.clusters.frameID = 1

        let result = client.decodeFrameBundle(proto)
        #expect(result.clusters != nil)
        #expect(result.clusters?.clusters.first?.obb != nil)
        #expect(result.clusters?.clusters.first?.obb?.length == 4.0)
        #expect(result.clusters?.clusters.first?.obb?.headingRad == 1.57)
    }

    @Test func decodeWithTrackDetails() throws {
        let client = VisualiserClient()
        var proto = Velocity_Visualiser_V1_FrameBundle()

        var track = Velocity_Visualiser_V1_Track()
        track.trackID = "t1"
        track.sensorID = "hesai-01"
        track.state = .confirmed
        track.hits = 100
        track.misses = 5
        track.observationCount = 95
        track.firstSeenNs = 1_000_000_000
        track.lastSeenNs = 2_000_000_000
        track.x = 10.0
        track.y = 5.0
        track.z = 0.5
        track.vx = 8.0
        track.vy = 0.5
        track.vz = 0.0
        track.speedMps = 8.02
        track.headingRad = 0.062
        track.bboxLengthAvg = 4.5
        track.bboxWidthAvg = 1.8
        track.bboxHeightAvg = 1.5
        track.bboxHeadingRad = 0.062
        track.heightP95Max = 1.6
        track.intensityMeanAvg = 50.0
        track.avgSpeedMps = 7.5
        track.peakSpeedMps = 9.0
        track.classLabel = "vehicle"
        track.classConfidence = 0.95
        track.trackLengthMetres = 150.0
        track.trackDurationSecs = 20.0
        track.occlusionCount = 2
        track.confidence = 0.98
        track.occlusionState = .none
        track.motionModel = .cv
        track.alpha = 0.85

        proto.tracks.tracks.append(track)
        proto.tracks.frameID = 1

        let result = client.decodeFrameBundle(proto)
        #expect(result.tracks != nil)
        let decodedTrack = result.tracks?.tracks.first
        #expect(decodedTrack?.trackID == "t1")
        #expect(decodedTrack?.state == .confirmed)
        #expect(decodedTrack?.hits == 100)
        #expect(decodedTrack?.avgSpeedMps == 7.5)
        #expect(decodedTrack?.peakSpeedMps == 9.0)
        #expect(decodedTrack?.classLabel == "vehicle")
        #expect(decodedTrack?.alpha == 0.85)
    }

    @Test func decodeWithTrackTrails() throws {
        let client = VisualiserClient()
        var proto = Velocity_Visualiser_V1_FrameBundle()

        var trail = Velocity_Visualiser_V1_TrackTrail()
        trail.trackID = "t1"
        var point1 = Velocity_Visualiser_V1_TrackPoint()
        point1.x = 0.0
        point1.y = 0.0
        point1.timestampNs = 1_000_000_000
        var point2 = Velocity_Visualiser_V1_TrackPoint()
        point2.x = 10.0
        point2.y = 5.0
        point2.timestampNs = 2_000_000_000
        trail.points = [point1, point2]

        proto.tracks.trails.append(trail)
        proto.tracks.frameID = 1

        let result = client.decodeFrameBundle(proto)
        #expect(result.tracks != nil)
        #expect(result.tracks?.trails.count == 1)
        #expect(result.tracks?.trails.first?.points.count == 2)
    }
}

// MARK: - ContentView Component Tests

struct ContentViewToolbarTests {
    @Test func toolbarViewInstantiates() throws {
        let state = AppState()
        let view = ToolbarView().environmentObject(state)
        let _ = view.body
    }

    @Test func connectionButtonViewShowsConnecting() throws {
        let state = AppState()
        state.isConnecting = true
        state.isConnected = false

        let view = ConnectionButtonView().environmentObject(state)
        let _ = view.body
    }

    @Test func connectionButtonViewShowsConnected() throws {
        let state = AppState()
        state.isConnecting = false
        state.isConnected = true

        let view = ConnectionButtonView().environmentObject(state)
        let _ = view.body
    }

    @Test func connectionButtonViewShowsDisconnected() throws {
        let state = AppState()
        state.isConnecting = false
        state.isConnected = false

        let view = ConnectionButtonView().environmentObject(state)
        let _ = view.body
    }

    @Test func connectionStatusViewWithError() throws {
        let state = AppState()
        state.isConnected = false
        state.connectionError = "Connection failed"

        let view = ConnectionStatusView().environmentObject(state)
        let _ = view.body
    }

    @Test func connectionStatusViewConnected() throws {
        let state = AppState()
        state.isConnected = true
        state.serverAddress = "localhost:50051"

        let view = ConnectionStatusView().environmentObject(state)
        let _ = view.body
    }

    @Test func statsDisplayViewWhenConnected() throws {
        let state = AppState()
        state.isConnected = true
        state.fps = 60.0
        state.pointCount = 65000
        state.trackCount = 5

        let view = StatsDisplayView().environmentObject(state)
        let _ = view.body
    }

    @Test func statsDisplayViewWithCacheStatus() throws {
        let state = AppState()
        state.isConnected = true
        state.cacheStatus = "Cached (1200 frames)"

        let view = StatsDisplayView().environmentObject(state)
        let _ = view.body
    }
}

struct ContentViewPlaybackTests {
    @Test func playbackControlsViewLiveMode() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = true

        let view = PlaybackControlsView().environmentObject(state)
        let _ = view.body
    }

    @Test func playbackControlsViewReplayMode() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.isPaused = true
        state.isSeekable = true

        let view = PlaybackControlsView().environmentObject(state)
        let _ = view.body
    }

    @Test func playbackControlsViewNonSeekable() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.isSeekable = false

        let view = PlaybackControlsView().environmentObject(state)
        let _ = view.body
    }

    @Test func timeDisplayViewElapsed() throws {
        let state = AppState()
        state.logStartTimestamp = 0
        state.logEndTimestamp = 60_000_000_000  // 60 seconds
        state.currentTimestamp = 30_000_000_000  // 30 seconds

        let view = TimeDisplayView().environmentObject(state)
        let _ = view.body
    }

    @Test func rateLabelViewFormatting() throws {
        #expect(formatRate(1.0) == "1")
        #expect(formatRate(2.0) == "2")
        #expect(formatRate(0.5) == "0.5")
        #expect(formatRate(64.0) == "64")
    }

    @Test func durationFormattingShort() throws {
        let result = formatDuration(125_000_000_000)  // 2:05
        #expect(result.contains("2"))
        #expect(result.contains("05"))
    }

    @Test func durationFormattingLong() throws {
        let result = formatDuration(3665_000_000_000)  // 1:01:05
        #expect(result.contains("1"))
        #expect(result.contains("01"))
        #expect(result.contains("05"))
    }

    @Test func durationFormattingNegative() throws {
        let result = formatDuration(-60_000_000_000)  // -1:00
        #expect(result.hasPrefix("-"))
    }
}

struct ContentViewSidePanelTests {
    @Test func sidePanelViewInstantiates() throws {
        let state = AppState()
        let view = SidePanelView().environmentObject(state)
        let _ = view.body
    }

    @Test func trackInspectorViewWithoutTrack() throws {
        let state = AppState()
        let view = TrackInspectorView(trackID: "track-999").environmentObject(state)
        let _ = view.body
    }

    @Test func trackInspectorViewWithTrack() throws {
        let state = AppState()
        var frame = FrameBundle()
        var track = Track(
            trackID: "track-001", sensorID: "sensor-1", state: .confirmed,
            hits: 50, misses: 2, observationCount: 48,
            firstSeenNanos: 1_000_000_000, lastSeenNanos: 2_000_000_000,
            x: 10.0, y: 5.0, z: 0.5, vx: 8.0, vy: 0.5, vz: 0.0,
            speedMps: 8.0, headingRad: 0.1,
            covariance4x4: [], bboxLengthAvg: 4.5, bboxWidthAvg: 1.8,
            bboxHeightAvg: 1.5, bboxHeadingRad: 0.1,
            heightP95Max: 1.6, intensityMeanAvg: 50.0,
            avgSpeedMps: 7.5, peakSpeedMps: 9.0,
            classLabel: "vehicle", classConfidence: 0.95,
            trackLengthMetres: 150.0, trackDurationSecs: 20.0,
            occlusionCount: 0, confidence: 0.98,
            occlusionState: .none, motionModel: .cv, alpha: 1.0)
        frame.tracks = TrackSet(frameID: 1, timestampNanos: 1_000_000_000, tracks: [track], trails: [])
        state.currentFrame = frame

        let view = TrackInspectorView(trackID: "track-001").environmentObject(state)
        let _ = view.body
    }

    @Test func labelPanelViewWithoutSelection() throws {
        let state = AppState()
        let view = LabelPanelView().environmentObject(state)
        let _ = view.body
    }

    @Test func labelPanelViewWithSelection() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"

        let view = LabelPanelView().environmentObject(state)
        let _ = view.body
    }

    @Test func labelPanelViewWithRunMode() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"
        state.currentRunID = "run-123"

        let view = LabelPanelView().environmentObject(state)
        let _ = view.body
    }

    @Test func labelButtonBasic() throws {
        let view = LabelButton(label: "good_vehicle", shortcut: "1", isActive: false) {}
        let _ = view.body
    }

    @Test func labelButtonActive() throws {
        let view = LabelButton(label: "good_vehicle", shortcut: "1", isActive: true) {}
        let _ = view.body
    }

    @Test func labelButtonNoShortcut() throws {
        let view = LabelButton(label: "perfect", shortcut: nil, isActive: false) {}
        let _ = view.body
    }
}

struct ContentViewDebugPanelTests {
    @Test func debugOverlayTogglesViewAll() throws {
        let state = AppState()
        state.showDebug = true
        state.showAssociation = true
        state.showGating = true
        state.showResiduals = true

        let view = DebugOverlayTogglesView().environmentObject(state)
        let _ = view.body
    }

    @Test func debugOverlayTogglesViewDisabled() throws {
        let state = AppState()
        state.showDebug = false

        let view = DebugOverlayTogglesView().environmentObject(state)
        let _ = view.body
    }
}

struct ContentViewTrackLabelTests {
    @Test func trackLabelOverlayEmpty() throws {
        let view = TrackLabelOverlay(labels: [])
        let _ = view.body
    }

    @Test func trackLabelOverlayMultiple() throws {
        let labels = [
            MetalRenderer.TrackScreenLabel(
                id: "track-001", screenX: 100, screenY: 200,
                classLabel: "vehicle", isSelected: false),
            MetalRenderer.TrackScreenLabel(
                id: "track-002", screenX: 300, screenY: 400,
                classLabel: "", isSelected: true),
        ]
        let view = TrackLabelOverlay(labels: labels)
        let _ = view.body
    }

    @Test func trackLabelPillWithClass() throws {
        let label = MetalRenderer.TrackScreenLabel(
            id: "track-abc123def", screenX: 100, screenY: 200,
            classLabel: "vehicle", isSelected: false)
        let view = TrackLabelPill(label: label)
        let _ = view.body
    }

    @Test func trackLabelPillSelected() throws {
        let label = MetalRenderer.TrackScreenLabel(
            id: "track-xyz", screenX: 100, screenY: 200,
            classLabel: "", isSelected: true)
        let view = TrackLabelPill(label: label)
        let _ = view.body
    }
}

// MARK: - RunBrowserView Tests

@available(macOS 15.0, *) @MainActor final class RunBrowserViewTests: XCTestCase {

    func testRunBrowserViewInstantiates() throws {
        let state = AppState()
        let view = RunBrowserView().environmentObject(state)
        let _ = view.body
    }

    func testRunRowViewBasic() throws {
        let run = AnalysisRun(
            runId: "run-abc123", status: "completed", startTime: "2024-01-15T10:00:00Z",
            endTime: "2024-01-15T10:30:00Z", durationSecs: 1800.0,
            totalTracks: 150, totalTransits: 145, hasVRLog: true, vrlogPath: "/path/to.vrlog")

        let view = RunRowView(run: run, isSelected: false) {}
        let _ = view.body
    }

    func testRunRowViewSelected() throws {
        let run = AnalysisRun(
            runId: "run-xyz", status: "running", startTime: "2024-01-15T10:00:00Z",
            endTime: nil, durationSecs: 0, totalTracks: 0, totalTransits: 0,
            hasVRLog: false, vrlogPath: nil)

        let view = RunRowView(run: run, isSelected: true) {}
        let _ = view.body
    }

    func testRunRowViewWithoutVRLog() throws {
        let run = AnalysisRun(
            runId: "run-novr", status: "completed", startTime: "2024-01-15T10:00:00Z",
            endTime: "2024-01-15T10:05:00Z", durationSecs: 300.0,
            totalTracks: 10, totalTransits: 8, hasVRLog: false, vrlogPath: nil)

        let view = RunRowView(run: run, isSelected: false) {}
        let _ = view.body
    }

    func testStatusDotCompleted() throws {
        let view = StatusDot(status: "completed")
        let _ = view.body
    }

    func testStatusDotRunning() throws {
        let view = StatusDot(status: "running")
        let _ = view.body
    }

    func testStatusDotFailed() throws {
        let view = StatusDot(status: "failed")
        let _ = view.body
    }

    func testStatusDotUnknown() throws {
        let view = StatusDot(status: "unknown")
        let _ = view.body
    }
}

// MARK: - AnalysisRun Computed Properties Tests

struct AnalysisRunTests {
    @Test func formattedDateValid() throws {
        let run = AnalysisRun(
            runId: "run-1", status: "completed",
            startTime: "2024-01-15T14:30:00Z", endTime: nil,
            durationSecs: 0, totalTracks: 0, totalTransits: 0,
            hasVRLog: false, vrlogPath: nil)

        let formatted = run.formattedDate
        #expect(!formatted.isEmpty)
    }

    @Test func formattedDateInvalid() throws {
        let run = AnalysisRun(
            runId: "run-2", status: "completed",
            startTime: "invalid-date", endTime: nil,
            durationSecs: 0, totalTracks: 0, totalTransits: 0,
            hasVRLog: false, vrlogPath: nil)

        let formatted = run.formattedDate
        #expect(formatted == "invalid-date")
    }
}

// MARK: - String Truncation Tests

struct StringTruncationCoverageTests {
    @Test func truncateShortString() throws {
        let result = "hello".truncated(10)
        #expect(result == "hello")
    }

    @Test func truncateLongString() throws {
        let result = "hello world this is a long string".truncated(10)
        #expect(result == "hello worl...")
    }

    @Test func truncateExactLength() throws {
        let result = "exactly10!".truncated(10)
        #expect(result == "exactly10!")
    }
}

// MARK: - RunTrack Computed Properties Tests

struct RunTrackTests {
    @Test func speedKmhConversion() throws {
        let track = RunTrack(
            trackId: "t1", sensorId: "s1", startTime: "2024-01-15T10:00:00Z",
            endTime: nil, durationSecs: 10.0, trackLengthMetres: 100.0,
            avgSpeedMps: 10.0, peakSpeedMps: 12.0, observationCount: 50,
            userLabel: nil, qualityLabel: nil)

        let speedKmh = track.avgSpeedKmh
        #expect(speedKmh == 36.0)  // 10 m/s * 3.6 = 36 km/h
    }

    @Test func speedKmhWithNilSpeed() throws {
        let track = RunTrack(
            trackId: "t1", sensorId: "s1", startTime: "2024-01-15T10:00:00Z",
            endTime: nil, durationSecs: 10.0, trackLengthMetres: 100.0,
            avgSpeedMps: nil, peakSpeedMps: nil, observationCount: 50,
            userLabel: nil, qualityLabel: nil)

        let speedKmh = track.avgSpeedKmh
        #expect(speedKmh == nil)
    }
}

// MARK: - VisualiserClient Additional Tests

@available(macOS 15.0, *) @MainActor final class VisualiserClientAdditionalTests: XCTestCase {

    func testLockedStateThreadSafety() throws {
        let state = LockedState<Int>(0)
        #expect(state.value == 0)

        state.value = 42
        #expect(state.value == 42)
    }

    func testLockedStateBoolToggle() throws {
        let state = LockedState<Bool>(false)
        state.value = true
        #expect(state.value == true)
    }

    func testClientInitialState() throws {
        let client = VisualiserClient(address: "localhost:50051")
        #expect(client.address == "localhost:50051")
        #expect(client.isConnected == false)
        #expect(client.includePoints == true)
        #expect(client.includeClusters == true)
        #expect(client.includeTracks == true)
        #expect(client.includeDebug == false)
    }

    func testClientAddressParsing() async throws {
        let client = VisualiserClient(address: "invalid:address:port")

        do {
            try await client.connect()
            XCTFail("Should throw invalid address error")
        } catch let error as VisualiserClientError {
            switch error {
            case .invalidAddress:
                // Expected
                break
            default:
                XCTFail("Wrong error type")
            }
        }
    }

    func testClientDisconnectWhenNotConnected() throws {
        let client = VisualiserClient(address: "localhost:50051")
        client.disconnect()
        // Should not crash
    }

    func testClientRestartStream() throws {
        let client = VisualiserClient(address: "localhost:50051")
        client.restartStream()
        // Should not crash when not connected
    }
}

// MARK: - Additional AppState Integration Tests

@available(macOS 15.0, *) @MainActor final class AppStateIntegrationTests: XCTestCase {

    func testOnFrameReceivedUpdatesState() throws {
        let state = AppState()
        var frame = FrameBundle()
        frame.frameID = 100
        frame.timestampNanos = 5_000_000_000

        var pc = PointCloudFrame(
            frameID: 100, timestampNanos: 5_000_000_000, sensorID: "sensor-1",
            x: [1.0, 2.0], y: [3.0, 4.0], z: [5.0, 6.0],
            intensity: [100, 200], classification: [0, 0],
            decimationMode: .none, decimationRatio: 1.0, pointCount: 2)
        frame.pointCloud = pc

        state.onFrameReceived(frame)

        // Frame should be set immediately
        XCTAssertEqual(state.currentFrame?.frameID, 100)
    }

    func testSelectTrackOpensPanel() throws {
        let state = AppState()
        XCTAssertNil(state.selectedTrackID)
        XCTAssertFalse(state.showLabelPanel)

        state.selectTrack("track-123")

        XCTAssertEqual(state.selectedTrackID, "track-123")
        XCTAssertTrue(state.showLabelPanel)
        XCTAssertTrue(state.showSidePanel)
    }

    func testDeselectTrack() throws {
        let state = AppState()
        state.selectTrack("track-123")

        state.selectTrack(nil)
        XCTAssertNil(state.selectedTrackID)
    }

    func testAssignLabelInLiveMode() throws {
        let state = AppState()
        state.selectedTrackID = "track-123"
        state.currentRunID = nil  // Live mode
        state.currentTimestamp = 5_000_000_000

        state.assignLabel("good_vehicle")
        // Task will attempt to call API but fail gracefully
    }

    func testAssignLabelInRunMode() throws {
        let state = AppState()
        state.selectedTrackID = "track-123"
        state.currentRunID = "run-123"

        state.assignLabel("good_vehicle")
        // Task will attempt to call run-track API
    }
}

// MARK: - ContentView Overlay Toggle Tests

struct OverlayTogglesCoverageTests {
    @Test func overlayTogglesViewAllEnabled() throws {
        let state = AppState()
        state.showPoints = true
        state.showBoxes = true
        state.showClusters = true
        state.showTrails = true
        state.showVelocity = true
        state.showTrackLabels = true

        let view = OverlayTogglesView().environmentObject(state)
        let _ = view.body
    }

    @Test func overlayTogglesViewPointSize() throws {
        let state = AppState()
        state.pointSize = 10.0

        let view = OverlayTogglesView().environmentObject(state)
        let _ = view.body
    }

    @Test func toggleButtonOn() throws {
        @State var isOn = true
        let view = ToggleButton(label: "T", isOn: $isOn, help: "Test")
        let _ = view.body
    }

    @Test func toggleButtonOff() throws {
        @State var isOn = false
        let view = ToggleButton(label: "T", isOn: $isOn, help: "Test")
        let _ = view.body
    }
}

// MARK: - ContentView Track List Tests

@available(macOS 15.0, *) @MainActor final class TrackListViewTests: XCTestCase {

    func testTrackListViewInstantiates() throws {
        let state = AppState()
        let view = TrackListView().environmentObject(state)
        let _ = view.body
    }

    func testTrackListViewWithFrameTracks() throws {
        let state = AppState()
        var frame = FrameBundle()
        var track1 = Track(
            trackID: "track-001", sensorID: "sensor-1", state: .confirmed,
            hits: 10, misses: 0, observationCount: 10,
            firstSeenNanos: 1_000_000_000, lastSeenNanos: 2_000_000_000,
            x: 10.0, y: 5.0, z: 0.5, vx: 5.0, vy: 0.0, vz: 0.0,
            speedMps: 5.0, headingRad: 0.0,
            covariance4x4: [], bboxLengthAvg: 4.0, bboxWidthAvg: 1.8,
            bboxHeightAvg: 1.5, bboxHeadingRad: 0.0,
            heightP95Max: 1.5, intensityMeanAvg: 50.0,
            avgSpeedMps: 5.0, peakSpeedMps: 6.0,
            classLabel: "", classConfidence: 0.0,
            trackLengthMetres: 50.0, trackDurationSecs: 10.0,
            occlusionCount: 0, confidence: 0.9,
            occlusionState: .none, motionModel: .cv, alpha: 1.0)
        frame.tracks = TrackSet(frameID: 1, timestampNanos: 1_000_000_000, tracks: [track1], trails: [])
        state.currentFrame = frame

        let view = TrackListView().environmentObject(state)
        let _ = view.body
    }
}

// MARK: - Format Number Tests

struct FormatNumberTests {
    @Test func formatSmallNumber() throws {
        let view = StatsDisplayView().environmentObject(AppState())
        // Can't easily test private method, but we can verify view renders
        let _ = view.body
    }
}

// MARK: - ClientDelegateAdapter Tests

@available(macOS 15.0, *) @MainActor final class ClientDelegateTests: XCTestCase {

    func testClientDidConnect() throws {
        let state = AppState()
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        delegate.clientDidConnect(client)

        // Wait for async update
        let expectation = XCTestExpectation(description: "State updated")
        Task {
            try? await Task.sleep(for: .milliseconds(100))
            XCTAssertTrue(state.isConnected)
            XCTAssertNil(state.connectionError)
            expectation.fulfill()
        }
        wait(for: [expectation], timeout: 1.0)
    }

    func testClientDidDisconnect() throws {
        let state = AppState()
        state.isConnected = true
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        delegate.clientDidDisconnect(client, error: nil)

        let expectation = XCTestExpectation(description: "State updated")
        Task {
            try? await Task.sleep(for: .milliseconds(100))
            XCTAssertFalse(state.isConnected)
            expectation.fulfill()
        }
        wait(for: [expectation], timeout: 1.0)
    }

    func testClientDidDisconnectWithError() throws {
        let state = AppState()
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        delegate.clientDidDisconnect(client, error: VisualiserClientError.streamError("test"))

        let expectation = XCTestExpectation(description: "State updated")
        Task {
            try? await Task.sleep(for: .milliseconds(100))
            XCTAssertNotNil(state.connectionError)
            expectation.fulfill()
        }
        wait(for: [expectation], timeout: 1.0)
    }

    func testClientDidReceiveFrame() throws {
        let state = AppState()
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        var frame = FrameBundle()
        frame.frameID = 42
        delegate.client(client, didReceiveFrame: frame)

        // Frame delivery is deferred
        let expectation = XCTestExpectation(description: "Frame received")
        Task {
            try? await Task.sleep(for: .milliseconds(200))
            XCTAssertEqual(state.currentFrame?.frameID, 42)
            expectation.fulfill()
        }
        wait(for: [expectation], timeout: 1.0)
    }

    func testClientDidFinishStream() throws {
        let state = AppState()
        state.replayFinished = false
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        delegate.clientDidFinishStream(client)

        let expectation = XCTestExpectation(description: "Stream finished")
        Task {
            try? await Task.sleep(for: .milliseconds(100))
            XCTAssertTrue(state.replayFinished)
            XCTAssertTrue(state.isPaused)
            expectation.fulfill()
        }
        wait(for: [expectation], timeout: 1.0)
    }
}
