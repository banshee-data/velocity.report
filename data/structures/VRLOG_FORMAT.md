# VRLOG wire format specification

## Overview

A **`.vrlog`** is a directory. Two layouts share the name and nothing else:

| Layout                                                                            | Holds                                                                        | Written and read by                                                                                                                                                                                                            |
| --------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| VRLOG 0.5 FrameBundle recording (this section onwards)                            | Timestamped `FrameBundle` display snapshots, for replay, labelling, analysis | [`recorder.go`](../../internal/lidar/l9endpoints/recorder/recorder.go)                                                                                                                                                         |
| [VRLOG 1.x observation container](#vrlog-1x-observation-container) (from 0.5.2.2) | Typed, checksummed L4 evidence: `l4bobserve` foreground-complete frames      | [`storage/vrlog`](../../internal/lidar/storage/vrlog/doc.go); `velocity lidar pcap-replay --observations` and [`observations`](../../internal/cmd/lidar/observations.go), [`vrlog-check`](../../cmd/tools/vrlog-check/main.go) |

Each reader refuses the other's directories by name and root magic, never by guessing from
payloads. The rest of this overview and the sections up to the 1.x container describe 0.5.

**Version:** 0.5
**Source:** [`internal/lidar/l9endpoints/recorder/recorder.go`](../../internal/lidar/l9endpoints/recorder/recorder.go)

## Directory layout

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

| Field                  | Type    | Description                                                            |
| ---------------------- | ------- | ---------------------------------------------------------------------- |
| `version`              | string  | Format version (currently `"0.5"`)                                     |
| `created_ns`           | int64   | Wall-clock creation time (Unix nanoseconds)                            |
| `sensor_id`            | string  | Sensor identifier (e.g. `"hesai-01"`)                                  |
| `total_frames`         | uint64  | Total records written; equals the number of `index.bin` entries        |
| `rotation_frames`      | uint64  | Sensor rotations only: foreground, full and empty frames               |
| `background_frames`    | uint64  | Background snapshots. `total_frames` = rotations + snapshots           |
| `start_ns`             | int64   | Timestamp of the first frame (Unix nanoseconds)                        |
| `end_ns`               | int64   | Timestamp of the last frame (Unix nanoseconds)                         |
| `coordinate_frame`     | object  | Coordinate frame metadata (see below)                                  |
| `source_type`          | string  | Recording source: `"live"`, `"pcap"`, `"synthetic"` (omitted if empty) |
| `pcap_path`            | string  | Original PCAP filename, basename only (omitted if empty)               |
| `playback_rate`        | float64 | Configured replay speed multiplier (omitted if 0)                      |
| `tuning_hash`          | string  | SHA-256 hex digest of the tuning config JSON (omitted if empty)        |
| `build_version`        | string  | velocity.report binary version that created this VRLOG                 |
| `build_git_sha`        | string  | Git SHA of the binary that created this VRLOG                          |
| `config_build_version` | string  | Binary version that composed the immutable run configuration           |
| `config_build_git_sha` | string  | Git SHA that composed the immutable run configuration                  |

`version` describes the VRLOG format. It is not the application version.
`build_version` and `build_git_sha` identify the recorder binary, even when
the recording reuses an immutable run configuration composed by a different
build. The `config_build_*` pair preserves that separate configuration
provenance.

### Planned S2 provenance extension

The upcoming geographic-indexing phase will add two optional fields to the
header of a VRLOG derived from a located static PCAP segment:

| Field          | Type   | Description                                             |
| -------------- | ------ | ------------------------------------------------------- |
| `s2_l13_token` | string | Canonical fine-cell token copied from split metadata    |
| `s2_l10_token` | string | Canonical `Parent(10)` token copied from split metadata |

For example, a source named
`capture-static-0-s2-l10-808581.pcap` produces
`capture-static-0-s2-l10-808581.vrlog/`, whose `header.json` carries
`"s2_l13_token": "80858004"` and `"s2_l10_token": "808581"`. The
canonical tokens are machine data; family displays and UI-only spacing do not
belong in either the path or header.

The recorder copies these fields from trusted PCAP split metadata. It must not
recover them by parsing the source filename. When both are present, validation
requires the CellID represented by `s2_l13_token` to have
`Parent(10) == s2_l10_token`, and requires the L10 filename tag to agree.
Legacy recordings, moving captures, and sensor-local recordings without
accepted WGS84 provenance may omit both fields; storing only one is invalid.
Adding the fields requires a wire-format version bump and analyser support.
The full cross-artefact contract lives in the [S2 geographic-indexing
plan](../../docs/plans/s2-geographic-indexing-plan.md#static-pcap-and-vrlog-artefact-conformance).

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

Fixed-size binary seek index: one entry per frame, written in order. All fields
are **little-endian**.

### Entry layout (24 bytes)

| Offset | Size | Type   | Field         | Description                               |
| ------ | ---- | ------ | ------------- | ----------------------------------------- |
| 0      | 8    | uint64 | `FrameID`     | Monotonic frame identifier                |
| 8      | 8    | int64  | `TimestampNs` | Frame timestamp (Unix nanoseconds)        |
| 16     | 4    | uint32 | `ChunkID`     | Zero-based chunk file index               |
| 20     | 4    | uint32 | `Offset`      | Byte offset of the frame within the chunk |

The total file size is `24 × total_frames` bytes. To seek to frame _N_, read
`index.bin` at offset `24 × N`, extract `ChunkID` and `Offset`, then read
the frame from the corresponding chunk file.

## Chunk files (`frames/chunk_NNNN.pb`)

Each chunk file contains a sequence of length-prefixed serialised frames.

### Frame encoding

```
┌──────────────┬────────────────────────────────────┐
│ uint32 LE    │ Frame data                         │
│ (4 bytes)    │ (variable length)                  │
│ = byte count │ = protobuf-serialised FrameBundle  │
└──────────────┴────────────────────────────────────┘
```

Frames are concatenated sequentially within the chunk with no padding or
delimiters beyond the length prefix.

### Serialisation format

**Written:** protobuf-serialised `FrameBundle`, per the
[protobuf schema](../../proto/velocity_visualiser/v1/visualiser.proto). The
recorder has no other mode: `frameSerializer` is bound to the protobuf
serialiser in [`recorder.go`](../../internal/lidar/l9endpoints/recorder/recorder.go),
and the `.pb` chunk extension describes what is actually inside.

**Read:** protobuf, or JSON for recordings made before the protobuf migration
(#381). `detectFrameEncoding` probes protobuf first and falls back to Go
`encoding/json`, so a legacy log still replays without conversion.

> **Note:** JSON is a read-only compatibility path for existing recordings, not
> a format anything still produces. It is retained because those recordings
> remain useful; nothing new is written in it.

### Chunk rotation policy

A new chunk file is created when **any** of the following conditions are met:

| Condition             | Threshold      |
| --------------------- | -------------- |
| Frame count per chunk | 1,000 frames   |
| Byte size per chunk   | 150 MB written |

The read-side hard limit is **200 MB** per chunk (rejects chunks larger than
this to prevent excessive memory allocation).

## FrameBundle model

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

### FrameType values

| Value | Constant              | Description                                          |
| ----- | --------------------- | ---------------------------------------------------- |
| 0     | `FrameTypeFull`       | Legacy: all points included                          |
| 1     | `FrameTypeForeground` | Split streaming: foreground + clusters + tracks only |
| 2     | `FrameTypeBackground` | Background grid snapshot                             |
| 3     | `FrameTypeDelta`      | Reserved for future incremental updates              |
| 4     | `FrameTypeEmpty`      | Placeholder: no foreground objects detected (v0.5+)  |

`FrameTypeEmpty` frames carry only metadata (`FrameID`, `TimestampNanos`,
`SensorID`, `CoordinateFrame`). All perception fields (`PointCloud`,
`Clusters`, `Tracks`) are nil. These frames exist for deterministic 1:1
PCAP-to-VRLOG mapping and must be preserved during replay/seek.

Full model definition: [`internal/lidar/l9endpoints/model.go`](../../internal/lidar/l9endpoints/model.go)

## Deterministic recording guarantee (v0.5+)

When recording from a PCAP source, the VRLOG guarantees a **1:1 mapping from
sensor rotations to VRLOG frames**. Given the same PCAP file, tuning
parameters, and build version, repeated recordings produce identical frame
counts.

### Variable rotation rate

The Hesai Pandar40P operates at 10–20 Hz depending on motor configuration.
The motor speed can vary within a single capture session. A frame boundary
is defined by **azimuth wrap-around** (360° rotation detected by the
FrameBuilder), not by a fixed time interval. Consequently:

- Inter-frame intervals are **not uniform**: they vary with motor speed.
- Each `TimestampNs` in the index reflects the actual rotation start time.
- Replay tools must use per-frame timestamps for pacing, not assumed Hz.

### Mechanism

1. **Rotation-triggered framing**: the FrameBuilder detects the end of each
   sensor rotation via azimuth wrap-around (≥ 340° coverage, ≥ 10 000 points).
   One rotation produces exactly one frame, regardless of rotation duration.

2. **Blocking frame channel**: the FrameBuilder uses a blocking channel send
   for all PCAP replay modes (analysis and scaled), providing back-pressure
   to the PCAP reader. No frames are silently dropped at the channel level.

3. **Empty frame recording**: when the perception pipeline determines a
   sensor rotation has no foreground objects (no moving targets), it still
   records a `FrameTypeEmpty` placeholder. The frame preserves the rotation's
   timestamp and monotonic ID.

4. **Throttle-safe recording**: when the pipeline's frame-rate throttle skips
   expensive clustering/tracking, the throttled frame is still recorded as a
   `FrameTypeEmpty` placeholder.

### Invariant

For a PCAP file producing _N_ sensor rotations within the configured duration
window:

```
VRLOG rotation_frames == N   (always)
```

**Use `rotation_frames`, not `total_frames`, for this comparison.**
`total_frames` counts every record so that it matches `index.bin`, and a
recording also stores background snapshots, which are pipeline state rather
than observations of the street. A recording deliberately opens with one so a
replay has a scene from its first frame. Comparing `total_frames` with a source
rotation count, or with an analysis run's frame count, will always disagree by
the number of snapshots.

Background snapshots inherit the most recent foreground timestamp rather than
using wall-clock time. In the settle-before-recording flow the settling pass has
already swept the capture, so the opening snapshot can carry a timestamp from
the end of the range the recording covers. Snapshots are therefore not ordered
with the rotations around them; a consumer that needs a monotonic timeline
filters them out rather than trusting their timestamps. `start_ns` and `end_ns`
are already derived from rotations only.

This invariant holds regardless of playback speed, motor RPM, background model
configuration, or pipeline processing latency. Inter-frame intervals vary
with the sensor's actual rotation rate.

## Replay mechanics

The `Replayer` supports:

- **Sequential playback**: `ReadFrame()` advances `currentFrame` and returns
  the next `FrameBundle` with `PlaybackInfo` injected.
- **Random seek by frame**: `Seek(frameIdx)` sets the read cursor.
- **Seek by timestamp**: `SeekToTimestamp(ns)` finds the nearest frame via
  linear scan (binary search planned).
- **Rate control**: `SetRate(rate)` adjusts playback speed (consumed by
  the gRPC streaming loop).
- **Pause/resume**: `SetPaused(paused)` pauses the streaming loop.

## VRLOG 1.x observation container

Container version 1.0 holds L4 evidence that capture owns: one
[`l4bobserve.FrameRecord`](../../internal/lidar/l4bobserve/frame.go) per frame the extractor
received, in source sequence, and explicit gap records. Its payloads are the
[`velocity.recording.v1`](../../proto/velocity_recording/v1/recording.proto) protobuf schema,
which is independent of the visualiser's `FrameBundle`. It is delivery phase 1 of the
[shared VRLOG plan](../../docs/plans/lidar-vrlog-observation-format-plan.md): codec, a
sealed-chunk writer and the offline reader. Commit generations, the durable frontier and
recovery are phase 2.

Nothing writes this layout unless asked. `velocity lidar pcap-replay --observations DIR
--replay-case-id ID` writes one from a replay; `velocity lidar observations verify`,
`inspect` and `compare` read it, and `vrlog-check` verifies it.

### Container layout

```
<name>.vrlog/
├── manifest               # root: capture, extraction, profile, limits, provenance
├── chunks/
│   ├── 00000000.chunk     # sealed chunk: header, whole records, seal
│   ├── 00000000.index     # per-chunk index, derived from its chunk
│   └── ...
└── summary                # optional: written by a clean close
```

The legacy names (`header.json`, `index.bin`, `frames/`) never appear. Each object is written
under a `.open` name, synchronised, then renamed. A reader lists a remaining `.open` object as an
unsealed tail and never reads it as evidence. Integers are little-endian.

### Object preamble

Every object opens with 16 bytes:

| Offset | Size | Field       | Value                                                            |
| ------ | ---- | ----------- | ---------------------------------------------------------------- |
| 0      | 8    | Root magic  | `89 56 52 4C 0D 0A 1A 0A` (`\x89VRL\r\n\x1a\n`)                  |
| 8      | 4    | Object kind | `MANI`, `CHNK`, `INDX` or `SUMM`                                 |
| 12     | 2    | Major       | `1`; a reader refuses any other                                  |
| 14     | 2    | Minor       | `0`; additive only, since a required feature marks what must not |

The magic follows PNG's pattern: the high-bit byte, CR-LF, `^Z` and LF expose the usual
text-mode and 7-bit transfer damage.

### Record envelope

Every record, including the manifest, has a fixed 48-byte envelope before its payload:

| Offset | Size | Field                                                                            |
| ------ | ---- | -------------------------------------------------------------------------------- |
| 0      | 4    | Record magic `VREC`                                                              |
| 4      | 2    | Record kind                                                                      |
| 6      | 2    | Codec: `1`, an uncompressed protobuf message                                     |
| 8      | 8    | Container tag: first 8 bytes of the manifest object's SHA-256 (zero in manifest) |
| 16     | 8    | Sequence: frame sequence; a gap's first sequence, or the next one if it has none |
| 24     | 8    | Capture time: frame capture start, or a bounded gap's start; otherwise zero      |
| 32     | 4    | Stored payload length                                                            |
| 36     | 4    | Uncompressed payload length (equal for codec 1)                                  |
| 40     | 4    | Flags, zero in 1.0                                                               |
| 44     | 4    | CRC32C (Castagnoli) of bytes 0–43, then of the payload                           |

| Kind | Name         | Object and position                                   |
| ---- | ------------ | ----------------------------------------------------- |
| 1    | manifest     | The only record in `manifest`                         |
| 2    | chunk-header | First record of a chunk: ordinal and manifest SHA-256 |
| 3    | chunk-seal   | Last record of a chunk                                |
| 4    | chunk-index  | The only record in an index                           |
| 5    | summary      | The only record in `summary`                          |
| 16   | frame        | A chunk record: one `FrameRecord`                     |
| 17   | gap          | A chunk record: one `GapRecord`                       |

A kind with bit `0x8000` set is ancillary: a reader that does not know it skips it. Every other
kind is critical, and an unknown critical kind refuses the container as needing a newer reader.

### Manifest

`RecordingManifest` is written, synchronised and published before any record is admitted, and
never rewritten. The directory must not already exist. It carries:

- **Stream:** schema version `1`, stream kind `observation`, the evidence profile name and its
  sorted capability list, `required_features`, and the semantic digest version.
- **Capture:** a random UUID allocated before ingestion, sensor, source type, parent capture and
  creation time. The UUID and creation time are the only fields in which two replays of one
  source legitimately differ.
- **Extraction:** the `l4bobserve` source and calibration identities, coordinate frame, replay
  case, ordered capture files with SHA-256, extractor identity and configured window.
- **Geometry and provenance:** the embedded calibration (16 row-major values), declared limits,
  build and writer identity, parameter hash and experiments, and small metadata objects embedded
  by value with their SHA-256 (a replay embeds its effective tuning as `tuning`).

A reader refuses an unknown schema version, stream kind, digest version, profile or required
feature, a known profile whose capability list differs from its declaration, and limits above
the hard ceilings. It recomputes the source identity from the capture files, case and extractor,
and the calibration identity from the transform, and refuses a mismatch. Container 1.0 defines no
required features.

### Chunks, seals and indexes

A chunk holds whole records only: no record spans chunks, and no chunk depends on another's
protobuf state. The writer seals a chunk once it reaches the target size, and rotates before a
record would cross the hard bound. The seal records the ordinal, record/frame/gap counts, the
covered sequences `[first, end)`, the first and last record start, the body length, the SHA-256
of every byte before the seal, and the semantic digest of the chunk's records.

The index lists each record's offset, length, kind, covered sequences, start and end time, a
time-known flag and a clock epoch (always `0` in 1.0) as parallel arrays. It is derived data. At
open the reader checks it against the chunk's size and seal, without reading the body. A missing
or damaged index is refused unless rebuilding is requested; a rebuild finds the seal by record
lengths, checks the chunk digest first, and validates every record. It never overrides a failed
digest, and never rewrites a file.

A clean close writes `summary`: chunk count, a chain digest over the chunk bodies
(`chain_i = SHA-256(chain_{i-1} ‖ body_sha256_i)`, starting from 32 zero bytes), record, frame
and gap counts, the end sequence, total bytes, frames refused for exceeding a limit, and the
semantic digest of the whole stream. A container without a summary is readable but reported as
unclosed. With one, the sealed chunks must match it exactly, which detects a missing final chunk.

### Limits

| Limit                | Default and hard ceiling | Notes                                                         |
| -------------------- | ------------------------ | ------------------------------------------------------------- |
| Target chunk size    | 8 MiB                    | The writer seals at or just after it                          |
| Chunk bound          | 128 MiB                  | Never crossed; a reader refuses larger                        |
| Encoded frame record | 64 MiB                   | An over-limit frame becomes a gap over its sequence           |
| Records per chunk    | 65,536                   | Bounds an index to a few MiB                                  |
| Points per frame     | 1,048,576                | A Pandar40P returns at most 72,000 per rotation, 144,000 dual |
| Clusters per frame   | 65,536                   |                                                               |
| Stage counts         | 64                       |                                                               |

These are the plan's provisional values, to be revisited from dense-scene measurements. A writer
may declare smaller limits, and a reader enforces the declared ones. The small objects have fixed
bounds: manifest 1 MiB, index 4 MiB, summary 4 KiB, seal 1 KiB and chunk header 256 bytes. No
frame is ever truncated to fit. The writer returns a typed error, records a gap with cause
`writer-limit: …` in its place, and counts it in the summary.

### Frame payload

| Domain field (`l4bobserve`)       | Protobuf (`FrameRecord`)                                              | Presence                                     |
| --------------------------------- | --------------------------------------------------------------------- | -------------------------------------------- |
| Sequence, sensor frame, times     | `sequence`, `sensor_frame_id`, `sfixed64` frame/start/end             | Always                                       |
| Completeness, lost packets        | enum, `optional uint64`                                               | Absent loss is unknown, never zero           |
| Background, disposition           | enums; stage and reason strings                                       | Unknown enum values are refused              |
| Stage counts                      | `repeated StageCount`, each count `optional uint64`                   | Order kept; absent is unknown                |
| Retained points                   | `RetainedPoints` message                                              | Message present: the domain is recorded      |
| XYZ                               | packed `double`, `point_count` long                                   | Always, with the points                      |
| Time offset, intensity, channel … | `sint64`, `bytes`, `uint32` …; one value per point                    | `columns` bit set; values then exactly match |
| Membership                        | `Membership`: clusters (sorted members, summary), unassigned, reasons | Message present: the partition is recorded   |
| Cluster summary                   | `float` fields at L4's float32 precision; `OrientedBox` message       | Box message absent: L4 produced none         |

Ring zero and intensity zero are data, so presence is never inferred from a value. The domain
holds only valid returns, so there is no per-point validity column. Before protobuf decodes a
payload, a wire prescan counts every repeated field (packed or unpacked, summed across merged
occurrences) against the declared limits, and refuses columns whose lengths disagree.

### Semantic digest

Protobuf bytes are not a canonical identity across languages or library versions. Fidelity is
therefore judged on values, by `l4bobserve.FrameDigest`, `GapDigest` and `StreamDigest`,
version `l4bobserve.semantic/v1`. The stream is SHA-256 over fixed-width little-endian integers,
length-prefixed strings and arrays, and IEEE-754 bits (so −0 differs from +0 and NaN payloads
count). Fields follow the declared order, presence is an explicit byte, and arrays keep their
stored order. An absent value's contents are not hashed, and a nil column equals an empty one.
The writer seals the digest of the values it was given. `Verify` recomputes it from decoded values,
so every container carries its own codec-fidelity check. A pinned golden value, cross-checked
against an independent implementation, guards the definition; any change needs a new version.

### Reading order

1. Root: the manifest's preamble, envelope, CRC and the manifest checks above.
2. Catalogue: contiguous chunk ordinals; each index checked against its chunk's size and seal;
   sequences continuous across chunks from zero; summary consistency.
3. Chunk: read within the chunk bound; body SHA-256 against the seal before any record is
   returned; header names this chunk and this manifest.
4. Record: bounds before allocation; envelope against the index entry; CRC; prescan; decode;
   the record must satisfy the manifest's profile.

Any failure is a `CorruptionError` naming its kind (checksum, chunk digest, truncation, length,
structure, index disagreement, sequence break, missing object, invalid record, semantic digest or
summary), object, byte range and affected source sequences. A corrupt chunk fails normal reads
and the cursor does not move past it. Only an explicit seek continues beyond the damage.

`SeekSequence` lands on the frame with that sequence, or on the gap that covers it. `SeekTime`
takes an epoch (only `0` exists in 1.0; clock resets will add epochs, across which a time is
ambiguous). It lands on the first record starting at or after the time, or on the preceding
record when that one's interval contains it. It includes any unknown-time gaps immediately
before, since the loss may lie there.

### Compatibility

- The 0.5 replayer checks for the root magic before it looks for `header.json`, and refuses a
  container with `ErrObservationContainer`. It must: its payload probe decodes a recording
  `FrameRecord` as a `FrameBundle` without error, because protobuf treats fields it cannot place
  as unknown. A version string could not prevent that.
- The 1.x reader refuses a 0.5 recording by name (`ErrLegacyRecording`), and a directory without
  a manifest object as not a container.
- 0.5 recordings replay unchanged.
- Legacy SQLite/JSON observations (`reduced-cluster-sample`) are not transcoded into 1.x. A
  transcoder must declare the reduced profile and absent fields, and can never claim
  foreground-complete.
- Additive protobuf fields unknown to a reader are ignored; anything a reader must not ignore is
  a required feature.

## Related

- [Recording-domain schema](../../proto/velocity_recording/v1/recording.proto): VRLOG 1.x payloads
- [Observation container](../../internal/lidar/storage/vrlog/doc.go): 1.x writer and reader
- [Protobuf schema](../../proto/velocity_visualiser/v1/visualiser.proto): gRPC API contract and target serialisation format
- [FrameBundle model](../../internal/lidar/l9endpoints/model.go): canonical Go struct
- [Recorder / Replayer](../../internal/lidar/l9endpoints/recorder/recorder.go): read/write implementation
- [gen-vrlog tool](../../cmd/tools/gen-vrlog/main.go): CLI tool to generate sample recordings
