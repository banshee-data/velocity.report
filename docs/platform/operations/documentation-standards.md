# Documentation standards

Rules and conventions for Markdown documentation across the velocity.report repository, covering metadata format, structure, and prose style.

Active plans:

- [platform-documentation-standardisation-plan.md](../../plans/platform-documentation-standardisation-plan.md)
- [line-width-standardisation-plan.md](../../plans/line-width-standardisation-plan.md)

## Metadata format

All docs under `docs/` use the `- **Key:** value` canonical metadata format.
Enforced by `scripts/check-doc-header-metadata.py` via `make lint-docs`.

### Key normalisation (applied)

- `Layer` → `Layers`
- `Related variants` → `Related`
- `Last updated` → banned (see below)

### Banned date keys

Date metadata is explicitly banned: `Created`, `Date`, `Last Updated`,
`Original Design Date`. Enforced via `BANNED_DATE_KEYS` and
`RE_KEY_DATE_SUFFIX` in the metadata checker.

## Structure rules

1. Capability docs remain under `docs/lidar/` and `docs/radar/`.
2. Client docs remain under `docs/ui/`.
3. Data science references live under `data/`, with stable maths docs in
   `data/maths/` and proposals in `data/maths/proposals/`.
4. Execution work remains under `docs/plans/`.
5. Root keeps only governance/reference docs (`README`, `COVERAGE`, `DEVLOG`).

## Opening paragraph rule

Every doc must have an opening paragraph after the `# Title` heading.
Source order:

1. Existing opening summary paragraph in the same file.
2. `Overview` / `Goal` / `Summary` / `Objective` section lead paragraph.
3. Main-branch equivalent file's opening narrative paragraph.
4. Manual editor-written summary (only when none of the above exists).

Constraints:

- One or two sentences describing document coverage.
- Must be narrative text, not filename echoes, status labels, or changelog
  fragments.

**Status:** ~40 of ~123 docs still missing a narrative opening paragraph.
No automated checker exists yet.

## Line-Width standard: 100 columns

100 is the single line-width standard across all code and documentation.

### Data supporting the choice

| Language | Files | ≤100 compliance |
| -------- | ----: | --------------: |
| Go       |   428 |           98.9% |
| Python   |    72 |           99.6% |
| TS/JS    |    44 |           99.8% |
| Svelte   |    17 |           99.2% |
| Swift    |    35 |           99.7% |

Three of five formatters already default to 100. Every language reaches
≥98.9% compliance at this width.

### Formatter configuration

| File                                | Setting                |
| ----------------------------------- | ---------------------- |
| `scripts/check-prose-line-width.py` | `DEFAULT_WIDTH = 100`  |
| `pyproject.toml`                    | `line-length = 100`    |
| `.golangci.yml`                     | `lll: line-length 100` |
| `web/.prettierrc`                   | Already 100            |
| `.swift-format`                     | Already 100            |
| `.sql-formatter.json`               | Stays at 70 (expr)     |

### Exemptions

- Tables, lists, code blocks, and headings in Markdown
- String literals in Go (`lll` configured to skip)
- SQL expression width (separate concern, stays at 70)
- Vendored/minified files

### Excluded files

Two files are excluded from prose-width checking entirely:

- **`CHANGELOG.md`** — almost entirely list items (already excluded by
  the checker); the few remaining prose lines are release intro paragraphs
  that read better long.
- **`docs/DECISIONS.md`** — single-line register entries with dense link
  references; wrapping them hurts scanability more than it helps
  readability.

### Enforcement rollout

1. **Phase 1**: Config changes (no source reformatting)
2. **Phase 2**: Mechanical reformat + `.git-blame-ignore-revs`
3. **Phase 3**: Weekly nag PR via GitHub Actions
4. **Phase 4**: Optional CI gate (`continue-on-error: true`, then `false`)
5. **Phase 5**: Opt-in pre-commit hook

## Validation gates

Run on every docs refactor:

1. Link integrity: `check_docs_links.sh`
2. Opening paragraph presence
3. No placeholder values (no filename echoes or status labels)
4. Drift report: list files using main-derived paragraph vs manual fallback

## CI integration

- Weekly lint-autofix workflow (Monday 06:00 UTC) runs `--fix` mode.
- `make lint-docs` (check) and `make format-docs` (fix) Makefile targets.
- Standard documented in `.github/knowledge/coding-standards.md`
  § Documentation Metadata.

## Edit governance

1. Do not run blanket rewrite scripts without dry-run output and approval.
2. Batch edits must include a candidate report before writes.
3. Metadata automation must skip files where candidate text is non-narrative.

## Error message voice

Consistent, humane voice across all user-facing error messages: Go server,
web frontend, Python tools, and shell scripts.

### Principles

1. **Sentence case**: error messages start with a capital, do not end with a
   full stop.
2. **Name the problem, not the user**: "Cannot open database" rather than
   "You provided an invalid path".
3. **Include a next step**: where possible, tell the reader what to try.
4. **Diagnostic hints** use `: try X` suffix or `\nTry: X` on a new line.

### Scope

The voice audit covers every user-visible string: HTTP error responses,
CLI `log.Fatalf` messages, web UI toast/error components, PDF generator
warnings, and shell script output.
