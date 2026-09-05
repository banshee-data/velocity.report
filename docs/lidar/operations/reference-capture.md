# Reference capture

`soma1-static-0.pcap` is the reference capture for scene export and web viewer
work. When a change needs a known recording to measure against, this is the one
to use, so that sizes, timings and rendering are comparable across changes.

This is a working default, not a permanent fixture. It was chosen because it is
the longest real recording currently available with a matching VRLOG, and
because the sensor is static throughout. Expect it to be replaced once the
`s2_sf_*` archive captures on `/Volumes/lidar/lidar/s2/` have VRLOGs of their
own; the intent is that whatever replaces it inherits this page rather than
adding a second reference.

It is distinct from [`kirk0.pcapng`](../../../internal/lidar/perf/pcap/kirk0.pcapng),
which stays the reference for frame-level parser and performance work and is
small enough to live in the repository. `soma1-static-0.pcap` is 1.5 GB and is
not committed.

## Capture

| Field    | Value                                                                   |
| -------- | ----------------------------------------------------------------------- |
| Path     | `/Users/david/code/sensor_data/lidar/static/soma1-static-0.pcap`        |
| Size     | 1,579,772,992 bytes (1.5 GiB)                                           |
| SHA-256  | `30cd7f148d0fcb3c9d50c741178ad4b4495443d7f637710ffe9fd7b4473c16b1`      |
| Sensor   | Hesai Pandar40P                                                         |
| Mounting | Static roadside; sensor is the coordinate origin, above the carriageway |

The capture is not in the repository and is not published. Only derived scene
assets are committed.

## Run

The VRLOG used for the published scene, and its run record in `sensor_data.db`:

| Field            | Value                                                  |
| ---------------- | ------------------------------------------------------ |
| Run ID           | `f84105d8-b3be-416f-8809-551ef6bfce10`                 |
| VRLOG            | `/Users/david/code/sensor_data/lidar/vrlog/f84105d8-…` |
| Build version    | `0.5.1-pre31`                                          |
| Schema version   | `run_config/v1`                                        |
| L3 / L4 / L5     | `ema_baseline_v1` / `dbscan_xy_v1` / `cv_kf_v1`        |
| Tracks persisted | 2,038, all confirmed                                   |
| Status           | `completed`                                            |

### Frame counts

This recording predates the header's rotation count, so its numbers have to be
derived by reading it:

| Frame type           | Count     |
| -------------------- | --------- |
| Foreground           | 6,604     |
| Empty placeholder    | 26        |
| **Sensor rotations** | **6,630** |
| Background snapshot  | 216       |
| Total records        | 6,846     |

6,630 rotations over 662.9 s is 10.00 Hz, which matches the sensor. The
recording is internally consistent.

The run record's **7,939 frames over 632.7 s** is not: that is 12.55 Hz, which
a Pandar40P cannot produce. The run over-counted, because the pipeline recorded
a frame only on the clustering path while three earlier stages could return
first; a run that re-entered those paths counted unevenly against the rotations
the recorder saw. Both sides now count one rotation per rotation, and new
recordings carry `rotation_frames` in the header so the two are directly
comparable without reading every chunk.

Do not compare a run record's frame count with `total_frames`: that figure
counts background snapshots too, because it indexes `index.bin`. Compare it
with `rotation_frames`.

## Observed characteristics

Measured from the exported scene rather than asserted:

| Property              | Value                                                                  |
| --------------------- | ---------------------------------------------------------------------- |
| Rotation rate         | ~10 Hz, varying 9.95–10.03 Hz within the capture                       |
| Tracks per frame      | ~10.7 mean, 50 peak                                                    |
| Class mix             | dynamic and unclassified dominant; then pedestrian, car, bird, cyclist |
| Fastest observed      | 7.7 m/s (~17 mph)                                                      |
| Median track extent   | 0.67 m; largest 4.83 m                                                 |
| Track position spread | ±56 m in X, −56 to +27 m in Y                                          |
| Ground plane          | ~2.2 m below the sensor                                                |

It is a pedestrian-heavy scene rather than a fast-traffic one. That makes it a
good test of small-object rendering and a poor test of high-speed tracking; use
a different capture when speed distribution is what matters.

## Reproducing the published scene

```bash
velocity scene export \
    --vrlog /Users/david/code/sensor_data/lidar/vrlog/f84105d8-b3be-416f-8809-551ef6bfce10 \
    --out   public_html/src/scenes/soma1/assets/part-000 \
    --stride 2 --site soma1 --title "SoMa 1"
```

Result, as committed:

| Property           | Value                                                              |
| ------------------ | ------------------------------------------------------------------ |
| Frames retained    | 3,315 — every second rotation of 6,630                             |
| Duration           | 662.8 s                                                            |
| Chunks             | 34 (100 retained frames each)                                      |
| Size on disk       | 1,262,561 bytes (1.2 MiB), 112 KB per minute                       |
| Source fingerprint | `da0b461a1c975489ecb3118da3b306ace562018bac4ca180d39ec6737eaedc35` |

The source fingerprint in `header.json` hashes the VRLOG's `header.json` and
`index.bin`, so a published asset can always be traced to the recording it came
from. It is not the capture's SHA-256.

Stride counts rotations, not records, so 3,315 is exactly half of 6,630.

The export omits background snapshots. A recording deliberately opens with one
so a replay has a scene from its first frame, and a snapshot inherits the most
recent foreground timestamp; after a settling pass that timestamp is the end of
the capture, so the opening snapshot is not ordered with the rotations around
it. It is pipeline state rather than an observation, and skipping it keeps the
exported timeline monotonic.

## Related

- [Web scene export plan](../../plans/lidar-web-scene-export-plan.md)
- [Scene catalogue publishing plan](../../plans/lidar-scene-catalogue-publishing-plan.md)
- [PCAP analysis mode](pcap-analysis-mode.md)
