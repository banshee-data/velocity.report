---
name: plan-review
description: Review a design plan or feature spec for scope, technical soundness, risks, and sequencing. Produces a structured PM + architecture review.
argument-hint: "[path/to/plan.md or plan name]"
---

# Skill: plan-review

Review a design plan, feature spec, or backlog item. Produces a structured review covering scope, architectural soundness, risks, and recommended sequencing.

## Usage

```
/plan-review path/to/plan.md
/plan-review "lidar L7 scene layer"
/plan-review  # review the most recently changed plan doc
```

## Procedure

### 1. Locate the plan

If a path is given, read it directly. If a name is given, search `docs/plans/` for a matching file. If no argument, identify the most recently modified file in `docs/plans/`.

Also read:

- `docs/BACKLOG.md` (for milestone context)
- `docs/DECISIONS.md` (for related decisions)
- Any files the plan references in its first section

### 2. Scope review (ruth's lens)

Answer each question:

- What is the actual decision this plan is trying to make?
- What is explicitly in scope? What is explicitly out of scope?
- Is there a NOT-in-scope list? If not, draft one.
- Does the plan try to resolve too much at once? Flag overreach.
- What happens if we do nothing?
- What existing code already solves part of this problem?

### 3. Technical soundness review (grace's lens)

- Are component boundaries clean?
- Is data flow described for happy path and failure paths?
- Is the migration path defined for existing deployments?
- Are there any privacy concerns? (camera/licence plate/PII/cloud transmission)
- Does the plan fit within Raspberry Pi 4 constraints?
- Are new dependencies justified?

### 4. Risk register (flo's lens)

Produce a risk register:

```markdown
| Risk               | Likelihood   | Impact       | Mitigation |
| ------------------ | ------------ | ------------ | ---------- |
| [risk description] | Low/Med/High | Low/Med/High | [action]   |
```

Minimum risk categories to check: technical, integration, data/migration, privacy, scope creep, dependency.

### 5. Sequencing check (flo's lens)

- Are tasks ordered to unblock dependencies early?
- Are the highest-risk items tackled first?
- Is there an incremental delivery path (not big-bang)?
- Does every task have acceptance criteria?

### 6. Backlog alignment

- Is this plan represented in `docs/BACKLOG.md`? In the right milestone?
- Does the plan imply any decisions that should be recorded in `docs/DECISIONS.md`?
- Does the plan have a `Canonical` hub doc link?

### 7. Verdict

Produce a short verdict:

```markdown
## Verdict

**Status:** [APPROVED | APPROVED WITH CHANGES | NEEDS REVISION | BLOCKED]

**Blocking issues:** [list, or "none"]
**Recommended changes:** [list, or "none"]
**Suggested next agent:** [Appius for implementation | Grace for redesign | Flo for re-sequencing]
```

## Output format

```markdown
# Plan Review: [plan name]

## Scope

[scope assessment + NOT-in-scope list]

## Technical Soundness

[architectural findings]

## Risk Register

[table]

## Sequencing

[sequencing assessment]

## Backlog Alignment

[alignment check]

## Verdict

[status + next steps]
```

## Notes

- This skill reads only. It does not modify the plan or backlog.
- If the plan is missing critical sections (acceptance criteria, migration path, privacy review), call them out explicitly rather than inferring.
- Apply `TENETS.md` constraints: flag any plan that proposes cameras, licence plates, PII collection, or cloud transmission as blocked.
