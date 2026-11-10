# Small artefacts refused at birth, or absorbed: both fail on kirk0, for different reasons

This report answers [l6cls0005](../../../data/experiments/l6cls/l6cls0005.md). It tests two
default-off L5 options on kirk0: one holds small artefact tracks back at birth, the other absorbs
small clusters into a neighbouring track. Neither passes the rule. The birth gate removes one
artefact in six and costs two reviewed pedestrians frames. Absorb takes fragments of movers, not
artefacts. It passes every other clause and gains on false negatives.

- **Status:** Measured on kirk0, 2026-11-09. Both arms fail. Nothing adopted, no default changed
- **Spec:** [l6cls0005](../../../data/experiments/l6cls/l6cls0005.md), rule committed at
  `fc6e362e` before any arm was replayed
- **Layers:** L5 birth and confirmation; scored by the per-frame evaluator and the label-free
  scripts
- **Parameter set:** the [defaults](../../../config/tuning.defaults.json), fingerprint
  `ab0c4035780d`. Both options are replay experiments, not tuning keys
- **Related:** [the November benchmark (l5id0019)](https://github.com/banshee-data/velocity.report/pull/994),
  [experiment review, D13 and D14](experiment-review-2026-11-05.md#d-what-counts-as-noise),
  [the l6cls0002 quality gates](pcl-noise-and-splits-2026-11.md)

## Summary

- **The birth gate fails four of five clauses.** kirk0's artefacts go from 6 to 5, against a
  target of 3 or fewer. The paired bootstrap establishes a rise in false negatives: +1, +9 and
  +23 at the 5th, 50th and 95th percentiles. Two reviewed pedestrians lose matched frames, one of
  them 7 of 23, and one reviewed car loses 1. The standing pedestrian among the stationary road
  users keeps 86.8 % of its rows, under the 90 % bound. Identity is non-inferior.
- **It holds pedestrians as well as artefacts.** At birth, a distant pedestrian is small, sparse
  and slow. That is the same shape as an artefact. Holding it delays its confirmation until it
  has moved 0.5 m or persisted for 1 s. Four of the six `a0` artefacts escape the gate: one
  persists for 6.2 s, and three have clusters that move 0.96 to 2.7 m during their tentative
  frames.
- **Absorb fails only the target, and it gains elsewhere.** Its artefacts go from 6 to 8.
  Fragments of movers go from 3 to 1. False negatives fall from 265 to 261 (paired −12, −4 and
  0). Fragmentations fall from 46 to 43 and switches from 90 to 86. No reviewed object loses a
  frame, one car gains 4, and every stationary road user is kept.
- **No stopped car is touched.** In both arms the 39 s stopped car keeps at least 99 % of its
  rows, and no moving track is lost. A size exemption at birth avoids the l6cls0002 failure, which
  was removing whole confirmed tracks.
- **Both options are bit-identical when off.** `a0` at this branch reproduces l5id0019's tracking
  baseline digest `79d9abff…` byte for byte, and the new scene test's two amd64 digests were taken
  on `origin/main` before the change.

## Question and rule

Question: can L5 refuse to confirm a small, near-static, sparsely supported tentative track that
has not persisted? The aim is to halve kirk0's artefacts without the l6cls0002 failure, which
removed stopped cars and cost reviewed objects frames. The rule was committed before any arm was
replayed (`fc6e362e`), kirk0 only, each arm against `a0`:

1. **Target:** artefact tracks (D13 class (a)) fall to half of `a0`'s count or fewer.
2. **False negatives:** the paired bootstrap's ΔFN 5th percentile is not above zero.
3. **Reviewed objects:** no reviewed object has fewer matched frames than under `a0`.
4. **Kept:** every `a0` stationary road user has an arm track within 1.0 m on 90 % or more of its
   rows, and no moving track is lost.
5. **Identity:** J1 non-inferiority. The HOTA and IDF1 5th percentiles must be −0.02 or above,
   and the switch 95th percentile +10 or below.

## The arms

- **`birth-gate`** (`artefact_birth_gate`, `TrackerConfig.ArtefactBirthGate`). A tentative track
  that has earned `hits_to_confirm` stays tentative while four things all hold:
  - every cluster it took is under 1.5 m long and under 1.5 m wide;
  - its latest centroid is under 0.5 m from its first;
  - its clusters average under 24 returns;
  - under 1.0 s of capture time has passed since its first observation.

  The shipped tentative miss rule (`max_misses`, 3) ends a held track if it flickers. The gate
  deletes nothing itself.

- **`absorb`** (`artefact_absorb`, `TrackerConfig.ArtefactAbsorb`). An unassociated cluster
  qualifies when it is under 1.5 m on both sides, has under 24 returns, and lies within 1.25 m of
  the drawn box of a confirmed track matched this frame. Such a cluster seeds no track. The
  neighbouring track's state is not updated from it: absorbing a cluster only means no track is
  born from it.

The spec gives each threshold's physical anchor, and
[artefact_birth.go](../../../internal/lidar/l5tracks/artefact_birth.go) restates them.

## Method

- **Replays.** `lidar-state-estimation-baseline` on kirk0 whole as a one-case corpus, as l5id0019
  ran it: `-warmup 6 -require-settled=false -sample-points 1024 -campaign-metrics`. Each replay
  kept its evidence, used the committed cohort manifest and a scratch use ledger at migration 71,
  and was repeat-verified (`baseline_equal` true for all three arms). The arms ran one at a time
  on a host shared with seven other agents, so no timing is reported.
- **Identity, false negatives and per-object frames.** `lidar-ground-truth-eval perframe` on the
  reconstructed reviewed pack (`ds_67842cb1b22b70e7`). The split was drafted with
  `-from-seconds 6 -split kirk0-tuning`, with `obj_788622b5` ignored. Online estimates served as
  declared baselines, against `a0`, with `-bootstrap 2000 -bootstrap-seed 20261108`. This branch
  extends the bootstrap: it now reports FN percentiles (`fn`) and scores each reviewed object
  alone with its attributed tracks (`by_object`). Clause 3 reads the per-object score's
  `matches`.
- **Label-free.** On each arm's evidence: `lidar_clutter_origin.py` (D13 classes),
  `lidar_noise_screen.py`, `lidar_flip_cost.py --match footprint`, and
  `lidar_figure_sensitivity.py` against `a0`.
- **Clause 4** used a scratch script. It restates `lidar_clutter_origin.py`'s class (b) test per
  `a0` track, then counts the `a0` rows that have an arm row within 1.0 m at the same frame.
- **Not run:** `lidar-box-eval`, so no duplicate pairs are reported. The spec listed them as
  reported-only.

## Results

### Against the rule

| Clause                      | `a0`                    | `birth-gate`                             | `absorb`                      |
| --------------------------- | ----------------------- | ---------------------------------------- | ----------------------------- |
| 1. Artefact tracks          | 6                       | 5: **fails** (need 3 or fewer)           | 8: **fails**                  |
| 2. ΔFN, 5th / 50th / 95th   | FN 265 (full)           | +1 / +9 / +23: **fails** (FN 274)        | −12 / −4 / 0: passes (FN 261) |
| 3. Objects losing frames    |                         | 3 of 19: **fails**                       | 0 of 19: passes (one gains 4) |
| 4. Stationary road users    | 2 (a car, a pedestrian) | car 99.2 %, pedestrian 86.8 %: **fails** | both 100 %: passes            |
| 4. Moving tracks lost       |                         | 0                                        | 0                             |
| 5. ΔHOTA, 5th / 50th / 95th |                         | −0.0039 / +0.0002 / +0.0059              | −0.0008 / +0.0012 / +0.0072   |
| 5. ΔIDF1, 5th / 50th / 95th |                         | −0.0037 / +0.0011 / +0.0037              | −0.0006 / +0.0025 / +0.0054   |
| 5. Δswitches, 95th          |                         | 0: passes                                | 0: passes                     |
| Verdict                     |                         | **Fails** 1 to 4                         | **Fails** 1                   |

### Identity, full comparison

| Arm          |   HOTA |   IDF1 | Switches | Frag |  FN |    FP | Matches | Tracks |
| ------------ | -----: | -----: | -------: | ---: | --: | ----: | ------: | -----: |
| `a0`         | 0.4284 | 0.4728 |       90 |   46 | 265 | 3,967 |   2,207 |     79 |
| `birth-gate` | 0.4296 | 0.4758 |       84 |   46 | 274 | 3,896 |   2,198 |     75 |
| `absorb`     | 0.4290 | 0.4745 |       86 |   43 | 261 | 3,953 |   2,211 |     77 |

`a0` reproduces l5id0019's figures exactly. The bootstrap's labelled-object HOTA is 0.4798 for
`a0`, 0.4799 for `birth-gate` and 0.4809 for `absorb`. Fragmentations and switches move under
`absorb` (paired 5th to 95th: −9 to 0 and −8 to 0), but neither interval clears zero, so neither
is a claimed gain.

### Reviewed objects whose matched frames changed

The split has 19 scored objects: 11 cars, 4 pedestrians, 3 trucks and 1 bus.

| Arm          | Object         | Class      | Matched, `a0` → arm | FN, `a0` → arm |
| ------------ | -------------- | ---------- | ------------------- | -------------- |
| `birth-gate` | `obj_bca3b9bb` | pedestrian | 23 → 16             | 15 → 22        |
| `birth-gate` | `obj_99987c9d` | pedestrian | 51 → 50             | 13 → 14        |
| `birth-gate` | `obj_4116ba22` | car        | 22 → 21             | 3 → 4          |
| `absorb`     | `obj_4929e754` | car        | 178 → 182           | 8 → 4          |

### Noise, clutter and counts

| Arm          | Tracks | Brief | Near-static | Clutter (artefact / stationary / fragment / unclassed) | Spurious a frame | Moving (lost / short / added) | p85, p98 (m/s) | Tracks confirmed | Gate: held / released | Absorbed clusters |
| ------------ | -----: | ----: | ----------: | ------------------------------------------------------ | ---------------: | ----------------------------- | -------------- | ---------------: | --------------------- | ----------------: |
| `a0`         |     79 |    12 |           3 | 15 (6 / 2 / 3 / 4)                                     |           0.6947 | 38                            | 7.06, 8.83     |               76 |                       |                   |
| `birth-gate` |     75 |     9 |           3 | 12 (5 / 1 / 2 / 4)                                     |           0.7710 | 39 (0 / 0 / 0)                | 7.06, 8.83     |               72 | 21 / 15               |                   |
| `absorb`     |     77 |    11 |           3 | 14 (8 / 1 / 1 / 4)                                     |           0.7038 | 39 (0 / 0 / 0)                | 7.06, 8.83     |               74 |                       |                35 |

The stationary column falls from 2 to 1 in both arms, but no road user was lost. The stopped car's
39 s track is still there, within 1 m on 99.2 % and 100 % of its rows. What changed is its p95
filter speed, which rose from 0.42 to 0.51 m/s. That crosses the near-static bound of 0.5 m/s, so
the track no longer counts as clutter. This is why clause 4 matches `a0`'s road users rather than
reading the arm's class column. Spurious clusters a frame
rise 11 % under the gate, because a held track's clusters belong to no lasting track. They rise
1.3 % under absorb, because an absorbed cluster belongs to no track either. The moving count's +1
in both arms is within the figure script's small-change band (2.6 %), and p85 does not move.

### What happened to each `a0` artefact

The birth-gate column says what held or released each track. The second table is read from the
raw clusters near each track's first row.

| `a0` artefact (life, box, returns, range) | `birth-gate`                                                     | `absorb` |
| ----------------------------------------- | ---------------------------------------------------------------- | -------- |
| 6.2 s, 0.59 × 0.28 m, 15, 12.2 m          | Kept: released at 1 s by persistence, near-static after          | Kept     |
| 0.6 s, 0.57 × 0.15 m, 9, 24.0 m           | **Removed**: held, ended tentative                               | Kept     |
| 0.8 s, 1.36 × 0.44 m, 44, 34.4 m          | Kept: not sparse, and moves 2.0 m                                | Kept     |
| 0.3 s, 0.66 × 0.33 m, 10, 61.4 m          | **Removed**                                                      | Kept     |
| 0.4 s, 0.96 × 0.25 m, 14, 20.3 m          | Kept: its tentative clusters move 2.7 m in 0.2 s                 | Kept     |
| 0.9 s, 0.56 × 0.17 m, 8, 52.7 m           | Kept: its clusters slide 0.96 m over 0.7 s, past the 0.5 m bound | Kept     |

Both arms gain an artefact `a0` did not have: a 0.19 × 0.13 m cluster of about 10 returns,
stationary at 10.4 m, held as a 3.0 to 3.4 s near-static track. The gate released it by
persistence. It lies about 0.9 m from the centre of the 39 s stopped car's track, and in `a0` no
separate track formed there. Absorb also gains a 0.4 s artefact at 66.6 m. These are ripples
through association, measured but not traced further.

## Interpretation

Each reading is marked measured or inferred.

**At birth, the artefact shape is also a distant pedestrian's.** Measured: the gate held 21
tentative tracks and released 15 of them, by motion, persistence or growth. Of the four reviewed
pedestrians, two lost frames, one of them 7 of 23. The standing pedestrian among the stationary
road users lost 5 of its 38 rows. Inferred: a person tens of metres away returns under 24
points in a box under 1.5 m, and walking slowly moves under 0.5 m in the 0.3 to 0.7 s before
confirmation. So the gate holds them, and the exemptions only bound the delay. The D13 class (b)
test for a pedestrian reads class and observed share, which a tentative track does not have. The
persistence substitute costs exactly what it waits for.

**Most of kirk0's artefacts are not the gate's target.** Measured: of six, one persists 6.2 s, two
have clusters that move 0.96 and 2.7 m while tentative, and one has 44 returns. Only two are the
young, static, sparse one-offs the gate was written for, and it removed both. Inferred: a
persistent small static track is a dwell question, not a birth question. The benchmark's ranking
already put the near-static small tracks under a dwell-type absorption. The moving artefacts are
jitter of sparse returns at range, which a displacement test cannot tell from motion over three
or four frames.

**Absorb is a fragment lever, as the spec expected.** Measured: it took 35 clusters, cut fragments
from 3 to 1, and moved no reviewed object's frames the wrong way. False negatives fell by 4, and
switches and fragmentations moved down without clearing zero. Inferred: D13 defines an artefact as
away from any moving track, so a rule that acts only beside a live body cannot reach one. Its
value is in what it does to the mover beside it. The car that gains 4 frames is consistent with a
fragment no longer competing for that car's clusters.

**No stopped car was at risk.** Measured: the 39 s stopped car (4.61 × 2.13 m) is covered on at
least 99 % of its rows in both arms. Inferred: a size floor applied to the clusters a track is born
from cannot remove a confirmed car-sized track. l6cls0002's gates removed whole confirmed tracks,
so they could.

## Recommendations

1. **Do not take the birth gate further in this form.** It fails four of five clauses. Its
   failure is structural rather than a matter of tuning: at birth, small size, few returns and
   slow motion describe a distant pedestrian as well as an artefact. A form that could pass would
   need evidence a tentative track does not have, such as height or the class's shape over
   several frames. It should be specified under a new rule, not rescued by moving these
   thresholds on kirk0.
2. **Read absorb as a fragment and duplicate lever, under its own rule.** It passed clauses 2 to 5. A new spec should read fragments, duplicate pairs (`lidar-box-eval`, not run here) and false
   negatives on the two l5id0019 placements' segments, with the d2 first segments after. Read
   that way, it is not a test of artefacts.
3. **Send the persistent small static tracks to the dwell policy.** Restrict `dwell_policy` to
   boxes under 1.5 m, as the benchmark's ranking already names it, and keep the labelled-pedestrian
   check: kirk0's standing pedestrian is one of these by size.
4. **Owed reads.** This run read kirk0 alone. Nothing here was read on the ashbury-downey and
   haight-divisadero segments, where l5id0019 counted 48 of the 54 pooled artefacts, or on the d2
   corpus with its held-out second segments. Absorb's wider read is the one owed first.

## What this does not establish

- **Anything beyond kirk0.** kirk0 has six artefacts. One track is 17 % of the target, so the
  clause 1 verdicts rest on one or two tracks. kirk0 is a tuning capture, and its `a0` clutter
  list was read before the thresholds were fixed.
- **That a held or absorbed track was not a real object.** The evidence holds confirmed tracks
  only. What the gate held and the miss rule ended is known from the tracker's counters
  (21 held, 15 released), not from rows.
- **Why the 10.4 m object became a near-static track in both arms.** That reading is measured but
  not traced through association.
- **The per-object attribution.** Each hypothesis track is attributed to one reviewed object. A
  track that crosses onto a second object counts for its own object only.
- **Duplicates and cost.** `lidar-box-eval` was not run, and the host was shared, so there are no
  timings.

## Provenance

| Item             | Value                                                                                                                                                                                                                                                                                                         |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base             | main `704b8e46`; spec committed at `fc6e362e` before any arm replay; bootstrap extension `4e223466`; options `9a775198`                                                                                                                                                                                       |
| Tools            | Built at `9a775198` with the Makefile's LDFLAGS (`version.Version` 0.5.1-pre52, `GitSHA` the full sha) and the `pcap` tag; every replay header reads `build_version` 0.5.1-pre52 and `build_git_sha` `9a775198…`. `lidar-state-estimation-baseline` `sha256:8586cf3a…`, `lidar-ground-truth-eval` `2e98a49b…` |
| Parameter set    | `config/tuning.defaults.json`, fingerprint `ab0c4035780d`, file SHA-256 `9ad7377f…`                                                                                                                                                                                                                           |
| Arm hashes       | `a0` `ab0c4035780d`; `birth-gate` `998977ae05df`; `absorb` `8040eb67d71f` (each folds its replay experiment into the tuning hash)                                                                                                                                                                             |
| Tracking digests | `a0` `79d9abff…` (equal to l5id0019's `a0`); `birth-gate` `f630b953…`; `absorb` `fb204745…`                                                                                                                                                                                                                   |
| Replay           | `lidar-state-estimation-baseline`, kirk0 whole, `-warmup 6 -require-settled=false -sample-points 1024 -campaign-metrics`, evidence per case, every arm repeat-verified                                                                                                                                        |
| Capture          | kirk0, `sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`, equal to the repository's LFS pointer                                                                                                                                                                                       |
| Labels           | label set `kirk0-reference`; pack `ds_67842cb1b22b70e7` reconstructed as in l5id0019; split drafted, `kirk0-tuning`, manifest `7194bb61…`, `obj_788622b5` ignored                                                                                                                                             |
| Scoring          | `perframe` with `-bootstrap 2000 -bootstrap-seed 20261108`; `lidar_clutter_origin.py`, `lidar_noise_screen.py`, `lidar_flip_cost.py --match footprint` and `lidar_figure_sensitivity.py` at `9a775198`; clause 4 by a scratch script restating class (b)                                                      |
| Host             | Claude Code cloud container, 4 cores shared with seven other agents; raw output in the session scratch, not kept                                                                                                                                                                                              |
