// PointCloud.metal
// Metal shaders for point cloud rendering.
//
// Points are rendered as point sprites with size based on distance.
// Colour is derived from classification (foreground/background) and intensity.

#include <metal_stdlib>
using namespace metal;

// MARK: - Uniforms

struct Uniforms {
    float4x4 modelViewProjection;
    float4x4 modelView;
    float pointSize;
    float time;
    float2 padding;
};

// MARK: - Point Cloud Shaders

// Point data is now [x, y, z, intensity, classification] (5 floats per point)
// We read two float4 values per vertex to get all 5 components.

struct PointVertexOut {
    float4 position [[position]];
    float pointSize [[point_size]];
    float intensity;
    float classification;
    float depth;
};

vertex PointVertexOut pointVertex(
    uint vid [[vertex_id]],
    constant float *pointData [[buffer(0)]],
    constant Uniforms &uniforms [[buffer(1)]]
) {
    // Read 5 floats per point: x, y, z, intensity, classification
    uint baseIndex = vid * 5;
    float3 pos = float3(pointData[baseIndex], pointData[baseIndex + 1], pointData[baseIndex + 2]);
    float intensity = pointData[baseIndex + 3];
    float classification = pointData[baseIndex + 4];

    float4 viewPos = uniforms.modelView * float4(pos, 1.0);
    float4 clipPos = uniforms.modelViewProjection * float4(pos, 1.0);

    PointVertexOut out;
    out.position = clipPos;

    // Size attenuation based on distance
    float dist = length(viewPos.xyz);
    out.pointSize = uniforms.pointSize * 10.0 / max(dist, 1.0);
    out.pointSize = clamp(out.pointSize, 1.0, 20.0);

    out.intensity = intensity;
    out.classification = classification;
    out.depth = viewPos.z;

    return out;
}

fragment float4 pointFragment(
    PointVertexOut in [[stage_in]],
    float2 pointCoord [[point_coord]]
) {
    // Circular point sprite
    float2 centered = pointCoord - 0.5;
    float dist = length(centered);
    if (dist > 0.5) {
        discard_fragment();
    }

    // Soft edge
    float alpha = 1.0 - smoothstep(0.3, 0.5, dist);

    // Colour based on classification (primary) and intensity (secondary)
    // Classification values are integers passed as floats: 0=background, 1=foreground, 2=ground
    // Use epsilon-based comparison for exact integer matching
    float3 colour;
    if (abs(in.classification - 1.0) < 0.01) {
        // Foreground: green with intensity modulation
        float3 lowColour = float3(0.1, 0.6, 0.2);   // dark green
        float3 highColour = float3(0.4, 1.0, 0.4); // bright green
        colour = mix(lowColour, highColour, in.intensity);
    } else if (abs(in.classification - 2.0) < 0.01) {
        // Ground: brown/tan
        float3 lowColour = float3(0.4, 0.3, 0.2);   // dark brown
        float3 highColour = float3(0.7, 0.6, 0.4); // tan
        colour = mix(lowColour, highColour, in.intensity);
    } else {
        // Background (classification ~= 0): grey with intensity modulation
        float3 lowColour = float3(0.3, 0.3, 0.35);  // dark grey-blue
        float3 highColour = float3(0.6, 0.6, 0.65); // light grey
        colour = mix(lowColour, highColour, in.intensity);
    }

    return float4(colour, alpha);
}

// MARK: - Box Shaders

struct BoxInstance {
    float4x4 transform;
    float4 colour;
};

struct BoxVertexOut {
    float4 position [[position]];
    float4 colour;
    float3 normal;
};

vertex BoxVertexOut boxVertex(
    uint vid [[vertex_id]],
    uint iid [[instance_id]],
    constant packed_float3 *boxVerts [[buffer(0)]],
    constant BoxInstance *instances [[buffer(1)]],
    constant Uniforms &uniforms [[buffer(2)]]
) {
    BoxInstance inst = instances[iid];
    float3 localPos = float3(boxVerts[vid]);

    // Transform to world space
    float4 worldPos = inst.transform * float4(localPos, 1.0);

    BoxVertexOut out;
    out.position = uniforms.modelViewProjection * worldPos;
    out.colour = inst.colour;
    out.normal = float3(0, 0, 1); // simplified normal

    return out;
}

fragment float4 boxFragment(BoxVertexOut in [[stage_in]]) {
    // Simple shading
    float3 colour = in.colour.rgb;
    float alpha = in.colour.a * 0.5; // semi-transparent boxes

    return float4(colour, alpha);
}

// MARK: - Trail Shaders

struct TrailVertexOut {
    float4 position [[position]];
    float alpha;
};

vertex TrailVertexOut trailVertex(
    uint vid [[vertex_id]],
    constant float4 *vertices [[buffer(0)]],
    constant Uniforms &uniforms [[buffer(1)]]
) {
    float4 vert = vertices[vid];
    float3 pos = vert.xyz;
    float alpha = vert.w;

    TrailVertexOut out;
    out.position = uniforms.modelViewProjection * float4(pos, 1.0);
    out.alpha = alpha;

    return out;
}

fragment float4 trailFragment(TrailVertexOut in [[stage_in]]) {
    // Trail colour with fade
    float3 colour = float3(0.2, 0.8, 0.3); // green
    return float4(colour, in.alpha);
}
