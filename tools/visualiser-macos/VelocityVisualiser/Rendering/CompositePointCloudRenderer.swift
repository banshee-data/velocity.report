// CompositePointCloudRenderer.swift
// Renderer that handles split streaming with cached background.
//
// For M3.5 optimisation:
// - Background frames are cached and reused
// - Foreground frames are rendered over the cached background
// - Reduces bandwidth from ~80 Mbps to ~3 Mbps
//
// For M7 optimisation:
// - Pre-allocated buffer reuse to reduce allocation pressure
// - Buffers are reused when point count fits within capacity
// - Reduces GC pressure at 10-20 fps with 70k points

import Metal
import MetalKit

/// Cache state for background point cloud.
enum BackgroundCacheState {
    case empty  // No background cached
    case cached(seq: UInt64)  // Background cached with sequence number
    case refreshing  // Background update in progress

    var description: String {
        switch self {
        case .empty: return "Empty"
        case .cached(let seq): return "Cached (seq: \(seq))"
        case .refreshing: return "Refreshing..."
        }
    }
}

/// Renderer that handles split streaming with cached background.
///
/// This renderer maintains two separate Metal buffers:
/// - Background buffer: Cached from background frames, reused across foreground frames
/// - Foreground buffer: Updated on every foreground frame
///
/// M7: Uses pre-allocated buffers to reduce allocation pressure. Buffers are reused
/// when the new point count fits within the existing capacity. A new buffer is only
/// allocated when capacity is insufficient or excessively large (>4x needed).
///
/// When rendering, both buffers are drawn in sequence to composite the full scene.
class CompositePointCloudRenderer {

    // MARK: - Properties

    private let device: MTLDevice

    // M7: Buffer capacity tracking for pre-allocation reuse.
    // We allocate larger buffers and reuse them when point count varies.
    // Reallocation thresholds:
    // - Grow: when needed capacity exceeds current buffer
    // - Shrink: when buffer is >4x larger than needed (avoid memory waste)
    private static let shrinkThreshold: Int = 4
    private static let growMargin: Float = 1.5  // Allocate 50% extra for headroom

    // Cached background buffer
    private var backgroundBuffer: MTLBuffer?
    private var backgroundBufferCapacity: Int = 0  // M7: Capacity in vertices (not bytes)
    private var backgroundPointCount: Int = 0
    private var backgroundSeq: UInt64 = 0

    // Current foreground buffer
    private var foregroundBuffer: MTLBuffer?
    private var foregroundBufferCapacity: Int = 0  // M7: Capacity in vertices
    private var foregroundPointCount: Int = 0

    // Cache state tracking
    private(set) var cacheState: BackgroundCacheState = .empty

    /// Returns true if the cache needs refresh (sequence mismatch).
    var isCacheStale: Bool {
        switch cacheState {
        case .empty, .refreshing: return true
        case .cached: return false
        }
    }

    /// Human-readable cache status for UI display.
    var cacheStatus: String { cacheState.description }

    // MARK: - Initialisation

    init(device: MTLDevice) { self.device = device }

    // MARK: - Frame Processing

    /// Process a frame bundle and update buffers accordingly.
    /// - Parameter frame: The frame bundle to process
    func processFrame(_ frame: FrameBundle) {
        switch frame.frameType {
        case .full:
            // Legacy mode: treat as foreground only
            if let pointCloud = frame.pointCloud { updateForegroundBuffer(pointCloud) }

        case .foreground:
            // M3.5 mode: foreground points only
            if let pointCloud = frame.pointCloud { updateForegroundBuffer(pointCloud) }

            // Check if background cache is valid
            if frame.backgroundSeq != backgroundSeq { cacheState = .empty }

        case .background:
            // M3.5 mode: background snapshot
            if let background = frame.background {
                updateBackgroundBuffer(background)
                backgroundSeq = background.sequenceNumber
                cacheState = .cached(seq: backgroundSeq)
            }

        case .delta:
            // Future: incremental update (not implemented yet)
            break
        }
    }

    // MARK: - M7 Buffer Management

    /// Determine if a buffer should be reallocated.
    /// Returns true if the buffer needs to grow or should shrink to avoid waste.
    private func shouldReallocate(currentCapacity: Int, neededCount: Int) -> Bool {
        // Need to grow: current capacity insufficient
        if neededCount > currentCapacity { return true }
        // Should shrink: buffer is excessively large (>4x needed)
        if currentCapacity > neededCount * Self.shrinkThreshold && neededCount > 0 { return true }
        return false
    }

    /// Calculate new buffer capacity with growth margin.
    private func calculateCapacity(for count: Int) -> Int {
        // Add 50% growth margin to reduce reallocations
        return Int(Float(count) * Self.growMargin)
    }

    /// Update the background buffer from a snapshot.
    /// M7: Reuses existing buffer when capacity permits.
    private func updateBackgroundBuffer(_ snapshot: BackgroundSnapshot) {
        let count = snapshot.pointCount
        guard count > 0 else {
            backgroundPointCount = 0
            return
        }

        cacheState = .refreshing

        // M7: Check if we need to reallocate the buffer
        let neededVertices = count * 5  // 5 floats per vertex
        if shouldReallocate(currentCapacity: backgroundBufferCapacity, neededCount: neededVertices)
        {
            let newCapacity = calculateCapacity(for: neededVertices)
            let bufferSize = newCapacity * MemoryLayout<Float>.stride
            backgroundBuffer = device.makeBuffer(length: bufferSize, options: .storageModeShared)
            backgroundBufferCapacity = newCapacity
        }

        // Copy data into buffer
        guard let buffer = backgroundBuffer else { return }
        let ptr = buffer.contents().bindMemory(to: Float.self, capacity: neededVertices)

        for i in 0..<count {
            ptr[i * 5 + 0] = snapshot.x[i]
            ptr[i * 5 + 1] = snapshot.y[i]
            ptr[i * 5 + 2] = snapshot.z[i]
            // Use confidence as intensity (scaled down from count to 0-1 range)
            let confidence = Float(snapshot.confidence[i])
            ptr[i * 5 + 3] = min(confidence / 10.0, 1.0)
            ptr[i * 5 + 4] = 0.0  // Classification: background
        }

        backgroundPointCount = count
    }

    /// Update the foreground buffer from a point cloud frame.
    /// M7: Reuses existing buffer when capacity permits.
    private func updateForegroundBuffer(_ pointCloud: PointCloudFrame) {
        let count = pointCloud.pointCount
        guard count > 0 else {
            foregroundPointCount = 0
            return
        }

        // M7: Check if we need to reallocate the buffer
        let neededVertices = count * 5  // 5 floats per vertex
        if shouldReallocate(currentCapacity: foregroundBufferCapacity, neededCount: neededVertices)
        {
            let newCapacity = calculateCapacity(for: neededVertices)
            let bufferSize = newCapacity * MemoryLayout<Float>.stride
            foregroundBuffer = device.makeBuffer(length: bufferSize, options: .storageModeShared)
            foregroundBufferCapacity = newCapacity
        }

        // Copy data into buffer
        guard let buffer = foregroundBuffer else { return }
        let ptr = buffer.contents().bindMemory(to: Float.self, capacity: neededVertices)

        for i in 0..<count {
            ptr[i * 5 + 0] = pointCloud.x[i]
            ptr[i * 5 + 1] = pointCloud.y[i]
            ptr[i * 5 + 2] = pointCloud.z[i]
            ptr[i * 5 + 3] = Float(pointCloud.intensity[i]) / 255.0
            // Classification: 0=background, 1=foreground, 2=ground
            var classification: Float = 1.0  // Default to foreground
            if i < pointCloud.classification.count {
                classification = Float(pointCloud.classification[i])
            }
            ptr[i * 5 + 4] = classification
        }

        foregroundPointCount = count
    }

    // MARK: - Rendering

    /// Render both background and foreground buffers.
    /// - Parameters:
    ///   - encoder: The render command encoder
    ///   - pipeline: The point cloud render pipeline
    ///   - uniforms: The uniform buffer (passed as inout for efficiency)
    func render(
        encoder: MTLRenderCommandEncoder, pipeline: MTLRenderPipelineState,
        uniforms: inout MetalRenderer.Uniforms
    ) {
        encoder.setRenderPipelineState(pipeline)

        // Draw background first (if cached)
        if let bgBuffer = backgroundBuffer, backgroundPointCount > 0 {
            encoder.setVertexBuffer(bgBuffer, offset: 0, index: 0)
            encoder.setVertexBytes(
                &uniforms, length: MemoryLayout<MetalRenderer.Uniforms>.stride, index: 1)
            encoder.drawPrimitives(type: .point, vertexStart: 0, vertexCount: backgroundPointCount)
        }

        // Draw foreground on top
        if let fgBuffer = foregroundBuffer, foregroundPointCount > 0 {
            encoder.setVertexBuffer(fgBuffer, offset: 0, index: 0)
            encoder.setVertexBytes(
                &uniforms, length: MemoryLayout<MetalRenderer.Uniforms>.stride, index: 1)
            encoder.drawPrimitives(type: .point, vertexStart: 0, vertexCount: foregroundPointCount)
        }
    }

    /// Clear all cached data.
    func clearCache() {
        backgroundBuffer = nil
        backgroundBufferCapacity = 0
        backgroundPointCount = 0
        backgroundSeq = 0
        foregroundBuffer = nil
        foregroundBufferCapacity = 0
        foregroundPointCount = 0
        cacheState = .empty
    }

    // MARK: - Statistics

    /// Get rendering statistics for display/debugging.
    func getStats() -> (background: Int, foreground: Int, total: Int) {
        let total = backgroundPointCount + foregroundPointCount
        return (background: backgroundPointCount, foreground: foregroundPointCount, total: total)
    }

    /// M7: Get buffer statistics for performance monitoring.
    func getBufferStats() -> (bgCapacity: Int, bgUsed: Int, fgCapacity: Int, fgUsed: Int) {
        return (
            bgCapacity: (backgroundBufferCapacity + 4) / 5,  // Convert vertices back to points (ceiling)
            bgUsed: backgroundPointCount, fgCapacity: (foregroundBufferCapacity + 4) / 5,
            fgUsed: foregroundPointCount
        )
    }
}
