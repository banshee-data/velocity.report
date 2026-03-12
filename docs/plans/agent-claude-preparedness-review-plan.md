# Agent Knowledge Architecture: Dual-Tool DRY Design

**Status:** Draft
**Created:** 2026-03-12
**Scope:** Restructure agent customisation for simultaneous Copilot + Claude Code use; eliminate knowledge duplication; prepare for team expansion to 10–15 agents

---

## 1. Problem Statement

velocity.report uses seven named AI agents defined as Copilot `.agent.md` files. The system works, but suffers from three structural problems:

1. **Massive duplication** — 3,456 lines across 6 files with ~45 duplication instances. Privacy principles appear in all 6 files. Build commands appear in 5. SQLite facts in 5. Python venv in 4.
2. **Tool lock-in** — all knowledge is in Copilot-specific formats (`.agent.md`, `copilot-instructions.md`). Adding Claude Code means either duplicating everything into `CLAUDE.md` or restructuring.
3. **Scaling problem** — planning to expand from 5 to 10–15 agents. Current approach (each agent carries its own copy of project knowledge) would mean maintaining 15+ copies of the same facts.

**This is NOT a migration away from Copilot.** Both tools will be used simultaneously. The goal is a DRY knowledge architecture that serves both.

---

## 2. Project Tenets (Global Scope)

These tenets apply to **every agent, every tool, every interaction**. They are non-negotiable project-wide principles that must be enforced at the outermost scope — before any role-specific or tool-specific knowledge loads.

### Data Ethics

- **No PII collection** — no names, addresses, licence plates, or personally identifiable information, ever
- **No cameras** — velocity measurements only; no image capture, no video, no optical surveillance
- **No black-box AI** — no opaque ML models making unauditable decisions about traffic data; all algorithms must be inspectable, tuneable, and explainable
- **Privacy by design** — local-only data storage, no cloud transmission, user owns their data

### Data Integrity

- **Defendable data** — every measurement must trace back to a calibrated sensor with known error bounds
- **Provable methodology** — use traffic engineering standards (p50, p85, p98 percentiles); methodology must withstand scrutiny from municipal officials and traffic engineers
- **Trustworthy reporting** — no cherry-picking, no statistical manipulation; reports present the full picture including limitations and confidence intervals

### Enforcement

These tenets belong in a single canonical file (proposed: `.github/TENETS.md`) that both `copilot-instructions.md` and `CLAUDE.md` reference. Agent files must NOT restate them — they inherit them from the global scope.

---

## 3. Current State Audit

### Inventory

| Asset                     | Lines     | Unique Content                           | Duplicated Content                            |
| ------------------------- | --------- | ---------------------------------------- | --------------------------------------------- |
| `copilot-instructions.md` | 417       | Git commit rules, repo structure         | Everything else shared                        |
| `hadaly.agent.md`         | 281       | Radar commands, test strategy            | Build, DB, paths, privacy, venv               |
| `ictinus.agent.md`        | 664       | Capability map, vision, opportunities    | Tech stack, DB, hardware, privacy, venv       |
| `jess.agent.md`           | 494       | Planning methodology, workflows          | Tech stack, constraints, build, privacy, venv |
| `malory.agent.md`         | 717       | Attack playbook, severity classification | Attack surface, DB, hardware, privacy         |
| `thompson.agent.md`       | 883       | Brand voice, style guide, content audit  | Privacy, repo structure                       |
| **Total**                 | **3,456** | ~40%                                     | **~60% duplicated**                           |

### Duplication Heat Map

| Knowledge Category           | Files Containing It                    | Copies |
| ---------------------------- | -------------------------------------- | ------ |
| Privacy principles           | ALL 6                                  | 6      |
| Build/test/lint commands     | copilot, hadaly, ictinus, jess, malory | 5      |
| SQLite/database facts        | copilot, hadaly, ictinus, jess, malory | 5      |
| Tech stack description       | copilot, hadaly, ictinus, jess         | 4      |
| Hardware specs (radar/LIDAR) | copilot, hadaly, ictinus, malory       | 4      |
| Python venv details          | copilot, hadaly, ictinus, jess         | 4      |
| Deployment target (RPi)      | copilot, hadaly, ictinus, jess         | 4      |
| Path conventions             | copilot, hadaly, ictinus, malory       | 4      |
| British English rules        | copilot, jess, thompson                | 3      |

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
│  ├── hardware.md            (radar, LIDAR specs)    │
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
│  Layer 3: AGENT DEFINITIONS (single source)         │
│  .github/agents/*.agent.md                          │
│  ← Persona + responsibilities + Layer 1/2 refs      │
│  ← NO project facts restated here                   │
│  ← Copilot loads natively; CLAUDE.md references     │
├─────────────────────────────────────────────────────┤
│  Layer 4: TOOL ENTRY POINTS (thin shims)            │
│  .github/copilot-instructions.md                    │
│  CLAUDE.md                                          │
│  ← Import tenets + shared knowledge + tool config   │
│  ← Minimal tool-specific glue only                  │
└─────────────────────────────────────────────────────┘
```

### 4.2 What Goes Where

**Layer 0 (`TENETS.md`)** — 1 file, ~30 lines. The project constitution. Changes here are rare and deliberate.

**Layer 1 (`.github/knowledge/`)** — 5–6 files, each a self-contained knowledge module. Any fact that more than one agent needs lives here. Updated when the project changes (new make targets, DB schema changes, new sensor types).

**Layer 2 (role mixins)** — 2 files. Role classes that group agents by capability needs:

| Role Class    | Agents                                     | Knowledge Included                                                       |
| ------------- | ------------------------------------------ | ------------------------------------------------------------------------ |
| **Technical** | Appius, Grace, Malory + future tech agents | Build system, venv, test commands, DB details, hardware specs, packaging |
| **Editorial** | Terry, Florence + future non-tech agents   | Brand voice, style guide, audience awareness, documentation standards    |

Some agents may include both mixins (e.g. a future "DevRel" agent who writes docs but also runs code).

**Layer 3 (agent files)** — slim persona definitions stored **once** in `.github/agents/`. Each agent file contains ONLY:

- YAML frontmatter (name, description) — Copilot uses this natively; Claude Code ignores it harmlessly
- Role and responsibilities
- Role-specific methodology (Malory's red-team playbook, Florence's sequencing rules, etc.)
- References to Layer 1/2 knowledge: `see .github/knowledge/build-and-test.md`
- Inter-agent coordination notes
- Forbidden actions

Both tools read from the same files. Copilot discovers them via the `.agent.md` convention. `CLAUDE.md` references them explicitly (e.g. `See .github/agents/malory.agent.md for security review persona`). No separate `.claude/agents/` directory — that would be duplication.

**Layer 4 (tool entry points)** — thin shim files:

- `copilot-instructions.md` — references `TENETS.md` + `knowledge/` modules + Copilot-specific config
- `CLAUDE.md` — references the same canonical sources + Claude-specific config

### 4.3 DRY Enforcement Rules

1. **No project fact may appear in more than one file.** If two agents need the same fact, it belongs in Layer 1.
2. **Agent files reference, never restate.** Use `See [build-and-test.md](.github/knowledge/build-and-test.md) for make targets and venv setup.`
3. **Tenets are inherited, never copied.** Every agent gets Layer 0 automatically through the tool entry point. Agent files must not restate privacy/ethics principles.
4. **Role mixins are opt-in by class.** Technical agents reference `role-technical.md`. Editorial agents reference `role-editorial.md`. Cross-functional agents reference both.
5. **Persona content is the one exception.** Agent methodology, coordination rules, and forbidden actions are duplicated across Copilot and Claude definitions to maximise each platform's native features. This bounded duplication (~40–80 lines/agent) is drift-checked weekly by `scripts/check-agent-drift.sh`. This is annoying, if platform parity allowed us to we would remove this anti-pattern.

---

## 5. Agent Role Taxonomy

### 5.1 Current Agents (7)

| Agent                                                                                            | Core Domain          | Class     | Prior Name | Unique Domain                                                                                            |
| ------------------------------------------------------------------------------------------------ | -------------------- | --------- | ---------- | -------------------------------------------------------------------------------------------------------- |
| **Euler** ([wiki](https://en.wikipedia.org/wiki/Leonhard_Euler)) (Research / Math)               | Algorithms           | Technical | -          | Algorithms, analytical models, computational foundations, statistical methods, traffic engineering maths |
| **Grace** ([wiki](https://en.wikipedia.org/wiki/Grace_Hopper)) (Architect / Theory)              | System architecture  | Technical | Ictinus    | System architecture, language design, computational models, capability mapping, design docs              |
| **Appius** ([wiki](https://en.wikipedia.org/wiki/Appius_Claudius_Caecus)) (Dev / Implementation) | Execution            | Technical | Hadaly     | Execution strategy, infrastructure thinking, durable systems, code review, test strategy                 |
| **Malory** ([wiki](https://en.wikipedia.org/wiki/Thomas_Malory)) (Pen Test)                      | Adversarial thinking | Technical | -          | Red-team playbook, vulnerability patterns, adversarial thinking, severity classification                 |
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

#### Executive (TBD name)

**Role:** Product direction and tradeoff decisions. The agent that challenges scope, picks between competing options, and ensures the team builds the _right_ thing — not just the _next_ thing.

**Reference:** [wiki page](docs/wiki/ruth.md)

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

#### Researcher (TBD name)

**Role:** Algorithmic rigour and mathematical methodology. The agent that validates statistical methods, reviews algorithm implementations, and ensures the maths is sound.

**Reference:** [wiki page](/docs/wiki/researcher.md) (to be created)

**Domain:**

- Kalman filtering and state estimation (L5 tracking layer)
- Clustering algorithms — DBSCAN, HDBSCAN, spatial segmentation (L4 perception)
- Background grid settling — EMA/EWA, Welford variance, convergence analysis (L3)
- Traffic engineering statistics — p50, p85, p98 percentiles, confidence intervals
- PCA, OBB computation, coordinate transforms
- Academic methodology — references (`docs/references.bib`), reproducibility, peer-review standards
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

| Mechanism           | Copilot (VS Code)                      | Claude Code                            |
| ------------------- | -------------------------------------- | -------------------------------------- |
| Root instructions   | `.github/copilot-instructions.md`      | `CLAUDE.md` (repo root)                |
| Named agents        | `.github/agents/*.agent.md`            | Not natively supported                 |
| Scoped instructions | `.instructions.md` with `applyTo`      | Subdirectory `CLAUDE.md` files         |
| Skills              | `SKILL.md` (on-demand loading)         | Not supported                          |
| File references     | Agent reads referenced files on demand | `CLAUDE.md` can import with file paths |

### 6.2 Agent Definition Strategy: DRY vs Native UX

The central design question: should agent personas live in one place or two?

#### What each tool expects

| Feature                         | Copilot expects                                                    | Claude Code expects                                                      |
| ------------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------ |
| **Agent discovery**             | `.github/agents/*.agent.md` — auto-populates agent picker dropdown | No native agent concept — personas are prompt-driven                     |
| **Structured metadata**         | YAML frontmatter: `name`, `description`, `tools`                   | No structured metadata — ignores YAML but doesn't break on it            |
| **Agent invocation**            | `@AgentName` mention in chat                                       | User prompts "act as Malory" or `CLAUDE.md` directs which persona to use |
| **Per-agent tool restrictions** | `tools:` field in YAML limits available tools                      | No equivalent — all tools always available                               |
| **Per-agent scoping**           | Each `.agent.md` is a separate context window                      | Must read referenced files explicitly                                    |

#### Option A: Single source in `.github/agents/` (current plan)

One set of `.agent.md` files. Copilot discovers natively. `CLAUDE.md` references them.

|                       | Assessment                                                                                                                                                              |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **DRY**               | ✅ Perfect — zero duplication                                                                                                                                           |
| **Copilot UX**        | ✅ Full native experience — agent picker, @mentions, tool restrictions                                                                                                  |
| **Claude UX**         | ⚠️ Degraded — no agent picker; user must name persona manually; Claude reads Copilot's YAML without understanding `tools:` restrictions; no per-agent context windowing |
| **Alignment risk**    | ✅ None — single source                                                                                                                                                 |
| **YAML harmlessness** | ✅ Claude treats YAML frontmatter as opaque text — it doesn't break, but `tools:` restrictions are silently ignored                                                     |
| **New agent cost**    | 1 file                                                                                                                                                                  |

**Copilot-specific features lost by Claude:** agent picker dropdown, `@mention` invocation, `tools:` restrictions. These are UI conveniences, not knowledge gaps — Claude still gets the full persona definition.

#### Option B: Two native sets (`.github/agents/` + `.claude/` or inline in `CLAUDE.md`)

Copilot gets `.agent.md` files. Claude gets its own persona definitions in whatever format it prefers.

|                    | Assessment                                                                                    |
| ------------------ | --------------------------------------------------------------------------------------------- |
| **DRY**            | ❌ Violated — persona definitions duplicated                                                  |
| **Copilot UX**     | ✅ Full native experience                                                                     |
| **Claude UX**      | ✅ Could tailor persona format to Claude's strengths (no YAML noise, Claude-native prompting) |
| **Alignment risk** | 🔴 Real — persona drift between two copies; grows linearly with agent count                   |
| **New agent cost** | 2 files + sync obligation                                                                     |

**Sync overhead at scale:**

| Team Size | Files to Sync | Drift Risk | Annual Maintenance      |
| --------- | ------------- | ---------- | ----------------------- |
| 5 agents  | 10            | Low        | ~2h/quarter             |
| 10 agents | 20            | Medium     | ~5h/quarter             |
| 15 agents | 30            | High       | ~8h/quarter + incidents |

At 15 agents, that's 30 persona files to keep aligned, with every methodology update requiring two edits. Drift is not hypothetical — it's the exact problem the current architecture has (§3 audit: 45 duplication instances, many already drifted).

#### Option C: ~~Claude-native format as source, Copilot imports~~ (Ruled Out)

Define personas in a tool-agnostic format. Copilot `.agent.md` files become thin wrappers that `#include` the shared definition.

|                    | Assessment                                                                                                                                               |
| ------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **DRY**            | ⚠️ Mostly — persona content is single-source but Copilot wrappers add ~5 lines each                                                                      |
| **Copilot UX**     | ❌ **Copilot does NOT resolve file references at agent-load time** (see §6.4). Agent picker would show agents with empty/incomplete persona definitions. |
| **Claude UX**      | ✅ Native format, no YAML noise                                                                                                                          |
| **Discovery risk** | ❌ **Confirmed blocker** — agent body content is loaded verbatim; Markdown links are not dereferenced                                                    |
| **Complexity**     | More indirection; harder to reason about what an agent "sees"                                                                                            |

**Verdict:** Ruled out. Empirical investigation (§6.4) confirms Copilot does not eagerly resolve file references in `.agent.md` bodies. Wrapper agents would appear empty in the agent picker. This option is architecturally unsound.

#### Recommendation: Option A (single source in `.github/agents/`)

**Rationale:**

1. **DRY is the primary objective.** The entire plan exists to eliminate the current 60% duplication problem. Option B reintroduces it at a different layer.
2. **Claude's UX gap is minor.** Claude lacks an agent picker regardless of file format — users will always prompt "act as Malory" whether the persona lives in `.agent.md` or a Claude-native file. The reading experience is identical.
3. **YAML frontmatter is genuinely harmless.** Claude reads it as text context. The `name:` and `description:` fields actually _help_ Claude understand the persona. Only `tools:` restrictions are silently ignored — and Claude has no equivalent mechanism anyway.
4. **The sync cost of Option B grows with the team.** At 15 agents, maintaining two aligned copies becomes a real operational burden — exactly the class of problem this architecture is designed to prevent.
5. **Option C is ruled out.** Copilot does not resolve file references at agent-load time (§6.4). Wrapper agents would appear empty. This removes the only alternative DRY strategy, making Option A the clear winner.

**Accepted tradeoff:** Claude users don't get an agent picker dropdown or `@mention` syntax. They prompt for personas manually. This is a UI convenience gap, not a knowledge gap. Every persona's full definition is available to Claude via `CLAUDE.md` references.

**Mitigation for Claude UX:** `CLAUDE.md` includes a clear agent roster with one-line descriptions and file paths, making it easy for users to direct Claude to the right persona:

```markdown
## Available Agent Personas

When you need a specialised perspective, reference the relevant agent file:

- **Appius** (Dev) — `.github/agents/appius.agent.md`
- **Malory** (Security) — `.github/agents/malory.agent.md`
- ...
```

### 6.3 SKILL.md Assessment

`SKILL.md` is a Copilot-only packaging convention for on-demand knowledge loading. In the restructured architecture, skills map naturally to **Layer 1 knowledge modules**:

- `build-and-test.md` ≈ a "build system" skill
- `hardware.md` ≈ a "sensor integration" skill
- `security-surface.md` ≈ a "security audit" skill

Whether to also create formal `SKILL.md` wrappers is a Copilot-specific UX decision, not an architectural one. The underlying knowledge is the same.

**Recommendation:** Defer `SKILL.md` creation. The knowledge modules serve the same purpose. Revisit if Copilot's skill discovery UX proves valuable enough to justify the thin wrapper files.

### 6.4 Finding: Copilot Agent File Reference Resolution

**Question:** If an `.agent.md` body contains `See [build-and-test.md](.github/knowledge/build-and-test.md)`, does Copilot eagerly include that file's content when the agent loads?

**Answer: No.** Copilot does not eagerly resolve Markdown links in `.agent.md` bodies. The agent load sequence is:

1. Parse YAML frontmatter (`name`, `description`, `tools`)
2. Load body content as the agent's system prompt (verbatim text)
3. Load workspace instructions (`copilot-instructions.md`) as additional context
4. That's it — no file traversal

Markdown links like `[build-and-test.md](.github/knowledge/build-and-test.md)` remain literal text in the prompt. The agent _can_ read those files at runtime using its `read` tool, but only on demand — not eagerly at load time.

**What this means for the architecture:**

| Concern                                  | Impact                                                                                                                                                                         | Severity                                           |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------- |
| **Option C viability**                   | Dead — Copilot wrapper `.agent.md` files cannot `#include` shared persona content at load time                                                                                 | Resolved                                           |
| **Layer 1/2 references in agent bodies** | References like "see build-and-test.md" are _prompting hints_, not file includes — the agent must actively decide to read those files                                          | Low — agents with `read` tool will follow the hint |
| **Knowledge module discovery**           | Agents don't auto-ingest Layer 1/2 content — they only get it if (a) the user mentions it, (b) the agent's `read` tool fetches it, or (c) `copilot-instructions.md` inlines it | Medium                                             |

**Design implication:** Layer 1/2 knowledge that agents need for _every_ interaction should be inlined in `copilot-instructions.md` (or the agent body), not just referenced. Knowledge that agents need _sometimes_ can safely live in external files as read-on-demand references.

This reinforces **Option A** — keeping persona content directly in `.agent.md` bodies rather than factoring it out into separate files that won't be auto-loaded.

**Contrast with SKILL.md:** Skills _do_ have progressive loading — relative file paths in `SKILL.md` body (e.g. `[script](./scripts/test.js)`) cause the agent to load resources when the skill activates. But this mechanism does not extend to `.agent.md` files.

**Revised Layer 1/2 strategy:** The knowledge modules in `.github/knowledge/` remain valuable for DRY authoring and human reference, but the tool entry points (`copilot-instructions.md` and `CLAUDE.md`) need to inline or summarise the most critical facts rather than just linking. This is a minor adjustment — the entry points become slightly larger (~150 lines instead of ~80) but still dramatically smaller than the current 417 + 3,040 lines.

### 6.5 Full Platform Feature Matrix

To make an informed architectural decision, we must enumerate _every_ feature each platform offers and assess how each option serves it. Features that only one platform supports represent native UX that can only be delivered via platform-specific definitions.

| #   | Feature                         | Copilot                                      | Claude Code                                 | Option A (single source)                                 | Option B (dual native)                                     |
| --- | ------------------------------- | -------------------------------------------- | ------------------------------------------- | -------------------------------------------------------- | ---------------------------------------------------------- |
| 1   | **Agent picker UI**             | `.agent.md` auto-populates dropdown          | No equivalent                               | ✅ Copilot native                                        | ✅ Copilot native                                          |
| 2   | **`@mention` invocation**       | `@Appius` in chat triggers agent             | No equivalent                               | ✅ Copilot native                                        | ✅ Copilot native                                          |
| 3   | **Structured YAML metadata**    | `name`, `description`, `tools` parsed        | Ignored (harmless noise)                    | ✅ / ⚠️ Claude sees YAML as text                         | ✅ / ✅ Claude gets clean prompt                           |
| 4   | **Per-agent tool restrictions** | `tools:` limits available MCP/built-in tools | No equivalent                               | ✅ / n/a                                                 | ✅ / n/a                                                   |
| 5   | **Root instructions**           | `copilot-instructions.md`                    | `CLAUDE.md`                                 | ✅ / ✅ separate files                                   | ✅ / ✅ separate files                                     |
| 6   | **Scoped instructions**         | `.instructions.md` + `applyTo` glob          | Subdirectory `CLAUDE.md` files              | ✅ / ✅ independent                                      | ✅ / ✅ independent                                        |
| 7   | **Hierarchical scoping**        | Flat (global + per-agent)                    | Nested subdirectory `CLAUDE.md` inheritance | ⚠️ Claude can't use nesting if persona is in `.agent.md` | ✅ Claude-native nesting                                   |
| 8   | **Knowledge-on-demand**         | `SKILL.md` progressive loading               | Agent reads files explicitly                | ✅ / ⚠️ Claude has no skill system                       | ✅ / ✅ each tool uses own mechanism                       |
| 9   | **Persona prompt format**       | Markdown body after YAML frontmatter         | Free-form Markdown (no YAML)                | ✅ / ⚠️ Claude sees YAML preamble                        | ✅ / ✅ Claude gets optimised prompt                       |
| 10  | **Slash commands**              | Custom slash commands in `.agent.md`         | `/` commands in `CLAUDE.md`                 | ✅ / ❌ Claude can't use Copilot slash commands          | ✅ / ✅ each defines its own                               |
| 11  | **Context window budget**       | Each agent loads in its own context          | Single context — all personas compete       | ✅ / ⚠️ Claude loads one agent at a time via reference   | ✅ / ✅ Claude file is purpose-built for its context model |
| 12  | **Agent-specific examples**     | Inline in `.agent.md` body                   | Inline in persona definition                | ✅ shared examples                                       | ✅ / ✅ examples tailored to each tool's capabilities      |
| 13  | **DRY compliance**              | n/a                                          | n/a                                         | ✅ zero duplication                                      | ⚠️ persona duplication (mitigated by drift detection)      |
| 14  | **New agent cost**              | n/a                                          | n/a                                         | 1 file                                                   | 2 files + sync                                             |

**Key insight from the matrix:** Features 3, 7, 9, 10, 11, and 12 represent real UX gains that Option A sacrifices for Claude users. The YAML noise (features 3/9) is minor, but hierarchical scoping (7), slash commands (10), and context-optimised persona prompts (11/12) are meaningful capability gaps.

### 6.6 Revised Recommendation: Option B with Drift Detection

The original analysis correctly identified sync overhead as Option B's primary risk. But the feature matrix (§6.5) reveals that Option A also has real costs — Claude users get a degraded experience across 6 features. With 5–15 agents planned, the question is: **can we afford the sync overhead of Option B, and can we make it safe?**

**Answer: yes — with automated drift detection.**

#### Why Option B wins

1. **Maximise feature coverage.** Each platform gets purpose-built agent definitions that exploit its native capabilities. Copilot agents use `tools:` restrictions and slash commands; Claude agents use clean prompts, hierarchical scoping, and context-optimised examples.
2. **The duplication is bounded.** Only persona definitions (name, description, methodology, coordination rules) are duplicated. Shared project knowledge (Layers 0–2) remains single-source in `.github/knowledge/`. The duplicated content is ~30–80 lines per agent — the role-specific core, not the 200+ lines of project facts currently repeated.
3. **Drift is detectable.** A script compares the semantic content of paired agent definitions weekly. Any drift is flagged in the planning review meeting. See §6.7.
4. **The sync cost is front-loaded.** Creating the initial Claude definitions is a one-time effort. Ongoing sync only triggers when persona methodology changes — not when project facts change (those live in shared knowledge modules).
5. **Option A's "harmless YAML" argument understates the cost.** Claude reading `tools: [run_in_terminal, grep_search]` doesn't break anything, but it wastes context tokens on instructions Claude can't act on. At scale (15 agents, each with 5–10 tool restrictions), this noise adds up.

#### What gets duplicated (bounded)

| Content                    | Duplicated?        | Location (Copilot)            | Location (Claude)                          |
| -------------------------- | ------------------ | ----------------------------- | ------------------------------------------ |
| Project tenets             | No                 | `.github/TENETS.md`           | `.github/TENETS.md` (same file)            |
| Build/test knowledge       | No                 | `.github/knowledge/`          | `.github/knowledge/` (same files)          |
| Role mixins                | No                 | `.github/knowledge/role-*.md` | `.github/knowledge/role-*.md` (same files) |
| Persona name + description | Yes (~2 lines)     | YAML `name:`, `description:`  | Inline in `.claude/agents/*.md`            |
| Persona methodology        | Yes (~30–60 lines) | `.agent.md` body              | `.claude/agents/*.md`                      |
| Tool restrictions          | Copilot-only       | YAML `tools:` field           | n/a                                        |
| Slash commands             | Platform-specific  | `.agent.md` body              | `.claude/agents/*.md`                      |
| Coordination rules         | Yes (~10–20 lines) | `.agent.md` body              | `.claude/agents/*.md`                      |

**Total duplication per agent:** ~40–80 lines of role-specific content. At 15 agents, ~600–1,200 lines duplicated — but drift-checked weekly and bounded to persona content only.

### 6.7 Drift Detection

Uncontrolled drift between Copilot and Claude agent definitions is the primary risk of Option B. The mitigation is a script that compares paired definitions and surfaces differences for human review.

**Script:** `scripts/check-agent-drift.sh`

The script:

1. Enumerates all `.github/agents/*.agent.md` files
2. Looks for a corresponding `.claude/agents/*.md` file
3. Extracts the persona-relevant sections (stripping YAML frontmatter and platform-specific directives)
4. Compares the normalised content and reports:
   - **Missing pairs** — agent exists in one platform but not the other
   - **Content drift** — persona methodology, coordination rules, or forbidden actions differ
   - **Acceptable divergence** — platform-specific features (tool restrictions, slash commands) are excluded from comparison
5. Outputs a Markdown summary suitable for the weekly planning review

**Integration:** Added to `scripts/florence-planning-review.sh` as a new section so drift surfaces alongside backlog health, plan coverage, and open questions in the weekly meeting.

**Make target:** `make check-agent-drift` runs the script standalone.

### 6.8 Adopted Patterns from External Stack Analysis

#### Per-Agent Adopted Patterns

**Ruth (Justice)** — scope modes are Ruth's primary tool:

- Three explicit modes: EXPANSION (blue-sky), HOLD (review existing scope), REDUCTION (cut ruthlessly). User selects mode at session start.
- Mandatory NOT-in-scope list on every scope decision — documents what was excluded and why.
- Mandatory output artefacts: scope decision record, tradeoff rationale, NOT-in-scope list.

**Grace (Architect)** — scope modes (secondary) + structured interaction:

- Scope modes available for architectural review (same three modes as Ruth, but Grace uses them to evaluate technical scope, not product scope).
- Interactive question protocol: one issue = one question. Present each finding with numbered options and a recommendation rather than dumping all findings at once.
- Mandatory output artefacts: architecture decision record, ASCII diagrams for system boundaries, failure registry.

**Malory (Pen Test)** — review discipline + externalised criteria:

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
| **Output format**                       | Extremely prescriptive: exact headings, tables, registries specified per skill | Open-ended — agents decide their own format                     | **Adopted for Malory, Florence, Ruth** — prescribe output format for review/coordination/judgment agents where consistency matters. Leave creative agents (Terry) and implementation agents (Appius, Grace, Euler) flexible. |
| **Read-only default**                   | Review skills only read and comment; write only when explicitly asked          | Agents read and write freely by default                         | **Adopted for Malory only** — audit-first discipline. Other agents (including Grace) remain read-write by default.                                                                                                           |
| **Persistent engineering context**      | Reusable coding standards block baked into every planning skill                | Standards in `copilot-instructions.md` but not in agent prompts | **Already solved** — maps directly to our Layer 1 `coding-standards.md`. No action needed.                                                                                                                                   |
| **Suppressions as first-class concept** | Explicit "do NOT flag these" lists per skill                                   | No equivalent — agents flag everything they find                | **Adopted for Malory and Appius** — suppressions live in the agent file (not the externalised checklist) so they stay tightly coupled to the persona's review methodology.                                                   |

---

## 7. Implementation Plan

### Phase 1: Tenets + Knowledge Extraction `M`

**Goal:** Eliminate duplication. Create the canonical knowledge layer.

1. Create `.github/TENETS.md` — project constitution (~30 lines)
2. Create `.github/knowledge/` directory with extracted modules:
   - `build-and-test.md` — make targets, dev servers, venv, test commands
   - `architecture.md` — tech stack, DB, data flow, deployment target
   - `coding-standards.md` — British English, formatting, commit conventions
   - `hardware.md` — radar specs, LIDAR specs, serial/UDP interfaces
   - `security-surface.md` — attack surface map (from Malory, deduplicated)
   - `security-checklist.md` — externalised review criteria with gate classification
3. Create `.github/knowledge/role-technical.md` and `role-editorial.md` mixins
4. Refactor `copilot-instructions.md` to reference Layer 0–2 instead of inlining

**Acceptance:** `copilot-instructions.md` shrinks from 417 lines to ~80 (references + tool-specific config only). No project fact appears in more than one file.

### Phase 2: Agent Condensation `M`

**Goal:** Slim agent files to role-specific content only.

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

**Acceptance:** Total agent file lines drop from ~3,040 to ~1,200. Each agent file is <200 lines. Agents with review responsibilities (Malory, Grace) reference externalised checklists rather than inlining criteria.

### Phase 3: Claude Code Entry Point + Native Agents `M`

**Goal:** Enable Claude Code with full native UX. Create platform-optimised agent definitions.

7. Create `CLAUDE.md` at repo root referencing `TENETS.md` + `knowledge/` modules
8. Create `.claude/agents/` directory with Claude-native persona definitions for each agent:
   - No YAML frontmatter — clean Markdown prompts
   - Reference shared knowledge modules (same `.github/knowledge/` files)
   - Include Claude-specific slash commands and context-optimised examples
   - Persona methodology and coordination rules mirrored from `.agent.md` (drift-checked)
9. Create `scripts/check-agent-drift.sh` — drift detection between paired agent definitions
10. Add `make check-agent-drift` target
11. Integrate drift check into `scripts/florence-planning-review.sh` for weekly review
12. Test Claude Code reads the knowledge modules and agent files correctly

**Acceptance:** Claude Code session has access to equivalent project knowledge as Copilot. Shared knowledge (Layers 0–2) is single-source. Persona definitions are platform-native in both tools. `make check-agent-drift` reports zero unreviewed drift.

### Phase 4: Agent Team Expansion (future cycle) `L`

**Goal:** Expand from 5 to 10–15 agents using the DRY architecture.

13. Scope and prioritise new agent candidates from §5.2
14. Create new agent files in both platforms (~50–100 lines each, referencing existing knowledge modules)
15. Validate new agents work in both Copilot and Claude Code
16. Run `make check-agent-drift` to confirm paired definitions are aligned
17. Add any new knowledge modules if new agents surface previously undocumented domain knowledge

**Acceptance:** Adding a new agent requires two new files (one per platform) + zero changes to shared knowledge (unless the project itself changed). Drift check passes.

---

## 8. File Tree (Target State)

```
.github/
├── TENETS.md                           # Layer 0: project constitution
├── copilot-instructions.md             # Layer 4: Copilot entry point (slim)
├── knowledge/                          # Layer 1 + 2: shared knowledge
│   ├── architecture.md                 #   tech stack, DB, data flow
│   ├── build-and-test.md               #   make targets, venv, test commands
│   ├── coding-standards.md             #   British English, formatting, commits
│   ├── hardware.md                     #   radar, LIDAR, serial, UDP
│   ├── security-surface.md             #   attack surface map
│   ├── security-checklist.md           #   review criteria + gate classification
│   ├── role-technical.md               #   mixin for technical agents
│   └── role-editorial.md              #   mixin for editorial agents
├── agents/                             # Layer 3: Copilot agent definitions
│   ├── euler.agent.md                    #   Research / Math (technical)
│   ├── grace.agent.md                   #   Architect (technical)
│   ├── appius.agent.md                  #   Dev (technical)
│   ├── malory.agent.md                  #   Pen Test (technical)
│   ├── florence.agent.md                #   PM (editorial)
│   ├── terry.agent.md                   #   Writer (editorial)
│   ├── ruth.agent.md                    #   Executive (both)
│   └── [future-agents].agent.md       #   further expansion planned
.claude/
├── agents/                             # Layer 3: Claude agent definitions
│   ├── euler.md                         #   Research / Math (technical) — Claude-native
│   ├── grace.md                        #   Architect (technical)
│   ├── appius.md                       #   Dev (technical)
│   ├── malory.md                       #   Pen Test (technical)
│   ├── florence.md                     #   PM (editorial)
│   ├── terry.md                        #   Writer (editorial)
│   ├── ruth.md                         #   Executive (both)
│   └── [future-agents].md             #   further expansion planned
CLAUDE.md                               # Layer 4: Claude entry point → refs .github/ + .claude/
scripts/
└── check-agent-drift.sh                # Drift detection between paired definitions
```

**Shared knowledge:** `.github/TENETS.md` and `.github/knowledge/` are the single source of truth for project facts — referenced by both Copilot and Claude agent definitions.
**Persona duplication:** Agent methodology, coordination rules, and forbidden actions are duplicated across `.github/agents/` and `.claude/agents/`. Drift is detected weekly by `scripts/check-agent-drift.sh`.

---

## 9. Estimated Impact

| Metric                                      | Current        | Target                   | Change             |
| ------------------------------------------- | -------------- | ------------------------ | ------------------ |
| Total lines (all agent/instruction files)   | 3,456          | ~2,000                   | -42%               |
| _Project fact_ duplication instances        | ~45            | 0                        | -100%              |
| _Persona_ duplication instances             | 0              | ~7–15 (bounded)          | +N (drift-checked) |
| Files to update when build changes          | 5              | 1                        | -80%               |
| Files to update when privacy policy changes | 6              | 1                        | -83%               |
| Cost to add a new agent                     | ~200–800 lines | ~100–160 lines (2 files) | -75%               |
| Tools supported                             | 1 (Copilot)    | 2 (Copilot + Claude)     | +100%              |
| Platform features used                      | ~60%           | ~95%                     | +58%               |
| Drift detection                             | None           | Weekly automated         | ∞                  |

---

## 10. Open Questions

- [ ] Does `.github/knowledge/` directory convention work with Copilot's file discovery, or do agents need explicit `#file` references?
- [x] ~~Should Claude Code agent personas be sections in `CLAUDE.md` or separate files in `.claude/`?~~ — Resolved: separate files in `.claude/agents/`. Platform-native definitions maximise feature coverage.
- [x] ~~Single source vs dual agent definitions?~~ — Resolved: **Option B (dual native) with drift detection.** Feature matrix (§6.5) showed Option A sacrifices 6 real UX features for Claude users. Persona duplication is bounded (~40–80 lines/agent) and drift-checked weekly. See §6.6.
- [x] ~~Does Copilot resolve `#file` references inside `.agent.md` at agent-load time?~~ — Investigated. **No, it does not eagerly resolve Markdown links.** See §6.4 for full analysis. This ruled out Option C and informed the decision to keep persona content directly in agent files rather than factoring it into shared includes.
- [x] ~~Which new agents to prioritise?~~ — Partially resolved: Executive and Researcher added to core roster (§5.1). Remaining candidates in §5.2 deferred to future cycle.
- [ ] Should `TENETS.md` be enforced via a CI check (e.g. grep for PII-related code patterns)?
- [x] ~~Review cadence — quarterly staleness check for knowledge modules?~~ — Resolved: weekly drift check via `scripts/check-agent-drift.sh`, integrated into `florence-planning-review.sh`. Knowledge module staleness reviewed alongside agent drift.
