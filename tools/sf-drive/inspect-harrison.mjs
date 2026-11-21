// Bounded map/contact diagnostic, including the user-located Harrison/Beale dip.
import fs from "node:fs";
import path from "node:path";
import { gunzipSync } from "node:zlib";
import { createHash } from "node:crypto";
import {
  groundAtWorld,
  tileAt,
} from "../../public_html/src/js/sf-drive-core.js";
const root = path.resolve(
  import.meta.dirname,
  "../../public_html/src/sf/tiles",
);
const bytes = fs.readFileSync(path.join(root, "manifest.json"));
const manifest = JSON.parse(bytes);
const tiles = manifest.tiles.map((row) =>
  JSON.parse(gunzipSync(fs.readFileSync(path.join(root, `${row.t}.json.gz`)))),
);
let roads = 0,
  segments = 0,
  samples = 0,
  missing = 0,
  seams = 0,
  maxStep = 0;
const owners = new Set();
for (const tile of tiles)
  for (const road of tile.r) {
    if (!/^Harrison (St|Street)$/i.test(tile.n[road[0]])) continue;
    roads++;
    const p = road[3];
    for (let i = 0; i + 3 < p.length; i += 2) {
      segments++;
      const ax = tile.o[0] + p[i] / 10,
        ay = tile.o[1] + p[i + 1] / 10;
      const bx = tile.o[0] + p[i + 2] / 10,
        by = tile.o[1] + p[i + 3] / 10;
      const n = Math.max(1, Math.ceil(Math.hypot(bx - ax, by - ay) / 2));
      let previous = null,
        owner = null;
      for (let j = 0; j <= n; j++) {
        const x = ax + ((bx - ax) * j) / n,
          y = ay + ((by - ay) * j) / n;
        const z = groundAtWorld(tiles, x, y, true),
          t = tileAt(manifest.tiles, x, y).t;
        samples++;
        owners.add(t);
        if (z === null) missing++;
        if (owner && owner !== t) seams++;
        if (z !== null && previous !== null)
          maxStep = Math.max(maxStep, Math.abs(z - previous));
        previous = z;
        owner = t;
      }
    }
  }
console.log(
  JSON.stringify(
    {
      manifestSha256: createHash("sha256").update(bytes).digest("hex"),
      roads,
      segments,
      samples,
      missing,
      seams,
      maxAdjacentHeightChangeM: maxStep,
      tiles: [...owners].sort(),
      limitation:
        "Centreline samples at <=2 m spacing; citywide summary only; the user-located junction probe follows. No independent source ground.",
    },
    null,
    2,
  ),
);

// User-located report: resolve paired Harrison/Beale signs, then project to Harrison.
// This is a local straight approach probe along the centreline tangent, not an asserted lane.
const { intersectionsForTile, nearestRoad, carStep } =
  await import("../../public_html/src/js/sf-drive-core.js");
const junction = tiles
  .flatMap((t) => intersectionsForTile(t, tiles))
  .find((p) => p.names.includes("Harrison St") && p.names.includes("Beale St"));
if (!junction)
  throw new Error("Harrison / Beale signs unavailable in this bake");
const road = nearestRoad(
  tiles,
  junction.x,
  junction.y,
  new Set(["Harrison St"]),
);
const localTiles = tiles.filter(
  (t) => Math.hypot(t.o[0] - road.x, t.o[1] - road.y) < 1800,
);
const hx = Math.cos(road.heading),
  hy = Math.sin(road.heading);
const profile = [];
for (let d = -100; d <= 100; d += 0.5) {
  const x = road.x + hx * d,
    y = road.y + hy * d;
  profile.push({
    d,
    x,
    y,
    z: groundAtWorld(tiles, x, y, true),
    tile: tileAt(manifest.tiles, x, y).t,
  });
}
const minimum = profile.reduce((a, b) => (a.z < b.z ? a : b));
const probes = [];
for (const direction of [1, -1]) {
  let car = {
    x: road.x - hx * 100 * direction,
    y: road.y - hy * 100 * direction,
    heading: road.heading + (direction < 0 ? Math.PI : 0),
    v: 20 / 2.2369362920544,
  };
  const trace = [];
  for (let tick = 0; tick < 120 * 40; tick++) {
    const ax = Math.cos(car.heading) * 1.35,
      ay = Math.sin(car.heading) * 1.35;
    const zf = groundAtWorld(tiles, car.x + ax, car.y + ay),
      zr = groundAtWorld(tiles, car.x - ax, car.y - ay);
    const currentRoad = nearestRoad(localTiles, car.x, car.y);
    const controls =
      car.v * 2.2369362920544 < 20 ? { throttle: 1 } : { brake: 0.3 };
    car = carStep(car, controls, 1 / 120, {
      slope: (zf - zr) / 2.7,
      offroad: !currentRoad || currentRoad.distance > currentRoad.width,
    });
    const z = groundAtWorld(tiles, car.x, car.y, true);
    const front = groundAtWorld(tiles, car.x + ax, car.y + ay),
      rear = groundAtWorld(tiles, car.x - ax, car.y - ay);
    const d = (car.x - road.x) * hx + (car.y - road.y) * hy;
    trace.push({
      tick,
      d,
      x: car.x,
      y: car.y,
      groundZ: z,
      renderedCarZ: Math.max(z, (front + rear) / 2),
      speedMph: car.v * 2.2369362920544,
      contact: "legacy height-following; no airborne state",
      tile: tileAt(manifest.tiles, car.x, car.y).t,
    });
    if (d * direction >= 100) break;
  }
  const low = trace.reduce((a, b) => (a.groundZ < b.groundZ ? a : b));
  probes.push({
    direction:
      direction > 0 ? "southwest along Harrison" : "northeast along Harrison",
    initialMph: 20,
    control: "full throttle below 20 mph; 30% brake otherwise",
    seconds: trace.length / 120,
    completed: trace.at(-1).d * direction >= 100,
    minimum: low,
    maxCarStepM: Math.max(
      ...trace
        .slice(1)
        .map((r, i) => Math.abs(r.renderedCarZ - trace[i].renderedCarZ)),
    ),
  });
  if (process.argv.includes("--trace"))
    console.log(JSON.stringify({ direction, trace }));
}
console.log(
  JSON.stringify(
    {
      junction,
      projectedHarrison: road,
      minimum,
      profileAt20m: profile.filter((r) => r.d % 20 === 0),
      probes,
      interpretation:
        "Dip reproduced in the baked height field in both directions; low point is inside 8085807c, not its seam. The city renderer and contact both use this single ground surface. No independent road/deck height evidence here: do not flatten, infill, or rebake from this diagnostic alone.",
    },
    null,
    2,
  ),
);
