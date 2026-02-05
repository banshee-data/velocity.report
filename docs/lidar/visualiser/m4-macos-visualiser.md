# M4 Track A: macOS Visualiser Cluster Rendering

## Overview

M4 Track A implements cluster bounding box rendering in the macOS visualiser, displaying detected foreground objects as coloured boxes alongside track visualisation.

## Features

### Cluster Box Rendering

Clusters are rendered as 3D wireframe boxes with:

- **Colour**: Cyan/blue (RGBA: 0.0, 0.8, 1.0, 0.7)
- **Transparency**: Slightly transparent (alpha=0.7) to distinguish from tracks
- **Dimensions**: AABB (axis-aligned bounding box) from cluster features
- **Orientation**: OBB heading if available, otherwise axis-aligned

### UI Controls

- **Toggle**: Press 'C' or use the toolbar button to show/hide clusters
- **State**: Stored in `AppState.showClusters`
- **Default**: Enabled

## Rendering Order

The visualiser draws objects in this order (back to front):

1. **Background points** (cached, grey)
2. **Foreground points** (dynamic, white/coloured by intensity)
3. **Cluster boxes** (cyan, semi-transparent) ← M4
4. **Track boxes** (colour-coded by state: green/yellow/red)
5. **Track trails** (motion history, fading lines)

This ensures tracks remain visually prominent over clusters.

## Track vs Cluster Boxes

| Property    | Clusters                  | Tracks                         |
| ----------- | ------------------------- | ------------------------------ |
| Colour      | Cyan (0, 0.8, 1.0)        | State-based (green/yellow/red) |
| Alpha       | 0.7 (semi-transparent)    | 1.0 (opaque)                   |
| Dimensions  | AABB (current frame)      | Running average bbox           |
| Orientation | OBB heading (if computed) | Smoothed heading               |
| Toggle      | 'C' button                | 'B' button                     |

## Implementation

### MetalRenderer Updates

**New Method**: `updateClusterInstances()`

```swift
private func updateClusterInstances(_ clusterSet: ClusterSet) {
    // Each box instance: [transform matrix (16 floats) + colour (4 floats)]
    var instances = [Float]()

    for cluster in clusterSet.clusters {
        // Build transform matrix using AABB dimensions
        let scale = simd_float4x4(diagonal: simd_float4(
            cluster.aabbLength > 0 ? cluster.aabbLength : 0.5,
            cluster.aabbWidth > 0 ? cluster.aabbWidth : 0.5,
            cluster.aabbHeight > 0 ? cluster.aabbHeight : 0.5,
            1.0))

        // Use OBB heading if available, otherwise no rotation
        let heading = cluster.obb?.headingRad ?? 0.0
        let rotation = simd_float4x4(rotationZ: heading)
        let translation = simd_float4x4(
            translation: simd_float3(
                cluster.centroidX,
                cluster.centroidY,
                cluster.centroidZ))
        let transform = translation * rotation * scale

        // Add transform (16 floats)
        for col in 0..<4 { for row in 0..<4 {
            instances.append(transform[col][row])
        }}

        // Add cyan colour (4 floats)
        instances.append(0.0)  // r
        instances.append(0.8)  // g
        instances.append(1.0)  // b
        instances.append(0.7)  // alpha (semi-transparent)
    }

    // Upload to GPU
    if !instances.isEmpty {
        let bufferSize = instances.count * MemoryLayout<Float>.stride
        clusterInstances = device.makeBuffer(
            bytes: instances,
            length: bufferSize,
            options: .storageModeShared)
        clusterInstanceCount = clusterSet.clusters.count
    } else {
        clusterInstanceCount = 0
    }
}
```

**New Properties**:

```swift
var clusterInstances: MTLBuffer?
var clusterInstanceCount: Int = 0
var showClusters: Bool = true
```

### Drawing Code

```swift
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
```

## Visual Examples

### Cluster States

**New Cluster (tentative)**:

- Cyan box with semi-transparency
- Small AABB dimensions
- No OBB heading yet (axis-aligned)

**Mature Cluster (before association)**:

- Cyan box with OBB heading
- Refined AABB dimensions
- Visible orientation

**Associated Cluster → Track**:

- Cluster box disappears (no longer in ClusterSet)
- Track box appears (green = confirmed)
- Smooth transition via confidence scores

## Performance

- **Cluster count**: Typically 5-20 per frame
- **GPU impact**: Negligible (same pipeline as track boxes)
- **CPU overhead**: ~0.1ms for instance buffer update

## Integration with M4 Backend

The Go backend (M4 Track B) sends clusters in every foreground frame:

```protobuf
message FrameBundle {
    PointCloudFrame point_cloud = 5;
    ClusterSet clusters = 6;        // M4: Clusters rendered as cyan boxes
    TrackSet tracks = 7;            // Tracks rendered as state-coloured boxes
}
```

## Future Enhancements

### M4.1: Track ID Labels

Optional enhancement to render track IDs as text labels near boxes:

- Use CoreText or Metal text rendering
- Position above track box centroid
- Fade in/out on track creation/deletion

### M4.2: Cluster Features

Display additional cluster metadata on hover:

- Point count
- Height P95
- Intensity mean
- Velocity estimate (if computed)

## See Also

- `docs/lidar/m4-tracking-milestone.md` - M4 backend tracking
- `docs/lidar/m3.5-macos-visualiser.md` - M3.5 split streaming
- `proto/velocity_visualiser/v1/visualiser.proto` - Cluster protobuf schema
