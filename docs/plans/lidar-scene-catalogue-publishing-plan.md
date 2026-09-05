# Scene catalogue publishing (v0.6.x)

- **Status:** Draft
- **Layers:** Cross-cutting (LiDAR pipeline, database, CLI, CI, web)
- **Target:** v0.6.x; turn 122 GB of stranded daily-driver captures into a statically served map of San Francisco scenes
- **Companion plans:** [lidar-web-scene-export-plan](lidar-web-scene-export-plan.md) owns the export format; [s2-geographic-indexing-plan](s2-geographic-indexing-plan.md) owns S2 conventions
- **Canonical:** [geographic-indexing.md](../lidar/architecture/geographic-indexing.md) for all S2 conventions

## Motivation

There are 122 GB of Hesai captures on `/Volumes/lidar/lidar/s2/` covering six
sites inside S2 cell `80858-1`, and not one byte of it is reachable by anyone
without a macOS visualiser and a copy of the drive. `segments.json` exists for
all 118 runs; none of them are in `sensor_data.db`.

This plan turns that archive into a map. Two asset tiers, both statically
servable: a **30-second foreground point-cloud clip** that shows what the sensor
actually sees, and **20-minute track segments** that cover whole recording
sessions at every site. The clip is the hook; the segments are the dataset.

Without this the captures stay a private archive on a removable volume, the S2
indexing work has no consumer, and every future site multiplies a problem
nobody can look at.

## Current state

### On the volume

122 GB, 118 PCAPs, five-minute chunks of ~724 MB, plus one
`pcap_split_analysis_20260903T150630Z/` directory holding per-run
`segments.json`, `motion_timeline.json`, `frame_metrics.csv` and a text summary.

| Site prefix | Runs | Static segs | Static min | Motion min | Median fg% |
| ----------- | ---- | ----------- | ---------- | ---------- | ---------- |
| `s2-1`      | 47   | 28          | 131.8      | 98.2       | 1.07       |
| `s2_sf_2`   | 42   | 29          | 134.2      | 68.9       | 1.49       |
| `s2_sf_3`   | 9    | 8           | 28.6       | 11.9       | 2.82       |
| `s2_sf_4`   | 10   | 8           | 23.6       | 17.7       | 2.98       |
| `s2_sf_5`   | 6    | 0           | 0.0        | 29.8       | 1.90       |
| `s2_sf_6`   | 4    | 0           | 0.0        | 18.8       | 1.27       |

Totals: 63 motion segments, 73 static segments, 72 of 118 runs carrying at least
one static segment. Capture geometry is consistent at 69,532 points/frame and
9.99 Hz.

The existing analysis run wrote **no PCAP bytes** — every segment record shows
`packet_count: 0` and the summary reports the intended filename only. Splitting
still has to happen.

### In `sensor_data.db` (3.4 GB, live)

| Table                      | Rows      | Note                                            |
| -------------------------- | --------- | ----------------------------------------------- |
| `lidar_run_records`        | 1,388     | 464 carry a `vrlog_path`                        |
| `lidar_replay_cases`       | 6         | all pre-S2, `pcap_file` is a bare relative path |
| `lidar_tracks`             | 119,738   | `track_id` globally unique; all `confirmed`     |
| `lidar_track_observations` | 6,142,691 | x, y, z, velocity, speed, heading, bbox per obs |
| `lidar_run_tracks`         | 85,427    | 58,628 distinct tracks; runs re-replayed        |
| `site`                     | 2         | Clarendon, Home — neither matches a disk prefix |

None of the 118 volume runs are imported: `source_path LIKE '%s2%'` returns zero.

### Corrections to the companion plan

Three assumptions in
[lidar-web-scene-export-plan](lidar-web-scene-export-plan.md)
came from `VRLOG_RUN_COMPARISON_2026_03_10.md` and are contradicted by the live
database. That document is stale and should not be cited for these facts again.

| Earlier claim                    | Measured reality                                                  |
| -------------------------------- | ----------------------------------------------------------------- |
| `max_speed_mps` reads 0.0        | 119,736 of 119,738 tracks positive; max 116.8 m/s                 |
| Confirmed-track ratio is 3.4%    | Every persisted track is `confirmed`; the DB is already the index |
| ~10 concurrent tracks, 13 s life | ~3 concurrent, 4.41 s mean life (44.1 obs/track at 10 Hz)         |

The occupancy correction makes both tiers materially cheaper. It also means
`lidar_tracks` + `lidar_track_observations` is already the confirmed-track index
the companion plan proposed building.

### Known data-quality defect

Grouping `lidar_track_observations` by `track_id` yields a maximum track span of
282,467,723 s (~9 years). Track IDs are unique, so this is the timestamp
inversion described in `VRLOG_RUN_COMPARISON` §2.2: some observations carry
wall-clock nanoseconds where others carry PCAP time. The importer must reject or
quarantine any run whose observation timestamps fall outside its capture window.

## Findings

| Area                     | Current state                                                        | Severity | Release view                                                      |
| ------------------------ | -------------------------------------------------------------------- | -------- | ----------------------------------------------------------------- |
| Site → position          | No WGS84 fix anywhere in split output; `site` has 2 rows, disk has 6 | Blocker  | Hand-survey the six prefixes; it is the only unblocking input     |
| Split bytes              | `segments.json` exists but no segment PCAPs were written             | High     | Re-run `pcap-split` in writing mode; reuse existing analysis JSON |
| Background source        | `s2_sf_5` and `s2_sf_6` have zero static segments                    | High     | No background for two sites; relax detection or accept cloud-only |
| Capture registry         | No table maps a PCAP file to a site, run, or S2 cell                 | Blocker  | New `lidar_capture_files` / `lidar_capture_segments`              |
| Published-asset registry | Nothing records what has been exported or published                  | High     | New `lidar_scene_exports`                                         |
| Export tooling           | No scene export path from VRLOG                                      | High     | New `velocity scene export`; narrows VRLOG, adds no format        |
| Timestamp domain         | Mixed wall-clock and PCAP nanoseconds in observations                | High     | Importer validates against the capture window and quarantines     |
| Track ID reuse           | Unique in DB, but not segment-local for publishing                   | Medium   | Re-key to segment-local on export, per the privacy invariant      |

## Design / approach

### 1. Two asset tiers

Both tiers are **JSON scene exports derived from a recorded VRLOG**, which
remains the source of truth. The browser needs no protobuf runtime, no bundler
and no database. Format owned by
[lidar-web-scene-export-plan](lidar-web-scene-export-plan.md).
Sizes measured over 600 frames with realistic per-track variation, gzip at best
compression, against this archive's occupancy (~3 concurrent tracks) and
foreground fraction (1.49%).

| Tier           | Export       | Window | Stride | Size (gzip) | Per site              |
| -------------- | ------------ | ------ | ------ | ----------- | --------------------- |
| **Clip**       | `clip`       | 30 s   | 1      | ~2.51 MB    | 1 clip                |
| **Segment**    | `tracks`     | 20 min | 2      | ~388 KB     | all available footage |
| **Background** | `background` | once   | —      | ~200 KB     | 1                     |

Measured rates: `tracks` at 3 concurrent is 38.8 KB/min gzip at 2 dp rounding,
halved by stride 2. `clip` at 1,036 points/frame is 2.51 MB per 30 s. Rounding
to 1 cm is the dominant lever — unrounded float64 costs 3× — and at 2 dp JSON
is 16% smaller than protobuf for tracks and 34% smaller for clips.

**Stride is not a frame rate.** `stride 2` means retain every second source
frame. Captures measure 9.95–10.03 Hz within a single file, so playback is
driven by recorded timestamps, never a fixed interval.

Budget at today's six sites is about 37 MB all-in. The design matters because it
scales:

| Scale                          | Backgrounds | Clips  | Segments | Total    | % of 1 GB    |
| ------------------------------ | ----------- | ------ | -------- | -------- | ------------ |
| 1 site (Phase 0), 20 min       | 0.2 MB      | 2.5 MB | 0.4 MB   | 3.1 MB   | <1%          |
| 6 sites, all available footage | 1.2 MB      | 15 MB  | 11 MB    | 27 MB    | 3%           |
| 50 sites, 8 segments each      | 10 MB       | 126 MB | 155 MB   | 291 MB   | 28%          |
| 176 sites, 8 segments each     | 35 MB       | 442 MB | 546 MB   | 1,023 MB | at the limit |

Clips are roughly half the projection at every scale. The ceiling arrives near
**176 sites**; the response is another Pages site, which buys another 1 GB _and_
another 100 GB of monthly bandwidth. Phase 0 fits in under 1% of a single site's
budget, so none of this constrains the first milestone.

### 2. Canonical filesystem layout

The archive root stays immutable. Everything derived lands under `derived/`,
keyed by a content-addressed run identifier so a re-run never clobbers a
previous one.

```text
/Volumes/lidar/lidar/s2/
  <site>_<yyyymmddThhmmss>_<seq>.pcap          # immutable daily driver
  derived/
    <capture-sha12>/
      segments.json                            # existing analysis, copied in
      motion_timeline.json
      frame_metrics.csv
      summary.txt
      motion/<base>-motion-<i>.pcap            # written by pcap-split
      static/<base>-static-<i>-s2-l10-<tok>.pcap
      vrlog/<base>-<i>.vrlog/                  # only for selected clip windows
      export/
        background/                            # ~200 KB, once per site
        clip-<i>/                              # 30 s foreground points
        segment-<i>/                           # 20 min tracks, stride 2
          header.json                          # export kind, stride, source SHA
          index.json                           # chunk start timestamps
          frames/chunk_NNNN.ndjson.gz          # 100 frames per chunk, ~7.7 KB
```

Only **static** segments carry an `s2-l10-<token>` filename tag, per the S2
style guide: a motion segment may cross cells and must not claim one. The
canonical L10 token is used in paths; the `80858-1` family display never is.

### 3. Database indexing

Three new tables plus S2 columns on existing ones. The grain is deliberate:
one row per capture file, one per segment, one per published asset. Track-level
data already exists and is not duplicated.

```sql
-- One row per immutable raw PCAP.
CREATE TABLE lidar_capture_files (
    capture_id       TEXT PRIMARY KEY,   -- sha256[:12] of file bytes
    site_id          INTEGER REFERENCES site (id),
    file_path        TEXT NOT NULL,
    file_bytes       INTEGER NOT NULL,
    sensor_id        TEXT NOT NULL,
    started_at_ns    INTEGER NOT NULL,
    duration_secs    REAL NOT NULL,
    total_frames     INTEGER,
    total_packets    INTEGER,
    avg_frame_rate_hz REAL,
    foreground_pct   REAL,
    imported_at_ns   INTEGER NOT NULL
);

-- One row per motion/static segment from segments.json.
CREATE TABLE lidar_capture_segments (
    segment_id        TEXT PRIMARY KEY,
    capture_id        TEXT NOT NULL REFERENCES lidar_capture_files (capture_id) ON DELETE CASCADE,
    segment_type      TEXT NOT NULL,     -- 'motion' | 'static'
    segment_index     INTEGER NOT NULL,
    start_secs        REAL NOT NULL,
    end_secs          REAL NOT NULL,
    start_frame       INTEGER,
    end_frame         INTEGER,
    packet_count      INTEGER,
    pcap_path         TEXT,              -- NULL until bytes are written
    s2_l13_token      TEXT,              -- NULL unless located
    s2_l10_token      TEXT,
    geographic_status TEXT NOT NULL,     -- 'located' | 'unavailable' | 'not_applicable'
    CHECK (segment_type IN ('motion', 'static')),
    CHECK (geographic_status IN ('located', 'unavailable', 'not_applicable'))
);

-- One row per published web asset.
CREATE TABLE lidar_scene_exports (
    export_id      TEXT PRIMARY KEY,
    segment_id     TEXT NOT NULL REFERENCES lidar_capture_segments (segment_id) ON DELETE CASCADE,
    run_id         TEXT REFERENCES lidar_run_records (run_id) ON DELETE SET NULL,
    tier           TEXT NOT NULL,        -- 'clip' | 'segment' | 'background'
    format_version TEXT NOT NULL,
    asset_path     TEXT NOT NULL,
    asset_sha256   TEXT NOT NULL,
    asset_bytes    INTEGER NOT NULL,
    start_frame    INTEGER NOT NULL,
    frame_count    INTEGER NOT NULL,
    track_count    INTEGER,
    interest_score REAL,
    published_at_ns INTEGER,
    CHECK (tier IN ('clip', 'segment', 'background'))
);
```

Additions to existing tables:

| Table                | Columns                                         | Why                              |
| -------------------- | ----------------------------------------------- | -------------------------------- |
| `site`               | `s2_l13_token`, `s2_l10_token`, `origin_source` | Surveyed origin, derived once    |
| `lidar_replay_cases` | `s2_l13_token`, `s2_l10_token`, `capture_id`    | Per the S2 conformance contract  |
| `lidar_run_records`  | `s2_l13_token`, `s2_l10_token`, `capture_id`    | Ties a run to its source capture |

All S2 columns are nullable. A row without an accepted WGS84 source stays NULL
and records `geographic_status = 'unavailable'`; partial tagging is invalid.

### 4. Site survey is the unblocking input

The six disk prefixes need hand-surveyed WGS84 origins. From those, `site` rows
gain an L13 token calculated once and an L10 token derived only via
`CellID.Parent(10)`. Everything downstream — filename tags, segment records,
map markers — reads from `site`, so the survey is entered once and never
recomputed from a filename.

Until the survey lands, ingest still runs and writes sensor-local artefacts with
NULL S2 columns. Nothing blocks on it except the geographic index and the map
markers.

### 5. Export contract

One subcommand, rebuilt from the existing `lidar-scene-extract` flow so it
accepts a recorded VRLOG as well as a PCAP. It writes JSON; the recorded VRLOG
stays the single source of truth for what was analysed.

```text
velocity scene export \
    --vrlog <path.vrlog> | --pcap <path.pcap> \
    --export tracks|clip|background \
    --start-frame <n> --frame-count <n> \
    --stride <n> \
    --out <dir>
```

Invariants every export must hold:

- Refuse any source whose recorded frame count disagrees with its capture's
  rotation count. A published asset that cannot be regenerated is not evidence.
- Re-key track identifiers **per part**, never reused across parts or sites.
- Round positions and dimensions to 2 dp; **never round timestamps** — they are
  the playback clock.
- Write `export`, `frame_stride`, `chunk_encoding` and `source_vrlog_sha256`
  into `header.json`, so an export always names the run it came from.
- Content-address the output by SHA-256 and register it in
  `lidar_scene_exports`.

The output is a derived artefact, **not** a VRLOG variant: `vrlog-analyse` and
the replayer read the recorded VRLOG only, and the plan makes no claim otherwise.
The replayer hard-codes `chunk_%04d.pb` at three call sites, so a compressed or
renamed chunk would not be found by it. Full definition in
[lidar-web-scene-export-plan](lidar-web-scene-export-plan.md).

### 6. Clip selection

Score every candidate 30-second window, rank, present for confirmation, publish
on approval. The score is recorded in `lidar_scene_exports.interest_score` so a
ranking is reproducible and arguable.

| Signal                 | Rationale                                                    |
| ---------------------- | ------------------------------------------------------------ |
| Peak concurrent tracks | An empty street is a poor showcase                           |
| Class diversity        | Pedestrian and cyclist presence is the point of the platform |
| Max track speed        | The measurement the project exists to make                   |
| Track completeness     | Prefer windows whose tracks enter and exit cleanly           |
| Absence of artefacts   | Penalise implausible speeds and 1-frame tracks               |

## Scope

Five workstreams. W1 and W2 are parallel from day one; W3 depends on W2's
migration landing; W4 and W5 depend on W3's format spec, not its
implementation, so they can start once the spec is written.

### Workstream 1: Filesystem and ingest (backend agent)

**Summary:** Get 122 GB of captures split, registered, and reproducible.

**Steps:**

1. Add `velocity lidar pcap-split --write` so segment PCAP bytes are actually
   emitted; today's run produced records with `packet_count: 0` only.
2. Define the `derived/<capture-sha12>/` layout above and a resolver that maps
   a raw PCAP to its derived directory.
3. Write `velocity lidar import-captures`: walk the archive, hash each file,
   read the existing `segments.json` where present, and populate
   `lidar_capture_files` and `lidar_capture_segments`.
4. Make import idempotent and resumable — a 122 GB walk will be interrupted.
5. Validate observation timestamps against the capture window; quarantine runs
   that fail rather than importing corrupt spans.
6. Report per-site coverage: runs, segments, static availability, gaps.

**Interfaces owned:** `derived/` layout, `capture_id` derivation, importer CLI.
**Depends on:** W2 migration for the two capture tables.
**Milestone:** v0.6.0

### Workstream 2: Database and migrations (data agent)

**Summary:** Land the schema above without disturbing 6.1 M existing rows.

**Steps:**

1. Migration `000039_capture_registry`: create `lidar_capture_files` and
   `lidar_capture_segments` with indexes on `site_id`, `started_at_ns`,
   `s2_l10_token`, and `(capture_id, segment_index)`.
2. Migration `000040_scene_exports`: create `lidar_scene_exports`.
3. Migration `000041_s2_columns`: add nullable S2 and `capture_id` columns to
   `site`, `lidar_replay_cases`, `lidar_run_records`.
4. Seed the six surveyed site rows once coordinates are supplied; derive L13
   from WGS84 and L10 only via `Parent(10)`.
5. Add a conformance check: any row with one S2 token must have both, and
   `L13.Parent(10)` must equal `L10`.
6. Down migrations for all three, per repo convention.

**Interfaces owned:** all schema, all S2 derivation helpers.
**Depends on:** surveyed coordinates for step 4 only.
**Milestone:** v0.6.0

### Workstream 3: Scene export at archive scale (backend agent)

**Summary:** Take the Phase 0 exporter and run it over the archive. The exporter
itself is built in Phase 0 and owned by the companion plan.

**Steps:**

1. Batch export across imported captures, keyed by `capture_id`.
2. Implement the interest scorer and `velocity scene rank` for clip selection.
3. Register every produced asset in `lidar_scene_exports` with its SHA-256,
   frame range and interest score.
4. Re-export detection: skip assets whose source SHA and export settings are
   unchanged.

**Interfaces owned:** batch export, the scorer, the export registry contract.
**Depends on:** Phase 0 exporter; W2 migrations; W1 for `capture_id`.
**Milestone:** v0.6.2

### Workstream 4: Catalogue repository and CI (infra agent)

**Summary:** A new public repository serving the assets from GitHub Pages.

**Steps:**

1. Create the catalogue repo: `sites/<s2-l10>/<site>/`, `index.db`, `viewer/`.
2. `make catalogue-build`: assemble assets, rebuild `index.db` from manifests
   and `lidar_scene_exports`.
3. PR checks: no file over 100 MB; per-site budget respected; manifest schema
   valid; S2 tokens internally consistent; track IDs segment-local; rebuilt
   `index.db` matches the committed one.
4. Pages deploy with a size-delta comment and preview link on every PR.
5. Verify Pages honours `Range` on `index.db`; fall back to sharded whole-file
   fetch if not.

**Interfaces owned:** repo layout, CI checks, deploy.
**Depends on:** W3 step 1 (spec) only.
**Milestone:** v0.6.1

### Workstream 5: Map and viewer (frontend agent)

**Summary:** The map people actually land on.

**Steps:**

1. Map view over San Francisco with a marker per site, drawn from `index.db`;
   show L10 cell boundaries as context using the existing `tools/s2-hilbert`
   assets.
2. Site page: background + clip player (30 s, point cloud) and a segment list.
3. Segment scrubber: frame-accurate seek, playback rate, client-derived trails
   with adjustable length, class colouring, track inspector.
4. Deep links by site, segment and frame.
5. Honour `prefers-reduced-motion`; verify at mobile widths.

**Interfaces owned:** viewer, map, `index.db` read schema.
**Depends on:** W3 step 1 (spec) only; can develop against fixtures.
**Milestone:** v0.6.2

## Phasing

**Phase 0 — one real scene.** Approximately 20 minutes from a single site,
published through the **existing** velocity.report Pages deployment under
`public_html/`. `s2_sf_2` is the candidate: 42 runs, 29 static segments, 134
static minutes. Tracks plus one clip and a background. No S2 metadata, no
archive importer, no catalogue database, no map, no separate repository. Owned
by [lidar-web-scene-export-plan](lidar-web-scene-export-plan.md); nothing in
this plan is required to reach it. Phase 0 is complete when a public static URL
renders that site from published static assets.

Everything below is deliberately behind that milestone. The one-site viewer
produces the real numbers — asset sizes, fetch and decode latency, whether a
tracks-only scene is spatially readable — that say how much of this machinery is
justified.

**Phase 1 — the other five sites.** Split and import the archive; capture
registry and export registry populated; segments for all available footage; one
ranked clip per site. This is where the workstreams below start.

**Phase 2 — geographic index and map.** Surveyed origins, S2 tagging, the map
view, and the catalogue repository, once there is more than one site to place on
a map.

**Phase 3 — archive and network scale.** The 200-site coverage design, once
sensor deployments exist.

## Dependencies

| Dependency                               | Gates                                  | Status                                                    |
| ---------------------------------------- | -------------------------------------- | --------------------------------------------------------- |
| Surveyed WGS84 origin per site prefix    | W2 step 4, all S2 tagging, map markers | Awaiting six coordinates                                  |
| Static segments at `s2_sf_5` / `s2_sf_6` | Background tier at those sites         | None detected; needs relaxed detection or acceptance      |
| `pcap-split` writing mode                | W1 entirely                            | Not implemented                                           |
| Deterministic replay path                | Export invariant                       | Believed sound in analysis mode; unverified at this scale |
| Pages `Range` support                    | W4 index strategy                      | Assumed, unverified                                       |

## Risks

| Risk                                                                  | Likelihood | Impact | Mitigation                                                                                                |
| --------------------------------------------------------------------- | ---------- | ------ | --------------------------------------------------------------------------------------------------------- |
| Survey coordinates never arrive                                       | Medium     | High   | Ingest and both asset tiers work with NULL S2; only the geographic index and map markers block            |
| Two sites can never produce a background                              | High       | Medium | Relax static detection thresholds for those captures, or publish them cloud-only with no background layer |
| Timestamp-domain corruption spreads into published assets             | Medium     | High   | Importer validates against the capture window and quarantines; export refuses quarantined runs            |
| 122 GB walk is slow or interrupted                                    | High       | Low    | Idempotent, resumable import keyed on content hash                                                        |
| Clip tier dominates the budget at 200 sites                           | Medium     | Medium | Clips are 408 MB of the 705 MB projection; cut clips per site before segments                             |
| Re-splitting produces different boundaries than the existing analysis | Medium     | Medium | Reuse the committed `segments.json` as the authority; splitting writes bytes only                         |
| Interest scorer showcases a tracking artefact                         | Medium     | Medium | Human confirmation step before publish; penalise implausible speeds                                       |

## Checklist

### Outstanding

- [ ] Supply six surveyed WGS84 origins (`S`, external) — unblocks all S2 work
- [ ] W1: `pcap-split --write` mode (`M`)
- [ ] W1: `derived/` layout and resolver (`S`)
- [ ] W1: `velocity lidar import-captures`, idempotent and resumable (`M`)
- [ ] W1: timestamp-window validation and quarantine (`S`)
- [ ] W2: migration `000039_capture_registry` (`M`)
- [ ] W2: migration `000040_scene_exports` (`S`)
- [ ] W2: migration `000041_s2_columns` (`S`)
- [ ] W2: S2 derivation helpers and conformance check (`M`)
- [ ] W3: batch export across imported captures, keyed by `capture_id` (`M`)
- [ ] W3: interest scorer and `velocity scene rank` (`M`)
- [ ] W3: export registry writes and re-export detection (`S`)
- [ ] W4: catalogue repo, build, PR checks, Pages deploy (`M`)
- [ ] W5: map view with site markers and L10 context (`M`)

Phase 0 has its own checklist in
[lidar-web-scene-export-plan](lidar-web-scene-export-plan.md); none of the items
above are required to reach it.

### Deferred

- [ ] Full-cloud publishing: 556 MB/min, 4× larger than the source PCAP. Never ship it
- [ ] Protobuf in the browser: JSON at 2 dp measured smaller and needs no toolchain
- [ ] Brotli chunk encoding: ~17% smaller than gzip, but `DecompressionStream` supports `br` only on Chromium
- [ ] SQLite catalogue index: a JSON manifest is sufficient at the scales projected here. The 1.1 M-row track query surface is a separate question, revisited when someone actually needs it
- [ ] Cross-part track association: deliberately not built; breaks the per-part identity invariant
- [ ] Background as polygons rather than points: superseded by [lidar-l7-scene-plan](lidar-l7-scene-plan.md)

### Accepted residuals (no action planned)

- [ ] `s2_sf_5` and `s2_sf_6` may ship without a background layer
- [ ] Motion segments carry no single S2 filename tag; they may cross cells
- [ ] The 116.8 m/s maximum track speed in the current DB is noise; the scorer penalises it rather than the pipeline rejecting it
