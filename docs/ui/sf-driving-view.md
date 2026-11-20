# San Francisco driving view (`/sf/`)

- **Status:** driving engine and emoji time trials.
- **Page:** [`public_html/src/sf/index.njk`](../../public_html/src/sf/index.njk); logic in
  [`sf-drive.js`](../../public_html/src/js/sf-drive.js) and
  [`sf-drive-core.js`](../../public_html/src/js/sf-drive-core.js) (tested in
  [`sf-drive-core.test.mjs`](../../public_html/src/js/__tests__/sf-drive-core.test.mjs)).
- **Data:** `public_html/src/sf/tiles/`, published at `/sf/tiles/`.
- **How the map was built:** [city map record](sf-city-map.md) (levels of detail, decisions, next
  steps), with the [size study](sf-city-map-size-study.md) and
  [review brief](sf-city-map-review-brief.md).

The page drives a car anywhere in San Francisco over measured geometry, so a reader can move between
survey sites with the city around them. A chase camera sits behind the car and a mirror shows the road
behind. A readout names the street under the car and the next signposted cross street ahead, and
street-name signs stand at intersections.

## Driving and time trials

Steering travels gradually and returns to centre. A bicycle model runs at 120 Hz, with
speed-dependent steering, a lateral tyre-grip limit, rolling resistance, aerodynamic drag,
and gravity along the measured hill slope. Down/S brakes before engaging reverse; space
brakes and holds the car on a hill. Off-road ground reduces grip and adds resistance.
The wheels steer and spin, and the body pitches with the terrain. Two rear brake lamps
and a high centre lamp brighten when space is pressed, opposing throttle brakes the car,
or recovery holds its brake.

Building roof-face outlines (including courtyard holes) and tree trunks have collision
checks in 32 m spatial buckets. An oriented rectangle covers the car body and bumpers;
short physics steps prevent it jumping through a trunk. A crash stops driving and plays
a two-second spin at the impact position before returning the car to its last reached
clear intersection, or the nearest clear loaded intersection if that checkpoint has
streamed out. The clock continues, pickups are paused, and R / Reset car cannot skip the
spin. Outside a crash, R / Reset car returns directly to the intersection. Inputs are
released on impact, recovery and lost focus. Kerbs and tree crowns do not cause a crash.
The car waits at a missing ground surface or an unloaded tile boundary.

Collect a different emoji at one intersection in each of the 137 tiles with signposted
intersections, in any order. The other 20 of the 157 tiles have no signposted intersection;
they remain on the map and are excluded from the completion requirement. Emoji beacons
appear in the world, and the north-up minimap shows every tile, loaded/loading tiles,
collected tiles, the car, and a line to the seeker target.

The compass seeker points to the uncollected emoji belonging to the current tile, even
when a neighboring tile's emoji is closer. It shows that emoji on the compass, a direction
arrow, distance, and the car's cardinal/numeric heading. The car's forward direction is
always up; compass letters and the target rotate relative to it. After the local pickup,
the seeker marks it collected and points to the nearest remaining target. Tiles without
a quest also seek the next target, and a completed rally clears the target arrow.

The car holds its brake before the first driving input, which starts the clock. It measures real elapsed time, including crashes,
recovery, loading and background time. Collected emojis survive respawns and tile unloading.
Completing the fixed target set stops the clock and saves the five fastest personal times
on this device, separately for each starting place and map manifest. New trial or choosing
a different starting place resets the entire run; Reset car keeps the clock running.
There is no shared leaderboard. When browser storage is unavailable, records last for the session.

`public_html/src/sf/quest-route.json` is a small gameplay index alongside the unchanged
city tiles. Rebuild it from the repository root with
`node tools/sf-drive/build-quest-route.mjs`. It pairs opposite-corner street signs,
projects their midpoint onto a named carriageway (including roads owned by a neighboring
tile), and chooses a grounded intersection clear of buildings and trunks. Its manifest
hash scopes saved records. The engine tests check complete tile coverage and unique emoji.

The map readout reports network response-body sizes for the currently loaded tiles, the
complete city tile set, and the full game asset download (tiles, manifest, quest index,
engine modules and three.js). It uses Resource Timing's `encodedBodySize`, so gzip/Brotli
responses count at their compressed size. Headers, protocol overhead, the page document,
and favicon are excluded. Cached assets count at their encoded download size rather than
zero; retries do not add the same file twice. This is the asset inventory, not a running
counter of this session's network traffic or an estimate of heap/GPU memory.

When response timings are unavailable, the build supplies gzip-level-9 estimates for text
assets and the tile manifest supplies the exact sizes of the existing gzipped tile files.
A total containing an estimate is marked `≈`. With gzip, the full asset download is about
30.6 MB, compared with the previous 32.4 MB on-disk total. The displayed number follows the
host's actual response sizes as they become available; an uncompressed development server
can therefore report more. No extra requests or full-city download are needed to measure it.

## Road rendering

Road and path colours and dashed markings are painted from the existing centreline and
kerb-width data onto the terrain's own triangles. This gives each road point one surface;
an independently interpolated asphalt mesh can no longer intersect the hill underneath.
Adjacent overlapping ground grids are clipped at their nearest-origin boundaries, and
height queries use the same triangle planes. Texture resolution is 2048 pixels per tile
on desktop and 1024 on touch devices. Buildings, trees, signs, kerbs and source tile files
retain their existing geometry.

## What it shows

| Layer                             | Source                                                                                                                                                                    | Treatment                                                                                                                                                                                                                                                                                                             |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Buildings                         | DataSF Building Footprints (`ynuv-fyni`, PDDL-1.0; 2010 heights) and roof planes fitted to USGS 3DEP lidar (flown April 2023, public domain) by Gerardus's city roof bake | Each roof face is a plane over its outline; walls stand on the City's outline. Faces whose lidar returns show canopy are given to a neighbour. Buildings without fitted roofs are boxes at their 2010 height                                                                                                          |
| Structures the 2010 outlines lack | 3DEP lidar: solid, planar returns at least 3 m up with no footprint, or a footprint whose lidar top stands well above its 2010 height                                     | Flat tiers at 1.5 m steps, shaded violet. Salesforce Tower and other post-2010 towers appear this way                                                                                                                                                                                                                 |
| Trees                             | DataSF Street Tree List (`tkzw-k3nq`, PDDL-1.0) inside the street right of way; 3DEP canopy tops elsewhere                                                                | A listed tree takes height and crown from the lidar canopy over it, or a size from its trunk diameter. No trunk stands on the carriageway                                                                                                                                                                             |
| Roads                             | DataSF streets (`3psu-pn9h`), Curbs and Islands (`emxt-b6yg`, 2016), Right of Way (`h8n7-e4ns`), all PDDL-1.0                                                             | Asphalt between the kerbs, from each centreline's distance to the first kerb on either side; kerbs raised 15 cm; dashed centrelines on two-way streets                                                                                                                                                                |
| Ground                            | 3DEP class-2 ground via Gerardus's city view tiles                                                                                                                        | An 8 m grid                                                                                                                                                                                                                                                                                                           |
| Signposts                         | DataSF streets and street nodes; Curbs and Islands                                                                                                                        | A pole on each of two opposite corners wherever two or more named streets meet, 1 m past the corner kerb on the bisector between the streets (median-island kerbs skipped). Each street gets a green blade lined up with that street, lettered on both faces. DataSF's placeholder names ("Unnamed 186") are left off |

The geometry is measured spatial data in the sense of
[geometry priors: data purpose](../geometry-priors-data-purpose.md): context for traffic data at a
stated acquisition time, not a survey, plan or specification. Positions are NAD83(2011) in EPSG:7131
less a fixed origin, heights NAVD88.

## Tiles

One gzipped JSON per S2 level-13 cell (about 1.1 km across, 157 cells with land), plus
`manifest.json` listing each tile's origin and half-width and the named starting places. A tile owns
the buildings, trees, roads and signs whose point falls inside its cell, so nothing is drawn twice.
All tiles share one planar frame, so they meet without seams. Inside a tile, positions are integer
decimetres from the tile origin. Tile format version 2 writes a signpost as
`[x, y, name id, blade angle, name id, blade angle, …]`, the angle in whole degrees from east, 0–179.

The November 2026 build has 176,866 buildings (2,932 lots redrawn from the lidar because it stands well
above their 2010 height, and 1,296 structures with no City outline), 343,046 trees, 15,671 street lines
and 15,046 signposts at 7,523 intersections. The tiles total 30 MB; the median tile is 0.2 MB and the largest 0.42 MB, so a view
of about nine tiles fetches 2 to 3 MB.

The page loads the tiles within about 900 m of the car's tile edge, nearest first, three at a time,
and drops tiles more than 1.6 km past their edge.

## Limits

- Roof shapes come from the April 2023 lidar and the City's 2010 outlines; buildings changed since are
  shown as the lidar saw them, or not at all.
- The canopy test reads the lidar at a coarser density than the per-site studies and flags somewhat
  more roof faces than they did.
- Tall post-2010 towers are flat tiers, not their curved forms.
- Collision uses the available building outlines and trunks; it does not model kerb impacts,
  tyre suspension, vehicle damage or traffic. Ground and roof heights retain the source limits.
- The tiles are gzipped JSON for ease of reading. A binary encoding of the same geometry measured about a
  third of the size in the asset study, and would be the next step if load time matters.
- Tree crowns are drawn as spheres from low on the trunk, so a blade can sit inside a street tree's
  crown that the City would have pruned clear of the sign.
- The tiles are baked outside this repository from Gerardus's caches; the manifest records the inputs
  and the build.
