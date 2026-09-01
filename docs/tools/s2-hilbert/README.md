# S2 Hilbert SVG generator

This documentation tool turns real S2 cells into editable SVG geometry. It generates the 64-cell traversal used by the infographic, four quieter 16-cell orientation examples, and a compact orientation legend. The production application and its geographic indexing code are not involved.

S2 orders cells along a Hilbert space-filling curve. Nearby positions usually receive nearby CellIDs, which makes spatial indexes rather more useful than a bag of unrelated numbers. Every level divides one cell into four children. An L10 parent therefore contains 16 L12 cells and 64 L13 cells.

The curves come from those CellIDs. The tool asks `s2js` for the descendants, sorts them by the library's Hilbert traversal, obtains each cell's centre and four vertices, projects the WGS84 coordinates through Web Mercator, and fits the result to an SVG `viewBox`. There is no stock curve waiting backstage to be rotated until it looks convincing.

## Dependency choice

The tool pins [`s2js`](https://www.npmjs.com/package/s2js) 1.44.0. It is an Apache-2.0 TypeScript port of Google's S2 geometry library with native `bigint` CellIDs, canonical token parsing, hierarchy operations, cell centres and vertices, and ESM support for Node.js and browsers. `@radarlabs/s2` provides capable native bindings, but its N-API implementation cannot run in a browser. Older pure-JavaScript packages either expose less of S2 or have been dormant for years.

`s2js` represents a CellID as a `bigint`, not as a class instance. The browser-safe modules keep decimal CellIDs and canonical tokens in the model so it remains JSON-friendly. The library's public `Cell.orientation` value uses the S2 swap and invert bits. The legend verifies all four values against actual parent cells before it writes a path.

## Canonical tokens are not display tokens

`808581` is the canonical S2 token. Documentation writes it as `80858-1` to make the shared five-character prefix visible. The hyphen carries no S2 meaning and must not reach the library.

The core API accepts canonical tokens only. The CLI accepts either form, but converts a display token at the command boundary before calling the core. Hierarchy always comes from CellID operations, never from string truncation.

## Generate the infographic assets

From the repository root:

```bash
make render-s2-hilbert
```

That installs the root documentation dependencies if they are missing, then regenerates every checked-in asset. The underlying scripts are also available directly:

```bash
pnpm install
pnpm run s2-hilbert:assets
```

The generator writes these files under `generated/`:

- `80858-1-l13-detailed.svg`: all 64 L13 descendants of `808581`
- four `l12-coarse.svg` files: 16 real L12 descendants for each infographic parent
- `s2-hilbert-orientations.svg`: one actual S2 example for each orientation bit state
- `s2-l10-quad-composite.svg`: all four L10 parents joined along their real shared edges
- `80858-1-l13.json`: the exact ordered tokens, decimal CellIDs, and centres

The output contains no timestamps, so identical inputs produce byte-for-byte identical files.

## Run the CLI

Generate a detailed SVG and the ordered child data:

```bash
npm run s2-hilbert -- \
  --parent 808581 \
  --target-level 13 \
  --land-mask docs/tools/s2-hilbert/land/sf-shoreline-and-islands.geojson \
  --output /tmp/80858-1-l13.svg \
  --json /tmp/80858-1-l13.json
```

Generate a coarse orientation example:

```bash
npm run s2-hilbert -- \
  --parent 80858-7 \
  --target-level 12 \
  --width 720 \
  --height 720 \
  --output /tmp/80858-7-l12.svg
```

Run `npm run s2-hilbert -- --help` for the small remainder of the options. `--view-box`, `--padding`, `--show-labels`, and the cell visibility flags change presentation without changing the S2 traversal.

## Why the four coarse curves differ

S2 rotates and reflects the Hilbert curve as it descends the hierarchy, so a child's traversal depends on where its parent sits. `Cell.orientation` reports that state as two bits: swap and invert. The four infographic parents come out as:

| Parent    | Orientation | First L12 child | Last L12 child | Net travel |
| --------- | ----------- | --------------- | -------------- | ---------- |
| `80858-1` | 2 (invert)  | `8085801`       | `808581f`      | north-west |
| `80858-7` | 0           | `8085861`       | `808587f`      | south-west |
| `808f7-d` | 2 (invert)  | `808f7c1`       | `808f7df`      | north-west |
| `808f7-f` | 1 (swap)    | `808f7e1`       | `808f7ff`      | north-east |

Two of the four share orientation 2, so **the reference set covers three of the four states, not all four**. Orientation 3 (swap + invert) is absent. This was checked three independent ways, which all agree:

1. `Cell.orientation` reports `2, 0, 2, 1`.
2. `cellid.faceIJOrientation` reports the same four values.
3. The geometry itself: reading the 16 L12 centres as a 4×4 lattice in S2 face coordinates, `80858-1` and `808f7-d` produce byte-identical traversals — same start corner, same end corner, same fifteen steps.

Read the lattice in S2's face coordinates, not in latitude and longitude. The cells are sheared with respect to lat/lng, so the 16 centres occupy 4 distinct longitudes but 16 distinct latitudes, and a lat/lng grid will not line up. `cellid.faceIJOrientation` is also the wrong tool for this: on a non-leaf cell it returns a corner leaf rather than a lattice index. `cellid.faceSiTi` gives the clean 4×4.

The legend therefore borrows `80858-5` for the swap-and-invert panel rather than mislabel a curve to fill the gap. The panels are captioned by orientation name only — the cell id tags were removed — but each still records its source cell in `data-s2-token` for anyone checking the geometry. `test/hierarchy.test.mjs` pins all of this so it cannot drift.

The cells render as parallelograms rather than squares. That is not a projection defect: S2 cells are faces of a projected cube, and this far from a face centre their edges do not align with meridians or parallels. The shear is already present in the raw WGS84 vertices before Web Mercator sees them.

## Reading the direction

Every straight run carries one chevron at its midpoint, pointing the way the traversal travels, so direction is legible without tracing the line by eye. The start is a green ring, the end a red arrowhead, and both are labelled; the labels are pushed along the traversal's own direction — backwards from the start, onwards past the end — so they never land on the arrowhead. Chevrons appear on the detailed view, on all four coarse L10 views, and in every legend panel.

Chevrons follow the same lit/dim styling as the line beneath them, one classification per straight run. Turn the labels off with `showEndpointLabels: false` when embedding at small sizes.

## Choosing which cells are lit

The coastline is one way to decide which parts of the traversal are emphasised. A per-cell selection is the other, and it is the one to reach for when the answer is editorial rather than geographic.

A selection is a list of objects, one per L13 cell:

```json
[
  { "index": 0, "token": "80858004", "displayToken": "80858-004", "enabled": false },
  { "index": 1, "token": "8085800c", "displayToken": "80858-00c", "enabled": true }
]
```

`token` and `enabled` are the load-bearing fields; `index` and `displayToken` are there to make the file readable while editing. A segment is lit when **both** of the cells it joins are enabled, so a run of consecutive opted-in cells lights up as one continuous stretch.

The committed selection lives at `selection/80858-1-l13-cells.json` — hand-edited input, not generated output, which is why it sits outside `generated/`. The first run seeds it from the coastline pass and reports how many cells it enabled. Every later run reads it back, so **regeneration never overwrites your edits**. To opt more cells in, flip `enabled` to `true` and run `make render-s2-hilbert` again.

To seed a fresh selection for a different parent or level:

```bash
npm run s2-hilbert -- \
  --parent 808581 \
  --target-level 13 \
  --land-mask docs/tools/s2-hilbert/land/sf-shoreline-and-islands.geojson \
  --seed-selection /tmp/80858-1-l13-cells.json
```

Then render with it:

```bash
npm run s2-hilbert -- \
  --parent 808581 \
  --target-level 13 \
  --cell-selection /tmp/80858-1-l13-cells.json \
  --output /tmp/80858-1-l13.svg
```

A selection takes precedence over `--land-mask`. A selection naming a cell that is not a descendant of the requested parent is refused rather than quietly ignored. The SVG records which route produced it in `data-classification-source`, which reads `selection`, `coastline`, or `none`.

The rendered group ids stay `hilbert-land` and `hilbert-water` in both modes, so the infographic's styling contract does not change when you switch. In selection mode read them as lit and dim.

## The four-cell composite

`generate-composite.mjs` draws all four L10 parents as one figure:

```bash
make render-s2-composite
```

`make render-s2-hilbert` runs it too, so one command still refreshes everything.

The four cells are genuine neighbours, and the composite proves it rather than assuming it. Every panel goes through **one shared projection** — `projectTraversals` fits all of them together instead of fitting each separately — so `80858-1` and `80858-7` meet along their real shared edge because S2 puts them there. Nothing is nudged into place. The generator refuses to write the file unless it finds exactly four shared edges, and `compositeAdjacency` reports the tiling:

| Relation | Pairs                                                                              |
| -------- | ---------------------------------------------------------------------------------- |
| Edge     | `80858-1`–`80858-7`, `80858-1`–`808f7-f`, `80858-7`–`808f7-d`, `808f7-d`–`808f7-f` |
| Corner   | `80858-1`–`808f7-d`, `80858-7`–`808f7-f`                                           |

Every cell draws the same two runs — the full 64-step L13 traversal in blue through the cell centres, and the 16-step L12 run in brown. Only the **weight** changes:

| Cell                            | Brown (16-step) | Blue (L13)                                     |
| ------------------------------- | --------------- | ---------------------------------------------- |
| `80858-1` (hero)                | light           | heavy across the shaded cells, light elsewhere |
| `80858-7`, `808f7-d`, `808f7-f` | solid           | light throughout                               |

The shaded cells come from the selection file, so the heavy blue traces exactly the cells that are enabled. Both runs join cell **centres**; the shading is a fill behind them, never a substitute for the line.

All four cells are labelled with their orientation name, derived from `Cell.orientation` and never hard-coded per token. Labels sit at the centre of each cell and are rotated onto the cell's own slant, read from the direction of its top edge, so they lie along the cell instead of cutting across it. Anchoring a label to the topmost _vertex_ does not work: the cells are sheared, so that vertex can sit far to one side and drift over a neighbour.

### Joining the cells into one run

The four 16-step runs are chained end to start so the eye can follow one path across the figure, and only the two ends of that whole chain keep a marker — interior junctions are where one cell's end meets the next cell's start, and marking both would put a full stop and a capital letter in the middle of a sentence.

The order is not read off the S2 curve, because it cannot be. **These four cells are geographic neighbours but are not all consecutive on the Hilbert curve**, so no single S2 traversal runs through just these four. `buildPanelChain` therefore picks the shortest end-to-start chain from the projected geometry, exhaustively over all orderings, which lands on `80858-7 → 808f7-d → 808f7-f → 80858-1` — the familiar Hilbert U through a 2×2.

Each junction records what it actually is, and the drawing does not pretend otherwise:

| Junction              | Span          | Drawn        | `next()` in S2? |
| --------------------- | ------------- | ------------ | --------------- |
| `80858-7` → `808f7-d` | about 4 steps | faint dashed | no              |
| `808f7-d` → `808f7-f` | one step      | solid        | **yes**         |
| `808f7-f` → `80858-1` | one step      | solid        | no              |

Two junctions span exactly one coarse step, so they read as a continuation and are drawn solid. The third spans four and is drawn as a faint dashed jump rather than implying a continuity that does not exist. Only `808f7-d` → `808f7-f` is a true S2 succession — `next(808f7d) === 808f7f` — and every connector carries `data-s2-consecutive` and `data-continuous` so the distinction survives into the file.

### Changing which tiles are shown

The hero's tiles come from `selection/80858-1-l13-cells.json`, the same file described above. To show more, flip `enabled` to `true` and regenerate:

```bash
make render-s2-hilbert
```

The composite reads the file on every run and never rewrites it, so hand-edits are the point rather than a hazard. The seed currently enables 20 of the 64 cells; the generator prints the count each run, and a selection naming a cell that is not an L13 descendant of the panel's own parent is refused rather than silently dropped.

## Land and water remain one journey

The optional land mask accepts GeoJSON `Polygon`, `MultiPolygon`, and collections of either. The tool projects the mask through the same Web Mercator transform as the S2 geometry, finds every coastline intersection along each centre-to-centre segment, and classifies the pieces by polygon containment. Land is drawn at full opacity. Water is drawn at 30 per cent opacity, not removed, so the traversal remains continuous.

The checked-in example mask is a roughly 20-metre simplification of the public-domain [DataSF SF Shoreline and Islands dataset](https://data.sfgov.org/d/rgcx-5tix). It contains San Francisco's mainland and islands, including Treasure Island and Yerba Buena Island. A bridge line is not land merely because people can drive over the water beneath it. Replace the file with a more suitable Polygon or MultiPolygon when another coastline, resolution, or definition of land is required.

## Reuse in a browser

`index.mjs` exports the hierarchy, projection, mask, model, and SVG functions without importing `fs`, streams, argument parsers, or `process`. A future browser explorer can call `buildS2HilbertModel` with a canonical parent and target level, then pass the model to `renderHilbertSvg` or to another renderer. The CLI and batch generator are the only Node-specific layers.

Web Mercator preserves the familiar web-map appearance, not area or distance. Latitudes are clamped to the projection's normal limit. Longitudes are unwrapped around the selected parent's centre so a local cell does not split at the antimeridian. S2 edges are represented by their four authoritative vertices; at these levels the projected edge curvature between vertices is negligible for the documentation graphic.

## Tests

Run the focused suite with:

```bash
make test-s2-hilbert
```

`make test` includes it. The suite is also available as `pnpm run test:s2-hilbert`.

The suite checks descendant counts, parentage, monotonic Hilbert order, uniqueness, token round-trips, deterministic models and SVG, projection behaviour, all four orientation states, and land-mask clipping. It also checks that Treasure Island is land while a point on the Bay Bridge crossing remains water under the supplied shoreline mask. Geography is data here, as it should be; the Hilbert algorithm has quite enough responsibilities already.
