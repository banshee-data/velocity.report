# Qualification perimeter: raw-capture loader audit

- **Status:** partial implementation; derived-data and dashboard perimeter remains open
- **Scope:** v0.5.4 eligibility, based on main `41c7ccf3`

## Changed boundaries

| Boundary                                              | Admission point                                          | Regression evidence                                                                      |
| ----------------------------------------------------- | -------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `lidar-box-eval`                                      | All comma-separated captures, before row files or replay | Renamed qualification input and an unknown second capture both refuse before output      |
| `velocity lidar settling-eval` and standalone wrapper | Before UDP auto-detection                                | Qualification, validation, unknown, renamed, missing policy and frozen held-out fixtures |
| `lidar-bench`                                         | Before UDP auto-detection and benchmark engine           | Same bypass matrix; existing admitted fixture benchmarks retain their output checks      |
| Job-worker benchmark executor                         | Before tuning and engine invocation                      | Direct executor call refuses qualification independently of CLI checks                   |
| `lidar-fg-diag replay`                                | Before directory creation, oracle loading or replay      | Renamed qualification fixture refuses before output                                      |

Every raw capture is hashed using the existing shared guard. Requests cover the whole capture:
warm-up and pre-roll expose data too. No caller-supplied site, purpose, lineage or run ID grants
admission. Manifest, frozen-split registry and persisted use ledger remain mandatory. Build
version/SHA and entry point are recorded. Help and box-variant listing need no data admission.
Production capture, ordinary replay and publication code paths are unchanged.

**Breaking for offline development tools:** the four newly guarded commands and worker benchmark
now require `VELOCITY_COHORT_MANIFEST`, `VELOCITY_ANNOTATION_ROOT` and
`VELOCITY_ELIGIBILITY_DB` (or the existing CLI flag equivalents). This includes `make test-perf`,
`perf-baseline` and `run-settling-eval` invocations and nightly jobs using those commands. Provision
the actual policy/registry/ledger before running them; no CI exemption or empty-registry fallback
is added. The tests' synthetic registries apply only to fixtures.

## Overlap and remaining implementation

The existing baseline/refinement evaluators, sweeps, auto-tuning, HINT and experimental replay
already call the guard; they were not given a second guard. The following boundaries still need
implementation, **not merely operator approval or a capture download**:

| Remaining boundary                                                                             | Required binding before reading samples                                                                                     |
| ---------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| Near-face evaluator; ground-truth evaluator and its perframe/fate/retention/bundle subcommands | Bind every reference pack and candidate arm to its exact capture/source and frozen split; preserve held-out score semantics |
| Track scorecard, including its optional reference database                                     | Independently bind both databases and all selected sources                                                                  |
| Evidence oracle                                                                                | Verify source manifest against actual input provenance and guarded capture identities                                       |
| Closeness audit                                                                                | Bind each selected background snapshot to capture/session; a sensor name alone cannot distinguish sessions                  |
| `vrlog-analyse` and `lidar-fg-diag vrlog`                                                      | Bind each recording and any cached report to capture provenance; a filename or supplied development PCAP is insufficient    |
| `lidar-fg-diag replay -oracle`                                                                 | The raw capture is guarded; independent oracle provenance is not yet bound                                                  |
| Tracks, Runs and scene diagnostic routes                                                       | Server-side eligibility before samples/track results leave the API; preserve ordinary production publication                |
| Historical tools staged by job workers                                                         | Old binaries can predate guard calls; a worker-wide policy boundary is still needed beyond the in-process benchmark fix     |

Do not close the plan's perimeter acceptance checkbox on this PR. Direct shared-engine calls are
not a storage security boundary. The manifest's access-control claims still cannot authorise
qualification scoring; real storage controls and custodian authentication remain absent.

## Verification

The synthetic fixtures contain no reserved data. Rejection tests deliberately use non-packet
bytes and assert an eligibility error rather than a decoder error, plus absence of output.
The box test invokes `main` in a subprocess, and the worker test invokes its executor directly.
The held-out fixture freezes a real synthetic annotation split, then invokes the evaluator
without naming that split. The original production/unguarded-purpose guard tests remain intact.

### Cloud validation outcome

- Passed the affected packages with `go test -tags pcap`: command router, eligibility,
  worker runner, server, box evaluator and foreground diagnostic.
- Passed the full default Go suite through `make test`; its Python phase then failed with
  six failures and 472 passes. The same six worker-hygiene/podcast failures reproduced from
  unchanged `origin/main` source copies. Worker hygiene encounters a missing `/dev/mqueue`
  mount entry; podcast publication fixtures report temporary destinations as Git checkouts.
- `make lint` passed Go/web checks and Markdown formatting, then stopped on two existing
  American spellings in `lidar-survey-lane-exports-plan.md`, also present on `origin/main`.
  The changed audit, handoff and qualification plan pass the spelling check.
- `make build-radar-local` succeeded. Invoked with multicall name `velocity`, the built
  `lidar settling-eval` command rejected an authored non-packet input with an explicit
  eligibility error and created no report. The legacy `velocity-report-local` name does not
  dispatch the `lidar` namespace; that naming refusal is not guard evidence.
- `make check-no-results`, `make check-devlog` and `git diff --check` passed. Raw logs and
  generated outputs remain outside the committed change.
