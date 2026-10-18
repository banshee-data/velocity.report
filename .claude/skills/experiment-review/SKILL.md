---
name: experiment-review
description: Review the answered experiments and their reports against one product goal, rank the next week's experiments, say which run unattended and which need a person at the annotation window, list the labelling and annotation priorities, the backlog items superseded and the engineering each step waits on, and write every recommendation that calls for more work as a proposed spec with its rule. Reads the papers only where a briefing's open question bears on the goal.
argument-hint: "[goal, e.g. following distance] [--days 7] [--area l8fol ...]"
---

# Skill: experiment-review

Read what the experiments established, decide what to run next for one goal, and leave the
queue in a state `/run-experiments` can pick up overnight and a person can pick up at the
annotation window in the morning. The method is the one the
[October 18 review](../../../docs/lidar/operations/experiment-review-2026-10-18.md) used for the
following-distance metric: the chain from a capture to the product's figure, each link's measured
state, and the cheapest experiment or the one labelled window that moves the weakest link. It is
the weekly queue review the
[experiment queue plan](../../../docs/plans/platform-experiment-queue-plan.md#8-the-weekly-cadence)
asks for, and it writes specs, backlog edits and a review; it approves nothing.

## Usage

```
/experiment-review                                 # the current goal, the next 7 days
/experiment-review following distance --days 7     # name the goal and the horizon
/experiment-review --area l5id --area geom          # only these areas' lines
```

Run it weekly, after the Monday research briefing and before an overnight `/run-experiments`,
or whenever a run's reports land and the queue has gone quiet.

## 1. Read the state, not the memory of it

Everything below is read at `origin/main`'s head, and the commit goes in the review's
provenance.

```bash
git fetch origin main && git log --oneline -40 origin/main
go run ./cmd/tools/experiments next          # the open queue and what each waits on
go run ./cmd/tools/experiments status        # approved experiments' units and time left
gh pr list --state open                      # or the GitHub tools; what is in flight
```

Then, in this order:

1. `data/experiments/DEPENDENCIES.md`: every answered experiment's report, every open one's
   dependencies and estimates, the longest open chain.
2. The **Recommendations**, **Summary** and **Limitations** of every report the answered specs
   name, newest first. A recommendation is a question with no identifier until a spec carries
   it; list each one and whether a spec already does.
3. The goal's plan and sprint plan: the ledger of gates, what each gate's current state and
   remaining evidence are, and the backlog sections the goal lives in.
4. The briefings under `docs/lidar/operations/brief/` for their **Open questions** and
   **Decisions**, and `docs/DECISIONS.md` for what was decided and what it was deferred on.
5. The memory notes of earlier overnight runs, where the session has them: which held-out
   segments were read for which measure, and the traps met.
6. The labelling tooling's state: `docs/lidar/operations/point-annotation-tool.md`,
   `label-sets.md`, and the plans' status lines, so a labelling task is sized against the tools
   that exist today, not the ones planned.

## 2. Name the goal and draw its chain

One product measure, in the user's words, and the chain from a capture to it: for following,
tracks, bodies, class, pairs, the gate, the field value. For each link write its measured state
with the figure and the report that measured it, and what blocks the link from being adopted or
read. The weakest link that a measurement can move is where the machine's week goes; the link
that only a person can move is where the operator's week goes. Do not skip a link because it is
unglamorous: the class gate was the one nobody had measured.

## 3. Rank the candidates

Every recommendation, open spec and briefing question that bears on the goal is a candidate.
Rank them as `/run-experiments` ranks, with one more test:

1. **The size of the defect, as measured**, in what a user sees. Measured beats suspected.
2. **What an answer unblocks**: a default, a gate, a decision, a line of work ended.
3. **Runnable without a person.** Every `requires.capabilities` on a host the run has, and no
   `manual:label`, `manual:annotate`, `manual:review`, `manual:capture`, `manual:decide` or Pi
   step before the answer. A `manual:implement` step an agent can close overnight counts as
   runnable; say how long it is.
4. **Cost**, machine and person, from the spec's estimate and October's actuals (agents close
   implementation steps in 15 to 35 minutes against 4 to 16 hour person estimates; machine steps
   have run from a fiftieth of their estimate to seven times it).
5. **It serves the goal.** A refinement off the goal's chain ranks below a measurement on it,
   however cheap. Say so, and leave it in the queue rather than retiring it.

Record the ranking as a table: rank, spec, question, runs without a person, machine time, agent
implementation time, what it unblocks. Then group the runnable ones into overnight runs (zero-code
measurements first, then the ones with implementation steps), and say which half runs in the
cloud with kirk0 alone. Keep a **held-out ledger**: which segments each measure has read, so no
spec reads a segment as held out that an earlier one tuned on for the same measure.

## 4. The person's week

The operator's hours are the constraint every gate names. List the labelling and annotation
tasks in order, each with its person time, the tooling that exists for it today (and the
increment that would halve it), and the gates it serves. Look for the window that serves several
gates at once: three plans asking for three captures are usually one lot-drawn segment reviewed
whole. State the total hours over the horizon and whether it is plausible.

Two rules hold: a held-out window is chosen by lot or by traffic, never for a failure, and a
window read once for a measure is tuning for it afterwards.

## 5. The supersession audit

For each backlog item on the goal's path, read it against the reports. An item is superseded
when a report answered it, a spec now carries it, or two items ask for one thing. Apply the
backlog's governance in the same change: split, consolidate, complete, point at the spec; never
delete, never strike through. Record each change in the review's table with its reason. Open
specs are reordered, not retired, unless a report says why they will not run.

## 6. The engineering list

Every `manual:implement` step the ranked specs carry, and every tool increment the labelling
tasks wait on, as a table: the work, its size (`S`, `M`, `L`, hardware), which spec reads it,
and why it matters for the goal. Agents close `S` and `M` items under `/run-experiments`; `L`
items and hardware go to the owner as decisions.

## 7. The papers, bounded

Read the briefings' verdicts, not the papers. For each source the briefings judged relevant to
the goal, one bullet: the measure or rule it gives, and which spec carries it. Name at most one
document worth fetching this week, with the reason, and say plainly that nothing else should be
read. The goal is the goal; a new paper is next Monday's briefing.

## 8. Write

1. **Specs.** Every recommendation that calls for more work becomes a `proposed` spec under
   `data/experiments/<area>/`, `go run ./cmd/tools/experiments new <area>` for the slug, a new
   area with its `README.md` header where the goal has none, with: `asks` naming the report
   section, plan or decision that posed it; arms as parameter sets under `config/experiments/`
   with their README rows; the corpus from the registry; steps with estimates in hours and
   minutes; and **the rule written now**, before any replay, with the target, the
   non-inferiority bounds, the held-out set and how much must survive it, or the statement that
   it is a measurement and what each reading decides. A spec needs an implementation step the
   worker cannot run today, say so in the step's note. `go run ./cmd/tools/experiments validate`
   must pass and `make experiments-graph` regenerates the graph.
2. **The review**, `docs/lidar/operations/experiment-review-YYYY-MM-DD.md`, named by the UTC
   date, with the metadata bullets `Status`, `Goal`, `Inputs`, `Method` and `Related`, and the
   sections `Answer`, `What the experiments established`, `Where the metric stands`, `The next
   seven days`, `Labelling and annotation priorities`, `Superseded and revised work`,
   `Engineering work needed`, `What the papers say to do, and what to leave`, `Decisions for
   the owner`, `What this review does not establish` and `Provenance`. The specs' `asks` may
   cite its section anchors. British English, prose wrapped at 100 columns, no em dashes, no
   verdict words about road users, and no raw output: every number is a report's.
3. **Backlog**, `docs/DECISIONS.md` only if a decision was actually made (it was not: the
   review proposes), the plan's open-questions row where a spec now carries it, one sentence in
   `docs/lidar/README.md` linking the review, and a devlog bullet under today's UTC date,
   prefixed `{branch-name}` until it merges.

## 9. Ship

1. Branch `claude/experiment-review-YYYY-MM-DD` from `origin/main`.
2. `go run ./cmd/tools/experiments validate`, `make experiments-graph`,
   `go test ./internal/experiments`, `make check-no-results`, `make format-docs` on the changed
   files and `python3 scripts/check-prose-line-width.py --report` on them; fix what is yours.
3. Commit as `[ai][docs]` with a message that states the week's answer, not the activity, and
   push with `git push -u origin <branch>`.
4. Open the PR from `.github/PULL_REQUEST_TEMPLATE.md` when the operator asked for one or the
   routine owns the run; otherwise leave the branch and say so.
5. Report in chat: the one-paragraph answer, the ranked table, the operator's list with its
   hours, the decisions only the owner can make, and the branch.

## Guardrails

- The review approves nothing: a spec stays `proposed` until the owner sets it `approved`.
- Never loosen a rule after seeing a result, and never write a rule after a replay.
- A window chosen for a failure is tuning, never held out; a segment read for a measure is no
  longer held out for it.
- Nothing from a `results` directory, no raw rows, no logs: `make check-no-results` before every
  commit.
- Keep the goal. A paper, a refinement or a tool that does not move the chain goes in the queue
  with its rank, not in this week's runs.
