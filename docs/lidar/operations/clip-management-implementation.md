# LiDAR clip management

- **Status:** Implemented; the D-31 clip cutover is migration 064
- **Canonical:** [LiDAR hub](../LIDAR.md)

A **clip** is a window chosen for evaluation or annotation. It names a sensor,
ordered capture rows, optional start and duration offsets, a reference run and
recommended tuning parameters. A **run** is one processing pass over that input.
The [platform vocabulary](../../platform/PLATFORM.md#platform-vocabulary) defines these nouns.

## Storage

`lidar_clips.clip_id` identifies the window. `lidar_clip_captures` holds its
ordered capture paths, keyed by `(clip_id, ordinal)`. `lidar_clip_evaluations`
compares reference and candidate runs; `lidar_runs` stores each run’s immutable
configuration provenance. The Go types are `sqlite.Clip`, `ClipCapture` and `Run`.

Migration 064 renames the existing tables and referring columns. It preserves
keys, row values and relationships, backfills missing capture associations from
the old clip path, then drops that path projection. Capture rows are the only
source for a clip’s paths. The clip/selection merge remains Phase 2.

## API and clients

`GET /api/lidar/clips` returns `{ "clips": [], "count": 0 }`. `POST` creates a
clip; `GET`, `PUT` and `DELETE /api/lidar/clips/{clip_id}` manage it. The
`/replay`, `/evaluations`, `/location` and `/clip` actions retain their operations.
The `/clip` action exports an annotation pack; it does not create another clip.

```json
{
  "sensor_id": "hesai-pandar40p",
  "pcap_files": ["first.pcap", "second.pcap"],
  "pcap_start_secs": 15,
  "pcap_duration_secs": 30,
  "description": "Evaluation window"
}
```

The captures must form one continuous packet stream. Offsets are measured from
the beginning of the complete sequence. Responses expose the ordered `files`
association and `file_count`, without a clip-level `pcap_file` projection.

The web page is `/lidar/clips`; old page bookmarks redirect for one release.
`/api/lidar/scenes` has no alias. Every in-repository caller uses `/api/lidar/clips`
and `clip_id`. The previous macOS DMG’s identifier key and label query parameter
remain registered aliases until the next DMG is the oldest supported; the current
app uses `clip_id`. See the [compatibility register](../../platform/operations/compatibility-register.md).

## Extracts and publishing

A clip names its source captures and offsets, and replays them in order. It
can also be cut to one standalone capture, its **extract**, and published as a
survey in the Hugging Face dataset. Migration 066 adds the state for both.

| Step                 | Surface                                                           | What it does                                                                                                                                                                                                          |
| -------------------- | ----------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Locate a period      | `/lidar/captures`: click a static period, then Create clip        | Records `source_period_id` with the clip; the form also takes coordinates, a dataset id, a north azimuth and whether to cut the extract now                                                                           |
| Cut the extract      | `POST /api/lidar/clips/{id}/extract`, or the Clips page           | Queues a `capture_export` job on the capture queue; `GET` lists the clip's extracts (`lidar_clip_extracts`) with progress                                                                                             |
| Choose what replays  | `PUT /api/lidar/clips/{id}/publishing` with `storage`             | `sources` replays the captures and offsets; `extract` replays the latest completed extract whole                                                                                                                      |
| Mark for the dataset | the same `PUT`, with `dataset_id` and `north_azimuth_deg`         | `dataset_id` is the survey's id in the dataset: the site-index `id` and the manifest `site_slug`, lowercase words joined by hyphens, unique across clips                                                              |
| Stage                | `POST /api/lidar/clips/{id}/stage`, or the Clips page             | Hard-links the extract to `raw/lidar/<stem>.pcapng` in the dataset clone (`--lidar-dataset-dir`, from `S2_CORPUS_DIR`) and adds or replaces the clip's `manifest.json` entry; every other entry is kept byte for byte |
| Index                | `GET /api/lidar/dataset/site-index`, then `make site-index-clips` | The server serves a site-index entry per staged-ready clip; `tools/s2-archive/merge-clip-sites.py` writes them into `site-index.json` with `"origin": "clip"`, which `build-site-index.py` keeps on rebuild           |
| Scene                | `make scene-assets SITES=<dataset id>`                            | Unchanged: the scene publisher reads the site index and the dataset manifest                                                                                                                                          |

The cut is `internal/lidar/capexport`, rubric 1 of the
[survey capture export plan](../../plans/platform-survey-capture-export-plan.md) for a
single part. It starts on the first LiDAR packet wholly past an azimuth wrap at or after the
clip's start, and ends before the first such packet at or after its end, so the file holds
whole rotations. Packets on every port are copied as captured, in file order. The section
comment names the rubric and the clip, and carries no build or host stamp, so the same
window cut twice gives the same bytes. Extracts are written under `clips/` in the replay
directory, named with the dataset stem when the clip is located and has a dataset id, and
`<YYYYMMDDTHHMM>_clip-<id>` otherwise. A sidecar JSON beside each follows
`export-static-pcaps.py`'s schema 1 and adds the clip, the rubric, the rotation count and why
the cut closed (`window` or `end`).

Staging refuses a clip with no dataset id, no location or no completed extract, a dataset
clone that is not there, and a dataset id another capture in the manifest already uses. It
never uploads: publishing the dataset stays a person's step. A part grid above 4 GB is not
cut yet; an extract is one file whatever its size.

## Locating a static period on a map

The Captures page lists, under "Only sessions with a static period that needs a location", the
sessions with a static period of at least 60 s whose clip is not located (or that has no clip).
`GET /api/lidar/capture/periods` carries each period's clip and whether it is located. Every static
period links to `/lidar/locate`.

The Locate page draws the period's **settled background** from above:
`GET /api/lidar/capture/periods/background?period_id=` replays the period's first 90 s into a
fresh L3 model (the motion pass's parameters, never refreshed) and returns the cells that settled,
in the sensor frame. The server builds it once per period and keeps the last eight. The operator
places the sensor by address search or by clicking, then drags it and its heading handle (the bearing
of +Y). Placement is manual; the OSM-outline Snap path is retired. Save calls
`POST /api/lidar/capture/periods/clip`, which returns the period's clip or makes one from its
captures and trim, then records the position (`geographic_source` operator) and the heading as
`north_azimuth_deg = 180 − heading`, the convention `site-index.json` and the Gerardus pane use.

**Known locations and checks.** "Start from a known location" applies another located clip's
position and heading (from `GET /api/lidar/clips?located=1`), so a place located once can be applied
to the other captures there and checked against each one's own background. The background remains a visual reference; the OSM-outline Check fit action is retired.
Opened with `?clip=`, the page checks an existing clip against its own window's background
(`GET /api/lidar/clips/{id}/background`) and saves back to that clip; the Clips page links it as
"Check alignment on map".

Map tiles and the address search (Nominatim) are external requests. Neither
is made until the operator allows OpenStreetMap requests, a choice this browser remembers until it is
withdrawn on the page; what leaves is the typed
address and the map area, never sensor data. Without them the page still draws the background on a
blank map, and the coordinates can be typed on the Captures page instead.

The page draws the period's settled background, measured at the capture time, over an optional browser map. The saved pose is the operator's placement;
it is not a survey, plan or specification of the buildings. See
[Geometry priors: data purpose and interpretation](../../geometry-priors-data-purpose.md).
