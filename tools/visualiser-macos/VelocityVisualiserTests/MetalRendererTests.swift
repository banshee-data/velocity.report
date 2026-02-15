//
//  MetalRendererTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for MetalRenderer components including Camera, matrix extensions,
//  and buffer allocation logic.
//

import Foundation
import Metal
import MetalKit
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

// MARK: - MetalRenderer Camera Control Tests

final class MetalRendererCameraControlTests: XCTestCase {

    private func createRenderer() throws -> MetalRenderer {
        guard let device = MTLCreateSystemDefaultDevice() else {
            throw XCTSkip("Metal not available")
        }
        let metalView = MTKView()
        metalView.device = device
        guard let renderer = MetalRenderer(metalView: metalView) else {
            throw XCTSkip("Could not create MetalRenderer (shader library may not be available)")
        }
        return renderer
    }

    func testResetCamera() throws {
        let renderer = try createRenderer()

        // Move camera
        renderer.camera.position = simd_float3(100, 100, 100)
        renderer.camera.target = simd_float3(50, 50, 50)

        // Reset
        renderer.resetCamera()

        XCTAssertEqual(renderer.camera.position.x, 0, accuracy: 0.001)
        XCTAssertEqual(renderer.camera.position.y, -30, accuracy: 0.001)
        XCTAssertEqual(renderer.camera.position.z, 20, accuracy: 0.001)
        XCTAssertEqual(renderer.camera.target.x, 0, accuracy: 0.001)
        XCTAssertEqual(renderer.camera.target.y, 10, accuracy: 0.001)
        XCTAssertEqual(renderer.camera.target.z, 0, accuracy: 0.001)
    }

    func testHandleZoomIn() throws {
        let renderer = try createRenderer()
        let initialDistance = simd.length(renderer.camera.position - renderer.camera.target)

        renderer.handleZoom(delta: 2.0)  // Zoom in

        let newDistance = simd.length(renderer.camera.position - renderer.camera.target)
        XCTAssertLessThan(newDistance, initialDistance, "Zooming in should reduce distance")
    }

    func testHandleZoomOut() throws {
        let renderer = try createRenderer()
        let initialDistance = simd.length(renderer.camera.position - renderer.camera.target)

        renderer.handleZoom(delta: -2.0)  // Zoom out

        let newDistance = simd.length(renderer.camera.position - renderer.camera.target)
        XCTAssertGreaterThan(newDistance, initialDistance, "Zooming out should increase distance")
    }

    func testHandleZoomClampsMinimumDistance() throws {
        let renderer = try createRenderer()

        // Zoom in aggressively
        for _ in 0..<100 { renderer.handleZoom(delta: 10.0) }

        let distance = simd.length(renderer.camera.position - renderer.camera.target)
        XCTAssertGreaterThanOrEqual(distance, 1.0, "Distance should not go below minimum")
    }

    func testHandleZoomClampsMaximumDistance() throws {
        let renderer = try createRenderer()

        // Zoom out aggressively
        for _ in 0..<100 { renderer.handleZoom(delta: -10.0) }

        let distance = simd.length(renderer.camera.position - renderer.camera.target)
        XCTAssertLessThanOrEqual(distance, 500.0, "Distance should not exceed maximum")
    }

    func testHandleMouseDragOrbit() throws {
        let renderer = try createRenderer()
        let initialPosition = renderer.camera.position

        renderer.handleMouseDrag(deltaX: 50, deltaY: 0, isRightButton: false, shiftHeld: false)

        // Position should change (orbit rotates around target)
        let moved =
            renderer.camera.position.x != initialPosition.x
            || renderer.camera.position.y != initialPosition.y
        XCTAssertTrue(moved, "Camera should move during orbit")
    }

    func testHandleMouseDragPanRightButton() throws {
        let renderer = try createRenderer()
        let initialPosition = renderer.camera.position
        let initialTarget = renderer.camera.target

        renderer.handleMouseDrag(deltaX: 20, deltaY: 10, isRightButton: true, shiftHeld: false)

        // Both position and target should move together (pan)
        let positionMoved =
            renderer.camera.position.x != initialPosition.x
            || renderer.camera.position.y != initialPosition.y
        let targetMoved =
            renderer.camera.target.x != initialTarget.x
            || renderer.camera.target.y != initialTarget.y
        XCTAssertTrue(positionMoved, "Camera position should move during pan")
        XCTAssertTrue(targetMoved, "Camera target should move during pan")
    }

    func testHandleMouseDragPanShiftHeld() throws {
        let renderer = try createRenderer()
        let initialPosition = renderer.camera.position

        renderer.handleMouseDrag(deltaX: 20, deltaY: 10, isRightButton: false, shiftHeld: true)

        // Pan mode when shift is held
        let moved =
            renderer.camera.position.x != initialPosition.x
            || renderer.camera.position.y != initialPosition.y
        XCTAssertTrue(moved, "Camera should move during shift-pan")
    }

    func testHandleKeyDownResetCamera() throws {
        let renderer = try createRenderer()
        renderer.camera.position = simd_float3(50, 50, 50)

        let handled = renderer.handleKeyDown(keyCode: 15, modifiers: [])  // R key
        XCTAssertTrue(handled)
        XCTAssertEqual(renderer.camera.position.x, 0, accuracy: 0.001)
    }

    func testHandleKeyDownLeftArrow() throws {
        let renderer = try createRenderer()
        let initialPos = renderer.camera.position

        let handled = renderer.handleKeyDown(keyCode: 123, modifiers: [])  // Left arrow
        XCTAssertTrue(handled)

        let moved =
            renderer.camera.position.x != initialPos.x || renderer.camera.position.y != initialPos.y
        XCTAssertTrue(moved, "Left arrow should pan camera")
    }

    func testHandleKeyDownRightArrow() throws {
        let renderer = try createRenderer()
        let initialPos = renderer.camera.position

        let handled = renderer.handleKeyDown(keyCode: 124, modifiers: [])  // Right arrow
        XCTAssertTrue(handled)

        let moved =
            renderer.camera.position.x != initialPos.x || renderer.camera.position.y != initialPos.y
        XCTAssertTrue(moved, "Right arrow should pan camera")
    }

    func testHandleKeyDownUpArrow() throws {
        let renderer = try createRenderer()
        let initialPos = renderer.camera.position

        let handled = renderer.handleKeyDown(keyCode: 126, modifiers: [])  // Up arrow
        XCTAssertTrue(handled)

        let moved =
            renderer.camera.position.x != initialPos.x || renderer.camera.position.y != initialPos.y
            || renderer.camera.position.z != initialPos.z
        XCTAssertTrue(moved, "Up arrow should pan camera forward")
    }

    func testHandleKeyDownDownArrow() throws {
        let renderer = try createRenderer()
        let initialPos = renderer.camera.position

        let handled = renderer.handleKeyDown(keyCode: 125, modifiers: [])  // Down arrow
        XCTAssertTrue(handled)

        let moved =
            renderer.camera.position.x != initialPos.x || renderer.camera.position.y != initialPos.y
            || renderer.camera.position.z != initialPos.z
        XCTAssertTrue(moved, "Down arrow should pan camera backward")
    }

    func testHandleKeyDownPlusZoomIn() throws {
        let renderer = try createRenderer()
        let initialDistance = simd.length(renderer.camera.position - renderer.camera.target)

        let handled = renderer.handleKeyDown(keyCode: 24, modifiers: [])  // + key
        XCTAssertTrue(handled)

        let newDistance = simd.length(renderer.camera.position - renderer.camera.target)
        XCTAssertLessThan(newDistance, initialDistance)
    }

    func testHandleKeyDownMinusZoomOut() throws {
        let renderer = try createRenderer()
        let initialDistance = simd.length(renderer.camera.position - renderer.camera.target)

        let handled = renderer.handleKeyDown(keyCode: 27, modifiers: [])  // - key
        XCTAssertTrue(handled)

        let newDistance = simd.length(renderer.camera.position - renderer.camera.target)
        XCTAssertGreaterThan(newDistance, initialDistance)
    }

    func testHandleKeyDownUnhandledKey() throws {
        let renderer = try createRenderer()

        let handled = renderer.handleKeyDown(keyCode: 99, modifiers: [])  // Unknown key
        XCTAssertFalse(handled)
    }
}

// MARK: - MetalRenderer Frame Update Tests

final class MetalRendererFrameUpdateTests: XCTestCase {

    private func createRenderer() throws -> MetalRenderer {
        guard let device = MTLCreateSystemDefaultDevice() else {
            throw XCTSkip("Metal not available")
        }
        let metalView = MTKView()
        metalView.device = device
        guard let renderer = MetalRenderer(metalView: metalView) else {
            throw XCTSkip("Could not create MetalRenderer")
        }
        return renderer
    }

    func testUpdateFrameEmpty() throws {
        let renderer = try createRenderer()
        let frame = FrameBundle()
        renderer.updateFrame(frame)

        XCTAssertEqual(renderer.pointCount, 0)
        XCTAssertEqual(renderer.boxInstanceCount, 0)
        XCTAssertEqual(renderer.clusterInstanceCount, 0)
    }

    func testUpdateFrameWithPointCloud() throws {
        let renderer = try createRenderer()

        var pc = PointCloudFrame()
        pc.x = [1.0, 2.0, 3.0]
        pc.y = [4.0, 5.0, 6.0]
        pc.z = [0.1, 0.2, 0.3]
        pc.intensity = [100, 150, 200]
        pc.classification = [0, 1, 0]
        pc.pointCount = 3

        var frame = FrameBundle()
        frame.frameType = .full
        frame.pointCloud = pc

        renderer.updateFrame(frame)
        XCTAssertEqual(renderer.pointCount, 3)
    }

    func testUpdateFrameWithTracks() throws {
        let renderer = try createRenderer()

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 0,
            tracks: [
                Track(
                    trackID: "track-001", state: .confirmed, x: 10.0, y: 20.0, z: 0.8,
                    bboxLengthAvg: 4.0, bboxWidthAvg: 1.8, bboxHeightAvg: 1.5),
                Track(
                    trackID: "track-002", state: .tentative, x: 30.0, y: 40.0, z: 0.9,
                    bboxLengthAvg: 2.0, bboxWidthAvg: 1.0, bboxHeightAvg: 1.0),
            ], trails: [])

        renderer.updateFrame(frame)
        XCTAssertEqual(renderer.boxInstanceCount, 2)
    }

    func testUpdateFrameWithClusters() throws {
        let renderer = try createRenderer()

        var frame = FrameBundle()
        frame.clusters = ClusterSet(
            frameID: 1, timestampNanos: 0,
            clusters: [
                Cluster(
                    clusterID: 1, centroidX: 10.0, centroidY: 20.0, centroidZ: 0.8, aabbLength: 3.0,
                    aabbWidth: 1.5, aabbHeight: 1.2),
                Cluster(
                    clusterID: 2, centroidX: 30.0, centroidY: 40.0, centroidZ: 0.9,
                    obb: OrientedBoundingBox(
                        centerX: 30.0, centerY: 40.0, centerZ: 0.9, length: 4.0, width: 2.0,
                        height: 1.5, headingRad: 0.5)),
            ], method: .dbscan)

        renderer.updateFrame(frame)
        XCTAssertEqual(renderer.clusterInstanceCount, 2)
    }

    func testUpdateFrameWithTrails() throws {
        let renderer = try createRenderer()

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 0, tracks: [],
            trails: [
                TrackTrail(
                    trackID: "track-001",
                    points: [
                        TrackPoint(x: 10.0, y: 20.0, timestampNanos: 100),
                        TrackPoint(x: 11.0, y: 21.0, timestampNanos: 200),
                        TrackPoint(x: 12.0, y: 22.0, timestampNanos: 300),
                    ])
            ])

        renderer.updateFrame(frame)
        XCTAssertEqual(renderer.trailSegments.count, 1)
        XCTAssertEqual(renderer.trailSegments[0].count, 3)
    }

    func testUpdateFrameWithSinglePointTrail() throws {
        let renderer = try createRenderer()

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 0, tracks: [],
            trails: [
                TrackTrail(
                    trackID: "track-001",
                    points: [TrackPoint(x: 10.0, y: 20.0, timestampNanos: 100)])// Only 1 point — not enough for a trail segment
            ])

        renderer.updateFrame(frame)
        XCTAssertTrue(renderer.trailSegments.isEmpty)
    }

    func testUpdateFrameWithDebugOverlays() throws {
        let renderer = try createRenderer()
        renderer.showDebug = true
        renderer.showAssociation = true
        renderer.showResiduals = true
        renderer.showGating = true

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 0,
            tracks: [Track(trackID: "track-001", state: .confirmed, x: 10.0, y: 20.0, z: 1.0)],
            trails: [])
        frame.debug = DebugOverlaySet(
            frameID: 1, timestampNanos: 0,
            associationCandidates: [
                AssociationCandidate(
                    clusterID: 1, trackID: "track-001", distance: 2.0, accepted: true)
            ],
            gatingEllipses: [
                GatingEllipse(
                    trackID: "track-001", centerX: 10.0, centerY: 20.0, semiMajor: 5.0,
                    semiMinor: 3.0, rotationRad: 0.2)
            ],
            residuals: [
                InnovationResidual(
                    trackID: "track-001", predictedX: 10.0, predictedY: 20.0, measuredX: 10.5,
                    measuredY: 20.3, residualMagnitude: 0.58)
            ], predictions: [])

        renderer.updateFrame(frame)
        XCTAssertGreaterThan(renderer.debugLineVertexCount, 0)
        XCTAssertGreaterThan(renderer.ellipseVertexCount, 0)
    }

    func testUpdateFrameDebugDisabled() throws {
        let renderer = try createRenderer()
        renderer.showDebug = false

        var frame = FrameBundle()
        frame.debug = DebugOverlaySet(
            frameID: 1, timestampNanos: 0,
            associationCandidates: [
                AssociationCandidate(
                    clusterID: 1, trackID: "track-001", distance: 2.0, accepted: true)
            ], gatingEllipses: [], residuals: [], predictions: [])

        renderer.updateFrame(frame)
        XCTAssertEqual(renderer.debugLineVertexCount, 0)
        XCTAssertEqual(renderer.ellipseVertexCount, 0)
    }

    func testUpdateFrameWithHeadingArrows() throws {
        let renderer = try createRenderer()

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 0,
            tracks: [
                Track(
                    trackID: "track-001", state: .confirmed, x: 10.0, y: 20.0, z: 0.8,
                    headingRad: Float.pi / 4, bboxLengthAvg: 4.0, bboxWidthAvg: 1.8,
                    bboxHeadingRad: Float.pi / 3)
            ], trails: [])

        renderer.updateFrame(frame)
        XCTAssertGreaterThan(renderer.headingArrowVertexCount, 0)
    }

    func testUpdateFrameWithSelectedTrack() throws {
        let renderer = try createRenderer()
        renderer.selectedTrackID = "track-001"

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 0,
            tracks: [
                Track(
                    trackID: "track-001", state: .confirmed, x: 10.0, y: 20.0, bboxLengthAvg: 4.0,
                    bboxWidthAvg: 1.8, bboxHeightAvg: 1.5)
            ], trails: [])

        renderer.updateFrame(frame)
        // Selected track should get white colour — just verify no crash
        XCTAssertEqual(renderer.boxInstanceCount, 1)
    }

    func testUpdateFrameForegroundType() throws {
        let renderer = try createRenderer()

        var pc = PointCloudFrame()
        pc.x = [1.0, 2.0]
        pc.y = [3.0, 4.0]
        pc.z = [0.5, 0.6]
        pc.intensity = [100, 150]
        pc.classification = [1, 1]
        pc.pointCount = 2

        var frame = FrameBundle()
        frame.frameType = .foreground  // Not .full — legacy path skipped
        frame.pointCloud = pc

        renderer.updateFrame(frame)
        // Point count from legacy path should be 0 (foreground goes to composite)
        XCTAssertEqual(renderer.pointCount, 0)
    }
}

// MARK: - MetalRenderer Hit Test & Projection Tests

final class MetalRendererHitTestTests: XCTestCase {

    private func createRenderer() throws -> MetalRenderer {
        guard let device = MTLCreateSystemDefaultDevice() else {
            throw XCTSkip("Metal not available")
        }
        let metalView = MTKView()
        metalView.device = device
        guard let renderer = MetalRenderer(metalView: metalView) else {
            throw XCTSkip("Could not create MetalRenderer")
        }
        return renderer
    }

    func testHitTestTrackNoTracks() throws {
        let renderer = try createRenderer()

        let result = renderer.hitTestTrack(
            at: CGPoint(x: 400, y: 300), viewSize: CGSize(width: 800, height: 600))
        XCTAssertNil(result, "Should return nil when no tracks")
    }

    func testHitTestTrackWithTracks() throws {
        let renderer = try createRenderer()

        // Set up tracks by updating a frame
        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 0,
            tracks: [Track(trackID: "track-001", state: .confirmed, x: 0.0, y: 10.0, z: 0.0)],
            trails: [])
        renderer.updateFrame(frame)

        // Try hit testing — the result depends on camera projection
        let _ = renderer.hitTestTrack(
            at: CGPoint(x: 400, y: 300), viewSize: CGSize(width: 800, height: 600))
        // Just verify no crash
    }

    func testProjectTrackLabelsNoTracks() throws {
        let renderer = try createRenderer()

        let labels = renderer.projectTrackLabels(viewSize: CGSize(width: 800, height: 600))
        XCTAssertTrue(labels.isEmpty, "Should return empty when no tracks")
    }

    func testProjectTrackLabelsWithTracks() throws {
        let renderer = try createRenderer()

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 0,
            tracks: [
                Track(
                    trackID: "track-001", state: .confirmed, x: 0.0, y: 10.0, z: 1.0,
                    classLabel: "car"),
                Track(trackID: "track-002", state: .tentative, x: 5.0, y: 15.0, z: 0.8),
                Track(trackID: "track-003", state: .deleted, x: 20.0, y: 30.0, z: 0.5),
                Track(trackID: "track-004", state: .unknown, x: -5.0, y: 5.0, z: 0.3),
            ], trails: [])
        renderer.updateFrame(frame)

        let labels = renderer.projectTrackLabels(viewSize: CGSize(width: 800, height: 600))
        // Deleted and unknown tracks should be filtered out
        // Only confirmed and tentative are shown
        XCTAssertLessThanOrEqual(labels.count, 2, "Deleted and unknown tracks should be excluded")
    }

    func testProjectTrackLabelsSelectedTrack() throws {
        let renderer = try createRenderer()
        renderer.selectedTrackID = "track-001"

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 0,
            tracks: [
                Track(
                    trackID: "track-001", state: .confirmed, x: 0.0, y: 10.0, z: 1.0,
                    classLabel: "car")
            ], trails: [])
        renderer.updateFrame(frame)

        let labels = renderer.projectTrackLabels(viewSize: CGSize(width: 800, height: 600))
        if let label = labels.first {
            XCTAssertTrue(label.isSelected, "Selected track label should be marked as selected")
        }
    }

    func testGetCacheStatus() throws {
        let renderer = try createRenderer()

        let status = renderer.getCacheStatus()
        // Should return something (composite renderer is initialised in init)
        XCTAssertFalse(status.isEmpty)
    }

    func testGetPointCloudStats() throws {
        let renderer = try createRenderer()

        let stats = renderer.getPointCloudStats()
        XCTAssertEqual(stats.total, 0)
        XCTAssertEqual(stats.background, 0)
        XCTAssertEqual(stats.foreground, 0)
    }

    func testGetPointCloudStatsAfterUpdate() throws {
        let renderer = try createRenderer()

        var pc = PointCloudFrame()
        pc.x = [1.0, 2.0, 3.0]
        pc.y = [4.0, 5.0, 6.0]
        pc.z = [0.1, 0.2, 0.3]
        pc.intensity = [100, 150, 200]
        pc.classification = [0, 1, 0]
        pc.pointCount = 3

        var frame = FrameBundle()
        frame.frameType = .full
        frame.pointCloud = pc
        renderer.updateFrame(frame)

        let stats = renderer.getPointCloudStats()
        XCTAssertGreaterThanOrEqual(stats.total, 3)
    }

    func testMtkViewDrawableSizeWillChange() throws {
        let renderer = try createRenderer()
        let metalView = MTKView()

        renderer.mtkView(metalView, drawableSizeWillChange: CGSize(width: 1920, height: 1080))
        XCTAssertEqual(renderer.camera.aspectRatio, Float(1920.0 / 1080.0), accuracy: 0.001)
    }
}

// MARK: - MetalRenderer Settings Tests

final class MetalRendererSettingsTests: XCTestCase {

    private func createRenderer() throws -> MetalRenderer {
        guard let device = MTLCreateSystemDefaultDevice() else {
            throw XCTSkip("Metal not available")
        }
        let metalView = MTKView()
        metalView.device = device
        guard let renderer = MetalRenderer(metalView: metalView) else {
            throw XCTSkip("Could not create MetalRenderer")
        }
        return renderer
    }

    func testDefaultSettings() throws {
        let renderer = try createRenderer()

        XCTAssertTrue(renderer.showPoints)
        XCTAssertTrue(renderer.showBackground)
        XCTAssertTrue(renderer.showBoxes)
        XCTAssertTrue(renderer.showClusters)
        XCTAssertTrue(renderer.showTrails)
        XCTAssertFalse(renderer.showDebug)
        XCTAssertFalse(renderer.showGating)
        XCTAssertFalse(renderer.showAssociation)
        XCTAssertFalse(renderer.showResiduals)
        XCTAssertEqual(renderer.pointSize, 5.0)
        XCTAssertNil(renderer.selectedTrackID)
    }

    func testSettingsModification() throws {
        let renderer = try createRenderer()

        renderer.showPoints = false
        renderer.showBackground = false
        renderer.showBoxes = false
        renderer.showClusters = false
        renderer.showTrails = false
        renderer.showDebug = true
        renderer.showGating = true
        renderer.showAssociation = true
        renderer.showResiduals = true
        renderer.pointSize = 15.0
        renderer.selectedTrackID = "track-001"

        XCTAssertFalse(renderer.showPoints)
        XCTAssertFalse(renderer.showBackground)
        XCTAssertFalse(renderer.showBoxes)
        XCTAssertFalse(renderer.showClusters)
        XCTAssertFalse(renderer.showTrails)
        XCTAssertTrue(renderer.showDebug)
        XCTAssertTrue(renderer.showGating)
        XCTAssertTrue(renderer.showAssociation)
        XCTAssertTrue(renderer.showResiduals)
        XCTAssertEqual(renderer.pointSize, 15.0)
        XCTAssertEqual(renderer.selectedTrackID, "track-001")
    }

    func testCompositeRendererInitialised() throws {
        let renderer = try createRenderer()
        XCTAssertNotNil(renderer.compositeRenderer)
    }
}

// MARK: - MetalRenderer Cluster with OBB vs AABB Tests

final class MetalRendererClusterOBBTests: XCTestCase {

    private func createRenderer() throws -> MetalRenderer {
        guard let device = MTLCreateSystemDefaultDevice() else {
            throw XCTSkip("Metal not available")
        }
        let metalView = MTKView()
        metalView.device = device
        guard let renderer = MetalRenderer(metalView: metalView) else {
            throw XCTSkip("Could not create MetalRenderer")
        }
        return renderer
    }

    func testClusterWithOBBUsesOBBTransform() throws {
        let renderer = try createRenderer()

        var frame = FrameBundle()
        frame.clusters = ClusterSet(
            frameID: 1, timestampNanos: 0,
            clusters: [
                Cluster(
                    clusterID: 1, centroidX: 10.0, centroidY: 20.0, centroidZ: 1.0,
                    obb: OrientedBoundingBox(
                        centerX: 10.0, centerY: 20.0, centerZ: 1.0, length: 5.0, width: 2.0,
                        height: 1.5, headingRad: Float.pi / 4))
            ], method: .dbscan)

        renderer.updateFrame(frame)
        XCTAssertEqual(renderer.clusterInstanceCount, 1)
    }

    func testClusterWithoutOBBUsesAABB() throws {
        let renderer = try createRenderer()

        var frame = FrameBundle()
        frame.clusters = ClusterSet(
            frameID: 1, timestampNanos: 0,
            clusters: [
                Cluster(
                    clusterID: 1, centroidX: 10.0, centroidY: 20.0, centroidZ: 1.0, aabbLength: 3.0,
                    aabbWidth: 1.5, aabbHeight: 1.0)
            ], method: .dbscan)

        renderer.updateFrame(frame)
        XCTAssertEqual(renderer.clusterInstanceCount, 1)
    }

    func testEmptyClusters() throws {
        let renderer = try createRenderer()

        var frame = FrameBundle()
        frame.clusters = ClusterSet(frameID: 1, timestampNanos: 0, clusters: [], method: .dbscan)

        renderer.updateFrame(frame)
        XCTAssertEqual(renderer.clusterInstanceCount, 0)
    }
}

// MARK: - MetalRenderer Heading Arrow Tests

final class MetalRendererHeadingArrowTests: XCTestCase {

    private func createRenderer() throws -> MetalRenderer {
        guard let device = MTLCreateSystemDefaultDevice() else {
            throw XCTSkip("Metal not available")
        }
        let metalView = MTKView()
        metalView.device = device
        guard let renderer = MetalRenderer(metalView: metalView) else {
            throw XCTSkip("Could not create MetalRenderer")
        }
        return renderer
    }

    func testHeadingArrowsFromTrack() throws {
        let renderer = try createRenderer()

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 0,
            tracks: [
                Track(
                    trackID: "t1", state: .confirmed, x: 10.0, y: 20.0, z: 0.8,
                    headingRad: Float.pi / 4, bboxLengthAvg: 4.0)
            ], trails: [])

        renderer.updateFrame(frame)
        XCTAssertGreaterThan(renderer.headingArrowVertexCount, 0)
    }

    func testHeadingArrowsFromClusterOBB() throws {
        let renderer = try createRenderer()
        renderer.showClusters = true

        var frame = FrameBundle()
        frame.clusters = ClusterSet(
            frameID: 1, timestampNanos: 0,
            clusters: [
                Cluster(
                    clusterID: 1, centroidX: 10.0, centroidY: 20.0, centroidZ: 1.0,
                    obb: OrientedBoundingBox(
                        centerX: 10.0, centerY: 20.0, centerZ: 1.0, length: 3.0, width: 1.5,
                        height: 1.0, headingRad: Float.pi / 6))
            ], method: .dbscan)

        renderer.updateFrame(frame)
        XCTAssertGreaterThan(renderer.headingArrowVertexCount, 0)
    }

    func testNoHeadingArrowsWithZeroHeading() throws {
        let renderer = try createRenderer()

        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 0,
            tracks: [
                Track(
                    trackID: "t1", state: .confirmed, x: 10.0, y: 20.0, headingRad: 0.0,
                    bboxHeadingRad: 0.0)
            ], trails: [])

        renderer.updateFrame(frame)
        // With both headings at zero, the guard skips this track
        XCTAssertEqual(renderer.headingArrowVertexCount, 0)
    }
}

// MARK: - TrackScreenLabel Tests

struct TrackScreenLabelTests {
    @Test func trackScreenLabelProperties() throws {
        let label = MetalRenderer.TrackScreenLabel(
            id: "track-001", screenX: 400.0, screenY: 300.0, classLabel: "car", isSelected: false)

        #expect(label.id == "track-001")
        #expect(label.screenX == 400.0)
        #expect(label.screenY == 300.0)
        #expect(label.classLabel == "car")
        #expect(label.isSelected == false)
    }

    @Test func trackScreenLabelSelected() throws {
        let label = MetalRenderer.TrackScreenLabel(
            id: "track-002", screenX: 200.0, screenY: 150.0, classLabel: "pedestrian",
            isSelected: true)

        #expect(label.isSelected == true)
        #expect(label.classLabel == "pedestrian")
    }

    @Test func trackScreenLabelEmptyClassLabel() throws {
        let label = MetalRenderer.TrackScreenLabel(
            id: "track-003", screenX: 0.0, screenY: 0.0, classLabel: "", isSelected: false)

        #expect(label.classLabel.isEmpty)
    }
}
