# Background grid and region overlays in recordings and viewers

- **Status:** Draft
- **Layers:** L3 Grid, L9 Endpoints (proto, VRLOG), L10 Clients (macOS visualiser and annotation window first; web as an optional view)
- **Target:** v0.5.x, after the annotation toolset lands; the overlay is what makes an L3 remedy testable
- **Companion plans:** [Review workflow](lidar-review-workflow-plan.md), [Occupancy column grid](lidar-occupancy-column-grid-plan.md), [Web scene export](lidar-web-scene-export-plan.md), [Point annotation](lidar-point-annotation-and-object-dataset-plan.md)
- **Canonical:** [adaptive-region-parameters.md](../lidar/operations/adaptive-region-parameters.md)

## Motivation

At Columbus and Broadway the background model settles while northbound Columbus traffic is
queued at the light. The cells covering that queue learn the stopped cars as background. When
the light changes and the queue moves, the returns that come back from those cells are the
parked cars behind: farther than the learned surface, stable, and wrong according to the model.
They are reported as foreground for the rest of the run, as a band of persistent noise clusters
and short tracks along the kerb.

An operator watching the replay sees the noise and cannot see the cause. Nothing on screen says
which cells settled on a car, which region they were assigned to, what parameters that region
runs with, or whether the regions were identified in this run or restored from an earlier one.
When the background does change, nothing shows that either. The consequence of leaving this
alone is that every L3 remedy is judged by its effect on track counts, several layers
downstream, rather than by whether the cells it was meant to fix were fixed.

This plan is about seeing the background model. It does not change how the model behaves. The
remedies are listed at the end so that the overlay is designed to evaluate them, and they belong
to an L3 experiment, not to this work.

## Current state

Verified against `main` plus the annotation branch on 2026-09-20.

**The model.** `internal/lidar/l3grid/background.go` holds a polar grid of `BackgroundCell`.
Each cell carries `AverageRangeMeters`, `RangeSpreadMeters`, `TimesSeenCount`,
`LastUpdateUnixNanos`, `FrozenUntilUnixNanos`, `RecentForegroundCount`, and a locked reference:
`LockedBaseline`, `LockedSpread`, `LockedAtCount`. A `Region` is a cell mask with its own
`RegionParams` (`noise_relative_fraction`, `neighbor_confirmation_count`,
`settle_update_fraction`) and the `MeanVariance` it was formed from.

**Regions are identified once.** `RegionManager.IdentifyRegions` runs at the end of settling from
per-cell variance and sets `IdentificationComplete`. There is no re-identification path.

**Regions outlive the run.** `TryRestoreRegionsByGridHash` restores a saved region snapshot, and
its linked grid snapshot, when the scene signature matches. A settle that went wrong is therefore
saved and re-applied to later runs of the same scene. Pass 7 of the option-scorecard sweep carries
a `no_region_overrides` configuration for this reason, and it moved track counts at all 24 sites.

**After settling the background does not learn.** `PostSettleUpdateFraction` defaults to 0. No
rule treats a return beyond the baseline differently from one in front of it: a search of
`foreground.go`, `background.go` and `background_closeness.go` for such handling finds none.

**What a recording carries.** `FrameBundle.background` is a `BackgroundSnapshot`: Cartesian
`x`, `y`, `z`, a `confidence` array holding `TimesSeenCount`, and `GridMetadata` (rings, azimuth
bins, ring elevations, `settling_complete`). It has no cell index, region ID, baseline, spread,
lock or freeze state. The publisher sends it every 30 s or when `sequence_number` changes, and
that number only increments on a grid reset. `FRAME_TYPE_DELTA` is reserved in the proto and
unused.

**What exists outside recordings.** `/debug/lidar/background/regions` and its dashboard serve
`RegionDebugInfo` (regions, parameters, the cell-to-region map) from a live server. None of it
is recorded, so none of it can be replayed, compared between runs, or shown beside the frame it
explains.

**Viewers.** The macOS renderer draws background points with a crossfade when a new snapshot
arrives, behind a `showBackground` toggle, in one colour. The web scene player loads one
voxel-downsampled settled snapshot per scene (`internal/scene/background.go`). An annotation
pack carries points only.

## Findings

| Area                                | Current state                                   | Severity | Release view                                       |
| ----------------------------------- | ----------------------------------------------- | -------- | -------------------------------------------------- |
| Cause of persistent kerb-side noise | Invisible in every viewer and every recording   | High     | Blocks evidence-based L3 tuning                    |
| Region provenance                   | Identified or restored is not recorded anywhere | High     | A bad settle propagates silently between runs      |
| Background change over time         | At most one refresh per 30 s, positions only    | Medium   | Re-lock and freeze behaviour cannot be observed    |
| Live region debug endpoint          | Works, but only against a running server        | Medium   | Not reproducible, not attachable to a run          |
| Cell identity in the snapshot       | Points are Cartesian; the cell index is lost    | Medium   | Overlay data cannot be joined to points without it |
| Web background                      | Voxelised, single snapshot                      | Low      | Adequate for presentation, not for diagnosis       |

## Design / approach

### Principle

Record the background model's state in the same stream as the frames it explains, indexed by
cell, with its changes as events. Every viewer then draws from the recording.

The macOS visualiser is where this is diagnosed, as it is where everything else in the
[review workflow](lidar-review-workflow-plan.md) is done. The overlay is complete there first,
in the main view and the annotation window, and the acceptance below is met without a browser.
A web view of the same data may follow for showing a finding to someone else. Nothing depends on
it.

### 1. Cell-indexed grid state in the proto

Add `BackgroundGridState` beside `BackgroundSnapshot`, as a new optional `FrameBundle` field. It
is indexed by cell (`ring * azimuth_bins + azimuth_bin`), not by position, so it joins to any
point whose ring and azimuth are known and survives a change of world frame:

- `region_id` per cell, with a reserved value for unassigned
- `baseline_range_m` and `spread_m` per cell, from the locked reference
- `times_seen` per cell
- `state` per cell, a bitfield: settled, locked, frozen, recently foreground
- `last_update_age_ms` per cell
- `regions`: one summary per region with its ID, parameters, cell count, mean variance, and
  provenance (`identified`, `restored` with the scene signature it matched, or `override`)

`BackgroundSnapshot` also gains a `cell_index` array parallel to `x`, `y`, `z`. That is the join
the viewers need: a background point can then be coloured by anything in the grid state.

Grid state carries the `source_id` and `calibration_id` of the evidence databases, where a run
has them. The same capture under the same calibration then has comparable grid state across
runs, and a region restored from an earlier run names the source it was learned on, not only a
scene signature.

The snapshot stays as it is for old clients. New fields are additive and ignored by anything that
does not know them. The VRLOG recorder stores bundles through the proto codec, so recording
follows from emitting; that assumption is to be confirmed in step 1 rather than relied on.

### 2. Keyframes and deltas

A full grid state is rings x azimuth bins values per array, too large to send per frame and
wasteful to resend when nothing moved. Emit:

- a **keyframe** with every refresh of the background snapshot, so a player that seeks never has
  to apply more than one interval of changes
- a **delta** between keyframes, listing only changed cell indices and their new values, when a
  cell's baseline moves beyond a threshold, its state bits change, or its region changes; rate
  limited to about once a second. This is what `FRAME_TYPE_DELTA` was reserved for

The delta is what answers "when settled regions update we want to see updates too": a re-lock is
a delta carrying a handful of cells at the moment it happens, not a difference an operator has to
spot between two snapshots half a minute apart.

### 3. Grid events

A small `GridEvent` list on the bundle, for the moments an operator needs on a timeline:
`SETTLING_COMPLETE`, `REGIONS_IDENTIFIED`, `REGIONS_RESTORED` (with the matched signature),
`REGION_PARAMS_CHANGED`, `CELLS_RELOCKED` (with a count), `GRID_RESET`. Both viewers draw these
as ticks on the playback timeline. `REGIONS_RESTORED` near the start of a Columbus run is, on its
own, most of the diagnosis.

### 4. One diagnostic the model does not keep today

Add a per-cell count of recent returns **beyond** the baseline by more than the acceptance band:
`beyond_baseline_count`, saturating and decaying like `RecentForegroundCount`. It is observation
only; nothing in L3 reads it.

This is the overlay the Columbus case needs. A real background surface cannot be seen through, so
a stable return from behind it is evidence that the learned surface was an occluder. Cells that
settled on the queue light up when the queue leaves and stay lit. Glass, railings, foliage gaps
and rain produce the same signal intermittently, which is why this is a count to look at and not
a rule to act on.

### 5. A region layer that toggles, and the modes it has

The overlay is a **layer**, switched on and off independently of everything else on screen, and
a **mode** that says what the layer is coloured by. It is off by default, so a viewer looks as
it does today until someone asks. The toggle and the chosen mode are remembered per user. It
has a toolbar button and a single-key shortcut beside the existing point, background, box,
cluster, trail and velocity toggles, and a legend that appears with it. Turning it off restores
the plain background exactly; it never replaces the background toggle.

One set of modes, the same names in every viewer, applied by colouring background points through
`cell_index`:

| Mode            | Colours by                                                          | Answers                                      |
| --------------- | ------------------------------------------------------------------- | -------------------------------------------- |
| Region          | `region_id`, categorical, with a legend of each region's parameters | Which parameters govern this kerb?           |
| Confidence      | `times_seen`                                                        | Where is the model thin?                     |
| Spread          | `spread_m`                                                          | Where is the acceptance band wide?           |
| Staleness       | `last_update_age_ms`                                                | What has not been confirmed for a long time? |
| State           | locked, frozen, recently foreground                                 | What is the model doing right now?           |
| Beyond baseline | `beyond_baseline_count`                                             | Where is the background provably wrong?      |

Cells with no background point (never settled) get a wedge outline in a second pass, so an
unsettled sector is visible as absence rather than as nothing.

### 6. Where it is drawn, macOS first

- **macOS visualiser.** The region toggle and its mode picker beside the existing background
  toggle, a legend, and event ticks on the timeline. Selecting a background point shows its cell:
  ring, azimuth, region, baseline, spread, state, and the region's parameters. The renderer
  already keeps a background cache with a crossfade; deltas update that cache in place.
- **Annotation window and review packs.** A review pack carries the grid state as a context
  layer: the keyframe in force at each sample, and a `cell_index` for every pack point, outside
  the point digest. The annotation window gets the same toggle and modes over its orthographic
  views. A person deciding whether a smear of returns along the kerb is a parked car or noise can
  switch the region layer on, see that those cells settled at a nearer range, and switch it off
  again to lasso. See the [review workflow](lidar-review-workflow-plan.md).
- **Proposers and headless use.** `velocity lidar grid-report --vrlog <dir> --at <t>` writes the
  grid state at a time as JSON plus a top-down PNG per mode. In the review workflow, pass 1 reads
  the region and the beyond-baseline state of the cells under a track's path as evidence, and
  pass 2's dossier draws the trail over the region-coloured background. A "track" that never
  leaves cells the model has wrong is most of a noise proposal on its own.
- **Static snapshots.** An exported still carries the active overlay mode and its legend, so an
  image attached to a finding says what its colours mean.
- **Web scene, optional.** `velocity scene export --grid-state` writes the grid state into the
  existing chunk scheme as a keyframe per chunk plus deltas, and the player gains the same toggle,
  mode picker and ticks. The web background is voxelised, so the join is made at export: each
  voxel takes the cell of the point that represents it. This is a presentation of the diagnosis,
  not the evidence for it, and no step of the diagnosis requires it.

### Invariants

- Observation only. Emitting grid state must not change a single foreground decision; a replay
  with it on and off produces the same clusters and tracks, byte for byte.
- Cell-indexed, never position-indexed, so state survives calibration and frame changes.
- Old recordings open. A viewer shows "grid state not recorded" for them, never an empty overlay
  that reads as "no regions".
- The perf gate holds. Keyframe and delta assembly run off the frame path or within its budget;
  see [performance regression testing](../lidar/operations/performance-regression-testing.md).

## Scope

### Item 1: grid state, deltas and events in the proto and the recorder

**Summary:** Emit and record cell-indexed background state, with changes as deltas and events.

**Steps:**

1. Confirm the VRLOG recorder persists unknown-to-it bundle fields unchanged, with a round-trip
   test; if it does not, that is the first fix.
2. Add `BackgroundGridState`, `BackgroundGridDelta`, `GridEvent`, and `cell_index` on the
   snapshot; regenerate Go and Swift stubs with `make proto-gen`.
3. Build keyframes from the grid under its existing lock discipline; build deltas from a
   per-cell dirty set fed by the update path.
4. Add `beyond_baseline_count` to the cell, observation only.
5. Prove the invariant: replay kirk0 and columbus-broadway with emission on and off and compare
   cluster and track output digests.
6. Recapture perf baselines in the same change if the fingerprint moves.

**Milestone:** v0.5.x

### Item 2: macOS region toggle, modes, cell inspector and timeline ticks

**Summary:** A region layer that toggles independently, with six modes, a legend, a cell inspector and grid events in replay.

**Steps:**

1. Toolbar toggle, shortcut and mode picker; off by default; state remembered per user; turning
   it off restores the plain background exactly.
2. Carry `cell_index` through the background cache; colour by mode in the point shader.
3. Apply deltas to the cache in place, with the existing crossfade reserved for keyframes.
4. Cell inspector on background-point selection.
5. Event ticks on the playback timeline, with `REGIONS_RESTORED` distinct from `IDENTIFIED`.
6. "Not recorded" state for recordings without grid state: the toggle is disabled and says why.

**Milestone:** v0.5.x

### Item 3: region layer in review packs and the annotation window

**Summary:** Grid state as a pack context layer, with the same toggle over the orthographic views.

**Steps:**

1. Pack export writes the grid keyframe in force at each sample and a `cell_index` per pack
   point, outside the point digest, so adding it never invalidates existing annotations.
2. The annotation window's toggle and modes, sharing the legend and shortcut with the main view.
3. A pack without the layer disables the toggle and says why.

**Milestone:** v0.5.x

### Item 4: grid report for proposers and evidence

**Summary:** A headless report of the background model at a moment, as JSON and images.

**Steps:**

1. `velocity lidar grid-report` reading a VRLOG; JSON schema versioned beside the proto.
2. Top-down PNG per mode at a fixed metric scale with a scale bar and legend.
3. Per-track cell context for pass 1 of the review workflow.
4. Use it to write up the Columbus case as the acceptance evidence below.

**Milestone:** v0.5.x

### Item 5: web scene export and player, optional

**Summary:** Export grid state into scene chunks and draw the same modes in the web player, for showing a finding to someone else.

**Steps:**

1. `--grid-state` on `velocity scene export`; voxel-to-cell join at export time.
2. Keyframe per chunk plus deltas, within the existing chunk size budget; measure the size cost
   on columbus-broadway before choosing delta thresholds.
3. Toggle, mode picker, legend and ticks in `scene-player.js`, shared by the tracks page.
4. Legend baked into exported stills.

**Milestone:** Unscheduled. Nothing above depends on it.

## Acceptance

On the columbus-broadway capture, from the recording alone, in the macOS tool and with no browser
open, an operator can:

1. see at settling-complete which northbound cells locked at the queue's range, nearer than the
   returns that later come from those cells;
2. watch the beyond-baseline overlay light along the kerb when the queue moves;
3. tell from a timeline tick whether the regions were identified in this run or restored, and
   from which scene signature;
4. re-run with a candidate remedy and see the same cells re-lock as deltas and the overlay clear,
   or see that they did not.

## Dependencies

- The annotation toolset (PR #579) lands first; this branch is stacked on it.
- `make proto-gen` toolchain for Go and Swift.
- The run-track finalisation fix, so that "fewer noise tracks" downstream is measured on real
  track lifetimes rather than first-sighting rows.

## Risks

- **Recording size.** Keyframes every 30 s are bounded; deltas are not, on a scene that never
  stops changing (foliage, rain). Mitigate with thresholds, the rate limit and a per-frame cap
  that degrades to "too many changes, see next keyframe" rather than dropping silently.
- **Lock contention in L3.** Reading the whole grid for a keyframe competes with the update path.
  Copy under the existing snapshot path rather than adding a second reader.
- **An overlay that implies a verdict.** Beyond-baseline is evidence, not a classification. Glass
  and gaps produce it legitimately. The legend says so.
- **Two sources of truth.** The live `/debug` endpoint and the recorded state must be built from
  one function, or they will drift.

## Remedies this overlay is meant to evaluate

Hypotheses for a separate L3 experiment, not decisions made here:

- **Re-lock on sustained returns beyond the baseline.** The physical argument above, with a
  persistence requirement and neighbour confirmation to reject glass, gaps and rain.
- **A non-zero post-settle update fraction.** Pass 7 already includes `post_settle_update_0p002`.
- **A settle-quality gate.** Do not lock a cell whose settling samples are bimodal, and hold
  settling open across at least two signal cycles at a signalised site; see
  [settling time optimisation](../lidar/operations/settling-time-optimisation.md).
- **Provenance-aware restore.** Record settle quality with a saved region snapshot and refuse to
  restore a poor one by scene signature alone.

## Checklist

### Outstanding

- [ ] Recorder round-trip test for unknown bundle fields (`S`)
- [ ] Proto messages, `cell_index`, stubs regenerated (`M`)
- [ ] Keyframe and delta emission with the observation-only digest proof (`L`)
- [ ] `beyond_baseline_count` (`S`)
- [ ] macOS region toggle, modes, legend, cell inspector, ticks (`L`)
- [ ] Region layer in review packs and the annotation window (`M`)
- [ ] `grid-report` CLI and the Columbus write-up (`M`)

### Deferred

- [ ] Web export flag, player toggle and modes, still legends: optional, unscheduled (`L`)
- [ ] The L3 remedies above: a separate experiment plan once the overlay exists
- [ ] Wedge polygons on a ground plane for unsettled cells, if outlines prove insufficient

### Accepted residuals (no action planned)

- [ ] Recordings made before this work carry no grid state and cannot be given one without a
      replay from PCAP
