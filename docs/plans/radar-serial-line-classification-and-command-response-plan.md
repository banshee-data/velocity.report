# Radar serial line classification and command response plan

- **Status:** Draft
- **Layers:** Go server (`internal/serialmux`, `internal/api`, radar ingest in `cmd/radar`)
- **Target:** Unscheduled. Enables the command dashboard (the `GET /api/commands` dropdown shipped on PR 461) to display the result of a query command, not just fire it.
- **Related:** [Security surface](../../.github/knowledge/security-surface.md), `internal/radar/commands.go` (advisory command catalogue)
- **Canonical:** none yet; this plan is the working design until a hub doc is warranted.

## Motivation

`POST /admin/radar/command` is write-only today. The operator sends an OPS24x command and the
sensor's reply vanishes into the database log and the localhost-only `/debug/tail`
stream. Query commands (`?V` firmware, `O?` output settings, `??` module info,
`A?` persistent settings, `N?` object count) are far more useful if the dashboard
can show the answer next to the command dropdown. Closing that loop is the
user-facing goal.

The obvious quick fix (subscribe to the serial stream, read for a few hundred
milliseconds after sending, return whatever arrived) is the wrong shape for this
codebase. It would add a _third_ consumer that independently interprets the raw
line stream, on top of the database persister and `/debug/tail`, each with its own
idea of what a line means. The classifier (`ClassifyPayload`) already lives in only
one of those consumers; bolting response capture on as another would duplicate and
risk diverging that logic. It would also, without care, return live measurement
data on a non-debug endpoint.

The consequence of inaction, or of the quick fix, is twofold: query commands stay
half-useful, and every new surface that wants to read the serial stream reinvents
line classification slightly differently. This plan makes classification a single
shared path and treats the command response as one well-behaved consumer of it.

## Current state

Facts as of this branch (`internal/serialmux/serialmux.go`, `parse.go`,
`handlers.go`; `internal/api/`; `cmd/radar/radar.go`).

- **Send is fire-and-forget.** `SendCommand` (serialmux.go:141) takes `commandMu`,
  writes `command + "\n"`, returns. It never reads. The control-character reject in
  `sendCommandHandler` (server.go:168) already prevents `\n`/`\r`/null smuggling.
- **One scanner, lossy fan-out of raw strings.** `Monitor` (serialmux.go:158) runs a
  single goroutine with a `bufio.Scanner`. Each scanned line is fanned out to every
  subscriber as a raw `string` via `select { case ch <- line: default: }`
  (serialmux.go:213). Subscriber channels are buffer size 1
  (`subscriberChannelBufferSize`, serialmux.go:31), so a line is dropped if the
  subscriber is not ready at that instant.
- **`Subscribe()` returns `(string, chan string)`** (serialmux.go:86): a raw,
  unclassified line stream.
- **Three independent consumers of that raw stream:**
  1. Database persistence (cmd/radar/radar.go:913): `Subscribe()` then
     `serialmux.HandleEvent` (handlers.go:75), which calls `ClassifyPayload` and
     routes to `HandleRadarObject` / `HandleRawData` / `HandleConfigResponse`.
     Config lines also merge into `currentState` (handlers.go:53).
  2. `/debug/tail` SSE (serialmux `AttachAdminRoutes`, serialmux.go:339+):
     `Subscribe()` then streams every raw line. No classification. Localhost or
     Tailscale only.
  3. API re-fan-out: `SerialPortManager.runEventFanout` (serial_reload.go:152) holds
     one mux subscription and re-fans to API-level subscribers (buffer 10, also
     lossy, logs "dropping event"). Survives a port reload.
- **`ClassifyPayload` is substring-based** (parse.go:15): `end_time`/`classifier` →
  `radar_object`; `magnitude`/`speed` → `raw_data`; leading `{` → `config`; else
  `unknown`. It runs in exactly one consumer (the persister). The substring test is
  fragile: a settings echo that contains the word "speed" classifies as `raw_data`,
  and any non-JSON line falls through to `unknown`.
- **The OPS24x protocol has no request/response framing.** In JSON mode (`OJ`, set in
  `Initialise`), a query's reply is emitted as JSON line(s) interleaved with the
  continuous measurement stream on the same UART. There is no tag tying a reply to
  the command that caused it.
- **`/api/events` is a database query, not a live stream** (server_admin.go:95). The only
  network-exposed live view of raw serial lines today is `/debug/tail`, behind the
  localhost/Tailscale boundary.

## Findings

| Area                    | Current state                                                                    | Severity | Release view                                          |
| ----------------------- | -------------------------------------------------------------------------------- | -------- | ----------------------------------------------------- |
| Classification location | Runs in one consumer (DB persister); other consumers see raw lines               | Medium   | Centralise before adding a third consumer             |
| Classifier robustness   | Substring match; misfiles config echoes containing "speed"; non-JSON → `unknown` | Medium   | Replace with structure-aware classifier               |
| Response correlation    | No protocol framing; reply interleaves with measurement stream                   | High     | Heuristic, best-effort only; document as such         |
| Fan-out reliability     | Buffer-1, lossy `default:` drop; reply can be dropped under load                 | High     | Needs a non-lossy armed tap for capture               |
| Live-data disclosure    | A naive capture window returns measurement JSON on `/admin/radar/command`        | High     | Filter by class structurally; never return object/raw |
| Stream type             | `Subscribe()` yields `chan string`; class is not carried                         | Medium   | Carry classification on the line; interface change    |

## Design / approach

The backbone is a single rule: **classify each line exactly once, at the point it
leaves `Monitor`, and fan out a classified line that every consumer reads the same
way.** The command-response feature is then a thin, well-behaved consumer of that
classified stream, not a new path that reinterprets raw lines.

### 1. One classified line type, one classifier, one place

Introduce a typed line and a class enum in `internal/serialmux`:

```go
type LineClass int

const (
    ClassRadarObject LineClass = iota // measurement: detected object
    ClassRawData                      // measurement: raw speed/magnitude sample
    ClassConfig                       // settings echo, query reply, module info (JSON object)
    ClassOther                        // banners, blank-data lines, free-text acks, errors, non-JSON
)

type Line struct {
    Raw    string         // the exact bytes scanned, for debug/tail and storage
    Class  LineClass      // computed once, in Monitor
    Fields map[string]any // parsed JSON object when present, else nil (avoids re-parsing downstream)
}
```

`Monitor` classifies once and fans out `Line`, not `string`. `Subscribe()` becomes
`(string, chan Line)`. Every surface receives the identical classified value:

- **Database persister** switches on `Line.Class` instead of calling
  `ClassifyPayload` itself. `HandleEvent` becomes a router over the attached class.
  Config lines still merge into `currentState`. `Line.Fields` is reused so JSON is
  parsed once, not again per consumer.
- **`/debug/tail`** renders `Line.Raw`, optionally annotated with the class label and
  optionally filterable by class via a query parameter. It no longer defines its own
  notion of line types; the raw view is just the `Raw` field.
- **Command response** (below) reads the same stream and selects by `Class`.

This is the DRY guarantee, stated precisely: one classifier function, invoked once
per line, in one location; one stream type; no second code path that sees raw,
unclassified lines and reinterprets them.

### 2. A more robust classifier

Replace the substring matcher with a structure-aware one. Classify once, attach the
parsed object so nobody parses twice:

1. Try to parse the line as a JSON object once (`json.Unmarshal` into
   `map[string]any`, or `json.Valid` plus a light decode).
2. If it is a JSON object, classify by **keys**, not substrings:
   - object-detection keys present (e.g. `end_time`, `classifier`) → `ClassRadarObject`;
   - measurement keys present as actual keys (e.g. `magnitude` and/or `speed`) →
     `ClassRawData`;
   - otherwise (settings echoes, version/info, persistent-memory dumps) →
     `ClassConfig`.
3. If it is not a JSON object (banner text, blank-data sentinel, free-text
   acknowledgement, error string) → `ClassOther`.

Checking keys removes the "speed appears in a config echo" false positive that the
current substring test produces. The exact key names must be taken from AN-010-Z and
a live JSON-mode capture, not guessed (see open questions). `ClassOther` is the
deliberate catch-all so an undocumented or malformed line is never silently
mistaken for measurement data.

### 3. Command response as a consumer of the classified stream

Add a correlation method on `SerialMux` and the interface:

```go
SendCommandWithResponse(ctx context.Context, command string, timeout time.Duration) (CommandResult, error)

type CommandResult struct {
    Sent     bool
    Response []string // Raw of captured ClassConfig / ClassOther lines only
    TimedOut bool
}
```

Behaviour:

- Hold `commandMu` for the **whole** exchange, not just the write. This serialises
  command/response pairs and removes cross-request bleed: two callers cannot capture
  each other's replies.
- Arm a capture **before** writing. Because the normal fan-out is lossy (buffer 1),
  the reply can be dropped exactly when it matters. So the armed capture uses a
  dedicated tap that `Monitor` writes to _without_ the `default:` drop while a
  capture is live (bounded, with the timeout as the backstop so a stalled reader can
  never block the scanner). The alternative, a best-effort larger-buffer subscriber,
  is simpler but can still miss the reply under load; the choice is an open question.
- After the write, read classified lines from the tap until the first `ClassConfig`
  line, or N lines, or the timeout, whichever comes first.
- Return only `ClassConfig` and `ClassOther` lines. Never return `ClassRadarObject`
  or `ClassRawData`. This is the structural answer to "do not display the live data":
  measurement lines are filtered out by class, by construction, not by an ad-hoc
  string filter at the handler.

The HTTP surface stays opt-in so the default never blocks:

- `POST /admin/radar/command` remains fire-and-forget by default.
- `POST /admin/radar/command?wait=true&timeout=…` calls `SendCommandWithResponse` and returns
  JSON `{ "sent": true, "response": [...], "timed_out": false }`.
- The existing control-character reject and advisory catalogue warning are kept
  unchanged in front of both paths.
- Default success responses speak JSON too: the fire-and-forget path returns
  `{ "sent": true }` rather than the plain-text `Command sent successfully`, matching
  the JSON error bodies the endpoint now returns. This is a body-shape change only;
  the default still never blocks. (2026-06 API error-body audit follow-in.)

### Boundaries and invariants

- The scanner goroutine in `Monitor` must never block. The armed tap is bounded and
  governed by the timeout; if the consumer is gone, the capture disarms.
- Classification happens at the mux `Monitor` (the earliest point), so both the
  mux-level and the API `SerialPortManager` tiers carry `Line` and never re-derive
  class.
- `currentState` merge semantics for config lines are preserved exactly.
- A timeout with no reply is a normal outcome (`TimedOut: true`), not an error. Some
  commands legitimately emit nothing.

## Scope

### Item 1: classify once, fan out a typed line

**Summary:** Introduce `Line` / `LineClass`, replace `ClassifyPayload` with a
structure-aware classifier, classify in `Monitor`, and change `Subscribe()` to yield
`chan Line`.

**Steps:**

1. Add `Line`, `LineClass`, and the key-based classifier in `internal/serialmux`.
2. Classify each scanned line once in `Monitor`; fan out `Line`.
3. Migrate the DB persister to switch on `Line.Class` and reuse `Line.Fields`.
4. Update `SerialMuxInterface`, `SerialPortManager`, `/debug/tail`, and the test
   mocks to the new stream type.
5. Tests: classifier truth table (object, raw, config echo containing "speed",
   non-JSON, malformed), and that every consumer sees identical class.

**Milestone:** unscheduled.

### Item 2: command/response correlation in SerialMux

**Summary:** Add `SendCommandWithResponse` with a non-lossy armed tap and
`commandMu` held across the exchange.

**Steps:**

1. Add the armed-capture tap to `Monitor` (or the best-effort variant; see questions).
2. Implement `SendCommandWithResponse`: arm, write, collect by class, stop on first
   config line / N lines / timeout.
3. Return `ClassConfig` + `ClassOther` only; filter measurement classes structurally.
4. Tests: reply captured under live measurement traffic; measurement lines never
   returned; timeout path; concurrent callers do not interleave.

**Milestone:** unscheduled.

### Item 3: opt-in HTTP surface

**Summary:** `POST /admin/radar/command?wait=true&timeout=…` returns the filtered response as
JSON; default stays fire-and-forget.

**Steps:**

1. Parse `wait` / `timeout` (clamp to a sane max), keep control-char reject and
   advisory warn.
2. Call `SendCommandWithResponse`; encode `{sent, response, timed_out}`.
3. Align the default fire-and-forget success body with the JSON error bodies the
   endpoint now returns: respond `{ "sent": true }` instead of the plain-text
   `Command sent successfully` (2026-06 API error-body audit follow-in; see
   `cli-restructuring-plan.md`). The default response shape changes, so update the
   handler tests that currently assert the plain-text body.
4. Handler tests for the wait path, timeout, and the default path.

**Milestone:** unscheduled.

### Item 4 (optional): class-aware `/debug/tail`

**Summary:** Annotate tail output with the class label and allow per-class filtering.
Proves the shared classified stream end to end.

**Milestone:** unscheduled.

## Dependencies

- AN-010-Z and a live JSON-mode capture to fix the exact response key names the
  classifier keys on. Without this the classifier is a guess.
- The advisory command catalogue (`internal/radar/commands.go`) and `GET /api/commands`
  shipped on PR 461 are the consumer this work serves.

## Caveats

These are inherent to the protocol and the architecture; the design manages them but
cannot remove them.

- **Correlation is heuristic.** The OPS24x emits replies without any tag. "First
  config line within the window" is a best guess, not a guarantee that the line
  belongs to this command rather than an unrelated config line or a concurrent
  device-initiated change. `commandMu` removes only the _self-inflicted_ concurrency.
- **The armed tap touches the hot loop.** Making capture non-lossy means `Monitor`
  behaves differently while a capture is live. It must be bounded and timeout-guarded
  so a slow or absent reader can never stall the scanner. This is the most delicate
  part of the change.
- **Classifier depends on a schema we must confirm.** Key-based classification is only
  as good as the key list. Undocumented or firmware-specific lines fall to
  `ClassOther`, which is safe (never returned as measurement) but may mean a real
  reply is returned as `Other` rather than `Config`.
- **Interface change has blast radius.** Moving `Subscribe()` from `chan string` to
  `chan Line` touches `SerialMuxInterface`, `SerialPortManager`, `/debug/tail`,
  `cmd/radar`, and several test mocks. Mechanical, but wide.
- **`commandMu` held across the window serialises commands.** A burst of `wait=true`
  calls queues behind the timeout. A tight default timeout and a hard cap keep this
  bounded.
- **Setter and reset commands vary.** A setter (`US`, `OJ`) usually echoes a config
  object; a reset (`AX`) may emit a banner (`ClassOther`) or a burst; some commands
  emit nothing. The result type treats all of these as first-class, including the
  empty/timeout case.

## Open questions for implementation time

1. **Response shapes.** What exact JSON keys distinguish object, raw, and config
   replies in JSON mode? Capture a live session and reconcile with AN-010-Z before
   finalising the classifier key list.
2. **Stream type migration.** Change `Subscribe()` to `chan Line` (clean, wider blast
   radius, recommended), or keep `chan string` and carry class via a parallel typed
   channel or wrapper (smaller change, reintroduces a second view)?
3. **Capture reliability.** Non-lossy armed tap in `Monitor` (reliable, more hot-loop
   complexity) versus a best-effort larger-buffer subscriber (simpler, can miss the
   reply under load). Which trade-off do we accept?
4. **API shape.** New `SendCommandWithResponse` plus `?wait=true` opt-in (recommended,
   keeps the default non-blocking), or change `/admin/radar/command` semantics outright?
5. **Stop condition.** First `ClassConfig` line, first N lines, or all lines within
   the window? What are N and the byte cap?
6. **Timeout policy.** Default and maximum (for example 500 ms default, 2 s cap). Is
   holding `commandMu` for the full window acceptable backpressure on the command
   path?
7. **Multi-line replies.** Does JSON mode collapse `??` / `A?` into a single object,
   or several lines? This decides the stop condition for those commands.
8. **Device-initiated config during a capture.** A config line not caused by this
   command can arrive mid-window. It is still captured and still flows to the DB. Is
   returning it acceptable, or do we need tighter scoping?
9. **Port reload mid-capture.** `SerialPortManager` can swap the underlying mux during
   a reload. Define capture behaviour across a swap (most likely: timeout and report
   not captured).
10. **`/debug/tail` scope.** Add class labels and per-class filtering now (good proof
    of the shared stream) or defer to a follow-up?

## Risks

| Risk                                                                 | Likelihood | Impact | Mitigation                                                                                                                                                                                                                                                                                                  |
| -------------------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Armed tap blocks the scanner goroutine                               | Medium     | High   | Bound the tap, govern with timeout, disarm on consumer exit; never block `Monitor`                                                                                                                                                                                                                          |
| Live measurement data leaks onto `/admin/radar/command`              | Medium     | High   | Filter by class structurally; return only `ClassConfig`/`ClassOther`, never measurement                                                                                                                                                                                                                     |
| Classifier mis-keys a real reply                                     | Medium     | Medium | Enumerate keys from spec + live capture; `ClassOther` catch-all is safe                                                                                                                                                                                                                                     |
| Interface change breaks compilation across packages                  | High       | Low    | Mechanical; the build surfaces every call site                                                                                                                                                                                                                                                              |
| `commandMu` held during window serialises commands                   | Medium     | Medium | Tight default timeout, hard cap, document the contract                                                                                                                                                                                                                                                      |
| Heuristic returns an unrelated config line                           | Low        | Medium | Serialise with `commandMu`; document best-effort; cap N                                                                                                                                                                                                                                                     |
| Duplicate response types / JSON detection vs `POST /api/serial/test` | Medium     | Medium | `serial.go` already returns `SerialCommandResult{Command, Response, IsJSON}` for the offline exclusive-open case; unify it onto `Line`/`LineClass` so JSON detection lives in one place. Its live path refuses to read (cannot second-open the port), which confirms the Monitor tap is the only live route |
| Overlap with SSE backpressure work (#380, v0.5.7)                    | Medium     | Medium | The outstanding drop/notify policy touches the same fan-out; coordinate the non-lossy armed tap with that work so the two do not diverge                                                                                                                                                                    |
| Item 1 refactor balloons into a big-bang interface change            | Medium     | Medium | Decompose: introduce `Line` behind a `chan string` shim, migrate consumers, then drop the shim, rather than one wide cut                                                                                                                                                                                    |

## Checklist

### Complete

- [x] Investigation: traced send path, fan-out, classifier, and the three stream
      consumers; confirmed `/api/events` is a DB query and `/debug/tail` is the only live
      exposure.

### Outstanding

- [ ] Item 1: classify once, fan out `Line`, robust key-based classifier (`L` effort)
- [ ] Item 2: `SendCommandWithResponse` with armed tap and `commandMu` serialisation (`M` effort)
- [ ] Item 3: opt-in `?wait=true` HTTP surface returning filtered JSON (`S` effort)
- [ ] Resolve open questions 1 to 10 before coding (`S` effort)

### Deferred

- [ ] Item 4: class-aware `/debug/tail` labels and filtering (proof surface, not required for the feature)

### Accepted residuals (no action planned)

- [ ] Perfect command/response correlation: impossible without protocol framing the
      OPS24x does not provide. Best-effort capture is the accepted ceiling.
