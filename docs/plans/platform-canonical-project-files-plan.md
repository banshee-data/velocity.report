# Canonical Plan Graduation Plan

- **Status:** Proposed
- **Layers:** Cross-cutting (documentation and tooling)

Use the existing hub structure in `docs/lidar/`, `docs/radar/`, and `docs/ui/` as the permanent home for substantial bodies of work, and treat `docs/plans/` as a temporary execution layer that eventually collapses into symlink aliases.

## 1. Problem

The repo already has the right long-lived structure:

- `docs/BACKLOG.md` is the single source of truth for priority and milestone placement.
- `docs/DECISIONS.md` is the closed-decision register.
- Non-plan docs already live in stable hub paths such as `docs/lidar/architecture/`, `docs/lidar/operations/`, `docs/radar/architecture/`, and `docs/ui/`.

The failure is not the hub layout. The failure is that one body of work can still spread across:

- two or more plan files,
- a plan plus a hub doc,
- multiple phase plans that all describe the same architectural identity.

That makes the project hard to reason about because readers have to reconstruct the real source of truth by hand.

The repo also has tooling assumptions that need to be handled deliberately. For example, [`scripts/flo-planning-review.sh`](../../scripts/flo-planning-review.sh) currently treats `docs/plans/` as a flat set of regular Markdown files via `find ... -type f`, so any symlink-based graduation model must update that script.

## 2. Design Goal

For any substantial body of work, a reader should be able to answer three questions quickly:

1. What is the stable doc that defines this thing?
2. What plan, if any, is currently driving execution?
3. What happened to the old plan URLs?

The answer should be:

1. The stable doc lives in the owning hub.
2. At most one active plan exists for that same body of work.
3. Old plan URLs survive as symlinks to the hub doc once the plan graduates.

## 3. Document Roles

Use three roles only.

| Role                 | Purpose                                                                | Path               | Notes                               |
| -------------------- | ---------------------------------------------------------------------- | ------------------ | ----------------------------------- |
| Canonical hub doc    | Stable architecture, implementation record, status, or operating model | existing hub paths | Permanent source of truth           |
| Active plan          | Temporary sequencing, phases, checklists, open execution work          | `docs/plans/*.md`  | Must point to one canonical hub doc |
| Graduated plan alias | Preserve legacy URLs after consolidation                               | `docs/plans/*.md`  | Symlink to canonical hub doc        |

No new `docs/projects/` tree and no registry file are needed.

## 4. Choosing The Canonical Home

The canonical doc should stay in the existing owning area, not in `docs/plans/`.

| If the lasting value is...                                                        | Canonical home                                       |
| --------------------------------------------------------------------------------- | ---------------------------------------------------- |
| enduring design, model, API contract, or system boundary                          | `<hub>/architecture/<topic>.md`                      |
| operating guidance, migration workflow, implementation status, or tuning practice | `<hub>/operations/<topic>.md`                        |
| UI architecture or implementation reference                                       | `docs/ui/<topic>.md` or the existing UI subdirectory |
| already-owned config or maths specification                                       | existing `config/` or `data/` doc                    |

Examples already present in the repo:

- [time-partitioned-data-tables.md](../radar/architecture/time-partitioned-data-tables.md) is a stable radar architecture doc that backlog items already point at directly.
- [vector-scene-map.md](../lidar/architecture/vector-scene-map.md) is a stable LiDAR architecture doc that can absorb planning history.
- [hint-sweep-mode.md](../lidar/operations/hint-sweep-mode.md) is an operations doc that already acts as the durable home for the sweep system.

## 5. One Body Of Work, One Canonical Doc

The main rule should be:

> One architectural identity gets one canonical hub doc.

Use this test when deciding whether multiple plan files should be merged:

1. Do they change the same owned system or surface?
2. Do they share the same long-lived architecture or operating model?
3. Would a single reader expect one stable place to understand the whole thing?

If the answer is "yes" to those questions, they should collapse into one canonical hub doc and at most one active plan.

Separate plan files are only justified when they are genuinely different bodies of work with different canonical homes, even if they are related.

## 6. Lifecycle

### 6.1 Incubation

An active plan may exist in `docs/plans/`, but it must identify its canonical hub doc using normal header metadata:

```md
- **Canonical:** [vector-scene-map.md](../lidar/architecture/vector-scene-map.md)
```

This is the minimal additional contract needed to make the relationship machine-checkable without introducing a second index.

### 6.2 Consolidation

As soon as the work has a stable architecture or operating shape:

1. Move or merge the authoritative content into the canonical hub doc.
2. Reduce the active plan to pure execution sequencing if it still needs to exist.
3. Merge sibling plans if they point at the same canonical hub doc.

### 6.3 Graduation

When the plan no longer needs to be a separate execution document:

1. Replace the plan file with a symlink to the canonical hub doc.
2. Keep the old plan path to preserve existing GitHub and repository links.
3. Continue linking new backlog entries to the canonical hub doc where practical.

## 7. What Collates The Work

No new central registry is needed. Collation comes from three existing structures:

1. `docs/BACKLOG.md` for scheduling.
2. The owning hub README and folder structure for navigation.
3. The canonical hub doc itself for the full body of work.

That keeps the model DRY:

- one place for schedule,
- one place for decisions,
- one place for the stable body of work.

## 8. CI Enforcement

Enforcement should be deterministic and local, with no LLM involvement.

Add `scripts/check-plan-canonical-links.py` and wire it into `make lint-docs` plus CI.

### 8.1 Rollout Sequence

To avoid breaking the existing repository (which currently has plan files without `- **Canonical:**` metadata), enforcement should roll out in three phases:

1. **Advisory only:** Land `scripts/check-plan-canonical-links.py` in advisory / report-only mode. Initially, run it locally and in CI as a non-fatal job that:
   - reports missing `Canonical` metadata,
   - reports obviously invalid targets (outside the repo or under `docs/plans/`),
   - does **not** fail CI.
2. **Repository refactor:** Use the advisory output to do a one-off refactor pass:
   - add `- **Canonical:**` metadata to all non-symlink plans under `docs/plans/`,
   - introduce or fix symlinked plans where appropriate,
   - move any remaining long-lived content into the correct hub docs.
3. **Hard-fail CI:** Once the repository is clean, update `make lint-docs` and CI so that the checker runs in hard-fail mode using the rules below.

### 8.2 Hard-Fail Gates

The checker should fail if any of the following are true:

1. A non-symlink plan file in `docs/plans/` does not contain a `- **Canonical:**` link.
2. A `Canonical` link points to another file under `docs/plans/`.
3. A `Canonical` link points outside the repository or to a missing file.
4. Two non-symlink plan files declare the same canonical hub doc target.
5. A symlinked plan file resolves to another file under `docs/plans/`.
6. A symlinked plan file resolves outside the repository.
7. A symlinked plan file resolves to a missing target.
8. A `Canonical` link appears more than once in the same plan header.
9. A `Canonical` target is not under an allowed hub-doc prefix (for example: `docs/lidar/`, `docs/radar/`, or `docs/ui/`).

### 8.3 Advisory Gates

The checker should also emit an advisory report for:

- hub docs with multiple related plan files before consolidation,
- backlog or decision links that still point at a non-symlink plan when a canonical hub doc already exists,
- plan files whose canonical target is not under the expected owning hub.

Advisories should not block merges, but they should appear in CI summaries and planning-review output so hygiene debt remains visible.

### 8.3 Suggested Make Targets

Add two targets:

- `make check-plan-hygiene` — runs the hard-fail checker locally and in CI.
- `make report-plan-hygiene` — emits the advisory report for PM and editorial review.

This separates merge blockers from cleanup signals.

## 9. Planning Review Updates

Update [`scripts/flo-planning-review.sh`](../../scripts/flo-planning-review.sh) so it:

1. includes plan symlinks in its inventory,
2. reports each active plan together with its `Canonical` target,
3. flags collisions where more than one active plan points at the same canonical doc.

That makes split-work detection explicit in the PM review flow.

## 10. Agent Workflows

The hygiene loop should be owned across PM, architecture, and implementation roles instead of relying on one generic docs pass.

### 10.1 Florence (PM) — Detect And Triage

When asked for a planning review or docs-hygiene pass:

1. Run `scripts/flo-planning-review.sh`.
2. Run `scripts/check-plan-canonical-links.py --report` when available, or perform the equivalent manual grouping by `Canonical` target.
3. Identify collisions where two or more active plans point at the same canonical hub doc.
4. Classify each collision as:
   - merge into one plan,
   - keep separate because the canonical homes differ,
   - graduate one or more plans to symlinks.
5. End with exact edits proposed to `docs/BACKLOG.md`, the affected hub doc, and the affected plan files.

### 10.2 Grace (Architect) — Choose The Canonical Home

When a collision or split topic is found:

1. Decide which existing hub doc should own the stable body of work.
2. Decide whether the correct permanent home is architecture, operations, UI, or an already-owned non-plan doc.
3. Specify what durable content must be merged into that canonical doc.
4. Decide whether multiple plans are truly separate projects or one fragmented project.
5. Write the consolidation rule before Appius edits files.

### 10.3 Appius (Dev) — Enforce And Land

When implementing hygiene fixes:

1. Add or repair the `Canonical` link in active plans.
2. Merge durable content into the chosen hub doc.
3. Replace graduated plans with symlinks.
4. Update `scripts/flo-planning-review.sh`, `make lint-docs`, and CI as required.
5. Verify hard-fail and advisory outputs before finishing.

### 10.4 Terry / Editorial — Polish After Structure

After consolidation lands:

1. Remove duplicated prose left behind by merges.
2. Tighten hub README navigation if the canonical doc should be easier to discover.
3. Ensure the canonical doc reads as a stable body of work, not as an abandoned plan.

## 11. One-Off Refactor

Implementation is a one-time cleanup plus ongoing light enforcement.

### 11.1 Inventory

1. Cluster existing plan files by subject area and owning hub.
2. Identify where multiple plans already describe one architectural identity.
3. Choose the hub doc that should become the stable body of work.

### 11.2 Consolidate

1. Merge durable architecture and implementation truth into the hub doc.
2. Keep only one active execution plan for that body of work.
3. Convert superseded plans into symlinks to the hub doc.

### 11.3 Enforce

1. Add the `Canonical` metadata link to any remaining active plans.
2. Add the CI checker.
3. Update planning-review tooling to surface collisions.

## 12. Non-Goals

- No new `docs/projects/` tree.
- No registry or generated project index.
- No semantic duplicate detection in CI.
- No change to `docs/BACKLOG.md` as the scheduling source of truth.

## 13. Success Criteria

This design is successful when:

1. Every substantial body of work has one stable doc in its existing owning hub.
2. No two active plan files claim the same canonical body of work.
3. Graduated plans preserve their old URLs through symlinks.
4. Readers no longer need to assemble one topic from two or more sibling plan docs.
