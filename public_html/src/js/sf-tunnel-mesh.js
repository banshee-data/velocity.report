import * as THREE from "../vendor/three/three.module.js";

function face(positions, a, b, c, d) {
  for (const [x, y, z] of [a, b, c, a, c, d]) positions.push(x, z, -y);
}

function section(surface, station) {
  const { profile, contact } = surface;
  const after = profile.findIndex(({ s_m }) => s_m >= station);
  const i = Math.max(0, after - 1);
  const j = Math.min(profile.length - 1, i + 1);
  const u = (station - profile[i].s_m) / (profile[j].s_m - profile[i].s_m);
  return [0, 1].map((side) => {
    const a = contact.vertices[i * 2 + side];
    const b = contact.vertices[j * 2 + side];
    return a.map((value, axis) => value + (b[axis] - value) * u);
  });
}

export function tunnelMesh(surface, appearance) {
  const { interior, clearance } = appearance;
  const [start, end] = interior;
  const stations = [
    start,
    ...surface.profile
      .map(({ s_m }) => s_m)
      .filter((s) => s > start && s < end),
    end,
  ];
  const shell = [];
  const stripes = [];
  const top = ([x, y, z]) => [x, y, z + clearance];
  let previous, previousStation;
  for (const station of stations) {
    const next = section(surface, station);
    if (previous) {
      const [a, b] = previous;
      const [c, d] = next;
      face(shell, a, c, top(c), top(a));
      face(shell, b, top(b), top(d), d);
      face(shell, top(a), top(c), top(d), top(b));
      if (Math.floor(previousStation / 12) !== Math.floor(station / 12)) {
        for (const fraction of [0.22, 0.78]) {
          const lamp = (p, q, u) => [
            p[0] + (q[0] - p[0]) * u,
            p[1] + (q[1] - p[1]) * u,
            p[2] + clearance - 0.06,
          ];
          face(
            stripes,
            lamp(a, b, fraction),
            lamp(c, d, fraction),
            lamp(c, d, fraction + 0.02),
            lamp(a, b, fraction + 0.02),
          );
        }
      }
    }
    previousStation = station;
    previous = next;
  }
  const mesh = (positions, material) => {
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute(
      "position",
      new THREE.Float32BufferAttribute(positions, 3),
    );
    geometry.computeVertexNormals();
    return new THREE.Mesh(geometry, material);
  };
  const group = new THREE.Group();
  group.add(
    mesh(
      shell,
      new THREE.MeshLambertMaterial({
        color: 0x59636a,
        side: THREE.DoubleSide,
      }),
    ),
    mesh(
      stripes,
      new THREE.MeshBasicMaterial({ color: 0xffe3a1, side: THREE.DoubleSide }),
    ),
  );
  return group;
}
