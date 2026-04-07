---
name: backlog-prune
description: Backlog grooming — verify completed items have PR numbers, identify release themes across the next 5 point releases, surface classification drift, and flag L/XL items for splitting.
argument-hint: "[--scan-all-prs]"
---

# Skill: backlog-prune

Groom `docs/BACKLOG.md`. Covers three distinct passes:

1. **Completeness audit** — every item in the Complete section must have a `[#NNN]` PR number.
2. **Theme coherence** — read the next 5 point releases, identify theme drift and rebalancing opportunities.
3. **Size audit** — flag L/XL items that should be split and propose candidate sub-tasks.

## Usage

```
/backlog-prune
/backlog-prune --scan-all-prs
```

`--scan-all-prs` fetches each referenced PR from GitHub to verify it exists and is merged. Without the flag, only format is checked (no network calls).

---

## Procedure

### 1. Read the backlog

```bash
cat docs/BACKLOG.md
```

Parse the file into two logical regions:

- **Pending** — all items above the `## Complete` heading, grouped by their milestone section (`### vX.Y.Z …`).
- **Complete** — all items under `## Complete`.

### 2. Completeness audit (Complete section)

For every line in the Complete section:

- **Has PR** — line begins with `[#NNN]` where NNN is a positive integer. ✅
- **Missing PR** — line begins with `(#NNN)` (issue-only), `-` with no bracket reference, or any other format. ❌

Collect all missing-PR items into a list.

#### If `--scan-all-prs` is set

For each `[#NNN]` reference found, fetch the PR to verify it is real and merged:

```bash
gh pr view NNN --json number,state,mergedAt --jq '[.number, .state, .mergedAt]'
```

Flag PRs that:

- Do not exist (gh exits non-zero)
- Are open (not yet merged)
- Are closed but not merged (abandoned)

#### Produce completeness report

```
## Completeness Audit

### Missing PR numbers (N items)
- Line NNN: "- (#381) …" — has issue reference only, needs [#PR]
- Line NNN: "- SSE backpressure …" — no reference at all

### PR verification (only with --scan-all-prs)
- #NNN: ✅ merged 2026-01-15
- #NNN: ❌ open — not yet merged
- #NNN: ❌ not found
```

If all complete items have valid `[#NNN]` references, write:

```
✅ All complete items carry a PR number.
```

### 3. Theme coherence (next 5 point releases)

Identify the **next 5 point releases** — the first five `### vX.Y.Z` sections in the pending region (in document order). If fewer than five exist, use all pending milestones.

For each release, extract:

- The milestone title (e.g. `v0.5.2 - Data Contracts + Layer Foundations`)
- The effort tags present: count of `XS`, `S`, `M`, `L`, `XL`
- The domain spread: infer domains from item wording and linked plan docs. Use these canonical domain labels:
  - `go-structure` — god-file splits, package hygiene, SQL boundary, context propagation
  - `go-lidar` — LiDAR pipeline layers L1–L9, perception, clustering, tracking
  - `go-radar` — radar ingest, transit worker, serial config
  - `data-schema` — migrations, schema changes, SQLite model
  - `web-ui` — Svelte frontend, charts, design system
  - `mac-visualiser` — Swift/Metal macOS app
  - `platform-infra` — deploy, packaging, CI, RPi image, scripts
  - `observability` — logging, metrics, performance harness, profiling
  - `docs-plans` — documentation, plan hygiene, canonical files
  - `algorithm` — maths proposals, perception algorithm changes, Kalman, DBSCAN
  - `api-contract` — HTTP API shape, proto contract, type safety, naming

Assess each release against these questions:

1. **Coherent theme?** Does the milestone title accurately describe the majority of its items, or are there unrelated items bundled in?
2. **Balanced effort?** Is any single effort tier (L or XL) dominating? More than 2 L/XL items in a single release is a risk.
3. **Domain spread healthy?** A release spanning more than 4 distinct domains risks losing focus.
4. **Cross-release dependencies visible?** Are items that depend on a prior release's output placed correctly (not before their dependency)?

#### Drift detection

Compare the inferred domain/theme of each item against the milestone's stated theme. An item is **drifted** if:

- Its domain does not match any of the milestone's primary domains (the 2–3 most common in that release), AND
- It has no explicit dependency note explaining why it is placed there

Collect drifted items. For each, propose a better home (a different existing milestone, or a note that it should become its own milestone entry).

Present drift proposals as suggestions only — do not modify the backlog. Use this format:

```
## Theme Coherence

### v0.5.2 - Data Contracts + Layer Foundations
Primary domains: go-structure, data-schema, go-lidar
Effort distribution: XS×0 S×2 M×5 L×1 XL×0

Theme fit: ✅ Items are consistent with "data contracts + layer foundations"

### v0.5.3 - Replay/Runtime Stabilisation
Primary domains: go-lidar, mac-visualiser, go-structure
Effort distribution: S×4 M×4 L×0 XL×0

Theme fit: ⚠️ 2 items appear drifted:
  - "Database SQL boundary consolidation" [go-structure / data-schema]
    → Better fit: v0.5.2 (Data Contracts) — depends on #406 which already landed there
    Proposed move: v0.5.2 or standalone v0.5.x cleanup item
  - "Frontend background debug surfaces" [web-ui / mac-visualiser]
    → Theme is mac-visualiser but item is web-specific; consider splitting into
      two items: one Mac (`S`) and one Web (`S`)
```

### 4. Size audit (L and XL items)

Scan all pending items for effort tags `L` or `XL`. For each:

- Read the linked design doc path if present (do not fetch the file — use the path as a signal)
- Assess whether the item description implies multiple independently deliverable phases

An item is a **split candidate** if any of these apply:

- It mentions multiple numbered phases (e.g. "Phases 1–3")
- It mentions more than one distinct layer or component (e.g. "Go store/API layer … and Web components")
- Its description exceeds ~25 words and references both a back-end and a front-end concern
- It is `XL` with no phase structure described

For each split candidate, propose 2–4 sub-task names with inferred effort tags. Use this format:

```
## Size Audit

### Split candidates

#### `Single velocity-report binary + subcommands` [L] — v0.6.0
Reason: Three independent delivery phases visible in design doc path (deploy-distribution-packaging-plan.md):
  binary consolidation, subcommand wiring, and one-line installer.
Proposed split:
  - `Unified binary scaffold: main entrypoint + subcommand router` [M]
  - `radar/lidar/pdf subcommand wiring` [M]
  - `One-line curl installer with platform detection` [S]  ← already a separate item, confirm dedup

#### `L8/L9/L10 layer refactor Phases 4–5` [L] — v0.6.0
Reason: Phases 4 and 5 are distinct: rename step (Phase 4) vs absorb/decompose (Phase 5).
Proposed split:
  - `Rename visualiser/ → l9endpoints/ and update all import paths` [S]
  - `Absorb chart/dashboard code from monitor/ into l9endpoints/` [M]
  - `Decompose monitor/ into server/ + layered packages` [M]
```

If no L/XL items are split candidates, write:

```
✅ All L/XL items appear appropriately scoped or already phase-structured.
```

### 5. Produce the full grooming report

Combine the three passes into a single structured output:

```markdown
# Backlog Grooming — [date]

## 1. Completeness Audit

[output from step 2]

## 2. Theme Coherence (next 5 point releases)

[output from step 3]

## 3. Size Audit

[output from step 4]

## 4. Recommended Actions

### Requires human decision

- [item requiring approval before backlog edit — one bullet per decision]

### Ready to apply (no decision needed)

- [backlog edit that follows directly from a finding — e.g. add a missing PR number once confirmed]

## 5. Proposed Backlog Edits

Present any proposed text changes as a diff-style block. Do NOT apply them — wait for approval.

\`\`\`diff

- - (#381) Classification display vs selectable enum split … `S`

* - [#NNN] (#381) Classification display vs selectable enum split … `S`
    \`\`\`
```

---

## Notes

- This skill **reads and proposes only**. It never writes to `docs/BACKLOG.md` directly.
  All edits require explicit human or agent approval before application.
- Apply the governance rule from the backlog header: never delete agreed items — only split,
  consolidate, move, or complete them.
- When proposing a split of an L/XL item, the original item should become a parent stub
  referencing the sub-items, or be replaced by the sub-items if the original wording is fully
  superseded.
- If a drifted item has a `(#NNN)` issue reference or explicit dependency comment that
  explains its placement, do not flag it as drift — it is intentionally anchored.
- The `--scan-all-prs` flag is slow (one `gh` call per PR). Warn the user before making
  more than 20 network calls.
- Do not propose moving items to milestones that are already complete or to versions earlier
  than the current latest shipped release.
- If a proposed split would create an item that already exists elsewhere in the backlog,
  flag the duplicate instead of creating a new entry.
