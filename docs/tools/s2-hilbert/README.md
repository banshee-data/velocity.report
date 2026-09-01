# S2 Hilbert SVG generator

This documentation tool turns real S2 cells into editable SVG geometry. It generates a four-cell composite of adjacent L10 parents, four quieter 16-cell orientation examples, and a compact orientation legend. The production application and its geographic indexing code are not involved.

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

That installs the root documentation dependencies if they are missing, then regenerates every checked-in asset. `make render-s2-composite` rebuilds just the composite. The underlying scripts are also available directly:

```bash
pnpm install
pnpm run s2-hilbert:assets
pnpm run s2-hilbert:composite
```

The generator writes exactly three kinds of file under `generated/`:

- `s2-l10-quad-composite.svg`: all four L10 parents joined along their real shared edges
- four `*-l12-coarse.svg` files: 16 real L12 descendants for each parent
- `s2-hilbert-orientations.svg`: one actual S2 example for each orientation bit state

The output contains no timestamps, so identical inputs produce byte-for-byte identical files.

## Run the CLI

The CLI renders any parent at any level, independently of the checked-in assets:

```bash
npm run s2-hilbert -- \
  --parent 808581 \
  --target-level 13 \
  --cell-selection tools/s2-hilbert/selection/80858-1-l13-cells.json \
  --output /tmp/80858-1-l13.svg \
  --json /tmp/80858-1-l13.json
```

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

## Why the files carry attributes as well as a stylesheet

Every drawn element is given its own `fill` and `stroke` presentation attributes, and the same values are also written into a `<style>` block. This is deliberate belt and braces.

Illustrator, Inkscape and Figma routinely ignore an embedded stylesheet. A `<path>` that relies on CSS alone then falls back to the SVG defaults — filled black, no stroke — which turns every Hilbert curve into a solid blob and makes the file useless for design work. Presentation attributes render correctly everywhere.

CSS still wins wherever it is honoured, because a stylesheet rule beats a presentation attribute, so swapping `styles` for your own keeps working exactly as before. [style-tokens.mjs](style-tokens.mjs) holds one table per figure and generates both forms from it, so the two can never drift apart. A test opens each generated asset, discards the `<defs>`, and fails if any drawn element would render unstyled.

## Reading the direction

Every straight run carries one chevron at its midpoint, pointing the way the traversal travels, so direction is legible without tracing the line by eye. The start is a green ring, the end a red arrowhead, and both are labelled; the labels are pushed along the traversal's own direction — backwards from the start, onwards past the end — so they never land on the arrowhead. Chevrons appear on the detailed view, on all four coarse L10 views, and in every legend panel.

Chevrons follow the same lit/dim styling as the line beneath them, one classification per straight run. Turn the labels off with `showEndpointLabels: false` when embedding at small sizes.

## Choosing which cells are lit

Which parts of a traversal are emphasised is an editorial decision, and it is made in one place: a per-cell selection listing every L13 cell of the parent.

```json
[
  { "index": 0, "token": "80858004", "displayToken": "80858-004", "enabled": false },
  { "index": 1, "token": "8085800c", "displayToken": "80858-00c", "enabled": true }
]
```

`token` and `enabled` are the load-bearing fields; `index` and `displayToken` are there to make the file readable while editing. A segment is emphasised when **both** of the cells it joins are enabled, so a run of consecutive opted-in cells reads as one continuous stretch.

The committed selection lives at `selection/80858-1-l13-cells.json`. It is hand-maintained input, not generated output, which is why it sits outside `generated/`; nothing overwrites it. To change which cells are emphasised, flip `enabled` and run `make render-s2-hilbert` again.

A selection naming a cell that is not a descendant of the requested parent is refused rather than quietly ignored.

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
| `80858-1` (hero)                | solid           | heavy across the shaded cells, light elsewhere |
| `80858-7`, `808f7-d`, `808f7-f` | solid           | light throughout                               |

One brown, one weight, everywhere. The blue run is the only thing that changes.

The shaded cells come from the selection file, so the heavy blue traces exactly the cells that are enabled. Both runs join cell **centres**; the shading is a fill behind them, never a substitute for the line.

Orientation captions are off by default, since the figure is normally lettered in a design tool. Each panel still records its orientation in `data-orientation-label`, derived from `Cell.orientation` and never hard-coded per token. Passing `showLabel: true` prints them: centred in the cell and rotated onto its own slant, read from the direction of its top edge, so a caption lies along the cell instead of cutting across it. Anchoring one to the topmost _vertex_ does not work — the cells are sheared, so that vertex can sit far to one side and drift over a neighbour.

### Where the curve enters and leaves

The four cells are geographic neighbours, but the Hilbert curve does **not** run through them one after another. Each cell's true predecessor and successor are `cellid.prev` and `cellid.next` at its own level, and for three of the four both lie outside the mapped area entirely.

So only one link is drawn inside the figure, and the rest are stubs pointing at wherever the real neighbour actually is:

| Cell      | Enters from               | Leaves towards            |
| --------- | ------------------------- | ------------------------- |
| `80858-1` | `80857f`, from its right  | `808583`, up and left     |
| `80858-7` | `808585`, from above      | `808589`, to the left     |
| `808f7-d` | `808f7b`, from below      | **`808f7f`, on the page** |
| `808f7-f` | **`808f7d`, on the page** | `808f81`, to the right    |

`808f7-d` to `808f7-f` is the only true succession here — `next(808f7d) === 808f7f` — so it is the only join drawn between two cells, and **both** runs cross it: the brown and the blue alike. The other six are stubs that run off the page.

Links and stubs are drawn as further steps of the brown run — same colour, same weight, same chevron for direction — so a join reads as the line carrying on rather than as separate notation. A stub is exactly one step long and is snapped to one of the cell's own edge axes: the run only ever travels along those two directions, so a stub aimed straight at a distant neighbour's centre comes out at a diagonal that belongs to nothing else in the drawing. Snapping keeps it parallel to the run it continues while still leaving by the side the S2 neighbour is actually on.

That junction is also the one that gets de-duplicated. `808f7-d`'s exit and `808f7-f`'s entry are the same joint, so it is drawn once, as the exit; no cell draws an entry stub for a predecessor already on the page.

None of this is inferred from what happens to look close together. An earlier version chose the shortest end-to-start chain across the four cells, which produced a plausible but wrong answer: it invented a route where the curve simply leaves the frame. Every entry and exit now comes from `prev` and `next`, and each stub carries `data-neighbour`, `data-kind` and `data-bearing` so it can be checked against S2 directly.

### Changing which tiles are shown

The hero's tiles come from the selection file described above. Flip `enabled` and run `make render-s2-hilbert`; the composite reads the file on every run and never rewrites it, so hand-edits are the point rather than a hazard. The generator prints the enabled count each run.

## Reuse in a browser

`index.mjs` exports the hierarchy, projection, selection, model, and SVG functions without importing `fs`, streams, argument parsers, or `process`. A future browser explorer can call `buildS2HilbertModel` with a canonical parent and target level, then pass the model to `renderHilbertSvg` or to another renderer. The CLI and batch generator are the only Node-specific layers.

Web Mercator preserves the familiar web-map appearance, not area or distance. Latitudes are clamped to the projection's normal limit. Longitudes are unwrapped around the selected parent's centre so a local cell does not split at the antimeridian. S2 edges are represented by their four authoritative vertices; at these levels the projected edge curvature between vertices is negligible for the documentation graphic.

## Tests

Run the focused suite with:

```bash
make test-s2-hilbert
```

`make test` includes it. The suite is also available as `pnpm run test:s2-hilbert`.

The suite checks descendant counts, parentage, monotonic Hilbert order, uniqueness, token round-trips, deterministic models and SVG, projection behaviour, all four orientation states, id namespacing, cell selection, and the composite's adjacency and entry/exit links. Which cells are emphasised is data here, as it should be; the Hilbert algorithm has quite enough responsibilities already.
