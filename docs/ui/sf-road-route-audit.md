# San Francisco rebaked road route audit

Twenty emoji-to-emoji courses exercised the ground contact in the 157-tile Gerardus
rebake. The audit tests street continuity, approximate longitudinal flatness and the
difference between a visible deck and the surface that supports the car.

## Release and method

[PR #1123](https://github.com/banshee-data/velocity.report/pull/1123) had already
published the rebake. The [deployed game manifest](https://velocity.report/sf/tiles/manifest.json)
and its five changed gzip tiles matched the merged checkout byte for byte. The browser
used the published game and its static tiles.

- Game code: `237ead0278567c39a2a9440919d29f0b5360beac`.
- Game tile manifest SHA-256: `cee97394d57d2c07efeffd8a99f57ab4708877a3140f649dbdd867f8ba3ea5ad`.
- Road layer: `sf-city-roads-97993185300b116bf296169b1f0e88f7685e9fe18af57d696c3b89742813e3cc`.
- [Course set](../../tools/sf-drive/road-audit-routes.json) SHA-256:
  `64e112dab81327e39c6d7afd7c02c97ce98847d673691158cedb885b392d5a20`.
- Node 22.17.1 and Chrome 157.0.8103.80 on Hydra.

**Goals and Traffic.** Each course starts at an existing emoji intersection and tries to
collect both endpoints. The test driver steers and uses the pedals at 120 Hz with targets
of 23 mph on streets and 40 mph on highways. It slows for turns. A separate graph follows
the baked centrelines and one-way flags and uses exact 0.1 m coordinate joins. The graph
does not know the level of a crossing. The start and finish may include short connectors
from the emoji position to a named centreline.

**World, Physics and Data.** The driver calls the game's terrain, vehicle, collision,
hard-landing and pickup functions. It ends at first serious impact, sustained stall,
missing ground or arrival at an unresolved tunnel. It preloads tiles, omits traffic and
does not replay recovery or mutate scenery. One-metre samples from each full planned
centreline give absolute grade p95 and peak. A dip flag is a sample more than 0.5 m below
the chord between points 4 m before and after it. Adjacent flags can belong to one dip.
These measurements describe the ground lookup in the game, including the ground below
a highway where the visible deck lacks contact. They do not measure road crossfall.
Former tunnel alignments mark the approach targets and are excluded from the road grade
metrics. Heights above those alignments describe exterior terrain.

**UI.** Short live Chrome drives used saved checkpoints, the Meat mode prompt and the
normal keyboard controls. The authored Bay Bridge controller was exercised separately.
The offline smoke reopened cached tiles, collected an emoji and restored the pickup and
three airtime records on another offline reload.

```sh
node tools/sf-drive/audit-road-routes.mjs
node tools/sf-drive/audit-road-flatness.mjs
node tools/sf-drive/replay-city-drive.mjs
SF_BASE_URL=https://velocity.report node tools/sf-drive/probe-road-routes.cjs
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-terrain-review.cjs
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-offline.cjs
```

The audit accepts one or more course IDs from the course set as positional arguments.
It reads removed tunnel alignments from the pinned parent commit, so Git history must
include that revision. To repeat the original audit after a new bake, use a checkout
at `5cf29faf5`, which contains the script and its matching tile bundle. For browser commands, make Playwright available on `NODE_PATH`
and set `CHROMIUM_PATH` to the Chrome executable.

## Route results

The driver covered **26.07 km in 42 min 55 s of simulation**. Eight routes collected
both endpoints. Six ended in tree impacts, one in a building impact, one at a contact
gap, one stalled, and three reached unresolved tunnel approaches. All 20 collected the
starting emoji. Across 22 crossed tile boundaries, the largest change in car altitude
over one 120 Hz step was 5.7 cm. That observation covers sampled crossings at driven
speeds and does not establish citywide seam continuity. The driver recorded 37 completed
flights with at least 5 cm of clearance. The highest clearance was 0.41 m on the
Dogpatch 21st Street course.

The 48.05 km of planned courses contained 86 one-metre intervals over 50% absolute
grade, 77 dip samples and 82 unsupported samples. Planned metrics include portions
beyond the finish of a blocked drive. The I-280 course alone contributed 51 steep
intervals and 50 dip samples. Its centreline is draped onto ground for this measurement.

| Course               | Emoji endpoints                                           | Driven / planned (km) | Ground grade p95 / peak | Dip samples | Outcome                                       |
| -------------------- | --------------------------------------------------------- | --------------------: | ----------------------: | ----------: | --------------------------------------------- |
| `waterfront`         | ⚓ Green St → 🌉 Bryant St                                |           0.73 / 1.94 |             1.3% / 2.7% |           0 | Tree on Embarcadero                           |
| `soma-under-deck`    | 🏙️ Harrison & Vassar → 🎭 Natoma & 6th                    |           1.20 / 1.49 |             6.5% / 8.8% |           0 | Driver stalled on Howard                      |
| `mission-bay`        | 🏀 Warriors Way → 🌉 Bryant St                            |           0.44 / 2.48 |             1.8% / 7.3% |           0 | Tree on Terry A Francois                      |
| `dogpatch-21st`      | 🛣️ Watchman & Missouri → 🐾 Louisiana & 21st              |           1.71 / 1.92 |           18.9% / 86.4% |           9 | Building near 22nd after descent via 20th     |
| `dogpatch-23rd`      | 🚂 Texas & 17th → 🐾 Louisiana & 21st                     |           1.30 / 2.45 |           18.9% / 21.4% |           0 | West 23rd contact gap                         |
| `stockton-tunnel`    | 🐉 Stockton & Bush → 🍝 Grant & Green                     |           0.38 / 1.66 |           11.3% / 15.6% |           0 | South approach                                |
| `broadway-tunnel`    | 🛤️ Broadway & Hyde → 🍝 Grant & Green                     |           0.17 / 1.55 |           10.7% / 41.0% |           0 | West approach                                 |
| `toll-plaza-tunnel`  | 🪵 Kobbe & Greenough → 🌁 US 101 ramp                     |           1.26 / 1.63 |            8.9% / 40.0% |           3 | South-west approach                           |
| `presidio-west`      | 🪵 Kobbe & Greenough → 🚲 26th & Lake                     |           2.18 / 2.19 |           11.0% / 23.2% |           0 | Both collected                                |
| `soma-i280`          | 🛞 5th St ramps → 🚚 Toland & Galvez                      |           0.33 / 9.42 |          20.5% / 123.4% |          50 | Car stayed below I-280 ramp and struck a tree |
| `freeway-to-bayview` | 📦 Sweeny & Barneveld → 🚵 Le Conte & Meade               |           1.39 / 2.79 |           15.0% / 57.2% |           9 | Tree at the US 101 alignment                  |
| `ocean-beach`        | 🏄 48th & Kirkham → 🐚 47th & Ulloa                       |           2.16 / 2.17 |             2.4% / 3.7% |           0 | Both collected                                |
| `north-beach-hills`  | 🍫 North Point & Columbus → 🍝 Grant & Green              |           0.15 / 1.24 |             6.2% / 7.8% |           0 | Tree on Columbus                              |
| `pacific-heights`    | 🦢 Lombard & Scott → 🪞 Laguna & Jackson                  |           1.70 / 1.71 |           17.9% / 19.1% |           0 | Both collected                                |
| `castro-mission`     | 🏳️‍🌈 Castro & 17th → 🌮 Capp & 20th                         |           2.01 / 2.02 |             5.1% / 6.5% |           0 | Both collected                                |
| `twin-peaks`         | 📷 Panorama & Longview → 🗻 Dellbrook & La Avanzada       |           1.20 / 1.21 |           16.1% / 26.6% |           0 | Both collected                                |
| `sunset-avenues`     | 🌈 28th & Moraga → 🏄 48th & Kirkham                      |           2.30 / 2.30 |            8.7% / 10.1% |           0 | Both collected                                |
| `golden-gate-park`   | 🛹 6th & Fulton → 🦬 Chain of Lakes & MLK Jr              |           1.32 / 3.72 |            6.7% / 44.6% |           2 | Tree on JFK Drive                             |
| `lake-merced`        | 🛶 Middlefield & Lake Merced → 🎣 John Muir & Lake Merced |           2.52 / 2.54 |             5.0% / 6.2% |           0 | Both collected                                |
| `excelsior`          | 🌞 Congo & Joost → 🗺️ Avalon & Madrid                     |           1.61 / 1.62 |           21.3% / 58.2% |           4 | Both collected despite sharp dip on Baden     |

## Reproductions and acceptance

World-plane coordinates below use the game origin and elevations use NAVD88.

| Site                                      | Observation                                                                                                                                                                                                                                                                                                                          | Review target                                                                                                                                            |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 5th Street I-280 south ramp               | At (3693, 1356) Chrome stayed near 3.1 m and entered a crash replay. The baked raised run `overpass/19330000/forward/1` is 13.75 m within 1 m horizontally. About 1 km farther along, the I-280 alignment samples ground at a 123% grade and a 1.46 m dip.                                                                           | Verify ramp entry contact and same-level joins. Keep the ground beneath the ramp usable on its own. Screen the deck profile independently of the ground. |
| 20th Street, Dogpatch                     | At (4226, 31), the game centreline descends 0.86 m in 1 m. The deepest local departure from the 8 m chord is 1.44 m near (4232, 32). Chrome lost contact at about (4216–4218, 31) under straight throttle.                                                                                                                           | Check whether the ground grid mixes the roadway and adjacent embankment. Compare pavement observations across the grade break before changing contact.   |
| 21st and 23rd streets, Dogpatch           | Chrome cleared the reviewed open lane at 21st and Louisiana. At 23rd it stopped at (4168, −542) at 15.11 m and recovered an old checkpoint to supported ground. The course found 71 unsupported samples across the affected crossing.                                                                                                | Keep the valid approach and require evidence for both sides of the 23rd Street crossing and the connection beneath I-280.                                |
| Embarcadero near Green to Bryant          | The controller hit a tree within 0.18 m of the test centreline at (3988, 3946). The short Chrome approach slowed to nearly zero on the same alignment.                                                                                                                                                                               | Check the trunk placement and near-kerb clearance against the road geometry. Review the comparable JFK, Columbus and Terry A Francois contacts.          |
| Stockton, Broadway and toll-plaza tunnels | The driver reached each approach and stopped before attempting unsupported contact. The rebake removes four road rows across these three structures. Exterior samples on their former alignments reach 80%, 88% and 57% peak grades, respectively. Chrome observed at-grade motion at Stockton and a crash on the Broadway approach. | Survey the interiors and both portals, including contact, clearance and joins. Do not reinterpret exterior ground as the tunnel surface.                 |
| Bay Bridge, Fremont and Fourth            | Live Chrome completed all six sections, including Fremont and Fourth. The fixed-step replay completed in 235.96 s with no contact loss or scenery collision. Both 50 mph rail probes rebounded at the Fourth Street entrance.                                                                                                        | Keep the authored route as the reference when promoting other elevated corridors.                                                                        |

The independent citywide audit again found 469 intervals above 50% absolute grade
among 1,191,156 supported intervals. Its highest flagged interval was 125.1% on
Blanken Avenue at (3945, −5441). The route sample also flags Baden Street at
(36, −3304) and the 10th Avenue connection in Golden Gate Park. Review support and
level classification at the highest flags before refitting pavement. A hill,
embankment, rail cutting or different road level can all produce a large number in
these screens.

Treat the successful drives as game checks on this immutable revision. Route selection,
steering and collision envelopes can contribute to failures. Field clearance,
unpaved detours and universal highway or tunnel traversability remain unverified.

## Follow-up on the Stockton and I-280 bake

Twenty new emoji courses on game commit `91e074ad02a8c9b734f10c9023602a97383f3b1d`
covered **35.78 km across 59 min 03 s of simulation**. Eleven collected both emojis.
The game served the 157-tile manifest with SHA-256
`180d246e5ca46e7eba8e7ea35a70af33250178b3ba065bbd67edc9fc3355a597`.
The [follow-up course set](../../tools/sf-drive/road-audit-layered-routes.json) has SHA-256
`0c899c373ca1777bf71ba6633dd069b6e2477612b4c2d485043d68b4ba46f0e9`.

```sh
node tools/sf-drive/audit-road-routes.mjs --plan=tools/sf-drive/road-audit-layered-routes.json
node tools/sf-drive/audit-road-flatness.mjs
node tools/sf-drive/replay-baked-structure.mjs
node tools/sf-drive/replay-city-drive.mjs
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-stockton-tunnel.cjs
```

Use `--assets=/path/to/sf` with an archived copy of this manifest, its tiles and its quest
file when the checked-out bundle has moved on. `--plan` selects a course set. Positional
course IDs limit a run to those courses.

**World and Physics.** In addition to terrain, the 120 Hz driver uses the game’s baked
road network and portal contact. Stockton’s two itineraries explicitly use its baked
floor. The I-280 approach detects the entrance during the drive and retains raised
contact until the last declared terminal. The check stops at contact loss or a serious
impact. It preloads tiles and excludes traffic. Chrome separately drove both Stockton
directions with keyboard controls and switched between tunnel and exterior cameras.
It read 50.41 m on the street above the bore. The Bay Bridge fixed-step drive completed
all six sections, including Fremont and Fourth, in 235.96 s.

**Goals and Data.** The car collected all 20 starting emojis. The table measures absolute
grade from grounded car positions, resampled at one-metre intervals along the driven
distance. A dip is more than 0.5 m below the chord over the surrounding eight metres.
An airborne sample invalidates its dip window. The separate planned-profile screen
queries pavement for explicit layered courses and terrain elsewhere. Its 62 steep
intervals, 36 dip samples and 82 unsupported samples include terrain below elevated
routes and portions beyond a drive’s stopping point.

| Course                 | Emoji endpoints                                       | Driven (km) | Grade p95 / peak | Outcome                    |
| ---------------------- | ----------------------------------------------------- | ----------: | ---------------: | -------------------------- |
| Stockton northbound    | 🚋 Market & Spear → 🍝 Grant & Green                  |        2.66 |      5.9% / 9.0% | Both collected             |
| Stockton southbound    | 🦀 North Point & Kearny → 🧱 Larkin & Cedar           |        3.54 |    10.3% / 14.2% | Both collected             |
| Broadway westbound     | 🦀 North Point & Kearny → 🛤️ Broadway & Hyde          |        1.48 |    12.0% / 14.2% | Unresolved east portal     |
| Toll Plaza northbound  | 🌸 First Dr West & North → 🌁 US 101 ramp             |        2.24 |     8.9% / 21.8% | Unresolved tunnel approach |
| Below I-80             | 🏹 Folsom & Embarcadero → 🏙️ Harrison & Vassar        |        1.06 |     8.0% / 98.1% | Stalled on 2nd St          |
| Fifth Street I-280     | 🛞 Fifth St ramps → 🚂 Texas & 17th                   |        1.14 |      5.5% / 8.6% | End of raised contact      |
| US 101 into Bayview    | 🪡 Palou & Phelps → 🥬 Arleta & Rutland               |        2.32 |    10.7% / 54.1% | Tree near US 101 alignment |
| Dogpatch 17th          | 🏥 Potrero & 18th → 🚂 Texas & 17th                   |        1.28 |    11.2% / 18.8% | Both collected             |
| Dogpatch 23rd          | 🏗️ Marin & Michigan → 🛣️ Watchman & Missouri          |        1.22 |      4.4% / 6.4% | Missing ground on 23rd     |
| Mission Creek          | ⚾ Bridgeview & Toni Stone → 🏀 Warriors Way          |        0.65 |      1.4% / 4.1% | Tree on Terry A Francois   |
| North waterfront       | 🍫 North Point & Columbus → ⚓ Green & Embarcadero    |        1.17 |      2.5% / 4.1% | Stalled on Embarcadero     |
| Presidio               | 🌸 First Dr West & North → 🚪 Arguello & Washington   |        1.33 |    13.2% / 20.6% | Both collected             |
| Outer Richmond         | 🌷 36th & Clement → 🦆 Fulton & 38th                  |        1.22 |    12.5% / 13.7% | Both collected             |
| Golden Gate Park west  | ⛳ JFK & Golf Course Access → 🚣 Stow Lake & JFK      |        1.63 |      4.5% / 7.7% | Tree on JFK Drive          |
| Sunset                 | 🪻 39th & Santiago → 🧗 8th & Ortega                  |        3.73 |    16.3% / 23.8% | Both collected             |
| Great Highway          | 🦒 Great Highway & Sloat → 🌅 48th & Pacheco          |        1.73 |      1.8% / 5.0% | Both collected             |
| Lake Merced southbound | 🐟 Lake Merced & Higuera → 🎣 Lake Merced & John Muir |        1.15 |      5.0% / 6.0% | Both collected             |
| Diamond Heights        | 💎 Jade & Gold Mine → ⛰️ Molimo & Myra                |        3.00 |    11.0% / 22.6% | Both collected             |
| Mission southbound     | 🖌️ 14th & Natoma → 🌮 Capp & 20th                     |        1.22 |      2.6% / 3.8% | Both collected             |
| Excelsior southbound   | 🌺 Yale & Silver → 🧳 Moscow & France                 |        2.02 |    17.0% / 22.1% | Both collected             |

Neither Stockton floor profile had a dip flag. Its baked centreline was 4.7% at the
95th percentile and 4.72% at the peak in both directions. The I-280 car remained on the
deck at (4035.5, 667.7), 18.21 m NAVD88, before the unsupported continuation. The
largest one-step altitude change across 34 driven tile crossings was 6.1 cm. There
were 27 completed flights at or above 5 cm clearance, with a 0.53 m maximum. No fully
grounded eight-metre window in the driven traces met the dip criterion.

The 2nd Street run showed a 98.1% rise on Harrison at (4297.5, 3046.4) and later
stopped at (3979.9, 2670.0). The US 101 run recorded a 54.1% rise on Silver Avenue
at (3047.2, −3048.0) and stopped at a tree near (3438.8, −4119.7). Review those levels
and the 23rd Street contact gap at
(4239.1, −536.2) before extending the routes. The independent terrain screen still found
469 intervals over 50%, including the 125.1% maximum on Blanken Avenue. The planned
profiles and observed car traces cover a subset of game contact. They describe game
behaviour and route availability rather than field grades or clearance.

## Follow-up on the Presidio and Toll Plaza bake

The [20-course regression set](../../tools/sf-drive/road-audit-presidio-release-routes.json)
uses candidate manifest `db80bd6d2ec42bf43da05e40a1ce8e29a6128c18fc46676b7a68014f59019031`.
It reuses 18 courses from the Stockton and I-280 follow-up and re-routes Broadway
to use the baked westbound bore and Toll Plaza to attempt the new eastbound
contact. The course set SHA-256 is
`95b177641d68ccbe4ec8d65f9447f33ca451bc79ee19ae4aa7d599cd5fe71633`.
Run it with
`node tools/sf-drive/audit-road-routes.mjs --plan=tools/sf-drive/road-audit-presidio-release-routes.json`.

Twelve of the 20 fixed-step drives collected both emojis. There were no
depressions over 0.5 m below the local eight-metre chord in the driven
profiles. The largest tile-seam step was 0.061 m. Both Stockton directions
and the Broadway drive used their lower surfaces. The eight unfinished
courses stopped under the following conditions.

| Course               | Stop                                                            | Peak driven grade |
| -------------------- | --------------------------------------------------------------- | ----------------: |
| Toll Plaza eastbound | Stalled on the upper approach and never selected the lower road |            27.05% |
| Fremont under-deck   | Stalled on 2nd Street at 18.5 m                                 |            98.07% |
| 5th Street I-280     | Reached the end of route contact before the destination         |             8.60% |
| US 101 Bayview       | Hit a tree on the southbound highway alignment                  |            54.08% |
| Dogpatch 23rd Street | Reached missing ground contact                                  |             6.40% |
| Mission Creek        | Hit a tree on Terry A Francois Boulevard                        |             4.14% |
| North Waterfront     | Stalled on The Embarcadero                                      |             4.09% |
| Park West            | Hit a tree on John F Kennedy Drive                              |             7.74% |

The largest grade flag occurred on Harrison Street at (4297.52, 3046.36)
in tile `8085807c`, before the 2nd Street stop. The Bayview course flagged
54.08% on Silver Avenue at (3047.17, −3048.00) in `808f7efc`. Both values
refer to sampled game contact.

Four separate Chrome corridor drives started on pavement immediately before
the Presidio and Toll Plaza portals. Both US 101 runs traversed Battery and
Main Post at peak speeds around 52 mph. Both Toll Plaza runs used the lower
road, at peak speeds around 20 mph. Each run returned to exterior pavement.
The Toll Plaza difference between the corridor and emoji runs identifies a
remaining approach routing or alignment problem. Full emoji itineraries
through the Presidio tunnels also remain unverified. See the
[portal and roof evidence](../plans/ui-sf-layered-roads-plan.md#presidio-parkway-and-toll-plaza-contact).
