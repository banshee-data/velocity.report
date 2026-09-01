import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import {
  buildS2HilbertModel,
  isGeographicPointOnLand,
  prepareLandMask,
  projectWebMercator,
  splitWorldSegmentByLandMask,
} from "../index.mjs";

const LAND_MASK_URL = new URL("../land/sf-shoreline-and-islands.geojson", import.meta.url);

async function loadLandMask() {
  return JSON.parse(await readFile(LAND_MASK_URL, "utf8"));
}

test("a segment crossing a polygon is retained as water, land, water", () => {
  const mask = prepareLandMask({
    type: "Polygon",
    coordinates: [
      [
        [-1, -1],
        [1, -1],
        [1, 1],
        [-1, 1],
        [-1, -1],
      ],
    ],
  });
  const pieces = splitWorldSegmentByLandMask(
    projectWebMercator({ lat: 0, lng: -2 }, 0),
    projectWebMercator({ lat: 0, lng: 2 }, 0),
    mask,
  );
  assert.deepEqual(
    pieces.map((piece) => piece.classification),
    ["water", "land", "water"],
  );
  assert.deepEqual(pieces[0].end, pieces[1].start);
  assert.deepEqual(pieces[1].end, pieces[2].start);
});

test("polygon holes remain water", () => {
  const mask = prepareLandMask({
    type: "Polygon",
    coordinates: [
      [
        [-3, -3],
        [3, -3],
        [3, 3],
        [-3, 3],
        [-3, -3],
      ],
      [
        [-1, -1],
        [-1, 1],
        [1, 1],
        [1, -1],
        [-1, -1],
      ],
    ],
  });
  assert.equal(isGeographicPointOnLand({ lat: 2, lng: 0 }, mask), true);
  assert.equal(isGeographicPointOnLand({ lat: 0, lng: 0 }, mask), false);
});

test("the supplied shoreline recognises islands without treating the Bay Bridge as land", async () => {
  const mask = prepareLandMask(await loadLandMask(), -122.4);
  assert.equal(
    isGeographicPointOnLand({ lat: 37.823, lng: -122.37 }, mask),
    true,
    "Treasure Island should be land",
  );
  assert.equal(
    isGeographicPointOnLand({ lat: 37.797, lng: -122.38 }, mask),
    false,
    "the bridge crossing should remain water",
  );
});

test("the detailed traversal has both land and water pieces and stays complete", async () => {
  const model = buildS2HilbertModel({
    parent: "808581",
    targetLevel: 13,
    landMask: await loadLandMask(),
  });
  assert.equal(model.landMaskApplied, true);
  assert.ok(model.classifiedSegments.land.length > 0);
  assert.ok(model.classifiedSegments.water.length > 0);
  assert.equal(
    model.classifiedSegments.land.length + model.classifiedSegments.water.length >= 63,
    true,
  );
  const representedPairs = new Set(
    [...model.classifiedSegments.land, ...model.classifiedSegments.water].map(
      (segment) => `${segment.fromIndex}:${segment.toIndex}`,
    ),
  );
  assert.equal(representedPairs.size, 63);
});
