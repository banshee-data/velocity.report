# Heading D2 implementation and experiment

This report separates the current experimental tracker from earlier A/B revisions and the
physical-heading and identity evidence still needed to enable it.

- **Status:** Experimental implementation delivered; rollout and physical acceptance open
- **Canonical:** [Tracking maths](../../data/maths/tracking-maths.md)
- **Scope:** D1.4 and D2.1–D2.3/D2.5 on root branch `dd/docs/state-est`; D2.4 remains open
- **Related:** [Sprint](lidar-heading-coherence-sprint-plan.md), [Readiness review](lidar-heading-d2-readiness-review.md), [Visibility-aware maths](../../data/maths/proposals/20260905-visibility-aware-object-tracking-research.md)

## Decision

Keep `obb_axis_coherence_enabled` false and `association_extent_cost_weight` zero by
default. The current committed experiment is the corroborated extent belief from `42265394c`
plus the bounded association term from `c863b09cb`. It is not the original running-mean
reference or a validated physical-geometry estimator.

| Evidence revision                | What changed                                                           | Current interpretation                                       |
| -------------------------------- | ---------------------------------------------------------------------- | ------------------------------------------------------------ |
| Initial D2 slice, `c73f9be77`    | Symmetric extent cost and slowly updated support mean                  | Historical baseline; replaced                                |
| Attribution/release, `71d590d1f` | Distinct abstentions, conditional reference release, acceptance metric | Retained mechanisms; the reference was subsequently replaced |
| Extent belief, `42265394c`       | Corroborated lower-bound histogram; aspect and excess costs separated  | Current committed axial reference                            |
| D2.2, `c863b09cb`                | Bounded association extent cost and box-overlap diagnostic             | Implemented, default disabled; not a proven identity fix     |

The latest two-capture association comparison is inconclusive: overlap candidates fall, but course
errors move in opposite directions and eligible track populations differ. Physical-object reference
masks are the next dependency. Annotation pack/export work is now committed in `71c3a46d7`, with
CLI error-path coverage extended in `6252be7f2`; neither a painting client nor labelled acceptance
is complete. See the [branch audit](lidar-state-estimation-branch-audit.md).

Sections headed “Historical” preserve the earlier experiment and its configuration. They must not
be read as the current algorithm or as evidence that a later model passed.

## Delivered boundary

| Item | Implemented                                                                                                                                                          | Still required for acceptance                                                               |
| ---- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| D2.5 | Frozen 200-frame recorded-output fixture, source hashes, warm-up/scoring separation, settling gate, replay manifest                                                  | Adjudicated object identity, point masks, physical pose/extent labels, and additional sites |
| D2.3 | Nullable course/terminal-lock/co-location comparisons, denominators, completed-track eligibility, course sampling, optional count band, default count reward removed | Site/window count reference and human-labelled quality gates; audit custom sweep weights    |
| D2.1 | Default-off aligned/swapped axial cost test, pre-update corroborated extent histogram, attributed abstentions, deterministic creation-order tie-breaking             | Robustness across view changes and calibrated evidence; no posterior-confidence claim       |
| D2.2 | Bounded asymmetric extent association cost, zero default weight, overlapping-box candidate diagnostic                                                                | Independent identity labels; partial-view, merge, and competing-vehicle acceptance          |
| D1.4 | Fresh measured OBB projected at the published filtered centre and heading, without recursive envelope feedback                                                       | Foreground-mask coverage and physical-size validation; it is not body reconstruction        |

D2.2 association compatibility is implemented as a disabled experiment, as detailed below. D2.4
visualiser work remains open. JSON class models, body-local surfaces, and the annotation selection
interface are not implemented by this slice.

## Current mathematical contract

Compare `(L, W, theta)` with `(W, L, theta + pi/2)` against the pre-update extent belief. The
axial residual is `atan2(sin(2 delta), cos(2 delta))/2`: it does not identify the front of a
vehicle. The cost combines log-aspect disagreement (scale 0.5), positive log-excess above the
believed spans (scale 0.30), and weak angular continuity (weight 0.25, scale pi/4). Acceptance
requires cost at most 16, margin at least 2, relative aspect difference at least 0.10, sufficient
points, and the declared visible-support floor (0.30 of the believed length). These are
experimental heuristics, not calibrated likelihoods.

The reference uses 0.25-metre bins over spans below 16 metres and the largest span reached by three
admitted observations; fewer observations use the available support. Gross out-of-range spans count
as conflicts. Known merge candidates do not revise an established belief. Lower observed spans are
compatible with censoring; they are not complete physical dimensions. Correct body-axis and
membership interpretation remains an assumption. A sustained merge can still corroborate an
inflated extent, so this is not a complete outlier or visibility model.

Accepted headings feed the axial smoother. Abstentions distinguish invalid/insufficient geometry,
near-square support, too little visible support, no fit, and tied interpretations. The release
mechanism is conditional, not a universal timeout: it must not pick a tied axis or re-seed from a
scrap merely because time passed. Belief support/conflicts and view provenance still need a durable
diagnostic contract; no posterior-confidence claim is warranted.

For valid input, D1.4 projects the fresh measured OBB at the published filtered centre and
heading. Each output half-extent includes the observed rectangle's support and the centre
offset on that axis. The previous envelope is never an input. Missing geometry leaves
stale box evidence; valid projection can enlarge a box during turning or centre lag.
Neither case reconstructs the unseen vehicle body.

D2.2 adds a bounded asymmetric extent penalty to geometrically admissible associations, capped
at a quarter of the configured gating threshold. Position remains dominant. Turning this term
on replaces the narrow hard fragment refusal; it does not itself establish membership or
correct the medoid position measurement.

## Measurement contract

Terminal episodes count distinct measurement decisions, not coasting/deleted publications. Five
locked decisions establish an episode; five accepted decisions establish recovery. An incomplete
release is `censored`. A recovered episode followed by a new sustained lock is `unrecovered`, not
historically `released`. Durations use capture timestamps. Existing lifetime fields remain for
compatibility and must not be mistaken for these terminal fields. Offline episodes are
reconstructed only from the recorded window: prefix decisions are not stored, so these are
window-local outcomes, not complete lifetime assessments.

Course comparisons require eligible moving tracks in both arms. Missing evidence omits the
delta, rather than becoming zero. Course is a direction-of-travel proxy, not body-yaw truth.
The sweep aggregate is a mean of eligible snapshot medians, not the offline median of
per-track medians. The live jitter field is raw-to-smoothed innovation, not published step
jitter, and is not interchangeable between paths.

Objective version `v2-heading-diagnostics` leaves course opt-in and removes the default positive
track-count reward. An explicitly supplied count band penalises both sides of a declared reference
interval. Missing/malformed evidence for an enabled term rejects the score using a finite
worst-score sentinel. Existing explicit legacy weights remain supported; operators must review
saved sweeps instead of assuming their weights were migrated. The three repository sweep examples
(`sweep-quality-tuning`, `sweep-overnight-10h`, and `sweep-velocity-jitter-10h`) previously
supplied positive count weights and negative heading innovation weights; both terms are now zero in
those files. No unlabelled count band or course-only replacement weight has been invented.

Co-location counts frames with two distinct non-deleted tracks within three metres, with
scored/live frame denominators. It is a proximity review signal: two genuine vehicles can be close.
Completed tracks that were confirmed in the recorded window can enter comparison. Temporal-overlap
matching is explicitly labelled a proxy, not an identity association. Historical fragmentation
still uses final-state counts; it is not a labelled split rate. Regenerate old analysis reports
when using these new fields; a same-version cached report may lack them.

## Historical initial same-capture A/B

The original `s2_sf_4_20260902153250_00003.pcap` was replayed on 6 September 2026 using the current
default tuning, with only the axis flag changed. Source SHA-256 is
`6d1270ccd6a9aa239b831f560cfb1db6615c28ffe0fa871e1134b8b0d651dd7b`. Each arm processed seconds
0–55, retained the background and tracker across a 35-second prefix, and scored seconds 35–55. Both
passed the pre-scoring grid-settling check. Each processed 551 frames, omitted 351 prefix frames,
and recorded 200 frames. Sensor pose and static-scene validity were not independently verified; the
manifest explicitly says so. The
[frozen diagnostic evidence](../../internal/lidar/analysis/testdata/heading-d2-ab-evidence.json)
retains manifests, full summary precision, comparison deltas, repeat checks, and the executable
SHA-256. The build is explicitly stamped as uncommitted work on `4d3bc0fe5`, not a release.

| Diagnostic                                                             |     Baseline |    Candidate |
| ---------------------------------------------------------------------- | -----------: | -----------: |
| Eligible moving tracks                                                 |           11 |           11 |
| Median per-track course error                                          | 42.3 degrees | 35.6 degrees |
| Mean per-track median course error                                     | 39.8 degrees | 40.6 degrees |
| Total tracks / final confirmed tracks                                  |       41 / 4 |       41 / 4 |
| Historical fragmentation ratio                                         |        0.439 |        0.439 |
| Co-location frames / scored frames                                     |     93 / 200 |     93 / 200 |
| Held-source publication share, legacy frame metric                     |        21.5% |        53.8% |
| Longest held-source publication run                                    |    20 frames |   106 frames |
| Terminal unrecovered / recovered / censored episodes, counted by track |    0 / 0 / 1 |    3 / 2 / 4 |

There are no assessed baseline terminal episodes in this window, so the terminal ratio
delta is absent, not zero. The candidate's ambiguous publications count as held, not as
confident successful estimates. The increased held population is a failure signal to
investigate, despite the lower median course proxy. No labelled containment or
physical-heading accuracy claim follows from this run.

## Verification and next gate

The test and coverage results in this section belong to the initial D2 slice, not a fresh
verification of every later commit. Subsequent experiment sections retain their own provenance.

The synthetic tests cover equivalent axis swaps, reverse motion, turns through angle wrap,
square/fragment/low-point/missing observations, non-recursive corner containment, tied-cost
assignment independent of UUIDs, and live/offline terminal-episode parity. The frozen source
fixture preserves frames 1000–1199, the two full track IDs, 123 co-publications, and the frame-1026
0.11-by-0.08-metre collapse. It is recorded output, not ground truth.

PCAP-tagged tests passed for configuration, L5 tracking, analysis, sweep, replay evaluation,
LiDAR CLI, the server, and config migration. Race tests passed for L5, analysis, and replay
evaluation. The changed-file coverage gate passed at 100% for both new internal production files,
above the required 98%. A repeated candidate run reproduced frame/track summaries and the speed
histogram; the evidence file records the UUID-independent per-track comparison. These checks do
not replace a visualiser or physical-device test.

The remaining gate is to label the turning car's point membership and axial pose independently
of the predicted UUIDs. Review the candidate's rejected views and initial partial-view
reference, then evaluate foreground containment, extent inflation, identity splits, and heading
error together. Repeat on `kirk0` and another static site before considering default
enablement. Evaluate the implemented D2.2 term separately so association changes cannot conceal
a heading regression. The face-aware model remains the structural successor, not a claim that
this heuristic has already implemented it.

## Historical abstention attribution, release valve, and re-measured A/B

Three changes were made on top of the slice above, in response to the first A/B
being undecidable rather than merely mixed.

| #   | Change                                                                                                                        | Why                                                                                                                       |
| --- | ----------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| 1   | Split the single `ambiguous` source into `axis_square`, `axis_no_fit`, and `ambiguous` (tie), each recorded in the stream     | Three situations with three different fixes shared one label, so "investigate the held population" had no way to proceed  |
| 2   | Added a bounded release on the axis path: after `obb_heading_lock_max_rejections` abstentions, re-seed the reference and snap | The axis path emitted no forced releases at all and held for up to 106 frames, reinstating the D1.3 ratchet in a new form |
| 3   | Report heading **acceptance** with the frames behind it, in the summary, the comparison, and the CLI                          | The two paths label accepted frames differently, so held share compared a change of vocabulary with a change of behaviour |

The release grows the reference only. An under-seeded reference and a fragment give the same
signal — a long run of observations that fit nothing — and are separated by direction, because
occlusion removes points and never adds them. An observation larger on both axes is evidence
the reference was the partial view; a smaller one is a partial view or a scrap and may not
redefine the object. A square or invalid observation advances the run but cannot fire it, and a
tie never fires it: both interpretations fit, and choosing one on a timer manufactures a
decision the evidence does not support. Setting the rejection limit to zero restores the
unbounded hold so the valve itself can be A/B'd.

The same capture and window were replayed with the same warm-up and settling gate. The baseline arm
reproduced the frozen evidence to the digit (41 tracks, 4 confirmed, fragmentation 0.439, median
42.3 degrees, 948 accepted of 1207), which is the determinism claim holding across a rebuild.

| Diagnostic                    |     Baseline | Candidate, frozen evidence | Candidate, with 1–3 |
| ----------------------------- | -----------: | -------------------------: | ------------------: |
| Heading acceptance            |        78.5% |                      46.2% |               66.9% |
| Accepted / held frames        |    948 / 259 |                  558 / 649 |           807 / 400 |
| Median per-track course error | 42.3 degrees |               35.6 degrees |        48.1 degrees |
| Mean per-track median         | 39.8 degrees |               40.6 degrees |        37.5 degrees |
| Longest held run              |    20 frames |                 106 frames |           32 frames |
| Forced releases               |           14 |                          0 |                   4 |
| Eligible moving tracks        |           11 |                         11 |                  11 |

The valve does what it was built to do. The longest hold falls from 106 frames to 32,
held frames fall from 649 to 400, and four releases are enough to do it, because each
one frees a track for the rest of its life.

It does not make the candidate better. Median course error is now 5.8 degrees worse than
the baseline, not 6.7 degrees better, and acceptance is still 11.7 points lower. The
earlier median was measured with the reference stuck; letting the estimator act on more
frames revealed that the headings it was holding back were not being held back for nothing.
Neither figure supports enabling the flag.

### What the breakdown settles

| Reason the candidate declined | Frames | Share of held |
| ----------------------------- | -----: | ------------: |
| Fits neither interpretation   |    370 |           92% |
| No distinguishable axis       |     25 |            6% |
| Interpretations tied          |      4 |            1% |
| Geometry missing or invalid   |      1 |            0% |

Four frames in 1207 are the quarter-turn ambiguity. D2.1 exists to resolve that ambiguity,
and on this capture it is almost never asked to. Ninety-two per cent of the time the
observation matches neither interpretation of the support reference, which is not an axis
problem: it is the reference being wrong.

That relocates the blocker. The axis test cannot be evaluated on its merits while the quantity it
compares against disagrees with the observation nine times out of ten, and no adjustment to the
cost ceiling, the score gap, or the aspect floor changes that — those gates are deciding almost
nothing here. The support reference is a running mean of raw extents, which the geometry proposal
already labels a heuristic and the visibility-aware review says must be separated from membership
evidence. This measurement is the case for doing that before tuning D2.1 further.

The release valve inherits the same weakness, and it should be recorded as a known risk rather than
discovered later: re-seeding from an observation larger on both axes cannot distinguish an
under-seeded reference from a merged cluster, and co-location runs at 46.5% of frames in this
window. Growing to a merge would adopt an oversized reference with the same confidence as a correct
correction. Bounding the hold was still worth doing — an unbounded one has no recovery at all — but
the valve is a floor under the failure, not a fix for it.

## Extent beliefs: completing the D2.1 declaration

The D2.1 declaration in the visibility-aware review requires selecting rectangle representations
"against a revisable, uncertainty-bearing geometry belief" and says plainly: "Do not promote
running raw extent means to physical dimensions." The first implementation did exactly that, and
the abstention breakdown above is what it cost.

§7.1 of the same document names the mechanism. A LiDAR return is censored evidence: occlusion,
range falloff and grazing incidence remove points and nothing adds them, so an observed span is a
lower bound on the dimension and says nothing about how much larger it might be. "A running mean of
partial widths shrinks the car. A raw maximum ratchets upwards on contamination."

Three changes follow from that.

**The reference is now a corroborated maximum, not a mean.** Accepted spans go into a fixed-bin
histogram per axis, and the estimate is the largest span several observations have reached, clamped
to a road-user range. A quantile was tried first and rejected on measurement: it tracks how often a
span is seen rather than how large it is, so with a minority of good views it can sit below the
mean it was meant to replace. A span beyond the range is recorded as a conflict rather than
clamped, and a cluster already flagged as a probable merge is refused as evidence.

**The cost is split.** Aspect agreement decides which axis, because the two interpretations differ
by twice the observation's own log-aspect: a square view carries no signal and an elongated one
carries plenty, which is correct in both cases. Excess over the belief decides whether the
observation is admissible at all. Observing less than the belief now costs nothing, because that is
what occlusion does. Conflating the two is what rejected legitimate partial views.

**A visible-support floor replaces extent as the defence against scraps.** Extent alone cannot
refuse a fragment — a short span is consistent with any longer object — so a view showing less than
a fixed fraction of the believed long axis abstains with its own reason instead.

### Measured on two captures

Same warmed windows, `--require-settled`, only the axis flag changed.

| Diagnostic              | s2_sf_4 baseline | s2_sf_4 candidate | s2-1_00006 baseline | s2-1_00006 candidate |
| ----------------------- | ---------------: | ----------------: | ------------------: | -------------------: |
| Median per-track course |         42.3 deg |      **37.3 deg** |            36.3 deg |         **10.7 deg** |
| Mean per-track median   |         39.8 deg |          39.2 deg |            33.2 deg |         **19.0 deg** |
| Heading acceptance      |            78.5% |             55.0% |               77.8% |                54.9% |
| Longest held run        |        20 frames |        131 frames |           35 frames |            90 frames |
| Eligible moving tracks  |               11 |                11 |                  10 |                   10 |

This is the first time the candidate has beaten the baseline on course error under
honest measurement, and it does so on both captures rather than one. On the reference
capture the abstentions it was aimed at halved: observations fitting neither
interpretation fell from 92% of held frames to 34%.

It is still not ready to enable. Acceptance is roughly 23 points below the baseline on
both captures, and the longest held run rose rather than fell. The abstentions did not
disappear so much as move to more honest reasons:

| Reason held                      | s2_sf_4 | s2-1_00006 |
| -------------------------------- | ------: | ---------: |
| Fits neither interpretation      |     34% |        51% |
| Too little of the object visible |     36% |        18% |
| Interpretations tied             |     28% |        29% |
| No distinguishable axis          |      2% |         2% |

Ties and low support are the two the release valve deliberately will not act on, which is why
the longest hold grew: a tie means both interpretations fit, and a scrap has no axis to seed
from. Neither can be resolved by extent evidence at all, which is the point §7.1 makes about the
thin side return carrying almost no information about full width. Resolving them needs
membership and surface evidence, which is D2.2. The remaining no-fit population is the part
still attributable to the belief itself.

Two open items follow, both recorded rather than guessed at: the visible-support fraction and the
corroboration count are unswept constants chosen on argument, not measurement; and the beliefs
are not yet surfaced in the recorded stream, so an over-estimating belief and a genuinely
occluded view are not distinguishable offline.

## D2.2: a bounded shape term in association

The D2.2 declaration asks to "compare observations with expected visible support, distinguish
membership acceptance from dimension-update acceptance, and measure duplicate identities separately
from proximity." The readiness review adds the case that matters: "A 0.11-metre fragment may belong
to the car even though it cannot measure the car's full size."

That distinction is now available. D1.5's fragment guard forbids the pairing outright, which
protects dimensions at the association gate and leaves the scrap free to seed a second track on the
same vehicle — the split-car symptom this sprint started from. Extent beliefs defend dimensions
where they are formed instead, admitting evidence only from confidently assigned, well-supported
observations, so the gate no longer has to.

Behind `association_extent_cost_weight` (default 0, which keeps the hard guard), the guard becomes
a bounded penalty added to the assignment cost. It is asymmetric for the same reason the axis cost
is: a cluster smaller than the belief is what occlusion produces on every pass and is charged
lightly, while a cluster larger than it needs a wrong belief or a merge. The term is capped at a
quarter of the gating threshold, so position remains the dominant term and the shape cost can never
push a pairing out of the gate on its own.

Overlapping-box candidates are now measured separately from proximity. `pair_frames` counts tracks
within three metres, which is not duplication: vehicles queue. `overlap_frames` counts frames where
two live boxes intersect. Estimated envelopes can overlap for distinct physical objects, so neither
metric measures verified duplicate identity.

### Measured, and inconclusive

| Diagnostic              | s2_sf_4 base | s2_sf_4 D2.2 | s2-1_00006 base | s2-1_00006 D2.2 |
| ----------------------- | -----------: | -----------: | --------------: | --------------: |
| Overlapping-box frames  |       68/200 |   **62/200** |          84/400 |      **47/400** |
| Co-located pair frames  |       93/200 |       93/200 |         131/400 |          94/400 |
| Median per-track course |     42.3 deg |     52.9 deg |        36.3 deg |        21.2 deg |
| Mean per-track median   |     39.8 deg |     43.9 deg |        33.2 deg |        30.0 deg |
| Heading acceptance      |        78.5% |        79.4% |           77.8% |           80.6% |
| Eligible moving tracks  |           11 |           10 |              10 |               9 |

What the two captures agree on: duplicate-identity candidates fall, by 9% on one capture and 44%
on the other, which is the effect the term was built for, and acceptance rises slightly on both.
Reduced fragment stranding is a hypothesis consistent with the mechanism, not an
identity-labelled causal finding from this comparison.

What they disagree on is course error, which moves in opposite directions. The medians look
dramatic — 10.6 degrees worse on one, 15.1 better on the other — but the eligible population
changed in both arms, from 11 tracks to 10 and from 10 to 9. With samples that small, dropping one
track moves a median by that much on its own. The mean of per-track medians, which is less
sensitive to which tracks are present, moves by 4.1 degrees the wrong way and 3.2 the right way.
Opposite directions remain in that statistic, but it also uses changed populations; neither
aggregate establishes a same-object improvement or regression.

So the mechanism and overlap diagnostic are delivered, and the weight stays at zero. A change
that reduces overlap candidates on both captures while moving course error in opposite
directions has not earned a default, and the reason it cannot be settled here is the one the
readiness review already named: without a physical-object annotation independent of the
predicted track UUIDs, a track appearing or disappearing from the eligible population changes
the comparison as much as the tracker does. That annotation is the next thing this workstream
needs, ahead of any further tuning of these constants.
