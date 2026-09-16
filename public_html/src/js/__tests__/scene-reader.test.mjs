// Tests for the scene export reader. Run with: pnpm test
//
// Fixtures are built in memory and served through a stubbed fetch, so these
// tests never touch the network or a published asset.

import { test, describe, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { gzipSync, gunzipSync } from "node:zlib";

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
            {
              id: "0",
              x: frameNo,
              y: 0,
              z: 0.8,
              spd: 5,
              hdg: 0,
              l: 4.4,
              w: 1.8,
              h: 1.5,
              bh: 0,
              c: "car",
            },
          ],
        }),
      );
      t1 = us;
      frameNo++;
      // Real rotations are not uniform: 198, 200, 202 ms.
      us += uneven ? STEP_US + (i % 3) * 2_000 - 2_000 : STEP_US;
    }
    files[`chunk_${String(c).padStart(4, "0")}.ndjson.gz`] = gzipSync(
      lines.join("\n") + "\n",
    );
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

/** A minimal Response stand-in exposing what the reader actually uses. */
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
    get body() {
      return new ReadableStream({
        start(c) {
          c.enqueue(bytes);
          c.close();
        },
      });
    },
  };
}

/** Installs a fetch stub serving the given parts, keyed by path fragment. */
function installFetch(parts, { manifest = null, fail = new Set() } = {}) {
  globalThis.fetch = async (url) => {
    const u = String(url);
    if (fail.has(u))
      return { ok: false, status: 500, statusText: "Server Error" };
    if (manifest && u.endsWith("manifest.json")) {
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

describe("PartReader", () => {
  test("reads header and index, and derives its span from the index", async () => {
    installFetch({ "/p0/": makePart({ chunks: 3, perChunk: 5 }) });
    const r = await new PartReader("https://example.test/p0/").open();

    assert.equal(r.frameCount, 15);
    assert.equal(r.startUs, 0);
    assert.equal(r.endUs, 14 * STEP_US);
    assert.ok(
      Math.abs(r.durationSec - 2.8) < 1e-9,
      `duration ${r.durationSec}`,
    );
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
    await assert.rejects(
      () => new PartReader("https://example.test/p0/").open(),
      SceneError,
    );
  });

  test("rejects a malformed chunk entry", async () => {
    const part = makePart();
    part.index.chunks[0] = { c: "nope", n: 4, t0: 1, t1: 2 };
    installFetch({ "/p0/": part });
    await assert.rejects(
      () => new PartReader("https://example.test/p0/").open(),
      /malformed/,
    );
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
      {
        fail: new Set(["https://example.test/p0/frames/chunk_0000.ndjson.gz"]),
      },
    );
    const r = await new PartReader("https://example.test/p0/").open();
    await assert.rejects(() => r.loadChunk(0), /chunk_0000/);
  });

  test("rejects a chunk that is neither gzip nor NDJSON", async () => {
    // Content without the gzip magic number is treated as already-decompressed
    // NDJSON, so garbage now fails at parsing rather than at decompression.
    const part = makePart();
    part.files["chunk_0000.ndjson.gz"] = Buffer.from("this is not a frame");
    installFetch({ "/p0/": part });
    const r = await new PartReader("https://example.test/p0/").open();
    await assert.rejects(() => r.loadChunk(0), /malformed frame/);
  });

  test("rejects a chunk with the gzip magic number but corrupt body", async () => {
    const part = makePart();
    const bad = Buffer.from(gzipSync("{}"));
    bad[10] = 0x00;
    bad[11] = 0xff;
    part.files["chunk_0000.ndjson.gz"] = bad;
    installFetch({ "/p0/": part });
    const r = await new PartReader("https://example.test/p0/").open();
    await assert.rejects(() => r.loadChunk(0), /could not be decompressed/);
  });

  test("rejects a frame with no timestamp", async () => {
    const part = makePart();
    part.files["chunk_0000.ndjson.gz"] = gzipSync(
      JSON.stringify({ f: 0, tr: [] }) + "\n",
    );
    installFetch({ "/p0/": part });
    const r = await new PartReader("https://example.test/p0/").open();
    await assert.rejects(() => r.loadChunk(0), /no timestamp/);
  });

  test("binary-searches the chunk containing an offset", async () => {
    installFetch({ "/p0/": makePart({ chunks: 4, perChunk: 5 }) });
    const r = await new PartReader("https://example.test/p0/").open();

    assert.equal(r.chunkIndexForOffset(0), 0, "start of recording");
    assert.equal(
      r.chunkIndexForOffset(-1e9),
      0,
      "before the start clamps to first",
    );
    assert.equal(r.chunkIndexForOffset(7 * STEP_US), 1, "inside chunk 1");
    assert.equal(r.chunkIndexForOffset(19 * STEP_US), 3, "end of recording");
    assert.equal(r.chunkIndexForOffset(1e12), 3, "past the end clamps to last");
  });

  test("finds the frame at or before an offset, never ahead of it", async () => {
    installFetch({ "/p0/": makePart({ chunks: 2, perChunk: 5 }) });
    const r = await new PartReader("https://example.test/p0/").open();

    assert.equal((await r.frameAtOffset(3 * STEP_US)).f, 3, "exact hit");
    assert.equal(
      (await r.frameAtOffset(3 * STEP_US + STEP_US / 2)).f,
      3,
      "between frames takes the earlier",
    );
    assert.equal((await r.frameAtOffset(0)).f, 0, "first frame");
    assert.equal((await r.frameAtOffset(9 * STEP_US)).f, 9, "last frame");
  });

  test("handles uneven frame intervals", async () => {
    installFetch({
      "/p0/": makePart({ chunks: 2, perChunk: 5, uneven: true }),
    });
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

  test("frameAtIndex returns frames by ordinal position, strict at the edges", async () => {
    installFetch({ "/p0/": makePart({ chunks: 3, perChunk: 5 }) });
    const r = await new PartReader("https://example.test/p0/").open();

    assert.equal((await r.frameAtIndex(0)).f, 0, "first frame");
    assert.equal((await r.frameAtIndex(14)).f, 14, "last frame");
    assert.equal((await r.frameAtIndex(7)).f, 7, "spans into a later chunk");

    await assert.rejects(() => r.frameAtIndex(15), SceneError, "past the end");
    await assert.rejects(() => r.frameAtIndex(-1), SceneError, "negative");
    await assert.rejects(() => r.frameAtIndex(1.5), SceneError, "non-integer");
  });

  test("framesInRange collects a window, spanning chunks as needed", async () => {
    installFetch({ "/p0/": makePart({ chunks: 3, perChunk: 5 }) });
    const r = await new PartReader("https://example.test/p0/").open();

    const inOneChunk = await r.framesInRange(1 * STEP_US, 3 * STEP_US);
    assert.deepEqual(
      inOneChunk.map((f) => f.f),
      [1, 2, 3],
    );

    // Chunk 0 holds frames 0-4, chunk 1 holds frames 5-9: this window crosses
    // the boundary between them.
    const spanning = await r.framesInRange(3 * STEP_US, 6 * STEP_US);
    assert.deepEqual(
      spanning.map((f) => f.f),
      [3, 4, 5, 6],
    );
  });

  test("framesInRange truncates at the part's own start rather than padding", async () => {
    installFetch({ "/p0/": makePart({ chunks: 2, perChunk: 5 }) });
    const r = await new PartReader("https://example.test/p0/").open();

    const frames = await r.framesInRange(-10 * STEP_US, 2 * STEP_US);
    assert.deepEqual(
      frames.map((f) => f.f),
      [0, 1, 2],
      "clamped to the part start, not padded with invented samples",
    );
  });

  test("framesInRange clamps at the part's own end", async () => {
    installFetch({ "/p0/": makePart({ chunks: 2, perChunk: 5 }) });
    const r = await new PartReader("https://example.test/p0/").open();

    const frames = await r.framesInRange(8 * STEP_US, 100 * STEP_US);
    assert.deepEqual(
      frames.map((f) => f.f),
      [8, 9],
    );
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
    const s = await new SceneSession(
      "https://example.test/scenes/demo/manifest.json",
    ).open();

    assert.equal(s.parts.length, 2);
    assert.equal(s.title, "Demo Street");
    assert.ok(Math.abs(s.duration - 1.6) < 1e-9, `duration ${s.duration}`);
  });

  test("rejects a manifest with no parts", async () => {
    installFetch(twoParts(), { manifest: { version: 1, site: {}, parts: [] } });
    await assert.rejects(
      () =>
        new SceneSession(
          "https://example.test/scenes/demo/manifest.json",
        ).open(),
      /no parts/,
    );
  });

  test("maps scene time across the part boundary", async () => {
    installFetch(twoParts(), { manifest });
    const s = await new SceneSession(
      "https://example.test/scenes/demo/manifest.json",
    ).open();

    assert.equal(s.locate(0).partIndex, 0, "start");
    assert.equal(s.locate(0.5).partIndex, 0, "mid part 0");
    assert.equal(s.locate(0.9).partIndex, 1, "past part 0 is part 1");
    assert.equal(s.locate(1.5).partIndex, 1, "late");
  });

  test("clamps seeks outside the timeline", async () => {
    installFetch(twoParts(), { manifest });
    const s = await new SceneSession(
      "https://example.test/scenes/demo/manifest.json",
    ).open();

    const before = s.locate(-10);
    assert.equal(before.partIndex, 0);
    assert.equal(before.us, s.parts[0].startUs);

    assert.equal(
      s.locate(9999).partIndex,
      1,
      "past the end stays in the last part",
    );
  });

  test("frameAt resolves at start, boundary and end", async () => {
    installFetch(twoParts(), { manifest });
    const s = await new SceneSession(
      "https://example.test/scenes/demo/manifest.json",
    ).open();

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

  test("frameAtIndex walks parts by their own frame count", async () => {
    installFetch(twoParts(), { manifest });
    const s = await new SceneSession(
      "https://example.test/scenes/demo/manifest.json",
    ).open();

    const first = await s.frameAtIndex(0);
    assert.equal(first.partIndex, 0);
    assert.equal(first.frame.f, 0);

    const lastOfPart0 = await s.frameAtIndex(4);
    assert.equal(lastOfPart0.partIndex, 0);
    assert.equal(lastOfPart0.frame.f, 4, "last of 5 frames in part 0");

    const firstOfPart1 = await s.frameAtIndex(5);
    assert.equal(
      firstOfPart1.partIndex,
      1,
      "index carries past the part boundary",
    );
    assert.equal(
      firstOfPart1.frame.f,
      0,
      "frame numbering is export-local per part",
    );

    await assert.rejects(
      () => s.frameAtIndex(10),
      SceneError,
      "past both parts",
    );
    await assert.rejects(() => s.frameAtIndex(-1), SceneError, "negative");
  });

  test("trailHistory does not reach into a different part even when the window would", async () => {
    installFetch(twoParts(), { manifest });
    const s = await new SceneSession(
      "https://example.test/scenes/demo/manifest.json",
    ).open();

    // 0.9s of scene time is 0.1s into part 1 (part 0 is 0.8s long). A 5s
    // lookback would nominally reach 4.1s before the scene even starts.
    const history = await s.trailHistory(0.9, 5);
    assert.equal(history.partIndex, 1);
    // Each part's own PartReader only ever reads its own chunks, so a bound
    // clamped to this part's start is what rules out part 0 entirely — its
    // frames live in a different PartReader instance, never consulted here.
    assert.equal(
      history.fromUs,
      0,
      "clamped to part 1's own start, not part 0's tail",
    );
  });

  // This is the literal acceptance test for "direct seek and sequential
  // playback produce identical trail samples at the same frame": trail
  // history is a pure function of (seconds, historySeconds), so reaching a
  // point directly must agree with reaching it through a sequence of earlier
  // calls, regardless of what those earlier calls cached along the way.
  test("trailHistory agrees whether reached by a direct seek or by sequential playback", async () => {
    const freshManifest = {
      version: 1,
      site: { id: "demo", title: "Demo Street" },
      parts: [{ url: "./p0/", start_seconds: 0 }],
    };
    const onePart = () => ({ "/p0/": makePart({ chunks: 3, perChunk: 5 }) });

    installFetch(onePart(), { manifest: freshManifest });
    const direct = await new SceneSession(
      "https://example.test/scenes/demo/manifest.json",
    ).open();
    const directHistory = await direct.trailHistory(2.4, 1);

    installFetch(onePart(), { manifest: freshManifest });
    const sequential = await new SceneSession(
      "https://example.test/scenes/demo/manifest.json",
    ).open();
    await sequential.trailHistory(0.6, 1);
    await sequential.trailHistory(1.2, 1);
    await sequential.trailHistory(1.8, 1);
    const sequentialHistory = await sequential.trailHistory(2.4, 1);

    assert.deepEqual(
      directHistory.frames.map((f) => f.f),
      sequentialHistory.frames.map((f) => f.f),
      "same frames regardless of path taken to get here",
    );
    assert.equal(directHistory.fromUs, sequentialHistory.fromUs);
    assert.equal(directHistory.toUs, sequentialHistory.toUs);
    assert.equal(directHistory.partIndex, sequentialHistory.partIndex);
  });
});

describe("chunk transport", () => {
  // A .gz asset may arrive already decompressed, because some hosts label it
  // with Content-Encoding: gzip and the browser expands it before JS runs.
  // The same file must work either way.
  test("reads a chunk the host already decompressed", async () => {
    const part = makePart({ chunks: 1, perChunk: 4 });
    const plain = Buffer.from(gunzipSync(part.files["chunk_0000.ndjson.gz"]));
    part.files["chunk_0000.ndjson.gz"] = plain;
    installFetch({ "/p0/": part });

    const r = await new PartReader("https://example.test/p0/").open();
    const frames = await r.loadChunk(0);
    assert.equal(
      frames.length,
      4,
      "plain NDJSON should parse without decompression",
    );
    assert.equal(frames[0].f, 0);
  });

  test("reads a chunk the host served as opaque gzip", async () => {
    installFetch({ "/p0/": makePart({ chunks: 1, perChunk: 4 }) });
    const r = await new PartReader("https://example.test/p0/").open();
    const frames = await r.loadChunk(0);
    assert.equal(frames.length, 4);
    assert.equal(frames[0].f, 0);
  });
});

describe("a session over parts that are already open", () => {
  // The operator tools read live runs out of the database rather than static
  // chunks, and drive the same player with them. fromParts is the seam: it
  // has to give those parts the same seeking and trail reconstruction a
  // published scene gets, or the two surfaces diverge.

  /** A part whose one chunk is already in memory. */
  class MemoryPart extends PartReader {
    constructor(frames, header = {}) {
      super("memory:///");
      this.frames = frames;
      this.header = header;
      this.chunks = [
        {
          c: 0,
          n: frames.length,
          t0: frames[0].t,
          t1: frames[frames.length - 1].t,
        },
      ];
    }
    async loadChunk(id) {
      return id === 0 ? this.frames : [];
    }
    prefetchAfter() {}
  }

  function memoryFrames(count, stepUs = STEP_US) {
    return Array.from({ length: count }, (_, i) => ({
      f: i,
      t: i * stepUs,
      tr: [{ id: "a", x: i, y: 0, z: 0, spd: i }],
    }));
  }

  test("takes its duration from the parts themselves", () => {
    const session = SceneSession.fromParts([
      new MemoryPart(memoryFrames(5)),
      new MemoryPart(memoryFrames(3)),
    ]);
    // 4 steps then 2 steps, in seconds. Summed per part, so the comparison
    // allows for the float accumulation that summing does.
    assert.ok(
      Math.abs(session.duration - (4 * STEP_US + 2 * STEP_US) / 1e6) < 1e-9,
      `duration was ${session.duration}`,
    );
  });

  test("locates a time in the right part", () => {
    const session = SceneSession.fromParts([
      new MemoryPart(memoryFrames(5)),
      new MemoryPart(memoryFrames(3)),
    ]);
    assert.equal(session.locate(0).partIndex, 0);
    // Past the first part's 0.8 s span.
    assert.equal(session.locate(0.9).partIndex, 1);
  });

  test("seeks and reconstructs a trail through the shared logic", async () => {
    const session = SceneSession.fromParts([new MemoryPart(memoryFrames(6))]);

    const at = await session.frameAt(0.5);
    assert.equal(at.frame.f, 2, "0.5 s falls on the third 200 ms frame");

    const history = await session.trailHistory(1.0, 0.45);
    assert.deepEqual(
      history.frames.map((f) => f.f),
      [3, 4, 5],
      "history should come from recorded samples, not rendered ones",
    );
  });

  test("titles the session for the caller", () => {
    const named = SceneSession.fromParts([new MemoryPart(memoryFrames(2))], {
      title: "Run 42",
    });
    assert.equal(named.title, "Run 42");
    // A caller that does not care still gets something printable.
    assert.equal(
      SceneSession.fromParts([new MemoryPart(memoryFrames(2))]).title,
      "Scene",
    );
  });

  test("refuses a session with no parts", () => {
    // An empty session would report a zero duration and then fail on the
    // first seek, which is a worse error than this one.
    assert.throws(() => SceneSession.fromParts([]), SceneError);
    assert.throws(() => SceneSession.fromParts(null), SceneError);
  });
});
