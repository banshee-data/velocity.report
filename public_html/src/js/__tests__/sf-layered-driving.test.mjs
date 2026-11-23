import assert from "node:assert/strict";
import { test } from "node:test";
import * as THREE from "../../vendor/three/three.module.js";
import { createFlightTimeline, sampleFlightProfile } from "../sf-airtime.js";
import { buildingWalls, validBuildingFace } from "../sf-building-walls.js";
import { createCarReveal } from "../sf-car-reveal.js";
import {
  applyCityDriveScenery,
  BAY_BRIDGE_CITY_DRIVE,
  buildCityGameDrive,
  cityDriveControls,
  cityDriveCanResume,
  cityDriveOnRoute,
  driveOnCityRoute,
} from "../sf-city-road-drive.js";
import { CAR_HALF_LENGTH, contactStep } from "../sf-drive-core.js";
import {
  BAY_BRIDGE_OVERLAY,
  CITY_ROAD_LAYERS,
  REGIONAL_ROAD_CATALOG,
  mapSources,
} from "../sf-drive-settings.js";
import {
  driveOnRoadSurface,
  driveOnStreetContact,
  indexRoadContact,
} from "../sf-road-contact.js";
import {
  crossRampEntrance,
  driveOnRoadNetwork,
  indexRoadNetwork,
  regionalStart,
} from "../sf-road-network.js";
import { MPH, soloHeight } from "../sf-solo.js";
import {
  barrierImpact,
  fallFromRoad,
  highwayRailSegments,
  leaveRoadSide,
  roadBoundary,
  roadLandingAt,
  strikeHighwayRail,
} from "../sf-highway-edge.js";

test("map settings accept a planar tile pack and the Gerardus road manifests", () => {
  assert.deepEqual(
    mapSources({
      tiles: "https://maps.example/sf/tiles/manifest.json",
      gerardus: "http://arrow-worker:50061",
    }),
    {
      tiles: "https://maps.example/sf/tiles/",
      overlay: BAY_BRIDGE_OVERLAY,
      catalog: REGIONAL_ROAD_CATALOG,
      city: CITY_ROAD_LAYERS,
    },
  );
  const overlay = "https://gerardus.example/other/revision/manifest.json";
  assert.equal(mapSources({ gerardus: overlay }).overlay, overlay);
  assert.equal(mapSources({ gerardus: REGIONAL_ROAD_CATALOG }).overlay, "");
  assert.equal(
    mapSources({ gerardus: REGIONAL_ROAD_CATALOG }).catalog,
    REGIONAL_ROAD_CATALOG,
  );
  assert.deepEqual(mapSources({ gerardus: CITY_ROAD_LAYERS }), {
    tiles: "",
    overlay: "",
    catalog: "",
    city: CITY_ROAD_LAYERS,
  });
  assert.deepEqual(mapSources({}), {
    tiles: "",
    overlay: "",
    catalog: "",
    city: "",
  });
  assert.throws(
    () => mapSources({ tiles: "file:///tmp/tiles" }),
    /HTTP or HTTPS/,
  );
});

test("the city drive crosses from a deck to street terrain and rejoins the highway through ordered ramps", () => {
  const section = (id, kind, start, rise) => ({
    id,
    kind,
    width_m: 8,
    rail: kind !== "ground",
    sources: [],
    positions: Array.from({ length: 26 }, (_, i) => [
      start + 2 * i,
      0,
      (kind === "bridge_deck" ? 10 : kind === "ground" || rise > 0 ? 2 : 10) +
        (rise * i) / 25,
    ]),
  });
  const drive = buildCityGameDrive(
    {
      id: BAY_BRIDGE_CITY_DRIVE,
      sections: [
        section("deck", "bridge_deck", 0, 0),
        section("exit", "ramp", 50, -8),
        section("street", "ground", 100, 0),
        section("fourth-street-entrance", "ramp", 150, 8),
        section("highway", "bridge_deck", 200, 0),
      ],
    },
    (x) => 2 + Math.max(0, 1 - Math.abs(x - 125) / 24),
  );
  const network = indexRoadNetwork(drive.topology.transitions);
  network.add(drive.topology);
  const surfaces = new Map(
    drive.surfaces.map((surface) => [surface.id, surface]),
  );
  let car = { ...drive.start, v: 0, z: 10, grounded: true };
  const seen = [car.surfaceId];
  for (let i = 0; i < 7200; i++) {
    const input = cityDriveControls(drive, car);
    assert.ok(input);
    if (input.finished) break;
    const next = driveOnRoadNetwork(
      car,
      input,
      1 / 120,
      surfaces.get(car.surfaceId),
      surfaces,
      network,
      () => false,
    );
    assert.ok(!next.blocked && next.grounded);
    assert.ok(Math.abs(next.z - car.z) < 0.25, "continuous vertical contact");
    if (next.surfaceId !== car.surfaceId) seen.push(next.surfaceId);
    car = next;
  }
  assert.deepEqual(seen, [
    "deck",
    "exit",
    "street",
    "fourth-street-entrance",
    "highway",
  ]);
  assert.equal(car.z, 10);
  assert.ok(cityDriveControls(drive, car).finished);
  assert.ok(Math.abs(drive.start.initialSpeed * MPH - 50) < 1e-6);
  const entrance = surfaces.get("fourth-street-entrance");
  entrance.railSegments = highwayRailSegments(entrance);
  for (const side of [-1, 1]) {
    const x = 175;
    const y = (entrance.highwayRailHalfWidth - CAR_HALF_LENGTH - 0.06) * side;
    const heading = (side * Math.PI) / 2;
    const hard = {
      x,
      y,
      z: entrance.heightAt(x, y),
      v: 24,
      heading,
      grounded: true,
    };
    const hit = strikeHighwayRail(hard, {}, 1 / 120, entrance);
    assert.equal(hit.edgeContact, "bounce");
    assert.equal(hit.roadImpact, undefined);
    assert.equal(hit.grounded, true);
    assert.ok(Math.sin(hit.heading) * hit.v * side < 0);
    assert.ok(Math.abs(hit.v) < hard.v);
  }
  assert.deepEqual(
    drive.surfaces
      .filter((surface) => surface.railBounceOnly)
      .map(({ id }) => id),
    ["fourth-street-entrance"],
  );
});

test("manual pavement can leave the guide, brake to rest, and self-driving rejoins without a reset", () => {
  const ground = () => 2;
  const section = (id, kind, start) => ({
    id,
    kind,
    width_m: 7,
    sources: [],
    positions: Array.from({ length: 26 }, (_, i) => [start + 2 * i, 0, 2]),
  });
  const drive = buildCityGameDrive(
    { sections: [section("street", "ground", 0), section("ramp", "ramp", 50)] },
    ground,
  );
  const network = indexRoadNetwork(drive.topology.transitions);
  network.add(drive.topology);
  const surfaces = new Map(drive.surfaces.map((s) => [s.id, s]));
  const step = (car, input, pavement) =>
    driveOnCityRoute(
      car,
      input,
      1 / 120,
      surfaces.get(car.surfaceId),
      surfaces,
      network,
      pavement,
    );
  let car = {
    x: 12,
    y: 2.5,
    heading: 0.35,
    z: 2,
    v: 7,
    grounded: true,
    surfaceId: "street",
  };
  assert.equal(
    step(car, {}, undefined).blocked,
    true,
    "the old guide contact rejects an otherwise open street",
  );
  for (let i = 0; i < 100; i++) {
    car = step(car, {}, ground);
    assert.equal(car.blocked, false);
  }
  assert.ok(car.y > 4 && car.v > 6);
  for (let i = 0; i < 120; i++) car = step(car, { brake: true }, ground);
  assert.equal(car.v, 0);
  assert.equal(cityDriveOnRoute(surfaces.get(car.surfaceId), car), false);
  assert.ok(cityDriveCanResume(drive, car));
  assert.equal(cityDriveCanResume(drive, { ...car, y: 30 }), false);
  assert.equal(cityDriveCanResume(drive, { ...car, heading: Math.PI }), false);
  const parked = { ...car };
  let joined = false;
  for (let i = 0; i < 480; i++) {
    const next = step(car, cityDriveControls(drive, car), ground);
    assert.ok(!next.blocked && next.grounded && next.z === 2);
    assert.ok(
      Math.hypot(next.x - car.x, next.y - car.y) < 0.3,
      "physical movement only",
    );
    car = next;
    if (cityDriveOnRoute(surfaces.get(car.surfaceId), car)) {
      joined = true;
      break;
    }
  }
  assert.ok(joined && car.v > 0 && car.x > parked.x);
  const backwards = {
    x: 50.2,
    y: 0,
    z: 2,
    v: 0,
    heading: 0,
    grounded: true,
    surfaceId: "ramp",
  };
  let reversing = backwards;
  for (let i = 0; i < 120 && reversing.surfaceId === "ramp"; i++)
    reversing = step(reversing, { throttle: -1 }, ground);
  assert.equal(
    reversing.surfaceId,
    "street",
    "manual reverse traverses the verified seam",
  );
  assert.ok(reversing.x < 50 && reversing.v < 0);
});

test("the city drive adjusts only the specified scenery records in its pinned base", () => {
  const first = [10, [1], 1];
  const second = [20, [1, 2], 1];
  const third = [20, [1, 2, 3], 1];
  const tile = {
    b: [first, second, third],
    tr: [
      [1, 2, 3, 4],
      [5, 6, 7, 8],
    ],
  };
  applyCityDriveScenery(tile, {
    b: { 0: [10, 1, 1], 1: [20, 2, 1], 2: [20, 1, 1] },
    tr: { 0: [1, 2, 3, 4] },
  });
  assert.deepEqual(tile, { b: [third], tr: [[5, 6, 7, 8]] });
});

const stripContact = (z) => ({
  vertices: [
    [0, -3, z],
    [30, -3, z],
    [30, 3, z],
    [0, 3, z],
  ],
  support: ["observed", "observed", "observed", "interpolated"],
  triangles: [
    [0, 1, 2],
    [0, 2, 3],
  ],
});
const strip = (z) => indexRoadContact(stripContact(z));

test("crossing contacts keep their selected height and stop at unsupported edges", () => {
  for (const [id, z] of [
    ["street", 3],
    ["deck", 54],
  ]) {
    const surface = { heightAt: strip(z) };
    for (const heading of [0, Math.PI]) {
      const x = heading === 0 ? 3 : 27;
      let car = { x, y: 0, z, heading, v: 6, surfaceId: id };
      let crossed = false;
      for (let i = 0; i < 1200; i++) {
        const previous = car;
        car = driveOnRoadSurface(car, { throttle: 1 }, 1 / 120, surface);
        assert.equal(car.z, z);
        assert.equal(car.surfaceId, id);
        if (Math.abs(car.x - x) > 15) crossed = true;
        if (car.blocked) {
          assert.equal(car.x, previous.x);
          break;
        }
      }
      assert.ok(crossed && car.blocked);
    }
  }
});

test("regional contact waits for a confirmed owner and crosses only on that mesh", () => {
  const makeSurface = (id, begin, end, height, end_ids, end_tiles) => ({
    id,
    profile: Array.from({ length: end - begin + 1 }, (_, i) => ({
      s_m: i,
      position: [begin + i, 0, height],
      support: "observed",
    })),
    end_ids,
    end_tiles,
    heightAt: (x, y) =>
      x >= begin && x <= end && Math.abs(y) <= 3 ? height : null,
  });
  const a = makeSurface(
    "a",
    0,
    10,
    3,
    ["west", "a-east"],
    ["owner-a", "owner-b"],
  );
  const b = makeSurface(
    "b",
    10,
    20,
    3,
    ["b-west", "east"],
    ["owner-b", "owner-b"],
  );
  const deck = makeSurface(
    "deck",
    0,
    20,
    12,
    ["deck-a", "deck-b"],
    ["owner-a", "owner-a"],
  );
  const surfaces = new Map([
    [a.id, a],
    [deck.id, deck],
  ]);
  const graph = indexRoadNetwork([{ id: "join", status: "joined" }]);
  const step = (car) =>
    driveOnRoadNetwork(
      car,
      { throttle: 1 },
      1 / 120,
      surfaces.get(car.surfaceId),
      surfaces,
      graph,
      (owner) => owner === "owner-b" && !surfaces.has("b"),
    );
  let car = regionalStart(
    { id: "start", start: { station_m: 1, forward: true } },
    a,
  );
  assert.equal(car.x, 2);
  car.v = 4;
  for (let i = 0; i < 1000 && !car.blocked; i++) car = step(car);
  assert.equal(car.waiting, true);
  assert.equal(car.surfaceId, a.id);
  const stop = car.x;
  assert.equal(step(car).x, stop);

  graph.add({
    endpoints: [
      {
        id: "a-east",
        surface_id: "a",
        surface_owner: "owner-a",
        support: "observed",
      },
      {
        id: "b-west",
        surface_id: "b",
        surface_owner: "owner-b",
        support: "observed",
      },
      {
        id: "deck-a",
        surface_id: "deck",
        surface_owner: "owner-a",
        support: "observed",
      },
    ],
    transitions: [
      {
        id: "join",
        from: "a-east",
        to: "b-west",
        direction: "forward",
        support: "observed",
      },
      {
        id: "unresolved",
        from: "a-east",
        to: "deck-a",
        direction: "both",
        support: "observed",
      },
    ],
  });
  surfaces.set(b.id, b);
  for (let i = 0; i < 1200 && car.x < 16; i++) {
    car = step(car);
    assert.equal(car.z, 3);
    assert.equal(car.blocked, false);
  }
  assert.equal(car.surfaceId, b.id);
  assert.ok(car.x >= 16);
  car = { ...car, heading: Math.PI, v: 4 };
  for (let i = 0; i < 1000 && !car.blocked; i++) car = step(car);
  assert.equal(car.blocked, true);
  assert.equal(car.waiting, false);
  assert.equal(car.surfaceId, b.id);
  assert.ok(car.x >= 11.35);
});

test("a grounded car can enter and leave the ramp only at the declared pavement join", () => {
  const terrain = () => 3.2;
  const ramp = {
    surfaceId: "ramp",
    entrance: 0,
    heading: 0,
    along: (car) => car.x,
    heightAt: (_, y) => (Math.abs(y) < 2 ? 3.25 : null),
  };
  const before = { x: -0.05, y: 0, z: 3.2, heading: 0, v: 6, grounded: true };
  const candidate = { ...before, x: 0.015 };
  const joined = crossRampEntrance(before, candidate, ramp, terrain);
  assert.equal(joined.surfaceId, "ramp");
  assert.equal(joined.z, 3.25);
  const backingOut = { ...joined, x: -0.05, v: -6 };
  const returned = crossRampEntrance(joined, backingOut, ramp, terrain);
  assert.equal(returned.surfaceId, undefined);
  assert.equal(returned.z, 3.2);
  for (const change of [
    { y: 1.8 },
    { heading: Math.PI / 2 },
    { grounded: false },
    { surfaceId: "other-level" },
  ]) {
    const car = { ...before, ...change };
    const next = { ...candidate, ...change };
    assert.equal(crossRampEntrance(car, next, ramp, terrain), next);
  }
  assert.equal(
    crossRampEntrance(
      before,
      candidate,
      { ...ramp, heightAt: () => 8 },
      terrain,
    ),
    candidate,
  );
});

test("a highway rail scrubs speed on a scrape, rebounds and crashes on a hard strike", () => {
  const north = { nx: 0, ny: 1 };
  const scrape = barrierImpact({ v: 45, heading: 0.035 }, north, 1 / 120);
  assert.equal(scrape.edgeContact, "scrape");
  assert.ok(scrape.v > 40 && scrape.v < 45);
  assert.ok(Math.abs(Math.sin(scrape.heading)) < 0.001);
  const bounce = barrierImpact({ v: 14, heading: 0.4 }, north, 1 / 120);
  assert.equal(bounce.edgeContact, "bounce");
  assert.ok(bounce.v < 14 && bounce.heading < 0);
  assert.equal(
    barrierImpact({ v: 40, heading: 0.7 }, north, 1 / 120).roadImpact,
    "highway barrier",
  );
  const fasterGraze = barrierImpact({ v: 30, heading: 0.2 }, north, 1 / 120);
  assert.equal(fasterGraze.roadImpact, undefined);
  assert.equal(fasterGraze.edgeContact, "bounce");
  assert.ok(fasterGraze.v > 20);
});

test("the rendered rail stops the body while the wheels still have contact", () => {
  const dt = 1 / 120;
  const surface = {
    heightAt: strip(10),
    profile: Array.from({ length: 31 }, (_, x) => ({
      s_m: x,
      position: [x, 0, 10],
    })),
    highwayRailHalfWidth: 2.6,
  };
  surface.railSegments = highwayRailSegments(surface);
  assert.ok(surface.railSegments.length > 0);
  for (const side of [-1, 1]) {
    const near = {
      x: 15,
      y: side * 0.34,
      z: 10,
      v: 7,
      heading: (side * Math.PI) / 2,
    };
    const input = { throttle: 1 };
    assert.equal(driveOnRoadSurface(near, input, dt, surface).blocked, false);
    const hit = strikeHighwayRail(near, input, dt, surface);
    assert.equal(hit.edgeContact, "bounce");
    assert.ok(Math.sin(hit.heading) * hit.v * side < 0);
    assert.ok(Math.abs(hit.y) < 0.37);
    assert.equal(strikeHighwayRail(hit, input, dt, surface), null);
    const fast = strikeHighwayRail({ ...near, v: 20 }, input, dt, surface);
    assert.equal(fast.roadImpact, "highway barrier");
  }
  assert.equal(
    strikeHighwayRail(
      { x: 2, y: 0.34, z: 10, v: 7, heading: Math.PI / 2 },
      { throttle: 1 },
      dt,
      surface,
    ),
    null,
  );
  let scraping = { x: 8, y: 1.58, z: 10, v: 6, heading: 0.02 };
  const input = { throttle: 0.15, steer: 0.35 };
  let scrapes = 0;
  for (let i = 0; i < 150; i++) {
    scraping =
      strikeHighwayRail(scraping, input, dt, surface) ||
      driveOnRoadSurface(scraping, input, dt, surface);
    if (scraping.edgeContact === "scrape") scrapes++;
    assert.equal(scraping.roadImpact, undefined);
    assert.equal(scraping.blocked, false);
    assert.ok(scraping.y < 1.65);
  }
  assert.ok(scrapes > 2 && scraping.x > 10 && scraping.v > 0);
});

test("a right-only ramp leaves its left edge open and still strikes on the right", () => {
  const dt = 1 / 120;
  const surface = {
    heightAt: strip(10),
    boundary: roadBoundary(stripContact(10)),
    profile: Array.from({ length: 31 }, (_, x) => ({
      s_m: x,
      position: [x, 0, 10],
    })),
    railSides: ["right"],
    highwayRailHalfWidth: 2.6,
  };
  surface.railSegments = highwayRailSegments(surface);
  assert.ok(surface.railSegments.length > 0);
  assert.ok(surface.railSegments.every(({ a, b }) => a[1] < 0 && b[1] < 0));
  const car = { x: 15, y: 0.34, z: 10, v: 7, heading: Math.PI / 2 };
  assert.equal(strikeHighwayRail(car, {}, dt, surface), null);
  const right = { ...car, y: -car.y, heading: -car.heading };
  assert.equal(strikeHighwayRail(right, {}, dt, surface).edgeContact, "bounce");
  const edge = { ...car, y: 2.99, grounded: true };
  assert.equal(leaveRoadSide(edge, {}, dt, surface).leavingRoad, true);
});

test("open sides release contact, but missing contact and road end caps stay blocked", () => {
  const boundary = roadBoundary(stripContact(10));
  const surface = {
    boundary,
    profile: [
      { position: [0, 0, 10], s_m: 0 },
      { position: [30, 0, 10], s_m: 30 },
    ],
    heightAt: (x, y) => (x >= 0 && x <= 30 && Math.abs(y) <= 3 ? 10 : null),
  };
  const car = {
    x: 15,
    y: 2.99,
    z: 10,
    v: 7,
    heading: Math.PI / 2,
    grounded: true,
    surfaceId: "upper",
  };
  const dt = 1 / 120;
  const falling = leaveRoadSide(car, { throttle: 1 }, dt, surface);
  assert.equal(falling.leavingRoad, true);
  assert.equal(falling.z, 10);
  assert.equal(
    leaveRoadSide({ ...car, x: 2 }, {}, dt, surface).leavingRoad,
    true,
    "unrailed side beside a join is open",
  );
  assert.ok(falling.y > 3);
  assert.equal(
    leaveRoadSide(
      { ...car, x: 29, y: 0, heading: 0 },
      { throttle: 1 },
      dt,
      surface,
    ),
    null,
  );
  assert.equal(
    leaveRoadSide({ ...car, y: 0.1 }, { throttle: 1 }, dt, {
      ...surface,
      heightAt: (x, y) => (y > 0.5 ? null : surface.heightAt(x, y)),
    }),
    null,
  );

  let down = falling;
  for (let i = 0; i < 200 && down.leavingRoad; i++)
    down = fallFromRoad(down, dt, () => ({ z: 7, surfaceId: "lower" }));
  assert.equal(down.grounded, true);
  assert.equal(down.roadImpact, null);
  assert.equal(down.z, 7);
  assert.equal(down.surfaceId, "lower");
  assert.ok(down.y > 7);
  const hard = fallFromRoad({ ...falling, z: 3.01, vz: -18 }, dt, () => ({
    z: 3,
  }));
  assert.equal(hard.roadImpact, "ground");
});

test("an indented outside edge releases the car while an enclosed hole stays blocked", () => {
  const xs = [0, 10, 20, 30];
  const ys = [-4, -1, 1, 4];
  const vertices = ys.flatMap((y) => xs.map((x) => [x, y, 10]));
  const support = vertices.map(() => "observed");
  for (const open of [true, false]) {
    const triangles = [];
    for (let y = 0; y < 3; y++)
      for (let x = 0; x < 3; x++) {
        if (x === 1 && y === (open ? 2 : 1)) continue;
        const a = y * 4 + x;
        triangles.push([a, a + 1, a + 4], [a + 1, a + 5, a + 4]);
      }
    const contact = { vertices, support, triangles };
    const surface = {
      boundary: roadBoundary(contact),
      heightAt: indexRoadContact(contact),
      profile: [
        { s_m: 0, position: [0, 0, 10] },
        { s_m: 30, position: [30, 0, 10] },
      ],
    };
    const start = {
      x: 15,
      y: open ? 0.99 : -1.01,
      z: 10,
      v: 6,
      heading: Math.PI / 2,
    };
    const next = leaveRoadSide(start, { throttle: 1 }, 1 / 120, surface);
    if (open) assert.equal(next.leavingRoad, true);
    else assert.equal(next, null);
  }
});

test("a surveyed underpass uses matching street pavement through a hole and ignores the deck", () => {
  const surface = {
    heightAt: (x, y) => (x < 5 && x > 3 && y > 0.3 ? null : stripHeight(x, y)),
  };
  const stripHeight = strip(3.2);
  const street = (x) => (x < 33 ? 3.24 : null);
  let car = {
    x: 3,
    y: 0,
    z: 3.2,
    v: 8,
    heading: 0,
    surfaceId: "street",
    grounded: true,
  };
  assert.equal(
    driveOnRoadSurface(car, { throttle: 1 }, 1 / 120, surface).blocked,
    true,
  );
  assert.equal(
    driveOnStreetContact(car, { throttle: 1 }, 1 / 120, surface, () => 12)
      .blocked,
    true,
  );
  for (let i = 0; i < 600 && car.surfaceId; i++) {
    car = driveOnStreetContact(car, { throttle: 1 }, 1 / 120, surface, street);
    assert.equal(car.blocked, false);
    assert.equal(car.grounded, true);
    assert.ok(car.z >= 3.19 && car.z <= 3.25);
  }
  assert.equal(car.surfaceId, undefined);
  assert.ok(car.x > 30);
});
test("a falling car hits rising terrain without snapping to a higher deck", () => {
  const roadSurfaces = [{ id: "overhead", heightAt: () => 12 }];
  const fall = {
    x: 0,
    y: 0,
    z: 5,
    vx: 16,
    vy: 0,
    vz: -2,
    heading: 0,
    leavingRoad: true,
  };
  const dt = 1 / 120;
  const next = fallFromRoad(fall, dt, (x, y, z) =>
    roadLandingAt(x, y, z, () => 5.4, roadSurfaces),
  );
  assert.equal(next.z, 5.4);
  assert.equal(next.surfaceId, undefined);
  assert.equal(next.grounded, true);
  assert.equal(next.roadImpact, null);
  assert.equal(next.v, 16);
  assert.equal(next.leavingRoad, false);
  const below = [{ id: "lower", heightAt: () => 4.99 }, ...roadSurfaces];
  assert.deepEqual(
    roadLandingAt(0, 0, 5, () => 3, below),
    { z: 4.99, surfaceId: "lower" },
  );
});

test("a moving city car cannot cross into a hole in the baked ground", () => {
  const pavement = (x) => ({ height: x < 10 ? 3 : null, drivable: true });
  for (const grounded of [true, false]) {
    const car = {
      x: 9.95,
      y: 0,
      z: grounded ? 3 : 4,
      heading: 0,
      v: 20,
      vx: 20,
      vy: 0,
      vz: -3,
      grounded,
    };
    const stopped = contactStep(car, { throttle: 1 }, 1 / 120, pavement, 1.35);
    assert.ok(stopped.blocked);
    assert.equal(stopped.x, car.x);
    assert.equal(stopped.z, car.z);
  }
});

test("the city contact step launches near 150 mph and lands back on the crest profile", () => {
  const dt = 1 / 120;
  let car = {
    x: -35,
    y: 0,
    z: soloHeight(-35),
    heading: 0,
    v: 150 / MPH,
    grounded: true,
  };
  car.vz =
    (car.v * (soloHeight(car.x + 1.35) - soloHeight(car.x - 1.35))) / 2.7;
  let peak = 0;
  let ticksInFlight = 0;
  let landed = false;
  for (let i = 0; i < 900; i++) {
    car = contactStep(
      car,
      {},
      dt,
      (x) => ({ height: soloHeight(x), drivable: true }),
      1.35,
    );
    if (!car.grounded) {
      ticksInFlight++;
      peak = Math.max(peak, car.z - soloHeight(car.x));
    } else if (ticksInFlight > 25) {
      assert.equal(car.z, soloHeight(car.x));
      landed = true;
      break;
    }
  }
  assert.ok(
    landed && peak > 1 && ticksInFlight > 45,
    JSON.stringify({ landed, peak, ticksInFlight }),
  );
});

test("airtime follows a curved street and distinguishes terrain beyond its end", () => {
  const road = {
    centreline: [0, 0, 400, 0, 400, 800],
    origin: [0, 0],
    segment: 0,
    fraction: 0.75,
    heading: 0,
  };
  const { profile, basis } = sampleFlightProfile(
    { x: 30, y: 0, heading: 0, v: 20 },
    road,
    (x, y) => x + 2 * y,
  );
  assert.equal(basis, "Road centreline");
  assert.equal(profile.find(({ s }) => s === 0).z, 30);
  assert.equal(profile.find(({ s }) => s === 30).z, 80);
  assert.equal(profile.find(({ s }) => s === 100).z, null);
  assert.equal(profile.find(({ s }) => s === 100).ground, 220);
});

test("airtime uses the adjoining named road through a tile seam", () => {
  const road = {
    centreline: [0, 0, 200, 0],
    origin: [0, 0],
    name: "Hill St",
    segment: 0,
    fraction: 0.9,
    heading: 0,
  };
  const tile = {
    o: [20, 0],
    n: ["Hill St", "Cross St"],
    r: [
      [0, 0, 0, [0, 0, 650, 0]],
      [1, 0, 0, [0, 0, 0, 650]],
    ],
  };
  const { profile } = sampleFlightProfile(
    { x: 18, y: 0, heading: 0, v: 15 },
    road,
    (x, y) => x + y,
    null,
    [tile],
  );
  assert.equal(profile.find(({ s }) => s === 8).z, 26);
});

test("airtime trace remains ordered and completed cards expire independently", () => {
  const timeline = createFlightTimeline();
  const ground = { x: 0, y: 0, z: 2, v: 10, heading: 0, grounded: true };
  const high = { ...ground, x: 4, z: 5, grounded: false };
  const across = { ...high, x: 2, y: 3, z: 4 };
  const land = { ...across, x: 1, z: 2, grounded: true };
  let loaded = false;
  const env = { heightAt: (x) => (loaded || x <= 2 ? 2 : null) };
  timeline.step(ground, high, 0.4, 400, env);
  const active = timeline.visible(400)[0];
  assert.equal(active.profile.find(({ s }) => s === 20).ground, null);
  loaded = true;
  timeline.step(high, across, 0.3, 700, env);
  timeline.step(across, land, 0.1, 800, env);
  assert.equal(active.profile.find(({ s }) => s === 20).ground, 2);
  assert.ok(active.trace.every(({ s }, i, a) => !i || s > a[i - 1].s));
  timeline.step(ground, high, 0.4, 1300, env);
  assert.deepEqual(
    timeline.visible(1300).map(({ label }) => label),
    ["Touchdown", "Airborne"],
  );
  timeline.step(high, land, 0.4, 1700, env);
  timeline.reset(true);
  assert.equal(timeline.visible(2799).length, 2);
  assert.equal(timeline.visible(2800).length, 1);
  assert.equal(timeline.visible(3699).length, 1);
  assert.equal(timeline.visible(3700).length, 0);
});

test("an impact on the landing frame saves the full flight exactly once", () => {
  const records = [];
  const timeline = createFlightTimeline((record) => records.push(record));
  const start = { x: 0, y: 0, z: 0, v: 20, heading: 0, grounded: true };
  const flying = { ...start, x: 8, z: 4, grounded: false };
  const landed = { ...start, x: 18 };
  const env = { heightAt: () => 0 };
  timeline.step(start, flying, 1.4, 1400, env);
  timeline.step(flying, landed, 1.5, 2900, { ...env, impact: "ground" });
  const [impact] = timeline.visible(2900);
  assert.equal(impact.label, "Impact");
  assert.deepEqual(impact.trace.at(-1), { s: 18, z: 0 });
  assert.deepEqual(records, [{ seconds: 2.9, peak: 4, ending: "impact" }]);
  timeline.finish(2900, "Impact");
  assert.equal(records.length, 1);

  timeline.step(start, flying, 0.4, 3400, env);
  timeline.step(flying, landed, 0.5, 3900, env);
  assert.deepEqual(records[1], { seconds: 0.9, peak: 4 });

  timeline.step(start, flying, 1.4, 5400, env);
  timeline.step(flying, { ...flying, z: -31 }, 1.5, 6900, {
    ...env,
    impact: "fall",
  });
  assert.equal(timeline.visible(6900)[0].label, "Out of map");
  assert.equal(
    records.length,
    2,
    "loss of map support does not complete a flight",
  );
});

test("reveal shader uses a car-relative rear plane for trees and walls", () => {
  const reveal = createCarReveal();
  const material = new THREE.MeshLambertMaterial();
  reveal.enableOn(material);
  const shader = {
    uniforms: {},
    vertexShader: "#include <common>\n#include <project_vertex>",
    fragmentShader: "#include <common>\n#include <alphatest_fragment>",
  };
  material.onBeforeCompile(shader);
  const camera = new THREE.PerspectiveCamera(62, 1, 0.1, 2500);
  camera.position.set(15, 28, 4);
  camera.lookAt(1, 0, 0);
  reveal.update(camera, new THREE.Vector3(2, 0, 0), 0, 800, 800, 1);
  const side = (x, y) => {
    const p = new THREE.Vector3(x, y, 0).applyMatrix4(
      camera.matrixWorldInverse,
    );
    return shader.uniforms.carRevealRear.value.dot(
      new THREE.Vector4(...p.toArray(), 1),
    );
  };
  assert.ok(side(4, 15) > 0 && side(0, 15) < 0);
  assert.match(shader.vertexShader, /carRevealPosition = mvPosition.xyz/);
  assert.match(shader.vertexShader, /modelViewMatrix \* instanceMatrix/);
  assert.match(shader.fragmentShader, /dot\(carRevealRear.*<= 0\.0/);
});

test("tier walls keep exposed steps and remove an internally split façade", () => {
  const left = [
    [0, 0, 20],
    [4, 0, 20],
    [4, 4, 20],
    [0, 4, 20],
  ];
  const right = [
    [4, 0, 16],
    [8, 0, 16],
    [8, 4, 16],
    [4, 4, 16],
    [4, 2, 16],
  ];
  const shared = [];
  const outer = [];
  buildingWalls([[left], [right]], 2, (a, b, baseA, baseB) =>
    (a[0] === 4 && b[0] === 4 ? shared : outer).push([
      a[2],
      b[2],
      baseA,
      baseB,
    ]),
  );
  assert.deepEqual(shared, [
    [20, 20, 16, 16],
    [20, 20, 16, 16],
  ]);
  assert.ok(outer.every((wall) => wall[2] === 2 && wall[3] === 2));
  assert.equal(
    validBuildingFace([[0, 0, -10000000000, 1, 0, 5, 2, 2, 5]]),
    false,
  );
});
