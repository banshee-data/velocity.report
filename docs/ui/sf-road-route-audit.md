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

The browser smoke checks both directions on 23rd Street from open
street to the lower deck and back. It requires covered camera contact
beneath I-280, a grounded crossing and an exterior view after the chase
camera clears the last ramp. The two freeway runs above it must stay on
their own decks and finish grounded.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  dogpatch-23rd-lower-{eastbound,westbound} \
  i280-{northbound,southbound}-over-dogpatch-23rd
```

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

On the later preview, a centreline approach at the Mariposa fork took the
Fifth Street exit. The Sixth Street check now approaches the fork in the
right lane. A second drive from the same entrance takes the left lane and
checks the Fifth Street exit separately. Chrome reached street level on
both drives without losing ground contact. The Sixth Street handoff was
at 3.20 m and the Fifth Street handoff was at 3.01 m NAVD88.

Run the browser checks with the exact tile revision and Chrome available.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  sixth-southbound sixth-northbound eighteenth-eastbound eighteenth-westbound
```

With the later preview running locally, use `eighteenth-to-fifth-northbound`
for the additional branch.

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

## Central Freeway to the lower Bay Bridge

The eastbound bake closes the 1.93 km gap between the interchange and the
lower Bay Bridge approach. From ordinary South Van Ness pavement, the
through route climbs to eastbound I-80, follows the lower merge at Essex,
crosses the West Span, and goes through the lower Yerba Buena bore. It
ends on the east approach before the Oakland span.

The April 2023 survey supplies 249 depth-12 EPT nodes. Near Essex, the
lower contact rises from 27.2 m to 32.43 m NAVD88 beneath an upper deck
about 8 m higher. The lower fit interpolates 7.9 m between two observed
stations. The steepest longitudinal grade is 4.4% on the lower climb and
8.5% on the merge. The game preserves the provisional upper-deck geometry.

The fixed-step drive covered all 15 sections in 454.8 s, reached 44.5 mph
and stayed grounded. The manual-input guide changes lanes around three
mapped trees. All 20 frozen emoji courses passed again, covering 52.45 km
with zero dips deeper than 0.5 m and a maximum seam step of 6.1 cm.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  central-to-lower-bridge
```

- Game manifest SHA-256 `6a11518133065f83704de6962a6aad072b74ea3e371705a72da2c7e1a8e7e27a`.
- Contact SHA-256 `e2f4dbaf7672691dc8062481cb60e163e56a72e18cc31067fc2f6741bdfd0b19`.
- The bake changes four tile files and keeps the existing emoji placement revision.

## Upper Bay Bridge into the city

The ordinary city tiles carry the upper Yerba Buena bore, the West Span
and the Fremont exit. The new through branch continues past the Fremont
split to westbound I-80 and the Central Freeway. Its independent upper
band follows 220 depth-12 EPT nodes from the April 2023 survey. The
alignment moves laterally by up to 8.5 m to stay on the upper road as
it descends to the shared Fourth Street seam.

From the upper island approach, the fixed-step drive reached Fremont
in 232.8 s and reached Octavia via the through branch in 418.9 s. The
Chrome tests seed the existing surface at the island end and send
manual steering and throttle. Chrome reached Fremont in 237.7 s.
Two full Octavia attempts crashed near the Fourth Street merge. The
isolated merge drive and the next two full runs passed, reaching
Octavia in 396.9 s and 396.3 s. The crash cause remains unconfirmed.
All 20 frozen emoji courses collected both endpoints, with zero dips
deeper than 0.5 m and a maximum seam step of 6.1 cm.

The Fremont exit's right guard retards a slow collision. Chrome
stayed grounded in the right-side scrape. A separate drive crossed the
open left edge, recorded 0.7 s of airtime, and landed on city terrain
without a crash. The two edge checks started on the ramp.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  upper-to-fremont upper-to-octavia
```

- Game manifest SHA-256 `c55a80eaf89383fbc1515246897a7e7482be42c1bbc451680a1f118f05c91545`.
- Contact SHA-256 `7b3be0fb974041eb6519479f4cd973da9699f8c111f6afdfcf7a7354b69ad5ff`.
- The bake changes seven tile files and keeps the existing emoji placement revision.

## Dogpatch northbound I-280 and 20th Street

The Dogpatch east-side ramp now reaches northbound I-280, passes
under the 18th Street viaduct and joins the Mariposa contact to Sixth
Street. The new 1.36 km uses 113 depth-12 EPT nodes. At the crossing,
the highway contact sits near 20.4 m NAVD88 and the viaduct near 26.8 m.
In Chrome the drive began on ordinary city pavement and returned to
street level at Sixth and Brannan in 185.4 s, reaching 40.8 mph.

At 20th Street, the old terrain-sampled route reached 86.4% grade where
it crossed the steep embankment. The observed deck runs from 16.04 m to
35.85 m NAVD88 over 284 m. Its maximum longitudinal grade is 11.3%.
The bake trims the terrain paint under the deck and retains the nearby
parallel ground road. Chrome crossed the deck westbound in 26.2 s and
eastbound in 24.7 s, and stayed grounded at both street handoffs.

All 20 frozen emoji courses passed over 52.45 km after both changes,
with zero driven dips deeper than 0.5 m and a maximum driven tile-seam
step of 6.1 cm.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  dogpatch-to-sixth-northbound dogpatch-twentieth-westbound dogpatch-twentieth-eastbound
```

- Game manifest SHA-256 `10a57e879a516ba1934412adb05c6c9e23ea73c14a58952de08ac4311d29a9e2`.
- Contact SHA-256 `152f1c139b493e07732803866e25cceb778cd46f46398901e52148ca1f20af18`.
- The bake changes three tiles and preserves the existing emoji placement revision.

## I-280 northbound city exit near Fifth Street

The northbound Mariposa fork connects to both the existing Sixth Street
exit and a 655 m exit near Fifth. The latter descends from 15.79 m to
3.01 m NAVD88 at a maximum grade of 6.5%. The fit uses 97 depth-12 EPT
nodes. Where the upper road hides the lower contact near station 423 m,
the bake interpolates between the observed lower pavement on both sides.
The ramp retains one identity across its two tile owners.

Chrome drove from ordinary Dogpatch pavement through the fork and
returned to ground contact near Fifth in 170.0 s, reaching 41.3 mph.
The Sixth Street branch completed in 184.6 s, reaching 40.7 mph.
All 20 frozen emoji courses passed over 52.45 km with zero driven dips
deeper than 0.5 m and a maximum driven tile-seam step of 6.1 cm.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  dogpatch-to-fifth-northbound dogpatch-to-sixth-northbound
```

- Game manifest SHA-256 `23b03ed2abf7c9d57c1768d8ed936f718b5ca93460bee3c0c4fef1dd32d34d0e`.
- Contact SHA-256 `66943cdc2de0326e744eadb77565d96a0db3abcb2e7690195eac768b3a361cb1`.
- The bake changes two tiles and preserves the existing emoji placement revision.

## Alemany to Dogpatch on northbound I-280

Five contact sections add 3.90 km from the Alemany entry to the existing
Dogpatch northbound deck. The maximum added centreline grade is 5.2%,
and the entrance matches ground contact within 2 cm. At the shared
footprint by the covered viaduct, northbound contact is 21.77 m NAVD88
and southbound contact is 29.58 m. Where the upper deck hides the lane,
the lower grade follows an interpolation constrained by measured side
returns. The two decks keep independent contact.

The northbound approach into Dogpatch had eight inferred roof faces
overlapping the surveyed pavement. The bake clips those overlaps in
four pinned building rows, preserving the lower faces and previous
southbound clearance. The street trees stay in place. The scripted
driver uses lane offsets at six of the closer trees and through the
covered section.

At 120 Hz, the keyboard-equivalent regression followed nine sections
from ordinary Alemany pavement through Dogpatch. It returned to
ground contact at Sixth in 353.0 s and near Fifth in 360.2 s. All 20
frozen emoji courses also passed over 52.45 km, with no driven dips
deeper than 0.5 m and a maximum driven tile-seam step of 6.1 cm.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  alemany-to-sixth-northbound alemany-to-fifth-northbound
```

- Game manifest SHA-256 `c258108a7752d400af15734ebb15b5ec343077b304e722973fe1133cc92403a2`.
- Contact SHA-256 `46f19986668756c23a8fc4191c3c414e4b79a8ceb36e5069daffc0014570a6d5`.
- The bake changes six tiles and preserves the existing emoji placement revision.

## Northbound I-280 exit toward Cesar Chavez

The lower viaduct now has a separate 808 m exit toward Cesar Chavez.
The branch shares one pavement seam with the route into Dogpatch and
descends from 12.76 m to the street-level handoff at 4.54 m NAVD88.
Maximum grade is 5.6%. The fit uses a separate set of side and ground
returns where the other deck obscures the exit. The source near the
handoff contains a higher band at roughly 16.6 m. The lower lane
remains on its own surface before it returns to ordinary terrain.

A 15-node clearance survey confirmed the pavement where an inferred
roof face blocked the centre lane. The bake clips 8.6 m² within the
observed travel strip. The centre lane now clears that location with
no steering offset. At 120 Hz, the driver took 202.1 s to reach ground
from Alemany and completed the through branch to Sixth in 354.2 s.
Chrome drove the same Alemany entrance and exit in 228.6 s, reaching
40.5 mph and handing off to ground contact. All 20 frozen emoji
courses also passed with no driven dips deeper than 0.5 m and a
maximum driven tile-seam step of 6.1 cm.

The public Chrome round trip continued across 275 m of street
pavement to the Pennsylvania Avenue entrance. It reached the
southbound deck in 288.0 s. The fixed-step run reached the same deck
in 262.5 s and travelled 273.6 m between the two ground portals.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  alemany-to-cesar-chavez-northbound alemany-to-sixth-northbound \
  cesar-chavez-pennsylvania-roundtrip
```

- Game manifest SHA-256 `e88693c02191df20ec39fafa0dfc5d15dbce5602148af81a92011f2ab8255e78`.
- Contact SHA-256 `256ee13da415e75629311fd0a061a56500cbd04979146c82542cc01d86f3f101`.
- The bake changes five tiles and preserves the existing emoji placement revision.

## Alemany to US 101 northbound

Four contact sections add 2.77 km of alternate northbound pavement.
The turn from I-280 reaches US 101 and the Bayshore fork, where the
driver can continue north or descend to street level. The existing
I-280 branch stays connected at the Alemany seam. At the Bayshore
crossing, the through lane remains more than 7 m above the ground
layer. Maximum added centreline grade is 6.3% on the exit.

The main fit uses 251 depth-12 EPT nodes. An additional 19-node
survey verifies the merged pavement under two inferred roof faces.
The bake clips 130.4 m² across their shared seam. The other faces in
the building row retain their original geometry.

The fixed-step driver reached the Bayshore street handoff in 176.5 s
and the through pavement in 198.2 s. Chrome completed the exit in
175.7 s and the through drive in 199.6 s. The exit's first browser
attempt stopped where both branches shared the same lane. The repeat
stayed in the exit lane through the split. All 20 emoji courses
passed across 52.45 km with zero driven dips deeper than 0.5 m and a
maximum driven tile seam of 6.1 cm.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  alemany-to-us101-exit alemany-to-us101-through
```

- Game manifest SHA-256 `cc8ae31fc12ebf5281e9762a35ffee38b77761d1538400d8be3561ca1ce6160f`.
- Contact SHA-256 `f499a5ca7f62bbff6267dcd89cc080c5d0f9322f6f5bc308d61e7c6d53a12e84`.
- The bake changes four tiles and preserves the existing emoji placement revision.

## Northbound US 101 through Potrero to eastbound I-80

Four sections add 2.26 km from the Bayshore through lane to Potrero
and eastbound I-80. The Potrero exit returns to street level at
33.53 m NAVD88. The through route joins the previously verified
eastbound contact at 17.58 m. Both the Central Freeway and US 101
approaches reach that same I-80 surface. The nearby ground is
independent, around 10 m below the interchange fork.

The fit uses 219 pinned depth-12 EPT nodes. The maximum longitudinal
grade of the new sections is 5.4%. The fixed-step drive reached the
Potrero street handoff in 281.4 s. A second run followed all 19 sections
from ordinary Alemany pavement onto US 101, across the lower Bay Bridge
and through Yerba Buena, ending on the eastern approach in 687.6 s.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  alemany-to-potrero-exit alemany-to-i80-lower-bay
```

- Game manifest SHA-256 `5cc3b7b5845630b4342b02c6cbd67231d6a2623ce061d63ce1ae9e4788a99f04`.
- Contact SHA-256 `0f96a86ac97f2ae4903b4704a1c0235ee280f4fb5502c7bdd0a19d64c8f54024`.
- The bake changes four tiles and preserves the existing emoji placement revision.

## Northbound US 101 to Ninth Street and Octavia

Three new contact sections run 1.15 km from the US 101 and I-80 split
across the Central Freeway viaduct to two destinations. The Ninth Street
and Bryant exit drops to 6.262 m NAVD88 at city pavement. The merge
continues onto the existing Central Freeway deck at 16.07 m. The whole
viaduct is identical in both owning L13 tiles. A lower crossing keeps its
own level at about 14.4 m, around 9 m above the local ground.

The source fit pins 110 depth-12 EPT nodes. Two unsupported 4 m stations
at the lower crossing are bracketed by measured side returns. The new
viaduct peaks at 5.9% longitudinal grade and the Ninth Street ramp at 7.0%.
The 120 Hz drives from Alemany reached Ninth Street in 341.3 s and Octavia
in 429.3 s. Chrome reached ordinary pavement at Octavia from Alemany in
436.2 s. Separate Chrome drives from the US 101 approach reached Ninth
Street in 84.0 s and Octavia in 167.6 s. An earlier long approach to Ninth
Street hit a roadside tree on the Silver Avenue section. The shorter
verification starts on the supported US 101 approach. All 20 frozen
emoji routes passed again over 52.45 km, with zero dips deeper than 0.5 m
and a maximum driven tile-seam step of 6.1 cm.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  us101-to-ninth-bryant us101-to-octavia
```

- Game manifest SHA-256 `c63a335182fdbe343fed759232c8a1efec1dfe3e4569ee3083c95630ecf5470b`.
- Contact SHA-256 `1d6efdf91da34f3a2f8ba45ad802bb18659226e62ccbad73298111597aee1519`.
- The bake changes two tiles and preserves the existing emoji placement revision.

## SoMa ramps at Bryant, Fourth, Fifth and Harrison

Four directed ramps add 1.51 km of contact. The eastbound Fourth Street
exit reaches ordinary pavement at 4.10 m NAVD88. The Bryant and Fifth
Street entrances climb onto the existing eastbound game lanes. The
Harrison Street entrance joins the westbound deck. The highest fitted
longitudinal grade is 7.1%.

An 8 m ramp now blends into its existing 11 or 12 m merge cross-section
over 24 m. The optional `widthTransitionMetres` field gives the taper
length per terminal, while the existing seam still supplies the shared
width and bearing. The bake and browser check the width contract before
accepting the contact.

The fit pins 154 depth-12 EPT nodes. A false roof volume on the Fifth
Street entrance occupied the measured pavement. A separate 23-node
check found at least 47 road-level returns at each sampled lane point.
The tile edit trims 192.9 m² of inferred roof faces within that corridor.
It retains the original roof geometry and base outside the measured lane.

All four 120 Hz drives completed. Fourth Street reached the street in
52.6 s. Bryant, Fifth Street and Harrison reached their assigned decks
in 67.4 s, 78.2 s and 49.0 s respectively. Chrome finished the same four
drives grounded in 49.8 s, 67.3 s, 81.0 s and 49.4 s.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  i80-fourth-bryant-exit i80-bryant-entry i80-fifth-bryant-entry i80-harrison-entry
```

- Game manifest SHA-256 `6c3972a007c14ff422144d7a51fb2d2d70c60088600818d26ce8dbea52e9189a`.
- Contact SHA-256 `82435df883f9bc57e9961c19664867f98762478b72d798d0eb55ae8d87ef6e48`.
- The bake changes four tiles and preserves the existing emoji placement revision.

## Seventh Street, Harrison and southbound US 101

Nine directed sections add 3.00 km of contact. Eastbound I-80 can reach
both Seventh Street exits at 5.78 m and 6.33 m NAVD88. From the west exit,
Chrome drove 113.4 m of ordinary pavement before entering the Harrison
Street ramp and rejoining westbound I-80. The preview round trip finished
grounded in 111.5 s. A repeat drove 114.1 m on ordinary pavement and
finished in 112.0 s. The fixed-step drives reached both Seventh Street
exits in 45.4 s and 43.7 s.

The westbound I-80 split keeps its original Central Freeway connector
and adds a separate path to southbound US 101. From Harrison Street, the
new route goes through the Potrero interchange to two ordinary street
exits. The fixed-step drives reached Cesar Chavez at 2.45 m NAVD88 in
242.3 s and Potrero Avenue at 8.55 m NAVD88 in 244.4 s. The shared ramp
continues through the two intermediate forks with linked contact. Chrome
reached the same streets in 224.5 s and 226.3 s. At the
Cesar Chavez terminal the last 20 m blend a 0.39 m difference between the
observed lower lane and the base street elevation into the ground seam.
The Potrero Avenue branch has a 12.45% fitted peak through its actual
climb from the lower junction.

The two fits pin 265 and 42 depth-12 EPT nodes. Individual obscured
stations use supported neighbours or side returns at the same level.
The browser steering lines use clear lanes through the existing trees.
The 20 frozen emoji routes finished across 52.45 km with zero driven
dips deeper than 0.5 m and a largest tile-seam step of 6.1 cm.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  i80-seventh-west-exit i80-seventh-east-exit i80-seventh-harrison-roundtrip \
  harrison-to-cesar-chavez harrison-to-potrero-avenue
```

- Game manifest SHA-256 `f3d584a73c0886953cf413980c2e6d0363539213a8ceff6cca8b2e89577072b2`.
- Contact SHA-256 `0330a3578eef22038531bc1e3df422df4001d0556c95dc255f2764b9b2b92f82`.
- The bake changes three tiles and preserves the existing emoji placement revision.

## Southbound US 101 to I-280 at Alemany

Eleven directed sections add 5.30 km of contact from the US 101 through
lanes to southbound I-280 and Alemany Boulevard. The former terrain handoff
near Dogpatch now continues as measured road contact. Both freeway lanes
reach the same merge at 9.43 m NAVD88. The Alemany entrance blends from 9 m
into the shared 12 m cross-section, then continues beyond the 27.56 m merge.
The I-280 exit reaches ordinary pavement at Alemany and Ellsworth.

The contact fits pin 295 and 196 depth-12 EPT nodes. Side returns keep the
lower highway separate from the higher crossings. The highest fitted grade
is 7.76%. Both Chrome drives from US 101
reached the southbound I-280 deck beyond Alemany. The direct drive took
246.6 s. The round trip took 329.6 s and travelled 787.6 m along Alemany
Boulevard between the two ramps. The browser reported 708.8 m with ground
contact on that part of the drive. A separate I-280 drive reached the
Ellsworth exit in 65.4 s.

The same 20 emoji pairs finished over 52.43 km, with no driven dip deeper
than 0.5 m. The largest driven tile-seam step remained 6.1 cm. The revised
Fifth Street to Sweeny Street course exits at Ellsworth and uses the
Alemany overpass later in the drive. This course took 617.7 s at 120 Hz
and its peak driven grade was 17.48%. See the
[route fixture](../../tools/sf-drive/road-audit-alemany-routes.json).

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  us101-south-to-i280-through-alemany us101-alemany-i280-roundtrip \
  i280-south-ellsworth-exit
```

- Game manifest SHA-256 `7bbdb55535b00ac79ae210565235f57e2da69b64196603a336b849a253f5d9e0`.
- Contact SHA-256 `3a03b874d072e1964ac5b0750c80634fe919d1d71a08a6ccdc41c6825e09f2bf`.
- The bake changes seven tiles and preserves the existing emoji placement revision.

## Central Freeway and Tenth Street to southbound US 101

The Central Freeway fork keeps its eastbound I-80 branch and adds a
southbound branch to US 101. Tenth and Bryant provides a second entrance.
Both southbound approaches join at 15.18 m NAVD88 and continue through
the same contact strip to the Potrero mainline at 20.37 m. The start of
the Tenth Street ramp joins ordinary street pavement at 5.3541 m.

The three sections cover 934.3 m. The fit pins 104 depth-12 EPT survey
nodes. Four Tenth Street stations between 293 m and 306 m interpolate
between observed lower pavement returns. The maximum fitted grade is
6.68%. The merge is present in both owner tiles with identical points
and cross-sections.

The fixed-step drives from ordinary Octavia and Tenth Street pavement
reached Cesar Chavez in 294.4 s and Potrero Avenue in 210.0 s. The
existing path from Octavia to eastbound I-80 passed in 171.2 s.
Chrome drove the two new routes on the static build using keyboard
controls and reached ordinary pavement in 271.2 s and 207.3 s.

The 20 frozen emoji pairs also finished over 52.43 km. No driven dip
exceeded 0.5 m. The largest driven seam step was 6.1 cm. The steepest
driven sample was 26.18% on Ordway Street. The
[course set](../../tools/sf-drive/road-audit-alemany-routes.json) used
the same routes and speeds with its manifest SHA updated to this release.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  octavia-to-cesar-chavez tenth-bryant-to-potrero-avenue central-octavia-entry
```

- Game manifest SHA-256 `c9b648b5490a0e5a045983a4bb45af6afbb1a581620324526bcb632a5f1f3756`.
- Contact SHA-256 `f410a1d4c9da14d1f3dcff94aa8df5cdfbba3feac995b0424f74d062c59680e8`.
- The bake changes two tiles and preserves the emoji placement revision.

## Presidio Parkway mainline and SR 1

Two mainline sections add 1,004.8 m of contact at the western ends of the
Presidio viaducts. The northbound US 101 lane shares its incoming seam at
38.45 m NAVD88 with the SR 1 branch. The southbound mainline joins the
existing viaduct at 35.64 m. Both new reaches meet the base terrain at
their exterior ends.

The fit pins 145 depth-12 EPT nodes. The highest fitted grades are 4.52%
northbound and 4.64% southbound. The first northbound Chrome drive struck
an inferred trunk near station 508 m. At its mapped position, 226 pavement
returns and 68 other survey returns within one metre all lay at pavement
height. The correction removes that single source row from the baked
tile. The centre-lane retest passed.

Chrome completed northbound US 101 in 116.0 s, southbound US 101 in
118.4 s, and the original northbound SR 1 exit in 102.4 s. Both Main Post
and Battery registered as enclosed on each drive, and all three drives
ended grounded beyond the baked contact. Peak speeds were 51.7, 52.5 and
51.9 mph respectively.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  presidio-doyle-northbound presidio-doyle-southbound northbound
```

- Game manifest SHA-256 `9d1c25e18dafc7b5defae541256ec0d4ac10167340b01d03684c83e22be977db`.
- Contact SHA-256 `c22f57e740ec8bffe95fb3519be38a58f6506ddfb2faa6fed751c8a69a5ee839`.
- The bake changes two tiles and preserves the existing emoji placement revision.

## I-280 Monterey Boulevard and Ocean Avenue

Eight directed contact sections add 4.33 km. Southbound I-280 can reach
Monterey Boulevard or continue to the Ocean Avenue exit. The northbound
entrance from Monterey joins the existing I-280 mainline at Alemany.

The street inventory puts both Monterey mouths at one intersection
coordinate. At that position, the game found two valid contact surfaces
when approaching from the street and continued on the terrain. Fitting
each throat to its surveyed carriageway puts their terrain joins 8.95 m
apart. Both joins preserve pavement height, at 66.0016 m and 65.8981 m
NAVD88.

The fit pins 553 depth-12 EPT nodes. Side returns and bounded
interpolation keep the lower pavement continuous beneath the
northbound flyovers. The steepest fitted section is the 9.75% climb to
Ocean Avenue.

A fixed-step drive exited southbound I-280 at Monterey, drove 150.5 m on
city pavement, and rejoined northbound I-280. It reached Alemany in
243.1 s. The independent Monterey exit, Monterey entrance and Ocean
Avenue exit passed in 78.6 s, 146.2 s and 165.0 s.

Chrome repeated the Monterey round trip in 233.5 s, including 149.8 m
between the two ramps. Ground contact was reported for 137.8 m of that
street section, and the car finished grounded at Alemany. The Ocean
Avenue drive reached ordinary pavement in 155.0 s. Both runs began
110 m into the existing southbound deck and used keyboard controls.

The same 20 emoji routes covered 52.43 km and all finished. No driven
dip exceeded 0.5 m. The largest driven tile-seam step remained 6.1 cm.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  i280-monterey-roundtrip i280-ocean-exit
```

- Game manifest SHA-256 `36f7b3f8a32f4f14ca1f8b435a97cfcf6dea44d1ec8d0a701fe61a5a7514c7b8`.
- Contact SHA-256 `f6da3b2f61f2a5a6c64a26e55f51d53ec5cc8b2f2e0592790fed3bc46fdc298c`.
- The bake changes six tiles and preserves the existing emoji placement revision.

## Marina Boulevard to northbound Presidio Parkway

The Marina Boulevard entrance adds 258.2 m of measured contact. It
starts on city pavement at 3.00 m NAVD88 and joins the existing section
beyond Main Post at 2.61 m. The final 24 m taper from the 8 m ramp into
the shared 10.8 m cross-section. Main Post retains its existing join.

The fit uses 44 depth-12 3DEP nodes, with a peak grade of 6.11%. Chrome
drove from Marina Boulevard through Battery and split to northbound
US 101 in 113.0 s and SR 1 in 99.2 s. Both reached ordinary pavement
grounded. The fixed-step drives took 161.1 s and 147.1 s respectively.
All 20 frozen emoji routes finished again across 52.43 km.

A separate survey check sampled the Mariposa Street ramps every 4 m.
Within the expected lower pavement bands, 85 of 90 northbound exit
stations and 68 of 90 southbound entrance stations had at least 12
returns at an eligible lateral offset. The longest distances between
supported stations were 24 m and 60 m. Both exceed the contact exporter's
22 m interpolation limit, so those spans remain provisional.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  presidio-marina-to-doyle presidio-marina-to-sr1
```

- Game manifest SHA-256 `ec425a74f03e286d1fc876a27e54bcb813597f0c2cb76cba1d2b68c73a30c67e`.
- Contact SHA-256 `d90935aee8175c6c6dde4d9db56727d20803cf82635f82f089115c49d2edf5fd`.
- The bake changes one tile and preserves the existing emoji placement revision.

## Northbound I-280 exit to Mariposa Street

Half-metre sampling of the April 2023 3DEP survey found lower pavement
returns at stations 205 and 225 m. The samples bound the gap at 20 m within
the existing 22 m interpolation limit. The southbound entrance still has
a 55.5 m gap and remains provisional.

The 354.38 m ramp leaves the Dogpatch northbound section at 24.47 m NAVD88
and joins Mariposa Street at 11.36 m. Its maximum fitted grade is 6.98%.
The 8 m ramp tapers from the shared 12 m section over its first 24 m.
The existing mainline branches separately towards 18th Street.

Chrome drove from the east Dogpatch entrance to Mariposa Street in 97.3 s
using keyboard inputs in manual mode. At the street handoff the car was
grounded, travelled at 12.4 mph and had no selected elevated surface.
The separate fixed-step drive completed the same route in 99.0 s. The 20
frozen emoji routes again covered 52.43 km and reached all targets. The
largest driven tile seam remained 6.1 cm. No driven dip exceeded 0.5 m.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  i280-mariposa-exit dogpatch-to-sixth-northbound
```

- Game manifest SHA-256 `fc4b55cf6a0f297a9c36a2bf2d5a4268a99ba8282f313644d9121b150543e68a`.
- Contact SHA-256 `c47287e09f6c14fc2e5c2efb2d8326fbb236b8c423edda790c8e16866015a9b7`.
- The bake changes one tile and preserves the existing emoji placement revision.

## Southbound US 101 from Silver Avenue to San Bruno Avenue

This bake adds ten directed sections, including the Industrial Street
entrance, separate Silliman exit and entrance contacts, and the San Bruno
Avenue exit. It keeps the I-280 branch at Silver Avenue and carries the
mainline across four tile owners. The city ramps are 8 m wide. The
through contacts widen to 13 m. The four ramp tapers run for 24 m.

The fit used 254 depth-12 nodes from the April 2023 3DEP survey. The
highest fitted grade is 9.27% on the San Bruno exit. The lowest city
handoff is Industrial Street at 6.75 m NAVD88. Both Silliman contacts
meet street pavement at 24.23 m, with separate directions into the
intersection. The San Bruno exit reaches 38.13 m.

The fixed-step car took the Silliman exit, drove 442.0 m on Silliman
Street, Girard Street, Silver Avenue and San Bruno Avenue, re-entered
US 101, and exited at San Bruno in 271.5 s. The Chrome keyboard runs
separately confirmed the Industrial entrance to Silliman in 67.5 s,
the Silliman entrance to San Bruno in 86.7 s, and the Silver approach
to Silliman in 125.9 s. The through route reached the end of the bake
in 188.2 s. The steering line stays inside the contact
strip at Industrial Street where a baked building footprint intersects
the test centreline. The building evidence needs a separate review.

All 20 frozen emoji routes reached both targets over 52.43 km. The
largest driven tile seam was 6.1 cm. No driven dip exceeded 0.5 m.
The highest driven grade was 26.18% on Ordway Street.

A separate check of the westbound I-80 loop to Harrison and Fremont
found a 25.5 m shadow in the lower return band from station 349.0 m
to 374.5 m, even with sampling every 0.5 m. The loop stays provisional
under the 22 m limit.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  industrial-to-silliman silliman-roundtrip silver-through-san-bruno
```

- Game manifest SHA-256 `0258e1b19df5d4acded858f4b18169cf4ff1238e8501e380d4a12bbdb8d2b8f0`.
- Contact SHA-256 `62b4b84067dc2c069b9158e7c65802f0721d91f78dd3c489fd9cef92de9a669d`.
- The bake changes four tiles and preserves emoji placement.

## Northbound SR 1 at the Ruckman fork

The northbound Ruckman viaduct now branches at 45.34 m NAVD88. One
75.0 m branch joins the southbound approach through Battery and Main
Post. The other runs 74.15 m to the existing Golden Gate approach
pavement. The split reuses the original end points and keeps the
southbound viaduct on its own contact. The fit uses 57 depth-12 3DEP
nodes. The peak grade across the three fitted sections is 6.72%.

Chrome drove from Kobbe Avenue to the northbound pavement handoff in
34.1 s. At the fork the second run took the southbound branch, passed
through both tunnel bores and reached ordinary street pavement in
157.7 s. Both runs crossed the gap between Kobbe and Ruckman on
grounded contact. The browser measured 47.0 m and 48.6 m respectively.
The fixed-step runs crossed 49.5 m of pavement between the viaducts.
The existing MacArthur route reached the northbound pavement in 75.6 s.
All 20 frozen emoji routes finished across 52.43 km, with a maximum
driven seam step of 6.1 cm and no driven dip over 0.5 m.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  sr1-kobbe-to-golden-gate sr1-kobbe-to-presidio
```

- Game manifest SHA-256 `dd498a4034ee906396e28d1ac1a0f5aa016be5c7b93901df822e90419e6af997`.
- Contact SHA-256 `4a7dd5528a174e318658f5e36cf6971a824c92746c7059661abd7f9643fa2519`.
- The bake changes one tile and preserves emoji placement.

## Westbound I-80 loop to Harrison and Fremont

The 504.57 m loop now leaves the westbound deck at 36.98 m NAVD88
and joins Harrison and Fremont at 21.89 m. Its peak fitted grade is
5.06%. The through branch keeps the same seam. At the stacked crossing,
the game samples 25.12 m on the loop, 33.61 m on the lower Bay Bridge
approach and 42.55 m on the upper approach.

The earlier 25.5 m observational gap narrowed to 21.5 m by using a
classified lower edge at station 348.5 m and fifteen non-withheld
unclassified returns near 24.57 m at station 370.0 m. The fit keeps
the 22 m limit. The two provisional ribbons sampled above the loop
came from the independent bridge approaches, whose contacts remain
in the bake.

Chrome took the new loop to city pavement in 55.6 s. The separate
through drive used the westbound deck and remained grounded.
The fixed-step replay reached the street in 57.2 s.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  upper-to-harrison-loop westbound-harrison-fork-through
```

- Game manifest SHA-256 `e44bc8ed81329168785076ec3d1d31774e2cd8fcaa8c3779531e33e36a8aaec9`.
- Contact SHA-256 `1cc4c318469b98446cb76612b491e91a5c92c834025fd61b8679bd920b9ac474`.
- The bake changes one tile and preserves emoji placement.

## Potrero Avenue and Cesar Chavez Street entrances to US 101

The Potrero Avenue and Cesar Chavez Street entrances join one elevated
ramp at 14.65 m NAVD88. The terrain beneath that merge is 9.11 m. Their
two street handoffs sit at 12.25 m and 9.35 m respectively. The common
167.91 m ramp enters the southern freeway at 15.69 m and crosses the
tile boundary as the same contact strip.

The fit uses 66 depth-12 April 2023 3DEP nodes. The peak fitted grade
is 8.39% on the Cesar Chavez entrance. The fixed-step drive reached the
southbound freeway in 58.3 s from Potrero and 41.8 s from Cesar Chavez.
Chrome reached the same freeway in 55.5 s and 39.7 s respectively.
Both cars stayed on the elevated merge before joining the mainline
and finished grounded. All 20 frozen emoji routes covered 52.43 km,
with a maximum driven seam step of 6.1 cm and no dip over 0.5 m.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  potrero-avenue-to-us101-south cesar-chavez-street-to-us101-south
```

- Game manifest SHA-256 `2afc0061690b36a5417d43c32b1c2f6a780687b204502c6a408a054205fcc12f`.
- Contact SHA-256 `150b0111e5f424ceb6896698456ed0992aaadb64f874c7a4a6310e5322b1605b`.
- The bake changes two tiles and preserves emoji placement.

## Southbound US 101 at Hester Avenue

The bake extends the Paul Avenue mainline to the Hester fork at 38.17 m
NAVD88. The collector divides at 44.14 m. One branch reaches Bayshore
Boulevard and Hester Avenue at 47.11 m. The other rejoins the mainline
at 38.39 m. Both routes to the southern city boundary share one
843.50 m strip across two tiles and meet the existing pavement at
10.72 m.

The fit uses 313 depth-12 April 2023 3DEP nodes and reaches a peak
grade of 9.42% on the open mainline. Lower shoulder returns bound a
17 m interval on the mainline and a 16 m interval on the collector.
The collector fit adjusts those outer returns by the measured 6%
crossfall. The bake trims two inferred solids within the lane and
raises the base of a third solid there to leave clearance beneath
observed higher pavement.

| Drive                      | Fixed step |  Chrome | End                                  |
| -------------------------- | ---------: | ------: | ------------------------------------ |
| Mainline from Paul Avenue  |    94.53 s |  92.2 s | Southern boundary pavement           |
| Collector to Hester Avenue |    59.73 s |  57.5 s | Bayshore Boulevard and Hester Avenue |
| Collector back onto US 101 |   110.72 s | 106.2 s | Southern boundary pavement           |

Chrome used keyboard input in manual mode with route assist disabled.
The fixed-step drives stayed grounded throughout. Chrome verified
grounded contact on each new section and at the pavement handoffs. The two entrances from the Hester street
network remain under review. The survey confirms two height bands
where they pass under higher pavement and leaves a gap in the lower
band. All 20 frozen emoji routes finished across 52.43 km. The largest
driven seam step was 6.1 cm and the driven traces had no dips over 0.5 m.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  us101-south-hester-mainline us101-south-hester-exit us101-south-hester-collector
```

- Game manifest SHA-256 `fa05b0da2b47348d7fba9386a7be86f792e43a32d6b10cefd3804b4e249c024d`.
- Contact SHA-256 `0aaf70acd3960c5c85ed1e4ff3fbbe149a5d7b225e4a53ff400264a0e24cf095`.
- Survey EPT node SHA-256 `b337922e28e56b179d4b4c2e19a55e3c15a18165cac0247e0e83b081b4d9f7ac`.
- The bake changes three tiles and preserves emoji placement.

## Southbound US 101 exit to northbound Bayshore Boulevard

The northbound Bayshore turn leaves the existing Potrero exit approach at
15.84 m NAVD88. The branch reaches 13.43 m at the split and meets the
northbound city pavement at 6.20 m. The survey and street inventory fit
352 m of contact through the two new sections. The steepest fitted grade
is 9.51%.

The April 2023 survey includes 46 classified depth-12 nodes and 17
depth-15 nodes in the lower pavement crop. The lower shoulders bracket a
30 m indirect section beneath the freeway. The game interpolates that
section, keeping the two upper freeway contacts at their separate
elevations. Two inferred tree trunks lay in the driving lane. The
survey shows lower pavement and separate high returns in those columns,
so the bake drops the two offending game rows.

The default chase camera begins to lower on the approach to the covered
interval and keeps the trailing view below the deck until the camera
and car both clear it. With route assist disabled, Chrome drove the
lower exit and continued onto city pavement in 50.5 s. It observed
grounded contact at the handoff and restored the ordinary camera on
the street. A regression test projects the camera sightline through
both upper decks along the exit. The independent southbound drive on the deck above
passed in 21.3 s and kept its exterior camera. The fixed-step lower drive
stayed grounded for 46.5 s. All 20 frozen emoji routes finished across
52.43 km with a largest driven seam step of 6.1 cm and no driven dips over
0.5 m.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  us101-south-bayshore-north-turn us101-south-over-bayshore-exit
```

- Game manifest SHA-256 `58a310838dfff50c673be3dd503712282ba485338ad9e487157e1ecffb1b278c`.
- Contact SHA-256 `53d021b92300c1d32e49d56efb6df071c66e674a4947ae239a617472fc90cc67`.
- The bake changes one tile and preserves emoji placement.

## Southbound US 101 exit to southbound Bayshore Boulevard

The southbound Bayshore turn takes the other branch of the same exit
at 13.43 m NAVD88. It crosses below the freeway and follows a 60 m
section of city street before handing off to regular pavement at
about 6.8 m. The fitted contact reaches a peak grade of 8.71%. Lower
pavement observations at stations 214.00 m and 242.78 m bound the
28.78 m inferred section in the survey shadow.

The original eight-metre terrain stood 1.75 m above the fitted road
near the centre of the crossing. A one-metre refinement keeps the
original grid and the elevations at the refinement boundary. The
largest remaining terrain overlap along the centreline is 0.13 m.
The refinement and contact represent inferred game geometry across
the aerial shadow. Forty classified nodes and 26 finer survey nodes
support the fit around the crossing.

A fixed-step drive took the southbound turn and reached city pavement
in 53.1 s. Chrome drove the route with the game’s route assist disabled
and regained the exterior camera on the city street in 58.2 s.
All 20 frozen emoji routes finished across 52.43 km. The largest driven
seam step was 6.1 cm, with no driven dips over 0.5 m.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  us101-south-bayshore-south-turn us101-south-bayshore-north-turn us101-south-over-bayshore-exit
```

- Game manifest SHA-256 `cda5d8bf606b92e51c36f40d486c47d126e5ccc518edd3157b1fbd677eaca972`.
- Contact SHA-256 `f8bc6ecd44262d161f66d48b9ef2062a829c7d42dab0b438576eef4657ffe87c`.
- The bake changes one tile and preserves emoji placement.

## Westbound I-80 exit to Fifth and Harrison

The Fifth Street exit now branches from the upper westbound carriageway
at 19.02 m NAVD88. It runs below the continuing freeway and reaches
Fifth and Harrison Streets at 3.38 m. The fitted strip is 515.97 m
long and reaches 5.50% grade. Both tiles carry the same complete strip
across their shared boundary.

The fit uses 114 classified survey nodes and 26 finer nodes around the
underpass. Lower pavement at stations 418 and 434 m bounds a 16 m
inferred stretch. The two roads differ by about 6.2 to 6.5 m where the
exit passes underneath. A separate 16-node crop supports the open
through lane. The bake trims a previously inferred solid inside that
lane while preserving its exterior.

Chrome drove the lower branch to Harrison Street in 48.3 s. The chase
view stayed low through the crossing and switched back on the way out.
Chrome drove the independent through carriageway in 38.2 s with the
exterior view. Both runs finished grounded with the game’s route assist
disabled. The fixed-step lower drive stayed grounded for 49.7 s. All
20 frozen emoji routes completed across 52.43 km, with a largest driven
seam step of 6.1 cm and no driven dips over 0.5 m.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  i80-westbound-fifth-harrison i80-westbound-over-fifth
```

- Game manifest SHA-256 `95e34b1d79f62f80679d67e7de4db960c954283208f41af326061189ef14005d`.
- Contact SHA-256 `9aaa7d474db6e8f5dfe512edfb86561f083662166bc3c7cec1e99760ad077c22`.
- The bake changes two tiles and preserves emoji placement.

## Alemany from northbound I-280 to southbound US 101

The new connector climbs from the existing Alemany fork at 14.99 m
NAVD88 to 38.86 m. The descent rejoins southbound US 101 at the Paul
Avenue approach at 21.94 m. The two sections cover 1.74 km and reach
7.1% grade. Each owning tile carries the complete contact through its
boundary. The I-280 through branch remains available at the same fork.

Lower shoulder returns constrain the short section under the first
deck. A 13-node clearance crop found open pavement where the game had
placed a trunk in the lane. The bake removes that one inferred tree.
Chrome reached the Paul Avenue approach in 143.2 s and cleared the
separate northbound fork in 29.3 s. Both drives finished grounded.
A centre-lane fixed-step drive reached Paul Avenue in 166.0 s.
All 20 frozen emoji routes finished
across 52.43 km. The largest driven seam step was 6.1 cm and the
driven traces had no dips over 0.5 m.

The separate citywide screen on the preceding Fifth and Harrison
release still flagged 451 ground-draped intervals above 50% grade
and 51 missing intervals. Those figures describe terrain lookup
along street rows, including local terrain under elevated roads.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  alemany-to-southbound-us101 alemany-northbound-through-fork
```

- Game manifest SHA-256 `4d67c35c0b2884125f40bae54718257a03e9a06b914315c843c89318f5f30af1`.
- Contact SHA-256 `f29bad47859811b7dc7fce6b16f7f38796e3119ba7a5603ac16ee21d35e093bf`.
- The bake changes three tiles and preserves emoji placement.

## Marin Street and Cesar Chavez Street to northbound US 101

The new Marin Street and Cesar Chavez Street entrances climb from
6.65 m and 7.25 m NAVD88 to the same junction at 17.14 m. A shared
ramp connects them to the Potrero approach at 20.07 m. The three
sections cover 742.83 m. The highest fitted grade is 7.34%.

The classified aerial survey supports the ramp heights. At the
shared crossing, the fit uses exposed lower shoulder returns and
applies the measured crossfall. Five inferred trunks were removed
where separate survey crops identified pavement and clearance.
The drive from Cesar Chavez uses the open side of its entrance lane
around an obstruction whose lower returns remain ambiguous.

Chrome drove from Marin to US 101 in 47.4 s and from Cesar Chavez
in 35.0 s on the exact bake. Both runs stayed grounded on the
shared ramp and reached the Potrero approach. The 20 frozen emoji
replays all finished over 52.43 km. The maximum driven seam step
was 6.1 cm, and no driven dip exceeded 0.5 m.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  marin-to-northbound-us101 cesar-chavez-to-northbound-us101
```

- Game manifest SHA-256 `227768ae526cb2b8d7fc57e725e5a2076ecb14eacdcd570c2a5309556a5bb28f`.
- Contact SHA-256 `df69a792b18b91dcc7ead69e3812d162ff0a644644c5496375926e9d9edf838f`.
- The bake changes one tile and preserves emoji placement.

## Mariposa Street beneath I-280 and the southbound entrance

The 353.59 m southbound entrance connects Mariposa Street at
12.97 m NAVD88 to the existing Dogpatch approach at 24.40 m.
The fitted strip reaches 6.22% grade. It runs below the existing
18th Street exit and viaduct.

The fit uses exposed lower shoulders. Stations 112 to 152 m on
the ramp and stations 12 to 52 m on the eastern street stretch
are inferred between measured returns. The one-metre terrain
detail joins the existing coarse-grid boundary and removes the
false slope through the lower street. Along the driven crossing,
the maximum sampled terrain grade fell from 53.4% to 10.1%.
A separate crop supports removing one inferred trunk from the
travel lane.

Chrome drove the west-side turn onto southbound I-280 and drove
straight under the viaduct on the same tile. The straight replay
stayed at the lower street level between 11.33 and 12.98 m. All
20 emoji courses finished over 52.43 km. The maximum driven seam
step was 6.1 cm and no driven dip exceeded 0.5 m.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  mariposa-west-to-southbound-i280 mariposa-upper-18th-viaduct i280-mariposa-exit
```

- Game manifest SHA-256 `bad1f9e8d08b7af1412cc795971a3b0290260f8b97c57a414857c0c588f67347`.
- Contact SHA-256 `210ac5a5150892a7d31eda7fba29eb51fe649c43bb0844113e018cf210b05df6`.
- The bake changes one tile and preserves emoji placement.

### Street grade and camera follow-up

The live through drive revealed a brief hop near the eastern end at about
15 mph. The change in grade peaked at 2.8 percentage points between
consecutive one-metre samples. The revised fit uses a continuous slope
through the edges of the surveyed ramp shadow. Over the same street
interval, the maximum grade fell from 10.1% to 8.2% and the largest change
between samples fell to 1.1 percentage points. The western pavement
also blends into the terrain over the observed approach. The test uses
the game's ground contact solver and axle spacing. It permits at most
two 120 Hz steps within 1 mm of the pavement.

The default chase camera now queries elevated pavement along its path on
the street and lowers beneath the deck. During the private Chrome crossing
the car remained grounded from start to finish, with the street and car
visible under the deck at 16 mph.

- Game manifest SHA-256 `45c8fe622fe1043a39813d59dac254a0c0c4d6cd0e275708e28068307dd287e2`.
- Contact SHA-256 `74d6fddf88106627e63c5b629ee86e6afd641132792d91c178cdc765be25c4a0`.

## Southern northbound US 101 and the Alemany entrance

The bake joins approximately 3.72 km of northbound US 101 from the
southern game boundary to the existing Silver Avenue approach. It adds a
309.7 m entrance from Alemany Boulevard climbing from 6.02 m to 13.65 m
NAVD88 and a 117.9 m shared merge. The entrance reaches 9.1% grade and the
new mainline reaches 5.2%. Shared endpoints have identical coordinates and
elevations on both sides of every join.

The first Chrome run crashed at an inferred building volume that filled the
lower highway at Hester. The Arrow crop identified the upper pavement
between 48 and 50 m and lower roadway returns on the exposed shoulder.
The follow-up keeps the upper surface and gives its inferred underside
clearance over the lower lanes. The central lower interval under the
deck interpolates between observed pavement on either side.

On the revised bake, Chrome drove all seven sections from the city edge
through Hester to Silver in 200 s, reached 49.6 mph, and finished grounded.
The Alemany entrance drive on the same bake reached Silver in 51.1 s. The 20 frozen
emoji courses finished over 52.43 km. The maximum driven seam step was
6.1 cm and no driven dip exceeded 0.5 m.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  southern-city-edge-to-northbound-us101 alemany-street-to-northbound-us101
```

- Game manifest SHA-256 `7496a0cc1eec79c5bd0f24d3e507ebb47af8481012691d182339caa9cf49c0d0`.
- Contact SHA-256 `a1fc582ad0ccc4cd9774714634de671fa57803f5933c0079f3b00b9ccbe64cb7`.
- Six tiles changed. Each tile intersecting a new road carries the complete strip.

### Silliman exit and Alemany return

The northbound Silliman exit adds a 219.6 m fork to Bayshore Boulevard.
It runs below the upper pavement before handing off at 22.28 m NAVD88.
The survey fit interpolates 20 m of lower contact between observations
on either side of that shadow.

Chrome drove off US 101 at Silliman, across the existing Silver Avenue
bridge, along San Bruno Avenue, and back onto the highway at Alemany.
The continuous drive took 180.6 s, with 558.4 m in ground contact on the
local streets. Chrome reached the Silver highway approach grounded.
The standalone exit took 30.4 s and ended on Bayshore pavement.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  northbound-us101-silliman-to-bayshore northbound-us101-silliman-alemany-loop
```

- Game manifest SHA-256 `607c547132d5f7680d446268dc93a67db93dcbcf034f0656e0cc2c4c25a983c8`.
- Contact SHA-256 `b4d9f136ccaa8570629c39540c6f357c6cef418400bd0d61c301c95d7420c307`.

### Northbound US 101 to southbound I-280

The lower connector adds 1.64 km of contact from the Paul Avenue fork to
southbound I-280. It uses 405 stations from the pinned survey and interpolates
six 8–12 m intervals between measured bounds. Baked absolute grade peaks at
6.9%, with a 95th percentile of 5.8%. Three tiles carry the complete contact
strip. At two stacked crossings, the game samples the upper pavement 7.5–7.6 m
above the lower connector at the same horizontal coordinates.

Chrome drove the full connector and merged onto southbound I-280. Separate
drives stayed on the upper connector and continued north on US 101 at the fork.
At a later crossing the first drive held lower contact while the upper deck
hid the car. The revised chase camera uses the selected road height for its
overhead probes. Chrome kept the car visible and grounded on the lower road
and retained the exterior view on the upper road.

The 20 frozen emoji courses finished over 52.43 km in 83.8 minutes of
simulation. Five routes used highways. Three used baked tunnel contact,
including both Stockton directions and the toll plaza eastbound. Across
50 driven joins, the maximum seam step was 6.1 cm. Route sampling flagged
seven depressions on the southbound I-280 course, up to 0.82 m at Alemany
Boulevard. The driven traces recorded zero dips greater than 0.5 m. The
highest driven grade was 26.2% on Ordway Street.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  northbound-us101-to-southbound-i280 alemany-upper-over-northbound-connector \
  alemany-late-lower-crossings-camera northbound-us101-through-alemany-fork
```

- Game manifest SHA-256 `f19e6efca4c6389c0df1a86fea8eb61795b5810fc24b1cf898b777ee73caef9d`.
- Contact SHA-256 `4c3029a0e9492df076e1abda7e797087b0c203b78249863a6fade74774969ef4`.

### Bayshore Boulevard entrance at Hester

Bayshore Boulevard runs over lower pavement beside Hester. The base terrain
is over 7 m below the observed upper street at the crossing. One tile adds
162.2 m of upper approach, 31.9 m of through street and the 388.7 m ramp to
northbound US 101. The ramp begins at 46.18 m NAVD88 and joins the highway
at 39.86 m. The upper approach peaks at 3.7% grade and the ramp at 3.6%.

Chrome entered from Bayshore pavement and stayed grounded to the northbound
mainline in 52.4 s. The through drive took 22.6 s and returned to Bayshore
pavement. A collision scan across four nearby tiles found no obstacles at the
baked centreline stations.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  bayshore-hester-entry-us101 bayshore-hester-street-through
```

- Game manifest SHA-256 `2ae88d9c134d47550d35a783cfb13067e1e70c8e43d5673fe33c5e59a656cb13`.
- Contact SHA-256 `aeb2071d9a4dbacd0ac62d9b6d74754e32099234458c11b37718376f7b3a0d35`.

### Third Street and Salinas entrances at Hester

One tile adds 275.7 m of Third Street ramp, 265.0 m of Salinas ramp and
192.0 m of shared collector. The two entrances lead to both the southbound
US 101 merge and the Bayshore exit. The lower junction is inferred at
34.20 m NAVD88 from visible bounds on both entrances and beyond the upper
decks. The longest interval between visible bounds is approximately 73 m.
At one shared horizontal position the northbound pavement is 8.7 m above
the Third Street ramp. The largest baked grade is 7.9% on that ramp.

Chrome approached on Third Street, turned on 31.9 m of city pavement and
reached southbound US 101 in 87.9 s. The Salinas drive followed the same
lower collector back to Bayshore pavement in 67.2 s. The chase camera
lowered under the upper deck on both drives and kept the exterior view
on the northbound freeway above them.

The fixed-step driver collected the Meade and Arleta emojis over 2.12 km
in 245.4 s.
The harness now uses the game's baked deck clearance when it checks trees.
The 20 frozen courses also finished over 52.43 km, with a largest driven
seam step of 6.1 cm and zero driven dips greater than 0.5 m.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  hester-third-street-to-southbound-us101 hester-salinas-to-bayshore \
  hester-upper-northbound-over-entrances
node tools/sf-drive/audit-road-routes.mjs --plan=tools/sf-drive/road-audit-hester-south-entrances.json
```

- Game manifest SHA-256 `fc5addac6d395951d1b9b22d5eb7bbc16b9d5178da2a62e149c3eaaf9b121dcc`.
- Contact SHA-256 `dac34f71c92183e2ecee256ef159ea4079c9f0126e1f4688272f36f9b42b0c78`.

### Northbound US 101 exits at Hester

The northbound approach now branches onto a 256.7 m shared exit. The
225.9 m Bayshore branch reaches pavement near Williams Avenue. The
208.7 m Third Street branch follows the lower surveyed lane and meets
city pavement about 72 m beyond the original intersection alignment. At
that alignment the surveyed street above it is about 4.2 m higher.
The largest baked grade is 9.3% on the Third Street branch.

The bake adjusts the terrain within the lower ramp corridor and removes
six inferred trunks where the survey indicates pavement or clear space.
Two were on the southbound Third Street re-entry and one was on the
northbound mainline at the fork. The largest residual terrain protrusion
is 0.38 m at one fork sample. A collision scan along the ramps and at
lateral offsets of 0.75 m returned zero hits across four tiles.

Chrome used the game's steering inputs to drive to Third Street in
47.5 s, reach Bayshore Boulevard in 46.6 s, and stay on northbound
US 101 through the fork in 14.3 s. Both exits handed over to grounded
city pavement. A continuous drive exited to Third Street, turned on
city streets and reached southbound US 101 in 151 s. The browser logged
202.9 m between ramp contacts, with 172.8 m logged as grounded at both
sample endpoints. It finished grounded without a crash.

All 20 frozen courses collected both emojis over 52.43 km, including
6.50 km of highway centreline. Their largest driven seam step was 6.1 cm
across 50 crossings. No driven dip exceeded 0.5 m. The median of the
course p95 grades was 9.5%. The steepest driven sample was 26.2% on
Ordway Street in the Bayview course.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  northbound-hester-exit-to-third-street northbound-hester-exit-to-bayshore \
  northbound-hester-stays-on-us101 hester-third-street-exit-and-reentry
```

- Game manifest SHA-256 `61dad03f1bfc517a73b9e3dc773db675563033016f70ae5622ab2a7c38bcbccf`.
- Contact SHA-256 `de316cc5cef51e5c582be75acea8db0d385b5079dc1dba1db05bf5516b20f542`.

## West Yerba Buena ramps at Treasure Island Road

The eastbound exit takes the lower Bay Bridge deck at 45.56 m NAVD88 to
Treasure Island Road. The westbound entrance takes the island road back
onto the upper deck at 55.03 m. The two pavement handoffs are about
14 m apart, and the mainline connections through both levels remain
open. Two tiles include the contact, terrain and scenery changes.

The April 2023 survey shows the upper ramp directly. The lower ramp
uses two inferred intervals beneath the upper deck, extending 64 m from
the split and from station 244 m to 272 m. The terrain refinement follows
both ramp levels within the island. A survey check cleared two inferred
building volumes and two inferred trunks from the pavement. The corrected
collision scan returned no hits along either centreline or at lateral
offsets up to 2.65 m across four nearby tiles. The upper ramp cleared all
sampled terrain across that width. One lateral sample at the lower road's
ground handoff retained 0.26 m of terrain above the contact surface. The survey predates the
West Side Bridges Retrofit. [SFCTA project record](https://www.sfcta.org/westsidebridges)

| Browser drive                     | Elapsed | Finish                                                        |
| --------------------------------- | ------: | ------------------------------------------------------------- |
| Eastbound exit                    |  44.9 s | Grounded on island pavement                                   |
| Westbound entrance                |  50.2 s | Grounded on the upper Bay Bridge deck                         |
| Lower mainline through the split  |  16.6 s | Grounded beneath the upper deck                               |
| Continuous island turn and return | 118.3 s | Grounded on the upper deck after reversing on island pavement |

The 20 frozen emoji routes collected both targets across 52.43 km.
Their largest driven seam step was 6.1 cm across 50 crossings, with no
driven dip greater than 0.5 m. The unchanged route definitions are in
`tools/sf-drive/road-audit-alemany-routes.json`. Set `manifestSha256` to
the revision below before replaying.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  island-west-eastbound-exit island-west-westbound-entry \
  island-west-eastbound-through island-west-exit-and-reentry
```

- Game manifest SHA-256 `d5b7a53795a748fb7a69e5473233da3e343d3410f49bf59f7a73b0ceddfe3e8f`.
- Contact SHA-256 `ca2e756da459ff0e802399f6373f68000c08f1adfed1f1972d34bca637340902`.

## Cross-city emoji routes and the east Yerba Buena entrance

The 20-course set in `tools/sf-drive/road-audit-cross-city-routes.json`
varies both ends of each drive across the city. It crosses Stockton and
Broadway, the Presidio toll underpass, I-280, US 101, city roads under I-80,
and neighbourhood grades from the ocean to Bernal Heights and Treasure
Island. Each fixed-step run starts at an emoji and records a finish when
it collects both. The driver uses normal car physics at 120 Hz with
23 mph street and 40 mph highway targets. Short turns can drive slower.

The route audit samples the planned path and the actual wheel-supported
trace at one-metre intervals. A depression is the distance below the
mean of points four metres either side of a sample. The screen counts
samples deeper than 0.5 m. A sequence of neighbouring samples can mark
one dip. Its tile-boundary measure is the largest height change in a
single simulation step crossing ownership, which includes the road's grade.
The browser checks use confirmation of manual takeover, keyboard events,
and saved emoji pickups. Browser traces can miss a short excursion
between rendered frames.

The first fixed-step pass used the published west-island manifest
`d5b7a53795a748fb7a69e5473233da3e343d3410f49bf59f7a73b0ceddfe3e8f`.
The routes planned 68.31 km and drove 38.96 km before collection or a stop.
The car drove 830 m through tunnel contact and 1,958 m over other baked
road levels. Eight drives collected both emojis. Among the remaining
drives, four stalled, six hit scenery, and two reached the end of an
unexpected baked road branch. The largest change at 37 crossed tile
boundaries was 8.2 cm. The median course p95 grade was 9.2%. The planned
paths flagged 19 dip samples and 71 samples without contact. The portions
actually driven included zero dip samples deeper than 0.5 m.

| Course                         | Fixed-step result                                            | Driven km | Peak grade |
| ------------------------------ | ------------------------------------------------------------ | --------: | ---------: |
| Chinatown north via Stockton   | Stalled on Columbus                                          |      2.64 |       9.0% |
| Chinatown south via Stockton   | Building on Minna                                            |      2.78 |      39.4% |
| Broadway to the Marina         | Building on Bartol before the tunnel                         |      0.57 |      31.8% |
| Presidio toll underpass        | Both emojis                                                  |      3.44 |      21.4% |
| Mission Bay to Pennsylvania    | Reached end of I-280 contact after taking the through branch |      2.18 |       8.8% |
| Mission Bay to Alemany         | Tree on Terry A Francois                                     |      0.44 |       1.2% |
| Silver Avenue bridge           | Both emojis                                                  |      3.39 |      17.8% |
| Hester to Bayshore             | Both emojis                                                  |      3.19 |      28.4% |
| Dogpatch to Potrero            | Reached end of Pennsylvania contact                          |      1.25 |      53.7% |
| Embarcadero to SoMa            | Stalled on Main                                              |      0.15 |       2.9% |
| Lake Street to the Presidio    | Stalled on Liggett                                           |      4.62 |      17.5% |
| Lands End to Outer Richmond    | Both emojis                                                  |      2.43 |      17.5% |
| Richmond to Golden Gate Park   | Tree on John F Kennedy                                       |      1.11 |      10.0% |
| Upper Haight to Inner Sunset   | Both emojis                                                  |      1.92 |       5.3% |
| Ocean Beach to Fort Funston    | Tree on Skyline                                              |      1.39 |       5.2% |
| Lake Merced to Ingleside       | Stalled on Font                                              |      1.10 |       4.7% |
| West Portal to Sunnyside       | Both emojis                                                  |      2.18 |      14.5% |
| Twin Peaks to Bernal           | Tree on Twin Peaks                                           |      0.30 |      18.7% |
| Excelsior to Visitacion Valley | Both emojis                                                  |      2.45 |      21.2% |
| Treasure Island north          | Both emojis                                                  |      1.42 |       7.5% |

Chrome collected both emojis on the northbound Stockton corridor in
377 s, on the toll underpass corridor in 355 s, and on Treasure Island
in 144 s. The southbound Stockton drive passed the tunnel and hit a
building on Minna. Separate Chrome drives on the public game crossed
both Presidio bores in both directions and completed the west-island
exit and re-entry in 117.8 s. The Mission Bay to Alemany keyboard run
collected both emojis over 9.49 km in 823 s. It took the southbound
I-280 exits and the existing Alemany overpass, with no driven dip deeper
than 0.5 m.

### Evans Avenue

The worst driven grade was 53.7% on Evans Avenue, where the coarse
terrain was 6.7 m below the observed pavement. A 102 m correction uses
survey returns in the centre and both outer tracks. At the centre of the
former dip, the corrected pavement stays near 13.9 m NAVD88. Chrome drove
101.8 m without airborne frames and measured a peak grade of 6.8% over
five metres. The Dogpatch course's largest driven grade fell to 21.3%,
now on 25th Street. The later Pennsylvania join still stops its fixed-step
drive. Seven planned dip samples remain on the route after the Evans fix.

### The east Yerba Buena entrance

The new westbound entrance runs from Macalla Road to the upper bore. Both
westbound routes use the same contact beyond the merge. The April 2023
survey supports the exposed pavement. The final 63 m under rock use an
inferred curve between the observed entrance and the upper-bore contact.
The named 50 mph Bay Bridge start uses the same split through the tunnel.
The tunnel walls open where the two bores overlap. A mesh ray test
checks that the eye line is clear along both approaches and that the
outer walls remain outside the merge.
Chrome reached the west approach on the new entrance in 40.6 s and on
the continuing upper mainline in 26.9 s. Both finished grounded in the
exterior view. The other east-island ramp connections remain under review.

- Game manifest SHA-256 `e1812dbb4ec28ac5b630de86de90d89472d8d76d7ffc3d79cda9f5696411ea9a`.
- Contact SHA-256 `881d997593a743f758413a56fd9acfc6267b03a0ac19a4df960017f2bd378f61`.

Use the manifest hash of the tested revision in a copy of the plan. Pass
`--export-courses` to prepare the same waypoints and speed targets for
Chrome and keep its trace outside Git.

```sh
node tools/sf-drive/audit-road-routes.mjs --plan=/tmp/road-audit-plan.json
node tools/sf-drive/audit-road-routes.mjs --plan=/tmp/road-audit-plan.json \
  --export-courses >/tmp/road-audit-courses.ndjson
SF_BASE_URL=https://velocity.report node tools/sf-drive/drive-road-audit.cjs \
  /tmp/road-audit-courses.ndjson
```

## 23rd Street over US 101 in Potrero

The cross-city course flagged a second drop on 23rd Street. At world
position (3264.1, −610.8), the coarse terrain was 21.7 m NAVD88 and the
observed 23rd Street pavement was 29.1 m. The cut contains both US 101
carriageways. The new 71 m contact strip follows the upper pavement and
joins city streets at both ends. The lower carriageways keep their
existing contact. At sampled vehicle positions beside the crossing, the
road levels are 7.53 m apart northbound and 6.75 m apart southbound. The
largest grade on the new strip is 6.9%.

| Chrome drive      | Elapsed | Finish                                |
| ----------------- | ------: | ------------------------------------- |
| 23rd St westbound |  19.2 s | Grounded on city pavement beyond deck |
| 23rd St eastbound |  21.2 s | Grounded on city pavement beyond deck |
| US 101 northbound |   6.7 s | Grounded on the lower carriageway     |
| US 101 southbound |  10.3 s | Grounded on the lower carriageway     |

The ground-following path also flagged a dip near Alemany Boulevard.
The Mission Bay to Alemany Chrome run used the existing upper road there
and recorded no dip deeper than 0.5 m. The lower terrain sample explains
why the planned path alone can flag a dip at a stacked crossing.

The browser audit now records the locations of the largest driven grade
and dip. A separate baseline Dogpatch drive recorded a 0.90 m depression
at Evans Avenue, near (3777.5, −1177.3), before that terrain correction.
The course runner checks the manifest Chrome loaded at the start of each
route so a site deployment does not silently change a batch's revision.

- Game manifest SHA-256 `e06d3eb6ef17c50ecbb6a0eb7725dd1416c5c776828e4a2ed6d75f71151cc3e6`.
- Contact SHA-256 `5cbc34edf34f2bca11d040daa88945f3282ae49b768eac7aac0ed732d93adbe4`.
- Aerial EPT node receipt `940858ef1d8ff2204e098b10c55cc85c0644611f5caf5f2ff13c8659ffee5b1a`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  potrero-23rd-westbound potrero-23rd-eastbound \
  potrero-23rd-under-northbound potrero-23rd-under-southbound
```

## Lower eastbound Yerba Buena exit

The lower eastbound branch now leaves the lower tunnel for Hillcrest Road.
The game contact uses 56 m of inferred alignment inside the survey shadow,
followed by returns from the lower ramp. It follows the measured lane past
the inventory endpoint to the first pavement handoff with matching elevation.
The handoff is at 48.30 m NAVD88. The 70 m exit bore and 144 m exposed ramp
preserve the lower level alongside the upper I-80 carriageway. Their
largest absolute grades are 9.85% and 6.45% respectively.

The lower through route retains both upper clearance and full continuity
through the split. The first tunnel and its east continuation share a
single seam. The wall openings follow the overlap between the diverging
contact strips. Both First Street and Essex use the same split in their
named drives.

Chrome took the lower exit and drove onwards on Hillcrest Road in 38.4 s.
The independent lower-deck through check finished in 15.6 s. Both finished
grounded. The simulation and mesh tests check the two legal joins, clear
eyelines and ground at the Hillcrest handoff.

The citywide ground screen on the 23rd Street release sampled 1,190,791
supported intervals and found 445 over 50% grade. There were 450 on the
immediately preceding release. Fifty-one intervals on both revisions had
missing ground. This screen follows the ground grid and can flag a normal
change at a stacked crossing. The driven route and the selected baked
surface establish which level needs correction.

- Game manifest SHA-256 `64a72513acc46128fc1ebf4e7265ed9e86a53f363aed488293bf01a8db73ebba`.
- Contact SHA-256 `2bd80f52352d2493e85d19c8a1681ae1efd525d3afb0888e3093d522dc382dc2`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  island-east-eastbound-exit island-east-lower-mainline
```

## Twenty cross-city drives in Chrome

The [cross-city course set](../../tools/sf-drive/road-audit-cross-city-routes.json)
was driven with the site's manual keyboard controls and saved emoji pickups.
Chrome attempted all 20 pairs, covered **49.12 km in 84.2 minutes of driving**
and collected both endpoints on 12 courses. Four stalled, three left the test
line and one crashed. The table gives one outcome per itinerary. The Broadway,
Dogpatch and Hester rows use repeat drives after the first bake. Targeted ramp
checks and incomplete runs interrupted by a site deployment are outside the
campaign totals.

The median course 95th-percentile longitudinal grade was **9.2%**. The
largest observed one-metre step was 50.6% on the southbound Stockton
itinerary, which completed the tunnel and reached Minna before the crash.
That grade covers the whole drive. The peak position was unavailable in
that recording. The fitted Stockton bore peaks at 4.7% on the fixed-step
check. Grade samples cover intervals when both surrounding
browser frames reported ground contact.

| Emoji course                   | Endpoints | Bake | Driven km | Grade p95 / peak | Browser outcome         |
| ------------------------------ | --------- | ---- | --------: | ---------------: | ----------------------- |
| Chinatown north via Stockton   | 🏙️ → 🍫   | A    |      3.73 |      6.5% / 8.8% | Both collected          |
| Chinatown south via Stockton   | 🛤️ → 🎭   | A    |      2.80 |    15.1% / 50.6% | Crash after tunnel      |
| Broadway to the Marina         | ⚓ → 🎟️   | B    |      0.54 |    21.4% / 22.6% | Stalled before tunnel   |
| Presidio toll underpass        | 🚪 → 🌁   | A    |      3.45 |     9.5% / 21.9% | Both collected          |
| Mission Bay to Pennsylvania    | ⚾ → 🚂   | A    |      0.02 |      1.9% / 2.5% | Left course             |
| Mission Bay to Alemany         | 🏀 → 🌺   | A    |      9.49 |     6.6% / 17.5% | Both collected          |
| Silver Avenue to Hunters Point | 🧰 → 🌺   | A    |      3.39 |     9.0% / 17.5% | Both collected          |
| Hester to Bayshore             | 🚸 → 🚊   | B    |      3.21 |     9.6% / 27.0% | Both collected          |
| Dogpatch to Potrero            | 🚚 → 🧁   | B    |      1.60 |    17.7% / 21.2% | Left course after Evans |
| Embarcadero to SoMa            | 🌉 → 🎭   | A    |      0.17 |      2.9% / 3.0% | Stalled                 |
| Lake Street to the Presidio    | 🚲 → 🥾   | A    |      0.02 |      0.8% / 1.2% | Left course             |
| Lands End to Outer Richmond    | 🖼️ → 🔭   | A    |      2.44 |     9.8% / 17.5% | Both collected          |
| Richmond to Golden Gate Park   | 🎬 → 🦬   | B    |      2.27 |     7.5% / 10.0% | Both collected          |
| Upper Haight to Inner Sunset   | ☮️ → ☕   | B    |      1.93 |      3.8% / 5.2% | Both collected          |
| Ocean Beach to Fort Funston    | 🐚 → 🪂   | B    |      0.69 |      4.0% / 5.2% | Stalled                 |
| Lake Merced to Ingleside       | 🛶 → 🧸   | C    |      3.46 |    17.5% / 23.8% | Both collected          |
| West Portal to Sunnyside       | 🚇 → 🎨   | C    |      1.88 |    12.2% / 14.0% | Stalled                 |
| Twin Peaks to Bernal           | 📷 → 📚   | C    |      4.16 |    19.8% / 27.8% | Both collected          |
| Excelsior to Visitacion Valley | 🗺️ → 🐕   | D    |      2.46 |    11.9% / 21.2% | Both collected          |
| Treasure Island north          | 🧭 → 🪁   | A    |      1.43 |      2.4% / 7.9% | Both collected          |

The table's traces record zero dips deeper than 0.5 m relative to an
eight-metre chord. The prior Dogpatch repeat measured a 0.90 m dip on Evans
Avenue. After correction, the browser cleared that part of the itinerary
without a dip over the threshold. The Mission Bay to Alemany course used
the southbound I-280 mainline, an exit and the existing Alemany overpass.
The Hester course used the southbound US 101 entrance and Bayshore exit.
The northbound Stockton and Presidio toll courses each passed their
baked tunnel. The Broadway approach remains a separate city-street repro.

The parallel 120 Hz runs on bake C covered 38.96 km, collected both emojis
on eight courses and flagged eight planned dip samples. Six were on the
planned Alemany path and two on the Dogpatch path. The portions actually
driven had zero dips over the same threshold. The different completion
counts reflect differences in steering, contact selection and game traffic.
A stall or course departure calls for a local reproduction before it can
establish a pavement defect. Browser sampling also leaves shorter events
between rendered frames unresolved.

| Bake | Static game manifest SHA-256                                       | Release                                                                                       |
| ---- | ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------- |
| A    | `d5b7a53795a748fb7a69e5473233da3e343d3410f49bf59f7a73b0ceddfe3e8f` | [West Yerba Buena ramps](https://github.com/banshee-data/velocity.report/pull/1192)           |
| B    | `e1812dbb4ec28ac5b630de86de90d89472d8d76d7ffc3d79cda9f5696411ea9a` | [East entrance and Evans Avenue](https://github.com/banshee-data/velocity.report/pull/1194)   |
| C    | `e06d3eb6ef17c50ecbb6a0eb7725dd1416c5c776828e4a2ed6d75f71151cc3e6` | [23rd Street over US 101](https://github.com/banshee-data/velocity.report/pull/1195)          |
| D    | `64a72513acc46128fc1ebf4e7265ed9e86a53f363aed488293bf01a8db73ebba` | [Lower eastbound Yerba Buena exit](https://github.com/banshee-data/velocity.report/pull/1196) |

Chrome 157.0.8103.80 checked the manifest loaded at the start of each
itinerary and recorded no requests to Arrow-worker. The tooling commands
above reproduce a course when its matching static game bundle is served.

## Hillcrest eastbound entrance

The DataSF `14214000` entrance climbs from Hillcrest pavement at 48.63 m
NAVD88 to the lower eastbound deck at 52.40 m. The 171 m strip follows
April 2023 survey returns. Interpolation covers source stations 92–112 m
in the survey shadow. The peak grade is 4.51%. The game uses the upper
road footprint for the covered camera view while the car stays on the
lower pavement.

The lower through drive and the entrance share the same forward deck
seam. Both named eastbound starts use this join. The game regression
checks the city pavement handoff, the shared deck seam, the grade and
more than 7 m of separation beneath the upper road. On the canonical
package, Chrome reached the deck from Hillcrest in 26.6 s and drove
through from the lower tunnel in 25.5 s. Both finished grounded. A
continuous run exited the lower bore, turned on Hillcrest and merged
back onto eastbound I-80 in 67.9 s. It recorded 36.3 m of grounded
travel during the 47.4 m city-street connection and finished grounded
on the shared deck.

- Game manifest SHA-256 `f50b36f76a7f4a4b0d2c6fb15a309b5fc3aee0775ff4652007c87012a2b05453`.
- Contact SHA-256 `724e07c4c529ed4b480d15be570b2cbf8a6173af746d2ce2a0d838aad60bc235`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  island-east-eastbound-entry island-east-lower-mainline \
  island-east-eastbound-exit island-east-roundtrip
```

A separate retry from Mission Bay used the corrected starting heading
on the preceding lower-exit revision. Chrome collected both emojis
in 262.2 s after 2.96 km, using the southbound I-280 entrance, the
18th Street split and the Pennsylvania Avenue exit. The largest driven
grade was 13.1% and the trace had zero dips deeper than 0.5 m. A
Lake Street to Presidio retry then covered 4.56 km, with 16.4% peak
grade and zero dips over the same threshold. It stalled at
(−1034.6, 4052.6), near the destination.

## Broadway lane scenery

A direct westbound run selected the lower Broadway surface and hit an
inferred trunk near (1887, 4032) on the west approach at about 55 m
NAVD88. A footprint sweep found five interfering inferred trunk rows
at four locations across the two lanes. The April 2023 survey showed
lower pavement at those locations and zero nearby returns in classes
3–6. The two-tile correction removes the five matching rows. The bores,
upper building footprints and adjacent scenery retain their existing
geometry.

The contact regression failed on the prior bake at the collision
footprint of one of the inferred trunks. Chrome repeated the westbound
run to exterior pavement in 54.4 s, then drove the eastbound lane in
59.4 s. Both finished grounded. The previous westbound attempt crashed
after 48.8 s. These were directed bore tests. The Broadway-to-Marina
emoji route stalled before reaching the bore.

- Game manifest SHA-256 `e31fcb0e984e4ba0b3d39f736aa16d3385f47c86c364264ef24240a55ef120ab`.
- Contact SHA-256 `8627bc2e65573c2c1d658729d450929d7c44054dcaf8000d5d78cd79a2452aa9`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  broadway-westbound broadway-eastbound
```

## Westbound East Span exit to Macalla Road

The westbound East Span now has a 143 m supported lead-in before its
Yerba Buena fork. The existing westbound selector starts there at
50 mph. Continuing on I-80 takes the upper bore to the West Span.
Taking the fork uses the 312 m ramp to Macalla Road. The ramp peaks at
7.77% grade and hands off to ground at 54.76 m NAVD88. The two forward
choices share one seam at 60.50 m. The active exit is shown in the
[transportation authority's interchange map](https://www.sfcta.org/projects/i-80-east-side-ramps-project).

The first rolling drive on the longer lead-in collided with an inferred
building volume at 59.4 m. The survey tracks pavement near 59.5 m and
separate overhead bands at 66–70 m. A local clearance keeps the high
face with a game underside of 63.8 m and leaves the outside footprints
in place. The contact regression reproduced the old building strike
and passed after the clearance bake. Chrome repeated the exit from a
49.9 mph rolling start, reached Macalla in 48.9 s and finished grounded.

The normal named start originally exposed only its automatic through
route to the manual driver. The manual junction graph now also offers
baked transitions where both levels and endpoint coordinates agree
with the authored road. Chrome selected the ordinary westbound start,
took manual control at about 50 mph, used the ramp and finished on
Macalla Road in 48.4 s.

- Game manifest SHA-256 `44c066ef6d38704c26e4278c1b06da5d49627d9c717100d481fb2e26bdcddccd`.
- Contact SHA-256 `a42dad8f53fb42f40c58306088e3b4cd83186c8e9ae21783b87be70e28c5b38b`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  island-east-westbound-exit island-east-westbound-through-exit
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-yerba-buena.cjs
```

On the published Broadway clearance release, two further Chrome emoji
routes used the two bores and collected both endpoints. The eastbound
route from Broadway and Hyde to Grant and Green covered 1.55 km. The
westbound route from North Point and Kearny to Lombard and Scott covered
4.75 km. Neither trace flagged a dip deeper than 0.5 m. The same public
release reopened the Bay Bridge and Embarcadero offline, saved a new
emoji pickup and restored it with all three airtime records on another
offline reload.

## Minna Street beneath the Chronicle building bridge

The southbound Stockton course crossed the tunnel before crashing on
Minna Street near Fifth. The planning record documents a street passage
under the connection between the Chronicle and Examiner buildings.
The April 2023 survey measures the opening approaches at 8.22 m and
8.78 m NAVD88 and a roof near 23.6 m over the passage. The roof obscures
the middle of the street from the aerial survey.
[5M design for development](https://default.sfplanning.org/devagreements/Draft_5M_D4D_07.07.2015.pdf)

The bake restores the passage across two inferred building volumes in
one tile. It retains the roofs and side walls and uses an inferred
underside at 12.3 m NAVD88. The existing game pavement along the
centreline ranges from 8.22 m to 8.92 m, reaches 4.2% grade at its
steepest one-metre step and has a maximum eight-metre chord departure
of 5.9 cm below the chord. These are samples of the game pavement.
The actual underside height and pavement within the survey shadow
remain unmeasured.

The collision regression failed against the earlier bake at the Fifth
Street entrance. It now checks 219 ground poses across three lines
through the opening, including contact with the roofs above and
buildings beside the lane. The scheduled SoMa survey drives identify
their scenery exclusions by footprint so the added building rows
preserve the existing route past Fifth and Howard.

- Game manifest SHA-256 `705747ae7596eb487cad7f8744af8d9627feb5fb8504ffa50e74d0e20feb56c4`.
- Contact SHA-256 `dacfc7e00883cb32a76bf25457520ed8a3240abc92d839bfdb249bbbeb0c690e`.

Chrome repeated Chinatown south via Stockton using the same two emoji
targets and traced 3.11 km in 328.6 s. It completed the Stockton bore and
the Minna passage, collected both emojis, and finished grounded. The
highest driven sampled grade was 38.9% near (1898, 4021), well before
Minna. The route had no sampled dip over 0.5 m. Including the earlier
Mission Bay retry, 14 of the original 20 emoji pairs have now finished
at least once.

## Macalla Road loop onto westbound I-80

The signed westbound entrance loops from Macalla Road onto the open
upper deck. The new 316.27 m contact follows that loop at the level
documented by SFCTA. The previously baked path follows the separate
street-inventory alignment below the loop. The two approaches shown
here, the loop and the upper through deck, now share one full-width
seam at 57.46 m NAVD88. [SFCTA east-side ramp map](https://www.sfcta.org/projects/i-80-east-side-ramps-project)

The approach meets Macalla Road at 53.66 m NAVD88 and peaks at 9.04%
grade. A cross-section scan found no game collisions at 477 poses
spanning the centreline and 2.25 m to either side. The largest sampled
terrain protrusion was 0.18 m. The horizontal reference comes from
OpenStreetMap contributors and the elevations come from the April 2023
USGS 3DEP survey. [OpenStreetMap data licence](https://www.openstreetmap.org/copyright)

Chrome drove 31.9 m on the approach to the loop, with 24.2 m recorded
as grounded at both sample endpoints. It merged, passed through both
upper bore sections and reached the West Span grounded in 57.0 s. The
separate through drive finished in 26.5 s. From the regular 50 mph
start, a single manual drive exited at Macalla, made a three-point turn
on the street, re-entered westbound I-80 and reached the West Span in
127.1 s. Chrome recorded 62.6 m between the ramps, with 57.4 m grounded
at both sample endpoints. The automatic named start reached Fremont and
returned to westbound I‑80 at Fourth.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  macalla-to-westbound-i80 macalla-westbound-roundtrip \
  island-east-upper-mainline island-east-westbound-exit
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-yerba-buena.cjs
```

- Game manifest SHA-256 `492001a86938d762ed9fa4184e86c7d5722cba37f19c230c02c69106fff38fe2`.
- Contact SHA-256 `3282852cb38e38683da188445b5216cc1e515021c8daa78f3704d8a62dc15693`.
- Guide SHA-256 `eaff6101e3f54c0534f46e8a85a0054bd3760a16b5a09961e5cc6984cb2e7c72`.
- The bake changes one L13 tile and retains the emoji placement revision.

## Golden Gate Bridge between the Presidio and Marin

The previous game stopped its playable road near the south approach. At
(-3284.545, 5511.667), the car selected ground at 27.427 m NAVD88, 34.688 m
below the observed bridge contact. Three L13 cells were also missing along
the crossing.

The new bake adds a connected contact surface from the Doyle mainline to
Vista Point in both directions. It streams the bridge and the three new
cells from the game's static tiles. The original 137 emoji targets and
their placement hash are preserved. The three new cells have no
signposted intersection. [Golden Gate carriageway survey and bake](https://github.com/banshee-data/gerardus/blob/f942c46bb0e27858be1fe6cc56bd8cf04ce288a6/docs/roads/SF-PRESIDIO-DOYLE-MAINLINE-CONTACT.md#golden-gate-bridge-to-marin)

| Carriageway | Fit including approaches | Maximum grade | 95th percentile grade | Maximum local depression |
| ----------- | ------------------------ | ------------- | --------------------- | ------------------------ |
| Northbound  | 3,349.10 m               | 3.25%         | 3.03%                 | 4.6 cm                   |
| Southbound  | 3,424.99 m               | 4.49%         | 2.98%                 | 4.4 cm                   |

The grade uses consecutive two-metre contact samples. The depression is
the difference between the pavement and the midpoint of the elevation
chord ten metres to each side. No fitted sample falls more than 5 cm
below that chord. The rendered strip is 9.4 m wide on each carriageway.
The model uses a fixed three-lane allocation in each direction. The
bridge operator changes the lane allocation in service. [Golden Gate Bridge lane management](https://www.goldengate.org/bridge/bridge-operations/traffic-management/)

The initial Chrome drives proved both lanes had inferred scenery that
overlapped the pavement. The correction clears 10,173 poses at the
centreline and 2.9 m to each side. It keeps the original Toll Plaza roof
over the southbound lane. Two pavement stations under that roof were
interpolated from adjacent survey returns.

The keyboard smoke drives use the regular manual takeover and assert
grounded contact through all six sections in each direction. The
northbound drive finishes grounded at Vista Point in 195.6 s. The
southbound drive continues onto the Doyle approach in 183.4 s. Both
drives reached approximately 52 mph.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  golden-gate-presidio-to-marin golden-gate-marin-to-presidio
```

- Game manifest SHA-256 `97122290264b0e0ef3d6e9e68b2a019e8c17ad0d09f9618b8ed43eab2c8d8bd5`.
- Contact SHA-256 `6582c5fb1b21db39514f8cbbc06f97285bd3e9f580b7acf7d911c6a710d446d6`.
- Source road revision `sf-city-roads-08b5bb95deebcba7b7d9ed745f230eb1e753fad73169487edee67c66b11b516a`.

## Main Street beneath the Bay Bridge

The Embarcadero-to-SoMa run stalled at (4469.063, 3031.251) on Main Street.
The car stayed at 4.83 m NAVD88. An inferred volume reached from the overhead
bridge to the ground and blocked the traffic lanes. The street continues
between Bryant and Harrison beside the bridge footings. [SFMTA Main Street striping plan](https://www.sfmta.com/sites/default/files/reports-and-documents/2023/02/item_12_tasc_packet_main_st_combined.pdf)

The correction raises the inferred volume only within the lane corridor.
At the crossing the lower and upper bridge surfaces remain at approximately
41.39 m and 50.18 m NAVD88. The fixed-step collision scan cleared all 237
street poses across three lines. The old scene collided at 88 of those
poses. The new scene still detects the inferred pier-side volumes outside
the corridor. [Gerardus survey provenance](https://github.com/banshee-data/gerardus/blob/733ce31394e9511b7d0d71ce864bbc55e39ebff9/docs/roads/SF-ESSEX-LOWER-BAY-BRIDGE-CONTACT.md#main-street-beneath-the-city-approach)

Chrome completed both 78 m passes at street level, with no crash or
switch to a bridge deck. The longer Embarcadero-to-SoMa replay cleared
Main Street and continued to Harrison and Fourth. It travelled 1.44 km
with a 7.84% grade p95, an 8.87% peak and no sampled dip over 0.5 m. It
stopped at the Fourth Street entrance after selecting the entrance
surface while following the street course.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-main-street.cjs
```

- Game manifest SHA-256 `65317b86333c38ce22ef685372e629071af9407a2b7a28dcfa4d1ba4643e2db9`.
- Contact SHA-256 `b3aa6a3b99df652fddd86eba1ab07a45dd897be92c9c8426f262795568b6791d`.
- One L13 tile changed. The emoji placement revision was preserved.

## Harrison Street past the Fourth Street entrance

The first Main Street retry exposed a second failure at Fourth and
Harrison. The car followed Harrison as intended until it crossed the
mouth of the Fourth Street entrance. The physics selected the ramp
within the shared low pavement, then stopped when the street-bound
heading ran out of the ramp strip.

The ramp portal now checks that the current direction of travel reaches
the ramp contact within five metres of the mouth. The check is capped at
half the strip length for shorter ramps. A driver aligned with the
entrance still acquires it at the same seam. A driver continuing along
Harrison stays at street level. The test uses the bundled Fourth Street
strip for both moves.

The 120 Hz Embarcadero-to-SoMa replay reached both emojis over 2.44 km.
It recorded a 7.07% grade p95, a 9.74% peak and no tracked dip over 0.5 m.
The sampled profile covers the driven route, including the Harrison
overpass and its approaches. Chrome then collected both emojis over
2.44 km in 247.8 seconds. It recorded a grade p95 of 7.38%, a peak of
8.86%, 200 m with airborne contact and no tracked dip over 0.5 m. It
spent no time blocked. A separate Chrome run deliberately took Fourth,
merged onto westbound I-80 and reached the street at Fifth in 78.2
seconds.

The 20-course fixed-step repeat on the same tiles covered 41.33 km.
Nine courses collected both emojis, three stalled, six stopped at
scenery impacts and two reached the end of contact. The median course
grade p95 was 9.15%. The maximum driven grade was 39.37% on the Chinatown
south itinerary. Eight planned samples exceeded the 0.5 m dip threshold,
and the driven portions recorded zero. The incomplete courses still
need local browser reproductions.

## Richland and Highland over the Bernal Cut

The previous terrain followed the San Jose Avenue cut below each bridge.
Near the crossing centres, it sat 13.64 m below the Richland pavement
and 10.52 m below Highland. The revised bake adds two 7.2 m driving
surfaces with ground portals at both ends. The road paint follows the
open approaches and the two decks. One inferred trunk on the Highland
lane was cleared against the survey. [San Francisco Public Works](https://sfpublicworks.org/project/richland-avenue-bridge-traffic-railing-project) · [Gerardus survey provenance](https://github.com/banshee-data/gerardus/blob/dfa1859f89235a65c87cad427cce2389362d82d4/docs/roads/SF-CITY-ROAD-LAYERS.md#richland-and-highland-over-the-bernal-cut)

| Bridge   | Fit with approaches | Peak fitted grade | Eastbound | Westbound |
| -------- | ------------------- | ----------------- | --------- | --------- |
| Richland | 86.00 m             | 3.90%             | 11.5 s    | 14.3 s    |
| Highland | 80.98 m             | 3.64%             | 12.5 s    | 12.2 s    |

The times are from Chrome manual keyboard runs. Each drive acquired
its deck, crossed it and returned to ordinary ground. Chrome also
drove 313.1 m southbound and 326.2 m northbound on the lower San Jose
Avenue lanes through both crossings. Both lower runs remained between
55.39 m and 59.58 m NAVD88, used street-level support, spent no time
blocked and recorded no dip above the 0.5 m threshold.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  bernal-cut-richland-eastbound bernal-cut-richland-westbound \
  bernal-cut-highland-eastbound bernal-cut-highland-westbound
```

- Game manifest SHA-256 `3d327ffd799ead97a209dcc6a2238d84b9852b3318b0c105b8e444520efbd9df`.
- Contact SHA-256 `8379a7c77b56e6525fa369d4ad77e321113bd71fb94867fb2abc0f6bfac49c9b`.
- One L13 tile changed. The game retained all 137 emoji placements and their revision.

## Broadway to the Marina via Polk Street

The original route generator used Bartol Street steps before the tunnel
and the divided approach at Larkin and Francisco after it. The repeated
route passes through Vallejo and Montgomery before entering the Broadway
bore, then takes Polk from Broadway. The route reaches the same two
emoji locations. [Bartol Street stair survey](https://www.sfstairways.com/stairways/bartol-street-at-vallejo-street/) ·
[DPW roadway structures inventory](https://data.sf.gov/api/views/x5tj-txci/rows.pdf?accessType=DOWNLOAD)

Chrome collected both emojis on the Bernal Cut release, covering 3.70 km
in 376.3 s. The traces identify the full Broadway bore as enclosed and put
the ground-contact 95th-percentile grade at 15.62%, with a peak of 22.79%
on Vallejo Street. There were no observed dips deeper than 0.5 m and no
blocked contact. The fixed-step run of the same itinerary used 734.85 m
of tunnel contact. These are game measurements.

The [Broadway-to-Marina course](../../tools/sf-drive/road-audit-broadway-marina.json)
pins the street intersections and the Ocean Avenue game manifest.

## Ocean Avenue and the northbound I-280 entrance

The measured 99.28 m Ocean Avenue deck crosses the terrain cut as a single
16 m-wide surface spanning two static tiles. An eastbound car can turn on
ordinary pavement into the northbound entrance. Its next highway seam
carries the car through Branch East and onto the existing Monterey to
Alemany roadway. The southbound Ocean exit remains available. [Caltrans I-280 interchange inventory](https://dot.ca.gov/-/media/dot-media/programs/safety-programs/documents/exit/calnexus-i-280-03-2020-a11y.pdf)

Chrome drove 13 m on ground between the bridge and entrance, then
finished the ramp grounded. The full northbound run crossed seven
contacts in 113.4 s and reached 45.5 mph. From a 49.5 mph rolling start,
a second drive took the southbound exit, circled via Geneva Avenue and
Howth Street, crossed Ocean eastbound and re-entered northbound I-280.
It finished beyond Monterey in 271.9 s with 628.4 m of grounded travel
between the ramp and bridge contacts. The static test checked 3,063
vehicle poses along the new and shared contacts at the centre and two
lateral offsets. All were clear with the same deck clearance used by the
game. The largest fitted dip was 0.183 m against an approximately 20 m
chord on the northbound mainline. The steepest fitted grade was 8.76%
at the eastern bridge approach. These local checks cover the new
bridge, ramp and northbound continuation. The southbound mainline
under Ocean Avenue and the southern Geneva entrance still need review.

- Game manifest SHA-256 `ca90679cade799d95fc2270c921a0eca05d7b3daf76998586ca2dbad5c5f54fd`.
- Contact SHA-256 `7ede428d23c5059bb326e8c9d251ed0af5b67078b1d4a45a2abefb3d2b385f97`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  ocean-avenue-bridge-eastbound ocean-avenue-bridge-westbound \
  ocean-bridge-to-i280-northbound i280-ocean-northbound-entry \
  i280-ocean-exit-and-reentry
```

## Dogpatch, Presidio and Fort Funston follow-up

The final three course fixtures cover the original emoji pairs. The
Dogpatch run approaches the 23rd Street lower crossing from the east
and continues from Pennsylvania Avenue to Potrero. Both Pennsylvania
off-ramps have exit-only ground portals, so through traffic on
Pennsylvania follows the street. The Liggett Avenue and Sloat Boulevard
runs encountered inferred trunks in the driving lane. The new bake
checks the scene rows against 3DEP pavement and low obstruction
returns before removing the eight false trunks across these routes.
On Skyline Boulevard the steering fixture uses the west side of the
pavement beside an uncertain return that stays in the scene.

| Course                      | Fixed-step distance | Ground grade p95 | Grade peak |
| --------------------------- | ------------------: | ---------------: | ---------: |
| Dogpatch to Potrero         |             4.63 km |            15.5% |      34.0% |
| Lake Street to the Presidio |             4.77 km |            10.5% |      17.5% |
| Ocean Beach to Fort Funston |             3.82 km |             6.4% |      10.0% |

All three fixed-step runs collected both emojis. The Dogpatch run used
the existing 81 m contact underneath I-280.

- Game manifest SHA-256 `20a1c8743338cc9e5e9827ba2c91fc2c7fc1cbeadda661d1990df74baa5ca9ce`.
- Contact SHA-256 `9f03bae8b7f550f2a2e4a63fd5d38fbede3af394ec42883422f464d14d58f6bb`.
- [Pinned course fixture](../../tools/sf-drive/road-audit-cross-city-lane-clearance.json).

## Chrome completion of the twenty emoji itineraries

Twelve original drives collected both emojis. Correcting the routes through
Broadway, Minna, Pennsylvania, the Embarcadero and West Portal raised the
count to seventeen. Chrome completed the final three with the lane-clearance
bake and manual keyboard input after switching off self-driving. These
measurements use the wheel contact while the car was grounded.

| Course                      | Driven distance |    Time | Ground grade p95 | Grade peak |
| --------------------------- | --------------: | ------: | ---------------: | ---------: |
| Dogpatch to Potrero         |         4.65 km | 485.1 s |            15.7% |      34.9% |
| Lake Street to the Presidio |         4.77 km | 487.0 s |            10.6% |      16.3% |
| Ocean Beach to Fort Funston |         3.83 km | 387.8 s |             6.5% |       9.5% |

All three collected both emojis, with zero measured dips over 0.5 m and
zero blocked seconds. Dogpatch used the 23rd Street contact below I-280.
The repeat drives complete the twenty-course audit. The short grade peaks
measure game contact over a 1 m baseline. They warrant review of the
source surfaces before drawing conclusions about physical pavement.

## I-280 across Geneva and Ocean

The Geneva bake adds about 3.43 km of connected pavement on both I-280
carriageways and the four Geneva Avenue ramps. The new northbound branch
keeps through traffic on the mainline and connects the right lane to
the exit. Both off-ramps release the car onto ordinary city pavement.
A recorded set of pavement observations brackets each longer under-deck
shadow. The fit interpolates inside four such intervals of 23.91 to
26.91 m.

A static clearance check sampled 5,163 vehicle poses at the centre and
two lateral offsets. All centre poses were clear. Two shoulder poses on
the northbound entrance overlapped an inferred trunk with ambiguous
survey support. The largest fitted local dip was 0.120 m against an
approximately 20 m chord. Peak fitted grades were 3.60% northbound and
4.66% southbound on the new through sections, and 8.14% on the steepest
ramp.

The Chrome trial started at 49.6 mph on southbound I-280, took the Geneva
exit, drove across Geneva Avenue and entered northbound I-280. It
travelled 69.3 m between the two road contacts and registered 53.5 m of
grounded movement there. The run finished in 108.7 s, with no browser
errors. The separate northbound exit also passed from a 49.9 mph rolling
start. Both through runs stayed on the lower carriageways beneath the
crossings. These results cover the Geneva corridor in the game.

The public build repeated the Geneva exit and re-entry in 106.5 s from
49.8 mph. It travelled 71.1 m between ramp contacts, with 47.6 m grounded
on city pavement. Chrome also restored all three airtimes and an emoji
collected after an offline restart.

- Game manifest SHA-256 `c91251bb788c18768ae9c270546820c0c5ded07297f35286aeb1c0d0b2abc29b`.
- Contact SHA-256 `57ce52172d15bc6c3b0908ee7f806b11466c29d25b9e2a18b39472576b4d0d5c`.
- [Gerardus source and survey provenance](https://github.com/banshee-data/gerardus/pull/164).

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  geneva-northbound-through geneva-southbound-through \
  geneva-northbound-entry geneva-southbound-entry \
  geneva-northbound-exit-rolling geneva-southbound-exit geneva-exit-and-reentry
```

## Blanken Avenue below US 101

The citywide ground screen on the Geneva game revision flagged 408
intervals above 50% grade. The largest flag was at Blanken Avenue,
where the draped street climbed to about 21 m on the freeway level.
Local survey observations put the pavement near 8.6 m and 7.7 m on
the two approaches. San Francisco Planning confirms the underpass
and shows a posted clearance of 14 ft 9 in.
[Visitacion Valley workshop, July 2018](https://sfplanning.s3.amazonaws.com/default/files/Citywide/Visitacion_Valley/vis_val_workshop_presentation-072818.pdf)

The bake adds 106.16 m of road contact and removes the obsolete ground
paint where it crossed the freeway. The interior profile interpolates
between surveyed approaches across a 56 m shadow. Its fitted peak
grade is 3.90%, and its deepest local dip is 0.034 m on an approximately
20 m chord. The game uses a 4.5 m clearance and an estimated 8.8 m
width. The static check found clear contact at the centre and two
lateral offsets across the whole strip. Chrome preview drives passed
eastbound and westbound and returned to street pavement at both ends.

The exact candidate repeated both directions in 14.4 s and 13.6 s,
respectively. Both US 101 drives stayed on their upper pavement at
roughly 50 mph. The static contact separation was 12.3 m southbound and
13.0 m northbound at the checked crossing positions. The updated ground
screen flags 392 intervals above 50%
grade. It now examines the open Blanken approaches and omits the
replaced draped segment. Green Street remains the highest ground flag,
at 115.4% over a short interval.

- Game manifest SHA-256 `b5a172e6d1d85cabce7d6f372dc77e9006319eedcf3d7731ebd7afdcb56435a9`.
- Contact SHA-256 `a7cf84c374804fce23c29c778e9cfb277fc8e9381ecad44e70cb36b8b682a939`.
- [Gerardus source and survey provenance](https://github.com/banshee-data/gerardus/pull/165).

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  blanken-underpass-eastbound blanken-underpass-westbound \
  blanken-us101-northbound blanken-us101-southbound
```

## I-280 from Geneva Avenue to SR 82

The seven new contact strips carry both I-280 carriageways from
Geneva Avenue to the southwest map edge. They also carry the
northbound San Jose Avenue entrance and both southbound branches.
The game previously used the ground grid in this corridor, which
understated the roadway by about 7.8 m at one northbound crossing
and 7.3 m at one southbound crossing. The new contact shares the
existing Geneva endpoints and keeps vehicles on the correct level.
[Caltrans interchange inventory](https://dot.ca.gov/-/media/dot-media/programs/safety-programs/documents/exit/calnexus-i-280-03-2020-a11y.pdf)

The April 2023 survey supported 1,996 of 2,005 stations. The longest
observation step was 14.98 m. The fitted mainline peaked at 4.84%
grade. The steepest ramp reached 6.34%. Its deepest local dip was
0.237 m near the bottom of the climb onto northbound I-280. Those
numbers use a chord of about 20 m and describe the game contact.
The clearance check sampled the centre and two lateral offsets.
All centre poses were clear. Two false trunks were removed from the
San Jose Avenue exit after reviewing the nearby survey returns.

Chrome preview drove both through carriageways and all three ramps.
The northbound through drive took 108.3 s. The southbound through
drive reached the terrain handoff at the map edge in 111.6 s.
Both used manual keyboard input. The final entrance handoff begins
16 m into the approach where the ground and fitted surface agree.

- Game manifest SHA-256 `6f884b2cd031c16ff9c93111ff4850f055e3387410f104ddd76e88bde40f531c`.
- Contact SHA-256 `d64ebea63ea919dc38195509e09c6e978162b4d82c686c19fafd4e30db941ea3`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  i280-sr82-northbound-through i280-sr82-southbound-through \
  i280-sr82-northbound-entry i280-sr82-southbound-exit \
  i280-sr82-southbound-edge-exit
```

## East Span from Yerba Buena to the map edge

The lower eastbound approach previously ended just beyond the Hillcrest
merge. At 45.6 mph the live game lost continuous contact and stopped.
The two source centre lines also diverged from the observed decks
over the water. The new package refits the lead-ins and carries each
direction to the existing game map boundary.

| Direction | Added contact | Peak grade | Deepest dip over about 20 m |
| --------- | ------------: | ---------: | --------------------------: |
| Eastbound |       906.7 m |      1.98% |                     0.016 m |
| Westbound |       785.0 m |      1.85% |                     0.020 m |

April 2023 3DEP observations supported all 609 fitting stations. The new
road surfaces are 20 m wide in the game. They taper into the existing
approaches over 32 m. An obstacle check exercised the centre and two
lateral offsets on both extensions and lead-ins. All 3,018 poses were
clear. The width and grade describe game contact. The game map ends
over water before the Oakland landing.

The First Street and Essex starts both continue onto the new eastbound
contact. The westbound start begins at 50 mph near the eastern map
boundary. The static game package carries both deck strips and their
directed joins in the existing island tile.

- Game manifest SHA-256 `da99f3fb3a282ae685a893b993240c071832cfe3841483095eb4d45c6dd2dd9c`.
- Contact SHA-256 `0ca165ab974f22961950439a6f6352524cad87565e36a3149db64ebc93d858ad`.
- [Gerardus recipe and provenance](https://github.com/banshee-data/gerardus/pull/167).

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  east-span-eastbound island-east-westbound-exit macalla-westbound-roundtrip
```

## San Jose Avenue above the Bosworth ramps

The live game followed the terrain of the lower interchange at the
San Jose Avenue crossing. A southwest keyboard drive dropped to
48.38 m NAVD88 about 150 m from the start. The same drive on the
baked upper surface stayed grounded at 57.08 m. The 343.3 m contact
strip joins ordinary terrain at both ends.

April 2023 3DEP observations supported all 116 centre stations
and all five offsets at each of 90 width-check stations. The game
strip is 8 m wide. It peaks at 3.19% grade, with a deepest local
dip of 0.047 m over a chord of about 20 m. The centre and two
lateral offsets cleared all 516 scenery checks.

| Chrome drive |   Time | Peak speed | Exit                      |
| ------------ | -----: | ---------: | ------------------------- |
| Southwest    | 30.4 s |   29.3 mph | Grounded on city pavement |
| Northeast    | 30.9 s |   29.6 mph | Grounded on city pavement |

Both drives used manual keyboard input. The upper street and lower
terrain remain separate. The nearby entrances require their own
joins to the southbound mainline.

- Game manifest SHA-256 `086026f0f07816bfab8b60707885de5062359872eeb8d9b30a00c38d42430984`.
- Contact SHA-256 `80fd985238790753815976b6af40bdc2b40aa3e0cf9011ce070331f5365700d6`.
- [Gerardus survey recipe](https://github.com/banshee-data/gerardus/pull/168).

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  san-jose-bosworth-southwest san-jose-bosworth-northeast
```

## Bosworth and San Jose Avenue onto southbound I-280

The two entrances now come together at 49.01 m NAVD88. A shared ramp
joins the existing Monterey to Ocean mainline contact at 50.03 m.
The Bosworth entrance stays about 8.4 m below the San Jose Avenue
deck at their crossing. Both tiles carry each complete approach, so
the road and its joins span the tile boundary.

The three fitted sections total 1.064 km. The April 2023 survey supports
356 of 359 fit stations. The longest observation step is 11.98 m. The
shared merge has the steepest grade at 5.32%. The largest fitted dip
over a chord of about 20 m is 0.101 m on the Bosworth approach.

Before correcting the inferred trunks, the first Chrome runs struck
scenery on the approaches. The bake removes six inferred trees at
measured pavement footprints. All 534 centreline poses then cleared
the game collision check. Seven sampled poses farther to the sides
still touched retained roadside trees.

| Chrome drive             |   Time | Peak speed | Finish                       |
| ------------------------ | -----: | ---------: | ---------------------------- |
| Bosworth Street entrance | 57.0 s |   40.5 mph | Grounded on southbound I-280 |
| San Jose Avenue entrance | 62.9 s |   40.6 mph | Grounded on southbound I-280 |

Both keyboard drives used the lower approaches and the shared ramp
under the overpasses. The separate upper San Jose Avenue route remains
available.

- Game manifest SHA-256 `b3dba8fb851d10d0c237d332b732da4870e1cdf2e137c804f3a4602cc058f575`.
- Contact SHA-256 `7eb6a51360263767558a3628059f567599395482f7c65ef4b3dd83e4faef8cf5`.
- [Gerardus survey recipe](https://github.com/banshee-data/gerardus/pull/169).

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  bosworth-southbound-entry san-jose-southbound-entry
```

## Twenty-route regression after the Bosworth entrances

The same frozen itineraries completed 19 of 20 routes on the earlier
and new game bakes. The failing route reached the same point on
southbound I-280 in both packages. Its replay still left the freeway
at the old ground handoff. The updated itinerary follows the paved
Alemany exit, crosses the Alemany overpass, and reaches the same Sweeny
Street pickup. The change only revises the test path. The new
[route fixture](../../tools/sf-drive/road-audit-bosworth-release-routes.json)
records the current manifest and requires every pickup and expected
contact exit.

All 20 fixed-step drives collected both emojis. They covered 52.43 km
in 83.8 minutes of simulated driving. The largest measured step at a
driven seam was 6.1 cm, and no driven dip exceeded 0.5 m. The highest
measured grade was 26.2% on Ordway Street in the US 101 to Bayview
course. That peak is a short game-contact measurement.

A separate citywide terrain screen examined 14,996 road rows.
It flagged 392 intervals above 50% grade and 51 missing intervals.
The screen follows ground under some raised roads, so its counts
identify areas for review rather than direct failures of raised game
contact.

The public Chrome run also reopened cached starts offline, collected
an emoji offline, and restored the pickup and three saved airtimes
after reload. Both southbound entrance drives reached the live
mainline beneath the overpasses in 57.3 s and 62.4 s. The earlier
westbound self-drive completed the Fremont and Fourth Street stops
and reached 115 mph on the legacy city approach, above the earlier
55 mph test target.

- Earlier manifest SHA-256 `6f884b2cd031c16ff9c93111ff4850f055e3387410f104ddd76e88bde40f531c`.
- Current manifest SHA-256 `b3dba8fb851d10d0c237d332b732da4870e1cdf2e137c804f3a4602cc058f575`.
- Current course fixture SHA-256 `d3e69f508b7c8cd58b96711dbe30fc5502f1aef8baaee2509d83ba5763ebdfa1`.

```sh
node tools/sf-drive/audit-road-routes.mjs \
  --plan=tools/sf-drive/road-audit-bosworth-release-routes.json
node tools/sf-drive/audit-road-flatness.mjs
```

## Monterey Boulevard and Branch East to San Jose Avenue

The previous live build lost road contact on the Monterey approach.
In Chrome, the car dropped about 9 m toward the lower ground and
crashed before reaching San Jose Avenue. The new contact carries
Monterey Boulevard and the Branch East exit into a shared deck,
then meets the San Jose Avenue terrain. A 46 m extension of the
exit viaduct joins the existing tunnel contact directly.

The baked upper pavement sits 7.37 m and 9.92 m above the lower
surfaces at the two checked crossings. Both owner tiles contain the
same complete Monterey approach, and the manifest records the joins.
The highest grade on the three fitted
strips is 4.22%, and the deepest fitted dip over a chord of about
20 m is 7.6 cm. All 864 sampled poses on the centre and two lateral
offsets cleared the game obstacle test.

| Chrome keyboard drive                 |   Time | Peak speed |
| ------------------------------------- | -----: | ---------: |
| Branch East tunnel to San Jose Avenue | 46.1 s |   31.6 mph |
| Monterey Boulevard to San Jose Avenue | 43.0 s |   28.8 mph |
| San Jose Avenue to Monterey Boulevard | 43.8 s |   28.4 mph |

All three preview drives reached street pavement grounded. The route
guide supplied keyboard inputs after manual takeover. The browser test
checks the order of the contacts, the ground handoff and a maximum
of 2 m of airborne travel. [Gerardus survey recipe](https://github.com/banshee-data/gerardus/pull/170).

The 20 fixed-step emoji courses collected both pickups again on the new
manifest. They covered 52.43 km in 83.8 simulated minutes. Their largest
measured seam step was 6.1 cm, and no driven dip exceeded 0.5 m.

- Game manifest SHA-256 `0a447b9a7943bd8efbc00860c0ddc20e81e56c91e9d2ebc173a6d02673ab156d`.
- Contact SHA-256 `a57eb8dacff2f87c89ccba63f9e91e02f0fff4779dad910670c805a17e4eb3d8`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  branch-east-ramp monterey-to-san-jose san-jose-to-monterey
```

## Potrero Avenue and the lower Bayshore route

The first connector preview returned to terrain before the freeway
shadow. Chrome reached Bayshore Boulevard after 4.29 to 5.73 m of
airborne travel on the lower street. A traced run located three short
flights beneath the freeway and near the far side of the underpass.

The next bake fits 212.4 m from the Potrero Avenue fork to the lower
Bayshore junction. It joins the 59.9 m continuation of the existing
US 101 southbound exit, and the two approaches share that pavement
through the shadow. The lower junction is 8.20 m below upper US 101
in the game. The curve reaches an 8.92% peak grade. The deepest
fitted dip across a chord of about 20 m is 19 cm on the new connector.
[Gerardus survey recipe](https://github.com/banshee-data/gerardus/pull/171).

All 248 centreline collision poses cleared on the three affected
strips. On the sampled offsets, three poses touched one retained
roadside tree near the lower exit. The fit used 68 observed stations,
with the final 10.3 m interpolated from lower-elevation returns to
the known junction under the freeway.

| Chrome keyboard drive                    |   Time | Airborne |
| ---------------------------------------- | -----: | -------: |
| Potrero Avenue to lower Bayshore         | 47.0 s |   2.79 m |
| US 101 southbound exit to lower Bayshore | 57.6 s |    1.2 m |
| Potrero Avenue entrance to US 101 south  | 55.8 s |      0 m |
| US 101 south above the Bayshore junction | 21.6 s |      0 m |

Both lower drives entered the covered camera and then reached
exterior street pavement. The freeway entrance and through lane
finished grounded on the upper carriageway.

The 20 fixed-step emoji courses collected both pickups on this manifest.
They covered 52.43 km in 83.8 simulated minutes. The largest driven seam
step was 6.1 cm, and no driven dip exceeded 0.5 m.

- Game manifest SHA-256 `d25cfa374e8b12a83f2e982f916db66c0f8a67903143d5322350c345ea7ad193`.
- Contact SHA-256 `bda38c25f1ad80d830e1adaf94151148e9cf2dffc5fc2b1f61dad7732c4d2a9e`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  potrero-avenue-to-bayshore us101-south-bayshore-south-turn \
  potrero-avenue-to-us101-south us101-south-over-bayshore-exit
```

### Bayshore street departure

The first public Potrero drive reached exterior street pavement
with 4.55 m of airborne travel. A second trace measured 3.18 m,
with two flights 25 m and 33 m beyond the lower portal. At those
locations the terrain was 17 cm and 10 cm above the surveyed
centreline. The one-metre terrain refinement extends with 28
survey stations across five lateral offsets. It blends into the
coarse grid and preserves the portal height to within 1 cm.
[Gerardus terrain recipe](https://github.com/banshee-data/gerardus/pull/172).

Chrome drove the corrected Potrero route twice and the southbound
US 101 exit once to exterior Bayshore Boulevard with zero airborne
travel in the preview. The same 3 m airtime check still covers the
Potrero run. All 20 fixed-step emoji courses completed again, with
a 6.1 cm largest seam step and zero driven dips over 0.5 m.

- Game manifest SHA-256 `d0d60fe379fadd327cdcfa8639b82ad99d76a8553b0d24349e27439b0b647240`.
- Contact SHA-256 `c7917d4bfbe1265f5d5f2887168e7ea3b40ba67e8990a363a9de6aff497b848a`.

## SR 1 south of MacArthur Tunnel

The previous live game put both directions onto the terrain below
the carriageways. Chrome crashed in both directions. At the
checked crossing the game terrain was 5.39 m below the observed
northbound pavement and 5.04 m below the southbound pavement.

The bake carries each direction about 222 m from ground to the
short pavement handoff at the south portal. Both tiles carry
the complete contacts and the portals match the adjoining ground.
The first 148 m preview resolved both crashes but exposed further
bumps on the remaining open approach. The extension uses 26
additional observed stations in each direction.

The inferred building row overlapped the measured lanes and
caused a collision even when the car was on the fitted pavement.
The bake clips that volume to the travel strips and removes an
inferred trunk from a survey-supported northbound lane footprint.
All 666 sampled centre and lateral collision poses cleared.
[Gerardus survey recipe](https://github.com/banshee-data/gerardus/pull/173).

| Chrome keyboard drive                         |   Time | Airborne |
| --------------------------------------------- | -----: | -------: |
| Northbound across SR 1 and through the tunnel | 62.9 s |      0 m |
| Southbound through the tunnel and across SR 1 | 63.4 s |   1.69 m |

Both cars crossed the short pavement handoff grounded and
finished on exterior street pavement. The game can transfer directly
between the approach and tunnel contacts in adjacent rendered frames.
The browser check verifies the contact order and grounded state through
that handoff. The maximum fitted grade is
8.50% northbound and 7.20% southbound. The largest local dip across
a chord of about 20 m is 6.7 cm.

All 20 fixed-step emoji courses collected both pickups over 52.43 km.
The largest driven seam step was 6.1 cm, and no driven dip exceeded
0.5 m.

- Game manifest SHA-256 `a80f8734f13ae6f037d72506e39c78942cfb8f1b4316680915452ca5ddfb0f67`.
- Contact SHA-256 `1e0d9aa803974d450a7307423ec1b3cc521d099d52d9e1d86f99b4fd63b56d56`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  macarthur-and-south-crossing-northbound macarthur-and-south-crossing-southbound
```

### Southbound I-280 left exit to southbound US 101

The bake adds 696 m of upper-deck contact for the left exit at Alemany.
Southbound I-280 continues past the fork and the new ramp joins the
existing southbound US 101 descent. The northbound I-280 viaduct stays
on its lower contact. Each tile touched by the added ramp carries the
complete strip. The 8.5 m ramp tapers into the adjoining widths.

The fitted upper strip reaches a 3.49% longitudinal grade. Its largest
local dip is 3.2 cm over an approximately 20 m chord. These are
measurements of the game contact, which uses the 2023 aerial survey
returns on the upper pavement. [Gerardus survey recipe](https://github.com/banshee-data/gerardus/pull/174).

| Chrome keyboard drive                 |   Time | Airborne |
| ------------------------------------- | -----: | -------: |
| Southbound I-280 left exit to US 101  | 87.4 s |      0 m |
| Southbound I-280 through the new fork | 21.2 s |      0 m |
| Existing upper approach to US 101     | 16.6 s |      0 m |
| Northbound I-280 beneath the exit     | 20.9 s |      0 m |

All twenty fixed-step emoji courses collected both pickups over
52.43 km. The I-280 course stayed on the mainline through the new
fork and reached the existing Alemany Boulevard exit. Across 50
tile crossings, the largest measured vertical step was 6.1 cm.
The ground-contact grade p95 had a median of 9.54% across the
twenty routes. None of the driven traces had a dip deeper than
0.5 m against an eight-metre chord. The separate citywide screen
flagged 392 intervals over 50% grade and 51 missing intervals among
the ground-draped road samples.

- Game manifest SHA-256 `b40ff3d8219a460bb4e94181ff73f6ab8a84e238863ec8b508b64b30eea8634d`.
- Contact SHA-256 `568875445fa6ea64f7d0f509c5f92271ac313904f7eac7290f9245ec46d418da`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  southbound-i280-left-exit-to-us101 southbound-i280-through-left-exit-fork \
  alemany-upper-over-northbound-connector northbound-i280-below-alemany-left-exit
```

### Follow-up routes through Alemany, Dogpatch and Geary

The full I-280 keyboard itinerary stopped at the Alemany fork when the
predicted line fit both outgoing surfaces equally. A tied fork now
continues on a uniquely matching DataSF alignment. The driver's heading
still selects the exit when its path has the smaller miss. The baked
widths carry the measured left lanes into both outgoing surfaces.

A second keyboard itinerary hit two inferred roof faces on the
southbound Dogpatch deck. The survey recorded 641 returns at the
pavement height and none in the sampled vehicle-height band. The bake
clears those two faces within the measured carriageway.

The Geary route exposed a steep step where the eastern street terrain
met the tunnel. The measured approach now meets the existing portal
plane. In five lane positions over the last 30 m, the largest sampled
grade fell from 68.3% to 4.7% and the largest join step fell from 68 cm
to less than 3 mm.

All 23 fixed-step courses collected both pickups over 66.57 km.
There were 62 tile crossings, a largest seam step of 6.1 cm and zero
driven dips deeper than 0.5 m against an eight-metre chord.
[Pinned course plan](../../tools/sf-drive/road-audit-alemany-geary-routes.json).

| Itinerary                           | Distance | Grade p95 | Peak grade |
| ----------------------------------- | -------: | --------: | ---------: |
| Fifth Street to Alemany Boulevard   |  8.79 km |     7.70% |     17.48% |
| Fifth Street to Portola via US 101  |  7.47 km |     7.91% |     26.57% |
| Divisadero to Mayfair via Geary     |  2.17 km |     8.18% |     20.36% |
| Lombard to Embarcadero via Broadway |  4.51 km |    16.64% |     23.34% |

- Game manifest SHA-256 `c5866d8204a880292ddb7fcb0f6a422f6d3665f4a511378d56cbe7e0ca397dcc`.
- Contact SHA-256 `40d4b797424c678f751c06aa1c9eb75501ba337ffaeb84b240d89e1e1e14f04a`.

```sh
node tools/sf-drive/audit-road-routes.mjs \
  --plan=tools/sf-drive/road-audit-alemany-geary-routes.json
```

#### Full keyboard drives in Chrome

The frozen plan produced 23 completed emoji-to-emoji drives in Chrome.
The driver used the game's manual throttle, braking and steering inputs.
Fourteen itineraries ran on the earlier `a80f8734` bake, five on
`b40ff3d8` and four on the `c5866d82` release candidate. The four
candidate drives used the exact tiles and game modules subsequently
published. Together the 23 drives covered 66.72 km in 103 combined drive
minutes. There were no local dips deeper than 0.5 m against the
eight-metre chord and no requests to Arrow from a drive page. The median
route grade p95 was 8.34%. The citywide terrain screen still put 392
intervals above 50% grade and flagged 51 missing intervals among
1,190,480 supported intervals.

| Itinerary on `c5866d82`             | Distance |        Time | Grade p95 | Peak grade |
| ----------------------------------- | -------: | ----------: | --------: | ---------: |
| Fifth Street to Alemany Boulevard   |  8.80 km | 10 min 20 s |     7.78% |     17.48% |
| Fifth Street to Portola via US 101  |  7.49 km |   8 min 6 s |     7.88% |     27.73% |
| Divisadero to Mayfair via Geary     |  2.17 km |  3 min 40 s |     8.03% |     10.62% |
| Lombard to Embarcadero via Broadway |  4.51 km |  7 min 24 s |    17.17% |     23.30% |

The Portola run recorded the 27.73% peak after leaving US 101. Earlier
runs on that itinerary struck inferred roof faces in Dogpatch. The
Alemany run had stopped at the highway fork. Both completed on the
corrected bake. Separate drives on the live game verified the Alemany
upper exit, upper mainline and lower northbound viaduct with zero metres
of airtime on those road sections. The Geary portal checks exercised
both directions. These grade and dip measurements describe sampled car
contact in the game.

### Presidio exits and the SR 1 merge

The Doyle rebake adds 1.29 km of linked contact. A first Chrome drive
left the lower loop at its former ground handoff. The bake now treats
that handoff and two others as road joins, using the same position,
elevation and cross-section on both sides. The two cross-tile roads
carry identical contact geometry in both tiles.
The game suppresses three provisional preview runs that followed the
upper pavement above the lower alignments.

| Keyboard drive                                 | Added contact on route | Duration | Airborne distance |
| ---------------------------------------------- | ---------------------: | -------: | ----------------: |
| Northbound US 101 loop to southbound SR 1      |                363.1 m |   71.9 s |               0 m |
| Golden Gate southbound exit to southbound SR 1 |                383.8 m |   68.6 s |               0 m |
| Ruckman northbound to the Golden Gate approach |                597.6 m |   57.6 s |               0 m |

Chrome used the covered camera while the car stayed on the lower loop
and again while northbound SR 1 passed under Doyle. The April 2023
survey observes the upper pavement and brackets a 56.81 m shadow on the
lower loop. The game interpolates the lower contact across that
interval. The interpolation gives a continuous drive in the game.
Field clearance inside the shadow remains unmeasured.

All 23 fixed-step emoji courses ran on this manifest. They collected
both pickups over 66.57 km, with 62 tile crossings and a largest
observed seam step of 6.1 cm. No sampled dip exceeded 0.5 m.

- Game manifest SHA-256 `a42741ed96a16082ffe549caa65efbfecba4af7df50afe09ac8122d350e9b6b4`.
- Contact SHA-256 `ea49a1125d8d526b433d33363cdd7c932857f6bc3bc4d1529cda537c5df3aaa3`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  doyle-northbound-loop-to-sr1 golden-gate-southbound-exit-to-sr1 \
  ruckman-northbound-to-golden-gate
```

## Local bridge decks in Golden Gate Park and on the waterfront

The current terrain grid followed the lower elevation where four
roadways crossed a gap. Chrome manual drives reproduced the falls on
the public bake. The April 2023 survey provided an exposed pavement
profile for each deck. Illinois Street uses a surveyed lateral approach
because the street inventory centreline approaches the bridge from the
lower side of the embankment. The three affected tiles carry the complete
contacts and retain the open ground approaches at both ends.

| Crossing                           | Lowest altitude in the original Chrome drive | Airborne distance in the original drive |
| ---------------------------------- | -------------------------------------------: | --------------------------------------: |
| Crossover Drive above JFK Drive    |                                      72.63 m |                                 23.05 m |
| Illinois Street above Islais Creek |                                       0.30 m |                                 15.28 m |
| Third Street above Islais Creek    |                                       3.49 m |                                 11.83 m |
| Fourth Street above Mission Creek  |                                       2.71 m |                                  6.49 m |

Those altitudes describe the simulated car. The survey found the
Crossover roadway near 79.5 m and the Illinois span near 4.4 m across
these traces. The game now carries each bridge on its own contact surface.
At Crossover, the lower JFK Drive contact follows its measured approaches
and interpolates their connection through the short aerial survey shadow.
The city lists Crossover Drive and the western section of JFK Drive as
open to vehicles. [Golden Gate Park road access](https://www.sfrecpark.org/1618/Getting-to-Golden-Gate-Park-by-Car).

The five contacts total 598.92 m. The largest difference between fitted
pavement and the underlying terrain was 6.89 m on Crossover Drive.
Illinois Street's largest difference was 4.16 m. In Chrome both lower JFK
lanes stayed on the lower contact, used the covered camera, and returned
to open pavement grounded. The westbound handoff extends past the first
street grade break. Its earlier portal allowed a 4.62 m bounce after
leaving the contact. The extended contact covers that break.

| Contact                            |   Length | Peak fitted grade |
| ---------------------------------- | -------: | ----------------: |
| Crossover Drive                    |  99.00 m |             2.91% |
| JFK Drive below Crossover          |  83.00 m |             6.40% |
| Third Street above Islais Creek    | 111.00 m |             2.35% |
| Illinois Street above Islais Creek | 241.92 m |             2.83% |
| Fourth Street above Mission Creek  |  64.00 m |             1.87% |

The lower JFK profile interpolates 26.68 m between the two surveyed
approaches inside the aerial shadow. The other four contacts have
observations at every fitting station with a maximum spacing of 3 m.
[Gerardus recipe and provenance](https://github.com/banshee-data/gerardus/pull/177).

All 23 fixed-step emoji routes reached both pickups on this bake. They
covered 67.59 km and crossed 63 tile seams. The largest observed seam
step was 6.1 cm and no local dip exceeded 0.5 m.

- Game manifest SHA-256 `20b637a302983dced7779cd75c09927284c805356f971f2e85798602a3e5c980`.
- Contact SHA-256 `949ce7fdbe9fc11a7128e0a5357f18d526b1aae818b78ae758516d2f0fac5261`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  crossover-drive-over-jfk-southbound crossover-drive-over-jfk-northbound \
  jfk-drive-under-crossover-westbound jfk-drive-under-crossover-eastbound \
  islais-creek-third-street-southbound islais-creek-third-street-northbound \
  islais-creek-illinois-street-southbound islais-creek-illinois-street-northbound \
  mission-creek-fourth-street-southbound mission-creek-fourth-street-northbound
```

## Lyell Street beneath I-280

The previous bake used the city terrain grid at the Lyell Street
undercrossing. Its centreline rose 2.38 m in 10 m, and a northbound
Chrome keyboard drive travelled 12.14 m airborne at about 25 mph.
The April 2023 aerial survey observes the freeway decks above the
lower street and the pavement beyond each side of the shadow.
[Public Works structure inventory](https://data.sf.gov/api/views/x5tj-txci/rows.pdf?accessType=DOWNLOAD).

The new bake gives Lyell a 103.87 m, 7 m wide contact underneath both
carriageways. The fit uses direct observations at 23 of 36 stations and
interpolates 41.60 m between the last and first observed lower pavement
on either side. One-metre terrain samples meet the lower road so the
coarse terrain stays below it. The fitted grade peaks at 14.84% on
the open north approach. That rise remains in the game.

Both final Chrome drives entered from street level, stayed on the lower
contact and returned to open pavement in the exterior view. The
northbound and southbound runs reached 26.7 mph and 28.2 mph respectively,
with zero airborne distance and no browser errors. An intermediate
candidate still lifted northbound just before entering the lower contact.
The final southern portal is 14 m earlier on the observed street.

All 23 fixed-step emoji routes completed again on the final package.
Across 67.59 km and 63 tile crossings, the largest observed seam step
was 6.1 cm and no driven dip exceeded 0.5 m over an 8 m chord.

- Game manifest SHA-256 `968f934d99974551f8e8ca9c0e563b766583c6858709cb66257a485482a287d7`.
- Contact SHA-256 `e25b0cd1ee8bcd15992b59b25171e60e7a0886e0b1174106587f0c784b4ad0e2`.
- [Gerardus recipe and survey provenance](https://github.com/banshee-data/gerardus/pull/177).

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  lyell-street-under-i280-northbound lyell-street-under-i280-southbound
```

## Beale Street beneath Harrison Street

The previous lower Beale Street followed an artificial rise in the
ground grid to 8.00 m at the crossing. Chrome recorded 10.85 m and
20.82 m of airborne travel in the southeast and northwest lanes. On
the northwest drive, the simulated car reached 9.22 m while airborne.
The April 2023 survey observes the lower road at about 5.2 m on both
sides, with a 21 m aerial shadow beneath the Harrison deck.

The new 66 m lower contact fits 17 of 23 stations directly and
interpolates the lower elevation through the shadow. The terrain in the
travel strip follows the lower contact. The Harrison deck continues at
about 15.58 m at the crossing. The bake clips the old draped paint from
both streets inside their respective contact extents.

Chrome drove both levels in both directions, entering from each street
and handing off grounded. All four drives reported zero airborne
distance. Peak speeds were 27.1 and 27.3 mph on Beale and 29.2 and
29.9 mph on Harrison. The covered camera remained in use at both Beale
handoffs as its view still extended beneath an overhead deck.

All 23 fixed-step emoji routes completed over 67.59 km. The largest
observed tile seam was 6.1 cm and no driven dip exceeded 0.5 m over an
8 m chord. The separate citywide draped-road screen flagged 384
intervals over 50% grade, down from 392 on the preceding bake.

- Game manifest SHA-256 `25fb1b3c000dc572d5b8c3358465866c329922b46b67f4e7b25ab045c5be2148`.
- Contact SHA-256 `f17402cba2493e85d9818e10bdbce5f8ef52feeb0f604f53f35554b35c945e76`.
- [Gerardus recipe and survey provenance](https://github.com/banshee-data/gerardus/pull/177).

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  harrison-beale-bridge-southwestbound harrison-beale-bridge-northeastbound \
  beale-street-under-harrison-southeastbound beale-street-under-harrison-northwestbound
```

## Southbound I-280 exit onto the 18th Street viaduct

The prior exit ended at 26.64 m with a declared ground handoff to
terrain at 19.02 m. Chrome stopped at 26.63 m in both a drive from
Fifth Street and a short replay from the off-ramp. The April 2023
survey observes the ramp and viaduct at approximately 26.66 m at
their shared node. [SFCTA's 18th Street ramp board](https://www.sfcta.org/sites/default/files/2026-10/Vision%20Zero%20Ramps%20-%20Round%202%20Outreach%20Boards.pdf#page=13).

The new bake joins the ramp to 18th Street with eastbound and
westbound curves fitted to the surveyed approach elevations. The
contact reader aligns each shared cross-section with its own road's
travel direction, letting the eastbound turn join the end of the
west-oriented viaduct. The west turn uses a 17 m landing onto the
open street. Straight travel across the viaduct still works in
both directions.

The asset regression checks all declared ground handoffs against
the contact tolerance used by the game. It also checks matching
endpoint coordinates and road geometry across tile owners. All 155
ground handoffs in this bake match their terrain within 7.5 cm.
The keyboard cases check both turns and the through paths separately.
Both exit turns reached exterior street contact with zero airborne
distance on the staged bake. The eastbound chase camera returned to
its exterior view on the open street beyond the deck.

Both full Fifth Street drives reached the westbound landing grounded.
One full run recorded 3.72 m airborne outside the intermediate
surface handoffs. The instrumented repeat recorded zero. The full
test checks uninterrupted contact between the elevated sections.
The two exit tests separately require zero airborne distance from
the off-ramp through the final street handoff.

All 23 fixed-step emoji routes completed over 67.59 km. The largest
observed tile seam was 6.1 cm and no driven dip exceeded 0.5 m over an
8 m chord.

- Game manifest SHA-256 `347163fe2ee9267d0f4c211c9880b29d5edb38bf3915e7dbccd6df797c89618f`.
- Contact SHA-256 `10fadb89e04d082f6e5500ea848be8a3253f17a1a040b1c606b1bee510c1547d`.
- [Gerardus recipe and survey provenance](https://github.com/banshee-data/gerardus/pull/177).

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  i280-18th-exit-westbound i280-18th-exit-eastbound \
  eighteenth-westbound eighteenth-eastbound i280-18th
```

### Live checks on the 18th Street revision

After publication, Chrome reached 18th Street by both turns and climbed
from 18th Street back onto northbound I-280. The westbound exit and
northbound entrance recorded zero airborne travel. One eastbound run
recorded 1.14 m of airborne travel after it had entered the final
viaduct section and still finished grounded on the open street. Two
instrumented repeats finished grounded with zero airborne travel.
The exact location of the intermittent lift remains unknown.

All 23 fixed-step emoji courses completed, including six with tunnel
contact and six with highway travel. They covered 67.59 km and crossed
63 tile seams. The largest seam height step was 6.1 cm and there were
no dips over 0.5 m on an 8 m chord. A separate course collected both
emojis via the new westbound exit, from Fifth Street to Potrero, over
3.30 km. The citywide screen of ground-draped road rows still flagged
384 intervals above 50% absolute grade.

### Music Concourse Garage approach

The steepest driven profile on the 23 fixed-step courses was the
10th Avenue park entrance, at 43.6%. The published game draws the
DataSF approach (CNN 469000) up to its mapped node on JFK Drive.
The April 2023 aerial survey records 69.48 m on the exposed garage
approach near the north portal and approximately 74.22 m on JFK
Drive above it. The grade screen caught the terrain rise between
those levels. SFMTA confirms through travel to Music Concourse
Drive via the garage. [SFMTA access diagram and drive walkthrough](https://www.sfmta.com/fr/media/32427/download?inline=).

The lower return at the mapped south portal is approximately 71.80 m.
These are portal observations in the game's NAVD88 frame. The route
within the garage appears in OpenStreetMap way 43736094 revision 7 and
ways 1551890169–1551890171 revision 1. The aerial survey cannot verify
the interior floor profile. The game fits 565 m of contact to the two
surveyed approaches and the inferred interior profile. The deck on
JFK Drive keeps its original elevation. The April 2023 survey predates
the south approach striping changes. [OpenStreetMap contributors, ODbL](https://www.openstreetmap.org/copyright) · [Rec and Park's south approach update](https://recpark.sf.gov/m/newsflash/home/detail/2225).

Chrome keyboard drives on the final candidate traversed the garage
in 104.1 s towards the Concourse and 104.7 s towards Fulton. Both
recorded zero airborne travel and returned to the exterior camera
grounded. Two fixed-step courses between the 6th and Fulton and
7th and Irving emojis completed through the garage over 2.08 km
and 1.80 km respectively. The west park course uses the baked JFK
Drive passage under Crossover Drive. A tile test checks that both
approaches join terrain and JFK Drive remains above the garage.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  garage-fulton-to-concourse garage-concourse-to-fulton
node tools/sf-drive/audit-road-routes.mjs \
  --plan=tools/sf-drive/road-audit-music-concourse-routes.json
```

- North portal survey receipt `8b9d5b3c132f5d5326c70b9fd9bc222df6df5e36d61e300f98147b29164d2e1d`.
- South portal survey receipt `110cd329764ab26a011c08b3490e6a4d270341ebfb74e053118db386baaa69ba`.
- Game manifest SHA-256 `f0f86ff2b793279775e21d2967b2262654d6ed899b41e9b043ceb76e88769746`.
- Contact SHA-256 `d8778a54a50d13fe0e354a9e75c893a01f7741ba9a7c0ee1617a1e4ee49f2e79`.
- [Gerardus bake recipe and support limits](https://github.com/banshee-data/gerardus/pull/178).

### Presidio, Yerba Buena and Branch East checks

The Doyle Drive northbound smoke now follows the published seam
onto the first Golden Gate Bridge section. Before that change, the
car stayed grounded through both tunnels but the smoke stopped when
the game selected the bridge. The lower Yerba Buena mainline smoke
still requires the covered view beneath the upper deck and the
exterior view at the finish. At the next lower section, the camera
can still see the deck above. The checked heights at the observed
transition were 52.41 m for the car and 60.91 m for the upper road.

At Branch East one browser replay measured 2.24 m of travel between
frames at an airborne sample 1.3 cm above the ground, before the
ramp entrance. Another replay logged a lift after the final handoff.
Four metres past the terminal, the old terrain gave 59.78 m and the
April 2023 survey observes 59.94 m. The new fit rises gently across
that interval and retains the older Lyell Street terrain patch in
the same tile. The smoke checks contact through the ramp joins and
reports the airborne travel after entering the ramp separately.

## Provisional overpass meshes

The game retained nine provisional strips after their playable
replacements were baked. The affected alignments run through Yerba
Buena, the Essex merge in SoMa, and I-280 near Alemany and Dogpatch.
The strips total 1.94 km. At each preview centre the released game
also had an authored deck within 0.8 m in elevation. Some provisional
profiles sampled the upper carriageway above the road named in the
source inventory.

On the northbound I-280 lower viaduct, the provisional mesh supplied
a false ceiling for the chase camera. The baseline Chrome drive
completed the section grounded at 14.17 m NAVD88, but kept the
covered view at the finish. The drive over the revised tiles starts
in genuine cover and returns to the exterior view on the same
lower deck at the finish.

The revised bake adds 11 profile references to eight playable
surfaces. Seven tile files carry those updated references. The
decoded comparison checks the full 280-surface network, the
transitions, terrain, and the companion driving routes against the
previous release. The game renderer can now suppress all 249
provisional strips in the current pack.

The 25 fixed-step emoji courses completed again over 70.46 km.
Their driving telemetry matched the previous release, including
the 6.1 cm largest tile seam and zero dips exceeding 0.5 m over 8 m.
The Bay Bridge physics replay now shares the game's deck clearance
for scenery contacts. It finished the complete westbound run through
Fremont and back onto I-80 at Fourth Street in 321.24 s of game time.
The eastbound run through the lower Yerba Buena bore finished in
255.22 s.

- Game manifest SHA-256 `4beb3c03f588b06661f88560fcb28c8b1ccd085b5565196af73e8daaee0bdfaf`.
- Contact SHA-256 `46d4bcc3c11a4874dcb5e8b27077d36fe0629210c07d82a10824ff18cf7352a0`.
- [Gerardus profile associations and source checks](https://github.com/banshee-data/gerardus/pull/179).

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  northbound-i280-clears-remaining-cover northbound-i280-below-alemany-left-exit \
  island-east-lower-mainline island-east-eastbound-entry island-east-upper-mainline
node tools/sf-drive/replay-city-drive.mjs yerba-buena-westbound
node tools/sf-drive/replay-city-drive.mjs yerba-buena-eastbound
```

## Geary Boulevard and Fillmore Street

The older tiles used a single terrain height through the crossing. Along
Fillmore the ground dropped from about 39.1 m to 34.5 m NAVD88. On Geary
beneath it, the ground rose to 36.7 m at the same crossing. The resulting
surfaces let the car cross at the wrong heights and launch from the
terrain. The frontage eastbound drive left its course, and the westbound
drive crashed.

The revised Arrow bake adds five contacts in `808580bc`. Fillmore and
both Geary frontage roads use the measured upper band around 39.3 m.
Separate eastbound and westbound Geary road surfaces descend to about
33.1 m beneath the bridge. The covered portions of the lower profiles
interpolate between visible pavement observations on either side. The
bridge deck and lower surface are about 6.2 m apart at the tested points.
This is a game surface separation. Real clearance below the bridge
remains unmeasured.
[Public Works roadway structure inventory](https://data.sf.gov/api/views/x5tj-txci/rows.pdf?accessType=DOWNLOAD).
[SFMTA on the Geary frontage roads and underpasses](https://www.sfmta.com/project-updates/geary-rapid-project-faqs).

The input centreline and terrain hashes are frozen in the source recipe.
A one-metre grid follows the two lower lanes through the trench, with a
taper back to the original terrain near each handoff. The layer test
checks both lower lanes, the bridge, the two frontage surfaces and the
height at the ground portals. The rounded frontage curves carry traffic
through the upper junction on their own road surfaces. Six Chrome
keyboard drives entered and exited the bridge, frontage roads and lower
through lanes in both directions. All six recorded zero airborne travel.
Both lower drives entered the covered view and returned to the exterior
view at the ground departure. The focused drives cover the crossings in
each direction. Turning between the upper roads remains a separate test.

Twenty-five fixed-step emoji courses again completed over 70.46 km with
identical driving telemetry. The Bay Bridge driving bundle and the emoji
placements share the earlier release hashes. The citywide terrain-only
screen found 381 intervals above 50% grade, down from 384 in the parent.
It samples the ground rows separately from the authored elevated contact.

- Game manifest SHA-256 `b2b15a9f62589ba554507bd94b8c0fa17748213836b7acac598d29c4e55970df`.
- Contact SHA-256 `fa80998a60721f521616902a082f7928d0ca0cdc7e183d7e15069871ede0f0a0`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  fillmore-geary-bridge-northbound fillmore-geary-bridge-southbound \
  geary-under-fillmore-eastbound geary-under-fillmore-westbound \
  geary-fillmore-eastbound-frontage geary-fillmore-westbound-frontage
```

The [live emoji route audit](sf-live-emoji-route-audit.md) records the public
browser runs, the ground-approach check and the two deployed tile hashes.

## Williams and Paul Avenue rail crossings

The citywide terrain screen found two further road levels in Bayview.
The Williams and Paul Avenue road rows followed the rail cut in the
older game tiles. Chrome completed the four baseline courses with
between 26.6 m and 34.7 m of recorded airborne travel. The westbound
Williams course reached a 74.0% sampled grade. The eastbound Williams
course recorded two supported dips over 0.5 m, with a deepest dip of
0.57 m.

The revised game tile carries two independent bridge contacts. The
observed centreline elevations near the middle of the decks are
17.5 m on Williams and 15.5 m on Paul, in NAVD88. At the sampled
crossings these are about 7.4 m and 6.1 m above the previous terrain.
The fitted courses reach both street portals and retain the rail cut
below the bridge surfaces. This bake uses the observed roadway band.
[Caltrain crossing inventory](https://www.caltrain.com/media/36811).
[Caltrain roadway bridges project](https://www.caltrain.com/projects/san-francisco-roadway-bridges-project).

The citywide terrain-only screen found 364 intervals above 50% grade,
compared with 381 in the parent release. The two bridge contacts have
peak fitted grades of 0.72% on Williams and 5.02% on Paul. Four Chrome
keyboard drives on the candidate entered both bridges at street level
and reached their opposite portals grounded. They recorded zero airborne
travel and zero sampled clearance on the ground approaches. The 25
fixed-step emoji courses matched the previous driving telemetry.

- Game manifest SHA-256 `81e31f112ce9808042fc96ca8b7e9d8a868391363f17c21a41f6eed8bb8339dc`.
- Contact SHA-256 `4c9fce3e9a77bd63522aaa9bdb2f1609520ec6333ef2a97c657a7c87e6aa1f74`.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  williams-avenue-caltrain-bridge-westbound williams-avenue-caltrain-bridge-eastbound \
  paul-avenue-caltrain-bridge-westbound paul-avenue-caltrain-bridge-eastbound
```

## Seven street overcrossings

**World and Physics.** The Public Works structure inventory and SFMTA clearance
inventory identify roads crossing at different levels. The screen projected
67 Public Works records and 111 SFMTA records with coordinates into the game
frame, looked for road rows within 60 m, and sampled up to 35 m along the row
on each side of the closest point. It compared the terrain with the closest
authored contact. An existing contact can belong to the other road level.
Seven sites went to Chrome and the April 2023 3DEP pavement review after
checking the road names and which carriageway crosses above the other.
[Public Works inventory](https://data.sf.gov/w/x5tj-txci/ikek-yizv).
[SFMTA clearance inventory](https://data.sf.gov/Transportation/Clearance-Heights-Bridges-and-Underpasses/eb2x-6eay/about_data).

The keyboard driver covered both directions at each site on the Bayview
release. Each crossing reached its exit. The terrain took the car down
towards the road or rail cut, with the observed driving contact lost
for these distances.

| Crossing                                        | Inventory                         |          Baseline airborne distance by direction |
| ----------------------------------------------- | --------------------------------- | -----------------------------------------------: |
| Lincoln Way over Sunset Boulevard               | Public Works 49 and SFMTA 92      |             26.43 m westbound, 23.97 m eastbound |
| Sloat Boulevard over Sunset Boulevard           | Public Works 48 and SFMTA 94      |             20.26 m westbound, 27.82 m eastbound |
| Oakdale Avenue above the rail cut               | Public Works 61                   |              8.27 m westbound, 13.53 m eastbound |
| Junipero Serra Boulevard over Brotherhood Way   | Public Works 194 and SFMTA 25     |            17.10 m southbound, 8.95 m northbound |
| Mission Street over I-280 and Alemany Boulevard | Public Works 32A/32B and SFMTA 20 |           27.61 m southbound, 31.19 m northbound |
| Baden Street over I-280                         | Public Works 297                  | 18.75 m north-westbound, 11.29 m south-eastbound |
| Paulding Street over I-280                      | Public Works 296                  |             19.88 m westbound, 27.51 m eastbound |

The city and clearance survey points identify crossings and nearby streets
for review. The elevation fits use the 3DEP returns. The contact and
paint change follows the observed upper road, with the lower terrain
left in place. Mission Street uses one continuous surface from the
northern approach across both I-280 carriageways and Alemany Boulevard
to the point where the existing grid meets pavement again. Baden and
Paulding retain their rising street grades.

At Lincoln Way the first candidate left a westbound terminal where its
entry region overlapped the eastbound terminal. A straight eastbound
approach could match both portals, leaving the car on ground contact
through the crossing. The westbound terminal moved 4.5 m towards the
crossing to the point where its pavement remains supported and the
approaches are farther apart. The regression tests approach the
eastbound terminal at the centre and on either side.

The first south-eastbound Baden trial went airborne at the handoff. The
contact now reaches the flatter observed pavement before the San Jose
Avenue junction. The fitted peak grades are 14.70% on Baden, 15.31% on
Paulding and between 1.27% and 5.60% on the other eight surfaces. The
terrain-only screen found 338 intervals above 50% grade, down from 364.

**UI.** On the downhill exits the rear chase-camera probe could interpret
the deck the player had just driven down as a ceiling above the old terrain.
The probe now samples the departing road behind the car while it continues
away from the portal. It still detects any separate deck above that road.

Fourteen Chrome keyboard drives on the candidate entered their intended
upper deck and exited onto the adjoining street without airborne travel.
Separate drives in both directions beneath Mission, Baden and Paulding
kept their existing I-280 contacts, used the covered view beneath each deck
and returned to the exterior view. The six lower ground drives on Sunset
Boulevard and Brotherhood Way reached their exits grounded. The largest
vertical clearance measured during an airborne flag on those approaches
was 0.12 m, below the 0.18 m airtime display cutoff. The ground samples
showed no dips deeper than 0.5 m over an 8 m chord.
All 25 fixed-step emoji routes again collected both pickups with identical
driving telemetry to the Bayview release. The Bay Bridge replay completed
in 235.96 s of game time.

The screen uses inventory locations and paved road rows to find candidates.
The drives exercise the selected approaches and exits. Turning between
the bridge and nearby ramps or side streets needs its own test.

- Baseline game manifest SHA-256 `81e31f112ce9808042fc96ca8b7e9d8a868391363f17c21a41f6eed8bb8339dc`.
- Candidate game manifest SHA-256 `7e4042b6ac32613896d9c121cc1cadfb91d8fe799e89225a0e351c62b84707d3`.
- Candidate contact SHA-256 `45787da7d665e81af1290b0f4c7e97fb18b099aba6b0ec534561310499603dc8`.
- April 2023 3DEP index SHA-256 `be5260b336632c63eb1e347a41ed2873f9a46761d9bcb8f1bed8e1a117eb173b`.
- Public Works snapshot SHA-256 `c7b76f2f3d07ba28d2142008d65edf1a466782de89dbca024c0fc7a5156b7d00`.
- SFMTA snapshot SHA-256 `3145a2bd5e6a81a5b4b25620e92c907d0576c5b4e3e76fbfba208c47ba5d982c`.

```bash
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  lincoln-way-sunset-{eastbound,westbound} sloat-boulevard-sunset-{eastbound,westbound} \
  oakdale-avenue-caltrain-bridge-{eastbound,westbound} \
  junipero-serra-brotherhood-{northbound,southbound} \
  mission-street-i280-viaduct-{northbound,southbound} \
  baden-street-i280-bridge-{northwestbound,southeastbound} \
  paulding-street-i280-bridge-{eastbound,westbound} \
  i280-{northbound,southbound}-under-{mission,baden,paulding}
```

## Cortland, Bacon and Paul beneath US 101

**World and Data.** Three lower city streets now run beneath the existing
US 101 decks on their own contact strips. The fit follows April 2023 3DEP
pavement returns on both approaches. It interpolates between the observed
sections through the aerial shadows. These observations establish the
approach elevations, with clearance and the underside of the freeway still
unmeasured. At Cortland, the lower lanes also intersected an inferred
building footprint. The bake opens that part of the volume at street level
and retains the inferred volume above it. The ground refinement tapers into
the original terrain. Both tiles intersecting Bacon Street carry its entire
contact, including its portals.
[SFMTA clearance inventory](https://data.sf.gov/Transportation/Clearance-Heights-Bridges-and-Underpasses/eb2x-6eay/about_data).

**Physics and UI.** On the baseline, Cortland eastbound stalled after
11.04 m flagged airborne and Cortland westbound collided with the inferred
volume. Bacon completed both ways but lost contact for 27.78 m westbound
and 17.08 m eastbound. At Paul, the game flagged 18.33 m westbound and
13.83 m eastbound, with vertical clearance below 0.08 m in both directions.

| Lower contact   | Length | Observed stations | Longest interpolated span | Peak fitted grade |
| --------------- | -----: | ----------------: | ------------------------: | ----------------: |
| Cortland Avenue |   70 m |           11 / 25 |                   43.75 m |             8.50% |
| Bacon Street    |   90 m |           13 / 31 |                   30.00 m |             3.03% |
| Paul Avenue     |   78 m |           14 / 27 |                   42.00 m |             5.67% |

Chrome keyboard drives in both directions enter the intended lower contact,
use the covered camera below US 101 and return to exterior view on the
adjoining street. The tests require grounded travel between the portals.
On the ground approaches and departures they compare any airborne frames
with the actual detailed ground height, using the game's 0.18 m airtime
display cutoff. The six candidate runs showed zero airborne travel between
portals and less than 1 mm of measured clearance on the departures. The
tile test samples three lateral positions on each street for collisions and
verifies at least 5 m between the game road surfaces at each crossing sample.

The 25 fixed-step emoji routes repeated with identical driving telemetry.
The Bay Bridge replay took 235.96 s of game time through Fremont and back
onto I-80 at Fourth. The terrain-only city screen still counted 338
intervals above 50% grade. The tests at these three underpasses cover
straight-through travel in both directions. Adjacent turns and freeway
ramp joins require their own drives.

- Baseline Chrome manifest SHA-256 `81e31f112ce9808042fc96ca8b7e9d8a868391363f17c21a41f6eed8bb8339dc`.
- Parent game manifest SHA-256 `7e4042b6ac32613896d9c121cc1cadfb91d8fe799e89225a0e351c62b84707d3`.
- Candidate game manifest SHA-256 `ecd960a1b1fae3c1f23c67f96db0bc29d457dfe9037f553e497e3513728f0c88`.
- Candidate contact SHA-256 `54fc0320ab08cec35fd4d2a54b012ff1aeeb9885a46df7874651f491ed9f84d5`.
- April 2023 3DEP index SHA-256 `be5260b336632c63eb1e347a41ed2873f9a46761d9bcb8f1bed8e1a117eb173b`.

```bash
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  cortland-avenue-under-us101-{eastbound,westbound} \
  bacon-street-under-us101-{eastbound,westbound} \
  paul-avenue-under-us101-{eastbound,westbound}
```

## 22nd Street beneath I-280 in Dogpatch

**World and Data.** At 22nd Street, the prior terrain dipped to 10.57 m
NAVD88 between visible pavement at 13.11 m and 14.35 m. The I-280 deck
above it measured about 24 m. April 2023 pavement returns support 10 of 23
stations in the new 65 m contact. The fit interpolates 41.36 m in the
aerial shadow and peaks at a 4.02% grade. The one-metre terrain refinement
extends the original rectangle in tile `808f7fb4`. The bake verifies that
every original detailed sample is retained, the expanded rectangle meets
the coarse terrain on all four sides and the freeway contacts remain
above the street. The inferred lower roadway through the shadow has
gameplay validation. Physical clearance needs a separate survey.
[Public Works roadway structure inventory](https://data.sf.gov/w/x5tj-txci/ikek-yizv).

**Physics and UI.** Baseline Chrome drives registered 19.37 m of airborne
travel westbound and 8.55 m eastbound. Maximum measured clearance above
the terrain was 0.23 m westbound and 0.13 m eastbound. Both candidate
keyboard drives completed with zero airborne travel, stayed on the lower
contact under I-280 and reached exterior view on the adjoining street.

- Baseline Chrome manifest SHA-256 `81e31f112ce9808042fc96ca8b7e9d8a868391363f17c21a41f6eed8bb8339dc`.
- Parent game manifest SHA-256 `ecd960a1b1fae3c1f23c67f96db0bc29d457dfe9037f553e497e3513728f0c88`.
- Candidate game manifest SHA-256 `2cb2d0ad4fcb578a5334f65ec2c2058e36463f62ee964e6149a78e859c8cbd36`.
- Candidate contact SHA-256 `3a9c76bb1dda33a4c17a1b35501c2aa07f8e36cd5988985dc0f16582347fdda1`.
- April 2023 3DEP index SHA-256 `be5260b336632c63eb1e347a41ed2873f9a46761d9bcb8f1bed8e1a117eb173b`.

```bash
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  dogpatch-twenty-second-under-i280-{westbound,eastbound}
```

## Hyde Street above the Broadway Tunnel

**World and Data.** The centreline screening drove six inventory sites in
both directions on the 22nd Street release, at about 22 mph where the
steering allowed. The drives recorded grounded longitudinal grades and
flagged dips deeper than 0.5 m below an eight-metre chord.

| Crossing                           |          Baseline airborne distance by direction | Peak driven grade | Deepest flagged dip |
| ---------------------------------- | -----------------------------------------------: | ----------------: | ------------------: |
| Hyde Street above Broadway         |            7.83 m northbound, 21.27 m southbound |            32.07% |              0.61 m |
| Bryant Street at Delancey          | 26.55 m south-westbound, 20.53 m north-eastbound |            28.28% |                 0 m |
| Sickles Avenue beneath I-280       |   8.09 m north-westbound, 5.26 m south-eastbound |            20.05% |                 0 m |
| San Jose Avenue at Sickles         | 21.49 m south-westbound, 14.16 m north-eastbound |            29.07% |                 0 m |
| Jerrold Avenue beneath the railway |                           0 m in both directions |             3.05% |                 0 m |
| Mariposa Street above the railway  |                           0 m in both directions |             3.74% |                 0 m |

At Hyde, the 3DEP pavement return is approximately 62 m NAVD88 at
Broadway. The terrain placed the street as low as 58.53 m in the tested
lane. The 37 m replacement contact follows the observed upper street
between the existing approaches. The fitted peak grade is 4.58%. Both
Broadway Tunnel bores still use their independent lower road contacts,
near 55.14 m at the crossing. The height difference describes the game
road surfaces. A survey of the underside would establish physical
clearance. The next review covers the grades and levels at Bryant,
Sickles and San Jose.
[Public Works roadway structure inventory](https://data.sf.gov/w/x5tj-txci/ikek-yizv).

**Physics and UI.** Chrome keyboard drives in both directions stayed on
the upper Hyde Street contact between the portals and returned to open
street pavement. The westbound and eastbound Broadway drives selected
their original bores, used the tunnel camera and returned to the
exterior view. The tile regression checks that the Hyde and Broadway
contacts give different heights at the same locations.

- Baseline game manifest SHA-256 `2cb2d0ad4fcb578a5334f65ec2c2058e36463f62ee964e6149a78e859c8cbd36`.
- Candidate game manifest SHA-256 `dd6011ab56d56560349d18a8c3829594c640aee5ae8e733137dc1be437469831`.
- Candidate contact SHA-256 `3c55add615cb6e1cd90501ab0ddcb9bb4043e0544ea7cb1f639cfcfb035cc167`.
- April 2023 3DEP index SHA-256 `be5260b336632c63eb1e347a41ed2873f9a46761d9bcb8f1bed8e1a117eb173b`.
- Public Works snapshot SHA-256 `c7b76f2f3d07ba28d2142008d65edf1a466782de89dbca024c0fc7a5156b7d00`.

```bash
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  hyde-street-{northbound,southbound} broadway-{westbound,eastbound}
```

## Bryant Street viaduct beside Delancey Street

**World and Data.** At the viaduct, the terrain falls to 6.47 m NAVD88
while the April 2023 pavement observations put Bryant Street at 11.44 m
at the same station. Across the 205 m contact, all 70 fitted stations have
survey support and the peak fitted grade is 7.81%. The two tile owners
carry identical copies of the whole viaduct and its street portals. The
lower ground beside the viaduct retains its earlier elevations.
[Public Works roadway structure inventory](https://data.sf.gov/w/x5tj-txci/ikek-yizv).

**Physics and UI.** On the baseline, the two Chrome runs flagged 26.55 m
and 20.53 m of airborne travel. Both candidate drives use the full contact
through the tile seam and return to the adjoining street. The test
measured zero airborne travel within the new contact and less than
3 mm of clearance on the ground departures.

The adjoining Delancey Street drives completed on the ground route in
both bakes. They flagged 42.36 m and 24.66 m of airborne travel on the
parent and 41.83 m and 18.75 m on the candidate. Both games use identical
Delancey road rows and terrain. Its existing elevation behaviour still
requires separate review.

- Parent game manifest SHA-256 `dd6011ab56d56560349d18a8c3829594c640aee5ae8e733137dc1be437469831`.
- Candidate game manifest SHA-256 `5c7886f4972799a151dce430453efcd7c20621aad3d93344caa103dec338ace1`.
- Candidate contact SHA-256 `e1d1eeb8fd61a1fa6828d6fa93e4f77d03de419a21df3c84cf9394970edb584a`.
- April 2023 3DEP index SHA-256 `be5260b336632c63eb1e347a41ed2873f9a46761d9bcb8f1bed8e1a117eb173b`.

```bash
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  bryant-street-{southwestbound,northeastbound}
```

## Sickles and San Jose avenues beneath I-280

**World and Data.** The Sickles crossing and the two mapped San Jose Avenue
alignments now use separate lower road contacts. The 82 m, 108 m and 102 m
profiles reach the existing street pavement at both ends. Survey returns
support the approaches. The road levels between those observations are
inferred across the survey shadows beneath the freeway. The terrain grid
uses the nearest pavement where the three refinements overlap. The existing
I-280 contacts remain above the streets, with 6.7–7.0 m of separation between
the game road surfaces at the regression points. This measures the game
surfaces. Physical clearance would require observations of the underside.

**Physics and UI.** The original screening recorded 5.26–8.09 m of airborne
travel on Sickles and 14.16–21.49 m on the tested San Jose alignment.
Chrome now follows each of the three lower contacts in both directions
without airborne travel within the crossing. The camera moves into covered
view under I-280 and returns to the exterior after the street departure.
Separate Chrome drives follow both freeway carriageways over the same area.

- Parent game manifest SHA-256 `5c7886f4972799a151dce430453efcd7c20621aad3d93344caa103dec338ace1`.
- Candidate game manifest SHA-256 `a92deef38b6575aaa793f415776092cb09d82675fd7c38347944556c8be979ea`.
- Candidate contact SHA-256 `3f1f3f4596e86de5e92d44299d848e8259ab3415f78315da9f1182cac79d5a62`.
- April 2023 3DEP index SHA-256 `be5260b336632c63eb1e347a41ed2873f9a46761d9bcb8f1bed8e1a117eb173b`.

```bash
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  sickles-{northwestbound,southeastbound} \
  san-jose-{first,second}-{southwestbound,northeastbound} \
  i280-sickles-{northbound,southbound}
```

## Delancey Street beside the Bryant viaduct

**World and Data.** The 141 m Delancey contact uses the lower April 2023
pavement observations. At station 144 m the pavement returns are at 5.27 m
NAVD88 and the parent terrain is at 5.83 m. At station 198 m the returns are
at 6.30 m and the parent terrain is at 7.18 m. The fitted peak grade is
2.35%. A one-metre refinement covers the lower pavement and preserves the
earlier terrain refinement in the same tile. The adjacent tile carries
clipped approach paint while both Delancey portals stay in `8085807c`.

**Physics and UI.** Both keyboard drives traversed the entire new contact
with zero airborne travel within it, returned to the adjoining streets and
finished grounded with the exterior camera. The northeast drive recorded
4.4–7.0 m of brief airborne travel on the older terrain beyond the portal.
It remained on Delancey and landed on the same street. Both Bryant viaduct
drives also retained their upper surface and completed their street joins.

- Parent game manifest SHA-256 `a92deef38b6575aaa793f415776092cb09d82675fd7c38347944556c8be979ea`.
- Candidate game manifest SHA-256 `3fb552c32b760ebf3cfd7655944f83bdde32c207eae36e051f06c3261aa86d1c`.
- Candidate contact SHA-256 `780d69cb9a52ee8528634f3ddd640fc4b7b9d9356f3cc854365ad99a94334223`.
- April 2023 3DEP index SHA-256 `be5260b336632c63eb1e347a41ed2873f9a46761d9bcb8f1bed8e1a117eb173b`.

```bash
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  delancey-{southwestbound,northeastbound} bryant-street-{southwestbound,northeastbound}
```

### Northeast Delancey approach

**World and Physics.** The 49 m extension reaches the flatter pavement
near the junction and increases the lower contact to 190 m. Each of the
two tiles carries the complete contact and both portal definitions. At the
previous dip near station 54 m, the new contact is 4.04 m NAVD88. A
survey-supported contact carries the car across the tile ownership boundary.

The northeast Chrome drives all finished on ground. They recorded 0–1.65 m
of airborne travel beyond the extended contact across three runs, with zero
airborne travel on the contact itself. The southwest test still reached the
adjoining street with zero airborne travel within the contact.

- Game manifest SHA-256 `dc95d093e0922fe73ece71d4f124649fc4f470716f8ba57d97e83ad88799d36a`.
- Contact SHA-256 `fa26715b8c6230469129d29bee106e812160f3e73a0bf658eaaf6ee3eb9b6c0e`.

## Federal tunnel inventory cross-check

**World and Data.** The 2026 National Tunnel Inventory lists ten entries in
San Francisco County. The four separate Battery and Main Post bores account
for four of the ten. The Geary Boulevard entry is the Masonic crossing.
[FHWA National Tunnel Inventory download](https://www.fhwa.dot.gov/bridge/inspection/tunnel/inventory/download.cfm).

**Physics and UI.** The Chrome drives used the published Sickles game bake
above. Each inventory entry had a grounded tunnel observation and a grounded
exterior finish. On the Branch East ramp, the upper I-280 drive separately
retained its exterior camera. The Stockton street-level probe also retained
its exterior camera. These measurements cover game contact, portal handoffs
and camera state along the tested lanes.

| FHWA ID    | Inventory entry              | Chrome drive                             |
| ---------- | ---------------------------- | ---------------------------------------- |
| `34 0004`  | Yerba Buena Crossing         | I-80 lower eastbound and upper westbound |
| `34 0016`  | Presidio, SR 1               | MacArthur northbound and southbound      |
| `34 0080`  | Branch East UC               | Northbound I-280 ramp to San Jose Avenue |
| `34 0161L` | Battery, US 101 southbound   | Presidio southbound                      |
| `34 0161R` | Battery, US 101 northbound   | Presidio northbound                      |
| `34 0163L` | Main Post, US 101 southbound | Presidio southbound                      |
| `34 0163R` | Main Post, US 101 northbound | Presidio northbound                      |
| `34C0006`  | Broadway Street              | Eastbound and westbound                  |
| `34C0040`  | Geary Boulevard              | Eastbound and westbound at Masonic       |
| `34C0044`  | Stockton Street              | Northbound and southbound                |

- FHWA XML SHA-256 `c0d53e43ad21b9a0ac97b902ca701f97d738649fb667b29958036558102bd6dc`.
- The I-80 upper drive uses the game's automatic controller. The other drives
  use the game's keyboard interface with steering generated by the smoke test.

## I-280 surface screening near Geneva and Alemany

**World and Data.** Public Works structure markers located the crossings.
The browser drives followed the mapped street rows, offset 1.5 m from the
centreline towards the travel lane, at a target of 22 mph. The start and
finish were up to 40 m from the structure projection, within the available
street row. The runs used the 141 m Delancey bake above. Measurements are
from the game's ground contact and steering, including any airtime before
the finish. [San Francisco roadway structures inventory](https://data.sf.gov/w/x5tj-txci/ikek-yizv).

| Street                                          | Chrome runs completing the sampled path | Airborne travel per run |
| ----------------------------------------------- | --------------------------------------- | ----------------------- |
| Geneva Avenue at I-280, two carriageways        | 1 of 4                                  | 1.65–11.36 m            |
| Congdon Street at the Justin Drive overcrossing | 2 of 2                                  | 13.37–22.80 m           |
| San Jose Avenue near Mount Vernon               | 2 of 2                                  | 20.47–20.91 m           |
| Western Alemany Boulevard undercrossing         | 2 of 2                                  | 7.91–16.27 m            |
| Saint Charles Avenue approach                   | 2 of 2                                  | 5.62–8.00 m             |

Three Geneva runs crashed. The April 2023 survey at station 36 on the
two Geneva carriageways gives about 75.65 m NAVD88, 6.1–7.2 m above the
terrain used by those runs. Separate street and freeway levels need contact
checks at both ends of the upper street. The Saint Charles samples cover
the mapped approach on one side of I-280, and the inventory crossing still
needs its alignment verified.

## Congdon Street at the Justin Drive overcrossing

**World and Data.** April 2023 survey returns support all 38 fitted stations
along the approximately 110 m contact. Near the crossing, the upper street
is about 36.4 m NAVD88. The two game freeway surfaces below it are about
29.7 m and 28.9 m at the sampled crossings. The fitted peak grade is 2.54%.
The contact follows the curve onto Justin Drive and joins the existing
pavement on both approaches. The bake carries forward the current Yerba
Buena classified tree tile and its recorded source hashes.

**Physics and UI.** Both Chrome street drives completed with zero airborne
travel on the upper contact and finished grounded on their adjoining streets.
Both I-280 drives completed on the lower surface with zero airborne travel.
The northbound camera still saw the nearby Alemany overpass at the finish.
The measured separation describes the playable pavement elevations. Physical
underside clearance requires its own survey.

**Goals.** All 25 fixed-step route results matched when the route alignment
was held to the parent tiles from `4cc496add657a6f2d8227cc902e3339377db0f5b`.
The fresh route planner took a longer after-exit path on the `i280-south-city-span`
course and struck a tree. The same course reached both emoji on the original
alignment using the new bake's terrain and collision geometry. The audit
runner's `--alignment-assets` option accepts the parent `public_html/src/sf`
directory and records the alignment manifest alongside the tested manifest.

- Game manifest SHA-256 `1d40f883d97f4f386448cda5617de14dcbdd095783ca0039499b61847ea0896e`.
- Contact SHA-256 `7b06f43b88a6bd3b17f9e7c66449d6df67ab88ed1299fb8eb5cd4d6c550b32c7`.
- April 2023 3DEP index SHA-256 `be5260b336632c63eb1e347a41ed2873f9a46761d9bcb8f1bed8e1a117eb173b`.

```bash
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  congdon-{northbound,southbound} i280-under-congdon-{northbound,southbound}
```

## Geneva Avenue deck and I-280 ramp loop

**World and Data.** Separate seven-metre contacts follow the eastbound and
westbound pavement above I-280. April 2023 survey returns support all 25
eastbound and 26 westbound fitted stations. The peak fitted grades are
3.02% and 4.05% respectively. The local road stands about 6.3–7.1 m above
the baked freeway surfaces at the tested crossing points. Physical underside
clearance requires its own observations.

The two ends on Geneva use the shared ground levels of 76.22 and 74.79 m
NAVD88. Four directed joins connect the two decks to their corresponding
off-ramps and on-ramps. The portal uses the same transverse boundary as the
baked pavement. Where several ground mouths meet at the same point, the
car's heading selects a clearly aligned mouth.

**Physics and UI.** One manual-control Chrome run left southbound I-280,
traversed the eastbound Geneva deck and rejoined northbound I-280. A second
run crossed westbound from northbound I-280 to the southbound on-ramp.
Both drives acquired every surface grounded and recorded zero airborne
travel. The lower freeway retains separate through routes beneath Geneva.

- Game manifest SHA-256 `18caec1cb37d15fcbc19d5d1af5f8f0f95ca008ad6033f6c370825b1cc69b6c4`.
- Contact SHA-256 `357ae56eca377a77a996bbec410027c57e4748b249a12bacdf8717588bde174a`.
- April 2023 3DEP index SHA-256 `be5260b336632c63eb1e347a41ed2873f9a46761d9bcb8f1bed8e1a117eb173b`.

```bash
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  geneva-avenue-{eastbound,westbound} geneva-{northbound,southbound}-through \
  geneva-exit-and-reentry geneva-westbound-exit-and-reentry
```

## San Jose Avenue at Mount Vernon

**World and Data.** The game bakes three street approaches and three curved
connections in tile `808f7c2c`. April 2023 survey returns supported all 59
approach fit stations. The same survey supported the 33 curve samples, with
a maximum absolute median residual of 1.9 cm at the sampled positions.
The fitted peak street grades are 11.49% on north San Jose, 3.60% on
south San Jose and 12.33% on Mount Vernon. The upper streets are 8.6–9.0 m
above the two I-280 game surfaces at the sampled crossings. Physical
underside clearance requires its own survey.

**Physics and UI.** Chrome completed both San Jose directions, the four
turns between San Jose and Mount Vernon, and both freeway directions beneath
them. Each street drive finished on the adjoining pavement after leaving
the baked surface. Both freeway runs kept the covered view on the lower
level at their finish. The turn probes steered towards the inside of their
lane before reaching the shared seams. Two public runs briefly raised the
physics airborne flag after the north ground handoff. A fixed-step probe
at that handoff reproduced less than 5 mm of clearance. The regression
check requires continuous contact through the crossing and measures
clearance on the departing street separately.

**Goals.** All 25 fixed-step course results matched the Geneva release
with the route alignment held to `4cc496add657a6f2d8227cc902e3339377db0f5b`.
The Bay Bridge replay reached Fremont and rejoined I-80 at Fourth Street
in 235.96 seconds of game time.

- Game manifest SHA-256 `151ae4163681a1bc063d425b04c71e4d24d9ec051af49969005cd5792185ddee`.
- Contact SHA-256 `54615f58eb974ccc6bada467ceebfcc8dae3f9201743a957b8ee760c1f6e1584`.
- April 2023 3DEP index SHA-256 `be5260b336632c63eb1e347a41ed2873f9a46761d9bcb8f1bed8e1a117eb173b`.

```bash
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  mount-vernon-{southwestbound,northeastbound,westbound,eastbound,from-south,from-west} \
  i280-mount-vernon-{northbound,southbound}
```

## Saint Charles Avenue and western Alemany Boulevard

**World and Data.** Caltrans identifies the Saint Charles span as bridge
`35 0176` at the county line. The 133 m game contact follows the DataSF
alignment to its endpoint and uses an SFMTA 54 route shape point to locate
the last approach towards Daly City BART. April 2023 pavement returns
support all 46 sampled stations. The fitted peak grade is 5.58%.
[Caltrans District 4 bridge log](https://dot.ca.gov/-/media/dot-media/programs/maintenance/documents/f0009151-logd04-a11y.pdf) ·
[SFMTA 54 Felton](https://www.sfmta.com/routes/54-felton) ·
[SFMTA GTFS data](https://data.sf.gov/Transportation/SFMTA-General-Transit-Feed-Specification-GTFS-Prod/dni7-qpv3).

The western Alemany Boulevard undercrossing is bridge `34 0026` in the
same inventory. It has separate 102 m eastbound and 80 m westbound game
contacts. The survey observed 19 of 35 eastbound and 12 of 28 westbound
stations. The floor is inferred for 51.00 m eastbound and 50.37 m westbound
between the visible pavement on either side of the freeway. Peak fitted
grades are 8.25% and 5.10% respectively. A one-metre terrain patch follows
the lower contacts and leaves the existing freeway surfaces above.

The sampled game pavements are 6.6–7.4 m apart. That separation is measured
between the playable road surfaces. Physical clearance remains unmeasured.

**Physics and UI.** Chrome drove both street directions at each crossing,
both I-280 mainlines at each site and the southbound exit below Saint
Charles. All nine candidate drives used their intended surfaces without
an airborne sample. The public game repeated all nine crossings and
finished grounded. The northbound Saint Charles street drive recorded
0.55 mm of clearance after the deck handoff, with continuous contact on
the deck itself. The Saint Charles freeway drives entered and cleared
the covered view. On lower Alemany, the rear camera still saw the freeway
deck at the finish.

**Goals.** The 25 fixed-step course results match the Mount Vernon release
on the same archived alignment. The Bay Bridge replay finished through
Fremont and Fourth Street again in 235.96 seconds of game time.

- Game manifest SHA-256 `42d6ffb1309d6e668ef0066e36151d0147a01c700f8cd8352a99d14616f78f92`.
- Contact SHA-256 `87dafe7398c427a018bb0d5424f704da1217c39bec4e1ef41870d69eb6b940c5`.
- April 2023 3DEP index SHA-256 `be5260b336632c63eb1e347a41ed2873f9a46761d9bcb8f1bed8e1a117eb173b`.
- SFMTA GTFS snapshot SHA-256 `533768f351d72ac647c468dc00a5004f61f2e11bc3706d862b914cc0f85fc0b0`.

```bash
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  saint-charles-{northbound,southbound} saint-charles-i280-{northbound,southbound,south-exit} \
  western-alemany-{eastbound,westbound} western-alemany-i280-{northbound,southbound}
```

## Packaged contact and remaining city crossings

**World and Data.** The southwestern release contains 319 distinct
playable surfaces and 269 joins across 160 tiles. The asset test checks
that each surface has identical geometry in both terminal owners and
every tile containing one of its sampled positions. Each terminal has
a declared ground portal or a join at the same position. The two outer
Bay Bridge endpoints sit at the map edge over water.

**Physics.** Chrome drove the following seven crossing alignments in both
directions using the same baked manifest listed above. It started on the
mapped street near each San Francisco Public Works structure marker and
followed the mapped drive lane 1.5 m from the centreline, at a target of
22 mph. The start and finish were up to 40 m from the projected marker.
The reported clearance is the largest sampled distance between the car
and the game terrain during an airborne flag. All 14 drives reached the
finish. Twelve finished grounded. The other two finished with airborne
flags and less than 1 mm of clearance. No driven sample exceeded the
0.5 m dip threshold over an eight-metre chord.
[Public Works roadway structure inventory](https://data.sf.gov/w/x5tj-txci/ikek-yizv).

| Alignment                                        | Public Works marker | Maximum sampled clearance |
| ------------------------------------------------ | ------------------- | ------------------------: |
| Cesar Chavez Street above the railway            | 57                  |                    2.3 mm |
| Third Street at Mission Creek                    | 74                  |                    0.3 mm |
| Mason Street above Broadway                      | 250C                |                    6.4 mm |
| Ingerson Avenue at the sidehill viaduct          | 238                 |                    6.4 mm |
| Alemany Boulevard above Junipero Serra Boulevard | 195                 |                    5.3 mm |
| Junipero Serra Boulevard below Alemany Boulevard | 195                 |                    0.5 mm |
| Evans Avenue at the bridge                       | 126                 |                    0.3 mm |

These drives establish playable passage on the tested game approaches.
The comparison uses the game's terrain grid. It does not establish
physical bridge clearances or coverage of every inventory record.

- Public Works snapshot SHA-256 `c7b76f2f3d07ba28d2142008d65edf1a466782de89dbca024c0fc7a5156b7d00`.

## Citywide baked road contact screen

**World, Physics and Data.** `audit-baked-road-contact.mjs` reads the
packaged assets and uses the game's contact triangles, tyre support,
vehicle hull, scenery collision and overhead tree clearance. It samples
the interior of each surface at intervals of up to 2 m, with 0.75 m
between lateral positions. A sampled route can continue straight or
shift one lateral step between stations. The audit also checks the
declared same-level joins within 2 mm and compares ground portals with
the surrounding terrain within 15 cm.

```bash
node tools/sf-drive/audit-baked-road-contact.mjs
node tools/sf-drive/audit-baked-road-contact.mjs /path/to/candidate/tiles
```

| Bake                                                 | Surfaces | Sampled vehicle poses | Ground portals | Declared joins | Surfaces with centreline collision samples |
| ---------------------------------------------------- | -------: | --------------------: | -------------: | -------------: | -----------------------------------------: |
| Published Hester package                             |      320 |               686,759 |            228 |            269 |                                          7 |
| Alana, Presidio, Industrial and Cesar Chavez preview |      323 |               688,011 |            234 |            269 |                                          6 |

Every surface in both packages retained a connected sequence of sampled
positions. The portal and join checks stayed within their tolerances.
The preview removed the southbound US 101 centreline obstruction above
Industrial Street. Its remaining collision samples were as follows.

| Surface                                | Centreline collision samples | Scenery                  |
| -------------------------------------- | ---------------------------: | ------------------------ |
| Southbound Main Post Tunnel            |                           14 | Inferred overhead volume |
| Geary–Masonic underpass                |                            9 | Three inferred trunks    |
| Northbound US 101 Potrero approach     |                            5 | Two inferred trunks      |
| MacArthur Tunnel                       |                            3 | One inferred trunk       |
| Northbound US 101 Potrero exit         |                            2 | One inferred trunk       |
| East Dogpatch ramp to northbound I-280 |                            1 | One inferred trunk       |

With Chrome steering along the centreline, the Geary–Masonic and
MacArthur drives crashed at the flagged trees. The southbound Main Post
drive completed with tunnel contact. Its existing left-lane clearance
remains relevant to the static centreline result. The screen samples
scenery on each surface independently. Browser drives cover rail impacts,
camera transitions, loading and steering between surfaces. The structure
inventory provides the separate check for roads absent from the package.

- Published manifest SHA-256 `eddfbe36c207b3dfb69cb502cb8aea0073ca3b28e37ff35db52f1f4f64875655`.
- Local preview manifest SHA-256 `7d3ce4dceb0aba016b23bca9e92aee210b67770ef5961c21d708311be6024acf`.

### Driving across baked joins and portals

**World, Physics and Data.** The optional driving pass starts a car before
each permitted join and at both usable sides of each ground portal. It
steps the game contact solver at 120 Hz, trying other lanes or speeds
when the first approach fails. Exit-only portals are exercised from the
road. A ground exit that continues onto a declared, separately tested
road is counted as a continuation. This pass keeps all tiles loaded and
checks tyre contact and rails. Scenery remains in the static audit and
in the browser drives. With `--drive-links`, the command returns a nonzero
status if a tested join or portal has no successful approach.

```bash
node tools/sf-drive/audit-baked-road-contact.mjs --drive-links
node tools/sf-drive/audit-baked-road-contact.mjs --drive-links /path/to/candidate/tiles
```

| Bake                     | Directed joins driven | Ground checks | Reached terrain or entered the road | Continued onto another road |
| ------------------------ | --------------------: | ------------: | ----------------------------------: | --------------------------: |
| Published Hester package |               280/280 |           447 |                                 446 |                           1 |
| Local Merchant Road bake |               290/290 |           490 |                                 488 |                           2 |

The local bake needed a lane or speed adjustment for 33 directed joins
and six ground handoffs. Its two continuations were Doyle Drive onto
the northbound Golden Gate approach and the Hester exit onto Bayshore
Boulevard at Le Conte. Both passed in Chrome. On the southbound bridge
approach, the centreline keyboard guide took the SR 1 branch in one
run. The Doyle Drive guide now approaches in the left lane. The right
lane has its own SR 1 drive.

- Local Merchant Road manifest SHA-256 `5b206c95ee87664339e22f06b60bcfcb2f19f701b4f191151235d3fb4a43ad0e`.

### Driving each packaged span

**World, Physics and Data.** `--drive-spans` drives individual spans in the
directions admitted by the game's road network and ground portals. The 120 Hz
replay uses the game's tyre support, baked rails and scenery collision checks. It tries the
centreline first, then slower speeds and offsets up to 1.5 m where needed.
It starts and finishes 2.58 m inside the ends so the chassis fits on the span.
Use `--drive-links` in the same run to cover the joins and ground handoffs.

```bash
node tools/sf-drive/audit-baked-road-contact.mjs /path/to/candidate/tiles --drive-links --drive-spans
```

| Surface type | Spans | Directional drives completed |
| ------------ | ----: | ---------------------------: |
| Bridge deck  |   163 |                          214 |
| Ramp         |   176 |                          202 |
| Tunnel       |    21 |                           30 |
| Total        |   360 |                          446 |

The published Hester package completed 391 of its 394 directional drives.
Scenery stopped both directions of the Geary–Masonic tunnel and one
southbound US 101 span between Silver Avenue and Industrial Street. All
280 directed joins and 447 ground checks completed, including one
continuation. The newer preview cleared all three routes.

The 160-tile preview passed all 316 directed joins. Of 502 ground handoff
checks, 499 reached terrain or entered the expected span, and three continued
onto a separately tested road. The 704,400 sampled chassis poses produced no
static flags. The two unconnected terminals are the eastbound end and
westbound start where the East Span reaches the map boundary.

The first replay completed every span and recorded three tree scrapes. The
car's centre was 0.12–0.14 m lower than the pavement where the hull met each
trunk. At Presidio Marina, Tenth–Bryant and I‑280 at Sixth Street, the clipped
trunks ended below that pavement. Looking up the supporting deck at the hit
point cleared all three contacts. The repeat finished all 446 drives without
a scenery scrape. Eighteen drives used a speed or lateral adjustment. The
game still checks the clipped trunks against the chassis below the deck.

| Site            | Pavement returns within 0.9 m | Returns in the car-height band | Chrome keyboard drive |
| --------------- | ----------------------------: | -----------------------------: | --------------------- |
| Presidio Marina |                           186 |                              0 | No scrape or lift     |
| Tenth–Bryant    |                           100 |                              0 | No scrape or lift     |
| I‑280 at Sixth  |                           160 |                              0 | No scrape or lift     |

The 2023 survey samples support the local pavement height. The first two
sites also had returns above the car-height band. The street tree inventory
had one nearby record at Sixth Street. These observations support keeping the
trees in the scene. [USGS San Francisco LiDAR survey](https://www.fisheries.noaa.gov/inport/item/73386/full-list), [DataSF street tree inventory](https://data.sf.gov/City-Infrastructure/San-Francisco-Street-Tree-Inventory/tkzw-k3nq).

```bash
node tools/sf-drive/smoke-presidio-tunnels.cjs \
  presidio-marina-deck-tree-clearance tenth-bryant-deck-tree-clearance i280-sixth-deck-tree-clearance
```

The runs measure the packaged game surfaces and supported travel directions.
Roads absent from the package require an inventory comparison, and the replay
keeps all tiles loaded. The browser tests exercise normal tile loading and
keyboard driving. Field clearances require their own survey.

- Local manifest SHA-256 `9b32ea6d68875d084fbdc0f062ae2080e32c2d44b62fed8e06be4e642010b6f1`.
- Published Hester manifest SHA-256 `eddfbe36c207b3dfb69cb502cb8aea0073ca3b28e37ff35db52f1f4f64875655`.

On the same preview, a continuous Chrome keyboard drive took manual control
at 49.7 mph on the Bay Bridge West Span. It used the Fremont exit, crossed
the city streets and entered westbound I‑80 at Fourth Street. The sampled
speed was 46.3–54.3 mph on the span and 10.3–54.3 mph across the full route.
There was no sampled airtime or crash. An earlier pass reached the finish
with an 8.8 mph minimum. The repeat took control earlier and held throttle
through the city turns. These figures use the game's driving HUD samples.

A separate Chrome drive started from a checkpoint farther up the Fremont
exit. The driver steered through the open left edge. The air card recorded
1.3 seconds, and the car touched down on the Folsom Street terrain after
dropping about 8.5 m. The car continued under manual control.

### Repeating the emoji courses on the preview

**Physics, Goals and Data.** The 120 Hz emoji-course driver repeated the
20 pairs on the same preview. It retained the 23 mph street and 40 mph
highway targets. The southbound I‑280 course included the new mainline
section at the Alemany fork. The replay recorded each outcome and kept
running after a blocked course.

Eighteen courses collected both emojis. They and the two partial drives
covered 48.03 km in 76 min 32 s of simulation. No driven sample met the
0.5 m dip threshold over an eight-metre chord. Of 44 crossed tile seams,
the largest single step was 10.7 cm on the Presidio course. The steepest
driven course by p95 was US 101 to Bayview at 20.6%, including the street
approach on Ordway Street.

| Course                     | Stop in the 20-course run      | Follow-up                                                                                                                                                       |
| -------------------------- | ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Southbound I‑280 city span | Entered the on-ramp at Alemany | The separate Chrome drive stayed grounded onto the street at Ellsworth. A later fixed-step guide cleared the exit by 20.1 m and stalled after repeated scrapes. |
| Western Golden Gate Park   | Tree impact on MLK Jr Drive    | A 2.5 m lane adjustment to either side cleared the first tree. Later adjustments reached Stow Lake Drive, where the car hit another tree.                       |

The span audit covers the contact on the packaged structures. These
longer courses also depend on ground waypoints and their steering guides.

- Course plan SHA-256 `5521cddbb5cadc6431a05115906f59ab470d51aeee0c13d7693504fde54cf99a`.

#### Ellsworth, San Bruno and the park lane corrections

Chrome repeated the original I‑280 exit and turned onto Ellsworth Street.
The car travelled 138.4 m from the ramp checkpoint, stayed on the ground
and finished clear of the motorway. The older emoji route had turned back
towards the adjacent entrance at the exit handoff.

The [revised course plan](../../tools/sf-drive/road-audit-preview-continuations-routes.json)
collects the same Fifth Street and Sweeny Street targets. It takes the
left freeway connection to southbound US 101 and exits onto San Bruno
Avenue. The connections between a target and its ramp use the street graph.
The replay completed 8.01 km with 1.48 km after the exit, zero airborne
travel and no driven dip over 0.5 m. A separate Chrome keyboard run
drove the San Bruno ramp and the street approach, finishing grounded
within 4 m of the Sweeny Street marker.

The follow-up park bake removes two unsupported inferred trunk rows at
measured pavement on Martin Luther King Jr Drive and Stow Lake Drive.
Chrome repeated each affected segment. The first changed from a stall
to a grounded finish and the second from a crash to a grounded finish.
The original park course still reached another tree on Martin Luther King
Jr Drive. The revised steering passes to the right of the retained trees
on Martin Luther King Jr Drive and Stow Lake Drive. This drive collected
both park targets without changing those tree rows. Chrome passed both
retained trees and stayed grounded on the sampled approaches.
[Gerardus lane clearance export](https://github.com/banshee-data/gerardus/pull/200).

The rerun collected both pickups on all 20 courses over 52.74 km. No
driven dip exceeded 0.5 m. The successor preview also passed 446
directional span drives, 316 directed joins and 502 ground handoff checks.

```sh
node tools/sf-drive/audit-road-routes.mjs \
  --plan=tools/sf-drive/road-audit-preview-continuations-routes.json \
  --assets=/path/to/preview
```

- Successor manifest SHA-256 `297e0a640bb78b4664c40162c5a2c624b19b5e957f5e70e27296f1a6e404b021`.
- Complete course plan SHA-256 `90e583255b3a50dfbc2da4202dcdb4d65e0f88e32bfcda5064a55eed4018a6bd`.

## Alexander Avenue northbound fork

**World and Physics.** The northbound entrance and the straight city branch share a
pavement seam on Alexander Avenue. Chrome took the straight branch when the sampled
ramp point used to judge steering was farther from the seam. Steering now compares
both centrelines at exactly eight metres along the road, or at the opposite end of a
shorter branch. The test keeps the fork geometry fixed, varies its sample spacing and
reverses one branch's stored direction. It also checks that a tied heading follows the
continuation of the current road.

Chrome repeated the right-lane turn with the same map and keyboard inputs. The car
took the ramp, joined northbound US 101 and finished grounded with zero sampled
airborne travel. The local map passed all 309 directed joins after the change.
The Alexander contact export remains a separate Gerardus release.
[Gerardus Alexander Avenue review](https://github.com/banshee-data/gerardus/pull/197).

- Local northbound candidate manifest SHA-256 `2e07583cdb77318096e74e90d506007955c4a1fc46d05d3585a69327fb909d4d`.

## Full-span Golden Gate exit and return checks

**World and Physics.** The Vista Point candidate extends the northbound
contact past the fork and connects the parking exit ramp back to US 101.
The same package carries the southbound Merchant Road exit and the
Welcome Center return to the northbound bridge approach. Smoke tests
follow the original route through each approach on the public package
and use the additional joins when they are present.
[Vista Point exporter](https://github.com/banshee-data/gerardus/pull/196).
[Merchant Road exporter](https://github.com/banshee-data/gerardus/pull/195).

Chrome used the regular manual takeover and keyboard controls to drive
each full span on the local Vista Point package. All three cases finished
grounded on northbound US 101 with the exterior camera. The Merchant Road
drive used the tunnel camera in the Toll Plaza underpass.

| Route                                   | Seconds | Grounded travel between roads | Sampled airborne travel |
| --------------------------------------- | ------: | ----------------------------: | ----------------------: |
| Presidio through Vista Point on US 101  |   196.9 |                           0 m |                     0 m |
| Presidio off at Vista Point and back on |   305.5 |                       298.7 m |                  2.31 m |
| Marin off at Merchant Road and back on  |   245.3 |                       159.4 m |                     0 m |

The asset tests ran on both the public and the combined candidate packages.
The tests follow the declared through branch past Gorgas, follow the lower
Yerba Buena exit through the Southgate connection when present, and check
each directed join and terrain handoff added to the Golden Gate approaches.

```sh
SF_BASE_URL=http://127.0.0.1:8097 node tools/sf-drive/smoke-presidio-tunnels.cjs \
  golden-gate-full-span-past-vista golden-gate-full-span-vista-and-back \
  golden-gate-full-span-merchant-toll-and-back
```

- Local Vista Point manifest SHA-256 `d2b4d3ec668aa0118e2dda60cda205f3cf6ee0e23441db53af6cad49becf3e6b`.
- Playable contact SHA-256 `5a01eeb500a5540cf55dec39c808839a6106b3e3294d1d90e3d76f4a3e8576a8`.

## Bayshore frontage road from Hester to Third Street

**World and Data.** The northbound Bayshore frontage carriageway uses
DataSF row `2820000` and continues onto Third Street row `233101`. The
game's terrain grid sat 9.1 m below the pavement near the Hester viaduct.
A Chrome drive on the terrain dropped to the lower level and stalled.
The new 417.5 m contact uses 141 survey-supported stations and reaches a
7.38% grade. Its southern portal joins Bayshore pavement near Hester.
Its northern portal reaches open pavement on Third Street after the
parallel US 101 exit. The existing upper approach to the US 101 entrance
follows a separate carriageway.

**Physics and UI.** The first Chrome candidate reached the correct upper
level and hit a rectangular collision volume. Classified returns identify
pavement in that footprint. The corrected bake places the tops of the
inferred overhead volumes below the driving surface and retains their
undersides over the lower roadway. It clears 16 inferred trunk rows at
three locations with pavement returns and no observed obstruction within
the vehicle-height band. These heights describe the game surfaces and
collision volumes. Physical clearance requires a separate survey.

On the candidate, Chrome drove the frontage in 38.6 s and the lower
US 101 exit in 45.3 s. Both reached Third Street grounded, with zero
airborne travel. The lower drive used the covered view on its approach
and ramp, then returned to the exterior view. A tile test checks the
vehicle hull at every packed frontage point and at 1.5 m on both sides.

- Parent manifest SHA-256 `42d6ffb1309d6e668ef0066e36151d0147a01c700f8cd8352a99d14616f78f92`.
- Candidate manifest SHA-256 `eddfbe36c207b3dfb69cb502cb8aea0073ca3b28e37ff35db52f1f4f64875655`.
- Candidate contact SHA-256 `0181c78ad37dcd6c4ea75556d88379104c44aa4bcf41f98a1128ceb6eb14339f`.
- Street source SHA-256 `9ad57b06cfb94eefc9b382e6412a5b99762c61cadbf4d78e9dec5a777cb663e6`.
- April 2023 3DEP index SHA-256 `be5260b336632c63eb1e347a41ed2873f9a46761d9bcb8f1bed8e1a117eb173b`.

```bash
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-presidio-tunnels.cjs \
  bayshore-hester-frontage-to-third bayshore-hester-street-through \
  bayshore-hester-entry-us101 northbound-hester-stays-on-us101 \
  northbound-hester-exit-to-third-street
```

## Three terrain-supported street crossings

**World and Physics.** Chrome drove Mariposa Street over the Caltrain tracks,
Evans Avenue beneath the railway and the Hyde Street sidehill viaduct near
Francisco Street. Each course followed the mapped street from approximately
50 m before the Public Works marker to 50 m after it, 1.5 m to the right of
the centreline in each direction. The keyboard driver targeted 22 mph with
route assist off. Both directions ran on the public Hester package and again
on the local Merchant Road package, for 12 passes.
[Public Works roadway structure inventory](https://data.sf.gov/w/x5tj-txci/ikek-yizv).

| Crossing                         | Public Works ID | Peak grade, public / local | Maximum sampled clearance, public / local |
| -------------------------------- | --------------- | -------------------------: | ----------------------------------------: |
| Mariposa Street over the railway | 70              |              3.78% / 3.76% |                              0.0 / 0.0 cm |
| Evans Avenue beneath the railway | 63B             |              2.65% / 2.50% |                            0.10 / 0.03 cm |
| Hyde Street sidehill viaduct     | 20              |            29.76% / 28.48% |                            4.02 / 5.45 cm |

All 12 drives reached their finish with no crash, tile wait, browser error
or request to Arrow Worker. None exceeded the 0.5 m dip threshold over an
eight-metre chord. Eleven finished with the grounded flag. The northbound
public Hyde drive finished with the airborne flag and no more than 1.5 mm
of measured clearance on that pass. Clearance here compares sampled car
altitude with the game terrain under it. These tests cover the street
approaches and provide no measurements of the underside of a structure.

**Data.** The projected markers in game metres were Mariposa (4091.01,
455.99), Evans (4082.15, −1557.78) and Hyde (1753.77, 4853.33). The courses
sampled the named road rows around each marker at intervals of at most
2.3 m. The finish tolerance was 4 m. Chrome checked the served manifest
hash before each drive.

- Public Works snapshot SHA-256 `c7b76f2f3d07ba28d2142008d65edf1a466782de89dbca024c0fc7a5156b7d00`.
- Public Hester manifest SHA-256 `eddfbe36c207b3dfb69cb502cb8aea0073ca3b28e37ff35db52f1f4f64875655`.
- Local Merchant Road manifest SHA-256 `5b206c95ee87664339e22f06b60bcfcb2f19f701b4f191151235d3fb4a43ad0e`.

### Market Street terrain contact at the sidehill structures

**Physics and World.** The baked terrain uses triangles. At one triangle's
edge, the car could compare a point height against a vertical velocity
estimated across its wheelbase and briefly lose contact. The revised step
uses the same wheelbase at the beginning and end of each tick when testing
downward acceleration. It also allows up to 12 cm of downward wheel travel
before a sudden drop releases support. In both versions the car follows the
baked height on the ground and retains its incoming momentum in flight.

The paired fixed-step drives used the same one-way centrelines, steering
inputs, obstacle index and 160-tile preview. The courses covered about
873 m south-west and 885 m north-east. The targets below are in-game
controller speeds. Distance is counted only on simulation ticks labelled
airborne.

| Direction  | Speed target | Baseline airborne distance | Revised airborne distance |
| ---------- | -----------: | -------------------------: | ------------------------: |
| South-west |       17 mph |                   100.42 m |                    2.30 m |
| South-west |       21 mph |                   118.65 m |                    2.97 m |
| North-east |       17 mph |                   142.52 m |                    0.41 m |
| North-east |       21 mph |                   157.02 m |                    1.42 m |

All eight fixed-step drives finished without a crash. In Chrome, the
north-east drive completed with zero sampled airborne travel. The
south-west drive again crashed near station 778 m, after 2.44 m of
sampled airborne travel. An instrumented repeat reached the finish with
5.23 m of sampled airborne travel. The inconsistent outcome at the
south-west crossing remains open. The revised physics still launches at
a 150 mph crest, falls over a sudden drop and holds the last state at
missing terrain in the automated regressions.

- Game manifest SHA-256 `ce428401fc1edf86a349cc357ac2a6c337e308cad0745fd4dca6e513597badd4`.
- Market Street course SHA-256 `9b3df3a86a2d4d9890fc3cb20419892a44a0d0d054dfcd2654b53cd49592fecb`.
- Baseline game revision `c8e524ff08afe3b01c63d7a596cfdc0f9b7e0ba9`.
- Revised contact module SHA-256 `3965a95a69a45239aae6ee30be3ecad016812235977a72b2ea032790a14050dc`.

The distances describe simulated motion on the game terrain. They do not
establish physical vehicle dynamics or road flatness.

## Terrain-supported crossings on the 160-tile park preview

**World and Data.** A proximity screen compared 293 Public Works, SFMTA and
Caltrans inventory markers with the preview. Thirty-nine markers had no
separately baked contact within 60 m. That count prompted a check of ordinary
road terrain at named crossings. A nearby contact can serve a different
road, and an inventory marker can describe a rail or pedestrian crossing.
Vehicle access and citywide coverage require checks beyond proximity.
[Public Works roadway structures](https://data.sf.gov/w/x5tj-txci/ikek-yizv),
[SFMTA clearance heights](https://data.sf.gov/Transportation/Clearance-Heights-Bridges-and-Underpasses/eb2x-6eay/about_data),
[Caltrans bridge inventory](https://caltrans-gis.dot.ca.gov/arcgis/rest/services/CHhighway/State_Highway_Bridges/FeatureServer/0).

**Physics.** Chrome used keyboard steering with route assist off. Each course
started 43 m before its projected Public Works marker and finished within
4 m of the target 43 m beyond it. The target speed was 22 mph and the initial
line was 1.5 m right of the mapped centreline. The peak grades below use
one-metre samples of the driven game trajectory. All finishes were grounded,
with zero sampled airborne distance and no dips deeper than 0.5 m over an
eight-metre chord.

| Alignment                                        | Public Works ID | Initial finishes | Peak grade by direction |
| ------------------------------------------------ | --------------: | ---------------: | ----------------------- |
| Cesar Chavez Street above the railway            |              57 |           2 of 2 | 3.61%, 3.05%            |
| Jerrold Avenue beneath the railway               |              62 |           2 of 2 | 3.15%, 2.78%            |
| Third Street across Mission Creek                |              74 |           2 of 2 | 1.80%, 2.48%            |
| Evans Avenue at the bridge                       |             126 |           2 of 2 | 4.87%, 5.13%            |
| Alemany Boulevard above Junipero Serra Boulevard |             195 |           1 of 2 | 8.57%, stalled          |
| Junipero Serra Boulevard below Alemany Boulevard |             195 |           2 of 2 | 2.61%, 2.62%            |

The reverse Alemany drive stopped after 0.29 m near the south-west approach.
The vehicle hull intersected tree row 1073 in tile `808f7c4c`. Within 0.8 m
of its base, the April 2023 survey had 47 returns within 0.2 m of the game
ground, 17 from 0.25 to 2 m above it and 226 at least 2 m above it. The
obstruction remains in the bake. [San Francisco LiDAR survey](https://www.fisheries.noaa.gov/inport/item/73386/full-list).

Reverse lines at the centreline, 0.75 m right and 3.75 m right all finished
grounded. The 3.0 m right line also stalled. The centreline and 0.75 m lines
peaked at 5.28% and 4.79% grade. The trials establish a playable route in
both directions and identify a blocked steering line on the approach.

- Game manifest SHA-256 `297e0a640bb78b4664c40162c5a2c624b19b5e957f5e70e27296f1a6e404b021`.
- Initial courses SHA-256 `c0886d142170bef1a94268405e82d3ce642f5ab1e6eb5f52f2b7e55d3566769b`.
- Alternate courses SHA-256 `b537883292928626ca6e33bdbcc697e31bac02c9bfa145d25d9f7fd940439237`.
- Survey node receipt SHA-256 `435b23ddd2ab36f57d8d14d45a9798c788194187ef68e7df11970a5831c8561e`.

The retained run is named `park-inventory-recheck-20261125`. Browser logs,
course coordinates and the survey nodes are kept with the local evidence.

## Staging the city road bake for publication

**World and Data.** The 160-tile package carries the reviewed contacts
through Alexander Avenue, the Presidio and the southern US 101 approaches.
It replaces 25 compressed tiles and their manifest and updates the quest
index to reference the new manifest. The placement fingerprint, all 137
pickup locations and the existing progression key carry forward. The game
reads the same-origin static package and caches it for offline play.
[Gerardus source export](https://github.com/banshee-data/gerardus/pull/200).

**Physics.** The Alexander Avenue check follows both US 101 carriageways
through their new joins and checks the ramps at Vista Point and Alexander
Avenue. At overlapping sampled positions the upper road contacts stand
7.02–7.69 m above the Alexander Avenue road contact. These measurements
describe the game's road surfaces. Inspection of the bridge underside
would establish physical clearance.

Chrome repeated the full Market Street sidehill courses on this package
with a 22 mph target and route assist off. The southwest course finished
grounded after 869.5 m with zero sampled airtime. The northeast course
finished grounded after 883.9 m with 2.69 m of sampled airborne travel.
Neither course flagged a dip deeper than 0.5 m over an eight-metre chord.
The focused inner-lane drive covered 52.9 m at a 20 mph target and finished
grounded with zero sampled airtime.

- Candidate manifest SHA-256 `297e0a640bb78b4664c40162c5a2c624b19b5e957f5e70e27296f1a6e404b021`.
- Gerardus contact SHA-256 `9b97681280229abd0d656a7a9c640e653569715c7c925fc0d3c530bb759e278e`.
- Market Street drive inputs and telemetry are retained under the local run
  name `market-sidehill-current-recheck-20261125`.

## Yerba Buena eastbound exit lane clearance

**Physics.** Fixed-step keyboard controls ran 446 allowed span directions on
the centreline and 0.75 m on either side. Each line used 8 m/s and retried at
5 m/s on failure. Positive offsets lie to the left of the stored span
direction. The Southgate feeder stalled on the positive offset, and the
eastbound exit bore scraped an inferred tree. Chrome reproduced the feeder
stall at game position `(6728.30, 5565.79)` with route assist off. With the
updated static tile, the same browser case reached the deck finish grounded,
with zero airborne travel and no scenery scrapes. A second Chrome run took
the lower tunnel through the exit bore and Southgate to Macalla. It left the
baked surfaces on ordinary pavement, with zero airborne travel and no scrapes.

| Offset from stored centreline | Previous bake | Updated bake |
| ----------------------------: | ------------: | -----------: |
|                       −0.75 m |       445/446 |      445/446 |
|                             0 |       446/446 |      446/446 |
|                       +0.75 m |       445/446 |      446/446 |

The remaining negative-offset result is an unsupported isolated start on the
north Pennsylvania Avenue fork at Dogpatch. That trial starts just inside the
fork and measures start support. A separate Chrome drive entered I-280 at
Fifth Street, followed the Dogpatch off-ramp onto the north fork and reached
Pennsylvania Avenue grounded, with zero airborne travel and no scrapes. It
used the merge line from the preceding ramp. The updated fixed-line runs also
recorded seven completed span drives with scenery scrapes. Three-lane sampling
gives no coverage guarantee for the entire carriageway or every steering trajectory.

**World and Data.** The April 2023 survey sampled game elevations of 51.83 m
at the tunnel mouth and 46.94 m on the feeder. Within 1.35 m of their inferred
tree positions, it had 107 and 333 returns within 0.2 m of pavement,
respectively, and zero from 0.25 to 2 m above it at either position. The survey
also recorded overhead returns at both sites. The classification removes two
inferred tree rows from tile `80858024` and retains earlier tree reviews and
the road contacts. The evidence describes the scanned volume and game
clearance at these samples. [San Francisco LiDAR survey](https://www.fisheries.noaa.gov/inport/item/73386/full-list).

A Chrome offline upgrade downloaded the one changed tile, reached 160 of
160 available offline, retained the collected pickup and loaded the car on
the ground in the upgraded map.

To repeat the span audit, run the following command for each of `-0.75`, `0`
and `0.75`. The Chrome regression is `island-east-southgate-offset` in
`tools/sf-drive/smoke-presidio-tunnels.cjs`.

```sh
node tools/sf-drive/audit-baked-road-contact.mjs public_html/src/sf/tiles --span-lane=0.75
```

- Parent manifest SHA-256 `297e0a640bb78b4664c40162c5a2c624b19b5e957f5e70e27296f1a6e404b021`.
- Updated manifest SHA-256 `cd46e612cdd240b704538133b44ed69a0e100739d4484cb1d466e3a1668ea4df`.
- Gerardus classification SHA-256 `d4c1132c94c61c54dece92523191174f6ab71ad36502ef7c3fa4d323a1fed20b`.

The retained run `off-centre-lanes-evidence-20261125` contains the span
replays, two site receipts and Chrome telemetry on the attached evidence volume.

## Fourth Street westbound merge clearance

**World and Data.** An inferred building face overlapped the westbound
carriageway beyond the Fourth Street entrance. It rose to 12.3 m in the
game, about 1.5 m above the road contact. The overlapping 7.8 m² within
2.8 m of the centreline had 457 class-17 returns within 0.25 m of the deck
across eight metres of travel, and zero class-6 returns above the deck.
The survey also contained sparse unclassified returns. The review caps
only that overlap at 10.3 m, about 0.5 m below the local baked contact.
It retains the adjacent faces and lower collision volume. The cap is
a game scenery correction, and the samples do not measure the structure
beneath the deck. [San Francisco LiDAR survey](https://www.fisheries.noaa.gov/inport/item/73386/full-list).

**Physics.** Chrome drove the same line 0.75 m left of the centreline at
an 8 m/s target with route assist off. The parent bake finished with a
building scrape. The corrected bake finished in 4.5 seconds, grounded
throughout, without a scrape. Chrome also finished the westbound through
drive over Fifth Street and took the separate exit to Harrison. Both
finished grounded with zero airborne travel and no scenery scrapes.
The fixed 0.75 m citywide replay passed all 446 directions, with two
remaining scenery scrapes on Tingley and the Hester entrance collector.

- Parent game manifest SHA-256 `cd46e612cdd240b704538133b44ed69a0e100739d4484cb1d466e3a1668ea4df`.
- Corrected game manifest SHA-256 `f2b4e5efa2258a968a99436d59ff1c12752fdd2d5a7243673ddbc5974bb79e05`.
- Gerardus review SHA-256 `f74c0919f93c614455c32a1de50c3c84841cc17cc98d8a8b1ba13cf945b7a824`.
- Survey node receipt SHA-256 `2caa9d61820cbd8c6fe3a29136c8780b2456beaf04ff19c6f49e4acaa79c9249`.

The browser case is `i80-westbound-fourth-merge-offset` in
`tools/sf-drive/smoke-presidio-tunnels.cjs`. The local evidence run
`fourth-merge-scrape-evidence-20261125` retains its replays and survey nodes.

## Skyline and city clearance inventory follow-up

**World and Data.** The screen matched 293 Public Works, SFMTA and Caltrans
records with coordinates to the Fourth Street release. At 39 records there
was no separately baked road contact within 60 m of the marker. The count
includes railway, pedestrian and equestrian crossings, and a nearby road
contact can belong to another level. The selected runs below follow the
street named in the inventory or the matched roadway at the marker.
[Public Works roadway structures](https://data.sf.gov/w/x5tj-txci/ikek-yizv),
[SFMTA clearances](https://data.sf.gov/Transportation/Clearance-Heights-Bridges-and-Underpasses/eb2x-6eay/about_data),
[Caltrans bridge log](https://dot.ca.gov/-/media/dot-media/programs/maintenance/documents/f0009151-logd04-a11y.pdf).

**Physics.** Chrome used keyboard steering with route assist off. The
Skyline runs targeted 22 mph. The other passes targeted 18–20 mph.
Both Skyline findings refer to the vehicle carriageways over the named
pedestrian or equestrian ways. Peak grade uses one-metre samples of the
driven game trajectory.

| Crossing                         | Inventory        | Grounded finishes |    Peak grade in completed drives |
| -------------------------------- | ---------------- | ----------------: | --------------------------------: |
| Skyline at Lakeside              | Caltrans 34 0002 |               2/2 |                       6.74–16.03% |
| Skyline at Hilltop               | Caltrans 34 0024 |               2/2 |                        5.53–8.90% |
| Gilman at Griffith               | SFMTA 29         |               2/2 |                        2.77–2.80% |
| Grand View near 23rd Street      | SFMTA 32         |               2/2 |                        8.86–8.93% |
| Portola near Marne Avenue        | SFMTA 99         |               2/2 |                       9.08–10.02% |
| Front, Davis and Battery streets | SFMTA 28, 27, 21 |               3/3 |                        0.90–1.49% |
| Lundeen at Crook                 | SFMTA 103        |               2/2 |                        2.68–2.71% |
| Howard at Moscone                | SFMTA 35         |               0/1 | Drive stopped at an inferred wall |

All completed drives recorded zero airborne travel. None had a measured dip
deeper than 0.5 m over an eight-metre chord. The Howard drive crashed
grounded, 45.6 m into the course.

- Public Works snapshot SHA-256 `c7b76f2f3d07ba28d2142008d65edf1a466782de89dbca024c0fc7a5156b7d00`.
- SFMTA snapshot SHA-256 `3145a2bd5e6a81a5b4b25620e92c907d0576c5b4e3e76fbfba208c47ba5d982c`.
- Caltrans snapshot SHA-256 `6a8fc541f8c4ec952e56053ac2d23105f83ef8e36b468cbc86d10e81beae909e`.

### Howard Street at the Moscone crossings

**World and Data.** The candidate lowers 94.6 m² of an inferred building
footprint beneath the roadway, retaining its adjacent faces. The April
2023 survey has 102,258 pavement returns in the checked strip. Returns
above the road support two distinct overhead spans. The candidate retains
them as scenery at estimated base and top elevations of 19.3–25.3 m and
16.2–18.1 m in the game. Neither span supplies motor-vehicle support.
These are inferred game volumes from the exterior returns. Physical
underside clearance requires separate measurements.
[Public Works Moscone expansion](https://sfpublicworks.org/node/34512).

**Physics.** The three-lane Chrome regression starts before the eastern
span, drives 132 m westbound and finishes beyond the western span. The
centre and both four-metre offset drives clear the two spans on ordinary
street contact. All three finish grounded without a crash or airborne
interval.

```sh
SF_BASE_URL=https://velocity.report node tools/sf-drive/smoke-main-street.cjs howard
```

- Parent game manifest SHA-256 `f2b4e5efa2258a968a99436d59ff1c12752fdd2d5a7243673ddbc5974bb79e05`.
- Corrected game manifest SHA-256 `ce013b65fc67cdb098dddc7f4852981b7ee34c30f7bceb74b93ad0220555376f`.
- Gerardus review SHA-256 `e0955894d68dd0907d25d1195d8215d4215fcfc90f9a4f0084c9a64534f69996`.
- Survey node receipt SHA-256 `7df8457960c10eb9ff0178d9a2d623ac5d68bdbeadd4ce41f59d5698651c29e8`.

The retained local run is `howard-crossing-evidence-20261125`. It includes
the inventory inputs, course recipes, Chrome telemetry and survey returns.

## Daly City separation tunnel and the map edge

**World and Data.** FHWA record `35 0173L` carries southbound I-280 beneath
the separation. The mapped bore runs for 220.7 m. The April 2023
survey returned lower pavement medians of 59.05 m and 56.41 m NAVD88 at
the two mouths. The upper surface at each mouth is about 7 m higher.
The bake infers an interior grade from the lower approaches and returns
the car to open ground 44 m beyond the south mouth. Both owner tiles
carry the full contact. The fitted grade ranges from −4.79% to +2.17%.
The interior elevation remains an inference.
[FHWA tunnel inventory](https://www.fhwa.dot.gov/bridge/inspection/tunnel/inventory/download.cfm)
and [2023 USGS survey](https://www.fisheries.noaa.gov/inport/item/73506).

**Physics.** The original centre-lane drive crashed on the north
approach. Four inferred trunk rows intersected the pavement, where
ground returns confirmed a clear running surface. With the four rows
removed, Chrome drove in three lane positions using the normal
keyboard controls with route assist off.

| Lane   | Result                       | Peak speed | Ground elevation after exit | Airborne |
| ------ | ---------------------------- | ---------: | --------------------------: | -------: |
| Left   | Grounded, no crash or scrape |   43.6 mph |                     57.17 m |      0 m |
| Centre | Grounded, no crash or scrape |   43.6 mph |                     57.12 m |      0 m |
| Right  | Grounded, no crash or scrape |   43.6 mph |                     57.38 m |      0 m |

All three passed from the existing I-280 carriageway to the bore and
then to open ground. The test establishes the southbound movement
through the bore. It stops before the end of the current terrain and
does not certify the separate upper-deck or interchange ramp movements.

```sh
node tools/sf-drive/smoke-presidio-tunnels.cjs \
  daly-separation-left daly-separation-centre daly-separation-right
```

- Parent game manifest SHA-256 `ce013b65fc67cdb098dddc7f4852981b7ee34c30f7bceb74b93ad0220555376f`.
- Corrected game manifest SHA-256 `cb2146825cb333672294cc1dc950bb52bc1ae55da0a77852b1c86040d83a7301`.
- Gerardus contact SHA-256 `0d0511934e81dd80c4f50729942d5eec91e9bad7840e963066836269fb778f20`.
- Survey node receipt SHA-256 `05668f82d2aa5c0b6b6aa4e95522c11c3d7b2fbc6f770ca4eed92c62a5d5938a`.

**World and Data.** At Baker–Barry, 26 of 241 centreline stations at
roughly 3 m spacing fell within the shipped tile envelopes. None had
baked terrain. Both Robin Williams tunnel bores lie beyond those
envelopes. Marin County terrain has been retained for the missing area.
As a check in the overlapping terrain, the sampled raster height at the
existing Alexander Avenue game contact was 82.13 m, compared with the
game anchor at 82.07 m. One check cannot establish a datum correction
for the full extension. Baker–Barry still needs connected tiles and
portal pavement measurements before a playable pass.
[Marin County terrain service](https://gis.marinpublic.com/arcgis/rest/services/LIDAR/DEM_Harn/ImageServer).

The retained local run is `game-map-edge-tunnels-evidence-20261125`.

### John Daly Boulevard above the separation

**World and Data.** The game carries two John Daly Boulevard
carriageways over the I-280 bore. The fit used 94 eastbound and 84
westbound survey stations. The p95 residual against the measured
pavement was 2.7 cm eastbound and 3.7 cm westbound. At the crossings,
the game puts the upper surfaces 14.9 m and 15.5 m above the inferred
tunnel road. These are separations between game running surfaces.
[Caltrans All Roads](https://gisdata.dot.ca.gov/arcgis/rest/services/CHhighway/All_Roads/FeatureServer/0)
and [2023 USGS survey](https://www.fisheries.noaa.gov/inport/item/73506).

**Physics.** Chrome drove the centre and both lane offsets in each
direction. All six drives started on open pavement, entered the upper
road and returned to ground with no crash, scrape or airborne travel.
The eastbound offsets were 4.5 m either side of centre and the
westbound offsets were 3 m. Peak speeds were 30.8–32.5 mph. A seventh
drive repeated southbound I-280 below, retaining the lower contact.

The static package replay passed 451 allowed span directions,
317 directed joins and 510 ground handoffs, including three
continuations onto other baked roads. The offline upgrade retained the
collected emoji and fetched one changed tile.

```sh
node tools/sf-drive/smoke-presidio-tunnels.cjs \
  john-daly-eastbound-centre john-daly-eastbound-left john-daly-eastbound-right \
  john-daly-westbound-centre john-daly-westbound-left john-daly-westbound-right \
  daly-separation-centre
```

- Game manifest SHA-256 `ea69d34343f04f8ee1ff7d9c5a43226e367f37492eb65dc76f1d4be547474bcf`.
- Gerardus contact SHA-256 `77fff6c061bffe630de69e5ac7b15cd001c7916cda55f5da1fe47d4f34746e66`.

The I-280 and SR 1 ramp movements remain under review.
