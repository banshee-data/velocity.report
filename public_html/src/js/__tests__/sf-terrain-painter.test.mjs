import test from "node:test";
import assert from "node:assert/strict";
import { createTerrainPainter } from "../sf-terrain-painter.js";

const box = (x0, x1) => [
  [x0, -2],
  [x1, -2],
  [x1, 2],
  [x0, 2],
];
const entry = (x, min = x - 1, max = x + 1) => ({
  tile: { o: [x, 0], half: 5, g: {} },
  region: box(x - 5, x + 5),
  roads: [{ polygon: box(min, max) }],
  markings: [box(min, max)],
});
function setup(useWorker = true) {
  const frames = new Map(),
    messages = [],
    jobs = [],
    paints = [],
    applied = [],
    errors = [];
  let serial = 0;
  const worker = {
    postMessage(message) {
      messages.push(message);
      if (message.type === "build") jobs.push(message);
    },
    terminate() {
      this.terminated = true;
    },
  };
  const painter = createTerrainPainter({
    worker: useWorker ? worker : null,
    paint: (e, sources, fallback) => paints.push({ e, sources, fallback }),
    apply: (e, positions) => applied.push({ e, positions }),
    onError: (e) => errors.push(e),
    requestFrame: (cb) => {
      frames.set(++serial, cb);
      return serial;
    },
    cancelFrame: (id) => frames.delete(id),
  });
  const frame = () => {
    const [id, cb] = frames.entries().next().value;
    frames.delete(id);
    cb();
  };
  const complete = () => {
    const job = jobs.shift();
    worker.onmessage({
      data: { id: job.id, positions: new Float32Array([job.id]) },
    });
    return job;
  };
  const settle = () => {
    for (let i = 0; frames.size || jobs.length; i++) {
      assert.ok(i < 100);
      if (frames.size) frame();
      if (jobs.length) complete();
    }
  };
  return {
    painter,
    worker,
    frames,
    messages,
    jobs,
    paints,
    applied,
    errors,
    frame,
    complete,
    settle,
  };
}

test("burst additions coalesce and texture painting is limited to one tile per frame", () => {
  const s = setup(),
    a = entry(0),
    b = entry(8),
    crossing = entry(4, 3, 5);
  s.painter.add("a", a);
  s.painter.add("b", b);
  s.painter.add("crossing", crossing);
  assert.equal(s.paints.length, 0);
  assert.equal(s.frames.size, 1);
  s.frame();
  assert.equal(s.paints.length, 1);
  assert.equal(s.jobs.length, 1);
  s.frame();
  assert.equal(s.paints.length, 2);
  assert.equal(s.jobs.length, 1, "only one clipping job in flight");
  s.settle();
  assert.equal(s.paints.filter((p) => p.e === a).length, 1);
  assert.equal(s.applied.length, 3);
});

test("loading/removing a crossing source repaints only overlapping dependants", () => {
  const s = setup(),
    a = entry(0),
    b = entry(8),
    far = entry(100),
    crossing = entry(4, 3, 5);
  s.painter.add("a", a);
  s.painter.add("b", b);
  s.painter.add("far", far);
  s.settle();
  s.paints.length = s.applied.length = 0;
  s.painter.add("crossing", crossing);
  s.settle();
  assert.deepEqual(
    new Set(s.paints.map((p) => p.e)),
    new Set([a, b, crossing]),
  );
  assert.deepEqual(
    new Set(s.applied.map((p) => p.e)),
    new Set([a, b, crossing]),
  );
  s.paints.length = s.applied.length = 0;
  s.painter.remove("crossing");
  s.settle();
  assert.deepEqual(new Set(s.paints.map((p) => p.e)), new Set([a, b]));
  assert.ok(s.paints.every((p) => !p.sources.includes(crossing)));
  assert.ok(
    s.messages.some((m) => m.type === "remove" && m.token === "crossing"),
  );
});

test("source bounds do not bypass the existing neighbour-distance limit", () => {
  const s = setup(),
    a = entry(0),
    distant = entry(1000, -1, 1001);
  s.painter.add("a", a);
  s.settle();
  s.paints.length = 0;
  s.painter.add("distant", distant);
  s.settle();
  assert.ok(s.paints.every((p) => p.e !== a));
});

test("an in-flight result is discarded when another source changes its target", () => {
  const s = setup(),
    a = entry(0),
    crossing = entry(4, 3, 5);
  s.painter.add("a", a);
  s.frame();
  s.painter.add("crossing", crossing);
  const stale = s.complete();
  s.frame();
  assert.equal(s.applied.length, 0);
  s.settle();
  assert.equal(s.applied.filter((p) => p.e === a).length, 1);
  assert.ok(s.applied.every((p) => p.positions[0] !== stale.id));
});

test("unloading and reloading the same token cannot apply its old result", () => {
  const s = setup(),
    old = entry(0),
    replacement = entry(0);
  s.painter.add("a", old);
  s.frame();
  s.painter.remove("a");
  s.painter.add("a", replacement);
  s.complete();
  s.frame();
  assert.equal(s.applied.length, 0);
  s.settle();
  assert.deepEqual(
    s.applied.map((p) => p.e),
    [replacement],
  );
});

test("worker failure drops vector results and uses bounded texture fallback", () => {
  const s = setup(),
    a = entry(0);
  s.painter.add("a", a);
  s.frame();
  s.worker.onerror({ message: "worker blocked" });
  assert.equal(s.worker.terminated, true);
  assert.equal(s.errors.length, 1);
  assert.equal(s.applied[0].positions.length, 0);
  s.jobs.length = 0;
  s.settle();
  assert.equal(s.paints.at(-1).fallback, true);
  s.painter.add("b", entry(8));
  s.settle();
  assert.equal(s.paints.at(-1).fallback, true);
  assert.equal(s.jobs.length, 0);
});

test("an unavailable worker still paints roads without synchronous clipping", () => {
  const s = setup(false);
  s.painter.add("a", entry(0));
  s.settle();
  assert.equal(s.paints[0].fallback, true);
  assert.equal(s.applied.length, 0);
});

test("dispose cancels queued work and ignores late worker messages", () => {
  const s = setup();
  s.painter.add("a", entry(0));
  s.frame();
  s.painter.add("b", entry(8));
  s.painter.dispose();
  assert.equal(s.frames.size, 0);
  assert.equal(s.painter.idle, true);
  s.complete();
  assert.equal(s.applied.length, 0);
  assert.equal(s.worker.terminated, true);
});
