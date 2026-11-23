import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { gunzipSync } from "node:zlib";
import { buildBakedRoadContact } from "../sf-baked-road-contact.js";
import {
  BAY_BRIDGE_CITY_DRIVE,
  cityDriveApproach,
} from "../sf-city-road-drive.js";
import {
  buildCollisionIndex,
  collisionAt,
  groundAtWorld,
} from "../sf-drive-core.js";

const path = (file) => new URL(`../../sf/${file}`, import.meta.url);
const file = (name) => readFileSync(path(name));
const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");

test("Yerba Buena connects continuously through the upper tunnel to the original bridge drive", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const drive = JSON.parse(file("gamedrives.json")).drives.find(
    ({ id }) => id === BAY_BRIDGE_CITY_DRIVE,
  );
  const entry = roadBake.driveApproaches.find(
    ({ id }) => id === "yerba-buena-westbound",
  );
  const selected = cityDriveApproach(drive, entry.id);
  assert.equal(selected.label, entry.label);
  assert.equal(selected.initial_speed_mps, 50 * 0.44704);
  assert.equal(cityDriveApproach(drive, "fremont-exit"), drive);
  assert.equal(drive.sections[0].id, "bay-bridge-westbound");
  assert.deepEqual(selected.sections.slice(3), drive.sections);
  const [approach, tunnel, exit, bridge] = selected.sections;
  assert.equal(tunnel.kind, "tunnel");
  assert.equal(tunnel.game_clearance_m, 4.4);
  assert.equal(tunnel.width_m, 16);
  for (const [before, after] of [
    [approach, tunnel],
    [tunnel, exit],
    [exit, bridge],
  ]) {
    assert.deepEqual(before.positions.at(-1), after.positions[0]);
  }
  let length = 0;
  for (let i = 1; i < tunnel.positions.length; i++) {
    const [ax, ay, az] = tunnel.positions[i - 1];
    const [bx, by, bz] = tunnel.positions[i];
    const ds = Math.hypot(bx - ax, by - ay);
    assert.ok(Math.abs(bz - az) / ds < 0.045);
    length += ds;
  }
  assert.ok(length > 164 && length < 166);
});

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

test("southbound I-280 retains both 18th Street fork branches and levels", () => {
  const t = "808f7fcc";
  const tile = JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`)));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const fork = "sf-contact/i280-king-to-mariposa:end";
  const through = "sf-contact/i280-south-through-18th";
  assert.deepEqual(
    roadBake.playableTransitions
      .filter(({ from }) => from === fork)
      .map(({ to }) => to)
      .sort(),
    ["sf-contact/i280-18th-off-ramp:start", `${through}:start`],
  );
  const road = tile.playableRoads.find(({ id }) => id === through);
  const { surface } = buildBakedRoadContact(road);
  const underRamp = surface.heightAt(4161.917, 311.135);
  assert.ok(underRamp > 20.3 && underRamp < 20.6);
  for (let i = 1; i < road.points.length; i++) {
    const [ax, ay, az] = road.points[i - 1];
    const [bx, by, bz] = road.points[i];
    assert.ok(Math.abs(bz - az) / Math.hypot(bx - ax, by - ay) < 0.04);
  }
});

test("Dogpatch keeps four ramp portals and the southbound through lane connected", () => {
  const tokens = ["808f7fb4", "808f7fac", "808f7efc"];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const prefix = "sf-contact/i280-";
  const road = (slug) =>
    tiles
      .flatMap(({ playableRoads }) => playableRoads)
      .find(({ id }) => id === prefix + slug);
  const connections = (slug) =>
    roadBake.playableTransitions
      .filter(({ from }) => from === `${prefix}${slug}:end`)
      .map(({ to }) => to.slice(prefix.length, -6))
      .sort();
  assert.deepEqual(connections("dogpatch-approach"), [
    "dogpatch-off-ramp",
    "south-through-dogpatch",
  ]);
  assert.deepEqual(connections("dogpatch-off-ramp"), [
    "dogpatch-pennsylvania-north",
    "dogpatch-pennsylvania-south",
  ]);
  for (const approach of [
    "dogpatch-pennsylvania-on-north",
    "dogpatch-pennsylvania-on-south",
  ])
    assert.deepEqual(connections(approach), ["dogpatch-on-ramp"]);
  for (const approach of ["dogpatch-on-ramp", "south-through-dogpatch"])
    assert.deepEqual(connections(approach), ["south-beyond-dogpatch"]);
  for (const slug of [
    "dogpatch-pennsylvania-north",
    "dogpatch-pennsylvania-south",
    "dogpatch-pennsylvania-on-north",
    "dogpatch-pennsylvania-on-south",
    "south-beyond-dogpatch",
  ]) {
    const { portals } = buildBakedRoadContact(road(slug));
    assert.equal(portals.length, 1, slug);
    const [{ x, y, z }] = portals;
    const pavement = groundAtWorld(tiles, x, y, true);
    assert.ok(Number.isFinite(pavement) && Math.abs(pavement - z) < 0.03, slug);
  }
  const { surface: approach } = buildBakedRoadContact(
    road("dogpatch-approach"),
  );
  const [x, y] = [4192.36, -173.02];
  assert.ok(Math.abs(approach.heightAt(x, y) - 23.48) < 0.05);
  assert.ok(groundAtWorld(tiles, x, y, true) < 11);
  const indexes = tiles.map((tile) => buildCollisionIndex(tile, tile.t));
  const { surface } = buildBakedRoadContact(road("south-through-dogpatch"));
  assert.equal(roadBake.overheadBuildingPatches["808f7fac"], 2);
  for (const [x, y, heading] of [
    [4262.41, -875.6, -1.67],
    [4230.53, -1027.3, -1.92],
  ]) {
    const z = surface.heightAt(x, y);
    assert.ok(Number.isFinite(z));
    assert.equal(collisionAt(indexes, { x, y, heading }, z), null);
  }
});

test("Alemany keeps its overpass complete in both tiles above northbound I-280", () => {
  const tokens = ["808f7e64", "808f7ef4"];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = tiles.map(({ playableRoads }) =>
    playableRoads.find(({ id }) => id === "sf-contact/alemany-i280-overpass"),
  );
  assert.deepEqual(roads[0], roads[1]);
  const { surface, portals } = buildBakedRoadContact(roads[0]);
  assert.equal(portals.length, 2);
  for (const { x, y, z } of portals)
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  const lower = groundAtWorld(tiles, 1293.207, -3176.618, true);
  const deck = surface.heightAt(1293.207, -3176.618);
  assert.ok(lower > 28.7 && lower < 29.2);
  assert.ok(deck > 36.35 && deck < 36.55);
  for (let i = 1; i < roads[0].points.length; i++) {
    const [ax, ay, az] = roads[0].points[i - 1];
    const [bx, by, bz] = roads[0].points[i];
    assert.ok(Math.abs(bz - az) / Math.hypot(bx - ax, by - ay) < 0.03);
  }
});

test("Geary's bidirectional tunnel joins the ground and stays below Masonic", () => {
  const tiles = ["808580b4", "80858734"].map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = tiles.map(({ playableRoads }) =>
    playableRoads.find(({ id }) => id === "sf-contact/geary-masonic-tunnel"),
  );
  assert.deepEqual(roads[0], roads[1]);
  assert.equal(roads[0].source_cnn, "6105000");
  const { surface, portals, appearance } = buildBakedRoadContact(roads[0]);
  assert.equal(portals.length, 2);
  for (const { x, y, z, halfWidth } of portals) {
    assert.ok(Math.abs(halfWidth - 8) < 0.002);
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  const above = groundAtWorld(tiles, -643.5, 2489.8, true);
  const bore = surface.heightAt(-643.5, 2489.8);
  assert.ok(above > 83.5 && above < 84.5);
  assert.ok(bore > 76.4 && bore < 77.5);
  assert.ok(above - bore > appearance.clearance);
  const length = appearance.interior[1] - appearance.interior[0];
  assert.ok(length > 160 && length < 162);
});

test("the Branch East exit and northbound I-280 keep their own road levels", () => {
  const tiles = ["808f7e7c", "808f7e64"].map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const matching = (tile, slug) =>
    tile.playableRoads.find(
      ({ id }) => id === `sf-contact/branch-east-${slug}`,
    );
  const bore = matching(tiles[0], "off-ramp");
  const mainline = matching(tiles[0], "i280-northbound");
  const viaduct = matching(tiles[1], "san-jose-viaduct");
  for (const [road, slug] of [
    [bore, "off-ramp"],
    [mainline, "i280-northbound"],
  ]) {
    assert.deepEqual(road, matching(tiles[1], slug));
  }
  for (const road of [bore, mainline, viaduct]) {
    const { portals } = buildBakedRoadContact(road);
    assert.equal(portals.length, 2);
    for (const { x, y, z } of portals)
      assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  const crossing = [451.516, -3185.15];
  const { surface, appearance } = buildBakedRoadContact(bore);
  const lower = surface.heightAt(...crossing);
  const upper = buildBakedRoadContact(mainline).surface.heightAt(...crossing);
  assert.ok(lower > 48 && lower < 48.25);
  assert.ok(upper > 53.8 && upper < 54.1);
  assert.ok(upper - lower > appearance.clearance);
  const farCrossing = [680.986, -2997.639];
  const farDeck = buildBakedRoadContact(viaduct).surface.heightAt(
    ...farCrossing,
  );
  assert.ok(farDeck > 56.5 && farDeck < 56.8);
  assert.ok(farDeck - groundAtWorld(tiles, ...farCrossing, true) > 9);
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

test("Toll Plaza joins the west street before rounding the bend into the lower bore", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808586ec.json.gz")));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/toll-plaza-tunnel",
  );
  const { surface, portals } = buildBakedRoadContact(road);
  const [west] = portals;
  assert.ok(Math.hypot(west.x + 3161.95, west.y - 5193.32) < 0.03);
  assert.ok(
    Math.abs(groundAtWorld([tile], west.x, west.y, true) - west.z) < 0.03,
  );
  const bearings = surface.profile
    .slice(1, 35)
    .map(({ position: [x, y] }, i) => {
      const [px, py] = surface.profile[i].position;
      return Math.atan2(y - py, x - px);
    });
  assert.ok(Math.abs(bearings[0] - 2.303) < 0.01);
  assert.ok(Math.abs(bearings.at(-1) - 0.627) < 0.01);
  for (let i = 1; i < bearings.length; i++)
    assert.ok(Math.abs(bearings[i] - bearings[i - 1]) < 0.17);
});
