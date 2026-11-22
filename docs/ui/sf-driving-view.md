# San Francisco driving view (`/sf/`)

- **Status:** driving engine, emoji time trials, survey traffic replay and solo safety practice.
- **Page:** [`public_html/src/sf/index.njk`](../../public_html/src/sf/index.njk); logic in
  [`sf-drive.js`](../../public_html/src/js/sf-drive.js) and
  [`sf-drive-core.js`](../../public_html/src/js/sf-drive-core.js) (tested in
  [`sf-drive-core.test.mjs`](../../public_html/src/js/__tests__/sf-drive-core.test.mjs)).
- **Data:** `public_html/src/sf/tiles/`, published at `/sf/tiles/`.
- **How the map was built:** [city map record](sf-city-map.md) (levels of detail, decisions, next
  steps), with the [size study](sf-city-map-size-study.md) and
  [review brief](sf-city-map-review-brief.md).

The page drives a car anywhere in San Francisco over measured geometry, so a reader can move between
survey sites with the city around them. The camera starts zoomed out behind and above the car;
Overhead and In car are selectable in the menu. A readout names the street under the car and the next signposted cross street ahead, and
street-name signs stand at intersections.

## Game design and review

Describe and review game updates using the six shared anchors: **World, Physics, Traffic,
Goals, UI, Data**. A change may span several anchors. The
[framing and references](../../.github/knowledge/game-design.md) define their boundaries,
feedback-loop questions, and future autonomous-traffic contracts. The
[game-design skill](../../.claude/skills/game-design/SKILL.md) applies that language in
discussion, implementation, and review; agent instructions route SF game changes to it.

The [safer driving plan](../plans/ui-sf-safer-driving-plan.md) proposes a solo safety
mode and a repeatable following and downhill braking scenario. The optional
[solo practice slice](sf-solo-practice.md) implements Milestone 1 on an explicitly
authored course; its contact model and evidence remain separate from the city time trial.

## Driving and time trials

**UI:** the normal driving screen has a 48 px status strip (street, speed, clock,
collection count and menu), a narrow portrait direction seeker, and two thumb clusters.
The speed numeral is 26 px with a smaller mph unit. Below the strip, the compass stays
in the left quarter and the in-car mirror sits at the top centre.
Steer left/right with the left thumb and brake/accelerate with the right. Keep holding
the brake to reverse after stopping. Both pedals together hold the brake. Map & game opens
a scrollable modal starting with a three-stop camera slider, followed by starting place, reset,
the full minimap, compressed download sizes, records and help. Light blue tile outlines
indicate the saved offline coverage for the current map revision. Map sources is last. Opening it releases held
controls and keeps keyboard driving inactive until it closes. The trial clock continues.
Desktop layouts also show City coverage in the bottom right during play.
Crash replay controls and completion messages stay visible during play.

The closed-menu mobile layout targets less than 25% of the viewport for visible UI,
including the control groups and the in-car mirror. Count the bounding rectangles of the
status strip, seeker, both thumb clusters and any visible mirror against viewport area;
the full-screen 3D canvas is excluded. The expanded menu is an explicit detail view and
does not share this gameplay-area budget. Short screens use a smaller seeker and 48 px
buttons; other touch screens use 56 px buttons. Safe-area insets keep controls clear of
the device edges, without disabling page zoom globally.

Camera choices are Zoomed out (default, 34 m behind and 26 m above), Overhead (85 m
directly above, with the car's heading up), and In car (attached at driver eye height,
following local road slope). In-car rendering hides the car body. Lateral acceleration
banks the cockpit outward up to six degrees, with a damped return as the car straightens.
The reduced motion setting keeps roll level. Only In car permits the rear-view mirror,
including on mobile. Its switch is in the menu. V cycles
through the cameras, and M toggles the mirror while in car. Changing cameras snaps to the
new pose. Camera poses are covered by
[`sf-drive-camera.test.mjs`](../../public_html/src/js/__tests__/sf-drive-camera.test.mjs).

In the two external views, trees and buildings in front of the car reveal it through
a circular cutout sized to the projected car, with a three CSS pixel feather. The mask
leaves terrain and the rear-view mirror intact. Collision still uses the full geometry.

The touch pad captures each pointer until release or cancellation, supports simultaneous
throttle/steering/braking, and prevents text selection, long-press callouts and double-tap
zoom on the controls. Held buttons have a visible pressed state. Losing focus, hiding the
page or recovering the car clears held input. Viewport zoom is not disabled globally.

Steering travels gradually and returns to centre. A bicycle model runs at 120 Hz, with
speed-dependent steering, a shared grip budget for braking, acceleration and cornering, rolling resistance, aerodynamic drag,
and gravity along the measured hill slope. Down/S brakes before engaging reverse; space
brakes and holds the car on a hill. Off-road ground reduces grip and adds resistance.
The GTI-inspired tune uses 350 hp, 420 N·m and a mass of 1,280 kg. Wheel force is
limited by torque at low speed, available power at high speed, and traction. The model
uses an idealised transmission, 88% drivetrain efficiency and a 250 km/h limiter.
On level asphalt the 120 Hz simulation reaches 60 mph in 4.34 seconds and 155 mph at
the limiter. These are game simulation results.
The wheels steer and spin, and the body pitches with the terrain. Two rear brake lamps
and a high centre lamp brighten when space is pressed, opposing throttle brakes the car,
or recovery holds its brake.

Building roof-face outlines (including courtyard holes) and tree trunks have collision
checks in 32 m spatial buckets. An oriented rectangle covers the car body and bumpers;
the velocity into the contact normal distinguishes a glancing scrape from a hard strike.
A scrape sheds speed across the wall and preserves motion along it. The tyres gradually
catch the slide. A hard strike disconnects steering, throttle and braking during the crash replay. Momentum carries the chassis through follow-up contacts with walls, trunks,
rails and the ground. A square hit can rebound, a corner strike can yaw, and a hard
landing on one side can roll the car. In flight, the chassis keeps its take-off pitch
and bank. Each ground contact takes its slope from the road or terrain it touches. The camera holds its approach direction during
recovery and shows the chassis from outside if the driver was in the cabin.

The trial clock continues and pickups pause until the car returns to its last clear
intersection, or the nearest clear loaded intersection if the checkpoint has streamed out.
The road drives retain their route recovery. **Skip replay** or Escape goes straight
to recovery. The automatic clip starts two simulation seconds before impact when that
much driving is available. It holds the final view for two seconds after ground contact,
linear speed and angular speed confirm rest. A fall out of the supported world or
a wreck still moving after 15 simulation seconds ends the clip at the recovery limit.

**Data:** **Saved replays** in the menu keeps the last eight crashes and eight longest
completed airtime clips in this browser. A flight that ends in a crash still
counts and is labelled **Impact ending**. A fall out of the map lacks a
completed flight. Older airtime records retain their scores.
Archive playback leaves the live drive paused. A clip from a different drive or practice
course opens its matching scene in another tab. City replays use the map tiles in that
browser's offline cache or available over the network. The clip stores quantised position
and quaternion keyframes chosen from 120 Hz simulation states. Playback interpolates
between keys and keeps the impact boundary and stationary hold explicit, so it does not
re-run contacts against the currently loaded map.
Inputs are released on impact, recovery and lost focus. The contact dimensions,
friction, restitution and thresholds are gameplay approximations. Kerbs and tree crowns
do not trigger city crashes.
The car waits at a missing ground surface or an unloaded tile boundary.

Replays use 0.99× speed, with 0.5× from 300 ms before to 300 ms after a crash
impact. An airtime replay ending at impact slows over its last 300 ms. The stationary hold takes two real seconds. The replay and library locate
a crash by its impact and a jump by its take-off, using the nearest city emoji
and the compass bearing and distance from the event to the emoji. The authored
practice course uses its finish flag.

Replay graphs store peak speed, deceleration and jerk in 100 ms bins from the
120 Hz simulated vehicle velocity. Deceleration is the positive drop in speed
per second. Jerk is the magnitude of the change in the acceleration vector per
second. The graphs keep short impact spikes visible and mark the impact with an
orange line. Older clips retain their trajectories. Vehicle telemetry is available
for clips captured with this version. A survival threshold needs occupant loads,
restraints and injury criteria. The game models the chassis. [NHTSA test procedure for occupant crash protection](https://www.nhtsa.gov/sites/nhtsa.gov/files/documents/tp-208-14_tag.pdf)

Collect a different emoji at one intersection in each of the 137 tiles with signposted
intersections, in any order. The other 20 of the 157 tiles have no signposted intersection;
they remain on the map and are excluded from the completion requirement. Emoji beacons
appear in the world, and the north-up minimap shows every tile, loaded/loading tiles,
collected tiles, the car, and a line to the seeker target.

The compass seeker points to the uncollected emoji belonging to the current tile, even
when a neighboring tile's emoji is closer. A gold, white-outlined arrow fills the compass
dial, with the target emoji and distance alongside it so the emoji cannot cover the arrow.
The car's forward direction is always up; compass letters and the target rotate relative
to it. After the local pickup, the seeker points to the nearest remaining target and
labels its scope Next tile; the collection count and minimap show progress. Tiles without
a quest also seek the next target, and a completed rally clears the target arrow.
The dial uses an explicit turn instruction such as
Straight ahead, Bear left, Turn right or Turn around.

The nearest uncollected emoji with a loaded marker has a gold button drawn above the
scene, including in front of intervening buildings. A dashed outline indicates the
marker is outside the playable view. Click, tap or activate the button with the keyboard
to centre the camera on the emoji. Activate it again or use a driving control to return
to the normal camera. A camera change, opening the menu, reset, crash or a change of
target ends the lock. Steering and tile loading still follow the car.

The whole city is the default. Open `/sf/?district=<id>` to load an optional district-only run. District runs
appear when the map includes authoritative district boundaries and quest membership; they
are unavailable in the current map. The frontend includes selection, a fixed district start,
filtered beacons/seeker/completion, a boundary overlay and separate best times. See the
[district metadata contract and separate Gerardus handoff](../plans/ui-sf-district-trials-plan.md).

The car holds its brake before the first driving input, which starts the clock. It measures real elapsed time, including crashes,
recovery, loading and background time. Collected emojis survive respawns and tile unloading.
Completing the fixed target set stops the clock and saves the five fastest personal times
on this device, separately for each starting place, map manifest and driving tune. The GTI
tune starts a fresh set of times. Earlier records stay in browser storage. The game restores your trial when you return to a starting place or reload. After finishing, choose **Start a new emoji trial** to try for a faster time. Reset car keeps the clock running.
There is no shared leaderboard. When browser storage is unavailable, records last for the session.

`public_html/src/sf/quest-route.json` is a small gameplay index alongside the unchanged
city tiles. Rebuild it from the repository root with
`node tools/sf-drive/build-quest-route.mjs`. It pairs opposite-corner street signs,
projects their midpoint onto a named carriageway (including roads owned by a neighboring
tile), and chooses a grounded intersection clear of buildings and trunks. Its manifest
hash scopes saved records. The engine tests check complete tile coverage and unique emoji.

City coverage draws the four S2 level-13 corners of each manifest token in the game's
EPSG:7131 frame. The projected cell boundaries share corners across tiles. The map shows
loaded and collected cells, a small white car arrow, the shoreline, larger park properties
major roads and red diamonds for replayable surveys. The driving compass provides emoji guidance.

The map uses the repository's [DataSF shoreline](https://data.sf.gov/d/rgcx-5tix),
[Recreation and Parks Properties](https://data.sf.gov/d/gtr9-ntp6), and road classes 1–3
from the existing tiles. The map simplifies these layers at city scale. Both DataSF
polygon sources use the Public Domain Dedication and License. Regenerate
`public_html/src/sf/coverage.json` with `python3 tools/sf-drive/build-coverage.py <parks.geojson>`
using `s2sphere==0.2.5`, `pyproj==3.7.2` and `shapely==2.1.2`. The checked-in map uses
the [December 2025 DataSF export](https://github.com/DasilvaKareem/sf_tourism_data/blob/1ace88d893f0b724c14949730b131099b88eca45/parks/recreation_parks.geojson).

The map readout reports network response-body sizes for the currently loaded tiles, the
complete city tile set, and the full game asset download (tiles, manifest, quest index,
coverage map, engine modules and three.js). It uses Resource Timing's `encodedBodySize`, so gzip/Brotli
responses count at their compressed size. Headers, protocol overhead, the page document,
favicon and on-demand survey recordings are excluded. Cached assets count at their encoded download size rather than
zero; retries do not add the same file twice. This is the asset inventory, not a running
counter of this session's network traffic or an estimate of heap/GPU memory.

When response timings are unavailable, the build supplies gzip-level-9 estimates for text
assets and the tile manifest supplies the exact sizes of the existing gzipped tile files.
A total containing an estimate is marked `≈`. With gzip, the full asset download is about
30.6 MB, compared with the previous 32.4 MB on-disk total. The displayed number follows the
host's actual response sizes as they become available; an uncompressed development server
can therefore report more. No extra requests or full-city download are needed to measure it.

## Survey traffic replay

**Data, World, UI.** The lower-left cue shows the name and straight-line distance of
the nearest survey ahead within 250 m. Drive within 115 m of a survey's placed sensor
to replace the cue with its speed card and start its recording at normal speed. The nearest eligible survey takes priority, with
a 15 m margin before switching between overlapping approaches. The replay closes beyond
145 m or when switching to solo safety practice. Each visit starts the recording again,
and the recording loops at its end. The red
diamonds on the minimap identify the 24 playable surveys. SoMa 1 is published without an
accepted pose.

The game streams the published survey's frames on approach. The replay shows classified
motor vehicles, pedestrians and cyclists as boxes. Their motion follows the recorded
timestamps. They are visual observations with no collisions or response to driving input.
The game starts each intersection independently and uses the survey's stationary coverage
when the export defines it. Broken replay assets leave the card and survey link available.

The compact lower-left card sits above the touch steering controls and shows the
distribution of peak speed per vehicle track and the P85 for the replayed recording.
It uses the survey's published bins and posted limit, and opens that recording in a new tab
to keep the drive in progress.

At build time, [the survey data join](../../public_html/sf-survey-data.cjs) pairs the placed
export in [the city index](city-view.md#citysitesjson-the-city-site-index) with its histogram.
It converts the accepted sensor pose through GRS80 ECEF to the game's EPSG:7131 grid and
NAVD88 heights. A local affine transform places track boxes within the surveyed area. The
rendering lifts any box whose measured base would fall under the game's coarser terrain.
The [placement tests](../../public_html/src/js/__tests__/sf-survey-replay.test.mjs) compare
sample positions with independent PROJ results.

**UI, World.** The game, homepage hero and survey viewer share a dark palette, red motor
vehicles, hero-green pedestrians and blue cyclists. Grey and tan buildings have
translucent faces and restrained outlines. Trees use dark emerald and brown. The
[game-design skill](../../.claude/skills/game-design/SKILL.md#public-scene-design)
describes how to carry these roles into future changes.

## View distance

**UI, World.** In the main view the city fades from 155 m to 320 m of camera depth and
clips at 360 m. The mirror fades from 55 m to 140 m and clips at 160 m. Buildings are
batched in 160 m groups so the renderer can cull distant blocks. The renderer still
uses the existing tile streaming and collision data.

## Road rendering

Road and path colours are painted from the existing centreline and kerb-width data onto
the terrain's own triangles. This gives each road point one surface;
an independently interpolated asphalt mesh can no longer intersect the hill underneath.
Adjacent overlapping ground grids are clipped at their nearest-origin boundaries, and
height queries use the same triangle planes. Texture resolution is 2048 pixels per tile
on desktop and 1024 on touch devices. Kerb lines also paint onto that texture. The
previous raised strips interpolated only between endpoints, so long segments could
float above or cut through hills. The paint follows every rendered terrain triangle
and stops at gaps in the ground. Independent height samples can still disagree at terrain
ownership seams.

Centre dashes use a separate unlit vector-paint mesh, clipped to those exact same ground
triangles and tile ownership boundaries. This keeps thin paint sharper than the mobile
texture without increasing its resolution. Only the depth buffer is offset; the paint's
vertices use the original terrain planes, including hill diagonals and missing-ground
holes. The
[`sf-road-markings`](../../public_html/src/js/sf-road-markings.js) helper carries the 3 m
dash / 3 m gap phase through each polyline's bends. Dashes are 0.30 m wide, drawn only
for existing two-way carriageways; they are stylised guidance rather than observed paint
or passing rules. No lane counts, stop bars, crossings or turn arrows are inferred.

Streaming paint uses [`sf-terrain-painter`](../../public_html/src/js/sf-terrain-painter.js)
and a module worker. A source tile invalidates only target ownership regions its road/
marking bounds can overlap, within the existing neighbour-distance limit. Unaffected
textures and mesh objects remain unchanged. Pending changes coalesce; the main thread
paints at most one terrain texture and applies at most one marking result per animation
frame. A tile's ground/region/markings are sent to the worker once on load. The worker
caches clipped geometry per target/source pair, reuses retained contributors when a
source leaves, and evicts those parts on unload. Transferred result buffers are separate
from cached parts. Target identity and revision checks reject obsolete results, including
unload/reload of the same token. If module workers fail, the same bounded texture queue
draws raster dashes instead of running clipping on the main thread.

The manual [road-paint benchmark](../../tools/sf-drive/benchmark-road-paint.mjs) runs with
`node tools/sf-drive/benchmark-road-paint.mjs`. It uses the nine `tilesAround` tiles centred
on `808580c4`, their manifest ownership regions and the original neighbour filter. In
Node v24.19.0 here, after warm-up, five passes produced these median CPU times:

| Work across all nine tiles           | Median    | Clipping calls with a warm cache |
| ------------------------------------ | --------- | -------------------------------- |
| Full clipping and Float32 conversion | 251.01 ms | Not cached                       |
| Cached result assembly               | 10.93 ms  | 0                                |

The tool checks identical Float32 geometry (340,212 vertices). This compares CPU geometry
work, excluding texture painting, worker startup/transport, GPU uploads and rendering;
the browser uses affected-tile scheduling rather than recomposing all nine each time.
Chromium touch emulation at 390×690 verified zero main-thread clipping calls while
streaming into that neighbourhood. Removing `808580b4` repainted/replaced only two
remaining targets, reused all retained clipped parts and preserved other mesh objects.
Observed callback maxima were 5.8 ms for texture paint, 0.7 ms for mesh replacement and
8.4 ms for adding/sending a source. These headless measurements are not mobile-device
frame-rate guarantees; physical-device validation remains outstanding.

The ordinary city game uses one ground surface at a horizontal position. On Hydra,
**Map & game → Open regional road review** opens the separate road surfaces and confirmed
connections published by Gerardus. See the
[regional review controls and scope](../plans/ui-sf-layered-roads-plan.md#regional-review-in-the-game).

The default **Bay Bridge · SoMa survey deliveries** drive starts rolling at 50 mph on
the west span, exits at Fremont Street, visits 1st at Mission, 3rd at Folsom and Howard
at 6th in that order, then takes the Fourth Street entrance and ends on I-80. Its
authored street itinerary preserves the bundled ramp joins. Each delivery requires
grounded contact within 40 m horizontally and 2 m vertically of terrain sampled at
the survey location. The street-only route remains available at `/sf/?city=soma`. Survey cylinders show the locations of all
placed replay sites, even beyond the local tile draw distance.

Delivery drives begin in **Self-driving**. Driving input opens a confirmation for
**Meat mode** while the car continues in self-driving. Confirming preserves car motion
and transfers control with neutral inputs. Release any held control and press again
to drive or brake. The meter charges 0.12¢ per second ($0.0012/s), with a half rate of
0.06¢ per second. The bottom-centre orange meter shows the running total in
dollars and the current rate in cents. **Resume self-driving** stops the meter,
preserves the spend and shows the purple self-driving indicator.
Manual driving on the delivery route can cross from the guide onto continuous street
pavement. Verified ramp joins support reversing across the seam. The controller
rejoins from nearby street pavement and can start again at rest. If the car is facing
away, too far from its route, or blocked when trying to resume, the game announces a
return to its last moving route checkpoint and continues from there. The delivery
count and insurance spend carry through that recovery.
Insurance lasts for the session. Saved trial checkpoints contain no insurance amounts. On the first visit to a survey, the driver
earns either ten free seconds or twenty half-rate seconds. Credits count down during
manual driving and free seconds are used first. The HUD tracks deliveries, spend and
crash infractions. These rates are game tuning values.

Choose **Bay Bridge approach** in **Start from** for the westbound drive at 50 mph.
The route crosses the bridge, takes the Fremont exit and rejoins I-80 from Fourth
Street. The picker also includes Fremont Street exit and city intersection starts.
Both Fourth Street entrance rails deflect the car and slow it down on contact. The
same self-driving and manual controls apply. The game draws the baked overpasses
in the city tiles. See the [citywide bake and game route](../plans/ui-sf-layered-roads-plan.md#citywide-bake-and-bay-bridge-game-route)
for supported geometry and authored connections.

The westbound bridge view loads the waterfront skyline before the car starts
across the span. As the car approaches San Francisco, the local city tiles take
over and restore the usual draw distance.

The last menu section, **Map Sources**, links to read-only map provenance. The game
loads its tiles and the bridge route from its own origin. On HTTPS and
localhost, a service worker stores the game shell and downloaded tiles in browser
Cache Storage. Use **Download all tiles** under **City coverage** in the menu and wait
for the completion message to save the whole city for offline play. If the download is
interrupted, try again and the game will reuse saved tiles. Return to the same origin
in the same browser to play offline. The browser can evict cached data.
An uncached area needs a connection before the game can load its tiles. Emoji
collections and the trial checkpoint are saved per start and district in local
storage. **Saved replays** plays the eight longest completed flights across starts.

## What it shows

| Layer                             | Source                                                                                                                                                                    | Treatment                                                                                                                                                                                                                                                                                                             |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Buildings                         | DataSF Building Footprints (`ynuv-fyni`, PDDL-1.0; 2010 heights) and roof planes fitted to USGS 3DEP lidar (flown April 2023, public domain) by Gerardus's city roof bake | Each roof face is a plane over its outline; walls stand on the City's outline. Faces whose lidar returns show canopy are given to a neighbour. Buildings without fitted roofs are boxes at their 2010 height                                                                                                          |
| Structures the 2010 outlines lack | 3DEP lidar: solid, planar returns at least 3 m up with no footprint, or a footprint whose lidar top stands well above its 2010 height                                     | Flat tiers at 1.5 m steps, shaded warm tan. Salesforce Tower and other post-2010 towers appear this way                                                                                                                                                                                                               |
| Trees                             | DataSF Street Tree List (`tkzw-k3nq`, PDDL-1.0) inside the street right of way; 3DEP canopy tops elsewhere                                                                | A listed tree takes height and crown from the lidar canopy over it, or a size from its trunk diameter. No trunk stands on the carriageway                                                                                                                                                                             |
| Roads                             | DataSF streets (`3psu-pn9h`), Curbs and Islands (`emxt-b6yg`, 2016), Right of Way (`h8n7-e4ns`), all PDDL-1.0                                                             | Asphalt between the kerbs, from each centreline's distance to the first kerb on either side. Kerb lines follow the terrain and two-way streets have dashed centrelines.                                                                                                                                               |
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
All tiles share one planar frame. Inside a tile, positions are integer
decimetres from the tile origin. Tile format version 2 writes a signpost as
`[x, y, name id, blade angle, name id, blade angle, …]`, the angle in whole degrees from east, 0–179.

The November 2026 build has 176,866 buildings (2,932 lots redrawn from the lidar because it stands well
above their 2010 height, and 1,296 structures with no City outline), 343,046 trees, 15,671 street lines
and 15,046 signposts at 7,523 intersections. The tiles total 30 MB; the median tile is 0.2 MB and the largest 0.42 MB, so a view
of about nine tiles fetches 2 to 3 MB.

The page loads the tiles within about 900 m of the car's tile edge, nearest first, three at a time,
and drops tiles more than 1.6 km past their edge.

## Limits

- With the Bay Bridge overlay loaded, the driving menu offers the supported upper deck and
  ground street as separate starts. Both runs stop at the limits of their contact mesh.
  See the [Bay Bridge driving pilot](../plans/ui-sf-layered-roads-plan.md#driving-the-bounded-pilot).
- The city car can leave the road when its momentum carries it over a crest. In flight it
  keeps horizontal momentum and falls under gravity until it meets supported ground.
  Airtime cards show the road profile, dashed terrain where the road profile ends, and the
  car's altitude. Each completed card stays for two seconds, and overlapping cards stack.
  The vehicle uses point support as a gameplay approximation.
- Roof shapes come from the April 2023 lidar and the City's 2010 outlines; buildings changed since are
  shown as the lidar saw them, or not at all.
- The canopy test reads the lidar at a coarser density than the per-site studies and flags somewhat
  more roof faces than they did.
- Tall post-2010 towers are flat tiers. The renderer joins adjacent roof faces at their shared
  edges and draws exterior walls and exposed steps. It keeps genuine gaps in the inferred
  footprint and skips faces with unassigned elevations.
- Collision uses the available building outlines and trunks; it does not model kerb impacts,
  tyre suspension, vehicle damage or traffic. Ground and roof heights retain the source limits.
- The tiles are gzipped JSON for ease of reading, 30,169,632 bytes. The asset study's 9.5 MB was an
  estimate for fewer layers in a compact binary encoding under Brotli, not a measurement of these
  tiles; the [size reconciliation](sf-city-map-size-reconciliation.md) measures the same data in each
  form and lists what a smaller format would need.
- Tree crowns are drawn as spheres from low on the trunk, so a blade can sit inside a street tree's
  crown that the City would have pruned clear of the sign.
- The tiles are baked outside this repository from Gerardus's caches; the manifest records the inputs
  and the build.
