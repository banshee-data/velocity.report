---
name: update-pr-description
description: Generate a PR title and description from the branch diff. Reads the commits, categorises changes, and updates the PR on GitHub.
argument-hint: "[optional: PR number or branch name]"
---

# Skill: update-pr-description

Generate a structured PR description from the actual branch diff and update the PR on GitHub.

## Usage

```
/update-pr-description
/update-pr-description 463
/update-pr-description feature/lidar-l7-scene
```

If no argument is given, use the current branch and find the open PR for it.

## Procedure

### 1. Identify the PR

If a PR number is given, use it directly. Otherwise:

```bash
git branch --show-current
```

Then find the open PR for that branch:

```bash
gh pr view --json number,title,body,state
```

If no open PR exists, stop and tell the operator.

### 2. Gather the diff

```bash
git log main..HEAD --oneline
git diff main --stat
```

Read the commit messages. They carry the intent. The diff stat shows the scope.

For large diffs, scan the actual changes in the most-modified files:

```bash
git diff main -- <file> | head -200
```

### 3. Categorise changes

Group changes into logical sections. Common categories:

- Feature or behaviour changes
- Refactoring or code quality
- Documentation rewrites or compliance passes
- Bug fixes
- Build, CI, or tooling changes
- Renames or file moves

Each category becomes a section heading in the PR description. Drop empty categories. If the PR only touches one category, skip the headings and write a flat description.

### 4. Draft the title

The PR title follows commit format: `[prefix] Description`.

Rules:

- Use the prefix(es) matching the dominant change type (`[docs]`, `[go]`, `[js]`, etc.)
- If the PR spans multiple prefixes, use the primary one or combine: `[go][js]`
- The description should summarise the whole PR in one line, not just the first commit
- Keep it under 72 characters if possible

### 5. Draft the body

Structure:

```markdown
One-sentence summary of what this PR does and why.

## Section heading

Paragraph or bullet list describing changes in this category. Be specific:
name the files, the functions, the migration. Do not pad.

## Section heading

...

## Checklist

- [x] Item that is done
- [ ] Item still pending (if any)
```

Writing rules:

- Lead with the user-visible or reviewer-visible impact
- Name specific files and functions when it helps the reviewer find the change
- State what was intentionally left unchanged when that is non-obvious
- Do not repeat the diff line-by-line; summarise the intent
- Use sentence case for section headings (per STYLE.md)
- British English spelling (per STYLE.md)

### 6. Update the PR

Use the GitHub MCP tool to update the title and body:

```
mcp_github_update_pull_request(
  owner, repo, pullNumber,
  title="[prefix] Description",
  body="..."
)
```

### 7. Confirm

Report the updated title and a one-line summary to the operator. Link to the PR.

## Edge cases

- **Draft PRs**: update normally; draft state is orthogonal to description quality.
- **Force-pushed branches**: gather the diff fresh; do not rely on cached commit history.
- **Multi-root workspaces**: determine the correct repo from the current working directory.
- **No open PR**: stop and tell the operator. Do not create a PR (that is a different skill).
