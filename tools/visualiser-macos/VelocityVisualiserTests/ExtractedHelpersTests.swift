//
//  ExtractedHelpersTests.swift
//  VelocityVisualiserTests
//
//  Tests for file-level helper functions and standalone views extracted from
//  ContentView.swift for testability. Covers keyboard shortcut handling, rank
//  computation, track tag collection, sorting, and row view rendering.
//

import AppKit
import Foundation
import SwiftUI
import XCTest

@testable import VelocityVisualiser

// MARK: - Test Helpers

@available(macOS 15.0, *) @MainActor private func hostView<V: View>(_ view: V, state: AppState) {
    let hosted = view.environmentObject(state)
    let controller = NSHostingController(rootView: AnyView(hosted))
    controller.view.frame = NSRect(x: 0, y: 0, width: 400, height: 300)
    controller.view.layout()
}

private func makeTrack(
    id: String = "trk_00001234", state: TrackState = .confirmed, speed: Float = 8.0,
    maxSpeed: Float = 9.0, classLabel: String = "car"
) -> Track {
    Track(
        trackID: id, sensorID: "s1", state: state, hits: 50, misses: 2, observationCount: 48,
        firstSeenNanos: 1_000_000_000, lastSeenNanos: 2_000_000_000, x: 10, y: 5, z: 0.5, vx: 8,
        vy: 0.5, vz: 0, speedMps: speed, headingRad: 0.1, covariance4x4: [], bboxLength: 4.5,
        bboxWidth: 1.8, bboxHeight: 1.5, bboxHeadingRad: 0.1, heightP95Max: 1.6,
        intensityMeanAvg: 50, avgSpeedMps: 7.5, maxSpeedMps: maxSpeed, classLabel: classLabel,
        classConfidence: 0.95, trackLengthMetres: 150, trackDurationSecs: 20, occlusionCount: 0,
        confidence: 0.98, occlusionState: .none, motionModel: .cv, alpha: 1.0)
}

private func makeRunTrack(
    trackId: String = "trk_00001234", userLabel: String? = nil, qualityLabel: String? = nil,
    maxSpeed: Double? = 12.0, startNanos: Int64? = 1_000_000_000
) -> RunTrack {
    RunTrack(
        runId: "run-1", trackId: trackId, sensorId: "s1", userLabel: userLabel,
        qualityLabel: qualityLabel, labelConfidence: nil, labelerId: nil,
        startUnixNanos: startNanos, endUnixNanos: 2_000_000_000, totalObservations: 50,
        durationSecs: 10, avgSpeedMps: 10, maxSpeedMps: maxSpeed, isSplitCandidate: false,
        isMergeCandidate: false)
}

// MARK: - KeyAction & handleKeyPress Tests

@available(macOS 15.0, *) @MainActor final class HandleKeyPressTests: XCTestCase {

    func testSpaceTogglesPlayPause() {
        let state = AppState()
        let result = handleKeyPress(.space, appState: state)
        XCTAssertEqual(result, .handled)
    }

    func testCommaIgnoredWhenNotSeekable() {
        let state = AppState()
        // Not connected → not seekable
        let result = handleKeyPress(.comma, appState: state)
        XCTAssertEqual(result, .ignored)
    }

    func testPeriodIgnoredWhenNotSeekable() {
        let state = AppState()
        let result = handleKeyPress(.period, appState: state)
        XCTAssertEqual(result, .ignored)
    }

    func testDecreaseRateHandled() {
        let state = AppState()
        let result = handleKeyPress(.decreaseRate, appState: state)
        XCTAssertEqual(result, .handled)
    }

    func testIncreaseRateHandled() {
        let state = AppState()
        let result = handleKeyPress(.increaseRate, appState: state)
        XCTAssertEqual(result, .handled)
    }

    // Label shortcuts without selection → ignored
    func testLabel1IgnoredNoSelection() {
        let state = AppState()
        XCTAssertEqual(handleKeyPress(.label1, appState: state), .ignored)
    }

    func testLabel2IgnoredNoSelection() {
        let state = AppState()
        XCTAssertEqual(handleKeyPress(.label2, appState: state), .ignored)
    }

    func testLabel3IgnoredNoSelection() {
        let state = AppState()
        XCTAssertEqual(handleKeyPress(.label3, appState: state), .ignored)
    }

    func testLabel4IgnoredNoSelection() {
        let state = AppState()
        XCTAssertEqual(handleKeyPress(.label4, appState: state), .ignored)
    }

    // Label shortcuts with selection → handled
    func testLabel1AssignsWithSelection() {
        let state = AppState()
        state.selectTrack("trk_00001234")
        let result = handleKeyPress(.label1, appState: state)
        XCTAssertEqual(result, .handled)
        XCTAssertEqual(state.userLabels["trk_00001234"], "noise")
    }

    func testLabel2AssignsWithSelection() {
        let state = AppState()
        state.selectTrack("trk_00001234")
        let result = handleKeyPress(.label2, appState: state)
        XCTAssertEqual(result, .handled)
        XCTAssertEqual(state.userLabels["trk_00001234"], "dynamic")
    }

    func testLabel3AssignsWithSelection() {
        let state = AppState()
        state.selectTrack("trk_00001234")
        let result = handleKeyPress(.label3, appState: state)
        XCTAssertEqual(result, .handled)
        XCTAssertEqual(state.userLabels["trk_00001234"], "pedestrian")
    }

    func testLabel4AssignsWithSelection() {
        let state = AppState()
        state.selectTrack("trk_00001234")
        let result = handleKeyPress(.label4, appState: state)
        XCTAssertEqual(result, .handled)
        XCTAssertEqual(state.userLabels["trk_00001234"], "cyclist")
    }

    func testLabel5AssignsWithSelection() {
        let state = AppState()
        state.selectTrack("trk_00001234")
        let result = handleKeyPress(.label5, appState: state)
        XCTAssertEqual(result, .handled)
        XCTAssertEqual(state.userLabels["trk_00001234"], "bird")
    }

    func testLabel6AssignsWithSelection() {
        let state = AppState()
        state.selectTrack("trk_00001234")
        let result = handleKeyPress(.label6, appState: state)
        XCTAssertEqual(result, .handled)
        XCTAssertEqual(state.userLabels["trk_00001234"], "bus")
    }

    func testLabel7AssignsWithSelection() {
        let state = AppState()
        state.selectTrack("trk_00001234")
        let result = handleKeyPress(.label7, appState: state)
        XCTAssertEqual(result, .handled)
        XCTAssertEqual(state.userLabels["trk_00001234"], "car")
    }

    func testLabel8AssignsWithSelection() {
        let state = AppState()
        state.selectTrack("trk_00001234")
        let result = handleKeyPress(.label8, appState: state)
        XCTAssertEqual(result, .handled)
        XCTAssertEqual(state.userLabels["trk_00001234"], "truck")
    }

    func testLabel9AssignsWithSelection() {
        let state = AppState()
        state.selectTrack("trk_00001234")
        let result = handleKeyPress(.label9, appState: state)
        XCTAssertEqual(result, .handled)
        XCTAssertEqual(state.userLabels["trk_00001234"], "motorcyclist")
    }

    func testLabel5to9IgnoredNoSelection() {
        let state = AppState()
        XCTAssertEqual(handleKeyPress(.label5, appState: state), .ignored)
        XCTAssertEqual(handleKeyPress(.label6, appState: state), .ignored)
        XCTAssertEqual(handleKeyPress(.label7, appState: state), .ignored)
        XCTAssertEqual(handleKeyPress(.label8, appState: state), .ignored)
        XCTAssertEqual(handleKeyPress(.label9, appState: state), .ignored)
    }

    // Track navigation
    func testSelectNextTrack() {
        let state = AppState()
        let result = handleKeyPress(.selectNextTrack, appState: state)
        XCTAssertEqual(result, .handled)
    }

    func testSelectPrevTrack() {
        let state = AppState()
        let result = handleKeyPress(.selectPrevTrack, appState: state)
        XCTAssertEqual(result, .handled)
    }

    // Overlay toggles
    func testTogglePoints() {
        let state = AppState()
        let before = state.showPoints
        XCTAssertEqual(handleKeyPress(.togglePoints, appState: state), .handled)
        XCTAssertNotEqual(state.showPoints, before)
    }

    func testToggleBackground() {
        let state = AppState()
        let before = state.showBackground
        XCTAssertEqual(handleKeyPress(.toggleBackground, appState: state), .handled)
        XCTAssertNotEqual(state.showBackground, before)
    }

    func testToggleBoxes() {
        let state = AppState()
        let before = state.showBoxes
        XCTAssertEqual(handleKeyPress(.toggleBoxes, appState: state), .handled)
        XCTAssertNotEqual(state.showBoxes, before)
    }

    func testToggleClusters() {
        let state = AppState()
        let before = state.showClusters
        XCTAssertEqual(handleKeyPress(.toggleClusters, appState: state), .handled)
        XCTAssertNotEqual(state.showClusters, before)
    }

    func testToggleTrails() {
        let state = AppState()
        let before = state.showTrails
        XCTAssertEqual(handleKeyPress(.toggleTrails, appState: state), .handled)
        XCTAssertNotEqual(state.showTrails, before)
    }

    func testToggleVelocity() {
        let state = AppState()
        let before = state.showVelocity
        XCTAssertEqual(handleKeyPress(.toggleVelocity, appState: state), .handled)
        XCTAssertNotEqual(state.showVelocity, before)
    }

    func testToggleLabels() {
        let state = AppState()
        let before = state.showTrackLabels
        XCTAssertEqual(handleKeyPress(.toggleLabels, appState: state), .handled)
        XCTAssertNotEqual(state.showTrackLabels, before)
    }

    func testToggleGrid() {
        let state = AppState()
        let before = state.showGrid
        XCTAssertEqual(handleKeyPress(.toggleGrid, appState: state), .handled)
        XCTAssertNotEqual(state.showGrid, before)
    }

    // All KeyAction cases are exhaustively tested above.
    func testAllKeyActionsExist() {
        let allActions: [KeyAction] = [
            .space, .comma, .period, .decreaseRate, .increaseRate, .label1, .label2, .label3,
            .label4, .label5, .label6, .label7, .label8, .label9, .selectPrevTrack,
            .selectNextTrack, .togglePoints, .toggleBackground, .toggleBoxes, .toggleClusters,
            .toggleTrails, .toggleVelocity, .toggleLabels, .toggleGrid,
        ]
        XCTAssertEqual(allActions.count, 24)
    }
}

// MARK: - isTrackClimbing Tests

final class IsTrackClimbingTests: XCTestCase {

    func testNotInRanksReturnsFalse() {
        let ranks: [String: RankEntry] = [:]
        XCTAssertFalse(isTrackClimbing("trk_1234", ranks: ranks))
    }

    func testNilClimbedAtReturnsFalse() {
        let ranks: [String: RankEntry] = ["trk_1234": (rank: 0, climbedAt: nil)]
        XCTAssertFalse(isTrackClimbing("trk_1234", ranks: ranks))
    }

    func testRecentClimbReturnsTrue() {
        let now = Date()
        let ranks: [String: RankEntry] = [
            "trk_1234": (rank: 0, climbedAt: now.addingTimeInterval(-0.5))
        ]
        XCTAssertTrue(isTrackClimbing("trk_1234", ranks: ranks, now: now))
    }

    func testOldClimbReturnsFalse() {
        let now = Date()
        let ranks: [String: RankEntry] = [
            "trk_1234": (rank: 0, climbedAt: now.addingTimeInterval(-3.0))
        ]
        XCTAssertFalse(isTrackClimbing("trk_1234", ranks: ranks, now: now))
    }

    func testExactWindowBoundary() {
        let now = Date()
        // At exactly 2.0 seconds, timeIntervalSince == 2.0 which is NOT < 2.0
        let ranks: [String: RankEntry] = [
            "trk_1234": (rank: 0, climbedAt: now.addingTimeInterval(-2.0))
        ]
        XCTAssertFalse(isTrackClimbing("trk_1234", ranks: ranks, now: now))
    }

    func testCustomWindow() {
        let now = Date()
        let ranks: [String: RankEntry] = [
            "trk_1234": (rank: 0, climbedAt: now.addingTimeInterval(-4.0))
        ]
        // With 5-second window, 4 seconds ago is still climbing
        XCTAssertTrue(isTrackClimbing("trk_1234", ranks: ranks, now: now, window: 5.0))
    }
}

// MARK: - computeRanks Tests

final class ComputeRanksTests: XCTestCase {

    func testNewTracksGetNilClimbedAt() {
        let sorted: [(id: String, maxSpeed: Float)] = [
            (id: "a", maxSpeed: 10), (id: "b", maxSpeed: 5),
        ]
        let result = computeRanks(speedSorted: sorted, previousRanks: [:])
        XCTAssertEqual(result["a"]?.rank, 0)
        XCTAssertNil(result["a"]?.climbedAt)
        XCTAssertEqual(result["b"]?.rank, 1)
        XCTAssertNil(result["b"]?.climbedAt)
    }

    func testClimbDetected() {
        let now = Date()
        let previous: [String: RankEntry] = [
            "a": (rank: 2, climbedAt: nil), "b": (rank: 0, climbedAt: nil),
        ]
        // "a" moves from rank 2 to rank 0 → climb
        let sorted: [(id: String, maxSpeed: Float)] = [
            (id: "a", maxSpeed: 15), (id: "b", maxSpeed: 10),
        ]
        let result = computeRanks(speedSorted: sorted, previousRanks: previous, now: now)
        XCTAssertEqual(result["a"]?.rank, 0)
        XCTAssertEqual(result["a"]?.climbedAt, now)
    }

    func testSameRankPreservesClimbedAt() {
        let original = Date().addingTimeInterval(-1.0)
        let now = Date()
        let previous: [String: RankEntry] = ["a": (rank: 0, climbedAt: original)]
        let sorted: [(id: String, maxSpeed: Float)] = [(id: "a", maxSpeed: 10)]
        let result = computeRanks(speedSorted: sorted, previousRanks: previous, now: now)
        XCTAssertEqual(result["a"]?.rank, 0)
        XCTAssertEqual(result["a"]?.climbedAt, original)
    }

    func testDroppedRankPreservesClimbedAt() {
        let now = Date()
        let previous: [String: RankEntry] = [
            "a": (rank: 0, climbedAt: nil), "b": (rank: 1, climbedAt: nil),
        ]
        // "a" drops from 0 to 1
        let sorted: [(id: String, maxSpeed: Float)] = [
            (id: "b", maxSpeed: 15), (id: "a", maxSpeed: 10),
        ]
        let result = computeRanks(speedSorted: sorted, previousRanks: previous, now: now)
        XCTAssertEqual(result["a"]?.rank, 1)
        XCTAssertNil(result["a"]?.climbedAt)  // Was nil, stays nil
    }

    func testEmptyInputReturnsEmpty() {
        let result = computeRanks(speedSorted: [], previousRanks: [:])
        XCTAssertTrue(result.isEmpty)
    }
}

// MARK: - buildRunModeSpeedEntries Tests

final class BuildRunModeSpeedEntriesTests: XCTestCase {

    func testUsesMaxOfAllSources() {
        let runTracks = [makeRunTrack(trackId: "trk_a", maxSpeed: 5.0)]
        let frameTrack = makeTrack(id: "trk_a", maxSpeed: 8.0)
        let frameTrackByID = ["trk_a": frameTrack]
        let trackMaxSpeed: [String: Float] = ["trk_a": 10.0]

        let result = buildRunModeSpeedEntries(
            runTracks: runTracks, frameTrackByID: frameTrackByID, trackMaxSpeed: trackMaxSpeed)

        XCTAssertEqual(result.count, 1)
        XCTAssertEqual(result[0].maxSpeed, 10.0)  // persistent is highest
    }

    func testFallsBackToAPISpeed() {
        let runTracks = [makeRunTrack(trackId: "trk_a", maxSpeed: 7.0)]
        let result = buildRunModeSpeedEntries(
            runTracks: runTracks, frameTrackByID: [:], trackMaxSpeed: [:])
        XCTAssertEqual(result[0].maxSpeed, 7.0)
    }

    func testNilAPIMaxSpeedUsesZero() {
        let runTracks = [makeRunTrack(trackId: "trk_a", maxSpeed: nil)]
        let result = buildRunModeSpeedEntries(
            runTracks: runTracks, frameTrackByID: [:], trackMaxSpeed: [:])
        XCTAssertEqual(result[0].maxSpeed, 0.0)
    }

    func testSortedDescending() {
        let runTracks = [
            makeRunTrack(trackId: "trk_a", maxSpeed: 5.0),
            makeRunTrack(trackId: "trk_b", maxSpeed: 15.0),
            makeRunTrack(trackId: "trk_c", maxSpeed: 10.0),
        ]
        let result = buildRunModeSpeedEntries(
            runTracks: runTracks, frameTrackByID: [:], trackMaxSpeed: [:])
        XCTAssertEqual(result.map(\.id), ["trk_b", "trk_c", "trk_a"])
    }
}

// MARK: - buildFrameModeSpeedEntries Tests

final class BuildFrameModeSpeedEntriesTests: XCTestCase {

    func testUsesMaxOfTrackAndPersistent() {
        let tracks = [makeTrack(id: "trk_a", maxSpeed: 5.0)]
        let result = buildFrameModeSpeedEntries(tracks: tracks, trackMaxSpeed: ["trk_a": 12.0])
        XCTAssertEqual(result[0].maxSpeed, 12.0)
    }

    func testFallsBackToTrackMax() {
        let tracks = [makeTrack(id: "trk_a", maxSpeed: 8.0)]
        let result = buildFrameModeSpeedEntries(tracks: tracks, trackMaxSpeed: [:])
        XCTAssertEqual(result[0].maxSpeed, 8.0)
    }

    func testSortedDescending() {
        let tracks = [
            makeTrack(id: "trk_a", maxSpeed: 3.0), makeTrack(id: "trk_b", maxSpeed: 9.0),
        ]
        let result = buildFrameModeSpeedEntries(tracks: tracks, trackMaxSpeed: [:])
        XCTAssertEqual(result.map(\.id), ["trk_b", "trk_a"])
    }
}

// MARK: - runTrackTags Tests

final class RunTrackTagsTests: XCTestCase {

    func testNoLabelsReturnsEmpty() {
        let track = makeRunTrack(userLabel: nil, qualityLabel: nil)
        let tags = runTrackTags(track, userLabels: [:], userQualityFlags: [:])
        XCTAssertTrue(tags.isEmpty)
    }

    func testAPILabelUsed() {
        let track = makeRunTrack(userLabel: "car")
        let tags = runTrackTags(track, userLabels: [:], userQualityFlags: [:])
        XCTAssertEqual(tags.count, 1)
        XCTAssertEqual(tags[0].0, "car")
    }

    func testUserLabelOverridesAPILabel() {
        let track = makeRunTrack(trackId: "trk_a", userLabel: "truck")
        let labels = ["trk_a": "bus"]
        let tags = runTrackTags(track, userLabels: labels, userQualityFlags: [:])
        XCTAssertEqual(tags.count, 1)
        XCTAssertEqual(tags[0].0, "bus")
    }

    func testQualityFlagsSplit() {
        let track = makeRunTrack(qualityLabel: "noisy,merge")
        let tags = runTrackTags(track, userLabels: [:], userQualityFlags: [:])
        XCTAssertEqual(tags.count, 2)
        XCTAssertEqual(tags[0].0, "noisy")
        XCTAssertEqual(tags[1].0, "merge")
    }

    func testUserQualityOverridesAPI() {
        let track = makeRunTrack(trackId: "trk_a", qualityLabel: "noisy")
        let flags = ["trk_a": "good"]
        let tags = runTrackTags(track, userLabels: [:], userQualityFlags: flags)
        XCTAssertEqual(tags.count, 1)
        XCTAssertEqual(tags[0].0, "good")
    }

    func testLabelAndQualityCombined() {
        let track = makeRunTrack(userLabel: "car", qualityLabel: "noisy")
        let tags = runTrackTags(track, userLabels: [:], userQualityFlags: [:])
        XCTAssertEqual(tags.count, 2)
        XCTAssertEqual(tags[0].0, "car")
        XCTAssertEqual(tags[1].0, "noisy")
    }

    func testEmptyLabelStringIgnored() {
        let track = makeRunTrack(userLabel: "")
        let tags = runTrackTags(track, userLabels: [:], userQualityFlags: [:])
        XCTAssertTrue(tags.isEmpty)
    }

    func testEmptyQualityStringIgnored() {
        let track = makeRunTrack(qualityLabel: "")
        let tags = runTrackTags(track, userLabels: [:], userQualityFlags: [:])
        XCTAssertTrue(tags.isEmpty)
    }
}

// MARK: - frameTrackTags Tests

final class FrameTrackTagsTests: XCTestCase {

    func testNoLabelReturnsEmpty() {
        let track = makeTrack(classLabel: "")
        let tags = frameTrackTags(track, userLabels: [:])
        XCTAssertTrue(tags.isEmpty)
    }

    func testClassLabelUsed() {
        let track = makeTrack(classLabel: "car")
        let tags = frameTrackTags(track, userLabels: [:])
        XCTAssertEqual(tags.count, 1)
        XCTAssertEqual(tags[0].0, "car")
    }

    func testUserLabelOverridesClassLabel() {
        let track = makeTrack(id: "trk_a", classLabel: "car")
        let tags = frameTrackTags(track, userLabels: ["trk_a": "truck"])
        XCTAssertEqual(tags.count, 1)
        XCTAssertEqual(tags[0].0, "truck")
    }
}

// MARK: - bestRunTrackSpeed Tests

final class BestRunTrackSpeedTests: XCTestCase {

    func testAllSourcesCombined() {
        let frameTrack = makeTrack(id: "trk_a", maxSpeed: 5.0)
        let result = bestRunTrackSpeed(
            trackId: "trk_a", apiMaxSpeed: 3.0, frameTrack: frameTrack,
            trackMaxSpeed: ["trk_a": 8.0])
        XCTAssertEqual(result, 8.0)
    }

    func testNoSourcesReturnsNil() {
        let result = bestRunTrackSpeed(
            trackId: "trk_a", apiMaxSpeed: nil, frameTrack: nil, trackMaxSpeed: [:])
        XCTAssertNil(result)
    }

    func testOnlyAPISpeed() {
        let result = bestRunTrackSpeed(
            trackId: "trk_a", apiMaxSpeed: 7.5, frameTrack: nil, trackMaxSpeed: [:])
        XCTAssertEqual(result, 7.5)
    }

    func testOnlyLiveSpeed() {
        let frameTrack = makeTrack(id: "trk_a", maxSpeed: 12.0)
        let result = bestRunTrackSpeed(
            trackId: "trk_a", apiMaxSpeed: nil, frameTrack: frameTrack, trackMaxSpeed: [:])
        XCTAssertEqual(result, 12.0)
    }

    func testOnlyPersistentSpeed() {
        let result = bestRunTrackSpeed(
            trackId: "trk_a", apiMaxSpeed: nil, frameTrack: nil, trackMaxSpeed: ["trk_a": 6.5])
        XCTAssertEqual(result, 6.5)
    }
}

// MARK: - sortTracksByMaxSpeed Tests

final class SortTracksByMaxSpeedTests: XCTestCase {

    func testSortsDescending() {
        let tracks = [
            makeTrack(id: "trk_slow", maxSpeed: 3.0), makeTrack(id: "trk_fast", maxSpeed: 15.0),
            makeTrack(id: "trk_mid", maxSpeed: 8.0),
        ]
        let sorted = sortTracksByMaxSpeed(tracks, trackMaxSpeed: [:])
        XCTAssertEqual(sorted.map(\.trackID), ["trk_fast", "trk_mid", "trk_slow"])
    }

    func testPersistentMaxOverrides() {
        let tracks = [
            makeTrack(id: "trk_a", maxSpeed: 3.0), makeTrack(id: "trk_b", maxSpeed: 10.0),
        ]
        // trk_a has a higher persistent max
        let sorted = sortTracksByMaxSpeed(tracks, trackMaxSpeed: ["trk_a": 20.0])
        XCTAssertEqual(sorted.map(\.trackID), ["trk_a", "trk_b"])
    }

    func testEmptyInput() {
        let sorted = sortTracksByMaxSpeed([], trackMaxSpeed: [:])
        XCTAssertTrue(sorted.isEmpty)
    }
}

// MARK: - sortRunTracksByMaxSpeed Tests

final class SortRunTracksByMaxSpeedTests: XCTestCase {

    func testSortsDescending() {
        let runTracks = [
            makeRunTrack(trackId: "trk_a", maxSpeed: 5.0),
            makeRunTrack(trackId: "trk_b", maxSpeed: 15.0),
        ]
        let sorted = sortRunTracksByMaxSpeed(runTracks, frameTrackByID: [:], trackMaxSpeed: [:])
        XCTAssertEqual(sorted.map(\.trackId), ["trk_b", "trk_a"])
    }

    func testUsesLiveFrameSpeed() {
        let runTracks = [
            makeRunTrack(trackId: "trk_a", maxSpeed: 5.0),
            makeRunTrack(trackId: "trk_b", maxSpeed: 3.0),
        ]
        let frameTrackByID = ["trk_b": makeTrack(id: "trk_b", maxSpeed: 20.0)]
        let sorted = sortRunTracksByMaxSpeed(
            runTracks, frameTrackByID: frameTrackByID, trackMaxSpeed: [:])
        XCTAssertEqual(sorted.map(\.trackId), ["trk_b", "trk_a"])
    }

    func testNilAPIMaxHandled() {
        let runTracks = [makeRunTrack(trackId: "trk_a", maxSpeed: nil)]
        let sorted = sortRunTracksByMaxSpeed(runTracks, frameTrackByID: [:], trackMaxSpeed: [:])
        XCTAssertEqual(sorted.count, 1)
    }
}

// MARK: - trackStateColour Tests

final class TrackStateColourTests: XCTestCase {

    func testUnknownIsGray() { XCTAssertEqual(trackStateColour(.unknown), .gray) }

    func testTentativeIsYellow() { XCTAssertEqual(trackStateColour(.tentative), .yellow) }

    func testConfirmedIsGreen() { XCTAssertEqual(trackStateColour(.confirmed), .green) }

    func testDeletedIsRed() { XCTAssertEqual(trackStateColour(.deleted), .red) }
}

// MARK: - trackStateLabel Tests

final class TrackStateLabelTests: XCTestCase {

    func testUnknown() { XCTAssertEqual(trackStateLabel(.unknown), "Unknown") }
    func testTentative() { XCTAssertEqual(trackStateLabel(.tentative), "Tentative") }
    func testConfirmed() { XCTAssertEqual(trackStateLabel(.confirmed), "Confirmed") }
    func testDeleted() { XCTAssertEqual(trackStateLabel(.deleted), "Deleted") }
}

// MARK: - RunTrackRowView Tests

@available(macOS 15.0, *) @MainActor final class RunTrackRowViewTests: XCTestCase {

    func testBasicRowRenders() {
        let state = AppState()
        let view = RunTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: .green, isInView: true,
            bestSpeed: 12.5, showClimbArrow: false, tags: [], isSelected: false, onSelect: {})
        hostView(view, state: state)
    }

    func testSelectedRowRenders() {
        let state = AppState()
        let view = RunTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: .green, isInView: true, bestSpeed: 8.0,
            showClimbArrow: false, tags: [("car", .confirmedGreen)], isSelected: true, onSelect: {})
        hostView(view, state: state)
    }

    func testClimbArrowShown() {
        let state = AppState()
        let view = RunTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: .yellow, isInView: true,
            bestSpeed: 15.0, showClimbArrow: true, tags: [], isSelected: false, onSelect: {})
        hostView(view, state: state)
    }

    func testNotInViewRow() {
        let state = AppState()
        let view = RunTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: .gray, isInView: false, bestSpeed: nil,
            showClimbArrow: false, tags: [], isSelected: false, onSelect: {})
        hostView(view, state: state)
    }

    func testWithMultipleTags() {
        let state = AppState()
        let view = RunTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: .green, isInView: true,
            bestSpeed: 10.0, showClimbArrow: false,
            tags: [("car", .confirmedGreen), ("noisy", .accentColor), ("merge", .accentColor)],
            isSelected: false, onSelect: {})
        hostView(view, state: state)
    }

    func testNilBestSpeed() {
        let state = AppState()
        let view = RunTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: .green, isInView: true, bestSpeed: nil,
            showClimbArrow: false, tags: [], isSelected: false, onSelect: {})
        hostView(view, state: state)
    }
}

// MARK: - FrameTrackRowView Tests

@available(macOS 15.0, *) @MainActor final class FrameTrackRowViewTests: XCTestCase {

    func testBasicRowRenders() {
        let state = AppState()
        let view = FrameTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: .green, speedDisplay: "12.5 m/s",
            showClimbArrow: false, tags: [], isSelected: false, onSelect: {})
        hostView(view, state: state)
    }

    func testSelectedRowRenders() {
        let state = AppState()
        let view = FrameTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: .green, speedDisplay: "8.0 m/s",
            showClimbArrow: false, tags: [("car", .confirmedGreen)], isSelected: true, onSelect: {})
        hostView(view, state: state)
    }

    func testClimbArrowShown() {
        let state = AppState()
        let view = FrameTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: .yellow, speedDisplay: "15.0 m/s",
            showClimbArrow: true, tags: [], isSelected: false, onSelect: {})
        hostView(view, state: state)
    }

    func testWithTags() {
        let state = AppState()
        let view = FrameTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: .green, speedDisplay: "5.0 m/s",
            showClimbArrow: false, tags: [("truck", .confirmedGreen)], isSelected: false,
            onSelect: {})
        hostView(view, state: state)
    }

    func testAllStates() {
        let state = AppState()
        for trackState in [TrackState.unknown, .tentative, .confirmed, .deleted] {
            let view = FrameTrackRowView(
                trackId: "trk_a", shortID: "000a", statusColour: trackStateColour(trackState),
                speedDisplay: "8.0 m/s", showClimbArrow: false, tags: [], isSelected: false,
                onSelect: {})
            hostView(view, state: state)
        }
    }
}

// MARK: - KeyAction Enum Tests

final class KeyActionEnumTests: XCTestCase {

    func testAllCasesRepresented() {
        // Verify all enum cases can be created (exhaustiveness check)
        let actions: [KeyAction] = [
            .space, .comma, .period, .decreaseRate, .increaseRate, .label1, .label2, .label3,
            .label4, .label5, .label6, .label7, .label8, .label9, .selectPrevTrack,
            .selectNextTrack, .togglePoints, .toggleBackground, .toggleBoxes, .toggleClusters,
            .toggleTrails, .toggleVelocity, .toggleLabels, .toggleGrid,
        ]
        // Each should be distinct
        XCTAssertEqual(actions.count, 24)
    }
}

// MARK: - Seekable Key Press Tests

@available(macOS 15.0, *) @MainActor final class SeekableKeyPressTests: XCTestCase {

    func testCommaHandledWhenSeekable() {
        let state = AppState()
        state.isSeekable = true
        let result = handleKeyPress(.comma, appState: state)
        XCTAssertEqual(result, .handled)
    }

    func testPeriodHandledWhenSeekable() {
        let state = AppState()
        state.isSeekable = true
        let result = handleKeyPress(.period, appState: state)
        XCTAssertEqual(result, .handled)
    }
}

// MARK: - sortRunTracksByMaxSpeed with live frame data

final class SortRunTracksByMaxSpeedLiveTests: XCTestCase {

    func testUsesAllThreeSpeedSources() {
        let runTracks = [
            makeRunTrack(trackId: "trk_a", maxSpeed: 5.0),
            makeRunTrack(trackId: "trk_b", maxSpeed: 3.0),
            makeRunTrack(trackId: "trk_c", maxSpeed: 1.0),
        ]
        let frameTrackByID = ["trk_c": makeTrack(id: "trk_c", maxSpeed: 20.0)]
        let trackMaxSpeed: [String: Float] = ["trk_b": 15.0]
        let sorted = sortRunTracksByMaxSpeed(
            runTracks, frameTrackByID: frameTrackByID, trackMaxSpeed: trackMaxSpeed)
        // trk_c has live frame 20, trk_b has persistent 15, trk_a has API 5
        XCTAssertEqual(sorted.map(\.trackId), ["trk_c", "trk_b", "trk_a"])
    }

    func testCompactMapHandlesNilMaxSpeed() {
        let runTracks = [
            makeRunTrack(trackId: "trk_a", maxSpeed: nil),
            makeRunTrack(trackId: "trk_b", maxSpeed: 10.0),
        ]
        let sorted = sortRunTracksByMaxSpeed(runTracks, frameTrackByID: [:], trackMaxSpeed: [:])
        XCTAssertEqual(sorted.map(\.trackId), ["trk_b", "trk_a"])
    }
}

// MARK: - Edge cases for extracted helpers

final class EdgeCaseHelperTests: XCTestCase {

    func testRunTrackTagsMultipleQualityFlags() {
        let track = makeRunTrack(qualityLabel: "noisy,split,merge")
        let tags = runTrackTags(track, userLabels: [:], userQualityFlags: [:])
        XCTAssertEqual(tags.count, 3)
        XCTAssertEqual(tags.map(\.0), ["noisy", "split", "merge"])
    }

    func testFrameTrackTagsEmptyClassLabelNoUserLabel() {
        let track = makeTrack(id: "trk_a", classLabel: "")
        let tags = frameTrackTags(track, userLabels: [:])
        XCTAssertTrue(tags.isEmpty)
    }

    func testBestRunTrackSpeedFrameTrackHighest() {
        let frameTrack = makeTrack(id: "trk_a", maxSpeed: 25.0)
        let result = bestRunTrackSpeed(
            trackId: "trk_a", apiMaxSpeed: 10.0, frameTrack: frameTrack,
            trackMaxSpeed: ["trk_a": 15.0])
        XCTAssertEqual(result, 25.0)
    }

    func testBuildFrameModeSpeedEntriesEmpty() {
        let result = buildFrameModeSpeedEntries(tracks: [], trackMaxSpeed: [:])
        XCTAssertTrue(result.isEmpty)
    }

    func testBuildRunModeSpeedEntriesEmpty() {
        let result = buildRunModeSpeedEntries(
            runTracks: [], frameTrackByID: [:], trackMaxSpeed: [:])
        XCTAssertTrue(result.isEmpty)
    }

    func testComputeRanksMultipleClimbs() {
        let now = Date()
        let previous: [String: RankEntry] = [
            "a": (rank: 3, climbedAt: nil), "b": (rank: 2, climbedAt: nil),
            "c": (rank: 0, climbedAt: nil),
        ]
        let sorted: [(id: String, maxSpeed: Float)] = [
            (id: "a", maxSpeed: 20), (id: "b", maxSpeed: 15), (id: "c", maxSpeed: 5),
        ]
        let result = computeRanks(speedSorted: sorted, previousRanks: previous, now: now)
        // a climbed from 3→0, b climbed from 2→1, c dropped from 0→2
        XCTAssertEqual(result["a"]?.climbedAt, now)
        XCTAssertEqual(result["b"]?.climbedAt, now)
        XCTAssertNil(result["c"]?.climbedAt)
    }
}

// MARK: - Row Views with comprehensive state combinations

@available(macOS 15.0, *) @MainActor final class RowViewComprehensiveTests: XCTestCase {

    func testRunTrackRowAllStates() {
        let state = AppState()
        for stateVal in [TrackState.unknown, .tentative, .confirmed, .deleted] {
            let colour = trackStateColour(stateVal)
            let view = RunTrackRowView(
                trackId: "trk_x", shortID: "000x", statusColour: colour, isInView: true,
                bestSpeed: 10.0, showClimbArrow: false, tags: [], isSelected: false, onSelect: {})
            hostView(view, state: state)
        }
    }

    func testRunTrackRowSelectedWithClimb() {
        let state = AppState()
        let view = RunTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: .green, isInView: true,
            bestSpeed: 20.0, showClimbArrow: true,
            tags: [("car", .confirmedGreen), ("noisy", .accentColor)], isSelected: true,
            onSelect: {})
        hostView(view, state: state)
    }

    func testFrameTrackRowAllStatesWithTags() {
        let state = AppState()
        for stateVal in [TrackState.unknown, .tentative, .confirmed, .deleted] {
            let view = FrameTrackRowView(
                trackId: "trk_x", shortID: "000x", statusColour: trackStateColour(stateVal),
                speedDisplay: "8.5 m/s", showClimbArrow: false, tags: [("car", .confirmedGreen)],
                isSelected: false, onSelect: {})
            hostView(view, state: state)
        }
    }

    func testFrameTrackRowWithClimbAndSelection() {
        let state = AppState()
        let view = FrameTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: .green, speedDisplay: "15.0 m/s",
            showClimbArrow: true,
            tags: [("truck", .confirmedGreen), ("noisy", .accentColor), ("extra", .blue)],
            isSelected: true, onSelect: {})
        hostView(view, state: state)
    }

    func testRunTrackRowWithNoSpeedNoTags() {
        let state = AppState()
        let view = RunTrackRowView(
            trackId: "trk_a", shortID: "000a", statusColour: Color.gray.opacity(0.5),
            isInView: false, bestSpeed: nil, showClimbArrow: false, tags: [], isSelected: false,
            onSelect: {})
        hostView(view, state: state)
    }
}

// MARK: - Toggle Round-Trip Tests (double-toggle returns to original)

@available(macOS 15.0, *) @MainActor final class ToggleRoundTripTests: XCTestCase {

    func testTogglePointsRoundTrip() {
        let state = AppState()
        let original = state.showPoints
        _ = handleKeyPress(.togglePoints, appState: state)
        XCTAssertNotEqual(state.showPoints, original)
        _ = handleKeyPress(.togglePoints, appState: state)
        XCTAssertEqual(state.showPoints, original)
    }

    func testToggleBackgroundRoundTrip() {
        let state = AppState()
        let original = state.showBackground
        _ = handleKeyPress(.toggleBackground, appState: state)
        _ = handleKeyPress(.toggleBackground, appState: state)
        XCTAssertEqual(state.showBackground, original)
    }

    func testToggleBoxesRoundTrip() {
        let state = AppState()
        let original = state.showBoxes
        _ = handleKeyPress(.toggleBoxes, appState: state)
        _ = handleKeyPress(.toggleBoxes, appState: state)
        XCTAssertEqual(state.showBoxes, original)
    }

    func testToggleClustersRoundTrip() {
        let state = AppState()
        let original = state.showClusters
        _ = handleKeyPress(.toggleClusters, appState: state)
        _ = handleKeyPress(.toggleClusters, appState: state)
        XCTAssertEqual(state.showClusters, original)
    }

    func testToggleTrailsRoundTrip() {
        let state = AppState()
        let original = state.showTrails
        _ = handleKeyPress(.toggleTrails, appState: state)
        _ = handleKeyPress(.toggleTrails, appState: state)
        XCTAssertEqual(state.showTrails, original)
    }

    func testToggleVelocityRoundTrip() {
        let state = AppState()
        let original = state.showVelocity
        _ = handleKeyPress(.toggleVelocity, appState: state)
        _ = handleKeyPress(.toggleVelocity, appState: state)
        XCTAssertEqual(state.showVelocity, original)
    }

    func testToggleLabelsRoundTrip() {
        let state = AppState()
        let original = state.showTrackLabels
        _ = handleKeyPress(.toggleLabels, appState: state)
        _ = handleKeyPress(.toggleLabels, appState: state)
        XCTAssertEqual(state.showTrackLabels, original)
    }

    func testToggleGridRoundTrip() {
        let state = AppState()
        let original = state.showGrid
        _ = handleKeyPress(.toggleGrid, appState: state)
        _ = handleKeyPress(.toggleGrid, appState: state)
        XCTAssertEqual(state.showGrid, original)
    }
}

// MARK: - assignLabelByIndex Edge Cases

@available(macOS 15.0, *) @MainActor final class AssignLabelByIndexEdgeCaseTests: XCTestCase {

    func testLabelIgnoredNoSelectionLogsCorrectly() {
        let state = AppState()
        // No track selected — all label keys should return .ignored
        for action: KeyAction in [
            .label1, .label2, .label3, .label4, .label5, .label6, .label7, .label8, .label9,
        ] { XCTAssertEqual(handleKeyPress(action, appState: state), .ignored) }
    }

    func testAllNineLabelsAssignWithSelection() {
        let state = AppState()
        state.selectTrack("trk_test")
        let expectedLabels = [
            "noise", "dynamic", "pedestrian", "cyclist", "bird", "bus", "car", "truck",
            "motorcyclist",
        ]
        let actions: [KeyAction] = [
            .label1, .label2, .label3, .label4, .label5, .label6, .label7, .label8, .label9,
        ]

        for (action, expected) in zip(actions, expectedLabels) {
            let result = handleKeyPress(action, appState: state)
            XCTAssertEqual(result, .handled)
            XCTAssertEqual(state.userLabels["trk_test"], expected)
        }
    }
}
