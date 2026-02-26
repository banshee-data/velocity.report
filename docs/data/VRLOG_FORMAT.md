# VRLOG Wire Format Specification

## Overview

A **`.vrlog`** file is a directory-based recording format for LiDAR frame data. It stores
timestamped `FrameBundle` snapshots from the velocity.report perception pipeline, enabling
seekable replay, labelling, and offline analysis.

**Version:** 1.0
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

| Field              | Type   | Description                                     |
| ------------------ | ------ | ----------------------------------------------- |
| `version`          | string | Format version (currently `"1.0"`)              |
| `created_ns`       | int64  | Wall-clock creation time (Unix nanoseconds)     |
| `sensor_id`        | string | Sensor identifier (e.g. `"hesai-01"`)           |
| `total_frames`     | uint64 | Total number of frames in the recording         |
| `start_ns`         | int64  | Timestamp of the first frame (Unix nanoseconds) |
| `end_ns`           | int64  | Timestamp of the last frame (Unix nanoseconds)  |
| `coordinate_frame` | object | Coordinate frame metadata (see below)           |

### coordinate_frame

| Field             | Type   | Description                  |
| ----------------- | ------ | ---------------------------- |
| `frame_id`        | string | e.g. `"site/hesai-01"`       |
| `reference_frame` | string | e.g. `"ENU"` (East-North-Up) |

### Example

```json
{
  "version": "1.0",
  "created_ns": 1740000000000000000,
  "sensor_id": "hesai-01",
  "total_frames": 12345,
  "start_ns": 1740000000000000000,
  "end_ns": 1740000600000000000,
  "coordinate_frame": {
    "frame_id": "site/hesai-01",
    "reference_frame": "ENU"
  }
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

**Current (v1.0):** JSON-serialised `FrameBundle` (Go `encoding/json`).

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

Full model definition: [`internal/lidar/visualiser/model.go`](../../internal/lidar/visualiser/model.go)

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
