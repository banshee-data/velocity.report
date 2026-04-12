# Agent Knowledge Architecture

- **Status:** Complete (Phase 3 shipped)
- **Plan:** [agent-claude-preparedness-review-plan.md](../../plans/agent-claude-preparedness-review-plan.md) — Complete

Layered knowledge architecture that supplies velocity.report's AI agents with project context whilst enforcing DRY principles across tools and platforms.

## Problem

velocity.report uses seven named AI agents. The system previously suffered from three structural problems:

1. **Massive duplication** — 4,723 lines across 7 agent files with ~50% duplicated content. Privacy principles, build commands, SQLite facts, and Python venv details appeared in multiple files.
2. **Tool lock-in** — all knowledge was in Copilot-specific formats. Adding Claude Code meant duplicating everything or restructuring.
3. **Scaling problem** — expanding from 7 to 10–15 agents with each agent carrying its own copy of project knowledge would mean maintaining 15+ copies of the same facts.

All three problems are resolved. The architecture is now live across both Copilot and Claude Code.

## Layered Knowledge Model

```
┌─────────────────────────────────────────────────────┐
│  Layer 0: PROJECT TENETS                            │
│  TENETS.md                                          │
│  Privacy · No PII · No cameras · No black-box AI   │
│  ← Every agent, every tool inherits this            │
├─────────────────────────────────────────────────────┤
│  Layer 1: SHARED PROJECT KNOWLEDGE                  │
│  .github/knowledge/                                 │
│  ├── build-and-test.md      (make targets, venv)    │
│  ├── architecture.md        (tech stack, DB, paths) │
│  ├── coding-standards.md    (British English, fmt)  │
│  ├── hardware.md            (radar, LiDAR specs)    │
│  ├── security-surface.md    (attack surface map)    │
│  └── security-checklist.md  (gate classification)   │
│  ← Referenced, never duplicated                     │
├─────────────────────────────────────────────────────┤
│  Layer 2: ROLE CLASS MIXINS                         │
│  .github/knowledge/                                 │
│  ├── role-technical.md   (build, venv, packaging)   │
│  └── role-editorial.md   (style, brand, audience)   │
│  ← Agents include the mixin matching their class    │
├─────────────────────────────────────────────────────┤
│  Layer 3A: AGENT PERSONAS (dual-native)             │
│  .github/agents/*.agent.md   (Copilot)              │
│  .claude/agents/*.md         (Claude Code)          │
│  ← Persona + responsibilities + Layer 1/2 refs      │
│  ← NO project facts restated here                   │
│  ← Drift-checked weekly: make check-agent-drift     │
├─────────────────────────────────────────────────────┤
│  Layer 3B: SHARED WORKFLOW SKILLS                   │
│  .claude/skills/<workflow>/SKILL.md                 │
│  ← /plan-review, /review-pr, /ship-change,          │
│     /weekly-retro                                   │
│  ← Reusable procedures, slash commands              │
│  ← Single source across both tools                  │
├─────────────────────────────────────────────────────┤
│  Layer 4: TOOL ENTRY POINTS (thin shims)            │
│  .github/copilot-instructions.md   (Copilot)        │
│  CLAUDE.md                         (Claude Code)    │
│  ← Import tenets + knowledge + tool config only     │
└─────────────────────────────────────────────────────┘
```

## DRY Enforcement Rules

1. **No project fact may appear in more than one file.** If two agents need the same fact, it belongs in Layer 1.
2. **Agent files reference, never restate.** Use `See build-and-test.md for make targets and venv setup.`
3. **Tenets are inherited, never copied.** Every agent gets Layer 0 automatically through the tool entry point.
4. **Role mixins are opt-in by class.** Technical agents reference `role-technical.md`; editorial agents reference `role-editorial.md`.
5. **Persona content is the one exception.** Methodology and coordination rules are duplicated across Copilot and Claude definitions (~40–80 lines/agent). Drift-checked weekly.
6. **Workflow logic lives in skills, not agents.** If a procedure is reusable and user-invocable, it belongs in a `SKILL.md`.

## Agent Roster

| Agent      | Domain               | Class     | Copilot                          | Claude                     |
|------------|----------------------|-----------|----------------------------------|----------------------------|
| **Appius** | Execution            | Technical | `.github/agents/appius.agent.md` | `.claude/agents/appius.md` |
| **Euler**  | Algorithms           | Technical | `.github/agents/euler.agent.md`  | `.claude/agents/euler.md`  |
| **Grace**  | System architecture  | Technical | `.github/agents/grace.agent.md`  | `.claude/agents/grace.md`  |
| **Malory** | Adversarial thinking | Technical | `.github/agents/malory.agent.md` | `.claude/agents/malory.md` |
| **Flo**    | Coordination         | Editorial | `.github/agents/flo.agent.md`    | `.claude/agents/flo.md`    |
| **Terry**  | Narrative            | Editorial | `.github/agents/terry.agent.md`  | `.claude/agents/terry.md`  |
| **Ruth**   | Judgment             | Both      | `.github/agents/ruth.agent.md`   | `.claude/agents/ruth.md`   |

### Role Class Boundaries

**Technical agents need:** Make targets, build system, Python venv, test commands, database schema, hardware interfaces, packaging targets, path conventions.

**Editorial agents need:** Brand voice, tone guidelines, target audience profiles, documentation structure, style guide (British English).

**Ruth (Both):** All of the above plus decision frameworks, tradeoff methodology, and scope challenge discipline.

## Workflow Skills

Eight slash-command skills live in `.claude/skills/`. They are the canonical workflow layer — single-source, invocable from both Claude Code and VS Code.

| Skill           | Command                      | Purpose                                                  |
|-----------------|------------------------------|----------------------------------------------------------|
| plan-review     | `/plan-review [plan]`        | Scope, technical, and risk review of a design plan       |
| review-pr       | `/review-pr [PR/branch]`     | Security, correctness, and maintainability review        |
| ship-change     | `/ship-change`               | Format → lint → test → build → commit                    |
| weekly-retro    | `/weekly-retro`              | Backlog health, plan consistency, and drift check        |
| standup         | `/standup`                   | Daily repo and worktree standup with priorities          |
| security-review | `/security-review [path]`    | Security audit: static analysis, fuzz targets, checklist |
| trace-matrix    | `/trace-matrix [task-group]` | Trace backend surfaces against MATRIX.md                 |
| fix-links       | `/fix-links [path]`          | Fix dead links and stale backtick paths in Markdown      |

Rule: if a procedure is reusable and user-invocable, it belongs in a `SKILL.md`, not in an agent body.

## Platform Strategy: Dual-Native with Drift Detection

Agent personas are defined natively for each platform. Shared project knowledge (Layers 0–2) and workflow skills (Layer 3B) remain single-source.

### What Gets Duplicated (Bounded)

| Content                    | Duplicated?          | Copilot                       | Claude                                     |
|----------------------------|----------------------|-------------------------------|--------------------------------------------|
| Project tenets             | No                   | `TENETS.md`                   | `TENETS.md` (same file)                    |
| Build/test knowledge       | No                   | `.github/knowledge/`          | `.github/knowledge/` (same files)          |
| Role mixins                | No                   | `.github/knowledge/role-*.md` | `.github/knowledge/role-*.md` (same files) |
| Persona name + description | Yes (~2 lines)       | YAML frontmatter              | Inline in `.claude/agents/*.md`            |
| Persona methodology        | Yes (~30–60 lines)   | `.agent.md` body              | `.claude/agents/*.md`                      |
| Tool restrictions          | Copilot-only         | YAML `tools:` field           | n/a                                        |
| Coordination rules         | Yes (~10–20 lines)   | `.agent.md` body              | `.claude/agents/*.md`                      |

Total bounded duplication per agent: ~40–80 lines.

### Drift Detection

`scripts/check-agent-drift.sh` compares paired definitions:

1. Enumerates all `.github/agents/*.agent.md` files
2. Looks for a corresponding `.claude/agents/*.md` file
3. Extracts persona sections (strips YAML and platform-specific directives)
4. Reports missing pairs, content drift, and acceptable divergence
5. Integrated into `scripts/flo-planning-review.sh` and `/weekly-retro` skill

Make target: `make check-agent-drift`

## Key Platform Findings

**Copilot agent file reference resolution:** Copilot does **not** eagerly resolve Markdown links in `.agent.md` bodies. References remain literal text; agents read files on demand using tools. This means Layer 1/2 knowledge that agents need on every interaction should be inlined in `copilot-instructions.md`, not just referenced.

**Skills are the shared workflow surface:** Skills are the only primitive that give native `/workflow-name` invocation in Claude Code, slash-command discovery in VS Code, bundled scripts and checklists, and on-demand loading. They are the correct home for workflows like `plan-review`, `review-pr`, `ship-change`, and `weekly-retro`.

**Prompt files are wrappers only:** `.github/prompts/*.prompt.md` files may select an agent, tools, or model for a workflow in VS Code, but they are not the canonical workflow definition.

## Adopted Design Patterns

| Pattern                         | Applied To               | Description                                                      |
|---------------------------------|--------------------------|------------------------------------------------------------------|
| Scope modes                     | Ruth (primary), Grace    | EXPANSION / HOLD / REDUCTION — user selects at session start     |
| Mandatory output artefacts      | Ruth, Flo, Grace, Malory | Prescribed output format for review/coordination/judgment agents |
| Checklist externalisation       | Malory                   | Review criteria in `security-checklist.md`, not inlined in agent |
| Two-pass gate classification    | Malory                   | CRITICAL (blocking) vs INFORMATIONAL (advisory)                  |
| Suppressions list               | Malory, Appius           | Explicit "do NOT flag" lists to reduce false positive noise      |
| Read-only by default            | Malory                   | Audit first, modify only with explicit permission                |
| Interactive question protocol   | Grace, Malory            | One issue = one question, numbered options with recommendation   |
| Directive voice                 | All agents               | "Do B. Here's why:" not "Option B might be worth considering."   |
| Context pressure prioritisation | All agents               | Priority hierarchy when context window is tight                  |

## File Tree

```
.github/TENETS.md                      # Layer 0: project constitution
.github/
├── knowledge/                         # Layer 1 + 2: shared knowledge
│   ├── architecture.md
│   ├── build-and-test.md
│   ├── coding-standards.md
│   ├── hardware.md
│   ├── security-surface.md
│   ├── security-checklist.md
│   ├── role-technical.md
│   └── role-editorial.md
├── agents/                            # Layer 3A: Copilot agent definitions
│   ├── appius.agent.md
│   ├── euler.agent.md
│   ├── flo.agent.md
│   ├── grace.agent.md
│   ├── malory.agent.md
│   ├── ruth.agent.md
│   └── terry.agent.md
└── copilot-instructions.md            # Layer 4: Copilot entry point
.claude/
├── agents/                            # Layer 3A: Claude agent definitions
│   ├── appius.md
│   ├── euler.md
│   ├── flo.md
│   ├── grace.md
│   ├── malory.md
│   ├── ruth.md
│   └── terry.md
└── skills/                            # Layer 3B: shared workflow skills
    ├── plan-review/SKILL.md
    ├── review-pr/SKILL.md
    ├── ship-change/SKILL.md
    └── weekly-retro/SKILL.md
CLAUDE.md                              # Layer 4: Claude Code entry point
scripts/
└── check-agent-drift.sh               # Drift detection
```

## Impact

| Metric                                      | Before           | After                             |
|---------------------------------------------|------------------|-----------------------------------|
| Total lines (all agent/instruction files)   | 4,723            | ~2,200                            |
| Project fact duplication instances          | ~45              | 0                                 |
| Files to update when build changes          | 5                | 1                                 |
| Files to update when privacy policy changes | 7                | 1                                 |
| Cost to add a new agent                     | ~200–800 lines   | ~100–160 lines (2 files)          |
| Tools supported                             | 1 (Copilot)      | 2 (Copilot + Claude Code)         |
| Workflow skills (slash commands)            | 0                | 4                                 |
| Drift detection                             | None             | Weekly (`make check-agent-drift`) |

## Future Expansion

Adding a new agent requires:

1. One new file in `.github/agents/` (~50–100 lines of role-specific content)
2. One paired file in `.claude/agents/` (~50–100 lines)
3. An `includes: role-technical.md` or `includes: role-editorial.md` reference in each
4. Zero changes to Layer 0 or Layer 1 unless the project itself changed

Agent candidates for future cycles (not yet scoped): QA/Test Lead, DevOps/Release, UX/Accessibility, Community Manager, Hardware/Sensor, Performance, Docs/Tutorial, DevRel. See the graduated plan for rationale on each.