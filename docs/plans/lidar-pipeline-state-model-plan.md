# LiDAR pipeline state model (v0.5.2)

- **Status:** Complete
- **Layers:** LiDAR pipeline (Go server, HTTP API, gRPC streaming, macOS visualiser)
- **Target:** v0.5.2; the settle-before-recording workflow landed on a state model that could not describe it
- **Canonical:** [data-source-switching.md](../lidar/operations/data-source-switching.md)

## Motivation

There was no single answer to "what is driving the pipeline right now, and what
is being captured from it". The answer was split across three stores that could
disagree, and the surfaces reporting it were variously stale, partial, or
hardcoded. An operator watching the `:8081` status page, the web UI, or the
macOS visualiser could not tell live from PCAP replay from VRLOG replay, and
could not tell whether a VRLOG was being recorded at all.

The immediate trigger was `settle_before_recording`, a two-pass PCAP replay that
settles the background grid unrecorded, reloads it, then replays the same window
while recording. Both passes reported identical state, and packet progress reset
between them, so a progress display ran 0-100% twice with nothing to say why.

## Current state (before this work)

Three stores:

1. `server.Server.currentSource` (`live`/`pcap`/`pcap_analysis`) under
   `dataSourceMu`, plus `pcapInProgress` and friends under a **different**
   mutex, `pcapMu`.
2. `l9endpoints.Server.replayMode` and `vrlogMode` booleans, plus
   `Publisher.vrlogActive`.
3. VRLOG recording as closure locals in `internal/cmd/server/radar.go`.

Four vocabularies: the three-value `DataSource`, `PlaybackStatusInfo.Mode`
(`live`/`pcap`/`vrlog`), the gRPC boolean pair, and proto `is_live` + `seekable`.

## Findings

| Area                         | Current state                                                                                                                                                  | Severity | Release view |
| ---------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | ------------ |
| `/api/lidar/playback/status` | `Config.GetPlaybackStatus` had zero assignment sites, so the handler always returned a hardcoded live status                                                   | Critical | v0.5.2       |
| `/api/lidar/data_source`     | VRLOG load/stop never touched the source, so it reported `live` throughout a VRLOG replay                                                                      | Critical | v0.5.2       |
| HTML status page             | Mode switch had no `pcap_analysis` case, rendering "Live UDP" over a preserved PCAP grid                                                                       | High     | v0.5.2       |
| Live ingest during replay    | VRLOG load left the UDP listener running, and `Publish` gated only background snapshots, so live and replayed frames interleaved and both reached the recorder | High     | v0.5.2       |
| Recording state              | Unobservable: a goroutine-local bool plus closure locals                                                                                                       | High     | v0.5.2       |
| Two-pass replay              | Settling and recorded passes indistinguishable from outside                                                                                                    | Medium   | v0.5.2       |
| `DataSourceManager`          | Held a shadow source production never wrote; `GetCurrentSource()` and `CurrentSource()` disagreed                                                              | Medium   | v0.5.2       |
| Locking                      | `currentSource` and `pcapInProgress` under different mutexes, read sequentially; `currentPCAPFile` written without a lock on the `StartPCAPInternal` path      | Medium   | v0.5.2       |
| Wire contract                | Mode encoded implicitly; the Swift client inferred it from two booleans                                                                                        | Medium   | v0.5.2       |

## Design

`PipelineState` in [`internal/lidar/server/pipeline_state.go`](../../internal/lidar/server/pipeline_state.go)
is the authoritative value, guarded by a `stateMu` held only for struct copies.

Two invariants carry the design:

- **Source and grid retention are separate axes.** `pcap_analysis` is a derived
  wire token, not a stored source. This is what lets resume-live report a live
  source with the grid still preserved.
- **Mode has one owner.** The monitor server initiates every transition, so it
  owns the mode. The streaming layer owns replay position, which changes via
  gRPC without HTTP involvement. Mode pushes outward; position is pulled on
  demand through a `PlaybackProbe` that deliberately carries no mode field.

`stateMu` must never be held across a call into another subsystem:
`onRecordingStart` reaches `applyRecordingMetadata`, which calls back into
`Server.CurrentSource()`.

`pcap_in_progress` stays narrowly "PCAP source with a replay running".
`Client.WaitForPCAPComplete` gates on it and the sweep runner calls that once
per combination, so widening it would hang every sweep during a VRLOG replay.

On the wire, a `SourceMode` enum carries the same vocabulary. `is_live` stays
populated, and `SOURCE_MODE_UNSPECIFIED` leaves older clients on their existing
inference. Seekability remains an independent axis: the Swift client derives the
badge label from the source and seek availability from `seekable` directly.

## Scope

All items delivered on branch `claude/pcap-state-backend-aef738`.

| Item | Summary                                                                                                                |
| ---- | ---------------------------------------------------------------------------------------------------------------------- |
| 1    | `PipelineState` store with a closed mutator set                                                                        |
| 2    | Migrate ~80 call sites; delete the old fields, the torn reads, and the unsynchronised write                            |
| 3    | Truthful HTTP surfaces: delete the dead callback, add `PlaybackProbe`, report VRLOG and recording, fix the status page |
| 4    | Remove the divergent `DataSourceManager` accessors and shadow source                                                   |
| 5    | Name the settling and recording passes                                                                                 |
| 6    | Stop the live listener during VRLOG replay                                                                             |
| 7    | Suppress live frames while a VRLOG replay is active                                                                    |
| 8    | `SourceMode` enum and `recording` on proto `PlaybackInfo`                                                              |
| 9    | Swift reads the source mode instead of inferring it                                                                    |
| 10   | Documentation, MATRIX corrections, backlog                                                                             |

## Follow-up

- ~~Confirm whether the explicit `SendBackgroundSnapshot()` call at VRLOG replay
  start overwrites the replay's own recorded background in the client cache.~~
  **Resolved.** It did. The live grid no longer overwrites a replay's own
  background, and a replay emits its recorded background at load. The remaining
  gap — the client's stream restarting _after_ that background was published, so
  the new stream missed it — is covered by
  [stream robustness](lidar-visualiser-stream-robustness-plan.md), which hands
  each subscribing client the current background.

## Parking waits for live

A finished replay parks: the recording stays the source, its last frame stays on
screen, and the operator decides what happens next. That was right as far as it
went, and wrong in one case — a replay loaded yesterday was still the source
this morning with the sensor streaming, because nothing was watching.

Parking now starts the live listener and watches for a packet. The recording
remains the source until one actually arrives, so the last frame is held rather
than replaced by an empty grid, and a quiet sensor leaves the replay parked
indefinitely as before. Only packets arriving after the park count:
`LastPacketAt` survives a replay, so an older timestamp says the sensor was
streaming before, not that it is streaming now.

The server owns the decision, so every client sees the same source. The watcher
is cancelled whenever something else decides — an operator going live, or
another replay starting — so one armed under an earlier replay cannot fire under
a later one.

## Related

- [Stream robustness](lidar-visualiser-stream-robustness-plan.md) — delivering this state to the client reliably
- [Data source switching](../lidar/operations/data-source-switching.md) — canonical operations reference
- [PCAP analysis mode](../lidar/operations/pcap-analysis-mode.md) — analysis replays and two-pass settling
- [Metrics registry](../platform/architecture/metrics-registry.md) — the source-mode vocabulary this adopts
