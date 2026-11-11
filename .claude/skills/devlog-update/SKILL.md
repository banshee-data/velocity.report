---
name: devlog-update
description: Record reviewed development updates as per-PR fragments without shared journal edits.
argument-hint: ""
---

# Skill: devlog-update

Record the current branch's work in unique fragments under `docs/devlog-fragments/pending/`.
Follow [the canonical fragment workflow](../../../docs/devlog-fragments/README.md).
Do not edit `docs/DEVLOG.md` in feature PRs.

## Procedure

1. Read `AGENTS.md`, `.github/STYLE.md`, and `docs/devlog-fragments/README.md`.
2. Inspect the branch diff against `origin/main`, its commits, and existing fragments on this
   branch. Summarise the work being delivered by this PR, not unrelated open branches.
3. Add a descriptive, unique filename with a short random suffix under `docs/devlog-fragments/pending/`.
   Update an existing fragment only if it is new in this same PR. Never edit a fragment already
   on `main`; add another fragment for follow-up work.
4. Write concise, past-tense, single-line bullets in British English. Describe what changed and
   why. Include report links, numerical findings, limitations, and decisions where material.
   Use inline links relative to the fragment. Omit dates, branch tags, and trailing PR numbers.
5. Compare with the PR's other fragments to avoid duplicates. Run `make check-devlog`.
6. Commit the fragment alongside the implementation or report it records. For work with no
   journal value, explain `<!-- devlog-skip: reason -->` in the PR body instead.

## Consolidation

The Monday/Thursday helper owns routine updates to `docs/DEVLOG.md`. It folds landed fragments
using UTC merge dates and PR numbers, and deletes consumed fragments in the same PR.
For an explicitly requested manual consolidation, use `make devlog-consolidate` from a clean,
fully fetched `main` checkout with authenticated `gh`. Review the diff and follow the guide's
maintenance PR procedure. Do not synthesise or rewrite historical entries during consolidation.
