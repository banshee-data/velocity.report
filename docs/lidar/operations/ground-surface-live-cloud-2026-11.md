# Surface-relative ground: tuning, live wiring and refit lifecycle

- **Status:** Implementation; default promotion remains open
- **Layers:** L3 background, L4 clipping
- **Related:** [ground0002](../../../data/experiments/ground/ground0002.md), [state plan](../../plans/lidar-state-estimation-plan.md)

The P11 surface clip was available only to replay flags and kept its first fit
indefinitely. The optional `surface_ground` tuning block now names the same
floor, ceiling and regional cell size for startup, runtime patches and replay.
It remains absent by default, preserving the shipped height band and fingerprint.

## Behaviour and invariants

All four block keys are required. Bounds are finite, ordered and non-negative;
zero floor is used literally and zero cell size selects a global plane. The
three registered arms use 10 m, global and 5 m cells with 0.2–4.5 m bounds.
Replay aliases and file settings must agree. `remove_ground` stays authoritative.

A monotonic background revision covers cell updates, parameter changes, restored
cells, absorbed returns, ring elevations and grid reset; snapshot persistence
cannot rewind it. All enabled consumers, including tracker/report-only fits, use the same reset and bounded
refresh lifecycle. Background-driven surface refits and failed-fit retries coalesce at one capture-time second. Runtime
region changes and re-enabling refit immediately. Reset/unsettled input and failed
refits invalidate the old model and its published pointer. The ordinary height
band applies while a fit is unavailable. An empty frame can invalidate a reset
model; no observation is invented. Refit failures are retried at bounded cadence.

Runtime patches validate before publishing one immutable block atomically. The
frame callback snapshots initial configuration and reads the runtime block once
per frame. L3-only profiles do not fit or apply an L4 surface.

## Validation and limits

Focused tests cover finite/ordered bounds, omitted-default fingerprints,
immutable accessors, runtime validation before publication, file/alias/flag
agreement and refusal, explicit zero floor, bounded changing-background refits,
disable/re-enable, reset on empty input, failed-fit recovery and capture rewind.
Existing tilted-plane and height-filter tests remain the geometric oracle.

This implementation is not evidence that surface clipping improves the tracker.
No ground0002 acceptance result or changed default is claimed. Its corpus,
reviewed geometry/identity score and Pi frame-time evidence remain outstanding.
The engineering tests use synthetic backgrounds and capture timestamps; they do
not validate a physical mounting calibration or cross-site recall.

## Engineering verification

The full Go suite and Go/documentation lint passed on Linux with Go 1.27.1.
The Makefile native build reported `0.5.1-pre50` and source commit
`355fc8aa7186a44baa0c537ed86a19858a9da747`. Runtime and replay tests load the
registered files, verify exact alias agreement and refuse conflicting settings.
No scientific surface-clip comparison was run, and no Pi result is claimed.

Registered configuration file SHA-256 values at that implementation:

- `config/experiments/surface-ground-10m.json`: `8dab80bfd3aa60af4e0ccbd64f374063e613e6ea0e7827f0db7e1e9968c3441e`.
- `config/experiments/surface-ground-global.json`: `518b304cfe6508bcc25d7af689b9b23dd3f7c8fea468200374dbb4febc969541`.
- `config/experiments/surface-ground-5m.json`: `9ec2f6d4dd9c28777bea62c81d0df3967c56ff096730cad96ba259fc938a6fc3`.
