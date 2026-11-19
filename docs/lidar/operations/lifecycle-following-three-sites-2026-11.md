# Lifecycle body proposals and three-site following review

- **Status:** full-capture replays complete; publication, numerical comparison and browser review in progress
- **Scope:** development evidence and proposed geometry; no physical qualification

## Question and method

Can the existing reviewed-point fit carry one body through its reviewed passage, and
can an operator inspect following distances, experimental DRAC and proposed lanes in
one browser review for Ashbury–Downey, Columbus–Broadway and Embarcadero–Bryant?

The lifecycle work extends the existing physical-reference fit, draft, inspection and
save workflow in VelocityVisualiser. It does not introduce a second annotation store
or replace the joint-facet proposal tools. The following work extends the shared scene
player and the existing headless following and lane-field commands.

Each capture is processed through EOF, with a 70-second processed settling prefix
before recording. Parameters come from
[`weekend-following-baseline.json`](../../../config/experiments/weekend-following-baseline.json),
whose file SHA-256 is
`9ad7377fa5d356ba5f9d8756d48721708071da3295702350c835110b38b41675`
and tuning fingerprint is
`sha256:ab0c4035780dcc678bc21b112890c1db6cdc9c802eb3d1ab5d0f8d442e872d08`.
No arm changes tracking parameters. The preparer's observation policy retains at most
256 sample points per observation; this is part of the evidence policy, not a promise
that every original return appears in the review recording.

The replay produces both the paired following evidence and a VRLOG. The lane-field
command reuses confirmed tracks in that VRLOG, rather than tracking the capture again.
Only privacy-thresholded lane fields are uploaded to Gerardus on `arrow-worker`.
Whole-run and two-half fields retain the existing minimum-five-distinct-tracks rule.
Lane derivation saves proposals; it does not approve or adopt them.

## Frozen sources and placement

| Site               | Capture date (San Francisco) | Capture SHA-256                                                    | Accepted registration used for placement |
| ------------------ | ---------------------------- | ------------------------------------------------------------------ | ---------------------------------------- |
| Ashbury–Downey     | 2026-09-01                   | `2eec35d381fa4c4ba5a6bf1218982c76e53749287d53691115261f14d8aefbed` | `gms:01M637XHPEK6ANSKE5PZKWQG6B`         |
| Columbus–Broadway  | 2026-09-02                   | `aa69875be4dd45094d79c371ca25e434b3a4d8bec0966c116931720cc1653b66` | `gms:01M5GKWEZRV8YEVPSRTA1YGE6Y`         |
| Embarcadero–Bryant | 2026-09-03                   | `03e10c86b718f068c8733559b1020b56a4e7ae1ec0361529f4fb19e75090fdf3` | `gms:01M684AR57ZPDAHJKC6ZTABDS8`         |

All three use the explicit sensor-to-sensor identity calibration
`calibration/v1/30226f66f62ba71b24694c07b3793ab761657aa67d43b929c4cb33ad6d099e28`.
This records the replay coordinate contract; it is not a surveyed site pose.
Lane placement uses the separately accepted Gerardus record and must pass the existing
settled-background placement check before vectors enter the browser bundle. A source
window or placement mismatch refuses the context rather than substituting a nearby pose.
Lane overlays provide review context; unapproved lanes do not become leader-choice evidence.

Required captures, databases, recordings, runtime logs and browser bundles remain on
the directly attached LiDAR disk. The isolated review registry preserves the source-use
ledger; the primary server database and original annotation packs are unchanged.

## What the lifecycle proposal means

Only reviewed membership contributes. Resolved headings without detached support can
support the persistent body's dimensions. Every reviewed mask is then considered for a
pose of that same body. Sparse returns, unresolved direction, detached support and an
unbounded pose produce explicit abstentions. Hidden endpoints remain inferred.

The proposal records its pack and membership identities, supporting extent frames and
latest information time. Earlier poses explicitly use future evidence. Replacing the
draft is one undoable action and removes old draft poses at abstained frames. Saving
still follows the existing physical-reference review rules.

This slice is body memory from reviewed masks. It does not automatically discover
cross-frame facet correspondences, propagate through unreviewed frames, or fuse the
joint-facet solver into the live tracker. Inspection chooses among admitted dimension
support frames; an unsupported large box cannot establish a maximum extent.

## Reading the browser metrics

The histogram counts software tracks with an eligible estimated minimum **net time
gap**, in bins below 0.5, 0.5–1, 1–1.5, 1.5–2 and at least 2 seconds. The display also
counts tracks without an eligible estimate. These are not counts of physical vehicles
unless identity continuity has independently been established.

Colours use the minimum over eligible review coverage and therefore use later
evidence when shown at earlier frames. The timeline separately shows instantaneous
eligible time-gap minima and supported experimental DRAC maxima. Unavailable DRAC is
not zero, and a headway speed-floor exclusion does not erase independently supported
DRAC. Leader measurements require both actors at the exact exported sample and verified
coordinate agreement; projected paths follow the existing local-path geometry.
Within review coverage, the track minimum remains available at opening or excluded frames without
supplying an instantaneous leader, gap or DRAC value. A coloured box is not evidence
that a following relationship is measurable at that particular instant. Outside catalogued
stationary periods, the full timeline remains seekable but context, relationship measures
and colours are suppressed. Shaded gaps identify these periods explicitly.

Lifecycle shell proposals are reviewed separately. They do not replace the online
shells used for these DRAC exports, so a more complete proposal is not evidence of an
improved measured following gap.

## Findings and follow-up

### Reviewed lifecycle passages

The existing Kirk development review provides three independently named objects with
reviewed membership. This is separate from the three-site online following export.
The initial CLI run used producer `130020360a560b4e965d4451497e7efaba64add3`,
membership revision 2233, and membership digest
`sha256:12b47778fdf9119e83d18e9e8e0ebeadaea1b52b5ba32b35441c83d8e8edfad8`.
The 35 files within the review view hashed identically before and after the run. No
proposal was imported or accepted.

| Reviewed object      | Masks considered | Proposed poses | Abstained | Length estimate and bounds (m) | Width estimate and bounds (m) |
| -------------------- | ---------------: | -------------: | --------: | ------------------------------ | ----------------------------- |
| Truck `obj_35c661db` |              543 |            218 |       325 | 10.697 [9.651, 10.890]         | 2.472 [0.871, 2.916]          |
| Car `obj_8c728991`   |               59 |             50 |         9 | 4.261 [2.221, 4.569]           | 1.719 [1.000, 2.045]          |
| Truck `obj_8e829a06` |              412 |            205 |       207 | 9.645 [8.819, 10.046]          | 2.767 [0.860, 3.072]          |

Across 1,014 reviewed masks, 473 received a proposed pose. Of 541 abstentions,
249 had insufficient reviewed returns and 292 had unresolved heading or detached
support. These are pose-availability counts, not observed surface-area coverage.
Height remained a lower bound in all three objects: at least 3.077, 1.100 and
3.127 metres respectively. No unseen roof or underside was supplied as observed geometry.

The information horizons were capture nanoseconds `1764973091025970330`,
`1764973077726132330` and `1764973101125828330` respectively. All earlier target
frames explicitly carry the future-evidence flag. Dimension-supporting sample ranges
were 308–621, 486–525 and 477–577; not every intervening sample supported an extent.

The first truck's proposed width remains 0.436 metres below the previously reviewed
nominal 2.908 metres, although that nominal lies within the broad fitted bounds.
That disagreement is still a review priority. More posed frames do not establish a
more accurate width, endpoint or following distance.

Review also found that an ordinary additive headless import could retain old poses
at abstained frames. Lifecycle imports now explicitly request complete pose-set
replacement through the existing importer, require replacement permission and a body,
and fail on older strict readers rather than silently changing meaning.

Native review on merged main `c17907c3f129e7fc83189b341a99629bdb508eec` reproduced
the first truck's 218-pose proposal and its abstention at sample 106, where only one
reviewed return was available. The service refused to save that replacement in the
existing review view: a retained following annotation still called the truck's rear
observed at sample 491, but the refinement abstained there. This is a useful integrity
gate, not evidence that either shell is physically correct. A replacement cannot keep
an old endpoint observation attached to a pose it has removed. Body-only exploration
uses a separate published review view; the frozen baseline and its gap measurements
remain unchanged. Following measurements must be re-evaluated against any replacement
shell before comparison.

The same pass found that Undo restored the previous 135-pose shell and width 2.449 m
while leaving the 218-pose fit's support counts and inspection links visible. The
native session now invalidates those diagnostics on draft changes and restoration,
while retaining support inspection for a saved proposal. This corrects the presentation
of evidence; it does not improve the estimator's physical accuracy.

### Native proposal inspection

The existing `labels publish-view` workflow created `lifecycle-review-20261118` from
the frozen `body-fit-qa` view and membership revision 2233. The strict physical importer
then revised only the copied following records to proposed, unknown gaps and endpoints,
with an explicit body-only review method. The original numerical observations remain
in `body-fit-qa` revision 78. Saved revision 80 of the new view contains the first
truck's 218 lifecycle poses, all proposed. No physical body or pose was marked reviewed.
All 35 frozen original-view files remained byte-identical after this inspection.

The native support-frame actions selected the following cases. Return counts describe
reviewed membership, not the vehicle's surface coverage.

| Sample | Selection              | Reviewed returns |       Heading bound | Endpoint account in the proposal                         |
| -----: | ---------------------- | ---------------: | ------------------: | -------------------------------------------------------- |
|    106 | First labelled frame   |                1 |         Unavailable | Abstained; no pose supplied                              |
|    466 | Longest supported view |           16,895 | ±0.5° from geometry | Front and rear recorded as observed; proposal unreviewed |
|    390 | Widest supported view  |            1,716 |   ±9.2° from course | Front observed, rear inferred from the persistent body   |

At sample 466 the side view exposed the long body and the endpoint evidence was tighter
than at sample 390. The latter's support inspection correctly disclosed an inferred rear
and a much weaker heading bound. The persistent width interval remained 0.871–2.916 m;
the inspection does not justify narrowing it. The boundary panel counted 195 front,
146 rear, 17 left and 295 right supporting frames. Those unequal counts identify a weak
side for further review; they are neither percentages of shell area nor calibrated
confidence scores.

The next refinement should rank candidate support frames by heading and endpoint
uncertainty as well as admitted span. A large supported span can still have weak pose
constraints. Reuse the existing facet picker to establish current body-relative anchors
on well-observed frames, compare their joint constraints with the planar fallback, and
keep the opposite boundary inferred where it is hidden. Do not repair a weak match by
declaring an unseen endpoint observed. Re-evaluate following gaps only after this body
review, using the original gap references as an unchanged comparison baseline.

The proposal save took 145.383 seconds on the service and the native request timed out
before receiving its answer. The application correctly treated the write's outcome as
unknown. Reading revision 80 and reloading the saved document reconciled the view without
a duplicate save. This demonstrates recovery from a lost reply; it does not establish
acceptable storage latency.

### Full-capture following and lanes

All three preparations reached EOF successfully. The first 70 seconds condition the
background and tracker; the scored recording then continues through EOF. The table
counts recorded frames and serialised following-evidence rows, including exclusions.
Neither column counts independently identified physical vehicles.

| Site               | Recorded duration (s) | Recorded frames | Evidence rows |
| ------------------ | --------------------: | --------------: | ------------: |
| Ashbury–Downey     |             1,157.260 |          11,574 |        31,000 |
| Columbus–Broadway  |             1,962.725 |          19,629 |       459,139 |
| Embarcadero–Bryant |             1,292.246 |          12,924 |       213,147 |
| Total              |             4,412.231 |          44,127 |       703,286 |

An independent check reproduced each `source/v2` identity from its ordered capture paths,
hashes, clip, extractor and requested window, and each `calibration/v1` identity from
the observation transform's IEEE-754 values. All agreed with their completed receipts
and replay manifests. The manifests' `calibration_sha256` field hashes the parser
configuration; `observation_calibration_id` identifies the separately supplied observation
transform. These are different records and their digests must not be interchanged.

Capture replay used `130020360a560b4e965d4451497e7efaba64add3`. The numerical comparison
uses that baseline reducer and reviewed main `c17907c3f129e7fc83189b341a99629bdb508eec`.
The subsequent stationary-scope implementation supplies the review scenes and player. The
baseline does not export a second copy of the same large scenes. Producer, reducer and
viewer identities remain separate; nothing is relabelled as having been captured by a
later UI build.

Lane publication initially reread complete VRLOGs after local field extraction. The
resumed job instead sent the cached canonical field JSON through Gerardus's existing
`UploadLaneField` API, preserving the same header and 4,000-cell chunking contract.
Ashbury's whole-field verification returned the exact existing field ID
`sha256:de5cb37a59d802bc8bbdda5c1ddd3e30a4148794351b10af4a9ecf289b622c73`
and `already_stored=true`, establishing equivalence to the normal uploader for that case.
All recording and field originals were retained.

The job also calls `ListLaneProposals` and requires the new whole-field ID to appear in
the derivation's `field_ids`. This verifies that stored fields actually contributed to
the proposals. Ashbury's model includes two whole fields, one of them this run's;
Columbus's includes this run's single whole field. Site aggregation is not evidence that
capture windows are independent. Existing adoption links remain as stored; this job
does not approve, reject or adopt a lane.

All nine whole-run and half-run fields were uploaded successfully. Verified proposal
counts are five for Ashbury, 23 for Columbus and ten for Embarcadero. These are derived
lane proposals, not reviewed physical lanes. The fields use complete recordings; their
derivation does not itself establish that every contributing period was stationary.

The static-context preflight passed the settled-background placement checks for all
three accepted records. It also found that the Columbus and Embarcadero recordings span
gaps between catalogued stationary periods. Extending one placement across those gaps
would overstate the evidence. The existing reducer and player therefore gain an explicit
catalogue-bound review scope: retain every frame and raw row, but admit measurement
intervals only within stationary coverage, and require both actors' complete local-path
support to lie in the same period. Do not trim a fitted path into a different fit.
Track counts retain the whole-recording denominator, including missing estimates.
The default unscoped analysis remains available for the frozen numerical comparison.

Full-capture following histograms and browser inspection are pending. The earlier
[four-site report](following-corpus-four-sites-2026-11.md)
remains the record for the previous outputs and must not be presented as this rerun.

The next accuracy opportunities use the same tools: retain confirmed facet matchsets
through the existing feature writer, compare the joint-facet pose proposals with this
body-memory fallback, and review follower-front and leader-rear bounds independently.
Endpoint and relative-speed errors need separate measurements before any claim that
refined shells improve DRAC. Identity breaks, class eligibility and changing visible
surfaces remain part of that review.

The saved facet document in this view is revision 25 and already contains two truck
corner features and 24 accepted proposal observations. Twenty observations pin membership
digest `sha256:d1478cbfe6c753069b4f0614e3cd519fa8fd2e560194d0e8729f52fdcf27c3a4`;
four pin `sha256:8c291942c18bda783a58426634d0d737a1e68b0e3df98cee7f337846a4209629`.
Neither is the current membership digest. These are reusable inspection candidates,
not current confirmed matches. Both also retain an unknown part relation and have no
saved body-relative anchor. The next operator pass should use the existing facet
picker and saved proposal overlay to review their current memberships and body-relative
anchors, then exercise the joint proposal tools. Automatically rebinding those pins
would discard the very evidence check needed for a trustworthy match.

The existing native Facets view was inspected at sample 295. It showed both saved
features, offered navigation to their supported frames and warned that the selected
support belonged to an older object mask. Registration remained disabled. No facet
support or physical review was saved during that inspection. The existing workflow
already exposes the required gate; a second labeller is unnecessary.

## Execution limits

The attached rotating disk's intermittent synchronisation delays remain relevant;
see the [earlier storage diagnosis](broderick-vv-walkthrough-2026-11.md#attached-drive-diagnosis).
A broad local annotation test run reached the default ten-minute timeout in
`os.Stat` while creating a temporary directory for an existing facet test. It did not
report an assertion failure. The later targeted Go lifecycle, import and following tests
passed on merged main `c17907c3f129e7fc83189b341a99629bdb508eec`. The native build
on LiDAR completed its test products, but its outer 55-minute guard expired during test
startup. Running those verified products on the separately approved WD_BLACK build
volume passed all 13 selected fit tests. These observations do not establish filesystem
health or a controlled performance comparison.

An optional concurrent passive checkpoint of Columbus's observation database added
disk contention and was cancelled without claiming a completed checkpoint. Its journal
was retained. The preparer's own final checkpoint completed before its EOF receipt.
The independent SQLite `quick_check` is still running against the observation database;
a stack sample found it waiting in database reads. The following exporter reads the
separately verified recording and paired evidence, not that observation database. A
separate metadata registry allows export to proceed while the integrity check continues.
This does not establish observation-database integrity: that remains an explicit pending gate.

The follow-up native build at `3f1cae6a33a57f7d4338c3b5b49fafa7153f6655` completed
through Make on WD_BLACK and passed all 28 tests in the physical-fit and physical-session
suites. A native regression check then refined the frozen truck to 218 poses and undid
it: the interface restored Saved, 135 poses and width 2.449 m, and removed the newer fit's
diagnostics and inspection links. Nothing was saved to the original view. The fix merged
as [PR #1078](https://github.com/banshee-data/velocity.report/pull/1078), with 13 successful
CI checks and one intentional skip. Recordings, runtime evidence and screenshots remain
on LiDAR; the user-approved WD_BLACK exception covers build products and caches.

Bulk replays, lane-field publication and corpus export are serialised. Native and targeted
Go checks use the separate build workspace. Ashbury's completed preparation is preserved
in its original workspace; Columbus and Embarcadero completed in the bulk workspace.
A second, 128 GiB sparse APFS workspace on the same physical LiDAR volume accommodates
the larger captures' temporary database/WAL footprint. Only remaining output locations
change; source files, source hashes, tuning parameters, calibration and replay windows
remain frozen. The original output plan is retained beside the updated run plan.
