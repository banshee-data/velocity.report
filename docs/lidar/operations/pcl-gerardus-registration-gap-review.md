# Gerardus already reports observability; descriptor proposals remain new

- **Status:** OQ-08 answered; Item 6 narrowed to a conditional initial-proposal comparison
- **Related:** [PCL review](pcl-algorithm-review.md), [candidate plan](../../plans/lidar-pcl-algorithm-candidates-plan.md#item-6-q3-recommendations-to-gerardus), [pose0001](gerardus-static-registration-2026-10.md), [pose0002](gerardus-background-window-2026-10.md)
- **Gerardus Git SHA:** main `a2c2ac10e6497c26d973488e38aba394c93ec345`, verified against GitHub on October 28, 2026
- **PCL Git SHA:** `3346843ea6b97697a1577293aa9cdf236b5c7ffb`, 1.15.1
- **Local baseline:** velocity.report `342211be9448183c278a5cdd64a5a61ac7290501`

## Disposition

Do not ask Gerardus to add a generic observability report: it already computes a weighted
point-to-plane information matrix, scaled eigenvalues, constraint rank, and named weak directions.
Rank participates in candidate gating, the API returns the diagnostics, and the CLI prints rank
and weak directions. OQ-01 correctly qualified the PCL primitive but had not checked whether
Gerardus already supplied the proposed behaviour. The broad recommendation is superseded.

Descriptor or congruent-set initial proposals remain a new comparison for the inspected static
registration path. Keep this as one conditional Gerardus proposal, not a runtime PCL dependency
or a funded campaign here. The current search already explores multiple raster peaks at every
heading; descriptor proposals must earn their place against that control. Planned mobile Scan
Context retrieval is a different, scan-to-scan loop-closure question.

Change-map attribution is not closed by this inspection. Export cleanup remains OQ-09, with the
OQ-02 adapter limitation and pose0002's negative window result preserved. No recommendation was
posted to another repository, and no Gerardus code or accepted pose was changed.

## Provenance and inspection boundary

The available Gerardus checkout was clean on `codex/bound-registration-options` at
`dc55cdb990d7482b756a745133ee07b64bf0d14b`. Its `origin/main` was `a2c2ac10`; the GitHub CLI confirmed
that exact remote main SHA. The local branch is not main: this report cites files read with `git show` from main's Git SHA.
The registration implementation and the two requested documents are identical between those
revisions; other files differ and use main's own line numbers below.

Read [REGISTRATION-STATUS][status], the [October 24 maths note][maths], its linked review, and the
implementation bodies. The status page explicitly preserves older measurements below dated
updates: its old statement that no capture is registered is not a current deployment inventory.
The pose0001 run produced six CANDIDATE sites out of 24 and accepted no poses. It did not establish
six physically validated registrations. No live deployment records were inspected in OQ-08.

## Claim-level evidence

| Claim                                                              | Main implementation                                                                                                                                                                                                                    | Disposition                                                                                                                                                |
| ------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Matched geometry already supplies information                      | [register.rs 789–865][information] forms J from the matched normal and sensor-relative lever arm, with Huber, reference and scan weights; Fit.info takes the final undamped accumulation                                               | Existing. Optimiser damping is not added to the reported information                                                                                       |
| Scaling, rank, and weak directions already exist                   | [register.rs 981–1019][constraints] normalises by fit weight, scales rotations with a 10 m lever, sorts eigenvalues, counts values at least 0.001 of the largest, and names directions below 0.01                                      | Existing. The weak labels summarise the dominant translational or rotational component; they are not a full exported eigenbasis or calibrated error bounds |
| These diagnostics reach callers and a gate                         | [quality 887–930][quality] fills FitQuality; [gates 1176–1199][gates] requires rank 6. [The schema 582–607][proto] carries eigenvalues, rank and weak strings; [CLI 1055–1082][cli] prints rank and weak strings                       | Existing observable behaviour, not an unused helper. The CLI does not print the full spectrum                                                              |
| Formal covariance is already separate from the rank report         | [register.rs 1027–1040][covariance] returns sigma squared times the inverse information when finite; the [maths note, section 3.3][maths] already discusses correlated residuals and effective sample size                             | Existing, with known calibration limits. No field-calibrated uncertainty claim follows from this implementation                                            |
| Initial proposals are raster search peaks                          | [register_against 1092–1139][proposals] searches around the hint's position over 360 degrees, retains spatial/angular peaks, refines and fits them, then merges fitted candidates within 0.5 m and 2 degrees                           | Existing multihypothesis search, not descriptor matching and not a single eye-read pose passed straight to ICP                                             |
| Seed agreement is a local repeatability check                      | [register.rs 1144–1187][seeds] perturbs the best pose and counts fits within 0.10 m and 0.5 degrees; those fits do not replace the ranked candidates                                                                                   | Existing. Increasing seed count does not introduce a new global proposal source                                                                            |
| Separation and rank answer different questions                     | [gates 1176–1199][gates] compares the runner-up and best costs separately from rank; [maths sections 1.4–1.5][maths] already distinguish local curvature from repeated-street alternatives                                             | Existing concern. Descriptor proposals do not fix the definition of distinct alternatives or justify relaxing the gate                                     |
| FPFH/prerejective or KFPCS initialisation is absent from this path | The complete register_against flow above has no feature-matching stage. Searches of reviewed src, tests, docs, and Cargo.toml found no FPFH, prerejective or KFPCS implementation; descriptor hits in code concern protobuf reflection | New candidate comparison. [Mobile Scan Context][mobile] is a plan for loop retrieval, not implemented static scan-to-aerial seeding                        |

Existing tests `a_corridor_leaves_position_along_the_street_free` and
`information_rank_agrees_with_constraints` assert a refused corridor, its named along-street
weakness, and invariance of rank/direction under positive information scaling
([register.rs 1413–1473][tests]). They were read, not rerun. This checkpoint establishes the
implementation and contract; it does not repeat Gerardus's physical or synthetic validation.

## Counterexamples and what remains worth asking

These are analytical limits, not new measured failures:

1. A ground plane plus parallel walls leaves along-street translation free. That is already
   Gerardus's corridor test. Adding another condition-number calculation would duplicate its
   diagnostics, not repair the missing geometry.
2. Repeated recesses can produce two well-constrained local minima one frontage apart. Each can
   have full local rank; both a raster search and descriptor matching may find the wrong one.
   Rank, low residual, and many agreeing local seeds do not select the physical street position.
3. Aerial LiDAR may see roofs while a kerbside scan sees lower facades, with different density and
   occlusion. A descriptor has no matching evidence where surfaces do not overlap. Congruent sets
   can likewise match a repeated pattern. A wider proposal mechanism is not new scene evidence.
4. A mixed translation/rotation weak eigenvector is reduced to a dominant-component label. The
   existing report is useful, but its words cannot be treated as an exact six-dimensional null
   direction or a confidence interval. A richer export would be a separate interface decision,
   not the original missing-observability claim.

The remaining Item 6 proposal is an offline, matched-budget screen of one initial-proposal route
against the current raster search. FPFH with prerejective matching and KFPCS are alternatives,
not synonyms; select one after checking surface overlap and feature stability. Freeze scan and
reference revisions, preprocessing, reference coverage, proposal budget, randomness, duplicate
basin handling, and the same ICP/refusal gates before scoring. Keep the existing search as the
control; a wider reference cut or more compute is a separate treatment. Record unique useful
basins, wrong-pose proposals, abstentions, physical-reference error where available, and cost.
Do not count newly passing candidates as physically correct without independent pose evidence.

Gerardus already owns the distinct-alternative concern and the along-street analysis in the
October 24 review. Do not present them as new PCL discoveries. Any proposal must retain those
limits, pose0001's acceptance discipline, and a held-out check before a default changes.

## Validation and exact next step

The primary source paths and line ranges were checked against the frozen Git objects. Searches
and call tracing cover static registration, not every possible future branch or private plugin.
No Gerardus build, registration run, PCL descriptor experiment, or deployment acceptance ran.
Source snapshots and command logs remain outside the checkout; this report is the engineering
record. Repository `make test`, `make lint`, final documentation checks, relative links, and
the no-results guard passed. The temporary dependency link and generated macOS BuildInfo stamp
were removed or restored before staging.

OQ-09 is next: decide whether moving least squares or statistical outlier removal merits a
bounded background-cleanup comparison against the guarded input. Read pose0002 first: none of
60, 90 or 180 s passed its improvement rule against 300 s. That does not prove background content
is irrelevant. Preserve its result, the remaining facade mismatch, valid sparse-edge support,
and OQ-02's sensor-model constraint; do not restart the window sweep.

[status]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/docs/REGISTRATION-STATUS.md#L1-L44
[maths]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/docs/research/2026-10-24-registration-and-slam-maths.md
[information]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/register.rs#L789-L865
[constraints]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/register.rs#L981-L1019
[quality]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/register.rs#L887-L930
[gates]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/register.rs#L1176-L1199
[proto]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/proto/gerardus/align/v1alpha1/align.proto#L582-L607
[cli]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/main.rs#L1055-L1082
[covariance]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/register.rs#L1027-L1040
[proposals]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/register.rs#L1092-L1139
[seeds]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/register.rs#L1144-L1187
[mobile]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/docs/plans/MOBILE-SLAM-REPLAY.md#L69-L80
[tests]: https://github.com/banshee-data/gerardus/blob/a2c2ac10e6497c26d973488e38aba394c93ec345/src/register.rs#L1413-L1473
