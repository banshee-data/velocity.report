//
//  MetalRendererTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for MetalRenderer components including Camera, matrix extensions,
//  and buffer allocation logic.
//

import Foundation
import Metal
import Testing
import XCTest
import simd

@testable import VelocityVisualiser

// MARK: - Camera Tests

struct CameraTests {
    @Test func defaultInitialisation() throws {
        let camera = Camera()

        // Default position
        #expect(camera.position.x == 0)
        #expect(camera.position.y == -30)
        #expect(camera.position.z == 20)

        // Default target
        #expect(camera.target.x == 0)
        #expect(camera.target.y == 10)
        #expect(camera.target.z == 0)

        // Default up vector
        #expect(camera.up.x == 0)
        #expect(camera.up.y == 0)
        #expect(camera.up.z == 1)

        // Default projection parameters
        #expect(camera.fov == 60.0)
        #expect(camera.aspectRatio == 1.0)
        #expect(camera.nearPlane == 0.1)
        #expect(camera.farPlane == 500.0)
    }

    @Test func customPosition() throws {
        var camera = Camera()
        camera.position = simd_float3(10, 20, 30)

        #expect(camera.position.x == 10)
        #expect(camera.position.y == 20)
        #expect(camera.position.z == 30)
    }

    @Test func customTarget() throws {
        var camera = Camera()
        camera.target = simd_float3(50, 60, 5)

        #expect(camera.target.x == 50)
        #expect(camera.target.y == 60)
        #expect(camera.target.z == 5)
    }

    @Test func customProjectionParameters() throws {
        var camera = Camera()
        camera.fov = 90.0
        camera.aspectRatio = 16.0 / 9.0
        camera.nearPlane = 0.5
        camera.farPlane = 1000.0

        #expect(camera.fov == 90.0)
        #expect(abs(camera.aspectRatio - 16.0 / 9.0) < 0.001)
        #expect(camera.nearPlane == 0.5)
        #expect(camera.farPlane == 1000.0)
    }

    @Test func viewMatrixIsValid() throws {
        let camera = Camera()
        let viewMatrix = camera.viewMatrix

        // View matrix should be a valid 4x4 matrix
        // Check that it's not the identity (camera is not at origin looking at origin)
        let isIdentity =
            viewMatrix.columns.0.x == 1 && viewMatrix.columns.1.y == 1
            && viewMatrix.columns.2.z == 1 && viewMatrix.columns.3.w == 1
        #expect(!isIdentity, "View matrix should not be identity for non-origin camera")
    }

    @Test func projectionMatrixIsValid() throws {
        let camera = Camera()
        let projMatrix = camera.projectionMatrix

        // Projection matrix should have reasonable values
        // The [3][3] element should be -1 for perspective projection
        #expect(projMatrix.columns.2.w == -1.0)

        // The diagonal elements should be related to FOV and aspect ratio
        #expect(projMatrix.columns.0.x > 0, "X scale should be positive")
        #expect(projMatrix.columns.1.y > 0, "Y scale should be positive")
    }

    @Test func viewMatrixChangesWithPosition() throws {
        var camera = Camera()
        let original = camera.viewMatrix

        camera.position = simd_float3(100, 100, 100)
        let changed = camera.viewMatrix

        // Matrices should be different
        let isDifferent =
            original.columns.3.x != changed.columns.3.x
            || original.columns.3.y != changed.columns.3.y
            || original.columns.3.z != changed.columns.3.z

        #expect(isDifferent, "View matrix should change with camera position")
    }

    @Test func projectionMatrixChangesWithFOV() throws {
        var camera = Camera()
        let original = camera.projectionMatrix

        camera.fov = 120.0
        let changed = camera.projectionMatrix

        // Y scale should be different with different FOV
        #expect(
            original.columns.1.y != changed.columns.1.y, "Projection matrix should change with FOV")
    }

    @Test func projectionMatrixChangesWithAspectRatio() throws {
        var camera = Camera()
        let original = camera.projectionMatrix

        camera.aspectRatio = 2.0
        let changed = camera.projectionMatrix

        // X scale should be different with different aspect ratio
        #expect(
            original.columns.0.x != changed.columns.0.x,
            "Projection matrix should change with aspect ratio")
    }
}

// MARK: - Matrix Extension Tests

struct MatrixExtensionTests {
    @Test func translationMatrixIdentityAtZero() throws {
        let matrix = simd_float4x4(translation: simd_float3(0, 0, 0))

        // Should be identity matrix
        #expect(matrix.columns.0.x == 1)
        #expect(matrix.columns.1.y == 1)
        #expect(matrix.columns.2.z == 1)
        #expect(matrix.columns.3.w == 1)
        #expect(matrix.columns.3.x == 0)
        #expect(matrix.columns.3.y == 0)
        #expect(matrix.columns.3.z == 0)
    }

    @Test func translationMatrixWithOffset() throws {
        let matrix = simd_float4x4(translation: simd_float3(10, 20, 30))

        // Translation should be in the fourth column
        #expect(matrix.columns.3.x == 10)
        #expect(matrix.columns.3.y == 20)
        #expect(matrix.columns.3.z == 30)
        #expect(matrix.columns.3.w == 1)

        // Rest should be identity
        #expect(matrix.columns.0.x == 1)
        #expect(matrix.columns.1.y == 1)
        #expect(matrix.columns.2.z == 1)
    }

    @Test func rotationZMatrixIdentityAtZero() throws {
        let matrix = simd_float4x4(rotationZ: 0)

        // Should be close to identity (within floating point tolerance)
        #expect(abs(matrix.columns.0.x - 1) < 0.0001)
        #expect(abs(matrix.columns.1.y - 1) < 0.0001)
        #expect(matrix.columns.2.z == 1)
        #expect(matrix.columns.3.w == 1)
    }

    @Test func rotationZMatrix90Degrees() throws {
        let matrix = simd_float4x4(rotationZ: Float.pi / 2)

        // At 90 degrees: cos = 0, sin = 1
        #expect(abs(matrix.columns.0.x - 0) < 0.0001)  // cos(90)
        #expect(abs(matrix.columns.0.y - 1) < 0.0001)  // sin(90)
        #expect(abs(matrix.columns.1.x - (-1)) < 0.0001)  // -sin(90)
        #expect(abs(matrix.columns.1.y - 0) < 0.0001)  // cos(90)
    }

    @Test func rotationZMatrix180Degrees() throws {
        let matrix = simd_float4x4(rotationZ: Float.pi)

        // At 180 degrees: cos = -1, sin = 0
        #expect(abs(matrix.columns.0.x - (-1)) < 0.0001)
        #expect(abs(matrix.columns.0.y - 0) < 0.0001)
        #expect(abs(matrix.columns.1.x - 0) < 0.0001)
        #expect(abs(matrix.columns.1.y - (-1)) < 0.0001)
    }

    @Test func rotationZMatrixNegativeAngle() throws {
        let positiveRotation = simd_float4x4(rotationZ: Float.pi / 4)
        let negativeRotation = simd_float4x4(rotationZ: -Float.pi / 4)

        // Negative rotation should mirror across X axis
        #expect(abs(positiveRotation.columns.0.x - negativeRotation.columns.0.x) < 0.0001)
        #expect(abs(positiveRotation.columns.0.y - (-negativeRotation.columns.0.y)) < 0.0001)
    }

    @Test func lookAtMatrixBasic() throws {
        let eye = simd_float3(0, -10, 5)
        let target = simd_float3(0, 0, 0)
        let up = simd_float3(0, 0, 1)

        let matrix = simd_float4x4(lookAt: target, from: eye, up: up)

        // Matrix should be valid (non-zero, non-NaN)
        #expect(!matrix.columns.0.x.isNaN)
        #expect(!matrix.columns.1.y.isNaN)
        #expect(!matrix.columns.2.z.isNaN)
    }

    @Test func perspectiveMatrixBasic() throws {
        let fovRadians: Float = 60.0 * .pi / 180.0
        let aspectRatio: Float = 16.0 / 9.0
        let near: Float = 0.1
        let far: Float = 100.0

        let matrix = simd_float4x4(
            perspectiveFov: fovRadians, aspectRatio: aspectRatio, near: near, far: far)

        // Perspective projection characteristics
        #expect(matrix.columns.2.w == -1.0)  // Perspective divide
        #expect(matrix.columns.0.x > 0)  // Positive X scale
        #expect(matrix.columns.1.y > 0)  // Positive Y scale
        #expect(matrix.columns.2.z < 0)  // Negative Z (depth mapping)
    }

    @Test func perspectiveMatrixAspectRatioEffect() throws {
        let fovRadians: Float = 60.0 * .pi / 180.0
        let near: Float = 0.1
        let far: Float = 100.0

        let narrow = simd_float4x4(
            perspectiveFov: fovRadians, aspectRatio: 1.0, near: near, far: far)
        let wide = simd_float4x4(perspectiveFov: fovRadians, aspectRatio: 2.0, near: near, far: far)

        // Wider aspect ratio should have smaller X scale
        #expect(wide.columns.0.x < narrow.columns.0.x)
        // Y scale should be unchanged (same FOV)
        #expect(abs(wide.columns.1.y - narrow.columns.1.y) < 0.0001)
    }
}

// MARK: - MetalRenderer Uniforms Tests

struct UniformsTests {
    @Test func uniformsDefaultValues() throws {
        let uniforms = MetalRenderer.Uniforms(
            modelViewProjection: matrix_identity_float4x4, modelView: matrix_identity_float4x4,
            pointSize: 5.0, time: 0, padding: simd_float2(0, 0))

        #expect(uniforms.pointSize == 5.0)
        #expect(uniforms.time == 0)
        #expect(uniforms.padding.x == 0)
        #expect(uniforms.padding.y == 0)
    }

    @Test func uniformsCustomValues() throws {
        let mvp = simd_float4x4(translation: simd_float3(1, 2, 3))
        let mv = simd_float4x4(translation: simd_float3(4, 5, 6))

        let uniforms = MetalRenderer.Uniforms(
            modelViewProjection: mvp, modelView: mv, pointSize: 10.0, time: 1.5,
            padding: simd_float2(0, 0))

        #expect(uniforms.pointSize == 10.0)
        #expect(uniforms.time == 1.5)
        #expect(uniforms.modelViewProjection.columns.3.x == 1)
        #expect(uniforms.modelView.columns.3.x == 4)
    }

    @Test func uniformsMemoryLayoutStride() throws {
        // Uniforms should have a consistent memory layout for GPU buffers
        let stride = MemoryLayout<MetalRenderer.Uniforms>.stride
        // Should be at least the size of two 4x4 matrices + 4 floats
        let minExpectedSize = (16 + 16 + 4) * MemoryLayout<Float>.stride
        #expect(stride >= minExpectedSize)
    }
}

// MARK: - Buffer Allocation Logic Tests (using CompositePointCloudRenderer as proxy)

struct BufferAllocationLogicTests {
    // These tests verify the buffer allocation logic through CompositePointCloudRenderer
    // which uses the same logic as MetalRenderer

    private func createDevice() throws -> MTLDevice {
        guard let device = MTLCreateSystemDefaultDevice() else { throw TestError.metalNotAvailable }
        return device
    }

    enum TestError: Error { case metalNotAvailable }

    @Test func bufferStatsInitiallyZero() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        let stats = renderer.getBufferStats()
        #expect(stats.bgCapacity == 0)
        #expect(stats.bgUsed == 0)
        #expect(stats.fgCapacity == 0)
        #expect(stats.fgUsed == 0)
    }

    @Test func bufferStatsAfterBackgroundFrame() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 1
        bg.x = Array(repeating: 0, count: 100)
        bg.y = Array(repeating: 0, count: 100)
        bg.z = Array(repeating: 0, count: 100)
        bg.confidence = Array(repeating: 0, count: 100)

        var frame = FrameBundle()
        frame.frameType = .background
        frame.background = bg

        renderer.processFrame(frame)

        let stats = renderer.getBufferStats()
        #expect(stats.bgUsed == 100)
        #expect(stats.bgCapacity >= 100)  // Should have at least the needed capacity
    }

    @Test func bufferStatsAfterForegroundFrame() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        var pc = PointCloudFrame()
        pc.x = Array(repeating: 0, count: 50)
        pc.y = Array(repeating: 0, count: 50)
        pc.z = Array(repeating: 0, count: 50)
        pc.intensity = Array(repeating: 0, count: 50)
        pc.classification = Array(repeating: 0, count: 50)
        pc.pointCount = 50

        var frame = FrameBundle()
        frame.frameType = .foreground
        frame.pointCloud = pc

        renderer.processFrame(frame)

        let stats = renderer.getBufferStats()
        #expect(stats.fgUsed == 50)
        #expect(stats.fgCapacity >= 50)
    }

    @Test func bufferReuseOnSmallerFrame() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        // First, process a large frame
        var pc1 = PointCloudFrame()
        pc1.x = Array(repeating: 0, count: 1000)
        pc1.y = Array(repeating: 0, count: 1000)
        pc1.z = Array(repeating: 0, count: 1000)
        pc1.intensity = Array(repeating: 0, count: 1000)
        pc1.classification = Array(repeating: 0, count: 1000)
        pc1.pointCount = 1000

        var frame1 = FrameBundle()
        frame1.frameType = .foreground
        frame1.pointCloud = pc1
        renderer.processFrame(frame1)

        let initialCapacity = renderer.getBufferStats().fgCapacity

        // Then process a smaller frame
        var pc2 = PointCloudFrame()
        pc2.x = Array(repeating: 0, count: 500)
        pc2.y = Array(repeating: 0, count: 500)
        pc2.z = Array(repeating: 0, count: 500)
        pc2.intensity = Array(repeating: 0, count: 500)
        pc2.classification = Array(repeating: 0, count: 500)
        pc2.pointCount = 500

        var frame2 = FrameBundle()
        frame2.frameType = .foreground
        frame2.pointCloud = pc2
        renderer.processFrame(frame2)

        let stats = renderer.getBufferStats()
        #expect(stats.fgUsed == 500)
        // Capacity should still be sufficient (not shrunk yet since not >4x)
        #expect(stats.fgCapacity >= 500)
    }

    @Test func clearCacheResetsAllBuffers() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        // Add some data
        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 1
        bg.x = [1.0, 2.0, 3.0]
        bg.y = [4.0, 5.0, 6.0]
        bg.z = [0.5, 0.6, 0.7]
        bg.confidence = [10, 20, 30]

        var frame = FrameBundle()
        frame.frameType = .background
        frame.background = bg
        renderer.processFrame(frame)

        #expect(renderer.getStats().background > 0)

        // Clear cache
        renderer.clearCache()

        let stats = renderer.getStats()
        #expect(stats.background == 0)
        #expect(stats.foreground == 0)
        #expect(stats.total == 0)

        switch renderer.cacheState {
        case .empty: #expect(true)
        default: #expect(Bool(false), "Cache state should be empty after clear")
        }
    }

    @Test func isCacheStaleWhenEmpty() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        #expect(renderer.isCacheStale == true)
    }

    @Test func isCacheStaleAfterBackground() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 1
        bg.x = [1.0]
        bg.y = [2.0]
        bg.z = [0.5]
        bg.confidence = [10]

        var frame = FrameBundle()
        frame.frameType = .background
        frame.background = bg
        renderer.processFrame(frame)

        #expect(renderer.isCacheStale == false)
    }

    @Test func cacheStatusDescriptions() throws {
        let device = try createDevice()
        let renderer = CompositePointCloudRenderer(device: device)

        #expect(renderer.cacheStatus == "Empty")

        var bg = BackgroundSnapshot()
        bg.sequenceNumber = 42
        bg.x = [1.0]
        bg.y = [2.0]
        bg.z = [0.5]
        bg.confidence = [10]

        var frame = FrameBundle()
        frame.frameType = .background
        frame.background = bg
        renderer.processFrame(frame)

        #expect(renderer.cacheStatus.contains("Cached"))
        #expect(renderer.cacheStatus.contains("42"))
    }
}

// MARK: - Background Cache State Tests

struct BackgroundCacheStateTests {
    @Test func emptyStateDescription() throws {
        let state: BackgroundCacheState = .empty
        #expect(state.description == "Empty")
    }

    @Test func cachedStateDescription() throws {
        let state: BackgroundCacheState = .cached(seq: 123)
        #expect(state.description.contains("Cached"))
        #expect(state.description.contains("123"))
    }

    @Test func refreshingStateDescription() throws {
        let state: BackgroundCacheState = .refreshing
        #expect(state.description == "Refreshing...")
    }
}

// MARK: - MetalRenderer XCTest Tests (for Metal device access)

final class MetalRendererDeviceTests: XCTestCase {

    func testMetalDeviceAvailable() throws {
        let device = MTLCreateSystemDefaultDevice()
        XCTAssertNotNil(device, "Metal device should be available on macOS")
    }

    func testCommandQueueCreation() throws {
        guard let device = MTLCreateSystemDefaultDevice() else {
            throw XCTSkip("Metal not available")
        }

        let commandQueue = device.makeCommandQueue()
        XCTAssertNotNil(commandQueue, "Should be able to create command queue")
    }

    func testBufferCreation() throws {
        guard let device = MTLCreateSystemDefaultDevice() else {
            throw XCTSkip("Metal not available")
        }

        let bufferSize = 1024
        let buffer = device.makeBuffer(length: bufferSize, options: .storageModeShared)
        XCTAssertNotNil(buffer, "Should be able to create buffer")
        XCTAssertEqual(buffer?.length, bufferSize, "Buffer should have correct size")
    }

    func testBufferContentsAccess() throws {
        guard let device = MTLCreateSystemDefaultDevice() else {
            throw XCTSkip("Metal not available")
        }

        let floatCount = 100
        let bufferSize = floatCount * MemoryLayout<Float>.stride
        guard let buffer = device.makeBuffer(length: bufferSize, options: .storageModeShared) else {
            XCTFail("Failed to create buffer")
            return
        }

        // Write some data
        let ptr = buffer.contents().bindMemory(to: Float.self, capacity: floatCount)
        for i in 0..<floatCount { ptr[i] = Float(i) }

        // Verify data
        for i in 0..<floatCount {
            XCTAssertEqual(ptr[i], Float(i), "Buffer contents should match written data")
        }
    }
}
