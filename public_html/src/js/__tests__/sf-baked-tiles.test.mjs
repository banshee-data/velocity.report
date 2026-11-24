import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { gunzipSync } from "node:zlib";
import { Raycaster, Vector3 } from "../../vendor/three/three.module.js";
import { buildBakedRoadContact } from "../sf-baked-road-contact.js";
import {
  BAY_BRIDGE_CITY_DRIVE,
  cityDriveApproach,
} from "../sf-city-road-drive.js";
import {
  buildCollisionIndex,
  collisionAt,
  contactStep,
  groundAtWorld,
} from "../sf-drive-core.js";
import { coveredRoadBlend, driveCameraPose } from "../sf-drive-camera.js";
import {
  highwayRailSegments,
  leaveRoadSide,
  roadBoundary,
  strikeHighwayRail,
} from "../sf-highway-edge.js";
import { driveThroughRoadPortal } from "../sf-road-portal.js";
import { contactGeometry, driveOnRoadSurface } from "../sf-road-contact.js";
import { deckClearanceIndex } from "../sf-tree-clearance.js";
import { tunnelMesh } from "../sf-tunnel-mesh.js";

const path = (file) => new URL(`../../sf/${file}`, import.meta.url);
const file = (name) => readFileSync(path(name));
const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");

test("Dogpatch northbound runs below 18th Street and keeps 20th Street on its own deck", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7fb4", "808f7fbc", "808f7fcc", "808f7fd4"].map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const names = [
    "i280-dogpatch-east-north-entry",
    "i280-dogpatch-northbound",
    "i280-south-of-eighteenth-northbound",
    "i280-mariposa-northbound",
  ].map((id) => `sf-contact/${id}`);
  for (const [index, id] of names.slice(1).entries()) {
    const a = roads.get(names[index]),
      b = roads.get(id);
    assert.deepEqual(a.points.at(-1), b.points[0], id);
    assert.deepEqual(a.seams.end, b.seams.start, id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${a.id}:end` &&
          to === `${b.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const northbound = buildBakedRoadContact(roads.get(names[2])).surface;
  const eighteenth = buildBakedRoadContact(
    roads.get("sf-contact/eighteenth-west-viaduct"),
  ).surface;
  const dogpatchZ = northbound.heightAt(4184.1, 312);
  const viaductZ = eighteenth.heightAt(4184.1, 312);
  assert.ok(dogpatchZ > 20 && dogpatchZ < 22);
  assert.ok(viaductZ > 25 && viaductZ < 28);
  const twentieth = roads.get("sf-contact/dogpatch-twentieth-viaduct");
  const { surface, portals } = buildBakedRoadContact(twentieth);
  assert.ok(surface.heightAt(4226.9, 31.34) > 31);
  const freeway = buildBakedRoadContact(roads.get(names[1])).surface;
  assert.ok(surface.heightAt(4209, 30.1) - freeway.heightAt(4209, 30.1) > 6);
  assert.equal(portals.length, 2);
  assert.ok(roadBake.coveredGroundRoadPatches["808f7fb4"] >= 1);
});

test("northbound I-280 branches onto the lower Mariposa exit and hands off to street pavement", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7fb4", "808f7fcc"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const incoming = roads.get("sf-contact/i280-dogpatch-northbound");
  const ramp = roads.get("sf-contact/i280-north-mariposa-exit");
  const through = roads.get("sf-contact/i280-south-of-eighteenth-northbound");
  for (const branch of [ramp, through]) {
    assert.deepEqual(incoming.points.at(-1), branch.points[0]);
    assert.deepEqual(incoming.seams.end, branch.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${incoming.id}:end` &&
          to === `${branch.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(ramp.groundEnds, ["end"]);
  assert.deepEqual(ramp.widthTransitionMetres, { start: 24 });
  const { surface, portals } = buildBakedRoadContact(ramp);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  const floor = surface.heightAt(4201.4533, 303.8024);
  assert.ok(floor > 18 && floor < 19, floor);
});

test("the westbound Harrison loop hands off below both Bay Bridge approaches", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/8085807c.json.gz")));
  const find = (name) => tile.playableRoads.find(({ id }) => id === name);
  const incoming = find("sf-contact/i80-westbound-fremont-split");
  const exit = find("sf-contact/i80-westbound-harrison-loop");
  const through = find("sf-contact/i80-westbound-upper-soma");
  for (const road of [exit, through]) {
    assert.deepEqual(road.points[0], incoming.points.at(-1));
    assert.deepEqual(road.seams.start, incoming.seams.end);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${incoming.id}:end` &&
          to === `${road.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const { surface, portals } = buildBakedRoadContact(exit);
  assert.deepEqual(exit.groundEnds, ["end"]);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);

  const overcrossing = [4280.5062, 2843.2714];
  const heightAt = (name) =>
    buildBakedRoadContact(find(name)).surface.heightAt(...overcrossing);
  const ramp = surface.heightAt(...overcrossing);
  const lowerDeck = heightAt("sf-eastbound-lower-approach");
  const upperDeck = heightAt("city-approach");
  assert.ok(ramp > 25 && ramp < 26);
  assert.ok(lowerDeck > 33 && lowerDeck < 34);
  assert.ok(upperDeck > 42 && upperDeck < 43);
});

test("northbound I-280 keeps both city exits connected across tile owners", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7fcc", "808f7fd4"].map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const road = (tile, name) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`);
  const incoming = road(tiles[0], "i280-mariposa-northbound");
  const fifth = road(tiles[1], "i280-north-fifth-exit");
  assert.deepEqual(fifth, road(tiles[0], "i280-north-fifth-exit"));
  assert.deepEqual(fifth.owners, ["808f7fcc", "808f7fd4"]);
  assert.deepEqual(fifth.points[0], incoming.points.at(-1));
  assert.deepEqual(fifth.seams.start, incoming.seams.end);
  const exits = roadBake.playableTransitions
    .filter(
      ({ from, direction }) =>
        from === `${incoming.id}:end` && direction === "forward",
    )
    .map(({ to }) => to)
    .sort();
  assert.deepEqual(exits, [
    "sf-contact/i280-north-fifth-exit:start",
    "sf-contact/i280-sixth-northbound:start",
  ]);
  assert.deepEqual(fifth.groundEnds, ["end"]);
  const end = fifth.points.at(-1);
  const [x, y, z] = end;
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.1);
});

test("Alemany northbound stays on its own deck and clears the surveyed Dogpatch pavement", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = [
    "808f7ef4",
    "808f7efc",
    "808f7f04",
    "808f7fa4",
    "808f7fac",
    "808f7fb4",
  ];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const lookup = (name) => roads.get(`sf-contact/${name}`);
  const chain = [
    "i280-north-alemany-entry",
    "i280-north-alemany-main",
    "i280-north-alemany-climb",
    "i280-north-lower-viaduct",
    "i280-north-south-dogpatch",
    "i280-dogpatch-northbound",
  ].map(lookup);
  assert.deepEqual(chain[0].groundEnds, ["start"]);
  const [x, y, z] = chain[0].points[0];
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.02);
  for (const [index, next] of chain.slice(1).entries()) {
    const previous = chain[index];
    assert.deepEqual(previous.points.at(-1), next.points[0], next.id);
    assert.deepEqual(previous.seams.end, next.seams.start, next.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${previous.id}:end` &&
          to === `${next.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const lower = buildBakedRoadContact(chain[3]).surface;
  const upper = buildBakedRoadContact(
    lookup("i280-south-beyond-dogpatch"),
  ).surface;
  const crossing = [3379.018, -2458.532];
  assert.ok(Math.abs(lower.heightAt(...crossing) - 21.77) < 0.03);
  assert.ok(Math.abs(upper.heightAt(...crossing) - 29.58) < 0.03);

  const obstacles = tiles.map((tile, i) =>
    buildCollisionIndex(tile, tokens[i]),
  );
  assert.ok(roadBake.overheadBuildingPatches["808f7fac"] >= 3);
  assert.equal(roadBake.overheadBuildingPatches["808f7fb4"], 1);
  const road = buildBakedRoadContact(chain[4]).surface;
  for (const station of [945, 975, 1129, 1155]) {
    const i = road.profile.findIndex(({ s_m }) => s_m >= station);
    assert.ok(i > 0);
    const [x, y, z] = road.profile[i].position;
    const [a, b] = road.profile[i - 1].position;
    const heading = Math.atan2(y - b, x - a);
    assert.equal(collisionAt(obstacles, { x, y, z, heading }, z), null);
  }
});

test("Cesar Chavez exit branches from the lower viaduct and returns to ground", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7efc", "808f7f04", "808f7fa4", "808f7fac", "808f7fb4"];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const incoming = roads.get("sf-contact/i280-north-lower-viaduct");
  const ramp = roads.get("sf-contact/i280-north-cesar-chavez-exit");
  const through = roads.get("sf-contact/i280-north-south-dogpatch");
  for (const branch of [ramp, through]) {
    assert.deepEqual(incoming.points.at(-1), branch.points[0], branch.id);
    assert.deepEqual(incoming.seams.end, branch.seams.start, branch.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${incoming.id}:end` &&
          to === `${branch.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(ramp.groundEnds, ["end"]);
  const [endX, endY, endZ] = ramp.points.at(-1);
  assert.ok(Math.abs(groundAtWorld(tiles, endX, endY, true) - endZ) < 0.03);

  const indexes = tiles.map((tile, i) => buildCollisionIndex(tile, tokens[i]));
  const { surface } = buildBakedRoadContact(ramp);
  const i = surface.profile.findIndex(({ s_m }) => s_m >= 112);
  assert.ok(i > 0);
  const [x, y, z] = surface.profile[i].position;
  const [a, b] = surface.profile[i - 1].position;
  assert.equal(
    collisionAt(indexes, { x, y, z, heading: Math.atan2(y - b, x - a) }, z),
    null,
  );
  assert.equal(roadBake.overheadBuildingPatches["808f7fac"], 4);
});

test("Alemany connects to US 101 and leaves the original I-280 branch available", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7ef4", "808f7efc", "808f7e54", "808f7e4c"];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const byId = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const road = (id) => byId.get(`sf-contact/${id}`);
  const approach = road("i280-north-alemany-main");
  const connector = road("i280-to-101-north-connector");
  const silver = road("us101-north-silver-approach");
  const exit = road("us101-north-bayshore-exit");
  const through = road("us101-north-bayshore-through");
  for (const [before, after] of [
    [approach, road("i280-north-alemany-climb")],
    [approach, connector],
    [connector, silver],
    [silver, exit],
    [silver, through],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0], after.id);
    assert.deepEqual(before.seams.end, after.seams.start, after.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(exit.groundEnds, ["end"]);
  assert.deepEqual(through.groundEnds, []);
  const [endX, endY, endZ] = exit.points.at(-1);
  assert.ok(Math.abs(groundAtWorld(tiles, endX, endY, true) - endZ) < 0.03);
  const { surface } = buildBakedRoadContact(through);
  const crossing = [3199.64, -1212.11];
  assert.ok(
    surface.heightAt(...crossing) - groundAtWorld(tiles, ...crossing, true) > 7,
  );

  const obstacles = tiles.map((tile, i) =>
    buildCollisionIndex(tile, tokens[i]),
  );
  const [x, y, z] = connector.points.at(-1);
  assert.equal(collisionAt(obstacles, { x, y, z, heading: 1.67 }, z), null);
  assert.equal(roadBake.overheadBuildingPatches["808f7e54"], 1);
});

test("US 101 reaches Potrero and joins the existing eastbound I-80 deck", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7e2c", "808f7e34", "808f7e4c", "808f7e54"];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const byId = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const road = (id) => byId.get(`sf-contact/${id}`);
  const incoming = road("us101-north-bayshore-through");
  const potrero = road("us101-north-potrero-approach");
  const exit = road("us101-north-potrero-exit");
  const i80Approach = road("us101-north-i80-approach");
  const ramp = road("us101-north-i80-east-connector");
  const i80 = road("i80-eastbound-interchange");
  for (const [before, after] of [
    [incoming, potrero],
    [potrero, exit],
    [potrero, i80Approach],
    [i80Approach, ramp],
    [ramp, i80],
    [road("central-freeway-i80-east-connector"), i80],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0], after.id);
    assert.deepEqual(before.seams.end, after.seams.start, after.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(exit.groundEnds, ["end"]);
  const [x, y, z] = exit.points.at(-1);
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  const [rampX, rampY, rampZ] = ramp.points[0];
  assert.ok(rampZ - groundAtWorld(tiles, rampX, rampY, true) > 9);
});

test("US 101 viaduct contact spans both tiles and forks to Ninth Street and Octavia", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7e2c", "808f7e34"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = tiles.flatMap((tile) => tile.playableRoads);
  const road = (id) => roads.find(({ id: key }) => key === `sf-contact/${id}`);
  const viaduct = road("us101-north-central-viaduct");
  const exit = road("us101-north-ninth-bryant-exit");
  const merge = road("us101-north-central-merge");

  assert.deepEqual(
    tiles[1].playableRoads.find(({ id }) => id === viaduct.id),
    viaduct,
  );
  for (const [before, after] of [
    [road("us101-north-i80-approach"), viaduct],
    [viaduct, exit],
    [viaduct, merge],
    [merge, road("central-freeway-east-northbound")],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0], after.id);
    assert.deepEqual(before.seams.end, after.seams.start, after.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }

  const { surface } = buildBakedRoadContact(viaduct);
  const crossing = [2922.0769, 994.6148];
  assert.ok(surface.heightAt(...crossing) < 14.6);
  assert.ok(
    surface.heightAt(...crossing) - groundAtWorld(tiles, ...crossing, true) > 8,
  );
  assert.deepEqual(exit.groundEnds, ["end"]);
  const [x, y, z] = exit.points.at(-1);
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
});

test("SoMa ramps join the correct I-80 lanes and clear the surveyed Fifth Street pavement", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["8085807c", "80858084", "808f7e2c", "808f7fd4"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const source = (name) => roads.get(`sf-contact/${name}`);
  const exit = source("i80-eastbound-fourth-bryant-exit");
  const fifth = source("i80-eastbound-fifth-bryant-entry");
  const bryant = source("i80-eastbound-bryant-entry");
  const harrison = source("i80-westbound-harrison-entry");
  const fourth = source("i80-eastbound-fourth");
  for (const [before, after] of [
    [fourth, exit],
    [bryant, fourth],
    [fifth, source("i80-eastbound-lower-climb")],
    [harrison, source("i80-westbound-interchange")],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0], after.id);
    assert.deepEqual(before.seams.end, after.seams.start, after.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  for (const [ramp, end] of [
    [exit, "end"],
    [fifth, "start"],
    [bryant, "start"],
    [harrison, "start"],
  ]) {
    assert.deepEqual(ramp.groundEnds, [end]);
    const [x, y, z] = ramp.points.at(end === "start" ? 0 : -1);
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03, ramp.id);
  }

  const indexes = tiles.map((tile, i) => buildCollisionIndex(tile, tokens[i]));
  const { surface } = buildBakedRoadContact(fifth);
  const [x, y] = [3516.59, 2126.49];
  const z = surface.heightAt(x, y);
  assert.ok(z > 5.6 && z < 5.9);
  assert.equal(collisionAt(indexes, { x, y, z, heading: 0.79 }, z), null);
  assert.ok(roadBake.overheadBuildingPatches["8085807c"] >= 1);
});

test("Seventh Street and southbound US 101 meet ordinary pavement only at their exits", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7e2c", "808f7e34", "808f7e4c"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const road = (name) => roads.get(`sf-contact/${name}`);
  const seventh = road("i80-eastbound-seventh-off-ramp");
  const west = road("i80-eastbound-seventh-west-exit");
  const east = road("i80-eastbound-seventh-east-exit");
  const south = road("i80-westbound-to-us101-south");
  const main = road("us101-south-potrero-main");
  const fork = road("us101-south-potrero-exit");
  const approach = road("us101-south-cesar-potrero-approach");
  const cesar = road("us101-south-cesar-chavez-exit");
  const potrero = road("us101-south-potrero-avenue-exit");
  for (const [before, after] of [
    [road("i80-eastbound-interchange"), seventh],
    [seventh, west],
    [seventh, east],
    [road("i80-westbound-interchange"), south],
    [south, main],
    [main, fork],
    [fork, approach],
    [approach, cesar],
    [approach, potrero],
  ]) {
    assert.deepEqual(before.groundEnds, [], before.id);
    assert.deepEqual(before.points.at(-1), after.points[0], after.id);
    assert.deepEqual(before.seams.end, after.seams.start, after.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  for (const exit of [west, east, cesar, potrero]) {
    assert.deepEqual(exit.groundEnds, ["end"], exit.id);
    const [x, y, z] = exit.points.at(-1);
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03, exit.id);
  }
  for (const owner of south.owners) {
    const tile = tiles.find(({ t }) => t === owner);
    assert.deepEqual(
      tile.playableRoads.find(({ id }) => id === south.id),
      south,
    );
  }
});

test("Alemany keeps both I-280 merges connected and their city portals at street level", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7e4c", "808f7e54", "808f7e64", "808f7ef4", "808f7efc"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const road = (name) => roads.get(`sf-contact/${name}`);
  const dogpatch = road("i280-south-beyond-dogpatch");
  const tail = road("i280-south-alemany-merge-tail");
  const approach = road("us101-south-to-i280-south-approach");
  const merge = road("us101-south-to-i280-south-merge");
  const through = road("i280-south-alemany-through");
  const main = road("i280-south-alemany-main");
  const entry = road("i280-south-alemany-entry");
  const beyond = road("i280-south-beyond-alemany");
  const eastExit = road("us101-south-alemany-east-exit");
  const westExit = road("i280-south-alemany-exit");
  for (const [before, after] of [
    [dogpatch, tail],
    [approach, merge],
    [approach, eastExit],
    [merge, through],
    [tail, through],
    [through, main],
    [through, westExit],
    [main, beyond],
    [entry, beyond],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0], after.id);
    assert.deepEqual(before.seams.end, after.seams.start, after.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(dogpatch.groundEnds, []);
  assert.deepEqual(entry.widthTransitionMetres, { end: 24 });
  for (const [ramp, end] of [
    [eastExit, "end"],
    [westExit, "end"],
    [entry, "start"],
  ]) {
    assert.deepEqual(ramp.groundEnds, [end]);
    const [x, y, z] = ramp.points.at(end === "start" ? 0 : -1);
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03, ramp.id);
  }
});

test("a narrow ramp tapers into the shared cross-section of a wider deck", () => {
  const seam = { bearing: 0, width: 12 };
  const ramp = {
    id: "narrow-ramp",
    kind: "ramp",
    width: 8,
    owners: ["8085807c", "8085807c"],
    points: Array.from({ length: 21 }, (_, i) => [i * 2, 0, 10]),
    seams: { end: seam },
    widthTransitionMetres: { end: 24 },
  };
  const deck = {
    ...ramp,
    id: "wide-deck",
    kind: "bridge_deck",
    width: 12,
    points: ramp.points.map(([x, y, z]) => [x + 40, y, z]),
    seams: { start: seam },
    widthTransitionMetres: {},
  };
  const { surface } = buildBakedRoadContact(ramp);
  const vertices = surface.contact.vertices;
  assert.equal(vertices[16][1], 4);
  assert.equal(vertices[28][1], 5);
  assert.equal(vertices.at(-2)[1], 6);
  assert.deepEqual(
    vertices.slice(-2),
    buildBakedRoadContact(deck).surface.contact.vertices.slice(0, 2),
  );
  assert.equal(
    buildBakedRoadContact({ ...ramp, widthTransitionMetres: {} }),
    null,
  );
  assert.equal(
    buildBakedRoadContact({ ...ramp, widthTransitionMetres: { start: 24 } }),
    null,
  );
  assert.equal(
    buildBakedRoadContact({ ...ramp, widthTransitionMetres: { end: 41 } }),
    null,
  );
});

test("tiled right-only guard rail bounces on the right and leaves the left edge open", () => {
  const road = {
    id: "test-highway-exit",
    kind: "ramp",
    owners: ["8085807c", "8085807c"],
    width: 6,
    points: Array.from({ length: 31 }, (_, x) => [x, 0, 10]),
    railSides: ["right"],
  };
  const { surface } = buildBakedRoadContact(road);
  surface.boundary = roadBoundary(surface.contact);
  surface.railSegments = highwayRailSegments(surface);
  assert.ok(surface.railSegments.length);
  assert.ok(surface.railSegments.every(({ a, b }) => a[1] < 0 && b[1] < 0));
  const left = { x: 15, y: 0.39, z: 10, v: 7, heading: Math.PI / 2 };
  const right = { ...left, y: -left.y, heading: -left.heading };
  assert.equal(strikeHighwayRail(left, {}, 1 / 120, surface), null);
  assert.equal(
    strikeHighwayRail(right, {}, 1 / 120, surface).edgeContact,
    "bounce",
  );
  assert.equal(
    leaveRoadSide({ ...left, y: 2.99, grounded: true }, {}, 1 / 120, surface)
      .leavingRoad,
    true,
  );
  assert.equal(buildBakedRoadContact({ ...road, railSides: ["center"] }), null);
});

test("the upper West Span shares its Fremont fork with the westbound freeway continuation", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = [
    "80858014",
    "80858024",
    "8085803c",
    "8085806c",
    "80858074",
    "8085807c",
    "80858084",
  ];
  const roads = new Map(
    tokens.flatMap((token) => {
      const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
      return tile.playableRoads.map((road) => [road.id, road]);
    }),
  );
  const upper = roads.get("yerba-buena-upper-tunnel");
  const lower = roads.get("yerba-buena-lower-tunnel");
  assert.ok(buildBakedRoadContact(upper).appearance);
  assert.ok(buildBakedRoadContact(lower).appearance);
  assert.ok(upper.points[0][2] > lower.points[0][2] + 10);
  for (const ids of [
    ["city-approach", "fremont-exit"],
    [
      "city-approach",
      "sf-contact/i80-westbound-fremont-split",
      "sf-contact/i80-westbound-upper-soma",
      "sf-contact/i80-westbound-fourth-merge",
      "westbound-i80",
    ],
  ]) {
    for (const [index, next] of ids.slice(1).entries()) {
      const before = ids[index];
      const a = roads.get(before);
      const b = roads.get(next);
      assert.deepEqual(a.points.at(-1), b.points[0], next);
      assert.deepEqual(a.seams.end, b.seams.start, next);
      assert.ok(
        roadBake.playableTransitions.some(
          ({ from, to, direction }) =>
            from === `${before}:end` &&
            to === `${next}:start` &&
            direction === "forward",
        ),
      );
    }
  }
  const exit = roads.get("fremont-exit");
  assert.deepEqual(exit.groundEnds, ["end"]);
  assert.deepEqual(exit.railSides, ["right"]);
});

test("the east island ramp joins the upper bore without changing the through tunnel level", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["80858024", "8085803c"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const ramp = roads.get("sf-contact/yerba-buena-east-westbound-on-ramp");
  const entranceBore = roads.get(
    "sf-contact/yerba-buena-east-westbound-entrance-tunnel",
  );
  const mainBore = roads.get("yerba-buena-upper-tunnel");
  const sharedBore = roads.get("yerba-buena-upper-tunnel-west-of-merge");
  const west = roads.get("yerba-buena-west-approach");
  assert.deepEqual(ramp.groundEnds, ["start"]);
  assert.equal(buildBakedRoadContact(ramp).appearance, null);
  assert.ok(buildBakedRoadContact(entranceBore).appearance);
  assert.ok(buildBakedRoadContact(sharedBore).appearance);
  for (const [before, after] of [
    [ramp, entranceBore],
    [entranceBore, sharedBore],
    [mainBore, sharedBore],
    [sharedBore, west],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const shells = [mainBore, entranceBore, sharedBore].map((road) => {
    const { surface, appearance } = buildBakedRoadContact(road);
    return tunnelMesh(surface, appearance);
  });
  const originalMainBore = buildBakedRoadContact(mainBore);
  const uncut = tunnelMesh(originalMainBore.surface, {
    ...originalMainBore.appearance,
    wallOpenings: {},
  });
  const eye = ([x, y, z]) => new Vector3(x, z + 1.0, -y);
  let uncutHits = 0;
  for (const { points } of [entranceBore, mainBore]) {
    for (let i = 1; i < points.length; i++) {
      const start = eye(points[i - 1]);
      const delta = eye(points[i]).sub(start);
      const end = delta.length();
      const ray = new Raycaster(start, delta.normalize(), 0, end);
      assert.equal(
        ray.intersectObjects(shells).length,
        0,
        `wall across tunnel at ${points[i]}`,
      );
      uncutHits += ray.intersectObject(uncut).length;
    }
  }
  assert.ok(uncutHits > 0, "the earlier wall would cross the entrance lane");
  const viewpoint = eye(mainBore.points[5]).lerp(eye(mainBore.points[6]), 0.5);
  const vertices = originalMainBore.surface.contact.vertices;
  const outerWall = eye(vertices[10]).lerp(eye(vertices[12]), 0.5);
  const sideways = outerWall.sub(viewpoint).normalize();
  const wall = new Raycaster(viewpoint, sideways, 0, 12);
  assert.ok(
    wall.intersectObject(shells[0]).length,
    "outside the merge, the outer wall remains",
  );
  const product = JSON.parse(file("gamedrives.json"));
  const route = cityDriveApproach(product.drives[0], "yerba-buena-westbound");
  for (const road of [mainBore, sharedBore]) {
    const guide = route.sections.find(({ id }) => id === road.id);
    assert.deepEqual(guide.positions, road.points);
    assert.deepEqual(guide.tunnel_wall_openings_m, road.tunnel_wall_openings_m);
  }
});

test("the lower island exit stays below the upper bore and meets Hillcrest pavement", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/80858024.json.gz")));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const roads = new Map(tile.playableRoads.map((road) => [road.id, road]));
  const entry = roads.get("yerba-buena-lower-tunnel");
  const through = roads.get("yerba-buena-lower-tunnel-east-of-exit");
  const exit = roads.get("sf-contact/yerba-buena-east-eastbound-exit-tunnel");
  const ramp = roads.get("sf-contact/yerba-buena-east-eastbound-off-ramp");
  for (const [before, after] of [
    [entry, through],
    [entry, exit],
    [exit, ramp],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.ok(exit.points[0][2] < 49);
  const [x, y, z] = ramp.points.at(-1);
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.005);
  const shells = [through, exit].map((road) => {
    const { surface, appearance } = buildBakedRoadContact(road);
    return tunnelMesh(surface, appearance);
  });
  for (const { points } of [through, exit]) {
    for (let i = 1; i < points.length; i++) {
      const eye = ([x, y, z]) => new Vector3(x, z + 1, -y);
      const start = eye(points[i - 1]);
      const delta = eye(points[i]).sub(start);
      const length = delta.length();
      const ray = new Raycaster(start, delta.normalize(), 0, length);
      assert.equal(
        ray.intersectObjects(shells).length,
        0,
        `wall across lower tunnel at ${points[i]}`,
      );
    }
  }
});

test("Hillcrest joins the lower east bridge deck through the same seam as the mainline", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/80858024.json.gz")));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const roads = new Map(tile.playableRoads.map((road) => [road.id, road]));
  const entry = roads.get("sf-contact/yerba-buena-east-eastbound-on-ramp");
  const mainline = roads.get("yerba-buena-lower-east-approach");
  const deck = roads.get("yerba-buena-lower-east-after-entry");
  const upper = buildBakedRoadContact(
    roads.get("yerba-buena-east-approach"),
  ).surface;
  for (const road of [entry, mainline]) {
    assert.deepEqual(road.points.at(-1), deck.points[0]);
    assert.deepEqual(road.seams.end, deck.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${road.id}:end` &&
          edge.to === `${deck.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  const { surface, portals } = buildBakedRoadContact(entry);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.005);
  const [start, end] = surface.coveredRange;
  const under = surface.profile.filter(
    ({ s_m }) => s_m > start + 5 && s_m < end - 5,
  );
  assert.ok(under.length > 12);
  for (const {
    position: [x, y, z],
  } of under) {
    assert.ok(upper.heightAt(x, y) - z > 7);
  }
  for (const [i, { position, s_m }] of surface.profile.slice(1).entries()) {
    const previous = surface.profile[i];
    assert.ok(
      Math.abs(position[2] - previous.position[2]) / (s_m - previous.s_m) <
        0.06,
    );
  }
});

test("23rd Street crosses both US 101 carriageways at its own level", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e4c.json.gz")));
  const other = JSON.parse(gunzipSync(file("tiles/808f7e34.json.gz")));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/potrero-23rd-us101-overpass",
  );
  const { surface } = buildBakedRoadContact(road);
  assert.deepEqual(road.groundEnds, ["start", "end"]);
  for (const [x, y, z] of [road.points[0], road.points.at(-1)]) {
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.005);
  }
  for (const id of [
    "sf-contact/us101-north-potrero-approach",
    "sf-contact/us101-south-potrero-main",
  ]) {
    const lower = tile.playableRoads.find((entry) => entry.id === id);
    assert.deepEqual(
      lower,
      other.playableRoads.find((entry) => entry.id === id),
    );
    const [x, y, z] = lower.points.reduce((best, point) =>
      Math.abs(point[1] + 611) < Math.abs(best[1] + 611) ? point : best,
    );
    const level = surface.heightAt(x, y);
    assert.ok(level - z > 6.5 && level - z < 8, id);
  }
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  assert.ok(roadBake.coveredGroundRoadPatches[tile.t]);
});

test("Evans Avenue retains its measured grade across the rebaked terrain strip", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7fac.json.gz")));
  const point = [3797.04, -1228.795];
  const end = [3760.65, -1133.32];
  const length = Math.hypot(end[0] - point[0], end[1] - point[1]);
  for (const offset of [-6, 0, 6]) {
    const heights = Array.from({ length: 102 }, (_, station) => {
      const x =
        point[0] +
        (station * (end[0] - point[0]) + offset * (end[1] - point[1])) / length;
      const y =
        point[1] +
        (station * (end[1] - point[1]) - offset * (end[0] - point[0])) / length;
      return groundAtWorld([tile], x, y, true);
    });
    assert.ok(heights.every((z) => z !== null));
    assert.ok(Math.min(...heights.slice(44, 65)) > 13.5);
    assert.ok(
      heights.slice(1).every((z, i) => Math.abs(z - heights[i]) < 0.12),
    );
  }
});

test("Central Freeway joins the lower Bay Bridge through contiguous eastbound contact", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = [
    "808f7e2c",
    "80858084",
    "808f7fd4",
    "8085807c",
    "80858074",
  ].map((token) => JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))));
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const chain = [
    "sf-contact/i80-eastbound-interchange",
    "sf-contact/i80-eastbound-midtown",
    "sf-contact/i80-eastbound-fourth",
    "sf-contact/i80-eastbound-soma",
    "sf-contact/i80-eastbound-lower-climb",
    "sf-contact/i80-eastbound-essex-merge",
    "sf-eastbound-lower-approach",
  ].map((id) => roads.get(id));
  for (const [index, road] of chain.slice(1).entries()) {
    const previous = chain[index];
    assert.ok(buildBakedRoadContact(road), road.id);
    assert.deepEqual(road.groundEnds, [], road.id);
    assert.deepEqual(previous.points.at(-1), road.points[0], road.id);
    assert.deepEqual(previous.seams.end, road.seams.start, road.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${previous.id}:end` &&
          to === `${road.id}:start` &&
          direction === "forward",
      ),
      road.id,
    );
  }
  const merge = chain.at(-2);
  const upper = tiles
    .flatMap(({ raisedRoads }) => raisedRoads ?? [])
    .find(({ id }) => id.startsWith("overpass/19365000/"));
  const levelGap = upper.points.at(-1)[2] - merge.points.at(-1)[2];
  assert.ok(levelGap > 8 && levelGap < 10);
  assert.ok(
    upper.profiles.every(
      (profile) => !roadBake.playableReplaces.includes(profile),
    ),
  );
});

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
  assert.deepEqual(selected.sections.slice(8), drive.sections);
  const [
    outerSpan,
    span,
    through,
    approach,
    merged,
    firstBore,
    sharedBore,
    exit,
    bridge,
  ] = selected.sections;
  assert.equal(outerSpan.id, "sf-contact/bay-bridge-east-span-westbound");
  assert.equal(span.id, "sf-contact/yerba-buena-upper-east-span");
  assert.equal(through.id, "sf-contact/yerba-buena-upper-east-through");
  const tunnels = [firstBore, sharedBore];
  for (const tunnel of tunnels) {
    assert.equal(tunnel.kind, "tunnel");
    assert.equal(tunnel.game_clearance_m, 4.4);
    assert.equal(tunnel.width_m, 16);
  }
  for (const [before, after] of [
    [outerSpan, span],
    [span, through],
    [through, approach],
    [approach, merged],
    [merged, firstBore],
    [firstBore, sharedBore],
    [sharedBore, exit],
    [exit, bridge],
  ]) {
    assert.deepEqual(before.positions.at(-1), after.positions[0]);
  }
  let length = 0;
  for (const { positions } of tunnels) {
    for (let i = 1; i < positions.length; i++) {
      const [ax, ay, az] = positions[i - 1];
      const [bx, by, bz] = positions[i];
      const ds = Math.hypot(bx - ax, by - ay);
      assert.ok(Math.abs(bz - az) / ds < 0.045);
      length += ds;
    }
  }
  assert.ok(length > 164 && length < 166);
});

test("both East Span carriageways reach the map edge on separate surveyed contacts", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/80858024.json.gz")));
  const roads = new Map(tile.playableRoads.map((road) => [road.id, road]));
  const east = roads.get("sf-contact/bay-bridge-east-span-eastbound");
  const west = roads.get("sf-contact/bay-bridge-east-span-westbound");
  const lower = roads.get("yerba-buena-lower-east-after-entry");
  const upper = roads.get("sf-contact/yerba-buena-upper-east-span");

  for (const [before, after] of [
    [lower, east],
    [west, upper],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }

  assert.ok(lower.points.at(-1)[2] > 53.6 && lower.points.at(-1)[2] < 54);
  assert.ok(east.points.at(-1)[0] >= 7630);
  assert.ok(west.points[0][0] >= 7630);
  assert.ok(west.points[0][1] - east.points.at(-1)[1] > 40);
  for (const [road, opposing] of [
    [east, west],
    [west, east],
  ]) {
    assert.equal(road.width, 20);
    assert.deepEqual(road.groundEnds, []);
    assert.ok(
      road.profiles.every((id) => roadBake.playableReplaces.includes(id)),
    );
    const { surface } = buildBakedRoadContact(road);
    assert.ok(surface.profile.at(-1).s_m > 780);
    const [x, y, z] = road.points[Math.floor(road.points.length / 2)];
    const [a, b] = road.points[Math.floor(road.points.length / 2) - 1];
    const heading = Math.atan2(y - b, x - a);
    for (const offset of [-8, 0, 8]) {
      const height = surface.heightAt(
        x - offset * Math.sin(heading),
        y + offset * Math.cos(heading),
      );
      assert.ok(Math.abs(height - z) < 0.1);
    }
    const [oppositeX, oppositeY] =
      opposing.points[Math.floor(opposing.points.length / 2)];
    assert.equal(surface.heightAt(oppositeX, oppositeY), null);
    for (let i = 1; i < road.points.length; i++) {
      const [ax, ay, az] = road.points[i - 1];
      const [bx, by, bz] = road.points[i];
      assert.ok(Math.abs(bz - az) / Math.hypot(bx - ax, by - ay) < 0.025);
    }
  }
});

test("the upper East Span forks to Macalla Road without crossing the lower deck", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/80858024.json.gz")));
  const roads = new Map(tile.playableRoads.map((road) => [road.id, road]));
  const road = (suffix) => roads.get(`sf-contact/${suffix}`);
  const stem = road("yerba-buena-upper-east-span");
  const through = road("yerba-buena-upper-east-through");
  const ramp = road("yerba-buena-east-westbound-off-ramp");
  const upper = roads.get("yerba-buena-east-approach");
  for (const [before, after] of [
    [stem, through],
    [stem, ramp],
    [through, upper],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const { surface: approach } = buildBakedRoadContact(stem);
  assert.ok(approach.profile.at(-1).s_m >= 140);
  const index = buildCollisionIndex(tile, tile.t);
  for (const [
    i,
    {
      position: [x, y, z],
    },
  ] of approach.profile.slice(1).entries()) {
    const [priorX, priorY] = approach.profile[i].position;
    const heading = Math.atan2(y - priorY, x - priorX);
    for (const lateral of [-2.2, 0, 2.2]) {
      const car = {
        x: x - lateral * Math.sin(heading),
        y: y + lateral * Math.cos(heading),
        heading,
      };
      assert.equal(
        collisionAt([index], car, z),
        null,
        `East Span at ${x}, ${y}`,
      );
    }
  }
  const overhead = { x: 6937.2, y: 5993.1, heading: -2.4 };
  assert.equal(collisionAt([index], overhead, 65), "building");
  const { surface: exit, portals } = buildBakedRoadContact(ramp);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.005);
  assert.ok(exit.profile[0].position[2] > 60);
  assert.ok(exit.profile.at(-1).position[2] > 54);
  assert.ok(exit.profile.at(-1).s_m > 310);
});

test("the eastbound bridge start connects First Street to the lower bore and east approach", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const drive = JSON.parse(file("gamedrives.json")).drives.find(
    ({ id }) => id === BAY_BRIDGE_CITY_DRIVE,
  );
  const entry = roadBake.driveApproaches.find(
    ({ id }) => id === "yerba-buena-eastbound",
  );
  const selected = cityDriveApproach(drive, entry.id);
  assert.equal(selected.label, entry.label);
  assert.equal(selected.sections[0].id, "first-street-eastbound-entry");
  assert.equal(drive.sections[0].id, "bay-bridge-westbound");
  const tunnels = selected.sections.filter(({ kind }) => kind === "tunnel");
  assert.deepEqual(
    tunnels.map(({ id }) => id),
    ["yerba-buena-lower-tunnel", "yerba-buena-lower-tunnel-east-of-exit"],
  );
  let length = 0;
  for (const tunnel of tunnels) {
    assert.equal(tunnel.game_clearance_m, 4.9);
    for (let i = 1; i < tunnel.positions.length; i++) {
      const [ax, ay, az] = tunnel.positions[i - 1];
      const [bx, by, bz] = tunnel.positions[i];
      const ds = Math.hypot(bx - ax, by - ay);
      assert.ok(Math.abs(bz - az) / ds < 0.05);
      length += ds;
    }
  }
  assert.ok(length > 286 && length < 288);
  assert.ok(
    tunnels[0].positions[0][2] < 44 && tunnels.at(-1).positions.at(-1)[2] > 51,
  );
  for (let i = 1; i < selected.sections.length; i++) {
    assert.deepEqual(
      selected.sections[i - 1].positions.at(-1),
      selected.sections[i].positions[0],
    );
  }
  assert.equal(
    selected.sections.at(-1).id,
    "sf-contact/bay-bridge-east-span-eastbound",
  );
});

test("Essex and First Street enter the same lower deck from ordinary city pavement", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const drive = JSON.parse(file("gamedrives.json")).drives.find(
    ({ id }) => id === BAY_BRIDGE_CITY_DRIVE,
  );
  const essex = cityDriveApproach(drive, "essex-eastbound");
  const first = cityDriveApproach(drive, "yerba-buena-eastbound");
  assert.equal(essex.sections[0].id, "essex-street-eastbound-entry");
  assert.deepEqual(essex.sections.slice(1), first.sections.slice(1));
  const city = JSON.parse(gunzipSync(file("tiles/8085807c.json.gz")));
  const deck = city.playableRoads.find(
    ({ id }) => id === "sf-eastbound-lower-approach",
  );
  for (const route of [essex, first]) {
    const ramp = city.playableRoads.find(
      ({ id }) => id === route.sections[0].id,
    );
    assert.deepEqual(ramp.points.at(-1), deck.points[0]);
    assert.deepEqual(ramp.seams.end, deck.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${ramp.id}:end` && edge.to === `${deck.id}:start`,
      ),
    );
    const { portals } = buildBakedRoadContact(ramp);
    assert.equal(portals.length, 1);
    const [{ x, y, z }] = portals;
    assert.ok(Math.abs(groundAtWorld([city], x, y, true) - z) < 0.15);
    if (route === first) {
      const [startX, startY] = first.sections[0].positions[0];
      assert.ok(Math.hypot(x - startX, y - startY) > 7);
    }
  }
});

test("ordinary city tiles carry the complete lower West Span and Yerba Buena bore", () => {
  const readRoad = (token, id) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))).playableRoads.find(
      (road) => road.id === id,
    );
  const westId = "bay-bridge-eastbound-lower";
  const west = readRoad("80858074", westId);
  for (const owner of ["80858014", "8085803c", "8085806c"])
    assert.deepEqual(readRoad(owner, westId), west);
  const boreId = "yerba-buena-lower-tunnel";
  const bore = readRoad("80858024", boreId);
  assert.deepEqual(readRoad("8085803c", boreId), bore);
  const { appearance, portals } = buildBakedRoadContact(bore);
  assert.equal(appearance.clearance, 4.9);
  const continuation = readRoad(
    "80858024",
    "yerba-buena-lower-tunnel-east-of-exit",
  );
  assert.deepEqual(bore.points.at(-1), continuation.points[0]);
  assert.ok(
    appearance.interior[1] +
      buildBakedRoadContact(continuation).appearance.interior[1] >
      286,
  );
  assert.deepEqual(portals, []);
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

test("MacArthur keeps one complete bore across tiles and only paints the open approaches", () => {
  const readTile = (token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const owners = ["80858724", "808586e4"];
  const tiles = owners.map(readTile);
  const roads = tiles.map(({ playableRoads }) =>
    playableRoads.find(({ id }) => id === "sf-contact/macarthur-tunnel"),
  );
  assert.deepEqual(roads[0], roads[1]);
  assert.deepEqual(roads[0].owners, owners);
  const { surface, portals, appearance } = buildBakedRoadContact(roads[0]);
  assert.equal(portals.length, 2);
  for (const { x, y, z, halfWidth } of portals) {
    assert.ok(Math.abs(halfWidth - 6.7) < 0.002);
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  const length = appearance.interior[1] - appearance.interior[0];
  assert.ok(length > 394 && length < 397);
  const centre = [-2602.5, 3800];
  const pavement = surface.heightAt(...centre);
  assert.ok(pavement > 69 && pavement < 74.3);
  assert.ok(groundAtWorld(tiles, ...centre, true) > pavement + 13);

  const source = readTile("8085871c");
  for (const direction of ["Northbound", "Southbound"]) {
    const rows = source.r.filter(
      ([name]) => source.n[name] === `Hwy 1 ${direction}`,
    );
    assert.equal(rows.length, 2);
    for (const row of rows) assert.equal(row[3].length, row[4].length);
    const northing = rows.map((row) =>
      row[3].filter((_, i) => i % 2).map((v) => source.o[1] + v / 10),
    );
    assert.ok(northing.some((stations) => Math.max(...stations) === 3617));
    assert.ok(northing.some((stations) => Math.min(...stations) === 4012));
  }
});

test("Kobbe Avenue remains below both SR 1 decks, which join the highway at each end", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808586e4.json.gz")));
  for (const [direction, x] of [
    ["northbound", -2594.4],
    ["southbound", -2603.3],
  ]) {
    const road = tile.playableRoads.find(
      ({ id }) => id === `sf-contact/kobbe-viaduct-${direction}`,
    );
    const { surface, portals } = buildBakedRoadContact(road);
    assert.equal(portals.length, 2);
    for (const { x, y, z } of portals) {
      assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
    }
    const street = groundAtWorld([tile], x, 4200, true);
    const deck = surface.heightAt(x, 4200);
    assert.ok(street > 55 && street < 59);
    assert.ok(deck > 65 && deck < 66);
    assert.ok(deck - street > 6);
    for (let i = 1; i < road.points.length; i++) {
      const [ax, ay, az] = road.points[i - 1];
      const [bx, by, bz] = road.points[i];
      assert.ok(Math.abs(bz - az) / Math.hypot(bx - ax, by - ay) < 0.08);
    }
  }
});

test("Ruckman keeps both SR 1 decks above the street and branches at the northbound fork", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808586dc.json.gz")));
  const road = (slug) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const north = road("ruckman-viaduct-northbound");
  const goldenGate = road("sr1-north-to-golden-gate-approach");
  const city = road("sr1-north-to-presidio-southbound");
  const doyle = road("presidio-sb-entry");
  for (const [from, to] of [
    [north, goldenGate],
    [north, city],
    [city, doyle],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: start, to: end, direction }) =>
          start === `${from.id}:end` &&
          end === `${to.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(north.groundEnds, ["start"]);
  assert.deepEqual(goldenGate.groundEnds, ["end"]);
  for (const [direction, x] of [
    ["northbound", -2589],
    ["southbound", -2596.4],
  ]) {
    const decks =
      direction === "northbound"
        ? [north, goldenGate]
        : [road("ruckman-viaduct-southbound")];
    const contact = decks.map(buildBakedRoadContact);
    const portals = contact.flatMap((deck) => deck.portals);
    assert.equal(portals.length, 2);
    for (const { x, y, z } of portals) {
      assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
    }
    const street = groundAtWorld([tile], x, 4547, true);
    const { surface } = contact.at(-1);
    const deck = surface.heightAt(x, 4547);
    assert.ok(street > 35 && street < 39);
    assert.ok(deck > 44 && deck < 45);
    assert.ok(deck - street > 5);
  }
});

test("Sixth Street reaches both I-280 levels through continuous paved approaches", () => {
  const tiles = ["808f7fcc", "808f7fd4"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const road = (slug, tile = tiles[0]) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const south = road("i280-sixth-southbound");
  const north = road("i280-sixth-northbound");
  const east = road("eighteenth-east-viaduct");
  const west = road("eighteenth-west-viaduct");
  const ramp = road("i280-eighteenth-north-entry");
  const mariposa = road("i280-mariposa-northbound");
  assert.deepEqual(south, road("i280-sixth-southbound", tiles[1]));
  assert.deepEqual(north, road("i280-sixth-northbound", tiles[1]));
  assert.equal(roadBake.coveredGroundRoadPatches["808f7fcc"], 3);
  for (const [from, to] of [
    [south, road("i280-king-to-mariposa")],
    [east, west],
    [east, ramp],
    [ramp, mariposa],
    [mariposa, north],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` && edge.to === `${to.id}:start`,
      ),
    );
  }
  for (const deck of [north, south, east, west]) {
    const { portals } = buildBakedRoadContact(deck);
    assert.equal(portals.length, 1);
    const [{ x, y, z }] = portals;
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  const indiana = [4259.48, 320.06];
  assert.ok(groundAtWorld(tiles, ...indiana, true) < 12);
  assert.ok(buildBakedRoadContact(east).surface.heightAt(...indiana) > 21);
});

test("Central Freeway connects Fourth Street, 13th, Octavia and South Van Ness", () => {
  const tokens = [
    "8085807c",
    "80858084",
    "8085809c",
    "808580a4",
    "808f7e24",
    "808f7e2c",
  ];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) =>
      (tile.playableRoads ?? []).map((road) => [road.id, road]),
    ),
  );
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const west = "westbound-i80";
  const north = "sf-contact/central-freeway-east-northbound";
  const south = "sf-contact/central-freeway-east-southbound";
  for (const [fromId, toId] of [
    ["fourth-street-entrance", west],
    [west, "sf-contact/i80-westbound-fifth"],
    [west, "sf-contact/i80-westbound-fifth-exit"],
    [north, "sf-contact/central-freeway-octavia-exit"],
    [north, "sf-contact/central-freeway-13th-exit"],
    ["sf-contact/central-freeway-octavia-entry", south],
    ["sf-contact/central-freeway-van-ness-merge", south],
    [south, "sf-contact/central-freeway-i80-east-connector"],
  ]) {
    const from = roads.get(fromId);
    const to = roads.get(toId);
    assert.ok(buildBakedRoadContact(from));
    assert.ok(buildBakedRoadContact(to));
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) => edge.from === `${fromId}:end` && edge.to === `${toId}:start`,
      ),
    );
  }
  const exit = roads.get("sf-contact/central-freeway-octavia-exit");
  const [x, y, z] = exit.points[Math.floor(exit.points.length / 2)];
  assert.ok(z - groundAtWorld(tiles, x, y, true) > 4);
  assert.equal(roadBake.overheadBuildingPatches["8085809c"], 1);
  assert.ok(!tiles[2].playableRoads?.some(({ id }) => id === exit.id));
});

test("southbound US 101 keeps the Silver fork, Industrial merge and Silliman street handoffs", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7e54", "808f7efc", "808f7ee4", "808f7f24"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) => playableRoads.map((r) => [r.id, r])),
  );
  const road = (id) => roads.get(`sf-contact/us101-south-${id}`);
  const assertJoin = (source, destination) => {
    const from = road(source);
    const to = road(destination);
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` &&
          edge.to === `${to.id}:start` &&
          edge.direction === "forward",
      ),
    );
  };
  for (const [from, to] of [
    ["silver-through", "to-i280-south-approach"],
    ["silver-through", "silver-to-industrial"],
    ["silver-to-industrial", "industrial-to-silliman"],
    ["industrial-entry", "industrial-to-silliman"],
    ["industrial-to-silliman", "silliman-exit"],
    ["industrial-to-silliman", "silliman-mainline"],
    ["silliman-mainline", "silliman-to-paul"],
    ["silliman-entry", "silliman-to-paul"],
    ["silliman-to-paul", "paul-approach"],
    ["paul-approach", "san-bruno-exit"],
    ["paul-approach", "paul-mainline"],
  ]) {
    assertJoin(from, to);
  }
  const exit = road("silliman-exit");
  const entry = road("silliman-entry");
  assert.deepEqual(exit.points.at(-1), entry.points[0]);
  assert.ok(
    !roadBake.playableTransitions.some(
      (edge) => edge.from === `${exit.id}:end`,
    ),
  );
  for (const [id, groundEnd] of [
    ["industrial-entry", "start"],
    ["silliman-entry", "start"],
    ["silliman-exit", "end"],
    ["san-bruno-exit", "end"],
  ]) {
    const surface = road(id);
    assert.deepEqual(surface.groundEnds, [groundEnd]);
    const { portals } = buildBakedRoadContact(surface);
    assert.equal(portals.length, 1);
    const [{ x, y, z }] = portals;
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03, id);
  }
  const acrossTiles = road("silliman-to-paul");
  assert.deepEqual(acrossTiles.owners, ["808f7efc", "808f7ee4"]);
  for (const token of acrossTiles.owners) {
    const tile = tiles.find(({ t }) => t === token);
    assert.deepEqual(
      tile.playableRoads.find(({ id }) => id === acrossTiles.id),
      acrossTiles,
    );
  }
});

test("southbound US 101 stays on its level through the Hester fork and collector merge", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7ee4", "808f7f24", "808f7f2c"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) => playableRoads.map((r) => [r.id, r])),
  );
  const road = (name) => roads.get(`sf-contact/us101-south-${name}`);
  const paul = road("paul-mainline");
  const main = road("hester-mainline");
  const entry = road("hester-collector-entry");
  const exit = road("hester-bayshore-exit");
  const collector = road("hester-collector-return");
  const edge = road("hester-to-city-edge");
  for (const [from, to] of [
    [paul, main],
    [paul, entry],
    [entry, exit],
    [entry, collector],
    [main, edge],
    [collector, edge],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: a, to: b, direction }) =>
          a === `${from.id}:end` &&
          b === `${to.id}:start` &&
          direction === "forward",
      ),
    );
  }
  for (const ramp of [paul, main, entry, collector]) {
    assert.deepEqual(ramp.groundEnds, []);
    assert.equal(buildBakedRoadContact(ramp).portals.length, 0);
  }
  for (const ramp of [exit, edge]) {
    const [{ x, y, z }] = buildBakedRoadContact(ramp).portals;
    assert.deepEqual(ramp.groundEnds, ["end"]);
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  assert.deepEqual(edge.owners, ["808f7f24", "808f7f2c"]);
  assert.deepEqual(
    tiles[1].playableRoads.find(({ id }) => id === edge.id),
    tiles[2].playableRoads.find(({ id }) => id === edge.id),
  );
  assert.equal(roadBake.overheadBuildingPatches["808f7f24"], 4);
});

test("Potrero and Cesar Chavez entrances merge above the street on southbound US 101", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7e4c", "808f7e54"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) => playableRoads.map((r) => [r.id, r])),
  );
  const road = (id) => roads.get(`sf-contact/us101-south-${id}`);
  const potrero = road("potrero-avenue-approach");
  const potreroRamp = road("potrero-entry");
  const chavez = road("cesar-chavez-entry");
  const merge = road("city-entrance-merge");
  const highway = road("silver-through");
  for (const [from, to] of [
    [potrero, potreroRamp],
    [potreroRamp, merge],
    [chavez, merge],
    [merge, highway],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` &&
          edge.to === `${to.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  for (const entrance of [potrero, chavez]) {
    assert.deepEqual(entrance.groundEnds, ["start"]);
    const { portals } = buildBakedRoadContact(entrance);
    assert.equal(portals.length, 1);
    const [{ x, y, z }] = portals;
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  assert.deepEqual(potreroRamp.groundEnds, []);
  assert.deepEqual(merge.groundEnds, []);
  const [x, y, z] = merge.points[0];
  assert.ok(z - groundAtWorld(tiles, x, y, true) > 5);
  for (const token of new Set(merge.owners)) {
    const owner = tiles.find(({ t }) => t === token);
    assert.deepEqual(
      owner.playableRoads.find(({ id }) => id === merge.id),
      merge,
    );
  }
});

test("the Bayshore exit passes below both freeway directions and keeps its city handoff", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e4c.json.gz")));
  const road = (id) =>
    tile.playableRoads.find((r) => r.id === `sf-contact/${id}`);
  const upstream = road("us101-south-potrero-exit");
  const approach = road("us101-south-bayshore-exit-approach");
  const exit = road("us101-south-bayshore-north-turn-exit");
  for (const [from, to] of [
    [upstream, approach],
    [approach, exit],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` &&
          edge.to === `${to.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  const { surface, portals } = buildBakedRoadContact(exit);
  assert.deepEqual(surface.coveredRange, [136, 170]);
  assert.equal(
    buildBakedRoadContact({ ...exit, covered_range_m: [136, 200] }),
    null,
  );
  const deckNames = [
    ["us101-south-cesar-through", 145],
    ["us101-north-bayshore-through", 164],
  ];
  const decks = [];
  for (const [id, crossing] of deckNames) {
    const upper = buildBakedRoadContact(road(id)).surface;
    decks.push(upper);
    const {
      position: [x, y, z],
    } = surface.profile.find(({ s_m }) => s_m >= crossing);
    const above = upper.heightAt(x, y);
    assert.ok(above - z > 7, id);
    assert.equal(surface.heightAt(x, y), z);
    assert.equal(upper.coveredRange, undefined);
  }
  const profile = surface.profile;
  for (let i = 1; i < profile.length; i++) {
    const {
      s_m,
      position: [x, y, z],
    } = profile[i];
    if (s_m < 134) continue;
    const [px, py] = profile[i - 1].position;
    const car = { x, y, heading: Math.atan2(y - py, x - px) };
    const { position } = driveCameraPose(
      "zoomed",
      car,
      z,
      surface.heightAt,
      0,
      0,
      coveredRoadBlend(surface, car),
    );
    const [cx, cy, cz] = position;
    const samples = Math.ceil(Math.hypot(cx - x, cy - y) / 0.25);
    for (let step = 0; step <= samples; step++) {
      const t = step / samples;
      const sx = cx + (x - cx) * t;
      const sy = cy + (y - cy) * t;
      const sightline = cz + (z + 0.8 - cz) * t;
      for (const deck of decks) {
        const height = deck.heightAt(sx, sy);
        if (height === null) continue;
        assert.ok(
          height - sightline > 0.5,
          `Camera under ${deck.id} at ${s_m.toFixed(1)} m`,
        );
      }
    }
  }
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
  for (const [px, py] of [
    [2972, -1960],
    [3012, -3560],
  ]) {
    assert.ok(!tile.tr.some(([tx, ty]) => tx === px && ty === py));
  }
});

test("Monterey keeps its ramp mouths separate and Ocean Avenue reaches city pavement", () => {
  const tokens = ["808f7e64", "808f7e7c", "808f7ef4", "808f7e84", "808f7c2c"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) => playableRoads.map((r) => [r.id, r])),
  );
  const road = (slug) => roads.get(`sf-contact/${slug}`);
  const north = road("i280-north-monterey-entry");
  const south = road("i280-south-monterey-exit");
  const northContact = buildBakedRoadContact(north);
  const southContact = buildBakedRoadContact(south);
  const [entry] = northContact.portals;
  const [exit] = southContact.portals;
  assert.ok(Math.hypot(entry.x - exit.x, entry.y - exit.y) > 8.9);
  const ground = (x, y) => groundAtWorld(tiles, x, y, true);
  for (const { x, y, z } of [entry, exit]) {
    assert.ok(Math.abs(ground(x, y) - z) < 0.03);
  }
  const x = entry.x - 0.05 * entry.nx;
  const y = entry.y - 0.05 * entry.ny;
  const car = {
    x,
    y,
    z: ground(x, y),
    heading: Math.atan2(entry.ny, entry.nx),
    v: 10,
    grounded: true,
  };
  const arrived = driveThroughRoadPortal(
    car,
    { throttle: 1 },
    1 / 120,
    [southContact.portals, northContact.portals],
    ground,
  );
  assert.ok(arrived?.grounded);
  assert.equal(arrived.surfaceId, north.id);

  const southbound = road("i280-south-beyond-alemany");
  const ocean = road("i280-south-ocean-exit");
  for (const chain of [
    [
      north,
      road("i280-north-monterey-to-alemany"),
      road("i280-north-alemany-west-approach"),
      road("i280-north-alemany-main"),
    ],
    [southbound, south],
    [
      southbound,
      road("i280-south-monterey-mainline"),
      road("i280-south-monterey-to-ocean"),
      road("i280-south-ocean-approach"),
      ocean,
    ],
  ]) {
    for (const [i, to] of chain.slice(1).entries()) {
      const from = chain[i];
      assert.deepEqual(from.points.at(-1), to.points[0]);
      assert.deepEqual(from.seams.end, to.seams.start);
      assert.ok(
        roadBake.playableTransitions.some(
          (edge) =>
            edge.from === `${from.id}:end` &&
            edge.to === `${to.id}:start` &&
            edge.direction === "forward",
        ),
      );
    }
  }
  const [oceanExit] = buildBakedRoadContact(ocean).portals;
  assert.ok(Math.abs(ground(oceanExit.x, oceanExit.y) - oceanExit.z) < 0.03);
  const fromBothTiles = tiles
    .slice(0, 2)
    .map(({ playableRoads }) =>
      playableRoads.find(({ id }) => id === north.id),
    );
  assert.deepEqual(fromBothTiles[0], fromBothTiles[1]);
});

test("Central Freeway and Tenth Street join southbound US 101 while I-80 stays connected", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7e2c", "808f7e34"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const road = (tile, slug) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const central = road(tiles[0], "central-freeway-east-southbound");
  const i80 = road(tiles[0], "central-freeway-i80-east-connector");
  const viaduct = road(tiles[0], "us101-south-central-viaduct");
  const entry = road(tiles[0], "us101-south-tenth-bryant-entry");
  const merge = road(tiles[0], "us101-south-central-merge");
  const south = road(tiles[1], "us101-south-potrero-main");
  assert.deepEqual(merge, road(tiles[1], "us101-south-central-merge"));
  assert.deepEqual(merge.owners, ["808f7e2c", "808f7e34"]);
  for (const [from, to] of [
    [central, i80],
    [central, viaduct],
    [viaduct, merge],
    [entry, merge],
    [merge, south],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` &&
          edge.to === `${to.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  const { portals } = buildBakedRoadContact(entry);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  assert.equal(buildBakedRoadContact(merge).portals.length, 0);
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
    assert.equal(portals.length, road === mainline ? 1 : 2);
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

test("the Broadway portal lanes clear inferred trunks at their supported road levels", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const owners = ["808580ec", "808580f4"];
  const tiles = owners.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const indexes = tiles.map((tile) => buildCollisionIndex(tile, tile.t));
  const bores = tiles[0].playableRoads.filter(({ id }) =>
    id.startsWith("sf-contact/broadway-"),
  );
  assert.equal(bores.length, 2);
  for (const { id, points } of bores) {
    for (const [i, [x, y, z]] of points.slice(1).entries()) {
      const [priorX, priorY] = points[i];
      const heading = Math.atan2(y - priorY, x - priorX);
      for (const offset of [-0.8, 0, 0.8]) {
        const car = {
          x: x - offset * Math.sin(heading),
          y: y + offset * Math.cos(heading),
          heading,
        };
        assert.equal(collisionAt(indexes, car, z), null, `${id} at ${x}, ${y}`);
      }
    }
  }
  assert.deepEqual(
    owners.map((token) => roadBake.inferredTreePatches[token]),
    [4, 1],
  );
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
    roadBake.playableTransitions
      .filter(
        ({ to }) => to !== "sf-contact/presidio-doyle-northbound-exit:start",
      )
      .map(({ from, to }) => [from, to]),
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

test("Marina Boulevard and Main Post join the same northbound Presidio contact", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808586d4.json.gz")));
  const road = (slug) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const marina = road("presidio-marina-northbound-entry");
  const mainPost = road("main-post-northbound");
  const northbound = road("presidio-nb-between");
  assert.deepEqual(marina.groundEnds, ["start"]);
  assert.deepEqual(marina.widthTransitionMetres, { end: 24 });
  for (const entrance of [marina, mainPost]) {
    assert.deepEqual(entrance.points.at(-1), northbound.points[0]);
    assert.deepEqual(entrance.seams.end, northbound.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${entrance.id}:end` &&
          edge.to === `${northbound.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  const { portals, surface } = buildBakedRoadContact(marina);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
  const [east, north] = [-1116.81, 4840.67];
  assert.ok(
    surface.heightAt(east, north) - groundAtWorld([tile], east, north, true) >
      4,
  );
});

test("Presidio Parkway keeps its mainline and SR 1 on distinct branches across tiles", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tileOwners = ["808586dc", "808586ec"];
  const tiles = tileOwners.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const road = (tile, slug) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const mainlineSouth = road(tiles[0], "presidio-doyle-southbound-entry");
  const mainlineNorth = road(tiles[0], "presidio-doyle-northbound-exit");
  const viaductSouth = road(tiles[0], "presidio-sb-viaduct");
  const viaductNorth = road(tiles[0], "presidio-nb-viaduct");
  const originalExit = road(tiles[0], "presidio-nb-exit");
  for (const mainline of [mainlineSouth, mainlineNorth]) {
    assert.deepEqual(
      mainline,
      tiles[1].playableRoads.find(({ id }) => id === mainline.id),
    );
    assert.deepEqual([...new Set(mainline.owners)].sort(), tileOwners);
    const { portals } = buildBakedRoadContact(mainline);
    assert.equal(portals.length, 1);
    const [{ x, y, z }] = portals;
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  for (const [from, to] of [
    [mainlineSouth, viaductSouth],
    [viaductNorth, mainlineNorth],
    [viaductNorth, originalExit],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: start, to: end, direction }) =>
          start === `${from.id}:end` &&
          end === `${to.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const [x, y, z] = mainlineNorth.points[20];
  assert.ok(z - groundAtWorld(tiles, x, y, true) > 5);
  const obstacles = tiles.map((tile, i) =>
    buildCollisionIndex(tile, tileOwners[i]),
  );
  const north = buildBakedRoadContact(mainlineNorth).surface;
  const treeSite = { x: -2897.4, y: 4993.2, heading: 2.35 };
  assert.equal(
    collisionAt(obstacles, treeSite, north.heightAt(treeSite.x, treeSite.y)),
    null,
  );
  assert.ok(roadBake.inferredTreePatches["808586ec"] >= 1);
  assert.ok(
    Math.hypot(
      mainlineNorth.points.at(-1)[0] - originalExit.points.at(-1)[0],
      mainlineNorth.points.at(-1)[1] - originalExit.points.at(-1)[1],
    ) > 250,
  );
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

test("the southbound Bayshore turn stays above its refined ground and joins the city street", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e4c.json.gz")));
  const baked = (id) =>
    tile.playableRoads.find((road) => road.id === `sf-contact/${id}`);
  const stem = baked("us101-south-bayshore-exit-approach");
  const south = baked("us101-south-bayshore-south-turn-exit");
  const north = baked("us101-south-bayshore-north-turn-exit");
  for (const branch of [south, north]) {
    assert.deepEqual(stem.points.at(-1), branch.points[0]);
    assert.deepEqual(stem.seams.end, branch.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${stem.id}:end` &&
          edge.to === `${branch.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  const { surface, portals } = buildBakedRoadContact(south);
  assert.equal(tile.g.s, 8);
  assert.equal(tile.g.detail.s, 1);
  assert.ok(roadBake.terrainDetailPatches["808f7e4c"]);
  for (const {
    position: [x, y, z],
    s_m,
  } of surface.profile) {
    if (s_m < 205 || s_m > 248) continue;
    assert.ok(
      groundAtWorld([tile], x, y, true) - z < 0.15,
      `Ground over lane at ${s_m} m`,
    );
  }
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
});

test("the westbound Fifth exit crosses below the continuing freeway and reaches Harrison Street", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["8085807c", "80858084"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const route = (id) =>
    tiles[0].playableRoads.find((r) => r.id === `sf-contact/${id}`);
  const approach = route("i80-westbound-upper-soma");
  const exit = route("i80-westbound-fifth-harrison-exit");
  const through = route("i80-westbound-fourth-merge");
  for (const branch of [exit, through]) {
    assert.deepEqual(approach.points.at(-1), branch.points[0]);
    assert.deepEqual(approach.seams.end, branch.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${approach.id}:end` &&
          edge.to === `${branch.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  assert.deepEqual(exit.owners, tokens);
  assert.deepEqual(
    tiles[1].playableRoads.find((r) => r.id === exit.id),
    exit,
  );
  const { surface, portals } = buildBakedRoadContact(exit);
  assert.deepEqual(surface.coveredRange, [410, 445]);
  const upper = buildBakedRoadContact(through).surface;
  const {
    position: [x, y, z],
  } = surface.profile.find((point) => point.s_m >= 421);
  assert.ok(upper.heightAt(x, y) - z > 6);
  const [{ x: east, y: north, z: ground }] = portals;
  assert.ok(Math.abs(groundAtWorld(tiles, east, north, true) - ground) < 0.03);
  const indexes = tiles.map((tile, i) => buildCollisionIndex(tile, tokens[i]));
  const {
    position: [px, py, pz],
  } = upper.profile.find((point) => point.s_m >= 159);
  const heading = -2.435;
  const offset = -2;
  const pose = {
    x: px - Math.sin(heading) * offset,
    y: py + Math.cos(heading) * offset,
    z: pz,
    heading,
  };
  assert.equal(collisionAt(indexes, pose, pz), null);
  assert.equal(roadBake.overheadBuildingPatches["8085807c"], 2);
});

test("the Alemany fork carries both I-280 through traffic and the ramp to southbound US 101", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7ef4", "808f7efc", "808f7ee4"];
  const tiles = new Map(
    tokens.map((token) => [
      token,
      JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
    ]),
  );
  const route = (token, id) =>
    tiles
      .get(token)
      .playableRoads.find((road) => road.id === `sf-contact/${id}`);
  const approach = route(tokens[0], "i280-north-alemany-west-approach");
  const entry = route(tokens[0], "i280-north-alemany-entry");
  const through = route(tokens[0], "i280-north-alemany-main");
  const connector = route(tokens[0], "i280-north-to-us101-south-connector");
  const descent = route(tokens[1], "i280-to-us101-south-descent");
  const onward = route(tokens[2], "us101-south-paul-approach");
  for (const [before, after] of [
    [approach, through],
    [approach, connector],
    [entry, connector],
    [connector, descent],
    [descent, onward],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${before.id}:end` &&
          edge.to === `${after.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  assert.deepEqual(
    route(tokens[1], "i280-north-to-us101-south-connector"),
    connector,
  );
  assert.deepEqual(route(tokens[2], "i280-to-us101-south-descent"), descent);
  assert.equal(connector.points.at(-1)[2], 38.86);
  assert.equal(descent.points.at(-1)[2], 21.94);
  const x = 2336.03;
  const y = -3057.44;
  const z = buildBakedRoadContact(connector).surface.heightAt(x, y);
  assert.ok(Math.abs(z - 11.29) < 0.05);
  const index = buildCollisionIndex(tiles.get(tokens[0]), tokens[0]);
  assert.equal(collisionAt([index], { x, y, heading: 0.45 }, z), null);
});

test("outer lanes keep momentum across the 12 m Dogpatch seam", async () => {
  const { driveOnRoadNetwork, indexRoadNetwork } =
    await import("../sf-road-network.js");
  const tile = JSON.parse(gunzipSync(file("tiles/808f7fcc.json.gz")));
  const pieces = [
    "i280-dogpatch-northbound",
    "i280-south-of-eighteenth-northbound",
  ].map((name) =>
    buildBakedRoadContact(
      tile.playableRoads.find((r) => r.id === "sf-contact/" + name),
    ),
  );
  const [from, to] = pieces.map((p) => p.surface);
  const graph = indexRoadNetwork([{ id: "fixture", status: "joined" }]);
  graph.add({
    endpoints: pieces.flatMap((p) => p.endpoints),
    transitions: [
      {
        id: "fixture",
        from: from.end_ids[1],
        to: to.end_ids[0],
        support: "observed",
        direction: "both",
      },
    ],
  });
  const surfaces = new Map(pieces.map((p) => [p.surface.id, p.surface]));
  const [x, y, z] = from.profile.at(-1).position,
    bearing = from.terminalBearings[1];
  for (const side of [-4.4, 4.4])
    for (const direction of [-1, 1]) {
      const heading = bearing + (direction < 0 ? Math.PI : 0);
      let c = {
        x: x - 3 * Math.cos(heading) - side * Math.sin(bearing),
        y: y - 3 * Math.sin(heading) + side * Math.cos(bearing),
        z,
        v: 12,
        heading,
        grounded: true,
        surfaceId: (direction > 0 ? from : to).id,
      };
      for (let n = 0; n < 65; n++) {
        c = driveOnRoadNetwork(
          c,
          {},
          1 / 120,
          surfaces.get(c.surfaceId),
          surfaces,
          graph,
          () => false,
        );
        assert.equal(
          c.blocked,
          false,
          `side ${side}, direction ${direction}, step ${n}`,
        );
      }
      assert.equal(c.surfaceId, (direction > 0 ? to : from).id);
      assert.ok(c.v > 10);
    }
});

test("Marin and Cesar Chavez climb from street level into the northbound Bayshore merge", () => {
  const token = "808f7e4c";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const route = (id) =>
    tile.playableRoads.find((r) => r.id === `sf-contact/${id}`);
  const ramps = [
    route("us101-north-marin-entry"),
    route("us101-north-cesar-chavez-entry"),
  ];
  const merge = route("us101-north-bayshore-entry-merge");
  const through = route("us101-north-potrero-approach");
  for (const ramp of ramps) {
    const { portals } = buildBakedRoadContact(ramp);
    const [{ x, y, z }] = portals;
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
    assert.ok(ramp.points.at(-1)[2] - z > 9);
  }
  for (const [from, to] of [
    ...ramps.map((ramp) => [ramp, merge]),
    [merge, through],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` &&
          edge.to === `${to.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  const { surface } = buildBakedRoadContact(merge);
  const x = 3284.69;
  const y = -851.62;
  const z = surface.heightAt(x, y);
  assert.ok(Math.abs(z - 19.91) < 0.1);
  assert.equal(
    collisionAt([buildCollisionIndex(tile, token)], { x, y, heading: 1.69 }, z),
    null,
  );
  assert.equal(roadBake.inferredTreePatches[token], 8);
});

test("Mariposa Street stays at the lower level through the viaduct and connects to southbound I-280", () => {
  const token = "808f7fcc";
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = [token, "808f7fb4", "808f7fd4"].map((owner) =>
    JSON.parse(gunzipSync(file(`tiles/${owner}.json.gz`))),
  );
  const [tile] = tiles;
  const roads = new Map(
    tiles.flatMap((entry) =>
      entry.playableRoads.map((road) => [road.id, road]),
    ),
  );
  const ramp = roads.get("sf-contact/i280-south-mariposa-entry");
  const through = roads.get("sf-contact/i280-dogpatch-approach");
  assert.deepEqual(ramp.points.at(-1), through.points[0]);
  assert.deepEqual(ramp.seams.end, through.seams.start);
  assert.ok(
    roadBake.playableTransitions.some(
      (edge) =>
        edge.from === `${ramp.id}:end` &&
        edge.to === `${through.id}:start` &&
        edge.direction === "forward",
    ),
  );
  const { surface, portals } = buildBakedRoadContact(ramp);
  const heightAt = (x, y) => groundAtWorld([tile], x, y, true);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(heightAt(x, y) - z) < 0.03);
  assert.deepEqual(surface.coveredRange, [0, 152]);
  const upper = buildBakedRoadContact(roads.get("sf-contact/i280-18th-exit"));
  const {
    position: [lowerX, lowerY, lowerZ],
  } = surface.profile.find((point) => point.s_m >= 104);
  assert.ok(upper.surface.heightAt(lowerX, lowerY) - lowerZ > 7.5);
  assert.ok(heightAt(lowerX, lowerY) <= lowerZ);

  const index = buildCollisionIndex(tile, token);
  let car = {
    x: 4082,
    y: 448.3,
    z: heightAt(4082, 448.3),
    heading: Math.atan(0.078),
    v: 0,
    grounded: true,
  };
  let airborneSteps = 0;
  for (let frame = 0; frame < 120 * 25 && car.x < 4185; frame++) {
    car = contactStep(
      car,
      { steer: 0, throttle: car.v < 5.5 ? 1 : 0, brake: 0 },
      1 / 120,
      (x, y) => ({ height: heightAt(x, y), drivable: true }),
      1.35,
    );
    if (!car.grounded) {
      airborneSteps++;
      assert.ok(car.z - heightAt(car.x, car.y) < 0.001);
    }
    assert.ok(car.z > 11.25 && car.z < 13.25);
    assert.equal(collisionAt([index], car, car.z), null);
  }
  assert.ok(
    airborneSteps <= 2,
    `Lost ground contact for ${airborneSteps} steps`,
  );
  assert.ok(car.x >= 4185);
  assert.ok(roadBake.terrainDetailPatches[token]);
  assert.ok(!tile.tr.some(([x, y]) => x === 2480 && y === -2627));
});

test("southern northbound US 101 joins the existing Silver approach across tiles", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const owners = [
    "808f7e54",
    "808f7ee4",
    "808f7efc",
    "808f7f1c",
    "808f7f24",
    "808f7f2c",
  ];
  const tiles = new Map(
    owners.map((owner) => [
      owner,
      JSON.parse(gunzipSync(file(`tiles/${owner}.json.gz`))),
    ]),
  );
  const roads = new Map(
    [...tiles.values()].flatMap(({ playableRoads }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const mainline = [
    "us101-north-city-edge-to-hester",
    "us101-north-hester-to-bayshore",
    "us101-north-bayshore-to-paul",
    "us101-north-paul-to-silliman",
    "us101-north-silliman-to-alemany",
    "us101-north-alemany-to-silver",
    "us101-north-silver-approach",
  ].map((name) => roads.get(`sf-contact/${name}`));
  const ramp = roads.get("sf-contact/us101-north-alemany-entry");
  for (const [from, to] of [
    ...mainline.slice(0, -1).map((road, i) => [road, mainline[i + 1]]),
    [ramp, mainline[5]],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0], from.id);
    assert.deepEqual(from.seams.end, to.seams.start, from.id);
    assert.ok(
      roadBake.playableTransitions.some(
        (join) =>
          join.from === `${from.id}:end` &&
          join.to === `${to.id}:start` &&
          join.direction === "forward",
      ),
      from.id,
    );
    for (const owner of new Set(from.owners)) {
      assert.deepEqual(
        tiles.get(owner).playableRoads.find(({ id }) => id === from.id),
        from,
      );
    }
    assert.ok(
      from.points.every(
        (point, i) =>
          i === 0 ||
          Math.abs(point[2] - from.points[i - 1][2]) /
            Math.hypot(
              point[0] - from.points[i - 1][0],
              point[1] - from.points[i - 1][1],
            ) <
            (from === ramp ? 0.095 : 0.055),
      ),
    );
  }
  const hesterTile = tiles.get("808f7f24");
  const hester = buildBakedRoadContact(mainline[1]).surface;
  const x = 3657.92,
    y = -4875.17;
  const z = hester.heightAt(x, y);
  assert.ok(z > 40 && z < 41);
  assert.deepEqual(hester.coveredRange, [133, 177]);
  assert.equal(
    collisionAt(
      [buildCollisionIndex(hesterTile, "808f7f24")],
      { x, y, heading: 1.936 },
      z,
    ),
    null,
  );
  assert.ok(
    hesterTile.b.some(
      ([bottom, faces]) =>
        bottom === 480 &&
        faces.some(
          ([roof]) =>
            roof[2] === 494 && roof[0] + hesterTile.o[0] * 10 === 36598,
        ),
    ),
  );
  const { portals } = buildBakedRoadContact(ramp);
  const [entrance] = portals;
  assert.ok(
    Math.abs(
      groundAtWorld([tiles.get(ramp.owners[0])], entrance.x, entrance.y, true) -
        entrance.z,
    ) < 0.05,
  );
});

test("the northbound Silliman fork hands off to Bayshore Boulevard below the deck", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808f7efc.json.gz")));
  const road = (name) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`);
  const main = road("us101-north-paul-to-silliman");
  const through = road("us101-north-silliman-to-alemany");
  const exit = road("us101-north-silliman-exit");
  assert.deepEqual(exit.points[0], main.points.at(-1));
  assert.deepEqual(exit.seams.start, through.seams.start);
  for (const fork of [through, exit]) {
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${main.id}:end` &&
          to === `${fork.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(exit.groundEnds, ["end"]);
  const { portals, surface } = buildBakedRoadContact(exit);
  const { x, y, z } = portals[0];
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.01);
  assert.ok(z > 22 && z < 23);
  assert.deepEqual(surface.coveredRange, [185, 212]);
  const shadow = surface.profile.find(({ s_m }) => s_m >= 200).position;
  assert.ok(shadow[2] > 22 && shadow[2] < 23);
  assert.equal(
    collisionAt(
      [buildCollisionIndex(tile, "808f7efc")],
      { x: shadow[0], y: shadow[1], heading: 0.5 },
      shadow[2],
    ),
    null,
  );
});

test("the Alemany connector keeps both stacked crossings and the US 101 through fork", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7ee4", "808f7efc", "808f7f1c"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const road = (name) => roads.get(`sf-contact/${name}`);
  const approach = road("us101-north-bayshore-to-paul");
  const connector = road("us101-north-to-i280-south");
  const through = road("us101-north-paul-to-silliman");
  const merge = road("i280-south-alemany-through");
  for (const [from, to] of [
    [approach, connector],
    [approach, through],
    [connector, merge],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: a, to: b, direction, status }) =>
          a === `${from.id}:end` &&
          b === `${to.id}:start` &&
          direction === "forward" &&
          status === "joined",
      ),
    );
  }
  for (const tile of tiles) {
    assert.deepEqual(
      tile.playableRoads.find(({ id }) => id === connector.id),
      connector,
    );
  }
  assert.deepEqual(connector.groundEnds, []);
  const { surface } = buildBakedRoadContact(connector);
  assert.deepEqual(surface.coveredRange, [675, 1025]);
  const obstacles = tiles.map((tile, index) =>
    buildCollisionIndex(tile, tokens[index]),
  );
  for (const [upperName, x, y, heading] of [
    ["i280-north-to-us101-south-connector", 3125.926, -3079.465, 1.997],
    ["i280-north-alemany-climb", 2815.336, -2802.036, 3.046],
  ]) {
    const lower = surface.heightAt(x, y);
    const upper = buildBakedRoadContact(road(upperName)).surface.heightAt(x, y);
    assert.ok(Number.isFinite(lower) && Number.isFinite(upper));
    assert.ok(upper - lower > 7 && upper - lower < 8);
    assert.equal(collisionAt(obstacles, { x, y, heading }, lower), null);
  }
});

test("Bayshore stays on the upper street through the Hester fork and merges onto US 101", () => {
  const token = "808f7f24";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const road = (name) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`);
  const approach = road("bayshore-north-hester-approach");
  const street = road("bayshore-north-hester-through");
  const ramp = road("us101-north-bayshore-hester-entry");
  const highway = road("us101-north-bayshore-to-paul");
  for (const [from, to] of [
    [approach, street],
    [approach, ramp],
    [ramp, highway],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: a, to: b, direction, status }) =>
          a === `${from.id}:end` &&
          b === `${to.id}:start` &&
          direction === "forward" &&
          status === "joined",
      ),
    );
  }
  assert.deepEqual(ramp.groundEnds, []);
  const contact = buildBakedRoadContact(approach);
  for (const { x, y, z } of [
    ...contact.portals,
    ...buildBakedRoadContact(street).portals,
  ]) {
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.02);
  }
  const x = 3641.582;
  const y = -4906.127;
  const z = contact.surface.heightAt(x, y);
  const ground = groundAtWorld([tile], x, y, true);
  assert.ok(Number.isFinite(z) && Number.isFinite(ground) && z - ground > 7);
  assert.equal(
    collisionAt([buildCollisionIndex(tile, token)], { x, y, heading: 1.2 }, z),
    null,
  );
});

test("both southbound Hester entrances stay below US 101 and join the collector fork", () => {
  const token = "808f7f24";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const road = (name) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`);
  const third = road("us101-south-hester-third-entry");
  const salinas = road("us101-south-hester-salinas-entry");
  const collector = road("us101-south-hester-entrance-collector");
  const south = road("us101-south-hester-collector-return");
  const street = road("us101-south-hester-bayshore-exit");
  for (const [from, to] of [
    [third, collector],
    [salinas, collector],
    [collector, south],
    [collector, street],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: a, to: b, direction, status }) =>
          a === `${from.id}:end` &&
          b === `${to.id}:start` &&
          direction === "forward" &&
          status === "joined",
      ),
    );
  }
  for (const ramp of [third, salinas]) {
    const [{ x, y, z }] = buildBakedRoadContact(ramp).portals;
    assert.deepEqual(ramp.groundEnds, ["start"]);
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.02);
  }
  assert.ok(Math.abs(third.points[0][0] - 3809.5041) < 0.01);
  assert.deepEqual(collector.groundEnds, []);
  assert.ok(Math.abs(collector.points[0][2] - 34.2) < 0.01);
  const upper = road("us101-north-hester-to-bayshore");
  const [x, y] = [3569.7184, -4516.3596];
  const upperHeight = buildBakedRoadContact(upper).surface.heightAt(x, y);
  const lowerHeight = buildBakedRoadContact(third).surface.heightAt(x, y);
  assert.ok(Number.isFinite(upperHeight) && Number.isFinite(lowerHeight));
  assert.ok(upperHeight - lowerHeight > 8 && upperHeight - lowerHeight < 9);
});

test("northbound Hester retains the mainline and both exits reach city pavement", () => {
  const token = "808f7f24";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const road = (name) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`);
  const approach = road("us101-north-city-edge-to-hester");
  const mainline = road("us101-north-hester-to-bayshore");
  const fork = road("us101-north-hester-off-ramp");
  const third = road("us101-north-hester-third-off-ramp");
  const bayshore = road("us101-north-hester-bayshore-off-ramp");
  for (const [from, to] of [
    [approach, mainline],
    [approach, fork],
    [fork, third],
    [fork, bayshore],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: a, to: b, direction, status }) =>
          a === `${from.id}:end` &&
          b === `${to.id}:start` &&
          direction === "forward" &&
          status === "joined",
      ),
    );
  }
  assert.deepEqual(fork.groundEnds, []);
  for (const exit of [third, bayshore]) {
    assert.deepEqual(exit.groundEnds, ["end"]);
    const [{ x, y, z }] = buildBakedRoadContact(exit).portals;
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.02);
  }
  const obstacles = [buildCollisionIndex(tile, token)];
  const throughPose = { x: 3728.98, y: -5012.988, heading: 2.0099 };
  const throughHeight = buildBakedRoadContact(mainline).surface.heightAt(
    throughPose.x,
    throughPose.y,
  );
  assert.ok(Number.isFinite(throughHeight));
  assert.equal(collisionAt(obstacles, throughPose, throughHeight), null);
  const reentry = buildBakedRoadContact(road("us101-south-hester-third-entry"));
  for (const [x, y, heading] of [
    [3678.8, -4457.4, 3.46],
    [3582.029, -4503.235, -2.365],
  ]) {
    const z = reentry.surface.heightAt(x, y);
    assert.ok(Number.isFinite(z));
    assert.equal(collisionAt(obstacles, { x, y, heading }, z), null);
  }
  for (const [ramp, x, y] of [
    [fork, 3670.3473, -4817.6891],
    [third, 3704.4769, -4632.0553],
  ]) {
    const z = buildBakedRoadContact(ramp).surface.heightAt(x, y);
    const separation = z - groundAtWorld([tile], x, y, true);
    assert.ok(separation > 0.08 && separation < 0.17);
    assert.equal(collisionAt(obstacles, { x, y, heading: 1.4 }, z), null);
  }
});

test("west Yerba Buena ramps join the island to the correct Bay Bridge decks", () => {
  const tokens = ["8085803c", "80858024"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const road = (name) =>
    tiles[0].playableRoads.find(
      ({ id }) => id === name || id === `sf-contact/${name}`,
    );
  const fromCity = road("bay-bridge-eastbound-lower");
  const lowerThrough = road("yerba-buena-lower-west-approach");
  const toIsland = road("yerba-buena-eastbound-off-ramp");
  const fromIsland = road("yerba-buena-westbound-on-ramp");
  const upperThrough = road("yerba-buena-west-approach");
  const toCity = road("bay-bridge-westbound");
  for (const [from, to] of [
    [fromCity, lowerThrough],
    [fromCity, toIsland],
    [fromIsland, toCity],
    [upperThrough, toCity],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: a, to: b, direction, status }) =>
          a === `${from.id}:end` &&
          b === `${to.id}:start` &&
          direction === "forward" &&
          status === "joined",
      ),
    );
  }
  assert.deepEqual(toIsland.groundEnds, ["end"]);
  assert.deepEqual(fromIsland.groundEnds, ["start"]);
  const lower = buildBakedRoadContact(toIsland);
  const upper = buildBakedRoadContact(fromIsland);
  for (const { x, y, z } of [...lower.portals, ...upper.portals]) {
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.02);
  }
  const obstacles = tiles.map((tile, i) =>
    buildCollisionIndex(tile, tokens[i]),
  );
  for (const [x, y] of [
    [6420.6852, 5368.4154],
    [6440, 5402],
    [6329.863, 5420.266],
  ]) {
    const bottom = lower.surface.heightAt(x, y);
    const top = upper.surface.heightAt(x, y);
    assert.ok(Number.isFinite(top) && Number.isFinite(bottom));
    assert.ok(top - bottom > 7 && top - bottom < 9);
    assert.ok(groundAtWorld(tiles, x, y, true) - bottom < 0.04);
    assert.equal(collisionAt(obstacles, { x, y, heading: 1.01 }, bottom), null);
    assert.equal(collisionAt(obstacles, { x, y, heading: -2.1 }, top), null);
  }
  const upperPoints = fromIsland.points;
  let station = 0;
  let probes = 0;
  for (let i = 1; i < upperPoints.length; i++) {
    const [a, b] = upperPoints[i - 1];
    const [x, y] = upperPoints[i];
    const distance = Math.hypot(x - a, y - b);
    station += distance;
    if (station < 220 || station > 308) continue;
    for (const offset of [-2.65, 2.65]) {
      const east = x - (offset * (y - b)) / distance;
      const north = y + (offset * (x - a)) / distance;
      const deck = upper.surface.heightAt(east, north);
      assert.ok(Number.isFinite(deck));
      assert.ok(groundAtWorld(tiles, east, north, true) - deck < 0.1);
      probes++;
    }
  }
  assert.ok(probes > 60);
});

test("Minna Street remains drivable under the retained building bridge", () => {
  const token = "80858084";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const obstacles = [buildCollisionIndex(tile, token)];
  const [startX, startY] = [3000.9, 2475.6];
  const heading = Math.atan2(2407.2 - startY, 2932.2 - startX);
  const along = Math.cos(heading);
  const north = Math.sin(heading);
  function pose(station, lateral) {
    return {
      x: startX + station * along - lateral * north,
      y: startY + station * north + lateral * along,
      heading,
    };
  }
  for (const lateral of [-1.5, 0, 1.5]) {
    let previous;
    for (let station = 13; station <= 85; station++) {
      const car = pose(station, lateral);
      const z = groundAtWorld([tile], car.x, car.y, true);
      assert.ok(z >= 8 && z <= 9.1, `pavement at ${station} m`);
      if (previous !== undefined) assert.ok(Math.abs(z - previous) < 0.12);
      previous = z;
      assert.equal(
        collisionAt(obstacles, car, z),
        null,
        `ground clearance at ${station} m, lateral ${lateral} m`,
      );
    }
  }
  assert.equal(collisionAt(obstacles, pose(42, 0), 13), "building");
  for (const lateral of [-5.5, 5.5]) {
    const car = pose(42, lateral);
    const z = groundAtWorld([tile], car.x, car.y, true);
    assert.equal(collisionAt(obstacles, car, z), "building");
  }
});

test("the Macalla loop merges into westbound I-80 at the upper level", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["8085801c", "80858024", "8085803c"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads = [] }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const ramp = roads.get("sf-contact/macalla-road-westbound-i80-entrance");
  const near = roads.get("yerba-buena-east-approach");
  const through = roads.get("sf-contact/yerba-buena-upper-after-macalla-merge");
  const tunnel = roads.get("yerba-buena-upper-tunnel");
  assert.ok(ramp, "the signed entrance has its own playable contact");
  for (const [before, after] of [
    [near, through],
    [ramp, through],
    [through, tunnel],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const { surface, portals } = buildBakedRoadContact(ramp);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.02);
  assert.ok(surface.profile.at(-1).s_m > 300);
  assert.ok(groundAtWorld(tiles, 6710.14, 5802.76, true) < 40);
  assert.ok(surface.heightAt(6710.14, 5802.76) > 56);
  const index = tiles.map((tile) => buildCollisionIndex(tile, tile.t));
  for (let i = 1; i < ramp.points.length; i++) {
    const [a, b, h] = ramp.points[i - 1];
    const [x, y, z] = ramp.points[i];
    const heading = Math.atan2(y - b, x - a);
    assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.12);
    for (const lateral of [-2.25, 0, 2.25]) {
      const car = {
        x: x - lateral * Math.sin(heading),
        y: y + lateral * Math.cos(heading),
        heading,
      };
      assert.equal(collisionAt(index, car, z), null, `Macalla at ${x}, ${y}`);
    }
  }
  const drive = JSON.parse(file("gamedrives.json")).drives[0];
  const named = cityDriveApproach(drive, "yerba-buena-westbound");
  for (const road of [near, through]) {
    assert.deepEqual(
      named.sections.find(({ id }) => id === road.id).positions,
      road.points,
    );
  }
});

test("both Golden Gate carriageways connect the Presidio and Marin above terrain", () => {
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const coverage = JSON.parse(file("coverage.json"));
  const tokens = [
    "808586dc",
    "808586e4",
    "808586ec",
    "80858694",
    "8085869c",
    "80858424",
    "8085842c",
  ];
  const tiles = tokens.map((token) => {
    assert.ok(
      manifest.tiles.some(({ t }) => t === token),
      token,
    );
    assert.equal(coverage.cells[token].length, 4, token);
    return JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  });
  const indexes = tiles.map((tile) => buildCollisionIndex(tile, tile.t));
  const roads = new Map(
    tiles.flatMap(({ playableRoads = [] }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  for (const direction of ["northbound", "southbound"]) {
    const crossing = Array.from({ length: 6 }, (_, i) =>
      roads.get(`sf-contact/golden-gate-${direction}-${i + 1}`),
    );
    const northbound = direction === "northbound";
    const doyle = roads.get(
      `sf-contact/presidio-doyle-${direction}-${northbound ? "exit" : "entry"}`,
    );
    const chain = northbound ? [doyle, ...crossing] : [...crossing, doyle];
    for (let i = 1; i < chain.length; i++) {
      const previous = chain[i - 1];
      const next = chain[i];
      assert.deepEqual(previous.points.at(-1), next.points[0], next.id);
      assert.deepEqual(previous.seams.end, next.seams.start, next.id);
      assert.ok(
        manifest.roadBake.playableTransitions.some(
          ({ from, to, direction }) =>
            from === `${previous.id}:end` &&
            to === `${next.id}:start` &&
            direction === "forward",
        ),
        next.id,
      );
    }

    let span = 0;
    for (const [section, road] of crossing.entries()) {
      const { surface } = buildBakedRoadContact(road);
      if (section > 0 && section < crossing.length - 1) {
        assert.equal(road.width, 9.4);
        assert.deepEqual(surface.railSides, ["left", "right"]);
        span += surface.profile.at(-1).s_m;
      }
      for (let i = 1; i < road.points.length; i++) {
        const [a, b, h] = road.points[i - 1];
        const [x, y, z] = road.points[i];
        const heading = Math.atan2(y - b, x - a);
        assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.05, road.id);
        for (const lateral of [-2.9, 0, 2.9]) {
          const car = {
            x: x - lateral * Math.sin(heading),
            y: y + lateral * Math.cos(heading),
            heading,
          };
          assert.equal(
            collisionAt(indexes, car, z),
            null,
            `${road.id} ${i} lateral ${lateral}`,
          );
        }
      }
    }
    assert.ok(span > 2700 && span < 2800, `${direction} span ${span}`);
    const marin = northbound ? crossing.at(-1) : crossing[0];
    const [{ x, y, z }] = buildBakedRoadContact(marin).portals;
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.02);
    const [deckX, deckY, deckZ] = crossing[2].points[190];
    assert.ok(groundAtWorld(tiles, deckX, deckY, true) < deckZ - 65);
  }
});

test("Main Street stays open beneath both Bay Bridge decks", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/80858074.json.gz")));
  const obstacles = [buildCollisionIndex(tile, tile.t)];
  const heading = (3 * Math.PI) / 4;
  const pose = (x, offset) => ({
    x: x - offset * Math.sin(heading),
    y: 7499.4 - x + offset * Math.cos(heading),
    heading,
  });
  const hit = (car) =>
    collisionAt(obstacles, car, groundAtWorld([tile], car.x, car.y, true));
  for (let x = 4439; x <= 4478; x += 0.5) {
    for (const offset of [-3, 0, 3]) {
      assert.equal(
        hit(pose(x, offset)),
        null,
        `Main Street at ${x}, ${offset}`,
      );
    }
  }
  assert.equal(hit(pose(4467, -9)), "building");
  assert.equal(hit(pose(4467, 9)), "building");
  const { x, y } = pose(4461, 0);
  const height = (id) =>
    buildBakedRoadContact(
      tile.playableRoads.find((road) => road.id === id),
    ).surface.heightAt(x, y);
  assert.ok(groundAtWorld([tile], x, y, true) < 6);
  assert.ok(Math.abs(height("sf-eastbound-lower-approach") - 41.39) < 0.02);
  assert.ok(Math.abs(height("city-approach") - 50.18) < 0.02);
});

test("Harrison traffic crosses the Fourth Street ramp mouth without taking it", () => {
  const tiles = ["8085807c", "80858084"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const road = tiles[0].playableRoads.find(
    ({ id }) => id === "fourth-street-entrance",
  );
  const { surface, portals } = buildBakedRoadContact(road);
  const [{ x, y, nx, ny }] = portals;
  const ground = (east, north) => groundAtWorld(tiles, east, north, true);
  for (const [east, north, heading, wantsRamp] of [
    [3570.3, 2321.8, (-3 * Math.PI) / 4, false],
    [x - 2 * nx, y - 2 * ny, Math.atan2(ny, nx), true],
  ]) {
    let car = {
      x: east,
      y: north,
      z: ground(east, north),
      heading,
      v: 10,
      grounded: true,
    };
    for (let step = 0; step < 480; step++) {
      const input = { throttle: 0.1 };
      car =
        driveThroughRoadPortal(car, input, 1 / 120, [portals], ground) ??
        contactStep(
          car,
          input,
          1 / 120,
          (px, py) => ({
            height: ground(px, py),
            drivable: true,
          }),
          1.35,
        );
      assert.equal(car.blocked, false);
      assert.ok(Math.abs(car.z - ground(car.x, car.y)) < 0.3);
      if (car.surfaceId || car.x < 3543) break;
    }
    assert.equal(car.surfaceId, wantsRamp ? surface.id : undefined);
    if (!wantsRamp)
      assert.ok(car.x < 3543, "through traffic cleared the mouth");
  }
});

test("Richland and Highland carry traffic above the San Jose Avenue cut", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e64.json.gz")));
  const obstacles = [buildCollisionIndex(tile, tile.t)];
  const ground = (x, y) => groundAtWorld([tile], x, y, true);
  for (const [id, length, clearance] of [
    ["richland", 85, 13],
    ["highland", 80, 10],
  ]) {
    const road = tile.playableRoads.find(
      ({ id: name }) => name === `sf-contact/${id}-avenue-bridge`,
    );
    const { surface, portals } = buildBakedRoadContact(road);
    assert.equal(road.width, 7.2);
    assert.equal(portals.length, 2);
    assert.ok(surface.profile.at(-1).s_m > length);
    for (const { x, y, z } of portals) {
      assert.ok(Math.abs(ground(x, y) - z) < 0.02);
    }
    const [east, north, height] =
      road.points[Math.floor(road.points.length / 2)];
    assert.ok(height - ground(east, north) > clearance);
    for (let i = 1; i < road.points.length; i++) {
      const [x, y, z] = road.points[i];
      const [a, b, h] = road.points[i - 1];
      const heading = Math.atan2(y - b, x - a);
      assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.05);
      for (const offset of [-2.4, 0, 2.4]) {
        const car = {
          x: x - offset * Math.sin(heading),
          y: y + offset * Math.cos(heading),
          heading,
        };
        assert.equal(collisionAt(obstacles, car, z), null, `${id} point ${i}`);
      }
    }
  }
  for (const [x, y] of [
    [1178.9, -2628.1],
    [1308.1, -2488.5],
  ]) {
    const z = ground(x, y);
    assert.ok(z > 55 && z < 60);
    assert.equal(collisionAt(obstacles, { x, y, heading: -2.3 }, z), null);
  }
});

test("Ocean Avenue crosses above I-280 and the northbound entrance joins at Monterey", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = [
    "808f7c2c",
    "808f7e84",
    "808f7e7c",
    "808f7e64",
    "808f7ef4",
  ].map((token) => JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))));
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const crossing = roads.get("sf-contact/ocean-avenue-bridge");
  assert.equal(crossing.width, 16);
  for (const owner of crossing.owners) {
    const tile = tiles.find(({ t }) => t === owner);
    assert.deepEqual(
      tile.playableRoads.find(({ id }) => id === crossing.id),
      crossing,
    );
  }
  const { surface, portals } = buildBakedRoadContact(crossing);
  assert.equal(portals.length, 2);
  assert.ok(
    surface.heightAt(-690, -4111.4) -
      groundAtWorld(tiles, -690, -4111.4, true) >
      6.5,
  );
  for (const { x, y, z } of portals) {
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.02);
  }

  const chain = [
    "i280-north-ocean-entry",
    "i280-north-ocean-connector",
    "i280-north-ocean-mainline",
    "i280-north-ocean-monterey",
    "branch-east-i280-northbound",
    "i280-north-monterey-merge",
    "i280-north-monterey-to-alemany",
  ].map((slug) => roads.get(`sf-contact/${slug}`));
  const [entry] = chain;
  const [east, north, height] = entry.points[0];
  assert.ok(Math.abs(groundAtWorld(tiles, east, north, true) - height) < 0.02);
  const [streetX, streetY] = crossing.points[0];
  assert.ok(Math.hypot(east - streetX, north - streetY) > 12);
  assert.deepEqual(entry.groundEnds, ["start"]);
  assert.deepEqual(chain[4].groundEnds, ["start"]);
  for (let i = 1; i < chain.length; i++) {
    const a = chain[i - 1];
    const b = chain[i];
    assert.deepEqual(a.points.at(-1), b.points[0]);
    assert.deepEqual(a.seams.end, b.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${a.id}:end` &&
          to === `${b.id}:start` &&
          direction === "forward",
      ),
      b.id,
    );
  }

  const decks = deckClearanceIndex();
  for (const road of roads.values()) {
    if (road.kind === "tunnel") continue;
    const contact = buildBakedRoadContact(road).surface.contact;
    const geometry = contactGeometry(contact, false);
    decks.add(geometry);
    geometry.dispose();
  }
  const obstacles = tiles.map((tile) =>
    buildCollisionIndex(tile, tile.t, new Map(), decks.ceiling),
  );
  for (const road of [crossing, ...chain.slice(0, -1)]) {
    for (let i = 1; i < road.points.length; i++) {
      const [x, y, z] = road.points[i];
      const [a, b, h] = road.points[i - 1];
      const heading = Math.atan2(y - b, x - a);
      assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.1, road.id);
      const lane = road.width < 10 ? 2.2 : 3.6;
      for (const offset of [-lane, 0, lane]) {
        const car = {
          x: x - offset * Math.sin(heading),
          y: y + offset * Math.cos(heading),
          heading,
        };
        assert.equal(
          collisionAt(obstacles, car, z),
          null,
          `${road.id} point ${i}, offset ${offset}`,
        );
      }
    }
  }
});

test("Geneva keeps both I-280 carriageways and their four ramps connected", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = [
    "808f7c24",
    "808f7c2c",
    "808f7e64",
    "808f7e7c",
    "808f7e84",
  ].map((token) => JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))));
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const road = (slug) => roads.get(`sf-contact/i280-${slug}`);
  const north = road("north-geneva-to-ocean");
  const south = road("south-ocean-to-geneva");
  for (const [a, b] of [
    ["north-geneva-approach", "north-geneva-to-ocean"],
    ["north-geneva-approach", "north-geneva-exit"],
    ["north-geneva-to-ocean", "north-ocean-mainline"],
    ["north-geneva-entry", "north-ocean-connector"],
    ["south-monterey-to-ocean", "south-ocean-to-geneva"],
    ["south-ocean-approach", "south-geneva-exit"],
    ["south-ocean-to-geneva", "south-beyond-geneva"],
    ["south-geneva-entry", "south-beyond-geneva"],
  ]) {
    const from = road(a);
    const to = road(b);
    assert.deepEqual(from.points.at(-1), to.points[0], to.id);
    assert.deepEqual(from.seams.end, to.seams.start, to.id);
    assert.ok(
      roadBake.playableTransitions.some(
        (join) =>
          join.from === `${from.id}:end` &&
          join.to === `${to.id}:start` &&
          join.direction === "forward",
      ),
      to.id,
    );
  }
  for (const direction of ["north", "south"]) {
    const exit = road(`${direction}-geneva-exit`);
    const entry = road(`${direction}-geneva-entry`);
    assert.deepEqual(exit.groundExitOnly, ["end"]);
    assert.deepEqual(entry.groundEnds, ["start"]);
    assert.deepEqual(exit.points.at(-1), entry.points[0]);
    const [x, y, z] = entry.points[0];
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.02);
    assert.ok(
      !roadBake.playableTransitions.some(
        (join) => join.from === `${exit.id}:end`,
      ),
    );
  }

  const decks = deckClearanceIndex();
  for (const item of roads.values()) {
    if (item.kind === "tunnel") continue;
    const contact = buildBakedRoadContact(item).surface.contact;
    const geometry = contactGeometry(contact, false);
    decks.add(geometry);
    geometry.dispose();
  }
  const obstacles = tiles.map((tile) =>
    buildCollisionIndex(tile, tile.t, new Map(), decks.ceiling),
  );
  for (const item of [
    north,
    south,
    road("north-geneva-approach"),
    road("south-beyond-geneva"),
    ...["north", "south"].flatMap((direction) => [
      road(`${direction}-geneva-entry`),
      road(`${direction}-geneva-exit`),
    ]),
  ]) {
    for (const owner of item.owners) {
      assert.deepEqual(
        tiles
          .find(({ t }) => t === owner)
          .playableRoads.find(({ id }) => id === item.id),
        item,
      );
    }
    const { surface } = buildBakedRoadContact(item);
    for (const [i, [x, y, z]] of item.points.entries()) {
      if (!i) continue;
      const [a, b, h] = item.points[i - 1];
      const heading = Math.atan2(y - b, x - a);
      assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.1, item.id);
      assert.equal(collisionAt(obstacles, { x, y, heading }, z), null, item.id);
    }
    const [x, y, z] = item.points[Math.floor(item.points.length / 2)];
    assert.ok(Math.abs(surface.heightAt(x, y) - z) < 0.005, item.id);
  }
  for (const [item, x, y, wanted] of [
    [north, -705.46, -4308.72, 68.89],
    [south, -724.16, -4297.39, 69.44],
    [road("north-geneva-entry"), -657.73, -4110.48, 64.79],
    [road("south-geneva-exit"), -713.81, -4107.39, 66.5],
  ]) {
    const z = buildBakedRoadContact(item).surface.heightAt(x, y);
    assert.ok(Math.abs(z - wanted) < 0.08, item.id);
    assert.ok(groundAtWorld(tiles, x, y, true) - z > 1.5, item.id);
  }
});

test("Blanken Avenue crosses below US 101 and releases at both street portals", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7f2c.json.gz")));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/blanken-avenue-underpass",
  );
  const { surface, portals, appearance } = buildBakedRoadContact(road);
  assert.equal(appearance.clearance, 4.5);
  assert.equal(portals.length, 2);
  assert.ok(appearance.interior[1] - appearance.interior[0] > 55);
  for (const { x, y, z } of portals) {
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.02);
  }
  const [x, y, z] = road.points[Math.floor(road.points.length / 2)];
  assert.ok(groundAtWorld([tile], x, y, true) - z > 10);
  assert.ok(Math.abs(surface.heightAt(x, y) - z) < 0.01);
  for (const [slug, east, north] of [
    ["us101-north-city-edge-to-hester", 3930, -5446.5],
    ["us101-south-hester-to-city-edge", 3907, -5447.5],
  ]) {
    const upper = buildBakedRoadContact(
      tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`),
    ).surface.heightAt(east, north);
    const lower = surface.heightAt(east, north);
    assert.ok(Number.isFinite(upper) && Number.isFinite(lower));
    assert.ok(upper - lower > 11, slug);
  }
  const paint = tile.r.filter(([name]) => tile.n[name] === "Blanken Ave");
  assert.ok(paint.length > 1);
  const tileEast = Math.round((x - tile.o[0]) * 10);
  for (const [, , , line] of paint) {
    const east = line.filter((_, i) => i % 2 === 0);
    assert.ok(Math.max(...east) < tileEast || Math.min(...east) > tileEast);
  }

  const obstacles = [buildCollisionIndex(tile, tile.t)];
  for (const [i, [x, y, z]] of road.points.entries()) {
    if (!i) continue;
    const [a, b, h] = road.points[i - 1];
    const heading = Math.atan2(y - b, x - a);
    assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.05);
    for (const offset of [-2.5, 0, 2.5]) {
      const car = {
        x: x - offset * Math.sin(heading),
        y: y + offset * Math.cos(heading),
        heading,
      };
      assert.equal(collisionAt(obstacles, car, z), null, `point ${i}`);
    }
  }
});

test("I-280 follows the surveyed carriageways and directed SR 82 ramps south of Geneva", () => {
  const { tiles: owners, roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = [
    "808f7c24",
    "808f7c2c",
    "808f7c34",
    "808f7c3c",
    "808f7e84",
  ].map((token) => JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))));
  const roads = new Map(
    tiles.flatMap((tile) =>
      (tile.playableRoads ?? []).map((road) => [road.id, road]),
    ),
  );
  const road = (slug) => roads.get(`sf-contact/i280-${slug}`);
  const northFirst = road("north-edge-to-sr82");
  const northNext = road("north-sr82-to-geneva");
  const northApproach = road("north-geneva-approach");
  const northEntry = road("north-sr82-entry");
  const southApproach = road("south-beyond-geneva");
  const southNext = road("south-geneva-to-sr82");
  const southLast = road("south-sr82-to-edge");
  const southExit = road("south-sr82-exit");
  const edgeExit = road("south-edge-exit");

  for (const [from, to] of [
    [northFirst, northNext],
    [northEntry, northNext],
    [northNext, northApproach],
    [southApproach, southNext],
    [southApproach, southExit],
    [southNext, southLast],
    [southNext, edgeExit],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0], to.id);
    assert.deepEqual(from.seams.end, to.seams.start, to.id);
    assert.ok(
      roadBake.playableTransitions.some(
        (join) =>
          join.from === `${from.id}:end` &&
          join.to === `${to.id}:start` &&
          join.direction === "forward",
      ),
      to.id,
    );
  }
  for (const item of [northApproach, southApproach])
    assert.deepEqual(item.groundEnds, [], item.id);
  for (const exit of [southExit, edgeExit]) {
    assert.deepEqual(exit.groundEnds, ["end"]);
    assert.deepEqual(exit.groundExitOnly, ["end"]);
  }
  assert.deepEqual(northEntry.groundEnds, ["start"]);
  const [east, north, height] = northEntry.points[0];
  const lead = Math.hypot(east + 1504.46, north + 5537.32);
  assert.ok(lead >= 14 && lead < 18);
  assert.ok(Math.abs(groundAtWorld(tiles, east, north, true) - height) < 0.02);

  const decks = deckClearanceIndex();
  for (const item of roads.values()) {
    if (item.kind === "tunnel") continue;
    const geometry = contactGeometry(
      buildBakedRoadContact(item).surface.contact,
      false,
    );
    decks.add(geometry);
    geometry.dispose();
  }
  const obstacles = tiles.map((tile) =>
    buildCollisionIndex(tile, tile.t, new Map(), decks.ceiling),
  );
  const tileByToken = new Map(tiles.map((tile) => [tile.t, tile]));
  for (const item of [
    northFirst,
    northNext,
    northEntry,
    southNext,
    southLast,
    southExit,
    edgeExit,
  ]) {
    const seen = new Set();
    for (const [i, [x, y, z]] of item.points.entries()) {
      const closest = owners.reduce((a, b) =>
        (x - a.x) ** 2 + (y - a.y) ** 2 < (x - b.x) ** 2 + (y - b.y) ** 2
          ? a
          : b,
      );
      seen.add(closest.t);
      if (!i) continue;
      const [a, b, h] = item.points[i - 1];
      const heading = Math.atan2(y - b, x - a);
      assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.08, item.id);
      assert.equal(collisionAt(obstacles, { x, y, heading }, z), null, item.id);
    }
    for (const token of seen) {
      const copy = tileByToken
        .get(token)
        .playableRoads.find(({ id }) => id === item.id);
      assert.deepEqual(copy, item, `${item.id} in ${token}`);
    }
  }
  for (const item of [northFirst, southNext]) {
    const differences = item.points.flatMap(([x, y, z]) => {
      const ground = groundAtWorld(tiles, x, y, true);
      return Number.isFinite(ground) ? [z - ground] : [];
    });
    assert.ok(Math.max(...differences) > 6, item.id);
  }
});
