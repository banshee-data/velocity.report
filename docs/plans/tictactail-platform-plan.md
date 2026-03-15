# TicTacTail Platform Plan

- **Layers:** Cross-cutting (platform library)
- **Decision:** D-23 — [DECISIONS.md](../DECISIONS.md)
- **Backlog:** v0.8 — [BACKLOG.md](../BACKLOG.md)

## Working Name

Selected working name: `TicTacTail`

Why:

- `tic tac` captures cadence and regular refresh
- `tail` captures persistent history output
- distinctive enough for a standalone package/repo name

Use `TicTacTail` in code/docs unless we later have a strong reason to rename it.

## Goal

Build a generic platform that takes flat key/value samples, refreshes a live
surface quickly, emits aligned history rows for one active aggregate window,
and keeps the heavy logic outside app-specific code.

The split should be:

1. `tictactail`: all aggregation, live refresh, history rendering, alignment,
   colours, spinner, and output mechanics
2. application emitter/projector: choose keys and feed flat samples
3. thin adapter: CLI command or route glue

The VRLOG checker should be a small import/config layer on top of this.

## What TicTacTail Owns

`TicTacTail` should own all generic behaviour:

- ingesting flat timestamped samples
- maintaining one active aggregation window
- producing aggregate rows and live snapshots
- tail-style history output
- live/status refresh loop
- one main TTY history pane plus a one-line live row and one-line status bar
- aligned columns between aggregate history and the live row
- ANSI colouring for persisted rows
- configurable live spinner styles
- one bounded history cache for resize handling
- TTY vs non-TTY behaviour
- startup schema scan, fixed-size allocation, and hard byte limits
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
- `ts_nanos`
- `sev`
- `src`
- `win_s`

Everything else is application payload and must remain unchanged.

`ts_nanos` is the only public timestamp field in the row contract. Adapters may
convert to or from `time.Time` internally, but the wire and engine-facing
format stays integer nanoseconds.

`ts_nanos` is mandatory on every input row. Rows without it are rejected.

## No Mapping

TicTacTail should never rename keys.

If an app wants short keys, it emits short keys.

If an app wants longer keys, it emits longer keys.

Examples:

- short: `fr`, `fps`, `ch_cur`, `ch_tot`, `tr`, `cl`, `fg`, `bg`
- long: `frames`, `fps`, `chunk_cur`, `chunk_tot`, `tracks`

TicTacTail stores and renders whatever keys it is given.
There is no special `ag` alias; aggregate rows carry raw `win_s=<seconds>`,
though a single-pane renderer may hide it by default.

## Value Semantics

TicTacTail only understands two application field kinds:

1. measure
2. increment counter

Rule:

- keys ending in `_inc` are summed within the active window as integer
  counters
- all other non-reserved keys are latest measures

Examples:

- `fps=201.2` -> latest measure
- `tr=15` -> latest measure
- `fr_inc=1` -> add one frame to the current window sum
- `er_inc=1` -> add one error to the current window sum

Additional rules:

- `_inc` fields must be emitted as `int64`
- count-like measures such as `frame_cur`, `frame_tot`, `tr`, `cl`, `fg`, and
  `bg` should prefer `int64`
- `float64` should be reserved for genuinely continuous measures such as `fps`
  or `st`

This keeps the engine simple and removes per-key strategy tables.

## Simple API

Keep the API tight.

```go
package tictactail

type ScalarValue any // nil, string, int64, float64, or bool; validated at ingest

type Row map[string]ScalarValue

type Config struct {
  Source        string
  WindowSeconds int
  RefreshHz     int
  SpinnerStyle  string
  Columns       []string
  HistoryRows   int
  MaxBytes      int
}

type Engine interface {
  Add(row Row) (*Row, error)
  Live(nowNanos int64) Row
}
```

`ScalarValue` is validated at ingest. Nested objects, arrays, structs, and
non-integer `_inc` values are rejected.

`WindowSeconds` selects the single active aggregate bucket. Changing it resets
the current bucket state and starts a fresh window.

`Columns` is ordering only. It is not key mapping.

`HistoryRows` bounds the aggregate history cache used for resize reflow.

`MaxBytes` is a hard cap on internal allocation after schema freeze. Rows that
would exceed the cap, introduce new keys after freeze, or change a key's value
type are rejected.

`SpinnerStyle` selects `moon`, `braille8`, or `ascii`. The zero value
defaults to `moon`, but the renderer should fall back to `ascii` when Unicode
output is disabled or unsafe.

## Input Structures

Applications feed simple flat rows.

### Sample Input

```json
{
  "ts_nanos": 1741435234123000000,
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
  "ts_nanos": 1741435235123000000,
  "kind": "event",
  "sev": "warn",
  "msg": "missing_bg_snapshot",
  "er_inc": 1
}
```

### Init Input

```json
{
  "ts_nanos": 1741435200000000000,
  "kind": "init",
  "src": "vrlog",
  "ver": "1.0",
  "hd": true,
  "ix": true,
  "frame_tot": 18210
}
```

## Aggregation Model

TicTacTail should keep one active aggregate window and emit one row when that
window closes.

Initial default:

- aggregate window `30`

Behaviour:

- maintain state for one configured window such as `30`
- update the active window on every sample
- emit one aggregate row whenever the active window closes
- stamp emitted rows with raw `win_s`
- if `WindowSeconds` changes, drop the partial bucket, reset measures and
  counters, and start a fresh window immediately

### Window Rules

For the active window:

- measure fields keep latest value seen in that window
- `_inc` fields keep running integer sums in that window

At cutover:

- emit one aggregate row for the closing window
- reset `_inc` sums for that window
- start a fresh measure state for that window

At live refresh:

- the live row shows the latest measures plus the current in-window `_inc`
  counts before flush
- the live row is not required to carry `win_s`

## Local Aggregation Implementation

Use a single-owner aggregation loop.

Suggested model:

- one ingest goroutine owns schema state and the active window
- `Add()` forwards rows into that owner
- the owner updates the active window
- the owner publishes immutable live snapshots
- the renderer reads snapshots at configured refresh cadence
- the renderer keeps one bounded history cache for resize handling
- startup rows are scanned once to freeze key and type layout before steady
  state ingest
- after schema freeze, allocations are fixed and bounded by `MaxBytes`

Internal state:

```go
type fieldKind uint8

type fieldSpec struct {
  Key  string
  Kind fieldKind
}

type windowState struct {
  WindowSeconds int
  WindowID      int64
  Latest        []ScalarValue
  Sums          []int64
}
```

Renderer-side cache:

```go
type rowCache struct {
  Rows     []Row // bounded ring buffer, newest last
  Head     int
  Count    int
  MaxBytes int
}
```

The engine freezes a key/type schema during startup, allocates fixed slots for
measures and counters, and rejects later rows that introduce unknown keys or
type drift. The history cache is a single bounded ring used only to repaint the
main pane after resize. Older history remains available through terminal
scrollback or optional log sinks, not by keeping an unbounded in-memory copy.

Update rule:

- reserved key -> route specially
- `ts_nanos` -> must parse as `int64` and is the source of truth
- key ends in `_inc` -> numeric add into indexed `Sums`
- otherwise -> overwrite indexed `Latest`
- unknown keys or type drift after schema freeze -> reject row

Cutover rule:

- `windowID := tsNanos / (int64(windowSeconds) * 1_000_000_000)`
- if the id changed, flush previous row and rotate state

Why this is efficient:

- O(keys) per sample
- suffix check only
- no nested traversal
- no key mapping
- fixed slices after schema freeze instead of unbounded mutable maps
- resize cost is bounded by cache size rather than replay length
- no unbounded buffer growth in steady state

## Rendering Contract

TicTacTail owns the visual contract too.

### Output Shape

TicTacTail should support both a simple line-oriented stream and a single-pane
TTY surface.

Default interactive TTY layout:

1. main pane: aggregate history, using the remaining terminal height
2. lower row: one-line live snapshot
3. bottom bar: one-line status bar, status-only in v1

Event rows render inline in the main history pane. The logical history is
unbounded, but only the rows that fit in the available height are visible at
once. A single bounded cache is used only to repaint recent rows after a
resize; the renderer uses the normal screen with scrollback rather than an
alternate screen.

### Refresh Cadence

Supported refresh targets:

- `5 Hz`
- `10 Hz`
- `20 Hz`

Default:

- `20 Hz` for interactive live use

The refresh rate affects only live-row and status-bar redraw cadence, not
aggregation math. The main pane repaints only when a new aggregate or event row
arrives or the terminal size changes.

### Spinner

Spinner frames are configurable for the live row only.

Default `moon` style:

```text
🌑 🌒 🌓 🌔 🌕 🌖 🌗 🌘
```

Optional `braille8` snake-chase style:

```text
⣾ ⣽ ⣻ ⢿ ⡿ ⣟ ⣯ ⣷
```

Optional `ascii` style for non-Unicode output:

```text
| / - \
```

Use `ascii` when Unicode output is disabled, the terminal is not UTF-8 clean,
or plain-text logs matter more than visual polish.

Persisted history rows do not use spinner frames.

### Colour

Persisted history rows keep ANSI colours when stdout is a TTY:

- green for ok
- yellow for warn
- red for fail

The live row uses the selected spinner prefix instead of a coloured status
dot.

### Alignment

TicTacTail should align main history and live-row columns to the same row shape
where possible.

If the TTY surface renders:

```text
🟢 12:00:30 fr=6014 fps=200 ch=7/19 tr=13 cl=21 fg=6312 bg=41077 st=0.87 dr=0 er=1
🌔 live      frame=6341/18210 fr=311 fps=201 ch=7/19 tr=15 cl=24 fg=6620 bg=41201 run
status src=vrlog win=30 rows=200 tty=on
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
- `win_s` -> `win_s=30` when explicitly enabled in the renderer

This is formatting, not key remapping.

### Resize And Cache Behaviour

The renderer should keep one bounded cache for aggregate history.

Rules:

- cache size is fixed at startup from `HistoryRows` and discovered row shape
- total allocated bytes must stay within `MaxBytes`
- cache capacity never grows after startup
- a resize should re-slice cached rows into the main pane without replaying
  input
- if the new terminal size exceeds cached history, show the newest rows and
  leave older rows in scrollback

### Non-TTY Mode

If stdout is not a TTY:

- disable the TTY compositor
- emit aggregate rows and event rows as line-oriented output
- optionally sample live rows at a lower cadence for logs

## Output Examples

Aggregate row (configured bucket, here `30` seconds):

```json
{
  "kind": "agg",
  "ts_nanos": 1741435230000000000,
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
  "ts_nanos": 1741435234123000000,
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
  "fr_inc": 311,
  "dr_inc": 0,
  "er_inc": 1
}
```

## Performance Validation

TicTacTail needs explicit coverage for both responsiveness and overhead.

### Test Matrix

Run the same file-processing workload with live/status refresh at:

- `5 Hz`
- `10 Hz`
- `20 Hz`

Validate:

- identical aggregate output across all three rates
- acceptable live/status responsiveness
- minimal file-processing overhead as refresh increases
- stable allocation behaviour
- predictable resize reflow cost from one bounded cache
- no allocation growth after schema freeze

### Required Benchmarks

- `BenchmarkEngineAdd_FileReplay`
- `BenchmarkLiveRowRender_5Hz`
- `BenchmarkLiveRowRender_10Hz`
- `BenchmarkLiveRowRender_20Hz`
- `BenchmarkMainPaneResizeReflow`
- `BenchmarkFileReplayWithUI_5Hz`
- `BenchmarkFileReplayWithUI_10Hz`
- `BenchmarkFileReplayWithUI_20Hz`

### Required Tests

- live row updates at configured cadence
- aggregate rows cut over correctly for the configured window
- changing `WindowSeconds` resets current measures and sums
- `_inc` values reset on cutover
- measures always reflect the latest value in the active window
- schema freezes on startup and rejects new keys or type drift
- resize reflows from cache without replaying the source
- aggregate rows are identical regardless of `5/10/20 Hz`
- internal buffers stay within `MaxBytes`

### Acceptance Criteria

- renderer work stays decoupled from ingest
- `20 Hz` live/status redraw is responsive
- the main pane redraws only on append or resize
- bounded caches stay within configured limits
- no unbounded allocation growth after schema freeze
- `20 Hz` does not materially distort file replay throughput
- if it does, file mode gets a lower default refresh than interactive TTY mode

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
  WindowSeconds: 30,
  RefreshHz:     20,
  SpinnerStyle:  "moon",
  Columns:       []string{"fr", "fps", "ch", "tr", "cl", "fg", "bg", "st", "dr", "er"},
  HistoryRows:   200,
  MaxBytes:      1 << 20,
})
```

The renderer then shows aggregate history in the main pane, one live row, and
one status bar.

Then the adapter just feeds projected samples and starts the renderer. If the
adapter changes `WindowSeconds`, the engine resets the current bucket and
starts a fresh one.

## Complexity Review

### Benefits

- one flat row contract lets multiple CLIs share aggregation and rendering
- raw `ts_nanos` plus scalar values keep the wire shape simple and language
  neutral
- integer counters avoid accidental floating-point drift in aggregate totals
- a single active window keeps the public API and renderer simpler
- bounded caches and fixed allocations reduce leak risk and make memory use
  predictable
- normal-screen rendering keeps scrollback available without extra TTY policy

### Costs And Shortcomings

- the public row contract is still runtime-validated rather than compile-time
  enforced
- startup schema freeze means unexpected new keys or type changes require a
  restart or explicit reinitialisation
- bounded caches improve resize behaviour but cannot recreate arbitrarily old
  history after a large resize
- a single-window design trades some flexibility for a smaller, safer surface
- a generic library can still grow too much UI policy if renderer options are
  not kept disciplined

### Resolved V1 Decisions

- `ts_nanos` is mandatory on every input row.
- the live row shows latest measures plus current in-window counts before
  flush.
- cache size is fixed at startup from configured row count and discovered row
  shape, with a hard byte ceiling.
- the bottom bar is status-only in v1.
- the TTY surface uses the normal screen with scrollback.
- immediate event rows render inline in the single main pane.
- rendered rows do not need to show `win_s` by default when one window is
  active.
- the library should provide a small set of safe renderer and cache options,
  while adapters own app-specific wiring, commands, and payload meaning.

## Likely Consumers

Beyond VRLOG:

- LiDAR live status tails
- sweep progress tails
- deploy progress / health tails
- transit rebuild tails
- generic operator CLIs with history + live row output

## Recommendation

Adopt `TicTacTail` as the working package/repo name, keep the first version to
one active aggregate window and one main pane, keep the API small, and keep
memory and buffer use strictly bounded. VRLOG should remain a thin
emitter/projector on top.
