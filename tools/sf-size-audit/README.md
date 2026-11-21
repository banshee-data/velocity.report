# SF city tile size audit

Measures what the `/sf/` city tiles' bytes are made of, and what the same data costs under other
serialisations, compressions and packaging. It backs the reconciliation in
[sf-city-map-size-reconciliation.md](../../docs/ui/sf-city-map-size-reconciliation.md).

```sh
node tools/sf-size-audit/audit.mjs --out tools/sf-size-audit/results/sf-size-audit.json
```

It reads `public_html/src/sf/tiles/` and never writes there. Node's built-in codecs only: gzip
level 9 and Brotli quality 11. A full run takes about 15–25 minutes, nearly all of it Brotli 11
over the 100 MB-scale citywide payload. Outputs go to `results/`, which Git ignores: they are
measurement dumps, kept local like other experiment output, and the
[reconciliation](../../docs/ui/sf-city-map-size-reconciliation.md) records the figures it uses.

## What it reports

| Key           | What                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `shipped`     | The gzipped tile files as committed, the manifest's byte counts, and their decompressed JSON                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `compression` | The same JSON per tile under gzip 9 and Brotli 11, and as one citywide bundle under gzip 9 and Brotli 11 (16 MiB window)                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `layers`      | Per JSON key: its raw size, its size gzipped alone per tile (`standalone`), and how much the full tiles shrink without it (`ablation`). The two differ because compression is not additive; neither sums to the whole                                                                                                                                                                                                                                                                                                                                                   |
| `counts`      | Buildings, faces, ring and unique vertices, trees, roads, kerbs, signposts, names, ground nodes, gaps, and ground nodes nearer another tile's origin (the overlap between tiles' grids)                                                                                                                                                                                                                                                                                                                                                                                 |
| `compact`     | The same data in a compact binary encoding: zig-zag varint deltas per column, one citywide stream per layer, Brotli 11 with a 16 MiB window, as the asset study measured. `whole_*` keys are single streams over several layers. Every stream keeps what the game reads: buildings keep each face's outer ring and holes, the ground keeps its gaps (as run lengths). Buildings come unwelded (as shipped) or welded into one vertex table per building; trees come at the shipped 0.1 m or the study's 0.5 m positions; ground as shipped or with each place kept once |

`study_crosscheck.py` runs in the asset study's work directory on arrow-worker
(`/home/banshee/city-assets-20261118`), which holds the study's outputs and encoders. It
recomputes the S6 budget term by term, applies the study's tree and kerb encoders to the shipped
tiles, and calibrates the audit codec's buildings against the study's S6 roof bytes on its three
windows.

```sh
cd /home/banshee/city-assets-20261118
mkdir -p <repo>/tools/sf-size-audit/results
venv/bin/python -I <repo>/tools/sf-size-audit/study_crosscheck.py <repo>/public_html/src/sf/tiles \
  > <repo>/tools/sf-size-audit/results/study-crosscheck.json
```

The compact codec is this tool's own, written to match the study's `delta_stream` scheme; it is
not the study's welded roof-graph encoder (`enc_graph2`), which needs plane and lattice data the
tiles do not carry.
