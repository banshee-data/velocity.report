import assert from "node:assert/strict";
import test from "node:test";

import {
  latToTile,
  lonToTile,
  tilePlan,
  MIN_ZOOM,
  MAX_ZOOM,
} from "../fetch-basemap.mjs";

const SITES = [
  { lat: 37.7985, lon: -122.424 },
  { lat: 37.779, lon: -122.413 },
  { lat: 37.8065, lon: -122.436 },
];

test("tile coordinates follow the Web Mercator convention", () => {
  // Longitude increases eastward with x; latitude increases northward while y
  // increases southward, which is the sign flip worth pinning.
  assert.ok(lonToTile(-122.4, 13) > lonToTile(-122.5, 13));
  assert.ok(latToTile(37.9, 13) < latToTile(37.7, 13));
  assert.equal(lonToTile(-180, 5), 0);
});

test("the plan covers every site at every cached zoom", () => {
  const plan = tilePlan(SITES);
  for (let z = MIN_ZOOM; z <= MAX_ZOOM; z++) {
    const atZoom = plan.filter((t) => t.z === z);
    assert.ok(atZoom.length > 0, `no tiles planned for zoom ${z}`);
    for (const site of SITES) {
      const x = lonToTile(site.lon, z);
      const y = latToTile(site.lat, z);
      assert.ok(
        atZoom.some((t) => t.x === x && t.y === y),
        `zoom ${z} does not cover ${site.lat},${site.lon}`,
      );
    }
  }
});

test("the plan pads beyond the sites, because the frame is wider than they are", () => {
  // The map fits the sites to the container's height, so the width shows well
  // past them. Without padding a visitor sees bare grid either side.
  const plan = tilePlan(SITES).filter((t) => t.z === 13);
  const xs = plan.map((t) => t.x);
  const siteXs = SITES.map((s) => lonToTile(s.lon, 13));
  assert.ok(Math.min(...xs) < Math.min(...siteXs), "no padding to the west");
  assert.ok(Math.max(...xs) > Math.max(...siteXs), "no padding to the east");
});

test("a plan never asks for a tile outside the world", () => {
  const plan = tilePlan([{ lat: 85, lon: 179.9 }], 2, 3);
  for (const { z, x, y } of plan) {
    assert.ok(x >= 0 && x < 2 ** z, `x ${x} outside zoom ${z}`);
    assert.ok(y >= 0 && y < 2 ** z, `y ${y} outside zoom ${z}`);
  }
});
