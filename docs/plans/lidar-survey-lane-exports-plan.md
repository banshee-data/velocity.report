# Survey lane polylines for web scenes and game exports

Carry the travel lanes reviewed in VelocityVisualiser and saved in Gerardus into web scenes and
game exports as directed road-segment polylines. The survey extent must include every trail in
the selected window, and the associated saved lanes must retain their complete geometry. Both
consumers should read the same versioned lane geometry and distinguish measured support from gaps.

- **Status:** Parked: execution waits for the full survey evidence sources and the `/sf/` lane adapter contract; revisit when those are available
- **Decisions:** D-32 (planning limits), D-33 (hub docs state what ships), D-34 (scene assets move out of the repository), D-36 (Gerardus owns static street geometry and registration), D-38 (measured spatial data), D-39 (lane provenance and storage); Gerardus [ADR-0011](https://github.com/banshee-data/gerardus/blob/main/docs/adr/0011-publish-snapshots-and-aggregates-not-trails.md) (aggregate-only boundary) and [ADR-0016](https://github.com/banshee-data/gerardus/blob/main/docs/adr/0016-lane-fields-proposals-and-limits.md) (lane proposals and authored lanes)
- **Layers:** Gerardus lane and road data; VelocityVisualiser; offline export tooling; public scene player; game export adapter
- **Target:** Unscheduled; the owner places the work in a release slot
- **Companion plans:** [lane review across the city](lidar-lane-review-citywide-plan.md), [lane snapshot and attribution](lidar-lane-snapshot-and-attribution-plan.md), [web scene export](lidar-web-scene-export-plan.md), [city view](lidar-city-view-plan.md), [SF layered roads](ui-sf-layered-roads-plan.md); Gerardus [lane proposals](https://github.com/banshee-data/gerardus/blob/main/docs/plans/LANE-PROPOSALS.md), [road surface](https://github.com/banshee-data/gerardus/blob/main/docs/plans/ROAD-SURFACE.md) and [road client interface](https://github.com/banshee-data/gerardus/blob/main/docs/INTERFACE-ROAD-CLIENT.md)
- **Canonical:** [vector scene map](../lidar/architecture/vector-scene-map.md)

## Motivation

An operator can settle lanes in the visualiser, yet those decisions are not consistently visible
in the public scene or available as a road-segment lane layer for a game. A street centreline is
not a lane centreline. The exports need the same directed lines and the full extent of the
trails included in the survey, with provenance and coverage carried alongside them.

Here **settled** means an operator-authored `travel_lane` saved in Gerardus, whether drawn by hand
or adopted from a proposal. A proposal's `high` grade alone is not an approval. The owner defines a
**survey bound** as the extent of object trails observed in a particular survey window. The
sensor's range is the ideal outer extent, but objects are not necessarily observed across all of
it. The owner subsequently specified that the bound must include all included trails, with no
trimming to an arbitrary radius or percentile. This is a coverage term, not a claim that the lane
geometry is a boundary survey.

## Current state

| Component      | What exists                                                                                                                                                                                                                                          | Gap for this work                                                                                                                                                                                                                                                                                       |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Lane review    | `StreetPainterSession.swift` saves directed `travel_lane` marks to Gerardus; proposals are separate and may be rejected or adopted. Gerardus `street_use::list` serves the latest non-removed authored rows per site, including feature and row IDs. | A site-scoped mark is not yet a citywide, segment-scoped lane. The [citywide review plan](lidar-lane-review-citywide-plan.md) proposes that change; it is not a prerequisite for an initial bounded export.                                                                                             |
| Local snapshot | `internal/lidar/storage/sqlite/lane_snapshot_store.go` keeps append-only copies of saved lanes and their pose; `internal/lidar/lanegeom` provides projection and membership.                                                                         | Reporting snapshots are a cache, not a replacement for Gerardus's authoritative authored rows. Selecting historical geometry and checking pose agreement require explicit rules.                                                                                                                        |
| Web export     | `tools/s2-archive/scene-vectors.py` requests `street_use`, retains travel lanes and their direction and limit tags, and transforms site coordinates into the sensor frame using an accepted pose. Its tests cover lane filtering.                    | `scene_features` retains a whole feature if any vertex is inside the radius: it neither clips to coverage nor catches a crossing whose endpoints are both outside. It omits row and proposal IDs, and `public_html/src/js/scene-vectors.js` has no `street_use` style, so those features are not drawn. |
| Game data      | The SF driving view already loads placed survey replays and can read Gerardus's Bay Bridge road overlay. Its game coordinates use EPSG:7131 and NAVD88 heights.                                                                                      | The game has no lane-fragment input for survey geometry. The proposed adapter needs an asset contract and explicit road-surface association.                                                                                                                                                            |
| Coverage       | Lane fields are sparse aggregates of at least five tracks per cell; accepted registration places them. A scene export radius and an S2 site bundle are retrieval/display bounds.                                                                     | No existing export establishes the lane-specific bound of object trails within the chosen survey window. Absence of a field cell does not prove a lane is absent.                                                                                                                                       |

## Proposed design

### One frozen lane product, two adapters

Gerardus remains the authority for saved marks and their revisions. Build a deterministic,
versioned lane product from a pinned set of authored rows, the corresponding road segments, and
the survey-bound evidence. The offline build may be a Gerardus export or a shared export
stage consuming Gerardus; choose its exact entry point with the game contract. It must not implement
independent lane fitting in the web and game clients. VelocityVisualiser continues to author and
review through its existing interface; no unsaved edit becomes exportable.

The logical product is a collection of **directed centreline fragments**, not paint markings or
lane polygons. Each fragment represents a lossless part of a saved lane over one road segment; the fragments
reassemble to the full selected saved lane, including portions outside the trail-supported extent. Preserve vertex order in the direction of travel, lane type and known width;
distinguish unknown width from zero. The `lane_markings` layer is separate evidence about paint,
not a substitute for travel lanes. Intersections, merges, turns and connectivity are not inferred
from proximity.

Each product manifest should contain a format version, product/content digest, generation version,
coordinate frame and units, acquisition window or windows, coverage source and its revision,
road data epoch, selected lane snapshot or exact lane row revisions, and registration record IDs
where relevant. Each fragment carries a stable parent lane feature ID, lane row revision, fragment
ID, segment ID and namespace (CNN where available), direction via vertex order, type, width if
known, evidence/review status and source references. Carry a proposal grade only as its original
grade, not as positional uncertainty or proof of accuracy. Retain signed speed-limit tags and
effective dates where permitted by the existing export policy; do not imply that a limit date is
the effective date of the geometry. Public adapters use an allow-list and omit author names, raw
fields, per-track data and private capture details.

Fragment IDs should be deterministic from the pinned inputs, parent lane, segment and directed
interval. A changed row or coverage version can change fragment IDs; the parent ID and row revision
are the lineage. Sort output, specify numeric precision and gzip metadata, and keep generated-at
time outside the content digest. A repeat build from identical frozen inputs must give identical
geometry and IDs. Existing lane IDs from different site bundles must not be assumed equal merely
because their lines overlap.

### Coverage, segmentation and coordinates

1. Read the complete set of object trails included in the selected survey window in the sensor
   frame, including distant, low-frequency and single-track evidence. Preserve their sample support
   state and retain unknown or predicted points in the input inventory, while keeping them distinct
   from positively observed evidence. No radius, percentile, class, count, five-second playback
   history or display fade may trim the input. The local bound contains every valid included sample
   and every included continuous trail segment; missing or invalid input is an explicit failure, not
   an empty substitute. A conservative search envelope may cover unobserved space; it is not proof
   that its interior was observed.
2. Freeze a consistent source view: site and sensor, accepted registration record and transform,
   lane feature and row revisions, road segment epoch, survey interval, selected input identities
   and their digests. Reconcile input frames, track IDs, samples and unmatched items against output
   accounting. A scene asset may have omitted source frames, so record that limitation and build
   from the complete local recording or observation store where available. Do not discard a saved
   lane because its survey-time validity is unknown: retain the geometry and explicitly label its
   edit time and the missing temporal evidence rather than asserting it existed at capture time.
3. Associate selected saved lanes with road segments using authoritative references where available;
   otherwise match geometry and direction against active segment centrelines with explicit, tested
   tolerances. Splitting at segment or attribute boundaries must be lossless: the complete original
   directed lane can be reconstructed from the pieces, including the ends. Preserve unassigned and
   ambiguous saved geometry with an explicit reason for review; never silently drop or force it onto
   a crossing road, deck, opposite direction or adjacent site.
4. Keep observation support as a separate layer of context on the full lane geometry. Do not cut a
   trail, omit a lane end, fill an occlusion, bridge a missing interval or infer a connection from
   proximity. Unknown or predicted support is labelled, not discarded. If a source or placement is
   missing, preserve what was read and report it as unplaced or unavailable; do not manufacture a
   position. Resolve ambiguous associations before presenting them as verified.
5. Work in a stated metric site frame with sufficient precision; transform to the scene sensor
   frame with `p_sensor = R^T (p_site - t)` using that scene's accepted record. Use the game's
   declared frame and vertical datum for its adapter. Preserve valid elevations; if the source is
   2D, drape only against a versioned supported ground surface, or carry unknown heights explicitly.
   Never substitute zero height or extrapolate missing ground as measured. Keep a lossless source
   representation, and record any adapter precision and transform.

### Full trail inventory and export bounds

Build the bound on the local device from the complete pinned survey window, without a maximum
radius or a minimum number of tracks. Every included trail contributes, even if it is the only
trail at a distance. An unknown support token or class does not cause an input trail to disappear;
it affects how the evidence is described. The raw samples and IDs remain local under D-39 and
Gerardus ADR-0011. Do not apply a five-track station threshold to decide which local trails to
keep. Aggregates that leave the device still obey the existing k = 5 rule; an aggregate that cannot
be published must not be used as a reason to trim an approved lane's geometry.

For a road-segment association, match the full trail set to settled lanes with explicit tolerances
and record unmatched or ambiguous trails. Store support intervals and gaps separately from the
complete lane lines. A short interpolated step between observed samples may be labelled interpolated
under the existing at-most-0.5-second lane-field rule, but this does not delete endpoints or turn a
longer gap into a continuous observation. A larger search envelope or a single distant sample does
not make the unobserved area inside it covered.

The [scene inventory](../lidar/operations/survey-trail-inventory-2026-11.md) scans every indexed
track sample in the 25 available scene assets without a radius or class filter: 104,314 part-local
track IDs, including 2,871 with a sample beyond 100 m and a farthest anchor of 210.64 m. These
assets use stride 2 and do not record observed versus predicted support, so the inventory is a
complete read of those assets, not of the original surveys. The earlier 60/70/80/100 m comparison
is historical; none of those values is a proposed cutoff.

The visualiser, web scene and game export must keep the entire selected survey extent and the full
associated lane geometry. Remove the lane export's current 90 m vertex-selection assumption and
ensure the public player's 150–170 m fade, five-second history and 2,000-point buffer do not hide
or silently shorten trails or lane lines in the full-survey view. Retain genuine temporal gaps;
use streaming or batching rather than truncation when the full view is large. Any optional shorter
view is explicitly chosen and does not alter the stored export.

For web playback, “full” means every sample already included under the existing scene-export
policy. Reading the original survey locally to find its full extent does not authorise adding raw
track observations or identities to public or game exports. Reconcile local input completeness
separately from the public asset's permitted content and label any difference.

### Capturing lanes headlessly across the corpus

The corpus driver runs per survey window and sensor placement:

1. Resolve the complete capture or VRLOG, its window, the correct accepted Gerardus record and
   the saved lane revisions. Record source digests, run configuration, build, pose record and
   coordinate frame. A missing recording or an ambiguous pose is an explicit skipped or failed
   case. For the overnight campaign, replay every pinned survey window from capture. Resume from
   an already validated recording only if it was produced during this same campaign.
2. Inventory every included local trail without distance, percentile or track-count trimming to
   establish the survey extent. Keep raw tracks locally. In parallel, the existing headless lane
   job builds the whole-window and two half-window lane fields, uploads their k-thresholded
   aggregates against the accepted pose, and asks Gerardus to derive proposals. Use
   `make lane-fields FORCE=1 DRY=1` to check which clips the existing batch considers eligible.
   Check that list against the manifest before queueing. The lane field's selection and k = 5
   rule apply to the aggregate. The full trail inventory must read the recording separately.
3. An operator may approve, edit, split or draw lanes in VelocityVisualiser and save them to
   Gerardus. A generated proposal is available headlessly, but it is not an approved lane merely
   because its grade is high. Read the saved `travel_lane` rows headlessly from
   `GET /api/v1/sites/{l16}/geojson?layers=street_use&frame=site` and freeze the feature IDs,
   row IDs, vertices, direction and relevant tags with the record above. `velocity lidar lanes
pull` can additionally create velocity.report's reporting snapshot; today it requires the
   accepted record ID and pose to be supplied explicitly.
4. Build one lane product from those frozen rows, preserving every selected lane's full line and
   recording any unmatched segment association. Use it to refresh the scene vectors and the game
   adapter without replaying the original capture. A missing saved lane, an unreviewed proposal,
   or an unavailable Gerardus instance has its own status; none is silently labelled no lane.

The existing scene publisher also requests lane proposals after replay, but its current lane step
may be skipped without failing the scene. The new corpus lane-product job must report that status
explicitly and must not declare lane completeness from a successful scene publish alone.

### Headless validation and per-site outcomes

Run the following checks for every site and window, with a machine-readable summary and retained
input provenance. This validator and the shared product builder are work to build:

- **Completeness:** reconcile source frames, samples and selected saved feature IDs; every valid
  included trail endpoint remains inside the local extent and every saved lane reassembles from
  its road-segment fragments. Check missing/duplicate IDs, geometry with fewer than two finite
  points, gaps, uncertain associations and lanes crossing a former 90 m boundary. Do not turn a
  distance or a lack of repeated support into a cut.
- **Placement and parity:** verify the chosen sensor, survey window and accepted pose; test a
  site-to-sensor-to-site round trip within the declared adapter precision. Compare the web and
  game outputs by parent lane ID, revision, direction and reconstructed geometry after transforming
  them back to a shared frame. Check old readers and sites with no lane layer.
- **Agreement with trails:** on the device, compare the full lane geometry to eligible observations
  per lane and direction, reporting covered length, gaps, offsets and unmatched or opposed movement.
  Keep unknown, predicted and observed support separate. These are diagnostic measurements: a
  quiet or unmatched stretch stays in the export with its support status, not cut from the line.
- **Independent accuracy:** where a separately drawn reference exists, run `velocity lidar lanes
score --site L16 --drawn-author NAME --json`. The existing lane0002 rule checks high-proposal
  lateral RMSE under 0.30 m, correct per-direction counts, at least 80% of drawn lanes found over
  at least 70% of their length, and no false high proposals. Those are proposal-versus-reference
  checks, not proof that an approved export is physically correct. The reference must be drawn
  without seeing the proposals; the real lane0002 check has not yet run.

Keep structural validity, trail agreement, independent accuracy and operator review as separate
statuses. A site with no independent reference is `not_validated`, not a pass; an absent or empty
lane product is not a passed lane-accuracy test. The scorer currently returns exit code zero after
printing a score even when its `passed` field is false, so the corpus wrapper must parse the JSON,
verify that a reference exists and use the field and clause results for its own outcome. Report
missing evidence, failures and ambiguous matches per site; never auto-approve lanes from the score.

### Representation for existing and new lanes

The shared logical representation is one directed `LineString` per lane or lossless
road-segment fragment. It carries a fragment ID, parent Gerardus feature and row IDs, a segment
ID when known, lane type and width when known, and evidence/review and temporal status. Vertex
order gives direction. The product manifest names its frame, units, capture window, accepted pose,
source revisions and precision. Reassemble fragments to the complete parent line; keep an
unmatched segment ID unknown with a reason rather than inventing one. The web adapter writes the
features in the existing `vectors/vectors.geojson.gz` collection in the sensor frame; the game
adapter reads the same lane product in its agreed world frame. Coordinates in that local GeoJSON
are metres, not WGS 84 longitude and latitude.

For illustration only, one proposed fragment in the web sensor frame could be:

```json
{
  "type": "Feature",
  "id": "fragment:example-1",
  "geometry": {
    "type": "LineString",
    "coordinates": [
      [2.0, -3.0, -1.2],
      [28.0, -3.0, -1.1]
    ]
  },
  "properties": {
    "layer": "street_use",
    "kind": "travel_lane",
    "parent_lane_id": "lane:example",
    "source_row_id": "row:example",
    "road_segment_id": "city:example",
    "lane_type": "general",
    "width_m": 3.2,
    "review_state": "authored",
    "coverage_status": "unknown"
  }
}
```

The property names above are a proposed contract, not a shipped schema. The existing scene vectors
already retain some travel-lane lines, but omit row IDs, use a 90 m vertex-based selection and the
player has no style for the layer. In this checkout, 24 scenes have vector sidecars and only
Ashbury at Downey's sidecar currently contains travel-lane features: 11 features, not necessarily
11 distinct physical lanes. This is an inventory of existing exported files, not a claim that the
other sites have no saved lanes in Gerardus. Refresh the saved rows from Gerardus for each site;
for sites with no saved lane yet, show that the lane layer is unavailable or awaiting review.

Across overlapping site bundles, key automatic deduplication on the authoritative lane revision.
Distinct saved rows that appear to describe the same lane go through the review below. Use the
segment identities from citywide lane review when available and retain the lineage.

### Ashbury pilot and corpus review in VelocityVisualiser

The checked-in Ashbury at Downey vector sidecar contains 11 authored travel-lane features and
five distinct directed polylines. Grouping by identical exported XYZ vertices in travel order
and by identical exported properties gives the following review candidates. Lengths are measured
along the exported 3D lines.

| Street and direction | Line length | Identical copies | Redundant copies |
| -------------------- | ----------: | ---------------: | ---------------: |
| Ashbury northeast    |     65.05 m |                4 |                3 |
| Ashbury southwest    |     57.93 m |                2 |                1 |
| Ashbury southwest    |     23.31 m |                3 |                2 |

Two other lines each occur once, a 34.25 m northeast Ashbury line and a 21.98 m southbound Downey
line. Keeping one line per identical group in this asset preserves all 202.53 m of distinct line
geometry, including both singleton lines. The five distinct lines are the starting set for review.
The sidecar rounds to
centimetres and omits source row IDs and some tags, so confirm the groups against live Gerardus
before editing. Check partial overlaps independently, especially through the southwest bend.

1. Open Ashbury at Downey (`808580aab`) in **City → Lanes to review** or open its accepted capture
   in **Alignment → Paint streets**. Check the Gerardus address and registration record. Overlay
   the placed background, **Traffic → The site**, saved marks, proposals and traced paint as
   available. Find coincident marks in the **Marks** list by feature ID, since a canvas hit cannot
   reliably distinguish copies. Compare the underlying row revisions, segment, direction, type,
   width, limit history and proposal adoption. Choose one survivor from each confirmed group,
   preferring the canonical ID with useful provenance or downstream references. Preserve any
   distinct attributes or unmatched ends before removing a redundant mark.
2. Save the confirmed removals in one street-use edit, recording the surviving and removed IDs
   and revision heads in a local review receipt. Refresh the live marks and proposals to confirm
   adoption, then update the reporting snapshot with **Send lanes to velocity.report**. Rebuild
   the staged scene vectors from the saved rows and verify that duplicate features disappear
   while the distinct lines and their endpoints survive. Gerardus can restore a historical row
   through a compensating edit if the review needs correcting.
3. Use the same review at every site. Review each road approach and direction, including turn,
   bike or bus lanes where supported. Prefer a saved lane that covers the observed approach with
   the right alignment and direction. For partial overlaps, compare both ends on the background
   and retain every supported extension. Keep distinct parallel lanes, opposite directions, grade
   separations and changes in lane attributes. Draw or extend a lane where the trails and reference
   support it and the proposals leave a gap. Record occluded or ambiguous portions for follow-up.

Implement a read-only duplicate and gap report before the overnight run. It should group exact
duplicates by ordered geometry, site and compatible lane semantics, and report near or partial
overlaps using same-direction tangent, lateral separation and overlap in both directions. Near
matches go to review. The report should surface each candidate by feature and revision ID with
enough detail to select it in the app and explain the recommended survivor. Check Ashbury first,
then a site with parallel lanes and a junction. Fixtures should cover duplicate rows, close parallel
lanes, reversed directions, contained lines with unmatched ends, a change in limits, and an
overlapping site bundle. Avoid carrying duplicate authoring forward by checking new proposals
against saved lanes and the current draft before approval. Gerardus's existing proposal adoption
check can provide candidate matches, while the reviewer decides whether a line adds coverage.

### Tonight's first deliverable and the overnight run

**Start with Ashbury at Downey.** Freshly replay its full pinned survey window headlessly before
any bulk queue. In the local review folder deliver the full directed lane lines in Gerardus site
metres, an overhead preview over the placed reference, and a manifest naming the capture window,
build, tuning, accepted pose, field and lane revisions. Show the saved marks and newly fitted
proposals separately in the preview. Include the duplicate groups, likely survivor IDs, uncovered
approaches and gaps. Preserve the ends of every distinct line. Open the result in
VelocityVisualiser and compare it with the Ashbury baseline and live saved marks. Record the
reviewer's decisions when available. This is tonight's first deliverable, before the other sites
run. A successful field upload or a lane count alone does not deliver the lines.

**Compare headless with app-driven replay on three windows.** Use Ashbury–Downey,
Columbus–Broadway and Embarcadero–Bay as the initial sample. Run a headless pass and a paced 1×
server replay viewed in VelocityVisualiser from each identical capture window. This interprets
“3x” as three paired windows. Start both arms with a clean pipeline and match ordered capture
hashes, processing and scoring bounds, warm-up and background policy, sensor, build, tuning,
accepted pose and Gerardus inputs. Capture the app-driven arm's server recording at full cadence.
Viewer delivery and frame counts are useful diagnostics for the app. Use the server recording
for the data comparison. Keep the comparison arms in isolated Gerardus stores seeded from the
same snapshot so a newer upload from one arm cannot change the other's proposed lanes.

Compare every recorded rotation and timestamp, trail samples and actual support, all three lane
field parts, and the directed proposal geometries. Canonicalise only run-specific IDs, output
ordering and serialization at a declared coordinate precision. Retain direction, lane types,
confidence grades, endpoints and gaps in the comparison. Mark 100% agreement only when all
three pairs have identical frame accounting, normalized field contents and complete matching
lines. Report the first divergent frame, cell or vertex and its cause on a mismatch. If the
paced arm cannot keep up or its setup differs, mark that pair inconclusive and report the
confounder. Use the verified headless path for staging and keep cross-mode parity open until
the three pairs pass. A mismatch in the headless completeness checks blocks that site's output.

**Preflight the whole corpus.** Start from the 24 indexed S2 surveys and account for any additional
legacy scenes. SoMa 1 is outside that index and needs its own capture and accepted pose resolved
before it can produce a placed lane result. Record its status explicitly in the morning ledger.
Freeze the expected site/window list before submitting jobs.
Record ordered capture paths and digests, interval, sensor, pose revision, map/lane revisions,
binary stamp, tuning fingerprint and output location. Use `make lane-fields FORCE=1 DRY=1`
on the replay host and reconcile its eligible, active and skipped clips against that list.
Pin the accepted record for any ambiguous clip. Use pilot wall time and disk use to size
concurrency and estimate the completion time. If the estimate misses the morning target,
state which windows lack a viable slot before starting the batch.

**Replay every pinned window overnight, then derive the lines.** Queue a new headless replay
per window, including clips with current proposals. Retain that night's VRLOG locally until the
trail inventory, the whole and half-window fields, and the directed lane output are validated.
The current headless lane job cleans up its temporary VRLOG on exit. Extend it to retain the
evidence for this campaign or use a retained headless recording followed by field derivation.
Checkpoint the replay and geometry steps independently so an export retry can reuse a validated
VRLOG from this campaign. Freeze the field set for each derivation, including the contributing
windows if several belong to one site. Generate the full local survey extent without a range,
percentile, or minimum-count cutoff. Reconcile line IDs, lane metres by approach and direction,
endpoints and gaps, with duplicate geometry counted once. Preserve uncovered portions of selected
saved lines and account for any unplaced, unsupported or ambiguous result.

**By morning deliver the full set of directed lines for following each lane.** Write one reviewable
set per expected site/window with the settled lines, clearly labelled candidate lines awaiting
selection, and a manifest that links to their replay and validation evidence. Record the count of
expected, replayed and staged windows and every failure or blocker. A complete run has a fresh
validated replay and usable line output for every expected window. Keep the human review state
explicit since the batch can deliver proposals while selection and saves in VelocityVisualiser
are still pending. Both export adapters consume the same selected lane geometry after review.

### Consumer contracts

- **Web scenes:** extend `scene-vectors.py` to read the frozen product, filter to the scene's
  survey and transform the complete lane fragments, with a format/version update and provenance. Add a
  distinct `street_use` travel-lane style in `public_html/src/js/scene-vectors.js`; render only
  the full centreline, with a small ground-aware lift if needed. Existing scenes without lane
  data continue to play; state whether lanes are unavailable or coverage is unknown. Update the
  survey provenance copy and the shared scene-player tests. Do not claim that a missing line
  means no lane exists.
- **Game:** target the [SF driving view](../ui/sf-driving-view.md#survey-traffic-replay) with a
  versioned lane sidecar derived from the same source fragments and segment IDs as the web
  vectors. Use the game's EPSG:7131 frame and NAVD88 heights. Reuse its accepted-pose placement
  contract and verify both adapters against the original lane line. Define how the sidecar pins
  the city tile epoch and, where available, the Gerardus road surface revision and level. Keep
  lane geometry independent of mesh simplification. Review joins and traffic rules separately
  before using the lines as route connections. Keep the game packaging out of the critical path
  for Ashbury's local review and the overnight evidence batch.
- **Publication:** follow D-34 and D-39: real lane data stays in Gerardus, the device or the
  external dataset, with the existing published scene-assets exception until migration. Check
  source and redistribution terms, including the game's OSM-free policy where applicable. Test
  fixtures committed to either repository use synthetic geometry. Report coverage and geometry
  as measured or inferred at the stated time, with uncertainty when known.

## Work and acceptance

| Step | Work and owner                                                                                                                                     | Done when                                                                                                                                                                                                                                                                                                            |
| ---- | -------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 0    | Inventory every included survey trail and define the SF lane sidecar format. velocity.report + Gerardus.                                           | Input counts and digests reconcile, no distance or count filter is applied, and missing original data is named. The SF lane sidecar interface and licence allow-list are identified. The checked-in scene inventory is complete for its 25 exported parts; original full-resolution survey input remains to be read. |
| 1    | Define the shared lane-fragment schema and build the complete extent and lossless segment association. Gerardus or the agreed shared export stage. | Every valid included trail sample lies inside the local extent. Full saved lanes reassemble from fragments; unassigned and ambiguous items remain accounted for. Identical pinned inputs reproduce the same content and IDs.                                                                                         |
| 2    | Add the game adapter and a consumer fixture. Game export owner.                                                                                    | The game receives the whole lane product in its own frame, preserves all fragments and IDs and records coordinate or precision loss. No connections or lane counts are invented.                                                                                                                                     |
| 3    | Connect the web export and full-survey view. velocity.report.                                                                                      | A scene displays the same complete lanes as the game and can show the complete included trails, without a hidden range fade, short history or fixed point cap; real gaps stay separate and legacy scenes still load.                                                                                                 |
| 4    | Validate on real surveys, including long-range and overlapping/junction cases. Both repositories and the operator.                                 | Compare input and exported lane geometry, sample inclusion and pose placement at matched revisions. Report every unavailable, ambiguous or unplaced input and investigate unexplained loss, duplicated pieces or reversed direction before release.                                                                  |

Steps 2 and 3 depend on the shared contract and fixture in step 1; either can proceed first. Step 4
requires actual reviewed evidence and is not satisfied by synthetic tests. Broader citywide lane
identity and automatic cross-site merging remain with the citywide review plan.

## Verification

- Geometry fixtures: straight and curved lanes, opposite directions, several lanes on a segment,
  full lane preservation across a survey boundary, holes and disjoint intervals, segment and tile
  seams, lossless attribute splits, repeated vertices and invalid coordinates.
- Support fixtures: observed and coasted samples, unknown support and class, single-track and
  distant trails including beyond 100 m and 170 m, duplicate track IDs, genuine gaps and missing
  windows. Verify all input endpoints are accounted for and no percentile, range, count or player
  history limit removes them; check the publication rule separately from local inclusion.
- Reconciliation fixtures: overlapping site bundles, conflicting rows, crossing roads at different
  elevations, adjacent and reversed lanes, ambiguous matches, and missing or stale source revisions.
- Time and placement fixtures: distinct capture and edit dates, a limit change without geometry
  evidence, missing or mismatched registration, rotated/translated and tilted poses, nonzero
  elevation and missing ground. Round-trip transformations and compare both adapters within their
  declared rounding tolerance.
- Contract fixtures: empty and unknown coverage, no lanes, rejected and unapproved proposals,
  a saved hand-drawn lane, old schema versions, deterministic rebuilds, partial output failure,
  allowed properties and absent private fields. Confirm game and web share parent IDs and covered
  intervals, not just similar-looking pictures.
- Run the relevant Gerardus, Python export and public-player suites in the implementation changes,
  plus the repository's documentation and publication checks. Record any unavailable real-site
  evidence and do not mark its acceptance gate as passed.

## Risks and open questions

| Question or risk                                                                        | Proposed handling                                                                                                                                                                                  |
| --------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Can every original trail be read?                                                       | Inventory all included inputs and compare counts; scene exports are already stride 2 and do not record observation support. Read original local evidence where available and name missing sources. |
| Does the search envelope claim unseen roads are covered?                                | No. Keep a conservative envelope separate from evidence intervals and preserve unknown or predicted status.                                                                                        |
| How should `/sf/` receive the lane product?                                             | Specify the sidecar packaging, tile and surface revision keys, and placement and rendering tests against the existing driving view.                                                                |
| Can historical lane geometry be tied to the scene's time?                               | Preserve the selected saved geometry with edit and capture dates; label unknown temporal validity rather than silently backdating or discarding it.                                                |
| What tolerances are acceptable for segment matching, seams, draping and simplification? | Set fixture and measured thresholds with the game and web consumers; fragment reassembly may not lose input geometry.                                                                              |
| How are conflicting marks from separate surveys resolved?                               | Preserve distinct identities and flag ambiguity for review; do not merge by proximity.                                                                                                             |

## Checklist

- [x] Inventory all indexed trails in the available scene assets without a radius cap (`S`)
- [x] Identify exact duplicate candidates in the checked-in Ashbury sidecar (`S`)
- [ ] Build the duplicate and coverage report, then review current Ashbury rows in VelocityVisualiser (`M`)
- [ ] Inventory the original full survey windows and specify the SF lane sidecar (`M`)
- [ ] Specify and build the versioned, complete lane-fragment product (`L`)
- [ ] Build the SF lane sidecar adapter and consumer (`M`)
- [ ] Adapt the web scene exporter, player and provenance (`M`)
- [ ] Deliver fresh Ashbury review lines and validate the three paired headless and app-driven replays (`M`)
- [ ] Replay every pinned corpus window overnight and deliver the staged lines and coverage ledger (`L`)
- [ ] Review real-site parity and document omissions and measured limitations (`M`)
