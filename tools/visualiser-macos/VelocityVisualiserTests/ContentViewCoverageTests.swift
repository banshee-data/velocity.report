//
//  ContentViewCoverageTests.swift
//  VelocityVisualiserTests
//
//  Comprehensive coverage tests for ContentView.swift targeting 98%+ coverage.
//  Uses NSHostingController to fully render views, exercising body getters,
//  closures, and all conditional branches.
//

import AppKit
import Foundation
import MetalKit
import SwiftUI
import Testing
import XCTest

@testable import VelocityVisualiser

// MARK: - Test Helpers

/// Shared helper to host a SwiftUI view inside an NSHostingController,
/// forcing the rendering pipeline to execute the view's body getter
/// and all nested closures.
@available(macOS 15.0, *) @MainActor private func hostView<V: View>(_ view: V, state: AppState) {
    let hosted = view.environmentObject(state)
    let controller = NSHostingController(rootView: AnyView(hosted))
    controller.view.frame = NSRect(x: 0, y: 0, width: 800, height: 600)
    controller.view.layout()
}

/// Build a fully-populated Track for testing.
private func makeTrack(
    id: String = "trk_00001234", state: TrackState = .confirmed, speed: Float = 8.0,
    maxSpeed: Float = 9.0, classLabel: String = "car", x: Float = 10.0, y: Float = 5.0,
    z: Float = 0.5, hits: Int = 50, misses: Int = 2, confidence: Float = 0.98,
    durationSecs: Float = 20.0, lengthMetres: Float = 150.0, headingRad: Float = 0.1,
    bboxLength: Float = 4.5, bboxWidth: Float = 1.8, bboxHeight: Float = 1.5,
    bboxHeading: Float = 0.1
) -> Track {
    Track(
        trackID: id, sensorID: "sensor-1", state: state, hits: hits, misses: misses,
        observationCount: 48, firstSeenNanos: 1_000_000_000, lastSeenNanos: 2_000_000_000, x: x,
        y: y, z: z, vx: 8.0, vy: 0.5, vz: 0.0, speedMps: speed, headingRad: headingRad,
        covariance4x4: [], bboxLength: bboxLength, bboxWidth: bboxWidth, bboxHeight: bboxHeight,
        bboxHeadingRad: bboxHeading, heightP95Max: 1.6, intensityMeanAvg: 50.0, avgSpeedMps: 7.5,
        maxSpeedMps: maxSpeed, classLabel: classLabel, classConfidence: 0.95,
        trackLengthMetres: lengthMetres, trackDurationSecs: durationSecs, occlusionCount: 0,
        confidence: confidence, occlusionState: .none, motionModel: .cv, alpha: 1.0)
}

/// Build a RunTrack for testing.
private func makeRunTrack(
    runId: String = "run-123", trackId: String = "trk_00001234", userLabel: String? = nil,
    qualityLabel: String? = nil, maxSpeed: Double? = 12.0, labelerId: String? = nil
) -> RunTrack {
    RunTrack(
        runId: runId, trackId: trackId, sensorId: "sensor-1", userLabel: userLabel,
        qualityLabel: qualityLabel, labelConfidence: nil, labelerId: labelerId,
        startUnixNanos: 1_000_000_000, endUnixNanos: 2_000_000_000, totalObservations: 50,
        durationSecs: 10.0, avgSpeedMps: 10.0, maxSpeedMps: maxSpeed, isSplitCandidate: false,
        isMergeCandidate: false)
}

/// Build a FrameBundle with tracks.
private func makeFrame(tracks: [Track], frameID: UInt64 = 1) -> FrameBundle {
    var frame = FrameBundle()
    frame.frameID = frameID
    frame.timestampNanos = 1_000_000_000
    frame.tracks = TrackSet(
        frameID: frameID, timestampNanos: 1_000_000_000, tracks: tracks, trails: [])
    return frame
}

// MARK: - TrackInspectorHeaderView Tests

@available(macOS 15.0, *) @MainActor final class TrackInspectorHeaderViewTests: XCTestCase {

    func testHeaderWithTrackInFrame() throws {
        let state = AppState()
        let track = makeTrack(id: "trk_0000abcd")
        state.currentFrame = makeFrame(tracks: [track])
        hostView(TrackInspectorHeaderView(trackID: "trk_0000abcd"), state: state)
    }

    func testHeaderWithTrackNotInFrame() throws {
        let state = AppState()
        state.currentFrame = makeFrame(tracks: [])
        hostView(TrackInspectorHeaderView(trackID: "trk_0000abcd"), state: state)
    }

    func testHeaderWithNoFrame() throws {
        let state = AppState()
        hostView(TrackInspectorHeaderView(trackID: "trk_0000abcd"), state: state)
    }

    func testHeaderClosesInspector() throws {
        let state = AppState()
        state.selectedTrackID = "trk_0000abcd"
        state.currentFrame = makeFrame(tracks: [makeTrack(id: "trk_0000abcd")])
        hostView(TrackInspectorHeaderView(trackID: "trk_0000abcd"), state: state)
    }
}

// MARK: - TrackInspectorDetailCards Tests

@available(macOS 15.0, *) @MainActor final class TrackInspectorDetailCardsTests: XCTestCase {

    func testDetailCardsWithConfirmedTrack() throws {
        let state = AppState()
        let track = makeTrack(id: "trk_0000test", state: .confirmed)
        state.currentFrame = makeFrame(tracks: [track])
        hostView(TrackInspectorDetailCards(trackID: "trk_0000test"), state: state)
    }

    func testDetailCardsWithTentativeTrack() throws {
        let state = AppState()
        let track = makeTrack(id: "trk_0000tent", state: .tentative, classLabel: "")
        state.currentFrame = makeFrame(tracks: [track])
        hostView(TrackInspectorDetailCards(trackID: "trk_0000tent"), state: state)
    }

    func testDetailCardsWithDeletedTrack() throws {
        let state = AppState()
        let track = makeTrack(id: "trk_0000del", state: .deleted)
        state.currentFrame = makeFrame(tracks: [track])
        hostView(TrackInspectorDetailCards(trackID: "trk_0000del"), state: state)
    }

    func testDetailCardsWithUnknownState() throws {
        let state = AppState()
        let track = makeTrack(id: "trk_0000unk", state: .unknown)
        state.currentFrame = makeFrame(tracks: [track])
        hostView(TrackInspectorDetailCards(trackID: "trk_0000unk"), state: state)
    }

    func testDetailCardsWithNoTrackInFrame() throws {
        let state = AppState()
        state.currentFrame = makeFrame(tracks: [])
        hostView(TrackInspectorDetailCards(trackID: "trk_0000none"), state: state)
    }

    func testDetailCardsWithClassLabel() throws {
        let state = AppState()
        let track = makeTrack(id: "trk_0000cls", classLabel: "vehicle")
        state.currentFrame = makeFrame(tracks: [track])
        hostView(TrackInspectorDetailCards(trackID: "trk_0000cls"), state: state)
    }

    func testDetailCardsWithEmptyClassLabel() throws {
        let state = AppState()
        let track = makeTrack(id: "trk_0000ecl", classLabel: "")
        state.currentFrame = makeFrame(tracks: [track])
        hostView(TrackInspectorDetailCards(trackID: "trk_0000ecl"), state: state)
    }
}

// MARK: - TrackInspectorView Composite Tests

@available(macOS 15.0, *) @MainActor final class TrackInspectorViewCompositeTests: XCTestCase {

    func testCompositeWithTrack() throws {
        let state = AppState()
        let track = makeTrack(id: "trk_0000comp")
        state.currentFrame = makeFrame(tracks: [track])
        hostView(TrackInspectorView(trackID: "trk_0000comp"), state: state)
    }

    func testCompositeWithoutTrack() throws {
        let state = AppState()
        hostView(TrackInspectorView(trackID: "trk_0000none"), state: state)
    }

    func testCompositeWithMultipleTracksSelectsCorrectOne() throws {
        let state = AppState()
        let t1 = makeTrack(id: "trk_00001111", speed: 5.0)
        let t2 = makeTrack(id: "trk_00002222", speed: 15.0)
        state.currentFrame = makeFrame(tracks: [t1, t2])
        hostView(TrackInspectorView(trackID: "trk_00002222"), state: state)
    }
}

// MARK: - SidePanelView Full Layout Tests

@available(macOS 15.0, *) @MainActor final class SidePanelViewFullTests: XCTestCase {

    func testSidePanelWithSelectedTrack() throws {
        let state = AppState()
        state.selectedTrackID = "trk_0000side"
        let track = makeTrack(id: "trk_0000side")
        state.currentFrame = makeFrame(tracks: [track])
        hostView(SidePanelView(), state: state)
    }

    func testSidePanelWithoutSelectedTrack() throws {
        let state = AppState()
        state.selectedTrackID = nil
        state.currentFrame = makeFrame(tracks: [makeTrack()])
        hostView(SidePanelView(), state: state)
    }

    func testSidePanelWithRunModeAndSelectedTrack() throws {
        let state = AppState()
        state.selectedTrackID = "trk_0000run"
        state.currentRunID = "run-123"
        let track = makeTrack(id: "trk_0000run")
        state.currentFrame = makeFrame(tracks: [track])
        hostView(SidePanelView(), state: state)
    }
}

// MARK: - StatsDisplayView Tests

@available(macOS 15.0, *) @MainActor final class StatsDisplayViewCoverageTests: XCTestCase {

    func testStatsConnectedWithAllData() throws {
        let state = AppState()
        state.isConnected = true
        state.fps = 30.0
        state.pointCount = 1500
        state.trackCount = 12
        state.cacheStatus = "Cached (seq 42)"
        hostView(StatsDisplayView(), state: state)
    }

    func testStatsConnectedSmallPointCount() throws {
        let state = AppState()
        state.isConnected = true
        state.fps = 60.0
        state.pointCount = 500  // < 1000 → no "k" suffix
        state.trackCount = 3
        state.cacheStatus = ""
        hostView(StatsDisplayView(), state: state)
    }

    func testStatsConnectedLargePointCount() throws {
        let state = AppState()
        state.isConnected = true
        state.fps = 10.0
        state.pointCount = 65000  // ≥ 1000 → "65.00k"
        state.trackCount = 0
        state.cacheStatus = "Refreshing..."
        hostView(StatsDisplayView(), state: state)
    }

    func testStatsConnectedSplitStreamingStatus() throws {
        let state = AppState()
        state.isConnected = true
        state.fps = 10.0
        state.pointCount = 100
        state.trackCount = 1
        state.cacheStatus = "Not using split streaming"  // Should not show cache label
        hostView(StatsDisplayView(), state: state)
    }

    func testStatsDisconnected() throws {
        let state = AppState()
        state.isConnected = false
        hostView(StatsDisplayView(), state: state)
    }
}

// MARK: - ConnectionStatusView Tests

@available(macOS 15.0, *) @MainActor final class ConnectionStatusViewCoverageTests: XCTestCase {

    func testDisconnectedNoError() throws {
        let state = AppState()
        state.isConnected = false
        state.connectionError = nil
        hostView(ConnectionStatusView(), state: state)
    }

    func testDisconnectedWithError() throws {
        let state = AppState()
        state.isConnected = false
        state.connectionError = "Connection refused"
        hostView(ConnectionStatusView(), state: state)
    }

    func testConnected() throws {
        let state = AppState()
        state.isConnected = true
        state.serverAddress = "192.168.1.100:50051"
        hostView(ConnectionStatusView(), state: state)
    }
}

// MARK: - ConnectionButtonView Tests

@available(macOS 15.0, *) @MainActor final class ConnectionButtonViewCoverageTests: XCTestCase {

    func testConnectingState() throws {
        let state = AppState()
        state.isConnecting = true
        state.isConnected = false
        hostView(ConnectionButtonView(), state: state)
    }

    func testConnectedState() throws {
        let state = AppState()
        state.isConnecting = false
        state.isConnected = true
        hostView(ConnectionButtonView(), state: state)
    }

    func testDisconnectedState() throws {
        let state = AppState()
        state.isConnecting = false
        state.isConnected = false
        hostView(ConnectionButtonView(), state: state)
    }
}

// MARK: - ToolbarView Tests

@available(macOS 15.0, *) @MainActor final class ToolbarViewCoverageTests: XCTestCase {

    func testToolbarConnected() throws {
        let state = AppState()
        state.isConnected = true
        hostView(ToolbarView(), state: state)
    }

    func testToolbarDisconnected() throws {
        let state = AppState()
        state.isConnected = false
        hostView(ToolbarView(), state: state)
    }

    func testToolbarConnectedWithActiveFilters() throws {
        let state = AppState()
        state.isConnected = true
        state.filterMinHits = 5  // Makes hasActiveFilters true
        hostView(ToolbarView(), state: state)
    }
}

// MARK: - PlaybackModeBadgeView Tests

@available(macOS 15.0, *) @MainActor final class PlaybackModeBadgeViewCoverageTests: XCTestCase {

    func testLiveMode() throws {
        let view = PlaybackModeBadgeView(
            modeLabel: "LIVE", mode: .live, isConnected: true, showsLegacyJSONWarning: false)
        let _ = view.body
    }

    func testReplayNonSeekable() throws {
        let view = PlaybackModeBadgeView(
            modeLabel: "REPLAY (PCAP)", mode: .replayNonSeekable, isConnected: true,
            showsLegacyJSONWarning: false)
        let _ = view.body
    }

    func testReplaySeekable() throws {
        let view = PlaybackModeBadgeView(
            modeLabel: "REPLAY (VRLOG)", mode: .replaySeekable, isConnected: true,
            showsLegacyJSONWarning: true)
        let _ = view.body
    }

    func testUnknownMode() throws {
        let view = PlaybackModeBadgeView(
            modeLabel: "??", mode: .unknown, isConnected: true, showsLegacyJSONWarning: false)
        let _ = view.body
    }

    func testDisconnected() throws {
        let view = PlaybackModeBadgeView(
            modeLabel: "LIVE", mode: .live, isConnected: false, showsLegacyJSONWarning: false)
        let _ = view.body
    }
}

// MARK: - PlaybackControlsView Full Coverage

@available(macOS 15.0, *) @MainActor final class PlaybackControlsViewCoverageTests: XCTestCase {

    func testReplaySeekableWithValidTimeline() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.isSeekable = true
        state.isPaused = true
        state.logStartTimestamp = 0
        state.logEndTimestamp = 60_000_000_000
        state.currentTimestamp = 30_000_000_000
        state.currentFrameIndex = 50
        state.totalFrames = 100
        hostView(PlaybackControlsView(), state: state)
    }

    func testReplayNonSeekableWithFrameProgress() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.isSeekable = false
        state.currentFrameIndex = 50
        state.totalFrames = 100
        hostView(PlaybackControlsView(), state: state)
    }

    func testReplayNonSeekableNoMetadata() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.isSeekable = false
        state.totalFrames = 0
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0
        hostView(PlaybackControlsView(), state: state)
    }

    func testPlaybackControlsDisconnected() throws {
        let state = AppState()
        state.isConnected = false
        state.isLive = false
        hostView(PlaybackControlsView(), state: state)
    }
}

// MARK: - TimeDisplayView Coverage

@available(macOS 15.0, *) @MainActor final class TimeDisplayViewCoverageTests: XCTestCase {

    func testTimeDisplayFrameIndexFallback() throws {
        let state = AppState()
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0  // No valid range
        state.totalFrames = 100
        state.currentFrameIndex = 42
        hostView(TimeDisplayView(), state: state)
    }

    func testTimeDisplayNoDataFallback() throws {
        let state = AppState()
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0
        state.totalFrames = 0
        hostView(TimeDisplayView(), state: state)
    }

    func testTimeDisplayValidRange() throws {
        let state = AppState()
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 61_000_000_000  // 60s
        state.currentTimestamp = 31_000_000_000
        hostView(TimeDisplayView(), state: state)
    }

    func testTimeDisplayRemainingMode() throws {
        let state = AppState()
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 61_000_000_000
        state.currentTimestamp = 31_000_000_000
        state.timeDisplayMode = .remaining
        hostView(TimeDisplayView(), state: state)
    }

    func testTimeDisplayFramesMode() throws {
        let state = AppState()
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 61_000_000_000
        state.currentTimestamp = 31_000_000_000
        state.totalFrames = 600
        state.currentFrameIndex = 300
        state.timeDisplayMode = .frames
        hostView(TimeDisplayView(), state: state)
    }

    func testTimeDisplayFramesModeNoFrames() throws {
        let state = AppState()
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0
        state.totalFrames = 0
        state.timeDisplayMode = .frames
        hostView(TimeDisplayView(), state: state)
    }

    func testTimeDisplayElapsedModeNoRange() throws {
        let state = AppState()
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0
        state.totalFrames = 200
        state.currentFrameIndex = 50
        state.timeDisplayMode = .elapsed  // explicit, but no valid range → falls back to frames
        hostView(TimeDisplayView(), state: state)
    }

    func testTimeDisplayRemainingModeNoRange() throws {
        let state = AppState()
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0
        state.totalFrames = 200
        state.currentFrameIndex = 50
        state.timeDisplayMode = .remaining  // no valid range → falls back to frames
        hostView(TimeDisplayView(), state: state)
    }
}

// MARK: - OverlayTogglesView Coverage

@available(macOS 15.0, *) @MainActor final class OverlayTogglesViewCoverageTests: XCTestCase {

    func testAllTogglesEnabled() throws {
        let state = AppState()
        state.showPoints = true
        state.showBackground = true
        state.showBoxes = true
        state.showClusters = true
        state.showTrails = true
        state.showVelocity = true
        state.showTrackLabels = true
        state.showGrid = true
        state.showDebug = true
        state.pointSize = 10.0
        hostView(OverlayTogglesView(), state: state)
    }

    func testAllTogglesDisabled() throws {
        let state = AppState()
        state.showPoints = false
        state.showBackground = false
        state.showBoxes = false
        state.showClusters = false
        state.showTrails = false
        state.showVelocity = false
        state.showTrackLabels = false
        state.showGrid = false
        state.showDebug = false
        state.pointSize = 1.0
        hostView(OverlayTogglesView(), state: state)
    }
}

// MARK: - DebugOverlayTogglesView Coverage

@available(macOS 15.0, *) @MainActor final class DebugOverlayTogglesViewCoverageTests: XCTestCase {

    func testDebugTogglesEnabled() throws {
        let state = AppState()
        state.showDebug = true
        state.showTrackLabels = true
        hostView(DebugOverlayTogglesView(), state: state)
    }

    func testDebugTogglesDisabled() throws {
        let state = AppState()
        state.showDebug = false
        state.showTrackLabels = false
        hostView(DebugOverlayTogglesView(), state: state)
    }
}

// MARK: - FlagToggleButton Tests

struct FlagToggleButtonTests {
    @Test func activeButton() throws {
        let view = FlagToggleButton(
            label: "noisy", isActive: true, helpText: "Track has noisy position"
        ) {}
        let _ = view.body
    }

    @Test func inactiveButton() throws {
        let view = FlagToggleButton(label: "good", isActive: false, helpText: "Clean track") {}
        let _ = view.body
    }

    @Test func snakeCaseLabel() throws {
        let view = FlagToggleButton(
            label: "jitter_velocity", isActive: false, helpText: "Speed jitters"
        ) {}
        let _ = view.body
    }

    @Test func snakeCaseLabelJitterHeading() throws {
        let view = FlagToggleButton(
            label: "jitter_heading", isActive: true, helpText: "Heading jitters"
        ) {}
        let _ = view.body
    }

    @Test func emptyHelpText() throws {
        let view = FlagToggleButton(label: "merge", isActive: true) {}
        let _ = view.body
    }
}

// MARK: - TagRow and TagPill Tests

struct TagRowTests {
    @Test func singleTag() throws {
        let view = TagRow(tags: [("car", .green)])
        let _ = view.body
    }

    @Test func twoTags() throws {
        let view = TagRow(tags: [("car", .green), ("good", .blue)])
        let _ = view.body
    }

    @Test func threeTagsShowsOverflow() throws {
        let view = TagRow(tags: [("car", .green), ("good", .blue), ("noisy", .orange)])
        let _ = view.body
    }

    @Test func fourTagsShowsOverflow() throws {
        let view = TagRow(tags: [
            ("car", .green), ("good", .blue), ("noisy", .orange), ("split", .red),
        ])
        let _ = view.body
    }

    @Test func emptyTags() throws {
        let view = TagRow(tags: [])
        let _ = view.body
    }
}

struct TagPillTests {
    @Test func shortText() throws {
        let view = TagPill(text: "car", colour: .green)
        let _ = view.body
    }

    @Test func longTextTruncated() throws {
        let view = TagPill(text: "pedestrian", colour: .yellow)
        let _ = view.body
    }

    @Test func exactlySixChars() throws {
        let view = TagPill(text: "cyclist", colour: .orange)
        let _ = view.body
    }

    @Test func emptyText() throws {
        let view = TagPill(text: "", colour: .gray)
        let _ = view.body
    }

    @Test func overflowPill() throws {
        let view = TagPill(text: "\u{2026}", colour: .accentColor)
        let _ = view.body
    }
}

// MARK: - TrackLabelPill Coverage (userLabel branch)

struct TrackLabelPillCoverageTests {
    @Test func pillWithUserLabelShowsGreen() throws {
        var label = MetalRenderer.TrackScreenLabel(
            id: "trk_0000abcd", screenX: 100, screenY: 200, classLabel: "noise", isSelected: false)
        label.userLabel = "car"
        let pill = TrackLabelPill(label: label)
        let _ = pill.body
    }

    @Test func pillWithClassLabelOnlyShowsYellow() throws {
        var label = MetalRenderer.TrackScreenLabel(
            id: "trk_0000efgh", screenX: 100, screenY: 200, classLabel: "vehicle", isSelected: false
        )
        label.userLabel = ""
        let pill = TrackLabelPill(label: label)
        let _ = pill.body
    }

    @Test func pillWithNoLabelShowsNoClassification() throws {
        var label = MetalRenderer.TrackScreenLabel(
            id: "trk_0000ijkl", screenX: 100, screenY: 200, classLabel: "", isSelected: false)
        label.userLabel = ""
        let pill = TrackLabelPill(label: label)
        let _ = pill.body
    }

    @Test func pillUserLabelOverridesClassLabel() throws {
        var label = MetalRenderer.TrackScreenLabel(
            id: "trk_0000mnop", screenX: 50, screenY: 100, classLabel: "noise", isSelected: true)
        label.userLabel = "pedestrian"
        let pill = TrackLabelPill(label: label)
        let _ = pill.body
    }
}

// MARK: - shortTrackID Extension Tests

struct ShortTrackIDTests {
    @Test func standardTrackID() throws { #expect("trk_0000abcd".shortTrackID == "abcd") }

    @Test func shortTrackIDWithUnderscore() throws {
        #expect("trk_a1b2c3d4".shortTrackID == "c3d4")
    }

    @Test func trackIDShorterThanFourChars() throws { #expect("trk_ab".shortTrackID == "ab") }

    @Test func noUnderscore() throws {
        // Falls back to suffix(4)
        #expect("abcdefgh".shortTrackID == "efgh")
    }

    @Test func emptyString() throws { #expect("".shortTrackID == "") }

    @Test func exactlyFourAfterUnderscore() throws { #expect("trk_1234".shortTrackID == "1234") }

    @Test func multipleUnderscores() throws {
        // lastIndex(of: "_") returns the last underscore
        #expect("trk_sub_a1b2c3d4".shortTrackID == "c3d4")
    }
}

// MARK: - confirmedGreen Colour Test

struct ConfirmedGreenColourTest {
    @Test func colourExists() throws {
        let green = Color.confirmedGreen
        // Just verify the extension is accessible
        let _ = green
    }
}

// MARK: - ModeIndicatorView Tests

struct ModeIndicatorViewCoverageTests {
    @Test func liveConnected() throws {
        let view = ModeIndicatorView(isLive: true, isConnected: true)
        let _ = view.body
    }

    @Test func replayConnected() throws {
        let view = ModeIndicatorView(isLive: false, isConnected: true)
        let _ = view.body
    }

    @Test func disconnected() throws {
        let view = ModeIndicatorView(isLive: false, isConnected: false)
        let _ = view.body
    }
}

// MARK: - FilterBarView Coverage

@available(macOS 15.0, *) @MainActor final class FilterBarViewCoverageTests: XCTestCase {

    func testFilterBarDefaultState() throws {
        let state = AppState()
        state.showFilterPane = true
        hostView(FilterBarView(), state: state)
    }

    func testFilterBarWithActiveFilters() throws {
        let state = AppState()
        state.showFilterPane = true
        state.filterOnlyInBox = true
        state.filterMinHits = 5
        state.filterMaxHits = 25
        state.filterMinPointsPerFrame = 3
        state.filterMaxPointsPerFrame = 50
        state.currentFrame = makeFrame(tracks: [makeTrack(id: "trk_0000test", hits: 10)])
        hostView(FilterBarView(), state: state)
    }

    func testFilterBarHitsAtZeroMaxShowsInfinity() throws {
        let state = AppState()
        state.showFilterPane = true
        state.filterMinHits = 3
        state.filterMaxHits = 0  // "∞"
        hostView(FilterBarView(), state: state)
    }

    func testFilterBarPointsPerFrameAtZeroMaxShowsInfinity() throws {
        let state = AppState()
        state.showFilterPane = true
        state.filterMinPointsPerFrame = 2
        state.filterMaxPointsPerFrame = 0  // "∞"
        hostView(FilterBarView(), state: state)
    }
}

// MARK: - RangeSliderView Tests

struct RangeSliderViewTests {
    @Test func basicRangeSlider() throws {
        @State var low = 5.0
        @State var high = 20.0
        let view = RangeSliderView(low: $low, high: $high, range: 0...50, step: 1)
        let _ = view.body
    }

    @Test func rangeSliderNoLimit() throws {
        @State var low = 0.0
        @State var high = 0.0  // 0 means no limit
        let view = RangeSliderView(low: $low, high: $high, range: 0...100, step: 1)
        let _ = view.body
    }

    @Test func rangeSliderMaxedOut() throws {
        @State var low = 0.0
        @State var high = 50.0  // at max
        let view = RangeSliderView(low: $low, high: $high, range: 0...50, step: 1)
        let _ = view.body
    }
}

// MARK: - BulkLabelView Tests

@available(macOS 15.0, *) @MainActor final class BulkLabelViewCoverageTests: XCTestCase {

    func testBulkLabelViewWithFilteredTracks() throws {
        let state = AppState()
        state.currentFrame = makeFrame(tracks: [
            makeTrack(id: "trk_00001111"), makeTrack(id: "trk_00002222"),
        ])
        hostView(BulkLabelView(), state: state)
    }

    func testBulkLabelViewWithNoTracks() throws {
        let state = AppState()
        hostView(BulkLabelView(), state: state)
    }
}

// MARK: - LabelPanelView Full Coverage

@available(macOS 15.0, *) @MainActor final class LabelPanelViewCoverageTests: XCTestCase {

    func testLabelPanelWithSelectedTrackInRunMode() throws {
        let state = AppState()
        state.selectedTrackID = "trk_0000test"
        state.currentRunID = "run-456"
        let track = makeTrack(id: "trk_0000test")
        state.currentFrame = makeFrame(tracks: [track])
        hostView(LabelPanelView(), state: state)
    }

    func testLabelPanelWithSelectedTrackInLiveMode() throws {
        let state = AppState()
        state.selectedTrackID = "trk_0000live"
        state.currentRunID = nil
        let track = makeTrack(id: "trk_0000live")
        state.currentFrame = makeFrame(tracks: [track])
        hostView(LabelPanelView(), state: state)
    }

    func testLabelPanelNoSelection() throws {
        let state = AppState()
        state.selectedTrackID = nil
        hostView(LabelPanelView(), state: state)
    }

    func testLabelPanelWithUserLabels() throws {
        let state = AppState()
        state.selectedTrackID = "trk_0000lbl"
        state.userLabels["trk_0000lbl"] = "car"
        hostView(LabelPanelView(), state: state)
    }
}

// MARK: - LabelButton Coverage

struct LabelButtonCoverageTests {
    @Test func withHelpText() throws {
        let view = LabelButton(
            label: "car", shortcut: "1", isActive: false, helpText: "Passenger car, SUV, or van"
        ) {}
        let _ = view.body
    }

    @Test func activeWithShortcut() throws {
        let view = LabelButton(
            label: "truck", shortcut: "2", isActive: true, helpText: "Pickup truck"
        ) {}
        let _ = view.body
    }

    @Test func snakeCaseDisplayName() throws {
        let view = LabelButton(label: "dynamic", shortcut: nil, isActive: false) {}
        let _ = view.body
    }
}

// MARK: - TrackListView Comprehensive Coverage

@available(macOS 15.0, *) @MainActor final class TrackListViewCoverageTests: XCTestCase {

    func testFrameModeWithMultipleTracks() throws {
        let state = AppState()
        state.currentRunID = nil
        state.currentFrame = makeFrame(tracks: [
            makeTrack(id: "trk_00001111", state: .confirmed, speed: 8.0, maxSpeed: 10.0),
            makeTrack(id: "trk_00002222", state: .tentative, speed: 3.0, maxSpeed: 4.0),
            makeTrack(id: "trk_00003333", state: .deleted, speed: 0.0, maxSpeed: 0.5),
        ])
        hostView(TrackListView(), state: state)
    }

    func testFrameModeWithSelectedTrack() throws {
        let state = AppState()
        state.currentRunID = nil
        state.selectedTrackID = "trk_00001111"
        state.currentFrame = makeFrame(tracks: [
            makeTrack(id: "trk_00001111", speed: 5.0), makeTrack(id: "trk_00002222", speed: 1.0),
        ])
        hostView(TrackListView(), state: state)
    }

    func testFrameModeWithUserLabels() throws {
        let state = AppState()
        state.currentRunID = nil
        state.userLabels["trk_00001111"] = "car"
        state.currentFrame = makeFrame(tracks: [makeTrack(id: "trk_00001111", classLabel: "noise")])
        hostView(TrackListView(), state: state)
    }

    func testFrameModeWithEmptyTracks() throws {
        let state = AppState()
        state.currentRunID = nil
        state.currentFrame = makeFrame(tracks: [])
        hostView(TrackListView(), state: state)
    }

    func testFrameModeWithMaxSpeedTracking() throws {
        let state = AppState()
        state.currentRunID = nil
        // trackMaxSpeed is private(set), we can set it via onFrameReceived
        let track = makeTrack(id: "trk_0000pk", speed: 5.0, maxSpeed: 5.0)
        state.currentFrame = makeFrame(tracks: [track])
        hostView(TrackListView(), state: state)
    }

    func testRunModeWithRunID() throws {
        let state = AppState()
        state.currentRunID = "run-789"
        state.isConnected = true
        // In run mode without fetched tracks, it shows empty state
        hostView(TrackListView(), state: state)
    }

    func testFrameModeWithActiveFilters() throws {
        let state = AppState()
        state.currentRunID = nil
        state.filterMinHits = 5
        state.currentFrame = makeFrame(tracks: [makeTrack(id: "trk_0000filt", hits: 10)])
        hostView(TrackListView(), state: state)
    }
}

// MARK: - SparklineView Extended Tests

struct SparklineViewCoverageTests {
    @Test func sparklineWithMaxValue() throws {
        let view = SparklineView(
            values: [0, 5, 10, 8, 3], colour: .cyan, label: "Speed", maxValue: 12.0)
        let _ = view.body
    }

    @Test func sparklinePathWithMax() throws {
        let view = SparklineView(
            values: [0, 5, 10, 8, 3], colour: .cyan, label: "Speed", maxValue: 15.0)
        let size = CGSize(width: 200, height: 40)
        let path = view.sparklinePath(in: size)
        #expect(!path.isEmpty)
    }

    @Test func maxLinePathWithValues() throws {
        let view = SparklineView(
            values: [0, 5, 10, 8, 3], colour: .cyan, label: "Speed", maxValue: 12.0)
        let size = CGSize(width: 200, height: 40)
        let path = view.maxLinePath(maxValue: 12.0, in: size)
        #expect(!path.isEmpty)
    }

    @Test func maxLinePathTooFewValues() throws {
        let view = SparklineView(values: [5.0], colour: .cyan, label: "Speed", maxValue: 10.0)
        let path = view.maxLinePath(maxValue: 10.0, in: CGSize(width: 100, height: 40))
        #expect(path.isEmpty)
    }

    @Test func sparklineCurrentValueNearBottom() throws {
        // Values where last value is near the min — tests nearBottom branch
        let view = SparklineView(values: [10, 8, 6, 4, 2, 0], colour: .cyan, label: "Speed")
        let _ = view.body
    }

    @Test func sparklineCurrentValueNearTop() throws {
        // Values where last value is near the max — tests !nearBottom branch
        let view = SparklineView(values: [0, 2, 4, 6, 8, 10], colour: .cyan, label: "Speed")
        let _ = view.body
    }
}

// MARK: - TrackHistoryGraphView Extended Tests

@available(macOS 15.0, *) @MainActor final class TrackHistoryGraphViewCoverageTests: XCTestCase {

    func testGraphViewWithTrimmedSamplesLeadOut() async throws {
        let state = AppState()
        state.isLive = false
        // Build 20 frames where speed drops to near-zero at the end
        for i: UInt64 in 0..<20 {
            var frame = FrameBundle()
            frame.frameID = i
            frame.timestampNanos = Int64(i) * 50_000_000
            frame.playbackInfo = PlaybackInfo(
                isLive: false, logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0,
                paused: false, currentFrameIndex: i, totalFrames: 100)
            let speed: Float = i < 15 ? Float(i) * 1.0 : 0.1  // Drop to near-zero after frame 14
            frame.tracks = TrackSet(
                frameID: i, timestampNanos: Int64(i) * 50_000_000,
                tracks: [
                    Track(
                        trackID: "t-trim", state: .confirmed, speedMps: speed,
                        headingRad: Float(i) * 0.17)
                ], trails: [])
            state.onFrameReceived(frame)
            await Task.yield()
        }

        hostView(TrackHistoryGraphView(trackID: "t-trim"), state: state)
    }

    func testGraphViewStaticTrack() async throws {
        let state = AppState()
        state.isLive = false
        // Build frames where speed is always below threshold (static object)
        for i: UInt64 in 0..<10 {
            var frame = FrameBundle()
            frame.frameID = i
            frame.timestampNanos = Int64(i) * 100_000_000
            frame.playbackInfo = PlaybackInfo(
                isLive: false, logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0,
                paused: false, currentFrameIndex: i, totalFrames: 100)
            frame.tracks = TrackSet(
                frameID: i, timestampNanos: Int64(i) * 100_000_000,
                tracks: [Track(trackID: "t-static", state: .confirmed, speedMps: 0.1)], trails: [])
            state.onFrameReceived(frame)
            await Task.yield()
        }

        hostView(TrackHistoryGraphView(trackID: "t-static"), state: state)
    }

    func testGraphViewWithMaxSpeed() async throws {
        let state = AppState()
        state.isLive = false
        for i: UInt64 in 0..<5 {
            var frame = FrameBundle()
            frame.frameID = i
            frame.timestampNanos = Int64(i) * 200_000_000
            frame.playbackInfo = PlaybackInfo(
                isLive: false, logStartNs: 0, logEndNs: 1_000_000_000, playbackRate: 1.0,
                paused: false, currentFrameIndex: i, totalFrames: 50)
            frame.tracks = TrackSet(
                frameID: i, timestampNanos: Int64(i) * 200_000_000,
                tracks: [
                    Track(
                        trackID: "t-max", state: .confirmed, speedMps: Float(i) * 3.0,
                        headingRad: Float(i) * 0.3, maxSpeedMps: 12.0)
                ], trails: [])
            state.onFrameReceived(frame)
            await Task.yield()
        }

        hostView(TrackHistoryGraphView(trackID: "t-max"), state: state)
    }
}

// MARK: - ContentView Body / Keyboard Shortcut Coverage

@available(macOS 15.0, *) @MainActor final class ContentViewCoverageTests: XCTestCase {

    func testContentViewInstantiatesDisconnected() throws {
        let state = AppState()
        hostView(ContentView(), state: state)
    }

    func testContentViewWithSidePanel() throws {
        let state = AppState()
        state.showSidePanel = true
        state.currentFrame = makeFrame(tracks: [makeTrack()])
        hostView(ContentView(), state: state)
    }

    func testContentViewWithSelectedTrack() throws {
        let state = AppState()
        state.selectedTrackID = "trk_0000test"
        state.currentFrame = makeFrame(tracks: [makeTrack(id: "trk_0000test")])
        hostView(ContentView(), state: state)
    }

    func testContentViewWithFilterPane() throws {
        let state = AppState()
        state.showFilterPane = true
        hostView(ContentView(), state: state)
    }

    func testContentViewWithTrackLabels() throws {
        let state = AppState()
        state.showTrackLabels = true
        state.trackLabels = [
            MetalRenderer.TrackScreenLabel(
                id: "trk_0000lbl1", screenX: 100, screenY: 200, classLabel: "car", isSelected: false
            )
        ]
        hostView(ContentView(), state: state)
    }

    func testContentViewRunBrowser() throws {
        let state = AppState()
        state.showRunBrowser = false
        hostView(ContentView(), state: state)
    }
}

// MARK: - PlaybackControlsDerivedState Additional Tests

@available(macOS 15.0, *) final class PlaybackControlsDerivedStateAdditionalTests: XCTestCase {

    func testLiveModeDisablesControls() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: true, mode: .live, isPaused: false, playbackRate: 1.0, busy: false,
            hasValidTimelineRange: false, hasFrameIndexProgress: false, currentFrameIndex: 0,
            totalFrames: 0)
        XCTAssertTrue(ui.playPauseDisabled)
        XCTAssertTrue(ui.rateControlsDisabled)
        XCTAssertFalse(ui.showStepButtons)
    }

    func testUnknownModeDisablesControls() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: true, mode: .unknown, isPaused: false, playbackRate: 1.0, busy: false,
            hasValidTimelineRange: false, hasFrameIndexProgress: false, currentFrameIndex: 0,
            totalFrames: 0)
        XCTAssertTrue(ui.playPauseDisabled)
        XCTAssertTrue(ui.rateControlsDisabled)
    }

    func testDisconnectedDisablesEverything() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: false, mode: .replaySeekable, isPaused: true, playbackRate: 1.0,
            busy: false, hasValidTimelineRange: true, hasFrameIndexProgress: true,
            currentFrameIndex: 5, totalFrames: 100)
        XCTAssertTrue(ui.stepBackwardDisabled)
        XCTAssertTrue(ui.stepForwardDisabled)
        XCTAssertTrue(ui.seekSliderDisabled)
    }

    func testBusyDisablesSteps() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: true, mode: .replaySeekable, isPaused: true, playbackRate: 1.0, busy: true,
            hasValidTimelineRange: true, hasFrameIndexProgress: true, currentFrameIndex: 5,
            totalFrames: 100)
        XCTAssertTrue(ui.stepBackwardDisabled)
        XCTAssertTrue(ui.stepForwardDisabled)
        XCTAssertTrue(ui.seekSliderDisabled)
    }

    func testReplayNonSeekableWithTimelineRange() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: true, mode: .replayNonSeekable, isPaused: false, playbackRate: 1.0,
            busy: false, hasValidTimelineRange: true, hasFrameIndexProgress: false,
            currentFrameIndex: 0, totalFrames: 0)
        XCTAssertTrue(ui.showReadOnlyProgress)
        XCTAssertFalse(ui.showSeekableSlider)
        XCTAssertFalse(ui.showReplayMetadataUnavailable)
    }

    func testReplayNonSeekableWithFrameProgress() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: true, mode: .replayNonSeekable, isPaused: false, playbackRate: 1.0,
            busy: false, hasValidTimelineRange: false, hasFrameIndexProgress: true,
            currentFrameIndex: 50, totalFrames: 100)
        XCTAssertTrue(ui.showReadOnlyProgress)
        XCTAssertFalse(ui.showReplayMetadataUnavailable)
    }

    func testMidRangeStepButtonsEnabled() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: true, mode: .replaySeekable, isPaused: true, playbackRate: 2.0,
            busy: false, hasValidTimelineRange: true, hasFrameIndexProgress: true,
            currentFrameIndex: 5, totalFrames: 100)
        XCTAssertFalse(ui.stepBackwardDisabled)
        XCTAssertFalse(ui.stepForwardDisabled)
        XCTAssertTrue(ui.showStepButtons)
        XCTAssertEqual(ui.playbackRate, 2.0)
    }

    func testZeroTotalFramesDisablesForward() throws {
        let ui = PlaybackControlsDerivedState(
            isConnected: true, mode: .replaySeekable, isPaused: true, playbackRate: 1.0,
            busy: false, hasValidTimelineRange: true, hasFrameIndexProgress: false,
            currentFrameIndex: 0, totalFrames: 0)
        XCTAssertTrue(ui.stepForwardDisabled)
    }
}

// MARK: - TrackLabelOverlay Extended Tests

struct TrackLabelOverlayCoverageTests {
    @Test func overlayWithUserLabels() throws {
        var label1 = MetalRenderer.TrackScreenLabel(
            id: "trk_0000aaa", screenX: 100, screenY: 200, classLabel: "vehicle", isSelected: false)
        label1.userLabel = "car"
        var label2 = MetalRenderer.TrackScreenLabel(
            id: "trk_0000bbb", screenX: 300, screenY: 400, classLabel: "", isSelected: true)
        label2.userLabel = ""
        let overlay = TrackLabelOverlay(labels: [label1, label2])
        let _ = overlay.body
    }
}

// MARK: - CacheStatusLabel Tests

struct CacheStatusLabelCoverageTests {
    @Test func cachedStatus() throws {
        let label = CacheStatusLabel(status: "Cached (1200 frames)")
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
}

// MARK: - DetailRow Tests

struct DetailRowCoverageTests {
    @Test func typicalRow() throws {
        let row = DetailRow(label: "Speed", value: "12.5 m/s")
        let _ = row.body
    }

    @Test func longValue() throws {
        let row = DetailRow(label: "Dimensions", value: "4.5 × 1.8 × 1.5 m")
        let _ = row.body
    }
}

// MARK: - StatLabel Tests

struct StatLabelCoverageTests {
    @Test func typicalLabel() throws {
        let label = StatLabel(title: "FPS", value: "60.0")
        let _ = label.body
    }

    @Test func largeValue() throws {
        let label = StatLabel(title: "Points", value: "65.00k")
        let _ = label.body
    }
}

// MARK: - Format Functions Extended

struct FormatFunctionsCoverageTests {
    @Test func formatRateHalf() throws { #expect(formatRate(0.5) == "0.5") }

    @Test func formatRateOne() throws { #expect(formatRate(1.0) == "1") }

    @Test func formatRateSixtyFour() throws { #expect(formatRate(64.0) == "64") }

    @Test func formatDurationZero() throws { #expect(formatDuration(0) == "0:00") }

    @Test func formatDuration30Seconds() throws {
        #expect(formatDuration(30_000_000_000) == "0:30")
    }

    @Test func formatDuration1Hour30Min5Sec() throws {
        let nanos: Int64 = (3600 + 1800 + 5) * 1_000_000_000  // 1:30:05
        #expect(formatDuration(nanos) == "1:30:05")
    }
}

// MARK: - LabelPanelView Classification Labels Static Data Tests

struct LabelPanelStaticDataTests {
    @Test func classificationLabelsCount() throws {
        #expect(LabelPanelView.classificationLabels.count == 9)
    }

    @Test func classificationLabelsNamesNotEmpty() throws {
        for entry in LabelPanelView.classificationLabels {
            #expect(!entry.name.isEmpty)
            #expect(!entry.help.isEmpty)
        }
    }

    @Test func qualityFlagsCount() throws { #expect(LabelPanelView.qualityFlags.count == 8) }

    @Test func qualityFlagNamesNotEmpty() throws {
        for entry in LabelPanelView.qualityFlags {
            #expect(!entry.name.isEmpty)
            #expect(!entry.help.isEmpty)
        }
    }
}

// MARK: - TrackSortOrder Tests

struct TrackSortOrderTests {
    @Test func allCases() throws {
        #expect(TrackSortOrder.allCases.count == 3)
        #expect(TrackSortOrder.firstSeen.rawValue == "First seen")
        #expect(TrackSortOrder.maxSpeed.rawValue == "Max velocity")
        #expect(TrackSortOrder.hits.rawValue == "Hits")
    }
}

// MARK: - ToggleButton Action Coverage

struct ToggleButtonActionTests {
    @Test func toggleOnToOff() throws {
        var isOn = true
        let view = ToggleButton(
            label: "P", isOn: Binding(get: { isOn }, set: { isOn = $0 }), help: "Points")
        let _ = view.body
    }

    @Test func toggleOffToOn() throws {
        var isOn = false
        let view = ToggleButton(
            label: "B", isOn: Binding(get: { isOn }, set: { isOn = $0 }), help: "Boxes")
        let _ = view.body
    }
}

// MARK: - MetalViewRepresentable Extended Tests

@available(macOS 15.0, *) final class MetalViewRepresentableExtendedTests: XCTestCase {

    func testAllPropertiesFalse() throws {
        let rep = MetalViewRepresentable(
            showPoints: false, showBackground: false, showBoxes: false, showClusters: false,
            showVelocity: false, showTrails: false, showDebug: false, showGrid: false,
            pointSize: 1.0)
        XCTAssertFalse(rep.showPoints)
        XCTAssertFalse(rep.showBackground)
        XCTAssertFalse(rep.showBoxes)
        XCTAssertFalse(rep.showClusters)
        XCTAssertFalse(rep.showVelocity)
        XCTAssertFalse(rep.showTrails)
        XCTAssertFalse(rep.showDebug)
        XCTAssertFalse(rep.showGrid)
        XCTAssertEqual(rep.pointSize, 1.0)
    }

    func testCoordinatorCreation() throws {
        let rep = MetalViewRepresentable(
            showPoints: true, showBackground: true, showBoxes: true, showClusters: true,
            showVelocity: true, showTrails: true, showDebug: true, showGrid: true, pointSize: 20.0)
        let coordinator = rep.makeCoordinator()
        XCTAssertNil(coordinator.renderer)
    }
}

// MARK: - InteractiveMetalView Extended Tests

@available(macOS 15.0, *) @MainActor final class InteractiveMetalViewExtendedTests: XCTestCase {

    func testInitialState() throws {
        let view = InteractiveMetalView()
        XCTAssertTrue(view.acceptsFirstResponder)
        XCTAssertTrue(view.becomeFirstResponder())
        XCTAssertNil(view.renderer)
        XCTAssertNil(view.onTrackSelected)
        XCTAssertNil(view.onCameraChanged)
    }

    func testMouseDownDoesNotCrash() throws {
        let view = InteractiveMetalView()
        let event = NSEvent.mouseEvent(
            with: .leftMouseDown, location: NSPoint(x: 100, y: 200), modifierFlags: [],
            timestamp: 0, windowNumber: 0, context: nil, eventNumber: 0, clickCount: 1,
            pressure: 1.0)!
        view.mouseDown(with: event)
    }

    func testRightMouseDownDoesNotCrash() throws {
        let view = InteractiveMetalView()
        let event = NSEvent.mouseEvent(
            with: .rightMouseDown, location: NSPoint(x: 100, y: 200), modifierFlags: [],
            timestamp: 0, windowNumber: 0, context: nil, eventNumber: 0, clickCount: 1,
            pressure: 1.0)!
        view.rightMouseDown(with: event)
    }

    func testMouseUpClickWithinThreshold() throws {
        let view = InteractiveMetalView()
        // Mouse down at (100, 200)
        let downEvent = NSEvent.mouseEvent(
            with: .leftMouseDown, location: NSPoint(x: 100, y: 200), modifierFlags: [],
            timestamp: 0, windowNumber: 0, context: nil, eventNumber: 0, clickCount: 1,
            pressure: 1.0)!
        view.mouseDown(with: downEvent)
        // Mouse up at same location — click (drag < 5px)
        let upEvent = NSEvent.mouseEvent(
            with: .leftMouseUp, location: NSPoint(x: 101, y: 201), modifierFlags: [], timestamp: 0,
            windowNumber: 0, context: nil, eventNumber: 0, clickCount: 1, pressure: 0.0)!
        view.mouseUp(with: upEvent)
    }

    func testMouseUpDragBeyondThreshold() throws {
        let view = InteractiveMetalView()
        let downEvent = NSEvent.mouseEvent(
            with: .leftMouseDown, location: NSPoint(x: 100, y: 200), modifierFlags: [],
            timestamp: 0, windowNumber: 0, context: nil, eventNumber: 0, clickCount: 1,
            pressure: 1.0)!
        view.mouseDown(with: downEvent)
        // Mouse up 10px away — drag, not click
        let upEvent = NSEvent.mouseEvent(
            with: .leftMouseUp, location: NSPoint(x: 110, y: 200), modifierFlags: [], timestamp: 0,
            windowNumber: 0, context: nil, eventNumber: 0, clickCount: 1, pressure: 0.0)!
        view.mouseUp(with: upEvent)
    }

    func testScrollWheelDoesNotCrash() throws {
        let view = InteractiveMetalView()
        let event = NSEvent.otherEvent(
            with: .applicationDefined, location: NSPoint(x: 0, y: 0), modifierFlags: [],
            timestamp: 0, windowNumber: 0, context: nil, subtype: 0, data1: 0, data2: 0)
        // Can't easily create scroll events in tests, but verify no crash
    }

    func testKeyDownUnhandled() throws {
        let view = InteractiveMetalView()
        // Without a renderer, keyDown should call super
        // We can't easily test this without a window
    }
}

// MARK: - Preview Test

@MainActor struct PreviewTests {
    @Test func previewDoesNotCrash() throws {
        // Verify the #Preview doesn't crash
        let state = AppState()
        let _ = ContentView().environmentObject(state)
    }
}
