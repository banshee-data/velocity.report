# Repository instructions

Read [CLAUDE.md](CLAUDE.md) for project commands and
[the coding standards](.github/knowledge/coding-standards.md) for repository conventions.
These instructions apply to every agent working in this repository.

## Keep experiment findings in Git and raw outputs local

Commit detailed, human-readable experiment reports in `docs/`. Preserve the question, methods,
arm definitions, numerical findings (including per-case summaries where they affect the decision),
interpretation, limitations, frozen provenance, and recommendations. Reports must distinguish
observed measurements from physical validation and explain what the evidence does not establish.
Link reports from the relevant README, plan, and development-log entry. A report is part of the
engineering record, not disposable output; do not remove it when cleaning up raw results.

Never commit raw experiment, replay, analysis, or benchmark outputs on any branch or PR. Recordings,
row-level data, generated metric/configuration dumps, runtime logs, coverage output, database
snapshots, and campaign state belong outside the checkout or in ignored local output directories.
Do not attach raw output to a report or rename it into `docs/` to evade this rule. Edited summary
tables and numerical findings that explain the experiment belong in the written report.

Directories named `results` at any depth are local output only. Never stage them, create a raw
results branch, or bypass the ignore rule with `git add -f`. Keep reusable implementation and
hand-authored test fixtures in their normal source locations. Reports must be readable without
links to untracked repository output; describe archived evidence and retain provenance identifiers
without importing raw artefacts.

Before committing or pushing, run `make check-no-results` and inspect the staged diff. The guard
rejects `results` directories; review must also catch raw outputs placed elsewhere. Fix a failing
check by removing output from the index while preserving needed local evidence. When a report is
mixed into an output directory, move and edit the report into `docs/` before removing that directory
from Git. For a branch that has already committed raw results, remove them from the proposed
commits as well as its final tree before publishing the PR. Do not rewrite shared main history.
