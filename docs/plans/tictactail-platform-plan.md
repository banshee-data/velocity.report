# TicTacTail Platform Plan

## Working Name

Selected working name: `TicTacTail`

Why:

- `tic tac` captures cadence and regular refresh
- `tail` captures persistent history output
- distinctive enough for a standalone package/repo name

Use `TicTacTail` in code/docs unless we later have a strong reason to rename it.

## Goal

Build a generic platform that takes flat key/value samples, refreshes a live
 footer quickly, emits aligned history rows, and keeps the heavy logic outside
 app-specific code.

The split should be:

1. `tictactail`: all aggregation, live refresh, history rendering, alignment,
   colors, spinner, and output mechanics
2. application emitter/projector: choose keys and feed flat samples
3. thin adapter: CLI command or route glue

The VRLOG checker should be a small import/config layer on top of this.

## What TicTacTail Owns

`TicTacTail` should own all generic behaviour:

- ingesting flat timestamped samples
- maintaining one or more hot aggregation windows
- producing aggregate rows and live snapshots
- tail-style history output
- live footer refresh loop
- aligned columns between history rows and live footer
- ANSI coloring for persisted rows
- moon-phase live spinner
- TTY vs non-TTY behaviour
- performance-sensitive local aggregation

If the UI contract is generic, it belongs here, not in the VRLOG-specific doc.

## Repo / Module Strategy

### Phase 1: Incubate Here

Use a public package path:

```text
pkg/tictactail/
```

Rules:

- no imports from `internal/*`
- no LiDAR- or VRLOG-specific types
- no key mapping or alias tables
- no app-specific metric names inside the engine

### Phase 2: Split Out

Once there is a second real consumer, split to:

```text
github.com/banshee-data/tictactail
```

That gives a publishable repo name that matches the product.

## Core Contract

TicTacTail accepts only flat rows:

- one layer JSON
- key/value only
- JSON scalar values only

Allowed values:

- `string`
- `int64`
- `float64`
- `bool`
- `null`

Disallowed:

- nested objects
- arrays
- app structs

Reserved keys:

- `kind`
- `ts_ns`
- `sev`
- `src`
- `win_s`

Everything else is application payload and must remain unchanged.

## No Mapping

TicTacTail should never rename keys.

If an app wants short keys, it emits short keys.

If an app wants longer keys, it emits longer keys.

Examples:

- short: `fr`, `fps`, `ch_cur`, `ch_tot`, `tr`, `cl`, `fg`, `bg`
- long: `frames`, `fps`, `chunk_cur`, `chunk_tot`, `tracks`

TicTacTail stores and renders whatever keys it is given.

## Value Semantics

TicTacTail only understands two application field kinds:

1. measure
2. increment counter

Rule:

- keys ending in `_inc` are summed within the active window
- all other non-reserved keys are latest measures

Examples:

- `fps=201.2` -> latest measure
- `tr=15` -> latest measure
- `fr_inc=1` -> add one frame to the current window sum
- `er_inc=1` -> add one error to the current window sum

This keeps the engine simple and removes per-key strategy tables.

## Simple API

Keep the API tight.

```go
package tictactail

import "time"

type Value any

type Row map[string]Value

type Sample struct {
	Ts   time.Time
	Vals Row
}

type Config struct {
	Source        string
	Windows       []int
	DefaultWindow int
	RefreshHz     int
	Columns       []string
}

type Engine interface {
	Add(sample Sample) ([]Row, error)
	Live(now time.Time) Row
	SetWindow(seconds int) error
	Window() int
}
```

`Columns` is ordering only. It is not key mapping.

## Input Structures

Applications feed simple flat rows.

### Sample Input

```json
{
  "ts_ns": 1741435234123000000,
  "fps": 201.0,
  "tr": 15,
  "cl": 24,
  "fg": 6620,
  "bg": 41201,
  "fr_inc": 1
}
```

Meaning:

- `fps`, `tr`, `cl`, `fg`, `bg` are latest measures
- `fr_inc` is a window counter

### Event Input

```json
{
  "ts_ns": 1741435235123000000,
  "kind": "event",
  "sev": "warn",
  "msg": "missing_bg_snapshot",
  "er_inc": 1
}
```

### Init Input

```json
{
  "ts_ns": 1741435200000000000,
  "kind": "init",
  "src": "vrlog",
  "ver": "1.0",
  "hd": true,
  "ix": true,
  "frame_tot": 18210
}
```

## Aggregation Model

TicTacTail should keep multiple windows hot, but only one active for emission.

Default:

- active window `30`

Fast toggle:

- active window `3`

Behaviour:

- maintain state for configured windows such as `3` and `30`
- update all hot windows on every sample
- emit rows only for the active window
- allow instant `30 <-> 3` switching with no cold start

### Window Rules

For each window:

- measure fields keep latest value seen in that window
- `_inc` fields keep running sums in that window

At cutover:

- emit one aggregate row for the closing window
- reset `_inc` sums for the new window
- start a fresh measure state for the new window

At live refresh:

- measures show latest value seen so far in the active window
- `_inc` fields show current running sums in the active window

## Local Aggregation Implementation

Use a single-owner aggregation loop.

Suggested model:

- one ingest goroutine owns all window state
- `Add()` forwards samples into that owner
- the owner updates all hot windows
- the owner publishes immutable live snapshots
- the renderer reads snapshots at configured refresh cadence

Per-window state:

```go
type windowState struct {
	WindowSeconds int
	WindowID      int64
	Latest        map[string]Value
	Sums          map[string]float64
}
```

Update rule:

- reserved key -> route specially
- key ends in `_inc` -> numeric add into `Sums`
- otherwise -> overwrite `Latest`

Cutover rule:

- `windowID := ts.Unix() / int64(windowSeconds)`
- if the id changed, flush previous row and rotate state

Why this is efficient:

- O(keys) per sample
- suffix check only
- no nested traversal
- no key mapping
- renderer can use atomic snapshot copies instead of sharing mutable maps

## Rendering Contract

TicTacTail owns the visual contract too.

### Output Shape

Two layers:

- immutable history rows
- one ephemeral live footer

History rows stay in scrollback.
The live footer redraws in place.

### Footer Refresh

Supported refresh targets:

- `5 Hz`
- `10 Hz`
- `20 Hz`

Default:

- `20 Hz` for interactive live use

The refresh rate affects only footer redraw cadence, not aggregation math.

### Spinner

Use moon phases on the live footer only:

```text
🌑 🌒 🌓 🌔 🌕 🌖 🌗 🌘
```

Persisted history rows do not use spinner frames.

### Color

Persisted history rows keep ANSI colors when stdout is a TTY:

- green for ok
- yellow for warn
- red for fail

The live footer uses the spinner prefix instead of a colored status dot.

### Alignment

TicTacTail should align live/footer columns to the history row shape.

If history rows render:

```text
🟢 12:00:30 ag=30 fr=6014 fps=200 ch=7/19 tr=13 cl=21 fg=6312 bg=41077 st=0.87 dr=0 er=1
```

then the live footer should render with the same field order:

```text
🌔 live    ag=30 fr=6341/18210 fps=201 ch=7/19 tr=15 cl=24 fg=6620 bg=41201 st=n/a  dr=0 er=1 run
```

This is why alignment logic belongs in TicTacTail rather than the application.

### Pair Formatting

TicTacTail may apply generic suffix conventions in the renderer without renaming
 keys.

Allowed generic formatting rules:

- `*_cur` + `*_tot` -> render as `<base>=cur/tot`
- `*_inc` -> render as `<base>=sum`

Examples:

- `ch_cur` + `ch_tot` -> `ch=7/19`
- `frame_cur` + `frame_tot` -> `frame=6341/18210`
- `fr_inc` -> `fr=6014`

This is formatting, not key remapping.

### Non-TTY Mode

If stdout is not a TTY:

- disable footer redraw
- emit history rows and event rows only
- optionally sample live rows at a lower cadence for logs

## Output Examples

Aggregate row:

```json
{
  "kind": "agg",
  "ts_ns": 1741435230000000000,
  "win_s": 30,
  "sev": "ok",
  "src": "vrlog",
  "fr_inc": 6014,
  "fps": 200.4,
  "ch_cur": 7,
  "ch_tot": 19,
  "tr": 13,
  "cl": 21,
  "fg": 6312,
  "bg": 41077,
  "st": 0.87,
  "dr_inc": 0,
  "er_inc": 1
}
```

Live row:

```json
{
  "kind": "live",
  "ts_ns": 1741435234123000000,
  "win_s": 30,
  "sev": "ok",
  "src": "vrlog",
  "frame_cur": 6341,
  "frame_tot": 18210,
  "fps": 201.0,
  "ch_cur": 7,
  "ch_tot": 19,
  "tr": 15,
  "cl": 24,
  "fg": 6620,
  "bg": 41201,
  "st": null,
  "dr_inc": 0,
  "er_inc": 1
}
```

## Performance Validation

TicTacTail needs explicit coverage for both responsiveness and overhead.

### Test Matrix

Run the same file-processing workload with footer refresh at:

- `5 Hz`
- `10 Hz`
- `20 Hz`

Validate:

- identical aggregate output across all three rates
- acceptable footer responsiveness
- minimal file-processing overhead as refresh increases
- stable allocation behaviour

### Required Benchmarks

- `BenchmarkEngineAdd_FileReplay`
- `BenchmarkFooterRender_5Hz`
- `BenchmarkFooterRender_10Hz`
- `BenchmarkFooterRender_20Hz`
- `BenchmarkFileReplayWithFooter_5Hz`
- `BenchmarkFileReplayWithFooter_10Hz`
- `BenchmarkFileReplayWithFooter_20Hz`

### Required Tests

- live footer updates at configured cadence
- aggregate rows still cut over correctly on `3` and `30`
- `_inc` values reset on cutover
- measures always reflect latest value in active window
- aggregate rows are identical regardless of `5/10/20 Hz`

### Acceptance Criteria

- renderer work stays decoupled from ingest
- `20 Hz` is responsive
- `20 Hz` does not materially distort file replay throughput
- if it does, file mode gets a lower default refresh than live mode

## Thin App Integration

Applications should have very little code around TicTacTail.

The app side should only provide:

- emitted key set
- sample projector
- window config
- source wiring

Example shape:

```text
pkg/tictactail/             # all heavy logic
internal/lidar/vrreport/    # projector + config only
cmd/radar/radar.go          # dispatch only
```

Minimal integration should look like:

```go
tail := tictactail.New(tictactail.Config{
	Source:        "vrlog",
	Windows:       []int{3, 30},
	DefaultWindow: 30,
	RefreshHz:     20,
	Columns:       []string{"ag", "fr", "fps", "ch", "tr", "cl", "fg", "bg", "st", "dr", "er"},
})
```

Then the adapter just feeds projected samples and starts the renderer.

## Likely Consumers

Beyond VRLOG:

- LiDAR live status tails
- sweep progress tails
- deploy progress / health tails
- transit rebuild tails
- generic operator CLIs with history + fast footer

## Recommendation

Adopt `TicTacTail` as the working package/repo name, move all visual/tail/live
 contract details here, and keep VRLOG as a thin emitter/projector on top.
