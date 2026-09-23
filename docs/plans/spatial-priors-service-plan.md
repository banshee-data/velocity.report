# Spatial priors service: experiment and engineering plan

Build a separate, open spatial-priors project that turns heterogeneous scans into
small, versioned descriptions of persistent places. First demonstrate that these priors
help a stationary LiDAR recover its pose and learn a useful background; the public
contribution platform follows that evidence.

- **Status:** Draft for review
- **Canonical:** [Geometry-prior service](../lidar/architecture/geometry-prior-service.md)
- **Scope:** Separate service/repository, controlled experiment,
  and optional velocity.report integration
- **Related:** [S2 conventions](../lidar/architecture/geographic-indexing.md),
  [vector scene map](../lidar/architecture/vector-scene-map.md),
  [static pose alignment](../lidar/operations/static-pose-alignment.md)

**Review update:** The user has selected catalogue and priors development together and identified
existing trip data and 20-minute static captures as the initial corpus. The sequence below remains
the original review draft; the [decision review](spatial-priors-service-review.md) records the
revised direction, technical findings, and choices still under discussion.

## 1. Review decisions and present state

The working project name is `spatial-priors`; naming is not an architectural commitment. This
document is a proposal, not an implementation report. Repository inspection at `2c6191ae6` found
the earlier geometry-prior architecture proposal but no Go declarations named
`ScenePriorProvider` or `PriorLoader`, or Go configuration references named `prior_service`. Do
not treat the earlier plan as shipped code.

Approve the experiment and interface direction first. Public uploads, sponsor sales, operational
service commitments, and runtime adoption require later evidence gates. No code is included here.

The earlier geometry-prior document remains the reference hub during review. If this plan is
accepted, reconcile the following decisions there before implementation:

| Earlier proposal                                                        | Proposed decision and reason                                                                                                                        |
| ----------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| GeoJSON polygons are the complete prior                                 | Retain GeoJSON for footprints and optional visual exports; use a typed 3D runtime product with frames, features, and uncertainty.                   |
| Contributions primarily arrive as geometry PRs                          | Curated scan manifests and object storage first; source code and schemas belong in Git, large scan history does not.                                |
| Remote access described as opt-in but configuration defaults to enabled | Remote access defaults to disabled. A local prior can work without GPS or a network.                                                                |
| Signatures influence geometric union weights                            | Signatures establish publisher authenticity, not geometric correctness. Quality comes from measured evidence.                                       |
| Daily polygon union                                                     | Rebuild only affected products with explicit provenance and visibility-aware evidence. A union cannot distinguish a parked vehicle from a building. |
| One aggregate file per cell                                             | One versioned manifest per cell may reference multiple products, resolutions, source licences, and boundary dependencies.                           |

## 2. Problem and non-goals

Scans vary in format, coordinate frame, precision, density, visibility, and age. A useful service
must make those differences explicit before combining observations. The product is a defensible
prior for a place: geometry, localisation landmarks, evidence, uncertainty, and change history.

`velocity.report` owns dynamic observations, tracks, traffic behaviour, and
safety metrics. The new project owns geographically registered static products.
Neither needs the other's internal database.

Non-goals are a universal SLAM engine, survey certification, autonomous-driving safety guarantees,
live traffic surveillance, complete worldwide coverage, permanent ownership of cells, mandatory
cloud access, and a neural scene representation that is the only usable export. Scanner purchase
decisions and commercial hosting commitments are outside this draft.

## 3. Subsystem scope and architecture

`MVP experiment` means work required to decide whether to proceed. `MVP` means the small curated
service built after that decision. `post-MVP` requires demonstrated demand. `probably unnecessary`
identifies complexity for which there is currently no credible need.

| Subsystem                                                              | Classification       | First useful boundary                                                              |
| ---------------------------------------------------------------------- | -------------------- | ---------------------------------------------------------------------------------- |
| Capture contract, E57/LAS/LAZ/COPC adapters                            | MVP experiment       | CLI ingestion of approved bundles; verify installed reader capabilities.           |
| Frames, trajectories, uncertainty, provenance                          | MVP experiment       | Versioned manifests and reproducible processing records.                           |
| Registration and control-point validation                              | MVP experiment       | Rigid local alignment with independent check points.                               |
| Static evidence and localisation features                              | MVP experiment       | Explainable geometric baseline and held-out evaluation.                            |
| Local velocity.report adapter and replay harness                       | MVP experiment       | Optional local-file prior; sensor-local fallback.                                  |
| S2 partitioning and immutable publication                              | MVP                  | L13 lookup, L10 grouping, bounded neighbouring products.                           |
| Catalogue and read API                                                 | MVP                  | One application and Postgres/PostGIS, static read snapshots.                       |
| Job orchestration                                                      | MVP                  | One worker process, database job table, leases, retries, and quotas.               |
| Curated contribution review, licence/privacy gates                     | MVP                  | Maintainer-approved submissions; no anonymous processing queue.                    |
| Release signing, moderation, revocation, backups                       | MVP                  | Small publisher and catalogue state machine.                                       |
| HTTP velocity.report provider                                          | MVP                  | Opt-in, bounded asynchronous fetch, validated local cache.                         |
| Coverage website                                                       | MVP                  | Static coverage and download pages, no custom 3D editor.                           |
| Self-service accounts, appeals UI, badges                              | post-MVP             | Build once manual review becomes a measured bottleneck.                            |
| Automated geographic sponsorship billing                               | post-MVP             | Trial manual sponsorship first.                                                    |
| GeoParquet exports and DuckDB analytics                                | post-MVP             | Add when bulk consumers need them; preserve exportable schemas now.                |
| Advanced semantics, meshes, seasonal models, consumer adapters         | post-MVP             | Each must improve measured downstream usefulness.                                  |
| Custom E57 decoder, custom SLAM, custom spatial database               | probably unnecessary | Reuse established implementations.                                                 |
| Kubernetes, message broker, per-stage microservices, schema repository | probably unnecessary | Revisit only for demonstrated isolation, throughput, or independent release needs. |

Run the experiment on a workstation using a single reproducible processing environment. For
the curated service, use one Go catalogue/publisher application, one Python worker invoking
PDAL/PROJ and Open3D, Postgres/PostGIS, and S3-compatible object storage. The worker may run
on the same machine; process isolation protects the API from expensive or malformed inputs.
No heavy processing occurs in an HTTP request. Publication exports static manifests, so
reading priors survives application outages.

```text
approved capture bundle or external asset reference
    -> quarantined fetch and validation
    -> normalisation / frame resolution / alignment
    -> repeated-observation evidence / static geometry / features
    -> immutable products + release manifest
    -> catalogue transaction + static cell snapshot
    -> local download or optional HTTP consumer

metadata and jobs: Postgres/PostGIS
large assets: object storage or verified external archives
review website: generated coverage pages and product links
```

## 4. Repository and consumer boundaries

The new repository owns ingestion, coordinate transformations, multi-capture alignment,
geographic fusion, quality assessment, catalogue, source attribution, publication, and
contribution governance. Keep the shared wire schema there initially, with tagged
releases and compatibility fixtures. Extract an independent module only when several
consumers need an independent release cadence.

`velocity.report/internal/prior/` would own a small decoded model, compatibility
validation, provider implementations, cache policy, and bounded retrieval. Sensor
calibration, live-to-prior pose fitting, background learning, foreground decisions, and
replay provenance remain in velocity.report. Large geospatial libraries and the server's
ingestion pipeline must not enter the sensing binary.

The conceptual `ScenePriorProvider.Prior` operation accepts cancellation, an S2 cell, an optional
pinned release, and supported product/schema versions; it returns an immutable scene prior with
provenance or a typed error. The simple cell-only interface from the prompt is a useful starting
point, but version selection and neighbouring coverage must not become hidden global state.

| Provider             | Contract                                                                                       |
| -------------------- | ---------------------------------------------------------------------------------------------- |
| `NoPriorProvider`    | Immediately returns unavailable; normal local background learning proceeds.                    |
| `LocalPriorProvider` | Reads an operator-selected bundle, verifies it, and never needs network access.                |
| `HTTPPriorProvider`  | Disabled by default; fetches bounded manifests and products into the same local bundle format. |

Expose unavailable, incompatible, revoked, corrupt, insufficient coverage, and cancellation
outcomes. They all preserve sensing availability. Fetching happens outside the packet path, with
initial caps of 5 seconds per request, 10 MB compressed runtime payload per selected site bundle,
and 100 MB decoded data. Exceeding a cap causes fallback; final limits depend on the experiment.

A prior supplies candidate landmarks and soft geometry constraints. Validate schema,
hashes, frame, age, coverage, and publisher policy, then estimate local alignment against
current static observations. Only accepted geometry may initialise candidate background
state. It must still earn confidence from local measurements. Never replace the sensor's
range/noise model with remote point occupancy.

Reject ambiguous alignment. Local disagreement, scene change, timeout, or a disabled provider
returns the deployment to its own background learning. Do not change an accepted pose
silently during a run; record the prior digest, alignment result, and any explicit transition
in replay/run provenance. No sensor packets, tracks, precise deployment coordinates, or
alignment results need to leave the site.

## 5. Data model and schema rules

Start with versioned JSON Schema for manifests and runtime products, plus REST/OpenAPI for
the curated service. Define schemas as field contracts before selecting a binary encoding.
Protobuf is post-MVP if size or cross-language tooling measurements justify it; reserve field
numbers and test old/new readers when introduced. Compression and fewer features may solve
the size problem without another encoding.

| Entity           | Required substance                                                                                                                                                                                             |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Observation      | Stable ID; capture/session group; acquisition interval and time basis; footprint; sensor class/model; submitter pseudonym; asset references; source licence and attribution; frame and uncertainty references. |
| Asset            | Logical role; source URL and mirrors; SHA-256; byte length; media type and format/version; bounds and CRS where applicable; metadata inventory; availability and last check.                                   |
| Frame            | ID; axes, handedness, units; horizontal and vertical reference; epoch where needed; local origin; transformations and calibration references.                                                                  |
| Trajectory       | Asset and source frame; clock basis; pose convention; sample fields; sensor/body extrinsic; covariance availability; gaps and time-offset model.                                                               |
| Alignment        | Source/target frames; rigid transform; covariance; method and parameters; control/check points; overlap, residuals, observability, acceptance reason.                                                          |
| Processing run   | Input hashes; software/container digest; parameter hash; random seed; transform/grid files; output hashes; timings; warnings and reviewer decisions.                                                           |
| Evidence element | Spatial support; hit/miss/unknown counts by independent session; view coverage; age; uncertainty; source references; persistence state.                                                                        |
| Feature          | Version-scoped ID; type; geometry and frame; optional orientation; stability; observation count; first/last seen; covariance; supporting observations.                                                         |
| Prior release    | Schema version; product type; cell and coverage; frame; immutable assets; processing/input DAG; source licences; quality profile; observation interval; publication time.                                      |
| Cell summary     | Canonical token and level; current eligible release; last observation; distinct sessions; coverage/density; separate accuracy summaries; contributors and sensor classes.                                      |
| Sponsorship      | Sponsor display identity; region definition; start/end; funding allocation and disclosure; no geometry-editing permission.                                                                                     |

Unknown values are explicit null/unknown states, never zero accuracy, epoch zero, or an
empty string masquerading as a CRS. Schema validation rejects non-finite coordinates,
invalid bounds, unsupported units, and absent mandatory frame information. Preserve
extension namespaces and the original source; an old consumer may ignore optional extensions
but must reject unsupported critical semantics.

IDs for observations and sites are independent of geography. Content hashes identify
exact bytes; observation IDs identify capture records. A corrected manifest is a new
revision that references its predecessor. Catalogue availability and moderation state can
change without rewriting original bytes.

## 6. Ingestion and metadata preservation

Accept a manifest plus assets, either uploaded into quarantine or referenced externally. A bundle
can contain `cloud.e57`, `trajectory.csv`, `metadata.json`, `calibration/`, and `quality.json`;
these names are conventions, while manifest roles are authoritative. The manifest declares which
files form one capture, their checksums, export software, and any omitted vendor fields.

| Format       | MVP experiment handling                                                                                                        | Important check                                                                                                         |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| E57          | PDAL adapter for points; libE57Format for scan structure and richer metadata inspection. Preserve the original and scan poses. | Multi-scan transforms must be applied exactly once; inventory images, invalid points, and extensions before conversion. |
| LAS 1.4      | Read through PDAL; retain point format, scale/offset, CRS records, VLR/EVLR data, and extra dimensions.                        | Quantisation, GPS time encoding, return/classification flags, and coordinate bounds must survive.                       |
| LAZ 1.4      | Same logical contract as LAS, with bounded decompression and decoder validation.                                               | Compressed extension is not a substitute for checking the embedded LAS header.                                          |
| COPC         | Accept conforming input; publish through PDAL's COPC writer after validation.                                                  | Verify hierarchy, bounds, point count, extra dimensions, and range-readable output.                                     |
| PLY/PCD/PCAP | post-MVP adapters; experiment-only export adapters allowed for consumer scans.                                                 | Never guess CRS, scale, sensor timing, or calibration from a filename.                                                  |

COPC is a particular LAZ 1.4 organisation with an octree, not a new lossless archive for arbitrary
E57 content. A valid LAZ file is not automatically COPC. See the
[COPC specification](https://copc.io/) and
[PDAL COPC writer](https://pdal.org/en/stable/stages/writers.copc.html).

Maintain a dimension ledger: source name/type/units, normalised name/type, preservation
destination, and transformations. Include XYZ, intensity, return number, time, scan angle,
ring/channel, classification, RGB, sensor metadata, and calibration. Use suitable extra
dimensions or hashed sidecars where the canonical cloud cannot represent a field.
Point-index sidecars require an explicit mapping because conversion can reorder points. Keep
scan-level metadata separate from point fields.

Compare point counts, invalid-point counts, dimension presence, numeric error bounds, and
source/export metadata on round trips. Do not claim lossless conversion merely because the output
opens. Preserve restricted source material privately until privacy review permits release;
preservation does not mean publishing embedded camera images or device serial numbers.

## 7. Coordinates, georeferencing, and uncertainty

Keep source coordinates immutable. Store the complete source CRS definition, axis order, units,
horizontal datum, vertical datum, and coordinate epoch where applicable. Use WGS84
longitude/latitude for catalogue footprints and S2 indexing. Process geometry in a suitable metric
local frame, with an explicit transform to the geographic reference. Runtime products use a
right-handed east/north/up frame in metres anchored at a declared geodetic origin, with
ellipsoidal height identified explicitly.

Orthometric and ellipsoidal height are different. Record the geoid model
and transformation grids, including versions and hashes. Missing grids,
ambiguous feet/metres, unknown vertical reference, and out-of-area
transformations block survey-quality publication. A source with unresolved
georeferencing may remain a local experiment asset; its guessed position
must not create verified public coverage. Use
[PROJ's transformation model](https://proj.org/en/stable/usage/transformation.html),
preserving the chosen operation and its dependencies. Do not silently
accept a low-accuracy fallback transformation.

Separate CRS conversion from georeferencing and registration: converting coordinates does not
repair an inaccurate GNSS fix, and fitting two scans does not establish global truth. Surveyed
control points anchor the experiment; independent check points evaluate it. A manually placed scan
is labelled as such even when it aligns neatly to another manually placed scan.

| Uncertainty component  | Representation and interpretation                                                                                                                  |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| Local/internal         | Scalar or covariance, measurement definition, spatial scale, confidence convention, and source; manufacturer precision is a claim, not validation. |
| Global horizontal      | East/north covariance where available; otherwise a labelled horizontal statistic with its confidence and method.                                   |
| Global vertical        | Separate sigma/confidence and height datum; never folded into horizontal accuracy.                                                                 |
| Registration           | Six-dimensional translation/rotation covariance and observability diagnostics in an explicit frame and perturbation convention.                    |
| Calibration/time       | Extrinsic covariance, clock offset uncertainty, interpolation uncertainty, and their affected observations.                                        |
| Provenance of estimate | Source-reported or independently estimated; method, sample count, residual distribution, control identifiers, and assessment date.                 |

Use one-sigma values only when the statistical interpretation supports them. Preserve reported
RMSE, 95% bounds, and unknown-confidence claims under their actual names. Local precision of 0.015
m can coexist with several metres of global error; eligibility must inspect both.

Propagate covariance through frame transforms using Jacobians, including calibration and pose
terms. For independent components use the sum of transformed covariance contributions; include
cross terms where correlations are known. Shared control, calibration, and SLAM sessions create
common errors that do not shrink with point count. Where covariance is missing or dependence is
unresolved, retain conservative bounds and flags rather than manufacture precision. Record
quantisation and simplification error separately. Check predicted intervals against held-out
errors before exposing confidence claims.

## 8. SLAM trajectory contract

The minimum trajectory fields are timestamp, x/y/z, and qx/qy/qz/qw. Declare timestamp
unit, UTC/GPS/ monotonic basis, epoch, leap-second handling, source clock, and
cloud-to-trajectory offset/drift. Pose means body-to-declared-frame, with translation in
metres and a unit quaternion in xyzw order. Store sensor-to-body extrinsics separately,
including their calibration version and uncertainty.

Check finite values, monotonic samples, duplicate timestamps, quaternion norms, discontinuities,
speed plausibility, gaps, and time overlap with point acquisition. Normalise quaternion sign
continuity without changing rotation. Interpolate position and rotation only inside bounded gaps;
never silently extrapolate through a missing trajectory segment. A scan without per-point timing
cannot support exact per-point sensor-pose reconstruction just because a trajectory file exists.

Preserve the original trajectory and derived corrections separately. MVP uses trajectories
for overlap, visibility, pose reconstruction where supported, and scan-quality diagnostics.
Loop-closure diagnosis, incidence-angle weighting, and spatially varying deformation
assessment are later improvements. A trajectory is supporting evidence, not independent
ground truth for its own SLAM reconstruction.

## 9. S2 hierarchy and boundary rules

Follow the repository's [S2 decision](../lidar/architecture/geographic-indexing.md): canonical L13
tokens for fine lookup and interoperability, L10 parents for coarse grouping. Derive parents with
`CellID.Parent(10)`, never token truncation. An L10 has 64 L13 descendants. Tokens and explicit
levels are machine values; grouped display labels are presentation only. The
[S2 hierarchy](https://s2geometry.io/devguide/s2cell_hierarchy) defines the geometry.

| Concern                                  | Initial choice                                                                                    |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------- |
| Catalogue, runtime discovery, cache keys | L13 coverage entries plus schema/product/release digest.                                          |
| Filesystem groups, coarse statistics     | L10 parent; aggregate distinct observations rather than summing duplicated references.            |
| Observation coverage                     | Actual footprint plus a bounded S2 covering; expand to L13 associations where affordable.         |
| Dense processing chunks                  | Local metric tiles with overlap; adapt size to point count, not a fixed S2 area.                  |
| COPC internal access                     | COPC octree; do not rebuild it as S2 point storage.                                               |
| City coverage and sponsorship            | Named polygons and sets of cells, potentially mixed-level coverings; cities are not single cells. |
| Coverage detail                          | Observed surface/area masks inside a cell; a single scan does not cover the whole cell.           |
| Rough anchor of an unregistered capture  | The L16 cell of its three-decimal fix, read with its eight neighbours; never verified coverage.   |

Store canonical token text and level with B-tree indexes, plus explicit L13-to-L10 associations.
Postgres signed `bigint` cannot directly represent every unsigned S2 ID; avoid accidental overflow
and lexical range queries by using canonical strings for exact lookup. PostGIS stores footprint
geometry with GiST for geometric intersection, independently of S2. No custom database extension is
required. See [PostGIS spatial indexing](https://postgis.net/documentation/faq/spatial-indexes/).

A capture may intersect many cells. Store its asset once and associate it with each covered
cell. Allocate a feature an owner cell from a stable anchor; neighbouring manifests reference
the same ID and digest. Use overlap halos for extraction and localisation; emit core products
once and deduplicate halo features by identity. Include neighbouring dependencies in a
release snapshot so clients cannot silently mix incompatible generations. Test cell edges,
cube faces, poles, and the antimeridian.

## 10. Processing pipeline and stage contracts

Every stage records input/output hashes, tool versions, parameters, warnings, and acceptance
reasons. Failures quarantine the affected candidate; the last accepted release stays available. The
table names additional provenance and uncertainty specific to each stage.

| Stage / class                      | Input → output                                                        | Quality checks and failure modes                                                                        | Provenance and uncertainty                                                              |
| ---------------------------------- | --------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| Acquire / MVP experiment           | Bundle or external reference → verified quarantine assets             | Hash, length, format, licence, limits; changed URL content, decompression bombs, incomplete uploads     | Retrieval identity, declared rights, original metadata; unknown accuracy stays unknown. |
| Normalise format / MVP experiment  | Source assets → working cloud and metadata ledger                     | Point/dimension reconciliation, scale, invalid points; dropped fields, double-applied scan poses        | Reader/export version, field mapping, quantisation bound.                               |
| Normalise CRS / MVP experiment     | Working cloud + frame → metric working frame and geographic footprint | Axis/unit round trip and bounds; missing datum/grid, implausible location                               | Transform operation, grids, epoch, propagated datum uncertainty.                        |
| Inspect quality / MVP experiment   | Cloud + trajectory → diagnostic report                                | Density distribution, coverage, clock gaps, local consistency; sparse regions, impossible jumps         | Source claims separated from estimated statistics and sampling uncertainty.             |
| Georeference / MVP experiment      | Local cloud + controls → anchored transform                           | Independent check points; wrong controls, global bias, unresolved height                                | Control lineage, residuals, global horizontal/vertical uncertainty.                     |
| Register / MVP experiment          | Overlapping anchored clouds → accepted rigid transforms               | Bidirectional residuals, overlap, degeneracy; local minima, repetitive façades, SLAM distortion         | Correspondence method, transform covariance, correlated-session groups.                 |
| Filter transients / MVP experiment | Registered captures + visibility → candidate static evidence          | Motion, isolated objects, inconsistent surfaces; parked objects mistaken for static, missing visibility | Rejection masks and reason codes; uncertain classifications retained.                   |
| Infer persistence / MVP experiment | Independent evidence sessions → static support model                  | Visible hit/miss accounting; occlusion mistaken for absence, duplicate votes                            | Evidence counts, temporal span, stability confidence and unknown regions.               |
| Partition / MVP                    | Static support + footprint → core/halo products and S2 associations   | Seam continuity and coverage completeness; split/duplicated features                                    | Owner/reference identities, boundary dependencies, unchanged frame uncertainty.         |
| Publish cloud / MVP                | Accepted aligned points → canonical COPC and manifest                 | Decode/range probes, bounds, dimensions; corrupt hierarchy, unsupported fields                          | Source linkage, compression and quantisation settings; no inferred accuracy upgrade.    |
| Derive geometry / MVP experiment   | Static support → surfaces, planes, clusters                           | Held-out surface distance and support; oversmoothing kerbs, invented surfaces                           | Fit covariance, simplification tolerance, support and class confidence.                 |
| Extract features / MVP experiment  | Stable geometry → runtime landmarks                                   | Repeatability, spatial distribution, rank; too few constraints, unstable vegetation                     | Feature support, pose/position covariance, stability and observation interval.          |
| Release / MVP                      | Products + QA + rights review → catalogue and static manifests        | Cross-asset hashes, schema, licence closure, review; partial release, stale pointer                     | Signed release, complete DAG, quality policy version, revocation reference.             |

Use deterministic ordering, fixed seeds, pinned dependencies, and recorded thread settings.
Aim for byte-identical manifests and same-environment outputs. Floating-point processing
across platforms may only reproduce within declared tolerances; record that distinction and
compare geometry as well as hashes. Jobs have idempotency keys from input and recipe hashes,
bounded retries, and explicit failure states. Publish assets first, verify them, then
atomically select the release in the catalogue.

## 11. Registration, persistence, and localisation features

### Registration baseline

Use surveyed control and coarse georeferencing for initial alignment. Voxel-downsample at several
resolutions, estimate normals, propose correspondences from geometric descriptors, and use robust
global registration when initial pose is insufficient. Refine with robust point-to-plane ICP,
excluding likely transients.
[Open3D](https://open3d.org/docs/release/tutorial/pipelines/global_registration.html) provides an
established baseline; evaluate it rather than write another ICP implementation.

Accept using independent control errors, held-out surface residuals, overlap, and
constraint rank. Low ICP residual alone is insufficient: a long wall leaves translation
along it weakly constrained. Estimate competing hypotheses and reject unresolved symmetry.
Prefer a failed alignment over a precise answer in the wrong street. Keep scans with
incompatible alignment in separate hypotheses/products.

Start with rigid six-degree-of-freedom transforms. Consumer scale error must be diagnosed
explicitly; a similarity fit may be an experiment diagnostic but cannot silently redefine
metric scale. Non-rigid SLAM correction is deferred because it can conceal bad
measurements and complicates covariance.

### From a rough fix to a registered capture

A stationary roadside capture is not registered when it is made. It arrives with a position
rounded to three decimal places — good to about 71 m at San Francisco's latitude; the figure
moves with latitude and must be re-measured per deployment, the same caveat the
[occupancy column grid](lidar-occupancy-column-grid-plan.md) plan makes for S2 cell size and skew
— and sometimes an operator's rough north. Everything it produces at that point is in a
**provisional** frame: the sensor's own, in metres. This is the path from there to a
**registered** frame, offline, from public priors. It applies the registration baseline above; it
does not replace it.

The sources keep the roles the [reference data plan](spatial-priors-reference-data-plan.md) gives
them. OSM and footprints are coarse discovery: search area, feature identity, candidate
placement. Public aerial LiDAR from USGS 3DEP is surface matching. Neither is control. Only
measured control and independent check points establish accuracy.

| Stage             | Input                                                        | What it does                                                                                                                                                                                                                  | Output                                                              |
| ----------------- | ------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| 0. Rough anchor   | Three-decimal fix                                            | S2 level 16 cell of the fix, read as that cell and its eight neighbours, which always contain the true position; level 13 and 10 by `Parent`                                                                                  | Where to look; which priors tile to fetch                           |
| 1. Candidates     | OSM ways and nodes in the anchor cells                       | Lists the intersections the capture could be at, usually a handful, each with the bearings of its approaches                                                                                                                  | Placement hypotheses, none preferred                                |
| 2. Coarse pose    | The capture's own persistent geometry and traffic            | Street bearings from the heading distribution of tracks; straight facade and kerb lines from the settled background. Matched to each candidate's way bearings and footprint edges for a heading and a position good to metres | One coarse pose per hypothesis, with its symmetries listed          |
| 3. Surface match  | 3DEP points cropped around each coarse pose, with a halo     | Robust point-to-plane ICP of the settled background against ground and building surfaces, transients excluded, from every surviving hypothesis. Height comes from the ground match, in the prior's stated vertical datum      | A six-degree-of-freedom transform and its covariance per hypothesis |
| 4. Accept or fail | Held-out surfaces, constraint rank, control where any exists | The acceptance rules above. Hypotheses that remain indistinguishable are reported as unresolved                                                                                                                               | `registered`, or `failed` with the reason                           |
| 5. Record         | The accepted transform                                       | Published with its covariance, residuals, method, the priors release digest, and a new calibration identity                                                                                                                   | A registration record consumers can cite                            |

What the capture contributes in stage 2 is the point of using a traffic sensor. Tracks run along
carriageways, so their headings give the street bearings directly, and a four-way grid
intersection gives them at right angles. That fixes heading up to the intersection's own
symmetry: a square grid is ambiguous by quarter turns, and only the buildings, which are not
symmetric, resolve it. A hypothesis whose symmetry cannot be broken fails. It is not guessed.

The background used in stages 2 and 3 is the settled one, with returns the model has marked
transient left out. A background that settled on queued vehicles will mislead the fit in exactly
the cells where it is wrong, so the settle-quality evidence from the
[background region overlay](lidar-background-region-overlay-plan.md) plan is an input to which
cells take part.

**Statuses.** `provisional` until stage 4 passes. `registered` after it, which implies stage 5
has run too: recording is part of what "registered" means, not an optional step after it.
`manually_placed` when
an operator sets the pose by eye or by tie points, which is allowed, labelled as such, and never
counts as verified coverage, even where it agrees with another manually placed capture.
`failed` when no hypothesis is accepted. Nothing is promoted in place: a registered capture gets
a new calibration identity, and products built on the provisional frame are rebuilt, not
relabelled. The [occupancy column grid](lidar-occupancy-column-grid-plan.md) plan is the first
consumer of this.

**Where it is reviewed.** An operator checks and, where needed, sets a placement in the macOS
visualiser, with the prior drawn over the capture in the sensor's frame: candidates, the accepted
pose, residuals, and tie points for the manual path. That is part of working with a capture and
follows the [review workflow](lidar-review-workflow-plan.md), in which the macOS tool is the one
instrument. Curating the reference data itself is a different job, and stays as decided in the
[bootstrap plan](sf-priors-bootstrap-plan.md).

**What is needed, and what is not yet known.** Comparing two visits column by column needs them
placed to within half a 0.5 m column: 0.25 m in position, and about 0.3 degrees in heading for
returns at 50 m. Whether matching to public aerial LiDAR reaches that from a roadside viewpoint
is unmeasured: an aerial survey sees roofs and ground well and walls poorly, and a roadside
sensor sees the reverse. The overlap is mostly ground, kerbs and the lower edges of facades.
Stage 4's held-out residuals on the first site are what settle it. Two captures registered to
the same priors release share its errors, and may agree with each other better than either
agrees with the Earth. That is a hypothesis for the experiment in section 16, not a result.

### Explainable persistent-world evidence

Accumulate sparse voxels or surfels in a metric frame at resolutions tested at 0.05, 0.10, and
0.20 m. For each independent session count a supported surface hit, a visibility-confirmed
absence, or unknown. Use trajectory/ray information when available. Without visibility, a
missing point is unknown. Cap contributions from the same session and keep correlated
processing variants in one evidence group.

A first baseline uses a Beta(1,1) prior updated by independent hits and confirmed
misses, with an interval and raw counts exposed. It is an evidence heuristic until
calibrated. Three hits and no misses give a posterior mean of 0.8, not proof of
permanence. Require temporal separation, sufficient spatial support, and a held-out
repeat observation before marking a feature runtime-eligible.

Reject moving clusters and use geometry/classification hints conservatively. A truck present
on all three visits can still be a truck. Vegetation is potentially persistent but
structurally variable; initially exclude foliage from pose anchors. Retain change candidates
separately, require recent confirmed evidence to remove established geometry, and never
average two conflicting walls together. Version changed products; retain first/last seen and
decay policies as explicit recipe inputs.

MVP outputs are ground patches, building/wall planes, stable vertical clusters, corners where
supported, and a sparse static cloud. Kerbs and pole primitives are experimental. Road versus
pavement semantics, vegetation classification, detailed meshes, and elevation rasters are post-MVP.
Free-space evidence is limited to observed rays and sensor range; unknown space remains unknown.

### First-class feature contract

Store position, orientation where meaningful, geometry extent, stability score, independent
observation count, first/last seen, positional covariance, optional orientation covariance,
and source observations. Planes include normal and offset; edges include
endpoints/direction; poles include axis and supported height; clusters include
representative points and extent. Attach a descriptor version if descriptors are used, and
describe uncertainty in the product's declared frame.

Feature IDs are stable within a release. Across releases use explicit
predecessor/split/merge links rather than promise eternal IDs through changed segmentation.
Plane intersections and corners inherit uncertainty from both surfaces; nearly parallel
intersections are unsuitable pose anchors. Prior eligibility depends on constraint
diversity, visibility, and conditioning, not merely feature count.

## 12. Products, catalogue, and interfaces

These are planning envelopes for a block-scale site with roughly 10–100 million
source points per capture, not measured compression ratios. Publish actual
bytes and generation time for every product.

| Layer              | Rough size                                                    | Access pattern                                                   |
| ------------------ | ------------------------------------------------------------- | ---------------------------------------------------------------- |
| Raw observation    | 0.3–10 GB per capture; multi-capture bundles can exceed 30 GB | Rare full download for reprocessing or audit; often external.    |
| Canonical COPC     | 0.1–3 GB per observation/site product                         | Range requests by area/resolution; occasional full offline copy. |
| Static geometry    | 5–200 MB per site/release                                     | Review, alignment research, optional offline analysis.           |
| Runtime prior      | Target 0.1–5 MB compressed per site; cap initially 10 MB      | One bounded deployment download, pin and reuse locally.          |
| Catalogue manifest | 5–100 KB per release, with paginated provenance               | Discovery and cache validation; no raw cloud in API responses.   |

Public cell summaries expose last observation, distinct session count, current eligible version,
coverage mask/score, density distribution, contributors, sensors, and quality components. Show best
local accuracy and best global accuracy separately, each with the source observation and assessment
basis; those best values may belong to different scans. Also show selected-prior quality and
missing evidence. Do not let a single excellent point imply excellent cell-wide coverage.

| Proposed REST interface                        | Behaviour                                                                                   |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `GET /v1/cells/{token}?level=13`               | Cell coverage and current eligible releases; canonical token/level validation.              |
| `GET /v1/observations?cell={token}&cursor=...` | Paginated metadata, licence, availability, and quality records.                             |
| `GET /v1/observations/{id}`                    | Observation revisions, source assets, transformations, and review state.                    |
| `GET /v1/priors/{release_id}`                  | Immutable manifest with schema/product version, hashes, coverage, and asset links.          |
| `GET /v1/regions?bbox=...&cursor=...`          | Bounded geographic discovery; reject oversized scans and split dateline requests.           |
| `GET /v1/revocations`                          | Signed, cacheable release revocations and replacements.                                     |
| `POST /v1/submissions`                         | MVP maintainer-authenticated manifest submission; returns job ID, not immediate processing. |
| `GET /v1/jobs/{id}`                            | Authenticated state, stage diagnostics, retryable/permanent failure reason.                 |

All responses declare schema version, bounded pagination, and machine-readable error codes. Use 404
for absent records, 410 for a withdrawn product where appropriate, 422 for invalid submission
metadata, 429 with retry guidance for quotas, and 503 for temporary service failure. Do not return
success with a silent empty prior. Immutable manifests and assets have long cache lifetimes;
mutable current pointers and revocations have short lifetimes and ETags. Read access is anonymous.

Static equivalents live beneath `cells/{l10}/{l13}/current.json` and
`releases/{release_id}/manifest.json`; data objects use content hashes. A downloaded bundle
includes runtime features, frame information, provenance, licence notices, publisher
verification material, and the selected neighbour snapshot. JSON is the initial wire
representation; no gRPC service is needed.

## 13. Storage, federation, versioning, and operations

Catalogue municipal, university, contributor-managed, and public-agency assets without requiring
central raw storage. OpenTopography-like, KartaView-like, USGS/NOAA, and other archives are source
categories, not presumed integrations or redistribution permissions. Each adapter must verify
format, rights, availability, and reproducible access for the actual dataset.

Record source URL, checksum, byte length, format, bounds, CRS, metadata, licence, provenance,
and availability. Distinguish available, temporarily unreachable, changed bytes, withdrawn, and
unknown. Recheck external assets on a bounded schedule and on reprocessing. A changed URL
target is a new asset, not a revision of a checksum. Derived products may remain available if
rights permit, with the missing source clearly marked. Guarantee reproducibility only for
inputs retained in an accessible archive.

Use S3-compatible storage for canonical and derived assets; host raw copies only by
explicit retention policy. HTTP range support and correct CORS headers are acceptance tests
for COPC delivery. Static Pages/object hosting suits documentation and catalogue snapshots;
heavy scans belong in object storage. Test actual browser range behaviour before selecting
CDN settings or building a custom viewer.

Keep observations and recipes immutable. Separate schema version, processing recipe
version, product release identity, observation time, publication time, and mutable
eligibility state. Release manifests reference a provenance DAG and an attribution
manifest. Rebuild affected descendants when a source is corrected or revoked. A schema
major change requires explicit consumer support; additive fields require compatibility
fixtures. Runtime clients pin releases for reproducible replay.

Sign release manifests with a project publisher key, retaining rotation/revocation
metadata. Hashes alone do not authenticate a malicious publisher. Contributor signatures
are optional provenance, never accuracy points. Operators can load their own locally
trusted bundles under an explicit local policy. Cached clients cannot learn a new
revocation while offline; record last verification and bound optional remote-cache age,
while preserving local sensing and operator-managed offline bundles.

MVP operations include daily catalogue backups, a tested restore procedure, publish rollback,
orphan cleanup after a grace period, job disk/CPU limits, spend alerts, and metrics for queue
age, failures, publication latency, asset availability, and served bytes. Target recovery
within one working day and at most one day of catalogue loss initially. Exported manifests
permit catalogue reconstruction, but they do not replace source retention. Test restoring a
release before inviting external contributors.

## 14. Validation, quality scoring, and useful contributions

Hard gates precede ranking: integrity, schema, known frames, acceptable rights, privacy review,
plausible location, valid sensor metadata, bounded processing, and no unresolved gross
geometric error. Near-duplicate detection compares both byte hashes and coarse spatial
fingerprints across reformatted and cropped copies. Record overlap and local/global
consistency; duplicated evidence earns no credit.

Use a versioned deterministic score only among eligible submissions. Proposed weights are coverage
25%, independent global checks 25%, local consistency 20%, registration 20%, and transient
cleanliness 10%. Map measured values to [0,1] using published thresholds calibrated on the
experiment. Missing components receive no earned points and remain visibly unknown; a total never
overrides a failed gate. Density above a useful threshold adds no points. Publish component values,
denominators, thresholds, and reasons, and report uncertainty separately from the ranking score.

Coverage needs an explicit denominator: the approved survey polygon and relevant visible surfaces.
Use residual quantiles and spatial distributions rather than a single RMSE; check duplicate walls,
floating surfaces, impossible scale, implausible ground slopes, and contradictory overlapping
geometry. Hold suspicious submissions for independent review and provide an appeal path. A low-cost
scan may be useful where nothing exists while remaining ineligible for precise localisation.

Pioneer/First Mapper recognises the earliest verified useful contribution, not the earliest
upload. Best Scan is scoped to a published quality profile, with local and global awards
kept distinct. Current Steward recognises recent verified maintenance and expires without
giving exclusive rights. Most Recent Verified Observation records acquisition time and
review time separately. Award credit for new coverage, independent confirmation, or verified
changes; cap repeated same-session contributions.

Do not reward point count, signatures, account age, or sponsor status as quality. Manual review and
authenticated contributors suffice in the MVP. Public leaderboards and elaborate anti-Sybil systems
are post-MVP; immutable evidence and a transparent appeal policy are needed earlier.

## 15. Security, privacy, and licensing

Treat parsers and external URLs as hostile inputs. Fetch through an isolated worker with no service
credentials, deny private/link-local/cloud-metadata destinations, revalidate DNS and redirects, and
allow only approved protocols. Bound archives, expansion ratios, file counts, RAM, disk, CPU, and
job duration. Reject path traversal and executable hooks. Use scoped upload credentials and
maintain separate quarantine, approved, and public storage permissions.

Escape contributor metadata in the website, validate asset paths, rate-limit reads and
submissions, and prevent arbitrary remote URL proxying. Publishing needs separate authority
from ingestion. Test poisoned geometry, forged provenance, signature/key changes,
inconsistent manifests, partial downloads, and unbounded range-request abuse. Keep an
auditable moderation record with reasons and replacements.

Public geometry is not automatically harmless. Scans may contain people, vehicles, private
interiors, window views, embedded camera images, device identities, and operator trajectories.
MVP publication is limited to reviewed outdoor geometry. Retain richer inputs in restricted
quarantine only as necessary; publish sanitised assets with a clear relationship to retained
originals. Restrict precise trajectory publication and remove contributor contact details from
public metadata unless explicitly chosen.

Do not add cameras or camera-dependent capture to velocity.report. Third-party RGB or E57 images
can be preserved as restricted source metadata, but public release is a separate decision. Provide
a report/takedown route, derivative withdrawal, CDN invalidation, and a minimal non-sensitive
tombstone. Immutability means accepted bytes are not silently edited; it does not prohibit
withdrawal. Copies already downloaded cannot be recalled.

Cell requests reveal approximate deployment location and IP address. S2 is not
anonymisation. Default to offline bundles, disable remote fetching until opt-in, minimise
access-log retention, and avoid tracking analytics. A regional bundle can reduce the
precision of location disclosed by later use.

Propose a permissive code licence, reviewed against dependencies, and explicit
per-asset data licences. Initially allow reviewed public-domain/CC0 and compatible CC
BY 4.0 data; retain attribution, source, licence link, and modification notices as
required by the [CC BY 4.0 terms](https://creativecommons.org/licenses/by/4.0/). Public
accessibility alone grants no assumed right to redistribute or derive products. Do not
relicense federated inputs by uploading them.

Keep licence compatibility as a publication gate, including database rights and compound
products. Unknown rights remain quarantined or metadata-only where permitted. Share-alike sources
need a separate compatibility decision and possibly separate products;
non-commercial/no-derivatives sources do not enter the initial general-purpose open prior pool.
Review actual scanner export terms and data ownership. Attribution must travel in offline
bundles, not depend on a website remaining reachable. These are proposed project policies, not a
determination of rights for any particular contribution.

## 16. Controlled 2–3 block experiment

Use the same outdoor 2–3 blocks, containing intersections, long façades, vegetation,
parked vehicles, and at least one poorly constrained corridor. Capture with a professional
mobile SLAM scanner, a custom conventional LiDAR plus IMU rig, and consumer iPhone/iPad
LiDAR. The consumer class may use a documented export-to-supported-format adapter;
preserve original scale and metadata limitations.

Collect three temporally separated sessions per class, ideally on different days with changed
parked vehicles and repeat viewpoints. Keep the first two sessions for building candidate priors
and the third for validation. Hold one block out of parameter tuning, then evaluate it with the
frozen recipe. Report results per scanner class and fused input; do not require a consumer scan to
match professional coverage to conclude that higher-grade inputs are useful.

Establish distributed survey controls and separate check points with recorded uncertainty,
targeting reference error below 0.03 m where feasible. The professional scanner is not its
own ground truth. Measure fixed sensor pose independently from scene-registration output,
including sensor-to-survey extrinsics. If reference uncertainty is too large, widen
reported bounds and withhold accuracy claims.

At six fixed P40/comparable poses, two per block, record three held-out sessions each. None of
these captures contributes to the map used to localise it. Test 20 seeded initialisations per
session: translation offsets up to 5 m, yaw up to 30 degrees, and smaller documented roll/pitch
perturbations. This yields 360 trials per prior variant. They share 18 recordings and are not 360
independent scenes; report clustered intervals by site/session and all failure counts.

The harness consumes a frozen capture manifest, recipe, controls, prior bundle, fixed-sensor
replay, initial-pose seed list, and machine description. It emits estimated/true poses, covariance,
acceptance reason, timing, memory, geometry errors, background masks, traffic-metric comparisons,
and a standalone report. Use deterministic replay/capture time, not wall-clock processing speed, to
compare background settling. Measure processing time separately. Keep reference labels and
evaluation captures out of feature selection and threshold tuning.

Compare no prior, single-capture prior, repeated-capture prior, and fused-class prior. Add
ablations for trajectory visibility, global control, and sparse features versus larger
static clouds. Deliberately test wrong-cell priors, stale geometry, missing neighbours,
corrupted files, offline operation, unsupported schemas, shifted global frames, repetitive
structures, and consumer scale distortion.

| Metric                  | Proposed gate, to freeze before held-out evaluation                                                                                                                 |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Pose recovery           | At least 95% accepted and correct trials in the stated initialisation envelope; report every class/block separately.                                                |
| Accepted-pose error     | 95th percentile translation ≤0.20 m and orientation geodesic error ≤1 degree against independent reference.                                                         |
| False confidence        | Zero accepted poses with translation error >1 m or orientation error >5 degrees in the trial and adversarial set; this is a pilot gate, not a population guarantee. |
| Static geometry         | 95th percentile supported-surface distance ≤0.15 m; report unsupported area separately rather than hiding it in distance statistics.                                |
| Coverage and density    | Measured completeness of relevant visible surfaces and density quantiles by class; identify insufficient-support regions explicitly.                                |
| Transient contamination | Report labelled transient fraction and confidence intervals; runtime anchors must contain no known person/vehicle clusters in reviewed pilot data.                  |
| Runtime cost            | ≤5 MB target prior and ≤10 seconds pose fitting with ≤256 MB extra peak memory on the target Pi-class device; workstation results alone do not pass.                |
| Background benefit      | At least 50% reduction in capture time to a predeclared background-quality threshold versus no prior; no more than 1 percentage point loss of foreground recall.    |
| Traffic behaviour       | No more than 1 percentage point degradation in labelled detection precision/recall; report count and speed deltas with label uncertainty.                           |
| Offline/failure path    | All missing/corrupt/rejected-prior cases continue local sensing without packet-path blocking.                                                                       |
| Reproducibility         | Identical selected inputs and manifests; geometric outputs within declared numeric tolerance on a second clean environment.                                         |

Define the background-quality threshold on development data: static false-foreground rate
below 2% and labelled foreground recall at least 95% for three consecutive 10-second
capture-time windows. Publish masks, denominators, and label sampling policy. If the
no-prior baseline cannot reach it, report that failure rather than claim an infinite
speed-up. Freeze thresholds before opening holdouts.

Also measure capture/person-hours, scanner rental or amortisation, survey cost, processing
CPU-hours, review time, storage bytes, and cost per mapped block. Quote these from the actual
experiment, not a vendor accuracy sheet. Missing trajectory, incomplete consumer coverage, and
licence-restricted exports are findings, not reasons to silently replace the test inputs.

## 17. Infrastructure cost model and geographic sponsorship

All figures below are planning estimates in USD, not provider quotes for a deployed system.
Define one site as a block-scale dataset: three retained 2 GB raw captures, one 1 GB canonical
cloud, 50 MB static geometry, 5 MB runtime prior, and 1 MB catalogue export. Budget three
retained derived generations: 1.166 GB hosted per site excluding raw, or 7.166 GB including
those three raw captures. Retaining three canonical generations instead adds 2 GB/site. A city
here means 1,000 such sites; several cities means 10,000. Actual city coverage is an
experiment result, not an S2-cell count.

For a transparent storage baseline use R2 Standard at
$0.015/GB-month, writes at $4.50/million and reads at $0.36/million, with no R2
Internet egress charge. Ignore free tiers and billing rounding in this model. These
rates were checked on September 9, 2026 against
[Cloudflare's pricing](https://developers.cloudflare.com/r2/pricing/); other
services and CDN features can add costs.

| Monthly item                                        | 10 sites | 100 sites | City: 1,000 sites | Several cities: 10,000 sites |
| --------------------------------------------------- | -------: | --------: | ----------------: | ---------------------------: |
| Metadata export storage                             | $0.00015 |   $0.0015 |            $0.015 |                        $0.15 |
| Static geometry + runtime priors, three generations |   $0.025 |     $0.25 |             $2.48 |                       $24.75 |
| Canonical COPC, one generation                      |    $0.15 |     $1.50 |               $15 |                         $150 |
| Optional hosted raw, 6 GB/site                      |    $0.90 |        $9 |               $90 |                         $900 |
| Range/object operations allowance                   |    $0.08 |     $0.81 |             $8.10 |                          $81 |
| Metadata database/API/backup budget                 |   $20–60 |   $30–100 |          $100–300 |                   $300–1,000 |
| Worker compute budget                               |   $10–50 |   $30–150 |          $100–500 |                   $500–2,000 |
| CDN/service add-on allowance, R2 egress $0          |    $0–10 |     $0–20 |            $0–100 |                       $0–300 |
| Approximate total, including hosted raw             |  $31–121 |   $72–282 |        $316–1,016 |                 $1,956–4,456 |

The operation allowance assumes 10,000 reads and 1,000 writes per site per month. Assume 2 GB
delivered per site/month to expose egress sensitivity: at a hypothetical
$0.09/GB charged-egress provider, add $1.80, $18, $180, or $1,800 respectively. That is a scenario
rate, not a verified competing quote. COPC browsing can exceed both assumptions; measure actual
range request counts and cache hit rates.

Compute budgets assume processing 10% of sites per month at 2–8 CPU-hours per site and a planning
rate of $0.10–0.25/CPU-hour, plus idle capacity and scratch headroom.
Initial full ingestion is additional: roughly $2–20,
$20–200, $200–2,000, or $2,000–20,000 across these scales under those assumptions. Neither runtime
nor provider compute rates have been benchmarked. Full extra raw/canonical mirrors, taxes, scanner
hire, surveying, engineering, moderation, and legal review are excluded. At small scale, people and
compute will dominate the few dollars of object storage.

Sponsorship is post-MVP, with a manually administered pilot if funding is needed earlier.
Offer fine L13 coverage, L10 neighbourhood groups, named city polygons, and general
infrastructure support. Meter actual hosted bytes, operations, processing, and allocated
shared costs; avoid pricing an empty cell as if it contained a city's point clouds. Attribute
shared assets once across overlapping sponsor regions.

At the stated assumptions, storage alone including raw is about
$1.29/site/year. Allocate the full service budget instead: 100 sites cost roughly $864–3,384/year,
and 1,000 sites $3,792–12,192/year. A trial asking price might be $100–250/year for a lightly
occupied fine cell,
$1,000–3,000 for a neighbourhood supporting around 100 sites, and $10,000–30,000 for the model
city. These are fundraising hypotheses with disclosed reserve/stewardship margin, not guaranteed
cost recovery or evidence of willingness to pay. Dense cells need a measured allocation;
infrastructure sponsors can cover review.

Show one restrained acknowledgement in the region details panel and a sponsor page, with an expiry
date. Avoid logos stamped on geometry, pop-ups, or map-wide badge clutter. Sponsorship never
changes quality rankings, licence, contributor rights, or publication decisions. Offer renewal
notice 60 days before expiry. On lapse, remove the current acknowledgement, retain historical
disclosure, and draw on a reserve or alternate sponsorship. Apply any storage tiering by a public
retention policy, never revoke openness or paywall a prior because a sponsor left. Do not promise
indefinite hosting without funding; publish export and mirror paths.

## 18. Tools to reuse and choices to defer

These are candidate components, not assertions that every required plugin is
installed. Pin versions, check licences and redistribution obligations, and run
fixture compatibility tests before adoption.

| Need                     | Recommendation and official reference                                                                                                                                                                                                    |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| E57 reading/writing      | [libE57Format](https://github.com/asmaloney/libE57Format) for full-format inspection; [PDAL E57 reader](https://pdal.org/en/stable/stages/readers.e57.html) for pipeline ingestion. Test actual plugin builds and metadata preservation. |
| LAS/LAZ/COPC             | [PDAL](https://pdal.org/) and its [COPC writer](https://pdal.org/en/stable/stages/writers.copc.html); use the writer's supported compression stack rather than implement a codec.                                                        |
| Point processing and ICP | [Open3D](https://www.open3d.org/docs/release/) for registration and geometry; [CloudCompare](https://github.com/CloudCompare/CloudCompare) as an independent inspection tool.                                                            |
| CRS and datum transforms | [PROJ](https://proj.org/); package required grids and preserve operation metadata.                                                                                                                                                       |
| SLAM interchange         | Explicit trajectory CSV plus calibration/frame manifest first; [MCAP](https://mcap.dev/) later for rich timestamped messages. MCAP alone does not define pose conventions.                                                               |
| S2                       | [golang/geo/s2](https://github.com/golang/geo) for catalogue/consumer helpers; official [S2](https://s2geometry.io/) implementation for processing where needed, sharing known-vector fixtures.                                          |
| Metadata geometry        | [PostGIS](https://postgis.net/documentation/) for footprints and spatial queries, ordinary columns for S2 associations.                                                                                                                  |
| COPC inspection          | [COPC viewer](https://viewer.copc.io/) linked by PDAL; use only approved public assets in external viewers.                                                                                                                              |
| Browser rendering        | [Potree](https://github.com/potree/potree) for large point-cloud review if needed; test its selected format path before assuming direct COPC support. Avoid a second converted publication tree in the MVP.                              |
| Bulk/offline metadata    | [GeoParquet](https://geoparquet.org/) and [DuckDB Parquet support](https://duckdb.org/docs/stable/data/parquet/overview) when bulk export demand appears.                                                                                |

Do not build a global map editor, semantic ML training system, bespoke binary geometry
format, account reputation economy, billing engine, or distributed scheduler to run a
nine-session comparison. Standard formats reduce implementation work; they do not remove
the need to understand the measurements.

## 19. Major risks and questions that need evidence

| Risk                                                  | Consequence                                        | Experiment or response                                                             |
| ----------------------------------------------------- | -------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Local SLAM precision hides global bias                | Confident prior in the wrong location              | Separate check points, datum audit, and deliberately shifted input tests.          |
| Sparse fixed LiDAR cannot match dense mobile features | Primary consumer gains little                      | Cross-sensor pose experiment with sparse feature and cloud ablations.              |
| Repeated façades and flat ground underconstrain pose  | Low residual, wrong transform                      | Rank diagnostics, competing hypotheses, and false-acceptance gate.                 |
| Parked objects become persistent                      | Prior suppresses valid foreground                  | Temporal separation, visibility masks, held-out labels, and soft-only integration. |
| Trajectory time/pose conventions differ               | Bad ray evidence and registration                  | Time-offset fixtures and trajectory-present/absent ablations.                      |
| Consumer scale/drift varies by export                 | Heterogeneous ingestion produces misleading fusion | Per-class results and scale diagnostics; permit lower eligibility.                 |
| Source rights or privacy prevent public release       | Attractive corpus cannot be redistributed          | Rights review before capture/import and restricted-source processing paths.        |
| External assets disappear                             | Pipeline cannot reproduce old products             | Archive critical experiment inputs and label unretained dependencies.              |
| Review and rescanning costs exceed hosting            | Unsustainable contribution service                 | Track reviewer minutes and cost per useful update, not just storage bills.         |
| Scene changes invalidate old priors                   | Wrong background or alignment                      | Current-observation checks, bounded cache policy, and release replacement.         |

Open experiments: what initial-pose envelope is realistic without accurate heading; which feature
types survive P40 sampling; which voxel size preserves useful kerbs; how much trajectory improves
visibility; how many independent visits justify persistence; whether uncertainty intervals
calibrate; whether a 5 MB bundle contains enough geometry; how seasonal vegetation affects
matching; and whether prior-assisted background settling improves traffic metrics on the actual
device. Architecture cannot decide these by adding another field to the manifest.

## 20. Recommended MVP architecture

Proceed with a workstation experiment, then a curated single-application service: Go catalogue
and publisher, Python/PDAL/PROJ/Open3D worker, Postgres/PostGIS metadata, S3-compatible
immutable products, static catalogue pages, and JSON runtime bundles. Host canonical and
derived products, catalogue raw assets wherever rights and availability permit, and mirror only
critical inputs. Begin integration with local files; enable bounded HTTP retrieval only after
offline fallback and pose rejection pass.

## 21. Proposed repository/file tree

This is a proposed layout for the separate repository, not files created by this planning change.

```text
spatial-priors/
├── README.md
├── LICENSE
├── CONTRIBUTING.md
├── schema/
│   ├── observation.schema.json
│   ├── frame.schema.json
│   ├── trajectory.schema.json
│   ├── prior.schema.json
│   ├── provenance.schema.json
│   └── fixtures/
├── cmd/priors/                 # catalogue, publisher, and administrative CLI
├── internal/
│   ├── catalogue/
│   ├── api/openapi.yaml
│   ├── s2index/
│   ├── storage/
│   └── jobs/
├── pipeline/
│   ├── ingest/                 # e57 and las/laz/copc adapters
│   ├── frames/
│   ├── alignment/
│   ├── static_map/
│   ├── localisation/
│   └── quality/
├── experiments/fixed-lidar/    # manifests, frozen protocol, evaluation reports
├── migrations/
├── web/                       # generated coverage pages
├── deploy/                    # one host, worker, database, object-store configuration
└── docs/                      # data rights, privacy, recipes, operations, decisions

velocity.report/
└── internal/prior/            # optional consumer model, providers, validation, cache
```

Keep large experiment assets outside Git with checksummed manifests. Do not create empty future
adapters or a separate schema module in anticipation of users who have not appeared.

## 22. Staged implementation roadmap

| Stage                              | Deliverable                                                                                                                         | Exit decision                                                                                                               |
| ---------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| A: contracts and capture readiness | Schema drafts, frame conventions, source rights, survey design, fixture corpus, frozen evaluation protocol                          | Review confirms inputs and independent ground truth are obtainable.                                                         |
| B: MVP experiment                  | Three capture classes over repeated visits; reproducible registration, static evidence, feature bundles, blinded fixed-LiDAR trials | Pose recovery, false-acceptance, and background gates pass for at least one viable capture route; otherwise narrow or stop. |
| C: curated publication             | 10 sites, S2 coverage, signed immutable releases, catalogue, source federation, privacy/licence review, restore test                | Every release is traceable and reproducible within its declared retention policy; measured costs fit budget.                |
| D: optional consumer integration   | Local/no-prior providers, replay provenance, then opt-in HTTP and offline cache                                                     | Target-device performance and all corruption/outage/rejection cases pass without sensing regression.                        |
| E: public pilot                    | 100 curated sites, documented contribution review, quality components, coverage pages, manual sponsorship trial                     | Useful contributions exceed review burden; independent users reproduce localisation results.                                |
| F: post-MVP growth                 | Self-service submissions, bulk exports, badges, sponsor billing, richer features                                                    | Implement only the bottlenecks demonstrated by the pilot.                                                                   |

Do not promise dates before scanner access, survey control, and export rights are
confirmed. Estimate engineering effort after Stage A and re-estimate using actual
processing and review time after Stage B.

## 23. First engineering issues to open

These are issue drafts for review, not remotely created issues.

1. **Freeze capture/frame/uncertainty contracts.** Accept representative manifests from all three
   classes, explicit unknowns, trajectory conventions, and fixtures that reject ambiguous frames.
2. **Secure the repeated-block corpus and independent reference.** Record rights,
   survey/check-point separation, three sessions per class, six fixed poses, and a cost ledger.
3. **Build metadata-preserving ingestion fixtures.** Verify E57 scan transforms, LAS/LAZ
   dimensions, COPC output, retained sidecars, and bounded malformed-file failures.
4. **Implement reproducible georeferencing and registration evaluation.** Publish held-out
   residuals, pose covariance/degeneracy diagnostics, and wrong-alignment rejection cases.
5. **Build visibility-aware static evidence and feature baseline.** Demonstrate hit/miss/unknown
   accounting, session deduplication, explainable persistence, and traceable feature support.
6. **Run the blinded fixed-LiDAR harness.** Compare baseline/prior variants on the target device,
   publish pose/background/traffic metrics and clustered uncertainty, and record go/no-go.
7. **Define runtime bundle and local consumer seam.** Exercise
   compatible/incompatible schemas, frames, byte caps, rejection, deterministic
   replay, and no-prior fallback without network access.
8. **Publish immutable S2 products and minimal catalogue.** Verify boundary/halo identity,
   canonical parents, release atomicity, signatures, external-asset states, and restore/rollback.
9. **Establish contribution publication gates.** Document rights/privacy review, duplicate and
   quality checks, parser isolation, withdrawal propagation, and reviewer audit records.
10. **Run a metered 10-site hosting pilot.** Measure storage by layer, processing/review cost,
    range requests, and offline downloads; revise sponsorship and 100-site budgets from evidence.

## 24. Highest-risk experiment before the public platform

Ask a held-out stationary P40 to recover its independently measured pose from a small prior built
by different sensors on earlier visits, then demonstrate faster background readiness without
suppressing real foreground. Include repetitive geometry, parked vehicles, imperfect
georeferencing, and a fully offline run. If the system produces confidently wrong poses, or needs
the full raw cloud to be useful, stop public-platform work and repair or narrow the product. A
successful catalogue is not evidence that the prior works.
