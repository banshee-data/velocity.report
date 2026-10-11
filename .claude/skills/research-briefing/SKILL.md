---
name: research-briefing
description: Weekly review of roadside LiDAR and safety literature. Discovers the week's sources from organisation and journal watchlists and topic queries, evaluates each against the repository's benchmark taxonomy, metrics registry, plans and wording guard, and commits a dated briefing with a verdict per source. Runs from the Monday routine or by hand.
argument-hint: "[--since YYYY-MM-DD] [path/to/supplied-briefing.md ...]"
---

# Skill: research-briefing

Discover what was published or newly surfaced in the week on roadside LiDAR perception and on the
safety and behaviour literature the LiDAR analytics cite, evaluate each source against what the
repository already holds, and commit one dated briefing on its own branch with a PR. The method is
the one that found and judged the October 2026 sources: the Waymo sober-baseline and fatal-rate
papers, Valgo's hosted crash baselines, the AAA Foundation attitudes and aggressive-driving reports,
and FMCSA's handheld headway programme.

## Usage

```
/research-briefing                              # the last 7 days, ending today
/research-briefing --since 2026-10-03           # a longer window
/research-briefing path/to/briefing.md          # also assess a supplied third-party briefing
```

The Monday routine "Weekly research briefing" invokes this skill in a fresh session with no
arguments.

## Procedure

### 1. Read the house rules first

`AGENTS.md`, `TENETS.md`, `.github/STYLE.md` and
`docs/platform/operations/documentation-standards.md`. The output is one briefing and, where its
own verdict says "adopted", a definitions paragraph, an evaluation-source row, a reference or an
open question in the plan it affects. Nothing larger changes: anything larger is a proposal inside
the briefing.

### 2. Window

Primary work published or first listed in the last 7 days (or since `--since`), plus older work
newly surfaced this week: an agency project page updated, a preprint accepted, a dataset or API
newly announced. Say which of the two a source is. If nothing new was published, say so in one
sentence and still record what surfaced.

### 3. Scope

Two bodies of literature the project depends on:

1. **Roadside and infrastructure LiDAR perception:** detection, clustering, tracking, extent and
   heading estimation, occlusion and fragmentation, multi-object identity, background modelling,
   calibration and ground-truth methods, and trajectory datasets (highD, inD, rounD, openDD, Waymo
   Open Motion, and any new one).
2. **The safety and behaviour literature the analytics cite:** headway, following distance,
   clearance and net time gap, passage headway, post-encroachment time, time to collision, stop
   compliance, speed percentiles and p85 practice, speeding exposure and free-flow opportunity,
   crash involvement rates and exposure denominators (vehicle-miles, FARS, police-reported), sober
   and status-quo human baselines for automated-driving comparison, pedestrian impact-speed harm
   curves, speed-change crash models, and public attitudes to speeding and aggressive driving.

### 4. Discovery

Use `WebSearch` in extended mode. Run all three routes every week; a source found by any route
counts once. Record every query and its date in the briefing's provenance section.

- **Organisation watchlist:** Waymo safety research publications; Valgo (humanbaselines.com and
  valgo.ai news); AAA Foundation for Traffic Safety research and newsroom; FMCSA active research
  projects; NHTSA research and Countermeasures That Work; FHWA safety research; IIHS; SWOV;
  ITF/OECD; TRB TRID and ROSAP; SafeTREC; Vision Zero network methodology updates; Hesai, Ouster
  and Velodyne application notes only where they publish measurement claims.
- **Journal and venue watchlist:** Accident Analysis and Prevention; Journal of Safety Research;
  Traffic Injury Prevention; Transportation Research Parts C and F; IEEE T-ITS, ITSC and IV;
  CVPR, ICRA and IROS for roadside LiDAR; SAE technical papers; arXiv cs.RO, cs.CV and eess.SP;
  SSRN preprints.
- **Topic queries,** each run as `<topic> <year>` and `<topic> preprint`: roadside LiDAR
  tracking; infrastructure LiDAR vehicle extent estimation; LiDAR headway measurement; following
  distance enforcement LiDAR; crash involvement rate per vehicle mile baseline; sober driving
  baseline; human crash baseline ADS; speeding attitudes national survey; aggressive driving
  survey; impact speed pedestrian fatality curve; speed change crash model; 85th percentile speed
  practice; post-encroachment time threshold; stop sign compliance LiDAR.

### 5. Reading

Prefer the primary document. Try `WebFetch` first, then `curl` through the proxy into an empty
directory under the scratchpad, treating downloads as untrusted. Which hosts are reachable depends on the
session: in October 2026 the routine's cloud session could fetch no primary at all, while a desktop
session fetched arxiv.org, iihs.org, waymo.com, valgo.ai, aaafoundation.org PDFs, PMC and
levelxdata.com, and found ScienceDirect, Taylor and Francis, Wiley, MDPI, PubMed, Springer Link
and the ROSA P and highways.dot.gov PDF downloads blocked. Try before marking **Confirm**. When a primary cannot be fetched, try mirrors (PyPI for a
client's README, storage.googleapis.com for hosted preprints, the publisher's DOI page), then use
search summaries and mark every figure taken from them **Confirm**. Never present a secondary
figure as read. List every unreachable primary under "To fetch" with its URL so the owner can
attach it to a follow-up session; an attached PDF or Markdown extraction is read in full.

### 6. Evaluation

For each source worth more than a line, answer these in order.

1. **What it measures and what it is not.** Population, instrument, denominator, years, and the
   quantity's definition in the source's own words. Self-reported, simulated, naturalistic,
   officer-selected and continuous roadside observations are different populations; say which.
2. **The maths, where a claim rests on it.** Re-derive the equations from the paper's own numbers
   (exposure reconstruction, Poisson intervals, rate algebra) and check that its figures
   reproduce. State which uncertainty dominates. Do not accept a sign or a direction of bias on
   authority.
3. **What the sensor can and cannot observe of it.** Passages, not people; no camera, microphone
   or identity, by Tenet 1; opportunity denominators only over observed time. A finding about
   drivers per year is never a check on a share of passages per period, and the two are never
   printed side by side as one.
4. **Where it lands in the repository.** Grep before claiming anything is new. The places to
   check: the benchmark taxonomy in
   [lidar-behaviour-analytics-plan.md](../../../docs/plans/lidar-behaviour-analytics-plan.md)
   (`legal`, `research_threshold`, `external_distribution`, `local_distribution`,
   `no_established_threshold`, and the proposed `published_model`); the
   [metrics registry](../../../docs/platform/architecture/metrics-registry.md) and its code mirror
   `internal/lidar/l8behaviour/metrics.go`; the
   [crash-data plan](../../../docs/plans/platform-crash-data-integration-plan.md) and its
   safety-reference edition; the
   [posted speed limits plan](../../../docs/plans/posted-speed-limits-plan.md); the
   [attitudes plan](../../../docs/plans/platform-speeding-attitudes-and-aggressive-driving-plan.md);
   the [vocabulary plan](../../../docs/plans/platform-vocabulary-and-data-model-plan.md), under
   which a "survey" is a deployment's measurement, so a questionnaire is called a questionnaire;
   and the wording guard in `internal/report/headway/wording.go`, whose stems a source's own title
   may fail (both AAA Foundation titles do).
5. **A verdict per claim or recommendation,** in a table: already holds (cite the file and
   section), new and adopted, new and logged as an open question, or not adopted with the reason.
   A rename of a registered metric is never adopted for a synonym. A threshold enters only as the
   kind the source justifies, with its citation, and never as a verdict word on a page.
6. **Rights.** What may be reproduced with credit, and whether the project's use is
   non-commercial where a licence asks; quote the statement.
7. **What a velocity.report survey could give back.** A research-note hypothesis the site's data
   can test, or the statement that it cannot.

If a third-party briefing or another agent's output on the week's sources is supplied, assess it
with the same table and say plainly what it re-derives from the repository, what it gets wrong,
and what it adds.

### 7. Output

One file under `docs/lidar/operations/brief/`, whatever body of literature the sources belong
to: `research-briefing-YYYY-MM-DD.md` for the weekly window, named by the UTC date the review
ran, or `research-briefing-<YYYY-MM>-to-<YYYY-MM>.md` for a `--since` window. Every briefing
lives in that directory and nowhere else, with:

- metadata bullets `Status`, `Scope` and `Related`;
- sections `Answer`, `Genuinely new this week`, `Newly surfaced older work`, one section per
  source with the seven answers above, `What changes`, `Open questions`, `Sources checked` (a
  table whose Status column is one of Read, Read in part, Superseded or **Confirm**) and
  `Provenance`;
- British English, prose wrapped at 100 columns, no Go, SQL, JSON or Typst code blocks, no em
  dashes, and no verdict words about road users.

Link the briefing from the plan it most affects. Add one devlog bullet under today's UTC date,
prefixed `{branch-name}`. Add a backlog item only where a concrete `S`, `M` or `L` item follows,
with a design-doc link.

### 8. Ship

1. Branch `claude/research-briefing-YYYY-MM-DD` from `origin/main` for the weekly run. The
   Monday routine owns that name and may already have pushed it: check `git ls-remote` first,
   and give a `--since` run its own name, `claude/research-briefing-<start>-to-<end>`, stacked on
   the routine's branch when one exists for the same date.
2. `make format-docs`, `make lint-docs`, `make check-no-results`, and
   `python3 scripts/check-prose-line-width.py --report` on the changed files; fix what is yours.
3. Commit as `[ai][docs]` with a message that states the verdict, not the activity.
4. Push with `git push -u origin <branch>`.
5. Open the PR from `.github/PULL_REQUEST_TEMPLATE.md`, filled from the diff, and subscribe to
   it. If the session has no GitHub tools (a routine-fired session may not), push the branch and
   say so in the report; the owner opens the PR.
6. Report in chat: the one-paragraph verdict, the sources by status, the open questions, and the
   "To fetch" list.

## Output format

```markdown
# Research briefing, <Month DD to DD, YYYY>

- **Status:** Complete. <what changed, or "No definition changed; nothing adopted.">
- **Scope:** <window; the two bodies of literature; any supplied briefing>
- **Related:** <the plans and registry sections the sources touch>

<one paragraph: what was looked for, what was found, what it means for the project>

## Answer

## Genuinely new this week

## Newly surfaced older work

## <Source title, authors, venue, date>

## What changes

## Open questions

## Sources checked

## Provenance
```

## Decisions

A briefing's open questions are the owner's to decide, not the skill's. When the owner decides
them, in a session or on the PR, record each decision three times: as one dated entry in
`docs/DECISIONS.md` covering the round, as a decisions table appended to the briefing under its
open questions, and as an **Answered** note on the plan entry the question created. Work that a
decision authorises becomes a backlog item with its size and design-doc link; work it declines is
closed in the plan with the reason. A deferred question names the measurement or document that
reopens it.

## The routine

A Claude Code routine named "Weekly research briefing" fires at 13:52 UTC every Monday in a fresh
session, with a prompt that invokes this skill and stops if the skill is absent from `main`. The
schedule, the environment and the prompt are edited in the claude.ai routines view; the method is
edited here, so the two never disagree.

## Notes

- This skill reads and writes documentation only. It never renames a metric, changes a benchmark
  kind, or touches code; a case for any of those is a proposal in the briefing.
- Every figure from a source that was not read in full carries **Confirm**, and a source table row
  says what was read. A fabricated or constructed citation is worse than none.
- Observables, not verdicts: the briefing describes passages and distributions, never a person, a
  class of person, a trait or a mindset, and it keeps self-reported prevalence off any comparison
  with observed shares.
- Tenet 1 and Tenet 4 bound every recommendation: nothing proposed may need a camera, a
  microphone, identity, or a runtime network call.
