// MetalRenderer.swift
// Main Metal renderer for point clouds, boxes, and trails.
//
// This renderer uses Metal for efficient GPU-accelerated rendering of:
// - Point clouds as point sprites
// - Bounding boxes as instanced geometry
// - Track trails as polylines
// - 2D overlays for debug visualisation

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
        modelViewProjection: matrix_identity_float4x4,
        modelView: matrix_identity_float4x4,
        pointSize: 5.0,
        time: 0,
        padding: simd_float2(0, 0)
    )

    // MARK: - Camera

    var camera = Camera()

    // MARK: - Frame Data

    var pointBuffer: MTLBuffer?
    var pointCount: Int = 0

    var boxVertices: MTLBuffer?
    var boxVertexCount: Int = 0
    var boxInstances: MTLBuffer?
    var boxInstanceCount: Int = 0

    var trailVertices: MTLBuffer?
    var trailVertexCount: Int = 0

    // MARK: - Settings

    var showPoints: Bool = true
    var showBoxes: Bool = true
    var showTrails: Bool = true
    var pointSize: Float = 5.0
    var backgroundColor: MTLClearColor = MTLClearColor(red: 0.1, green: 0.1, blue: 0.15, alpha: 1.0)

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
            } catch {
                print("Failed to create point cloud pipeline: \(error)")
            }
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

            do {
                boxPipeline = try device.makeRenderPipelineState(descriptor: descriptor)
            } catch {
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

            do {
                trailPipeline = try device.makeRenderPipelineState(descriptor: descriptor)
            } catch {
                print("Failed to create trail pipeline: \(error)")
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
            -0.5, -0.5, 0, 0.5, -0.5, 0,
            0.5, -0.5, 0, 0.5, 0.5, 0,
            0.5, 0.5, 0, -0.5, 0.5, 0,
            -0.5, 0.5, 0, -0.5, -0.5, 0,
            // Top face edges
            -0.5, -0.5, 1, 0.5, -0.5, 1,
            0.5, -0.5, 1, 0.5, 0.5, 1,
            0.5, 0.5, 1, -0.5, 0.5, 1,
            -0.5, 0.5, 1, -0.5, -0.5, 1,
            // Vertical edges
            -0.5, -0.5, 0, -0.5, -0.5, 1,
            0.5, -0.5, 0, 0.5, -0.5, 1,
            0.5, 0.5, 0, 0.5, 0.5, 1,
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

        // Update point cloud buffer
        if let pointCloud = frame.pointCloud, pointCloud.pointCount > 0 {
            updatePointBuffer(pointCloud)
        }

        // Update box instances from tracks
        if let tracks = frame.tracks {
            updateBoxInstances(tracks)
        }

        // Update trails
        if let tracks = frame.tracks {
            updateTrailBuffer(tracks)
        }
    }

    private var frameUpdateCount: Int = 0

    private func updatePointBuffer(_ pointCloud: PointCloudFrame) {
        let count = pointCloud.pointCount
        guard count > 0 else { return }

        // Create interleaved buffer: [x, y, z, intensity] per point
        var vertices = [Float](repeating: 0, count: count * 4)
        for i in 0..<count {
            vertices[i * 4 + 0] = pointCloud.x[i]
            vertices[i * 4 + 1] = pointCloud.y[i]
            vertices[i * 4 + 2] = pointCloud.z[i]
            vertices[i * 4 + 3] = Float(pointCloud.intensity[i]) / 255.0
        }

        let bufferSize = vertices.count * MemoryLayout<Float>.stride
        pointBuffer = device.makeBuffer(
            bytes: vertices, length: bufferSize, options: .storageModeShared)
        pointCount = count
    }

    private func updateBoxInstances(_ trackSet: TrackSet) {
        // Each box instance: [transform matrix (16 floats) + colour (4 floats)]
        var instances = [Float]()

        for track in trackSet.tracks {
            // Build transform matrix
            let scale = simd_float4x4(
                diagonal: simd_float4(
                    track.bboxLengthAvg > 0 ? track.bboxLengthAvg : 1.0,
                    track.bboxWidthAvg > 0 ? track.bboxWidthAvg : 1.0,
                    track.bboxHeightAvg > 0 ? track.bboxHeightAvg : 1.0,
                    1.0
                ))

            let rotation = simd_float4x4(rotationZ: track.headingRad)
            let translation = simd_float4x4(translation: simd_float3(track.x, track.y, track.z))
            let transform = translation * rotation * scale

            // Add transform (16 floats)
            for col in 0..<4 {
                for row in 0..<4 {
                    instances.append(transform[col][row])
                }
            }

            // Add colour based on state (4 floats)
            let colour = track.state.colour
            instances.append(colour.r)
            instances.append(colour.g)
            instances.append(colour.b)
            instances.append(1.0)  // alpha
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

    private func updateTrailBuffer(_ trackSet: TrackSet) {
        // Trail vertices: [x, y, z, alpha] per vertex
        var vertices = [Float]()

        for trail in trackSet.trails {
            let pointCount = trail.points.count
            guard pointCount >= 2 else { continue }

            for (i, point) in trail.points.enumerated() {
                let alpha = Float(i) / Float(pointCount - 1)  // fade from 0 to 1
                vertices.append(point.x)
                vertices.append(point.y)
                vertices.append(0.1)  // slight Z offset
                vertices.append(alpha)
            }
        }

        if !vertices.isEmpty {
            let bufferSize = vertices.count * MemoryLayout<Float>.stride
            trailVertices = device.makeBuffer(
                bytes: vertices, length: bufferSize, options: .storageModeShared)
            trailVertexCount = vertices.count / 4
        } else {
            trailVertexCount = 0
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
        else {
            return
        }

        // Update uniforms
        uniforms.modelViewProjection = camera.projectionMatrix * camera.viewMatrix
        uniforms.modelView = camera.viewMatrix
        uniforms.pointSize = pointSize
        uniforms.time += 1.0 / 60.0

        encoder.setDepthStencilState(depthStencilState)

        // Draw point cloud
        if showPoints, let pipeline = pointCloudPipeline, let buffer = pointBuffer, pointCount > 0 {
            encoder.setRenderPipelineState(pipeline)
            encoder.setVertexBuffer(buffer, offset: 0, index: 0)
            encoder.setVertexBytes(&uniforms, length: MemoryLayout<Uniforms>.stride, index: 1)
            encoder.drawPrimitives(type: .point, vertexStart: 0, vertexCount: pointCount)
        }

        // Draw boxes
        if showBoxes, let pipeline = boxPipeline, let boxVerts = boxVertices,
            let instances = boxInstances, boxInstanceCount > 0
        {
            encoder.setRenderPipelineState(pipeline)
            encoder.setVertexBuffer(boxVerts, offset: 0, index: 0)
            encoder.setVertexBuffer(instances, offset: 0, index: 1)
            encoder.setVertexBytes(&uniforms, length: MemoryLayout<Uniforms>.stride, index: 2)
            // Draw wireframe boxes as lines (24 vertices per box = 12 edges)
            encoder.drawPrimitives(
                type: .line,
                vertexStart: 0,
                vertexCount: boxVertexCount,
                instanceCount: boxInstanceCount
            )
        }

        // Draw trails
        if showTrails, let pipeline = trailPipeline, let vertices = trailVertices,
            trailVertexCount > 0
        {
            encoder.setRenderPipelineState(pipeline)
            encoder.setVertexBuffer(vertices, offset: 0, index: 0)
            encoder.setVertexBytes(&uniforms, length: MemoryLayout<Uniforms>.stride, index: 1)
            encoder.drawPrimitives(type: .lineStrip, vertexStart: 0, vertexCount: trailVertexCount)
        }

        encoder.endEncoding()
        commandBuffer.present(drawable)
        commandBuffer.commit()
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

    var viewMatrix: simd_float4x4 {
        simd_float4x4(lookAt: target, from: position, up: up)
    }

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
                simd_float4(1, 0, 0, 0),
                simd_float4(0, 1, 0, 0),
                simd_float4(0, 0, 1, 0),
                simd_float4(t.x, t.y, t.z, 1)
            ))
    }

    init(rotationZ angle: Float) {
        let c = cos(angle)
        let s = sin(angle)
        self.init(
            columns: (
                simd_float4(c, s, 0, 0),
                simd_float4(-s, c, 0, 0),
                simd_float4(0, 0, 1, 0),
                simd_float4(0, 0, 0, 1)
            ))
    }

    init(lookAt target: simd_float3, from eye: simd_float3, up: simd_float3) {
        let z = normalize(eye - target)
        let x = normalize(cross(up, z))
        let y = cross(z, x)

        self.init(
            columns: (
                simd_float4(x.x, y.x, z.x, 0),
                simd_float4(x.y, y.y, z.y, 0),
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
                simd_float4(x, 0, 0, 0),
                simd_float4(0, y, 0, 0),
                simd_float4(0, 0, z, -1),
                simd_float4(0, 0, z * near, 0)
            ))
    }
}
