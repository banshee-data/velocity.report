# Heading D2 implementation and experiment

- **Status:** Experimental implementation delivered; rollout and physical acceptance open
- **Scope:** D2.5, D2.3, D2.1, and D1.4 on root branch `dd/docs/state-est`
- **Related:** [Sprint](lidar-heading-coherence-sprint-plan.md), [Readiness review](lidar-heading-d2-readiness-review.md), [Visibility-aware maths](../../data/maths/proposals/20260905-visibility-aware-object-tracking-research.md)

## Decision

Keep `obb_axis_coherence_enabled` off by default. The implementation resolves supported
quarter-turn axis swaps and publishes an internally consistent observed envelope, but the
first same-capture comparison is mixed. A lower median course error is not enough to approve
a physical heading estimator. No change here merges the two named tracks into one object.

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
