# Game design and review anchors

Use this shared vocabulary to discuss, implement, and review the San Francisco driving game.
The six anchors describe the target of a change; they also keep future traffic simulations,
multiple players, and individual autonomous drivers within the same design language.

## The six anchors

Use these exact one-word labels. A change may span several anchors. Name its primary target
and the other anchors it actually affects; do not add labels merely to fill the list.

| Anchor      | What it targets                                                                                                                                   | Boundary                                                                                                                                                          |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **World**   | Roads, intersections, terrain, buildings, trees, and drivable space; geometry, topology, coordinates, and tile ownership                          | The environment actors inhabit. Vehicle response belongs to Physics; an actor's route choice belongs to Traffic.                                                  |
| **Physics** | Vehicle motion, steering response, grip, friction, braking, hill forces, integration, and physical collision detection/resolution                 | How an action moves a vehicle and how bodies make contact, whether controlled by a person or a policy. Following, yielding, and changing lanes belong to Traffic. |
| **Traffic** | Interactions between road users, individual driving styles, route/behaviour policies, right of way, following, merging, yielding, and road safety | What each actor chooses in the situation it perceives. Aggressiveness is a policy choice, not an unexplained change in friction or collision rules.               |
| **Goals**   | Quests, pickups, eligibility, scoring, time trials, completion, recovery rules, and saved results                                                 | What counts as progress, success, failure, or a penalty. A timer's meaning belongs here; its display belongs to UI and its recorded evidence to Data.             |
| **UI**      | Controls, input bindings, touch/keyboard access, layout, cameras, visual presentation, navigation cues, and feedback                              | What the player can do, see, hear, and understand. Graph styling belongs here; the metric, recorded events, and replay semantics belong to Data.                  |
| **Data**    | Metric definitions, graphs and analysis, recorded events, actor traces, measurements, provenance, and replays                                     | What is observed, calculated, recorded, and explained. Source geometry belongs to World; policy decisions belong to Traffic.                                      |

Performance, accessibility, privacy, reproducibility, and multiplayer consistency are review
considerations across the anchors, not extra category names. Use the relevant anchors for an
optimisation: for example, a geometry index is World, a contact solver is Physics, and batched
driver decisions are Traffic. Keep the separate asset-accounting investigation linked as
evidence; this framing does not settle its numbers or authorise an asset-format migration.

## Shared description language

Separate the implemented change, its effect during play, and the intended experience:

- **Mechanic:** a concrete rule, control, parameter, or state transition, such as a pickup
  radius or a two-second recovery interval.
- **Dynamics:** behaviour that emerges during play, such as route choice, following gaps,
  repeated crash/reset shortcuts, or interactions between cautious and aggressive drivers.
  Vehicle dynamics is specifically Physics; game dynamics can span every anchor.
- **Experience:** the intended effect on the player, such as legibility, mastery, exploration,
  or confidence in what happened. In the MDA reference, this is the aesthetics lens, not a
  synonym for visual rendering.

Describe an update as: **anchor(s), trigger, before/after behaviour, intended effect, evidence**.
Use one row per coherent change when a PR contains several different targets. Prefer a
concrete description such as "Physics: reduce steering angle as speed increases" to a broad
claim such as "improve the engine" or "make driving realistic". State what was checked before
claiming physical fidelity, safety improvement, or support for a particular actor count.

Keep these terms distinct:

| Terms                                | Meaning                                                                                                                                                                        |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Collision, impact feedback, recovery | Physics detects/resolves contact; UI communicates it; Goals defines when/where control resumes and what happens to the run.                                                    |
| Heading, bearing, relative direction | Heading is the vehicle's orientation; bearing is the target's world direction; relative direction is the target angle from the vehicle's heading.                              |
| Geometry, topology, policy           | World supplies surfaces and connections; Traffic chooses actions and routes using declared information.                                                                        |
| Measurement, estimate, inference     | Report their evidence and assumptions separately. A fitted surface, simulated driver, or extrapolated byte budget is not physical validation.                                  |
| Replay, re-simulation                | Playback of recorded states is a replay. Re-running inputs/policies requires a stated determinism contract; do not promise identical results across browsers without evidence. |
| Download, disk, decoded data, memory | Encoded HTTP payload, stored files, expanded content, and heap/GPU usage are different measures. State units, compression, cache treatment, and coverage.                      |

Every relevant review checks the feedback loop: **player or actor action, state change,
observable signal, next decision**. Feedback remains explicit within UI rather than becoming
a duplicate anchor. A brake-lamp cue can therefore be primarily UI with a Physics dependency.

## Individual actors and shared streets

For proposed autonomous traffic, describe each actor's contract in this order:

1. **World observations:** the road geometry, nearby actors, signals, hazards, and navigation
   information it can use, with coordinate frame, range, observation age, and any occlusion
   or noise assumptions. Distinguish local perception from privileged global state.
2. **Traffic decisions:** its goals, route, right-of-way policy, driving style, and response to
   other actors. Identify whether input comes from a human, local policy, or remote player.
3. **Physics execution:** how intent becomes steering, throttle, and braking, and how motion
   and contact advance. Make vehicle parameters explicit rather than secretly changing the
   physical rules for different input owners.
4. **UI and Data:** what the player sees and what a trace records about observations, intent,
   motion, contacts, and outcomes. An observer's ground truth and an actor's perceived state
   must not be confused.

For a blocked route, unexpected actor, stale observation, missing tile, or disagreement
between actors, name the fallback: for example, slow, yield, stop, or replan. Check that the
actor only uses information its declared observation contract permits. Different degrees of
aggressiveness should be comparable under the same World and Physics assumptions.

Traffic is a road-safety anchor, including pedestrians and cyclists when relevant. Ask what
behaviour a rule or reward encourages, who bears the consequences, and what the evidence
supports. Faster completion is not itself evidence of safer behaviour. Distinguish an
intentional game-design tradeoff from a correctness defect; the framing does not prohibit
time trials or introduce a new approval step.

Multiple autonomous actors, multiplayer, and hundreds of active actors are design goals until
implemented and measured. For a change that claims them, identify active actor count, scene,
observation neighbourhoods, policy and physics tick rates, frame-time distribution, and
CPU/GPU/memory/network budgets. Distinguish simulated actors from rendered actors. For shared
play, state authority, ownership of input, clock/tick ordering, and how stale or conflicting
updates are handled. Do not require a networking architecture for an unrelated local UI edit.

## Review examples

| Change                                           | Primary anchor | Other affected anchors and the question to check                                                                                  |
| ------------------------------------------------ | -------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| Reduce tyre grip on grass                        | Physics        | World: surface classification; Traffic/Goals when route choice or recorded time comparisons change.                               |
| Give a driver a shorter following-gap preference | Traffic        | Physics: attainable braking; Data: gaps and conflicts measured; UI: visible explanation if needed.                                |
| Correct a road/ground overlap                    | World          | UI: visible surface; Physics if the drivable surface or queried height also changes.                                              |
| Collect one emoji per eligible tile              | Goals          | World: ownership/eligibility; UI: seeker/counter; Data: collection and completion evidence.                                       |
| Add a crash spin before recovery                 | UI             | Goals: wait, pickup eligibility, clock and recovery destination; Physics only when physical motion/contact also changes.          |
| Add brake lights or change a compass layout      | UI             | Physics for the actual brake state; Goals for target selection; check what the cue communicates.                                  |
| Define a near-miss metric and a replay graph     | Data           | Traffic: which interactions it describes; UI: plotting/readability; distinguish a simulated proxy from validated safety evidence. |
| Re-encode unchanged map geometry                 | World          | Data: byte/provenance accounting; UI if labels change; preserve decoded geometry and report actual transfer measurements.         |

These examples establish vocabulary, not immutable gameplay rules. Read the current code and
the requested change before describing a contract as preserved or intentionally changed.

## References and sources of truth

Read the sources relevant to the change, rather than treating every reference as a checklist.

| Reference                                                                                                                                  | Use                                                                                                             |
| ------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------- |
| [SF driving behaviour and implementation](../../docs/ui/sf-driving-view.md)                                                                | Current controls, run/recovery behaviour, navigation, rendering, and accounting.                                |
| [City map record](../../docs/ui/sf-city-map.md) and [size study](../../docs/ui/sf-city-map-size-study.md)                                  | World data, representation decisions, and the provenance/limits of size claims.                                 |
| [Geometry data purpose](../../docs/geometry-priors-data-purpose.md)                                                                        | Language for measured/inferred geometry and its acquisition limits.                                             |
| [Project tenets](../../TENETS.md)                                                                                                          | Privacy, evidence, and protection of vulnerable road users.                                                     |
| [MDA: A Formal Approach to Game Design and Game Research](https://users.cs.northwestern.edu/~hunicke/MDA.pdf), Hunicke, LeBlanc, and Zubek | Separate mechanics, emergent dynamics, and player experience; do not replace the six anchors with these lenses. |
| [Fix Your Timestep!](https://gafferongames.com/post/fix_your_timestep/), Glenn Fiedler                                                     | Reason about simulation stepping, stability, and frame-rate dependence.                                         |
| [Game Accessibility Guidelines](https://gameaccessibilityguidelines.com/)                                                                  | Input access, readable feedback, and alternatives to a single sensory cue.                                      |
| [FHWA Zero Deaths resources](https://highways.dot.gov/safety/zero-deaths)                                                                  | Safe System context for road-user vulnerability and consequences; not validation of a game metric.              |
| [Resource Timing: encodedBodySize](https://developer.mozilla.org/en-US/docs/Web/API/PerformanceResourceTiming/encodedBodySize)             | Encoded response bodies versus expanded content, headers, and cache effects.                                    |

The six-anchor vocabulary is canonical here. The
[game-design skill](../../.claude/skills/game-design/SKILL.md) defines how agents apply it;
client entry points and folder instructions link to these shared sources.
