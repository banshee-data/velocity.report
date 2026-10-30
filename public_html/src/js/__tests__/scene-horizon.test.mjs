import { test } from "node:test";
import assert from "node:assert/strict";
import * as THREE from "three";
import {
  horizonBasis,
  createGroundSurface,
  groundOrbit,
} from "../scene-horizon.js";
import { createSceneCamera } from "../scene-camera.js";
import { applyCapturePose } from "../scene-capture.js";
const upSensor = [0.17, -0.23, Math.sqrt(1 - 0.17 ** 2 - 0.23 ** 2)];
const basis = horizonBasis(upSensor);
const height = (x, z) => 0.12 * x + 0.18 * z;
const sensor = (p) => {
  const q = basis.fromLevel(p);
  return [q[0], -q[2], q[1]];
};
const ground = {
  cell_m: 20,
  positions: [
    [-120, -120],
    [120, -120],
    [120, 120],
    [-120, 120],
  ].map(([x, z]) => sensor([x, height(x, z), z])),
  indices: [0, 1, 2, 0, 2, 3],
};
const element = () => ({
  style: {},
  addEventListener() {},
  removeEventListener() {},
});
const near = (a, b, eps = 1e-8) =>
  assert.ok(Math.abs(a - b) < eps, `${a} != ${b}`);
test("accepted vertical creates an orthonormal horizon and round-trips positions", () => {
  const p = [12, 3, -8];
  const q = basis.fromLevel(basis.toLevel(p));
  p.forEach((v, i) => near(v, q[i]));
  near(new THREE.Vector3(...basis.up).dot(new THREE.Vector3(...basis.east)), 0);
  near(Math.hypot(...basis.up), 1);
  assert.deepEqual(horizonBasis(null).up, [0, 1, 0]);
  assert.deepEqual(horizonBasis([0, 0, 0]).up, [0, 1, 0]);
});
test("true-vertical ground sampling respects slope, triangle boundaries and holes", () => {
  const surface = createGroundSurface(ground, basis);
  near(surface.heightAt(12, -7), height(12, -7));
  assert.equal(surface.heightAt(130, 0), null);
  const sparse = createGroundSurface({ ...ground, indices: [0, 1, 2] }, basis);
  assert.equal(sparse.heightAt(-100, 100), null);
});
test("terrain-following camera keeps clearance and level roll around a complete tilted survey", () => {
  const camera = new THREE.PerspectiveCamera(52, 1, 0.5, 2000);
  const controller = createSceneCamera({ camera, element: element(), THREE });
  controller.setGeometry({ placement: { up_sensor: upSensor }, ground });
  controller.frame({ centerX: 0, centerZ: 0, groundY: -2, span: 100 });
  const polar = 68,
    zoom = 0.25;
  const distance = (100 / 2 / Math.tan((52 * Math.PI) / 360)) * 1.25 * zoom;
  const clearance = distance * Math.cos((polar * Math.PI) / 180);
  const ys = [];
  for (let i = 0; i <= 128; i++) {
    controller.applyVantage({
      azimuth_deg: (i * 360) / 128,
      polar_deg: polar,
      zoom,
    });
    const p = basis.toLevel(camera.position.toArray());
    ys.push(p[1]);
    near(p[1] - height(p[0], p[2]), clearance, 0.025);
    const right = new THREE.Vector3(1, 0, 0).applyQuaternion(camera.quaternion);
    near(right.dot(new THREE.Vector3(...basis.up)), 0);
  }
  assert.ok(
    Math.max(...ys) - Math.min(...ys) > 10,
    "camera must change true elevation on the hill",
  );
  near(ys[0], ys.at(-1));
  controller.dispose();
});
test("the flight profile closes with continuous height and vertical speed, including gaps", () => {
  const profile = groundOrbit(createGroundSurface(ground, basis), {
    radius: 35,
  });
  const eps = 1e-4;
  near(profile.heightAt(0), profile.heightAt(2 * Math.PI));
  near(
    (profile.heightAt(eps) - profile.heightAt(0)) / eps,
    (profile.heightAt(0) - profile.heightAt(-eps)) / eps,
    0.002,
  );
  const sparse = groundOrbit(
    { heightAt: (x, z) => (z < 0 ? null : x * 0.2) },
    { radius: 20 },
  );
  for (let i = 0; i < 360; i++)
    assert.ok(Number.isFinite(sparse.heightAt((i * Math.PI) / 180)));
  const absent = groundOrbit(null, { radius: 20, fallbackHeight: 7 });
  near(absent.heightAt(1), 7);
});
test("capture poses keep their sensor coordinates and use the same true horizon", () => {
  const camera = new THREE.PerspectiveCamera();
  applyCapturePose(
    camera,
    { x: 10, y: -30, z: 15 },
    { x: 0, y: 0, z: 0 },
    52,
    upSensor,
  );
  assert.deepEqual(camera.position.toArray(), [10, 15, 30]);
  near(
    new THREE.Vector3(1, 0, 0)
      .applyQuaternion(camera.quaternion)
      .dot(new THREE.Vector3(...basis.up)),
    0,
  );
});

test("flight resumption can retain the exact camera pose rather than rounded editor values", () => {
  const camera = new THREE.PerspectiveCamera(52, 1, 0.5, 2000);
  const controller = createSceneCamera({ camera, element: element(), THREE });
  controller.setGeometry({ placement: { up_sensor: upSensor }, ground });
  controller.frame({ centerX: 0, centerZ: 0, groundY: -2, span: 100 });
  controller.applyVantage({
    azimuth_deg: 31.23456,
    polar_deg: 68.12345,
    zoom: 0.254321,
    offset_x: 1.2345,
    offset_y: -3.4567,
  });
  const before = camera.position.clone();
  const exact = controller.currentVantage({ precise: true });
  assert.notEqual(exact.zoom, controller.currentVantage().zoom);
  controller.applyVantage(exact);
  near(camera.position.distanceTo(before), 0);
  controller.dispose();
});
