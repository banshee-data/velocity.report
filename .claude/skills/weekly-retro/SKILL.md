---
name: weekly-retro
description: Run the weekly planning review and agent drift check. Produces a PM review of backlog health, plan consistency, decisions needed, and agent drift report.
argument-hint: ""
---

# Skill: weekly-retro

Run the weekly planning review. Covers backlog health, plan consistency, decisions needing resolution, and agent drift between Copilot and Claude definitions.

## Usage

```
/weekly-retro
```

## Procedure

### 1. Planning snapshot

Run the planning review script if it exists:

```bash
scripts/flo-planning-review.sh
```

If the script does not exist, gather the equivalent manually:

- List all files in `docs/plans/` modified in the last 7 days
- Read `docs/BACKLOG.md`
- Read `docs/DECISIONS.md`

### 2. Agent drift check

```bash
make check-agent-drift
```

Report any agents present in `.github/agents/` but missing from `.claude/agents/`, or vice versa. Report content drift between paired files (persona methodology, coordination rules, forbidden actions). Acceptable divergence: platform-specific metadata (YAML frontmatter differences, tool restriction syntax).

### 3. Plan consistency audit

For each plan in `docs/plans/` modified in the last 14 days:

- Does it have a `Canonical` hub-doc link?
- Is it represented in `docs/BACKLOG.md` at the right milestone?
- Does it imply any decisions not yet recorded in `docs/DECISIONS.md`?
- Has it graduated? If Complete on `main`, should it be a symlink?

Flag three classes of issue:

1. Active plans missing a `Canonical` hub doc
2. Multiple active plans pointing at the same canonical hub doc
3. Graduated plans that should now be symlinks but still contain duplicated body text

### 4. Backlog health

Read `docs/BACKLOG.md` and assess:

- Are any milestone sections overloaded (more than ~10 items)?
- Are any items stale (no supporting plan doc, no recent activity)?
- Are there items that should be merged, split, or moved?
- Is the current milestone realistic given recent velocity?

### 5. Decisions audit

Read `docs/DECISIONS.md`:

- Are any open questions older than 4 weeks without resolution?
- Do any recently merged plans imply decisions not yet recorded?
- Are there contradictions between decisions and active plans?

### 6. Produce weekly review

```markdown
# Weekly Planning Review — [date]

## Agent Drift

[drift check results — missing pairs, content drift, acceptable divergence]

## New Or Changed Plans (last 14 days)

[list with status: new / updated / graduated / needs-symlink]

## Plan Consistency Issues

[canonical link issues, backlog gaps, decision gaps]

## Backlog Health

[overloaded milestones, stale items, recommended changes]

## Decisions Needed

[open questions, contradictions, items requiring resolution — owner and consequence for each]

## Recommended Changes

### docs/BACKLOG.md

- [exact item to add, move, merge, or remove]

### docs/DECISIONS.md

- [decision to record]

### Agent files

- [drift to resolve]

## Next Priority

[1–3 highest-value items for the coming week]
```

## Notes

- This skill reads only. It does not modify backlog, decisions, or agent files. Recommendations are for the human or for Flo/Appius to act on.
- If `scripts/flo-planning-review.sh` produces output, include it in the review rather than replacing it.
- Flag any plan that proposes cameras, licence plates, PII collection, or cloud transmission: these contradict `TENETS.md` and should be marked for revision regardless of milestone.
