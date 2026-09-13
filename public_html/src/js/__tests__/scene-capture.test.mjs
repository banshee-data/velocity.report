// Tests for deterministic capture preparation: URL param parsing, strict
// frame-selector resolution, and view validation. Fixtures are built in
// memory and served through a stubbed fetch, matching scene-reader.test.mjs.

import { test, describe, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { gzipSync } from "node:zlib";

import {
  parseCaptureParams,
  serializeCaptureParams,
  resolveFrameSelector,
  validateView,
  validateCaptureCoordinateFrame,
} from "../scene-capture.js";
import { SceneSession, SceneError } from "../scene-reader.js";

const STEP_US = 200_000;

function makePart({ chunks = 2, perChunk = 5 } = {}) {
  const files = {};
  const index = { version: 1, chunks: [] };
  let us = 0;
  let frameNo = 0;
  for (let c = 0; c < chunks; c++) {
    const lines = [];
    const t0 = us;
    let t1 = us;
    for (let i = 0; i < perChunk; i++) {
      lines.push(
        JSON.stringify({
          f: frameNo,
          t: us,
          tr: [{ id: "0", x: frameNo, y: 0, z: 0.8, h: 1.5, c: "car" }],
        }),
      );
      t1 = us;
      frameNo++;
      us += STEP_US;
    }
    files[`chunk_${String(c).padStart(4, "0")}.ndjson.gz`] = gzipSync(
      lines.join("\n") + "\n",
    );
    index.chunks.push({ c, n: perChunk, t0, t1 });
  }
  return {
    files,
    index,
    header: { version: 1, export: "tracks", sensor_id: "test", start_ns: "0" },
  };
}

function chunkResponse(buf) {
  const bytes = new Uint8Array(buf);
  return {
    ok: true,
    status: 200,
    async arrayBuffer() {
      return bytes.buffer.slice(
        bytes.byteOffset,
        bytes.byteOffset + bytes.byteLength,
      );
    },
  };
}

function installFetch(parts, manifest) {
  globalThis.fetch = async (url) => {
    const u = String(url);
    if (u.endsWith("manifest.json")) {
      return { ok: true, status: 200, json: async () => manifest };
    }
    for (const [prefix, part] of Object.entries(parts)) {
      if (!u.includes(prefix)) continue;
      if (u.endsWith("header.json"))
        return { ok: true, status: 200, json: async () => part.header };
      if (u.endsWith("index.json"))
        return { ok: true, status: 200, json: async () => part.index };
      const m = u.match(/frames\/(chunk_\d+\.ndjson\.gz)$/);
      if (m && part.files[m[1]]) return chunkResponse(part.files[m[1]]);
    }
    return { ok: false, status: 404, statusText: "Not Found" };
  };
}

const origFetch = globalThis.fetch;
beforeEach(() => {
  globalThis.window = {
    location: { href: "https://example.test/scenes/demo/" },
  };
});
afterEach(() => {
  globalThis.fetch = origFetch;
  delete globalThis.window;
});

async function openSession() {
  const manifest = {
    version: 1,
    site: { id: "demo", title: "Demo" },
    parts: [{ url: "./p0/", start_seconds: 0 }],
  };
  installFetch({ "/p0/": makePart({ chunks: 3, perChunk: 5 }) }, manifest);
  return new SceneSession(
    "https://example.test/scenes/demo/manifest.json",
  ).open();
}

describe("parseCaptureParams / serializeCaptureParams", () => {
  test("returns null when no camera is present", () => {
    assert.equal(parseCaptureParams(new URLSearchParams("")), null);
    assert.equal(parseCaptureParams(new URLSearchParams("dev=true")), null);
  });

  test("parses a full view, defaulting layers and trail history", () => {
    const spec = parseCaptureParams(
      new URLSearchParams(
        "cam_x=1&cam_y=2&cam_z=3&tgt_x=0&tgt_y=0&tgt_z=0&fov_deg=45&boxes=true&trails=false&timestamp_us=500000",
      ),
    );
    assert.deepEqual(spec.camera, { x: 1, y: 2, z: 3 });
    assert.deepEqual(spec.target, { x: 0, y: 0, z: 0 });
    assert.equal(spec.fovDeg, 45);
    assert.equal(spec.layers.boxes, true);
    assert.equal(spec.layers.trails, false);
    assert.equal(spec.layers.lidar, false, "lidar defaults off when absent");
    assert.equal(spec.layers.trailHistorySec, 5, "default history window");
    assert.equal(spec.frame.timestampUs, 500000);
    assert.equal(spec.frame.frameIndex, null);
  });

  test("round-trips through serializeCaptureParams and back", () => {
    const spec = {
      camera: { x: 1.5, y: -2, z: 3 },
      target: { x: 0, y: 0, z: 0.8 },
      fovDeg: 60,
      layers: { lidar: true, boxes: true, trails: true, trailHistorySec: 3 },
      frame: { timestampUs: null, frameIndex: 42 },
    };
    const back = parseCaptureParams(serializeCaptureParams(spec));
    assert.deepEqual(back, spec);
  });
});

describe("resolveFrameSelector", () => {
  test("resolves a timestamp to the frame at or before it", async () => {
    const session = await openSession();
    const { partIndex, frame, us } = await resolveFrameSelector(session, {
      timestampUs: 3 * STEP_US,
    });
    assert.equal(partIndex, 0);
    assert.equal(frame.f, 3);
    assert.equal(us, 3 * STEP_US);
    const resolved = await resolveFrameSelector(session, {
      timestampUs: 3 * STEP_US,
    });
    assert.equal(resolved.frameIndex, 3);
  });

  test("resolves a frame index directly", async () => {
    const session = await openSession();
    const { partIndex, frame, us } = await resolveFrameSelector(session, {
      frameIndex: 7,
    });
    assert.equal(partIndex, 0);
    assert.equal(frame.f, 7);
    assert.equal(us, frame.t);
    const resolved = await resolveFrameSelector(session, { frameIndex: 7 });
    assert.equal(resolved.frameIndex, 7);
  });

  test("rejects a timestamp past the end of the scene rather than clamping", async () => {
    const session = await openSession();
    await assert.rejects(
      () =>
        resolveFrameSelector(session, {
          timestampUs: Math.round((session.duration + 10) * 1e6),
        }),
      SceneError,
    );
  });

  test("rejects a negative timestamp", async () => {
    const session = await openSession();
    await assert.rejects(
      () => resolveFrameSelector(session, { timestampUs: -1 }),
      SceneError,
    );
  });

  test("rejects an out-of-range frame index rather than clamping", async () => {
    const session = await openSession();
    await assert.rejects(
      () => resolveFrameSelector(session, { frameIndex: 999 }),
      SceneError,
    );
  });

  test("rejects both selectors given together", async () => {
    const session = await openSession();
    await assert.rejects(
      () => resolveFrameSelector(session, { timestampUs: 0, frameIndex: 0 }),
      /exactly one/,
    );
  });

  test("rejects neither selector given", async () => {
    const session = await openSession();
    await assert.rejects(
      () => resolveFrameSelector(session, {}),
      /exactly one/,
    );
  });
});

describe("validateView", () => {
  const ORIGIN = { x: 0, y: 0, z: 0 };

  test("accepts a well-formed view", () => {
    assert.doesNotThrow(() =>
      validateView({
        camera: { x: 10, y: 0, z: 2 },
        target: ORIGIN,
        fovDeg: 50,
      }),
    );
  });

  test("rejects a non-finite coordinate", () => {
    assert.throws(
      () => validateView({ camera: { x: NaN, y: 0, z: 2 }, target: ORIGIN }),
      /finite/,
    );
  });

  test("rejects a coincident camera and target", () => {
    assert.throws(
      () => validateView({ camera: { ...ORIGIN }, target: ORIGIN }),
      /coincident/,
    );
  });

  test("rejects a camera looking straight down at the target", () => {
    assert.throws(
      () => validateView({ camera: { x: 0, y: 0, z: 10 }, target: ORIGIN }),
      /orientation/,
    );
  });

  test("rejects a field of view outside (0, 180)", () => {
    assert.throws(
      () =>
        validateView({
          camera: { x: 10, y: 0, z: 2 },
          target: ORIGIN,
          fovDeg: 180,
        }),
      /fov_deg/,
    );
    assert.throws(
      () =>
        validateView({
          camera: { x: 10, y: 0, z: 2 },
          target: ORIGIN,
          fovDeg: 0,
        }),
      /fov_deg/,
    );
  });

  test("a missing fov_deg is fine — resolved from the renderer instead", () => {
    assert.doesNotThrow(() =>
      validateView({
        camera: { x: 10, y: 0, z: 2 },
        target: ORIGIN,
        fovDeg: null,
      }),
    );
  });
});

describe("validateCaptureCoordinateFrame", () => {
  test("accepts an export declaring ENU", () => {
    assert.deepEqual(
      validateCaptureCoordinateFrame({
        coordinate_frame: { frame_id: "site/sensor", reference_frame: "ENU" },
      }),
      { frameId: "site/sensor", referenceFrame: "ENU" },
    );
  });

  test("rejects an absent or unsupported coordinate declaration", () => {
    assert.throws(() => validateCaptureCoordinateFrame({}), /reference_frame/);
    assert.throws(
      () =>
        validateCaptureCoordinateFrame({
          coordinate_frame: { reference_frame: "NED" },
        }),
      /reference_frame/,
    );
  });
});
