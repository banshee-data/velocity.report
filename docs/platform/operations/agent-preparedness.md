# Agent Knowledge Architecture

Active plan: [agent-claude-preparedness-review-plan.md](../../plans/agent-claude-preparedness-review-plan.md)

## Problem

velocity.report uses seven named AI agents defined as Copilot `.agent.md`
files. The system suffered from three structural problems:

1. **Massive duplication** — 4,723 lines across 7 agent files with ~50%
   duplicated content. Privacy principles, build commands, SQLite facts, and
   Python venv details appeared in multiple files.
2. **Tool lock-in** — all knowledge was in Copilot-specific formats. Adding
   Claude Code meant duplicating everything or restructuring.
3. **Scaling problem** — expanding from 7 to 10–15 agents with each agent
   carrying its own copy of project knowledge would mean maintaining 15+
   copies of the same facts.

## Layered Knowledge Model

```
┌─────────────────────────────────────────────────────┐
│  Layer 0: PROJECT TENETS                            │
│  .github/TENETS.md                                  │
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
│  └── role-editorial.md  (style, brand, audience)    │
│  ← Agents include mixin matching their class        │
├─────────────────────────────────────────────────────┤
│  Layer 3: AGENT DEFINITIONS (single source)         │
│  .github/agents/*.agent.md                          │
│  ← Persona + responsibilities + Layer 1/2 refs      │
│  ← NO project facts restated here                   │
├─────────────────────────────────────────────────────┤
│  Layer 4: TOOL ENTRY POINTS (thin shims)            │
│  .github/copilot-instructions.md                    │
│  CLAUDE.md                                          │
│  ← Import tenets + shared knowledge + tool config   │
└─────────────────────────────────────────────────────┘
```

## DRY Enforcement Rules

1. **No project fact may appear in more than one file.** If two agents need
   the same fact, it belongs in Layer 1.
2. **Agent files reference, never restate.** Use
   `See build-and-test.md for make targets and venv setup.`
3. **Tenets are inherited, never copied.** Every agent gets Layer 0
   automatically through the tool entry point.
4. **Role mixins are opt-in by class.** Technical agents reference
   `role-technical.md`; editorial agents reference `role-editorial.md`.
5. **Persona content is the one exception.** Methodology and coordination
   rules are duplicated across Copilot and Claude definitions because
   neither platform supports shared includes at agent-load time. This
   bounded duplication (~40–80 lines/agent) is drift-checked weekly.

## Agent Roster

| Agent      | Domain               | Class     | Unique Domain                                                      |
| ---------- | -------------------- | --------- | ------------------------------------------------------------------ |
| **Euler**  | Algorithms           | Technical | Statistical methods, Kalman filtering, convergence analysis        |
| **Grace**  | System architecture  | Technical | Capability mapping, design docs, computational models              |
| **Appius** | Execution            | Technical | Durable systems, code review, test strategy, infrastructure        |
| **Malory** | Adversarial thinking | Technical | Red-team playbook, vulnerability patterns, severity classification |
| **Flo**    | Coordination         | Editorial | Scope definition, sequencing, risk identification                  |
| **Terry**  | Narrative            | Editorial | Brand voice, copy editing, content quality                         |
| **Ruth**   | Judgment             | Both      | Product direction, tradeoff decisions, scope challenges            |

### Role Class Boundaries

**Technical agents need:** Make targets, build system, Python venv, test
commands, database schema, hardware interfaces, packaging targets, path
conventions.

**Editorial agents need:** Brand voice, tone guidelines, target audience
profiles, documentation structure, style guide (British English).

**Executive agents need:** Both mixins plus decision frameworks, tradeoff
methodology, scope challenge discipline.

## Platform Strategy: Option B (Dual Native with Drift Detection)

Agent personas are defined natively for each platform. Shared project
knowledge (Layers 0–2) remains single-source.

### What Gets Duplicated (Bounded)

| Content                    | Duplicated?        | Copilot                       | Claude                                     |
| -------------------------- | ------------------ | ----------------------------- | ------------------------------------------ |
| Project tenets             | No                 | `.github/TENETS.md`           | `.github/TENETS.md` (same file)            |
| Build/test knowledge       | No                 | `.github/knowledge/`          | `.github/knowledge/` (same files)          |
| Role mixins                | No                 | `.github/knowledge/role-*.md` | `.github/knowledge/role-*.md` (same files) |
| Persona name + description | Yes (~2 lines)     | YAML frontmatter              | Inline in `.claude/agents/*.md`            |
| Persona methodology        | Yes (~30–60 lines) | `.agent.md` body              | `.claude/agents/*.md`                      |
| Tool restrictions          | Copilot-only       | YAML `tools:` field           | n/a                                        |
| Coordination rules         | Yes (~10–20 lines) | `.agent.md` body              | `.claude/agents/*.md`                      |

Total duplication per agent: ~40–80 lines of role-specific content.

### Drift Detection

`scripts/check-agent-drift.sh` compares paired definitions weekly:

1. Enumerates all `.github/agents/*.agent.md` files
2. Looks for corresponding `.claude/agents/*.md` file
3. Extracts persona sections (strips YAML and platform-specific directives)
4. Reports missing pairs, content drift, and acceptable divergence
5. Integrated into `scripts/flo-planning-review.sh`

Make target: `make check-agent-drift`

## Copilot Agent File Reference Resolution (Key Finding)

Copilot does **not** eagerly resolve Markdown links in `.agent.md` bodies.
The load sequence is:

1. Parse YAML frontmatter (`name`, `description`, `tools`)
2. Load body content as system prompt (verbatim text)
3. Load workspace instructions (`copilot-instructions.md`) as context
4. No file traversal

References like `[build-and-test.md](../../.github/knowledge/build-and-test.md)`
remain literal text. The agent can read those files on demand using tools,
but not eagerly at load time. This:

- Rules out "wrapper" `.agent.md` files that `#include` shared content
- Means Layer 1/2 knowledge that agents need for every interaction should be
  inlined in `copilot-instructions.md`, not just referenced
- Reinforces keeping persona content directly in agent bodies

## Adopted Design Patterns

| Pattern                         | Applied To               | Description                                                      |
| ------------------------------- | ------------------------ | ---------------------------------------------------------------- |
| Scope modes                     | Ruth (primary), Grace    | EXPANSION / HOLD / REDUCTION — user selects at session start     |
| Mandatory output artefacts      | Ruth, Flo, Grace, Malory | Prescribed output format for review/coordination/judgment agents |
| Checklist externalisation       | Malory                   | Review criteria in `security-checklist.md`, not inlined in agent |
| Two-pass gate classification    | Malory                   | CRITICAL (blocking) vs INFORMATIONAL (advisory)                  |
| Suppressions list               | Malory, Appius           | Explicit "do NOT flag" lists to reduce false positive noise      |
| Read-only by default            | Malory                   | Audit first, modify only with explicit permission                |
| Interactive question protocol   | Grace, Malory            | One issue = one question, numbered options with recommendation   |
| Directive voice                 | All agents               | "Do B. Here's why:" not "Option B might be worth considering."   |
| Context pressure prioritisation | All agents               | Priority hierarchy when context window is tight                  |

## Target File Tree

```
.github/
├── TENETS.md                          # Layer 0
├── knowledge/                         # Layer 1 + 2
│   ├── architecture.md
│   ├── build-and-test.md
│   ├── coding-standards.md
│   ├── hardware.md
│   ├── security-surface.md
│   ├── security-checklist.md
│   ├── role-technical.md
│   └── role-editorial.md
├── agents/                            # Layer 3 (Copilot)
│   ├── appius.agent.md
│   ├── euler.agent.md
│   ├── flo.agent.md
│   ├── grace.agent.md
│   ├── malory.agent.md
│   ├── ruth.agent.md
│   └── terry.agent.md
├── copilot-instructions.md            # Layer 4 (Copilot)
.claude/
├── agents/                            # Layer 3 (Claude)
│   ├── appius.md … terry.md
CLAUDE.md                              # Layer 4 (Claude)
scripts/
└── check-agent-drift.sh              # Drift detection
```

## Impact

| Metric                               | Before (pre-work) | After (Phase 2) | Target (Phase 3)         |
| ------------------------------------ | ----------------- | --------------- | ------------------------ |
| Total lines (all agent/instruction)  | 4,723             | 1,953           | ~2,200 (+Claude shim)    |
| Project fact duplication instances   | ~45               | 0               | 0                        |
| Files to update when build changes   | 5                 | 1               | 1                        |
| Files to update when privacy changes | 7                 | 1               | 1                        |
| Cost to add a new agent              | ~200–800 lines    | ~50–100 lines   | ~100–160 lines (2 files) |
| Tools supported                      | 1 (Copilot)       | 1 (Copilot)     | 2 (Copilot + Claude)     |

## Current Status

- **Phase 1 (Tenets + Knowledge Extraction):** Complete. 10 files created,
  `copilot-instructions.md` reduced from 301 to 95 lines (68% reduction).
- **Phase 2 (Agent Condensation):** Complete. 4,723 → 1,858 lines (61%
  reduction). All agents ≤400 lines. Zero duplicated project facts.
- **Phase 3 (Claude Code Entry Point):** Pending. Drift detection
  infrastructure already built (scripts, make target, flo integration).
  Remaining: create `CLAUDE.md`, `.claude/agents/`, and test.
