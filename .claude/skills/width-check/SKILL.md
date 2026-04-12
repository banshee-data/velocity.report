---
name: width-check
description: Advisory prose width check. Reports Markdown prose lines over 100 columns. Never fails CI. Use before merging a docs-heavy PR or as part of release prep.
argument-hint: "[--width 80|100] [path/to/file.md]"
---

# Skill: width-check

Run the prose line-width check across all Markdown files (or a specific file)
and report violations. Advisory only: this never blocks CI.

## Usage

```
/width-check
/width-check README.md
/width-check docs/VISION.md
/width-check --width 80
```

## What it checks

Running prose only. Excluded from the count:

- Fenced code blocks (` ``` `)
- Tables (`|`)
- Headings (`#`)
- Image references (`![`)
- Link definitions (`[label]:`)
- HTML comments (`<!--`)
- Horizontal rules (`---`, `***`)
- Bold metadata lines (`**key:**`)

The default limit is 100 columns. Pass `--width 80` to check against a
stricter limit (useful for ASCII art context).

## Procedure

### 1. Run the check

```bash
make check-prose-width
```

Or directly:

```bash
python3 scripts/check-prose-line-width.py --report
```

For a single file:

```bash
python3 scripts/check-prose-line-width.py --report path/to/file.md
```

For a custom column limit:

```bash
python3 scripts/check-prose-line-width.py --report --width 80
```

### 2. Triage the output

For each violation, determine the category:

| Category                                  | Action                                                                                                                       |
| ----------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Long URL in prose                         | Acceptable. Move the URL to a link reference if it clutters the line.                                                        |
| List item that cannot wrap                | Acceptable. List items are excluded anyway; if it still appears, it is probably a prose paragraph that starts with a hyphen. |
| Blockquote prose line                     | Fix: Prettier does not wrap blockquotes — break manually.                                                                    |
| Genuine overlong sentence                 | Fix: break the sentence or shorten it. Do not insert a hard newline mid-sentence purely to hit the column limit.             |
| Pre-formatted prose in a non-fenced block | Fix: either wrap or fence the block.                                                                                         |

### 3. Fix genuine violations

Run `make format-docs` first: Prettier wraps most prose automatically at 100
columns. For anything Prettier did not catch (blockquotes, certain list items),
fix manually.

After fixing:

```bash
make check-prose-width
```

Confirm the violation count dropped.

### 4. Report

State how many violations were found, how many were acceptable (URLs, special
contexts), and how many were fixed. Note any files that consistently produce
violations: those may warrant a prose tightening pass.

## Notes

- `make check-prose-width` is advisory. It is included in `/docs-release-prep`
  as step 6 but is deliberately excluded from `make lint` and CI.
- To promote it to a hard gate, add `check-prose-width` to the `lint-docs`
  target in the Makefile. That is a deliberate decision deferred until the
  existing prose is fully compliant.
- ASCII art lives in fenced code blocks and is therefore excluded from all
  width checks.
