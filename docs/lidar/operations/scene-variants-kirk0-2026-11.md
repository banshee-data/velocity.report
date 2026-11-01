# What the solid-body configs draw on kirk0, on main: November 2026

geom0014 replays 30 s of kirk0 under the shipped defaults and the four solid-body parameter sets,
exports each run as a web scene with the scene-variant tooling, and reads the exported frames: the
boxes and headings a published scene draws. It repeats on main a run the tooling's first use made
on October 11 at build `0dd608f`, whose observations were kept on a private page and never
recorded here. Every one of these files has changed since then: the defaults gained the published
box model (`box_model`) and the track guard, and L3 was retuned.

- **Status:** Measured, November 1, 2026 (experiment
  [geom0014](../../../data/experiments/geom/geom0014.md)). Nothing shipped changes
- **Layers:** L5 tracking and the published box, as the web scene draws them
- **Related:** [scene variant exports plan](../../plans/lidar-scene-variant-exports-plan.md)
  ([first run](../../plans/lidar-scene-variant-exports-plan.md#first-run-against-a-server)),
  [rectangle heading screen](solid-body-rectangle-heading-2026-10.md),
  [replay determinism](replay-determinism-2026-10.md)

## Summary

- **The shadow draws the defaults' tracks exactly.** In all 151 exported frames, every box the
  solid-body shadow draws has the defaults' position, extents, heading, speed and class. Only the
  export-local track ids differ. Its `tuning_hash` differs from the defaults', so the file was
  applied and still changed nothing, which is the shadow's contract. Rule 1 passes.
- **A replay carries nothing from the ones before it.** The defaults, replayed again on the same
  server process after four other configs (the last of them the convergence candidate), export
  the same tracks as the first defaults replay in every frame. The October run had no such check
  and predates the replay-history fixes of perf0002 and l3bg0021. Rule 2 passes.
- **The arms whose solid body drives the track still turn the box less.** The median heading
  change between consecutive frames is 0.23° under A2 and 0.34° under alignment and convergence,
  against 0.80° under the defaults. The 90th percentile is 4.8° to 5.6° against 7.9°. October's
  direction holds (rule 3).
- **On main the same three arms also halve the length spread, which October did not show.** The
  median per-track standard deviation of box length is 0.49 to 0.52 m under the three driving
  arms, against 0.97 m under the defaults. In October every config sat near 1.2 m. Width spread
  is 0.26 to 0.31 m for every config, half of October's or less, and no arm stands out.

## Method

The question, the rule and the arms were committed in
[geom0014](../../../data/experiments/geom/geom0014.md) before any replay.

- **Capture and window.** kirk0 (`internal/lidar/perf/pcap/kirk0.pcapng`, the Kirkham and 9th
  placement of December 5, 2025), from 45 s for 30 s: the window the October run used. Each
  replay starts 40 s earlier, at 5 s, so the export can drop its 400 transient frames, and runs
  70 s in all.
- **Settings.** The published replay settings on main: scaled at 0.5, stride 2, the background
  settled before recording (`SCENE_SETTLE=1`, which October ran without), corpus source, the
  online trail. Each export holds 151 frames over 30.0 s.
- **Tooling.** `tools/s2-archive/export-scene-variants.py` against one scratch server built with
  `make dev-go-lidar` at the spec's commit, with a fresh database. Each arm's file was applied
  through `tuning_config` on `POST /api/lidar/pcap/start`, and the server echoed the fingerprint
  it lists for every one.
- **Order.** The defaults, the shadow, A2, alignment and convergence as one set, then the defaults
  alone as a second set on the same process. The second set is needed because the tool exports
  two variants that share a file from one recording.
- **Figures.** Read from each export's frames, the data the scene player draws. Per arm: the track
  ids, the track-frames, the tracks lasting at least 3 s (15 frames), and the moving ones among
  them (peak speed at least 3 m/s). Over the moving tracks: the median across tracks of each
  track's standard deviation of box length and of width, and the median and 90th percentile of
  the absolute heading change between consecutive frames of the same track.
- **Equality.** Two exports are equal when every frame has the same timestamp and the same set of
  track records once the id is removed, compared exactly as exported: positions and extents to
  0.01 m, headings to 0.001 rad.

October's sixth config, `config/tuning.optimised.json`, is not an arm. It is not a parameter set
under `config/experiments/`, and it still carries L3 values the defaults have since left, so a
difference from it would not say which change made it.

## Arms

| Arm         | Parameter set                                            | Changes from the defaults                                                                                                                                                        | Fingerprint    |
| ----------- | -------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------- |
| defaults    | `config/tuning.defaults.json`                            | None                                                                                                                                                                             | `e44f577dc1df` |
| shadow      | `config/experiments/solid-body-shadow.json`              | Solid body as a shadow beside the tracked filter: face hysteresis, course-aligned faces, full members                                                                            | `335d8f629548` |
| a2          | `config/experiments/solid-body-a2.json`                  | The shadow's options plus near-edge tracking, so the solid body drives the tracked state                                                                                         | `136f127a78e1` |
| alignment   | `config/experiments/alignment-candidate.json`            | A2 with course heading, extent growth admission, open-prior end-face centring, the vehicle extent floor, containment                                                             | `be11b8bb7231` |
| convergence | `config/experiments/geometry-convergence-candidate.json` | A2 with extent growth admission, open-prior end-face centring, the vehicle extent floor, containment, the rectangle heading with course fusion, the rectangle fit's sigma at 1.5 | `68b2d40b8a5c` |

Every arm carries the defaults' `box_model` and track guard: the parameter sets differ from the
defaults only in their `solid_body` block.

## Findings

### The rule

| Clause                                                   | Result                                                 | Verdict |
| -------------------------------------------------------- | ------------------------------------------------------ | ------- |
| 1. The shadow's tracks equal the defaults'               | 151 of 151 frames equal; `tuning_hash` differs         | Pass    |
| 2. The repeat's tracks equal the first defaults'         | 151 of 151 frames equal, after the convergence arm     | Pass    |
| 3. A2, alignment and convergence turn less than defaults | 0.23°, 0.34° and 0.34° against 0.80°, median per frame | Holds   |

### The figures on main

| Arm         | Track ids | Track-frames | ≥ 3 s | Moving | Length spread | Width spread | Turn, median | Turn, p90 |
| ----------- | --------: | -----------: | ----: | -----: | ------------: | -----------: | -----------: | --------: |
| defaults    |       136 |        1,578 |    25 |     15 |        0.97 m |       0.29 m |        0.80° |      7.9° |
| shadow      |       136 |        1,578 |    25 |     15 |        0.97 m |       0.29 m |        0.80° |      7.9° |
| a2          |       140 |        1,587 |    29 |     19 |        0.49 m |       0.27 m |        0.23° |      5.6° |
| alignment   |       137 |        1,588 |    30 |     17 |        0.51 m |       0.31 m |        0.34° |      5.4° |
| convergence |       131 |        1,637 |    30 |     17 |        0.52 m |       0.26 m |        0.34° |      4.8° |

The repeated defaults replay's figures are the defaults' row, as rule 2 requires.

Under the three driving arms, two or three tracks end classed as a bus, which no track is under
the defaults or the shadow. kirk0's labels were not read, so whether those are the capture's
trucks is not known. The convergence candidate has the fewest track ids and the most
track-frames.

### Against October

October's run was build `0dd608f` with the configs as they were then, without settling before
recording, exporting 153 frames.

| Arm         | Length spread, Oct → Nov | Width spread, Oct → Nov | Turn median, Oct → Nov | Turn p90, Oct → Nov |
| ----------- | -----------------------: | ----------------------: | ---------------------: | ------------------: |
| defaults    |            1.20 → 0.97 m |           0.62 → 0.29 m |          0.69° → 0.80° |         9.3° → 7.9° |
| shadow      |            1.20 → 0.97 m |           0.62 → 0.29 m |          0.69° → 0.80° |         9.3° → 7.9° |
| a2          |            1.15 → 0.49 m |           0.74 → 0.27 m |          0.40° → 0.23° |         6.3° → 5.6° |
| alignment   |            1.20 → 0.51 m |           0.68 → 0.31 m |          0.40° → 0.34° |         7.3° → 5.4° |
| convergence |            1.29 → 0.52 m |           0.58 → 0.26 m |          0.40° → 0.34° |         7.0° → 4.8° |

## Interpretation

The two equality checks are the firmest result. The shadow behaves as a shadow on main, and a
replay no longer depends on what the server replayed before it, at least across these five
configs on kirk0. A set's arms can therefore be compared without restarting the server between
them, which is how the tooling runs.

The comparison with October is not controlled. Between the two runs the defaults gained the
published box model (a rectangle-fit axis, a 3.8 m vehicle length floor and an extent window),
the track guard and an L3 retune, and published scenes began settling before recording. Width
spread halving under every arm fits the box model, which every arm inherits, but this run does not
isolate it.

A track's length spread over its life is mostly the box growing as more of the vehicle comes into
view, which is the convergence the solid body aims at. A lower spread under the driving arms fits
a box that reaches its length sooner, and equally fits a box that is held short. This run cannot
tell those apart; the labelled geometry screens can.

## What this does not establish

- **Accuracy.** kirk0's labels were not read. A steady box is not a correct one, and a replay
  measures what the pipeline drew, not where the vehicles were.
- **Paired differences.** Track ids are export-local and are not matched across arms, and the
  moving tracks are a different set under each arm (15 to 19), so the medians are over different
  vehicles.
- **Generality.** One capture, 30 s, fewer than twenty moving tracks per arm.
- **The cause of the change since October.** Several changes landed together; see Interpretation.
- **The geometry-convergence set.** Its end-on truck windows on 1st-mission and 3rd-folsom need
  captures that are on the operator's machine and were not available to this run.

## Recommendations

- **Quote this report, not the October page,** for how the solid-body arms draw on kirk0. The
  October figures describe a build and configs that no longer exist.
- **Run the geometry-convergence set where its captures are,** and read the length-spread effect
  on the end-on trucks it was chosen for, alongside the labelled screens that can say whether the
  shorter spread is earlier convergence or a box held short.
- **Make a repeated baseline a built-in option of the variant tooling** (the plan's open item). It
  cost one replay here (2.8 min) and was exact. Because variants that share a file share a
  recording today, the option has to force a fresh replay.

## Provenance

- **Build:** `v0.5.1-pre50`, git `30458943f064` (main at `3b49a1ac` plus the geom0014 spec),
  built through `make` on November 1, 2026 at 05:15:54 UTC; the binary's stamp matched `HEAD`.
- **Capture:** kirk0, `internal/lidar/perf/pcap/kirk0.pcapng`, 200,657,872 bytes, Git LFS object
  `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`.
- **Sets:** scratch set files `kirk0-geom0014` (`sha256:756fce31f3b6…`) and
  `kirk0-geom0014-repeat` (`sha256:2f93114bfa37…`), both with the window above; neither problem
  list recorded anything.
- **Runs:** fingerprint (listed and echoed), `tuning_hash`, run id and source VRLOG `sha256`:

  | Arm                | Fingerprint    | `tuning_hash`   | Run         | VRLOG `sha256`  |
  | ------------------ | -------------- | --------------- | ----------- | --------------- |
  | defaults           | `e44f577dc1df` | `d948a67f4943…` | `1884b7ab…` | `64cfecad11ed…` |
  | shadow             | `335d8f629548` | `9197aa44a495…` | `0e906792…` | `ef35fcabfc49…` |
  | a2                 | `136f127a78e1` | `7215f0260d9f…` | `e5ef7557…` | `540428d23673…` |
  | alignment          | `be11b8bb7231` | `e5d00c29602a…` | `0138520c…` | `e1d620547628…` |
  | convergence        | `68b2d40b8a5c` | `5f97b22e774a…` | `15ab93fa…` | `c20a7102dcc4…` |
  | defaults, repeated | `e44f577dc1df` | `d948a67f4943…` | `9a0e47a1…` | `eaac3ffbff4b…` |

  The repeat's VRLOG hash differs from the first defaults' while its exported tracks are equal:
  the hash covers the VRLOG's header, and the two headers differ only in the time each was
  created.

- **Output:** the database, the six VRLOGs and the exports were written to a Claude Code cloud
  session's scratch directory and are not kept: the container is reclaimed when the session ends.
  Every figure the rule was applied to is in this report.
- **October:** set `kirk0-renders` (`sha256:b385683a3464…`), build `0.5.1-pre45`, git `0dd608fd`,
  six configs on one server at settle off, 153 frames per export.
- **Run time:** 2.8 min per arm, 16.5 min for both sets, against the spec's 3 min an arm.
