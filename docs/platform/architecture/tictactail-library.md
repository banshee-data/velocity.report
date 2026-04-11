# TicTacTail Library

A generic platform library that takes flat key/value samples, refreshes a live
surface quickly, emits aligned history rows for one active aggregate window,
and keeps the heavy logic outside app-specific code.

Active plan: [tictactail-platform-plan.md](../../plans/tictactail-platform-plan.md)

## Working Name

`TicTacTail` — `tic tac` captures cadence and regular refresh; `tail`
captures persistent history output. Distinctive enough for a standalone
package/repo name.

## Ownership Split

1. `tictactail`: all aggregation, live refresh, history rendering, alignment,
   colours, spinner, and output mechanics
2. Application emitter/projector: choose keys and feed flat samples
3. Thin adapter: CLI command or route glue

TicTacTail owns all generic behaviour:

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

## Repo / Module Strategy

### Phase 1: Incubate Here

Public package path: `pkg/tictactail/`

Rules:

- no imports from `internal/*`
- no LiDAR- or VRLOG-specific types
- no key mapping or alias tables
- no app-specific metric names inside the engine

### Phase 2: Split Out

Once there is a second real consumer, split to
`github.com/banshee-data/tictactail`.

## Core Contract

TicTacTail accepts only flat rows:

- one-layer JSON
- key/value only
- JSON scalar values only

Allowed values: `string`, `int64`, `float64`, `bool`, `null`.

Disallowed: nested objects, arrays, app structs.

Reserved keys:

| Key        | Purpose                                   |
| ---------- | ----------------------------------------- |
| `kind`     | Row type (`agg`, `live`, `event`, `init`) |
| `ts_nanos` | Mandatory timestamp (int64 nanoseconds)   |
| `sev`      | Severity level                            |
| `src`      | Source identifier                         |
| `win_s`    | Window duration in seconds                |

Everything else is application payload and must remain unchanged.

`ts_nanos` is mandatory on every input row. Rows without it are rejected.

## No Mapping

TicTacTail never renames keys.

If an app wants short keys, it emits short keys. If an app wants longer keys,
it emits longer keys. TicTacTail stores and renders whatever keys it is given.

There is no special `ag` alias; aggregate rows carry raw `win_s=<seconds>`,
though a single-pane renderer may hide it by default.

## Value Semantics

Two application field kinds only:

1. **Measure** — all non-reserved keys not ending in `_inc`; keeps latest
   value seen in the active window
2. **Increment counter** — keys ending in `_inc`; summed within the active
   window as integer counters

Rules:

- `_inc` fields must be emitted as `int64`
- count-like measures (`frame_cur`, `frame_tot`, `tr`, `cl`, `fg`, `bg`)
  should prefer `int64`
- `float64` is reserved for genuinely continuous measures (`fps`, `st`)

## API

> **Source:** `pkg/tictactail/` (when implemented). `ScalarValue` is `any` restricted to nil, string, int64, float64, or bool (validated at ingest). `Row` is `map[string]ScalarValue`. `Config` struct carries Source, WindowSeconds, RefreshHz, SpinnerStyle, Columns (ordering only), HistoryRows, and MaxBytes. `Engine` interface exposes `Add(Row) (*Row, error)` and `Live(nowNanos int64) Row`.

- `ScalarValue` validated at ingest. Nested objects, arrays, structs, and
  non-integer `_inc` values are rejected.
- `WindowSeconds` selects the single active aggregate bucket. Changing it
  resets the current bucket and starts a fresh window.
- `Columns` is ordering only, not key mapping.
- `HistoryRows` bounds the aggregate history cache used for resize reflow.
- `MaxBytes` is a hard cap on internal allocation after schema freeze.
- `SpinnerStyle` selects `moon`, `braille8`, or `ascii`. Zero value defaults
  to `moon`; falls back to `ascii` when Unicode output is disabled.

## Input Examples

### Sample

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

### Event

```json
{
  "ts_nanos": 1741435235123000000,
  "kind": "event",
  "sev": "warn",
  "msg": "missing_bg_snapshot",
  "er_inc": 1
}
```

### Init

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

One active aggregate window; emit one row when that window closes.

Default window: `30` seconds.

### Window Rules

For the active window:

- measure fields keep latest value seen
- `_inc` fields keep running integer sums

At cutover:

- emit one aggregate row for the closing window
- reset `_inc` sums
- start fresh measure state

At live refresh:

- the live row shows latest measures plus current in-window `_inc` counts
  before flush
- the live row is not required to carry `win_s`

If `WindowSeconds` changes, drop the partial bucket, reset measures and
counters, and start a fresh window immediately.

## Local Aggregation Implementation

Single-owner aggregation loop:

- one ingest goroutine owns schema state and the active window
- `Add()` forwards rows into that owner
- the owner updates the active window
- the owner publishes immutable live snapshots
- the renderer reads snapshots at configured refresh cadence
- the renderer keeps one bounded history cache for resize handling
- startup rows are scanned once to freeze key and type layout before steady
  state ingest
- after schema freeze, allocations are fixed and bounded by `MaxBytes`

### Internal State

> **Source:** Same package. `fieldSpec` pairs a key name with a `fieldKind` (measure vs increment). `windowState` tracks the active WindowSeconds, WindowID, a `Latest []ScalarValue` for measures, and `Sums []int64` for `_inc` counters. All slices are fixed-size after schema freeze.

Renderer-side cache:

> **Source:** Same package. `rowCache` is a bounded ring buffer of `Row` values (newest last) with Head, Count, and MaxBytes fields. Capacity is fixed at startup.

### Update Rule

- reserved key → route specially
- `ts_nanos` → must parse as `int64`, source of truth
- key ends in `_inc` → numeric add into indexed `Sums`
- otherwise → overwrite indexed `Latest`
- unknown keys or type drift after schema freeze → reject row

### Cutover Rule

```
windowID := tsNanos / (int64(windowSeconds) * 1_000_000_000)
```

If the ID changed, flush previous row and rotate state.

### Complexity

- O(keys) per sample
- suffix check only
- no nested traversal
- no key mapping
- fixed slices after schema freeze instead of unbounded mutable maps
- resize cost bounded by cache size rather than replay length
- no unbounded buffer growth in steady state

## Rendering Contract

### Output Shape

Default interactive TTY layout:

1. **Main pane:** aggregate history, using the remaining terminal height
2. **Lower row:** one-line live snapshot
3. **Bottom bar:** one-line status bar (status-only in V1)

Event rows render inline in the main history pane. The logical history is
unbounded, but only the rows that fit in the available height are visible at
once. A single bounded cache is used only to repaint recent rows after a
resize; the renderer uses the normal screen with scrollback rather than an
alternate screen.

### Refresh Cadence

Supported targets: `5 Hz`, `10 Hz`, `20 Hz`.

Default: `20 Hz` for interactive live use.

The refresh rate affects only live-row and status-bar redraw cadence, not
aggregation math. The main pane repaints only when a new aggregate or event
row arrives or the terminal size changes.

### Spinner

Spinner frames for the live row only:

| Style      | Frames                  | When                |
| ---------- | ----------------------- | ------------------- |
| `moon`     | 🌑 🌒 🌓 🌔 🌕 🌖 🌗 🌘 | Default (Unicode)   |
| `braille8` | ⣾ ⣽ ⣻ ⢿ ⡿ ⣟ ⣯ ⣷         | Alternate (Unicode) |
| `ascii`    | `\| / - \`              | Non-Unicode / logs  |

Persisted history rows do not use spinner frames.

### Colour

Persisted history rows keep ANSI colours when stdout is a TTY:

- green for ok
- yellow for warn
- red for fail

The live row uses the selected spinner prefix instead of a coloured status dot.

### Alignment

TicTacTail aligns main history and live-row columns to the same row shape:

```
🟢 12:00:30 fr=6014 fps=200 ch=7/19 tr=13 cl=21 fg=6312 bg=41077 st=0.87 dr=0 er=1
🌔 live      frame=6341/18210 fr=311 fps=201 ch=7/19 tr=15 cl=24 fg=6620 bg=41201 run
status src=vrlog win=30 rows=200 tty=on
```

### Pair Formatting

Generic suffix conventions in the renderer (formatting, not key remapping):

- `*_cur` + `*_tot` → render as `<base>=cur/tot`
- `*_inc` → render as `<base>=sum`

Examples:

- `ch_cur` + `ch_tot` → `ch=7/19`
- `frame_cur` + `frame_tot` → `frame=6341/18210`
- `fr_inc` → `fr=6014`

### Resize And Cache

- cache size fixed at startup from `HistoryRows` and discovered row shape
- total allocated bytes stay within `MaxBytes`
- cache capacity never grows after startup
- resize re-slices cached rows into the main pane without replaying input
- if new size exceeds cached history, show newest rows and leave older in
  scrollback

### Non-TTY Mode

- disable TTY compositor
- emit aggregate and event rows as line-oriented output
- optionally sample live rows at lower cadence for logs

## Output Examples

### Aggregate Row

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

### Live Row

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

### Test Matrix

Run the same file-processing workload with live/status refresh at 5, 10, and
20 Hz. Validate:

- identical aggregate output across all three rates
- acceptable live/status responsiveness
- minimal file-processing overhead as refresh increases
- stable allocation behaviour
- predictable resize reflow cost from one bounded cache
- no allocation growth after schema freeze

### Required Benchmarks

- `BenchmarkEngineAdd_FileReplay`
- `BenchmarkLiveRowRender_{5,10,20}Hz`
- `BenchmarkMainPaneResizeReflow`
- `BenchmarkFileReplayWithUI_{5,10,20}Hz`

### Required Tests

- live row updates at configured cadence
- aggregate rows cut over correctly for the configured window
- changing `WindowSeconds` resets current measures and sums
- `_inc` values reset on cutover
- measures always reflect the latest value in the active window
- schema freezes on startup and rejects new keys or type drift
- resize reflows from cache without replaying the source
- aggregate rows are identical regardless of refresh rate
- internal buffers stay within `MaxBytes`

### Acceptance Criteria

- renderer work stays decoupled from ingest
- `20 Hz` live/status redraw is responsive
- main pane redraws only on append or resize
- bounded caches stay within configured limits
- no unbounded allocation growth after schema freeze
- `20 Hz` does not materially distort file replay throughput

## App Integration

Applications should have very little code around TicTacTail:

```text
pkg/tictactail/             # all heavy logic
internal/lidar/vrreport/    # projector + config only
cmd/radar/radar.go          # dispatch only
```

Minimal integration:

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

The renderer shows aggregate history in the main pane, one live row, and one
status bar. The adapter feeds projected samples and starts the renderer.

## Design Review

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

- `ts_nanos` mandatory on every input row
- live row shows latest measures plus current in-window counts before flush
- cache size fixed at startup from configured row count and discovered row shape
  with a hard byte ceiling
- bottom bar is status-only in V1
- TTY surface uses normal screen with scrollback
- immediate event rows render inline in the single main pane
- rendered rows do not show `win_s` by default when one window is active
- the library provides a small set of safe renderer and cache options; adapters
  own app-specific wiring, commands, and payload meaning
