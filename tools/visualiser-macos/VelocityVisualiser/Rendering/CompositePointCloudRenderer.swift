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
import QuartzCore

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

    // Crossfade transition state: when a new background arrives, we blend
    // from the previous snapshot to the new one over transitionDuration seconds.
    // Three buffers during transition:
    //   previousBackgroundBuffer — old positions (source, read-only)
    //   backgroundBuffer — new target positions (read-only during transition)
    //   blendBuffer — interpolated positions written each frame for rendering
    private var previousBackgroundBuffer: MTLBuffer?
    private var previousBackgroundPointCount: Int = 0
    private var blendBuffer: MTLBuffer?
    private var blendBufferCapacity: Int = 0
    private var transitionStartTime: Double = 0  // CACurrentMediaTime
    private var transitionDuration: Double = 0.5  // seconds
    private var isTransitioning: Bool = false

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

    /// Optional closure to test if a foreground point should be included.
    /// When set, foreground points are skipped unless the closure returns true.
    /// Parameters: (x: Float, y: Float) -> Bool
    var foregroundPointFilter: ((Float, Float) -> Bool)?

    // MARK: - Initialisation

    init(device: MTLDevice) { self.device = device }

    // MARK: - Frame Processing

    /// Process a frame bundle and update buffers accordingly.
    /// - Parameter frame: The frame bundle to process
    func processFrame(_ frame: FrameBundle) {
        let trace = PerformanceTrace.begin(
            "CompositeProcessFrame",
            "frame=\(frame.frameID) type=\(frame.frameType.rawValue) bgSeq=\(frame.backgroundSeq)")
        defer {
            trace.end(
                "bgPoints=\(backgroundPointCount) fgPoints=\(foregroundPointCount) cache=\(cacheStatus)"
            )
        }

        switch frame.frameType {
        case .full:
            // Legacy mode: treat point cloud as foreground
            if let pointCloud = frame.pointCloud { updateForegroundBuffer(pointCloud) }
            // Also ingest background data when present (e.g. first VRLOG frame)
            if let background = frame.background {
                updateBackgroundBuffer(background)
                backgroundSeq = background.sequenceNumber
                cacheState = .cached(seq: backgroundSeq)
            }

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
    /// Crossfade: preserves the previous background for smooth blending.
    private func updateBackgroundBuffer(_ snapshot: BackgroundSnapshot) {
        let count = snapshot.pointCount
        let trace = PerformanceTrace.begin(
            "UpdateBackgroundBuffer", "seq=\(snapshot.sequenceNumber) points=\(count)")
        defer { trace.end("used=\(backgroundPointCount) capacity=\(backgroundBufferCapacity / 5)") }

        guard count > 0 else {
            backgroundPointCount = 0
            return
        }

        cacheState = .refreshing

        // Preserve previous background for crossfade transition.
        // Only transition if we already had a rendered background.
        if backgroundPointCount > 0, let existingBuffer = backgroundBuffer {
            previousBackgroundBuffer = existingBuffer
            previousBackgroundPointCount = backgroundPointCount
            transitionStartTime = CACurrentMediaTime()
            isTransitioning = true
            // Force new allocation so old buffer stays valid during transition
            backgroundBuffer = nil
            backgroundBufferCapacity = 0
        }

        // M7: Check if we need to reallocate the buffer
        let neededVertices = count * 5  // 5 floats per vertex
        if backgroundBuffer == nil
            || shouldReallocate(
                currentCapacity: backgroundBufferCapacity, neededCount: neededVertices)
        {
            let newCapacity = calculateCapacity(for: neededVertices)
            let bufferSize = newCapacity * MemoryLayout<Float>.stride
            PerformanceTrace.event(
                "BackgroundBufferReallocated",
                "seq=\(snapshot.sequenceNumber) oldCapacity=\(backgroundBufferCapacity / 5) newCapacity=\(newCapacity / 5)"
            )
            if let newBuffer = device.makeBuffer(length: bufferSize, options: .storageModeShared) {
                backgroundBuffer = newBuffer
                backgroundBufferCapacity = newCapacity
            } else {
                // Allocation failed; keep state consistent and abort update
                backgroundBuffer = nil
                backgroundBufferCapacity = 0
                backgroundPointCount = 0
                return
            }
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
        let trace = PerformanceTrace.begin(
            "UpdateForegroundBuffer", "frame=\(pointCloud.frameID) points=\(count)")
        defer { trace.end("used=\(foregroundPointCount) capacity=\(foregroundBufferCapacity / 5)") }

        guard count > 0 else {
            foregroundPointCount = 0
            return
        }

        // M7: Check if we need to reallocate the buffer
        let neededVertices = count * 5  // 5 floats per vertex
        if foregroundBuffer == nil
            || shouldReallocate(
                currentCapacity: foregroundBufferCapacity, neededCount: neededVertices)
        {
            let newCapacity = calculateCapacity(for: neededVertices)
            let bufferSize = newCapacity * MemoryLayout<Float>.stride
            if let newBuffer = device.makeBuffer(length: bufferSize, options: .storageModeShared) {
                foregroundBuffer = newBuffer
                foregroundBufferCapacity = newCapacity
            } else {
                // Allocation failed; keep state consistent and abort update
                foregroundBuffer = nil
                foregroundBufferCapacity = 0
                foregroundPointCount = 0
                return
            }
        }

        // Copy data into buffer, optionally filtering points
        guard let buffer = foregroundBuffer else { return }
        let ptr = buffer.contents().bindMemory(to: Float.self, capacity: neededVertices)

        var outputCount = 0
        for i in 0..<count {
            // Apply foreground point filter if set
            if let filter = foregroundPointFilter {
                let px = pointCloud.x[i]
                let py = pointCloud.y[i]
                var classification: Float = 1.0
                if i < pointCloud.classification.count {
                    classification = Float(pointCloud.classification[i])
                }
                // Only filter foreground points (classification=1), pass background/ground through
                if classification == 1.0 && !filter(px, py) { continue }
            }

            ptr[outputCount * 5 + 0] = pointCloud.x[i]
            ptr[outputCount * 5 + 1] = pointCloud.y[i]
            ptr[outputCount * 5 + 2] = pointCloud.z[i]
            ptr[outputCount * 5 + 3] = Float(pointCloud.intensity[i]) / 255.0
            // Classification: 0=background, 1=foreground, 2=ground
            var classification: Float = 1.0  // Default to foreground
            if i < pointCloud.classification.count {
                classification = Float(pointCloud.classification[i])
            }
            ptr[outputCount * 5 + 4] = classification
            outputCount += 1
        }

        foregroundPointCount = outputCount
    }

    // MARK: - Rendering

    /// Advance crossfade transition if active. Called before rendering to
    /// interpolate background positions from previous → current snapshot
    /// into a blend buffer. Returns true if the transition is still in
    /// progress (needs continued redraws).
    func advanceTransition() -> Bool {
        guard isTransitioning, let prevBuffer = previousBackgroundBuffer,
            let targetBuffer = backgroundBuffer,
            previousBackgroundPointCount == backgroundPointCount, backgroundPointCount > 0
        else {
            if isTransitioning {
                // Point counts differ (grid reset) — snap immediately
                isTransitioning = false
                previousBackgroundBuffer = nil
                previousBackgroundPointCount = 0
                blendBuffer = nil
                blendBufferCapacity = 0
            }
            return false
        }

        let elapsed = CACurrentMediaTime() - transitionStartTime
        let t = Float(min(elapsed / transitionDuration, 1.0))

        let count = backgroundPointCount
        let vertexFloats = count * 5

        // Ensure blend buffer is allocated
        if blendBuffer == nil || blendBufferCapacity < vertexFloats {
            let capacity = calculateCapacity(for: vertexFloats)
            let bufferSize = capacity * MemoryLayout<Float>.stride
            if let buf = device.makeBuffer(length: bufferSize, options: .storageModeShared) {
                blendBuffer = buf
                blendBufferCapacity = capacity
            } else {
                isTransitioning = false
                return false
            }
        }

        let prevPtr = prevBuffer.contents().bindMemory(to: Float.self, capacity: vertexFloats)
        let targetPtr = targetBuffer.contents().bindMemory(to: Float.self, capacity: vertexFloats)
        let blendPtr = blendBuffer!.contents().bindMemory(to: Float.self, capacity: vertexFloats)

        // Lerp x, y, z positions (indices 0, 1, 2) from previous → target.
        // Copy intensity (3) and classification (4) from target.
        for i in 0..<count {
            let base = i * 5
            blendPtr[base + 0] = prevPtr[base + 0] + t * (targetPtr[base + 0] - prevPtr[base + 0])
            blendPtr[base + 1] = prevPtr[base + 1] + t * (targetPtr[base + 1] - prevPtr[base + 1])
            blendPtr[base + 2] = prevPtr[base + 2] + t * (targetPtr[base + 2] - prevPtr[base + 2])
            blendPtr[base + 3] = targetPtr[base + 3]
            blendPtr[base + 4] = targetPtr[base + 4]
        }

        if t >= 1.0 {
            isTransitioning = false
            previousBackgroundBuffer = nil
            previousBackgroundPointCount = 0
            blendBuffer = nil
            blendBufferCapacity = 0
            return false
        }

        return true
    }

    /// Render background and/or foreground buffers.
    /// - Parameters:
    ///   - encoder: The render command encoder
    ///   - pipeline: The point cloud render pipeline
    ///   - uniforms: The uniform buffer (passed as inout for efficiency)
    ///   - drawBackground: Whether to draw background points (K toggle)
    ///   - drawForeground: Whether to draw foreground points (F toggle)
    func render(
        encoder: MTLRenderCommandEncoder, pipeline: MTLRenderPipelineState,
        uniforms: inout MetalRenderer.Uniforms, drawBackground: Bool = true,
        drawForeground: Bool = true
    ) {
        encoder.setRenderPipelineState(pipeline)

        // Draw background first (if cached and enabled)
        if drawBackground, backgroundPointCount > 0 {
            // During crossfade, render from the blend buffer instead
            let renderBuffer =
                (isTransitioning ? blendBuffer : backgroundBuffer) ?? backgroundBuffer
            if let bgBuffer = renderBuffer {
                encoder.setVertexBuffer(bgBuffer, offset: 0, index: 0)
                encoder.setVertexBytes(
                    &uniforms, length: MemoryLayout<MetalRenderer.Uniforms>.stride, index: 1)
                encoder.drawPrimitives(
                    type: .point, vertexStart: 0, vertexCount: backgroundPointCount)
            }
        }

        // Draw foreground on top (if enabled)
        if drawForeground, let fgBuffer = foregroundBuffer, foregroundPointCount > 0 {
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
        previousBackgroundBuffer = nil
        previousBackgroundPointCount = 0
        blendBuffer = nil
        blendBufferCapacity = 0
        isTransitioning = false
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
