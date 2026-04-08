# PCAP Motion Detection and Scene Split Plan

- **Status:** Proposed
- **Layers:** L1 Packets, L3 Grid, CLI Tools
- **Scope:** add a `--motion` analysis mode to pcap-analyse and implement the pcap-split tool from existing design, shipping movement detection as a two-phase capability
- **Related:** [pcap-split design doc](../lidar/operations/pcap-split-tool.md), [settling time optimisation](../lidar/operations/settling-time-optimisation.md), [adaptive region parameters](../lidar/operations/adaptive-region-parameters.md)

## 1. Problem

Long PCAP captures from mobile observation sessions contain mixed driving and parked data. The background model only functions during static periods — motion segments are unusable for perception but still occupy analysis time. Today an operator must manually scrub through captures, guess transition points, and split files with external tools (tcpdump, editcap). This is slow, error-prone, and blocks the mobile-observation workflow.

## 2. What Already Exists

| Capability                  | Location                                      | Status                                                             |
| --------------------------- | --------------------------------------------- | ------------------------------------------------------------------ |
| `CheckForSensorMovement()`  | `internal/lidar/l3grid/background_drift.go`   | Implemented — foreground-ratio spike detector (>20% threshold)     |
| `IsSettlingComplete()`      | `internal/lidar/l3grid/background_manager.go` | Implemented — settling convergence check                           |
| `GetGridStatus()`           | `internal/lidar/l3grid/background_manager.go` | Implemented — total/frozen/times-seen cell stats                   |
| Region classification       | `internal/lidar/l3grid/background_region.go`  | Implemented — stable/variable/volatile regions after settling      |
| Scene hash matching         | `internal/lidar/l3grid/background.go`         | Implemented — SHA256 hash for location fingerprinting              |
| pcap-analyse L1–L6 pipeline | `cmd/tools/pcap-analyse/main.go`              | Implemented — full pipeline with stats, benchmark, CSV/JSON export |
| pcap-split reference design | `docs/lidar/operations/pcap-split-tool.md`    | Design only — ~1,200-line spec, not implemented                    |

### Gap Analysis

The pcap-split design doc is comprehensive and sound. The primary gaps before shipping are:

1. **No per-frame settling metrics API** — `BackgroundManager` exposes settling completion and grid status but not the per-frame `FrameSettlingMetrics` struct the analyser needs (settled cell count, percent settled, noise deviation). These metrics exist internally but are not surfaced.
2. **No noise bounds deviation API** — the design calls for `GetNoiseBoundsDeviation()` and `IsWithinNoiseBounds()`; neither exists.
3. **No PCAP writer** — the project reads PCAPs (via gopacket) but never writes them. A new `SegmentWriter` with proper PCAP header handling is needed.
4. **pcap-analyse has no motion awareness** — it processes every frame identically regardless of sensor movement. Adding a `--motion` flag that tags frames and reports static/motion periods is a lightweight prerequisite that validates the detection algorithm before building the full splitter.

## 3. Design

### 3.1 Phased Delivery

**Phase 1 — Motion Detection in pcap-analyse** (`S`)

Add a `--motion` flag to pcap-analyse that:

- Runs `CheckForSensorMovement()` on every processed frame
- Computes per-frame settling metrics (settled cell %, foreground %, noise deviation)
- Classifies each frame as `motion` or `static` using the state machine from the pcap-split design doc
- Emits a motion timeline in the summary output and optionally in `--json` / `--csv` exports
- Adds a `motion_periods` section to `CaptureStats`:

```
Motion Timeline:
  [  0.0s –  195.0s]  motion   (3m 15s)
  [195.0s –  495.0s]  static   (5m 00s, settled after 60s)
  [495.0s –  630.0s]  motion   (2m 15s)
  [630.0s –  677.0s]  static   (0m 47s, settled after 60s)
```

This is safe to ship without the splitter — it gives operators immediate visibility into capture quality and validates the detection heuristics against real-world PCAPs.

**Phase 2 — BackgroundManager API Extensions** (`S`)

Expose the three new APIs the settling analyser requires:

| Method                                      | Purpose                                                      |
| ------------------------------------------- | ------------------------------------------------------------ |
| `GetFrameSettlingMetrics(settledThreshold)` | Per-frame settled/nonzero/frozen cell counts and percentages |
| `GetNoiseBoundsDeviation()`                 | Aggregate deviation from expected noise envelope             |
| `IsWithinNoiseBounds(threshold)`            | Boolean check for noise envelope compliance                  |

These are read-only accessors over existing grid data. No state changes, no new storage.

**Phase 3 — pcap-split Tool** (`M`)

Implement `cmd/tools/pcap-split/` per the [existing design doc](../lidar/operations/pcap-split-tool.md):

- `internal/lidar/pcapsplit/analyser.go` — settling analyser implementing `FrameBuilder`, state machine, metric tracking
- `internal/lidar/pcapsplit/writer.go` — segment PCAP writer with sequential naming and packet buffering
- CLI orchestrator with flags from the design doc
- JSON/CSV metadata export and human-readable summary
- Makefile target: `build-pcap-split`

### 3.2 Architecture

```
                     Phase 1                              Phase 3
              ┌─────────────────┐                  ┌─────────────────┐
              │  pcap-analyse   │                  │   pcap-split    │
              │  --motion flag  │                  │   CLI tool      │
              └────────┬────────┘                  └────────┬────────┘
                       │                                    │
                       ▼                                    ▼
              ┌─────────────────────────────────────────────────────┐
              │              Settling Analyser                      │
              │  • Frame classification (motion/static)             │
              │  • State machine with hysteresis                    │
              │  • Metric collection per frame                      │
              └────────────────────┬────────────────────────────────┘
                                   │
              Phase 2              ▼
              ┌─────────────────────────────────────────────────────┐
              │           BackgroundManager Extensions              │
              │  • GetFrameSettlingMetrics(threshold)               │
              │  • GetNoiseBoundsDeviation()                        │
              │  • IsWithinNoiseBounds(threshold)                   │
              └─────────────────────────────────────────────────────┘
```

Phase 1 can use the existing `CheckForSensorMovement()` and `GetGridStatus()` for a simpler initial classifier. Phase 3 upgrades to the full settling analyser with the Phase 2 API extensions.

### 3.3 State Machine (from pcap-split design)

```
      ┌──────────┐
      │  Initial │
      └────┬─────┘
           │
           ▼
      ┌──────────┐    60s stable     ┌────────┐
 ┌────│  Motion  │──────────────────►│ Static │────┐
 │    └──────────┘                   └────────┘    │
 │         ▲         5s motion            │        │
 │         └──────────────────────────────┘        │
 └─────────────────────────────────────────────────┘
```

- Motion → Static: 60s sustained stability (configurable via `--settling-sec`)
- Static → Motion: 5s sustained motion
- Intersection bridging: stops < 30s stay in motion (configurable via `--max-motion-gap-sec`)

### 3.4 Detection Criteria

A frame is classified as **stable** when all four conditions hold:

1. **Foreground activity < 5%** of total points
2. **Settled cells > 70%** (cells with `TimesSeenCount >= threshold`)
3. **Noise deviation < 2.0 sigma**
4. **Within expected variance bounds**

Phase 1 uses a simplified version: conditions 1 and 2 only (available from existing APIs). Phase 3 adds conditions 3 and 4 via Phase 2 API extensions.

## 4. Failure Modes

| Failure                                | Impact                                | Mitigation                                                  |
| -------------------------------------- | ------------------------------------- | ----------------------------------------------------------- |
| False motion trigger (wind, tree sway) | Over-segmentation                     | Hysteresis (60s sustained stability requirement)            |
| Missed short stops at intersections    | Short static segments in motion data  | `--max-motion-gap-sec` bridges stops under threshold        |
| PCAP ends mid-settling                 | Incomplete final segment              | Write partial segment with `incomplete: true` metadata flag |
| Very large PCAP (>100 GB)              | Memory pressure from packet buffering | Streaming write — flush segment to disk on each transition  |

## 5. Testing Strategy

| Phase   | Tests                                                                                                                                                       |
| ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Phase 1 | Unit tests for frame classifier; integration test with known-motion PCAP; validate motion timeline matches manual observation                               |
| Phase 2 | Unit tests for each new `BackgroundManager` method; property: metrics are consistent with existing `GetGridStatus()`                                        |
| Phase 3 | State machine unit tests (all transitions, edge cases); integration tests with crafted PCAPs; output PCAP integrity validation; metadata consistency checks |

## 6. Effort and Dependencies

| Phase                                | Effort | Dependencies                             |
| ------------------------------------ | ------ | ---------------------------------------- |
| Phase 1 — `--motion` in pcap-analyse | `S`    | None — uses existing APIs                |
| Phase 2 — BackgroundManager API      | `S`    | None                                     |
| Phase 3 — pcap-split tool            | `M`    | Phase 2; gopacket PCAP writer capability |

Total: `M` (Phases 1+2 are `S` each and can ship independently; Phase 3 is the bulk).

## 7. What This Plan Does Not Cover

- **Real-time splitting** — live UDP stream segmentation (future Phase 5 in the design doc)
- **ML-based classification** — the rule-based state machine is sufficient for the mobile-observation use case
- **SLAM/odometry for motion segments** — motion segments are split out for future processing, not processed here
- **Multi-sensor fusion** — single-sensor only
