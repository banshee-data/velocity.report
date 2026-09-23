# Experiment: route-regime speed accuracy against a fixed radar

Compare moving-rig speed measurements with a fixed radar, retaining timing and pose uncertainty
alongside the result. The comparison determines which observations are suitable for publication.

- **Status:** Proposed
- **Layers:** L5 Tracks, L7 Scene, L8 Analytics, radar
- **Related plan:** [lidar-route-capture-plan.md](../../../docs/plans/lidar-route-capture-plan.md)

## Goal

Decide whether speeds measured from a moving rig, after ego-motion compensation, are trustworthy
enough to publish as segment aggregates, and set the ego-uncertainty gate and the minimum sample
rule from evidence rather than judgement.

## Hypothesis

For vehicles seen by both sensors, world-frame maximum speeds from the route regime agree with the
fixed radar's per-transit maximum speeds with a bias under 0.5 m/s and a scatter under 1.5 m/s,
first for the archive's car drives and then for the cargo bike, and the disagreement grows with
the trajectory's reported ego uncertainty rather than with vehicle speed.

## Method

1. Deploy the fixed radar at a site on a straight segment the rigs can pass repeatedly, with its
   cosine correction configured as for any radar site.
2. Car proxy first: drive past the site several times in each direction with the LiDAR running,
   then the bike, then the backpack at walking pace, each on the same day and hour band.
3. Process each pass through the route regime: odometry, trajectory file, world-anchored
   foreground, world-frame tracking, projection onto the segment.
4. Match vehicles between the two sensors by direction and time of passing the radar's beam, with
   a tolerance derived from the segment geometry; discard ambiguous matches and count them.
5. Compare per-vehicle maxima: bias, scatter, and their dependence on ego uncertainty, on ego
   speed, and on distance from the rig; repeat for the typical observed speed.
6. Compute band aggregates from the LiDAR passes and from the radar over the same window and
   report both with sample sizes.

## Inputs and setup

- Radar: the production OPS243 path and a site record with cosine correction.
- LiDAR: the rig's PCAP bundle per pass, with IMU and GNSS streams where fitted.
- Timing: qualify the [acquisition clock and delay model](../../../docs/lidar/architecture/portable-capture-timing.md)
  independently, retain timing rejection counts, and validate the radar-to-LiDAR epoch used for
  matching. Agreement after a freely fitted time shift is not proof of accurate acquisition time.
- Config: `config/tuning.defaults.json` plus the route-regime parameters, recorded with each run.
- Tooling: `velocity lidar pcap-split` for regime labelling; the odometry recipe from the priors
  experiment; analysis-mode replay with the file-backed pose provider.
- Environment: workstation replay; no live pipeline.

## Success criteria

- [ ] Car proxy: bias under 0.5 m/s and scatter under 1.5 m/s on at least 30 matched vehicles.
- [ ] Bike: the same bounds, or a stated ego-uncertainty threshold above which observations are
      excluded that brings the remainder inside them.
- [ ] Backpack at walking pace: inside the bounds without exclusions.
- [ ] Disagreement correlates with ego uncertainty, so the gate has something to act on.
- [ ] Band aggregates from LiDAR and radar agree within the radar's own run-to-run variation at
      the same sample size.

## Risks and controls

| Risk                                              | Control                                                                              |
| ------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Radar and LiDAR see different vehicle populations | Match by time and direction; report unmatched counts on both sides                   |
| Radar cosine error masquerades as LiDAR bias      | Use a straight segment with a small radar angle; verify the site's correction first  |
| The rig's own passage perturbs traffic            | Compare the radar's aggregates with and without the rig present                      |
| Few vehicles during the passes                    | Repeat passes until the matched count reaches the criterion; report presence minutes |

## Outputs

- Matched-vehicle table per pass with both speeds, ego uncertainty, and distance.
- Bias and scatter summary per rig, with plots against ego uncertainty and ego speed.
- Timing uncertainty/state and accepted coverage per pass, including range and angular-motion bins.
- Recommended ego-uncertainty gate and minimum sample size for band aggregates.

## Result

Fill after running.

## Next action

Adopt the gate and sample rule in the plan, promote to `data/explore/` with the tables, or hold
segment aggregates back until the bike's registration improves.
