---
name: standup
description: Run the daily standup. Reads repo and worktree state, checks sync, and produces a concise PM standup with today's priorities, risks, and options.
argument-hint: ""
---

# Skill: standup

Run a daily repo standup. Surfaces worktree health, branch sync status, and today's priorities.

## Usage

```
/standup
```

## Procedure

### 1. Gather repo facts

Run the standup script if it exists:

```bash
scripts/flo-standup.sh --all-branches
```

If the script does not exist, gather the equivalent manually:

```bash
git worktree list
git branch -vv
git status
```

For each worktree: identify the branch, whether it tracks an upstream, and whether it is ahead, behind, or diverged.

### 2. Check sync before planning

Surface these conditions explicitly before proposing any work:

- Dirty worktrees (uncommitted changes)
- Branches behind upstream
- Branches behind `origin/main`
- Detached HEAD worktrees: map to the containing local/remote ref
- Duplicate or overlapping work across worktrees

Do not move to priorities until sync issues are identified.

### 3. Read planning context

After the snapshot:

- Read `docs/BACKLOG.md`
- Read only the plan docs that match the active branches or recently changed areas (do not load the full `docs/plans/` directory)

### 4. Produce the standup

```markdown
## State

- [Repo/worktree/branch snapshot]
- [Sync status against upstream and `origin/main`]

## Today

1. [Top priority]
2. [Second priority]
3. [Optional third priority]

## Risks

- [Blocker, ambiguity, or migration concern]

## Options

- Option A: [Fastest path]
- Option B: [Safer path]
- Option C: [Cleanup/refactor path]
```

### 5. Adapt to delivery mode

- **Interactive session**: keep the summary brief, offer options, ask at most one concrete prioritisation question
- **PR/comment mode**: convert into a written report with explicit next actions and owners

## Notes

- This skill reads only. It does not modify any files or plans.
- Treat worktrees as first-class: include detached worktrees and call out branch ambiguity explicitly.
- If priorities are clear and there are no sync issues, keep options brief.
- Do not load all plan docs: only those relevant to active branches.
