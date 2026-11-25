import * as THREE from "../vendor/three/three.module.js";
import { tunnelSection } from "./sf-tunnel-mesh.js";

const approach = 22;
const bore = 5;
const mouthUniforms = new WeakMap();

function through(section) {
  return section[0].map((value, axis) => (value + section[1][axis]) / 2);
}

export function tunnelMouths(surface, { interior, clearance }) {
  return interior
    .map((station, end) => {
      const section = tunnelSection(surface, station);
      const [x, y, z] = through(section);
      const [u, v, w] = through(
        tunnelSection(
          surface,
          station +
            (end ? -1 : 1) * Math.min(2, (interior[1] - interior[0]) / 2),
        ),
      );
      const length = Math.hypot(u - x, v - y);
      return {
        key: surface.id + ":" + end,
        section,
        x,
        y,
        z,
        nx: (u - x) / length,
        ny: (v - y) / length,
        grade: (w - z) / length,
        halfWidth:
          Math.hypot(
            section[1][0] - section[0][0],
            section[1][1] - section[0][1],
          ) / 2,
        clearance,
      };
    })
    .filter(({ nx, ny }) => Number.isFinite(nx) && Number.isFinite(ny));
}

// Matched covered ends are internal seams. A split into two bores can share
// the same seam, while two stacked approaches have distinct elevations.
export function exteriorTunnelMouths(mouths) {
  return mouths.filter(
    (mouth) =>
      !mouths.some(
        (other) =>
          mouth !== other &&
          Math.hypot(mouth.x - other.x, mouth.y - other.y) < 1.2 &&
          Math.abs(mouth.z - other.z) < 0.65 &&
          mouth.nx * other.nx + mouth.ny * other.ny < -0.5,
      ),
  );
}

export function mouthIntersectsTile(mouth, tile) {
  const distance = approach + mouth.halfWidth;
  return (
    Math.abs(mouth.x - tile.x) <= tile.half + distance &&
    Math.abs(mouth.y - tile.y) <= tile.half + distance
  );
}

export function showTunnelMouths(material, mouths) {
  const signature = mouths.map(({ key }) => key).join(" ");
  if ((material.userData.tunnelMouths ?? "") === signature) return;
  material.userData.tunnelMouths = signature;
  const size = mouths.length;
  if (!mouthUniforms.has(material)) mouthUniforms.set(material, new Map());
  const counts = mouthUniforms.get(material);
  if (!counts.has(size)) counts.set(size, { origin: {}, direction: {} });
  const { origin, direction } = counts.get(size);
  origin.value = mouths.map(
    ({ x, y, z, halfWidth }) => new THREE.Vector4(x, -y, z, halfWidth),
  );
  direction.value = mouths.map(
    ({ nx, ny, grade, clearance }) =>
      new THREE.Vector4(nx, -ny, grade, clearance),
  );
  material.customProgramCacheKey = () => "tunnel-mouths:" + size;
  material.onBeforeCompile = (shader) => {
    if (!size) return;
    // Material programs can be cached by aperture count as tiles stream.
    // Previously compiled variants keep these same uniform objects.
    shader.uniforms.tunnelOrigin = origin;
    shader.uniforms.tunnelDirection = direction;
    shader.vertexShader =
      "varying vec3 portalPosition;\n" +
      shader.vertexShader.replace(
        "#include <begin_vertex>",
        "#include <begin_vertex>\nportalPosition = position;",
      );
    shader.fragmentShader =
      `
      varying vec3 portalPosition;
      uniform vec4 tunnelOrigin[${size}];
      uniform vec4 tunnelDirection[${size}];
      ` +
      shader.fragmentShader.replace(
        "#include <clipping_planes_fragment>",
        `
        #include <clipping_planes_fragment>
        for (int i = 0; i < ${size}; i++) {
          vec4 origin = tunnelOrigin[i];
          vec4 direction = tunnelDirection[i];
          vec2 offset = portalPosition.xz - origin.xy;
          float along = dot(offset, direction.xy);
          float side = dot(offset, vec2(-direction.y, direction.x));
          float above = portalPosition.y - origin.z - direction.z * along;
          if (along > -${approach}.0 && along < ${bore}.0 &&
              abs(side) < origin.w && above > 0.08 &&
              (along < 0.1 || above < direction.w - 0.05)) discard;
        }
      `,
      );
  };
  material.needsUpdate = true;
}

export function tunnelMouthFrame(mouth, material) {
  const [left, right] = mouth.section;
  const dx = (left[0] - right[0]) / (2 * mouth.halfWidth);
  const dy = (left[1] - right[1]) / (2 * mouth.halfWidth);
  const shift = ([x, y, z], side, up) => [x + dx * side, y + dy * side, z + up];
  const topLeft = shift(left, 0, mouth.clearance);
  const topRight = shift(right, 0, mouth.clearance);
  const outerLeft = shift(topLeft, 0.45, 0.55);
  const outerRight = shift(topRight, -0.45, 0.55);
  const positions = [];
  function panel(a, b, c, d) {
    for (const [x, y, z] of [a, b, c, a, c, d]) positions.push(x, z, -y);
  }
  panel(left, topLeft, outerLeft, shift(left, 0.45, 0));
  panel(topLeft, topRight, outerRight, outerLeft);
  panel(topRight, right, shift(right, -0.45, 0), outerRight);
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute(
    "position",
    new THREE.Float32BufferAttribute(positions, 3),
  );
  geometry.computeVertexNormals();
  return new THREE.Mesh(geometry, material);
}
