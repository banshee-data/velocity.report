# VRLOG Check Subcommand Plan

## Goal

Add a VRLOG inspection command as a subcommand of the main `velocity-report`
binary, not as a separate tool binary. The command should validate VRLOG
structure and format version, analyse contents in real time, and present the
results in a tail-style terminal stream suitable for both existing `.vrlog`
recordings and live streams.

Proposed command family:

```bash
velocity-report vrlog check [flags] <path>
velocity-report vrlog check --live [stream flags]
```

This should be the reference pattern for future `/cmd` tools that need to move
under the main binary as first-class subcommands.

This command should be built on the generic TicTacTail platform described in
[tictactail-platform-plan.md](../../plans/tictactail-platform-plan.md). All
live/status rendering, pane layout, alignment, colour, spinner, and refresh
policy should live there. VRLOG should only provide emitted keys, projection,
and validation rules.

## Why Put It In The Main Binary

- Keeps operator entry points consistent: one binary, many subcommands.
- Reuses the existing manual subcommand dispatch pattern in
  `cmd/radar/radar.go`.
- Lets file analysis and live-pipeline analysis share runtime types already used
  by the server (`visualiser.FrameBundle`, `recorder.Replayer`, publisher stats).
- Avoids duplicating config, logging, and sensor/runtime wiring in separate
  binaries.

## Proposed CLI Shape

Top-level dispatch:

```text
velocity-report vrlog <command>
```

Initial subcommands:

- `velocity-report vrlog check <path>`
  - Validate `header.json`, `index.bin`, chunk files, frame decode, and
    supported version.
- `velocity-report vrlog check --live`
  - Analyse a live stream from the pipeline without requiring a recorded file.
- `velocity-report vrlog version <path>`
  - Print only format/version information for scripts.

Recommended flags for `vrlog check`:

- `--live`: consume live frames instead of a `.vrlog` directory.
- `--sensor-id <id>`: required in live mode where needed.
- `--refresh-hz <n>`: default `20`; common values `5`, `10`, and `20`.
- `--fast-window <seconds>`: fast aggregate pane window in integer seconds,
  default `3`.
- `--slow-window <seconds>`: slow aggregate pane window in integer seconds,
  default `30`.
- `--recent-lines <n>`: maximum visible rows in the fast aggregate pane,
  default `10`.
- `--no-ui`: disable the split-pane UI and print line-oriented logs only.
- `--json`: machine-readable final summary for CI or scripting.
- `--strict`: treat warnings such as unknown fields or version mismatches as
  failures.

## Dispatch / Package Layout

Keep command dispatch in `cmd/radar/radar.go`, but split generic aggregation
from VRLOG-specific reporting.

Proposed layout:

```text
cmd/radar/radar.go
pkg/tictactail/
  engine.go         # generic windowed aggregation
  row.go            # flat row contract
  render.go         # generic tail/live rendering
internal/lidar/vrreport/
  command.go        # flag parsing and command dispatch
  checker.go        # vrlog validation and orchestration
  source_file.go    # .vrlog reader source
  source_live.go    # live pipeline source adapter
  schema.go         # vrlog emitted keys and row conventions
  projector.go      # FrameBundle/live frame -> flat sample rows
  report.go         # final summaries / exit policy
```

Pattern in `main()`:

```go
if subcommand == "vrlog" {
    runVRLogCommand(flag.Args()[1:])
    return
}
```

This matches the current `migrate` and `transits` handling style.

## Modes

### 1. File Mode

Input is a `.vrlog` directory.

Checks:

- `header.json` exists and parses.
- `header.version` is supported.
- `index.bin` exists and size is divisible by 24 bytes.
- `index.bin` frame count matches `header.total_frames`.
- referenced chunk files exist.
- each frame length prefix is valid.
- each frame deserialises to `FrameBundle`.
- timestamps are monotonic.
- frame IDs are monotonic.
- coordinate frame is stable unless explicitly allowed to vary.

Output:

- immediate validation errors
- rolling content stats while scanning
- final summary with pass/warn/fail counts

### 2. Live Mode

Input is a live frame stream from the running pipeline.

Checks:

- stream connectivity
- frame cadence
- schema/version marker for emitted VRLOG-compatible frames
- per-frame field completeness
- rolling operational stats

Live mode should not pretend to validate on-disk artefacts such as
`header.json/index.bin`; it validates the stream against the same logical
frame contract and reports that it is checking a live source rather than a
recorded archive.

## Emitted Keys

Preferred emitted keys for VRLOG:

- reserved keys from TicTacTail: `ts_nanos`, `kind`, `sev`, `src`, `win_s`

- `fr_inc`
- `frame_cur`
- `frame_tot`
- `fps`
- `ch_cur`
- `ch_tot`
- `tr`
- `cl`
- `fg`
- `bg`
- `st`
- `dr_inc`
- `er_inc`
- `ver`
- `hd`
- `ix`
- `tf`
- `ev`

Rendered forms may collapse generic suffix pairs:

- `fr_inc` -> `fr`
- `ch_cur` + `ch_tot` -> `ch`
- `dr_inc` -> `dr`
- `er_inc` -> `er`

VRLOG should emit those exact flat keys into `tictactail`. No key mapping should
exist in the platform.

## Metrics To Show

The VRLOG layer should define the domain metrics and emit them directly into
TicTacTail. Keep the row contract flat, one-layer, and key/value only.

The command should explicitly separate three classes of metrics:

### A. File-backed metrics

Safe to show as facts from the recording or stream payload:

- VRLOG version
- total frames
- current frame index
- timestamp range
- tracks per frame/window
- clusters per frame/window
- foreground point count per frame/window
- background point count per frame/window
- background snapshot presence
- coordinate frame metadata

### B. Runtime transport metrics

Only available when instrumented at runtime:

- frame ingest rate
- decode rate
- render refresh rate
- queue depth
- dropped frame count in the checker itself
- source lag

### C. Downstream-derived metrics

Not represented directly in VRLOG and must be labelled as derived:

- settled / not settled
- downstream classifier outputs
- post-replay drop estimates
- algorithmic anomaly counts
- quality scores added after ingest

## Aggregation Window

VRLOG should keep both a fast and a slow aggregate window hot at all times.

Defaults:

- fast window = `3`
- slow window = `30`

The stream should emit both rolling windows, not a single selected window.

TTY layout:

- top pane: `30` second aggregates, using the remaining height
- middle pane: recent `3` second aggregates, capped at `10` visible rows
- lower pane: one-line live snapshot
- bottom bar: one-line status/input bar

The requested `3 / 30 seconds` behaviour should still use rolling windows, not
lifetime averages.

Implementation:

- keep accumulators for both `3` and `30` second windows
- emit `win_s=3` rows into the recent pane
- emit `win_s=30` rows into the long-history pane
- keep bounded caches for both buckets so resizes can repaint without replay
- keep the live pane unwindowed and driven by the latest sample state

Do not collapse `3` second and `30` second buckets into one synthetic history
line. The primary read should remain temporal: one row per completed window.

TicTacTail should keep both windows hot internally so VRLOG can show fast and
slow history at the same time with no cold start.

Recommended live-pane fields:

- current frame progress
- instantaneous fps
- current track count
- current cluster count
- current fg/bg point counts
- latest known settled/drops/errors state

## Handling Metrics Not Represented In The File

This is the main correctness constraint: the command must not imply that a
downstream-computed signal was stored in the VRLOG if it was not.

Plan:

1. Add provenance to every metric definition.
   - `SourceFile`
   - `SourceRuntime`
   - `SourceDerived`

2. Render derived-only metrics in a separate `PROVENANCE` or `DERIVED` block.

3. For file mode, compute derived metrics by rerunning the relevant downstream
   algorithm over the decoded frames when available.

4. For live mode, compute the same derived metrics from the live frame stream,
   but tag them as `live-derived`.

5. If a metric cannot be reproduced from the available input, show:
   - `n/a`
   - a short reason such as `not present in vrlog`

Examples:

- `settled`
  - Not a direct VRLOG header field today.
  - Can be inferred from `Background.GridMetadata.SettlingComplete` when a
    background snapshot is present.
  - If background snapshots are absent, show `n/a`, not `false`.

- `drops`
  - File mode: only checker-local decode drops/corrupt frames are factual.
  - Live mode: may also include runtime queue drops from publisher/checker.
  - Do not report historic publisher drops from a VRLOG file unless they were
    recorded as metadata.

- `errors`
  - split into `format errors`, `decode errors`, and `derived algo errors`
  - do not collapse them into one unlabeled number

## Validation Policy

Severity levels:

- `FAIL`: structure corrupt, unsupported version, unreadable chunk, bad length,
  impossible index offset
- `WARN`: unusual but readable data, unknown optional field, missing optional
  section, timestamp jump
- `INFO`: operational progress and throughput

Exit behaviour:

- exit `0` for clean pass
- exit `1` for validation fail
- exit `2` for command/runtime error

## Logging Alongside The Live Surface

Support both the tail stream and periodic machine-readable or plain-text logs.

Recommended behaviour:

- live pane and status bar redraw at `15-20 Hz`
- emit one compact `3` second aggregate row whenever the fast window closes
- emit one compact `30` second aggregate row whenever the slow window closes
- emit immediate event rows for structural failures, decode failures, and mode
  transitions
- when `--no-ui` is set, keep only the aggregate and event rows

This makes the command act like a structured terminal surface rather than a
full-screen dashboard while still preserving line-oriented output when UI is
disabled.

## Phased Delivery

### Phase 1: File Validator

- Add `velocity-report vrlog check <path>`
- Validate structure and version
- Decode all frames
- Emit tail-style `3` second and `30` second aggregate rows
- Add split-pane TTY output with slow history, recent history, live, and status
  bars

### Phase 2: Live Stream Checker

- Add `--live`
- Plug into publisher or pipeline stream
- Reuse the same stats model and tail renderer
- Add runtime-only counters
- Reuse the same split-pane layout in live mode
- Decide whether the status/input bar is status-only or accepts commands

### Phase 3: Derived Metrics

- Add provenance-tagged downstream metrics
- Run optional post-frame algorithms for signals not persisted in VRLOG
- Mark all derived numbers clearly in UI and final report

### Phase 4: Scriptable Output

- Add `--json`
- Add `vrlog version <path>`
- Make exit codes stable for CI

## Key Implementation Risks

- `20 Hz` redraw can waste CPU if more than the live pane and status bar are
  rewritten on every tick. Keep history pane redraw scoped to row append and
  resize.
- live mode and file mode do not expose identical truth; provenance must remain
  explicit.
- terminals vary in emoji width handling. Keep an ASCII fallback.
- pane layout and resize reflow add complexity; bounded caches need explicit
  budgets and tests.
- bottom-line control can get messy when other goroutines print concurrently.
  Funnel all output through one renderer.
- long scans need deterministic summaries even when live redraw is disabled or
  output is redirected.

## Recommendation

Start with `velocity-report vrlog check <path>` inside the main binary and make
the checker architecture source-agnostic from day one. That gives a clean
template for other former `cmd/tools/*` features while avoiding a one-off file
validator that later has to be rewritten for live streams.
