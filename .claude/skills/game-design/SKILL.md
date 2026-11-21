---
name: game-design
description: Frame SF driving-game discussions, changes, and reviews using World, Physics, Traffic, Goals, UI, and Data. Use for the /sf/ page, sf-prefixed browser modules and tests, game asset helpers, tools/sf-drive, and SF game documentation; also use for the public scene design shared with the homepage hero and survey viewer.
argument-hint: "[discussion topic, file paths, branch, or PR]"
---

# Skill: game-design

Apply a consistent design vocabulary to the SF game and its reviews. This is the canonical
procedure for Codex, Copilot, Claude, and other agents; it does not select a model or start
another agent.

Invoke it through the client's skill picker, with `$game-design` in Codex, or with
`/game-design` in Claude. A client without skill discovery can read this file directly.
The repository and scoped review instructions require this framing for game changes even
when the reviewer was not given an explicit skill invocation.

## Read the framing

Read [the six anchors and their references](../../../.github/knowledge/game-design.md) before
planning, changing, or reviewing game behaviour. The exact labels are **World, Physics,
Traffic, Goals, UI, Data**. Their definitions live in that module, not in client wrappers.
Read [AGENTS.md](../../../AGENTS.md) and the current implementation/documentation relevant to
the task. Use [repo-docs](../repo-docs/SKILL.md) when producing a durable design or review document.

Apply this workflow to:

- `public_html/src/sf/**`, including the page, route, manifest, and tile assets.
- `public_html/src/js/sf-*` and `public_html/src/js/__tests__/sf-*` tests.
- `public_html/sf-*` and `tools/sf-drive/**`.
- `docs/ui/sf-*.md` and changes to this framing or its agent entry points.
- Shared build, rendering, input, or data code when the change affects the SF game.

Apply the public scene design guidance below to changes in the homepage hero or survey
viewer that affect the shared appearance. Apply the game anchors when the change affects
the game. Other public-site content and LiDAR processing have their own conventions.

## Public scene design

Use the [shared scene palette](../../../public_html/src/js/scene-colours.js) for WebGL
and the generated `--scene-*` CSS variables for page styles. Define any new scene
colour role there. Use the survey viewer as the reference for measured object boxes.

| Object                | Colour role                        | Treatment |
| --------------------- | ---------------------------------- | --------- |
| Motor vehicles        | `vehicle` red                      | Bright outline, subdued fill. The player car also uses red, with a solid body and wheels. |
| Pedestrians           | `walking` hero green               | Bright outline, subdued fill. |
| Cyclists              | `cycle` blue                       | Bright outline, subdued fill. |
| Buildings             | `buildingWall`, `buildingRoof`     | Muted grey walls, tan roofs, translucent faces and quiet edges. Use `buildingInferred` for the game's inferred tiers. |
| Trees                 | `treeCrown`, `treeTrunk`           | Dark emerald foliage and brown trunks. Keep the silhouette softer than a road-user outline. |

Keep the dark canvas, crisp class-coloured outlines, subdued geometry and legible mono
readouts across the three surfaces. The homepage uses a procedural illustration and
labels it as such. The survey and game replays use the shared
[box geometry and placement conventions](../../../public_html/src/js/scene-track-box.js).
Preserve track class and measured position when restyling playback. Keep measurement
colours such as histogram limit bands and the game's gold navigation cues attached to
their existing meanings.

When changing a shared role or object, inspect the homepage hero, a survey with vectors,
and `/sf/` near a survey. Check the player's view through translucent buildings, the
class outlines and the mobile controls and histogram. For replay changes, verify that
the chart and link still identify the same recording as the traffic shown.

## Apply it

1. Inspect the requested behaviour, diff, relevant code, and existing evidence. A discussion
   uses the framing without requiring a code change. An implementation or review also traces
   its affected files, tests, and player/actor state transitions.
2. For each coherent change, name its primary anchor and any other anchors it affects.
   Multiple labels are allowed; justify each by an actual dependency or observable effect.
   Keep a PR containing several changes as several rows rather than forcing one label for all.
3. Describe the trigger and before/after behaviour, the concrete mechanic, the play dynamics
   it may produce, and the intended experience. Separate observed effects from hypotheses.
4. Check the feedback loop: action, state change, observable signal, next decision. Trace
   relevant coupling across input, motion, traffic policy, objectives, feedback, and records.
   Examples include a crash's effect on the timer/pickups and a metric's relationship to the
   behaviour shown in a replay.
5. For autonomous or shared traffic, apply the actor contract in the framing: observations,
   decisions, physical execution, and visible/recorded evidence. Name handling of unexpected
   hazards, missing/stale information, and conflicting actions. Distinguish physical contact
   from behavioural interaction and an implemented feature from a future scale target.
6. Choose validation appropriate to the affected contract. Use meaningful simulation/state
   tests for behaviour, browser checks for inputs/cues/layout, and measured workloads for
   performance claims. Documentation-only edits need document/link checks. Follow the
   repository's required checks without inventing an extra approval gate.
7. Update descriptions and relevant documentation with the same anchors and terms. Preserve
   existing scope constraints and authorised decisions; do not turn this review into an
   unrequested geometry rebake, asset-format migration, or traffic-engine implementation.

## Output and review behaviour

For a proposal or implementation, scale this compact shape to the work:

| Change             | Primary    | Also affects     | Trigger and before/after          | Evidence or open question                            |
| ------------------ | ---------- | ---------------- | --------------------------------- | ---------------------------------------------------- |
| Concrete behaviour | One anchor | Relevant anchors | What the player/actor experiences | Test, observation, measurement, or stated hypothesis |

For code review, preserve the client's required finding format. Use the anchors to explain
actionable findings, with affected lines, a concrete failure or design consequence, and the
supporting evidence. Check correctness, maintainability, privacy, and performance alongside
the design framing. Separate bugs from design tradeoffs and preferences. Do not manufacture
findings, demand all six headings, or treat a missing category label alone as a correctness
blocker. If there are no actionable findings, say so and name any material unverified claims.

For a short change, an anchor and a clear sentence are enough. This is shared review language,
not a requirement to write a long design report for every edit.
