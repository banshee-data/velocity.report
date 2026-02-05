//
//  CompositePointCloudRendererTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for M3.5 CompositePointCloudRenderer.
//

import Foundation
import Metal
import Testing

@testable import VelocityVisualiser

// MARK: - Composite Renderer Tests

struct CompositePointCloudRendererTests {

    // Helper to create a Metal device (or skip test if unavailable)
    private func createDevice() throws -> MTLDevice {
        guard let device = MTLCreateSystemDefaultDevice() else {
            throw TestError.metalNotAvailable
        }
        return device
    }

    enum TestError: Error {
        case metalNotAvailable
    }

    // MARK: - Initialisation Tests

    @Test func rendererInitialisation() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        let stats = renderer.getStats()
        #expect(stats.background == 0)
        #expect(stats.foreground == 0)
        #expect(stats.total == 0)
        #expect(renderer.cacheState.description == "Empty")
    }

    @Test func cacheStateInitiallyEmpty() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        switch renderer.cacheState {
        case .empty:
            #expect(true)
        default:
            #expect(Bool(false), "Expected empty state")
        }
    }

    // MARK: - Background Caching Tests

    @Test func processBackgroundFrame() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 1
        bg.timestampNanos = 1_000_000_000
        bg.x = [1.0, 2.0, 3.0, 4.0, 5.0]
        bg.y = [6.0, 7.0, 8.0, 9.0, 10.0]
        bg.z = [0.5, 0.6, 0.7, 0.8, 0.9]
        bg.confidence = [5, 10, 15, 20, 25]

        var frame = FrameBundle()
        frame.frameType = .background
        frame.background = bg

        renderer.processFrame(frame)

        let stats = renderer.getStats()
        #expect(stats.background == 5)
        #expect(stats.foreground == 0)
        #expect(stats.total == 5)

        // Cache should be in cached state
        switch renderer.cacheState {
        case .cached(let seq):
            #expect(seq == 1)
        default:
            #expect(Bool(false), "Expected cached state")
        }
    }

    @Test func processForegroundFrame() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        var pc = PointCloudFrame()
        pc.frameID = 100
        pc.x = [10.0, 20.0]
        pc.y = [30.0, 40.0]
        pc.z = [1.0, 1.5]
        pc.intensity = [200, 250]
        pc.pointCount = 2

        var frame = FrameBundle()
        frame.frameType = .foreground
        frame.pointCloud = pc
        frame.backgroundSeq = 0  // No background cached

        renderer.processFrame(frame)

        let stats = renderer.getStats()
        #expect(stats.foreground == 2)
        #expect(stats.background == 0)
        #expect(stats.total == 2)
    }

    @Test func processFullFrameLegacy() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        var pc = PointCloudFrame()
        pc.x = [1.0, 2.0, 3.0]
        pc.y = [4.0, 5.0, 6.0]
        pc.z = [0.5, 0.6, 0.7]
        pc.intensity = [100, 150, 200]
        pc.pointCount = 3

        var frame = FrameBundle()
        frame.frameType = .full
        frame.pointCloud = pc

        renderer.processFrame(frame)

        let stats = renderer.getStats()
        #expect(stats.foreground == 3)
        #expect(stats.total == 3)
    }

    // MARK: - Cache Validation Tests

    @Test func cacheSequenceTracking() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        // Process background frame
        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 42
        bg.x = [1.0]
        bg.y = [2.0]
        bg.z = [0.5]
        bg.confidence = [10]

        var bgFrame = FrameBundle()
        bgFrame.frameType = .background
        bgFrame.background = bg
        renderer.processFrame(bgFrame)

        // Check cache is valid
        switch renderer.cacheState {
        case .cached(let seq):
            #expect(seq == 42)
        default:
            #expect(Bool(false), "Expected cached state")
        }

        // Process foreground with matching sequence
        var fgFrame = FrameBundle()
        fgFrame.frameType = .foreground
        fgFrame.backgroundSeq = 42
        var pc = PointCloudFrame()
        pc.x = [10.0]
        pc.y = [20.0]
        pc.z = [1.0]
        pc.intensity = [200]
        pc.pointCount = 1
        fgFrame.pointCloud = pc

        renderer.processFrame(fgFrame)

        // Cache should still be valid
        switch renderer.cacheState {
        case .cached(let seq):
            #expect(seq == 42)
        default:
            #expect(Bool(false), "Expected cached state")
        }
    }

    @Test func cacheInvalidationOnSequenceMismatch() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        // Cache background with seq 1
        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 1
        bg.x = [1.0]
        bg.y = [2.0]
        bg.z = [0.5]
        bg.confidence = [10]

        var bgFrame = FrameBundle()
        bgFrame.frameType = .background
        bgFrame.background = bg
        renderer.processFrame(bgFrame)

        // Process foreground with mismatched sequence
        var fgFrame = FrameBundle()
        fgFrame.frameType = .foreground
        fgFrame.backgroundSeq = 99  // Mismatch!
        var pc = PointCloudFrame()
        pc.x = [10.0]
        pc.y = [20.0]
        pc.z = [1.0]
        pc.intensity = [200]
        pc.pointCount = 1
        fgFrame.pointCloud = pc

        renderer.processFrame(fgFrame)

        // Cache should be invalidated
        switch renderer.cacheState {
        case .empty:
            #expect(true)
        default:
            #expect(Bool(false), "Expected empty state after mismatch")
        }
    }

    @Test func clearCache() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        // Populate cache
        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 5
        bg.x = [1.0, 2.0]
        bg.y = [3.0, 4.0]
        bg.z = [0.5, 0.6]
        bg.confidence = [10, 15]

        var bgFrame = FrameBundle()
        bgFrame.frameType = .background
        bgFrame.background = bg
        renderer.processFrame(bgFrame)

        var statsBeforeClear = renderer.getStats()
        #expect(statsBeforeClear.background > 0)

        // Clear cache
        renderer.clearCache()

        let statsAfterClear = renderer.getStats()
        #expect(statsAfterClear.background == 0)
        #expect(statsAfterClear.foreground == 0)
        #expect(statsAfterClear.total == 0)

        switch renderer.cacheState {
        case .empty:
            #expect(true)
        default:
            #expect(Bool(false), "Expected empty state after clear")
        }
    }

    // MARK: - Composite Rendering Tests

    @Test func compositeRenderingStats() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        // Add background
        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 1
        bg.x = Array(repeating: 1.0, count: 100)
        bg.y = Array(repeating: 2.0, count: 100)
        bg.z = Array(repeating: 0.5, count: 100)
        bg.confidence = Array(repeating: 10, count: 100)

        var bgFrame = FrameBundle()
        bgFrame.frameType = .background
        bgFrame.background = bg
        renderer.processFrame(bgFrame)

        // Add foreground
        var pc = PointCloudFrame()
        pc.x = Array(repeating: 10.0, count: 50)
        pc.y = Array(repeating: 20.0, count: 50)
        pc.z = Array(repeating: 1.0, count: 50)
        pc.intensity = Array(repeating: 200, count: 50)
        pc.pointCount = 50

        var fgFrame = FrameBundle()
        fgFrame.frameType = .foreground
        fgFrame.pointCloud = pc
        fgFrame.backgroundSeq = 1
        renderer.processFrame(fgFrame)

        let stats = renderer.getStats()
        #expect(stats.background == 100)
        #expect(stats.foreground == 50)
        #expect(stats.total == 150)
    }

    @Test func emptyBackgroundSnapshot() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 1
        // Empty arrays (pointCount = 0)

        var bgFrame = FrameBundle()
        bgFrame.frameType = .background
        bgFrame.background = bg
        renderer.processFrame(bgFrame)

        let stats = renderer.getStats()
        #expect(stats.background == 0)
    }

    @Test func emptyForegroundFrame() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        var pc = PointCloudFrame()
        pc.pointCount = 0
        // Empty arrays

        var fgFrame = FrameBundle()
        fgFrame.frameType = .foreground
        fgFrame.pointCloud = pc
        renderer.processFrame(fgFrame)

        let stats = renderer.getStats()
        #expect(stats.foreground == 0)
    }

    // MARK: - Large Point Cloud Tests

    @Test func largeBackgroundSnapshot() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        let count = 65_000  // Typical full LIDAR frame
        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 10
        bg.x = Array(repeating: 1.0, count: count)
        bg.y = Array(repeating: 2.0, count: count)
        bg.z = Array(repeating: 0.5, count: count)
        bg.confidence = Array(repeating: 15, count: count)

        var bgFrame = FrameBundle()
        bgFrame.frameType = .background
        bgFrame.background = bg
        renderer.processFrame(bgFrame)

        let stats = renderer.getStats()
        #expect(stats.background == 65_000)
    }

    @Test func largeForegroundFrame() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        let count = 10_000  // Typical foreground point count
        var pc = PointCloudFrame()
        pc.x = Array(repeating: 10.0, count: count)
        pc.y = Array(repeating: 20.0, count: count)
        pc.z = Array(repeating: 1.0, count: count)
        pc.intensity = Array(repeating: 200, count: count)
        pc.pointCount = count

        var fgFrame = FrameBundle()
        fgFrame.frameType = .foreground
        fgFrame.pointCloud = pc
        renderer.processFrame(fgFrame)

        let stats = renderer.getStats()
        #expect(stats.foreground == 10_000)
    }

    // MARK: - Cache Status Tests

    @Test func cacheStatusDescriptions() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        // Initially empty
        #expect(renderer.cacheStatus.contains("Empty"))

        // After caching background
        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 7
        bg.x = [1.0]
        bg.y = [2.0]
        bg.z = [0.5]
        bg.confidence = [10]

        var bgFrame = FrameBundle()
        bgFrame.frameType = .background
        bgFrame.background = bg
        renderer.processFrame(bgFrame)

        #expect(renderer.cacheStatus.contains("Cached"))
        #expect(renderer.cacheStatus.contains("7"))  // Should show sequence number
    }
}
