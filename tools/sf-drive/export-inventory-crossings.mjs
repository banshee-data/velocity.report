import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { streetCourseWaypoints } from "./steering-line.mjs";

const assets = new URL("../../public_html/src/sf/", import.meta.url);
const read = (path) => readFileSync(new URL(path, assets));
const manifest = read("tiles/manifest.json");
const manifestSha256 = createHash("sha256").update(manifest).digest("hex");
const { progressBaseSha256 } = JSON.parse(manifest).terrainRebake;
const { placementSha256 } = JSON.parse(read("quest-route.json"));

// Test the mapped approaches in the game-local metre frame. The way IDs
// identify the frozen inventory alignments discussed in the road audit.
const crossings = [
  {
    id: "third-street-outer-northbound",
    way: 27656674,
    points: [
      [4399.955, 1841.119],
      [4376.091, 1879.141],
    ],
    lanes: [{ laneOffset: 0 }],
    approach: 14,
    departure: 22,
    speed: 9,
  },
  {
    id: "third-street-outer-southbound",
    way: 674496104,
    points: [
      [4365.057, 1872.031],
      [4389.44, 1835.019],
    ],
    lanes: [{ laneOffset: 0 }],
    approach: 14,
    departure: 22,
    speed: 9,
  },
  {
    id: "third-street-inner",
    way: 1006830791,
    points: [
      [4395.059, 1838.031],
      [4370.851, 1875.697],
    ],
    lanes: [{ suffix: "northbound" }, { suffix: "southbound", reverse: true }],
    laneOffset: 1.7,
    approach: 14,
    departure: 22,
    speed: 9,
  },
  {
    id: "evans-rail-crossing",
    way: 283080371,
    points: [
      [3781.506, -1190.579],
      [3776.66, -1178.261],
      [3770.279, -1161.705],
    ],
  },
  {
    id: "mariposa-rail-crossing",
    way: 544431983,
    points: [
      [4174.583, 457.317],
      [4132.115, 453.773],
    ],
  },
  {
    id: "first-street-transit-center",
    way: 175507524,
    points: [
      [3775.0047, 3267.6631],
      [3830.9465, 3211.2273],
    ],
    lanes: [{ laneOffset: 0 }],
    approach: 0,
    departure: 0,
    speed: 6.7,
  },
];

function extend([x, y], [a, b], distance) {
  const length = Math.hypot(x - a, y - b);
  return [x + ((x - a) * distance) / length, y + ((y - b) * distance) / length];
}

for (const {
  id,
  way,
  points,
  lanes = [{ suffix: "forward" }, { suffix: "reverse", reverse: true }],
  laneOffset = 1.45,
  approach = 12,
  departure = 16,
  speed = 8,
} of crossings) {
  for (const lane of lanes) {
    const alignment = lane.reverse ? points.toReversed() : points;
    const line = [
      extend(alignment[0], alignment[1], approach),
      ...alignment,
      extend(alignment.at(-1), alignment.at(-2), departure),
    ];
    const length = line.slice(1).reduce((distance, [x, y], i) => {
      const [a, b] = line[i];
      return distance + Math.hypot(x - a, y - b);
    }, 0);
    console.log(
      JSON.stringify({
        id: lane.suffix ? `${id}-${lane.suffix}` : id,
        way,
        manifestSha256,
        progressBaseSha256,
        placementSha256,
        finishOnStreet: true,
        expectedSurfaces: [],
        expectedEnclosed: [],
        expectNoScrapes: true,
        maxDips: 0,
        maxAirborneMetres: 0,
        waypoints: streetCourseWaypoints(
          line,
          0,
          Math.floor(length) - 1,
          speed,
          {
            laneOffset: lane.laneOffset ?? laneOffset,
            spacing: 1,
          },
        ),
      }),
    );
  }
}
