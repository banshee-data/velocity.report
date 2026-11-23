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

## Harrison and Silver Avenue crossing trials

The class-17 returns follow an upper roadway where the ground reader dropped
towards a different level. At Harrison and Beale, the sampled upper road is
15.58 m NAVD88 above the 8.00 m ground mesh. On Silver Avenue above US 101
the two game contacts are 22.69 m and 16.21 m. The April 2023 survey resolves
the upper Silver Avenue deck through station 60 m, then sees other decks
around 31 m and 38 m over the road. The game interpolates the lower street
between that last supported band and the ground east of the crossing.

The [two-course check](../../tools/sf-drive/road-audit-stacked-streets.json)
pins candidate game manifest
`895a62bdaaeb007844f5562d58e2cce5171d1a2d018c6baefd9e81e1bc8e1c9c`.
Run it with
`node tools/sf-drive/audit-road-routes.mjs --plan=tools/sf-drive/road-audit-stacked-streets.json`.

| Crossing                  | Prior peak driven grade | Peak grade on the baked strip | Drive beyond exit |
| ------------------------- | ----------------------: | ----------------------------: | ----------------: |
| Harrison over Beale       |                  98.07% |                         6.99% |             536 m |
| Silver Avenue over US 101 |                  54.08% |                         2.64% |           2.43 km |

Both courses collected both emojis. The revised Silver Avenue course
crosses westbound, then follows exterior streets to Rutland. The grade
comparison is against the earlier itinerary, which continued along US 101.
Chrome drove over each new strip in both directions. Two additional Chrome
runs drove underneath on Beale and southbound US 101. They stayed on the
lower surface at 4.99–8.27 m and 15.76–16.82 m, respectively. The upper
deck bake retains the existing lower terrain. The Beale rise in the aerial
shadow still needs lower-road support review.

## Dogpatch 23rd Street lower deck

A 2023 3DEP depth-12 crop separated the lower roadway at about 15.09 m
NAVD88 from the I-280 band at 24.27–25.81 m. The lower returns continue
to about (4194, −539.7), then disappear through roughly 44 m of aerial
shadow before the east approach. Caltrain's project records confirm the
23rd Street road bridge over the tracks. They give the track clearance
below the bridge and leave the street grade in the freeway shadow
undetermined. [Caltrain’s bridge project](https://www.caltrain.com/projects/san-francisco-roadway-bridges-project) ·
[CPUC crossing record](https://docs.cpuc.ca.gov/PublishedDocs/Published/G000/M076/K494/76494704.PDF)

The game fits continuous lower contact between the observed 15.09 m deck
and the existing 9.90 m eastern pavement. The ground triangles still have
the open aerial gap. The game contact replaces the old display preview
and leaves the upper I-280 band separate. Its steepest sampled grade is
16.46%, within the game interpolation. The shadowed grade remains
unmeasured.

The [20-course regression](../../tools/sf-drive/road-audit-dogpatch-release-routes.json)
pins manifest `4efed0d53bfed9d295f158a3808e5620a6fc10f0381616578677bde2ab1c412a`.
Its SHA-256 is
`bd24039683ab893666fa2d45c7cf5c5f4817452aa188cd72e8960c32de6975b5`.
Fifteen courses collected both emojis, compared with fourteen on the
Harrison and Silver Avenue bake. The 23rd Street course used the lower
surface and continued 910 m after leaving it. Chrome also cleared the
two directions at peak speeds of 21.4 and 20.5 mph. The replay found no
driven dips over 0.5 m and measured a maximum seam step of 0.061 m across
the 20 courses. The five remaining stops were Toll Plaza's emoji approach,
the end of the selected I-280 contact, a stall on The Embarcadero and two
tree impacts on Terry A Francois Boulevard and John F Kennedy Drive.

## Rounded Toll Plaza approach

The first Toll Plaza bake accepted a car that was already lined up with the
tunnel. The Presidio emoji course arrived at the west junction while still
turning. In that run the car followed the upper terrain past the lower
portal. The revised game contact starts 16 m back on the connected west
street and rounds the 96° turn into the same lower bore. The curve is game
geometry pinned to the existing street and tunnel alignments.

The [rounded-approach regression](../../tools/sf-drive/road-audit-toll-rounded-release-routes.json)
pins manifest `b6f59e014f14bdc281f909d04e1d9fe2c9a114639c5c06d1029677fae594e2ab`.
Its SHA-256 is
`2993f2dcb4a49890c10669f29dac5b677545d15101fc6babe080ce266733b617`.
The Toll Plaza car collected both emojis and reached 251 m beyond its
eastern handoff. Chrome drove both directions and verified grounded
exterior contact at each exit.

Sixteen of the 20 fixed-step emoji courses completed. The largest tile-seam
step was 0.061 m. None of the driven profiles had a depression over 0.5 m
using the eight-metre chord test. The remaining stops were the end of the
selected I-280 route, a stall on The Embarcadero and tree impacts on Terry
A Francois Boulevard and John F Kennedy Drive.

## Southbound I-280 through the 18th Street split

The DataSF southbound mainline CNN `19340000` shares node `52165000`
with the end of the existing 5th Street course and the existing 18th
Street off-ramp. The 2023 aerial depth-12 sample follows the mainline near
20.49 m at station 372 m and 20.36 m at station 396 m. At station 384 m
it sees the crossing ramp near 26.99 m while the lower mainline is
shadowed. The bake interpolates that lower interval and matches game
terrain at station 420 m near 20.38 m. The added strip has a peak grade
of 3.27%.

Chrome drove from the 5th Street entrance through the split and under
the separate ramp. The mainline run reached 47.4 mph and returned to
grounded contact at 20.38 m. The Pennsylvania Avenue and 18th Street
exit drives approached the split at ramp speed and steered into the
exit lane before the fork. The geometry retains both connections.

Run `node tools/sf-drive/audit-road-routes.mjs --plan=tools/sf-drive/road-audit-i280-through-routes.json`
for the three fixed-step courses on the pinned manifest. The Pennsylvania
course collected both emojis and drove 388 m after leaving the off-ramp.
The westbound 18th Street course drove 1.73 km beyond its exit before
the previously observed tree impact. The through course reached 811 m
beyond the new strip. Its peak grade of 73.3% was on an existing I-280
ground profile south of the strip near (4192, −173). It stopped at
missing contact around (4198, −534) on the next off-ramp.

The Chrome regression uses
`tools/sf-drive/smoke-presidio-tunnels.cjs i280-mainline i280-pennsylvania i280-18th`.
The new game manifest SHA-256 is
`259d1ff5fc3faa6a32ff0bc68c7321546ff22834d15229c46d234f190754a2b5`.

## Dogpatch I-280 exits, entrances and through lanes

At (4192.36, −173.02), the aerial ground surface sits at 9.84 m
while the class-17 highway pavement sits at 23.48 m. The new game
contact follows the upper pavement over the lower roads and through
both Pennsylvania Avenue exit branches. Two street approaches join
the southbound mainline again. It continues to the matched terrain
handoff near US 101 at about 9.51 m.

Transects through the exit fork supported a roughly 9 m game strip.
The fit shifts the centre 1.5 m west where the observed pavement is
asymmetric. The mainline continuation peaks at 6.43% grade. Its two
inferred building-face overlaps are clipped inside the pavement strip.
The bake carries identical complete spans in their intersecting tiles
and preserves the separate 23rd Street crossing below.

The fixed-step runs collected both emojis via each Pennsylvania Avenue
exit. The north exit continued 1.15 km beyond its portal and the south
exit continued 694 m. The southbound course from 5th Street to Sweeny
and Barneveld completed approximately 8.8 km. It flagged a steep
segment farther along Alemany Boulevard for follow-up.

Seventeen of the 20 courses completed. The driven profiles had zero
depressions over 0.5 m and a maximum tile-seam step of 0.061 m. The
remaining stops were The Embarcadero stall and tree impacts on Terry
A Francois Boulevard and John F Kennedy Drive. This round follows the
new I-280 continuation to Sweeny and Barneveld in place of the shorter
I-280 course.

Run `node tools/sf-drive/audit-road-routes.mjs --plan=tools/sf-drive/road-audit-i280-dogpatch-loop-routes.json`
to replay the 20 courses on game manifest
`50a3d9a63b2a667d15bf5805ee5b6fd9fd476707f995c1345c246313473aec02`.
The Chrome regression covers the through lane, both exits and both
entrances with the named I-280 cases in
`tools/sf-drive/smoke-presidio-tunnels.cjs`.

Chrome also drove a continuous trip from 5th Street, off at Pennsylvania
Avenue, along the street and onto southbound I-280 again. The car was
grounded on the highway at 14.69 m after rejoining.

## Alemany Boulevard over I-280

The 2023 aerial sample shows Alemany pavement near 36.45 m and the
northbound freeway from 28.63 to 28.90 m in the crossing area. The
earlier game course crossed the lower ground triangles and measured
a peak grade of 62.68% near Alemany. The new strip follows the upper
band and reaches the existing pavement at both ends. Its peak grade
is 2.94%. Both owners receive the same complete 156 m strip.

Chrome crossed the upper street in both directions at up to 25.5 mph
and drove the lower freeway within the 28.63–28.90 m band. The 5th
Street to Sweeny and Barneveld replay collected both emojis again.
The maximum driven grade on that course was 17.48% on Cambridge
Street. No driven depression was greater than 0.5 m.

The Alemany game manifest SHA-256 is
`88617e9851bae2ab896d5c02ac28d40f828d8534ac054d984d9610df9bea844a`.

## Yerba Buena westbound to Fremont and Fourth Street

The 592 m Yerba Buena start adds an east approach, the 164.52 m upper
tunnel, and a west approach to the existing Bay Bridge drive. The tunnel
connects the exposed pavement at 55.96 m and 52.31 m NAVD88 with an
interpolated profile. The maximum tunnel grade is 2.92%. The three
handoffs through the tunnel and onto the west span use common endpoints.
The lower eastbound bore and ramp contact still need elevation
and alignment evidence.

The fixed-step run started at 50 mph and completed all nine sections in
261.91 s of game time, with zero contact loss or scenery strikes. It
left I-80 at Fremont and returned to westbound I-80 at Fourth. Both
50 mph Fourth Street side probes rebounded without a crash. The original
six-section run completed in 235.96 s.

`node tools/sf-drive/replay-city-drive.mjs yerba-buena-westbound` replays the
new start. With Playwright available via `NODE_PATH`,
`node tools/sf-drive/smoke-yerba-buena.cjs` drives it in Chrome. The browser
test checks grounded contact, the tunnel camera and the west span, Fremont
and Fourth transitions. The game ships the full route with its checksummed
same-origin bundle. Its manifest SHA-256 is
`dcab6482510a25336b68c5cc81c7437c053ae1c423f2f27f4502dd215f5b7e9b`.

## Geary Boulevard beneath Masonic

The Geary–Masonic regional review verified some surface joins and left the
through connection unresolved. DataSF CNN 6105000 identifies the through
alignment. The 2023 aerial returns at the east and west mouths place the
lower pavement at 76.34 m and 77.33 m NAVD88. The 161 m covered section
interpolates between those observations. The full contact is 303 m with a
peak grade of 3.19%. Both owners carry the identical strip.
[Caltrans tunnel inventory](https://dot.ca.gov/-/media/dot-media/programs/maintenance/documents/f0009163-localtunnellist-a11y.pdf)

The portal search uses the contact width to accept cars entering in the
outer lane. Chrome drove westbound at up to 33.9 mph and eastbound at up
to 34.0 mph, returning to exterior ground at both ends. The eastbound
test used the clear inner lane after a wider line hit a tree on the open
approach. A separate Masonic crossing stayed near 84 m above the bore.
The test runs are `geary-westbound` and `geary-eastbound` in
`tools/sf-drive/smoke-presidio-tunnels.cjs`. The game manifest SHA-256 is
`9c1957990db3cbed492a4bf37bd43a841e5715486c8d2db97850b560ff5fb440`.

## Branch East off-ramp to San Jose Avenue

The bake fits three contact strips in the Branch East crossing. The
northbound exit runs near 48.1 m NAVD88 beneath the I-280 deck near
54.0 m. The viaduct farther along holds the exit at 56.7 m where the
ground drops to 46.6 m. The 144 m covered part of the exit interpolates
the two visible portal elevations. The exit and mainline each keep one
complete strip across both owners.

The continuous Chrome preview followed the northbound freeway fork
through the lower bore and across the viaduct to San Jose Avenue at up
to 38.4 mph. The first run hit a tree before the bore. The accepted
run used the clear lane around the trees and grounded after both
portals. A separate drive stayed on the upper I-280 deck. The isolated
checks are `branch-east-ramp`, `branch-east-upper` and
`branch-east-san-jose` in `tools/sf-drive/smoke-presidio-tunnels.cjs`.
The game manifest SHA-256 is
`025e4d4a9f7a223e0070cc1fab406b25673671bdfb93a810625051c5a07740a9`.

## Twenty-course replay on the Branch East bake

The centreline steering pass completed 17 of 20 courses over 49.35 km. It
hit trees on Terry A Francois Boulevard and John F Kennedy Drive and
stalled after repeated scrapes on The Embarcadero. The lane replay
completed all 20 courses over 52.45 km. It recorded zero driven depressions
over 0.5 m, a maximum 0.061 m tile seam step and a maximum driven grade of
26.18% on Ordway Street. The three adjusted courses collected both emojis.

| Course           | Driven distance | Lane choices near the stopped poses                                                              |
| ---------------- | --------------: | ------------------------------------------------------------------------------------------------ |
| Mission Creek    |         1.09 km | Right at `(4648.379, 1440.285)`                                                                  |
| North waterfront |         2.07 km | Right at `(3025.806, 5175.886)`                                                                  |
| West park        |         3.38 km | Right at `(-4639.486, 1180.067)`, then left at `(-3842.138, 1131.472)` and `(-3258.29, 1277.75)` |

The steering target moves 2.8 m to the specified side with a 22 m taper
on either side of each detour. The car advances through the game physics
at 120 steps per second. The tree collision geometry is still active.
Run the exact plan with
`node tools/sf-drive/audit-road-routes.mjs --plan=tools/sf-drive/road-audit-branch-east-routes.json`.
The plan SHA-256 is
`d53c3625514a7a2d31d28deab91ee1ba9abfaedaede3ae034083f569bf15757f`.

## MacArthur Tunnel on SR 1

The bake carries the full MacArthur contact strip in both owner tiles. The
13.4 m strip joins the highway at both ends. The two highway paint rows
stop at the portals and resume on the open approaches, leaving the
Presidio terrain visible above the bore.

The April 2023 USGS survey anchors the pavement at 65.82 m NAVD88 at the
south portal and 74.30 m at the north portal. The covered 395 m profile
interpolates between those approaches. The highest modelled grade on the
complete strip is 6.2%. [Gerardus MacArthur source review](https://github.com/banshee-data/gerardus/pull/107)

Chrome drove both directions through the bore and back onto the street.
The 20-course regression on the preview repeated all 20 completions and
52.45 km, with zero driven dips over 0.5 m and a largest seam of 6.1 cm.
The immutable bake has manifest SHA-256
`e3db8cbd5f858084738a7d77ab389c27a21b30518d16ab8699563f130d369b44`.

## SR 1 over Kobbe Avenue

Two deck strips with independently supported elevations carry the highway
across Kobbe Avenue north of MacArthur Tunnel. The game joins the strips
to the highway at both ends. The northbound and southbound strips span
about 246 m, with peak grades of 7.5% and 6.8%.

Chrome drove both directions continuously through the tunnel, along the
open segment and across the viaduct. The runs took 60.6 s northbound and
61.4 s southbound. A separate drive on Kobbe Avenue stayed below both
decks, at 56.48–57.84 m NAVD88 through the crossing. This pass used a
neighbourhood speed, peaking at 20.3 mph.

The immutable bake has manifest SHA-256
`9a9db992d0cc8d16719e1f140d15ff6f54f496c77e45a0244c3b9e92fab9eba3`.

## SR 1 over Ruckman Avenue

The 2023 3DEP transects show independent pavement bands above the valley.
The two 168 m strips use those bands for contact and join the existing
highway at either end. The northbound strip follows the mapped road
directly onto the US 101 connector. The largest grade on either strip
is 6.9%.

Chrome drove from the south approach through MacArthur, Kobbe and
Ruckman in 75.3 s. The reverse drive took 76.1 s. Both reached about
37 mph and exited onto exterior ground. On the lower road, an uphill
Storey Avenue drive stayed grounded at 35.92–37.93 m NAVD88. A downhill
pass briefly lifted at up to 22.5 mph and landed on the lower road.
The survey samples place the viaduct pavement around 44.3 m NAVD88 above
that crossing.

The immutable bake has manifest SHA-256
`829aefb5571c12ff6b2427e8765be866e680c192916ad82b23178213dc734d20`.

## Eastbound Bay Bridge and lower Yerba Buena bore

The named eastbound start takes the First Street on-ramp onto the lower
west span, through the lower Yerba Buena bore, and onto the east
approach. The drive uses the static game package. Its covered road
heights are inferred from the survey bands on the approach and both
sides of the west span. The lower bore starts at 43.35 m NAVD88 and
joins the east approach at 51.24 m. The inferred bore profile spans
286.5 m. [Caltrans tunnel assessment](https://catc.ca.gov/-/media/ctc-media/documents/programs/baseline-agreements/2025-10/04-2y780-baseline-agreement-new.pdf)

The fixed-step replay completed all six eastbound sections in 209.77 s.
Chrome completed the preview in 208.2 s, reached the lower span at
49.5 mph and entered the tunnel at 42.1 mph. The original westbound
replay still completed all nine sections through Fremont and Fourth
Street in 261.91 s. The named eastbound drive ends on the east approach
before the Oakland span.

The immutable bake has manifest SHA-256
`35cae286cdc658c4828417c71d8bb30ef6e94f1710e7f078fe43e9571c0acb9b`.

## Sixth Street I-280 and the 18th Street viaduct

The 6th–Brannan contact follows two independent I-280 carriageways. The
southbound road climbs from Sixth Street onto the existing mainline towards
Mariposa and the 18th Street exit. The northbound drive climbs the 18th Street
viaduct from the east, turns right onto the ramp, joins the Mariposa mainline
and reaches the Sixth Street surface. The 18th Street deck also supports
through traffic in both directions.

The depth-12 3DEP survey from April 2023 shows the 18th Street ramp junction
near 26.4 m NAVD88, about 7.7 m above the terrain lookup at that location.
The bake removes the ground paint only between the two viaduct handoffs.
The browser stayed near 11.1 m along Indiana Street beneath the viaduct and
between 2.98 and 3.25 m on Townsend Street beneath the two freeway decks.

The two Sixth Street contact sections are 669 m and 800 m long with maximum
longitudinal grades of 6.6% and 7.6%. The east approach to the 18th Street
viaduct reaches 12.3%. The game models the right turn within the measured
pavement. Its lane width and the interpolated transition are game geometry.

The same 20 guided emoji courses again collected both endpoints, covering
52.45 km with zero driven dips above 0.5 m and a maximum tile-seam step of
6.1 cm. The keyboard smoke test uses the right-turn lane at the 18th Street
fork. A centreline-only attempt continued west across the viaduct.

Run the browser checks with the exact tile revision and Chrome available.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  sixth-southbound sixth-northbound eighteenth-eastbound eighteenth-westbound
```

- Game manifest SHA-256 `a6347c623642489f6d134af040320ef1df74de4b3baa141ce79f12bb6cffe3bb`.
- Contact SHA-256 `9a6be35d2c85635675d6cf081c0de20a5018405617c7594e574d02d6cba2138a`.
- The bake changes two tile payloads and reuses the existing emoji placement
  revision, so saved pickups remain compatible.
- The upstream northbound mainline before the 18th Street merge is outside
  this fit. More elevated alignments remain provisional across the city.

## Essex Street and the tiled lower Bay Bridge

Essex Street and First Street now reach the same eastbound lower deck from
the normal game map. The named Essex course starts on the city ramp and joins
the existing lower West Span, Yerba Buena bore and east approach. The exit to
Oakland lies beyond this bake.

The 207 m Essex strip peaks at 6.1% grade. Above the covered merge the
upper sheet reaches about 41.1 m NAVD88, while measured side returns bracket
the lower game contact at about 32.3 m. The fit uses interpolation through
the occlusion. The driving line clears the mapped approach trees.

The first bake placed the First Street handoff at the Harrison junction.
The Fremont under-deck course picked up that contact while staying on
Harrison. Moving the handoff 8 m down the ramp let the same course collect
both emojis on its intended level. All 20 guided courses then passed over
52.45 km, with zero driven dips above 0.5 m and a maximum tile-seam step
of 6.1 cm. The named First Street start retains its original position.

The Chrome manual-input replay entered from ordinary Essex pavement,
stayed grounded through the lower bore and reached the east approach in
218.3 s. The named route completed in about 212 s. The westbound replay
still reached Fremont, the city streets and Fourth Street in 261.91 s.

Six tile payloads carry the lower route. Four carry the complete lower West
Span record and two carry the complete bore. The bake clips three previously
excluded bridge scenery records where their inferred volumes obstruct the deck
corridors. The city-mode game reads the road contacts from its static tiles.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  essex-tiled first-street-tiled
```

- Game manifest SHA-256 `072b63d8d7baf5596a583c3c1a50eb2666b4d4d5b361f9d6fdd2b83ac67e7f35`.
- Tiled contact SHA-256 `c5b8dfdc033fb76c2de29ab1f255e83aee03de44a487b5a8b2a11c546dcb8740`.

## Central Freeway and the I-80 interchange

The new contact joins the existing Fourth Street on-ramp to westbound I-80,
the US 101 connector and the Central Freeway. Drivers can return to city
pavement at the westbound exit near Fifth, 13th Street or Octavia. Octavia
and South Van Ness also have entries that join the eastbound I-80 approach.
The section ends on I-80 at the interchange. Citywide eastbound continuity
beyond that point still needs a verified connection.

The fit uses 364 depth-12 EPT nodes from the April 2023 3DEP survey. The
Central Freeway mainline peaks at 4.3% grade, the South Van Ness ramp at
7.8%, and the westbound freeway connector at 8.2%. Junctions share the
same endpoints at tile seams. The bake clears three overlapping inferred
scenery volumes from the contact corridor, including one owned by an
adjacent tile.

| Drive                                          | Fixed step | Chrome on the preview |
| ---------------------------------------------- | ---------: | --------------------: |
| Fourth Street to Octavia                       |    230.1 s |               203.5 s |
| Fourth Street to the westbound exit near Fifth |     81.2 s |                76.0 s |
| Fourth Street to the 13th Street exit          |    195.1 s |                     — |
| Octavia to eastbound I-80                      |    153.8 s |                     — |
| South Van Ness to eastbound I-80               |    132.9 s |               124.0 s |

The Chrome smoke tests send manual steering and throttle inputs. The first
centreline attempt took the Fifth exit instead of following the mainline.
The repeat selected the through lane, and local steering cues cleared the
mapped trees on the Octavia and South Van Ness approaches. All 20 guided
emoji courses again collected both endpoints, with 52.45 km driven, zero
dips deeper than 0.5 m and a maximum driven tile-seam step of 6.1 cm.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  central-from-fourth central-fifth-exit central-van-ness-entry \
  central-thirteenth-exit central-octavia-entry
```

- Game manifest SHA-256 `acce98f2f1ff8528e988cda1b2452e3b3819f8d390013855d9fe7b8edf90e19f`.
- Contact SHA-256 `4c535f963f6be0eb2a990925c6bf0e9b1bd53b560652695973cb0a6da4bd77f1`.
- The bake changes six tile files and keeps the existing emoji placement revision.
