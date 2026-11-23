import assert from "node:assert/strict";
import { test } from "node:test";
import { buildBakedRoadContact } from "../sf-baked-road-contact.js";
import { contactStep } from "../sf-drive-core.js";
import { crashRoadSupport } from "../sf-highway-edge.js";
import { tunnelMesh } from "../sf-tunnel-mesh.js";
import { driveOnRoadSurface } from "../sf-road-contact.js";
import { driveThroughRoadPortal, roadPortals } from "../sf-road-portal.js";

const DT = 1 / 120;

function section(interior) {
  return buildBakedRoadContact({
    id: "second-level",
    kind: interior > 0 ? "ramp" : "tunnel",
    width: 6,
    points: [0, 10, 30, 40].map((x) => [
      x,
      0,
      x === 0 || x === 40 ? 0 : interior,
    ]),
    owners: ["808f7fd4", "808f7fcc"],
    groundEnds: ["start", "end"],
  }).surface;
}

function flatCar(x, y, heading, z = 0) {
  return { x, y, z, heading, v: 6, grounded: true };
}

for (const [kind, z] of [
  ["overpass", 3],
  ["tunnel", -3],
]) {
  test(`the ${kind} uses both of its declared portals in either direction`, () => {
    const surface = section(z);
    const portals = roadPortals(surface, ["start", "end"]);
    const pavement = () => 0;
    for (const direction of [1, -1]) {
      let car = flatCar(
        direction === 1 ? -5 : 45,
        0,
        direction === 1 ? 0 : Math.PI,
      );
      const seen = new Set();
      for (let i = 0; i < 1300; i++) {
        const input = { throttle: 0.08 };
        const next =
          driveThroughRoadPortal(car, input, DT, [portals], pavement) ||
          (car.surfaceId
            ? driveOnRoadSurface(car, input, DT, surface)
            : contactStep(car, input, DT, () => ({ height: 0 }), 1.35));
        assert.ok(
          !next.blocked && next.grounded,
          JSON.stringify({ car, next }),
        );
        assert.ok(Math.abs(next.z - car.z) < 0.1);
        car = next;
        seen.add(car.surfaceId);
        if (Math.abs(car.x - 20) < 1) assert.equal(car.z, z);
        if (direction * (car.x - (direction === 1 ? 44 : -4)) > 0) break;
      }
      assert.deepEqual(seen, new Set([undefined, surface.id]));
      assert.equal(car.surfaceId, undefined);
      assert.equal(car.z, 0);
    }
  });
}

test("a sideways, airborne or wrong-level crossing cannot acquire a road portal", () => {
  const surface = section(3);
  const portals = roadPortals(surface, ["start", "end"]);
  const cross = (car, ground = 0) =>
    driveThroughRoadPortal(car, { throttle: 1 }, DT, [portals], () => ground);
  const across = flatCar(20, -0.01, Math.PI / 2);
  assert.equal(cross(across), null);
  assert.equal(cross(flatCar(-0.01, 4, 0)), null);
  assert.equal(cross(flatCar(-0.01, 0, 0, -2), -2), null);
  assert.equal(cross({ ...flatCar(-0.01, 0, 0), grounded: false }), null);
});

test("a portal without terrain support or with two eligible targets stays unselected", () => {
  const surface = section(3);
  const entry = flatCar(-0.01, 0, 0);
  const portals = roadPortals(surface, ["start"]);
  const cross = (points, height) =>
    driveThroughRoadPortal(entry, { throttle: 1 }, DT, [points], () => height);
  assert.equal(cross(portals, null), null);
  assert.equal(cross([...portals, ...portals], 0), null);
});

test("tunnel crash support keeps the selected floor below the exterior terrain", () => {
  const tunnel = { ...section(-3), kind: "tunnel" };
  const all = new Map([[tunnel.id, tunnel]]);
  const crashUnder = crashRoadSupport(() => 8, all, tunnel);
  assert.equal(crashUnder(20, 0, { x: 20, y: 0, z: -2.9 }).z, -3);
  const crashOutside = crashRoadSupport(() => null, all);
  assert.equal(crashOutside(20, 0, { x: 20, y: 0, z: 6 }), null);
});

test("tunnel walls and roof stop at the baked portal stations", () => {
  const tunnel = section(-3);
  const shell = tunnelMesh(tunnel, { interior: [6, 33], clearance: 4.5 });
  const positions = shell.children[0].geometry.getAttribute("position");
  const east = Array.from({ length: positions.count }, (_, i) =>
    positions.getX(i),
  );
  assert.equal(Math.min(...east), 6);
  assert.equal(Math.max(...east), 33);
  assert.ok(shell.children[1].geometry.getAttribute("position").count > 0);
});
