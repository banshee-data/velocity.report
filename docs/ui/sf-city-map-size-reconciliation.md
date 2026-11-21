# San Francisco city map: size reconciliation

Why the `/sf/` city tiles are 30,169,632 bytes when the [city map record](sf-city-map.md) quoted
about 9.5 MB for level S6, traced figure by figure. The two numbers measure different things: 9.5 MB
was an estimate for fewer layers in a compact binary encoding under Brotli. The tiles are those
layers plus the ground, signposts and kerb widths, as JSON under gzip. Every figure below is
labelled **measured**, **estimated** or **recorded** (quoted from the study without a saved
output), with the command that produces it.

- **Status:** Audit of `main` at `9ff3666e` (tiles built 2026-11-20T18:26Z); no tile, geometry or
  HUD change
- **Tool:** [tools/sf-size-audit/](../../tools/sf-size-audit/README.md); its outputs stay local
  in `tools/sf-size-audit/results/` and the figures used are recorded here

## Summary

The 9.5 MB cannot be reproduced as a measurement because it never was one. It added an S6 roof
figure scaled from three 520 m windows to trees and lidar-only buildings scaled by area, plus
measured roads, in the study's welded, plane-coded binary encoding under Brotli 11. It left out
the ground, signposts, street-name tables and kerb widths.

Re-measured on the shipped geometry, those same layers come to about 8.2 MB (estimated). The game's
extra layers add about 2.7 MB, and the encoding, serialisation and compression in use account for
the rest of the 30.2 MB.

| Step                                                                                        | Payload (bytes) | Change (bytes) | Label     |
| ------------------------------------------------------------------------------------------- | --------------: | -------------: | --------- |
| S6 budget: roofs, unroofed boxes, lidar-only buildings, trees, centrelines, kerbs           |       9,464,631 |                | estimated |
| Same layers re-measured on the shipped geometry, study encoders                             |       8,173,887 |     −1,290,744 | estimated |
| Add the game's layers outside the budget: ground, roads resampled with widths, signs, names |      10,896,636 |     +2,722,749 | estimated |
| Trees at the shipped 0.1 m positions instead of 0.5 m                                       |      11,080,550 |       +183,914 | estimated |
| Buildings in the audit's codec instead of the study's welded, plane-coded encoder           |      14,045,545 |     +2,964,995 | measured  |
| Same layers as one Brotli stream                                                            |      14,042,291 |         −3,254 | measured  |
| JSON instead of compact binary, one citywide bundle, Brotli 11                              |      21,572,373 |     +7,530,082 | measured  |
| 157 tile files instead of one bundle, Brotli 11                                             |      22,014,251 |       +441,878 | measured  |
| gzip 9 instead of Brotli 11: **the shipped tiles**                                          |  **30,169,632** |     +8,155,381 | measured  |

Rows 2 to 4 rest on one estimate: the shipped buildings in the study's encoder, 6,909,066 bytes,
from the audit codec's measured 9,828,576 bytes divided by a calibration ratio of 1.4226 measured
on the study's three windows (see [Encoders](#encoders)). The other terms in those rows are measured.
From row 5 on, every row is a whole-payload measurement of the same data. The order of the steps
is a choice; compression is not additive, so the changes hold only along this path.

## Where 9.5 MB came from

`scripts/relabel.py` in the study's work directory (on arrow-worker,
`/home/banshee/city-assets-20261118`) computed the S6 total as
`18.0 × S6 ÷ L2 + 0.81 + trees + lidar-only`. `study_crosscheck.py` recomputes it term by term from
the study's saved outputs:

| Term                                    | Bytes or factor | Source                                                                                                                                                             | Label     |
| --------------------------------------- | --------------: | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------- |
| L2 roofs, whole city                    |      18,012,884 | `out/roofs-city.json`: every baked roof face, 0.25 m outline tolerance, 5 cm grid, one citywide stream, Brotli 11, 16 MiB window; used as 18.0 MB                  | measured  |
| L2 roofs, three windows                 |          95,227 | `viewer/data2/summary-<site>.json`, unwelded rings plus boxes, Brotli 11                                                                                           | measured  |
| S6 roofs, three windows                 |          33,914 | the same summaries: `enc_graph2` welded roof graph, implicit apex and ridge-end vertices, plane code (0.5° / 0.25° / 2 cm), 0.5 m lattice, DataSF eaves, Brotli 11 | measured  |
| **S6 roofs, whole city**                |   **6,410,493** | 18.0 MB × 33,914 ÷ 95,227: the windows' S6-to-L2 ratio applied to the city                                                                                         | estimated |
| Boxes for the 11,756 unroofed buildings |         350,000 | quoted in the round-1 report; no saved output records the computation                                                                                              | recorded  |
| Centrelines                             |         162,307 | `out/roads-city.json`: 15,671 lines, 54,728 vertices, 0.1 m grid, with names, class and one-way, Brotli 11                                                         | measured  |
| Kerbs                                   |         298,610 | `out/roads-city.json`: 0.25 m tolerance, 5 cm grid, Brotli 11                                                                                                      | measured  |
| Area factor                             |          149.16 | 121 km² of city over three 0.2704 km² windows                                                                                                                      | estimated |
| **Trees, whole city**                   |   **1,607,964** | 10,780 bytes for 4,502 trees in the windows (0.5 m positions, 1 m height and crown) × 149.16, which implies 671,526 trees                                          | estimated |
| **Lidar-only buildings, whole city**    |     **636,175** | 4,265 bytes in the windows × 149.16                                                                                                                                | estimated |
| **Total**                               |   **9,464,631** | about 9.5 MB                                                                                                                                                       | estimated |

Only the L2 roofs, centrelines and kerbs were measured over the whole city. The S6 roofs, trees and
lidar-only buildings were scaled from 0.81 km² of the city that over-weights downtown. The budget has no ground, signposts, street-name tables or kerb widths: the study
listed the ground as extra (2.4–5 MB as a heightmap), and assumed the client would derive widths
from centrelines and kerbs.

Each layer was compressed as its own citywide Brotli stream and the stream sizes were added. No
single 9.5 MB file was written, and no decoder for `enc_graph2` exists, so the S6 roof bytes have
never been round-tripped.

## What ships

`node tools/sf-size-audit/audit.mjs` counts and measures the 157 tiles. Node's gzip 9 per tile
reproduces the committed files byte for byte (30,169,632 bytes), and the manifest's byte counts
sum to the same figure.

| Layer              | Shipped                                                                                                                                                   | In the 9.5 MB budget                                                     | Gzip ablation (bytes) |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ | --------------------: |
| Buildings          | 176,866 (1,296 lidar-only); 581,152 face rings, 3,781,835 ring vertices (3,431,828 distinct per building), each with its own height, 0.1 m grid, unwelded | 165,267 baked buildings as a welded plane-coded graph; boxes; lidar-only |            19,047,936 |
| Ground             | 157 grids of 192 × 192 nodes at 8 m: 5,787,648 nodes over 370 km², of which 2,886,519 (49.9 %) lie nearer another tile's origin and 1,174,472 are gaps    | not included                                                             |             4,873,043 |
| Roads              | 15,671 lines resampled every 3 m to 657,659 vertices, 15,494 with left and right kerb distances per vertex                                                | 15,671 lines, 54,728 vertices, no widths                                 |             3,403,883 |
| Trees              | 343,046 at 0.1 m positions, 1 m height and crown; 15,689 share a position with another tree in the same tile                                              | 671,526 (area-scaled) at 0.5 m positions                                 |             1,994,571 |
| Kerbs              | 10,669 lines clipped to their cells, 141,129 vertices                                                                                                     | the same source at 0.25 m tolerance                                      |               648,054 |
| Signposts          | 15,046 posts at 7,523 intersections, with blade angles                                                                                                    | not included                                                             |               124,544 |
| Street-name tables | 4,629 entries in 157 per-tile tables, 2,438 distinct names                                                                                                | inside the centreline stream                                             |                52,165 |

The ablation is how much the gzipped tiles shrink without that layer (measured). The figures do
not add up to the whole (30,144,196 against 30,169,632) because compression is not additive.

Duplication is limited to the ground: buildings, roads, kerbs and signposts each belong to one
tile, and no tree appears in two tiles. Each tile's ground grid spans 1,536 m around its origin
while its cell is about 1.1 km across, so about half of every grid repeats a neighbour's. The
15,689 trees stacked on another tree's position are a geometry matter, not packaging.

## How much each difference explains

| Difference                                            | Measured on                    | Effect (bytes)          | Label     |
| ----------------------------------------------------- | ------------------------------ | ----------------------- | --------- |
| gzip 9 against Brotli 11, per tile                    | shipped JSON                   | 30,169,632 → 22,014,251 | measured  |
| Per-tile files against one bundle, Brotli 11          | shipped JSON                   | 22,014,251 → 21,572,373 | measured  |
| Per-tile files against one bundle, gzip 9             | shipped JSON                   | 30,169,632 → 30,189,966 | measured  |
| JSON against the audit's compact codec, Brotli 11     | all shipped layers, one stream | 21,572,373 → 14,042,291 | measured  |
| Ground with each place once                           | compact, Brotli 11             | 2,280,671 → 1,093,893   | measured  |
| Roads resampled with widths against plain centrelines | compact, Brotli 11             | 518,834 → 304,982       | measured  |
| Trees at 0.1 m against 0.5 m positions                | compact, Brotli 11             | 991,136 → 807,222       | measured  |
| Buildings, audit codec against the study's encoder    | the study's three windows      | ratio 1.4226            | measured  |
| Buildings, audit codec against the study's encoder    | whole city                     | 9,828,576 → 6,909,066   | estimated |
| Trees in the budget against the shipped trees         | the study's tree encoder       | 1,607,964 → 805,155     | measured  |

Compression is the largest single factor and costs no geometry: Brotli 11 is 27 % smaller than gzip
9 on the same tiles. Bundling the tiles helps Brotli by 2 % and gzip not at all, whose 32 KiB window
cannot see across tiles. Serialisation is the next: JSON's decimal text and repeated brackets cost
54 % over the compact codec under the same Brotli. Then come the study's roof encoder (an estimated
2.9 MB of buildings), the ground's overlap (1.2 MB) and the 3 m road resampling.

The budget over-estimated in two places, which hid part of the gap: it scaled 4,502 window trees
to 671,526 where the city tiles hold 343,046 (0.80 MB too many), and its buildings come out 0.49 MB
above the calibrated estimate for the shipped ones.

## Encoders

The audit's codec writes zig-zag varint deltas one column at a time, the scheme of the study's
`delta_stream`, and keeps everything the game reads: each face's outer ring and holes (18,051 faces
have holes) and the ground's gaps, as run lengths. On the shipped tiles it agrees with the study's encoders (measured,
`study_crosscheck.py`): 807,222 against 805,155 bytes for the trees at 0.5 m, and 297,211 against
297,359 for the kerbs.

For roofs it is a simpler codec than the study's `enc_graph2`, which stores a welded graph, solves
apexes and ridge ends from the planes and codes each plane against the building's axis. The tiles
do not carry the planes, lattice or axis that encoder needs, so it cannot run on them.

Instead, `study_crosscheck.py` encodes the shipped buildings that fall in the study's three windows
with the audit codec and compares them with the study's S6 bytes for the same windows: 54,312
against 38,179 bytes over 729 buildings, a ratio of 1.4226 (measured). The ratio is an upper bound
on the encoder's advantage: the tile side also counts unroofed boxes, which the study's window
figure leaves out. Welding the audit codec's vertices into one table per building gains nothing on
its own (9,872,142 against 9,828,576 bytes): the saving needs the plane code and implicit vertices.

## The download readout

The readout from #1089 is consistent; no correction to its arithmetic is needed. Checked against
the code and the live site on 2026-11-20:

| Figure                                       |      Bytes | How                                                                                                                                                                             | Label     |
| -------------------------------------------- | ---------: | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------- |
| City tiles (map only)                        | 30,169,632 | manifest byte counts; equal to the files, and to the live `content-length` (tiles are served as `application/gzip` without a content encoding, so the encoded body is the file) | measured  |
| Other assets, build-time gzip 9 estimate     |    449,948 | `sfDownloadAssets()`: manifest 3,297; quest route 7,252; game modules 22,111; three.js 417,288                                                                                  | estimated |
| Other assets, live encoded bodies            |    461,437 | `curl` of each asset with `Accept-Encoding: gzip, br`; the host answered gzip, never Brotli                                                                                     | measured  |
| Complete asset download shown before timings | 30,619,580 | tiles plus the estimate, marked `≈`                                                                                                                                             | estimated |
| Complete asset download from live sizes      | 30,631,069 | tiles plus the live encoded bodies                                                                                                                                              | measured  |
| Page document, not counted                   |      4,159 | live gzip body of `/sf/`; excluded by design, with the favicon                                                                                                                  | measured  |
| On disk (the "previous 32.4 MB")             | 32,400,188 | tile directory with the uncompressed manifest, plus the uncompressed modules, quest route and three.js                                                                          | measured  |

What the readout does, and why it holds:

- It counts each URL once at its encoded body size, so cache hits, `304` revalidations and retries
  do not change the total, and headers are excluded.
- Bytes are decimal: `formatBytes` divides by 10⁶ and 10³.
- "Loaded" is the compressed size of the resident tiles, not memory; heap and GPU memory are not
  measured. The map-only figure (city tiles) is kept apart from the complete download.
- The build estimates text assets 11,489 bytes (2.5 %) below the live gzip, because the host
  compresses less hard than level 9. The browser replaces each estimate with its measured size
  once seen.
- Resource Timing keeps 250 entries by default, so a long drive can stop recording new tiles. They
  then keep their manifest sizes, which are exact, so the total does not drift.

## Is 9.5 MB achievable

Not with the current tile format, and not proven for any format. With every layer the game needs
and the current geometry:

| Payload                                                                         |       Bytes | Label     |
| ------------------------------------------------------------------------------- | ----------: | --------- |
| Shipped: JSON, 157 files, gzip 9                                                |  30,169,632 | measured  |
| Same JSON, Brotli 11                                                            |  22,014,251 | measured  |
| Compact codec, all layers, one Brotli stream                                    |  14,042,291 | measured  |
| The same with the ground kept once per place                                    |  12,854,778 | measured  |
| The same with buildings in the study's encoder (−2,963,076 at the window ratio) | ≈ 9,892,000 | estimated |
| The same with trees at 0.5 m                                                    | ≈ 9,708,000 | estimated |

About 9.5–10 MB is plausible only if all of these are built:

1. A binary tile format with a decoder in `sf-drive.js`, replacing JSON (about 7.6 MB measured).
2. The study's welded, plane-coded roof encoding, with the decoder that was never written and a
   round-trip test against the current roofs (about 2.9 MB estimated). The tile bake would have to
   keep the planes, lattice and axis it needs.
3. Ground stored once per place: grids cut to each tile's own cell, or one citywide heightmap
   (about 1.2 MB measured).
4. Brotli at quality 11 for the payload (about 27 % against gzip 9, measured). The live host
   answered gzip only, even when Brotli was accepted, so this needs pre-compressed files and a
   client-side Brotli decoder; browser support for decoding them was not tested here. Under gzip,
   the compact layers measure 5 to 20 % larger than under Brotli.

Optional further cuts: trees at 0.5 m (0.18 MB estimated), removing the 15,689 stacked trees, and a
coarser or smoother ground code.

## Missing evidence

- No whole-city measurement of the shipped buildings in `enc_graph2`. The 6,909,066-byte figure
  rests on 729 buildings in three windows.
- No decoder for `enc_graph2`, so its bytes have not been shown to decode to the same roofs.
- The 0.35 MB for unroofed boxes is quoted in the round-1 report without a saved output.
- No test of a Brotli payload on the live host, or of the browsers' ability to decode one.

## Reproduce

From a checkout (tiles are in the repository):

```sh
node tools/sf-size-audit/audit.mjs --out tools/sf-size-audit/results/sf-size-audit.json
```

On arrow-worker, in the study's work directory, for the budget terms, the study's encoders on the
shipped tiles, and the window calibration:

```sh
cd /home/banshee/city-assets-20261118
venv/bin/python -I <repo>/tools/sf-size-audit/study_crosscheck.py <repo>/public_html/src/sf/tiles \
  > <repo>/tools/sf-size-audit/results/study-crosscheck.json
```

The readout's inventory and live sizes:

```sh
cd public_html && node -e 'const a=require("./sf-asset-sizes.cjs").sfDownloadAssets(); console.log(a, a.reduce((s,x)=>s+x.bytes,0))'
curl -sS -o /dev/null -D - -H 'Accept-Encoding: gzip, br' https://velocity.report/sf/tiles/80858064.json.gz
curl -sS -o /dev/null -w '%{size_download}\n' -H 'Accept-Encoding: gzip, br' https://velocity.report/sf/tiles/manifest.json
```

The live module path changes with each build; take it from the `/sf/` page's script tag.
