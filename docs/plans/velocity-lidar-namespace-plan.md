# velocity lidar namespace — fold pcap-analyse into the bundled binary

- **Status:** Implemented
- **Layers:** CLI Tools, L1–L6 (pcap-analyse pipeline)
- **Canonical:** [pcap-analysis-mode.md](../lidar/operations/pcap-analysis-mode.md)
- **Scope:** add a `lidar` namespace to the single `velocity` binary exposing `pcap-analyse`, `pcap-split`, and `settling-eval` as `velocity lidar <command>`, each sharing one implementation between the namespace applet and the standalone tool
- **Related:** [internal/cmd/lidar/](../../internal/cmd/lidar/), [internal/cmd/root/root.go](../../internal/cmd/root/root.go), [internal/cmd/tune/sweep.go](../../internal/cmd/tune/sweep.go)

> **Implemented.** All three LiDAR PCAP tools are folded into the `velocity lidar` namespace: `velocity lidar pcap-analyse`, `velocity lidar pcap-split`, and `velocity lidar settling-eval`. Each engine lives in an importable package (`internal/lidar/pcapanalyse`, `internal/lidar/pcapsplit`, `internal/lidar/settlingeval`); the `internal/cmd/lidar` applets parse flags and call the engine; the original `cmd/tools/*` binaries are now thin wrappers over the same applets, so existing Makefile targets and CI are unchanged. A `//go:build !pcap` stub keeps the default toolchain building. The sections below describe the original pcap-analyse design; pcap-split and settling-eval followed the same pattern.

## 1. Problem

`pcap-analyse` ships as a separate `internal/lidar/lidarbench` binary built with `-tags=pcap`. Operators and CI build and distribute it on its own, separate from the single multi-call `velocity` binary that already folds in the server, device, data, report, and tune surfaces. The tool is a first-class LiDAR diagnostic (it now also carries the `--motion` timeline), so it belongs inside `velocity` like the others — `velocity lidar pcap-analyse …` — giving one binary to ship and one help surface to discover.

## 2. What already exists

| Capability         | Location                                                                                                    | Notes                                                                                             |
| ------------------ | ----------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| Namespace router   | [internal/cmd/root/root.go](../../internal/cmd/root/root.go)                                                | `Dispatch(prog, args)` switches on `args[0]`; each namespace delegates to a `Main(args) int`      |
| Applet pattern     | [internal/cmd/tune/sweep.go](../../internal/cmd/tune/sweep.go)                                              | `tune.Main` uses a per-applet `flag.NewFlagSet`, then calls heavy logic in `internal/lidar/sweep` |
| Always-pcap build  | `Makefile` `VELOCITY_BUILD_TAGS := pcap`                                                                    | The `velocity` binary is always built with the `pcap` tag                                         |
| Tag stub pattern   | [internal/lidar/l1packets/network/pcap.go](../../internal/lidar/l1packets/network/pcap.go) + `pcap_stub.go` | `//go:build pcap` real impl + `//go:build !pcap` stub so dependents compile without libpcap       |
| pcap-analyse logic | [internal/lidar/lidarbench/lidarbench.go](../../internal/lidar/lidarbench/lidarbench.go)                    | ~1.6k lines, `package main`, `//go:build pcap`, parses the global `flag.CommandLine`              |

### Gap analysis

1. **Logic is trapped in `package main`.** The analysis pipeline, `Config`, `CaptureStats`, exporters, and benchmark mode live in `internal/lidar/lidarbench/lidarbench.go` and cannot be imported. They must move to an importable package.
2. **Global flag parsing.** pcap-analyse uses `flag.StringVar(&…)` on the default `flag.CommandLine`. An applet inside the multi-call binary must use its own `flag.NewFlagSet` (per the `tune` convention) so it composes with the other namespaces.
3. **No `lidar` namespace.** `root.go` has no `lidar` case and no stub for non-pcap builds.

## 3. Design

### 3.1 Target surface

```
velocity lidar pcap-analyse -pcap capture.pcapng -motion -stats
velocity lidar pcap-analyse -pcap capture.pcapng -benchmark -compare-baseline base.json
```

The `lidar` namespace is built to grow: `pcap-split` and `settling-eval` can move under it later (`velocity lidar pcap-split`, `velocity lidar settling-eval`) with no further router changes.

### 3.2 Package moves

1. **Extract the engine** → new `internal/lidar/pcapanalyse/` (importable, `//go:build pcap`). Move the pipeline driver, `Config`, `AnalysisResult`/`CaptureStats`, exporters, and benchmark code from `internal/lidar/lidarbench/lidarbench.go`. Expose a small entry such as `func Run(cfg Config) (*AnalysisResult, error)` (and the benchmark variant). Keep the existing `--motion` behaviour (which already calls `pcapsplit.BuildTimeline`).
2. **Applet** → new `internal/cmd/lidar/analyse.go` (`//go:build pcap`): an `analyseMain(args []string) int` that builds a `pcapanalyse.Config` from a `flag.NewFlagSet("velocity-lidar-pcap-analyse", …)` (mirroring [sweep.go](../../internal/cmd/tune/sweep.go)) and calls `pcapanalyse.Run`.
3. **Namespace router** → new `internal/cmd/lidar/lidar.go` (`//go:build pcap`): `func Main(args []string) int` switches on `args[0]` (`pcap-analyse`) and prints namespace usage otherwise.
4. **Non-pcap stub** → `internal/cmd/lidar/lidar_stub.go` (`//go:build !pcap`): `func Main(args []string) int` prints "lidar tools require a pcap-enabled build" and returns non-zero, so `go build ./...` / `go test ./...` / the non-pcap velocity build still compile (the `network` stub precedent).
5. **Standalone tool stays thin.** Rewrite [internal/lidar/lidarbench/lidarbench.go](../../internal/lidar/lidarbench/lidarbench.go) to `func main() { os.Exit(lidar.AnalyseMain(os.Args[1:])) }` (export `AnalyseMain`) so the existing binary, `run-pcap-stats` Makefile targets, and the benchmark CI keep working against the same code path. No behaviour change.

### 3.3 Router wiring

In [internal/cmd/root/root.go](../../internal/cmd/root/root.go):

```go
var lidarMain = lidar.Main   // resolves to the pcap impl or the stub by build tag

case "lidar":
    return lidarMain(args[1:])
```

Add a `lidar  LiDAR diagnostics: pcap-analyse` line to `topLevelUsage`.

### 3.4 Build & tags

No Makefile tag change: `VELOCITY_BUILD_TAGS := pcap` already compiles the real applet into `velocity`. The `//go:build !pcap` stub keeps the default toolchain (`go vet ./...`, `go test ./...`, IDE) green. `internal/cmd/root` imports `internal/cmd/lidar` unconditionally; the build tag selects real vs stub.

## 4. Affected files

| File                                                    | Change                                            |
| ------------------------------------------------------- | ------------------------------------------------- |
| `internal/lidar/pcapanalyse/*.go` (new)                 | Engine extracted from the current tool `main.go`  |
| `internal/cmd/lidar/lidar.go`, `analyse.go` (new, pcap) | Namespace router + applet `Main` with a `FlagSet` |
| `internal/cmd/lidar/lidar_stub.go` (new, !pcap)         | Stub `Main`                                       |
| `internal/cmd/root/root.go`                             | Register `case "lidar"`; usage text               |
| `internal/lidar/lidarbench/lidarbench.go`               | Reduce to a thin wrapper over the shared applet   |
| `internal/cmd/root/root_test.go`                        | Cover `lidar` routing (and unknown-subcommand)    |

## 5. Testing

- Unit: `root_test.go` asserts `velocity lidar pcap-analyse …` reaches the applet and that an unknown `lidar` subcommand returns exit 2 (mirror existing `tune`/`data` routing tests).
- Regression: `velocity lidar pcap-analyse -pcap internal/lidar/perf/pcap/kirk0.pcapng -motion -stats` produces the same summary as today's `pcap-analyse` (and the standalone wrapper stays byte-identical in behaviour).
- Build: `go build ./...` (no tags) compiles via the stub; `make build-velocity` compiles the real applet; `make test-go` stays green.

## 6. Open questions

1. **Naming:** `velocity lidar pcap-analyse` (keeps the familiar tool name) vs `velocity lidar analyse`. Recommend `pcap-analyse` for continuity.
2. **Fold the siblings now or later?** `pcap-split` and `settling-eval` are natural `velocity lidar` subcommands. Recommend landing `pcap-analyse` first, then moving the others in a follow-up once the namespace shape is proven.
3. **Retire the standalone binaries?** Keep `internal/lidar/lidarbench` as a thin wrapper for now (CI/baseline scripts depend on it); revisit removal once callers migrate to `velocity lidar …`.

## 7. Effort

`M` — most of the cost is the mechanical extraction of the ~1.6k-line tool `main.go` into an importable package and converting global-flag parsing to a `FlagSet`; the router and stub wiring are `S`.
