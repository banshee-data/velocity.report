// CityTiles.metal
// Shaders for Gerardus's city tiles (lidar-city-view-plan Item 6).
//
// The tiles carry positions and indices only, no normals. Buildings are
// shaded flat from each triangle's own normal, taken from the screen-space
// derivatives of the view-space position; the ground and the street lines
// are drawn in one colour. Positions are Float32 metres from the tile's
// origin; the model-view is computed on the CPU in Double from the tile's
// offset to the camera, so the values here stay small near the eye.

#include <metal_stdlib>
using namespace metal;

struct CityTileUniforms {
    float4x4 modelViewProjection;
    float4x4 modelView;
    float4 colour;
    // x, y: the faded disc's centre in the tile's frame; z: its inner
    // radius (fully faded within); w: its outer radius (no fade beyond).
    // w == 0 turns the fade off.
    float4 fade;
    // x: 1 to shade flat from the triangle's normal, 0 for flat colour.
    float4 options;
};

struct CityTileOut {
    float4 position [[position]];
    float3 viewPosition;
    float2 tilePosition;
};

vertex CityTileOut cityTileVertex(
    uint vid [[vertex_id]],
    const device packed_float3 *positions [[buffer(0)]],
    constant CityTileUniforms &u [[buffer(1)]]
) {
    float3 p = float3(positions[vid]);
    CityTileOut out;
    out.position = u.modelViewProjection * float4(p, 1.0);
    out.viewPosition = (u.modelView * float4(p, 1.0)).xyz;
    out.tilePosition = p.xy;
    return out;
}

fragment float4 cityTileFragment(CityTileOut in [[stage_in]], constant CityTileUniforms &u [[buffer(1)]]) {
    float4 colour = u.colour;
    if (u.options.x > 0.5) {
        float3 n = normalize(cross(dfdx(in.viewPosition), dfdy(in.viewPosition)));
        // A light from over the viewer's shoulder, in view space.
        float3 light = normalize(float3(0.35, 0.6, 0.7));
        float shade = 0.45 + 0.55 * abs(dot(n, light));
        colour.rgb *= shade;
    }
    if (u.fade.w > 0.0) {
        float d = distance(in.tilePosition, u.fade.xy);
        float keep = smoothstep(u.fade.z, u.fade.w, d);
        colour.a *= keep;
        if (colour.a < 0.01) discard_fragment();
    }
    return colour;
}
