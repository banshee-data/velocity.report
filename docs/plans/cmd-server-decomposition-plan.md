# Serve namespace decomposition (v0.5.2)

- **Status:** Draft
- **Layers:** CLI composition (`internal/cmd/server`), busybox namespace surface
- **Target:** v0.5.2; `internal/cmd/server/radar.go` sits at 18.24% coverage and is the single largest untested surface in the binary
- **Companion plans:** [deploy-single-binary-image-consolidation-plan](deploy-single-binary-image-consolidation-plan.md), [binary-size-reduction-plan](binary-size-reduction-plan.md) <!-- link-ignore -->
- **Canonical:** [distribution-packaging.md](../platform/operations/distribution-packaging.md)

## Motivation

`internal/cmd/server/radar.go` is 18.24% covered across 592 statements — 472 of them
untested, which is more than every other gap in the LiDAR tree combined. It is not
low-coverage because it is hard to reach; it is low-coverage because 818 of its
~1100 lines are a single `Main()` function that opens the database, configures
logging, loads tuning, wires the serial mux, initialises LiDAR, starts the transit
worker, serves HTTP and gRPC, and handles signals — inline, in one scope, with no
seam a test can hold.

The repository already contains the fix. The `device` namespace is the same kind of
package and is decomposed the way this plan proposes, at a fraction of the size and
four times the coverage. Nothing new needs to be invented; `serve` simply never got
the same treatment.

The name is its own small problem. `radar.go` contains no radar-specific logic. It
is called that because the binary used to be the radar server, and it is now the
first file a newcomer opens when looking for radar handling.

## Current state

### The busybox layering, as shipped

Per [distribution-packaging.md](../platform/operations/distribution-packaging.md),
velocity.report ships one busybox-style binary whose canonical surface is
`velocity <namespace> ...`. The layering that implements it:

| Layer              | Package             | Responsibility                                      |
| ------------------ | ------------------- | --------------------------------------------------- |
| Multi-call entry   | `cmd/velocity`      | `argv[0]` compatibility, hands off to root dispatch |
| Namespace dispatch | `internal/cmd/root` | Selects the namespace (223 lines)                   |
| Namespace CLI      | `internal/cmd/<ns>` | Flag parsing, subcommand routing, composition       |
| Domain             | `internal/<domain>` | No CLI knowledge, no flags                          |

### The two namespaces compared

|                     |   `internal/cmd/device` | `internal/cmd/server` |
| ------------------- | ----------------------: | --------------------: |
| `Main()`            |            **58 lines** |         **818 lines** |
| Non-test source     |    10 files, 1256 lines |  10 files, 4057 lines |
| Shape               | one file per subcommand |          one function |
| Package coverage    |               **87.4%** |                     — |
| `radar.go` coverage |                       — |            **18.24%** |

`device` splits into `backup.go`, `check.go`, `install.go`, `rollback.go`,
`status.go`, `upgrade.go`, `tailscale.go`, each exposing `runX(args) error` with its
own test file — nine test files for ten source files.

### Domain placement is already correct

This was checked rather than assumed. Radar domain code is not trapped in the CLI
layer:

| Concern            | Lives in                                   |
| ------------------ | ------------------------------------------ |
| Serial transport   | `internal/serialmux/`                      |
| Event ingest       | `serialmux.HandleEvent(database, payload)` |
| Persistence        | `internal/db/db_radar.go`                  |
| HTTP surface       | `internal/api/`                            |
| OPS243 command set | `internal/radar/commands.go`               |

`Main()` only wires these together. No package boundary needs to move.

### One documentation defect found

CLAUDE.md states: "**Radar ingest** (`internal/radar/`): serial port reader for
OmniPreSense OPS243-A → inserts `radar_data` and `radar_objects`". `internal/radar/`
contains `commands.go` and its test, and nothing else — 261 lines of command table.
The reader is `internal/serialmux/`; the inserts are `internal/db/`.

## Findings

| Area                        | Current state                            | Severity | Release view                                        |
| --------------------------- | ---------------------------------------- | -------- | --------------------------------------------------- |
| `Main()` size               | 818 lines, ~10 inline phases             | High     | Blocks any coverage target on the `serve` namespace |
| File naming                 | `radar.go` holds no radar code           | Medium   | Cheap to fix alongside the split                    |
| Domain placement            | Already correct                          | None     | No package moves needed                             |
| Extraction progress         | ~20 helpers already extracted and tested | Low      | The work is further along than the number suggests  |
| CLAUDE.md radar description | Names the wrong package                  | Low      | One-line docs fix                                   |

### F1 — The extraction is already half-done

`radar.go` holds 20 functions. Most are small, already isolated, and already
covered: `parseMigrateCommandArgs`, `resolveDataCommandDBPathWith`,
`installedApplianceLayoutPresent`, `defaultRuntimeSerialOptions`,
`runtimeSerialSnapshot`, `runtimeSerialFactory`, `newRuntimeSerialManager`,
`tailscaleServeTarget`, `runTransitsCommand`, and the four adapter types. The file's
18.24% is one function's shadow, not a package-wide absence of tests.

### F2 — `Main()` already carries its own seam list

The function is commented into phases. Reading them off in order gives the split
directly:

1. Subcommand dispatch (before flag parsing, so subcommand flags are not consumed)
2. Logging configuration and verbosity
3. Version flags
4. Tuning config load, plus the hash used for VRLOG provenance
5. Database path resolution and open
6. Serial mux construction, or the no-op when `--disable-radar` is set
7. LiDAR component initialisation
8. Transit worker controller
9. HTTP, gRPC and visualiser start
10. Signal handling and the wait group

Phases 4, 6 and 7 already have their helpers extracted — into `lidar_helpers.go`,
which this branch took from 93.22% to 100%.

### F3 — 98% is the wrong target for a composition root

Even fully decomposed, a composition root keeps a residue that is not worth testing:
signal handling, the final `wg.Wait()`, the `os.Exit` paths. `device` lands at 87.4%
with exactly this shape. Set the bar for `internal/cmd/server` at ~90% and treat the
remainder as accepted residual rather than debt.

## Design / approach

Split `Main()` along the phases it already documents, into files named for what they
do, following `device`'s layout. No new packages, no domain moves.

| New file           | Holds                                                                  | Rough size |
| ------------------ | ---------------------------------------------------------------------- | ---------- |
| `main.go`          | `Main()`: subcommand dispatch, flag parsing, then a call to `runServe` | ~80 lines  |
| `serve.go`         | `runServe(cfg) error`: the composition, start, and wait                | ~200 lines |
| `logging.go`       | Logging stream and verbosity setup                                     | ~60 lines  |
| `serial.go`        | The serial-mux helpers already in `radar.go`                           | ~120 lines |
| `lidar_helpers.go` | Unchanged — already the LiDAR init seam                                | —          |
| `transits.go`      | `runTransitsCommand` and its helpers                                   | ~120 lines |

`radar.go` disappears. The invariant that makes the split safe is that each phase
takes its inputs as arguments and returns an error, so `runServe` becomes a sequence
of calls rather than a scope full of shared locals — that is what turns the 472
untested statements into reachable ones.

**Boundaries to hold:** domain packages continue to know nothing about flags;
`internal/cmd/server` continues to be the only place that knows both the CLI and the
domain; and phases return errors rather than calling `log.Fatal`, so a test can
observe the failure. The existing `logfFunc` indirection in `lidar_helpers.go` is the
pattern to follow — it is why `ensureValidLidarNetworkingFlags` is testable today.

**Binary size is unaffected.** The same code with the same imports compiles to the
same binary; this is a source-layout change, so there is no tension with the
[binary size plan](binary-size-reduction-plan.md).

## Scope

### Item 1: Rename and split the entry point

**Summary:** `radar.go` becomes `main.go` plus `serve.go`, matching `device`.

**Steps:**

1. Move `Main()` into `main.go`, keeping only dispatch and flag parsing.
2. Extract the composition and start sequence into `runServe` in `serve.go`.
3. Delete `radar.go`.
4. Update any references to the old filename in docs.

**Milestone:** v0.5.2

### Item 2: Extract the remaining phases

**Summary:** Lift logging, serial and transit-worker setup out of `runServe` into
their own files, each phase taking arguments and returning an error.

**Steps:**

1. `logging.go`: logging stream and verbosity setup.
2. `serial.go`: move the serial-mux helpers already present in `radar.go`.
3. `transits.go`: move `runTransitsCommand`.
4. Replace `log.Fatal` inside phases with returned errors, following the `logfFunc`
   pattern already used in `lidar_helpers.go`.

**Milestone:** v0.5.2

### Item 3: Cover the extracted phases

**Summary:** Bring `internal/cmd/server` to ~90%, the level `device` holds.

**Steps:**

1. A test per phase, asserting the error paths that currently exit the process.
2. Record the residual — signal handling, `wg.Wait()`, `os.Exit` — as accepted.

**Milestone:** v0.5.2

### Item 4: Fix the CLAUDE.md radar description

**Summary:** Point the radar-ingest line at the packages that actually do it.

**Steps:**

1. Correct the `internal/radar/` description to name the OPS243 command table.
2. Add `internal/serialmux/` for transport and ingest.

**Milestone:** v0.5.2. Independent of the rest.

## Dependencies

| Dependency                                                                      | Relationship                                                                                      |
| ------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| #565                                                                            | Touches `internal/cmd/server/lidar_helpers.go`; land first to avoid conflicts in the same package |
| [Single-binary consolidation](deploy-single-binary-image-consolidation-plan.md) | Defines the namespace surface this plan aligns `serve` with; already shipped in v0.5.1            |

## Risks

| Risk                                            | Likelihood | Impact | Mitigation                                                                                                                                                           |
| ----------------------------------------------- | ---------- | ------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Startup-order regression during the split       | Medium     | High   | Phases move whole; no reordering in the same commit as the extraction. The `serve` path has no integration test, so ordering is verified by review and a manual boot |
| Chasing 98% on a composition root               | Medium     | Medium | F3: target ~90%, matching `device`; record the residual explicitly                                                                                                   |
| Split stalls half-done, leaving two idioms      | Medium     | Medium | Items 1 and 2 are one release; `radar.go` is deleted in Item 1 so there is no partial state to live with                                                             |
| Reviewers read the rename as a behaviour change | Low        | Low    | Rename in its own commit, ahead of any extraction                                                                                                                    |

## Checklist

### Complete

- [x] Establish that domain code is correctly placed and no package moves are needed
- [x] Measure `Main()` at 818 lines against `device`'s 58, and the coverage gap that follows
- [x] Confirm ~20 helpers are already extracted and covered
- [x] Identify the CLAUDE.md radar-ingest description as inaccurate

### Outstanding

- [ ] Item 1: rename and split the entry point (`M`)
- [ ] Item 2: extract the remaining phases (`M`)
- [ ] Item 3: cover the extracted phases to ~90% (`M`)
- [ ] Item 4: fix the CLAUDE.md radar description (`S`)

### Deferred

- [ ] An integration test that boots `serve` end to end: worth having, but it is a
      different kind of work from this decomposition and should not gate it

### Accepted residuals (no action planned)

- [ ] Signal handling, `wg.Wait()` and `os.Exit` paths stay untested; `device` carries
      the same residual at 87.4%
- [ ] `internal/radar/` stays a 261-line command table. The radar domain genuinely is
      small — transport is `serialmux`, persistence is `db`, the HTTP surface is `api`.
      Balancing package sizes for their own sake would move code away from the layer
      that owns it
