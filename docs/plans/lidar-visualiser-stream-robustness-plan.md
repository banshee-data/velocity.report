# LiDAR visualiser stream robustness (v0.5.7)

- **Status:** Complete, with one open item
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

The last two were the same bug wearing different clothes, and the first is still
open. Getting there required making the stream describe its own failures, which
is most of what this plan delivers.

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
| Inspector pane          | Shown on `showSidePanel                                                                              |          | selectedTrackID != nil`, so the toggle could not close it with a track selected | Medium |

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
the source mode, and the visualiser shows `SETTLING 5.9s` in place of the source
badge — settling outranks the source deliberately, because it is the thing that
needs explaining and it resolves on its own.

## Scope

All items delivered on branch `dd/go/test-pacp-replay`.

| Item | Summary                                                                                    |
| ---- | ------------------------------------------------------------------------------------------ |
| 1    | Separate publish-stage and client-stage frame loss; stop counting a source change as drops |
| 2    | Bound a stream send; report a stalled send instead of severing the stream                  |
| 3    | Name the stalling frame: id, type, point count, serialised size                            |
| 4    | Deliver buffered frames in capture order; confine the frame-rate throttle to replays       |
| 5    | Summarise dropped frames instead of logging each one                                       |
| 6    | Hand a newly subscribed client the current background, with correct reference counting     |
| 7    | Send a background snapshot as settling completes, on the transition alone                  |
| 8    | Report settling on `/api/lidar/data_source` and proto `PlaybackInfo`                       |
| 9    | Live/Replay segmented control; disabled while live, since a recording must be loaded first |
| 10   | Return to live restarts the gRPC stream, symmetrically with loading a replay               |
| 11   | A stream that ends clears the connection, not only the replay-finished flag                |
| 12   | Inspector visibility keyed on `showSidePanel` alone                                        |
| 13   | Replace `.disabled()` with `.inert` wherever availability changes, ending the cycles       |

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

## Open items

- **Client-side hang (beachball).** A ~42 s main-actor stall in the Swift app
  that recovers unaided. Server-side evidence is conclusive about what it is
  _not_: not frame size, not replay-specific, not a leaked stream, not the
  background snapshot. Diagnosing it needs client-side instrumentation — timing
  the per-frame `await MainActor.run` hop and logging when it exceeds a
  threshold, the same play that worked server-side.
- **`settling_max_spread_delta` may be mis-scaled.** It is documented as a
  per-frame mean delta but evaluated every `SettlingCheckInterval` frames, so it
  measures a second of drift against a per-frame bar. Convergence currently
  fires regardless, so this is a latent tightening rather than a live fault.

## Related

- [Pipeline state model](lidar-pipeline-state-model-plan.md) — the state this stream reports
- [Settling time optimisation](../lidar/operations/settling-time-optimisation.md) — Phase 4, the convergence work this surfaces
- [Data source switching](../lidar/operations/data-source-switching.md) — canonical operations reference
