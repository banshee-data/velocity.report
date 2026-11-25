import * as THREE from "../vendor/three/three.module.js";

function face(positions, a, b, c, d) {
  for (const [x, y, z] of [a, b, c, a, c, d]) positions.push(x, z, -y);
}

export function tunnelSection(surface, station) {
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

export function validTunnelWallOpenings(openings, [start, end]) {
  if (openings === undefined) return true;
  return (
    openings !== null &&
    typeof openings === "object" &&
    !Array.isArray(openings) &&
    Object.entries(openings).every(
      ([side, ranges]) =>
        ["left", "right"].includes(side) &&
        Array.isArray(ranges) &&
        ranges.every(
          (range, i) =>
            Array.isArray(range) &&
            range.length === 2 &&
            range.every(Number.isFinite) &&
            start <= range[0] &&
            range[0] < range[1] &&
            range[1] <= end &&
            (i === 0 || ranges[i - 1][1] <= range[0]),
        ),
    )
  );
}

export function tunnelMesh(surface, appearance) {
  const { interior, clearance, wallOpenings = {} } = appearance;
  const [start, end] = interior;
  const profileStations = surface.profile
    .map(({ s_m }) => s_m)
    .filter((s) => s > start && s < end);
  const openingStations = Object.values(wallOpenings).flat(2);
  const stations = [
    ...new Set([start, ...profileStations, ...openingStations, end]),
  ].sort((a, b) => a - b);
  const open = (side, station) =>
    wallOpenings[side]?.some(([a, b]) => a < station && station < b);
  const shell = [];
  const stripes = [];
  const top = ([x, y, z]) => [x, y, z + clearance];
  let previous, previousStation;
  for (const station of stations) {
    const next = tunnelSection(surface, station);
    if (previous) {
      const [a, b] = previous;
      const [c, d] = next;
      const midpoint = (previousStation + station) / 2;
      if (!open("left", midpoint)) face(shell, a, c, top(c), top(a));
      if (!open("right", midpoint)) face(shell, b, top(b), top(d), d);
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
