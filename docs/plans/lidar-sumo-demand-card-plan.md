# SUMO demand card for published surveys (v0.7.8)

- **Status:** Parked (D-32): the card publishes with the surveys, which wait on the scene asset move and on which scene data may be public (D-34); revisit when D-34's step 2 is settled or survey publishing is next. Was: Draft. The offline comparison tools in `tools/sumo/` are built
- **Decisions:** D-18 (nearest-rank percentiles), D-28 (following metric names, which this card does not use), D-32 (planning limits), D-34 (scene assets leave the repository; the export and its privacy test stay)
- **Layers:** Cross-cutting (scene export, site metadata, offline tooling, survey site)
- **Target:** v0.7.8 (survey publishing); nothing here ships before the surveys it describes
- **Companion plans:** [lidar-scene-catalogue-publishing-plan](lidar-scene-catalogue-publishing-plan.md) owns publishing and hosting, [lidar-web-scene-export-plan](lidar-web-scene-export-plan.md) owns the export format, [lidar-behaviour-analytics-plan](lidar-behaviour-analytics-plan.md) owns following metrics
- **Canonical:** [Pipeline ownership](../lidar/architecture/lidar-pipeline-reference.md)
- **Evidence:** [counting-line headways at Columbus and Broadway](../lidar/operations/sumo-columbus-broadway-headways-2026-10.md)

## Motivation

A SUMO user can build a San Francisco road network from OpenStreetMap in ten minutes. What they
cannot get is how real traffic arrives at a junction: flows by lane, the spacing of vehicles
leaving a queue, and the gaps between platoons. Our published surveys measure exactly that, but
only as a 3D player and a frame stream in the sensor's own coordinates. Anyone who wants to
calibrate a simulation against them has to repeat the work of the evidence report: find the
travel directions, identify the streets, locate the stop line and lanes, and turn tracks into
counting-line events.

The card does that once per survey, beside the survey, in SUMO's own file formats. Without it the
surveys stay a demonstration rather than a dataset, and the measurement that would be most
useful to traffic researchers is the hardest to reach.

## Current state

- **Published surveys.** 25 scenes in `public_html/src/scenes/<site>/assets/`: a manifest, one
  part's `header.json` (with the absolute `start_ns`), 60 s chunks of per-frame tracks at 5 Hz in
  sensor-local metres, a 5 s timeline and a background (types in `internal/scene/types.go`).
  Site metadata in `public_html/src/_data/scenes.json` and `tools/s2-archive/site-index.json`:
  approximate latitude and longitude, grid and north azimuth, posted limit, static parts.
- **Hugging Face dataset.** `banshee-data/sf-street-speeds` reserves `derived/tracks/` and
  `derived/metrics/`, both empty; the licence is not declared
  (`tools/s2-archive/huggingface-dataset-README.md`).
- **Comparison tools.** `tools/sumo/lidar-counting-line.py` reads a published scene into
  counting-line headways; `tools/sumo/columbus-broadway/` builds one junction in SUMO and runs
  three car-following models against it.
- **Evidence.** At Columbus and Broadway the stop-line p15 headway (2.4 s) is reproduced by IDM
  and EIDM, not Krauss; the gaps between platoons are not reproduced by any model on a guessed
  signal; flow matches and the lane split is reversed. The site's north azimuth appears about
  18° off. One site, 28.6 minutes.
- **Not built.** Lane assignment, signal phase inference, stop-line location, links from a
  survey's approaches to OSM ways, and crossing events in the export.

## Findings

| Area                    | Current state                                                          | Severity | Release view |
| ----------------------- | ---------------------------------------------------------------------- | -------- | ------------ |
| Approach geometry       | Inferred by hand from track directions and OSM bearings                | High     | v0.7.8       |
| Site azimuth            | Operator-read; 18° off at the one site checked                         | High     | v0.7.8       |
| Lanes and stop line     | Inferred by hand from lateral and stopped-position histograms          | Medium   | v0.7.8       |
| Split tracks            | 15 of 145 stop-line crossings were duplicates at Columbus and Broadway | Medium   | v0.7.8       |
| Signal timing           | Not inferable from one approach; SUMO's guess misses the platoon gaps  | Medium   | Deferred     |
| Absolute time in export | `header.json` publishes `start_ns`                                     | High     | D-34         |
| Licence                 | Not declared for the dataset; an OSM-derived network is ODbL           | High     | D-34         |

## Design

### What a card holds

One `sumo/` folder per survey part, next to `part-000/`, with a `sumo` block in the site's
`scenes.json` entry pointing at it.

| File                     | Contents                                                                                           | Tier |
| ------------------------ | -------------------------------------------------------------------------------------------------- | ---- |
| `card.json`              | Site, window, approaches and their OSM ways, counting lines, lanes, summary statistics, provenance | 1    |
| `network-recipe.sh`      | The `netconvert` call: bounding box, type overrides, flags, and the OSM snapshot it was built from | 1    |
| `counting-lines.add.xml` | Per-vehicle loops at the measured counting lines, on the recipe's edges                            | 1    |
| `flows.rou.xml`          | Statistical demand: 5-minute flows per approach and lane, headway model parameters                 | 1    |
| `score.py`               | Reads a SUMO run's loop output and prints each alignment metric beside the measured value          | 1    |
| `crossings.parquet`      | One row per counting-line crossing: relative time, approach, lane, speed, binned length            | 2    |
| `demand.rou.xml`         | Replay demand: each measured vehicle at its relative time, lane and speed                          | 2    |

The network itself is not shipped: built from OSM it is a derived database under the ODbL, and the
recipe with a pinned snapshot reproduces it. Edge IDs come from OSM way IDs, so the card names the
snapshot that its counting lines and routes refer to.

### `card.json` fields

| Field           | Meaning                                                                                |
| --------------- | -------------------------------------------------------------------------------------- |
| `site`, `title` | As in `scenes.json`                                                                    |
| `window`        | Day type and hour bucket, static minutes, never an absolute time                       |
| `approaches[]`  | Name, compass bearing, sensor-frame bearing, OSM way IDs, SUMO edge, lane count        |
| `lines[]`       | Approach, along position in metres, role (stop line, exit), SUMO lane and position     |
| `lanes[]`       | Approach, lane index (0 is the kerb lane), lateral band in metres                      |
| `stats[]`       | Per line and lane: count, flow, p15, p50, p85, share under 2.5 s, bootstrap intervals  |
| `speed`         | Per approach: p50 and p85 of vehicle maximum speeds (D-18), posted limit               |
| `signal`        | Inferred cycle if established, else null with the reason                               |
| `provenance`    | `source_vrlog_sha256`, `tuning_hash`, `build_version`, OSM snapshot hash, tool version |
| `licence`       | The dataset's declared licence, and ODbL for anything derived from OSM                 |

### Alignment metrics

`score.py` reports each metric beside its measured value and interval, so a user can see which
part of a scenario is off:

| Metric                             | Tests                      | Measured from        |
| ---------------------------------- | -------------------------- | -------------------- |
| GEH of 5-minute counts per lane    | Demand                     | Crossing counts      |
| Lane shares per approach           | Demand and lane choice     | Crossing lanes       |
| p15 headway at the stop line       | Car-following (discharge)  | Stop-line crossings  |
| p85 headway and share under 2.5 s  | Signal timing and arrivals | Stop-line crossings  |
| Speed p50 and p85 at the exit line | Desired speed              | Track maximum speeds |

### Three levels of use

1. **Statistical.** `flows.rou.xml` reproduces counts and lane shares, and the user calibrates
   `accel` and `tau` against the stop-line p15. Tier 1 only.
2. **Replay.** `demand.rou.xml` reproduces the measured arrival sequence upstream, so platoon
   structure is right by construction and only the junction and downstream behaviour are
   tested. Needs Tier 2.
3. **Calibration.** Signal timing and car-following parameters are searched until the scored
   metrics fall inside the measured intervals. Needs real signal timing.

### Privacy

The card must disclose no more than the export it is built from, and should disclose less. What
makes a track linkable to a person, most to least: absolute time with place; trips that start or
end in view (a driveway or a doorway); rare cells, such as one vehicle of unusual dimensions;
pedestrians and cyclists; linking the same vehicle across sites recorded the same day; and
per-vehicle speed attribution.

| Mitigation                                        | Applies to | Cost to a SUMO user          |
| ------------------------------------------------- | ---------- | ---------------------------- |
| Relative time, day type and hour bucket only      | All tiers  | None                         |
| Cells under five vehicles suppressed              | Tier 1     | Low                          |
| Length binned to 0.25 m                           | Tier 2     | Low for car-following        |
| Crossings only: no positions between lines        | Tier 2     | Replay starts at a line      |
| Vehicles only; pedestrians and cyclists as counts | All tiers  | Moderate for safety research |

Tier 1 is aggregates and can follow the surveys as soon as a licence is declared. Tier 2 is what
a loop detector or a radar speed sign records, one row per vehicle with no identity, and the
radar side of the project already stores per-vehicle speeds. D-34 plans an export test that
refuses personal fields and trajectories; whether per-vehicle crossing events fall inside that
rule is the owner's decision, and Tier 2 waits for it. The published tracks export already
carries per-vehicle positions at 5 Hz and an absolute start time, so the same decision should
cover it.

## Scope

### Item 1: site approach geometry

**Summary:** Record each survey's approaches, lanes, stop lines and counting lines as site
metadata, from the method the evidence report used.

**Steps:**

1. Turn `lidar-counting-line.py`'s `modes` and `profile` into a proposal per site, reviewed by a
   person: approach bearings matched to OSM ways by signed crossing angle, lane bands, queue front.
2. Store the reviewed geometry beside the site's static parts and azimuth.
3. Check every site's `north_azimuth_deg` against its matched OSM bearings and flag any off by
   more than 5°.

**Milestone:** v0.7.8

### Item 2: counting-line crossings in the export

**Summary:** `velocity scene export` writes per-line crossings, so the card is built from the
same code and privacy test as the rest of the export (D-34).

**Steps:**

1. Interpolated crossings per line and lane, split-track duplicates under 0.8 s dropped, with the
   count dropped recorded.
2. Relative time only; length binned; vehicles only.
3. The export privacy test covers the crossings file.

**Milestone:** v0.7.8

### Item 3: card generator and scorer

**Summary:** One command builds a survey's `sumo/` folder from the crossings, the site geometry
and a pinned OSM snapshot, and `score.py` compares any SUMO run with it.

**Steps:**

1. Generalise `tools/sumo/columbus-broadway/` to read the site geometry, not hand-set edges.
2. Fit the headway model per lane for `flows.rou.xml` and emit Tier 1 files.
3. Emit Tier 2 files only when the owner's D-34 decision allows them.
4. Test on Columbus and Broadway against the evidence report's figures, then one more junction.

**Milestone:** v0.7.8

### Item 4: signal timing

**Summary:** Give the card a signal cycle, from the city's timing records or from inference over
all of a junction's approaches together.

**Steps:**

1. Ask SFMTA for the timing cards of the surveyed junctions.
2. Failing that, infer green onsets from queue discharges on every approach at once, and publish a
   cycle only when its fold is unambiguous.

**Milestone:** Deferred

## Dependencies

- D-34's step 2: what survey data may be public, which gates Tier 2 and any publishing at all.
- A declared licence for the dataset.
- The scene asset store D-34 creates, where the card would live beside its survey.
- Split-track rates from the [duplicate tracks](../lidar/operations/l5-duplicate-tracks-2026-10.md)
  work: fewer splits mean fewer dropped crossings.

## Risks

| Risk                                               | Likelihood | Impact | Mitigation                                                               |
| -------------------------------------------------- | ---------- | ------ | ------------------------------------------------------------------------ |
| Per-vehicle crossings judged personal under D-34   | Medium     | Medium | Tier 1 stands alone; Tier 2 is separable                                 |
| OSM edits change way IDs and break a card's edges  | High       | Low    | Pin the snapshot and its hash in every card                              |
| Short windows give wide intervals                  | High       | Medium | Publish intervals, never bare percentiles                                |
| Kerb-lane occlusion biases kerb-lane headways long | Medium     | Medium | Report per lane; flag lanes the sensor sees through another              |
| A card is read as a validated measurement          | Medium     | High   | State in `card.json` and on the page that it is not physically validated |

## Checklist

### Complete

- [x] Evidence report at one junction, with matched-parameter comparison of three models
- [x] Offline tools: `tools/sumo/lidar-counting-line.py` and `tools/sumo/columbus-broadway/`

### Outstanding

- [ ] Owner decides whether per-vehicle crossings may be public (D-34)
- [ ] Dataset licence declared
- [ ] Item 1: site approach geometry and azimuth check
- [ ] Item 2: crossings in the export, under its privacy test
- [ ] Item 3: card generator and scorer, tested on two junctions
- [ ] Item 4: signal timing
