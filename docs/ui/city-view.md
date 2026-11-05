# City view

- **Status:** Design, partly built. The [placement files](#placement-files) are built (plan
  Item 1). In the visualiser, switching between the sites of `city/sites.json` ships (toolbar site
  picker; Settings ▸ Servers ▸ City view); the city map itself does not yet.
- **Plan:** [lidar-city-view-plan.md](../plans/lidar-city-view-plan.md) (the work and its order)
- **Companion:** Gerardus
  [CITY-VIEW-TILES](https://github.com/banshee-data/gerardus/blob/main/docs/plans/CITY-VIEW-TILES.md)
  (the city tiles)

A whole-city view, in the macOS visualiser and on the public scene site. It shows the street grid,
the buildings and every site with data. A reader zooms into an intersection to play its recording,
and moves to the next site with data while the city stays drawn around it.

## What it draws

- **City tiles:** the City's building footprints with their 2010 heights, the City's street
  centrelines and 3DEP ground. They are measured spatial data at each source's acquisition time,
  drawn as context for the traffic, at three levels of detail.
- **Street names:** the City's own names, as labels for finding your way around.
- **Sites:** one marker per site with data, with its recording date. A site is placed in the city
  only by an accepted Gerardus pose. A site without one is a marker labelled "not yet placed".
- **Playing sites:** sites in view play in place, simplified with distance. Each plays only inside
  its own coverage, on its own clock and date, coloured by class and never by track. Each
  intersection is recorded on its own. Vehicles are not followed from one intersection to the next.

None of it is a plan, survey or specification of a street, and a feature that is not drawn is not
asserted absent. See [Geometry priors: data purpose](../geometry-priors-data-purpose.md).

## Files it reads

Everything is static, and there are no city-view endpoints:

- the Gerardus city tiles: a `tileset.json` and one `.glb` per tile;
- `city/sites.json`: every site with data and its placement;
- each scene's `assets/placement.json`;
- for the visualiser's live and replay data, the existing velocity.report calls.

The plan gives the frames, the level-of-detail rules and the open questions.

## Placement files

Two static files place the sites. Both are written by scene tooling and read as they are by the
web view and the visualiser, which do no geodesy. Built: plan Item 1. The clients that read them
are not built yet.

### Frames and matrices

| Frame  | Origin and axes                                                                                                    |
| ------ | ------------------------------------------------------------------------------------------------------------------ |
| sensor | The scene's own frame: origin at the sensor, Z up. Points, boxes, background and vectors are all in it.            |
| site   | Gerardus's site frame: east-north-up at the centre of the site's S2 L16 cell, NAD83(2011), NAVD88 0 at origin.     |
| city   | The frame of Gerardus's city tiles (`gerardus-city-tiles-1`): east-north-up at 37.76, −122.44, NAVD88 0 at origin. |

In the site and city frames, NAVD88 heights are taken as ellipsoidal at the origin, as Gerardus
does, so z falls away from NAVD88 height by the earth's curvature (about d²/2R, 2.9 m at 6 km).
The city tiles use the same z, so a scene placed by these matrices sits on the tiles' ground. The
tileset's root transform onto ECEF puts NAVD88 zero at about −32.2 m ellipsoidal; that offset only
places the city on the globe.

Every matrix is 4×4, **row-major**, in metres, with `p_out = M[0:3][0:3] · p_in + M[0:3][3]` and a
last row of `[0, 0, 0, 1]`. Rotation entries are rounded to 1e-12 and translations to 0.1 mm, all
computed in double precision. `sensor_to_city = site_to_city × sensor_to_site`. Keep the
translation in double precision and render camera-relative: a city translation is kilometres, and
Float32 resolves it only to about a millimetre.

`site_to_city` is computed in `tools/s2-archive/city_frame.py` from the site's L16 cell centre and
the city origin through ECEF on GRS80. It is a translation and a small rotation (the two frames'
meridians differ by up to a few hundredths of a degree across the city). Tests hold it to
Gerardus's tile origins and to one site's GeoJSON in the site frame and in WGS 84, each to the
millimetre.

### `assets/placement.json`, per scene

Written by `tools/s2-archive/publish-scenes.py` beside each scene's other assets, from the record
and check the scene's vectors were placed by. Served at `/surveys/<id>/assets/placement.json`.

| Key                         | Meaning                                                                                                                                                                           |
| --------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `version`, `export`, `site` | `1`, `"placement"`, the scene id.                                                                                                                                                 |
| `state`                     | `placed`, or `not_placed` for a scene with no accepted pose checked against it.                                                                                                   |
| `reason`                    | `not_placed` only: why.                                                                                                                                                           |
| `record`                    | The Gerardus registration record: id, state, pose source, model and bundle ids, review time, sigmas, and the check.                                                               |
| `frames`                    | The sensor, site (with its L16 token and origin) and city frames.                                                                                                                 |
| `sensor_to_site`            | The accepted pose: `rotation_wxyz` (unit quaternion), `translation_m` and `matrix`.                                                                                               |
| `site_to_city`              | `matrix`.                                                                                                                                                                         |
| `sensor_to_city`            | `matrix`: the one a client draws the scene by.                                                                                                                                    |
| `heading_deg`               | Azimuth of the sensor's +Y axis, clockwise from true north, in [0, 360).                                                                                                          |
| `up_sensor`                 | True vertical in sensor axes, as in the vectors file.                                                                                                                             |
| `marker`                    | Where to draw the site's marker: `city_enu_m`, and `source` (`accepted_pose`, the sensor's own position, or `eye_read`, with `lat_deg`/`lon_deg`). `null` for an unlocated scene. |

A `not_placed` sidecar has no transforms. Its marker, where it has one, is read by eye from the
field map at neighbourhood precision. It shows where a site is, and a client draws no scene there.

### `city/sites.json`, the city site index

Written by `tools/s2-hilbert/generate-scene-map.mjs` (`make render-scene-data`) to
`public_html/src/city/sites.json` and served at `/city/sites.json`. CI regenerates it and fails on
any difference, as it does for `scenes.json`. Top level: `version`, `city_frame`, `matrix_layout`,
`site_count`, `placed_count` and `sites`. One entry per site with data, published or recorded:

| Key           | Meaning                                                                                                                                                                                   |
| ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `id`, `title` | The site's id and the intersection it is at.                                                                                                                                              |
| `published`   | Whether a scene is published.                                                                                                                                                             |
| `cells`       | `l10`, `l13`, `l16` S2 tokens, or `null` for an unlocated scene.                                                                                                                          |
| `scene`       | `page`, `assets` and `placement` paths, or `null` for a site with no published scene.                                                                                                     |
| `captures`    | The capture's path in the published dataset.                                                                                                                                              |
| `recorded_on` | Recording date in San Francisco, `YYYY-MM-DD`.                                                                                                                                            |
| `placement`   | `state`, `record_id`, `registration_state`, `heading_deg`, `sensor_to_city` and `marker`, copied from the sidecar. A recorded site with no scene is `not_placed` with an eye-read marker. |

Neither file holds a track, a speed, a person, an address or a building's name. Tests list every
key each may carry and fail on any other.
