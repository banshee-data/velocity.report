import test from "node:test";
import assert from "node:assert/strict";
import {
  Box3,
  BufferGeometry,
  Color,
  Float32BufferAttribute,
  Group,
  Matrix4,
  Vector3,
} from "../../vendor/three/three.core.js";
import { createMeshQueue } from "../sf-mesh-queue.js";
import { meshSteps } from "../sf-mesh-steps.js";
import { addTrees } from "../sf-tree-layer.js";
import { deckClearanceIndex } from "../sf-tree-clearance.js";

function clock() {
  const frames = [];
  return {
    schedule: (cb) => frames.push(cb),
    tick: () => frames.shift()?.(),
    get pending() {
      return frames.length;
    },
  };
}

const colours = {
  roof: { r: 1, g: 0, b: 0 },
  wall: { r: 0, g: 1, b: 0 },
  extra: { r: 0, g: 0, b: 1 },
};
const tile = {
  o: [0, 0],
  g: { n: 2, s: 4, x0: 0, y0: 0, z: [0, 0, 0, 0] },
  b: [[0, [[[0, 0, 50, 40, 0, 50, 0, 40, 50]]]]],
};
const local = {
  type: "tile",
  tile,
  colours,
  region: [
    [0, 0],
    [4, 0],
    [4, 4],
    [0, 4],
  ],
};
function finish(data) {
  const work = meshSteps(data);
  let step;
  do {
    step = work.next();
  } while (!step.done);
  return step.value;
}

test("tile geometry keeps terrain, roof, walls and outlines with transferable attributes", () => {
  const {
    terrain,
    buildings: [building],
  } = finish(local);
  assert.equal(terrain.positions.length, 18);
  assert.equal(terrain.uv.length, 12);
  assert.equal(building.positions.length, 7 * 9);
  assert.equal(building.positions.length, building.normals.length);
  assert.equal(building.positions.length, building.colours.length);
  assert.ok(building.outlines.length > 0);
  for (const array of [
    terrain.positions,
    terrain.uv,
    terrain.normals,
    ...Object.values(building),
  ]) {
    assert.ok(array instanceof Float32Array && array.every(Number.isFinite));
  }
  assert.deepEqual(
    finish({ ...local, region: null }),
    { terrain: null, buildings: [] },
    "low roofs stay out of the distant skyline",
  );
});

test("one in-flight build, tile priority, latest revision and cancellation across unload/reload", async () => {
  const time = clock(),
    sent = [],
    applied = [];
  const worker = { postMessage: (m) => sent.push(m) };
  const queue = createMeshQueue(worker, time.schedule);
  const record = (id) => ({
    prepare: () => ({ id }),
    apply: (value) => applied.push([id, value]),
  });
  const group = record("original"),
    neighbour = record("neighbour"),
    reload = record("reloaded");
  queue.update(group);
  queue.update(neighbour);
  queue.update(group);
  assert.equal(time.pending, 1);
  time.tick();
  assert.equal(sent.length, 1);
  assert.equal(sent[0].data.id, "neighbour");
  queue.update(neighbour);
  const build = queue.tile(local);
  worker.onmessage({ data: { id: sent[0].id, result: { stale: true } } });
  time.tick();
  assert.deepEqual(applied, []);
  assert.equal(sent[1].data.type, "tile");
  worker.onmessage({ data: { id: sent[1].id, result: { mesh: true } } });
  time.tick();
  assert.deepEqual(await build, { mesh: true });
  assert.equal(sent[2].data.id, "original");
  queue.cancel(group);
  queue.update(reload);
  queue.cancel(neighbour);
  worker.onmessage({ data: { id: sent[2].id, result: { stale: true } } });
  time.tick();
  assert.equal(sent[3].data.id, "reloaded");
  worker.onmessage({ data: { id: sent[3].id, result: { fresh: true } } });
  time.tick();
  assert.deepEqual(applied, [["reloaded", { fresh: true }]]);
});

test("a worker startup error retries the current job in frame slices and drains the queue", async () => {
  const time = clock();
  let elapsed = 0,
    terminated = false;
  const worker = {
    postMessage() {},
    terminate() {
      terminated = true;
    },
  };
  const queue = createMeshQueue(
    worker,
    time.schedule,
    () => (elapsed += 4),
    time.schedule,
  );
  const first = queue.tile(local);
  time.tick();
  worker.onerror({ preventDefault() {} });
  let ticks = 0;
  while (time.pending && ticks++ < 30) time.tick();
  assert.ok(terminated && ticks > 2 && ticks < 30);
  assert.deepEqual(await first, finish(local));
  assert.equal(time.pending, 0);
});

test("static tree cuts deduplicate unchanged roads, stay capped, and survive a failed worker and unload", () => {
  const deck = deckClearanceIndex(),
    shape = new BufferGeometry();
  shape.setAttribute(
    "position",
    new Float32BufferAttribute(
      [-4, 4, -4, 0, 4, -4, 0, 4, 4, -4, 4, -4, 0, 4, 4, -4, 4, 4],
      3,
    ),
  );
  const remove = deck.add(shape),
    scene = new Group(),
    time = clock(),
    sent = [];
  const worker = { postMessage: (m) => sent.push(m), terminate() {} };
  const queue = createMeshQueue(worker, time.schedule, () => 0, time.schedule);
  addTrees(
    scene,
    {
      o: [0, 0],
      tr: [
        [0, 0, 6, 2],
        [200, 0, 6, 2],
      ],
    },
    () => 0,
    { tree: new Color("green"), trunk: new Color("brown") },
    { enableOn() {} },
    deck,
    queue,
  );
  const crown = scene.children.find((m) => m.isInstancedMesh);
  const matrix = new Matrix4();
  crown.getMatrixAt(0, matrix);
  assert.equal(matrix.determinant(), 0);
  crown.getMatrixAt(1, matrix);
  assert.notEqual(matrix.determinant(), 0);
  const bounds = new Box3(new Vector3(-4, 4, -4), new Vector3(0, 4, 4));
  time.tick();
  scene.userData.refreshTreeCuts([bounds]);
  worker.onerror({ preventDefault() {} });
  for (let n = 0; time.pending && n < 30; n++) time.tick();
  const cut = scene.children.find((m) => !m.isInstancedMesh);
  assert.ok(cut?.geometry.attributes.position.count > 0);
  scene.userData.refreshTreeCuts([bounds]);
  assert.equal(time.pending, 0);
  assert.ok(scene.children.includes(cut));
  const broken = { stage: "broken", north: 0, east: 1 };
  scene.userData.drawTree(0, broken, 0.4);
  assert.ok(
    !scene.children.includes(cut),
    "the static mesh yields to the replay pose",
  );
  scene.userData.drawTree(0);
  for (let n = 0; time.pending && n < 30; n++) time.tick();
  const restored = scene.children.find((m) => !m.isInstancedMesh);
  assert.ok(
    restored?.geometry.attributes.position.count > 0,
    "restoring the upright pose queues its capped geometry again",
  );
  remove();
  scene.userData.refreshTreeCuts([bounds]);
  assert.ok(!scene.children.includes(restored));
  crown.getMatrixAt(0, matrix);
  assert.notEqual(matrix.determinant(), 0);
  deck.add(shape);
  scene.userData.refreshTreeCuts([bounds]);
  scene.userData.cancelTreeCuts();
  time.tick();
  assert.equal(time.pending, 0);
});
