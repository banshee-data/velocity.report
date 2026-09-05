// Reader for velocity.report scene exports.
//
// A scene export is a static directory written by `velocity scene export`:
// header.json, index.json, and gzipped NDJSON chunks. This module is the
// transport layer only — it fetches, decompresses, parses and caches. It holds
// no rendering logic and does not import three.js.
//
// Playback timing is never inferred here. A VRLOG frame is one sensor
// rotation, and the rotation rate varies within a capture, so frame timestamps
// are the only clock.
//
// Frame times are microsecond offsets from the part's first frame. The export
// uses offsets because absolute nanosecond capture times (~1.8e18) exceed
// Number.MAX_SAFE_INTEGER and would silently lose ~200 ns of precision as JSON
// numbers. The absolute instant is header.start_ns, carried as a string.

/** Chunks kept in memory: the one being played plus its neighbours. */
const CHUNK_CACHE_LIMIT = 3;

/** Thrown when a scene cannot be loaded or is structurally invalid. */
export class SceneError extends Error {
  constructor(message, cause) {
    super(message);
    this.name = "SceneError";
    this.cause = cause;
  }
}

function assertDecompressionSupport() {
  if (typeof DecompressionStream === "undefined") {
    throw new SceneError(
      "This browser cannot decompress the scene data. " +
        "DecompressionStream is required; it is available in Chrome, Edge, " +
        "Firefox and Safari from 2023 onwards.",
    );
  }
}

async function fetchJSON(url) {
  let res;
  try {
    res = await fetch(url);
  } catch (err) {
    throw new SceneError(`Could not reach ${url}`, err);
  }
  if (!res.ok) {
    throw new SceneError(`${url} returned ${res.status} ${res.statusText}`);
  }
  try {
    return await res.json();
  } catch (err) {
    throw new SceneError(`${url} is not valid JSON`, err);
  }
}

/**
 * Reads one exported part: a single recording's worth of frames.
 */
export class PartReader {
  /** @param {string} baseURL directory holding header.json and index.json */
  constructor(baseURL) {
    this.baseURL = baseURL.endsWith("/") ? baseURL : `${baseURL}/`;
    this.header = null;
    this.chunks = [];
    this._cache = new Map();
    this._inflight = new Map();
  }

  async open() {
    assertDecompressionSupport();

    const [header, index] = await Promise.all([
      fetchJSON(`${this.baseURL}header.json`),
      fetchJSON(`${this.baseURL}index.json`),
    ]);

    if (!Array.isArray(index.chunks) || index.chunks.length === 0) {
      throw new SceneError(`${this.baseURL}index.json lists no chunks`);
    }
    for (const c of index.chunks) {
      if (
        !Number.isInteger(c.c) ||
        !Number.isInteger(c.n) ||
        typeof c.t0 !== "number" ||
        typeof c.t1 !== "number"
      ) {
        throw new SceneError(`${this.baseURL}index.json has a malformed chunk entry`);
      }
      if (c.t1 < c.t0) {
        throw new SceneError(`${this.baseURL}index.json chunk ${c.c} ends before it starts`);
      }
    }

    this.header = header;
    this.chunks = index.chunks;
    return this;
  }

  /** First frame offset in microseconds; zero for a well-formed export. */
  get startUs() {
    return this.chunks[0].t0;
  }

  /** Last frame offset in microseconds. */
  get endUs() {
    return this.chunks[this.chunks.length - 1].t1;
  }

  /** Absolute capture time of the first frame, as a decimal string. */
  get startNs() {
    return this.header?.start_ns ?? "0";
  }

  /** Recording length in seconds. */
  get durationSec() {
    return (this.endUs - this.startUs) / 1e6;
  }

  get frameCount() {
    return this.chunks.reduce((n, c) => n + c.n, 0);
  }

  /**
   * Index of the chunk whose span contains offset `us`, by binary search.
   * Clamps to the first or last chunk when `us` is outside the recording.
   */
  chunkIndexForOffset(us) {
    const chunks = this.chunks;
    if (us <= chunks[0].t0) return 0;
    if (us >= chunks[chunks.length - 1].t1) return chunks.length - 1;

    let lo = 0;
    let hi = chunks.length - 1;
    while (lo <= hi) {
      const mid = (lo + hi) >> 1;
      if (us < chunks[mid].t0) hi = mid - 1;
      else if (us > chunks[mid].t1) lo = mid + 1;
      else return mid;
    }
    // Between two chunks: take the earlier one so playback never jumps ahead.
    return Math.max(0, Math.min(chunks.length - 1, hi));
  }

  /** Fetches, decompresses and parses one chunk. Cached. */
  async loadChunk(id) {
    if (this._cache.has(id)) {
      const frames = this._cache.get(id);
      this._cache.delete(id);
      this._cache.set(id, frames); // refresh LRU position
      return frames;
    }
    if (this._inflight.has(id)) return this._inflight.get(id);

    const name = `chunk_${String(id).padStart(4, "0")}.ndjson.gz`;
    const url = `${this.baseURL}frames/${name}`;

    const task = (async () => {
      let res;
      try {
        res = await fetch(url);
      } catch (err) {
        throw new SceneError(`Could not reach ${name}`, err);
      }
      if (!res.ok) {
        throw new SceneError(`${name} returned ${res.status} ${res.statusText}`);
      }

      let text;
      try {
        const stream = res.body.pipeThrough(new DecompressionStream("gzip"));
        text = await new Response(stream).text();
      } catch (err) {
        throw new SceneError(`${name} is not valid gzip data`, err);
      }

      const frames = [];
      for (const line of text.split("\n")) {
        if (!line) continue;
        let frame;
        try {
          frame = JSON.parse(line);
        } catch (err) {
          throw new SceneError(`${name} contains a malformed frame`, err);
        }
        if (typeof frame.t !== "number") {
          throw new SceneError(`${name} contains a frame with no timestamp`);
        }
        if (!Array.isArray(frame.tr)) frame.tr = [];
        frames.push(frame);
      }
      if (frames.length === 0) {
        throw new SceneError(`${name} contains no frames`);
      }
      return frames;
    })();

    this._inflight.set(id, task);
    try {
      const frames = await task;
      this._cache.set(id, frames);
      while (this._cache.size > CHUNK_CACHE_LIMIT) {
        this._cache.delete(this._cache.keys().next().value);
      }
      return frames;
    } finally {
      this._inflight.delete(id);
    }
  }

  /**
   * The frame at or immediately before offset `us`. Returns null only when the
   * part holds no frames at all.
   */
  async frameAtOffset(us) {
    const chunkIdx = this.chunkIndexForOffset(us);
    const frames = await this.loadChunk(this.chunks[chunkIdx].c);

    let lo = 0;
    let hi = frames.length - 1;
    let best = 0;
    while (lo <= hi) {
      const mid = (lo + hi) >> 1;
      if (frames[mid].t <= us) {
        best = mid;
        lo = mid + 1;
      } else {
        hi = mid - 1;
      }
    }
    return frames[best] ?? null;
  }

  /** Warms the chunk after offset `us` so forward playback does not stall. */
  prefetchAfter(us) {
    const idx = this.chunkIndexForOffset(us);
    const next = this.chunks[idx + 1];
    if (next && !this._cache.has(next.c)) {
      this.loadChunk(next.c).catch(() => {
        /* prefetch is best-effort */
      });
    }
  }
}

/**
 * Composes several exported parts into one logical timeline.
 *
 * Track identifiers are export-local by design, so they are not assumed to
 * survive a part boundary; consumers should reset per-track render state when
 * `partIndex` changes.
 */
export class SceneSession {
  constructor(manifestURL) {
    this.manifestURL = manifestURL;
    this.manifest = null;
    this.parts = [];
    this._offsets = [];
    this.duration = 0;
  }

  async open() {
    const manifest = await fetchJSON(this.manifestURL);
    if (!Array.isArray(manifest.parts) || manifest.parts.length === 0) {
      throw new SceneError("Scene manifest lists no parts");
    }
    this.manifest = manifest;

    const base = new URL(this.manifestURL, window.location.href);
    this.parts = await Promise.all(
      manifest.parts.map((p) => new PartReader(new URL(p.url, base).href).open()),
    );

    // Durations come from each part's own index, so the manifest cannot drift
    // out of step with the data it points at.
    this._offsets = [];
    let acc = 0;
    for (const part of this.parts) {
      this._offsets.push(acc);
      acc += part.durationSec;
    }
    this.duration = acc;
    return this;
  }

  get title() {
    return this.manifest?.site?.title ?? "Scene";
  }

  /** Maps scene time in seconds to a part and a timestamp within it. */
  locate(seconds) {
    const t = Math.max(0, Math.min(seconds, this.duration));
    let idx = this._offsets.length - 1;
    for (let i = 0; i < this._offsets.length; i++) {
      const end = this._offsets[i] + this.parts[i].durationSec;
      if (t < end) {
        idx = i;
        break;
      }
    }
    const part = this.parts[idx];
    const withinSec = t - this._offsets[idx];
    return { partIndex: idx, part, us: part.startUs + withinSec * 1e6 };
  }

  /** The frame to display at `seconds` of scene time. */
  async frameAt(seconds) {
    const { partIndex, part, us } = this.locate(seconds);
    const frame = await part.frameAtOffset(us);
    part.prefetchAfter(us);
    return { partIndex, frame, sceneSeconds: seconds };
  }
}
