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
surface quickly, emits aligned history rows, and keeps the heavy logic outside
app-specific code.

The split should be:

1. `tictactail`: all aggregation, live refresh, history rendering, alignment,
   colours, spinner, and output mechanics
2. application emitter/projector: choose keys and feed flat samples
3. thin adapter: CLI command or route glue

The VRLOG checker should be a small import/config layer on top of this.

## What TicTacTail Owns

`TicTacTail` should own all generic behaviour:

- ingesting flat timestamped samples
- maintaining one or more hot aggregation windows
- producing aggregate rows and live snapshots
- tail-style history output
- live/status refresh loop
- split-pane TTY layout with slow/fast/live/status bands
- aligned columns between history panes and the live pane
- ANSI colouring for persisted rows
- configurable live spinner styles
- bounded per-window redraw caches for resize handling
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
- `ts_nanos`
- `sev`
- `src`
- `win_s`

Everything else is application payload and must remain unchanged.

`ts_nanos` is the only public timestamp field in the row contract. Adapters may
convert to or from `time.Time` internally, but the wire and engine-facing
format stays integer nanoseconds.

## No Mapping

TicTacTail should never rename keys.

If an app wants short keys, it emits short keys.

If an app wants longer keys, it emits longer keys.

Examples:

- short: `fr`, `fps`, `ch_cur`, `ch_tot`, `tr`, `cl`, `fg`, `bg`
- long: `frames`, `fps`, `chunk_cur`, `chunk_tot`, `tracks`

TicTacTail stores and renders whatever keys it is given.
There is no special `ag` alias; windowed rows render as `win_s=<seconds>`.

## Value Semantics

TicTacTail only understands two application field kinds:

1. measure
2. increment counter

Rule:

- keys ending in `_inc` are summed within each configured window as integer
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
  WindowSeconds []int
  RefreshHz     int
  SpinnerStyle  string
  Columns       []string
}

type Engine interface {
  Add(row Row) ([]Row, error)
  Live(nowNanos int64) Row
}
```

`ScalarValue` is validated at ingest. Nested objects, arrays, structs, and
non-integer `_inc` values are rejected.

`WindowSeconds` lists all hot aggregate buckets kept in parallel. The initial
TTY layout assumes `3` and `30`.

`Columns` is ordering only. It is not key mapping.

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

TicTacTail should keep multiple windows hot and emit rows for every configured
window on cutover.

Initial default:

- fast window `3`
- slow window `30`

Behaviour:

- maintain state for configured windows such as `3` and `30`
- update all hot windows on every sample
- emit an aggregate row whenever any configured window closes
- stamp emitted rows with raw `win_s`
- allow renderers to route `3` second rows and `30` second rows into different
  panes without changing keys

### Window Rules

For each window:

- measure fields keep latest value seen in that window
- `_inc` fields keep running integer sums in that window

At cutover:

- emit one aggregate row for the closing window
- reset `_inc` sums for that window
- start a fresh measure state for that window

At live refresh:

- the live row shows the latest unwindowed sample snapshot
- the live row is not required to carry `win_s`

## Local Aggregation Implementation

Use a single-owner aggregation loop.

Suggested model:

- one ingest goroutine owns all window state
- `Add()` forwards rows into that owner
- the owner updates all hot windows
- the owner publishes immutable live snapshots
- the renderer reads snapshots at configured refresh cadence
- the renderer keeps bounded per-window redraw caches for resize handling

Per-window state:

```go
type windowState struct {
  WindowSeconds int
  WindowID      int64
  Latest        map[string]ScalarValue
  Sums          map[string]int64
}
```

Renderer-side cache:

```go
type rowCache struct {
  WindowSeconds int
  Rows          []Row // bounded ring buffer, newest last
}
```

Use one bounded cache per rendered aggregate bucket, for example `3` and `30`.
Caches overlap in time and exist only to repopulate panes after resize. Older
history remains available through terminal scrollback or optional log sinks,
not by keeping an unbounded in-memory copy.

Update rule:

- reserved key -> route specially
- `ts_nanos` -> parse once as `int64` and use as the source of truth
- key ends in `_inc` -> numeric add into `Sums`
- otherwise -> overwrite `Latest`

Cutover rule:

- `windowID := tsNanos / (int64(windowSeconds) * 1_000_000_000)`
- if the id changed, flush previous row and rotate state

Why this is efficient:

- O(keys) per sample
- suffix check only
- no nested traversal
- no key mapping
- renderer can use atomic snapshot copies instead of sharing mutable maps
- resize cost is bounded by cache size rather than replay length

## Rendering Contract

TicTacTail owns the visual contract too.

### Output Shape

TicTacTail should support both a simple line-oriented stream and a split-pane
TTY surface.

Default interactive TTY layout:

1. top pane: `30` second aggregates, using the remaining terminal height
2. middle pane: recent `3` second aggregates, capped at `10` visible rows
3. lower pane: one-line live snapshot
4. bottom bar: one-line status/input bar, Vim-like in feel

The logical `30` second history is unbounded, but only the rows that fit in the
available height are visible at once. Bounded per-window caches are used only
to repaint recent rows after a resize; terminal scrollback or log sinks remain
the long-lived record.

### Refresh Cadence

Supported refresh targets:

- `5 Hz`
- `10 Hz`
- `20 Hz`

Default:

- `20 Hz` for interactive live use

The refresh rate affects only live-pane and status-bar redraw cadence, not
aggregation math.
The `3` second and `30` second panes repaint only when new rows arrive or the
terminal size changes.

### Spinner

Spinner frames are configurable for the live pane only.

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

The live pane uses the selected spinner prefix instead of a coloured status
dot.

### Alignment

TicTacTail should align slow history, fast history, and live-pane columns to
the same row shape where possible.

If panes render:

```text
🟢 12:00:30 win_s=30 fr=6014 fps=200 ch=7/19 tr=13 cl=21 fg=6312 bg=41077 st=0.87 dr=0 er=1
🟢 12:00:27 win_s=3  fr=602  fps=201 ch=7/19 tr=15 cl=24 fg=6620 bg=41201 st=n/a  dr=0 er=0
🌔 live          frame=6341/18210 fps=201 ch=7/19 tr=15 cl=24 fg=6620 bg=41201 run
-- NORMAL -- src=vrlog q quit / filter
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
- `win_s` -> `win_s=30`

This is formatting, not key remapping.

### Resize And Cache Behaviour

The renderer should keep separate bounded caches for each configured aggregate
window, at least one for `3` second rows and one for `30` second rows.

Rules:

- the `3` second cache must exceed the visible `10` row pane height
- the `30` second cache should be bounded by row count or memory budget, never
  unbounded
- caches may overlap in time because both windows describe the same source at
  different granularities
- a resize should re-slice cached rows into panes without replaying input
- if the new terminal size exceeds cached history, show the newest rows and
  leave older rows in scrollback

### Non-TTY Mode

If stdout is not a TTY:

- disable the pane compositor
- emit `3` second rows, `30` second rows, and event rows as line-oriented
  output
- optionally sample live rows at a lower cadence for logs

## Output Examples

Aggregate row (`30` second bucket):

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

Aggregate row (`3` second bucket):

```json
{
  "kind": "agg",
  "ts_nanos": 1741435233000000000,
  "win_s": 3,
  "sev": "ok",
  "src": "vrlog",
  "fr_inc": 602,
  "fps": 201.0,
  "ch_cur": 7,
  "ch_tot": 19,
  "tr": 15,
  "cl": 24,
  "fg": 6620,
  "bg": 41201,
  "st": null,
  "dr_inc": 0,
  "er_inc": 0
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
- predictable resize reflow cost from bounded caches

### Required Benchmarks

- `BenchmarkEngineAdd_FileReplay`
- `BenchmarkLivePaneRender_5Hz`
- `BenchmarkLivePaneRender_10Hz`
- `BenchmarkLivePaneRender_20Hz`
- `BenchmarkPaneResizeReflow`
- `BenchmarkFileReplayWithUI_5Hz`
- `BenchmarkFileReplayWithUI_10Hz`
- `BenchmarkFileReplayWithUI_20Hz`

### Required Tests

- live pane updates at configured cadence
- `3` second and `30` second rows cut over correctly and independently
- `_inc` values reset on cutover
- measures always reflect latest value in each hot window
- the `3` second pane never renders more than `10` visible rows
- resize reflows from cache without replaying the source
- aggregate rows are identical regardless of `5/10/20 Hz`

### Acceptance Criteria

- renderer work stays decoupled from ingest
- `20 Hz` live/status redraw is responsive
- panes redraw only on append or resize
- bounded caches stay within configured limits
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
  WindowSeconds: []int{3, 30},
  RefreshHz:     20,
  SpinnerStyle:  "moon",
  Columns:       []string{"win_s", "fr", "fps", "ch", "tr", "cl", "fg", "bg", "st", "dr", "er"},
})
```

The initial renderer then places `30` second rows in the long-history pane and
`3` second rows in the recent pane.

Then the adapter just feeds projected samples and starts the renderer.

## Complexity Review

### Benefits

- one flat row contract lets multiple CLIs share aggregation and rendering
- raw `ts_nanos` plus scalar values keep the wire shape simple and language
  neutral
- integer counters avoid accidental floating-point drift in aggregate totals
- concurrent `3` second and `30` second buckets support fast and slow views at
  the same time
- bounded caches let the UI recover from terminal resizes without replaying the
  source

### Costs And Shortcomings

- the public row contract is still runtime-validated rather than compile-time
  enforced
- split-pane terminal UI is materially more complex than a simple footer-only
  tail UI
- bounded caches improve resize behaviour but cannot recreate arbitrarily old
  history after a large resize
- keeping multiple hot windows and redraw caches increases memory pressure
- a generic library can easily grow too much UI policy if pane behaviour is not
  kept disciplined

### Questions To Resolve

- should `ts_nanos` be mandatory on every input row, or may adapters stamp it
  if the source omits it?
- should live rows remain strictly unwindowed, or may they optionally expose
  running `3` second / `30` second counters too?
- what cache budget should each window get: fixed rows, fixed bytes, or dynamic
  by terminal size?
- should the bottom status/input bar accept commands in v1, or remain
  status-only until later?
- should pane mode use the normal screen with scrollback, or an alternate
  screen when interactive input is enabled?
- how should immediate event rows appear when panes are active: inline,
  transient overlay, or a dedicated event lane?
- should `win_s` always be visible in rendered aggregate rows, or may some
  consumers hide it when pane position already implies the bucket?
- where is the boundary between generic pane policy in `tictactail` and
  application-specific behaviour in the adapter?

## Likely Consumers

Beyond VRLOG:

- LiDAR live status tails
- sweep progress tails
- deploy progress / health tails
- transit rebuild tails
- generic operator CLIs with history + live panes

## Recommendation

Adopt `TicTacTail` as the working package/repo name, move all visual/tail/live
contract details here, and keep VRLOG as a thin emitter/projector on top.
