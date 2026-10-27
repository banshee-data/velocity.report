---
configs:
  - config_name: manifest
    data_files: manifest.json
---

# San Francisco street-speed dataset

**Measure velocity, not identity**

This dataset contains replayable LiDAR evidence from 24 stationary surveys on
San Francisco streets. It keeps the raw packets beside derived browser-scene
exports, with one manifest joining the two. The point is to let researchers
inspect what traffic did without turning the people using the street into the
product.

There are no camera images, licence-plate photographs, or intentionally
identifying fields. There are raw LiDAR packets. They are evidence, not
decoration, and should be handled with the care due to a detailed measurement
of a public place.

```text
                                ░░░░░░                                  ░░░░░░
   ░░░░░    ░░░░░░░░░░░    ░░░░░ :::. ░░░░░░░░░░    ░░░░░░░░░░░    ░░░░░ :::.
  ░▒▒▒▒▒░░░░▒▒▒▒▒▒▒▒▒▒▒░░░░▒▒▒▒▒ .:.  ▒▒▒▒▒▒▒▒▒▒░░░░▒▒▒▒▒▒▒▒▒▒▒░░░░▒▒▒▒▒ .:.
░░▒▓▓▓▓▓▒▒▒▒▓▓▓▓▓▓▓▓▓▓▓▒▒▒▒ .::.::: : ▓▓▓▓▓▓▓▓▓▓▒▒▒▒▓▓▓▓▓▓▓▓▓▓▓▒▒▒▒ .::.:.: :
▒▒▓.  . ▓▓▓▓████ ... . ▓▓▓▓ :....:::::. ▓▓▓.  . ▓▓▓▓████ ... . ▓▓▓▓ :.::..::::
▓▓█ ..::████████ :.::: ████ :: :  : . ::▓▓▓ ..::████████ :.::: ████ :: :: : ::
:.: : ::████:.:: : ::: ████ .: : .  ... :.: ::::████:.:: : ::: ████ .::: .:::.
.::  .  .   ::.:    .  .          .     .::  .  .   ::.: ::::  .          .
    .     .       .
                         ▄▀▀▀▀▀▀▄
                       ▄▄█▄▄▄  ▄▄▌
                         ▌▐▀▀▀█  ▌
▀▀▀▀▀▀█████▀▀▀▀▀▀▀▀▀█▀▀█▀▐▀  ▄▄▀▀█▀▀▀▀▀▀▀█▀▀██▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
▄▄▄▀▀▀░░ ░ ░ ░▒▓▄▄▀ ▄▄▀▀ ▄▓▀▀▀  ░▐▌       ▀▀▄▄▀▀▄▄▓▒ ▒   ░ ░    ░  ░   ░  ░
 ░ ░░ ░ ░ ▒ ▄▄▀▀▄▄▀▀▄▄▄▄▓░   ░▄  ░█           ▀▀▄▄▀▀▄▄  ▒ ░   ░   ░   ░ ░    ░
  ░  ░ ▒▄▄▀▀▄▄▀▀   ▀▄▀█░ ▄▄▀█▀░░ ░█               ▀▀▄▄▀▀▄▄   ▒ ░    ░      ░
░░ ▒▄▄▀▀▄▄▀▀       █▀▀▓▀▀▌  ▐▌░ ░░▒▌                  ▀▀▄▄▀▀▄▄   ░ ░   ░  ░  ░
▄▄▀▀▄▄▀▀           ▓  ▓  ▌  █▀▀▀▀▀▀█                      ▀▀▄▄▀▀▄▄  ▒░ ▒
▄▄▀▀               ▓▀▀▓▀▀▌ ▐▌░  ░  ▐▌                         ▀▀▄▄▀▀▄▄   ░ ▒
                   ▓__▓__▌ █░  █▄░ ▐▌                             ▀▀▄▄▀▀▄▄   ░
                   ▓  ▓  ▌ █░ ▐▌ █░ █                                 ▀▀▄▄▀▀▄▄
▄▄▄  ▄▄▄▄▄▄  ▄▄▄▄  ▓  ▓  ▌▒▒▒▒▒  ▒▒▒▒   ▄▄▄▄▄  ▄▄▄▄   ▄▄▄▄▄  ▄▄▄▄▄  ▄▄▄▄▄▄▄  ▄
█▀ ▄█████▀ ▄████▀  ▄████   ████   ████  ██████  ▀███▄  ▀████▄ ▀████▄ ▀██████▄
 ▄█████▀  ▄████▀  ▄████▀  ▄████   █████  ██████  ▀███▄  ▀████▄  █████▄ ▀██████
█████▀  ▄█████   ▄████▀   █████   █████   ██████   ████▄  █████  ▀█████▄ ▀████
████▀  ▄█████   ▄█████    ████▀   ▀████▄   ██████   ████▄  ▀████▄  ▀█████  ▀██
██▀  ▄█████▀   ▄█████    ▄████     █████    ██████   ▀████▄ ▀█████  ▀█████▄  ▀
▀   ▄█████▀   ▄█████▀    █████     ██████    ██████   ▀████▄  █████▄  ▀█████▄
```

## What is here

| Measure                   |                           Value |
| ------------------------- | ------------------------------: |
| Static LiDAR captures     |                              24 |
| Recording dates           |             September 1–3, 2026 |
| Recorded duration         |              8 hours 26 minutes |
| Raw PCAPNG size           | 73,228,262,480 bytes (68.2 GiB) |
| S2 level-13 cells         |                              23 |
| Derived web-scene exports |                              24 |
| Filename timezone         |           `America/Los_Angeles` |

Each PCAPNG covers an operator-approved interval in which the sensor platform
was parked. `static` describes the platform, not the street: vehicles,
pedestrians, cyclists, and all the ordinary business of a city continue moving
through the point cloud.

The source recorder rolled files every few minutes. Each published PCAPNG clips
the contributing source files to one indexed site interval and merges the
pieces in capture order. The manifest retains the source names, byte counts,
and SHA-256 digests so that this lineage is inspectable rather than merely
asserted.

## Layout

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
  <capture stem>/
reports/
```

Directories answer one question: what kind of artefact is this? Dates, S2
cells, and sites do not create another staircase of folders. Filesystems are
quite capable of becoming taxonomies without encouragement.

This release populates `raw/lidar/` and `derived/scenes/`, and `labels/` for
the captures that carry reference labels. The other directories
reserve the agreed homes for later radar, track, map, metric, and report
artefacts. Their presence is not a claim that those artefacts exist.

The scene exports are directories because the browser format is chunked. Each
contains its manifest and one or more parts with headers, timelines, indexes,
and compressed frame chunks. Repacking that tree as a pretend single VRLOG
would make the name simpler and the data less useful, which is the wrong
exchange rate.

## Labels

Reference labels say which returns belong to which road user, frame by frame.
They are published as label sets, one folder per capture:

```text
labels/<capture stem>/
  <label set name>/     labels.json and the head files: objects, windows, masks
    views/<view>/       a view: one run's points of the capture, with physical
                        references, linked to the label set it sits in
```

A label set cites returns by a digest of the sensor's UDP payload, so it
applies to the raw PCAPNG and to any other file holding the same packets. Each
mask names the foreground extraction it was drawn on. A return outside that
foreground was never shown to the labeller and is unlabelled, not a negative.
Masks are `partial`: they claim what the foreground showed, not the whole
vehicle. The view is the run the labels were drawn on, and it is what the
annotation tools open. A capture's manifest row lists its label sets in
`label_set_paths` and its views in `annotation_pack_paths`.

| Capture                              | Label set         | Objects | Masks (reviewed) | Physical references                                                         |
| ------------------------------------ | ----------------- | ------: | ---------------: | --------------------------------------------------------------------------- |
| `20251205T1417_8085875c_kirkham-9th` | `kirk0-reference` |      30 |    4,102 (4,102) | 3 vehicles: 3 bodies and 22 of 44 keyframes reviewed, 2 following decisions |

The label sets are maintained with the
[velocity.report](https://github.com/banshee-data/velocity.report) annotation
tools. To label or correct a capture you need its labels, not the 74 GB of
captures. Clone without LFS content, check out only that capture's folder, and
fetch its LFS content alone:

```bash
GIT_LFS_SKIP_SMUDGE=1 git clone --no-checkout https://huggingface.co/datasets/banshee-data/sf-street-speeds
cd sf-street-speeds
git sparse-checkout set --no-cone /labels/<capture stem>/ /manifest.json /README.md /.gitattributes /.gitignore
GIT_LFS_SKIP_SMUDGE=1 git checkout
git lfs pull --include="labels/<capture stem>/**"
```

For kirk0 that is about 110 MB in under a minute. Then make a branch, edit with
the project's annotation window, run its `check-view` check, and open a pull
request. The steps, and what a change commits, are in the project's
[label set guide](https://github.com/banshee-data/velocity.report/blob/main/docs/lidar/operations/label-sets.md#editing-from-the-dataset).

## Filenames

Capture-associated artefacts share this stem:

```text
<timestamp>_<s2-l13-display>_<site-slug>
```

For example:

```text
raw/lidar/20260902T1320_80858-0f4_columbus-broadway.pcapng
derived/scenes/20260902T1320_80858-0f4_columbus-broadway_vrlog/
```

- `timestamp` is the indexed capture start in `YYYYMMDDTHHMM` form.
- `s2-l13-display` is velocity.report's human-readable 5+3 form.
- `site-slug` is lowercase ASCII with words separated by hyphens.

The timezone is fixed in the manifest rather than repeated in every filename.
The slug is a label for people, not an authoritative location identifier.

## Geographic identity

The filename uses a readable S2 L13 value such as `80858-0f4`. The manifest
stores its canonical form, `808580f4`, and the canonical L10 parent separately.
Software should use those manifest fields for grouping and joins.

S2 identifies a geographic partition. It does not uniquely identify a capture,
survey, sensor deployment, road, or semantic site. Two captures may share an
L13 cell, as two of these do, without becoming the same observation by a feat
of cartographic optimism.

Latitude and longitude came from an operator-marked field map. They are useful
neighbourhood-level positions, not surveyed junction coordinates. Do not use
them for lane-level registration, legal boundaries, or any task where a few
hundred metres would be an exciting surprise.

## The manifest is the index

`manifest.json` is the authoritative machine-readable corpus index. It contains
one object per capture and joins raw evidence to derived artefacts.

| Field                                                    | Meaning                                                                     |
| -------------------------------------------------------- | --------------------------------------------------------------------------- |
| `capture_id`                                             | Stable corpus identity using the minute, canonical L13 token, and site slug |
| `timestamp`, `timezone`                                  | Exact indexed start and its named timezone                                  |
| `filename_timestamp`                                     | Minute-resolution timestamp used in filenames                               |
| `latitude`, `longitude`                                  | Approximate WGS84 field-map position                                        |
| `s2_l10`, `s2_l13`                                       | Canonical S2 tokens for machine use                                         |
| `site_slug`                                              | Human-readable site label                                                   |
| `sensor_type`, `sensor_model`                            | Sensor class and model when known                                           |
| `capture_type`                                           | How the capture interval was selected                                       |
| `duration_seconds`                                       | Exact indexed interval duration                                             |
| `raw_path`, `scene_path`                                 | Dataset-relative artefact paths                                             |
| `tracks_path`, `map_path`, `metrics_path`, `report_path` | Related products, or `null` when absent                                     |
| `pipeline_version`, `pipeline_git_sha`                   | Splitter build provenance when known                                        |
| `raw_sha256`, `raw_bytes`                                | Integrity metadata for the published PCAPNG                                 |
| `export_provenance`                                      | Static-export policy, operator joins, and contributing source files         |

Paths are relative to the dataset root and use forward slashes. Unknown values
are `null`. A null is an honest gap; a plausible-looking guess is merely a bug
wearing a tie.

For example, inspect one capture with:

```bash
jq '.[] | select(.site_slug == "columbus-broadway")' manifest.json
```

Verify its raw file against the recorded digest with:

```bash
shasum -a 256 raw/lidar/20260902T1320_80858-0f4_columbus-broadway.pcapng
```

## Provenance and processing

Seventeen captures were segmented with `pcap-split` 0.5.1-pre31. Seven captures
from the continuous September 1 analysis used 0.5.1-pre32. The exact pipeline
Git SHA is not available for this historical release, so `pipeline_git_sha` is
null rather than reconstructed from circumstantial evidence.

The web scenes are derived views of the corresponding capture. They make the
point clouds practical to inspect in a browser, but they do not replace the raw
PCAPNG evidence. When the two serve different questions, use the raw capture as
the record and the scene as the convenient view.

## What the geometry is

The raw packets, and the scene exports derived from them, are measured spatial
data: where a LiDAR sensor saw surfaces, at the time it recorded. Buildings,
trees, poles, parked vehicles, and other stationary features appear only as
returns from whatever faced the sensor. This is a measurement of observable
physical conditions, not a plan, drawing, or specification of any structure. It
does not aim to capture hidden elements, materials, finishes, interiors, or
methods of construction.

- **When.** Recording dates are in the table above, and the manifest holds each
  capture's `timestamp`, `timezone`, and `duration_seconds`. Streets change; this
  describes the recorded interval, not today.
- **Source and terms.** The project's own sensor recorded every packet, and no
  outside map, survey, or building data is merged in. Site positions come from
  an operator-marked field map (see [Geographic identity](#geographic-identity)).
  Redistribution terms are not yet declared (see [Project](#project)).
- **Frame and units.** Scene coordinates are metres in a local right-handed
  frame with the sensor at the origin and Z up. A header's `coordinate_frame`
  says `ENU`, but the release carries no measured north: X and Y follow the
  sensor's orientation, not a compass bearing. Nothing is georeferenced, and
  captures are not registered to each other or to a map.
- **Observed.** The raw PCAPNG: returns as the sensor reported them, clipped to
  the site interval. A scene's `clip` points, where it has them, are the returns
  the pipeline classed as foreground, thinned and rounded to 2 decimal places.
- **Derived or inferred.** A scene's background layer is a settled background
  snapshot thinned to one point per 0.25 m cell, with `ground_z` a low
  percentile of Z. Tracks, boxes, trails, headings, and speeds are the tracker's
  estimates. The raw capture stays the record.
- **Accuracy and coverage.** No positional uncertainty is recorded, and the
  sensor's ranging is not independently validated here. The sensor stood at one
  spot per site (a tripod nudge aside), so it saw only surfaces facing it:
  vehicles, trees, and buildings hide what is behind them, returns thin with
  range, the vertical field of view clips tall façades, and glass, dark, or wet
  surfaces may return little. A surface missing from the data is not evidence
  that nothing is there.
- **Version.** The manifest's `pipeline_version` and `raw_sha256`, and a scene
  header's `build_version`, `tuning_hash`, and `source_vrlog_sha256`, name the
  code, tuning, and recording behind each artefact.

Use it for localisation, alignment, change detection, and traffic research. It
is not a land or boundary survey, a structural assessment, or a plan or
specification, and it is not an authoritative statement of current site
conditions. The project's
[statement of purpose](https://github.com/banshee-data/velocity.report/blob/main/docs/geometry-priors-data-purpose.md)
says the same at more length.

## Known limits

- The positions are approximate readings from a field map.
- Geometry has one viewpoint per site and no per-point uncertainty; see
  [What the geometry is](#what-the-geometry-is).
- `sensor_model` is not established in the release provenance and remains
  `null`.
- `pipeline_git_sha` is unknown for the historical splitter runs.
- Reference labels cover only the captures listed under [Labels](#labels). They
  are foreground-domain and `partial`: a return the labelling run's foreground
  left out is unlabelled. There are no ground-truth tracks, trajectories, or
  reference speeds.
- Track, map, metric, report, and radar artefacts are not included yet.
- The filename timestamp is minute-resolution; the manifest preserves the full
  timestamp and offset.
- A capture may include an operator-approved short tripod nudge within the same
  site interval. Such overrides are recorded in `export_provenance`.

These are limits, not invitations to quietly fill the gaps. Research data has
enough uncertainty without adding confidence by typography.

## Privacy and responsible use

velocity.report measures movement without cameras, faces, licence plates, or
identity tracking. This release follows the same principle. It is intended for
street-safety research, reproducible perception work, and community evidence,
not for identifying or monitoring individuals.

Raw point clouds still describe activity in public space. Users should assess
their own derived outputs before publication and avoid adding identity-bearing
data from other sources. Privacy is not inherited automatically by every model
that happens to read a privacy-preserving input.

## Splits and subsets

The filesystem does not encode `train`, `validation`, or `test`. If the corpus
later needs benchmark splits or named subsets, define them explicitly in this
dataset card's YAML configuration. That keeps the evidence layout stable while
allowing several research views of the same corpus.

## Project

The dataset was produced by
[velocity.report](https://github.com/banshee-data/velocity.report), an
open-source, local-first project that helps communities measure traffic speeds
and make the case for safer streets without building a camera network to do it.

The dataset licence and preferred academic citation have not yet been declared.
Set both before public release. A missing licence is not a licence with better
manners.
