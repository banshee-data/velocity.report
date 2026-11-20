# San Francisco driving view (`/sf/`)

- **Status:** first release, November 2026.
- **Page:** [`public_html/src/sf/index.njk`](../../public_html/src/sf/index.njk); logic in
  [`sf-drive.js`](../../public_html/src/js/sf-drive.js) and
  [`sf-drive-core.js`](../../public_html/src/js/sf-drive-core.js) (tested in
  [`sf-drive-core.test.mjs`](../../public_html/src/js/__tests__/sf-drive-core.test.mjs)).
- **Data:** `public_html/src/sf/tiles/`, published at `/sf/tiles/`.

The page drives a car anywhere in San Francisco over measured geometry, so a reader can move between
survey sites with the city around them. A chase camera sits behind the car and a mirror shows the road
behind. A readout names the street under the car and the next signposted cross street ahead, and
street-name signs stand at intersections.

## What it shows

| Layer | Source | Treatment |
| --- | --- | --- |
| Buildings | DataSF Building Footprints (`ynuv-fyni`, PDDL-1.0; 2010 heights) and roof planes fitted to USGS 3DEP lidar (flown April 2023, public domain) by Gerardus's city roof bake | Each roof face is a plane over its outline; walls stand on the City's outline. Faces whose lidar returns show canopy are given to a neighbour. Buildings without fitted roofs are boxes at their 2010 height |
| Structures the 2010 outlines lack | 3DEP lidar: solid, planar returns at least 3 m up with no footprint, or a footprint whose lidar top stands well above its 2010 height | Flat tiers at 1.5 m steps, shaded violet. Salesforce Tower and other post-2010 towers appear this way |
| Trees | DataSF Street Tree List (`tkzw-k3nq`, PDDL-1.0) inside the street right of way; 3DEP canopy tops elsewhere | A listed tree takes height and crown from the lidar canopy over it, or a size from its trunk diameter. No trunk stands on the carriageway |
| Roads | DataSF streets (`3psu-pn9h`), Curbs and Islands (`emxt-b6yg`, 2016), Right of Way (`h8n7-e4ns`), all PDDL-1.0 | Asphalt between the kerbs, from each centreline's distance to the first kerb on either side; kerbs raised 15 cm; dashed centrelines on two-way streets |
| Ground | 3DEP class-2 ground via Gerardus's city view tiles | An 8 m grid |
| Signposts | DataSF streets and street nodes | One sign wherever two or more named streets meet, at the nearest kerb corner; DataSF's placeholder names ("Unnamed 186") are left off |

The geometry is measured spatial data in the sense of
[geometry priors: data purpose](../geometry-priors-data-purpose.md): context for traffic data at a
stated acquisition time, not a survey, plan or specification. Positions are NAD83(2011) in EPSG:7131
less a fixed origin, heights NAVD88.

## Tiles

One gzipped JSON per S2 level-13 cell (about 1.1 km across, 157 cells with land), plus
`manifest.json` listing each tile's origin and half-width and the named starting places. A tile owns
the buildings, trees, roads and signs whose point falls inside its cell, so nothing is drawn twice.
All tiles share one planar frame, so they meet without seams. Inside a tile, positions are integer
decimetres from the tile origin.

The November 2026 build has 176,866 buildings (2,932 lots redrawn from the lidar because it stands well
above their 2010 height, and 1,296 structures with no City outline), 343,046 trees, 15,671 street lines
and 7,523 signposts. The tiles total 30 MB; the median tile is 0.2 MB and the largest 0.42 MB, so a view
of about nine tiles fetches 2 to 3 MB.

The page loads the tiles within about 900 m of the car's tile edge, nearest first, three at a time,
and drops tiles more than 1.6 km past their edge.

## Limits

- Roof shapes come from the April 2023 lidar and the City's 2010 outlines; buildings changed since are
  shown as the lidar saw them, or not at all.
- The canopy test reads the lidar at a coarser density than the per-site studies and flags somewhat
  more roof faces than they did.
- Tall post-2010 towers are flat tiers, not their curved forms.
- There is no collision with buildings or kerbs; the car follows the ground grid and pitches with it.
- The tiles are gzipped JSON for ease of reading. A binary encoding of the same geometry measured about a
  third of the size in the asset study, and would be the next step if load time matters.
- The tiles are baked outside this repository from Gerardus's caches; the manifest records the inputs
  and the build.
