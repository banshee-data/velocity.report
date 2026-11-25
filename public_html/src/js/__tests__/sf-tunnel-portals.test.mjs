import test from "node:test";
import assert from "node:assert/strict";
import { MeshBasicMaterial } from "../../vendor/three/three.core.js";
import { buildBakedRoadContact } from "../sf-baked-road-contact.js";
import {
  exteriorTunnelMouths,
  mouthIntersectsTile,
  showTunnelMouths,
  tunnelMouthFrame,
  tunnelMouths,
} from "../sf-tunnel-portals.js";

function bore(id, x, z) {
  return buildBakedRoadContact({
    id,
    kind: "tunnel",
    points: [
      [x, 0, z],
      [x + 15, 0, z],
      [x + 30, 0, z],
    ],
    width: 8,
    owners: ["80858024", "80858024"],
    interior_range_m: [0, 30],
    game_clearance_m: 4.5,
  });
}

test("both ends are open while covered seams and stacked bores keep their levels", () => {
  const roads = [
    bore("west", 0, 10),
    bore("east", 30, 10),
    bore("lower", 30, 4),
  ];
  const mouths = exteriorTunnelMouths(
    roads.flatMap(({ surface, appearance }) =>
      tunnelMouths(surface, appearance),
    ),
  );
  assert.deepEqual(
    mouths.map(({ key }) => key),
    ["west:0", "east:1", "lower:0", "lower:1"],
  );
  assert.ok(mouthIntersectsTile(mouths[0], { x: 15, y: 0, half: 30 }));
  assert.ok(!mouthIntersectsTile(mouths[0], { x: 300, y: 0, half: 30 }));
  const material = new MeshBasicMaterial();
  const frame = tunnelMouthFrame(mouths[0], material);
  const positions = frame.geometry.attributes.position;
  for (let i = 0; i < positions.count; i++) {
    assert.equal(positions.getX(i), 0);
    assert.ok(Math.abs(positions.getZ(i)) >= 4 || positions.getY(i) >= 14.5);
  }
  showTunnelMouths(material, mouths.slice(0, 1));
  const shader = {
    vertexShader: "#include <begin_vertex>",
    fragmentShader: "#include <clipping_planes_fragment>",
    uniforms: {},
  };
  material.onBeforeCompile(shader);
  assert.deepEqual(
    shader.uniforms.tunnelOrigin.value[0].toArray(),
    [0, -0, 10, 4],
  );
  assert.match(shader.fragmentShader, /above < direction.w - 0.05/);
  const origin = shader.uniforms.tunnelOrigin;
  showTunnelMouths(material, mouths.slice(1, 2));
  assert.equal(material.customProgramCacheKey(), "tunnel-mouths:1");
  assert.equal(origin.value[0].x, 60);
  showTunnelMouths(material, mouths.slice(1, 3));
  showTunnelMouths(material, mouths.slice(2, 3));
  assert.equal(material.customProgramCacheKey(), "tunnel-mouths:1");
  assert.equal(origin.value[0].z, 4);
  showTunnelMouths(material, []);
  assert.equal(material.customProgramCacheKey(), "tunnel-mouths:0");
});
