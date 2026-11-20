# San Francisco city map: size study

What Gerardus had for the whole of San Francisco on 2026-11-18, what its plans say it will
publish, and how small the city's buildings and roads get at each level of detail, with what each
level keeps and loses. This is round 1 of the [city map record](sf-city-map.md), with the first
two roof rounds (V and W) added after it.

- **Status:** Measured 2026-11-18 and 2026-11-19 on arrow-worker; S6 later superseded these levels

## Summary

| Question                | Answer          | What it holds                                                                                                                                                           |
| ----------------------- | --------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Smallest useful city    | about 2.5–6 MB  | Every building as a footprint plus height (LoD1, 2.5D), every street centreline and kerb line, brotli-compressed; the client raises each outline to its measured height |
| With real roofs (LoD2)  | about 14–19 MB  | The city roof bake's planes for 165,267 buildings, stored as plane and outline per face; the rest fall back to boxes                                                    |
| As ready-to-draw meshes | about 90–300 MB | Roofed glTF at full detail; meshopt compression cuts it 2.3–3.3×. The served city tiles (boxes, ground, three levels) are 277 MB                                        |
| Roads alone             | under 1 MB      | 1,960 km of centrelines with name, class and one-way: 0.16 MB. Kerbs and street-area polygons add about 0.3 MB each                                                     |

The ground under the roads is what costs bytes, not the roads.

## The size ladder for the whole city

Each rung is buildings and roads for all of San Francisco, compressed. "Measured" rungs were
measured over the whole city; "scaled" rungs were scaled from three sample L13 cells (downtown
`80858064`, the Sunset `80858784`, the Presidio `808586dc`), for meshes that were not baked
citywide on arrow. Ground rows are optional and are neither buildings nor roads.

| Rung                                        | Detail                                       | Whole city | Basis    |
| ------------------------------------------- | -------------------------------------------- | ---------- | -------- |
| City view tiles, all 3 levels, roofed       | ground, roofs, streets, labels               | 474 MB     | scaled   |
| City view tiles today, all 3 levels         | boxes, ground, streets, labels (served)      | 277 MB     | measured |
| Roofed L16 buildings and streets, glTF      | gzip, full precision                         | 299 MB     | scaled   |
| Same, meshopt 14-bit                        | gltfpack `-cc`                               | 131 MB     | scaled   |
| Same, meshopt 10-bit                        | about 15 cm grid                             | 91 MB      | scaled   |
| Box L16 buildings and streets, glTF         | gzip, as served                              | 102 MB     | measured |
| LoD2 planes, LoD1 for the rest, roads       | 0.25 m outlines, 5 cm grid                   | 18.8 MB    | measured |
| LoD2 planes, 1 m outlines                   | seams open, dormers deform                   | 13.8 MB    | measured |
| LoD2 welded graph, true apexes (V5)         | no seams; scaled from 3 sites                | 10.5 MB    | scaled   |
| LoD1 full outlines and roads                | 10 cm grid, all vertices                     | 5.71 MB    | measured |
| LoD1 0.5 m and roads                        | p95 shift 0.49 m                             | 4.02 MB    | measured |
| LoD1 1 m, 25 m² and over, 0.5 m grid, roads | sheds dropped (estimated from measured rows) | 2.4 MB     | measured |
| Blocks 5 m and arterials                    | 56,961 masses                                | 0.67 MB    | measured |
| Skyline blocks and arterials                | 14,843 masses                                | 0.22 MB    | measured |
| Ground, 4 m mesh (optional)                 | today's L16 ground                           | 105 MB     | measured |
| Ground, 4 m heightmap (optional)            | 5 cm steps                                   | 5.0 MB     | scaled   |

Mesh rungs are gzip of glTF (`.glb`), as a static host serves it. Vector rungs are brotli of a
quantised delta stream (counts, then zig-zag varint deltas, columnar), the idea MVT and FlatGeobuf
with compression use. It is a floor for what a real format reaches: GeoParquet of the same
full-precision footprints is 30.5 MB, and DataSF's own GeoJSON is 124 MB raw and 18.8 MB brotli.

## What each level keeps and loses

| Level                                                                                                | City size             | Keeps                                                                                                                     | Loses                                                                                            | Measured loss                                   |
| ---------------------------------------------------------------------------------------------------- | --------------------- | ------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ | ----------------------------------------------- |
| Roofed meshes, L16 glTF, full detail                                                                 | about 300 MB (scaled) | Roof faces from 3DEP (fit v6), walls to each roof edge, steps between faces, boxes elsewhere, draped street lines, labels | Nothing against the bake; still 2010 DataSF outlines and no post-2010 towers without footprints  | —                                               |
| Roofed meshes, meshopt (gltfpack `-cc`, about 1 cm)                                                  | about 130 MB (scaled) | Same geometry, positions quantised to 14 bits per tile                                                                    | Sub-centimetre precision; needs the `EXT_meshopt` decoder (three.js, Cesium and Babylon have it) | 2.3× smaller than gzip alone                    |
| Roofed meshes, meshopt 10-bit                                                                        | about 90 MB (scaled)  | Same shapes                                                                                                               | Vertices snap to about 15 cm at L16, coarser on large tiles; small roof steps blur               | 3.3× smaller                                    |
| LoD2 planes: each roof face as its outline plus z = ax + by + c, 0.25 m outline tolerance, 5 cm grid | about 18.8 MB         | Every roof face and its slope; ground height; client rebuilds walls and triangles                                         | Faces stored unwelded, so outline simplification opens hairline seams between faces              | Face outlines 0.6 % area changed                |
| LoD2 planes, 1 m tolerance                                                                           | about 13.8 MB         | Every face and slope                                                                                                      | Dormers and small faces deform; seams open                                                       | 6.7 % face area changed; 16 more faces collapse |
| LoD1 boxes, full outline (footprint, ground, 2010 median height)                                     | 5.7 MB                | Every DataSF footprint vertex at 10 cm, height and ground at 10 cm; all centrelines and kerbs                             | Roof shapes (flat tops at median height); post-2010 buildings                                    | None against DataSF                             |
| LoD1, 0.5 m tolerance                                                                                | 4.0 MB                | All 177,023 buildings                                                                                                     | Bay-window and cornice wiggles in outlines                                                       | 0.95 % area; p95 shift 0.49 m                   |
| LoD1, 1 m tolerance, no sheds under 25 m², 0.5 m grid                                                | about 2.4 MB          | 165,684 buildings, 99.6 % of footprint area                                                                               | 11,339 sheds and kiosks; outline detail                                                          | 2.8 % area; p95 shift 0.99 m                    |
| City blocks: touching footprints merged, 5 m tolerance                                               | 0.67 MB               | 56,961 block masses, arterials                                                                                            | Individual buildings and their own heights; courtyards filled                                    | —                                               |
| Skyline only: blocks over 400 m², 10 m tolerance, arterials                                          | 0.22 MB               | 14,843 block masses, 293 km of arterials                                                                                  | Most of the city's fabric; a far-zoom silhouette only                                            | —                                               |

LoD2 rows add the boxes for the 11,756 buildings the bake has no faces for (0.35 MB) and roads
(0.46 MB). Heights cost little: footprints without heights are 4.97 MB against 5.25 MB with them.

## Roads in detail

| Layer (citywide, DataSF PDDL)                                       | Features | Length or area | Source text                | Compact                         | Notes                                                                                                                                            |
| ------------------------------------------------------------------- | -------- | -------------- | -------------------------- | ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| Street centrelines (`3psu-pn9h`), active, with name, class, one-way | 15,671   | 1,960 km       | 0.57 MB brotli (WKT)       | 0.16 MB                         | 2D only. In Gerardus for every L13 cell. 10 m tolerance: 0.12 MB. Arterials only: 0.03 MB                                                        |
| Curbs and Islands (`emxt-b6yg`, 2016)                               | 7,195    | 2,861 km       | 104 MB raw, 22.9 MB brotli | 0.30 MB                         | 0.25 m tolerance, 5 cm grid; the source has 2.6 million vertices, mostly arc densification. Gerardus caches it only at the 49 sites              |
| Right-of-way polygons (`h8n7-e4ns`, 2014)                           | 22,777   | 29.6 km²       | 32.7 MB raw, 7.1 MB brotli | 0.32 MB                         | Property line to property line, so it includes sidewalks. Split into segment and intersection pieces by CNN. Dissolved into one surface: 0.12 MB |
| Draped street lines in the served tiles (L16)                       | 15,384   | —              | —                          | 6.9 MB gzip                     | 3D lines densified to the 4 m ground; meshopt about 3.3 MB                                                                                       |
| Parametric road surface (ROAD-SURFACE plan)                         | —        | 1,950 km       | —                          | 15–18 MB, under 8 MB compressed | A plan estimate, not built: reference line, height, cross-fall, widths, friction and hazard tags                                                 |

Roads become expensive only when they carry their own height. Draping them on a ground model is
the cheap way, and the ground can be much smaller than the served mesh:

| Ground, whole city                  | Size               | How measured                                       |
| ----------------------------------- | ------------------ | -------------------------------------------------- |
| 4 m triangle mesh, served L16 tiles | 105 MB gzip        | measured: 16.1 million triangles                   |
| Same mesh, meshopt / 10-bit         | about 23 / 16 MB   | scaled from the three cells                        |
| 4 m heightmap, 1 cm / 5 cm steps    | about 7.3 / 5.0 MB | scaled: lattice heights, planar prediction, brotli |
| 10 m heightmap, 5 cm steps          | about 0.7 MB       | scaled from the L13 grids                          |

## What Gerardus had citywide

| Asset                                                       | Coverage                                                                                                              | Where                                                                    | State                                                                          |
| ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------------------ |
| DataSF building footprints with 2010 heights                | 177,023, all 344 L13 cells                                                                                            | `data/footprints/l13/`, 84 MB                                            | cached                                                                         |
| City roof bake (LoD2 fit v6)                                | 831 units; 165,267 roofed, 774,310 faces; 1,965 with no points, 891 with no planes, 8,750 left to sites at unit edges | `/mnt/results/gerardus/roofs-city`, 313 MB                               | baked 5–6 November in 20.3 h on arrow; a cache keyed to the fit, not a release |
| City view tiles (3D Tiles 1.1, L10 / L13 / L16)             | 8,428 tiles: ground, boxes, street lines, labels                                                                      | `city-tiles/current`, epoch `20261105T200804Z`, served at `/city-tiles/` | served; baked before the roofs finished, so every building is a box            |
| Street centrelines, nodes, names                            | 17,176 segments, all cells                                                                                            | `data/streets/l13/`, 6.8 MB                                              | cached                                                                         |
| Kerbs, speed limits, lane markings, street furniture, trees | 49 site cells only                                                                                                    | `data/kerbs` and others                                                  | site-driven; the citywide kerbs were fetched for this study (7,195 lines)      |
| Road surface                                                | One Golden Gate Park preview corridor                                                                                 | `data/roads/preview/site-808587423`                                      | prototype; 0.5 m grid, held-out RMS 0.018 m                                    |
| 3DEP aerial lidar (QL0, 2023)                               | 13.1 billion points                                                                                                   | `sf-gis/CA_SanFrancisco_1_B23` EPT, 63 GB                                | indexed                                                                        |

## What the plans say

Gerardus plans as read on 2026-11-18:

- **Buildings ([CITY-BUILDINGS](https://github.com/banshee-data/gerardus/blob/main/docs/plans/CITY-BUILDINGS.md)).**
  One CC0 layer of every building's parts, roof planes and walls, from 3DEP and DataSF only (no
  OSM, no street scans). Release formats: GeoParquet first, CityJSONSeq per L16 for LoD 2.2
  solids, PMTiles for web maps, FlatGeobuf; 3D Tiles later. Built: the bake (C2), its whole-city
  run (C4, as a cache) and the server reading it (C6). Not built: exports (C5), validation of LoD
  2.2 solids and publication (C7). Gates: a stable fit version and W9's hand-labelled accuracy
  sample before anything ships with confidence tags; the plan records only 23–27 % of main parts
  at high confidence.
- **City view ([CITY-VIEW-TILES](https://github.com/banshee-data/gerardus/blob/main/docs/plans/CITY-VIEW-TILES.md)).**
  T0–T3 built; the epoch on arrow is the first T4 run. Roofs at L16 (§10) are implemented but
  need a rebake after the roof bake. Caps: 2 MB per L10 tile, 1 MB per L13, 256 KB per L16; 1 GB
  per epoch since roofs were added.
- **Roads ([ROAD-SURFACE](https://github.com/banshee-data/gerardus/blob/main/docs/plans/ROAD-SURFACE.md),
  [ROAD-WEB-BUILD](https://github.com/banshee-data/gerardus/blob/main/docs/plans/ROAD-WEB-BUILD.md)).**
  Every road as curves: a reference line of straights,
  spirals and arcs, with height, cross-fall, widths and friction along it, plus sparse hazard
  tags; about 15 MB for the city against 19 GB for a 5 cm grid. CC0 data with an Apache-2.0
  reader (ADR-0021); the vehicle model and engine adapters are a separate licensed SDK. Nothing of
  R0–R11 is built; the first slice is the Golden Gate Park prototype (`gerardus road-preview`).
  The web game targets under 6 MB before play and under 1 MB streamed per kilometre, with LoD1
  scenery from the city tiles.

## Between L2 and L3: roofs with true apexes (V1–V5)

L2 and L3 store each roof face's outline on its own. The outlines follow the fit's 0.25 m raster,
so ridges are staircases and neighbouring faces leave hairline seams. The V variants rebuild each
roof as one graph: every junction where three or more faces meet moves to the point where their
planes meet (the true apex); ridges, hips and valleys become straight edges between those
junctions; only eaves and steps between roof tiers are line-simplified. Measured on the three
viewer intersections (4,500 roof faces) and scored against 3DEP points that full detail already
fits within 25 cm.

| Variant                          | Size against L2 | City, roofs                   | Points broken > 25 cm | Points uncovered | What changes                                                                                                      |
| -------------------------------- | --------------- | ----------------------------- | --------------------- | ---------------- | ----------------------------------------------------------------------------------------------------------------- |
| L2, 0.25 m outlines              | 100 %           | 18.0 MB                       | 0.14 %                | 0.07 %           | Reference for this step: staircase ridges, seams between faces                                                    |
| V1, true apexes, straight ridges | 96 %            | about 17.3 MB                 | 0.42 %                | 0.11 %           | Same faces, edges at 0.25 m, no seams                                                                             |
| V2, welded graph                 | 71 %            | about 12.7 MB                 | 0.66 %                | 0.14 %           | Shared vertices stored once; faces within 3° and 15 cm merge; eaves and steps at 0.5 m; apexes solved from planes |
| V3, small faces absorbed         | 66 %            | about 11.8 MB                 | 1.00 %                | 0.18 %           | Faces under 2 m² join a neighbour; steps at 0.75 m                                                                |
| V4, on the City's eaves          | 63 %            | about 11.3 MB on top of boxes | 1.15 %                | 0.20 %           | Eaves are the DataSF outline the box layer already carries; only the ridge graph is stored                        |
| **V5, compact**                  | **54 %**        | **about 9.7 MB**              | 1.42 %                | 0.26 %           | 0.75 m eaves, 1 m steps, faces under 3 m² absorbed, 10 cm grid, 0.05 % slopes                                     |
| L3, 1 m outlines                 | 69 %            | 12.4 MB                       | 2.01 %                | 1.36 %           | Staircases cut, but seams open and dormers deform                                                                 |

City sizes scale L2's measured 18.0 MB of roofs by each variant's byte ratio over the three
sites. The same scaling puts L3 at 12.4 MB against 13.1 MB measured citywide, so read them as
about ±5 %. With the unroofed boxes and roads, a whole-city asset is about 18.1 MB at V1, 13.5 MB
at V2, 12.6 MB at V3 and 10.5 MB at V5. V4 suits a tiered product: the 5.7 MB box layer plus
11.3 MB of roofs. Apex error against the lidar's 99.5th-percentile roof height is the same for
every variant as for full detail (90th percentile 0.9–1.8 m, set by chimneys and rooftop
equipment). "Points uncovered" is the share of good roof points no face covers, mostly the seams
in L2 and L3.

## Right-angle roofs (W1–W5)

In the viewer the V variants showed edges at odd angles, especially on downtown's stepped flat
roofs. The second round added three rules: the eaves are always the DataSF outline; steps between
roof tiers run along or across each building's own axis, taken from its DataSF footprint and so
from the street grid; and W4 and W5 put the remaining step vertices on a lattice aligned with
those axes. Apexes and ridge ends are exempt. Snapping the planes themselves to the axes was tried
and dropped: it moved faces up to 10 cm and broke more lidar points for no measurable gain in
regularity.

| Variant                               | Roofs on boxes   | Alone, whole city | Edges off axis | Points broken > 25 cm | Points uncovered |
| ------------------------------------- | ---------------- | ----------------- | -------------- | --------------------- | ---------------- |
| L2, 0.25 m outlines                   | —                | 18.8 MB           | 15.1 %         | 0.14 %                | 0.07 %           |
| V4 (previous round)                   | about 11.5 MB    | about 13.7 MB     | 21.3 %         | 1.15 %                | 0.20 %           |
| W1, right-angle steps at 0.25 m       | about 13.2 MB    | about 15.5 MB     | 13.5 %         | 1.00 %                | 0.37 %           |
| W2, coplanar faces merged, 0.5 m      | about 12.2 MB    | about 14.5 MB     | 14.4 %         | 1.28 %                | 0.39 %           |
| W3, faces under 2 m² absorbed, 0.75 m | about 11.3 MB    | about 13.7 MB     | 15.1 %         | 1.33 %                | 0.22 %           |
| W4, 0.25 m axis lattice               | about 10.4 MB    | about 11.7 MB     | 15.4 %         | 1.86 %                | 0.66 %           |
| **W5, compact, 0.5 m lattice**        | **about 8.7 MB** | **about 9.8 MB**  | 16.1 %         | 2.29 %                | 0.50 %           |
| L3, 1 m outlines                      | —                | 13.8 MB           | 30.1 %         | 2.01 %                | 1.36 %           |

"Roofs on boxes" is the roof layer when the 5.7 MB box layer (footprints, heights, roads) ships
too, so the eaves cost nothing. "Alone" adds roads and boxes for the unroofed buildings. "Edges
off axis" is the share of roof edge length not within 3° of the building's axis, its
perpendicular or 45°; what remains is mostly hips between unequal slopes and irregular lot lines
in the DataSF outline. Same three sites, same lidar test and same ±5 % scaling as above.

## What this suggested

- **Ship vectors, not meshes, for a whole-city download.** The LoD1 vector layer is 50× smaller
  than the roofed meshes and holds the same footprints. Meshes belong in streamed tiles, where a
  client fetches only what is near the view.
- **The LoD2 bake fits in about 19 MB** as planes and outlines, and in about 10.5 MB as a welded
  roof graph with true apexes (V5): smaller than L3, fewer broken lidar points, no seams.
- **Turn meshopt on in `city-tiles`.** It cut every sampled layer 2–10× (ground 4.8×, L13 tiles
  5–6×) with no visible loss at 14 bits; the roofed epoch would come in under 200 MB instead of
  about 470 MB.
- **Replace the ground mesh with a heightmap** wherever the client can build the grid itself:
  about 5 MB at 4 m against 105 MB.
- **Rebake `city-tiles`** so the served epoch carries the roofs. A citywide roofed bake on arrow ran
  4 h 20 min at 1.7–2.2 GB without finishing and was stopped when the server went into swap; run
  it with the server quiet, or on a Mac.

## Method

- Gerardus on arrow was updated to `main` (b928e79, #89, docs only) before these runs, through
  `gerardus-deploy update`, which backed up the cells first.
- Vector measurements ran over the whole city from Gerardus's caches and the roof bake, plus Curbs
  and Islands and right-of-way polygons fetched whole from data.sf.gov. Footprints are
  deduplicated by id and ordered along the S2 Hilbert curve before delta coding. Simplification is
  Douglas–Peucker with topology kept; losses are symmetric-difference area and Hausdorff distance
  against the source.
- Mesh measurements come from `gerardus city-tiles` on the three sample cells, with and without
  roofs, each layer repacked with gltfpack 0.22 (`-cc`, `-vp 12`, `-vp 10`). Scaled rows multiply
  the samples' ratios onto the live epoch's measured per-layer sizes (L16 boxes 95.1 MB, ground
  105.0 MB, streets 6.9 MB). In the samples, roofs made L16 buildings 3.07× larger than boxes.
- Scripts and outputs are on arrow in `/home/banshee/city-assets-20261118/` (`scripts/`,
  `out/*.json`).
