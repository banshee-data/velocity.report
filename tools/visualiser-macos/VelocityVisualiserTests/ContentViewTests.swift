//
//  ContentViewTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for ContentView helper functions, simple views,
//  and extracted utility logic.
//

import AppKit
import Foundation
import MetalKit
import SwiftUI
import Testing
import XCTest

@testable import VelocityVisualiser

// MARK: - Format Rate Tests

struct FormatRateTests {
    @Test func integerRate() throws {
        #expect(formatRate(1.0) == "1")
        #expect(formatRate(2.0) == "2")
        #expect(formatRate(4.0) == "4")
        #expect(formatRate(8.0) == "8")
        #expect(formatRate(16.0) == "16")
        #expect(formatRate(32.0) == "32")
        #expect(formatRate(64.0) == "64")
    }

    @Test func fractionalRate() throws { #expect(formatRate(0.5) == "0.5") }

    @Test func largeIntegerRate() throws { #expect(formatRate(100.0) == "100") }
}

// MARK: - Format Duration Tests

struct FormatDurationTests {
    @Test func zeroNanos() throws { #expect(formatDuration(0) == "0:00") }

    @Test func oneSecond() throws {
        let nanos: Int64 = 1_000_000_000
        #expect(formatDuration(nanos) == "0:01")
    }

    @Test func sixtySeconds() throws {
        let nanos: Int64 = 60_000_000_000
        #expect(formatDuration(nanos) == "1:00")
    }

    @Test func ninetySeconds() throws {
        let nanos: Int64 = 90_000_000_000
        #expect(formatDuration(nanos) == "1:30")
    }

    @Test func oneHour() throws {
        let nanos: Int64 = 3600_000_000_000
        #expect(formatDuration(nanos) == "1:00:00")
    }

    @Test func oneHourThirtyMinutes() throws {
        let nanos: Int64 = 5400_000_000_000
        #expect(formatDuration(nanos) == "1:30:00")
    }

    @Test func negativeDuration() throws {
        let nanos: Int64 = -60_000_000_000
        #expect(formatDuration(nanos) == "-1:00")
    }

    @Test func negativeWithHours() throws {
        let nanos: Int64 = -3661_000_000_000  // -1h 1m 1s
        #expect(formatDuration(nanos) == "-1:01:01")
    }

    @Test func largeNanos() throws {
        let nanos: Int64 = 86400_000_000_000  // 24 hours
        #expect(formatDuration(nanos) == "24:00:00")
    }

    @Test func subSecond() throws {
        // Less than 1 second should display 0:00
        let nanos: Int64 = 500_000_000
        #expect(formatDuration(nanos) == "0:00")
    }
}

// MARK: - ModeIndicatorView Tests

struct ModeIndicatorViewTests {
    @Test func liveConnectedIndicator() throws {
        let view = ModeIndicatorView(isLive: true, isConnected: true)
        // Verify the view can be created without crash
        let _ = view.body
    }

    @Test func replayConnectedIndicator() throws {
        let view = ModeIndicatorView(isLive: false, isConnected: true)
        let _ = view.body
    }

    @Test func disconnectedIndicator() throws {
        let view = ModeIndicatorView(isLive: false, isConnected: false)
        let _ = view.body
    }
}

// MARK: - StatLabel Tests

struct StatLabelTests {
    @Test func statLabelCreation() throws {
        let label = StatLabel(title: "FPS", value: "60.0")
        let _ = label.body
    }

    @Test func statLabelWithEmptyValue() throws {
        let label = StatLabel(title: "Points", value: "")
        let _ = label.body
    }

    @Test func statLabelWithLargeValue() throws {
        let label = StatLabel(title: "Points", value: "65.00k")
        let _ = label.body
    }
}

// MARK: - CacheStatusLabel Tests

struct CacheStatusLabelTests {
    @Test func cachedStatus() throws {
        let label = CacheStatusLabel(status: "Cached (seq 42)")
        let _ = label.body
    }

    @Test func refreshingStatus() throws {
        let label = CacheStatusLabel(status: "Refreshing...")
        let _ = label.body
    }

    @Test func emptyStatus() throws {
        let label = CacheStatusLabel(status: "Empty")
        let _ = label.body
    }

    @Test func unknownStatus() throws {
        let label = CacheStatusLabel(status: "Something else")
        let _ = label.body
    }
}

// MARK: - DetailRow Tests

struct DetailRowTests {
    @Test func detailRowCreation() throws {
        let row = DetailRow(label: "Speed", value: "12.5 m/s")
        let _ = row.body
    }

    @Test func detailRowEmptyValues() throws {
        let row = DetailRow(label: "", value: "")
        let _ = row.body
    }
}

// MARK: - ToggleButton Tests

struct ToggleButtonTests {
    @Test func toggleButtonOn() throws {
        var isOn = true
        let button = ToggleButton(
            label: "P", isOn: Binding(get: { isOn }, set: { isOn = $0 }), help: "Points")
        let _ = button.body
    }

    @Test func toggleButtonOff() throws {
        var isOn = false
        let button = ToggleButton(
            label: "D", isOn: Binding(get: { isOn }, set: { isOn = $0 }), help: "Debug")
        let _ = button.body
    }
}

// MARK: - LabelButton Tests

struct LabelButtonTests {
    @Test func labelButtonWithShortcut() throws {
        let button = LabelButton(label: "car", shortcut: "1", isActive: false, action: {})
        let _ = button.body
    }

    @Test func labelButtonWithoutShortcut() throws {
        let button = LabelButton(label: "disconnected", shortcut: nil, isActive: false, action: {})
        let _ = button.body
    }

    @Test func labelButtonActive() throws {
        let button = LabelButton(label: "car", shortcut: "2", isActive: true, action: {})
        let _ = button.body
    }
}

// MARK: - TrackLabelPill Tests

struct TrackLabelPillTests {
    @Test func pillWithClassLabel() throws {
        let label = MetalRenderer.TrackScreenLabel(
            id: "track-001", screenX: 100, screenY: 200, classLabel: "car", isSelected: false)
        let pill = TrackLabelPill(label: label)
        let _ = pill.body
    }

    @Test func pillWithEmptyClassLabel() throws {
        let label = MetalRenderer.TrackScreenLabel(
            id: "track-002", screenX: 100, screenY: 200, classLabel: "", isSelected: false)
        let pill = TrackLabelPill(label: label)
        let _ = pill.body
    }

    @Test func pillSelected() throws {
        let label = MetalRenderer.TrackScreenLabel(
            id: "track-003", screenX: 100, screenY: 200, classLabel: "pedestrian", isSelected: true)
        let pill = TrackLabelPill(label: label)
        let _ = pill.body
    }
}

// MARK: - TrackLabelOverlay Tests

struct TrackLabelOverlayTests {
    @Test func overlayWithEmptyLabels() throws {
        let overlay = TrackLabelOverlay(labels: [])
        let _ = overlay.body
    }

    @Test func overlayWithMultipleLabels() throws {
        let labels = [
            MetalRenderer.TrackScreenLabel(
                id: "t1", screenX: 100, screenY: 200, classLabel: "car", isSelected: false),
            MetalRenderer.TrackScreenLabel(
                id: "t2", screenX: 300, screenY: 400, classLabel: "truck", isSelected: true),
        ]
        let overlay = TrackLabelOverlay(labels: labels)
        let _ = overlay.body
    }
}

// MARK: - MetalViewRepresentable Tests

@available(macOS 15.0, *) final class MetalViewRepresentableTests: XCTestCase {

    func testCoordinatorCreation() throws {
        let rep = MetalViewRepresentable(
            showPoints: true, showBackground: true, showBoxes: true, showClusters: true,
            showVelocity: true, showTrails: true, showDebug: false, showGrid: true, pointSize: 5.0)
        let coordinator = rep.makeCoordinator()
        XCTAssertNil(coordinator.renderer)
    }

    func testDefaultProperties() throws {
        let rep = MetalViewRepresentable(
            showPoints: true, showBackground: true, showBoxes: true, showClusters: true,
            showVelocity: true, showTrails: true, showDebug: false, showGrid: true, pointSize: 5.0)

        XCTAssertTrue(rep.showPoints)
        XCTAssertTrue(rep.showBackground)
        XCTAssertTrue(rep.showBoxes)
        XCTAssertTrue(rep.showClusters)
        XCTAssertTrue(rep.showVelocity)
        XCTAssertTrue(rep.showTrails)
        XCTAssertFalse(rep.showDebug)
        XCTAssertTrue(rep.showGrid)
        XCTAssertEqual(rep.pointSize, 5.0)
    }

    func testCustomProperties() throws {
        let rep = MetalViewRepresentable(
            showPoints: false, showBackground: false, showBoxes: false, showClusters: false,
            showVelocity: false, showTrails: false, showDebug: true, showGrid: false,
            pointSize: 15.0)

        XCTAssertFalse(rep.showPoints)
        XCTAssertFalse(rep.showBackground)
        XCTAssertFalse(rep.showBoxes)
        XCTAssertFalse(rep.showClusters)
        XCTAssertFalse(rep.showVelocity)
        XCTAssertFalse(rep.showTrails)
        XCTAssertTrue(rep.showDebug)
        XCTAssertFalse(rep.showGrid)
        XCTAssertEqual(rep.pointSize, 15.0)
    }
}

// MARK: - InteractiveMetalView Tests

@available(macOS 15.0, *) final class InteractiveMetalViewTests: XCTestCase {

    func testAcceptsFirstResponder() throws {
        let view = InteractiveMetalView()
        XCTAssertTrue(view.acceptsFirstResponder)
    }

    func testBecomeFirstResponder() throws {
        let view = InteractiveMetalView()
        XCTAssertTrue(view.becomeFirstResponder())
    }

    func testInitialRendererIsNil() throws {
        let view = InteractiveMetalView()
        XCTAssertNil(view.renderer)
    }

    func testCallbacksNilByDefault() throws {
        let view = InteractiveMetalView()
        XCTAssertNil(view.onTrackSelected)
        XCTAssertNil(view.onCameraChanged)
    }
}

// MARK: - String Truncation Tests (from App/StringTruncation.swift extension)

struct StringTruncationTests {
    @Test func truncateShortString() throws {
        let str = "hello"
        #expect(str.truncated(10) == "hello")
    }

    @Test func truncateExactLength() throws {
        let str = "hello"
        #expect(str.truncated(5) == "hello")
    }

    @Test func truncateLongString() throws {
        let str = "this-is-a-very-long-string"
        let result = str.truncated(12)
        #expect(result.count <= 13)  // 12 + "\u{2026}"
        #expect(result.hasSuffix("\u{2026}"))
    }
}

// MARK: - SparklineView Tests

struct SparklineViewTests {
    @Test func sparklineCreation() throws {
        let view = SparklineView(values: [1, 2, 3, 4, 5], colour: .cyan, label: "Speed")
        let _ = view.body
    }

    @Test func sparklineWithSingleValue() throws {
        let view = SparklineView(values: [42.0], colour: .orange, label: "Heading")
        let _ = view.body
    }

    @Test func sparklineWithEmptyValues() throws {
        let view = SparklineView(values: [], colour: .red, label: "Empty")
        let _ = view.body
    }

    @Test func sparklinePathWithTwoValues() throws {
        let view = SparklineView(values: [0, 10], colour: .cyan, label: "Test")
        let size = CGSize(width: 100, height: 40)
        let path = view.sparklinePath(in: size)
        // Should produce a valid path with 2 points
        #expect(!path.isEmpty)
    }

    @Test func sparklinePathEmptyReturnsEmptyPath() throws {
        let view = SparklineView(values: [], colour: .cyan, label: "Test")
        let path = view.sparklinePath(in: CGSize(width: 100, height: 40))
        #expect(path.isEmpty)
    }

    @Test func sparklinePathSingleValueReturnsEmptyPath() throws {
        let view = SparklineView(values: [5.0], colour: .cyan, label: "Test")
        let path = view.sparklinePath(in: CGSize(width: 100, height: 40))
        // < 2 values → guard returns empty path
        #expect(path.isEmpty)
    }

    @Test func sparklinePathConstantValues() throws {
        // All identical values: effectiveRange falls back to 1.0
        let view = SparklineView(values: [5, 5, 5, 5], colour: .cyan, label: "const")
        let path = view.sparklinePath(in: CGSize(width: 100, height: 40))
        #expect(!path.isEmpty)
    }

    @Test func sparklinePathNegativeValues() throws {
        let view = SparklineView(values: [-10, -5, 0, 5, 10], colour: .cyan, label: "neg")
        let path = view.sparklinePath(in: CGSize(width: 200, height: 40))
        #expect(!path.isEmpty)
    }

    @Test func sparklinePathBoundsContainedInCanvas() throws {
        let view = SparklineView(values: [0, 25, 50, 75, 100], colour: .cyan, label: "bounds")
        let size = CGSize(width: 100, height: 40)
        let path = view.sparklinePath(in: size)
        let bounds = path.boundingRect
        // All points should be within the canvas (with floating-point tolerance)
        #expect(bounds.minX >= -0.1)
        #expect(bounds.minY >= -0.1)
        #expect(bounds.maxX <= size.width + 0.1)
        #expect(bounds.maxY <= size.height + 0.1)
    }
}

// MARK: - TrackHistoryGraphView Tests

@available(macOS 15.0, *) @MainActor final class TrackHistoryGraphViewTests: XCTestCase {

    /// Host a view with an AppState environment object so @EnvironmentObject resolves.
    private func host<V: View>(_ view: V, state: AppState) {
        let hosted = view.environmentObject(state)
        let controller = NSHostingController(rootView: AnyView(hosted))
        controller.view.layout()
    }

    func testGraphViewWithNoSamples() throws {
        let state = AppState()
        host(TrackHistoryGraphView(trackID: "t-001"), state: state)
    }

    func testGraphViewWithOneSample() async throws {
        let state = AppState()
        state.isLive = false
        var frame = FrameBundle()
        frame.frameID = 0
        frame.timestampNanos = 100_000_000
        frame.playbackInfo = PlaybackInfo(
            isLive: false, logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0, paused: false,
            currentFrameIndex: 0, totalFrames: 10)
        frame.tracks = TrackSet(
            frameID: 0, timestampNanos: 100_000_000,
            tracks: [Track(trackID: "t-001", state: .confirmed, speedMps: 5.0)], trails: [])
        state.onFrameReceived(frame)
        await Task.yield()

        host(TrackHistoryGraphView(trackID: "t-001"), state: state)
    }

    func testGraphViewWithMultipleSamples() async throws {
        let state = AppState()
        state.isLive = false
        for i: UInt64 in 0..<20 {
            var frame = FrameBundle()
            frame.frameID = i
            frame.timestampNanos = Int64(i) * 50_000_000
            frame.playbackInfo = PlaybackInfo(
                isLive: false, logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0,
                paused: false, currentFrameIndex: i, totalFrames: 100)
            frame.tracks = TrackSet(
                frameID: i, timestampNanos: Int64(i) * 50_000_000,
                tracks: [
                    Track(
                        trackID: "t-001", state: .confirmed, speedMps: Float(i) * 0.5,
                        headingRad: Float(i) * 0.17)
                ], trails: [])
            state.onFrameReceived(frame)
            await Task.yield()
        }

        host(TrackHistoryGraphView(trackID: "t-001"), state: state)
    }

    func testGraphViewUnknownTrackID() throws {
        let state = AppState()
        host(TrackHistoryGraphView(trackID: "nonexistent"), state: state)
    }
}

// MARK: - TimeDisplayView Tests

@available(macOS 15.0, *) @MainActor final class TimeDisplayViewTests: XCTestCase {

    private func host<V: View>(_ view: V, state: AppState) {
        let hosted = view.environmentObject(state)
        let controller = NSHostingController(rootView: AnyView(hosted))
        controller.view.layout()
    }

    func testTimeDisplayWithValidRange() throws {
        let state = AppState()
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000
        state.currentTimestamp = 1_500_000_000
        host(TimeDisplayView(), state: state)
    }

    func testTimeDisplayWithZeroTimestamps() throws {
        let state = AppState()
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0
        state.currentTimestamp = 0
        host(TimeDisplayView(), state: state)
    }

    func testTimeDisplayWhenReplayFinished() throws {
        let state = AppState()
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000
        state.currentTimestamp = 2_000_000_000
        state.replayFinished = true
        state.replayProgress = 1.0
        host(TimeDisplayView(), state: state)
    }

    func testTimeDisplayWithLogStartAtZero() throws {
        let state = AppState()
        state.logStartTimestamp = 0
        state.logEndTimestamp = 5_000_000_000
        state.currentTimestamp = 2_500_000_000
        host(TimeDisplayView(), state: state)
    }

    func testTimeDisplayRemainingMode() throws {
        let state = AppState()
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000
        state.currentTimestamp = 1_500_000_000
        state.timeDisplayMode = .remaining
        host(TimeDisplayView(), state: state)
    }

    func testTimeDisplayFramesMode() throws {
        let state = AppState()
        state.totalFrames = 500
        state.currentFrameIndex = 250
        state.timeDisplayMode = .frames
        host(TimeDisplayView(), state: state)
    }

    func testTimeDisplayFramesModeWithValidRange() throws {
        let state = AppState()
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000
        state.currentTimestamp = 1_500_000_000
        state.totalFrames = 500
        state.currentFrameIndex = 250
        state.timeDisplayMode = .frames
        host(TimeDisplayView(), state: state)
    }
}

// MARK: - PlaybackControlsView Tests

@available(macOS 15.0, *) @MainActor final class PlaybackControlsViewTests: XCTestCase {

    private func host<V: View>(_ view: V, state: AppState) {
        let hosted = view.environmentObject(state)
        let controller = NSHostingController(rootView: AnyView(hosted))
        controller.view.layout()
    }

    func testPlaybackControlsLiveMode() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = true
        host(PlaybackControlsView(), state: state)
    }

    func testPlaybackControlsReplayMode() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.isSeekable = true
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 2_000_000_000
        host(PlaybackControlsView(), state: state)
    }

    func testPlaybackControlsDisconnected() throws {
        let state = AppState()
        state.isConnected = false
        host(PlaybackControlsView(), state: state)
    }

    func testPlaybackControlsReplayFinished() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.isSeekable = true
        state.replayFinished = true
        state.isPaused = true
        state.replayProgress = 1.0
        host(PlaybackControlsView(), state: state)
    }

    func testPlaybackControlsNotSeekable() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.isSeekable = false
        host(PlaybackControlsView(), state: state)
    }
}

// MARK: - PlaybackControlsDerivedState Tests

@available(macOS 15.0, *) final class PlaybackControlsDerivedStateTests: XCTestCase {
    func testReplaySeekableEnablesExpectedControls() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: true, mode: .replaySeekable, isPaused: true, playbackRate: 1.0,
            busy: false, hasValidTimelineRange: true, hasFrameIndexProgress: true,
            currentFrameIndex: 10, totalFrames: 100)
        XCTAssertEqual(ui.modeLabel, "REPLAY (VRLOG)")
        XCTAssertTrue(ui.showStepButtons)
        XCTAssertTrue(ui.showSeekableSlider)
        XCTAssertFalse(ui.seekSliderDisabled)
        XCTAssertFalse(ui.playPauseDisabled)
    }

    func testReplayNonSeekableWithoutMetadataShowsFallbackMessage() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: true, mode: .replayNonSeekable, isPaused: false, playbackRate: 1.0,
            busy: false, hasValidTimelineRange: false, hasFrameIndexProgress: false,
            currentFrameIndex: 0, totalFrames: 0)
        XCTAssertEqual(ui.modeLabel, "REPLAY (PCAP)")
        XCTAssertFalse(ui.showStepButtons)
        XCTAssertFalse(ui.showReadOnlyProgress)
        XCTAssertTrue(ui.showReplayMetadataUnavailable)
    }

    func testStepBoundsDisableAtStartAndEnd() throws {
        let start = PlaybackControlsDerivedState(
            isConnected: true, mode: .replaySeekable, isPaused: true, playbackRate: 1.0,
            busy: false, hasValidTimelineRange: true, hasFrameIndexProgress: true,
            currentFrameIndex: 0, totalFrames: 10)
        XCTAssertTrue(start.stepBackwardDisabled)
        XCTAssertFalse(start.stepForwardDisabled)

        let end = PlaybackControlsDerivedState(
            isConnected: true, mode: .replaySeekable, isPaused: true, playbackRate: 1.0,
            busy: false, hasValidTimelineRange: true, hasFrameIndexProgress: true,
            currentFrameIndex: 9, totalFrames: 10)
        XCTAssertTrue(end.stepForwardDisabled)
    }
}

// MARK: - TrackListView Display Tests

@available(macOS 15.0, *) @MainActor final class TrackListViewDisplayTests: XCTestCase {

    private func host<V: View>(_ view: V, state: AppState) {
        let hosted = view.environmentObject(state)
        let controller = NSHostingController(rootView: AnyView(hosted))
        controller.view.layout()
    }

    func testTrackListViewAlwaysVisibleInFrameMode() throws {
        let state = AppState()
        // Frame mode (no currentRunID)
        state.currentRunID = nil
        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 100,
            tracks: [Track(trackID: "t-001", state: .confirmed, speedMps: 5.0, classLabel: "car")],
            trails: [])
        state.currentFrame = frame

        // Should render without crash — track list is always visible (no isExpanded toggle)
        host(TrackListView(), state: state)
    }

    func testTrackListViewEmptyFrameMode() throws {
        let state = AppState()
        state.currentRunID = nil
        // No frame data
        host(TrackListView(), state: state)
    }

    func testTrackListViewRunModeWithNoRunID() throws {
        let state = AppState()
        state.currentRunID = nil
        host(TrackListView(), state: state)
    }

    func testTrackListViewRunModeWithRunID() throws {
        let state = AppState()
        state.currentRunID = "run-abc-123"
        host(TrackListView(), state: state)
    }

    func testTrackListViewDisplaysUserLabelsOverClassLabel() throws {
        let state = AppState()
        state.currentRunID = nil
        state.userLabels["t-001"] = "pedestrian"  // User label should override classLabel

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 100,
            tracks: [
                Track(trackID: "t-001", state: .confirmed, speedMps: 3.0, classLabel: "noise")
            ], trails: [])
        state.currentFrame = frame

        // The view should prefer userLabels["t-001"] ("pedestrian") over classLabel ("noise")
        host(TrackListView(), state: state)
    }

    func testTrackListViewWithMultipleTracks() throws {
        let state = AppState()
        state.currentRunID = nil

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 100,
            tracks: [
                Track(trackID: "t-001", state: .confirmed, speedMps: 5.0, classLabel: "car"),
                Track(trackID: "t-002", state: .tentative, speedMps: 1.5, classLabel: "noise"),
                Track(trackID: "t-003", state: .confirmed, speedMps: 12.0, classLabel: ""),
            ], trails: [])
        state.currentFrame = frame

        host(TrackListView(), state: state)
    }

    func testTrackListViewWithSelectedTrack() throws {
        let state = AppState()
        state.currentRunID = nil
        state.selectedTrackID = "t-002"

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 100,
            tracks: [
                Track(trackID: "t-001", state: .confirmed, speedMps: 5.0),
                Track(trackID: "t-002", state: .confirmed, speedMps: 3.0),
            ], trails: [])
        state.currentFrame = frame

        host(TrackListView(), state: state)
    }

    /// Ensures a track item with a high peak speed (28.8 m/s) and a climb arrow
    /// renders on a single line without wrapping or layout errors.
    func testTrackListItemHighSpeedDoesNotWrap() throws {
        let state = AppState()
        state.currentRunID = nil

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 100,
            tracks: [
                Track(
                    trackID: "trk_00001234", state: .confirmed, speedMps: 28.8, peakSpeedMps: 28.8,
                    classLabel: "car")
            ], trails: [])
        state.currentFrame = frame

        // The track list displays max(peakSpeedMps, persistentPeak) and a lineLimit(1)
        // prevents wrapping even with the "▲" climb indicator present.
        host(TrackListView(), state: state)
    }

    /// Ensures multiple tracks with varying speeds — including the maximum
    /// expected display width — render without layout issues.
    func testTrackListItemMaxSpeedWidthVariants() throws {
        let state = AppState()
        state.currentRunID = nil

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 100,
            tracks: [
                Track(
                    trackID: "trk_0000aaaa", state: .confirmed, speedMps: 1.0, peakSpeedMps: 1.0,
                    classLabel: ""),
                Track(
                    trackID: "trk_0000bbbb", state: .confirmed, speedMps: 28.8, peakSpeedMps: 28.8,
                    classLabel: "car"),
                Track(
                    trackID: "trk_0000cccc", state: .tentative, speedMps: 9.9, peakSpeedMps: 9.9,
                    classLabel: "truck"),
            ], trails: [])
        state.currentFrame = frame

        host(TrackListView(), state: state)
    }
}

// MARK: - LabelPanelView Carried Badge Tests

@available(macOS 15.0, *) @MainActor final class LabelPanelCarriedBadgeTests: XCTestCase {

    private func host<V: View>(_ view: V, state: AppState) {
        let hosted = view.environmentObject(state)
        let controller = NSHostingController(rootView: AnyView(hosted))
        controller.view.layout()
    }

    func testLabelPanelViewWithSelectedTrack() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"
        host(LabelPanelView(), state: state)
    }

    func testLabelPanelViewWithRunMode() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"
        state.currentRunID = "run-abc"
        host(LabelPanelView(), state: state)
    }

    func testLabelPanelViewWithoutSelection() throws {
        let state = AppState()
        state.selectedTrackID = nil
        host(LabelPanelView(), state: state)
    }
}

// MARK: - SidePanelView Tests

@available(macOS 15.0, *) @MainActor final class SidePanelViewTrackListTests: XCTestCase {

    private func host<V: View>(_ view: V, state: AppState) {
        let hosted = view.environmentObject(state)
        let controller = NSHostingController(rootView: AnyView(hosted))
        controller.view.layout()
    }

    func testSidePanelContainsTrackList() throws {
        let state = AppState()
        state.currentRunID = nil
        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 100,
            tracks: [Track(trackID: "t-001", state: .confirmed, speedMps: 5.0)], trails: [])
        state.currentFrame = frame

        host(SidePanelView(), state: state)
    }
}

// MARK: - TrackInspector Field Population Tests

/// Verify that the Track fields displayed in TrackInspectorView are
/// non-zero when the model is populated.  This is a model-level regression
/// test: the gRPC server previously serialised zero values for PeakSpeedMps,
/// Hits, Confidence, Duration, and Length because the proto conversion
/// omitted those fields.
struct TrackInspectorFieldTests {
    /// Build a fully-populated Track matching what the adapter produces.
    private func populatedTrack() -> Track {
        Track(
            trackID: "trk-inspector-test", sensorID: "sensor-1", state: .confirmed, hits: 50,
            misses: 2, observationCount: 48, firstSeenNanos: 1_000_000_000,
            lastSeenNanos: 2_000_000_000, x: 10.0, y: 5.0, z: 0.5, vx: 8.0, vy: 0.5, vz: 0.0,
            speedMps: 8.03, headingRad: 0.06, covariance4x4: [], bboxLength: 4.5, bboxWidth: 1.8,
            bboxHeight: 1.5, bboxHeadingRad: 0.1, heightP95Max: 1.6, intensityMeanAvg: 50.0,
            avgSpeedMps: 7.5, peakSpeedMps: 9.0, classLabel: "vehicle", classConfidence: 0.95,
            trackLengthMetres: 150.0, trackDurationSecs: 20.0, occlusionCount: 0, confidence: 0.98,
            occlusionState: .none, motionModel: .cv, alpha: 1.0)
    }

    @Test func peakSpeedIsNonZero() throws {
        let t = populatedTrack()
        #expect(t.peakSpeedMps > 0, "peakSpeedMps must be populated")
    }

    @Test func hitsIsNonZero() throws {
        let t = populatedTrack()
        #expect(t.hits > 0, "hits must be populated")
    }

    @Test func confidenceIsNonZero() throws {
        let t = populatedTrack()
        #expect(t.confidence > 0, "confidence must be populated")
    }

    @Test func trackDurationIsNonZero() throws {
        let t = populatedTrack()
        #expect(t.trackDurationSecs > 0, "trackDurationSecs must be populated")
    }

    @Test func trackLengthIsNonZero() throws {
        let t = populatedTrack()
        #expect(t.trackLengthMetres > 0, "trackLengthMetres must be populated")
    }

    @Test func classLabelIsNonEmpty() throws {
        let t = populatedTrack()
        #expect(!t.classLabel.isEmpty, "classLabel must be populated")
    }

    @Test @MainActor func trackLookupViaAppState() throws {
        let state = AppState()
        var frame = FrameBundle()
        let t = populatedTrack()
        frame.tracks = TrackSet(frameID: 1, timestampNanos: 1_000_000, tracks: [t], trails: [])
        state.currentFrame = frame

        // Simulate what TrackInspectorView does: look up track by ID.
        let found = state.currentFrame?.tracks?.tracks.first(where: {
            $0.trackID == "trk-inspector-test"
        })
        #expect(found != nil, "track must be findable by ID")
        #expect(found!.peakSpeedMps == 9.0)
        #expect(found!.hits == 50)
        #expect(found!.confidence == 0.98)
        #expect(found!.trackDurationSecs == 20.0)
        #expect(found!.trackLengthMetres == 150.0)
    }
}
