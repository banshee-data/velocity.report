# Heading D2 implementation and experiment

- **Status:** Experimental implementation delivered; rollout and physical acceptance open
- **Scope:** D2.5, D2.3, D2.1, and D1.4 on root branch `dd/docs/state-est`
- **Related:** [Sprint](lidar-heading-coherence-sprint-plan.md), [Readiness review](lidar-heading-d2-readiness-review.md), [Visibility-aware maths](../../data/maths/proposals/20260905-visibility-aware-object-tracking-research.md)

## Decision

Keep `obb_axis_coherence_enabled` off by default. The implementation resolves supported
quarter-turn axis swaps and publishes an internally consistent observed envelope, but the
first same-capture comparison is mixed. A lower median course error is not enough to approve
a physical heading estimator. No change here merges the two named tracks into one object.

Once abstentions were attributed and the unbounded hold was bounded, the comparison stopped
being mixed and became negative: acceptance 11.7 points lower and median course error 5.8
degrees worse than the baseline. The breakdown says why, and it is not the axis test. Four
frames in 1207 are the quarter-turn ambiguity; 92% of abstentions are observations that fit
neither interpretation of the support reference. The blocker is the reference, so the next
move is the visibility-aware extent model, not further tuning of the axis gates. See
[Abstention attribution, the release valve, and a re-measured A/B](#abstention-attribution-the-release-valve-and-a-re-measured-ab).

## Delivered boundary

| Item | Implemented                                                                                                                                                          | Still required for acceptance                                                               |
| ---- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| D2.5 | Frozen 200-frame recorded-output fixture, source hashes, warm-up/scoring separation, settling gate, replay manifest                                                  | Adjudicated object identity, point masks, physical pose/extent labels, and additional sites |
| D2.3 | Nullable course/terminal-lock/co-location comparisons, denominators, completed-track eligibility, course sampling, optional count band, default count reward removed | Site/window count reference and human-labelled quality gates; audit custom sweep weights    |
| D2.1 | Default-off aligned/swapped axial cost test, separate pre-update support reference, ambiguity/insufficiency sources, deterministic creation-order tie-breaking       | Robustness across view changes and calibrated evidence; no posterior-confidence claim       |
| D1.4 | Fresh measured OBB projected at the published filtered centre and heading, without recursive envelope feedback                                                       | Foreground-mask coverage and physical-size validation; it is not body reconstruction        |

D2.2 association compatibility and D2.4 visualiser work remain separate. JSON class models,
body-local surfaces, and the annotation interface are not implemented by this slice.

## Mathematical contract

The candidate evaluates `(L, W, theta)` and `(W, L, theta + pi/2)`. Angular residuals use
`atan2(sin(2 delta), cos(2 delta))/2`, so reversing travel does not label the front of a car.
The cost is the sum of squared log-extent residuals scaled by 0.35 plus a weak angular
continuity term, weighted by 0.25 with a pi/4 scale. Acceptance requires cost at most 16,
a best-versus-alternative gap of at least 2, sufficient points, and relative aspect difference
at least 0.10. These constants are bounded experimental heuristics, not measured noise.

The first supported observation seeds a separate observed-support reference. Accepted support
within 85–115% of each reference dimension updates it with weight 0.1. Other observations do
not revise it. This prevents gross fragment updates, but gradual partial-view drift remains
possible. A reference seeded from a partial view can also become misleading. Neither the
reference nor its score gap is a physical dimension estimate or confidence interval.

Supported decisions feed an axial exponential smoother using the configured alpha. The old
Guard 3 does not reject an accepted candidate because the smoother lags. Ambiguous and
insufficient decisions retain the heading and record distinct sources. Missing/invalid geometry
retains the last box as stale evidence; it does not invent a new envelope.

For valid geometry, each new half-extent is the support of the raw observed rectangle on the
published axis plus the absolute centre offset on that axis. All four measured OBB corners
are therefore enclosed at the filtered centre. The preceding published envelope is never an
input. This can make the output larger, especially during turns or centre lag; containment
does not make it an accurate physical vehicle box.

## Measurement contract

Terminal episodes count distinct measurement decisions, not coasting/deleted publications.
Five locked decisions establish an episode; five accepted decisions establish recovery.
An incomplete release is `censored`. A recovered episode followed by a new sustained lock
is `unrecovered`, not historically `released`. Durations use capture timestamps. Existing
lifetime fields remain for compatibility and must not be mistaken for these terminal fields.
Offline episodes are reconstructed only from the recorded window: prefix decisions are not
stored, so these are window-local outcomes, not complete lifetime assessments.

Course comparisons require eligible moving tracks in both arms. Missing evidence omits the
delta, rather than becoming zero. Course is a direction-of-travel proxy, not body-yaw truth.
The sweep aggregate is a mean of eligible snapshot medians, not the offline median of
per-track medians. The live jitter field is raw-to-smoothed innovation, not published step
jitter, and is not interchangeable between paths.

Objective version `v2-heading-diagnostics` leaves course opt-in and removes the default
positive track-count reward. An explicitly supplied count band penalises both sides of a
declared reference interval. Missing/malformed evidence for an enabled term rejects the score
using a finite worst-score sentinel. Existing explicit legacy weights remain supported;
operators must review saved sweeps instead of assuming their weights were migrated. The three
repository sweep examples (`sweep-quality-tuning`, `sweep-overnight-10h`, and
`sweep-velocity-jitter-10h`) previously supplied positive count weights and negative heading
innovation weights; both terms are now zero in those files. No unlabelled count band or
course-only replacement weight has been invented.

Co-location counts frames with two distinct non-deleted tracks within three metres, with
scored/live frame denominators. It is a proximity review signal: two genuine vehicles can be
close. Completed tracks that were confirmed in the recorded window can enter comparison.
Temporal-overlap matching is explicitly labelled a proxy, not an identity association.
Historical fragmentation still uses final-state counts; it is not a labelled split rate.
Regenerate old analysis reports when using these new fields; a same-version cached report
may lack them.

## Same-capture A/B

The original `s2_sf_4_20260902153250_00003.pcap` was replayed on 6 September 2026 using the
current default tuning, with only the axis flag changed. Source SHA-256 is
`6d1270ccd6a9aa239b831f560cfb1db6615c28ffe0fa871e1134b8b0d651dd7b`.
Each arm processed seconds 0–55, retained the background and tracker across a 35-second
prefix, and scored seconds 35–55. Both passed the pre-scoring grid-settling check.
Each processed 551 frames, omitted 351 prefix frames, and recorded 200 frames. Sensor pose
and static-scene validity were not independently verified; the manifest explicitly says so.
The [frozen diagnostic evidence](../../internal/lidar/analysis/testdata/heading-d2-ab-evidence.json)
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

There are no assessed baseline terminal episodes in this window, so the terminal ratio delta
is absent, not zero. The candidate's ambiguous publications count as held, not as confident
successful estimates. The increased held population is a failure signal to investigate,
despite the lower median course proxy. No labelled containment or physical-heading accuracy
claim follows from this run.

## Verification and next gate

The synthetic tests cover equivalent axis swaps, reverse motion, turns through angle wrap,
square/fragment/low-point/missing observations, non-recursive corner containment, tied-cost
assignment independent of UUIDs, and live/offline terminal-episode parity. The frozen source
fixture preserves frames 1000–1199, the two full track IDs, 123 co-publications, and the
frame-1026 0.11-by-0.08-metre collapse. It is recorded output, not ground truth.

PCAP-tagged tests passed for configuration, L5 tracking, analysis, sweep, replay evaluation,
LiDAR CLI, the server, and config migration. Race tests passed for L5, analysis, and replay
evaluation. The changed-file coverage gate passed at 100% for both new internal production
files, above the required 98%. A repeated candidate run reproduced frame/track summaries and
the speed histogram; the evidence file records the UUID-independent per-track comparison.
These checks do not replace a visualiser or physical-device test.

Next, label the turning car's point membership and axial pose independently of the predicted
UUIDs. Review the candidate's rejected views and initial partial-view reference, then evaluate
foreground containment, extent inflation, identity splits, and heading error together. Repeat
on `kirk0` and another static site before considering default enablement. Add D2.2 separately
so association changes cannot conceal a heading regression. The face-aware model remains the
structural successor, not a claim that this heuristic has already implemented it.

## Abstention attribution, the release valve, and a re-measured A/B

Three changes were made on top of the slice above, in response to the first A/B
being undecidable rather than merely mixed.

| #   | Change                                                                                                                        | Why                                                                                                                       |
| --- | ----------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| 1   | Split the single `ambiguous` source into `axis_square`, `axis_no_fit`, and `ambiguous` (tie), each recorded in the stream     | Three situations with three different fixes shared one label, so "investigate the held population" had no way to proceed  |
| 2   | Added a bounded release on the axis path: after `obb_heading_lock_max_rejections` abstentions, re-seed the reference and snap | The axis path emitted no forced releases at all and held for up to 106 frames, reinstating the D1.3 ratchet in a new form |
| 3   | Report heading **acceptance** with the frames behind it, in the summary, the comparison, and the CLI                          | The two paths label accepted frames differently, so held share compared a change of vocabulary with a change of behaviour |

The release grows the reference only. An under-seeded reference and a fragment
give the same signal — a long run of observations that fit nothing — and are
separated by direction, because occlusion removes points and never adds them.
An observation larger on both axes is evidence the reference was the partial
view; a smaller one is a partial view or a scrap and may not redefine the
object. A square or invalid observation advances the run but cannot fire it,
and a tie never fires it: both interpretations fit, and choosing one on a timer
manufactures a decision the evidence does not support. Setting the rejection
limit to zero restores the unbounded hold so the valve itself can be A/B'd.

The same capture and window were replayed with the same warm-up and settling
gate. The baseline arm reproduced the frozen evidence to the digit (41 tracks,
4 confirmed, fragmentation 0.439, median 42.3 degrees, 948 accepted of 1207),
which is the determinism claim holding across a rebuild.

| Diagnostic                    |     Baseline | Candidate, frozen evidence | Candidate, with 1–3 |
| ----------------------------- | -----------: | -------------------------: | ------------------: |
| Heading acceptance            |        78.5% |                      46.2% |               66.9% |
| Accepted / held frames        |    948 / 259 |                  558 / 649 |           807 / 400 |
| Median per-track course error | 42.3 degrees |               35.6 degrees |        48.1 degrees |
| Mean per-track median         | 39.8 degrees |               40.6 degrees |        37.5 degrees |
| Longest held run              |    20 frames |                 106 frames |           32 frames |
| Forced releases               |           14 |                          0 |                   4 |
| Eligible moving tracks        |           11 |                         11 |                  11 |

The valve does what it was built to do. The longest hold falls from 106 frames
to 32, held frames fall from 649 to 400, and four releases are enough to do it,
because each one frees a track for the rest of its life.

It does not make the candidate better. Median course error is now 5.8 degrees
worse than the baseline, not 6.7 degrees better, and acceptance is still 11.7
points lower. The earlier median was measured with the reference stuck; letting
the estimator act on more frames revealed that the headings it was holding back
were not being held back for nothing. Neither figure supports enabling the flag.

### What the breakdown settles

| Reason the candidate declined | Frames | Share of held |
| ----------------------------- | -----: | ------------: |
| Fits neither interpretation   |    370 |           92% |
| No distinguishable axis       |     25 |            6% |
| Interpretations tied          |      4 |            1% |
| Geometry missing or invalid   |      1 |            0% |

Four frames in 1207 are the quarter-turn ambiguity. D2.1 exists to resolve that
ambiguity, and on this capture it is almost never asked to. Ninety-two per cent
of the time the observation matches neither interpretation of the support
reference, which is not an axis problem: it is the reference being wrong.

That relocates the blocker. The axis test cannot be evaluated on its merits
while the quantity it compares against disagrees with the observation nine
times out of ten, and no adjustment to the cost ceiling, the score gap, or the
aspect floor changes that — those gates are deciding almost nothing here. The
support reference is a running mean of raw extents, which the geometry proposal
already labels a heuristic and the visibility-aware review says must be
separated from membership evidence. This measurement is the case for doing that
before tuning D2.1 further.

The release valve inherits the same weakness, and it should be recorded as a
known risk rather than discovered later: re-seeding from an observation larger
on both axes cannot distinguish an under-seeded reference from a merged
cluster, and co-location runs at 46.5% of frames in this window. Growing to a
merge would adopt an oversized reference with the same confidence as a correct
correction. Bounding the hold was still worth doing — an unbounded one has no
recovery at all — but the valve is a floor under the failure, not a fix for it.

## Extent beliefs: completing the D2.1 declaration

The D2.1 declaration in the visibility-aware review requires selecting rectangle
representations "against a revisable, uncertainty-bearing geometry belief" and says plainly:
"Do not promote running raw extent means to physical dimensions." The first implementation did
exactly that, and the abstention breakdown above is what it cost.

§7.1 of the same document names the mechanism. A LiDAR return is censored evidence: occlusion,
range falloff and grazing incidence remove points and nothing adds them, so an observed span
is a lower bound on the dimension and says nothing about how much larger it might be. "A
running mean of partial widths shrinks the car. A raw maximum ratchets upwards on
contamination."

Three changes follow from that.

**The reference is now a corroborated maximum, not a mean.** Accepted spans go into a fixed-bin
histogram per axis, and the estimate is the largest span several observations have reached,
clamped to a road-user range. A quantile was tried first and rejected on measurement: it tracks
how often a span is seen rather than how large it is, so with a minority of good views it can
sit below the mean it was meant to replace. A span beyond the range is recorded as a conflict
rather than clamped, and a cluster already flagged as a probable merge is refused as evidence.

**The cost is split.** Aspect agreement decides which axis, because the two interpretations
differ by twice the observation's own log-aspect: a square view carries no signal and an
elongated one carries plenty, which is correct in both cases. Excess over the belief decides
whether the observation is admissible at all. Observing less than the belief now costs nothing,
because that is what occlusion does. Conflating the two is what rejected legitimate partial
views.

**A visible-support floor replaces extent as the defence against scraps.** Extent alone cannot
refuse a fragment — a short span is consistent with any longer object — so a view showing less
than a fixed fraction of the believed long axis abstains with its own reason instead.

### Measured on two captures

Same warmed windows, `--require-settled`, only the axis flag changed.

| Diagnostic              | s2_sf_4 baseline | s2_sf_4 candidate | s2-1_00006 baseline | s2-1_00006 candidate |
| ----------------------- | ---------------: | ----------------: | ------------------: | -------------------: |
| Median per-track course |         42.3 deg |      **37.3 deg** |            36.3 deg |         **10.7 deg** |
| Mean per-track median   |         39.8 deg |          39.2 deg |            33.2 deg |         **19.0 deg** |
| Heading acceptance      |            78.5% |             55.0% |               77.8% |                54.9% |
| Longest held run        |        20 frames |        131 frames |           35 frames |            90 frames |
| Eligible moving tracks  |               11 |                11 |                  10 |                   10 |

This is the first time the candidate has beaten the baseline on course error under honest
measurement, and it does so on both captures rather than one. On the reference capture the
abstentions it was aimed at halved: observations fitting neither interpretation fell from 92%
of held frames to 34%.

It is still not ready to enable. Acceptance is roughly 23 points below the baseline on both
captures, and the longest held run rose rather than fell. The abstentions did not disappear so
much as move to more honest reasons:

| Reason held                      | s2_sf_4 | s2-1_00006 |
| -------------------------------- | ------: | ---------: |
| Fits neither interpretation      |     34% |        51% |
| Too little of the object visible |     36% |        18% |
| Interpretations tied             |     28% |        29% |
| No distinguishable axis          |      2% |         2% |

Ties and low support are the two the release valve deliberately will not act on, which is why
the longest hold grew: a tie means both interpretations fit, and a scrap has no axis to seed
from. Neither can be resolved by extent evidence at all, which is the point §7.1 makes about
the thin side return carrying almost no information about full width. Resolving them needs
membership and surface evidence, which is D2.2. The remaining no-fit population is the part
still attributable to the belief itself.

Two open items follow, both recorded rather than guessed at: the visible-support fraction and
the corroboration count are unswept constants chosen on argument, not measurement; and the
beliefs are not yet surfaced in the recorded stream, so an over-estimating belief and a
genuinely occluded view are not distinguishable offline.
