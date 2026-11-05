# Containment claims with an assignment-cardinality safeguard

<!-- ignore-style-length -->

- **Status:** Engineering safeguard passed; both development candidates failed; default off
- **Experiment:** [l5id0018](../../../data/experiments/l5id/l5id0018.md)

The optional guard prevents claim filtering from reducing the number of assignments. It
improves both unguarded claim arms on kirk0, but neither guarded arm passes the preregistered
duplicate-rate or HOTA threshold against the contained-body control. The safeguard is available
for experiments; no association, covariance, coasting or birth default changes.

## Change and question

The [original S3 screen](l5-likelihood-containment-2026-10.md) exposed a counterexample:
keeping a bidder in every cluster row can reduce the global assignment from two pairs to one.
This candidate tentatively applies the same refusals, compares maximum matching cardinality
with the original matrix, and restores the whole matrix if it fell. Originally forbidden edges
stay forbidden and surviving costs are unchanged. Accepted refusals and whole-matrix fallback
events have separate continuity counters.

The optional `association.cardinality_guard` block enables the safeguard for startup and replay.
It is absent by default; the original four S3 files and shipped tuning fingerprint are unchanged.
It preserves matching size at this stage, not identity correctness or a no-birth invariant for
the entire lifecycle. Whole-matrix fallback can discard useful refusals alongside a conflicting
one; this conservative choice keeps the invariant simple and deterministic.

The [rule](../../../data/experiments/l5id/l5id0018.md#rule-written-before-replay) was committed
before any replay. Five checked-in sets isolate the guard with and without likelihood cost:
`a0` is the contained-body control, `claim` is the original claim, `claim-guard` adds only the
guard, and `lik-claim`/`lik-claim-guard` repeat that pair with likelihood cost. The default
production configuration is not renamed `a0`; this is the same contained-body control as S3.

## Duplicate and coverage screen

| Measure                   |   `a0` | `claim` | `claim-guard` | `lik-claim` | `lik-claim-guard` |
| ------------------------- | -----: | ------: | ------------: | ----------: | ----------------: |
| Vehicle tracks            |     44 |      53 |            51 |          56 |                52 |
| Duplicate pairs           |     10 |      21 |            16 |          22 |                18 |
| Pairs per hundred tracks  | 22.727 |  39.623 |        31.373 |      39.286 |            34.615 |
| Overlapping pair-frames   |    452 |     655 |           522 |         492 |               457 |
| Vehicle-sized scored rows |   5996 |    6236 |          6178 |        6047 |              5972 |
| Observed vehicle rows     |   4523 |    4509 |          4545 |        4546 |              4570 |
| Births, whole replay      |    148 |     158 |           148 |         152 |               143 |
| Accepted claim refusals   |      0 |     415 |           351 |         316 |               250 |
| Guard fallback events     |      0 |       0 |            19 |           0 |                31 |

Without likelihood, the guard changes births from 158 to 148, switches from 129 to 78, and
duplicate pairs from 21 to 16. With likelihood, births change from 152 to 143 and pairs from
22 to 18. The corresponding duplicate-rate improvements against each unguarded arm are
20.82 % and 11.89 %. These are outcomes of the complete changed trajectories, not a count of
births individually attributed to a particular fallback event.

Against `a0`, guarded duplicate rates remain 38.04 % and 52.31 % higher. The target for a
one-third reduction is at most 15.152 pairs per hundred tracks; neither reaches it. Both retain
more than the 4,296.85 observed rows required by the 95 % local coverage clause.

## Labelled identity screen

| Measure                      |     `a0` |  `claim` | `claim-guard` | `lik-claim` | `lik-claim-guard` |
| ---------------------------- | -------: | -------: | ------------: | ----------: | ----------------: |
| HOTA                         | 0.428375 | 0.401343 |      0.401524 |    0.401904 |          0.407094 |
| IDF1                         | 0.472820 | 0.418955 |      0.430152 |    0.408088 |          0.417473 |
| Matches                      |     2207 |     2195 |          2204 |        2200 |              2208 |
| Identity switches            |       90 |      129 |            78 |          16 |                14 |
| Fragmentations               |       46 |       41 |            37 |          37 |                35 |
| False positives, upper bound |     3967 |     3964 |          4000 |        3983 |              3996 |
| False negatives              |      265 |      277 |           268 |         272 |               264 |

The minimum local HOTA is 0.418375 (`a0` minus 0.01). Both guarded arms fall below it:
0.401524 and 0.407094. They pass the switch and fragmentation bounds but fail the combined
local rule. Fewer switches alone would hide the association and detection loss reflected in
HOTA and IDF1. The original [l5id0011](../../../data/experiments/l5id/l5id0011.md) corpus and
held-out acceptance remains open; this new engineering screen does not replace or loosen it.

## Noise diagnostics

| Measure                                            |   `a0` | `claim` | `claim-guard` | `lik-claim` | `lik-claim-guard` |
| -------------------------------------------------- | -----: | ------: | ------------: | ----------: | ----------------: |
| Confirmed evidence tracks                          |     79 |      90 |            88 |          97 |                92 |
| Brief tracks                                       |     12 |      17 |            17 |          16 |                15 |
| Lasting tracks                                     |     58 |      63 |            62 |          70 |                68 |
| Near-static tracks                                 |      3 |       4 |             2 |           3 |                 3 |
| Spurious clusters per observation timestamp, proxy | 0.6947 |  0.7878 |        0.7322 |      0.7439 |            0.7012 |
| Candidate split links                              |      0 |       1 |             0 |           3 |                 1 |

The safeguard improves each unguarded arm's spurious-cluster proxy, but neither improves the
control. The guard alone also leaves brief tracks at 17 against the control's 12. These
diagnostics support keeping the candidates off; they are not a reviewed clutter classification.

## Implementation acceptance

The reported two-by-two counterexample now keeps both assignments. A one-row case keeps a
useful refusal, while a fallback restores the original costs and increments only the fallback
counter. An independent brute-force assignment oracle checks 7,380 sparse graphs with one to
four rows and two columns, plus 1,000 random rectangular cases with negative, non-finite and
forbidden costs, each repeated. No graph loses maximum matching size or changes a surviving
original cost. Config tests verify omission, fingerprint round-trips, the claim prerequisite,
the required nested `enabled` key, and startup/replay parity for both registered files.

The full Go unit suite, Go vet/lint, documentation lint, parameter-key ordering and experiment
registry checks passed. These establish the engineering safeguard; they do not establish a
physical or metric improvement. The extra two assignment solves and matrix copy run only when
the optional guard has a proposed refusal. No Pi cost was measured.

## Method and interpretation limits

Every arm used the same source build, full kirk0 capture, six-second warm-up, evidence sampling
of 1,024 points, and `require-settled=false`. The baseline tool recorded 832 rotations and
771 scored frames per run; each arm ran twice and its baseline comparison had to agree exactly.
The noise screen reads the evidence database; the box tool separately replays the same file and
configuration. Thus duplicate summaries and identity summaries are separate deterministic reads,
not one row-level trace proving the cause of a particular birth.

A vehicle track has a confirmed published box with a side at least 3 m on at least five scored
frames. A duplicate pair overlaps more than half the smaller box on at least five frames.
Pairs per hundred tracks are `100 * duplicate pairs / vehicle tracks`. Observed vehicle rows
sum measured track-frames; they are neither distinct sensor rotations nor reviewed recall.

The label-free noise proxy counts confirmed tracks, with brief meaning under one second and
near-static meaning a lasting track with less than 2 m first-to-last displacement and p95 speed
under 0.5 m/s. Its spurious-cluster rate approximates observations not taken by lasting tracks;
there is no cluster ID in that table. These are diagnostic counts, not reviewed false positives.

The per-frame evaluator scores online positions as declared baselines on the tuning split,
with the parked `obj_788622b5` ignored. The 20-object draft spans samples 61–831; after the ignore
there are 2,472 reference points in one episode. The existing evaluator's position-based match
is not physical body overlap or a containment test. Unlabelled hypotheses contribute to the
false-positive count, making absolute false positives an upper bound. No bootstrap interval or significance test was computed for this screen. HOTA/IDF1 here should not
be read as qualified final physical trajectories or independent held-out results.

The exact packet and return identities were rebuilt and verified. Decoded points have a
fingerprint different from the published pack, and background assets were omitted. The control
reproduces the published reconstructed-pack HOTA 0.4284, 90 switches and 46 fragmentations;
that consistency supports comparison on these labels, not an assertion that coordinates were
physically validated. No retention coordinate-match threshold was relaxed for this screen,
because no retention score was requested or used.

The old four-arm screen's box counts reproduce here, while observation speed persistence has
since been corrected in #935. The fresh paired controls are used for every conclusion rather
than comparing a new noise measure with an older source build.

## Provenance and reproducibility

- Source: `3815baee8698a7824546f1aec6a7326c75239bd4`, stamped `0.5.1-pre50`, clean before tool builds. Later test/report/integration commits do not identify the measured executable.
- Preregistration: `b3884e9e` with registered-worker estimate correction `999477ee`, both before the first replay. The implementation and all measurement-changing parameters were committed before tool compilation.
- Build: Go 1.27.1, Linux amd64, Makefile version flags and `pcap` tag; all tools built through Makefile targets.
- Capture: kirk0, 200,657,872 bytes, SHA-256 `2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`.
- Dataset: `banshee-data/sf-street-speeds` revision `37ef1befc1876ad860800beea1bbeaa6c9078da9`; downloaded JSON/NDJSON text, rebuilt binary payloads locally. No CDN policy bypass.
- Labels: `kirk0-reference` revision 2233, state `12b47778fdf9119e83d18e9e8e0ebeadaea1b52b5ba32b35441c83d8e8edfad8`.
- Reconstructed pack: `ds_67842cb1b22b70e7`, digest `f0ed3414d395a46463d1c263ff3c1cbfb429549588d48964fa7685a4701615dc`.
- Packets: 149,741, digest `3ffeba7610b9df86ecf7f6481f38c5130a5be63cb2506ffe2b54de204070d036`; returns digest `f0f9ca2783d7f94b4c75994084ddcb8337f43754361ff588765c166ba1ffa4b6`, both equal to published digests.
- Rebuilt points: `7860a86c32a5cee5403c9085f1c5c1a128eaa391daa91def27064f9fcbf13832`; published points: `004a847e4ada57ad2b8dafb2a6949630d76992cff6dd61ba9f49bc670cc5c5b7`. The difference's cause is not established here.
- Split: drafted `kirk0-tuning`, digest `7194bb61d23a8739d9b883bec349a984220bc5e3b9d3497773c66184d7ecfca3`, samples 61–831; parked object ignored in scoring. No independent held-out read.

| Arm               | Checked-in tuning file                                        | Normalised tuning SHA-256                                          | File SHA-256                                                       |
| ----------------- | ------------------------------------------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------ |
| `a0`              | `config/experiments/association-containment-control.json`     | `6bd780e38d5696309ab18da64348a09360ee33c878c2f7ca46206e671b264ea3` | `667f40645b39554b1672761101dcdd0378016d3cd8fe19d5f9827ffefc666d31` |
| `claim`           | `config/experiments/association-containment-claim.json`       | `b3ef9d5b7f9111d632c5529ea29e7b60e2a62e63ee90698f055ce33ceed14dcb` | `7420c9f9883332bed6eb459e2de71a42d7ccbc454b7722d99657f6ee0141fb19` |
| `claim-guard`     | `config/experiments/association-containment-claim-guard.json` | `bcf2b7ee2d1261d327ea8f54f9ac5b6c34a5bf4295721d939d1ad63c3cf2a8bc` | `e0d3197b04eafa6c5a1ef58807450db3f0c2d1b7c92a7be84234d958760201a0` |
| `lik-claim`       | `config/experiments/association-likelihood-claim.json`        | `32b3bed799066a65201a333f1b54dff9a9dd694ee53424c6ad75be0a2e5ec3db` | `6a89d51af9f28d171ba72069bf21cd865d53d962761313dae191b24411e4c434` |
| `lik-claim-guard` | `config/experiments/association-likelihood-claim-guard.json`  | `5f6c10be8698ab1cacae7ff9858df29823b5e139909480f123a1b18b67fc22e8` | `7c4dfe459a3aadec1014fed2da49884c5dd3999dff5dcec342472c6d255b3c28` |

The normalised tuning digest is the SHA-256 of the decoded configuration's JSON; the tuning
fingerprint is its first 12 hex characters. File hashes additionally pin the hand-authored bytes.

| Tool                              | Binary SHA-256                                                     |
| --------------------------------- | ------------------------------------------------------------------ |
| `lidar-state-estimation-baseline` | `2bb3e381fc69c0d79c34ffbfe4544ee094434333c056de1e65600ed75787693f` |
| `lidar-pack-rebuild`              | `123fa078796c19e972718102a6142ec4849769013c7cddaf421dce891fe670d6` |
| `lidar-annotation-split-draft`    | `b7e71d1820ed5db297a31059400168ad2898d8e49930dcdb406f8df87d841b33` |
| `lidar-box-eval`                  | `99f909eb865a990a6dd345bcedb7baee4ee5e24b07cf4d9b334f0e3ffae53e9b` |
| `lidar-ground-truth-eval`         | `4e2ec10743373bdac424403cdea2f8cc0fdebea120c517d8fbb0789fbc4f4c56` |

| Arm               | Replay seconds, including repeat | Archived uncompressed evidence SHA-256                             |
| ----------------- | -------------------------------: | ------------------------------------------------------------------ |
| `a0`              |                          251.742 | `395c493e82ed31eab8320ef134fad382244e1951063824e07df2889aa9052e86` |
| `claim`           |                           85.475 | `290f5bff79efba80fb3718b2a0bd76e1c68e7fa4ae16e8331081386b7b1712c2` |
| `claim-guard`     |                           86.192 | `2f839c142dad7dc8530f855d8d88a5df155e2b77ef34ce5641dc2f10accb9398` |
| `lik-claim`       |                           86.181 | `c85a45b1688c69434b3512645d93ffb2d6416b7c95b83b7e1e6bad38f8a1edf2` |
| `lik-claim-guard` |                           84.637 | `ace4cd430ebc037253f20e71c5f32425aeee62080283a190ba2036bb53fd12ee` |

The first control replay overlapped local Go tests and lint, so its wall time is not comparable
with the other arms. None of these wall times qualifies target-device cost. Commands, source
manifests, repeat comparisons, label scores and noise/box summaries remain in local scratch.
Each evidence database was compressed and its decompressed SHA-256 verified before the raw
copy was removed. Raw outputs are not committed; the decision figures are recorded above.

## Decision and remaining work

Ship the optional assignment-count safeguard and its regressions, with both guarded candidates
off. Neither clears this local screen, so do not spend another held-out window promoting the
same configuration or increase coasting to compensate. A future candidate needs a new lever
and preregistered rule, a body-aware review of what the duplicate and identity metrics count,
and independent reviewed/frozen truth before any default decision.

W2 already has [containment/corpus evidence](solid-body-containment-corpus-2026-10.md). This
screen does not resolve its held-share/short-box criteria (G5/G6), the population definition of
G10, or target-device cost (G12). The criteria must be met or explicitly reviewed under the
geometry plan; these assignment tests do not amend them. The native trail publication and
physical body/heading acceptance work also remains separate.
