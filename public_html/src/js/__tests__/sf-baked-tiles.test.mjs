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
import {
  highwayRailSegments,
  leaveRoadSide,
  roadBoundary,
  strikeHighwayRail,
} from "../sf-highway-edge.js";

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
  const tunnel = selected.sections.find(({ kind }) => kind === "tunnel");
  assert.equal(tunnel.id, "yerba-buena-lower-tunnel");
  assert.equal(tunnel.game_clearance_m, 4.9);
  let length = 0;
  for (let i = 1; i < tunnel.positions.length; i++) {
    const [ax, ay, az] = tunnel.positions[i - 1];
    const [bx, by, bz] = tunnel.positions[i];
    const ds = Math.hypot(bx - ax, by - ay);
    assert.ok(Math.abs(bz - az) / ds < 0.05);
    length += ds;
  }
  assert.ok(length > 286 && length < 288);
  assert.ok(tunnel.positions[0][2] < 44 && tunnel.positions.at(-1)[2] > 51);
  for (let i = 1; i < selected.sections.length; i++) {
    assert.deepEqual(
      selected.sections[i - 1].positions.at(-1),
      selected.sections[i].positions[0],
    );
  }
  assert.equal(selected.sections.at(-1).id, "yerba-buena-lower-east-approach");
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
  assert.ok(appearance.interior[1] > 286);
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

test("Ruckman keeps both SR 1 decks above the street and meets the northbound connector", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808586dc.json.gz")));
  for (const [direction, x] of [
    ["northbound", -2589],
    ["southbound", -2596.4],
  ]) {
    const road = tile.playableRoads.find(
      ({ id }) => id === `sf-contact/ruckman-viaduct-${direction}`,
    );
    const { surface, portals } = buildBakedRoadContact(road);
    assert.equal(portals.length, 2);
    for (const { x, y, z } of portals) {
      assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
    }
    const street = groundAtWorld([tile], x, 4547, true);
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
