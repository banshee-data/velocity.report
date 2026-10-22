# Counting-line headways at Columbus and Broadway: published tracks against SUMO

Can a published scene tell a traffic simulator how real traffic arrives and leaves a queue? This
report reads counting-line headways for Columbus Avenue north-west-bound off the published
columbus-broadway tracks, runs the same junction in SUMO under three car-following models with
matched parameters, and compares the two at the same line. It is the evidence behind the
[SUMO demand card plan](../../plans/lidar-sumo-demand-card-plan.md).

- **Status:** Complete. One offline analysis of published assets, not a queued experiment: no replay was run and no tuning parameter changed
- **Layers:** published scene export (L5 tracks as exported for the web), offline tooling
- **Related:** [demand card plan](../../plans/lidar-sumo-demand-card-plan.md), [behaviour analytics plan](../../plans/lidar-behaviour-analytics-plan.md), [duplicate tracks](l5-duplicate-tracks-2026-10.md), [web scene export plan](../../plans/lidar-web-scene-export-plan.md)
- **Tools:** [tools/sumo](../../../tools/sumo/README.md)

## Summary

- **Leaving the queue, IDM and EIDM match and Krauss does not.** At the stop line, the measured
  p15 headway (the closest of these statistics to the gap between vehicles discharging from a
  queue) is 2.4 s in both lanes, 95 % bootstrap interval 2.1 to 2.9 s in the inner lane. IDM
  gives 2.4 s and EIDM 2.4 to 2.5 s with the same shared parameters; Krauss gives 1.9 s, outside
  the inner lane's interval.
- **Between platoons, the simulation waits too long.** The inner lane's measured p85 is 35 s
  (interval 19 to 66 s). Krauss and EIDM give 84 to 85 s and IDM 51 s. All three run the same
  guessed signal, a 90 s cycle with three 23 s greens, so this points at the signal timing, not
  the car-following model.
- **Demand matches; the lane split does not.** Measured north-west-bound flow at the stop line is
  272 vehicles an hour against about 280 simulated, but the inner lane carries 58 % of it, while
  SUMO puts 62 % in the kerb lane.
- **The signal cycle is not established.** Folding the 34 measured queue-discharge starts over
  candidate cycles gives only a weak, implausible 45 s peak.
- **The site's north azimuth looks about 18° off.** The sensor-frame axes match the streets'
  crossing angle to within a degree, but under the convention `siteCalibration` documents, both streets
  come out 18° anticlockwise of their mapped bearings: the site would be about 53°, not 35°.

## Terms

**Counting-line headway** is the time between successive vehicles' fronts crossing the same line
in the same lane: the traffic-engineering headway a simulator's detector reports. It is not the
**following net time gap** of the [behaviour analytics plan](../../plans/lidar-behaviour-analytics-plan.md)
(bumper to bumper along a common path, D-28), and nothing here feeds that metric. p15, p50 and
p85 are nearest-rank percentiles over a lane's headways (D-18). The share under 2.5 s is the
fraction of headways shorter than 2.5 s, a common platoon threshold. Lane 0 is the kerb lane, as
in SUMO.

## Data and provenance

| Item                  | Value                                                                                                                   |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| Published scene       | `public_html/src/scenes/columbus-broadway/assets/part-000`, rebuilt in #802                                             |
| Build                 | `0.5.1-pre45`, exported `2026-10-20T10:39:30Z`, trail estimator `cv_kf_v1/online`                                       |
| Tuning hash           | `sha256:ade9f1afa5b40adadae0b97fc5c9f70d9ce69ef6fd2a13000a9c0880fcdd2303`                                               |
| Source VRLOG          | `bcedd3e55a9538c00db492fd354cb8477cfbf15939ef18649e1f065384a4db70`                                                      |
| Recording             | September 2, 2026, 13:20:37 to 13:54:30 PDT, 10,165 frames at stride 2 (5 Hz)                                           |
| Static time           | 28.6 min in eight static parts from `tools/s2-archive/site-index.json`; four gaps left out                              |
| OpenStreetMap extract | Overpass, `osm_base` `2024-09-17T04:52:36Z`, SHA-256 `ce5c8a1a92f70bff105bd7592b5e61e0714ef5b11b23a0a0a84aad60a41f5018` |
| SUMO                  | Eclipse SUMO `v1_27_1+0679-6b4094c74e9`, source build                                                                   |

The four gaps in the static parts (122 to 137 s, 272 to 315 s, 760 to 1000 s and 1201 to 1216 s
from the start) are times when the platform was not parked. Crossings inside them are dropped and
no headway spans one. The rebuild in #802 holds 9 to 20 % fewer track rows at this site than the
recording it replaced; these figures are for the rebuilt export only.

## Method

### Measured side

1. **Vehicles by motion.** Of 11,621 published tracks, 574 travel at least 15 m, reach 4 m/s,
   have a median box length of at least 2.5 m and are never labelled pedestrian, cyclist or bird.
   Class alone would not do: 8,015 tracks are unclassified in most of their frames.
2. **Directions.** Their travel directions fall into four modes in the sensor frame: 103.3°
   (181 tracks), 152.0° (125), 282.6° (152) and 330.1° (123). The two axes are 48.7° apart.
   In the SUMO network, built from OpenStreetMap, Columbus north-west-bound runs at 310° and
   Broadway west-bound at 261°, 49° apart, and only one assignment of the axes has that signed
   angle: 103.3° is Columbus north-west-bound and 152.0° Broadway west-bound. The site's recorded
   `north_azimuth_deg` of 35 settles the remaining 180° ambiguity.
3. **Stop line and lanes.** Along 103.3°, stopped observations (under 0.5 m/s) end at about
   −5 m: the queue front. A bus dwelling at the kerb from about 1,500 s to 1,760 s, at +15 to
   +20 m, is not a queue. Lateral positions of moving observations form two lanes, centred near
   −4.8 m and −2.2 m with a trough near −3.6 m, which is the lane split.
4. **Crossings.** A track's first forward crossing of a line is interpolated linearly between
   frames. Lines sit at −2 m (just past the stop line), +10 m (inside the junction) and +25 m (its
   exit). Same-lane crossings under 0.8 s apart are one vehicle split into two tracks (see
   [duplicate tracks](l5-duplicate-tracks-2026-10.md)) and the later one is dropped: 15 at the
   stop line.
5. **Intervals.** 2,000 bootstrap resamples of each lane's headways, seed 1.
6. **Cycle.** A discharge start is a vehicle stopped within 10 m behind the queue front that
   reaches 2 m/s; starts within 15 s of each other are merged. Their times are folded over every
   cycle from 40 to 130 s and scored by phase concentration.

### Simulated side

The network is built with `netconvert` from the extract above, over
`-122.4110,37.7955,-122.4020,37.8010`: 229 edges and 22 signalised junctions under 20 guessed
signal plans. Two corrections were
needed before the junction behaved: roads with no OSM speed limit came in at 62 mph and are set
to 25 mph, and at that speed netconvert would make every mid-block junction yield to the right,
so its right-before-left threshold is lowered to 5 km/h.

| Setting                | Value                                                                                    |
| ---------------------- | ---------------------------------------------------------------------------------------- |
| Signal at the junction | netconvert's guess: 90 s cycle, three 23 s greens, joined with the neighbouring junction |
| Demand                 | Columbus north-west-bound 280/h, starting 72 m before Broadway; Broadway 400/h each way  |
| Shared vehicle type    | accel 2.6, decel 4.5, tau 1.0, minGap 2.5, length 4.6, speed factor normal(1, 0.1)       |
| Model-specific         | Krauss sigma 0.5; IDM and EIDM delta 4                                                   |
| Counting lines         | Per-vehicle loops at the stop line and 20 m past the junction, both lanes                |
| Run                    | 3,900 s at 0.1 s steps, seed 42; first 300 s left out                                    |

Columbus traffic starts 72 m before Broadway because the block upstream has a single through
lane in the OSM lane data, which held back 160 to 245 vehicles at a 600/h demand.

## Findings

### At the stop line

| Source       | Lane  | Flow (/h) | Headways | p15 (s) | p50 (s) | p85 (s) | Under 2.5 s |
| ------------ | ----- | --------- | -------- | ------- | ------- | ------- | ----------- |
| Lidar        | kerb  | 115       | 47       | 2.4     | 15.7    | 55.2    | 17 %        |
| Lidar        | inner | 157       | 67       | 2.4     | 6.6     | 35.2    | 19 %        |
| SUMO, Krauss | kerb  | 176       | 162      | 1.9     | 5.6     | 66.9    | 23 %        |
| SUMO, Krauss | inner | 107       | 98       | 1.9     | 3.2     | 85.1    | 23 %        |
| SUMO, IDM    | kerb  | 176       | 162      | 2.4     | 11.6    | 51.1    | 23 %        |
| SUMO, IDM    | inner | 105       | 96       | 2.4     | 37.9    | 50.9    | 23 %        |
| SUMO, EIDM   | kerb  | 174       | 160      | 2.5     | 3.8     | 78.0    | 18 %        |
| SUMO, EIDM   | inner | 109       | 100      | 2.4     | 3.7     | 84.2    | 24 %        |

The measured 95 % bootstrap intervals:

| Lane  | p15 (s)    | p85 (s)  | Under 2.5 s |
| ----- | ---------- | -------- | ----------- |
| kerb  | 1.9 to 3.4 | 38 to 79 | 6 to 28 %   |
| inner | 2.1 to 2.9 | 19 to 66 | 10 to 30 %  |

### Along the measured approach

| Line  | Crossings | Duplicates dropped | Kerb p15 / p50 / p85 (s) | Inner p15 / p50 / p85 (s) |
| ----- | --------- | ------------------ | ------------------------ | ------------------------- |
| −2 m  | 130       | 15                 | 2.4 / 15.7 / 55.2        | 2.4 / 6.6 / 35.2          |
| +10 m | 128       | 16                 | 2.9 / 19.9 / 55.1        | 2.4 / 6.8 / 45.8          |
| +25 m | 109       | 8                  | 2.7 / 17.5 / 55.0        | 2.5 / 16.8 / 60.3         |

The inner lane's p15 holds at 2.4 to 2.5 s through the junction. At +25 m, 21 fewer vehicles
cross than at the stop line: tracks that end, or vehicles that turn, before the junction's exit.

### 20 m past the junction, simulated only

| Model  | Kerb p15 / p85 (s), under 2.5 s | Inner p15 / p85 (s), under 2.5 s | Columbus time loss |
| ------ | ------------------------------- | -------------------------------- | ------------------ |
| Krauss | 1.6 / 79.0, 45 %                | 1.6 / 86.9, 64 %                 | 82.4 s             |
| IDM    | 2.3 / 69.8, 27 %                | 2.3 / 85.4, 32 %                 | 97.2 s             |
| EIDM   | 2.0 / 79.9, 41 %                | 2.0 / 85.7, 43 %                 | 96.5 s             |

Time loss is the mean over the 281 Columbus vehicles' whole 10-edge route, three or four signals,
not this junction alone. The measured tracks thin out before this line, so it has no measured
counterpart.

### Signal cycle

The 34 merged discharge starts are 15 to 215 s apart, most between 15 and 40 s, which is shorter
than any plausible cycle for this junction: some starts are a second stop within one green, or a
stop that was not the signal's. The best fold is 45 s at a concentration of 0.39, then 101 s at
0.26. Neither is credible, and the simulation keeps netconvert's 90 s.

## Interpretation

The p15 headway at the stop line is the measurement here that bears most directly on
car-following: it is set by how drivers pull away from a queue, which is what `accel` and `tau`
govern. With every shared parameter equal, IDM and EIDM reproduce it and Krauss is too tight. The
p85 and the median are set by the signal and by arrivals, and no car-following model moves them
much: Krauss and EIDM differ by 0.5 s at p15 and by under 1 s at p85 in the inner lane. Matching
the measured p85 needs the junction's real timing, which these tracks do not give.

The lane split is a demand setting, not behaviour: `departLane="best"` sends most vehicles to the
kerb lane. The real inner-lane preference may come from the bus stop past the junction and from
right turns onto Broadway, neither of which the simulation models.

## What this does not establish

- **One afternoon, one approach.** 47 and 67 headways from 28.6 minutes on September 2. The
  bootstrap treats headways as independent, which platoons are not, so the true intervals are
  wider than those shown.
- **Not physically validated.** Crossings come from published tracks, not annotated references.
  Inner-lane vehicles stand between the sensor and the kerb lane and can hide kerb-lane vehicles,
  which would lengthen that lane's headways. Splits beyond the 0.8 s rule, and tracks that start
  or end near a line, are not measured.
- **Frame timing.** Frames are 0.2 s apart; interpolated crossing times carry that resolution.
- **The orientation rests on geometry.** Which axis is Columbus comes from the 49° angle; which
  end is north-west comes from `north_azimuth_deg`, whose convention is documented as not
  cross-checked. The 18° disagreement found here is evidence about that value, not a correction
  to it.
- **One simulation seed, one guessed signal.** SUMO figures are single runs. The signal timing is
  netconvert's guess, not SFMTA's, and the network is a 2024 OSM extract.
- **Different populations.** Measured flow includes buses and any vehicle the motion filter
  admits; the simulation has one passenger car type.

## Recommendations

1. Use IDM or EIDM, not Krauss, as the starting car-following model for SUMO scenarios built
   from these surveys, and calibrate `accel` and `tau` against the stop-line p15.
2. Do not compare platoon gaps until the signal timing is real: obtain SFMTA's timing card for
   Columbus and Broadway, or infer phases from stopped queues on all four approaches together.
3. Set the lane split from the measured shares (`departLane` per lane) when a card is built.
4. Check the site's `north_azimuth_deg` with the "Align from above" tool: this analysis implies
   about 53° under `siteCalibration`'s convention.
5. Repeat on a second junction before treating any of this as general.

## Reproducing

From the repository root, with `SUMO_HOME` set and the extract above at `<extract.osm>`:

```bash
python3 tools/sumo/lidar-counting-line.py modes --centres 103 152 282 331
python3 tools/sumo/lidar-counting-line.py profile --bearing 103.3
python3 tools/sumo/lidar-counting-line.py headways --bearing 103.3 --lines -2 10 25
tools/sumo/columbus-broadway/build-net.sh <extract.osm>
tools/sumo/columbus-broadway/compare.sh krauss idm eidm
```

The generated network and simulation output stay local in `tools/sumo/columbus-broadway/`
(ignored by Git).
