# San Francisco street-speed dataset layout

This is the release layout for the Hugging Face dataset rooted at
`/Volumes/lidar/lidar/pcaps/sf-street-speeds`. It keeps evidence, products and reporting separate
without making readers descend through a calendar, S2 tree or one directory per
junction.

```text
README.md
manifest.json
raw/
  lidar/
  radar/
derived/
  scenes/
  tracks/
  maps/
  metrics/
labels/
  <capture stem>/<label set name>/
  <capture stem>/<label set name>/views/<view>/
reports/
```

`manifest.json` is the authoritative index for this first release. It is a
JSON array, deliberately shaped so it can later be written as
`manifest.parquet` without renaming fields or files. A Parquet file is not
required until a consumer needs columnar scans.

## Names and paths

Every capture-associated artifact starts with:

```text
<timestamp>_<s2-l13-display>_<site-slug>
```

For example:

```text
raw/lidar/20260902T1430_80858-0ac_broadway-columbus.pcapng
derived/scenes/20260902T1430_80858-0ac_broadway-columbus_vrlog/
derived/tracks/20260902T1430_80858-0ac_broadway-columbus.parquet
reports/20260902T1430_80858-0ac_broadway-columbus.md
```

The timestamp is the capture start, formatted `YYYYMMDDTHHMM`. It is interpreted
as `America/Los_Angeles`; timezone information belongs in the manifest and
dataset card, not in every filename.

The filename carries the human-facing S2 L13 **5+3 display** form, such as
`80858-0ac`. The manifest stores the canonical, hyphen-free S2 token, such as
`808580ac`, in `s2_l13`. S2 is a geographic partition only: it never replaces
the capture ID or the human site label.

The site slug is lowercase ASCII and hyphen-separated. It is a readable label,
not an authoritative location identifier.

## Manifest

One entry represents one capture and joins all known artifacts for it. The
initial JSON manifest uses these fields:

```text
capture_id
timestamp
timezone
latitude
longitude
s2_l10
s2_l13
site_slug
sensor_type
sensor_model
capture_type
duration_seconds
raw_path
scene_path
tracks_path
map_path
metrics_path
report_path
pipeline_version
pipeline_git_sha
```

Paths are dataset-root-relative POSIX paths. Unknown facts are `null`, never a
made-up sensor model, commit, duration or capture type. Additional provenance
that does not belong in a filename may be added as clearly named manifest
fields.

## Label sets

A label set holds reference objects and their per-frame membership, citing the capture's returns by
packet key (a digest of each UDP payload), so it applies to every run of the capture and to every
file holding its packets. The format is specified in
[`internal/lidar/annotation/labelset/README.md`](../../internal/lidar/annotation/labelset/README.md)
and operated as described in [label sets](../../docs/lidar/operations/label-sets.md).

| Rule                  | What it means here                                                                                                                                                                                                                                                                                                                                                                           |
| --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Path                  | `labels/<capture stem>/<label set name>/`, the stem being the capture's raw file name without its extension                                                                                                                                                                                                                                                                                  |
| Join                  | A row's `label_set_paths` lists the label sets attached to it. A label set is attached only when its `labels.json` `applies_to` names the row's `raw_sha256`                                                                                                                                                                                                                                 |
| Content               | The current state only: `labels.json` and the head files, records one per line and masks and foreground returns in `masks/` and `domain-returns/` by window range. The journal, its base, the lock and the commit marker stay with whoever edits the set; the dataset's Git history is the history                                                                                           |
| Line endings and size | `labels/** -text`. The text is plain Git, so the Hub shows edited lines and Git merges edits to different masks; every text file stays under Hugging Face's 10 MiB plain-Git limit, which staging checks. The Hub also refuses a push in which any commit holds a binary file in plain Git, even one a later commit moves to LFS/Xet. `packets.bin` goes through LFS/Xet by the `*.bin` rule |
| Domain                | Every mask names the foreground extraction it was made on. A return outside that foreground is unlabelled, never a negative                                                                                                                                                                                                                                                                  |
| Review status         | `reviewed` masks on `reviewed` objects are reference; `proposed` and `rejected` are not                                                                                                                                                                                                                                                                                                      |
| View                  | `labels/<capture stem>/<label set name>/views/<view>/`, inside its set, `<view>` being the run's cut time and id: the run's pack data and physical references, linked to `../..`. A row's `annotation_pack_paths` lists them                                                                                                                                                                 |
| Working state         | Ignored, never committed: the journal and its base, locks, the commit marker (`.pending`), the view's revision archives and proposal layers, and staging directories                                                                                                                                                                                                                         |

Stage a label set with `tools/s2-archive/stage-label-sets.py`. With `--capture-id`, it first binds
the label set to that row's published PCAPNG, which reads the file and records its digest. It then
publishes the current state, adds the path to `label_set_paths` and adds the rules above to the
dataset's `.gitattributes` and `.gitignore`. A set already published at the same head is kept;
`--replace` refreshes one that is behind and refuses one someone has edited.

A label set has no points, so it is published with a view: `--view PACK` publishes the pack the
labels were drawn on beside its set, checked as a fresh copy would hold it, and adds it to
`annotation_pack_paths`. A published view is never overwritten. Editing from the dataset, and what
a change commits, is described in
[label sets](../../docs/lidar/operations/label-sets.md#editing-from-the-dataset).

The dataset card's known limits say which captures carry labels, that they are foreground-domain
and `partial`, and how many masks are reviewed.

## Existing-artifact mapping

The current static release at
`/Volumes/lidar/lidar/pcaps/s2/static-huggingface` contains 24 extracted PCAPNGs and
their web-scene exports. It maps as follows:

| Existing artifact                     | Dataset destination                      | Notes                                                                                                                                                            |
| ------------------------------------- | ---------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Static PCAPNG                         | `raw/lidar/<stem>.pcapng`                | Keep `.pcapng`: it is the actual format, not the example's `.pcap`.                                                                                              |
| Sidecar JSON                          | Manifest fields                          | Do not carry one sidecar per raw file into the shallow release.                                                                                                  |
| Web scene `assets/` export            | `derived/scenes/<stem>_vrlog/`           | This is a chunked directory tree, not a portable single `.vrlog` file. Its `part-*` and frame chunks are the concrete scaling reason for this one nested export. |
| Radar, tracks, maps, metrics, reports | Corresponding empty artifact directories | No current artifact maps to these paths; no placeholders are fabricated.                                                                                         |

Two historical facts are intentionally represented as null until better source
evidence exists: `sensor_model` and `pipeline_git_sha`. The site coordinates are
field-map readings, so consumers must treat them as approximate.

The static PCAPNG export begins at the indexed static interval. That may differ
from the first source-file packet when the source capture spans a drive; the
manifest records the actual indexed start and does not imply that it is the
start of every contributing raw file.

## Relationship to repository S2 guidance

The general repository geographic-indexing guide reserves canonical tokens for
machine identifiers and normally avoids a display token in filenames. This
dataset convention deliberately uses the requested L13 display form in its
human-facing filename while retaining canonical L10/L13 tokens in the manifest.
It is a dataset packaging boundary only; it does not change application or API
identifier conventions.

## Hugging Face subsets and splits

The layout does not encode `train`, `validation` or `test`. When the dataset
needs subsets or splits, declare them explicitly in the YAML front matter of
the dataset-card `README.md`. Hugging Face supports mappings from YAML
`configs` to files or globs, so the physical evidence layout can remain stable.

## Migration safety

`/Volumes/lidar/lidar/pcaps/sf-street-speeds` is the target root. Do not overwrite an existing
target artifact. First inventory the current release, then copy or move only
after the 24 planned stems, paths and manifest entries have been reviewed.
The present source tree is about 68 GiB, so duplicating it for a staging copy is
a material storage decision rather than an implicit build step.

When migrating the present release, retain its old per-PCAP sidecars outside the
new dataset root until the manifest has been independently accepted. They are a
recovery record; the new shallow dataset itself carries their provenance in
`manifest.json` and does not publish them as parallel per-file metadata.
