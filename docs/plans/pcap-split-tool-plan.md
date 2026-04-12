# PCAP split tool design document

- **Status:** Proposed
- **Canonical:** [pcap-analysis-mode.md](../lidar/operations/pcap-analysis-mode.md)

## Executive summary

This document describes the design for `pcap-split`, a Go command-line tool that automatically segments LIDAR PCAP files into non-overlapping periods of motion and stability. The tool enables separate analysis pipelines for mobile observation (driving) and static observation (parked) data collection scenarios.

## Problem statement

### Background

When collecting LIDAR data from a mobile observation vehicle, the dataset contains two fundamentally different data types:

1. **Static Periods** - Vehicle is parked, background model can settle, stable reference frame
2. **Motion Periods** - Vehicle is moving, background constantly changing, dynamic reference frame

These periods require different processing pipelines:

- **Static data**: High-quality background subtraction, accurate object detection, track analysis
- **Motion data**: SLAM/odometry processing, mobile mapping, dynamic scene reconstruction

### Current limitation

Currently, long PCAP captures from mobile observation sessions contain mixed motion/static data. Analysts must:

1. Manually review captures to identify static vs motion periods
2. Manually split PCAPs using external tools (tcpdump, editcap)
3. Manually track timing information for alignment
4. Risk human error in identifying transition points

### Proposed solution

Automated PCAP segmentation tool that:

1. Loads PCAP into LIDAR processing pipeline
2. Monitors background settling metrics in real-time
3. Detects motion/static transitions based on configurable thresholds
4. Splits PCAP into labeled segments with precise cut times
5. Outputs ready-to-analyse static and motion segments

## Use cases

### Primary use case: mobile observation route

**Scenario**: Observer drives route with multiple stop points

[Depart] → Drive 3min → [Stop A: 5min] → Drive 2min → [Stop B: 8min] → [Return]
**Expected Output**:

out-motion-0.pcap # Depart → Stop A (3 min)
out-static-0.pcap # Stop A parking (5 min)
out-motion-1.pcap # Stop A → Stop B, including 30s intersection wait (2 min)
out-static-1.pcap # Stop B parking (8 min)
out-motion-2.pcap # Return journey (variable)
**Key Requirements**:

- Keep 30-second intersection pauses within motion segments
- Only split after 60+ seconds of continuous stability (configurable)
- Precise cut times at exact moment motion stops

### Secondary use case: long-duration monitoring

**Scenario**: Overnight capture with brief vehicle movements

[Static: 4 hours] → [Car moves: 30s] → [Static: 4 hours]
**Expected Output**:

out-static-0.pcap # 4 hours stable
out-motion-0.pcap # 30s movement
out-static-1.pcap # 4 hours stable

### Tertiary use case: data quality assessment

**Scenario**: Evaluate background settling quality across multiple captures

for pcap in \*.pcap; do
pcap-split --pcap $pcap --output analysis/$pcap
done
Analysts can then:

- Compare settling rates across locations
- Identify problematic environments (moving trees, vibration)
- Validate sensor mounting stability

## Requirements

### Functional requirements

**FR1: PCAP Input Processing**

- Read standard PCAP/PCAPNG files
- Parse Hesai Pandar40P UDP packets (port 2369)
- Support files from 1GB to 100GB+
- Handle packet loss gracefully

**FR2: Background Settling Detection**

- Load PCAP through existing `BackgroundManager` pipeline
- Monitor frame-by-frame settling metrics:
  - Percent nonzero cells in last frame
  - Settled cell count (configurable threshold)
  - Within-bounds variance
  - Deviations from noise bounds
- Classify each frame as "stable" or "in-motion"

**FR3: Motion/Static Classification**

- **Stable State**: Background metrics stable for 60+ seconds (configurable)
- **Motion State**: Background metrics showing change/disruption
- Hysteresis to prevent chattering at transitions
- Configurable settling threshold (default: 60s continuous stability)

**FR4: PCAP Segmentation**

- Split input PCAP at detected transition points
- Output separate files for each motion/static segment
- Preserve packet integrity (no truncated frames)
- Sequential numbering: `out-static-0.pcap`, `out-motion-0.pcap`, `out-static-1.pcap`, ...

**FR5: Timestamp Alignment**

- Track timestamps for each segment:
  - Per-frame timestamp (LiDAR sensor time)
  - PCAP file offset
  - Optional: Global Unix epoch time
- Export CSV with segment timing metadata

**FR6: Configurable Parameters**

- Settling duration threshold (default: 60s)
- Settled cell threshold (default: varies by environment)
- Minimum segment duration (default: 5s, prevents micro-segments)
- Maximum motion gap to bridge (default: 30s, keeps intersection waits)

### Non-Functional requirements

**NFR1: Performance**

- Process 80K packets (28.7M points) in < 30 seconds
- Memory usage < 2GB for typical PCAP files
- Streaming processing (no full PCAP load required)

**NFR2: Reliability**

- Graceful handling of malformed packets
- Checkpoint/resume for interrupted processing
- Validation of output PCAP integrity

**NFR3: Usability**

- Single command execution
- Clear progress reporting
- Human-readable output summaries
- JSON metadata for programmatic access

**NFR4: Maintainability**

- Reuse existing `internal/lidar` components
- Follow repository conventions (Makefile, testing, docs)
- Comprehensive unit tests (target: 80%+ coverage)

## Design

### Architecture

┌─────────────────────────────────────────────────────────────┐
│ pcap-split CLI Tool │
│ (cmd/tools/pcap-split) │
└─────────────────────────────────────────────────────────────┘
│
▼
┌─────────────────────────────────────────────────────────────┐
│ Split Orchestrator │
│ • Manages state machine (motion/static detection) │
│ • Tracks segment boundaries │
│ • Coordinates reader/analyser/writer │
└─────────────────────────────────────────────────────────────┘
│ │ │
▼ ▼ ▼
┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐
│ PCAP Reader │ │ Settling │ │ PCAP Writer │
│ (network/) │ │ Analyser │ │ (pcapsplit/) │
│ │ │ (BackgroundMgr) │ │ │
│ • Parse packets │ │ │ │ • Buffer packets │
│ • Extract points │ │ • Track metrics │ │ • Write segments │
│ • Timestamp │ │ • Detect state │ │ • Sequential IDs │
└──────────────────┘ └──────────────────┘ └──────────────────┘

### Component details

#### 1. PCAP reader (reuse existing)

**Location**: `internal/lidar/network/pcap.go`

**Existing Capabilities**:

- `ReadPCAPFile()` - Reads PCAP, filters UDP, parses packets
- Integrates with `Parser` and `FrameBuilder` interfaces
- Reports packet statistics

**Usage in pcap-split**: create a `context.Background()`, `Pandar40PParser`, and `SettlingAnalyser`, then call `network.ReadPCAPFile(ctx, inputFile, 2369, parser, analyser, stats)`.

#### 2. Settling analyser (new component)

**Location**: `internal/lidar/pcapsplit/analyser.go` (new package)

**Responsibilities**:

- Implements `network.FrameBuilder` interface (including `SetMotorSpeed()`)
- Processes frames through `BackgroundManager`
- Tracks settling metrics per frame
- Tracks motor speed from packet stream for dynamic frame rate calculation
- Detects motion/static transitions
- Emits segment boundary events

**Key Metrics Tracked**:

**FrameMetrics** fields:

| Field              | Type        | Description                            |
| ------------------ | ----------- | -------------------------------------- |
| FrameID            | `int`       |                                        |
| Timestamp          | `time.Time` |                                        |
| PcapOffsetBytes    | `int64`     |                                        |
| TotalPoints        | `int`       |                                        |
| ForegroundPoints   | `int`       |                                        |
| BackgroundPoints   | `int`       |                                        |
| NonzeroCells       | `int`       |                                        |
| SettledCells       | `int`       | cells with TimesSeenCount >= threshold |
| FrozenCells        | `int`       |                                        |
| PercentNonzero     | `float64`   | nonzeroCells / totalCells              |
| PercentSettled     | `float64`   | settledCells / totalCells              |
| WithinBounds       | `bool`      | variance within expected noise bounds  |
| DeviationFromNoise | `float64`   | how far outside noise envelope         |
| IsStable           | `bool`      | computed stability classification      |

**State Machine**:

        ┌──────────┐
        │  Initial │
        └─────┬────┘
              │
              ▼
        ┌──────────┐     60s stable      ┌────────┐

┌────│ Motion │─────────────────────▶ Static │───┐
│ └──────────┘ └────────┘ │
│ ▲ │ │
│ │ motion detected │ │
│ └────────────────────────────────┘ │
│ │
└──────────────────────────────────────────────────┘

Notes:

- Transition to Static requires 60s (configurable) of sustained stability
- Transition to Motion requires 5s of sustained motion
- MaxMotionGapSec bridges short stable periods within motion segments
  **State Transition Logic**:

**classifyFrame** algorithm:

Stability criteria (all must be true):

- 1. Low foreground activity (< 5% of points)
- 2. High settled cell percentage (> 70%)
- 3. Low deviation from noise bounds (< 2.0 sigma)
- 4. Within expected variance bounds
- Compute frame rate from motor speed (RPM to Hz)
- Motor speed comes from parser.GetLastMotorSpeed() (see internal/lidar/parse/extract.go)
- Typical values: 600-1200 RPM → 10-20 Hz frame rate
- Require sustained stability before declaring static
- Require sustained motion before declaring motion
- MaxMotionGapSec: Bridge short stable periods (e.g., intersection waits)
- If stable duration < MaxMotionGapSec AND we were in motion, stay in motion
- Bridge this gap - don't transition to static yet
- Maintain previous state during ambiguous periods

#### 3. PCAP writer (new component)

**Location**: `internal/lidar/pcapsplit/writer.go` (new package)

**Responsibilities**:

- Buffer packets for current segment
- Write complete segments to disk on transition
- Generate sequential filenames
- Preserve PCAP header and packet metadata

**Implementation**:

**Segment** fields:

| Field          | Type          | Description          |
| -------------- | ------------- | -------------------- |
| outputDir      | `string`      |                      |
| currentSegment | `*Segment`    |                      |
| packetBuffer   | `[]RawPacket` |                      |
| staticCount    | `int`         |                      |
| motionCount    | `int`         |                      |
| Type           | `SegmentType` | "static" or "motion" |
| ID             | `int`         |                      |
| StartTime      | `time.Time`   |                      |
| EndTime        | `time.Time`   |                      |
| PcapOffset     | `int64`       |                      |
| PacketCount    | `int`         |                      |
| Filename       | `string`      |                      |

#### 4. Split orchestrator (new component)

**Location**: `cmd/tools/pcap-split/main.go`

**Responsibilities**:

- Parse CLI flags
- Initialise components
- Coordinate processing pipeline
- Generate summary report
- Export metadata JSON/CSV

**CLI Interface**:

- `pcap-split [options]`
- `Options:`
- `--pcap FILE             Input PCAP file (required)`
- `--output DIR            Output directory (default: current dir)`
- `--prefix NAME           Output filename prefix (default: "out")`
- `--settling-sec N        Settling duration threshold in seconds (default: 60)`
- `--min-segment-sec N     Minimum segment duration in seconds (default: 5)`
- `--max-motion-gap-sec N  Maximum motion gap to bridge in seconds (default: 30)`
- `--settled-threshold N   Settled cell count threshold (default: auto)`
- `--sensor-id ID          Sensor identifier (default: "hesai-pandar40p")`
- `--port N                UDP port (default: 2369)`
- `--export-metrics        Export detailed frame metrics CSV`
- `--export-json           Export segment metadata JSON`
- `--verbose               Verbose logging`
- `--help                  Show usage`
- `Examples:`
- Basic usage - split with defaults: `pcap-split --pcap capture.pcap --output ./segments`
- Custom settling threshold (faster splits): `pcap-split --pcap capture.pcap --settling-sec 30`
- Export metrics for analysis: `pcap-split --pcap capture.pcap --export-metrics --export-json`
- Bridge longer gaps (e.g., at traffic lights): `pcap-split --pcap capture.pcap --max-motion-gap-sec 60`
  **Output Files**:

output/
├── out-motion-0.pcap # First motion segment
├── out-static-0.pcap # First static segment
├── out-motion-1.pcap # Second motion segment
├── out-static-1.pcap # Second static segment
├── segments.json # Segment metadata (--export-json)
├── frame_metrics.csv # Per-frame metrics (--export-metrics)
└── summary.txt # Human-readable summary
**Segment Metadata JSON** (`segments.json`):

JSON object with fields: `input_file`, `processing_time_ms`, `total_packets`, `total_frames`, `config`, `settling_duration_sec`, `min_segment_sec`, `max_motion_gap_sec`, `settled_threshold`, `segments`, `type`, `id`, `filename`, `start_time`, `end_time`.
**Frame Metrics CSV** (`frame_metrics.csv`):

| frame_id | timestamp                | pcap_offset | total_points | foreground_points | nonzero_cells | settled_cells | percent_settled | deviation_from_noise | state    |
| -------- | ------------------------ | ----------- | ------------ | ----------------- | ------------- | ------------- | --------------- | -------------------- | -------- |
| 0        | 2025-01-15T10:30:00.000Z | 0           | 40000        | 8000              | 12000         | 0             | 0.00            | 5.2                  | motion   |
| 1        | 2025-01-15T10:30:00.100Z | 100000      | 40000        | 7500              | 12500         | 100           | 0.14            | 4.8                  | motion   |
| ...      |                          |             |              |                   |               |               |                 |                      |          |
| 195      | 2025-01-15T10:33:15.000Z | 19500000    | 40000        | 2000              | 60000         | 40000         | 55.56           | 1.2                  | motion   |
| 196      | 2025-01-15T10:33:15.100Z | 19600000    | 40000        | 1500              | 62000         | 45000         | 62.50           | 0.8                  | settling |
| ...      |                          |             |              |                   |               |               |                 |                      |          |
| 256      | 2025-01-15T10:34:15.000Z | 25600000    | 40000        | 500               | 65000         | 58000         | 80.56           | 0.3                  | static   |

**Summary Report** (`summary.txt`):

# PCAP Split Analysis Summary

Input File: /path/to/capture.pcap
Processing Time: 24.5s
Total Packets: 84,230
Total Frames: 842
Total Duration: 14m 2s

Configuration:
Settling Threshold: 60s
Min Segment Duration: 5s
Max Motion Gap: 30s

Segments:
Motion Segments: 3 (total: 8m 15s)
Static Segments: 2 (total: 5m 47s)

Detailed Breakdown:
[0] motion 10:30:00 → 10:33:15 (3m 15s) 19,500 packets
[1] static 10:33:15 → 10:38:15 (5m 00s) 30,000 packets (settled after 60s)
[2] motion 10:38:15 → 10:40:30 (2m 15s) 13,500 packets
[3] static 10:40:30 → 10:41:17 (0m 47s) 4,700 packets (settled after 60s)
[4] motion 10:41:17 → 10:44:02 (2m 45s) 16,530 packets

Output Files:
out-motion-0.pcap (19.5 MB)
out-static-0.pcap (30.0 MB)
out-motion-1.pcap (13.5 MB)
out-static-1.pcap (4.7 MB)
out-motion-2.pcap (16.5 MB)

Metrics Export:
segments.json (segment metadata)
frame_metrics.csv (per-frame metrics)

### Required API enhancements

#### Existing APIs to use

From `BackgroundManager`:

| Method                 | Parameters                           | Returns                  |
| ---------------------- | ------------------------------------ | ------------------------ |
| `GridStatus`           | ``                                   | `map[string]interface{}` |
| `GetGridHeatmap`       | `azimuthBucketDeg, settledThreshold` | `*GridHeatmap`           |
| `GetAcceptanceMetrics` | ``                                   | `*AcceptanceMetrics`     |

#### New API methods needed

**1. Per-Frame Settling Metrics** (add to `BackgroundManager`):

**FrameSettlingMetrics** fields:

| Field           | Type      | Description |
| --------------- | --------- | ----------- |
| TotalCells      | `int`     |             |
| NonzeroCells    | `int`     |             |
| SettledCells    | `int`     |             |
| FrozenCells     | `int`     |             |
| PercentNonzero  | `float64` |             |
| PercentSettled  | `float64` |             |
| PercentFrozen   | `float64` |             |
| ForegroundCount | `int64`   |             |
| BackgroundCount | `int64`   |             |

**2. Noise Bounds Deviation** (add to `BackgroundManager`):

- GetNoiseBoundsDeviation computes aggregate deviation from expected noise bounds

**GetNoiseBoundsDeviation** algorithm:

- Deviation in units of expected noise (sigma-like metric)
  **3. Within Bounds Check** (add to `BackgroundManager`):

- IsWithinNoiseBounds returns true if most cells are within expected noise envelope

**IsWithinNoiseBounds** algorithm:

### Data structures

#### Configuration

**SplitConfig** fields:

| Field                | Type               | Description                  |
| -------------------- | ------------------ | ---------------------------- |
| PCAPFile             | `string`           | Input/Output                 |
| OutputDir            | `string`           |                              |
| OutputPrefix         | `string`           |                              |
| SettlingDurationSec  | `float64`          | Default: 60.0                |
| MinSegmentSec        | `float64`          | Default: 5.0                 |
| MaxMotionGapSec      | `float64`          | Default: 30.0                |
| SettledCellThreshold | `uint32`           | Default: 50 (TimesSeenCount) |
| BackgroundParams     | `BackgroundParams` | Background Parameters        |
| SensorID             | `string`           | Sensor Configuration         |
| UDPPort              | `int`              |                              |
| ExportMetrics        | `bool`             | Export Options               |
| ExportJSON           | `bool`             |                              |
| Verbose              | `bool`             |                              |

#### State machine

**StateTransition** fields:

| Field         | Type         | Description |
| ------------- | ------------ | ----------- |
| type State    | `int`        |             |
| StateMotion   | `(embedded)` |             |
| StateStatic   | `(embedded)` |             |
| FromState     | `State`      |             |
| ToState       | `State`      |             |
| Timestamp     | `time.Time`  |             |
| FrameID       | `int`        |             |
| PcapOffset    | `int64`      |             |
| TriggerReason | `string`     |             |

### Processing algorithm

**High-Level Flow**:

1. Initialise components:
   - PCAP reader (network.ReadPCAPFile)
   - Parser (parse.NewPandar40PParser)
   - Settling analyser (implements FrameBuilder)
   - Segment writer

2. Process PCAP streaming:
   for each packet:
   a. Parser extracts points
   b. Analyser accumulates frame
   c. On frame completion: - Process through BackgroundManager - Compute settling metrics - Classify frame (motion/static) - Detect state transitions - Buffer packet for current segment - On transition: flush segment, start new

3. Finalize:
   - Flush final segment
   - Write metadata files
   - Generate summary report
     **Detailed Analyser Logic**:

**processFrame** algorithm:

- 1. Process through background manager
- 2. Count foreground/background
- 3. Get settling metrics
- 4. Build frame record
- 5. Classify frame state
- 6. Detect transition
- Notify writer of state change
- Log transition
- 7. Store metrics for export

## Technical considerations

### Performance optimisation

**1. Streaming Processing**

- Never load entire PCAP into memory
- Process packet-by-packet with buffering
- Flush segments to disk immediately on transition

**2. Background Manager Efficiency**

- Reuse existing optimised grid structure (40×1800 = 72K cells)
- Lock-free metrics reading where possible
- Batch metric computations per frame

**3. Packet Buffering**

- Buffer packets for current segment only (~100MB typical)
- Pre-allocate buffer capacity based on estimated segment size
- Immediate write on state transition to free memory

### Edge cases

**1. Very Short Segments**

[Motion 2s] → [Static 3s] → [Motion 2s]
**Handling**: Apply `min-segment-sec` threshold (default 5s)

- Segments < threshold are merged with previous segment
- Prevents micro-segments from intersection bumps

**2. Ambiguous Settling**

[Motion] → [Settling: oscillating metrics] → [Static?]
**Handling**: Hysteresis in state machine

- Require sustained stability (60s default) before declaring static
- Brief disruptions during settling don't restart counter
- Use frozen cell count as additional signal

**3. Long Intersection Wait**

[Motion] → [Red light: 45s stopped] → [Motion]
**Handling**: `max-motion-gap-sec` parameter (default 30s)

- Stops < 30s remain in motion segment
- Stops > 30s trigger static segment
- Configurable for different use cases

**4. PCAP Ends During Settling**

[Motion] → [Settling 30s] → [EOF]
**Handling**: Finalize partial segment

- Write incomplete segment with metadata flag
- Note settling was incomplete in metadata
- Still useful for analysis with caveat

### Error handling

**1. Malformed Packets**

- Skip packet, log warning
- Continue processing
- Report in summary (X packets skipped)

**2. Disk Full During Write**

- Abort gracefully
- Clean up incomplete segment file
- Report error with partial progress

**3. Parser Failure**

- Log detailed error with packet offset
- Attempt to continue with next packet
- Include error count in summary

### Testing strategy

**Unit Tests**:

- State machine logic (all transitions)
- Metric computation (edge cases)
- Segment naming and sequencing
- Metadata generation

**Integration Tests**:

- Small test PCAPs with known transitions
- Validate output PCAP integrity
- Verify split point accuracy
- Metadata consistency checks

**Performance Tests**:

- Large PCAP processing (1GB+)
- Memory usage profiling
- CPU utilisation monitoring

## Implementation plan

### Phase 1: core infrastructure (week 1)

**1.1 Package Structure**

- Create `internal/lidar/pcapsplit/` package
- Set up test infrastructure
- Add to build system (Makefile targets)

**1.2 Background Manager API Extensions**

- Implement `GetFrameSettlingMetrics(settledThreshold uint32)`
- Implement `GetNoiseBoundsDeviation()`
- Implement `IsWithinNoiseBounds(threshold float64)`
- Add unit tests for new methods

**1.3 Basic State Machine**

- Implement state definitions and transitions
- Basic classification logic (simplified)
- Unit tests for state machine

### Phase 2: PCAP splitting logic (week 2)

**2.1 Settling Analyser**

- Implement `FrameBuilder` interface
- Integrate with `BackgroundManager`
- Metric tracking per frame
- State classification algorithm

**2.2 Segment Writer**

- PCAP file writing (using gopacket)
- Sequential filename generation
- Packet buffering and flushing
- Output validation

**2.3 Integration Tests**

- Create small test PCAP files
- Known motion/static transitions
- Validate split accuracy

### Phase 3: CLI tool (week 3)

**3.1 Command-Line Interface**

- Flag parsing and validation
- Usage documentation
- Error handling and reporting

**3.2 Orchestrator**

- Coordinate reader/analyser/writer
- Progress reporting
- Summary generation

**3.3 Metadata Export**

- JSON segment metadata
- CSV frame metrics
- Human-readable summary

### Phase 4: polish and documentation (week 4)

**4.1 Performance Optimisation**

- Profile and optimise hot paths
- Memory usage optimisation
- Benchmark against target specs

**4.2 Documentation**

- User guide (this document)
- README for tool
- Code examples
- Troubleshooting guide

**4.3 Testing and Validation**

- Real-world PCAP testing
- Edge case validation
- Performance verification
- User acceptance testing

## Success criteria

### Functional success

✅ Tool processes 80K packet PCAP in < 30 seconds
✅ Correctly identifies motion/static transitions
✅ Generates valid PCAP segments
✅ Exports accurate metadata (JSON/CSV)
✅ Handles edge cases gracefully
✅ Clear, actionable error messages

### Quality success

✅ 80%+ code coverage (unit tests)
✅ Integration tests for all transition types
✅ Documentation complete and accurate
✅ Follows repository conventions
✅ No memory leaks (validated with profiling)
✅ Passes all linting and formatting checks

### User success

✅ Single command execution (no manual steps)
✅ Clear progress reporting during processing
✅ Intuitive parameter names and defaults
✅ Useful summary output
✅ Easy to integrate into analysis workflows

## Future enhancements

### Phase 5+: advanced features

**1. Multi-Sensor Support**

- Process multiple sensors simultaneously
- Cross-sensor motion correlation
- Fused motion detection

**2. Real-Time Mode**

- Live UDP streaming with on-the-fly splitting
- Continuous segmentation during collection
- Automatic archival of completed segments

**3. ML-Based Classification**

- Train model on labeled motion/static data
- More accurate transition detection
- Adaptive thresholds per environment

**4. Visualisation**

- Web UI for segment review
- Interactive timeline of motion/static periods
- Settling metric plots

**5. Cloud Integration**

- S3/blob storage output
- Distributed processing for large datasets
- API for programmatic access

## Related documentation

- [PCAP Analysis Mode](../lidar/operations/pcap-analysis-mode.md) - Web UI analysis workflow
- Background Subtraction (see [`internal/lidar/l3grid/background.go`](../../internal/lidar/l3grid/background.go)) - Settling algorithm details
- [LIDAR Tracking Pipeline](../lidar/architecture/foreground-tracking.md) - Object detection pipeline
- [Architecture](../../ARCHITECTURE.md) - System overview

## Glossary

**Background Grid**: 40×1800 cell grid tracking expected range values per azimuth/elevation

**Foreground**: Points that deviate significantly from background model (potential objects)

**Settled Cell**: Grid cell with sufficient observations (TimesSeenCount >= threshold) to be reliable

**Frozen Cell**: Grid cell temporarily locked (no updates) after foreground detection to prevent corruption

**Settling Period**: Duration required for background model to converge (typically 60+ seconds)

**Motion Period**: Time when vehicle is moving, background constantly changing

**Static Period**: Time when vehicle is parked, background stable

**Hysteresis**: Delay in state transitions to prevent rapid oscillation (requires sustained change)

**Frame**: Single 360° rotation of LIDAR sensor (~10 Hz typical)

**PCAP**: Packet Capture format (tcpdump, Wireshark standard)

## Revision history

| Version | Date       | Author  | Changes                 |
| ------- | ---------- | ------- | ----------------------- |
| 1.0     | 2025-12-06 | Ictinus | Initial design document |
