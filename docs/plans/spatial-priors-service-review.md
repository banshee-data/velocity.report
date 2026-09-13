# Spatial priors: engineering review and decision record

Develop the public catalogue and useful runtime priors together, using the same small corpus and
separate release criteria. This review identifies changes needed in the draft and compares the
decisions that determine the project's cost, usefulness, and technical risk.

- **Status:** Review in progress; recommendations awaiting discussion
- **Canonical:** [Geometry-prior service](../lidar/architecture/geometry-prior-service.md)
- **Scope:** Design review, source checks, alternatives, and
  interactive decisions; no implementation
- **Related:** [Service plan](spatial-priors-service-plan.md),
  [vector scene map](../lidar/architecture/vector-scene-map.md),
  [S2 conventions](../lidar/architecture/geographic-indexing.md)

## 1. Decisions and recommendation

**Reference-data follow-up:** The user requests protection from inaccurate GPS and L10 reference
packages maintained by community stewards/sponsors, including targeted records
acquisition. The [reference-data research](spatial-priors-reference-data-plan.md)
identifies free sources and proposes their roles. SF parcel GIS is explicitly unsuitable
for precision use; verified control and measured surface references should establish
placement, with parcels retained as supporting context.

**Confirmed by the user:** develop the public catalogue and useful priors together, accepting a larger
scope. This replaces the draft's recommendation to defer the catalogue until localisation succeeds.
It does not imply that every catalogued scan is safe to use as a runtime prior.

**Confirmed by the user:** existing trip data and static 20-minute windows are available, with PCAP
timestamps only, at `/Volumes/lidar/lidar/s2`. The user proposes SLAM along paths and
using small movements during static windows to improve scene sampling. Start with
this corpus; new scanner acquisition is not a dependency. The user also requests
hardware additions for better future captures.

**Confirmed direction:** evaluate how much operator assistance is practical rather than choosing
an automation level in advance. Test a roughly ten-second OSM corner-snap workflow
where suitable geometry exists, escalating to alignment against known control points
when needed. Intersection corners are candidate anchors; their physical definition
and coordinate accuracy require validation.

**Confirmed by the user:** measure reference points for the first test. Reserve separate alignment
points and independent check points before fitting or tuning either the OSM or measured-control
workflow. The measurement method, equipment, and achieved uncertainty remain to be established.

**Pending:** reference measurement method and the practical resource envelope. Actual
packet time quality, source sensor identity, and navigation fields have not yet been decoded. Do
not assume survey access, IMU data, or accurate global placement.

Build a common capture/asset/frame/provenance foundation, then two delivery tracks:

| Track | First deliverable | Independent success criterion |
| --- | --- | --- |
| Public catalogue | Curated, openly licensed site records, source links, COPC previews/downloads, S2 coverage, and history | Another user can discover, understand, retrieve, and reproduce an approved dataset. |
| Runtime priors | Local bundle, held-out pose recovery, and scene-context evaluation | A fixed LiDAR can use it with measured error and reject misleading matches. |

Join the tracks through release manifests. Treat experimental, locally alignable, geographically
validated, and runtime-approved as separate capabilities/assessments, not a single quality ladder.
An excellent local map can have uncertain global placement. A globally accurate aerial cloud can be
poor for matching a street-level sensor.

## 2. Evidence boundary

Reviewed the service draft, earlier geometry-prior specification, vector-scene specification, the
linked static-pose document, S2 conventions, and relevant production background/transform code.
Ground-plane documentation was checked for integration context, not audited line by line. No
scanner, registration, runtime-performance, or traffic experiment was run.

Production evidence matters because the linked specifications describe substantial future work:

- [Tracking pipeline](../../internal/lidar/pipeline/tracking_pipeline.go) currently passes a nil
  pose to `TransformToWorld` in its foreground path.
- [Transform helper](../../internal/lidar/l4perception/cluster.go) supports a supplied pose, but
  helper support is not deployed geographic alignment.
- [Background cells](../../internal/lidar/l3grid/background.go) contain sensor-range means,
  spreads, observation counts, and locked baselines.
- [Foreground classification](../../internal/lidar/l3grid/foreground.go) compares returns against
  those learned ranges and confidence-dependent thresholds.
- [Ground filtering](../../internal/lidar/l4perception/ground.go) explicitly warns that input
  heights are sensor-relative unless a real transform has been applied.

The [static-pose document](../lidar/operations/static-pose-alignment.md) largely concerns deferred
7DOF object tracking, not the fixed-sensor registration problem. Its assertions about current
tracking should not be imported into this project without a separate code audit.

## 3. Findings that change implementation order

### A. Separate pose recovery, scene context, and background seeding

The draft's integration discussion and experiment combine three benefits. They require different
mechanisms and can succeed independently. Recovering pose can improve deployment/calibration even
without faster background settling. Scene planes can improve height/context queries without
replacing beam statistics. Seeding background requires view-dependent projection, occlusion
handling, expected returns, and conservative confidence initialisation.

**Change:** first accept alignment and context bundles. Evaluate background seeding as an independent
experiment with its own feature flag and rejection criteria. Do not make the entire
catalogue fail because the ambitious 50% settling improvement is absent. Do not require an
L3 redesign to publish a useful pose result.

### B. Keep sensor-local processing stable initially

Wiring a geographic pose into the existing foreground path changes the
meaning of height thresholds, regions, track coordinates, and recorded
output. It is broader than adding a provider package.

**Change:** initially transform the selected prior into the established sensor-local frame. Record a
separate sensor-to-map alignment with direction and uncertainty. Apply world coordinates to exports
only where consumers explicitly support them. A later perception-frame migration needs a dedicated
audit of filters, regions, stored tracks, replay, and UI assumptions.

### C. Tighten error criteria around the sensing area

The draft proposes 0.20 m translation and 1 degree orientation error. A 1 degree rotation produces
about 0.87 m transverse displacement at 50 m: 50 × sin(1 degree). This is a geometric scale
example, not a claim that all surfaces suffer identical range error. Translation and rotation
limits alone therefore cannot establish safe background correspondence.

**Change:** report transformed-point/surface error over the declared operating range, per direction,
along with pose error and uncertainty. Derive the allowed angular error from the required spatial
error budget. At 50 m, an angular-only 0.10 m displacement budget is about 0.115 degrees;
translation, map error, calibration, and sensor noise also consume the budget. Avoid declaring that
number a universal requirement before measuring the actual foreground tolerances.

### D. The capture protocol mixes training and validation roles

The draft reserves the third mobile session for validation but also uses a held-out repeat
observation to establish runtime eligibility. Using that same session to select features would
contaminate the final evaluation. Six poses and hundreds of seeded trials also provide fewer
independent scenes than their headline trial count suggests, as the draft correctly notes.

**Change:** name fitting, selection/calibration, and final evaluation partitions explicitly. Either
freeze eligibility using development visits and preserve the third visit strictly for evaluation,
or collect a fourth visit if the third is needed for selection. Keep the final fixed-P40 captures
and reference poses blind to parameter selection. Begin with one site to debug the harness, then
execute the complete three-class comparison with frozen rules.

### E. Arbitrary quality weights should not govern early publication

The proposed 25/25/20/20/10 score has no measured calibration. It can penalise a useful locally
alignable dataset because global controls are absent, while rewarding geography that offers few
observable street-level features. The Beta persistence model is a useful baseline but not a
physical probability of permanence, especially with dependent visits and parked objects.

**Change:** publish the component measurements and explicit eligibility profiles first. Keep any
combined ranking experimental until comparisons show that it predicts useful outcomes. Preserve
hit/miss/unknown counts, session independence, visibility, temporal span, and class-specific
cautions. Do not use a score to hide unknown global uncertainty.

### F. The catalogue schema should evaluate STAC before becoming bespoke

The draft recommends a new observation REST API without evaluating an
established catalogue standard.
[STAC](https://stacspec.org/en/about/stac-spec/) supports static and dynamic
catalogues, allowing an initial object-hosted catalogue to precede a searchable
server. Its
[point-cloud extension](https://github.com/stac-extensions/pointcloud)
describes counts, dimensions, density, and statistics; the inspected extension
is marked Pilot, so pin and validate its version.

**Change:** use STAC Items/Collections/assets for discovery where they fit, with namespaced S2,
uncertainty, and prior-release metadata. Keep the runtime 3D bundle and full processing evidence as
linked project schemas. STAC does not replace the localisation model or define sensor trajectories.
Avoid claiming complete STAC API compliance from a few similarly named endpoints.

### G. The 5 MB prior target should shape optimisation, not discovery

An early 10 MB hard rejection can rule out a useful product before measuring its tradeoff. The
draft also names a 100 MB decoded-data cap but a 256 MB extra peak-memory target: these need
separate definitions because parsing and registration indexes allocate additional memory.

**Change:** compare a compact sparse cloud, primitives, and a hybrid at explicit byte budgets. Measure
download size, decoded representation, peak registration memory, and runtime
separately. Start with a bounded generous research budget, then select a Pi deployment
profile. Keep production caps even when research tools accept larger files; do not
silently download an entire cell to satisfy a small site.

### H. The service needs an explicit area-selection contract

A cell identifies a partition, not a deployment footprint. One L13 cell can contain several sites;
one sensor may require neighbouring cells. The plan acknowledges halos but its simple provider call
does not fully express product selection and byte budgets.

**Change:** distinguish discovery by region/cell from retrieval by immutable release ID. A deployment
selects an area of interest, range/coverage requirements, supported product profile, and byte
budget. Offline tools prepare a site bundle; the live provider consumes it. Retain canonical
L13/L10 indexing without using cell boundaries as processing boundaries or site identities.

## 4. Older specifications need reconciliation

These are design-document findings, not claims of production defects.

| Location | Problem | Required disposition |
| --- | --- | --- |
| Geometry-prior configuration and privacy sections | Remote access defaults to true despite explicit opt-in prose. | Resolve to disabled by default before implementing configuration. |
| Geometry-prior trust and union sections | A valid signature is used as a quality signal and signed files are called verified. | Separate authenticity, review state, and measured geometric quality. |
| Geometry-prior open questions | Merging remains an open question after the document already specifies daily weighted union. | Replace contradictory directions with explicit source, candidate, and release identities. |
| Vector scene §4.1 | Local adjacent-tile merge thresholds are claimed to guarantee a globally merged plane within 3 cm. | Require residual checks against the final fit; pairwise agreement does not bound accumulated curvature. |
| Vector scene §4.1 and §8 | Convex hulls and gap-free coverage can fill unobserved holes and concavities. | Preserve observed-support masks and distinguish display coverage from measurement support. |
| Vector scene §4.3 | A 120-second detail expiry is unsuitable as a persistent-world lifetime. | Separate live working-cache expiry from observation age, review age, and product change policy. |
| Vector scene §6.4 | Near-zero object velocity for 30 seconds can promote a parked vehicle into persistent scene geometry. | Use repeated-session evidence and explicit transient/unknown handling. |
| Vector scene §6.3 | Background cells are assumed to expose surface orientation for wall grouping. | Define extraction from retained/projected points; current cell statistics do not themselves supply wall normals. |
| Vector scene §12 | OSM structures are the preferred prior; the new service also derives measured building geometry. | Keep coarse map context and measured localisation anchors as distinct source roles. |
| Static-pose reference | Title suggests sensor alignment while the content mainly concerns object boxes/tracking. | Remove it as an implied implementation dependency; link a dedicated fixed-sensor alignment design. |

GeoJSON can carry 3D geographic positions; it is not inherently limited to 2D. However,
[RFC 7946](https://www.rfc-editor.org/rfc/rfc7946) defines geographic coordinates, so sensor-local
metres cannot become standard GeoJSON simply by adding a `coordinate_system` property. Use GeoJSON
for geographic footprints/exports and a frame-explicit runtime schema for metric 3D geometry.

The draft correctly preserves source metadata, uncertainty distinctions, optional
connectivity, immutable releases, and source rights. Keep these. Its strongest improvement
over the earlier union proposal is that conflicting observations remain evidence to
resolve rather than polygons to average.

## 5. Approach tradeoffs

### Project shape

| Approach | Benefit | Cost or limitation | Position |
| --- | --- | --- | --- |
| Priors research first | Lowest platform distraction; fastest test of fixed-LiDAR benefit | No early public catalogue; reusable data work remains less visible | Not the user's selected direction. |
| Catalogue first | Early public utility and easier external collaboration | Can succeed without producing useful runtime priors | Insufficient alone for the requested project. |
| Shared corpus, catalogue and priors together | Early open-data utility and continuous feedback from the consumer | Requires two release criteria and disciplined scope | Selected direction; recommended implementation structure. |
| Full public uploads, rankings, billing, and mapping together | Broad launch feature set | Moderation and platform work compete with unresolved geometry | Defer unless operating resources explicitly support it. |

### Publication and operations

| Approach | Benefit | Cost or limitation | Recommendation |
| --- | --- | --- | --- |
| Static STAC catalogue and immutable objects | Simple publication, mirroring, anonymous access, no read-server dependency | Limited interactive search; writes require a publication process | Start here for a curated corpus if uploads remain maintainer-reviewed. |
| Postgres/PostGIS catalogue plus static exports | Better region search, mutable review/job state, concurrent submissions | Database operation, migrations, backups, and API maintenance | Adopt early if authenticated submissions or concurrent curation are first-release requirements. |
| Bespoke catalogue without STAC mapping | Exact project vocabulary | Custom integrations and avoidable schema/API maintenance | Use custom schemas for priors, not automatically for generic asset discovery. |

Both static and database-backed choices can support public coverage pages. Starting static
does not mean deferring the catalogue. A database is justified by workflow/search needs rather
than a specific site count. Avoid building both Python and Go web services: choose one control
plane; the processing worker remains isolated. Go is reasonable given velocity.report
experience, while a Python control plane may reduce friction with the geospatial pipeline.
Team maintenance capacity should decide.

### Runtime representation

| Approach | Benefit | Cost or limitation | Recommendation |
| --- | --- | --- | --- |
| Sparse points/surfels | Few extraction assumptions; preserves irregular structures and supports established registration | Larger data and matching cost; weaker semantic explanation | Establish as the reference baseline. |
| Planes/edges/poles only | Compact, interpretable, potentially fast | Extraction instability, lost detail, degenerate scenes | Benchmark against the point baseline; do not assume superiority. |
| Hybrid landmarks plus sparse support cloud | Compact anchors with fallback geometry and auditable support | Two representations and explicit consistency/provenance needs | Likely best deployment product, subject to measurement. |
| Dense COPC at runtime | Maximum retained detail and research flexibility | Pi memory, latency, bandwidth, and unnecessary parsing | Keep for research and review rather than default sensing. |

### Registration assistance and execution location

| Approach | Benefit | Cost or limitation |
| --- | --- | --- |
| Operator supplies approximate pose and reviews fit | Smallest search space; practical deployment debugging; works offline | Human labour and possible confirmation bias; must still check against independent evidence. |
| Automatic within a known site | Repeatable deployment with little interaction | Needs robust hypothesis rejection and enough distinctive overlap. |
| Global place discovery | Minimal initial location knowledge | Much larger retrieval/search/ambiguity problem; not needed for a known installation. |
| Offline workstation solves, Pi validates | Reuses mature tools; avoids shipping a full registration stack on the Pi | Installation package becomes pose-specific; moving the sensor requires re-alignment. |
| Pi solves locally | Self-contained and can handle redeployment | Requires an identified Go/C++/other registration implementation and measured ARM performance. |
| Cloud solves deployment pose | Central compute and easier algorithm updates | Upload/privacy dependency and loss of offline autonomy; optional later service only. |

The draft names Open3D for service processing but leaves the live runtime solver
implementation open. Specify whether the first deployment consumes an offline-computed pose
or runs an on-device solver before committing to a 10-second Pi gate. Benchmarking
workstation Python does not establish either.

### Georeferencing, sources, and fusion

| Approach | Benefit | Cost or limitation | Recommendation |
| --- | --- | --- | --- |
| High-quality local map with approximate geographic placement | Useful local alignment without immediate survey cost | Cannot promise global position accuracy | Catalogue with explicit global uncertainty; allow local-use profile. |
| Survey/control-anchored map | Independent global accuracy and consistent multi-source alignment | Survey access, calibration, datum, and field cost | Needed for absolute-accuracy claims and selected reference sites. |
| OSM/coarse public geometry | Broad coverage and understandable context | Tags/footprints do not establish measured facade-level accuracy | Context/coarse initialisation only until validated for a specific site. |
| Separate accepted source maps | Preserves disagreements and reduces early fusion risk | Consumer/product selection remains necessary | First release should support selecting a reference map plus supporting observations. |
| Immediate multi-source fusion | Potential completeness and persistence gains | Correlated errors, inconsistent scale, artefacts, and complex uncertainty | Add after single-source baselines; require demonstrated improvement. |

[OSM Simple 3D Buildings](https://wiki.openstreetmap.org/wiki/Simple_3D_buildings) models building
outlines, parts, and height-related attributes. My recommendation to treat these as coarse context
is an engineering inference: that representation does not itself supply the independent alignment
and uncertainty evidence required by this service's proposed runtime gates. Keep
attribution/licence lineage distinct when combining it with measured geometry.

## 6. Resource and funding review

The draft's layer-by-layer object-storage arithmetic is useful as a sensitivity model. Its overall
hosting totals are not yet a credible operating budget because scan distribution, rebuild
frequency, scratch peaks, mirrors, and human review are unmeasured. A single canonical generation
also needs a policy: byte-identical reproduction of old derived products may require the historical
canonical asset or a reproducibly regenerable equivalent and its exact dependencies.

Prioritise measuring capture cost, reviewer minutes, failed-job rate, peak scratch space,
CPU-hours, source retention, and cost per useful published site. Track catalogue operation and
experimental processing separately. Require an explicit monthly experiment cap and a public-service
budget before choosing managed infrastructure or promising retention.

Sponsorship can begin manually, as support for a region and its upkeep. Do not price a
region solely from storage bytes: surveying, re-observation, and stewardship can dominate.
Publish the allocation and reserve policy, avoid ownership language, and keep sponsor
status out of quality decisions. The draft's annual sponsorship amounts are fundraising
hypotheses, not validated willingness to pay.

## 7. Recommended sequence for the selected direction

1. Agree the deployment assistance level and inventory real inputs,
   rights, reference measurements, and budget.
2. Define one shared site/capture/asset/frame contract and map its discovery fields to STAC; retain
   detailed priors/evidence schemas separately.
3. Publish one reviewed site with source provenance, coverage, uncertainty, and COPC access while
   building the local alignment harness against it.
4. Establish sparse-cloud registration and operator-assisted/offline alignment as baselines unless
   the user selects a stricter automation requirement.
5. Add primitives/hybrid products and compare pose, downstream spatial error, memory,
   time, and acceptance/rejection behaviour.
6. Execute the full repeated 2–3 block, three-class experiment after
   the harness and blind partitions are sound.
7. Release runtime eligibility separately from catalogue publication; integrate scene
   context before experimental background seeding.
8. Expand to the curated pilot with static or database-backed publication selected from actual
   contribution workflows; add measured funding and retention policies.

Successful localisation earns continued runtime investment. Unsuccessful localisation
does not erase the catalogue's usefulness, but it must prevent claims that the
catalogue provides approved runtime priors. Equally, a successful viewer must not be
mistaken for a successful localisation experiment.

## 8. Initial experiment using the existing captures

A read-only filesystem inventory found 190 `.pcap` files totalling approximately 130.79 GB
recursively under the supplied directory, alongside analysis, CSV/JSON, and VRLOG
artefacts. These are not 190 independent captures: the tree includes derived material, and
one file named as a summary is only 716 bytes. No full packet decode, duration
verification, or scene classification was performed.

Inventory source-to-derived relationships before choosing trials or estimating storage.
Inspect the existing analysis summaries to find a trip overlapping a long stop, then
validate that selection from packet data. Directory names and timestamps do not
establish geography or whether the sensor moved.

Trips provide viewpoint diversity, route coverage, and potential connections between
sites. Long stops provide repeated local measurements and changed visibility as
traffic passes. Twenty minutes of one parked vehicle is still one parked vehicle, not
evidence that it belongs to the static world.

| Method | Benefit | Risk | Initial role |
| --- | --- | --- | --- |
| Accumulate the complete stop with no motion correction | Simple reference for density and sampling gaps | Wobble thickens roads/walls and smears kerbs | Diagnostic baseline. |
| Select stable intervals and accumulate | Avoids much motion distortion with limited processing | Discards useful views and retains scan-line gaps | Conservative baseline. |
| Register short submaps, deskew where timing permits, then accumulate | Tests whether small movements fill gaps while keeping surfaces sharp | Estimated pose noise can exceed actual wobble; motion may be weakly observable | Main static-map experiment. |
| Independently reconstruct trip odometry/SLAM | Larger local map and overlapping views | Drift, dynamic objects, and false loop closure | Route-map baseline. |
| Jointly optimise trip and stop maps | Potentially improves coverage and local constraints | Circular validation and correlated errors | Follow independent map comparisons. |

Estimate small movements from stable structure distributed around the sensor, not the road plane
alone. Use near-stationarity as a hypothesis, not proof of zero motion. Avoid letting noisy ICP
invent a trajectory. Preserve time and pose estimates with their uncertainty; cap repeated
contributions from the same time block so dense observations do not overpower independent views.

Distinguish packet-capture arrival time from point acquisition time. The existing
[point types](../../internal/lidar/l2frames/types.go) include acquisition-time fields, but
that does not prove the supplied files retain reliable sensor timing. Inspect payload
timestamps, firing-time offsets, monotonicity, wraparound, dropped packets, and
capture-clock jumps before selecting deskew. GPS-disciplined timestamps, if present in
payloads, do not imply recorded GPS positions.

Do not accumulate a wobbling capture into the production polar grid as though each ring/azimuth
still observes the same world ray. Reconstruct metric 3D submaps outside the live sensing pipeline.
Separate scan-to-scan alignment from within-scan motion correction, and report the remaining error
when timing or motion is insufficient. The wobble hypothesis is plausible but remains untested.

For timestamps-only recordings, start with a LiDAR-only baseline such as
[KISS-ICP](https://github.com/PRBonn/kiss-icp). It is odometry, not a complete loop-closed
map. Evaluate [KISS-SLAM](https://github.com/PRBonn/kiss-slam) for loop-closed mapping
where the route supports it. [LIO-SAM](https://github.com/TixiaoShan/LIO-SAM) becomes a
candidate for future recordings with suitable synchronised IMU, calibration, and point
timing. No candidate's compatibility with these files has been established. Never fabricate
IMU inputs to satisfy a pipeline interface.

Use trip-built maps to localise static captures excluded from map fitting, then reverse
the test using independently built stop maps and held-out trip sections. If a stop is part
of the trip file, exclude its interval and fitting overlap explicitly. Time slices from
one stop measure repeatability, not independent global accuracy. Loop closure also
measures consistency rather than absolute truth.

Compare 30 seconds, 2, 5, 10, and 20 minutes of accumulation. Measure coverage at fixed density,
road/wall thickness, kerb sharpness, held-out surface error, pose stability, transient
contamination, processing cost, and output size. Choose duration from the coverage-versus-blur
result; longer may plateau or degrade. Keep raw data and reproducible recipes for every variant.

Publish a reviewed catalogue entry and approved derived COPC alongside the comparison. Approximate
global placement must carry uncertainty and may span several S2 cells. A locally useful map can be
selected explicitly even before its global placement is validated. The first milestone is one
reproducible trip/stop pair, not new hardware or a full three-scanner survey. Keep the larger
campaign as a later cross-sensor robustness test.

## 9. Hardware additions for better future captures

Improve acquisition in layers: repeatable mechanics, common timing, motion measurement, geographic
position, and independent reference. Hardware does not retroactively fix the existing recordings.
The existing corpus remains useful for the LiDAR-only baseline and for identifying which limitation
actually needs an upgrade. The options below are engineering candidates, not purchase commitments.

### Priority and tradeoffs

| Addition | Main benefit | Tradeoff or prerequisite | Priority |
| --- | --- | --- | --- |
| Rigid common plate for LiDAR and IMU, repeatable mount, cable strain relief, measured reference marks | Keeps relative sensor geometry stable and makes recalibration detectable | Soft independent mounts can create unmeasured relative motion; dimensions and repeatability need checks | First for any moving rig. |
| Reliable capture computer, sustained-write storage, packet-loss counters, stable power | Preserves complete scans and diagnostics | Faster storage does not solve timing; instrument drops and clock resets | Verify existing setup before replacing it. |
| Common clock/synchronisation wiring, PPS or supported PTP, logged lock state | Aligns LiDAR, IMU, and GNSS acquisition times | Requires compatible electrical interfaces, firmware, clock discipline, and measured offset | Essential alongside added navigation sensors. |
| Calibrated, raw-output IMU with hardware sync and suitable dynamic range/noise | Helps estimate wobble and intra-scan motion; supports inertial-aided odometry | Bias, temperature, time offset, and extrinsics need calibration; acceleration alone does not provide stable position | Highest-value sensing addition for motion correction. |
| Multi-band GNSS with raw observables and RTK/PPK capability, good antenna and ground plane | Anchors routes/sites globally; supports correction and later reprocessing | Corrections/base reference, multipath, antenna lever arm, and sky view govern actual accuracy | Next when geographic alignment is the main constraint. |
| Dual-antenna GNSS heading with a rigid measured baseline | Heading while stationary or moving slowly without relying on magnetic compass/course-over-ground | More space, antenna cost, sky visibility, and baseline-to-LiDAR calibration | Valuable for frequent deployment/redeployment or automatic known-site alignment. |
| Survey targets plus independent check measurements, rented RTK rover or total station as appropriate | Measures actual global/local map error instead of internal consistency | Field effort and access; GNSS can struggle near walls, total station needs sight lines/control | Early validation investment at one or two reference sites. |
| Calibrated indexed tilt/pan arrangement for static capture | Deliberate additional ray coverage with repeatable scan positions | Mechanism and pose calibration; each changed pose must be registered | Optional experiment if fixed-ring sampling remains the bottleneck. |
| Higher-density or professional mobile scanner | More complete observations and an additional capture class | Substantial acquisition cost; exports, proprietary processing, and truth claims still need review | Borrow/rent later for comparison before buying. |

Do not choose an IMU by output rate alone. Check noise, bias stability, timestamp semantics, raw
accelerometer/gyro availability, clipping, temperature recording, and synchronisation support. A
200 Hz-class logging target is a starting engineering envelope, not a proven minimum or promise of
sufficient bandwidth for every vibration. Derive the required rate from measured motion and
filtering. Mount LiDAR and IMU rigidly together; isolate the complete assembly only with a
justified mechanical design. A flexing mount between the two invalidates the calibrated transform.

For timing, a 10 ms mismatch at 10 m/s corresponds to 0.10 m translation before rotational effects.
That makes end-to-end offset validation more useful than merely having a high-resolution PCAP
clock. PPS marks seconds; the system also needs epoch/time messages and acquisition timestamps. PTP
support must be checked against the exact LiDAR model, firmware, network path, and grandmaster
profile. Do not assume plugging all devices into the same computer synchronises them.

### Concrete product families to evaluate

- **Integrated IMU/GNSS:** [SBG Ellipse-N](https://www.sbg-systems.com/ins/ellipse-n/) is an
  example combining inertial and GNSS measurements with synchronisation interfaces. An
  integrated unit reduces some wiring/calibration work but costs more and may introduce
  proprietary post-processing. Verify raw-data export and the exact generation's specifications;
  it does not eliminate LiDAR-to-unit calibration.
- **Modular GNSS:** [u-blox ZED-F9P documentation][f9p] describes a multi-band RTK receiver
  family and timing integration. A development board still needs suitable antennas, power,
  correction/logging arrangements, and mounting. Receiver accuracy specifications are
  conditional, not a guarantee in urban streets.
- **Stationary heading:**
  [ArduSimple simpleRTK3B Heading](https://www.ardusimple.com/user-guide-simplertk3b-heading/)
  provides a dual-antenna route to heading. Select baseline geometry for required accuracy and
  practical mounting; record antenna order and heading offset. Dual antennas do not replace an IMU
  for high-frequency motion or provide complete orientation from one baseline alone.
- **Independent control:** [Emlid's control-point workflow][control] illustrates surveyed
  references and raw logging. Borrowing/renting a survey receiver or engaging a surveyor may be
  more useful initially than adding a premium navigation unit to every capture rig. Check-point
  measurements must remain independent of map fitting.
- **LiDAR timing interface:** Consult the exact sensor's [Hesai Pandar40P manual][p40] before
  selecting PPS/NMEA or PTP hardware. The manual was located through official search, but a full
  fetch timed out in this review; pinout, voltage, firmware, and compatibility are unverified.

These sources establish capabilities to investigate, not an interoperable bill of materials. No
current prices have been established. Before purchase, compare complete installed cost: antennas,
base/correction service, cables, logger, power, enclosure, mounting, software, calibration, and
field time. An inexpensive module can require more integration work than an integrated instrument.

[f9p]: https://content.u-blox.com/sites/default/files/ZED-F9P_IntegrationManual_UBX-18010802.pdf
[control]: https://docs.emlid.com/reachrs3/ppk-quickstart/ppk-uav-mapping/configuring-reach-for-mapping/
[p40]: https://www.hesaitech.com/wp-content/uploads/2025/02/Pandar40P_User_Manual_402-en-241220.pdf

### Suggested packages

| Package | Contents | Choose when |
| --- | --- | --- |
| Existing-data research | Current PCAPs, workstation, LiDAR-only reconstruction, independent review | Begin immediately; no purchase required to test route/stop agreement. |
| Better relative geometry | Repeatable mount + synchronised IMU + loss/timing instrumentation | Wobble or driving distortion dominates; geographic accuracy can remain explicitly uncertain. |
| Better geographic maps | Relative-geometry package + raw-logging RTK/PPK GNSS + reference checks | Combining sites/trips needs defensible global placement. |
| Easier repeat deployment | Geographic package + dual-antenna heading or an appropriate integrated INS | Operator heading input is undesirable and repeated setup justifies complexity. |

My provisional recommendation is to start the existing-data comparison now, then add a synchronised
IMU and repeatable mounting for a short repeat capture. Add GNSS in the same build if geographic
placement is already a blocking requirement, rather than rebuild the harness twice. Establish
independent control at one test site before buying a more expensive scanner. Final choice depends
on the user's automation preference, rig geometry, measured timing quality, and budget.

## 10. Operator assistance as an experimental variable

The user's ten-second corner snap is a target to test, not a promised task duration. Begin with
automatic candidate alignment and a lightweight review, then expose additional controls only when
the candidate is absent or fails validation. Compare these levels on the same held-out captures:

| Level | Operator action | Benefit | Limitation |
| --- | --- | --- | --- |
| Suggested fit | Inspect and accept an automatically proposed alignment | Least interaction when geometry is distinctive | Must detect wrong but visually plausible matches. |
| Quick map snap | Match a corner and its edge directions, or several corresponding corners, in scan and OSM | Fast coarse placement using existing context | OSM accuracy and feature correspondence limit global accuracy; map height may be unknown. |
| Controlled rigid alignment | Match distributed physical points to coordinates with declared uncertainty | More defensible global placement and measurable fit | Reference collection and point selection take time. |
| Local-only acceptance | Keep a good local map with uncertain geographic placement | Preserves utility when references are inadequate | Cannot claim validated global position or exact geographic coverage. |

Distinguish three transforms: scan-to-local-map reconstruction,
local-map-to-geographic-reference placement, and a later deployment sensor-to-map alignment. A
user placing the map geographically does not automatically solve every future sensor pose.
Record and version each transform separately.

Keep measured local geometry unchanged while estimating a rigid rotation and translation. Do not
warp a good scan to fit imperfect OSM geometry. Translation alone suffices only when orientation is
already trustworthy. A single point match does not determine rotation; a corner with reliable edge
directions adds orientation constraints, but symmetry and correspondence still need checking. For a
fully 3D rigid fit, use at least three non-collinear 3D correspondences, preferably more,
distributed across the scene, and separate check points. A 2D map fit cannot establish height or
complete tilt information without additional assumptions or measurements. Keep scale fixed for
metric LiDAR unless an explicit calibration investigation establishes a scale problem.

Intersection corners are useful when they identify the same stable physical feature in both
sources. Define whether the point is a building wall intersection, the top or bottom of a kerb, or
the intersection of fitted kerb lines. A rounded kerb has no unique sharp corner. An OSM
road-centreline junction, parcel vertex, roof outline, and street-level wall corner are different
objects and must not be snapped together merely because they appear close on a map.

OSM can provide a coarse alignment anchor without being a surveyed control network. Its own
[accuracy discussion](https://wiki.openstreetmap.org/wiki/Accuracy) describes differing
source errors and the risk of interpreting agreement within one source as accuracy. Label a
fit as OSM-aligned, imagery-aligned, or measured-control-aligned, preserving source version,
selected features, coordinate reference, and uncertainty. The label alone does not set a
numeric accuracy: independent checks do.

For each assistance level record active operator time, loading/compute time, point selections,
retries, abandonment, residuals at fit points, and error at independent check points. Report the
fraction of deployments completed within 10, 30, and 120 seconds and the accuracy of those accepted
fits. Repeat a subset with different operators or blinded repeat selections to measure sensitivity
to clicking. Pre-stage assets for the ten-second interaction trial, but report total cold-start
time separately. Do not force acceptance when the time budget expires.

The simplest useful workflow is: propose a rigid fit, let the operator correct
correspondence or placement, check withheld geometry, and accept or retain local-only
status. Use measured controls when geographic error matters and the map anchors cannot
support the requirement. If independent controls are unavailable, report relative fit and
operator repeatability without an absolute-accuracy claim. The experiment should discover
the minimum intervention that produces a reliable result.

### First-test reference measurements

Prepare a point register before capture/alignment: stable point ID, exact physical definition,
measurement method, coordinate frame/datum, height convention, acquisition time, uncertainty, and
whether the point is for fitting or checking. Choose identifiable features distributed across the
test area rather than several adjacent corners. Include vertical constraints if assessing full 3D
alignment; a plan-view corner match alone does not provide them.

As an initial test design, aim for 6–10 identifiable references, reserving at least three for
checks and enough well-distributed fitting points to constrain the intended transform. This is a
starting budget, not a universal survey requirement. Keep check coordinates out of fitting,
operator snapping, and threshold tuning. Report the check-point measurement uncertainty alongside
alignment error; shared survey datum or instrument errors can still affect both sets.

Run the quick OSM workflow first without revealing measured coordinates to its operator.
Evaluate its result against the reserved checks. Then run the measured-control workflow using
only designated fitting points and evaluate against the same checks. Record
reference-collection time separately from per-deployment operator time, so a ten-second
alignment does not hide hours of preparation. Use separate operators or blinded/reordered
trials where practical to limit remembered placements.

## 11. Next review decisions

| Decision | Current state | What the answer changes |
| --- | --- | --- |
| Catalogue and priors together | Confirmed | Two delivery tracks sharing a corpus; update draft sequencing. |
| Assisted versus automatic alignment | Evaluate assistance levels; quick corner snap is a target | Measure time versus independently checked error instead of prescribing automation. |
| Intersection control coordinates | Measure references for first test; method and uncertainty pending | Independently compare OSM snapping with measured-control alignment. |
| Available capture assets | Trips and 20-minute stops confirmed; directory inventoried | Begin with existing data and preserve source/derived relationships. |
| Navigation/timing | User reports PCAP timestamps only; payload timing inspection pending | LiDAR-only baseline now; future synchronised IMU/GNSS optional. |
| Curated publishing versus immediate public submission | Not yet decided | Static publication versus database/API/review operation. |
| First useful prior: pose/context versus background acceleration | Recommendation: pose/context first | Integration depth and independent acceptance criteria. |
| Working budget and maintainer capacity | Not yet decided | Hosting choice, capture commitments, and realistic milestone scope. |

Reconcile the original plan after these choices. Preserve the earlier review
draft so its assumptions remain inspectable; do not silently present the
provisional alternatives as agreed architecture.
