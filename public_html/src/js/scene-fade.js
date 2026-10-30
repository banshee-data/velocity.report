// Sensor-centred range fade stays fixed while the camera circles or pans.
export const FADE_NEAR_M = 150;
export const FADE_FAR_M = 170;
/** Match the shader for DOM annotations that are anchored to faded geometry. */
export function sceneRangeOpacity(distance) {
  const t = Math.max(
    0,
    Math.min(1, (distance - FADE_NEAR_M) / (FADE_FAR_M - FADE_NEAR_M)),
  );
  return 1 - t * t * (3 - 2 * t);
}
export function sceneFade(material) {
  // Even otherwise opaque points must participate in alpha blending.
  material.transparent = true;
  material.onBeforeCompile = (shader) => {
    shader.vertexShader = shader.vertexShader
      .replace(
        "#include <common>",
        "#include <common>\nvarying vec3 vSceneRangePosition;",
      )
      .replace(
        "#include <project_vertex>",
        "#include <project_vertex>\nvSceneRangePosition = (modelMatrix * vec4(transformed, 1.0)).xyz;",
      );
    shader.fragmentShader = shader.fragmentShader
      .replace(
        "#include <common>",
        "#include <common>\nvarying vec3 vSceneRangePosition;",
      )
      .replace(
        "#include <opaque_fragment>",
        `diffuseColor.a *= 1.0 - smoothstep(${FADE_NEAR_M.toFixed(1)}, ${FADE_FAR_M.toFixed(1)}, length(vSceneRangePosition));\n#include <opaque_fragment>`,
      );
  };
  material.customProgramCacheKey = () => "scene-range-fade-150-170";
  return material;
}
