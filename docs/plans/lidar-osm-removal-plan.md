# Remove OSM from LiDAR alignment and reference preparation

Remove the OSM download and geometry paths from Gerardus and its LiDAR clients while retaining
city reference data, measured scene geometry and browser maps.

- **Status:** Implemented and locally validated; coordinated release checks remain.
- **Scope:** Gerardus and VelocityVisualiser, including their shared protocol and the web clients that consume Gerardus geometry.
- **Preserved:** Browser map tiles, cycle maps and explicitly requested browser map lookups.

## Outcome

Preparing, updating, regenerating, registering or viewing a LiDAR site must never download,
read, derive, serve, render or snap against OpenStreetMap geometry. The existing browser map
experience remains available. An operator must not need to disable an OSM switch to achieve this.

This is a removal of active OSM data dependencies. Historical source records, experiment reports,
licence evidence and terminology borrowed from a public schema remain truthful engineering records.
They are not an active dependency on OSM data.

## Original trigger

Before this change, Gerardus emitted `osm: downloading L13 ...` in `src/site.rs` during site preparation.
Its builder had an Overpass endpoint enabled by default and fetched the site's L13 cells and their neighbours.
`Place from hint`, reference updates and regeneration can reach this builder. Changing who may
receive the OSM layers, or hiding them in the visualiser, does not stop preparation from fetching
them. Progress is then forwarded to the visualiser.

The current Go packages under `internal/lidar/` contain no OSM downloader or reference integration.
The dependency is in Gerardus and its clients, outside the Go L1–L9 perception stages.

The inventory was checked against Gerardus main at `4cbd776` and velocity.report at `c134b2f3e`,
with the separately proposed alignment-control changes applied. Recheck the inventory before
implementation if either base changes.

## Removal inventory

| Area                             | Remove or change                                                                                                                                                                                                                    | Keep                                                                                                                            |
| -------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| Gerardus preparation             | OSM endpoint configuration, L13 fetch/cache cycle, way merging, OSM line generation, OSM intersection generation, progress stages, source entries and bundle hashing of those inputs in `src/site.rs`                               | 3DEP, DataSF footprints and streets, city intersections, kerbs, SFMTA inventories, trees, registered scans and derived geometry |
| Gerardus commands and deployment | `fetch-osm`, `--osm-endpoint`, `--osm-to-local-peers`, `--street-source osm`, `DEV_OSM_LOCAL`, startup messages and associated deployment defaults in `src/main.rs`, `Makefile`, `tools/dev-server.sh`, `deploy/gerardus-deploy.sh` | Normal server startup, scan-serving controls, admin identity and author/reviewer controls                                       |
| Gerardus services                | OSM layer advertisement, streaming, GeoJSON export and access branches in `src/server.rs`, `src/geojson.rs` and OSM-specific policy members                                                                                         | DataSF city streets/intersections and unrelated authorisation                                                                   |
| Shared geometry                  | Extract neutral helpers from `src/osm.rs`, then delete its downloader, Overpass parser, OSM-specific geometry and the module                                                                                                        | Clipping, densification, walking, site coordinate context and a neutral HTTP user agent required by DataSF modules              |
| Diagnostics                      | OSM building-offset checks, OSM-only examples, OSM intersection lookup/report columns and OSM cache fixtures                                                                                                                        | DataSF-to-3DEP checks and historical reports describing past measurements                                                       |
| Protocol                         | Retire OSM layer values and OSM-specific capabilities/comments in the alignment and map contracts; regenerate clients                                                                                                               | Existing numeric identities of surviving layers and generic line/intersection/building messages                                 |
| Swift session and UI             | OSM requests, summary counts, visibility state, attribution overlay, toggles, regeneration copy and progress explanations                                                                                                           | Aerial and DataSF controls, registration, manual placement, shared radius and refinement                                        |
| Swift rendering                  | OSM vertex arrays, buffers, counters, draw calls and OSM geometry decoding                                                                                                                                                          | DataSF geometry, registered scans, measured layers and their rendering styles                                                   |
| Swift street painter             | OSM road fallback for centreline snapping and fallback ground heights, and related copy                                                                                                                                             | City centrelines, kerbs, footprints, box walls and measured ground                                                              |
| Gerardus-backed web viewer       | OSM/intersection feature-layer requests, controls, styles and consent text that claim Gerardus serves them                                                                                                                          | Browser base maps and city/3DEP/scan geometry                                                                                   |
| Web capture locator              | OSM building/wall outline downloads and their Snap/Check fit path                                                                                                                                                                   | Manual placement and optional browser maps/address search; keep reusable geometry algorithms only where still used              |

### Extract shared helpers before deleting `osm.rs`

`footprints.rs`, `streets.rs`, `kerbs.rs`, `lane_markings.rs`, `street_furniture.rs` and `trees.rs`
use `osm::SiteContext`; several also import `clip`, `densify` or `walk`. DataSF requests reuse
`osm::USER_AGENT`. Move those utilities to a neutral geometry/context module and neutral network
constant first. Preserve their coordinate and ground-draping behaviour and move their tests.
Deleting the whole module without this extraction would break the city geometry being retained.

### Remove the street painter's OSM fallback

`StreetPainterSession.swift` currently chooses OSM road lines when `cityStreets` is empty. Those
lines also provide fallback heights. Remove both paths. Without city centrelines the painter
must say that they are unavailable, continue using any available kerbs/walls/measured ground,
and retain manual placement. It must not substitute another external geometry source.

### Browser map exception

Keep `public_html/src/js/scene-map-mode.js`, its CyclOSM tiles and attribution. Keep the ordinary
web map/tile controls, map links and their explicit external-request consent. Browser-only
Overpass requests used to draw a map can remain.

The locator's Overpass outlines currently feed LiDAR Snap and Check fit, so this plan removes
that alignment use even though it runs in a browser. Split the map/address functions from the
outline-to-alignment functions in `web/src/lib/align/osm.ts`; preserve consent for the map uses.
The Gerardus site viewer should stop offering OSM vector layers once the service retires them,
while its browser base map continues working.

### Retain neutral schemas and provenance checks

Roof tags such as `roof:shape` and Simple 3D Buildings field names are schema vocabulary used for
geometry inferred from 3DEP and DataSF. Keep the vocabulary and its meaning; do not delete roofs
because a comment names OSM's vocabulary. Retain WGS84/geodetic conversion needed by other inputs
and web maps.

Keep the checks that reject ODbL/OSM-derived data from incompatible derived products and imported
changesets. Removing the downloader is not evidence that every future imported dataset is clean.
Keep caller identity, admin roles, review history and other policy that also serves non-OSM work.
Only remove live OSM-serving capability, including an obsolete `osm_allowed` response field if its
protocol value can be retired safely.

## Existing bundles and protocol compatibility

1. Reserve retired protobuf enum/field numbers and names rather than renumbering or reusing them.
   Regenerate both repositories' clients from the matching contracts. Older clients that explicitly
   request an OSM layer receive a clear unsupported-layer error.
2. Filter legacy OSM layers from advertisement, default streaming and HTTP site-layer discovery
   immediately. An old bundle containing `osm.pb` or `intersections.pb` must not make those layers
   available after the upgrade, including when a client requests every layer.
3. Version the prepared-bundle contract or add an explicit stale-bundle check. Rebuild active bundles
   without OSM inputs and source entries on preparation, using retained city/aerial caches where
   possible. Give rebuilt bundles their correct content fingerprints.
4. Keep accepted registration records and their original source/bundle provenance intact. Rebuilding
   reference bundles does not justify rewriting recorded poses, replaying every capture, or altering
   past experiment findings.
5. Do not delete existing OSM caches, old bundle files or database/history rows in this change. They
   become unused evidence. Any later storage cleanup needs a separate, reviewed retention scope.

## Implementation sequence

1. Land the existing alignment fixes independently: Gerardus capability/radius/manual-placement
   support, then the visualiser controls that use it.
2. In Gerardus, extract neutral helpers and migrate DataSF callers without changing their outputs.
   Add checks proving the retained layers still prepare correctly.
3. Remove OSM preparation, commands, deployment options, service layers and diagnostics. Add the
   stale-bundle and unsupported-layer behaviour before updating clients.
4. Retire protocol values and regenerate both repositories' stubs. Remove Swift requests, UI,
   rendering and street-painter fallback. Remove Gerardus-dependent OSM web layers and the web
   locator's OSM alignment path while retaining its map functions.
5. Update operator documentation, component READMEs, active plans and development logs. Preserve
   historical ADRs and reports; mark superseded active guidance instead of erasing its history.
6. Review and merge the service/client changes together, then deploy Gerardus and relaunch the
   updated visualiser. Check a cold site, a cached site and an old bundle before normal use.

The source changes are on `codex/remove-osm-lidar` in both repositories, based on the merged
alignment fixes (`5ccde291d` in velocity.report and `c87c461` in Gerardus). Live deployment and
stored-data cleanup are separate release operations.

## Implementation record

- Extracted `src/site_geometry.rs` and the shared dataset user agent before deleting `src/osm.rs`.
- Removed preparation, commands, deployment flags, access branches, HTTP/gRPC layers, diagnostics,
  Swift controls/buffers and road fallbacks. Retained city kerbs, marking records and observed paint.
- Reserved alignment layer identities 2 and 4 and map player field 7. Legacy-layer requests fail
  before streaming; old advertised metadata is filtered and stale manifests rebuild to new generations.
- Kept historical generations and accepted records, with a regression checking their original
  bundle/source evidence survives a rebuild.
- Removed the web locator's outline fitter and its unused ICP module. Browser maps, address search,
  map consent and the public cycle-map implementation remain.
- Added parser/tool checks to Gerardus CI and coverage for retired CLI flags, legacy manifests,
  HTTP/1.1 errors, ignored old responses and absence of OSM road/height snap targets.

The following validation section separates the acceptance checklist from observed local results.

## Validation and acceptance

- A failing mock Overpass endpoint records zero requests during cold preparation, cached preparation,
  update, regeneration and registration. No OSM progress stage is emitted, including for old bundles.
- New bundles contain no OSM layers, files, source entries or OSM contribution to their fingerprints.
  Old bundles cannot advertise or stream those layers over gRPC or HTTP, even with an empty layer list.
- DataSF footprints, city streets/intersections, kerbs and retained inventories still build; coordinate
  transforms, clipping and ground draping retain their established test results.
- Aerial registration, manual Snap acceptance, the 1–30 m radius and site refinement remain covered.
  Neither registration nor painter fallback reads an OSM geometry file.
- Swift tests cover absent city streets, legacy OSM responses and toggles/buffers no longer existing.
  Check the pane interactively to confirm its movement controls remain stationary.
- Web tests prove map consent and cycle tiles still work, while OSM outlines cannot enter LiDAR fit
  or snap operations and Gerardus OSM vector layers are not requested.
- Scan production paths for retired endpoints, flags, layer requests and fallback reads; allow only
  explicit compatibility handling, historical evidence, provenance rejection and preserved browser maps.
- Run Gerardus tests and lint, `make test-mac`, focused web tests/build, documentation checks and
  `make check-no-results`. Runtime verification is separate from passing synthetic/local tests.

## Tradeoffs for review

Outside DataSF's coverage, removing OSM leaves fewer reference overlays and no replacement road
snapping. Aerial/scan geometry and manual placement remain available where their data exists. The
plan does not add another road provider.

The web locator loses its OSM Snap/Check fit path. Its manual placement and map remain. A separate
future integration could expose Gerardus's retained geometry there; that is not required to remove
OSM and is outside this plan.

This is more than suppressing one progress line: it removes the download dependency, derived OSM
layers, their transport and rendering, and the paths that can feed OSM into alignment. It does not
claim a change in measured accuracy without subsequent validation on representative captures.

## Local validation results

- Gerardus: `cargo test --locked --no-fail-fast -- --test-threads=2` passed all 294 Rust tests;
  `make test-tools` passed both Python tests. `cargo fmt --check` and Clippy across all targets
  passed; Clippy reports an existing `manual_is_multiple_of` suggestion in `aerial_outlines.rs`.
- macOS: `make test-mac` passed 798 XCTest cases and 1,692 Swift Testing cases. These cover the
  existing bounded/manual-placement controls and the new legacy-layer and painter regressions.
- Web: all 852 tests in 43 suites passed; Svelte reported zero errors and warnings. Changed-file
  ESLint/Prettier checks and `make build-web` passed. The public site's 262 tests passed, including
  its preserved map behaviour.
- Formatting, geometry wording, whitespace and `make check-no-results` guards passed.
- Repository-wide `make lint` stops on existing Go formatting violations inside the separate,
  untracked `dd/ui-vis` checkout. `make test` stops in Go because this sandbox cannot access parts
  of the Go cache or bind test sockets. No Go production code changed in this removal.

These are synthetic/local checks, not deployment or physical-scene validation. Gerardus and the
visualiser still need their coordinated release, a cold/cached/legacy-site check against the
running service, and an interactive pane check. No deployed service, recorded placement, cache
archive or historical bundle was deleted or rewritten by this implementation.
