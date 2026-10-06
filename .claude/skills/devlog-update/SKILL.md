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

## Format reference

The devlog uses one H2 header per UTC date:

```markdown
## April 7, 2026 - Short Theme Title

- Concise bullet describing what changed and why (#512).
- Another bullet. References files in `backticks`, links to [design docs](plans/foo.md) (#513, #514). <!-- link-ignore -->
- {branch-name} Unlanded work on a feature branch.
```

The register rules (one line per bullet, one heading per day, PR numbers) are canonical in `.github/STYLE.md` under "Logs and registers"; this skill applies them.

### Conventions

- **Date format:** `Month DD, YYYY` (e.g. `April 7, 2026`).
- **Separator:** `-` (hyphen) between date and theme. Never use em-dashes in headers.
- **Branch metadata:** unlanded bullets begin with `{branch-name}` (the branch slug in curly braces, immediately after `- `). No italic `_Branch:_` headers. This is the only format for denoting branch work.
- **Bullet style:** each bullet is exactly one physical line starting `- `. Never hard-wrap a bullet, however long: no indented continuation lines. Concise, action-focused, past tense.
- **Content per bullet:** what changed, which files/packages/layers, why, and references to design docs or PRs where relevant.
- **Ordering:** newest entry first (prepend to the file, after its introduction and formatting note).
- **One heading per date:** exactly one `##` heading per UTC calendar day that has work. No date ranges (`February 9-10`) and no second heading for a day that already has one: append to that entry and broaden its theme title instead. Merge related commits into themed bullets rather than listing every commit individually.
- **PR references:** every main-landed bullet ends with the number(s) of the PR(s) that delivered it, before the final full stop: `(#622)` or `(#605, #606)`. No "Merged as" or "merged to main" phrasing. Add file count only when notable. Work pushed straight to main without a PR has no number.
- **Links:** link to plan docs only when the entry is primarily about creating or updating that plan. Do not link every file mentioned.
- **Version bumps:** include only for actual releases. Omit pre-release bump bullets.
- **Tone:** factual, developer-journal style. No marketing language. Record decisions and rationale when non-obvious.

### Unlanded branch work within a daily entry

Every bullet for work not yet on main begins with `{branch-name}` (the branch slug in curly braces, immediately after `- `). Main-landed bullets have no tag. This is the only format: there are no italic `_Branch:_` headers or `<details>` blocks.

```markdown
## April 7, 2026 - Short Theme Title

- Bullet about work landed on main.
- Another main bullet.
- {copilot/complete-phase-1-image} Unlanded bullet about branch work.
- {copilot/complete-phase-1-image} Another unlanded bullet.
- {dd/mac/dmg-signing} Work on a different branch.
```

When a day has **only** branch work (nothing on main), the same format applies: every bullet gets the tag, and there is no special header.

```markdown
## April 7, 2026 - TLS & Image Polish

- {copilot/complete-phase-1-image} Fixed TLS certificate persistence across renewals.
- {copilot/complete-phase-1-image} Updated MOTD ASCII art for RPi login banners.
```

Rules:

- Main-landed bullets come first (no tag).
- Unlanded bullets follow, each beginning with `{branch-name}` (the branch slug in curly braces).
- Do not duplicate bullets: if a commit appears on both main and a branch, record it under main only.

### Cleaning up landed branch tags

As part of each devlog update run, scan existing entries for `{branch-name}` tags and check whether that branch has since merged to main:

```bash
# For each {branch-name} found in the devlog:
gh pr list --state merged --head "$branch" --json number --jq '.[0].number'
```

If the branch has merged, remove the `{branch-name}` tag from the bullet (the work is now on main) and append the merged PR number before the final full stop. When removing a tag, delete the `{branch-name} ` prefix (including the trailing space). The bullet stays under the date the work was done.

A merged PR can deliver work recorded under a different head branch (for example a rebased or renamed branch). When `gh pr list --head` finds nothing, find the PR that added the line with `git log origin/main --format='%h %s' -S '<distinctive phrase>' -- docs/DEVLOG.md`.

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

### 1. Fetch remote refs

Always fetch before scanning so that branch tips are current:

```bash
git fetch --quiet --all
```

### 2. Read the current devlog

```bash
head -80 docs/DEVLOG.md
```

Identify the date and full bullet list of the most recent entry. This is the **anchor entry**. Its date is the **anchor date**. Read enough of the file to capture at least the 3 most recent entries (they are needed for gap-fill in step 6).

### 3. Determine the scan window

Calculate `start_date` = anchor date minus 3 days (to catch any commits that landed just before or on the same day as the last entry but weren't captured).

Calculate `end_date` = today.

### 4. Gather git history

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

When scanning open PR branches, compare each branch's commits against the devlog to find uncaptured work.

When scanning merged PRs, verify each `(#NNN)` reference appears in the devlog under the UTC date the PR merged, and move a bullet recorded under the wrong date. The exception is work first recorded as `{branch-name}` bullets: those stay under the day the work was done when the tag is replaced by the PR number.

### 5. Group commits by calendar day

For each day in the scan window that has commits:

1. Identify which branches the commits are on (`main`, `copilot/*`, `codex/*`, etc.)
2. Group related commits into themes (e.g. "RPi image hardening", "web frontend fixes", "documentation updates")
3. For merged PRs on main, note the PR number

### 6. Gap-fill existing entries

For each day that **already has a devlog entry**, compare the full commit list for that day against the bullets already written. Look for commits whose work is not represented by any existing bullet.

Common causes of gaps:

- The entry was written mid-day and more commits landed later the same UTC day.
- Commits on open PR branches were not scanned when the entry was first written.
- A late-night session produced commits attributed to the same UTC date.

For each uncovered commit (or group of related commits), synthesise new bullets following the format reference. **Append** these bullets to the end of the day's single entry, before the next `## ` header (and before any `{branch-name}` bullets, which stay last). Never add a second heading for the same date. Preserve existing bullets apart from the permitted changes listed in Notes.

If the gap-fill adds enough new content to make the theme title inaccurate, update the theme title to reflect the broader scope (e.g. `## April 7 - RPi Image` becomes `## April 7 - RPi Image, Shell Hardening & Map Editor`).

### 7. Synthesise new-day entries

For each day **not already in the devlog**, write an entry following the format reference above:

- **Choose a theme title** that captures the day's primary focus area(s)
- **Write 3-12 bullets** summarising the day's work, merging related commits into single bullets
- **Add branch metadata** if work is on a feature branch
- **Include PR references** for anything merged to main
- **Link to design docs** when commits reference plan files

Do NOT copy commit messages verbatim. Synthesise them into coherent, human-readable summaries that describe _what was accomplished_ rather than _what was typed into git_.

### 8. Check for overlap

Before inserting or appending, verify the new bullets don't duplicate information already in the devlog. A commit is "covered" if an existing bullet describes the same change, even if the wording differs. When in doubt, skip the bullet rather than risk a duplicate.

### 9. Insert and amend

- **New-day entries:** insert into `docs/DEVLOG.md` immediately before the first `## ` entry, after the `# Development log` title, its introduction and the **Formatting:** note, in reverse chronological order (newest first).
- **Gap-fill bullets:** append to the day's existing entry, after its main-landed bullets and before its `{branch-name}` bullets. Do not reorder or rewrite existing bullets.

### 10. Verify

```bash
# Check the file looks right
head -80 docs/DEVLOG.md

# One heading per date: both commands must print nothing
grep "^## " docs/DEVLOG.md | sed -E 's/^## ([^-]+, [0-9]{4}).*/\1/' | sort | uniq -d
grep -nE "^## [A-Z][a-z]+ [0-9]+-[0-9]+," docs/DEVLOG.md

# One line per bullet: must print nothing
grep -n "^  " docs/DEVLOG.md

# Untagged bullets without a PR number: each should be a direct push to main
grep -n "^- " docs/DEVLOG.md | grep -v ":- {" | grep -vE "\(([^()]*, )?#[0-9]+" | head -20
```

## Notes

- This skill **writes** to `docs/DEVLOG.md`. It does not modify any other files.
- Existing bullets are never reworded or deleted. The permitted changes are: appending bullets to fill gaps, broadening a theme title, removing a landed `{branch-name}` tag, adding a missing PR number, joining a hard-wrapped bullet onto one line, merging a second heading for the same date into the first, and moving a `(#NNN)` bullet to the UTC date its PR merged.
- Commits on `backup/*` branches should be ignored (these are rescue snapshots, not development work).
- Coverage-update commits (`Update coverage data`) should be ignored: they are automated.
- When multiple branches have work on the same day, group by theme rather than by branch. Mention the branch in the metadata line.
- Keep bullets concise. A day with 60 commits should produce 5-10 bullets, not 60.
- Use British English spelling consistent with the rest of the repository (e.g. "standardisation" not "standardization", "colour" not "color").
