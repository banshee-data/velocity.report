import assert from "node:assert/strict";
import { test } from "node:test";
import * as THREE from "../../vendor/three/three.module.js";
import { sampleFlightProfile } from "../sf-airtime.js";
import { buildingWalls, validBuildingFace } from "../sf-building-walls.js";
import { createCarReveal } from "../sf-car-reveal.js";
import { contactStep } from "../sf-drive-core.js";
import { BAY_BRIDGE_OVERLAY, mapSources } from "../sf-drive-settings.js";
import { driveOnRoadSurface, indexRoadContact } from "../sf-road-contact.js";
import { MPH, soloHeight } from "../sf-solo.js";

test("map settings accept a planar tile pack and either form of the Gerardus source", () => {
  assert.deepEqual(
    mapSources({
      tiles: "https://maps.example/sf/tiles/manifest.json",
      gerardus: "http://arrow-worker:50061",
    }),
    { tiles: "https://maps.example/sf/tiles/", overlay: BAY_BRIDGE_OVERLAY },
  );
  const overlay = "https://gerardus.example/other/revision/manifest.json";
  assert.equal(mapSources({ gerardus: overlay }).overlay, overlay);
  assert.deepEqual(mapSources({}), { tiles: "", overlay: "" });
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
