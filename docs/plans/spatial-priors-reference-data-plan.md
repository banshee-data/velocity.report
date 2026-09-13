# Spatial priors: public reference data and protection from GPS error

Build a versioned reference-data package for each L10 region, maintained by community
stewards and funded where useful by sponsors. Preserve local LiDAR geometry
independently of GPS, and use measured control and suitable public GIS to establish
geographic placement with explicit uncertainty.

- **Status:** Research findings and proposed implementation plan
- **Canonical:** [Geometry-prior service](../lidar/architecture/geometry-prior-service.md)
- **Related:** [Service plan](spatial-priors-service-plan.md),
  [decision review](spatial-priors-service-review.md),
  [S2 conventions](../lidar/architecture/geographic-indexing.md)
- **Scope:** United States sources, with San Francisco/Bay Area as the first worked example

The regional focus follows the existing capture context, not a decoded GPS location. Source pages
were checked on September 9, 2026. Public discovery and documentation were inspected; full
datasets, local coverage, endpoint completeness, and achieved alignment accuracy have not been
verified. No records were purchased, external requests submitted, or upstream maps edited.

## 1. Recommended position

The [SF bootstrap and editor plan](sf-priors-bootstrap-plan.md) develops the selected aerial-LiDAR
baseline with SF and OSM vectors, including source-frame pitfalls and reviewed correction rules.

Use a hierarchy of evidence with separate roles, rather than replace GPS with one
supposedly perfect map. Legal boundary records, geodetic control, and observable
geometry answer different questions. A parcel line can be authoritative for a land
record yet unsuitable for matching a kerb or wall.

| Reference role | Preferred evidence | What it establishes |
| --- | --- | --- |
| Geographic control | Recovered survey marks, current published coordinates, independently measured check points | Position/height in a defined reference frame, for the components actually measured. |
| Surface matching | Suitable public LiDAR, documented photogrammetry, surveyed kerb/building geometry | Correspondences to physical surfaces, subject to age, visibility, and survey error. |
| Cadastral context | Parcel fabric, recorded surveys, right-of-way maps, plats | Land divisions and documentary ties; physical correspondence must be demonstrated. |
| Coarse discovery/context | GPS, OSM, general orthoimagery, aggregated building footprints | Search area, feature identity, and candidate placement, not automatic precision. |

These are default roles, not an unconditional ranking by institution. Evaluate the actual metadata,
survey method, independently checked error, physical feature, and acquisition date of each source.
Do not assume that the newest portal timestamp is the newest survey.

## 2. Why GPS must not distort the prior

[GPS.gov](https://www.gps.gov/gps-accuracy) describes roughly 4.9 m typical smartphone accuracy
under open sky and worse performance near obstructions. This is illustrative, not an error bound or
a sigma for our equipment. Receiver design, geometry, blockage, and multipath matter. A consumer
GPS point cannot establish sub-decimetre placement merely by carrying many decimal places.

Keep four independent records:

1. Original point measurements and sensor calibration.
2. Local reconstruction and relative scan/trajectory constraints.
3. Raw GPS observations, timestamps, receiver quality indicators, and reported uncertainty.
4. A versioned local-map-to-geographic transform with control evidence and validation results.

Use GPS initially to retrieve candidate reference regions. A new fix must not move an
accepted prior, change its owner cell, or reshape a settled local reconstruction. When
location is uncertain, query all intersecting candidate cells, including across L10
boundaries, with a bounded search budget. An uncertainty footprint is not verified
coverage and should not earn mapping credit.

First implement geographic placement as an offline rigid transform estimated from approved
reference correspondences. If GPS-assisted trajectory optimisation is later added, retain a
LiDAR-only baseline, use robust residual gating and a defensible uncertainty floor, and allow bad
GPS constraints to be rejected. A long sequence of correlated biased fixes must not outweigh a few
independent controls. Keep the original fixes even when excluded, with exclusion reasons.

Treat shared GNSS bias, bad timestamps, lever-arm errors, and datum mismatch as possible systematic
errors. More samples from one receiver do not necessarily remove them. Abrupt position changes
should trigger review, not automatic map deformation. If reliable constraints disagree, preserve
candidate alignments and fail geographic acceptance rather than silently averaging the conflict.

Keep runtime geometry in a local metric frame. Resolve geographic re-indexing only when publishing
a newly accepted alignment revision. Store predecessor links and update affected cell manifests
atomically; consumers pin release IDs. No-prior and offline sensing remain available throughout.

## 3. Free or readily accessible sources worth using

The access column describes the documented acquisition route, not proof that every
file has been downloaded successfully. Free viewing, free downloads, and
permission to redistribute are different.

| Dataset/provider | Access and effort | Best use | Limitations and rights status |
| --- | --- | --- | --- |
| [CCSF Geodetic Network](https://sfpublicworks.org/services/ccsf-geodetic-network) | Public coordinate/report downloads, station descriptions, recovery diagrams, and KMZ; modest manual preparation | First source for locating SF control and planning our reference measurements | Check recovery condition, current validity, horizontal/vertical components, local datum, and reuse notices. |
| [NGS survey marks and datasheets](https://www.ngs.noaa.gov/datasheets/) | Public map, datasheets, state shapefiles, and county/location search | National control discovery; reference evidence outside SF as well | Some marks have useful height but weak horizontal position, or vice versa. Physical recovery and full datasheet review are essential. |
| [USGS 3DEP LidarExplorer](https://www.usgs.gov/tools/lidarexplorer) | Public geographic search and downloads; moderate data volume | Independent surface/terrain comparison and coarse 3D alignment | Check each survey's age, point density, horizontal/vertical accuracy, and rights. Airborne data may poorly observe street-level walls or kerbs. |
| [NOAA Digital Coast](https://coast.noaa.gov/digitalcoast/tools/dav.html) | Public area selection and configurable downloads; [bulk LAZ/EPT catalogue](https://coast.noaa.gov/htdata/lidar1_z/index.html) | Coastal/Bay Area LiDAR and elevation alternatives | Contains contributed datasets: check source metadata. May mirror the same survey as USGS, not independent evidence. |
| [SF building footprints](https://data.sfgov.org/d/ynuv-fyni) | Public DataSF dataset, documented GeoJSON/CSV exports | Candidate building corners and coarse alignment overlay | Derived from older imagery/model data; footprint splitting does not make every vertex a surveyed wall corner. Catalogue lists PDDL. |
| [SF current subdivision parcels](https://data.sfgov.org/d/45et-ht7c) | Public DataSF parcel layer; easy context acquisition | Parcel IDs, block context, locating relevant records | City's guide explicitly excludes precision use. Confirm dataset-specific reuse terms before packaging. |
| [SF recorded parcel history](https://data.sfgov.org/d/25dk-perw) | Public DataSF historical/current records | Explain parcel changes and identify historical references | Not a survey-accuracy substitute. Remove owner/person attributes from the public prior package. |
| [Caltrans right-of-way and survey records](https://dot.ca.gov/programs/right-of-way/rw-maps-surveys-records) | District 4 collection available through Maps on Demand; other districts vary | Road-adjacent surveys, monument ties, and right-of-way evidence | Document extraction and interpretation require more work than GIS download; verify date, coordinate basis, and use terms. |
| [USGS NAIP Plus imagery service](https://imagery.nationalmap.gov/arcgis/rest/services/USGSNAIPPlus/ImageServer) | Public image service; documented free public-domain orthoimagery download route | Visual reference, source identification, coarse independent placement | Pixel resolution is not positional accuracy. Inspect acquisition-level metadata and roof/ground displacement. |
| [Overture Maps](https://docs.overturemaps.org/) | Open object-store releases, CLI and geographic queries | Broad building coverage where local sources are absent | Aggregates upstream sources including OSM. Not independent survey control; licences vary by theme/source. |

Prioritise SF control, public LiDAR, and physical building/road geometry before buying
parcel records. Use parcel layers immediately for context, while seeking survey-quality
ties only where the first experiment shows a material accuracy gap. Do not purchase
county-wide archives to solve one corner.

### San Francisco findings that change the design

The city's [parcel guidance][sf-parcels] says the geometry should not be used for work requiring
precision. It also describes stacked condominium parcels sharing a base parcel. Do not count those
duplicates as independent geometric support. Recorded parcel maps are documentary evidence; their
digitised polygons are not automatically control coordinates.

The [geodetic-network page](https://sfpublicworks.org/services/ccsf-geodetic-network) lists both
horizontal and vertical control material, local frame documentation, and superseded records. It
explicitly invalidates HPN 119 after a plaque was moved during construction. Therefore the package
needs mark validity and recovery status, not just a coordinate table. A public coordinate can be
precise and no longer describe the physical marker.

The [building-footprint catalogue](https://catalog.data.gov/dataset/building-footprints-04ba1)
describes geometry derived from a 2010 model and subsequently split using parcel information. A
2026 catalogue update does not establish a 2026 building survey. Retain acquisition and publication
dates separately, and test selected corners against current scan geometry and withheld controls.

[sf-parcels]: https://sfdigitalservices.gitbook.io/data-standards-guide/standard-reference-data/basemap/parcels

## 4. What lot lines can and cannot establish

Do not force a LiDAR wall onto the nearest parcel line. Setbacks, overhangs, party walls, road
rights-of-way, fences, and boundary geometry can differ. An intersection corner needs an exact
physical definition: kerb top, kerb-line intersection, wall intersection, or recovered monument.
Legal parcel vertices and road centreline intersections are different features.

A useful recorded survey can tie a recovered mark to visible geometry through bearings, distances,
and a documented coordinate basis. A scanned assessor map without such ties is context. Digitising
or georeferencing a PDF adds measurement error; tracing more vertices does not remove it. Retain
the document, extraction method, units, closure checks where applicable, and reviewer conclusions.

Use the county/city surveyor and public-works GIS office for control, corner records, and survey
ties; use the recorder or assessor for recorded maps and parcel history. Office responsibilities
differ by jurisdiction. [Caltrans' survey guidance][caltrans-survey] lists these distinct records
sources. The job is technical reference acquisition, not adjudication of property boundaries.

[caltrans-survey]: https://dot.ca.gov/-/media/dot-media/programs/right-of-way/documents/ls-manual/10-surveys-a11y.pdf

## 5. Datum, age, and independence checks

Require source horizontal CRS, datum realisation, epoch where applicable, units, vertical
reference, and transformation method. Do not treat every latitude/longitude as interchangeable
WGS84, or label local city elevations NAVD88 without a verified conversion. Preserve original
coordinates and the normalised product. Use the
[NGS transformation resources](https://ngs.noaa.gov/datums/newdatums/GetPrepared.shtml) and tested
PROJ operations where appropriate; missing required transformations block precision use.

Separate resolution from accuracy: a 0.5 m grid or dense cloud does not promise 0.5 m or better
geolocation.
[USGS 3DEP documentation](https://www.usgs.gov/3d-elevation-program/about-3dep-products-services)
requires examining quality attributes, not just nominal pulse spacing. Do not apply national DEM
summary statistics as the accuracy of a particular high-resolution survey, nor assume vertical
accuracy describes horizontal placement.

Track a source-family ID and acquisition ID. NOAA and USGS may expose the same
survey; OSM and a building aggregate may share the same traced geometry or aerial
source. Agreement between copies is not independent validation.
[Overture's licensing/source documentation](https://docs.overturemaps.org/attribution/)
helps identify source overlap; it is not an error model. Prefer withheld field
checks over a vote among correlated basemaps.

## 6. L10 reference package and loading model

Make L10 the stewardship, discovery, and manifest unit, not a promise of uniform accuracy or a
single correction transform. Counties and source surveys can overlap multiple L10 cells. Download
each source once where practical; reference it from every intersecting package. Use actual S2
parent operations and canonical tokens, following the repository convention.

An immutable reference release under `references/{l10}/{release_id}/` contains:

| Component | Contents |
| --- | --- |
| Manifest | Canonical L10, footprint/coverage, jurisdictions, release identity, source/asset hashes, schema and previous release. |
| Control inventory | Source ID, physical description, horizontal/vertical coordinates and uncertainty, datum/epoch, validity, recovery state, documentary links. |
| Geometry layers | Building/road/parcel references kept separate, with source feature IDs and intended use. |
| Surface assets | Links to clipped COPC, DEM, or imagery assets where rights allow; source acquisition and lineage retained. |
| Frame operations | Original/normalised CRS, transformation recipe and grid hashes, units, height treatment, and known limits. |
| Quality report | Coverage gaps, source disagreement, date differences, independent checks, and approved uses. |
| Rights manifest | Licence/terms snapshot, attribution, redistribution/derivation permission, and restrictions. |
| Review record | Steward, independent technical review where needed, acquisition expense, decision reasons, and refresh state. |

The L10 manifest is small and references detailed assets. Load geometry on demand
for a deployment's area plus a halo, retaining L13 discovery where useful. Do not
fetch an entire regional point cloud onto a Pi. Support a prepared offline site
bundle with pinned reference and prior versions.

Do not crop away a control mark just outside the cell that anchors a relevant survey.
Retain source footprints, cross-boundary references, and neighbour dependencies. Cache keys
include source release, datum recipe, and product profile, not cell token alone. A
withdrawn mark invalidates dependent alignment candidates and triggers review of releases
that used it; it does not rewrite raw scans.

The [NGS Data Explorer API](https://www.ngs.noaa.gov/web_services/data-explorer.shtml) supports
bounding and radial discovery but documents a 500-result limit. An adapter must subdivide queries
and reconcile IDs before claiming complete L10 coverage. Fetch full datasheets for candidate
controls because the API exposes only a subset of their attributes. Other portal adapters likewise
need pagination and completeness checks.

## 7. Community steward and sponsor job

Create a recurring operational responsibility, not a scheduled automation in this planning change:
**Prepare and maintain reference data for L10 {cell}.** A steward owns the work queue temporarily;
geography and publication rights remain open and non-exclusive. Sponsors fund the work without
choosing which geometry is declared correct.

| Stage | Steward work | Completion evidence |
| --- | --- | --- |
| Inventory | Identify jurisdictions, official GIS, geodetic control, public LiDAR, imagery, and record indexes | Source register, coverage, dates, access route, and gaps. |
| Acquire free sources | Download or catalogue reusable assets; retain metadata and hashes | Reproducible acquisition and rights review. |
| Evaluate | Check frames, physical correspondence, age, duplication, and independent errors | Approved roles and reasons for exclusions. |
| Request missing records | Ask the responsible surveyor/recorder/GIS office for specific records and terms | Request text, reference IDs, response, quoted fees, and expected benefit. |
| Fund targeted gaps | Sponsor copying fees, digitisation, field recovery, or independent survey | Agreed budget and licence permitting the intended open product. |
| Publish | Release the package with technical review and a source conflict report | Immutable manifest, QA, attribution, and rollback path. |
| Maintain | Check source revisions, mark destruction/movement, construction, and broken links | Changed-data report and reviewed replacement releases. |

An initial refresh policy can check portal metadata quarterly and before new precision releases,
with immediate review on reported mark movement or major construction. This is a proposed policy,
not a guarantee of current physical conditions. A meaningful invalidation should be visible to
clients and stewards even if no new source file is available.

A request template should seek digital control coordinates and datum/epoch, monument/corner
recovery records, relevant recorded survey or right-of-way sheets, recent measured
surface/orthoimagery data, accuracy reports, and permission to redistribute and derive open
products. Ask for the fee schedule and digital alternatives before paying. Do not request owner
names or other personal parcel data when geometry, stable IDs, and documentary references suffice.

Payment for access or copies does not establish reuse rights. Prefer freely redistributable data,
and fund field measurements or extraction of permissible factual reference information when a
restricted archive cannot enter the open package. Record provenance and review the actual terms; do
not assume all material hosted by a public authority is public domain.

## 8. Correcting for OSM differences

Use OSM as a separate overlay and candidate-reference source. Fit local maps to independent
control first, then measure discrepancies against matching OSM features. If a local OSM
overlay needs a translation/rotation, store that as a versioned display/comparison
transform with its geographic extent, source snapshot, residuals, and uncertainty. Never
quietly move the measured map to hide it.

Do not estimate one offset for an entire L10 from two corners. OSM errors can vary by
contributor, source imagery, feature type, and neighbourhood. Even a block-scale correction
requires spatially distributed support and withheld checks. Reject a global rigid correction when
residuals indicate local shape changes, inconsistent sources, or a mistaken feature
correspondence. Do not warp measured geometry into agreement.

Separate internal offset handling from contributing changes upstream. Changes to OSM should be
reviewable feature-specific proposals with lawful source provenance and the relevant community
process. This plan authorises neither edits nor bulk imports. OSM-derived data must retain
applicable [ODbL attribution and licensing](https://www.openstreetmap.org/copyright); a custom
schema or a separate file does not automatically eliminate derivative-data obligations. Review the
actual publication use, particularly when fitting published coordinates from OSM geometry.

## 9. First experiment and acceptance criteria

Select one intersection and its containing L10. Use the existing trip/stop comparison and the
user's planned reference measurements. Start with SF sources if that location is in San Francisco;
otherwise substitute the responsible county and municipal sources without changing the contract.

1. Find published control and recover its physical condition;
   establish fit points and separate checks.
2. Assemble free control, public LiDAR, building footprints,
   parcel context, and imagery with source dates.
3. Run OSM-only assisted placement, public-GIS-assisted placement, and
   measured-control placement as separate trials.
4. Test erroneous GPS discovery/placement inputs: fixed offsets of 2, 5, 10, and 30 m,
   jumps, correlated drift, underestimated uncertainty, and missing GPS. These are stress
   cases, not an asserted error distribution.
5. Test invalid marks, duplicated sources, old survey geometry, incompatible
   heights/units, and crossings of L10/L13 boundaries.
6. Measure withheld horizontal/vertical error, scene-space error, rejected false matches, operator
   time, download bytes, and preparation cost.

Protection passes when bad GPS cannot alter immutable local geometry or silently move an accepted
release. It must either find the correct candidate region and validated fit or report failure and
retain local-only operation. Low-accuracy context must not receive control-level weight merely
because it comes from an official portal. Withheld checks determine numeric placement eligibility.

The decision after this test is which freely available combination makes operator alignment
reliable, and which remaining error is worth funding through survey work or specific records
acquisition. Store results by site and source profile; do not extrapolate a successful
intersection to an entire city. The reusable deliverable is the L10 package plus a
documented acquisition and validation job.
