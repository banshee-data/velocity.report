---
name: release-prep
description: Full release readiness check. Runs format, lint, test, build, agent drift check, STYLE.md compliance, docs prep, and changelog verification. Produces a go/no-go summary.
argument-hint: "[--scope go|python|web|all] [--skip-docs]"
---

# Skill: release-prep

Run the full release readiness workflow for a point release. This skill
orchestrates the quality gate, agent/skill consistency checks, documentation
preparedness, and changelog verification in a single pass.

## Usage

```
/release-prep
/release-prep --scope go
/release-prep --skip-docs
```

## When to Run

- Before tagging a point release.
- Before building a disk image for deployment.
- After a large feature branch lands on `main`.
- When you want a single "are we ready?" answer.

## Relationship to Other Skills

This skill orchestrates checks that overlap with other skills. It does not
replace them — it calls them or runs their core checks in sequence:

| Skill               | What release-prep uses from it                            |
| ------------------- | --------------------------------------------------------- |
| `ship-change`       | Format → lint → test → build gate                         |
| `docs-release-prep` | Link check, length audit, plan graduation, open questions |
| `weekly-retro`      | Agent drift check                                         |

If you have already run `/docs-release-prep` and `/ship-change` recently,
`/release-prep` will re-verify rather than redo heavy work.

## Procedure

### 1. Format

```bash
make format
```

If any files change, stage them and note it in the report.

### 2. Lint

```bash
make lint
```

All subsystems must pass. If lint fails, stop and fix before proceeding.

### 3. Test

```bash
make test
```

All test suites must pass. If scoping to a single subsystem:

```bash
make test-go       # Go only
make test-python   # Python only
make test-web      # Web only
```

### 4. Build

Verify all build targets succeed:

```bash
make build-radar-local   # Go server (requires libpcap)
make build-web           # Svelte frontend
make build-ctl           # velocity-ctl
```

If `build-radar-local` fails due to missing pcap, use `make build-radar-linux`.

### 5. Agent drift check

```bash
make check-agent-drift
```

All agents must be aligned. If drift is reported, resolve before release.

Acceptable divergence (handled by the normaliser):

- YAML frontmatter differences (Claude vs Copilot metadata)
- Portrait comment blocks (Copilot only)
- Relative path prefixes (`../../` in Claude agents vs root-relative in Copilot)

### 6. Style compliance

Run a full STYLE.md pass on every file listed in the **🔑 Key Documents**
section of `README.md`. That section is the canonical list; do not duplicate
it here.

**Excluded from style and link rewrites:** `DECISIONS.md` preserves historical
decision records. Verify its links are functional but do not reformat headings
or prose for style compliance.

Check each file against the full rule set in `.github/STYLE.md`. Common violations:

- [ ] No em dashes — use colons, commas, or parentheses
- [ ] British English (`-ise` not `-ize`, `-our` not `-or`, `-re` not `-er`)
- [ ] Sentence-case headings (not Title Case)
- [ ] Colons introduce lists and expansions, not dashes
- [ ] No pre-built code blocks in design docs (see STYLE.md § Documentation Structure)
- [ ] Product name: `velocity.report` (lowercase v, no spaces)
- [ ] `VelocityVisualiser` (PascalCase for macOS app)
- [ ] No date metadata in doc headers (enforced by linter)
- [ ] Oxford comma in lists of three or more
- [ ] Active voice preferred; no hedging language

This is not a spot-check. Read each file and fix violations before proceeding.

### 7. Documentation preparedness

Unless `--skip-docs` is specified, run the docs-release-prep checks:

```bash
# Link validation
python3 scripts/check-relative-links.py --report 2>&1
python3 scripts/check-backtick-paths.py --report 2>&1
```

```bash
# Over-length documents
find docs/ -name '*.md' -not -path '*/plans/*' | while read f; do
  lines=$(wc -l < "$f")
  if [ "$lines" -gt 800 ]; then
    echo "$lines  $f"
  fi
done | sort -rn
```

```bash
# Plans ready for graduation
grep -rl '^\- \*\*Status:\*\* Complete' docs/plans/ 2>/dev/null | while read f; do
  if [ ! -L "$f" ]; then
    echo "Ready to graduate: $f"
  fi
done
```

If any links are broken or docs are over-length, note in the report.

### 8. Changelog verification

Read `CHANGELOG.md` and verify:

- [ ] An entry exists for the upcoming version
- [ ] The version number follows SemVer (`MAJOR.MINOR.PATCH`)
- [ ] All significant changes since the last release are listed
- [ ] No future/aspirational entries are present

Cross-reference with recent git history:

```bash
git log --oneline $(git describe --tags --abbrev=0 2>/dev/null || echo HEAD~50)..HEAD
```

### 9. Produce release readiness report

```markdown
# Release Readiness Report — [version] — [date]

## Quality Gate

| Check       | Status | Notes                                            |
| ----------- | ------ | ------------------------------------------------ |
| Format      | ✅/❌  | [files changed or clean]                         |
| Lint        | ✅/❌  | [subsystems checked]                             |
| Tests       | ✅/❌  | [pass count, any skips]                          |
| Build       | ✅/❌  | [targets built]                                  |
| Agent drift | ✅/❌  | [aligned count]                                  |
| Style       | ✅/❌  | [issues found]                                   |
| Docs        | ✅/❌  | [broken links, over-length, graduations pending] |
| Changelog   | ✅/❌  | [version entry present, coverage]                |

## Blockers

[list anything that must be fixed before release]

## Warnings

[non-blocking issues worth noting]

## Verdict

**Ready / Not Ready**

[one-sentence summary]
```

## Notes

- This skill reads and validates. It does not tag, push, or publish.
- If format changes files, it stages them but does not commit.
- Run `/ship-change` after fixing any issues to commit the cleanup.
- For docs-only releases, use `/docs-release-prep` directly.
