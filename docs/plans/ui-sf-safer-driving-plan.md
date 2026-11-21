# SF safer driving milestones

This proposes two successive milestones for the San Francisco driving game: a solo run
that rewards controlled driving, then a repeatable following and braking scenario.
It records a design for review, not implemented behaviour or a claim about real road safety.

- **Status:** proposed; scoring weights and the role of existing time trials remain open.
- **Consumer:** [SF driving view](../ui/sf-driving-view.md).
- **Framing:** [game-design anchors](../../.github/knowledge/game-design.md).

## Scope and success

The proposed main safety mode asks the player to **drive like Grandma's in the car**:
plan ahead, stay on the road, and leave space. Show elapsed time as context, not as a
reward, tie-breaker, or target. The existing game has emoji pickups and time trials;
whether to retain them as a separate mode, change their role, or retire them is an open
decision. This plan neither changes the current timer nor assumes that pickups advance
a safety score.

| Change                                      | Primary | Also affects              | Player-facing intent                                                |
| ------------------------------------------- | ------- | ------------------------- | ------------------------------------------------------------------- |
| Verify the reported dip and define surfaces | World   | Physics, Data             | Real crests behave consistently; a map fault is not a driving test. |
| Separate tyre contact from airborne motion  | Physics | World, Goals, UI, Data    | Carry momentum over a crest, then regain grip on landing.           |
| Score a solo route without a speed bonus    | Goals   | World, Physics, UI, Data  | Reward controlled driving and show the evidence behind deductions.  |
| Add a repeatable lead car and intersection  | Traffic | World, Physics, Goals, UI | Anticipate following, signals, and downhill stopping.               |
| Explain individual events and route trends  | Data    | Traffic, Goals, UI        | See what happened, why it mattered, and what to try next.           |

Controls and cues sit under **UI**, not a separate anchor. Keep the live driving
view clear; richer feedback belongs at a stop or at the end of a run.

The [implemented solo practice slice](../ui/sf-solo-practice.md) supplies a controlled
fixture, separate run lifecycle and evidence-first results. The Harrison/Beale road-level attribution,
city-route applicability and production weights remain unresolved; this does not close
all milestone acceptance gates.

## Milestone 1: Solo driving and a credible crest

### World and Physics: separate the reported dip from an actual crest

David located the dip at Harrison Street and Beale Street. The
[Milestone 1 investigation](../ui/sf-solo-practice.md#harrison-at-beale-reproduced-dip-unresolved-road-level-attribution)
reproduces the baked height valley in both directions with pinned geometry and contact
traces; the correct road level and source cause remain unverified. Preserve route,
direction, speed, coordinates, tile and manifest revision when comparing a fix. Compare the
height profile with the source geometry. Check tile seams, interpolation, draped asphalt,
and contact calculations before deciding that the tile is wrong. If a map fault is verified,
repair or rebake only the affected tile or dependency set, then repeat the same approach
and check adjacent seams and neighbouring roads. Do not flatten a legitimate crest or
rebake the city to hide an unlocated report.

Use a separate, verified crest fixture to develop the driving behaviour. Maintain a
grounded/airborne contact state. A grounded vehicle can lose contact as the road falls away;
a launch carries its existing velocity, and gravity governs its subsequent vertical motion.
Tyre steering, traction, and braking forces do not act in mid-air. Normal steering and grip
return on landing. This is a game response to verified geometry, not an assertion that the
current coarse terrain grid reproduces a particular vehicle's real jump.

### Goals, UI, and Data: count what the player did

- **Speed:** For a known, sourced posted limit, measure positive excess in non-overlapping
  bands: `(0, 5)`, `[5, 10)`, and `≥10` mph. Accumulate elapsed time in each band from
  simulation time, not frame counts; show the largest excess over a known limit. Exactly
  5 or 10 mph over belongs to the next band; zero excess belongs to none. Surface known-limit
  coverage as time over total accounted driving time, with unknown-limit time separate.
  Never substitute a guessed limit or count unknown time as compliant. Make ambiguous
  segments and time during which driving is not evaluated explicit in the accounting.
- **Course:** Count a continuous departure from drivable space once, from exit to re-entry,
  and distinguish separate kerb strikes from sustained contact. Use the drivable surface
  beneath an airborne car at the applicable level for this classification: being airborne
  above that surface is not itself an off-road excursion. Preserve the excursion and
  contact evidence, but charge one score incident when a strike and excursion are part
  of the same manoeuvre. Document the association and re-entry/reset rules before scoring.
- **Feedback:** Show the known limit or its absence, and give a brief, legible reason for
  an incident. A route summary gives time by excess band, peak excess, limit coverage, and
  distinct incidents. Do not award a benefit for driving faster. Set exact score weights
  only after comparative, repeated runs on the same route with contrasting driving styles.
  Time, coverage, metrics, and their eventual score contribution should remain separable.

### Acceptance

- [ ] Reproduce and confirm or dismiss the reported dip independently of the crest fixture.
      A targeted map fix, if warranted, removes the false dip without changing an actual crest
      or introducing a seam.
- [ ] At the same crest, a slow pass retains contact and a suitable faster pass takes off,
      carries momentum under gravity, does not steer in the air, and regains tyre control on
      landing. Repeat at different rendering rates with the same simulation input.
- [ ] Boundary cases at exactly 0, 5, and 10 mph excess, a posted-limit change, and an
      unknown segment reconcile to elapsed time without overlaps or assumed compliance.
- [ ] One sustained departure, a re-entry followed by a second departure, and sustained
      versus separate kerb contacts give the expected event counts. Test a strike leading to
      one excursion, and airborne travel above a drivable road, to prevent duplicate or false penalties.
- [ ] On repeated controlled and rushed passes, faster completion alone cannot improve a
      safety result. Review the live cues and summary before adopting score weights.

This milestone depends on confirmed route geometry, applicable posted-limit provenance and
coverage, and usable drivable-boundary and kerb-contact definitions. The existing renderer's
kerb lines are not evidence that kerb impacts are already modelled.

## Milestone 2: Following and a downhill intersection

Add one deterministic lead car and one downhill intersection before general traffic.
Use a fixed starting state, route, and scripted lead-car and signal timing for comparable
runs; document when traffic flow or right of way can legitimately change the player's
choices. Label scenario-authored controls clearly. Do not infer a real signal, stop line,
crosswalk, directional limit, or road level from a road centreline or signpost.

**World and Traffic:** The scenario declares grade, the drivable approach, applicable
controls, the stop line and crosswalk, and the intended lead and signal sequence.
The lead car's policy responds to its declared observations and rules, then uses normal
vehicle physics. Record both the scenario ground truth and what the actors could observe.
If a required tile, target, or signal state is missing, fail the affected exercise or
withhold its score instead of inventing certainty. Broader traffic, multilayer roads, and
networked play are outside this milestone.

**Data:** While moving, record time headway, bumper-to-bumper gap, and relative closing
speed in the same travel path. Report experimental deceleration rate to avoid a crash
(DRAC) only for eligible closing cases with a positive supported gap, and explain its
constant-lead-speed assumption. Do not report missing or non-closing evidence as a
measured safe value. These are simulation traces; neither a game scenario nor the
[experimental survey method](../lidar/operations/survey-experimental-drac.md)
establishes independent physical accuracy. At a stop, record the distinct stopped gap.
Use seeing the lead vehicle's tyres meeting the tarmac as a coaching cue, not a universal
distance threshold; its visibility depends on vehicle and view geometry.

**Physics, Goals, and UI:** Record braking onset and speed at onset, distance and time to
stop, peak deceleration, and peak jerk. Define event endpoints, units, and the jerk calculation
before comparing runs. Compare the final stop with the stop line and crosswalk and report
signal compliance. Separate comfort from safety: planned smooth braking can be good
coaching, but never reward delaying an emergency response to preserve a comfort score.
When a hard brake is warranted, explain the reason separately from the severity.
Offer a short, annotated playback of an event only when stopped, exit it on renewed
movement, and roll up comparable events and coverage at the end of the route.

### Acceptance

- [ ] Replays of the same declared scenario and inputs have comparable lead behaviour,
      signals, and event traces at the stated simulation step. Verify both a shared-path
      lead and cases with no valid lead or insufficient evidence.
- [ ] Exercise a stable moving gap, a closing pair, and a stopped queue; verify the gap and
      timing calculations against known simulation states and distinguish an unavailable
      DRAC value from a non-closing condition.
- [ ] On the same downhill approach, compare early planned braking, a late stop or
      overrun, and a necessary emergency stop. Check onset speed, stop distance/time,
      peak deceleration/jerk, line and crossing outcomes, and that comfort does not override
      justified emergency action.
- [ ] From a complete route, each annotation resolves to the recorded event and the
      aggregate reconciles with the trace. Playback never interrupts driving and exits when
      motion resumes. Keep the compact controls and live visibility tested on mobile.

Start this milestone after the solo contact, course, and accounting gates pass. A first
fixture can use an at-grade approach with verified grade and explicit scenario controls.
Live district export and the [layered-road work](ui-sf-layered-roads-plan.md) are not
dependencies unless the selected route requires them. Do not score a crossing or grade
separation that the map cannot distinguish.

## Decisions before implementation

1. Confirm the first repeatable solo and downhill routes and verify road-level attribution at Harrison/Beale.
2. Pick posted-limit and approach-control sources, identify unknown coverage, and distinguish
   verified street features from authored scenario inputs.
3. Decide what happens to pickups and time trials when the proposed safety mode becomes the
   main experience.
4. Use comparative runs to set score weights and review coaching thresholds and messaging.
   Keep raw traces local and the score breakdown transparent so a rule change can be evaluated separately.
