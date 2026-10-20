# Decision keeping and the compatibility register (v0.5.8)

- **Status:** Proposed. Expands option b of P5 in the [architecture review](platform-architecture-review-plan.md#p5-decisions-that-the-code-does-not-keep)
- **Layers:** Cross-cutting (decision register, plan template, review skills, Go route and flag tables, Make, CI)
- **Target:** v0.5.8
- **Decisions:** D-20 (no compatibility shims), D-14, D-17, D-25, and every decision through its Kept-by line
- **Companion plans:** [platform-simplification-and-deprecation-plan](platform-simplification-and-deprecation-plan.md) (Project F, the Make trim), [platform-vocabulary-cutovers-plan](platform-vocabulary-cutovers-plan.md), [platform-planning-limits-plan](platform-planning-limits-plan.md)
- **Canonical:** [v050-release-migration.md](../platform/operations/v050-release-migration.md)

Make every recorded decision name what keeps it, carry a state, and be cited by the plans that
rely on it; and make D-20 checkable, with a register of the old names that may still survive and
a test that removes them on time.

## Motivation

Of the 22 decisions the review checked, 9 are kept and 13 are drifted, overtaken, unbuilt or
pending, and nothing noticed: no check, skill or template asks whether a change keeps or breaks
a decision. The plainest case is D-20, which forbids temporary compatibility shims, while nineteen
old names and readers survive and a programme plan proposes eighteen more without citing it. A
decision that nothing re-reads is read as the present by the next contributor and is wrong.

## Current state

- [DECISIONS.md](../DECISIONS.md) records 29 decisions as a heading and one paragraph each. It has no state and no evidence line, and is edited once or twice a sprint.
- The plan template has no field for the decisions a plan relies on or would supersede; `/plan-review` reads DECISIONS.md but has no rule for a plan that contradicts an uncited decision.
- D-20's record ([v050-release-migration.md](../platform/operations/v050-release-migration.md)) lists the items retained in v0.5.0 and nothing since.
- The review's register lists nineteen surviving names: two allowed by D-20 by name, three necessary under the consumer test, and fourteen that no exception covers.

## Findings

| Area                | Current state                                                         | Severity | Release view |
| ------------------- | --------------------------------------------------------------------- | -------- | ------------ |
| Decision evidence   | No decision names what keeps it                                       | High     | v0.5.8       |
| Decision state      | Drifted and overtaken decisions read as current                       | High     | v0.5.8       |
| Plans and decisions | Plans need not cite decisions; one proposes eighteen shims under D-20 | High     | v0.5.8       |
| D-20                | Nineteen survivors; no register, no expiry, no test                   | Medium   | v0.5.8       |
| Audit cadence       | None                                                                  | Medium   | v0.5.8       |

## Design / approach

### Every decision names what keeps it

Each entry in DECISIONS.md gains two lines under its paragraph:

| Line       | Holds                                                                                                                                                                               |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `State:`   | Kept, Drifted (naming the backlog item that fixes it), Partly drifted, Overtaken (restated in the next edit), Unbuilt (naming its release), Pending, Superseded by D-NN, or Retired |
| `Kept by:` | The check, test or artefact that fails when the decision is broken, or the skill that reviews it when only review can                                                               |

A decision that can name neither a check nor a reviewing skill is a preference, and belongs in
the vision or a hub doc. Nothing is deleted from the register: a retired decision keeps its entry
with its state.

### Plans cite decisions

The plan template gains a `- **Decisions:**` metadata line listing the decisions the plan relies
on or proposes to supersede. `/plan-review` checks the plan against the decisions its subject
touches and refuses one that contradicts a decision it does not cite. A plan that needs to break
a decision says so and proposes the superseding decision; it does not proceed on silence.

### The audit runs at release

`/release-prep` re-runs the audit table (decision, state, evidence, fix) and fails its go
decision on a decision whose state has changed without a register edit. `/weekly-retro` takes the
review-only decisions in turn, a few a week.

### D-20: the consumer test and the register

An old name, field or format survives only while something the release does not replace depends
on it. The review sets out the test and the six treatments; this plan builds the mechanism.

| Piece                | What it is                                                                                                                                                           |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| The register         | A table in D-20's record: one row per surviving name, with where it lives, the consumer the release does not replace, the treatment, and the release that removes it |
| Route aliases        | Declared in the LiDAR route table with the register row they belong to; a test fails on an alias route without a row                                                 |
| Flag tombstones      | One helper that registers an old flag name as a refusal printing the new one; a test lists them against the register                                                 |
| Make tombstones      | One pattern rule for renamed targets, and the Make check that refuses alias recipes (Project F)                                                                      |
| The expiry test      | Reads the register and fails when the Makefile's `VERSION` reaches a row's removal release                                                                           |
| Dual readers in code | Caught in review: `/review-pr` reads the register and refuses a new old-format reader or projection without a row                                                    |

### The first register

The review's nineteen rows, with treatments:

| Kind                              | Rows                                                                                                                                                                    | Treatment                                                  |
| --------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| Allowed by D-20 by name           | Database legacy detection; the type aliases that break import cycles                                                                                                    | Keep, registered with no removal release                   |
| Necessary under the consumer test | The `velocity-report` program name (the systemd unit on flashed devices); the replay stop aliases the macOS app calls; `config-migrate` for operators' files            | Keep, registered with a removal release                    |
| Release unit, no exception        | `/api/lidar/monitor`, `/tracks/history`, the `-tuning` flag, the tuning client's polling fallback, `MakeLegacyParamSet`, the root alias package, seventeen Make aliases | Delete in v0.5.8, tombstones where a person types the name |
| Data at rest                      | The version 1 pack reader (all packs migrated), the `legacy_centroid_v0` fallback, legacy JSON VRLOG frames, the macOS `split` and `merge` tag labels                   | Migrate or convert, then delete the reader                 |
| Renamed by a cutover              | The `pcap_file` projection and the `pcap` wire value                                                                                                                    | Removed in the clip cutover                                |
| Not yet a shim                    | The recorder's `TuningHash`, still read by scene export and the analysis report                                                                                         | Decide: un-deprecate, or move readers to the fingerprint   |

## Scope

### Item 1: the register's states and Kept-by lines

**Summary:** the states and Kept-by lines entered for every decision in one register edit, with the restatements Ruth decided.

**Milestone:** v0.5.8 (this branch carries the first edit)

### Item 2: plans cite decisions

**Summary:** the `Decisions` line in the plan template and in every active plan touched after this; `/plan-review` refuses an uncited contradiction.

**Steps:**

1. Template and the plan hygiene docs gain the line.
2. `/plan-review` gains the check; `/weekly-retro` lists active plans without the line.
3. Active plans gain the line when next edited; no bulk edit.

**Milestone:** v0.5.8

### Item 3: the release audit

**Summary:** `/release-prep` re-runs the audit table; `/weekly-retro` reviews the review-only decisions in rotation.

**Milestone:** v0.5.8

### Item 4: the compatibility register and its tests

**Summary:** the register in D-20's record; route aliases and flag tombstones declared through it; the expiry test; `/review-pr` reads it.

**Steps:**

1. Move the review's register into the record, with removal releases.
2. Route table: an alias names its row; a test fails on an alias without one.
3. Flag helper for tombstones; `-tuning` becomes the first.
4. The expiry test reads the register and the Makefile `VERSION`.
5. `/review-pr` gains the register check.

**Milestone:** v0.5.8

### Item 5: remove what no exception covers

**Summary:** the release-unit rows deleted; the macOS app and in-repo scripts moved to `/replay/stop`; the data-at-rest rows migrated or converted.

**Steps:**

1. Delete `/api/lidar/monitor` and `/tracks/history` with their one web and two script callers.
2. Delete the polling fallback, `MakeLegacyParamSet`, the version 1 pack reader.
3. A backfill migration for `measurement_source`, then delete the fallback.
4. Convert the three JSON VRLOG recordings, verify, delete the decoder.
5. Switch the macOS app and the scene publisher to `/replay/stop`; the two aliases go in the release after the next DMG.

**Milestone:** v0.5.8, except the aliases, which follow the DMG

## Dependencies

- Item 1 is Ruth's register edit and needs nothing else.
- Item 4's Make part rides on Project F of the simplification plan.
- Item 5's `pcap` rows wait for the clip cutover.

## Risks

| Risk                                             | Likelihood | Impact | Mitigation                                                                                             |
| ------------------------------------------------ | ---------- | ------ | ------------------------------------------------------------------------------------------------------ |
| Kept-by lines become boilerplate                 | Medium     | Medium | A line must name a file, a target or a skill; the audit reads them                                     |
| The register misses a shim that lives in code    | Medium     | Low    | `/review-pr` reads it; the expiry test cannot see what is not registered, so review stays the backstop |
| Deleting an alias breaks an operator's script    | Low        | Low    | Tombstones print the new name for a release                                                            |
| An older macOS app breaks against a newer server | Medium     | Medium | Aliases the app calls stay until the next DMG is the oldest supported                                  |

## Checklist

### Complete

- [x] Audit of 22 decisions and the first register (the review's P5)

### Outstanding

- [ ] Item 1: states and Kept-by lines in DECISIONS.md (`S`)
- [ ] Item 2: plans cite decisions; `/plan-review` check (`S`)
- [ ] Item 3: release audit in `/release-prep` (`S`)
- [ ] Item 4: compatibility register and its tests (`S`)
- [ ] Item 5: remove the shims no exception covers (`M`)

### Deferred

- [ ] A machine-readable decision register: only if the Markdown lines prove too loose to audit

### Accepted residuals (no action planned)

- [ ] Seven decisions (D-06, D-10, D-12, D-13, D-15, D-28, D-29) were not checked in the first audit; the first release audit covers them
