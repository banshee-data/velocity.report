import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { gunzipSync } from "node:zlib";
import { buildBakedRoadContact } from "../sf-baked-road-contact.js";
import {
  buildCollisionIndex,
  collisionAt,
  groundAtWorld,
} from "../sf-drive-core.js";

const path = (file) => new URL(`../../sf/${file}`, import.meta.url);
const file = (name) => readFileSync(path(name));
const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");

test("bundled road runs have identical geometry across tile owners", () => {
  const manifestBytes = file("tiles/manifest.json");
  const { tiles, roadBake, terrainReview } = JSON.parse(manifestBytes);
  const driveBytes = file("gamedrives.json");
  assert.equal(digest(driveBytes), roadBake.driveSha256);
  assert.equal(
    JSON.parse(driveBytes).base_manifest_sha256,
    roadBake.baseSha256,
  );
  assert.equal(
    JSON.parse(file("quest-route.json")).manifestSha256,
    digest(manifestBytes),
  );
  const runs = new Map();
  const seamKinds = new Set();
  for (const { t, bytes } of tiles) {
    const tile = file(`tiles/${t}.json.gz`);
    assert.equal(tile.length, bytes, t);
    for (const run of JSON.parse(gunzipSync(tile)).raisedRoads || []) {
      assert.ok(run.points.length > 1, run.id);
      assert.ok(
        run.points.every(
          (point) => point.length === 3 && point.every(Number.isFinite),
        ),
        run.id,
      );
      if (runs.has(run.id)) {
        assert.deepEqual(run, runs.get(run.id), run.id);
        seamKinds.add(run.kind);
      }
      runs.set(run.id, run);
    }
  }
  const reviewedRoads = new Set(terrainReview?.addedRoads);
  for (const id of reviewedRoads) assert.ok(runs.has(id), id);
  assert.equal(runs.size, roadBake.runs + reviewedRoads.size);
  assert.deepEqual(seamKinds, new Set(["bridge", "overpass"]));
});

test("the rebake keeps tunnel alignments off the aerial ground while retaining Tunnel Avenue", () => {
  const { tiles, terrainRebake } = JSON.parse(file("tiles/manifest.json"));
  const tunnelNames = new Set([
    "Robert C Levy Tunnel",
    "Stockton Tunnel",
    "Toll Plaza Tunnel",
  ]);
  assert.equal(
    Object.values(terrainRebake.removedTunnelRows).reduce((a, b) => a + b),
    4,
  );
  let tunnelAvenue = false;
  for (const { t } of tiles) {
    const { n, r } = JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`)));
    for (const [index] of r) {
      assert.ok(!tunnelNames.has(n[index]), t);
      if (n[index] === "Tunnel Ave") tunnelAvenue = true;
    }
  }
  assert.ok(tunnelAvenue);
});

test("the playable I-280 parts keep identical contact across neighbouring owners", () => {
  const { tiles, roadBake } = JSON.parse(file("tiles/manifest.json"));
  assert.match(roadBake.playableSha256, /^[0-9a-f]{64}$/);
  const roads = new Map();
  const duplicated = new Set();
  const terminals = new Map();
  for (const { t } of tiles) {
    const { playableRoads = [] } = JSON.parse(
      gunzipSync(file(`tiles/${t}.json.gz`)),
    );
    for (const road of playableRoads) {
      if (roads.has(road.id)) {
        assert.deepEqual(road, roads.get(road.id), road.id);
        duplicated.add(road.id);
      }
      roads.set(road.id, road);
      terminals.set(`${road.id}:start`, [road.points[0], road.seams?.start]);
      terminals.set(`${road.id}:end`, [road.points.at(-1), road.seams?.end]);
    }
  }
  const entry = roads.get("sf-contact/i280-fifth-south-entry");
  assert.deepEqual(entry.groundEnds, ["start"]);
  assert.ok(duplicated.has(entry.id));
  for (const { from, to } of roadBake.playableTransitions) {
    const terminal = terminals.get(from);
    assert.ok(terminal?.[1], from);
    assert.deepEqual(terminal, terminals.get(to));
  }
  const fork = "sf-contact/i280-18th-off-ramp:end";
  const exits = [
    "sf-contact/i280-18th-exit",
    "sf-contact/i280-pennsylvania-exit",
  ];
  assert.deepEqual(
    roadBake.playableTransitions
      .filter(({ from }) => from === fork)
      .map(({ to }) => to)
      .sort(),
    exits.map((exit) => `${exit}:start`),
  );
  for (const exit of exits)
    assert.deepEqual(roads.get(exit).groundEnds, ["end"]);
  const stockton = roads.get("sf-contact/stockton-tunnel");
  assert.deepEqual(stockton.groundEnds, ["start", "end"]);
  assert.equal(stockton.kind, "tunnel");
  assert.ok(
    Math.abs(
      stockton.interior_range_m[1] - stockton.interior_range_m[0] - 276.44,
    ) < 0.01,
  );
});

test("Broadway's lower lanes clear the building while its upper surfaces stay solid", () => {
  const owners = ["808580ec", "808580f4"];
  const tiles = owners.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  assert.deepEqual(
    owners.map((t) => roadBake.overheadBuildingPatches[t]),
    [1, 2],
  );
  const bores = tiles.map(({ playableRoads }) =>
    playableRoads.filter(({ id }) => id.startsWith("sf-contact/broadway-")),
  );
  assert.deepEqual(bores[0], bores[1]);
  assert.deepEqual(
    bores[0].map(({ id }) => id),
    ["sf-contact/broadway-westbound", "sf-contact/broadway-eastbound"],
  );
  for (const road of bores[0])
    assert.deepEqual(road.groundEnds, ["start", "end"]);

  const { surface } = buildBakedRoadContact(bores[0][0]);
  const indexes = tiles.map((tile) => buildCollisionIndex(tile, tile.t));
  const entry = { x: 2529.5, y: 4132.1, heading: -2.917 };
  const floor = surface.heightAt(entry.x, entry.y);
  assert.ok(floor > 40 && floor < 41);
  assert.equal(collisionAt(indexes, entry, floor), null);
  assert.equal(collisionAt(indexes, entry, 55), "building");

  const east = 2191.961;
  const north = 4077.38;
  assert.ok(Math.abs(surface.heightAt(east, north) - 52.91) < 0.1);
  assert.ok(Math.abs(groundAtWorld(tiles, east, north, true) - 93.81) < 0.1);
});

test("Presidio's four bores turn into continuous contact at their portals", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808586d4", "808586dc", "808586ec"].map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) => playableRoads.map((r) => [r.id, r])),
  );
  const transitions = new Map(
    roadBake.playableTransitions.map(({ from, to }) => [from, to]),
  );
  for (const [direction, bores] of [
    ["nb", ["main-post-northbound", "battery-northbound"]],
    ["sb", ["battery-southbound", "main-post-southbound"]],
  ]) {
    const start = `sf-contact/presidio-${direction}-entry`;
    const end = `sf-contact/presidio-${direction}-exit`;
    let id = start;
    const tunnels = [];
    const visited = new Set();
    while (id) {
      assert.ok(!visited.has(id), `Road cycle at ${id}`);
      visited.add(id);
      const road = roads.get(id);
      assert.ok(road, id);
      const baked = buildBakedRoadContact(road);
      assert.ok(baked, id);
      if (road.kind === "tunnel") {
        assert.ok(baked.appearance, id);
        tunnels.push(id);
      }
      for (const terminal of road.groundEnds) {
        const [x, y, z] = road.points.at(terminal === "start" ? 0 : -1);
        assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03, id);
      }
      for (let i = 1; i < road.points.length; i++) {
        const [x, y, z] = road.points[i];
        const [px, py, pz] = road.points[i - 1];
        assert.ok(Math.abs(z - pz) / Math.hypot(x - px, y - py) < 0.075, id);
      }
      id = transitions.get(`${id}:end`)?.replace(/:start$/, "");
    }
    assert.deepEqual(
      tunnels,
      bores.map((bore) => `sf-contact/${bore}`),
    );
    assert.deepEqual(roads.get(start).groundEnds, ["start"]);
    assert.deepEqual(roads.get(end).groundEnds, ["end"]);
    assert.equal([...visited].at(-1), end);
  }

  const [north, , plaza] = tiles;
  for (const [tile, name, entry, upper] of [
    [north, "main-post-southbound", { x: -1038.3, y: 4807.3, heading: 0 }, 8.3],
    [
      plaza,
      "toll-plaza-tunnel",
      { x: -3169.55, y: 5207.47, heading: 0.625 },
      55.4,
    ],
  ]) {
    const road = roads.get(`sf-contact/${name}`);
    const { surface, appearance, portals } = buildBakedRoadContact(road);
    assert.ok(appearance, name);
    if (tile === plaza) {
      assert.equal(portals.length, 2);
      for (const { x, y, z } of portals)
        assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
    }
    const floor = surface.heightAt(entry.x, entry.y);
    assert.ok(Number.isFinite(floor), name);
    const indexes = [buildCollisionIndex(tile, tile.t)];
    assert.equal(collisionAt(indexes, entry, floor), null, name);
    assert.equal(collisionAt(indexes, entry, upper), "building", name);
  }
});

test("Harrison and Silver Avenue decks span the lower roads at independent heights", () => {
  const cases = [
    [
      "8085807c",
      "harrison-beale-bridge",
      4300.061,
      3048.916,
      15.58,
      8.0,
      0.075,
    ],
    [
      "808f7efc",
      "silver-avenue-bridge",
      3058.179,
      -3041.466,
      22.69,
      16.21,
      0.03,
    ],
  ];
  for (const [token, id, x, y, upper, lower, maxGrade] of cases) {
    const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
    const road = tile.playableRoads.find(
      (entry) => entry.id === `sf-contact/${id}`,
    );
    const { surface, portals } = buildBakedRoadContact(road);
    assert.equal(surface.kind, "bridge_deck", id);
    assert.equal(portals.length, 2, id);
    for (const portal of portals)
      assert.ok(
        Math.abs(groundAtWorld([tile], portal.x, portal.y, true) - portal.z) <
          0.03,
        id,
      );
    assert.ok(Math.abs(surface.heightAt(x, y) - upper) < 0.03, id);
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - lower) < 0.03, id);
    for (let i = 1; i < road.points.length; i++) {
      const a = road.points[i - 1],
        b = road.points[i];
      assert.ok(
        Math.abs(b[2] - a[2]) / Math.hypot(b[0] - a[0], b[1] - a[1]) < maxGrade,
        id,
      );
    }
  }
});
