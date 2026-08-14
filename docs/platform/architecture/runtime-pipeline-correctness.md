# Runtime pipeline correctness

This document records the invariants that keep captured measurements trustworthy as they pass
through ingest, replay, persistence, and the API. It is deliberately about behaviour rather than
package layout: moving code is cheap; preserving a quiet data-loss bug is not.

Active plan: [go-runtime-pipeline-correctness-plan.md](../../plans/go-runtime-pipeline-correctness-plan.md).

## Delivered baseline

The foundations below are present in the current implementation and its focused tests. They are
not evidence that every remediation phase in the active plan is complete.

| Delivered boundary               | Current behaviour                                                                                                                                                                                                                    | Evidence                                                                                                        |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------- |
| Named sensor capabilities        | The capabilities response distinguishes named radar and LiDAR sensor state, including the default radar and disabled LiDAR state. The runtime capability provider exposes explicit starting, ready, error, and disabled transitions. | `internal/api/server.go`, `internal/cmd/server/capabilities.go`, and `internal/cmd/server/capabilities_test.go` |
| Shared safe-path primitive       | A common path validator resolves filesystem paths before accepting them beneath an allowed directory, with coverage for symlink-sensitive cases. It is available for replay code to adopt rather than recreating path checks.        | `internal/security/pathvalidation.go` and `internal/security/pathvalidation_test.go`                            |
| Magnitude-only input recognition | The radar raw-data classifier recognises magnitude-only serial payloads, so this supported input shape reaches the persistence/transit boundary rather than being rejected at classification.                                        | `internal/serialmux/parse.go` and `internal/serialmux/handlers_test.go`                                         |
| Replay control surfaces          | PCAP and VRLOG replay handlers exist and expose their replay modes through the server. This establishes the surface that the plan is correcting; it does not prove default analysis or safe VRLOG validation.                        | `internal/lidar/server/playback_handlers.go`                                                                    |

The LiDAR lifecycle remains only partially wired: startup records a starting state when the LiDAR
server is created, but the runtime does not yet report the observed ready or error outcome. The
transition API is delivered; the lifecycle truthfulness work is still owned by the active plan.

## Invariants

- A PCAP analysis run must create and retain semantically processed output, not merely preserve
  frame counts while wall-clock throttling discards the useful work.
- Accepted radar raw rows must remain usable by transit derivation, including magnitude-only input
  where that shape is part of the ingest contract.
- LiDAR capabilities must move from startup into an observed ready or error state when the runtime
  knows the outcome.
- Replay file access must use the shared symlink-safe path validator rather than a second,
  less-careful interpretation of a safe directory.

## Remediation status

The following gaps remain open. They are stated here to prevent the delivered foundations above
from being mistaken for completed correctness work.

| Plan area                                  | Current boundary                                                                                                                                         | Graduation status          |
| ------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- |
| PCAP analysis defaults and semantic output | An omitted replay-mode value can still replace the intended analysis default, and wall-clock pacing can drop semantic processing during analysis runs.   | Phase 1 remains active.    |
| VRLOG file validation                      | VRLOG loading still has its own cleaned-path and string-prefix checks instead of the shared symlink-safe validator.                                      | Phase 2 remains active.    |
| Magnitude-only transit derivation          | Input recognition is delivered, but transit derivation does not yet carry the nullable magnitude-only value through its speed-oriented query and scan.   | Phase 3 remains scheduled. |
| LiDAR readiness and failure                | Capability transition methods are available, but the LiDAR runtime has not yet connected successful initialisation and failures to ready or error state. | Phase 4 remains active.    |

## Delivery boundary

The active plan owns sequencing, remediation checklists, phase status, and acceptance evidence.
This document records only verified delivery boundaries and the invariants they must satisfy; it
does not advance a plan phase or substitute implementation intent for evidence. Related clock,
performance, metrics, extraction, and UI plans retain their own scope and consume these invariants
rather than quietly redefining them.
