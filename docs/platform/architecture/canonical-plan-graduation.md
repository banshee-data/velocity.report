# Canonical Plan Graduation

How `docs/plans/` relates to the hub documentation tree, and the lifecycle by which
plan files graduate into stable hub docs.

Graduated plan: [platform-canonical-project-files-plan.md](../../plans/platform-canonical-project-files-plan.md)

## Problem

One body of work can spread across two or more plan files, a plan plus a hub
doc, or multiple phase plans that all describe the same architectural identity.
That makes the project hard to reason about because readers must reconstruct
the real source of truth by hand.

## Design Goal

For any substantial body of work, a reader should be able to answer three
questions quickly:

1. What is the stable doc that defines this thing?
2. What plan, if any, is currently driving execution?
3. What happened to the old plan URLs?

## Document Roles

| Role                 | Purpose                                              | Path                     |
| -------------------- | ---------------------------------------------------- | ------------------------ |
| Canonical hub doc    | Stable architecture, implementation record, or model | Hub paths below          |
| Active plan          | Temporary sequencing, phases, checklists             | `docs/plans/*.md`        |
| Graduated plan alias | Preserve legacy URLs after consolidation             | Symlink in `docs/plans/` |

## Hub Structure

Four mutually exclusive hubs, chosen by domain-first sorting:

| Hub              | Scope                                   |
| ---------------- | --------------------------------------- |
| `docs/lidar/`    | LiDAR pipeline, clustering, QC          |
| `docs/radar/`    | Radar pipeline, time-series             |
| `docs/ui/`       | Web frontend, macOS app, PDF generation |
| `docs/platform/` | Go packages, deploy, DB, tooling        |

Additional prefixes `config/` and `data/` are allowed for docs that live beside
the artefacts they describe.

### Sorting Test

To place a doc, ask: _which domain owns the lasting value?_

| If the lasting value is...                            | Canonical home                  |
| ----------------------------------------------------- | ------------------------------- |
| Enduring design, model, API contract, system boundary | `<hub>/architecture/<topic>.md` |
| Operating guidance, migration, implementation status  | `<hub>/operations/<topic>.md`   |
| UI architecture or implementation reference           | `docs/ui/<topic>.md`            |
| Config or maths specification already in `config/`    | Existing `config/` or `data/`   |

## One Body Of Work, One Canonical Doc

Use this test when deciding whether multiple plans should merge:

1. Do they change the same owned system or surface?
2. Do they share the same long-lived architecture or operating model?
3. Would a single reader expect one stable place to understand the whole thing?

If yes to all three, they collapse into one canonical hub doc and at most one
active plan.

## Lifecycle

### Incubation

Every active plan must identify its canonical hub doc in the header metadata:

```markdown
- **Canonical:** [foreground-tracking.md](../../lidar/architecture/foreground-tracking.md)
```

This is the minimal contract that makes the relationship machine-checkable.

### Consolidation

When the work has a stable architecture or operating shape:

1. Move or merge authoritative content into the canonical hub doc.
2. Reduce the active plan to pure execution sequencing.
3. Merge sibling plans if they point at the same canonical hub doc.

### Graduation

When the plan no longer needs to be a separate execution document:

1. Mark the plan status as **Complete** and merge that change to `main`.
2. On a **separate branch** (after the completion lands on `main`), replace the plan file with a symlink to the canonical hub doc.
3. The old plan path survives, preserving existing links.
4. New backlog entries link to the canonical hub doc directly.

**Two-PR rule:** A plan must be marked Complete on `main` before it can be
replaced with a symlink. Never complete a plan and create its symlink on the
same feature branch. This ensures git history on `main` contains the full
completed plan before the file becomes a symlink — reviewers can always find
the final plan state in the commit log.

## Enforcement

### Hard-Fail Gates

`scripts/check-plan-canonical-links.py` enforces 8 gates in `--check` mode
(run via `make check-plan-hygiene`):

| #   | Gate                                                        |
| --- | ----------------------------------------------------------- |
| 1   | Non-symlink plan missing `- **Canonical:**` link            |
| 2   | `Canonical` target points under `docs/plans/`               |
| 3   | `Canonical` target points outside repo or to a missing file |
| 5   | Symlink resolves under `docs/plans/`                        |
| 6   | Symlink resolves outside the repository                     |
| 7   | Symlink resolves to a missing target                        |
| 8   | `Canonical` link appears more than once in the same header  |
| 9   | `Canonical` target not under an allowed hub prefix          |

### Advisory Notes

Gate 4 (two plans sharing the same canonical target) is advisory only, because
6 hub docs legitimately serve as the shared canonical home for more than one
plan. These appear in `make report-plan-hygiene` output but do not block merges.

### Make Targets

| Target                     | Mode      | Purpose                |
| -------------------------- | --------- | ---------------------- |
| `make check-plan-hygiene`  | Hard-fail | CI gate (blocks merge) |
| `make report-plan-hygiene` | Advisory  | PM/editorial review    |

### Rollout

1. **Phase 1 — Tooling:** Checker, Makefile targets, CI advisory job. _Complete._
2. **Phase 2 — Repository refactor:** `Canonical` metadata on all plans, stub hub docs created. _Complete (69/69 plans, 46 stubs, 0 gate violations)._
3. **Phase 3 — Hard-fail CI:** Wire `check-plan-hygiene` into `make lint-docs` and fail CI. _Complete._

## Current State

As of branch `dd/docs/merge-canonical`:

- 69 plan files, all with `Canonical` metadata
- 111 hub docs across 4 hubs (excluding READMEs)
- 26 plans graduated to symlinks
- 0 gate violations, 6 advisory notes (deliberate shared targets)

## Success Criteria

1. Every substantial body of work has one stable doc in its owning hub.
2. No two active plans claim the same canonical body of work (or are documented as advisory exceptions).
3. Graduated plans preserve their old URLs through symlinks.
4. Readers no longer need to assemble one topic from two or more sibling plan docs.
