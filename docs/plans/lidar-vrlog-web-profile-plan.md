# VRLOG web profile (v0.6.x)

- **Status:** Draft
- **Layers:** LiDAR pipeline (L9 endpoints), Web frontend, CI
- **Target:** v0.6.x; make a recorded VRLOG viewable in a browser without inventing a second wire format
- **Companion plans:** [lidar-scene-catalogue-publishing-plan](lidar-scene-catalogue-publishing-plan.md) owns ingest, indexing and the catalogue; this plan owns the export format
- **Canonical:** [VRLOG_FORMAT.md](../../data/structures/VRLOG_FORMAT.md)

## Motivation

A `.vrlog` directory is the single source of truth for what a run analysed and
logged. It is already a seekable, chunked, protobuf-encoded frame store with a
byte-offset index. What it is not is fetchable: it lives on a Raspberry Pi or a
removable volume, and the only reader is a macOS visualiser speaking gRPC.

The gap is distribution, not representation. **This plan adds no new format.**
It defines a _web profile_ of VRLOG: the same directory layout, the same
`FrameBundle` protobuf, the same `index.bin`, filtered to the frame content a
browser needs and packaged so a static host can serve it. The recorded VRLOG
stays authoritative; a web profile is a derived, lossy-by-selection view of it,
regenerable at any time from the source.

The consequence of inventing a parallel format instead: two schemas to keep in
step, two decoders to test, and a published artefact that cannot be diffed
against the run it claims to represent.

## Current state

| Fact                  | Value                                              | Source                                       |
| --------------------- | -------------------------------------------------- | -------------------------------------------- |
| VRLOG layout          | `header.json`, `index.bin`, `frames/chunk_NNNN.pb` | `VRLOG_FORMAT.md`                            |
| Frame encoding        | protobuf `FrameBundle`, length-prefixed            | `recorder/proto_codec.go`                    |
| Index entry           | 24 bytes: FrameID, TimestampNs, ChunkID, Offset    | `VRLOG_FORMAT.md`                            |
| Chunk rotation        | 1,000 frames or 150 MB                             | `recorder.go:29,36`                          |
| Tier enum             | `FRAME_TYPE_FOREGROUND` / `_BACKGROUND` / `_EMPTY` | `visualiser.proto:43-47`                     |
| Foreground decimation | `DECIMATION_FOREGROUND_ONLY`                       | `visualiser.proto:39`                        |
| Generated stubs       | Go and Swift only                                  | `Makefile` `proto-gen-go`, `proto-gen-swift` |
| Archive occupancy     | ~3 concurrent tracks, 4.41 s mean life             | `sensor_data.db`, 119,738 tracks             |
| Archive foreground    | median 1.49% of 69,532 pts = ~1,036 pts/frame      | 118 `segments.json` on `/Volumes/lidar`      |

Everything the web profile needs already exists in the format. What is missing
is a filter, a web-sized chunk policy, chunk compression, and a JS decoder.

## Findings

Measured through `recorder.Record` at 600 frames (one minute at 10 Hz), gzip at
best compression, brotli q11.

| Area                 | Current state                                                      | Severity | Release view                                                       |
| -------------------- | ------------------------------------------------------------------ | -------- | ------------------------------------------------------------------ |
| Format proliferation | A custom int16 encoding is 3.7–5.8× smaller but is a second schema | High     | Do not build it; the web profile is within budget at current scale |
| Chunk size           | 1,000 frames / 150 MB is an archival policy, not a web one         | High     | Web profile rotates at 100 frames so a chunk fetches whole         |
| Compression          | Chunks are written uncompressed                                    | High     | gzip the chunks; `DecompressionStream('gzip')` is baseline         |
| Brotli in-browser    | 17% smaller than gzip but Chromium-only in `DecompressionStream`   | Medium   | Use gzip; revisit if a WASM decoder is ever justified              |
| JS decoder           | No TypeScript stubs generated from `visualiser.proto`              | High     | Add `proto-gen-ts`; same schema contract as Go and Swift           |
| Frame rate           | 10 Hz doubles segment cost for no visible gain on smoothed tracks  | Medium   | Decimate segments to 5 Hz; keep clips at 10 Hz                     |
| Scale ceiling        | Web profile busts a 1 GB Pages site somewhere past ~130 sites      | Medium   | Shard catalogue repos; do not respond by inventing a format        |

### Measured cost of the web profile

| Tier                          | raw KB/min | gzip KB/min | brotli KB/min |
| ----------------------------- | ---------- | ----------- | ------------- |
| Tracks only, 2 concurrent     | 245.0      | 39.4        | 32.5          |
| **Tracks only, 3 concurrent** | **344.6**  | **46.2**    | **38.3**      |
| Tracks only, 5 concurrent     | 543.8      | 61.7        | 49.8          |
| Tracks only, 10 concurrent    | 1,041.8    | 97.8        | 76.8          |
| Foreground cloud, 1,036 pts   | 9,177.4    | 7,725.5     | 7,204.2       |
| Foreground cloud, 2,639 pts   | 22,798.3   | 19,426.0    | 18,227.4      |

Derived asset sizes at this archive's occupancy:

| Asset                              | 10 Hz   | 5 Hz    |
| ---------------------------------- | ------- | ------- |
| 20-minute tracks segment (gzip)    | 924 KB  | 462 KB  |
| 30-second foreground clip (gzip)   | 3.8 MB  | 1.9 MB  |
| Background snapshot (single frame) | ~214 KB | ~214 KB |

A purpose-built int16 encoding measured 5.8× smaller for tracks and 3.7× smaller
for clouds. **That saving does not justify a second format at this scale.** It is
recorded here so the trade is explicit if the ceiling is ever reached.

## Design / approach

### 1. The web profile is VRLOG, narrowed

A web profile directory is a valid `.vrlog` in every respect a replayer cares
about. The differences are policy, not schema:

```text
<name>.vrlog/                       <name>.web.vrlog/
  header.json                         header.json          + "profile" field
  index.bin                           index.bin            identical layout
  frames/chunk_0000.pb                frames/chunk_0000.pb.gz
    1,000 frames or 150 MB              100 frames, gzip
    all FrameBundle fields              filtered by profile
```

Three profiles, each a filter over the frames the source already contains. All
three use `FrameType` values that exist today; no enum is added.

| Profile      | `FrameType`  | PointCloud                   | Tracks | Rate  |
| ------------ | ------------ | ---------------------------- | ------ | ----- |
| `tracks`     | `FOREGROUND` | nil                          | yes    | 5 Hz  |
| `clip`       | `FOREGROUND` | `DECIMATION_FOREGROUND_ONLY` | yes    | 10 Hz |
| `background` | `BACKGROUND` | `BackgroundSnapshot`         | no     | once  |

A frame with no tracks is written as `FRAME_TYPE_EMPTY`, which the format
already defines and which costs 56.9 bytes.

### 2. `header.json` gains one field

```jsonc
{
  "version": "0.5",
  "profile": "tracks", // absent on a recorded VRLOG; present on every web profile
  "source_vrlog_sha256": "…", // the run this was derived from
  "frame_stride": 2, // 1 = every frame; 2 = 5 Hz from a 10 Hz source
  "chunk_encoding": "gzip",
}
```

`profile` absent means a full recorded VRLOG — existing files stay valid and
existing readers are unaffected. A reader that does not understand `profile`
still decodes every frame correctly, because the frames are ordinary
`FrameBundle` messages with some fields unset.

### 3. `index.bin` is already the Range map

The 24-byte index entry carries `ChunkID` and `Offset`. That is precisely what a
browser needs to seek: read `index.bin` once (24 bytes × frame count — 14 KB for
a 20-minute segment at 5 Hz), then fetch only the chunks a scrub position
touches. No new index, no manifest of byte ranges.

Chunk size drops to **100 frames** so a chunk is a whole-file fetch rather than a
Range request. This sidesteps the conflict between range requests and whole-file
compression: each chunk is independently gzipped and independently fetchable. At
5 Hz a chunk is 20 seconds of footage and about 15 KB.

### 4. Compression: gzip, decoded natively

`DecompressionStream` has been Baseline since May 2023 for `gzip` and `deflate`.
The `br` format is standardised only in Chromium, so brotli would need a WASM
polyfill on Firefox and Safari. Brotli is 17% smaller; a polyfill is not worth
17% on a 462 KB asset.

```js
const res = await fetch(`${base}/frames/chunk_0003.pb.gz`);
const buf = await new Response(
  res.body.pipeThrough(new DecompressionStream("gzip")),
).arrayBuffer();
// then: length-prefixed FrameBundle messages, decoded by generated stubs
```

### 5. The decoder is generated, not written

Add `proto-gen-ts` alongside the existing Go and Swift targets, emitting
TypeScript from `proto/velocity_visualiser/v1/visualiser.proto`. The browser
decodes the same messages the visualiser does, from the same schema, checked by
the same `make proto-gen` drift gate. A hand-written parser would reintroduce
exactly the dual-maintenance problem this plan exists to avoid.

### 6. Scale ceiling, stated plainly

Per site: 8 × 20-minute segments at 5 Hz (3.7 MB) + 1 × 30-second clip at 10 Hz
(3.8 MB) + background (0.2 MB) = **7.7 MB**. Against a 1 GB Pages site that is
about **133 sites per catalogue repository**.

| Scale                          | Segments | Clips  | Total    | Fits 1 GB?   |
| ------------------------------ | -------- | ------ | -------- | ------------ |
| 6 sites, all available footage | 26 MB    | 46 MB  | 72 MB    | Yes, 7%      |
| 50 sites                       | 185 MB   | 190 MB | 385 MB   | Yes, 38%     |
| 133 sites                      | 492 MB   | 505 MB | 1,007 MB | At the limit |
| 200 sites                      | 739 MB   | 760 MB | 1,499 MB | No — shard   |

Beyond ~130 sites, shard across catalogue repositories. Each repository carries
its own 1 GB storage _and_ its own 100 GB monthly bandwidth, so sharding buys
both. The response to the ceiling is more repositories, not a new format.

### 7. Privacy invariants

Unchanged from the catalogue plan and enforced at export:

- **Track identifiers are re-keyed to be segment-local** and never reused across
  segments or sites, so a trajectory cannot be linked from one site to another.
- **`tracks` profile carries no point cloud.** Gait and body shape live in point
  clouds; they are absent by profile, not by filter.
- **Clips are 30 seconds and disjoint**, not a continuous movement record.
- Origin latitude and longitude are stripped from `CoordinateFrameInfo` unless
  the site is a surveyed public location published deliberately.

## Scope

### Item 1: Web profile specification

**Summary:** Extend `VRLOG_FORMAT.md` with the profile section; do not create a
new format document.

**Steps:**

1. Add a "Web profile" section to `VRLOG_FORMAT.md` covering the three profiles,
   the `header.json` additions, the 100-frame chunk policy, and gzip framing.
2. State the compatibility rule: `profile` absent means a recorded VRLOG, and
   any existing reader decodes a web profile without modification.
3. Bump `VRLOGFormatVersion` and record the change in the format history.

**Milestone:** v0.6.0

### Item 2: Export command

**Summary:** `velocity scene export` reads a recorded VRLOG and writes a web
profile.

**Steps:**

1. Implement filtering as a `FrameBundle` transform: strip `PointCloud` for
   `tracks`, keep foreground-only cloud for `clip`, select `BACKGROUND` frames
   for `background`.
2. Implement frame striding (`--stride`), defaulting to 2 for `tracks` and 1 for
   `clip`.
3. Re-key track identifiers to segment-local values.
4. Add a recorder option for web chunk size and gzip chunk output rather than
   post-processing files.
5. Refuse a source whose frame count disagrees with its capture's rotation count.
6. Record `source_vrlog_sha256` so an export can always be traced to its run.

**Milestone:** v0.6.0

### Item 3: TypeScript stubs and reader

**Summary:** Generated decoder plus a thin fetch-and-seek layer.

**Steps:**

1. Add `proto-gen-ts` to the Makefile and wire it into `proto-gen`.
2. Extend `make check-agent-drift` or an equivalent gate to catch stale stubs.
3. Write a `VrlogReader` class: fetch `header.json` and `index.bin`, seek by
   frame or timestamp, fetch and cache chunks, decode with the generated stubs.
4. Unit-test the reader against a fixture web profile committed to the repo.

**Milestone:** v0.6.1

### Item 4: Viewer

**Summary:** Render a web profile in the browser.

**Steps:**

1. Extend `public_html/src/js/hero-scene.js` into a scene viewer: OBB rendering,
   class colouring, trails accumulated client-side from the track stream.
2. Timeline scrubber with frame-accurate seek and playback rate.
3. Load `background` once, then `tracks` or `clip` over it.
4. Track inspector: class, confidence, speed, dimensions, duration.
5. Honour `prefers-reduced-motion`; verify at mobile widths.

**Milestone:** v0.6.2

## Dependencies

| Dependency                      | Gates         | Status                              |
| ------------------------------- | ------------- | ----------------------------------- |
| `protoc-gen-ts` toolchain       | Item 3        | Not installed; new build dependency |
| Deterministic replay path       | Item 2 step 5 | Believed sound in analysis mode     |
| Catalogue repository and ingest | Publishing    | Owned by the catalogue plan         |
| Surveyed site origins           | Map placement | Awaiting six coordinates            |

## Risks

| Risk                                                 | Likelihood | Impact | Mitigation                                                                                     |
| ---------------------------------------------------- | ---------- | ------ | ---------------------------------------------------------------------------------------------- |
| Web profile is mistaken for an archival format       | Medium     | High   | `profile` field is mandatory in the header; exports carry source SHA                           |
| Generated TS stubs drift from the proto              | Medium     | Medium | `proto-gen` emits all three languages; drift gate in CI                                        |
| 100-frame chunks produce many small files            | High       | Low    | 60 chunks per 20-minute segment; acceptable in a git tree and on a CDN                         |
| Clip tier dominates the budget                       | High       | Medium | Clips are half the projection at every scale; cut clips before segments                        |
| A future site is far busier than 3 concurrent tracks | Medium     | Medium | Cost is measured to 10 concurrent (97.8 KB/min gzip); still under 2 MB per 20 min              |
| Pages does not serve `.gz` with the right headers    | Low        | Medium | Chunks are fetched as opaque bytes and decoded in JS, not by the browser's content negotiation |

## Checklist

### Outstanding

- [ ] Add the web profile section to `VRLOG_FORMAT.md` (`S`)
- [ ] Bump `VRLOGFormatVersion` and record format history (`S`)
- [ ] Recorder option: web chunk size and gzip chunk output (`M`)
- [ ] `velocity scene export` with profile filtering and striding (`M`)
- [ ] Segment-local track re-keying (`S`)
- [ ] `proto-gen-ts` Makefile target and drift gate (`M`)
- [ ] `VrlogReader` fetch/seek/decode layer plus fixture tests (`M`)
- [ ] Viewer: scrubber, OBB rendering, client-derived trails (`L`)

### Deferred

- [ ] Purpose-built int16 track encoding: 3.7–5.8× smaller, rejected as a second
      schema. Revisit only if repository sharding stops being sufficient
- [ ] Brotli chunk encoding: 17% smaller, needs a WASM polyfill outside Chromium
- [ ] Cross-segment track association: breaks the segment-local identity invariant

### Accepted residuals (no action planned)

- [ ] The web profile is 3.7–5.8× larger than a purpose-built format. This is the
      cost of one schema, and it is affordable to ~130 sites per repository
- [ ] A 30-second clip is 3.8 MB at 10 Hz; slow on a poor mobile connection
- [ ] Frame striding to 5 Hz makes segments non-identical to the source run;
      `frame_stride` in the header records exactly what was dropped
