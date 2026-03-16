# Documentation Standardisation Plan

- **Status:** In Progress — metadata and structure rules enforced, opening paragraph gap remains
- **Layers:** Cross-cutting (documentation)

Controlled process to stabilise documentation structure and metadata quality by reconciling branch drift against main, preserving authoritative summaries, and enforcing repeatable review gates.

### Completion Checklist

- [x] Contract defined (this document)
- [x] Metadata format standardised — `- **Key:** value` canonical format enforced across 86+ files
- [x] Key normalisation — Layer→Layers, Related variants→Related, Last updated→Last Updated (57→49 unique keys)
- [x] Date metadata removed — Created, Date, Last Updated, Original Design Date purged from 15 files
- [x] Date enforcement linter — `BANNED_DATE_KEYS` and `RE_KEY_DATE_SUFFIX` in `check-doc-header-metadata.py`
- [x] Summary deduplication — resolved 1 file with both `- **Summary:**` bullet and `## Summary` heading
- [x] Structure rule compliance — zero misplaced docs, all categories correctly organised
- [x] CI integration — weekly lint-autofix workflow (Monday 06:00 UTC) runs `--fix` mode
- [x] Makefile integration — `lint-docs` (check) and `format-docs` (fix) targets
- [x] Standard documented — `coding-standards.md` § Documentation Metadata
- [ ] Opening paragraph rule — ~40 of ~123 docs still missing a narrative opening paragraph
- [ ] Opening paragraph validation gate — no automated checker exists yet
- [ ] Placeholder/filename-echo detector — §7.3 not implemented
- [ ] Link integrity gate in CI — §7.1 references `/tmp/check_docs_links.sh` (not permanent)
- [ ] Main-branch drift reconciliation — §3 not executed
- [ ] Drift report — §7.4 not implemented

## 1. Objective

Reduce documentation churn and prevent low-signal edits by applying one repeatable contract for structure, metadata, and migration behaviour.

## 2. Scope

- All markdown files under `docs/`.
- Structure governance for hubs: `lidar`, `radar`, `ui`, `maths`, `plans`.
- Document structure governance (opening paragraph, optional `**Status:**` metadata).

## 3. Baseline Reconciliation (Main-First)

1. Compare every doc against `main` using rename-aware mapping.
2. If a file differs only in metadata header lines, restore body content from `main`.
3. If body content diverges materially, keep current content and manually resolve metadata from in-document summary sections.

## 4. Opening Paragraph Rule

Every doc must have an opening paragraph after the `# Title` heading. Source order:

1. Existing opening summary paragraph in the same file.
2. `Overview` / `Goal` / `Summary` / `Objective` section lead paragraph.
3. Main-branch equivalent file's opening narrative paragraph.
4. Manual editor-written summary only when none of the above exists.

Constraints:

- One or two sentences describing document coverage.
- Must be narrative text, not filename echoes, status labels, or changelog fragments.
- Bold `**Status:**` metadata is optional — use only on docs that track implementation progress.

## 5. Structure Rule

1. Capability docs remain under `docs/lidar` and `docs/radar`.
2. Client docs remain under `docs/ui`.
3. Data science references live under `data/`, with stable maths docs in `data/maths` and proposals in `data/maths/proposals`.
4. Execution work remains under `docs/plans`.
5. Root keeps only governance/reference docs (`README`, `COVERAGE`, `DEVLOG`).

## 6. Edit Governance

1. Do not run blanket rewrite scripts across all docs without dry-run output and approval.
2. Batch edits must include a candidate report before writes.
3. Any metadata automation must skip files where candidate text is non-narrative (`Status:`, `Date:`, Q/A labels, note blocks).

## 7. Validation Gates

Run on every docs refactor:

1. Link integrity: `/tmp/check_docs_links.sh`.
2. Opening paragraph presence: every doc has a narrative opening paragraph after the title.
3. No placeholder values: opening paragraphs must not echo the filename or contain status labels.
4. Drift report: list files using main-derived opening paragraph vs manual fallback.

## 8. Execution Steps

1. Freeze non-essential docs edits.
2. Reconcile metadata-only drifts against `main`.
3. Repair metadata fields from in-doc summaries where needed.
4. Run validation gates and publish audit output.
5. Resume normal docs work under this contract.
