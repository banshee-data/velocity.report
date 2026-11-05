# Trail support and capture-time correctness in HTTP and web scenes

- **Status:** Implemented cloud slice; native publication and visual acceptance pending
- **Scope:** Backlog item 2, HTTP API and shared web scene player

The shared scene player previously drew a trail through the filter position while placing
its box at the separately published body centre. Its live adapter grouped RFC3339 timestamps
with `Date.parse`, losing sub-millisecond precision despite promising exact frame grouping.
One continuous line also bridged missing samples and mixed estimate revisions.

## Contract and behaviour

Observation endpoints retain the existing RFC3339 acquisition timestamp and add decimal-text
`timestamp_ns` and `frame_timestamp_ns`. The latter is the sensor rotation time and groups
observations into frames; older rows fall back to acquisition time. JavaScript uses `BigInt`
for absolute time and converts only the relative offset to microseconds, including fractions.
The older-server RFC3339 fallback preserves all nine fractional digits. Malformed exact
metadata is refused rather than silently downgraded to millisecond time.

Live history points add decimal-text nanoseconds, the recorded support token and the explicit
`filter_state` anchor. Missing support remains the empty token. Database observation responses
mark rows observed only when measurement provenance was recorded; legacy rows stay unknown.
These endpoints do not manufacture coast rows or final trajectory estimates.

The shared renderer uses a complete published `bx, by` pair for both box and trail. Otherwise
both use the recorded position. Every playback or seek reconstructs disconnected edges from
the same history window. Missing tracks, non-increasing capture time, gaps over 500 ms,
invalid positions and building-mask crossings cannot connect two samples. Source, anchor,
estimate-stage or revision changes replace older geometry for that track. Current-frame track
IDs and part-bounded reader history prevent expired IDs and earlier recordings from leaking
into a new view. Reacquisition can reconnect only adjacent, same-estimate samples within the
gap bound; no extrapolated point is introduced.

Observed support is solid in the track colour. Predicted/coasted edges are dashed amber;
unrecorded support is dashed grey. Explicit provisional and final observed estimates are cyan
and purple respectively. Prediction takes precedence over estimate stage. Unknown estimate
stage is never relabelled final. Tooltip text on the shared trails control explains the styles.
Edge buffers are bounded and batched by style, with independent line pairs preventing false
joins even when styles share one geometry.

## Validation and limits

Regression tests exercise nanosecond-separated rotations, separate acquisition/rotation times,
legacy and malformed timestamp metadata, missing support, complete and incomplete box anchors,
missing IDs, gaps, backward time, revision/stage/source changes, building crossings, duplicate
samples, seek reconstruction, dashed materials, visibility and scene resource disposal.

The HTTP history test verifies observed, coasted and unrecorded tokens and exact JSON string
encoding. The observation helper tests distinguish recorded measurement provenance from legacy
rows. Shared reader tests already enforce part boundaries, and the live adapter is tested
through the real reader rather than a second timeline implementation.

The shared public scene suite passed all 297 tests. The live-adapter suite passed all 24
tests, Svelte checking reported zero errors and warnings, and targeted ESLint and documentation
lint passed.

This is engineering validation, not evidence of improved physical tracking accuracy or a
qualified visual result. gRPC/protobuf/Swift support publication, macOS rendering and Xcode
validation remain open. The API does not yet select persisted versioned final trajectories;
explicit scene metadata is honoured when supplied, not inferred from a track quality score.
No tracker parameter, association, smoothing or expiry default changed.
