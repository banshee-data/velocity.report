---
name: review-pr
description: Security, correctness, and maintainability review of a pull request or set of changed files. Produces a structured code review.
argument-hint: "[PR number, branch name, or file paths]"
---

# Skill: review-pr

Review a pull request, branch diff, or set of changed files for security, correctness, test coverage, and maintainability.

## Usage

```
/review-pr 123
/review-pr feature/lidar-l7-scene
/review-pr internal/lidar/l5tracks/tracker.go
```

## Procedure

### 1. Get the diff

If a PR number is given:

```bash
gh pr view <number> --json title,body,files
gh pr diff <number>
```

If a branch name is given:

```bash
git diff main...<branch> --stat
git diff main...<branch>
```

If file paths are given, read the files directly.

### 2. Understand context

- Read the PR description or the commit messages for intent.
- Read the test files alongside the changed code.
- Read any plan or design doc linked in the PR.

### 3. Security pass (Malory's lens — read-only, gate classification)

Check in priority order:

1. **Input validation** — are all external inputs validated? (radar JSON, LiDAR UDP, API request bodies, config files)
2. **Privacy leaks** — could PII reach a log, response body, or export? Flag CRITICAL if yes.
3. **Injection vectors** — SQL, command, path traversal, LaTeX injection
4. **Auth & access** — are new API routes authenticated? Can unauthenticated users reach sensor data?
5. **Error messages** — do error messages leak internal state or sensitive data?
6. **Dependencies** — are new dependencies introduced? Any known CVEs?

Apply gate classification:

- **CRITICAL**: must fix before merge (RCE, auth bypass, PII exposure, data exfiltration)
- **INFO**: note for backlog (minor misconfig, stale dep, non-sensitive info disclosure)

### 4. Correctness pass (Appius's lens)

- Are the invariants named and preserved?
- Are failure modes visible and handled?
- Are there untested joins or missing edge cases?
- Are retries bounded?
- Are migrations safe? Do they have rollback paths?
- Are logs meaningful for operators?

### 5. Test coverage

- Are happy path, edge path, and failure path tests present?
- Do tests exercise the actual change or just existing behaviour?
- Is test setup clear and isolated?

### 6. Maintainability

- Is the changed boundary clear?
- Is the code legible without private knowledge of the author's intent?
- Are new abstractions justified by a real burden they carry?
- Is technical debt introduced? Is it named?

### 7. Quality gate check

Confirm the PR passed (or will pass) the mandatory gate:

```bash
make lint
make test
make build-radar-local  # or build-radar-linux if pcap unavailable
```

### 8. Verdict

```markdown
## Verdict

**Status:** [APPROVED | APPROVED WITH COMMENTS | CHANGES REQUIRED | BLOCKED — CRITICAL SECURITY]

**CRITICAL findings:** [list, or "none"]
**Changes required:** [list, or "none"]
**Advisory notes (INFO):** [list, or "none"]
```

## Output Format

```markdown
# PR Review: [title or number]

## Security Pass

[CRITICAL findings first, then INFO]

## Correctness

[correctness findings]

## Test Coverage

[coverage assessment]

## Maintainability

[maintainability notes]

## Quality Gate

[lint/test/build status or expectation]

## Verdict

[status + required changes]
```

## Suppressions

Do NOT flag:

- Go's explicit error handling verbosity — it is deliberate
- HTTP without TLS on localhost — local-only deployment
- SQLite without encryption at rest — local-only device, out of threat model
- Standard Go interface size choices unless the interface exceeds its documented contract
- `os.exec` calls in Makefile-invoked scripts — build tooling, not runtime code
- Formatting issues — handled by `make format`

## Notes

- CRITICAL security findings block merge regardless of scope mode.
- Read-only: this skill does not modify code. It produces recommendations for Appius to implement.
- Apply British English rules when reviewing documentation changes (see `.github/knowledge/coding-standards.md`).
