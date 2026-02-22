# Documentation Standardization Plan

Status: Planned
Purpose: Defines a controlled process to stabilize documentation structure and metadata quality by reconciling branch drift against main, preserving authoritative summaries, and enforcing repeatable review gates.

## 1. Objective

Reduce documentation churn and prevent low-signal edits by applying one repeatable contract for structure, metadata, and migration behavior.

## 2. Scope

- All markdown files under `docs/`.
- Structure governance for hubs: `lidar`, `radar`, `ui`, `maths`, `plans`.
- Metadata governance for `Status` and `Purpose`/`Summary`.

## 3. Baseline Reconciliation (Main-First)

1. Compare every doc against `main` using rename-aware mapping.
2. If a file differs only in metadata header lines, restore body content from `main`.
3. If body content diverges materially, keep current content and manually resolve metadata from in-document summary sections.

## 4. Metadata Source Rule

Use this precedence for metadata text:

1. Existing opening summary paragraph in the same file.
2. `Overview` / `Goal` / `Summary` / `Objective` section lead paragraph.
3. Main-branch equivalent file’s opening narrative paragraph.
4. Manual editor-written summary only when none of the above exists.

Constraints:

- Exactly one of `Purpose` or `Summary`.
- 20-30 words.
- Must describe document coverage, not filename, status labels, or changelog fragments.

## 5. Structure Rule

1. Capability docs remain under `docs/lidar` and `docs/radar`.
2. Client docs remain under `docs/ui`.
3. Maths docs remain under `docs/maths` with proposals in `docs/maths/proposals`.
4. Execution work remains under `docs/plans`.
5. Root keeps only governance/reference docs (`README`, `COVERAGE`, `DEVLOG`).

## 6. Edit Governance

1. Do not run blanket rewrite scripts across all docs without dry-run output and approval.
2. Batch edits must include a candidate report before writes.
3. Any metadata automation must skip files where candidate text is non-narrative (`Status:`, `Date:`, Q/A labels, note blocks).

## 7. Validation Gates

Run on every docs refactor:

1. Link integrity: `/tmp/check_docs_links.sh`.
2. Metadata presence: every doc has `Status` and one of `Purpose`/`Summary`.
3. Metadata quality: 20-30 words, no filename-only values, no `Date:` metadata line.
4. Drift report: list files using main-derived metadata vs manual fallback.

## 8. Execution Steps

1. Freeze non-essential docs edits.
2. Reconcile metadata-only drifts against `main`.
3. Repair metadata fields from in-doc summaries where needed.
4. Run validation gates and publish audit output.
5. Resume normal docs work under this contract.
