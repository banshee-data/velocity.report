# Repository instructions

Read [CLAUDE.md](CLAUDE.md) for project commands and
[the coding standards](.github/knowledge/coding-standards.md) for repository conventions.
These instructions apply to every agent working in this repository.

## Keep results out of Git

Never commit experiment, replay, analysis or benchmark results. Generated reports, raw recordings,
derived datasets, runtime logs, coverage output, database snapshots and campaign state belong
outside the checkout or in ignored local output directories. This applies to every branch and PR,
not only main.

Directories named `results` at any depth are local output only. Never stage them, create a results
branch, or bypass the ignore rule with `git add -f`. Keep reusable implementation and hand-authored
test fixtures in their normal source locations; do not rename generated results to evade this rule.
Durable explanations, methods and conclusions may be written in documentation without embedding
raw result artefacts or linking to untracked repository output.

Before committing or pushing, run `make check-no-results` and inspect the staged diff. A failing
check must be fixed by removing output from the index while preserving needed local evidence.
For a branch that has already committed results, remove them from the proposed commits as well as
its final tree before publishing the PR. Do not rewrite shared main history.
