//
//  RunBrowserViewTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for RunBrowserView components: StatusDot, String.truncated,
//  RunRowView layout, and AnalysisRun computed properties.
//

import Foundation
import SwiftUI
import Testing
import XCTest

@testable import VelocityVisualiser

// MARK: - StatusDot Tests

struct StatusDotTests {
    @Test func completedStatus() throws {
        let dot = StatusDot(status: "completed")
        let _ = dot.body
    }

    @Test func runningStatus() throws {
        let dot = StatusDot(status: "running")
        let _ = dot.body
    }

    @Test func failedStatus() throws {
        let dot = StatusDot(status: "failed")
        let _ = dot.body
    }

    @Test func unknownStatus() throws {
        let dot = StatusDot(status: "something_else")
        let _ = dot.body
    }

    @Test func emptyStatus() throws {
        let dot = StatusDot(status: "")
        let _ = dot.body
    }
}

// MARK: - AnalysisRun Computed Properties Tests

struct AnalysisRunComputedPropertyTests {
    @Test func hasVRLogTrue() throws {
        let run = makeRun(vrlogPath: "/data/test.vrlog")
        #expect(run.hasVRLog == true)
    }

    @Test func hasVRLogFalseNil() throws {
        let run = makeRun(vrlogPath: nil)
        #expect(run.hasVRLog == false)
    }

    @Test func hasVRLogFalseEmpty() throws {
        let run = makeRun(vrlogPath: "")
        #expect(run.hasVRLog == false)
    }

    @Test func formattedDateNonEmpty() throws {
        let run = makeRun()
        #expect(!run.formattedDate.isEmpty)
    }

    @Test func identifiableID() throws {
        let run = makeRun()
        #expect(run.id == "run-test")
    }

    @Test func statusCompleted() throws {
        let run = makeRun(status: "completed")
        #expect(run.status == "completed")
    }

    @Test func statusFailed() throws {
        let run = makeRun(status: "failed")
        #expect(run.status == "failed")
    }

    @Test func errorMessage() throws {
        let run = makeRun(errorMessage: "Something went wrong")
        #expect(run.errorMessage == "Something went wrong")
    }

    @Test func notesNil() throws {
        let run = makeRun(notes: nil)
        #expect(run.notes == nil)
    }

    @Test func notesNonNil() throws {
        let run = makeRun(notes: "Test run with short recording")
        #expect(run.notes == "Test run with short recording")
    }

    private func makeRun(
        vrlogPath: String? = nil,
        status: String = "completed",
        errorMessage: String? = nil,
        notes: String? = nil
    ) -> AnalysisRun {
        AnalysisRun(
            runId: "run-test", createdAt: Date(), sourceType: "vrlog",
            sourcePath: "/data/test.vrlog", sensorId: "hesai-01", durationSecs: 120.0,
            totalFrames: 1200, totalClusters: 500, totalTracks: 25, confirmedTracks: 20,
            status: status, errorMessage: errorMessage, vrlogPath: vrlogPath, notes: notes)
    }
}

// MARK: - RunTrack Computed Properties Tests

struct RunTrackComputedPropertyTests {
    @Test func isLabelledWithUserLabel() throws {
        let track = makeTrack(userLabel: "good_vehicle")
        #expect(track.isLabelled == true)
    }

    @Test func isNotLabelledWhenNil() throws {
        let track = makeTrack(userLabel: nil)
        #expect(track.isLabelled == false)
    }

    @Test func isNotLabelledWhenEmpty() throws {
        let track = makeTrack(userLabel: "")
        #expect(track.isLabelled == false)
    }

    @Test func identifiableIDIsTrackId() throws {
        let track = makeTrack()
        #expect(track.id == "track-001")
    }

    @Test func splitCandidate() throws {
        let track = RunTrack(
            runId: "run-001", trackId: "track-001", sensorId: "hesai-01",
            userLabel: nil, qualityLabel: nil, labelConfidence: nil,
            labelerId: nil, startUnixNanos: nil, endUnixNanos: nil, totalObservations: nil,
            durationSecs: nil, avgSpeedMps: nil, peakSpeedMps: nil,
            isSplitCandidate: true, isMergeCandidate: false)
        #expect(track.isSplitCandidate == true)
        #expect(track.isMergeCandidate == false)
    }

    @Test func mergeCandidate() throws {
        let track = RunTrack(
            runId: "run-001", trackId: "track-001", sensorId: "hesai-01",
            userLabel: nil, qualityLabel: nil, labelConfidence: nil,
            labelerId: nil, startUnixNanos: nil, endUnixNanos: nil, totalObservations: nil,
            durationSecs: nil, avgSpeedMps: nil, peakSpeedMps: nil,
            isSplitCandidate: false, isMergeCandidate: true)
        #expect(track.isSplitCandidate == false)
        #expect(track.isMergeCandidate == true)
    }

    @Test func allOptionalFieldsPresent() throws {
        let track = RunTrack(
            runId: "run-001", trackId: "track-001", sensorId: "hesai-01",
            userLabel: "car", qualityLabel: "perfect", labelConfidence: 0.95,
            labelerId: "david", startUnixNanos: 1_000_000_000, endUnixNanos: 5_000_000_000,
            totalObservations: 40, durationSecs: 4.0, avgSpeedMps: 8.5, peakSpeedMps: 12.0,
            isSplitCandidate: false, isMergeCandidate: false)
        #expect(track.qualityLabel == "perfect")
        #expect(track.labelConfidence == 0.95)
        #expect(track.durationSecs == 4.0)
        #expect(track.avgSpeedMps == 8.5)
        #expect(track.peakSpeedMps == 12.0)
    }

    private func makeTrack(userLabel: String? = nil) -> RunTrack {
        RunTrack(
            runId: "run-001", trackId: "track-001", sensorId: "hesai-01",
            userLabel: userLabel, qualityLabel: nil, labelConfidence: nil,
            labelerId: nil, startUnixNanos: nil, endUnixNanos: nil, totalObservations: nil,
            durationSecs: nil, avgSpeedMps: nil, peakSpeedMps: nil, isSplitCandidate: nil,
            isMergeCandidate: nil)
    }
}

// MARK: - LabellingProgress Model Tests

struct LabellingProgressModelTests {
    @Test func fullProgress() throws {
        let progress = LabellingProgress(
            runId: "run-001", total: 50, labelled: 50,
            byClass: ["good_vehicle": 30, "noise": 20], progressPct: 100.0)
        #expect(progress.progressPct == 100.0)
        #expect(progress.total == progress.labelled)
    }

    @Test func zeroProgress() throws {
        let progress = LabellingProgress(
            runId: "run-001", total: 50, labelled: 0, byClass: nil, progressPct: 0.0)
        #expect(progress.progressPct == 0.0)
        #expect(progress.byClass == nil)
    }

    @Test func partialProgress() throws {
        let progress = LabellingProgress(
            runId: "run-001", total: 100, labelled: 60,
            byClass: ["good_vehicle": 40, "noise": 15, "static_object": 5], progressPct: 60.0)
        #expect(progress.labelled == 60)
        #expect(progress.byClass?.count == 3)
    }
}

// MARK: - PlaybackStatus Model Tests

struct PlaybackStatusModelTests {
    @Test func liveMode() throws {
        let status = PlaybackStatus(
            mode: "live", paused: false, rate: 1.0, seekable: false,
            currentFrame: 0, totalFrames: 0, timestampNs: 0, logStartNs: 0, logEndNs: 0,
            vrlogPath: nil)
        #expect(status.mode == "live")
        #expect(status.seekable == false)
        #expect(status.vrlogPath == nil)
    }

    @Test func replayMode() throws {
        let status = PlaybackStatus(
            mode: "replay", paused: true, rate: 4.0, seekable: true,
            currentFrame: 500, totalFrames: 1200, timestampNs: 5_000_000_000,
            logStartNs: 1_000_000_000, logEndNs: 12_000_000_000,
            vrlogPath: "/data/test.vrlog")
        #expect(status.mode == "replay")
        #expect(status.paused == true)
        #expect(status.rate == 4.0)
        #expect(status.totalFrames == 1200)
        #expect(status.vrlogPath == "/data/test.vrlog")
    }
}

// MARK: - RunRowView View Tests

@available(macOS 15.0, *) final class RunRowViewTests: XCTestCase {

    func testRunRowViewWithVRLog() throws {
        let run = AnalysisRun(
            runId: "run-001", createdAt: Date(), sourceType: "vrlog",
            sourcePath: "/data/test.vrlog", sensorId: "hesai-01", durationSecs: 120.0,
            totalFrames: 1200, totalClusters: 500, totalTracks: 25, confirmedTracks: 20,
            status: "completed", errorMessage: nil, vrlogPath: "/data/test.vrlog", notes: nil)

        let view = RunRowView(run: run, isSelected: false, onSelect: {})
        let _ = view.body
    }

    func testRunRowViewWithoutVRLog() throws {
        let run = AnalysisRun(
            runId: "run-002", createdAt: Date(), sourceType: "live",
            sourcePath: nil, sensorId: "hesai-01", durationSecs: 60.0,
            totalFrames: 600, totalClusters: 200, totalTracks: 10, confirmedTracks: 8,
            status: "running", errorMessage: nil, vrlogPath: nil, notes: nil)

        let view = RunRowView(run: run, isSelected: false, onSelect: {})
        let _ = view.body
    }

    func testRunRowViewSelected() throws {
        let run = AnalysisRun(
            runId: "run-001", createdAt: Date(), sourceType: "vrlog",
            sourcePath: nil, sensorId: "hesai-01", durationSecs: 120.0,
            totalFrames: 1200, totalClusters: 500, totalTracks: 25, confirmedTracks: 20,
            status: "completed", errorMessage: nil, vrlogPath: "/data/test.vrlog", notes: nil)

        let view = RunRowView(run: run, isSelected: true, onSelect: {})
        let _ = view.body
    }

    func testRunRowViewFailedStatus() throws {
        let run = AnalysisRun(
            runId: "run-003", createdAt: Date(), sourceType: "vrlog",
            sourcePath: nil, sensorId: "hesai-01", durationSecs: 5.0,
            totalFrames: 50, totalClusters: 10, totalTracks: 0, confirmedTracks: 0,
            status: "failed", errorMessage: "Sensor disconnected", vrlogPath: nil, notes: nil)

        let view = RunRowView(run: run, isSelected: false, onSelect: {})
        let _ = view.body
    }
}
