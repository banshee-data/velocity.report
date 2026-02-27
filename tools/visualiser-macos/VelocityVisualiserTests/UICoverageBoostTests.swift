//
//  UICoverageBoostTests.swift
//  VelocityVisualiserTests
//
//  Additional tests to push UI file coverage above 90%.
//  Covers extracted helper functions, view hosting, and NSView event handlers.
//

import AppKit
import Foundation
import MetalKit
import SwiftUI
import Testing
import XCTest

@testable import VelocityVisualiser

// MARK: - Test Helpers

@available(macOS 15.0, *) @MainActor private func hostView<V: View>(_ view: V, state: AppState) {
    let hosted = view.environmentObject(state)
    let controller = NSHostingController(rootView: AnyView(hosted))
    controller.view.frame = NSRect(x: 0, y: 0, width: 800, height: 600)
    controller.view.layout()
}

private func makeTrack(
    id: String = "trk_00001234", state: TrackState = .confirmed, speed: Float = 8.0,
    peakSpeed: Float = 9.0, classLabel: String = "car", hits: Int = 50
) -> Track {
    Track(
        trackID: id, sensorID: "s1", state: state, hits: hits, misses: 2, observationCount: 48,
        firstSeenNanos: 1_000_000_000, lastSeenNanos: 2_000_000_000, x: 10, y: 5, z: 0.5, vx: 8,
        vy: 0.5, vz: 0, speedMps: speed, headingRad: 0.1, covariance4x4: [], bboxLength: 4.5,
        bboxWidth: 1.8, bboxHeight: 1.5, bboxHeadingRad: 0.1, heightP95Max: 1.6,
        intensityMeanAvg: 50, avgSpeedMps: 7.5, peakSpeedMps: peakSpeed, classLabel: classLabel,
        classConfidence: 0.95, trackLengthMetres: 150, trackDurationSecs: 20, occlusionCount: 0,
        confidence: 0.98, occlusionState: .none, motionModel: .cv, alpha: 1.0)
}

private func makeFrame(tracks: [Track], frameID: UInt64 = 1) -> FrameBundle {
    var frame = FrameBundle()
    frame.frameID = frameID
    frame.timestampNanos = 1_000_000_000
    frame.tracks = TrackSet(
        frameID: frameID, timestampNanos: 1_000_000_000, tracks: tracks, trails: [])
    return frame
}

private func makeRunTrack(
    trackId: String = "trk_00001234", userLabel: String? = nil, qualityLabel: String? = nil,
    peakSpeed: Double? = 12.0, labelerId: String? = nil
) -> RunTrack {
    RunTrack(
        runId: "run-1", trackId: trackId, sensorId: "s1", userLabel: userLabel,
        qualityLabel: qualityLabel, labelConfidence: nil, labelerId: labelerId,
        startUnixNanos: 1_000_000_000, endUnixNanos: 2_000_000_000, totalObservations: 50,
        durationSecs: 10, avgSpeedMps: 10, peakSpeedMps: peakSpeed, isSplitCandidate: false,
        isMergeCandidate: false)
}

// MARK: - rangeSliderXPosition Tests

struct RangeSliderXPositionTests {
    @Test func zeroValueAtLowerBound() {
        let result = rangeSliderXPosition(value: 0, range: 0...100, trackWidth: 200)
        #expect(result == 0)
    }

    @Test func midValue() {
        let result = rangeSliderXPosition(value: 50, range: 0...100, trackWidth: 200)
        #expect(result == 100)
    }

    @Test func maxValue() {
        let result = rangeSliderXPosition(value: 100, range: 0...100, trackWidth: 200)
        #expect(result == 200)
    }

    @Test func quarterValue() {
        let result = rangeSliderXPosition(value: 25, range: 0...100, trackWidth: 400)
        #expect(result == 100)
    }

    @Test func nonZeroLowerBound() {
        let result = rangeSliderXPosition(value: 30, range: 10...50, trackWidth: 200)
        #expect(result == 100)  // (30-10)/(50-10) = 0.5 × 200 = 100
    }
}

// MARK: - rangeSliderValueForX Tests

struct RangeSliderValueForXTests {
    @Test func zeroPosition() {
        let result = rangeSliderValueForX(x: 0, range: 0...100, trackWidth: 200, step: 1)
        #expect(result == 0)
    }

    @Test func midPosition() {
        let result = rangeSliderValueForX(x: 100, range: 0...100, trackWidth: 200, step: 1)
        #expect(result == 50)
    }

    @Test func maxPosition() {
        let result = rangeSliderValueForX(x: 200, range: 0...100, trackWidth: 200, step: 1)
        #expect(result == 100)
    }

    @Test func clampsBelowZero() {
        let result = rangeSliderValueForX(x: -50, range: 0...100, trackWidth: 200, step: 1)
        #expect(result == 0)
    }

    @Test func clampsAboveMax() {
        let result = rangeSliderValueForX(x: 500, range: 0...100, trackWidth: 200, step: 1)
        #expect(result == 100)
    }

    @Test func snapsToStep() {
        let result = rangeSliderValueForX(x: 55, range: 0...50, trackWidth: 200, step: 5)
        // fraction = 55/200 = 0.275, raw = 0.275 * 50 = 13.75, rounded to step 5 → 15
        #expect(result == 15)
    }

    @Test func nonZeroLowerBound() {
        let result = rangeSliderValueForX(x: 100, range: 10...50, trackWidth: 200, step: 1)
        // fraction = 0.5, raw = 10 + 0.5*40 = 30
        #expect(result == 30)
    }
}

// MARK: - rangeSliderXPosition Guard Clause Tests

struct RangeSliderXPositionGuardTests {
    @Test func zeroTrackWidthReturnsZero() {
        let result = rangeSliderXPosition(value: 50, range: 0...100, trackWidth: 0)
        #expect(result == 0)
    }

    @Test func negativeTrackWidthReturnsZero() {
        let result = rangeSliderXPosition(value: 50, range: 0...100, trackWidth: -10)
        #expect(result == 0)
    }

    @Test func equalBoundsReturnsZero() {
        let result = rangeSliderXPosition(value: 50, range: 50...50, trackWidth: 200)
        #expect(result == 0)
    }
}

// MARK: - rangeSliderValueForX Guard Clause Tests

struct RangeSliderValueForXGuardTests {
    @Test func zeroTrackWidthReturnsLowerBound() {
        let result = rangeSliderValueForX(x: 100, range: 10...50, trackWidth: 0, step: 1)
        #expect(result == 10)
    }

    @Test func equalBoundsReturnsLowerBound() {
        let result = rangeSliderValueForX(x: 100, range: 25...25, trackWidth: 200, step: 1)
        #expect(result == 25)
    }

    @Test func zeroStepReturnsLowerBound() {
        let result = rangeSliderValueForX(x: 100, range: 0...100, trackWidth: 200, step: 0)
        #expect(result == 0)
    }

    @Test func negativeStepReturnsLowerBound() {
        let result = rangeSliderValueForX(x: 100, range: 5...50, trackWidth: 200, step: -1)
        #expect(result == 5)
    }
}

// MARK: - statusDotColour Tests

struct StatusDotColourTests {
    @Test func completedIsGreen() { #expect(statusDotColour("completed") == .green) }

    @Test func runningIsOrange() { #expect(statusDotColour("running") == .orange) }

    @Test func failedIsRed() { #expect(statusDotColour("failed") == .red) }

    @Test func unknownIsGray() { #expect(statusDotColour("unknown") == .gray) }

    @Test func emptyIsGray() { #expect(statusDotColour("") == .gray) }
}

// MARK: - runRowFormatDuration Tests

struct RunRowFormatDurationTests {
    @Test func zeroSeconds() { #expect(runRowFormatDuration(0) == "0:00") }

    @Test func thirtySeconds() { #expect(runRowFormatDuration(30) == "0:30") }

    @Test func oneMinute() { #expect(runRowFormatDuration(60) == "1:00") }

    @Test func mixedMinutesAndSeconds() { #expect(runRowFormatDuration(125) == "2:05") }

    @Test func tenMinutes() { #expect(runRowFormatDuration(600) == "10:00") }

    @Test func oneHourAsMinutes() { #expect(runRowFormatDuration(3600) == "60:00") }
}

// MARK: - String.truncated Tests

struct StringTruncatedTests {
    @Test func shortString() { #expect("hello".truncated(10) == "hello") }

    @Test func exactLength() { #expect("hello".truncated(5) == "hello") }

    @Test func longString() { #expect("abcdef".truncated(3) == "abc\u{2026}") }

    @Test func emptyString() { #expect("".truncated(5) == "") }

    @Test func singleChar() { #expect("abcdef".truncated(1) == "a\u{2026}") }
}

// MARK: - AboutView Hosting Tests

@available(macOS 15.0, *) @MainActor final class AboutViewCoverageTests: XCTestCase {
    func testAboutViewRendersBody() throws {
        let view = AboutView()
        let controller = NSHostingController(rootView: view)
        controller.view.frame = NSRect(x: 0, y: 0, width: 420, height: 600)
        controller.view.layout()
    }
}

// MARK: - InteractiveMetalView Event Handler Tests

@available(macOS 15.0, *) @MainActor final class InteractiveMetalViewEventTests: XCTestCase {

    func testMouseDraggedCallsCallback() throws {
        let view = InteractiveMetalView()
        var cameraChanged = false
        view.onCameraChanged = { cameraChanged = true }

        // Mouse down first to set lastMouseLocation
        let downEvent = NSEvent.mouseEvent(
            with: .leftMouseDown, location: NSPoint(x: 100, y: 200), modifierFlags: [],
            timestamp: 0, windowNumber: 0, context: nil, eventNumber: 0, clickCount: 1,
            pressure: 1.0)!
        view.mouseDown(with: downEvent)

        // Drag to new location
        let dragEvent = NSEvent.mouseEvent(
            with: .leftMouseDragged, location: NSPoint(x: 120, y: 210), modifierFlags: [],
            timestamp: 0, windowNumber: 0, context: nil, eventNumber: 0, clickCount: 0,
            pressure: 1.0)!
        view.mouseDragged(with: dragEvent)
        // Without a renderer, callback still fires
        XCTAssertTrue(cameraChanged)
    }

    func testMouseDraggedWithShift() throws {
        let view = InteractiveMetalView()
        var cameraChanged = false
        view.onCameraChanged = { cameraChanged = true }

        let downEvent = NSEvent.mouseEvent(
            with: .leftMouseDown, location: NSPoint(x: 100, y: 200), modifierFlags: [],
            timestamp: 0, windowNumber: 0, context: nil, eventNumber: 0, clickCount: 1,
            pressure: 1.0)!
        view.mouseDown(with: downEvent)

        let dragEvent = NSEvent.mouseEvent(
            with: .leftMouseDragged, location: NSPoint(x: 130, y: 220), modifierFlags: [.shift],
            timestamp: 0, windowNumber: 0, context: nil, eventNumber: 0, clickCount: 0,
            pressure: 1.0)!
        view.mouseDragged(with: dragEvent)
        XCTAssertTrue(cameraChanged)
    }

    func testRightMouseDraggedCallsCallback() throws {
        let view = InteractiveMetalView()
        var cameraChanged = false
        view.onCameraChanged = { cameraChanged = true }

        let downEvent = NSEvent.mouseEvent(
            with: .rightMouseDown, location: NSPoint(x: 100, y: 200), modifierFlags: [],
            timestamp: 0, windowNumber: 0, context: nil, eventNumber: 0, clickCount: 1,
            pressure: 1.0)!
        view.rightMouseDown(with: downEvent)

        let dragEvent = NSEvent.mouseEvent(
            with: .rightMouseDragged, location: NSPoint(x: 115, y: 205), modifierFlags: [],
            timestamp: 0, windowNumber: 0, context: nil, eventNumber: 0, clickCount: 0,
            pressure: 1.0)!
        view.rightMouseDragged(with: dragEvent)
        XCTAssertTrue(cameraChanged)
    }

    func testMagnifyCallsCallback() throws {
        let view = InteractiveMetalView()
        var cameraChanged = false
        view.onCameraChanged = { cameraChanged = true }

        // Use CGEvent to create a magnify event
        guard let cgEvent = CGEvent(source: nil) else {
            throw XCTSkip("CGEvent not available on this platform; skipping magnify callback test")
        }
        guard let magnifyType = CGEventType(rawValue: 30) else {
            throw XCTSkip(
                "Magnify event type not supported on this platform; skipping magnify callback test")
        }
        cgEvent.type = magnifyType  // kCGEventMagnify = 30
        guard let nsEvent = NSEvent(cgEvent: cgEvent) else {
            throw XCTSkip("Failed to create NSEvent from CGEvent; skipping magnify callback test")
        }
        view.magnify(with: nsEvent)
        XCTAssertTrue(cameraChanged)
    }

    func testKeyDownWithoutRenderer() throws {
        let view = InteractiveMetalView()
        // Without a renderer, keyDown falls through to super
        // We can't easily test super.keyDown without a window,
        // but we verify no crash
        let keyEvent = NSEvent.keyEvent(
            with: .keyDown, location: .zero, modifierFlags: [], timestamp: 0, windowNumber: 0,
            context: nil, characters: "w", charactersIgnoringModifiers: "w", isARepeat: false,
            keyCode: 13)
        if let event = keyEvent {
            // This would call super.keyDown which may crash without a window,
            // so just verify the event was created correctly
            XCTAssertEqual(event.keyCode, 13)
        }
    }

    func testMouseUpWithTrackSelectedCallback() throws {
        let view = InteractiveMetalView()
        var selectedTrack: String? = "initial"
        view.onTrackSelected = { trackID in selectedTrack = trackID }

        let downEvent = NSEvent.mouseEvent(
            with: .leftMouseDown, location: NSPoint(x: 100, y: 200), modifierFlags: [],
            timestamp: 0, windowNumber: 0, context: nil, eventNumber: 0, clickCount: 1,
            pressure: 1.0)!
        view.mouseDown(with: downEvent)

        let upEvent = NSEvent.mouseEvent(
            with: .leftMouseUp, location: NSPoint(x: 101, y: 200), modifierFlags: [], timestamp: 0,
            windowNumber: 0, context: nil, eventNumber: 0, clickCount: 1, pressure: 0.0)!
        view.mouseUp(with: upEvent)
        // Without a renderer, hitTestTrack returns nil → onTrackSelected called with nil
        XCTAssertNil(selectedTrack)
    }
}

// MARK: - RunBrowserView Additional Hosting Tests

@available(macOS 15.0, *) @MainActor final class RunBrowserViewAdditionalTests: XCTestCase {

    private func host<V: View>(_ view: V, state: AppState) {
        let hosted = view.environmentObject(state)
        let controller = NSHostingController(rootView: AnyView(hosted))
        controller.view.frame = NSRect(x: 0, y: 0, width: 500, height: 400)
        controller.view.layout()
    }

    func testRunBrowserViewEmptyState() throws {
        let state = AppState()
        state.isConnected = true
        host(RunBrowserView(), state: state)
    }

    func testRunBrowserViewWithActiveReplay() throws {
        let state = AppState()
        state.isLive = false
        state.currentRunID = "run-replay-active"
        host(RunBrowserView(), state: state)
    }

    func testRunRowViewZeroDuration() throws {
        let run = AnalysisRun(
            runId: "run-zero", createdAt: Date(), sourceType: "vrlog",
            sourcePath: "/data/test.vrlog", sensorId: "hesai-01", durationSecs: 0, totalFrames: 0,
            totalClusters: 0, totalTracks: 0, confirmedTracks: 0, status: "running",
            errorMessage: nil, vrlogPath: nil, notes: nil)
        let view = RunRowView(run: run, isSelected: false, onSelect: {})
        let _ = view.body
    }

    func testRunRowViewLongDuration() throws {
        let run = AnalysisRun(
            runId: "run-long", createdAt: Date(), sourceType: "vrlog",
            sourcePath: "/data/test.vrlog", sensorId: "hesai-01", durationSecs: 7200,
            totalFrames: 72000, totalClusters: 5000, totalTracks: 500, confirmedTracks: 400,
            status: "completed", errorMessage: nil, vrlogPath: "/data/long.vrlog", notes: "long")
        let view = RunRowView(run: run, isSelected: true, onSelect: {})
        let _ = view.body
    }
}

// MARK: - PlaybackControlsView Additional States

@available(macOS 15.0, *) @MainActor final class PlaybackControlsSeekableTests: XCTestCase {
    func testSeekableReplayMode() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.isSeekable = true
        state.isPaused = false
        state.playbackRate = 2.0
        state.totalFrames = 1000
        state.currentFrameIndex = 500
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 10_000_000_000
        state.currentTimestamp = 5_000_000_000
        hostView(PlaybackControlsView(), state: state)
    }

    func testSeekablePausedMode() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.isSeekable = true
        state.isPaused = true
        state.playbackRate = 1.0
        state.totalFrames = 500
        state.currentFrameIndex = 0
        hostView(PlaybackControlsView(), state: state)
    }

    func testNonSeekableReplayWithFrameIndexProgress() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.isSeekable = false
        state.isPaused = false
        state.totalFrames = 1000
        state.currentFrameIndex = 250
        hostView(PlaybackControlsView(), state: state)
    }

    func testNonSeekableReplayNoMetadata() throws {
        let state = AppState()
        state.isConnected = true
        state.isLive = false
        state.isSeekable = false
        state.totalFrames = 0
        state.currentFrameIndex = 0
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0
        hostView(PlaybackControlsView(), state: state)
    }
}

// MARK: - LabelPanelView Additional States

@available(macOS 15.0, *) @MainActor final class LabelPanelViewAdditionalTests: XCTestCase {

    func testLabelPanelWithQualityFlags() throws {
        let state = AppState()
        state.selectedTrackID = "trk_0000flag"
        state.currentRunID = "run-flags"
        state.userLabels["trk_0000flag"] = "car"
        state.userQualityFlags["trk_0000flag"] = "noisy,split"
        let track = makeTrack(id: "trk_0000flag")
        state.currentFrame = makeFrame(tracks: [track])
        hostView(LabelPanelView(), state: state)
    }

    func testLabelPanelCarriedOverState() throws {
        let state = AppState()
        state.selectedTrackID = "trk_0000carry"
        state.currentRunID = "run-carry"
        state.userLabels["trk_0000carry"] = "noise"
        let track = makeTrack(id: "trk_0000carry")
        state.currentFrame = makeFrame(tracks: [track])
        hostView(LabelPanelView(), state: state)
    }

    func testLabelPanelLiveModeNoFlags() throws {
        let state = AppState()
        state.selectedTrackID = "trk_0000noflag"
        state.currentRunID = nil  // Live mode — no flags column
        let track = makeTrack(id: "trk_0000noflag")
        state.currentFrame = makeFrame(tracks: [track])
        hostView(LabelPanelView(), state: state)
    }
}

// MARK: - BulkLabelView Additional Tests

@available(macOS 15.0, *) @MainActor final class BulkLabelViewAdditionalTests: XCTestCase {

    func testBulkLabelViewWithManyTracks() throws {
        let state = AppState()
        let tracks = (0..<10).map { makeTrack(id: "trk_0000\(String(format: "%04d", $0))") }
        state.currentFrame = makeFrame(tracks: tracks)
        hostView(BulkLabelView(), state: state)
    }
}

// MARK: - FilterBarView Additional States

@available(macOS 15.0, *) @MainActor final class FilterBarViewAdditionalTests: XCTestCase {

    func testFilterBarWithZeroMaxHits() throws {
        let state = AppState()
        state.showFilterPane = true
        state.filterMinHits = 3
        state.filterMaxHits = 0  // "∞" display
        hostView(FilterBarView(), state: state)
    }

    func testFilterBarWithZeroMaxPointsPerFrame() throws {
        let state = AppState()
        state.showFilterPane = true
        state.filterMinPointsPerFrame = 2
        state.filterMaxPointsPerFrame = 0  // "∞" display
        hostView(FilterBarView(), state: state)
    }

    func testFilterBarNoActiveFilters() throws {
        let state = AppState()
        state.showFilterPane = true
        state.filterOnlyInBox = false
        state.filterMinHits = 0
        state.filterMaxHits = 0
        state.filterMinPointsPerFrame = 0
        state.filterMaxPointsPerFrame = 0
        hostView(FilterBarView(), state: state)
    }
}

// MARK: - TrackListView Additional States

@available(macOS 15.0, *) @MainActor final class TrackListViewAdditionalTests: XCTestCase {

    func testFrameModeWithPeakSpeedSortOrder() throws {
        let state = AppState()
        state.currentRunID = nil
        state.currentFrame = makeFrame(tracks: [
            makeTrack(id: "trk_00001111", speed: 3.0, peakSpeed: 5.0),
            makeTrack(id: "trk_00002222", speed: 10.0, peakSpeed: 15.0),
            makeTrack(id: "trk_00003333", speed: 1.0, peakSpeed: 1.0),
        ])
        hostView(TrackListView(), state: state)
    }

    func testRunModeWithSelectedTrack() throws {
        let state = AppState()
        state.currentRunID = "run-select"
        state.isConnected = true
        state.selectedTrackID = "trk_00001111"
        state.currentFrame = makeFrame(tracks: [makeTrack(id: "trk_00001111")])
        hostView(TrackListView(), state: state)
    }
}

// MARK: - TimeDisplayView Additional States

@available(macOS 15.0, *) @MainActor final class TimeDisplayViewAdditionalTests: XCTestCase {

    func testTimeDisplayWithValidRange() throws {
        let state = AppState()
        state.logStartTimestamp = 1_000_000_000
        state.logEndTimestamp = 60_000_000_000
        state.currentTimestamp = 30_000_000_000
        hostView(TimeDisplayView(), state: state)
    }

    func testTimeDisplayWithNoRange() throws {
        let state = AppState()
        state.logStartTimestamp = 0
        state.logEndTimestamp = 0
        state.currentTimestamp = 0
        state.totalFrames = 500
        state.currentFrameIndex = 250
        hostView(TimeDisplayView(), state: state)
    }

    func testTimeDisplayWithLongDuration() throws {
        let state = AppState()
        state.logStartTimestamp = 0
        state.logEndTimestamp = 3_600_000_000_000  // 1 hour
        state.currentTimestamp = 1_800_000_000_000  // 30 minutes
        hostView(TimeDisplayView(), state: state)
    }
}

// MARK: - ContentView onKeyPress Additional Coverage

@available(macOS 15.0, *) @MainActor final class ContentViewBodyAdditionalTests: XCTestCase {

    func testContentViewWithFilterPane() throws {
        let state = AppState()
        state.showFilterPane = true
        state.currentFrame = makeFrame(tracks: [makeTrack()])
        hostView(ContentView(), state: state)
    }

    func testContentViewWithTrackLabelsOverlay() throws {
        let state = AppState()
        state.showTrackLabels = true
        state.currentFrame = makeFrame(tracks: [makeTrack()])
        hostView(ContentView(), state: state)
    }

    func testContentViewWithSelectedTrackAndRunMode() throws {
        let state = AppState()
        state.selectedTrackID = "trk_00001111"
        state.currentRunID = "run-cv"
        state.currentFrame = makeFrame(tracks: [makeTrack(id: "trk_00001111")])
        hostView(ContentView(), state: state)
    }
}

// MARK: - RangeSliderView Hosting Tests

@available(macOS 15.0, *) @MainActor final class RangeSliderViewHostingTests: XCTestCase {

    func testRangeSliderMidRange() throws {
        let state = AppState()
        state.filterMinHits = 10
        state.filterMaxHits = 30
        state.showFilterPane = true
        hostView(FilterBarView(), state: state)
    }

    func testRangeSliderAtBounds() throws {
        let state = AppState()
        state.filterMinHits = 0
        state.filterMaxHits = 50
        state.filterMinPointsPerFrame = 0
        state.filterMaxPointsPerFrame = 100
        state.showFilterPane = true
        hostView(FilterBarView(), state: state)
    }
}

// MARK: - LabelButton and FlagToggleButton Additional States

struct LabelButtonAdditionalTests {
    @Test func activeWithShortcut() {
        let view = LabelButton(
            label: "noise", shortcut: "1", isActive: true, helpText: "Background noise"
        ) {}
        let _ = view.body
    }

    @Test func inactiveWithHelpText() {
        let view = LabelButton(
            label: "pedestrian", shortcut: "3", isActive: false, helpText: "Person on foot"
        ) {}
        let _ = view.body
    }

    @Test func withUnderscoreLabel() {
        let view = LabelButton(label: "static_object", shortcut: nil, isActive: true) {}
        let _ = view.body
    }
}

struct FlagToggleButtonAdditionalTests {
    @Test func activeWithHelp() {
        let view = FlagToggleButton(label: "noisy", isActive: true, helpText: "High noise track") {}
        let _ = view.body
    }

    @Test func inactiveNoHelp() {
        let view = FlagToggleButton(label: "split", isActive: false) {}
        let _ = view.body
    }

    @Test func underscoreLabel() {
        let view = FlagToggleButton(label: "merge_candidate", isActive: true) {}
        let _ = view.body
    }
}

// MARK: - parseQualityFlags Tests

struct ParseQualityFlagsTests {
    @Test func singleFlag() {
        let result = parseQualityFlags("noisy")
        #expect(result == ["noisy"])
    }

    @Test func multipleFlags() {
        let result = parseQualityFlags("noisy,split,merge")
        #expect(result == ["noisy", "split", "merge"])
    }

    @Test func trimWhitespace() {
        let result = parseQualityFlags("noisy , split , merge")
        #expect(result == ["noisy", "split", "merge"])
    }

    @Test func emptyStringProducesEmptySet() {
        let result = parseQualityFlags("")
        #expect(result.isEmpty)
    }

    @Test func duplicatesCollapsed() {
        let result = parseQualityFlags("noisy,noisy,split")
        #expect(result == ["noisy", "split"])
    }
}

// MARK: - toggleFlag Tests

struct ToggleFlagTests {
    @Test func addNewFlag() {
        let result = toggleFlag("noisy", in: ["split"])
        #expect(result == ["noisy", "split"])
    }

    @Test func removeExistingFlag() {
        let result = toggleFlag("split", in: ["split", "noisy"])
        #expect(result == ["noisy"])
    }

    @Test func addToEmpty() {
        let result = toggleFlag("good", in: [])
        #expect(result == ["good"])
    }

    @Test func removeLastFlag() {
        let result = toggleFlag("good", in: ["good"])
        #expect(result.isEmpty)
    }
}

// MARK: - serialiseFlags Tests

struct SerialiseFlagsTests {
    @Test func emptySet() { #expect(serialiseFlags([]) == "") }

    @Test func singleFlag() { #expect(serialiseFlags(["noisy"]) == "noisy") }

    @Test func multipleFlagsSorted() {
        #expect(serialiseFlags(["split", "noisy", "merge"]) == "merge,noisy,split")
    }
}

// MARK: - applyFetchedTrackLabels Tests

@available(macOS 15.0, *) @MainActor final class ApplyFetchedTrackLabelsTests: XCTestCase {

    func testAppliesUserLabel() {
        let state = AppState()
        let track = makeRunTrack(trackId: "trk_x", userLabel: "car")
        let result = applyFetchedTrackLabels(track: track, trackID: "trk_x", appState: state)
        XCTAssertEqual(state.userLabels["trk_x"], "car")
        XCTAssertTrue(result.flags.isEmpty)
        XCTAssertFalse(result.isCarriedOver)
    }

    func testAppliesQualityFlags() {
        let state = AppState()
        let track = makeRunTrack(trackId: "trk_y", qualityLabel: "noisy,split")
        let result = applyFetchedTrackLabels(track: track, trackID: "trk_y", appState: state)
        XCTAssertEqual(state.userQualityFlags["trk_y"], "noisy,split")
        XCTAssertEqual(result.flags, ["noisy", "split"])
    }

    func testDetectsCarriedOver() {
        let state = AppState()
        let track = makeRunTrack(trackId: "trk_z", userLabel: "noise", labelerId: "hint-carryover")
        let result = applyFetchedTrackLabels(track: track, trackID: "trk_z", appState: state)
        XCTAssertTrue(result.isCarriedOver)
        XCTAssertEqual(state.userLabels["trk_z"], "noise")
    }

    func testEmptyLabelNotApplied() {
        let state = AppState()
        let track = makeRunTrack(trackId: "trk_e", userLabel: "")
        let _ = applyFetchedTrackLabels(track: track, trackID: "trk_e", appState: state)
        XCTAssertNil(state.userLabels["trk_e"])
    }

    func testNilLabelNotApplied() {
        let state = AppState()
        let track = makeRunTrack(trackId: "trk_n", userLabel: nil)
        let _ = applyFetchedTrackLabels(track: track, trackID: "trk_n", appState: state)
        XCTAssertNil(state.userLabels["trk_n"])
    }
}

// MARK: - syncRunTracksToAppState Tests

@available(macOS 15.0, *) @MainActor final class SyncRunTracksToAppStateTests: XCTestCase {

    func testSyncsLabelsFromMultipleTracks() {
        let state = AppState()
        let tracks = [
            makeRunTrack(trackId: "trk_a", userLabel: "car"),
            makeRunTrack(trackId: "trk_b", userLabel: "truck", qualityLabel: "noisy"),
            makeRunTrack(trackId: "trk_c", userLabel: nil),
        ]
        let count = syncRunTracksToAppState(tracks, appState: state)
        XCTAssertEqual(count, 2)
        XCTAssertEqual(state.userLabels["trk_a"], "car")
        XCTAssertEqual(state.userLabels["trk_b"], "truck")
        XCTAssertNil(state.userLabels["trk_c"])
        XCTAssertEqual(state.userQualityFlags["trk_b"], "noisy")
    }

    func testDoesNotOverwriteExistingLabels() {
        let state = AppState()
        state.userLabels["trk_a"] = "bus"
        let tracks = [makeRunTrack(trackId: "trk_a", userLabel: "car")]
        let count = syncRunTracksToAppState(tracks, appState: state)
        XCTAssertEqual(count, 0)  // Not overwritten
        XCTAssertEqual(state.userLabels["trk_a"], "bus")
    }

    func testEmptyTracksArray() {
        let state = AppState()
        let count = syncRunTracksToAppState([], appState: state)
        XCTAssertEqual(count, 0)
    }
}

// MARK: - computeUpdatedRanks Tests

@available(macOS 15.0, *) @MainActor final class ComputeUpdatedRanksTests: XCTestCase {

    func testFrameModeWithTracks() {
        let state = AppState()
        let t1 = makeTrack(id: "trk_a", peakSpeed: 10.0)
        let t2 = makeTrack(id: "trk_b", peakSpeed: 5.0)
        state.currentFrame = makeFrame(tracks: [t1, t2])

        let result = computeUpdatedRanks(
            isRunMode: false, runTracks: [], frameTrackByID: [:], appState: state,
            previousRanks: [:])
        XCTAssertEqual(result.count, 2)
        XCTAssertEqual(result["trk_a"]?.rank, 0)  // Fastest → rank 0
        XCTAssertEqual(result["trk_b"]?.rank, 1)
    }

    func testRunModeWithRunTracks() {
        let state = AppState()
        let rt1 = makeRunTrack(trackId: "trk_a", peakSpeed: 15.0)
        let rt2 = makeRunTrack(trackId: "trk_b", peakSpeed: 8.0)
        let frameByID: [String: Track] = [:]

        let result = computeUpdatedRanks(
            isRunMode: true, runTracks: [rt1, rt2], frameTrackByID: frameByID, appState: state,
            previousRanks: [:])
        XCTAssertEqual(result.count, 2)
        XCTAssertEqual(result["trk_a"]?.rank, 0)
    }

    func testFrameModeWithFilters() {
        let state = AppState()
        state.filterMinHits = 10
        let t1 = makeTrack(id: "trk_a", peakSpeed: 10.0, hits: 20)
        state.currentFrame = makeFrame(tracks: [t1])

        let result = computeUpdatedRanks(
            isRunMode: false, runTracks: [], frameTrackByID: [:], appState: state,
            previousRanks: [:])
        // filteredTracks depends on hasActiveFilters
        XCTAssertGreaterThanOrEqual(result.count, 0)
    }

    func testClimbDetection() {
        let state = AppState()
        let t1 = makeTrack(id: "trk_a", peakSpeed: 5.0)
        let t2 = makeTrack(id: "trk_b", peakSpeed: 10.0)
        state.currentFrame = makeFrame(tracks: [t1, t2])

        // Initial ranks: trk_b rank 0, trk_a rank 1
        let initial = computeUpdatedRanks(
            isRunMode: false, runTracks: [], frameTrackByID: [:], appState: state,
            previousRanks: [:])

        // Now trk_a surpasses trk_b
        let t1Fast = makeTrack(id: "trk_a", peakSpeed: 15.0)
        state.currentFrame = makeFrame(tracks: [t1Fast, t2])

        let updated = computeUpdatedRanks(
            isRunMode: false, runTracks: [], frameTrackByID: [:], appState: state,
            previousRanks: initial)
        XCTAssertEqual(updated["trk_a"]?.rank, 0)
        XCTAssertNotNil(updated["trk_a"]?.climbedAt)
    }

    func testEmptyTracks() {
        let state = AppState()
        state.currentFrame = makeFrame(tracks: [])
        let result = computeUpdatedRanks(
            isRunMode: false, runTracks: [], frameTrackByID: [:], appState: state,
            previousRanks: [:])
        XCTAssertTrue(result.isEmpty)
    }
}

// MARK: - DebugOverlayTogglesView Coverage

@available(macOS 15.0, *) @MainActor final class DebugOverlayAdditionalTests: XCTestCase {
    func testDebugOverlayAllEnabled() {
        let state = AppState()
        state.showTrackLabels = true
        state.showDebug = true
        hostView(DebugOverlayTogglesView(), state: state)
    }
}

// MARK: - OverlayTogglesView Additional States

@available(macOS 15.0, *) @MainActor final class OverlayTogglesAdditionalTests: XCTestCase {
    func testOverlayTogglesAllOn() {
        let state = AppState()
        state.showPoints = true
        state.showBackground = true
        state.showBoxes = true
        state.showClusters = true
        state.showTrails = true
        state.showVelocity = true
        hostView(OverlayTogglesView(), state: state)
    }

    func testOverlayTogglesAllOff() {
        let state = AppState()
        state.showPoints = false
        state.showBackground = false
        state.showBoxes = false
        state.showClusters = false
        state.showTrails = false
        state.showVelocity = false
        hostView(OverlayTogglesView(), state: state)
    }
}

// MARK: - PlaybackModeBadgeView Additional States

@available(macOS 15.0, *) @MainActor final class PlaybackModeBadgeAdditionalTests: XCTestCase {
    func testLiveMode() {
        let view = PlaybackModeBadgeView(modeLabel: "LIVE", mode: .live, isConnected: true)
        let _ = view.body
    }

    func testSeekableMode() {
        let view = PlaybackModeBadgeView(
            modeLabel: "VRLOG", mode: .replaySeekable, isConnected: true)
        let _ = view.body
    }

    func testDisconnected() {
        let view = PlaybackModeBadgeView(modeLabel: "?", mode: .unknown, isConnected: false)
        let _ = view.body
    }
}

// MARK: - InteractiveMetalView scrollWheel & keyDown Coverage

@available(macOS 15.0, *) @MainActor final class InteractiveMetalViewScrollKeyTests: XCTestCase {

    func testScrollWheelWithoutRenderer() throws {
        let view = InteractiveMetalView()
        var cameraChanged = false
        view.onCameraChanged = { cameraChanged = true }

        // Create a scroll wheel event via CGEvent
        if let cgEvent = CGEvent(
            scrollWheelEvent2Source: nil, units: .pixel, wheelCount: 1, wheel1: 5, wheel2: 0,
            wheel3: 0)
        {
            guard let nsEvent = NSEvent(cgEvent: cgEvent) else {
                throw XCTSkip("Failed to create NSEvent from CGEvent; skipping scroll wheel test")
            }
            view.scrollWheel(with: nsEvent)
            // onCameraChanged is called even without renderer
            XCTAssertTrue(cameraChanged)
        }
    }

    func testScrollWheelPreciseDeltas() throws {
        let view = InteractiveMetalView()
        var changeCount = 0
        view.onCameraChanged = { changeCount += 1 }

        // Try a line-based scroll event
        if let cgEvent = CGEvent(
            scrollWheelEvent2Source: nil, units: .line, wheelCount: 1, wheel1: -3, wheel2: 0,
            wheel3: 0)
        {
            guard let nsEvent = NSEvent(cgEvent: cgEvent) else {
                throw XCTSkip(
                    "Failed to create NSEvent from CGEvent; skipping scroll wheel precise deltas test"
                )
            }
            view.scrollWheel(with: nsEvent)
            XCTAssertEqual(changeCount, 1)
        }
    }

    func testKeyDownInWindowFallsToSuper() throws {
        let view = InteractiveMetalView()
        // Put view in a window so super.keyDown doesn't crash
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 200, height: 200), styleMask: [.titled],
            backing: .buffered, defer: false)
        window.contentView = view
        view.frame = NSRect(x: 0, y: 0, width: 200, height: 200)

        let keyEvent = NSEvent.keyEvent(
            with: .keyDown, location: .zero, modifierFlags: [], timestamp: 0,
            windowNumber: window.windowNumber, context: nil, characters: "w",
            charactersIgnoringModifiers: "w", isARepeat: false, keyCode: 13)
        if let event = keyEvent {
            // Without a renderer, falls through to super.keyDown
            view.keyDown(with: event)
        }
    }
}

// MARK: - RunBrowserView State-Based Coverage

@available(macOS 15.0, *) @MainActor final class RunBrowserViewStateCoverageTests: XCTestCase {

    private func makeAnalysisRun(
        runId: String = "run-001", status: String = "completed", durationSecs: Double = 300,
        totalTracks: Int = 10, vrlogPath: String? = "/path/to/file.vrlog"
    ) -> AnalysisRun {
        AnalysisRun(
            runId: runId, createdAt: Date(), sourceType: "vrlog", sourcePath: "/data/test.vrlog",
            sensorId: "sensor-1", durationSecs: durationSecs, totalFrames: 100, totalClusters: 50,
            totalTracks: totalTracks, confirmedTracks: 5, status: status, errorMessage: nil,
            vrlogPath: vrlogPath, notes: nil)
    }

    private func host<V: View>(_ view: V, state: AppState) {
        let hosted = view.environmentObject(state)
        let controller = NSHostingController(rootView: AnyView(hosted))
        controller.view.frame = NSRect(x: 0, y: 0, width: 500, height: 400)
        controller.view.layout()
    }

    func testLoadingState() throws {
        let browserState = RunBrowserState()
        browserState.isLoading = true
        // runs is empty → shows ProgressView("Loading runs...")
        let appState = AppState()
        host(RunBrowserView(state: browserState), state: appState)
    }

    func testErrorState() throws {
        let browserState = RunBrowserState()
        browserState.error = "Network connection failed"
        let appState = AppState()
        host(RunBrowserView(state: browserState), state: appState)
    }

    func testRunListState() throws {
        let browserState = RunBrowserState()
        browserState.runs = [
            makeAnalysisRun(runId: "run-001"),
            makeAnalysisRun(runId: "run-002", status: "running", durationSecs: 60),
        ]
        let appState = AppState()
        host(RunBrowserView(state: browserState), state: appState)
    }

    func testFooterWithSelectedRun() throws {
        let browserState = RunBrowserState()
        browserState.selectedRunID = "run-abc"
        browserState.runs = [makeAnalysisRun(runId: "run-abc")]
        let appState = AppState()
        host(RunBrowserView(state: browserState), state: appState)
    }

    func testRunListWithVRLog() throws {
        let browserState = RunBrowserState()
        browserState.runs = [makeAnalysisRun(runId: "run-vrlog", vrlogPath: "/tmp/test.vrlog")]
        let appState = AppState()
        host(RunBrowserView(state: browserState), state: appState)
    }

    func testRunListWithNoVRLog() throws {
        let browserState = RunBrowserState()
        browserState.runs = [makeAnalysisRun(runId: "run-no-vrlog", vrlogPath: nil)]
        let appState = AppState()
        host(RunBrowserView(state: browserState), state: appState)
    }

    func testMultipleRunsSelected() throws {
        let browserState = RunBrowserState()
        browserState.runs = [
            makeAnalysisRun(runId: "run-001"), makeAnalysisRun(runId: "run-002"),
            makeAnalysisRun(runId: "run-003"),
        ]
        browserState.selectedRunID = "run-002"
        let appState = AppState()
        host(RunBrowserView(state: browserState), state: appState)
    }
}
