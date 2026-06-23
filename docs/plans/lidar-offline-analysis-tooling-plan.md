# LiDAR offline analysis tooling — parked capabilities

- **Status:** Future work (parked). Not currently built or exposed.
- **Canonical:** [pcap-analysis-mode.md](../lidar/operations/pcap-analysis-mode.md)

## Why this exists

The offline LiDAR tooling was reduced to the three capabilities that are actually
used day to day — **scan** (capture/health stats), **motion stats** (the
motion/static timeline), and **splits** (segment PCAPs) — all served by
`velocity lidar pcap-split`, plus the unchanged `settling-eval` and the dev/CI
`lidar-bench` perf gate. See the reduction itself in the git history around the
removal of `internal/lidar/pcapanalyse` and `cmd/tools/pcap-analyse`.

Three capabilities that `pcap-analyse` carried were dropped because nothing
automated consumed them. They are recorded here as **self-contained architecture
outlines** so each can be rebuilt from this document alone — the team
squash-merges to `main`, so "recover the old file from a branch SHA" is not a
reliable plan.

The shared seam for all three is the offline L1–L6 pipeline run, which still
exists: `internal/lidar/lidarbench` builds the same background model and runs
foreground → DBSCAN (L4) → Kalman tracking (L5) → classification (L6) per frame,
exactly as the live server does, purely to benchmark it. Reviving any capability
below means tapping that pipeline run (its `tracker.GetConfirmedTracks()` and the
per-frame foreground mask) and adding an output stage — no new perception code.

---

## 1. Offline perception / track analysis (CSV + JSON export)

**Purpose.** Turn a capture into a per-track table for offline study: how many
vehicles/pedestrians, their speeds, sizes, and trajectories. This was
`pcap-analyse`'s default output (`-csv` / `-json`).

**Inputs.**

- A PCAP and a tuning config (the same `-config` / sensor / `-port` plumbing
  `pcap-split` uses; auto-detect the port when omitted).
- The confirmed tracks from one pipeline pass: `tracker.GetConfirmedTracks()`
  after the read completes (the tracker is an `l5tracks.Tracker`; classification
  is applied by an `l6objects.TrackClassifier` once a track has ≥5 observations).

**Processing.**

1. Run the pipeline pass (reuse the `lidarbench` frame builder, which already does
   foreground → cluster → track → classify; add a non-benchmark entry that keeps
   the tracker rather than discarding it).
2. For each confirmed track, fold its observation history into a summary row.
3. Aggregate per class and over the whole population for the summary block.

**Output schema.**

- **`tracks.csv` / `tracks` (JSON array)** — one row per confirmed track:
  `track_id, class, confidence, start_time, end_time, duration_secs,
observations, avg_speed_mps, max_speed_mps, avg_height_m, avg_length_m,
avg_width_m, height_p95_max_m, start_x_m, start_y_m, end_x_m, end_y_m,
total_distance_m`.
- **Per-class distribution** — `{class: {count, avg_speed_mps,
avg_duration_secs, avg_observations}}`.
- **Speed statistics** — population `min/max/avg/p50/p85/p98` of per-track max
  speeds. Use floor-based percentile indexing to match
  `l6objects.ComputeSpeedPercentiles`; the speed percentiles are aggregate over a
  population of vehicle max speeds, **not** per-observation (see
  [coding-standards](../../.github/knowledge/coding-standards.md)). Note the p85
  → p98 high-end alignment tracked in
  [data-completeness-remediation.md](../lidar/operations/data-completeness-remediation.md).

**Integration seam.** A new `--export-tracks csv|json` flag on `pcap-split`, or a
small dedicated `lidar-analyse` tool over the `lidarbench` pipeline. Either way it
consumes `GetConfirmedTracks()`; no clustering/tracking code is re-implemented.

**Related.** [lidar-pipeline-reference.md](../lidar/architecture/lidar-pipeline-reference.md),
[clustering-diagnostics.md](../lidar/operations/clustering-diagnostics.md).

---

## 2. ML training-data export (foreground blobs)

**Purpose.** Emit labelled per-frame foreground point sets for offline classifier
training. This was `pcap-analyse -training`.

**Inputs.** Same pipeline pass as §1, plus, per frame: the extracted foreground
points (`l3grid.ExtractForegroundPoints(points, mask)`), the cluster count, and
the active-track count at that frame.

**Processing.** On each frame with foreground, capture a training record. Encode
the foreground point set as a compact binary blob (the prior implementation used
`adapters.EncodeForegroundBlob`; re-establish or replace that encoder). Sampling
can be every-frame or every-Nth to bound output size.

**Output schema.** One record per sampled frame:
`frame_id, timestamp, sensor_id, total_points, foreground_points, clusters,
active_tracks` plus the binary `foreground_blob` (written as a sidecar/length-
prefixed stream, not inlined in JSON). Define a small container format
(e.g. JSONL index + a blob file) so a training pipeline can stream it.

**Integration seam.** A `--export-training DIR` flag hanging off the same pipeline
pass; it needs the per-frame foreground mask, so it taps the frame builder's
`processCurrentFrame`, not just the final tracks.

**Related.** [lidar-ml-classifier-training-plan.md](lidar-ml-classifier-training-plan.md).

---

## 3. SQLite track persistence

**Purpose.** Persist analysed tracks + run metadata to a SQLite DB for querying
across many captures. This was `pcap-analyse -db`.

**Inputs.** The confirmed tracks from §1 and run-level metadata (input file,
duration, frame/packet/point counts, tuning identity, timestamp).

**Processing.** Open (or create) the DB, upsert a `run` row, then insert one row
per track keyed to the run. Reuse the project's `internal/db` layer and the
`modernc.org/sqlite` driver (the only permitted SQLite driver — see
[coding-standards](../../.github/knowledge/coding-standards.md)); follow the
JSON-first schema convention (store the track record as JSON with generated
columns for the indexed fields: `track_id`, `class`, `max_speed_mps`).

**Output schema.** Two tables:

- **`analysis_run`** — `id, input_file, started_at, duration_secs, total_frames,
total_packets, total_points, tuning_sha, created_at`.
- **`analysis_track`** — `id, run_id (FK), track_id, class, payload (JSON of the
§1 row)` with generated columns `class`, `max_speed_mps` for indexing.

Add a forward migration under `internal/db/migrations/`; use `DROP COLUMN`
directly if iterating (SQLite 3.35+).

**Integration seam.** A `--db PATH` flag on the §1 exporter; persistence runs
after the pipeline pass and the in-memory track summary are built, so it depends
on §1, not on perception internals.

---

## Sequencing

§1 (track analysis) is the prerequisite: §2 and §3 both consume its pipeline pass
and track summary. Recommended order: §1 → §3 (cheap, reuses §1 rows) → §2
(needs the per-frame foreground encoder). None of the three is required for the
current scan/motion/split workflow; build them only when a concrete consumer
appears (a training run, a cross-capture query, a perception study).
