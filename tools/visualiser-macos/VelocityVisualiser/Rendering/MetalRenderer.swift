// MetalRenderer.swift
// Main Metal renderer for point clouds, boxes, and trails.
//
// This renderer uses Metal for efficient GPU-accelerated rendering of:
// - Point clouds as point sprites
// - Bounding boxes as instanced geometry
// - Track trails as polylines
// - 2D overlays for debug visualisation
//
// M7: Uses pre-allocated buffer reuse to reduce allocation pressure
// at 10-20 fps with 70k points per frame.

import MetalKit
import simd

/// Main renderer that coordinates all Metal drawing.
class MetalRenderer: NSObject, MTKViewDelegate {

    // MARK: - Metal Resources

    let device: MTLDevice
    let commandQueue: MTLCommandQueue

    // Render pipelines
    var pointCloudPipeline: MTLRenderPipelineState?
    var boxPipeline: MTLRenderPipelineState?
    var trailPipeline: MTLRenderPipelineState?
    var linePipeline: MTLRenderPipelineState?  // M6: Debug overlay lines

    // Depth stencil
    var depthStencilState: MTLDepthStencilState?

    // MARK: - Uniforms

    struct Uniforms {
        var modelViewProjection: simd_float4x4
        var modelView: simd_float4x4
        var pointSize: Float
        var time: Float
        var padding: simd_float2
    }

    var uniforms = Uniforms(
        modelViewProjection: matrix_identity_float4x4, modelView: matrix_identity_float4x4,
        pointSize: 5.0, time: 0, padding: simd_float2(0, 0))

    // MARK: - Camera

    var camera = Camera()

    // Camera control state
    private var isDragging = false
    private var lastMouseLocation = CGPoint.zero
    private var cameraModifier: CameraModifier = .orbit

    enum CameraModifier {
        case orbit  // Rotate around target
        case pan  // Move parallel to view
    }

    // MARK: - Frame Data

    // M3.5: Composite renderer for split streaming
    var compositeRenderer: CompositePointCloudRenderer?

    // Legacy point buffer (for backwards compatibility)
    var pointBuffer: MTLBuffer?
    var pointCount: Int = 0
    // M7: Buffer capacity tracking for pre-allocation reuse
    private var pointBufferCapacity: Int = 0

    var boxVertices: MTLBuffer?
    var boxVertexCount: Int = 0
    var boxInstances: MTLBuffer?
    var boxInstanceCount: Int = 0

    // M4: Cluster rendering
    var clusterInstances: MTLBuffer?
    var clusterInstanceCount: Int = 0

    var trailVertices: MTLBuffer?
    var trailVertexCount: Int = 0
    var trailSegments: [(start: Int, count: Int)] = []  // Each trail's range in the buffer

    // M5: Heading arrow vertices (rendered as lines showing OBB heading direction)
    var headingArrowVertices: MTLBuffer?
    var headingArrowVertexCount: Int = 0

    // M6: Debug overlay buffers
    var debugLineVertices: MTLBuffer?  // Association lines + residual vectors
    var debugLineVertexCount: Int = 0
    var ellipseVertices: MTLBuffer?  // Gating ellipses
    var ellipseVertexCount: Int = 0
    var ellipseSegments: [(start: Int, count: Int)] = []  // Each ellipse's range

    // MARK: - Settings

    var showPoints: Bool = true
    var showBoxes: Bool = true
    var showClusters: Bool = true  // M4: Toggle for cluster rendering
    var showTrails: Bool = true
    var showDebug: Bool = false  // M6: Master debug overlay toggle
    var showGating: Bool = false  // M6: Gating ellipses
    var showAssociation: Bool = false  // M6: Association lines
    var showResiduals: Bool = false  // M6: Residual vectors
    var pointSize: Float = 5.0
    var backgroundColor: MTLClearColor = MTLClearColor(red: 0.1, green: 0.1, blue: 0.15, alpha: 1.0)

    // M7: Track selection
    var selectedTrackID: String?

    // MARK: - Initialisation

    init?(metalView: MTKView) {
        guard let device = MTLCreateSystemDefaultDevice() else {
            NSLog("[MetalRenderer] Metal is not supported on this device")
            return nil
        }

        self.device = device

        guard let commandQueue = device.makeCommandQueue() else {
            NSLog("[MetalRenderer] Failed to create command queue")
            return nil
        }
        self.commandQueue = commandQueue

        super.init()

        metalView.device = device
        metalView.delegate = self
        metalView.colorPixelFormat = .bgra8Unorm
        metalView.depthStencilPixelFormat = .depth32Float
        metalView.clearColor = backgroundColor

        // Build pipelines
        buildPipelines(metalView: metalView)
        buildDepthStencil()
        buildBoxGeometry()

        // M3.5: Initialise composite renderer
        compositeRenderer = CompositePointCloudRenderer(device: device)
    }

    // MARK: - Pipeline Setup

    private func buildPipelines(metalView: MTKView) {
        guard let library = device.makeDefaultLibrary() else {
            print("Failed to create shader library")
            return
        }

        // Point cloud pipeline
        if let vertexFunc = library.makeFunction(name: "pointVertex"),
            let fragmentFunc = library.makeFunction(name: "pointFragment")
        {

            let descriptor = MTLRenderPipelineDescriptor()
            descriptor.vertexFunction = vertexFunc
            descriptor.fragmentFunction = fragmentFunc
            descriptor.colorAttachments[0].pixelFormat = metalView.colorPixelFormat
            descriptor.depthAttachmentPixelFormat = metalView.depthStencilPixelFormat

            // Enable blending for point sprites
            descriptor.colorAttachments[0].isBlendingEnabled = true
            descriptor.colorAttachments[0].sourceRGBBlendFactor = .sourceAlpha
            descriptor.colorAttachments[0].destinationRGBBlendFactor = .oneMinusSourceAlpha

            do {
                pointCloudPipeline = try device.makeRenderPipelineState(descriptor: descriptor)
            } catch { print("Failed to create point cloud pipeline: \(error)") }
        }

        // Box pipeline
        if let vertexFunc = library.makeFunction(name: "boxVertex"),
            let fragmentFunc = library.makeFunction(name: "boxFragment")
        {

            let descriptor = MTLRenderPipelineDescriptor()
            descriptor.vertexFunction = vertexFunc
            descriptor.fragmentFunction = fragmentFunc
            descriptor.colorAttachments[0].pixelFormat = metalView.colorPixelFormat
            descriptor.depthAttachmentPixelFormat = metalView.depthStencilPixelFormat

            do { boxPipeline = try device.makeRenderPipelineState(descriptor: descriptor) } catch {
                print("Failed to create box pipeline: \(error)")
            }
        }

        // Trail pipeline
        if let vertexFunc = library.makeFunction(name: "trailVertex"),
            let fragmentFunc = library.makeFunction(name: "trailFragment")
        {

            let descriptor = MTLRenderPipelineDescriptor()
            descriptor.vertexFunction = vertexFunc
            descriptor.fragmentFunction = fragmentFunc
            descriptor.colorAttachments[0].pixelFormat = metalView.colorPixelFormat
            descriptor.depthAttachmentPixelFormat = metalView.depthStencilPixelFormat

            // Enable blending for trail fade
            descriptor.colorAttachments[0].isBlendingEnabled = true
            descriptor.colorAttachments[0].sourceRGBBlendFactor = .sourceAlpha
            descriptor.colorAttachments[0].destinationRGBBlendFactor = .oneMinusSourceAlpha

            do { trailPipeline = try device.makeRenderPipelineState(descriptor: descriptor) } catch
            { print("Failed to create trail pipeline: \(error)") }
        }

        // M6: Debug line pipeline (uses debugLine shaders for coloured lines)
        if let vertexFunc = library.makeFunction(name: "debugLineVertex"),
            let fragmentFunc = library.makeFunction(name: "debugLineFragment")
        {

            let descriptor = MTLRenderPipelineDescriptor()
            descriptor.vertexFunction = vertexFunc
            descriptor.fragmentFunction = fragmentFunc
            descriptor.colorAttachments[0].pixelFormat = metalView.colorPixelFormat
            descriptor.depthAttachmentPixelFormat = metalView.depthStencilPixelFormat

            // Enable blending for debug overlays
            descriptor.colorAttachments[0].isBlendingEnabled = true
            descriptor.colorAttachments[0].sourceRGBBlendFactor = .sourceAlpha
            descriptor.colorAttachments[0].destinationRGBBlendFactor = .oneMinusSourceAlpha

            do { linePipeline = try device.makeRenderPipelineState(descriptor: descriptor) } catch {
                print("Failed to create debug line pipeline: \(error)")
            }
        }
    }

    private func buildDepthStencil() {
        let descriptor = MTLDepthStencilDescriptor()
        descriptor.depthCompareFunction = .less
        descriptor.isDepthWriteEnabled = true
        depthStencilState = device.makeDepthStencilState(descriptor: descriptor)
    }

    private func buildBoxGeometry() {
        // Unit cube wireframe vertices (centered at origin, size 1x1x1)
        // Will be scaled/rotated/translated by instance transforms
        let vertices: [Float] = [
            // Bottom face edges
            -0.5, -0.5, 0, 0.5, -0.5, 0, 0.5, -0.5, 0, 0.5, 0.5, 0, 0.5, 0.5, 0, -0.5, 0.5, 0, -0.5,
            0.5, 0, -0.5, -0.5, 0,
            // Top face edges
            -0.5, -0.5, 1, 0.5, -0.5, 1, 0.5, -0.5, 1, 0.5, 0.5, 1, 0.5, 0.5, 1, -0.5, 0.5, 1, -0.5,
            0.5, 1, -0.5, -0.5, 1,
            // Vertical edges
            -0.5, -0.5, 0, -0.5, -0.5, 1, 0.5, -0.5, 0, 0.5, -0.5, 1, 0.5, 0.5, 0, 0.5, 0.5, 1,
            -0.5, 0.5, 0, -0.5, 0.5, 1,
        ]

        let bufferSize = vertices.count * MemoryLayout<Float>.stride
        boxVertices = device.makeBuffer(
            bytes: vertices, length: bufferSize, options: .storageModeShared)
        boxVertexCount = vertices.count / 3  // Each vertex is 3 floats (x, y, z)
    }

    // MARK: - Frame Update

    /// Update the renderer with a new frame of data.
    func updateFrame(_ frame: FrameBundle) {
        frameUpdateCount += 1

        // M3.5: Use composite renderer for split streaming
        compositeRenderer?.processFrame(frame)

        // Legacy path: Update point cloud buffer directly for full frames
        if frame.frameType == .full, let pointCloud = frame.pointCloud, pointCloud.pointCount > 0 {
            updatePointBuffer(pointCloud)
        }

        // M4: Update box instances from tracks
        if let tracks = frame.tracks { updateBoxInstances(tracks) }

        // M4: Update cluster instances
        if let clusters = frame.clusters { updateClusterInstances(clusters) }

        // Update trails
        if let tracks = frame.tracks { updateTrailBuffer(tracks) }

        // M5: Update heading arrows from tracks + clusters
        updateHeadingArrows(tracks: frame.tracks, clusters: frame.clusters)

        // M6: Update debug overlays
        if showDebug, let debug = frame.debug {
            updateDebugOverlays(debug, tracks: frame.tracks)
        } else {
            debugLineVertexCount = 0
            ellipseVertexCount = 0
            ellipseSegments = []
        }
    }

    private var frameUpdateCount: Int = 0

    // M7: Buffer reallocation thresholds for pre-allocated buffer reuse
    private static let shrinkThreshold: Int = 4  // Shrink if buffer is >4x needed
    private static let growMargin: Float = 1.5  // Allocate 50% extra for headroom

    /// M7: Determine if a buffer should be reallocated.
    private func shouldReallocateBuffer(currentCapacity: Int, neededCount: Int) -> Bool {
        // Need to grow: current capacity insufficient
        if neededCount > currentCapacity { return true }
        // Should shrink: buffer is excessively large (>4x needed)
        if currentCapacity > neededCount * Self.shrinkThreshold && neededCount > 0 { return true }
        return false
    }

    /// M7: Calculate new buffer capacity with growth margin.
    private func calculateBufferCapacity(for count: Int) -> Int {
        return Int(Float(count) * Self.growMargin)
    }

    /// M7: Update point buffer using pre-allocated buffer reuse.
    /// Only reallocates when capacity is insufficient or excessively large.
    private func updatePointBuffer(_ pointCloud: PointCloudFrame) {
        let count = pointCloud.pointCount
        guard count > 0 else { return }

        // M7: Check if we need to reallocate the buffer
        let neededVertices = count * 5  // 5 floats per vertex
        if shouldReallocateBuffer(currentCapacity: pointBufferCapacity, neededCount: neededVertices)
        {
            let newCapacity = calculateBufferCapacity(for: neededVertices)
            let bufferSize = newCapacity * MemoryLayout<Float>.stride
            pointBuffer = device.makeBuffer(length: bufferSize, options: .storageModeShared)
            pointBufferCapacity = newCapacity
        }

        // Copy data into buffer
        guard let buffer = pointBuffer else { return }
        let ptr = buffer.contents().bindMemory(to: Float.self, capacity: neededVertices)

        for i in 0..<count {
            ptr[i * 5 + 0] = pointCloud.x[i]
            ptr[i * 5 + 1] = pointCloud.y[i]
            ptr[i * 5 + 2] = pointCloud.z[i]
            ptr[i * 5 + 3] = Float(pointCloud.intensity[i]) / 255.0
            // Classification: 0=background, 1=foreground, 2=ground
            var classification: Float = 0.0
            if i < pointCloud.classification.count {
                classification = Float(pointCloud.classification[i])
            }
            ptr[i * 5 + 4] = classification
        }

        pointCount = count
    }

    private func updateBoxInstances(_ trackSet: TrackSet) {
        // Cache tracks for hit testing
        _lastTracks = trackSet.tracks

        // Each box instance: [transform matrix (16 floats) + colour (4 floats)]
        var instances = [Float]()

        for track in trackSet.tracks {
            // Build transform matrix
            let scale = simd_float4x4(
                diagonal: simd_float4(
                    track.bboxLengthAvg > 0 ? track.bboxLengthAvg : 1.0,
                    track.bboxWidthAvg > 0 ? track.bboxWidthAvg : 1.0,
                    track.bboxHeightAvg > 0 ? track.bboxHeightAvg : 1.0, 1.0))

            // Use OBB heading for box orientation (aligns box to physical shape);
            // fall back to velocity heading if OBB heading unavailable.
            let boxHeading = track.bboxHeadingRad != 0 ? track.bboxHeadingRad : track.headingRad
            let rotation = simd_float4x4(rotationZ: boxHeading)
            let translation = simd_float4x4(translation: simd_float3(track.x, track.y, track.z))
            let transform = translation * rotation * scale

            // Add transform (16 floats)
            for col in 0..<4 { for row in 0..<4 { instances.append(transform[col][row]) } }

            // Add colour based on state (4 floats)
            // M6: Highlight selected track
            let isSelected = selectedTrackID == track.trackID
            if isSelected {
                instances.append(1.0)  // r (white highlight)
                instances.append(1.0)  // g
                instances.append(1.0)  // b
                instances.append(track.alpha)  // alpha (supports fade-out)
            } else {
                let colour = track.state.colour
                instances.append(colour.r)
                instances.append(colour.g)
                instances.append(colour.b)
                instances.append(track.alpha)  // alpha (supports fade-out)
            }
        }

        if !instances.isEmpty {
            let bufferSize = instances.count * MemoryLayout<Float>.stride
            boxInstances = device.makeBuffer(
                bytes: instances, length: bufferSize, options: .storageModeShared)
            boxInstanceCount = trackSet.tracks.count
        } else {
            boxInstanceCount = 0
        }
    }

    /// M4: Update cluster box instances.
    /// M5: Uses OBB (oriented bounding box) dimensions when available.
    private func updateClusterInstances(_ clusterSet: ClusterSet) {
        // Each box instance: [transform matrix (16 floats) + colour (4 floats)]
        var instances = [Float]()

        for cluster in clusterSet.clusters {
            let transform: simd_float4x4

            if let obb = cluster.obb {
                // M5: Use full OBB (centre, dimensions, heading)
                let scale = simd_float4x4(
                    diagonal: simd_float4(
                        obb.length > 0 ? obb.length : 0.5, obb.width > 0 ? obb.width : 0.5,
                        obb.height > 0 ? obb.height : 0.5, 1.0))

                let rotation = simd_float4x4(rotationZ: obb.headingRad)
                let translation = simd_float4x4(
                    translation: simd_float3(obb.centerX, obb.centerY, obb.centerZ))
                transform = translation * rotation * scale
            } else {
                // Fallback: AABB dimensions centred at centroid
                let scale = simd_float4x4(
                    diagonal: simd_float4(
                        cluster.aabbLength > 0 ? cluster.aabbLength : 0.5,
                        cluster.aabbWidth > 0 ? cluster.aabbWidth : 0.5,
                        cluster.aabbHeight > 0 ? cluster.aabbHeight : 0.5, 1.0))

                let translation = simd_float4x4(
                    translation: simd_float3(
                        cluster.centroidX, cluster.centroidY, cluster.centroidZ))
                transform = translation * scale
            }

            // Add transform (16 floats)
            for col in 0..<4 { for row in 0..<4 { instances.append(transform[col][row]) } }

            // Add colour: cyan/blue for clusters (4 floats)
            instances.append(0.0)  // r
            instances.append(0.8)  // g (cyan)
            instances.append(1.0)  // b
            instances.append(0.7)  // alpha (slightly transparent)
        }

        if !instances.isEmpty {
            let bufferSize = instances.count * MemoryLayout<Float>.stride
            clusterInstances = device.makeBuffer(
                bytes: instances, length: bufferSize, options: .storageModeShared)
            clusterInstanceCount = clusterSet.clusters.count
        } else {
            clusterInstanceCount = 0
        }
    }

    private func updateTrailBuffer(_ trackSet: TrackSet) {
        // Trail vertices: [x, y, z, alpha] per vertex
        var vertices = [Float]()
        var segments: [(start: Int, count: Int)] = []

        for trail in trackSet.trails {
            let pointCount = trail.points.count
            guard pointCount >= 2 else { continue }

            let segmentStart = vertices.count / 4  // Start index for this trail

            for (i, point) in trail.points.enumerated() {
                let alpha = Float(i) / Float(pointCount - 1)  // fade from 0 to 1
                vertices.append(point.x)
                vertices.append(point.y)
                vertices.append(0.1)  // slight Z offset
                vertices.append(alpha)
            }

            segments.append((start: segmentStart, count: pointCount))
        }

        if !vertices.isEmpty {
            let bufferSize = vertices.count * MemoryLayout<Float>.stride
            trailVertices = device.makeBuffer(
                bytes: vertices, length: bufferSize, options: .storageModeShared)
            trailVertexCount = vertices.count / 4
            trailSegments = segments
        } else {
            trailVertexCount = 0
            trailSegments = []
        }
    }

    // MARK: - M5: Heading Arrows

    /// Generate heading arrow lines for tracks and clusters.
    /// Each arrow is a line from centre along heading direction with a fixed length.
    private func updateHeadingArrows(tracks: TrackSet?, clusters: ClusterSet?) {
        // Debug line format: [x, y, z, r, g, b, a] per vertex (7 floats)
        var vertices = [Float]()

        // Track heading arrows (green)
        if let trackSet = tracks {
            for track in trackSet.tracks {
                guard track.bboxHeadingRad != 0 || track.headingRad != 0 else { continue }
                // Prefer velocity-based heading (direction of travel) over PCA heading
                // for track arrows. PCA heading (bboxHeadingRad) is used for box rotation
                // but velocity heading better represents direction of motion.
                let heading = track.headingRad != 0 ? track.headingRad : track.bboxHeadingRad
                let arrowLength = max(track.bboxLengthAvg, track.bboxWidthAvg, 1.0) * 0.8

                let tipX = track.x + cos(heading) * arrowLength
                let tipY = track.y + sin(heading) * arrowLength

                let isSelected = selectedTrackID == track.trackID
                let colour = track.state.colour
                let arrowAlpha = track.alpha * (isSelected ? 1.0 : 0.8)

                // Main arrow shaft
                vertices.append(contentsOf: [
                    track.x, track.y, track.z + 0.05, colour.r, colour.g, colour.b, arrowAlpha,
                ])
                vertices.append(contentsOf: [
                    tipX, tipY, track.z + 0.05, colour.r, colour.g, colour.b, arrowAlpha,
                ])

                // Arrowhead left
                let headAngle: Float = .pi / 6  // 30 degrees
                let headLen = arrowLength * 0.25
                let leftX = tipX - cos(heading - headAngle) * headLen
                let leftY = tipY - sin(heading - headAngle) * headLen
                vertices.append(contentsOf: [
                    tipX, tipY, track.z + 0.05, colour.r, colour.g, colour.b, arrowAlpha,
                ])
                vertices.append(contentsOf: [
                    leftX, leftY, track.z + 0.05, colour.r, colour.g, colour.b, arrowAlpha,
                ])

                // Arrowhead right
                let rightX = tipX - cos(heading + headAngle) * headLen
                let rightY = tipY - sin(heading + headAngle) * headLen
                vertices.append(contentsOf: [
                    tipX, tipY, track.z + 0.05, colour.r, colour.g, colour.b, arrowAlpha,
                ])
                vertices.append(contentsOf: [
                    rightX, rightY, track.z + 0.05, colour.r, colour.g, colour.b, arrowAlpha,
                ])
            }
        }

        // Cluster heading arrows (cyan, only when OBB is present)
        if showClusters, let clusterSet = clusters {
            for cluster in clusterSet.clusters {
                guard let obb = cluster.obb else { continue }
                let arrowLength = max(obb.length, obb.width, 0.5) * 0.6

                let tipX = obb.centerX + cos(obb.headingRad) * arrowLength
                let tipY = obb.centerY + sin(obb.headingRad) * arrowLength

                // Arrow shaft
                vertices.append(contentsOf: [
                    obb.centerX, obb.centerY, obb.centerZ + 0.05, 0.0, 0.8, 1.0, 0.6,  // cyan
                ])
                vertices.append(contentsOf: [tipX, tipY, obb.centerZ + 0.05, 0.0, 0.8, 1.0, 0.6])

                // Arrowhead
                let headAngle: Float = .pi / 6
                let headLen = arrowLength * 0.25
                let leftX = tipX - cos(obb.headingRad - headAngle) * headLen
                let leftY = tipY - sin(obb.headingRad - headAngle) * headLen
                let rightX = tipX - cos(obb.headingRad + headAngle) * headLen
                let rightY = tipY - sin(obb.headingRad + headAngle) * headLen

                vertices.append(contentsOf: [tipX, tipY, obb.centerZ + 0.05, 0.0, 0.8, 1.0, 0.6])
                vertices.append(contentsOf: [leftX, leftY, obb.centerZ + 0.05, 0.0, 0.8, 1.0, 0.6])
                vertices.append(contentsOf: [tipX, tipY, obb.centerZ + 0.05, 0.0, 0.8, 1.0, 0.6])
                vertices.append(contentsOf: [
                    rightX, rightY, obb.centerZ + 0.05, 0.0, 0.8, 1.0, 0.6,
                ])
            }
        }

        if !vertices.isEmpty {
            let bufferSize = vertices.count * MemoryLayout<Float>.stride
            headingArrowVertices = device.makeBuffer(
                bytes: vertices, length: bufferSize, options: .storageModeShared)
            headingArrowVertexCount = vertices.count / 7  // 7 floats per vertex
        } else {
            headingArrowVertexCount = 0
        }
    }

    // MARK: - M6: Debug Overlays

    /// Generate debug overlay geometry from DebugOverlaySet.
    private func updateDebugOverlays(_ debug: DebugOverlaySet, tracks: TrackSet?) {
        // Build a track position lookup for association lines
        var trackPositions: [String: (x: Float, y: Float, z: Float)] = [:]
        if let trackSet = tracks {
            for track in trackSet.tracks {
                trackPositions[track.trackID] = (x: track.x, y: track.y, z: track.z)
            }
        }

        // Debug line format: [x, y, z, r, g, b, a] per vertex (7 floats)
        var lineVertices = [Float]()

        // Association lines (dashed: accepted=solid green, rejected=dashed red)
        if showAssociation {
            for candidate in debug.associationCandidates {
                guard let trackPos = trackPositions[candidate.trackID] else { continue }

                // We need cluster positions - derive from residual measured positions
                // For now, draw from track to a point offset by distance in an estimated direction
                // This is a simplification; full implementation would need cluster centroids
                let colour: (r: Float, g: Float, b: Float, a: Float) =
                    candidate.accepted
                    ? (0.0, 1.0, 0.0, 0.7)  // green for accepted
                    : (1.0, 0.3, 0.3, 0.4)  // red for rejected

                // Look for matching residual to get measured position
                if let residual = debug.residuals.first(where: { $0.trackID == candidate.trackID })
                {
                    lineVertices.append(contentsOf: [
                        trackPos.x, trackPos.y, trackPos.z + 0.1, colour.r, colour.g, colour.b,
                        colour.a,
                    ])
                    lineVertices.append(contentsOf: [
                        residual.measuredX, residual.measuredY, trackPos.z + 0.1, colour.r,
                        colour.g, colour.b, colour.a,
                    ])
                }
            }
        }

        // Residual vectors (predicted → measured, magenta)
        if showResiduals {
            for residual in debug.residuals {
                let z: Float = trackPositions[residual.trackID]?.z ?? 0.1

                // Predicted position (yellow dot end)
                lineVertices.append(contentsOf: [
                    residual.predictedX, residual.predictedY, z + 0.15, 1.0, 0.8, 0.0, 0.8,  // yellow (predicted)
                ])
                // Measured position (magenta dot end)
                lineVertices.append(contentsOf: [
                    residual.measuredX, residual.measuredY, z + 0.15, 1.0, 0.0, 1.0, 0.8,  // magenta (measured)
                ])
            }
        }

        if !lineVertices.isEmpty {
            let bufferSize = lineVertices.count * MemoryLayout<Float>.stride
            debugLineVertices = device.makeBuffer(
                bytes: lineVertices, length: bufferSize, options: .storageModeShared)
            debugLineVertexCount = lineVertices.count / 7
        } else {
            debugLineVertexCount = 0
        }

        // Gating ellipses (rendered as line strips approximating ellipses)
        if showGating {
            var ellipseVerts = [Float]()
            var segments: [(start: Int, count: Int)] = []
            let segmentCount = 32  // segments per ellipse

            for ellipse in debug.gatingEllipses {
                let segmentStart = ellipseVerts.count / 7
                let z: Float = trackPositions[ellipse.trackID]?.z ?? 0.1

                let isSelected = selectedTrackID == ellipse.trackID

                for i in 0...segmentCount {
                    let angle = Float(i) / Float(segmentCount) * 2.0 * .pi

                    // Ellipse point in local frame
                    let localX = ellipse.semiMajor * cos(angle)
                    let localY = ellipse.semiMinor * sin(angle)

                    // Rotate by ellipse rotation
                    let cosR = cos(ellipse.rotationRad)
                    let sinR = sin(ellipse.rotationRad)
                    let worldX = ellipse.centerX + localX * cosR - localY * sinR
                    let worldY = ellipse.centerY + localX * sinR + localY * cosR

                    ellipseVerts.append(contentsOf: [
                        worldX, worldY, z + 0.12, 0.3, 0.6, 1.0, isSelected ? 0.9 : 0.5,  // light blue
                    ])
                }

                segments.append((start: segmentStart, count: segmentCount + 1))
            }

            if !ellipseVerts.isEmpty {
                let bufferSize = ellipseVerts.count * MemoryLayout<Float>.stride
                ellipseVertices = device.makeBuffer(
                    bytes: ellipseVerts, length: bufferSize, options: .storageModeShared)
                ellipseVertexCount = ellipseVerts.count / 7
                ellipseSegments = segments
            } else {
                ellipseVertexCount = 0
                ellipseSegments = []
            }
        } else {
            ellipseVertexCount = 0
            ellipseSegments = []
        }
    }

    // MARK: - MTKViewDelegate

    func mtkView(_ view: MTKView, drawableSizeWillChange size: CGSize) {
        camera.aspectRatio = Float(size.width / size.height)
    }

    func draw(in view: MTKView) {
        guard let drawable = view.currentDrawable,
            let descriptor = view.currentRenderPassDescriptor,
            let commandBuffer = commandQueue.makeCommandBuffer(),
            let encoder = commandBuffer.makeRenderCommandEncoder(descriptor: descriptor)
        else { return }

        // Update uniforms
        uniforms.modelViewProjection = camera.projectionMatrix * camera.viewMatrix
        uniforms.modelView = camera.viewMatrix
        uniforms.pointSize = pointSize
        uniforms.time += 1.0 / 60.0

        encoder.setDepthStencilState(depthStencilState)

        // Draw point cloud
        if showPoints, let pipeline = pointCloudPipeline {
            // M3.5: Use composite renderer if available
            if let composite = compositeRenderer {
                composite.render(encoder: encoder, pipeline: pipeline, uniforms: &uniforms)
            } else if let buffer = pointBuffer, pointCount > 0 {
                // Legacy path: single buffer
                encoder.setRenderPipelineState(pipeline)
                encoder.setVertexBuffer(buffer, offset: 0, index: 0)
                encoder.setVertexBytes(&uniforms, length: MemoryLayout<Uniforms>.stride, index: 1)
                encoder.drawPrimitives(type: .point, vertexStart: 0, vertexCount: pointCount)
            }
        }

        // M4: Draw cluster boxes first (behind tracks)
        if showClusters, let pipeline = boxPipeline, let boxVerts = boxVertices,
            let instances = clusterInstances, clusterInstanceCount > 0
        {
            encoder.setRenderPipelineState(pipeline)
            encoder.setVertexBuffer(boxVerts, offset: 0, index: 0)
            encoder.setVertexBuffer(instances, offset: 0, index: 1)
            encoder.setVertexBytes(&uniforms, length: MemoryLayout<Uniforms>.stride, index: 2)
            encoder.drawPrimitives(
                type: .line, vertexStart: 0, vertexCount: boxVertexCount,
                instanceCount: clusterInstanceCount)
        }

        // Draw track boxes (on top of clusters)
        if showBoxes, let pipeline = boxPipeline, let boxVerts = boxVertices,
            let instances = boxInstances, boxInstanceCount > 0
        {
            encoder.setRenderPipelineState(pipeline)
            encoder.setVertexBuffer(boxVerts, offset: 0, index: 0)
            encoder.setVertexBuffer(instances, offset: 0, index: 1)
            encoder.setVertexBytes(&uniforms, length: MemoryLayout<Uniforms>.stride, index: 2)
            // Draw wireframe boxes as lines (24 vertices per box = 12 edges)
            encoder.drawPrimitives(
                type: .line, vertexStart: 0, vertexCount: boxVertexCount,
                instanceCount: boxInstanceCount)
        }

        // Draw trails (each trail as a separate lineStrip)
        if showTrails, let pipeline = trailPipeline, let vertices = trailVertices,
            !trailSegments.isEmpty
        {
            encoder.setRenderPipelineState(pipeline)
            encoder.setVertexBuffer(vertices, offset: 0, index: 0)
            encoder.setVertexBytes(&uniforms, length: MemoryLayout<Uniforms>.stride, index: 1)

            // Draw each trail as a separate line strip
            for segment in trailSegments {
                encoder.drawPrimitives(
                    type: .lineStrip, vertexStart: segment.start, vertexCount: segment.count)
            }
        }

        // M5: Draw heading arrows
        if showBoxes || showClusters, let pipeline = linePipeline,
            let vertices = headingArrowVertices, headingArrowVertexCount > 0
        {
            encoder.setRenderPipelineState(pipeline)
            encoder.setVertexBuffer(vertices, offset: 0, index: 0)
            encoder.setVertexBytes(&uniforms, length: MemoryLayout<Uniforms>.stride, index: 1)
            encoder.drawPrimitives(
                type: .line, vertexStart: 0, vertexCount: headingArrowVertexCount)
        }

        // M6: Draw debug overlays
        if showDebug, let pipeline = linePipeline {
            // Debug lines (association lines + residual vectors)
            if let vertices = debugLineVertices, debugLineVertexCount > 0 {
                encoder.setRenderPipelineState(pipeline)
                encoder.setVertexBuffer(vertices, offset: 0, index: 0)
                encoder.setVertexBytes(&uniforms, length: MemoryLayout<Uniforms>.stride, index: 1)
                encoder.drawPrimitives(
                    type: .line, vertexStart: 0, vertexCount: debugLineVertexCount)
            }

            // Gating ellipses (line strips)
            if let vertices = ellipseVertices, !ellipseSegments.isEmpty {
                encoder.setRenderPipelineState(pipeline)
                encoder.setVertexBuffer(vertices, offset: 0, index: 0)
                encoder.setVertexBytes(&uniforms, length: MemoryLayout<Uniforms>.stride, index: 1)
                for segment in ellipseSegments {
                    encoder.drawPrimitives(
                        type: .lineStrip, vertexStart: segment.start, vertexCount: segment.count)
                }
            }
        }

        encoder.endEncoding()
        commandBuffer.present(drawable)
        commandBuffer.commit()
    }

    // MARK: - Camera Controls

    /// Handle mouse/trackpad drag for camera orbit or pan.
    func handleMouseDrag(deltaX: CGFloat, deltaY: CGFloat, isRightButton: Bool, shiftHeld: Bool) {
        if isRightButton || shiftHeld {
            // Pan: move camera and target together
            let sensitivity: Float = 0.05
            let right = normalize(cross(camera.up, camera.position - camera.target))
            let up = camera.up

            let offset = right * Float(-deltaX) * sensitivity + up * Float(deltaY) * sensitivity
            camera.position += offset
            camera.target += offset
        } else {
            // Orbit: rotate around target
            let sensitivity: Float = 0.005

            // Horizontal rotation (azimuth) around up axis
            let azimuthDelta = Float(-deltaX) * sensitivity

            // Vertical rotation (elevation)
            let elevationDelta = Float(-deltaY) * sensitivity

            // Get current camera offset from target
            var offset = camera.position - camera.target
            let distance = length(offset)

            // Convert to spherical coordinates
            let currentElevation = asin(offset.z / distance)
            let currentAzimuth = atan2(offset.y, offset.x)

            // Apply deltas
            let newAzimuth = currentAzimuth + azimuthDelta
            let newElevation = max(
                -.pi / 2 + 0.1, min(.pi / 2 - 0.1, currentElevation + elevationDelta))

            // Convert back to cartesian
            offset = simd_float3(
                distance * cos(newElevation) * cos(newAzimuth),
                distance * cos(newElevation) * sin(newAzimuth), distance * sin(newElevation))

            camera.position = camera.target + offset
        }
    }

    /// Handle scroll wheel or pinch for zoom.
    func handleZoom(delta: CGFloat) {
        let sensitivity: Float = 0.1
        let zoomFactor = 1.0 - Float(delta) * sensitivity

        // Move camera toward/away from target
        var offset = camera.position - camera.target
        let newDistance = max(1.0, min(500.0, length(offset) * zoomFactor))
        offset = normalize(offset) * newDistance
        camera.position = camera.target + offset
    }

    /// Reset camera to default view.
    func resetCamera() {
        camera.position = simd_float3(0, -30, 20)
        camera.target = simd_float3(0, 10, 0)
        camera.up = simd_float3(0, 0, 1)
    }

    /// M3.5: Get background cache status for UI display.
    func getCacheStatus() -> String {
        compositeRenderer?.cacheStatus ?? "Not using split streaming"
    }

    /// M6: Hit test tracks at a screen position.
    /// Projects each track position to screen space and finds the nearest within a tolerance.
    func hitTestTrack(at point: CGPoint, viewSize: CGSize) -> String? {
        guard let frame = lastFrameData else { return nil }
        guard !frame.isEmpty else { return nil }

        let mvp = camera.projectionMatrix * camera.viewMatrix
        let halfWidth = Float(viewSize.width) * 0.5
        let halfHeight = Float(viewSize.height) * 0.5
        let tolerance: Float = 20.0  // pixels

        var bestTrackID: String?
        var bestDistance: Float = tolerance

        for (trackID, pos) in frame {
            // Project world position to clip space
            let clip = mvp * simd_float4(pos.x, pos.y, pos.z, 1.0)
            guard clip.w > 0 else { continue }

            // NDC to screen
            let ndcX = clip.x / clip.w
            let ndcY = clip.y / clip.w
            let screenX = (ndcX + 1.0) * halfWidth
            let screenY = (ndcY + 1.0) * halfHeight  // Metal Y is bottom-up like NSView

            let dx = screenX - Float(point.x)
            let dy = screenY - Float(point.y)
            let distance = sqrt(dx * dx + dy * dy)

            if distance < bestDistance {
                bestDistance = distance
                bestTrackID = trackID
            }
        }

        return bestTrackID
    }

    /// Cache of track positions for hit testing (updated per frame).
    private var lastFrameData: [String: simd_float3]? {
        guard let tracks = _lastTracks else { return nil }
        var result = [String: simd_float3]()
        for track in tracks { result[track.trackID] = simd_float3(track.x, track.y, track.z) }
        return result
    }
    private var _lastTracks: [Track]?

    /// M3.5: Get point cloud statistics.
    func getPointCloudStats() -> (background: Int, foreground: Int, total: Int) {
        compositeRenderer?.getStats() ?? (background: 0, foreground: pointCount, total: pointCount)
    }

    /// Handle keyboard input for camera control.
    /// Returns true if the key was handled.
    func handleKeyDown(keyCode: UInt16, modifiers: NSEvent.ModifierFlags) -> Bool {
        let moveStep: Float = 2.0
        let right = normalize(cross(camera.up, camera.position - camera.target))
        let forward = normalize(camera.target - camera.position)

        switch keyCode {
        case 15:  // R - Reset camera
            resetCamera()
            return true
        case 123:  // Left arrow - pan left
            let offset = right * moveStep
            camera.position += offset
            camera.target += offset
            return true
        case 124:  // Right arrow - pan right
            let offset = right * -moveStep
            camera.position += offset
            camera.target += offset
            return true
        case 125:  // Down arrow - pan backward
            let offset = forward * -moveStep
            camera.position += offset
            camera.target += offset
            return true
        case 126:  // Up arrow - pan forward
            let offset = forward * moveStep
            camera.position += offset
            camera.target += offset
            return true
        case 24:  // + (equals key) - zoom in
            handleZoom(delta: 1.0)
            return true
        case 27:  // - (minus key) - zoom out
            handleZoom(delta: -1.0)
            return true
        default: return false
        }
    }
}

// MARK: - Camera

struct Camera {
    var position: simd_float3 = simd_float3(0, -30, 20)
    var target: simd_float3 = simd_float3(0, 10, 0)
    var up: simd_float3 = simd_float3(0, 0, 1)

    var fov: Float = 60.0  // degrees
    var aspectRatio: Float = 1.0
    var nearPlane: Float = 0.1
    var farPlane: Float = 500.0

    var viewMatrix: simd_float4x4 { simd_float4x4(lookAt: target, from: position, up: up) }

    var projectionMatrix: simd_float4x4 {
        simd_float4x4(
            perspectiveFov: fov * .pi / 180, aspectRatio: aspectRatio, near: nearPlane,
            far: farPlane)
    }
}

// MARK: - Matrix Extensions

extension simd_float4x4 {
    init(translation t: simd_float3) {
        self.init(
            columns: (
                simd_float4(1, 0, 0, 0), simd_float4(0, 1, 0, 0), simd_float4(0, 0, 1, 0),
                simd_float4(t.x, t.y, t.z, 1)
            ))
    }

    init(rotationZ angle: Float) {
        let c = cos(angle)
        let s = sin(angle)
        self.init(
            columns: (
                simd_float4(c, s, 0, 0), simd_float4(-s, c, 0, 0), simd_float4(0, 0, 1, 0),
                simd_float4(0, 0, 0, 1)
            ))
    }

    init(lookAt target: simd_float3, from eye: simd_float3, up: simd_float3) {
        let z = normalize(eye - target)
        let x = normalize(cross(up, z))
        let y = cross(z, x)

        self.init(
            columns: (
                simd_float4(x.x, y.x, z.x, 0), simd_float4(x.y, y.y, z.y, 0),
                simd_float4(x.z, y.z, z.z, 0),
                simd_float4(-dot(x, eye), -dot(y, eye), -dot(z, eye), 1)
            ))
    }

    init(perspectiveFov fovRadians: Float, aspectRatio: Float, near: Float, far: Float) {
        let y = 1 / tan(fovRadians * 0.5)
        let x = y / aspectRatio
        let z = far / (near - far)

        self.init(
            columns: (
                simd_float4(x, 0, 0, 0), simd_float4(0, y, 0, 0), simd_float4(0, 0, z, -1),
                simd_float4(0, 0, z * near, 0)
            ))
    }
}
