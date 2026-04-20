---
name: address-pr-comments
description: Read all review comments on a PR, triage them, apply fixes, commit per-area with one prefix each, and draft replies that reference the fix commits. Interactive — asks before pushing.
argument-hint: "[optional: PR number or branch name]"
---

# Skill: address-pr-comments

Walk through every open review comment on a PR, decide which ones are actionable, apply fixes, commit the work split by area, and reply to each thread referencing the commit SHA that resolves it.

## Usage

```
/address-pr-comments
/address-pr-comments 473
/address-pr-comments feature/lidar-l7-scene
```

If no argument is given, use the current branch and find its open PR.

## Procedure

### 1. Identify the PR

If a PR number is given, use it directly. Otherwise:

```bash
git branch --show-current
gh pr view --json number,title,state,headRefName,baseRefName
```

If no open PR exists, stop and tell the operator.

### 2. Collect all comments

There are three comment surfaces on a PR and they live in different endpoints. Fetch all of them:

```bash
# Inline review comments (attached to code lines)
gh api repos/{owner}/{repo}/pulls/{number}/comments --paginate

# Review summary comments (the top-level body of each review)
gh api repos/{owner}/{repo}/pulls/{number}/reviews --paginate

# Issue-style comments (general PR conversation, not attached to code)
gh api repos/{owner}/{repo}/issues/{number}/comments --paginate
```

For each comment record keep: `id`, `user.login`, `path`, `line`, `body`, `in_reply_to_id`, `pull_request_review_id`.

Filter out:

- Your own prior replies (same bot/user that would be posting)
- Threads that already have a reply from the PR author accepting or rejecting the point (look for `in_reply_to_id` chains)
- Outdated comments (the API marks these with `position: null`)

Group remaining comments into threads by `in_reply_to_id` so each thread is addressed once.

### 3. Triage each comment

For every live thread, classify it:

| Class            | Meaning                                                               | Action                                                                 |
| ---------------- | --------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| **actionable**   | Clear code change requested, you understand what to do                | Fix it                                                                 |
| **disagree**     | Reviewer is wrong (misreads the diff, contradicts project convention) | Reply only, no code change                                             |
| **ambiguous**    | Valid point but several ways to fix, or you aren't sure what is meant | **Ask the operator before proceeding**                                 |
| **out-of-scope** | Valid but belongs in a follow-up PR                                   | Reply acknowledging + link to backlog entry, no code change in this PR |
| **nit**          | Style preference the reviewer flagged as non-blocking                 | Fix if trivial, else reply and skip                                    |

Print the triage table to the operator before doing anything destructive:

```
#1 src/foo.go:42  [actionable]  "extract helper for retry loop"
#2 Makefile:354   [disagree]    "restore -f svg -f png" → WireViz 0.4.1 uses -f sp
#3 web/app.ts:10  [ambiguous]   "handle the null case" → null from which source?
...
```

**Pause for any `ambiguous` rows.** Ask the operator which interpretation to take, one question per ambiguity. Do not guess.

### 4. Plan the commits

Group `actionable` + trivial `nit` items into commits. Rules:

- **One prefix per commit.** Do not combine `[go]` and `[py]` in one commit; split them.
- **One area per commit.** Do not bundle unrelated subsystems even if they share a prefix. Two Go commits are fine if they touch unrelated packages.
- **Multiple comments per commit are fine** when they sit in the same area and share intent (e.g. three `[py]` docstring tweaks in one helper module).
- **Split further** if comments in the same area pull in opposite directions — a rename and an algorithm fix in the same file deserve separate commits.

Present the commit plan to the operator:

```
Commit A: [py] clarify svg_to_png boolean contract (comments #2, #3, #4)
Commit B: [docs] correct pinout caption wording (comment #5)
Commit C: [make] document WireViz -f sp flag format in comment (comment #1 — no code change, reply only)
```

If the operator is happy, proceed. If not, reshuffle.

### 5. Apply fixes

For each planned commit:

1. Make the edits for that commit's comments only.
2. Run the relevant formatter + lint + test for the area touched (`make format-<area>`, `make lint`, scoped tests). Do not run the full `make test` for every commit — scope to the touched subsystem, but never skip lint.
3. Stage only the files for this commit (`git add <paths>` — never `git add -A`).
4. Commit with the single-prefix message:

   ```bash
   git commit -m "$(cat <<'EOF'
   [py] clarify svg_to_png boolean contract

   Documents that backend conversion errors propagate as exceptions;
   False is reserved for "no converter installed". Reworded the three
   warning call-sites to match.
   EOF
   )"
   ```

5. Capture the resulting SHA: `SHA=$(git rev-parse HEAD)`.
6. Record a `thread_id → sha` mapping for the reply step.

AI edits must use `[ai][lang]` per the repo's commit-prefix rules; do not drop `[ai]`.

### 6. Draft replies

For each live thread, compose a reply that:

- States the resolution in one short sentence (fixed / disagreeing / deferred).
- References the fix commit by short SHA if applicable: ``Fixed in `abc1234`.``
- For `disagree` threads, states the reason crisply and links to the evidence (docs, CLI `--help` output, upstream issue). No hedging.
- For `out-of-scope` threads, links to the backlog entry or new issue.

Example replies:

```
Fixed in `a1b2c3d` — docstring now documents that backend errors propagate;
only a missing converter returns False.

Disagree. WireViz 0.4.1 `--help` shows the format flag takes a single
char-string (`-f sp` for SVG+PNG); the repeated `-f svg -f png` form this
PR is fixing was the actual bug (it parsed the second flag as literal
`n`). Leaving as-is.

Valid point but out of scope here. Filed follow-up: #491.
```

Do not post replies yet — show the draft list to the operator and wait for approval.

### 7. Post replies

After operator approval, post each reply to the correct endpoint:

```bash
# Reply to an inline review comment (thread)
gh api repos/{owner}/{repo}/pulls/{number}/comments \
  -F body="..." \
  -F in_reply_to=<comment_id> \
  -X POST

# Reply to a general issue-style comment
gh api repos/{owner}/{repo}/issues/{number}/comments \
  -F body="..." \
  -X POST
```

Use `in_reply_to` so replies nest under the original thread rather than appearing as orphan comments.

### 8. Push

Confirm with the operator before pushing:

```
About to push N commits to origin/<branch>. Proceed?
```

Only after explicit confirmation:

```bash
git push
```

If the branch tracks an upstream that diverged (e.g. a rebase happened elsewhere), stop and ask. Do not force-push without explicit instruction.

### 9. Report

Summarise for the operator:

- Commits created (short SHA + subject)
- Threads replied to (count per class: fixed / disagreed / deferred / nit-skipped)
- Threads still requiring operator input (if any `ambiguous` were deferred)
- Push status

## Interactive gates

This skill pauses for the operator at three points:

1. **After triage** — if any comment is `ambiguous`.
2. **After commit plan** — to confirm grouping.
3. **Before push** — always.

Never skip a gate to "save a round-trip". The gates are the point.

## Edge cases

- **Empty comment set**: tell the operator the PR has no open threads and exit.
- **Only your own comments**: ditto — nothing to act on.
- **Merge conflicts while applying fixes**: stop, tell the operator, do not resolve silently.
- **Reviewer edits a comment mid-session**: refetch before replying; posting against a stale body can double-post.
- **Branch is protected / push rejected**: report the rejection verbatim and stop. Do not retry with `--force`.
- **Suggested change blocks (`suggestion`)**: treat these as actionable unless you disagree; a suggestion is still a proposal, not an auto-merge.

## Suppressions

Do not flag or re-open:

- Comments the PR author already resolved or dismissed
- Comments on lines removed by a later commit in the same PR
- Copilot / bot comments that restate project conventions already covered in `.github/knowledge/coding-standards.md` — reply "per coding-standards.md §X" and skip

## Notes

- This skill writes commits and pushes code. It is not read-only.
- British English in replies where prose is involved (per `STYLE.md`).
- Never use `--no-verify` to bypass a failing hook; fix the underlying issue.
- Never amend a commit that's already been pushed; add a new commit instead.
