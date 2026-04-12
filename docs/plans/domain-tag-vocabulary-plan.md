# Domain tag vocabulary

- **Status:** Proposed
- **Layers:** Cross-cutting (documentation, backlog, CI)
- **Canonical:** [label-vocabulary.md](../lidar/architecture/label-vocabulary.md)

Introduce a closed vocabulary of domain tags for classifying backlog items and plan documents. Tags appear inline in backlog entries and as metadata in plan doc headers, enabling automated theme coherence checks and per-domain filtering.

## Motivation

The backlog-prune skill already infers domain labels from item wording during theme coherence analysis. Making these tags explicit and machine-readable has three benefits:

1. **Deterministic theme checks**: backlog-prune can validate domain spread from tags instead of inferring from prose, eliminating false positives.
2. **Filtering**: scripts can list all items in a domain (e.g. "show me everything tagged `algorithm`") without full-text heuristics.
3. **Plan–backlog traceability**: a plan's `Domains` metadata and its backlog entry's `{domain}` tags should agree; drift is lintable.

## Tag vocabulary

Eleven canonical domain tags. Each tag is a lowercase hyphenated slug.

| Tag              | Scope                                                               |
| ---------------- | ------------------------------------------------------------------- |
| `go-structure`   | God-file splits, package hygiene, SQL boundary, context propagation |
| `go-lidar`       | LiDAR pipeline layers L1–L9, perception, clustering, tracking       |
| `go-radar`       | Radar ingest, transit worker, serial config                         |
| `data-schema`    | Migrations, schema changes, SQLite model                            |
| `web-ui`         | Svelte frontend, charts, design system                              |
| `mac-visualiser` | Swift/Metal macOS app                                               |
| `platform-infra` | Deploy, packaging, CI, RPi image, scripts                           |
| `observability`  | Logging, metrics, performance harness, profiling                    |
| `docs-plans`     | Documentation, plan hygiene, canonical files                        |
| `algorithm`      | Maths proposals, perception algorithm changes, Kalman, DBSCAN       |
| `api-contract`   | HTTP API shape, proto contract, type safety, naming                 |

### Adding new tags

New tags require a PR that updates the vocabulary table above, the backlog header reference, the lint script's known-tags list, and the backlog-prune skill definition. Keep the vocabulary small: prefer reusing an existing tag over introducing a new one.

## Phase 1: backlog inline tags

Add domain tags to backlog items in curly braces between the issue reference and the title:

```
- [#NNN] (#NNN) {go-lidar,algorithm} Title of the item: description `M`
- (#NNN) {web-ui} Title without a PR: description `S`
- {platform-infra,docs-plans} Title with no issue: description `XS`
```

Rules:

- Tags appear inside `{...}`, comma-separated, no spaces.
- One or two tags per item (three maximum for genuinely cross-cutting work).
- Position: after `[#PR]` and `(#issue)` references, before the title text.
- Items in the Complete section do not require tags (they are historical).
- The backlog header documents the tag vocabulary with a brief reference table.

### Effort

Tag all ~80 pending backlog items. Mechanical: read each item, assign 1–2 tags. Half a day.

## Phase 2: plan doc metadata

Add a `- **Domains:**` metadata field to plan document headers:

# My Plan Title

- **Status:** Proposed
- **Layers:** L4 Perception, L5 Tracks
- **Domains:** go-lidar, algorithm
- **Canonical:** [hub-doc.md](../path/to/hub.md) <!-- link-ignore -->

Rules:

- `Domains` uses the same tag vocabulary as backlog items.
- Comma-separated with spaces after commas (consistent with existing metadata style).
- Complements `Layers` (which describes pipeline layers): `Domains` describes the cross-cutting concern area.
- Existing `Layers` field is unchanged; it serves a different purpose.

### Effort

Add `Domains` to ~69 plan docs. Mechanical: most can be inferred from existing `Layers` field. Half a day.

## Phase 3: lint enforcement

Add a lightweight lint script (`scripts/check-domain-tags.py`) that:

1. **Validates backlog tags**: every `{...}` block in pending items contains only known tags.
2. **Validates plan metadata**: every `- **Domains:**` field contains only known tags.
3. **Cross-checks**: if a backlog item links to a plan doc, the backlog item's tags should be a subset of or equal to the plan's `Domains`. Warn on mismatches (do not hard-fail initially).

Wire into `make lint-docs`.

### Effort

Small Python script, similar in structure to `check-plan-canonical-links.py`. Half a day.

## Phase 4: backlog-prune integration

Update the backlog-prune skill to:

1. **Read tags** from `{...}` blocks instead of inferring domains from prose.
2. **Fall back** to inference for untagged items (during transition period).
3. **Report untagged items** as a new audit finding: "N items missing domain tags".

### Effort

Edit [.claude/skills/backlog-prune/SKILL.md](../../.claude/skills/backlog-prune/SKILL.md). Trivial.

## Effort summary

| Phase                  | Work                               | Effort       |
| ---------------------- | ---------------------------------- | ------------ |
| 1. Backlog inline tags | Tag ~80 items                      | S (half day) |
| 2. Plan doc metadata   | Add `Domains` to ~69 plans         | S (half day) |
| 3. Lint script         | `check-domain-tags.py` + CI wiring | S (half day) |
| 4. Skill integration   | Update backlog-prune               | XS           |
| **Total**              |                                    | **2 days**   |

## What does not change

- The `Layers` metadata field in plan docs: it describes pipeline layers, not domains.
- The commit prefix scheme (`[go]`, `[js]`, etc.): it describes file types, not domains.
- The existing backlog governance rules: tags are additive metadata.
- Items in the Complete section: no tags required (historical).
