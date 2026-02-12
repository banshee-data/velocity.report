//
//  CoverageBoostTests.swift
//  VelocityVisualiserTests
//
//  Additional unit tests to improve coverage for AppState, VisualiserClient,
//  ContentView, and RunBrowserView.
//

import Foundation
import SwiftUI
import Testing
import XCTest

@testable import VelocityVisualiser

// MARK: - VisualiserClient decodeFrameBundle Tests

struct DecodeFrameBundleTests {
    @Test func decodeEmptyBundle() throws {
        let client = VisualiserClient()
        let proto = Velocity_Visualiser_V1_FrameBundle()
        let result = client.decodeFrameBundle(proto)
        #expect(result.frameID == 0)
        #expect(result.timestampNanos == 0)
        #expect(result.sensorID == "")
        #expect(result.pointCloud == nil)
        #expect(result.clusters == nil)
        #expect(result.tracks == nil)
        #expect(result.playbackInfo == nil)
        #expect(result.debug == nil)
    }

    @Test func decodeBundleWithPlaybackInfo() throws {
        let client = VisualiserClient()
        var proto = Velocity_Visualiser_V1_FrameBundle()
        proto.frameID = 42
        proto.timestampNs = 1_000_000_000
        proto.sensorID = "hesai-01"
        proto.playbackInfo.isLive = true
        proto.playbackInfo.playbackRate = 2.0
        proto.playbackInfo.totalFrames = 1200
        proto.playbackInfo.seekable = true

        let result = client.decodeFrameBundle(proto)
        #expect(result.frameID == 42)
        #expect(result.timestampNanos == 1_000_000_000)
        #expect(result.sensorID == "hesai-01")
        #expect(result.playbackInfo?.isLive == true)
        #expect(result.playbackInfo?.playbackRate == 2.0)
        #expect(result.playbackInfo?.totalFrames == 1200)
    }

    @Test func decodeBundleWithCoordinateFrame() throws {
        let client = VisualiserClient()
        var proto = Velocity_Visualiser_V1_FrameBundle()
        proto.coordinateFrame.frameID = "enu"
        proto.coordinateFrame.referenceFrame = "wgs84"
        proto.coordinateFrame.originLat = 37.7749
        proto.coordinateFrame.originLon = -122.4194
        proto.coordinateFrame.originAlt = 10.0
        proto.coordinateFrame.rotationDeg = 45.0

        let result = client.decodeFrameBundle(proto)
        #expect(result.coordinateFrame?.frameID == "enu")
        #expect(result.coordinateFrame?.originLat == 37.7749)
    }

    @Test func decodeBundleWithPointCloud() throws {
        let client = VisualiserClient()
        var proto = Velocity_Visualiser_V1_FrameBundle()
        proto.frameID = 10
        proto.pointCloud.frameID = 10
        proto.pointCloud.timestampNs = 100
        proto.pointCloud.sensorID = "hesai-01"
        proto.pointCloud.x = [1.0, 2.0, 3.0]
        proto.pointCloud.y = [4.0, 5.0, 6.0]
        proto.pointCloud.z = [7.0, 8.0, 9.0]
        proto.pointCloud.pointCount = 3

        let result = client.decodeFrameBundle(proto)
        #expect(result.pointCloud != nil)
        #expect(result.pointCloud?.pointCount == 3)
    }

    @Test func decodeBundleWithClusters() throws {
        let client = VisualiserClient()
        var proto = Velocity_Visualiser_V1_FrameBundle()
        var cluster = Velocity_Visualiser_V1_Cluster()
        cluster.clusterID = "c1"
        cluster.centroidX = 1.0
        cluster.centroidY = 2.0
        cluster.centroidZ = 3.0
        cluster.pointsCount = 10
        proto.clusters.clusters.append(cluster)
        proto.clusters.frameID = 1

        let result = client.decodeFrameBundle(proto)
        #expect(result.clusters != nil)
        #expect(result.clusters?.clusters.count == 1)
    }

    @Test func decodeBundleWithTracks() throws {
        let client = VisualiserClient()
        var proto = Velocity_Visualiser_V1_FrameBundle()
        var track = Velocity_Visualiser_V1_Track()
        track.trackID = "t1"
        track.sensorID = "hesai-01"
        track.hits = 10
        track.speedMps = 8.5
        track.headingRad = 1.57
        proto.tracks.tracks.append(track)
        proto.tracks.frameID = 1

        let result = client.decodeFrameBundle(proto)
        #expect(result.tracks != nil)
        #expect(result.tracks?.tracks.count == 1)
        #expect(result.tracks?.tracks[0].speedMps == 8.5)
    }
}

// MARK: - ContentView Simple Component Tests

struct CacheStatusLabelCoverageTests {
    @Test func cachedStatus() throws {
        let view = CacheStatusLabel(status: "Cached (1200 frames)")
        let _ = view.body
    }

    @Test func refreshingStatus() throws {
        let view = CacheStatusLabel(status: "Refreshing...")
        let _ = view.body
    }

    @Test func emptyStatus() throws {
        let view = CacheStatusLabel(status: "")
        let _ = view.body
    }
}

struct DetailRowCoverageTests {
    @Test func basicRow() throws {
        let view = DetailRow(label: "Speed", value: "30.5 km/h")
        let _ = view.body
    }

    @Test func emptyValues() throws {
        let view = DetailRow(label: "", value: "")
        let _ = view.body
    }
}

// MARK: - AppState Additional Tests

@available(macOS 15.0, *) @MainActor final class AppStateAdditionalTests: XCTestCase {

    func testSetSliderEditing() throws {
        let state = AppState()
        XCTAssertFalse(state.isSeekingInProgress)

        state.setSliderEditing(true)
        XCTAssertTrue(state.isSeekingInProgress)

        state.setSliderEditing(false)
        XCTAssertFalse(state.isSeekingInProgress)
    }

    func testMarkAsSplitNoTrack() throws {
        let state = AppState()
        state.selectedTrackID = nil
        state.markAsSplit(true)
    }

    func testMarkAsSplitWithTrack() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"
        state.currentRunID = "run-001"
        state.markAsSplit(true)
    }

    func testMarkAsMergeNoTrack() throws {
        let state = AppState()
        state.selectedTrackID = nil
        state.markAsMerge(true)
    }

    func testMarkAsMergeWithTrack() throws {
        let state = AppState()
        state.selectedTrackID = "track-001"
        state.currentRunID = "run-001"
        state.markAsMerge(true)
    }

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

        state.toggleDebug()
        XCTAssertFalse(state.showDebug)
    }

    func testSendOverlayPreferencesNoClient() throws {
        let state = AppState()
        state.sendOverlayPreferences()
    }

    func testDecreaseRateClamps() throws {
        let state = AppState()
        state.isLive = false
        state.playbackRate = 0.5

        state.decreaseRate()
        XCTAssertEqual(state.playbackRate, 0.25)

        state.decreaseRate()
        XCTAssertEqual(state.playbackRate, 0.125)
    }

    func testResetRate() throws {
        let state = AppState()
        state.isLive = false
        state.playbackRate = 8.0

        state.resetRate()
        XCTAssertEqual(state.playbackRate, 1.0)
    }
}

// MARK: - VisualiserClient Config Tests

struct VisualiserClientConfigTests {
    @Test func defaultConfigOptions() throws {
        let client = VisualiserClient()
        #expect(client.includePoints == true)
        #expect(client.includeClusters == true)
        #expect(client.includeTracks == true)
        #expect(client.includeDebug == false)
    }

    @Test func customDecimation() throws {
        let client = VisualiserClient()
        client.decimationMode = .spatial
        client.decimationRatio = 0.5
        #expect(client.decimationMode == .spatial)
        #expect(client.decimationRatio == 0.5)
    }

    @Test func toggleDebugConfig() throws {
        let client = VisualiserClient()
        client.includeDebug = true
        #expect(client.includeDebug == true)
        client.includeDebug = false
        #expect(client.includeDebug == false)
    }

    @Test func disconnectWhenNotConnected() throws {
        let client = VisualiserClient()
        client.disconnect()
    }
}
