import test from "node:test";
import assert from "node:assert/strict";
import {
  districtTrials,
  pointInDistrict,
  seekerDirection,
} from "../sf-district-trials.js";
import { createTrial, advanceTrial, questSeeker } from "../sf-drive-core.js";

const box = (a, b, c, d) => [
  [a, b],
  [c, b],
  [c, d],
  [a, d],
  [a, b],
];
const manifest = {
  origin_epsg7131: [48881.186, 25109.973],
  tiles: [{ t: "a" }, { t: "b" }, { t: "c" }],
};
const targets = [
  { t: "a", x: 1, y: 1, heading: 0, emoji: "🍎" },
  { t: "b", x: 20, y: 1, heading: 0, emoji: "🍒" },
];
function route() {
  return {
    targets,
    districts: {
      version: 1,
      scheme: "example-districts",
      revision: "one",
      frame: "EPSG:7131",
      origin_epsg7131: manifest.origin_epsg7131,
      areas: [
        {
          id: "west",
          name: "West",
          overlapTiles: ["a", "c"],
          targetTiles: ["a"],
          geometry: { type: "Polygon", coordinates: [box(0, 0, 10, 10)] },
        },
        {
          id: "east",
          name: "East",
          overlapTiles: ["b", "c"],
          targetTiles: ["b"],
          geometry: {
            type: "MultiPolygon",
            coordinates: [[box(10, 0, 30, 10)]],
          },
        },
      ],
    },
  };
}

test("districts are optional; overlap tiles and pickup ownership are separate", () => {
  assert.deepEqual(districtTrials({ targets }, manifest), []);
  const districts = districtTrials(route(), manifest);
  assert.equal(districts.length, 2);
  assert.deepEqual(districts[0].targets, [targets[0]]);
  assert.ok(
    districts.every((d) => d.overlapTiles.has("c")),
    "one cell can overlap both areas without duplicating a pickup",
  );
  assert.equal(districts[0].start.x, 1);
  assert.equal(districts[0].start.id, "district-west");
  const modified = route();
  modified.districts.revision = "two";
  assert.notEqual(
    districtTrials(modified, manifest)[0].recordScope,
    districts[0].recordScope,
  );
  const differentStart = route();
  differentStart.districts.areas = differentStart.districts.areas.slice(0, 1);
  differentStart.districts.areas[0].targetTiles = ["a", "b"];
  differentStart.districts.areas[0].geometry.coordinates = [box(0, 0, 30, 10)];
  const first = districtTrials(differentStart, manifest)[0];
  differentStart.districts.areas[0].targetTiles.reverse();
  assert.notEqual(
    districtTrials(differentStart, manifest)[0].recordScope,
    first.recordScope,
    "start changes separate records even when membership is identical",
  );
});

test("bad ownership, projection and geometry cannot create an easier partial run", () => {
  for (const change of [
    (r) => (r.districts.origin_epsg7131 = [0, 0]),
    (r) => (r.districts.areas[0].targetTiles = ["unknown"]),
    (r) => (r.districts.areas[0].targetTiles = ["b"]),
    (r) => (r.districts.areas[1].targetTiles = ["a"]),
    (r) =>
      (r.districts.areas[0].geometry.coordinates[0] = [
        [0, 0],
        [1, 1],
        [2, 2],
      ]),
    (r) => (r.districts.areas[0].id = "city"),
  ]) {
    const r = route();
    change(r);
    assert.throws(() => districtTrials(r, manifest));
  }
  const outside = route();
  outside.districts.areas[0].geometry.coordinates = [box(2, 2, 10, 10)];
  assert.throws(() => districtTrials(outside, manifest), /outside/);
});

test("district boundaries support islands, holes and edge points", () => {
  const geometry = [
    [box(0, 0, 10, 10), box(3, 3, 7, 7)],
    [box(20, 20, 30, 30)],
  ];
  assert.ok(pointInDistrict(0, 5, geometry));
  assert.ok(pointInDistrict(25, 25, geometry));
  assert.equal(pointInDistrict(5, 5, geometry), false);
  assert.equal(pointInDistrict(15, 15, geometry), false);
});

test("district trial finishes and seeks only its own emojis", () => {
  const [area] = districtTrials(route(), manifest),
    trial = createTrial(area.targets);
  const seeker = questSeeker(trial, [{ t: "b", x: 20, y: 1 }], {
    x: 20,
    y: 1,
    heading: 0,
  });
  assert.equal(seeker.target.t, "a");
  advanceTrial(trial, 1, { x: 20, y: 1 }, true);
  assert.equal(trial.collected.size, 0);
  advanceTrial(trial, 1, { x: 1, y: 1 }, true);
  assert.equal(trial.status, "finished");
  assert.equal(trial.collected.size, 1);
});

test("turn instructions clearly distinguish forward, left, right and behind", () => {
  assert.equal(seekerDirection(0), "Straight ahead");
  assert.equal(seekerDirection(-40), "Bear left");
  assert.equal(seekerDirection(90), "Turn right");
  assert.equal(seekerDirection(-179), "Turn around");
  assert.equal(seekerDirection(null), "All collected");
});
