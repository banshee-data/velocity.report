import * as THREE from "../vendor/three/three.module.js";
import { SCENE_COLOURS } from "./scene-colours.js";

export function surveyBeacons(sites) {
  const height = 720;
  const material = new THREE.ShaderMaterial({
    uniforms: {
      time: { value: 0 },
      colour: { value: new THREE.Color(SCENE_COLOURS.cycle) },
    },
    transparent: true,
    depthWrite: false,
    side: THREE.BackSide,
    vertexShader: `
      varying vec2 beaconUv;
      void main() {
        beaconUv = uv;
        gl_Position = projectionMatrix * modelViewMatrix * instanceMatrix * vec4(position, 1.0);
      }
    `,
    fragmentShader: `
      uniform float time;
      uniform vec3 colour;
      varying vec2 beaconUv;
      void main() {
        float filament = pow(max(0.0, cos(beaconUv.x * 113.0973)), 12.0);
        float flow = 0.55 + 0.45 * sin(beaconUv.y * 36.0 - time * 0.8 + beaconUv.x * 12.5664);
        float fade = 1.0 - smoothstep(0.48, 1.0, beaconUv.y);
        gl_FragColor = vec4(colour, fade * (0.065 + 0.38 * filament * flow));
        #include <colorspace_fragment>
      }
    `,
  });
  const mesh = new THREE.InstancedMesh(
    new THREE.CylinderGeometry(58, 58, height, 48, 1, true),
    material,
    sites.length,
  );
  const matrix = new THREE.Matrix4();
  for (const [i, site] of sites.entries())
    mesh.setMatrixAt(
      i,
      matrix.makeTranslation(
        site.x,
        site.transform[2][3] + height / 2,
        -site.y,
      ),
    );
  mesh.instanceMatrix.needsUpdate = true;
  return mesh;
}
