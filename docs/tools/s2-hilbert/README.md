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

Install the root documentation dependencies, then regenerate every checked-in asset:

```bash
pnpm install
pnpm run s2-hilbert:assets
```

The generator writes these files under `generated/`:

- `80858-1-l13-detailed.svg`: all 64 L13 descendants of `808581`
- four `l12-coarse.svg` files: 16 real L12 descendants for each infographic parent
- `s2-hilbert-orientations.svg`: one actual S2 example for each orientation bit state
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

## Land and water remain one journey

The optional land mask accepts GeoJSON `Polygon`, `MultiPolygon`, and collections of either. The tool projects the mask through the same Web Mercator transform as the S2 geometry, finds every coastline intersection along each centre-to-centre segment, and classifies the pieces by polygon containment. Land is drawn at full opacity. Water is drawn at 30 per cent opacity, not removed, so the traversal remains continuous.

The checked-in example mask is a roughly 20-metre simplification of the public-domain [DataSF SF Shoreline and Islands dataset](https://data.sfgov.org/d/rgcx-5tix). It contains San Francisco's mainland and islands, including Treasure Island and Yerba Buena Island. A bridge line is not land merely because people can drive over the water beneath it. Replace the file with a more suitable Polygon or MultiPolygon when another coastline, resolution, or definition of land is required.

## Reuse in a browser

`index.mjs` exports the hierarchy, projection, mask, model, and SVG functions without importing `fs`, streams, argument parsers, or `process`. A future browser explorer can call `buildS2HilbertModel` with a canonical parent and target level, then pass the model to `renderHilbertSvg` or to another renderer. The CLI and batch generator are the only Node-specific layers.

Web Mercator preserves the familiar web-map appearance, not area or distance. Latitudes are clamped to the projection's normal limit. Longitudes are unwrapped around the selected parent's centre so a local cell does not split at the antimeridian. S2 edges are represented by their four authoritative vertices; at these levels the projected edge curvature between vertices is negligible for the documentation graphic.

## Tests

Run the focused suite with:

```bash
pnpm run test:s2-hilbert
```

The suite checks descendant counts, parentage, monotonic Hilbert order, uniqueness, token round-trips, deterministic models and SVG, projection behaviour, all four orientation states, and land-mask clipping. It also checks that Treasure Island is land while a point on the Bay Bridge crossing remains water under the supplied shoreline mask. Geography is data here, as it should be; the Hilbert algorithm has quite enough responsibilities already.
