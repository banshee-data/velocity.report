---
name: fix-links
description: Fix stale backtick-quoted paths and dead Markdown links across the repo. Repairs what can be determined automatically; surfaces ambiguous cases to the operator.
argument-hint: "[optional: file or directory to limit scope]"
---

# Skill: fix-links

Audit all Markdown files for dead Markdown links and stale `` `backtick/paths` ``,
fix those that can be resolved automatically, and surface anything ambiguous
to the operator with a clear decision prompt.

## Usage

```
/fix-links
/fix-links .claude/agents/
/fix-links docs/plans/some-plan.md
```

## Procedure

### 1. Run both link checkers

```bash
python3 scripts/check-relative-links.py --report 2>&1
python3 scripts/check-backtick-paths.py --report 2>&1
```

Collect every finding into a working list. If both checkers report clean,
print "All links OK: nothing to fix." and stop.

### 2. Triage each finding

For every item apply this decision tree in order:

**a. Placeholder or template token**: skip silently.
Examples: entries like `linked-plan-name.md` or `other-plan.md`
in `docs/plans/TEMPLATE.md`. These are intentional scaffolding.

**b. The file moved: unique match exists**; fix automatically.
Search the repo for the bare filename:

```bash
find . -name "<filename>" \
  ! -path "*/node_modules/*" ! -path "*/.venv/*" \
  ! -path "*/.git/*" ! -path "*/.build/*" \
  ! -path "*/DerivedData/*"
```

If exactly one result is found, compute the correct relative or repo-root path
and apply the edit. Record it in the **Fixed** list.

**c. Multiple candidates exist**: surface to operator (see §3).

**d. No match anywhere in the repo**: surface to operator (see §3).

**e. The reference is to a file that should exist but has not been created yet**
(e.g. a plan references a future schema file): surface to operator as
"referenced file not yet created".

### 3. Surface ambiguous cases

After exhausting automatic fixes, print a **Needs operator decision** section
formatted as a numbered list. For each item include:

- The file and line number
- The broken reference as written
- What was searched for and what (if anything) was found
- A concrete question: "Should this link be removed, updated to X, or left as-is?"

Example format:

```
## Needs operator decision

1. docs/lidar/architecture/vector-scene-map.md:13
   Reference: `../lidar/architecture/ground-plane-extraction.md`
   Searched for: ground-plane-extraction.md
   Found: (no match anywhere in repo)
   Question: Has this document been deleted, renamed, or not yet written?
   Options: (a) remove the reference, (b) update to the new path, (c) leave as a known gap

2. data/structures/MATRIX.md:17
   Reference: `../../.github/agents/matrix-tracer.agent.md`
   Searched for: matrix-tracer.agent.md
   Found: (no match anywhere in repo)
   Question: Was matrix-tracer.agent.md removed intentionally?
   Options: (a) remove the reference, (b) create the file, (c) update to the replacement path
```

Wait for the operator to answer each item before editing those files.

### 4. Apply operator decisions

Once the operator provides answers, apply the agreed edits immediately.
Then re-run both checkers to confirm the count is reduced:

```bash
python3 scripts/check-relative-links.py --report 2>&1
python3 scripts/check-backtick-paths.py --report 2>&1
```

### 5. Report

Print a summary:

```
## Fix-links summary

- Fixed automatically:  N
- Skipped (templates):  N
- Resolved by operator: N
- Remaining (deferred): N

Run `make lint-docs` to confirm the full quality gate passes.
```

## What counts as fixable automatically

| Pattern                                                                     | Action                                                  |
| --------------------------------------------------------------------------- | ------------------------------------------------------- |
| File moved, unique match                                                    | Update path to new location                             |
| Path root is wrong (e.g. `.github/TENETS.md` exists but ref uses bare name) | Update to the location that exists <!-- link-ignore --> |
| Backtick path uses wrong prefix depth                                       | Recompute correct relative path from file location      |
| Markdown link and backtick reference to same dead target in same file       | Fix both in one edit                                    |

## What is never auto-fixed

- Any reference where two or more candidate files exist
- References to files that do not exist anywhere in the repo
- Any edit that would change semantic meaning (e.g. a link pointing to a
  different concept just because the filename is similar)

## Suppressing known-stale references

Add `<!-- link-ignore -->` at the end of any line to permanently suppress it
from both `check-relative-links.py` and `check-backtick-paths.py`.

Use this for:

- Intentional template placeholders (e.g. `docs/plans/TEMPLATE.md`)
- Planned-but-not-yet-created files referenced in plan docs
- Historical paths in completed plan docs that describe what was moved
- References to the other platform's file layout conventions

```
| Image CI workflow | `.github/workflows/build-image.yml` | Planned |
3. [x] Migrated from `../server/` → `../platform/` <!-- link-ignore -->
```
