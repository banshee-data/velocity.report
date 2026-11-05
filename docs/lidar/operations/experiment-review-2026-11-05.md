# Experiment review, November 5, 2026: tracking stability, noise and confidence after the PCL round

<!-- ignore-style-length -->

The November round (l3bg0022, l3bg0027, l3bg0028, l4clus0005, l5id0006, l5id0012, l6cls0002,
l6cls0003, ground0006, pose0003) answered ten questions in four days and changed no default. This
review reads what it established for three goals, in order: one vehicle, one track, held through
its pass; fewer tracks and clusters that are not road users; and figures (counts, p85, p98) the
platform can state and defend. It sorts each failure into the candidate's, the measure's or the
bar's, builds an error budget for stability from what is already measured, and ranks eight
experiments for the next round, preferring a rescoring of archived evidence over a new replay and a
corrected measure over a new candidate. The specs it proposes are under `data/experiments/` with
their rules written now; the owner approves them.

- **Status:** Review, 2026-11-05; no measurement was run and no default changed
- **Goal:** tracking stability, then noise, then confidence in the published figures, as the owner
  asked on November 5
- **Inputs:** the [PCL review](pcl-algorithm-review.md) and its
  [plan](../../plans/lidar-pcl-algorithm-candidates-plan.md); the round's reports
  ([noise and splits](pcl-noise-and-splits-2026-11.md),
  [the overlap end as default](overlap-end-default-cost-2026-11.md),
  [settling convergence](settling-convergence-corpus-2026-11.md),
  [constrained plane](ground-constrained-plane-2026-10.md),
  [window consensus](gerardus-window-consensus-2026-10.md)); the changes that worked
  ([foreground refinement](l3-foreground-refinement-2026-10.md),
  [mean cluster centre](l4-cluster-mean-centre-2026-11.md),
  [one vehicle, two tracks](l5-duplicate-tracks-2026-10.md)); the
  [October 18 review](experiment-review-2026-10-18.md); the open queue
- **Method:** the [experiment-review skill](../../../.claude/skills/experiment-review/SKILL.md),
  answering the owner's twenty-six questions in groups A to G
- **Related:** [experiment queue plan](../../plans/platform-experiment-queue-plan.md),
  [label0001](../../../data/experiments/label/label0001.md),
  [W3 retention and fragmentation](w3-retention-fragmentation-kirk0-2026-10.md),
  [the input cap](l4-dbscan-input-cap-2026-10.md)

Throughout, each answer separates what was **measured** (a report's figure), what this review
**infers** from it, and what **nothing in the repository can tell yet**. Every number is a
report's, read at the commit in the provenance; the archived rows and evidence databases the
round left on hydra's LiDAR volume and the results share were not reachable from where this
review was written, so nothing here was rescored. Where a question needs a rescoring, the review
writes the corrected measure down as a spec and leaves the figure to the run.

## Answer

The round found no candidate because it looked in the wrong layer and measured with the wrong
ruler. Every PCL-derived arm acts on one frame's geometry in L3 or L4, and every one failed; the
three changes that moved kirk0's identity this autumn (foreground refinement, the mean cluster
centre, the overlap end) act on evidence over time or on the track's own state. The overlap end
with tentative priority is the one large lever: identity switches 87 to 5 on kirk0 and duplicate
pairs down at every one of 45 site-segments read. It is stuck on a cost that is partly real and
partly the scorer's: about 85 % of the "lost" tracks l5id0012 counted are the surviving half of a
duplicate pair on a long vehicle, which a 2 m point match cannot see, and the brief tracks it
leaves behind (61 to 580 on the tuning set) have never been looked at. The round also spent the
last label-free held-out segments the corpus had for identity, noise and counts, so from here on
an unlabelled segment can screen but cannot decide; the one thing that can is label0001, eight
hours of a person's time, proposed since October 13 and still not started. The next round is
therefore mostly rescoring: the archived rows rescored with a body-aware match, the brief tracks
classified, a bootstrap over kirk0's twenty-one reviewed objects so a delta of 0.008 HOTA can be
told from noise, and a first read of how much the survey's own figures move between arms, which no
report has yet asked. One new candidate is worth building: a birth gate that refuses to confirm a
track inside a live track's observed body, instead of ending it afterwards.

## What the experiments established

### A. Were the failures real, or did the rules fail?

**A1. Each failed arm, sorted.** The round's figures are the
[noise and splits report](pcl-noise-and-splits-2026-11.md) unless another is named.

| Arm                         | Deciding figure (measured)                                                                                                    | Cause (inferred)                                                                                                                                                                                                                                                        |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `veil` (l3bg0028)           | kirk0 retention at 50 m+ 0.779 to 0.766 (limit one point); spurious clusters −0.8 % at the median site (need −5)              | The candidate is worse, slightly: withholding veil returns removes real far support and buys almost no noise. The classifier is a range-step rule in PCL's vocabulary, not PCL's extractor, so it does not retire PCL's method; it retires this form                    |
| `radius-graph` (l4clus0005) | 20–35 m fragmented 6.5 to 4.5 %, 2.0 of the 5 points required; 50 m+ 8.9 to 17.5 %; HOTA 0.4293 to 0.4337                     | The bar was set against an outdated baseline: l4clus0003 wrote "five points" when `a0` read about 35 % in the band; after #901 it reads 6.5 %, so five points is most of what is left. The 50 m+ rise is unexplained and is a denominator question (B7)                 |
| `conditional`, both caps    | ID switches 87 to 263 and 317; fragmented 17.8 %                                                                              | The candidate is worse: the height condition splits bodies the plain graph joins                                                                                                                                                                                        |
| `held` (l3bg0027)           | L4 input ×4.3 (3,053 to 13,089 points a frame); vehicle tracks 36 to 138; HOTA 0.4293 to 0.2381; 8 cross-object merges        | The candidate is worse, and its premise had lapsed: the returns inside a tracked box are box paint, not lower body, and `edge_follow_cluster` is off since #901                                                                                                         |
| `g1` to `g3` (l6cls0002)    | kirk0 false negatives +11 to +13; removed tracks of 3.8 × 1.7 m for 39 s                                                      | The measure worked and the candidate failed it: "clutter" as defined includes stopped cars, and the gates removed them                                                                                                                                                  |
| `g4` alone                  | 47.5 % clutter at the median site against 50 %; kirk0 false negatives +2                                                      | Close on the target; the candidate was written after seeing `g1`'s result, so kirk0 is its tuning set and the shortfall is a fair fail                                                                                                                                  |
| `g4` on the overlap end     | −26.5 % at the median site (need −40); worst leave-one-out −26.2 % (need −30); range −62 % to +46 % by site                   | The bar assumed the gate would remove the overlap end's added brief tracks; most of them are not under 1.5 m wide, so it cannot. The measure cannot see what those brief tracks are (C9)                                                                                |
| overlap end as default      | lost 10.0 % and 10.2 % (bound 1 %); 212 of 251 and 224 of 267 lost tracks had an `a0` partner within 6 m on half their frames | The measure cannot see the gain: a 2 m point match on a bus or truck reads the surviving duplicate as a lost vehicle. Clause 3 (clutter +21 per hundred, bound 5) and clause 5 (p95 Δp85 1.97 m/s, bound 1.0) are real costs the measure saw correctly                  |
| `delta-0.005`, `delta-0.01` | 3 and 15 of 23 sites converge before 30 s (need 18); 0.001 converges none by 120 s                                            | The bar was unreachable and the criterion measures the wrong thing: the spread delta holds each site's traffic level (0.003 to 0.05) and does not fall with time; region stability reads 1.0 at every check because no regions exist before settling completes          |
| constrained plane, DoN      | p95 of held-back road residuals 0.582 to 1.169 m (Marina), 0.403 to 0.638 m (Columbus); 413 of 566 regions rise over 1 cm     | The candidate is worse for this estimator's job: MSAC fits the densest 0.05 m slab and leaves every other surface in the tail; the DoN flags 26 to 41 % of a one-return-per-cell background. The oracle met its contract, so this is PCL's method failing on this input |
| window consensus (pose0003) | 17 of 24 sites agree, 11 of them fail separation; 0 of 5 wrong-peak sites registered by agreement                             | The candidate does not do what was hoped; agreement across windows of one capture is not independence                                                                                                                                                                   |

**A2. What a rescoring would change.** Measured: l5id0012's rows (every scored frame of every
confirmed track, with `x`, `y`, `psi_deg`, `length`, `width`, `speed`) are archived for both arms
at 23 first segments and 22 third segments, and the pcl-20261102 evidence databases hold the
first and fourth captures for `a0`, `veil`, `radius-graph` and `body-older-tentative`. Inferred:
clause 1 (lost real tracks) and clause 2 (shortened) can be reread without a replay with a
body-aware match, written down here before any row is looked at: a row of `a0` is covered when
some `overlap-end` row at the same frame has a drawn footprint (its `psi_deg`, `length` and
`width`, grown by 0.25 m as `overlap_end_body` grows a live body) that contains the `a0` row's
centre, or whose footprint overlaps the `a0` row's by half the smaller. Nothing can tell yet
whether the roughly 40 unexplained losses a set (1.6 % of moving tracks, still over the 1 %
bound) are real vehicles; that is [l5id0016](../../../data/experiments/l5id/l5id0016.md)'s read,
and only labels would settle it. No other failed arm changes under a corrected measure: `veil`,
`held`, `conditional` and the constrained plane fail on figures the measure saw correctly, and the
radius graph's bar is the owner's to reset, not a rescoring's.

**A3. kirk0's `a0` is not pinned.** Measured: `a0` on kirk0 reads HOTA 0.4293 with 87 switches
in the noise and splits run (main `ef787825`, tools built bare at `a2a0317d0`, 1,024 sample
points), 0.4283 with 90 switches in l5id0012 (main `84492f9e`, tools at `934e0f493`), and 0.3936
with 92 switches in l3bg0022's `ceiling` arm (the honest first check, settling at the 30 s
ceiling; false negatives 1,091 against 502). The third gap is explained: under the ceiling kirk0,
a 59 s capture, has foreground for its last 29 s only. The first gap is not: the defaults'
fingerprint (`9ad7377f…`) and the pack (ad8b9438 version 2, `kirk0-tuning`) are the same in both,
and replays are byte-deterministic within a build, so 0.4293 against 0.4283 is a difference of
tool commit or replay option that neither report records. Inferred: a delta of that size (0.001
HOTA, 3 switches) is below what two runs of the same arm agree on, and several clauses in this
round read at 0.005 to 0.01. Nothing can tell yet how wide the sample makes these figures:
`lidar-ground-truth-eval` has no bootstrap, and kirk0's split is 21 reviewed road users, two of
them trucks that carry most of the fragmentation. A bootstrap over objects is
[l5id0015](../../../data/experiments/l5id/l5id0015.md); until it reports, this review treats a
kirk0 delta under 0.01 HOTA or under 10 switches as not distinguishable from `a0`, and reads the
round's claimed deltas that way: the overlap end's switches (87 to 5) and the honest check's
false negatives (502 to 1,091) stand; the overlap end's HOTA (+0.0074) and the radius graph's
(+0.0044) do not.

**A4. HOTA and false positives against a selective pack.** Measured: the pack labels the objects
a reviewer selected; the foreground refinement report found 24 of the unlabelled tracks its
candidate added travel 10 m or more, and read HOTA and false positives as not a loss for that
reason. In this round the verdicts that rested on those two figures were `held` (HOTA 0.24, but
also 8 reviewed merges and vehicle tracks ×3.8, so it fails on labelled figures too), the
conditional graph (IDF1 −0.11 and switches ×3, labelled), and the gates' clause 3 (false
positives fall, which reads as a gain only if every removed track was not a road user). Inferred:
no verdict in the round turned on HOTA or false positives alone, so none is overturned by this;
but every future gate clause that counts false positives as a gain should be read as "unlabelled
tracks removed" until a window is labelled whole. Nothing can tell yet what share of kirk0's
4,171 `a0` false-positive frames are real unlabelled road users.

### B. Where is stability actually lost?

**B5. The error budget.** All figures are measured; the ceiling column is this review's inference
of what a lever would gain if it removed the cause entirely, in what a user sees.

| Cause                                | Measured size                                                                                                                                                                         | Where measured                                                                                               | Ceiling if removed (inferred)                                                                                                                          |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Duplicates (a coasting track on top) | 15.5 pairs per hundred vehicle tracks, 26 % of vehicle tracks in a pair; on kirk0 121 of 188 pair-frames are one track coasting on the live one, 51 a body split in two               | [duplicates](l5-duplicate-tracks-2026-10.md#how-often-and-how)                                               | The whole of the switch budget the overlap end removed: 87 to 5 switches, counts down 11 to 12 %. Already measured; the lever exists                   |
| Fragmentation over time              | kirk0 fragmentations 47 under `a0`, 35 under the overlap end                                                                                                                          | [noise and splits](pcl-noise-and-splits-2026-11.md#kirk0)                                                    | The 35 that remain are what a linker (l5id0007 to l5id0010) could join: a quarter of the fragmentation budget after duplicates are gone                |
| Fragmentation in a frame             | 36 % of clustered instants split; 54 % at 20 to 35 m and 46 % at 35 to 50 m under the shipped L3 (October); 6.5 % at 20 to 35 m under the refined defaults (November)                 | [W3](w3-retention-fragmentation-kirk0-2026-10.md), [noise and splits](pcl-noise-and-splits-2026-11.md#kirk0) | At most the 6.5 points left in the band and 32 % at 35 to 50 m; the radius graph took 2 points. Mostly trucks: 59 % of truck instants split in October |
| Brief tracks                         | 61 of 2,503 moving tracks under `a0` on the tuning set (2.4 per hundred); 580 under the overlap end                                                                                   | [overlap end as default](overlap-end-default-cost-2026-11.md)                                                | Small under the defaults; the whole of the overlap end's clutter cost, which is why the birth gate (C10) is the lever worth building                   |
| Lost lower body and far misses       | 53 % of reviewed instants reach no cluster; 550 of the 917 near misses are one parked car in the background; beyond 50 m most objects are missed outright                             | [W3](w3-retention-fragmentation-kirk0-2026-10.md)                                                            | Not a stability cost in itself: a vehicle the pipeline never sees is not a split track. It bounds counts at range, which no report has measured        |
| Settling and warm-up                 | Under the honest ceiling kirk0 loses its first 29 s of foreground (false negatives 502 to 1,091); under the shipped first check the grid settles at 5.9 s and nothing is lost on kirk | [settling](settling-convergence-corpus-2026-11.md)                                                           | Zero in live operation as shipped; the cost is that the criterion decides nothing, so a scene that has not settled is not told apart from one that has |
| The L4 input cap                     | 10 of 23 sites capped on 1 % or more of frames; kept fraction down to 0.26 at Van Ness; on kirk0 `nocap` raises switches 87 to 148 while retention rises 0.851 to 0.993               | [density](l4-subsampled-density-2026-10.md), [noise and splits](pcl-noise-and-splits-2026-11.md#kirk0)       | Unknown sign: more returns cost identity on kirk0, which says the tracker, not the cap, is the sensitivity (B8)                                        |

**B6. Is per-frame geometry still the binding limit?** Measured: the three changes that moved
kirk0's identity acted over time or on track state (foreground refinement: switches 440 to 117,
by relearning cells from agreed runs of returns; the mean centre: 117 to 87, by steadying the
observation the filter reads; the overlap end: 87 to 5, by ending a duplicate). The four
per-frame candidates (veil, radius graph, held returns, the constrained plane) moved identity by
at most +3 switches or made it worse. Inferred: on kirk0, the binding limit on stability is the
tracker's lifecycle (birth, confirmation, coasting, death), then fragment joining over time, and
per-frame geometry's remaining share is the 6.5 % of 20 to 35 m instants and the 35 to 50 m band.
That inference would be refuted if, off kirk0, the brief and near-static tracks that make up
clutter turn out to be born from per-frame clusters (edge paint, vegetation, mixed returns) rather
than from lifecycle events on a real body: the radius graph did cut spurious clusters 5 % at 20 of
21 sites, so noise (goal 2) may still have a per-frame limit even where stability (goal 1) does
not. [l6cls0004](../../../data/experiments/l6cls/l6cls0004.md) is the read that tells them apart.

**B7. Why the radius graph splits far objects.** Measured: at 50 m+ the radius graph raises
retention from 0.779 to 0.853 and fragmented instants from 8.9 % to 17.5 % at once. Inferred:
"fragmented" is a share of the instants that reach a cluster, so when a graph with a component
floor of 5 admits far sparse instants that DBSCAN's core rule called noise, the instants it adds
are the sparse ones and arrive split. The doubling may be a denominator effect, not a splitting
rule. Nothing can tell yet, because the report carries shares and not counts; the fragmented
count at 50 m+ under each arm is in the archived kirk0 evidence and is a zero-code read
([l4clus0009](../../../data/experiments/l4clus/l4clus0009.md)). A range-conditioned join is not a
defensible arm before that read, and kirk0 has too few far objects to tune one on without it
becoming a kirk0 result.

**B8. Why more returns make more switches.** Measured: `nocap` on kirk0 lifts retention to 0.993
and switches from 87 to 148, with HOTA 0.4293 to 0.4216; in October the same lift (16,000 cap)
joined no fragment and changed HOTA by 0.001 under CMR. Inferred: more returns make more clusters
a frame (the parked car's, lower bodies, far sparse bodies), so more tentative tracks compete for
the same clusters, and under the shipped gate a confirmed track whose own object dropped out takes
a newborn's cluster (K10's finding, which tentative priority exists to stop). If so, the cap is
hiding an association sensitivity rather than saving identity, and `nocap` with
`tentative_priority` should bring the switches back without losing the returns. That is a
zero-code kirk0 arm in l4clus0009. Nothing can tell yet whether the Pi can afford it: the cap's
job is the frame-time tail (p99 48 to 153 ms on the Mac at 16,000), and perf0001 has not run.

### C. The overlap end: the one large lever

**C9. What the brief tracks are.** Measured: brief tracks (under 1 s) rise from 61 to 580 on the
tuning set and 90 to 816 held out; clutter added is 21 and 27 per hundred moving tracks; the
noise and splits report read them as "each duplicate it ends can leave a short track behind".
Inferred, from the tracker's code and l5id0004's mechanism: when the overlap end ends the younger
track while the live one is the younger, the older takes the cluster back next frame; in the
frames between, the body's cluster is unassociated and seeds a new tentative track, which at
`hits_to_confirm` 4 confirms in four frames and is then itself ended on the body, leaving a track
of under a second. If that is right, the brief tracks are re-births on the surviving body and are
the same physical object as the survivor. Nothing can tell yet: the archived rows can, because
each brief track's rows either lie inside a surviving track's footprint at the same frames or do
not. [l5id0016](../../../data/experiments/l5id/l5id0016.md) classifies them as re-births, splits
on the surviving body, or other, with the definitions fixed in its spec.

**C10. A birth gate.** Measured: the on-body test exists (`liveConfirmedBodies`, the cluster's
members' extent along the live track's heading grown by 0.25 m) and is applied to coasting
tracks only, after the association step; confirmation is `Hits >= HitsToConfirm` with no spatial
condition. Tentative priority changes who wins a cluster inside a confirmed track's 99 % ellipse;
it does not stop a cluster no confirmed track wins from seeding a track on a body another track
already holds. Inferred: a gate that refuses to count hits for a tentative track whose predicted
position lies inside a live confirmed track's observed body, and deletes it after `max_misses`
such frames, would remove the duplicate before it is ever published, so it leaves no brief track
and never ends a confirmed track. It is not expressible with the existing options; it is a small
`TrackerConfig` option beside `OverlapEndOnBody`, reusing the same test. Its spec, with the rule
written before any replay, is [l5id0014](../../../data/experiments/l5id/l5id0014.md). What could
mislead: two cars in a queue whose clusters touch, where a real second car's first frames lie
inside the first car's grown extent; l5id0003 lost 3 to 7 % of observed vehicle frames to exactly
that with drawn boxes, and l5id0004's on-body test held them at 99.6 %. The rule keeps the
observed-vehicle-frame clause for that reason.

**C11. Which track should survive.** Measured: keeping the older track (`body-older`) is what
lifted association accuracy from 0.679 to 0.736 and IDF1 from 0.350 to 0.435 in October, against
`body` (keep the live track) at 0.662 and 0.392. The speed tail (p95 |Δp85| 1.97 m/s on the
tuning set, 2.30 held out, bound 1.0) is measured over matched pairs where the covering track may
be a different one from the one `a0` carried. Inferred: a surviving older track carries a
converged velocity, so the tail is unlikely to come from the survivor; it is more likely the
re-born brief tracks of C9, whose p85 over a few frames of a young filter is noise, or pairs where
the covering track sits on a different part of a long body with the same speed but a different
`a0` partner. Nothing can tell yet which: the rows carry each track's speed and age, so l5id0016
bins the tail by the covering track's age at the match and reports it; it decides nothing, and
l5id0014's speed clause is read on the birth gate's own rows.

**C12. The lost tracks, rescored.** Measured: 251 and 267 lost; 212 and 224 with an `a0` partner
within 6 m on half their frames; 218 and 229 with an `overlap-end` track within 6 m; about 40 a
set with neither. Inferred: the 6 m diagnostic was written after the result and is itself a
point match; the body-aware match of A2 is the right ruler, and it would also reread the 112 and
132 "shortened" tracks, some of which are a body carried by one track where `a0` carried it in
two. Nothing can tell yet whether the 40 are real; l5id0016 lists them per site with their span,
length and class so the owner can look at the worst site's in the visualiser, and label0003's
spans (F20) are the first truth that could count them.

### D. What counts as noise?

**D13. Definitions.** Measured: clutter as defined (brief under 1 s, or near-static under 2 m net
with p95 speed under 0.5 m/s) removed stopped cars 3.8 × 1.7 m and 2.9 × 1.5 m on kirk0 under
`g1`; the dwell policy already absorbs a track that has stood still for 20 s. Inferred: parked
and queued vehicles must be a class of their own in every label-free measure, because the
product counts them in neither the noise nor the moving column. The review proposes, fixed in
[l6cls0004](../../../data/experiments/l6cls/l6cls0004.md) before any evidence is read: (a) an
**artefact** is a clutter track whose rows lie within 1 m of no moving track's footprint at the
same frames, and whose mean box is under 1.5 m long and wide; (b) a **stationary road user** is a
near-static track whose mean box is 1.5 m or more in length or width, or classed `pedestrian`
and observed in half or more of the frames of its life; (c) a **fragment of a mover** is a brief
or shortened track at least one of whose rows lies inside a moving track's footprint, grown by
0.25 m, at the same frame. What the archived evidence can tell apart today: all three, from the
rows (centre, heading, length, width, class, time), with the proviso that (a) against (b) is a
size prior, not truth. What it cannot: whether an artefact is the sensor's, the background's or
a real small object.

**D14. Where spurious clusters come from.** Measured: spurious clusters a frame are observations
no lasting track took; the radius graph cut them 5 % at the median site and `veil` 0.8 %; the
foreground refinement report found painted background from a vehicle that stood through settling
and left (763 of 820 painted returns in one frame 5 to 24 m behind the learned surface), frozen
cells, and edge follow. Inferred: a cheap diagnostic exists without a filter: bin every unowned
observation's position in 0.5 m cells over the scored window and count recurrence; a cell that
recurs across the window with no moving track is a background cause (vegetation, a learned
surface that moved, a wall edge); one that recurs beside moving tracks' paths is edge paint or
mixed returns; one that does not recur is a fragment or a one-off. l6cls0004 carries it per site.
Nothing can tell yet which cause dominates at which site.

**D15. Settling.** Measured: the spread delta holds each site's traffic level and does not fall
with time; the ratio to the mean spread runs 0.047 to 0.084 by site, and at 0.075 19 of 23 sites
converge before 30 s, none at the first measured check, at a median of 16.9 s; region stability
cannot fail during warm-up. Inferred: l3bg0030's relative criterion is the right next step for the
criterion's own question, but the question that matters downstream is what an unsettled scene
costs: spurious clusters a frame in the 30 s after settling against the same site's 60 to 120 s
steady state. This review fixes l3bg0030's ratio at 0.075 from l3bg0022's figures, names the
second segments as its held-out trajectories (settling has read only the first), and adds that
downstream read as a reported clause. The region-stability criterion should be dropped from
warm-up or made to read something; that is engineering, not an experiment.

### E. Confidence in the outputs

**E16. How much the figures move.** Measured: nothing. No report has asked how a site's vehicle
count, p85 or p98 moves between arms; the nearest figures are l5id0012's pooled moving-track
count (0.891 and 0.880 of `a0`) and its matched-pair p85 differences (median 0.007 m/s, p95
1.97 m/s), which are per track, not the population percentile a survey publishes. Inferred: the
rows archived for l5id0012 and the evidence databases of the pcl-20261102 run hold every scored
frame's speed for every confirmed track under `a0` and seven arms, so the survey's own figures
can be computed per site and per arm without a replay. That is
[figure0001](../../../data/experiments/figure/figure0001.md), a new area, with the definition of a
track's maximum speed fixed in the spec. Which stability work the headline does and does not
depend on cannot be said until it reports.

**E17. An independent speed reference.** Measured: the repository records no radar capture
co-located in time and place with any LiDAR capture in the corpus; the radar's own transit record
(`radar_data_transits`, per-transit maximum speed) exists for the radar sites; kirk0's physical
references are reviewed poses at keyframes for three vehicles (car 8's centre a median 0.25 m
from the reference), from which a reference speed could be differenced but has not been; the
[route capture plan](../../plans/lidar-route-capture-plan.md) proposes a fixed radar beside a
moving rig. Inferred: the cheapest reference is the product's own OPS243 beside the LiDAR for one
20-minute placement at a straight corpus site, giving a per-transit maximum speed for every
vehicle both sensors see; the radar's cosine correction is already configured per site. It is a
capture, so it is the owner's decision below, not a spec here. Nothing can tell yet how far a
LiDAR track's maximum speed is from a vehicle's.

**E18. Per-track confidence.** Inferred from what the rows already carry: observed span (s),
the share of a track's frames observed rather than coasted, median return count, median range,
whether the track was ever in a duplicate pair, whether it was shortened (carried in pieces),
whether it was born or died inside the field of view rather than at its edge, and whether its
class changed. Nothing can tell yet which of these predicts a wrong maximum speed, because no
speed reference exists (E17); the kirk0 references give centres and headings, not speeds. The
scorer that could judge it is the physical scorer with a speed clause, after E17's capture.

**E19. What the platform can state today.** From the reports: that a replay reproduces byte for
byte on one build; that the published box holds its returns with its axis 1.6° from the
direction of travel at the median held out; that under the defaults about one vehicle track in
four shares its vehicle with another, so counts at busy sites are high by up to 11 to 12 %, and a
measured option removes that at every site read; that on the one labelled capture identity
scores HOTA about 0.43 and IDF1 about 0.48 on 21 reviewed road users; that speeds carry no
calibrated bound. The single measurement that would most widen what it can state is label0001's
window, because every adoption and every held-out identity claim waits on it; the single capture
is E17's radar beside the LiDAR.

### F. Truth, held-out budget and process

**F20. What label0001 blocks.** Measured from the specs and backlog: the overlap end's default
(l5id0004, l5id0006, l5id0012's successor), l3bg0009's Stage B for the CMR retune, l5id0011's
held-out clause, the geometry line's W7 score, l8fol0005's first held-out following score,
label0002's bodies and following records, and any track gate (l6cls0002 and l6cls0003 both name
it). Inferred: the overlap end decision needs less than the full window. kirk1's reference
(60a4774c) is 49 road users as spans with no per-frame masks, and l3bg0022 and l5id0012 read it
as "covered when any track overlaps 30 % of the span": that product takes about two hours, not
eight, and it decides l5id0012's clause 1 (a lost real vehicle is a span no track covers) and the
counts with truth, though not HOTA. [label0003](../../../data/experiments/label/label0003.md)
proposes spans first on label0001's own lot-drawn window, so no second window is spent and
label0001's masks follow on the same segment.

**F21. The held-out ledger.** Measured from the archive index: 20 of the 24 sites hold four
five-minute segments (20 to 23 minutes), four hold three, lombard-laguna seven and
columbus-broadway six; embarcadero-folsom is the declared held-out site and label0001's lot. Read
so far for identity, noise and counts: the first segments (every screen), the second (l5id0004
duplicates and frames, l6cls0003 clutter), the third (l5id0005 duplicates, l5id0012 every
measure), the fourth (l5id0006 duplicates, the gates), the fifth where it exists (l5id0003, 9
sites). Inferred: the label-free held-out budget for identity, noise and counts is spent on every
corpus site. What remains unread for those measures is embarcadero-folsom (which must stay unread
label-free, or label0001 is tuned on), lombard-laguna's sixth and seventh segments and
columbus-broadway's sixth, and kirk1 for noise. Settling and ground have read only the first
segments, so the second to fourth are held out for them. The ledger this review proposes, kept in
each area's README from now on: a table of site-segment by measure family (identity, noise and
counts; geometry; settling; ground), where a segment read once for a family is tuning for that
family afterwards, and a spec names its held-out cases from the unread cells only. The rule for
the next round: no identity, noise or count experiment claims a label-free held-out read on the
corpus; its corpus read is a screen, and its held-out clause is label0003's spans or label0001's
masks.

**F22. Provenance.** Measured: the noise and splits run's tools were built with a bare
`go build -tags=pcap` and stamp `dev`; l5id0012 and l3bg0022 built with the Makefile's LDFLAGS;
the two `a0` kirk0 scores differ (A3) with no recorded cause; the gerardus-window-consensus report
reused no pose0001 export because the defaults' L3 had changed, which was the right call and was
recorded. Inferred: two checks would catch this before a run. First, the run-experiments preflight
refuses to start a replay when `<tool> version` reports `dev` or a sha other than HEAD, as the
scene-assets preflight already does for the server. Second, an `a0` anchor: each spec that scores
kirk0 records in its provenance the `a0` HOTA, IDF1 and switch count the last answered spec in its
area reported, and a run whose `a0` differs stops and records the cause before any candidate is
read. The backlog's P0 "experiment specs pin the tuning fingerprint at creation" is the same idea
for parameters and should carry the anchor too.

**F23. Rules for the next round.** Yes to both. Sites as the unit with a worst leave-one-out
line, as l6cls0002 wrote it, caught the +46 % site the pooled gate figures would have hidden.
Every kirk0 clause should state its bound against the `a0` bootstrap interval once l5id0015
reports; until then the specs below read a kirk0 delta under 0.01 HOTA or 10 switches as not
distinguishable, and say so.

## Where the metric stands

The chain from a capture to the three goals, each link with its measured state:

1. **Settling.** The criterion decides nothing: the shipped first check settles every scene at
   the minimum and the honest one waits out the ceiling. No scene's background is known to be
   right when it settles. l3bg0030, with its rule fixed here.
2. **Foreground.** Refined since #901: painted structure ×0.12, moving-track time ×1.97 held out.
   What clutter is made of at each site is unmeasured. l6cls0004.
3. **Clusters.** 6.5 % of 20 to 35 m instants split on kirk0; the cap bites at 10 of 23 sites and
   its identity cost is unexplained. l4clus0009.
4. **Tracks.** One vehicle track in four duplicated under the defaults; the overlap end removes
   that at every site read and is off, failing a cost account whose lost-track clause mostly
   counts the duplicates it removed. l5id0016, then l5id0014.
5. **Scores.** 21 reviewed objects on one capture, two of them trucks; no interval on any figure;
   `a0` differs by 0.001 HOTA between reports. l5id0015.
6. **Figures.** Counts, p85 and p98 by site have never been compared between arms; no speed
   reference exists. figure0001; E17's capture.
7. **Truth.** label0001 proposed since October 13; the label-free held-out budget for identity,
   noise and counts is spent. label0003, then label0001.

## The next seven days

Ranked by the defect measured, what an answer unblocks, whether it runs without a person, and
cost. Machine time is the spec's estimate for the M1 Pro or arrow-worker; an agent closes the
`manual:implement` steps, which October's runs did in 15 to 35 minutes against 2 to 6 hour person
estimates. Rescoring archived evidence happens on the host that holds it (hydra's LiDAR volume
for the pcl-20261102 evidence, the results share for l5id0012's rows), outside any checkout.

| Rank | Spec                                                                     | Question                                                                                                                                                   | Runs without a person             | Machine | Agent implement | Unblocks                                                                                                      |
| ---- | ------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------- | ------- | --------------- | ------------------------------------------------------------------------------------------------------------- |
| 1    | [l5id0016](../../../data/experiments/l5id/l5id0016.md) (new)             | l5id0012's lost and shortened clauses reread with a body-aware match on the archived rows; the brief tracks classified; the speed tail binned by track age | Yes, zero replay                  | 45m     | 2h              | Whether the overlap end's cost is the scorer's or real; what l5id0014 must remove; the 40 losses a set listed |
| 2    | [l5id0015](../../../data/experiments/l5id/l5id0015.md) (new)             | A bootstrap over kirk0's reviewed objects: how wide HOTA, IDF1 and the switch counts are, and which of the round's deltas stand                            | Yes, in the cloud on kirk0        | 20m     | 2h              | Every kirk0 clause written after it; the `a0` anchor                                                          |
| 3    | [l5id0014](../../../data/experiments/l5id/l5id0014.md) (new)             | A birth gate on the live body, with the overlap end: duplicates gone without brief tracks, under l5id0012's clauses and the body-aware match               | Yes; held-out clause waits on 6   | 2h40m   | 3h              | The candidate for label0003's spans and label0001's window; the only new lever this round                     |
| 4    | [l6cls0004](../../../data/experiments/l6cls/l6cls0004.md) (new)          | Clutter sorted into artefact, stationary road user and fragment of a mover on the archived evidence; spurious-cluster recurrence per site                  | Yes, zero replay                  | 1h      | 2h              | Which layer owns noise at which site; the clutter definition every later gate reads                           |
| 5    | [figure0001](../../../data/experiments/figure/figure0001.md) (new area)  | Vehicle count, p85 and p98 by site under every archived arm against `a0`                                                                                   | Yes, zero replay                  | 1h      | 2h              | Which stability work the headline figures depend on; the first confidence statement the survey can make       |
| 6    | [label0003](../../../data/experiments/label/label0003.md) (new)          | Road-user spans on label0001's lot-drawn window, before masks: the truth l5id0012's clause 1 and the counts need                                           | No: 2h15m operator                | none    | none            | l5id0014's held-out clause; a lost-vehicle count with truth; label0001's window drawn and cut                 |
| 7    | [l4clus0009](../../../data/experiments/l4clus/l4clus0009.md) (new)       | `nocap` with tentative priority on kirk0; the radius graph's 50 m+ split as a count; zero code                                                             | Yes, in the cloud on kirk0        | 24m     | none            | Whether the cap hides a tracker sensitivity; whether the radius graph is retired or its bar reset             |
| 8    | [l3bg0030](../../../data/experiments/l3bg/l3bg0030.md) (rule fixed here) | The relative settling criterion at 0.075, held out on the second segments, with the downstream cost of an unsettled scene reported                         | Yes                               | 1h40m   | 2h              | Whether settling can decide anything; nothing on goals 1 and 2 directly, which is why it is last              |
| 9    | [label0001](../../../data/experiments/label/label0001.md) (October 13)   | The labelled, frozen held-out window                                                                                                                       | No: 8h15m operator, 2h with spans | none    | none            | Every adoption; unchanged in rank from October, and still the critical path                                   |

**Two overnight runs.** The first is rescoring and zero-code reads on hosts that hold the
evidence: l5id0016 and figure0001 on the results share, l6cls0004 on hydra's volume, l4clus0009
and l5id0015 anywhere kirk0 is. Each needs its `manual:implement` step closed first, and all five
are scripts or a scorer flag with fixture tests, not pipeline code. The second night runs
l5id0014 and l3bg0030, the two with tracker or grid code, each rule committed with its spec
before its replays. In the cloud only kirk0 runs (l5id0015, l4clus0009, l5id0014's kirk step);
the corpus steps need the NAS or the LiDAR volume, and the rescoring steps need the archived
rows, which never enter a checkout.

**What is read first.** l5id0016's answer decides l5id0014's shape: if the brief tracks are not
re-births on the surviving body, the birth gate is the wrong lever and the spec is amended before
its replays. l5id0015's intervals decide how the rest of the round's kirk0 clauses are read.
figure0001's answer decides whether goal 3 has a problem to solve or a statement to make.

**Held-out ledger, this round.** No corpus segment is claimed as held out for identity, noise or
counts (F21). l3bg0030 reads the second segments as held out for settling, which only the first
have been read for. l6cls0004 and figure0001 read archived evidence of segments already read and
claim nothing held out. l5id0014's held-out clause is label0003's spans. embarcadero-folsom stays
unread label-free.

## Labelling and annotation priorities

| Order | Task                                                                                                                                                                                                                                    | Person time | Tooling that exists                                                                                                                    | Gates it serves                                                       |
| ----- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------- | -------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| 1     | **label0003, select and cut.** label0001's draw: `velocity lidar segments --finder random --role held_out` on embarcadero-folsom, the draw recorded, the pack cut with `annotation-clip`. An agent does everything but confirm the draw | 15m         | [segments finder](../../plans/lidar-annotation-segment-finder-plan.md), [one-step clip](point-annotation-tool.md#choosing-what-to-cut) | Everything below                                                      |
| 2     | **label0003, spans.** Every road user's first and last frame, class, and whether it is the same vehicle as another span, as kirk1's reference 60a4774c holds them; frozen with role `held_out`                                          | 2h          | The label sets and the annotation window; kirk1's reference is the worked example                                                      | l5id0014's held-out clause; the lost-vehicle count with truth; counts |
| 3     | **label0001, masks and review.** The same window's road users and full-scene labels, then the second pass that grades every proposal                                                                                                    | 7h15m       | [label sets](label-sets.md), [checks as warnings](point-annotation-tool.md#checks)                                                     | The per-frame identity comparison, l3bg0009, W7, every gate           |
| 4     | **l5id0016's forty.** Open the worst site's listed lost tracks in the visualiser and say what they are (an hour, after l5id0016 reports)                                                                                                | 1h          | The visualiser's VRLOG replay                                                                                                          | Whether the 1 % bound is met in truth where the scorer cannot say     |
| 5     | **E17's capture.** The OPS243 beside the LiDAR for one 20-minute placement at a straight corpus site, both recording                                                                                                                    | 2h          | The radar ingest and transit worker; the LiDAR recorder                                                                                | A calibrated bound on track speeds; figure0001's successor            |

About ten and a half hours for items 1 to 4; item 5 is fieldwork the owner schedules. The two
rules hold: the window is drawn by lot, never for a failure, and label0003 and label0001 are one
window, not two.

## Superseded and revised work

| Item                                                                      | Change                                                                                                                                                           | Why                                                                                                         |
| ------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| Overlap end with tentative priority on label0001 (Parked, PCL candidates) | Names l5id0016 for "what its added brief tracks are" and l5id0014 as the candidate that goes to the window; label0003's spans as the first truth it is scored on | The item asked a question with no spec; the rescoring answers it without a replay                           |
| Reacquisition and identity acceptance, v0.5.6                             | Gains l5id0016, l5id0014, l5id0015 and label0003 beside l5id0006                                                                                                 | The identity gate's evidence now has an interval and a smaller truth product                                |
| Organised PCD export and the reviewed oracle (Parked, PCL candidates)     | Parked harder: no remaining experiment in this review needs a PCL oracle; it returns if geom0013 or a Gerardus comparator is scheduled                           | Both PCL arms failed on figures an oracle would not change; the ground oracle already exists for ground0006 |
| Road prior after ground0006 (Parked)                                      | Unchanged, and off this round's path                                                                                                                             | It serves the box's base and the clip, not stability or noise                                               |
| PCL plan, Outstanding: l3bg0028 `coast` arm                               | Parked until label0001: the overlap end already removes the ghost tracks the footprint evidence was to expire                                                    | 87 to 5 switches leaves little for a coasting-evidence arm to show on kirk0                                 |
| PCL plan, Outstanding: the radius-graph port                              | Not built; l4clus0009 reads whether the 50 m+ figure is a denominator effect, and the five-point bar is the owner's to reset                                     | 2 of 5 points against a baseline that moved from 35 % to 6.5 %                                              |
| l3bg0030                                                                  | Rule fixed: ratio 0.075, held out on the second segments, the downstream cost reported                                                                           | The spec left its rule to the next run; this is that run                                                    |

Nothing is retired. geom0013, l5id0007 to l5id0011, l3bg0026, l3bg0009 and perf0001 stand as
proposed or approved; this review changes their order, not their content.

## Engineering work needed

| Work                                                                                                                                                                                                    | Size     | Reads it               | Why it matters                                                                                        |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | ---------------------- | ----------------------------------------------------------------------------------------------------- |
| `lidar_flip_cost.py --match footprint`: coverage by drawn footprint (heading, length, width grown 0.25 m) rather than a 2 m point; `--brief-origin` classifying each brief track; the speed tail by age | S        | l5id0016               | The lost-track clause as written counts what the option removes                                       |
| `lidar-ground-truth-eval perframe -bootstrap N`: resample reviewed objects with replacement, paired across arms, 5th to 95th percentiles of HOTA, IDF1, switches, fragmentations                        | S {math} | l5id0015               | No kirk0 clause has an interval; the `a0` anchor                                                      |
| `TrackerConfig.BirthGateOnBody` (replay `birth_gate_body`): no hits accrue for a tentative track inside a live confirmed body; deleted after `max_misses` such frames; reuses `liveConfirmedBodies`     | S        | l5id0014               | Removes a duplicate before it is published, so no brief track is left and no confirmed track is ended |
| `lidar_clutter_origin.py`: the three clutter classes of D13 and the 0.5 m recurrence grid of D14, from evidence databases or rows                                                                       | S        | l6cls0004              | Says which layer owns noise at each site before any filter is built                                   |
| `lidar_figure_sensitivity.py`: moving tracks, each track's maximum speed by the fixed definition, count, p85 and p98 per site and arm                                                                   | S        | figure0001             | The survey's figures have never been compared between arms                                            |
| The relative settling criterion (`settling_relative_0075`, the ratio in its name) and the spurious-cluster read in the 30 s after settling against 60 to 120 s                                          | S        | l3bg0030               | The absolute criterion decides nothing                                                                |
| Drop or redefine the region-stability criterion for warm-up                                                                                                                                             | S        | l3bg0030               | It reads 1.0 before regions exist                                                                     |
| Run preflight: refuse a replay from a tool whose `version` reports `dev` or a sha other than HEAD; record the kirk0 `a0` anchor in each spec's provenance                                               | S        | every spec             | F22: the round's tools were built bare and `a0` differs between reports                               |
| A held-out ledger table in each area's README (site-segment by measure family)                                                                                                                          | S {docs} | every spec             | F21: the label-free budget for identity, noise and counts is spent and nothing records it             |
| Span labels as a first-class label product in the annotation window (kirk1's form), frozen with a role                                                                                                  | M        | label0003              | Two hours of truth instead of eight for the lost-vehicle count                                        |
| A speed clause in the physical scorer, from differenced reviewed poses or a radar transit record                                                                                                        | M {math} | figure0001's successor | E18: nothing judges a track's maximum speed                                                           |

## What the papers say to do, and what to leave

This review did not read the briefings in full; it reads the October 18 review's verdicts and the
November 2 briefing's title. Dong et al. 2026 and Luo et al. 2025 give the one measure on this
goal, count error on a 50 m zone before and after fragment stitching, which l5id0006's implement
step was to add to `lidar-box-eval` and did not; figure0001 carries the count per site instead,
against `a0` rather than truth. Jang's crash-composition preprint (November 2) is a method row
for safety reporting and is off this path. Nothing should be fetched this week; the round's
evidence is in the archived rows, not in a paper.

## Decisions for the owner

| Decision                                                                                      | Recommendation                                                                                                                                                                        |
| --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Approve l5id0016, l5id0015, l6cls0004, figure0001 and l4clus0009 for the first overnight run  | Yes: five rescorings and zero-code reads, no default at risk, and together they say whether the round's failures were the candidates' or the measures'                                |
| Approve l5id0014 and l3bg0030 for the second night                                            | Yes, l5id0014 after l5id0016 reports; amend it first if the brief tracks are not re-births                                                                                            |
| Approve the `figure` area                                                                     | Yes: goal 3 had no area and no measure                                                                                                                                                |
| Approve label0003 and schedule its two hours, then label0001's                                | Yes, first among the person's tasks; it is the cheapest truth the identity decision can have and spends no second window                                                              |
| Reset the radius graph's five-point bar, or retire the arm                                    | Neither yet: read l4clus0009's count first. If the 50 m+ doubling is a denominator effect, a bar of two points at 20 to 35 m with no rise in the 50 m+ fragmented count is defensible |
| The `a0` anchor and the `dev`-stamp refusal in the run preflight                              | Yes; both are an hour of an agent's time and would have caught F22                                                                                                                    |
| One 20-minute placement of the OPS243 beside the LiDAR at a straight corpus site              | Schedule it; it is the only path to a calibrated speed bound, and it costs an afternoon                                                                                               |
| Whether a kirk0 delta under 0.01 HOTA or 10 switches decides anything before l5id0015 reports | No: read it as not distinguishable, and write the next rules against the interval                                                                                                     |

## Since this review was written

Three pull requests merged to main on November 5, after the review's commit, and the specs were
renumbered around them: this review's rescoring is l5id0016 and its zero-code kirk0 reads are
l4clus0009, because main's l5id0013 and l4clus0008 had taken those slugs.

- [Reacquisition acceptance on current defaults](reacquisition-cloud-acceptance-2026-11.md)
  (l5id0013, #929) puts the overlap end and tentative priority in a checked-in
  `l5.cv_kf_v1.identity` block (parameter sets `identity-body-older.json` and
  `identity-body-older-tentative.json`) and adds `lidar-ground-truth-eval perframe
-identity-gate`. Its kirk0 screen on the current build measures brief tracks 12 under `a0`, 29
  under `body-older` and 18 under `body-older-tentative`, with duplicate pairs per hundred 22.7,
  7.7 and 10.8: the brief-track cost this review's C9 asks about, on kirk0 alone. Labelled
  figures were not scored. l5id0014's and l5id0016's arms may run from those parameter sets
  instead of replay options; the rules are unchanged.
- [Motion-supported fragment joining](w3-motion-fragment-joining-cloud-2026-11.md) (l4clus0008,
  #930) joins components inside one recent, well-constrained moving body's predicted footprint:
  39 joins on kirk0, spurious clusters a frame −8.9 %, duplicates unchanged, labelled figures not
  scored. It is a temporal lever on a per-frame problem, which is what B6 infers the next gains
  will be.
- The empty-frame lifecycle (#928) advances L5 through empty and unavailable frames. It may move
  kirk0's `a0` figures again, which is one more reason l5id0015 records the anchor at its own
  commit before reading any delta.

## What this review does not establish

It ran nothing and rescored nothing: the archived rows and evidence were not reachable from where
it was written, so every classification of a failure as the measure's is an inference from the
reports' own diagnostics (the 6 m partner read, the scorer's definitions) and from the tracker's
code, not a rescored figure. The error budget's ceilings are arithmetic on kirk0, one capture
with 21 reviewed objects, and say nothing about the corpus's queues, which is where the overlap
end's earlier form failed. The birth gate's mechanism (C9) is read from the code, not observed;
l5id0016 may refute it, in which case l5id0014 is amended before its replays. The segment counts
in F21 are the archive index's minutes divided by five, not the registry's case sets, which
differ for ashbury-downey. The person-time estimates for span labels are kirk1's reference by
analogy; no operator has timed that product. The briefings were not read.

## Provenance

- Repository at `1ec1cce` (main, 2026-11-05), branch `claude/optimistic-volta-musnv5`.
- `go run ./cmd/tools/experiments next` at that commit: 30 experiments open, of which l5id0011,
  l8fol0004, perf0001 and perf0004 approved; label0001 proposed since 2026-10-13.
- Reports read in full: the noise and splits run, the overlap end as default, settling
  convergence, the PCL review and plan, the October 18 review; by Summary, Limitations and
  Recommendations: the constrained plane, window consensus, duplicate tracks, foreground
  refinement, mean cluster centre, W3, return fate, input cap, subsampled density, replay
  determinism, physical-reference pilot.
- Specs read: l3bg0022, l3bg0027, l3bg0028, l3bg0030, l4clus0005, l5id0004, l5id0006, l5id0007,
  l5id0011, l5id0012, l6cls0002, l6cls0003, ground0006, pose0003, label0001, label0002.
- Code read: `internal/lidar/l5tracks/overlap_end.go`, `tracking.go` (steps 3d and 4),
  `tracking_config.go`, `identity.go`; `internal/lidar/replayeval/experiments.go`;
  `scripts/lidar_flip_cost.py`, `scripts/lidar_noise_screen.py`;
  `cmd/tools/lidar-box-eval/main.go` (the rows header); `internal/experiments/registry.go`;
  `tools/s2-archive/site-index.json` for segment counts.
- No raw output was produced or read; nothing under a `results` directory was touched.
