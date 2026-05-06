---
name: style-fix
description: Run a focused STYLE.md conformance pass across docs, auto-fix safe issues, honour per-file ignore markers, and report anything that still needs editorial judgement.
argument-hint: "[optional: path] [--ignore-plans] [--check-only]"
---

# Skill: style-fix

Audit Markdown prose against `.github/STYLE.md`, apply safe automatic fixes,
and report anything that still needs a human sentence rather than a blind
substitution.

## Usage

```bash
/style-fix
/style-fix docs --ignore-plans
/style-fix docs/ui --check-only
```

## Scope and ignore markers

- Default scope: `docs/`
- Optional path argument narrows the scan to a file or subdirectory
- `--ignore-plans` skips `docs/plans/**`
- `<!-- ignore-style -->` anywhere in a file: skip the file entirely
- `<!-- ignore-style-length -->` anywhere in a file: skip only the 800-line check

Use `<!-- ignore-style -->` for files whose format is intentionally hostile to
mechanical prose cleanup, such as the decisions register.

Use `<!-- ignore-style-length -->` for files that are intentionally long but
still valid for their role, such as an ongoing log.

## What this skill checks

### 1. British English

Look for the explicit STYLE.md pairs in prose outside fenced code blocks:

- `analyse`, not `analyze`
- `behaviour`, not `behavior`
- `centre`, not `center`
- `colour`, not `color`
- `licence` (noun), not `license`
- `metre`, not `meter`
- `neighbour`, not `neighbor`
- `organisation`, not `organization`
- `travelled`, not `traveled`
- `visualisation`, not `visualization`

Do not rewrite external API names, CSS properties, CLI flags, file paths, or
quoted identifiers that intentionally use American spelling.

### 2. Punctuation

Find em dashes in prose outside fenced code blocks.

Prefer:

- a colon for consequence or explanation
- a comma for a short aside
- a full stop when the clause deserves its own sentence

Do not rewrite mathematical arrows, ASCII diagrams, table separators, or code.

### 3. Machine timestamps

Find machine-style timestamps in docs prose and examples. Convert them to UTC
ISO 8601 with trailing `Z` when the text is presenting a machine-written value.

Examples:

- `2025-03-02 00:00:00 UTC` -> `2025-03-02T00:00:00Z`
- `2025-03-02 00:00:15 UTC` -> `2025-03-02T00:00:15Z`

Do not rewrite human-readable dates such as `April 7, 2026`.

### 4. Length target

Report Markdown files over 800 lines unless they contain
`<!-- ignore-style-length -->`.

Length is a report-only check. Do not auto-split files inside this skill.

### 5. Banned code-block languages

Report fenced blocks in docs that use languages banned by STYLE.md, such as
`go`, `sql`, `json`, `yaml`, `typescript`, `swift`, `python`, `html`, `css`,
`latex`, or `makefile`.

This skill may rewrite a small block into prose only when the replacement is
obvious and local. If the block carries real design content, report it for
manual editorial work instead of guessing.

## Procedure

### 1. Build the file list

Start from the requested path or `docs/` by default.

- Exclude `docs/plans/**` when `--ignore-plans` is present
- Skip any file containing `<!-- ignore-style -->`

### 2. Run the checks

Check prose outside fenced code blocks for:

- British-English violations
- em-dash usage
- machine timestamps without trailing `Z`
- length over 800 lines, unless `<!-- ignore-style-length -->` is present
- banned fenced code-block languages

### 3. Apply safe fixes

Apply only low-risk, semantics-preserving edits automatically:

- obvious noun spelling fixes such as `license plates` -> `licence plates`
- obvious punctuation swaps from em dash to colon, comma, or full stop when the
  sentence structure is unambiguous
- obvious machine timestamp rewrites to ISO 8601 with trailing `Z`
- adding ignore markers when the operator explicitly asked for them

Do not auto-fix:

- dense decision registers where punctuation changes would touch the whole file
- any sentence where choosing between colon, comma, parentheses, or split
  sentence requires judgement
- anything inside fenced code blocks

### 4. Re-run focused validation

After edits, re-run the same checks on the touched files.

If links were edited, also run:

```bash
python3 scripts/check-relative-links.py
```

### 5. Report

Print a summary with four buckets:

- Fixed automatically
- Ignored by `<!-- ignore-style -->`
- Ignored for length by `<!-- ignore-style-length -->`
- Needs editorial decision

For every unresolved item, include the file, line, the exact text, and the
smallest concrete question needed to decide the rewrite.

## Output pattern

```text
## Style-fix summary

- Fixed automatically: N
- Ignored entirely: N
- Ignored for length: N
- Needs editorial decision: N
```
