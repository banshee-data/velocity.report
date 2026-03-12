# Agent Knowledge Architecture: Dual-Tool DRY Design

**Status:** Draft
**Created:** 2026-03-12
**Scope:** Restructure agent customisation for simultaneous Copilot + Claude Code use; eliminate knowledge duplication; prepare for team expansion to 10–15 agents

---

## 1. Problem Statement

velocity.report uses five named AI agents (Hadaly, Ictinus, Jess, Malory, Thompson) defined as Copilot `.agent.md` files. The system works, but suffers from three structural problems:

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
│  Layer 3: AGENT DEFINITIONS (role-specific only)    │
│  .github/agents/*.agent.md  (Copilot)               │
│  .claude/agents/*.md        (Claude Code, if added) │
│  ← Persona + responsibilities + Layer 1/2 refs      │
│  ← NO project facts restated here                   │
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

| Role Class    | Agents                                       | Knowledge Included                                                       |
| ------------- | -------------------------------------------- | ------------------------------------------------------------------------ |
| **Technical** | Hadaly, Ictinus, Malory + future tech agents | Build system, venv, test commands, DB details, hardware specs, packaging |
| **Editorial** | Thompson, Jess + future non-tech agents      | Brand voice, style guide, audience awareness, documentation standards    |

Some agents may include both mixins (e.g. a future "DevRel" agent who writes docs but also runs code).

**Layer 3 (agent files)** — slim persona definitions. Each agent file contains ONLY:

- YAML frontmatter (name, description)
- Role and responsibilities
- Role-specific methodology (Malory's red-team playbook, Jess's sequencing rules, etc.)
- References to Layer 1/2 knowledge: `see .github/knowledge/build-and-test.md`
- Inter-agent coordination notes
- Forbidden actions

**Layer 4 (tool entry points)** — thin shim files:

- `copilot-instructions.md` — references `TENETS.md` + `knowledge/` modules + Copilot-specific config
- `CLAUDE.md` — references the same canonical sources + Claude-specific config

### 4.3 DRY Enforcement Rules

1. **No project fact may appear in more than one file.** If two agents need the same fact, it belongs in Layer 1.
2. **Agent files reference, never restate.** Use `See [build-and-test.md](.github/knowledge/build-and-test.md) for make targets and venv setup.`
3. **Tenets are inherited, never copied.** Every agent gets Layer 0 automatically through the tool entry point. Agent files must not restate privacy/ethics principles.
4. **Role mixins are opt-in by class.** Technical agents reference `role-technical.md`. Editorial agents reference `role-editorial.md`. Cross-functional agents reference both.

---

## 5. Agent Role Taxonomy

### 5.1 Current Agents (5)

| Agent                   | Class     | Unique Domain                                                      |
| ----------------------- | --------- | ------------------------------------------------------------------ |
| **Hadaly** (Dev)        | Technical | Implementation methodology, code review, test strategy             |
| **Ictinus** (Architect) | Technical | Feature ideation, capability mapping, design docs                  |
| **Malory** (Pen Test)   | Technical | Red-team playbook, vulnerability patterns, severity classification |
| **Jess** (PM)           | Editorial | Scope definition, sequencing, risk identification, coordination    |
| **Thompson** (Writer)   | Editorial | Brand voice, copy editing, marketing, content quality              |

### 5.2 Planned Expansion (5–10 new agents, future cycle)

These are **candidates only** — to be scoped and prioritised in a future planning cycle. Listed here to validate the architecture scales.

| Candidate              | Class     | Proposed Domain                                        | Rationale                               |
| ---------------------- | --------- | ------------------------------------------------------ | --------------------------------------- |
| **QA / Test Lead**     | Technical | Test strategy, coverage analysis, regression detection | Currently spread across Hadaly          |
| **Data Analyst**       | Technical | Statistical analysis, data quality, metric validation  | Complement Ictinus on data layer        |
| **DevOps / Release**   | Technical | CI/CD, packaging, deployment, release management       | Currently ad-hoc                        |
| **UX / Accessibility** | Editorial | UI review, accessibility audits, user journey mapping  | Gap in current team                     |
| **Community Manager**  | Editorial | Issue triage, contributor onboarding, community comms  | Complement Thompson                     |
| **Compliance / Legal** | Editorial | Licence review, data governance, regulatory awareness  | Strengthen privacy posture              |
| **Hardware / Sensor**  | Technical | Sensor calibration, firmware, signal processing        | Deep domain currently in Hadaly/Ictinus |
| **Performance**        | Technical | Profiling, benchmarks, resource constraints (RPi)      | Currently implicit                      |
| **Docs / Tutorial**    | Editorial | Setup guides, tutorials, API docs, examples            | Complement Thompson                     |
| **DevRel**             | Both      | Blog posts, demos, conference talks, ecosystem         | Cross-functional                        |

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

**Both classes need (via Layer 0):**

- Project tenets (privacy, data ethics, integrity)
- High-level project purpose and positioning

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

### 6.2 Agent File Portability

Copilot's `.agent.md` files are Copilot-only (YAML frontmatter + agent picker UI). Claude Code does not recognise them. However, the restructured architecture makes this irrelevant:

- **Layer 0–2 files** are plain Markdown — both tools can read them
- **Layer 3 agent files** remain Copilot-specific (that's fine — they're slim persona wrappers)
- **`CLAUDE.md`** references the same Layer 0–2 files and describes agent personas inline or via prompt conventions

The shared knowledge lives in tool-agnostic Markdown. Only the thin entry points are tool-specific.

### 6.3 SKILL.md Assessment

`SKILL.md` is a Copilot-only packaging convention for on-demand knowledge loading. In the restructured architecture, skills map naturally to **Layer 1 knowledge modules**:

- `build-and-test.md` ≈ a "build system" skill
- `hardware.md` ≈ a "sensor integration" skill
- `security-surface.md` ≈ a "security audit" skill

Whether to also create formal `SKILL.md` wrappers is a Copilot-specific UX decision, not an architectural one. The underlying knowledge is the same.

**Recommendation:** Defer `SKILL.md` creation. The knowledge modules serve the same purpose. Revisit if Copilot's skill discovery UX proves valuable enough to justify the thin wrapper files.

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
3. Create `.github/knowledge/role-technical.md` and `role-editorial.md` mixins
4. Refactor `copilot-instructions.md` to reference Layer 0–2 instead of inlining

**Acceptance:** `copilot-instructions.md` shrinks from 417 lines to ~80 (references + tool-specific config only). No project fact appears in more than one file.

### Phase 2: Agent Condensation `M`

**Goal:** Slim agent files to role-specific content only.

5. Refactor each `.agent.md` file:
   - Remove all duplicated project facts
   - Add references to relevant Layer 1/2 modules
   - Keep only: persona, methodology, coordination notes, forbidden actions
6. Validate each agent still functions correctly in Copilot

**Acceptance:** Total agent file lines drop from ~3,040 to ~1,200. Each agent file is <200 lines.

### Phase 3: Claude Code Entry Point `S`

**Goal:** Enable Claude Code with zero knowledge duplication.

7. Create `CLAUDE.md` at repo root referencing `TENETS.md` + `knowledge/` modules
8. Document agent personas in `CLAUDE.md` as available roles (no `.agent.md` equivalent needed)
9. Test Claude Code reads the knowledge modules correctly

**Acceptance:** Claude Code session has access to equivalent project knowledge as Copilot. No facts duplicated between `CLAUDE.md` and `copilot-instructions.md`.

### Phase 4: Agent Team Expansion (future cycle) `L`

**Goal:** Expand from 5 to 10–15 agents using the DRY architecture.

10. Scope and prioritise new agent candidates from §5.2
11. Create new agent files (~50–100 lines each, referencing existing knowledge modules)
12. Validate new agents work in both Copilot and Claude Code
13. Add any new knowledge modules if new agents surface previously undocumented domain knowledge

**Acceptance:** Adding a new agent requires only one new file + zero changes to shared knowledge (unless the project itself changed).

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
│   ├── role-technical.md               #   mixin for technical agents
│   └── role-editorial.md              #   mixin for editorial agents
├── agents/                             # Layer 3: agent definitions
│   ├── hadaly.agent.md                 #   Dev (technical)
│   ├── ictinus.agent.md                #   Architect (technical)
│   ├── jess.agent.md                   #   PM (editorial)
│   ├── malory.agent.md                 #   Pen Test (technical)
│   ├── thompson.agent.md              #   Writer (editorial)
│   └── [future-agents].agent.md       #   5–10 more planned
CLAUDE.md                               # Layer 4: Claude Code entry point (slim)
```

---

## 9. Estimated Impact

| Metric                                      | Current        | Target               | Change |
| ------------------------------------------- | -------------- | -------------------- | ------ |
| Total lines (6 files)                       | 3,456          | ~1,600               | -54%   |
| Duplication instances                       | ~45            | 0                    | -100%  |
| Files to update when build changes          | 5              | 1                    | -80%   |
| Files to update when privacy policy changes | 6              | 1                    | -83%   |
| Cost to add a new agent                     | ~200–800 lines | ~50–100 lines        | -75%   |
| Tools supported                             | 1 (Copilot)    | 2 (Copilot + Claude) | +100%  |

---

## 10. Open Questions

- [ ] Does `.github/knowledge/` directory convention work with Copilot's file discovery, or do agents need explicit `#file` references?
- [ ] Should Claude Code agent personas be sections in `CLAUDE.md` or separate files in `.claude/`?
- [ ] Which 5–10 new agents to prioritise? (Deferred to future planning cycle with Jess)
- [ ] Should `TENETS.md` be enforced via a CI check (e.g. grep for PII-related code patterns)?
- [ ] Review cadence — quarterly staleness check for knowledge modules?
