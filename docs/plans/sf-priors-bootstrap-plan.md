# San Francisco priors: aerial baseline and alignment editor

- **Status:** Proposed; source metadata reviewed, extraction and field validation pending
- **Canonical:** [Geometry-prior service](../lidar/architecture/geometry-prior-service.md)
- **Related:** [Decision review](spatial-priors-service-review.md),
  [reference data](spatial-priors-reference-data-plan.md)
- **Updated:** 2026-09-13
- **Scope:** Bootstrap design and editor requirements; no dataset ingestion or implementation

## 1. Recommendation

Use the 2023 USGS aerial point cloud as the default geometric reference for SF. Overlay SF city
building vectors and a dated OSM extract as separately attributable evidence. Publish terrain,
building outlines and conservative heights first. Add roof planes and ground-capture refinements
where demonstrated useful. Develop the catalogue alongside these products, with separate gates.

Canonical means the accepted, versioned reference against which other observations are compared. It
does not mean every return is correct or every surface is visible. Independently measured control
can establish an aerial error; that creates a reviewed correction or exclusion and a new release. A
GPS reading or an attractive vector overlay cannot silently move the baseline.

Recommend a **web curator with a local/offline mode**, followed by a small velocity visualiser
integration for PCAP evidence and deployment alignment. Share bundles, measurements and correction
records between them. Keep ingestion, registration and release compilation outside either UI.

## 2. Review of the attached proposals

The core proposal is credible. The distinction between measured geometry, derived geometry and
semantic context needs to be stronger before it becomes a service contract.

| Proposal | Review and consequence |
| --- | --- |
| USGS 0.25 m aerial baseline | Use the point cloud plus DEM. A bare-earth DEM alone removes buildings and cannot supply roof geometry. |
| Aerial LiDAR is canonical | Accept as the default geometric anchor, subject to local checks, coverage and acquisition date. |
| Footprints plus roof reconstruction | Begin with association, heights and simple volumes. Reliable alignment and roof-point selection come before reconstruction. |
| City and OSM corroborate each other | Track shared lineage. Agreement between copied geometries is not independent survey evidence. |
| Fixed PCAP accumulation adds detail | Useful after motion correction and registration; repeated frames do not establish permanence. |
| Vehicle tracks demonstrate free space | Treat as uncertain traversability evidence only. Tracks cannot establish all intervening space as empty; visibility requires ray and occlusion analysis. Keep traffic observations outside public geometry releases. |
| A stable ID comes from the polygon | Give service features independent IDs, with source IDs and split/merge history. Geometry hashes identify revisions, not enduring buildings. |
| Ten to twenty centimetres registration | An experiment target, not a demonstrated capability or universal release threshold. Evaluate spatial error over the sensing area. |

### Verified source facts

The [USGS project report][usgs-report] identifies acquisition on 20 April 2023, Quality Level 0,
LAS 1.4, a 0.25 m DEM grid, horizontal EPSG:7131 and vertical EPSG:5703 with GEOID18. Its tested
non-vegetated point-cloud vertical RMSE is 1.73 cm. This is a project-level vertical statistic, not
centimetre horizontal accuracy at every roof edge. Classes include ground, unclassified, medium
vegetation, water, noise and bridges; building class 6 is not listed.

The [NOAA record][noaa] describes 0.15 m nominal pulse spacing and 654 delivery tiles of 1,000 by
1,000 metres. It links [original LAZ][laz] and [EPT][ept]. Its processing lineage says EPT uses
EPSG:3857 horizontally while retaining NAVD88/GEOID18 heights. Read each asset's actual metadata
rather than applying the original LAZ frame to every distribution. NOAA's horizontal accuracy
paragraph contains inconsistent units; do not promote that value to a release guarantee.

The earlier
[SF source review](spatial-priors-reference-data-plan.md)
records the city's footprint product as derived from a 2010 model. Verify the selected snapshot's
lineage during ingestion. Portal update dates do not establish acquisition dates. Differences
between those outlines, 2023 surfaces and current PCAPs may be real change.

## 3. Bootstrap pipeline

1. **Choose a bounded pilot.** Start around a fixed-PCAP site with planned measured
   control. Include adjoining blocks and at least one source-tile boundary. Add a
   second, geometrically different area before claiming the approach generalises.
   Confirm actual coverage before allocating jobs.
2. **Freeze inputs.** Record source URLs, versions, retrieval/acquisition dates, hashes, licences,
   extents, CRS definitions and vertical references. Preserve original assets and source IDs.
   Snapshot SF and OSM independently; record any known shared provenance.
3. **Extract with a halo.** Use bounded EPT reads for reconnaissance; obtain original LAZ for the
   pilot's reproducible reference extraction and source-attribute checks. Keep full resolution for
   measurement, with separate decimated previews. [PDAL's EPT reader][pdal-ept] supports spatial
   bounds and resolution selection. Log the actual selection and processing versions.
4. **Normalise explicitly.** Use the documented CS13 metre frame for SF measurement, with a nearby
   rendering origin for floating-point precision. Preserve orthometric heights. Store the exact
   transformation pipeline and required grids. Acquisition date, datum epoch and publication date
   are different fields. Web Mercator coordinates are for display, not local distance fitting.
5. **Check the cloud.** Inspect bounds, point classes, flags, density, ground
   coverage, overlaps and withheld/noise handling. Compare independently measured
   control. Inspect tile and flight-strip boundaries where attributes permit; missing
   strip identity is a recorded limitation.
6. **Associate buildings.** Repair invalid polygons without losing source geometry or holes. Match
   city and OSM candidates with many-to-many relationships for parts and merged buildings. Keep
   competing outlines visible. Do not union them into a single supposedly observed footprint.
7. **Derive conservative geometry.** Estimate local ground, select plausible roof returns using
   height, planarity and neighbourhood evidence, and retain rejected/uncertain areas. Never relabel
   all unclassified points as roofs. Publish robust roof-height summaries, coverage and residuals;
   preserve measured roof edges separately from inferred wall lines and extrusion surfaces.
8. **Review disagreement.** Classify it as frame error, rigid displacement, local deformation,
   outdated geometry, different physical feature, occlusion or insufficient evidence. Fit a
   correction only to comparable stable geometry, then test withheld features.
9. **Compile a candidate release.** Produce a source catalogue, inspection cloud, terrain/coverage
   products, attributed vectors, alignment diagnostics and a compact runtime bundle. Keep richer
   meshes optional. Sign/version the accepted manifest and preserve rollback to its predecessor.

Do not download the entire city merely to discover whether the first site works. Measure bytes,
wall time, peak memory, scratch space and operator time in the pilot; use those measurements to
estimate city-wide processing. Cache immutable inputs and rerun only affected dependencies.

### Reconstruction choices

| Approach | Benefit | Cost or limitation | Position |
| --- | --- | --- | --- |
| Footprints and robust heights | Cheap, inspectable baseline; readily highlights disagreements | Extruded walls may be inferred; roof overhangs need separate treatment | First release |
| Roof planes and detailed reconstruction | Better roof structure and matching surfaces | Requires clean roof points and aligned outlines; more failure cases | Selective second stage |
| Extract all outlines from LiDAR | Can expose omissions and changed buildings | Vegetation, touching roofs and occlusion make identity/topology harder | Generate review candidates |
| Fuse ground PCAP detail immediately | Potentially useful façades and kerbs | Adds pose, timing, persistence and visibility uncertainty | Separate evidence layer until validated |

[Roofer's architecture][roofer] requires classified points and aligned footprints and does not
provide automatic footprint extraction. It is a later reconstruction candidate, not the alignment
solution. Evaluate its pinned version, licence and output validation before integration.

## 4. Source precedence and inheritance

| Property | Default authority | Exception |
| --- | --- | --- |
| Geographic placement | Accepted aerial baseline in its documented frame | Independent control demonstrates a source error |
| Ground and observed roof surfaces | Valid aerial returns | Excluded region, poor coverage or reviewed newer measurement |
| Wall/ground-contact outline | Best supported physical evidence; city/OSM candidates remain labelled | Roof edge alone does not establish wall location |
| Names, uses and identity associations | Attributed city/OSM fields | Conflicts retained for review; geometry does not decide semantics |
| Kerbs and façades absent from aerial coverage | Unknown or explicitly inferred initially | Accepted ground survey/capture evidence |
| Parcel boundaries | Attributed cadastral context | Never automatically snap physical surfaces to lot lines |

Use two separate systems for inheritance:

- **Policy:** project defaults, then L10 steward policy, then explicit L13/area policy, then a
  feature exception. Record the effective value and its origin for each field. An exception cannot
  waive mandatory provenance, datum or release validation. Conflicting overlapping area rules
  require explicit resolution; ordering by load time is forbidden.
- **Geometry:** source frame conversion, followed by an explicitly approved correction for a
  survey, strip, tile, capture or bounded feature set. Geographic containment alone never applies a
  transform. An override names the correction it replaces; composition is explicit and acyclic.

A source tile may cross many S2 cells, and an S2 cell may contain several sources. Maintain a
many-to-many intersection index. L10 is stewardship and packaging; L13 is retrieval. Follow
[S2 parent/token conventions](../lidar/architecture/geographic-indexing.md); neither is a survey
unit. Preserve whole-feature identity across clipped outputs, and use halos for seam validation.

Approving a correction identifies every affected feature, tile and cell, plus dependent products
that need rebuilding. Replacing a parent policy or source revision marks dependent approvals stale
for review; it must not silently reinterpret a published release. Do not average incompatible
surfaces or smooth a seam merely to conceal disagreement.

## 5. What the editor must show and permit

The primary task is: select a disputed region, understand the cause, measure it, preview a proposed
resolution, and approve an auditable change. Freehand polygon editing is secondary.

| View or action | Required behaviour |
| --- | --- |
| Layer stack | Independently toggle aerial points, ground raster, SF outlines, OSM, derived geometry, measured controls and local captures; show dates and source revisions |
| Linked plan, 3D and section views | Share selection and clipping slab; separate roof edges from walls/kerbs at street level; support opacity and before/after comparison |
| Source boundaries | Toggle source tiles, strips when available, L10/L13 and correction footprints independently |
| Guidelines | Draw point pairs, lines, planes and right-angle/parallel guides; distinguish visual aids, soft constraints and independently measured control |
| Alignment inspector | Select moving and reference sources explicitly; fit translation or rigid pose as justified, with axes/units shown; preserve raw coordinates |
| Residual display | Signed east/north/up vectors, normal distances and cross-sections; fixed colour scale and legend; original and proposed residuals |
| Regional dashboard | Aggregate mismatch magnitude, support and review status by cell or tile; distinguish untested, unobservable and passed regions |
| Rule inspector | Explain why a feature has its current source/geometry, what it inherits and which override wins |
| Change review | Show scope, dependencies, held-out checks and geometry differences; draft, accept, reject, supersede and rollback |

Every fit records sample count and spatial distribution, correspondence type, median and p95
residual, RMSE where meaningful, inlier fraction, estimated translation/rotation and uncertainty.
Separate fitted residuals from independent check errors. Point-to-plane distance alone may conceal
motion along a plane: a flat road cannot constrain horizontal translation or yaw. Report those
components as unobservable rather than displaying a reassuring zero.

An arrow means **the proposed correction to this named source in this named frame**. A tile's
residual summary is diagnostic, not automatic permission to move the tile. If neighbouring tiles
share a bias, investigate the parent survey/frame before allowing individual fixes. If only one
building differs, inspect date and feature semantics first. Non-rigid corrections require a
separate review beyond the MVP rigid-fit workflow.

Ten-second corner snapping remains an operator-time experiment. A guide is not a surveyed point,
and one corner does not constrain full pose. Store the selected correspondences, preparation time
and interaction time; measure the result against controls withheld from fitting and operator view.

## 6. Web versus velocity visualiser

| Choice | Advantages | Tradeoffs |
| --- | --- | --- |
| macOS visualiser first | Existing point-cloud rendering and capture workflow; convenient for local PCAP comparison | Platform restriction; catalogue, GIS editing and review model still need building |
| Web curator first | Community access, linked maps, attribution and review queues; can run against local bundles | Point-cloud streaming, offline caching and precise 3D picking need a performance prototype |
| Two full editors | Every workflow in both environments | Duplicated interaction, validation and release logic |

The current [web package](../../web/package.json) includes Svelte and Leaflet. The native
[renderer][native-renderer]
already composites background/foreground clouds, and
[MetalRenderer](../../tools/visualiser-macos/VelocityVisualiser/Rendering/MetalRenderer.swift)
supports debug overlays. These are useful foundations, not evidence that either already
supports multi-source geospatial curation.

Start with a local-served web inspection prototype and shared change records. Benchmark
representative full-detail crops on the intended hardware before selecting a 3D rendering
library. The native app should later open the same site/release, overlay the prior in
sensor coordinates and export alignment evidence. Preserve existing sensor-local processing
until its world-frame integration is validated. Public upload is explicit; private PCAPs
and traffic tracks remain local by default.

## 7. Pilot gates and next decisions

| Gate | Evidence required |
| --- | --- |
| Reproducibility | Immutable inputs, pinned processing and repeatable output/manifest hashes |
| Frame correctness | Original LAZ/EPT comparison after documented conversion; independently checked horizontal and vertical reference |
| Geometry quality | Report coverage, uncertain roof selection, holes/parts and source disagreement; inferred surfaces visibly labelled |
| Alignment | Held-out controls across the sensing region; residuals before/after; no regressions across neighbouring boundaries |
| Editor usefulness | Time and error for automatic suggestion, guided snapping and measured-control workflows; failed cases retained |
| Fault detection | Deliberately shifted/rotated tile, wrong height reference, changed building and underconstrained flat scene are distinguished |
| Offline delivery | Open a frozen site bundle without network; missing dependencies fail visibly; prior release remains usable |

Before processing, choose an intended-use error budget for horizontal, vertical and edge placement.
The previously discussed 10–20 cm is a provisional registration experiment target. Measure
reference uncertainty too; insufficiently accurate control cannot certify that target. Catalogue
publication may proceed while a site's runtime-geometry gate remains failed or unknown.

Next review choices are the first site and extent, the measurement method, and agreement on
web-first curation with a narrow native bridge. A steward owns source acquisition,
licence/provenance records and the mismatch queue; a sponsor can fund control or records
acquisition. No purchase or upstream OSM edit is implied by this plan.

[usgs-report]: https://prd-tnm.s3.amazonaws.com/StagedProducts/Elevation/metadata/CA_SanFrancisco_B23/USGS_CA_SanFrancisco_B23_Project_Report.pdf
[noaa]: https://www.fisheries.noaa.gov/inport/item/73386/full-list
[laz]: https://rockyweb.usgs.gov/vdelivery/Datasets/Staged/Elevation/LPC/Projects/CA_SanFrancisco_B23/CA_SanFrancisco_1_B23/LAZ/
[ept]: https://s3-us-west-2.amazonaws.com/usgs-lidar-public/CA_SanFrancisco_1_B23/ept.json
[pdal-ept]: https://pdal.io/en/stable/stages/readers.ept.html
[roofer]: https://github.com/3DBAG/roofer/blob/main/ARCHITECTURE.md
[native-renderer]: ../../tools/visualiser-macos/VelocityVisualiser/Rendering/CompositePointCloudRenderer.swift
