# PCL hypothesis verification does not earn a separate duplicate-track arm

- **Status:** OQ-07 answered; standalone Greedy/Papazov arm dropped
- **Related:** [Duplicate-track evidence](l5-duplicate-tracks-2026-10.md), [PCL review](pcl-algorithm-review.md), [candidate plan](../../plans/lidar-pcl-algorithm-candidates-plan.md)
- **Source:** PCL 1.15.1, `3346843ea6b97697a1577293aa9cdf236b5c7ffb`
- **Local baseline:** `e1349cf87`, following main `94a1fb9d6`

## Disposition

Drop rank 12 as a standalone l5id experiment. GreedyVerification and PapazovHV provide static
point-support competition, but no temporal identity evidence or policy for choosing a surviving
track. Their unsupported-model penalty needs an expected-visible model that the current track
boxes do not supply. A useful adaptation would therefore have to define visibility, uncertain
body support, temporal continuity, and survivor handling before it could be scored. Those are the
existing identity and visibility workstreams, not a bounded substitution of this PCL verifier.

This is a scope decision from reviewed implementation and existing measurements, not a measured
failure of a new PCL arm. Preserve the support-before-conflict principle as a design reference.
Do not add a spec, replay selector, tuning set, or default change. Reconsider only if a concrete
residual failure and a sensor-valid evidence model justify a different hypothesis from the
current overlap-end, containment-claim, and survivor experiments.

## Claim-level source evidence

Let E be the number of unique downsampled scene indices within the inlier radius of any visible model
point, U the number of visible model points with no scene neighbour, and C the downsampled
complete-model point count. These are different populations; their ratios are not probabilities.
The scene itself is voxelised before its search tree is built ([base, lines 293–311][base-scene]).

| Claim                                                                         | Reviewed implementation                                                                                                                                                                                                                                                                                                                | Consequence for the proposed track arm                                                                                                                                                         |
| ----------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Greedy uses visible-model support and unsupported samples                     | [Initialisation, lines 57–103][greedy-init] voxelises each visible model, collects every scene neighbour within the radius, deduplicates E, and counts U                                                                                                                                                                               | Support is proximity to supplied geometry, not a reviewed vehicle identity; U is a missing neighbour, not verified free space                                                                  |
| Greedy accepts positive support after a penalty, in an initially sorted order | [Header, lines 82–164][greedy-order] sorts by E − regulariser × U; [verify, lines 111–140][greedy-verify] accepts when its mutable good count exceeds regulariser × its mutable bad count. Accepting a hypothesis decrements good information and increments bad information for every hypothesis explaining each of its scene indices | It penalises shared point support; it does not compare track ages, preserve an identity, or re-sort after every acceptance. The scores mix scene and model sampling counts                     |
| Papazov first gates each hypothesis                                           | [Initialisation, lines 63–124][pap-init] accepts U/C ≤ penalty threshold and E/C ≥ support threshold                                                                                                                                                                                                                                   | Complete-model sampling affects admission even though support and penalties are computed from visible samples; an arbitrary box surface is not an expected-return model                        |
| Papazov conflicts mean shared explained scene points                          | [Graph construction, lines 175–216][pap-graph] adds an edge when shared scene count divided by either complete-model count exceeds the conflict threshold                                                                                                                                                                              | This is neither box intersection nor a physical same-vehicle decision; disjoint supported fragments have no conflict edge                                                                      |
| Papazov selects by explained scene count, including ties                      | [Suppression, lines 131–160][pap-suppress] removes a vertex when a still-active neighbour explains at least as many scene points                                                                                                                                                                                                       | Equal-support survival can depend on model order; there is no oldest-track or history-inheritance rule                                                                                         |
| Visibility is caller-supplied or camera-based                                 | [Base, lines 195–280][base-models] accepts visible and complete models separately; default addModels disables occlusion reasoning. Its optional organised path uses focal length 525, and the unorganised path uses a projective z buffer; [projection, lines 72–119][projection] uses x/z and y/z                                     | A caller can supply LiDAR-valid visibility, so a camera adapter is not mandatory. PCL does not supply that evidence contract here; native ring/azimuth layout cannot be used as a camera image |

For Papazov, callers that bypass occlusion reasoning must also supply the complete models; the
implementation iterates that collection and uses its sizes. A current cluster can be passed as
both collections, but then there is no independent model of what should have returned. Copying a
cluster into the scene and hypothesis largely tests self-support.

## Counterexamples

These are analytical consequences of the cited loops, not executed fixtures or capture results.
They refute a general identity guarantee, not every possible adaptation.

1. **One vehicle, two disjoint fragments.** Suppose each hypothesis explains ten distinct scene
   points, has ten complete-model points and no unsupported visible samples, and neither explains
   the other's points. Greedy accepts both at its default positive regulariser. Papazov's default
   support/penalty gates accept both (1 and 0 respectively), and the zero shared count creates no
   edge. Both tracks survive even when an independent label says they are one vehicle.
2. **Identical support, different track ages.** Two Papazov hypotheses each explain the same ten
   scene points with C = 10 and U = 0. Both pass the default gates and their shared fraction of 1
   exceeds the default conflict threshold 0.02. The first visited vertex sees an active equal
   neighbour and is removed; the second survives. Reversing model order reverses the surviving
   track. The point evidence cannot choose the older identity. Defaults are in
   [hv_papazov.h, lines 96–100][pap-defaults].
3. **A hidden body has no current neighbour.** Stale body points behind an occluder contribute to
   U if they are mistakenly supplied as visible. Dropout or an uncovered beam can do the same.
   Rejecting that hypothesis does not establish that the vehicle has gone. A valid LiDAR
   visibility/coverage contract must precede this penalty.
4. **Two cars share a proximity neighbourhood.** A generous radius, pose error, or sparse scene
   can let two distinct bodies explain the same scene indices. Either verifier can then suppress
   a real neighbour. Reducing radius can instead remove support from a valid, uncertain track;
   a lower duplicate proxy alone cannot choose between these errors.

## Mapping to the existing work

The current [overlap-end implementation](../../../internal/lidar/l5tracks/overlap_end.go) is a
narrow temporal rule: compare a coasting prediction against a live confirmed track's observed
extent, grown by 0.25 m; the older-track option ends the younger identity. Members are preferred,
then retained points, then the cluster's OBB as a fallback. It is not the same algorithm as PCL
point-support competition, and the fallback is not full observed-surface evidence.

The [duplicate-track report](l5-duplicate-tracks-2026-10.md#l5id0004-on-the-bodys-returns-keeping-the-older-track)
records why the temporal policy matters. On tuning, both `body` and `body-older` reduced duplicate
pairs from 15.5 to 1.3 per hundred tracks. On the second segments of 22 sites, both reached 1.2
against a0's 15.1, but observed vehicle frames were 97.75% for `body` (failed its rule) and 101.7%
for `body-older` (passed). The third-segment replication reduced pairs from 18.5 to 1.4 with
100.5% of a0's observed frames. These are existing replay measurements, not new runs here.

That does not establish physical recall or general identity accuracy: per-site second-segment
frame ratios ranged from 83% to 187%, the proxy counts tracks rather than vehicles, and the
labelled held-out identity gate remains outstanding in those reports. On kirk0's tuning split,
`body-older` improved HOTA from 0.376 to 0.395 and IDF1 from 0.350 to 0.435, but fragmentations
rose from 48 to 53. A support-based deletion rule must not be funded from the duplicate count
alone. Of the reported 188 overlapping kirk0 frames, 121 had one observed track and one coast,
51 had two observed tracks, and 16 had two coasts; a single static verifier is not a demonstrated
answer to all three cases.

Existing owners cover the concrete follow-ups:

- [l5id0006](../../../data/experiments/l5id/l5id0006.md) tests survivor box inheritance and tentative
  priority beside body-older. PCL's survivor choice supplies neither.
- [l5id0011](../../../data/experiments/l5id/l5id0011.md) tests likelihood and containment claims
  before association, with matched full-member inputs and held-out identity gates. It is a
  different mechanism, not evidence that static verification has already been tested.
- [l3bg0028](../../../data/experiments/l3bg/l3bg0028.md) owns proposed visibility evidence; OQ-02's
  adapter failure still prevents treating native grid cells as calibrated PCL camera visibility.

A reopened proposal must state its new target cases, model/scene sampling, expected-visible and
unknown support, shared-return conflict definition, temporal survival and evidence transfer.
Then freeze matched controls and per-frame identity/retention gates before a replay. Preserve the
historical l5id0004/0005 rules and results rather than adding a retrospective PCL arm to them.

## Validation and resume point

Source paths, line ranges, admission inequalities, graph construction, and suppression were
checked against the clean reviewed checkout. Local implementation and experiment records above
were read at the stated baseline. Repository lint, documentation links, and the no-results guard
passed. The full repository test suite passed for OQ-06 before these documentation-only changes.
No Greedy/Papazov binary, fixture, replay, or benchmark was run;
no performance or identity gain is claimed. The review, plan, documentation index, development
log, and handover carry this disposition. OQ-08 is next: inspect Gerardus before deciding which
observability and descriptor-seeding recommendations are actually new.

[base-scene]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/recognition/include/pcl/recognition/hv/hypotheses_verification.h#L293-L311
[greedy-init]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/recognition/include/pcl/recognition/impl/hv/greedy_verification.hpp#L57-L103
[greedy-order]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/recognition/include/pcl/recognition/hv/greedy_verification.h#L82-L164
[greedy-verify]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/recognition/include/pcl/recognition/impl/hv/greedy_verification.hpp#L111-L140
[pap-init]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/recognition/include/pcl/recognition/impl/hv/hv_papazov.hpp#L63-L124
[pap-graph]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/recognition/include/pcl/recognition/impl/hv/hv_papazov.hpp#L175-L216
[pap-suppress]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/recognition/include/pcl/recognition/impl/hv/hv_papazov.hpp#L131-L160
[base-models]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/recognition/include/pcl/recognition/hv/hypotheses_verification.h#L195-L280
[projection]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/recognition/include/pcl/recognition/hv/occlusion_reasoning.h#L72-L119
[pap-defaults]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/recognition/include/pcl/recognition/hv/hv_papazov.h#L96-L100
