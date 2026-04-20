# Line-Width standardisation plan

- **Status:** On hold (April 2026 review)
- **Canonical:** [documentation-standards.md](../platform/operations/documentation-standards.md)

Plan to adopt 100 columns as the single line-width standard across all code and documentation, with
gradual enforcement via weekly nag PRs, optional CI gating, and an opt-in pre-commit hook.

## April 2026 findings (do not ship now)

Current recommendation: pause rollout and do not ship the broad Markdown reflow batch.

### Review scope

- Reviewed the full current Markdown width-change set in the working tree.
- Files touched: 278 Markdown files.
- Largest diffs include:
  - `docs/DEVLOG.md` (1002 changed lines)
  - `data/maths/pipeline-review-open-questions.md` (799)
  - `docs/BACKLOG.md` (741)
  - `CONTRIBUTING.md` (514)

### Findings

1. Change surface is too large for safe review in one pass.
2. Some edits are harmless prose reflow, but there are high-risk structured-doc degradations.
3. At least one flattened list-style addition pattern was detected (`: - ` in added lines).
4. 74 added Markdown lines exceed 120 columns, indicating inconsistent outcomes from the width pass.
5. Concrete structural break examples were observed in technical plan docs, including compressed
   pseudo-code/comment lines such as:
   - `type Pose struct { T [16]float64 // ... // ... }`
   - `State: x = [x, y, vx, vy]ᵀ Covariance: P (4×4 matrix)`

### Decision

- Do not ship the current wide Markdown line-width batch.
- Re-scope to targeted, file-by-file changes with explicit reviewer ownership for each docs area.
- Keep line-width checks advisory-only until structured-doc safety checks are added.

### Required follow-up before re-attempt

1. Add guardrails to reflow tooling for structured prose patterns (lists, pseudo-code blocks,
   formula-heavy sections, and comment-style lines in Markdown).
2. Run the formatter in small batches by doc area and review each batch independently.
3. Add a pre-merge detector for suspicious Markdown rewrites (flattened lists, merged pseudo-code,
   and excessive long-line regressions).
4. Re-open rollout only after a narrow pilot batch passes manual review with no structural regressions.

## Problem

The repository currently uses five different line widths:

| Component            | Width | Formatter                        |
| -------------------- | ----: | -------------------------------- |
| Go                   |     - | gofmt (no width enforcement)     |
| Python               |    88 | black                            |
| TypeScript/JS/Svelte |   100 | prettier                         |
| Swift                |   100 | swift-format                     |
| SQL                  |    70 | sql-formatter (expression width) |
| Markdown prose       |    90 | check-prose-line-width.py        |

Five widths means five mental models for when to wrap. The
inconsistency also produces noisy diffs when text moves between
documentation and code comments or between languages.

## Data

Lines analysed across all source and documentation files
(March 2026, excluding vendored/minified assets):

| Language | Files |   Lines |   ≤80 |   ≤90 |  ≤100 |  ≤110 |  ≤120 |
| -------- | ----: | ------: | ----: | ----: | ----: | ----: | ----: |
| Go       |   428 | 181,437 | 96.1% | 97.9% | 98.9% | 99.4% | 99.6% |
| Python   |    72 |  28,107 | 96.9% | 99.3% | 99.6% | 99.8% | 99.8% |
| TS/JS    |    44 |  16,557 | 97.3% | 99.3% | 99.8% | 99.9% | 99.9% |
| Svelte   |    17 |   8,943 | 95.0% | 97.9% | 99.2% | 99.5% | 99.6% |
| Swift    |    35 |  24,742 | 92.2% | 96.4% | 99.7% | 99.9% | 99.9% |

Swift shows the strongest pressure: 825 lines sit in the
81–100 band. These are function signatures, buffer allocations,
and conditionals that read naturally at width and become harder
to follow when force-wrapped across three lines.

Go lines exceeding 100 are predominantly string literals
(1,361), function signatures (217), and conditionals (139).
Most string literals should be exempt from a width linter.

### What each threshold costs

| Width   | Effect                                                                                        |
| ------- | --------------------------------------------------------------------------------------------- |
| 80      | Fights every formatter in use. Forces 3,783 Go and 1,929 Swift lines to wrap.                 |
| 90      | Still wraps 825 natural Swift lines. Gains two columns over black's 88: not worth the churn.  |
| **100** | Matches three of five formatters. Every language ≥98.9% compliant. Natural convergence point. |
| 110     | Non-standard. No formatter defaults here. Marginal gain.                                      |
| 120     | Too wide for side-by-side diff review.                                                        |
| 132     | Historical terminal width. No practical advantage over 120.                                   |

## Decision: 100 columns

100 is where the data, the tooling, and the existing configs
converge.

Three of five formatters already use it. Every language reaches
≥98.9% compliance at this width. The remaining Go violations
are predominantly string literals: best handled by linter
exemption, not forced wrapping.

100 also aligns with established external standards: Google
Swift style (100), Rust's rustfmt default (100), and the
majority of projects using prettier.

### Architectural perspective (Grace)

A single number means a single mental model. The formatters
do not care: black accepts `--line-length`, prettier accepts
`printWidth`, swift-format accepts `lineLength`. The only
holdout is gofmt, which enforces nothing, making it the
easiest to align rather than the hardest. Layer a linter
beside it.

The Raspberry Pi is a deployment target, not a development
environment. Operators read logs over SSH, not source code.
At 100 the overflow past a standard 80-column terminal is
20 columns: the terminal soft-wraps at the edge.

### Infrastructure perspective (Appius)

The rollout separates cleanly: change configs first, reformat
second, enforce third. Pre-commit hooks already delegate to
`make format-*` targets, so they follow the Makefile
automatically. One mechanical reformat PR with
`.git-blame-ignore-revs` protection pays the cost once.

## Implementation

### Phase 1: adopt configs

One PR. Only config files change; no source reformatting yet.

| File                                                                               | Change                                         |
| ---------------------------------------------------------------------------------- | ---------------------------------------------- |
| [scripts/check-prose-line-width.py](../../scripts/check-prose-line-width.py)       | `DEFAULT_WIDTH = 90` → `100`                   |
| Makefile `check-prose-width` comment                                               | Update "90" → "100"                            |
| `pyproject.toml` (new, root)                                                       | `[tool.black] line-length = 100`               |
|                                                                                    | `[tool.ruff] line-length = 100`                |
| `.golangci.yml` (new, root)                                                        | Enable `lll` linter, `line-length: 100`        |
| [web/.prettierrc](../../web/.prettierrc)                                           | Already 100: no change                         |
| [tools/visualiser-macos/.swift-format](../../tools/visualiser-macos/.swift-format) | Already 100: no change                         |
| [.sql-formatter.json](../../.sql-formatter.json)                                   | Leave at 70 (expression width, not line width) |

### Phase 2: reformat

One PR. Run `make format` under the new configs. The diff is
large but mechanical: reviewers verify the config, not every
line.

Create `.git-blame-ignore-revs` at the repo root containing
the reformat commit SHA. GitHub honours this file
automatically. Document the one-time local setup in
[CONTRIBUTING.md](../../CONTRIBUTING.md):

```bash
git config blame.ignoreRevsFile .git-blame-ignore-revs
```

### Phase 3: weekly nag PR

A scheduled GitHub Actions workflow runs weekly (e.g. Sunday
night). It checks all code and documentation against the 100-
column standard, and if violations exist, opens or updates a
standing PR with the fixes applied.

**Workflow:** `.github/workflows/line-width-nag.yml` <!-- link-ignore -->

| Setting  | Value                          |
| -------- | ------------------------------ |
| Name     | Line-width nag                 |
| Schedule | `0 3 * * 0` (Sunday 03:00 UTC) |
| Trigger  | schedule + workflow_dispatch   |
| Runner   | `ubuntu-latest`                |

**Steps:**

1. Checkout via `actions/checkout@v4`.
2. Run `python3 scripts/check-prose-line-width.py --width 100 --report` and capture output.
3. Run Go `lll` check via golangci-lint (placeholder until wired in).
4. Auto-format and open nag PR via `peter-evans/create-pull-request@v7`:
   - Branch: `chore/line-width-nag`
   - Commit message: `[ai][style] auto-fix line-width violations`
   - Labels: `housekeeping`, `style`
   - Delete branch after merge.

The workflow uses `--report` mode so it never blocks other
work. It simply keeps the current state visible via a
standing PR that is easy to review and merge.

### Phase 4: optional CI gate

Add a `check-line-width` job to the main CI workflow, gated
behind a `continue-on-error: true` flag. This makes width
violations visible in PR checks without blocking merges.

When the team is ready to enforce, flip `continue-on-error`
to `false`. The Makefile target `check-prose-width` also
drops its `--report` flag at that point.

Add a `check-line-width` CI job:

| Setting             | Value                                                   |
| ------------------- | ------------------------------------------------------- |
| Runner              | `ubuntu-latest`                                         |
| `continue-on-error` | `true` (advisory until enforced)                        |
| Step                | `python3 scripts/check-prose-line-width.py --width 100` |

### Phase 5: opt-in pre-commit hook

The existing [.pre-commit-config.yaml](../../.pre-commit-config.yaml) already delegates to
`make format-*` targets. Add a new local hook for width
checking that contributors can opt into:

| Setting          | Value                                       |
| ---------------- | ------------------------------------------- |
| Hook ID          | `check-prose-width`                         |
| Entry            | `python3 scripts/check-prose-line-width.py` |
| Language         | `system`                                    |
| Types            | `[markdown]`                                |
| `pass_filenames` | `true`                                      |

This hook runs only on staged Markdown files. Contributors
enable it by running `pre-commit install`. It is not
required: the weekly nag and CI catch anything missed.

For code width, the existing `format-go`, `format-python`,
`format-web`, and `format-mac` hooks already reformat to the
configured width on commit. Once Phase 1 updates the configs,
these hooks enforce 100 columns automatically for anyone who
has pre-commit installed.

## What stays exempt

- **Tables** in Markdown: excluded by the prose linter
- **Lists** in Markdown: excluded by the prose linter
- **Code blocks** in Markdown: excluded by the prose linter
- **Headings** in Markdown: excluded by the prose linter
- **String literals** in Go: configure `lll` to skip them
- **SQL expression width**: separate concern, stays at 70
- **Vendored/minified files**: excluded by directory skips

## Rollout order

```
Phase 1 (config PR)
  ↓
Phase 2 (reformat PR + blame-ignore)
  ↓
Phase 3 (weekly nag workflow)    ← can ship with Phase 1
  ↓
Phase 4 (optional CI gate)       ← flip to enforcing when ready
  ↓
Phase 5 (pre-commit hook entry)  ← can ship with Phase 1
```

Phases 1, 3, and 5 can land in a single PR. Phase 2 is a
separate mechanical reformat. Phase 4 flips a boolean when
the team decides to enforce.
