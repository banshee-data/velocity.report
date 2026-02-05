//
//  ModelM35Tests.swift
//  VelocityVisualiserTests
//
//  Unit tests for M3.5 split streaming models.
//

import Foundation
import Testing

@testable import VelocityVisualiser

// MARK: - FrameType Tests

struct FrameTypeTests {
    @Test func frameTypeRawValues() throws {
        #expect(FrameType.full.rawValue == 0)
        #expect(FrameType.foreground.rawValue == 1)
        #expect(FrameType.background.rawValue == 2)
        #expect(FrameType.delta.rawValue == 3)
    }

    @Test func frameTypeFromRawValue() throws {
        #expect(FrameType(rawValue: 0) == .full)
        #expect(FrameType(rawValue: 1) == .foreground)
        #expect(FrameType(rawValue: 2) == .background)
        #expect(FrameType(rawValue: 3) == .delta)
    }

    @Test func frameTypeInvalidRawValue() throws {
        #expect(FrameType(rawValue: 99) == nil)
        #expect(FrameType(rawValue: -1) == nil)
    }
}

// MARK: - GridMetadata Tests

struct GridMetadataTests {
    @Test func gridMetadataDefaultValues() throws {
        let grid = GridMetadata()
        #expect(grid.rings == 40)
        #expect(grid.azimuthBins == 1800)
        #expect(grid.ringElevations.isEmpty)
        #expect(grid.settlingComplete == false)
    }

    @Test func gridMetadataCustomValues() throws {
        var grid = GridMetadata()
        grid.rings = 64
        grid.azimuthBins = 3600
        grid.ringElevations = [
            -25.0, -20.0, -15.0, -10.0, -5.0, 0.0, 5.0, 10.0, 15.0, 20.0,
        ]
        grid.settlingComplete = true

        #expect(grid.rings == 64)
        #expect(grid.azimuthBins == 3600)
        #expect(grid.ringElevations.count == 10)
        #expect(grid.ringElevations[0] == -25.0)
        #expect(grid.ringElevations[9] == 20.0)
        #expect(grid.settlingComplete == true)
    }

    @Test func gridMetadataNegativeElevations() throws {
        var grid = GridMetadata()
        grid.ringElevations = [-15.0, -10.0, -5.0, 0.0, 5.0, 10.0, 15.0]

        #expect(grid.ringElevations.count == 7)
        #expect(grid.ringElevations.first == -15.0)
        #expect(grid.ringElevations.last == 15.0)
    }
}

// MARK: - BackgroundSnapshot Tests

struct BackgroundSnapshotTests {
    @Test func backgroundSnapshotDefaultValues() throws {
        let bg = BackgroundSnapshot()
        #expect(bg.sequenceNumber == 0)
        #expect(bg.timestampNanos == 0)
        #expect(bg.x.isEmpty)
        #expect(bg.y.isEmpty)
        #expect(bg.z.isEmpty)
        #expect(bg.confidence.isEmpty)
        #expect(bg.gridMetadata == nil)
        #expect(bg.pointCount == 0)
    }

    @Test func backgroundSnapshotWithPoints() throws {
        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 42
        bg.timestampNanos = 1_234_567_890_000
        bg.x = [1.0, 2.0, 3.0]
        bg.y = [4.0, 5.0, 6.0]
        bg.z = [0.5, 0.6, 0.7]
        bg.confidence = [5, 8, 12]

        #expect(bg.sequenceNumber == 42)
        #expect(bg.timestampNanos == 1_234_567_890_000)
        #expect(bg.pointCount == 3)
        #expect(bg.x[0] == 1.0)
        #expect(bg.y[1] == 5.0)
        #expect(bg.z[2] == 0.7)
        #expect(bg.confidence[0] == 5)
        #expect(bg.confidence[2] == 12)
    }

    @Test func backgroundSnapshotPointCountConsistency() throws {
        var bg = BackgroundSnapshot()
        bg.x = Array(repeating: 0, count: 1000)
        bg.y = Array(repeating: 0, count: 1000)
        bg.z = Array(repeating: 0, count: 1000)
        bg.confidence = Array(repeating: 0, count: 1000)

        #expect(bg.pointCount == 1000)
        #expect(bg.x.count == bg.y.count)
        #expect(bg.y.count == bg.z.count)
        #expect(bg.z.count == bg.confidence.count)
    }

    @Test func backgroundSnapshotWithGrid() throws {
        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 1
        bg.x = [10.0, 20.0]
        bg.y = [30.0, 40.0]
        bg.z = [0.0, 1.0]
        bg.confidence = [10, 15]

        var grid = GridMetadata()
        grid.rings = 40
        grid.azimuthBins = 1800
        grid.settlingComplete = true
        bg.gridMetadata = grid

        #expect(bg.gridMetadata != nil)
        #expect(bg.gridMetadata?.rings == 40)
        #expect(bg.gridMetadata?.azimuthBins == 1800)
        #expect(bg.gridMetadata?.settlingComplete == true)
    }

    @Test func backgroundSnapshotSequenceNumberIncrement() throws {
        var bg1 = BackgroundSnapshot()
        bg1.sequenceNumber = 1

        var bg2 = BackgroundSnapshot()
        bg2.sequenceNumber = 2

        var bg3 = BackgroundSnapshot()
        bg3.sequenceNumber = 3

        #expect(bg1.sequenceNumber == 1)
        #expect(bg2.sequenceNumber == 2)
        #expect(bg3.sequenceNumber == 3)
        #expect(bg3.sequenceNumber > bg1.sequenceNumber)
    }

    @Test func backgroundSnapshotLargePointCloud() throws {
        var bg = BackgroundSnapshot()
        let count = 65_000  // Typical LIDAR frame size

        bg.x = Array(repeating: Float.random(in: -100...100), count: count)
        bg.y = Array(repeating: Float.random(in: -100...100), count: count)
        bg.z = Array(repeating: Float.random(in: 0...10), count: count)
        bg.confidence = Array(repeating: UInt32.random(in: 0...255), count: count)

        #expect(bg.pointCount == 65_000)
        #expect(bg.x.count == count)
        #expect(bg.y.count == count)
        #expect(bg.z.count == count)
        #expect(bg.confidence.count == count)
    }

    @Test func backgroundSnapshotEmptyPointCloud() throws {
        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 10
        bg.timestampNanos = 1_000_000_000
        // Arrays left empty

        #expect(bg.pointCount == 0)
        #expect(bg.x.isEmpty)
        #expect(bg.y.isEmpty)
        #expect(bg.z.isEmpty)
        #expect(bg.confidence.isEmpty)
    }
}

// MARK: - FrameBundle M3.5 Tests

struct FrameBundleM35Tests {
    @Test func frameBundleDefaultFrameType() throws {
        let frame = FrameBundle()
        #expect(frame.frameType == .full)
        #expect(frame.background == nil)
        #expect(frame.backgroundSeq == 0)
    }

    @Test func frameBundleForegroundFrame() throws {
        var frame = FrameBundle()
        frame.frameID = 100
        frame.frameType = .foreground
        frame.backgroundSeq = 42

        var pc = PointCloudFrame()
        pc.frameID = 100
        pc.x = [1.0, 2.0, 3.0]
        pc.y = [4.0, 5.0, 6.0]
        pc.z = [0.5, 0.6, 0.7]
        pc.intensity = [100, 150, 200]
        pc.pointCount = 3
        frame.pointCloud = pc

        #expect(frame.frameType == .foreground)
        #expect(frame.backgroundSeq == 42)
        #expect(frame.pointCloud?.pointCount == 3)
        #expect(frame.background == nil)
    }

    @Test func frameBundleBackgroundFrame() throws {
        var frame = FrameBundle()
        frame.frameID = 50
        frame.frameType = .background

        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 1
        bg.timestampNanos = 5_000_000_000
        bg.x = [10.0, 20.0, 30.0]
        bg.y = [40.0, 50.0, 60.0]
        bg.z = [0.0, 0.5, 1.0]
        bg.confidence = [5, 10, 15]
        frame.background = bg
        frame.backgroundSeq = bg.sequenceNumber

        #expect(frame.frameType == .background)
        #expect(frame.background != nil)
        #expect(frame.background?.sequenceNumber == 1)
        #expect(frame.background?.pointCount == 3)
        #expect(frame.backgroundSeq == 1)
    }

    @Test func frameBundleFullFrameLegacy() throws {
        var frame = FrameBundle()
        frame.frameType = .full

        var pc = PointCloudFrame()
        pc.x = Array(repeating: 1.0, count: 100)
        pc.y = Array(repeating: 2.0, count: 100)
        pc.z = Array(repeating: 0.5, count: 100)
        pc.intensity = Array(repeating: 128, count: 100)
        pc.pointCount = 100
        frame.pointCloud = pc

        #expect(frame.frameType == .full)
        #expect(frame.pointCloud?.pointCount == 100)
        #expect(frame.background == nil)
    }

    @Test func frameBundleSequenceNumberTracking() throws {
        // Simulate sequence: background, foreground, foreground
        var bgFrame = FrameBundle()
        bgFrame.frameType = .background
        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 1
        bgFrame.background = bg
        bgFrame.backgroundSeq = 1

        var fgFrame1 = FrameBundle()
        fgFrame1.frameType = .foreground
        fgFrame1.backgroundSeq = 1  // References background seq 1

        var fgFrame2 = FrameBundle()
        fgFrame2.frameType = .foreground
        fgFrame2.backgroundSeq = 1  // Still valid

        #expect(bgFrame.backgroundSeq == 1)
        #expect(fgFrame1.backgroundSeq == 1)
        #expect(fgFrame2.backgroundSeq == 1)
        #expect(fgFrame1.backgroundSeq == bgFrame.backgroundSeq)
    }

    @Test func frameBundleSequenceMismatch() throws {
        // Background gets reset (new sequence number)
        var bgFrame = BackgroundSnapshot()
        bgFrame.sequenceNumber = 2  // Incremented due to reset

        var fgFrame = FrameBundle()
        fgFrame.frameType = .foreground
        fgFrame.backgroundSeq = 1  // Old sequence

        // Client should detect mismatch and request new background
        #expect(bgFrame.sequenceNumber != fgFrame.backgroundSeq)
        #expect(bgFrame.sequenceNumber > fgFrame.backgroundSeq)
    }
}
