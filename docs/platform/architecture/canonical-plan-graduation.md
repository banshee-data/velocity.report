# Canonical plan graduation

How [docs/plans/](../../plans) relates to the hub documentation tree, and the lifecycle by which
plan files graduate into stable hub docs.

## Problem

One body of work can spread across two or more plan files, a plan plus a hub
doc, or multiple phase plans that all describe the same architectural identity.
That makes the project hard to reason about because readers must reconstruct
the real source of truth by hand.

## Design goal

For any substantial body of work, a reader should be able to answer three
questions quickly:

1. What is the stable doc that defines this thing?
2. What plan, if any, is currently driving execution?
3. What happened to the old plan URLs?

## Document roles

| Role                 | Purpose                                              | Path                                  |
| -------------------- | ---------------------------------------------------- | ------------------------------------- |
| Canonical hub doc    | Stable architecture, implementation record, or model | Hub paths below                       |
| Active plan          | Temporary sequencing, phases, checklists             | `docs/plans/*.md`                     |
| Graduated plan alias | Preserve legacy URLs after consolidation             | Symlink in [docs/plans/](../../plans) |

## Hub structure

Four mutually exclusive hubs, chosen by domain-first sorting:

| Hub                        | Scope                                   |
| -------------------------- | --------------------------------------- |
| [docs/lidar/](../../lidar) | LiDAR pipeline, clustering, QC          |
| [docs/radar/](../../radar) | Radar pipeline, time-series             |
| [docs/ui/](../../ui)       | Web frontend, macOS app, PDF generation |
| [docs/platform/](..)       | Go packages, deploy, DB, tooling        |

Additional prefixes `config/` and `data/` are allowed for docs that live beside
the artefacts they describe.

### Sorting test

To place a doc, ask: _which domain owns the lasting value?_

| If the lasting value is...                            | Canonical home                  |
| ----------------------------------------------------- | ------------------------------- |
| Enduring design, model, API contract, system boundary | `<hub>/architecture/<topic>.md` |
| Operating guidance, migration, implementation status  | `<hub>/operations/<topic>.md`   |
| UI architecture or implementation reference           | `docs/ui/<topic>.md`            |
| Config or maths specification already in `config/`    | Existing `config/` or `data/`   |

## One body of work, one canonical doc

Use this test when deciding whether multiple plans should merge:

1. Do they change the same owned system or surface?
2. Do they share the same long-lived architecture or operating model?
3. Would a single reader expect one stable place to understand the whole thing?

If yes to all three, they collapse into one canonical hub doc and at most one
active plan.

## Statuses and limits (D-32)

[D-32](../../DECISIONS.md#d-32--planning-limits-and-backlog-governance) (October 20, 2026) closes
the status vocabulary and bounds the work in progress. The limits are decided; the lint that checks
them is planned, and `make report-plan-hygiene` counts the active plans in the meantime.

| Status       | Meaning                                                                                              |
| ------------ | ---------------------------------------------------------------------------------------------------- |
| `Draft`      | Being written; not yet put to review                                                                 |
| `Proposed`   | Written and awaiting a decision to start                                                             |
| `Active`     | Being worked; `In progress` is read as `Active`                                                      |
| `Parked`     | Agreed in principle, not being worked; the status line gives a one-line reason and a revisit trigger |
| `Complete`   | Delivered; graduates to a symlink under the two-PR rule                                              |
| `Superseded` | Replaced by the plan or decision it names                                                            |

| Limit                         | Value                                                |
| ----------------------------- | ---------------------------------------------------- |
| Active bodies of work         | twelve; a programme with its phase plans counts once |
| Active plans per body of work | one                                                  |

A plan is never deleted to meet a limit: it is parked. A hub doc that presents an unbuilt feature
as present is removed under D-33, and its intent stays in its plan.

## Lifecycle

Three phases: incubation, consolidation, graduation; described in the
`/plan-graduation` skill ([.claude/skills/plan-graduation/SKILL.md](../../../.claude/skills/plan-graduation/SKILL.md)).

**Two-PR rule:** A plan must be marked Complete on `main` before it can be
replaced with a symlink. Never complete a plan and create its symlink on the
same feature branch. This ensures git history on `main` contains the full
completed plan before the file becomes a symlink: reviewers can always find
the final plan state in the commit log.

## Enforcement

[scripts/check-plan-canonical-links.py](../../../scripts/check-plan-canonical-links.py) enforces 8 gates in `--check` mode
(run via `make check-plan-hygiene`):

| #   | Gate                                                        |
| --- | ----------------------------------------------------------- |
| 1   | Non-symlink plan missing `- **Canonical:**` link            |
| 2   | `Canonical` target points under [docs/plans/](../../plans)  |
| 3   | `Canonical` target points outside repo or to a missing file |
| 5   | Symlink resolves under [docs/plans/](../../plans)           |
| 6   | Symlink resolves outside the repository                     |
| 7   | Symlink resolves to a missing target                        |
| 8   | `Canonical` link appears more than once in the same header  |
| 9   | `Canonical` target not under an allowed hub prefix          |
| 10  | Symlink created before plan was Complete on `main`          |

Gate 4 (two plans sharing the same canonical target) is advisory only: some
hub docs legitimately serve multiple plans.

| Target                     | Mode      | Purpose                |
| -------------------------- | --------- | ---------------------- |
| `make check-plan-hygiene`  | Hard-fail | CI gate (blocks merge) |
| `make report-plan-hygiene` | Advisory  | PM/editorial review    |

## Current state

Every non-symlink plan under `docs/plans/` carries `Canonical` metadata, and
completed plans are graduated to symlinks that point at their owning hub doc in
one of the four hubs. This batch raises the number of plan aliases from 31 to
34; derive the current value on demand after later graduations:

- `make report-plan-hygiene` — plan count, gate violations, advisory notes.
- `find docs/plans -maxdepth 1 -type l | wc -l` — plans graduated to symlinks.

## Success criteria

1. Every substantial body of work has one stable doc in its owning hub.
2. No two active plans claim the same canonical body of work (or are documented as advisory exceptions).
3. Graduated plans preserve their old URLs through symlinks.
4. Readers no longer need to assemble one topic from two or more sibling plan docs.
