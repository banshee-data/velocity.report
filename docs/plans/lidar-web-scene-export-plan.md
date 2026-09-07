# Web scene export (v0.6.x)

- **Status:** Phase 0 delivered; Phase 1 not started
- **Layers:** LiDAR pipeline (L9 endpoints), Web frontend, CI
- **Target:** v0.6.x; publish one real recorded scene on the existing velocity.report Pages site, then generalise
- **Companion plans:** [lidar-scene-catalogue-publishing-plan](lidar-scene-catalogue-publishing-plan.md) owns ingest, indexing and archive-scale publishing
- **Canonical:** [VRLOG_FORMAT.md](../../data/structures/VRLOG_FORMAT.md) remains the source-of-truth recording format

## Motivation

A `.vrlog` directory is the single source of truth for what a run analysed and
logged, and it stays that way. What it is not is fetchable: it lives on a
Raspberry Pi or a removable volume, and the only reader is a macOS visualiser
speaking gRPC.

This plan defines the **web scene export**: a JSON view of a recording, derived
from a VRLOG (or from a PCAP through the existing perception path), small enough
to serve statically and simple enough that a browser needs no protobuf runtime,
no WASM, and no database. There is precedent in the repository already —
`cmd/tools/lidar-scene-extract` writes exactly this shape of JSON for the
homepage hero, and `hero-scene.js` consumes it. That flow is rebuilt to take
VRLOG as an input and to emit playable, seekable scenes.

The recording stays authoritative; the export is a derived, lossy-by-selection
view, regenerable at any time.

## Current state

| Fact                     | Value                                                   | Source                                                           |
| ------------------------ | ------------------------------------------------------- | ---------------------------------------------------------------- |
| VRLOG layout             | `header.json`, `index.bin`, `frames/chunk_NNNN.pb`      | `VRLOG_FORMAT.md`                                                |
| Frame encoding on disk   | protobuf `FrameBundle`, length-prefixed                 | `recorder/proto_codec.go`                                        |
| Chunk path is hard-coded | `fmt.Sprintf("chunk_%04d.pb", …)` at three call sites   | `recorder.go:228`, `:424`, `:720`                                |
| Existing JSON exporter   | PCAP → scene JSON, background + per-frame moving points | `cmd/tools/lidar-scene-extract/main.go`                          |
| Existing JSON consumer   | three.js, static background + per-frame points          | `public_html/src/js/hero-scene.js`                               |
| Site build               | Eleventy, copies JS; no bundler, no TypeScript step     | `public_html/`                                                   |
| Archive occupancy        | ~3 concurrent tracks, 4.41 s mean life                  | `sensor_data.db`, 119,738 tracks                                 |
| Archive foreground       | median 1.49% of 69,532 pts = ~1,036 pts/frame           | 118 `segments.json` on `/Volumes/lidar`                          |
| Rotation rate            | 9.95–10.03 Hz within a single capture                   | `segments.json` `frame_rate_10s`                                 |
| Reference capture        | `soma1-static-0.pcap`, 1.5 GiB, static sensor           | [reference-capture.md](../lidar/operations/reference-capture.md) |

Phase 0 has since landed: `velocity scene export` reads a VRLOG,
`public_html/src/scenes/soma1/` is a published page, and `scene-reader.js`,
`scene-camera.js`, `scene-timeline.js` and `scene-player.js` are the browser
side. The paragraph this replaced said none of that existed.

## Findings

Measured over 600 frames with realistic per-track variation — independent
positions, per-vehicle dimension jitter, mixed classes, evolving speed and
heading. Earlier figures using near-identical synthetic tracks flattered gzip
and are superseded.

| Encoding, ~3 concurrent tracks | raw      | gzip KB/min | 20-min gzip |
| ------------------------------ | -------- | ----------- | ----------- |
| NDJSON, unrounded float64      | 527.5 KB | 116.9       | 2,338 KB    |
| NDJSON, 3 dp (mm)              | 288.2 KB | 45.0        | 901 KB      |
| **NDJSON, 2 dp (cm)**          | 273.6 KB | **38.8**    | **776 KB**  |
| protobuf `FrameBundle`         | 344.6 KB | 46.2        | 924 KB      |

| 30-second foreground clip, 1,036 pts/frame | raw    | gzip        |
| ------------------------------------------ | ------ | ----------- |
| NDJSON, 3 dp                               | 7.9 MB | 3.08 MB     |
| **NDJSON, 2 dp**                           | 7.0 MB | **2.51 MB** |
| protobuf `FrameBundle`                     | 9.0 MB | 3.8 MB      |

| Area              | Current state                                                | Severity | Release view                                                         |
| ----------------- | ------------------------------------------------------------ | -------- | -------------------------------------------------------------------- |
| Rounding          | Unrounded float64 costs 3× the rounded equivalent            | High     | Round to 2 dp at export; it is the single largest lever              |
| Container choice  | JSON at 2 dp beats protobuf by 16% (tracks) and 34% (clips)  | Low      | JSON is not a compromise here; it also removes the browser toolchain |
| Browser toolchain | Protobuf would need generated stubs, a runtime and a bundler | Medium   | JSON needs none; plain ES modules work with the existing copy step   |
| Playback timing   | Rotation rate varies 9.95–10.03 Hz within one capture        | High     | Drive playback from recorded timestamps, never a fixed interval      |
| Frame stride      | "stride 2" and "5 Hz" are not the same statement             | Medium   | `frame_stride` means retain every Nth frame; Hz is an observation    |
| Multi-part scenes | 20 minutes is four ~5-minute captures, not one recording     | High     | A manifest composes parts into one logical timeline                  |
| Seeking           | Linear scan over thousands of frames is unacceptable         | Medium   | Binary-search the per-part timestamp index                           |

### Correction: gzipped chunks are not transparent to existing tooling

An earlier draft claimed a compressed VRLOG profile could be read by existing
tools without modification. That was **false**. The replayer builds chunk paths
with `fmt.Sprintf("chunk_%04d.pb", …)` at three call sites, so neither a
renamed nor a recompressed chunk would be found.

The web export sidesteps the problem rather than solving it: it is a separate
derived artefact in its own directory, not a VRLOG variant. No claim is made
that `vrlog-analyse` or the replayer reads it. The recorded VRLOG remains the
only thing those tools open, which is the correct boundary.

### Correction: index arithmetic

An earlier draft sized a 20-minute seek index at 14 KB. At 6,000 retained
frames and 24 bytes per entry that is **144,000 bytes (~141 KiB)** — the figure
was computed for 600 frames, not 6,000. Likewise a per-chunk figure of ~15 KB
should have been ~7.7 KB. Both are corrected in the sizing above, which uses
JSON and does not carry `index.bin` at all.

## Design / approach

### 1. Round at export; it is the whole game

Positions, velocities and dimensions are written at **2 decimal places (1 cm)**;
angles at 3 dp (~0.06°). The Pandar40P's range accuracy is ±2 cm, so 1 cm sits
at the sensor's noise floor and the rounding is lossless in any meaningful
sense. Going to 1 mm costs 16% for precision the sensor never had; leaving
float64 untouched costs 3×, because gzip cannot compress 17 significant digits
of arithmetic noise.

Timestamps are **never** rounded. They are the playback clock.

### 2. Shape: NDJSON frames, one small manifest

One line per frame keeps the export streamable, appendable, and trivially
seekable by line offset, without loading a 20-minute array into memory.

```text
frames.ndjson — one line per retained frame, newline-delimited

{"f":1204,"t":1756700123456789012,"tr":[{"id":"a7","x":-12.44,"y":3.08,"z":0.8,
"vx":11.62,"vy":-0.31,"spd":11.63,"hdg":-0.027,"l":4.41,"w":1.83,"h":1.52,
"bh":-0.027,"c":"car","cf":0.87}]}

(wrapped here for the page; it is a single line on disk)
```

Short keys are deliberate: at three tracks per frame and 6,000 frames, field
names are a material share of the bytes.

The manifest describes **composition only**, and does not restate what each
part's own header already says:

```jsonc
{
  "version": 1,
  "site": { "id": "soma1", "title": "SoMa 1" },
  "parts": [
    { "url": "./part-000/", "start_seconds": 0 },
    { "url": "./part-001/", "start_seconds": 300 },
  ],
}
```

Duration, frame count and time bounds are read from each part's `header.json`,
so the manifest cannot drift out of step with the data it indexes.

### 3. Timestamps are authoritative; stride is not a frame rate

`frame_stride: 2` means **retain every second source frame**. It does not mean
5 Hz. A capture measured between 9.95 and 10.03 Hz within a single file, and
inter-frame intervals are not uniform by design — a VRLOG frame is one sensor
rotation, not one tick of a clock.

The player therefore advances on `t[n+1] − t[n]` and must render correctly when
retained intervals are 198 ms, 203 ms, 201 ms. A `setInterval(200)` player is
wrong even though it would usually look right.

### 4. Chunking and compression

Frames are grouped at **100 retained frames per chunk** — about 20 seconds at
stride 2 — and each chunk is gzipped independently, so a chunk is a whole-file
fetch and seeking never needs a byte range. At the measured 38.8 KB/min a chunk
averages **~7.7 KB**.

Chunks are decompressed with the platform:

```js
const res = await fetch(url);
if (!res.ok) throw new Error(`${url}: ${res.status}`);
const text = await new Response(res.body.pipeThrough(new DecompressionStream("gzip"))).text();
```

`DecompressionStream` has been Baseline since May 2023 for `gzip`. Brotli would
be ~17% smaller but is standardised only on Chromium, so it would need a WASM
polyfill on Firefox and Safari.

### 5. No browser toolchain is added

Because the payload is JSON, the browser needs no protobuf runtime, no generated
stubs, and no bundler. Plain ES modules work with Eleventy's existing copy step.
This removes an entire class of work — schema generation, drift gates, bundle
configuration — that a protobuf payload would have required.

The asset base URL is a parameter, not a constant, so heavy assets can later move
to object storage without touching the player.

### 6. Reading and seeking

Each part carries a small `index.json`: the first and last timestamp, frame
count, chunk count, and the first timestamp of each chunk. Seeking is a binary
search over chunk start times, then a binary search within the decoded chunk.
Never a linear scan.

Cache the previous, current and next chunk; prefetch forward during playback.
Do not download the whole scene before the first frame renders.

Every parse validates before trusting: chunk line counts, monotonic timestamps,
and numeric ranges. Downloaded data is untrusted input.

### 7. Privacy invariants

- **Track identifiers are re-keyed per part** and never reused across parts or
  sites, so a trajectory cannot be linked from one recording to another. They
  need only be stable enough for trail continuity within a part.
- **The `tracks` export carries no point cloud.** Gait and body shape live in
  point clouds; they are absent by construction.
- **No source paths, machine paths, usernames or private capture metadata** in
  published headers or manifests.
- Clips are 30 seconds and disjoint, not a continuous movement record.

### 8. A scene's vantages live in exactly one file

A vantage is a named viewpoint: `azimuth_deg`, `polar_deg`, `zoom`, and an
offset across the ground, all relative to the scene's own framing so a saved
angle survives a re-export. The labels are the point — "Eastbound Howard" tells
a reader what they are looking along, "From east" does not.

They are stored in **`vantages.json` at the scene root**, beside
`manifest.json`, and nowhere else. Vantages describe a place rather than a run,
so a scene split into several parts has one set of them, not one per part.

Four stores existed briefly, and the arrangement failed exactly as duplication
does: the viewer preferred the export header, so editing `vantages.json` looked
like it did nothing. Two of the four were never even wired up —
`Recorder.SetVantages` had no callers, and nothing copied the scene index's
column into an export, so editing that column in the web editor changed nothing
a visitor could see while looking authoritative.

| Former store                 | Why it is gone                                                                   |
| ---------------------------- | -------------------------------------------------------------------------------- |
| Export part `header.json`    | Per-part copy of a scene-level fact; the viewer chose between it and the file    |
| VRLOG `header.json`          | A recording holds what the sensor measured; a viewpoint is a presentation choice |
| `lidar_scenes.vantages_json` | Never reached a published page (dropped in migration 41)                         |

The editing loop is the viewer itself: frame an angle, press **Copy this view**,
paste the JSON it emits into `vantages.json`. `velocity scene vantages FILE`
checks one before it is published, because a typo otherwise falls back to
compass bearings and mislabels an intersection without saying so.

Guards in both languages fail if a second store returns: an export writes no
vantages into any JSON it produces, a recording carries none, the scene table
has no such column, the player reads no header field and holds no fallback
chain, and no published asset but `vantages.json` mentions them.

### 9. Transport: one timeline, and it loops

The annotated strip **is** the scrubber. It carries traffic by mode diverging
from a zero line, peak speed, and the playhead; a bare range input underneath
said less and took more room, so the transport is `Play`, clock, strip,
duration, speed, in one row. Because a canvas answers to no keyboard by
default, the strip takes `role="slider"` and handles arrows, Page, Home and
End — the range input was the only pointer-free way to scrub and that could not
be silently dropped.

Playback wraps at the end rather than stopping. A scene is a loop of street,
not a film with an ending. Trails are dropped at the wrap and at any seek,
since a trail is accumulated motion and carrying one across a jump draws a line
between two unrelated moments.

Speeds are 1x, 4x, 8x and 16x. The frame-step clamp is on wall time, not scene
time, so 16x stays 16x and only a stalled frame is bounded.

### 10. The camera flies itself

A junction seen from one fixed three-quarter angle reads as a diagram. Seen
from a slow circle it reads as a place, and the same traffic is shown from
every approach without anyone touching a control. So the viewer opens with a
**virtual drone** on, circling on its own clock — it keeps turning while
playback is paused, and does not fly at sixteen times the speed when the
recording does.

The vantages are treated as **points on a circle, not stations to stop at**.
Flying vantage-to-vantage means accelerating, arriving, holding and setting off
again once per approach, and every one of those is a jolt. Instead one orbit is
fitted to the eligible vantages — a plain mean of their elevation, distance and
centre — and swept at a constant rate: no stops, no easing, no change of tilt
or zoom, one continuous turn at about 9 degrees a second, 40 seconds a lap.

The fit passes through every vantage's bearing exactly and through the vantage
itself only as closely as a single circle can. That is the trade, and it is
worth it: for a junction surveyed from four equivalent approaches the mean _is_
each of them, and `velocity scene vantages` reports the fit so a mismatched
elevation shows up before publication rather than as a lurch on the live page.

Two kinds of vantage are wrong to average into an orbit, so a scene marks them
`"fly": false`: an overhead plan view would flatten the circle onto the ground,
and a wide establishing shot would push it out past the scene. Everything else
joins by default.

The controls follow from one rule — a person always outranks the drone:

| Action                                      | Effect                                             |
| ------------------------------------------- | -------------------------------------------------- |
| Picking a vantage                           | Lands the drone, holds that view                   |
| Dragging, pinching, or an arrow key         | Lands the drone where the camera stands            |
| The **Fly** switch, left of the vantage bar | Takes off again from the bearing already on screen |

Landing never moves the camera, and taking off resumes at the current bearing
rather than wherever the flight clock had drifted, so neither transition cuts
to an unrelated view. A drag that is overwritten by the next animation frame
reads as a broken control, which is why the camera reports human input
separately from its own motion.

## Scope

### Item 1: Export command

**Summary:** Rebuild the scene-export flow to read a VRLOG (or a PCAP through
the existing perception path) and write web scene JSON.

**Steps:**

1. Generalise `cmd/tools/lidar-scene-extract` into `velocity scene export` with
   a VRLOG input path alongside the existing PCAP path.
2. Implement the `tracks` export: NDJSON frames, 2 dp rounding, 100-frame gzip
   chunks, `index.json`, `header.json`, configurable stride.
3. Implement the `clip` export: foreground points at 2 dp, 30-second window.
4. Implement the `background` export: single downsampled static cloud.
5. Re-key track identifiers per part.
6. Refuse a source whose frame count disagrees with its capture's rotation
   count; record the source VRLOG SHA-256 in `header.json`.

**Milestone:** v0.6.0

### Item 2: Browser reader

**Summary:** A transport layer with no rendering in it.

**Steps:**

1. `SceneReader`: fetch `header.json` and `index.json`, resolve a timestamp to a
   chunk, fetch, decompress, parse, cache three chunks, prefetch forward.
2. `SceneSession`: compose multiple parts into one logical timeline; handle part
   boundaries without assuming track IDs survive across them.
3. Binary search by timestamp at both chunk and frame level.
4. Validate all downloaded structures; fail with a useful message when
   `DecompressionStream` is unavailable.
5. Unit tests against a small committed fixture — not against a large real asset.

**Milestone:** v0.6.0

### Item 3: Player and scene page

**Summary:** The page a person actually opens.

**Steps:**

1. New `scene-player.js` beside `hero-scene.js`; extract shared three.js helpers
   only where it genuinely reduces duplication. Do not refactor the hero first.
2. Render oriented boxes from `x/y/z`, `l/w/h`, `bh`; colour by class; show
   speed in mph in the UI while keeping m/s internally.
3. Accumulate trails client-side from recent frames.
4. Transport: play, pause, scrub, playback rate. Advance on recorded timestamps.
5. Load background first if present, then tracks over it.
6. Honour `prefers-reduced-motion`; verify at mobile widths.

**Milestone:** v0.6.1

### Item 4: Publish one real scene

**Summary:** Phase 0 — the milestone that proves the path.

**Steps:**

1. Use the reference capture, `soma1-static-0.pcap`. It is the longest real
   recording with a matching VRLOG and a static sensor throughout; see
   [reference-capture.md](../lidar/operations/reference-capture.md) for its
   provenance and observed characteristics.
2. Export the existing VRLOG for that capture. When a longer scene is wanted,
   compose several parts through the manifest rather than concatenating PCAPs
   to manufacture one large VRLOG.
3. Export `tracks` for all parts, one `clip`, and a `background`.
4. Commit the derived assets under `public_html/`; never the raw PCAPs.
5. Publish through the existing Pages deployment.
6. Record source capture, run configuration, build version, export command and
   resulting asset sizes in
   [reference-capture.md](../lidar/operations/reference-capture.md).

**Milestone:** v0.6.1

## Phasing

**Phase 0 — one real scene.** The reference capture,
[`soma1-static-0.pcap`](../lidar/operations/reference-capture.md), published
through the existing velocity.report Pages deployment: 11 minutes at stride 2,
1.2 MiB. `tracks` first, then a `clip` and a `background`. The browser must render oriented boxes, play on
recorded timestamps, and seek across the whole recording with no backend, no
database, and no separate repository. A small JSON manifest composes several
recording parts into one timeline. No S2 metadata, archive importer, map or
catalogue is required. Phase 0 is complete when a public static URL renders the
real site from published static assets.

**Phase 1 — the other five sites**, once Phase 0 has produced real numbers.

**Phase 2 — archive scale**, per the catalogue plan.

## Budget

Per site: 8 × 20-minute `tracks` segments at stride 2 (388 KB each) + 1 clip
(2.51 MB) + background (~0.2 MB) = **5.8 MB**. Against a 1 GB Pages site that is
roughly **176 sites**. Phase 0 is one site at about 3 MB.

## Risks

| Risk                                                    | Likelihood | Impact | Mitigation                                                                 |
| ------------------------------------------------------- | ---------- | ------ | -------------------------------------------------------------------------- |
| Player written against a fixed frame interval           | Medium     | High   | Advance on recorded timestamps; fixture with non-uniform intervals         |
| Rounding applied to timestamps                          | Low        | High   | Timestamps are integers and explicitly excluded from rounding              |
| Track IDs assumed to survive a part boundary            | High       | Medium | IDs are re-keyed per part; the session layer resets trail state            |
| Whole scene downloaded before first paint               | Medium     | Medium | Three-chunk cache, forward prefetch, index fetched separately              |
| Export drifts from the recording it claims to represent | Medium     | High   | `source_vrlog_sha256` and build version in every exported header           |
| Tracks-only scene is spatially unreadable               | Medium     | Medium | Background export is in scope for Phase 0, not deferred                    |
| Malformed or truncated chunk crashes the player         | Medium     | Low    | Validate structure and bounds on every parse; treat downloads as untrusted |

## Checklist

### Outstanding

- [ ] Phase 1: the other five sites, once the surveyed coordinates arrive (`M`)
- [ ] Re-export soma1 from the VRLOG so the published assets carry the current exporter's output (`S`)
- [x] Generalise the scene exporter to accept a VRLOG input (`M`)
- [x] `tracks` export: NDJSON, 2 dp, gzip chunks, index (`M`)
- [x] `clip` and `background` exports (`M`)
- [x] Per-part track-ID re-keying (`S`)
- [x] `SceneReader`: fetch, decompress, parse, cache, prefetch (`M`)
- [x] `SceneSession`: multi-part timeline and boundary handling (`M`)
- [x] Binary-search seek at chunk and frame level (`S`)
- [x] Exporter tests: stride, rounding, timestamps unchanged, chunk rollover, gzip round-trip (`M`)
- [x] Reader tests: malformed input, bounds, seek at start / boundary / end (`M`)
- [x] `scene-player.js`: boxes, class colour, trails, transport (`L`)
- [x] Named vantages in one file, with guards against a second store (`M`) — §8
- [x] Background point cloud, orbit/pan camera, mode toggle, timeline annotation (`M`)
- [x] Looping playback, single timeline, keyboard scrubbing (`S`) — §9
- [x] Constant-speed drone orbit, on by default, with a Fly switch (`M`) — §10
- [x] Publish the reference capture through existing Pages (`M`) — 11 min, 1.2 MiB
- [x] Document sources, commands and measured asset sizes (`S`) — [reference-capture.md](../lidar/operations/reference-capture.md)

### Deferred

- [ ] Committed fixture with non-uniform frame intervals: exporter tests build
      their VRLOGs through the real recorder, which already exercises uneven
      timestamps, so a checked-in binary fixture would add weight without cover
- [ ] Protobuf in the browser: JSON at 2 dp is smaller here and needs no toolchain
- [ ] Brotli chunks: ~17% smaller, but `DecompressionStream` supports `br` only on Chromium
- [ ] Compressed-chunk VRLOG variant: the replayer hard-codes `chunk_%04d.pb`; the web export is a separate artefact instead
- [ ] Cross-part track association: breaks the per-part identity invariant

### Accepted residuals (no action planned)

- [ ] Rounding to 1 cm discards precision below the sensor's ±2 cm accuracy
- [ ] The web export is not readable by `vrlog-analyse` or the replayer, and does
      not claim to be; the recorded VRLOG remains the only input to those tools
- [ ] A 30-second clip is 2.51 MB; slow on a poor mobile connection
