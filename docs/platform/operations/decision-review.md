# Decision review by voice

The decision-review workflow prepares open repository questions for a lightweight voice
discussion, records the person's choices, and evaluates their downstream changes before a PR
is ready to merge. A versioned packet keeps the report, decisions, and follow-ups together.

## Run the workflow

Use these entry points in a client that discovers `.claude/skills/`:

```text
/decision-review following distance --minutes 20
/decision-review-voice docs/platform/operations/decision-review-following-distance.md
/decision-review-integrate docs/platform/operations/decision-review-following-distance.md
```

The first command builds context with a high-capability model. Without a scope, it surveys
project questions, undecided schema changes, maths decisions, and research review questions,
then chooses an agenda that fits the time budget. The report retains the rest of the inventory,
including resolved questions and deferrals, so they are not repeatedly discussed as new work.

The second command uses a lightweight model to talk through the prepared options, one question
at a time. Enable voice through the client's controls when available. A clear user choice is
saved as **ready for integration** with its rationale, conditions, confirmation, and follow-ups.
Tentative answers stay open. The voice phase edits the packet; downstream changes happen after
technical review.

The third command uses a high-capability model to check the answers, changed evidence, and new
follow-ups. It applies supported choices through the dependency graph and validates the result.
Material conflicts return as specific questions for another voice discussion. Independent
decisions can progress while coupled decisions wait for their prerequisites.

The [canonical procedure](../../../.claude/skills/decision-review/SKILL.md) owns the workflow.
The [voice entry point](../../../.claude/skills/decision-review-voice/SKILL.md) and
[integration entry point](../../../.claude/skills/decision-review-integrate/SKILL.md) are thin
phase wrappers. Claude Code requests `opus`, `haiku`, and `opus`; other clients select equivalent
capability tiers and read these files directly. These selectors do not provision models or
voice access. Record the actual model only when the client exposes it.

## What the packet contains

Each topic has one packet under this operations directory, created from the
[template](../../../.claude/skills/decision-review/references/packet-template.md). It contains:

- A concise voice brief with an agenda in prerequisite order, options, and recommendations.
- A source inventory distinguishing unresolved choices, completed decisions, implementation
  pending, deferred work, duplicates, and questions needing evidence.
- Detailed evidence cards with maths checks, schema compatibility, and research limitations.
- An impact matrix tracing each option to canonical documents, implementation, and consumers.
- An append-only decision log, follow-up owners and acceptance conditions, and integration review.
- A resume handoff with the branch, PR, packet revision, model role, and next question.

Packet-local question IDs remain stable. A source question retains its file and heading as well
as its original ID, since different briefings reuse identifiers such as `Q1`. Preparation checks
for later resolutions in the same document, the decisions register, current code, and open PRs.

## State and review boundaries

| State                 | Meaning                                                         | Next action                                          |
| --------------------- | --------------------------------------------------------------- | ---------------------------------------------------- |
| open                  | Candidate question has not been analysed                        | Prepare with high capability                         |
| prepared              | Options and evidence are ready for discussion                   | Discuss with voice or text                           |
| ready for integration | The person explicitly confirmed a choice                        | Review with high capability                          |
| needs voice review    | Integration found a material conflict or new tradeoff           | Discuss the focused return question                  |
| integrated            | Review and all impact dispositions are recorded with validation | Retain any linked implementation work and follow-ups |

An integrated deferral remains a deferral, with a reopening trigger. An integrated plan decision
may still have implementation pending, which is recorded with a linked work item. The state
does not claim that a feature shipped or a PR merged. Missing tools or evidence leave a recorded
blocker rather than turning a confirmed choice into an invented technical approval.

The integration model may settle mechanical details within the person's constraints. It returns
material changes of intent to voice. A reversal appends a superseding decision and invalidates
dependent recommendations so they are checked again.

## PR checkpoints and resuming

Preparation opens a draft PR containing the report. Its description identifies it as a
preparation checkpoint, with no user decisions integrated. Voice updates the packet on that
branch; integration updates the same PR with decisions, changed files, checks, and follow-ups.
The final disposition is ready for review, needs voice review, or blocked.

A report-only checkpoint may be reviewed and merged for the next session through normal
repository gates and user authorisation. The next run then creates a new branch and links a
successor PR to the packet's earlier checkpoint. Its questions keep their current states.
Ready for integration never authorises an automatic merge.

Resume using the packet path and its handoff, even in a fresh chat. Read the persisted entries
before asking another question, save each confirmed choice, and carry unanswered items forward.
The packet revision and source blob IDs allow the next model to detect changes between sessions.
It can continue independent current cards while stale cards wait for refreshed analysis.

If the voice client cannot write files, it returns a copyable decision handoff labelled
**unsaved**. The receiving model checks the packet revision and saves those confirmed entries
before integrating. Full audio, transcripts, and runtime logs stay outside Git; the packet holds
edited summaries and decision provenance.

## Acceptance walkthroughs

Use these cases when evaluating the skill. These are procedure checks, not claimed executions
of a voice client or model router.

| Case                                                     | Expected behaviour                                                                                      |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| No scope is given                                        | Inventory all four categories; record missing access; select a bounded agenda and retain overflow       |
| Briefing still lists Q1 but later records its decision   | Link the resolution and exclude Q1 from the agenda unless explicitly reopened                           |
| Two files each contain Q1                                | Preserve source-qualified IDs and assign distinct packet IDs, unless they are the same decision         |
| Schema change is decided but migration is unfinished     | Classify implementation pending; do not ask the person to decide it again                               |
| User clearly chooses B                                   | Read back conditions and immediately save ready for integration without another vote                    |
| User says a tentative yes, or a unit is ambiguous        | Keep the item unresolved and clarify the material ambiguity                                             |
| User reverses a prior choice                             | Append a superseding entry and flag dependent cards for review                                          |
| Voice introduces an unsupported equation or new option   | Record the choice or question accurately and require high-capability evaluation                         |
| Source or PR head changes during the handoff             | Refresh affected cards and dependent choices; preserve confirmed intent                                 |
| A migration affects a store, API, and client             | Account for the whole trace, generation, compatibility, and tests before completing integration         |
| One of two independent decisions conflicts with evidence | Integrate the supported closure and return the conflicting ID with a focused voice question             |
| A prerequisite is unresolved                             | Keep the coupled changes pending; do not claim a complete integration                                   |
| Voice stops, cannot write, or lacks model controls       | Save a resume point or return an unsaved handoff; report actual capability accurately                   |
| Checkpoint PR was already merged                         | Resume from the tracked packet and create a linked successor PR                                         |
| A follow-up is deferred                                  | Preserve its owner, acceptance condition, durable work item, and reopening trigger in the packet and PR |

Validate links, IDs, confirmations, state transitions, dependency closure, and resume information.
Run the documentation checks and repository gates required by
[AGENTS.md](../../../AGENTS.md) before publishing; implementation changes also need their
affected tests and builds. A skill definition supplies a procedure and model requests, so a
real voice session and cross-model handoff still require a compatible host.
