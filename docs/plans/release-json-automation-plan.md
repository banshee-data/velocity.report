# Release-JSON automation and `main` protection

- **Status:** Draft
- **Layers:** Cross-cutting (CI, branch protection, release workflow)
- **Target:** after v0.5.2; unblocks single-PR release flow while tightening `main`
- **Companion plans:** [Version-bump consolidation](version-bump-consolidation-plan.md); [Deploy: rpi-imager fork](deploy-rpi-imager-fork-plan.md); [Deploy: distribution packaging](deploy-distribution-packaging-plan.md)
- **Canonical:** this document

## Motivation

Every release takes two PRs:

1. **PR 1 — version bump.** Merges to `main`, triggers tag `vX.Y.Z`, which triggers `.github/workflows/build-image.yml`. The workflow publishes radar binaries, the `.img.xz` image, and (for stable tags) the macOS visualiser `.dmg` to the GitHub Release.
2. **PR 2 — asset metadata.** A human runs `scripts/update-release-json.py --ci --channel <stable|prerelease>` locally, which fetches the newly-published assets, computes SHA256 (and `extract_sha256` for the RPi image), and rewrites:
   - `public_html/src/_data/release.json` (consumed by the Eleventy site and by `/rpi.json` for the RPi Imager repo)
   - `image/os-list-velocity.json` (the RPi Imager OS list)
     The PR is then opened and merged.

PR 2 exists only because the SHA256s don't exist until the tag has run the build workflow, and because the current ruleset on `main` prevents `build-image.yml` from committing the answer back. Removing PR 2 requires changing how the workflow interacts with `main`.

Separately, we want to tighten `main` so that — once the project has more contributors — merges to `main` require at least one approving review. Today the ruleset sets `required_approving_review_count: 0`, which is fine for a solo-ish project but not for a broader contributor base.

These two goals are in tension: raising the approval floor makes the "single-PR release" harder, not easier, unless we carve out a well-scoped exception for release-metadata updates.

## Current state

### Ruleset on `main` (id `6362131`, "protect main")

```
enforcement:       active
conditions:        refs/heads/main
bypass_actors:     [] (current_user_can_bypass: never)
rules:
  - deletion
  - non_fast_forward
  - required_linear_history
  - pull_request
      required_approving_review_count:    0
      dismiss_stale_reviews_on_push:      false
      require_code_owner_review:          false
      require_last_push_approval:         false
      required_review_thread_resolution:  true
      allowed_merge_methods:              [squash]
  - copilot_code_review (review_on_push: false)
repo-level:
  web_commit_signoff_required:  true
  allow_auto_merge:             true
```

No `CODEOWNERS` file exists anywhere in the repo.

### Release-asset plumbing

- `build-image.yml` already distinguishes prereleases from stable by tag pattern: `contains(github.ref_name, '-')` is the canonical test (matches `v0.5.1-pre6`, not `v0.5.0`). This flag is passed to both `gh release create --prerelease` and `softprops/action-gh-release`.
- `scripts/update-release-json.py` already accepts `--channel {stable,prerelease,both}`, `--ci` (picks `linux_arm64`, `mac_arm64`, `rpi_image` — the three CI-built artefacts), and `--tag vX.Y.Z` (pin to a specific release). It writes both JSON files atomically; any mid-flight error leaves them untouched.
- `release.json` carries two channels (`stable`, `prerelease`) for `linux_arm64` / `mac_arm64` / `visualiser`, plus a single top-level `rpi_image` slot. `os-list-velocity.json` mirrors the `rpi_image` fields into its first `os_list[0]` entry.

## Constraints GitHub imposes

Several things that seem natural are not currently possible and shape every option below.

1. **Rulesets cannot be path-scoped.** A branch ruleset applies to every file in the branch. There is no "require 0 approvals when only these two files change, otherwise require 1" dial. Path conditions exist on org-level rulesets for repository properties, not for per-file rules within a branch ruleset.
2. **`GITHUB_TOKEN` cannot approve pull requests.** The PR-review API rejects `GITHUB_TOKEN`-authored approvals. Path-scoped auto-approval therefore needs either a personal access token on a machine user, or a second GitHub App acting as the approver (distinct from whichever identity opened the PR, because no identity can approve its own PR).
3. **`GITHUB_TOKEN`-authored PRs do not trigger other workflows.** If the release workflow opens PR 2 using the default token, required status checks never run and automerge stalls. A GitHub App token (via `actions/create-github-app-token`) or a PAT is required for the PR to be treated as a normal contributor PR by CI.
4. **Bypass is branch-level, not path-level.** Ruleset `bypass_actors` exempts an identity from _all_ rules on that branch, not just "these two files." A bypass actor can push arbitrary changes to `main`.
5. **`web_commit_signoff_required: true`** means any commit pushed via the GitHub web API (including an App commit) needs a `Signed-off-by:` trailer. This is a small concern — both Apps and the GitHub API accept arbitrary commit messages — but it must be honoured by whatever workflow commits release metadata.

## Proposed protection changes (applies to all three options)

Raise the floor on `main` and give named admins an escape hatch:

- `required_approving_review_count: 1` (up from 0)
- `require_last_push_approval: true` (re-approve if new commits land after approval)
- Add `bypass_actors`:
  - Role: `Repository admin` — `bypass_mode: always`
  - Optionally a named team (e.g. `@banshee-data/maintainers`) — same mode
- Keep `required_linear_history`, `non_fast_forward`, `deletion`, `copilot_code_review`, squash-only, review-thread resolution as-is.

Effect: contributors open PRs that need one approving review before merge. Admins can push directly when they need to (hotfixes, release metadata, ruleset debugging) without waiting on a reviewer.

The remaining question is how the release-metadata update relates to the "1 approval" floor. That is what the three options below address.

## Options

### Option A — Auto-PR with path-scoped auto-approval, then automerge

The release workflow opens PR 2 itself, a separate identity approves it if and only if the diff is restricted to a well-defined allow-list, and GitHub's automerge closes it out.

**Flow**

1. After `build-image.yml` finishes uploading assets to the release, a new job `update-release-json` runs:
   - Determines the channel from the tag (`contains(github.ref_name, '-') ? prerelease : stable`).
   - Mints a GitHub App installation token (App #1: `velocity-release-bot`, `contents:write` + `pull_requests:write`) via `actions/create-github-app-token`.
   - Checks out `main`, runs `scripts/update-release-json.py --ci --channel <channel> --tag $GITHUB_REF_NAME`.
   - Commits the two JSON files on a branch `auto/release-json-${{ github.ref_name }}` with a `Signed-off-by:` trailer, pushes it.
   - Opens a PR titled `[ci] update release.json for <tag>` with a body that names the tag, channel, and the exact SHA256s computed.
2. A second workflow (`auto-approve-release-json.yml`) triggers on `pull_request_target` for PRs authored by `velocity-release-bot[bot]`:
   - Fetches the PR's changed files via the API.
   - Rejects if the set is not a subset of `{public_html/src/_data/release.json, image/os-list-velocity.json}`.
   - Rejects if either file's diff touches any key outside the expected schema (optional: run a JSON-diff check comparing the new values against the GitHub Release API a second time, so the approver independently re-derives the SHA256s rather than trusting the PR body).
   - If checks pass, approves the PR using App #2 (`velocity-release-approver`, `pull_requests:write`) and enables `--auto --squash` merge.
3. Required CI runs against the PR (public_html lint, markdown lint, etc.). When they pass, automerge squash-merges the PR.

**What this requires**

- Two GitHub Apps installed on the repo: one to open PRs, one to approve. (Apps cannot review their own PRs.)
- A `CODEOWNERS` entry optionally mapping the two JSON paths to a `@banshee-data/release-bots` team containing both Apps, if we later turn on `require_code_owner_review: true`. Not required for the 1-approval floor.
- The approver workflow's path-allow-list logic must be defensive: compute `added + modified + removed + renamed`, not just `modified`.
- `pull_request_target` is used (not `pull_request`) so the approver workflow runs with repo secrets even when the PR branch is not in `main`. Standard care: the workflow must not `checkout` the PR ref and execute code from it. It only needs the API.

**What it buys**

- The `main` ruleset stays strict: 1 approval required for every merge, including the bot's. The bot is not a bypass actor; it's a contributor whose PRs happen to meet a narrow auto-approval policy.
- Full audit trail in the PR list. Every release-metadata change is a merge commit on `main` with CI evidence.
- Admins retain direct-push bypass for emergencies via the new `bypass_actors` entries.
- Rollback is a revert PR like any other.

**What it costs**

- Two GitHub Apps to create, install, and manage secrets for (App private keys rotate; losing one blocks releases until replaced).
- One extra workflow (`auto-approve-release-json.yml`) to maintain — and the auto-approval logic is security-sensitive. If a contributor ever gets the bot to open a PR containing arbitrary changes (e.g. by controlling the tag), the approver must refuse. The path-allow-list is the primary defence; the schema re-derivation is a belt-and-braces second check.
- Release time increases by ~1–3 minutes for the extra PR + CI cycle.

**Open questions**

- Should the approver workflow also verify that the PR branch is `auto/release-json-*` and that the head SHA is reachable from the bot's push event, to defeat an attacker who convinces the approver to review an unrelated branch?
- Do we want the approver to require at least one passing status check before approving, or approve immediately and let automerge wait for checks?
- If `--ci --channel prerelease` cannot find all three platforms (e.g. macOS build failed), the script errors and writes nothing. Do we want the workflow to surface that as a release-workflow failure, or swallow it and continue?

### Option B — Direct commit to `main` by a bypass-actor App _(alternative)_

_Documented for completeness; less isolation than Option A._

Add a single GitHub App (`velocity-release-bot`, `contents:write`) to the ruleset `bypass_actors` list with `bypass_mode: always`, and have `build-image.yml` commit the updated JSON files directly to `main` after assets are published. Tag detection for stable-vs-prerelease is identical to Option A.

**Flow**

1. `build-image.yml` finishes uploading assets.
2. New job mints an App installation token.
3. Checkout `main`, run `update-release-json.py`, commit both JSON files with `Signed-off-by:`, push to `main`. Because the App is a bypass actor, the ruleset allows the push despite linear-history, approval-required, and squash-only rules (bypass ignores all of them).

**Trade-offs**

- No extra PR, no second App, no approver workflow. Fastest path for the release author.
- The App can push _any_ change to `main`, not just to the two JSON files. Rulesets don't express "bypass scoped to paths." The security posture therefore depends entirely on the App's private key and on the workflow file not being trivially extensible. If the App key leaks, `main` is writable without review.
- No PR audit trail for release-metadata changes; the commit appears directly on `main` with the bot as author. `git log --author` is the only record.
- Future admins who want to understand "why can this identity write to main?" have to read the ruleset UI. Worth leaving a `docs/platform/operations/` note if we pick this.
- Still needs the `web_commit_signoff_required` trailer in commit messages.

**Open questions**

- Do we pin the App's installed-workflow allow-list (via the App's permissions UI) to only the release workflow file, to reduce blast radius if another workflow is later added that calls it?
- Should we rotate the App's private key on a schedule (e.g. quarterly) independent of compromise signals?

### Option C — Move release metadata out of `main` _(alternative)_

_Documented for completeness; larger structural change._

Stop storing `release.json` and `os-list-velocity.json` on `main`. Instead publish them as release artifacts or on a dedicated data branch, and have the consumers fetch them at build (or runtime) time.

**Candidate shapes**

- **Release-asset JSON.** `update-release-json.py` still runs, but uploads `release.json` and `os-list-velocity.json` as assets on the GitHub Release itself. The Eleventy site fetches `release.json` at build time from the "latest stable" release; `/rpi.json` is served from the same source. No commit to `main` occurs at release time.
- **Orphan `release-data` branch.** A long-lived orphan branch holds only the two JSON files. The workflow pushes commits there (ruleset would not cover the branch). The Eleventy site reads from a raw GitHub URL on that branch, or fetches via a small build-time script.
- **Separate repo.** `banshee-data/velocity-release-data` with push access for a dedicated App. Eleventy pulls it as a git submodule or build-time fetch.

**Trade-offs**

- Structurally eliminates the problem: `main` never needs to be written to at release time. Release protection can be as strict as we want (1 approval, no bypass) without affecting the release flow.
- Biggest refactor: Eleventy data-loading changes, `rpi.json` routing changes (currently served as a static file under `velocity.report/rpi.json`), CDN cache behaviour for the imager JSON needs re-confirming, and `os-list-velocity.json`'s consumers need a new fetch path.
- Introduces a new source of truth for "what is the current release," which may drift from `main` (e.g. `main` claims `v0.5.3` is in the works while the release-data branch still points at `v0.5.2`). That drift is fine for the site, but it changes how we reason about `main` as the canonical snapshot.
- For the RPi Imager integration specifically, the asset-URL scheme matters: `os-list-velocity.json` currently lives at a stable path under `image/`, and the fork of `rpi-imager` referenced in `deploy-rpi-imager-fork-plan.md` may already encode that path. Any move has to land in lockstep with the fork.

**Open questions**

- Does the Eleventy site's data layer (11ty `_data/`) need a network fetch at build time, or can we ship a tiny pre-build script that downloads the JSON into `_data/` before `eleventy` runs?
- How does `velocity.report/rpi.json` (the URL RPi Imager hits) get served? Today it's a static file in `public_html/`. If we move the source, do we proxy, redirect, or regenerate at publish?
- Do we want to freeze `release.json` on `main` at the _last stable_ snapshot (so the repo still has a record), updating it on a slower cadence?

## Decision surface

Pick at most one of A, B, C for the release-metadata path. The `main`-protection changes (raise approvals to 1, add admin bypass) stand independent of which option wins — they improve posture whether or not PR 2 is automated, and they are a prerequisite for scaling the contributor base.

## Work units

Independent of which option is chosen, this work divides into:

1. **Ruleset tightening.** Raise `required_approving_review_count` to 1, set `require_last_push_approval: true`, add `Repository admin` (and optional maintainer team) to `bypass_actors`. Document in `docs/platform/operations/`.
2. **Release-metadata automation.** One of Options A / B / C above.
3. **`CODEOWNERS`.** Add at minimum a default entry (`* @tyrion-skynet` or equivalent) so the 1-approval floor has a natural reviewer assignment. Consider per-path rules for `public_html/src/_data/release.json` and `image/os-list-velocity.json` if Option A is chosen and we later want to formalise the bot reviewer.
4. **Operational docs.** Update `docs/platform/operations/rpi-imager.md` and any release-runbook docs to describe the new single-PR flow and the bypass policy.

## References

- Existing ruleset: `gh api repos/banshee-data/velocity.report/rulesets/6362131`
- Release workflow: `.github/workflows/build-image.yml`
- Metadata updater: `scripts/update-release-json.py`
- Consumed by: `public_html/src/_data/release.json` (Eleventy site, `/rpi.json` route) and `image/os-list-velocity.json` (RPi Imager fork — see `deploy-rpi-imager-fork-plan.md`)
- Prerelease flag in GitHub Releases: set by `build-image.yml` via `contains(github.ref_name, '-')`; read by `update-release-json.py --channel`
