# Development log fragments

Feature PRs add a uniquely named Markdown file in `pending/`, for example
`velocity-prior-shadow-a81c.md`. Do not update `docs/DEVLOG.md` or this index per PR.
The fragment is the reviewed engineering record until consolidation.

Write one or more past-tense bullets explaining what changed, why, and the evidence or
limitations that matter. Preserve experiment findings and report links. Each bullet occupies one
physical line. Use British English, no headings, dates, branch tags, or trailing PR numbers.
Use ordinary inline Markdown links relative to the fragment file; URL-encode spaces and parentheses
in destinations. The helper relocates these links when folding the fragment into DEVLOG.

For example:

```markdown
- Fixed the tracker gate and documented the measured limits.
```

Use a descriptive slug plus a short random suffix to avoid filename collisions. More than one
fragment per PR is allowed. Add new fragments for follow-up work; do not edit pending fragments
that have already landed. Existing open PRs should move their proposed DEVLOG additions into
fragments when rebasing. Leave historical entries and legacy branch tags in DEVLOG alone.

## Validation and exemptions

Run `make check-devlog` locally to validate fragment syntax. PR CI also requires a new fragment
and rejects direct DEVLOG edits or changes to existing fragments. For changes with no journal
value, explain in the PR body using `<!-- devlog-skip: reason -->`. This exemption does not permit
DEVLOG edits. Maintainers can apply the `devlog-maintenance` label for journal corrections,
workflow migrations, or consolidation. Label changes re-run the check.

## Consolidation

A GitHub Actions helper runs Monday and Thursday at 06:30 UTC and supports manual dispatch.
It checks out `main`, runs `make devlog-consolidate`, and maintains one PR on
`ci/devlog-consolidation`. Only that PR edits DEVLOG during normal operation.

The helper finds each fragment's introducing commit and its merged PR through GitHub's API.
It uses the UTC merge date, adds the PR number, adjusts relative inline links, and inserts bullets
under one daily heading. Existing headings retain their titles; new days use “Engineering updates”.
It preserves reviewed wording and historical entries. It stops before writing if provenance is
missing or ambiguous, including direct pushes with no associated PR. Resolve these cases in a
maintenance PR; do not guess dates or PR numbers.

DEVLOG additions and deletion of exactly the consumed fragments share one commit. A rerun on
unchanged `main` produces the same patch; a rerun after merging finds nothing to repeat. New
fragments arriving after checkout wait for the next snapshot. The job serialises runs, and updates
its existing PR from current `main`. Review and merge that PR normally. No automatic merging.

The GitHub repository must allow Actions to create pull requests. The fragment gate recognises
consolidation PRs by the Actions bot author and fixed branch. The maintenance label is only needed
for manual corrections or migrations. PRs created with `GITHUB_TOKEN` do not
normally trigger other PR workflows; the scheduled job therefore runs the fragment tests and
output guard itself. Maintainers should run any additional required checks before merging, or
configure an approved GitHub App token if automatic PR check dispatch is needed.

To reproduce locally, start from a clean, fully fetched checkout of `main` with authenticated
`gh`, then run `make devlog-consolidate`. Inspect the diff and commit both DEVLOG and fragment
deletions together. The script does not stage, commit, push, or open a PR locally.
