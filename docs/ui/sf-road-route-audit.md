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
