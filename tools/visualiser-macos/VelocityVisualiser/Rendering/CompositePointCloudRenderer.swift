// CompositePointCloudRenderer.swift
// Renderer that handles split streaming with cached background.
//
// For M3.5 optimisation:
// - Background frames are cached and reused
// - Foreground frames are rendered over the cached background
// - Reduces bandwidth from ~80 Mbps to ~3 Mbps

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
/// When rendering, both buffers are drawn in sequence to composite the full scene.
class CompositePointCloudRenderer {

    // MARK: - Properties

    private let device: MTLDevice

    // Cached background buffer
    private var backgroundBuffer: MTLBuffer?
    private var backgroundPointCount: Int = 0
    private var backgroundSeq: UInt64 = 0

    // Current foreground buffer
    private var foregroundBuffer: MTLBuffer?
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

    /// Update the background buffer from a snapshot.
    private func updateBackgroundBuffer(_ snapshot: BackgroundSnapshot) {
        let count = snapshot.pointCount
        guard count > 0 else {
            backgroundPointCount = 0
            return
        }

        cacheState = .refreshing

        // Create interleaved buffer: [x, y, z, intensity, classification] per point (5 floats each)
        // Background points are classified as 0 (background)
        var vertices = [Float](repeating: 0, count: count * 5)
        for i in 0..<count {
            vertices[i * 5 + 0] = snapshot.x[i]
            vertices[i * 5 + 1] = snapshot.y[i]
            vertices[i * 5 + 2] = snapshot.z[i]
            // Use confidence as intensity (scaled down from count to 0-1 range)
            let confidence = Float(snapshot.confidence[i])
            vertices[i * 5 + 3] = min(confidence / 10.0, 1.0)  // Normalize to 0-1
            vertices[i * 5 + 4] = 0.0  // Classification: background
        }

        let bufferSize = vertices.count * MemoryLayout<Float>.stride
        backgroundBuffer = device.makeBuffer(
            bytes: vertices, length: bufferSize, options: .storageModeShared)
        backgroundPointCount = count
    }

    /// Update the foreground buffer from a point cloud frame.
    private func updateForegroundBuffer(_ pointCloud: PointCloudFrame) {
        let count = pointCloud.pointCount
        guard count > 0 else {
            foregroundPointCount = 0
            return
        }

        // Create interleaved buffer: [x, y, z, intensity, classification] per point (5 floats each)
        var vertices = [Float](repeating: 0, count: count * 5)
        for i in 0..<count {
            vertices[i * 5 + 0] = pointCloud.x[i]
            vertices[i * 5 + 1] = pointCloud.y[i]
            vertices[i * 5 + 2] = pointCloud.z[i]
            vertices[i * 5 + 3] = Float(pointCloud.intensity[i]) / 255.0
            // Classification: 0=background, 1=foreground, 2=ground
            var classification: Float = 1.0  // Default to foreground
            if i < pointCloud.classification.count {
                classification = Float(pointCloud.classification[i])
            }
            vertices[i * 5 + 4] = classification
        }

        let bufferSize = vertices.count * MemoryLayout<Float>.stride
        foregroundBuffer = device.makeBuffer(
            bytes: vertices, length: bufferSize, options: .storageModeShared)
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
        backgroundPointCount = 0
        backgroundSeq = 0
        foregroundBuffer = nil
        foregroundPointCount = 0
        cacheState = .empty
    }

    // MARK: - Statistics

    /// Get rendering statistics for display/debugging.
    func getStats() -> (background: Int, foreground: Int, total: Int) {
        let total = backgroundPointCount + foregroundPointCount
        return (background: backgroundPointCount, foreground: foregroundPointCount, total: total)
    }
}
