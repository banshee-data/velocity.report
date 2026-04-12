---
name: ship-change
description: Run the full validation and commit workflow for a completed change. Format, lint, test, build, then produce a properly formatted commit message.
argument-hint: "[optional: brief description of what changed]"
---

# Skill: ship-change

Run the complete quality gate, verify the change is ready to ship, and produce a properly formatted commit.

## Usage

```
/ship-change
/ship-change "add Kalman covariance inflation during coasting"
```

## Procedure

### 1. Understand what changed

```bash
git status
git diff --stat
git diff
```

Identify which subsystem(s) are affected: Go (`[go]`), Python (`[py]`), Web (`[js]`), Swift (`[mac]`), docs (`[docs]`), SQL (`[sql]`), Makefile (`[make]`), config (`[cfg]`).

### 2. Format

Run the formatter(s) matching the changed subsystem(s):

```bash
# Go changes
make format-go

# Python changes
make format-python

# Web changes
make format-web

# All
make format
```

### 3. Lint

```bash
make lint
```

If lint fails, fix the issues before proceeding. Do not skip lint.

### 4. Test

Run the tests for the affected subsystem(s):

```bash
# Go
make test-go

# Python
make test-python

# Web
make test-web

# All
make test
```

If tests fail, stop. Fix the failures before proceeding.

For a single Go package under test:

```bash
go test ./internal/path/to/package/... -v
```

For a single Python test file:

```bash
source .venv/bin/activate
cd tools/pdf-generator
pytest pdf_generator/tests/test_file.py -v
```

### 5. Build verification

Verify the build still compiles:

```bash
# Go (local dev, requires libpcap)
make build-radar-local

# If pcap unavailable
make build-radar-linux
```

For web changes:

```bash
make build-web
```

### 6. Review the change summary

Produce a one-paragraph summary of what changed and why. Answer:

- What problem does this solve?
- What is the scope of the change (files, components)?
- Are there any known limitations or follow-up items?

### 7. Compose commit message

Format: `[prefix(es)] Description of change`

Prefixes based on what changed:

- `[go]`: Go code
- `[py]`: Python code
- `[js]`: JavaScript/TypeScript
- `[mac]`: Swift/macOS
- `[docs]`: Markdown documentation
- `[sql]`: Database migrations
- `[make]`: Makefile
- `[cfg]`: Configuration files
- `[ai]`: AI-authored edits (always include alongside language tag)

Rules:

- One description line (imperative mood, lowercase after prefix)
- Optional body paragraph if context is non-obvious
- AI edits: always `[ai][go]`, `[ai][py]`, etc.

Example:

```
[go] add covariance inflation to Kalman tracker during coasting frames

Coasted tracks now inflate process noise by a configurable factor per
missed frame, widening the association gate for returning detections.
```

### 8. Stage and commit

Stage the specific files changed (not `git add .`):

```bash
git add internal/lidar/l5tracks/tracker.go
git add internal/lidar/l5tracks/tracker_test.go
git commit -m "[go][ai] add covariance inflation to Kalman tracker during coasting frames"
```

### 9. Post-commit check

```bash
git status    # confirm working tree is clean
git log -1    # confirm commit looks right
```

## Notes

- Do not commit if any step in the quality gate fails.
- Do not use `git add -A` or `git add .`: stage specific files to avoid accidentally including `.env`, credentials, or large binaries.
- Do not skip lint with `--no-verify`.
- If a pre-commit hook fails, fix the issue and create a **new** commit: do not amend.
- Documentation changes should also trigger doc update checks: did ARCHITECTURE.md, component READMEs, or `public_html/src/guides/setup.md` need updating?
