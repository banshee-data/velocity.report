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
