import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";

// Reproduce four keyboard turns at the surveyed Oakdale and Dunshee junction.
const assets = new URL("../../public_html/src/sf/", import.meta.url);
const manifest = readFileSync(new URL("tiles/manifest.json", assets));
const manifestSha256 = createHash("sha256").update(manifest).digest("hex");
const { progressBaseSha256 } = JSON.parse(manifest).terrainRebake;
const { placementSha256 } = JSON.parse(
  readFileSync(new URL("quest-route.json", assets)),
);

const west = [3904.2, -2479.3];
const east = [4020, -2562.4];
const dunshee = [3932.6, -2549.8];
const junction = [3956.3, -2516.7];
const distance = (a, b) => Math.hypot(b[0] - a[0], b[1] - a[1]);
const mix = (a, b, t) => a.map((value, i) => value * (1 - t) + b[i] * t);
const shift = (p, offset) => p.map((value, i) => value + offset[i]);

function rightOf(a, b) {
  const scale = 1.4 / distance(a, b);
  return [(b[1] - a[1]) * scale, (a[0] - b[0]) * scale];
}

function waypoints(start, end) {
  const approach = rightOf(start, junction);
  const departure = rightOf(junction, end);
  const before = shift(
    mix(junction, start, 5 / distance(start, junction)),
    approach,
  );
  const apex = shift(junction, mix(approach, departure, 0.5));
  const after = shift(
    mix(junction, end, 5 / distance(junction, end)),
    departure,
  );
  const turn = Array.from({ length: 13 }, (_, i) => {
    const t = i / 12;
    return mix(mix(before, apex, t), mix(apex, after, t), t);
  });
  const route = [shift(start, approach), ...turn, shift(end, departure)];
  const stations = [0];
  for (let i = 1; i < route.length; i++)
    stations.push(stations[i - 1] + distance(route[i - 1], route[i]));
  const length = stations.at(-1);
  const steps = Math.ceil(length / 1.4);
  let segment = 1;
  return Array.from({ length: steps + 1 }, (_, i) => {
    const station = (i * length) / steps;
    while (stations[segment] < station && segment < route.length - 1) segment++;
    const fraction =
      (station - stations[segment - 1]) /
      (stations[segment] - stations[segment - 1]);
    const [x, y] = mix(route[segment - 1], route[segment], fraction);
    const speed = Math.min(
      5.2,
      2.8 + Math.max(0, Math.abs(station - stations[7]) - 3) * 0.155,
    );
    return [x, y, station, speed];
  });
}

for (const [id, start, end] of [
  ["west-to-dunshee", west, dunshee],
  ["east-to-dunshee", east, dunshee],
  ["dunshee-to-east", dunshee, east],
  ["dunshee-to-west", dunshee, west],
]) {
  console.log(
    JSON.stringify({
      id,
      manifestSha256,
      progressBaseSha256,
      placementSha256,
      finishOnStreet: true,
      expectedSurfaces: ["sf-contact/oakdale-avenue-caltrain-bridge"],
      maxAirborneMetres: 0,
      maxDips: 0,
      waypoints: waypoints(start, end),
    }),
  );
}
