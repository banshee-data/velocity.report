# Beam-wise background subtraction against L3: adversarial review

A review of the [comparison note](20261011-beamwise-background-subtraction-comparison.md), made
the same day. The brief: attack Section 9 claim by claim, saying which can be falsified from
code or arithmetic alone and which need data; check the Section 5 derivation and the D9 bound;
find what the note missed; challenge the order and costs in Section 6.2; propose a better
decision rule for Section 7. No code and no tuning default was changed.

- **Status:** Review. Arithmetic; two simulations run against the shipped L3 code; a packet
  survey of the reference capture; a survey of deployed background snapshots. No replay, no
  labels, no dataset downloaded. The throwaway harnesses were run and removed; Section 8
  describes them exactly enough to rerun
- **Layers:** L3 Grid, with consequences for L5 lifecycle and the scene-change plan
- **Related:** [comparison note](20261011-beamwise-background-subtraction-comparison.md),
  [gap analysis §4](../paper-implementation-gap-analysis.md) (B3, B4, B7, B8),
  [parameter campaign 2026-09, pass 7](../../../docs/lidar/operations/parameter-experiment-campaign-2026-09.md),
  [scene-change recovery sprint](../../../docs/plans/lidar-scene-change-recovery-sprint-plan.md),
  [replay experiments](../../../internal/lidar/replayeval/experiments.go)

## 1. Verdict

Of the twelve claims, five stand (C1, C2, C7, C8, C10), two fall (C4, C11), three are revised by
measurement (C3, C5, C6), and two are narrowed by data already in the repository (C9, C12). The
note's central result, a range-invariant height band that L3 absorbs above the road, survives
re-derivation with two additions: the band is larger for cells inside the warm-up ramp, and the
paper's own labels cannot see it. The experiment plan is right in aim and wrong in mechanics: a
new config key moves the perf fingerprint whether or not it is on, the replay harness already
carries options without doing that, and the first experiment needs no new code.

One finding of this review outranks what it reviewed. **L3 has no policy for an object that
arrives and stays.** Whether a parked car is ever absorbed depends on its range, its depth against
the old background, the region multiplier and the warm-up ramp; when it is absorbed, the wait
equals the age of its cells' background, which is minutes in a replay and hours in a deployment;
with region overrides off it is never. The departure case the note worried about (C5) is the
lesser problem: a revealed surface flanked by clean neighbours clears in seconds. Section 5.1 has
the measurements. The scene-change sprint should read it before its S3.

## 2. The absorbed-height derivation (Section 5)

**Re-derived; it holds.** A beam from a mount at height $H$ at depression $\theta$ reaches the
road at $r_{road} = H / \sin\theta$. A point at height $h$ above the road on that beam is at
$r = (H - h)/\sin\theta$, so it sits $h/\sin\theta$ in front of the road along the beam. The
decision in `foreground.go` compares that gap with the window evaluated at the **observed**
range, `ForegroundClosenessWindowMetres(..., p.Distance, ...)`, which is
$W(r) = k\,(s + k_n r + 0.01)\,\mathrm{warm} + \text{safety}$. Substituting $r$ and solving
$h/\sin\theta \le W(r)$ gives

$$h_{abs} = \frac{k\,k_n\,\mathrm{warm}\,H + c\sin\theta}{1 + k\,k_n\,\mathrm{warm}}, \qquad c = k\,(s + 0.01)\,\mathrm{warm} + \text{safety}.$$

The note's expression is this with $\mathrm{warm} = 1$. At $H = 2.3$ m and $s = 0.01$ m the band
is 0.13 to 0.17 m as configured, 0.35 m under the ×3 override, 0.45 to 0.48 m under ×4 and 0.75 m
under ×8, across depressions of 2° to 10°. Those are the note's numbers.

**Assumptions the derivation needs, and what each costs.**

1. _The beam's background is the road, and the road is a plane._ True of lane cells; not of
   cells backed by a wall, hedge or parked car, where the absorbed depth is the gap to that
   surface instead and the band does not apply.
2. _The cell is settled, $\mathrm{warm} = 1$._ The warm-up multiplier scales the whole bracket,
   so $k\,k_n$ becomes $k\,k_n\,\mathrm{warm}$. For a cell at the confidence floor
   ($\mathrm{warm} = 3.91$) the band is 0.45 m as configured and 1.1 m under ×4. The closeness
   audit found 8.8 to 27.8% of cells inside the ramp; the snapshot survey in Section 8 finds 0.3
   to 2.4% in live runs. One mechanism puts lane cells there permanently: `TimesSeenCount` gains
   one per accepted return and loses one per rejected return, so a cell occupied by traffic more
   than half the time drains to the floor and stays. Heavy-traffic sites, not residential ones.
3. _The cell's mean is the road range during the transit._ The first absorbed returns pull the
   mean toward the vehicle, with the ×5 reacquisition boost when a rejected return preceded them
   (an approaching vehicle), so $\alpha_{eff} = 0.1$ over about three absorbed frames per cell per
   transit moves the mean by at most $0.3\,W$ and inflates the spread by at most $0.1\,W$ per
   frame, relaxing at 0.02 per frame afterwards. Second order; the static band is a lower bound
   during long dwells, not an overestimate.
4. _Flat ground, no roll, $H - h = r\sin\theta$._ A 3° roll (HighwayScene's own ground normal is
   3° off the sensor axis) shifts $r_{road}$ by 5%, not the band.

**What the band means.** Both the band and the vehicle subtend angles proportional to $1/r$, so
the share of a vehicle's rings that fall in the band is $h_{abs}$ over vehicle height at every
range: a third of a 1.4 m car and the lower legs of a pedestrian at 0.46 m. Ring spacing then
quantises what a test can see: the lowest ring striking a vehicle above the band sits up to one
spacing higher (0.33° in the dense band, 6 cm at 10 m and 17 cm at 30 m; 1° outside it). So a
height histogram of foreground points compares arms against each other, not against the formula.

**The paper cannot adjudicate this band.** Its Ouster and Blickfeld labels are lane volumes:
every point inside is dynamic. From the published Ouster configuration the two main volumes are
pitched 0.3° over 100 m, so relative to the fitted ground plane their floors run from 17 cm below
it to 35 cm above. Where the floor is below the road, road returns are labelled dynamic; where it
is above, the lowest 0 to 35 cm of every vehicle is labelled static. Either way the band this
note is about is not reliably labelled, and a 99.4% F1 there says nothing about it. CoopScenes'
object boxes, and our own full-scene labels, can.

## 3. The QG contamination bound (D9)

Verified against `models/qg.py`. With fit quantile $q$, QG keeps the samples at or above the
$q$-quantile of a beam's valid returns and takes their median and MAD. Occlusion only shortens
ranges, so an occupancy fraction $f$ contaminates the bottom $f$ of the sample. The kept set is
clean iff $f \le q$; between $q$ and $(1+q)/2$ the MAD inflates and the window widens; the median
breaks at $f \ge (1+q)/2$. For the Ouster's $q = 0.48$: clean to 48% occupancy, located to 74%.
For CoopScenes scene 3, $q = 0.025$ uses nearly every sample and has no such protection; it was
tuned that way because a vehicle present throughout training _was_ that beam's background, and
no far tail existed to prefer.

The GMM's rule is the counterpart: a component is background only if $w_k \ge 0.36$ (Ouster), so
an occluder present in fewer than 36% of training frames is never background. CFTA's coarse stage
takes the farthest peak holding at least 40% of samples. Three tolerances, 36 to 74%, all
against transient occupancy.

The shared blind spot is the one the paper names in scene 3: an occluder present throughout
training is the background, and when it leaves, the revealed surface is foreground for the life of
the frozen model. Section 5.1 shows L3 has the same blindness in a different form. Nothing in any
of these models is a clock.

## 4. Claims

| #   | Verdict                                           | Decided by                                                                                                                                                                                                                             | Cheapest next test                                                                                                                            |
| --- | ------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| C1  | Stands, sharpened                                 | The campaign already shows it: `closeness_multiplier` 1.5 to 2 and `no_region_overrides` move tracks 1.9 to 2.6× at 23 to 24 of 24 sites. The live question is the window's **shape**, flat or proportional to range                   | Flat-window arm against a proportional arm at equal near-field width, regions off, recovery by range band on kirk0 and kirk1. Existing keys   |
| C2  | Stands; larger in ramp cells; paper cannot see it | Section 2                                                                                                                                                                                                                              | Lowest-point height per labelled track from existing replays (M2 in Section 7)                                                                |
| C3  | Revised: sensor-dependent                         | Decomposing the committed results (Section 4.1): the filter outweighs the model on the Ouster and Aeva, the model outweighs the filter on the Blickfeld. The Ouster column is the one that resembles a Pandar40P                       | None needed                                                                                                                                   |
| C4  | Falls on available evidence                       | The campaign's extra tracks live 2 to 3× longer, at 23 to 24 of 24 sites, and recover 13 of 16 labelled tracks; noise clusters do not live longer. One confound remains (Section 5.1): stationary objects never absorbed               | M3 in Section 7 separates long-lived-because-stationary from long-lived-because-real                                                          |
| C5  | Revised by simulation                             | Without clean neighbours the ghost lasts the object's dwell plus about 210 s; flanked by road cells on the same ring it clears in 56 frames. The real exposure is arrival, not departure                                               | The two simulations in Section 8 as unit tests, in the commit that implements E2                                                              |
| C6  | Confirmed, stronger                               | A cell that returns only when tall vehicles pass flags 5 of 80 returns for the first ten vehicles and 0 of 1,520 after; it learns mean 14.7 m, spread 2.4 m. The mechanism is spread inflation plus the ramp, not the seed alone       | Count cells above the horizon with a mean in the road band and spread over 1 m in a settled snapshot; the audit's `-by-ring` attributes rings |
| C7  | Stands                                            | Scene 3 in the paper; Section 5.1 here                                                                                                                                                                                                 | None                                                                                                                                          |
| C8  | Stands                                            | The committed configurations: QG $z$ 2.2 to 30, radius-filter $n_{min}$ 6 to 88                                                                                                                                                        | Needs their data                                                                                                                              |
| C9  | Narrowed                                          | Per scene, density-aware CFTA beats MOG2 in 3 of 4 and QG in 4 of 4 (sign test $p = 1/16$); QG trails both in every scene. Means 93.8, 93.2, 91.8                                                                                      | Needs per-frame scores                                                                                                                        |
| C10 | Stands                                            | Ranks above; GMM is last of the four tuned models on CoopScenes, first on every HighwayScene sensor                                                                                                                                    | A site survey, not a replay                                                                                                                   |
| C11 | Falls                                             | Fixed-point arithmetic (Section 4.2): a closed-loop spread under a $z\sigma$ gate collapses only for $z$ below about 1.6; at $z = 2$ the self-consistent gate is $0.83z$; at $z \ge 4$ it is $z$. Laplace noise has the same threshold | None                                                                                                                                          |
| C12 | Resolved for kirk0                                | The reference capture's packets carry return mode `0x38` (single, last return) in all 1,999 sampled, at 597 to 600 rpm. Dual-return averaging does not occur there                                                                     | Read byte 14 of the tail on each deployment's first packet                                                                                    |

### 4.1 Model against filter, per sensor

From `configs/paper/reference_results.json`: model gain is each tuned model's F1 without the
filter minus the original CFTA's without `pcdenoise`; filter gain is the same model with its
tuned filter minus without.

| Sensor    | Model              | Model gain | Filter gain | Final F1 |
| --------- | ------------------ | ---------: | ----------: | -------: |
| Ouster    | Density-aware CFTA |      +1.85 |       +2.55 |    98.22 |
| Ouster    | GMM                |      +2.96 |       +2.65 |    99.43 |
| Ouster    | QG                 |      +1.47 |       +3.96 |    99.25 |
| Ouster    | MOG2               |      −1.44 |       +4.59 |    96.97 |
| Blickfeld | Density-aware CFTA |     +11.84 |       +1.26 |    94.42 |
| Blickfeld | GMM                |     +13.10 |       +2.53 |    96.95 |
| Aeva      | Density-aware CFTA |      +4.96 |      +11.40 |    97.36 |
| Aeva      | GMM                |      +5.26 |      +11.33 |    97.59 |
| Aeva      | QG                 |      −3.08 |      +16.47 |    94.40 |

On the Blickfeld the model matters because density-awareness handles a sparse solid-state
return pattern. On a dense rotating sensor the filter is worth as much as or more than the choice
of model.

### 4.2 Truncation does not collapse the spread

Let noise be $N(0,\sigma^2)$ and the spread be learned from returns inside $\pm t\sigma$. Then
$E|x| = \sigma\sqrt{2/\pi}\,(1 - e^{-t^2/2})/\mathrm{erf}(t/\sqrt{2})$, and a window of
$z\,s/0.798$ gives the self-consistent gate $t = z\,(1 - e^{-t^2/2})/\mathrm{erf}(t/\sqrt{2})$.
Near zero the right side is $0.627\,z\,t$, so zero is stable only when $z < 1.6$. Solved
numerically: $z = 1.7$ gives $t = 0.87$; $z = 2$ gives $1.65$; $z = 3$ gives $2.97$; $z \ge 4$
gives $z$ to three figures. For Laplace noise the small-$t$ slope is the same and so is the
threshold. E1 at any $z$ the paper used needs no floor to avoid collapse; it needs the floor for
the different reason the note gave, that the in-situ spread is below the sensor's specification.

## 5. What the note missed

### 5.1 Arrival is bistable, and its clock is the background's age

The simulation in Section 8 ran the shipped L3 with region overrides off on one ring of 60 bins
at 10 Hz. Two sequences: a car face at 8 m present from frame 0 and then replaced by the road at
15 m (the paper's scene 3), and a road learned for 1,000 frames, then the car for `parked`
frames, then the road.

| Road cells on the ring | Car present from | Parked (frames) | Car absorbed during parking at frame | Ghost after departure: first cell clear / half / all (frames) |
| ---------------------- | ---------------- | --------------: | -----------------------------------: | ------------------------------------------------------------: |
| 0                      | frame 0          |           1,000 |                                    0 |                                         3,104 / 3,104 / 3,109 |
| 0                      | frame 0          |           3,000 |                                    0 |                                         5,104 / 5,104 / 5,107 |
| 0                      | frame 0          |           9,000 |                                    0 |                                      11,104 / 11,104 / 11,108 |
| 20                     | frame 0          |     1,000–9,000 |                                    0 |                                                   0 / 34 / 56 |
| 0 or 20                | frame 1,000      |     1,000–9,000 |                            **never** |                                                   no ghost: 0 |

Three mechanisms, all in `foreground.go`, account for every row.

- **Departure drains the count.** A rejected return decrements `TimesSeenCount`; a settled cell
  has as many counts as returns it accepted, so the ghost lasts the dwell. Below 100 the freeze
  fires on each thaw (5 s, one decrement per cycle) until the ramp has widened the window enough
  that the 15 m return falls inside three times it; then the deadlock breaker relearns. That tail
  is about 2,100 frames here and is why the ghost is the dwell plus 3.5 minutes, not the dwell.
- **Neighbours erode a flanked ghost in seconds.** An edge cell is accepted by same-ring
  neighbour confirmation, its first accepted 7 m deviation adds $\alpha\,\delta = 0.14$ m to its
  spread (0.42 m to its window), and within a few frames it vouches for the next cell. Forty cells
  cleared in 56 frames. Every real departure has road on both sides of the car on every ring, so
  the parked-car ghost the note feared is a seconds-long event in practice. A sensor nudge shifts
  every cell at once, leaves no clean neighbours, and gets the first regime: that is consistent
  with the Lombard–Laguna persistence the sprint is investigating.
- **Arrival has no erosion and may have no exit.** The car's return at 8 m is compared with the
  window at **8 m**, not 15 m, and neighbours' backgrounds are the road, so nothing vouches for
  it. The count drains at one per return; below 100 the freeze cycles; at the floor the widest
  window is $3\,(s + 0.02 r + 0.01)\cdot 3.91 + 0.15$ and the car is absorbed only if its depth
  $\delta$ is within three times that: with overrides off, $\delta \le 0.70\,r + 35\,s + 0.8$ m,
  so 6.8 m at 8 m against a 7 m depth, and never. Under the ×4 stable-region multiplier the bound
  is $2.8\,r + 0.8$ m and the car is absorbed a few hundred frames after the count drains.

**The drain is the age of the background.** `TimesSeenCount` has no cap: it grows by one per
accepted return for as long as the sensor runs, and it is persisted and restored. The snapshot
survey (Section 8) finds p50 counts of 24,500 after about 75 minutes of a live run, growing at
about 5.4 per second per cell. A car that parks after eight hours of uptime therefore carries a
foreground blob for about eight hours before the shipped configuration absorbs it, and for ever
under `no_region_overrides`. In a 200 s campaign replay the same car is absorbed in two to three
minutes. **Replay results on stopped objects do not transfer to deployment**, and the campaign's
`no_region_overrides` arm, read as "B8 widens windows", also carries "stopped objects are never
absorbed", which is one candidate for its 2.3× track lifetime.

For the product this is a permanent zero-speed track that occludes the scene and merges with
passing traffic under a fixed 0.8 m DBSCAN eps. The paper's frozen models have the same hole
(nothing arriving after training is ever background) and no remedy. L3 cannot fix it alone:
a cell cannot tell a parked car from a skip. The remedy is a dwell policy at the object level, L5
knowing a track has been stationary for $T$ and asking L3 to absorb its cells, paired with E2 for
the departure. That is a new step, and it belongs before E1 if a long capture confirms this.

### 5.2 Neighbour confirmation has two jobs, and the note priced one

As a recall filter it is the paper's model-consistency filter, their worst (93.4 against 97.6 F1
on the Aeva). As a ghost eraser it is the 56-frame mechanism above. E4 can take the first job;
nothing in the note takes the second. Weakening neighbour confirmation before E2 lands turns a
seconds-long departure ghost into the dwell-long one.

### 5.3 Spread inflation is a feedback the paper's statistics do not have

$s \leftarrow (1-\alpha)s + \alpha\,|x - \mu_{old}|$ over accepted returns means one accepted
far return adds $\alpha\delta$ to the spread and $3\alpha\delta$ to the window: 0.42 m per frame
for a 7 m deviation at $\alpha = 0.02$, 2.1 m under the ×5 boost. It drives the fast erosion, it
is why the sky cell ends with a 2.4 m spread and accepts every vehicle, and it is a large part of
what the volatile tercile is. The MAD of a fixed far-tail sample has no such loop; Stauffer and
Grimson update only the matched component. Any E1 that keeps a learned spread term must decide
what a far accepted return is allowed to teach it.

### 5.4 Index stability

The paper keys cells by native (channel, measurement id). L3 bins a 0.01°-resolution block
azimuth, after per-channel and firetime corrections, into 0.2° bins at a motor speed the capture
shows wandering between 597 and 600 rpm, so a given firing straddles a bin edge from one
revolution to the next. The survey's accepted-return rate of about 5.4 per second per cell, against
10 frames per second, is consistent with firings landing in the neighbouring bin about half the
time, though missing returns also lower it. The consequence is that part of the measured spread is
azimuth jitter on oblique surfaces, and that the paper's per-beam statistics are cleaner than ours
for a reason unrelated to the model.

### 5.5 Smaller points

- **DBSCAN is a density test.** Fixed eps 0.8 m and minPts 5 make it strict near the sensor and
  lax at range; the paper's radius filter scales with range. "No density test until DBSCAN"
  should read "no range-adaptive density test in index space".
- **Their grid filters flip one point per cell**, the nearest foreground return
  (`_build_fg_index_grid`); other foreground points in the cell survive. The code assumes one
  return per beam per frame, which the Ouster gives and which E4 must not.
- **The comparison's Section 10, question 1 is answered for kirk0:** single return.
- **Their max-range cut** (100 m, 150 m) removes the band where L3's windows are widest; a
  like-for-like score must apply the same cut.

## 6. Order and cost of the steps (Section 6.2)

**P1 is the wrong mechanism.** `TuningConfig.Fingerprint` is the SHA-256 of `json.Marshal` over
the whole struct, so a new key moves it whether the key is on or off, and the perf gate then
refuses every committed baseline. [experiments.go](../../../internal/lidar/replayeval/experiments.go)
exists for exactly this: "deliberately not tuning keys ... a new key would move the fingerprint
and make the committed perf baselines refuse to compare before anyone had measured whether the
option helps". E1 to E4 should be prototyped as replay experiments, which cost no fingerprint,
run offline only, and are hashed into the run metadata. The note's "memory: none" costs were
right; its silence on fingerprints was not.

**The first experiment needs no new code.** The campaign's pass 7 already ran
`closeness_1p5`, `closeness_2` and `no_region_overrides`. What it did not run is the arm the
note's C1 turns on, a **flat** window: `no_region_overrides` with `noise_relative` at the smallest
value validation accepts and `closeness_multiplier` 3 gives $W \approx 3s + 0.18$ m plus a
negligible range term. Add that arm, the same with `neighbour_confirmation_count` 0, and the
metrics M1 to M5 of Section 7, all from existing replays, VRLOG foreground and the ground plane.
Size S. P0, an M tool that needs external data and two dataset adapters, is not where to start.

**E4 replaces neighbour confirmation; E2 lands first.** Section 5.2. The note has E4 as an
addition and E2 after E1.

**A new step ahead of E1: the dwell policy.** Section 5.1. An object-level timer at L5 that asks
L3 to absorb a track's cells after $T$ stationary seconds, with E2 handling the reverse. Size M.
It is the one step that changes what the product does with a parked car, and nothing in the
paper offers it.

**A cheap half of E6: a spread cap (E3b).** The sky-cell result shows a mixture does not help by
itself; the GMM's active ingredient is its component rule, $\sigma_k \le \sigma_{max}$ and
$w_k \ge w_{min}$. A cell whose spread exceeds a cap has no background, so its returns are
foreground until E4 says otherwise. Size S beside E3, and it covers most of what E6 would, for
intermittent cells.

**P0 is under-priced.** HighwayScene records are protobufs read through the authors'
`highwayscene` package; CoopScenes is `.4mse`. A Go reader for either is M by itself. The cheap
route is a Python shim registered through their `register_method()` that writes each
preprocessed frame's (channel, horizontal index, range, label) to flat files, and a Go harness
that reads those: S plus M. The harness passes its own grid dimensions, so the OS0's beam count
is not a problem, but neighbour confirmation's ±3 bins mean different angles at 1,024 or 2,048
columns than at 1,800.

**E7 is over-priced for QG.** A per-cell streaming quantile (the P² estimator) is $O(1)$ memory;
the 173 MB range matrix is only needed for CFTA's histogram, and a 64-bin uint16 histogram per
cell is 9 MB.

**Revised order.** Z0, the zero-code arms with M1 to M5. Z1, an arrival check on a long capture:
a car parking after at least an hour of uptime, or the Lombard–Laguna VRLOG read for stationary
track lifetimes against uptime. Then E2 with the dwell policy; then E1 with E4 replacing neighbour
confirmation, as replay experiments; then E3 and E3b; then P0 for the point-wise comparison
beside the paper's table; E5 to E7 as research arms.

## 7. A decision rule that can be pre-registered

The note's rule has three weaknesses: "every band past 10 m" lets an empty band veto, "+5 points
of recall" is impossible where recall is already 95% and is not the quantity C2 predicts, and
"post-L4 precision" mixes point and cluster units. Replace it with two stages and explicit kill
criteria.

**Stage A: existing captures, no new labels.** Arms Z0 above, each at 24 campaign sites and on
kirk0 and kirk1.

| Metric | Definition                                                                                                                  | Adopt if                                                                        |
| ------ | --------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| M1     | Labelled tracks recovered (existing evaluator, M5 caveat)                                                                   | At least the best existing arm: 13 of 16 on kirk0, 33 of 49 on kirk1            |
| M2     | Per labelled vehicle track, the 5th percentile of height above the ground plane of its cluster points; median across tracks | At most 0.15 m, against the 0.35 m or more the shipped arm is predicted to show |
| M3     | Tracks with lifetime over 60 s and p90 speed under 0.3 m/s, per site-hour                                                   | No more than the shipped arm's                                                  |
| M4     | Settle frame within each site's run-to-run band (the campaign's rule)                                                       | 24 of 24 sites                                                                  |
| M5     | p95 L3 plus filter time per frame on the Pi                                                                                 | Inside the frame budget                                                         |

**Kill criteria.** If the shipped arm's M2 is already under 0.15 m, C2 is false and E1 loses its
rationale: stop and re-read Section 2. If any arm raises M3, it needs the dwell policy before it
ships, whatever M1 and M2 say.

**Stage B: point labels.** kirk0 labelled full-scene is the gate: recall of foreground points
below 0.5 m above ground at least twice the shipped arm's, point precision after E4 within one
point of the shipped arm's, both with frame-bootstrap intervals, by range band for the record but
not as a veto. CoopScenes and HighwayScene are reported beside the paper's table with its own
labels' limits stated (Section 2), and do not gate anything.

**What is reported either way.** Every arm's M1 to M5, the ghost and arrival simulations as
tests, and the per-band numbers, so a negative result closes the question rather than reopening
it.

## 8. What was run

All harnesses were throwaway files inside the module, run once, and deleted; nothing was
committed. The two simulations should become tests in the commit that implements E2, since a
behaviour change must carry its tests.

**Simulation A, parked car.** `NewBackgroundManagerDI` with 1 ring × 60 azimuth bins, parameters
from `config/tuning.defaults.json` through `BackgroundConfigFromActiveTuning(...).ToBackgroundParams()`,
`DisableRegionOverrides` true, replay mode on, no store. Frames every 100 ms from a fixed epoch
through `ProcessFramePolarWithMaskAt`; every bin gets one return per frame with Gaussian noise
of 10 mm (seed 1). Bins below `roadCells` return 15 m always; the rest return 15 m, or 8 m while
the car is present. Settling completes at frame 300. The table in Section 5.1 is the output.

**Simulation B, sky cell.** 1 ring × 8 bins, same parameters. Bin 7 returns 5 m every frame so
the frame counter advances; bin 0 returns nothing for 400 frames, then 200 vehicles, each 8
frames at a range drawn uniformly from 10 to 20 m (noise 20 mm, seed 2) followed by 40 empty
frames. Foreground flags on bin 0: 5 of 80 for vehicles 1 to 10, 0 of 1,520 for vehicles 11 to
200; final cell mean 14.67 m, spread 2.36 m, `TimesSeenCount` 1,590.

**Packet survey.** `tcpdump -x` over the first 1,999 UDP packets of
`internal/lidar/perf/pcap/kirk0.pcapng`; tail at payload offset 1,240; byte 14 was `0x38` in all,
bytes 8 to 9 gave 597 to 600 rpm.

**Snapshot survey.** An APFS clone of the root `sensor_data.db` (369 `lidar_bg_snapshot` rows,
one sensor, 2026-01-24 to 2026-10-11), read through `internal/db`, every fifteenth snapshot by
time, cells with `TimesSeenCount` of at least 10 and a mean over 0.5 m. Abridged:

| Taken (UTC)      | Reason         |  Cells |    p10 |    p50 |    p90 |    max | Below 100 |
| ---------------- | -------------- | -----: | -----: | -----: | -----: | -----: | --------: |
| 2026-02-04 18:02 | periodic_flush | 64,816 |  2,379 |  2,393 |  2,393 |  2,759 |      0.6% |
| 2026-02-04 18:33 | periodic_flush | 65,034 | 20,801 | 20,979 | 20,982 | 24,432 |      0.3% |
| 2026-02-04 19:17 | periodic_flush | 64,945 | 24,415 | 24,541 | 24,545 | 28,769 |      0.4% |
| 2026-02-06 23:34 | periodic_flush | 65,862 |  4,753 |  5,214 |  5,254 |  5,819 |      0.6% |
| 2026-08-27 20:02 | region_settle  | 71,576 |    300 |    300 |    301 |    365 |      0.4% |
| 2026-10-11 07:57 | region_settle  | 69,341 |     55 |     59 |     59 |     81 |    100.0% |

The `region_settle` rows are replay runs snapshotted at settle. The clone was deleted afterwards;
no raw output was kept.

**Arithmetic.** Sections 2, 3, 4.1 and 4.2, by hand and in short scripts outside the repository.

## 9. Open after this review

1. Does a deployment show the arrival effect? The snapshots cannot show objects. It needs a
   capture with a known parking event after at least an hour of uptime, or the Lombard–Laguna
   VRLOG's stationary tracks read against uptime.
2. What is the smallest `noise_relative` validation accepts? The code coerces values at or below
   zero to 0.01; the flat-window arm depends on a small positive value being allowed.
3. Which tercile do kirk0's lane cells fall in? Regions are persisted, so a settle snapshot
   answers it without a replay.
4. Is the 5.4 per second count rate bin straddling or missing returns? One PCAP frame sequence
   with per-cell hit counts answers it.
5. The HighwayScene OS0's beam count and whether its records carry measurement ids: unchanged
   from the note's question 4.
