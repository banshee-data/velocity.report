# The background's length for registration: 60, 90, 180 and 300 s at the wrong-peak sites

In [pose0001](gerardus-static-registration-2026-10.md), five archive sites registered against
3DEP on a wrong peak from 300 s backgrounds. At one of them, R1's 83 s background of the same
capture registers with the same code. This report replays the first 60, 90 and 180 s of each
capture at those five sites and at five sites that pass. It answers
[pose0002](../../../data/experiments/pose/pose0002.md), whose rule was committed (`04955dfae`)
before any replay.

- **Status:** Complete, 2026-10-25. No window adopted and no pose accepted
- **Layers:** L3 (the settled background, by window), Gerardus `register` with the defaults
- **Related:** [static registration of the 24 sites](gerardus-static-registration-2026-10.md),
  [L3 track guard](l3-track-guard-kirk-2026-10.md)
- **Parameter sets:** the [defaults](../../../config/tuning.defaults.json) for every window

## Summary

- **No window is better than 300 s under the rule, so nothing is adopted.** 60 s recovers two of
  the five wrong-peak sites, fulton-divisadero and laguna-eddy, but costs two controls on ground
  p95. 90 s recovers none, and 180 s recovers one.
- **The wrong-peak sites flip between peaks from window to window.** The exported point count
  changes by about 0.3 % between 60 and 300 s. fulton-divisadero registers at 60 s and fails at 90,
  180 and 300 s. laguna-eddy registers at 60 and 180 s and fails at 90 and 300 s. hyde-ofarrell
  alternates between two wrong peaks, and broadway-gough is wrong at every window. The coarse
  search's best peak at these sites is decided by differences far smaller than the window.
- **embarcadero-folsom finds R1's pose at every short window**: heading 49.4°, 5.6 m from the hint,
  separation 1.49–1.55. It fails only facade p75, at 0.25–0.33 m against R1's 0.198 m. The 300 s
  background pushes it onto a peak 23 m away. So the window does decide its peak. Its remaining
  facade difference is in how the background was made, not its length: R1's 83 s background passes
  with today's Gerardus.
- **The ground p95 gate sits inside window-to-window noise at two controls.** At a pose that moves
  no more than 2 cm, ground p95 runs 0.177–0.225 m at columbus-broadway and 0.127–0.203 m at
  van-ness-sacramento. Each control's CANDIDATE state therefore depends on the window.
- Rule 5 expected a window that is no better to mean the window is not the cause. The run does not
  support that inference, and this report does not draw it. See [Interpretation](#interpretation).

## Question

Do a shorter background's cells register the five wrong-peak sites (fulton-divisadero,
broadway-gough, laguna-eddy, hyde-ofarrell, embarcadero-folsom)? Does a shorter window cost the
sites that already pass (ashbury-downey, lombard-laguna, columbus-broadway, van-ness-sacramento,
franklin-mcallister)?

## Method

As pose0001's `base` arm, except for the window. Each capture's first 60, 90 or 180 s was replayed
in analysis mode on the scratch server, with the defaults and the track guard on. The same server
binary was used as for pose0001 (`v0.5.1-pre47`, `feb0652cb`). The preserved grid was persisted and
exported with `cmd/tools/bgasc`. Registration used the same Gerardus build
(`claude/register-experiments`) and the same copy of the data directory, with the defaults, from
the eye-read position. The 300 s results are pose0001's, read again rather than rerun.

The rule, in short:

- A wrong-peak site is _recovered_ when it registers as CANDIDATE.
- A control is _costed_ when it stops being CANDIDATE, its pose moves more than 0.5 m or 1°, or its
  ground p95 rises by more than 0.02 m.
- A window is _better_ than 300 s when it recovers at least two sites and costs no control. The
  shortest better window is chosen.

## Results

### Per site and window

Points is the exported background's point count. "Δ hint" is the best heading less 180° −
`north_azimuth_deg`. "Δ from w300" is the best pose's distance and turn from the 300 s pose. A
`NaN` facade p75 means fewer than 100 held-out wall points met the facade selection.

| Site                | Role       | Window | Points | State     | Best heading | Δ hint | Ground p95 | Facade p75 | Separation | Seeds | Failing gates                                                          | Δ from w300 (m, °) |
| ------------------- | ---------- | ------ | -----: | --------- | -----------: | -----: | ---------: | ---------: | ---------: | ----: | ---------------------------------------------------------------------- | ------------------ |
| fulton-divisadero   | wrong peak | 60 s   |  70917 | CANDIDATE |         80.8 |   +1.8 |      0.149 |      0.096 |       1.40 |  1.00 | –                                                                      | 15.94, -157.0      |
| fulton-divisadero   | wrong peak | 90 s   |  70957 | FAILED    |        245.4 | +166.4 |      0.182 |      0.492 |       1.01 |  0.00 | facade_p75, separation, seed_agreement                                 | 4.37, +7.6         |
| fulton-divisadero   | wrong peak | 180 s  |  71002 | FAILED    |        242.2 | +163.2 |      0.191 |      0.580 |       1.01 |  0.00 | facade_p75, separation, seed_agreement                                 | 5.66, +4.4         |
| fulton-divisadero   | wrong peak | 300 s  |  71114 | FAILED    |        237.8 | +158.8 |      0.216 |        NaN |       1.00 |  0.25 | ground_p95, separation, seed_agreement                                 | 0.00, +0.0         |
| broadway-gough      | wrong peak | 60 s   |  70195 | FAILED    |        162.5 |  +81.5 |      0.256 |      0.517 |       1.00 |  0.00 | ground_p95, facade_p75, separation, seed_agreement                     | 40.74, +174.1      |
| broadway-gough      | wrong peak | 90 s   |  70224 | FAILED    |        347.4 |  -93.6 |      0.222 |      0.391 |       1.09 |  0.65 | ground_p95, facade_p75, separation, seed_agreement                     | 0.15, -1.0         |
| broadway-gough      | wrong peak | 180 s  |  70258 | FAILED    |        347.3 |  -93.7 |      0.237 |      0.533 |       1.01 |  0.00 | ground_p95, facade_p75, separation, seed_agreement                     | 2.25, -1.1         |
| broadway-gough      | wrong peak | 300 s  |  70278 | FAILED    |        348.4 |  -92.6 |      0.221 |      0.391 |       1.05 |  0.25 | ground_p95, facade_p75, facade_range_slope, separation, seed_agreement | 0.00, +0.0         |
| laguna-eddy         | wrong peak | 60 s   |  71631 | CANDIDATE |        170.3 |   -0.7 |      0.071 |      0.129 |       1.52 |  1.00 | –                                                                      | 28.66, -40.8       |
| laguna-eddy         | wrong peak | 90 s   |  71638 | FAILED    |        268.5 |  +97.5 |      0.146 |        NaN |       1.03 |  0.85 | separation                                                             | 23.29, +57.4       |
| laguna-eddy         | wrong peak | 180 s  |  71658 | CANDIDATE |        170.3 |   -0.7 |      0.086 |      0.133 |       1.56 |  1.00 | –                                                                      | 28.67, -40.8       |
| laguna-eddy         | wrong peak | 300 s  |  71785 | FAILED    |        211.1 |  +40.1 |      0.211 |        NaN |       1.00 |  0.15 | ground_p95, separation, seed_agreement                                 | 0.00, +0.0         |
| hyde-ofarrell       | wrong peak | 60 s   |  71408 | FAILED    |        347.8 | -178.2 |      0.182 |      0.488 |       1.04 |  0.80 | facade_p75, separation                                                 | 39.70, -89.1       |
| hyde-ofarrell       | wrong peak | 90 s   |  71424 | FAILED    |         75.5 |  -90.5 |      0.190 |      0.683 |       1.00 |  0.20 | facade_p75, separation, seed_agreement                                 | 7.63, -1.4         |
| hyde-ofarrell       | wrong peak | 180 s  |  71447 | FAILED    |        347.9 | -178.1 |      0.179 |      0.517 |       1.05 |  0.95 | facade_p75, separation                                                 | 37.99, -89.0       |
| hyde-ofarrell       | wrong peak | 300 s  |  71457 | FAILED    |         76.9 |  -89.1 |      0.168 |      0.533 |       1.01 |  0.00 | facade_p75, separation, seed_agreement                                 | 0.00, +0.0         |
| embarcadero-folsom  | wrong peak | 60 s   |  67618 | FAILED    |         49.4 |   +0.4 |      0.101 |      0.246 |       1.55 |  1.00 | facade_p75                                                             | 23.18, +173.6      |
| embarcadero-folsom  | wrong peak | 90 s   |  68003 | FAILED    |         49.4 |   +0.4 |      0.115 |      0.333 |       1.49 |  1.00 | facade_p75                                                             | 23.16, +173.6      |
| embarcadero-folsom  | wrong peak | 180 s  |  68417 | FAILED    |         49.4 |   +0.4 |      0.103 |      0.287 |       1.50 |  1.00 | facade_p75                                                             | 23.16, +173.6      |
| embarcadero-folsom  | wrong peak | 300 s  |  68579 | FAILED    |        235.8 | -173.2 |      0.173 |        NaN |       1.00 |  0.00 | separation, seed_agreement                                             | 0.00, +0.0         |
| ashbury-downey      | control    | 60 s   |  71277 | CANDIDATE |        171.0 |   -1.0 |      0.082 |      0.115 |       1.32 |  1.00 | –                                                                      | 0.00, +0.0         |
| ashbury-downey      | control    | 90 s   |  71290 | CANDIDATE |        171.0 |   -1.0 |      0.085 |      0.107 |       1.32 |  1.00 | –                                                                      | 0.00, +0.0         |
| ashbury-downey      | control    | 180 s  |  71342 | CANDIDATE |        171.0 |   -1.0 |      0.087 |      0.105 |       1.37 |  1.00 | –                                                                      | 0.00, +0.0         |
| ashbury-downey      | control    | 300 s  |  71370 | CANDIDATE |        171.0 |   -1.0 |      0.083 |      0.110 |       1.37 |  1.00 | –                                                                      | 0.00, +0.0         |
| lombard-laguna      | control    | 60 s   |  69045 | CANDIDATE |        265.8 |   +0.8 |      0.154 |      0.083 |       1.26 |  1.00 | –                                                                      | 0.01, +0.0         |
| lombard-laguna      | control    | 90 s   |  69079 | CANDIDATE |        265.8 |   +0.8 |      0.137 |      0.081 |       1.27 |  1.00 | –                                                                      | 0.02, +0.0         |
| lombard-laguna      | control    | 180 s  |  69249 | CANDIDATE |        265.8 |   +0.8 |      0.125 |      0.074 |       1.27 |  1.00 | –                                                                      | 0.00, +0.0         |
| lombard-laguna      | control    | 300 s  |  71510 | CANDIDATE |        265.8 |   +0.8 |      0.122 |      0.078 |       1.26 |  1.00 | –                                                                      | 0.00, +0.0         |
| columbus-broadway   | control    | 60 s   |  69686 | CANDIDATE |        143.6 |   -1.4 |      0.177 |      0.161 |       1.29 |  1.00 | –                                                                      | 0.02, +0.0         |
| columbus-broadway   | control    | 90 s   |  69870 | FAILED    |        143.6 |   -1.4 |      0.225 |      0.174 |       1.41 |  1.00 | ground_p95                                                             | 0.01, +0.0         |
| columbus-broadway   | control    | 180 s  |  70114 | FAILED    |        143.6 |   -1.4 |      0.203 |      0.184 |       1.23 |  1.00 | ground_p95, separation                                                 | 0.00, +0.0         |
| columbus-broadway   | control    | 300 s  |  70387 | CANDIDATE |        143.6 |   -1.4 |      0.185 |      0.174 |       1.46 |  1.00 | –                                                                      | 0.00, +0.0         |
| van-ness-sacramento | control    | 60 s   |  71324 | FAILED    |        175.2 |   +5.2 |      0.203 |      0.172 |       1.43 |  1.00 | ground_p95                                                             | 0.01, -0.2         |
| van-ness-sacramento | control    | 90 s   |  71458 | CANDIDATE |        175.2 |   +5.2 |      0.132 |      0.184 |       1.66 |  1.00 | –                                                                      | 0.01, -0.2         |
| van-ness-sacramento | control    | 180 s  |  71498 | CANDIDATE |        175.4 |   +5.4 |      0.127 |      0.191 |       1.43 |  1.00 | –                                                                      | 0.01, +0.0         |
| van-ness-sacramento | control    | 300 s  |  71801 | CANDIDATE |        175.4 |   +5.4 |      0.146 |      0.173 |       1.58 |  1.00 | –                                                                      | 0.00, +0.0         |
| franklin-mcallister | control    | 60 s   |  70904 | CANDIDATE |        347.5 |   -0.5 |      0.147 |      0.131 |       1.50 |  1.00 | –                                                                      | 0.00, +0.0         |
| franklin-mcallister | control    | 90 s   |  71175 | CANDIDATE |        347.5 |   -0.5 |      0.171 |      0.127 |       1.50 |  1.00 | –                                                                      | 0.02, +0.0         |
| franklin-mcallister | control    | 180 s  |  71250 | CANDIDATE |        347.5 |   -0.5 |      0.174 |      0.154 |       1.51 |  1.00 | –                                                                      | 0.01, +0.0         |
| franklin-mcallister | control    | 300 s  |  71295 | CANDIDATE |        347.5 |   -0.5 |      0.174 |      0.130 |       1.46 |  1.00 | –                                                                      | 0.00, +0.0         |

### Against the rule

| Window | Recovered (rule 1)                | Right-headed, not CANDIDATE | Controls costed (rule 2)                                                            | Better than 300 s (rule 3) |
| ------ | --------------------------------- | --------------------------- | ----------------------------------------------------------------------------------- | -------------------------- |
| 60 s   | 2: fulton-divisadero, laguna-eddy | embarcadero-folsom          | lombard-laguna (ground p95 +0.032), van-ness-sacramento (failed; ground p95 +0.057) | no                         |
| 90 s   | 0: –                              | embarcadero-folsom          | columbus-broadway (failed; ground p95 +0.040)                                       | no                         |
| 180 s  | 1: laguna-eddy                    | embarcadero-folsom          | columbus-broadway (failed)                                                          | no                         |

No window is better than 300 s (rule 3). The run took 14 minutes for the 30 backgrounds and
10 minutes for the 30 registrations.

## Interpretation

1. **The rule's verdict stands: no window is adopted, and REGISTRATION-STATUS does not change.**
   60 s comes closest. It recovers fulton-divisadero and laguna-eddy, and leaves embarcadero-folsom
   one gate short. It fails the rule on two controls whose poses did not move.
2. **The window is not a dial for these sites; the coarse search is unstable there.** If length
   were the cause, outcomes would move steadily with the window. Instead fulton-divisadero and
   laguna-eddy alternate between a right and a wrong peak, and hyde-ofarrell between two wrong
   ones. The backgrounds differ by a few hundred points, so the two peaks score within the noise of
   which cells settled first. pose0001 recorded separations of 1.00 to 1.05 at these sites, which
   says the same: the best and second hypotheses cost almost the same. Choosing a window would only
   choose which side of the coin lands up.
3. **Rule 5's inference does not hold.** It expected that, with no better window, the window would
   not be the cause at these sites. At embarcadero-folsom it is: every short window finds R1's pose,
   and 300 s does not. The open part there is facade p75, 0.25–0.33 m against 0.198 m. R1's
   background registers with today's code (pose0001's `r1bg` arm), so the difference is in how that
   background was made on 2026-10-14. That covers the L3 defaults changed since (l3bg0023 and the
   track guard) and the replay path. pose0001 already showed the guard alone does not move this site
   at 300 s.
4. **Two gates sit inside the background's own noise.** Ground p95 moves 0.05–0.08 m between windows
   at a fixed pose at columbus-broadway and van-ness-sacramento. Their margin to the 0.20 m gate is
   smaller than that. One background is one draw. A state decided by one draw near the threshold is
   not a measurement of the pose.
5. **What follows: register from several windows and accept on agreement.** Each wrong-peak site
   already has one window that lands on the hint's heading with every other gate passing. Requiring
   the same pose from several windows would separate a stable pose from a lucky draw. That is the
   proposed [pose0003](../../../data/experiments/pose/pose0003.md). It is a change to the
   procedure, not to Gerardus's gates, and it needs no new code.

## Limitations

- No pose is checked against a surveyed position.
- Ten sites, one night, each window counted from the start of the capture.
- The controls are five of the six pose0001 candidates. embarcadero-bryant has no facade gate, so
  it was left out.

## Provenance

- Spec and rule: `04955dfae` on `claude/pose-background-window`, on `main` `ecec0ba22` (#860).
- Server binary `v0.5.1-pre47`, git sha `feb0652cb`; tuning fingerprint `e44f577dc1df`.
- Gerardus `claude/register-experiments` (banshee-data/gerardus#34), the build used for pose0001.
- Raw outputs stay under `/Volumes/lidar/gerardus-exp-20261024/`, outside the repository.
