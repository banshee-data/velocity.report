# LiDAR visualiser stream robustness (v0.5.7)

- **Status:** Complete
- **Layers:** L9 endpoints (gRPC streaming, publisher), macOS visualiser (Swift client, UI)
- **Target:** v0.5.7; the state model landed, and then the stream carrying it proved unreliable
- **Canonical:** [data-source-switching.md](../lidar/operations/data-source-switching.md)

## Motivation

[The pipeline state model](lidar-pipeline-state-model-plan.md) made the server
able to say what was driving the pipeline. Using it exposed a second class of
problem: the client frequently was not receiving what the server sent, and
neither end could tell you so.

Three symptoms, reported as separate bugs, turned out to share that root:

- Selecting a VRLOG beachballed the app for over a minute.
- The visualiser showed `LIVE` over an empty scene, indistinguishable from a
  dead sensor.
- Bounding boxes "disappeared" in both live and replay.

The last two were the same bug wearing different clothes. The first is reduced
to a longer field-confirmation item in the backlog: the implementation work is
complete, but one clean 25-minute session is evidence rather than a lifetime
warranty. Getting there required making the stream describe its own failures,
which is most of what this plan delivers.

## Findings

| Area                    | Before                                                                                               | Severity |
| ----------------------- | ---------------------------------------------------------------------------------------------------- | -------- |
| Drop accounting         | Publish-stage and client-stage loss summed into one ratio, pegging at 50% and hiding the real rate   | High     |
| Stall visibility        | A blocked send severed the stream with no indication which frame or client was at fault              | High     |
| Stream lifecycle        | A stream ending signalled only "replay finished"; a server restart left the client showing connected | Critical |
| Background on subscribe | Published once per interval, so a client connecting moments later rendered over nothing              | High     |
| Background after settle | Sent on a 30s timer only, so a 6s settling left ~24s of foreground over an empty grid                | High     |
| Settling visibility     | Not reported at all; an empty warm-up scene looked like a fault                                      | High     |
| Source switching        | A `Picker` whose binding rejected every change, rendering as two inert buttons                       | Medium   |
| Return to live          | Did not restart the gRPC stream, so a wedged replay stream stayed wedged and live never appeared     | High     |
| Inspector pane          | Shown when either the toggle or a selected track asked for it, so the toggle could not close it      | Medium   |

## Design

Three principles carry the work.

**A stream must describe its own failure.** The stall warning names the frame,
its type, its point count, and its serialised size, because a stall is nearly
always one specific message a client cannot digest and the identity is the whole
question. Drop accounting separates publish-stage from client-stage loss rather
than summing them, so neither hides the other.

**A client's first frame must be sufficient to render.** A background frame is
published once and not repeated until the next refresh. The publisher now
remembers the last one and hands it to each client as it subscribes, and sends a
fresh one the moment settling completes. Only backgrounds are cached: caching a
foreground frame would replay one arbitrary moment of perception data to every
future client.

**An empty scene must say why it is empty.** Until the background grid settles,
foreground extraction yields nothing. Settling state travels on the wire beside
the source mode, and the visualiser shows `SETTLING 06s` in place of the source
badge — settling outranks the source deliberately, because it is the thing that
needs explaining and it resolves on its own. A live source whose sensor has not
sent a packet yet, or whose last packet is more than three seconds old, reports
`sensor_silent` and reads `IDLE`; that flag is live-only and is never copied
onto a replay.

## Scope

All items delivered on branch `dd/go/test-pacp-replay`.

| Item | Summary                                                                                       |
| ---- | --------------------------------------------------------------------------------------------- |
| 1    | Separate publish-stage and client-stage frame loss; stop counting a source change as drops    |
| 2    | Bound a stream send; report a stalled send instead of severing the stream                     |
| 3    | Name the stalling frame: id, type, point count, serialised size                               |
| 4    | Deliver buffered frames in capture order; confine the frame-rate throttle to replays          |
| 5    | Summarise dropped frames instead of logging each one                                          |
| 6    | Hand a newly subscribed client the current background, with correct reference counting        |
| 7    | Send a background snapshot as settling completes, on the transition alone                     |
| 8    | Report settling on `/api/lidar/data_source` and proto `PlaybackInfo`                          |
| 9    | Live/Replay segmented control; disabled while live, since a recording must be loaded first    |
| 10   | Return to live restarts the gRPC stream, symmetrically with loading a replay                  |
| 11   | A stream that ends clears the connection, not only the replay-finished flag                   |
| 12   | Inspector visibility keyed on `showSidePanel` alone                                           |
| 13   | Replace `.disabled()` with `.inert` wherever availability changes, ending the cycles          |
| 14   | Delete the `is_live` compatibility layer and the source inference it fed                      |
| 15   | Require settling reporting on the interface; drop the unread settling fraction                |
| 16   | Route the gRPC handler through normal client registration so every client gets the background |
| 17   | Clear the cached replay background when returning to live                                     |
| 18   | Report silent live input as `IDLE`, with settling taking precedence                           |
| 19   | Show replay rate controls only alongside a replay timeline                                    |

## Evidence

Two measurements pinned the diagnosis and are worth keeping.

**The client stall is client-side.** With the frame named, the blocked send was
`frame 9353 (type=1 points=39 msg=5.9KB)` — a tiny foreground frame, in live
mode, blocked for 41.9 s before resuming. This disproved the initial hypothesis
that a 2.4 MB JSON background frame was choking the decoder: frame size is
irrelevant, the client simply stops reading, and whichever frame is in flight
gets blamed. Connects and disconnects balanced (18/17, max 2 concurrent), so it
is not a leaked stream either.

**The "lost bounding boxes" were a dead stream.** The 13:42 run shows five
minutes of server uptime with **zero client connections**, while the visualiser
reported itself connected throughout. A client on a dead stream receives no
tracks, so no boxes, and whatever is on screen is the last frame before the
server went away.

## AttributeGraph cycles

Every captured cycle was rooted at `-[NSCell setEnabled:]`, which calls
`nextValidKeyView`, which makes AppKit recompute the window's key-view loop,
which re-enters SwiftUI's view graph through `NSHostingView.responderNode` —
inside the update that changed the enabled state. None of the app's own frames
appeared in the stacks: it is AppKit and SwiftUI re-entering each other, and a
control's availability changing is what starts it.

`.inert(_:hint:)` dims and stops hit-testing without touching AppKit's enabled
state. Every view that disables on changing state now uses it. Constant
`.disabled(true)` is untouched: a value that never changes cannot trigger
`setEnabled:` after the first layout, and it is the flipping that re-enters the
graph.

Measured: 24 cycles, 24 again after converting one control, then **0** across a
three-minute run streaming 800 frames. The first attempt converted the control
that had just been written rather than the ones the stack counts named, which
is why a test now fails on any view that disables on changing state.

This did not affect the stall. The watchdog showed the main thread responsive
throughout, so the cycles were real jank and log noise rather than its cause.

## The stall

Stalls of 37 to 105 seconds, on frames as small as 0.1 KB, in live and replay
alike. Both ends waited on each other: the server blocked in `Send`, the client
idle on a background thread with a responsive main thread and nothing queued
for the main actor. Frame size, replay mode, leaked streams, a wedged main
thread and the app's own frame handling were each ruled out in turn, and none
of it found the cause, because every measurement was taken inside the one
process.

A second client settled it. `make debug-grpc-probe` streams from the same
server with an unrelated HTTP/2 stack: **2000 frames, 14.6 MB, one 2.5 s gap
and no stall**. The server can push megabytes to a client that keeps reading,
so the fault was never the server's, and 11 MB delivered cleanly also disposes
of any window-exhaustion theory on that side.

The client now sets its HTTP/2 window explicitly at 16 MB rather than
inheriting grpc-swift's default. Stalls per run, against the build:

| Build       | Stalls | Window set |
| ----------- | ------ | ---------- |
| `d2bc837a9` | 7      | no         |
| `3b59fe5b1` | 11     | no         |
| `dc3c08361` | 12     | no         |
| `85e3b661f` | 30     | no         |
| `92ca155e2` | **0**  | yes        |

An hour-long soak on 2026-08-31 settles it. Four concurrent streams, each on
its own connection, took **35,987 frames and 320.5 MB apiece — 1.28 GB in
total — at a sustained 10.0 frames per second**, the sensor's own rate, with
the visualiser attached throughout.

Four gaps over a second, worst 2.144 s. Nothing resembling the 37 to 105 second
stalls that prompted this work, and no drift in throughput across the hour.

All four streams saw each gap within 5 ms of one another, which rules out
per-client flow control: independent connections do not stall in lockstep. One
gap lines up exactly with a source change, a parked replay taking live input
and resetting the grid. The other three have no logged cause — no frame
discards, no drop-rate warning, packet rate steady at 1800/s either side — so
they are a brief pause in frame production rather than anything the transport
did.

The window change is what tracks with this. Stalls per run before it: 7, 11, 12
and 30, of 37 to 105 seconds each.

### What this does not establish

Every observation in this document comes from **one machine, with client and
server on loopback**. That is the easiest case the transport will ever see: no
real network, no packet loss, no latency, and a flow-control window that never
has to cover a round trip. A 16 MB window that suffices there may not survive
a real RTT, and the fault was in the client's transport rather than anything
about this host, so nothing here is host-specific in a way that would make the
result generalise on its own.

Three environments remain unexercised, and each is tracked in the backlog:

| Environment            | Why it differs                                                         |
| ---------------------- | ---------------------------------------------------------------------- |
| Server on another host | Real RTT and loss; the window has to cover the bandwidth-delay product |
| Linux server build     | Different scheduler and network stack under the same Go server         |
| Raspberry Pi           | The deployment target: less CPU, slower memory, contended network      |

`make debug-grpc-soak` takes `ADDR`, so the same hour can be run against each
with the visualiser attached. Record the per-host numbers here rather than
treating the Mac result as the answer.

### The residual gaps

Four gaps over a second across the hour, worst 2.144 s. All four streams saw
each within 5 ms, so frame production paused rather than the transport
stalling. One lines up with a source change — a parked replay taking live
input, with the grid reset that follows — and is expected. The other three have
no logged cause: no frame discards, no drop-rate warning, packet rate steady at
1800/s either side.

They are far below what prompted this work and are not a reason to hold the
change. Attributing them needs a per-frame production timestamp on the publish
path, so a pause can be located rather than inferred; that is tracked
separately.
Longer field confirmation remains explicitly tracked in `BACKLOG.md`; it is not
presented here as hardware validation the branch did not perform.

## Simplifications taken after review

The branch was reviewed for compatibility code that would make later work
harder. Four things went:

- **`is_live`.** Only ever `source_mode == SOURCE_MODE_LIVE`, kept for a
  server/client version mismatch that cannot occur when both ship from one
  repository. It fed an inference path that reconstructed the source from
  `is_live` and `seekable` — the reconstruction `source_mode` was introduced to
  replace.
- **Optional settling reporting.** Made optional so seven test fakes would not
  need updating; the production bridge then silently did not implement it and
  the feature no-oped for a release cycle. It is required now, so the compiler
  catches what a runtime warning had been papering over.
- **`settling_progress`.** Plumbed through proto, both models, an `@Published`
  property and the HTTP API, and never displayed. Superseded by elapsed
  seconds.
- **The frame-drop bound and its instrumentation.** Added on the theory that a
  wedged main thread was starving the read loop, which the watchdog disproved
  and the probe then buried. The awaited hand-off is back: it applies natural
  back-pressure and drops nothing.

Three further candidates are tracked in the backlog rather than taken here: the
four independent frame-drop paths, the `.inert` fork's house rule, and the
legacy JSON VRLOG encoding.

## Where this stands

| Concern                        | Status                                                               |
| ------------------------------ | -------------------------------------------------------------------- |
| Transport stall (beachball)    | Resolved on macOS loopback; unverified off-host, on Linux, on the Pi |
| AttributeGraph cycles          | Resolved, 24 to 0                                                    |
| Background delivery to clients | Resolved: on subscribe, on settle, and on return to live             |
| Residual 1-2s gaps             | Understood as frame-production pauses; three of four unattributed    |
| Frame-drop path count          | Four independent paths, unconsolidated                               |

## Tracked follow-up

- **`settling_max_spread_delta` may be mis-scaled.** It is documented as a
  per-frame mean delta but evaluated every `SettlingCheckInterval` frames, so it
  measures a second of drift against a per-frame bar. Convergence currently
  fires regardless, so this is a latent tightening rather than a live fault.
- **The stall is unverified off this machine.** An hour-long four-stream soak
  on 2026-08-31 found no trace of it, but every observation here is macOS with
  client and server on loopback. Re-running against another host, the Linux
  build and a Raspberry Pi is tracked in the backlog: loopback never exercises
  a real round trip, which is exactly what a flow-control window has to cover.
- **Three of the four residual gaps have no attributed cause.** Locating them
  needs a per-frame production timestamp on the publish path, so a pause can be
  measured rather than inferred from its absence in the logs.

## Related

- [Pipeline state model](lidar-pipeline-state-model-plan.md) — the state this stream reports
- [Settling time optimisation](../lidar/operations/settling-time-optimisation.md) — Phase 4, the convergence work this surfaces
- [Data source switching](../lidar/operations/data-source-switching.md) — canonical operations reference
