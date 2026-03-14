# VRLOG Wire Format Specification

## Overview

A **`.vrlog`** file is a directory-based recording format for LiDAR frame data. It stores
timestamped `FrameBundle` snapshots from the velocity.report perception pipeline, enabling
seekable replay, labelling, and offline analysis.

**Version:** 0.5
**Source:** [`internal/lidar/visualiser/recorder/recorder.go`](../../internal/lidar/visualiser/recorder/recorder.go)

## Directory Layout

```
<name>.vrlog/
├── header.json          # Log metadata (JSON)
├── index.bin            # Binary seek index
└── frames/
    ├── chunk_0000.pb    # Length-prefixed frame bundles
    ├── chunk_0001.pb
    └── ...
```

## header.json

JSON object written when the recorder closes. Contains log-level metadata.

| Field              | Type    | Description                                                            |
| ------------------ | ------- | ---------------------------------------------------------------------- |
| `version`          | string  | Format version (currently `"0.5"`)                                     |
| `created_ns`       | int64   | Wall-clock creation time (Unix nanoseconds)                            |
| `sensor_id`        | string  | Sensor identifier (e.g. `"hesai-01"`)                                  |
| `total_frames`     | uint64  | Total number of frames in the recording                                |
| `start_ns`         | int64   | Timestamp of the first frame (Unix nanoseconds)                        |
| `end_ns`           | int64   | Timestamp of the last frame (Unix nanoseconds)                         |
| `coordinate_frame` | object  | Coordinate frame metadata (see below)                                  |
| `source_type`      | string  | Recording source: `"live"`, `"pcap"`, `"synthetic"` (omitted if empty) |
| `pcap_path`        | string  | Original PCAP filename, basename only (omitted if empty)               |
| `playback_rate`    | float64 | Configured replay speed multiplier (omitted if 0)                      |
| `tuning_hash`      | string  | SHA-256 hex digest of the tuning config JSON (omitted if empty)        |
| `build_version`    | string  | velocity.report version that created the recording                     |

### coordinate_frame

| Field             | Type   | Description                  |
| ----------------- | ------ | ---------------------------- |
| `frame_id`        | string | e.g. `"site/hesai-01"`       |
| `reference_frame` | string | e.g. `"ENU"` (East-North-Up) |

### Example

```json
{
  "version": "0.5",
  "created_ns": 1740000000000000000,
  "sensor_id": "hesai-01",
  "total_frames": 12345,
  "start_ns": 1740000000000000000,
  "end_ns": 1740000600000000000,
  "coordinate_frame": {
    "frame_id": "site/hesai-01",
    "reference_frame": "ENU"
  },
  "source_type": "pcap",
  "pcap_path": "site-capture-2026-03-10.pcap",
  "playback_rate": 1.0,
  "tuning_hash": "a1b2c3d4e5f6...",
  "build_version": "0.5.0-pre16"
}
```

## index.bin

Fixed-size binary seek index — one entry per frame, written in order. All fields
are **little-endian**.

### Entry Layout (24 bytes)

| Offset | Size | Type   | Field         | Description                               |
| ------ | ---- | ------ | ------------- | ----------------------------------------- |
| 0      | 8    | uint64 | `FrameID`     | Monotonic frame identifier                |
| 8      | 8    | int64  | `TimestampNs` | Frame timestamp (Unix nanoseconds)        |
| 16     | 4    | uint32 | `ChunkID`     | Zero-based chunk file index               |
| 20     | 4    | uint32 | `Offset`      | Byte offset of the frame within the chunk |

The total file size is `24 × total_frames` bytes. To seek to frame _N_, read
`index.bin` at offset `24 × N`, extract `ChunkID` and `Offset`, then read
the frame from the corresponding chunk file.

## Chunk Files (`frames/chunk_NNNN.pb`)

Each chunk file contains a sequence of length-prefixed serialised frames.

### Frame Encoding

```
┌──────────────┬────────────────────────────────┐
│ uint32 LE    │ Frame data                     │
│ (4 bytes)    │ (variable length)              │
│ = byte count │ = JSON-serialised FrameBundle  │
└──────────────┴────────────────────────────────┘
```

Frames are concatenated sequentially within the chunk with no padding or
delimiters beyond the length prefix.

### Serialisation Format

**Current (v0.5):** JSON-serialised `FrameBundle` (Go `encoding/json`).

> **Note:** The TODO in the source code indicates a future migration to
> Protocol Buffers for the on-disk frame encoding. The `.pb` file extension
> is forward-looking; the current format is JSON. See the
> [protobuf schema](../../proto/velocity_visualiser/v1/visualiser.proto)
> for the target wire format.

### Chunk Rotation Policy

A new chunk file is created when **any** of the following conditions are met:

| Condition             | Threshold      |
| --------------------- | -------------- |
| Frame count per chunk | 1,000 frames   |
| Byte size per chunk   | 150 MB written |

The read-side hard limit is **200 MB** per chunk (rejects chunks larger than
this to prevent excessive memory allocation).

## FrameBundle Model

The `FrameBundle` struct is the canonical internal model. Key fields serialised
into each chunk frame:

| Field             | Type                | Description                              |
| ----------------- | ------------------- | ---------------------------------------- |
| `FrameID`         | uint64              | Monotonic frame counter                  |
| `TimestampNanos`  | int64               | Frame timestamp (Unix nanoseconds)       |
| `SensorID`        | string              | Sensor identifier                        |
| `CoordinateFrame` | CoordinateFrameInfo | Spatial reference                        |
| `PointCloud`      | PointCloudFrame?    | Raw 3D points (X, Y, Z, intensity, etc.) |
| `Clusters`        | ClusterSet?         | Segmentation output                      |
| `Tracks`          | TrackSet?           | Tracked objects with velocity            |
| `Debug`           | DebugOverlaySet?    | Algorithm debug data (optional)          |
| `PlaybackInfo`    | PlaybackInfo?       | Added at read time by the Replayer       |
| `Background`      | BackgroundSnapshot? | Background grid state (split streaming)  |
| `FrameType`       | int                 | Frame type (see below)                   |

### FrameType Values

| Value | Constant              | Description                                          |
| ----- | --------------------- | ---------------------------------------------------- |
| 0     | `FrameTypeFull`       | Legacy: all points included                          |
| 1     | `FrameTypeForeground` | Split streaming: foreground + clusters + tracks only |
| 2     | `FrameTypeBackground` | Background grid snapshot                             |
| 3     | `FrameTypeDelta`      | Reserved for future incremental updates              |
| 4     | `FrameTypeEmpty`      | Placeholder: no foreground objects detected (v0.6+)  |

`FrameTypeEmpty` frames carry only metadata (`FrameID`, `TimestampNanos`,
`SensorID`, `CoordinateFrame`). All perception fields (`PointCloud`,
`Clusters`, `Tracks`) are nil. These frames exist for deterministic 1:1
PCAP-to-VRLOG mapping and must be preserved during replay/seek.

Full model definition: [`internal/lidar/visualiser/model.go`](../../internal/lidar/visualiser/model.go)

## Deterministic Recording Guarantee (v0.6+)

When recording from a PCAP source, the VRLOG guarantees a **1:1 mapping from
sensor rotations to VRLOG frames**. Given the same PCAP file, tuning
parameters, and build version, repeated recordings produce identical frame
counts.

### Variable Rotation Rate

The Hesai Pandar40P operates at 10–20 Hz depending on motor configuration.
The motor speed can vary within a single capture session. A frame boundary
is defined by **azimuth wrap-around** (360° rotation detected by the
FrameBuilder), not by a fixed time interval. Consequently:

- Inter-frame intervals are **not uniform** — they vary with motor speed.
- Each `TimestampNs` in the index reflects the actual rotation start time.
- Replay tools must use per-frame timestamps for pacing, not assumed Hz.

### Mechanism

1. **Rotation-triggered framing** — the FrameBuilder detects the end of each
   sensor rotation via azimuth wrap-around (≥ 340° coverage, ≥ 10 000 points).
   One rotation produces exactly one frame, regardless of rotation duration.

2. **Blocking frame channel** — the FrameBuilder uses a blocking channel send
   for all PCAP replay modes (analysis and scaled), providing back-pressure
   to the PCAP reader. No frames are silently dropped at the channel level.

3. **Empty frame recording** — when the perception pipeline determines a
   sensor rotation has no foreground objects (no moving targets), it still
   records a `FrameTypeEmpty` placeholder. The frame preserves the rotation's
   timestamp and monotonic ID.

4. **Throttle-safe recording** — when the pipeline's frame-rate throttle skips
   expensive clustering/tracking, the throttled frame is still recorded as a
   `FrameTypeEmpty` placeholder.

### Invariant

For a PCAP file producing _N_ sensor rotations within the configured duration
window:

```
VRLOG total_frames == N   (always)
```

This invariant holds regardless of playback speed, motor RPM, background model
configuration, or pipeline processing latency. Inter-frame intervals vary
with the sensor's actual rotation rate.

## Replay Mechanics

The `Replayer` supports:

- **Sequential playback** — `ReadFrame()` advances `currentFrame` and returns
  the next `FrameBundle` with `PlaybackInfo` injected.
- **Random seek by frame** — `Seek(frameIdx)` sets the read cursor.
- **Seek by timestamp** — `SeekToTimestamp(ns)` finds the nearest frame via
  linear scan (binary search planned).
- **Rate control** — `SetRate(rate)` adjusts playback speed (consumed by
  the gRPC streaming loop).
- **Pause/resume** — `SetPaused(paused)` pauses the streaming loop.

## Related

- [Protobuf schema](../../proto/velocity_visualiser/v1/visualiser.proto) — gRPC API contract and target serialisation format
- [FrameBundle model](../../internal/lidar/visualiser/model.go) — canonical Go struct
- [Recorder / Replayer](../../internal/lidar/visualiser/recorder/recorder.go) — read/write implementation
- [gen-vrlog tool](../../cmd/tools/gen-vrlog/main.go) — CLI tool to generate sample recordings
