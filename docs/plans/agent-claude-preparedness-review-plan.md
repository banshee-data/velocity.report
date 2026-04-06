# Agent Knowledge Architecture: Dual-Tool DRY Design

- **Canonical:** [agent-preparedness.md](../platform/operations/agent-preparedness.md)
- **Status:** Complete (all three phases shipped)

This plan defined a DRY, dual-tool knowledge architecture for velocity.report's AI agents, enabling shared project knowledge across Copilot and Claude while reducing duplication and supporting future expansion.

All phases are complete. The canonical reference for the architecture, file tree, agent roster, and future expansion guidance is now **[agent-preparedness.md](../platform/operations/agent-preparedness.md)**. This plan is retained as a design record.

**Scope:** Restructure agent customisation for simultaneous Copilot + Claude Code use; eliminate knowledge duplication; prepare for team expansion to 10–15 agents
**Layers:** docs, ai-agents

---

> **Problem statement and project tenets:** see [agent-preparedness.md](../platform/operations/agent-preparedness.md).

## 3. Current State Audit

### Inventory

> **Note:** Line counts below reflect the state after the persona refinement pass (2026-03-12). Agents have been renamed, given distinct historical personas with tailored voices, and expanded with role-specific methodology. The original pre-rename inventory is in git history.

| Asset                     | Lines     | Unique Content                                           | Voice / Personality                                                      |
| ------------------------- | --------- | -------------------------------------------------------- | ------------------------------------------------------------------------ |
| `copilot-instructions.md` | 301       | Git commit rules, repo structure                         | Neutral (project-level)                                                  |
| `appius.agent.md`         | 1,074     | Infrastructure, durable systems, code review, test       | Long-sighted builder (Appius Claudius Caecus)                            |
| `euler.agent.md`          | 362       | Algorithms, convergence, statistics, maths documentation | Kind, patient, humble researcher (Leonhard Euler)                        |
| `flo.agent.md`            | 331       | Planning, sequencing, risk, coordination                 | Evidence-based, environmental, empathetic planner (Florence Nightingale) |
| `grace.agent.md`          | 396       | Capability mapping, design docs, feature ideation        | Bold pirate innovator, democratiser (Grace Hopper)                       |
| `malory.agent.md`         | 228       | Attack playbook, severity classification                 | Curt, factual, lowercase, one SHOUT max (security researcher)            |
| `ruth.agent.md`           | 381       | Scope modes, tradeoff decisions, decision records        | Firm but kind, data-driven, community-safety focused (RBG)               |
| `terry.agent.md`          | 1,650     | Brand voice, style guide, content audit                  | Humane satire, wit, zero pomposity (Terry Pratchett)                     |
| **Total**                 | **4,723** | ~50%                                                     | **~50% duplicated (down from ~60%)**                                     |

### Duplication Heat Map

> **Note:** Heat map reflects pre-condensation state. Duplication will be eliminated in Phase 1 (knowledge extraction).

| Knowledge Category           | Files Containing It                      | Copies |
| ---------------------------- | ---------------------------------------- | ------ |
| Privacy principles           | ALL 7                                    | 7      |
| Build/test/lint commands     | copilot, appius, grace, florence, malory | 5      |
| SQLite/database facts        | copilot, appius, grace, florence, malory | 5      |
| Tech stack description       | copilot, appius, grace, florence         | 4      |
| Hardware specs (radar/LIDAR) | copilot, appius, grace, malory           | 4      |
| Python venv details          | copilot, appius, grace, florence         | 4      |
| Deployment target (RPi)      | copilot, appius, grace, florence         | 4      |
| Path conventions             | copilot, appius, grace, malory           | 4      |
| British English rules        | copilot, florence, terry                 | 3      |

---

## 4. Target Architecture

### 4.1 Layered Knowledge Model

```
┌─────────────────────────────────────────────────────┐
│  Layer 0: PROJECT TENETS                            │
│  .github/TENETS.md                                  │
│  Privacy · No PII · No cameras · No black-box AI   │
│  Defendable · Provable · Trustworthy                │
│  ← Every agent, every tool inherits this            │
├─────────────────────────────────────────────────────┤
│  Layer 1: SHARED PROJECT KNOWLEDGE                  │
│  .github/knowledge/                                 │
│  ├── build-and-test.md      (make targets, venv)    │
│  ├── architecture.md        (tech stack, DB, paths) │
│  ├── coding-standards.md    (British English, fmt)  │
│  ├── commit-conventions.md  (prefixes, format)      │
│  ├── hardware.md            (radar, LiDAR specs)    │
│  └── security-surface.md   (attack surface map)     │
│  ← Referenced, never duplicated                     │
├─────────────────────────────────────────────────────┤
│  Layer 2: ROLE CLASS MIXINS                         │
│  .github/knowledge/                                 │
│  ├── role-technical.md   (build, venv, packaging,   │
│  │                        test commands, coverage)  │
│  └── role-editorial.md  (style guide, brand voice,  │
│                           tone, audience awareness) │
│  ← Agents include the mixin matching their class    │
├─────────────────────────────────────────────────────┤
│  Layer 3A: AGENT PERSONAS                           │
│  .github/agents/*.agent.md                          │
│  .claude/agents/*.md                                │
│  ← Persona + responsibilities + tool boundaries     │
│  ← Native to each platform; drift-checked weekly    │
├─────────────────────────────────────────────────────┤
│  Layer 3B: SHARED WORKFLOW SKILLS                   │
│  .claude/skills/<workflow>/SKILL.md                 │
│  ← Reusable procedures, slash commands, scripts     │
│  ← Canonical workflow logic shared across tools     │
├─────────────────────────────────────────────────────┤
│  Layer 4: TOOL ENTRY POINTS (thin shims)            │
│  .github/copilot-instructions.md                    │
│  CLAUDE.md                                          │
│  .github/prompts/*.prompt.md (optional wrappers)    │
│  ← Import tenets + shared knowledge + tool config   │
│  ← Tool-specific glue and workflow dispatch only    │
└─────────────────────────────────────────────────────┘
```

### 4.2 What Goes Where

**Layer 0 (`TENETS.md`)** — 1 file, ~30 lines. The project constitution. Changes here are rare and deliberate.

**Layer 1 (`.github/knowledge/`)** — 5–6 files, each a self-contained knowledge module. Any fact that more than one agent needs lives here. Updated when the project changes (new make targets, DB schema changes, new sensor types).

**Layer 2 (role mixins)** — 2 files. Role classes that group agents by capability needs:

| Role Class    | Agents                                     | Knowledge Included                                                       |
| ------------- | ------------------------------------------ | ------------------------------------------------------------------------ |
| **Technical** | Appius, Grace, malory + future tech agents | Build system, venv, test commands, DB details, hardware specs, packaging |
| **Editorial** | Terry, Florence + future non-tech agents   | Brand voice, style guide, audience awareness, documentation standards    |

Some agents may include both mixins (e.g. a future "DevRel" agent who writes docs but also runs code).

**Layer 3A (agent files)** — slim persona definitions stored natively per platform in `.github/agents/` and `.claude/agents/`. Each agent file contains ONLY:

- Name, description, role, responsibilities, tool boundaries, coordination notes
- Role and responsibilities
- Role-specific methodology (malory's red-team playbook, Florence's sequencing rules, etc.)
- References to Layer 1/2 knowledge: `see .github/knowledge/build-and-test.md`
- Inter-agent coordination notes
- Forbidden actions

Agent files define **who** is doing the work, not the reusable runbook for **how** a specific workflow runs. Persona content is duplicated across platforms to preserve native UX and tool controls, then checked by `scripts/check-agent-drift.sh`.

**Layer 3B (workflow skills)** — shared, user-invocable workflows stored in `.claude/skills/`. Each workflow skill contains:

- The reusable procedure for a task such as plan review, PR review, release preparation, or weekly retro
- Slash-command metadata (`name`, `description`, `argument-hint`) and invocation controls
- Optional supporting files: examples, templates, scripts, checklists
- Zero persona prose beyond the minimum needed to run the workflow correctly

Workflow skills define **what procedure runs**. They are the preferred home for multi-step workflows because Claude Code exposes them as native `/` commands and VS Code also discovers skills as slash commands.

**Layer 4 (tool entry points)** — thin shim files:

- `copilot-instructions.md` — references `TENETS.md` + `knowledge/` modules + Copilot-specific config
- `CLAUDE.md` — references the same canonical sources + Claude-specific config
- `.github/prompts/*.prompt.md` — optional Copilot-only wrappers when a workflow needs explicit agent/model/tool routing or a polished VS Code entry point

Prompt files are not canonical workflow definitions. They are thin dispatch shims.

### 4.3 DRY Enforcement Rules

1. **No project fact may appear in more than one file.** If two agents need the same fact, it belongs in Layer 1.
2. **Agent files reference, never restate.** Use `See [build-and-test.md](../../.github/knowledge/build-and-test.md) for make targets and venv setup.`
3. **Tenets are inherited, never copied.** Every agent gets Layer 0 automatically through the tool entry point. Agent files must not restate privacy/ethics principles.
4. **Role mixins are opt-in by class.** Technical agents reference `role-technical.md`. Editorial agents reference `role-editorial.md`. Cross-functional agents reference both.
5. **Persona content is the one exception.** Agent methodology, coordination rules, and forbidden actions are duplicated across Copilot and Claude definitions to maximise each platform's native features. This bounded duplication (~40–80 lines/agent) is drift-checked weekly by `scripts/check-agent-drift.sh`.
6. **Workflow logic lives in skills, not agents.** If a procedure is reusable and user-invocable, it belongs in a `SKILL.md`, not in an agent body.
7. **Prompt files are wrappers only.** Copilot prompt files may select an agent, tools, or model for a workflow, but they must not become a second full copy of the runbook.
8. **Handoffs are optional UX sugar.** They may suggest the next step in Copilot, but they are never the canonical definition of a workflow.

---

## 5. Agent Role Taxonomy

### 5.1 Current Agents (7)

| Agent                                                                                            | Core Domain          | Class     | Prior Name | Unique Domain                                                                                            |
| ------------------------------------------------------------------------------------------------ | -------------------- | --------- | ---------- | -------------------------------------------------------------------------------------------------------- |
| **Euler** ([wiki](https://en.wikipedia.org/wiki/Leonhard_Euler)) (Research / Math)               | Algorithms           | Technical | -          | Algorithms, analytical models, computational foundations, statistical methods, traffic engineering maths |
| **Grace** ([wiki](https://en.wikipedia.org/wiki/Grace_Hopper)) (Architect / Theory)              | System architecture  | Technical | Ictinus    | System architecture, language design, computational models, capability mapping, design docs              |
| **Appius** ([wiki](https://en.wikipedia.org/wiki/Appius_Claudius_Caecus)) (Dev / Implementation) | Execution            | Technical | Hadaly     | Execution strategy, infrastructure thinking, durable systems, code review, test strategy                 |
| **malory** ([wiki](https://en.wikipedia.org/wiki/Thomas_malory)) (Pen Test)                      | Adversarial thinking | Technical | -          | Red-team playbook, vulnerability patterns, adversarial thinking, severity classification                 |
| **Florence** ([wiki](https://en.wikipedia.org/wiki/Florence_Nightingale)) (PM)                   | Coordination         | Editorial | Jess       | Scope definition, sequencing, risk identification, project management                                    |
| **Terry** ([wiki](https://en.wikipedia.org/wiki/Terry_Pratchett)) (Writer)                       | Narrative            | Editorial | Thompson   | Brand voice, copy editing, marketing, content quality                                                    |
| **Ruth** ([wiki](https://en.wikipedia.org/wiki/Ruth_Bader_Ginsburg)) (Justice)                   | Judgment             | Both      | -          | Product direction, tradeoff decisions, scope challenges, taste, scope mode selection                     |

### 5.2 Planned Expansion (future cycle)

These are **candidates only** — to be scoped and prioritised in a future planning cycle. Listed here to validate the architecture scales.

| Candidate              | Class     | Proposed Domain                                        | Rationale                             |
| ---------------------- | --------- | ------------------------------------------------------ | ------------------------------------- |
| **QA / Test Lead**     | Technical | Test strategy, coverage analysis, regression detection | Currently spread across Appius        |
| **DevOps / Release**   | Technical | CI/CD, packaging, deployment, release management       | Currently ad-hoc                      |
| **UX / Accessibility** | Editorial | UI review, accessibility audits, user journey mapping  | Gap in current team                   |
| **Community Manager**  | Editorial | Issue triage, contributor onboarding, community comms  | Complement Terry                      |
| **Compliance / Legal** | Editorial | Licence review, data governance, regulatory awareness  | Strengthen privacy posture            |
| **Hardware / Sensor**  | Technical | Sensor calibration, firmware, signal processing        | Deep domain currently in Appius/Grace |
| **Performance**        | Technical | Profiling, benchmarks, resource constraints (RPi)      | Currently implicit                    |
| **Docs / Tutorial**    | Editorial | Setup guides, tutorials, API docs, examples            | Complement Terry                      |
| **DevRel**             | Both      | Blog posts, demos, conference talks, ecosystem         | Cross-functional                      |

**Note:** Data Analyst candidate was absorbed by the Researcher agent (§5.1). Statistical analysis, data quality, and metric validation are now part of the Researcher’s core domain.

**Architecture validation:** Adding a new agent should require:

1. One new file in `.github/agents/` (~50–100 lines of role-specific content)
2. One `includes: role-technical.md` or `includes: role-editorial.md` reference
3. Zero changes to Layer 0 or Layer 1 (unless the project itself changed)

### 5.3 Role Class Boundaries

**Technical agents need:**

- Make targets and build system patterns
- Python venv location and activation
- Test commands and coverage expectations
- Database schema awareness
- Hardware interface details (role-dependent)
- Packaging and cross-compilation targets
- Path conventions (production vs development)

**Editorial agents need:**

- Brand voice and tone guidelines
- Target audience profiles
- Documentation structure and locations
- Style guide (British English, terminology)
- Content quality checklists

**Executive class agents need (both mixins + additions):**

- All Technical context (to make informed product calls)
- All Editorial context (brand, positioning, audience)
- Decision frameworks, tradeoff methodology
- Scope challenge discipline (expansion/hold/reduction modes)

**Both classes need (via Layer 0):**

- Project tenets (privacy, data ethics, integrity)
- High-level project purpose and positioning

### 5.4 New Agent Scope Notes

> **Note:** Both the Executive (Ruth) and Researcher (Euler) agents referenced below have been fully implemented as of 2026-03-12. The scope notes below document the original design intent for reference.

#### Executive — Ruth (implemented)

**Role:** Product direction and tradeoff decisions. The agent that challenges scope, picks between competing options, and ensures the team builds the _right_ thing — not just the _next_ thing.

**Reference:** internal Ruth wiki page

**Domain:**

- Scope challenges — three explicit modes: EXPANSION (blue-sky thinking), HOLD (rigorous review of existing scope), REDUCTION (cut ruthlessly). User selects mode at start of interaction.
- Build-vs-defer-vs-kill decisions on features and capabilities
- Cross-agent coordination (arbitrates when Grace and Florence disagree on scope)
- Product taste and user empathy — "is this the 10-star version?"
- Tradeoff documentation — records WHY decisions were made, not just WHAT
- Mandatory NOT-in-scope list — every scope decision must explicitly document what was excluded and why

**Class: Both** — needs technical depth to assess feasibility and editorial awareness to assess positioning. The only agent that spans both mixins by design.

**Relationship to other agents:**

- Complements **Grace** (who identifies _what's possible_) by deciding _what to pursue_
- Complements **Florence** (who sequences _how to deliver_) by deciding _whether to deliver_

#### Researcher — Euler (implemented)

**Role:** Algorithmic rigour and mathematical methodology. The agent that validates statistical methods, reviews algorithm implementations, and ensures the maths is sound.

**Reference:** internal Researcher wiki page (to be created)

**Domain:**

- Kalman filtering and state estimation (L5 tracking layer)
- Clustering algorithms — DBSCAN, HDBSCAN, spatial segmentation (L4 perception)
- Background grid settling — EMA/EWA, Welford variance, convergence analysis (L3)
- Traffic engineering statistics — p50, p85, p98 percentiles, confidence intervals
- PCA, OBB computation, coordinate transforms
- Academic methodology — references (`data/maths/references.bib`), reproducibility, peer-review standards
- Tuning parameter validation — convergence bounds, stability analysis (`config/README.maths.md`)
- Future research proposals — IMM, geometry-coherent tracking, velocity-coherent foreground

**Class: Technical** — deep algorithmic domain. Needs build/test knowledge to validate implementations.

**Relationship to other agents:**

- Absorbs the **Data Analyst** candidate from §5.2 — statistical analysis and metric validation are core Researcher responsibilities
- Complements **Appius** (who implements algorithms) by validating the underlying maths
- Complements **Grace** (who proposes new capabilities) by assessing mathematical feasibility
- The “no black-box AI” tenet (Layer 0) is especially relevant — the Researcher ensures all algorithms are inspectable, tuneable, and explainable

---

## 6. Convention Comparison

### 6.1 How Each Tool Discovers Knowledge

| Mechanism              | Copilot (VS Code)                                                     | Claude Code                                                    |
| ---------------------- | --------------------------------------------------------------------- | -------------------------------------------------------------- |
| Root instructions      | `.github/copilot-instructions.md`, `AGENTS.md`, `CLAUDE.md`           | `CLAUDE.md`                                                    |
| Named agents           | `.github/agents/*.agent.md`                                           | `.claude/agents/*.md`                                          |
| Scoped instructions    | `.instructions.md` with `applyTo`; `.claude/rules/` also works        | `.claude/rules/*.md`, nested `CLAUDE.md`                       |
| Skills / workflows     | `SKILL.md` in `.github/skills/`, `.claude/skills/`, `.agents/skills/` | `SKILL.md` in `.claude/skills/` and legacy `.claude/commands/` |
| Prompt-style entry     | `.github/prompts/*.prompt.md`                                         | No native prompt-file equivalent                               |
| Agent-to-agent routing | `handoffs:` in `.agent.md`                                            | Manual chaining, `@` mentions, or skill-driven dispatch        |
| File references        | Read on demand; links can be referenced in prompts/instructions       | Read on demand; skills can bundle supporting files             |

### 6.2 Persona Definition Strategy: Dual-Native Agents

The central persona question is no longer whether Claude supports native agents. It does. The real question is whether persona definitions should be single-source or native per platform.

| Feature                         | Copilot expects                                                     | Claude Code expects                                                        |
| ------------------------------- | ------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| **Agent discovery**             | `.github/agents/*.agent.md` — auto-populates agent picker dropdown  | `.claude/agents/*.md` — managed via `/agents`, auto-delegation, and `@`    |
| **Structured metadata**         | YAML frontmatter: `name`, `description`, `tools`, `handoffs`        | YAML frontmatter: `name`, `description`, `tools`, `disallowedTools`, etc.  |
| **Agent invocation**            | Agent picker, `@AgentName`, prompt file `agent:` routing            | Auto-delegation, `@` mention, `claude --agent`, project `agent` setting    |
| **Per-agent tool restrictions** | `tools:` field in YAML limits available tools                       | `tools` / `disallowedTools` fields limit available tools                   |
| **Per-agent scoping**           | Custom agent context with optional subagent invocation and handoffs | Separate subagent context with independent permissions and optional memory |
| **Hooks / lifecycle control**   | Preview custom-agent hooks                                          | Native per-agent hooks and session hook events                             |

#### Recommendation

Keep **dual-native persona definitions**: `.github/agents/*.agent.md` for Copilot and `.claude/agents/*.md` for Claude.

Why this remains correct:

1. Both tools now have first-class agent concepts, but their metadata, permission controls, and lifecycle hooks are not identical.
2. Persona prompts are short enough that bounded duplication is acceptable.
3. Drift is already managed by `scripts/check-agent-drift.sh`.
4. Attempting to collapse personas into one file would save little and cost native UX on both sides.

### 6.3 Workflow Definition Strategy: Skills First, Not Agent Bodies

The more important design question for the next phase is not persona storage. It is workflow definition.

Current anti-pattern: embedding reusable workflows inside agent definitions.

Why this is the wrong abstraction:

1. It bloats persona prompts with procedural detail that only matters for some tasks.
2. It couples user invocation to a specific persona file rather than a stable workflow name.
3. It forces duplication when the same workflow should be reachable from both Claude and Copilot.
4. It makes review, versioning, and future automation harder because the runbook is hidden inside role prose.

#### Recommended split

| Need                                                                                 | Primitive                  | Reason                                    |
| ------------------------------------------------------------------------------------ | -------------------------- | ----------------------------------------- |
| Persistent role, tone, tool boundary, delegation rules                               | Custom agent / subagent    | Defines **who** is doing the work         |
| Reusable multi-step procedure, slash-invocable by users, can bundle scripts/examples | Skill (`SKILL.md`)         | Defines **what workflow runs**            |
| Copilot-only entry point that should choose a specific agent/model/tools             | Prompt file (`.prompt.md`) | Thin VS Code wrapper, not canonical logic |
| Guided next-step button after one agent finishes                                     | Copilot handoff            | Helpful UX, but not source of truth       |
| Deterministic validation around tool use                                             | Hook                       | Enforcement, not invocation               |

#### Decision

Use **skills as the canonical workflow layer**.

For this repo, the default pattern should be:

1. Put the reusable workflow in `.claude/skills/<workflow>/SKILL.md`.
2. Keep the agent files lean; they may mention which skills they commonly use, but they do not contain the runbook.
3. If VS Code needs a polished entry point that pins a workflow to a persona, add a tiny `.github/prompts/<workflow>.prompt.md` wrapper.
4. If Copilot benefits from a next-step button, add a handoff in the relevant agent file, but point it at the same workflow name.

### 6.4 Why Skills Are the Shared Workflow Surface

Skills are the only primitive in this stack that give us all of the following at once:

- Native `/workflow-name` invocation in Claude Code
- Slash-command discovery in VS Code
- Support for bundled scripts, examples, and checklists
- On-demand loading instead of always-on context bloat
- A plausible single-source workflow layer across both tools

That makes them the correct home for workflows such as:

- `plan-review`
- `review-pr`
- `ship-change`
- `weekly-retro`

#### Invocation pattern by tool

| Tool              | Primary user trigger                              | Native execution pattern                                                  |
| ----------------- | ------------------------------------------------- | ------------------------------------------------------------------------- |
| Claude Code       | `/workflow-name`                                  | Skill runs inline or in forked context; may target a named agent          |
| Copilot / VS Code | `/workflow-name` skill or optional prompt wrapper | Skill loads into current chat, or prompt wrapper selects a specific agent |

#### Rule of thumb

- If the workflow must exist in both tools, start with a skill.
- If the workflow is only lightweight prompt sugar for VS Code, use a prompt file.
- If the workflow exists mainly to move from one persona to the next, use a handoff.
- Do not start by editing an agent file unless the change is genuinely about persona behaviour.

### 6.5 Platform Feature Matrix for Workflows

| Feature                                  | Copilot (VS Code)                             | Claude Code                                         | Recommended use                               |
| ---------------------------------------- | --------------------------------------------- | --------------------------------------------------- | --------------------------------------------- |
| **Persona selection**                    | Agent picker, `@` mention, prompt `agent:`    | Auto-delegation, `@` mention, `--agent`             | Native agents per platform                    |
| **Slash-command workflow**               | Skills and prompt files appear after `/`      | Skills appear after `/`; legacy commands still work | Prefer shared skills                          |
| **Workflow can bundle scripts/examples** | Yes, via skills                               | Yes, via skills                                     | Put reusable runbooks in skills               |
| **Workflow can target isolated context** | Agent/subagent context, prompt-selected agent | `context: fork` plus agent selection in skills      | Use when the workflow is noisy or high-volume |
| **Guided next-step UX**                  | Agent `handoffs:` buttons                     | No direct equivalent                                | Optional Copilot sugar only                   |
| **Deterministic enforcement**            | Hooks (preview)                               | Hooks                                               | Use for guardrails, not for discovery         |
| **Cross-tool portability**               | Skills are portable; prompts are not          | Skills are native; commands are legacy              | Skills are the common denominator             |

### 6.6 Revised Recommendation: Dual-Native Agents + Shared Skills + Thin Shims

The revised architecture is:

1. **Dual-native agents for personas.** Keep `.github/agents/` and `.claude/agents/` paired and drift-checked.
2. **Shared skills for workflows.** Put reusable workflows in `.claude/skills/` so Claude gets native slash commands and VS Code can discover the same skills.
3. **Copilot prompt files only when needed.** Use `.github/prompts/` as a thin wrapper layer when a workflow should force a specific agent, model, or tool set in VS Code.
4. **Handoffs only for ergonomics.** They may suggest the next stage of a sequence in Copilot, but they do not own the procedure.

This answers the immediate product question directly:

- **Claude slash command:** implement the workflow as a skill.
- **Copilot/Codex equivalent:** expose the same workflow as a skill in VS Code; if agent routing matters, add a thin prompt wrapper.
- **What not to do:** keep writing long workflow procedures inside agent definitions.

### 6.7 Drift Detection

Uncontrolled drift between Copilot and Claude agent definitions is the primary risk of the dual-native persona layer. The mitigation is a script that compares paired definitions and surfaces differences for human review.

**Script:** `scripts/check-agent-drift.sh`

The script:

1. Enumerates all `.github/agents/*.agent.md` files
2. Looks for a corresponding `.claude/agents/*.md` file
3. Extracts the persona-relevant sections (stripping YAML frontmatter and platform-specific directives)
4. Compares the normalised content and reports:
   - **Missing pairs** — agent exists in one platform but not the other
   - **Content drift** — persona methodology, coordination rules, or forbidden actions differ
   - **Acceptable divergence** — platform-specific features (tool restrictions, slash commands, handoffs) are excluded from comparison
5. Outputs a Markdown summary suitable for the weekly planning review

**Integration:** Added to `scripts/flo-planning-review.sh` as a new section so drift surfaces alongside backlog health, plan coverage, and open questions in the weekly meeting.

**Make target:** `make check-agent-drift` runs the script standalone.

### 6.8 Finding: Copilot Agent File Reference Resolution

**Question:** If an `.agent.md` body contains `See [build-and-test.md](../../.github/knowledge/build-and-test.md)`, does Copilot eagerly include that file's content when the agent loads?

**Answer: No.** Copilot does not eagerly resolve Markdown links in `.agent.md` bodies. The agent load sequence is:

1. Parse YAML frontmatter (`name`, `description`, `tools`)
2. Load body content as the agent's system prompt (verbatim text)
3. Load workspace instructions (`copilot-instructions.md`) as additional context
4. That's it — no file traversal

Markdown links remain literal text in the prompt. The agent can read those files at runtime, but only on demand.

**Design implication:** knowledge needed on every interaction belongs in the tool entry points or directly in the agent body. Knowledge needed sometimes can safely live in files that the agent reads on demand. This is another reason not to hide critical workflow logic in linked documents inside agent files.

### 6.9 Notes on Shared Knowledge Loading

Skills and prompts do better than agent files here because they support richer loading behaviour:

- Skills progressively load their body and supporting files when invoked.
- Prompts can act as small wrappers that select an agent and tools.
- Agent files still do **not** eagerly dereference Markdown links at load time.

This is another reason to keep reusable runbooks out of agent bodies.

### 6.10 Adopted Patterns from External Stack Analysis

#### Per-Agent Adopted Patterns

**Ruth (Justice)** — scope modes are Ruth's primary tool:

- Three explicit modes: EXPANSION (blue-sky), HOLD (review existing scope), REDUCTION (cut ruthlessly). User selects mode at session start.
- Mandatory NOT-in-scope list on every scope decision — documents what was excluded and why.
- Mandatory output artefacts: scope decision record, tradeoff rationale, NOT-in-scope list.

**Grace (Architect)** — scope modes (secondary) + structured interaction:

- Scope modes available for architectural review (same three modes as Ruth, but Grace uses them to evaluate technical scope, not product scope).
- Interactive question protocol: one issue = one question. Present each finding with numbered options and a recommendation rather than dumping all findings at once.
- Mandatory output artefacts: architecture decision record, ASCII diagrams for system boundaries, failure registry.

**malory (Pen Test)** — review discipline + externalised criteria:

- Two-pass gate classification: CRITICAL (blocking) vs INFORMATIONAL (advisory). Prevents "everything is a security finding" fatigue. Clear escalation rules per category.
- Checklist externalisation: security review criteria live in `.github/knowledge/security-checklist.md`, not inlined in the agent definition. Benefits: evolves independently, referenceable by other agents, version-controlled.
- Suppressions list: explicit list of things NOT to flag. Reduces false positive noise and prevents repeated non-issues from cluttering reviews.
- Read-only by default: audit first, modify only with explicit permission.
- Interactive question protocol: one issue = one question (same discipline as Grace).

**Appius (Dev)** — review suppressions:

- Suppressions list for code review: explicit list of patterns NOT to flag. Prevents style-preference noise from drowning out real issues.

**Florence (PM)** — structured output:

- Mandatory output artefacts: scope summary, risk register, sequencing rationale, completion criteria.
- Trend tracking: JSON snapshot persistence for weekly review metrics (deferred — requires workflow infrastructure beyond agent prompting, revisit alongside §5.2 expansion).

**All agents** — context pressure discipline:

- Priority hierarchy: when context window is tight, prioritise core responsibilities over secondary ones. Explicit guidance prevents shallow across-the-board coverage when the agent should go deep on the most important areas.
- Directive voice: "Do B. Here's why:" not "Option B might be worth considering."

#### Patterns Deferred

- **Non-interactive release automation** — fully automated test-to-PR pipeline. Requires a dedicated release agent. Deferred to Phase 4 expansion (maps to "DevOps / Release" candidate in §5.2).
- **Compiled browser testing** — Playwright daemon for QA. Infrastructure-heavy. Not a priority but validates the "QA / Test Lead" candidate in §5.2.
- **JSON snapshot persistence** — valuable for Florence's weekly review trend tracking but requires workflow infrastructure beyond agent prompting. Revisit alongside §5.2 expansion.

#### Style Decisions

> **DECIDED.** Style gaps from the external stack analysis, resolved below:

| Style Gap                               | External Approach                                                              | Our Current Approach                                            | Decision                                                                                                                                                                                                                     |
| --------------------------------------- | ------------------------------------------------------------------------------ | --------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Voice**                               | Directive: "Do B. Here's why:"                                                 | Mix of directive and suggestive                                 | **Adopted globally** — all agents use assertive voice repo-wide. Suggestive hedging wastes tokens and weakens recommendations.                                                                                               |
| **Output format**                       | Extremely prescriptive: exact headings, tables, registries specified per skill | Open-ended — agents decide their own format                     | **Adopted for malory, Florence, Ruth** — prescribe output format for review/coordination/judgment agents where consistency matters. Leave creative agents (Terry) and implementation agents (Appius, Grace, Euler) flexible. |
| **Read-only default**                   | Review skills only read and comment; write only when explicitly asked          | Agents read and write freely by default                         | **Adopted for malory only** — audit-first discipline. Other agents (including Grace) remain read-write by default.                                                                                                           |
| **Persistent engineering context**      | Reusable coding standards block baked into every planning skill                | Standards in `copilot-instructions.md` but not in agent prompts | **Already solved** — maps directly to our Layer 1 `coding-standards.md`. No action needed.                                                                                                                                   |
| **Suppressions as first-class concept** | Explicit "do NOT flag these" lists per skill                                   | No equivalent — agents flag everything they find                | **Adopted for malory and Appius** — suppressions live in the agent file (not the externalised checklist) so they stay tightly coupled to the persona's review methodology.                                                   |

---

## 7. Implementation Plan

### Phase 1: Tenets + Knowledge Extraction `M`

**Goal:** Eliminate duplication. Create the canonical knowledge layer.

> **Status (2026-03-12): COMPLETE.**
>
> All deliverables created:
>
> - `.github/TENETS.md` — 34 lines, 7 tenets ranked by precedence
> - `.github/knowledge/architecture.md` — 125 lines (tech stack, data flow, schema, repo structure)
> - `.github/knowledge/build-and-test.md` — 94 lines (setup, quality gate, testing, dev servers)
> - `.github/knowledge/coding-standards.md` — 81 lines (British English, commit prefixes, paths, formatting)
> - `.github/knowledge/hardware.md` — 91 lines (OPS243A radar, Hesai P40 LIDAR, Raspberry Pi, gRPC)
> - `.github/knowledge/security-surface.md` — 121 lines (attack surface, vulnerability patterns, privacy)
> - `.github/knowledge/security-checklist.md` — 86 lines (gate classification, severity, checklist, pen test phases)
> - `.github/knowledge/role-technical.md` — 45 lines (shared mixin for Appius, Grace, Euler, malory)
> - `.github/knowledge/role-editorial.md` — 60 lines (shared mixin for Florence, Terry, Ruth)
> - `copilot-instructions.md` refactored: 301 → 95 lines (summary + knowledge module references)
>
> **Total:** 832 lines across 10 files. `copilot-instructions.md` reduced by 68%.

1. Create `.github/TENETS.md` — project constitution (~30 lines)
2. Create `.github/knowledge/` directory with extracted modules:
   - `build-and-test.md` — make targets, dev servers, venv, test commands
   - `architecture.md` — tech stack, DB, data flow, deployment target
   - `coding-standards.md` — British English, formatting, commit conventions
   - `hardware.md` — radar specs, LIDAR specs, serial/UDP interfaces
   - `security-surface.md` — attack surface map (from malory, deduplicated)
   - `security-checklist.md` — externalised review criteria with gate classification
3. Create `.github/knowledge/role-technical.md` and `role-editorial.md` mixins
4. Refactor `copilot-instructions.md` to reference Layer 0–2 instead of inlining

**Acceptance:** `copilot-instructions.md` shrinks from 301 lines to ~95 (summary + references + task guidance). Detailed project facts live in knowledge modules only.

### Phase 2: Agent Condensation `M`

**Goal:** Slim agent files to role-specific content only.

> **Status (2026-03-12): COMPLETE.**
>
> All 7 agent files condensed from 4,723 → 1,858 lines (61% reduction). Each agent file contains only persona, methodology, coordination notes, and Layer 1/2 references — zero duplicated project facts.
>
> **Final line counts:**
>
> - `appius.agent.md` — 395 lines
> - `terry.agent.md` — 332 lines
> - `flo.agent.md` — 285 lines
> - `ruth.agent.md` — 272 lines
> - `malory.agent.md` — 225 lines
> - `euler.agent.md` — 195 lines
> - `grace.agent.md` — 154 lines
>
> **All ≤400 lines** — acceptance criterion met.
>
> **Adopted patterns incorporated:**
>
> - Grace: scope modes, mandatory output artefacts, interactive question protocol
> - Malory: checklist reference, gate classification, suppressions list, read-only-by-default discipline
> - Appius: suppressions list for code review
> - All agents: priority hierarchy under context pressure, directive voice
>
> **Additional deliverables (not in original plan):**
>
> - `.github/agents/README.md` — 23-line agent directory with 2-column portrait table, domains, taglines, and image attribution
> - `.github/agents/portraits/` — 7 × 120px JPEG head shots generated from 1920px Wikimedia/Flickr sources
> - `.github/agents/portraits/generate.py` — permanent script with CLI args, source URLs, crop coordinates, licences, rate-limit-aware downloads
> - Each agent `.agent.md` file includes portrait image (wrapped in `<!-- portrait: not part of the agent instructions -->` comment so AI agents ignore it)

5. Refactor each `.agent.md` file:
   - Remove all duplicated project facts
   - Add references to relevant Layer 1/2 modules
   - Keep only: persona, methodology, coordination notes, forbidden actions
   - Incorporate adopted patterns:
     - Grace: scope modes (expansion/hold/reduction), mandatory output artefacts, interactive question protocol
     - Malory: checklist reference, gate classification, suppressions list, read-only-by-default discipline
     - Appius: suppressions list for code review
     - All agents: priority hierarchy under context pressure, directive voice
6. Validate each agent still functions correctly in Copilot

**Acceptance:** After condensation, agent files contain only persona/role content — no duplicated project facts. Agents with review responsibilities (Malory, Grace) reference externalised checklists rather than inlining criteria. Target: each agent file ≤400 lines once project facts are extracted to Layer 1/2 modules.

### Phase 3: Claude Code Entry Point + Native Agents `M`

**Goal:** Enable Claude Code with full native UX, and introduce a cross-tool workflow layer that is not embedded in agent files.

> **Status (2026-04-06): COMPLETE.**
>
> All deliverables shipped:
>
> - `CLAUDE.md` — Layer 4 Claude entry point with agents + skills sections
> - `.claude/agents/appius.md`, `euler.md`, `grace.md`, `malory.md`, `flo.md`, `terry.md`, `ruth.md` — Claude-native persona definitions
> - `.claude/skills/plan-review/SKILL.md` — plan review slash command
> - `.claude/skills/review-pr/SKILL.md` — PR review slash command
> - `.claude/skills/ship-change/SKILL.md` — quality gate + commit slash command
> - `.claude/skills/weekly-retro/SKILL.md` — weekly planning review slash command
> - `docs/platform/operations/agent-preparedness.md` — updated to reflect complete state; canonical reference going forward
>
> `make check-agent-drift` is available and integrated into `/weekly-retro`.

7. Create `CLAUDE.md` at repo root referencing `TENETS.md` + `knowledge/` modules
8. Create `.claude/agents/` directory with Claude-native persona definitions for each agent:
   - Claude-native frontmatter and prompts
   - Reference shared knowledge modules (same `.github/knowledge/` files)
   - Keep persona methodology and coordination rules only
   - Persona methodology and coordination rules mirrored from `.agent.md` (drift-checked)
9. Create `.claude/skills/` with shared workflow skills for the first reusable workflows:
   - `plan-review`
   - `review-pr`
   - `ship-change`
   - `weekly-retro`
   - Mark side-effecting workflows `disable-model-invocation: true`
   - Bundle checklists, examples, and scripts in the skill directories rather than agent files
10. Add optional `.github/prompts/` wrappers only where VS Code needs explicit agent routing or better UX
11. Test workflow discovery and invocation in both tools:

- Claude Code: `/workflow-name`
- VS Code: slash menu discovers the same skill, or the thin prompt wrapper

12. Document when to use skill vs prompt vs handoff in `CLAUDE.md` and workspace instructions
13. Test Claude Code reads the knowledge modules and agent files correctly

**Acceptance:** Claude Code session has access to equivalent project knowledge as Copilot. Shared knowledge (Layers 0–2) is single-source. Persona definitions are platform-native in both tools. Reusable workflows are slash-invocable without living inside agent files. `make check-agent-drift` reports zero unreviewed drift.

### Phase 4: Agent Team Expansion (future cycle) `L`

**Goal:** Expand from 7 to 10–15 agents using the DRY architecture.

14. Scope and prioritise new agent candidates from §5.2
15. Create new agent files in both platforms (~50–100 lines each, referencing existing knowledge modules)
16. Add or extend shared workflow skills only when the new agent introduces a genuinely reusable procedure
17. Validate new agents and workflows in both Copilot and Claude Code
18. Run `make check-agent-drift` to confirm paired definitions are aligned
19. Add any new knowledge modules if new agents surface previously undocumented domain knowledge

**Acceptance:** Adding a new agent requires two persona files (one per platform) and usually zero workflow changes unless the role introduces a new reusable procedure. Drift check passes.

---

## 8. File Tree (Target State)

```
.github/
├── TENETS.md                           # Layer 0: project constitution
├── knowledge/                          # Layer 1 + 2: shared knowledge
│   ├── architecture.md                 #   tech stack, DB, data flow
│   ├── build-and-test.md               #   make targets, venv, test commands
│   ├── coding-standards.md             #   British English, formatting, commits
│   ├── hardware.md                     #   radar, LIDAR, serial, UDP
│   ├── security-surface.md             #   attack surface map
│   ├── security-checklist.md           #   review criteria + gate classification
│   ├── role-technical.md               #   mixin for technical agents
│   └── role-editorial.md               #   mixin for editorial agents
├── agents/                             # Layer 3: Copilot agent definitions
│   ├── euler.agent.md                  #   Research / Math (technical)
│   ├── grace.agent.md                  #   Architect (technical)
│   ├── appius.agent.md                 #   Dev (technical)
│   ├── malory.agent.md                 #   Pen Test (technical)
│   ├── flo.agent.md                    #   PM (editorial)
│   ├── terry.agent.md                  #   Writer (editorial)
│   ├── ruth.agent.md                   #   Executive (both)
│   └── [future-agents].agent.md        #   further expansion planned
├── copilot-instructions.md             # Layer 4: Copilot entry point (slim)
├── prompts/                            # Layer 4: optional Copilot workflow shims
│   ├── plan-review.prompt.md
│   ├── review-pr.prompt.md
│   └── [workflow].prompt.md
.claude/
├── agents/                             # Layer 3: Claude agent definitions
│   ├── euler.md                        #   Research / Math (technical) — Claude-native
│   ├── grace.md                        #   Architect (technical)
│   ├── appius.md                       #   Dev (technical)
│   ├── malory.md                       #   Pen Test (technical)
│   ├── flo.md                          #   PM (editorial)
│   ├── terry.md                        #   Writer (editorial)
│   ├── ruth.md                         #   Executive (both)
│   └── [future-agents].md              #   further expansion planned
├── skills/                             # Layer 3B: shared workflow skills
│   ├── plan-review/
│   │   └── SKILL.md
│   ├── review-pr/
│   │   └── SKILL.md
│   └── [workflow]/
│       └── SKILL.md
CLAUDE.md                               # Layer 4: Claude entry point → refs .github/ + .claude/
scripts/
└── check-agent-drift.sh                # Drift detection between paired definitions
```

**Shared knowledge:** `.github/TENETS.md` and `.github/knowledge/` are the single source of truth for project facts — referenced by both Copilot and Claude agent definitions.
**Persona duplication:** Agent methodology, coordination rules, and forbidden actions are duplicated across `.github/agents/` and `.claude/agents/`. Drift is detected weekly by `scripts/check-agent-drift.sh`.
**Workflow single-sourcing:** Reusable procedures live in `.claude/skills/`. Copilot prompt files are optional wrappers, not second copies of the runbook.

---

## 9. Estimated Impact

| Metric                                      | Original       | Post-persona (pre-work) | Current (post-Phase 2)           | Target (post-Phase 3)                         | Change (original→current) |
| ------------------------------------------- | -------------- | ----------------------- | -------------------------------- | --------------------------------------------- | ------------------------- |
| Total lines (all agent/instruction files)   | 3,456          | 4,723                   | **1,953** (1,858 + 95)           | ~2,200 (+Claude shim)                         | **-43%**                  |
| _Project fact_ duplication instances        | ~45            | ~45                     | **0**                            | 0                                             | **-100%**                 |
| _Persona_ duplication instances             | 0              | 0                       | 0                                | ~7–15 (bounded, drift-checked)                | —                         |
| _Workflow_ duplication instances            | n/a            | n/a                     | n/a                              | **0 canonical / optional thin wrappers only** | —                         |
| Agent persona completeness                  | Generic        | Full (all 7 distinct)   | **Full**                         | Full                                          | ✅                        |
| Agent voice/personality distinction         | None           | All 7 differentiated    | **Maintained**                   | Maintained                                    | ✅                        |
| Files to update when build changes          | 5              | 5                       | **1**                            | 1                                             | **-80%**                  |
| Files to update when privacy policy changes | 6              | 7                       | **1**                            | 1                                             | **-86%**                  |
| Cost to add a new agent                     | ~200–800 lines | ~200–800 lines          | **~50–100 lines** (1 file)       | ~100–160 lines (2 files)                      | **-88%**                  |
| Tools supported                             | 1 (Copilot)    | 1 (Copilot)             | **1 (Copilot)**                  | 2 (Copilot + Claude)                          | —                         |
| Platform features used                      | ~60%           | ~60%                    | **~60%**                         | ~95%                                          | —                         |
| Drift detection                             | None           | None                    | **Script ready** (no Claude yet) | Weekly automated                              | ✅                        |
| Agent portraits                             | None           | None                    | **7 × 120px** (1920px sources)   | 7 × 120px                                     | ✅                        |
| Agent README / directory                    | None           | None                    | **23 lines** (table + attrib)    | Maintained                                    | ✅                        |

---

## 10. Open Questions

- [x] ~~Does `.github/knowledge/` directory convention work with Copilot's file discovery, or do agents need explicit `#file` references?~~ — Resolved: **yes, the convention works.** `copilot-instructions.md` is auto-loaded into agent context; knowledge modules are fetched on demand via tools when the agent follows the inline `see .github/knowledge/...` signposts. Explicit `#file` references would not help — Copilot does not eagerly resolve them (see §6.4). Validated empirically across the Phase 2 work.
- [x] ~~Should Claude Code agent personas be sections in `CLAUDE.md` or separate files in `.claude/`?~~ — Resolved: separate files in `.claude/agents/`. Platform-native definitions maximise feature coverage.
- [x] ~~Single source vs dual agent definitions?~~ — Resolved: **Option B (dual native) with drift detection.** Feature matrix (§6.5) showed Option A sacrifices 6 real UX features for Claude users. Persona duplication is bounded (~40–80 lines/agent) and drift-checked weekly. See §6.6.
- [x] ~~Does Copilot resolve `#file` references inside `.agent.md` at agent-load time?~~ — Investigated. **No, it does not eagerly resolve Markdown links.** See §6.4 for full analysis. This ruled out Option C and informed the decision to keep persona content directly in agent files rather than factoring it into shared includes.
- [x] ~~Where should reusable workflows live?~~ — Resolved: **skills first**. Shared workflows live in `.claude/skills/` so Claude gets native slash commands and VS Code can discover the same skills. Copilot prompt files are optional thin wrappers, not the source of truth.
- [x] ~~Which new agents to prioritise?~~ — Partially resolved: Executive and Researcher added to core roster (§5.1). Remaining candidates in §5.2 deferred to future cycle.
- [x] ~~Should `TENETS.md` be enforced via a CI check (e.g. grep for PII-related code patterns)?~~ — Resolved: **yes.** Add CI checks that enforce core tenets (e.g. grep for PII-related code patterns, camera/licence-plate references, cloud transmission endpoints). Implement as part of Phase 1 alongside `TENETS.md` creation.
- [x] ~~Review cadence — quarterly staleness check for knowledge modules?~~ — Resolved: weekly drift check via `scripts/check-agent-drift.sh`, integrated into `flo-planning-review.sh`. Knowledge module staleness reviewed alongside agent drift.

---

## 11. Remaining Work Checklist

### Phase 3: Claude Code Entry Point ✅ Complete

- [x] Create `CLAUDE.md` at repo root — reference `TENETS.md`, `knowledge/` modules, agent roster with file paths
- [x] Create `.claude/agents/euler.md` — Claude-native persona
- [x] Create `.claude/agents/grace.md`
- [x] Create `.claude/agents/appius.md`
- [x] Create `.claude/agents/malory.md`
- [x] Create `.claude/agents/flo.md`
- [x] Create `.claude/agents/terry.md`
- [x] Create `.claude/agents/ruth.md`
- [x] Create `.claude/skills/plan-review/SKILL.md`
- [x] Create `.claude/skills/review-pr/SKILL.md`
- [x] Create `.claude/skills/ship-change/SKILL.md`
- [x] Create `.claude/skills/weekly-retro/SKILL.md`
- [x] Update `docs/platform/operations/agent-preparedness.md` to reflect complete state
- [x] Mark plan Complete; canonical architecture lives in ops doc

### Phase 4: Agent Team Expansion (future cycle)

- [ ] Scope and prioritise new agent candidates from §5.2
- [ ] Create new agent files in both platforms
- [ ] Validate new agents in both Copilot and Claude Code
- [ ] Run `make check-agent-drift` to confirm alignment
- [ ] Add knowledge modules if new agents surface undocumented domain knowledge

### Deferred / Low Priority

- [ ] CI check enforcing TENETS.md (grep for PII patterns, camera references, cloud endpoints)
- [ ] Revisit whether any workflow needs Copilot-only prompt sugar after the shared skill layer lands
- [ ] JSON snapshot persistence for Flo's weekly review trend tracking
- [ ] Non-interactive release automation (DevOps agent prerequisite)
