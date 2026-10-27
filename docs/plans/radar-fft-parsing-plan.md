# Radar FFT parsing and scene evidence plan

- **Status:** Parked: queued for v0.7.3; revisit when that release slot becomes next (D-32).
- **Decisions:** D-30 (one binary and role boundaries), D-31 (coordinated vocabulary cutovers), D-32 (planning limits), D-33 (shipped behaviour versus intent).
- **Layers:** Radar ingest, local evidence storage, operator charts, LiDAR track association.
- **Target:** v0.7.3; six independently deliverable items in [BACKLOG.md](../BACKLOG.md).
- **Canonical:** [RADAR_FFT_FORMAT.md](../../data/structures/RADAR_FFT_FORMAT.md)
- **Related:** [Exploration](../radar/operations/fft-stream-exploration.md), [serial classification](radar-serial-line-classification-and-command-response-plan.md), [L7 scene](lidar-l7-scene-plan.md).

This plan turns OPS243-A FFT output into replayable motion evidence and compares
it with positioned LiDAR tracks. It proposes implementation; this documentation
change enables no sensor mode and changes no runtime behaviour.

## Current state and motivation

The current serial classifier recognises object summaries and scalar speed or
magnitude messages. A plain FFT object reaches configuration handling, which keeps
its latest value in memory instead of recording a spectral history. The serial
multiplexer has one-message subscriber buffers and a silent drop path.

Gerardus provides vector geometry and registered sensor poses outside this
repository. Its layers can be drawn by the operator clients, but accepted poses
do not yet populate velocity.report's own recording frame. A scene marker or
identity CoordinateFrameInfo is not an adequate calibration for radar fusion.

The FFT can expose motion components beyond the strongest reported speed. It
cannot supply target position: the OPS243-A has no range or bearing measurement.
Keep components, candidate associations, and vehicle tracks distinct.

## Design

Acquire once, classify structurally, sequence before fan-out, and record immutable
evidence before display consumers can drop it. Decode against immutable device
configuration and clock epochs. Derive peaks and temporal components separately.
Compare those with track radial velocities in a qualified site frame.

Reuse the serial plan's shared classifier if delivered; otherwise land the minimum
shared typed-line prerequisite in Item 1 and update that plan's outstanding scope.
Do not build another serial reader or duplicate the command-response project.
FFT recording uses the same source authority as ordinary radar ingest.

## Item 1: typed parser and calibration

**Outcome:** validated FFT and I/Q messages become typed source evidence.

- Add FFT, ADC, config, speed, object, and rejected-message classes, inspecting
  keys and array shapes rather than substrings.
- Capture product, firmware, sample rate, sample count, FFT size, bin layout,
  magnitude semantics, and direction calibration in configuration epochs.
- Establish framing and bounded message sizes against actual OF output. Preserve
  unsupported payloads with a reason; never silently guess the layout.
- Pair separate I/Q buffers only across verified continuous ordering.
- Reconcile command-catalogue case and enable/disable descriptions with the
  manufacturer's API before exposing FFT control.

**Acceptance:** fixtures exercise partial reads, interleaved replies, both Doppler
signs, unsupported layouts, malformed arrays, oversized messages, configuration
changes, and broken I/Q pairing. Unknown calibration produces bins only.

## Item 2: recording, loss accounting, and replay

**Outcome:** a captured spectrum sequence can be replayed with its original gaps.

- Add immutable chunks and a SQLite index following the canonical contract.
- Record acquisition and arrival timing separately. Add source sequencing before
  fan-out, explicit queue/storage failure accounting, and completeness status.
- Recover interrupted writes; retain configuration, clock, placement, and source
  digests. Apply an explicit retention policy.
- Measure backpressure and sustainable cadence instead of treating a lossy display
  subscription as the archival recorder.

**Acceptance:** replay preserves values, order, epochs and known gaps; queue
overflow, reconnect, storage failure and truncated final chunks are exercised.
A device without a counter retains unknown upstream completeness.

## Item 3: spectral components

**Outcome:** versioned lobe and temporal-component records retain ambiguity.

- Estimate noise, group adjacent bins, describe lobe width and prominence, and
  track persistence through time with explicit gap boundaries.
- Keep opposite-frequency candidates independently, with no mirrored-peak winner.
- Retain crossings, merged components and unassigned evidence. Never equate peak
  count with vehicle count or infer acceleration solely from Doppler slope.

**Acceptance:** controlled fixtures cover a single broad lobe, separated targets,
opposing equal-speed targets, same-speed merging, sidelobes, crossings, and gaps.
Report resolvability limits independently of bin spacing.

## Item 4: spectrum and waterfall inspection

**Outcome:** local operators can inspect captured spectra and their provenance.

- Add spectrum and time–radial-speed views with grouped components, firmware speed
  reports, acquisition/arrival timing, configuration changes, and missing intervals.
- Make seek and replay deterministic; label quantity and normalisation explicitly.
- Keep unknown speed calibration on a bin axis and unknown direction unlabelled.

**Acceptance:** views agree with replay values and gaps, bound memory on long
captures, and retain the same selected instant when synchronised with a scene.

## Item 5: scene associations and positional overlays

**Outcome:** operators can compare radar components with qualified LiDAR estimates.

- Supply explicit radar placement, beam geometry, LiDAR-to-site transform and clock
  mapping; freeze their revisions with each association run.
- Project each track's velocity onto its radar line of sight and gate by timing,
  geometry and uncertainty. Retain all plausible candidates.
- Display the beam, positioned associations, residuals and unresolved components.
  Keep coasting age and increasing positional uncertainty visible.
- Suppress positional matching when calibration or timing is unavailable.

**Acceptance:** isolated and ambiguous multi-track scenes demonstrate correct
projection, refusal, candidate retention, and bounded coasting. Accepted
associations stay an evidence overlay; state-estimator feedback is separate work.
No point-level radar fusion or new L7 canonical-object implementation is implied.

## Item 6: field qualification and release decision

**Outcome:** a documented Pi 4 gate decides the supported FFT capture mode.

- Freeze a capture protocol and parameter sets before running comparisons.
- Capture isolated passes, opposing traffic, similar-speed followers, occlusion,
  and near-transverse motion with independent truth where available.
- Measure actual UART/USB throughput, device report cadence, losses, latency,
  clock uncertainty, CPU, memory, disk growth, and ordinary radar ingest impact.
- Compare component separability and association ambiguity against the existing
  scalar radar and LiDAR-only baselines. Report physical error only with adequate
  independent reference; otherwise mark the result insufficient evidence.

**Acceptance:** publish per-case findings, failure cases, provenance, and the
enable/experimental/reject decision. Fix numerical promotion thresholds before
collection; do not choose them after inspecting results. No default-on fusion,
vehicle counting, or speed-accuracy claim follows from an unlabelled waterfall.

## Dependencies and boundaries

Item 2 follows Item 1; Item 3 follows replayable evidence; Item 4 reads Items 2–3;
Item 5 requires Item 4 plus qualified pose and clock mappings. Item 6 gates release
claims across all previous items. Missing Gerardus pose persistence does not block
radar-only inspection: an explicit versioned alignment sidecar can qualify a local
comparison without claiming integration already ships.

Preserve existing scalar transits, report populations, and command-response
boundaries. Shared capture/deployment vocabulary follows its owning plans.
Hardware range upgrades, moving-radar ego compensation, public trajectory
exports, and closed-loop tracker fusion are outside this release.

## Risks and open choices

| Risk or choice                   | Required disposition                                                           |
| -------------------------------- | ------------------------------------------------------------------------------ |
| UART saturation                  | Qualify actual transport; USB or lower capture cadence if necessary            |
| Silent loss                      | Sequence before consumers and distinguish known loss from unknown completeness |
| Firmware layout/sign differences | Freeze captures and compatibility fixtures before interpreting bins            |
| Equal radial speeds              | Retain unresolved mixtures and competing track associations                    |
| Clock or pose error              | Suppress matching outside measured uncertainty limits                          |
| Storage growth                   | Measure chunk encoding and retention before choosing defaults                  |
| Calibration copied from sample   | Require observed settings and configuration epochs                             |

## Checklist

- [ ] Item 1: typed parser and calibration (`M`).
- [ ] Item 2: recording, loss accounting, and replay (`M`).
- [ ] Item 3: spectral components (`M`).
- [ ] Item 4: spectrum and waterfall inspection (`M`).
- [ ] Item 5: scene associations and positional overlays (`L`).
- [ ] Item 6: field qualification and release decision (`M`).
