# SF tunnels, overpasses and bridges

This is the handoff between the existing SF game tiles and Gerardus's layered
road export. It separates improvements possible with the published data from the World
and Physics work needed to drive above and below the same horizontal position.

- **Status:** the SF game reads the citywide Gerardus road layers directly from Arrow on
  Hydra. The Bay Bridge west span, Fremont Street exit and Fourth Street westbound entrance
  have a playable route. The ten regional reviews still record their surveyed connections
  and gaps.
- **Anchors:** World and Physics for road levels; UI for visibility and markings; Traffic,
  Goals and Data where connectivity, pickups and provenance depend on the selected level.
- **Consumer:** [SF driving view](../ui/sf-driving-view.md).

## What the existing tiles actually contain

The published version-2 city has 157 L13 tiles and 15,671 road records. Every road row
has five fields: name index, class, one-way flag, XY centreline and paired left/right
kerb widths. Road vertices have no road-surface height, stable road/structure ID,
deck level, tunnel flag, portal, clearance, lane count or paint inventory. Each tile
has a single 8 m ground-height grid, plus building prisms, trees, kerbs and signposts.
Tile and quest files are unchanged by the current frontend work.

The renderer paints asphalt on that ground grid; `groundAtWorld` also puts the car
on it. Two roads crossing in XY therefore share the same surface. A ground grid
cannot simultaneously represent a bridge deck, the street beneath it and a tunnel
below that. Roads and kerbs draped over a hill cannot establish a tunnel opening.
Building prisms can also obstruct an entrance or underpass when their semantics are
wrong; deleting a whole building in XY would remove unrelated observed geometry.

## What can be done without another bake

| Improvement                 | Existing evidence                                                     | Limit                                                                                                                                                     |
| --------------------------- | --------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Sharper centre dashes       | Two-way flag, ordered centreline, existing road width                 | Implemented as stylised game guidance, not an inventory of observed paint or a passing rule                                                               |
| Continuous dash phase       | Distance along each existing road polyline                            | Continuous across its vertices and terrain ownership boundaries; no stable ID to join separate road rows                                                  |
| Clear road/ground rendering | Original ground triangles and tile ownership regions                  | Paint is clipped onto those same planes and missing-ground holes, with depth-buffer offset; no new road elevation is inferred                             |
| Clearer camera/HUD          | Existing camera poses, compass target and speed                       | Portrait compass and edge-docked mirror keep the central half of the view clear of panels below the status strip; trees/buildings still occlude naturally |
| At-grade collision/recovery | Existing building bases/outlines, trunks and signposted intersections | Remains the current single-surface game; it cannot select the right deck by height/connectivity                                                           |

Vector markings use 0.30 m-wide, 3 m dashes with 3 m gaps, carry phase through
polyline bends and use an unlit paint material for contrast. They reuse the current
1024 px mobile terrain texture instead of increasing its GPU allocation. The width
is a display choice. One-way roads and paths receive no new centre dashes. Lane
dividers, stop bars, crossings, turn arrows and restrictions need an appropriate
inventory; road width and nearby signposts alone cannot establish them, especially
at an apparent crossing that may be grade-separated.

## Gerardus work to hand off

Build on the existing
[Road Surface plan and Bay Bridge approach pilot](https://github.com/banshee-data/gerardus/blob/main/docs/plans/ROAD-SURFACE.md#14-bay-bridge-approach-pilot),
[road-surface preview implementation](https://github.com/banshee-data/gerardus/blob/main/src/road_surface.rs)
and [RoadSurfacePatch/Chunk contract](https://github.com/banshee-data/gerardus/blob/main/proto/gerardus/align/v1alpha1/align.proto).
That plan already identifies separate elevation groups and tunnel gaps; its current
ground-preview version must not be described as implemented multilevel driving.

1. **Identify surfaces and connectivity.** Assign stable road/surface and structure
   IDs, structure kind, direction and connected endpoints. A crossing in XY becomes
   a junction only when the connected road level agrees. Represent ramps and tunnel
   portals as explicit transitions, including transitions between different L13 tiles.
2. **Export supported contact geometry.** Retain separate height profiles or indexed
   contact meshes for the ground street, each deck and connecting ramps. Use reviewed
   class-17 deck support and compatible class-1 continuation as the existing pilot
   proposes; do not use class-2 ground as a deck or label every high return a bridge.
   Missing ground over water must not erase a supported deck. Preserve unsupported gaps.
3. **Handle tunnels explicitly.** Aerial ground cannot observe the tunnel interior.
   Export known portals and a separately labelled interpolated road profile where
   evidence permits; retain missing/uncertain support. Interior scans or another
   supported source are needed for observed walls, ceiling and clearance. A straight
   join between portals is an inference, not a measured tunnel.
4. **Keep contact surfaces separate from obstacles.** Deck tops belong to Road
   Surface. Undersides, piers, walls, barriers and abutments need supported obstacle
   geometry and shared structure IDs, following the pilot's separate-layer gate.
   Do not extrude a deck to terrain as a building, remove everything under its XY
   footprint, or infer safe headroom from the height of the deck top.
5. **Version a bounded game export.** Reuse the Road Surface products in an optional
   per-L13 sidecar or versioned tile extension, with consistent EPSG:7131 origin,
   NAVD88 heights, units, ownership/halo rules, product hashes and source epochs.
   Carry observed/interpolated/missing support and compatibility version. A targeted
   pilot overlay can ship alongside the existing city tiles. The
   [Bay Bridge preview](#bay-bridge-preview-from-gerardus) uses this arrangement.
6. **Add markings as data when available.** Export lane counts, directions, widths,
   crossings, stop lines and arrows keyed to the same surface ID, with source and
   observation dates. Preserve unknowns rather than deriving legal rules from width.

## Game engine work once the export exists

### Bay Bridge preview from Gerardus

[Gerardus #91](https://github.com/banshee-data/gerardus/pull/91) introduced the optional
L13 overlay. Its real pilot covers the upper deck and the ground street near
Embarcadero–Bryant. The lower deck, ramp joins and clearance still need evidence.

On arrow-worker, generate a revision from the deployed release's
`docs/roads/bay-bridge-approach.json` and the exact SF game `tiles/manifest.json`.
Set `road-overlay --out` to the existing Gerardus city-tile directory's `epochs`
directory. The command verifies the prepared source bundle and publishes the complete
revision by directory rename. The same Gerardus listener serves it at
`/city-tiles/<product_hash>/manifest.json` and `/city-tiles/<product_hash>/<l13>.json`.
Use the hash URL to preserve the ordinary city tiles' current epoch.

Run `make dev-docs` on Hydra and open `/sf/roads/` on `http://localhost:8090`.
Paste the Gerardus manifest URL or pass it as the `overlay` query parameter.
The browser reads the manifest and contact meshes from arrow-worker over HTTP and
checks the base manifest and tile hashes before drawing. Use Above and Under the deck
to inspect the two levels, drag to orbit, and toggle Game terrain to isolate the surfaces.
Gold identifies triangles with interpolated support.

### Driving the bounded pilot

On Hydra, open `/sf/?at=bay-bridge-deck` or `/sf/?at=bay-bridge-street`.
Both starts are also available in the game menu after loading the revision. The browser
fetches the pinned product from arrow-worker and checks it against the game manifest.
In **Map & game → Map sources**, set the Gerardus address or enter a full overlay
manifest URL. The optional SF tile directory must serve the same planar tile pack as
the bundled routes. The menu remains usable if a source fails, so either address can
be changed or cleared before reloading.
The two starts use the usual steering, throttle, cameras and crash recovery. The game
checks contact on the selected mesh at the car centre and at all four wheel positions.
At unsupported edges the car stops and can reverse. **R** returns to the start on that level.
The upper deck and street remain separate throughout the XY crossing. The pilot has no
supported join between them. Its free drive excludes rally pickups and records.

The pinned base tile contains an inferred building that follows the bridge approach as
solid prisms down to ground. The pilot replaces that one record with the verified contact
meshes. The overlay carries no obstacle geometry for this clip. The view shows the deck
top from both sides for orientation. It supplies no measured underside or clearance.

### Regional review in the game

On Hydra, open **Map & game → Open regional road review**. The **Road review region**
and **Published review drive** selectors list the ten regions and their pinned starts.
For example, `/sf/?region=embarcadero-bryant&review=bay-bridge-street` opens the Embarcadero
underpass. For a different deployment, enter the regional catalog manifest URL under
**Map sources**. Use **Road review region → City map** to return to the ordinary game.

The browser fetches the catalog, region manifest and review report from arrow-worker.
The game verifies the base frame and the checksums of the selected revision, report
and contact tiles. The starting profile station may move by up to six metres to fit
all four wheels. A drive without a supported footprint is marked unavailable.
Approaching a confirmed junction preloads its endpoint and destination owners.
The car waits on the current mesh if required contact has not arrived. A missing
strip or unresolved transition stops the car at the edge of supported contact.
The review menu lists the number of unresolved connections per region.

Select **6th–Brannan · game ramp approach** to start on city pavement and drive
onto the fitted strip for CNN 19338000. A 12 m game-authored approach spans the
step at the foot of the strip. It is scoped to the exact Arrow contact geometry
and baked terrain used in this review. The car enters only at the approach line
with pavement and ramp contact agreeing at all four wheels. At the top of the
strip, the game stops at the unresolved freeway connection.

The Bay Bridge pilot has an authored game rail along the sides of its deck. A shallow
strike slows the car and lets it move along the rail. A larger strike rebounds it.
The car body meets the same finite rail segments drawn in the game while its tyres
are still on the deck.
The game crashes at 12 m/s into the rail, or at 5.5 m/s into the rail when the car
is travelling at least 28 m/s. Where the game has no rail, driving beyond the side
of a fitted strip releases the car into free fall. It can land on a lower loaded
surface. Profile ends and holes in contact
still stop the car. These are game rules. The review revisions carry no guardrail
inventory or measured clearance.

Under the Bay Bridge, the game uses city pavement where the fitted street and
baked terrain agree within 0.45 m. That allows the tyres to cross small holes in
the street contact and lets the drive continue onto the city map at the end of
the clip. The deck stays separate.

The catalog is revision
`sf-road-index-80faea50f62a8c6b81f1d73340041af9281f5f576b7638bf489b98c839fd19a8`.
Arrow's review reports ten confirmed ground joins. The eastbound Harrison–Essex
lower-deck merge and the paired freeway moves at 6th–Brannan still lack supported joins.
Regional findings gate promotion of these connections. A compatible road sidecar
can reuse the pinned base tiles. If fixes change the base terrain or obstacle records,
publish a new base manifest and regenerate the game assets keyed to it, including the
quest route and coverage manifest. A lane inventory and recorded survey replays have
their own data requirements.

#### Hydra browser checks

Chromium loaded one published drive in each of the ten regions. In the delayed
cross-tile checks, 6th–Brannan held at 3.446 m NAVD88 and entered the next strip at
3.451 m. Geary–Masonic held at 84.469 m and entered the next strip at 84.400 m.
The Bay Bridge crossing stayed on its chosen level through the vertical overlap
and reset on that level. The upper drive ranged from 52.12 to 55.39 m and the
underpass from 3.19 to 3.29 m. The pilot rebounded from its rail at 52.333 m during
the slower run.
The faster run triggered crash recovery. At 6th–Brannan, steering off the open
side released the car at 6.675 m and the browser observed at least 0.5 m of fall.
Driving straight on the same strip stopped at the contact gap at 4.808 m.
These checks exercise the game and the exact Arrow revision. Regional acceptance
also needs evidence for the unresolved freeway moves and coverage gaps.

### Citywide bake and Bay Bridge game route

The [published Arrow revision](http://arrow-worker:50061/city-tiles/sf-city-roads-97993185300b116bf296169b1f0e88f7685e9fe18af57d696c3b89742813e3cc/manifest.json)
contains 15,491 road profiles, 9,875 ground continuations, 273 provisional overpass spans
and 17 provisional bridge spans across 157 tiles. Arrow verified all 161 files and 13 regional
references over HTTP. The game bake embeds 248 supported continuous runs in
34 of the 157 game tiles. Every owning tile carries the complete run so the display
can extend across a tile edge when the next tile is unavailable. The game draws one
copy per run ID. The bake retains gaps where the source lacks supported positions.

**Bay Bridge approach** is available in **Start from**. It starts westbound near Yerba Buena Island at 50 mph in self-driving mode.
Fremont Street exit and the city intersections start at rest in the same picker.
Driving input opens the Meat mode confirmation.
The route follows the west span to the city approach, takes the Fremont Street exit,
follows Fremont, Howard, Second and Harrison, and rejoins westbound I-80 at Fourth Street.
The named route is 6.22 km.

Run the [game tile embedder](../../tools/sf-drive/embed-road-layers.py) with
`--source` set to the immutable Gerardus revision directory or URL,
`--base` set to the matching original game tile directory, and
`--quest` set to the game's quest route. It writes the full set of compressed tiles,
the patched manifest and quest route, and the checksummed game drive under
`--output`. Review and copy those files into `public_html/src/sf/`. Publish them
with the game on its own origin. Playable ramps and overpasses need continuous
supported contact, directed same-level joins, shoulder and rail metadata, and
drive checks for each legal direction.

The citywide road rebake samples the corrected Dogpatch terrain with the game's
triangle split, leaves missing contact in the 23rd Street shadow and retains the
reviewed display strip. It records four DataSF tunnel alignments as unresolved
and leaves their interior profiles without heights. The game omits their aerial
ground drapes. Tunnel Avenue remains a surface road. The rebake retains the
existing regional references where the referenced base tiles are byte-identical.
See the [rebake inputs and output hashes](../ui/sf-city-map.md#citywide-road-rebake).
The [20-course route audit](../ui/sf-road-route-audit.md) gives reproduction
coordinates for the 5th Street ramp contact gap, the Dogpatch grade flags and the
unresolved tunnel approaches.

### Baked I-280 contact trial

The game now has a baked contact path for the 5th Street southbound I-280
entrance and the mainline as far as the next junction near Mariposa. The
[Gerardus game exporter](https://github.com/banshee-data/gerardus/pull/93) pins
road CNNs 19330000 and 19339000 and requires shared DataSF node 50100000.
It matches the entry to ground at station 48 m. A short interpolated section
at stations 305.88–333.33 m follows the lower ramp through the upper-deck
occlusion. The authored approach and interpolation stay in the game product
with their own provenance.

Playable strips carry joint widths and bearings and appear whole in each
intersecting tile. A grounded car can select the strip only through a declared
terrain portal or a compatible baked join. Surface-street pickups and
checkpoints remain attached to surface streets. The normal browser game reads
these strips from its static tiles and can reuse them offline.

Run `node tools/sf-drive/replay-baked-structure.mjs` to exercise the pavement
entry, the lower ramp and the merge using the bundled contacts and collision
geometry. The [Chrome smoke test](../../tools/sf-drive/smoke-baked-structure.cjs)
checks entry through manual controls and that the overlapping street remains
at 3.13 m. In the replay, the car enters at 2.87 m and joins the mainline at
18.15 m. The [route audit](../ui/sf-road-route-audit.md) describes the
original wrong-level failure.

### Southbound I-280 at 18th Street

**World and Physics.** The playable contact continues from the southbound
mainline to the common off-ramp, then separates into Pennsylvania Avenue and
18th Street. The branches share one transverse seam and each has a ground
portal. A car can only choose a branch reached through a declared connection.
When both ribbons cover the wheels at the split, the solver compares the car’s
direction of travel with the next eight metres of each branch. An unloaded
owner is requested before the join.

The measured alignment and grade sources are the pinned DataSF centrelines and
3DEP returns. Both street terminals use the baked ground height. The 18th
Street portal sits four metres before the mapped end, where the ramp and
pavement are already at matching levels and the driver has room to turn on
the street. Three new strips live in tile `808f7fcc`. The adjacent 5th Street
entry keeps its existing cross-tile contact, and the bake reuses the current
emoji placements.

**Data.** The contact product is
`bf8877c35256e0e73e75eb23240ec79fb2afd6b10ac9b559391fe5f396ef16eb`.
Run the [two-course exit check](../../tools/sf-drive/road-audit-i280-exits.json)
with `node tools/sf-drive/audit-road-routes.mjs --plan=tools/sf-drive/road-audit-i280-exits.json`.
It requires each declared join and at least 20 m on exterior pavement after
the exit. The Pennsylvania Avenue run collected both emojis. The 18th Street
run continued 1.73 km on city streets before a tree stopped it near Potrero.
Neither baked branch flagged a dip over the surrounding eight-metre chord.
Their full baked approach profiles peaked at 14.0% and 8.1%, respectively.

### Broadway tunnel contact

**World and Data.** DataSF CNNs `3186201` and `3186101` carry opposite travel
directions, with the same street endpoints and about 3.05 m separation across
most of the mapped corridor. The [FHWA tunnel inventory](https://data.transportation.gov/Roadways-and-Bridges/National-Tunnel-Inventory/64m4-zwar)
lists two bores and a length of 1,954 ft for `34C0006`. The original project
manager’s [1950 construction account](https://www.usmodernist.org/WCN/WCN-1950-12.pdf)
records a 1,616 ft main bore and a maximum grade of 3%. The game trial models
the main bore separately from both approaches. Its interior floor interpolates
between the lower bands observed at the approaches. The surface elevations
above the tunnel are distinct measurements.

The April 2023 3DEP sample found pavement at 40.5 m NAVD88 near station 94 m,
46.3 m at 142 m and 55.5 m around 676–700 m on the westbound alignment. The
nominal FHWA western point projects to station 689 m. A second upper band at
about 62 m near station 735 m marks the Hyde Street crossing. The trial
checksums the DataSF source cell and fits each outer handoff to the exact game
terrain. The two lane widths and their offsets are game parameters.

**Physics.** The first westbound drive selected its baked floor and stopped
at (2531.7, 4132.1), 40.45 m. The collision was against the game’s inferred
building volume, which starts at 40.5 m and fills the lane below the health
centre. Public Works documents [the building over the road at the east portal](https://bidopportunities.apps.sfdpw.org/CaseLoad/GetFile/6123).
The first eastbound drive stayed on exterior pavement and stalled at
(1759.4, 4007.5) near a median tree. A diagnostic drive steered around the
median trees and suppressed the suspected overhead collisions. Both directions
crossed the baked floor and completed their emoji itineraries in that
diagnostic. Release acceptance requires the normal collision path and
confirmation that traffic on the upper streets retains its own level.

The [Gerardus exporter](https://github.com/banshee-data/gerardus/pull/96)
includes the overhead clearance in the contact product. The game baker
checks each of the three affected building records against the terrain tile
before applying the replacement. The pieces to either side keep their original
base. Existing roof vertices keep their elevations, and new cut edges use the
roof plane. The manifest pins the combined game contact product at
`b1bf53a51b03a6478e38ddcbf639e655bb03fbea749a6a1d3ab09c046b3bec00`.

Both courses completed with normal collision checks. The westbound course
continued 2.45 km past the exterior handoff, and the eastbound course
continued 567 m. The [Broadway course set](../../tools/sf-drive/road-audit-broadway-routes.json)
pins the candidate manifest. The approach waypoints stay to either side of
the median trees. Run the set with
`node tools/sf-drive/audit-road-routes.mjs --plan=tools/sf-drive/road-audit-broadway-routes.json`.
The game’s collision test checks that the roadway clears the eastern building
while the upper volume stays solid. The main floor is a game interpolation.

### Stockton tunnel contact

**World and Physics.** The Stockton trial bakes a distinct tunnel floor and both
pavement joins into the same tile as the exterior street. The municipal portal
positions and the lower pavement returns at the two mouths anchor the profile.
The interior and its walls, lighting and ceiling are game geometry. The exported
road measurements retain the original unobserved interior.

**UI and Data.** The player follows the road into a tunnel camera and returns to
their chosen view on exit. A crash uses the selected tunnel floor for contact.
The local [Chrome smoke](../../tools/sf-drive/smoke-stockton-tunnel.cjs) traversed
Stockton both ways and checked that the street above kept its 50.4 m ground
altitude. The northbound and southbound exits returned to exterior pavement
near 42.2 m and 28.1 m NAVD88 respectively.

The [federal tunnel inventory](https://data.transportation.gov/Roadways-and-Bridges/National-Tunnel-Inventory/64m4-zwar)
identifies additional structures with no current game tunnel contact, including
Yerba Buena, the Battery and Main Post bores, and Geary Boulevard. The inventory
has portal locations for candidate matching. Contact, direction, clearance and
compatible joins still need their own evidence.

Both rails at the Fourth Street entrance use a rebound for side impacts at any speed.
The car loses speed and stays on the ramp. The replay checks a 50 mph strike on each
rail before driving the six connected sections from the rolling bridge start.

The [route baker](../../tools/sf-drive/bake-city-drive.py) adds one checksummed game
asset to the immutable Gerardus city revision. It uses the fitted west-span profiles and
directed streets, with authored lane widths, rails, and interpolation through gaps on
the city approach and ramps. It adjusts five intersecting scenery records only in the
route mode and only when their pinned base signatures match. The published measured
layers retain their original support and routing status. The eastbound Essex lower-deck
merge and the Bay Bridge east span towards Oakland remain outside the playable route.

The bake smooths the authored path before cutting it into road sections. It
samples the result in the existing vector format, gives neighbouring sections
the same position at their join, and blends ramp grades into the ground pavement.
The first smoothing pass reduced the largest change of bearing between adjacent
stations from 58.7° to 17.5°. The ramp-join pass samples pavement heights at
about one-metre intervals and matches the ramp to the street heading and grade
at each join. The Fremont exit join now changes bearing by 3.74° (previously
17.53°) and grade by 0.43 percentage points (previously 1.00). The Fourth Street
entrance join changes bearing by 2.57° (previously 12.29°) and grade by 0.05
percentage points (previously 0.34). Both joins share a single elevation.
The car's steering angle and rate now carry across key presses and releases.
At 45 mph, the current tyre model needs roughly 54 m of radius, well above
what the Fremont and city turns provide.

If an analytic alignment becomes a shared Gerardus product, add optional
stationed line and cubic curve segments to the alignment contract. Give each
segment an owner and a stable ID, carry its station bounds and endpoint tangents,
and sample the same curve on both sides of a tile boundary. Keep the elevation
profile and support keyed to that station axis and to the selected road level.
The existing sampled profile can stay as the fallback for older clients.

The bridge view preloads four waterfront and downtown L13 tiles. A distant
pass draws their tall buildings during the crossing and fades as the normal
city tiles become visible at the San Francisco landing.

The Fremont exit has an open left edge in the westbound direction and a rail
on the right. **Start from → Fremont Street exit** places the car near the
lower end of the ramp. Steering over the open edge releases the car into a
fall, with the normal impact rules applied at the first lower surface.

#### Airtime replay checks

The earlier graph reprojected airborne positions onto the nearest centreline
segment, which could move the trace backwards around a bend. Its road sample
also ended at the first centreline record, even when terrain farther ahead was
loaded. The graph now uses travelled horizontal distance, joins compatible
sections of the same street, and draws dashed terrain where the road profile
ends. An airborne card becomes a touchdown card for two seconds. Flights that
overlap in display time stack, with an independent expiry for each touchdown.

Run `node tools/sf-drive/replay-airtime.mjs --examples` from the repository
root to repeat the 50 scripted flights. The replay pins the Arrow revision
and verifies the local base tile hashes before sampling. The 120 Hz runs check
the trace order, the final height, a usable elevation profile, and the exact
expiry time.

| Replay group   | Flights | Used dashed terrain | Had a road or deck trace | Hard landings |
| -------------- | ------: | ------------------: | -----------------------: | ------------: |
| Practice crest |      10 |                   0 |                       10 |             0 |
| SF streets     |      20 |                   9 |                       15 |             2 |
| Fremont exit   |      20 |                  20 |                       20 |            19 |

The table uses the passive chassis and the normal closing speed at wheel contact.
The Fremont sweep includes 7.5 and 12.5 m/s approaches to cover 20 qualifying
flights with the shared tyre grip model. The earlier point-contact run recorded
zero hard landings on SF streets and 13 on the Fremont exit.

Chrome also showed a grounded landing on pavement about 0.4 seconds after
driving off the lower ramp. The scripted results describe game contact on the
pinned geometry and controlled inputs.

For the stacked HUD check, copy `tools/sf-drive/airtime-browser.html` to
`public_html/_site/sf-airtime-check/index.html` once the dev server is running,
then open `/sf-airtime-check/`. **Run timing check** plays two scripted jumps
through the game HUD. Chrome removed the charts 2,000 ms and 2,004 ms after
their respective touchdowns. The same check advances the clock to 1,999 ms and
2,000 ms after each touchdown to verify the exact expiry boundaries.
**Show both charts** pauses the replay with both touchdown cards visible.

Run `node tools/sf-drive/replay-city-drive.mjs` to repeat the physics check against
the bundled route and matching local tiles. The check asserts continuous contact and collision-free progression across all
six sections. It reaches the westbound continuation at 11.08 m NAVD88 after dropping
to 7.31 m at the Fremont street join and ascending from 4.72 m at Fourth Street.

On Hydra, one uninterrupted Chrome drive finished grounded on westbound I-80 at
12.59 m NAVD88, after streaming 13 elevated layer tiles from Arrow. A separate
drive stayed on the Embarcadero street beneath the deck at 3.32 m. The 6th–Brannan
approach reached its deck at 5.47 m, and Geary–Masonic crossed its ground owner
seam before reaching the end of the published contact.

### General driving integration

- [ ] Load/validate the optional surface products and include their compressed bytes
      and hashes in asset accounting. Keep old city tiles readable. Show unsupported
      areas honestly rather than claiming the flattened fallback is structure-aware.
- [ ] Extend the pilot's selected-surface contact to explicit connected ramps and portal
      transitions across tiles. Brake when the required next surface has not loaded.
- [ ] Render decks and ramps on their own supported surfaces, and openings at known
      tunnel portals without flattening the terrain globally. Project markings onto
      the selected surface. Camera cutaways, if added, affect presentation only.
- [ ] Extend collision/headroom checks for ceilings, piers and barriers. Existing 2D
      building/trunk collision and one ground height are insufficient for an underpass.
- [ ] Make street lookup, intersection connections, pickups and recovery checkpoints
      surface-aware. Do not award an emoji on the deck while driving beneath it or
      reset a car onto a nearby disconnected level. Scope records to the revised route
      and surface products; retain the documented crash interval and timer semantics.
- [ ] Keep future Traffic policies on connected lanes at the same level. An apparent
      crossing does not imply a following/yielding interaction. Add vertical seeker
      cues only when a supported target and route establish the right surface.

## Acceptance before calling structures drivable

Use a deterministic fixture containing two crossing decks, the ground road, a ramp,
a tunnel portal/interior, an unsupported gap and neighbouring building/tree geometry.
Test both driving directions and headings, with fixed above/below/in-car views.
Verify no level switching at crossings; continuous ramp/portal contact; correct street,
pickup and recovery ownership; true obstacles and retained under-deck space; no
floating paint; missing-surface braking; and equivalent results when neighbouring
tiles stream in a different order. Include a cross-L13 transition and interrupted load.

Freeze input/product hashes, frame, evidence/support labels and source dates. Validate
geometry against independent held-out evidence as in the Gerardus pilot, and distinguish
display tolerances from physical clearance or safety claims. Measure frame time, contact
query cost and CPU/GPU memory on a stated mobile workload before expanding the pilot.
