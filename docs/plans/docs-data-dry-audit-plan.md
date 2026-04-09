# Documentation and Data Dry Audit Plan

- **Status:** Active
- **Layers:** Cross-cutting (documentation infrastructure)
- **Target:** v0.6.0 — documentation coherence is a prerequisite for onboarding contributors and
  reducing maintenance drag on the growing doc corpus
- **Canonical:** [documentation-standards.md](../platform/operations/documentation-standards.md)

## Motivation

The `docs/` and `data/` trees have grown substantially — 206 Markdown files under `docs/`
and 32 under `data/` — through a healthy process of writing plans before implementation and
graduating them to hub docs on completion. That discipline is working: the canonical-plan
graduation machinery exists, 69 plans carry `Canonical` metadata, and 11 have already been
promoted to symlinks (per `canonical-plan-graduation.md §Current State`).

The problem is the **completion backlog**: 14 gate violations are reported by
`make report-plan-hygiene` (advisory mode — CI does not yet hard-fail on these),
16 further plans are marked Complete but have not yet been graduated to symlinks (leaving
their canonical docs carrying stale "Active plan:" links back to redundant files), several
docs are misclassified in the wrong directory, and a cluster of UI subdirectory files is
fragmented when a single consolidated doc would serve readers better.

The consequence of inaction is a documentation tree where readers must assemble one topic
from two or three files, where gate violations accumulate unchecked, and where the plans/
directory fills with noise that obscures genuinely active work.

## Current State

| Area                             | Current state                                                    | Severity |
| -------------------------------- | ---------------------------------------------------------------- | -------- |
| Gate violations (advisory)       | 14 violations across 8 plan files                                | High     |
| Complete plans not graduated     | 16 plans marked Complete, not yet symlinked                      | High     |
| Misclassified docs in `plans/`   | 4 files that are guides or research notes, not plans             | Medium   |
| UI subdirectory fragmentation    | 5 files under `velocity-visualiser-app/` fragment one topic      | Medium   |
| Ops docs that are actually plans | 3 ops files carrying "Planning" or design-phase content          | Medium   |
| Cross-doc parameter repetition   | Config keys repeated in maths README and ops tuning guides       | Low      |
| Resolved troubleshooting stubs   | 1 troubleshooting file with Status: Resolved, never consolidated | Low      |

Running `make report-plan-hygiene` as of audit date produces:

```
14 gate violation(s), 7 advisory note(s)
```

CI currently runs `make report-plan-hygiene` (advisory mode, exit 0). These violations are
reported but not yet enforced. The hard-fail target `make check-plan-hygiene` exists locally
but is not wired into CI workflows.

## Findings

### 1 DRY Violations

#### 1.1 Pipeline layer table (acceptable shallow duplication)

The L1–L10 layer table appears in:

- `ARCHITECTURE.md` — summary table (explicitly references `lidar-data-layer-model.md` as
  canonical, then adds package names)
- `docs/lidar/architecture/lidar-data-layer-model.md` — canonical, full definition, locked
- `docs/lidar/architecture/lidar-pipeline-reference.md` — extended reference with code paths

`ARCHITECTURE.md` is the system-overview entry point; its summary table is intentional
shallow duplication with a reference link. This is acceptable. The pipeline reference adds
detail not in the model. **No action required** — these serve different reader levels.

However, a further 7 plan files (e.g. `lidar-clock-abstraction-and-time-domain-model-plan.md`,
`lidar-l8-analytics-l9-endpoints-l10-clients-plan.md`) embed their own layer tables to
establish context. These are in-plan summaries, acceptable while active. They drift as the
layer model evolves; graduated plans that become symlinks fix this naturally.

#### 1.2 Config parameter keys (medium-severity DRY)

The full set of L3–L5 config keys appears in two independent locations:

- `data/maths/README.md` §Config Mapping — complete key lists with source paths (authoritative
  for maths consumers)
- `docs/lidar/operations/config-param-tuning.md` §Core Parameter Groups — grouped by tuning
  workflow (authoritative for operators)

These serve different readers and different purposes; neither can simply point to the other.
But the content is close enough that a parameter rename or addition requires two edits.
**Medium-priority fix**: add a cross-reference from `config-param-tuning.md` to the maths
README's config mapping section, and vice versa. No merge needed; just a "See also:" link.

A similar dual-appearance exists with `auto-tuning.md` §Tunable Parameters, which lists
all tunable keys in narrative form. This serves a distinct purpose (explaining what each
parameter _does_ during automated sweep) and does not require remediation.

#### 1.3 ARCHITECTURE.md Perception Pipeline section (acceptable)

`ARCHITECTURE.md` §Perception Pipeline contains detailed prose descriptions of L3–L6 (several
hundred words each). The same information, at greater technical depth, lives in:

- `docs/lidar/architecture/foreground-tracking.md`
- `docs/lidar/architecture/ground-plane-extraction.md`
- `data/maths/tracking-maths.md`
- `data/maths/clustering-maths.md`

`ARCHITECTURE.md` is the public-facing entry point for contributors and external readers. Its
prose is intentionally self-contained at a higher level of abstraction. **No action required**;
the architecture doc links to the detailed references where present.

---

### 2 Superseded and Redundant Plans

#### 2.1 Complete plans eligible for immediate graduation

These plans are marked **Complete** on `main`, carry a `Canonical` link, and have a hub doc
that has absorbed their content. They satisfy the two-PR graduation rule: their Complete status
is on `main`, so a separate branch may replace them with symlinks.

| Plan file                                                 | Canonical hub doc                | Hub location                   |
| --------------------------------------------------------- | -------------------------------- | ------------------------------ |
| `agent-claude-preparedness-review-plan.md`                | `agent-preparedness.md`          | `platform/operations/`         |
| `data-database-alignment-plan.md`                         | `database-sql-boundary.md`       | `platform/architecture/`       |
| `data-sqlite-client-standardisation-plan.md`              | `database-sql-boundary.md`       | `platform/architecture/`       |
| `go-god-file-split-plan.md`                               | `go-package-structure.md`        | `platform/architecture/`       |
| `label-vocabulary-consolidation-plan.md`                  | `label-vocabulary.md`            | `lidar/architecture/`          |
| `lidar-analysis-run-infrastructure-plan.md`               | `pcap-analysis-mode.md`          | `lidar/operations/`            |
| `lidar-immutable-run-config-asset-plan.md`                | `immutable-run-config.md`        | `lidar/operations/`            |
| `lidar-l2-dual-representation-plan.md`                    | `l2-dual-representation.md`      | `lidar/architecture/`          |
| `lidar-layer-dependency-hygiene-plan.md`                  | `lidar-data-layer-model.md`      | `lidar/architecture/`          |
| `lidar-sweep-hint-mode-plan.md`                           | `hint-sweep-mode.md`             | `lidar/operations/`            |
| `lidar-tracks-table-consolidation-plan.md`                | `track-storage-consolidation.md` | `lidar/architecture/`          |
| `lidar-visualiser-run-list-labelling-rollup-icon-plan.md` | `run-list-labelling-rollup.md`   | `lidar/operations/visualiser/` |
| `platform-canonical-project-files-plan.md`                | `canonical-plan-graduation.md`   | `platform/architecture/`       |
| `schema-simplification-migration-030-plan.md`             | `schema-migration-030.md`        | `platform/operations/`         |
| `tooling-python-venv-consolidation-plan.md`               | `python-venv.md`                 | `platform/operations/`         |
| `v050-tech-debt-removal-plan.md`                          | `v050-release-migration.md`      | `platform/operations/`         |

Each canonical hub doc carries a stale "Active plan:" link back to the plan file. Once the
symlink is in place that link resolves correctly. **This is 16 plans**; graduating them
removes 16 redundant files from the active plans list and shrinks the directory from 82 to
66 entries without losing any content.

#### 2.2 Implemented plans pending merge — not yet graduatable

One plan is marked "Implemented" but its implementation is on a branch, not `main`. The
two-PR rule prohibits graduation until the Complete status lands on `main`.

| Plan file                         | Status                                                          | Action                                                      |
| --------------------------------- | --------------------------------------------------------------- | ----------------------------------------------------------- |
| `lidar-schema-robustness-plan.md` | Implemented on branch `codex/draft-schema-improvement-proposal` | Mark Complete after merge; then graduate on separate branch |

#### 2.3 Plans near completion — must be marked Complete before graduation

| Plan file                                         | Current status                                            | Gap                                                                   |
| ------------------------------------------------- | --------------------------------------------------------- | --------------------------------------------------------------------- |
| `platform-simplification-and-deprecation-plan.md` | Approved (Phase 1 complete)                               | Remaining phases must complete or be deferred before marking Complete |
| `lidar-track-labelling-auto-aware-tuning-plan.md` | Phases 1-5 complete, Phase 6 deferred, others outstanding | Resolve deferred items in checklist, then mark Complete               |

---

### 3 Gate Violations — advisory today, worth fixing proactively

`make report-plan-hygiene` currently reports 14 violations across 8 files. These are
advisory (CI does not hard-fail) but should be resolved to keep the plan corpus clean.

| File                                              | Violation                                                         | Correct action                                                                                                    |
| ------------------------------------------------- | ----------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `asset-naming-plan.md`                            | [G1] missing `Canonical` metadata                                 | Add `- **Canonical:** [platform/architecture doc]` (new hub doc needed or BACKLOG.md)                             |
| `domain-tag-vocabulary-plan.md`                   | [G9] Canonical points to `BACKLOG.md` (not an allowed hub prefix) | Create stub `platform/architecture/domain-tag-vocabulary.md` or point to `lidar/architecture/label-vocabulary.md` |
| `error-surface-voice-audit-plan.md`               | [G2, G3, G9] Canonical is the literal string "This document"      | Create stub `platform/operations/error-surface-voice.md`, point Canonical there                                   |
| `lidar-replay-case-terminology-alignment-plan.md` | [G1] missing `Canonical` metadata                                 | Add `- **Canonical:** [lidar/architecture/lidar-pipeline-reference.md]` or new stub                               |
| `macos-local-server-plan.md`                      | [G2, G3, G9] Canonical is "This document"                         | Point to `ui/velocity-visualiser-architecture.md` (the existing home for server manager features)                 |
| `pcap-motion-detection-and-split-plan.md`         | [G1] missing `Canonical` metadata                                 | Add `- **Canonical:** [lidar/operations/pcap-analysis-mode.md]`                                                   |
| `setup-guide-publication-plan.md`                 | [G1] missing `Canonical` metadata                                 | Add `- **Canonical:** [public_html/src/guides/setup.md]` or a platform ops stub                                   |
| `tailscale-remote-access-guide.md`                | [G2, G3, G9] Canonical is "this document"                         | Move to `platform/operations/tailscale-remote-access.md`; replace with symlink                                    |

The `tailscale-remote-access-guide.md` is not really a plan — it is a complete operational
guide. It belongs in `docs/platform/operations/`. Create it there, redirect the plan file to
that canonical home.

---

### 4 Fragmented Topics

#### 4.1 `docs/ui/velocity-visualiser-app/` — five-file fragment

The subdirectory `docs/ui/velocity-visualiser-app/` contains five files written during the
initial macOS visualiser design phase:

| File                               | Content                                                                 | Status                                                                   |
| ---------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| `00-repo-inspection-notes.md`      | Research notes from design phase                                        | Superseded — `velocity-visualiser-architecture.md` absorbed the findings |
| `01-problem-and-user-workflows.md` | Problem statement and user workflows                                    | Partially absorbed into architecture doc                                 |
| `02-api-contracts.md`              | API contract specification (self-described as "most critical document") | Active reference — but orphaned in subdirectory                          |
| `05-troubleshooting.md`            | macOS build troubleshooting                                             | Operational but stranded                                                 |
| `performance-investigation.md`     | Investigation of M3.5 streaming, frame throttle                         | Substantially complete                                                   |

The split test: same owned system (`docs/ui/`), same long-lived architecture, same reader
expectation. These should be one body of work in `docs/ui/`, not five files in a subdirectory.

Correct action:

- `00-repo-inspection-notes.md` — archive or delete (content superseded)
- `01-problem-and-user-workflows.md` — merge relevant material into `velocity-visualiser-architecture.md`
- `02-api-contracts.md` — promote to `docs/ui/velocity-visualiser-api-contracts.md` (top-level `ui/`)
- `05-troubleshooting.md` — merge into root `TROUBLESHOOTING.md` under a macOS Visualiser section
- `performance-investigation.md` — merge relevant conclusions into `velocity-visualiser-architecture.md`

#### 4.2 PCAP tool docs — three ops files that span design and ops

Three operational docs under `docs/lidar/operations/` describe PCAP tools at varying stages:

| File                               | Actual status                                                                                     |
| ---------------------------------- | ------------------------------------------------------------------------------------------------- |
| `pcap-analysis-mode.md`            | Implemented — correct as ops doc                                                                  |
| `pcap-split-tool.md`               | Design document ("Executive Summary" structure) — should be a plan or be promoted to architecture |
| `pcap-ground-plane-export-tool.md` | Status: Planning — is effectively a plan living in ops/                                           |

The `pcap-ground-plane-export-tool.md` belongs in `docs/plans/` (it is forward-looking design),
or its planning content should move to `pcap-motion-detection-and-split-plan.md` and the ops
file should describe only what is implemented. The `pcap-split-tool.md` design doc should move
to `docs/lidar/architecture/pcap-split-tool.md` once the tool is built, or to a plan while still
in design.

#### 4.3 Two convergence experiment results — minor fragmentation

`data/explore/convergence-neighbour/` contains two nearly identical experiment result files:
`convergence-findings.md` and `convergence-neighbour-closeness-findings.md`. These are
sequential sweeps (noise sweep, then closeness/neighbour interaction sweep) from the same
investigation. They could be one consolidated experiment log. Low priority — empirical
data files in `data/explore/` are expected to accumulate; they do not need to be single docs.
**Advisory only** — consolidate if passing through this area.

---

### 5 Structural Issues

#### 5.1 Misclassified docs in `plans/`

| File                                    | Problem                                                                                               | Correct location                                                      |
| --------------------------------------- | ----------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| `tailscale-remote-access-guide.md`      | A complete operational guide, not a plan                                                              | `docs/platform/operations/tailscale-remote-access.md`                 |
| `homepage-responsive-gif-strategies.md` | Implementation notes on one UI decision; Canonical is `ui/homepage.md`                                | Graduate once Complete; `ui/homepage.md` already absorbed the content |
| `server-manager.md`                     | Feature spec / design notes, not a phased plan; Canonical is `ui/velocity-visualiser-architecture.md` | OK to remain in plans/ while active; move to `docs/ui/` on completion |
| `wireshark-menu-alignment.md`           | Design research notes; Canonical is `ui/macos-menu-layout-design.md`                                  | OK to remain in plans/ while active; graduate on completion           |

#### 5.2 Ops docs carrying planning-phase content

| File                                                      | Problem                                                            | Correct action                                                                                                                                                                          |
| --------------------------------------------------------- | ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `docs/lidar/operations/pcap-ground-plane-export-tool.md`  | Status: Planning — forward-looking design, not implemented         | Move content to `docs/plans/pcap-motion-detection-and-split-plan.md` or a new plan; stub or delete the ops file                                                                         |
| `docs/lidar/operations/webserver-tuning-schema-parity.md` | Lists missing API keys as a backlog item, not an operational guide | Move to a plan or to an open section of `docs/plans/unpopulated-data-structures-remediation-plan.md`; or create `docs/platform/architecture/api-tuning-schema.md` as its canonical home |

#### 5.3 Resolved troubleshooting entries never consolidated

`docs/lidar/troubleshooting/warmup-trails-fix.md` has **Status: Resolved** (January 2026).
This is a fixed bug log. Its content should be incorporated into the main `TROUBLESHOOTING.md`
as a known-and-fixed issue note, and the file archived or deleted. Keeping resolved bug logs in
the active troubleshooting directory creates misleading "known issues" for readers.

#### 5.4 Stale architecture review artefacts

| File                                                               | Issue                                                                                                                                                                                                                                             |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `docs/lidar/architecture/lidar-layer-alignment-refactor-review.md` | Dated 2026-02-17; reviews alignment of the layer model. Findings have been absorbed into the layer model and pipeline reference. Verify content is represented in current canonical docs before archiving.                                        |
| `docs/lidar/architecture/math-foundations-audit.md`                | An audit document. Check whether its action items are tracked in plans or the maths README. If fully absorbed, archive.                                                                                                                           |
| `docs/lidar/architecture/coordinate-flow-audit.md`                 | Audit of polar/Cartesian bounce; findings are factual and correct. Remains useful as a reference for the `l2-dual-representation` design rationale. **Keep** — but add a status note indicating the findings are informational, not action items. |

#### 5.5 README indirection docs

The following README files are pure navigation indices with no architectural content:

- `docs/lidar/README.md`
- `docs/lidar/architecture/README.md`
- `docs/platform/README.md`

These are legitimate; directory listings are the right navigation mechanism per the docs
standards. No action required — they intentionally avoid stale index maintenance. The
`docs/lidar/README.md` does provide layer context. **Keep as-is.**

#### 5.6 `docs/lidar/playback-speed-vs-track-quality.md` — undirected root file

This file sits directly under `docs/lidar/` rather than in `architecture/` or `operations/`.
Its content (how PCAP replay speed affects tracking accuracy) is operational knowledge.
Move to `docs/lidar/operations/playback-speed-vs-track-quality.md` and update
`docs/lidar/README.md` to point to the new location.

#### 5.7 Hub docs that may not outlive their plans

Two docs currently serve as canonical hub homes for plans but may not have long-term value
once those plans complete. Rather than protecting them indefinitely, they should be
consolidated when their owning plan graduates:

| File                                                  | Issue                                                                                                                                                                                                                                     | Recommended action                                                                                                                          |
| ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `docs/lidar/operations/foundations-fixit-progress.md` | 42-line progress tracker that is the canonical hub for `lidar-architecture-foundations-fixit-plan`. Its content is a status snapshot, not durable operational guidance. When the plan completes, this doc has no standalone reader value. | Merge remaining-gaps content into the plan's completion notes or the relevant architecture docs, then delete. Do not protect as permanent.  |
| `docs/ui/design-review-and-improvement.md`            | 437-line improvement backlog with P1 items. Functions as a plan but lives in `docs/ui/` outside the plan hygiene system. Not tracked by `check-plan-hygiene`, so its status is invisible to tooling.                                      | Either move to `docs/plans/` with proper metadata (Canonical → `docs/ui/DESIGN.md`), or merge actionable items into BACKLOG.md and archive. |

#### 5.8 Advisory: multiple plans sharing one canonical hub doc

`make report-plan-hygiene` flags 7 advisory notes. Three are worth tracking:

| Canonical hub doc                                | Multiple plans pointing to it                                                             | Notes                                                                                                                    |
| ------------------------------------------------ | ----------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `platform/architecture/go-package-structure.md`  | `go-cmd-extraction-plan`, `go-codebase-structural-hygiene-plan`, `go-god-file-split-plan` | `go-god-file-split-plan` is Complete (graduate it); the other two are active distinct work streams — advisory is correct |
| `platform/operations/documentation-standards.md` | `line-width-standardisation-plan`, `platform-documentation-standardisation-plan`          | Two genuinely separate sub-tasks of the same canonical; acceptable until both complete                                   |
| `platform/operations/pdf-reporting.md`           | `pdf-go-chart-migration-plan`, `pdf-latex-precompiled-format-plan`                        | Two independent PDF improvement tracks; acceptable                                                                       |

---

### 6 What the data/ Tree Is Doing Well

For completeness: `data/` is structurally sound.

- `data/maths/` has a comprehensive README that doubles as the research roadmap.
- `data/structures/` holds specification-grade wire format docs in UPPER_CASE.
- `data/experiments/` and `data/explore/` accumulate empirical work without polluting `docs/`.
- `data/QUESTIONS.md` is the single-source of open research questions.

The only data/ concern is the minor duplication between `data/maths/README.md §Config Mapping`
and `docs/lidar/operations/config-param-tuning.md` (see §1.2).

---

## Strategies

### Strategy A — Modest Coherence (low effort, high impact)

Fix gate violations and graduate complete plans. This is pure hygiene — no content changes —
and it immediately clears the CI backlog, shrinks the plans directory by ~20%, and removes
stale "Active plan:" links from hub docs.

**Effort:** ~3–4 hours spread across 2 PRs per plan batch.
**Impact:** CI green, plans directory coherent, hub docs no longer point to dead plans.
**Risk:** Low — mechanical symlink creation and header edits.

### Strategy B — Standard Coherence (medium effort, meaningful improvement)

Strategy A plus: consolidate the `velocity-visualiser-app/` subdirectory, move
`tailscale-remote-access-guide.md` to its proper ops home, fix misclassified ops docs
(`pcap-ground-plane-export-tool.md`, `webserver-tuning-schema-parity.md`), resolve the
`warmup-trails-fix.md` closed bug, and add the DRY cross-references between the maths README
and the ops tuning guide.

**Effort:** ~8–10 hours across 4–6 PRs.
**Impact:** Eliminates the most confusing navigation traps; makes `docs/ui/` and `docs/lidar/operations/`
self-consistent; reduces "is this still active?" questions.
**Risk:** Low-medium — content moves require link-checker pass.

### Strategy C — Maximum Coherence (full DRY, single-source, no redundancy)

Strategy B plus: review and archive `lidar-layer-alignment-refactor-review.md` and
`math-foundations-audit.md` (verify absorption into current canonical docs first), consolidate
the two convergence experiment result files in `data/explore/`, move `playback-speed-vs-track-quality.md`
to `operations/`, add "See also" cross-references between all config parameter docs, and do a
full opening-paragraph audit of the ~40 docs still missing narrative opening paragraphs per the
`platform-documentation-standardisation-plan`.

**Effort:** ~20–30 hours across many PRs.
**Impact:** Documentation tree is maximally coherent; every doc has a clear single-source
relationship; no ambiguous status.
**Risk:** Medium — touching many files increases merge conflict risk; opening paragraph automation
requires careful batch review.

**Recommendation: Start with Strategy A immediately; follow with targeted Strategy B items
as they surface during normal development. Defer Strategy C to a dedicated documentation
sprint when the `platform-documentation-standardisation-plan` enters its final phase.**

---

## Recommended Actions Checklist

### Group 1 — Quick Wins (CI fixes and complete-plan graduation, ~4 hours total)

These can all be batched into 2–3 PRs. Group graduation symlinks by hub to minimise
review surface.

#### 1a. Fix gate violations (≈ 1 hour)

- [ ] `asset-naming-plan.md` — create stub `docs/platform/architecture/asset-naming.md` as the canonical home for asset naming conventions, then add `- **Canonical:** [asset-naming.md](../platform/architecture/asset-naming.md)` to the plan. Do not point to `canonical-plan-graduation.md` — that doc describes plan lifecycle mechanics, not asset naming. (`S`) <!-- link-ignore -->
- [ ] `domain-tag-vocabulary-plan.md` — change Canonical from BACKLOG.md to either `lidar/architecture/label-vocabulary.md` or a new stub `platform/architecture/domain-tag-vocabulary.md` (`S`)
- [ ] `error-surface-voice-audit-plan.md` — create stub `docs/platform/operations/error-surface-voice.md` and set as Canonical (`S`)
- [ ] `lidar-replay-case-terminology-alignment-plan.md` — add `- **Canonical:** [lidar-pipeline-reference.md](../lidar/architecture/lidar-pipeline-reference.md)` (`S`)
- [ ] `macos-local-server-plan.md` — fix Canonical from "This document" to `[velocity-visualiser-architecture.md](../ui/velocity-visualiser-architecture.md)` (`S`)
- [ ] `pcap-motion-detection-and-split-plan.md` — add `- **Canonical:** [pcap-analysis-mode.md](../lidar/operations/pcap-analysis-mode.md)` (`S`)
- [ ] `setup-guide-publication-plan.md` — add `- **Canonical:** [setup.md](../../public_html/src/guides/setup.md)` (requires `public_html/` in `ALLOWED_HUB_PREFIXES` — added by this PR) (`S`)
- [ ] `tailscale-remote-access-guide.md` — create `docs/platform/operations/tailscale-remote-access.md` (move content), then fix Canonical in the plan file to point there; graduate to symlink once Complete (`M`)

Verify `make check-plan-hygiene` passes after each batch.

#### 1b. Graduate complete plans — LiDAR hub batch (≈ 1 hour)

One branch per hub to keep PRs reviewable. Each PR: replace plan file with symlink pointing
to its canonical hub doc. Verify hub doc no longer says "Active plan:" after merge.

- [ ] `lidar-layer-dependency-hygiene-plan.md` → symlink → `../lidar/architecture/lidar-data-layer-model.md` (`S`)
- [ ] `lidar-l2-dual-representation-plan.md` → symlink → `../lidar/architecture/l2-dual-representation.md` (`S`)
- [ ] `lidar-immutable-run-config-asset-plan.md` → symlink → `../lidar/operations/immutable-run-config.md` (`S`)
- [ ] `lidar-sweep-hint-mode-plan.md` → symlink → `../lidar/operations/hint-sweep-mode.md` (`S`)
- [ ] `lidar-tracks-table-consolidation-plan.md` → symlink → `../lidar/architecture/track-storage-consolidation.md` (`S`)
- [ ] `lidar-analysis-run-infrastructure-plan.md` → symlink → `../lidar/operations/pcap-analysis-mode.md` (`S`)
- [ ] `label-vocabulary-consolidation-plan.md` → symlink → `../lidar/architecture/label-vocabulary.md` (`S`)
- [ ] `lidar-visualiser-run-list-labelling-rollup-icon-plan.md` → symlink → `../lidar/operations/visualiser/run-list-labelling-rollup.md` (`S`)

#### 1c. Graduate complete plans — Platform hub batch (≈ 1 hour)

- [ ] `agent-claude-preparedness-review-plan.md` → symlink → `../platform/operations/agent-preparedness.md` (`S`)
- [ ] `data-database-alignment-plan.md` → symlink → `../platform/architecture/database-sql-boundary.md` (`S`)
- [ ] `data-sqlite-client-standardisation-plan.md` → symlink → `../platform/architecture/database-sql-boundary.md` (`S`)
- [ ] `go-god-file-split-plan.md` → symlink → `../platform/architecture/go-package-structure.md` (`S`)
- [ ] `platform-canonical-project-files-plan.md` → symlink → `../platform/architecture/canonical-plan-graduation.md` (`S`)
- [ ] `schema-simplification-migration-030-plan.md` → symlink → `../platform/operations/schema-migration-030.md` (`S`)
- [ ] `tooling-python-venv-consolidation-plan.md` → symlink → `../platform/operations/python-venv.md` (`S`)
- [ ] `v050-tech-debt-removal-plan.md` → symlink → `../platform/operations/v050-release-migration.md` (`S`)

#### 1d. Remove stale "Active plan:" links from hub docs whose plans have graduated

After graduation PRs merge, update each canonical hub doc to remove or convert the
"Active plan: …" link (since the plan file is now a symlink that resolves correctly, the
link remains valid but the label is misleading). Change to "Graduated plan (archived):" or
remove the line entirely.

- [ ] Update `immutable-run-config.md` — remove/update Active plan link (`S`)
- [ ] Update `hint-sweep-mode.md` — remove/update Active plan link (`S`)
- [ ] Update `l2-dual-representation.md` — remove/update Active plan link (`S`)
- [ ] Update `track-storage-consolidation.md` — remove/update Active plan link (`S`)
- [ ] Update `python-venv.md` — remove/update Active plan link (`S`)
- [ ] Update `go-package-structure.md` — remove/update Active plan link for `go-god-file-split-plan` only (other two plans are still active) (`S`)
- [ ] Update `database-sql-boundary.md` — remove/update Active plan links for both graduated plans (`S`)
- [ ] Update `canonical-plan-graduation.md` — update §Current State count after graduation batch (`S`)

---

### Group 2 — Consolidations (structural moves, ~2 hours each)

#### 2a. Consolidate `docs/ui/velocity-visualiser-app/` subdirectory (≈ 3 hours total)

- [ ] Verify `00-repo-inspection-notes.md` content is fully absorbed in `velocity-visualiser-architecture.md`; if yes, delete the file and remove from any index (`M`)
- [ ] Merge relevant user workflow content from `01-problem-and-user-workflows.md` into `velocity-visualiser-architecture.md` §User Workflows or §Problem Statement; delete source file (`M`)
- [ ] Promote `02-api-contracts.md` to `docs/ui/velocity-visualiser-api-contracts.md` (top-level `ui/`); update all inbound links (`M`)
- [ ] Merge `05-troubleshooting.md` macOS visualiser content into root `TROUBLESHOOTING.md` under new §macOS Visualiser section; delete source file (`M`)
- [ ] Merge `performance-investigation.md` conclusions into `velocity-visualiser-architecture.md` §Performance Notes; delete source file (`M`)
- [ ] Remove the `velocity-visualiser-app/` subdirectory once empty; update `docs/ui/` README or navigation if present (`S`)

#### 2b. Resolve misclassified PCAP ops docs (≈ 1 hour)

- [ ] `pcap-ground-plane-export-tool.md` — move planning-only content to `pcap-motion-detection-and-split-plan.md` §Scope or a new scope item; stub or delete the ops file if no implementation exists yet (`M`)
- [ ] `pcap-split-tool.md` — determine if the tool is implemented: if yes, revise as a true ops guide; if no, move to `docs/plans/` or merge into `pcap-motion-detection-and-split-plan.md` (`M`)

#### 2c. Relocate `tailscale-remote-access-guide.md` (≈ 45 minutes)

- [ ] Create `docs/platform/operations/tailscale-remote-access.md` with the current content of `tailscale-remote-access-guide.md` (`S`)
- [ ] Update `tailscale-remote-access-guide.md` to point Canonical at the new file; mark plan as Complete; graduate to symlink on next branch (`M`)
- [ ] Verify `make check-plan-hygiene` passes (`S`)

#### 2d. Relocate `docs/lidar/playback-speed-vs-track-quality.md` (≈ 30 minutes)

- [ ] Move file to `docs/lidar/operations/playback-speed-vs-track-quality.md` (`S`)
- [ ] Update `docs/lidar/README.md` and any inbound links (`S`)

---

### Group 3 — DRY Fixes (cross-reference additions, ~1 hour each)

#### 3a. Config parameter cross-references (≈ 30 minutes)

- [ ] Add "See also:" reference in `docs/lidar/operations/config-param-tuning.md` §6 Deep References pointing to `data/maths/README.md §Config Mapping` (`S`)
- [ ] Add reciprocal link in `data/maths/README.md §Config Mapping` noting that the operational tuning workflow is in `docs/lidar/operations/config-param-tuning.md` (`S`)

#### 3b. Resolve `warmup-trails-fix.md` closed bug (≈ 30 minutes)

- [ ] Add a "Known fixed issues" note to `TROUBLESHOOTING.md` §LiDAR / Background Grid summarising the warmup trails fix (January 2026, resolved via region persistence) (`S`)
- [ ] Delete `docs/lidar/troubleshooting/warmup-trails-fix.md` or move to `data/explore/` as historical reference (`S`)

---

### Group 4 — Structural Improvements (verification and cleanup, ~4 hours)

#### 4a. Verify and archive stale architecture review artefacts (≈ 2 hours)

- [ ] Read `docs/lidar/architecture/lidar-layer-alignment-refactor-review.md` §Findings: verify each finding is addressed in `lidar-data-layer-model.md`, `lidar-pipeline-reference.md`, or `go-package-structure.md`. If fully absorbed, archive the file (move to `data/explore/` or delete). If still active as a reference, add a Status note and a Canonical link. (`M`)
- [ ] Read `docs/lidar/architecture/math-foundations-audit.md` §Action items: verify each item is tracked in an active plan or the maths README. If fully absorbed, archive. (`M`)
- [ ] `docs/lidar/architecture/coordinate-flow-audit.md` — add a status note: "Findings are informational; the described coordinate bounce is confirmed and intentional per `l2-dual-representation.md`." No structural action needed. (`S`)

#### 4b. Classify `webserver-tuning-schema-parity.md` correctly (≈ 30 minutes)

- [ ] Determine if the webserver params gap is tracked in `unpopulated-data-structures-remediation-plan.md`. If yes, add a cross-reference there and delete `webserver-tuning-schema-parity.md`. If no, either create a new plan with proper Canonical metadata or move the file to `docs/plans/` with correct structure. (`M`)

#### 4c. Plans with no Status line — add status metadata (≈ 30 minutes)

Several plans are missing a `**Status:**` line, which makes their state invisible to the
plan hygiene tooling and to readers:

- [ ] `data-track-description-language-plan.md` — add Status line (`S`)
- [ ] `lidar-architecture-foundations-fixit-plan.md` — add Status line (check current state against `foundations-fixit-progress.md`) (`S`)
- [ ] `lidar-distributed-sweep-workers-plan.md` — add Status line (the canonical `distributed-sweep.md` says "Proposed"; verify) (`S`)
- [ ] `lidar-ml-classifier-training-plan.md` — add Status line (`S`)
- [ ] `macos-local-server-plan.md` — add Status line after fixing Canonical (`S`)
- [ ] `platform-data-science-metrics-first-plan.md` — add Status line (`S`)
- [ ] `platform-quality-coverage-improvement-plan.md` — add Status line (`S`)
- [ ] `platform-typed-uuid-prefixes-plan.md` — add Status line (`S`)
- [ ] `tictactail-platform-plan.md` — add Status line (`S`)
- [ ] `homepage-responsive-gif-strategies.md` — add Status line; mark Complete if `ui/homepage.md` has absorbed all content; then graduate (`S`)

---

## Dependencies

- `make check-plan-hygiene` must pass after every PR in Group 1.
- `make check-md-links` (link integrity) must pass after every file move in Group 2.
- Graduation symlinks (Groups 1b, 1c) must land on a **separate branch** from the plan
  completion marking, per the two-PR graduation rule in `canonical-plan-graduation.md`.
- Group 2a (velocity-visualiser-app consolidation) must be sequenced: verify absorption
  before deleting source files.

## Risks

| Risk                                                         | Likelihood | Impact | Mitigation                                                                         |
| ------------------------------------------------------------ | ---------- | ------ | ---------------------------------------------------------------------------------- |
| Symlink creates stale inbound links from external references | Low        | Medium | Run `make check-md-links` after every graduation PR                                |
| Hub doc "Active plan:" links break after symlink graduation  | Low        | Low    | Update hub docs in same PR as graduation (Group 1d)                                |
| Content lost during velocity-visualiser-app consolidation    | Low        | High   | Read all 5 files before deleting; verify destination doc contains each key concept |
| Gate violation fixes introduce new gate violations           | Low        | Medium | Run `make report-plan-hygiene` before and after Group 1a PRs                       |
| Strategy B moves disturb active work                         | Medium     | Low    | Coordinate PCAP doc moves with the `pcap-motion-detection-and-split-plan` owner    |

## Checklist

### Complete

**Group 1 — Quick wins**

- [x] Fix 8 gate violations (Group 1a) (`S` × 8)
- [x] Graduate LiDAR hub plans — 7 symlinks (Group 1b) (`S` × 7; `label-vocabulary-consolidation-plan` skipped — not Complete)
- [x] Graduate Platform hub plans — 7 symlinks (Group 1c) (`S` × 7; `schema-simplification-migration-030-plan` skipped — status is "Implemented")
- [x] Graduate `tailscale-remote-access-guide.md` — marked Complete, content moved to hub, replaced with symlink
- [x] Update hub docs: remove stale "Active plan:" links (Group 1d) (`S` × 5)

**Group 2 — Consolidations**

- [x] Consolidate `velocity-visualiser-app/` subdirectory (Group 2a) (`M`)
- [x] Resolve misclassified PCAP ops docs (Group 2b) (`M`)
- [x] Relocate `tailscale-remote-access-guide.md` to `platform/operations/` (Group 2c) (`M`)
- [x] Relocate `playback-speed-vs-track-quality.md` to `lidar/operations/` (Group 2d) (`S`)

**Group 3 — DRY fixes**

- [x] Add config parameter cross-references (Group 3a) (`S`)
- [x] Resolve `warmup-trails-fix.md` closed bug (Group 3b) (`S`)

### Outstanding

**Group 4 — Structural improvements**

- [ ] Verify and archive stale architecture review artefacts (Group 4a) (`M`)
- [ ] Classify `webserver-tuning-schema-parity.md` correctly (Group 4b) (`M`)
- [ ] Add missing Status lines to 10 plans (Group 4c) (`S` × 10)

### Deferred

- [ ] Strategy C full opening-paragraph audit (~40 docs still missing narrative openings) —
      tracked by [platform-documentation-standardisation-plan.md](platform-documentation-standardisation-plan.md) <!-- link-ignore -->
- [ ] Consolidate `data/explore/convergence-neighbour/` two experiment files — low priority;
      empirical data accumulation is expected
- [ ] Archive or resolve `lidar-schema-robustness-plan.md` after its branch merges to `main`

### Accepted residuals (no action planned)

- [ ] Pipeline layer tables repeated in active plans — plans embed summary tables for
      self-contained context; this drift is eliminated automatically as plans graduate to symlinks.
      Not worth fixing in active plans that are about to complete.
- [ ] Detailed L3–L6 prose in `ARCHITECTURE.md` §Perception Pipeline duplicating individual
      architecture docs — `ARCHITECTURE.md` is a self-contained public entry point; intentional
      duplication at a higher abstraction level. Maintained separately with references to canonical
      docs.
- [ ] Config key listings in `auto-tuning.md` — serves a different reader (automated sweep
      operators) than `config-param-tuning.md` (human tuners). Cross-reference added (Group 3a);
      full merge would destroy both docs' narrative coherence.

---

## What NOT to Touch

The following docs look redundant but serve a distinct purpose and should stay:

| Doc                                                      | Why it stays                                                                                                                                                                                                                                                                               |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `docs/lidar/architecture/coordinate-flow-audit.md`       | Factual reference explaining the intentional polar/Cartesian representation bounce; documents the `l2-dual-representation` design decision with measured evidence. Not superseded — clarified.                                                                                             |
| `docs/lidar/terminology.md`                              | Single-source glossary for LiDAR-specific terms. Referenced by `lidar-replay-case-terminology-alignment-plan.md` and multiple other docs. Serves a distinct purpose from architecture docs.                                                                                                |
| `docs/lidar/operations/parameter-comparison.md`          | A snapshot of an optimisation result (specific parameter values, before/after). Unlike the tuning guide, it is a concrete recommendation with measured impact, not a workflow. Readers consult it when deploying a specific tuning baseline.                                               |
| `docs/lidar/operations/settling-time-optimisation.md`    | Documents the region-persistence approach to the settling problem. Complementary to `adaptive-region-parameters.md` (which explains the feature) — this one explains why the feature was needed and what approaches were considered.                                                       |
| `data/maths/README.md` §Config Mapping                   | The maths README's config section is written for algorithm researchers, not operators. It maps config keys to the mathematical models they govern. This is a different reader and purpose from `config-param-tuning.md`. Cross-reference added; merge would lose the mathematical framing. |
| `docs/lidar/troubleshooting/garbage-tracks-checklist.md` | States it is the canonical document for garbage-track remediation. It combines review and checklist. Not superseded by any architecture doc; still operationally relevant.                                                                                                                 |
| `docs/lidar/troubleshooting/pipeline-diagnosis.md`       | Symptom-based diagnosis guide (jitter, fragmentation, empty boxes). Complements the tuning guide, which is parameter-focused. Different reader intent: "something is wrong" vs "I want to improve it".                                                                                     |
| `docs/ui/DESIGN.md`                                      | Product design language reference — colour palette, typography, layout constraints. Serves UI contributors who are not consulting architecture or plans.                                                                                                                                   |
| All `data/maths/proposals/` files                        | Mathematical proposals are primary research artefacts, not plans. They document the mathematical justification for future algorithm changes. They should remain in `data/maths/proposals/`, not in `docs/plans/`.                                                                          |
| `data/QUESTIONS.md`                                      | Canonical index of open research questions. Single-source; well-structured. Lives correctly in `data/`.                                                                                                                                                                                    |
| `data/structures/MATRIX.md`                              | Full surface matrix for the entire system. Maintained by the `trace-matrix` skill. Not redundant with architecture docs — it is a machine-readable checklist of implemented vs. planned surfaces.                                                                                          |
