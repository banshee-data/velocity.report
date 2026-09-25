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
        throw new SceneError(
          `${this.baseURL}index.json has a malformed chunk entry`,
        );
      }
      if (c.t1 < c.t0) {
        throw new SceneError(
          `${this.baseURL}index.json chunk ${c.c} ends before it starts`,
        );
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
        throw new SceneError(
          `${name} returned ${res.status} ${res.statusText}`,
        );
      }

      // Some static hosts and CDNs label a .gz file with
      // Content-Encoding: gzip, which makes the browser decompress it before
      // JS sees the body. Others serve it as opaque bytes. Sniff the gzip
      // magic number rather than trusting either, so the same asset works on
      // both without a server-side content-negotiation dance.
      let text;
      try {
        const raw = new Uint8Array(await res.arrayBuffer());
        const isGzip = raw.length > 1 && raw[0] === 0x1f && raw[1] === 0x8b;
        if (isGzip) {
          const stream = new Blob([raw])
            .stream()
            .pipeThrough(new DecompressionStream("gzip"));
          text = await new Response(stream).text();
        } else {
          text = new TextDecoder().decode(raw);
        }
      } catch (err) {
        throw new SceneError(`${name} could not be decompressed`, err);
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
        if (!Array.isArray(frame.p)) frame.p = [];
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
   * The frame at ordinal position `n` within this part (0-based, export
   * order). Unlike frameAtOffset, this is strict: an out-of-range index
   * throws rather than clamping, because a capture recipe naming frame 500 of
   * a 400-frame part is a mistake to surface, not a request to round down.
   */
  async frameAtIndex(n) {
    if (!Number.isInteger(n) || n < 0 || n >= this.frameCount) {
      throw new SceneError(
        `Frame index ${n} is out of range for this part (0-${this.frameCount - 1})`,
      );
    }
    let remaining = n;
    for (const chunk of this.chunks) {
      if (remaining < chunk.n) {
        const frames = await this.loadChunk(chunk.c);
        return frames[remaining];
      }
      remaining -= chunk.n;
    }
    // frameCount is the sum of chunk.n, so this is unreachable given the
    // bounds check above; kept only so a future accounting bug fails loudly
    // instead of returning undefined.
    throw new SceneError(`Frame index ${n} could not be located`);
  }

  /**
   * Every frame with `t` in [fromUs, toUs], clamped to this part's own
   * bounds. Used to reconstruct trail history from recorded samples rather
   * than from whichever frames the browser happened to render, so a direct
   * seek and sequential playback arrive at the same trail.
   */
  async framesInRange(fromUs, toUs) {
    const from = Math.max(fromUs, this.startUs);
    const to = Math.min(toUs, this.endUs);
    if (to < from) return [];
    const firstChunk = this.chunkIndexForOffset(from);
    const lastChunk = this.chunkIndexForOffset(to);
    const frames = [];
    for (let i = firstChunk; i <= lastChunk; i++) {
      const chunkFrames = await this.loadChunk(this.chunks[i].c);
      for (const f of chunkFrames) {
        if (f.t >= from && f.t <= to) frames.push(f);
      }
    }
    return frames;
  }

  /** The frame and export-local index at or immediately before `us`. */
  async frameAtOffsetWithIndex(us) {
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
    const frame = frames[best] ?? null;
    if (!frame) return null;
    const frameIndex =
      this.chunks
        .slice(0, chunkIdx)
        .reduce((total, chunk) => total + chunk.n, 0) + best;
    return { frame, frameIndex };
  }

  /** The frame at or immediately before offset `us`. */
  async frameAtOffset(us) {
    return (await this.frameAtOffsetWithIndex(us))?.frame ?? null;
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

  /**
   * A session over parts that are already open, for a caller whose frames do
   * not come from a published export directory.
   *
   * The operator tools read live runs out of the database rather than static
   * chunks, but they want the same playback, seeking and trail reconstruction
   * the published scenes get. Building the session from parts means that
   * logic is shared rather than reimplemented against a second clock.
   *
   * @param {PartReader[]} parts already-open parts, in display order
   * @param {{title?: string}} [options]
   */
  static fromParts(parts, { title } = {}) {
    if (!Array.isArray(parts) || parts.length === 0) {
      throw new SceneError("A session needs at least one part");
    }
    const session = new SceneSession("");
    session.manifest = { site: { title: title ?? "Scene" }, parts: [] };
    session.parts = parts;
    session._offsets = [];
    let acc = 0;
    for (const part of parts) {
      session._offsets.push(acc);
      acc += part.durationSec;
    }
    session.duration = acc;
    return session;
  }

  async open() {
    const manifest = await fetchJSON(this.manifestURL);
    if (!Array.isArray(manifest.parts) || manifest.parts.length === 0) {
      throw new SceneError("Scene manifest lists no parts");
    }
    this.manifest = manifest;

    const base = new URL(this.manifestURL, window.location.href);
    this.parts = await Promise.all(
      manifest.parts.map((p) =>
        new PartReader(new URL(p.url, base).href).open(),
      ),
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

  /**
   * The frame at ordinal position `n` across the whole session (all parts, in
   * manifest order). Strict: throws rather than clamping, matching
   * PartReader.frameAtIndex.
   */
  async frameAtIndex(n) {
    if (!Number.isInteger(n) || n < 0) {
      throw new SceneError(`Frame index ${n} is out of range`);
    }
    let remaining = n;
    for (let i = 0; i < this.parts.length; i++) {
      const part = this.parts[i];
      if (remaining < part.frameCount) {
        const frame = await part.frameAtIndex(remaining);
        return { partIndex: i, frame };
      }
      remaining -= part.frameCount;
    }
    const total = this.parts.reduce((sum, p) => sum + p.frameCount, 0);
    throw new SceneError(
      `Frame index ${n} is out of range for this scene (0-${total - 1})`,
    );
  }

  /**
   * Reconstructs trail history ending at `us` (microseconds within
   * `partIndex`'s own timeline) over the trailing `historySeconds` window,
   * clamped to that part's own start so a trail never implies motion from a
   * different recording. Near a part's start this returns whatever is
   * available — `fromUs` reports the actual earliest sample used, rather
   * than padding or waiting for a full window.
   *
   * Split from trailHistory so a caller that has already resolved a part and
   * an offset — capture's frame-index selector, which does not go through
   * locate() — is not forced to round-trip through scene-wide seconds.
   */
  async trailHistoryForPart(partIndex, us, historySeconds) {
    const part = this.parts[partIndex];
    const requestedFromUs = us - Math.max(0, historySeconds) * 1e6;
    const fromUs = Math.max(part.startUs, requestedFromUs);
    const frames = await part.framesInRange(fromUs, us);
    return { partIndex, frames, fromUs, toUs: us };
  }

  /** trailHistoryForPart for the part and offset showing at `seconds`. */
  async trailHistory(seconds, historySeconds) {
    const { partIndex, us } = this.locate(seconds);
    return this.trailHistoryForPart(partIndex, us, historySeconds);
  }
}
