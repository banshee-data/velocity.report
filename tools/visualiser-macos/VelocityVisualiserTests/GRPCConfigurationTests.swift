//
//  GRPCConfigurationTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for gRPC configuration and decimation modes.
//

import Foundation
import Testing

@testable import VelocityVisualiser

// MARK: - Decimation Mode Configuration Tests

struct DecimationModeConfigurationTests {
    @Test func decimationModeNoneRawValue() throws { #expect(DecimationMode.none.rawValue == 0) }

    @Test func decimationModeUniformRawValue() throws {
        #expect(DecimationMode.uniform.rawValue == 1)
    }

    @Test func decimationModeVoxelRawValue() throws { #expect(DecimationMode.voxel.rawValue == 2) }

    @Test func decimationModeForegroundOnlyRawValue() throws {
        #expect(DecimationMode.foregroundOnly.rawValue == 3)
    }

    // Note: decimationModeFromRawValue tests are in VelocityVisualiserTests.swift
}

// MARK: - Track Alpha Rendering Tests

struct TrackAlphaRenderingTests {
    @Test func trackDefaultAlpha() throws {
        let track = Track()
        #expect(track.alpha == 1.0)
    }

    @Test func trackWithFadeOutAlpha() throws {
        var track = Track()
        track.alpha = 0.5
        #expect(track.alpha == 0.5)
    }

    @Test func trackWithZeroAlpha() throws {
        var track = Track()
        track.alpha = 0.0
        #expect(track.alpha == 0.0)
    }

    @Test func trackAlphaRange() throws {
        var track = Track()

        // Full opacity
        track.alpha = 1.0
        #expect(track.alpha == 1.0)

        // Partial opacity
        track.alpha = 0.75
        #expect(track.alpha == 0.75)

        // Low opacity
        track.alpha = 0.25
        #expect(track.alpha == 0.25)

        // Transparent
        track.alpha = 0.0
        #expect(track.alpha == 0.0)
    }
}

// MARK: - Track State Colour Tests

struct TrackStateColourExtendedTests {
    @Test func unknownStateColourIsGrey() throws {
        let colour = TrackState.unknown.colour
        #expect(colour.r == 0.5)
        #expect(colour.g == 0.5)
        #expect(colour.b == 0.5)
    }

    @Test func tentativeStateColourIsYellow() throws {
        let colour = TrackState.tentative.colour
        #expect(colour.r == 1.0)
        #expect(colour.g == 1.0)
        #expect(colour.b == 0.0)
    }

    @Test func confirmedStateColourIsGreen() throws {
        let colour = TrackState.confirmed.colour
        #expect(colour.r == 0.0)
        #expect(colour.g == 1.0)
        #expect(colour.b == 0.0)
    }

    @Test func deletedStateColourIsRed() throws {
        let colour = TrackState.deleted.colour
        #expect(colour.r == 1.0)
        #expect(colour.g == 0.0)
        #expect(colour.b == 0.0)
    }

    @Test func trackStateFromValidRawValues() throws {
        #expect(TrackState(rawValue: 0) == .unknown)
        #expect(TrackState(rawValue: 1) == .tentative)
        #expect(TrackState(rawValue: 2) == .confirmed)
        #expect(TrackState(rawValue: 3) == .deleted)
    }

    @Test func trackStateFromInvalidRawValue() throws {
        #expect(TrackState(rawValue: 4) == nil)
        #expect(TrackState(rawValue: -1) == nil)
    }
}

// MARK: - Oriented Bounding Box Validation Tests

struct OrientedBoundingBoxValidationTests {
    @Test func obbWithPositiveDimensions() throws {
        let obb = OrientedBoundingBox(
            centerX: 10.0, centerY: 20.0, centerZ: 1.0, length: 5.0, width: 2.0, height: 1.5,
            headingRad: 0.0)

        #expect(obb.length > 0)
        #expect(obb.width > 0)
        #expect(obb.height > 0)
    }

    @Test func obbHeadingRangeNegativePi() throws {
        var obb = OrientedBoundingBox()
        obb.headingRad = -Float.pi

        #expect(obb.headingRad == -Float.pi)
    }

    @Test func obbHeadingRangePositivePi() throws {
        var obb = OrientedBoundingBox()
        obb.headingRad = Float.pi

        #expect(obb.headingRad == Float.pi)
    }

    @Test func obbHeadingRangeZero() throws {
        var obb = OrientedBoundingBox()
        obb.headingRad = 0.0

        #expect(obb.headingRad == 0.0)
    }

    @Test func obbSmallDimensions() throws {
        let obb = OrientedBoundingBox(
            centerX: 0, centerY: 0, centerZ: 0, length: 0.1, width: 0.1, height: 0.1, headingRad: 0)

        #expect(obb.length == 0.1)
        #expect(obb.width == 0.1)
        #expect(obb.height == 0.1)
    }

    @Test func obbLargeDimensions() throws {
        // Simulate a large vehicle like a bus or truck
        let obb = OrientedBoundingBox(
            centerX: 50, centerY: 100, centerZ: 1.5, length: 12.0,  // 12m long
            width: 2.5,  // 2.5m wide
            height: 3.5,  // 3.5m tall
            headingRad: Float.pi / 4)

        #expect(obb.length == 12.0)
        #expect(obb.width == 2.5)
        #expect(obb.height == 3.5)
    }
}

// MARK: - Cluster OBB Integration Tests

struct ClusterOBBIntegrationTests {
    @Test func clusterWithoutOBB() throws {
        let cluster = Cluster()
        #expect(cluster.obb == nil)
    }

    @Test func clusterWithOBB() throws {
        var cluster = Cluster()
        cluster.obb = OrientedBoundingBox(
            centerX: 10, centerY: 20, centerZ: 1, length: 4, width: 2, height: 1.5, headingRad: 0.5)

        #expect(cluster.obb != nil)
        #expect(cluster.obb?.length == 4)
        #expect(cluster.obb?.width == 2)
        #expect(cluster.obb?.height == 1.5)
        #expect(cluster.obb?.headingRad == 0.5)
    }

    @Test func clusterOBBOverridesAABB() throws {
        var cluster = Cluster()

        // Set AABB dimensions
        cluster.aabbLength = 3.0
        cluster.aabbWidth = 1.5
        cluster.aabbHeight = 1.0

        // Set OBB with different dimensions
        cluster.obb = OrientedBoundingBox(
            centerX: 0, centerY: 0, centerZ: 0, length: 4.0, width: 2.0, height: 1.5, headingRad: 0)

        // OBB should have its own dimensions
        #expect(cluster.obb?.length == 4.0)
        #expect(cluster.aabbLength == 3.0)

        // Both exist independently
        #expect(cluster.obb != nil)
        #expect(cluster.aabbLength != cluster.obb?.length)
    }
}

// MARK: - Frame Type Tests

struct FrameTypeExtendedTests {
    @Test func allFrameTypesHaveUniqueRawValues() throws {
        let rawValues = [
            FrameType.full.rawValue, FrameType.foreground.rawValue, FrameType.background.rawValue,
            FrameType.delta.rawValue,
        ]

        let uniqueValues = Set(rawValues)
        #expect(uniqueValues.count == 4, "All frame types should have unique raw values")
    }

    @Test func frameTypeRoundTrip() throws {
        for i in 0...3 {
            let original = FrameType(rawValue: i)!
            let rawValue = original.rawValue
            let restored = FrameType(rawValue: rawValue)
            #expect(original == restored, "Frame type should round-trip through raw value")
        }
    }
}

// MARK: - Point Cloud Classification Tests

struct PointCloudClassificationTests {
    @Test func classificationBackground() throws {
        var pc = PointCloudFrame()
        pc.classification = [0, 0, 0]

        for c in pc.classification { #expect(c == 0, "Classification 0 = background") }
    }

    @Test func classificationForeground() throws {
        var pc = PointCloudFrame()
        pc.classification = [1, 1, 1]

        for c in pc.classification { #expect(c == 1, "Classification 1 = foreground") }
    }

    @Test func classificationGround() throws {
        var pc = PointCloudFrame()
        pc.classification = [2, 2, 2]

        for c in pc.classification { #expect(c == 2, "Classification 2 = ground") }
    }

    @Test func mixedClassifications() throws {
        var pc = PointCloudFrame()
        pc.classification = [0, 1, 2, 0, 1, 2]

        let backgroundCount = pc.classification.filter { $0 == 0 }.count
        let foregroundCount = pc.classification.filter { $0 == 1 }.count
        let groundCount = pc.classification.filter { $0 == 2 }.count

        #expect(backgroundCount == 2)
        #expect(foregroundCount == 2)
        #expect(groundCount == 2)
    }
}

// MARK: - Confidence Value Tests

struct ConfidenceValueTests {
    @Test func trackConfidenceRange() throws {
        var track = Track()

        track.confidence = 0.0
        #expect(track.confidence >= 0)

        track.confidence = 1.0
        #expect(track.confidence <= 1.0)

        track.confidence = 0.5
        #expect(track.confidence == 0.5)
    }

    @Test func classConfidenceRange() throws {
        var track = Track()

        track.classConfidence = 0.95
        #expect(track.classConfidence == 0.95)

        track.classConfidence = 0.0
        #expect(track.classConfidence == 0.0)
    }

    @Test func backgroundConfidenceValues() throws {
        var bg = BackgroundSnapshot()
        bg.confidence = [1, 5, 10, 50, 255]

        #expect(bg.confidence[0] == 1)  // Low confidence
        #expect(bg.confidence[4] == 255)  // High confidence (maximum 8-bit confidence value, UInt8.max)
    }
}

// MARK: - Timestamp Nanosecond Tests

struct TimestampNanosecondTests {
    @Test func timestampInNanoseconds() throws {
        var frame = FrameBundle()
        frame.timestampNanos = 1_000_000_000  // 1 second

        let seconds = Double(frame.timestampNanos) / 1_000_000_000.0
        #expect(seconds == 1.0)
    }

    @Test func timestampMillisecondConversion() throws {
        let nanoseconds: Int64 = 1_500_000_000  // 1.5 seconds
        let milliseconds = nanoseconds / 1_000_000

        #expect(milliseconds == 1500)
    }

    @Test func timestampMicrosecondConversion() throws {
        let nanoseconds: Int64 = 1_234_567_890
        let microseconds = nanoseconds / 1_000

        #expect(microseconds == 1_234_567)
    }

    @Test func trackDurationCalculation() throws {
        var track = Track()
        track.firstSeenNanos = 1_000_000_000
        track.lastSeenNanos = 5_000_000_000

        let durationNanos = track.lastSeenNanos - track.firstSeenNanos
        let durationSeconds = Double(durationNanos) / 1_000_000_000.0

        #expect(durationSeconds == 4.0)
    }
}

// MARK: - Velocity Calculation Tests

struct VelocityCalculationTests {
    @Test func speedFromVelocityComponents() throws {
        var track = Track()
        track.vx = 3.0
        track.vy = 4.0
        track.vz = 0.0

        // Manual speed calculation: sqrt(3^2 + 4^2) = 5
        let calculatedSpeed = sqrt(track.vx * track.vx + track.vy * track.vy + track.vz * track.vz)
        #expect(abs(calculatedSpeed - 5.0) < 0.001)
    }

    @Test func headingFromVelocity() throws {
        var track = Track()
        track.vx = 1.0
        track.vy = 0.0

        // Heading for moving along +X axis should be 0
        let heading = atan2(track.vy, track.vx)
        #expect(abs(heading - 0.0) < 0.001)
    }

    @Test func headingFor90Degrees() throws {
        var track = Track()
        track.vx = 0.0
        track.vy = 1.0

        // Heading for moving along +Y axis should be pi/2
        let heading = atan2(track.vy, track.vx)
        #expect(abs(heading - Float.pi / 2) < 0.001)
    }

    @Test func speedInKmh() throws {
        var track = Track()
        track.speedMps = 10.0  // 10 m/s

        let speedKmh = track.speedMps * 3.6
        #expect(abs(speedKmh - 36.0) < 0.001)
    }

    @Test func speedInMph() throws {
        var track = Track()
        track.speedMps = 10.0  // 10 m/s

        let speedMph = track.speedMps * 2.237
        #expect(abs(speedMph - 22.37) < 0.01)
    }
}
