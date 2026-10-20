# Decision audit

- **Status:** Living record; refreshed by release preparation. First audit October 19, 2026; states decided October 20, 2026

The state of each decision in the [decisions register](../../DECISIONS.md): whether the code and
the backlog still keep it, and what was done where they do not. The register records what was
decided and, in each entry's `Kept by:` line, what would fail if the decision were broken. This
audit records whether it currently holds. It is kept apart from the register so that the register
can keep its cadence of one or two edits a sprint (D-32).

## States

| State          | Meaning                                                                                                    |
| -------------- | ---------------------------------------------------------------------------------------------------------- |
| Kept           | The code and the backlog agree with the entry                                                              |
| Drifted        | They disagree; the fix names a backlog item                                                                |
| Partly drifted | Part of the entry holds and part does not                                                                  |
| Overtaken      | Delivered work went past the entry; the entry is restated                                                  |
| Unbuilt        | Decided and scheduled, not yet built; a new entry is Unbuilt until the check in its `Kept by:` line exists |
| Pending        | Decided for a later release, as scheduled                                                                  |
| Superseded     | Replaced by a later entry, which it names                                                                  |

## Audit, October 20, 2026

The first audit was taken on October 19, 2026 in the [architecture
review](../../plans/platform-architecture-review-plan.md#p5-decisions-that-the-code-does-not-keep).
Ruth's pass on October 20 decided the states below and restated seven entries.

| Decision | Found          | Evidence                                                                                                                  | Action, October 20                                                      | State now                                   |
| -------- | -------------- | ------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------- |
| D-01     | Kept           | No fused transit table                                                                                                    | Kept-by line added                                                      | Kept                                        |
| D-02     | Kept           | No FFT ingest                                                                                                             | Kept-by line added                                                      | Kept                                        |
| D-03     | Unbuilt        | Scheduled v0.8.0                                                                                                          | Kept-by line added                                                      | Unbuilt                                     |
| D-04     | Drifted        | The entry said "v0.6 cycle"; the backlog places #391 in v0.5.7                                                            | Restated without a milestone                                            | Kept                                        |
| D-05     | Kept           | P1 v0.5.7, P2 and P4 v1.0, P3 v2.0                                                                                        | Kept-by line added                                                      | Kept                                        |
| D-07     | Overtaken      | The plan's Phase 9 is profile comparison; Swift-native labelling shipped through the annotation window (#656)             | Restated as the profile comparison UI; duplicate backlog items to merge | Pending                                     |
| D-08     | Kept           | `typst_embed` is in the default build tags                                                                                | Kept-by line added                                                      | Kept                                        |
| D-09     | Kept           | One product binary; the closure test that would keep it is planned                                                        | Kept-by line added                                                      | Kept                                        |
| D-11     | Pending        | go-echarts still in `go.mod`; report charts are Go SVG as decided                                                         | Kept-by line added                                                      | Pending                                     |
| D-14     | Drifted        | Plot, stats and API wrappers, `--lidar-listen` and the foreground-forward flags remain; Make targets grew from 118 to 231 | Restated without "before v0.6.0"; the Make trim is Project F            | Drifted                                     |
| D-16     | Drifted        | The entry said "v0.8 placement"; the backlog places it in v1.0, after posted speed limits                                 | Restated: it follows the posted-limit record                            | Kept                                        |
| D-17     | Drifted        | The entry said `grid-heatmap` moved to a Go subcommand; none exists and the Python tool still runs from Make              | Restated: `grid-heatmap` is retired                                     | Drifted until the tool is deleted           |
| D-18     | Partly drifted | Per-track p50, p85 and p95 speed features in `l6objects/classification.go` and `features.go`                              | Kept-by line added                                                      | Partly drifted                              |
| D-19     | Kept           | #352                                                                                                                      | Kept-by line added                                                      | Kept                                        |
| D-20     | Drifted        | Nineteen survivors, listed in the [compatibility register](compatibility-register.md)                                     | Restated with the consumer test and tombstones                          | Drifted until the register's v0.5.8 rows go |
| D-21     | Kept           | `include_debug` in the protocol; the emitter's gate not traced                                                            | Kept-by line added                                                      | Kept, weakly verified                       |
| D-22     | Overtaken      | `velocity device upgrade` reads `release.json` through `internal/ctl` and installs in place                               | Restated as operator-run upgrades                                       | Kept                                        |
| D-23     | Unbuilt        | No `pkg/` directory, no code, no second consumer                                                                          | Superseded by D-35                                                      | Superseded                                  |
| D-24     | Kept           | No per-track speed percentile column in the schema                                                                        | Kept-by line added                                                      | Kept                                        |
| D-25     | Partly drifted | The drift check runs in two skills, not CI; personas run 117 to 286 non-empty lines against "about 40 to 80"              | Kept-by line added; the length bound is an open question                | Partly drifted                              |
| D-26     | Kept           | `verify-static-elf.sh` and the static build workflow                                                                      | Kept-by line added                                                      | Kept                                        |
| D-27     | Unbuilt        | The content tag hashes the size and the first chunk; no whole-file digest yet                                             | Kept-by line added                                                      | Unbuilt                                     |
| D-30     | new            | The closure test is planned for v0.5.8                                                                                    | Entered                                                                 | Unbuilt                                     |
| D-31     | new            | The register test is planned for v0.5.8                                                                                   | Entered                                                                 | Unbuilt                                     |
| D-32     | new            | Active plans counted by `report-plan-hygiene`; 18 active against a limit of twelve (11 `Active`, 7 `In progress`)         | Entered                                                                 | Drifted until the parking pass              |
| D-33     | new            | Twelve visualiser docs carry a "Source plan" header; those describing unbuilt features are to be removed                  | Entered                                                                 | Drifted until the stubs go                  |
| D-34     | new            | Scene assets are in git and copied into the image                                                                         | Entered                                                                 | Unbuilt                                     |
| D-35     | new            | The v0.8.0 incubation item moves to deferred                                                                              | Entered                                                                 | Kept                                        |

Not audited: D-06, D-10, D-12, D-13, D-15, D-28 and D-29. The first release audit covers them.

## How the audit runs

At each release, `/release-prep` re-reads every entry against its `Kept by:` line, records the
state here, and refuses its go decision when a state has changed without a register edit or a
backlog item. `/weekly-retro` takes the entries kept only by review in turn, a few a week. A state
of Drifted always names the backlog item that fixes it.
