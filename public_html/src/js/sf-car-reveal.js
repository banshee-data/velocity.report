import * as THREE from "../vendor/three/three.module.js";

// All occluder materials share the projected car and the current viewport.
// Pixel dithering feathers the edge while preserving opaque depth writes.
export function createCarReveal() {
  const circle = { value: new THREE.Vector4() };
  const feather = { value: 3 };
  const centre = new THREE.Vector3();
  const screen = new THREE.Vector3();
  const radius = 2.6; // m, bounds the body and wheels around the centre
  return {
    enableOn(material) {
      material.onBeforeCompile = (shader) => {
        shader.uniforms.carReveal = circle;
        shader.uniforms.carRevealFeather = feather;
        shader.fragmentShader = shader.fragmentShader
          .replace(
            "#include <common>",
            `#include <common>
          uniform vec4 carReveal;
          uniform float carRevealFeather;`,
          )
          .replace(
            "#include <alphatest_fragment>",
            `#include <alphatest_fragment>
          if (carReveal.z > 0.0 && gl_FragCoord.z < carReveal.w) {
            float distanceToCar = length(gl_FragCoord.xy - carReveal.xy);
            if (distanceToCar < carReveal.z + carRevealFeather) {
              float opacity = smoothstep(
                carReveal.z, carReveal.z + carRevealFeather, distanceToCar
              );
              float threshold = fract(52.9829189 *
                fract(dot(floor(gl_FragCoord.xy), vec2(0.06711056, 0.00583715))));
              if (opacity <= threshold) discard;
            }
          }`,
          );
      };
    },
    update(camera, carPosition, width, height, pixelRatio) {
      camera.updateMatrixWorld();
      centre.copy(carPosition);
      centre.y += 0.9;
      centre.applyMatrix4(camera.matrixWorldInverse);
      screen.copy(centre).applyMatrix4(camera.projectionMatrix);
      // Use the nearest possible point for the radius and the far side for
      // the depth cutoff, so the full car fits inside the reveal.
      const scale = camera.projectionMatrix.elements[5] * height * 0.5;
      const pixels = (radius * scale) / (-centre.z - radius);
      centre.z -= radius;
      centre.applyMatrix4(camera.projectionMatrix);
      circle.value.set(
        (screen.x + 1) * width * 0.5,
        (screen.y + 1) * height * 0.5,
        pixels,
        (centre.z + 1) * 0.5,
      );
      feather.value = 3 * pixelRatio;
    },
    disable() {
      circle.value.z = 0;
    },
  };
}
