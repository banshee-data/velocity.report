# Decision review: <topic>

<One paragraph explaining the scope and what this packet enables the person to decide. Replace
all placeholders before publishing; repeat per-question sections as needed. Remove example rows
and decision entries that have not happened, leaving their section headings for the next phase.>

- **Packet:** <stable ID>
- **Revision:** <integer; increment on material evidence, agenda, or decision changes>
- **Phase:** <prepare, voice, or integrate>
- **Branch:** <branch>
- **Related:** <owning plan/review and checkpoint PR links>

## Voice brief

<Brief overview grouped by project, schema, maths, and research; counts, urgent blockers, and
what can be decided today. This section and the handoff must stand alone for the voice agent.>

- Time budget: <minutes>; selected question IDs in dependency order: <IDs>.
- Carry forward: <overflow, unavailable evidence, and deferred triggers>.

### Q-01: <spoken title>

<Single decision, why now, essential evidence, options A/B/C, recommendation and confidence,
one question to ask, estimated discussion time, prerequisite IDs, and evidence-card revision.
Keep this short enough to read aloud in about a minute.>

## Question index

| ID   | Category                        | Source ID and link             | State    | Depends on    | Current decision entry |
| ---- | ------------------------------- | ------------------------------ | -------- | ------------- | ---------------------- |
| Q-01 | <project/schema/maths/research> | <file + heading + original ID> | prepared | <IDs or none> | <entry link or none>   |

## Candidate inventory

| Source question      | Disposition                                                                        | Reason or resolution | Agenda ID or reopening trigger |
| -------------------- | ---------------------------------------------------------------------------------- | -------------------- | ------------------------------ |
| <link and source ID> | <discuss/already decided/implementation pending/deferred/duplicate/needs evidence> | <evidence>           | <ID or trigger>                |

## Evidence and options

### Evidence Q-01: <title>

- Card revision: <integer>.
- Current behaviour and decision boundary: <facts; distinguish decision from implementation>.
- Sources: <paths + headings/symbols, primary citations, related PR threads>.
- Evidence quality: <observed, derived, assumed, or missing; uncertainty and access limits>.
- Technical check: <maths definitions/units/derivation, schema compatibility, or research limits>.
- Prerequisites and cycles: <IDs and what must be settled first>.

| Option | Result and tradeoff              | Effort and reversibility | Evidence and uncertainty |
| ------ | -------------------------------- | ------------------------ | ------------------------ |
| A      | <current behaviour or candidate> | <assessment>             | <citations>              |
| B      | <candidate>                      | <assessment>             | <citations>              |
| C      | <candidate or defer, if useful>  | <assessment>             | <citations>              |

- Recommendation: <option and reason, confidence, what would change it>.
- Decision requested: <one precise question>.

### Impact Q-01

<Trace direct and transitive dependencies for each option. After the choice, record the selected
option's closure and why other option rows no longer apply. Every target needs a disposition.>

| Option  | Target path and symbol         | Trace evidence            | Required change                  | Prerequisite | Validation                      | Integration disposition                                                 |
| ------- | ------------------------------ | ------------------------- | -------------------------------- | ------------ | ------------------------------- | ----------------------------------------------------------------------- |
| <A/B/C> | <canonical source or consumer> | <actual reference/caller> | <action or reason for no change> | <ID or none> | <check or acceptance condition> | <pending/changed + commit/no change + reason/deferred + work-item link> |

## Decision log

<Append edited decision entries only after explicit user confirmation. Leave empty during
preparation. Preserve earlier entries and link superseding entries rather than rewriting them.>

### E-01: Q-01

- State: <ready for integration, needs voice review, or integrated>.
- Disposition: <adopt, reject, or defer>.
- Choice and constraints: <precise normalised answer, including scope and exceptions>.
- Rationale: <the person's reason and the evidence they accepted>.
- Confirmation: <edited summary of the explicit instruction; ambiguity resolved or still open>.
- Provenance: <session/turn reference when available, modality, UTC ISO 8601 timestamp>.
- Reviewed card: <packet ID/revision and evidence-card revision>.
- Supersedes: <prior entry link or none>.
- Follow-ups: <IDs or none; reopening trigger for a deferral>.

## Follow-ups

| ID   | Question or decision entry | Action and reason                                        | Owner                             | Blocks        | Acceptance condition                      | Disposition and durable destination                             |
| ---- | -------------------------- | -------------------------------------------------------- | --------------------------------- | ------------- | ----------------------------------------- | --------------------------------------------------------------- |
| F-01 | <ID>                       | <new question, evidence request, or implementation task> | <named role/person or unassigned> | <IDs or none> | <observable completion or reopening rule> | <pending/resolved + evidence/returned to voice/deferred + link> |

## Integration review

<Leave empty until high-capability review. Record source drift, conflicts, calculations checked,
and the treatment of each decision and follow-up. Link applied commits and remaining work.>

| Entry       | Technical verdict                                              | Dependency closure             | Checks and limitations              | Outcome                                                                                              | Return question                     |
| ----------- | -------------------------------------------------------------- | ------------------------------ | ----------------------------------- | ---------------------------------------------------------------------------------------------------- | ----------------------------------- |
| <E-ID/F-ID> | <supported/mechanical clarification/material conflict/blocked> | <impact rows and dispositions> | <commands and results or why unrun> | <implemented/rejected/deferred/approved plan with implementation pending/needs voice review/blocked> | <one question and options, or none> |

- PR disposition: <preparation checkpoint/ready for review/needs voice review/blocked>.
- Merge event: <pending, or verified URL and commit after an authorised merge>.

## Provenance and coverage

- Preparation snapshot: <commit SHA, branch, remote freshness, and dirty-worktree limits>.
- Source inventory: <relevant paths and blob IDs; available PR URLs and head SHAs>.
- Search coverage: <categories, queries, scope, excluded areas, and unavailable sources>.
- Phase runs: <phase, packet revision, requested tier, actual model or unknown, session reference
  when available, UTC ISO 8601 timestamp>.
- Validation: <link/ID/state/confirmation/closure checks, repository checks, and limitations>.

## Resume handoff

- Next phase and requested tier: <prepare/high, voice/light, or integrate/high>.
- Packet and revision: <path, ID, revision>.
- Branch and PR: <branch and URL; saved/committed/pushed status stated accurately>.
- Read first: <bounded sections; selected IDs and evidence-card revisions>.
- Resume at: <next question ID; confirmed IDs; returned IDs; blockers and overflow>.
- Invocation: <next command with this packet path>.
- Handoff prompt: <task, chosen constraints, unresolved work, and stop condition; usable without
  previous chat history>.
