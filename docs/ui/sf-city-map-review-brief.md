# San Francisco city map: review brief, rounds 1–8

The procedures, metrics, results, gaps and review checklist for the San Francisco building, road
and tree layers as they stood after round 8 (2026-11-19), written so another agent could check
the work. The [city map record](sf-city-map.md) supersedes it for the current state: S6 was chosen,
the Street Tree List is now used, and the citywide tiles are baked.

- **Status:** Snapshot after round 8; kept for its procedures, metrics and checklist

## Purpose and short answer

A whole San Francisco building and road layer fits in about 11-16 MB with true roof apexes, right-angled steps on the City's own outlines, a 3DEP tree layer and the towers the City's 2010 outlines miss; this brief records how that was measured so another agent can check it.

The owner asked what Gerardus plans for exportable SF building and road assets, what exists citywide, and how small buildings plus roads could get. Over four review rounds in the SF Level Compare viewer they asked for roofs between the old L2 and L3, true apexes, right angles snapped to the street grid, and finally for three fixes: canopy fitted as roofs, Salesforce Tower missing, and impossible acute shapes.

- **Recommended set:** S1-S5 below, 11.4-16.5 MB for the city. S3 (13.9 MB) is the best balance; S1 is the closest to the raw fit.
- **Status:** measured on three 520 m windows only (Mission & Beale, Sunset Blvd & Noriega, Sheridan & Montgomery). City sizes are scaled, not measured. Nothing is in a repository.
- **Check first:** the three lidar rules (canopy faces, 3DEP-only buildings, tree tops), because every downstream number depends on them; then the scaling.

Companions: the [size study](sf-city-map-size-study.md) (round 1) and the SF Level Compare viewer, an interactive page in the work directory (`viewer/sf-level-compare.html`).

## Data, host and tools

All work ran on `arrow-worker` (4 vCPU, about 9.9 GB RAM, Debian 13) beside the live Gerardus server, which runs `main` at b928e79 since 2026-11-18. Work directory: `/home/banshee/city-assets-20261118/`.

| Input                                                       | What it is                                                                                                                   | Where on arrow                                                 | Licence             |
| ----------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- | ------------------- |
| DataSF Building Footprints (`ynuv-fyni`)                    | 177,023 outlines, 2010 median height and lowest ground                                                                       | `/mnt/results/gerardus/data/footprints/l13/` (344 cells)       | PDDL-1.0            |
| City roof bake (Gerardus `bake-roofs`, fit v6)              | 831 units; 165,267 buildings with 774,310 roof faces, each a polygon plus a plane z = ax + by + c, ENU per unit, NAVD88      | `/mnt/results/gerardus/roofs-city/units/*.json.gz`             | derived, CC0 intent |
| City view tiles (`city-tiles`)                              | 3D Tiles L10/L13/L16, ground + box buildings + street lines, epoch 20261105 (boxes only)                                     | `/mnt/results/gerardus/city-tiles/current`                     | CC0                 |
| DataSF streets (`3psu-pn9h`)                                | 15,671 active segments, 1,960 km                                                                                             | `/mnt/results/gerardus/data/streets/l13/`                      | PDDL-1.0            |
| Curbs and Islands (`emxt-b6yg`), Right of Way (`h8n7-e4ns`) | citywide kerb lines (2016) and street-area polygons (2014)                                                                   | `raw/kerbs/`, `raw/row/` in the work directory                 | PDDL-1.0            |
| USGS 3DEP `CA_SanFrancisco_1_B23`                           | 13.1 billion points, EPT, EPSG:3857 with NAD83(2011), NAVD88 heights; classes 1, 2, 7 only (no building or vegetation class) | `/mnt/results/sf-gis/CA_SanFrancisco_1_B23`                    | public domain       |
| DataSF Street Tree List (`tkzw-k3nq`)                       | street trees with species and DBH                                                                                            | not used yet; the trees layer is from 3DEP only (a gap, below) | PDDL-1.0            |

Tools: a Python 3.13 venv in the work directory (numpy, shapely 2, pyarrow, brotli, zstandard, pyproj, scipy, s2sphere, laspy with lazrs), gltfpack 0.22 and Playwright 1.55.1 from npm, and the `gerardus` CLI (`city-tiles`, `bake-roofs`).

## Procedures used

The work ran in five rounds, each driven by the owner's review of the previous one in the viewer.

1. **Whole-city size ladder (2026-11-18).** Encoded every footprint, roof face, centreline, kerb and right-of-way polygon citywide as a quantised delta stream (counts, then zig-zag varint deltas, columnar) compressed with brotli-11, at several Douglas-Peucker tolerances and grids. Mesh sizes came from `gerardus city-tiles` on three sample L13 cells (downtown `80858064`, Sunset `80858784`, Presidio `808586dc`), repacked per layer with gltfpack (`-cc`, `-vp 12`, `-vp 10`) and scaled to the city by layer from the live epoch's measured sizes. The ground was also re-encoded as a heightmap. A whole-city roofed `city-tiles` bake ran 4 h 20 min on arrow without finishing and was stopped.
2. **Viewer (levels L0-L8).** `scripts/viewer_prep.py` builds one JSON per test intersection (Mission & Beale, Sunset Blvd & Noriega, Sheridan & Montgomery): a 520 m square window in a local ENU frame, buildings, roads and ground at every level, a car route, and differences against full detail and against the next finer level. Each bake unit's roofs are moved by the median offset to the DataSF outlines, as `city-tiles` does. The page is a three.js page with chase, orbit and top views, split screen, highlight and roof-edge switches.
3. **Roof variants V1-V5 (apex-true).** `scripts/roofsimplify.py` rebuilds a roof as a noded edge graph: junctions where 3+ faces meet move to the point where their planes meet; ridges, hips and valleys become straight segments; eaves and steps get Douglas-Peucker; pieces from `polygonize` go to the face they overlap most. The encoder stores each roof as one welded vertex table with face loops and planes; apex and ridge-end vertices that the planes reproduce are not stored.
4. **Right-angle variants W1-W5.** Eaves are always the DataSF outline. Steps between tiers are snapped to each building's axis, taken from its footprint's length-weighted edge directions. W4 and W5 put step vertices on a 0.25 or 0.5 m lattice along that axis, coded as lattice steps. Plane snapping was tried, measured and turned off.
5. **This round: canopy, missing buildings, spikes (S1-S5).** 3DEP class-1 points are read at EPT depth 9 with their return counts (`scripts/ept.py`, `scripts/lidar_layers.py`). A roof face is canopy when over 30 % of its points are multi-return and their median distance to the plane exceeds 15 cm, or the median exceeds 40 cm; near-vertical faces under 15 m² are flagged too. Flagged faces are absorbed into the neighbour they share the longest edge with (base C0). Trees are local maxima of a 1 m canopy height model of rough, multi-return cells, with crown radii from the canopy cells nearest each top. Buildings DataSF lacks are solid, planar cells at least 3 m above ground outside every footprint (1.5 m buffer) and fitted roof, split into flat tiers at 1.5 m steps. After simplification, spike vertices under 20° are removed when exactly two faces share them and the wedge is under 1 m², and thin or wedge faces under 2-4 m² are absorbed.

Every round was scored by the lidar test of the next section, on the same three windows.

## Metrics

The one fidelity measure that held up is the share of good lidar roof points a level breaks; the others are guards against specific failures.

| Metric                        | Definition                                                                                                                                                            | Catches                                    | Misses                                                                                                           |
| ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------- |
| Bytes                         | brotli-11 of the level's own encoder (unwelded rings, or the welded graph with `enc_graph2`); "on boxes" leaves out vertices the DataSF outline layer already carries | relative size between levels               | real file formats; FlatGeobuf or GeoParquet will be larger. Measured only at the decoder-free level              |
| City size                     | three-site byte ratio against L2, times L2's measured citywide 18.0 MB of roofs, plus 0.81 MB of boxes and roads                                                      | order of magnitude                         | the three windows over-weight downtown; the same scaling predicted L3 5 % low                                    |
| Points broken > 25 cm         | of 3DEP class-1 points the reference model fits within 25 cm (the "good" points), the share a level misses by more than 25 cm or no longer covers                     | surface error where the roof is truly seen | errors the reference shares (the reference is the bake's own fit); roofs under canopy, which have no good points |
| Uncovered                     | good points no face covers                                                                                                                                            | seams and lost faces                       | nothing beyond that                                                                                              |
| Apex error                    | per building, highest model vertex against the 99.5th percentile of its good points                                                                                   | apex height drift                          | chimneys and rooftop plant dominate it (p90 about 1 m at every level), so it only flags large regressions        |
| Off-axis edge length          | roof edge length not within 3° of the building's axis, its perpendicular or 45°                                                                                       | free-angle steps and eaves                 | hips between unequal slopes are off-axis by design                                                               |
| Faces with a corner under 20° | share of roof face rings whose sharpest corner is under 20°                                                                                                           | spikes, wedges, slivers                    | real acute lot corners in DataSF count too                                                                       |
| Height difference (viewer)    | per face, 90th percentile of the vertical gap to the reference height field on a 1 m grid                                                                             | where a level departs in the viewer        | the raw L0 reference overshoots ridges by up to one raster step, so it overstates errors at ridges               |

A trap found on the way: scoring apexes against L0's highest vertex penalised the plane-intersection apexes, because L0's raster edges overshoot the true ridge. Score against the lidar instead.

## Results

The proposed set spans 11.4-16.5 MB for the whole city, buildings, roads and trees included; S3 (13.9 MB) is the best balance found, breaking 0.9-1.4 % of good lidar roof points.

**Earlier rounds, for context.** Whole city: footprints with 2010 heights 5.25 MB; centrelines 0.16 MB, kerbs 0.30 MB, right of way 0.32 MB; L2 roof planes 18.0 MB of roofs (18.8 MB with boxes and roads); L3 13.8 MB; today's served city tiles 277 MB gzip; roofed tiles about 470 MB (scaled). The V and W rounds reached about 10-15 MB but carried canopy faces, stale 2010 outlines and sharp wedges, which this round fixes.

**This round (three windows, 0.81 km²).** City sizes are standalone totals: roofs scaled by the byte ratio to L2, plus 0.81 MB of boxes and roads, 2.87 MB of trees and 0.62 MB of 3DEP-only buildings, both scaled by area. Broken and sharp-corner shares are Presidio / Sunset / downtown.

| Level                   | Whole city        | Roofs on top of boxes | Points broken > 25 cm | Faces with a corner < 20° | What it drops                  |
| ----------------------- | ----------------- | --------------------- | --------------------- | ------------------------- | ------------------------------ |
| C0 cleaned full detail  | ≈ 23.2 MB         | 18.9 MB               | reference             | 12.1 / 11.9 / 3.0 %       | canopy and steep faces only    |
| S1 welded, 0.5 m steps  | ≈ 16.5 MB         | 10.7 MB               | 0.66 / 1.10 / 0.98 %  | 3.5 / 3.1 / 5.1 %         | steps < 0.5 m, faces < 1 m²    |
| S2 small faces absorbed | ≈ 15.7 MB         | 9.9 MB                | 0.85 / 1.24 / 1.15 %  | 4.1 / 3.3 / 5.4 %         | faces < 2 m², steps < 0.75 m   |
| **S3 0.25 m lattice**   | **≈ 13.9 MB**     | **9.0 MB**            | 0.87 / 1.28 / 1.42 %  | 4.5 / 4.8 / 5.1 %         | step vertices move up to 18 cm |
| S4 0.5 m lattice        | ≈ 12.3 MB         | 7.6 MB                | 1.00 / 2.12 / 2.18 %  | 2.5 / 5.0 / 5.4 %         | faces < 3 m², steps < 1 m      |
| S5 compact              | ≈ 11.4 MB         | 6.7 MB                | 1.20 / 3.60 / 3.10 %  | 2.5 / 5.9 / 5.1 %         | faces < 4 m², steps < 1.25 m   |
| L0 raw bake             | 28.2 MB as planes | —                     | 0.06 / 0.06 / 0.13 %  | 9.2 / 7.9 / 2.4 %         | nothing; keeps canopy faces    |
| L2 old                  | 18.8 MB           | —                     | 0.22 / 0.17 / 0.23 %  | 4.7 / 3.2 / 0.9 %         | seams; keeps canopy faces      |

**The three problems the owner raised:**

- **Trees taken for roofs.** 680 of 4,241 raw faces were absorbed: Presidio 57, Sunset 433, downtown 190 (including Salesforce Park's garden). The trees layer holds 1,555 / 2,764 / 2,171 trees, about 2.9 MB for the city at an area-scaled 0.97 million trees (an often-quoted estimate for San Francisco is about 700,000, from memory and not checked here, so the windows probably over-weight forest). A few false trees remain in the narrow gaps beside downtown towers.
- **Salesforce Tower missing.** Found as a 330.6 m 3DEP-only building. Downtown also has 13 DataSF lots rebuilt since 2010 (for example a 0.2 m lot now 325.7 m, a 4.5 m lot now 181.4 m) and 19 3DEP-only buildings in all, including 181 Fremont at 250.6 m; the Presidio has 3 and the Sunset none.
- **Sharp acute angles.** Causes found: thin raster teeth in the raw fit, canopy faces, near-vertical faces, junctions moved up to 1.5 m, and edges meeting the eaves at shallow angles. Faces with a corner under 20° fell from 8-12 % (C0) to 2.5-5.9 % in S1-S5; downtown's S levels (5 %) are still above L2 (0.9 %).

**Follow-up, trees off the road (2026-11-19).** The owner found trunks in the carriageway. A trunk now counts as on the carriageway when it lies in the right of way, the line to its nearest centreline crosses no kerb, and it is not inside a closed kerb ring (a block or a median). Where the City has no kerb or right-of-way data (the Presidio), anything within 3.5 m of a centreline counts. Such trees move to the nearest Street Tree List trunk (`tkzw-k3nq`, 138,896 positions) within 8 m, else 0.9 m past the nearest kerb, else they are dropped. Moved: Presidio 70, Sunset 589, downtown 802; dropped 8 + 53. Near buildings trees are capped at 30 m, which removed most false trees made from tower facade returns. Trees now: 1,555 / 2,756 / 2,131.

**S5 by layer, whole city:** buildings about 8.0 MB (roofs 7.0, boxes for unroofed buildings 0.35, 3DEP-only and rebuilt 0.6); roads 0.46 MB (centrelines 0.16, kerbs 0.30; street area +0.32); trees 3.1 MB at 0.1 m or 2.4 MB at 0.5 m positions (area-scaled, about 0.96 million trees, likely high). Total about 11.6 MB, or 10.8 MB with compact trees. Ground is extra: about 2.4-5 MB as a 4 m heightmap, 0.6 MB at 10 m.

**S6, under 10 MB (2026-11-19).** The owner chose S5's look and asked for a bundle under 10 MB with coarser trees, fewer faces and points still on their faces. Three changes, each scored with the lidar test:

1. **Merge by fit, not by angle.** Neighbouring faces merge, cheapest pair first, only while one least-squares plane over both stays within 18 cm of the original surface at the 90th percentile and 45 cm at worst (`roofsimplify.merge_by_fit`). Faces barely fell (2,607 against S5's 2,517): S5's angle rule had been merging planes that do not fit. Points broken fell to 1.17 / 2.82 / 2.66 % from S5's 1.20 / 3.60 / 3.10 %.
2. **Plane code.** Each plane is stored as its direction relative to the building axis (0.5°), pitch (0.25°) and height at the face's first corner (2 cm), not as a, b and an offset at a far origin (`quantize_plane`). Plane bytes fell 28 %; fit unchanged within 0.1 point.
3. **Simpler eaves.** DataSF outlines simplified at 0.5 m before the roof is built on them.

Trees ship at 0.5 m positions and 1 m height and crown, re-checked against the carriageway after rounding. A further rule removes tree tops on or beside roofs found in the lidar (solid, planar cells) unless the canopy stands 2 m above the roof; that fixed trees along Presidio rooflines and also removed some backyard trees no taller than the house next to them.

| Layer, whole city                                     | S6           |
| ----------------------------------------------------- | ------------ |
| Roofs                                                 | 6.41 MB      |
| Boxes for buildings without a fitted roof             | 0.35 MB      |
| Buildings only 3DEP sees, and lots rebuilt since 2010 | 0.62 MB      |
| Roads: centrelines 0.16, kerbs 0.30                   | 0.46 MB      |
| Trees (about 0.71 million, area-scaled)               | 1.66 MB      |
| **Total**                                             | **≈ 9.5 MB** |

Points broken: 1.17 / 2.82 / 2.47 % (Presidio / Sunset / downtown). Faces with a corner under 20°: 2.8 / 3.4 / 5.7 %. Trees now 1,134 / 1,747 / 1,858. Open: the 3DEP-only buildings still have 1 m cell staircase edges; downtown keeps a few stubs beside Salesforce Tower.

**Trees reconciled with the Street Tree List, and visible roads (2026-11-19).** Inside the street right of way the City's list is now the authority (`lidar_layers.reconcile_trees`): each listed tree takes the nearest unclaimed lidar top within 4 m for its height and crown, or a size from its trunk diameter (height 2.5 + 0.45 × DBH in inches, 3-20 m; crown 0.6 + 0.12 × DBH, 1-6 m). Unmatched lidar tops in the street are dropped when within 6 m of a listed tree or under 6 m tall, so big unlisted trees such as Sunset Blvd's median stay. Off the street, tops closer than 45 % of their combined crown radii merge into one. Trees: Presidio 1,116 (no list there), Sunset 1,978 (713 listed, 140 matched; 573 sized from the list, mostly young trees the 2023 lidar does not see), downtown 1,408 (was 1,858). City scale: about 4,500 in the windows, 1.6 MB, and the bundle stays at about 9.5 MB.

Roads in the viewer now draw the carriageway as asphalt between the kerbs, using each centreline's distance to the first kerb on either side (median-smoothed), with kerbs as 0.3 m ribbons raised 15 cm, dashed centrelines, and the rest of the right of way tinted as sidewalk. The widths are derived from centrelines and kerbs already in the bundle, so nothing is added to its size.

## Tall and curved buildings: cross-validation and better shapes

Salesforce Tower and the other post-2010 towers are drawn as 1 m lidar cells grouped into flat tiers, which matches the roofs but steps every curve; a first attempt at a better shape did not beat the tiers, and the reason points at what to build next.

**What was tried (2026-11-19).** For every 3DEP-only structure over 60 m, the shaft above 60 m was rebuilt as stacked 4 m slices, each the concave outline of the lidar returns in its height band, and scored against the tiers on the share of shaft returns within 1 m of the model's wall:

| Tower (3DEP id, height)             | Tiers: within 1 m | Slices: within 1 m | Kept   |
| ----------------------------------- | ----------------- | ------------------ | ------ |
| 3dep:24, 330.6 m (Salesforce Tower) | 47.7 %            | 47.5 %             | tiers  |
| 3dep:37, 250.6 m (181 Fremont)      | 75.2 %            | 37.7 %             | tiers  |
| 3dep:91, 140.8 m                    | 93.0 %            | 49.6 %             | tiers  |
| 3dep:154, 122.1 m                   | 95.9 %            | 45.4 %             | tiers  |
| 3dep:142, 95.6 m                    | 90.3 %            | 26.5 %             | tiers  |
| 3dep:113, 94.9 m                    | 32.0 %            | 38.7 %             | slices |

Why it fell short: aerial returns on facades are sparse and oblique, so a concave outline per band is noisy; lidar components merge a tower with its podium and neighbours (Salesforce Tower's took in the Transit Center deck); and the score credits tiers for roof returns at tier tops. The score itself needs fixing before any shape comparison: separate facade from roof returns by local normal first.

**Datasets to cross-validate against** (validation only unless the licence is public domain, per ADR-0008):

| Source                                                                                            | Checks                                            | Use                                                                                               |
| ------------------------------------------------------------------------------------------------- | ------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| DataSF building permits (new-construction filings)                                                | lots rebuilt since 2010, stories, proposed height | flag stale outlines before the lidar does; confirm the 13 rebuilt downtown lots                   |
| DataSF Assessor secured roll (year built, stories)                                                | building age and storeys                          | stories × storey height against the fitted roof; year built after 2010 marks a stale 2010 outline |
| SF Planning height and bulk districts                                                             | legal height limits                               | upper bound; anything above it needs a reason                                                     |
| Published heights of tall buildings (Salesforce Tower 326 m to the roof, 181 Fremont about 245 m) | absolute height of the 30-40 towers over 120 m    | a fixed QA list; the lidar's 330.6 m includes the crown                                           |
| Earlier USGS San Francisco lidar                                                                  | change since then                                 | separates new buildings from fit errors                                                           |
| Public-domain orthoimagery (USGS, NAIP)                                                           | roof edges, curved outlines                       | outline refinement and visual QA                                                                  |
| Our own street-level scans (velocity.report sites)                                                | facades within about 30 m of a sensor             | ground truth for walls where aerial returns are thin                                              |
| OSM, Overture, Microsoft building footprints                                                      | outlines and heights                              | comparison only (ODbL or similar); never an input                                                 |

**Approaches for tall curved towers, aligned to their points:**

1. **Split returns by surface first.** Estimate a normal per point from its neighbours; near-vertical normals are facade, near-horizontal are roof. Fit and score walls only on facade returns.
2. **Parametric shafts.** Fit each tower shaft as a lofted cross-section whose parameters vary smoothly with height: a superellipse (rounded square to circle) with centre, half-widths, corner exponent and rotation as low-order functions of z. Salesforce Tower is a tapering rounded square with a crown; about a dozen numbers describe it, the surface is smooth, and it compresses to almost nothing. Choose among prism, tapered prism, cylinder and superellipse loft per tower by fit and parameter count.
3. **Denser points for tall structures.** Read the EPT to full depth inside tall components only (a few million points per tower) so each band has enough facade returns.
4. **Separate the parts.** Split podium, shaft and crown by height and footprint change; draw an open crown (Salesforce Tower's lattice) as its own component.
5. **A reference mesh for validation.** Screened Poisson reconstruction of a tower's points, decimated, as a check on the shipped primitive, not as the shipped shape.
6. **A per-tower report.** Height against the published value, cross-section against plans or permits, and facade residual at p50 and p90.

Suggested next step: prototype the superellipse loft on Salesforce Tower with full-depth points and normal-split scoring, then run it on the six towers above.

## Gaps and risks

The largest gap is that no independent truth exists: every fidelity number compares levels with the bake's own fit or with lidar points that fit already agrees with.

| Gap                                | Severity | Detail                                                                                                                                                                                                                                                        | What closes it                                                                                                 |
| ---------------------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| No hand-labelled truth             | high     | The lidar test only scores points the reference fits within 25 cm; errors shared by every level (a missed dormer, a mis-split roof) are invisible                                                                                                             | CITY-BUILDINGS C3 / W9: 20-30 hand-traced roofs per kind of place, scored with polygon IoU and vertex distance |
| Three windows only                 | high     | 0.81 km² of about 121 km²; downtown is over-weighted; city sizes are byte ratios against L2 (L3 came out 5 % low this way); trees and 3DEP-only buildings are scaled by area                                                                                  | Run `viewer_prep2.py` logic over 20-30 random L13 cells, or the city, and measure                              |
| Canopy rule is tuned, not learned  | medium   | Multi-return over 25 %, over 15 % of core points more than 1 m below the plane, under 40 m above ground. Presidio eucalyptus over 40 m and dense evergreen canopy that stops pulses would be missed; a roof garden counts as canopy                           | Label a few hundred faces in the viewer; check against the Street Tree List (`tkzw-k3nq`, not used yet)        |
| Roof under canopy is guessed       | medium   | An absorbed canopy face takes its neighbour's plane; no good points exist there to test it                                                                                                                                                                    | Accept, or keep those faces as a separate "occluded" class                                                     |
| 3DEP-only buildings are flat tiers | medium   | Tiers at 1.5 m steps with the 75th-percentile top; curved crowns (Salesforce Tower) and sloped roofs come out stepped. The 2010-height test flags a footprint when its lidar top is 10 m and 50 % above its 2010 height; adjacent tower facades could trip it | Fit planes in those outlines with the same fit the bake uses; review the 13 flagged downtown lots              |
| Unit offset, not per building      | medium   | Roofs are moved onto DataSF outlines by each bake unit's median offset; downtown faces spill 0.5-1.5 m over streets in places                                                                                                                                 | A per-building shift, as Gerardus bundles fit per block                                                        |
| Sharp corners remain               | medium   | Junctions where 3+ faces meet and edges meeting the eaves at shallow angles still leave 3-8 % of faces with a corner under 20° (raw bake: 3-9 %)                                                                                                              | Constrain the junction move by the angles it creates; snap ridge ends to eave vertices                         |
| No decoder, no real format         | medium   | Bytes come from a custom columnar encoder; implicit vertices are re-solved from quantised planes on the encoder side only                                                                                                                                     | Write the decoder and round-trip test; measure GeoParquet, CityJSONSeq and meshopt glTF for the chosen level   |
| Viewer solids are prisms           | low      | Each face is drawn as a prism from the building's base to its plane; there is no closed LoD 2.2 solid and no val3dity check                                                                                                                                   | CITY-BUILDINGS §5.6 export with validation                                                                     |
| Not in Gerardus                    | medium   | All code is scripts in the work directory; nothing is versioned, reviewed or tied to the fit version                                                                                                                                                          | Port `roofsimplify` into Gerardus's bake or a `city-roofs` export, keyed to `FIT_VERSION`                      |
| Citywide roofed tiles never baked  | low      | The roofed `city-tiles` run took over 4 h on arrow without finishing beside the live server                                                                                                                                                                   | Run on a Mac or with the server quiet                                                                          |
| Old data                           | known    | DataSF heights are 2010; kerbs 2016; right of way 2014 (includes sidewalks); 3DEP April 2023                                                                                                                                                                  | Dates go in any release manifest                                                                               |

## Next steps and review checklist

The reviewing agent should first try to break the three lidar rules, because every result downstream depends on them.

- [ ] Open the SF Level Compare viewer at the three intersections and compare S1-S5 against C0 and L2 with "Show roof edges" on; list any roof that still looks impossible, with its building id from the data file.
- [ ] Check the canopy rule on 50 flagged and 50 unflagged faces per site (`lidar_layers.face_flags`), especially downtown's 190 flagged faces and the Salesforce Park roof garden.
- [ ] Review the 13 downtown footprints marked as rebuilt since 2010 and the 3DEP-only buildings (downtown 19 including Salesforce Tower at 330.6 m and 181 Fremont at 250.6 m; Presidio 3; Sunset 0) against current imagery.
- [ ] Check the tree layer against the Street Tree List (`tkzw-k3nq`): share of listed trees with a detected top within 3 m, and detected tops on roofs.
- [ ] Re-derive one number end to end (for example S3's Sunset bytes and broken share) from `viewer/data2/summary-sunset.json` and the scripts.
- [ ] Run the S-level pipeline on 20-30 random L13 cells away from these three windows, to replace the area and byte-ratio scaling with measured city sizes.
- [ ] Write the decoder for `enc_graph2` and round-trip the three sites; then measure the chosen level as GeoParquet and as meshopt glTF.
- [ ] Decide with the owner which S level to carry forward, then port `roofsimplify` and `lidar_layers` into Gerardus (a `city-roofs` export keyed to `FIT_VERSION`), with tests, before any citywide run.
- [ ] Rebake the served city tiles with roofs once a level is chosen, off arrow or with the server quiet.

Open questions for the owner: whether trees and 3DEP-only buildings belong in the CC0 release (both come from public-domain 3DEP), whether the tiered form (box layer plus roof add-on) or a single file is the product, and whether S1's 16 MB or S4's 12 MB is the right default.

## How to reproduce

Everything is in `/home/banshee/city-assets-20261118/` on arrow-worker; nothing is committed to a repository yet.

| Step                              | Command (from the work directory)                                                                                                                         | Output                                            |
| --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| Citywide vector sizes             | `venv/bin/python -I scripts/buildings.py /mnt/results/gerardus/data out/buildings-city.json`, `scripts/roofs.py`, `scripts/roads.py`                      | `out/*-city.json`                                 |
| Sample mesh sizes                 | `gerardus city-tiles --offline --cells 80858064,80858784,808586dc --out tiles-roofed --no-caps`, then `scripts/strip.py`, gltfpack, `scripts/glbstats.py` | `out/glb-*.json`, `out/layers-sample.json`        |
| Roof variants with the lidar test | `venv/bin/python -I scripts/roof_variants.py out/roof-variants2-<site>.json <site>`; `scripts/rv_summary.py roof-variants2`                               | per-site metrics                                  |
| Viewer data, this round           | `venv/bin/python -I scripts/viewer_prep2.py viewer/data2 <site>` (about 2-4 min a site)                                                                   | `viewer/data2/<site>.json`, `summary-<site>.json` |
| Viewer page                       | `viewer/sf-level-compare.html` with `data2/*.json`, opened as a page                                                                                      |                                                   |

Modules: `common.py` (projection, varint, compressors), `roofsimplify.py` (topology rebuild, right angles, lattice, absorb, despike, encoders), `lidar_layers.py` (canopy flags, grids, trees, missing buildings), `ept.py` (EPT reader), `viewer_prep.py` (frames, ground, roads, diffs).

Traps on arrow:

- Memory: the server holds 4-6 GB. Run Python jobs under `ulimit -v 5500000`, and never make shapely Point objects for millions of lidar points (use `shapely.contains_xy` on arrays). Two earlier out-of-memory kills pushed the server 4.6 GB into swap.
- A process guard's `pkill -f` pattern must not match its own command line; anchor it (`'^gerardus city-tiles'`).
- WebGL screenshots: Playwright 1.55.1 from npm with the cached Chromium in `~/.cache/ms-playwright`, launched with `--use-gl=swiftshader --enable-unsafe-swiftshader`.
- Brotli-11 with a 24-bit window is slow on 100 MB inputs; `FAST=1` makes `common.sizes` gzip-only.
