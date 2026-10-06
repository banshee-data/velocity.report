# Runtime pipeline correctness

This document records the invariants that keep captured measurements trustworthy as they pass
through ingest, replay, persistence, and the API. It is deliberately about behaviour rather than
package layout: moving code is cheap; preserving a quiet data-loss bug is not.

Active plan: [go-runtime-pipeline-correctness-plan.md](../../plans/go-runtime-pipeline-correctness-plan.md).

## Delivered baseline

The foundations below are present in the current implementation and its focused tests. They are
not evidence that every remediation phase in the active plan is complete.

| Delivered boundary               | Current behaviour                                                                                                                                                                                                                    | Evidence                                                                                                                             |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------ |
| Named sensor capabilities        | The capabilities response distinguishes named radar and LiDAR sensor state, including the default radar and disabled LiDAR state. The runtime capability provider exposes explicit starting, ready, error, and disabled transitions. | `internal/api/server.go`, `internal/cmd/server/capabilities.go`, and `internal/cmd/server/capabilities_test.go`                      |
| Shared safe-path primitive       | A common path validator resolves filesystem paths before accepting them beneath an allowed directory, with coverage for symlink-sensitive cases. It is available for replay code to adopt rather than recreating path checks.        | `internal/security/pathvalidation.go` and `internal/security/pathvalidation_test.go`                                                 |
| Magnitude-only input recognition | The radar raw-data classifier recognises magnitude-only serial payloads, so this supported input shape reaches the persistence/transit boundary rather than being rejected at classification.                                        | `internal/serialmux/parse.go` and `internal/serialmux/handlers_test.go`                                                              |
| Magnitude-only transit contract  | Magnitude-only rows are stored diagnostics, not transit inputs. Transit derivation and gap detection read only rows with a speed, so such a row can neither fail a window's scan nor leave its hour as a permanent gap.              | `internal/db/transit_worker.go`, `internal/db/transit_gaps.go` and `internal/db/transit_magnitude_only_test.go`                      |
| Replay control surfaces          | PCAP and VRLOG replay handlers exist and expose their replay modes through the server.                                                                                                                                               | `internal/lidar/server/playback_handlers.go`                                                                                         |
| PCAP analysis default and pacing | An omitted `analysis_mode` keeps the true default, and the clients send it explicitly. Analysis-mode replays bypass the wall-clock frame-rate throttle, so a foreground burst reaches clustering and tracking.                       | `internal/lidar/server/datasource_handlers.go`, `internal/lidar/server/client.go` and `internal/lidar/pipeline/tracking_pipeline.go` |
| VRLOG load validation            | `handleVRLogLoad` resolves the requested path with the shared symlink-safe validator and loads the canonical resolved path.                                                                                                          | `internal/lidar/server/playback_handlers.go` and `internal/lidar/server/playback_api_test.go`                                        |

The LiDAR lifecycle remains only partially wired: startup records a starting state when the LiDAR
server is created, but the runtime does not yet report the observed ready or error outcome. The
transition API is delivered; the lifecycle truthfulness work is still owned by the active plan.

## Invariants

- A PCAP analysis run must create and retain semantically processed output, not merely preserve
  frame counts while wall-clock throttling discards the useful work.
- Accepted radar raw rows must never break transit derivation. Magnitude-only input is accepted and
  stored as diagnostics; transits, which are speed sessions, are derived from rows with a speed.
- LiDAR capabilities must move from startup into an observed ready or error state when the runtime
  knows the outcome.
- Replay file access must use the shared symlink-safe path validator rather than a second,
  less-careful interpretation of a safe directory.

## Remediation status

The following gaps remain open. They are stated here to prevent the delivered foundations above
from being mistaken for completed correctness work.

| Plan area                   | Current boundary                                                                                                                                         | Graduation status       |
| --------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------- |
| LiDAR readiness and failure | Capability transition methods are available, but the LiDAR runtime has not yet connected successful initialisation and failures to ready or error state. | Phase 4 remains active. |

## Delivery boundary

The active plan owns sequencing, remediation checklists, phase status, and acceptance evidence.
This document records only verified delivery boundaries and the invariants they must satisfy; it
does not advance a plan phase or substitute implementation intent for evidence. Related clock,
performance, metrics, extraction, and UI plans retain their own scope and consume these invariants
rather than quietly redefining them.
