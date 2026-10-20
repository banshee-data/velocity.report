# Planning limits: a backlog of items and a plan corpus with a work-in-progress limit

- **Status:** Proposed. Expands option b of P2 and P3 in the [architecture review](platform-architecture-review-plan.md#p2-the-backlog-is-a-progress-log-wearing-a-lists-clothes)
- **Layers:** Cross-cutting (docs/BACKLOG.md, docs/plans/, plan template, lint, planning skills)
- **Target:** v0.5.8
- **Decisions:** DECISIONS.md milestone principle 6 (ten to twelve items a milestone); BACKLOG.md governance (agreed items are never deleted); ADR-3 and ADR-4 of the review
- **Companion plans:** [domain-tag-vocabulary-plan](domain-tag-vocabulary-plan.md) (its plan-backlog cross-check), [platform-documentation-standardisation-plan](platform-documentation-standardisation-plan.md), [platform-release-cadence-plan](platform-release-cadence-plan.md) (bucket names), [platform-decision-keeping-plan](platform-decision-keeping-plan.md) (plans cite decisions)
- **Canonical:** [canonical-plan-graduation.md](../platform/architecture/canonical-plan-graduation.md)

Keep the backlog a list a person can read in one sitting and the plan corpus a set of things
being built. Two bodies of rules, one owner: P2 (the backlog) and P3 (the plans) share the same
surface, the same readers and the same lint, so they are one plan.

## Motivation

| Measure                                                | Value on `acdb03e`                                 | Rule it breaks                                   |
| ------------------------------------------------------ | -------------------------------------------------- | ------------------------------------------------ |
| Open backlog items                                     | 307 in 23 release buckets                          | none, but nobody can rank 307 things             |
| Items in v0.5.2                                        | 55 in five sprint sub-buckets                      | DECISIONS.md: ten to twelve a milestone          |
| Average item length                                    | 372 characters; 36 items over 600                  | BACKLOG.md: at most two short sentences          |
| Items that carry results, branch names or PR narrative | common in v0.5.2 to v0.5.7                         | BACKLOG.md: rationale belongs in the linked plan |
| Active plan files                                      | 149; 80 proposed, draft or planned                 | none: there is no limit                          |
| Plans that say they are ready to graduate and have not | 2                                                  | the graduation rule                              |
| Hub docs describing features that do not exist         | 10 visualiser stubs whose header is "Source plan:" | a hub doc records what shipped                   |

The cost is real: priorities are invisible, the release that should ship cannot be named, and the
review itself had to count by script to know what the backlog holds.

## Design / approach

### What may leave the backlog, and what must stay

BACKLOG.md's governance holds: an agreed item is never deleted, only split, consolidated,
completed or moved. What leaves an item is its narrative: results, measurements, branch names and
the history of what landed. That moves to the item's plan, into its checklist or progress
section, and the item keeps one or two sentences and its link. Sprint sub-buckets are a plan's
phases, so the phase structure moves to the plan while every item stays in the backlog under the
release it is next due in.

### Limits

| Limit                                               | Value                               | Enforced by                                            |
| --------------------------------------------------- | ----------------------------------- | ------------------------------------------------------ |
| Items per release bucket                            | twelve                              | backlog lint, advisory for a release, then gated       |
| Characters per item                                 | 400, about two sentences and a link | backlog lint, advisory then gated                      |
| Buckets named for a release                         | the next release and the one after  | backlog lint; later work sits under programme headings |
| Active plans                                        | twelve                              | `report-plan-hygiene`, advisory                        |
| Active plans with no linked PR or commit in 90 days | listed                              | `report-plan-hygiene`, advisory                        |

### Plan statuses

One closed vocabulary: `Draft`, `Proposed`, `Active`, `Parked`, `Complete`, `Superseded`. `Parked`
is new and means agreed in principle but not being worked; it carries a one-line reason and the
trigger that would make it Active. The status checker accepts a free-text suffix after the
status word, so existing detail survives.

### Hub docs record what shipped

A hub doc whose only content is a pointer to an unbuilt plan is removed; the plan's `Canonical`
moves to the hub that will own the feature when it ships (for the visualiser stubs,
`docs/ui/visualiser/VISUALISER.md`). The graduation rule then runs as written: the plan is the
intent, the hub doc is written when the work lands.

## Scope

### Item 1: the backlog re-baseline

**Summary:** the bucket moves, retitles, splits and shortenings from Flo's planning pass, applied with every agreed item kept.

**Milestone:** v0.5.8 (this branch carries the first pass)

### Item 2: the backlog lint

**Summary:** a script beside the existing documentation checks that counts items per bucket and characters per item, and lists buckets named for releases beyond the next two.

**Steps:**

1. The check in `scripts/`, run by `make lint-docs`, advisory for one release.
2. Gate it once the re-baseline has brought every bucket within the limits.

**Milestone:** v0.5.8

### Item 3: plan statuses and the work-in-progress limit

**Summary:** the closed status vocabulary with `Parked`; `report-plan-hygiene` counts Active plans and lists the stale ones; the parking pass applied.

**Steps:**

1. Template, graduation hub and the plan hygiene docs gain the vocabulary.
2. The status check in `scripts/check-plan-canonical-links.py` (or a sibling), advisory.
3. The parking pass from Flo's review: each parked plan's status line gains its reason and trigger.
4. Graduate the two plans that say they are ready, under the two-PR rule.

**Milestone:** v0.5.8

### Item 4: hub stubs

**Summary:** the ten visualiser hub stubs removed and their plans' `Canonical` lines moved to the visualiser hub.

**Milestone:** v0.5.8

## Dependencies

- Item 1 needs Flo's list and Ruth's ruling on what may move.
- Item 2's gate waits for Item 1.
- Item 3's graduations follow the two-PR rule: Complete on `main` first, the symlink after.

## Risks

| Risk                                   | Likelihood | Impact | Mitigation                                                              |
| -------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------- |
| Narrative moved out of an item is lost | Low        | Medium | It moves into the plan in the same commit; the item links it            |
| Parking reads as cancelling            | Medium     | Low    | Parked keeps the plan, its reason and its trigger                       |
| The limits are gamed by vague items    | Medium     | Medium | Items still name one deliverable outcome; `/backlog-prune` reviews them |

## Checklist

### Outstanding

- [ ] Item 1: the backlog re-baseline (`M`)
- [ ] Item 2: the backlog lint (`S`)
- [ ] Item 3: plan statuses, the WIP limit and the parking pass (`M`)
- [ ] Item 4: hub stubs (`S`)

### Accepted residuals (no action planned)

- [ ] The Complete section of the backlog keeps its length: it is the record of delivered work
