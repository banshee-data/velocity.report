// Tests for the scene export reader. Run with: pnpm test
//
// Fixtures are built in memory and served through a stubbed fetch, so these
// tests never touch the network or a published asset.

import { test, describe, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { gzipSync } from "node:zlib";

import { PartReader, SceneSession, SceneError } from "../scene-reader.js";

/** 200 ms in microseconds: a stride-2 sample of a ~10 Hz rotation. */
const STEP_US = 200_000;
const ABS_START = "1765057342040018330";

/** Builds one part: `chunks` chunks of `perChunk` frames each. */
function makePart({ chunks = 2, perChunk = 4, uneven = false } = {}) {
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
          tr: [
            { id: "0", x: frameNo, y: 0, z: 0.8, spd: 5, hdg: 0, l: 4.4, w: 1.8, h: 1.5, bh: 0, c: "car" },
          ],
        }),
      );
      t1 = us;
      frameNo++;
      // Real rotations are not uniform: 198, 200, 202 ms.
      us += uneven ? STEP_US + (i % 3) * 2_000 - 2_000 : STEP_US;
    }
    files[`chunk_${String(c).padStart(4, "0")}.ndjson.gz`] = gzipSync(lines.join("\n") + "\n");
    index.chunks.push({ c, n: perChunk, t0, t1 });
  }

  return {
    files,
    index,
    header: {
      version: 1,
      export: "tracks",
      sensor_id: "test",
      frame_stride: 2,
      chunk_encoding: "gzip",
      start_ns: ABS_START,
    },
  };
}

function bodyStream(buf) {
  return new ReadableStream({
    start(c) {
      c.enqueue(new Uint8Array(buf));
      c.close();
    },
  });
}

/** Installs a fetch stub serving the given parts, keyed by path fragment. */
function installFetch(parts, { manifest = null, fail = new Set() } = {}) {
  globalThis.fetch = async (url) => {
    const u = String(url);
    if (fail.has(u)) return { ok: false, status: 500, statusText: "Server Error" };
    if (manifest && u.endsWith("manifest.json")) {
      return { ok: true, status: 200, json: async () => manifest };
    }
    for (const [prefix, part] of Object.entries(parts)) {
      if (!u.includes(prefix)) continue;
      if (u.endsWith("header.json")) return { ok: true, status: 200, json: async () => part.header };
      if (u.endsWith("index.json")) return { ok: true, status: 200, json: async () => part.index };
      const m = u.match(/frames\/(chunk_\d+\.ndjson\.gz)$/);
      if (m && part.files[m[1]]) return { ok: true, status: 200, body: bodyStream(part.files[m[1]]) };
    }
    return { ok: false, status: 404, statusText: "Not Found" };
  };
}

const origFetch = globalThis.fetch;
beforeEach(() => {
  globalThis.window = { location: { href: "https://example.test/scenes/demo/" } };
});
afterEach(() => {
  globalThis.fetch = origFetch;
  delete globalThis.window;
});

describe("PartReader", () => {
  test("reads header and index, and derives its span from the index", async () => {
    installFetch({ "/p0/": makePart({ chunks: 3, perChunk: 5 }) });
    const r = await new PartReader("https://example.test/p0/").open();

    assert.equal(r.frameCount, 15);
    assert.equal(r.startUs, 0);
    assert.equal(r.endUs, 14 * STEP_US);
    assert.ok(Math.abs(r.durationSec - 2.8) < 1e-9, `duration ${r.durationSec}`);
  });

  test("keeps absolute capture time as a string, out of Number's range", async () => {
    installFetch({ "/p0/": makePart() });
    const r = await new PartReader("https://example.test/p0/").open();

    assert.equal(r.startNs, ABS_START);
    assert.ok(
      Number(ABS_START) > Number.MAX_SAFE_INTEGER,
      "fixture should exercise the precision problem the string avoids",
    );
  });

  test("rejects an index with no chunks", async () => {
    const part = makePart();
    part.index.chunks = [];
    installFetch({ "/p0/": part });
    await assert.rejects(() => new PartReader("https://example.test/p0/").open(), SceneError);
  });

  test("rejects a malformed chunk entry", async () => {
    const part = makePart();
    part.index.chunks[0] = { c: "nope", n: 4, t0: 1, t1: 2 };
    installFetch({ "/p0/": part });
    await assert.rejects(() => new PartReader("https://example.test/p0/").open(), /malformed/);
  });

  test("rejects a chunk that ends before it starts", async () => {
    const part = makePart();
    part.index.chunks[1].t1 = part.index.chunks[1].t0 - 1;
    installFetch({ "/p0/": part });
    await assert.rejects(
      () => new PartReader("https://example.test/p0/").open(),
      /ends before it starts/,
    );
  });

  test("surfaces a useful error when a chunk fails to load", async () => {
    installFetch(
      { "/p0/": makePart() },
      { fail: new Set(["https://example.test/p0/frames/chunk_0000.ndjson.gz"]) },
    );
    const r = await new PartReader("https://example.test/p0/").open();
    await assert.rejects(() => r.loadChunk(0), /chunk_0000/);
  });

  test("rejects a chunk that is not gzip", async () => {
    const part = makePart();
    part.files["chunk_0000.ndjson.gz"] = Buffer.from("this is not gzip");
    installFetch({ "/p0/": part });
    const r = await new PartReader("https://example.test/p0/").open();
    await assert.rejects(() => r.loadChunk(0), /not valid gzip/);
  });

  test("rejects a frame with no timestamp", async () => {
    const part = makePart();
    part.files["chunk_0000.ndjson.gz"] = gzipSync(JSON.stringify({ f: 0, tr: [] }) + "\n");
    installFetch({ "/p0/": part });
    const r = await new PartReader("https://example.test/p0/").open();
    await assert.rejects(() => r.loadChunk(0), /no timestamp/);
  });

  test("binary-searches the chunk containing an offset", async () => {
    installFetch({ "/p0/": makePart({ chunks: 4, perChunk: 5 }) });
    const r = await new PartReader("https://example.test/p0/").open();

    assert.equal(r.chunkIndexForOffset(0), 0, "start of recording");
    assert.equal(r.chunkIndexForOffset(-1e9), 0, "before the start clamps to first");
    assert.equal(r.chunkIndexForOffset(7 * STEP_US), 1, "inside chunk 1");
    assert.equal(r.chunkIndexForOffset(19 * STEP_US), 3, "end of recording");
    assert.equal(r.chunkIndexForOffset(1e12), 3, "past the end clamps to last");
  });

  test("finds the frame at or before an offset, never ahead of it", async () => {
    installFetch({ "/p0/": makePart({ chunks: 2, perChunk: 5 }) });
    const r = await new PartReader("https://example.test/p0/").open();

    assert.equal((await r.frameAtOffset(3 * STEP_US)).f, 3, "exact hit");
    assert.equal((await r.frameAtOffset(3 * STEP_US + STEP_US / 2)).f, 3, "between frames takes the earlier");
    assert.equal((await r.frameAtOffset(0)).f, 0, "first frame");
    assert.equal((await r.frameAtOffset(9 * STEP_US)).f, 9, "last frame");
  });

  test("handles uneven frame intervals", async () => {
    installFetch({ "/p0/": makePart({ chunks: 2, perChunk: 5, uneven: true }) });
    const r = await new PartReader("https://example.test/p0/").open();

    const frames = await r.loadChunk(0);
    const deltas = frames.slice(1).map((f, i) => f.t - frames[i].t);
    assert.ok(new Set(deltas).size > 1, "fixture should vary the interval");
    for (const d of deltas) assert.ok(d > 0, "timestamps must advance");
  });

  test("caches chunks and evicts beyond the limit", async () => {
    installFetch({ "/p0/": makePart({ chunks: 6, perChunk: 4 }) });
    const inner = globalThis.fetch;
    let chunkFetches = 0;
    globalThis.fetch = async (u) => {
      if (String(u).includes("chunk_")) chunkFetches++;
      return inner(u);
    };

    const r = await new PartReader("https://example.test/p0/").open();
    await r.loadChunk(0);
    await r.loadChunk(0);
    assert.equal(chunkFetches, 1, "a cached chunk should not be refetched");

    for (const c of [1, 2, 3, 4]) await r.loadChunk(c);
    const before = chunkFetches;
    await r.loadChunk(0);
    assert.equal(chunkFetches, before + 1, "chunk 0 should have been evicted");
  });
});

describe("SceneSession", () => {
  const manifest = {
    version: 1,
    site: { id: "demo", title: "Demo Street" },
    parts: [
      { url: "./p0/", start_seconds: 0 },
      { url: "./p1/", start_seconds: 1 },
    ],
  };

  // 5 frames at 200 ms => 0.8 s span each.
  const twoParts = () => ({
    "/p0/": makePart({ chunks: 1, perChunk: 5 }),
    "/p1/": makePart({ chunks: 1, perChunk: 5 }),
  });

  test("composes parts into one timeline using their own durations", async () => {
    installFetch(twoParts(), { manifest });
    const s = await new SceneSession("https://example.test/scenes/demo/manifest.json").open();

    assert.equal(s.parts.length, 2);
    assert.equal(s.title, "Demo Street");
    assert.ok(Math.abs(s.duration - 1.6) < 1e-9, `duration ${s.duration}`);
  });

  test("rejects a manifest with no parts", async () => {
    installFetch(twoParts(), { manifest: { version: 1, site: {}, parts: [] } });
    await assert.rejects(
      () => new SceneSession("https://example.test/scenes/demo/manifest.json").open(),
      /no parts/,
    );
  });

  test("maps scene time across the part boundary", async () => {
    installFetch(twoParts(), { manifest });
    const s = await new SceneSession("https://example.test/scenes/demo/manifest.json").open();

    assert.equal(s.locate(0).partIndex, 0, "start");
    assert.equal(s.locate(0.5).partIndex, 0, "mid part 0");
    assert.equal(s.locate(0.9).partIndex, 1, "past part 0 is part 1");
    assert.equal(s.locate(1.5).partIndex, 1, "late");
  });

  test("clamps seeks outside the timeline", async () => {
    installFetch(twoParts(), { manifest });
    const s = await new SceneSession("https://example.test/scenes/demo/manifest.json").open();

    const before = s.locate(-10);
    assert.equal(before.partIndex, 0);
    assert.equal(before.us, s.parts[0].startUs);

    assert.equal(s.locate(9999).partIndex, 1, "past the end stays in the last part");
  });

  test("frameAt resolves at start, boundary and end", async () => {
    installFetch(twoParts(), { manifest });
    const s = await new SceneSession("https://example.test/scenes/demo/manifest.json").open();

    const start = await s.frameAt(0);
    assert.equal(start.partIndex, 0);
    assert.equal(start.frame.f, 0);

    const boundary = await s.frameAt(0.85);
    assert.equal(boundary.partIndex, 1, "boundary lands in part 1");
    assert.ok(boundary.frame, "boundary frame resolves");

    const end = await s.frameAt(s.duration);
    assert.equal(end.partIndex, 1);
    assert.ok(end.frame, "end frame resolves");
  });
});
