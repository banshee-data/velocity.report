import assert from "node:assert/strict";
import { test } from "node:test";
import * as THREE from "../../vendor/three/three.module.js";
import { sampleFlightProfile } from "../sf-airtime.js";
import { buildingWalls, validBuildingFace } from "../sf-building-walls.js";
import { createCarReveal } from "../sf-car-reveal.js";
import { contactStep } from "../sf-drive-core.js";
import {
  BAY_BRIDGE_OVERLAY,
  REGIONAL_ROAD_CATALOG,
  mapSources,
} from "../sf-drive-settings.js";
import { driveOnRoadSurface, indexRoadContact } from "../sf-road-contact.js";
import {
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
} from "../sf-highway-edge.js";

test("map settings accept a planar tile pack and either form of the Gerardus source", () => {
  assert.deepEqual(
    mapSources({
      tiles: "https://maps.example/sf/tiles/manifest.json",
      gerardus: "http://arrow-worker:50061",
    }),
    {
      tiles: "https://maps.example/sf/tiles/",
      overlay: BAY_BRIDGE_OVERLAY,
      catalog: REGIONAL_ROAD_CATALOG,
    },
  );
  const overlay = "https://gerardus.example/other/revision/manifest.json";
  assert.equal(mapSources({ gerardus: overlay }).overlay, overlay);
  assert.equal(mapSources({ gerardus: REGIONAL_ROAD_CATALOG }).overlay, "");
  assert.equal(
    mapSources({ gerardus: REGIONAL_ROAD_CATALOG }).catalog,
    REGIONAL_ROAD_CATALOG,
  );
  assert.deepEqual(mapSources({}), { tiles: "", overlay: "", catalog: "" });
  assert.throws(
    () => mapSources({ tiles: "file:///tmp/tiles" }),
    /HTTP or HTTPS/,
  );
});

const strip = (z) =>
  indexRoadContact({
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
  assert.equal(
    barrierImpact({ v: 30, heading: 0.2 }, north, 1 / 120).roadImpact,
    "highway barrier",
  );
});

test("open sides release contact, but missing contact and road end caps stay blocked", () => {
  const boundary = roadBoundary([
    [0, -3, 10],
    [30, -3, 10],
    [30, 3, 10],
    [0, 3, 10],
  ]);
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

  const railSegments = highwayRailSegments({ ...surface, highwayRails: true });
  assert.equal(railSegments.length, 2);
  assert.equal(
    leaveRoadSide(car, { throttle: 1 }, dt, { ...surface, railSegments })
      .edgeContact,
    "bounce",
  );
  let down = falling;
  for (let i = 0; i < 200 && down.leavingRoad; i++)
    down = fallFromRoad(down, dt, () => ({ z: 7, surfaceId: "lower" }));
  assert.equal(down.grounded, true);
  assert.equal(down.roadImpact, null);
  assert.equal(down.z, 7);
  assert.equal(down.surfaceId, "lower");
  assert.ok(down.y > 7);
  const hard = fallFromRoad({ ...falling, z: 3.01, vz: -14 }, dt, () => ({
    z: 3,
  }));
  assert.equal(hard.roadImpact, "ground");
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

test("airtime profile follows a curved street through its bend and leaves unknown ends blank", () => {
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
