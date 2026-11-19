# Four-site following reduction and distance scene exports

- **Status:** Software reproducibility and export checks; provisional physical estimates
- **Question:** Can the four prepared development recordings be analysed without VV, expose the same distribution from the web, and export exact-sample following distances?
- **Related:** [Corpus workflow](following-corpus-analysis.md), [weekend plan](../../plans/lidar-weekend-following-drac-plan.md), [baseline reproducibility](following-baseline-reproducibility-2026-11.md)

The batch and software walkthrough ran on November 17–18, 2026.

## Method and population

The new corpus command reduced receipt-verified paired evidence from four explicit registered
recordings. This was one baseline reduction, with an identical repeat; there were no new
perception arms, parameter changes or packet replays. Existing paired preparations hold the
original online estimator's decisions. The registration snapshot copied only the necessary
metadata dependencies into a separate local database, leaving the primary service unchanged.

Each recording contributes one minimum estimated net time gap for each software follower track
with an eligible, positive-duration sample. Net time gap is projected front-to-rear gap divided
by follower speed. Non-closing pairs may contribute, although closing DRAC is unavailable.
Excluded pairings, missing evidence, speed-floor exclusions and terminal samples do not
contribute a value. Missing tracks stay in the denominator. Multiple fragments of one vehicle
can contribute separate minima; these counts are not independently identified vehicles.

The bins are lower-inclusive: `[0,0.5)`, `[0.5,1)`, `[1,1.5)`, `[1.5,2)` and `[2,infinity)`
seconds. Minima keep their exact capture time, leader and gap. Row counts below describe stored
samples rather than exposure seconds. Recording durations and missingness differ, so the table
is not a ranking of sites or a population traffic distribution.

## Observed results

| Site                  | Tracks | With a minimum | Without | <0.5 s | 0.5–1 s | 1–1.5 s | 1.5–2 s | 2+ s |
| --------------------- | -----: | -------------: | ------: | -----: | ------: | ------: | ------: | ---: |
| Kirkham–9th           |     71 |              1 |      70 |      0 |       0 |       0 |       0 |    1 |
| Ashbury–Downey        |    350 |             24 |     326 |      0 |       0 |       4 |       4 |   16 |
| Columbus–Broadway     |  4,130 |            140 |   3,990 |      2 |       5 |      24 |      27 |   82 |
| Embarcadero–Bryant    |  3,826 |            132 |   3,694 |      4 |       9 |      26 |      26 |   67 |
| Total software tracks |  8,377 |            297 |   8,080 |      6 |      14 |      54 |      57 |  166 |

| Site               | Evidence rows | Eligible bucket rows | Following excluded | No leader | Pairing excluded | Record gaps | Export frames | Distance samples |
| ------------------ | ------------: | -------------------: | -----------------: | --------: | ---------------: | ----------: | ------------: | ---------------: |
| Kirkham–9th        |         5,624 |                    3 |                222 |     1,521 |            3,531 |         347 |           631 |                2 |
| Ashbury–Downey     |        31,000 |                1,333 |                281 |    11,781 |           15,018 |       2,587 |        11,574 |            1,332 |
| Columbus–Broadway  |       459,139 |                6,164 |              8,267 |    90,866 |          323,783 |      30,059 |        19,629 |            6,158 |
| Embarcadero–Bryant |       213,147 |                6,417 |              4,552 |    47,072 |          133,421 |      21,685 |        12,924 |            6,402 |

The five row buckets sum to each recording's rows. There are 23 eligible-bucket rows without an
exported distance: eligibility alone does not guarantee a positive-duration measurement.
Zero-duration samples are intentionally omitted; no absent-leader rows were found in these exports. The exports contain 44,758 frames and 13,894 distance-bearing samples.
All 297 minimum value/time anchors were found at their exact exported frames. This numerical
join check does not independently validate physical actor identity.

Recorded per-frame classes do not always agree with analysis eligibility. Of the distance
samples, 2,141 are labelled `dynamic`, 171 `pedestrian`, 183 `cyclist`, 20 `bus` and 11,379 `car`
in the recordings. These classes are preserved. The scene reader now displays the evidence's
estimated distance even when the recorded class differs, without changing speed-label class
rules or promoting the actor to a car. Investigate those disagreements before treating the
histogram as vehicle behaviour.

## Reproduction and provenance

The reduction/export binary was built through Make as `v0.5.1-pre52`, producer
`c4f769eab844fc5bd5718acc2ca013447f37f10e`, built `2026-11-18T06:45:01Z`.
The later serialiser extraction and regression tests leave the numerical reduction unchanged;
the shared player's class-independent distance label was checked from its later source.
The scene header separately retains the original estimator producer, calibration and canonical
evidence identity. The reduction producer must not be substituted for the estimator producer.

The unchanged baseline parameter set is `config/experiments/weekend-following-baseline.json`:
byte SHA-256 `9ad7377fa5d356ba5f9d8756d48721708071da3295702350c835110b38b41675`,
parameter fingerprint `sha256:ab0c4035780dcc678bc21b112890c1db6cdc9c802eb3d1ab5d0f8d442e872d08`.
All four inputs retain calibration
`calibration/v1/30226f66f62ba71b24694c07b3793ab761657aa67d43b929c4cb33ad6d099e28`;
this is a sensor-frame identity calibration, not a surveyed site pose.

| Site               | Registered review ID                                    | Canonical evidence SHA-256                                         |
| ------------------ | ------------------------------------------------------- | ------------------------------------------------------------------ |
| Kirkham–9th        | `following-review-9ea0ede2-e4fc-48b7-a6a0-9f84fddd6202` | `3365f3d5b615a1313493be7c118d97a2094e841d18fb81a6f5093590a219ace9` |
| Ashbury–Downey     | `following-review-a5254a5f-82c5-4c69-a3a3-0c40f8a9fde1` | `6189612a1ef2e36efffef03be89907f967398a3faf65bafa55ebf5b448ba12ac` |
| Columbus–Broadway  | `following-review-b71ec25e-18ee-477e-994b-7b4e09cca73d` | `76cf7427c14afad57114d95a8daa50430fcd930fb481327e7b71af39fae50023` |
| Embarcadero–Bryant | `following-review-9e777570-8286-4f3a-be91-74e913b4513d` | `b961ff87666c2cddf2f70ad5e2cee44a05d3eb29efd82ca32d02dfa3dd83b427` |

Kirkham's prepared evidence producer is `c6a4f98bfecce8d9b7f6d67555df52ed488bf265`;
the other three retain `0d9e55fbe7eec91c3468b361046e60248022b396`. The method is
`following_encounter_v1/2789569b98eff6a4`, estimator `cv_kf_v1`, observation model
`published_box/medoid_v0`, stage `online`. The completed corpus retains complete source/v2,
header, binding and policy identities for every input.

The repeat without scene export produced byte-identical `complete.json`, SHA-256
`d6be738d66ce8ba844d4be4d8463f612a2d5096ea42206eadee67f680e2952d7`.
The checker read every compressed frame, checked monotone timestamps, finite geometry and
measurements, unique local actor keys, present leader references, header/evidence binding and
all minimum anchors. No raw track key appeared as a scene actor key.

Runtime evidence remains on the directly attached volume under
`/Volumes/velocity-build/work.noindex/following-development-c4-20261117`, with independent
verification and repeat receipts alongside it. That APFS mount is backed by the sparse image
on `/Volumes/lidar/work/velocity-broderick-walkthrough-20261117/`; it is not an internal-disk
build or a NAS dependency. Raw rows, recordings, databases and generated JSON remain outside Git.

## UI checks and interpretation

The web Runs action computed Kirkham's 1/70 usable/missing counts and `[0,0,0,0,1]` bins.
Selecting Ashbury cleared the previous result; its analysis showed 24/326 and `[0,0,4,4,16]`,
matching the headless output. No file paths were supplied by the browser. An isolated backend
served the copied registrations; the existing primary service was not replaced.

The current shared scene player showed Kirkham's exact 43.499492-second sample as
`estimated gap 30.8 m · 29.73 s` despite its recorded `dynamic` class. Seeking past that sample
removed the label. This verifies display and stale-value clearing, not the unusually large
estimated time gap's physical correctness. The export preserves the estimate for diagnosis.

The main limitation is missing usable evidence, followed by unresolved identity/class,
common-path, endpoint and speed accuracy. The batch makes these reviewable and reproducible;
it does not improve the frozen tracker or qualify DRAC. Broderick was not included in the
four-site development corpus. Its earlier software walkthrough exposure remains governed by
its recorded validation-cohort restrictions.

Next, review the minimum anchors together with excluded periods, starting with class
conflicts and weak endpoints. Use independently reviewed body/facet references to test gap
and speed error. Keep partial surfaces and unbounded dimensions unknown, and retain rejected
or ambiguous cases rather than filtering until the distribution looks plausible.

## Body proposal checks

The separate body-fit change uses `fit:mask_v2`: the nominal selected-view percentile is kept,
but the interval contains every selected span and its own stated bound, rounded outwards.
Every contributing frame is cited and receives a pose when one can be bounded. VV shows
resolved front/rear/left/right boundary support and opens a supported frame for inspection.
Those counts describe constraints, not a percentage of the shell's surface.

The existing Kirkham reference pack `ds_67842cb1b22b70e7`, digest
`sha256:67842cb1b22b70e7dff46aa53af48405e44ddd69503d3c43a0c7670450a1507e`,
was copied with its linked label set to an isolated attached-drive workspace. The original
33 pack files and 24 label-set files matched before the copied link was relocated into the
current capture/set/views layout. Physical revision 77, membership revision 2233 and membership
digest `sha256:12b47778fdf9119e83d18e9e8e0ebeadaea1b52b5ba32b35441c83d8e8edfad8`
opened without stale links or review drift. The original labels were not replaced.

| Reviewed object | Proposal length m [interval] | Proposal width m [interval] | Height lower bound m | Diagnostic frames | Fitted poses |
| --------------- | ---------------------------- | --------------------------- | -------------------: | ----------------: | -----------: |
| `obj_35c661db`  | 10.697 [9.651,10.890]        | 2.449 [0.742,2.834]         |                3.077 |               542 |          130 |
| `obj_8c728991`  | 4.258 [2.221,4.569]          | 1.729 [1.000,2.318]         |                1.100 |                59 |           24 |
| `obj_8e829a06`  | 9.634 [1.843,13.552]         | 2.836 [0.860,3.108]         |                3.127 |               412 |           60 |

These are software proposals generated with the existing default fit options, not surveyed
vehicle dimensions or new accepted references. The intervals reveal weak support which the
previous median-bound summary could hide. In particular, truck 1's proposed width is below
its existing reviewed nominal width of 2.908 m, and truck 2's length interval is very broad.
Neither should be promoted automatically into following measurements. Width measured on a
thin end region differs from the whole-object cross-axis extent shown in frame diagnostics;
those quantities must not be confused when checking a shell against returns. Unseen surfaces
and upper height bounds remain unknown. This work does not implement or qualify an online
persistent facet constellation, nor feed these proposals back into the frozen corpus gaps.

VV's fit panel counted front/rear/left/right support as 195/146/17/295 frames on truck 1.
Its Inspect actions opened samples 328 (front/right), 455 (rear, with both ends constrained
by the visible side) and 254 (left, a sparse 20-return view without a saved fitted pose).
The orthographic views and 3D proposal stayed tied to the chosen sample. At sample 328 the
rear remained inferred; at 455 both ends were constrained; 254 did not invent a box where
no pose was stored. These checks assess display and evidence availability, not physical truth.

The isolated proposal save committed as revision 78, retaining proposed status and resetting
its dependent reviewed poses. Attached-drive I/O delayed the response to 215.790 seconds,
beyond the native request timeout. The client retained its draft and required a reload;
reading the stored revision before continuing avoided a duplicate write. The original pack
stayed at revision 77. This is a remaining operational latency limit, not a successful
low-latency save claim. The shared facet pane loaded two retained facets (20 and 4 supported
frames) and required reviewed body/pose support for metric registration; none was promoted.

The fresh native build passed all 10 PhysicalFitTests. The walkthrough also found the long
annotation checks list hiding object selection; a collapsible list keeps review warnings
available while making the object list reachable. Support diagnostics currently describe the
current fit response; the saved body and poses persist separately. Refit to regenerate those
all-frame diagnostics after reopening rather than interpreting saved pose counts as shell coverage.

Native tests ran on source `652b221cad628663da2fd105bf167c5bedfd1250`. The subsequent Make
build at `189db745b3ecb578f4d2aeb0a64d4f7e7156cfd4` included the collapsible checks list.
After restarting that build, the warnings expanded and collapsed, object selection remained
reachable, and the saved revision 78 reopened with the proposed body and 135 keyframes intact.
