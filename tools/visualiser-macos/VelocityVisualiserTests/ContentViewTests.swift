//
//  ContentViewTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for ContentView helper functions, simple views,
//  and extracted utility logic.
//

import Foundation
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

    @Test func fractionalRate() throws {
        #expect(formatRate(0.5) == "0.5")
    }

    @Test func largeIntegerRate() throws {
        #expect(formatRate(100.0) == "100")
    }
}

// MARK: - Format Duration Tests

struct FormatDurationTests {
    @Test func zeroNanos() throws {
        #expect(formatDuration(0) == "0:00")
    }

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

@available(macOS 15.0, *) struct ModeIndicatorViewTests {
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

@available(macOS 15.0, *) struct StatLabelTests {
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

@available(macOS 15.0, *) struct CacheStatusLabelTests {
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

@available(macOS 15.0, *) struct DetailRowTests {
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

@available(macOS 15.0, *) struct ToggleButtonTests {
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

@available(macOS 15.0, *) struct LabelButtonTests {
    @Test func labelButtonWithShortcut() throws {
        let button = LabelButton(
            label: "good_vehicle", shortcut: "1", isActive: false, action: {})
        let _ = button.body
    }

    @Test func labelButtonWithoutShortcut() throws {
        let button = LabelButton(label: "stopped_recovered", shortcut: nil, isActive: false, action: {})
        let _ = button.body
    }

    @Test func labelButtonActive() throws {
        let button = LabelButton(label: "car", shortcut: "2", isActive: true, action: {})
        let _ = button.body
    }
}

// MARK: - TrackLabelPill Tests

@available(macOS 15.0, *) struct TrackLabelPillTests {
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

@available(macOS 15.0, *) struct TrackLabelOverlayTests {
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
            showPoints: true, showBoxes: true, showClusters: true, showTrails: true,
            showDebug: false, pointSize: 5.0)
        let coordinator = rep.makeCoordinator()
        XCTAssertNil(coordinator.renderer)
    }

    func testDefaultProperties() throws {
        let rep = MetalViewRepresentable(
            showPoints: true, showBoxes: true, showClusters: true, showTrails: true,
            showDebug: false, pointSize: 5.0)

        XCTAssertTrue(rep.showPoints)
        XCTAssertTrue(rep.showBoxes)
        XCTAssertTrue(rep.showClusters)
        XCTAssertTrue(rep.showTrails)
        XCTAssertFalse(rep.showDebug)
        XCTAssertEqual(rep.pointSize, 5.0)
    }

    func testCustomProperties() throws {
        let rep = MetalViewRepresentable(
            showPoints: false, showBoxes: false, showClusters: false, showTrails: false,
            showDebug: true, pointSize: 15.0)

        XCTAssertFalse(rep.showPoints)
        XCTAssertFalse(rep.showBoxes)
        XCTAssertFalse(rep.showClusters)
        XCTAssertFalse(rep.showTrails)
        XCTAssertTrue(rep.showDebug)
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

// MARK: - String Truncation Tests (from RunBrowserView extension)

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
        #expect(result.count <= 15)  // 12 + "..."
        #expect(result.hasSuffix("..."))
    }
}
