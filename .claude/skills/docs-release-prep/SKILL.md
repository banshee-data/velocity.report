---
name: docs-release-prep
description: Prepare documentation for a release. Fixes links, graduates completed plans, simplifies over-length docs, splits large files, resolves open questions, updates design decisions, and verifies docs are ready for the disk image.
argument-hint: "[--dry-run] [--scope radar|lidar|platform|ui|all]"
---

# Skill: docs-release-prep

Run the full documentation preparedness workflow before a release. This skill
codifies the sequence of editorial and structural checks that make docs
accurate, navigable, and sized for purpose.

## Usage

```
/docs-release-prep
/docs-release-prep --dry-run
/docs-release-prep --scope lidar
```

## Goals

1. Every Markdown link resolves to a real file.
2. Every completed plan is graduated (symlink + hub-doc consolidation).
3. No spec document exceeds the length target without justification.
4. Large documents with distinct topics are split into focused files.
5. Open design questions are surfaced; answered questions are recorded.
6. Design decision tables reflect current implementation, not stale drafts.
7. Documentation included in the disk image is complete and correct.

## Rubrics

### Length Target: 800 Lines

Specification and architecture documents should target **≤ 800 lines**. This
is a guideline, not a hard wall — a 900-line doc with dense tables is fine;
a 2,000-line doc that wanders is not.

**What to cut:**

- Pre-built code blocks (Go structs, JS snippets, HTML templates) that
  duplicate what exists in the source tree. Replace with a prose description
  and a file reference.
- Completion checklists for finished features (compress to a summary line
  or remove entirely).
- Duplicated content that exists in another canonical document.

**What to keep:**

- Design discussion and ideation (the whole point of a spec).
- Architecture rationale and rejected-alternative analysis.
- ASCII/text UI mockups (design artefacts, not implementation).
- Pseudocode describing algorithm flow (ideation, not pre-built code).
- API endpoint tables with error contracts and status codes.
- Tables capturing design decisions, trade-offs, and precedence rules.
- Round-duration examples, timing tables, and worked scenarios.
- Security considerations and threat mitigations.

**Rule of thumb:** if a block would be copied into a source file verbatim,
it is pre-built code and should go. If it describes _what_ the system should
do and _why_, it is design discussion and stays.

### Split Threshold

If a document covers two or more clearly independent topics and exceeds
800 lines, split it:

1. Extract the secondary topic into a new file in the same directory.
2. Add a cross-reference link from the original to the new file.
3. Keep the original as the primary document for the core topic.

Example: a vector scene map doc that also specifies the geometry-prior
service should split the prior service into its own file.

### Open Questions

Every spec should have an **Open Questions** section (or confirm none remain).
Questions fall into two categories:

- **Open** — genuinely unanswered. State the question, the trade-offs, and
  any recommendations. Do not fabricate answers.
- **Resolved** — answered by the author or through implementation. Move to a
  **Resolved Design Questions** or **Design Decisions** table with the
  actual resolution.

**Never invent answers to open questions.** If the answer is unknown, leave
the question open and surface it to the operator.

### Plan Files vs Reference Docs

Reference documents (architecture overviews, CLI guides, API references,
configuration docs) describe the system **as it is implemented right now**.
Plan files (`docs/plans/`) describe **future work** — proposed features,
restructuring ideas, phased rollouts, aspirational architectures.

**The boundary rule:** if a section describes something that does not exist
in the codebase today, it belongs in a plan file, not in a reference doc.

**When auditing a document:**

1. Check every feature claim against the codebase. If a flag, endpoint,
   binary, or behaviour is documented but does not exist, either remove it
   or move it to a plan file.
2. If a reference doc has a "Proposed" or "Future" or "Long-Term" section,
   extract it to a corresponding plan file in `docs/plans/` and replace it
   with a cross-reference link.
3. When a plan is implemented (code lands on `main`), the relevant facts
   move **from** the plan file **into** the reference doc. The plan file
   then becomes a candidate for graduation (symlink).

**Ghost entries** — features documented as current but actually deleted,
renamed, or never implemented — are the most dangerous form of stale
content. Each audit pass should verify implementation status against the
source code, ideally by checking the actual flag definitions, route
registrations, or binary directories.

### Design Decision Tables

Every spec with non-trivial design choices should have a decision table:

| Decision | Resolution |
| -------- | ---------- |
| Question | Answer     |

Entries must reflect the _actual_ implemented state or the _actual_ author
decision, not a plausible-sounding guess. When updating these tables, verify
each entry against the codebase or the author's stated intent.

## Procedure

### 1. Fix Links

Run the fix-links skill first — broken links undermine every subsequent step.

```
/fix-links
```

Or manually:

```bash
python3 scripts/check-relative-links.py --report 2>&1
python3 scripts/check-backtick-paths.py --report 2>&1
```

Fix all automatically resolvable links. Surface ambiguous cases to the
operator.

### 2. Graduate Completed Plans

List all plan files and check for graduation eligibility:

```bash
grep -rl '^\- \*\*Status:\*\* Complete' docs/plans/ | sort
```

For each Complete plan that is not yet a symlink:

```bash
file docs/plans/<plan-file>.md
```

If it is a regular file (not a symlink), run:

```
/plan-graduation docs/plans/<plan-file>.md
```

Respect the two-PR rule: a plan must be Complete on `main` before the
symlink PR. If the completion hasn't landed yet, note it for the next cycle.

### 3. Audit Document Length

```bash
find docs/ -name '*.md' -not -path '*/plans/*' | while read f; do
  lines=$(wc -l < "$f")
  if [ "$lines" -gt 800 ]; then
    echo "$lines  $f"
  fi
done | sort -rn
```

For each document over the target:

1. Check whether it covers multiple independent topics → split (§Split Threshold).
2. Check for pre-built code blocks → replace with prose + file reference.
3. Check for completion checklists of shipped features → compress or remove.
4. Check for duplicated content → remove and add a cross-reference.
5. If the document is genuinely dense with design discussion, leave it and
   note the justification.

Report the before/after line counts.

### 4. Resolve Open Questions

For each spec under `docs/`:

```bash
grep -rl 'Open Question' docs/ --include='*.md' | sort
```

Read each Open Questions section. For each question:

- If the implementation has answered it, move to Resolved with the correct
  answer (verify against code or author).
- If it is still genuinely open, leave it and surface to the operator.
- **Do not fabricate answers.**

### 5. Update Design Decision Tables

For each spec with a Design Decisions or Resolved Design Questions table:

- Verify each entry against the current codebase.
- Update entries that have drifted from implementation.
- Add new decisions that were made during implementation but not recorded.

### 6. Tone and Style Pass

Quick scan for the most common tone issues:

- [ ] **Heading case:** sentence case, not title case (except brand names).
- [ ] **Anti-phrases:** replace `utilise`, `leverages`, `cutting-edge`,
      `seamless`, `end-to-end` on sight.
- [ ] **Passive voice accumulation:** rewrite passages with 3+ consecutive
      passive sentences.
- [ ] **Product name:** `velocity.report` (lowercase v, no spaces).
- [ ] **British English:** `-ise` not `-ize`, `-our` not `-or`.

### 7. Disk Image Readiness

Check that documentation referenced by the disk image build is present and
correct:

```bash
# Verify README and key docs exist
ls -la README.md ARCHITECTURE.md TROUBLESHOOTING.md CHANGELOG.md

# Verify the web build includes help content
ls -la web/build/ 2>/dev/null || echo "Web not built — run make build-web"

# Check image config references
grep -r 'docs/' image/config/ 2>/dev/null || true
```

Verify:

- [ ] `README.md` is current and reflects the latest release.
- [ ] `CHANGELOG.md` has an entry for the upcoming release.
- [ ] `ARCHITECTURE.md` matches the current component layout.
- [ ] `TROUBLESHOOTING.md` covers known deployment issues.
- [ ] Any docs bundled in the image (`static/`, `web/build/`) are up to date.

### 8. Report

Print a summary:

```
## Docs Release Prep Summary

- Links fixed:           N (N ambiguous, surfaced to operator)
- Plans graduated:       N
- Documents simplified:  N (before/after line counts)
- Documents split:       N (new files created)
- Questions resolved:    N (N still open, surfaced to operator)
- Decisions updated:     N
- Tone fixes:            N files
- Disk image:            Ready / N issues

Next: review changes, then /ship-change
```

## When to Run

- Before every point release (part of the release checklist).
- Before building a disk image for deployment.
- After a major documentation branch lands on `main`.
- Quarterly, as a documentation health check.

## What This Skill Does Not Do

- Does not write new documentation from scratch.
- Does not make architectural decisions — surfaces them to the operator.
- Does not commit or push — leaves changes staged for review.
- Does not restructure code — only documentation files.
