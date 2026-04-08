---
name: devlog-update
description: Update the development log with new entries synthesised from git history since the last entry.
argument-hint: ""
---

# Skill: devlog-update

Bring `docs/DEVLOG.md` up to date by reading git history across all branches and synthesising new entries in the established format.

## Usage

```
/devlog-update
```

## Format Reference

The devlog uses H2 headers for entries:

```markdown
## April 7, 2026 - Short Theme Title

- Concise bullet describing what changed and why.
- Another bullet. References files in `backticks`, links to [design docs](plans/foo.md). <!-- link-ignore -->
- Unlanded work on a feature branch. {branch-name}
```

### Conventions

- **Date format:** `Month DD, YYYY` (e.g. `April 7, 2026`).
- **Separator:** `-` (hyphen) between date and theme. Never use em-dashes in headers.
- **Branch metadata:** unlanded bullets end with `{branch-name}` (the branch slug in curly braces). No italic `_Branch:_` headers. This is the only format for denoting branch work.
- **Bullet style:** each bullet is a single `- ` line; concise, action-focused, past tense.
- **Content per bullet:** what changed, which files/packages/layers, why, and references to design docs or PRs where relevant.
- **Ordering:** newest entry first (prepend to the file, after the `# Development Log` title).
- **Granularity:** one entry per calendar day that has commits. Merge related commits into themed bullets rather than listing every commit individually.
- **PR references:** `(#NNN)` inline. No "Merged as" or "merged to main" phrasing. Add file count only when notable.
- **Links:** link to plan docs only when the entry is primarily about creating or updating that plan. Do not link every file mentioned.
- **Version bumps:** include only for actual releases. Omit pre-release bump bullets.
- **Tone:** factual, developer-journal style. No marketing language. Record decisions and rationale when non-obvious.

### Unlanded branch work within a daily entry

Every bullet for work not yet on main ends with `{branch-name}` (the branch slug in curly braces). Main-landed bullets have no tag. This is the only format: there are no italic `_Branch:_` headers or `<details>` blocks.

```markdown
## April 7, 2026 - Short Theme Title

- Bullet about work landed on main.
- Another main bullet.
- Unlanded bullet about branch work. {copilot/complete-phase-1-image}
- Another unlanded bullet. {copilot/complete-phase-1-image}
- Work on a different branch. {dd/mac/dmg-signing}
```

When a day has **only** branch work (nothing on main), the same format applies: every bullet gets the tag, and there is no special header.

```markdown
## April 7, 2026 - TLS & Image Polish

- Fixed TLS certificate persistence across renewals. {copilot/complete-phase-1-image}
- Updated MOTD ASCII art for RPi login banners. {copilot/complete-phase-1-image}
```

Rules:

- Main-landed bullets come first (no tag).
- Unlanded bullets follow, each ending with `{branch-name}` (the branch slug in curly braces).
- Do not duplicate bullets: if a commit appears on both main and a branch, record it under main only.

### Cleaning up landed branch tags

As part of each devlog update run, scan existing entries for `{branch-name}` tags and check whether that branch has since merged to main:

```bash
# For each {branch-name} found in the devlog:
gh pr list --state merged --head "$branch" --json number --jq '.[0].number'
```

If the branch has merged, remove the `{branch-name}` tag from the bullet (the work is now on main). If the bullet has no other distinguishing content, leave it as-is with the tag removed.

### STYLE.md compliance

All devlog text must follow the project writing conventions in `.github/STYLE.md`:

- **British English:** analyse, behaviour, colour, visualisation, etc. Preserve American spelling only in code identifiers.
- **No em-dashes:** use a colon to introduce a consequence or explanation, a comma for a natural pause, parentheses for genuine asides, or a full stop for a separate thought.
- **Active voice:** "Added X" not "X was added". "Fixed the race condition" not "The race condition was fixed".
- **Oxford comma:** yes. "Go, Python, and Swift".
- **Past tense:** throughout. No present-tense descriptions of current behaviour. Write "Configured nginx to serve..." not "nginx serves...".
- **Bullet length:** target 15-40 words. One idea per bullet. Split compound sentences into separate bullets rather than joining with semicolons.
- **Short sentences:** short sentences do the work. Split overly long bullets that exceed ~50 words.

## Procedure

### 1. Read the current devlog

```bash
head -20 docs/DEVLOG.md
```

Identify the date of the most recent entry. This is the **anchor date**.

### 2. Determine the scan window

Calculate `start_date` = anchor date minus 3 days (to catch any commits that landed just before or on the same day as the last entry but weren't captured).

Calculate `end_date` = today.

### 3. Gather git history

Fetch commits across **all branches** in the scan window:

```bash
# All branches, grouped by date
git log --all --oneline --since="$start_date" --format="%h %ad %an %s" --date=short | sort -t' ' -k2,2

# Main branch specifically (to identify merged PRs)
git log main --oneline --since="$start_date" --format="%h %ad %s" --date=short

# Merged PRs — cross-check that every landed PR appears in the devlog
gh pr list --state merged --limit 100 --json number,title,mergedAt,headRefName \
  --jq '.[] | "\(.mergedAt | split("T")[0]) #\(.number) \(.title)"' | sort -r

# Open PR branches — list branch-only commits (not on main)
gh pr list --state open --json number,headRefName --jq '.[] | "\(.number) \(.headRefName)"'
# For each open PR branch:
git log origin/$branch --not origin/main --oneline --format="%h %ad %s" --date=short
```

**Date attribution:** use the **UTC date** from `git log --date=iso-strict` or GitHub's `mergedAt` field. Do not convert to the author's local timezone. A commit at `2026-03-31T02:01:38Z` belongs to the March 31 entry, regardless of the author's local time. This matches the repo-wide timestamp convention in `coding-standards.md`.

When scanning open PR branches, compare each branch's commits against the devlog to find uncaptured work. Add branch subsections to existing daily entries (see "Branch sections within a daily entry" above).

When scanning merged PRs, verify each `(#NNN)` reference appears in the correct day's entry (by UTC date). A PR recorded under the wrong UTC date should be moved.

### 4. Group commits by calendar day

For each day in the scan window that has commits:

1. Identify which branches the commits are on (`main`, `copilot/*`, `codex/*`, etc.)
2. Group related commits into themes (e.g. "RPi image hardening", "web frontend fixes", "documentation updates")
3. For merged PRs on main, note the PR number
4. Skip days that are already covered by existing devlog entries (compare against anchor date and prior entries)

### 5. Synthesise entries

For each new day (not already in the devlog), write an entry following the format reference above:

- **Choose a theme title** that captures the day's primary focus area(s)
- **Write 3-12 bullets** summarising the day's work, merging related commits into single bullets
- **Add branch metadata** if work is on a feature branch
- **Include PR references** for anything merged to main
- **Link to design docs** when commits reference plan files

Do NOT copy commit messages verbatim. Synthesise them into coherent, human-readable summaries that describe _what was accomplished_ rather than _what was typed into git_.

### 6. Check for overlap

Before inserting new entries, verify they don't duplicate information already in the devlog. If the anchor entry partially covers a day, only add bullets for work not already documented.

### 7. Insert entries

Prepend new entries to `docs/DEVLOG.md` immediately after the `# Development Log` title line, in reverse chronological order (newest first). Do not modify existing entries.

### 8. Verify

```bash
# Check the file looks right
head -80 docs/DEVLOG.md

# Verify no duplicate date headers
grep -E "^## |^- \*\*" docs/DEVLOG.md | head -20
```

## Notes

- This skill **writes** to `docs/DEVLOG.md`. It does not modify any other files.
- Existing entries are never modified or deleted. New entries are prepended only.
- If the scan window overlaps with existing entries, only add genuinely new information.
- Commits on `backup/*` branches should be ignored (these are rescue snapshots, not development work).
- Coverage-update commits (`Update coverage data`) should be ignored — they are automated.
- When multiple branches have work on the same day, group by theme rather than by branch. Mention the branch in the metadata line.
- Keep bullets concise. A day with 60 commits should produce 5-10 bullets, not 60.
- Use British English spelling consistent with the rest of the repository (e.g. "standardisation" not "standardization", "colour" not "color").
